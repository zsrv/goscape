package script

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/pathfinder/loc"
)

// requireActiveLoc returns an error tagged with the opcode name if the
// script has no ActiveLoc bound. All LOC_* read handlers start with
// this check to mirror TS `checkedHandler(ActiveLoc, ...)`.
func requireActiveLoc(s *ScriptState, op string) error {
	if s.ActiveLoc == nil {
		return fmt.Errorf("%s: no active loc", op)
	}
	return nil
}

// handleLocFind is a stub for the LOC_FIND opcode. TS:
//
//	const [coord, locId] = state.popInts(2);
//	loc = World.getLoc(coord.x, coord.z, coord.level, locType.id);
//	if loc: activeLoc = loc; pushInt(1); else: pushInt(0);
//
// MVP stub: pop both args, push 0 (not found). Scripts that branch
// on "found" take the else-branch, which is almost always the safe
// path (e.g. check_chest_macro_gas proc early-returns on LOC_FIND=0).
// Real implementation needs world-wide loc iteration + ActiveLoc
// setup; ships with a later S6 sub-spec.
func handleLocFind(s *ScriptState) error {
	_ = s.PopInt() // locId (type)
	_ = s.PopInt() // coord (packed)
	s.PushInt(0)
	return nil
}

// handleLocOp pops a 1-indexed op slot and pushes the ActiveLoc's
// LocType.Op[op-1] string. Pushes "" if:
//   - Configs is nil (test-only defensive guard)
//   - LocType is not loaded for the ActiveLoc's type ID
//   - op is out of [1, len(Op)] range
//
// Mirrors handleNpcHasOp (handlers_npc.go:87) in structure — same read
// path through Configs → LocType.Op — but returns the string rather
// than a bool (NPC_HASOP's boolean form).
func handleLocOp(s *ScriptState) error {
	if err := requireActiveLoc(s, "LOC_OP"); err != nil {
		return err
	}
	op := s.PopInt()
	if s.Configs == nil {
		s.PushString("")
		return nil
	}
	cfg := s.Configs.LocType(s.ActiveLoc.LocType())
	if cfg == nil {
		s.PushString("")
		return nil
	}
	idx := op - 1
	if idx < 0 || idx >= len(cfg.Op) {
		s.PushString("")
		return nil
	}
	s.PushString(cfg.Op[idx])
	return nil
}

// handleLocCoord pushes the ActiveLoc's packed (level, x, z) coord onto
// the int stack. TS:
//
//	pushInt(CoordGrid.packCoord(activeLoc.level, activeLoc.x, activeLoc.z));
//
// Requires an ActiveLoc; returns "LOC_COORD: no active loc" otherwise.
func handleLocCoord(s *ScriptState) error {
	if err := requireActiveLoc(s, "LOC_COORD"); err != nil {
		return err
	}
	x, z, level := s.ActiveLoc.Coords()
	s.PushInt(coordgrid.PackCoord(level, x, z))
	return nil
}

// handleLocAngle pushes the ActiveLoc's rotation onto the int stack,
// validated through the [0,3] LocAngle range. TS:
//
//	pushInt(check(activeLoc.angle, LocAngleValid));
//
// Requires an ActiveLoc; returns "LOC_ANGLE: no active loc" otherwise.
func handleLocAngle(s *ScriptState) error {
	if err := requireActiveLoc(s, "LOC_ANGLE"); err != nil {
		return err
	}
	angle := s.ActiveLoc.Angle()
	if err := checkLocAngle(angle); err != nil {
		return fmt.Errorf("LOC_ANGLE: %w", err)
	}
	s.PushInt(angle)
	return nil
}

// handleLocType pushes the ActiveLoc's resolved LocType ID. Mirrors TS
// LOC_TYPE: pushInt(check(activeLoc.type, LocTypeValid).id).
//
// Goscape ActiveLoc.LocType() returns the int ID directly; the TS
// LocTypeValid non-null check translates to a Configs lookup nil-check
// (matches handleNpcParam pattern at handlers_config.go:307-308).
func handleLocType(s *ScriptState) error {
	if err := requireConfigs(s, "LOC_TYPE"); err != nil {
		return err
	}
	if err := requireActiveLoc(s, "LOC_TYPE"); err != nil {
		return err
	}
	id := s.ActiveLoc.LocType()
	lt := s.Configs.LocType(id)
	if lt == nil {
		return fmt.Errorf("LOC_TYPE: unknown loc id %d", id)
	}
	s.PushInt(id)
	return nil
}

