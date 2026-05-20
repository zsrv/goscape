package world

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zsrv/goscape/pkg/friendspb"
	"github.com/zsrv/goscape/pkg/loginpb"
	jstring "github.com/zsrv/goscape/pkg/util/jstring"
)

// FriendsBridge mirrors TS World.friendThread.postMessage(...) for
// social-list mutations and chat-mode propagation. Real impl bridges
// to the modules/friends gRPC service (slice 1). Production impl is
// grpcFriendsBridge (this file); wired by NewServer via
// defaultFriendsBridge.
type FriendsBridge interface {
	AddFriend(playerUsername string, target uint64)
	RemoveFriend(playerUsername string, target uint64)
	AddIgnore(playerUsername string, target uint64)
	RemoveIgnore(playerUsername string, target uint64)
	SetChatMode(playerUsername string, privateChat int)

	// PrivateMessage posts a /tell-style private chat message to the
	// friends-server. Mirrors TS World.sendPrivateMessage payload
	// (World.ts:1631-1643): {username, staffLvl, pmId, target, message,
	// coord}. coord is the packed coordgrid.PackCoord value.
	// Real impl: grpcFriendsBridge.PrivateMessage (this file) fans a
	// friendspb.PrivateMessageRequest out to the friends server.
	PrivateMessage(playerUsername string, staffLvl int32, pmId uint32, target uint64, message string, coord int)

	// PublicMessage audit-logs a public-chat utterance to the friends
	// server. sessionUUID is the per-login UUID (Player.session,
	// populated by slice 7). coord is the packed coordgrid.PackCoord
	// value at utterance. message is the WordPack-decoded text (not
	// the raw word-packed bytes — see handleMessagePublic for the
	// decode site). Real impl: grpcFriendsBridge.PublicMessage (this
	// file) fans a friendspb.PublicMessageRequest out to the friends
	// server. Mirrors TS World.sendPublicMessageLog inline payload
	// (FriendServer.ts:670-694 publicMessage emitter).
	PublicMessage(sessionUUID string, coord int, message string)
}

// LoginBridgeMod mirrors TS World.loginThread.postMessage('player_ban'/
// 'player_mute', ...). Production impl is loginGRPCBridgeMod (below),
// which delegates to LoginClient.PlayerBan / .PlayerMute.
type LoginBridgeMod interface {
	NotifyPlayerBan(staff, username string, until time.Time)
	NotifyPlayerMute(staff, username string, until time.Time)
}

// LoggerBridge is the structured-log sink for engine analytics events.
// Mirrors TS World.loggerThread.postMessage(...) for the 'report' and
// 'input_track' channels. Default impl is slogLoggerBridge (see
// logger_bridge.go); tests bind a recordingBridges capture impl.
type LoggerBridge interface {
	// NotifyPlayerReport posts an abuse report (TS World.notifyPlayerReport
	// at World.ts:2297-2313, channel 'report'). reason is the string label
	// of the ReportAbuseReason enum value (e.g. "MACROING").
	NotifyPlayerReport(player *Player, offender, reason string)

	// SubmitInputTracking posts a per-player input-recording blob from the
	// anti-cheat tracking subsystem (TS World.submitInputTracking at
	// World.ts:2314-2321, channel 'input_track'). blob is the raw bytes
	// from the EVENT_TRACKING client packet.
	SubmitInputTracking(player *Player, blob []byte)

	// SubmitSessionLogs posts the per-tick batch of session-log entries.
	// Mirrors TS LoggerThread 'session_log' channel (LoggerThread.ts:31-37,
	// dispatched from World.cycle at World.ts:435-442). Called once per
	// tick by Server.processSessionLogs when the buffer is non-empty.
	SubmitSessionLogs(logs []SessionLog)
}

// FriendsDispatcher is the world-side sink for server -> world friends
// updates received over the SubscribeUpdates stream. Production impl
// is emitFriendsDispatcher (NAI-182-D5, 2026-05-19) which enqueues the
// real ServerGameProt packet emit on the tick-goroutine via
// s.relayActionQueue. slogFriendsDispatcher remains as a debug-only
// fallback for null-friends-server / test paths.
//
// Retired tags:
//   NAI-S4A-D-NO-INGAME-PACKET-EMIT — RETIRED 2026-05-19 (NAI-182-D5).
//     OnFriendlistUpdate / OnIgnorelistUpdate now emit UPDATE_FRIENDLIST /
//     UPDATE_IGNORELIST to the recipient's wire.
//   NAI-S4B-D-NO-INGAME-PM-EMIT — RETIRED 2026-05-19 (NAI-182-D5).
//     OnPrivateMessage now emits MESSAGE_PRIVATE to the recipient's wire.
type FriendsDispatcher interface {
	OnFriendlistUpdate(viewer uint64, entries []*friendspb.FriendEntry)
	OnIgnorelistUpdate(viewer uint64, ignored []uint64)
	OnPrivateMessage(target uint64, from uint64, staffLvl int32, pmId uint32, chat string)
}

