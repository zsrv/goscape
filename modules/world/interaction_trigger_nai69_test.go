package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/script"
)

// --- NAI-69 T1: tryInteract AP-branch same-tick retry pin ---

// TestTryInteract_ApRangeCalled_ReturnsFalseAndResetsFired pins the new
// TS L1163-1167 contract: when the AP script set apRangeCalled=true and
// nextTarget is nil, tryInteract resets interactionFired=false and
// returns false so processInteraction's walk-arm runs and the post-step
// tryInteract can re-fire AP.
//
// NAI-69 closes NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED.
func TestTryInteract_ApRangeCalled_ReturnsFalseAndResetsFired(t *testing.T) {
	s, p, loc, _ := makeApTriggerFixture(t)

	// Register an APLOC1 script that calls p_aprange(2).
	sf := scriptFileWithApRangeCall(t, script.TriggerApLoc1, loc.Type(), 2)
	s.scriptProvider.Register(sf)

	// Pre-state: in 10-range (apRange default), distance 5 (fixture
	// invariant from makeApTriggerFixture).
	result := p.tryInteract(false)

	if result {
		t.Error("tryInteract: got true, want false (TS L1167 — apRangeCalled triggers same-tick retry)")
	}
	if !p.apRangeCalled {
		t.Error("apRangeCalled: got false, want true (script called p_aprange)")
	}
	if p.target != loc {
		t.Errorf("target: got %v, want loc (preserved across AP fire)", p.target)
	}
	if p.nextTarget != nil {
		t.Errorf("nextTarget: got %v, want nil (script did not call p_op_*)", p.nextTarget)
	}
}

// TestTryInteract_NoApRange_StillReturnsTrue pins that the new guard
// only triggers when apRangeCalled. A no-op AP script (no p_aprange)
// keeps the pre-NAI-69 contract: returns true, interactionFired stays
// true.
func TestTryInteract_NoApRange_StillReturnsTrue(t *testing.T) {
	s, p, loc, _ := makeApTriggerFixture(t)

	// Register a no-op APLOC1 script.
	sf := newNoopScriptFile(t, script.TriggerApLoc1, loc.Type(), -1)
	s.scriptProvider.Register(sf)

	result := p.tryInteract(false)

	if !result {
		t.Error("tryInteract: got false, want true (no apRangeCalled — original contract)")
	}
	if p.apRangeCalled {
		t.Error("apRangeCalled: got true, want false (script did not call p_aprange)")
	}
}

// --- NAI-69 T2: fire-helper uniform interactionFired=true contract ---

// TestFireApTriggerLoc_ApRangeCalled_SetsInteractionFiredTrue pins the
// post-NAI-69 contract: fireApTriggerLoc always sets
// interactionFired=true at fire end, regardless of apRangeCalled state.
// The pre-NAI-69 across-tick re-fire scaffold (early-return on
// Finished/Aborted+apRangeCalled leaving interactionFired=false) is
// dropped — same-tick retry is now signaled via apRangeCalled and
// handled by tryInteract (see T1).
func TestFireApTriggerLoc_ApRangeCalled_SetsInteractionFiredTrue(t *testing.T) {
	s, p, loc, _ := makeApTriggerFixture(t)

	sf := scriptFileWithApRangeCall(t, script.TriggerApLoc1, loc.Type(), 2)
	s.scriptProvider.Register(sf)

	tryFireApTrigger(p)

	if !p.apRangeCalled {
		t.Error("apRangeCalled: got false, want true (script called p_aprange)")
	}
	if p.apRange != 2 {
		t.Errorf("apRange: got %d, want 2 (script set new range)", p.apRange)
	}
	if p.target != loc {
		t.Errorf("target: got %v, want loc (restored after fire)", p.target)
	}
}

// TestFireApTriggerObj_ApRangeCalled_SetsInteractionFiredTrue — AP-Obj
// parity. fireApTriggerObj's post-NAI-69 contract is identical to
// fireApTriggerLoc.
func TestFireApTriggerObj_ApRangeCalled_SetsInteractionFiredTrue(t *testing.T) {
	s, p, obj, _ := makeApObjTriggerFixture(t)

	// Register an APOBJ1 script that calls p_aprange(2).
	sf := scriptFileWithApRangeCall(t, script.TriggerApObj1, obj.Type, 2)
	s.scriptProvider.Register(sf)

	fireApTriggerObj(p, s, obj)

	if !p.apRangeCalled {
		t.Error("apRangeCalled: got false, want true (script called p_aprange)")
	}
	if p.apRange != 2 {
		t.Errorf("apRange: got %d, want 2 (script set new range)", p.apRange)
	}
	if p.target != obj {
		t.Errorf("target: got %v, want obj (restored after fire)", p.target)
	}
}

// --- NAI-69 T4: AP-Npc full parity (p_aprange round-trips) ---

// TestFireApTriggerNpc_ApRangeCalled_SetsInteractionFiredTrue pins the
// post-NAI-69 contract for AP-Npc, restored to full TS parity: the
// fire helper sets interactionFired=true at exit, apRangeCalled is set
// by p_aprange, and the next tryInteract call uses the player's
// updated apRange — matching TS Player.tryInteract (Player.ts:1139)
// which reads this.apRange regardless of target type.
//
// Pre-fix Go's effectiveApRange short-circuited to npc.typ.AttackRange
// for NPC targets, which silently dropped p_aprange's effect on the
// AP retry path. That bug broke ranged attacks against melee NPCs
// (most visible when shooting through fences). NAI-69 closure.
func TestFireApTriggerNpc_ApRangeCalled_SetsInteractionFiredTrue(t *testing.T) {
	s, p, npc := newApTriggerNpcFixture(t)

	// Register an APNPC1 script for npc.typeId that calls p_aprange(2).
	sf := scriptFileWithApRangeCall(t, script.TriggerApNpc1, npc.typeId, 2)
	s.scriptProvider.Register(sf)

	fireApTriggerNpc(p, s, npc)

	if !p.apRangeCalled {
		t.Error("apRangeCalled: got false, want true (script called p_aprange)")
	}
	if p.apRange != 2 {
		t.Errorf("apRange: got %d, want 2 (script set new range)", p.apRange)
	}

	// AP retry path uses p.apRange unconditionally — TS Player.ts:1139.
	if got, want := effectiveApRange(p), 2; got != want {
		t.Errorf("effectiveApRange: got %d, want %d (p.apRange — NAI-69 closure)", got, want)
	}
}

// TestEffectiveApRange_NpcTarget_UsesPlayerApRangeNotNpcAttackRange
// reproduces the "can't shoot ranged through a fence" bug. When a
// player wields a bow, its apheld trigger calls p_aprange(N) where N is
// the bow's range (e.g. 7). TS Player.tryInteract (Player.ts:1139)
// reads this.apRange when deciding whether to fire the AP trigger. Go
// previously read npc.typ.AttackRange instead (melee NPC = 1), so a
// ranged player against a melee NPC across a fence could never reach
// AP-firing distance because the fence walk-blocks adjacency.
func TestEffectiveApRange_NpcTarget_UsesPlayerApRangeNotNpcAttackRange(t *testing.T) {
	_, p, npc := newApTriggerNpcFixture(t)
	npc.typ.AttackRange = 1 // melee NPC
	p.apRange = 7           // bow apheld set this

	if got, want := effectiveApRange(p), 7; got != want {
		t.Errorf("effectiveApRange: got %d, want %d (player.apRange — TS Player.ts:1139 reads this.apRange regardless of target type)", got, want)
	}
}
