package routefinder

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/pathfinder/flag"
	"github.com/zsrv/goscape/pkg/pathfinder/reach"
	"github.com/zsrv/goscape/pkg/pathfinder/rotation"
)

const (
	routefinderDefaultSearchMapSize                       = 128
	routefinderDefaultRingBufferSize                      = 4096
	routefinderDefaultDistanceValue                       = 99_999_999
	routefinderDefaultSrcDirectionValue                   = 99
	routefinderMaxAlternativeRouteLowestCost              = 1000
	routefinderMaxAlternativeRouteSeekRange               = 100
	routefinderMaxAlternativeRouteDistanceFromDestination = 10
)

type RouteFinder struct {
	flags                collision.FlagMap
	searchMapSize        int
	ringBufferSize       int
	useRouteBlockerFlags bool

	directions  []int
	distances   []int
	validLocalX []int
	validLocalZ []int

	currLocalX     int
	currLocalZ     int
	bufReaderIndex int
	bufWriterIndex int
}

func NewRouteFinder(flags collision.FlagMap, searchMapSize int, ringBufferSize int, useRouteBlockerFlags bool) RouteFinder {
	pf := RouteFinder{
		flags:                flags,
		searchMapSize:        searchMapSize,
		ringBufferSize:       ringBufferSize,
		useRouteBlockerFlags: useRouteBlockerFlags,

		directions:  make([]int, searchMapSize*searchMapSize),
		distances:   make([]int, searchMapSize*searchMapSize),
		validLocalX: make([]int, ringBufferSize),
		validLocalZ: make([]int, ringBufferSize),
	}

	for i := range pf.distances {
		pf.distances[i] = routefinderDefaultDistanceValue
	}

	return pf
}

func NewRouteFinderDefault(flags collision.FlagMap) RouteFinder {
	return NewRouteFinder(flags, routefinderDefaultSearchMapSize, routefinderDefaultRingBufferSize, false)
}

// FindRoute creates a validated Route from (srcX, srcZ) to (destX, destZ) on height level,
// avoiding obstacles in the appropriate manner respective to the given collision type.
//
// TODO: returns/throws
//
// destWidth is the absolute width of the destination. This value should not be changed when passing
// the width of a rotated loc (it is done within the function).
//
// destLength is the absolute length of the destination. Similar to destWidth, this value should not
// be changed or altered for rotated locs.
//
// locAngle is the angle of the target loc being used as the destination. If the path is meant for
// something that is not a loc, this value should be passed as the default 0.
//
// locShape is the shape of the target loc being used as the destination. If the path is meant for
// something that is not a loc, this value should be passed as the default 0.
//
// blockAccessFlags are packed directional bitflags that should be blocked off when a "reach strategy"
// (aka exit strategy) is checked. This can be seen in locs such as staircases, where all directions
// excluding the direction with access to the steps are "blocked" (see flag.BlockAccess).
func (pf *RouteFinder) FindRoute(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, locAngle, locShape int,
	moveNear bool, blockAccessFlags int, maxWaypoints int, collisionType collision.Type) Route {
	// DEFAULTS: srcSize destWidth destLength = 1
	// locShape = -1
	// moveNear = true
	// maxWaypoints = 25
	// collisionType = collision.TypeNormal
	// TODO: this appears to be a top-level function. recover from panics here and have this func return (Route, error)?
	if !(srcX >= 0 && srcX <= 0x7FFF && srcZ >= 0 && srcZ <= 0x7FFF) {
		panic(fmt.Sprintf("srcX (%d) and srcZ (%d) must both be in range [0, 0x7FFF]", srcX, srcZ))
	}
	if !(destX >= 0 && destX <= 0x7FFF && destZ >= 0 && destZ <= 0x7FFF) {
		panic(fmt.Sprintf("destX (%d) and destZ (%d) must both be in range [0, 0x7FFF]", destX, destZ))
	}
	if !(level >= 0 && level <= 0x3) {
		panic(fmt.Sprintf("level (%d) must be in range [0, 3]", level))
	}
	pf.reset()
	baseX := srcX - (pf.searchMapSize / 2)
	baseZ := srcZ - (pf.searchMapSize / 2)
	localSrcX := srcX - baseX
	localSrcZ := srcZ - baseZ
	localDestX := destX - baseX
	localDestZ := destZ - baseZ
	pf.appendDirection(localSrcX, localSrcZ, routefinderDefaultSrcDirectionValue, 0)

	var pathFound bool
	if pf.useRouteBlockerFlags {
		if srcSize == 1 {
			pathFound = pf.routeBlockerFindSize1(baseX, baseZ, level, localDestX, localDestZ, destWidth, destLength, srcSize, locAngle, locShape, blockAccessFlags, collisionType)
		} else if srcSize == 2 {
			pathFound = pf.routeBlockerFindSize2(baseX, baseZ, level, localDestX, localDestZ, destWidth, destLength, srcSize, locAngle, locShape, blockAccessFlags, collisionType)
		} else {
			pathFound = pf.routeBlockerFindBig(baseX, baseZ, level, localDestX, localDestZ, destWidth, destLength, srcSize, locAngle, locShape, blockAccessFlags, collisionType)
		}
	} else {
		if srcSize == 1 {
			pathFound = pf.routeFindSize1(baseX, baseZ, level, localDestX, localDestZ, destWidth, destLength, srcSize, locAngle, locShape, blockAccessFlags, collisionType)
		} else if srcSize == 2 {
			pathFound = pf.routeFindSize2(baseX, baseZ, level, localDestX, localDestZ, destWidth, destLength, srcSize, locAngle, locShape, blockAccessFlags, collisionType)
		} else {
			pathFound = pf.routeFindBig(baseX, baseZ, level, localDestX, localDestZ, destWidth, destLength, srcSize, locAngle, locShape, blockAccessFlags, collisionType)
		}
	}
	if !pathFound {
		if !moveNear {
			return Route{}
		}
		foundApproachPoint := pf.findClosestApproachPoint(localDestX, localDestZ, rotation.Rotate(locAngle, destWidth, destLength), rotation.Rotate(locAngle, destLength, destWidth))
		if !foundApproachPoint {
			return Route{}
		}
	}
	var waypoints []RouteCoordinates
	nextDir := pf.directions[pf.localIndex(pf.currLocalX, pf.currLocalZ)]
	currDir := -1

	for index := 0; index < len(pf.directions); index++ {
		if pf.currLocalX == localSrcX && pf.currLocalZ == localSrcZ {
			break
		}
		if currDir != nextDir {
			currDir = nextDir
			if len(waypoints) >= maxWaypoints {
				waypoints = waypoints[:len(waypoints)-1]
			}
			coords := NewRouteCoordinates(baseX+pf.currLocalX, baseZ+pf.currLocalZ, level)
			waypoints = append([]RouteCoordinates{coords}, waypoints...)
		}
		if currDir&flag.DirectionEast != 0 {
			pf.currLocalX++
		} else if currDir&flag.DirectionWest != 0 {
			pf.currLocalX--
		}
		if currDir&flag.DirectionNorth != 0 {
			pf.currLocalZ++
		} else if currDir&flag.DirectionSouth != 0 {
			pf.currLocalZ--
		}
		nextDir = pf.directions[pf.localIndex(pf.currLocalX, pf.currLocalZ)]
	}
	return Route{
		Waypoints:   waypoints,
		Alternative: !pathFound,
		Success:     true,
	}
}

