package routefinder

import (
	"fmt"
	"math/rand/v2"

	"github.com/zsrv/goscape/pkg/pathfinder/collision"
)

// https://gist.github.com/Z-Kris/2eb1c2fbc22aa7486a57089c82f293f8
// https://gist.github.com/Z-Kris/fe476d75a51374f12dca999700f009f7

var directions = [][]int{
	{-1, 0}, // West
	{1, 0},  // East
	{0, 1},  // North
	{0, -1}, // South
}

type NaiveRouteFinder struct {
	stepValidator StepValidator
}

func NewNaiveRouteFinder(stepValidator StepValidator) NaiveRouteFinder {
	return NaiveRouteFinder{stepValidator: stepValidator}
}

func (rf NaiveRouteFinder) FindRoute(level, srcX, srcZ, destX, destZ, srcWidth, srcLength, destWidth, destLength, blockAccessFlags int, collision collision.Type) Route {
	if !(srcX >= 0 && srcX <= 0x7FFF && srcZ >= 0 && srcZ <= 0x7FFF) {
		panic(fmt.Sprintf("srcX (%d) and srcZ (%d) must both be in range [0, 0x7FFF]", srcX, srcZ))
	}
	if !(destX >= 0 && destX <= 0x7FFF && destZ >= 0 && destZ <= 0x7FFF) {
		panic(fmt.Sprintf("destX (%d) and destZ (%d) must both be in range [0, 0x7FFF]", destX, destZ))
	}
	if !(level >= 0 && level <= 0x3) {
		panic(fmt.Sprintf("level (%d) must be in range [0, 3]", level))
	}

	// If we are intersecting at all, the path needs to try to move out of the way.
	if rf.intersects(srcX, srcZ, srcWidth, srcLength, destX, destZ, destWidth, destLength) {
		return Route{
			Waypoints:   []RouteCoordinates{rf.cardinalDestination(level, srcX, srcZ)},
			Alternative: false,
			Success:     true,
		}
	}

	dest := rf.NaiveDestination(level, srcX, srcZ, srcWidth, srcLength, destX, destZ, 1, 1)
	dx := dest.Waypoints[0].X()
	dz := dest.Waypoints[0].Z()
	if rf.isDiagonal(dx, dz, srcWidth, srcLength, destX, destZ, destWidth, destLength) {
		return dest
	}
	// If we can interact from this coord(or overlap with the target), allow the routefinder to exit.
	if rf.intersects(dx, dz, srcWidth, srcLength, destX, destZ, destWidth, destLength) {
		return dest
	}
	currX := dx
	currZ := dz
	for currX != destX && currZ != destZ {
		dx := rf.sign(destX - currX)
		dz := rf.sign(destZ - currZ)
		if rf.stepValidator.CanTravel(level, currX, currZ, dx, dz, srcWidth, blockAccessFlags, collision) {
			currX += dx
			currZ += dz
		} else if dx != 0 && rf.stepValidator.CanTravel(level, currX, currZ, dx, 0, srcWidth, blockAccessFlags, collision) {
			currX += dx
		} else if dz != 0 && rf.stepValidator.CanTravel(level, currX, currZ, 0, dz, srcWidth, blockAccessFlags, collision) {
			currZ += dz
		} else {
			// If we can't step anywhere, exit out, we've arrived
			break
		}
	}
	return Route{
		Waypoints:   []RouteCoordinates{NewRouteCoordinates(currX, currZ, level)},
		Alternative: false,
		Success:     true,
	}
}

func (rf NaiveRouteFinder) sign(n int) int {
	if n == 0 {
		return 0
	}
	if n < 0 {
		return -1
	}
	return 1
}

/**
 * Fast way to check if two squares are intersecting.
 * @param srcX The starting SW X.
 * @param srcZ The starting SW Z.
 * @param srcWidth The width on the X axis.
 * @param srcLength The length on the Z axis.
 * @param destX The ending SW X.
 * @param destZ The ending SW Z.
 * @param destWidth The end width on the X axis.
 * @param destLength The end length on the Z axis.
 */
func (rf NaiveRouteFinder) intersects(srcX, srcZ, srcWidth, srcLength, destX, destZ, destWidth, destLength int) bool {
	srcHorizontal := srcX + srcWidth
	srcVertical := srcZ + srcLength
	destHorizontal := destX + destWidth
	destVertical := destZ + destLength

	return !(destX >= srcHorizontal || destHorizontal <= srcX || destZ >= srcVertical || destVertical <= srcZ)
}

