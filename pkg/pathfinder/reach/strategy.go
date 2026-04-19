package reach

import (
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/pathfinder/loc"
	"github.com/zsrv/goscape/pkg/pathfinder/rotation"
)

const (
	wallStrategy int = iota
	wallDecoStrategy
	rectangleStrategy
	noStrategy
	rectangleExclusiveStrategy
)

// Reached reports whether the coordinates (srcX, srcZ) can reach coordinates (destX, destZ),
// taking into account the dimensions destWidth, destLength and srcSize.
//
// destWidth is the absolute width of the destination. This value should not be changed when passing
// the width of a rotated loc (it is done within the function).
//
// destLength is the absolute length of the destination. Similar to destWidth, this value should not
// be changed for rotated locs.
//
// locAngle is the angle of the target loc being used as the destination. If the path is meant for
// something that is not a loc, this value should be passed as the default 0.
//
// locShape is the shape of the target loc being used as the destination. If the path is meant for
// something that is not a loc, this value should be passed as the default 0.
//
// blockAccessFlags are packed directional bitflags where interaction should be blocked. This can
// be seen in locs such as staircases, where all directions excluding the direction with access to
// the steps are "blocked". See flag.BlockAccessFlag.
func Reached(flags collision.FlagMap, level, srcX, srcZ, destX, destZ, destWidth, destLength, srcSize, locAngle, locShape, blockAccessFlags int) bool {
	strategy := exitStrategy(locShape)
	if strategy != rectangleExclusiveStrategy && srcX == destX && srcZ == destZ {
		return true
	}

	switch strategy {
	case wallStrategy:
		return ReachWall(flags, level, srcX, srcZ, destX, destZ, srcSize, locShape, locAngle)
	case wallDecoStrategy:
		return ReachWallDeco(flags, level, srcX, srcZ, destX, destZ, srcSize, locShape, locAngle)
	case rectangleStrategy:
		return ReachRectangle(flags, level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, locAngle, blockAccessFlags)
	case rectangleExclusiveStrategy:
		return ReachExclusiveRectangle(flags, level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, locAngle, blockAccessFlags)
	default:
		return false
	}
}

func ReachRectangle(flags collision.FlagMap, level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, locAngle, blockAccessFlags int) bool {
	rotatedWidth := rotation.Rotate(locAngle, destWidth, destLength)
	rotatedLength := rotation.Rotate(locAngle, destLength, destWidth)
	rotatedBlockAccess := rotation.RotateFlags(locAngle, blockAccessFlags)

	collides := Collides(srcX, srcZ, destX, destZ, srcSize, srcSize, rotatedWidth, rotatedLength)
	if srcSize > 1 {
		return collides || reachRectangleN(flags, level, srcX, srcZ, destX, destZ, srcSize, srcSize, rotatedWidth, rotatedLength, rotatedBlockAccess)
	}
	return collides || reachRectangle1(flags, level, srcX, srcZ, destX, destZ, rotatedWidth, rotatedLength, rotatedBlockAccess)
}

func ReachExclusiveRectangle(flags collision.FlagMap, level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, locAngle, blockAccessFlags int) bool {
	rotatedWidth := rotation.Rotate(locAngle, destWidth, destLength)
	rotatedLength := rotation.Rotate(locAngle, destLength, destWidth)
	rotatedBlockAccess := rotation.RotateFlags(locAngle, blockAccessFlags)

	collides := Collides(srcX, srcZ, destX, destZ, srcSize, srcSize, rotatedWidth, rotatedLength)
	if srcSize > 1 {
		return !collides && reachRectangleN(flags, level, srcX, srcZ, destX, destZ, srcSize, srcSize, rotatedWidth, rotatedLength, rotatedBlockAccess)
	}
	return !collides && reachRectangle1(flags, level, srcX, srcZ, destX, destZ, rotatedWidth, rotatedLength, rotatedBlockAccess)
}

func ReachWall(flags collision.FlagMap, level, srcX, srcZ, destX, destZ, srcSize, locShape, locAngle int) bool {
	if srcSize == 1 && srcX == destX && srcZ == destZ {
		return true
	}
	if srcSize != 1 && destX >= srcX && srcSize+srcX-1 >= destX && destZ >= srcZ && srcSize+srcZ-1 >= destZ {
		return true
	}
	if srcSize == 1 {
		return ReachWall1(flags, level, srcX, srcZ, destX, destZ, locShape, locAngle)
	}
	return reachWallN(flags, level, srcX, srcZ, destX, destZ, srcSize, locShape, locAngle)
}

