package gamemap

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// TestNAI99_FountainFootprintDump_Lumbridge loads real m48_50 (Lumbridge
// mapsquare), replays the production server.go:populateStaticLocsIntoZones
// collision-write loop in-test, and dumps every loc instance in the bbox
// around the Lumbridge fountain (NAI-98 smoke residual coords).
//
// User smoke 2026-05-05: walked NW from spawn (~3221, 3218); fountain is
// "multi-tile but treated as 1 tile wide; player walks partway in then
// stuck."
//
// bbox: x ∈ [3217..3225], z ∈ [3214..3220], level=0.
//
// Output is captured via t.Logf and lands in the NAI-99 diagnosis report
// as Stage 1.1 input. No assertions — this is a probe.
//
// Disposition: always passes; t.Skipf if cache fixture unavailable.
func TestNAI99_FountainFootprintDump_Lumbridge(t *testing.T) {
	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "maps", "m48_50")); err != nil {
		t.Skipf("data/pack/server/maps/m48_50 unavailable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "loc.dat")); err != nil {
		t.Skipf("data/pack/server/loc.dat unavailable: %v", err)
	}

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
		xLo   = 3217
		xHi   = 3225
		zLo   = 3214
		zHi   = 3220
	)

	type seen struct {
		x, z                int
		shape, angle, layer int
		ltID                int
		ltName              string
		ltWidth, ltLength   int
		blockWalk, blockRange bool
		active              int
	}
	var inBbox []seen

	// Replay populateStaticLocsIntoZones' collision-write gate
	// (modules/world/server.go:324-330) for every static loc — but only
	// inside the bbox, so the FlagMap stays focused and dump output is
	// manageable.
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
		// Mirror server.go:327 gate.
		if lt.BlockWalk {
			gm.ChangeLocCollision(l.Shape(), l.Angle(), lt.BlockRange,
				l.Length, l.Width, lt.Active, l.X, l.Z, l.Level, true)
		}
		inBbox = append(inBbox, seen{
			x: l.X, z: l.Z,
			shape: l.Shape(), angle: l.Angle(), layer: l.Layer(),
			ltID: ltID, ltName: lt.DebugName,
			ltWidth: lt.Width, ltLength: lt.Length,
			blockWalk: lt.BlockWalk, blockRange: lt.BlockRange,
			active: lt.Active,
		})
	}

	// Dump per-loc info. ltName fountain match is the primary identification key.
	t.Logf("=== NAI-99 Stage 1.1 loc dump: bbox x∈[%d,%d] z∈[%d,%d] level=%d ===", xLo, xHi, zLo, zHi, level)
	t.Logf("loc instances in bbox: %d", len(inBbox))
	for _, s := range inBbox {
		t.Logf("loc x=%d z=%d shape=%d angle=%d layer=%d locTypeID=%d name=%q W=%d L=%d BlockWalk=%v BlockRange=%v Active=%d",
			s.x, s.z, s.shape, s.angle, s.layer, s.ltID, s.ltName,
			s.ltWidth, s.ltLength, s.blockWalk, s.blockRange, s.active)
	}

	// Dump non-zero FlagMap state at every tile in bbox.
	t.Logf("=== NAI-99 Stage 1.1 FlagMap dump (post loc-collision-write replay) ===")
	flaggedCount := 0
	for z := zLo; z <= zHi; z++ {
		for x := xLo; x <= xHi; x++ {
			flag := gm.Pathfinder.Flags.Get(x, z, level)
			if flag == 0 {
				continue
			}
			t.Logf("flag x=%d z=%d level=%d = 0x%x", x, z, level, flag)
			flaggedCount++
		}
	}
	t.Logf("flagged tiles in bbox: %d", flaggedCount)
}
