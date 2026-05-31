package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// interaction-4 regression pin.
//
// TS PathingEntity.clearInteraction (PathingEntity.ts:550-555) resets the
// interaction anchor AND the target-identity snapshot:
//
//	this.target = null;
//	this.targetOp = -1;
//	this.targetSubject = { type: -1, com: -1 };
//	this.apRange = 10;
//	this.apRangeCalled = false;
//
// goscape's targetSubject additionally carries the loc/obj x/z/level snapshot
// (written by handler_oploc/handler_opobj for locStillValid/objStillValid),
// so a faithful clear must reset all five fields to -1. The audit found
// ClearInteraction omitted the targetSubject reset entirely, leaving a stale
// subject identity that survived an interaction clear.

// TestClearInteraction_ResetsTargetSubject pins that ClearInteraction clears
// the full targetSubject identity snapshot (typ/x/z/level/com) along with the
// target and targetOp anchor.
func TestClearInteraction_ResetsTargetSubject(t *testing.T) {
	p := &Player{}
	npc := NewNpc(1, 7, 101, 100, 0, &objtype.NpcType{})

	// Simulate an active interaction whose subject identity was snapshotted:
	// typ/x/z/level as handler_oploc/handler_opobj write them, com as
	// SetInteraction writes it.
	p.target = npc
	p.targetOp = 1
	p.targetSubject.typ = 7
	p.targetSubject.x = 101
	p.targetSubject.z = 100
	p.targetSubject.level = 0
	p.targetSubject.com = 42

	p.ClearInteraction()

	if p.target != nil {
		t.Fatalf("expected target nil after clear, got %v", p.target)
	}
	if p.targetOp != -1 {
		t.Fatalf("expected targetOp -1 after clear, got %d", p.targetOp)
	}
	if got := p.targetSubject; got.typ != -1 || got.x != -1 || got.z != -1 ||
		got.level != -1 || got.com != -1 {
		t.Fatalf("expected targetSubject fully reset to -1 after clear "+
			"(TS PathingEntity.ts:550), got %+v", got)
	}
}

// player-script-7 regression pin.
//
// TS Player.clearPendingAction (Player.ts:950-953) is just clearInteraction
// + closeModal. clearInteraction (PathingEntity.ts:550-555) resets target/
// targetOp/targetSubject/apRange/apRangeCalled. goscape's pre-fix
// ClearPendingAction did a partial inline reset (target + targetOp only),
// leaving apRange / apRangeCalled / targetSubject stuck at their last
// interaction's values. Many handlers funnel through ClearPendingAction
// (op_player, opheld, minimap-walk modal-close — see handler_opheld.go
// + handler_op_player.go), so any of them could carry a stale apRange or
// targetSubject into the next interaction.
//
// TestClearPendingAction_FullyClearsInteractionState pins the full TS-faithful
// reset: target/targetOp PLUS apRange/apRangeCalled/targetSubject. Without
// the fix apRange would stay at the prior 7, apRangeCalled at true, and
// targetSubject would still hold the stale {typ:7 x:101 z:100 level:0 com:42}.
func TestClearPendingAction_FullyClearsInteractionState(t *testing.T) {
	p := &Player{}
	npc := NewNpc(1, 7, 101, 100, 0, &objtype.NpcType{})

	// Active interaction with non-default apRange + apRangeCalled,
	// matching the shape SetApRange + SetInteraction produce.
	p.target = npc
	p.targetOp = 1
	p.targetSubject.typ = 7
	p.targetSubject.x = 101
	p.targetSubject.z = 100
	p.targetSubject.level = 0
	p.targetSubject.com = 42
	p.apRange = 7
	p.apRangeCalled = true

	p.ClearPendingAction()

	if p.target != nil {
		t.Errorf("target: got %v, want nil (TS clearInteraction)", p.target)
	}
	if p.targetOp != -1 {
		t.Errorf("targetOp: got %d, want -1 (TS clearInteraction)", p.targetOp)
	}
	if p.apRange != 10 {
		t.Errorf("apRange: got %d, want 10 (TS clearInteraction default; pre-fix this stayed at 7)", p.apRange)
	}
	if p.apRangeCalled {
		t.Errorf("apRangeCalled: got true, want false (TS clearInteraction; pre-fix this stayed true)")
	}
	if got := p.targetSubject; got.typ != -1 || got.x != -1 || got.z != -1 ||
		got.level != -1 || got.com != -1 {
		t.Errorf("targetSubject: got %+v, want all -1 (TS clearInteraction; pre-fix this stayed at the stale snapshot)", got)
	}
}
