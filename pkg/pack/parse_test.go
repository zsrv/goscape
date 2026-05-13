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
