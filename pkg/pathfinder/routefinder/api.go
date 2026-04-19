package routefinder

import (
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/pathfinder/loc"
)

//var Flags = collision.NewFlagMap()
//var RouteFinder = routefinder.NewPathFinderAPI(Flags, routefinder.routefinderDefaultSearchMapSize, routefinder.routefinderDefaultRingBufferSize)
//var StepValidator = routefinder.NewStepValidator(Flags)
//var LineValidator = routefinder.NewLineValidator(Flags)
//var LineRouteFinder = routefinder.NewLineRouteFinder(Flags)
//var NaiveRouteFinder = routefinder.NewNaiveRouteFinder(StepValidator)

type PathFinderAPI struct {
	Flags            collision.FlagMap
	RouteFinder      RouteFinder
	StepValidator    StepValidator
	LineValidator    LineValidator
	LineRouteFinder  LineRouteFinder
	NaiveRouteFinder NaiveRouteFinder
}

// new one for each world
func NewPathFinderAPI() PathFinderAPI {
	var pf PathFinderAPI
	pf.Flags = collision.NewFlagMap() // TODO: this might have to be a pointer, OR pass address of it to the other funcs below
	pf.RouteFinder = NewRouteFinderDefault(pf.Flags)
	pf.StepValidator = NewStepValidator(pf.Flags)
	pf.LineValidator = NewLineValidator(pf.Flags)
	pf.LineRouteFinder = NewLineRouteFinder(pf.Flags)
	pf.NaiveRouteFinder = NewNaiveRouteFinder(pf.StepValidator)
	return pf
}

// Deprecated: replace with pf.RouteFinder.FindRouteDefault
func (pf PathFinderAPI) FindPathDefault(level, srcX, srcZ, destX, destZ int) Route {
	return pf.FindPath(level, srcX, srcZ, destX, destZ, 1, 1, 1, 0, -1, true, 0, 25, collision.TypeNormal)
}

// Deprecated
func (pf PathFinderAPI) FindPath(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, angle, shape int, moveNear bool, blockAccessFlags, maxWaypoints int, collisionType collision.Type) Route {
	return pf.RouteFinder.FindRoute(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, angle, shape, moveNear, blockAccessFlags, maxWaypoints, collisionType)
}

func (pf PathFinderAPI) ChangeFloor(x, z, level int, add bool) {
	if add {
		pf.Flags.Add(x, z, level, collision.FlagBlockWalk)
	} else {
		pf.Flags.Remove(x, z, level, collision.FlagBlockWalk)
	}
}

func (pf PathFinderAPI) ChangeLoc(x, z, level, width, length int, blockRange, breakRouteFinding, add bool) {
	mask := collision.FlagLoc
	if blockRange {
		mask |= collision.FlagLocProjBlocker
	}
	if breakRouteFinding {
		mask |= collision.FlagLocRouteBlocker
	}
	for index := 0; index < width*length; index++ {
		deltaX := x + (index % width)
		deltaZ := z + index/width
		if add {
			pf.Flags.Add(deltaX, deltaZ, level, mask)
		} else {
			pf.Flags.Remove(deltaX, deltaZ, level, mask)
		}
	}
}

func (pf PathFinderAPI) ChangeNPC(x, z, level, size int, add bool) {
	mask := collision.FlagBlockNPCs
	for index := 0; index < size*size; index++ {
		deltaX := x + (index % size)
		deltaZ := z + index/size
		if add {
			pf.Flags.Add(deltaX, deltaZ, level, mask)
		} else {
			pf.Flags.Remove(deltaX, deltaZ, level, mask)
		}
	}
}

func (pf PathFinderAPI) ChangePlayer(x, z, level, size int, add bool) {
	mask := collision.FlagBlockPlayers
	for index := 0; index < size*size; index++ {
		deltaX := x + (index % size)
		deltaZ := z + index/size
		if add {
			pf.Flags.Add(deltaX, deltaZ, level, mask)
		} else {
			pf.Flags.Remove(deltaX, deltaZ, level, mask)
		}
	}
}

func (pf PathFinderAPI) ChangeRoof(x, z, level int, add bool) {
	if add {
		pf.Flags.Add(x, z, level, collision.FlagRoof)
	} else {
		pf.Flags.Remove(x, z, level, collision.FlagRoof)
	}
}

