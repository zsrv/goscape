// pkg/pack/compiler/codegen/codegen_stmt.go — ports the statement walker arms
// from TS CodeGenerator.ts.
package codegen

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
)

func (g *CodeGenerator) visitReturnStatement(rs *ast.ReturnStatement) {
	g.visitExpressions(rs.Expressions)
	g.LineInstruction(rs)
	g.instructionUnit(Return, rs.Source())
}

func (g *CodeGenerator) visitIfStatement(is *ast.IfStatement) {
	ifTrue := g.labelGenerator.Generate("if_true")
	var ifElse *Label
	if is.ElseStatement != nil {
		ifElse = g.labelGenerator.Generate("if_else")
	}
	ifEnd := g.labelGenerator.Generate("if_end")

	// Generate condition into the current block.
	falseTarget := ifEnd
	if ifElse != nil {
		falseTarget = ifElse
	}
	g.generateCondition(is.Condition, g.block, ifTrue, falseTarget)

	// if_true block.
	g.bind(g.generateBlockLabel(ifTrue))
	g.VisitNodeOrNull(is.ThenStatement)
	g.Instruction(Branch, ifEnd, is.Source())

	// Optional if_else block.
	if ifElse != nil {
		g.bind(g.generateBlockLabel(ifElse))
		g.VisitNodeOrNull(is.ElseStatement)
		g.Instruction(Branch, ifEnd, is.Source())
	}

	// if_end block.
	g.bind(g.generateBlockLabel(ifEnd))
}

func (g *CodeGenerator) visitWhileStatement(ws *ast.WhileStatement) {
	whileStart := g.labelGenerator.Generate("while_start")
	whileBody := g.labelGenerator.Generate("while_body")
	whileEnd := g.labelGenerator.Generate("while_end")

	startBlock := g.bind(g.generateBlockLabel(whileStart))
	g.generateCondition(ws.Condition, startBlock, whileBody, whileEnd)

	g.bind(g.generateBlockLabel(whileBody))
	g.VisitNodeOrNull(ws.ThenStatement)
	g.Instruction(Branch, whileStart, ws.Source())

	g.bind(g.generateBlockLabel(whileEnd))
}

func (g *CodeGenerator) visitSwitchStatement(ss *ast.SwitchStatement) {
	rs := g.activeScript()
	if rs == nil {
		return
	}
	table := rs.GenerateSwitchTable()
	hasDefault := ss.DefaultCase != nil
	var switchDefault *Label
	if hasDefault {
		switchDefault = g.labelGenerator.Generate("switch_default_case")
	}
	switchEnd := g.labelGenerator.Generate("switch_end")

	// Visit the discriminant.
	g.VisitNodeOrNull(ss.Condition)
	g.Instruction(Switch, table, ss.Source())

	// First-case-default short-circuit: if the first case is NOT default,
	// emit a Branch to default-or-end so the discriminant has a fallthrough.
	// Mirrors TS L344.
	var first *ast.SwitchCase
	if len(ss.Cases) > 0 {
		first = ss.Cases[0]
	}
	if first == nil || !first.IsDefault() {
		target := switchEnd
		if switchDefault != nil {
			target = switchDefault
		}
		g.Instruction(Branch, target, ss.Source())
	}

	for _, caseEntry := range ss.Cases {
		var caseLabel *Label
		if caseEntry.IsDefault() {
			if switchDefault == nil {
				panic("visitSwitchStatement: default case present but switchDefault label nil")
			}
			caseLabel = switchDefault
		} else {
			caseLabel = g.labelGenerator.Generate(fmt.Sprintf("switch_%d_case", table.ID))
		}

		keys := make([]any, 0, len(caseEntry.Keys))
		for _, keyExpr := range caseEntry.Keys {
			k := resolveConstantValue(keyExpr)
			if k == nil {
				diagnostics.ReportErrorAt(g.diagnostics, keyExpr,
					diagnostics.MessageNullConstant, opShorthand(keyExpr))
				continue
			}
			keys = append(keys, k)
		}
		table.AddCase(SwitchCase{Label: caseLabel, Keys: keys})

		g.bind(g.generateBlockLabel(caseLabel))
		for _, st := range caseEntry.Statements {
			g.VisitNodeOrNull(st)
		}
		g.Instruction(Branch, switchEnd, ss.Source())
	}

	g.bind(g.generateBlockLabel(switchEnd))
}

// resolveConstantValue attempts to read a constant value from an expression.
// Returns nil if the expression is not constant or has no usable reference.
// Mirrors TS resolveConstantValue (CodeGenerator.ts L390).
func resolveConstantValue(expr ast.Expression) any {
	switch e := expr.(type) {
	case *ast.ConstantVariableExpression:
		if e.SubExpression != nil {
			return resolveConstantValue(e.SubExpression)
		}
		return nil
	case *ast.Identifier:
		if e.Reference != nil {
			return e.Reference
		}
		return nil
	case *ast.IntegerLiteral:
		if e.Reference != nil {
			return e.Reference
		}
		return e.Value
	case *ast.StringLiteral:
		if e.Reference != nil {
			return e.Reference
		}
		return e.Value
	case *ast.BooleanLiteral:
		// Boolean values are pushed as ints 0/1 — resolveConstantValue
		// returns the raw bool for switch-case key matching.
		return e.Value
	case *ast.CharacterLiteral:
		return e.Value
	case *ast.CoordLiteral:
		return e.Value
	case *ast.NullLiteral:
		return nil
	}
	return nil
}
