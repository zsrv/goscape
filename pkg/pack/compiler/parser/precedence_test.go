package parser

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
)

func TestParseParenthesizedExpression(t *testing.T) {
	e := parseSingleExprStmt(t, "(42)")
	pe, ok := e.(*ast.ParenthesizedExpression)
	if !ok {
		t.Fatalf("expr = %T, want *ParenthesizedExpression", e)
	}
	if _, ok := pe.Expression.(*ast.IntegerLiteral); !ok {
		t.Fatalf("inner = %T, want *IntegerLiteral", pe.Expression)
	}
}

func TestParseCommandCall_NoArgs(t *testing.T) {
	e := parseSingleExprStmt(t, "mes()")
	cc, ok := e.(*ast.CommandCallExpression)
	if !ok {
		t.Fatalf("expr = %T, want *CommandCallExpression", e)
	}
	if cc.Name.Text != "mes" {
		t.Errorf("Name.Text = %q, want %q", cc.Name.Text, "mes")
	}
	if cc.IsStar() {
		t.Error("IsStar = true, want false")
	}
	if got, want := len(cc.Arguments), 0; got != want {
		t.Errorf("len(Arguments) = %d, want %d", got, want)
	}
}

func TestParseCommandCall_OneArg(t *testing.T) {
	e := parseSingleExprStmt(t, `mes("hi")`)
	cc := e.(*ast.CommandCallExpression)
	if got, want := len(cc.Arguments), 1; got != want {
		t.Fatalf("len(Arguments) = %d, want %d", got, want)
	}
}

func TestParseCommandCall_Star(t *testing.T) {
	e := parseSingleExprStmt(t, "foo*(1)(2)")
	cc, ok := e.(*ast.CommandCallExpression)
	if !ok {
		t.Fatalf("expr = %T, want *CommandCallExpression", e)
	}
	if !cc.IsStar() {
		t.Fatal("IsStar = false, want true")
	}
	if got, want := len(cc.Arguments), 1; got != want {
		t.Fatalf("len(Arguments) = %d, want %d", got, want)
	}
	if got, want := len(cc.Arguments2), 1; got != want {
		t.Fatalf("len(Arguments2) = %d, want %d", got, want)
	}
}

func TestParseProcCall(t *testing.T) {
	e := parseSingleExprStmt(t, "~min(1, 2)")
	pc, ok := e.(*ast.ProcCallExpression)
	if !ok {
		t.Fatalf("expr = %T, want *ProcCallExpression", e)
	}
	if pc.Name.Text != "min" {
		t.Errorf("Name.Text = %q, want %q", pc.Name.Text, "min")
	}
	if got, want := len(pc.Arguments), 2; got != want {
		t.Errorf("len(Arguments) = %d, want %d", got, want)
	}
}

func TestParseProcCall_NoParens(t *testing.T) {
	e := parseSingleExprStmt(t, "~foo")
	pc, ok := e.(*ast.ProcCallExpression)
	if !ok {
		t.Fatalf("expr = %T, want *ProcCallExpression", e)
	}
	if got, want := len(pc.Arguments), 0; got != want {
		t.Errorf("len(Arguments) = %d, want %d", got, want)
	}
}

func TestParseJumpCall(t *testing.T) {
	e := parseSingleExprStmt(t, "@label(1)")
	jc, ok := e.(*ast.JumpCallExpression)
	if !ok {
		t.Fatalf("expr = %T, want *JumpCallExpression", e)
	}
	if jc.Name.Text != "label" {
		t.Errorf("Name.Text = %q, want %q", jc.Name.Text, "label")
	}
}

func TestParseCalcExpression(t *testing.T) {
	e := parseSingleExprStmt(t, "calc(1 + 2)")
	ce, ok := e.(*ast.CalcExpression)
	if !ok {
		t.Fatalf("expr = %T, want *CalcExpression", e)
	}
	ae, ok := ce.Expression.(*ast.ArithmeticExpression)
	if !ok {
		t.Fatalf("calc inner = %T, want *ArithmeticExpression", ce.Expression)
	}
	if ae.Operator.Text != "+" {
		t.Errorf("Operator.Text = %q, want %q", ae.Operator.Text, "+")
	}
}

