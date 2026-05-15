package parser

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
)

func parseSingleExprStmt(t *testing.T, src string) ast.Expression {
	t.Helper()
	stmts, _ := parseSingleScript(t, src+";")
	es, ok := stmts[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("stmts[0] = %T, want *ExpressionStatement", stmts[0])
	}
	return es.Expression
}

func TestParseExpression_IntegerDecimal(t *testing.T) {
	e := parseSingleExprStmt(t, "42")
	il, ok := e.(*ast.IntegerLiteral)
	if !ok {
		t.Fatalf("expr = %T, want *IntegerLiteral", e)
	}
	if il.Value != 42 {
		t.Errorf("Value = %d, want 42", il.Value)
	}
}

func TestParseExpression_IntegerNegative(t *testing.T) {
	e := parseSingleExprStmt(t, "-5")
	il, ok := e.(*ast.IntegerLiteral)
	if !ok {
		t.Fatalf("expr = %T, want *IntegerLiteral", e)
	}
	if il.Value != -5 {
		t.Errorf("Value = %d, want -5", il.Value)
	}
}

func TestParseExpression_IntegerHex(t *testing.T) {
	e := parseSingleExprStmt(t, "0xff")
	il, ok := e.(*ast.IntegerLiteral)
	if !ok {
		t.Fatalf("expr = %T, want *IntegerLiteral", e)
	}
	if il.Value != 0xff {
		t.Errorf("Value = %d, want 0xff", il.Value)
	}
}

func TestParseExpression_IntegerBin(t *testing.T) {
	e := parseSingleExprStmt(t, "0b1010")
	il, ok := e.(*ast.IntegerLiteral)
	if !ok {
		t.Fatalf("expr = %T, want *IntegerLiteral", e)
	}
	if il.Value != 10 {
		t.Errorf("Value = %d, want 10", il.Value)
	}
}

func TestParseExpression_Boolean(t *testing.T) {
	e := parseSingleExprStmt(t, "true")
	bl, ok := e.(*ast.BooleanLiteral)
	if !ok {
		t.Fatalf("expr = %T, want *BooleanLiteral", e)
	}
	if !bl.Value {
		t.Errorf("Value = false, want true")
	}
}

func TestParseExpression_Null(t *testing.T) {
	e := parseSingleExprStmt(t, "null")
	nl, ok := e.(*ast.NullLiteral)
	if !ok {
		t.Fatalf("expr = %T, want *NullLiteral", e)
	}
	if nl.Value() != -1 {
		t.Errorf("Value() = %d, want -1", nl.Value())
	}
}

func TestParseExpression_Character(t *testing.T) {
	e := parseSingleExprStmt(t, "'x'")
	cl, ok := e.(*ast.CharacterLiteral)
	if !ok {
		t.Fatalf("expr = %T, want *CharacterLiteral", e)
	}
	if cl.Value != "x" {
		t.Errorf("Value = %q, want %q", cl.Value, "x")
	}
}

func TestParseExpression_CoordLiteral(t *testing.T) {
	e := parseSingleExprStmt(t, "0_50_50_0_0")
	cl, ok := e.(*ast.CoordLiteral)
	if !ok {
		t.Fatalf("expr = %T, want *CoordLiteral", e)
	}
	// x=(50<<6)|(0&0x3fff)=3200; z=(50<<6)|(0&0x3fff)=3200; y=0
	// packed = 3200|(3200<<14)|(0<<28) = 52432000
	// Parity with TS AstBuilder.visitCoordLiteral lines 306-316.
	if cl.Value != 52432000 {
		t.Errorf("Value = %d, want 52432000", cl.Value)
	}
}

func TestParseExpression_StringLiteralNoInterp(t *testing.T) {
	e := parseSingleExprStmt(t, `"hello"`)
	sl, ok := e.(*ast.StringLiteral)
	if !ok {
		t.Fatalf("expr = %T, want *StringLiteral", e)
	}
	if sl.Value != "hello" {
		t.Errorf("Value = %q, want %q", sl.Value, "hello")
	}
}

func TestParseExpression_StringLiteralWithEscape(t *testing.T) {
	e := parseSingleExprStmt(t, `"hello \"world\""`)
	sl, ok := e.(*ast.StringLiteral)
	if !ok {
		t.Fatalf("expr = %T, want *StringLiteral", e)
	}
	if sl.Value != `hello "world"` {
		t.Errorf("Value = %q, want %q", sl.Value, `hello "world"`)
	}
}

