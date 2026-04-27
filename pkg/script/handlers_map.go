// Package script — handlers for ServerOps map opcodes (MAP_PLAYERCOUNT
// and the family being landed in NAI-35).
package script

import (
	"fmt"
	"math/rand/v2"

	"github.com/zsrv/goscape/pkg/coordgrid"
)

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

// handleMapFindSquare (MAP_FINDSQUARE, opcode 1009) finds a free walkable
// square near origin, optionally gated by line-of-walk or line-of-sight.
// Mirrors TS ServerOps.ts:254-374.
//
// Pop order (top-of-stack first): type, maxRadius, minRadius, coord.
// TS popInts(4) returns [coord, minRadius, maxRadius, type] so type is
// at top of stack. Validation order matches TS line 256-259:
// NumberPositive(min), NumberPositive(max), FindSquareValid(type),
// CoordValid(coord). On hit: pushes packed coord. On exhaustion: pushes
// the input coord (TS line 373 fall-through).
//
// NAI-35-D4: uses math/rand/v2 (TS uses Math.random); behaviorally
// equivalent for non-deterministic per-call random.
func handleMapFindSquare(s *ScriptState) error {
	typeArg := s.PopInt()
	maxRadius := s.PopInt()
	minRadius := s.PopInt()
	coord := s.PopInt()

	if err := checkNumberPositive(minRadius, "MAP_FINDSQUARE"); err != nil {
		return err
	}
	if err := checkNumberPositive(maxRadius, "MAP_FINDSQUARE"); err != nil {
		return err
	}
	if err := checkFindSquareType(typeArg, "MAP_FINDSQUARE"); err != nil {
		return err
	}
	level, originX, originZ, err := checkCoord(coord, "MAP_FINDSQUARE")
	if err != nil {
		return err
	}

	if s.World == nil {
		s.PushInt(coord)
		return nil
	}
	freeWorld := s.World.MapMembers() == 0
	findType := MapFindSquareType(typeArg)

	if maxRadius < 10 {
		// Random-50-attempts branch (TS lines 261-316).
		for range 50 {
			distX := rand.IntN(2*maxRadius+1) - maxRadius
			distZ := rand.IntN(2*maxRadius+1) - maxRadius
			distance := absMax(distX, distZ)
			if distance < minRadius || distance > maxRadius {
				continue
			}
			randomX := originX + distX
			randomZ := originZ + distZ
			if freeWorld && !s.World.IsFreeToPlay(randomX, randomZ) {
				continue
			}
			ok := false
			switch findType {
			case MapFindSquareNone:
				ok = !s.World.IsMapBlocked(level, randomX, randomZ)
			case MapFindSquareLineOfWalk:
				ok = isLineOfWalk(s, level, randomX, randomZ, originX, originZ) &&
					!s.World.IsMapBlocked(level, randomX, randomZ)
			case MapFindSquareLineOfSight:
				ok = isLineOfSight(s, level, randomX, randomZ, originX, originZ) &&
					!s.World.IsMapBlocked(level, randomX, randomZ)
			}
			if ok {
				s.PushInt(coordgrid.PackCoord(level, randomX, randomZ))
				return nil
			}
		}
	} else {
		// West-bias iteration branch (imps; TS lines 317-370).
		for x := originX - maxRadius; x <= originX+maxRadius; x++ {
			distX := x - originX
			distZ := rand.IntN(2*maxRadius+1) - maxRadius
			distance := absMax(distX, distZ)
			if distance < minRadius || distance > maxRadius {
				continue
			}
			randomZ := originZ + distZ
			if freeWorld && !s.World.IsFreeToPlay(x, randomZ) {
				continue
			}
			ok := false
			switch findType {
			case MapFindSquareNone:
				ok = !s.World.IsMapBlocked(level, x, randomZ) &&
					!coordgrid.IsWithinDistanceSW(x, randomZ, originX, originZ, minRadius)
			case MapFindSquareLineOfWalk:
				ok = isLineOfWalk(s, level, x, randomZ, originX, originZ) &&
					!s.World.IsMapBlocked(level, x, randomZ) &&
					!coordgrid.IsWithinDistanceSW(x, randomZ, originX, originZ, minRadius)
			case MapFindSquareLineOfSight:
				ok = isLineOfSight(s, level, x, randomZ, originX, originZ) &&
					!s.World.IsMapBlocked(level, x, randomZ) &&
					!coordgrid.IsWithinDistanceSW(x, randomZ, originX, originZ, minRadius)
			}
			if ok {
				s.PushInt(coordgrid.PackCoord(level, x, randomZ))
				return nil
			}
		}
	}

	s.PushInt(coord)
	return nil
}

