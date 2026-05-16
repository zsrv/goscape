// pkg/pack/compiler/cfg/pointer_checker_labels.go
package cfg

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/pointer"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// labelJumpCommands names the two commands that take a label-typed local
// parameter and jump to it (the dot variant is post-2009). Mirrors TS
// PointerChecker.LABEL_JUMP_COMMANDS.
var labelJumpCommands = map[string]struct{}{
	"jump":  {},
	".jump": {},
}

// argPushOpcodes are the opcodes whose operand pushes a value usable as a
// call argument. Mirrors TS PointerChecker.ARG_PUSH_OPCODES.
var argPushOpcodes = map[codegen.Opcode]struct{}{
	codegen.PushConstantInt:    {},
	codegen.PushConstantString: {},
	codegen.PushConstantLong:   {},
	codegen.PushConstantSymbol: {},
	codegen.PushLocalVar:       {},
	codegen.PushVar:            {},
	codegen.PushVar2:           {},
}

// buildStaticLabelArgsByCall scans script for Gosub/Jump calls where one or
// more label-typed parameters receive a static PushConstantSymbol argument.
// Returns the per-instruction param-index → label-symbol map. Mirrors TS
// buildStaticLabelArgsByCall.
func (p *PointerChecker) buildStaticLabelArgsByCall(script *codegen.RuneScript) map[*codegen.Instruction]map[int]symbol.Symbol {
	result := map[*codegen.Instruction]map[int]symbol.Symbol{}

	for _, b := range script.Blocks {
		insts := b.Instructions
		for i := range insts {
			inst := &insts[i]
			if inst.Opcode == codegen.LineNumber {
				continue
			}
			if inst.Opcode != codegen.Gosub && inst.Opcode != codegen.Jump {
				continue
			}
			sym, ok := inst.Operand.(symbol.Symbol)
			if !ok {
				continue
			}
			scriptSym, ok := sym.(*symbol.ServerScriptSymbol)
			if !ok {
				continue
			}
			paramTypes := typ.TupleToList(scriptSym.Parameters)
			if len(paramTypes) == 0 {
				continue
			}
			argPushes := collectArgumentPushes(insts, i, len(paramTypes))
			if argPushes == nil {
				continue
			}
			staticArgs := map[int]symbol.Symbol{}
			for paramIndex, paramType := range paramTypes {
				if !isLabelType(paramType) {
					continue
				}
				argInst := argPushes[paramIndex]
				if argInst.Opcode != codegen.PushConstantSymbol {
					continue
				}
				argSym, ok := argInst.Operand.(symbol.Symbol)
				if !ok {
					continue
				}
				if scriptArg, ok := argSym.(*symbol.ServerScriptSymbol); ok && scriptArg.Trigger != nil && scriptArg.Trigger.Identifier == "label" {
					staticArgs[paramIndex] = argSym
				}
			}
			if len(staticArgs) > 0 {
				result[inst] = staticArgs
			}
		}
	}

	return result
}

// collectArgumentPushes walks backward from callIndex-1 in insts and collects
// the `count` arg-push instructions in source order. Returns nil if any
// intervening instruction is not an arg-push (LineNumber is skipped).
func collectArgumentPushes(insts []codegen.Instruction, callIndex, count int) []*codegen.Instruction {
	if count <= 0 {
		return []*codegen.Instruction{}
	}
	var result []*codegen.Instruction
	for i := callIndex - 1; i >= 0 && len(result) < count; i-- {
		inst := &insts[i]
		if inst.Opcode == codegen.LineNumber {
			continue
		}
		if _, ok := argPushOpcodes[inst.Opcode]; !ok {
			return nil
		}
		result = append(result, inst)
	}
	if len(result) != count {
		return nil
	}
	// Reverse: callers want source order.
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}

// isLabelType reports whether t is a MetaScript whose trigger identifier
// is "label". Mirrors TS PointerChecker.isLabelType.
func isLabelType(t typ.Type) bool {
	if _, _, ok := typ.IsMetaScript(t); !ok {
		return false
	}
	ident, ok := typ.MetaScriptTriggerIdent(t)
	return ok && ident == "label"
}

