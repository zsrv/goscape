package maps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPack_BytePinned ports a minimal .jm2 input through Pack and
// asserts the four output files exist with non-zero sizes.
func TestPack_BytePinned(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	mapsDir := filepath.Join(src, "maps")
	if err := os.MkdirAll(mapsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// Filename per TS map/Pack.js:122-124: m<XZ>.jm2 — basename minus
	// extension minus first char, so "m5050.jm2" → mapXZ = "5050".
	// Pack writes outputs as m5050, l5050, n5050, o5050.
	body := strings.Join([]string{
		"==== MAP ====",
		"0 5 7: h10 o2 u3",
		"==== LOC ====",
		"0 5 7: 100",
		"==== NPC ====",
		"0 5 7: 42",
		"==== OBJ ====",
		"0 5 7: 99 10",
	}, "\n")
	if err := os.WriteFile(filepath.Join(mapsDir, "m5050.jm2"), []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	out := filepath.Join(tmp, "out")
	if err := Pack(src, out); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	for _, name := range []string{"m5050", "l5050", "n5050", "o5050"} {
		dest := filepath.Join(out, "server", "maps", name)
		info, err := os.Stat(dest)
		if err != nil {
			t.Errorf("Stat %q: %v", dest, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("%q is empty", dest)
		}
	}

	// Client-side compressed maps should also exist for m/l.
	for _, name := range []string{"m5050", "l5050"} {
		dest := filepath.Join(out, "client", "maps", name)
		if _, err := os.Stat(dest); err != nil {
			t.Errorf("Stat client %q: %v", dest, err)
		}
	}
}

// TestPack_MissingMapsDirNoOp pins the freshness-gated no-op when
// <srcDir>/maps doesn't exist.
func TestPack_MissingMapsDirNoOp(t *testing.T) {
	tmp := t.TempDir()
	if err := Pack(filepath.Join(tmp, "src"), filepath.Join(tmp, "out")); err != nil {
		t.Errorf("Pack: %v, want nil", err)
	}
}
