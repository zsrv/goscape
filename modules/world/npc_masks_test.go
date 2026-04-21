package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

// npcWithHP builds an Npc whose NpcType.Stats[NpcStatHitpoints] = maxHP,
// then overrides curHP if needed. NewNpc seeds both curHP and baseHP from
// Stats[NpcStatHitpoints] as of S6d, so the override is only meaningful
// when the caller wants a starting curHP distinct from max.
func npcWithHP(t *testing.T, maxHP, curHP int) *Npc {
	t.Helper()
	typ := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 7, DebugName: "rat"},
	}
	typ.Stats = []uint16{0, 0, 0, uint16(maxHP), 0, 0}
	npc := NewNpc(0, 7, 3222, 3218, 0, typ)
	npc.curHP = curHP
	return npc
}

func TestNpcDamageDecrementsHPAndSetsMask(t *testing.T) {
	npc := npcWithHP(t, 10, 10)
	npc.Damage(3, 1)
	if npc.curHP != 7 {
		t.Errorf("curHP: got %d, want 7", npc.curHP)
	}
	if npc.baseHP != 10 {
		t.Errorf("baseHP: got %d, want 10", npc.baseHP)
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
	if npc.curHP != 0 {
		t.Errorf("curHP: got %d, want 0 (clamped)", npc.curHP)
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
	if npc.curHP != 10 {
		t.Errorf("curHP: got %d, want 10 (negative amount must not heal)", npc.curHP)
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
	if npc.curHP != 20 {
		t.Errorf("curHP: got %d, want 20", npc.curHP)
	}
	if npc.baseHP != 20 {
		t.Errorf("baseHP: got %d, want 20", npc.baseHP)
	}
}

func TestNewNpcWithEmptyStatsSeedsZeroHP(t *testing.T) {
	// &NpcType{} has nil Stats, so initialHP returns 0.
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}}
	npc := NewNpc(0, 7, 3222, 3218, 0, typ)
	if npc.curHP != 0 || npc.baseHP != 0 {
		t.Errorf("curHP/baseHP: got %d/%d, want 0/0", npc.curHP, npc.baseHP)
	}
}

func TestNpcDamagePersistsAcrossResetMasks(t *testing.T) {
	npc := npcWithHP(t, 10, 10)
	npc.Damage(3, 1)
	if npc.curHP != 7 {
		t.Fatalf("pre-reset curHP: got %d, want 7", npc.curHP)
	}
	npc.ResetMasks()
	if npc.curHP != 7 {
		t.Errorf("post-reset curHP: got %d, want 7 (persistent)", npc.curHP)
	}
	if npc.damageAmt != -1 {
		t.Errorf("damageAmt: got %d, want -1 (per-tick cleared)", npc.damageAmt)
	}
}

func TestNpcBaseHPPersistsAcrossResetMasks(t *testing.T) {
	npc := npcWithHP(t, 10, 10)
	npc.Damage(3, 1)
	npc.ResetMasks()
	if npc.baseHP != 10 {
		t.Errorf("post-reset baseHP: got %d, want 10 (persistent)", npc.baseHP)
	}
}

func TestNpcResetHP(t *testing.T) {
	npc := npcWithHP(t, 10, 10)
	npc.Damage(7, 1)
	if npc.curHP != 3 {
		t.Fatalf("pre-reset curHP: got %d, want 3", npc.curHP)
	}
	npc.ResetHP()
	if npc.curHP != 10 {
		t.Errorf("curHP after ResetHP: got %d, want 10", npc.curHP)
	}
	if npc.baseHP != 10 {
		t.Errorf("baseHP after ResetHP: got %d, want 10", npc.baseHP)
	}
}

func TestNpcResetHPWithNilTypDirectConstruction(t *testing.T) {
	// NewNpc would panic on nil typ (PatrolCoord / WanderRange / etc.), so
	// build *Npc directly to exercise the initialHP nil-guard path via
	// ResetHP. Any future caller that manually constructs an Npc must
	// survive ResetHP cleanly.
	npc := &Npc{}
	npc.ResetHP()
	if npc.curHP != 0 || npc.baseHP != 0 {
		t.Errorf("after ResetHP on nil-typ npc: got %d/%d, want 0/0", npc.curHP, npc.baseHP)
	}
}
