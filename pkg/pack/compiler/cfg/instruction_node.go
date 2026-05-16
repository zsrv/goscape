// Package cfg ports TS src/compiler/codegen/script/config/ at HEAD
// b8c338801fbb72d294ff9576a58925a8d3f6de47. Provides control-flow-graph
// types (InstructionNode), the GraphGenerator that builds a CFG from a
// RuneScript's Blocks, and PointerChecker which validates entity-pointer
// flow over that CFG.
//
// NAI-208-D-PACKAGE-NAMES: TS path codegen/script/config/ → goscape cfg/
// (avoids deep nesting; mirrors NAI-207's flat codegen/ choice).
//
// T5 inlines the PointerSet field onto InstructionNode (rather than using a
// separate PointerInstructionNode subtype) to keep the graph slice
// homogeneous. NewPointerInstructionNode is retained as a named constructor
// so call-site intent stays explicit. Compare TS PointerInstructionNode
// subclass.
package cfg

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/pointer"
)

// InstructionNode is one CFG node wrapping a single Instruction. The Next
// and Previous edges form a directed graph. Mirrors TS InstructionNode.
//
// Instruction is *codegen.Instruction pointing into a Block.Instructions
// slice element. Per NAI-208-D-INSTRUCTION-POINTER-KEY, post-codegen
// Block.Instructions slices are not appended to, so &block.Instructions[i]
// is a stable map key for the cfg pass.
//
// Instruction is nil for two synthetic node kinds:
//   - the GraphGenerator's start node prepended to every graph
//   - synthetic pointer-set nodes injected on conditional-pointer-set arcs
//     (PointerSet is non-nil for these)
//
// PointerSet is set on synthetic pointer-instruction nodes carrying the set
// of pointers a conditional-pointer-setter command marks on this arc.
// GraphGenerator injects these via NewPointerInstructionNode;
// cfg.PointerChecker consumes them in getAnalysis. Inlining the field onto
// InstructionNode (rather than using a separate subtype + interface) keeps
// the graph slice homogeneous. Compare TS PointerInstructionNode subclass.
type InstructionNode struct {
	Instruction *codegen.Instruction
	Next        []*InstructionNode
	Previous    []*InstructionNode
	PointerSet  *pointer.PointerSet
}

// NewInstructionNode constructs an InstructionNode wrapping inst (which may
// be nil for synthetic nodes).
func NewInstructionNode(inst *codegen.Instruction) *InstructionNode {
	return &InstructionNode{Instruction: inst}
}

// AddNext appends other to n.Next and n to other.Previous. Mirrors TS
// InstructionNode.addNext.
func (n *InstructionNode) AddNext(other *InstructionNode) {
	n.Next = append(n.Next, other)
	other.Previous = append(other.Previous, n)
}

// NewPointerInstructionNode returns a synthetic node with PointerSet set.
// Kept as a constructor (not a type) so callers' intent stays explicit.
func NewPointerInstructionNode(set *pointer.PointerSet) *InstructionNode {
	return &InstructionNode{PointerSet: set}
}
