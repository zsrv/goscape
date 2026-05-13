package pack

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestLoadOrderNumericLines(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.order")
	if err := os.WriteFile(p, []byte("0\n5\n\n12\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := LoadOrder(p)
	want := []int{0, 5, 12}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestLoadOrderMissing(t *testing.T) {
	if got := LoadOrder("/nonexistent.order"); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestLoadPackSparseArray(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.pack")
	if err := os.WriteFile(p, []byte("0=alpha\n3=delta\n5=epsilon"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := LoadPack(p)
	if len(got) < 6 {
		t.Fatalf("len=%d, want ≥6 (sparse)", len(got))
	}
	if got[0] != "alpha" || got[3] != "delta" || got[5] != "epsilon" {
		t.Fatalf("indexed values wrong: %v", got)
	}
	if got[1] != "" || got[2] != "" || got[4] != "" {
		t.Fatalf("gaps not preserved as empty: %v", got)
	}
}

func TestLoadDirExactDoesNotFilterEmpties(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.txt"), []byte("a\n\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	var got [][]string
	LoadDirExact(dir, ".txt", func(src []string, _, _ string) {
		got = append(got, src)
	})
	if len(got) != 1 {
		t.Fatalf("expected 1 file, got %d", len(got))
	}
	// TS LoadDirExact does NOT filter empties; should see ["a","","b",""] (trailing newline → empty).
	if len(got[0]) != 4 {
		t.Fatalf("expected 4 lines (incl empties), got %d: %v", len(got[0]), got[0])
	}
}

func TestNameMapLoadDirFiltersEmpties(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.txt"), []byte("a\n\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	var got [][]string
	NameMapLoadDir(dir, ".txt", func(src []string, _, _ string) {
		got = append(got, src)
	})
	if len(got) != 1 || len(got[0]) != 2 {
		t.Fatalf("expected 1 file with 2 non-empty lines, got %v", got)
	}
}
