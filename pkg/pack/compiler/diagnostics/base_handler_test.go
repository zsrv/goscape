// pkg/pack/compiler/diagnostics/base_handler_test.go
package diagnostics

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/pack/compiler/lexer"
)

// writeFile is a test helper used across base_handler_test.go to seed
// fixture files into t.TempDir() before invoking the handler.
func writeFile(t *testing.T, path, content string) error {
	t.Helper()
	return os.WriteFile(path, []byte(content), 0o644)
}

// TestBaseDiagnosticsHandler_FormatsLocationTypeMessage pins the TS L98
// "<path>:<line>:<column>: <type>: <message>" header line. Mirrors TS
// BaseDiagnosticsHandler.handleShared L100.
func TestBaseDiagnosticsHandler_FormatsLocationTypeMessage(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "f.rs2")
	// File contents don't matter for this test; we only assert the header.
	if err := writeFile(t, tmp, "irrelevant\n"); err != nil {
		t.Fatalf("write tmp: %v", err)
	}

	var buf bytes.Buffer
	h := &BaseDiagnosticsHandler{Out: &buf}
	d := &Diagnostics{}
	d.Report(Diagnostic{
		Type:           DiagnosticError,
		SourceLocation: lexer.NodeSourceLocation{Name: tmp, Line: 1, Column: 1},
		Message:        "boom",
	})

	h.HandleParse(d)

	got := buf.String()
	want := tmp + ":1:1: ERROR: boom"
	if !strings.Contains(got, want) {
		t.Errorf("output missing header %q; got:\n%s", want, got)
	}
}

// TestBaseDiagnosticsHandler_RendersSourceLineAndCaret pins the source-line
// readout (with tabs expanded to 4 spaces) and the caret-pointer offset.
// Mirrors TS handleShared L102-116. The test file uses a literal tab on
// line 2 col 2 (1-based: '\t' is col 1, 'h' is col 2) — so caret offset =
// tabCount*3 + (col-1) = 3 + 1 = 4 spaces.
func TestBaseDiagnosticsHandler_RendersSourceLineAndCaret(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "f.rs2")
	if err := writeFile(t, tmp, "line1\n\thello\nline3\n"); err != nil {
		t.Fatalf("write tmp: %v", err)
	}

	var buf bytes.Buffer
	h := &BaseDiagnosticsHandler{Out: &buf}
	d := &Diagnostics{}
	d.Report(Diagnostic{
		Type:           DiagnosticError,
		SourceLocation: lexer.NodeSourceLocation{Name: tmp, Line: 2, Column: 2},
		Message:        "bad",
	})

	h.HandleParse(d)

	got := buf.String()
	wantSrc := "    >     hello" // 4-space prefix + tab-expanded "    hello"
	wantCaret := "    > " + strings.Repeat(" ", 4) + "^"
	if !strings.Contains(got, wantSrc) {
		t.Errorf("output missing source line %q; got:\n%s", wantSrc, got)
	}
	if !strings.Contains(got, wantCaret) {
		t.Errorf("output missing caret line %q; got:\n%s", wantCaret, got)
	}
}
