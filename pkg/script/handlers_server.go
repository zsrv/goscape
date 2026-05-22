package script

import "fmt"

// handleMapClock pushes the server's current tick counter. TS:
// state.pushInt(World.currentTick).
func handleMapClock(s *ScriptState) error {
	if s.World == nil {
		return fmt.Errorf("MAP_CLOCK: %w", ErrNoWorld)
	}
	s.PushInt(s.World.CurrentTick())
	return nil
}

// handlePlayerCount pushes the number of players currently in the world.
// TS: state.pushInt(World.getTotalPlayers()).
func handlePlayerCount(s *ScriptState) error {
	if s.World == nil {
		return fmt.Errorf("PLAYERCOUNT: %w", ErrNoWorld)
	}
	s.PushInt(s.World.PlayerCount())
	return nil
}

// handleMapMembers pushes 1 if the server is a members world, else 0.
// TS: state.pushInt(Environment.NODE_MEMBERS ? 1 : 0).
func handleMapMembers(s *ScriptState) error {
	if s.World == nil {
		return fmt.Errorf("MAP_MEMBERS: %w", ErrNoWorld)
	}
	s.PushInt(s.World.MapMembers())
	return nil
}

// handleMapLive pushes 1 if the server is in production, else 0.
// TS: state.pushInt(Environment.NODE_PRODUCTION ? 1 : 0).
func handleMapLive(s *ScriptState) error {
	if s.World == nil {
		return fmt.Errorf("MAP_LIVE: %w", ErrNoWorld)
	}
	s.PushInt(s.World.MapLive())
	return nil
}

// handleInZone pops [from, to, pos] (pos on top) and pushes 1 if pos
// is inside the box [from..to] on all of x/z/level, else 0. Matches
// TS ServerOps.ts INZONE (axis-aligned inclusive bounds).
func handleInZone(s *ScriptState) error {
	cPos := s.PopInt()
	cTo := s.PopInt()
	cFrom := s.PopInt()
	fromLevel, fromX, fromZ := unpackCoord(cFrom)
	toLevel, toX, toZ := unpackCoord(cTo)
	posLevel, posX, posZ := unpackCoord(cPos)

	if posX < fromX || posX > toX ||
		posLevel < fromLevel || posLevel > toLevel ||
		posZ < fromZ || posZ > toZ {
		s.PushInt(0)
		return nil
	}
	s.PushInt(1)
	return nil
}

// handleMoveCoord pops [coord, x, y, z] (z on top) and pushes a new
// packed coord with the original level/x/z offset by (y, x, z) where TS
// uses y for the level delta. Matches TS ServerOps.ts MOVECOORD.
func handleMoveCoord(s *ScriptState) error {
	z := s.PopInt()
	y := s.PopInt()
	x := s.PopInt()
	coord := s.PopInt()

	level := (coord >> 28) & 0x3
	cx := (coord >> 14) & 0x3fff
	cz := coord & 0x3fff

	level += y
	cx += x
	cz += z

	s.PushInt((level << 28) | (cx << 14) | cz)
	return nil
}

// handleSeqLength (SEQ_LENGTH) pushes the configured duration of a
// SeqType. Mirrors TS LostCityRS/Engine-TS/.../ServerOps.ts:109-111:
//
//	state.pushInt(check(state.popInt(), SeqTypeValid).duration);
func handleSeqLength(s *ScriptState) error {
	id := s.PopInt()
	if err := checkSeqType(s, id, "SEQ_LENGTH"); err != nil {
		return err
	}
	s.PushInt(s.Configs.SeqType(id).Duration)
	return nil
}

// handleMapIndoors (MAP_INDOORS, opcode 1010) pops a packed coord and
// pushes 1 if the tile at that position has the Roof collision flag set,
// else 0. Mirrors TS ServerOps.ts:
//
//	[ScriptOpcode.MAP_INDOORS]: state => {
//	    const coord = check(state.popInt(), CoordValid);
//	    state.pushInt(World.isIndoors(Position.level(coord), Position.x(coord), Position.z(coord)) ? 1 : 0);
//	}
func handleMapIndoors(s *ScriptState) error {
	if s.World == nil {
		return fmt.Errorf("MAP_INDOORS: %w", ErrNoWorld)
	}
	coord := s.PopInt()
	level, x, z, err := checkCoord(coord, "MAP_INDOORS")
	if err != nil {
		return err
	}
	if s.World.IsIndoors(x, z, level) {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	return nil
}

// handleWorldDelay (WORLD_DELAY, opcode 1021) suspends the active
// script to the world-script queue. The wakeup-tick value is NOT
// popped here — it remains on the script's int stack and is popped by
// the suspending caller (resumeOrFinish for player path, resumeOrFinishNpc
// for npc path, processWorldQueue for world-self-loop) at suspend
// time before re-enqueueing. Mirrors TS ServerOps.ts:166-169 verbatim:
//
//	[ScriptOpcode.WORLD_DELAY]: state => {
//	    // arg is popped elsewhere
//	    state.execution = ScriptState.WORLD_SUSPENDED;
//	}
//
// The "arg popped elsewhere" semantics are load-bearing: the script
// bytecode pushes the wakeup-tick before WORLD_DELAY and expects
// the resumer to consume it. Adding a pop here would break the
// bytecode contract.
//
// The consumer side (worldScriptQueue + processWorldQueue +
// resumeOrFinishWorld) is in modules/world/{world_script_queue,script}.go,
// wired into the tick at tick.go (start-of-cycle). Shipped under
// NAI-37 T8-T12 + NAI-42 panic recovery + NAI-44 / NAI-54 / NAI-55.
func handleWorldDelay(s *ScriptState) error {
	s.Execution = WorldSuspended
	return nil
}
