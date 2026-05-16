// pkg/pack/compiler/cfg/graph_generator.go
package cfg

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/pointer"
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
)

// terminalOpcodes are opcodes that do not fall through to the next
// instruction. Mirrors TS TERMINAL_OPCODES (GraphGenerator.ts L256).
var terminalOpcodes = map[codegen.Opcode]struct{}{
	codegen.Branch: {},
	codegen.Jump:   {},
	codegen.Return: {},
}

// branchOpcodes are opcodes whose label operand is a jump target. Mirrors
// TS BRANCH_OPCODES (GraphGenerator.ts L258-274).
var branchOpcodes = map[codegen.Opcode]struct{}{
	codegen.Branch:                        {},
	codegen.BranchNot:                     {},
	codegen.BranchEquals:                  {},
	codegen.BranchLessThan:                {},
	codegen.BranchGreaterThan:             {},
	codegen.BranchLessThanOrEquals:        {},
	codegen.BranchGreaterThanOrEquals:     {},
	codegen.LongBranchNot:                 {},
	codegen.LongBranchEquals:              {},
	codegen.LongBranchLessThan:            {},
	codegen.LongBranchGreaterThan:         {},
	codegen.LongBranchLessThanOrEquals:    {},
	codegen.LongBranchGreaterThanOrEquals: {},
	codegen.ObjBranchNot:                  {},
	codegen.ObjBranchEquals:               {},
}

// GraphGenerator turns a RuneScript's Blocks into a CFG of InstructionNodes.
// Mirrors TS GraphGenerator (GraphGenerator.ts).
type GraphGenerator struct {
	commandPointers       map[string]*pointer.PointerHolder
	allowPointerInversion bool
}

// NewGraphGenerator constructs a GraphGenerator. commandPointers is keyed
// by command name (Symbol.SymbolName()). allowPointerInversion is the
// inverse of features.DisablePointerInversion (TS reads
// features.pointerInversion !== false).
func NewGraphGenerator(
	commandPointers map[string]*pointer.PointerHolder,
	features semantics.StrictFeatureLevel,
) *GraphGenerator {
	return &GraphGenerator{
		commandPointers:       commandPointers,
		allowPointerInversion: !features.DisablePointerInversion,
	}
}

// Generate builds the CFG. Returns nodes in insertion order: the synthetic
// start node first, then each instruction node in block order. Returns
// nil for an empty block list. Mirrors TS GraphGenerator.generate.
func (g *GraphGenerator) Generate(blocks []*codegen.Block) []*InstructionNode {
	if len(blocks) == 0 {
		return nil
	}

	nodeCache := map[*codegen.Instruction]*InstructionNode{}
	var nodes []*InstructionNode

	labelToBlock := map[*codegen.Label]*codegen.Block{}
	blockIndex := map[*codegen.Block]int{}
	firstValidByBlock := map[*codegen.Block]*codegen.Instruction{}

	for i, b := range blocks {
		blockIndex[b] = i
		labelToBlock[b.Label] = b
		firstValidByBlock[b] = firstValidInstruction(b.Instructions)
	}

	start := NewInstructionNode(nil)
	start.AddNext(g.firstInstruction(blocks[0], nodeCache, blocks, blockIndex, firstValidByBlock))
	nodes = append(nodes, start)

	potentialConditionalPointer := false

	for blockIdx, b := range blocks {
		for instIdx := range b.Instructions {
			inst := &b.Instructions[instIdx]

			if inst.Opcode == codegen.LineNumber {
				continue
			}

			node := getOrCreate(nodeCache, inst)
			nodes = append(nodes, node)

			if potentialConditionalPointer && inst.Opcode == codegen.BranchEquals && g.checkInvertedConditional(b.Instructions, instIdx) {
				// Inverted pointer-set: command + push 0 + branch_equals.
				if g.allowPointerInversion {
					if instIdx+1 >= len(b.Instructions) {
						panic("graph_generator: invalid inverted conditional layout")
					}
					next := &b.Instructions[instIdx+1]
					nextNode := getOrCreate(nodeCache, next)
					if next.Opcode != codegen.Branch {
						panic("graph_generator: expected Branch opcode after inverted conditional")
					}
					commandInst := &b.Instructions[instIdx-2]
					if commandInst.Opcode != codegen.Command {
						panic("graph_generator: expected Command before inverted conditional")
					}
					commandName := commandInst.Operand.(symbol.Symbol).SymbolName()
					holder := g.commandPointers[commandName]
					if holder == nil {
						panic(fmt.Sprintf("graph_generator: missing commandPointers for %q", commandName))
					}
					pin := NewPointerInstructionNode(holder.Set)
					nodes = append(nodes, pin)
					node.AddNext(pin)
					pin.AddNext(nextNode)
				} else if _, terminal := terminalOpcodes[inst.Opcode]; !terminal {
					var next *codegen.Instruction
					switch {
					case instIdx+1 < len(b.Instructions):
						next = &b.Instructions[instIdx+1]
					case blockIdx+1 < len(blocks):
						next = &blocks[blockIdx+1].Instructions[0]
					default:
						panic("graph_generator: no next instruction (inversion disabled fallback)")
					}
					node.AddNext(getOrCreate(nodeCache, next))
				}
				potentialConditionalPointer = false
			} else if _, terminal := terminalOpcodes[inst.Opcode]; !terminal {
				var next *codegen.Instruction
				switch {
				case instIdx+1 < len(b.Instructions):
					next = &b.Instructions[instIdx+1]
				case blockIdx+1 < len(blocks):
					next = &blocks[blockIdx+1].Instructions[0]
				default:
					panic("graph_generator: no next instruction (fallthrough)")
				}
				node.AddNext(getOrCreate(nodeCache, next))
			}

			if potentialConditionalPointer && inst.Opcode == codegen.BranchEquals && g.checkConditional(b.Instructions, instIdx) {
				// Non-inverted pointer-set: command + push 1 + branch_equals.
				lbl := inst.Operand.(*codegen.Label)
				jumpBlock := labelToBlock[lbl]
				if jumpBlock == nil {
					panic("graph_generator: unknown label on conditional pointer arc")
				}
				commandInst := &b.Instructions[instIdx-2]
				if commandInst.Opcode != codegen.Command {
					panic("graph_generator: expected Command before conditional pointer arc")
				}
				commandName := commandInst.Operand.(symbol.Symbol).SymbolName()
				holder := g.commandPointers[commandName]
				if holder == nil {
					panic(fmt.Sprintf("graph_generator: missing commandPointers for %q", commandName))
				}
				pin := NewPointerInstructionNode(holder.Set)
				nodes = append(nodes, pin)
				node.AddNext(pin)
				pin.AddNext(g.firstInstruction(jumpBlock, nodeCache, blocks, blockIndex, firstValidByBlock))

				potentialConditionalPointer = false
			} else if _, ok := branchOpcodes[inst.Opcode]; ok {
				lbl := inst.Operand.(*codegen.Label)
				jumpBlock := labelToBlock[lbl]
				if jumpBlock == nil {
					panic("graph_generator: unknown label on branch")
				}
				node.AddNext(g.firstInstruction(jumpBlock, nodeCache, blocks, blockIndex, firstValidByBlock))
			} else if inst.Opcode == codegen.Switch {
				table := inst.Operand.(*codegen.SwitchTable)
				for _, c := range table.Cases() {
					if len(c.Keys) == 0 {
						continue
					}
					jumpBlock := labelToBlock[c.Label]
					if jumpBlock == nil {
						panic("graph_generator: unknown label on switch")
					}
					node.AddNext(g.firstInstruction(jumpBlock, nodeCache, blocks, blockIndex, firstValidByBlock))
				}
			}

			if g.isConditionalPointerSetter(inst) {
				potentialConditionalPointer = true
			}
		}
	}

	return nodes
}

