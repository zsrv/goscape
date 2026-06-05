package pack

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLoadFileMissing(t *testing.T) {
	got := LoadFile("/nonexistent/file.txt")
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestLoadFileFullStripsSingleLineComment(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.rs2")
	if err := os.WriteFile(p, []byte("foo  // trailing\nbar"), 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	lines, err := LoadFileFull(p)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"foo", "bar"}
	if !slices.Equal(lines, want) {
		t.Fatalf("got %q, want %q", lines, want)
	}
}

func TestLoadFileFullStripsSameLineMultiComment(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.rs2")
	if err := os.WriteFile(p, []byte("a/* in */b"), 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	lines, err := LoadFileFull(p)
	if err != nil {
		t.Fatal(err)
	}
	// TS substring(0,1)+substring(idx+2) = "a" + "b" = "ab"
	want := []string{"ab"}
	if !slices.Equal(lines, want) {
		t.Fatalf("got %q, want %q", lines, want)
	}
}

func TestLoadFileFullStripsMultiLineComment(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.rs2")
	if err := os.WriteFile(p, []byte("first\n/* multi\nline */\nlast"), 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	lines, err := LoadFileFull(p)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"first", "last"}
	if !slices.Equal(lines, want) {
		t.Fatalf("got %q, want %q", lines, want)
	}
}

func TestLoadFileFullTSQuirkDoubleStarOnOneLine(t *testing.T) {
	// TS-parity pin per spec §3.6: the outer block strips ONLY the
	// first /* */ pair; a trailing */ literal survives in the output
	// WITH its leading space (TS does no post-substring trim).
	//
	// Input:  "/* /* */ */"  (11 chars; first "*/" at idx 6)
	// TS:     line.substring(0,0) + line.substring(8) = "" + " */"
	// Result pushed verbatim: " */"
	dir := t.TempDir()
	p := filepath.Join(dir, "f.rs2")
	if err := os.WriteFile(p, []byte("/* /* */ */"), 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	lines, err := LoadFileFull(p)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{" */"}
	if !slices.Equal(lines, want) {
		t.Fatalf("got %q, want %q", lines, want)
	}
}

func TestLoadFileFullUnclosedCommentErrors(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.rs2")
	if err := os.WriteFile(p, []byte("ok\n/* never closes\nstill open"), 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	_, err := LoadFileFull(p)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unclosed multi-line comment") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("expected error to name line 2, got: %v", err)
	}
}

func TestReadConfigsAggregatesAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	scripts := filepath.Join(dir, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scripts, "a.obj"),
		[]byte("[coins]\nmodel=coins_obj\n[bronze_dagger]\nmodel=bd"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scripts, "b.obj"),
		[]byte("[oak_log]\nmodel=ol"), 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	cfg, err := ReadConfigs(dir, ".obj")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg) != 3 {
		t.Fatalf("got %d configs, want 3: %v", len(cfg), cfg)
	}
	if !slices.Equal(cfg["coins"], []string{"model=coins_obj"}) {
		t.Fatalf("coins=%v", cfg["coins"])
	}
}

func TestReadConfigsDuplicateErrors(t *testing.T) {
	dir := t.TempDir()
	scripts := filepath.Join(dir, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scripts, "dup.obj"),
		[]byte("[coins]\nmodel=a\n[coins]\nmodel=b"), 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	_, err := ReadConfigs(dir, ".obj")
	if err == nil {
		t.Fatal("expected duplicate error")
	}
	if !strings.Contains(err.Error(), "duplicate config") || !strings.Contains(err.Error(), "coins") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadDirExtFullPropagatesUnclosedCommentError(t *testing.T) {
	dir := t.TempDir()
	scripts := filepath.Join(dir, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scripts, "bad.obj"),
		[]byte("[ok]\n/* never closes\nstill open"), 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	err := LoadDirExtFull(scripts, ".obj", func(_ []string, _ string) {
		t.Fatal("callback should not run after error")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unclosed multi-line comment") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListFilesExtFiltersByExtension(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.obj", "b.npc", "c.obj", "d.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ClearFsCache()
	got := ListFilesExt(dir, ".obj")
	want := []string{dir + "/a.obj", dir + "/c.obj"}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// TestLoadFile_CRNormalization pins the TS 244 behaviour of
// readTextNormalize (Parse.ts:6-12): ALL \r in the file content are
// stripped before splitting on \n, so a mid-line \r (not part of a
// \r\n pair) is removed from the line content rather than preserved.
//
// Fixture: "x\ry\nz"
//
//	Before fix (split /\r?\n/ / TrimSuffix \r): lines = ["x\ry", "z"]
//	After  fix (.replace(/\r/g,'').split('\n')):  lines = ["xy", "z"]
func TestLoadFile_CRNormalization(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cr.txt")
	// Write literal bytes: 'x', CR(0x0D), 'y', LF(0x0A), 'z'
	if err := os.WriteFile(p, []byte("x\ry\nz"), 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	lines := LoadFile(p)
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %q", len(lines), lines)
	}
	if lines[0] != "xy" {
		t.Errorf("lines[0] = %q, want %q (mid-line \\r must be stripped)", lines[0], "xy")
	}
	if lines[1] != "z" {
		t.Errorf("lines[1] = %q, want %q", lines[1], "z")
	}
}

// TestReadTextNormalize_CRNormalization pins the readTextNormalize helper
// (Parse.ts:6-12): missing file returns "", present file has all \r stripped.
func TestReadTextNormalize_CRNormalization(t *testing.T) {
	// Missing file → empty string.
	got := readTextNormalize("/nonexistent/__cr_norm_test__.txt")
	if got != "" {
		t.Fatalf("missing file: want %q, got %q", "", got)
	}

	// Present file: "a\rb\r\nc" → all \r stripped → "ab\nc"
	dir := t.TempDir()
	p := filepath.Join(dir, "cr2.txt")
	if err := os.WriteFile(p, []byte("a\rb\r\nc"), 0o644); err != nil {
		t.Fatal(err)
	}
	got = readTextNormalize(p)
	const want = "ab\nc"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
