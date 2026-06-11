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

	repos     *repositories
	subs      *subscriptions
	worldSubs *worldSubscriptions
	cfg       Config
	log       *slog.Logger
}

// ensureWorld lazy-inits the world's player slot for the given profile if
// not already known. TS-faithful behavior: FriendServer.ts:108-115 (and
// similar branches) lazily call initializeWorld on the first non-WorldConnect
// message from a new world. Kept permanently.
//
// NAI-S1-D-LAZY-WORLDINIT — for reviewer traceability; not retired.
func (h *handler) ensureWorld(profile string, worldId int32) {
	h.repos.get(profile).initializeWorldIfAbsent(worldId, h.cfg.WorldPlayerLimit)
}

// WorldConnect initializes the world's slot for the given profile.
// Mirrors TS FriendServer WORLD_CONNECT (FriendServer.ts:92-103).
// Re-init by the same world resets that world's player counter to 0.
//
// Note: TS 244 REMOVED the profile-mismatch reject that existed at 225
// (verified at FriendServer.ts:92-103 in commit 9aadcec4 — the block
// simply sets world/profile and calls initializeWorld with no comparison
// against a server-side configured profile). The server accepts any
// profile string and routes it into the corresponding per-profile
// repository.
//
// L45: TS WORLD_CONNECT also calls initializeWorld(profile, world, socket),
// which terminates any prior socket for that (profile, world) pair
// (FriendServer.ts:432-440, `socketByWorld[profile][world].terminate()`).
// goscape splits TS's single per-world WebSocket into two pieces — this
// one-shot WorldConnect init RPC and the persistent SubscribeWorldEvents
// push stream — so there is no socket to terminate HERE. The terminate-prior
// semantics are preserved in the equivalent layer:
// worldSubscriptions.register (world_subscriptions.go) closes the prior
// subscriber's done channel on re-subscribe per (profile, worldId).
// See NAI-S4A-D-PERPLAYER-NOT-PERWORLD-STREAM.
func (h *handler) WorldConnect(_ context.Context, req *friendspb.WorldConnectRequest) (*emptypb.Empty, error) {
	h.repos.get(req.Profile).InitializeWorld(req.WorldId, h.cfg.WorldPlayerLimit)
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
	repo := h.repos.get(req.Profile)
	h.ensureWorld(req.Profile, req.WorldId)
	pc := coercePrivateChat(req.PrivateChat)
	// TS-faithful: PLAYER_LOGIN unregisters first to dedupe across worlds.
	repo.Unregister(req.Username37)
	accepted := repo.Register(req.WorldId, req.Username37, pc, req.StaffLvl)
	if !accepted {
		h.log.Warn("friends-server player cap reached",
			slog.Int("world_id", int(req.WorldId)),
			slog.Uint64("username37", req.Username37),
		)
		// No broadcast on rejection — player isn't on any world.
		return &friendspb.PlayerLoginResponse{Accepted: false}, nil
	}
	h.broadcastWorldToFollowers(ctx, req.Profile, req.Username37)
	return &friendspb.PlayerLoginResponse{Accepted: true}, nil
}

// PlayerLogout removes the player from whichever world they're on.
// Idempotent on unknown players. Broadcasts the (now-offline) world to
// followers after Unregister.
func (h *handler) PlayerLogout(ctx context.Context, req *friendspb.PlayerLogoutRequest) (*emptypb.Empty, error) {
	repo := h.repos.get(req.Profile)
	h.ensureWorld(req.Profile, req.WorldId)
	repo.Unregister(req.Username37)
	h.broadcastWorldToFollowers(ctx, req.Profile, req.Username37)
	return &emptypb.Empty{}, nil
}

// ChatSetMode updates the player's privateChat setting. Invalid values
// are coerced to 0 (ON), matching TS FriendServer.ts:176-179. No-op on
// unknown player (state lives at the player record, which doesn't exist
// pre-login).
func (h *handler) ChatSetMode(ctx context.Context, req *friendspb.ChatSetModeRequest) (*emptypb.Empty, error) {
	repo := h.repos.get(req.Profile)
	h.ensureWorld(req.Profile, req.WorldId)
	repo.SetChatMode(req.Username37, coercePrivateChat(req.PrivateChat))
	h.broadcastWorldToFollowers(ctx, req.Profile, req.Username37)
	return &emptypb.Empty{}, nil
}

