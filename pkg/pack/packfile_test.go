package pack

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNewPackFileMissingNoError(t *testing.T) {
	dir := t.TempDir()
	ClearFsCache()
	pf, err := NewPackFile(dir, "ghost", nil)
	if err != nil {
		t.Fatal(err)
	}
	if pf.Size() != 0 {
		t.Fatalf("expected empty, size=%d", pf.Size())
	}
}

func TestPackFileLoadValid(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pack"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pack", "obj.pack"),
		[]byte("0=coins\n1=bronze_dagger\n2=oak_log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	pf, err := NewPackFile(dir, "obj", nil)
	if err != nil {
		t.Fatal(err)
	}
	if pf.Size() != 3 {
		t.Fatalf("size=%d, want 3", pf.Size())
	}
	if pf.GetByID(1) != "bronze_dagger" {
		t.Fatalf("GetByID(1)=%q", pf.GetByID(1))
	}
	if pf.GetByName("coins") != 0 {
		t.Fatalf("GetByName(coins)=%d", pf.GetByName("coins"))
	}
	if pf.Max != 3 {
		t.Fatalf("Max=%d, want 3", pf.Max)
	}
}

func TestPackFileLoadGapMax(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pack"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pack", "x.pack"),
		[]byte("0=a\n5=b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	pf, err := NewPackFile(dir, "x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if pf.Max != 6 {
		t.Fatalf("Max=%d, want 6 (max id + 1)", pf.Max)
	}
}

func TestPackFileLoadEmptyNameErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pack"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pack", "x.pack"),
		[]byte("0=ok\n1=\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	_, err := NewPackFile(dir, "x", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "empty name") {
		t.Fatalf("expected error to mention 'empty name', got: %v", err)
	}
	if !strings.Contains(msg, "x.pack:2") {
		t.Fatalf("expected error to name x.pack:2, got: %v", err)
	}
}

func TestPackFileRefreshNamesAsymmetry(t *testing.T) {
	// Pin TS PackFileBase.ts:refreshNames behavior (spec §3.7):
	// RefreshNames rebuilds Names + Max from Pack but does NOT rebuild
	// NameToID. NameToID is maintained incrementally by Register/Delete.
	pf := &PackFile{
		Pack:     map[int]string{},
		Names:    map[string]struct{}{},
		NameToID: map[string]int{},
	}
	pf.Register(0, "alpha")
	pf.Register(1, "beta")
	// Manually corrupt NameToID to verify RefreshNames does NOT fix it.
	pf.NameToID["stale"] = 99
	pf.RefreshNames()
	if _, ok := pf.NameToID["stale"]; !ok {
		t.Fatal("RefreshNames must NOT touch NameToID (TS parity)")
	}
	if _, ok := pf.Names["alpha"]; !ok {
		t.Fatal("Names should contain alpha after RefreshNames")
	}
	if pf.Max != 2 {
		t.Fatalf("Max=%d, want 2", pf.Max)
	}
}

func TestPackFileClearEmpties(t *testing.T) {
	pf := &PackFile{
		Pack:     map[int]string{},
		Names:    map[string]struct{}{},
		NameToID: map[string]int{},
	}
	pf.Register(0, "alpha")
	pf.RefreshNames()
	pf.Clear()
	if len(pf.Pack) != 0 || len(pf.Names) != 0 || len(pf.NameToID) != 0 || pf.Max != 0 {
		t.Fatalf("Clear left state: pack=%v names=%v nameToId=%v max=%d",
			pf.Pack, pf.Names, pf.NameToID, pf.Max)
	}
}

func TestPackFileSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pack"), 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte("0=alpha\n2=gamma\n5=epsilon\n")
	if err := os.WriteFile(filepath.Join(dir, "pack", "y.pack"), original, 0o644); err != nil {
		t.Fatal(err)
	}
	ClearFsCache()
	pf, err := NewPackFile(dir, "y", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := pf.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "pack", "y.pack"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, original) {
		t.Fatalf("round-trip mismatch:\nwant %q\ngot  %q", original, got)
	}
}

