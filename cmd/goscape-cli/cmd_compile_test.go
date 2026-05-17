package main

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/packall"
)

// TestRunCompile_HappyPath_Check seeds the same minimal fixture pack
// uses (via the seedMinimalPackFixture helper in cmd_pack_test.go),
// then compiles helper.rs2 in --check mode. Expects exit 0 and
// "compile succeeded" in the captured stderr (logger output).
func TestRunCompile_HappyPath_Check(t *testing.T) {
	dir := t.TempDir()
	seedMinimalPackFixture(t, dir)
	// Deviation: LoadCompilerSymbols reads .dat files under
	// <datapack-dir>/server/, which the source-only fixture does not
	// create. Pack first so the cache exists; --datapack-dir then
	// points to a real packed dir.
	if err := packall.PackAll(dir, dir, dir); err != nil {
		t.Fatalf("PackAll seed: %v", err)
	}

	var stderr bytes.Buffer
	code := runCompile([]string{
		"--src-dir", dir,
		"--datapack-dir", dir,
		"--check",
		filepath.Join(dir, "scripts", "helper.rs2"),
	}, io.Discard, &stderr)

	if code != 0 {
		t.Fatalf("runCompile returned %d, want 0; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "compile succeeded") {
		t.Errorf("stderr %q missing success log", stderr.String())
	}
}

// TestRunCompile_SourceError seeds the minimal fixture but replaces
// helper.rs2 with an invalid source. Expects exit 1.
func TestRunCompile_SourceError(t *testing.T) {
	dir := t.TempDir()
	seedMinimalPackFixture(t, dir)
	if err := packall.PackAll(dir, dir, dir); err != nil {
		t.Fatalf("PackAll seed: %v", err)
	}
	// Overwrite helper.rs2 with a clearly-broken source (unknown
	// command "not_a_command").
	writeFile(t, filepath.Join(dir, "scripts", "helper.rs2"),
		"[proc,helper]\nnot_a_command;\n")

	var stderr bytes.Buffer
	code := runCompile([]string{
		"--src-dir", dir,
		"--datapack-dir", dir,
		"--check",
		filepath.Join(dir, "scripts", "helper.rs2"),
	}, io.Discard, &stderr)

	if code != 1 {
		t.Fatalf("runCompile returned %d, want 1; stderr=%q", code, stderr.String())
	}
}

// TestRunCompile_MissingPath expects exit 2 when no positional arg
// is provided.
func TestRunCompile_MissingPath(t *testing.T) {
	var stderr bytes.Buffer
	code := runCompile([]string{
		"--src-dir", "irrelevant",
	}, io.Discard, &stderr)
	if code != 2 {
		t.Fatalf("runCompile returned %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "exactly one source path") {
		t.Errorf("stderr %q missing missing-path diagnostic", stderr.String())
	}
}
