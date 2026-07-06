package friends

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/zsrv/goscape/pkg/friendspb"
	util "github.com/zsrv/goscape/pkg/util/jstring"
)

// nameToBase37 is a slice-5a-test-only convenience for inline username
// literals. The handler's RELAY_* methods are username-agnostic at the
// routing layer; the value is opaque uint64.
func nameToBase37(s string) uint64 { return util.ToBase37(s) }

// newTestHandler returns a handler wired to a fresh in-memory repos,
// configured with WorldPlayerLimit=10.
func newTestHandler(t *testing.T) *handler {
	t.Helper()
	log := noopLogger()
	return &handler{
		repos: newRepositories(createTestDB(t)),
		subs:  newSubscriptions(log),
		cfg: Config{
			WorldPlayerLimit: 10,
		},
		log: log,
	}
}

func TestHandler_WorldConnect_OK(t *testing.T) {
	h := newTestHandler(t)
	if _, err := h.WorldConnect(t.Context(), &friendspb.WorldConnectRequest{
		WorldId: 1,
		Profile: "main",
	}); err != nil {
		t.Fatalf("WorldConnect: %v", err)
	}
	// Indirect verification: a Register on world 1 must now succeed.
	if !h.repos.get("main").Register(1, 0xAAAA, 0, 0) {
		t.Errorf("Register after WorldConnect: got false, want true")
	}
}

// TestHandler_WorldConnect_AnyProfileAccepted verifies that TS 244 removed
// the server-side profile-mismatch reject: any profile string is accepted.
// The 225 server gated WorldConnect on a configured NodeProfile; TS 244
// removed that field and the server accepts any profile string (verified at
// FriendServer.ts:92-103, pin 9aadcec4).
func TestHandler_WorldConnect_AnyProfileAccepted(t *testing.T) {
	h := newTestHandler(t)
	for _, profile := range []string{"main", "beta", "dev", ""} {
		if _, err := h.WorldConnect(t.Context(), &friendspb.WorldConnectRequest{
			WorldId: 1,
			Profile: profile,
		}); err != nil {
			t.Errorf("WorldConnect profile %q: got error %v, want nil", profile, err)
		}
	}
}

func TestHandler_PlayerLogin_BeforeWorldConnect_LazyInit(t *testing.T) {
	h := newTestHandler(t)
	resp, err := h.PlayerLogin(t.Context(), &friendspb.PlayerLoginRequest{
		WorldId:     1,
		Profile:     "main",
		Username37:  0xAAAA,
		PrivateChat: 0,
		StaffLvl:    0,
	})
	if err != nil {
		t.Fatalf("PlayerLogin without prior WorldConnect: %v", err)
	}
	if !resp.Accepted {
		t.Errorf("Accepted: got false, want true")
	}
	if got := h.repos.get("main").GetWorld(0xAAAA); got != 1 {
		t.Errorf("GetWorld after lazy-init login: got %d, want 1", got)
	}
}

func TestHandler_PlayerLogin_PrivateChatCoercion(t *testing.T) {
	h := newTestHandler(t)
	if _, err := h.WorldConnect(t.Context(), &friendspb.WorldConnectRequest{WorldId: 1, Profile: "main"}); err != nil {
		t.Fatalf("WorldConnect: %v", err)
	}
	if _, err := h.PlayerLogin(t.Context(), &friendspb.PlayerLoginRequest{
		WorldId:     1,
		Profile:     "main",
		Username37:  0xAAAA,
		PrivateChat: 99, // invalid -> coerce to 0 (ON)
		StaffLvl:    0,
	}); err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if got := h.repos.get("main").GetChatMode(0xAAAA); got != 0 {
		t.Errorf("GetChatMode after invalid coercion: got %d, want 0", got)
	}
}

func TestHandler_PlayerLogin_PlayerCapAccepted_False(t *testing.T) {
	h := newTestHandler(t)
	h.cfg.WorldPlayerLimit = 2
	if _, err := h.WorldConnect(t.Context(), &friendspb.WorldConnectRequest{WorldId: 1, Profile: "main"}); err != nil {
		t.Fatalf("WorldConnect: %v", err)
	}
	for i := uint64(1); i <= 2; i++ {
		resp, err := h.PlayerLogin(t.Context(), &friendspb.PlayerLoginRequest{WorldId: 1, Profile: "main", Username37: i})
		if err != nil {
			t.Fatalf("PlayerLogin %d: %v", i, err)
		}
		if !resp.Accepted {
			t.Errorf("Accepted #%d: got false, want true", i)
		}
	}
	resp, err := h.PlayerLogin(t.Context(), &friendspb.PlayerLoginRequest{WorldId: 1, Profile: "main", Username37: 3})
	if err != nil {
		t.Fatalf("PlayerLogin beyond cap: %v", err)
	}
	if resp.Accepted {
		t.Errorf("Accepted past cap: got true, want false")
	}
}

func TestHandler_PlayerLogout_Idempotent(t *testing.T) {
	h := newTestHandler(t)
	if _, err := h.PlayerLogout(t.Context(), &friendspb.PlayerLogoutRequest{
		WorldId:    1,
		Profile:    "main",
		Username37: 0xDEADBEEF,
	}); err != nil {
		t.Fatalf("PlayerLogout on unknown player: %v", err)
	}
}

func TestHandler_PlayerLogout_RemovesPlayer(t *testing.T) {
	h := newTestHandler(t)
	if _, err := h.WorldConnect(t.Context(), &friendspb.WorldConnectRequest{WorldId: 1, Profile: "main"}); err != nil {
		t.Fatalf("WorldConnect: %v", err)
	}
	if _, err := h.PlayerLogin(t.Context(), &friendspb.PlayerLoginRequest{WorldId: 1, Profile: "main", Username37: 0xAAAA}); err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if _, err := h.PlayerLogout(t.Context(), &friendspb.PlayerLogoutRequest{WorldId: 1, Profile: "main", Username37: 0xAAAA}); err != nil {
		t.Fatalf("PlayerLogout: %v", err)
	}
	if got := h.repos.get("main").GetWorld(0xAAAA); got != 0 {
		t.Errorf("GetWorld after logout: got %d, want 0", got)
	}
}

func TestHandler_ChatSetMode_UpdatesState(t *testing.T) {
	h := newTestHandler(t)
	if _, err := h.WorldConnect(t.Context(), &friendspb.WorldConnectRequest{WorldId: 1, Profile: "main"}); err != nil {
		t.Fatalf("WorldConnect: %v", err)
	}
	if _, err := h.PlayerLogin(t.Context(), &friendspb.PlayerLoginRequest{WorldId: 1, Profile: "main", Username37: 0xAAAA, PrivateChat: 0}); err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if _, err := h.ChatSetMode(t.Context(), &friendspb.ChatSetModeRequest{
		WorldId:     1,
		Profile:     "main",
		Username37:  0xAAAA,
		PrivateChat: 2,
	}); err != nil {
		t.Fatalf("ChatSetMode: %v", err)
	}
	if got := h.repos.get("main").GetChatMode(0xAAAA); got != 2 {
		t.Errorf("GetChatMode after ChatSetMode(OFF): got %d, want 2", got)
	}
}