// slogFriendsDispatcher is the debug-only fallback FriendsDispatcher.
// Logs each event at Debug; does NOT emit ServerGameProt packets to
// the player. Production binds emitFriendsDispatcher instead — see
// FriendsDispatcher interface doc-comment above.
type slogFriendsDispatcher struct {
	log *slog.Logger
}

func newSlogFriendsDispatcher(log *slog.Logger) FriendsDispatcher {
	return &slogFriendsDispatcher{log: log}
}

func (d *slogFriendsDispatcher) OnFriendlistUpdate(viewer uint64, entries []*friendspb.FriendEntry) {
	d.log.Debug("friends dispatch: friendlist update",
		slog.Uint64("viewer", viewer),
		slog.Int("entries", len(entries)))
}

func (d *slogFriendsDispatcher) OnIgnorelistUpdate(viewer uint64, ignored []uint64) {
	d.log.Debug("friends dispatch: ignorelist update",
		slog.Uint64("viewer", viewer),
		slog.Int("ignored", len(ignored)))
}

// OnPrivateMessage logs the inbound PM at Debug. This is the fallback
// impl; production binds emitFriendsDispatcher (see FriendsDispatcher
// interface doc-comment) which writes the real MESSAGE_PRIVATE packet
// to the recipient via the tick-goroutine relayActionQueue.
func (d *slogFriendsDispatcher) OnPrivateMessage(target uint64, from uint64, staffLvl int32, pmId uint32, chat string) {
	d.log.Debug("friends dispatch: private message",
		slog.Uint64("target", target),
		slog.Uint64("from", from),
		slog.Uint64("pm_id", uint64(pmId)))
}

// emitFriendsDispatcher is the production FriendsDispatcher. Each
// method enqueues a closure on s.relayActionQueue so the writeOut on
// the resolved Player runs on the tick goroutine (the only goroutine
// allowed to touch Player.client.bufw + ISAAC stream). The recipient
// is resolved inside the closure (not at enqueue time) so a player who
// logs out between event arrival and tick-drain is correctly skipped.
//
// slogFriendsDispatcher remains the default fallback for tests and
// when friends-server is disabled — that path never reaches a real
// Player.
//
// Retires NAI-S4A-D-NO-INGAME-PACKET-EMIT / NAI-S4B-D-NO-INGAME-PM-EMIT
// (NAI-182-D5, 2026-05-19).
type emitFriendsDispatcher struct {
	s   *Server
	log *slog.Logger
}

func newEmitFriendsDispatcher(s *Server, log *slog.Logger) FriendsDispatcher {
	return &emitFriendsDispatcher{s: s, log: log}
}

func (d *emitFriendsDispatcher) OnFriendlistUpdate(viewer uint64, entries []*friendspb.FriendEntry) {
	d.log.Debug("friends dispatch: friendlist update",
		slog.Uint64("viewer", viewer),
		slog.Int("entries", len(entries)))
	d.s.enqueueRelayAction(func() {
		p := d.s.lookupPlayerByUsername37(viewer)
		if p == nil {
			return
		}
		for _, e := range entries {
			sendUpdateFriendList(p, e.Username37, int(e.WorldId))
		}
	})
}

func (d *emitFriendsDispatcher) OnIgnorelistUpdate(viewer uint64, ignored []uint64) {
	d.log.Debug("friends dispatch: ignorelist update",
		slog.Uint64("viewer", viewer),
		slog.Int("ignored", len(ignored)))
	d.s.enqueueRelayAction(func() {
		p := d.s.lookupPlayerByUsername37(viewer)
		if p == nil {
			return
		}
		sendUpdateIgnoreList(p, ignored)
	})
}

func (d *emitFriendsDispatcher) OnPrivateMessage(target uint64, from uint64, staffLvl int32, pmId uint32, chat string) {
	d.log.Debug("friends dispatch: private message",
		slog.Uint64("target", target),
		slog.Uint64("from", from),
		slog.Uint64("pm_id", uint64(pmId)))
	d.s.enqueueRelayAction(func() {
		p := d.s.lookupPlayerByUsername37(target)
		if p == nil {
			return
		}
		sendMessagePrivate(p, from, pmId, staffLvl, chat)
	})
}

