package routefinder

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/pathfinder/flag"
)

func TestLineRouteFinderLineOfSightValidWhenOnTopOfBlockingCollisionFlagIfTargetOnSameCoordinate(t *testing.T) {
	m := collision.NewFlagMap()
	x, z := 3200, 3200

	for level := range 4 {
		m.Set(x, z, level, collision.FlagLoc)
	}

	rf := NewLineRouteFinder(m)

	for level := range 4 {
		if !rf.LineOfSight(level, x, z, x, z, 1, 0, 0, 0).Success {
			t.Fatalf("line of sight failed for level %d", level)
		}
	}
}

func TestLineRouteFinderLineOfSightValidWhenTargetCoordinateIsMarkedWithExtraFlagCollisionFlag(t *testing.T) {
	m := collision.NewFlagMap()
	m.Add(3200, 3200, 0, collision.FlagBlockPlayers)

	rf := NewLineRouteFinder(m)
	rayCast := rf.LineOfSight(0, 3200, 3202, 3200, 3200, 1, 1, 1, collision.FlagBlockPlayers)

	if len(rayCast.Coordinates) != 2 {
		t.Fatalf("len(rayCast.Coordinates) == %d, expected %d", len(rayCast.Coordinates), 2)
	}
	if !rayCast.Success {
		t.Fatal("rayCast.Success == false, expected true")
	}
	if rayCast.Alternative {
		t.Fatal("rayCast.Alternative == true, expected false")
	}
	if rayCast.Coordinates[0].X() != 3200 || rayCast.Coordinates[0].Z() != 3201 {
		t.Fatalf("rayCast coordinates for the first element are (%d, %d), expected (3200, 3201)", rayCast.Coordinates[0].X(), rayCast.Coordinates[0].Z())
	}
	if rayCast.Coordinates[1].X() != 3200 || rayCast.Coordinates[1].Z() != 3200 {
		t.Fatalf("rayCast coordinates for the last element are (%d, %d), expected (3200, 3200)", rayCast.Coordinates[1].X(), rayCast.Coordinates[1].Z())
	}
}

func TestLineRouteFinderLineOfSightFailWhenBlockedByExtraFlagBeforeReachingTarget(t *testing.T) {
	m := collision.NewFlagMap()
	m.Add(3200, 3200, 0, collision.FlagBlockPlayers)

	rf := NewLineRouteFinder(m)
	rayCast := rf.LineOfSight(0, 3200, 3202, 3200, 3199, 1, 1, 1, collision.FlagBlockPlayers)

	if len(rayCast.Coordinates) == 0 {
		t.Fatal("len(rayCast.Coordinates) == 0")
	}
	if rayCast.Success {
		t.Fatal("rayCast.Success == true, expected false")
	}
	if !rayCast.Alternative {
		t.Fatal("rayCast.Alternative == false, expected true")
	}
	if rayCast.Coordinates[len(rayCast.Coordinates)-1].X() != 3200 || rayCast.Coordinates[len(rayCast.Coordinates)-1].Z() != 3201 {
		t.Fatalf("rayCast coordinates for the last element are (%d, %d), expected (3200, 3200)", rayCast.Coordinates[len(rayCast.Coordinates)-1].X(), rayCast.Coordinates[len(rayCast.Coordinates)-1].Z())
	}
}

func TestLineRouteFinderLineOfSightFailWhenOnTopOfBlockingCollisionFlag(t *testing.T) {
	m := collision.NewFlagMap()
	m.Add(3200, 3200, 0, collision.FlagLoc)

	rf := NewLineRouteFinder(m)
	rayCast := rf.LineOfSight(0, 3200, 3200, 3200, 3201, 1, 0, 0, 0)

	if len(rayCast.Coordinates) != 0 {
		t.Fatalf("len(rayCast.Coordinates) == %d, expected 0", len(rayCast.Coordinates))
	}
	if rayCast.Success {
		t.Fatal("rayCast.Success == true, expected false")
	}
	if rayCast.Alternative {
		t.Fatal("rayCast.Alternative == true, expected false")
	}
}

func TestLineRouteFinderLineOfSightFailWhenOnTopOfExtraFlagCollisionFlag(t *testing.T) {
	m := collision.NewFlagMap()
	m.Add(3200, 3200, 0, collision.FlagBlockPlayers)

	rf := NewLineRouteFinder(m)
	rayCast := rf.LineOfSight(0, 3200, 3200, 3200, 3201, 1, 0, 0, collision.FlagBlockPlayers)

	if len(rayCast.Coordinates) != 0 {
		t.Fatalf("len(rayCast.Coordinates) == %d, expected 0", len(rayCast.Coordinates))
	}
	if rayCast.Success {
		t.Fatal("rayCast.Success == true, expected false")
	}
	if rayCast.Alternative {
		t.Fatal("rayCast.Alternative == true, expected false")
	}
}

