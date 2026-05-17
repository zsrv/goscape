package pack

import (
	"path/filepath"
	"testing"
)

// TestRegistry_LazyConstruct pins NAI-213 spec §Architecture: each
// EnsureX accessor lazy-constructs on first call and memoizes.
func TestRegistry_LazyConstruct(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "pack", "obj.pack"), "0=test\n")
	reg := &Registry{SrcDir: tmp}

	if reg.Obj != nil {
		t.Fatal("Obj should be nil pre-Ensure")
	}
	if _, err := reg.EnsureObj(); err != nil {
		t.Fatalf("EnsureObj: %v", err)
	}
	if reg.Obj == nil {
		t.Fatal("Obj should be non-nil post-Ensure")
	}
	first := reg.Obj
	if _, err := reg.EnsureObj(); err != nil {
		t.Fatalf("EnsureObj (second): %v", err)
	}
	if reg.Obj != first {
		t.Errorf("EnsureObj not idempotent: got new *PackFile on second call")
	}
}

// TestRegistry_PackConfigsForRegistry pins NAI-213 spec §Architecture:
// PackConfigsForRegistry returns a populated Registry.
func TestRegistry_PackConfigsForRegistry(t *testing.T) {
	tmp := t.TempDir()
	srcDir := filepath.Join(tmp, "src")
	outDir := filepath.Join(tmp, "out")
	// No source files needed — PackConfigs runs over an empty srcDir
	// without error (see TestPackConfigs_NoSourceFilesReturnsNoError).
	ClearFsCache()

	reg, err := PackConfigsForRegistry(srcDir, outDir)
	if err != nil {
		t.Fatalf("PackConfigsForRegistry: %v", err)
	}
	if reg == nil {
		t.Fatal("reg is nil")
	}
	if reg.SrcDir != srcDir {
		t.Errorf("SrcDir=%q, want %q", reg.SrcDir, srcDir)
	}
}

// TestPackConfigs_BackwardCompat pins that the original 2-arg signature
// still works (just discards the Registry).
func TestPackConfigs_BackwardCompat(t *testing.T) {
	tmp := t.TempDir()
	srcDir := filepath.Join(tmp, "src")
	outDir := filepath.Join(tmp, "out")
	ClearFsCache()

	if err := PackConfigs(srcDir, outDir); err != nil {
		t.Fatalf("PackConfigs: %v", err)
	}
}
