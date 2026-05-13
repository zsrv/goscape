package pack

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCheckVarNameUniqueness_DistinctNamesAcrossPacks verifies that nil
// is returned when all names across multiple PackFiles are distinct.
func TestCheckVarNameUniqueness_DistinctNamesAcrossPacks(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	pfA, err := newPackFileWithEntries(dir, "varp", map[int]string{0: "alpha", 1: "beta"})
	if err != nil {
		t.Fatalf("setup varp: %v", err)
	}
	pfB, err := newPackFileWithEntries(dir, "varn", map[int]string{0: "gamma", 1: "delta"})
	if err != nil {
		t.Fatalf("setup varn: %v", err)
	}
	pfC, err := newPackFileWithEntries(dir, "vars", map[int]string{0: "epsilon"})
	if err != nil {
		t.Fatalf("setup vars: %v", err)
	}

	if err := checkVarNameUniqueness(pfA, pfB, pfC); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

// TestCheckVarNameUniqueness_DuplicateAcrossPacks verifies that an error
// is returned when the same name appears in two different PackFiles, and
// that the error message mentions the duplicated name.
func TestCheckVarNameUniqueness_DuplicateAcrossPacks(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	pfA, err := newPackFileWithEntries(dir, "varp", map[int]string{0: "alpha", 1: "dup_name"})
	if err != nil {
		t.Fatalf("setup varp: %v", err)
	}
	pfB, err := newPackFileWithEntries(dir, "varn", map[int]string{0: "gamma", 1: "dup_name"})
	if err != nil {
		t.Fatalf("setup varn: %v", err)
	}

	err = checkVarNameUniqueness(pfA, pfB)
	if err == nil {
		t.Fatal("expected error for duplicate name, got nil")
	}
	if !strings.Contains(err.Error(), "dup_name") {
		t.Errorf("error %q does not mention the duplicate name %q", err.Error(), "dup_name")
	}
}

// TestCheckVarNameUniqueness_EmptySlotsIgnored verifies that sparse packs
// with empty slots (id present but name is "") do not cause false-positive
// duplicate errors.
func TestCheckVarNameUniqueness_EmptySlotsIgnored(t *testing.T) {
	t.Parallel()

	// Manually build sparse PackFiles: ids 0 and 2 are populated,
	// id 1 is absent (sparse gap produces "" from GetByID).
	pfA := &PackFile{
		Type:     "varp",
		Pack:     map[int]string{0: "alpha", 2: "beta"},
		Names:    map[string]struct{}{"alpha": {}, "beta": {}},
		NameToID: map[string]int{"alpha": 0, "beta": 2},
		Max:      3,
	}
	pfB := &PackFile{
		Type:     "varn",
		Pack:     map[int]string{0: "gamma", 2: "delta"},
		Names:    map[string]struct{}{"gamma": {}, "delta": {}},
		NameToID: map[string]int{"gamma": 0, "delta": 2},
		Max:      3,
	}

	if err := checkVarNameUniqueness(pfA, pfB); err != nil {
		t.Errorf("expected nil for sparse packs with distinct names, got %v", err)
	}
}

// newPackFileWithEntries is a test helper that constructs a PackFile from
// an in-memory map of id→name by writing a temporary .pack file and
// loading it via PackFile.Load.
func newPackFileWithEntries(tmpDir, packType string, entries map[int]string) (*PackFile, error) {
	packDir := filepath.Join(tmpDir, "pack")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		return nil, err
	}
	packPath := filepath.Join(packDir, packType+".pack")
	var content strings.Builder
	for id, name := range entries {
		content.WriteString(fmt.Sprintf("%d=%s\n", id, name))
	}
	if err := os.WriteFile(packPath, []byte(content.String()), 0o644); err != nil {
		return nil, err
	}
	pf := &PackFile{
		Type:     packType,
		SrcDir:   tmpDir,
		Pack:     map[int]string{},
		Names:    map[string]struct{}{},
		NameToID: map[string]int{},
	}
	if err := pf.Load(packPath); err != nil {
		return nil, err
	}
	return pf, nil
}
