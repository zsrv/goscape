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
