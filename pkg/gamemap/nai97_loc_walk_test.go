package gamemap

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// TestNAI97_LocWalkDump_Lumbridge loads real m48_50 (Lumbridge mapsquare),
// replays the production server.go:populateStaticLocsIntoZones collision-
// write loop in-test, and dumps every loc in the bbox around the NAI-96
// smoke residual coords:
//
//	NPC 943 reach: (3221, 3218) → (3218, 3216)
//	NPC   3 reach: (3218, 3213) → (3223, 3216)
//
// bbox: x ∈ [3215..3225], z ∈ [3211..3220], level=0.
//
// Output is captured via t.Logf and lands in the NAI-97 diagnosis report
// as Stage 1.1 input. No assertions — this is a probe.
//
// Disposition: always passes; t.Skipf if cache fixture unavailable.
func TestNAI97_LocWalkDump_Lumbridge(t *testing.T) {
	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "maps", "m48_50")); err != nil {
		t.Skipf("data/pack/server/maps/m48_50 unavailable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "loc.dat")); err != nil {
		t.Skipf("data/pack/server/loc.dat unavailable: %v", err)
	}

	// Discard logger so test output isn't drowned in gamemap.Init INFO lines.
	gm := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := gm.Init(cacheDir); err != nil {
		t.Fatalf("gamemap.Init: %v", err)
	}

	cfgs, err := objtype.LoadLocTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadLocTypes: %v", err)
	}

	const (
		level = 0
		xLo   = 3215
		xHi   = 3225
		zLo   = 3211
		zHi   = 3220
	)

	// Replay populateStaticLocsIntoZones' collision-write gate
	// (modules/world/server.go:324-330) for every static loc — but only
	// inside the bbox, so we don't pollute the FlagMap globally and so
	// the dump output stays manageable.
	type seen struct {
		x, z, layer, locID, ltID int
		ltName                   string
		blockWalk                bool
		active                   int
	}
	var inBbox []seen

	for _, l := range gm.StaticLocs() {
		if l.Level != level {
			continue
		}
		if l.X < xLo || l.X > xHi || l.Z < zLo || l.Z > zHi {
			continue
		}
		ltID := l.Type()
		if ltID < 0 || ltID >= len(cfgs.Configs) {
			t.Logf("loc at (%d,%d) typeID %d out of range", l.X, l.Z, ltID)
			continue
		}
		lt := cfgs.Configs[ltID]
		if lt == nil {
			t.Logf("loc at (%d,%d) typeID %d nil config", l.X, l.Z, ltID)
			continue
		}
		// Mirror server.go:327 gate: only write if lt.BlockWalk.
		if lt.BlockWalk {
			gm.ChangeLocCollision(l.Shape(), l.Angle(), lt.BlockRange,
				l.Length, l.Width, lt.Active, l.X, l.Z, l.Level, true)
		}
		inBbox = append(inBbox, seen{
			x: l.X, z: l.Z, layer: l.Layer(), locID: ltID,
			ltID: ltID, ltName: lt.DebugName,
			blockWalk: lt.BlockWalk, active: lt.Active,
		})
	}

	// Dump per-loc info.
	t.Logf("=== NAI-97 Stage 1.1 loc dump: bbox x∈[%d,%d] z∈[%d,%d] level=%d ===", xLo, xHi, zLo, zHi, level)
	t.Logf("locs in bbox: %d", len(inBbox))
	for _, s := range inBbox {
		t.Logf("loc x=%d z=%d layer=%d locTypeID=%d name=%q BlockWalk=%v Active=%d",
			s.x, s.z, s.layer, s.locID, s.ltName, s.blockWalk, s.active)
	}

	// Dump FlagMap state at every tile in bbox.
	t.Logf("=== NAI-97 Stage 1.1 FlagMap dump (post loc-collision-write replay) ===")
	for z := zLo; z <= zHi; z++ {
		for x := xLo; x <= xHi; x++ {
			flag := gm.Pathfinder.Flags.Get(x, z, level)
			if flag == 0 {
				continue // skip clean tiles to keep output focused
			}
			t.Logf("flag x=%d z=%d level=%d = 0x%x", x, z, level, flag)
		}
	}
}
