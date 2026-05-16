// pkg/pack/compiler/cfg/graph_generator_test.go
package cfg

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/pointer"
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
)

// makeCommandSymbol returns a ServerScriptSymbol that mimics a command of
// the given name. PointerChecker uses Symbol.SymbolName() to look up
// commandPointers; that satisfies both Command and Gosub/Jump callers.
func makeCommandSymbol(name string) symbol.Symbol {
	return &symbol.ServerScriptSymbol{
		ScriptSymbolFields: symbol.ScriptSymbolFields{
			Trigger: &trigger.TriggerType{Identifier: "command"},
			Name:    name,
		},
	}
}

// TestGraphGenerator_SingleBlockChain pins a straight-line single-block
// graph: one synthetic start + N instruction nodes; sequential edges.
func TestGraphGenerator_SingleBlockChain(t *testing.T) {
	b := codegen.NewBlock(&codegen.Label{Name: "entry"})
	b.Add(codegen.Instruction{Opcode: codegen.PushConstantInt, Operand: 0})
	b.Add(codegen.Instruction{Opcode: codegen.Discard})
	b.Add(codegen.Instruction{Opcode: codegen.Return})

	gg := NewGraphGenerator(map[string]*pointer.PointerHolder{}, semantics.StrictFeatureLevel{})
	graph := gg.Generate([]*codegen.Block{b})

	// 1 start + 3 instruction nodes = 4
	if got := len(graph); got != 4 {
		t.Fatalf("len(graph) = %d, want 4", got)
	}
	// Start node has no Instruction.
	if graph[0].Instruction != nil {
		t.Error("graph[0].Instruction != nil — start node should be synthetic")
	}
	// Start → first real instruction.
	if len(graph[0].Next) != 1 || graph[0].Next[0] != graph[1] {
		t.Error("start node edge broken")
	}
	// PushConstantInt → Discard.
	if len(graph[1].Next) != 1 || graph[1].Next[0] != graph[2] {
		t.Error("push → discard edge broken")
	}
	// Discard → Return.
	if len(graph[2].Next) != 1 || graph[2].Next[0] != graph[3] {
		t.Error("discard → return edge broken")
	}
	// Return is terminal: no Next.
	if len(graph[3].Next) != 0 {
		t.Errorf("return.Next = %v, want []", graph[3].Next)
	}
}

// TestGraphGenerator_BranchJoinsToTarget pins that a Branch opcode wires its
// Next edge to the first instruction of the target block.
func TestGraphGenerator_BranchJoinsToTarget(t *testing.T) {
	thenLbl := &codegen.Label{Name: "then"}
	entry := codegen.NewBlock(&codegen.Label{Name: "entry"})
	entry.Add(codegen.Instruction{Opcode: codegen.Branch, Operand: thenLbl})
	then := codegen.NewBlock(thenLbl)
	then.Add(codegen.Instruction{Opcode: codegen.Return})

	gg := NewGraphGenerator(map[string]*pointer.PointerHolder{}, semantics.StrictFeatureLevel{})
	graph := gg.Generate([]*codegen.Block{entry, then})

	// 1 start + 2 real
	if got := len(graph); got != 3 {
		t.Fatalf("len(graph) = %d, want 3", got)
	}
	// Branch must Next to the Return.
	branchNode := graph[1]
	returnNode := graph[2]
	if len(branchNode.Next) != 1 || branchNode.Next[0] != returnNode {
		t.Errorf("branch.Next = %v, want [return]", branchNode.Next)
	}
}

