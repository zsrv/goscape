package script

import "fmt"

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
