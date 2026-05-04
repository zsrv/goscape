package script

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/coordgrid"
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
