package reach

import (
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/pathfinder/flag"
)

func Collides(srcX int, srcZ int, destX int, destZ int, srcWidth int, srcLength int, destWidth int, destLength int) bool {
	if srcX >= destX+destWidth || srcX+srcWidth <= destX {
		return false
	}
	return srcZ < destZ+destLength && destZ < srcLength+srcZ
}

func reachRectangle1(flags collision.FlagMap, level int, srcX int, srcZ int, destX int, destZ int, destWidth int, destLength int, blockAccessFlags int) bool {
	east := destX + destWidth - 1
	north := destZ + destLength - 1

	if srcX == destX-1 &&
		srcZ >= destZ &&
		srcZ <= north &&
		flags.Get(srcX, srcZ, level)&collision.FlagWallEast == 0 &&
		blockAccessFlags&int(flag.BlockAccessWest) == 0 {
		return true
	}

	if srcX == east+1 &&
		srcZ >= destZ &&
		srcZ <= north &&
		flags.Get(srcX, srcZ, level)&collision.FlagWallWest == 0 &&
		blockAccessFlags&int(flag.BlockAccessEast) == 0 {
		return true
	}

	if srcZ+1 == destZ &&
		srcX >= destX &&
		srcX <= east &&
		flags.Get(srcX, srcZ, level)&collision.FlagWallNorth == 0 &&
		blockAccessFlags&int(flag.BlockAccessSouth) == 0 {
		return true
	}

	return srcZ == north+1 &&
		srcX >= destX &&
		srcX <= east &&
		flags.Get(srcX, srcZ, level)&collision.FlagWallSouth == 0 &&
		blockAccessFlags&int(flag.BlockAccessNorth) == 0
}

func reachRectangleN(flags collision.FlagMap, level int, srcX int, srcZ int, destX int, destZ int, srcWidth int, srcLength int, destWidth int, destLength int, blockAccessFlags int) bool {
	srcEast := srcX + srcWidth
	srcNorth := srcLength + srcZ
	destEast := destWidth + destX
	destNorth := destLength + destZ

	if destEast == srcX && blockAccessFlags&int(flag.BlockAccessEast) == collision.FlagOpen {
		fromZ := max(srcZ, destZ)
		toZ := min(srcNorth, destNorth)
		for sideZ := fromZ; sideZ < toZ; sideZ++ {
			if flags.Get(destEast-1, sideZ, level)&collision.FlagWallEast == collision.FlagOpen {
				return true
			}
		}
	} else if srcEast == destX && blockAccessFlags&int(flag.BlockAccessWest) == collision.FlagOpen {
		fromZ := max(srcZ, destZ)
		toZ := min(srcNorth, destNorth)
		for sideZ := fromZ; sideZ < toZ; sideZ++ {
			if flags.Get(destX, sideZ, level)&collision.FlagWallWest == collision.FlagOpen {
				return true
			}
		}
	} else if srcZ == destNorth && blockAccessFlags&int(flag.BlockAccessNorth) == collision.FlagOpen {
		fromX := max(srcX, destX)
		toX := min(srcEast, destEast)
		for sideX := fromX; sideX < toX; sideX++ {
			if flags.Get(sideX, destNorth-1, level)&collision.FlagWallNorth == collision.FlagOpen {
				return true
			}
		}
	} else if destZ == srcNorth && blockAccessFlags&int(flag.BlockAccessSouth) == collision.FlagOpen {
		fromX := max(srcX, destX)
		toX := min(srcEast, destEast)
		for sideX := fromX; sideX < toX; sideX++ {
			if flags.Get(sideX, destZ, level)&collision.FlagWallSouth == collision.FlagOpen {
				return true
			}
		}
	}

	return false
}
