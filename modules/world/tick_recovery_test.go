package world

import (
	"errors"
	"net"
	"sync/atomic"
	"testing"
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

// TestRecoverPlayer_NoPanic: when no panic, recoverPlayer is a no-op.
// requestLogout stays false, the conn stays open.
func TestRecoverPlayer_NoPanic(t *testing.T) {
	mc := &mockConn{}
	p := &Player{username: "alice", client: &client{conn: mc}}
	log := discardLogger()

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
	log := discardLogger()

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
	log := discardLogger()

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
	log := discardLogger()

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
	log := discardLogger()

	func() {
		defer recoverPlayer(p, "test", log)
		panic(errors.New("typed error"))
	}()

	if !p.requestLogout {
		t.Error("requestLogout: want true after error-typed panic")
	}
}

// TestRecoverNpc_NoPanic: when no panic, recoverNpc is a no-op —
// n.dead stays false, removeNpc not called.
func TestRecoverNpc_NoPanic(t *testing.T) {
	s := &Server{log: discardLogger()}
	n := &Npc{nid: 7, typeId: 42}
	log := discardLogger()

	func() {
		defer recoverNpc(n, s, "test", log)
		// no panic
	}()

	if n.dead {
		t.Error("n.dead: want false on clean run, got true")
	}
}

// TestRecoverNpc_PanicCallsRemoveNpc: a panic inside the deferred frame
// must drive s.removeNpc(n,-1) (TS World.ts:686-688). The bare Server
// + bare Npc fixture exercises removeNpc's nil-safe early-returns
// (zoneMap, rsbuf, gamemap all nil); the load-bearing assertion is that
// n.dead flips to true.
func TestRecoverNpc_PanicCallsRemoveNpc(t *testing.T) {
	s := &Server{log: discardLogger()}
	n := &Npc{nid: 7, typeId: 42}
	log := discardLogger()

	func() {
		defer recoverNpc(n, s, "test", log)
		panic("boom")
	}()

	if !n.dead {
		t.Error("n.dead: want true after panic (TS removeNpc must fire), got false")
	}
}

// TestRecoverNpc_NilNpcSafe: recovery must not panic when n is nil.
// removeNpc would nil-deref, so the helper skips it when either n or
// s is nil. The panic is still recovered (caller's loop continues).
func TestRecoverNpc_NilNpcSafe(t *testing.T) {
	s := &Server{log: discardLogger()}
	log := discardLogger()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("recoverNpc should not propagate; got: %v", r)
		}
	}()

	func() {
		defer recoverNpc(nil, s, "test", log)
		panic("boom")
	}()
}

// TestRecoverNpc_NilServerSafe: nil server must not nil-panic the helper.
// Production callers always pass non-nil; this guards the test-fixture path.
func TestRecoverNpc_NilServerSafe(t *testing.T) {
	n := &Npc{nid: 7, typeId: 42}
	log := discardLogger()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("recoverNpc should not propagate; got: %v", r)
		}
	}()

	func() {
		defer recoverNpc(n, nil, "test", log)
		panic("boom")
	}()

	// With nil server we cannot call removeNpc, so n.dead stays false —
	// the recovery logs and returns. The contract is just "no propagation".
	if n.dead {
		t.Error("n.dead: want false when server is nil (removeNpc cannot fire)")
	}
}

// TestLogWorldScriptPanic_NilStateSafe: nil state must not nil-panic the
// logger (defensive; production callers always pass non-nil). The recover
// itself now lives in fireWorldScript; panic-detection and clean-return
// coverage are in arch1_tick_recovery_test.go (TestFireWorldScript_*).
func TestLogWorldScriptPanic_NilStateSafe(t *testing.T) {
	log := discardLogger()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("logWorldScriptPanic should not nil-panic; got: %v", r)
		}
	}()

	logWorldScriptPanic(nil, "boom", log)
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
	log := discardLogger()

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
