// Package script — handlers for ServerOps map opcodes (MAP_PLAYERCOUNT
// and the family being landed in NAI-35).
package script

// handleMapPlayerCount (MAP_PLAYERCOUNT, opcode 1015) pops two coords
// (rect bounds) and pushes the count of players whose (x, z) falls
// inside the rect on from.level. Mirrors TS ServerOps.ts:27-45.
//
// Pop order: top-of-stack is c2; c1 is below. TS popInts(2) returns
// [c1, c2]. NAI-35-D1: TS uses from.level for inner getZone with no
// to.level validation; cross-level rect silently iterates only
// from.level zones. goscape mirrors.
func handleMapPlayerCount(s *ScriptState) error {
	c2 := s.PopInt()
	c1 := s.PopInt()

	fromLevel, fromX, fromZ, err := checkCoord(c1, "MAP_PLAYERCOUNT")
	if err != nil {
		return err
	}
	_, toX, toZ, err := checkCoord(c2, "MAP_PLAYERCOUNT")
	if err != nil {
		return err
	}

	if s.PlayerLookup == nil {
		s.PushInt(0)
		return nil
	}

	count := 0
	// Zone iteration: from floor(fromX/8) to ceil(toX/8), inclusive.
	// (fromX >> 3) is floor; (toX + 7) >> 3 is the equivalent of ceil
	// for positive coords.
	for zx := fromX >> 3; zx <= (toX+7)>>3; zx++ {
		for zz := fromZ >> 3; zz <= (toZ+7)>>3; zz++ {
			for _, p := range s.PlayerLookup.ZonePlayers(fromLevel, zx<<3, zz<<3) {
				if p.X() >= fromX && p.X() <= toX && p.Z() >= fromZ && p.Z() <= toZ {
					count++
				}
			}
		}
	}
	s.PushInt(count)
	return nil
}
