package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

// npcWithHP builds an Npc whose NpcType.Stats[NpcStatHitpoints] = maxHP,
// then overrides levels[HP] if needed. NewNpc seeds both levels[HP] and
// baseLevels[HP] from Stats[NpcStatHitpoints] as of S6d (NAI-17 extends
// the array to all 6 stats), so the override is only meaningful when the
// caller wants a starting HP distinct from max.
func npcWithHP(t *testing.T, maxHP, curHP int) *Npc {
	t.Helper()
	typ := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 7, DebugName: "rat"},
	}
	typ.Stats = []uint16{0, 0, 0, uint16(maxHP), 0, 0}
	npc := NewNpc(0, 7, 3222, 3218, 0, typ)
	npc.levels[objtype.NpcStatHitpoints] = curHP
	return npc
}

func TestNpcDamageDecrementsHPAndSetsMask(t *testing.T) {
	npc := npcWithHP(t, 10, 10)
	npc.Damage(3, 1)
	if npc.CurHP() != 7 {
		t.Errorf("curHP: got %d, want 7", npc.CurHP())
	}
	if npc.BaseHP() != 10 {
		t.Errorf("baseHP: got %d, want 10", npc.BaseHP())
	}
	if npc.damageAmt != 3 {
		t.Errorf("damageAmt: got %d, want 3", npc.damageAmt)
	}
	if npc.damageType != 1 {
		t.Errorf("damageType: got %d, want 1", npc.damageType)
	}
	if npc.masks&rsbuf.NpcMaskDamage == 0 {
		t.Error("NpcMaskDamage bit not set on npc.masks")
	}
}

func TestNpcDamageClampsAtZero(t *testing.T) {
	npc := npcWithHP(t, 10, 2)
	npc.Damage(5, 1)
	if npc.CurHP() != 0 {
		t.Errorf("curHP: got %d, want 0 (clamped)", npc.CurHP())
	}
	// damageAmt clamps to prev curHP on overkill — matches TS
	// Npc.applyDamage (hitmarkDamage = current), not the raw requested amount.
	if npc.damageAmt != 2 {
		t.Errorf("damageAmt: got %d, want 2 (clamped to prev curHP on overkill)", npc.damageAmt)
	}
}

func TestNpcDamageNegativeAmountClampsToZero(t *testing.T) {
	npc := npcWithHP(t, 10, 10)
	npc.Damage(-3, 1)
	if npc.CurHP() != 10 {
		t.Errorf("curHP: got %d, want 10 (negative amount must not heal)", npc.CurHP())
	}
	if npc.damageAmt != 0 {
		t.Errorf("damageAmt: got %d, want 0 (negative amount clamped)", npc.damageAmt)
	}
	if npc.masks&rsbuf.NpcMaskDamage == 0 {
		t.Error("NpcMaskDamage bit not set (mask should still flip even at zero damage)")
	}
}

func TestNewNpcSeedsHPFromStats(t *testing.T) {
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}}
	typ.Stats = []uint16{0, 0, 0, 20, 0, 0} // NpcStatHitpoints = 3
	npc := NewNpc(0, 7, 3222, 3218, 0, typ)
	if npc.CurHP() != 20 {
		t.Errorf("curHP: got %d, want 20", npc.CurHP())
	}
	if npc.BaseHP() != 20 {
		t.Errorf("baseHP: got %d, want 20", npc.BaseHP())
	}
}

func TestNewNpcWithEmptyStatsSeedsZeroHP(t *testing.T) {
	// &NpcType{} has nil Stats, so the NAI-17 seeding loop runs zero
	// iterations and levels[HP]/baseLevels[HP] stay zero-valued.
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}}
	npc := NewNpc(0, 7, 3222, 3218, 0, typ)
	if npc.CurHP() != 0 || npc.BaseHP() != 0 {
		t.Errorf("curHP/baseHP: got %d/%d, want 0/0", npc.CurHP(), npc.BaseHP())
	}
}

