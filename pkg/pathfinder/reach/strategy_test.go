package reach

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/pathfinder/flag"
	"github.com/zsrv/goscape/pkg/pathfinder/internal"
)

// TestReachStrategyReachLocWithAngle0And2 tests that loc angles are taken into account within Reached and
// do not rely on external modifications.
func TestReachStrategyReachLocWithAngle0And2(t *testing.T) {
	type dimension struct {
		width  int
		length int
	}

	type args struct {
		locX      int
		locZ      int
		dimension dimension
	}

	rotatedLocs := []args{
		{locX: 3203, locZ: 3203, dimension: dimension{width: 1, length: 1}},
		{locX: 3203, locZ: 3203, dimension: dimension{width: 1, length: 2}},
		{locX: 3203, locZ: 3203, dimension: dimension{width: 1, length: 3}},
		{locX: 3203, locZ: 3203, dimension: dimension{width: 2, length: 1}},
		{locX: 3203, locZ: 3203, dimension: dimension{width: 2, length: 2}},
		{locX: 3203, locZ: 3203, dimension: dimension{width: 2, length: 3}},
		{locX: 3203, locZ: 3203, dimension: dimension{width: 3, length: 1}},
		{locX: 3203, locZ: 3203, dimension: dimension{width: 3, length: 2}},
		{locX: 3203, locZ: 3203, dimension: dimension{width: 3, length: 3}},
	}

	for _, rl := range rotatedLocs {
		width := rl.dimension.width
		length := rl.dimension.length
		minX, minZ := rl.locX-16, rl.locZ-16
		maxX, maxZ := rl.locX+16, rl.locZ+16

		m := internal.BuildCollisionMap(minX, minZ, maxX, maxZ)
		internal.Flag(m, rl.locX, rl.locZ, width, length, collision.FlagLoc)

		reached := func(srcX, srcZ, angle, blockAccessFlags int) bool {
			return Reached(m, 0, srcX, srcZ, rl.locX, rl.locZ, width, length, 1, angle,
				-2, // use rectangular exclusive strategy
				blockAccessFlags)
		}

		for x := range width {
			// test coming from south tiles
			if !reached(rl.locX+x, rl.locZ-1, 0, 0) {
				t.Fatal("reached == false when coming from south tiles at angle 0, expected true")
			}
			if !reached(rl.locX+x, rl.locZ-1, 2, 0) {
				t.Fatal("reached == false when coming from south tiles at angle 2, expected true")
			}

			// test coming from north tiles
			if !reached(rl.locX+x, rl.locZ+length, 0, 0) {
				t.Fatal("reached == false when coming from north tiles at angle 0, expected true")
			}
			if !reached(rl.locX+x, rl.locZ+length, 2, 0) {
				t.Fatal("reached == false when coming from north tiles at angle 2, expected true")
			}

			// test coming from south tiles with access blocked
			if reached(rl.locX+x, rl.locZ-1, 0, int(flag.BlockAccessSouth)) {
				t.Fatal("reached == true when coming from south tiles with access blocked at angle 0, expected false")
			}
			if reached(rl.locX+x, rl.locZ-1, 2, int(flag.BlockAccessNorth)) {
				t.Fatal("reached == true when coming from south tiles with access blocked at angle 2, expected false")
			}

			// test coming from north tiles with access blocked
			if reached(rl.locX+x, rl.locZ+length, 0, int(flag.BlockAccessNorth)) {
				t.Fatal("reached == true when coming from north tiles with access blocked at angle 0, expected false")
			}
			if reached(rl.locX+x, rl.locZ+length, 2, int(flag.BlockAccessSouth)) {
				t.Fatal("reached == true when coming from north tiles with access blocked at angle 2, expected false")
			}
		}

		for z := range length {
			// test coming from west tiles
			if !reached(rl.locX-1, rl.locZ+z, 0, 0) {
				t.Fatal("reached == false when coming from west tiles at angle 0, expected true")
			}
			if !reached(rl.locX-1, rl.locZ+z, 2, 0) {
				t.Fatal("reached == false when coming from west tiles at angle 2, expected true")
			}

			// test coming from east tiles
			if !reached(rl.locX+width, rl.locZ+z, 0, 0) {
				t.Fatal("reached == false when coming from east tiles at angle 0, expected true")
			}
			if !reached(rl.locX+width, rl.locZ+z, 2, 0) {
				t.Fatal("reached == false when coming from east tiles at angle 2, expected true")
			}

			// test coming from west tiles with access blocked
			if reached(rl.locX-1, rl.locZ+z, 0, int(flag.BlockAccessWest)) {
				t.Fatal("reached == true when coming from west tiles with access blocked at angle 0, expected false")
			}
			if reached(rl.locX-1, rl.locZ+z, 2, int(flag.BlockAccessEast)) {
				t.Fatal("reached == true when coming from west tiles with access blocked at angle 2, expected false")
			}

			// test coming from east tiles with access blocked
			if reached(rl.locX+width, rl.locZ+z, 0, int(flag.BlockAccessEast)) {
				t.Fatal("reached == true when coming from east tiles with access blocked at angle 0, expected false")
			}
			if reached(rl.locX+width, rl.locZ+z, 2, int(flag.BlockAccessWest)) {
				t.Fatal("reached == true when coming from east tiles with access blocked at angle 2, expected false")
			}
		}
	}
}

