// pkg/pack/compiler/cfg/pointer_checker_core_test.go
package cfg

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/pointer"
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
)

// newReturnOnlyScript builds a minimal RuneScript: trigger=proc, single
// block "entry" containing only a Return instruction. Used for the
// no-pointer-state baseline.
func newReturnOnlyScript(name string) *codegen.RuneScript {
	tr := &trigger.TriggerType{ID: 0, Identifier: "proc"}
	sym := &symbol.ServerScriptSymbol{
		ScriptSymbolFields: symbol.ScriptSymbolFields{
			Trigger: tr,
			Name:    name,
		},
	}
	rs := codegen.NewRuneScript("test.rs2", sym, tr, name, nil)
	b := codegen.NewBlock(&codegen.Label{Name: "entry"})
	b.Add(codegen.Instruction{Opcode: codegen.Return})
	rs.Blocks = []*codegen.Block{b}
	return rs
}

func TestPointerChecker_GetPointersOnReturnOnly_Empty(t *testing.T) {
	rs := newReturnOnlyScript("p1")
	d := &diagnostics.Diagnostics{}
	pc := NewPointerChecker(d, []*codegen.RuneScript{rs}, map[string]*pointer.PointerHolder{}, semantics.StrictFeatureLevel{})

	h := pc.GetPointers(rs.Symbol)
	if h.Required.Len() != 0 {
		t.Errorf("Required.Len = %d, want 0", h.Required.Len())
	}
	if h.Set.Len() != 0 {
		t.Errorf("Set.Len = %d, want 0", h.Set.Len())
	}
	if h.Corrupted.Len() != 0 {
		t.Errorf("Corrupted.Len = %d, want 0", h.Corrupted.Len())
	}
	if h.ConditionalSet {
		t.Error("ConditionalSet should be false")
	}
}

func TestPointerChecker_GetPointersCaches(t *testing.T) {
	rs := newReturnOnlyScript("p1")
	d := &diagnostics.Diagnostics{}
	pc := NewPointerChecker(d, []*codegen.RuneScript{rs}, map[string]*pointer.PointerHolder{}, semantics.StrictFeatureLevel{})

	first := pc.GetPointers(rs.Symbol)
	second := pc.GetPointers(rs.Symbol)
	if first != second {
		t.Error("GetPointers returned a fresh holder on second call (cache miss)")
	}
}

func TestPointerChecker_GetGraphCaches(t *testing.T) {
	rs := newReturnOnlyScript("p1")
	d := &diagnostics.Diagnostics{}
	pc := NewPointerChecker(d, []*codegen.RuneScript{rs}, map[string]*pointer.PointerHolder{}, semantics.StrictFeatureLevel{})

	g1 := pc.GetGraph(rs)
	g2 := pc.GetGraph(rs)
	if &g1[0] != &g2[0] {
		t.Error("GetGraph returned a fresh slice on second call (cache miss)")
	}
}

// TestPointerChecker_CommandRequiresPropagates pins that a Command whose
// holder says required={ActivePlayer} bubbles up to the script's required
// set.
func TestPointerChecker_CommandRequiresPropagates(t *testing.T) {
	tr := &trigger.TriggerType{ID: 0, Identifier: "proc"}
	sym := &symbol.ServerScriptSymbol{
		ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: tr, Name: "p1"},
	}
	rs := codegen.NewRuneScript("test.rs2", sym, tr, "p1", nil)
	b := codegen.NewBlock(&codegen.Label{Name: "entry"})
	cmdSym := makeCommandSymbol("p_kickout")
	b.Add(codegen.Instruction{Opcode: codegen.Command, Operand: cmdSym})
	b.Add(codegen.Instruction{Opcode: codegen.Return})
	rs.Blocks = []*codegen.Block{b}

	cp := map[string]*pointer.PointerHolder{
		"p_kickout": {Required: pointer.NewPointerSet(pointer.ActivePlayer)},
	}
	d := &diagnostics.Diagnostics{}
	pc := NewPointerChecker(d, []*codegen.RuneScript{rs}, cp, semantics.StrictFeatureLevel{})

	h := pc.GetPointers(rs.Symbol)
	if !h.Required.Has(pointer.ActivePlayer) {
		t.Errorf("Required = %v, want has ActivePlayer", h.Required.All())
	}
}

