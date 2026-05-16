// Package cfg ports TS src/compiler/codegen/script/config/ at HEAD
// b8c338801fbb72d294ff9576a58925a8d3f6de47. Provides control-flow-graph
// types (InstructionNode + PointerInstructionNode), the GraphGenerator that
// builds a CFG from a RuneScript's Blocks, and PointerChecker which
// validates entity-pointer flow over that CFG.
//
// NAI-208-D-PACKAGE-NAMES: TS path codegen/script/config/ → goscape cfg/
// (avoids deep nesting; mirrors NAI-207's flat codegen/ choice).
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
// is a stable map key.
//
// Instruction is nil for two synthetic node kinds:
//   - the GraphGenerator's start node prepended to every graph
//   - PointerInstructionNode injected for conditional-pointer set arcs
type InstructionNode struct {
	Instruction *codegen.Instruction
	Next        []*InstructionNode
	Previous    []*InstructionNode
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

// PointerInstructionNode is a synthetic node that records pointers
// explicitly set by a conditional-pointer-setter command. Injected by
// GraphGenerator on the conditional arc when a `command + push 1 +
// branch_equals` triple appears (or push-0+branch for the inverted form).
// Mirrors TS PointerInstructionNode.
//
// Embeds InstructionNode (not subclasses it). The embedded address is the
// identity used by PointerChecker analysis arrays — callers MUST reference
// &pn.InstructionNode (or pass *PointerInstructionNode through a
// *InstructionNode parameter, where field-promotion does this automatically).
type PointerInstructionNode struct {
	InstructionNode
	Set *pointer.PointerSet
}

// NewPointerInstructionNode constructs a synthetic node whose Set holds the
// pointers that the preceding conditional command marks as set on this arc.
func NewPointerInstructionNode(set *pointer.PointerSet) *PointerInstructionNode {
	return &PointerInstructionNode{Set: set}
}

// BaseNode returns the embedded *InstructionNode address — used by
// PointerChecker.getAnalysis to file the synthetic node in analysis maps
// under the same identity AddNext stores in Previous edges.
func (p *PointerInstructionNode) BaseNode() *InstructionNode {
	return &p.InstructionNode
}