func TestParseCalcExpression_PrecedenceMulOverAdd(t *testing.T) {
	e := parseSingleExprStmt(t, "calc(1 + 2 * 3)")
	ce := e.(*ast.CalcExpression)
	outer, ok := ce.Expression.(*ast.ArithmeticExpression)
	if !ok {
		t.Fatalf("outer = %T, want *ArithmeticExpression", ce.Expression)
	}
	if outer.Operator.Text != "+" {
		t.Errorf("outer op = %q, want %q", outer.Operator.Text, "+")
	}
	inner, ok := outer.Right.(*ast.ArithmeticExpression)
	if !ok {
		t.Fatalf("outer.Right = %T, want *ArithmeticExpression", outer.Right)
	}
	if inner.Operator.Text != "*" {
		t.Errorf("inner op = %q, want %q", inner.Operator.Text, "*")
	}
}

func TestParseCalcExpression_LeftAssoc(t *testing.T) {
	e := parseSingleExprStmt(t, "calc(1 - 2 - 3)")
	ce := e.(*ast.CalcExpression)
	outer := ce.Expression.(*ast.ArithmeticExpression)
	if outer.Operator.Text != "-" {
		t.Errorf("outer op = %q, want %q", outer.Operator.Text, "-")
	}
	if _, ok := outer.Right.(*ast.IntegerLiteral); !ok {
		t.Errorf("outer.Right = %T, want *IntegerLiteral (3)", outer.Right)
	}
	innerLeft, ok := outer.Left.(*ast.ArithmeticExpression)
	if !ok {
		t.Fatalf("outer.Left = %T, want *ArithmeticExpression", outer.Left)
	}
	if innerLeft.Operator.Text != "-" {
		t.Errorf("innerLeft op = %q, want %q", innerLeft.Operator.Text, "-")
	}
}

func TestParseIfStatement_ConditionPrecedence(t *testing.T) {
	stmts, _ := parseSingleScript(t, "if ($x < 5 & $y > 10) ;")
	is := stmts[0].(*ast.IfStatement)
	cond, ok := is.Condition.(*ast.ConditionExpression)
	if !ok {
		t.Fatalf("Condition = %T, want *ConditionExpression", is.Condition)
	}
	if cond.Operator.Text != "&" {
		t.Errorf("outer op = %q, want %q", cond.Operator.Text, "&")
	}
	left, ok := cond.Left.(*ast.ConditionExpression)
	if !ok {
		t.Fatalf("Condition.Left = %T, want *ConditionExpression", cond.Left)
	}
	if left.Operator.Text != "<" {
		t.Errorf("left op = %q, want %q", left.Operator.Text, "<")
	}
}

func TestParseExpression_ClientScriptStandaloneNotApplicable(t *testing.T) {
	stmts, _ := parseSingleScript(t, "mes(\"hi\");")
	_ = stmts
}

// TestParseCalcExpression_SpanCoversMultiLine pins the spec §8 rule:
// spanOfNodes must extend EndLine past the start Line for a binary
// expression whose operands are on different lines.
func TestParseCalcExpression_SpanCoversMultiLine(t *testing.T) {
	src := "[proc,t]\ncalc(1\n+\n2);"
	p, cl := newTestParserCollecting(t, src)
	sf := p.ParseScriptFile()
	if sf == nil {
		t.Fatalf("ParseScriptFile() = nil; errors: %+v", cl.Errors)
	}
	es := sf.Scripts[0].Statements[0].(*ast.ExpressionStatement)
	ce := es.Expression.(*ast.CalcExpression)
	ae := ce.Expression.(*ast.ArithmeticExpression)
	if ae.Source().EndLine <= ae.Source().Line {
		t.Errorf("span does not span multiple lines: Line=%d EndLine=%d",
			ae.Source().Line, ae.Source().EndLine)
	}
}
