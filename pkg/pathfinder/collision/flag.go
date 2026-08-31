package collision

const (
	// FlagNull is the sentinel FlagMap.Get returns for an unallocated
	// (off-map) zone, and the value blockWalkFlag returns for an entity that
	// cannot move at all. It mirrors TS CollisionFlag.NULL = 0x7FFFFFFF
	// (@2004scape/rsmod-pathfinder): every collision bit set EXCEPT FlagRoof
	// (bit 31). An off-map tile thus blocks all movement (every walk/wall/loc
	// bit is set) yet is NOT reported as indoors — IsIndoors(off-map) is
	// false, matching TS isIndoors. Was -1 (all bits incl. bit 31), which
	// wrongly classified off-map tiles as indoors. Equality checks against
	// FlagNull (e.g. blockWalkFlag's no-move sentinel) are value-agnostic;
	// only the FlagRoof bit observes the change. Pinned by flag_test.go. L43.
	FlagNull int = 0x7FFFFFFF
	FlagOpen int = 0
)

const (
	FlagWallNorthWest int = 1 << iota
	FlagWallNorth
	FlagWallNorthEast
	FlagWallEast
	FlagWallSouthEast
	FlagWallSouth
	FlagWallSouthWest
	FlagWallWest
	FlagLoc
	FlagWallNorthWestProjBlocker
	FlagWallNorthProjBlocker
	FlagWallNorthEastProjBlocker
	FlagWallEastProjBlocker
	FlagWallSouthEastProjBlocker
	FlagWallSouthProjBlocker
	FlagWallSouthWestProjBlocker
	FlagWallWestProjBlocker
	FlagLocProjBlocker
	FlagGroundDecor

	// FlagNpcOcc marks a tile occupied by an npc. Entities that respect npc
	// occupancy include it in their block flag; an npc that sets no collision
	// of its own (blockwalk=none) opts out and walks through other npcs.
	// TS CollisionFlag.NPC_OCC (renamed from NPC by 8139461a; same bit).
	FlagNpcOcc

	// FlagBlockNpcAndPlayers is the hard block written by an entity that
	// stops both npcs and players — locs, walls, and blockwalk=all npcs. It
	// is never opted out of.
	// TS CollisionFlag.BLOCK_NPC_AND_PLAYERS (renamed from PLAYER by
	// 8139461a; same bit).
	FlagBlockNpcAndPlayers

	FlagBlockWalk
)

const (
	// FlagPlayerOcc marks a tile occupied by a player. It reuses bit 22,
	// freed when 8139461a deleted the route-blocker subsystem.
	//
	// That subsystem was write-only: it fed only CollisionType.LINE_OF_SIGHT
	// and nothing ever set its bits, because changeLocCollision hardcoded
	// breakroutefinding=false. goscape was in the same state —
	// pkg/gamemap/gamemap.go passed a literal false to every ChangeWall /
	// ChangeLoc call, and ChangeLocCollision was their only caller — so
	// removing the nine WALL_*_ROUTE_BLOCKER / LOC_ROUTE_BLOCKER flags and
	// their twelve composite masks was dead-code removal, not a behaviour
	// change.
	//
	// TS CollisionFlag.PLAYER_OCC (flags.ts:22-27 @1d25566c).
	FlagPlayerOcc int = 1 << 22

	// FlagRoof is used to bind NPCs to not leave the buildings they spawn in.
	// This is a custom flag. Pinned explicitly at bit 31: it used to fall
	// there by iota after the nine route-blocker flags, and must not drift
	// down now that they are gone.
	FlagRoof int = 1 << 31
)

const (
	// FlagFloorBlocked is a shorthand combination of both floor flags.
	FlagFloorBlocked = FlagBlockWalk | FlagGroundDecor

	FlagWalkBlocked = FlagLoc | FlagFloorBlocked

	// Mixed masks of the above flags

	FlagBlockWest  = FlagWallEast | FlagWalkBlocked
	FlagBlockEast  = FlagWallWest | FlagWalkBlocked
	FlagBlockSouth = FlagWallNorth | FlagWalkBlocked
	FlagBlockNorth = FlagWallSouth | FlagWalkBlocked

	FlagBlockSouthWest         = FlagWallNorth | FlagWallNorthEast | FlagWallEast | FlagWalkBlocked
	FlagBlockSouthEast         = FlagWallNorthWest | FlagWallNorth | FlagWallWest | FlagWalkBlocked
	FlagBlockNorthWest         = FlagWallEast | FlagWallSouthEast | FlagWallSouth | FlagWalkBlocked
	FlagBlockNorthEast         = FlagWallSouth | FlagWallSouthWest | FlagWallWest | FlagWalkBlocked
	FlagBlockNorthAndSouthEast = FlagWallNorth | FlagWallNorthEast | FlagWallEast | FlagWallSouthEast | FlagWallSouth | FlagWalkBlocked
	FlagBlockNorthAndSouthWest = FlagWallNorthWest | FlagWallNorth | FlagWallSouth | FlagWallSouthWest | FlagWallWest | FlagWalkBlocked
	FlagBlockNorthEastAndWest  = FlagWallNorthWest | FlagWallNorth | FlagWallNorthEast | FlagWallEast | FlagWallWest | FlagWalkBlocked
	FlagBlockSouthEastAndWest  = FlagWallEast | FlagWallSouthEast | FlagWallSouth | FlagWallSouthWest | FlagWallWest | FlagWalkBlocked
)
