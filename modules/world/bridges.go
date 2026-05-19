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
// (slogFriendsDispatcher, below) logs each event at Debug; the
// in-game ServerGameProt packet emit (UPDATE_FRIENDLIST /
// UPDATE_IGNORELIST / MESSAGE_PRIVATE writes to the player's client
// connection) is gated on NAI-182-D5 (the "social cluster"
// ServerGameProt deferral noted at tick.go:226).
//
// NAI-S4A-D-NO-INGAME-PACKET-EMIT — retires when NAI-182-D5 retires
// and the dispatcher is wired through to player.write(...).
type FriendsDispatcher interface {
	OnFriendlistUpdate(viewer uint64, entries []*friendspb.FriendEntry)
	OnIgnorelistUpdate(viewer uint64, ignored []uint64)
	OnPrivateMessage(target uint64, from uint64, staffLvl int32, pmId uint32, chat string)
}

// slogFriendsDispatcher is the default FriendsDispatcher. Logs each
// event at Debug; does NOT emit ServerGameProt packets to the player.
// See NAI-S4A-D-NO-INGAME-PACKET-EMIT above.
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

func (d *slogFriendsDispatcher) OnPrivateMessage(target uint64, from uint64, staffLvl int32, pmId uint32, chat string) {
	d.log.Debug("friends dispatch: private message",
		slog.Uint64("target", target),
		slog.Uint64("from", from),
		slog.Uint64("pm_id", uint64(pmId)))
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
func (noopBridges) NotifyPlayerBan(string, string, time.Time)  {}
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
