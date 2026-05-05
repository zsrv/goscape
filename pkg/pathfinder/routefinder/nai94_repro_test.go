package routefinder

import (
	"testing"
)

// TestNAI94_HansCheb2_StraightLineMustReach is the H1 reproducer for NAI-94.
// World coords: src=(3219, 3224), dest=(3219, 3222). Cheb=2, straight-line N→S
// move with empty FlagMap (no walls). Smoke (2026-05-05) showed real-game
// dispatch returns waypoint_idx=-1 for this exact shape. This unit test pins
// the same coords against the actual RouteFinder API to determine whether
// the bug is in the pathfinder algo itself or in something upstream.
//
// Disposition (per NAI-94 plan §"Conventions"):
//   - If FAILS (anomaly reproduces here): wrap in t.Skip, pin observed
//     behavior, route to NAI-95.
//   - If PASSES: H1 is eliminated against the unit-level pathfinder; the
//     real-game bug is upstream of the pathfinder API. Document in diagnosis.
func TestNAI94_HansCheb2_StraightLineMustReach(t *testing.T) {
	t.Skip("NAI-94: H1 reproducer — pathfinder returns no/short path on cheb=2 straight-line with empty FlagMap. " +
		"Observed Route at NAI-94 audit time: {Waypoints:[] Alternative:false Success:true}. " +
		"Lift this skip in NAI-95 once the fix lands.")

	pf := NewPathFinderAPI()

	const (
		level = 0
		srcX  = 3219
		srcZ  = 3224
		dstX  = 3219
		dstZ  = 3222
	)

	route := pf.FindPathPlain(level, srcX, srcZ, dstX, dstZ)

	if !route.Success {
		t.Fatalf("Route.Success=false; expected pathfinder to succeed on cheb=2 straight-line with empty FlagMap. Route=%+v", route)
	}
	if len(route.Waypoints) == 0 {
		t.Fatalf("Route.Waypoints empty; expected at least one waypoint reaching (%d, %d)", dstX, dstZ)
	}
	last := route.Waypoints[len(route.Waypoints)-1]
	if last.X() != dstX || last.Z() != dstZ {
		t.Fatalf("last waypoint = (%d, %d); want (%d, %d). Full waypoints: %+v", last.X(), last.Z(), dstX, dstZ, route.Waypoints)
	}
}