func TestNpcDamagePersistsAcrossResetMasks(t *testing.T) {
	npc := npcWithHP(t, 10, 10)
	npc.Damage(3, 1)
	if npc.CurHP() != 7 {
		t.Fatalf("pre-reset curHP: got %d, want 7", npc.CurHP())
	}
	npc.ResetMasks()
	if npc.CurHP() != 7 {
		t.Errorf("post-reset curHP: got %d, want 7 (persistent)", npc.CurHP())
	}
	if npc.damageAmt != -1 {
		t.Errorf("damageAmt: got %d, want -1 (per-tick cleared)", npc.damageAmt)
	}
}

func TestNpcBaseHPPersistsAcrossResetMasks(t *testing.T) {
	npc := npcWithHP(t, 10, 10)
	npc.Damage(3, 1)
	npc.ResetMasks()
	if npc.BaseHP() != 10 {
		t.Errorf("post-reset baseHP: got %d, want 10 (persistent)", npc.BaseHP())
	}
}

func TestNpcResetHP(t *testing.T) {
	npc := npcWithHP(t, 10, 10)
	npc.Damage(7, 1)
	if npc.CurHP() != 3 {
		t.Fatalf("pre-reset curHP: got %d, want 3", npc.CurHP())
	}
	npc.ResetHP()
	if npc.CurHP() != 10 {
		t.Errorf("curHP after ResetHP: got %d, want 10", npc.CurHP())
	}
	if npc.BaseHP() != 10 {
		t.Errorf("baseHP after ResetHP: got %d, want 10", npc.BaseHP())
	}
}

func TestNpcResetHPWithNilTypDirectConstruction(t *testing.T) {
	// NewNpc would panic on nil typ (PatrolCoord / WanderRange / etc.), so
	// build *Npc directly to exercise the nil-typ guard in ResetHP. Any
	// future caller that manually constructs an Npc must survive ResetHP
	// cleanly.
	npc := &Npc{}
	npc.ResetHP()
	if npc.CurHP() != 0 || npc.BaseHP() != 0 {
		t.Errorf("after ResetHP on nil-typ npc: got %d/%d, want 0/0", npc.CurHP(), npc.BaseHP())
	}
}

// TestNewNpcSetsEntityMaskToFaceEntity — NAI-13 Task 1.
// Mirrors TS PathingEntity.ts:107 where `this.entitymask = entitymask` is
// set at construction. For NPC, this is NpcMaskFaceEntity; the consumer
// is SetInteraction / resetDefaults which emit the faceEntity bit via
// `n.masks |= n.entitymask`. Before this change the field was always 0
// and the `|=` lines were no-ops.
func TestNewNpcSetsEntityMaskToFaceEntity(t *testing.T) {
	n := newTestNpc(1)
	if n.entitymask != rsbuf.NpcMaskFaceEntity {
		t.Errorf("entitymask: got %d, want %d (NpcMaskFaceEntity)", n.entitymask, rsbuf.NpcMaskFaceEntity)
	}
}

// TestResetDefaultsEmitsEntityMask — NAI-13 Task 1.
// Mirrors TS Npc.ts:416 where resetDefaults ends with `this.masks |=
// this.entitymask`. Before NAI-13 the `|=` line in resetDefaults did
// not exist; this test guards the new line + proves entitymask is
// non-zero post-construction.
func TestResetDefaultsEmitsEntityMask(t *testing.T) {
	n := newTestNpc(1)
	n.masks = 0 // clear any construction-time bits
	n.resetDefaults()
	if n.masks&rsbuf.NpcMaskFaceEntity == 0 {
		t.Errorf("masks & NpcMaskFaceEntity: got 0, want nonzero (resetDefaults should emit faceEntity bit)")
	}
}

