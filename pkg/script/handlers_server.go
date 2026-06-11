package script

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/coordgrid"
)

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

// handleMapLive (MAP_LIVE, opcode 1011) pushes 1 if the server runs in
// production mode, else 0. Mirrors TS ServerOps.ts @2e3bcf43:
//
//	[ScriptOpcode.MAP_LIVE]: state => {
//	    state.pushInt(Environment.NODE_PRODUCTION ? 1 : 0);
//	}
//
// 2e3bcf43 restores the 225-era MAP_LIVE op; the 244-era MAP_PRODUCTION
// debug op (10001) carried the same body and was deleted upstream. The
// WorldVars surface keeps its Go-side MapProduction() name.
func handleMapLive(s *ScriptState) error {
	if s.World == nil {
		return fmt.Errorf("MAP_LIVE: %w", ErrNoWorld)
	}
	s.PushInt(s.World.MapProduction())
	return nil
}

// handleMidiLength (MIDI_LENGTH, opcode 1022) — A10 STUB. TS ServerOps.ts
// @2e3bcf43:
//
//	[ScriptOpcode.MIDI_LENGTH]: state => {
//	    const track = state.popInt();
//	    state.pushInt(Midi.getTickLength(track));
//	}
//
// Depends on the Midi cache (src/cache/midi/Midi.ts, new at the 254
// pin-advance) which Task A10 ports. Until A10 lands Midi.getTickLength,
// this errors like the other unimplemented-op stubs. The track id is
// popped first so the stack contract already matches TS.
// TODO(A10): replace the error with a midi-length registry lookup.
func handleMidiLength(s *ScriptState) error {
	_ = s.PopInt() // track id — popped per the TS stack contract
	return fmt.Errorf("MIDI_LENGTH: unimplemented — A10 lands Midi.getTickLength")
}

// handleInZone pops [from, to, pos] (pos on top) and pushes 1 if pos
// is inside the box [from..to] on all of x/z/level, else 0. Matches
// TS ServerOps.ts INZONE (axis-aligned inclusive bounds).
func handleInZone(s *ScriptState) error {
	cPos := s.PopInt()
	cTo := s.PopInt()
	cFrom := s.PopInt()
	// L18: validate each coord (TS CoordValid) in TS order from→to→pos; a
	// negative/out-of-range packed coord aborts rather than silently masking.
	fromLevel, fromX, fromZ, err := checkCoord(cFrom, "INZONE")
	if err != nil {
		return err
	}
	toLevel, toX, toZ, err := checkCoord(cTo, "INZONE")
	if err != nil {
		return err
	}
	posLevel, posX, posZ, err := checkCoord(cPos, "INZONE")
	if err != nil {
		return err
	}

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

	// L18: only the base coord is CoordValid-checked (TS); x/y/z are deltas.
	level, cx, cz, err := checkCoord(coord, "MOVECOORD")
	if err != nil {
		return err
	}

	level += y
	cx += x
	cz += z

	// h-server-1 (2026-05-28 audit): TS ServerOps.ts:106 packs via
	// CoordGrid.packCoord (CoordGrid.ts:136-138), which applies the
	// 0x3fff/0x3 masks `(z & 0x3fff) | ((x & 0x3fff) << 14) | ((level & 0x3) << 28)`.
	// goscape pre-fix packed raw, so a cx (or cz) delta that pushed
	// the value above 0x3fff would bleed into the level field, and a
	// level delta > 3 would bleed past bit 31 into the sign bit.
	// pkg/coordgrid.PackCoord (coordgrid.go:167) is the byte-equivalent
	// port of TS CoordGrid.packCoord — reuse it.
	s.PushInt(coordgrid.PackCoord(level, cx, cz))
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

// handleWorldDelay (WORLD_DELAY, opcode 1029) suspends the active
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
