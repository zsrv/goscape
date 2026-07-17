package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/gamemap"
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

	p := setupInfoPlayer(t, s, 1, 50, 50, 0)
	// Reserve the matching rsbuf player slot so ComputePlayer's
	// pid bounds-check writes to a real entry instead of no-op.
	s.rsbuf.AddPlayer(int32(p.slot))

	// Drive processInfo (the function containing the new ComputePlayer
	// push). Should not panic.
	s.processInfo()
	s.processInfo()

	// Cross-zone move + another processInfo run. Update lastTick* so any
	// zone-change-aware logic (NAI-30 retired the grid Add/Remove block)
	// runs cleanly alongside the rsbuf push.
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

func TestProcessCleanup_RsbufCleanupSmoke(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	// Drive the end-of-tick cleanup. Should not panic on the new
	// s.rsbuf.Cleanup() call.
	s.processCleanup()
	s.processCleanup()
	// Smoke: rsbuf queries post-cleanup must not panic.
	if got := s.rsbuf.GetNpcObservers(0); got != 0 {
		t.Errorf("post-cleanup GetNpcObservers: got %d, want 0", got)
	}
}

// TestProcessInfo_PassesRealOrientationFields — NAI-30 Bundle 1 Task 1.5.
// Pins that the OrientationX/Z field values set on modules/world.Player
// propagate through tick.go's ComputePlayer call into b.players[pid]
// after s.processInfo(). Closes the loop on T1.3 (field plumbing).
func TestProcessInfo_PassesRealOrientationFields(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	s.renderer = rsbuf.NewRenderer()

	p := setupInfoPlayer(t, s, 1, 50, 50, 0)
	s.rsbuf.AddPlayer(int32(p.slot))

	p.OrientationX = 1234
	p.OrientationZ = 5678

	s.processInfo()

	rp := s.rsbuf.PlayerForTest(int32(p.slot))
	if rp == nil {
		t.Fatal("rsbuf player slot not populated after processInfo")
	}
	if rp.OrientationX != 1234 {
		t.Errorf("OrientationX: got %d, want 1234", rp.OrientationX)
	}
	if rp.OrientationZ != 5678 {
		t.Errorf("OrientationZ: got %d, want 5678", rp.OrientationZ)
	}
}

// TestProcessInfo_PassesRealLastAppearance — NAI-30 Bundle 1 Task 1.5.
// Pins that lastAppearance set on modules/world.Player propagates
// through tick.go's ComputePlayer call into b.players[pid].LastAppearance.
// Closes the loop on T1.4 (field plumbing + producer).
func TestProcessInfo_PassesRealLastAppearance(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	s.renderer = rsbuf.NewRenderer()

	p := setupInfoPlayer(t, s, 1, 50, 50, 0)
	s.rsbuf.AddPlayer(int32(p.slot))

	p.lastAppearance = 42

	s.processInfo()

	rp := s.rsbuf.PlayerForTest(int32(p.slot))
	if rp == nil {
		t.Fatal("rsbuf player slot not populated after processInfo")
	}
	if rp.LastAppearance != 42 {
		t.Errorf("LastAppearance: got %d, want 42", rp.LastAppearance)
	}
}

// TestProcessInfoDoesNotReorientPlayer pins TS World.ts @4c95f87e
// (e31a8719): processInfo no longer reorients players — that moved to
// player.reorient(), called from processPlayerReorient in the tick loop
// (after processInteractions, before processEnergy). A player with a
// *Loc target and cached targetX/Z + stepsTaken == 0 keeps that state
// untouched across processInfo's rsbuf ComputePlayers pass. Uses a valid
// slot (slot=1) and initialises s.renderer so ComputePlayers actually
// executes rather than being short-circuited by slot<1.
func TestProcessInfoDoesNotReorientPlayer(t *testing.T) {
	s := newTestServer(t)
	s.renderer = rsbuf.NewRenderer()

	// setupInfoPlayer wires slot=1, active=true, adds to s.rsbuf and
	// s.players — ComputePlayers will execute for this slot.
	p := setupInfoPlayer(t, s, 1, 100, 100, 0)

	loc := entitypkg.NewLoc(0, 105, 108, 1, 1, entitypkg.LifecycleForever, 0, 10, 0)
	p.target = loc
	p.targetX = 999
	p.targetZ = 1001
	p.stepsTaken = 0
	preFaceX, preFaceZ := p.faceAngleX, p.faceAngleZ

	s.processInfo()

	if p.targetX != 999 {
		t.Errorf("targetX: got %d, want 999 (unchanged — processInfo must not reorient)", p.targetX)
	}
	if p.targetZ != 1001 {
		t.Errorf("targetZ: got %d, want 1001 (unchanged — processInfo must not reorient)", p.targetZ)
	}
	if p.faceAngleX != preFaceX {
		t.Errorf("faceAngleX: got %d, want %d (unchanged — processInfo must not reorient)", p.faceAngleX, preFaceX)
	}
	if p.faceAngleZ != preFaceZ {
		t.Errorf("faceAngleZ: got %d, want %d (unchanged — processInfo must not reorient)", p.faceAngleZ, preFaceZ)
	}
}

// TestProcessInfoDoesNotReorientNpc pins TS World.ts:1045 @4c95f87e
// (e31a8719): processInfo no longer reorients npcs — that moved to
// Npc.turn() (reorientEntity/reorient, npc_ai.go). An NPC with a *Loc
// target, cached targetX/Z, and stepsTaken == 0 keeps that state
// untouched across processInfo's rsbuf ComputeNpcs pass.
func TestProcessInfoDoesNotReorientNpc(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	s.renderer = rsbuf.NewRenderer()

	// rev-274 processInfo gate (World.ts:979-981): the info pass is skipped
	// when the world is empty. This test exercises the NPC-side path, so it
	// must have at least one player online.
	_ = setupInfoPlayer(t, s, 1, 50, 52, 0)

	n := makeInteractionNpc(t, s, 1, 50, 50, 0)

	loc := entitypkg.NewLoc(0, 55, 58, 1, 1, entitypkg.LifecycleForever, 0, 10, 0)
	n.target = loc
	n.targetX = 888
	n.targetZ = 777
	n.stepsTaken = 0
	preFaceX, preFaceZ := n.faceAngleX, n.faceAngleZ

	s.processInfo()

	if n.targetX != 888 {
		t.Errorf("npc targetX: got %d, want 888 (unchanged — processInfo must not reorient)", n.targetX)
	}
	if n.targetZ != 777 {
		t.Errorf("npc targetZ: got %d, want 777 (unchanged — processInfo must not reorient)", n.targetZ)
	}
	if n.faceAngleX != preFaceX {
		t.Errorf("npc faceAngleX: got %d, want %d (unchanged — processInfo must not reorient)", n.faceAngleX, preFaceX)
	}
	if n.faceAngleZ != preFaceZ {
		t.Errorf("npc faceAngleZ: got %d, want %d (unchanged — processInfo must not reorient)", n.faceAngleZ, preFaceZ)
	}
}
