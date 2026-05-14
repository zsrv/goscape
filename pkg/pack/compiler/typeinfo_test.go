package compiler

import (
	"os"
	"path/filepath"
	"testing"
)

// TestNewTypeInfo_ZeroValues pins spec §5.8: constructors return *TypeInfo
// with Max=-1 and all maps non-nil so callers can write immediately.
func TestNewTypeInfo_ZeroValues(t *testing.T) {
	p := newTypeInfo()
	if p.Max != -1 {
		t.Fatalf("Max: got %d, want -1", p.Max)
	}
	if p.Map == nil {
		t.Fatal("Map: got nil, want non-nil empty map")
	}
	if len(p.Map) != 0 {
		t.Fatalf("Map: got len %d, want 0", len(p.Map))
	}
	if p.NameMap == nil {
		t.Fatal("NameMap: got nil, want non-nil empty map")
	}
	if p.VarType == nil || p.Protect == nil || p.Require == nil || p.Require2 == nil ||
		p.Conditional == nil || p.Set == nil || p.Set2 == nil ||
		p.Corrupt == nil || p.Corrupt2 == nil {
		t.Fatal("auxiliary maps must be non-nil so NAI-201 populator can write without re-init")
	}
}

// TestAdd_UpdateMaxFalse pins spec §7.11: updateMax=false skips Max bump
// even when id > Max.
func TestAdd_UpdateMaxFalse(t *testing.T) {
	p := newTypeInfo()
	p.Add(0, "a", true)
	p.Add(5, "b", false)
	p.Add(2, "c", true)

	if p.Max != 3 {
		t.Fatalf("Max: got %d, want 3 (-1→1 via id=0; skip via updateMax=false; 1→3 via id=2)", p.Max)
	}
	if p.Map[0] != "a" || p.Map[5] != "b" || p.Map[2] != "c" {
		t.Fatalf("Map: got %v, want {0:a, 5:b, 2:c}", p.Map)
	}
}

// TestAdd_MaxMonotonic pins spec §7.12: Max is monotonic non-decreasing —
// a smaller id following a larger id does NOT shrink Max.
func TestAdd_MaxMonotonic(t *testing.T) {
	p := newTypeInfo()
	p.Add(0, "a", true)
	p.Add(5, "b", true)
	p.Add(2, "c", true)

	if p.Max != 6 {
		t.Fatalf("Max: got %d, want 6 (id=5 bumps to 6; id=2 does NOT re-bump since 6<2 is false)", p.Max)
	}
}

// TestLoad_HappyPath pins spec §7.1: three dense entries land as
// Map[i]=name, Max=3.
func TestLoad_HappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "category.pack")
	if err := os.WriteFile(path, []byte("0=alpha\n1=bravo\n2=charlie\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if p.Max != 3 {
		t.Fatalf("Max: got %d, want 3", p.Max)
	}
	if p.Map[0] != "alpha" || p.Map[1] != "bravo" || p.Map[2] != "charlie" {
		t.Fatalf("Map: got %v, want {0:alpha,1:bravo,2:charlie}", p.Map)
	}
	if len(p.NameMap) != 0 {
		t.Fatalf("NameMap: got %v, want empty (Load writes only Map)", p.NameMap)
	}
}

// TestLoad_MissingFile pins spec §7.2: nonexistent path returns
// empty *TypeInfo, nil error (TS !fs.existsSync early-return).
func TestLoad_MissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does_not_exist.pack")

	p, err := Load(path)
	if err != nil {
		t.Fatalf("err: got %v, want nil", err)
	}
	if p == nil {
		t.Fatal("p: got nil, want empty *TypeInfo")
	}
	if p.Max != -1 {
		t.Fatalf("Max: got %d, want -1", p.Max)
	}
	if len(p.Map) != 0 {
		t.Fatalf("Map: got %v, want empty", p.Map)
	}
}

// TestLoad_FilterCases pins spec §7.3: blank lines, no-=, name=="null",
// name=="null:null" are skipped; name=="" (line "1=") is RETAINED.
func TestLoad_FilterCases(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mixed.pack")
	content := "0=valid_alpha\n\n1=\n2=null\n3=null:null\nnot_an_equals_line\n4=valid_beta\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	// Expected: id 0 → "valid_alpha", id 1 → "" (empty name retained per TS),
	// ids 2 and 3 skipped (null/null:null sentinels), id 4 → "valid_beta".
	// "not_an_equals_line" skipped (no '=').
	wantMap := map[int]string{0: "valid_alpha", 1: "", 4: "valid_beta"}
	if len(p.Map) != len(wantMap) {
		t.Fatalf("Map len: got %d (%v), want %d (%v)", len(p.Map), p.Map, len(wantMap), wantMap)
	}
	for k, v := range wantMap {
		if p.Map[k] != v {
			t.Fatalf("Map[%d]: got %q, want %q", k, p.Map[k], v)
		}
	}
	if p.Max != 5 {
		t.Fatalf("Max: got %d, want 5 (last Add(4, _, true) bumps 1→5)", p.Max)
	}
}

// TestLoad_IOError pins spec §7.4: passing a directory path triggers a
// genuine IO error (not IsNotExist) and returns (nil, err).
func TestLoad_IOError(t *testing.T) {
	dir := t.TempDir() // dir IS a directory; os.ReadFile on it returns a non-nil error

	p, err := Load(dir)
	if err == nil {
		t.Fatal("err: got nil, want non-nil (read of directory)")
	}
	if p != nil {
		t.Fatalf("p: got %+v, want nil on IO error", p)
	}
}

// TestLoad_DuplicateID pins spec §7.5: TS-faithful no-validation —
// later `0=second` silently overwrites earlier `0=first`. Max bumps
// only once (on the first Add(0,_,true): -1→1) because the second
// Add(0,_,true) sees Max=1 and 1<0 is false.
func TestLoad_DuplicateID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dup.pack")
	if err := os.WriteFile(path, []byte("0=first\n0=second\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if p.Map[0] != "second" {
		t.Fatalf("Map[0]: got %q, want %q (second write overwrites silently)", p.Map[0], "second")
	}
	if p.Max != 1 {
		t.Fatalf("Max: got %d, want 1", p.Max)
	}
}

// TestLoadArray_HappyPath pins spec §7.6: each index → Add(i,
// lowercase(s), true); Max = len(input).
func TestLoadArray_HappyPath(t *testing.T) {
	p := LoadArray([]string{"Alpha", "BRAVO", "Charlie"})

	if p.Map[0] != "alpha" || p.Map[1] != "bravo" || p.Map[2] != "charlie" {
		t.Fatalf("Map: got %v, want {0:alpha,1:bravo,2:charlie}", p.Map)
	}
	if p.Max != 3 {
		t.Fatalf("Max: got %d, want 3 (len-1 + 1)", p.Max)
	}
}

// TestLoadArray_Empty pins spec §7.7: empty input → no Add calls →
// Max remains -1.
func TestLoadArray_Empty(t *testing.T) {
	p := LoadArray([]string{})

	if p.Max != -1 {
		t.Fatalf("Max: got %d, want -1", p.Max)
	}
	if len(p.Map) != 0 {
		t.Fatalf("Map: got %v, want empty", p.Map)
	}
}
