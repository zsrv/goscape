// pkg/pack/compiler/writer/helpers.go
package writer

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// GenerateLineNumberTable walks the script's instruction stream and returns
// (pc → source-line) for every instruction that introduces a *new* source
// line (TS skips runs of same-line instructions). Mirrors TS
// BaseScriptWriter.generateLineNumberTable L219-235.
//
// NAI-209-D-LINENUMBER-ORDER-SLICE: also returns a []int of pcs in insertion
// order, since Go map iteration is randomized but BinaryScriptWriterContext.Finish
// must emit entries in pc-ascending order for byte-identical output.
func GenerateLineNumberTable(script *codegen.RuneScript) (map[int]int, []int) {
	tbl := map[int]int{}
	var order []int
	index := 0
	prevLine := -1
	for _, block := range script.Blocks {
		for _, ins := range block.Instructions {
			line := ins.Source.Line
			if line != 0 && line != prevLine {
				tbl[index] = line
				order = append(order, index)
				prevLine = line
			}
			index++
		}
	}
	return tbl, order
}

// GenerateJumpTable returns label → pc-of-its-first-instruction for every
// Block. Mirrors TS BaseScriptWriter.generateJumpTable L243-251.
func GenerateJumpTable(script *codegen.RuneScript) map[*codegen.Label]int {
	tbl := map[*codegen.Label]int{}
	index := 0
	for _, block := range script.Blocks {
		tbl[block.Label] = index
		index += len(block.Instructions)
	}
	return tbl
}

// GetParameterCount returns the number of parameters in locals whose
// Type.BaseType matches baseType. Mirrors TS getParameterCount L262-264.
func GetParameterCount(locals *codegen.LocalTable, baseType typ.BaseVarType) int {
	n := 0
	for _, p := range locals.Parameters {
		if bt, ok := p.Type.BaseType(); ok && bt == baseType {
			n++
		}
	}
	return n
}

// GetLocalCount returns the number of locals (including parameters) of
// baseType, excluding ArrayType locals UNLESS they are parameters.
// Mirrors TS getLocalCount L269-271.
func GetLocalCount(locals *codegen.LocalTable, baseType typ.BaseVarType) int {
	n := 0
	for _, v := range locals.All {
		bt, ok := v.Type.BaseType()
		if !ok || bt != baseType {
			continue
		}
		if _, isArr := v.Type.(*typ.ArrayType); isArr && !containsLocal(locals.Parameters, v) {
			continue
		}
		n++
	}
	return n
}

// GetVariableId returns the unique runtime ID for local within its locals
// table. The ID is the index among locals of the *same* slot-shape
// (array-vs-scalar; scalar further partitioned by BaseVarType, with
// non-parameter arrays excluded from the scalar pool).
//
// Returns -1 if local is not a member of locals.All; callers must treat -1
// as a programming-error sentinel — it would corrupt the binary output if
// emitted as an opcode operand.
//
// Mirrors TS getVariableId L276-282.
func GetVariableId(locals *codegen.LocalTable, local *symbol.LocalVariableSymbol) int {
	if _, isArr := local.Type.(*typ.ArrayType); isArr {
		n := 0
		for _, v := range locals.All {
			if v == local {
				return n
			}
			if _, isArr := v.Type.(*typ.ArrayType); isArr {
				n++
			}
		}
		return -1
	}
	bt, _ := local.Type.BaseType()
	n := 0
	for _, v := range locals.All {
		if v == local {
			return n
		}
		vbt, ok := v.Type.BaseType()
		if !ok || vbt != bt {
			continue
		}
		if _, isArr := v.Type.(*typ.ArrayType); isArr && !containsLocal(locals.Parameters, v) {
			continue
		}
		n++
	}
	return -1
}

func containsLocal(xs []*symbol.LocalVariableSymbol, target *symbol.LocalVariableSymbol) bool {
	for _, x := range xs {
		if x == target {
			return true
		}
	}
	return false
}
