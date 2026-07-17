package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

// (a) TestNpcReorientEntityPathingTargetPlayer pins TS
// PathingEntity.reorientEntity (PathingEntity.ts:364-369 @4c95f87e) on the
// Npc side: target is *Player → focus on fine, client=false. No face-coord
// mask; targetX/Z untouched.
func TestNpcReorientEntityPathingTargetPlayer(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	other, wait := makeInteractionPlayer(t, s, 110, 120, 0)
	defer wait()

	npc.target = other
	npc.faceSquareX, npc.faceSquareZ = -1, -1
	npc.masks = 0

	npc.reorientEntity()

	if npc.faceAngleX != coordgrid.Fine(110, 1) {
		t.Errorf("faceAngleX: got %d, want %d", npc.faceAngleX, coordgrid.Fine(110, 1))
	}
	if npc.faceAngleZ != coordgrid.Fine(120, 1) {
		t.Errorf("faceAngleZ: got %d, want %d", npc.faceAngleZ, coordgrid.Fine(120, 1))
	}
	if npc.faceSquareX != -1 || npc.faceSquareZ != -1 {
		t.Errorf("faceSquare: got (%d,%d), want (-1,-1) (client=false)", npc.faceSquareX, npc.faceSquareZ)
	}
	if npc.masks&rsbuf.NpcMaskFaceCoord != 0 {
		t.Errorf("masks: NpcMaskFaceCoord bit set, want unset (client=false)")
	}
	if npc.targetX != -1 || npc.targetZ != -1 {
		t.Errorf("targetX/Z: got (%d,%d), want (-1,-1) (unchanged for PathingEntity branch)", npc.targetX, npc.targetZ)
	}
}

// (a) TestNpcReorientEntityPathingTargetNpc — symmetric, *Npc target.
// makeInteractionNpc builds with typ.Size=0, so other.size defaults to 0.
// We set other.size = 1 to match the real Npc.New default for typ.Size=1.
func TestNpcReorientEntityPathingTargetNpc(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	other := makeInteractionNpc(t, s, 2, 105, 108, 0)
	other.size = 1 // matches Npc.New default for typ.Size=1

	npc.target = other

	npc.reorientEntity()

	if npc.faceAngleX != coordgrid.Fine(105, 1) {
		t.Errorf("faceAngleX: got %d, want %d", npc.faceAngleX, coordgrid.Fine(105, 1))
	}
	if npc.faceAngleZ != coordgrid.Fine(108, 1) {
		t.Errorf("faceAngleZ: got %d, want %d", npc.faceAngleZ, coordgrid.Fine(108, 1))
	}
}

// (a) TestNpcReorientEntityPathingTargetNpcSize2 pins the size>1 path on
// the NPC side. Mirrors TestPlayerReorientEntityPathingTargetNpcSize2.
// Without the Fix 1 change Fine(t.x, 1) would be returned instead of
// Fine(t.x, 2).
func TestNpcReorientEntityPathingTargetNpcSize2(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	other := makeInteractionNpc(t, s, 2, 105, 108, 0)
	other.size = 2 // size-2 target: Fine(x, 2) = x*2+2

	npc.target = other

	npc.reorientEntity()

	if npc.faceAngleX != coordgrid.Fine(105, 2) {
		t.Errorf("faceAngleX: got %d, want %d (Fine(105,2))", npc.faceAngleX, coordgrid.Fine(105, 2))
	}
	if npc.faceAngleZ != coordgrid.Fine(108, 2) {
		t.Errorf("faceAngleZ: got %d, want %d (Fine(108,2))", npc.faceAngleZ, coordgrid.Fine(108, 2))
	}
}

// (a) TestNpcReorientEntityNonPathingTargetNoop pins that reorientEntity
// no-ops for a Loc/Obj (non-pathing) target on the Npc side too.
func TestNpcReorientEntityNonPathingTargetNoop(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)

	loc := entitypkg.NewLoc(0, 105, 108, 1, 1, entitypkg.LifecycleForever, 0, 10, 0)
	npc.target = loc
	preFaceX, preFaceZ := npc.faceAngleX, npc.faceAngleZ

	npc.reorientEntity()

	if npc.faceAngleX != preFaceX || npc.faceAngleZ != preFaceZ {
		t.Errorf("faceAngle changed on non-pathing target: got (%d,%d), want (%d,%d)", npc.faceAngleX, npc.faceAngleZ, preFaceX, preFaceZ)
	}
}

