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
	if p.interactionFired {
		t.Error("interactionFired: got true, want false (reset by tryInteract for post-step re-fire)")
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
	if !p.interactionFired {
		t.Error("interactionFired: got false, want true (no retry signal)")
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

	if !p.interactionFired {
		t.Error("interactionFired: got false, want true (NAI-69: fire helper uniform exit)")
	}
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

	if !p.interactionFired {
		t.Error("interactionFired: got false, want true (NAI-69: fire helper uniform exit)")
	}
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

// --- NAI-69 T4: AP-Npc structural parity (no-op) pin ---

// TestFireApTriggerNpc_ApRangeCalled_SetsInteractionFiredTrueStructural
// pins the post-NAI-69 contract for AP-Npc: the fire helper sets
// interactionFired=true at exit and apRangeCalled is set by p_aprange
// (mechanism activates structurally) — but the next tryInteract call
// would re-evaluate using effectiveApRange = npc.typ.AttackRange
// (fixed per-type, not p.apRange), so the retry path is a behavioral
// no-op for NPC targets. This preserves the preexisting goscape
// divergence at interaction.go:404 (effectiveApRange).
func TestFireApTriggerNpc_ApRangeCalled_SetsInteractionFiredTrueStructural(t *testing.T) {
	s, p, npc := newApTriggerNpcFixture(t)

	// Register an APNPC1 script for npc.typeId that calls p_aprange(2).
	sf := scriptFileWithApRangeCall(t, script.TriggerApNpc1, npc.typeId, 2)
	s.scriptProvider.Register(sf)

	fireApTriggerNpc(p, s, npc)

	// Mechanism activates structurally:
	if !p.interactionFired {
		t.Error("interactionFired: got false, want true (NAI-69: uniform exit)")
	}
	if !p.apRangeCalled {
		t.Error("apRangeCalled: got false, want true (script called p_aprange)")
	}
	if p.apRange != 2 {
		t.Errorf("apRange: got %d, want 2 (script set new range — but effectiveApRange reads npc.typ.AttackRange for NPC targets)", p.apRange)
	}

	// effectiveApRange divergence pin: for NPC targets, the retry
	// decision uses npc.typ.AttackRange, NOT p.apRange. Verify the
	// in-range check still uses the NPC's AttackRange.
	if effectiveApRange(p) != int(npc.typ.AttackRange) {
		t.Errorf("effectiveApRange: got %d, want %d (NPC AttackRange, not p.apRange — preexisting divergence)",
			effectiveApRange(p), npc.typ.AttackRange)
	}
}
