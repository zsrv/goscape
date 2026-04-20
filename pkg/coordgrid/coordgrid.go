package coordgrid

import (
	"fmt"
	"math"
)

type Direction int

const (
	DirectionNorthwest Direction = iota
	DirectionNorth
	DirectionNortheast
	DirectionWest
	DirectionEast
	DirectionSouthwest
	DirectionSouth
	DirectionSoutheast
)

func Zone(pos int) int {
	return pos >> 3
}

func ZoneCenter(pos int) int {
	return Zone(pos) - 6
}

func ZoneOrigin(pos int) int {
	return ZoneCenter(pos) << 3
}

func MapSquare(pos uint16) uint16 {
	return pos >> 6
}

func Local(pos int, origin int) int {
	return pos - (ZoneCenter(origin) << 3)
}

func Face(srcX, srcZ, dstX, dstZ int) Direction {
	if srcX == dstX {
		if srcZ > dstZ {
			return DirectionSouth
		} else if srcZ < dstZ {
			return DirectionNorth
		}
	} else if srcX > dstX {
		if srcZ > dstZ {
			return DirectionSouthwest
		} else if srcZ < dstZ {
			return DirectionNorthwest
		} else {
			return DirectionWest
		}
	} else {
		if srcZ > dstZ {
			return DirectionSoutheast
		} else if srcZ < dstZ {
			return DirectionNortheast
		} else {
			return DirectionEast
		}
	}
	return -1
}

func DeltaX(dir Direction) int {
	switch dir {
	case DirectionSoutheast, DirectionNortheast, DirectionEast:
		return 1
	case DirectionSouthwest, DirectionNorthwest, DirectionWest:
		return -1
	default:
		return 0
	}
}

func DeltaZ(dir Direction) int {
	switch dir {
	case DirectionNorthwest, DirectionNortheast, DirectionNorth:
		return 1
	case DirectionSouthwest, DirectionSoutheast, DirectionSouth:
		return -1
	default:
		return 0
	}
}

func MoveX(pos int, dir Direction) int {
	return pos + DeltaX(dir)
}

func MoveZ(pos int, dir Direction) int {
	return pos + DeltaZ(dir)
}

func Closest(posX, posZ, posWidth, posLength, otherX, otherZ, otherWidth, otherLength int) (x, z int) {
	occupiedX := posX + posWidth - 1
	occupiedZ := posZ + posLength - 1
	if otherX <= posX {
		x = posX
	} else if otherX >= occupiedX {
		x = occupiedX
	} else {
		x = otherX
	}
	if otherZ <= posZ {
		z = posZ
	} else if otherZ >= occupiedZ {
		z = occupiedZ
	} else {
		z = otherZ
	}
	return
}

func DistanceTo(posX, posZ, posWidth, posLength, otherX, otherZ, otherWidth, otherLength int) int {
	p1X, p1Z := Closest(posX, posZ, posWidth, posLength, otherX, otherZ, otherWidth, otherLength)
	p2X, p2Z := Closest(otherX, otherZ, otherWidth, otherLength, posX, posZ, posWidth, posLength)
	return int(max(math.Abs(float64(p1X-p2X)), math.Abs(float64(p1Z-p2Z))))
}

func DistanceToSW(posX, posZ, otherX, otherZ int) int {
	dx := math.Abs(float64(posX - otherX))
	dz := math.Abs(float64(posZ - otherZ))
	return int(max(dx, dz))
}

func IsWithinDistanceSW(posX, posZ, otherX, otherZ, distance int) bool {
	if int(math.Abs(float64(posX-otherX))) > distance || int(math.Abs(float64(posZ-otherZ))) > distance {
		return false
	}
	return true
}

type Position struct {
	Level int
	X     int
	Z     int
}

func UnpackCoord(coord int) Position {
	return Position{
		Level: (coord >> 28) & 0x3,
		X:     (coord >> 14) & 0x3FFF,
		Z:     coord & 0x3FFF,
	}
}

func PackCoord(level, x, z int) int {
	return (z & 0x3fff) | ((x & 0x3fff) << 14) | ((level & 0x3) << 28)
}

// PackZoneCoord packs the zone-local low bits of a world-absolute (x, z)
// into a single byte: (x&7)<<4 | (z&7). Used inside every zone-nested
// packet encoder to identify which tile within the 8x8 zone an event
// refers to.
func PackZoneCoord(x, z int) byte {
	return byte((x&0x7)<<4 | (z & 0x7))
}

func Intersects(srcX, srcZ, srcWidth, srcHeight, destX, destZ, destWidth, destHeight int) bool {
	srcHorizontal := srcX + srcWidth
	srcVertical := srcZ + srcHeight
	destHorizontal := destX + destWidth
	destVertical := destZ + destHeight
	return !(destX >= srcHorizontal || destHorizontal <= srcX || destZ >= srcVertical || destVertical <= srcZ)
}

func FormatString(level, x, z int, separator string) string {
	mx := x >> 6
	mz := z >> 6
	lx := x & 0x3F
	lz := z & 0x3F
	return fmt.Sprintf("%d%s%d%s%d%s%d%s%d", level, separator, mx, separator, mz, separator, lx, separator, lz)
}

// ZoneIndex packs (worldX, worldZ, level) into a single int using the
// layout shared with the TS reference's ZoneMap.zoneIndex:
//
//	zone_x = worldX >> 3, zone_z = worldZ >> 3
//	index  = (zone_x & 0x7FF) | ((zone_z & 0x7FF) << 11) | ((level & 0x3) << 22)
func ZoneIndex(worldX, worldZ, level int) int {
	return ((worldX >> 3) & 0x7FF) | (((worldZ >> 3) & 0x7FF) << 11) | ((level & 0x3) << 22)
}

// UnpackZoneIndex reverses ZoneIndex. Returns TILE-unit coordinates at
// the zone's SW corner (zoneX<<3, zoneZ<<3).
func UnpackZoneIndex(index int) (worldX, worldZ, level int) {
	worldX = (index & 0x7FF) << 3
	worldZ = ((index >> 11) & 0x7FF) << 3
	level = (index >> 22) & 0x3
	return
}
