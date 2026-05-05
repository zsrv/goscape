package routefinder

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pathfinder/collision"
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

// TestNAI94_RouteBlockerFlag_Consulted is the H2 reproducer. Builds a synthetic
// scenario with a single FlagWallWestRouteBlocker at (3002, 3000). With
// useRouteBlockerFlags=true (NPC pathing in TS), the route should refuse to
// step W→E across that tile boundary. With useRouteBlockerFlags=false
// (player pathing in TS), the route should pass through.
//
// In goscape, RouteFinder is constructed via NewRouteFinderDefault with
// useRouteBlockerFlags=false (per api.go). If the field is unconsulted
// (the // TODO at routefinder.go), both subtests behave identically and
// H2 is confirmed.
func TestNAI94_RouteBlockerFlag_Consulted(t *testing.T) {
	const (
		level = 0
		// Use synthetic local coords centered well away from real mapsquares
		// to avoid accidentally seeding any pre-existing flag state.
		srcX = 3000
		srcZ = 3000
		dstX = 3004
		dstZ = 3000
	)

	for _, tc := range []struct {
		name                 string
		useRouteBlockerFlags bool
		wantSuccess          bool
		wantReachesDest      bool
	}{
		{name: "BlockerHonored_RefusesToCross", useRouteBlockerFlags: true, wantSuccess: false, wantReachesDest: false},
		{name: "BlockerIgnored_PassesThrough", useRouteBlockerFlags: false, wantSuccess: true, wantReachesDest: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Skip("NAI-94: H2 reproducer — useRouteBlockerFlags appears unconsulted; observed: " +
				"both subtests pass through blocker (BlockerHonored_RefusesToCross got Success=true, " +
				"BlockerIgnored_PassesThrough got Success=true). Lift this skip in NAI-95 once the fix lands.")

			flags := collision.NewFlagMap()
			// Plant a route-blocker on tile (3002, 3000) blocking westward step
			// from (3003, 3000) → (3002, 3000). FlagWallWestRouteBlocker on the
			// destination tile blocks entry from the east side.
			flags.Add(3002, 3000, level, collision.FlagWallWestRouteBlocker)

			rf := NewRouteFinder(flags, routefinderDefaultSearchMapSize, routefinderDefaultRingBufferSize, tc.useRouteBlockerFlags)

			route := rf.FindRoute(level, srcX, srcZ, dstX, dstZ, 1, 1, 1, 0, -1, true, 0, 25, collision.TypeNormal)

			if route.Success != tc.wantSuccess {
				t.Errorf("Route.Success = %v; want %v. Route=%+v", route.Success, tc.wantSuccess, route)
			}
			if tc.wantReachesDest && len(route.Waypoints) > 0 {
				last := route.Waypoints[len(route.Waypoints)-1]
				if last.X() != dstX || last.Z() != dstZ {
					t.Errorf("last waypoint = (%d, %d); want (%d, %d) [BlockerIgnored expects passage]", last.X(), last.Z(), dstX, dstZ)
				}
			}
		})
	}
}
