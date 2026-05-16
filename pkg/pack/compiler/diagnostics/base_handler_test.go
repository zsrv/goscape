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