func ReachWallDeco(flags collision.FlagMap, level, srcX, srcZ, destX, destZ, srcSize, locShape, locAngle int) bool {
	if srcSize == 1 && srcX == destX && srcZ == destZ {
		return true
	}
	if srcSize != 1 && destX >= srcX && srcSize+srcX-1 >= destX && destZ >= srcZ && srcSize+srcZ-1 >= destZ {
		return true
	}
	if srcSize == 1 {
		return reachWallDeco1(flags, level, srcX, srcZ, destX, destZ, locShape, locAngle)
	}
	return reachWallDecoN(flags, level, srcX, srcZ, destX, destZ, srcSize, locShape, locAngle)
}

func ReachWall1(flags collision.FlagMap, level, srcX, srcZ, destX, destZ, locShape, locAngle int) bool {
	collisionFlags := flags.Get(srcX, srcZ, level)
	switch loc.Shape(locShape) {
	case loc.ShapeWallStraight:
		switch locAngle {
		case loc.AngleWest:
			if srcX == destX-1 && srcZ == destZ {
				return true
			}
			if srcX == destX && srcZ == destZ+1 && (collisionFlags&collision.FlagBlockNorth) == collision.FlagOpen {
				return true
			}
			if srcX == destX && srcZ == destZ-1 && (collisionFlags&collision.FlagBlockSouth) == collision.FlagOpen {
				return true
			}
			return false
		case loc.AngleNorth:
			if srcX == destX && srcZ == destZ+1 {
				return true
			}
			if srcX == destX-1 && srcZ == destZ && (collisionFlags&collision.FlagBlockWest) == collision.FlagOpen {
				return true
			}
			if srcX == destX+1 && srcZ == destZ && (collisionFlags&collision.FlagBlockEast) == collision.FlagOpen {
				return true
			}
			return false
		case loc.AngleEast:
			if srcX == destX+1 && srcZ == destZ {
				return true
			}
			if srcX == destX && srcZ == destZ+1 && (collisionFlags&collision.FlagBlockNorth) == collision.FlagOpen {
				return true
			}
			if srcX == destX && srcZ == destZ-1 && (collisionFlags&collision.FlagBlockSouth) == collision.FlagOpen {
				return true
			}
			return false
		case loc.AngleSouth:
			if srcX == destX && srcZ == destZ-1 {
				return true
			}
			if srcX == destX-1 && srcZ == destZ && (collisionFlags&collision.FlagBlockWest) == collision.FlagOpen {
				return true
			}
			if srcX == destX+1 && srcZ == destZ && (collisionFlags&collision.FlagBlockEast) == collision.FlagOpen {
				return true
			}
			return false
		default:
			return false
		}
	case loc.ShapeWallL:
		switch locAngle {
		case loc.AngleWest:
			if srcX == destX-1 && srcZ == destZ {
				return true
			}
			if srcX == destX && srcZ == destZ+1 {
				return true
			}
			if srcX == destX+1 && srcZ == destZ && (collisionFlags&collision.FlagBlockEast) == collision.FlagOpen {
				return true
			}
			if srcX == destX && srcZ == destZ-1 && (collisionFlags&collision.FlagBlockSouth) == collision.FlagOpen {
				return true
			}
			return false
		case loc.AngleNorth:
			if srcX == destX-1 && srcZ == destZ && (collisionFlags&collision.FlagBlockWest) == collision.FlagOpen {
				return true
			}
			if srcX == destX && srcZ == destZ+1 {
				return true
			}
			if srcX == destX+1 && srcZ == destZ {
				return true
			}
			if srcX == destX && srcZ == destZ-1 && (collisionFlags&collision.FlagBlockSouth) == collision.FlagOpen {
				return true
			}
			return false
		case loc.AngleEast:
			if srcX == destX-1 && srcZ == destZ && (collisionFlags&collision.FlagBlockWest) == collision.FlagOpen {
				return true
			}
			if srcX == destX && srcZ == destZ+1 && (collisionFlags&collision.FlagBlockNorth) == collision.FlagOpen {
				return true
			}
			if srcX == destX+1 && srcZ == destZ {
				return true
			}
			if srcX == destX && srcZ == destZ-1 {
				return true
			}
			return false
		case loc.AngleSouth:
			if srcX == destX-1 && srcZ == destZ {
				return true
			}
			if srcX == destX && srcZ == destZ+1 && (collisionFlags&collision.FlagBlockNorth) == collision.FlagOpen {
				return true
			}
			if srcX == destX+1 && srcZ == destZ && (collisionFlags&collision.FlagBlockEast) == collision.FlagOpen {
				return true
			}
			if srcX == destX && srcZ == destZ-1 {
				return true
			}
			return false
		}
	case loc.ShapeWallDiagonal:
		if srcX == destX && srcZ == destZ+1 && (collisionFlags&collision.FlagWallSouth) == collision.FlagOpen {
			return true
		}
		if srcX == destX && srcZ == destZ-1 && (collisionFlags&collision.FlagWallNorth) == collision.FlagOpen {
			return true
		}
		if srcX == destX-1 && srcZ == destZ && (collisionFlags&collision.FlagWallEast) == collision.FlagOpen {
			return true
		}
		if srcX == destX+1 && srcZ == destZ && (collisionFlags&collision.FlagWallWest) == collision.FlagOpen {
			return true
		}
		return false
	}
	return false
}