// (b) TestNpcReorientLocTargetStepsZero pins TS PathingEntity.reorient
// (PathingEntity.ts:377-388 @4c95f87e) on the Npc side: focus + clear on
// the default branch when stepsTaken == 0 and targetX != -1, now shipping
// the face-coord mask (client=true).
func TestNpcReorientLocTargetStepsZero(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)

	loc := entitypkg.NewLoc(0, 105, 108, 1, 1, entitypkg.LifecycleForever, 0, 10, 0)
	npc.target = loc
	npc.targetX = 999
	npc.targetZ = 1001
	npc.stepsTaken = 0
	npc.faceSquareX, npc.faceSquareZ = -1, -1
	npc.masks = 0

	npc.reorient()

	if npc.faceAngleX != 999 {
		t.Errorf("faceAngleX: got %d, want 999", npc.faceAngleX)
	}
	if npc.faceAngleZ != 1001 {
		t.Errorf("faceAngleZ: got %d, want 1001", npc.faceAngleZ)
	}
	if npc.faceSquareX != 999 {
		t.Errorf("faceSquareX: got %d, want 999 (client=true now writes faceSquare)", npc.faceSquareX)
	}
	if npc.faceSquareZ != 1001 {
		t.Errorf("faceSquareZ: got %d, want 1001 (client=true now writes faceSquare)", npc.faceSquareZ)
	}
	if npc.masks&rsbuf.NpcMaskFaceCoord == 0 {
		t.Errorf("masks: NpcMaskFaceCoord bit not set (masks=%d) — reorient() must ship client=true", npc.masks)
	}
	if npc.targetX != -1 {
		t.Errorf("targetX: got %d, want -1 (cleared after focus)", npc.targetX)
	}
	if npc.targetZ != -1 {
		t.Errorf("targetZ: got %d, want -1 (cleared after focus)", npc.targetZ)
	}
}

// (c) TestNpcReorientLocTargetStepsNonzero pins the early-out.
func TestNpcReorientLocTargetStepsNonzero(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)

	loc := entitypkg.NewLoc(0, 105, 108, 1, 1, entitypkg.LifecycleForever, 0, 10, 0)
	npc.target = loc
	npc.targetX = 999
	npc.targetZ = 1001
	npc.stepsTaken = 3
	preFaceX := npc.faceAngleX
	preFaceZ := npc.faceAngleZ

	npc.reorient()

	if npc.faceAngleX != preFaceX {
		t.Errorf("faceAngleX: got %d, want %d (unchanged; stepsTaken>0 early-out)", npc.faceAngleX, preFaceX)
	}
	if npc.faceAngleZ != preFaceZ {
		t.Errorf("faceAngleZ: got %d, want %d", npc.faceAngleZ, preFaceZ)
	}
	if npc.targetX != 999 || npc.targetZ != 1001 {
		t.Errorf("targetX/Z: got (%d,%d), want (999,1001) (unchanged)", npc.targetX, npc.targetZ)
	}
}

// (d) TestNpcReorientPathingTargetEarlyReturn pins TS
// PathingEntity.reorient's guard (PathingEntity.ts:378-380 @4c95f87e) on
// the Npc side: reorient() early-returns for a pathing target even when
// targetX/Z are stale and non-(-1).
func TestNpcReorientPathingTargetEarlyReturn(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	other, wait := makeInteractionPlayer(t, s, 110, 120, 0)
	defer wait()

	npc.target = other
	npc.targetX = 999
	npc.targetZ = 1001
	npc.stepsTaken = 0
	preFaceX, preFaceZ := npc.faceAngleX, npc.faceAngleZ

	npc.reorient()

	if npc.faceAngleX != preFaceX || npc.faceAngleZ != preFaceZ {
		t.Errorf("faceAngle changed on pathing target: got (%d,%d), want (%d,%d) (early-return)", npc.faceAngleX, npc.faceAngleZ, preFaceX, preFaceZ)
	}
	if npc.targetX != 999 || npc.targetZ != 1001 {
		t.Errorf("targetX/Z: got (%d,%d), want (999,1001) (unchanged by early-return)", npc.targetX, npc.targetZ)
	}
}

// TestNpcReorientNilTarget pins the no-op.
func TestNpcReorientNilTarget(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)

	npc.target = nil
	npc.targetX = -1
	npc.targetZ = -1
	preFaceX := npc.faceAngleX
	preFaceZ := npc.faceAngleZ

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("reorient panicked on nil target: %v", r)
		}
	}()

	npc.reorient()

	if npc.faceAngleX != preFaceX || npc.faceAngleZ != preFaceZ {
		t.Errorf("faceAngle changed on nil target: got (%d,%d), want (%d,%d)", npc.faceAngleX, npc.faceAngleZ, preFaceX, preFaceZ)
	}
}