var _ FriendsDispatcher = (*emitFriendsDispatcher)(nil)

// FriendsAdminBridge mirrors TS World.friendThread.postMessage(...) for
// cross-world RELAY_* admin commands (slice 5a). Production impl is
// grpcFriendsAdminBridge (modules/world/admin_bridge.go); wired by
// NewServer via defaultFriendsAdminBridge. When FriendsClient is nil
// (friends-server disabled), the bridge resolves to noopAdminBridge{}.
//
// The bridge is the surface that admin-action code paths use to issue
// cross-world commands. Slice 5a exposes the surface; slice 5b layers
// dispatcher actions on the receiving side. Admin chat-command wiring
// (::kick, ::mute, etc.) is future integration work — slice 5 does not
// touch existing cheat handlers.
type FriendsAdminBridge interface {
	Mute(targetWorldID int32, username37 uint64, mutedUntilMs int64)
	Kick(targetWorldID int32, username37 uint64)
	Shutdown(targetWorldID int32, durationTicks int32)
	Broadcast(targetWorldID int32, message string)
	Track(targetWorldID int32, username37 uint64, state int32)
	Reload(targetWorldID int32)
	ClearLogins(targetWorldID int32)
	ClearLogouts(targetWorldID int32)
	QueueScript(targetWorldID int32, scriptName string, username37 uint64)
}

// WorldEventsDispatcher is the world-side sink for inbound RELAY_*
// admin events received over the SubscribeWorldEvents stream (slice 5a).
//
// Default no-effects impl: slogWorldEventsDispatcher (this file) — logs
// each event at Info.
//
// Production impl: actionWorldEventsDispatcher (world_events_dispatcher.go,
// slice 5b) — composes the slog impl with WorldStateOps so each event
// also applies its world-state effect on the tick goroutine.
//
// NAI-S5A-D-DISPATCHER-NO-ACTION — RETIRED 2026-05-20 (slice 5b):
// eight wired opcodes (MUTE, KICK, SHUTDOWN, BROADCAST, TRACK, RELOAD,
// CLEARLOGINS, CLEARLOGOUTS-tagged-noop) apply real effects.
// QUEUESCRIPT remains slog-warn only; tracked separately by
// NAI-S5B-D-NO-RUNESCRIPT-RUNTIME on actionWorldEventsDispatcher.OnQueueScript.
//
// Slice 5b opens these new tags:
//
// NAI-S5B-D-CLEARLOGOUTS-NO-GOSCAPE-QUEUE — permanent (architectural
//   divergence from TS; goscape has no logout-request queue). See
//   (*Server).ClearLogouts in world_state_ops.go.
//
// NAI-S5B-D-NO-RUNESCRIPT-RUNTIME — retires when the runescript runtime
//   can resolve [queue,<name>] triggers by name and enqueue on a
//   player. See actionWorldEventsDispatcher.OnQueueScript.
type WorldEventsDispatcher interface {
	OnMute(username37 uint64, mutedUntilMs int64)
	OnKick(username37 uint64)
	OnShutdown(durationTicks int32)
	OnBroadcast(message string)
	OnTrack(username37 uint64, state int32)
	OnReload()
	OnClearLogins()
	OnClearLogouts()
	OnQueueScript(scriptName string, username37 uint64)
}

// WorldStateOps is the world-side action surface invoked by
// actionWorldEventsDispatcher on inbound RELAY_* events. *Server
// implements it (world_state_ops.go). Tests bind recordingWorldStateOps.
//
// Methods correspond 1:1 to wired RELAY_* opcodes. QUEUESCRIPT is NOT
// on this interface — it stays slog-warn behind
// NAI-S5B-D-NO-RUNESCRIPT-RUNTIME until the runscript runtime can
// resolve [queue,<name>] triggers.
//
// All methods are safe to call from any goroutine. Production *Server
// impls enqueue a closure on relayActionQueue and return immediately;
// the tick goroutine drains the queue at the top of each iteration
// (see Server.drainRelayActions in world_state_ops.go).
//
// Plan deviation: the spec/plan named the shutdown/reload methods
// `Shutdown` and `Reload`. Both names already exist on *Server
// (Shutdown() = full TCP teardown; Reload(clearInvs bool) error =
// content reload). To avoid Go method-name collision, both methods
// here carry the `Relay` prefix matching the originating RPC opcode.
// The other six methods kept their plan-named identifiers (no
// existing-*Server conflict).
type WorldStateOps interface {
	SetPlayerMute(username37 uint64, mutedUntilMs int64)
	KickPlayer(username37 uint64)
	RelayShutdown(durationTicks int32)
	BroadcastMessage(message string)
	SetPlayerInputTracking(username37 uint64, state int32)
	RelayReload()
	ClearLogins()
	// ClearLogouts is a tagged no-op: goscape has no logout-request
	// queue analogous to TS's World.logoutRequests. See
	// NAI-S5B-D-CLEARLOGOUTS-NO-GOSCAPE-QUEUE (slice 5b spec §6.4).
	ClearLogouts()
}

