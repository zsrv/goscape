// pkg/pack/compiler/codegen/codegen_stmt.go — ports the statement walker arms
// from TS CodeGenerator.ts.
package codegen

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
	"github.com/zsrv/goscape/pkg/pack/compiler/symbol"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
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

	// if_true block. The trailing Branch carries no source — mirrors TS
	// CodeGenerator.ts L254 (`this.instruction(Opcode.Branch, ifEnd)` with
	// undefined source). A real source would inflate LineNumberTable with a
	// spurious back-edge entry.
	g.bind(g.generateBlockLabel(ifTrue))
	g.VisitNodeOrNull(is.ThenStatement)
	g.Instruction(Branch, ifEnd, lexer.NodeSourceLocation{})

	// Optional if_else block. Trailing Branch unsourced (TS L263).
	if ifElse != nil {
		g.bind(g.generateBlockLabel(ifElse))
		g.VisitNodeOrNull(is.ElseStatement)
		g.Instruction(Branch, ifEnd, lexer.NodeSourceLocation{})
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
	// Body's back-edge Branch unsourced (TS L282).
	g.Instruction(Branch, whileStart, lexer.NodeSourceLocation{})

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
		// Fallthrough Branch unsourced (TS L346).
		g.Instruction(Branch, target, lexer.NodeSourceLocation{})
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
		// Case-trailer Branch unsourced (TS L380).
		g.Instruction(Branch, switchEnd, lexer.NodeSourceLocation{})
	}

	g.bind(g.generateBlockLabel(switchEnd))
}

// visitBlockStatement iterates over the block's statements. Mirrors TS
// visitBlockStatement (CodeGenerator.ts L271).
func (g *CodeGenerator) visitBlockStatement(bs *ast.BlockStatement) {
	for _, st := range bs.Statements {
		g.VisitNodeOrNull(st)
	}
}

// visitDeclaration lowers a `def_T $name (= expr)?;` declaration.
// Mirrors TS visitDeclarationStatement (CodeGenerator.ts L415-L440).
//
// If an initializer is present it is visited (pushes value); otherwise the
// type's default value is pushed. PopLocalVar is always emitted at the end.
func (g *CodeGenerator) visitDeclaration(ds *ast.DeclarationStatement) {
	sym, _ := ds.Symbol.(*symbol.LocalVariableSymbol)
	if rs := g.activeScript(); rs != nil && sym != nil {
		rs.Locals.All = append(rs.Locals.All, sym)
	}
	if ds.Initializer != nil {
		g.VisitNodeOrNull(ds.Initializer)
	} else if sym != nil {
		// Default-value pushes are synthetic; TS omits the `source` argument on
		// CodeGenerator.ts L420-L427, leaving the instruction's source.line
		// undefined. GenerateLineNumberTable's `line !== prevLine` guard skips
		// undefined-line instructions, so synthesized pushes do not create a
		// LineNumberTable entry. Goscape must do the same — passing
		// ds.Source() here would attach the declaration's line to the
		// synthetic push and shift every subsequent PC up by 1 vs the TS-
		// packed cache.
		def := defaultValueFor(sym.Type)
		switch dv := def.(type) {
		case int:
			g.Instruction(PushConstantInt, dv, lexer.NodeSourceLocation{})
		case string:
			g.Instruction(PushConstantString, dv, lexer.NodeSourceLocation{})
		case int64:
			g.Instruction(PushConstantLong, dv, lexer.NodeSourceLocation{})
		default:
			panic(fmt.Sprintf("visitDeclaration: unsupported default-value type %T for symbol %v", def, sym))
		}
	}
	g.Instruction(PopLocalVar, sym, ds.Source())
}

// visitArrayDeclaration lowers a `def_Tarray $name(size);` declaration.
// Mirrors TS visitArrayDeclarationStatement (CodeGenerator.ts L442-L449).
func (g *CodeGenerator) visitArrayDeclaration(ads *ast.ArrayDeclarationStatement) {
	sym, _ := ads.Symbol.(*symbol.LocalVariableSymbol)
	if rs := g.activeScript(); rs != nil && sym != nil {
		rs.Locals.All = append(rs.Locals.All, sym)
	}
	g.VisitNodeOrNull(ads.Initializer)
	g.Instruction(DefineArray, sym, ads.Source())
}

