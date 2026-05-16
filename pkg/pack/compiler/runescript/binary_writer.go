// pkg/pack/compiler/runescript/binary_writer.go
package runescript

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
	"github.com/zsrv/goscape/pkg/pack/compiler/writer"
)

// BinaryOutput is the abstract hook for the file-output sink. TS uses an
// abstract `outputScript(script, data)` method on BinaryScriptWriter; goscape
// hoists it to an interface field so concrete sinks (binary file, JagFile,
// etc.) can be injected without subclassing.
//
// NAI-209-D-BINARYOUTPUT-INTERFACE.
type BinaryOutput interface {
	OutputScript(script *codegen.RuneScript, data []byte)
}

// BinaryScriptWriter implements writer.OpcodeWriter and produces a binary
// script blob for each RuneScript handed to Write(). Mirrors TS
// BinaryScriptWriter.ts.
type BinaryScriptWriter struct {
	IdProvider writer.IdProvider
	Output     BinaryOutput

	ctx *BinaryScriptWriterContext // set per Write() call
}

// Compile-time check that BinaryScriptWriter satisfies OpcodeWriter.
var _ writer.OpcodeWriter = (*BinaryScriptWriter)(nil)

// NewBinaryScriptWriter constructs a writer bound to idp + output.
func NewBinaryScriptWriter(idp writer.IdProvider, output BinaryOutput) *BinaryScriptWriter {
	return &BinaryScriptWriter{IdProvider: idp, Output: output}
}

// Write is the public entry-point: compute the lookup key, drive the
// dispatch through writer.WriteScript, then call Finish + emit via Output.
func (b *BinaryScriptWriter) Write(script *codegen.RuneScript) {
	b.ctx = NewBinaryScriptWriterContext(script, b.generateLookupKey(script))
	writer.WriteScript(b, b.ctx.BaseContext, script)
	data := b.ctx.Finish()
	if b.Output != nil {
		b.Output.OutputScript(script, data)
	}
}

// =============================================================================
// OpcodeWriter implementation
// =============================================================================

func (b *BinaryScriptWriter) EnterBlock(*codegen.Block) {} // TS L87-89 NO-OP

func (b *BinaryScriptWriter) WritePushConstantInt(value int32) {
	b.ctx.Instruction(writer.OpPushConstantInt, value)
}

func (b *BinaryScriptWriter) WritePushConstantString(value string) {
	b.ctx.InstructionString(writer.OpPushConstantString, value)
}

func (b *BinaryScriptWriter) WritePushConstantLong(int64) {
	panic("BinaryScriptWriter: PushConstantLong not supported") // NAI-209-D-PUSHLONG-PANIC
}

// WritePushConstantSymbol handles three arms (TS L103-120):
//   - LocalVariableSymbol → variable id
//   - BasicSymbol whose Type is MetaType.Type → take the inner type's char code
//   - any other → IdProvider.Get
func (b *BinaryScriptWriter) WritePushConstantSymbol(sym symbol.Symbol) {
	var id int32
	switch s := sym.(type) {
	case *symbol.LocalVariableSymbol:
		id = int32(writer.GetVariableId(b.ctx.Script.Locals, s))
	case *symbol.BasicSymbol:
		if inner, ok := typ.IsMetaWrapping(s.Type); ok {
			code, hasCode := inner.Code()
			if !hasCode || code == "" {
				panic(fmt.Sprintf("BinaryScriptWriter: MetaType.Type inner %v has no char code", inner))
			}
			id = int32(code[0])
		} else {
			id = int32(b.IdProvider.Get(sym))
		}
	default:
		id = int32(b.IdProvider.Get(sym))
	}
	b.ctx.Instruction(writer.OpPushConstantInt, id)
}

func (b *BinaryScriptWriter) WritePushLocalVar(s *symbol.LocalVariableSymbol) {
	id := int32(writer.GetVariableId(b.ctx.Script.Locals, s))
	op := localVarOpcode(s, true)
	b.ctx.Instruction(op, id)
}

