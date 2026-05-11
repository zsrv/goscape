// Package script — handlers for ServerOps map opcodes (MAP_PLAYERCOUNT
// and the family being landed in NAI-35).
package script

import (
	"errors"
	"fmt"
	"math/rand/v2"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/loc"
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

// isLineOfWalk delegates to s.LineValidator. Mirrors TS
// GameMap.ts:425-427: rsmod.hasLineOfWalk(level, sX, sZ, dX, dZ, 1, 1, 1, 1, 0).
// goscape's srcSize collapses TS srcWidth+srcHeight (both 1) into a single
// arg via RayCast's `srcSize, srcSize` (linevalidator.go:21); destWidth and
// destLength are passed verbatim. NAI-165-D-LOW-ARG-SHAPE-FIX widens this
// wrapper from the pre-fix (1, 0, 0, 0) shape to TS-faithful (1, 1, 1, 0);
// existing MapFindSquareLineOfWalk callers at lines 117, 147 inherit the
// corrected endpoint semantics. Pessimistic-allow on nil validator.
// NAI-35-T6 (NAI-165). NAI-166-D-LOW-ARG-SHAPE-SWEEP retired the
// iterator-side stragglers in pkg/script/player_iterator.go and
// pkg/script/npc_iterator.go (the modules/world hunt sites were already
// canonical at HEAD when NAI-166 opened).
func isLineOfWalk(s *ScriptState, level, srcX, srcZ, destX, destZ int) bool {
	if s.LineValidator == nil {
		return true
	}
	return s.LineValidator.HasLineOfWalk(level, srcX, srcZ, destX, destZ, 1, 1, 1, 0)
}

// isLineOfSight delegates to s.LineValidator. Mirrors TS
// GameMap.ts:429-431: rsmod.hasLineOfSight(level, sX, sZ, dX, dZ, 1, 1, 1, 1, 0).
// goscape's srcSize collapses TS srcWidth+srcHeight (both 1) into a single
// arg via RayCast's `srcSize, srcSize` (linevalidator.go:21); destWidth and
// destLength are passed verbatim. NAI-163-D-LOS-ARG-SHAPE-FIX widens this
// wrapper from the pre-fix (1, 0, 0, 0) shape to TS-faithful (1, 1, 1, 0);
// existing MapFindSquareLineOfSight callers at lines 119-120, 150-151
// inherit the corrected endpoint semantics. NAI-35-T6 (NAI-163 B1 T0).
func isLineOfSight(s *ScriptState, level, srcX, srcZ, destX, destZ int) bool {
	if s.LineValidator == nil {
		return true
	}
	return s.LineValidator.HasLineOfSight(level, srcX, srcZ, destX, destZ, 1, 1, 1, 0)
}

// handleLineOfSight (LINEOFSIGHT, opcode 1005) pops [from, to] coords and
// pushes 1 iff a line-of-sight ray from `from` to `to` is clear. Mirrors TS
// ServerOps.ts:144-162:
//
//	const [c1, c2] = state.popInts(2);
//	const from: CoordGrid = check(c1, CoordValid);
//	const to:   CoordGrid = check(c2, CoordValid);
//	if (from.level !== to.level) { state.pushInt(0); return; }
//	if (!NODE_MEMBERS && !World.gameMap.isFreeToPlay(to.x, to.z)) {
//	    state.pushInt(0); return;
//	}
//	state.pushInt(isLineOfSight(from.level, from.x, from.z, to.x, to.z) ? 1 : 0);
//
// Pop order (top first): c2 (to), c1 (from). Gate order pinned by tests:
// level-mismatch fires before F2P gate, which fires before LineValidator.
// nil-LineValidator inherits the wrapper's pessimistic-allow (handlers_map.go:181)
// — TS calls unconditionally; goscape's nil-guard is a defensive add (goscape
// defensive; TS skips this check). NAI-163 B1.
func handleLineOfSight(s *ScriptState) error {
	c2 := s.PopInt()
	c1 := s.PopInt()
	fromLevel, fromX, fromZ, err := checkCoord(c1, "LINEOFSIGHT")
	if err != nil {
		return err
	}
	toLevel, toX, toZ, err := checkCoord(c2, "LINEOFSIGHT")
	if err != nil {
		return err
	}
	if fromLevel != toLevel {
		s.PushInt(0)
		return nil
	}
	if s.World.MapMembers() == 0 && !s.World.IsFreeToPlay(toX, toZ) {
		s.PushInt(0)
		return nil
	}
	if isLineOfSight(s, fromLevel, fromX, fromZ, toX, toZ) {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	return nil
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

// checkSpotAnimType validates a spotanim type id by mirroring TS
// SpotAnimTypeValid (ScriptValidators.ts). Rejects negatives and
// any id not present in the SpotanimType config registry.
func checkSpotAnimType(s *ScriptState, id int, op string) error {
	if id < 0 {
		return fmt.Errorf("%s: invalid spotanim id (%d)", op, id)
	}
	if s.Configs.SpotAnimType(id) == nil {
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
	if err := checkSpotAnimType(s, spotanim, "SPOTANIM_MAP"); err != nil {
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

// handleMapLocAddUnsafe (MAP_LOCADDUNSAFE, opcode 1012) reports whether
// the input coord is occupied by an active loc that would block a new
// loc-add at that tile. Pops one packed coord; pushes 1 if any qualifying
// loc occupies the tile, else 0. Mirrors TS ServerOps.ts:212-252.
//
// Per-loc filter (TS line 218 + 224):
//
//   - LocType.Active != 1 → skip the loc entirely (no occupancy check).
//   - !loc.Active() && layer == LayerWall → skip the loc entirely
//     (goscape defensive note: TS skips inactive walls only; inactive
//     ground / ground-decor locs ARE checked).
//
// Per-layer occupancy check (TS lines 228-249):
//
//   - LayerWall (TS LocLayer.WALL): exact (x, z) match.
//   - LayerGround (TS LocLayer.GROUND): footprint covers (coord.x, coord.z),
//     where width/length are LocType.Width/Length swapped if Angle is
//     AngleNorth or AngleSouth.
//   - LayerGroundDecor (TS LocLayer.GROUND_DECOR): exact (x, z) match.
//   - LayerWallDecor: not enumerated by TS; falls through to push 0.
//
// Configs nil-handling: a nil LocType lookup silently skips the loc
// (mirrors TS check(loc.type, LocTypeValid) which would throw, but
// goscape defensive — script execution continues with the next loc to
// avoid aborting the firemaking chain on a malformed cache entry;
// goscape defensive; TS throws).
func handleMapLocAddUnsafe(s *ScriptState) error {
	coord := s.PopInt()
	level, x, z, err := checkCoord(coord, "MAP_LOCADDUNSAFE")
	if err != nil {
		return err
	}
	if s.LocOps == nil {
		s.PushInt(0)
		return nil
	}

	for _, l := range s.LocOps.AllLocsInZone(level, x, z) {
		var lt *objtype.LocType
		if s.Configs != nil {
			lt = s.Configs.LocType(l.LocType())
		}
		if lt == nil || lt.Active != 1 {
			continue
		}

		layer := l.Layer()
		if !l.Active() && layer == int(loc.LayerWall) {
			continue
		}

		lx, lz, _ := l.Coords()
		switch layer {
		case int(loc.LayerWall):
			if lx == x && lz == z {
				s.PushInt(1)
				return nil
			}
		case int(loc.LayerGround):
			width, length := lt.Width, lt.Length
			if l.Angle() == loc.AngleNorth || l.Angle() == loc.AngleSouth {
				width, length = lt.Length, lt.Width
			}
			for index := range width * length {
				deltaX := lx + (index % width)
				deltaZ := lz + (index / width)
				if deltaX == x && deltaZ == z {
					s.PushInt(1)
					return nil
				}
			}
		case int(loc.LayerGroundDecor):
			if lx == x && lz == z {
				s.PushInt(1)
				return nil
			}
		}
	}
	s.PushInt(0)
	return nil
}

// handleLineOfWalk (LINEOFWALK, opcode 1006) reports whether a 1-tile
// entity at c1 has line-of-walk to c2. Pop order: top-of-stack is c2,
// c1 below. Pushes 1 on success, 0 on fail.
//
// Same-level guard: differing levels push 0 immediately.
// F2P short-circuit: in a non-members world, destination tile not in
// an F2P zone pushes 0.
// Nil-LineValidator: routes through the isLineOfWalk wrapper
// (pessimistic-allow), matching handleLineOfSight. NAI-166-D-LOW-WRAPPER-ROUTING
// closed the prior explicit nil-guard / pessimistic-deny divergence.
//
// Arg shape: HasLineOfWalk(..., 1, 1, 1, 0) via isLineOfWalk wrapper;
// matches TS GameMap.ts:425-427. NAI-165-D-LOW-ARG-SHAPE-FIX.
//
// Mirrors TS ServerOps.ts:65-82.
func handleLineOfWalk(s *ScriptState) error {
	c2 := s.PopInt()
	c1 := s.PopInt()

	fromLevel, fromX, fromZ, err := checkCoord(c1, "LINEOFWALK")
	if err != nil {
		return err
	}
	toLevel, toX, toZ, err := checkCoord(c2, "LINEOFWALK")
	if err != nil {
		return err
	}
	if fromLevel != toLevel {
		s.PushInt(0)
		return nil
	}
	if s.World.MapMembers() == 0 && !s.World.IsFreeToPlay(toX, toZ) {
		s.PushInt(0)
		return nil
	}
	if isLineOfWalk(s, fromLevel, fromX, fromZ, toX, toZ) {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	return nil
}

// handleMapMultiway (MAP_MULTIWAY, opcode 1014) reports whether the tile at
// the popped coord is in a multi-combat zone. Mirrors TS ServerOps.ts:376-380:
//
//	state.pushInt(World.gameMap.isMulti(coord) ? 1 : 0);
//
// TS does NOT call CoordValid on the coord (unlike MAP_BLOCKED). Goscape
// matches: pass the unpacked coord directly to WorldVars.IsMulti. Nil-World
// returns an error (goscape defensive; TS always has a live World). NAI-120
// Bundle 2A.
func handleMapMultiway(s *ScriptState) error {
	coord := s.PopInt()
	if s.World == nil {
		return errors.New("MAP_MULTIWAY: no world surface")
	}
	level, x, z := unpackCoord(coord)
	if s.World.IsMulti(level, x, z) {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	return nil
}