// handleLocName pushes the ActiveLoc's LocType name, with "null" fallback
// when the name is empty. Mirrors TS LOC_NAME: pushString(check(activeLoc.type,
// LocTypeValid).name ?? 'null').
//
// Note: TS active-loc LOC_NAME does NOT fall back to debugname (only the
// locID-arg LC_NAME does). LC_NAME itself currently uses DebugName with a
// stale comment claiming Name is unset server-side; that's a tracked
// follow-up (NAI-N+1 — fix LC_NAME to use Name → DebugName → "null"
// per TS LocConfigOps.ts:12). LOC_NAME ships TS-correct from the start.
func handleLocName(s *ScriptState) error {
	if err := requireConfigs(s, "LOC_NAME"); err != nil {
		return err
	}
	if err := requireActiveLoc(s, "LOC_NAME"); err != nil {
		return err
	}
	id := s.ActiveLoc.LocType()
	lt := s.Configs.LocType(id)
	if lt == nil {
		return fmt.Errorf("LOC_NAME: unknown loc id %d", id)
	}
	if lt.Name != "" {
		s.PushString(lt.Name)
	} else {
		s.PushString("null")
	}
	return nil
}

// handleLocShape pushes the ActiveLoc's shape, validated through the
// [0,22] LocShape range. TS:
//
//	pushInt(check(activeLoc.shape, LocShapeValid));
//
// Requires an ActiveLoc; returns "LOC_SHAPE: no active loc" otherwise.
// Range-validates because Loc.Shape()'s mask is [0,31] — wider than
// the LocShape valid range.
func handleLocShape(s *ScriptState) error {
	if err := requireActiveLoc(s, "LOC_SHAPE"); err != nil {
		return err
	}
	shape := s.ActiveLoc.Shape()
	if err := checkLocShape(shape); err != nil {
		return fmt.Errorf("LOC_SHAPE: %w", err)
	}
	s.PushInt(shape)
	return nil
}

// checkDuration mirrors TS DurationValid (ScriptValidators.ts:108) — a
// range validator rejecting [<1, >2147483647]. Reused by LOC_CHANGE,
// LOC_ADD, LOC_DEL.
func checkDuration(v int) error {
	if v < 1 || v > 2147483647 {
		return fmt.Errorf("duration out of range [1, 2147483647]: %d", v)
	}
	return nil
}

// handleLocChange pops [id, duration] from the int stack and asks
// LocOps to mutate the ActiveLoc's type to id, preserving shape/angle.
// Mirrors TS LOC_CHANGE (LocOps.ts:60-67):
//
//	const [id, duration] = state.popInts(2);
//	check(duration, DurationValid);
//	check(id, LocTypeValid);
//	World.changeLoc(state.activeLoc, id, state.activeLoc.shape, state.activeLoc.angle, duration);
func handleLocChange(s *ScriptState) error {
	if err := requireActiveLoc(s, "LOC_CHANGE"); err != nil {
		return err
	}
	if err := requireConfigs(s, "LOC_CHANGE"); err != nil {
		return err
	}
	duration := s.PopInt()
	id := s.PopInt()
	if err := checkDuration(duration); err != nil {
		return fmt.Errorf("LOC_CHANGE: %w", err)
	}
	if s.Configs.LocType(id) == nil {
		return fmt.Errorf("LOC_CHANGE: unknown loc id %d", id)
	}
	if s.LocOps == nil {
		return fmt.Errorf("LOC_CHANGE: LocOps unavailable")
	}
	return s.LocOps.ChangeLoc(s.ActiveLoc, id, s.ActiveLoc.Shape(), s.ActiveLoc.Angle(), duration)
}

