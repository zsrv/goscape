package gamemap

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/loc"
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
// bbox: x ∈ [3217..3225], z ∈ [3214..3228], level=0. Initial run with
// z ∈ [3214..3220] (NAI-99 T2 first commit) returned 37 locs but no
// fountain — the actual Lumbridge "fountain" LocType (typeID=879,
// W=2 L=2 BlockWalk=true Active=1) sits at (3221, 3226) per global scan;
// widened zHi to 3228 to cover the W×L footprint plus a 1-tile margin.
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
		zHi   = 3228
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

// TestNAI99_FountainCoverage_Lumbridge asserts every tile in the
// W×L footprint of the Lumbridge fountain LocType (rotated by Angle
// per the LayerGround swap convention at gamemap.go:67-71) carries
// the expected flag — FlagBlockWalk for GroundDecor active=1, FlagLoc
// for LayerGround.
//
// User smoke 2026-05-05: fountain "treated like 1 tile wide; player
// walks partway in then stuck." This test pins which footprint tiles
// are flagged vs unflagged after collision-write replay.
//
// Fountain LocType identified in Stage 1.1 dump: typeID=879
// ("fountain", W=2, L=2, BlockWalk=true, BlockRange=false, Active=1)
// at static placement (3221, 3226) shape=10 angle=0.
//
// Disposition: if reproduces (some footprint tiles unflagged), add a
// t.Skip wrapper above the body with full assertion-failure output
// per skip_pin_full_struct_capture; lifting the skip is NAI-100's
// success criterion.
func TestNAI99_FountainCoverage_Lumbridge(t *testing.T) {
	const fountainTypeID = 879

	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "maps", "m48_50")); err != nil {
		t.Skipf("data/pack/server/maps/m48_50 unavailable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "loc.dat")); err != nil {
		t.Skipf("data/pack/server/loc.dat unavailable: %v", err)
	}

	cfgs, err := objtype.LoadLocTypes(cacheDir)
	if err != nil {
		t.Fatalf("LoadLocTypes: %v", err)
	}

	gm := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	gm.SetLocTypes(cfgs)
	if err := gm.Init(cacheDir); err != nil {
		t.Fatalf("gamemap.Init: %v", err)
	}

	// Replay collision-write globally (not bbox-limited) so we don't
	// miss adjacent-zone allocations that the W×L footprint may span.
	for _, l := range gm.StaticLocs() {
		ltID := l.Type()
		if ltID < 0 || ltID >= len(cfgs.Configs) {
			continue
		}
		lt := cfgs.Configs[ltID]
		if lt == nil || !lt.BlockWalk {
			continue
		}
		gm.ChangeLocCollision(l.Shape(), l.Angle(), lt.BlockRange,
			l.Length, l.Width, lt.Active, l.X, l.Z, l.Level, true)
	}

	// Find every static-loc instance of fountainTypeID.
	var fountains []*entity.Loc
	for _, l := range gm.StaticLocs() {
		if l.Type() == fountainTypeID {
			fountains = append(fountains, l)
		}
	}
	if len(fountains) == 0 {
		t.Fatalf("NAI-99: no fountain instance with typeID=%d found in StaticLocs", fountainTypeID)
	}
	t.Logf("NAI-99: %d fountain instance(s) found for typeID=%d", len(fountains), fountainTypeID)

	// Assert footprint coverage on the first multi-tile instance.
	lt := cfgs.Configs[fountainTypeID]
	for idx, f := range fountains {
		// Apply the same length/width swap as gamemap.go:67-71 for LayerGround
		// (NORTH/SOUTH = identity; EAST/WEST = swap). For LayerGroundDecor,
		// rotation does not swap — single-tile origin.
		w, l := lt.Width, lt.Length
		if loc.LayerOf(loc.Shape(f.Shape())) == loc.LayerGround {
			if f.Angle() != loc.AngleNorth && f.Angle() != loc.AngleSouth {
				w, l = l, w
			}
		}

		var unflagged []string
		var flagged []string
		for dz := 0; dz < l; dz++ {
			for dx := 0; dx < w; dx++ {
				tx, tz := f.X+dx, f.Z+dz
				flag := gm.Pathfinder.Flags.Get(tx, tz, f.Level)
				cell := fmt.Sprintf("(%d,%d)=0x%x", tx, tz, flag)
				if flag == 0 {
					unflagged = append(unflagged, cell)
				} else {
					flagged = append(flagged, cell)
				}
			}
		}
		t.Logf("NAI-99 instance %d: typeID=%d origin=(%d,%d,%d) shape=%d angle=%d W=%d L=%d (rotated W=%d L=%d) flagged=%v unflagged=%v",
			idx, fountainTypeID, f.X, f.Z, f.Level, f.Shape(), f.Angle(), lt.Width, lt.Length, w, l, flagged, unflagged)
		if idx == 0 {
			if len(unflagged) > 0 {
				t.Errorf("NAI-99: instance 0 footprint coverage divergence — flagged=%v unflagged=%v expected all %d tiles flagged",
					flagged, unflagged, w*l)
			}
		}
	}
}