// FriendlistAdd appends target to the player's friend set (idempotent).
// Sends a single-friend update to the adder for `target` (TS
// sendPlayerWorldUpdate at FriendServer.ts:200) and then broadcasts the
// adder's world to all followers (TS FriendServer.ts:204).
func (h *handler) FriendlistAdd(ctx context.Context, req *friendspb.FriendlistAddRequest) (*emptypb.Empty, error) {
	repo := h.repos.get(req.Profile)
	h.ensureWorld(req.Profile, req.WorldId)
	if err := repo.AddFriend(ctx, req.Username37, req.TargetUsername37); err != nil {
		return nil, status.Errorf(codes.Internal, "AddFriend: %v", err)
	}
	h.sendPlayerWorldUpdate(ctx, req.Profile, req.Username37, req.TargetUsername37)
	h.broadcastWorldToFollowers(ctx, req.Profile, req.Username37)
	return &emptypb.Empty{}, nil
}

// FriendlistDel removes target from the player's friend set (idempotent).
// Broadcasts the remover's world to followers (TS FriendServer.ts:221).
func (h *handler) FriendlistDel(ctx context.Context, req *friendspb.FriendlistDelRequest) (*emptypb.Empty, error) {
	repo := h.repos.get(req.Profile)
	h.ensureWorld(req.Profile, req.WorldId)
	if err := repo.DeleteFriend(ctx, req.Username37, req.TargetUsername37); err != nil {
		return nil, status.Errorf(codes.Internal, "DeleteFriend: %v", err)
	}
	h.broadcastWorldToFollowers(ctx, req.Profile, req.Username37)
	return &emptypb.Empty{}, nil
}

// IgnorelistAdd appends target to the player's ignore set (idempotent).
// Broadcasts the adder's world to followers (TS FriendServer.ts:238).
func (h *handler) IgnorelistAdd(ctx context.Context, req *friendspb.IgnorelistAddRequest) (*emptypb.Empty, error) {
	repo := h.repos.get(req.Profile)
	h.ensureWorld(req.Profile, req.WorldId)
	if err := repo.AddIgnore(ctx, req.Username37, req.TargetUsername37); err != nil {
		return nil, status.Errorf(codes.Internal, "AddIgnore: %v", err)
	}
	h.broadcastWorldToFollowers(ctx, req.Profile, req.Username37)
	return &emptypb.Empty{}, nil
}

// IgnorelistDel removes target from the player's ignore set (idempotent).
// Broadcasts the remover's world to followers (TS FriendServer.ts:255).
func (h *handler) IgnorelistDel(ctx context.Context, req *friendspb.IgnorelistDelRequest) (*emptypb.Empty, error) {
	repo := h.repos.get(req.Profile)
	h.ensureWorld(req.Profile, req.WorldId)
	if err := repo.DeleteIgnore(ctx, req.Username37, req.TargetUsername37); err != nil {
		return nil, status.Errorf(codes.Internal, "DeleteIgnore: %v", err)
	}
	h.broadcastWorldToFollowers(ctx, req.Profile, req.Username37)
	return &emptypb.Empty{}, nil
}