func reachWallN(flags collision.FlagMap, level, srcX, srcZ, destX, destZ, srcSize, locShape, locAngle int) bool {
	collisionFlags := flags.Get(srcX, srcZ, level)
	east := srcX + srcSize - 1
	north := srcZ + srcSize - 1
	switch loc.Shape(locShape) {
	case loc.ShapeWallStraight:
		switch locAngle {
		case loc.AngleWest:
			if srcX == destX-srcSize && srcZ <= destZ && north >= destZ {
				return true
			}
			if destX >= srcX && destX <= east && srcZ == destZ+1 && (collisionFlags&collision.FlagBlockNorth) == collision.FlagOpen {
				return true
			}
			if destX >= srcX && destX <= east && srcZ == destZ-srcSize && (collisionFlags&collision.FlagBlockSouth) == collision.FlagOpen {
				return true
			}
			return false
		case loc.AngleNorth:
			if destX >= srcX && destX <= east && srcZ == destZ+1 {
				return true
			}
			if srcX == destX-srcSize && srcZ <= destZ && north >= destZ && (collisionFlags&collision.FlagBlockWest) == collision.FlagOpen {
				return true
			}
			if srcX == destX+1 && srcZ <= destZ && north >= destZ && (collisionFlags&collision.FlagBlockEast) == collision.FlagOpen {
				return true
			}
			return false
		case loc.AngleEast:
			if srcX == destX+1 && srcZ <= destZ && north >= destZ {
				return true
			}
			if destX >= srcX && destX <= east && srcZ == destZ+1 && (collisionFlags&collision.FlagBlockNorth) == collision.FlagOpen {
				return true
			}
			if destX >= srcX && destX <= east && srcZ == destZ-srcSize && (collisionFlags&collision.FlagBlockSouth) == collision.FlagOpen {
				return true
			}
			return false
		case loc.AngleSouth:
			if destX >= srcX && destX <= east && srcZ == destZ-srcSize {
				return true
			}
			if srcX == destX-srcSize && srcZ <= destZ && north >= destZ && (collisionFlags&collision.FlagBlockWest) == collision.FlagOpen {
				return true
			}
			if srcX == destX+1 && srcZ <= destZ && north >= destZ && (collisionFlags&collision.FlagBlockEast) == collision.FlagOpen {
				return true
			}
			return false
		}
		return false
	case loc.ShapeWallL:
		switch locAngle {
		case loc.AngleWest:
			if srcX == destX-srcSize && srcZ <= destZ && north >= destZ {
				return true
			}
			if destX >= srcX && destX <= east && srcZ == destZ+1 {
				return true
			}
			if srcX == destX+1 && srcZ <= destZ && north >= destZ && (collisionFlags&collision.FlagBlockEast) == collision.FlagOpen {
				return true
			}
			if destX >= srcX && destX <= east && srcZ == destZ-srcSize && (collisionFlags&collision.FlagBlockSouth) == collision.FlagOpen {
				return true
			}
			return false
		case loc.AngleNorth:
			if srcX == destX-srcSize && srcZ <= destZ && north >= destZ && (collisionFlags&collision.FlagBlockWest) == collision.FlagOpen {
				return true
			}
			if destX >= srcX && destX <= east && srcZ == destZ+1 {
				return true
			}
			if srcX == destX+1 && srcZ <= destZ && north >= destZ {
				return true
			}
			if destX >= srcX && destX <= east && srcZ == destZ-srcSize && (collisionFlags&collision.FlagBlockSouth) == collision.FlagOpen {
				return true
			}
			return false
		case loc.AngleEast:
			if srcX == destX-srcSize && srcZ <= destZ && north >= destZ && (collisionFlags&collision.FlagBlockWest) == collision.FlagOpen {
				return true
			}
			if destX >= srcX && destX <= east && srcZ == destZ+1 && (collisionFlags&collision.FlagBlockNorth) == collision.FlagOpen {
				return true
			}
			if srcX == destX+1 && srcZ <= destZ && north >= destZ {
				return true
			}
			if destX >= srcX && destX <= east && srcZ == destZ-srcSize {
				return true
			}
			return false
		case loc.AngleSouth:
			if srcX == destX-srcSize && srcZ <= destZ && north >= destZ {
				return true
			}
			if destX >= srcX && destX <= east && srcZ == destZ+1 && (collisionFlags&collision.FlagBlockNorth) == collision.FlagOpen {
				return true
			}
			if srcX == destX+1 && srcZ <= destZ && north >= destZ && (collisionFlags&collision.FlagBlockEast) == collision.FlagOpen {
				return true
			}
			if destX >= srcX && destX <= east && srcZ == destZ-srcSize {
				return true
			}
			return false
		}
		return false
	case loc.ShapeWallDiagonal:
		if destX >= srcX && destX <= east && srcZ == destZ+1 && (collisionFlags&collision.FlagBlockNorth) == collision.FlagOpen {
			return true
		}
		if destX >= srcX && destX <= east && srcZ == destZ-srcSize && (collisionFlags&collision.FlagBlockSouth) == collision.FlagOpen {
			return true
		}
		if srcX == destX-srcSize && srcZ <= destZ && north >= destZ && (collisionFlags&collision.FlagBlockWest) == collision.FlagOpen {
			return true
		}
		if srcX == destX+1 && srcZ <= destZ && north >= destZ && (collisionFlags&collision.FlagBlockEast) == collision.FlagOpen {
			return true
		}
		return false
	default:
		return false
	}
}

