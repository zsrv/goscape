// pkg/pack/compiler/cfg/instruction_node_test.go
package cfg

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/pointer"
)

func TestInstructionNode_AddNextPopulatesBothEndpoints(t *testing.T) {
	a := NewInstructionNode(nil)
	b := NewInstructionNode(nil)

	a.AddNext(b)

	if len(a.Next) != 1 || a.Next[0] != b {
		t.Fatalf("a.Next = %v, want [b]", a.Next)
	}
	if len(b.Previous) != 1 || b.Previous[0] != a {
		t.Fatalf("b.Previous = %v, want [a]", b.Previous)
	}
}

func TestInstructionNode_InstructionFieldStored(t *testing.T) {
	inst := &codegen.Instruction{Opcode: codegen.Return}
	n := NewInstructionNode(inst)
	if n.Instruction != inst {
		t.Errorf("Instruction = %p, want %p", n.Instruction, inst)
	}
}

func TestInstructionNode_NilInstructionAllowed(t *testing.T) {
	n := NewInstructionNode(nil)
	if n.Instruction != nil {
		t.Error("nil Instruction should remain nil for synthetic start node")
	}
}

func TestPointerInstructionNode_CarriesSet(t *testing.T) {
	set := pointer.NewPointerSet(pointer.ActivePlayer)
	pn := NewPointerInstructionNode(set)

	if pn.Instruction != nil {
		t.Error("PointerInstructionNode.Instruction should be nil (synthetic)")
	}
	if !pn.Set.Has(pointer.ActivePlayer) {
		t.Error("PointerInstructionNode.Set lost ActivePlayer")
	}
}

func TestPointerInstructionNode_EmbedsInstructionNode(t *testing.T) {
	set := pointer.NewPointerSet()
	pn := NewPointerInstructionNode(set)
	other := NewInstructionNode(nil)
	pn.AddNext(other) // method promoted from InstructionNode

	if len(pn.Next) != 1 || pn.Next[0] != other {
		t.Error("AddNext promotion broken")
	}
	if len(other.Previous) != 1 || other.Previous[0] != &pn.InstructionNode {
		t.Errorf("AddNext sets Previous to embedded base address; got %p want %p", other.Previous[0], &pn.InstructionNode)
	}
}
