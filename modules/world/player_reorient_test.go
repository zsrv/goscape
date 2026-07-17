package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

// (a) TestPlayerReorientEntityPathingTargetPlayer pins TS
// PathingEntity.reorientEntity (PathingEntity.ts:364-369 @4c95f87e):
// target is *Player → focus(fine(t.x, 1), fine(t.z, 1), client=false).
// No face-coord mask; targetX/Z untouched (reorientEntity never reads
// or writes them).
func TestPlayerReorientEntityPathingTargetPlayer(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	other, otherWait := makeInteractionPlayer(t, s, 110, 120, 0)
	defer otherWait()

	p.target = other
	p.faceSquareX, p.faceSquareZ = -1, -1
	p.masks = 0

	p.reorientEntity()

	if p.faceAngleX != coordgrid.Fine(110, 1) {
		t.Errorf("faceAngleX: got %d, want %d", p.faceAngleX, coordgrid.Fine(110, 1))
	}
	if p.faceAngleZ != coordgrid.Fine(120, 1) {
		t.Errorf("faceAngleZ: got %d, want %d", p.faceAngleZ, coordgrid.Fine(120, 1))
	}
	if p.faceSquareX != -1 || p.faceSquareZ != -1 {
		t.Errorf("faceSquare: got (%d,%d), want (-1,-1) (client=false, no faceSquare write)", p.faceSquareX, p.faceSquareZ)
	}
	if p.masks&rsbuf.MaskFaceCoord != 0 {
		t.Errorf("masks: MaskFaceCoord bit set, want unset (client=false)")
	}
	if p.targetX != -1 || p.targetZ != -1 {
		t.Errorf("targetX/Z: got (%d,%d), want (-1,-1) (unchanged for PathingEntity branch)", p.targetX, p.targetZ)
	}
}

// (a) TestPlayerReorientEntityPathingTargetNpc — symmetric to above, *Npc
// target. makeInteractionNpc builds with typ.Size=0, so npc.size defaults
// to 0. We set npc.size = 1 to match the real Npc.New default (NpcType.Size
// defaults to 1 in production configs, NpcType.RegisterFlagsAndApplyDefaults
// line ~310). Fine(x, 1) is the expected centre for a size-1 NPC.
func TestPlayerReorientEntityPathingTargetNpc(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	npc := makeInteractionNpc(t, s, 1, 105, 108, 0)
	npc.size = 1 // matches Npc.New default for typ.Size=1

	p.target = npc

	p.reorientEntity()

	if p.faceAngleX != coordgrid.Fine(105, 1) {
		t.Errorf("faceAngleX: got %d, want %d", p.faceAngleX, coordgrid.Fine(105, 1))
	}
	if p.faceAngleZ != coordgrid.Fine(108, 1) {
		t.Errorf("faceAngleZ: got %d, want %d", p.faceAngleZ, coordgrid.Fine(108, 1))
	}
}

// (a) TestPlayerReorientEntityPathingTargetNpcSize2 pins the size>1 path.
// Without the Fix 1 production change Fine(t.x, 1) would be returned
// instead of Fine(t.x, 2), exposing a 1-fine-unit centre-offset error.
func TestPlayerReorientEntityPathingTargetNpcSize2(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	npc := makeInteractionNpc(t, s, 1, 105, 108, 0)
	npc.size = 2 // size-2 NPC: Fine(x, 2) = x*2+2, not x*2+1

	p.target = npc

	p.reorientEntity()

	if p.faceAngleX != coordgrid.Fine(105, 2) {
		t.Errorf("faceAngleX: got %d, want %d (Fine(105,2))", p.faceAngleX, coordgrid.Fine(105, 2))
	}
	if p.faceAngleZ != coordgrid.Fine(108, 2) {
		t.Errorf("faceAngleZ: got %d, want %d (Fine(108,2))", p.faceAngleZ, coordgrid.Fine(108, 2))
	}
}

