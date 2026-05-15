// pkg/pack/compiler/semantics/type_checking_stmt_test.go
package semantics

import (
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

func TestVisitEmptyStatement_NoOp(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	tc.Visit(&ast.EmptyStatement{})
	if got := len(tc.diagnostics.List()); got != 0 {
		t.Errorf("emit count = %d, want 0 for empty stmt", got)
	}
}

func TestVisitReturnStatement_Orphan(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	rs := &ast.ReturnStatement{}
	// No currentScript ⇒ orphan return ⇒ MessageReturnOrphan.
	tc.Visit(rs)
	if got := len(tc.diagnostics.List()); got != 1 {
		t.Fatalf("emit count = %d, want 1 orphan return diag", got)
	}
	if !strings.Contains(tc.diagnostics.List()[0].Message, "Orphaned") {
		t.Errorf("diag = %q, want it to mention 'Orphaned'", tc.diagnostics.List()[0].Message)
	}
}

func TestVisitReturnStatement_VoidReturn_NoError(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	// Script declares void return (MetaUnit); return-stmt has no
	// expressions. typeHintExpressionList([]) returns [] → TupleFromList
	// returns MetaUnit on both sides → checkTypeMatch passes.
	// T17 (IntegerLiteral arm) is not needed here.
	s := &ast.Script{
		Name:       &ast.Identifier{Text: "foo"},
		ReturnType: typ.MetaUnit,
	}
	tc.currentScript = s
	rs := &ast.ReturnStatement{} // no expressions
	tc.Visit(rs)
	if got := len(tc.diagnostics.List()); got != 0 {
		t.Errorf("emit count = %d, want 0 (void return in void script); diags=%v", got, tc.diagnostics.List())
	}
}

func TestVisitReturnStatement_TypeMismatch_EmitsError(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	// Script declares int return; return-stmt has no expressions →
	// actual = MetaUnit, expected = PrimitiveInt → mismatch → 1 error.
	s := &ast.Script{
		Name:       &ast.Identifier{Text: "foo"},
		ReturnType: typ.PrimitiveInt,
	}
	tc.currentScript = s
	rs := &ast.ReturnStatement{} // no expressions — actual=MetaUnit
	tc.Visit(rs)
	if got := len(tc.diagnostics.List()); got != 1 {
		t.Errorf("emit count = %d, want 1 (type mismatch diag); diags=%v", got, tc.diagnostics.List())
	}
}

func TestVisitBlockStatement_NoCrash(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	rootTable := tc.table
	block := &ast.BlockStatement{Statements: []ast.Statement{&ast.EmptyStatement{}}}
	tc.Visit(block)
	if tc.table != rootTable {
		t.Error("after block visit: tc.table not restored to root")
	}
}

func TestVisitBlockStatement_NestedDoesNotCrash(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	inner := &ast.BlockStatement{Statements: []ast.Statement{&ast.EmptyStatement{}}}
	outer := &ast.BlockStatement{Statements: []ast.Statement{inner}}
	tc.Visit(outer)
	if got := len(tc.diagnostics.List()); got != 0 {
		t.Errorf("emit = %d, want 0 for fully-empty nested blocks", got)
	}
}

func TestVisitIfStatement_NoCrash(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	cond := &ast.BooleanLiteral{Value: true}
	cond.Type = typ.PrimitiveBoolean
	is := &ast.IfStatement{
		Condition:     cond,
		ThenStatement: &ast.EmptyStatement{},
		ElseStatement: &ast.EmptyStatement{},
	}
	tc.Visit(is)
	// T8 checkCondition is a stub (no boolean enforcement) — assertion
	// is only that visit completes without crash. T12 lands the validator.
}

func TestVisitWhileStatement_NoCrash(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	cond := &ast.BooleanLiteral{Value: true}
	cond.Type = typ.PrimitiveBoolean
	ws := &ast.WhileStatement{
		Condition:     cond,
		ThenStatement: &ast.EmptyStatement{},
	}
	tc.Visit(ws)
}

func TestVisitScriptFile_VisitsAllScripts(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	// Two scripts with empty body — no diagnostics expected (empty
	// statements are no-ops) assuming nil Block falls through gracefully.
	s1 := &ast.Script{Name: &ast.Identifier{Text: "a"}}
	s2 := &ast.Script{Name: &ast.Identifier{Text: "b"}}
	sf := &ast.ScriptFile{Scripts: []*ast.Script{s1, s2}}
	tc.Visit(sf)
	// No crash, no diagnostics (no statements to check).
	if got := len(tc.diagnostics.List()); got != 0 {
		t.Errorf("emit = %d, want 0 for empty-body script file", got)
	}
}

func TestVisitScript_SetsAndRestoresCurrentScript(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	// A return inside a script body should NOT be orphaned — currentScript
	// is set during visitScript's body traversal.
	// Use ReturnType=MetaUnit (void) and empty expression list.
	s := &ast.Script{
		Name:       &ast.Identifier{Text: "void_script"},
		ReturnType: typ.MetaUnit,
		Statements: []ast.Statement{
			&ast.ReturnStatement{}, // no expressions — void return
		},
	}
	tc.Visit(s)
	// currentScript should be restored to nil after visiting.
	if tc.currentScript != nil {
		t.Error("after visitScript: currentScript not restored to nil")
	}
	// Void return against MetaUnit return type: no type mismatch.
	if got := len(tc.diagnostics.List()); got != 0 {
		t.Errorf("emit = %d, want 0 for void return in void script; diags=%v", got, tc.diagnostics.List())
	}
}