func reachWallDeco1(flags collision.FlagMap, level, srcX, srcZ, destX, destZ, locShape, locAngle int) bool {
	collisionFlags := flags.Get(srcX, srcZ, level)
	switch loc.Shape(locShape) {
	case loc.ShapeWallDecorDiagonalOffset, loc.ShapeWallDecorDiagonalNoOffset:
		altered := alteredAngle(locAngle, locShape)
		switch altered {
		case loc.AngleWest:
			if srcX == destX+1 && srcZ == destZ && (collisionFlags&collision.FlagWallWest) == collision.FlagOpen {
				return true
			}
			if srcX == destX && srcZ == destZ-1 && (collisionFlags&collision.FlagWallNorth) == collision.FlagOpen {
				return true
			}
			return false
		case loc.AngleNorth:
			if srcX == destX-1 && srcZ == destZ && (collisionFlags&collision.FlagWallEast) == collision.FlagOpen {
				return true
			}
			if srcX == destX && srcZ == destZ-1 && (collisionFlags&collision.FlagWallNorth) == collision.FlagOpen {
				return true
			}
			return false
		case loc.AngleEast:
			if srcX == destX-1 && srcZ == destZ && (collisionFlags&collision.FlagWallEast) == collision.FlagOpen {
				return true
			}
			if srcX == destX && srcZ == destZ+1 && (collisionFlags&collision.FlagWallSouth) == collision.FlagOpen {
				return true
			}
			return false
		case loc.AngleSouth:
			if srcX == destX+1 && srcZ == destZ && (collisionFlags&collision.FlagWallWest) == collision.FlagOpen {
				return true
			}
			if srcX == destX && srcZ == destZ+1 && (collisionFlags&collision.FlagWallSouth) == collision.FlagOpen {
				return true
			}
			return false
		default:
			return false
		}
	case loc.ShapeWallDecorDiagonalBoth:
		if srcX == destX && srcZ == destZ+1 && (collisionFlags&collision.FlagWallSouth) == collision.FlagOpen {
			return true
		}
		if srcX == destX && srcZ == destZ-1 && (collisionFlags&collision.FlagWallNorth) == collision.FlagOpen {
			return true
		}
		if srcX == destX-1 && srcZ == destZ && (collisionFlags&collision.FlagWallEast) == collision.FlagOpen {
			return true
		}
		if srcX == destX+1 && srcZ == destZ && (collisionFlags&collision.FlagWallWest) == collision.FlagOpen {
			return true
		}
		return false
	default:
		return false
	}
}

