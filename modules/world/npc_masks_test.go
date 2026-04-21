package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

func npcWithHP(t *testing.T, maxHP, curHP int) *Npc {
	t.Helper()
	typ := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 7, DebugName: "rat"},
	}
	// NpcType.Stats[3] is the Hitpoints slot (npcStatHitpoints=3 in
	// pkg/objtype/npctype.go). Stats is []uint16.
	typ.Stats = make([]uint16, 6)
	typ.Stats[3] = uint16(maxHP)
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
	if npc.damageAmt != 5 {
		t.Errorf("damageAmt: got %d, want 5 (actual requested amount, not floored)", npc.damageAmt)
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
