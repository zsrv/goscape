package routefinder

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/pathfinder/internal"
)

// TestRouteFinderRouteFindBig_SouthToNorthLoopBound covers pathfinder-1
// (2026-05-28 fresh-audit MEDIUM): the routeFindBig south-to-north
// inner-edge loop bound must mirror rsmod's `1..src_size-1`, NOT
// `srcSize+1`. Pre-fix the loop probed two extra tiles (indexes
// srcSize-1 and srcSize), one of which lies OUTSIDE the actor's
// destination footprint at (srcX+srcSize, srcZ+srcSize). A
// FlagWalkBlocked at that outside tile spuriously rejected a legal
// north step for a 3×3 actor; post-fix the rejection no longer fires.
//
// Fixture: srcSize=3 actor at (3200, 3200). Target one tile north at
// (3200, 3201). Block tile (3203, 3203) — strictly outside both source
// AND destination footprints, only touched by the pre-fix extra probe
// at index=3 of the south-to-north inner-edge loop.
func TestRouteFinderRouteFindBig_SouthToNorthLoopBound(t *testing.T) {
	srcX, srcZ := 3200, 3200
	destX, destZ := 3200, 3201

	flags := internal.BuildCollisionMap(srcX, srcZ, destX, destZ)
	// FlagWalkBlocked is in FlagBlockSouthEastAndWest, so any
	// CanMove(_, FlagBlockSouthEastAndWest, TypeNormal) probe of this
	// tile reports blocked. (3203, 3203) is the index=3 extra probe
	// from the first north-step iteration; canonical (rsmod
	// `1..src_size-1` = indexes 1..src_size-2, i.e. only index=1 for
	// srcSize=3) never touches it.
	flags.Add(3203, 3203, 0, collision.FlagWalkBlocked)

	rf := NewRouteFinderDefault(flags)
	route := rf.FindRoute(0, srcX, srcZ, destX, destZ,
		3,    // srcSize=3 → routeFindBig branch
		1, 1, // destWidth, destLength
		0, -1, // angle, shape
		true,  // moveNear
		0, 25, // blockAccessFlags, maxWaypoints
		collision.TypeNormal,
	)

	if !route.Success {
		t.Fatalf("route.Success == false, want true (post-fix: outside tile (3203,3203) is no longer probed; rsmod loop bound is `1..src_size-1`, pathfinder1_test pin)")
	}
	// Post-fix the direct one-step north move is the entire path.
	if len(route.Waypoints) != 1 {
		t.Errorf("len(route.Waypoints) == %d, want 1 (direct north step should succeed when outside tile is the only blocker)", len(route.Waypoints))
		for i, w := range route.Waypoints {
			t.Logf("  waypoint[%d] = (%d, %d, level=%d)", i, w.X(), w.Z(), w.Level())
		}
		return
	}
	if route.Waypoints[0].X() != destX || route.Waypoints[0].Z() != destZ {
		t.Errorf("route.Waypoints[0] == (%d, %d), want (%d, %d) (TS routeFindBig south-to-north must reach the direct dest)",
			route.Waypoints[0].X(), route.Waypoints[0].Z(), destX, destZ)
	}
}
