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

// TestLoadPack_CRNormalization pins the TS 244 behaviour of loadPack
// (NameMap.ts:19-34): ALL \r are stripped before splitting, so a
// mid-line \r is removed from the line content rather than preserved.
//
// Fixture: "0=alpha\r\n1\r2=beta"
//   - \r\n pair → normal line ending → "0=alpha", "12=beta" (mid-line \r stripped)
//   - id 0 → "alpha", id 12 → "beta"
//
// Before fix (TrimSuffix \r only): line "1\r2=beta" keeps \r →
// strconv.Atoi("1\r2") fails → "beta" never stored.
// After  fix (.replace(/\r/g,'')): "12=beta" → id=12, name="beta".
func TestLoadPack_CRNormalization(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cr.pack")
	// Bytes: "0=alpha\r\n1\r2=beta"
	if err := os.WriteFile(p, []byte("0=alpha\r\n1\r2=beta"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := LoadPack(p)
	if len(got) < 13 {
		t.Fatalf("want len >= 13 (id 12 present), got len=%d: %v", len(got), got)
	}
	if got[0] != "alpha" {
		t.Errorf("got[0] = %q, want %q", got[0], "alpha")
	}
	if got[12] != "beta" {
		t.Errorf("got[12] = %q, want %q (mid-line \\r must be stripped)", got[12], "beta")
	}
}

// TestLoadOrder_CRNormalization pins NameMap.ts:4-15 at 244: ALL \r
// are stripped. A mid-line \r in a numeric line makes the parse fail
// under the old split(/\r?\n/) contract but succeeds after the fix.
//
// Fixture: "1\r2\n3"  →  after strip: "12\n3"  →  [12, 3]
// Before fix: "1\r2" is not a valid int → only [3].
func TestLoadOrder_CRNormalization(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cr.order")
	if err := os.WriteFile(p, []byte("1\r2\n3"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := LoadOrder(p)
	want := []int{12, 3}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v (mid-line \\r must be stripped)", got, want)
	}
}
