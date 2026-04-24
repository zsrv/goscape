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