func TestHandler_ChatSetMode_PrivateChatCoercion(t *testing.T) {
	h := newTestHandler(t)
	if _, err := h.WorldConnect(t.Context(), &friendspb.WorldConnectRequest{WorldId: 1, Profile: "main"}); err != nil {
		t.Fatalf("WorldConnect: %v", err)
	}
	if _, err := h.PlayerLogin(t.Context(), &friendspb.PlayerLoginRequest{WorldId: 1, Profile: "main", Username37: 0xAAAA, PrivateChat: 0}); err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if _, err := h.ChatSetMode(t.Context(), &friendspb.ChatSetModeRequest{
		WorldId:     1,
		Profile:     "main",
		Username37:  0xAAAA,
		PrivateChat: 99,
	}); err != nil {
		t.Fatalf("ChatSetMode: %v", err)
	}
	if got := h.repos.get("main").GetChatMode(0xAAAA); got != 0 {
		t.Errorf("GetChatMode after coercion: got %d, want 0", got)
	}
}

func TestHandler_FriendlistAdd_Persists(t *testing.T) {
	h := newTestHandler(t)
	ctx := t.Context()
	seedAccount(t, h.repos.db, 0xAAAA)
	seedAccount(t, h.repos.db, 0xBBBB)
	if _, err := h.FriendlistAdd(ctx, &friendspb.FriendlistAddRequest{
		WorldId:          1,
		Profile:          "main",
		Username37:       0xAAAA,
		TargetUsername37: 0xBBBB,
	}); err != nil {
		t.Fatalf("FriendlistAdd: %v", err)
	}
	got, err := h.repos.get("main").GetFriends(ctx, 0xAAAA)
	if err != nil {
		t.Fatalf("GetFriends: %v", err)
	}
	if len(got) != 1 || got[0] != 0xBBBB {
		t.Errorf("GetFriends after FriendlistAdd: got %v, want [0xBBBB]", got)
	}
}

func TestHandler_FriendlistDel_RemovesEntry(t *testing.T) {
	h := newTestHandler(t)
	ctx := t.Context()
	seedAccount(t, h.repos.db, 0xAAAA)
	seedAccount(t, h.repos.db, 0xBBBB)
	if err := h.repos.get("main").AddFriend(ctx, 0xAAAA, 0xBBBB); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	if _, err := h.FriendlistDel(ctx, &friendspb.FriendlistDelRequest{
		WorldId:          1,
		Profile:          "main",
		Username37:       0xAAAA,
		TargetUsername37: 0xBBBB,
	}); err != nil {
		t.Fatalf("FriendlistDel: %v", err)
	}
	got, err := h.repos.get("main").GetFriends(ctx, 0xAAAA)
	if err != nil {
		t.Fatalf("GetFriends: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("GetFriends after FriendlistDel: got %v, want empty", got)
	}
}

func TestHandler_IgnorelistAdd_Persists(t *testing.T) {
	h := newTestHandler(t)
	ctx := t.Context()
	seedAccount(t, h.repos.db, 0xAAAA)
	if _, err := h.IgnorelistAdd(ctx, &friendspb.IgnorelistAddRequest{
		WorldId:          1,
		Profile:          "main",
		Username37:       0xAAAA,
		TargetUsername37: 0xBBBB,
	}); err != nil {
		t.Fatalf("IgnorelistAdd: %v", err)
	}
	got, err := h.repos.get("main").GetIgnores(ctx, 0xAAAA)
	if err != nil {
		t.Fatalf("GetIgnores: %v", err)
	}
	if len(got) != 1 || got[0] != 0xBBBB {
		t.Errorf("GetIgnores after IgnorelistAdd: got %v, want [0xBBBB]", got)
	}
}

func TestHandler_IgnorelistDel_RemovesEntry(t *testing.T) {
	h := newTestHandler(t)
	ctx := t.Context()
	seedAccount(t, h.repos.db, 0xAAAA)
	if err := h.repos.get("main").AddIgnore(ctx, 0xAAAA, 0xBBBB); err != nil {
		t.Fatalf("AddIgnore: %v", err)
	}
	if _, err := h.IgnorelistDel(ctx, &friendspb.IgnorelistDelRequest{
		WorldId:          1,
		Profile:          "main",
		Username37:       0xAAAA,
		TargetUsername37: 0xBBBB,
	}); err != nil {
		t.Fatalf("IgnorelistDel: %v", err)
	}
	got, err := h.repos.get("main").GetIgnores(ctx, 0xAAAA)
	if err != nil {
		t.Fatalf("GetIgnores: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("GetIgnores after IgnorelistDel: got %v, want empty", got)
	}
}

// TestPrivateMessage_NoSubscription pins TS-faithful silent-drop on
// absent recipient subscription. Mirrors FriendServer.ts:482-484
// (`if (!socket) return Promise.resolve()`). The registry's send method
// implements the no-op (subscriptions.go:85-87).
func TestPrivateMessage_NoSubscription(t *testing.T) {
	h := newTestHandler(t)
	// Both endpoints need accounts under the TS 9aadcec4 re-key —
	// LogPrivateMessage now resolves from/to against the central database.
	seedAccount(t, h.repos.db, 0xAAAA)
	seedAccount(t, h.repos.db, 0xBBBB)
	// No SubscribeUpdates call for the target — registry is empty for
	// username37=0xBBBB.
	if _, err := h.PrivateMessage(t.Context(), &friendspb.PrivateMessageRequest{
		WorldId:          1,
		Profile:          "main",
		Username37:       0xAAAA,
		TargetUsername37: 0xBBBB,
		StaffLvl:         0,
		PmId:             1,
		Chat:             "hi",
		Coord:            12345,
	}); err != nil {
		t.Fatalf("PrivateMessage: %v", err)
	}
}

// testStream is a minimal friendspb.FriendsService_SubscribeUpdatesServer
// impl that captures Send calls into a channel. Cancel ctx to stop the
// handler's drain loop.
type testStream struct {
	grpc.ServerStream
	ctx    context.Context
	cancel context.CancelFunc
	out    chan *friendspb.FriendsUpdate
}

func newTestStream(t *testing.T) *testStream {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	return &testStream{ctx: ctx, cancel: cancel, out: make(chan *friendspb.FriendsUpdate, 32)}
}

func (s *testStream) Context() context.Context { return s.ctx }
func (s *testStream) Send(u *friendspb.FriendsUpdate) error {
	select {
	case s.out <- u:
	default:
	}
	return nil
}

// recvWithin waits up to d for the next update on s.out; t.Fatal on
// timeout.
func (s *testStream) recvWithin(t *testing.T, d time.Duration) *friendspb.FriendsUpdate {
	t.Helper()
	select {
	case u := <-s.out:
		return u
	case <-time.After(d):
		t.Fatalf("timed out waiting for update")
		return nil
	}
}

func TestSubscribeUpdates_InitialSnapshots(t *testing.T) {
	db := createTestDB(t)
	repos := newRepositories(db)
	r := repos.get("main")
	log := noopLogger()
	cfg := Config{WorldPlayerLimit: 100}
	h := &handler{repos: repos, subs: newSubscriptions(log), cfg: cfg, log: log}
	r.InitializeWorld(1, 100)
	r.Register(1, 100, 0, 0)
	seedAccount(t, db, 100)
	seedAccount(t, db, 200)
	if err := r.AddFriend(t.Context(), 100, 200); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	if err := r.AddIgnore(t.Context(), 100, 300); err != nil {
		t.Fatalf("AddIgnore: %v", err)
	}

	stream := newTestStream(t)
	errc := make(chan error, 1)
	go func() {
		errc <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 1, Profile: "main", Username37: 100}, stream)
	}()
	t.Cleanup(func() {
		stream.cancel()
		<-errc
	})

	// First message: FriendlistUpdate with one entry for 200.
	u1 := stream.recvWithin(t, 2*time.Second)
	fl, ok := u1.Update.(*friendspb.FriendsUpdate_Friendlist)
	if !ok {
		t.Fatalf("first update = %T, want FriendsUpdate_Friendlist", u1.Update)
	}
	if len(fl.Friendlist.Entries) != 1 || fl.Friendlist.Entries[0].Username37 != 200 {
		t.Fatalf("entries = %v, want one entry for 200", fl.Friendlist.Entries)
	}

	// Second message: IgnorelistUpdate with [300].
	u2 := stream.recvWithin(t, 2*time.Second)
	il, ok := u2.Update.(*friendspb.FriendsUpdate_Ignorelist)
	if !ok {
		t.Fatalf("second update = %T, want FriendsUpdate_Ignorelist", u2.Update)
	}
	if len(il.Ignorelist.Username37) != 1 || il.Ignorelist.Username37[0] != 300 {
		t.Fatalf("ignored = %v, want [300]", il.Ignorelist.Username37)
	}
}

