package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
)

// --- A8 / ee28c1aa: PathingEntity.setFaceEntity extraction -----------------
//
// TS PathingEntity.ts:506-524 @2e3bcf43:
//
//	setFaceEntity(): void {
//	    const oldEntity = this.faceEntity;
//	    if (this.target instanceof Player) {
//	        const playerSlot: number = this.target.slot + 32768;
//	        if (this.faceEntity !== playerSlot) { this.faceEntity = playerSlot; }
//	    } else if (this.target instanceof Npc) {
//	        const nid: number = this.target.nid;
//	        if (this.faceEntity !== nid) { this.faceEntity = nid; }
//	    } else {
//	        this.faceEntity = -1;
//	    }
//	    if (this.faceEntity !== oldEntity) { this.masks |= this.entitymask; }
//	}

// TestPlayerSetFaceEntityPlayerTarget pins the *Player target arm:
// faceEntity = target.slot + 32768 + entitymask emission.
func TestPlayerSetFaceEntityPlayerTarget(t *testing.T) {
	p, _ := newTestPlayer(t)
	other, _ := newTestPlayer(t)
	other.slot = 5
	p.target = other

	p.setFaceEntity()

	if want := 5 + 32768; p.faceEntity != want {
		t.Errorf("faceEntity: got %d, want %d (slot+32768)", p.faceEntity, want)
	}
	if p.masks&MaskFaceEntity == 0 {
		t.Error("MaskFaceEntity bit should be set on change")
	}
}

// TestPlayerSetFaceEntityNpcTarget pins the *Npc target arm: faceEntity = nid.
func TestPlayerSetFaceEntityNpcTarget(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 7, 100, 100, 0)
	p, _ := newTestPlayer(t)
	p.target = npc

	p.setFaceEntity()

	if p.faceEntity != npc.nid {
		t.Errorf("faceEntity: got %d, want %d (npc.nid)", p.faceEntity, npc.nid)
	}
	if p.masks&MaskFaceEntity == 0 {
		t.Error("MaskFaceEntity bit should be set on change")
	}
}

// TestPlayerSetFaceEntityNilTargetClears pins the else arm for nil target:
// faceEntity snaps to -1 and the mask fires (changed from 42).
func TestPlayerSetFaceEntityNilTargetClears(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.faceEntity = 42
	p.target = nil

	p.setFaceEntity()

	if p.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (nil target)", p.faceEntity)
	}
	if p.masks&MaskFaceEntity == 0 {
		t.Error("MaskFaceEntity bit should be set on change to -1")
	}
}

// TestPlayerSetFaceEntityLocTargetClears pins the NEW ee28c1aa else-arm
// semantics: a *Loc target also clears faceEntity (TS `else { this.faceEntity
// = -1; }` — instanceof Player/Npc both fail for Loc). Pre-ee28c1aa the
// Loc/Obj clear only happened in the World.ts post-decode block.
func TestPlayerSetFaceEntityLocTargetClears(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.faceEntity = 42
	p.target = entitypkg.NewLoc(0, 50, 60, 3, 2, entitypkg.LifecycleForever, 0, 10, 0)

	p.setFaceEntity()

	if p.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (Loc target → else arm)", p.faceEntity)
	}
	if p.masks&MaskFaceEntity == 0 {
		t.Error("MaskFaceEntity bit should be set on change to -1")
	}
}

// TestPlayerSetFaceEntityNoChangeNoMask pins the change-gate: a second call
// with the same target must NOT re-emit the mask (TS `if (this.faceEntity
// !== oldEntity)` at PathingEntity.ts:521).
func TestPlayerSetFaceEntityNoChangeNoMask(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 7, 100, 100, 0)
	p, _ := newTestPlayer(t)
	p.target = npc

	p.setFaceEntity()
	p.masks = 0
	p.setFaceEntity()

	if p.masks&MaskFaceEntity != 0 {
		t.Error("MaskFaceEntity must NOT re-emit when faceEntity unchanged")
	}
	// Same for the already--1 idle case: no target, faceEntity already -1.
	p.target = nil
	p.faceEntity = -1
	p.masks = 0
	p.setFaceEntity()
	if p.masks != 0 {
		t.Errorf("masks: got %d, want 0 (idle no-op)", p.masks)
	}
}

// TestNpcSetFaceEntityPlayerTarget pins the Npc fork's *Player arm.
func TestNpcSetFaceEntityPlayerTarget(t *testing.T) {
	s := newTestServer(t)
	n := makeInteractionNpc(t, s, 7, 100, 100, 0)
	p, _ := newTestPlayer(t)
	p.slot = 9
	n.target = p

	n.setFaceEntity()

	if want := 9 + 32768; n.faceEntity != want {
		t.Errorf("faceEntity: got %d, want %d (slot+32768)", n.faceEntity, want)
	}
	if n.masks&NpcMaskFaceEntity == 0 {
		t.Error("NpcMaskFaceEntity bit should be set on change")
	}
}