// getJumpParamNodes returns the per-script map of param-index → []nodes for
// nodes that consist of `push_local_var(param) ; command(jump|.jump)`.
// Cached. Mirrors TS getJumpParamNodes.
func (p *PointerChecker) getJumpParamNodes(script *codegen.RuneScript) map[int][]*InstructionNode {
	if cached, ok := p.jumpParamNodesByScript[script.Symbol]; ok {
		return cached
	}

	nodeMap := map[*codegen.Instruction]*InstructionNode{}
	for _, n := range p.GetGraph(script) {
		if n.Instruction != nil {
			nodeMap[n.Instruction] = n
		}
	}

	paramIndexBySymbol := map[*symbol.LocalVariableSymbol]int{}
	if script.Locals != nil {
		for i, ps := range script.Locals.Parameters {
			paramIndexBySymbol[ps] = i
		}
	}

	out := map[int][]*InstructionNode{}
	for _, b := range script.Blocks {
		insts := b.Instructions
		for i := range insts {
			inst := &insts[i]
			if inst.Opcode == codegen.LineNumber {
				continue
			}
			if inst.Opcode != codegen.Command {
				continue
			}
			cmd, ok := inst.Operand.(symbol.Symbol)
			if !ok {
				continue
			}
			if _, ok := labelJumpCommands[cmd.SymbolName()]; !ok {
				continue
			}
			prev := previousNonLine(insts, i-1)
			if prev == nil || prev.Opcode != codegen.PushLocalVar {
				continue
			}
			local, ok := prev.Operand.(*symbol.LocalVariableSymbol)
			if !ok {
				continue
			}
			paramIndex, present := paramIndexBySymbol[local]
			if !present {
				continue
			}
			if script.Locals == nil || paramIndex >= len(script.Locals.Parameters) {
				continue
			}
			paramType := script.Locals.Parameters[paramIndex].Type
			if !isLabelType(paramType) {
				continue
			}
			node, present := nodeMap[inst]
			if !present {
				continue
			}
			out[paramIndex] = append(out[paramIndex], node)
		}
	}

	p.jumpParamNodesByScript[script.Symbol] = out
	return out
}

func previousNonLine(insts []codegen.Instruction, start int) *codegen.Instruction {
	for i := start; i >= 0; i-- {
		if insts[i].Opcode == codegen.LineNumber {
			continue
		}
		return &insts[i]
	}
	return nil
}

// requiresPointerAtNodes reports whether any node in nodes requires pt
// without first reaching the graph's start. Mirrors TS
// requiresPointerAtNodes.
func (p *PointerChecker) requiresPointerAtNodes(script *codegen.RuneScript, pt *pointer.PointerType, nodes []*InstructionNode) bool {
	if len(nodes) == 0 {
		return false
	}
	analysis := p.getAnalysis(script)
	i := pointer.Index(pt)
	return p.findEdgePath(nodes, func(n *InstructionNode) bool { return n == analysis.graph[0] }, analysis.setNodes[i]) != nil
}

// addStaticLabelRequirements files the static-label-arg requirements onto
// the caller node so calculatePointers picks them up. Mirrors TS.
func (p *PointerChecker) addStaticLabelRequirements(
	required [][]*InstructionNode,
	callerNode *InstructionNode,
	calledSym symbol.Symbol,
	staticArgs map[int]symbol.Symbol,
) {
	called, ok := p.scriptsBySymbol[calledSym]
	if !ok {
		return
	}
	jumpParamNodes := p.getJumpParamNodes(called)
	if len(jumpParamNodes) == 0 {
		return
	}
	for paramIndex, labelSym := range staticArgs {
		nodes := jumpParamNodes[paramIndex]
		if len(nodes) == 0 {
			continue
		}
		labelHolder := p.GetPointers(labelSym)
		for _, pt := range labelHolder.Required.All() {
			if p.requiresPointerAtNodes(called, pt, nodes) {
				i := pointer.Index(pt)
				required[i] = append(required[i], callerNode)
			}
		}
	}
}