func reachWallDecoN(flags collision.FlagMap, level, srcX, srcZ, destX, destZ, srcSize, locShape, locAngle int) bool {
	collisionFlags := flags.Get(srcX, srcZ, level)
	east := srcX + srcSize - 1
	north := srcZ + srcSize - 1
	switch loc.Shape(locShape) {
	case loc.ShapeWallDecorDiagonalOffset, loc.ShapeWallDecorDiagonalNoOffset:
		alteredAngle := alteredAngle(locAngle, locShape)
		switch alteredAngle {
		case loc.AngleWest:
			if srcX == destX+1 && srcZ <= destZ && north >= destZ && (collisionFlags&collision.FlagWallWest) == collision.FlagOpen {
				return true
			}
			if srcX <= destX && srcZ == destZ-srcSize && east >= destX && (collisionFlags&collision.FlagWallNorth) == collision.FlagOpen {
				return true
			}
			return false
		case loc.AngleNorth:
			if srcX == destX-srcSize && srcZ <= destZ && north >= destZ && (collisionFlags&collision.FlagWallEast) == collision.FlagOpen {
				return true
			}
			if srcX <= destX && srcZ == destZ-srcSize && east >= destX && (collisionFlags&collision.FlagWallNorth) == collision.FlagOpen {
				return true
			}
			return false
		case loc.AngleEast:
			if srcX == destX-srcSize && srcZ <= destZ && north >= destZ && (collisionFlags&collision.FlagWallEast) == collision.FlagOpen {
				return true
			}
			if srcX <= destX && srcZ == destZ+1 && east >= destX && (collisionFlags&collision.FlagWallSouth) == collision.FlagOpen {
				return true
			}
			return false
		case loc.AngleSouth:
			if srcX == destX+1 && srcZ <= destZ && north >= destZ && (collisionFlags&collision.FlagWallWest) == collision.FlagOpen {
				return true
			}
			if srcX <= destX && srcZ == destZ+1 && east >= destX && (collisionFlags&collision.FlagWallSouth) == collision.FlagOpen {
				return true
			}
			return false
		default:
			return false
		}
	case loc.ShapeWallDecorDiagonalBoth:
		if srcX <= destX && srcZ == destZ+1 && east >= destX && (collisionFlags&collision.FlagWallSouth) == 0 {
			return true
		}
		if srcX <= destX && srcZ == destZ-srcSize && east >= destX && (collisionFlags&collision.FlagWallNorth) == collision.FlagOpen {
			return true
		}
		if srcX == destX-srcSize && srcZ <= destZ && north >= destZ && (collisionFlags&collision.FlagWallEast) == collision.FlagOpen {
			return true
		}
		if srcX == destX+1 && srcZ <= destZ && north >= destZ && (collisionFlags&collision.FlagWallWest) == collision.FlagOpen {
			return true
		}
		return false
	default:
		return false
	}
}

func exitStrategy(locShape int) int {
	if locShape == -2 {
		return rectangleExclusiveStrategy
	}
	if locShape == -1 {
		return noStrategy
	}
	if (locShape >= 0 && locShape <= 3) || locShape == 9 {
		return wallStrategy
	}
	if locShape < 9 {
		return wallDecoStrategy
	}
	if (locShape >= 10 && locShape <= 11) || locShape == 22 {
		return rectangleStrategy
	}
	return noStrategy
}

func alteredAngle(angle int, shape int) int {
	if loc.Shape(shape) == loc.ShapeWallDecorDiagonalNoOffset {
		return (angle + 2) & 0x3
	}
	return angle
}