// TestPointerChecker_RecursiveGosubHandled pins the recursion guard: A
// gosubs B which gosubs A — both calls must terminate without recursing.
func TestPointerChecker_RecursiveGosubHandled(t *testing.T) {
	tr := &trigger.TriggerType{ID: 0, Identifier: "proc"}
	symA := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: tr, Name: "a"}}
	symB := &symbol.ServerScriptSymbol{ScriptSymbolFields: symbol.ScriptSymbolFields{Trigger: tr, Name: "b"}}

	rsA := codegen.NewRuneScript("test.rs2", symA, tr, "a", nil)
	bA := codegen.NewBlock(&codegen.Label{Name: "entry"})
	bA.Add(codegen.Instruction{Opcode: codegen.Gosub, Operand: symB})
	bA.Add(codegen.Instruction{Opcode: codegen.Return})
	rsA.Blocks = []*codegen.Block{bA}

	rsB := codegen.NewRuneScript("test.rs2", symB, tr, "b", nil)
	bB := codegen.NewBlock(&codegen.Label{Name: "entry"})
	bB.Add(codegen.Instruction{Opcode: codegen.Gosub, Operand: symA})
	bB.Add(codegen.Instruction{Opcode: codegen.Return})
	rsB.Blocks = []*codegen.Block{bB}

	d := &diagnostics.Diagnostics{}
	pc := NewPointerChecker(d, []*codegen.RuneScript{rsA, rsB}, map[string]*pointer.PointerHolder{}, semantics.StrictFeatureLevel{})

	// Must terminate, not stack-overflow.
	_ = pc.GetPointers(symA)
	_ = pc.GetPointers(symB)
}

func TestPointerChecker_SetsPointerTriggerDefault_NilPointers(t *testing.T) {
	rs := newReturnOnlyScript("p1") // trigger.Pointers == nil
	d := &diagnostics.Diagnostics{}
	pc := NewPointerChecker(d, []*codegen.RuneScript{rs}, map[string]*pointer.PointerHolder{}, semantics.StrictFeatureLevel{})

	if pc.SetsPointerTrigger(rs, pointer.ActivePlayer) {
		t.Error("trigger.Pointers=nil should not set any pointer")
	}
}

func TestPointerChecker_SetsPointerTriggerDefault_TriggerSets(t *testing.T) {
	rs := newReturnOnlyScript("p1")
	rs.Trigger.Pointers = pointer.NewPointerSet(pointer.ActivePlayer)
	d := &diagnostics.Diagnostics{}
	pc := NewPointerChecker(d, []*codegen.RuneScript{rs}, map[string]*pointer.PointerHolder{}, semantics.StrictFeatureLevel{})

	if !pc.SetsPointerTrigger(rs, pointer.ActivePlayer) {
		t.Error("trigger.Pointers has ActivePlayer; want true")
	}
	if pc.SetsPointerTrigger(rs, pointer.ActiveNpc) {
		t.Error("trigger.Pointers lacks ActiveNpc; want false")
	}
}

// TestPointerChecker_FindEdgePath_EmptyStarts pins that an empty starts
// slice returns nil immediately.
func TestPointerChecker_FindEdgePath_EmptyStarts(t *testing.T) {
	rs := newReturnOnlyScript("p1")
	d := &diagnostics.Diagnostics{}
	pc := NewPointerChecker(d, []*codegen.RuneScript{rs}, map[string]*pointer.PointerHolder{}, semantics.StrictFeatureLevel{})

	if p := pc.findEdgePath(nil, func(*InstructionNode) bool { return true }, map[*InstructionNode]struct{}{}); p != nil {
		t.Errorf("findEdgePath(nil) = %v, want nil", p)
	}
}

// TestPointerChecker_FindEdgePath_ReachableEnd pins the happy path.
func TestPointerChecker_FindEdgePath_ReachableEnd(t *testing.T) {
	a := NewInstructionNode(nil)
	b := NewInstructionNode(nil)
	c := NewInstructionNode(nil)
	a.AddNext(b)
	b.AddNext(c)

	rs := newReturnOnlyScript("p1")
	d := &diagnostics.Diagnostics{}
	pc := NewPointerChecker(d, []*codegen.RuneScript{rs}, map[string]*pointer.PointerHolder{}, semantics.StrictFeatureLevel{})

	// Walk backward from c through b to a.
	path := pc.findEdgePath([]*InstructionNode{c}, func(n *InstructionNode) bool { return n == a }, map[*InstructionNode]struct{}{})
	if len(path) == 0 || path[len(path)-1] != a {
		t.Errorf("path tail = %v, want a", path)
	}
	if path[0] != c {
		t.Errorf("path head = %v, want c", path[0])
	}
}

// TestPointerChecker_FindEdgePath_BlockedAll pins that all-blocked walks
// return nil.
func TestPointerChecker_FindEdgePath_BlockedAll(t *testing.T) {
	a := NewInstructionNode(nil)
	b := NewInstructionNode(nil)
	a.AddNext(b)

	rs := newReturnOnlyScript("p1")
	d := &diagnostics.Diagnostics{}
	pc := NewPointerChecker(d, []*codegen.RuneScript{rs}, map[string]*pointer.PointerHolder{}, semantics.StrictFeatureLevel{})

	blocked := map[*InstructionNode]struct{}{a: {}}
	path := pc.findEdgePath([]*InstructionNode{b}, func(*InstructionNode) bool { return true }, blocked)
	if path != nil {
		t.Errorf("path = %v, want nil (only neighbour was blocked)", path)
	}
}
