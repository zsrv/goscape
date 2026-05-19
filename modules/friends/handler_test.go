package friends

import (
	"context"
	"testing"

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

func TestHandler_PrivateMessage_NoOp_Slice1(t *testing.T) {
	h := newTestHandler(t)
	// Acceptance is the assertion: returns OK without erroring. Delivery
	// is slice 4, persistence is slice 6.
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

// subscribeUpdatesRecorder captures Send calls from SubscribeUpdates and
// provides a cancellable Context so the drain loop can be stopped cleanly
// in tests. The optional notifyCh is closed after each Send so tests can
// synchronise on delivery.
type subscribeUpdatesRecorder struct {
	friendspb.FriendsService_SubscribeUpdatesServer
	ctx      context.Context
	cancel   context.CancelFunc
	sent     []*friendspb.FriendsUpdate
	notifyCh chan struct{}
}

func newSubscribeUpdatesRecorder(t *testing.T) *subscribeUpdatesRecorder {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	return &subscribeUpdatesRecorder{
		ctx:      ctx,
		cancel:   cancel,
		notifyCh: make(chan struct{}, 64),
	}
}

func (r *subscribeUpdatesRecorder) Context() context.Context { return r.ctx }
func (r *subscribeUpdatesRecorder) Send(u *friendspb.FriendsUpdate) error {
	r.sent = append(r.sent, u)
	select {
	case r.notifyCh <- struct{}{}:
	default:
	}
	return nil
}

// TestHandler_SubscribeUpdates_InitialSnapshots verifies that
// SubscribeUpdates sends UPDATE_FRIENDLIST + UPDATE_IGNORELIST snapshots
// on attach, then exits cleanly when the stream context is cancelled.
func TestHandler_SubscribeUpdates_InitialSnapshots(t *testing.T) {
	h := newTestHandler(t)
	ctx := t.Context()

	// Seed data: viewer 0xAAAA has friend 0xBBBB and ignores 0xCCCC.
	if err := h.repo.AddFriend(ctx, 0xAAAA, 0xBBBB); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	if err := h.repo.AddIgnore(ctx, 0xAAAA, 0xCCCC); err != nil {
		t.Fatalf("AddIgnore: %v", err)
	}

	rec := newSubscribeUpdatesRecorder(t)

	// Run SubscribeUpdates in a goroutine so we can cancel after
	// the initial snapshots have been delivered.
	done := make(chan error, 1)
	go func() {
		done <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{
			WorldId:    1,
			Username37: 0xAAAA,
		}, rec)
	}()

	// Wait for both snapshots to arrive.
	for i := 0; i < 2; i++ {
		select {
		case <-rec.notifyCh:
		case <-ctx.Done():
			t.Fatalf("timed out waiting for snapshot %d", i+1)
		}
	}
	rec.cancel()

	if err := <-done; err != nil {
		t.Fatalf("SubscribeUpdates: %v", err)
	}

	// Expect exactly 2 messages: friendlist then ignorelist.
	if len(rec.sent) != 2 {
		t.Fatalf("sent count: got %d, want 2; msgs=%v", len(rec.sent), rec.sent)
	}
	fl := rec.sent[0].GetFriendlist()
	if fl == nil {
		t.Fatalf("first message is not a FriendlistUpdate")
	}
	if len(fl.Entries) != 1 || fl.Entries[0].Username37 != 0xBBBB {
		t.Errorf("friendlist entries: got %v, want [{username37:0xBBBB}]", fl.Entries)
	}
	il := rec.sent[1].GetIgnorelist()
	if il == nil {
		t.Fatalf("second message is not an IgnorelistUpdate")
	}
	if len(il.Username37) != 1 || il.Username37[0] != 0xCCCC {
		t.Errorf("ignorelist: got %v, want [0xCCCC]", il.Username37)
	}
}

// TestHandler_SubscribeUpdates_SubscriberRegistered verifies that the
// subscriber is live in the registry during the stream and absent after.
func TestHandler_SubscribeUpdates_SubscriberRegistered(t *testing.T) {
	h := newTestHandler(t)
	ctx := t.Context()

	rec := newSubscribeUpdatesRecorder(t)

	// Run SubscribeUpdates in a goroutine; cancel after confirming
	// the sub is present.
	done := make(chan error, 1)
	go func() {
		done <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{
			WorldId:    1,
			Username37: 0xAAAA,
		}, rec)
	}()

	// Wait for initial snapshots (2 sends) to be sure the subscriber
	// is registered before we inspect the registry.
	for i := 0; i < 2; i++ {
		select {
		case <-rec.notifyCh:
		case <-ctx.Done():
			t.Fatalf("timed out waiting for snapshot %d", i+1)
		}
	}

	// Subscriber must be present in the registry during the stream.
	if h.subs.get(0xAAAA) == nil {
		t.Errorf("subscriber not in registry during active stream")
	}

	rec.cancel()
	if err := <-done; err != nil {
		t.Fatalf("SubscribeUpdates: %v", err)
	}

	// After the stream exits, deregister must have run.
	if h.subs.get(0xAAAA) != nil {
		t.Errorf("subscriber still present after stream exit")
	}
}
