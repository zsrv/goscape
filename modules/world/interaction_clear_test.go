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
