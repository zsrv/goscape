package world

import (
	"bytes"
	"testing"
	"time"
)

// recordingBridges captures every bridge call into typed slices for
// per-handler assertion. Used by handler_chatsetmode_test.go,
// handler_social_list_test.go, handler_reportabuse_test.go.
type recordedFriendsCall struct {
	method           string // "AddFriend" | "RemoveFriend" | "AddIgnore" | "RemoveIgnore" | "SetChatMode"
	playerUsername   string
	targetUsername37 uint64
	privateChatMode  int // SetChatMode only
}
type recordedLoginModCall struct {
	method   string // "NotifyPlayerBan" | "NotifyPlayerMute"
	staff    string
	username string
	until    time.Time
}
type recordedLoggerCall struct {
	method   string // "NotifyPlayerReport"
	player   *Player
	offender string
	reason   string
}
type recordedInputTrackingCall struct {
	method string // "SubmitInputTracking"
	player *Player
	blob   []byte
}

type recordingBridges struct {
	friends     []recordedFriendsCall
	loginMod    []recordedLoginModCall
	logger      []recordedLoggerCall
	inputTracks []recordedInputTrackingCall // NAI-73
}

func (r *recordingBridges) AddFriend(p string, t uint64) {
	r.friends = append(r.friends, recordedFriendsCall{method: "AddFriend", playerUsername: p, targetUsername37: t})
}
func (r *recordingBridges) RemoveFriend(p string, t uint64) {
	r.friends = append(r.friends, recordedFriendsCall{method: "RemoveFriend", playerUsername: p, targetUsername37: t})
}
func (r *recordingBridges) AddIgnore(p string, t uint64) {
	r.friends = append(r.friends, recordedFriendsCall{method: "AddIgnore", playerUsername: p, targetUsername37: t})
}
func (r *recordingBridges) RemoveIgnore(p string, t uint64) {
	r.friends = append(r.friends, recordedFriendsCall{method: "RemoveIgnore", playerUsername: p, targetUsername37: t})
}
func (r *recordingBridges) SetChatMode(p string, privateChat int) {
	r.friends = append(r.friends, recordedFriendsCall{method: "SetChatMode", playerUsername: p, privateChatMode: privateChat})
}
func (r *recordingBridges) NotifyPlayerBan(staff, username string, until time.Time) {
	r.loginMod = append(r.loginMod, recordedLoginModCall{method: "NotifyPlayerBan", staff: staff, username: username, until: until})
}
func (r *recordingBridges) NotifyPlayerMute(staff, username string, until time.Time) {
	r.loginMod = append(r.loginMod, recordedLoginModCall{method: "NotifyPlayerMute", staff: staff, username: username, until: until})
}
func (r *recordingBridges) NotifyPlayerReport(player *Player, offender, reason string) {
	r.logger = append(r.logger, recordedLoggerCall{method: "NotifyPlayerReport", player: player, offender: offender, reason: reason})
}
func (r *recordingBridges) SubmitInputTracking(player *Player, blob []byte) {
	// Copy blob to defend against caller mutation.
	cp := make([]byte, len(blob))
	copy(cp, blob)
	r.inputTracks = append(r.inputTracks, recordedInputTrackingCall{method: "SubmitInputTracking", player: player, blob: cp})
}

// installRecordingBridges wires a recordingBridges into all 3 Server
// bridge fields and returns the recorder. Used by per-handler tests.
func installRecordingBridges(s *Server) *recordingBridges {
	rec := &recordingBridges{}
	s.friendsBridge = rec
	s.loginBridgeMod = rec
	s.loggerBridge = rec
	return rec
}

// Compile-time: recordingBridges and noopBridges both satisfy all 3
// interfaces. Breaks the build if any signature drifts.
var (
	_ FriendsBridge  = (*recordingBridges)(nil)
	_ LoginBridgeMod = (*recordingBridges)(nil)
	_ LoggerBridge   = (*recordingBridges)(nil)
	_ FriendsBridge  = noopBridges{}
	_ LoginBridgeMod = noopBridges{}
	_ LoggerBridge   = noopBridges{}
)

// TestNoopBridgesAllMethods exercises every noopBridges method to keep
// 100% coverage on the no-op impl (catches accidental panics in any
// future signature change).
func TestNoopBridgesAllMethods(t *testing.T) {
	var b noopBridges
	b.AddFriend("u", 1)
	b.RemoveFriend("u", 1)
	b.AddIgnore("u", 1)
	b.RemoveIgnore("u", 1)
	b.SetChatMode("u", 0)
	now := time.Now()
	b.NotifyPlayerBan("s", "u", now)
	b.NotifyPlayerMute("s", "u", now)
	b.NotifyPlayerReport(nil, "off", "REASON")
	b.SubmitInputTracking(nil, []byte{1, 2, 3})
}

// TestRecordingBridgesCapturesAllCalls exercises every recordingBridges
// method and verifies the slices grow as expected.
func TestRecordingBridgesCapturesAllCalls(t *testing.T) {
	rec := &recordingBridges{}
	rec.AddFriend("alice", 100)
	rec.RemoveFriend("alice", 101)
	rec.AddIgnore("alice", 102)
	rec.RemoveIgnore("alice", 103)
	rec.SetChatMode("alice", 1)
	if len(rec.friends) != 5 {
		t.Errorf("friends: got %d records, want 5", len(rec.friends))
	}
	if rec.friends[0].method != "AddFriend" || rec.friends[0].targetUsername37 != 100 {
		t.Errorf("AddFriend record: %+v", rec.friends[0])
	}
	if rec.friends[4].method != "SetChatMode" || rec.friends[4].privateChatMode != 1 {
		t.Errorf("SetChatMode record: %+v", rec.friends[4])
	}

	now := time.Now()
	rec.NotifyPlayerBan("auto", "evilbob", now)
	rec.NotifyPlayerMute("alice", "evilbob", now)
	if len(rec.loginMod) != 2 {
		t.Errorf("loginMod: got %d, want 2", len(rec.loginMod))
	}

	rec.NotifyPlayerReport(nil, "evilbob", "MACROING")
	if len(rec.logger) != 1 || rec.logger[0].reason != "MACROING" {
		t.Errorf("NotifyPlayerReport record: %+v", rec.logger)
	}
}

func TestRecordingBridgesCapturesSubmitInputTracking(t *testing.T) {
	rec := &recordingBridges{}
	callerBlob := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	rec.SubmitInputTracking(nil, callerBlob)
	if len(rec.inputTracks) != 1 {
		t.Fatalf("inputTracks: got %d, want 1", len(rec.inputTracks))
	}
	got := rec.inputTracks[0]
	if got.method != "SubmitInputTracking" {
		t.Errorf("method: got %q, want SubmitInputTracking", got.method)
	}
	want := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	if !bytes.Equal(got.blob, want) {
		t.Errorf("blob: got %x, want %x", got.blob, want)
	}
	// Mutation defense: mutate caller's original slice; stored copy must be unaffected.
	callerBlob[0] = 0x00
	if rec.inputTracks[0].blob[0] != 0xDE {
		t.Error("blob copy must be defensive (not aliasing caller bytes)")
	}
}
