package script

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