func TestLineRouteFinderLineOfSightValidAndEmptyRayCastWhenOnTopOfTarget(t *testing.T) {
	m := collision.NewFlagMap()
	m.AllocateIfAbsent(3200, 3200, 0)

	rf := NewLineRouteFinder(m)
	rayCast := rf.LineOfSight(0, 3200, 3200, 3200, 3200, 1, 0, 0, 0)

	if len(rayCast.Coordinates) != 0 {
		t.Fatalf("len(rayCast.Coordinates) == %d, expected 0", len(rayCast.Coordinates))
	}
	if !rayCast.Success {
		t.Fatal("rayCast.Success == false, expected true")
	}
	if rayCast.Alternative {
		t.Fatal("rayCast.Alternative == true, expected false")
	}
}

func TestLineRouteFinderLineOfSightPartialRayCastWhenBlocked(t *testing.T) {
	m := collision.NewFlagMap()
	m.Set(3200, 3205, 0, collision.FlagLocProjBlocker)

	rf := NewLineRouteFinder(m)
	rayCast := rf.LineOfSight(0, 3200, 3200, 3200, 3207, 1, 0, 0, 0)

	if len(rayCast.Coordinates) != 4 {
		t.Fatalf("len(rayCast.Coordinates) == %d, expected 4", len(rayCast.Coordinates))
	}
	if rayCast.Success {
		t.Fatal("rayCast.Success == true, expected false")
	}
	if !rayCast.Alternative {
		t.Fatal("rayCast.Alternative == false, expected true")
	}
}

func TestLineRouteFinderLineOfSightValidWhenPassingThroughNonBlockingCollisionFlags(t *testing.T) {
	validFlags := []int{
		collision.FlagOpen,
		collision.FlagBlockWalk,
		collision.FlagGroundDecor,
		collision.FlagBlockWalk | collision.FlagGroundDecor,
	}

	for _, f := range validFlags {
		for dirEnum, dir := range flag.DirectionToOffset {
			m := collision.NewFlagMap()
			srcX, srcZ := 3200, 3200
			destX := srcX + (dir.OffX * 3)
			destZ := srcZ + (dir.OffZ * 3)

			for level := range 4 {
				for z := min(srcZ, destZ); z <= max(srcZ, destZ); z++ {
					for x := min(srcX, destX); x <= max(srcX, destX); x++ {
						m.AllocateIfAbsent(x, z, level)
					}
				}
			}

			for level := range 4 {
				m.Set(srcX+dir.OffX, srcZ+dir.OffZ, level, f)
			}

			rf := NewLineRouteFinder(m)
			for level := range 4 {
				rayCast := rf.LineOfSight(level, srcX, srcZ, destX, destZ, 1, 0, 0, 0)
				if len(rayCast.Coordinates) == 0 {
					t.Fatalf("[flag %d, dir %d, level %d] len(rayCast.Coordinates) == 0, expected non-zero", f, dirEnum, level)
				}
				if !rayCast.Success {
					t.Fatalf("[flag %d, dir %d, level %d] rayCast.Success == false, expected true", f, dirEnum, level)
				}
				if rayCast.Alternative {
					t.Fatalf("[flag %d, dir %d, level %d] rayCast.Alternative == true, expected false", f, dirEnum, level)
				}
			}
		}
	}
}

func TestLineRouteFinderLineOfSightFailWhenBlockedByLoc(t *testing.T) {
	for dirEnum, dir := range flag.DirectionToOffset {
		m := collision.NewFlagMap()
		srcX, srcZ := 3200, 3200
		destX := srcX + (dir.OffX * 3)
		destZ := srcZ + (dir.OffZ * 3)

		for level := range 4 {
			for z := min(srcZ, destZ); z <= max(srcZ, destZ); z++ {
				for x := min(srcX, destX); x <= max(srcX, destX); x++ {
					m.AllocateIfAbsent(x, z, level)
				}
			}
		}

		for level := range 4 {
			m.Set(srcX+dir.OffX, srcZ+dir.OffZ, level, collision.FlagLocProjBlocker)
		}

		rf := NewLineRouteFinder(m)
		for level := range 4 {
			if rf.LineOfSight(level, srcX, srcZ, destX, destZ, 1, 0, 0, 0).Success {
				t.Fatalf("[dir %d, level %d] line of sight success == true, expected false", dirEnum, level)
			}
		}
	}
}

func TestLineRouteFinderLineOfSightFailWhenBlockedByExtraFlagCollisionFlag(t *testing.T) {
	extraFlags := []int{
		collision.FlagBlockPlayers,
		collision.FlagBlockNPCs,
		collision.FlagBlockPlayers | collision.FlagBlockNPCs,
	}

	for _, f := range extraFlags {
		for dirEnum, dir := range flag.DirectionToOffset {
			m := collision.NewFlagMap()
			srcX, srcZ := 3200, 3200
			destX := srcX + (dir.OffX * 3)
			destZ := srcZ + (dir.OffZ * 3)

			for level := range 4 {
				for z := min(srcZ, destZ); z <= max(srcZ, destZ); z++ {
					for x := min(srcX, destX); x <= max(srcX, destX); x++ {
						m.AllocateIfAbsent(x, z, level)
					}
				}
			}

			for level := range 4 {
				m.Set(srcX+dir.OffX, srcZ+dir.OffZ, level, f)
			}

			rf := NewLineRouteFinder(m)
			for level := range 4 {
				if rf.LineOfSight(level, srcX, srcZ, destX, destZ, 1, 0, 0, f).Success {
					t.Fatalf("[dir %d, level %d] line of sight success == true, expected false", dirEnum, level)
				}
			}
		}
	}
}

