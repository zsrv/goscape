package parser

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
)

func TestParseSingleExpression_IntegerLiteral(t *testing.T) {
	p := NewSingleExpressionParser("42", "<const>")
	expr := p.ParseSingleExpression()
	if expr == nil {
		t.Fatal("ParseSingleExpression returned nil")
	}
	il, ok := expr.(*ast.IntegerLiteral)
	if !ok {
		t.Fatalf("got %T, want *ast.IntegerLiteral", expr)
	}
	if il.Value != 42 {
		t.Errorf("Value = %d, want 42", il.Value)
	}
}

func TestParseSingleExpression_StringLiteral(t *testing.T) {
	p := NewSingleExpressionParser(`"hello"`, "<const>")
	expr := p.ParseSingleExpression()
	if expr == nil {
		t.Fatal("ParseSingleExpression returned nil")
	}
	sl, ok := expr.(*ast.StringLiteral)
	if !ok {
		t.Fatalf("got %T, want *ast.StringLiteral", expr)
	}
	if sl.Value != "hello" {
		t.Errorf("Value = %q, want %q", sl.Value, "hello")
	}
}

func TestParseSingleExpression_SyntaxErrorReturnsNil(t *testing.T) {
	// With error listeners cleared, syntax errors should yield nil
	// (not a partial AST).
	p := NewSingleExpressionParser("def_int $bad", "<const>")
	p.RemoveErrorListeners()
	expr := p.ParseSingleExpression()
	if expr != nil {
		t.Errorf("expected nil for non-expression input, got %T", expr)
	}
}

func TestParseSingleExpression_TrailingTokensRejected(t *testing.T) {
	// "1 2" — first expression parses fine, but trailing "2" with no
	// connecting operator should fail (return nil + accumulate errors).
	p := NewSingleExpressionParser("1 2", "<const>")
	p.RemoveErrorListeners()
	expr := p.ParseSingleExpression()
	if expr != nil {
		t.Errorf("expected nil for input with trailing tokens, got %T", expr)
	}
}
