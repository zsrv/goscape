package maps

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/pack"
)

// TestMain stubs the packWorldmap seam for the whole package: the
// synthetic map trees in these unit tests carry no flo.dat / sprites /
// fonts / labels fixtures, so the real worldmap.Pack (invoked by Pack
// since 254 — TS Pack.js:383-385 @ 2e3bcf43) would fail on missing
// inputs. The wiring itself is pinned by TestPack_WorldmapRebuildSeam
// below; full worldmap output parity is covered by the packall
// full-tree gate (mapview/worldmap.jag in ref274_manifest.txt).
func TestMain(m *testing.M) {
	packWorldmap = func(srcDir, outDir string) error {
		worldmapCalls++
		return nil
	}
	os.Exit(m.Run())
}

// worldmapCalls counts stubbed packWorldmap invocations.
var worldmapCalls int

// TestPack_WorldmapRebuildSeam pins the 254 worldmap wiring
// (TS Pack.js:189 + :222 + :383-385 @ 2e3bcf43):
//  1. a fresh pack (map rebuilt) calls packWorldmap;
//  2. a re-run with everything fresh AND mapview/worldmap.jag present
//     does NOT call it;
//  3. a re-run with outputs fresh but worldmap.jag missing DOES call it
//     (the !fs.existsSync seed at TS Pack.js:189).
func TestPack_WorldmapRebuildSeam(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	out := filepath.Join(tmp, "out")
	if err := os.MkdirAll(filepath.Join(src, "maps"), 0o755); err != nil {
		t.Fatal(err)
	}
	jm2 := "==== MAP ====\n0 0 0: h1\n"
	if err := os.WriteFile(filepath.Join(src, "maps", "m50_50.jm2"), []byte(jm2), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. Fresh pack → map rebuilt → worldmap repacked.
	worldmapCalls = 0
	if err := Pack(src, out, nil, nil, nil); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	if worldmapCalls != 1 {
		t.Fatalf("fresh pack: packWorldmap called %d times, want 1", worldmapCalls)
	}

	// 2. Everything fresh + worldmap.jag present → no repack.
	// ClearFsCache between runs like packAll does (the freshness checks
	// read through the stat cache; run 1 cached the pre-write stats).
	if err := os.MkdirAll(filepath.Join(out, "mapview"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "mapview", "worldmap.jag"), []byte{0}, 0o644); err != nil {
		t.Fatal(err)
	}
	pack.ClearFsCache()
	worldmapCalls = 0
	if err := Pack(src, out, nil, nil, nil); err != nil {
		t.Fatalf("Pack (fresh re-run): %v", err)
	}
	if worldmapCalls != 0 {
		t.Fatalf("fresh re-run: packWorldmap called %d times, want 0", worldmapCalls)
	}

	// 3. Outputs fresh but worldmap.jag missing → repack (TS Pack.js:189).
	if err := os.Remove(filepath.Join(out, "mapview", "worldmap.jag")); err != nil {
		t.Fatal(err)
	}
	pack.ClearFsCache()
	worldmapCalls = 0
	if err := Pack(src, out, nil, nil, nil); err != nil {
		t.Fatalf("Pack (missing jag): %v", err)
	}
	if worldmapCalls != 1 {
		t.Fatalf("missing jag: packWorldmap called %d times, want 1", worldmapCalls)
	}
}
