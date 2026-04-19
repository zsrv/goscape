package routefinder

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"testing"

	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/pathfinder/flag"
	"github.com/zsrv/goscape/pkg/pathfinder/internal"
)

// TODO: write the rest of the routefinder tests (and benchmarks)

func TestRouteFinderEnsureRouteWaypointsMatchRequestLevel(t *testing.T) {
	srcX := 3200
	srcZ := 3200
	destX := 3201
	destZ := 3200

	flags := internal.BuildCollisionMap(srcX, srcZ, destX, destZ)
	routeFinder := NewRouteFinderDefault(flags)

	type args struct {
		level int
		srcX  int
		srcZ  int
		destX int
		destZ int
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "level 0",
			args: args{
				level: 0,
			},
		},
		{
			name: "level 1",
			args: args{
				level: 1,
			},
		},
		{
			name: "level 2",
			args: args{
				level: 2,
			},
		},
		{
			name: "level 3",
			args: args{
				level: 3,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route := routeFinder.FindRouteDefault(tt.args.level, srcX, srcZ, destX, destZ)
			if !route.Success {
				t.Error("route.Success == false, want true")
			}
			for i := range len(route.Waypoints) {
				if route.Waypoints[i].Level() != tt.args.level {
					t.Errorf("route.Waypoints[%d].Level() == %d, want %d", i, route.Waypoints[i].Level(), tt.args.level)
				}
			}
		})
	}
}

func TestRouteFinderReturnAlternateRouteWhenSurroundedByLocsWithMoveNearFlag(t *testing.T) {
	srcX := 3200
	srcZ := 3200
	destX := 3205
	destZ := 3200

	flags := internal.BuildCollisionMap(srcX, srcZ, destX, destZ)
	internal.Flag(flags, srcX-1, srcZ-1, 3, 3, collision.FlagLoc)
	flags.Set(srcX, srcZ, 0, collision.FlagOpen) // remove collision flag from source tile

	routeFinder := NewRouteFinderDefault(flags)
	route := routeFinder.FindRouteDefault(0, srcX, srcZ, destX, destZ)
	if !route.Alternative {
		t.Error("route.Alternative == false, want true")
	}
	if len(route.Waypoints) > 0 {
		t.Errorf("len(route.Waypoints) == %d, want 0", len(route.Waypoints))
	}
}

func TestRouteFinderFailRouteWhenSurroundedByLocsWithoutMoveNearFlag(t *testing.T) {
	srcX := 3200
	srcZ := 3200
	destX := 3205
	destZ := 3200

	flags := internal.BuildCollisionMap(srcX, srcZ, destX, destZ)
	internal.Flag(flags, srcX-1, srcZ-1, 3, 3, collision.FlagLoc)
	flags.Set(srcX, srcZ, 0, collision.FlagOpen) // remove collision flag from source tile

	routeFinder := NewRouteFinderDefault(flags)
	route := routeFinder.FindRoute(0, srcX, srcZ, destX, destZ, 1, 1, 1, 0, -1, false, 0, 25, collision.TypeNormal)

	if route.Success {
		t.Error("route.Success == true, want false")
	}
	if len(route.Waypoints) > 0 {
		t.Errorf("len(route.Waypoints) == %d, want 0", len(route.Waypoints))
	}
}