// (e) TestNpcSetInteractionNoLongerFocuses pins TS
// PathingEntity.setInteraction @4c95f87e on the Npc side: setInteraction
// no longer calls focus() at all — facing only changes during the
// entity's own turn (reorientEntity/reorient, from Npc.turn). A loc
// target with kind=InteractionEngine used to focus(instant=true)
// immediately (pre-e31a8719); now faceSquareX/Z and NpcMaskFaceCoord are
// UNCHANGED, faceAngle is UNCHANGED, and targetX/Z are still recorded
// (NonPathingEntity cache consumed later by reorient()). Mirrors
// TestPlayerSetInteractionNoLongerFocuses (player_reorient_test.go).
func TestNpcSetInteractionNoLongerFocuses(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	npc.faceSquareX, npc.faceSquareZ = -1, -1
	npc.masks = 0
	preFaceX, preFaceZ := npc.faceAngleX, npc.faceAngleZ

	loc := entitypkg.NewLoc(0, 105, 108, 1, 1, entitypkg.LifecycleForever, 0, 10, 0)

	ok := npc.SetInteraction(InteractionEngine, loc, 1, -1)
	if !ok {
		t.Fatal("SetInteraction returned false")
	}

	if npc.faceAngleX != preFaceX || npc.faceAngleZ != preFaceZ {
		t.Errorf("faceAngle: got (%d,%d), want unchanged (%d,%d) — SetInteraction must not focus", npc.faceAngleX, npc.faceAngleZ, preFaceX, preFaceZ)
	}
	if npc.faceSquareX != -1 || npc.faceSquareZ != -1 {
		t.Errorf("faceSquare: got (%d,%d), want (-1,-1) — SetInteraction must not write faceSquare", npc.faceSquareX, npc.faceSquareZ)
	}
	if npc.masks&rsbuf.NpcMaskFaceCoord != 0 {
		t.Errorf("masks: NpcMaskFaceCoord bit set, want unset — SetInteraction must not OR the mask")
	}
	wantTX := coordgrid.Fine(105, 1)
	wantTZ := coordgrid.Fine(108, 1)
	if npc.targetX != wantTX || npc.targetZ != wantTZ {
		t.Errorf("targetX/Z: got (%d,%d), want (%d,%d) — still recorded for reorient() to consume", npc.targetX, npc.targetZ, wantTX, wantTZ)
	}
}

// (f) TestProcessInfoDoesNotReorientNpcWithoutTurn pins TS World.ts
// @4c95f87e: processInfo no longer reorients players or npcs — facing
// (reorientEntity/reorient) runs in Npc.turn(), not in processInfo. An
// npc that takes no turn this tick (e.g. delayed/dead-gated, or simply
// not reached this tick) keeps its spawn orientation even though it
// holds a live target. Runs processInfo() alone, with no npc.turn() call.
func TestProcessInfoDoesNotReorientNpcWithoutTurn(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	if err := s.gamemap.Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	s.renderer = rsbuf.NewRenderer()

	// processInfo's 0-player gate (World.ts:979-981) requires a player
	// online; use setupInfoPlayer (full 3a/3b scaffolding) so
	// buildArea.rebuildNormal() has what it needs.
	_ = setupInfoPlayer(t, s, 1, 50, 52, 0)

	npc := makeInteractionNpc(t, s, 1, 50, 50, 0)
	npc.unfocus() // spawn orientation
	spawnFaceX, spawnFaceZ := npc.faceAngleX, npc.faceAngleZ

	loc := entitypkg.NewLoc(0, 55, 58, 1, 1, entitypkg.LifecycleForever, 0, 10, 0)
	npc.target = loc
	npc.targetX = 888
	npc.targetZ = 777
	npc.stepsTaken = 0

	s.processInfo() // no npc.turn() call — the npc took no turn this tick

	if npc.faceAngleX != spawnFaceX || npc.faceAngleZ != spawnFaceZ {
		t.Errorf("faceAngle: got (%d,%d), want spawn orientation (%d,%d) unchanged — processInfo must not reorient",
			npc.faceAngleX, npc.faceAngleZ, spawnFaceX, spawnFaceZ)
	}
	if npc.targetX != 888 || npc.targetZ != 777 {
		t.Errorf("targetX/Z: got (%d,%d), want (888,777) unchanged — processInfo must not clear the cache", npc.targetX, npc.targetZ)
	}
}
