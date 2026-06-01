package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/script"
)

// TestCanAccess_ShutdownRelaxation pins TS Player.canAccess L806-808:
// once World.shutdown is true (currentTick >= shutdownTick), the
// function returns true unconditionally — every "blocked" predicate
// (delayed, modal open, protected script) is overridden so the
// shutdown drain can complete. 2026-05-28 audit row world-tick-2.
func TestCanAccess_ShutdownRelaxation(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s

	// Stack every block predicate: delayed + modal main + protected
	// script. Under pre-fix behaviour CanAccess returns false because
	// the shutdown gate is missing.
	p.delayed = true
	p.modalState = modalStateMain
	p.activeScript = &script.ScriptState{Pointers: script.PtrProtectedActivePlayer}
	p.protect = true // NAI-111-D1: Player.protect is the TS-faithful gate, set alongside activeScript fixture

	// Sanity (pre-condition): with no shutdown scheduled, blocks fire.
	s.shutdownTick = -1
	if p.CanAccess() {
		t.Fatalf("precondition: shutdownTick=-1 → CanAccess should be false (delayed+modal+protected)")
	}

	// Pending shutdown but currentTick < shutdownTick: still blocked.
	s.currentTick = 100
	s.shutdownTick = 150
	if p.CanAccess() {
		t.Errorf("pending shutdown (currentTick=100, shutdownTick=150): CanAccess=true, want false (relaxation only at shutdownTick exhaustion)")
	}

	// Past shutdown deadline: blocks lifted (TS Player.ts:806-808).
	s.currentTick = 200
	s.shutdownTick = 150
	if !p.CanAccess() {
		t.Errorf("post-shutdown (currentTick=200, shutdownTick=150): CanAccess=false, want true (TS World.shutdown relaxation must override delayed/modal/protect)")
	}

	// Edge: currentTick == shutdownTick (TS get shutdown() uses >=).
	s.currentTick = 150
	s.shutdownTick = 150
	if !p.CanAccess() {
		t.Errorf("at-shutdown (currentTick=shutdownTick=150): CanAccess=false, want true")
	}
}

// TestCanAccess_NilServerSafe pins the nil-guard on p.client/p.client.server:
// a bare Player without a wired server (the newPlayer/newTestClient
// fixture path) must still evaluate CanAccess without nil-deref. This
// preserves TestPlayerCanAccess (server_test.go) and the broader
// fixture corpus that constructs Players without a Server.
func TestCanAccess_NilServerSafe(t *testing.T) {
	c, _ := newTestClient(t)
	p := newPlayer(c)
	// c.server is nil by construction (newClient leaves it commented-out).
	if !p.CanAccess() {
		t.Errorf("bare Player (no server): CanAccess=false, want true (nil-guard must skip shutdown check and reach the default true branch)")
	}
}
