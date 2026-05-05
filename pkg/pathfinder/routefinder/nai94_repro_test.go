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

// TestNAI94_SurvivalExpert_BlockedPassage is the H3 reproducer. Smoke shape
// from 2026-05-05 NAI-92 run: player at (3101, 3103) → NPC typeId=943 at
// (3103, 3095). Cheb=8. Real-game observation: player gets within ~6 tiles,
// no closer.
//
// This unit test uses an EMPTY FlagMap to isolate the question: does the
// pathfinder return a clean reaching path when there's no obstacle? If yes,
// the "gets within 6 tiles" symptom is downstream of FlagMap state (real
// cabin wall flags); if no, the truncation/algo issue reproduces even
// without walls. A second subtest plants a synthetic minimal cabin-wall to
// see whether moveNear closest-approach matches the in-game ~6-tile result.
//
// Real-mapsquare m48_50 fixture loading is OUT of scope for this plan
// (would drag world wiring into a unit test). NAI-95 may revisit this.
func TestNAI94_SurvivalExpert_BlockedPassage(t *testing.T) {
	const (
		level = 0
		srcX  = 3101
		srcZ  = 3103
		dstX  = 3103
		dstZ  = 3095
	)

	t.Run("EmptyFlagMap_MustReach", func(t *testing.T) {
		t.Skip("NAI-94: H3 — empty-flagmap cheb=8 path failed; observed: " +
			"Route.Success=true, Route.Waypoints=[] (same empty-waypoint shape as H1). " +
			"Lift this skip in NAI-95 once the fix lands.")

		pf := NewPathFinderAPI()

		route := pf.FindPathPlain(level, srcX, srcZ, dstX, dstZ)

		if !route.Success {
			t.Fatalf("Route.Success=false on empty FlagMap; cheb=8 unobstructed must succeed. Route=%+v", route)
		}
		if len(route.Waypoints) == 0 {
			t.Fatalf("Route.Waypoints empty on empty FlagMap")
		}
		last := route.Waypoints[len(route.Waypoints)-1]
		if last.X() != dstX || last.Z() != dstZ {
			t.Fatalf("last waypoint = (%d, %d); want (%d, %d). Full waypoints: %+v",
				last.X(), last.Z(), dstX, dstZ, route.Waypoints)
		}
	})

	t.Run("SyntheticCabinWall_MoveNearReports", func(t *testing.T) {
		pf := NewPathFinderAPI()

		// Synthetic horizontal wall at z=3099 spanning x=[3100..3105], blocking
		// north→south traversal. Player at z=3103 must detour around it. With
		// no detour available within search bounds, moveNear=true should yield
		// closest-approach. This is NOT the real m48_50 layout — it's a
		// minimal repro shape. Document divergence at the test site.
		level0 := 0
		for x := 3100; x <= 3105; x++ {
			pf.Flags.Add(x, 3099, level0, collision.FlagWallNorth)
			pf.Flags.Add(x, 3100, level0, collision.FlagWallSouth)
		}

		route := pf.FindPathPlain(level, srcX, srcZ, dstX, dstZ)

		// Just record the result — no assertion. The diagnosis report
		// captures the observed behavior. (Use t.Logf so -v shows it.)
		t.Logf("synthetic-wall route: Success=%v Alternative=%v len(Waypoints)=%d",
			route.Success, route.Alternative, len(route.Waypoints))
		if len(route.Waypoints) > 0 {
			last := route.Waypoints[len(route.Waypoints)-1]
			t.Logf("last waypoint: (%d, %d)", last.X(), last.Z())
		}
	})
}
