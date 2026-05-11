package collision

// IsIndoors reports whether the given tile flag carries the FlagRoof
// bit. Mirrors TS isIndoors (GameMap.ts:417-419), which calls
// isFlagged(x, z, level, CollisionFlag.ROOF). Caller is responsible
// for resolving (x, z, level) to a flag via the FlagMap.
//
// Adaptation vs plan: flag is typed as int (not uint32) to match the
// package's flag.go convention (all FlagXxx constants are int).
func IsIndoors(flag int) bool {
	return flag&FlagRoof != 0
}