// TestReachStrategyReachLocWithAngle1And3 tests that loc angles are taken into account within Reached and
// do not rely on external modifications.
func TestReachStrategyReachLocWithAngle1And3(t *testing.T) {
	type dimension struct {
		width  int
		length int
	}

	type args struct {
		locX      int
		locZ      int
		dimension dimension
	}

	rotatedLocs := []args{
		{locX: 3203, locZ: 3203, dimension: dimension{width: 1, length: 1}},
		{locX: 3203, locZ: 3203, dimension: dimension{width: 1, length: 2}},
		{locX: 3203, locZ: 3203, dimension: dimension{width: 1, length: 3}},
		{locX: 3203, locZ: 3203, dimension: dimension{width: 2, length: 1}},
		{locX: 3203, locZ: 3203, dimension: dimension{width: 2, length: 2}},
		{locX: 3203, locZ: 3203, dimension: dimension{width: 2, length: 3}},
		{locX: 3203, locZ: 3203, dimension: dimension{width: 3, length: 1}},
		{locX: 3203, locZ: 3203, dimension: dimension{width: 3, length: 2}},
		{locX: 3203, locZ: 3203, dimension: dimension{width: 3, length: 3}},
	}

	for _, rl := range rotatedLocs {
		width, length := rl.dimension.width, rl.dimension.length
		minX, minZ := rl.locX-16, rl.locZ-16
		maxX, maxZ := rl.locX+16, rl.locZ+16

		m := internal.BuildCollisionMap(minX, minZ, maxX, maxZ)
		internal.Flag(m, rl.locX, rl.locZ, length, width, collision.FlagLoc)

		reached := func(srcX, srcZ, angle, blockAccessFlags int) bool {
			return Reached(m, 0, srcX, srcZ, rl.locX, rl.locZ, width, length, 1, angle,
				-2, // use rectangular exclusive strategy
				blockAccessFlags)
		}

		for x := range length {
			// test coming from south tiles
			if !reached(rl.locX+x, rl.locZ-1, 1, 0) {
				t.Fatal("reached == false when coming from south tiles at angle 1, expected true")
			}
			if !reached(rl.locX+x, rl.locZ-1, 3, 0) {
				t.Fatal("reached == false when coming from south tiles at angle 3, expected true")
			}

			// test coming from north tiles
			if !reached(rl.locX+x, rl.locZ+width, 1, 0) {
				t.Fatal("reached == false when coming from north tiles at angle 1, expected true")
			}
			if !reached(rl.locX+x, rl.locZ+width, 3, 0) {
				t.Fatal("reached == false when coming from north tiles at angle 3, expected true")
			}

			// test coming from south tiles with access blocked
			if reached(rl.locX+x, rl.locZ-1, 1, int(flag.BlockAccessEast)) {
				t.Fatal("reached == true when coming from south tiles with access blocked at angle 1, expected false")
			}
			if reached(rl.locX+x, rl.locZ-1, 3, int(flag.BlockAccessWest)) {
				t.Fatal("reached == true when coming from south tiles with access blocked at angle 3, expected false")
			}

			// test coming from north tiles with access blocked
			if reached(rl.locX+x, rl.locZ+width, 1, int(flag.BlockAccessWest)) {
				t.Fatal("reached == true when coming from north tiles with access blocked at angle 1, expected false")
			}
			if reached(rl.locX+x, rl.locZ+width, 3, int(flag.BlockAccessEast)) {
				t.Fatal("reached == true when coming from north tiles with access blocked at angle 3, expected false")
			}
		}

		for z := range width {
			// test coming from west tiles
			if !reached(rl.locX-1, rl.locZ+z, 1, 0) {
				t.Fatal("reached == false when coming from west tiles at angle 1, expected true")
			}
			if !reached(rl.locX-1, rl.locZ+z, 3, 0) {
				t.Fatal("reached == false when coming from west tiles at angle 3, expected true")
			}

			// test coming from east tiles
			if !reached(rl.locX+length, rl.locZ+z, 1, 0) {
				t.Fatal("reached == false when coming from east tiles at angle 1, expected true")
			}
			if !reached(rl.locX+length, rl.locZ+z, 3, 0) {
				t.Fatal("reached == false when coming from east tiles at angle 3, expected true")
			}

			// test coming from west tiles with access blocked
			if reached(rl.locX-1, rl.locZ+z, 1, int(flag.BlockAccessSouth)) {
				t.Fatal("reached == true when coming from west tiles with access blocked at angle 1, expected false")
			}
			if reached(rl.locX-1, rl.locZ+z, 3, int(flag.BlockAccessNorth)) {
				t.Fatal("reached == true when coming from west tiles with access blocked at angle 3, expected false")
			}

			// test coming from east tiles with access blocked
			if reached(rl.locX+length, rl.locZ+z, 1, int(flag.BlockAccessNorth)) {
				t.Fatal("reached == true when coming from east tiles with access blocked at angle 1, expected false")
			}
			if reached(rl.locX+length, rl.locZ+z, 3, int(flag.BlockAccessSouth)) {
				t.Fatal("reached == true when coming from east tiles with access blocked at angle 3, expected false")
			}
		}
	}
}

