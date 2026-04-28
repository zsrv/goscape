package world

import (
	"errors"
	"log/slog"
	"net"
	"sync/atomic"
	"testing"

	"github.com/zsrv/goscape/pkg/script"
)

// mockConn satisfies net.Conn; only Close is observable. Read/Write etc.
// will nil-panic if called — tests must not exercise them.
type mockConn struct {
	net.Conn
	closed atomic.Bool
}

func (m *mockConn) Close() error {
	m.closed.Store(true)
	return nil
}

func newDiscardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// TestRecoverPlayer_NoPanic: when no panic, recoverPlayer is a no-op.
// requestLogout stays false, the conn stays open.
func TestRecoverPlayer_NoPanic(t *testing.T) {
	mc := &mockConn{}
	p := &Player{username: "alice", client: &client{conn: mc}}
	log := newDiscardLogger()

	func() {
		defer recoverPlayer(p, "test", log)
		// no panic
	}()

	if p.requestLogout {
		t.Error("requestLogout: want false on clean run, got true")
	}
	if mc.closed.Load() {
		t.Error("conn.closed: want false on clean run, got true")
	}
}

// TestRecoverPlayer_PanicSetsLogout: a panic inside the deferred frame
// must set requestLogout = true (mirrors TS player.logout()).
func TestRecoverPlayer_PanicSetsLogout(t *testing.T) {
	mc := &mockConn{}
	p := &Player{username: "alice", client: &client{conn: mc}}
	log := newDiscardLogger()

	func() {
		defer recoverPlayer(p, "test", log)
		panic("boom")
	}()

	if !p.requestLogout {
		t.Error("requestLogout: want true after panic, got false")
	}
}

// TestRecoverPlayer_PanicClosesConn: a panic must close the player's
// client connection (mirrors TS player.client.close()).
func TestRecoverPlayer_PanicClosesConn(t *testing.T) {
	mc := &mockConn{}
	p := &Player{username: "alice", client: &client{conn: mc}}
	log := newDiscardLogger()

	func() {
		defer recoverPlayer(p, "test", log)
		panic("boom")
	}()

	if !mc.closed.Load() {
		t.Error("conn.closed: want true after panic, got false")
	}
}

// TestRecoverPlayer_NilClientSafe: recovery must not panic when
// p.client is nil (test players often have no wire connection).
func TestRecoverPlayer_NilClientSafe(t *testing.T) {
	p := &Player{username: "alice"} // client is nil
	log := newDiscardLogger()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("recoverPlayer should not propagate; got: %v", r)
		}
	}()

	func() {
		defer recoverPlayer(p, "test", log)
		panic("boom")
	}()

	if !p.requestLogout {
		t.Error("requestLogout: want true even with nil client")
	}
}

// TestRecoverPlayer_PanicWithErrorValue: panics with an error value
// must be recovered (Go panic value can be any).
func TestRecoverPlayer_PanicWithErrorValue(t *testing.T) {
	p := &Player{username: "alice"}
	log := newDiscardLogger()

	func() {
		defer recoverPlayer(p, "test", log)
		panic(errors.New("typed error"))
	}()

	if !p.requestLogout {
		t.Error("requestLogout: want true after error-typed panic")
	}
}

// TestRecoverWorldScript_NoPanic: no-op when the deferred frame
// returns normally.
func TestRecoverWorldScript_NoPanic(t *testing.T) {
	state := &script.ScriptState{Script: &script.ScriptFile{Name: "[world,demo]"}}
	log := newDiscardLogger()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("clean run should not propagate; got: %v", r)
		}
	}()

	func() {
		defer recoverWorldScript(state, log)
		// no panic
	}()
}

// TestRecoverWorldScript_PanicSwallowed: a panic during world-script
// execution must be swallowed (caller's loop continues).
func TestRecoverWorldScript_PanicSwallowed(t *testing.T) {
	state := &script.ScriptState{Script: &script.ScriptFile{Name: "[world,demo]"}}
	log := newDiscardLogger()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("recoverWorldScript should swallow; got: %v", r)
		}
	}()

	func() {
		defer recoverWorldScript(state, log)
		panic("boom")
	}()
}

// TestRecoverWorldScript_NilStateSafe: nil state must not nil-panic
// inside the recovery (defensive; production callers always pass non-nil).
func TestRecoverWorldScript_NilStateSafe(t *testing.T) {
	log := newDiscardLogger()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("recoverWorldScript should not nil-panic; got: %v", r)
		}
	}()

	func() {
		defer recoverWorldScript(nil, log)
		panic("boom")
	}()
}

// TestRecoverPlayer_ThreePlayers_OnePanics: integration-style. Three
// players in a per-player iteration; the second panics; the first and
// third must run cleanly and only the second is force-disconnected.
// Mirrors TS World.processClients per-iteration recovery scope.
func TestRecoverPlayer_ThreePlayers_OnePanics(t *testing.T) {
	mc1, mc2, mc3 := &mockConn{}, &mockConn{}, &mockConn{}
	p1 := &Player{username: "p1", client: &client{conn: mc1}}
	p2 := &Player{username: "p2", client: &client{conn: mc2}}
	p3 := &Player{username: "p3", client: &client{conn: mc3}}
	players := []*Player{p1, p2, p3}
	log := newDiscardLogger()

	var ran [3]bool
	for i, p := range players {
		func(i int, p *Player) {
			defer recoverPlayer(p, "test", log)
			ran[i] = true
			if p.username == "p2" {
				panic("boom")
			}
		}(i, p)
	}

	if !ran[0] || !ran[1] || !ran[2] {
		t.Errorf("ran: want all three reached fn body, got %v", ran)
	}
	if p1.requestLogout || p3.requestLogout {
		t.Errorf("requestLogout: want only p2, got p1=%v p3=%v",
			p1.requestLogout, p3.requestLogout)
	}
	if !p2.requestLogout {
		t.Error("requestLogout: want true for panicking player p2")
	}
	if mc1.closed.Load() || mc3.closed.Load() {
		t.Error("conn.closed: only p2's conn should close")
	}
	if !mc2.closed.Load() {
		t.Error("conn.closed: want true for p2's conn after panic")
	}
}
