package script

import "fmt"

// checkDbTable mirrors TS DbTableTypeValid (ScriptValidators.ts:135) — a
// ScriptInputConfigTypeValidator over DbTableType. Range + presence checks
// both collapse into "s.Configs.DbTableType(id) != nil" per the Configs
// interface contract. Follows the S7c checkInvType pattern.
func checkDbTable(s *ScriptState, id int, op string) error {
	if s.Configs == nil || s.Configs.DbTableType(id) == nil {
		return fmt.Errorf("%s: no DbTableType with value (%d) found", op, id)
	}
	return nil
}

// checkDbRow mirrors TS DbRowTypeValid (ScriptValidators.ts:134) — same
// contract-collapse as checkDbTable above (range + presence collapsed into
// the Configs lookup).
func checkDbRow(s *ScriptState, id int, op string) error {
	if s.Configs == nil || s.Configs.DbRowType(id) == nil {
		return fmt.Errorf("%s: no DbRowType with value (%d) found", op, id)
	}
	return nil
}

// handleDbGetFieldCount is an MVP stub: pops (tableColumnPacked, row) and
// pushes 0. TS DbOps.ts:135 returns the column's stored field count from
// DbRowType.columnValues. Real implementation ships in S7d Task 5 alongside
// DB_GETFIELD; the stub lets scripts that iterate rows-up-to-count exit
// cleanly on zero in the interim.
func handleDbGetFieldCount(s *ScriptState) error {
	_ = s.PopInt() // tableColumnPacked
	_ = s.PopInt() // row
	s.PushInt(0)
	return nil
}
