// pkg/pack/compiler/diagnostics/parser_error_listener_test.go
package diagnostics

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
)

// TestParserErrorListener_SyntaxErrorPushesDiagnostic pins that one
// SyntaxError callback produces one SyntaxError-typed Diagnostic with the
// constructor's sourceName and the callback's line/column/msg captured.
// Mirrors TS ParserErrorListener.syntaxError.
func TestParserErrorListener_SyntaxErrorPushesDiagnostic(t *testing.T) {
	d := &Diagnostics{}
	p := NewParserErrorListener("foo.rs2", d)

	p.SyntaxError("foo.rs2", 4, 7, "expected token")

	got := d.List()
	if len(got) != 1 {
		t.Fatalf("got %d diagnostics, want 1", len(got))
	}
	entry := got[0]
	if entry.Type != DiagnosticSyntaxError {
		t.Errorf("Type: got %v, want DiagnosticSyntaxError", entry.Type)
	}
	want := lexer.NodeSourceLocation{Name: "foo.rs2", Line: 4, Column: 7}
	if entry.SourceLocation != want {
		t.Errorf("SourceLocation: got %+v, want %+v", entry.SourceLocation, want)
	}
	if entry.Message != "%s" {
		t.Errorf("Message: got %q, want %q", entry.Message, "%s")
	}
	if len(entry.MessageArgs) != 1 || entry.MessageArgs[0] != "expected token" {
		t.Errorf("MessageArgs: got %v, want [\"expected token\"]", entry.MessageArgs)
	}
}

// TestParserErrorListener_SourceNameOverridesCallback pins that the
// constructor's sourceName wins over the callback's sourceName arg.
// Mirrors TS ParserErrorListener which captures the file at construction.
func TestParserErrorListener_SourceNameOverridesCallback(t *testing.T) {
	d := &Diagnostics{}
	p := NewParserErrorListener("ctor.rs2", d)

	p.SyntaxError("cb.rs2", 1, 1, "msg")

	got := d.List()
	if len(got) != 1 {
		t.Fatalf("got %d diagnostics, want 1", len(got))
	}
	if got[0].SourceLocation.Name != "ctor.rs2" {
		t.Errorf("SourceLocation.Name: got %q, want %q", got[0].SourceLocation.Name, "ctor.rs2")
	}
}
