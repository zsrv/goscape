package friends

import (
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

