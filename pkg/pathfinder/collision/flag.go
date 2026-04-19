package collision

const (
	FlagNull int = -1
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

	// FlagBlockNPCs is a custom flag dedicated to blocking NPCs.
	// It should be noted that this is a custom flag, and you do not need to use this.
	// The routefinder takes the flag as a custom option, so you may use any other flag, this just defines
	// a reliable constant to use.
	FlagBlockNPCs

	// FlagBlockPlayers is a custom flag dedicated to blocking players, projectiles and NPCs.
	// An example of a monster to set this flag is Brawler. Note that it is unclear if this flag
	// prevents NPCs, as there is a separate flag option for it.
	// This flag is similar to the one above, except it's strictly for NPCs.
	FlagBlockPlayers

	FlagBlockWalk
	FlagWallNorthWestRouteBlocker
	FlagWallNorthRouteBlocker
	FlagWallNorthEastRouteBlocker
	FlagWallEastRouteBlocker
	FlagWallSouthEastRouteBlocker
	FlagWallSouthRouteBlocker
	FlagWallSouthWestRouteBlocker
	FlagWallWestRouteBlocker
	FlagLocRouteBlocker

	// FlagRoof is used to bind NPCs to not leave the buildings they spawn in.
	// This is a custom flag.
	FlagRoof
)

const (
	// FlagFloorBlocked is a shorthand combination of both floor flags.
	FlagFloorBlocked = FlagBlockWalk | FlagGroundDecor

	FlagWalkBlocked = FlagLoc | FlagFloorBlocked // TODO: missing in kt

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

	// Route blocker flags. These are used in ~550+ clients to generate paths through bankers and such.

	FlagBlockWestRouteBlocker              = FlagWallEastRouteBlocker | FlagLocRouteBlocker | FlagFloorBlocked
	FlagBlockEastRouteBlocker              = FlagWallWestRouteBlocker | FlagLocRouteBlocker | FlagFloorBlocked
	FlagBlockSouthRouteBlocker             = FlagWallNorthRouteBlocker | FlagLocRouteBlocker | FlagFloorBlocked
	FlagBlockNorthRouteBlocker             = FlagWallSouthRouteBlocker | FlagLocRouteBlocker | FlagFloorBlocked
	FlagBlockSouthWestRouteBlocker         = FlagWallNorthRouteBlocker | FlagWallNorthEastRouteBlocker | FlagWallEastRouteBlocker | FlagLocRouteBlocker | FlagFloorBlocked
	FlagBlockSouthEastRouteBlocker         = FlagWallNorthWestRouteBlocker | FlagWallNorthRouteBlocker | FlagWallWestRouteBlocker | FlagLocRouteBlocker | FlagFloorBlocked
	FlagBlockNorthWestRouteBlocker         = FlagWallEastRouteBlocker | FlagWallSouthEastRouteBlocker | FlagWallSouthRouteBlocker | FlagLocRouteBlocker | FlagFloorBlocked
	FlagBlockNorthEastRouteBlocker         = FlagWallSouthRouteBlocker | FlagWallSouthWestRouteBlocker | FlagWallWestRouteBlocker | FlagLocRouteBlocker | FlagFloorBlocked
	FlagBlockNorthAndSouthEastRouteBlocker = FlagWallNorthRouteBlocker | FlagWallNorthEastRouteBlocker | FlagWallEastRouteBlocker | FlagWallSouthEastRouteBlocker | FlagWallSouthRouteBlocker | FlagLocRouteBlocker | FlagFloorBlocked
	FlagBlockNorthAndSouthWestRouteBlocker = FlagWallNorthWestRouteBlocker | FlagWallNorthRouteBlocker | FlagWallSouthRouteBlocker | FlagWallSouthWestRouteBlocker | FlagWallWestRouteBlocker | FlagLocRouteBlocker | FlagFloorBlocked
	FlagBlockNorthEastAndWestRouteBlocker  = FlagWallNorthWestRouteBlocker | FlagWallNorthRouteBlocker | FlagWallNorthEastRouteBlocker | FlagWallEastRouteBlocker | FlagWallWestRouteBlocker | FlagLocRouteBlocker | FlagFloorBlocked
	FlagBlockSouthEastAndWestRouteBlocker  = FlagWallEastRouteBlocker | FlagWallSouthEastRouteBlocker | FlagWallSouthRouteBlocker | FlagWallSouthWestRouteBlocker | FlagWallWestRouteBlocker | FlagLocRouteBlocker | FlagFloorBlocked
)
