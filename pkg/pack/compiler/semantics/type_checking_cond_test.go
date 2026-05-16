package semantics

import (
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func TestVisitParenthesizedExpression_RelaysHintAndType(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	inner := &ast.IntegerLiteral{Value: 1}
	inner.Type = typ.PrimitiveInt
	paren := &ast.ParenthesizedExpression{Expression: inner}
	paren.TypeHint = typ.PrimitiveInt
	tc.Visit(paren)
	gotType, _ := paren.Type.(typ.Type)
	if gotType != typ.PrimitiveInt {
		t.Errorf("paren.Type = %v, want PrimitiveInt", paren.Type)
	}
}

func TestVisitConditionExpression_BasicEquality(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	left := &ast.IntegerLiteral{Value: 1}
	left.Type = typ.PrimitiveInt
	right := &ast.IntegerLiteral{Value: 2}
	right.Type = typ.PrimitiveInt
	c := &ast.ConditionExpression{
		Left:     left,
		Operator: &ast.Token{Text: "="},
		Right:    right,
	}
	tc.Visit(c)
	gotType, _ := c.Type.(typ.Type)
	if gotType != typ.PrimitiveBoolean {
		t.Errorf("c.Type = %v, want PrimitiveBoolean; diags=%v", c.Type, tc.diagnostics.List())
	}
}

func TestVisitConditionExpression_LogicalAndDisabled(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	tc.features.DisableLogicalAnd = true
	left := &ast.BooleanLiteral{Value: true}
	left.Type = typ.PrimitiveBoolean
	right := &ast.BooleanLiteral{Value: false}
	right.Type = typ.PrimitiveBoolean
	c := &ast.ConditionExpression{
		Left:     left,
		Operator: &ast.Token{Text: "&"},
		Right:    right,
	}
	tc.Visit(c)
	found := false
	for _, d := range tc.diagnostics.List() {
		if strings.Contains(d.Message, "is disabled") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected FEATURE_DISABLED_OPERATOR; got %v", tc.diagnostics.List())
	}
}

func TestCheckCondition_NonBinaryExpression(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	// Bare IntegerLiteral as condition — TS findInvalidConditionExpression
	// returns it ⇒ MessageConditionInvalidNodeType.
	tc.checkCondition(&ast.IntegerLiteral{Value: 1})
	found := false
	for _, d := range tc.diagnostics.List() {
		if strings.Contains(d.Message, "only allowed to be binary") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected MessageConditionInvalidNodeType; got %v", tc.diagnostics.List())
	}
}

func TestIsConditionExpression_BareCondition(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	c := &ast.ConditionExpression{}
	if !tc.isConditionExpression(c) {
		t.Error("bare ConditionExpression should be classified as condition")
	}
}

func TestIsConditionExpression_ParenthesizedCondition(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	c := &ast.ConditionExpression{}
	p := &ast.ParenthesizedExpression{Expression: c}
	if !tc.isConditionExpression(p) {
		t.Error("ParenthesizedExpression around ConditionExpression should be classified as condition")
	}
}
