package parser

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
)

// parseSingleScript is a test helper: parses src with a single script
// wrapper around the supplied body, returning the body's parsed
// statements.
func parseSingleScript(t *testing.T, body string) ([]ast.Statement, []byte) {
	t.Helper()
	src := "[proc,t] " + body
	p, cl := newTestParserCollecting(t, src)
	sf := p.ParseScriptFile()
	if sf == nil {
		t.Fatalf("ParseScriptFile() = nil; errors: %+v", cl.Errors)
	}
	if got, want := len(sf.Scripts), 1; got != want {
		t.Fatalf("len(Scripts) = %d, want %d", got, want)
	}
	return sf.Scripts[0].Statements, nil
}

func TestParseBlockStatement_Empty(t *testing.T) {
	stmts, _ := parseSingleScript(t, "{}")
	if got, want := len(stmts), 1; got != want {
		t.Fatalf("len(stmts) = %d, want %d", got, want)
	}
	b, ok := stmts[0].(*ast.BlockStatement)
	if !ok {
		t.Fatalf("stmts[0] type = %T, want *BlockStatement", stmts[0])
	}
	if got, want := len(b.Statements), 0; got != want {
		t.Fatalf("len(b.Statements) = %d, want %d", got, want)
	}
}

func TestParseBlockStatement_WithInner(t *testing.T) {
	stmts, _ := parseSingleScript(t, "{ ; ; }")
	b := stmts[0].(*ast.BlockStatement)
	if got, want := len(b.Statements), 2; got != want {
		t.Fatalf("len(b.Statements) = %d, want %d", got, want)
	}
	if _, ok := b.Statements[0].(*ast.EmptyStatement); !ok {
		t.Fatalf("Statements[0] = %T, want *EmptyStatement", b.Statements[0])
	}
}

func TestParseEmptyStatement(t *testing.T) {
	stmts, _ := parseSingleScript(t, ";")
	if _, ok := stmts[0].(*ast.EmptyStatement); !ok {
		t.Fatalf("stmts[0] = %T, want *EmptyStatement", stmts[0])
	}
}

func TestParseReturnStatement_NoArgs(t *testing.T) {
	stmts, _ := parseSingleScript(t, "return;")
	r, ok := stmts[0].(*ast.ReturnStatement)
	if !ok {
		t.Fatalf("stmts[0] = %T, want *ReturnStatement", stmts[0])
	}
	if got, want := len(r.Expressions), 0; got != want {
		t.Fatalf("len(Expressions) = %d, want %d", got, want)
	}
}

func TestParseReturnStatement_EmptyParens(t *testing.T) {
	stmts, _ := parseSingleScript(t, "return();")
	r := stmts[0].(*ast.ReturnStatement)
	if got, want := len(r.Expressions), 0; got != want {
		t.Fatalf("len = %d, want %d", got, want)
	}
}

func TestParseReturnStatement_OneExpr(t *testing.T) {
	stmts, _ := parseSingleScript(t, "return(42);")
	r := stmts[0].(*ast.ReturnStatement)
	if got, want := len(r.Expressions), 1; got != want {
		t.Fatalf("len = %d, want %d", got, want)
	}
	lit, ok := r.Expressions[0].(*ast.IntegerLiteral)
	if !ok {
		t.Fatalf("expr type = %T, want *IntegerLiteral", r.Expressions[0])
	}
	if lit.Value != 42 {
		t.Errorf("Value = %d, want 42", lit.Value)
	}
}

func TestParseReturnStatement_MultiExpr(t *testing.T) {
	stmts, _ := parseSingleScript(t, "return(1, 2, 3);")
	r := stmts[0].(*ast.ReturnStatement)
	if got, want := len(r.Expressions), 3; got != want {
		t.Fatalf("len = %d, want %d", got, want)
	}
}

func TestParseIfStatement_NoElse(t *testing.T) {
	stmts, _ := parseSingleScript(t, "if (true) ;")
	is, ok := stmts[0].(*ast.IfStatement)
	if !ok {
		t.Fatalf("stmts[0] = %T, want *IfStatement", stmts[0])
	}
	if is.ElseStatement != nil {
		t.Fatalf("ElseStatement = %+v, want nil", is.ElseStatement)
	}
	if _, ok := is.ThenStatement.(*ast.EmptyStatement); !ok {
		t.Fatalf("ThenStatement = %T, want *EmptyStatement", is.ThenStatement)
	}
}

func TestParseIfStatement_WithElse(t *testing.T) {
	stmts, _ := parseSingleScript(t, "if (true) ; else ;")
	is := stmts[0].(*ast.IfStatement)
	if is.ElseStatement == nil {
		t.Fatal("ElseStatement = nil, want non-nil")
	}
	if _, ok := is.ElseStatement.(*ast.EmptyStatement); !ok {
		t.Fatalf("ElseStatement = %T, want *EmptyStatement", is.ElseStatement)
	}
}

func TestParseWhileStatement(t *testing.T) {
	stmts, _ := parseSingleScript(t, "while (true) ;")
	ws, ok := stmts[0].(*ast.WhileStatement)
	if !ok {
		t.Fatalf("stmts[0] = %T, want *WhileStatement", stmts[0])
	}
	if _, ok := ws.ThenStatement.(*ast.EmptyStatement); !ok {
		t.Fatalf("ThenStatement = %T, want *EmptyStatement", ws.ThenStatement)
	}
}