// checkConditional pins TS GraphGenerator.checkConditional: at index i, the
// instruction at i-2 must be a conditional-pointer-setter command and the
// instruction at i-1 must be `push 1`.
func (g *GraphGenerator) checkConditional(instructions []codegen.Instruction, i int) bool {
	if i < 2 {
		return false
	}
	if !g.isConditionalPointerSetter(&instructions[i-2]) {
		return false
	}
	prev := instructions[i-1]
	return prev.Opcode == codegen.PushConstantInt && operandIntEquals(prev.Operand, 1)
}

func (g *GraphGenerator) checkInvertedConditional(instructions []codegen.Instruction, i int) bool {
	if i < 2 {
		return false
	}
	if !g.isConditionalPointerSetter(&instructions[i-2]) {
		return false
	}
	prev := instructions[i-1]
	return prev.Opcode == codegen.PushConstantInt && operandIntEquals(prev.Operand, 0)
}

// isConditionalPointerSetter returns true when inst is a Command whose
// symbol resolves to a PointerHolder with ConditionalSet=true.
func (g *GraphGenerator) isConditionalPointerSetter(inst *codegen.Instruction) bool {
	if inst.Opcode != codegen.Command {
		return false
	}
	sym, ok := inst.Operand.(symbol.Symbol)
	if !ok {
		return false
	}
	holder := g.commandPointers[sym.SymbolName()]
	return holder != nil && holder.ConditionalSet
}

func (g *GraphGenerator) firstInstruction(
	b *codegen.Block,
	cache map[*codegen.Instruction]*InstructionNode,
	blocks []*codegen.Block,
	blockIndex map[*codegen.Block]int,
	firstValidByBlock map[*codegen.Block]*codegen.Instruction,
) *InstructionNode {
	if first := firstValidByBlock[b]; first != nil {
		return getOrCreate(cache, first)
	}
	startIdx, ok := blockIndex[b]
	if !ok {
		panic("graph_generator: block index not found")
	}
	for i := startIdx; i < len(blocks); i++ {
		if first := firstValidByBlock[blocks[i]]; first != nil {
			return getOrCreate(cache, first)
		}
	}
	panic("graph_generator: no instructions remaining")
}

func firstValidInstruction(insts []codegen.Instruction) *codegen.Instruction {
	for i := range insts {
		if insts[i].Opcode != codegen.LineNumber {
			return &insts[i]
		}
	}
	return nil
}

func getOrCreate(cache map[*codegen.Instruction]*InstructionNode, inst *codegen.Instruction) *InstructionNode {
	if node, ok := cache[inst]; ok {
		return node
	}
	node := NewInstructionNode(inst)
	cache[inst] = node
	return node
}

func operandIntEquals(operand any, want int) bool {
	switch v := operand.(type) {
	case int:
		return v == want
	case int32:
		return int(v) == want
	case int64:
		return int(v) == want
	default:
		return false
	}
}