func (pf *RouteFinder) FindRouteDefault(level, srcX, srcZ, destX, destZ int) Route {
	return pf.FindRoute(level, srcX, srcZ, destX, destZ, 1, 1, 1, 0, -1, true, 0, 25, collision.TypeNormal)
}

func (pf *RouteFinder) routeFindSize1(baseX, baseZ, level, localDestX, localDestZ, destWidth, destLength, srcSize, locAngle, locShape, blockAccessFlags int, collisionType collision.Type) bool {
	var x int
	var z int
	var clipFlag int
	var dirFlag int
	relativeSearchSize := pf.searchMapSize - 1

	for pf.bufWriterIndex != pf.bufReaderIndex {
		pf.currLocalX = pf.validLocalX[pf.bufReaderIndex]
		pf.currLocalZ = pf.validLocalZ[pf.bufReaderIndex]
		pf.bufReaderIndex = (pf.bufReaderIndex + 1) & (pf.ringBufferSize - 1)

		reached := reach.Reached(pf.flags, level, pf.currLocalX+baseX, pf.currLocalZ+baseZ, localDestX+baseX, localDestZ+baseZ, destWidth, destLength, srcSize, locAngle, locShape, blockAccessFlags)
		if reached {
			return true
		}

		nextDistance := pf.distances[pf.localIndex(pf.currLocalX, pf.currLocalZ)] + 1

		// east to west
		x = pf.currLocalX - 1
		z = pf.currLocalZ
		clipFlag = collision.FlagBlockWest
		dirFlag = flag.DirectionEast
		if pf.currLocalX > 0 && pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), clipFlag, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}

		// west to east
		x = pf.currLocalX + 1
		z = pf.currLocalZ
		clipFlag = collision.FlagBlockEast
		dirFlag = flag.DirectionWest
		if pf.currLocalX < relativeSearchSize && pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), clipFlag, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}

		// north to south
		x = pf.currLocalX
		z = pf.currLocalZ - 1
		clipFlag = collision.FlagBlockSouth
		dirFlag = flag.DirectionNorth
		if pf.currLocalZ > 0 && pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), clipFlag, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}

		// south to north
		x = pf.currLocalX
		z = pf.currLocalZ + 1
		clipFlag = collision.FlagBlockNorth
		dirFlag = flag.DirectionSouth
		if pf.currLocalZ < relativeSearchSize && pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), clipFlag, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}

		// northeast to southwest
		x = pf.currLocalX - 1
		z = pf.currLocalZ - 1
		dirFlag = flag.DirectionNortheast
		if pf.currLocalX > 0 &&
			pf.currLocalZ > 0 &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), collision.FlagBlockSouthWest, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ, level), collision.FlagBlockWest, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX, z, level), collision.FlagBlockSouth, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}

		// northwest to southeast
		x = pf.currLocalX + 1
		z = pf.currLocalZ - 1
		dirFlag = flag.DirectionNorthwest
		if pf.currLocalX < relativeSearchSize &&
			pf.currLocalZ > 0 &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), collision.FlagBlockSouthEast, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ, level), collision.FlagBlockEast, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX, z, level), collision.FlagBlockSouth, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}

		// southeast to northwest
		x = pf.currLocalX - 1
		z = pf.currLocalZ + 1
		dirFlag = flag.DirectionSoutheast
		if pf.currLocalX > 0 &&
			pf.currLocalZ < relativeSearchSize &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), collision.FlagBlockNorthWest, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ, level), collision.FlagBlockWest, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX, z, level), collision.FlagBlockNorth, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}

		// southwest to northeast
		x = pf.currLocalX + 1
		z = pf.currLocalZ + 1
		dirFlag = flag.DirectionSouthwest
		if pf.currLocalX < relativeSearchSize &&
			pf.currLocalZ < relativeSearchSize &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), collision.FlagBlockNorthEast, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ, level), collision.FlagBlockEast, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX, z, level), collision.FlagBlockNorth, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}
	}
	return false
}