func TestRouteFinderManeuverAroundThroughSingleExitPoint(t *testing.T) {
	srcX := 3200
	srcZ := 3200
	destX := 3200
	destZ := 3205

	flags := internal.BuildCollisionMap(srcX, srcZ, destX, destZ)
	internal.Flag(flags, srcX-1, srcZ-1, 3, 3, collision.FlagLoc)
	flags.Set(srcX, srcZ, 0, collision.FlagOpen)   // remove collision flag from source tile
	flags.Set(srcX, srcZ-1, 0, collision.FlagOpen) // remove collision flag from tile south of source tile

	routeFinder := NewRouteFinderDefault(flags)
	route := routeFinder.FindRouteDefault(0, srcX, srcZ, destX, destZ)
	if !route.Success {
		t.Error("route.Success == false, want true")
	}

	if len(route.Waypoints) != 4 {
		t.Errorf("len(route.Waypoints) == %d, want 4", len(route.Waypoints))
	}

	if route.Waypoints[0].X() != 3200 {
		t.Errorf("route.Waypoints[0].X() == %d, want 3200", route.Waypoints[0].X())
	}
	if route.Waypoints[0].Z() != 3198 {
		t.Errorf("route.Waypoints[0].Z() == %d, want 3198", route.Waypoints[0].Z())
	}

	if route.Waypoints[1].X() != 3198 {
		t.Errorf("route.Waypoints[1].X() == %d, want 3198", route.Waypoints[1].X())
	}
	if route.Waypoints[1].Z() != 3198 {
		t.Errorf("route.Waypoints[1].Z() == %d, want 3198", route.Waypoints[1].Z())
	}

	if route.Waypoints[2].X() != 3198 {
		t.Errorf("route.Waypoints[2].X() == %d, want 3198", route.Waypoints[2].X())
	}
	if route.Waypoints[2].Z() != 3203 {
		t.Errorf("route.Waypoints[2].Z() == %d, want 3203", route.Waypoints[2].Z())
	}

	if route.Waypoints[3].X() != destX {
		t.Errorf("route.Waypoints[3].X() == %d, want %d", route.Waypoints[3].X(), destX)
	}
	if route.Waypoints[3].Z() != destZ {
		t.Errorf("route.Waypoints[3].Z() == %d, want %d", route.Waypoints[3].Z(), destZ)
	}
}

func TestRouteFinderReturnEmptyAndSuccessfulRouteWhenStandingOnFinalRouteCoordinate(t *testing.T) {
	srcX := 3200
	srcZ := 3200
	locX := 3200
	locZ := 3201

	flags := internal.BuildCollisionMap(srcX, srcZ, locX, locZ)
	flags.Add(locX, locZ, 0, collision.FlagWallNorth|collision.FlagWallSouth|collision.FlagWallWest|collision.FlagWallEast)

	routeFinder := NewRouteFinderDefault(flags)
	route := routeFinder.FindRouteDefault(0, srcX, srcZ, locX, locZ)
	if !route.Success {
		t.Error("route.Success == false, want true")
	}
	if !route.Alternative {
		t.Error("route.Alternative == false, want true")
	}
	if len(route.Waypoints) > 0 {
		t.Errorf("len(route.Waypoints) == %d, want 0", len(route.Waypoints))
	}
}

func TestRouteFinderFindValidRouteTowardsDirection(t *testing.T) {
	for dirEnum, dir := range flag.DirectionToOffset {
		srcX, srcZ := 3200, 3200
		destX := srcX + dir.OffX
		destZ := srcZ + dir.OffZ

		m := internal.BuildCollisionMap(srcX, srcZ, destX, destZ)

		rf := NewRouteFinderDefault(m)
		for level := range 4 {
			route := rf.FindRouteDefault(level, srcX, srcZ, destX, destZ)
			if len(route.Waypoints) != 1 {
				t.Fatalf("[dir %d, level %d] len(route.Waypoints) == %d, want 1", dirEnum, level, len(route.Waypoints))
			}
			if route.Waypoints[len(route.Waypoints)-1].X() != destX {
				t.Fatalf("[dir %d, level %d] last route waypoint x == %d, expected %d", dirEnum, level, route.Waypoints[len(route.Waypoints)-1].X(), destX)
			}
			if route.Waypoints[len(route.Waypoints)-1].Z() != destZ {
				t.Fatalf("[dir %d, level %d] last route waypoint z == %d, expected %d", dirEnum, level, route.Waypoints[len(route.Waypoints)-1].Z(), destX)
			}
		}
	}
}

func TestRouteFinderFailRouteTowardsDirectionWhenBlocked(t *testing.T) {
	for dirEnum, dir := range flag.DirectionToOffset {
		srcX, srcZ := 3200, 3200
		destX := srcX + dir.OffX
		destZ := srcZ + dir.OffZ

		m := internal.BuildCollisionMap(srcX, srcZ, destX, destZ)
		internal.Flag(m, destX, destZ, 1, 1, collision.FlagBlockWalk)

		rf := NewRouteFinderDefault(m)
		for level := range 4 {
			route := rf.FindRoute(level, srcX, srcZ, destX, destZ, 1, 1, 1, 0, -1, false, 0, 25, collision.TypeNormal)
			if route.Success {
				t.Fatalf("[dir %d, level %d] route.Success == true, expected false", dirEnum, level)
			}
			if len(route.Waypoints) != 0 {
				t.Fatalf("[dir %d, level %d] len(route.Waypoints) == %d, want 0", dirEnum, level, len(route.Waypoints))
			}
			if route.Alternative {
				t.Fatalf("[dir %d, level %d] route.Alternative == true, expected false", dirEnum, level)
			}
		}
	}
}

