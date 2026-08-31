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
		// TS 8139461a dropped the route-blocker term: LINE_OF_SIGHT used to OR
		// in `(blockFlag & LINE_OF_SIGHT_ROUTE) >>> 13`, but nothing ever set
		// those bits, so the term was always zero (CollisionStrategy.ts:26-29
		// @1d25566c).
		movementFlags := (blockFlag & lineOfSightBlockMovement) << 9
		return (tileFlag & movementFlags) == FlagOpen
	default:
		panic("unknown collision type")
	}
}