func TestPlayerLogin_BroadcastsToFollowers(t *testing.T) {
	db := createTestDB(t)
	repos := newRepositories(db)
	r := repos.get("main")
	log := noopLogger()
	cfg := Config{WorldPlayerLimit: 100}
	h := &handler{repos: repos, subs: newSubscriptions(log), cfg: cfg, log: log}
	r.InitializeWorld(1, 100)
	seedAccount(t, db, 100)
	seedAccount(t, db, 200)
	// Follower 100 friended target 200.
	if err := r.AddFriend(t.Context(), 100, 200); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	// 100 is online so the subscription has a presence row to query.
	r.Register(1, 100, 0, 0)

	// 100 subscribes.
	stream := newTestStream(t)
	errc := make(chan error, 1)
	go func() {
		errc <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 1, Profile: "main", Username37: 100}, stream)
	}()
	t.Cleanup(func() {
		stream.cancel()
		<-errc
	})
	// Drain initial snapshots.
	stream.recvWithin(t, 2*time.Second)
	stream.recvWithin(t, 2*time.Second)

	// 200 logs in on world 1.
	if _, err := h.PlayerLogin(t.Context(), &friendspb.PlayerLoginRequest{
		WorldId:     1,
		Profile:     "main",
		Username37:  200,
		PrivateChat: 0,
		StaffLvl:    0,
	}); err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}

	// 100's stream should see a one-entry FriendlistUpdate naming 200, world=1.
	u := stream.recvWithin(t, 2*time.Second)
	fl, ok := u.Update.(*friendspb.FriendsUpdate_Friendlist)
	if !ok {
		t.Fatalf("update = %T, want FriendsUpdate_Friendlist", u.Update)
	}
	if len(fl.Friendlist.Entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(fl.Friendlist.Entries))
	}
	e := fl.Friendlist.Entries[0]
	if e.Username37 != 200 || e.WorldId != 1 {
		t.Fatalf("entry = (%d, %d), want (1, 200)", e.WorldId, e.Username37)
	}
}

func TestBroadcast_ChatModeOffHidesWorld(t *testing.T) {
	db := createTestDB(t)
	repos := newRepositories(db)
	r := repos.get("main")
	log := noopLogger()
	cfg := Config{WorldPlayerLimit: 100}
	h := &handler{repos: repos, subs: newSubscriptions(log), cfg: cfg, log: log}
	r.InitializeWorld(1, 100)
	seedAccount(t, db, 100)
	seedAccount(t, db, 200)
	if err := r.AddFriend(t.Context(), 100, 200); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	r.Register(1, 100, 0, 0)

	stream := newTestStream(t)
	errc := make(chan error, 1)
	go func() {
		errc <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 1, Profile: "main", Username37: 100}, stream)
	}()
	t.Cleanup(func() {
		stream.cancel()
		<-errc
	})
	stream.recvWithin(t, 2*time.Second)
	stream.recvWithin(t, 2*time.Second)

	// 200 logs in with privateChat OFF.
	if _, err := h.PlayerLogin(t.Context(), &friendspb.PlayerLoginRequest{
		WorldId:     1,
		Profile:     "main",
		Username37:  200,
		PrivateChat: 2, // OFF
		StaffLvl:    0,
	}); err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}

	u := stream.recvWithin(t, 2*time.Second)
	fl := u.Update.(*friendspb.FriendsUpdate_Friendlist)
	if fl.Friendlist.Entries[0].WorldId != 0 {
		t.Fatalf("WorldId = %d, want 0 (privateChat OFF should hide)", fl.Friendlist.Entries[0].WorldId)
	}
}

func TestPlayerLogout_BroadcastsZeroWorld(t *testing.T) {
	db := createTestDB(t)
	repos := newRepositories(db)
	r := repos.get("main")
	log := noopLogger()
	cfg := Config{WorldPlayerLimit: 100}
	h := &handler{repos: repos, subs: newSubscriptions(log), cfg: cfg, log: log}
	r.InitializeWorld(1, 100)
	seedAccount(t, db, 100)
	seedAccount(t, db, 200)
	if err := r.AddFriend(t.Context(), 100, 200); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	r.Register(1, 100, 0, 0)
	r.Register(1, 200, 0, 0)

	stream := newTestStream(t)
	errc := make(chan error, 1)
	go func() {
		errc <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 1, Profile: "main", Username37: 100}, stream)
	}()
	t.Cleanup(func() {
		stream.cancel()
		<-errc
	})
	stream.recvWithin(t, 2*time.Second)
	stream.recvWithin(t, 2*time.Second)

	if _, err := h.PlayerLogout(t.Context(), &friendspb.PlayerLogoutRequest{
		WorldId:    1,
		Profile:    "main",
		Username37: 200,
	}); err != nil {
		t.Fatalf("PlayerLogout: %v", err)
	}

	u := stream.recvWithin(t, 2*time.Second)
	fl := u.Update.(*friendspb.FriendsUpdate_Friendlist)
	if fl.Friendlist.Entries[0].WorldId != 0 || fl.Friendlist.Entries[0].Username37 != 200 {
		t.Fatalf("entry = (%d, %d), want (0, 200)", fl.Friendlist.Entries[0].WorldId, fl.Friendlist.Entries[0].Username37)
	}
}

