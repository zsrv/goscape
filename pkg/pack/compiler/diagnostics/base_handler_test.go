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

// TestBaseDiagnosticsHandler_LineOutOfBoundsSkipsSource asserts no `>` line
// is emitted when the diagnostic line exceeds the file length. The header
// still prints.
func TestBaseDiagnosticsHandler_LineOutOfBoundsSkipsSource(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "f.rs2")
	if err := writeFile(t, tmp, "a\nb\nc\n"); err != nil {
		t.Fatalf("write tmp: %v", err)
	}

	var buf bytes.Buffer
	h := &BaseDiagnosticsHandler{Out: &buf}
	d := &Diagnostics{}
	d.Report(Diagnostic{
		Type:           DiagnosticError,
		SourceLocation: lexer.NodeSourceLocation{Name: tmp, Line: 99, Column: 1},
		Message:        "oob",
	})

	h.HandleParse(d)

	got := buf.String()
	if !strings.Contains(got, "ERROR: oob") {
		t.Errorf("header missing; got:\n%s", got)
	}
	if strings.Contains(got, "    >") {
		t.Errorf("source/caret line should be skipped; got:\n%s", got)
	}
}

// TestBaseDiagnosticsHandler_FileMissingSkipsSource asserts no panic + no
// `>` line when the path does not exist on disk. The header still prints.
func TestBaseDiagnosticsHandler_FileMissingSkipsSource(t *testing.T) {
	var buf bytes.Buffer
	h := &BaseDiagnosticsHandler{Out: &buf}
	d := &Diagnostics{}
	d.Report(Diagnostic{
		Type:           DiagnosticError,
		SourceLocation: lexer.NodeSourceLocation{Name: "/nonexistent/path.rs2", Line: 1, Column: 1},
		Message:        "ghost",
	})

	h.HandleParse(d) // must not panic
	got := buf.String()
	if !strings.Contains(got, "ERROR: ghost") {
		t.Errorf("header missing; got:\n%s", got)
	}
	if strings.Contains(got, "    >") {
		t.Errorf("source/caret line should be skipped; got:\n%s", got)
	}
}

// TestBaseDiagnosticsHandler_MessageArgsFormatted pins that printf-style
// %s verbs in Message are substituted from MessageArgs (mirrors TS L100
// `util.format(message, ...messageArgs)`).
func TestBaseDiagnosticsHandler_MessageArgsFormatted(t *testing.T) {
	var buf bytes.Buffer
	h := &BaseDiagnosticsHandler{Out: &buf}
	d := &Diagnostics{}
	d.Report(Diagnostic{
		Type:           DiagnosticError,
		SourceLocation: lexer.NodeSourceLocation{Name: "x", Line: 1, Column: 1},
		Message:        "bad %s",
		MessageArgs:    []any{"foo"},
	})

	h.HandleParse(d)

	if !strings.Contains(buf.String(), "ERROR: bad foo") {
		t.Errorf("MessageArgs not formatted; got:\n%s", buf.String())
	}
}

// TestBaseDiagnosticsHandler_AllFourPhaseMethodsDispatchSame pins TS
// L58-72: each of the four HandleXxx methods delegates to handleShared
// with identical output.
func TestBaseDiagnosticsHandler_AllFourPhaseMethodsDispatchSame(t *testing.T) {
	mkDiag := func() *Diagnostics {
		d := &Diagnostics{}
		d.Report(Diagnostic{
			Type:           DiagnosticError,
			SourceLocation: lexer.NodeSourceLocation{Name: "x", Line: 1, Column: 1},
			Message:        "shared",
		})
		return d
	}

	captures := []string{}
	for _, call := range []func(h *BaseDiagnosticsHandler, d *Diagnostics){
		func(h *BaseDiagnosticsHandler, d *Diagnostics) { h.HandleParse(d) },
		func(h *BaseDiagnosticsHandler, d *Diagnostics) { h.HandleTypeChecking(d) },
		func(h *BaseDiagnosticsHandler, d *Diagnostics) { h.HandleCodeGeneration(d) },
		func(h *BaseDiagnosticsHandler, d *Diagnostics) { h.HandlePointerChecking(d) },
	} {
		var buf bytes.Buffer
		call(&BaseDiagnosticsHandler{Out: &buf}, mkDiag())
		captures = append(captures, buf.String())
	}

	for i := 1; i < len(captures); i++ {
		if captures[i] != captures[0] {
			t.Errorf("phase method %d output diverges:\nfirst:\n%s\nthis:\n%s", i, captures[0], captures[i])
		}
	}
}

// TestBaseDiagnosticsHandler_NoOsExit pins NAI-211-D-NO-PROCESS-EXIT:
// even when diagnostics contain ERRORs, handleShared returns normally
// (no os.Exit). Test-process survival is the assertion.
func TestBaseDiagnosticsHandler_NoOsExit(t *testing.T) {
	var buf bytes.Buffer
	h := &BaseDiagnosticsHandler{Out: &buf}
	d := &Diagnostics{}
	d.Report(Diagnostic{
		Type:           DiagnosticError,
		SourceLocation: lexer.NodeSourceLocation{Name: "x", Line: 1, Column: 1},
		Message:        "fatal-but-not-really",
	})
	if !d.HasErrors() {
		t.Fatal("test setup: expected HasErrors()==true")
	}
	h.HandleParse(d) // would call os.Exit(1) if NAI-211-D-NO-PROCESS-EXIT regressed
	// If we got here, the deviation is honored.
}
