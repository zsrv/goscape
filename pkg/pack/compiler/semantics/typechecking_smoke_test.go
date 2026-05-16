package semantics

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

// TestNAI206_Smoke_MinimalReturnStatement drives the simplest possible
// end-to-end shape through the TypeChecker: a Script with a Return-int
// statement and a pre-typed IntegerLiteral. Verifies the dispatch chain
// (visitScript → visitReturn → typeHintExpressionList → visitInteger)
// emits no diagnostics on a happy path.
func TestNAI206_Smoke_MinimalReturnStatement(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	il := &ast.IntegerLiteral{Value: 7}
	il.Type = typ.PrimitiveInt
	rs := &ast.ReturnStatement{Expressions: []ast.Expression{il}}
	s := &ast.Script{
		Name:       &ast.Identifier{Text: "test"},
		ReturnType: typ.PrimitiveInt,
		Statements: []ast.Statement{rs},
	}
	tc.Visit(s)
	if got := len(tc.diagnostics.List()); got != 0 {
		t.Errorf("smoke emitted %d diagnostics: %v", got, tc.diagnostics.List())
	}
}

// TestNAI206_Smoke_DispatchCoverage exercises every walker arm wired in
// T8-T18 to confirm none of them panic on a minimal valid input. Each
// arm is exercised individually rather than via a full parse — saves
// dependency on ScriptRegistration's exact API.
func TestNAI206_Smoke_DispatchCoverage(t *testing.T) {
	tc := newBasicCheckingFixture(t)
	// EmptyStatement (T8)
	tc.Visit(&ast.EmptyStatement{})
	// IntegerLiteral (T17)
	il := &ast.IntegerLiteral{Value: 1}
	tc.Visit(il)
	// BooleanLiteral (T17)
	tc.Visit(&ast.BooleanLiteral{Value: true})
	// CoordLiteral (T17)
	tc.Visit(&ast.CoordLiteral{Value: 0})
	// CharacterLiteral (T17)
	tc.Visit(&ast.CharacterLiteral{Value: "x"})
	// NullLiteral (T17)
	tc.Visit(&ast.NullLiteral{})
	// StringLiteral (T17)
	tc.Visit(&ast.StringLiteral{Value: "x"})
	// JoinedString (T17)
	tc.Visit(&ast.JoinedStringExpression{})
	// All cases above should complete without panic. Diagnostics may
	// or may not be emitted depending on context — the smoke only asserts
	// no-panic and no compile error.
}
