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
	subs *subscriptions
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

// FriendlistAdd appends target to the player's friend set (idempotent).
// NAI-S1-D-NO-FOLLOWER-BROADCAST — slice 4 will broadcast.
func (h *handler) FriendlistAdd(ctx context.Context, req *friendspb.FriendlistAddRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	if err := h.repo.AddFriend(ctx, req.Username37, req.TargetUsername37); err != nil {
		return nil, status.Errorf(codes.Internal, "AddFriend: %v", err)
	}
	return &emptypb.Empty{}, nil
}

// FriendlistDel removes target from the player's friend set (idempotent).
// NAI-S1-D-NO-FOLLOWER-BROADCAST — slice 4 will broadcast.
func (h *handler) FriendlistDel(ctx context.Context, req *friendspb.FriendlistDelRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	if err := h.repo.DeleteFriend(ctx, req.Username37, req.TargetUsername37); err != nil {
		return nil, status.Errorf(codes.Internal, "DeleteFriend: %v", err)
	}
	return &emptypb.Empty{}, nil
}

// IgnorelistAdd appends target to the player's ignore set (idempotent).
// NAI-S1-D-NO-FOLLOWER-BROADCAST — slice 4 will broadcast.
func (h *handler) IgnorelistAdd(ctx context.Context, req *friendspb.IgnorelistAddRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	if err := h.repo.AddIgnore(ctx, req.Username37, req.TargetUsername37); err != nil {
		return nil, status.Errorf(codes.Internal, "AddIgnore: %v", err)
	}
	return &emptypb.Empty{}, nil
}

// IgnorelistDel removes target from the player's ignore set (idempotent).
// NAI-S1-D-NO-FOLLOWER-BROADCAST — slice 4 will broadcast.
func (h *handler) IgnorelistDel(ctx context.Context, req *friendspb.IgnorelistDelRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	if err := h.repo.DeleteIgnore(ctx, req.Username37, req.TargetUsername37); err != nil {
		return nil, status.Errorf(codes.Internal, "DeleteIgnore: %v", err)
	}
	return &emptypb.Empty{}, nil
}

// PrivateMessage accepts and logs the call. Delivery is deferred to
// slice 4 (server -> world push via SubscribeUpdates). Persistence is
// deferred to slice 6 (private_chat DB table).
//
// NAI-S1-D-PM-NO-DELIVERY — slice 4 retires.
// NAI-S1-D-PM-NO-PERSISTENCE — slice 6 retires.
func (h *handler) PrivateMessage(_ context.Context, req *friendspb.PrivateMessageRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	h.log.Debug("friends-server received private message",
		slog.Int("world_id", int(req.WorldId)),
		slog.Uint64("from", req.Username37),
		slog.Uint64("to", req.TargetUsername37),
		slog.Uint64("pm_id", uint64(req.PmId)),
	)
	return &emptypb.Empty{}, nil
}

// SubscribeUpdates streams server -> world friends updates for one
// (worldId, username37) pair. Mirrors TS FriendServer's WebSocket-per-
// world push channel, but proto-typed per (world, player). Sends initial
// UPDATE_FRIENDLIST + UPDATE_IGNORELIST snapshots on attach, then drains
// the subscriber's channel until the stream context or done signal.
//
// Replaces the slice-1 codes.Unimplemented stub.
//
// NAI-S4A-D-PERPLAYER-NOT-PERWORLD-STREAM — proto-baked architectural
// choice; TS keeps one socket per world, goscape one stream per
// (world, player). Permanent.
func (h *handler) SubscribeUpdates(req *friendspb.SubscribeUpdatesRequest, stream friendspb.FriendsService_SubscribeUpdatesServer) error {
	h.ensureWorld(req.WorldId)

	sub := newSubscriber(req.WorldId, req.Username37)
	h.subs.register(sub)
	defer h.subs.deregister(sub)

	ctx := stream.Context()

	// Initial snapshots (TS FriendServer sendFriendsListToPlayer +
	// sendIgnoreListToPlayer, FriendServer.ts:138-139, but on subscribe
	// instead of login).
	if err := h.sendInitialFriendlist(ctx, stream, req.Username37); err != nil {
		return err
	}
	if err := h.sendInitialIgnorelist(ctx, stream, req.Username37); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-sub.done:
			return nil
		case u := <-sub.ch:
			if err := stream.Send(u); err != nil {
				return err
			}
		}
	}
}

