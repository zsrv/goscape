package script

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/objtype"
)

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

// handleDbGetFieldCount (DB_GETFIELDCOUNT, opcode 7503) pops a row id and a
// (table << 12 | column << 4) packed key and pushes the number of stored
// tuples in the row at that column. Pushes 0 when the row's TableID doesn't
// match the packed table (cross-table reads are no-ops on count). Mirrors
// TS DbOps.ts:135.
func handleDbGetFieldCount(s *ScriptState) error {
	tableColumnPacked := s.PopInt()
	row := s.PopInt()

	table := (tableColumnPacked >> 12) & 0xffff
	column := (tableColumnPacked >> 4) & 0x7f

	if err := checkDbRow(s, row, "DB_GETFIELDCOUNT"); err != nil {
		return err
	}
	if err := checkDbTable(s, table, "DB_GETFIELDCOUNT"); err != nil {
		return err
	}

	rowType := s.Configs.DbRowType(row)
	tableType := s.Configs.DbTableType(table)

	if rowType.TableID != table {
		s.PushInt(0)
		return nil
	}
	s.PushInt(len(rowType.IntValues[column]) / len(tableType.Types[column]))
	return nil
}

// handleDbGetField (DB_GETFIELD, opcode 7502) pops listIndex, a
// (table << 12 | column << 4 | tupleIndex+1) packed key, and a row id,
// and pushes the requested value(s) in order. Type-directed push uses
// the table's type schema (not the row's). When the row's TableID doesn't
// match the packed table, falls back to the table's column default.
// Mirrors TS DbOps.ts:97.
func handleDbGetField(s *ScriptState) error {
	listIndex := s.PopInt()
	packed := s.PopInt()
	row := s.PopInt()

	fieldTable := (packed >> 12) & 0xffff
	fieldColumn := (packed >> 4) & 0x7f
	tupleIndex := (packed & 0xf) - 1

	if err := checkDbRow(s, row, "DB_GETFIELD"); err != nil {
		return err
	}
	if err := checkDbTable(s, fieldTable, "DB_GETFIELD"); err != nil {
		return err
	}

	rowType := s.Configs.DbRowType(row)
	tableType := s.Configs.DbTableType(fieldTable)
	valueTypes := tableType.Types[fieldColumn]

	off, length := 0, len(valueTypes)
	if tupleIndex >= 0 {
		if tupleIndex >= length {
			return fmt.Errorf("DB_GETFIELD: tuple index out-of-bounds. Requested: %d, Max: %d", tupleIndex, length)
		}
		off = tupleIndex
		length = tupleIndex + 1
	}

	var ints []int32
	var strs []string
	if rowType.TableID != fieldTable {
		ints, strs, _ = tableType.GetDefault(fieldColumn)
	} else {
		ints, strs, _ = rowType.GetValue(fieldColumn, listIndex, tableType)
	}

	for i := off; i < length; i++ {
		if valueTypes[i] == objtype.ScriptVarTypeString {
			s.PushString(strs[i])
		} else {
			s.PushInt(int(ints[i]))
		}
	}
	return nil
}

// handleDbGetRowTable (DB_GETROWTABLE, opcode 7505) pops a row id and
// pushes the row's TableID. Mirrors TS DbOps.ts:175.
func handleDbGetRowTable(s *ScriptState) error {
	row := s.PopInt()
	if err := checkDbRow(s, row, "DB_GETROWTABLE"); err != nil {
		return err
	}
	s.PushInt(s.Configs.DbRowType(row).TableID)
	return nil
}

// dbListAll is the shared helper behind DB_LISTALL / DB_LISTALL_WITH_COUNT.
// Selects the given table as the cursor's current DbTable, resets DbRow
// to -1, and populates DbRowQuery with all row IDs in ascending order.
// When withCount is true, also pushes len(DbRowQuery). Mirrors TS
// DbOps.ts:25.
func dbListAll(s *ScriptState, withCount bool) error {
	table := s.PopInt()
	if err := checkDbTable(s, table, "DB_LISTALL"); err != nil {
		return err
	}

	s.DbTable = s.Configs.DbTableType(table)
	s.DbRow = -1
	s.DbRowQuery = append(s.DbRowQuery[:0], s.Configs.DbRowsInTable(table)...)

	if withCount {
		s.PushInt(len(s.DbRowQuery))
	}
	return nil
}

// handleDbListAll (DB_LISTALL, opcode 7510).
func handleDbListAll(s *ScriptState) error { return dbListAll(s, false) }

// handleDbListAllWithCount (DB_LISTALL_WITH_COUNT, opcode 7504).
func handleDbListAllWithCount(s *ScriptState) error { return dbListAll(s, true) }