// TestNpcSetFaceEntityNpcTargetAndIdle pins the Npc fork's *Npc arm + the
// idle change-gate.
func TestNpcSetFaceEntityNpcTargetAndIdle(t *testing.T) {
	s := newTestServer(t)
	n := makeInteractionNpc(t, s, 7, 100, 100, 0)
	other := makeInteractionNpc(t, s, 8, 101, 100, 0)
	n.target = other

	n.setFaceEntity()
	if n.faceEntity != other.nid {
		t.Errorf("faceEntity: got %d, want %d (other.nid)", n.faceEntity, other.nid)
	}

	// Change-gate: same target, no re-emit.
	n.masks = 0
	n.setFaceEntity()
	if n.masks&NpcMaskFaceEntity != 0 {
		t.Error("NpcMaskFaceEntity must NOT re-emit when unchanged")
	}

	// nil target → -1 + mask.
	n.target = nil
	n.masks = 0
	n.setFaceEntity()
	if n.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (nil target)", n.faceEntity)
	}
	if n.masks&NpcMaskFaceEntity == 0 {
		t.Error("NpcMaskFaceEntity bit should be set on clear")
	}
}

// --- ee28c1aa call sites ----------------------------------------------------

// TestPlayerResetMasksRefreshesFacingFromTarget pins the resetPathingEntity
// tail call (TS PathingEntity.ts:626 @2e3bcf43: `this.setFaceEntity();`
// replaced the old `if (!this.target && this.faceEntity !== -1)` block):
// with a live *Npc target and a stale faceEntity, ResetMasks now REFRESHES
// facing to the target instead of leaving it stale.
func TestPlayerResetMasksRefreshesFacingFromTarget(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 7, 100, 100, 0)
	p, _ := newTestPlayer(t)
	p.target = npc
	p.faceEntity = -1 // stale

	p.ResetMasks()

	if p.faceEntity != npc.nid {
		t.Errorf("faceEntity: got %d, want %d (ResetMasks tail must call setFaceEntity)", p.faceEntity, npc.nid)
	}
	if p.masks&MaskFaceEntity == 0 {
		t.Error("MaskFaceEntity bit should be armed for next tick's info-pass")
	}
}

// TestPlayerResetMasksClearsFacingForLocTarget pins the ee28c1aa semantic
// change at the ResetMasks tail: a *Loc target clears a lingering
// faceEntity (else arm), where the pre-ee28c1aa block preserved it
// (`!this.target` was false).
func TestPlayerResetMasksClearsFacingForLocTarget(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.target = entitypkg.NewLoc(0, 50, 60, 1, 1, entitypkg.LifecycleForever, 0, 10, 0)
	p.faceEntity = 42

	p.ResetMasks()

	if p.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (Loc target → setFaceEntity else arm)", p.faceEntity)
	}
	if p.masks&MaskFaceEntity == 0 {
		t.Error("MaskFaceEntity bit should be armed on clear")
	}
}

// TestNpcResetMasksRefreshesFacingFromTarget is the Npc fork of the
// ResetMasks-tail pin (TS PathingEntity.ts:626 via Npc resetEntity →
// super.resetPathingEntity).
func TestNpcResetMasksRefreshesFacingFromTarget(t *testing.T) {
	s := newTestServer(t)
	n := makeInteractionNpc(t, s, 7, 100, 100, 0)
	p, _ := newTestPlayer(t)
	p.slot = 3
	n.target = p
	n.faceEntity = -1 // stale

	n.ResetMasks()

	if want := 3 + 32768; n.faceEntity != want {
		t.Errorf("faceEntity: got %d, want %d (ResetMasks tail must call setFaceEntity)", n.faceEntity, want)
	}
	if n.masks&NpcMaskFaceEntity == 0 {
		t.Error("NpcMaskFaceEntity bit should be armed")
	}
}

// TestNpcTurnFacesTargetAfterMovementInteraction pins the Npc.turn call
// site (TS Npc.ts:182-184 @2e3bcf43: processMovementInteraction →
// "// Update target facing" → setFaceEntity()). An NPC whose target was
// set outside SetInteraction (which no longer writes faceEntity) gets
// its faceEntity derived during turn().
func TestNpcTurnFacesTargetAfterMovementInteraction(t *testing.T) {
	s := newTestServer(t)
	n := makeInteractionNpc(t, s, 7, 100, 100, 0)
	p, _ := newTestPlayer(t)
	p.slot = 4
	n.target = p
	n.targetOp = 0 // NPCModeNone — processMovementInteraction noMode path

	n.turn(s)

	if want := 4 + 32768; n.faceEntity != want {
		t.Errorf("faceEntity: got %d, want %d (turn() must call setFaceEntity after processMovementInteraction)", n.faceEntity, want)
	}
}