// sendInitialFriendlist mirrors TS sendFriendsListToPlayer
// (FriendServer.ts:421-431). Builds one FriendlistUpdate containing
// every friend with the friend's current world (0 if offline). Visibility
// rules are applied via worldIfVisible (scalar IsVisibleTo per entry —
// the broadcast hot path uses IsVisibleToMany instead).
func (h *handler) sendInitialFriendlist(ctx context.Context, stream friendspb.FriendsService_SubscribeUpdatesServer, viewer uint64) error {
	friends, err := h.repo.GetFriends(ctx, viewer)
	if err != nil {
		return status.Errorf(codes.Internal, "GetFriends: %v", err)
	}
	entries := make([]*friendspb.FriendEntry, 0, len(friends))
	for _, f := range friends {
		entries = append(entries, &friendspb.FriendEntry{
			WorldId:    h.worldIfVisible(ctx, viewer, f),
			Username37: f,
		})
	}
	return stream.Send(&friendspb.FriendsUpdate{
		Update: &friendspb.FriendsUpdate_Friendlist{
			Friendlist: &friendspb.FriendlistUpdate{Entries: entries},
		},
	})
}

// sendInitialIgnorelist mirrors TS sendIgnoreListToPlayer
// (FriendServer.ts:433-443).
func (h *handler) sendInitialIgnorelist(ctx context.Context, stream friendspb.FriendsService_SubscribeUpdatesServer, viewer uint64) error {
	ignores, err := h.repo.GetIgnores(ctx, viewer)
	if err != nil {
		return status.Errorf(codes.Internal, "GetIgnores: %v", err)
	}
	return stream.Send(&friendspb.FriendsUpdate{
		Update: &friendspb.FriendsUpdate_Ignorelist{
			Ignorelist: &friendspb.IgnorelistUpdate{Username37: ignores},
		},
	})
}

// worldIfVisible is the per-entry visibility helper used by initial
// snapshots. For the broadcast hot path use IsVisibleToMany.
func (h *handler) worldIfVisible(ctx context.Context, viewer, other uint64) int32 {
	visible, err := h.repo.IsVisibleTo(ctx, viewer, other)
	if err != nil {
		h.log.Warn("IsVisibleTo failed; treating as not visible",
			slog.Uint64("viewer", viewer),
			slog.Uint64("other", other),
			slog.Any("err", err))
		return 0
	}
	if !visible {
		return 0
	}
	return h.repo.GetWorld(other)
}

// broadcastWorldToFollowers fans out a one-entry FriendlistUpdate to
// each of `other`'s followers that has an open subscription. Mirrors
// TS FriendServer.broadcastWorldToFollowers (FriendServer.ts:445-451).
// Errors are logged at Warn but never block the RPC caller; the
// friends-server is best-effort by design.
//
// NAI-S1-D-NO-FOLLOWER-BROADCAST — slice 4a retires this tag by wiring
// this helper into the seven mutating RPC handlers.
func (h *handler) broadcastWorldToFollowers(ctx context.Context, other uint64) {
	followers, err := h.repo.GetFollowers(ctx, other)
	if err != nil {
		h.log.Warn("broadcastWorldToFollowers: GetFollowers failed",
			slog.Uint64("other", other),
			slog.Any("err", err))
		return
	}
	if len(followers) == 0 {
		return
	}
	visibility, err := h.repo.IsVisibleToMany(ctx, followers, other)
	if err != nil {
		h.log.Warn("broadcastWorldToFollowers: IsVisibleToMany failed",
			slog.Uint64("other", other),
			slog.Any("err", err))
		return
	}
	otherWorld := h.repo.GetWorld(other)
	for _, viewer := range followers {
		worldForViewer := int32(0)
		if visibility[viewer] {
			worldForViewer = otherWorld
		}
		h.subs.send(viewer, &friendspb.FriendsUpdate{
			Update: &friendspb.FriendsUpdate_Friendlist{
				Friendlist: &friendspb.FriendlistUpdate{
					Entries: []*friendspb.FriendEntry{{
						WorldId:    worldForViewer,
						Username37: other,
					}},
				},
			},
		})
	}
}

// sendPlayerWorldUpdate pushes a single-friend update to viewer's
// subscription. Mirrors TS FriendServer.sendPlayerWorldUpdate
// (FriendServer.ts:462-478). Called by FriendlistAdd to notify the
// adder of the new friend's current world.
func (h *handler) sendPlayerWorldUpdate(ctx context.Context, viewer, other uint64) {
	visible, err := h.repo.IsVisibleTo(ctx, viewer, other)
	if err != nil {
		h.log.Warn("sendPlayerWorldUpdate: IsVisibleTo failed",
			slog.Uint64("viewer", viewer),
			slog.Uint64("other", other),
			slog.Any("err", err))
		visible = false
	}
	worldForViewer := int32(0)
	if visible {
		worldForViewer = h.repo.GetWorld(other)
	}
	h.subs.send(viewer, &friendspb.FriendsUpdate{
		Update: &friendspb.FriendsUpdate_Friendlist{
			Friendlist: &friendspb.FriendlistUpdate{
				Entries: []*friendspb.FriendEntry{{
					WorldId:    worldForViewer,
					Username37: other,
				}},
			},
		},
	})
}
