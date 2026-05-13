package pack

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestFileExistsMissing(t *testing.T) {
	ClearFsCache()
	if FileExists("/nonexistent/path/should/not/be/here") {
		t.Fatal("expected false for missing path")
	}
}

func TestFileExistsCachesResult(t *testing.T) {
	ClearFsCache()
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(p, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !FileExists(p) {
		t.Fatal("expected true")
	}
	// Remove file; cached "true" should persist until ClearFsCache.
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if !FileExists(p) {
		t.Fatal("expected cached true after removal")
	}
	ClearFsCache()
	if FileExists(p) {
		t.Fatal("expected false after ClearFsCache")
	}
}

func TestListDirMissingReturnsNil(t *testing.T) {
	ClearFsCache()
	got := ListDir("/nonexistent/somewhere")
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestListDirRecursesWithSubdirSuffix(t *testing.T) {
	ClearFsCache()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got := ListDir(root)
	want := []string{
		root + "/a.txt",
		root + "/sub/",
		root + "/sub/b.txt",
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestListDirTrailingSlashNormalized(t *testing.T) {
	ClearFsCache()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "x"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got := ListDir(root + "/")
	want := []string{root + "/x"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFileStatCaches(t *testing.T) {
	ClearFsCache()
	root := t.TempDir()
	p := filepath.Join(root, "f")
	if err := os.WriteFile(p, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := FileStat(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 1 {
		t.Fatalf("size=%d", info.Size())
	}
	// Modify file; cached size should persist.
	if err := os.WriteFile(p, []byte("longer"), 0o644); err != nil {
		t.Fatal(err)
	}
	info2, err := FileStat(p)
	if err != nil {
		t.Fatal(err)
	}
	if info2.Size() != 1 {
		t.Fatalf("expected cached size=1, got %d", info2.Size())
	}
}
