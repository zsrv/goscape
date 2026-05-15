package compiler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadCompilerConstants_StripsLeadingCaret pins TS Compiler.ts:162-164:
// names beginning with '^' have the '^' stripped before storage.
func TestLoadCompilerConstants_StripsLeadingCaret(t *testing.T) {
	dir := t.TempDir()
	writeConstantFile(t, dir, "a.constant", "^FOO=bar\n")

	m, err := loadCompilerConstants(filepath.Join(dir, "scripts"))
	if err != nil {
		t.Fatalf("loadCompilerConstants: %v", err)
	}
	if got, want := m["FOO"], "bar"; got != want {
		t.Errorf("m[\"FOO\"] = %q, want %q", got, want)
	}
	if _, present := m["^FOO"]; present {
		t.Errorf("m has both ^FOO and FOO; caret should have been stripped")
	}
}

// TestLoadCompilerConstants_StripsSurroundingQuotes pins TS Compiler.ts:166-169.
func TestLoadCompilerConstants_StripsSurroundingQuotes(t *testing.T) {
	dir := t.TempDir()
	writeConstantFile(t, dir, "a.constant",
		"A=\"quoted\"\nB=unquoted\nC=\"mismatch\nD=mismatch\"\nE=\"in\"middle\"\n",
	)

	m, err := loadCompilerConstants(filepath.Join(dir, "scripts"))
	if err != nil {
		t.Fatalf("loadCompilerConstants: %v", err)
	}
	cases := map[string]string{
		"A": "quoted",      // both-sided quotes stripped
		"B": "unquoted",    // no quotes, unchanged
		"C": "\"mismatch",  // open-only, unchanged
		"D": "mismatch\"",  // close-only, unchanged
		"E": "in\"middle",  // input "in"middle" — outer pair stripped, inner quote retained
	}
	for k, want := range cases {
		if got := m[k]; got != want {
			t.Errorf("m[%q] = %q, want %q", k, got, want)
		}
	}
}

// TestLoadCompilerConstants_LastWriterWins pins NAI-202-D-CONSTANT-LOOSE-PARSER:
// duplicate names within the same file resolve last-line-wins (no error,
// unlike pkg/pack.LoadConstants which errors).
func TestLoadCompilerConstants_LastWriterWins(t *testing.T) {
	dir := t.TempDir()
	writeConstantFile(t, dir, "a.constant", "K=a\nK=b\n")

	m, err := loadCompilerConstants(filepath.Join(dir, "scripts"))
	if err != nil {
		t.Fatalf("loadCompilerConstants: %v (NAI-202-D-CONSTANT-LOOSE-PARSER: dup must not error)", err)
	}
	if got, want := m["K"], "b"; got != want {
		t.Errorf("m[\"K\"] = %q, want %q (last-line-wins per loose parser)", got, want)
	}
}

// TestLoadCompilerConstants_SkipsComments pins TS Compiler.ts:155:
// lines beginning with '//' are skipped.
func TestLoadCompilerConstants_SkipsComments(t *testing.T) {
	dir := t.TempDir()
	writeConstantFile(t, dir, "a.constant", "// K=a\nK=b\n")

	m, err := loadCompilerConstants(filepath.Join(dir, "scripts"))
	if err != nil {
		t.Fatalf("loadCompilerConstants: %v", err)
	}
	if got, want := m["K"], "b"; got != want {
		t.Errorf("m[\"K\"] = %q, want %q (// line skipped)", got, want)
	}
	if len(m) != 1 {
		t.Errorf("len(m) = %d, want 1; map = %v", len(m), m)
	}
}

// TestLoadCompilerConstants_DiscardsPastSecondEquals pins TS-faithful
// destructure of unbounded split: parts[0]=name, parts[1]=value, parts[2:]
// dropped. K=v=extra → m["K"] = "v" (not "v=extra").
func TestLoadCompilerConstants_DiscardsPastSecondEquals(t *testing.T) {
	dir := t.TempDir()
	writeConstantFile(t, dir, "a.constant", "K=v=extra\n")

	m, err := loadCompilerConstants(filepath.Join(dir, "scripts"))
	if err != nil {
		t.Fatalf("loadCompilerConstants: %v", err)
	}
	if got, want := m["K"], "v"; got != want {
		t.Errorf("m[\"K\"] = %q, want %q (parts past second '=' discarded)", got, want)
	}
}

// TestLoadCompilerConstants_ErrorsOnMissingEquals pins TS-faithful
// behaviour: a line with no '=' triggers an undefined-index throw in TS.
// Goscape returns a wrapped error including the file path.
func TestLoadCompilerConstants_ErrorsOnMissingEquals(t *testing.T) {
	dir := t.TempDir()
	writeConstantFile(t, dir, "a.constant", "broken_line_no_equals\n")

	_, err := loadCompilerConstants(filepath.Join(dir, "scripts"))
	if err == nil {
		t.Fatal("expected error on line missing '=', got nil")
	}
	if !strings.Contains(err.Error(), "a.constant") {
		t.Errorf("error %q must mention the offending file path", err.Error())
	}
}

// TestLoadCompilerConstants_TrimsWhitespace pins TS Compiler.ts:161,166
// which call .trim() on both name and value.
func TestLoadCompilerConstants_TrimsWhitespace(t *testing.T) {
	dir := t.TempDir()
	writeConstantFile(t, dir, "a.constant", "  K  =  v  \n")

	m, err := loadCompilerConstants(filepath.Join(dir, "scripts"))
	if err != nil {
		t.Fatalf("loadCompilerConstants: %v", err)
	}
	if got, want := m["K"], "v"; got != want {
		t.Errorf("m[\"K\"] = %q, want %q (whitespace trimmed)", got, want)
	}
}

// TestLoadCompilerConstants_EmptyScriptsDir pins: missing scripts dir
// returns an empty map with nil error.
func TestLoadCompilerConstants_EmptyScriptsDir(t *testing.T) {
	dir := t.TempDir()
	// No scripts/ subdir created.

	m, err := loadCompilerConstants(filepath.Join(dir, "scripts"))
	if err != nil {
		t.Fatalf("loadCompilerConstants: %v (missing dir must not error)", err)
	}
	if len(m) != 0 {
		t.Errorf("len(m) = %d, want 0", len(m))
	}
}

// writeConstantFile writes content to <dir>/scripts/<rel>, creating the
// scripts subdir + any intermediate dirs of rel.
func writeConstantFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, "scripts", rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
