package routefinder

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/pathfinder/collision"
)

type StepValidator struct {
	flags collision.FlagMap
}

func NewStepValidator(flags collision.FlagMap) StepValidator {
	return StepValidator{flags: flags}
}

func (v StepValidator) CanTravel(level, x, z, offsetX, offsetZ, size, extraFlag int, collision collision.Type) bool {
	// size default = 1
	// collision default = collision.TypeNormal
	var blocked bool
	if offsetX == 0 && offsetZ == -1 {
		blocked = v.isBlockedSouth(level, x, z, size, extraFlag, collision)
	} else if offsetX == 0 && offsetZ == 1 {
		blocked = v.isBlockedNorth(level, x, z, size, extraFlag, collision)
	} else if offsetX == -1 && offsetZ == 0 {
		blocked = v.isBlockedWest(level, x, z, size, extraFlag, collision)
	} else if offsetX == 1 && offsetZ == 0 {
		blocked = v.isBlockedEast(level, x, z, size, extraFlag, collision)
	} else if offsetX == -1 && offsetZ == -1 {
		blocked = v.isBlockedSouthWest(level, x, z, size, extraFlag, collision)
	} else if offsetX == -1 && offsetZ == 1 {
		blocked = v.isBlockedNorthWest(level, x, z, size, extraFlag, collision)
	} else if offsetX == 1 && offsetZ == -1 {
		blocked = v.isBlockedSouthEast(level, x, z, size, extraFlag, collision)
	} else if offsetX == 1 && offsetZ == 1 {
		blocked = v.isBlockedNorthEast(level, x, z, size, extraFlag, collision)
	} else {
		// TODO: Error? or panic and recover at the root pathfinding func
		panic(fmt.Sprintf("invalid step tile offset: %d, %d", offsetX, offsetZ))
	}
	return !blocked
}

func (v StepValidator) isBlockedSouth(level, x, z, size, extraFlag int, collisionType collision.Type) bool {
	switch size {
	case 1:
		return !collision.CanMove(v.flags.Get(x, z-1, level), collision.FlagBlockSouth|extraFlag, collisionType)
	case 2:
		return !collision.CanMove(v.flags.Get(x, z-1, level), collision.FlagBlockSouthWest|extraFlag, collisionType) ||
			!collision.CanMove(v.flags.Get(x+1, z-1, level), collision.FlagBlockSouthEast|extraFlag, collisionType)
	default:
		if !collision.CanMove(v.flags.Get(x, z-1, level), collision.FlagBlockSouthWest|extraFlag, collisionType) {
			return true
		}
		if !collision.CanMove(v.flags.Get(x+size-1, z-1, level), collision.FlagBlockSouthEast|extraFlag, collisionType) {
			return true
		}
		for midX := x + 1; midX < x+size-1; midX++ {
			if !collision.CanMove(v.flags.Get(midX, z-1, level), collision.FlagBlockNorthEastAndWest|extraFlag, collisionType) {
				return true
			}
		}
		return false
	}
}

func (v StepValidator) isBlockedNorth(level, x, z, size, extraFlag int, collisionType collision.Type) bool {
	switch size {
	case 1:
		return !collision.CanMove(v.flags.Get(x, z+1, level), collision.FlagBlockNorth|extraFlag, collisionType)
	case 2:
		return !collision.CanMove(v.flags.Get(x, z+2, level), collision.FlagBlockNorthWest|extraFlag, collisionType) ||
			!collision.CanMove(v.flags.Get(x+1, z+2, level), collision.FlagBlockNorthEast|extraFlag, collisionType)
	default:
		if !collision.CanMove(v.flags.Get(x, z+size, level), collision.FlagBlockNorthWest|extraFlag, collisionType) {
			return true
		}
		if !collision.CanMove(v.flags.Get(x+size-1, z+size, level), collision.FlagBlockNorthEast|extraFlag, collisionType) {
			return true
		}
		for midX := x + 1; midX < x+size-1; midX++ {
			if !collision.CanMove(v.flags.Get(midX, z+size, level), collision.FlagBlockSouthEastAndWest|extraFlag, collisionType) {
				return true
			}
		}
		return false
	}
}