func (pf *RouteFinder) routeFindSize2(baseX, baseZ, level, localDestX, localDestZ, destWidth, destLength, srcSize, locAngle, locShape, blockAccessFlags int, collisionType collision.Type) bool {
	var x int
	var z int
	var dirFlag int
	relativeSearchSize := pf.searchMapSize - 2

	for pf.bufWriterIndex != pf.bufReaderIndex {
		pf.currLocalX = pf.validLocalX[pf.bufReaderIndex]
		pf.currLocalZ = pf.validLocalZ[pf.bufReaderIndex]
		pf.bufReaderIndex = (pf.bufReaderIndex + 1) & (pf.ringBufferSize - 1)

		reached := reach.Reached(pf.flags, level, pf.currLocalX+baseX, pf.currLocalZ+baseZ, localDestX+baseX, localDestZ+baseZ, destWidth, destLength, srcSize, locAngle, locShape, blockAccessFlags)
		if reached {
			return true
		}

		nextDistance := pf.distances[pf.localIndex(pf.currLocalX, pf.currLocalZ)] + 1

		// east to west
		x = pf.currLocalX - 1
		z = pf.currLocalZ
		dirFlag = flag.DirectionEast
		if pf.currLocalX > 0 &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), collision.FlagBlockSouthWest, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ+1, level), collision.FlagBlockNorthWest, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}

		// west to east
		x = pf.currLocalX + 1
		z = pf.currLocalZ
		dirFlag = flag.DirectionWest
		if pf.currLocalX < relativeSearchSize &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+2, z, level), collision.FlagBlockSouthEast, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+2, pf.currLocalZ+1, level), collision.FlagBlockNorthEast, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}

		// north to south
		x = pf.currLocalX
		z = pf.currLocalZ - 1
		dirFlag = flag.DirectionNorth
		if pf.currLocalZ > 0 &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), collision.FlagBlockSouthWest, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+1, z, level), collision.FlagBlockSouthEast, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}

		// south to north
		x = pf.currLocalX
		z = pf.currLocalZ + 1
		dirFlag = flag.DirectionSouth
		if pf.currLocalZ < relativeSearchSize &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ+2, level), collision.FlagBlockNorthWest, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+1, pf.currLocalZ+2, level), collision.FlagBlockNorthEast, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}

		// northeast to southwest
		x = pf.currLocalX - 1
		z = pf.currLocalZ - 1
		dirFlag = flag.DirectionNortheast
		if pf.currLocalX > 0 &&
			pf.currLocalZ > 0 &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ, level), collision.FlagBlockNorthAndSouthEast, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), collision.FlagBlockSouthWest, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX, z, level), collision.FlagBlockNorthEastAndWest, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}

		// northwest to southeast
		x = pf.currLocalX + 1
		z = pf.currLocalZ - 1
		dirFlag = flag.DirectionNorthwest
		if pf.currLocalX < relativeSearchSize &&
			pf.currLocalZ > 0 &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), collision.FlagBlockNorthEastAndWest, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+2, z, level), collision.FlagBlockSouthEast, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+2, pf.currLocalZ, level), collision.FlagBlockNorthAndSouthWest, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}

		// southeast to northwest
		x = pf.currLocalX - 1
		z = pf.currLocalZ + 1
		dirFlag = flag.DirectionSoutheast
		if pf.currLocalX > 0 &&
			pf.currLocalZ < relativeSearchSize &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), collision.FlagBlockNorthAndSouthEast, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ+2, level), collision.FlagBlockNorthWest, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX, pf.currLocalZ+2, level), collision.FlagBlockSouthEastAndWest, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}

		// southwest to northeast
		x = pf.currLocalX + 1
		z = pf.currLocalZ + 1
		dirFlag = flag.DirectionSouthwest
		if pf.currLocalX < relativeSearchSize &&
			pf.currLocalZ < relativeSearchSize &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ+2, level), collision.FlagBlockSouthEastAndWest, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+2, pf.currLocalZ+2, level), collision.FlagBlockNorthEast, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+2, z, level), collision.FlagBlockNorthAndSouthWest, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}
	}
	return false
}

