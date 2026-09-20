package world

import (
	"io"
	"sync"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/tapper"
	jstring "github.com/zsrv/goscape/pkg/util/jstring"
)

// recordingTapper is the test double for the pkg/tapper seam: it keeps every
// packet it is handed and every close reason SessionEnded reports, so a test
// can assert what a consumer would actually see.
type recordingTapper struct {
	mu      sync.Mutex
	packets []tappedPacket
	reasons []string
}

type tappedPacket struct {
	dir     tapper.Direction
	opcode  uint8
	payload []byte
}

func (r *recordingTapper) Enabled() bool                           { return true }
func (r *recordingTapper) SessionStarted(int64, string, time.Time) {}

func (r *recordingTapper) Tap(_ int64, _ string, dir tapper.Direction, opcode uint8, payload []byte, _ time.Time) {
	// payload aliases a live connection buffer, so copy it exactly as a real
	// implementation must (the Tapper contract in pkg/tapper).
	r.mu.Lock()
	r.packets = append(r.packets, tappedPacket{dir: dir, opcode: opcode, payload: append([]byte(nil), payload...)})
	r.mu.Unlock()
}

func (r *recordingTapper) SessionEnded(_ int64, _ string, _ time.Time, closeReason string) {
	r.mu.Lock()
	r.reasons = append(r.reasons, closeReason)
	r.mu.Unlock()
}

func (r *recordingTapper) closeReasons() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.reasons...)
}

func (r *recordingTapper) tapped() []tappedPacket {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]tappedPacket(nil), r.packets...)
}

// attachTap wires a recording tapper onto c and gives it the session id the
// teardown path requires before it reports anything.
func attachTap(c *client) *recordingTapper {
	tp := &recordingTapper{}
	c.tap = tp
	c.sessionID = "11111111-2222-3333-4444-555555555555"
	return tp
}

// wantCloseReason drives the teardown report and asserts the single reason the
// tapper was handed.
func wantCloseReason(t *testing.T, c *client, tp *recordingTapper, want string) {
	t.Helper()
	c.reportSessionEnded(time.Now())
	got := tp.closeReasons()
	if len(got) != 1 {
		t.Fatalf("SessionEnded called %d times, want exactly 1", len(got))
	}
	if got[0] != want {
		t.Fatalf("close reason = %q, want %q", got[0], want)
	}
}

// TestSessionCloseReason_DefaultsToDisconnect pins the fallback: a socket that
// errors or EOFs with no server-side decision behind it is a disconnect, which
// is what every session used to report unconditionally.
func TestSessionCloseReason_DefaultsToDisconnect(t *testing.T) {
	p, _ := newTestPlayer(t)
	tp := attachTap(p.client)
	wantCloseReason(t, p.client, tp, tapper.CloseReasonDisconnect)
}

// TestSessionCloseReason_ClientRequestedLogout covers P_LOGOUT: the script
// opcode a player's own logout button drives.
func TestSessionCloseReason_ClientRequestedLogout(t *testing.T) {
	p, _ := newTestPlayer(t)
	tp := attachTap(p.client)

	p.RequestLogout()

	wantCloseReason(t, p.client, tp, tapper.CloseReasonLogout)
}

// TestSessionCloseReason_IdleTimerOpcode covers the client's own IDLE_TIMER
// signal, sent after 4500 idle input cycles.
func TestSessionCloseReason_IdleTimerOpcode(t *testing.T) {
	s := newTestServer(t)
	p := registerActivePlayer(t, s, "bob", 1)
	tp := attachTap(p.client)

	if err := handleIdleTimer(p, nil); err != nil {
		t.Fatalf("handleIdleTimer: %v", err)
	}
	if !p.requestIdleLogout {
		t.Fatal("preflight: handleIdleTimer did not flag the idle logout")
	}

	wantCloseReason(t, p.client, tp, tapper.CloseReasonTimeout)
}

// TestSessionCloseReason_NoResponseTimeout covers processLogouts' forcing
// branch: the player stopped answering for timeoutNoResponse ticks.
func TestSessionCloseReason_NoResponseTimeout(t *testing.T) {
	s := newTestServer(t)
	p := registerActivePlayer(t, s, "bob", 1)
	tp := attachTap(p.client)

	s.currentTick = timeoutNoResponse
	p.lastResponse = 0
	p.lastConnected = s.currentTick
	s.processLogouts()

	wantCloseReason(t, p.client, tp, tapper.CloseReasonTimeout)
}