func (b *BinaryScriptWriter) WritePopLocalVar(s *symbol.LocalVariableSymbol) {
	id := int32(writer.GetVariableId(b.ctx.Script.Locals, s))
	op := localVarOpcode(s, false)
	b.ctx.Instruction(op, id)
}

func localVarOpcode(s *symbol.LocalVariableSymbol, push bool) *writer.ServerScriptOpcode {
	if _, isArr := s.Type.(*typ.ArrayType); isArr {
		if push {
			return writer.OpPushArrayInt
		}
		return writer.OpPopArrayInt
	}
	bt, _ := s.Type.BaseType()
	switch bt {
	case typ.BaseVarString:
		if push {
			return writer.OpPushStringLocal
		}
		return writer.OpPopStringLocal
	case typ.BaseVarInteger:
		if push {
			return writer.OpPushIntLocal
		}
		return writer.OpPopIntLocal
	}
	panic(fmt.Sprintf("BinaryScriptWriter: unsupported local variable type %v", s.Type))
}

func (b *BinaryScriptWriter) WritePushVar(s *symbol.BasicSymbol, dot bool) {
	id := b.IdProvider.Get(s)
	op := varOpcode(s, true)
	operand := int32(id)
	if dot {
		operand += 1 << 16
	}
	b.ctx.Instruction(op, operand)
}

func (b *BinaryScriptWriter) WritePopVar(s *symbol.BasicSymbol, dot bool) {
	id := b.IdProvider.Get(s)
	op := varOpcode(s, false)
	operand := int32(id)
	if dot {
		operand += 1 << 16
	}
	b.ctx.Instruction(op, operand)
}

func varOpcode(s *symbol.BasicSymbol, push bool) *writer.ServerScriptOpcode {
	switch s.Type.(type) {
	case *typ.VarPlayerType:
		if push {
			return writer.OpPushVarp
		}
		return writer.OpPopVarp
	case *typ.VarBitType:
		if push {
			return writer.OpPushVarbit
		}
		return writer.OpPopVarbit
	case *typ.VarNpcType:
		if push {
			return writer.OpPushVarn
		}
		return writer.OpPopVarn
	case *typ.VarSharedType:
		if push {
			return writer.OpPushVars
		}
		return writer.OpPopVars
	}
	panic(fmt.Sprintf("BinaryScriptWriter: unsupported variable type %v", s.Type))
}

func (b *BinaryScriptWriter) WriteDefineArray(s *symbol.LocalVariableSymbol) {
	id := writer.GetVariableId(b.ctx.Script.Locals, s)
	arr, ok := s.Type.(*typ.ArrayType)
	if !ok {
		panic(fmt.Sprintf("BinaryScriptWriter: WriteDefineArray on non-ArrayType %v", s.Type))
	}
	code, hasCode := arr.Inner().Code()
	if !hasCode || code == "" {
		panic(fmt.Sprintf("BinaryScriptWriter: ArrayType inner %v has no char code", arr.Inner()))
	}
	operand := int32((id << 16) | int(code[0]))
	b.ctx.Instruction(writer.OpDefineArray, operand)
}

func (b *BinaryScriptWriter) WriteSwitch(table *codegen.SwitchTable) {
	b.ctx.Switch(table.ID, func() int {
		total := 0
		for _, c := range table.Cases() {
			jumpLocation, ok := b.ctx.JumpTable[c.Label]
			if !ok {
				panic(fmt.Sprintf("BinaryScriptWriter: label %q not in jump table", c.Label.Name))
			}
			relativeJump := int32(jumpLocation - b.ctx.CurIndex - 1)
			for _, key := range c.Keys {
				b.ctx.SwitchCase(b.resolveSwitchKey(key), relativeJump)
				total++
			}
		}
		return total
	})
}