func (pf PathFinderAPI) ChangeWall(x, z, level, angle, shape int, blockRange, breakRouteFinding, add bool) {
	switch loc.Shape(shape) {
	case loc.ShapeWallStraight:
		pf.changeWallStraight(x, z, level, angle, blockRange, breakRouteFinding, add)
	case loc.ShapeWallDiagonalCorner, loc.ShapeWallSquareCorner:
		pf.changeWallCorner(x, z, level, angle, blockRange, breakRouteFinding, add)
	case loc.ShapeWallL:
		pf.changeWallL(x, z, level, angle, blockRange, breakRouteFinding, add)
	default:
		panic("unsupported Shape")
	}
}

func (pf PathFinderAPI) changeWallStraight(x, z, level, angle int, blockRange, breakRouteFinding, add bool) {
	var west, east, north, south int
	if breakRouteFinding {
		west = collision.FlagWallWestRouteBlocker
		east = collision.FlagWallEastRouteBlocker
		north = collision.FlagWallNorthRouteBlocker
		south = collision.FlagWallSouthRouteBlocker
	} else if blockRange {
		west = collision.FlagWallWestProjBlocker
		east = collision.FlagWallEastProjBlocker
		north = collision.FlagWallNorthProjBlocker
		south = collision.FlagWallSouthProjBlocker
	} else {
		west = collision.FlagWallWest
		east = collision.FlagWallEast
		north = collision.FlagWallNorth
		south = collision.FlagWallSouth
	}

	switch angle {
	case loc.AngleWest:
		if add {
			pf.Flags.Add(x, z, level, west)
			pf.Flags.Add(x-1, z, level, east)
		} else {
			pf.Flags.Remove(x, z, level, west)
			pf.Flags.Remove(x-1, z, level, east)
		}
	case loc.AngleNorth:
		if add {
			pf.Flags.Add(x, z, level, north)
			pf.Flags.Add(x, z+1, level, south)
		} else {
			pf.Flags.Remove(x, z, level, north)
			pf.Flags.Remove(x, z+1, level, south)
		}
	case loc.AngleEast:
		if add {
			pf.Flags.Add(x, z, level, east)
			pf.Flags.Add(x+1, z, level, west)
		} else {
			pf.Flags.Remove(x, z, level, east)
			pf.Flags.Remove(x+1, z, level, west)
		}
	case loc.AngleSouth:
		if add {
			pf.Flags.Add(x, z, level, south)
			pf.Flags.Add(x, z-1, level, north)
		} else {
			pf.Flags.Remove(x, z, level, south)
			pf.Flags.Remove(x, z-1, level, north)
		}
	}

	if breakRouteFinding {
		pf.changeWallStraight(x, z, level, angle, blockRange, false, add)
		return
	}
	if blockRange {
		// If just blocked projectiles, then block normally next
		pf.changeWallStraight(x, z, level, angle, false, false, add)
		return
	}
}