func (pf *RouteFinder) routeFindBig(baseX, baseZ, level, localDestX, localDestZ, destWidth, destLength, srcSize, locAngle, locShape, blockAccessFlags int, collisionType collision.Type) bool {
	var x int
	var z int
	var dirFlag int
	relativeSearchSize := pf.searchMapSize - srcSize

	for pf.bufWriterIndex != pf.bufReaderIndex {
		pf.currLocalX = pf.validLocalX[pf.bufReaderIndex]
		pf.currLocalZ = pf.validLocalZ[pf.bufReaderIndex]
		pf.bufReaderIndex = (pf.bufReaderIndex + 1) & (pf.ringBufferSize - 1)

		reached := reach.Reached(pf.flags, level, pf.currLocalX+baseX, pf.currLocalZ+baseZ, localDestX+baseX, localDestZ+baseZ, destWidth, destLength, srcSize, locAngle, locShape, blockAccessFlags)
		if reached {
			return true
		}

		nextDistance := pf.distances[pf.localIndex(pf.currLocalX, pf.currLocalZ)] + 1

		// east to west
		x = pf.currLocalX - 1
		z = pf.currLocalZ
		dirFlag = flag.DirectionEast
		if pf.currLocalX > 0 &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), collision.FlagBlockSouthWest, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ+srcSize-1, level), collision.FlagBlockNorthWest, collisionType) {
			clipFlag := collision.FlagBlockNorthAndSouthEast
			blocked := false
			for index := 1; index < srcSize-1; index++ {
				if !collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ+index, level), clipFlag, collisionType) {
					blocked = true
					break
				}
			}
			if !blocked {
				pf.appendDirection(x, z, dirFlag, nextDistance)
			}
		}

		// west to east
		x = pf.currLocalX + 1
		z = pf.currLocalZ
		dirFlag = flag.DirectionWest
		if pf.currLocalX < relativeSearchSize &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+srcSize, z, level), collision.FlagBlockSouthEast, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+srcSize, pf.currLocalZ+srcSize-1, level), collision.FlagBlockNorthEast, collisionType) {
			clipFlag := collision.FlagBlockNorthAndSouthWest
			blocked := false
			for index := 1; index < srcSize-1; index++ {
				if !collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+srcSize, pf.currLocalZ+index, level), clipFlag, collisionType) {
					blocked = true
					break
				}
			}
			if !blocked {
				pf.appendDirection(x, z, dirFlag, nextDistance)
			}
		}

		// north to south
		x = pf.currLocalX
		z = pf.currLocalZ - 1
		dirFlag = flag.DirectionNorth
		if pf.currLocalZ > 0 &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), collision.FlagBlockSouthWest, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+srcSize-1, z, level), collision.FlagBlockSouthEast, collisionType) {
			clipFlag := collision.FlagBlockNorthEastAndWest
			blocked := false
			for index := 1; index < srcSize-1; index++ {
				if !collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+index, z, level), clipFlag, collisionType) {
					blocked = true
					break
				}
			}
			if !blocked {
				pf.appendDirection(x, z, dirFlag, nextDistance)
			}
		}

		// south to north
		x = pf.currLocalX
		z = pf.currLocalZ + 1
		dirFlag = flag.DirectionSouth
		if pf.currLocalZ < relativeSearchSize &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ+srcSize, level), collision.FlagBlockNorthWest, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+srcSize-1, pf.currLocalZ+srcSize, level), collision.FlagBlockNorthEast, collisionType) {
			clipFlag := collision.FlagBlockSouthEastAndWest
			blocked := false
			for index := 1; index < srcSize+1; index++ {
				if !collision.CanMove(pf.collisionFlag(baseX, baseZ, x+index, pf.currLocalZ+srcSize, level), clipFlag, collisionType) {
					blocked = true
					break
				}
			}
			if !blocked {
				pf.appendDirection(x, z, dirFlag, nextDistance)
			}
		}

		// northeast to southwest
		x = pf.currLocalX - 1
		z = pf.currLocalZ - 1
		dirFlag = flag.DirectionNortheast
		if pf.currLocalX > 0 &&
			pf.currLocalZ > 0 &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), collision.FlagBlockSouthWest, collisionType) {
			clipFlag1 := collision.FlagBlockNorthAndSouthEast
			clipFlag2 := collision.FlagBlockNorthEastAndWest
			blocked := false
			for index := 1; index < srcSize; index++ {
				if !collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ+index-1, level), clipFlag1, collisionType) ||
					!collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+index-1, z, level), clipFlag2, collisionType) {
					blocked = true
					break
				}
			}
			if !blocked {
				pf.appendDirection(x, z, dirFlag, nextDistance)
			}
		}

		// northwest to southeast
		x = pf.currLocalX + 1
		z = pf.currLocalZ - 1
		dirFlag = flag.DirectionNorthwest
		if pf.currLocalX < relativeSearchSize &&
			pf.currLocalZ > 0 &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+srcSize, z, level), collision.FlagBlockSouthEast, collisionType) {
			clipFlag1 := collision.FlagBlockNorthAndSouthWest
			clipFlag2 := collision.FlagBlockNorthEastAndWest
			blocked := false
			for index := 1; index < srcSize; index++ {
				if !collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+srcSize, pf.currLocalZ+index-1, level), clipFlag1, collisionType) ||
					!collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+index, z, level), clipFlag2, collisionType) {
					blocked = true
					break
				}
			}
			if !blocked {
				pf.appendDirection(x, z, dirFlag, nextDistance)
			}
		}

		// southeast to northwest
		x = pf.currLocalX - 1
		z = pf.currLocalZ + 1
		dirFlag = flag.DirectionSoutheast
		if pf.currLocalX > 0 &&
			pf.currLocalZ < relativeSearchSize &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ+srcSize, level), collision.FlagBlockNorthWest, collisionType) {
			clipFlag1 := collision.FlagBlockNorthAndSouthEast
			clipFlag2 := collision.FlagBlockSouthEastAndWest
			blocked := false
			for index := 1; index < srcSize; index++ {
				if !collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ+index, level), clipFlag1, collisionType) ||
					!collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+index-1, pf.currLocalZ+srcSize, level), clipFlag2, collisionType) {
					blocked = true
					break
				}
			}
			if !blocked {
				pf.appendDirection(x, z, dirFlag, nextDistance)
			}
		}

		// southwest to northeast
		x = pf.currLocalX + 1
		z = pf.currLocalZ + 1
		dirFlag = flag.DirectionSouthwest
		if pf.currLocalX < relativeSearchSize &&
			pf.currLocalZ < relativeSearchSize &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+srcSize, pf.currLocalZ+srcSize, level), collision.FlagBlockNorthEast, collisionType) {
			clipFlag1 := collision.FlagBlockSouthEastAndWest
			clipFlag2 := collision.FlagBlockNorthAndSouthWest
			blocked := false
			for index := 1; index < srcSize; index++ {
				if !collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+index, pf.currLocalZ+srcSize, level), clipFlag1, collisionType) ||
					!collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+srcSize, pf.currLocalZ+index, level), clipFlag2, collisionType) {
					blocked = true
					break
				}
			}
			if !blocked {
				pf.appendDirection(x, z, dirFlag, nextDistance)
			}
		}
	}
	return false
}

