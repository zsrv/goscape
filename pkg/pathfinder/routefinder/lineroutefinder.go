package routefinder

import (
	"math"

	"github.com/zsrv/goscape/pkg/pathfinder/collision"
)

// lineRouteCoordsCap is the initial capacity for the per-call Coordinates
// slice. Ray-casts append one or two coords per tile traversed along the
// major axis -- short LOS/LOW checks are a few tiles, mid-range hunt /
// projectile checks are ~30 tiles, the absolute long-axis upper bound is
// ~200 coords. 64 covers the common case in a single allocation while
// keeping the worst-case overshoot small (vs cap=200 which adds ~1.7KB
// per call for the typical short check). The append-doubling-growth path
// still handles the rare long ray-cast correctly.
//
// See PORTING.md NEW-F. Lineage: Arc 17 dd133052 / Arc 12 a089cdaf used
// the same "pre-allocate to skip doubling growth" pattern for the cache
// CRC + type-config snapshot bundles.
const lineRouteCoordsCap = 64

type LineRouteFinder struct {
	flags collision.FlagMap
}

func NewLineRouteFinder(flags collision.FlagMap) LineRouteFinder {
	return LineRouteFinder{
		flags: flags,
	}
}

func (pf LineRouteFinder) LineOfSight(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) RayCast {
	// srcSize default = 1
	return pf.RayCast(level, srcX, srcZ, destX, destZ, srcSize, srcSize, destWidth, destLength,
		LineSightBlockedWest|extraFlag,
		LineSightBlockedEast|extraFlag,
		LineSightBlockedSouth|extraFlag,
		LineSightBlockedNorth|extraFlag,
		collision.FlagLoc|extraFlag,
		collision.FlagLocProjBlocker|extraFlag,
		true)
}

func (pf LineRouteFinder) LineOfWalk(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) RayCast {
	return pf.RayCast(level, srcX, srcZ, destX, destZ, srcSize, srcSize, destWidth, destLength,
		LineWalkBlockedWest|extraFlag,
		LineWalkBlockedEast|extraFlag,
		LineWalkBlockedSouth|extraFlag,
		LineWalkBlockedNorth|extraFlag,
		collision.FlagLoc|extraFlag,
		collision.FlagLocProjBlocker|extraFlag,
		false)
}

func (pf LineRouteFinder) RayCast(level, srcX, srcZ, destX, destZ, srcWidth, srcLength, destWidth, destLength,
	flagWest, flagEast, flagSouth, flagNorth, flagLocation, flagProjectileBlocker int, los bool) RayCast {
	startX := lineCoordinate(srcX, destX, srcWidth)
	startZ := lineCoordinate(srcZ, destZ, srcLength)

	endX := lineCoordinate(destX, srcX, destWidth)
	endZ := lineCoordinate(destZ, srcZ, destLength)

	if startX == endX && startZ == endZ {
		return RayCastSuccessNoCoords()
	}

	if los && pf.flags.IsFlagged(startX, startZ, level, flagLocation) {
		return RayCastFailed()
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

	coordinates := make([]RouteCoordinates, 0, lineRouteCoordsCap)
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
			if pf.flags.IsFlagged(currX, currZ, level, xFlags) {
				return RayCast{
					Coordinates: coordinates,
					Alternative: len(coordinates) > 0,
					Success:     false,
				}
			}
			coordinates = append(coordinates, NewRouteCoordinates(currX, currZ, level))

			scaledZ += tangent

			nextZ := lineScaleDown(scaledZ)
			if nextZ != currZ {
				if los && currX == endX && nextZ == endZ {
					zFlags = zFlags & ^flagProjectileBlocker
				}
				if pf.flags.IsFlagged(currX, nextZ, level, zFlags) {
					return RayCast{
						Coordinates: coordinates,
						Alternative: len(coordinates) > 0,
						Success:     false,
					}
				}
				coordinates = append(coordinates, NewRouteCoordinates(currX, nextZ, level))
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
			if pf.flags.IsFlagged(currX, currZ, level, zFlags) {
				return RayCast{
					Coordinates: coordinates,
					Alternative: len(coordinates) > 0,
					Success:     false,
				}
			}
			coordinates = append(coordinates, NewRouteCoordinates(currX, currZ, level))

			scaledX += tangent

			nextX := lineScaleDown(scaledX)
			if nextX != currX {
				if los && nextX == endX && currZ == endZ {
					xFlags = xFlags & ^flagProjectileBlocker
				}
				if pf.flags.IsFlagged(nextX, currZ, level, xFlags) {
					return RayCast{
						Coordinates: coordinates,
						Alternative: len(coordinates) > 0,
						Success:     false,
					}
				}
				coordinates = append(coordinates, NewRouteCoordinates(nextX, currZ, level))
			}
		}
	}
	return RayCast{
		Coordinates: coordinates,
		Alternative: false,
		Success:     true,
	}
}
