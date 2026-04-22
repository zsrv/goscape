package objtype

import (
	"testing"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

func TestHuntTypeDefaults(t *testing.T) {
	ht := NewHuntType(42)

	if ht.ID != 42 {
		t.Errorf("ID: got %d, want 42", ht.ID)
	}
	if ht.Type != HuntModeOff {
		t.Errorf("Type: got %d, want HuntModeOff (%d)", ht.Type, HuntModeOff)
	}
	if ht.CheckVis != HuntVisOff {
		t.Errorf("CheckVis: got %d, want HuntVisOff", ht.CheckVis)
	}
	if ht.CheckNotTooStrong != HuntCheckNotTooStrongOff {
		t.Errorf("CheckNotTooStrong: got %d, want HuntCheckNotTooStrongOff", ht.CheckNotTooStrong)
	}
	if ht.CheckNotBusy {
		t.Errorf("CheckNotBusy: got true, want false")
	}
	if ht.FindKeepHunting {
		t.Errorf("FindKeepHunting: got true, want false")
	}
	if ht.FindNewMode != NPCModeNull {
		t.Errorf("FindNewMode: got %d, want NPCModeNull (%d)", ht.FindNewMode, NPCModeNull)
	}
	if ht.NobodyNear != HuntNobodyNearPauseHunt {
		t.Errorf("NobodyNear: got %d, want HuntNobodyNearPauseHunt", ht.NobodyNear)
	}
	if ht.CheckNotCombat != -1 {
		t.Errorf("CheckNotCombat: got %d, want -1", ht.CheckNotCombat)
	}
	if ht.CheckNotCombatSelf != -1 {
		t.Errorf("CheckNotCombatSelf: got %d, want -1", ht.CheckNotCombatSelf)
	}
	if !ht.CheckAfk {
		t.Errorf("CheckAfk: got false, want true")
	}
	if ht.Rate != 1 {
		t.Errorf("Rate: got %d, want 1", ht.Rate)
	}
	for name, got := range map[string]int{
		"CheckCategory": ht.CheckCategory,
		"CheckNpc":      ht.CheckNpc,
		"CheckObj":      ht.CheckObj,
		"CheckLoc":      ht.CheckLoc,
		"CheckInv":      ht.CheckInv,
		"CheckObjParam": ht.CheckObjParam,
		"CheckInvVal":   ht.CheckInvVal,
	} {
		if got != -1 {
			t.Errorf("%s: got %d, want -1", name, got)
		}
	}
	if ht.CheckInvCondition != "" {
		t.Errorf("CheckInvCondition: got %q, want empty", ht.CheckInvCondition)
	}
	if ht.CheckVars != nil {
		t.Errorf("CheckVars: got %v, want nil", ht.CheckVars)
	}

	// Silence unused-import warning until Task 2 uses it.
	_ = packet2.NewPacket
}