func TestPackFileSaveCreatesPackDir(t *testing.T) {
	dir := t.TempDir()
	ClearFsCache()
	pf, err := NewPackFile(dir, "z", nil)
	if err != nil {
		t.Fatal(err)
	}
	pf.Register(0, "hello")
	pf.RefreshNames()
	if err := pf.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "pack", "z.pack")); err != nil {
		t.Fatalf("expected pack dir created: %v", err)
	}
}

func TestPackFileValidatorRunsOnReload(t *testing.T) {
	called := 0
	validator := func(pf *PackFile) error {
		called++
		pf.Register(7, "from_validator")
		pf.RefreshNames()
		return nil
	}
	pf, err := NewPackFile(t.TempDir(), "v", validator)
	if err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Fatalf("validator called %d times, want 1", called)
	}
	if pf.GetByID(7) != "from_validator" {
		t.Fatalf("validator did not populate: %v", pf.Pack)
	}
}

func TestPackFileValidatorErrorSurfaces(t *testing.T) {
	wantErr := errors.New("boom")
	_, err := NewPackFile(t.TempDir(), "v", func(*PackFile) error { return wantErr })
	if !errors.Is(err, wantErr) {
		t.Fatalf("got %v, want %v", err, wantErr)
	}
}

func TestPackFileDeleteRefreshesNames(t *testing.T) {
	pf := &PackFile{
		Pack:     map[int]string{},
		Names:    map[string]struct{}{},
		NameToID: map[string]int{},
	}
	pf.Register(0, "alpha")
	pf.Register(1, "beta")
	pf.RefreshNames()
	pf.Delete(0)
	if pf.GetByID(0) != "" {
		t.Fatal("Delete(0) did not remove from Pack")
	}
	if _, ok := pf.NameToID["alpha"]; ok {
		t.Fatal("Delete(0) did not remove alpha from NameToID")
	}
	if _, ok := pf.Names["alpha"]; ok {
		t.Fatal("Delete(0) did not run RefreshNames (alpha still in Names)")
	}
}

func TestPackFileDeleteByName(t *testing.T) {
	pf := &PackFile{
		Pack:     map[int]string{},
		Names:    map[string]struct{}{},
		NameToID: map[string]int{},
	}
	pf.Register(0, "alpha")
	pf.RefreshNames()
	pf.DeleteByName("alpha")
	if pf.Size() != 0 {
		t.Fatalf("expected empty after DeleteByName, size=%d", pf.Size())
	}
}

func TestPackFileGetByNameMissingReturnsMinusOne(t *testing.T) {
	pf := &PackFile{
		Pack:     map[int]string{},
		Names:    map[string]struct{}{},
		NameToID: map[string]int{},
	}
	if id := pf.GetByName("nope"); id != -1 {
		t.Fatalf("got %d, want -1", id)
	}
}

func TestPackFileGetByIDMissingReturnsEmpty(t *testing.T) {
	pf := &PackFile{
		Pack:     map[int]string{},
		Names:    map[string]struct{}{},
		NameToID: map[string]int{},
	}
	if s := pf.GetByID(42); s != "" {
		t.Fatalf("got %q, want empty", s)
	}
}

// TestValidateConfigPackNames_UniversalGate pins the rev-244 B6 change:
// validateConfigPack no longer gates the "name not found in any <ext> files"
// BUILD_VERIFY check behind `if (transmitted)`. The check now runs for ALL
// config packs.
//
// TS diff e1dea19f..9aadcec4 -- tools/pack/PackFile.ts:
//
//	-    if (transmitted) {
//	-        for (const name of pack.names) {
//	-            if (Environment.BUILD_VERIFY && !configNames.has(name) ...
//	-    }
//	+    for (const name of pack.names) {
//	+        if (Environment.BUILD_VERIFY && !configNames.has(name) ...
//
// In Go, BUILD_VERIFY is not an env-var gate — the check is always-on
// (consistent with Go dropping env-var gates in favour of structural enforcement).
func TestValidateConfigPackNames_UniversalGate(t *testing.T) {
	// Pack contains "ghost_name" which is NOT in any source config file.
	pf := &PackFile{
		Type:     "obj",
		Pack:     map[int]string{0: "ghost_name"},
		Names:    map[string]struct{}{"ghost_name": {}},
		NameToID: map[string]int{"ghost_name": 0},
	}
	configNames := map[string]struct{}{"real_obj": {}}

	err := ValidateConfigPackNames(pf, configNames, ".obj")
	if err == nil {
		t.Fatal("want error for name absent from source configs, got nil")
	}
	// Error message must mirror TS shape: "<type>: <name> was not found in any <ext> files..."
	// TS PackFile.ts:119 @ 9aadcec4: `${pack.type}: ${name} was not found in any ${ext} files`
	if !strings.Contains(err.Error(), "obj") {
		t.Errorf("error should name the pack type; got: %v", err)
	}
	if !strings.Contains(err.Error(), "ghost_name") {
		t.Errorf("error should name the offending entry; got: %v", err)
	}
	if !strings.Contains(err.Error(), "was not found in any") {
		t.Errorf("error should use TS phrase 'was not found in any'; got: %v", err)
	}
	if !strings.Contains(err.Error(), ".obj") {
		t.Errorf("error should name the extension; got: %v", err)
	}
}

