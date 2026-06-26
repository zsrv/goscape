package gamemap

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
)

// TestNAI98_RealCacheReachProbe_NPC943 — Repro A: player (3221, 3218),
// NPC 943 at (3218, 3216). Three-signal probe pins which sub-hypothesis
// (H6 BFS / H7 StepValidator vs BFS / H8 tickloop) fires. See spec
// §3.2 in docs/superpowers/specs/2026-05-05-nai-98-grounddecor-reach-stage2-design.md
func TestNAI98_RealCacheReachProbe_NPC943(t *testing.T) {
	runRealCacheReachProbe(t, 3221, 3218, 3218, 3216)
}

// TestNAI98_RealCacheReachProbe_NPC3 — Repro B: player (3218, 3213),
// NPC 3 at (3223, 3216). Same probe shape, different geometry.
func TestNAI98_RealCacheReachProbe_NPC3(t *testing.T) {
	runRealCacheReachProbe(t, 3218, 3213, 3223, 3216)
}

func runRealCacheReachProbe(t *testing.T, srcX, srcZ, dstX, dstZ int) {
	t.Helper()

	cacheDir := filepath.Join("..", "..", "data", "pack")
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "maps", "m48_50")); err != nil {
		t.Skipf("data/pack/server/maps/m48_50 unavailable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "server", "loc.dat")); err != nil {
		t.Skipf("data/pack/server/loc.dat unavailable: %v", err)
	}

	// Setup: real cache load + LocTypes + production populateStaticLocsIntoZones
	// replay (mirrors modules/world/server.go:315-330).
	gm := New(slog.New(slog.NewTextHandler(io.Discard, nil)))
	gm.SetMembers(true) // members world so real-cache content is not F2P-gated
	if err := gm.Init(cacheDir); err != nil {
		t.Fatalf("gamemap.Init: %v", err)
	}
	cfgs, err := objtype.LoadLocTypes(cacheDir)
	if err != nil {
		t.Skipf("LoadLocTypes (cross-revision data/pack?): %v", err)
	}
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

	const (
		level      = 0
		srcSize    = 1
		destWidth  = 1
		destLength = 1
	)

	// Signal H6 — BFS / reach predicate.
	route := gm.Pathfinder.FindPathToEntity(level, srcX, srcZ, dstX, dstZ, srcSize, destWidth, destLength)
	if !route.Success {
		t.Fatalf("H6 FIRES: FindPathToEntity Success=false on real-cache geometry. Route=%+v", route)
	}
	if len(route.Waypoints) == 0 {
		t.Fatalf("H6 FIRES: FindPathToEntity returned zero waypoints on real-cache geometry. Route=%+v", route)
	}
	last := route.Waypoints[len(route.Waypoints)-1]
	if dx := last.X() - dstX; dx < -1 || dx > 1 {
		t.Fatalf("H6 FIRES: last waypoint=(%d,%d) cheb-X=%d > 1 from dst=(%d,%d). Route=%+v",
			last.X(), last.Z(), dx, dstX, dstZ, route)
	}
	if dz := last.Z() - dstZ; dz < -1 || dz > 1 {
		t.Fatalf("H6 FIRES: last waypoint=(%d,%d) cheb-Z=%d > 1 from dst=(%d,%d). Route=%+v",
			last.X(), last.Z(), dz, dstX, dstZ, route)
	}

	// Signal H7 — StepValidator vs BFS-CanMove divergence.
	// BFS waypoints are direction-change points (per spec §3.2; routefinder.go:130-153);
	// walk single tiles along each waypoint→waypoint straight segment.
	x, z := srcX, srcZ
	for segIdx, wp := range route.Waypoints {
		dx, dz := wp.X()-x, wp.Z()-z
		sx := sgn(dx)
		sz := sgn(dz)
		if sx == 0 && sz == 0 {
			t.Skipf("Phase 1 surfaces unexpected route shape (degenerate same-tile waypoint at segment %d). Route=%+v", segIdx, route)
		}
		for x != wp.X() || z != wp.Z() {
			if !gm.CanTravel(level, x, z, sx, sz, 1, 0, collision.TypeNormal) {
				t.Fatalf("H7 FIRES at sub-step (%d,%d)→(%d,%d) inside segment %d/%d (waypoint (%d,%d)→(%d,%d)) step=(%d,%d) but CanTravel=false. Route=%+v",
					x, z, x+sx, z+sz, segIdx+1, len(route.Waypoints), x, z, wp.X(), wp.Z(), sx, sz, route)
			}
			x += sx
			z += sz
		}
	}

	// Post-fix durable regression: BFS path internally consistent and
	// StepValidator-walkable on real-cache geometry. Pre-NAI-98 surfaced
	// sub-H8 (tickloop level) by elimination here; closed by NAI-98 Phase 2
	// (commit eb64adf) port of TS Player.pathToPathingTarget. This probe
	// remains as a regression test for the BFS + StepValidator layer.
	t.Logf("BFS path internally consistent on (%d,%d)→(%d,%d): %d waypoints, last=(%d,%d), every sub-step CanTravel-passes.",
		srcX, srcZ, dstX, dstZ, len(route.Waypoints), last.X(), last.Z())
}

// sgn returns the sign of x in {-1, 0, 1}.
func sgn(x int) int {
	if x > 0 {
		return 1
	}
	if x < 0 {
		return -1
	}
	return 0
}