// TestRouteFinderValidateRoutePathAgainstLocWithAngle0And2 tests that loc angles are taken
// into account within RouteFinder.FindRoute and do not rely on external modifications.
func TestRouteFinderValidateRoutePathAgainstLocWithAngle0And2(t *testing.T) {
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
		t.Run(fmt.Sprintf("locX %d, locZ %d, width %d, length %d",
			rl.locX, rl.locZ, rl.dimension.width, rl.dimension.length), func(t *testing.T) {
			width := rl.dimension.width
			length := rl.dimension.length
			minCoords := NewRouteCoordinates(rl.locX-16, rl.locZ-16, 0)
			maxCoords := NewRouteCoordinates(rl.locX+16, rl.locZ+16, 0)

			flags := internal.BuildCollisionMap(minCoords.X(), minCoords.Z(), maxCoords.X(), maxCoords.Z())
			internal.Flag(flags, rl.locX, rl.locZ, width, length, collision.FlagLoc)

			routeFinder := NewRouteFinderDefault(flags)
			route := func(srcX, srcZ, angle, blockAccessFlags int) Route {
				return routeFinder.FindRoute(0, srcX, srcZ, rl.locX, rl.locZ,
					1, width, length, angle,
					-2, // use rectangular exclusive strategy
					true, blockAccessFlags, 25, collision.TypeNormal)
			}

			for x := range width {
				t.Run(fmt.Sprintf("width %d", x), func(t *testing.T) {
					t.Run("coming from south tiles at angle 0", func(t *testing.T) {
						r := route(rl.locX+x, rl.locZ-3, 0, 0)
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].X() != rl.locX+x {
							t.Errorf("last waypoint x == %d, want %d", r.Waypoints[len(r.Waypoints)-1].X(), rl.locX+x)
						}
						if r.Waypoints[len(r.Waypoints)-1].Z() != rl.locZ-1 {
							t.Errorf("last waypoint z == %d, want %d", r.Waypoints[len(r.Waypoints)-1].Z(), rl.locZ-1)
						}
					})
					t.Run("coming from south tiles at angle 2", func(t *testing.T) {
						r := route(rl.locX+x, rl.locZ-3, 2, 0)
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].X() != rl.locX+x {
							t.Errorf("last waypoint x == %d, want %d", r.Waypoints[len(r.Waypoints)-1].X(), rl.locX+x)
						}
						if r.Waypoints[len(r.Waypoints)-1].Z() != rl.locZ-1 {
							t.Errorf("last waypoint z == %d, want %d", r.Waypoints[len(r.Waypoints)-1].Z(), rl.locZ-1)
						}
					})

					t.Run("coming from north tiles at angle 0", func(t *testing.T) {
						r := route(rl.locX+x, rl.locZ+length+3, 0, 0)
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].X() != rl.locX+x {
							t.Errorf("last waypoint x == %d, want %d", r.Waypoints[len(r.Waypoints)-1].X(), rl.locX+x)
						}
						if r.Waypoints[len(r.Waypoints)-1].Z() != rl.locZ+length {
							t.Errorf("last waypoint z == %d, want %d", r.Waypoints[len(r.Waypoints)-1].Z(), rl.locZ+length)
						}
					})
					t.Run("coming from north tiles at angle 2", func(t *testing.T) {
						r := route(rl.locX+x, rl.locZ+length+3, 2, 0)
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].X() != rl.locX+x {
							t.Errorf("last waypoint x == %d, want %d", r.Waypoints[len(r.Waypoints)-1].X(), rl.locX+x)
						}
						if r.Waypoints[len(r.Waypoints)-1].Z() != rl.locZ+length {
							t.Errorf("last waypoint z == %d, want %d", r.Waypoints[len(r.Waypoints)-1].Z(), rl.locZ+length)
						}
					})

					t.Run("coming from south tiles with access blocked at angle 0", func(t *testing.T) {
						r := route(rl.locX+x, rl.locZ-3, 0, int(flag.BlockAccessSouth))
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].Z() == rl.locZ-1 {
							t.Errorf("last waypoint z == %d", r.Waypoints[len(r.Waypoints)-1].Z())
						}
					})
					t.Run("coming from south tiles with access blocked at angle 2", func(t *testing.T) {
						r := route(rl.locX+x, rl.locZ-3, 2, int(flag.BlockAccessNorth))
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].Z() == rl.locZ-1 {
							t.Errorf("last waypoint z == %d", r.Waypoints[len(r.Waypoints)-1].Z())
						}
					})

					t.Run("coming from north tiles with access blocked at angle 0", func(t *testing.T) {
						r := route(rl.locX+x, rl.locZ+length+3, 0, int(flag.BlockAccessNorth))
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].Z() == rl.locZ+length {
							t.Errorf("last waypoint z == %d", r.Waypoints[len(r.Waypoints)-1].Z())
						}
					})
					t.Run("coming from north tiles with access blocked at angle 2", func(t *testing.T) {
						r := route(rl.locX+x, rl.locZ+length+3, 2, int(flag.BlockAccessSouth))
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].Z() == rl.locZ+length {
							t.Errorf("last waypoint z == %d", r.Waypoints[len(r.Waypoints)-1].Z())
						}
					})
				})
			}

			for z := range length {
				t.Run(fmt.Sprintf("length %d", z), func(t *testing.T) {
					t.Run("coming from west tiles at angle 0", func(t *testing.T) {
						r := route(rl.locX-3, rl.locZ+z, 0, 0)
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].X() != rl.locX-1 {
							t.Errorf("last waypoint x == %d, want %d", r.Waypoints[len(r.Waypoints)-1].X(), rl.locX-1)
						}
						if r.Waypoints[len(r.Waypoints)-1].Z() != rl.locZ+z {
							t.Errorf("last waypoint z == %d, want %d", r.Waypoints[len(r.Waypoints)-1].Z(), rl.locZ+z)
						}
					})
					t.Run("coming from west tiles at angle 2", func(t *testing.T) {
						r := route(rl.locX-3, rl.locZ+z, 2, 0)
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].X() != rl.locX-1 {
							t.Errorf("last waypoint x == %d, want %d", r.Waypoints[len(r.Waypoints)-1].X(), rl.locX-1)
						}
						if r.Waypoints[len(r.Waypoints)-1].Z() != rl.locZ+z {
							t.Errorf("last waypoint z == %d, want %d", r.Waypoints[len(r.Waypoints)-1].Z(), rl.locZ+z)
						}
					})

					t.Run("coming from east tiles at angle 0", func(t *testing.T) {
						r := route(rl.locX+width+3, rl.locZ+z, 0, 0)
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].X() != rl.locX+width {
							t.Errorf("last waypoint x == %d, want %d", r.Waypoints[len(r.Waypoints)-1].X(), rl.locX+width)
						}
						if r.Waypoints[len(r.Waypoints)-1].Z() != rl.locZ+z {
							t.Errorf("last waypoint z == %d, want %d", r.Waypoints[len(r.Waypoints)-1].Z(), rl.locZ+z)
						}
					})
					t.Run("coming from east tiles at angle 2", func(t *testing.T) {
						r := route(rl.locX+width+3, rl.locZ+z, 2, 0)
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].X() != rl.locX+width {
							t.Errorf("last waypoint x == %d, want %d", r.Waypoints[len(r.Waypoints)-1].X(), rl.locX+width)
						}
						if r.Waypoints[len(r.Waypoints)-1].Z() != rl.locZ+z {
							t.Errorf("last waypoint z == %d, want %d", r.Waypoints[len(r.Waypoints)-1].Z(), rl.locZ+z)
						}
					})

					t.Run("coming from west tiles with access blocked at angle 0", func(t *testing.T) {
						r := route(rl.locX-3, rl.locZ+z, 0, int(flag.BlockAccessWest))
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].X() == rl.locX-1 {
							t.Errorf("last waypoint x == %d", r.Waypoints[len(r.Waypoints)-1].X())
						}
					})
					t.Run("coming from west tiles with access blocked at angle 2", func(t *testing.T) {
						r := route(rl.locX-3, rl.locZ+z, 2, int(flag.BlockAccessEast))
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].X() == rl.locX-1 {
							t.Errorf("last waypoint x == %d", r.Waypoints[len(r.Waypoints)-1].X())
						}
					})

					t.Run("coming from east tiles with access blocked at angle 0", func(t *testing.T) {
						r := route(rl.locX+width+3, rl.locZ+z, 0, int(flag.BlockAccessEast))
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].X() == rl.locX+width {
							t.Errorf("last waypoint x == %d", r.Waypoints[len(r.Waypoints)-1].X())
						}
					})
					t.Run("coming from east tiles with access blocked at angle 2", func(t *testing.T) {
						r := route(rl.locX+width+3, rl.locZ+z, 2, int(flag.BlockAccessWest))
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].X() == rl.locX+width {
							t.Errorf("last waypoint x == %d", r.Waypoints[len(r.Waypoints)-1].X())
						}
					})
				})
			}
		})
	}
}

