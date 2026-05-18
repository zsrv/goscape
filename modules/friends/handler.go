package friends

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/zsrv/goscape/pkg/friendspb"
)

// handler implements friendspb.FriendsServiceServer.
type handler struct {
	friendspb.UnimplementedFriendsServiceServer

	repo *Repository
	cfg  Config
	log  *slog.Logger
}

// ensureWorld lazy-inits the world's player slot if not already known.
// TS-faithful behavior: FriendServer.ts:108-115 (and similar branches)
// lazily call initializeWorld on the first non-WorldConnect message
// from a new world. Kept permanently.
//
// NAI-S1-D-LAZY-WORLDINIT — for reviewer traceability; not retired.
func (h *handler) ensureWorld(worldId int32) {
	h.repo.initializeWorldIfAbsent(worldId, h.cfg.WorldPlayerLimit)
}

// WorldConnect validates the profile and initializes the world's slot.
// Mirrors TS FriendServer WORLD_CONNECT (FriendServer.ts:89-106).
// Re-init by the same world resets that world's player counter to 0.
func (h *handler) WorldConnect(_ context.Context, req *friendspb.WorldConnectRequest) (*emptypb.Empty, error) {
	if req.Profile != h.cfg.NodeProfile {
		return nil, status.Errorf(codes.InvalidArgument,
			"profile mismatch: got %q, want %q", req.Profile, h.cfg.NodeProfile)
	}
	h.repo.InitializeWorld(req.WorldId, h.cfg.WorldPlayerLimit)
	return &emptypb.Empty{}, nil
}

// coercePrivateChat clamps a TS ChatModePrivate value to the valid range.
// Invalid values become 0 (ON). Mirrors TS FriendServer.ts:120-123.
func coercePrivateChat(v int32) int32 {
	if v < 0 || v > 2 {
		return 0
	}
	return v
}

// PlayerLogin registers the player on the given world. Always returns OK;
// PlayerLoginResponse.Accepted is false iff the world's player cap is
// reached.
//
// NAI-S1-D-PLAYERCAP-LOG-ONLY — cap rejection logs warn but does not error.
// Slice 4 surfaces Accepted to callers; slice 1 callers ignore the field.
func (h *handler) PlayerLogin(_ context.Context, req *friendspb.PlayerLoginRequest) (*friendspb.PlayerLoginResponse, error) {
	h.ensureWorld(req.WorldId)
	pc := coercePrivateChat(req.PrivateChat)
	// TS-faithful: PLAYER_LOGIN unregisters first to dedupe across worlds.
	h.repo.Unregister(req.Username37)
	accepted := h.repo.Register(req.WorldId, req.Username37, pc, req.StaffLvl)
	if !accepted {
		h.log.Warn("friends-server player cap reached",
			slog.Int("world_id", int(req.WorldId)),
			slog.Uint64("username37", req.Username37),
		)
	}
	return &friendspb.PlayerLoginResponse{Accepted: accepted}, nil
}

// PlayerLogout removes the player from whichever world they're on.
// Idempotent on unknown players.
func (h *handler) PlayerLogout(_ context.Context, req *friendspb.PlayerLogoutRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	h.repo.Unregister(req.Username37)
	return &emptypb.Empty{}, nil
}

// ChatSetMode updates the player's privateChat setting. Invalid values
// are coerced to 0 (ON), matching TS FriendServer.ts:176-179. No-op on
// unknown player (state lives at the player record, which doesn't exist
// pre-login).
//
// NAI-S1-D-NO-FOLLOWER-BROADCAST — slice 4 will broadcast the new mode
// to followers; slice 1 just mutates state.
func (h *handler) ChatSetMode(_ context.Context, req *friendspb.ChatSetModeRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	h.repo.SetChatMode(req.Username37, coercePrivateChat(req.PrivateChat))
	return &emptypb.Empty{}, nil
}