// TestSetInteractionEmitsEntityMask — NAI-13 Task 1.
// The line `n.masks |= n.entitymask` at npc_interaction.go SetInteraction
// was a no-op before NAI-13 because entitymask was 0. With entitymask
// now wired in NewNpc (Step 1.3), that line emits the faceEntity bit.
// Uses an *Npc target to avoid Player construction overhead — SetInteraction's
// target branch that sets faceEntity fires for both Player and Npc targets.
func TestSetInteractionEmitsEntityMask(t *testing.T) {
	n := newTestNpc(1)
	n.masks = 0
	n.server = &Server{log: discardLogger()} // SetInteraction does not touch log on happy path

	// Use a live *Npc target (has Coords, IsValid via !dead). Npc is an entity.
	target := newTestNpc(2)

	ok := n.SetInteraction(InteractionScript, target, objtype.NPCModeOpNpc1, 0)
	if !ok {
		t.Fatal("SetInteraction returned false (target.IsValid failed?)")
	}
	if n.masks&rsbuf.NpcMaskFaceEntity == 0 {
		t.Errorf("masks & NpcMaskFaceEntity: got 0, want nonzero (SetInteraction should emit faceEntity bit for *Npc target)")
	}
}

// TestNpcResetDefaultsClearsFaceEntity — NAI-14 Task 1.
// Named companion for the faceEntity-clear half of resetDefaults that
// lands with NAI-14. Separated from TestResetDefaultsEmitsEntityMask
// (NAI-13 mask-bit assertion) and from
// TestNpcResetDefaultsClearsTargetKeepsOtherState (the regression guard
// for the stripped NAI-11 shape). Mirrors TS Npc.ts:415:
// `this.faceEntity = -1;`.
func TestNpcResetDefaultsClearsFaceEntity(t *testing.T) {
	n := newTestNpc(1)
	n.faceEntity = 42
	n.resetDefaults()
	if n.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (resetDefaults should clear per TS Npc.ts:415)", n.faceEntity)
	}
}

// TestNpcClearInteractionEmitsEntityMaskAndClearsFaceEntity — NAI-14 Task 1.
// Named companion for the faceEntity-clear + mask-emit pair that
// clearInteraction gains in NAI-14. Mirrors TS Npc.ts:407-408:
// `this.faceEntity = -1; this.masks |= NpcInfoProt.FACE_ENTITY;`.
// Separated from TestNpcClearInteractionResetsState (the full-state
// regression guard) so the TS-line mapping is explicit in one test name.
func TestNpcClearInteractionEmitsEntityMaskAndClearsFaceEntity(t *testing.T) {
	n := newTestNpc(1)
	n.faceEntity = 42
	n.masks = 0
	n.clearInteraction()
	if n.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (clearInteraction should clear per TS Npc.ts:407)", n.faceEntity)
	}
	if n.masks&rsbuf.NpcMaskFaceEntity == 0 {
		t.Error("masks & NpcMaskFaceEntity: got 0, want nonzero (clearInteraction should emit per TS Npc.ts:408)")
	}
}

// TestResetMasksTrailingClearFires — NAI-14 Task 2.
// When target is nil but faceEntity is still set, ResetMasks emits the
// entitymask bit and clears faceEntity. Mirrors TS
// PathingEntity.ts:611-614 with one-tick-lag deviation (Go's ResetMasks
// runs at tick end, so the mask bit is consumed by the next tick's
// info-pass).
func TestResetMasksTrailingClearFires(t *testing.T) {
	n := newTestNpc(1)
	n.target = nil
	n.faceEntity = 42
	n.masks = 0
	n.ResetMasks()
	if n.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (trailing clear should run)", n.faceEntity)
	}
	if n.masks&rsbuf.NpcMaskFaceEntity == 0 {
		t.Error("masks & NpcMaskFaceEntity: got 0, want nonzero (trailing clear should emit)")
	}
}

// TestResetMasksTrailingClearSkippedWhenTargetPresent — NAI-14 Task 2.
// Quirk guard: trailing clear must not fire when target is non-nil
// (the NPC is still facing someone, by design).
func TestResetMasksTrailingClearSkippedWhenTargetPresent(t *testing.T) {
	n := newTestNpc(1)
	other := newTestNpc(2)
	n.target = other
	n.faceEntity = 42
	n.masks = 0
	n.ResetMasks()
	if n.faceEntity != 42 {
		t.Errorf("faceEntity: got %d, want 42 (trailing clear should be skipped — target present)", n.faceEntity)
	}
	if n.masks&rsbuf.NpcMaskFaceEntity != 0 {
		t.Error("masks & NpcMaskFaceEntity: got nonzero, want 0 (trailing clear should not emit — target present)")
	}
}