// TestRouteFinderValidateRoutePathAgainstLocWithAngle1And3 tests that loc angles are taken
// into account within RouteFinder.FindRoute and do not rely on external modifications.
func TestRouteFinderValidateRoutePathAgainstLocWithAngle1And3(t *testing.T) {
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
		t.Run(fmt.Sprintf("locX %d, locZ %d, width %d, length %d",
			rl.locX, rl.locZ, rl.dimension.width, rl.dimension.length), func(t *testing.T) {
			width := rl.dimension.width
			length := rl.dimension.length
			minCoords := NewRouteCoordinates(rl.locX-16, rl.locZ-16, 0)
			maxCoords := NewRouteCoordinates(rl.locX+16, rl.locZ+16, 0)

			flags := internal.BuildCollisionMap(minCoords.X(), minCoords.Z(), maxCoords.X(), maxCoords.Z())
			internal.Flag(flags, rl.locX, rl.locZ, length, width, collision.FlagLoc)

			routeFinder := NewRouteFinderDefault(flags)
			route := func(srcX, srcZ, angle, blockAccessFlags int) Route {
				return routeFinder.FindRoute(0, srcX, srcZ, rl.locX, rl.locZ,
					1, width, length, angle,
					-2, // use rectangular exclusive strategy
					true, blockAccessFlags, 25, collision.TypeNormal)
			}

			for x := range length {
				t.Run(fmt.Sprintf("length %d", x), func(t *testing.T) {
					t.Run("coming from south tiles at angle 1", func(t *testing.T) {
						r := route(rl.locX+x, rl.locZ-3, 1, 0)
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].X() != rl.locX+x {
							t.Errorf("last waypoint x == %d, want %d", r.Waypoints[len(r.Waypoints)-1].X(), rl.locX+x)
						}
						if r.Waypoints[len(r.Waypoints)-1].Z() != rl.locZ-1 {
							t.Errorf("last waypoint z == %d, want %d", r.Waypoints[len(r.Waypoints)-1].Z(), rl.locZ-1)
						}
					})
					t.Run("coming from south tiles at angle 3", func(t *testing.T) {
						r := route(rl.locX+x, rl.locZ-3, 3, 0)
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].X() != rl.locX+x {
							t.Errorf("last waypoint x == %d, want %d", r.Waypoints[len(r.Waypoints)-1].X(), rl.locX+x)
						}
						if r.Waypoints[len(r.Waypoints)-1].Z() != rl.locZ-1 {
							t.Errorf("last waypoint z == %d, want %d", r.Waypoints[len(r.Waypoints)-1].Z(), rl.locZ-1)
						}
					})

					t.Run("coming from north tiles at angle 1", func(t *testing.T) {
						r := route(rl.locX+x, rl.locZ+width+3, 1, 0)
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].X() != rl.locX+x {
							t.Errorf("last waypoint x == %d, want %d", r.Waypoints[len(r.Waypoints)-1].X(), rl.locX+x)
						}
						if r.Waypoints[len(r.Waypoints)-1].Z() != rl.locZ+width {
							t.Errorf("last waypoint z == %d, want %d", r.Waypoints[len(r.Waypoints)-1].Z(), rl.locZ+width)
						}
					})
					t.Run("coming from north tiles at angle 3", func(t *testing.T) {
						r := route(rl.locX+x, rl.locZ+width+3, 3, 0)
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].X() != rl.locX+x {
							t.Errorf("last waypoint x == %d, want %d", r.Waypoints[len(r.Waypoints)-1].X(), rl.locX+x)
						}
						if r.Waypoints[len(r.Waypoints)-1].Z() != rl.locZ+width {
							t.Errorf("last waypoint z == %d, want %d", r.Waypoints[len(r.Waypoints)-1].Z(), rl.locZ+width)
						}
					})

					t.Run("coming from south tiles with access blocked at angle 1", func(t *testing.T) {
						r := route(rl.locX+x, rl.locZ-3, 1, int(flag.BlockAccessEast))
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].Z() == rl.locZ-1 {
							t.Errorf("last waypoint z == %d", r.Waypoints[len(r.Waypoints)-1].Z())
						}
					})
					t.Run("coming from south tiles with access blocked at angle 3", func(t *testing.T) {
						r := route(rl.locX+x, rl.locZ-3, 3, int(flag.BlockAccessWest))
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].Z() == rl.locZ-1 {
							t.Errorf("last waypoint z == %d", r.Waypoints[len(r.Waypoints)-1].Z())
						}
					})

					t.Run("coming from north tiles with access blocked at angle 1", func(t *testing.T) {
						r := route(rl.locX+x, rl.locZ+width+3, 1, int(flag.BlockAccessWest))
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].Z() == rl.locZ+width {
							t.Errorf("last waypoint z == %d", r.Waypoints[len(r.Waypoints)-1].Z())
						}
					})
					t.Run("coming from north tiles with access blocked at angle 3", func(t *testing.T) {
						r := route(rl.locX+x, rl.locZ+width+3, 3, int(flag.BlockAccessEast))
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].Z() == rl.locZ+width {
							t.Errorf("last waypoint z == %d", r.Waypoints[len(r.Waypoints)-1].Z())
						}
					})
				})
			}

			for z := range width {
				t.Run(fmt.Sprintf("length %d", z), func(t *testing.T) {
					t.Run("coming from west tiles at angle 1", func(t *testing.T) {
						r := route(rl.locX-3, rl.locZ+z, 1, 0)
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].X() != rl.locX-1 {
							t.Errorf("last waypoint x == %d, want %d", r.Waypoints[len(r.Waypoints)-1].X(), rl.locX-1)
						}
						if r.Waypoints[len(r.Waypoints)-1].Z() != rl.locZ+z {
							t.Errorf("last waypoint z == %d, want %d", r.Waypoints[len(r.Waypoints)-1].Z(), rl.locZ+z)
						}
					})
					t.Run("coming from west tiles at angle 3", func(t *testing.T) {
						r := route(rl.locX-3, rl.locZ+z, 3, 0)
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].X() != rl.locX-1 {
							t.Errorf("last waypoint x == %d, want %d", r.Waypoints[len(r.Waypoints)-1].X(), rl.locX-1)
						}
						if r.Waypoints[len(r.Waypoints)-1].Z() != rl.locZ+z {
							t.Errorf("last waypoint z == %d, want %d", r.Waypoints[len(r.Waypoints)-1].Z(), rl.locZ+z)
						}
					})

					t.Run("coming from east tiles at angle 1", func(t *testing.T) {
						r := route(rl.locX+length+3, rl.locZ+z, 1, 0)
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].X() != rl.locX+length {
							t.Errorf("last waypoint x == %d, want %d", r.Waypoints[len(r.Waypoints)-1].X(), rl.locX+length)
						}
						if r.Waypoints[len(r.Waypoints)-1].Z() != rl.locZ+z {
							t.Errorf("last waypoint z == %d, want %d", r.Waypoints[len(r.Waypoints)-1].Z(), rl.locZ+z)
						}
					})
					t.Run("coming from east tiles at angle 3", func(t *testing.T) {
						r := route(rl.locX+length+3, rl.locZ+z, 3, 0)
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].X() != rl.locX+length {
							t.Errorf("last waypoint x == %d, want %d", r.Waypoints[len(r.Waypoints)-1].X(), rl.locX+length)
						}
						if r.Waypoints[len(r.Waypoints)-1].Z() != rl.locZ+z {
							t.Errorf("last waypoint z == %d, want %d", r.Waypoints[len(r.Waypoints)-1].Z(), rl.locZ+z)
						}
					})

					t.Run("coming from west tiles with access blocked at angle 1", func(t *testing.T) {
						r := route(rl.locX-3, rl.locZ+z, 1, int(flag.BlockAccessSouth))
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].X() == rl.locX-1 {
							t.Errorf("last waypoint x == %d", r.Waypoints[len(r.Waypoints)-1].X())
						}
					})
					t.Run("coming from west tiles with access blocked at angle 3", func(t *testing.T) {
						r := route(rl.locX-3, rl.locZ+z, 3, int(flag.BlockAccessNorth))
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].X() == rl.locX-1 {
							t.Errorf("last waypoint x == %d", r.Waypoints[len(r.Waypoints)-1].X())
						}
					})

					t.Run("coming from east tiles with access blocked at angle 1", func(t *testing.T) {
						r := route(rl.locX+length+3, rl.locZ+z, 1, int(flag.BlockAccessNorth))
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].X() == rl.locX+length {
							t.Errorf("last waypoint x == %d", r.Waypoints[len(r.Waypoints)-1].X())
						}
					})
					t.Run("coming from east tiles with access blocked at angle 3", func(t *testing.T) {
						r := route(rl.locX+length+3, rl.locZ+z, 3, int(flag.BlockAccessSouth))
						if !r.Success {
							t.Error("route.Success == false, want true")
						}
						if r.Alternative {
							t.Error("route.Alternative == true, want false")
						}

						if r.Waypoints[len(r.Waypoints)-1].X() == rl.locX+length {
							t.Errorf("last waypoint x == %d", r.Waypoints[len(r.Waypoints)-1].X())
						}
					})
				})
			}
		})
	}
}

