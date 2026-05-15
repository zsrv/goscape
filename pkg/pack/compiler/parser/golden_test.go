package parser

import (
	"os"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
)

func TestGoldenScriptRoundTrip(t *testing.T) {
	src, err := os.ReadFile("testdata/golden_script.src")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	p, cl := newTestParserCollecting(t, string(src))
	sf := p.ParseScriptFile()
	if sf == nil {
		t.Fatalf("ParseScriptFile() = nil; errors: %+v", cl.Errors)
	}
	if got, want := len(sf.Scripts), 1; got != want {
		t.Fatalf("len(Scripts) = %d, want %d", got, want)
	}
	s := sf.Scripts[0]
	if s.Trigger.Text != "proc" {
		t.Errorf("Trigger = %q, want %q", s.Trigger.Text, "proc")
	}
	if s.Name.Text != "test_script" {
		t.Errorf("Name = %q, want %q", s.Name.Text, "test_script")
	}
	if got, want := len(s.Parameters), 1; got != want {
		t.Fatalf("len(Parameters) = %d, want %d", got, want)
	}
	if got, want := len(s.Statements), 1; got != want {
		t.Fatalf("len(Statements) = %d, want %d", got, want)
	}
	es, ok := s.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("Statements[0] = %T, want *ExpressionStatement", s.Statements[0])
	}
	cc, ok := es.Expression.(*ast.CommandCallExpression)
	if !ok {
		t.Fatalf("Expression = %T, want *CommandCallExpression", es.Expression)
	}
	if cc.Name.Text != "mes" {
		t.Errorf("call name = %q, want %q", cc.Name.Text, "mes")
	}
	if got, want := len(cc.Arguments), 1; got != want {
		t.Fatalf("len(Arguments) = %d, want %d", got, want)
	}
	if _, ok := cc.Arguments[0].(*ast.JoinedStringExpression); !ok {
		t.Fatalf("Arguments[0] = %T, want *JoinedStringExpression", cc.Arguments[0])
	}
	if len(cl.Errors) != 0 {
		t.Errorf("unexpected errors: %+v", cl.Errors)
	}
}