// (a) TestPlayerReorientEntityNonPathingTargetNoop pins that reorientEntity
// no-ops for a Loc/Obj (non-pathing) target — the switch has no default
// arm in TS reorientEntity (PathingEntity.ts:364-369 @4c95f87e).
func TestPlayerReorientEntityNonPathingTargetNoop(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	loc := entitypkg.NewLoc(0, 105, 108, 1, 1, entitypkg.LifecycleForever, 0, 10, 0)
	p.target = loc
	preFaceX, preFaceZ := p.faceAngleX, p.faceAngleZ

	p.reorientEntity()

	if p.faceAngleX != preFaceX || p.faceAngleZ != preFaceZ {
		t.Errorf("faceAngle changed on non-pathing target: got (%d,%d), want (%d,%d)", p.faceAngleX, p.faceAngleZ, preFaceX, preFaceZ)
	}
}

// (b) TestPlayerReorientLocTargetStepsZero pins TS PathingEntity.reorient
// (PathingEntity.ts:377-388 @4c95f87e): Loc target, stepsTaken==0,
// targetX != -1 → focus(targetX, targetZ, client=true) — faceSquare AND
// the face-coord mask are now written (the key wire-visible change from
// the pre-e31a8719 instant=false behavior), then targetX/Z are cleared.
func TestPlayerReorientLocTargetStepsZero(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	loc := entitypkg.NewLoc(0, 105, 108, 1, 1, entitypkg.LifecycleForever, 0, 10, 0)
	p.target = loc
	p.targetX = 999
	p.targetZ = 1001
	p.stepsTaken = 0
	p.faceSquareX, p.faceSquareZ = -1, -1
	p.masks = 0

	p.reorient()

	if p.faceAngleX != 999 {
		t.Errorf("faceAngleX: got %d, want 999", p.faceAngleX)
	}
	if p.faceAngleZ != 1001 {
		t.Errorf("faceAngleZ: got %d, want 1001", p.faceAngleZ)
	}
	if p.faceSquareX != 999 {
		t.Errorf("faceSquareX: got %d, want 999 (client=true now writes faceSquare)", p.faceSquareX)
	}
	if p.faceSquareZ != 1001 {
		t.Errorf("faceSquareZ: got %d, want 1001 (client=true now writes faceSquare)", p.faceSquareZ)
	}
	if p.masks&rsbuf.MaskFaceCoord == 0 {
		t.Errorf("masks: MaskFaceCoord bit not set (masks=%d) — reorient() must ship client=true", p.masks)
	}
	if p.targetX != -1 {
		t.Errorf("targetX: got %d, want -1 (cleared after focus)", p.targetX)
	}
	if p.targetZ != -1 {
		t.Errorf("targetZ: got %d, want -1 (cleared after focus)", p.targetZ)
	}
}

// (c) TestPlayerReorientLocTargetStepsNonzero pins the early-out:
// Loc target with stepsTaken > 0 → no focus, no clear.
func TestPlayerReorientLocTargetStepsNonzero(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	loc := entitypkg.NewLoc(0, 105, 108, 1, 1, entitypkg.LifecycleForever, 0, 10, 0)
	p.target = loc
	p.targetX = 999
	p.targetZ = 1001
	p.stepsTaken = 3
	preFaceX := p.faceAngleX
	preFaceZ := p.faceAngleZ

	p.reorient()

	if p.faceAngleX != preFaceX {
		t.Errorf("faceAngleX: got %d, want %d (unchanged; stepsTaken>0 early-out)", p.faceAngleX, preFaceX)
	}
	if p.faceAngleZ != preFaceZ {
		t.Errorf("faceAngleZ: got %d, want %d", p.faceAngleZ, preFaceZ)
	}
	if p.targetX != 999 || p.targetZ != 1001 {
		t.Errorf("targetX/Z: got (%d,%d), want (999,1001) (unchanged)", p.targetX, p.targetZ)
	}
}

