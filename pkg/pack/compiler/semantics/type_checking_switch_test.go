package semantics

import (
	"fmt"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/diagnostics"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func TestVisitSwitchStatement_InvalidType(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	cond := &ast.IntegerLiteral{Value: 0}
	cond.Type = typ.PrimitiveInt
	sw := &ast.SwitchStatement{
		TypeToken: &ast.Token{Text: "switch_nonexistent"},
		Condition: cond,
	}
	tc.Visit(sw)
	if got := len(tc.diagnostics.List()); got < 1 {
		t.Fatal("expected at least 1 diagnostic for invalid switch type")
	}
	// MessageGenericInvalidType format string contains %s; MessageArgs[0] has the type name.
	// Formatted message: "'nonexistent' is not a valid type."
	found := false
	for _, d := range tc.diagnostics.List() {
		if d.Message == diagnostics.MessageGenericInvalidType {
			formatted := fmt.Sprintf(d.Message, d.MessageArgs...)
			if strings.Contains(formatted, "nonexistent") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("no GenericInvalidType diag mentioned 'nonexistent'; diags=%v", tc.diagnostics.List())
	}
}

func TestVisitSwitchStatement_DuplicateDefault(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	// Register PrimitiveInt so the switch_int lookup succeeds.
	if err := tc.typeManager.RegisterByRepresentation(typ.PrimitiveInt); err != nil {
		t.Fatalf("RegisterByRepresentation: %v", err)
	}
	d1 := &ast.SwitchCase{Keys: nil} // default
	d2 := &ast.SwitchCase{Keys: nil} // duplicate default
	cond := &ast.IntegerLiteral{Value: 0}
	cond.Type = typ.PrimitiveInt
	sw := &ast.SwitchStatement{
		TypeToken: &ast.Token{Text: "switch_int"},
		Condition: cond,
		Cases:     []*ast.SwitchCase{d1, d2},
	}
	tc.Visit(sw)
	found := false
	for _, d := range tc.diagnostics.List() {
		if strings.Contains(d.Message, "Duplicate default") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected MessageSwitchDuplicateDefault; got %v", tc.diagnostics.List())
	}
	if sw.DefaultCase != d1 {
		t.Errorf("DefaultCase = %p, want %p", sw.DefaultCase, d1)
	}
}

func TestIsConstantExpression_IntegerLiteral(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	if !tc.isConstantExpression(&ast.IntegerLiteral{Value: 7}) {
		t.Error("integer literal should be constant")
	}
}

func TestIsConstantExpression_BooleanLiteral(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	if !tc.isConstantExpression(&ast.BooleanLiteral{Value: true}) {
		t.Error("boolean literal should be constant")
	}
}

func TestIsConstantExpression_NullLiteral(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	if !tc.isConstantExpression(&ast.NullLiteral{}) {
		t.Error("null literal should be constant")
	}
}

func TestIsConstantExpression_ConstantVariable(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	cv := &ast.ConstantVariableExpression{Name: &ast.Identifier{Text: "FOO"}}
	if !tc.isConstantExpression(cv) {
		t.Error("constant variable should be constant")
	}
}

func TestIsConstantExpression_LocalVariable_NotConstant(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	lv := &ast.LocalVariableExpression{Name: &ast.Identifier{Text: "x"}}
	if tc.isConstantExpression(lv) {
		t.Error("local variable should NOT be constant")
	}
}