func (pf *RouteFinder) routeBlockerFindSize1(baseX, baseZ, level, localDestX, localDestZ, destWidth, destLength, srcSize, locAngle, locShape, blockAccessFlags int, collisionType collision.Type) bool {
	var x int
	var z int
	var clipFlag int
	var dirFlag int
	relativeSearchSize := pf.searchMapSize - 1

	for pf.bufWriterIndex != pf.bufReaderIndex {
		pf.currLocalX = pf.validLocalX[pf.bufReaderIndex]
		pf.currLocalZ = pf.validLocalZ[pf.bufReaderIndex]
		pf.bufReaderIndex = (pf.bufReaderIndex + 1) & (pf.ringBufferSize - 1)

		reached := reach.Reached(pf.flags, level, pf.currLocalX+baseX, pf.currLocalZ+baseZ, localDestX+baseX, localDestZ+baseZ, destWidth, destLength, srcSize, locAngle, locShape, blockAccessFlags)
		if reached {
			return true
		}

		nextDistance := pf.distances[pf.localIndex(pf.currLocalX, pf.currLocalZ)] + 1

		// east to west
		x = pf.currLocalX - 1
		z = pf.currLocalZ
		clipFlag = collision.FlagBlockWestRouteBlocker
		dirFlag = flag.DirectionEast
		if pf.currLocalX > 0 && pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), clipFlag, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}

		// west to east
		x = pf.currLocalX + 1
		z = pf.currLocalZ
		clipFlag = collision.FlagBlockEastRouteBlocker
		dirFlag = flag.DirectionWest
		if pf.currLocalX < relativeSearchSize && pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), clipFlag, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}

		// north to south
		x = pf.currLocalX
		z = pf.currLocalZ - 1
		clipFlag = collision.FlagBlockSouthRouteBlocker
		dirFlag = flag.DirectionNorth
		if pf.currLocalZ > 0 && pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), clipFlag, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}

		// south to north
		x = pf.currLocalX
		z = pf.currLocalZ + 1
		clipFlag = collision.FlagBlockNorthRouteBlocker
		dirFlag = flag.DirectionSouth
		if pf.currLocalZ < relativeSearchSize && pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), clipFlag, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}

		// northeast to southwest
		x = pf.currLocalX - 1
		z = pf.currLocalZ - 1
		dirFlag = flag.DirectionNortheast
		if pf.currLocalX > 0 &&
			pf.currLocalZ > 0 &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), collision.FlagBlockSouthWestRouteBlocker, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ, level), collision.FlagBlockWestRouteBlocker, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX, z, level), collision.FlagBlockSouthRouteBlocker, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}

		// northwest to southeast
		x = pf.currLocalX + 1
		z = pf.currLocalZ - 1
		dirFlag = flag.DirectionNorthwest
		if pf.currLocalX < relativeSearchSize &&
			pf.currLocalZ > 0 &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), collision.FlagBlockSouthEastRouteBlocker, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ, level), collision.FlagBlockEastRouteBlocker, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX, z, level), collision.FlagBlockSouthRouteBlocker, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}

		// southeast to northwest
		x = pf.currLocalX - 1
		z = pf.currLocalZ + 1
		dirFlag = flag.DirectionSoutheast
		if pf.currLocalX > 0 &&
			pf.currLocalZ < relativeSearchSize &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), collision.FlagBlockNorthWestRouteBlocker, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ, level), collision.FlagBlockWestRouteBlocker, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX, z, level), collision.FlagBlockNorthRouteBlocker, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}

		// southwest to northeast
		x = pf.currLocalX + 1
		z = pf.currLocalZ + 1
		dirFlag = flag.DirectionSouthwest
		if pf.currLocalX < relativeSearchSize &&
			pf.currLocalZ < relativeSearchSize &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), collision.FlagBlockNorthEastRouteBlocker, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ, level), collision.FlagBlockEastRouteBlocker, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX, z, level), collision.FlagBlockNorthRouteBlocker, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}
	}
	return false
}

