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

func TestParseSwitchStatement_SingleCase(t *testing.T) {
	stmts, _ := parseSingleScript(t, "switch_int (1) { case 1 : return; }")
	s, ok := stmts[0].(*ast.SwitchStatement)
	if !ok {
		t.Fatalf("stmts[0] = %T, want *SwitchStatement", stmts[0])
	}
	if s.TypeToken.Text != "switch_int" {
		t.Errorf("TypeToken.Text = %q, want %q", s.TypeToken.Text, "switch_int")
	}
	if got, want := len(s.Cases), 1; got != want {
		t.Fatalf("len(Cases) = %d, want %d", got, want)
	}
	if got, want := len(s.Cases[0].Keys), 1; got != want {
		t.Fatalf("len(Cases[0].Keys) = %d, want %d", got, want)
	}
	if s.Cases[0].IsDefault() {
		t.Fatal("Cases[0] should not be default")
	}
}

func TestParseSwitchStatement_DefaultCase(t *testing.T) {
	stmts, _ := parseSingleScript(t, "switch_int (1) { case default : return; }")
	s := stmts[0].(*ast.SwitchStatement)
	if !s.Cases[0].IsDefault() {
		t.Fatal("Cases[0] should be default (empty Keys)")
	}
	if got, want := len(s.Cases[0].Keys), 0; got != want {
		t.Fatalf("len(Keys) = %d, want %d", got, want)
	}
}

func TestParseSwitchStatement_MultiKeyCase(t *testing.T) {
	stmts, _ := parseSingleScript(t, "switch_int (1) { case 1, 2, 3 : return; }")
	s := stmts[0].(*ast.SwitchStatement)
	if got, want := len(s.Cases[0].Keys), 3; got != want {
		t.Fatalf("len(Keys) = %d, want %d", got, want)
	}
}

func TestParseDeclarationStatement_NoInit(t *testing.T) {
	stmts, _ := parseSingleScript(t, "def_int $var;")
	d, ok := stmts[0].(*ast.DeclarationStatement)
	if !ok {
		t.Fatalf("stmts[0] = %T, want *DeclarationStatement", stmts[0])
	}
	if d.TypeToken.Text != "def_int" {
		t.Errorf("TypeToken.Text = %q, want %q", d.TypeToken.Text, "def_int")
	}
	if d.Name.Text != "var" {
		t.Errorf("Name.Text = %q, want %q", d.Name.Text, "var")
	}
	if d.Initializer != nil {
		t.Errorf("Initializer = %+v, want nil", d.Initializer)
	}
}

func TestParseDeclarationStatement_WithInit(t *testing.T) {
	stmts, _ := parseSingleScript(t, "def_int $var = 0;")
	d := stmts[0].(*ast.DeclarationStatement)
	if d.Initializer == nil {
		t.Fatal("Initializer = nil, want non-nil")
	}
	lit, ok := d.Initializer.(*ast.IntegerLiteral)
	if !ok {
		t.Fatalf("Initializer type = %T, want *IntegerLiteral", d.Initializer)
	}
	if lit.Value != 0 {
		t.Errorf("Value = %d, want 0", lit.Value)
	}
}

func TestParseArrayDeclarationStatement(t *testing.T) {
	stmts, _ := parseSingleScript(t, "def_int $ints(50);")
	a, ok := stmts[0].(*ast.ArrayDeclarationStatement)
	if !ok {
		t.Fatalf("stmts[0] = %T, want *ArrayDeclarationStatement", stmts[0])
	}
	if a.Name.Text != "ints" {
		t.Errorf("Name.Text = %q, want %q", a.Name.Text, "ints")
	}
	lit, ok := a.Initializer.(*ast.IntegerLiteral)
	if !ok {
		t.Fatalf("Initializer = %T, want *IntegerLiteral", a.Initializer)
	}
	if lit.Value != 50 {
		t.Errorf("Initializer value = %d, want 50", lit.Value)
	}
}