// TestResetMasksTrailingClearSkippedWhenFaceEntityAlreadyMinusOne — NAI-14 Task 2.
// Quirk guard: trailing clear must not fire when faceEntity is already
// -1 (no pending clear to sync).
func TestResetMasksTrailingClearSkippedWhenFaceEntityAlreadyMinusOne(t *testing.T) {
	n := newTestNpc(1)
	n.target = nil
	n.faceEntity = -1
	n.masks = 0
	n.ResetMasks()
	if n.masks != 0 {
		t.Errorf("masks: got 0x%x, want 0 (trailing clear should be skipped — faceEntity already -1)", n.masks)
	}
}

// TestNpcDelayedAfterStepClearsStaleWalkDir pins NAI-157 root-cause fix:
// when an NPC walks at tick N and is script-delayed at tick N+1,
// processCleanup's resetPathingEntity call at the end of tick N clears the
// step dir so tick N+1's processInfo sees walkDir == -1.
//
// Without the fix, processMovementInteraction early-returns on
// n.delayed (npc_interaction.go:159-161), updateMovement never runs,
// and rsbuf receives the stale dir — visible client-side as the NPC
// continuing to walk through walls.
func TestNpcDelayedAfterStepClearsStaleWalkDir(t *testing.T) {
	n := newTestNpc(1)
	// Simulate updateMovement having set step dirs during tick N.
	n.walkDir = 4
	n.runDir = 5

	// End-of-tick cleanup runs (processCleanup → resetPathingEntity).
	n.resetPathingEntity()

	// Simulate the NPC being script-delayed for the next tick.
	n.delayed = true

	// processMovementInteraction would early-return on n.delayed next
	// tick, so updateMovement would not run. The dir state must already
	// be clear at this point so processInfo pushes -1/-1 to rsbuf.
	if n.walkDir != -1 || n.runDir != -1 {
		t.Fatalf("stale dir leaked past cleanup: walkDir=%d runDir=%d", n.walkDir, n.runDir)
	}
}

// TestNpc_ResetPathingEntity_ResetsWalkRunDir pins NAI-167: resetPathingEntity
// (mirroring TS PathingEntity.ts:579-580) resets walkDir/runDir. This is the
// new home of the NAI-157 walkDir/runDir reset (migrated from ResetMasks).
func TestNpc_ResetPathingEntity_ResetsWalkRunDir(t *testing.T) {
	n := newTestNpc(1)
	n.walkDir = 4
	n.runDir = 7

	n.resetPathingEntity()

	if n.walkDir != -1 {
		t.Errorf("walkDir: got %d, want -1", n.walkDir)
	}
	if n.runDir != -1 {
		t.Errorf("runDir: got %d, want -1", n.runDir)
	}
}

// TestNpc_ResetPathingEntity_AdvancesLastTickCoords pins NAI-167: resetPathingEntity
// advances lastTickX/Z/lastLevel to the current x/z/level at end-of-tick, mirroring
// TS PathingEntity.ts:583-585.
func TestNpc_ResetPathingEntity_AdvancesLastTickCoords(t *testing.T) {
	n := newTestNpc(1)
	n.x = 5
	n.z = 6
	n.level = 0
	n.lastTickX = -1
	n.lastTickZ = -1
	n.lastLevel = -1

	n.resetPathingEntity()

	if n.lastTickX != 5 {
		t.Errorf("lastTickX: got %d, want 5", n.lastTickX)
	}
	if n.lastTickZ != 6 {
		t.Errorf("lastTickZ: got %d, want 6", n.lastTickZ)
	}
	if n.lastLevel != 0 {
		t.Errorf("lastLevel: got %d, want 0", n.lastLevel)
	}
}

