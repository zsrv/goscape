// pkg/pack/compiler/codegen/codegen_cond.go — ports generateCondition +
// BRANCH_MAPPINGS from TS CodeGenerator.ts.
package codegen

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

const (
	logicalAnd = "&"
	logicalOr  = "|"
)

// intBranches maps op-text → branch opcode for integer base. Mirrors TS
// INT_BRANCHES (CodeGenerator.ts L840).
var intBranches = map[string]Opcode{
	"=":  BranchEquals,
	"!":  BranchNot,
	"<":  BranchLessThan,
	">":  BranchGreaterThan,
	"<=": BranchLessThanOrEquals,
	">=": BranchGreaterThanOrEquals,
}

// objBranches maps op-text → branch opcode for string (object) base.
// Mirrors TS OBJ_BRANCHES (CodeGenerator.ts L852).
var objBranches = map[string]Opcode{
	"=": ObjBranchEquals,
	"!": ObjBranchNot,
}

// longBranches maps op-text → branch opcode for long base. Mirrors TS
// LONG_BRANCHES (CodeGenerator.ts L860).
var longBranches = map[string]Opcode{
	"=":  LongBranchEquals,
	"!":  LongBranchNot,
	"<":  LongBranchLessThan,
	">":  LongBranchGreaterThan,
	"<=": LongBranchLessThanOrEquals,
	">=": LongBranchGreaterThanOrEquals,
}

// branchMappings returns the inner map for a BaseVarType, or nil.
// Mirrors TS BRANCH_MAPPINGS.get(baseType).
func branchMappings(base typ.BaseVarType) map[string]Opcode {
	switch base {
	case typ.BaseVarInteger:
		return intBranches
	case typ.BaseVarString:
		return objBranches
	case typ.BaseVarLong:
		return longBranches
	default:
		return nil
	}
}

// generateCondition lowers a condition expression into branch/branch-target
// instructions in `block`. branchTrue/branchFalse are the target labels for
// the two outcomes. Recursively handles logical-and / logical-or chains.
// Mirrors TS generateCondition (CodeGenerator.ts L288).
//
// NAI-207-D-COND-NO-ARITH: TS generateCondition has an ArithmeticExpression
// arm as a fallback for grammars that place '='/'!' under arithmetic. Goscape's
// parser (NAI-203/NAI-204) always emits ConditionExpression for condition
// operators; the ArithmeticExpression arm is unreachable and is omitted.
func (g *CodeGenerator) generateCondition(condition ast.Expression, block *Block, branchTrue, branchFalse *Label) {
	switch c := condition.(type) {
	case *ast.ConditionExpression:
		g.generateConditionBinary(c.Operator.Text, c.Left, c.Right, block, branchTrue, branchFalse)
	case *ast.ParenthesizedExpression:
		g.generateCondition(c.Expression, block, branchTrue, branchFalse)
	default:
		diagnostics.ReportErrorAt(g.diagnostics, condition, diagnostics.MessageInvalidCondition, opShorthand(condition))
	}
}

