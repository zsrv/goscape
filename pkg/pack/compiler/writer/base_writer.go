// pkg/pack/compiler/writer/base_writer.go
package writer

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// OpcodeWriter is the visitor surface for one binary writer. Each method
// corresponds to one TS BaseScriptWriter abstract protected method
// (BaseScriptWriter.ts L150-206). Implementations live in
// pkg/pack/compiler/runescript/binary_writer.go (and any future text writer).
//
// NAI-209-D-OPCODE-WRITER-INTERFACE: TS abstract-class virtual dispatch →
// Go interface + free-function WriteScript.
type OpcodeWriter interface {
	EnterBlock(block *codegen.Block)

	WritePushConstantInt(value int32)
	WritePushConstantString(value string)
	WritePushConstantLong(value int64)
	WritePushConstantSymbol(sym symbol.Symbol)
	WritePushLocalVar(sym *symbol.LocalVariableSymbol)
	WritePopLocalVar(sym *symbol.LocalVariableSymbol)
	WritePushVar(sym *symbol.BasicSymbol, dot bool)
	WritePopVar(sym *symbol.BasicSymbol, dot bool)
	WriteDefineArray(sym *symbol.LocalVariableSymbol)
	WriteSwitch(table *codegen.SwitchTable)
	WriteBranch(opcode codegen.Opcode, label *codegen.Label)
	WriteJoinString(count int)
	WriteDiscard(baseType typ.BaseVarType)
	WriteJump(sym symbol.Symbol)
	WriteGosub(sym symbol.Symbol)
	WriteCommand(sym symbol.Symbol)
	WriteReturn()
	WriteMath(opcode codegen.Opcode)
}

// WriteScript drives a script's block list through w, mirroring TS
// BaseScriptWriter.write (L25-39). CurIndex on ctx is incremented AFTER each
// per-opcode method returns — WriteBranch and WriteSwitch read CurIndex to
// compute relative jumps and depend on this ordering.
func WriteScript(w OpcodeWriter, ctx *BaseContext, script *codegen.RuneScript) {
	for _, block := range script.Blocks {
		w.EnterBlock(block)
		for _, ins := range block.Instructions {
			dispatch(w, ins)
			ctx.CurIndex++
		}
	}
}

// dispatch resolves one instruction to its matching writer method. Mirrors
// TS BaseScriptWriter.writeInstruction (L55-148) one-for-one.
func dispatch(w OpcodeWriter, ins codegen.Instruction) {
	switch ins.Opcode {
	case codegen.PushConstantInt:
		w.WritePushConstantInt(toInt32(ins.Operand))
	case codegen.PushConstantString:
		w.WritePushConstantString(ins.Operand.(string))
	case codegen.PushConstantLong:
		w.WritePushConstantLong(ins.Operand.(int64))
	case codegen.PushConstantSymbol:
		w.WritePushConstantSymbol(ins.Operand.(symbol.Symbol))
	case codegen.PushLocalVar:
		w.WritePushLocalVar(ins.Operand.(*symbol.LocalVariableSymbol))
	case codegen.PopLocalVar:
		w.WritePopLocalVar(ins.Operand.(*symbol.LocalVariableSymbol))
	case codegen.PushVar:
		w.WritePushVar(ins.Operand.(*symbol.BasicSymbol), false)
	case codegen.PushVar2:
		w.WritePushVar(ins.Operand.(*symbol.BasicSymbol), true)
	case codegen.PopVar:
		w.WritePopVar(ins.Operand.(*symbol.BasicSymbol), false)
	case codegen.PopVar2:
		w.WritePopVar(ins.Operand.(*symbol.BasicSymbol), true)
	case codegen.DefineArray:
		w.WriteDefineArray(ins.Operand.(*symbol.LocalVariableSymbol))
	case codegen.Switch:
		w.WriteSwitch(ins.Operand.(*codegen.SwitchTable))
	case codegen.Branch,
		codegen.BranchNot, codegen.BranchEquals,
		codegen.BranchLessThan, codegen.BranchGreaterThan,
		codegen.BranchLessThanOrEquals, codegen.BranchGreaterThanOrEquals,
		codegen.LongBranchNot, codegen.LongBranchEquals,
		codegen.LongBranchLessThan, codegen.LongBranchGreaterThan,
		codegen.LongBranchLessThanOrEquals, codegen.LongBranchGreaterThanOrEquals,
		codegen.ObjBranchNot, codegen.ObjBranchEquals:
		w.WriteBranch(ins.Opcode, ins.Operand.(*codegen.Label))
	case codegen.JoinString:
		w.WriteJoinString(toInt(ins.Operand))
	case codegen.Discard:
		w.WriteDiscard(ins.Operand.(typ.BaseVarType))
	case codegen.Gosub:
		w.WriteGosub(ins.Operand.(symbol.Symbol))
	case codegen.Jump:
		w.WriteJump(ins.Operand.(symbol.Symbol))
	case codegen.Command:
		w.WriteCommand(ins.Operand.(symbol.Symbol))
	case codegen.Return:
		w.WriteReturn()
	case codegen.Add, codegen.Sub, codegen.Multiply, codegen.Divide,
		codegen.Modulo, codegen.Or, codegen.And,
		codegen.LongAdd, codegen.LongSub, codegen.LongMultiply,
		codegen.LongDivide, codegen.LongModulo, codegen.LongOr, codegen.LongAnd:
		w.WriteMath(ins.Opcode)
	case codegen.LineNumber:
		panic("writer: LineNumber opcode should not exist at write time")
	default:
		panic("writer: unknown opcode " + ins.Opcode.Name)
	}
}

// toInt32 accepts the codegen-side untyped int operand for PushConstantInt.
// codegen emits Go int values; cast to int32 (binary writer encodes 4 bytes).
// Rejects int64 — PushConstantLong has its own operand kind, and silent
// narrowing here would corrupt binary output.
func toInt32(v any) int32 {
	switch x := v.(type) {
	case int:
		return int32(x)
	case int32:
		return x
	}
	panic("writer: PushConstantInt operand is not an int-like value")
}

// toInt accepts the codegen-side untyped int operand for JoinString count.
func toInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int32:
		return int(x)
	}
	panic("writer: JoinString count is not an int-like value")
}