// TestNpc_ResetPathingEntity_ClearsTele pins NAI-167: resetPathingEntity clears
// n.tele at end-of-tick, mirroring TS PathingEntity.ts:582.
func TestNpc_ResetPathingEntity_ClearsTele(t *testing.T) {
	n := newTestNpc(1)
	n.tele = true

	n.resetPathingEntity()

	if n.tele {
		t.Errorf("tele: got true, want false")
	}
}

// TestNpc_ResetPathingEntity_ResetsStepsTaken pins NAI-167: resetPathingEntity
// zeroes stepsTaken at end-of-tick, mirroring TS PathingEntity.ts:586.
func TestNpc_ResetPathingEntity_ResetsStepsTaken(t *testing.T) {
	n := newTestNpc(1)
	n.stepsTaken = 5

	n.resetPathingEntity()

	if n.stepsTaken != 0 {
		t.Errorf("stepsTaken: got %d, want 0", n.stepsTaken)
	}
}

// TestNpc_DelayedNpc_GetsLastTickAdvancedAtCleanup is the regression pin for
// the delayed-NPC reset gap: a script-delayed NPC must still have its
// lastTickX/Z advanced at processCleanup. Pre-fix, lastTickX/Z were written
// inside processMovementInteraction which early-returns on n.delayed —
// so delayed NPCs silently kept stale lastTick coords indefinitely.
func TestNpc_DelayedNpc_GetsLastTickAdvancedAtCleanup(t *testing.T) {
	n := newTestNpc(1)
	n.delayed = true
	n.x = 5
	n.z = 6
	n.lastTickX = -1
	n.lastTickZ = -1

	// End-of-tick processCleanup calls resetPathingEntity unconditionally.
	n.resetPathingEntity()

	if n.lastTickX != 5 {
		t.Errorf("lastTickX: got %d, want 5 (delayed NPC reset gap regression)", n.lastTickX)
	}
	if n.lastTickZ != 6 {
		t.Errorf("lastTickZ: got %d, want 6 (delayed NPC reset gap regression)", n.lastTickZ)
	}
}

// === rev-244 B2 T13: damage2 + hitmarkSlot alternation (Npc fork) ===
//
// TS contract mirrors Player fork exactly: Npc.ts:475-494 (244).

// TestNpcDamage2AlternationSlot0SetsDamage pins that the first Damage call
// sets damageAmt/damageType + NpcMaskDamage (slot%2==0). TS Npc.ts:489-492.
func TestNpcDamage2AlternationSlot0SetsDamage(t *testing.T) {
	npc := npcWithHP(t, 20, 20)
	npc.Damage(5, 1) // slot 0 → DAMAGE
	if npc.masks&rsbuf.NpcMaskDamage == 0 {
		t.Error("slot 0: NpcMaskDamage should be set (TS Npc.ts:491)")
	}
	if npc.masks&rsbuf.NpcMaskDamage2 != 0 {
		t.Error("slot 0: NpcMaskDamage2 must NOT be set")
	}
	if npc.damageAmt != 5 {
		t.Errorf("slot 0: damageAmt: got %d, want 5", npc.damageAmt)
	}
	if npc.damage2Amt != -1 {
		t.Errorf("slot 0: damage2Amt: got %d, want -1 (untouched)", npc.damage2Amt)
	}
	if npc.hitmarkSlot != 1 {
		t.Errorf("slot 0: hitmarkSlot: got %d, want 1", npc.hitmarkSlot)
	}
}

// TestNpcDamage2AlternationSlot1SetsDamage2 pins that the second Damage call
// sets damage2Amt/damage2Type + NpcMaskDamage2 (slot%2==1). TS Npc.ts:484-488.
func TestNpcDamage2AlternationSlot1SetsDamage2(t *testing.T) {
	npc := npcWithHP(t, 20, 20)
	npc.Damage(5, 1) // slot 0 → DAMAGE
	npc.Damage(3, 2) // slot 1 → DAMAGE2
	if npc.masks&rsbuf.NpcMaskDamage2 == 0 {
		t.Error("slot 1: NpcMaskDamage2 should be set (TS Npc.ts:487)")
	}
	if npc.damage2Amt != 3 {
		t.Errorf("slot 1: damage2Amt: got %d, want 3", npc.damage2Amt)
	}
	if npc.damage2Type != 2 {
		t.Errorf("slot 1: damage2Type: got %d, want 2", npc.damage2Type)
	}
	if npc.hitmarkSlot != 2 {
		t.Errorf("slot 1: hitmarkSlot: got %d, want 2", npc.hitmarkSlot)
	}
}

