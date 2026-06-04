// pkg/pack/compiler/codegen/codegen_expr.go — ports the expression visitor
// arms from TS CodeGenerator.ts (T7: variable/paren/calc; T8: arithmetic;
// T10: literals + JoinedString + Identifier).
package codegen

import (
	"fmt"
	"strconv"
	"unicode/utf8"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// visitLocalVar lowers a local-variable read (`$name` or `$name(index)`).
// Mirrors TS visitLocalVariableExpression (CodeGenerator.ts L870-L879).
func (g *CodeGenerator) visitLocalVar(lv *ast.LocalVariableExpression) {
	ref, _ := lv.Reference.(*symbol.LocalVariableSymbol)
	if ref == nil {
		diagnostics.ReportErrorAt(g.diagnostics, lv, diagnostics.MessageSymbolIsNull)
		return
	}
	g.LineInstruction(lv)
	if lv.Index != nil {
		g.VisitNodeOrNull(lv.Index)
	}
	g.Instruction(PushLocalVar, ref, lv.Source())
}

// visitGameVar lowers a game-variable read (`%name` or `.%name`).
// Mirrors TS visitGameVariableExpression (CodeGenerator.ts L828-L837).
func (g *CodeGenerator) visitGameVar(gv *ast.GameVariableExpression) {
	ref, _ := gv.Reference.(*symbol.BasicSymbol)
	if ref == nil {
		diagnostics.ReportErrorAt(g.diagnostics, gv, diagnostics.MessageSymbolIsNull)
		return
	}
	g.LineInstruction(gv)
	if gv.Dot {
		g.Instruction(PushVar2, ref, gv.Source())
	} else {
		g.Instruction(PushVar, ref, gv.Source())
	}
}

// visitConstantVar lowers a constant-variable read (`^name`). The constant's
// value is carried by SubExpression (resolved during type-checking).
// Mirrors TS visitConstantVariableExpression (CodeGenerator.ts L822-L826).
func (g *CodeGenerator) visitConstantVar(cv *ast.ConstantVariableExpression) {
	if cv.SubExpression == nil {
		diagnostics.ReportErrorAt(g.diagnostics, cv, diagnostics.MessageExpressionNoSubExpr)
		return
	}
	g.VisitNodeOrNull(cv.SubExpression)
}

// visitParenthesized lowers a parenthesized expression `(expr)`.
// Mirrors TS visitParenthesizedExpression (CodeGenerator.ts L839-L842).
func (g *CodeGenerator) visitParenthesized(pe *ast.ParenthesizedExpression) {
	g.LineInstruction(pe)
	g.VisitNodeOrNull(pe.Expression)
}

// visitCalc lowers a calc expression `calc(arithmetic)`.
// Mirrors TS visitCalcExpression (CodeGenerator.ts L844-L847).
func (g *CodeGenerator) visitCalc(ce *ast.CalcExpression) {
	g.LineInstruction(ce)
	g.VisitNodeOrNull(ce.Expression)
}

// intOperations maps operator text → opcode for integer math. Mirrors TS
// INT_OPERATIONS (CodeGenerator.ts L880).
var intOperations = map[string]Opcode{
	"+": Add,
	"-": Sub,
	"*": Multiply,
	"/": Divide,
	"%": Modulo,
	"&": And,
	"|": Or,
}

// longOperations mirrors TS LONG_OPERATIONS (CodeGenerator.ts L893).
var longOperations = map[string]Opcode{
	"+": LongAdd,
	"-": LongSub,
	"*": LongMultiply,
	"/": LongDivide,
	"%": LongModulo,
	"&": LongAnd,
	"|": LongOr,
}

// visitIntegerLiteral lowers an integer literal. When the literal carries a
// Reference (resolved by type-checking), emit PushConstantSymbol; when the
// expression type is string (enum-constant-as-string context), emit
// PushConstantString of the decimal text; otherwise PushConstantInt.
// Mirrors TS visitIntegerLiteral (CodeGenerator.ts L700-L720).
//
// Retires NAI-207-D-INTLIT-T5-STUB: the T5 inline stub in Visit dispatch is
// replaced by delegation to this function in T10.
func (g *CodeGenerator) visitIntegerLiteral(il *ast.IntegerLiteral) {
	g.LineInstruction(il)
	if il.Reference != nil {
		g.Instruction(PushConstantSymbol, il.Reference, il.Source())
		return
	}
	t := getExpressionType(il)
	if t == typ.PrimitiveString {
		g.Instruction(PushConstantString, strconv.Itoa(int(il.Value)), il.Source())
		return
	}
	g.Instruction(PushConstantInt, int(il.Value), il.Source())
}

// visitCoordLiteral lowers a coord literal (packed int32 from N_N_N_N_N syntax).
// Mirrors TS visitCoordLiteral (CodeGenerator.ts L722-L725).
func (g *CodeGenerator) visitCoordLiteral(cl *ast.CoordLiteral) {
	g.LineInstruction(cl)
	g.Instruction(PushConstantInt, int(cl.Value), cl.Source())
}

// visitBooleanLiteral lowers true/false. When the expression type is string,
// emits the string representation; otherwise emits PushConstantInt(0 or 1).
// Mirrors TS visitBooleanLiteral (CodeGenerator.ts L727-L736).
func (g *CodeGenerator) visitBooleanLiteral(bl *ast.BooleanLiteral) {
	g.LineInstruction(bl)
	t := getExpressionType(bl)
	if t == typ.PrimitiveString {
		s := "false"
		if bl.Value {
			s = "true"
		}
		g.Instruction(PushConstantString, s, bl.Source())
		return
	}
	v := 0
	if bl.Value {
		v = 1
	}
	g.Instruction(PushConstantInt, v, bl.Source())
}

// visitCharacterLiteral lowers a character literal. cl.Value is a string
// containing exactly one unicode codepoint; extract with DecodeRuneInString
// and emit its codepoint as PushConstantInt.
// Mirrors TS visitCharacterLiteral (CodeGenerator.ts L738-L741).
func (g *CodeGenerator) visitCharacterLiteral(cl *ast.CharacterLiteral) {
	g.LineInstruction(cl)
	if len(cl.Value) == 0 {
		return
	}
	r, _ := utf8.DecodeRuneInString(cl.Value)
	g.Instruction(PushConstantInt, int(r), cl.Source())
}

// visitNullLiteral lowers the null literal. Dispatches on the expression base
// type: BaseVarString → PushConstantString("null"), BaseVarLong →
// PushConstantLong(-1), default → PushConstantInt(-1). When the expression
// type is a MetaType.Hook, an extra PushConstantString("") is emitted after
// the int. Mirrors TS visitNullLiteral (CodeGenerator.ts L743-L762).
func (g *CodeGenerator) visitNullLiteral(nl *ast.NullLiteral) {
	g.LineInstruction(nl)
	t := getExpressionType(nl)
	var base typ.BaseVarType
	var hasBase bool
	if t != nil {
		base, hasBase = t.BaseType()
	}
	if hasBase {
		switch base {
		case typ.BaseVarString:
			g.Instruction(PushConstantString, "null", nl.Source())
			return
		case typ.BaseVarLong:
			g.Instruction(PushConstantLong, int64(-1), nl.Source())
			return
		}
	}
	g.Instruction(PushConstantInt, -1, nl.Source())

	// Hook special case: MetaType.Hook context emits trailing PushConstantString("").
	// Mirrors TS L756-L760.
	if _, isHook := typ.IsMetaHook(t); isHook {
		g.Instruction(PushConstantString, "", nl.Source())
	}
}

// visitStringLiteral lowers a plain string literal. When SubExpression is set
// (clientscript re-parse target, NAI-206), delegates to VisitNodeOrNull.
// When Reference is set, emits PushConstantSymbol. Otherwise PushConstantString.
// Mirrors TS visitStringLiteral (CodeGenerator.ts L764-L773).
func (g *CodeGenerator) visitStringLiteral(sl *ast.StringLiteral) {
	g.LineInstruction(sl)
	if sl.SubExpression != nil {
		g.VisitNodeOrNull(sl.SubExpression)
		return
	}
	if sl.Reference != nil {
		g.Instruction(PushConstantSymbol, sl.Reference, sl.Source())
		return
	}
	g.Instruction(PushConstantString, sl.Value, sl.Source())
}

// visitJoinedString lowers an interpolated string. Emits each part in order;
// when there are 2+ parts, emits JoinString(count) after them.
// Mirrors TS visitJoinedStringExpression (CodeGenerator.ts L775-L783).
func (g *CodeGenerator) visitJoinedString(je *ast.JoinedStringExpression) {
	for _, part := range je.Parts {
		g.visitJoinedStringPart(part)
	}
	if len(je.Parts) > 1 {
		g.LineInstruction(je)
		g.Instruction(JoinString, len(je.Parts), je.Source())
	}
}

// visitJoinedStringPart lowers one part of a JoinedStringExpression.
// BasicStringPart and PTagStringPart → PushConstantString;
// ExpressionStringPart → VisitNodeOrNull on the inner expression.
// Mirrors TS visitStringPart (CodeGenerator.ts L785-L793).
//
// PTagStringPart `extends BasicStringPart` in TS — `_ instanceof BasicStringPart`
// is true for PTag instances via JS prototype-chain semantics, so TS routes
// them through the same emit path. Goscape's struct types are distinct, so
// emit the PTag branch explicitly.
func (g *CodeGenerator) visitJoinedStringPart(part ast.StringPart) {
	g.LineInstruction(part)
	switch p := part.(type) {
	case *ast.BasicStringPart:
		g.Instruction(PushConstantString, p.Value, p.Source())
	case *ast.PTagStringPart:
		g.Instruction(PushConstantString, p.Value, p.Source())
	case *ast.ExpressionStringPart:
		g.VisitNodeOrNull(p.Expression)
	default:
		panic(fmt.Sprintf("visitJoinedStringPart: unsupported StringPart %T", part))
	}
}

// visitIdentifier lowers an identifier expression. When the reference is nil
// but the type is string, emits PushConstantString(id.Text) (bare-name
// enum-key context). When the reference is nil for any other type, reports a
// diagnostic. When the reference is a ServerScriptSymbol with CommandTrigger,
// routes through emitDynamicCommand or falls back to Command(ref). Otherwise
// emits PushConstantSymbol(ref). Mirrors TS visitIdentifier (CodeGenerator.ts
// L795-L826).
func (g *CodeGenerator) visitIdentifier(id *ast.Identifier) {
	g.LineInstruction(id)
	ref := id.Reference
	t := getExpressionType(id)
	if ref == nil && t == typ.PrimitiveString {
		g.Instruction(PushConstantString, id.Text, id.Source())
		return
	}
	if ref == nil {
		diagnostics.ReportErrorAt(g.diagnostics, id, diagnostics.MessageSymbolIsNull)
		return
	}
	// ServerScriptSymbol + CommandTrigger: route through emitDynamicCommand
	// or fall back to Command(ref). ss.Trigger is a field on ScriptSymbolFields.
	if ss, ok := ref.(*symbol.ServerScriptSymbol); ok && ss.Trigger == trigger.CommandTrigger {
		if g.emitDynamicCommand(id.Text, id) {
			return
		}
		g.Instruction(Command, ss, id.Source())
		return
	}
	g.Instruction(PushConstantSymbol, ref, id.Source())
}

// visitArith lowers a binary arithmetic expression inside calc(). The opcode
// is selected by the left operand's base type (int vs long) and the operator
// symbol. Mirrors TS visitArithmeticExpression (CodeGenerator.ts L906-L938).
func (g *CodeGenerator) visitArith(ae *ast.ArithmeticExpression) {
	leftType := getExpressionType(ae.Left)
	if leftType == nil {
		panic("visitArith: left operand has no type — TypeChecker should have set it")
	}
	base, ok := leftType.BaseType()
	if !ok {
		panic("visitArith: left operand type has no base type")
	}
	var mappings map[string]Opcode
	switch base {
	case typ.BaseVarInteger:
		mappings = intOperations
	case typ.BaseVarLong:
		mappings = longOperations
	default:
		panic(fmt.Sprintf("visitArith: no mapping for BaseVarType %d", base))
	}
	op, ok := mappings[ae.Operator.Text]
	if !ok {
		panic("visitArith: no mapping for operator " + ae.Operator.Text)
	}
	g.VisitNodeOrNull(ae.Left)
	g.VisitNodeOrNull(ae.Right)
	// TS CodeGenerator.ts visitArithmeticExpression L568 calls
	// `this.instructionUnit(opcode)` with no source argument — the
	// arithmetic opcode itself carries no line-number entry. Passing
	// ae.Source() here inflates the LineNumberTable on every chained calc
	// (same bug family as 567b9035 / codegen_branch_unsourced_fix).
	g.instructionUnit(op, lexer.NodeSourceLocation{})
}
