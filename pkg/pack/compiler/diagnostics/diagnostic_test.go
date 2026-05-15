// pkg/pack/compiler/diagnostics/diagnostic_test.go
package diagnostics

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
)

func TestDiagnosticType_IsErrorTypes(t *testing.T) {
	cases := []struct {
		typ  DiagnosticType
		want bool
	}{
		{DiagnosticInfo, false},
		{DiagnosticHint, false},
		{DiagnosticWarning, false},
		{DiagnosticError, true},
		{DiagnosticSyntaxError, true},
	}
	for _, c := range cases {
		d := Diagnostic{Type: c.typ}
		if got := d.IsError(); got != c.want {
			t.Fatalf("IsError(%v) = %v, want %v", c.typ, got, c.want)
		}
	}
}

func TestNewDiagnostic_FieldShape(t *testing.T) {
	loc := lexer.NodeSourceLocation{Name: "f.rs2", Line: 3, Column: 4}
	d := NewDiagnostic(loc, DiagnosticError, MessageScriptTriggerInvalid, "proc")
	if d.Type != DiagnosticError {
		t.Fatalf("Type = %v, want Error", d.Type)
	}
	if d.SourceLocation != loc {
		t.Fatalf("SourceLocation = %+v, want %+v", d.SourceLocation, loc)
	}
	if d.Message != MessageScriptTriggerInvalid {
		t.Fatalf("Message = %q, want template constant", d.Message)
	}
	if len(d.MessageArgs) != 1 || d.MessageArgs[0] != "proc" {
		t.Fatalf("MessageArgs = %v, want [\"proc\"]", d.MessageArgs)
	}
}