// resolveSwitchKey ports TS BinaryScriptWriter.findCaseKeyValue (L239-249).
// Numeric keys flow through; symbol keys are mapped via IdProvider.
func (b *BinaryScriptWriter) resolveSwitchKey(key any) int32 {
	switch v := key.(type) {
	case int:
		return int32(v)
	case int32:
		return v
	case symbol.Symbol:
		return int32(b.IdProvider.Get(v))
	}
	panic(fmt.Sprintf("BinaryScriptWriter: unsupported switch key %T", key))
}

func (b *BinaryScriptWriter) WriteBranch(opcode codegen.Opcode, label *codegen.Label) {
	op, ok := branchOpcode(opcode)
	if !ok {
		panic(fmt.Sprintf("BinaryScriptWriter: unsupported branch opcode %s", opcode.Name))
	}
	jumpLocation, ok := b.ctx.JumpTable[label]
	if !ok {
		panic(fmt.Sprintf("BinaryScriptWriter: label %q not in jump table", label.Name))
	}
	operand := int32(jumpLocation - b.ctx.CurIndex - 1)
	b.ctx.Instruction(op, operand)
}

// branchOpcode maps a codegen.Opcode to the matching ServerScriptOpcode.
// Returns (nil, false) for non-branch inputs. Mirrors TS L251-278.
//
// NAI-209-D-LONGBRANCH-OBJBRANCH-PANIC: LongBranch*/ObjBranch* opcodes are
// emitted by codegen but have no ServerScriptOpcode mapping (TS throws the
// same `Unsupported opcode` error). Reaching this case at runtime indicates
// a script using long or obj comparison expressions, which the current
// binary format does not encode.
func branchOpcode(opcode codegen.Opcode) (*writer.ServerScriptOpcode, bool) {
	switch opcode {
	case codegen.Branch:
		return writer.OpBranch, true
	case codegen.BranchNot:
		return writer.OpBranchNot, true
	case codegen.BranchEquals:
		return writer.OpBranchEquals, true
	case codegen.BranchLessThan:
		return writer.OpBranchLessThan, true
	case codegen.BranchGreaterThan:
		return writer.OpBranchGreaterThan, true
	case codegen.BranchLessThanOrEquals:
		return writer.OpBranchLessThanOrEquals, true
	case codegen.BranchGreaterThanOrEquals:
		return writer.OpBranchGreaterThanOrEquals, true
	}
	return nil, false
}

func (b *BinaryScriptWriter) WriteJoinString(count int) {
	b.ctx.Instruction(writer.OpJoinString, int32(count))
}

func (b *BinaryScriptWriter) WriteDiscard(baseType typ.BaseVarType) {
	switch baseType {
	case typ.BaseVarInteger:
		b.ctx.Instruction(writer.OpPopIntDiscard, 0)
	case typ.BaseVarString:
		b.ctx.Instruction(writer.OpPopStringDiscard, 0)
	default:
		panic(fmt.Sprintf("BinaryScriptWriter: unsupported discard base type %v", baseType))
	}
}

func (b *BinaryScriptWriter) WriteGosub(sym symbol.Symbol) {
	id := int32(b.IdProvider.Get(sym))
	b.ctx.Instruction(writer.OpGosubWithParams, id)
}

func (b *BinaryScriptWriter) WriteJump(sym symbol.Symbol) {
	id := int32(b.IdProvider.Get(sym))
	b.ctx.Instruction(writer.OpJumpWithParams, id)
}

// WriteCommand mirrors TS L319-326: emit InstructionRaw with the command's
// IdProvider id as the opcode and 1/0 secondary flag based on the leading dot.
func (b *BinaryScriptWriter) WriteCommand(sym symbol.Symbol) {
	op := b.IdProvider.Get(sym)
	if op == -1 {
		panic(fmt.Sprintf("BinaryScriptWriter: missing opcode id for command %q", sym.SymbolName()))
	}
	secondary := 0
	if strings.HasPrefix(sym.SymbolName(), ".") {
		secondary = 1
	}
	b.ctx.InstructionRaw(op, secondary)
}

func (b *BinaryScriptWriter) WriteReturn() {
	b.ctx.Instruction(writer.OpReturn, 0)
}