// Rectangular exclusive reach strategy tests

func TestRectangularExclusiveReachStrategyReachWithBlockAccessFlagFromAppropriateDirections(t *testing.T) {
	for blockedDir := range flag.CardinalDirectionToOffset {
		locX, locZ := 3205, 3205
		m := internal.BuildCollisionMap(locX, locZ, locX, locZ)
		internal.Flag(m, locX, locZ, 1, 1, collision.FlagLoc)

		for dir, offsets := range flag.CardinalDirectionToOffset {
			srcX, srcZ := locX+offsets.OffX, locZ+offsets.OffZ
			m.AllocateIfAbsent(srcX, srcZ, 0)
			reached := ReachExclusiveRectangle(m, 0, srcX, srcZ, locX, locZ, 1, 1, 1, 0, blockedDir)
			if dir == blockedDir {
				if reached {
					t.Errorf("should not be able to reach loc with blockAccessFlag %d from direction %d", blockedDir, dir)
				}
			} else {
				if !reached {
					t.Errorf("should be able to reach loc with blockAccessFlag %d from direction %d", blockedDir, dir)
				}
			}
		}
	}
}

func TestRectangularExclusiveReachStrategyReachFromAllValidBorderCoordinates(t *testing.T) {
	type dimension struct {
		width  int
		length int
	}

	dimensions := []dimension{
		{width: 1, length: 1},
		{width: 1, length: 2},
		{width: 1, length: 3},
		{width: 2, length: 1},
		{width: 2, length: 2},
		{width: 2, length: 3},
		{width: 3, length: 1},
		{width: 3, length: 2},
		{width: 3, length: 3},
	}

	for _, d := range dimensions {
		width, length := d.width, d.length
		locX, locZ := 3202+width, 3202

		m := internal.BuildCollisionMap(locX, locZ, locX+width+1, locZ+length+1)
		internal.Flag(m, locX, locZ, width, length, collision.FlagLoc)

		reached := func(srcX, srcZ, destX, destZ int) bool {
			return ReachExclusiveRectangle(m, 0, srcX, srcZ, destX, destZ, 1, width, length, 0, 0)
		}

		if reached(locX-2, locZ-1, locX, locZ) {
			t.Fatalf("[width %d, length %d] should not be able to reach (locX, locZ) from (locX - 2, locZ - 1)", width, length)
		}

		if reached(locX-1, locZ-2, locX, locZ) {
			t.Fatalf("[width %d, length %d] should not be able to reach (locX, locZ) from (locX - 1, locZ - 2)", width, length)
		}

		for z := -1; z < length+1; z++ {
			for x := -1; x < width+1; x++ {
				r := reached(locX+x, locZ+z, locX, locZ)
				southwest := z == -1 && x == -1
				southeast := z == -1 && x == width
				northwest := z == length && x == -1
				northeast := z == length && x == width
				diagonal := southwest || northeast || southeast || northwest
				if diagonal {
					if r {
						t.Fatalf("[width %d, length %d] should not reach with offset (%d, %d)", width, length, x, z)
					}
					continue
				}
				inLocArea := x >= 0 && x < width && z >= 0 && z < length
				if inLocArea {
					if r {
						t.Fatalf("[width %d, length %d] should not reach from within loc area. (%d, %d)", width, length, x, z)
					}
					continue
				}
				if !r {
					t.Fatalf("[width %d, length %d] should reach with offset (%d, %d)", width, length, x, z)
				}
			}
		}
	}
}

