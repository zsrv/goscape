package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/inventory"
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

func TestReconcileInvs_Shared_RebuildsFreshFromType(t *testing.T) {
	sentinel := &inventory.Inventory{} // distinguishable from FromType output
	serverInvs := map[int]*inventory.Inventory{42: sentinel}
	invTypes := &objtype.InvTypeConfigs{
		Configs: makeInvConfigs(5, map[int]int{3: objtype.InvTypeScopeShared}),
	}
	fresh := reconcileInvs(serverInvs, nil, invTypes)
	if _, ok := fresh[42]; ok {
		t.Errorf("sentinel at id 42 leaked through clear")
	}
	if fresh[3] == sentinel {
		t.Errorf("SHARED id 3 not replaced with fresh inv (still sentinel)")
	}
	if fresh[3] == nil {
		t.Errorf("SHARED id 3 missing fresh inv")
	}
}

func TestReconcileInvs_Temp_DeletesFromAllPlayers(t *testing.T) {
	sentinel := &inventory.Inventory{}
	p1 := &Player{invs: map[int]*inventory.Inventory{7: sentinel}}
	p2 := &Player{invs: map[int]*inventory.Inventory{7: sentinel}}
	players := []*Player{nil, p1, p2} // index 0 is nil per goscape's slot-1-indexed convention
	invTypes := &objtype.InvTypeConfigs{
		Configs: makeInvConfigs(10, map[int]int{7: objtype.InvTypeScopeTemp}),
	}
	_ = reconcileInvs(nil, players, invTypes)
	if _, ok := p1.invs[7]; ok {
		t.Errorf("p1.invs[7] should be deleted")
	}
	if _, ok := p2.invs[7]; ok {
		t.Errorf("p2.invs[7] should be deleted")
	}
}

func TestReconcileInvs_Perm_LeftUntouched(t *testing.T) {
	sentinel := &inventory.Inventory{}
	p1 := &Player{invs: map[int]*inventory.Inventory{9: sentinel}}
	invTypes := &objtype.InvTypeConfigs{
		Configs: makeInvConfigs(10, map[int]int{9: objtype.InvTypeScopePerm}),
	}
	_ = reconcileInvs(nil, []*Player{p1}, invTypes)
	if p1.invs[9] != sentinel {
		t.Errorf("SCOPE_PERM inv reconciled (should be untouched)")
	}
}

func TestReconcileInvs_NilInvTypes_ReturnsEmptyMap(t *testing.T) {
	fresh := reconcileInvs(nil, nil, nil)
	if fresh == nil {
		t.Fatal("expected empty non-nil map, got nil")
	}
	if len(fresh) != 0 {
		t.Errorf("expected empty map, got %d entries", len(fresh))
	}
}

// makeInvConfigs builds a []*objtype.InvType of size n with default
// InvTypeScopePerm, overriding specific ids per the scopes map.
func makeInvConfigs(n int, scopes map[int]int) []*objtype.InvType {
	configs := make([]*objtype.InvType, n)
	for i := 0; i < n; i++ {
		configs[i] = &objtype.InvType{Scope: objtype.InvTypeScopePerm}
	}
	for id, scope := range scopes {
		configs[id].Scope = scope
	}
	return configs
}
