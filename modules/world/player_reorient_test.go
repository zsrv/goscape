package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
)

// TestPlayerReorientPathingTargetPlayer pins TS PathingEntity.ts:351-353
// branch: target is *Player → focus on fine(t.x, 1), fine(t.z, 1).
// targetX/Z left untouched.
func TestPlayerReorientPathingTargetPlayer(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	other, otherWait := makeInteractionPlayer(t, s, 110, 120, 0)
	defer otherWait()

	p.target = other

	p.reorient()

	if p.faceAngleX != coordgrid.Fine(110, 1) {
		t.Errorf("faceAngleX: got %d, want %d", p.faceAngleX, coordgrid.Fine(110, 1))
	}
	if p.faceAngleZ != coordgrid.Fine(120, 1) {
		t.Errorf("faceAngleZ: got %d, want %d", p.faceAngleZ, coordgrid.Fine(120, 1))
	}
	if p.targetX != -1 || p.targetZ != -1 {
		t.Errorf("targetX/Z: got (%d,%d), want (-1,-1) (unchanged for PathingEntity branch)", p.targetX, p.targetZ)
	}
}

// TestPlayerReorientPathingTargetNpc — symmetric to above, *Npc target.
// makeInteractionNpc builds with typ.Size=0, so npc.size defaults to 0.
// We set npc.size = 1 to match the real Npc.New default (NpcType.Size
// defaults to 1 in production configs, NpcType.RegisterFlagsAndApplyDefaults
// line ~310). Fine(x, 1) is the expected centre for a size-1 NPC.
func TestPlayerReorientPathingTargetNpc(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	npc := makeInteractionNpc(t, s, 1, 105, 108, 0)
	npc.size = 1 // matches Npc.New default for typ.Size=1

	p.target = npc

	p.reorient()

	if p.faceAngleX != coordgrid.Fine(105, 1) {
		t.Errorf("faceAngleX: got %d, want %d", p.faceAngleX, coordgrid.Fine(105, 1))
	}
	if p.faceAngleZ != coordgrid.Fine(108, 1) {
		t.Errorf("faceAngleZ: got %d, want %d", p.faceAngleZ, coordgrid.Fine(108, 1))
	}
}

// TestPlayerReorientPathingTargetNpcSize2 pins the size>1 path. Without
// the Fix 1 production change Fine(t.x, 1) would be returned instead of
// Fine(t.x, 2), exposing a 32-fine-unit centre-offset error.
func TestPlayerReorientPathingTargetNpcSize2(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	npc := makeInteractionNpc(t, s, 1, 105, 108, 0)
	npc.size = 2 // size-2 NPC: Fine(x, 2) = x*64+63, not x*64+31

	p.target = npc

	p.reorient()

	if p.faceAngleX != coordgrid.Fine(105, 2) {
		t.Errorf("faceAngleX: got %d, want %d (Fine(105,2))", p.faceAngleX, coordgrid.Fine(105, 2))
	}
	if p.faceAngleZ != coordgrid.Fine(108, 2) {
		t.Errorf("faceAngleZ: got %d, want %d (Fine(108,2))", p.faceAngleZ, coordgrid.Fine(108, 2))
	}
}

// TestPlayerReorientLocTargetStepsZero pins the default-arm focus + clear:
// Loc target with stepsTaken==0 and targetX != -1 → focus on cached
// targetX/Z, then clear to -1.
func TestPlayerReorientLocTargetStepsZero(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	loc := entitypkg.NewLoc(0, 105, 108, 1, 1, entitypkg.LifecycleForever, 0, 10, 0)
	p.target = loc
	p.targetX = 999
	p.targetZ = 1001
	p.stepsTaken = 0

	p.reorient()

	if p.faceAngleX != 999 {
		t.Errorf("faceAngleX: got %d, want 999", p.faceAngleX)
	}
	if p.faceAngleZ != 1001 {
		t.Errorf("faceAngleZ: got %d, want 1001", p.faceAngleZ)
	}
	if p.targetX != -1 {
		t.Errorf("targetX: got %d, want -1 (cleared after focus)", p.targetX)
	}
	if p.targetZ != -1 {
		t.Errorf("targetZ: got %d, want -1 (cleared after focus)", p.targetZ)
	}
}

// TestPlayerReorientLocTargetStepsNonzero pins the early-out:
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
