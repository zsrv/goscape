package audio

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPackMidi_CompressesNew(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	for _, sub := range []string{"jingles", "songs"} {
		d := filepath.Join(src, sub)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(filepath.Join(d, "x.dat"), []byte("midi-bytes"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	out := filepath.Join(tmp, "out")
	if err := PackMidi(src, out); err != nil {
		t.Fatalf("PackMidi: %v", err)
	}

	for _, sub := range []string{"jingles", "songs"} {
		dest := filepath.Join(out, "client", sub, "x.dat")
		info, err := os.Stat(dest)
		if err != nil {
			t.Errorf("Stat %q: %v", dest, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("%q is empty", dest)
		}
	}
}

func TestPackMidi_SkipsExisting(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	out := filepath.Join(tmp, "out")
	for _, sub := range []string{"jingles", "songs"} {
		_ = os.MkdirAll(filepath.Join(src, sub), 0o755)
		_ = os.MkdirAll(filepath.Join(out, "client", sub), 0o755)
		_ = os.WriteFile(filepath.Join(src, sub, "x.dat"), []byte("new"), 0o644)
		_ = os.WriteFile(filepath.Join(out, "client", sub, "x.dat"), []byte("old"), 0o644)
	}

	if err := PackMidi(src, out); err != nil {
		t.Fatalf("PackMidi: %v", err)
	}
	for _, sub := range []string{"jingles", "songs"} {
		got, _ := os.ReadFile(filepath.Join(out, "client", sub, "x.dat"))
		if string(got) != "old" {
			t.Errorf("%s/x.dat = %q, want %q (skip not honored)", sub, got, "old")
		}
	}
}

func TestPackMidi_MissingSrcReturnsNil(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	out := filepath.Join(tmp, "out")
	if err := PackMidi(src, out); err != nil {
		t.Errorf("PackMidi: %v, want nil", err)
	}
}
