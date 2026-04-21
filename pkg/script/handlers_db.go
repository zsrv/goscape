package script

// handleDbGetFieldCount is a stub for the DB_GETFIELDCOUNT opcode. TS:
//
//	const [row, tableColumnPacked] = state.popInts(2);
//	... look up db table + field schema ...
//	state.pushInt(fieldCount);
//
// MVP stub: pop both args, push 0 (empty row). Scripts that loop
// over DB table rows exit immediately (e.g. inzone_coord_pair_table
// proc iterates rows up to count and checks INZONE per row; with
// count=0 the loop body is skipped and the proc returns "not in
// zone"). Real implementation needs DbTableType / DbRowType cache
// loaders + per-column value decoding — deferred to a DB sub-spec.
func handleDbGetFieldCount(s *ScriptState) error {
	_ = s.PopInt() // tableColumnPacked
	_ = s.PopInt() // row
	s.PushInt(0)
	return nil
}
