package world

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zsrv/goscape/pkg/loginpb"
)

// FriendsBridge mirrors TS World.friendThread.postMessage(...) for
// social-list mutations and chat-mode propagation. Real impl is a
// future friends-server module (see NAI-72-D-FRIENDS-SERVER-BRIDGE).
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
	// Real impl deferred via NAI-72-D-FRIENDS-SERVER-BRIDGE.
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