func TestParseExpression_LocalVariable(t *testing.T) {
	e := parseSingleExprStmt(t, "$var")
	lv, ok := e.(*ast.LocalVariableExpression)
	if !ok {
		t.Fatalf("expr = %T, want *LocalVariableExpression", e)
	}
	if lv.Name.Text != "var" {
		t.Errorf("Name.Text = %q, want %q", lv.Name.Text, "var")
	}
	if lv.IsArray() {
		t.Error("IsArray = true, want false")
	}
}

func TestParseExpression_LocalArrayVariable(t *testing.T) {
	e := parseSingleExprStmt(t, "$arr(0)")
	lv, ok := e.(*ast.LocalVariableExpression)
	if !ok {
		t.Fatalf("expr = %T, want *LocalVariableExpression", e)
	}
	if !lv.IsArray() {
		t.Fatal("IsArray = false, want true")
	}
	if lv.Index == nil {
		t.Fatal("Index = nil")
	}
}

func TestParseExpression_GameVariableMod(t *testing.T) {
	e := parseSingleExprStmt(t, "%var")
	gv, ok := e.(*ast.GameVariableExpression)
	if !ok {
		t.Fatalf("expr = %T, want *GameVariableExpression", e)
	}
	if gv.Dot {
		t.Error("Dot = true, want false")
	}
	if gv.Name.Text != "var" {
		t.Errorf("Name.Text = %q, want %q", gv.Name.Text, "var")
	}
}

func TestParseExpression_GameVariableDotMod(t *testing.T) {
	e := parseSingleExprStmt(t, ".%var")
	gv, ok := e.(*ast.GameVariableExpression)
	if !ok {
		t.Fatalf("expr = %T, want *GameVariableExpression", e)
	}
	if !gv.Dot {
		t.Error("Dot = false, want true")
	}
}

func TestParseExpression_ConstantVariable(t *testing.T) {
	e := parseSingleExprStmt(t, "^max")
	cv, ok := e.(*ast.ConstantVariableExpression)
	if !ok {
		t.Fatalf("expr = %T, want *ConstantVariableExpression", e)
	}
	if cv.Name.Text != "max" {
		t.Errorf("Name.Text = %q, want %q", cv.Name.Text, "max")
	}
}

func TestParseExpression_BareIdentifier(t *testing.T) {
	e := parseSingleExprStmt(t, "some_name")
	id, ok := e.(*ast.Identifier)
	if !ok {
		t.Fatalf("expr = %T, want *Identifier", e)
	}
	if id.Text != "some_name" {
		t.Errorf("Text = %q, want %q", id.Text, "some_name")
	}
}

func TestParseExpressionStatement(t *testing.T) {
	stmts, _ := parseSingleScript(t, "42;")
	es, ok := stmts[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("stmts[0] = %T, want *ExpressionStatement", stmts[0])
	}
	if _, ok := es.Expression.(*ast.IntegerLiteral); !ok {
		t.Fatalf("Expression = %T, want *IntegerLiteral", es.Expression)
	}
}

func TestParseAssignmentStatement_Single(t *testing.T) {
	stmts, _ := parseSingleScript(t, "$var = 0;")
	as, ok := stmts[0].(*ast.AssignmentStatement)
	if !ok {
		t.Fatalf("stmts[0] = %T, want *AssignmentStatement", stmts[0])
	}
	if got, want := len(as.Vars), 1; got != want {
		t.Fatalf("len(Vars) = %d, want %d", got, want)
	}
	if got, want := len(as.Expressions), 1; got != want {
		t.Fatalf("len(Expressions) = %d, want %d", got, want)
	}
}

func TestParseAssignmentStatement_Multi(t *testing.T) {
	stmts, _ := parseSingleScript(t, "$a, $b = 1, 2;")
	as := stmts[0].(*ast.AssignmentStatement)
	if got, want := len(as.Vars), 2; got != want {
		t.Fatalf("len(Vars) = %d, want %d", got, want)
	}
	if got, want := len(as.Expressions), 2; got != want {
		t.Fatalf("len(Expressions) = %d, want %d", got, want)
	}
}

func TestParseAssignmentStatement_GameVarLhs(t *testing.T) {
	stmts, _ := parseSingleScript(t, "%var = 1;")
	as := stmts[0].(*ast.AssignmentStatement)
	if _, ok := as.Vars[0].(*ast.GameVariableExpression); !ok {
		t.Fatalf("Vars[0] = %T, want *GameVariableExpression", as.Vars[0])
	}
}
