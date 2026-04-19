package routefinder

import (
	"math"

	"github.com/zsrv/goscape/pkg/pathfinder/collision"
)

type LineValidator struct {
	flags collision.FlagMap
}

func NewLineValidator(flags collision.FlagMap) LineValidator {
	return LineValidator{
		flags: flags,
	}
}

func (v LineValidator) HasLineOfSight(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool {
	// srcSize default = 1, rest 0
	return v.RayCast(level, srcX, srcZ, destX, destZ, srcSize, srcSize, destWidth, destLength,
		LineSightBlockedWest|extraFlag,
		LineSightBlockedEast|extraFlag,
		LineSightBlockedSouth|extraFlag,
		LineSightBlockedNorth|extraFlag,
		collision.FlagLoc|extraFlag,
		collision.FlagLocProjBlocker|extraFlag,
		true)
}

func (v LineValidator) HasLineOfWalk(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool {
	return v.RayCast(level, srcX, srcZ, destX, destZ, srcSize, srcSize, destWidth, destLength,
		LineWalkBlockedWest|extraFlag,
		LineWalkBlockedEast|extraFlag,
		LineWalkBlockedSouth|extraFlag,
		LineWalkBlockedNorth|extraFlag,
		collision.FlagLoc|extraFlag,
		collision.FlagLocProjBlocker|extraFlag,
		false)
}

func (v LineValidator) RayCast(level, srcX, srcZ, destX, destZ, srcWidth, srcLength, destWidth, destLength,
	flagWest, flagEast, flagSouth, flagNorth, flagLocation, flagProjectileBlocker int, los bool) bool {
	startX := lineCoordinate(srcX, destX, srcWidth)
	startZ := lineCoordinate(srcZ, destZ, srcLength)

	endX := lineCoordinate(destX, srcX, destWidth)
	endZ := lineCoordinate(destZ, srcZ, destLength)

	if startX == endX && startZ == endZ {
		return true
	}

	if los && v.flags.IsFlagged(startX, startZ, level, flagLocation) {
		return false
	}

	deltaX := endX - startX
	deltaZ := endZ - startZ
	absoluteDeltaX := int(math.Abs(float64(deltaX)))
	absoluteDeltaZ := int(math.Abs(float64(deltaZ)))

	travelEast := deltaX >= 0
	travelNorth := deltaZ >= 0

	var xFlags int
	if travelEast {
		xFlags = flagWest
	} else {
		xFlags = flagEast
	}

	var zFlags int
	if travelNorth {
		zFlags = flagSouth
	} else {
		zFlags = flagNorth
	}

	if absoluteDeltaX > absoluteDeltaZ {
		var offsetX int
		if travelEast {
			offsetX = 1
		} else {
			offsetX = -1
		}

		var offsetZ int
		if travelNorth {
			offsetZ = 0
		} else {
			offsetZ = -1
		}

		scaledZ := lineScaleUp(startZ) + lineHalfTile + offsetZ
		tangent := lineScaleUp(deltaZ) / absoluteDeltaX

		currX := startX
		for currX != endX {
			currX += offsetX
			currZ := lineScaleDown(scaledZ)
			if los && currX == endX && currZ == endZ {
				xFlags = xFlags & ^flagProjectileBlocker
			}
			if v.flags.IsFlagged(currX, currZ, level, xFlags) {
				return false
			}

			scaledZ += tangent

			nextZ := lineScaleDown(scaledZ)
			if los && currX == endX && nextZ == endZ {
				zFlags = zFlags & ^flagProjectileBlocker
			}
			if nextZ != currZ && v.flags.IsFlagged(currX, nextZ, level, zFlags) {
				return false
			}
		}
	} else {
		var offsetX int
		if travelEast {
			offsetX = 0
		} else {
			offsetX = -1
		}

		var offsetZ int
		if travelNorth {
			offsetZ = 1
		} else {
			offsetZ = -1
		}

		scaledX := lineScaleUp(startX) + lineHalfTile + offsetX
		tangent := lineScaleUp(deltaX) / absoluteDeltaZ

		currZ := startZ
		for currZ != endZ {
			currZ += offsetZ
			currX := lineScaleDown(scaledX)
			if los && currX == endX && currZ == endZ {
				zFlags = zFlags & ^flagProjectileBlocker
			}
			if v.flags.IsFlagged(currX, currZ, level, zFlags) {
				return false
			}

			scaledX += tangent

			nextX := lineScaleDown(scaledX)
			if los && nextX == endX && currZ == endZ {
				xFlags = xFlags & ^flagProjectileBlocker
			}
			if nextX != currX && v.flags.IsFlagged(nextX, currZ, level, xFlags) {
				return false
			}
		}
	}

	return true
}