func TestFriendlistAdd_AdderGetsTargetWorldAndFollowersBroadcast(t *testing.T) {
	db := createTestDB(t)
	repos := newRepositories(db)
	r := repos.get("main")
	log := noopLogger()
	cfg := Config{WorldPlayerLimit: 100}
	h := &handler{repos: repos, subs: newSubscriptions(log), cfg: cfg, log: log}
	r.InitializeWorld(1, 100)
	seedAccount(t, db, 50)
	seedAccount(t, db, 100)
	seedAccount(t, db, 200)
	// Pre-existing: adder 100 has a follower 50 who already friended 100.
	if err := r.AddFriend(t.Context(), 50, 100); err != nil {
		t.Fatalf("AddFriend (50->100): %v", err)
	}
	r.Register(1, 100, 0, 0)
	r.Register(1, 50, 0, 0)
	r.Register(1, 200, 0, 0)

	// Both subscribers attach.
	adderStream := newTestStream(t)
	followerStream := newTestStream(t)
	errAdder := make(chan error, 1)
	errFollower := make(chan error, 1)
	go func() {
		errAdder <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 1, Profile: "main", Username37: 100}, adderStream)
	}()
	go func() {
		errFollower <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 1, Profile: "main", Username37: 50}, followerStream)
	}()
	t.Cleanup(func() {
		adderStream.cancel()
		followerStream.cancel()
		<-errAdder
		<-errFollower
	})
	// Drain initial snapshots from both.
	adderStream.recvWithin(t, 2*time.Second)
	adderStream.recvWithin(t, 2*time.Second)
	followerStream.recvWithin(t, 2*time.Second)
	followerStream.recvWithin(t, 2*time.Second)

	// 100 adds 200 as a friend.
	if _, err := h.FriendlistAdd(t.Context(), &friendspb.FriendlistAddRequest{
		WorldId:          1,
		Profile:          "main",
		Username37:       100,
		TargetUsername37: 200,
	}); err != nil {
		t.Fatalf("FriendlistAdd: %v", err)
	}

	// Adder (100): single-entry update for 200 (sendPlayerWorldUpdate) +
	// broadcast (100 is in its own followers? no — 50 follows 100, 100
	// doesn't follow itself). So adder sees only the sendPlayerWorldUpdate.
	uAdder := adderStream.recvWithin(t, 2*time.Second)
	adderFL := uAdder.Update.(*friendspb.FriendsUpdate_Friendlist)
	if adderFL.Friendlist.Entries[0].Username37 != 200 {
		t.Fatalf("adder entry = %d, want 200", adderFL.Friendlist.Entries[0].Username37)
	}
	// Follower (50): broadcast about 100's world.
	uFollower := followerStream.recvWithin(t, 2*time.Second)
	followerFL := uFollower.Update.(*friendspb.FriendsUpdate_Friendlist)
	if followerFL.Friendlist.Entries[0].Username37 != 100 || followerFL.Friendlist.Entries[0].WorldId != 1 {
		t.Fatalf("follower entry = (%d, %d), want (1, 100)", followerFL.Friendlist.Entries[0].WorldId, followerFL.Friendlist.Entries[0].Username37)
	}
}

// TestPrivateMessage_DeliveredToRecipient pins slice 4b's contract:
// the server's PrivateMessage RPC routes the message into the target's
// open SubscribeUpdates stream as a PrivateMessageDelivery update.
// Mirrors TS FriendServer.sendPrivateMessage (FriendServer.ts:480-497).
func TestPrivateMessage_DeliveredToRecipient(t *testing.T) {
	db := createTestDB(t)
	repos := newRepositories(db)
	r := repos.get("main")
	log := noopLogger()
	cfg := Config{WorldPlayerLimit: 100}
	h := &handler{repos: repos, subs: newSubscriptions(log), cfg: cfg, log: log}
	r.InitializeWorld(1, 100)
	r.Register(1, 200, 0, 0) // recipient online so the subscription can attach
	seedAccount(t, db, 100)
	seedAccount(t, db, 200)

	// Recipient subscribes; drain initial empty snapshots.
	stream := newTestStream(t)
	errc := make(chan error, 1)
	go func() {
		errc <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 1, Profile: "main", Username37: 200}, stream)
	}()
	t.Cleanup(func() {
		stream.cancel()
		<-errc
	})
	stream.recvWithin(t, 2*time.Second) // empty friendlist snapshot
	stream.recvWithin(t, 2*time.Second) // empty ignorelist snapshot

	// Sender (100) PMs recipient (200).
	if _, err := h.PrivateMessage(t.Context(), &friendspb.PrivateMessageRequest{
		WorldId:          1,
		Profile:          "main",
		Username37:       100,
		TargetUsername37: 200,
		StaffLvl:         2,
		PmId:             0xCAFEBABE,
		Chat:             "hello",
		Coord:            12345,
	}); err != nil {
		t.Fatalf("PrivateMessage: %v", err)
	}

	// Recipient's stream should see a PrivateMessageDelivery.
	u := stream.recvWithin(t, 2*time.Second)
	pm, ok := u.Update.(*friendspb.FriendsUpdate_PrivateMessage)
	if !ok {
		t.Fatalf("update = %T, want FriendsUpdate_PrivateMessage", u.Update)
	}
	got := pm.PrivateMessage
	if got.FromUsername37 != 100 {
		t.Errorf("FromUsername37 = %d, want 100", got.FromUsername37)
	}
	if got.StaffLvl != 2 {
		t.Errorf("StaffLvl = %d, want 2", got.StaffLvl)
	}
	if got.PmId != 0xCAFEBABE {
		t.Errorf("PmId = %#x, want 0xCAFEBABE", got.PmId)
	}
	if got.Chat != "hello" {
		t.Errorf("Chat = %q, want %q", got.Chat, "hello")
	}
}

// TestPrivateMessage_CrossWorld pins that registry routing is keyed
// solely by (profile, username37), so a PM from a sender on world 1
// reaches a recipient subscribed on world 20.
func TestPrivateMessage_CrossWorld(t *testing.T) {
	db := createTestDB(t)
	repos := newRepositories(db)
	r := repos.get("main")
	log := noopLogger()
	cfg := Config{WorldPlayerLimit: 100}
	h := &handler{repos: repos, subs: newSubscriptions(log), cfg: cfg, log: log}
	r.InitializeWorld(1, 100)
	r.InitializeWorld(20, 100)
	r.Register(1, 100, 0, 0)  // sender on world 1
	r.Register(20, 200, 0, 0) // recipient on world 20
	seedAccount(t, db, 100)
	seedAccount(t, db, 200)

	stream := newTestStream(t)
	errc := make(chan error, 1)
	go func() {
		errc <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 20, Profile: "main", Username37: 200}, stream)
	}()
	t.Cleanup(func() {
		stream.cancel()
		<-errc
	})
	stream.recvWithin(t, 2*time.Second) // empty friendlist snapshot
	stream.recvWithin(t, 2*time.Second) // empty ignorelist snapshot

	if _, err := h.PrivateMessage(t.Context(), &friendspb.PrivateMessageRequest{
		WorldId:          1, // sender's world
		Profile:          "main",
		Username37:       100,
		TargetUsername37: 200,
		StaffLvl:         0,
		PmId:             0xDEADBEEF,
		Chat:             "cross-world hi",
		Coord:            0,
	}); err != nil {
		t.Fatalf("PrivateMessage: %v", err)
	}

	u := stream.recvWithin(t, 2*time.Second)
	pm, ok := u.Update.(*friendspb.FriendsUpdate_PrivateMessage)
	if !ok {
		t.Fatalf("update = %T, want FriendsUpdate_PrivateMessage", u.Update)
	}
	if pm.PrivateMessage.PmId != 0xDEADBEEF {
		t.Errorf("PmId = %#x, want 0xDEADBEEF", pm.PrivateMessage.PmId)
	}
	if pm.PrivateMessage.Chat != "cross-world hi" {
		t.Errorf("Chat = %q, want %q", pm.PrivateMessage.Chat, "cross-world hi")
	}
}

