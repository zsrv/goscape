// pkg/pack/compiler/codegen/codegen_call.go — ports the call-expression arms
// from TS CodeGenerator.ts (visitCommandCall / visitProcCall / visitJumpCall +
// emitDynamicCommand). visitClientScript lands in T11.
package codegen

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func (g *CodeGenerator) visitCommandCall(cc *ast.CommandCallExpression) {
	sym, _ := cc.Symbol.(symbol.Symbol)
	if sym == nil {
		diagnostics.ReportErrorAt(g.diagnostics, cc, diagnostics.MessageSymbolIsNull)
		return
	}
	if g.emitDynamicCommand(cc.NameString(), cc) {
		return
	}
	g.visitExpressions(cc.Arguments)
	g.LineInstruction(cc)
	g.Instruction(Command, sym, cc.Source())
}

func (g *CodeGenerator) visitProcCall(pc *ast.ProcCallExpression) {
	sym, _ := pc.Symbol.(symbol.Symbol)
	if sym == nil {
		diagnostics.ReportErrorAt(g.diagnostics, pc, diagnostics.MessageSymbolIsNull)
		return
	}
	g.visitExpressions(pc.Arguments)
	g.LineInstruction(pc)
	g.Instruction(Gosub, sym, pc.Source())
}

func (g *CodeGenerator) visitJumpCall(jc *ast.JumpCallExpression) {
	sym, _ := jc.Symbol.(symbol.Symbol)
	if sym == nil {
		diagnostics.ReportErrorAt(g.diagnostics, jc, diagnostics.MessageSymbolIsNull)
		return
	}
	g.visitExpressions(jc.Arguments)
	g.LineInstruction(jc)
	g.Instruction(Jump, sym, jc.Source())
}

// visitClientScript emits the instruction sequence for a ClientScriptExpression.
// Mirrors TS CodeGenerator.ts visitClientScript.
//
// Emission order:
//  1. PushConstantSymbol — the script reference.
//  2. visitExpressions(cse.Arguments) — parameter values.
//  3. If TransmitList is non-empty:
//     a. visitExpressions(cse.TransmitList) — transmit values.
//     b. 'Y' appended to the typecode string.
//     c. PushConstantInt(len(cse.TransmitList)).
//  4. PushConstantString — the typecode string built from parameter codes.
//
// The typecode string is built by iterating TupleToList(paramType) and
// collecting the single-char Code() of each element type. ClientScriptSymbol
// and ServerScriptSymbol both embed ScriptSymbolFields.Parameters (typ.Type),
// accessed via a type-switch on the symbol.Symbol marker value.
func (g *CodeGenerator) visitClientScript(cse *ast.ClientScriptExpression) {
	sym, _ := cse.Symbol.(symbol.Symbol)
	if sym == nil {
		diagnostics.ReportErrorAt(g.diagnostics, cse, diagnostics.MessageSymbolIsNull)
		return
	}

	// Extract Parameters from whichever concrete script-symbol type was
	// resolved. Both *ServerScriptSymbol and *ClientScriptSymbol embed
	// ScriptSymbolFields which carries the Parameters field.
	var paramType typ.Type
	switch s := sym.(type) {
	case *symbol.ServerScriptSymbol:
		paramType = s.Parameters
	case *symbol.ClientScriptSymbol:
		paramType = s.Parameters
	default:
		panic(fmt.Sprintf("visitClientScript: unsupported script symbol type %T", sym))
	}

	// Build the per-parameter typecode string.
	paramList := typ.TupleToList(paramType)
	typecodes := make([]byte, 0, len(paramList)+1)
	for _, p := range paramList {
		if p == nil {
			panic("visitClientScript: nil parameter type in tuple")
		}
		c, ok := p.Code()
		if !ok || c == "" {
			panic(fmt.Sprintf("visitClientScript: parameter type %v has no type code", p))
		}
		typecodes = append(typecodes, c[0])
	}

	// 1. Emit the script reference.
	g.Instruction(PushConstantSymbol, sym, cse.Source())

	// 2. Emit argument expressions.
	g.visitExpressions(cse.Arguments)

	// 3. Transmit list (optional).
	if len(cse.TransmitList) > 0 {
		g.visitExpressions(cse.TransmitList)
		typecodes = append(typecodes, 'Y')
		g.Instruction(PushConstantInt, len(cse.TransmitList), cse.Source())
	}

	// 4. Emit the final typecode string.
	g.Instruction(PushConstantString, string(typecodes), cse.Source())
}

// emitDynamicCommand looks up a DynamicCommandHandler by name and dispatches
// its GenerateCode. Returns true iff a handler was registered. Per
// NAI-207-D-DYNCOMMAND-BOOLRESULT, GenerateCode==false means handler did not
// emit code — fall back to "emit args + Command" (TS L600-L605).
//
// NAI-207-D-DYNCOMMAND-FALLBACK-VISITEXPR: ctx.VisitNodes takes []ast.Node
// but ctx.Arguments() returns []ast.Expression. The fallback iterates
// ctx.Arguments() via VisitNodeOrNull to avoid a type-conversion slice. The
// Command() method on CodeGeneratorContext emits LineInstruction + Command(sym).
func (g *CodeGenerator) emitDynamicCommand(name string, expr ast.Expression) bool {
	h, ok := g.dynamicCommands[name]
	if !ok {
		return false
	}
	ctx := NewCodeGeneratorContext(g, g.rootTable, expr, g.diagnostics)
	if !h.GenerateCode(ctx) {
		for _, arg := range ctx.Arguments() {
			g.VisitNodeOrNull(arg)
		}
		ctx.Command()
	}
	return true
}