func (v StepValidator) isBlockedWest(level, x, z, size, extraFlag int, collisionType collision.Type) bool {
	switch size {
	case 1:
		return !collision.CanMove(v.flags.Get(x-1, z, level), collision.FlagBlockWest|extraFlag, collisionType)
	case 2:
		return !collision.CanMove(v.flags.Get(x-1, z, level), collision.FlagBlockSouthWest|extraFlag, collisionType) ||
			!collision.CanMove(v.flags.Get(x-1, z+1, level), collision.FlagBlockNorthWest|extraFlag, collisionType)
	default:
		if !collision.CanMove(v.flags.Get(x-1, z, level), collision.FlagBlockSouthWest|extraFlag, collisionType) {
			return true
		}
		if !collision.CanMove(v.flags.Get(x-1, z+size-1, level), collision.FlagBlockNorthWest|extraFlag, collisionType) {
			return true
		}
		for midZ := z + 1; midZ < z+size-1; midZ++ {
			if !collision.CanMove(v.flags.Get(x-1, midZ, level), collision.FlagBlockNorthAndSouthEast|extraFlag, collisionType) {
				return true
			}
		}
		return false
	}
}

func (v StepValidator) isBlockedEast(level, x, z, size, extraFlag int, collisionType collision.Type) bool {
	switch size {
	case 1:
		return !collision.CanMove(v.flags.Get(x+1, z, level), collision.FlagBlockEast|extraFlag, collisionType)
	case 2:
		return !collision.CanMove(v.flags.Get(x+2, z, level), collision.FlagBlockSouthEast|extraFlag, collisionType) ||
			!collision.CanMove(v.flags.Get(x+2, z+1, level), collision.FlagBlockNorthEast|extraFlag, collisionType)
	default:
		if !collision.CanMove(v.flags.Get(x+size, z, level), collision.FlagBlockSouthEast|extraFlag, collisionType) {
			return true
		}
		if !collision.CanMove(v.flags.Get(x+size, z+size-1, level), collision.FlagBlockNorthEast|extraFlag, collisionType) {
			return true
		}
		for midZ := z + 1; midZ < z+size-1; midZ++ {
			if !collision.CanMove(v.flags.Get(x+size, midZ, level), collision.FlagBlockNorthAndSouthWest|extraFlag, collisionType) {
				return true
			}
		}
		return false
	}
}

func (v StepValidator) isBlockedSouthWest(level, x, z, size, extraFlag int, collisionType collision.Type) bool {
	switch size {
	case 1:
		return !collision.CanMove(v.flags.Get(x-1, z-1, level), collision.FlagBlockSouthWest|extraFlag, collisionType) ||
			!collision.CanMove(v.flags.Get(x-1, z, level), collision.FlagBlockWest|extraFlag, collisionType) ||
			!collision.CanMove(v.flags.Get(x, z-1, level), collision.FlagBlockSouth|extraFlag, collisionType)
	case 2:
		return !collision.CanMove(v.flags.Get(x-1, z, level), collision.FlagBlockNorthAndSouthEast|extraFlag, collisionType) ||
			!collision.CanMove(v.flags.Get(x-1, z-1, level), collision.FlagBlockSouthWest|extraFlag, collisionType) ||
			!collision.CanMove(v.flags.Get(x, z-1, level), collision.FlagBlockNorthEastAndWest|extraFlag, collisionType)
	default:
		if !collision.CanMove(v.flags.Get(x-1, z-1, level), collision.FlagBlockSouthWest|extraFlag, collisionType) {
			return true
		}
		for mid := 1; mid < size; mid++ {
			if !collision.CanMove(v.flags.Get(x-1, z+mid-1, level), collision.FlagBlockNorthAndSouthEast|extraFlag, collisionType) {
				return true
			}
			if !collision.CanMove(v.flags.Get(x+mid-1, z-1, level), collision.FlagBlockNorthEastAndWest|extraFlag, collisionType) {
				return true
			}
		}
		return false
	}
}