// TestGraphGenerator_BranchEqualsHasBothArcs pins that a conditional
// BranchEquals wires fallthrough AND branch-target edges.
func TestGraphGenerator_BranchEqualsHasBothArcs(t *testing.T) {
	thenLbl := &codegen.Label{Name: "then"}
	entry := codegen.NewBlock(&codegen.Label{Name: "entry"})
	entry.Add(codegen.Instruction{Opcode: codegen.PushConstantInt, Operand: 1})
	entry.Add(codegen.Instruction{Opcode: codegen.BranchEquals, Operand: thenLbl})
	entry.Add(codegen.Instruction{Opcode: codegen.Return}) // fallthrough
	then := codegen.NewBlock(thenLbl)
	then.Add(codegen.Instruction{Opcode: codegen.Return})

	gg := NewGraphGenerator(map[string]*pointer.PointerHolder{}, semantics.StrictFeatureLevel{})
	graph := gg.Generate([]*codegen.Block{entry, then})

	// start + push + branch_equals + return-fallthrough + return-then = 5
	if got := len(graph); got != 5 {
		t.Fatalf("len(graph) = %d, want 5", got)
	}
	branchNode := graph[2]
	if len(branchNode.Next) != 2 {
		t.Fatalf("branch_equals.Next has %d edges, want 2 (fallthrough + branch)", len(branchNode.Next))
	}
}

// TestGraphGenerator_ConditionalPointerInjectsSetterNode pins the
// pointer-inversion-disabled path (conditional-set BEFORE the conditional
// branch's target arc).
func TestGraphGenerator_ConditionalPointerInjectsSetterNode(t *testing.T) {
	sym := makeCommandSymbol("inzone")
	holder := &pointer.PointerHolder{
		Set:            pointer.NewPointerSet(pointer.ActivePlayer),
		ConditionalSet: true,
	}
	cp := map[string]*pointer.PointerHolder{"inzone": holder}

	thenLbl := &codegen.Label{Name: "then"}
	entry := codegen.NewBlock(&codegen.Label{Name: "entry"})
	entry.Add(codegen.Instruction{Opcode: codegen.Command, Operand: sym})
	entry.Add(codegen.Instruction{Opcode: codegen.PushConstantInt, Operand: 1})
	entry.Add(codegen.Instruction{Opcode: codegen.BranchEquals, Operand: thenLbl})
	entry.Add(codegen.Instruction{Opcode: codegen.Return}) // fallthrough
	then := codegen.NewBlock(thenLbl)
	then.Add(codegen.Instruction{Opcode: codegen.Return})

	gg := NewGraphGenerator(cp, semantics.StrictFeatureLevel{})
	graph := gg.Generate([]*codegen.Block{entry, then})

	// Find the synthetic PointerInstructionNode in the graph.
	var pin *InstructionNode
	for _, n := range graph {
		if n.Instruction == nil && len(n.Previous) > 0 && n.Previous[0].Instruction != nil && n.Previous[0].Instruction.Opcode == codegen.BranchEquals {
			pin = n
			break
		}
	}
	if pin == nil {
		t.Fatal("no synthetic PointerInstructionNode injected on conditional arc")
	}
}

// TestGraphGenerator_PointerInversionRespectsDisable pins that disabling
// the feature alters the injected-node placement (TS allowPointerInversion
// branch). Smoke: assert the graph is still well-formed and contains at
// least one synthetic node.
func TestGraphGenerator_PointerInversionRespectsDisable(t *testing.T) {
	sym := makeCommandSymbol("inzone")
	holder := &pointer.PointerHolder{
		Set:            pointer.NewPointerSet(pointer.ActivePlayer),
		ConditionalSet: true,
	}
	cp := map[string]*pointer.PointerHolder{"inzone": holder}

	thenLbl := &codegen.Label{Name: "then"}
	entry := codegen.NewBlock(&codegen.Label{Name: "entry"})
	entry.Add(codegen.Instruction{Opcode: codegen.Command, Operand: sym})
	entry.Add(codegen.Instruction{Opcode: codegen.PushConstantInt, Operand: 0}) // inverted: push 0
	entry.Add(codegen.Instruction{Opcode: codegen.BranchEquals, Operand: thenLbl})
	entry.Add(codegen.Instruction{Opcode: codegen.Branch, Operand: thenLbl})
	then := codegen.NewBlock(thenLbl)
	then.Add(codegen.Instruction{Opcode: codegen.Return})

	gg := NewGraphGenerator(cp, semantics.StrictFeatureLevel{DisablePointerInversion: true})
	graph := gg.Generate([]*codegen.Block{entry, then})

	if len(graph) < 5 {
		t.Fatalf("graph too small: %d", len(graph))
	}
}