// TestHandler_PrivateMessage_PersistsBeforeSending pins slice 6's
// insert-then-send ordering: the handler writes to private_chat
// before pushing PrivateMessageDelivery to the recipient's stream.
// Mirrors TS FriendServer.ts:273-285.
func TestHandler_PrivateMessage_PersistsBeforeSending(t *testing.T) {
	db := createTestDB(t)
	repos := newRepositories(db)
	r := repos.get("main")
	log := noopLogger()
	cfg := Config{WorldPlayerLimit: 100}
	h := &handler{repos: repos, subs: newSubscriptions(log), cfg: cfg, log: log}
	r.InitializeWorld(1, 100)
	r.Register(1, 200, 0, 0) // recipient online
	fromID := seedAccount(t, db, 100)
	toID := seedAccount(t, db, 200)

	stream := newTestStream(t)
	errc := make(chan error, 1)
	go func() {
		errc <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 1, Profile: "main", Username37: 200}, stream)
	}()
	t.Cleanup(func() {
		stream.cancel()
		<-errc
	})
	stream.recvWithin(t, 2*time.Second) // empty friendlist snapshot
	stream.recvWithin(t, 2*time.Second) // empty ignorelist snapshot

	if _, err := h.PrivateMessage(t.Context(), &friendspb.PrivateMessageRequest{
		WorldId:          1,
		Profile:          "main",
		Username37:       100,
		TargetUsername37: 200,
		StaffLvl:         0,
		PmId:             0xCAFEBABE,
		Chat:             "hi",
		Coord:            12345,
	}); err != nil {
		t.Fatalf("PrivateMessage: %v", err)
	}

	// Persistence — TS 9aadcec4 re-key: private_chat is keyed by resolved
	// account ids (account_id, to_account_id), not raw username37s.
	var from, to int64
	var coord int32
	var msg string
	if err := db.QueryRowContext(t.Context(),
		`SELECT account_id, to_account_id, coord, message FROM private_chat`).
		Scan(&from, &to, &coord, &msg); err != nil {
		t.Fatalf("SELECT private_chat: %v", err)
	}
	if from != fromID || to != toID || coord != 12345 || msg != "hi" {
		t.Errorf("row = (%d, %d, %d, %q), want (%d, %d, 12345, %q)",
			from, to, coord, msg, fromID, toID, "hi")
	}

	// Delivery
	u := stream.recvWithin(t, 2*time.Second)
	pm, ok := u.Update.(*friendspb.FriendsUpdate_PrivateMessage)
	if !ok {
		t.Fatalf("update = %T, want FriendsUpdate_PrivateMessage", u.Update)
	}
	if pm.PrivateMessage.PmId != 0xCAFEBABE {
		t.Errorf("PmId = %#x, want 0xCAFEBABE", pm.PrivateMessage.PmId)
	}
}

// TestHandler_PrivateMessage_InsertErrorBlocksSend pins that a SQL
// failure on private_chat insert returns codes.Internal AND does not
// deliver the PM. Forces the failure by closing the *sql.DB after
// the initial-snapshot reads complete. Mirrors the TS thrown-await
// pattern.
func TestHandler_PrivateMessage_InsertErrorBlocksSend(t *testing.T) {
	db := createTestDB(t)
	repos := newRepositories(db)
	r := repos.get("main")
	log := noopLogger()
	cfg := Config{WorldPlayerLimit: 100}
	h := &handler{repos: repos, subs: newSubscriptions(log), cfg: cfg, log: log}
	r.InitializeWorld(1, 100)
	r.Register(1, 200, 0, 0)
	// Seed both endpoints so LogPrivateMessage's account resolution
	// would otherwise succeed. Note that db.Close() below still fails
	// the very first query LogPrivateMessage issues — resolving
	// `from`'s account id — not the later INSERT step; the point of
	// this test is that a non-errAccountMissing failure anywhere in
	// LogPrivateMessage still maps to codes.Internal, regardless of
	// which query trips it.
	seedAccount(t, db, 100)
	seedAccount(t, db, 200)

	stream := newTestStream(t)
	errc := make(chan error, 1)
	go func() {
		errc <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 1, Profile: "main", Username37: 200}, stream)
	}()
	t.Cleanup(func() {
		stream.cancel()
		<-errc
	})
	// Drain initial snapshots BEFORE closing db — those reads need the
	// DB to be open.
	stream.recvWithin(t, 2*time.Second)
	stream.recvWithin(t, 2*time.Second)

	// Force a LogPrivateMessage failure: close the underlying *gamedb.DB.
	// The subscriber goroutine is now in select{} waiting for new
	// updates; it doesn't query the DB until something arrives on its
	// channel.
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close: %v", err)
	}

	_, err := h.PrivateMessage(t.Context(), &friendspb.PrivateMessageRequest{
		WorldId:          1,
		Profile:          "main",
		Username37:       100,
		TargetUsername37: 200,
		PmId:             0xDEADBEEF,
		Chat:             "should not arrive",
	})
	if err == nil {
		t.Fatalf("PrivateMessage on closed DB: got nil error, want Internal")
	}
	if status.Code(err) != codes.Internal {
		t.Fatalf("PrivateMessage err code = %v, want %v", status.Code(err), codes.Internal)
	}

	// No delivery should land on the recipient stream. Short non-fatal
	// poll — recvWithin would t.Fatal on timeout.
	select {
	case u := <-stream.out:
		t.Fatalf("unexpected delivery after insert error: %T", u.Update)
	case <-time.After(200 * time.Millisecond):
		// expected: nothing arrives
	}
}