// (d) TestPlayerReorientPathingTargetEarlyReturn pins TS
// PathingEntity.reorient's guard (PathingEntity.ts:378-380 @4c95f87e):
// `if (this.target instanceof PathingEntity) return;` — reorient()
// early-returns for a pathing target even when targetX/Z are stale and
// non-(-1) (e.g. left over from a prior Loc/Obj interaction). No focus,
// no clear: reorientEntity() is the pathing-target's own driver and
// already ran earlier this tick.
func TestPlayerReorientPathingTargetEarlyReturn(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	other, otherWait := makeInteractionPlayer(t, s, 110, 120, 0)
	defer otherWait()

	p.target = other
	p.targetX = 999
	p.targetZ = 1001
	p.stepsTaken = 0
	preFaceX, preFaceZ := p.faceAngleX, p.faceAngleZ

	p.reorient()

	if p.faceAngleX != preFaceX || p.faceAngleZ != preFaceZ {
		t.Errorf("faceAngle changed on pathing target: got (%d,%d), want (%d,%d) (early-return)", p.faceAngleX, p.faceAngleZ, preFaceX, preFaceZ)
	}
	if p.targetX != 999 || p.targetZ != 1001 {
		t.Errorf("targetX/Z: got (%d,%d), want (999,1001) (unchanged by early-return)", p.targetX, p.targetZ)
	}
}

// TestPlayerReorientNilTarget pins the no-op when target is nil.
func TestPlayerReorientNilTarget(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	p.target = nil
	p.targetX = -1
	p.targetZ = -1
	preFaceX := p.faceAngleX
	preFaceZ := p.faceAngleZ

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("reorient panicked on nil target: %v", r)
		}
	}()

	p.reorient()

	if p.faceAngleX != preFaceX || p.faceAngleZ != preFaceZ {
		t.Errorf("faceAngle changed on nil target: got (%d,%d), want (%d,%d)", p.faceAngleX, p.faceAngleZ, preFaceX, preFaceZ)
	}
}

// (e) TestPlayerSetInteractionNoLongerFocuses pins TS
// PathingEntity.setInteraction @4c95f87e: setInteraction no longer calls
// focus() at all — facing only changes during the entity's own turn
// (reorientEntity/reorient). A loc target with kind=InteractionEngine
// used to focus(instant=true) immediately (pre-e31a8719); now
// faceSquareX/Z and the face-coord mask are UNCHANGED, faceAngle is
// UNCHANGED, and targetX/Z are still recorded (NonPathingEntity cache
// consumed later by reorient()).
func TestPlayerSetInteractionNoLongerFocuses(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	p.faceSquareX, p.faceSquareZ = -1, -1
	p.masks = 0
	preFaceX, preFaceZ := p.faceAngleX, p.faceAngleZ

	loc := entitypkg.NewLoc(0, 105, 108, 1, 1, entitypkg.LifecycleForever, 0, 10, 0)

	p.SetInteraction(InteractionEngine, loc, 1, -1)

	if p.faceAngleX != preFaceX || p.faceAngleZ != preFaceZ {
		t.Errorf("faceAngle: got (%d,%d), want unchanged (%d,%d) — SetInteraction must not focus", p.faceAngleX, p.faceAngleZ, preFaceX, preFaceZ)
	}
	if p.faceSquareX != -1 || p.faceSquareZ != -1 {
		t.Errorf("faceSquare: got (%d,%d), want (-1,-1) — SetInteraction must not write faceSquare", p.faceSquareX, p.faceSquareZ)
	}
	if p.masks&rsbuf.MaskFaceCoord != 0 {
		t.Errorf("masks: MaskFaceCoord bit set, want unset — SetInteraction must not OR the mask")
	}
	wantTX := coordgrid.Fine(105, 1)
	wantTZ := coordgrid.Fine(108, 1)
	if p.targetX != wantTX || p.targetZ != wantTZ {
		t.Errorf("targetX/Z: got (%d,%d), want (%d,%d) — still recorded for reorient() to consume", p.targetX, p.targetZ, wantTX, wantTZ)
	}
}
