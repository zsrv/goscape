package friends

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/zsrv/goscape/pkg/friendspb"
)

// newTestHandler returns a handler wired to a fresh in-memory repo,
// configured with NodeProfile="main" and WorldPlayerLimit=10.
func newTestHandler(t *testing.T) *handler {
	t.Helper()
	log := noopLogger()
	return &handler{
		repo: NewRepository(createTestDB(t), "test"),
		subs: newSubscriptions(log),
		cfg: Config{
			NodeProfile:      "main",
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
	if !h.repo.Register(1, 0xAAAA, 0, 0) {
		t.Errorf("Register after WorldConnect: got false, want true")
	}
}

func TestHandler_WorldConnect_ProfileMismatch(t *testing.T) {
	h := newTestHandler(t)
	_, err := h.WorldConnect(t.Context(), &friendspb.WorldConnectRequest{
		WorldId: 1,
		Profile: "wrong",
	})
	if err == nil {
		t.Fatalf("WorldConnect with bad profile: got nil error")
	}
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Errorf("status code: got %v, want InvalidArgument", got)
	}
}

func TestHandler_PlayerLogin_BeforeWorldConnect_LazyInit(t *testing.T) {
	h := newTestHandler(t)
	resp, err := h.PlayerLogin(t.Context(), &friendspb.PlayerLoginRequest{
		WorldId:     1,
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
	if got := h.repo.GetWorld(0xAAAA); got != 1 {
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
		Username37:  0xAAAA,
		PrivateChat: 99, // invalid -> coerce to 0 (ON)
		StaffLvl:    0,
	}); err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if got := h.repo.GetChatMode(0xAAAA); got != 0 {
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
		resp, err := h.PlayerLogin(t.Context(), &friendspb.PlayerLoginRequest{WorldId: 1, Username37: i})
		if err != nil {
			t.Fatalf("PlayerLogin %d: %v", i, err)
		}
		if !resp.Accepted {
			t.Errorf("Accepted #%d: got false, want true", i)
		}
	}
	resp, err := h.PlayerLogin(t.Context(), &friendspb.PlayerLoginRequest{WorldId: 1, Username37: 3})
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
	if _, err := h.PlayerLogin(t.Context(), &friendspb.PlayerLoginRequest{WorldId: 1, Username37: 0xAAAA}); err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if _, err := h.PlayerLogout(t.Context(), &friendspb.PlayerLogoutRequest{WorldId: 1, Username37: 0xAAAA}); err != nil {
		t.Fatalf("PlayerLogout: %v", err)
	}
	if got := h.repo.GetWorld(0xAAAA); got != 0 {
		t.Errorf("GetWorld after logout: got %d, want 0", got)
	}
}

func TestHandler_ChatSetMode_UpdatesState(t *testing.T) {
	h := newTestHandler(t)
	if _, err := h.WorldConnect(t.Context(), &friendspb.WorldConnectRequest{WorldId: 1, Profile: "main"}); err != nil {
		t.Fatalf("WorldConnect: %v", err)
	}
	if _, err := h.PlayerLogin(t.Context(), &friendspb.PlayerLoginRequest{WorldId: 1, Username37: 0xAAAA, PrivateChat: 0}); err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if _, err := h.ChatSetMode(t.Context(), &friendspb.ChatSetModeRequest{
		WorldId:     1,
		Username37:  0xAAAA,
		PrivateChat: 2,
	}); err != nil {
		t.Fatalf("ChatSetMode: %v", err)
	}
	if got := h.repo.GetChatMode(0xAAAA); got != 2 {
		t.Errorf("GetChatMode after ChatSetMode(OFF): got %d, want 2", got)
	}
}

func TestHandler_ChatSetMode_PrivateChatCoercion(t *testing.T) {
	h := newTestHandler(t)
	if _, err := h.WorldConnect(t.Context(), &friendspb.WorldConnectRequest{WorldId: 1, Profile: "main"}); err != nil {
		t.Fatalf("WorldConnect: %v", err)
	}
	if _, err := h.PlayerLogin(t.Context(), &friendspb.PlayerLoginRequest{WorldId: 1, Username37: 0xAAAA, PrivateChat: 0}); err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if _, err := h.ChatSetMode(t.Context(), &friendspb.ChatSetModeRequest{
		WorldId:     1,
		Username37:  0xAAAA,
		PrivateChat: 99,
	}); err != nil {
		t.Fatalf("ChatSetMode: %v", err)
	}
	if got := h.repo.GetChatMode(0xAAAA); got != 0 {
		t.Errorf("GetChatMode after coercion: got %d, want 0", got)
	}
}

func TestHandler_FriendlistAdd_Persists(t *testing.T) {
	h := newTestHandler(t)
	ctx := t.Context()
	if _, err := h.FriendlistAdd(ctx, &friendspb.FriendlistAddRequest{
		WorldId:          1,
		Username37:       0xAAAA,
		TargetUsername37: 0xBBBB,
	}); err != nil {
		t.Fatalf("FriendlistAdd: %v", err)
	}
	got, err := h.repo.GetFriends(ctx, 0xAAAA)
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
	if err := h.repo.AddFriend(ctx, 0xAAAA, 0xBBBB); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	if _, err := h.FriendlistDel(ctx, &friendspb.FriendlistDelRequest{
		WorldId:          1,
		Username37:       0xAAAA,
		TargetUsername37: 0xBBBB,
	}); err != nil {
		t.Fatalf("FriendlistDel: %v", err)
	}
	got, err := h.repo.GetFriends(ctx, 0xAAAA)
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
	if _, err := h.IgnorelistAdd(ctx, &friendspb.IgnorelistAddRequest{
		WorldId:          1,
		Username37:       0xAAAA,
		TargetUsername37: 0xBBBB,
	}); err != nil {
		t.Fatalf("IgnorelistAdd: %v", err)
	}
	got, err := h.repo.GetIgnores(ctx, 0xAAAA)
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
	if err := h.repo.AddIgnore(ctx, 0xAAAA, 0xBBBB); err != nil {
		t.Fatalf("AddIgnore: %v", err)
	}
	if _, err := h.IgnorelistDel(ctx, &friendspb.IgnorelistDelRequest{
		WorldId:          1,
		Username37:       0xAAAA,
		TargetUsername37: 0xBBBB,
	}); err != nil {
		t.Fatalf("IgnorelistDel: %v", err)
	}
	got, err := h.repo.GetIgnores(ctx, 0xAAAA)
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
	// No SubscribeUpdates call for the target — registry is empty for
	// username37=0xBBBB.
	if _, err := h.PrivateMessage(t.Context(), &friendspb.PrivateMessageRequest{
		WorldId:          1,
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
	r, _ := newTestRepo(t)
	log := noopLogger()
	cfg := Config{NodeProfile: "main", WorldPlayerLimit: 100}
	h := &handler{repo: r, subs: newSubscriptions(log), cfg: cfg, log: log}
	r.InitializeWorld(1, 100)
	r.Register(1, 100, 0, 0)
	if err := r.AddFriend(t.Context(), 100, 200); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	if err := r.AddIgnore(t.Context(), 100, 300); err != nil {
		t.Fatalf("AddIgnore: %v", err)
	}

	stream := newTestStream(t)
	errc := make(chan error, 1)
	go func() {
		errc <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 1, Username37: 100}, stream)
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
	r, _ := newTestRepo(t)
	log := noopLogger()
	cfg := Config{NodeProfile: "main", WorldPlayerLimit: 100}
	h := &handler{repo: r, subs: newSubscriptions(log), cfg: cfg, log: log}
	r.InitializeWorld(1, 100)
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
		errc <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 1, Username37: 100}, stream)
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
	r, _ := newTestRepo(t)
	log := noopLogger()
	cfg := Config{NodeProfile: "main", WorldPlayerLimit: 100}
	h := &handler{repo: r, subs: newSubscriptions(log), cfg: cfg, log: log}
	r.InitializeWorld(1, 100)
	if err := r.AddFriend(t.Context(), 100, 200); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	r.Register(1, 100, 0, 0)

	stream := newTestStream(t)
	errc := make(chan error, 1)
	go func() {
		errc <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 1, Username37: 100}, stream)
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
	r, _ := newTestRepo(t)
	log := noopLogger()
	cfg := Config{NodeProfile: "main", WorldPlayerLimit: 100}
	h := &handler{repo: r, subs: newSubscriptions(log), cfg: cfg, log: log}
	r.InitializeWorld(1, 100)
	if err := r.AddFriend(t.Context(), 100, 200); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	r.Register(1, 100, 0, 0)
	r.Register(1, 200, 0, 0)

	stream := newTestStream(t)
	errc := make(chan error, 1)
	go func() {
		errc <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 1, Username37: 100}, stream)
	}()
	t.Cleanup(func() {
		stream.cancel()
		<-errc
	})
	stream.recvWithin(t, 2*time.Second)
	stream.recvWithin(t, 2*time.Second)

	if _, err := h.PlayerLogout(t.Context(), &friendspb.PlayerLogoutRequest{
		WorldId:    1,
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
	r, _ := newTestRepo(t)
	log := noopLogger()
	cfg := Config{NodeProfile: "main", WorldPlayerLimit: 100}
	h := &handler{repo: r, subs: newSubscriptions(log), cfg: cfg, log: log}
	r.InitializeWorld(1, 100)
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
		errAdder <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 1, Username37: 100}, adderStream)
	}()
	go func() {
		errFollower <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 1, Username37: 50}, followerStream)
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
	r, _ := newTestRepo(t)
	log := noopLogger()
	cfg := Config{NodeProfile: "main", WorldPlayerLimit: 100}
	h := &handler{repo: r, subs: newSubscriptions(log), cfg: cfg, log: log}
	r.InitializeWorld(1, 100)
	r.Register(1, 200, 0, 0) // recipient online so the subscription can attach

	// Recipient subscribes; drain initial empty snapshots.
	stream := newTestStream(t)
	errc := make(chan error, 1)
	go func() {
		errc <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 1, Username37: 200}, stream)
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
// solely by username37, so a PM from a sender on world 1 reaches a
// recipient subscribed on world 20.
func TestPrivateMessage_CrossWorld(t *testing.T) {
	r, _ := newTestRepo(t)
	log := noopLogger()
	cfg := Config{NodeProfile: "main", WorldPlayerLimit: 100}
	h := &handler{repo: r, subs: newSubscriptions(log), cfg: cfg, log: log}
	r.InitializeWorld(1, 100)
	r.InitializeWorld(20, 100)
	r.Register(1, 100, 0, 0)  // sender on world 1
	r.Register(20, 200, 0, 0) // recipient on world 20

	stream := newTestStream(t)
	errc := make(chan error, 1)
	go func() {
		errc <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 20, Username37: 200}, stream)
	}()
	t.Cleanup(func() {
		stream.cancel()
		<-errc
	})
	stream.recvWithin(t, 2*time.Second) // empty friendlist snapshot
	stream.recvWithin(t, 2*time.Second) // empty ignorelist snapshot

	if _, err := h.PrivateMessage(t.Context(), &friendspb.PrivateMessageRequest{
		WorldId:          1, // sender's world
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

