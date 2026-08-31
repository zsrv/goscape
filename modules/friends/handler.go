package friends

import (
	"context"
	"errors"
	"log/slog"
	"uuid"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zsrv/goscape/pkg/eventspb"
	"github.com/zsrv/goscape/pkg/friendspb"
	"github.com/zsrv/goscape/pkg/telemetry"
)

// handler implements friendspb.FriendsServiceServer.
type handler struct {
	friendspb.UnimplementedFriendsServiceServer

	repo      *Repository
	subs      *subscriptions
	worldSubs *worldSubscriptions
	cfg       Config
	log       *slog.Logger
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
//
// L45: TS WORLD_CONNECT also calls initializeWorld(world, socket), which
// terminates any prior socket for that world (FriendServer.ts:412-419,
// `socketByWorld[world].terminate()`). goscape splits TS's single
// per-world WebSocket into two pieces — this one-shot WorldConnect init
// RPC and the persistent SubscribeWorldEvents push stream — so there is
// no socket to terminate HERE. The terminate-prior semantics are
// preserved in the equivalent layer: worldSubscriptions.register
// (world_subscriptions.go:57-64) closes the prior subscriber's done
// channel on re-subscribe. See NAI-S4A-D-PERPLAYER-NOT-PERWORLD-STREAM.
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
// reached. The world acts on the rejection in modules/world/tick.go's
// processLogins callback (slice 4c).
func (h *handler) PlayerLogin(ctx context.Context, req *friendspb.PlayerLoginRequest) (*friendspb.PlayerLoginResponse, error) {
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
		// No broadcast on rejection — player isn't on any world.
		return &friendspb.PlayerLoginResponse{Accepted: false}, nil
	}
	h.broadcastWorldToFollowers(ctx, req.Username37)
	return &friendspb.PlayerLoginResponse{Accepted: true}, nil
}

// PlayerLogout removes the player from whichever world they're on.
// Idempotent on unknown players. Broadcasts the (now-offline) world to
// followers after Unregister.
func (h *handler) PlayerLogout(ctx context.Context, req *friendspb.PlayerLogoutRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	h.repo.Unregister(req.Username37)
	h.broadcastWorldToFollowers(ctx, req.Username37)
	return &emptypb.Empty{}, nil
}

// ChatSetMode updates the player's privateChat setting. Invalid values
// are coerced to 0 (ON), matching TS FriendServer.ts:176-179. No-op on
// unknown player (state lives at the player record, which doesn't exist
// pre-login).
func (h *handler) ChatSetMode(ctx context.Context, req *friendspb.ChatSetModeRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	h.repo.SetChatMode(req.Username37, coercePrivateChat(req.PrivateChat))
	h.broadcastWorldToFollowers(ctx, req.Username37)
	return &emptypb.Empty{}, nil
}

// FriendlistAdd appends target to the player's friend set (idempotent).
// Sends a single-friend update to the adder for `target` (TS
// sendPlayerWorldUpdate at FriendServer.ts:200) and then broadcasts the
// adder's world to all followers (TS FriendServer.ts:204).
func (h *handler) FriendlistAdd(ctx context.Context, req *friendspb.FriendlistAddRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	if err := h.repo.AddFriend(ctx, req.Username37, req.TargetUsername37); err != nil {
		return nil, status.Errorf(codes.Internal, "AddFriend: %v", err)
	}
	h.sendPlayerWorldUpdate(ctx, req.Username37, req.TargetUsername37)
	h.broadcastWorldToFollowers(ctx, req.Username37)
	return &emptypb.Empty{}, nil
}

// FriendlistDel removes target from the player's friend set (idempotent).
// Broadcasts the remover's world to followers (TS FriendServer.ts:221).
func (h *handler) FriendlistDel(ctx context.Context, req *friendspb.FriendlistDelRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	if err := h.repo.DeleteFriend(ctx, req.Username37, req.TargetUsername37); err != nil {
		return nil, status.Errorf(codes.Internal, "DeleteFriend: %v", err)
	}
	h.broadcastWorldToFollowers(ctx, req.Username37)
	return &emptypb.Empty{}, nil
}

// IgnorelistAdd appends target to the player's ignore set (idempotent).
// Broadcasts the adder's world to followers (TS FriendServer.ts:238).
func (h *handler) IgnorelistAdd(ctx context.Context, req *friendspb.IgnorelistAddRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	if err := h.repo.AddIgnore(ctx, req.Username37, req.TargetUsername37); err != nil {
		return nil, status.Errorf(codes.Internal, "AddIgnore: %v", err)
	}
	h.broadcastWorldToFollowers(ctx, req.Username37)
	return &emptypb.Empty{}, nil
}

