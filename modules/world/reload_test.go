package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestResizeVarShared_CountUnchanged_ReturnsInputs(t *testing.T) {
	oldVars := []int32{10, 20, 30}
	oldStrs := []string{"a", "b", "c"}
	cfgs := []*objtype.VarSharedType{
		{Type: objtype.ScriptVarTypeInt},
		{Type: objtype.ScriptVarTypeInt},
		{Type: objtype.ScriptVarTypeInt},
	}
	newVars, newStrs := resizeVarShared(oldVars, oldStrs, cfgs)
	// No allocation expected — pointer-identity check.
	if &newVars[0] != &oldVars[0] {
		t.Errorf("expected pass-through on count match (no realloc)")
	}
	if newStrs[0] != "a" || newStrs[2] != "c" {
		t.Errorf("strs not preserved on pass-through: %v", newStrs)
	}
}

func TestResizeVarShared_CountGrew_ClobbersAllNonStringSlots(t *testing.T) {
	oldVars := []int32{10, 20, 30}
	oldStrs := []string{"a", "b", "c"}
	cfgs := []*objtype.VarSharedType{
		{Type: objtype.ScriptVarTypeInt},  // i=0: was 10 → clobbered to 0
		{Type: objtype.ScriptVarTypeInt},  // i=1: was 20 → clobbered to 0
		{Type: objtype.ScriptVarTypeInt},  // i=2: was 30 → clobbered to 0
		{Type: objtype.ScriptVarTypeObj},  // i=3: net-new, OBJ default = -1
		{Type: objtype.ScriptVarTypeLoc},  // i=4: net-new, non-INT non-STRING default = -1
	}
	newVars, _ := resizeVarShared(oldVars, oldStrs, cfgs)
	want := []int32{0, 0, 0, -1, -1}
	for i, v := range want {
		if newVars[i] != v {
			t.Errorf("newVars[%d]: got %d, want %d (DEVIATION-NAI-190-D3-CANDIDATE clobber-after-copy)", i, newVars[i], v)
		}
	}
}

func TestResizeVarShared_StringType_KeepsCopiedValue(t *testing.T) {
	oldVars := []int32{0, 0, 0}
	oldStrs := []string{"hello", "world", "foo"}
	cfgs := []*objtype.VarSharedType{
		{Type: objtype.ScriptVarTypeString},
		{Type: objtype.ScriptVarTypeString},
		{Type: objtype.ScriptVarTypeString},
		{Type: objtype.ScriptVarTypeString}, // net-new STRING slot
	}
	_, newStrs := resizeVarShared(oldVars, oldStrs, cfgs)
	if newStrs[0] != "hello" || newStrs[1] != "world" || newStrs[2] != "foo" {
		t.Errorf("STRING slots clobbered: %v (expected [hello world foo \"\"])", newStrs)
	}
	if newStrs[3] != "" {
		t.Errorf("net-new STRING slot non-empty: %q", newStrs[3])
	}
}

func TestResizeVarShared_NilConfigSlot_Skipped(t *testing.T) {
	oldVars := []int32{10}
	oldStrs := []string{"x"}
	cfgs := []*objtype.VarSharedType{
		{Type: objtype.ScriptVarTypeInt},
		nil, // defensive
		{Type: objtype.ScriptVarTypeInt},
	}
	newVars, _ := resizeVarShared(oldVars, oldStrs, cfgs)
	if len(newVars) != 3 {
		t.Fatalf("newVars len: got %d, want 3", len(newVars))
	}
	if newVars[1] != 0 {
		t.Errorf("nil-config slot should remain zero: got %d", newVars[1])
	}
}