func TestRouteFinderSimulation(t *testing.T) {
	routeParameterFiles := []string{
		"testdata/barb-village.json",
		"testdata/gnome-maze.json",
		"testdata/lumbridge.json",
	}

	for _, routeParameterFile := range routeParameterFiles {
		params, err := RouteParameterFromFile(routeParameterFile)
		if err != nil {
			t.Fatal(err)
		}
		m := params.ToCollisionFlags()
		rf := NewRouteFinderDefault(m)
		route := rf.FindRouteDefault(params.Level, params.SrcX, params.SrcZ, params.DestX, params.DestZ)

		if route.Waypoints[len(route.Waypoints)-1].X() != params.ExpectedX {
			t.Errorf("[%s] last waypoint x == %d, expected %d", routeParameterFile, route.Waypoints[len(route.Waypoints)-1].X(), params.ExpectedX)
		}
		if route.Waypoints[len(route.Waypoints)-1].Z() != params.ExpectedZ {
			t.Errorf("[%s] last waypoint z == %d, expected %d", routeParameterFile, route.Waypoints[len(route.Waypoints)-1].Z(), params.ExpectedZ)
		}
	}
}

type RouteParameter struct {
	Level     int   `json:"level"`
	SrcX      int   `json:"srcX"`
	SrcZ      int   `json:"srcZ"`
	DestX     int   `json:"destX"`
	DestZ     int   `json:"destZ"`
	ExpectedX int   `json:"expectedX"`
	ExpectedZ int   `json:"expectedZ"`
	Flags     []int `json:"flags"`
}

func (rp RouteParameter) ToCollisionFlags() collision.FlagMap {
	collisionFlags := collision.NewFlagMap()
	mapSearchSize := int(math.Sqrt(float64(len(rp.Flags))))
	half := mapSearchSize / 2
	centerX := rp.SrcX
	centerZ := rp.SrcZ

	for z := centerZ - half; z < centerZ+half; z++ {
		for x := centerX - half; x < centerX+half; x++ {
			lx := x - (centerX - half)
			lz := z - (centerZ - half)
			index := (lz * mapSearchSize) + lx
			collisionFlags.Set(x, z, rp.Level, rp.Flags[index])
		}
	}

	return collisionFlags
}

func RouteParameterFromFile(path string) (RouteParameter, error) {
	var rp RouteParameter
	data, err := os.ReadFile(path)
	if err != nil {
		return RouteParameter{}, err
	}
	err = json.Unmarshal(data, &rp)
	if err != nil {
		return RouteParameter{}, err
	}

	return rp, nil
}
