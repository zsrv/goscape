// pkg/pack/compiler/diagnostics/diagnostics_test.go
package diagnostics

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
)

func TestDiagnostics_ReportAndList(t *testing.T) {
	d := &Diagnostics{}
	loc := lexer.NodeSourceLocation{Name: "f.rs2", Line: 1}
	d.Report(NewDiagnostic(loc, DiagnosticInfo, MessageGenericInvalidType, "x"))
	d.Report(NewDiagnostic(loc, DiagnosticError, MessageGenericInvalidType, "y"))

	got := d.List()
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if !d.HasErrors() {
		t.Fatal("HasErrors() = false, want true (Error is in the list)")
	}
}

func TestDiagnostics_NoErrors(t *testing.T) {
	d := &Diagnostics{}
	loc := lexer.NodeSourceLocation{Name: "f.rs2", Line: 1}
	d.Report(NewDiagnostic(loc, DiagnosticInfo, MessageGenericInvalidType, "x"))
	d.Report(NewDiagnostic(loc, DiagnosticWarning, MessageGenericInvalidType, "y"))
	if d.HasErrors() {
		t.Fatal("HasErrors() = true, want false (no Error or SyntaxError)")
	}
}
