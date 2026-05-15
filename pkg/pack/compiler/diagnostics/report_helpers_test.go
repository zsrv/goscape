// pkg/pack/compiler/diagnostics/report_helpers_test.go
package diagnostics

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/ast"
	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
)

func TestReportErrorAt_NodeLocation(t *testing.T) {
	loc := lexer.NodeSourceLocation{Name: "f.rs2", Line: 5, Column: 2}
	node := &ast.Identifier{SrcLoc: loc, Text: "foo"}
	d := &Diagnostics{}
	ReportErrorAt(d, node, MessageScriptTriggerInvalid, "proc")
	list := d.List()
	if len(list) != 1 {
		t.Fatalf("list len = %d, want 1", len(list))
	}
	if list[0].Type != DiagnosticError {
		t.Fatalf("Type = %v, want Error", list[0].Type)
	}
	if list[0].SourceLocation != loc {
		t.Fatalf("SourceLocation = %+v, want %+v", list[0].SourceLocation, loc)
	}
}

func TestReportAt_PreservesType(t *testing.T) {
	loc := lexer.NodeSourceLocation{Name: "f.rs2", Line: 1}
	node := &ast.Token{SrcLoc: loc, Text: "x"}
	d := &Diagnostics{}
	ReportAt(d, node, DiagnosticWarning, MessageGenericInvalidType, "x")
	if list := d.List(); len(list) != 1 || list[0].Type != DiagnosticWarning {
		t.Fatalf("list = %+v", list)
	}
}