func (v StepValidator) isBlockedNorthWest(level, x, z, size, extraFlag int, collisionType collision.Type) bool {
	switch size {
	case 1:
		return !collision.CanMove(v.flags.Get(x-1, z+1, level), collision.FlagBlockNorthWest|extraFlag, collisionType) ||
			!collision.CanMove(v.flags.Get(x-1, z, level), collision.FlagBlockWest|extraFlag, collisionType) ||
			!collision.CanMove(v.flags.Get(x, z+1, level), collision.FlagBlockNorth|extraFlag, collisionType)
	case 2:
		return !collision.CanMove(v.flags.Get(x-1, z+1, level), collision.FlagBlockNorthAndSouthEast|extraFlag, collisionType) ||
			!collision.CanMove(v.flags.Get(x-1, z+2, level), collision.FlagBlockNorthWest|extraFlag, collisionType) ||
			!collision.CanMove(v.flags.Get(x, z+2, level), collision.FlagBlockSouthEastAndWest|extraFlag, collisionType)
	default:
		if !collision.CanMove(v.flags.Get(x-1, z+size, level), collision.FlagBlockNorthWest|extraFlag, collisionType) {
			return true
		}
		for mid := 1; mid < size; mid++ {
			if !collision.CanMove(v.flags.Get(x-1, z+mid, level), collision.FlagBlockNorthAndSouthEast|extraFlag, collisionType) {
				return true
			}
			if !collision.CanMove(v.flags.Get(x+mid-1, z+size, level), collision.FlagBlockSouthEastAndWest|extraFlag, collisionType) {
				return true
			}
		}
		return false
	}
}

func (v StepValidator) isBlockedSouthEast(level, x, z, size, extraFlag int, collisionType collision.Type) bool {
	switch size {
	case 1:
		return !collision.CanMove(v.flags.Get(x+1, z-1, level), collision.FlagBlockSouthEast|extraFlag, collisionType) ||
			!collision.CanMove(v.flags.Get(x+1, z, level), collision.FlagBlockEast|extraFlag, collisionType) ||
			!collision.CanMove(v.flags.Get(x, z-1, level), collision.FlagBlockSouth|extraFlag, collisionType)
	case 2:
		return !collision.CanMove(v.flags.Get(x+1, z-1, level), collision.FlagBlockNorthEastAndWest|extraFlag, collisionType) ||
			!collision.CanMove(v.flags.Get(x+2, z-1, level), collision.FlagBlockSouthEast|extraFlag, collisionType) ||
			!collision.CanMove(v.flags.Get(x+2, z, level), collision.FlagBlockNorthAndSouthWest|extraFlag, collisionType)
	default:
		for mid := 1; mid < size; mid++ {
			if !collision.CanMove(v.flags.Get(x+size, z+mid-1, level), collision.FlagBlockNorthAndSouthWest|extraFlag, collisionType) {
				return true
			}
			if !collision.CanMove(v.flags.Get(x+mid, z-1, level), collision.FlagBlockNorthEastAndWest|extraFlag, collisionType) {
				return true
			}
		}
		return false
	}
}

func (v StepValidator) isBlockedNorthEast(level, x, z, size, extraFlag int, collisionType collision.Type) bool {
	switch size {
	case 1:
		return !collision.CanMove(v.flags.Get(x+1, z+1, level), collision.FlagBlockNorthEast|extraFlag, collisionType) ||
			!collision.CanMove(v.flags.Get(x+1, z, level), collision.FlagBlockEast|extraFlag, collisionType) ||
			!collision.CanMove(v.flags.Get(x, z+1, level), collision.FlagBlockNorth|extraFlag, collisionType)
	case 2:
		return !collision.CanMove(v.flags.Get(x+1, z+2, level), collision.FlagBlockSouthEastAndWest|extraFlag, collisionType) ||
			!collision.CanMove(v.flags.Get(x+2, z+2, level), collision.FlagBlockNorthEast|extraFlag, collisionType) ||
			!collision.CanMove(v.flags.Get(x+2, z+1, level), collision.FlagBlockNorthAndSouthWest|extraFlag, collisionType)
	default:
		if !collision.CanMove(v.flags.Get(x+size, z+size, level), collision.FlagBlockNorthEast|extraFlag, collisionType) {
			return true
		}
		for mid := 1; mid < size; mid++ {
			if !collision.CanMove(v.flags.Get(x+mid, z+size, level), collision.FlagBlockSouthEastAndWest|extraFlag, collisionType) {
				return true
			}
			if !collision.CanMove(v.flags.Get(x+size, z+mid, level), collision.FlagBlockNorthAndSouthWest|extraFlag, collisionType) {
				return true
			}
		}
		return false
	}
}