// slogWorldEventsDispatcher is the default WorldEventsDispatcher. Logs
// each event at Info; does NOT apply world-state effects. See
// NAI-S5A-D-DISPATCHER-NO-ACTION above.
type slogWorldEventsDispatcher struct {
	log *slog.Logger
}

func newSlogWorldEventsDispatcher(log *slog.Logger) WorldEventsDispatcher {
	return &slogWorldEventsDispatcher{log: log}
}

func (d *slogWorldEventsDispatcher) OnMute(username37 uint64, mutedUntilMs int64) {
	d.log.Info("world event: mute",
		slog.Uint64("username37", username37),
		slog.Int64("muted_until_ms", mutedUntilMs))
}

func (d *slogWorldEventsDispatcher) OnKick(username37 uint64) {
	d.log.Info("world event: kick", slog.Uint64("username37", username37))
}

func (d *slogWorldEventsDispatcher) OnShutdown(durationTicks int32) {
	d.log.Info("world event: shutdown", slog.Int("duration_ticks", int(durationTicks)))
}

func (d *slogWorldEventsDispatcher) OnBroadcast(message string) {
	d.log.Info("world event: broadcast", slog.String("message", message))
}

func (d *slogWorldEventsDispatcher) OnTrack(username37 uint64, state int32) {
	d.log.Info("world event: track",
		slog.Uint64("username37", username37),
		slog.Int("state", int(state)))
}

func (d *slogWorldEventsDispatcher) OnReload() {
	d.log.Info("world event: reload")
}

func (d *slogWorldEventsDispatcher) OnClearLogins() {
	d.log.Info("world event: clear_logins")
}

func (d *slogWorldEventsDispatcher) OnClearLogouts() {
	d.log.Info("world event: clear_logouts")
}

func (d *slogWorldEventsDispatcher) OnQueueScript(scriptName string, username37 uint64) {
	d.log.Info("world event: queue_script",
		slog.String("script_name", scriptName),
		slog.Uint64("username37", username37))
}

// noopBridges is the default impl wired into NewServer. Records nothing,
// performs no I/O.
type noopBridges struct{}

func (noopBridges) AddFriend(string, uint64)                   {}
func (noopBridges) RemoveFriend(string, uint64)                {}
func (noopBridges) AddIgnore(string, uint64)                   {}
func (noopBridges) RemoveIgnore(string, uint64)                {}
func (noopBridges) SetChatMode(string, int)                    {}
func (noopBridges) PrivateMessage(string, int32, uint32, uint64, string, int) {}
func (noopBridges) PublicMessage(string, int, string)                         {}
func (noopBridges) NotifyPlayerBan(string, string, time.Time)                 {}
func (noopBridges) NotifyPlayerMute(string, string, time.Time) {}
func (noopBridges) NotifyPlayerReport(*Player, string, string) {}
func (noopBridges) SubmitInputTracking(*Player, []byte)        {}
func (noopBridges) SubmitSessionLogs([]SessionLog)             {}
func (noopBridges) OnFriendlistUpdate(uint64, []*friendspb.FriendEntry)          {}
func (noopBridges) OnIgnorelistUpdate(uint64, []uint64)                          {}
func (noopBridges) OnPrivateMessage(uint64, uint64, int32, uint32, string)       {}

// loginGRPCBridgeMod is the production LoginBridgeMod impl. Translates
// moderation actions into gRPC RPCs against the login server. Calls are
// fired in a goroutine so packet handlers and the tick loop never block
// on network I/O — mirrors the goroutine fan-out used by autosave
// (server.go:1018) and force-logout (server.go:982). Callers must
// compute the absolute deadline (time.Now().Add(d)) before invocation;
// the bridge does not coerce zero times.
type loginGRPCBridgeMod struct {
	client LoginClient
	log    *slog.Logger
}

