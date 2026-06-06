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

// TestRegistry_AnimSet_MapPack_Midi pins NAI-213 / rev-244 B6: the three new
// PackFile singletons from TS PackFile.ts:191/205/206 @ 9aadcec4 are
// accessible via EnsureAnimSet / EnsureMap / EnsureMidi.
//
// TS: AnimSetPack = new PackFile('animset', validateFilesPack, [BUILD_SRC_DIR/models], '.anim')
// TS: MapPack     = new PackFile('map',     validateFilesPack, [BUILD_SRC_DIR/maps],   '.jm2',  false)
// TS: MidiPack    = new PackFile('midi',    validateFilesPack, [BUILD_SRC_DIR/jingles, BUILD_SRC_DIR/songs], '.mid')
func TestRegistry_AnimSet_MapPack_Midi(t *testing.T) {
	tmp := t.TempDir()
	for _, packType := range []string{"animset", "map", "midi"} {
		writeFile(t, filepath.Join(tmp, "pack", packType+".pack"), "0=test\n")
	}
	reg := &Registry{SrcDir: tmp}

	// AnimSet
	if reg.AnimSet != nil {
		t.Fatal("AnimSet should be nil pre-Ensure")
	}
	animSet, err := reg.EnsureAnimSet()
	if err != nil {
		t.Fatalf("EnsureAnimSet: %v", err)
	}
	if animSet == nil {
		t.Fatal("EnsureAnimSet returned nil")
	}
	if animSet.Type != "animset" {
		t.Errorf("AnimSet.Type=%q, want %q", animSet.Type, "animset")
	}
	if animSet.SrcDir != tmp {
		t.Errorf("AnimSet.SrcDir=%q, want %q", animSet.SrcDir, tmp)
	}
	// Idempotent
	second, err := reg.EnsureAnimSet()
	if err != nil {
		t.Fatalf("EnsureAnimSet (second): %v", err)
	}
	if second != animSet {
		t.Error("EnsureAnimSet not idempotent: new *PackFile returned on second call")
	}

	// Map
	if reg.Map != nil {
		t.Fatal("Map should be nil pre-Ensure")
	}
	mapPack, err := reg.EnsureMap()
	if err != nil {
		t.Fatalf("EnsureMap: %v", err)
	}
	if mapPack == nil {
		t.Fatal("EnsureMap returned nil")
	}
	if mapPack.Type != "map" {
		t.Errorf("Map.Type=%q, want %q", mapPack.Type, "map")
	}
	if mapPack.SrcDir != tmp {
		t.Errorf("Map.SrcDir=%q, want %q", mapPack.SrcDir, tmp)
	}
	// Idempotent
	mapSecond, err := reg.EnsureMap()
	if err != nil {
		t.Fatalf("EnsureMap (second): %v", err)
	}
	if mapSecond != mapPack {
		t.Error("EnsureMap not idempotent: new *PackFile returned on second call")
	}

	// Midi
	if reg.Midi != nil {
		t.Fatal("Midi should be nil pre-Ensure")
	}
	midiPack, err := reg.EnsureMidi()
	if err != nil {
		t.Fatalf("EnsureMidi: %v", err)
	}
	if midiPack == nil {
		t.Fatal("EnsureMidi returned nil")
	}
	if midiPack.Type != "midi" {
		t.Errorf("Midi.Type=%q, want %q", midiPack.Type, "midi")
	}
	// Idempotent
	midiSecond, err := reg.EnsureMidi()
	if err != nil {
		t.Fatalf("EnsureMidi (second): %v", err)
	}
	if midiSecond != midiPack {
		t.Error("EnsureMidi not idempotent: new *PackFile returned on second call")
	}
}
