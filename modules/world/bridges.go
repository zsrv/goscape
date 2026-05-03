package world

import "time"

// FriendsBridge mirrors TS World.friendThread.postMessage(...) for
// social-list mutations and chat-mode propagation. Real impl is a
// future friends-server module (see NAI-72-D-FRIENDS-SERVER-BRIDGE).
type FriendsBridge interface {
	AddFriend(playerUsername string, target uint64)
	RemoveFriend(playerUsername string, target uint64)
	AddIgnore(playerUsername string, target uint64)
	RemoveIgnore(playerUsername string, target uint64)
	SetChatMode(playerUsername string, privateChat int)
}

// LoginBridgeMod mirrors TS World.loginThread.postMessage('player_ban'/
// 'player_mute', ...). The existing LoginClient is auth-only; this is a
// separate moderation channel. Real impl deferred via
// NAI-72-D-LOGIN-SERVER-BRIDGE-MOD.
type LoginBridgeMod interface {
	NotifyPlayerBan(staff, username string, until time.Time)
	NotifyPlayerMute(staff, username string, until time.Time)
}

// LoggerBridge mirrors TS World.loggerThread.postMessage('report', ...).
// Real impl deferred via NAI-72-D-LOGGER-BRIDGE. The same closure path
// will activate the EventTracking handler.
type LoggerBridge interface {
	// NotifyPlayerReport posts an abuse report. reason is the string label
	// of the ReportAbuseReason enum value (e.g. "MACROING").
	NotifyPlayerReport(player *Player, offender, reason string)
}

// noopBridges is the default impl wired into NewServer. Records nothing,
// performs no I/O.
type noopBridges struct{}

func (noopBridges) AddFriend(string, uint64)                  {}
func (noopBridges) RemoveFriend(string, uint64)               {}
func (noopBridges) AddIgnore(string, uint64)                  {}
func (noopBridges) RemoveIgnore(string, uint64)               {}
func (noopBridges) SetChatMode(string, int)                   {}
func (noopBridges) NotifyPlayerBan(string, string, time.Time) {}
func (noopBridges) NotifyPlayerMute(string, string, time.Time) {}
func (noopBridges) NotifyPlayerReport(*Player, string, string) {}