func (b *loginGRPCBridgeMod) NotifyPlayerBan(staff, username string, until time.Time) {
	go b.client.PlayerBan(context.Background(), &loginpb.PlayerBanRequest{
		Staff:    staff,
		Username: username,
		Until:    timestamppb.New(until),
	})
}

func (b *loginGRPCBridgeMod) NotifyPlayerMute(staff, username string, until time.Time) {
	go b.client.PlayerMute(context.Background(), &loginpb.PlayerMuteRequest{
		Staff:    staff,
		Username: username,
		Until:    timestamppb.New(until),
	})
}

var _ LoginBridgeMod = (*loginGRPCBridgeMod)(nil)

// defaultLoginBridgeMod returns the production LoginBridgeMod for the
// given LoginClient: a goroutine-fanout gRPC adapter when client != nil,
// otherwise noopBridges{}. Called from NewServer; broken out for
// testability without spinning up the full Server.
func defaultLoginBridgeMod(client LoginClient, log *slog.Logger) LoginBridgeMod {
	if client != nil {
		return &loginGRPCBridgeMod{client: client, log: log}
	}
	return noopBridges{}
}

// grpcFriendsBridge is the production FriendsBridge impl. Translates
// social-list mutations / chat-mode propagation / private-message
// posting into gRPC RPCs against the friends server. Each call is
// fired in a goroutine so packet handlers and the tick loop never
// block on network I/O — mirrors loginGRPCBridgeMod's fan-out pattern.
// worldID is captured at construction time from cfg.NodeID.
type grpcFriendsBridge struct {
	client  FriendsClient
	worldID int32
	log     *slog.Logger
}

func (b *grpcFriendsBridge) AddFriend(playerUsername string, target uint64) {
	go b.client.FriendlistAdd(context.Background(), &friendspb.FriendlistAddRequest{
		WorldId:          b.worldID,
		Username37:       jstring.ToBase37(playerUsername),
		TargetUsername37: target,
	})
}

func (b *grpcFriendsBridge) RemoveFriend(playerUsername string, target uint64) {
	go b.client.FriendlistDel(context.Background(), &friendspb.FriendlistDelRequest{
		WorldId:          b.worldID,
		Username37:       jstring.ToBase37(playerUsername),
		TargetUsername37: target,
	})
}

func (b *grpcFriendsBridge) AddIgnore(playerUsername string, target uint64) {
	go b.client.IgnorelistAdd(context.Background(), &friendspb.IgnorelistAddRequest{
		WorldId:          b.worldID,
		Username37:       jstring.ToBase37(playerUsername),
		TargetUsername37: target,
	})
}

func (b *grpcFriendsBridge) RemoveIgnore(playerUsername string, target uint64) {
	go b.client.IgnorelistDel(context.Background(), &friendspb.IgnorelistDelRequest{
		WorldId:          b.worldID,
		Username37:       jstring.ToBase37(playerUsername),
		TargetUsername37: target,
	})
}

func (b *grpcFriendsBridge) SetChatMode(playerUsername string, privateChat int) {
	go b.client.ChatSetMode(context.Background(), &friendspb.ChatSetModeRequest{
		WorldId:     b.worldID,
		Username37:  jstring.ToBase37(playerUsername),
		PrivateChat: int32(privateChat),
	})
}

func (b *grpcFriendsBridge) PrivateMessage(playerUsername string, staffLvl int32, pmId uint32, target uint64, message string, coord int) {
	go b.client.PrivateMessage(context.Background(), &friendspb.PrivateMessageRequest{
		WorldId:          b.worldID,
		Username37:       jstring.ToBase37(playerUsername),
		TargetUsername37: target,
		StaffLvl:         staffLvl,
		PmId:             pmId,
		Chat:             message,
		Coord:            int32(coord),
	})
}

func (b *grpcFriendsBridge) PublicMessage(sessionUUID string, coord int, message string) {
	go b.client.PublicMessage(context.Background(), &friendspb.PublicMessageRequest{
		WorldId:     b.worldID,
		SessionUuid: sessionUUID,
		Coord:       int32(coord),
		Chat:        message,
	})
}

var _ FriendsBridge = (*grpcFriendsBridge)(nil)

// defaultFriendsBridge returns the production FriendsBridge for the
// given FriendsClient + worldID: a goroutine-fanout gRPC adapter when
// client != nil, otherwise noopBridges{}. Called from NewServer; broken
// out for testability without spinning up the full Server.
func defaultFriendsBridge(client FriendsClient, worldID int32, log *slog.Logger) FriendsBridge {
	if client != nil {
		return &grpcFriendsBridge{client: client, worldID: worldID, log: log}
	}
	return noopBridges{}
}