func TestLineRouteFinderLineOfWalkValidWhenOnTopOfTargetCoordinates(t *testing.T) {
	m := collision.NewFlagMap()
	m.AllocateIfAbsent(3200, 3200, 0)
	rf := NewLineRouteFinder(m)
	if !rf.LineOfWalk(0, 3200, 3200, 3200, 3200, 1, 0, 0, 0).Success {
		t.Fatal("line of walk success == false, expected true")
	}
}

func TestLineRouteFinderLineOfWalkPartialRayCastWhenBlocked(t *testing.T) {
	m := collision.NewFlagMap()
	m.Set(3200, 3205, 0, collision.FlagLoc)

	rf := NewLineRouteFinder(m)
	rayCast := rf.LineOfWalk(0, 3200, 3200, 3200, 3207, 1, 0, 0, 0)

	if len(rayCast.Coordinates) != 4 {
		t.Fatalf("len(rayCast.Coordinates) == %d, expected 4", len(rayCast.Coordinates))
	}
	if rayCast.Success {
		t.Fatal("rayCast.Success == true, expected false")
	}
	if !rayCast.Alternative {
		t.Fatal("rayCast.Alternative == false, expected true")
	}
}

func TestLineRouteFinderLineOfWalkValidWhenPathClearOfCollisionFlags(t *testing.T) {
	for dirEnum, dir := range flag.DirectionToOffset {
		m := collision.NewFlagMap()
		srcX, srcZ := 3200, 3200
		destX := srcX + (dir.OffX * 3)
		destZ := srcZ + (dir.OffZ * 3)

		for level := range 4 {
			for z := min(srcZ, destZ); z <= max(srcZ, destZ); z++ {
				for x := min(srcX, destX); x <= max(srcX, destX); x++ {
					m.AllocateIfAbsent(x, z, level)
				}
			}
		}

		rf := NewLineRouteFinder(m)
		for level := range 4 {
			rayCast := rf.LineOfWalk(level, srcX, srcZ, destX, destZ, 1, 0, 0, 0)
			if len(rayCast.Coordinates) == 0 {
				t.Fatalf("[dir %d, level %d] len(rayCast.Coordinates) == 0, expected non-zero", dirEnum, level)
			}
			if !rayCast.Success {
				t.Fatalf("[dir %d, level %d] rayCast.Success == false, expected true", dirEnum, level)
			}
			if rayCast.Alternative {
				t.Fatalf("[dir %d, level %d] rayCast.Alternative == true, expected false", dirEnum, level)
			}
		}
	}
}

func TestLineRouteFinderLineOfWalkFailWhenPathBlockedByLoc(t *testing.T) {
	for dirEnum, dir := range flag.DirectionToOffset {
		m := collision.NewFlagMap()
		srcX, srcZ := 3200, 3200
		destX := srcX + (dir.OffX * 3)
		destZ := srcZ + (dir.OffZ * 3)

		for level := range 4 {
			m.Set(srcX+dir.OffX, srcZ+dir.OffZ, level, collision.FlagLoc)
		}

		rf := NewLineRouteFinder(m)
		for level := range 4 {
			if rf.LineOfWalk(level, srcX, srcZ, destX, destZ, 1, 0, 0, 0).Success {
				t.Fatalf("[dir %d, level %d] line of walk success == false, expected true", dirEnum, level)
			}
		}
	}
}

func TestLineRouteFinderLineOfWalkFailWhenPathBlockedByExtraFlagCollisionFlag(t *testing.T) {
	extraFlags := []int{
		collision.FlagBlockPlayers,
		collision.FlagBlockNPCs,
		collision.FlagBlockPlayers | collision.FlagBlockNPCs,
	}

	for _, f := range extraFlags {
		for dirEnum, dir := range flag.DirectionToOffset {
			m := collision.NewFlagMap()
			srcX, srcZ := 3200, 3200
			destX := srcX + (dir.OffX * 3)
			destZ := srcZ + (dir.OffZ * 3)

			for level := range 4 {
				m.Set(srcX+dir.OffX, srcZ+dir.OffZ, level, f)
			}

			rf := NewLineRouteFinder(m)
			for level := range 4 {
				if rf.LineOfWalk(level, srcX, srcZ, destX, destZ, 1, 0, 0, f).Success {
					t.Fatalf("[dir %d, level %d] line of sight success == true, expected false", dirEnum, level)
				}
			}
		}
	}
}