// TestPrivateMessage_MissingTarget_DroppedSilently pins the TS 9aadcec4
// re-key's restored account-existence check (TS FriendServer.ts:275-276
// @9aadcec4: executeTakeFirstOrThrow on either endpoint throws, the
// outer catch swallows it — no insert, no delivery, socket stays
// healthy). goscape: the RPC succeeds, no private_chat row is written,
// and nothing is delivered to the target's stream, even though the
// target is subscribed and would otherwise receive it.
func TestPrivateMessage_MissingTarget_DroppedSilently(t *testing.T) {
	db := createTestDB(t)
	repos := newRepositories(db)
	r := repos.get("main")
	log := noopLogger()
	cfg := Config{WorldPlayerLimit: 100}
	h := &handler{repos: repos, subs: newSubscriptions(log), cfg: cfg, log: log}
	r.InitializeWorld(1, 100)
	seedAccount(t, db, 1) // sender exists; target 2 does not

	// Target's would-be stream: subscribed so a wrongful delivery would
	// be observable.
	stream := newTestStream(t)
	errc := make(chan error, 1)
	go func() {
		errc <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 1, Profile: "main", Username37: 2}, stream)
	}()
	t.Cleanup(func() {
		stream.cancel()
		<-errc
	})
	stream.recvWithin(t, 2*time.Second) // empty friendlist snapshot
	stream.recvWithin(t, 2*time.Second) // empty ignorelist snapshot

	if _, err := h.PrivateMessage(t.Context(), &friendspb.PrivateMessageRequest{
		WorldId: 1, Profile: "main", Username37: 1, TargetUsername37: 2, Coord: 0, Chat: "psst",
	}); err != nil {
		t.Fatalf("PrivateMessage: got %v, want nil (silent drop)", err)
	}

	var n int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM private_chat`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("private_chat rows: got %d, want 0", n)
	}

	// No delivery should land on the target's stream. Short non-fatal
	// poll — recvWithin would t.Fatal on timeout.
	select {
	case u := <-stream.out:
		t.Fatalf("unexpected delivery to missing target: %T", u.Update)
	case <-time.After(200 * time.Millisecond):
		// expected: nothing arrives
	}
}

// TestPrivateMessage_BothExist_PersistedAndDelivered dual-pins the
// presence side of TestPrivateMessage_MissingTarget_DroppedSilently:
// when both endpoints resolve, the PM is still persisted to
// private_chat and delivered to the target's stream.
func TestPrivateMessage_BothExist_PersistedAndDelivered(t *testing.T) {
	db := createTestDB(t)
	repos := newRepositories(db)
	r := repos.get("main")
	log := noopLogger()
	cfg := Config{WorldPlayerLimit: 100}
	h := &handler{repos: repos, subs: newSubscriptions(log), cfg: cfg, log: log}
	r.InitializeWorld(1, 100)
	seedAccount(t, db, 1)
	seedAccount(t, db, 2)

	stream := newTestStream(t)
	errc := make(chan error, 1)
	go func() {
		errc <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 1, Profile: "main", Username37: 2}, stream)
	}()
	t.Cleanup(func() {
		stream.cancel()
		<-errc
	})
	stream.recvWithin(t, 2*time.Second) // empty friendlist snapshot
	stream.recvWithin(t, 2*time.Second) // empty ignorelist snapshot

	if _, err := h.PrivateMessage(t.Context(), &friendspb.PrivateMessageRequest{
		WorldId: 1, Profile: "main", Username37: 1, TargetUsername37: 2, Coord: 7, Chat: "hello",
	}); err != nil {
		t.Fatalf("PrivateMessage: %v", err)
	}

	var n int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM private_chat`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("private_chat rows: got %d, want 1", n)
	}

	u := stream.recvWithin(t, 2*time.Second)
	pm, ok := u.Update.(*friendspb.FriendsUpdate_PrivateMessage)
	if !ok {
		t.Fatalf("update = %T, want FriendsUpdate_PrivateMessage", u.Update)
	}
	if pm.PrivateMessage.Chat != "hello" {
		t.Errorf("Chat = %q, want %q", pm.PrivateMessage.Chat, "hello")
	}
}

// --- slice 5a: RELAY_* handler routing tests ---

func TestHandler_RelayKick_RoutesToSubscriber(t *testing.T) {
	h, _, worldSubs := newTestHandlerWithWorldSubs(t)
	sub := newWorldSubscriber("main", 2)
	worldSubs.register(sub)
	defer worldSubs.deregister(sub)

	_, err := h.RelayKick(context.Background(), &friendspb.RelayKickRequest{
		Profile:       "main",
		TargetWorldId: 2,
		Username37:    nameToBase37("alice"),
	})
	if err != nil {
		t.Fatalf("RelayKick: %v", err)
	}
	select {
	case ev := <-sub.ch:
		kick := ev.GetKick()
		if kick == nil {
			t.Fatalf("got event variant %T, want Kick", ev.Event)
		}
		if kick.Username37 != nameToBase37("alice") {
			t.Fatalf("kick.Username37 = %d, want %d", kick.Username37, nameToBase37("alice"))
		}
	default:
		t.Fatal("expected KickEvent on subscriber channel")
	}
}

func TestHandler_RelayKick_NoSubscriberSilent(t *testing.T) {
	h, _, _ := newTestHandlerWithWorldSubs(t)
	// No subscriber registered for world 99.
	_, err := h.RelayKick(context.Background(), &friendspb.RelayKickRequest{
		Profile:       "main",
		TargetWorldId: 99,
		Username37:    nameToBase37("alice"),
	})
	if err != nil {
		t.Fatalf("RelayKick silent-drop expected OK; got %v", err)
	}
}

func TestHandler_RelayMute_RoutesPayload(t *testing.T) {
	h, _, worldSubs := newTestHandlerWithWorldSubs(t)
	sub := newWorldSubscriber("main", 2)
	worldSubs.register(sub)
	defer worldSubs.deregister(sub)
	_, err := h.RelayMute(context.Background(), &friendspb.RelayMuteRequest{
		Profile: "main", TargetWorldId: 2, Username37: nameToBase37("bob"), MutedUntilMs: 12345,
	})
	if err != nil {
		t.Fatalf("RelayMute: %v", err)
	}
	ev := mustRecvWorldEvent(t, sub)
	mute := ev.GetMute()
	if mute == nil || mute.Username37 != nameToBase37("bob") || mute.MutedUntilMs != 12345 {
		t.Fatalf("mute payload mismatch: %+v", mute)
	}
}

func TestHandler_RelayShutdown_RoutesPayload(t *testing.T) {
	h, _, worldSubs := newTestHandlerWithWorldSubs(t)
	sub := newWorldSubscriber("main", 2)
	worldSubs.register(sub)
	defer worldSubs.deregister(sub)
	_, err := h.RelayShutdown(context.Background(), &friendspb.RelayShutdownRequest{
		Profile: "main", TargetWorldId: 2, DurationTicks: 50,
	})
	if err != nil {
		t.Fatalf("RelayShutdown: %v", err)
	}
	ev := mustRecvWorldEvent(t, sub)
	if sd := ev.GetShutdown(); sd == nil || sd.DurationTicks != 50 {
		t.Fatalf("shutdown payload mismatch: %+v", sd)
	}
}

func TestHandler_RelayBroadcast_RoutesPayload(t *testing.T) {
	h, _, worldSubs := newTestHandlerWithWorldSubs(t)
	sub := newWorldSubscriber("main", 2)
	worldSubs.register(sub)
	defer worldSubs.deregister(sub)
	_, err := h.RelayBroadcast(context.Background(), &friendspb.RelayBroadcastRequest{
		Profile: "main", TargetWorldId: 2, Message: "hello world",
	})
	if err != nil {
		t.Fatalf("RelayBroadcast: %v", err)
	}
	ev := mustRecvWorldEvent(t, sub)
	if bc := ev.GetBroadcast(); bc == nil || bc.Message != "hello world" {
		t.Fatalf("broadcast payload mismatch: %+v", bc)
	}
}

