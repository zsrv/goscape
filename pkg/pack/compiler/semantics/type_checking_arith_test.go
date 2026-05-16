package semantics

import (
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func TestVisitArithmetic_IntInt(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	left := &ast.IntegerLiteral{Value: 1}
	left.Type = typ.PrimitiveInt
	right := &ast.IntegerLiteral{Value: 2}
	right.Type = typ.PrimitiveInt
	a := &ast.ArithmeticExpression{
		Left:     left,
		Operator: &ast.Token{Text: "+"},
		Right:    right,
	}
	tc.Visit(a)
	gotType, _ := a.Type.(typ.Type)
	if gotType != typ.PrimitiveInt {
		t.Errorf("a.Type = %v, want PrimitiveInt; diags=%v", a.Type, tc.diagnostics.List())
	}
}

func TestVisitArithmetic_StringRejected(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	left := &ast.StringLiteral{Value: "x"}
	left.Type = typ.PrimitiveString
	right := &ast.IntegerLiteral{Value: 2}
	right.Type = typ.PrimitiveInt
	a := &ast.ArithmeticExpression{
		Left:     left,
		Operator: &ast.Token{Text: "+"},
		Right:    right,
	}
	tc.Visit(a)
	gotType, _ := a.Type.(typ.Type)
	if gotType != typ.MetaError {
		t.Errorf("a.Type = %v, want MetaError on string+int", a.Type)
	}
	found := false
	for _, d := range tc.diagnostics.List() {
		if strings.Contains(d.Message, "Operator") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected MessageBinopInvalidTypes; got %v", tc.diagnostics.List())
	}
}

func TestVisitCalc_FeatureDisabled(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	tc.features.DisableCalc = true
	inner := &ast.IntegerLiteral{Value: 1}
	inner.Type = typ.PrimitiveInt
	c := &ast.CalcExpression{Expression: inner}
	tc.Visit(c)
	found := false
	for _, d := range tc.diagnostics.List() {
		if strings.Contains(d.Message, "calc") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected MessageFeatureDisabledCalc; got %v", tc.diagnostics.List())
	}
	gotType, _ := c.Type.(typ.Type)
	if gotType != typ.MetaError {
		t.Errorf("c.Type = %v, want MetaError", c.Type)
	}
}

func TestVisitCalc_IntPasses(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	inner := &ast.IntegerLiteral{Value: 1}
	inner.Type = typ.PrimitiveInt
	c := &ast.CalcExpression{Expression: inner}
	tc.Visit(c)
	gotType, _ := c.Type.(typ.Type)
	if gotType != typ.PrimitiveInt {
		t.Errorf("c.Type = %v, want PrimitiveInt; diags=%v", c.Type, tc.diagnostics.List())
	}
}
