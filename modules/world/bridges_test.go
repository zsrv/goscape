package world

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/loginpb"
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
type recordedPrivateMessageCall struct {
	method         string // "PrivateMessage"
	playerUsername string
	staffLvl       int32
	pmId           uint32
	target         uint64
	message        string
	coord          int
}
type recordedPublicMessageCall struct {
	method      string // "PublicMessage"
	sessionUUID string
	coord       int
	message     string
}

type recordingBridges struct {
	friends              []recordedFriendsCall
	loginMod             []recordedLoginModCall
	logger               []recordedLoggerCall
	inputTracks          []recordedInputTrackingCall  // NAI-73
	submittedSessionLogs [][]SessionLog               // NAI-74 — one element per tick flush
	privateMsgs          []recordedPrivateMessageCall // NAI-158
	publicMsgs           []recordedPublicMessageCall  // public_chat follow-up
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
func (r *recordingBridges) PrivateMessage(p string, staffLvl int32, pmId uint32, target uint64, message string, coord int) {
	r.privateMsgs = append(r.privateMsgs, recordedPrivateMessageCall{
		method: "PrivateMessage", playerUsername: p, staffLvl: staffLvl,
		pmId: pmId, target: target, message: message, coord: coord,
	})
}
func (r *recordingBridges) PublicMessage(sessionUUID string, coord int, message string) {
	r.publicMsgs = append(r.publicMsgs, recordedPublicMessageCall{
		method: "PublicMessage", sessionUUID: sessionUUID, coord: coord, message: message,
	})
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
func (r *recordingBridges) SubmitSessionLogs(logs []SessionLog) {
	// Snapshot: defends against caller mutation between the call and assertion.
	snap := make([]SessionLog, len(logs))
	copy(snap, logs)
	r.submittedSessionLogs = append(r.submittedSessionLogs, snap)
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
	_ FriendsBridge     = (*recordingBridges)(nil)
	_ LoginBridgeMod    = (*recordingBridges)(nil)
	_ LoggerBridge      = (*recordingBridges)(nil)
	_ FriendsBridge     = noopBridges{}
	_ LoginBridgeMod    = noopBridges{}
	_ LoggerBridge      = noopBridges{}
	_ FriendsDispatcher = noopBridges{}
	_ FriendsBridge     = (*grpcFriendsBridge)(nil)
)

// TestNoopBridgesAllMethods exercises every noopBridges method to keep
// 100% coverage on the no-op impl (catches accidental panics in any
// future signature change).
func TestNoopBridgesAllMethods(t *testing.T) {
	var b noopBridges
	b.OnFriendlistUpdate(1, nil)
	b.OnIgnorelistUpdate(1, nil)
	b.OnPrivateMessage(1, 2, 0, 0, "")
	b.AddFriend("u", 1)
	b.RemoveFriend("u", 1)
	b.AddIgnore("u", 1)
	b.RemoveIgnore("u", 1)
	b.SetChatMode("u", 0)
	b.PrivateMessage("u", 0, 0, 1, "x", 0)
	b.PublicMessage("uuid", 0, "msg")
	now := time.Now()
	b.NotifyPlayerBan("s", "u", now)
	b.NotifyPlayerMute("s", "u", now)
	b.NotifyPlayerReport(nil, "off", "REASON")
	b.SubmitInputTracking(nil, []byte{1, 2, 3})
	b.SubmitSessionLogs(nil)
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

// TestRecordingBridgesCapturesPrivateMessage pins the NAI-158
// PrivateMessage capture: every arg is recorded verbatim.
func TestRecordingBridgesCapturesPrivateMessage(t *testing.T) {
	rec := &recordingBridges{}
	rec.PrivateMessage("alice", 2, 0xDEADBEEF, 1234, "hi bob", 0xC0DE)
	if len(rec.privateMsgs) != 1 {
		t.Fatalf("privateMsgs: got %d, want 1", len(rec.privateMsgs))
	}
	got := rec.privateMsgs[0]
	if got.method != "PrivateMessage" || got.playerUsername != "alice" ||
		got.staffLvl != 2 || got.pmId != 0xDEADBEEF || got.target != 1234 ||
		got.message != "hi bob" || got.coord != 0xC0DE {
		t.Errorf("PrivateMessage record: %+v", got)
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

// TestNoopBridgesSubmitSessionLogs exercises the noop SubmitSessionLogs.
func TestNoopBridgesSubmitSessionLogs(t *testing.T) {
	var b noopBridges
	b.SubmitSessionLogs([]SessionLog{{SessionUUID: "x"}})
	// Must not panic; nothing else to assert.
}

// TestRecordingBridgesCapturesSubmitSessionLogs verifies snapshot semantics.
func TestRecordingBridgesCapturesSubmitSessionLogs(t *testing.T) {
	rec := &recordingBridges{}
	caller := []SessionLog{
		{SessionUUID: "alice", Timestamp: 1000, Coord: 50, Event: "hi", EventType: LoggerEventTypeModerator},
		{SessionUUID: "bob", Timestamp: 2000, Coord: 60, Event: "ho", EventType: LoggerEventTypeEngine},
	}
	rec.SubmitSessionLogs(caller)
	if len(rec.submittedSessionLogs) != 1 {
		t.Fatalf("submittedSessionLogs: got %d batches, want 1", len(rec.submittedSessionLogs))
	}
	got := rec.submittedSessionLogs[0]
	if len(got) != 2 || got[0].SessionUUID != "alice" || got[1].SessionUUID != "bob" {
		t.Errorf("batch contents: %+v", got)
	}
	// Mutation defense: mutate caller's slice; recorded snapshot must be unaffected.
	caller[0].SessionUUID = "MUTATED"
	if rec.submittedSessionLogs[0][0].SessionUUID != "alice" {
		t.Error("snapshot must not alias caller slice")
	}
}

func TestLoginGRPCBridgeMod_NotifyPlayerBan_FiresRPC(t *testing.T) {
	fake := newFakeLoginClient()
	bridge := &loginGRPCBridgeMod{client: fake, log: discardLogger()}

	until := time.Unix(1747569600, 0)
	bridge.NotifyPlayerBan("alice", "evilbob", until)

	select {
	case got := <-fake.playerBanReqs:
		if got.Staff != "alice" {
			t.Errorf("Staff: got %q, want alice", got.Staff)
		}
		if got.Username != "evilbob" {
			t.Errorf("Username: got %q, want evilbob", got.Username)
		}
		if !got.Until.AsTime().Equal(until) {
			t.Errorf("Until: got %v, want %v", got.Until.AsTime(), until)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for PlayerBan RPC")
	}
}

func TestLoginGRPCBridgeMod_NotifyPlayerMute_FiresRPC(t *testing.T) {
	fake := newFakeLoginClient()
	bridge := &loginGRPCBridgeMod{client: fake, log: discardLogger()}

	until := time.Unix(1747569600, 0)
	bridge.NotifyPlayerMute("alice", "evilbob", until)

	select {
	case got := <-fake.playerMuteReqs:
		if got.Staff != "alice" {
			t.Errorf("Staff: got %q, want alice", got.Staff)
		}
		if got.Username != "evilbob" {
			t.Errorf("Username: got %q, want evilbob", got.Username)
		}
		if !got.Until.AsTime().Equal(until) {
			t.Errorf("Until: got %v, want %v", got.Until.AsTime(), until)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for PlayerMute RPC")
	}
}

// gatedLoginClient is a one-off fake whose PlayerBan blocks on <-gate
// before recording. Used to verify the bridge's go-fan-out: the
// synchronous NotifyPlayerBan call must return before the underlying
// RPC completes.
type gatedLoginClient struct {
	*fakeLoginClient
	gate chan struct{}
	hit  chan struct{}
}

func (g *gatedLoginClient) PlayerBan(ctx context.Context, req *loginpb.PlayerBanRequest) {
	<-g.gate
	g.fakeLoginClient.PlayerBan(ctx, req)
	close(g.hit)
}

func TestLoginGRPCBridgeMod_FireAndForget_DoesNotBlock(t *testing.T) {
	gate := make(chan struct{})
	gated := &gatedLoginClient{
		fakeLoginClient: newFakeLoginClient(),
		gate:            gate,
		hit:             make(chan struct{}),
	}
	bridge := &loginGRPCBridgeMod{client: gated, log: discardLogger()}

	done := make(chan struct{})
	go func() {
		bridge.NotifyPlayerBan("alice", "evilbob", time.Now())
		close(done)
	}()

	select {
	case <-done:
		// expected: synchronous call returned before gate opened
	case <-time.After(100 * time.Millisecond):
		t.Fatal("NotifyPlayerBan blocked on RPC despite go-fan-out")
	}

	close(gate)

	select {
	case <-gated.hit:
		// expected: after gate, underlying PlayerBan completed
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for gated PlayerBan to fire")
	}
}

func TestDefaultLoginBridgeMod_NonNilClient_ReturnsGRPCBridge(t *testing.T) {
	got := defaultLoginBridgeMod(newFakeLoginClient(), discardLogger())
	if _, ok := got.(*loginGRPCBridgeMod); !ok {
		t.Fatalf("defaultLoginBridgeMod: got %T, want *loginGRPCBridgeMod", got)
	}
}

func TestDefaultLoginBridgeMod_NilClient_ReturnsNoop(t *testing.T) {
	got := defaultLoginBridgeMod(nil, discardLogger())
	if _, ok := got.(noopBridges); !ok {
		t.Fatalf("defaultLoginBridgeMod: got %T, want noopBridges", got)
	}
}
