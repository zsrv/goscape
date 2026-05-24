package routefinder

import (
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
)

const (
	LineSightBlockedNorth = collision.FlagLocProjBlocker | collision.FlagWallNorthProjBlocker
	LineSightBlockedEast  = collision.FlagLocProjBlocker | collision.FlagWallEastProjBlocker
	LineSightBlockedSouth = collision.FlagLocProjBlocker | collision.FlagWallSouthProjBlocker
	LineSightBlockedWest  = collision.FlagLocProjBlocker | collision.FlagWallWestProjBlocker

	LineWalkBlockedNorth = collision.FlagWallNorth | collision.FlagWalkBlocked
	LineWalkBlockedEast  = collision.FlagWallEast | collision.FlagWalkBlocked
	LineWalkBlockedSouth = collision.FlagWallSouth | collision.FlagWalkBlocked
	LineWalkBlockedWest  = collision.FlagWallWest | collision.FlagWalkBlocked

	lineHalfTile = (1 << 16) / 2 // lineScaleUp(1) / 2
)

func lineScaleUp(tiles int) int {
	return tiles << 16
}

// lineScaleDown converts a fixed-point value (16-bit fraction) back to a tile
// coordinate. The authoritative rsmod line-of-sight code shifts a signed int;
// a JS transliteration of it uses the unsigned `>>> 16`. The two differ only
// for a negative operand, and the operand never is: in RayCast the scaled
// value tracks a coordinate linearly interpolated between two non-negative map
// tiles (scaledZ/scaledX start at startZ/X<<16 + halfTile and step toward
// endZ/X<<16, both >= 0), so arithmetic `>>` equals logical `>>> 16` over the
// entire reachable domain. Kept as `>>` — idiomatic Go and what the signed-int
// source does. Pinned by TestLineScaleDown_NonNegativeMatchesLogicalShift. L42.
func lineScaleDown(tiles int) int {
	return tiles >> 16
}

func lineCoordinate(a int, b int, size int) int {
	if a >= b {
		return a
	}
	if a+size-1 <= b {
		return a + size - 1
	}
	return b
}