// isLineOfWalk delegates to s.LineValidator. Pessimistic-allow on nil
// validator (matches NpcIterator passesFilter HuntAll behavior). Calls
// HasLineOfWalk with src=(srcX,srcZ), dest=(destX,destZ); the goscape
// convention uses srcSize=1, destWidth=0, destLength=0, extraFlag=0,
// matching player_iterator.go and npc_iterator.go. NAI-35-T6.
func isLineOfWalk(s *ScriptState, level, srcX, srcZ, destX, destZ int) bool {
	if s.LineValidator == nil {
		return true
	}
	return s.LineValidator.HasLineOfWalk(level, srcX, srcZ, destX, destZ, 1, 0, 0, 0)
}

// isLineOfSight delegates to s.LineValidator. See isLineOfWalk for arg-shape
// rationale. NAI-35-T6.
func isLineOfSight(s *ScriptState, level, srcX, srcZ, destX, destZ int) bool {
	if s.LineValidator == nil {
		return true
	}
	return s.LineValidator.HasLineOfSight(level, srcX, srcZ, destX, destZ, 1, 0, 0, 0)
}

// handleMapBlocked (MAP_BLOCKED, opcode 1007) reports whether the tile at
// the unpacked coord blocks walking. F2P-world short-circuit: any tile
// that's not F2P-zoned pushes 1 (effectively "blocked" for non-members
// content). Mirrors TS ServerOps.ts:129-138.
func handleMapBlocked(s *ScriptState) error {
	coord := s.PopInt()
	level, x, z, err := checkCoord(coord, "MAP_BLOCKED")
	if err != nil {
		return err
	}
	// F2P-world gate: !NODE_MEMBERS && !isFreeToPlay → push 1
	if s.World.MapMembers() == 0 && !s.World.IsFreeToPlay(x, z) {
		s.PushInt(1)
		return nil
	}
	if s.World.IsMapBlocked(level, x, z) {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	return nil
}

// checkSpotAnimType validates a spotanim type id. Per NAI-36-D2: full
// SpotAnimType config-port is absent at HEAD; fall back to range
// validation (id < 0 rejected). When a SpotAnimType config accessor lands
// on the Configs interface, this helper should be tightened to mirror TS
// SpotAnimTypeValid (presence check against config table).
func checkSpotAnimType(id int, op string) error {
	if id < 0 {
		return fmt.Errorf("%s: invalid spotanim id (%d)", op, id)
	}
	return nil
}

// handleSpotAnimMap (SPOTANIM_MAP, opcode 1020) broadcasts a tile-anchored
// spotanim event at the unpacked coord. Pop order: 4 ints (spotanim, coord,
// height, delay) — TS uses popInts(4) which destructures top-down.
// Mirrors TS ServerOps.ts:84-90.
func handleSpotAnimMap(s *ScriptState) error {
	delay := s.PopInt()
	height := s.PopInt()
	coord := s.PopInt()
	spotanim := s.PopInt()

	level, x, z, err := checkCoord(coord, "SPOTANIM_MAP")
	if err != nil {
		return err
	}
	if err := checkSpotAnimType(spotanim, "SPOTANIM_MAP"); err != nil {
		return err
	}
	s.World.AnimMap(level, x, z, spotanim, height, delay)
	return nil
}

// absMax returns max(|a|, |b|). Mirrors TS Math.max(Math.abs(distX), Math.abs(distZ))
// at ServerOps.ts:266 etc. NAI-35-T6.
func absMax(a, b int) int {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	return max(a, b)
}