func (pf *RouteFinder) routeBlockerFindSize2(baseX, baseZ, level, localDestX, localDestZ, destWidth, destLength, srcSize, locAngle, locShape, blockAccessFlags int, collisionType collision.Type) bool {
	var x int
	var z int
	var dirFlag int
	relativeSearchSize := pf.searchMapSize - 2

	for pf.bufWriterIndex != pf.bufReaderIndex {
		pf.currLocalX = pf.validLocalX[pf.bufReaderIndex]
		pf.currLocalZ = pf.validLocalZ[pf.bufReaderIndex]
		pf.bufReaderIndex = (pf.bufReaderIndex + 1) & (pf.ringBufferSize - 1)

		reached := reach.Reached(pf.flags, level, pf.currLocalX+baseX, pf.currLocalZ+baseZ, localDestX+baseX, localDestZ+baseZ, destWidth, destLength, srcSize, locAngle, locShape, blockAccessFlags)
		if reached {
			return true
		}

		nextDistance := pf.distances[pf.localIndex(pf.currLocalX, pf.currLocalZ)] + 1

		// east to west
		x = pf.currLocalX - 1
		z = pf.currLocalZ
		dirFlag = flag.DirectionEast
		if pf.currLocalX > 0 &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), collision.FlagBlockSouthWestRouteBlocker, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ+1, level), collision.FlagBlockNorthWestRouteBlocker, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}

		// west to east
		x = pf.currLocalX + 1
		z = pf.currLocalZ
		dirFlag = flag.DirectionWest
		if pf.currLocalX < relativeSearchSize &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+2, z, level), collision.FlagBlockSouthEastRouteBlocker, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+2, pf.currLocalZ+1, level), collision.FlagBlockNorthEastRouteBlocker, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}

		// north to south
		x = pf.currLocalX
		z = pf.currLocalZ - 1
		dirFlag = flag.DirectionNorth
		if pf.currLocalZ > 0 &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), collision.FlagBlockSouthWestRouteBlocker, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+1, z, level), collision.FlagBlockSouthEastRouteBlocker, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}

		// south to north
		x = pf.currLocalX
		z = pf.currLocalZ + 1
		dirFlag = flag.DirectionSouth
		if pf.currLocalZ < relativeSearchSize &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ+2, level), collision.FlagBlockNorthWestRouteBlocker, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+1, pf.currLocalZ+2, level), collision.FlagBlockNorthEastRouteBlocker, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}

		// northeast to southwest
		x = pf.currLocalX - 1
		z = pf.currLocalZ - 1
		dirFlag = flag.DirectionNortheast
		if pf.currLocalX > 0 &&
			pf.currLocalZ > 0 &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ, level), collision.FlagBlockNorthAndSouthEastRouteBlocker, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), collision.FlagBlockSouthWestRouteBlocker, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX, z, level), collision.FlagBlockNorthEastAndWestRouteBlocker, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}

		// northwest to southeast
		x = pf.currLocalX + 1
		z = pf.currLocalZ - 1
		dirFlag = flag.DirectionNorthwest
		if pf.currLocalX < relativeSearchSize &&
			pf.currLocalZ > 0 &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), collision.FlagBlockNorthEastAndWestRouteBlocker, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+2, z, level), collision.FlagBlockSouthEastRouteBlocker, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+2, pf.currLocalZ, level), collision.FlagBlockNorthAndSouthWestRouteBlocker, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}

		// southeast to northwest
		x = pf.currLocalX - 1
		z = pf.currLocalZ + 1
		dirFlag = flag.DirectionSoutheast
		if pf.currLocalX > 0 &&
			pf.currLocalZ < relativeSearchSize &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), collision.FlagBlockNorthAndSouthEastRouteBlocker, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ+2, level), collision.FlagBlockNorthWestRouteBlocker, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX, pf.currLocalZ+2, level), collision.FlagBlockSouthEastAndWestRouteBlocker, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}

		// southwest to northeast
		x = pf.currLocalX + 1
		z = pf.currLocalZ + 1
		dirFlag = flag.DirectionSouthwest
		if pf.currLocalX < relativeSearchSize &&
			pf.currLocalZ < relativeSearchSize &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ+2, level), collision.FlagBlockSouthEastAndWestRouteBlocker, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+2, pf.currLocalZ+2, level), collision.FlagBlockNorthEastRouteBlocker, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+2, z, level), collision.FlagBlockNorthAndSouthWestRouteBlocker, collisionType) {
			pf.appendDirection(x, z, dirFlag, nextDistance)
		}
	}
	return false
}

