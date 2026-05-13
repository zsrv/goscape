package pack

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetModifiedMissingReturnsZero(t *testing.T) {
	ClearFsCache()
	if got := GetModified("/does/not/exist"); got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
}

func TestGetModifiedReturnsMs(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f")
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	got := GetModified(p)
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	want := info.ModTime().UnixMilli()
	if got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
}

func TestGetLatestModifiedMaxAcrossExtension(t *testing.T) {
	dir := t.TempDir()
	older := filepath.Join(dir, "a.obj")
	newer := filepath.Join(dir, "b.obj")
	if err := os.WriteFile(older, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(older, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newer, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	// Sibling file with a different extension that's newer than both;
	// must be excluded by the extension filter.
	other := filepath.Join(dir, "ignored.txt")
	if err := os.WriteFile(other, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(1 * time.Hour)
	if err := os.Chtimes(other, future, future); err != nil {
		t.Fatal(err)
	}

	ClearFsCache()
	got := GetLatestModified(dir, ".obj")
	newerInfo, _ := os.Stat(newer)
	want := newerInfo.ModTime().UnixMilli()
	if got != want {
		t.Fatalf("got %d, want %d (ignored.txt should be excluded)", got, want)
	}
}

func TestShouldBuildMissingOut(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "in.obj"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	if !ShouldBuild(dir, ".obj", "/nope") {
		t.Fatal("expected true when out missing")
	}
}

func TestShouldBuildOutNewer(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "x.obj")
	if err := os.WriteFile(in, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(in, old, old); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "o")
	if err := os.WriteFile(out, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	if ShouldBuild(dir, ".obj", out) {
		t.Fatal("expected false when out newer than all inputs")
	}
}

func TestShouldBuildFileAnyRecurses(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(dir, "a", "b", "deep.txt")
	if err := os.WriteFile(deep, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out")
	if err := os.WriteFile(out, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(out, old, old); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	if !ShouldBuildFileAny(dir, out) {
		t.Fatal("expected true: deep file newer than out")
	}
}
