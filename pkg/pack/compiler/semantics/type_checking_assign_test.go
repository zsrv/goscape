package semantics

import (
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func TestVisitAssignment_MultiAssignArrayRejected(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	// LHS array element (Index non-nil ⇒ IsArray)
	arr := &ast.LocalVariableExpression{
		Name:  &ast.Identifier{Text: "arr"},
		Index: &ast.IntegerLiteral{Value: 0},
	}
	scalar := &ast.LocalVariableExpression{Name: &ast.Identifier{Text: "y"}}
	rhs1 := &ast.IntegerLiteral{Value: 1}
	rhs1.Type = typ.PrimitiveInt
	rhs2 := &ast.IntegerLiteral{Value: 2}
	rhs2.Type = typ.PrimitiveInt
	a := &ast.AssignmentStatement{
		Vars:        []ast.VariableExpressionNode{arr, scalar},
		Expressions: []ast.Expression{rhs1, rhs2},
	}
	tc.Visit(a)
	found := false
	for _, d := range tc.diagnostics.List() {
		if d.Message == diagnostics.MessageAssignMultiArray {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected MessageAssignMultiArray; got %v", tc.diagnostics.List())
	}
}

func TestVisitExpressionStatement_NoSideEffectWarns(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	lit := &ast.IntegerLiteral{Value: 1}
	lit.Type = typ.PrimitiveInt
	es := &ast.ExpressionStatement{Expression: lit}
	tc.Visit(es)
	found := false
	for _, d := range tc.diagnostics.List() {
		if strings.Contains(d.Message, "side effect") || d.Message == diagnostics.MessageExpressionStatementNoSideEffect {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected MessageExpressionStatementNoSideEffect; got %v", tc.diagnostics.List())
	}
}

func TestExpressionHasSideEffects_ProcCall(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	call := &ast.ProcCallExpression{Name: &ast.Identifier{Text: "doit"}}
	if !tc.expressionHasSideEffects(call) {
		t.Error("ProcCall should have side effects")
	}
}

func TestExpressionHasSideEffects_IntegerLiteral(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	if tc.expressionHasSideEffects(&ast.IntegerLiteral{Value: 5}) {
		t.Error("IntegerLiteral should NOT have side effects")
	}
}

func TestCommandHasSideEffects_Unit(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	if !tc.commandHasSideEffects(typ.MetaUnit) {
		t.Error("MetaUnit return ⇒ command has side effects (TS treats unit-return commands as side-effectful)")
	}
}

func TestCommandHasSideEffects_NonUnit(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	if tc.commandHasSideEffects(typ.PrimitiveInt) {
		t.Error("PrimitiveInt return ⇒ command has NO side effects (TS treats non-unit-return commands as pure)")
	}
}
