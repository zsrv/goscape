package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// writeBytes is a test helper: writes content to dir/relpath, creating
// parent directories as needed.
func writeBytes(t *testing.T, dir, relpath string, content []byte) {
	t.Helper()
	abs := filepath.Join(dir, relpath)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(abs), err)
	}
	if err := os.WriteFile(abs, content, 0o644); err != nil {
		t.Fatalf("write %s: %v", abs, err)
	}
}

func TestSnapshotOutDir_MissingDirReturnsEmpty(t *testing.T) {
	snap, err := snapshotOutDir(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("missing dir: got err %v, want nil", err)
	}
	if len(snap) != 0 {
		t.Errorf("missing dir: got %d entries, want 0", len(snap))
	}
}

func TestSnapshotOutDir_HashesRegularFilesAndNestedDirs(t *testing.T) {
	dir := t.TempDir()
	writeBytes(t, dir, "a.bin", []byte("hello"))
	writeBytes(t, dir, "nested/b.bin", []byte("world"))
	if err := os.MkdirAll(filepath.Join(dir, "empty-subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	snap, err := snapshotOutDir(dir)
	if err != nil {
		t.Fatalf("snapshotOutDir: %v", err)
	}
	if len(snap) != 2 {
		t.Errorf("got %d entries, want 2 (regular files only): %v", len(snap), snap)
	}
	// Relpaths are forward-slash, even on platforms with backslash separators.
	if _, ok := snap["a.bin"]; !ok {
		t.Errorf("missing relpath %q in snapshot %v", "a.bin", snap)
	}
	if _, ok := snap["nested/b.bin"]; !ok {
		t.Errorf("missing relpath %q in snapshot %v", "nested/b.bin", snap)
	}
	// Hashes are sha256 hex = 64 chars.
	for k, v := range snap {
		if len(v) != 64 {
			t.Errorf("snap[%s] hash len = %d, want 64", k, len(v))
		}
	}
}

func TestDeltaFiles_DetectsAddsAndModifications(t *testing.T) {
	prev := stageSnapshot{"unchanged.bin": "aa", "modified.bin": "bb"}
	next := stageSnapshot{"unchanged.bin": "aa", "modified.bin": "cc", "added.bin": "dd"}
	got := deltaFiles(prev, next)
	want := []string{"added.bin", "modified.bin"}
	slices.Sort(got) // result is sorted by contract; sort defensively
	if len(got) != len(want) {
		t.Fatalf("len got=%d want=%d (got=%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("delta[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestDeltaFiles_DeletionsIgnored(t *testing.T) {
	prev := stageSnapshot{"gone.bin": "aa"}
	next := stageSnapshot{}
	got := deltaFiles(prev, next)
	if len(got) != 0 {
		t.Errorf("deletion: got delta %v, want empty (we never diff deletions)", got)
	}
}

func TestDeltaFiles_SortedResult(t *testing.T) {
	prev := stageSnapshot{}
	next := stageSnapshot{"c": "1", "a": "1", "b": "1"}
	got := deltaFiles(prev, next)
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Errorf("delta not sorted: %v", got)
			break
		}
	}
}

func TestDiffOneFile_MatchReturnsNil(t *testing.T) {
	dir := t.TempDir()
	writeBytes(t, dir, "out/a.bin", []byte("hello"))
	writeBytes(t, dir, "ref/a.bin", []byte("hello"))
	d, err := diffOneFile(filepath.Join(dir, "out", "a.bin"), filepath.Join(dir, "ref", "a.bin"))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if d != nil {
		t.Errorf("identical files: got %+v, want nil", d)
	}
}

func TestDiffOneFile_ByteMismatchReportsOffsetAndBytes(t *testing.T) {
	dir := t.TempDir()
	writeBytes(t, dir, "out/a.bin", []byte{0x01, 0x02, 0x03, 0x04})
	writeBytes(t, dir, "ref/a.bin", []byte{0x01, 0x02, 0xff, 0x04})
	d, err := diffOneFile(filepath.Join(dir, "out", "a.bin"), filepath.Join(dir, "ref", "a.bin"))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if d == nil {
		t.Fatal("got nil, want DIFF")
	}
	if d.Kind != "DIFF" {
		t.Errorf("Kind=%q, want DIFF", d.Kind)
	}
	if d.Offset != 2 {
		t.Errorf("Offset=%d, want 2", d.Offset)
	}
	if d.Got != 0x03 {
		t.Errorf("Got=%#x, want 0x03", d.Got)
	}
	if d.Want != 0xff {
		t.Errorf("Want=%#x, want 0xff", d.Want)
	}
}

func TestDiffOneFile_SizeMismatch(t *testing.T) {
	dir := t.TempDir()
	writeBytes(t, dir, "out/a.bin", []byte{0x01, 0x02, 0x03})
	writeBytes(t, dir, "ref/a.bin", []byte{0x01, 0x02, 0x03, 0x04})
	d, err := diffOneFile(filepath.Join(dir, "out", "a.bin"), filepath.Join(dir, "ref", "a.bin"))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if d == nil || d.Kind != "SIZE" {
		t.Fatalf("got %+v, want Kind=SIZE", d)
	}
	if d.OutSize != 3 {
		t.Errorf("OutSize=%d, want 3", d.OutSize)
	}
	if d.RefSize != 4 {
		t.Errorf("RefSize=%d, want 4", d.RefSize)
	}
}

func TestDiffOneFile_MissingReference(t *testing.T) {
	dir := t.TempDir()
	writeBytes(t, dir, "out/a.bin", []byte{0x01})
	d, err := diffOneFile(filepath.Join(dir, "out", "a.bin"), filepath.Join(dir, "ref", "absent.bin"))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if d == nil || d.Kind != "MISS" {
		t.Fatalf("got %+v, want Kind=MISS", d)
	}
}
