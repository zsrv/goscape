package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
)

// TestNpcReorientPathingTargetPlayer pins TS PathingEntity.ts:351-353
// branch on the Npc side: target is *Player → focus on fine.
func TestNpcReorientPathingTargetPlayer(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	other, wait := makeInteractionPlayer(t, s, 110, 120, 0)
	defer wait()

	npc.target = other

	npc.reorient()

	if npc.faceAngleX != coordgrid.Fine(110, 1) {
		t.Errorf("faceAngleX: got %d, want %d", npc.faceAngleX, coordgrid.Fine(110, 1))
	}
	if npc.faceAngleZ != coordgrid.Fine(120, 1) {
		t.Errorf("faceAngleZ: got %d, want %d", npc.faceAngleZ, coordgrid.Fine(120, 1))
	}
	if npc.targetX != -1 || npc.targetZ != -1 {
		t.Errorf("targetX/Z: got (%d,%d), want (-1,-1) (unchanged for PathingEntity branch)", npc.targetX, npc.targetZ)
	}
}

// TestNpcReorientPathingTargetNpc — symmetric, *Npc target.
// makeInteractionNpc builds with typ.Size=0, so other.size defaults to 0.
// We set other.size = 1 to match the real Npc.New default for typ.Size=1.
func TestNpcReorientPathingTargetNpc(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	other := makeInteractionNpc(t, s, 2, 105, 108, 0)
	other.size = 1 // matches Npc.New default for typ.Size=1

	npc.target = other

	npc.reorient()

	if npc.faceAngleX != coordgrid.Fine(105, 1) {
		t.Errorf("faceAngleX: got %d, want %d", npc.faceAngleX, coordgrid.Fine(105, 1))
	}
	if npc.faceAngleZ != coordgrid.Fine(108, 1) {
		t.Errorf("faceAngleZ: got %d, want %d", npc.faceAngleZ, coordgrid.Fine(108, 1))
	}
}

// TestNpcReorientPathingTargetNpcSize2 pins the size>1 path on the NPC side.
// Mirrors TestPlayerReorientPathingTargetNpcSize2. Without the Fix 1 change
// Fine(t.x, 1) would be returned instead of Fine(t.x, 2).
func TestNpcReorientPathingTargetNpcSize2(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	other := makeInteractionNpc(t, s, 2, 105, 108, 0)
	other.size = 2 // size-2 target: Fine(x, 2) = x*2+2

	npc.target = other

	npc.reorient()

	if npc.faceAngleX != coordgrid.Fine(105, 2) {
		t.Errorf("faceAngleX: got %d, want %d (Fine(105,2))", npc.faceAngleX, coordgrid.Fine(105, 2))
	}
	if npc.faceAngleZ != coordgrid.Fine(108, 2) {
		t.Errorf("faceAngleZ: got %d, want %d (Fine(108,2))", npc.faceAngleZ, coordgrid.Fine(108, 2))
	}
}

// TestNpcReorientLocTargetStepsZero pins focus + clear on the default
// branch when stepsTaken == 0 and targetX != -1.
func TestNpcReorientLocTargetStepsZero(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)

	loc := entitypkg.NewLoc(0, 105, 108, 1, 1, entitypkg.LifecycleForever, 0, 10, 0)
	npc.target = loc
	npc.targetX = 999
	npc.targetZ = 1001
	npc.stepsTaken = 0

	npc.reorient()

	if npc.faceAngleX != 999 {
		t.Errorf("faceAngleX: got %d, want 999", npc.faceAngleX)
	}
	if npc.faceAngleZ != 1001 {
		t.Errorf("faceAngleZ: got %d, want 1001", npc.faceAngleZ)
	}
	if npc.targetX != -1 {
		t.Errorf("targetX: got %d, want -1 (cleared after focus)", npc.targetX)
	}
	if npc.targetZ != -1 {
		t.Errorf("targetZ: got %d, want -1 (cleared after focus)", npc.targetZ)
	}
}

// TestNpcReorientLocTargetStepsNonzero pins the early-out.
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
