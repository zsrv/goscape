package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/grid"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

// Smoke tests for NAI-29 Bundle 4 Task 4.4 — ComputePlayer per-tick
// state push in tick.go's processInfo. Verifies the per-tick path
// runs without panic across multiple tick iterations + cross-zone
// moves. Implementation correctness verified by visual diff review
// of the wiring point at processInfo's end.
func TestProcessInfo_ComputePlayerPushSmoke(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	s.renderer = rsbuf.NewRenderer()
	s.grid = grid.New()

	p := setupInfoPlayer(t, s, 1, 50, 50, 0)
	// Reserve the matching rsbuf player slot so ComputePlayer's
	// pid bounds-check writes to a real entry instead of no-op.
	s.rsbuf.AddPlayer(int32(p.slot))

	// Drive processInfo (the function containing the new ComputePlayer
	// push). Should not panic.
	s.processInfo()
	s.processInfo()

	// Cross-zone move + another processInfo run. Update lastTick* so the
	// pre-existing grid Add/Remove block exercises its zone-change branch
	// cleanly alongside the new rsbuf push.
	p.lastTickX, p.lastTickZ, p.lastLevel = p.x, p.z, p.level
	p.x = 64
	s.processInfo()

	// Move back.
	p.lastTickX, p.lastTickZ, p.lastLevel = p.x, p.z, p.level
	p.x = 50
	s.processInfo()
}

// Smoke test for NAI-29 Bundle 4 Task 4.5 — ComputeNpc per-tick state
// push in tick.go's processInfo. Verifies the per-tick npc loop runs
// without panic across multiple tick iterations. Implementation
// correctness verified by visual diff review of the wiring point.
func TestProcessInfo_ComputeNpcPushSmoke(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	s.renderer = rsbuf.NewRenderer()
	s.grid = grid.New()

	n := newTestNpc(50)
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}

	// Drive processInfo (the function containing the new ComputeNpc push).
	// Should not panic across multiple ticks.
	s.processInfo()
	s.processInfo()
	s.processInfo()

	// Smoke: post-tick rsbuf queries must not panic. No players were
	// added, so the npc has no observers.
	if got := s.rsbuf.GetNpcObservers(int32(n.nid)); got != 0 {
		t.Errorf("after 3 processInfo: GetNpcObservers got %d, want 0", got)
	}
}