// TestSessionCloseReason_NoConnectionTimeout covers processLogouts' idle
// branch: nothing has arrived for timeoutNoConnection ticks.
func TestSessionCloseReason_NoConnectionTimeout(t *testing.T) {
	s := newTestServer(t)
	p := registerActivePlayer(t, s, "bob", 1)
	tp := attachTap(p.client)

	s.currentTick = timeoutNoConnection
	p.lastResponse = s.currentTick
	p.lastConnected = 0
	s.processLogouts()

	wantCloseReason(t, p.client, tp, tapper.CloseReasonTimeout)
}

// TestSessionCloseReason_Kick covers the administrative relay (RELAY_KICK),
// which shares its teardown with the ::kick staff command.
func TestSessionCloseReason_Kick(t *testing.T) {
	s := newTestServer(t)
	p := registerActivePlayer(t, s, "bob", 1)
	tp := attachTap(p.client)

	var ops WorldStateOps = s
	ops.KickPlayer(jstring.ToBase37("bob"))
	s.drainRelayActions()

	if !p.loggingOut {
		t.Fatal("preflight: KickPlayer did not flag the logout")
	}
	wantCloseReason(t, p.client, tp, tapper.CloseReasonKick)
}

// TestSessionCloseReason_WorldShutdown covers processShutdown evicting every
// connected player when the world stops or reboots.
func TestSessionCloseReason_WorldShutdown(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	p.slot = 1
	s.players.set(1, p)
	go io.Copy(io.Discard, cc)
	tp := attachTap(p.client)

	s.shutdownTick = s.currentTick
	s.processShutdown()

	wantCloseReason(t, p.client, tp, tapper.CloseReasonShutdown)
}

// TestSessionCloseReason_RecoveredPanic covers recoverPlayer, the one path
// that genuinely recovers a panic and tears the connection down to contain it.
func TestSessionCloseReason_RecoveredPanic(t *testing.T) {
	p, _ := newTestPlayer(t)
	tp := attachTap(p.client)

	func() {
		defer recoverPlayer(p, "test", discardLogger())
		panic("boom")
	}()

	wantCloseReason(t, p.client, tp, tapper.CloseReasonCrash)
}

// TestSessionCloseReason_ProtocolViolation covers readPacket rejecting an
// opcode that is absent from this revision's table.
func TestSessionCloseReason_ProtocolViolation(t *testing.T) {
	enc, dec := isaacPair([4]uint32{5, 6, 7, 8})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec
	tp := attachTap(p.client)

	p.client.in.Write([]byte{encryptOpcode(enc, 3)})
	if _, _, _, err := p.readPacket(); err == nil {
		t.Fatal("preflight: readPacket accepted an unknown opcode")
	}

	wantCloseReason(t, p.client, tp, tapper.CloseReasonProtocol)
}

// TestSessionCloseReason_FirstReasonWins pins the write-once rule: the
// decision that STARTED the teardown explains it, and the socket error that
// follows as the peer goes away must not overwrite it.
func TestSessionCloseReason_FirstReasonWins(t *testing.T) {
	p, _ := newTestPlayer(t)
	tp := attachTap(p.client)

	p.setCloseReason(closeReasonCodeKick)
	p.setCloseReason(closeReasonCodeTimeout)
	p.client.setCloseReason(closeReasonCodeLogout)

	wantCloseReason(t, p.client, tp, tapper.CloseReasonKick)
}

// TestSessionCloseReason_ReportIsIdempotent pins that the teardown reports a
// session exactly once: handleTCPConn's defer clears sessionID afterwards, and
// nothing downstream should see a second end for the same session.
func TestSessionCloseReason_ReportIsIdempotent(t *testing.T) {
	p, _ := newTestPlayer(t)
	tp := attachTap(p.client)

	p.client.reportSessionEnded(time.Now())
	p.client.reportSessionEnded(time.Now())

	if got := len(tp.closeReasons()); got != 1 {
		t.Fatalf("SessionEnded called %d times, want exactly 1", got)
	}
}