// IgnorelistDel removes target from the player's ignore set (idempotent).
// Broadcasts the remover's world to followers (TS FriendServer.ts:255).
func (h *handler) IgnorelistDel(ctx context.Context, req *friendspb.IgnorelistDelRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	if err := h.repo.DeleteIgnore(ctx, req.Username37, req.TargetUsername37); err != nil {
		return nil, status.Errorf(codes.Internal, "DeleteIgnore: %v", err)
	}
	h.broadcastWorldToFollowers(ctx, req.Username37)
	return &emptypb.Empty{}, nil
}

// PrivateMessage resolves both endpoint accounts, emits the PM's
// PrivateChatEvent (chat is Kafka-only — documented TS divergence, spec
// docs/superpowers/specs/2026-07-07-chat-kafka-only-design.md; TS
// FriendServer.ts:266-285 @e1dea19f inserts a private_chat row here
// instead), then routes a PrivateMessageDelivery to the target's open
// stream (if any). Both endpoint accounts are resolved against the
// central database first; if either is missing the PM is dropped
// silently — no event, no delivery, successful result (TS throws inside
// the message handler and the outer catch swallows it,
// FriendServer.ts:270-271). Other resolve failures keep the established
// codes.Internal posture of FriendlistAdd/Del/IgnorelistAdd/Del.
//
// req.Coord rides the event (and is otherwise unused for routing).
// req.WorldId is unused for routing because the registry is keyed
// solely by username37; cross-world routing therefore falls out for
// free.
//
// Retires NAI-S4A-D-FED-NO-ACCOUNT-EXISTENCE-CHECK: that exception
// existed because the friends server was federated from the login/
// account store with no `account` table to JOIN against. Now that
// friends is a client of the central database (P4 central-db port),
// the existence check TS always had is restored via
// Repository.ResolvePrivateMessageEndpoints's errAccountMissing sentinel.
func (h *handler) PrivateMessage(ctx context.Context, req *friendspb.PrivateMessageRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	fromID, toID, err := h.repo.ResolvePrivateMessageEndpoints(ctx, req.Username37, req.TargetUsername37)
	if err != nil {
		if errors.Is(err, errAccountMissing) {
			return &emptypb.Empty{}, nil
		}
		return nil, status.Errorf(codes.Internal, "ResolvePrivateMessageEndpoints: %v", err)
	}
	telemetry.Get().EmitPlayerInput(&eventspb.PlayerInputEnvelope{
		SchemaVersion: 1,
		EventId:       uuid.New().String(),
		Ts:            timestamppb.Now(),
		AccountId:     fromID,
		WorldId:       req.WorldId,
		Payload: &eventspb.PlayerInputEnvelope_PrivateChat{
			PrivateChat: &eventspb.PrivateChatEvent{
				RecipientAccountId: toID,
				Text:               req.Chat,
				Coord:              req.Coord,
			},
		},
	})
	h.subs.send(req.TargetUsername37, &friendspb.FriendsUpdate{
		Update: &friendspb.FriendsUpdate_PrivateMessage{
			PrivateMessage: &friendspb.PrivateMessageDelivery{
				FromUsername37: req.Username37,
				StaffLvl:       req.StaffLvl,
				PmId:           req.PmId,
				Chat:           req.Chat,
			},
		},
	})
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

// --- slice 5a: RELAY_* admin relay handlers ---
//
// All Relay* methods forward a WorldEvent to the target world's
// SubscribeWorldEvents subscriber. No-op if no world is subscribed for
// req.TargetWorldId (matches TS FriendServer.ts:298-302 silent-drop on
// missing socketByWorld). No auth check at this layer.
//
// NAI-S5A-D-NO-ADMIN-AUTH-AT-SERVER — friends-server is dumb routing;
//   admin checks live on both sender and receiver world. Permanent.
// NAI-S5A-D-DISPATCHER-NO-ACTION — slice 5a default WorldEventsDispatcher
//   on the receiving side logs only; slice 5b retires this piecewise as
//   each opcode's world-state action is wired.

func (h *handler) RelayMute(_ context.Context, req *friendspb.RelayMuteRequest) (*emptypb.Empty, error) {
	h.worldSubs.send(req.TargetWorldId, &friendspb.WorldEvent{
		Event: &friendspb.WorldEvent_Mute{Mute: &friendspb.MuteEvent{
			Username37:   req.Username37,
			MutedUntilMs: req.MutedUntilMs,
		}},
	})
	return &emptypb.Empty{}, nil
}

func (h *handler) RelayKick(_ context.Context, req *friendspb.RelayKickRequest) (*emptypb.Empty, error) {
	h.worldSubs.send(req.TargetWorldId, &friendspb.WorldEvent{
		Event: &friendspb.WorldEvent_Kick{Kick: &friendspb.KickEvent{Username37: req.Username37}},
	})
	return &emptypb.Empty{}, nil
}

func (h *handler) RelayShutdown(_ context.Context, req *friendspb.RelayShutdownRequest) (*emptypb.Empty, error) {
	h.worldSubs.send(req.TargetWorldId, &friendspb.WorldEvent{
		Event: &friendspb.WorldEvent_Shutdown{Shutdown: &friendspb.ShutdownEvent{DurationTicks: req.DurationTicks}},
	})
	return &emptypb.Empty{}, nil
}

func (h *handler) RelayBroadcast(_ context.Context, req *friendspb.RelayBroadcastRequest) (*emptypb.Empty, error) {
	h.worldSubs.send(req.TargetWorldId, &friendspb.WorldEvent{
		Event: &friendspb.WorldEvent_Broadcast{Broadcast: &friendspb.BroadcastEvent{Message: req.Message}},
	})
	return &emptypb.Empty{}, nil
}

func (h *handler) RelayTrack(_ context.Context, req *friendspb.RelayTrackRequest) (*emptypb.Empty, error) {
	h.worldSubs.send(req.TargetWorldId, &friendspb.WorldEvent{
		Event: &friendspb.WorldEvent_Track{Track: &friendspb.TrackEvent{Username37: req.Username37, State: req.State}},
	})
	return &emptypb.Empty{}, nil
}

func (h *handler) RelayReload(_ context.Context, req *friendspb.RelayReloadRequest) (*emptypb.Empty, error) {
	h.worldSubs.send(req.TargetWorldId, &friendspb.WorldEvent{
		Event: &friendspb.WorldEvent_Reload{Reload: &friendspb.ReloadEvent{}},
	})
	return &emptypb.Empty{}, nil
}

func (h *handler) RelayClearLogins(_ context.Context, req *friendspb.RelayClearLoginsRequest) (*emptypb.Empty, error) {
	h.worldSubs.send(req.TargetWorldId, &friendspb.WorldEvent{
		Event: &friendspb.WorldEvent_ClearLogins{ClearLogins: &friendspb.ClearLoginsEvent{}},
	})
	return &emptypb.Empty{}, nil
}

func (h *handler) RelayClearLogouts(_ context.Context, req *friendspb.RelayClearLogoutsRequest) (*emptypb.Empty, error) {
	h.worldSubs.send(req.TargetWorldId, &friendspb.WorldEvent{
		Event: &friendspb.WorldEvent_ClearLogouts{ClearLogouts: &friendspb.ClearLogoutsEvent{}},
	})
	return &emptypb.Empty{}, nil
}

func (h *handler) RelayQueueScript(_ context.Context, req *friendspb.RelayQueueScriptRequest) (*emptypb.Empty, error) {
	h.worldSubs.send(req.TargetWorldId, &friendspb.WorldEvent{
		Event: &friendspb.WorldEvent_QueueScript{QueueScript: &friendspb.QueueScriptEvent{
			ScriptName: req.ScriptName,
			Username37: req.Username37,
		}},
	})
	return &emptypb.Empty{}, nil
}

// SubscribeWorldEvents streams server -> world admin events for one
// world. One subscriber per worldId; re-subscribe terminates the prior.
// Slice 5a opens the stream; slice 5b layers world-state action handlers
// on the world side via WorldEventsDispatcher.
//
// Replaces the slice-1 codes.Unimplemented stub.
func (h *handler) SubscribeWorldEvents(req *friendspb.SubscribeWorldEventsRequest, stream friendspb.FriendsService_SubscribeWorldEventsServer) error {
	sub := newWorldSubscriber(req.WorldId)
	h.worldSubs.register(sub)
	defer h.worldSubs.deregister(sub)

	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-sub.done:
			return nil
		case ev := <-sub.ch:
			if err := stream.Send(ev); err != nil {
				return err
			}
		}
	}
}