// PrivateMessage persists the PM to private_chat under the request profile,
// then routes a PrivateMessageDelivery to the target's open stream
// (if any). Mirrors TS FriendServer.ts:266-285 — insert-then-send,
// fail the RPC on insert error (matches the established
// codes.Internal posture of FriendlistAdd/Del/IgnorelistAdd/Del).
//
// req.Coord is server-side-persisted (and otherwise unused for
// routing). req.WorldId is unused for routing because the registry
// is keyed solely by (profile, username37); cross-world routing
// therefore falls out for free.
//
// NAI-S4A-D-FED-NO-ACCOUNT-EXISTENCE-CHECK — CONFIRMED EXCEPTION
// (closes friend-server-5 from the 2026-05-28 fresh audit). TS resolves
// both endpoint accounts via executeTakeFirstOrThrow (FriendServer.ts
// :270-285), which throws on a missing account and drops the PM. goscape
// stores the raw username37 ints with no existence check, so a PM to a
// non-existent account is logged and routed.
//
// This is structural, not a bug: per the DB-2 schema-decoupling rationale
// at modules/friends/db.go:21-35, the friends server is intentionally
// federated from the login/account store (separate SQLite DB, separate
// gRPC service). There is no `account` table to JOIN against — the
// equivalent existence check would require a cross-service RPC into
// login, adding a latency-and-availability dependency to every PM RPC,
// which contradicts the federation choice. Subscriber-routing already
// degrades correctly: a PM to an account that no live world is hosting
// silently no-ops at h.subs.send (no subscriber → drop). Orphan
// private_chat rows from deleted accounts are accepted as the same
// federation trade-off documented in db.go for friendlist / ignorelist /
// public_chat rows.
func (h *handler) PrivateMessage(ctx context.Context, req *friendspb.PrivateMessageRequest) (*emptypb.Empty, error) {
	repo := h.repos.get(req.Profile)
	h.ensureWorld(req.Profile, req.WorldId)
	if err := repo.LogPrivateMessage(ctx, req.Username37, req.TargetUsername37, req.Coord, req.Chat); err != nil {
		return nil, status.Errorf(codes.Internal, "LogPrivateMessage: %v", err)
	}
	h.subs.send(req.Profile, req.TargetUsername37, &friendspb.FriendsUpdate{
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
// (profile, worldId, username37) triple. Mirrors TS FriendServer's
// WebSocket-per-world push channel, but proto-typed per (world, player).
// Sends initial UPDATE_FRIENDLIST + UPDATE_IGNORELIST snapshots on
// attach, then drains the subscriber's channel until the stream context
// or done signal.
//
// Replaces the slice-1 codes.Unimplemented stub.
//
// NAI-S4A-D-PERPLAYER-NOT-PERWORLD-STREAM — proto-baked architectural
// choice; TS keeps one socket per (profile, world), goscape one stream
// per (profile, world, player). Permanent.
func (h *handler) SubscribeUpdates(req *friendspb.SubscribeUpdatesRequest, stream friendspb.FriendsService_SubscribeUpdatesServer) error {
	h.ensureWorld(req.Profile, req.WorldId)

	sub := newSubscriber(req.Profile, req.WorldId, req.Username37)
	h.subs.register(sub)
	defer h.subs.deregister(sub)

	ctx := stream.Context()

	// Initial snapshots (TS FriendServer sendFriendsListToPlayer +
	// sendIgnoreListToPlayer, FriendServer.ts:138-139, but on subscribe
	// instead of login).
	if err := h.sendInitialFriendlist(ctx, req.Profile, stream, req.Username37); err != nil {
		return err
	}
	if err := h.sendInitialIgnorelist(ctx, req.Profile, stream, req.Username37); err != nil {
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
// (FriendServer.ts:449-459). Builds one FriendlistUpdate containing
// every friend with the friend's current world (0 if offline). Visibility
// rules are applied via worldIfVisible (scalar IsVisibleTo per entry —
// the broadcast hot path uses IsVisibleToMany instead).
func (h *handler) sendInitialFriendlist(ctx context.Context, profile string, stream friendspb.FriendsService_SubscribeUpdatesServer, viewer uint64) error {
	repo := h.repos.get(profile)
	friends, err := repo.GetFriends(ctx, viewer)
	if err != nil {
		return status.Errorf(codes.Internal, "GetFriends: %v", err)
	}
	entries := make([]*friendspb.FriendEntry, 0, len(friends))
	for _, f := range friends {
		entries = append(entries, &friendspb.FriendEntry{
			WorldId:    h.worldIfVisible(ctx, profile, viewer, f),
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
// (FriendServer.ts:461-471).
func (h *handler) sendInitialIgnorelist(ctx context.Context, profile string, stream friendspb.FriendsService_SubscribeUpdatesServer, viewer uint64) error {
	repo := h.repos.get(profile)
	ignores, err := repo.GetIgnores(ctx, viewer)
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
func (h *handler) worldIfVisible(ctx context.Context, profile string, viewer, other uint64) int32 {
	repo := h.repos.get(profile)
	visible, err := repo.IsVisibleTo(ctx, viewer, other)
	if err != nil {
		h.log.Warn("IsVisibleTo failed; treating as not visible",
			slog.String("profile", profile),
			slog.Uint64("viewer", viewer),
			slog.Uint64("other", other),
			slog.Any("err", err))
		return 0
	}
	if !visible {
		return 0
	}
	return repo.GetWorld(other)
}

// broadcastWorldToFollowers fans out a one-entry FriendlistUpdate to
// each of `other`'s followers that has an open subscription. Mirrors
// TS FriendServer.broadcastWorldToFollowers (FriendServer.ts:473-479).
// Errors are logged at Warn but never block the RPC caller; the
// friends-server is best-effort by design.
//
// NAI-S1-D-NO-FOLLOWER-BROADCAST — slice 4a retires this tag by wiring
// this helper into the seven mutating RPC handlers.
func (h *handler) broadcastWorldToFollowers(ctx context.Context, profile string, other uint64) {
	repo := h.repos.get(profile)
	followers, err := repo.GetFollowers(ctx, other)
	if err != nil {
		h.log.Warn("broadcastWorldToFollowers: GetFollowers failed",
			slog.String("profile", profile),
			slog.Uint64("other", other),
			slog.Any("err", err))
		return
	}
	if len(followers) == 0 {
		return
	}
	visibility, err := repo.IsVisibleToMany(ctx, followers, other)
	if err != nil {
		h.log.Warn("broadcastWorldToFollowers: IsVisibleToMany failed",
			slog.String("profile", profile),
			slog.Uint64("other", other),
			slog.Any("err", err))
		return
	}
	otherWorld := repo.GetWorld(other)
	for _, viewer := range followers {
		worldForViewer := int32(0)
		if visibility[viewer] {
			worldForViewer = otherWorld
		}
		h.subs.send(profile, viewer, &friendspb.FriendsUpdate{
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
// (FriendServer.ts:490-506). Called by FriendlistAdd to notify the
// adder of the new friend's current world.
func (h *handler) sendPlayerWorldUpdate(ctx context.Context, profile string, viewer, other uint64) {
	repo := h.repos.get(profile)
	visible, err := repo.IsVisibleTo(ctx, viewer, other)
	if err != nil {
		h.log.Warn("sendPlayerWorldUpdate: IsVisibleTo failed",
			slog.String("profile", profile),
			slog.Uint64("viewer", viewer),
			slog.Uint64("other", other),
			slog.Any("err", err))
		visible = false
	}
	worldForViewer := int32(0)
	if visible {
		worldForViewer = repo.GetWorld(other)
	}
	h.subs.send(profile, viewer, &friendspb.FriendsUpdate{
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
// All Relay* methods forward a WorldEvent to the target (profile, world)'s
// SubscribeWorldEvents subscriber. No-op if no world is subscribed for
// (req.Profile, req.TargetWorldId) (matches TS FriendServer.ts:298-302
// silent-drop on missing socketByWorld[profile][world]). No auth check
// at this layer.
//
// NAI-S5A-D-NO-ADMIN-AUTH-AT-SERVER — friends-server is dumb routing;
//   admin checks live on both sender and receiver world. Permanent.
// NAI-S5A-D-DISPATCHER-NO-ACTION — slice 5a default WorldEventsDispatcher
//   on the receiving side logs only; slice 5b retires this piecewise as
//   each opcode's world-state action is wired.

func (h *handler) RelayMute(_ context.Context, req *friendspb.RelayMuteRequest) (*emptypb.Empty, error) {
	h.worldSubs.send(req.Profile, req.TargetWorldId, &friendspb.WorldEvent{
		Event: &friendspb.WorldEvent_Mute{Mute: &friendspb.MuteEvent{
			Username37:   req.Username37,
			MutedUntilMs: req.MutedUntilMs,
		}},
	})
	return &emptypb.Empty{}, nil
}

func (h *handler) RelayKick(_ context.Context, req *friendspb.RelayKickRequest) (*emptypb.Empty, error) {
	h.worldSubs.send(req.Profile, req.TargetWorldId, &friendspb.WorldEvent{
		Event: &friendspb.WorldEvent_Kick{Kick: &friendspb.KickEvent{Username37: req.Username37}},
	})
	return &emptypb.Empty{}, nil
}

func (h *handler) RelayShutdown(_ context.Context, req *friendspb.RelayShutdownRequest) (*emptypb.Empty, error) {
	h.worldSubs.send(req.Profile, req.TargetWorldId, &friendspb.WorldEvent{
		Event: &friendspb.WorldEvent_Shutdown{Shutdown: &friendspb.ShutdownEvent{DurationTicks: req.DurationTicks}},
	})
	return &emptypb.Empty{}, nil
}

func (h *handler) RelayBroadcast(_ context.Context, req *friendspb.RelayBroadcastRequest) (*emptypb.Empty, error) {
	h.worldSubs.send(req.Profile, req.TargetWorldId, &friendspb.WorldEvent{
		Event: &friendspb.WorldEvent_Broadcast{Broadcast: &friendspb.BroadcastEvent{Message: req.Message}},
	})
	return &emptypb.Empty{}, nil
}

func (h *handler) RelayTrack(_ context.Context, req *friendspb.RelayTrackRequest) (*emptypb.Empty, error) {
	h.worldSubs.send(req.Profile, req.TargetWorldId, &friendspb.WorldEvent{
		Event: &friendspb.WorldEvent_Track{Track: &friendspb.TrackEvent{Username37: req.Username37, State: req.State}},
	})
	return &emptypb.Empty{}, nil
}

func (h *handler) RelayReload(_ context.Context, req *friendspb.RelayReloadRequest) (*emptypb.Empty, error) {
	h.worldSubs.send(req.Profile, req.TargetWorldId, &friendspb.WorldEvent{
		Event: &friendspb.WorldEvent_Reload{Reload: &friendspb.ReloadEvent{}},
	})
	return &emptypb.Empty{}, nil
}

func (h *handler) RelayClearLogins(_ context.Context, req *friendspb.RelayClearLoginsRequest) (*emptypb.Empty, error) {
	h.worldSubs.send(req.Profile, req.TargetWorldId, &friendspb.WorldEvent{
		Event: &friendspb.WorldEvent_ClearLogins{ClearLogins: &friendspb.ClearLoginsEvent{}},
	})
	return &emptypb.Empty{}, nil
}

func (h *handler) RelayClearLogouts(_ context.Context, req *friendspb.RelayClearLogoutsRequest) (*emptypb.Empty, error) {
	h.worldSubs.send(req.Profile, req.TargetWorldId, &friendspb.WorldEvent{
		Event: &friendspb.WorldEvent_ClearLogouts{ClearLogouts: &friendspb.ClearLogoutsEvent{}},
	})
	return &emptypb.Empty{}, nil
}

func (h *handler) RelayQueueScript(_ context.Context, req *friendspb.RelayQueueScriptRequest) (*emptypb.Empty, error) {
	h.worldSubs.send(req.Profile, req.TargetWorldId, &friendspb.WorldEvent{
		Event: &friendspb.WorldEvent_QueueScript{QueueScript: &friendspb.QueueScriptEvent{
			ScriptName: req.ScriptName,
			Username37: req.Username37,
		}},
	})
	return &emptypb.Empty{}, nil
}

// PublicMessage persists one row to public_chat. Mirrors TS
// FriendServer.ts:286-297 @2e3bcf43 — append-only, no delivery, no
// validation. Insert error → codes.Internal (matches slice 6
// PrivateMessage posture and FRIENDLIST/IGNORELIST mutation handlers).
//
// rev-254 A3 re-key: rows are keyed by req.SessionUuid (TS 254 inserts
// `session_uuid` directly with no account resolution). goscape keeps
// the world column from 244 and the profile scoping — TS recovers them
// via the login DB session table join, which the federated friends DB
// lacks (DB-2, db.go:21-35).
//
// Retires NAI-S6-D-PUBLIC-CHAT-DEFERRED.
func (h *handler) PublicMessage(ctx context.Context, req *friendspb.PublicMessageRequest) (*emptypb.Empty, error) {
	repo := h.repos.get(req.Profile)
	if err := repo.LogPublicMessage(ctx, req.WorldId, req.SessionUuid, req.Coord, req.Chat); err != nil {
		return nil, status.Errorf(codes.Internal, "LogPublicMessage: %v", err)
	}
	return &emptypb.Empty{}, nil
}

// SubscribeWorldEvents streams server -> world admin events for one
// (profile, world) pair. One subscriber per (profile, worldId);
// re-subscribe terminates the prior. Slice 5a opens the stream; slice 5b
// layers world-state action handlers on the world side via
// WorldEventsDispatcher.
//
// Replaces the slice-1 codes.Unimplemented stub.
func (h *handler) SubscribeWorldEvents(req *friendspb.SubscribeWorldEventsRequest, stream friendspb.FriendsService_SubscribeWorldEventsServer) error {
	sub := newWorldSubscriber(req.Profile, req.WorldId)
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