func TestHandler_RelayTrack_RoutesPayload(t *testing.T) {
	h, _, worldSubs := newTestHandlerWithWorldSubs(t)
	sub := newWorldSubscriber("main", 2)
	worldSubs.register(sub)
	defer worldSubs.deregister(sub)
	_, err := h.RelayTrack(context.Background(), &friendspb.RelayTrackRequest{
		Profile: "main", TargetWorldId: 2, Username37: nameToBase37("carol"), State: 1,
	})
	if err != nil {
		t.Fatalf("RelayTrack: %v", err)
	}
	ev := mustRecvWorldEvent(t, sub)
	tk := ev.GetTrack()
	if tk == nil || tk.Username37 != nameToBase37("carol") || tk.State != 1 {
		t.Fatalf("track payload mismatch: %+v", tk)
	}
}

func TestHandler_RelayReload_RoutesEmpty(t *testing.T) {
	h, _, worldSubs := newTestHandlerWithWorldSubs(t)
	sub := newWorldSubscriber("main", 2)
	worldSubs.register(sub)
	defer worldSubs.deregister(sub)
	_, err := h.RelayReload(context.Background(), &friendspb.RelayReloadRequest{Profile: "main", TargetWorldId: 2})
	if err != nil {
		t.Fatalf("RelayReload: %v", err)
	}
	ev := mustRecvWorldEvent(t, sub)
	if ev.GetReload() == nil {
		t.Fatalf("expected Reload variant; got %T", ev.Event)
	}
}

func TestHandler_RelayClearLogins_RoutesEmpty(t *testing.T) {
	h, _, worldSubs := newTestHandlerWithWorldSubs(t)
	sub := newWorldSubscriber("main", 2)
	worldSubs.register(sub)
	defer worldSubs.deregister(sub)
	_, err := h.RelayClearLogins(context.Background(), &friendspb.RelayClearLoginsRequest{Profile: "main", TargetWorldId: 2})
	if err != nil {
		t.Fatalf("RelayClearLogins: %v", err)
	}
	if ev := mustRecvWorldEvent(t, sub); ev.GetClearLogins() == nil {
		t.Fatalf("expected ClearLogins variant; got %T", ev.Event)
	}
}

func TestHandler_RelayClearLogouts_RoutesEmpty(t *testing.T) {
	h, _, worldSubs := newTestHandlerWithWorldSubs(t)
	sub := newWorldSubscriber("main", 2)
	worldSubs.register(sub)
	defer worldSubs.deregister(sub)
	_, err := h.RelayClearLogouts(context.Background(), &friendspb.RelayClearLogoutsRequest{Profile: "main", TargetWorldId: 2})
	if err != nil {
		t.Fatalf("RelayClearLogouts: %v", err)
	}
	if ev := mustRecvWorldEvent(t, sub); ev.GetClearLogouts() == nil {
		t.Fatalf("expected ClearLogouts variant; got %T", ev.Event)
	}
}

func TestHandler_RelayQueueScript_RoutesPayload(t *testing.T) {
	h, _, worldSubs := newTestHandlerWithWorldSubs(t)
	sub := newWorldSubscriber("main", 2)
	worldSubs.register(sub)
	defer worldSubs.deregister(sub)
	_, err := h.RelayQueueScript(context.Background(), &friendspb.RelayQueueScriptRequest{
		Profile: "main", TargetWorldId: 2, ScriptName: "debug:dump", Username37: nameToBase37("dan"),
	})
	if err != nil {
		t.Fatalf("RelayQueueScript: %v", err)
	}
	ev := mustRecvWorldEvent(t, sub)
	qs := ev.GetQueueScript()
	if qs == nil || qs.ScriptName != "debug:dump" || qs.Username37 != nameToBase37("dan") {
		t.Fatalf("queue_script payload mismatch: %+v", qs)
	}
}

func TestHandler_SubscribeWorldEvents_DupKicksPrior(t *testing.T) {
	h, _, _ := newTestHandlerWithWorldSubs(t)

	// Open the first subscription in a goroutine; capture its EOF via
	// the stream's err return.
	srv1 := newFakeWorldEventsServerStream(t)
	done1 := make(chan error, 1)
	go func() {
		done1 <- h.SubscribeWorldEvents(&friendspb.SubscribeWorldEventsRequest{WorldId: 7, Profile: "main"}, srv1)
	}()

	// Spin until srv1 has installed its subscriber (registry has entry).
	waitFor(t, func() bool {
		h.worldSubs.mu.Lock()
		defer h.worldSubs.mu.Unlock()
		return h.worldSubs.by[wsubKey{profile: "main", worldId: 7}] != nil
	})

	// Open a second subscription for the same (profile, worldId).
	srv2 := newFakeWorldEventsServerStream(t)
	done2 := make(chan error, 1)
	go func() {
		done2 <- h.SubscribeWorldEvents(&friendspb.SubscribeWorldEventsRequest{WorldId: 7, Profile: "main"}, srv2)
	}()

	// srv1's stream goroutine sees done closed and returns nil.
	select {
	case err := <-done1:
		if err != nil {
			t.Fatalf("prior stream returned %v; want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for prior stream to exit")
	}

	// Close srv2's ctx to terminate the second stream.
	srv2.cancel()
	<-done2
}

// newTestHandlerWithWorldSubs constructs a handler with both per-player
// and per-world subscription registries wired up. Returns (handler, subs,
// worldSubs). Re-uses the test-DB helper pattern used by the existing
// handler tests (createTestDB + noopLogger).
func newTestHandlerWithWorldSubs(t *testing.T) (*handler, *subscriptions, *worldSubscriptions) {
	t.Helper()
	log := noopLogger()
	repos := newRepositories(createTestDB(t))
	subs := newSubscriptions(log)
	worldSubs := newWorldSubscriptions(log)
	h := &handler{
		repos:     repos,
		subs:      subs,
		worldSubs: worldSubs,
		cfg:       Config{WorldPlayerLimit: 2000},
		log:       log,
	}
	return h, subs, worldSubs
}

// mustRecvWorldEvent reads one event from sub.ch with a short timeout
// (helpers in this file use 1s for similar drains). Fails the test if
// no event arrives.
func mustRecvWorldEvent(t *testing.T, sub *worldSubscriber) *friendspb.WorldEvent {
	t.Helper()
	select {
	case ev := <-sub.ch:
		return ev
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for world event")
		return nil
	}
}

// fakeWorldEventsServerStream is a test impl of
// friendspb.FriendsService_SubscribeWorldEventsServer.
type fakeWorldEventsServerStream struct {
	grpc.ServerStream
	ctx    context.Context
	cancel context.CancelFunc
	sent   chan *friendspb.WorldEvent
}

func newFakeWorldEventsServerStream(t *testing.T) *fakeWorldEventsServerStream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	return &fakeWorldEventsServerStream{
		ctx:    ctx,
		cancel: cancel,
		sent:   make(chan *friendspb.WorldEvent, 16),
	}
}

