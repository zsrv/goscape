// pkg/pack/compiler/command/debug_dump_handler.go
package command

// DumpCommandHandler is a developer debug command that converts
// `dump(expr1, expr2, ...)` into the joined string `"expr1=<expr1>, expr2=<expr2>, ..."`.
// Mirrors TS debug/DumpCommandHandler.ts.
//
// TypeCheck:
//   - Rejects zero arguments.
//   - Accepts any non-tuple, non-unit type (int or string base type).
//   - Sets expression type to PrimitiveString.
//
// GenerateCode:
//   - Emits LineInstruction.
//   - For each argument: emits PushConstantString("expr="), then visits the expr,
//     then converts to string if needed, and emits separator ","  for non-last args.
//   - Emits JoinString(parts).
//
// NAI-207-D-COHORT-B-DUMP-MINIMAL: TS DumpCommandHandler uses ExpressionGenerator
// (an AstVisitor<string>) and rootTable.find(CommandTrigger) to look up `escape`
// and `toString` conversion commands. Goscape cannot use AstVisitor (see
// NAI-204-D-AST-NO-VISITOR); instead a local dumpExprText switch generates the
// expression text. rootTable lookup is ported faithfully via cgc.SymbolTable.

import (
	"fmt"
	"strings"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/codegen"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/semantics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	"github.com/zsrv/goscape/pkg/pack/compiler/trigger"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

const (
	// dumpDiagInvalidSignature mirrors TS DIAGNOSTIC_INVALID_SIGNATURE.
	dumpDiagInvalidSignature = "Type mismatch: '<unit>' was given but 'any...' was expected."
	// dumpDiagTupleType mirrors TS DIAGNOSTIC_TUPLE_TYPE.
	dumpDiagTupleType = "Unable to dump multi-value types: %s"
	// dumpDiagUnitType mirrors TS DIAGNOSTIC_UNIT_TYPE.
	dumpDiagUnitType = "Unable to debug expression with no return value."
)

// DumpCommandHandler implements DynamicCommandHandler for the `dump` debug command.
type DumpCommandHandler struct{}

// TypeCheck ports DumpCommandHandler.ts typeCheck verbatim.
func (h *DumpCommandHandler) TypeCheck(ctx *semantics.TypeCheckingContext) {
	args := ctx.Arguments()
	if len(args) == 0 {
		// TS: context.expression.reportError(...). Mirrors NAI-205-D-NO-NODE-REPORT-ERROR.
		diagnostics.ReportErrorAt(ctx.Diagnostics, ctx.Expression(), dumpDiagInvalidSignature)
		ctx.SetType(typ.PrimitiveString)
		return
	}

	for i, arg := range args {
		ctx.CheckArgument(i, typ.MetaAny, false)

		t := semantics.ExprType(arg)
		if t == nil {
			continue
		}
		if _, ok := t.(*typ.TupleType); ok {
			diagnostics.ReportErrorAt(ctx.Diagnostics, arg, fmt.Sprintf(dumpDiagTupleType, t.Representation()))
		} else {
			base, ok := t.BaseType()
			if ok && base != typ.BaseVarInteger && base != typ.BaseVarString {
				diagnostics.ReportErrorAt(ctx.Diagnostics, arg, dumpDiagUnitType)
			}
		}
	}

	ctx.SetType(typ.PrimitiveString)
}

// GenerateCode ports DumpCommandHandler.ts generateCode verbatim.
// Emits: LineInstruction + per-arg (PushConstantString("name=") + expr + toString) +
// separator + JoinString(parts).
// Mirrors TS L44-L70.
//
// NAI-207-D-COHORT-B-DUMP-MINIMAL: dumpExprText replaces the TS ExpressionGenerator
// AstVisitor. The rootTable symbol lookup for `escape`/`toString` is ported faithfully
// via cgc.SymbolTable.Find(SymbolTypeServerScript(CommandTrigger), name).
func (h *DumpCommandHandler) GenerateCode(ctx semantics.CodeGenContext) bool {
	cgc := ctx.(*codegen.CodeGeneratorContext)

	cgc.LineInstruction(cgc.Expression)

	parts := 0
	args := cgc.Arguments()

	for i, arg := range args {
		argString := dumpExprText(arg)

		// Emit the "expr=" prefix string.
		cgc.Instruction(codegen.PushConstantString, argString+"=", cgc.Expression.Source())
		parts++

		// Evaluate the expression.
		cgc.VisitNode(arg)

		// Convert to string if needed. Mirrors TS L60-L62.
		h.typeToString(cgc, semantics.ExprType(arg))

		// Separator ", " for non-last args. Mirrors TS L63-L66.
		if i != len(args)-1 {
			cgc.Instruction(codegen.PushConstantString, ", ", cgc.Expression.Source())
			parts++
		}
	}

	// Emit JoinString(parts). Mirrors TS L69.
	cgc.Instruction(codegen.JoinString, parts, cgc.Expression.Source())

	return true
}

// typeToString emits the conversion command for the given type. Mirrors TS
// DumpCommandHandler.typeToString (L72-L88).
//
// For PrimitiveString: looks up and emits `escape`.
// For BaseVarInteger: looks up and emits `toString`.
// Other types panic (mirrors TS `throw new Error(...)`).
func (h *DumpCommandHandler) typeToString(cgc *codegen.CodeGeneratorContext, t typ.Type) {
	if t == nil {
		return
	}

	var convName string
	if t == typ.PrimitiveString {
		convName = "escape"
	} else if base, ok := t.BaseType(); ok && base == typ.BaseVarInteger {
		convName = "toString"
	} else {
		// Unsupported type: mirrors TS `throw new Error(...)`.
		// In production this is a pre-validated path; panic preserves the contract.
		panic(fmt.Sprintf("DumpCommandHandler.typeToString: unsupported type %v", t))
	}

	// Look up the conversion command in the root symbol table.
	// Mirrors TS: context.rootTable.find(SymbolType.serverScript(CommandTrigger), name).
	convSym := cgc.SymbolTable.Find(
		symbol.SymbolTypeServerScript(trigger.CommandTrigger),
		convName,
	)
	if convSym != nil {
		cgc.Instruction(codegen.Command, convSym, cgc.Expression.Source())
	}
}

// dumpExprText generates the text representation of an expression for use as the
// debug label in dump output. Mirrors TS ExpressionGenerator (a visitor<string>).
//
// NAI-207-D-COHORT-B-DUMP-MINIMAL: goscape uses a Go type-switch in place of the
// AstVisitor<string> because NAI-204-D-AST-NO-VISITOR eliminated the visitor pattern.
// The type-switch mirrors ExpressionGenerator.ts method-by-method.
func dumpExprText(expr ast.Expression) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.ArithmeticExpression:
		// TS: visitBinaryExpression — `${left} ${op} ${right}`
		op := ""
		if e.Operator != nil {
			op = e.Operator.Text
		}
		return dumpExprText(e.Left) + " " + op + " " + dumpExprText(e.Right)
	case *ast.CalcExpression:
		// TS: visitCalcExpression — `calc${inner}`
		return "calc" + dumpExprText(e.Expression)
	case *ast.CommandCallExpression:
		// TS: visitCommandCallExpression — `~name(args...)`
		name := ""
		if e.Name != nil {
			name = e.Name.Text
		}
		result := "~" + name
		if len(e.Arguments) > 0 {
			parts := make([]string, 0, len(e.Arguments))
			for _, a := range e.Arguments {
				parts = append(parts, dumpExprText(a))
			}
			result += "(" + strings.Join(parts, "") + ")"
		}
		return result
	case *ast.LocalVariableExpression:
		// TS: visitLocalVariableExpression — `$name`
		if e.Name != nil {
			return "$" + e.Name.Text
		}
		return "$"
	case *ast.GameVariableExpression:
		// TS: visitGameVariableExpression — `%name`
		if e.Name != nil {
			return "%" + e.Name.Text
		}
		return "%"
	case *ast.ConstantVariableExpression:
		// TS: visitConstantVariableExpression — `^name`
		if e.Name != nil {
			return "^" + e.Name.Text
		}
		return "^"
	case *ast.CharacterLiteral:
		// TS: visitCharacterLiteral — `'x'`
		return "'" + e.Value + "'"
	case *ast.NullLiteral:
		// TS: visitNullLiteral — `null`
		return "null"
	case *ast.StringLiteral:
		// TS: visitStringLiteral — `"value"`
		return `"` + e.Value + `"`
	case *ast.IntegerLiteral:
		// TS: visitLiteral — `String(value)`
		return fmt.Sprintf("%d", e.Value)
	case *ast.CoordLiteral:
		// TS: visitLiteral — `String(value)`
		return fmt.Sprintf("%d", e.Value)
	case *ast.BooleanLiteral:
		// TS: visitLiteral — `String(value)` (true/false)
		if e.Value {
			return "true"
		}
		return "false"
	case *ast.JoinedStringExpression:
		// TS: visitJoinedStringExpression — `"parts..."`
		var sb strings.Builder
		sb.WriteByte('"')
		for _, p := range e.Parts {
			switch part := p.(type) {
			case *ast.BasicStringPart:
				sb.WriteString(part.Value)
			case *ast.ExpressionStringPart:
				sb.WriteByte('<')
				sb.WriteString(dumpExprText(part.Expression))
				sb.WriteByte('>')
			}
		}
		sb.WriteByte('"')
		return sb.String()
	case *ast.Identifier:
		// TS: visitIdentifier — `identifier.text`
		return e.Text
	default:
		return ""
	}
}