// TestNpcDamage2AlternationSlot2OverwritesDamage pins slot-2 wrap-around:
// third call overwrites damageAmt (slot%2==0 again).
func TestNpcDamage2AlternationSlot2OverwritesDamage(t *testing.T) {
	npc := npcWithHP(t, 20, 20)
	npc.Damage(5, 1) // slot 0
	npc.Damage(3, 2) // slot 1
	npc.Damage(1, 3) // slot 2 → overwrites DAMAGE
	if npc.damageAmt != 1 {
		t.Errorf("slot 2: damageAmt: got %d, want 1 (overwritten)", npc.damageAmt)
	}
	if npc.damageType != 3 {
		t.Errorf("slot 2: damageType: got %d, want 3 (overwritten)", npc.damageType)
	}
	if npc.hitmarkSlot != 3 {
		t.Errorf("slot 2: hitmarkSlot: got %d, want 3", npc.hitmarkSlot)
	}
}

// TestNpcResetMasksClearsDamage2AndHitmarkSlot pins the per-tick reset for
// the Npc fork. TS PathingEntity.ts:606-610 (244).
func TestNpcResetMasksClearsDamage2AndHitmarkSlot(t *testing.T) {
	npc := npcWithHP(t, 20, 20)
	npc.Damage(5, 1) // slot 0
	npc.Damage(3, 2) // slot 1 → damage2 set
	npc.ResetMasks()
	if npc.damage2Amt != -1 {
		t.Errorf("damage2Amt after ResetMasks: got %d, want -1", npc.damage2Amt)
	}
	if npc.damage2Type != -1 {
		t.Errorf("damage2Type after ResetMasks: got %d, want -1", npc.damage2Type)
	}
	if npc.hitmarkSlot != 0 {
		t.Errorf("hitmarkSlot after ResetMasks: got %d, want 0 (TS PathingEntity.ts:610)", npc.hitmarkSlot)
	}
}

// TestNpcDamage2InitiallyMinusOne pins that NewNpc initialises damage2Amt=-1,
// damage2Type=-1, hitmarkSlot=0.
func TestNpcDamage2InitiallyMinusOne(t *testing.T) {
	npc := npcWithHP(t, 10, 10)
	if npc.damage2Amt != -1 {
		t.Errorf("damage2Amt initial: got %d, want -1", npc.damage2Amt)
	}
	if npc.damage2Type != -1 {
		t.Errorf("damage2Type initial: got %d, want -1", npc.damage2Type)
	}
	if npc.hitmarkSlot != 0 {
		t.Errorf("hitmarkSlot initial: got %d, want 0", npc.hitmarkSlot)
	}
}

// TestNpcDamage2AmtAccessorReturnsField pins that Damage2Amt()/Damage2Type()
// return the real damage2Amt/damage2Type fields.
func TestNpcDamage2AmtAccessorReturnsField(t *testing.T) {
	npc := npcWithHP(t, 20, 20)
	npc.Damage(5, 1) // slot 0
	npc.Damage(4, 7) // slot 1 → damage2Amt=4
	if npc.Damage2Amt() != 4 {
		t.Errorf("Damage2Amt(): got %d, want 4", npc.Damage2Amt())
	}
	if npc.Damage2Type() != 7 {
		t.Errorf("Damage2Type(): got %d, want 7", npc.Damage2Type())
	}
}

