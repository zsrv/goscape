// pkg/pack/compiler/codegen/codegen_expr.go — ports the expression visitor
// arms from TS CodeGenerator.ts (T7: variable/paren/calc; T8: arithmetic).
package codegen

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
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
	g.instructionUnit(op, ae.Source())
}