// generateConditionBinary handles the binary/condition cases. Splits out
// of generateCondition for clarity; the type-switch above dispatches to
// this helper. Mirrors TS generateCondition L289-L323.
func (g *CodeGenerator) generateConditionBinary(opText string, left, right ast.Expression, block *Block, branchTrue, branchFalse *Label) {
	if opText == logicalAnd || opText == logicalOr {
		// Chained logical: generate a follow-up block for the rhs.
		var nextLbl *Label
		if opText == logicalOr {
			nextLbl = g.labelGenerator.Generate("condition_or")
		} else {
			nextLbl = g.labelGenerator.Generate("condition_and")
		}
		trueLbl, falseLbl := branchTrue, branchFalse
		if opText == logicalOr {
			falseLbl = nextLbl
		} else {
			trueLbl = nextLbl
		}
		// First recurse on lhs.
		g.generateCondition(left, block, trueLbl, falseLbl)
		nextBlock := g.bind(g.generateBlockLabel(nextLbl))
		g.generateCondition(right, nextBlock, branchTrue, branchFalse)
		return
	}

	// Non-logical: pure comparison. Look up branch opcode by lhs base type.
	leftType := getExpressionType(left)
	if leftType == nil {
		diagnostics.ReportErrorAt(g.diagnostics, left, diagnostics.MessageTypeHasNoBaseType, "<nil>")
		return
	}
	base, ok := leftType.BaseType()
	if !ok {
		diagnostics.ReportErrorAt(g.diagnostics, left, diagnostics.MessageTypeHasNoBaseType, leftType.Representation())
		return
	}
	mappings := branchMappings(base)
	if mappings == nil {
		panic(fmt.Sprintf("generateCondition: no branch mapping for BaseVarType %d", base))
	}
	branchOp, ok := mappings[opText]
	if !ok {
		panic("generateCondition: no branch opcode for operator " + opText)
	}

	g.VisitNodeOrNull(left)
	g.VisitNodeOrNull(right)
	// Condition branches carry no source — mirrors TS CodeGenerator.ts
	// L310-311 (`undefined` source). leftSrc(left) was emitting a real
	// source that, while subsumed by prior pushes on the same line in most
	// cases, is a TS-parity divergence.
	g.Instruction(branchOp, branchTrue, lexer.NodeSourceLocation{})
	g.Instruction(Branch, branchFalse, lexer.NodeSourceLocation{})
	_ = block // present for parity with TS; the active-block contract handles it.
}

// getExpressionType reads the Type field from an Expression's ExpressionBase
// mixin. Mirrors TS expr.type. Imported helper from semantics is unexported;
// codegen has its own.
func getExpressionType(expr ast.Expression) typ.Type {
	if expr == nil {
		return nil
	}
	if eb, ok := expressionBaseOf(expr); ok && eb != nil {
		if t, ok2 := eb.Type.(typ.Type); ok2 {
			return t
		}
	}
	return nil
}

// expressionBaseOf returns a pointer to the ExpressionBase embedded in expr.
// Returns (nil, false) if expr does not embed ExpressionBase.
//
// This type-switch duplicates the pattern in semantics/dynamic_command.go's
// setTypeHint/getType/setType family; it is intentional because the codegen
// package cannot import semantics.
func expressionBaseOf(expr ast.Expression) (*ast.ExpressionBase, bool) {
	switch e := expr.(type) {
	case *ast.IntegerLiteral:
		return &e.ExpressionBase, true
	case *ast.CoordLiteral:
		return &e.ExpressionBase, true
	case *ast.BooleanLiteral:
		return &e.ExpressionBase, true
	case *ast.CharacterLiteral:
		return &e.ExpressionBase, true
	case *ast.StringLiteral:
		return &e.ExpressionBase, true
	case *ast.NullLiteral:
		return &e.ExpressionBase, true
	case *ast.Identifier:
		return &e.ExpressionBase, true
	case *ast.LocalVariableExpression:
		return &e.ExpressionBase, true
	case *ast.GameVariableExpression:
		return &e.ExpressionBase, true
	case *ast.ConstantVariableExpression:
		return &e.ExpressionBase, true
	case *ast.ConditionExpression:
		return &e.ExpressionBase, true
	case *ast.ArithmeticExpression:
		return &e.ExpressionBase, true
	case *ast.CalcExpression:
		return &e.ExpressionBase, true
	case *ast.ParenthesizedExpression:
		return &e.ExpressionBase, true
	case *ast.JoinedStringExpression:
		return &e.ExpressionBase, true
	case *ast.CommandCallExpression:
		return &e.ExpressionBase, true
	case *ast.ProcCallExpression:
		return &e.ExpressionBase, true
	case *ast.JumpCallExpression:
		return &e.ExpressionBase, true
	case *ast.ClientScriptExpression:
		return &e.ExpressionBase, true
	}
	return nil, false
}

// opShorthand returns a short human-readable representation of n's type.
func opShorthand(n ast.Node) string {
	if n == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%T", n)
}