func (s *fakeWorldEventsServerStream) Send(ev *friendspb.WorldEvent) error {
	s.sent <- ev
	return nil
}
func (s *fakeWorldEventsServerStream) Context() context.Context { return s.ctx }

// waitFor polls cond at 10ms intervals up to 2s.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("waitFor: condition not met within 2s")
}

// TestMultiProfile_WorldIsolation pins the 244 multi-profile server
// (FriendServer.ts:61-75 repositories[profile] +
// socketByWorld[profile][world]): the same world id under two profiles
// is two independent registries — registration, presence, and the
// player cap are all profile-scoped, and the WorldConnect
// profile-mismatch reject is GONE (TS deleted it at 244).
func TestMultiProfile_WorldIsolation(t *testing.T) {
	h := newTestHandler(t)
	for _, profile := range []string{"main", "beta"} {
		if _, err := h.WorldConnect(t.Context(), &friendspb.WorldConnectRequest{
			WorldId: 1, Profile: profile,
		}); err != nil {
			t.Fatalf("WorldConnect %s: %v", profile, err)
		}
	}
	for _, profile := range []string{"main", "beta"} {
		resp, err := h.PlayerLogin(t.Context(), &friendspb.PlayerLoginRequest{
			WorldId: 1, Profile: profile, Username37: 0xB0B,
		})
		if err != nil || !resp.Accepted {
			t.Fatalf("PlayerLogin %s: %v / %v", profile, resp, err)
		}
	}
	if _, err := h.PlayerLogout(t.Context(), &friendspb.PlayerLogoutRequest{
		WorldId: 1, Profile: "beta", Username37: 0xB0B,
	}); err != nil {
		t.Fatal(err)
	}
	if got := h.repos.get("main").GetWorld(0xB0B); got != 1 {
		t.Errorf("main presence after beta logout: world %d, want 1", got)
	}
	if got := h.repos.get("beta").GetWorld(0xB0B); got != 0 {
		t.Errorf("beta presence after beta logout: world %d, want 0", got)
	}
}

// --- public_chat audit: TS 9aadcec4 account_id+profile+world shape ---

// TestHandler_PublicMessage_PersistsRow pins the happy path: a valid
// PublicMessageRequest whose username resolves to an account returns
// (Empty, nil) AND the row is visible in public_chat under r.profile,
// keyed by the resolved account_id (not the raw username string). No
// delivery, no subscription. Mirrors TS FriendServer.ts:291-306
// @9aadcec4.
func TestHandler_PublicMessage_PersistsRow(t *testing.T) {
	db := createTestDB(t)
	repos := newRepositories(db)
	log := noopLogger()
	cfg := Config{WorldPlayerLimit: 100}
	h := &handler{repos: repos, subs: newSubscriptions(log), cfg: cfg, log: log}
	acctID := seedAccount(t, db, nameToBase37("alice"))

	resp, err := h.PublicMessage(t.Context(), &friendspb.PublicMessageRequest{
		WorldId:  10,
		Profile:  "main",
		Username: "alice",
		Coord:    9876,
		Chat:     "audit me",
	})
	if err != nil {
		t.Fatalf("PublicMessage: %v", err)
	}
	if resp == nil {
		t.Fatalf("PublicMessage: nil response, want non-nil Empty")
	}

	var gotAccountID int64
	var gotProfile string
	var gotWorld, gotCoord int32
	var msg string
	if err := db.QueryRowContext(t.Context(),
		`SELECT account_id, profile, world, coord, message FROM public_chat`).
		Scan(&gotAccountID, &gotProfile, &gotWorld, &gotCoord, &msg); err != nil {
		t.Fatalf("SELECT public_chat: %v", err)
	}
	if gotAccountID != acctID || gotProfile != "main" || gotWorld != 10 || gotCoord != 9876 || msg != "audit me" {
		t.Errorf("row = (%d, %q, %d, %d, %q), want (%d, main, 10, 9876, audit me)",
			gotAccountID, gotProfile, gotWorld, gotCoord, msg, acctID)
	}
}

// TestHandler_PublicMessage_MissingAccount_DroppedSilently pins the
// audit's second headline delta: TS resolves `username` against
// `account` via executeTakeFirstOrThrow (FriendServer.ts:294
// @9aadcec4), which throws on a missing account; the outer
// per-connection try/catch (:88/419) swallows it — no persisted row,
// no client-visible error. goscape maps Repository.LogPublicMessage's
// errAccountMissing to the same silent-success posture.
func TestHandler_PublicMessage_MissingAccount_DroppedSilently(t *testing.T) {
	db := createTestDB(t)
	repos := newRepositories(db)
	log := noopLogger()
	cfg := Config{WorldPlayerLimit: 100}
	h := &handler{repos: repos, subs: newSubscriptions(log), cfg: cfg, log: log}

	resp, err := h.PublicMessage(t.Context(), &friendspb.PublicMessageRequest{
		WorldId:  10,
		Profile:  "main",
		Username: "nobody",
		Coord:    0,
		Chat:     "should not persist",
	})
	if err != nil {
		t.Fatalf("PublicMessage(missing account): got %v, want nil (silent drop)", err)
	}
	if resp == nil {
		t.Fatalf("PublicMessage(missing account): nil response, want non-nil Empty")
	}
	var n int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM public_chat`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("public_chat rows: got %d, want 0", n)
	}
}

// TestHandler_PublicMessage_InsertErrorReturnsInternal pins that a SQL
// failure (of any kind, not just errAccountMissing) returns
// codes.Internal. Forces the failure by closing the *gamedb.DB before
// the call — account resolution itself fails first (a generic DB
// error, not sql.ErrNoRows), so this never takes the errAccountMissing
// silent-drop path. Mirrors the slice 6
// TestHandler_PrivateMessage_InsertErrorBlocksSend pattern (minus the
// delivery half, which doesn't exist for public_chat).
func TestHandler_PublicMessage_InsertErrorReturnsInternal(t *testing.T) {
	db := createTestDB(t)
	repos := newRepositories(db)
	log := noopLogger()
	cfg := Config{WorldPlayerLimit: 100}
	h := &handler{repos: repos, subs: newSubscriptions(log), cfg: cfg, log: log}

	if err := db.Close(); err != nil {
		t.Fatalf("db.Close: %v", err)
	}

	resp, err := h.PublicMessage(t.Context(), &friendspb.PublicMessageRequest{
		WorldId:  10,
		Profile:  "main",
		Username: "uuid-pub-err",
		Coord:    0,
		Chat:     "should not persist",
	})
	if err == nil {
		t.Fatalf("PublicMessage on closed DB: got nil error, want Internal")
	}
	if resp != nil {
		t.Errorf("PublicMessage err path: resp = %+v, want nil", resp)
	}
	if status.Code(err) != codes.Internal {
		t.Fatalf("PublicMessage err code = %v, want %v", status.Code(err), codes.Internal)
	}
}
