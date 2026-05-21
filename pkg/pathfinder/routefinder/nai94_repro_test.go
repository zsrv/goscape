package routefinder

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/pathfinder/internal"
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
	t.Skip("NAI-94: H1 reproducer — pathfinder returns empty Waypoints on cheb=2 straight-line with empty FlagMap. " +
		"Observed Route at NAI-94 audit time: {Waypoints:[] Alternative:true Success:true}. " +
		"NOTE (post-T4 audit): empty FlagMap is a DEGENERATE case — flags.Get() returns FlagNull=-1 " +
		"for unallocated zones, and CanMove(-1, mask, TypeNormal)=false for any non-zero mask, so BFS " +
		"cannot expand any direction. With zones allocated (FlagOpen=0 default) the pathfinder works " +
		"correctly (see TestNAI94_AllocatedZones_PathfinderWorks). Lift this skip in NAI-95 if the " +
		"production-side allocation gap is fixed in the pathfinder layer; otherwise this skip stays " +
		"because empty-FlagMap is not the production bug.")

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

// TestNAI94_RouteBlockerFlag_Consulted is the H2 reproducer for NAI-94 / NAI-95.
// Uses FlagLocRouteBlocker on the destination tile as a clean discriminator:
// that bit appears in every FlagBlock*RouteBlocker mask but in none of the
// regular FlagBlock* masks (see collision/flag.go), so a route-blocker-aware
// pathfinder refuses entry from all 8 directions while a regular pathfinder
// is unaffected.
//
// With useRouteBlockerFlags=true (NPC pathing in TS), the destination is
// unreachable and moveNear=false forces a clean Route{} (Success=false).
// With useRouteBlockerFlags=false (player pathing in TS), the route steps
// directly onto the destination.
//
// If useRouteBlockerFlags is unconsulted by RouteFinder (the prior // TODO
// at routefinder.go:44), both subtests behave identically.
func TestNAI94_RouteBlockerFlag_Consulted(t *testing.T) {
	const (
		level = 0
		// Synthetic local coords away from real mapsquares. dst is one tile
		// east of src so the regular-pathfinder case has a trivial 1-step route.
		srcX = 3000
		srcZ = 3000
		dstX = 3001
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
			// Allocate zones around src/dst so empty-FlagMap degeneracy
			// (FlagNull=-1 → CanMove returns false everywhere) doesn't
			// confound the H2 signal. See TestNAI94_AllocatedZones_PathfinderWorks
			// for the same setup convention.
			flags := internal.BuildCollisionMap(2995, 2995, 3010, 3010)
			// Plant a route-blocker on the destination tile. FlagLocRouteBlocker
			// appears in all FlagBlock*RouteBlocker masks but not in regular
			// FlagBlock* masks, so it blocks entry from every direction iff
			// useRouteBlockerFlags is honored.
			flags.Add(dstX, dstZ, level, collision.FlagLocRouteBlocker)

			rf := NewRouteFinder(flags, routefinderDefaultSearchMapSize, routefinderDefaultRingBufferSize, tc.useRouteBlockerFlags)

			// moveNear=false forces Route{} (Success=false) when the destination
			// can't be reached, rather than falling through to the closest-approach
			// branch which always yields Success=true.
			route := rf.FindRoute(level, srcX, srcZ, dstX, dstZ, 1, 1, 1, 0, -1, false, 0, 25, collision.TypeNormal)

			if route.Success != tc.wantSuccess {
				t.Errorf("Route.Success = %v; want %v. Route=%+v", route.Success, tc.wantSuccess, route)
			}
			if tc.wantReachesDest {
				if len(route.Waypoints) == 0 {
					t.Fatalf("Route.Waypoints empty; want path reaching (%d, %d). Route=%+v", dstX, dstZ, route)
				}
				last := route.Waypoints[len(route.Waypoints)-1]
				if last.X() != dstX || last.Z() != dstZ {
					t.Errorf("last waypoint = (%d, %d); want (%d, %d) [BlockerIgnored expects passage]", last.X(), last.Z(), dstX, dstZ)
				}
			} else {
				if len(route.Waypoints) != 0 {
					t.Errorf("Route.Waypoints non-empty (%d) but blocker should refuse passage. Route=%+v", len(route.Waypoints), route)
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
		t.Skip("NAI-94: H3 — empty-flagmap cheb=8 path produces empty Waypoints; observed: " +
			"{Waypoints:[] Alternative:true Success:true} (same shape as H1). " +
			"Same FlagNull-blocks-expansion degenerate case (see H1 skip note + TestNAI94_AllocatedZones_PathfinderWorks). " +
			"Lift this skip in NAI-95 only if production-allocation gap is fixed in pathfinder layer.")

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

// TestNAI94_AllocatedZones_PathfinderWorks is the corrective companion to the
// H1/H3 empty-FlagMap reproducers. After T4 audit (controller-side, post-T3
// commit), it became clear that the empty-FlagMap probe surfaces a DEGENERATE
// case (FlagNull=-1 returned for unallocated zones blocks all CanMove checks
// under TypeNormal), not the production bug. This test pins the same H1/H3
// coords against a FlagMap whose relevant zones ARE allocated (defaulting to
// FlagOpen=0, the real-world non-blocked tile state), confirming the
// unit-level pathfinder works correctly. This is positive elimination
// evidence: the NAI-92-surfaced "waypoint_idx=-1" bug is upstream of the
// pathfinder algorithm — likely in production-side FlagMap allocation, the
// caller's coord/srcSize/destWidth/destLength threading, or the consumer
// layer's interpretation of the returned Route.
//
// If this test ever FAILS at HEAD: a regression has been introduced into the
// pathfinder algorithm itself. Investigate before assuming H1/H3 fired.
func TestNAI94_AllocatedZones_PathfinderWorks(t *testing.T) {
	t.Run("HansCheb2", func(t *testing.T) {
		// Allocate a 3x3 zone box covering src and dst (and a margin).
		flags := internal.BuildCollisionMap(3216, 3220, 3222, 3226)
		rf := NewRouteFinderDefault(flags)
		route := rf.FindRouteDefault(0, 3219, 3224, 3219, 3222)

		if !route.Success {
			t.Fatalf("Success=false; cheb=2 with allocated empty zones must succeed. Route=%+v", route)
		}
		if route.Alternative {
			t.Fatalf("Alternative=true; cheb=2 unobstructed must reach via primary path. Route=%+v", route)
		}
		if len(route.Waypoints) == 0 {
			t.Fatalf("Waypoints empty; expected reaching path. Route=%+v", route)
		}
		last := route.Waypoints[len(route.Waypoints)-1]
		if last.X() != 3219 || last.Z() != 3222 {
			t.Fatalf("last waypoint (%d, %d) != dest (3219, 3222). Route=%+v", last.X(), last.Z(), route)
		}
	})

	t.Run("SurvivalExpertCheb8", func(t *testing.T) {
		// Allocate zones covering the full search window for src=(3101,3103), dst=(3103,3095).
		flags := internal.BuildCollisionMap(3095, 3088, 3110, 3110)
		rf := NewRouteFinderDefault(flags)
		route := rf.FindRouteDefault(0, 3101, 3103, 3103, 3095)

		if !route.Success {
			t.Fatalf("Success=false; cheb=8 with allocated empty zones must succeed. Route=%+v", route)
		}
		if route.Alternative {
			t.Fatalf("Alternative=true; cheb=8 unobstructed must reach via primary path. Route=%+v", route)
		}
		if len(route.Waypoints) == 0 {
			t.Fatalf("Waypoints empty; expected reaching path. Route=%+v", route)
		}
		last := route.Waypoints[len(route.Waypoints)-1]
		if last.X() != 3103 || last.Z() != 3095 {
			t.Fatalf("last waypoint (%d, %d) != dest (3103, 3095). Route=%+v", last.X(), last.Z(), route)
		}
	})
}