// TestValidateConfigPackNames_CertPrefix pins that cert_ names are
// exempted from the check — TS PackFile.ts:118 @ 9aadcec4:
// `!name.startsWith('cert_')`.
func TestValidateConfigPackNames_CertPrefix(t *testing.T) {
	pf := &PackFile{
		Type:     "obj",
		Pack:     map[int]string{0: "cert_bronze_dagger"},
		Names:    map[string]struct{}{"cert_bronze_dagger": {}},
		NameToID: map[string]int{"cert_bronze_dagger": 0},
	}
	configNames := map[string]struct{}{} // cert_ name absent from configs — should NOT error

	if err := ValidateConfigPackNames(pf, configNames, ".obj"); err != nil {
		t.Fatalf("cert_ names must be exempt from the check, got: %v", err)
	}
}

// TestValidateConfigPackNames_KnownName pins the happy path: a pack name
// that IS in the config names set must not produce an error.
func TestValidateConfigPackNames_KnownName(t *testing.T) {
	pf := &PackFile{
		Type:     "npc",
		Pack:     map[int]string{0: "rat"},
		Names:    map[string]struct{}{"rat": {}},
		NameToID: map[string]int{"rat": 0},
	}
	configNames := map[string]struct{}{"rat": {}}

	if err := ValidateConfigPackNames(pf, configNames, ".npc"); err != nil {
		t.Fatalf("known name must pass; got: %v", err)
	}
}

// TestValidateConfigPackNames_MultiOrphanDeterministic pins that when
// MULTIPLE pack names are orphaned, the error deterministically reports the
// one with the LOWEST id. TS iterates pack.names — a JS Set in insertion
// order, i.e. .pack file line order, which the machine-written pack files
// keep id-ascending (PackFile.ts:117-119 @9aadcec4) — so the first throw is
// the lowest-id orphan. The pre-fix Go ranged the Names map, so which
// orphan got reported was map-iteration random (the rev-244-era
// "ValidateConfigPackNames multi-orphan map-order" residual).
//
// The orphan at the LOWER id ("zz_orphan") sorts lexicographically AFTER
// the higher-id one ("aa_orphan") so this test distinguishes id-order from
// accidental name-sorting. 50 iterations defeat Go's per-range map seed.
func TestValidateConfigPackNames_MultiOrphanDeterministic(t *testing.T) {
	pf := &PackFile{
		Type: "npc",
		Pack: map[int]string{0: "rat", 1: "zz_orphan", 2: "aa_orphan"},
		Names: map[string]struct{}{
			"rat": {}, "zz_orphan": {}, "aa_orphan": {},
		},
		NameToID: map[string]int{"rat": 0, "zz_orphan": 1, "aa_orphan": 2},
	}
	configNames := map[string]struct{}{"rat": {}}

	for i := range 50 {
		err := ValidateConfigPackNames(pf, configNames, ".npc")
		if err == nil {
			t.Fatal("want error for orphaned names, got nil")
		}
		if !strings.Contains(err.Error(), "zz_orphan") {
			t.Fatalf("iteration %d: error must report the lowest-id orphan (id 1, zz_orphan); got: %v", i, err)
		}
	}
}