// Rectangular reach strategy tests

func TestRectangularReachStrategyFailReachWhenDividedByAppropriateWallCollision(t *testing.T) {
	srcX, srcZ := 3200, 3200
	locX, locZ := 3200, 3201
	m := internal.BuildCollisionMap(srcX, srcZ, locX, locZ)
	// wall is located on same tile as source and flagged north
	m.Set(srcX, srcZ, 0, collision.FlagWallNorth)

	if ReachRectangle(m, 0, srcX, srcZ, locX, locZ, 1, 1, 1, 0, 0) {
		t.Fatal("ReachRectangle should return false")
	}

	// wall in every other direction should allow ReachRectangle to return true
	flags := []int{
		collision.FlagWallEast,
		collision.FlagWallSouth,
		collision.FlagWallWest,
		collision.FlagWallNorthWest,
		collision.FlagWallNorthEast,
		collision.FlagWallSouthEast,
		collision.FlagWallSouthWest,
	}
	for _, f := range flags {
		m.Set(srcX, srcZ, 0, f)
		if !ReachRectangle(m, 0, srcX, srcZ, locX, locZ, 1, 1, 1, 0, 0) {
			t.Fatalf("should be reachable with collision flag %d", f)
		}
	}
}

func TestRectangularReachStrategyReachWithBlockAccessFlagFromAppropriateDirections(t *testing.T) {
	for blockedDir := range flag.CardinalDirectionToOffset {
		locX, locZ := 3205, 3205
		m := internal.BuildCollisionMap(locX, locZ, locX, locZ)
		internal.Flag(m, locX, locZ, 1, 1, collision.FlagLoc)

		for dir, offsets := range flag.CardinalDirectionToOffset {
			srcX, srcZ := locX+offsets.OffX, locZ+offsets.OffZ
			m.AllocateIfAbsent(srcX, srcZ, 0)
			reached := ReachRectangle(m, 0, srcX, srcZ, locX, locZ, 1, 1, 1, 0, blockedDir)
			if dir == blockedDir {
				if reached {
					t.Errorf("should not be able to reach loc with blockAccessFlag %d from direction %d", blockedDir, dir)
				}
			} else {
				if !reached {
					t.Errorf("should be able to reach loc with blockAccessFlag %d from direction %d", blockedDir, dir)
				}
			}
		}
	}
}

func TestRectangularReachStrategyReachFromAllValidBorderCoordinates(t *testing.T) {
	type dimension struct {
		width  int
		length int
	}

	dimensions := []dimension{
		{width: 1, length: 1},
		{width: 1, length: 2},
		{width: 1, length: 3},
		{width: 2, length: 1},
		{width: 2, length: 2},
		{width: 2, length: 3},
		{width: 3, length: 1},
		{width: 3, length: 2},
		{width: 3, length: 3},
	}

	for _, d := range dimensions {
		width, length := d.width, d.length
		locX, locZ := 3202+width, 3202

		m := internal.BuildCollisionMap(locX-1, locZ-1, locX+width, locZ+length)
		internal.Flag(m, locX, locZ, width, length, collision.FlagLoc)

		reached := func(srcX, srcZ, destX, destZ int) bool {
			return ReachRectangle(m, 0, srcX, srcZ, destX, destZ, 1, width, length, 0, 0)
		}

		if reached(locX-2, locZ-1, locX, locZ) {
			t.Fatalf("[width %d, length %d] should not be able to reach (locX, locZ) from (locX - 2, locZ - 1)", width, length)
		}

		if reached(locX-1, locZ-2, locX, locZ) {
			t.Fatalf("[width %d, length %d] should not be able to reach (locX, locZ) from (locX - 1, locZ - 2)", width, length)
		}

		for z := -1; z < length+1; z++ {
			for x := -1; x < width+1; x++ {
				r := reached(locX+x, locZ+z, locX, locZ)
				southwest := z == -1 && x == -1
				southeast := z == -1 && x == width
				northwest := z == length && x == -1
				northeast := z == length && x == width
				diagonal := southwest || northeast || southeast || northwest
				if diagonal {
					if r {
						t.Fatalf("[width %d, length %d] should not reach with offset (%d, %d)", width, length, x, z)
					}
					continue
				}
				if !r {
					t.Fatalf("[width %d, length %d] should reach with offset (%d, %d)", width, length, x, z)
				}
			}
		}
	}
}