// visitAssignment lowers an assignment statement. Mirrors TS
// visitAssignmentStatement (CodeGenerator.ts L451-L487).
//
// Array-index special case: if the first LHS is a LocalVariableExpression
// with a non-nil Index, push the index before visiting the RHS — mirrors
// TS L451-L454.
//
// Vars are popped in reverse order so the pop sequence matches the order RHS
// values were pushed — mirrors TS L460-L480 reverse-for.
func (g *CodeGenerator) visitAssignment(as *ast.AssignmentStatement) {
	vars := as.Vars
	if len(vars) == 0 {
		return
	}
	// Array-index special case: push index before RHS expressions.
	if lv, ok := vars[0].(*ast.LocalVariableExpression); ok && lv.Index != nil {
		g.VisitNodeOrNull(lv.Index)
	}

	g.visitExpressions(as.Expressions)

	// Reverse-iterate so pops match RHS push order.
	for i := len(vars) - 1; i >= 0; i-- {
		variable := vars[i]
		ref := referenceOf(variable)
		if ref == nil {
			diagnostics.ReportErrorAt(g.diagnostics, variable, diagnostics.MessageSymbolIsNull)
			return
		}
		switch refTyped := ref.(type) {
		case *symbol.LocalVariableSymbol:
			g.Instruction(PopLocalVar, refTyped, variable.Source())
		case *symbol.BasicSymbol:
			gv, ok := variable.(*ast.GameVariableExpression)
			if !ok {
				panic("visitAssignment: expected GameVariableExpression for BasicSymbol reference")
			}
			if gv.Dot {
				g.Instruction(PopVar2, refTyped, variable.Source())
			} else {
				g.Instruction(PopVar, refTyped, variable.Source())
			}
		default:
			panic(fmt.Sprintf("visitAssignment: unsupported reference type %T", ref))
		}
	}
}

// visitExpressionStatement lowers an expression-as-statement. Mirrors TS
// visitExpressionStatement (CodeGenerator.ts L489-L501).
//
// The expression is visited (pushes values); each returned type is discarded
// via a Discard instruction with the appropriate BaseVarType operand.
func (g *CodeGenerator) visitExpressionStatement(es *ast.ExpressionStatement) {
	g.VisitNodeOrNull(es.Expression)
	exprType := getExpressionType(es.Expression)
	types := typ.TupleToList(exprType)
	for _, t := range types {
		if t == nil {
			continue
		}
		base, ok := t.BaseType()
		if !ok {
			diagnostics.ReportErrorAt(g.diagnostics, es, diagnostics.MessageTypeHasNoBaseType, t)
			return
		}
		g.Instruction(Discard, base, es.Source())
	}
}

// visitEmptyStatement is a no-op. Mirrors TS visitEmptyStatement (CodeGenerator.ts L501).
func (g *CodeGenerator) visitEmptyStatement(es *ast.EmptyStatement) {
	_ = es
}

// defaultValueFor returns the language-level zero value for a type. Mirrors
// TS Type.defaultValue, but for the subset codegen actually emits:
//   - PrimitiveInt → int(0) (special-cased to 0, not -1)
//   - BaseVarInteger (non-int) → int(-1)
//   - BaseVarString → string("")
//   - BaseVarLong → int64(-1)
//   - other/unknown → int(0)
//
// PRECONDITION: t is non-nil. Caller (visitDeclaration) guards sym != nil.
func defaultValueFor(t typ.Type) any {
	if t == typ.PrimitiveInt {
		return 0
	}
	base, ok := t.BaseType()
	if !ok {
		return 0
	}
	switch base {
	case typ.BaseVarInteger:
		return -1
	case typ.BaseVarString:
		return ""
	case typ.BaseVarLong:
		return int64(-1)
	default:
		return 0
	}
}

// referenceOf reads the Reference field from a variable AST expression.
// Returns nil if the expression has no reference or is not a variable expression.
// Mirrors TS CodeGenerator.ts L465-L475 per-variable reference dispatch.
func referenceOf(expr ast.Expression) any {
	switch e := expr.(type) {
	case *ast.LocalVariableExpression:
		return e.Reference
	case *ast.GameVariableExpression:
		return e.Reference
	}
	return nil
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