func (pf *RouteFinder) routeBlockerFindBig(baseX, baseZ, level, localDestX, localDestZ, destWidth, destLength, srcSize, locAngle, locShape, blockAccessFlags int, collisionType collision.Type) bool {
	var x int
	var z int
	var dirFlag int
	relativeSearchSize := pf.searchMapSize - srcSize

	for pf.bufWriterIndex != pf.bufReaderIndex {
		pf.currLocalX = pf.validLocalX[pf.bufReaderIndex]
		pf.currLocalZ = pf.validLocalZ[pf.bufReaderIndex]
		pf.bufReaderIndex = (pf.bufReaderIndex + 1) & (pf.ringBufferSize - 1)

		reached := reach.Reached(pf.flags, level, pf.currLocalX+baseX, pf.currLocalZ+baseZ, localDestX+baseX, localDestZ+baseZ, destWidth, destLength, srcSize, locAngle, locShape, blockAccessFlags)
		if reached {
			return true
		}

		nextDistance := pf.distances[pf.localIndex(pf.currLocalX, pf.currLocalZ)] + 1

		// east to west
		x = pf.currLocalX - 1
		z = pf.currLocalZ
		dirFlag = flag.DirectionEast
		if pf.currLocalX > 0 &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), collision.FlagBlockSouthWestRouteBlocker, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ+srcSize-1, level), collision.FlagBlockNorthWestRouteBlocker, collisionType) {
			clipFlag := collision.FlagBlockNorthAndSouthEastRouteBlocker
			blocked := false
			for index := 1; index < srcSize-1; index++ {
				if !collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ+index, level), clipFlag, collisionType) {
					blocked = true
					break
				}
			}
			if !blocked {
				pf.appendDirection(x, z, dirFlag, nextDistance)
			}
		}

		// west to east
		x = pf.currLocalX + 1
		z = pf.currLocalZ
		dirFlag = flag.DirectionWest
		if pf.currLocalX < relativeSearchSize &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+srcSize, z, level), collision.FlagBlockSouthEastRouteBlocker, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+srcSize, pf.currLocalZ+srcSize-1, level), collision.FlagBlockNorthEastRouteBlocker, collisionType) {
			clipFlag := collision.FlagBlockNorthAndSouthWestRouteBlocker
			blocked := false
			for index := 1; index < srcSize-1; index++ {
				if !collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+srcSize, pf.currLocalZ+index, level), clipFlag, collisionType) {
					blocked = true
					break
				}
			}
			if !blocked {
				pf.appendDirection(x, z, dirFlag, nextDistance)
			}
		}

		// north to south
		x = pf.currLocalX
		z = pf.currLocalZ - 1
		dirFlag = flag.DirectionNorth
		if pf.currLocalZ > 0 &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), collision.FlagBlockSouthWestRouteBlocker, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+srcSize-1, z, level), collision.FlagBlockSouthEastRouteBlocker, collisionType) {
			clipFlag := collision.FlagBlockNorthEastAndWestRouteBlocker
			blocked := false
			for index := 1; index < srcSize-1; index++ {
				if !collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+index, z, level), clipFlag, collisionType) {
					blocked = true
					break
				}
			}
			if !blocked {
				pf.appendDirection(x, z, dirFlag, nextDistance)
			}
		}

		// south to north
		x = pf.currLocalX
		z = pf.currLocalZ + 1
		dirFlag = flag.DirectionSouth
		if pf.currLocalZ < relativeSearchSize &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ+srcSize, level), collision.FlagBlockNorthWestRouteBlocker, collisionType) &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+srcSize-1, pf.currLocalZ+srcSize, level), collision.FlagBlockNorthEastRouteBlocker, collisionType) {
			clipFlag := collision.FlagBlockSouthEastAndWestRouteBlocker
			blocked := false
			for index := 1; index < srcSize+1; index++ {
				if !collision.CanMove(pf.collisionFlag(baseX, baseZ, x+index, pf.currLocalZ+srcSize, level), clipFlag, collisionType) {
					blocked = true
					break
				}
			}
			if !blocked {
				pf.appendDirection(x, z, dirFlag, nextDistance)
			}
		}

		// northeast to southwest
		x = pf.currLocalX - 1
		z = pf.currLocalZ - 1
		dirFlag = flag.DirectionNortheast
		if pf.currLocalX > 0 &&
			pf.currLocalZ > 0 &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, z, level), collision.FlagBlockSouthWestRouteBlocker, collisionType) {
			clipFlag1 := collision.FlagBlockNorthAndSouthEastRouteBlocker
			clipFlag2 := collision.FlagBlockNorthEastAndWestRouteBlocker
			blocked := false
			for index := 1; index < srcSize; index++ {
				if !collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ+index-1, level), clipFlag1, collisionType) ||
					!collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+index-1, z, level), clipFlag2, collisionType) {
					blocked = true
					break
				}
			}
			if !blocked {
				pf.appendDirection(x, z, dirFlag, nextDistance)
			}
		}

		// northwest to southeast
		x = pf.currLocalX + 1
		z = pf.currLocalZ - 1
		dirFlag = flag.DirectionNorthwest
		if pf.currLocalX < relativeSearchSize &&
			pf.currLocalZ > 0 &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+srcSize, z, level), collision.FlagBlockSouthEastRouteBlocker, collisionType) {
			clipFlag1 := collision.FlagBlockNorthAndSouthWestRouteBlocker
			clipFlag2 := collision.FlagBlockNorthEastAndWestRouteBlocker
			blocked := false
			for index := 1; index < srcSize; index++ {
				if !collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+srcSize, pf.currLocalZ+index-1, level), clipFlag1, collisionType) ||
					!collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+index, z, level), clipFlag2, collisionType) {
					blocked = true
					break
				}
			}
			if !blocked {
				pf.appendDirection(x, z, dirFlag, nextDistance)
			}
		}

		// southeast to northwest
		x = pf.currLocalX - 1
		z = pf.currLocalZ + 1
		dirFlag = flag.DirectionSoutheast
		if pf.currLocalX > 0 &&
			pf.currLocalZ < relativeSearchSize &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ+srcSize, level), collision.FlagBlockNorthWestRouteBlocker, collisionType) {
			clipFlag1 := collision.FlagBlockNorthAndSouthEastRouteBlocker
			clipFlag2 := collision.FlagBlockSouthEastAndWestRouteBlocker
			blocked := false
			for index := 1; index < srcSize; index++ {
				if !collision.CanMove(pf.collisionFlag(baseX, baseZ, x, pf.currLocalZ+index, level), clipFlag1, collisionType) ||
					!collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+index-1, pf.currLocalZ+srcSize, level), clipFlag2, collisionType) {
					blocked = true
					break
				}
			}
			if !blocked {
				pf.appendDirection(x, z, dirFlag, nextDistance)
			}
		}

		// southwest to northeast
		x = pf.currLocalX + 1
		z = pf.currLocalZ + 1
		dirFlag = flag.DirectionSouthwest
		if pf.currLocalX < relativeSearchSize &&
			pf.currLocalZ < relativeSearchSize &&
			pf.directions[pf.localIndex(x, z)] == 0 &&
			collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+srcSize, pf.currLocalZ+srcSize, level), collision.FlagBlockNorthEastRouteBlocker, collisionType) {
			clipFlag1 := collision.FlagBlockSouthEastAndWestRouteBlocker
			clipFlag2 := collision.FlagBlockNorthAndSouthWestRouteBlocker
			blocked := false
			for index := 1; index < srcSize; index++ {
				if !collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+index, pf.currLocalZ+srcSize, level), clipFlag1, collisionType) ||
					!collision.CanMove(pf.collisionFlag(baseX, baseZ, pf.currLocalX+srcSize, pf.currLocalZ+index, level), clipFlag2, collisionType) {
					blocked = true
					break
				}
			}
			if !blocked {
				pf.appendDirection(x, z, dirFlag, nextDistance)
			}
		}
	}
	return false
}