// handleLocParam pops paramID, resolves the ActiveLoc's LocType, and
// delegates to paramLookup. Mirrors TS LOC_PARAM (LocOps.ts:114-123) —
// the active-loc-bound counterpart of LC_PARAM (handlers_config.go:163).
func handleLocParam(s *ScriptState) error {
	if err := requireConfigs(s, "LOC_PARAM"); err != nil {
		return err
	}
	if err := requireActiveLoc(s, "LOC_PARAM"); err != nil {
		return err
	}
	paramID := s.PopInt()
	id := s.ActiveLoc.LocType()
	lt := s.Configs.LocType(id)
	if lt == nil {
		return fmt.Errorf("LOC_PARAM: unknown loc id %d", id)
	}
	return paramLookup(s, lt.Params, paramID)
}

// handleLocDel pops [duration] and removes the ActiveLoc. Mirrors TS
// LOC_DEL (LocOps.ts:74-77).
func handleLocDel(s *ScriptState) error {
	if err := requireActiveLoc(s, "LOC_DEL"); err != nil {
		return err
	}
	duration := s.PopInt()
	if err := checkDuration(duration); err != nil {
		return fmt.Errorf("LOC_DEL: %w", err)
	}
	if s.LocOps == nil {
		return fmt.Errorf("LOC_DEL: LocOps unavailable")
	}
	return s.LocOps.RemoveLoc(s.ActiveLoc, duration)
}

// handleLocAnim pops [seq], validates against Configs.SeqType, and
// dispatches an animation event for the ActiveLoc. Mirrors TS LOC_ANIM
// (LocOps.ts:50-54).
func handleLocAnim(s *ScriptState) error {
	if err := requireActiveLoc(s, "LOC_ANIM"); err != nil {
		return err
	}
	if err := requireConfigs(s, "LOC_ANIM"); err != nil {
		return err
	}
	seq := s.PopInt()
	if s.Configs.SeqType(seq) == nil {
		return fmt.Errorf("LOC_ANIM: unknown seq id %d", seq)
	}
	if s.LocOps == nil {
		return fmt.Errorf("LOC_ANIM: LocOps unavailable")
	}
	return s.LocOps.AnimLoc(s.ActiveLoc, seq)
}

// handleLocAdd pops [coord, type, angle, shape, duration] and either
// (a) finds a same-layer loc at coord and changes it, or (b) creates a
// new DESPAWN-lifecycle loc. Mirrors TS LOC_ADD (LocOps.ts:18-43):
//
//	const [coord, type, angle, shape, duration] = state.popInts(5);
//	[validators]
//	for loc at zone-coord:
//	    if loc.layer === locShapeLayer(shape):
//	        World.changeLoc(loc, type, shape, angle, duration); return
//	const created = new Loc(level, x, z, locType.width, locType.length, DESPAWN, type, shape, angle);
//	World.addLoc(created, duration);
func handleLocAdd(s *ScriptState) error {
	if err := requireConfigs(s, "LOC_ADD"); err != nil {
		return err
	}
	duration := s.PopInt()
	shape := s.PopInt()
	angle := s.PopInt()
	typ := s.PopInt()
	coord := s.PopInt()

	if s.Configs.LocType(typ) == nil {
		return fmt.Errorf("LOC_ADD: unknown loc id %d", typ)
	}
	if err := checkLocAngle(angle); err != nil {
		return fmt.Errorf("LOC_ADD: %w", err)
	}
	if err := checkLocShape(shape); err != nil {
		return fmt.Errorf("LOC_ADD: %w", err)
	}
	if err := checkDuration(duration); err != nil {
		return fmt.Errorf("LOC_ADD: %w", err)
	}
	if s.LocOps == nil {
		return fmt.Errorf("LOC_ADD: LocOps unavailable")
	}

	pos := coordgrid.UnpackCoord(coord)
	wantLayer := int(loc.LayerOf(loc.Shape(shape)))
	for _, existing := range s.LocOps.LocsAtCoord(pos.Level, pos.X, pos.Z) {
		if existing.Layer() == wantLayer {
			if err := s.LocOps.ChangeLoc(existing, typ, shape, angle, duration); err != nil {
				return err
			}
			s.ActiveLoc = existing
			return nil
		}
	}
	created, err := s.LocOps.AddLoc(pos.Level, pos.X, pos.Z, typ, shape, angle, duration)
	if err != nil {
		return err
	}
	s.ActiveLoc = created
	return nil
}
