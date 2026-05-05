package routefinder

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pathfinder/collision"
)

// newTestPathFinderAPI builds a PathFinderAPI with an empty FlagMap so
// FindPath calls return a Route deterministically (no walls, src=dest).
func newTestPathFinderAPI() PathFinderAPI {
	return NewPathFinderAPI()
}

// TestFindPathPlain_DelegatesToFindPath_WithDefaultArgs pins the
// 5-arg → 14-arg expansion: srcSize=1, destWidth=1, destLength=1,
// angle=0, shape=-1, moveNear=true, blockAccessFlags=0, maxWaypoints=25,
// type=Normal. Mirrors TS findPath (GameMap.ts:378-380).
func TestFindPathPlain_DelegatesToFindPath_WithDefaultArgs(t *testing.T) {
	pf := newTestPathFinderAPI()

	wrapper := pf.FindPathPlain(0, 100, 100, 100, 100)
	expanded := pf.FindPath(0, 100, 100, 100, 100, 1, 1, 1, 0, -1, true, 0, 25, collision.TypeNormal)

	if !routesEqual(wrapper, expanded) {
		t.Errorf("FindPathPlain != FindPath(... 1, 1, 1, 0, -1, true, 0, 25, Normal)")
	}
}

// TestFindPathToEntity_DelegatesToFindPath_WithEntitySentinel pins the
// shape=-2 entity-target sentinel and the threaded srcSize/destWidth/
// destLength. Mirrors TS findPathToEntity (GameMap.ts:382-384).
func TestFindPathToEntity_DelegatesToFindPath_WithEntitySentinel(t *testing.T) {
	pf := newTestPathFinderAPI()

	wrapper := pf.FindPathToEntity(0, 100, 100, 105, 105, 1, 2, 3)
	expanded := pf.FindPath(0, 100, 100, 105, 105, 1, 2, 3, 0, -2, true, 0, 25, collision.TypeNormal)

	if !routesEqual(wrapper, expanded) {
		t.Errorf("FindPathToEntity != FindPath(... srcSize, destW, destL, 0, -2, true, 0, 25, Normal)")
	}
}

// TestFindPathToLoc_DelegatesToFindPath_WithLocShapeAngle pins angle/
// shape/blockAccessFlags threaded through. Mirrors TS findPathToLoc
// (GameMap.ts:386-388).
func TestFindPathToLoc_DelegatesToFindPath_WithLocShapeAngle(t *testing.T) {
	pf := newTestPathFinderAPI()

	wrapper := pf.FindPathToLoc(0, 100, 100, 105, 105, 1, 1, 1, 2 /*angleEast*/, 0 /*wallStraight*/, 7 /*forceapproach*/)
	expanded := pf.FindPath(0, 100, 100, 105, 105, 1, 1, 1, 2, 0, true, 7, 25, collision.TypeNormal)

	if !routesEqual(wrapper, expanded) {
		t.Errorf("FindPathToLoc != FindPath(... srcSize, destW, destL, angle, shape, true, blockAccessFlags, 25, Normal)")
	}
}

// TestFindNaivePath_DelegatesToNaiveRouteFinder pins the 11-arg
// pass-through to NaiveRouteFinder.FindRoute. Mirrors TS findNaivePath
// (GameMap.ts:390-392).
func TestFindNaivePath_DelegatesToNaiveRouteFinder(t *testing.T) {
	pf := newTestPathFinderAPI()

	wrapper := pf.FindNaivePath(0, 100, 100, 105, 105, 1, 1, 1, 1, 0, collision.TypeNormal)
	expanded := pf.NaiveRouteFinder.FindRoute(0, 100, 100, 105, 105, 1, 1, 1, 1, 0, collision.TypeNormal)

	if !routesEqual(wrapper, expanded) {
		t.Errorf("FindNaivePath != NaiveRouteFinder.FindRoute pass-through")
	}
}

// routesEqual compares two Routes by Waypoints + Alternative + Success.
func routesEqual(a, b Route) bool {
	if a.Alternative != b.Alternative || a.Success != b.Success {
		return false
	}
	if len(a.Waypoints) != len(b.Waypoints) {
		return false
	}
	for i := range a.Waypoints {
		if a.Waypoints[i] != b.Waypoints[i] {
			return false
		}
	}
	return true
}