// TestNpc_StepsTakenResetEnablesReorientGate_AcrossTicks pins NAI-167: the
// reorient gate at npc_interaction.go:932 (`n.targetX != -1 && n.stepsTaken == 0`)
// was forever-broken because stepsTaken accumulated across ticks but was never
// reset. After resetPathingEntity runs at end-of-tick, stepsTaken is 0 at the
// start of tick N+1, so the gate fires correctly for a loc/obj target.
//
// Simulates: post-tick-N NPC state (stepsTaken=1 from a step) → resetPathingEntity
// (end of tick N) → reorient() (start of tick N+1) → targetX/Z cleared.
func TestNpc_StepsTakenResetEnablesReorientGate_AcrossTicks(t *testing.T) {
	s := newTestServer(t)
	n := makeInteractionNpc(t, s, 1, 100, 100, 0)

	// Post-tick-N state: NPC moved toward a loc target (stepsTaken=1).
	n.target = nil
	n.targetX = 999
	n.targetZ = 1001
	n.stepsTaken = 1

	// End-of-tick cleanup: resetPathingEntity zeroes stepsTaken.
	n.resetPathingEntity()

	if n.stepsTaken != 0 {
		t.Fatalf("resetPathingEntity: stepsTaken not cleared: got %d", n.stepsTaken)
	}

	// Start of tick N+1: reorient() should fire the gate (stepsTaken==0, targetX!=−1).
	n.reorient()

	if n.targetX != -1 || n.targetZ != -1 {
		t.Errorf("reorient gate did not fire: targetX/Z = (%d,%d), want (-1,-1)", n.targetX, n.targetZ)
	}
}

// TestChangeTypeRefreshesTypSnapshot verifies that ChangeType (CHANGETYPE
// path with reset=true) refreshes n.typ from the npcTypes registry, so
// post-changetype geometry reads (NAI-18 inApproachDistance LoS via
// n.typ.Size, future combat / wander reads) see the new type.
//
// Pre-NAI-19 bug: changeTypeImpl wrote n.typeId but did NOT reassign n.typ,
// leaving stale typ snapshots (see nai_followups.md § "From NAI-18 → Stale
// *Npc.typ snapshot after changetype").
func TestChangeTypeRefreshesTypSnapshot(t *testing.T) {
	s := newTestServer(t)
	sourceTyp := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}, Size: 1}
	morphTyp := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 8}, Size: 2}
	s.npcTypes = &objtype.NPCTypeConfigs{Configs: make([]*objtype.NpcType, 9)}
	s.npcTypes.Configs[7] = sourceTyp
	s.npcTypes.Configs[8] = morphTyp

	n := NewNpc(0, 7, 100, 100, 0, sourceTyp)
	n.server = s
	n.lifecycle = NpcLifecycleRespawn

	n.ChangeType(8, 50)

	if n.typ == nil {
		t.Fatal("n.typ: got nil, want morphTyp")
	}
	if n.typ.Size != 2 {
		t.Errorf("n.typ.Size: got %d, want 2 (post-changetype must reflect morphTyp)", n.typ.Size)
	}
	if n.typeId != 8 {
		t.Errorf("n.typeId: got %d, want 8", n.typeId)
	}
}

// TestChangeTypeKeepAllRefreshesTypSnapshot verifies that ChangeTypeKeepAll
// (KEEPALL path with reset=false) ALSO refreshes n.typ. The staleness bug
// affects both reset and keepall paths — geometry reads are
// reset-orthogonal.
func TestChangeTypeKeepAllRefreshesTypSnapshot(t *testing.T) {
	s := newTestServer(t)
	sourceTyp := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}, Size: 1}
	morphTyp := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 8}, Size: 3}
	s.npcTypes = &objtype.NPCTypeConfigs{Configs: make([]*objtype.NpcType, 9)}
	s.npcTypes.Configs[7] = sourceTyp
	s.npcTypes.Configs[8] = morphTyp

	n := NewNpc(0, 7, 100, 100, 0, sourceTyp)
	n.server = s
	n.lifecycle = NpcLifecycleRespawn

	n.ChangeTypeKeepAll(8, 50)

	if n.typ == nil {
		t.Fatal("n.typ: got nil, want morphTyp")
	}
	if n.typ.Size != 3 {
		t.Errorf("n.typ.Size: got %d, want 3 (KEEPALL path must also refresh)", n.typ.Size)
	}
}
