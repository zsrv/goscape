// pkg/pack/compiler/codegen/codegen_call.go — ports the call-expression arms
// from TS CodeGenerator.ts (visitCommandCall / visitProcCall / visitJumpCall +
// emitDynamicCommand). visitClientScript lands in T11.
package codegen

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
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