func (pf PathFinderAPI) changeWallCorner(x, z, level, angle int, blockRange, breakRouteFinding, add bool) {
	var northwest, southeast, northeast, southwest int
	if breakRouteFinding {
		northwest = collision.FlagWallNorthWestRouteBlocker
		southeast = collision.FlagWallSouthEastRouteBlocker
		northeast = collision.FlagWallNorthEastRouteBlocker
		southwest = collision.FlagWallSouthWestRouteBlocker
	} else if blockRange {
		northwest = collision.FlagWallNorthWestProjBlocker
		southeast = collision.FlagWallSouthEastProjBlocker
		northeast = collision.FlagWallNorthEastProjBlocker
		southwest = collision.FlagWallSouthWestProjBlocker
	} else {
		northwest = collision.FlagWallNorthWest
		southeast = collision.FlagWallSouthEast
		northeast = collision.FlagWallNorthEast
		southwest = collision.FlagWallSouthWest
	}

	switch angle {
	case loc.AngleWest:
		if add {
			pf.Flags.Add(x, z, level, northwest)
			pf.Flags.Add(x-1, z+1, level, southeast)
		} else {
			pf.Flags.Remove(x, z, level, northwest)
			pf.Flags.Remove(x-1, z+1, level, southeast)
		}
	case loc.AngleNorth:
		if add {
			pf.Flags.Add(x, z, level, northeast)
			pf.Flags.Add(x+1, z+1, level, southwest)
		} else {
			pf.Flags.Remove(x, z, level, northeast)
			pf.Flags.Remove(x+1, z+1, level, southwest)
		}
	case loc.AngleEast:
		if add {
			pf.Flags.Add(x, z, level, southeast)
			pf.Flags.Add(x+1, z-1, level, northwest)
		} else {
			pf.Flags.Remove(x, z, level, southeast)
			pf.Flags.Remove(x+1, z-1, level, northwest)
		}
	case loc.AngleSouth:
		if add {
			pf.Flags.Add(x, z, level, southwest)
			pf.Flags.Add(x-1, z-1, level, northeast)
		} else {
			pf.Flags.Remove(x, z, level, southwest)
			pf.Flags.Remove(x-1, z-1, level, northeast)
		}
	}

	if breakRouteFinding {
		pf.changeWallCorner(x, z, level, angle, blockRange, false, add)
		return
	}
	if blockRange {
		// If just blocked projectiles, then block normally next
		pf.changeWallCorner(x, z, level, angle, false, false, add)
		return
	}
}

func (pf PathFinderAPI) changeWallL(x, z, level, angle int, blockRange, breakRouteFinding, add bool) {
	var west, east, north, south int
	if breakRouteFinding {
		west = collision.FlagWallWestRouteBlocker
		east = collision.FlagWallEastRouteBlocker
		north = collision.FlagWallNorthRouteBlocker
		south = collision.FlagWallSouthRouteBlocker
	} else if blockRange {
		west = collision.FlagWallWestProjBlocker
		east = collision.FlagWallEastProjBlocker
		north = collision.FlagWallNorthProjBlocker
		south = collision.FlagWallSouthProjBlocker
	} else {
		west = collision.FlagWallWest
		east = collision.FlagWallEast
		north = collision.FlagWallNorth
		south = collision.FlagWallSouth
	}

	switch angle {
	case loc.AngleWest:
		if add {
			pf.Flags.Add(x, z, level, north|west)
			pf.Flags.Add(x-1, z, level, east)
			pf.Flags.Add(x, z+1, level, south)
		} else {
			pf.Flags.Remove(x, z, level, north|west)
			pf.Flags.Remove(x-1, z, level, east)
			pf.Flags.Remove(x, z+1, level, south)
		}
	case loc.AngleNorth:
		if add {
			pf.Flags.Add(x, z, level, north|east)
			pf.Flags.Add(x, z+1, level, south)
			pf.Flags.Add(x+1, z, level, west)
		} else {
			pf.Flags.Remove(x, z, level, north|east)
			pf.Flags.Remove(x, z+1, level, south)
			pf.Flags.Remove(x+1, z, level, west)
		}
	case loc.AngleEast:
		if add {
			pf.Flags.Add(x, z, level, south|east)
			pf.Flags.Add(x+1, z, level, west)
			pf.Flags.Add(x, z-1, level, north)
		} else {
			pf.Flags.Remove(x, z, level, south|east)
			pf.Flags.Remove(x+1, z, level, west)
			pf.Flags.Remove(x, z-1, level, north)
		}
	case loc.AngleSouth:
		if add {
			pf.Flags.Add(x, z, level, south|west)
			pf.Flags.Add(x, z-1, level, north)
			pf.Flags.Add(x-1, z, level, east)
		} else {
			pf.Flags.Remove(x, z, level, south|west)
			pf.Flags.Remove(x, z-1, level, north)
			pf.Flags.Remove(x-1, z, level, east)
		}
	}

	if breakRouteFinding {
		pf.changeWallL(x, z, level, angle, blockRange, false, add)
		return
	}
	if blockRange {
		// If just blocked projectiles, then block normally next
		pf.changeWallL(x, z, level, angle, false, false, add)
		return
	}
}

// Deprecated: use loc.LayerOf
func LocShapeLayer(shape loc.Shape) loc.Layer {
	return 0
}
