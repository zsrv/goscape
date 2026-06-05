package audio

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/pack"
)

func TestPackSound_BytePinned(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	synthDir := filepath.Join(src, "synth")
	packDir := filepath.Join(src, "pack")
	if err := os.MkdirAll(synthDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	if err := os.WriteFile(filepath.Join(synthDir, "a.synth"), []byte{1, 2}, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(synthDir, "b.synth"), []byte{3, 4, 5}, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "synth.pack"), []byte("0=a\n1=b\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "synth.order"), []byte("0\n1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	reg := &pack.Registry{SrcDir: src}
	if _, err := reg.EnsureSynth(); err != nil {
		t.Fatalf("EnsureSynth: %v", err)
	}

	outDir := filepath.Join(tmp, "out")
	// nil cache: no cache write; build-verify will log to stderr (fixture data
	// does not match soundCRCMagic — expected behaviour for synthetic input).
	if err := PackSound(reg, src, outDir, nil); err != nil {
		t.Fatalf("PackSound: %v", err)
	}

	jag, err := jagfile.LoadJagfile(filepath.Join(outDir, "client", "sounds"))
	if err != nil {
		t.Fatalf("LoadJagfile: %v", err)
	}
	soundsDat, err := jag.Read("sounds.dat")
	if err != nil {
		t.Fatalf("Read sounds.dat: %v", err)
	}
	want := []byte{0, 0, 1, 2, 0, 1, 3, 4, 5, 0xff, 0xff}
	got := make([]byte, soundsDat.Length())
	soundsDat.GData(got, soundsDat.Length())
	if string(got) != string(want) {
		t.Errorf("sounds.dat = %v, want %v", got, want)
	}
}

// TestPackSound_WritesToCache pins that when a non-nil FileStream is passed,
// PackSound writes the client/sounds bytes to cache(0, 8).
func TestPackSound_WritesToCache(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	_ = os.MkdirAll(filepath.Join(src, "synth"), 0o755)
	_ = os.MkdirAll(filepath.Join(src, "pack"), 0o755)
	// Minimal fixture: no synth files, empty order → terminator-only sounds.dat.
	_ = os.WriteFile(filepath.Join(src, "pack", "synth.order"), []byte(""), 0o644)

	reg := &pack.Registry{SrcDir: src}
	if _, err := reg.EnsureSynth(); err != nil {
		t.Fatalf("EnsureSynth: %v", err)
	}

	outDir := filepath.Join(tmp, "out")
	cacheDir := t.TempDir()
	fs := filestream.New(cacheDir, true, false)
	defer fs.Close()

	if err := PackSound(reg, src, outDir, fs); err != nil {
		t.Fatalf("PackSound: %v", err)
	}

	if !fs.Has(0, 8) {
		t.Fatal("cache(0,8) has no entry after PackSound")
	}
}