func (rf NaiveRouteFinder) isDiagonal(srcX, srcZ, srcWidth, srcLength, destX, destZ, destWidth, destLength int) bool {
	if srcX+srcWidth == destX && srcZ+srcLength == destZ {
		return true
	}
	if srcX-1 == destX+destWidth-1 && srcZ-1 == destZ+destLength-1 {
		return true
	}
	if srcX+srcWidth == destX && srcZ-1 == destZ+destLength-1 {
		return true
	}
	return srcX-1 == destX+destWidth-1 && srcZ+srcLength == destZ
}

func (rf NaiveRouteFinder) cardinalDestination(level, srcX, srcZ int) RouteCoordinates {
	direction := directions[rand.IntN(len(directions))]
	return NewRouteCoordinates(srcX+direction[0], srcZ+direction[1], level)
}

// NaiveDestination calculates coordinates for [sourceX]/[sourceZ] to move to interact with [targetX]/[targetZ]
// We first determine the cardinal direction of the source relative to the target by comparing if
// the source lies to the left or right of diagonal \ and anti-diagonal / lines.
//
// We then further bisect the area into three section relative to the south-west tile (zero):
// 1. Greater than zero: follow their diagonal until the target side is reached (clamped at the furthest most tile)
// 2. Less than zero: zero minus the size of the source
// 3. Equal to zero: move directly towards zero / the south-west coordinate
//
// This method is equivalent to returning the last coordinate in a sequence of steps towards south-west when moving
// ordinal then cardinally until entity side comes into contact with another.
func (rf NaiveRouteFinder) NaiveDestination(level, srcX, srcZ, srcWidth, srcLength, destX, destZ, destWidth, destLength int) Route {
	diagonal := srcX - destX + (srcZ - destZ)
	anti := srcX - destX - (srcZ - destZ)
	southwestClockwise := anti < 0
	northwestClockwise := diagonal >= destLength-1-(srcWidth-1)
	northeastClockwise := anti > srcWidth-srcLength
	southeastClockwise := diagonal <= destWidth-1-(srcLength-1)

	if southwestClockwise && !northwestClockwise {
		// West
		offZ := 0
		if diagonal >= -srcWidth {
			offZ = coerceAtMost(diagonal+srcWidth, destLength-1)
		} else if anti > -srcWidth {
			offZ = -(srcWidth + anti)
		}
		return Route{
			Waypoints:   []RouteCoordinates{NewRouteCoordinates(-srcWidth+destX, offZ+destZ, level)},
			Alternative: false,
			Success:     true,
		}
	} else if northwestClockwise && !northeastClockwise {
		// North
		offX := 0
		if anti >= -destLength {
			offX = coerceAtMost(anti+destLength, destWidth-1)
		} else if diagonal < destLength {
			offX = coerceAtLeast(diagonal-destLength, -(srcWidth - 1))
		}
		return Route{
			Waypoints:   []RouteCoordinates{NewRouteCoordinates(offX+destX, destLength+destZ, level)},
			Alternative: false,
			Success:     true,
		}
	} else if northeastClockwise && !southeastClockwise {
		// East
		offZ := 0
		if anti <= destWidth {
			offZ = destLength - anti
		} else if diagonal < destWidth {
			offZ = coerceAtLeast(diagonal-destWidth, -(srcLength - 1))
		}
		return Route{
			Waypoints:   []RouteCoordinates{NewRouteCoordinates(destWidth+destX, offZ+destZ, level)},
			Alternative: false,
			Success:     true,
		}
	} else {
		if !(southeastClockwise && !southwestClockwise) {
			panic("southeastClockwise must be true and southwestClockwise must be false")
		}
		// South
		offX := 0
		if diagonal > -srcLength {
			offX = coerceAtMost(diagonal+srcLength, destWidth-1)
		} else if anti < srcLength {
			offX = coerceAtLeast(anti-srcLength, -(srcLength - 1))
		}
		return Route{
			Waypoints:   []RouteCoordinates{NewRouteCoordinates(offX+destX, -srcLength+destZ, level)},
			Alternative: false,
			Success:     true,
		}
	}
}

/**
 * Ensures that this value is not greater than the specified maximumValue.
 */
func coerceAtMost(value, maximumValue int) int {
	if value > maximumValue {
		return maximumValue
	}
	return value
}

/**
 * Ensures that this value is not less than the specified minimumValue.
 */
func coerceAtLeast(value, minimumValue int) int {
	if value < minimumValue {
		return minimumValue
	}
	return value
}