func (pf *RouteFinder) findClosestApproachPoint(localDestX, localDestZ, width, length int) bool {
	lowestCost := routefinderMaxAlternativeRouteLowestCost
	maxAlternativePath := routefinderMaxAlternativeRouteSeekRange
	alternativeRouteRange := routefinderMaxAlternativeRouteDistanceFromDestination
	for x := localDestX - alternativeRouteRange; x <= localDestX+alternativeRouteRange; x++ {
		for z := localDestZ - alternativeRouteRange; z <= localDestZ+alternativeRouteRange; z++ {
			if !(x >= 0 && x < pf.searchMapSize) || !(z >= 0 && z < pf.searchMapSize) || pf.distances[pf.localIndex(x, z)] >= routefinderMaxAlternativeRouteSeekRange {
				continue
			}

			dx := 0
			if x < localDestX {
				dx = localDestX - x
			} else if x > localDestX+width-1 {
				dx = x - (width + localDestX - 1)
			}

			dz := 0
			if z < localDestZ {
				dz = localDestZ - z
			} else if z > localDestZ+length-1 {
				dz = z - (length + localDestZ - 1)
			}

			cost := dx*dx + dz*dz
			if cost < lowestCost || (cost == lowestCost && maxAlternativePath > pf.distances[pf.localIndex(x, z)]) {
				pf.currLocalX = x
				pf.currLocalZ = z
				lowestCost = cost
				maxAlternativePath = pf.distances[pf.localIndex(x, z)]
			}
		}
	}
	return lowestCost != routefinderMaxAlternativeRouteLowestCost
}

func (pf *RouteFinder) localIndex(x, z int) int {
	return x*pf.searchMapSize + z
}

func (pf *RouteFinder) collisionFlag(baseX, baseZ, localX, localZ, level int) int {
	return pf.flags.Get(baseX+localX, baseZ+localZ, level)
}

func (pf *RouteFinder) appendDirection(x, z, direction, distance int) {
	index := pf.localIndex(x, z)
	//index := (x * pf.searchMapSize) + z
	pf.directions[index] = direction
	pf.distances[index] = distance
	pf.validLocalX[pf.bufWriterIndex] = x
	pf.validLocalZ[pf.bufWriterIndex] = z
	pf.bufWriterIndex = (pf.bufWriterIndex + 1) & (pf.ringBufferSize - 1)
}

func (pf *RouteFinder) reset() {
	for i := range pf.directions {
		pf.directions[i] = 0
	}
	for i := range pf.distances {
		pf.distances[i] = routefinderDefaultDistanceValue
	}
	pf.bufReaderIndex = 0
	pf.bufWriterIndex = 0
}