func (b *BinaryScriptWriter) WriteMath(opcode codegen.Opcode) {
	op := mathOpcode(opcode)
	if op == nil {
		panic(fmt.Sprintf("BinaryScriptWriter: unsupported math opcode %s", opcode.Name))
	}
	b.ctx.Instruction(op, 0)
}

// mathOpcode maps a codegen.Opcode to the matching ServerScriptOpcode.
// Returns nil for unknown inputs. Mirrors TS L335-358.
//
// NAI-209-D-LONGMATH-PANIC: LongAdd/LongSub/LongMultiply/LongDivide/LongModulo/
// LongOr/LongAnd are emitted by codegen but have no ServerScriptOpcode mapping
// (TS throws the same `Unsupported math opcode` error). Reaching this case at
// runtime indicates a script using long arithmetic, which the current binary
// format does not encode.
func mathOpcode(opcode codegen.Opcode) *writer.ServerScriptOpcode {
	switch opcode {
	case codegen.Add:
		return writer.OpAdd
	case codegen.Sub:
		return writer.OpSub
	case codegen.Multiply:
		return writer.OpMultiply
	case codegen.Divide:
		return writer.OpDivide
	case codegen.Modulo:
		return writer.OpModulo
	case codegen.Or:
		return writer.OpOr
	case codegen.And:
		return writer.OpAnd
	}
	return nil
}

// generateLookupKey ports TS BinaryScriptWriter.generateLookupKey (L58-85).
//
// Three arms:
//   - SubjectMode.Name           → -1
//   - SubjectMode.Type + subject → trigger.ID + (typeMarker<<8) + (subjectId<<10)
//   - otherwise (Mode.None)      → trigger.ID
//
// subjectId comes from strconv.Atoi(subject.SymbolName()) for MAPZONE/COORD
// primitives, or from IdProvider.Get for any other type.
//
// NAI-209-D-MAPZONE-COORD-PARSE-PANIC: invalid Atoi panics (TS would silently
// produce NaN-corrupted output).
//
// NAI-209-D-TYPEMARKER-CATEGORY-DISCRIMINATOR: TS L80 reads
// `subjectType === ScriptVarType.CATEGORY` — a per-script check on the
// subject symbol's runtime type. Goscape currently reads `tm.Category` — a
// per-trigger flag on TypeMode — because no `PrimitiveCategory` primitive is
// ported yet. The result diverges when a trigger has tm.Category=true and is
// used with a non-category subject: TS emits typeMarker=2, goscape emits 1.
// Resolving requires porting PrimitiveCategory and the per-subject type
// check; tracked for NAI-210 or a follow-up slice.
func (b *BinaryScriptWriter) generateLookupKey(script *codegen.RuneScript) int32 {
	if trigger.IsNameMode(script.Trigger.SubjectMode) {
		return -1
	}
	key := int32(script.Trigger.ID)
	tm, ok := trigger.IsTypeMode(script.Trigger.SubjectMode)
	if !ok || script.SubjectReference == nil {
		return key
	}
	subject, ok := script.SubjectReference.(symbol.Symbol)
	if !ok {
		panic(fmt.Sprintf("BinaryScriptWriter: SubjectReference %T is not a symbol.Symbol", script.SubjectReference))
	}
	subjectId := b.resolveSubjectId(subject)
	var typeMarker int32 = 2
	if tm.Category {
		typeMarker = 1
	}
	key += (typeMarker << 8) + (subjectId << 10)
	return key
}

func (b *BinaryScriptWriter) resolveSubjectId(subject symbol.Symbol) int32 {
	if bs, ok := subject.(*symbol.BasicSymbol); ok {
		switch bs.Type {
		case typ.PrimitiveMapzone, typ.PrimitiveCoord:
			n, err := strconv.Atoi(subject.SymbolName())
			if err != nil {
				panic(fmt.Sprintf("BinaryScriptWriter: invalid MAPZONE/COORD subject %q: %v",
					subject.SymbolName(), err))
			}
			return int32(n)
		}
	}
	return int32(b.IdProvider.Get(subject))
}
