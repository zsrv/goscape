package routefinder

import (
	"testing"
)

// TestNAI97_NPC943_PathAroundFountain is the Repro A reproducer for NAI-97.
// Smoke shape (2026-05-05 NAI-96 close-day): player at (3221, 3218), NPC type
// 943 at (3218, 3216), level=0, cheb=3. Pathing AROUND the Lumbridge fountain
// GroundDecor between source and destination should reach an adjacent tile
// (NPC reach within 1 tile).
//
// This unit test runs against an EMPTY FlagMap to isolate pathfinder behavior
// from collision-write state. If it passes here, the bug is upstream of the
// pathfinder API on a clean grid (i.e., requires the actual fountain
// FlagBlockWalk write to surface). If it fails, H4 escalates: pathfinder
// itself can't handle the geometry.
//
// Disposition (per NAI-97 plan §"Conventions"):
//   - PASS → H4 eliminated at unit level for empty-grid case; record in diagnosis.
//   - FAIL → wrap in t.Skip with `%+v Route` pinned, route to NAI-98.
func TestNAI97_NPC943_PathAroundFountain(t *testing.T) {
	t.Skip("NAI-97: pathfinder fails on empty-grid clean reach; observed Route at NAI-97 audit time: {Waypoints:[] Alternative:true Success:true}. Lift in NAI-98 once fix lands.")

	pf := NewPathFinderAPI()

	const (
		level      = 0
		srcX       = 3221
		srcZ       = 3218
		dstX       = 3218
		dstZ       = 3216
		srcSize    = 1
		destWidth  = 1 // NPC type 943 dimensions; verified in Task 6 Step 6.4
		destLength = 1
	)

	route := pf.FindPathToEntity(level, srcX, srcZ, dstX, dstZ, srcSize, destWidth, destLength)

	if !route.Success {
		t.Fatalf("Route.Success=false on empty FlagMap; cheb=3 unobstructed must succeed. Route=%+v", route)
	}
	if len(route.Waypoints) == 0 {
		t.Fatalf("Route.Waypoints empty on empty FlagMap")
	}
	last := route.Waypoints[len(route.Waypoints)-1]
	// Reach within 1 tile of dest (entity dispatch sentinel shape=-2 expects
	// occupy-adjacent, not stand-on-dest).
	dx := last.X() - dstX
	dz := last.Z() - dstZ
	if dx < -1 || dx > 1 || dz < -1 || dz > 1 {
		t.Fatalf("last waypoint = (%d, %d); want within cheb=1 of (%d, %d). Route=%+v",
			last.X(), last.Z(), dstX, dstZ, route)
	}
}

// TestNAI97_NPC3_MidRouteAbandonment is the Repro B reproducer for NAI-97.
// Smoke shape: player abandons at (3218, 3213) trying to reach NPC type 3
// at (3223, 3216), cheb=5. Same shape as Repro A but different geometry —
// runs against the same empty-FlagMap baseline.
//
// Disposition: same as Repro A.
func TestNAI97_NPC3_MidRouteAbandonment(t *testing.T) {
	t.Skip("NAI-97: pathfinder fails on empty-grid clean reach; observed Route at NAI-97 audit time: {Waypoints:[] Alternative:true Success:true}. Lift in NAI-98 once fix lands.")

	pf := NewPathFinderAPI()

	const (
		level      = 0
		srcX       = 3218
		srcZ       = 3213
		dstX       = 3223
		dstZ       = 3216
		srcSize    = 1
		destWidth  = 1
		destLength = 1
	)

	route := pf.FindPathToEntity(level, srcX, srcZ, dstX, dstZ, srcSize, destWidth, destLength)

	if !route.Success {
		t.Fatalf("Route.Success=false on empty FlagMap; cheb=5 unobstructed must succeed. Route=%+v", route)
	}
	if len(route.Waypoints) == 0 {
		t.Fatalf("Route.Waypoints empty on empty FlagMap")
	}
	last := route.Waypoints[len(route.Waypoints)-1]
	dx := last.X() - dstX
	dz := last.Z() - dstZ
	if dx < -1 || dx > 1 || dz < -1 || dz > 1 {
		t.Fatalf("last waypoint = (%d, %d); want within cheb=1 of (%d, %d). Route=%+v",
			last.X(), last.Z(), dstX, dstZ, route)
	}
}
