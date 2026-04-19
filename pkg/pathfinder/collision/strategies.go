package collision

const lineOfSightBlockMovement = FlagWallNorthWest |
	FlagWallNorth |
	FlagWallNorthEast |
	FlagWallEast |
	FlagWallSouthEast |
	FlagWallSouth |
	FlagWallSouthWest |
	FlagWallWest |
	FlagLoc

const lineOfSightBlockRoute = FlagWallNorthWestRouteBlocker |
	FlagWallNorthRouteBlocker |
	FlagWallNorthEastRouteBlocker |
	FlagWallEastRouteBlocker |
	FlagWallSouthEastRouteBlocker |
	FlagWallSouthRouteBlocker |
	FlagWallSouthWestRouteBlocker |
	FlagWallWestRouteBlocker |
	FlagLocRouteBlocker

func CanMove(tileFlag int, blockFlag int, collisionType Type) bool {
	switch collisionType {
	case TypeNormal:
		return tileFlag&blockFlag == FlagOpen
	case TypeBlocked:
		f := blockFlag & ^FlagBlockWalk
		return (tileFlag&f) == 0 && (tileFlag&FlagBlockWalk) != FlagOpen
	case TypeIndoors:
		return (tileFlag&blockFlag) == 0 && (tileFlag&FlagRoof) != FlagOpen
	case TypeOutdoors:
		return (tileFlag & (blockFlag | FlagRoof)) == FlagOpen
	case TypeLineOfSight:
		movementFlags := (blockFlag & lineOfSightBlockMovement) << 9
		routeFlags := (blockFlag & lineOfSightBlockRoute) >> 13
		finalBlockFlag := movementFlags | routeFlags
		return (tileFlag & finalBlockFlag) == FlagOpen
	default:
		// TODO: Error
		panic("unknown collision type")
	}
}
