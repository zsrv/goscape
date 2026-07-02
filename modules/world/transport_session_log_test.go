package world

import (
	"errors"
	"io"
	"net"
	"testing"
)

// fakeNetTimeoutErr satisfies net.Error with Timeout()==true so the
// disconnect-classifier can be exercised without a real syscall.
type fakeNetTimeoutErr struct{}

func (fakeNetTimeoutErr) Error() string   { return "i/o timeout" }
func (fakeNetTimeoutErr) Timeout() bool   { return true }
func (fakeNetTimeoutErr) Temporary() bool { return true }

// TestDisconnectSessionLogEvent_ClassifiesByErrType pins the TS
// TcpServer.ts:44-67 close/error/timeout split: EOF or net.ErrClosed
// → "TCP socket closed"; net.Error{Timeout:true} → "TCP socket
// timeout"; anything else → "TCP socket error" with err.Error() as
// the extra arg.
func TestDisconnectSessionLogEvent_ClassifiesByErrType(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantMsg  string
		wantArgs []string
	}{
		{"EOF", io.EOF, "TCP socket closed", nil},
		{"ErrClosed", net.ErrClosed, "TCP socket closed", nil},
		{"timeout", fakeNetTimeoutErr{}, "TCP socket timeout", nil},
		{"generic", errors.New("boom"), "TCP socket error", []string{"boom"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg, args := disconnectSessionLogEvent(tc.err)
			if msg != tc.wantMsg {
				t.Errorf("message: got %q, want %q", msg, tc.wantMsg)
			}
			if len(args) != len(tc.wantArgs) {
				t.Fatalf("args len: got %d (%v), want %d (%v)", len(args), args, len(tc.wantArgs), tc.wantArgs)
			}
			for i := range args {
				if args[i] != tc.wantArgs[i] {
					t.Errorf("args[%d]: got %q, want %q", i, args[i], tc.wantArgs[i])
				}
			}
		})
	}
}

// TestRemovePlayerOnTick_EmitsLoggedOutSessionLog pins TS
// World.removePlayer (World.ts:1606): every graceful logout pushes a
// MODERATOR session log "Logged out" before flush/cleanup. The log
// must be enqueued on Server.sessionLogs so the per-tick dispatch can
// flush it via loggerBridge.
func TestRemovePlayerOnTick_EmitsLoggedOutSessionLog(t *testing.T) {
	s := newTestServer(t)
	_, invTypes := newTestPlayerForLoadSave(t)
	s.invTypes = invTypes

	c, _ := newTestClient(t)
	c.server = s
	p := newPlayer(c)
	p.session = "sess-logout"
	p.username = "alice"
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	s.removePlayerOnTick(p)

	s.sessionLogsMu.Lock()
	defer s.sessionLogsMu.Unlock()
	var found bool
	for _, lg := range s.sessionLogs {
		if lg.EventType == LoggerEventTypeModerator && lg.Event == "Logged out" {
			if lg.SessionUUID != "sess-logout" {
				t.Errorf("SessionUUID: got %q, want sess-logout", lg.SessionUUID)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no MODERATOR \"Logged out\" session log emitted (TS World.ts:1606 / logger-transport-4); got %d logs: %+v",
			len(s.sessionLogs), s.sessionLogs)
	}
}

// TestRemovePlayerOnDisconnect_EmitsLoggedOutSessionLog confirms the
// disconnect path (which enqueues removePlayerOnTick on the relay queue)
// also produces the "Logged out" log once drained on-tick. logger-transport-4
// fires from removePlayerOnTick, so this is parity with the graceful path.
func TestRemovePlayerOnDisconnect_EmitsLoggedOutSessionLog(t *testing.T) {
	s := newTestServer(t)
	_, invTypes := newTestPlayerForLoadSave(t)
	s.invTypes = invTypes

	c, _ := newTestClient(t)
	c.server = s
	p := newPlayer(c)
	p.session = "sess-disc"
	p.username = "bob"
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	s.removePlayerOnDisconnect(p)
	s.drainRemovals()

	s.sessionLogsMu.Lock()
	defer s.sessionLogsMu.Unlock()
	var found bool
	for _, lg := range s.sessionLogs {
		if lg.EventType == LoggerEventTypeModerator && lg.Event == "Logged out" && lg.SessionUUID == "sess-disc" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no MODERATOR \"Logged out\" session log emitted on disconnect path; got %d logs: %+v",
			len(s.sessionLogs), s.sessionLogs)
	}
}

// Compile-time guard: errors.As(_, &netErr) in disconnectSessionLogEvent
// requires fakeNetTimeoutErr to satisfy net.Error; assert it here so the
// "TCP socket timeout" subtest fails loudly at compile time if the
// interface contract drifts.
var _ net.Error = fakeNetTimeoutErr{}
