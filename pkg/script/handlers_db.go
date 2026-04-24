package script

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/objtype"
)

// TS ScriptOpcodePointers.ts gates find_db asymmetrically: DB_LISTALL,
// DB_LISTALL_WITH_COUNT, and DB_FIND set the flag; DB_FINDNEXT and
// DB_FIND_REFINE require it. Conspicuously omitted from the gate table:
// DB_FIND_WITH_COUNT, DB_FIND_REFINE_WITH_COUNT, DB_FINDBYINDEX. The
// WITH_COUNT variants mutate DbRowQuery identically to their plain
// counterparts but never set the flag, so a refine after a with-count find
// fails the gate despite having valid cursor state. This may be a TS bug,
// but per the project's TS-faithfulness gate we preserve the asymmetry;
// tests pin it. If upstream ever fixes it, remove this comment and the
// asymmetric branches in dbFind / dbFindRefine.

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
	s.Pointers |= PtrFindDb // S7g: TS DB_LISTALL / DB_LISTALL_WITH_COUNT set find_db.

	if withCount {
		s.PushInt(len(s.DbRowQuery))
	}
	return nil
}

// handleDbListAll (DB_LISTALL, opcode 7510).
func handleDbListAll(s *ScriptState) error { return dbListAll(s, false) }

// handleDbListAllWithCount (DB_LISTALL_WITH_COUNT, opcode 7504).
func handleDbListAllWithCount(s *ScriptState) error { return dbListAll(s, true) }

// handleDbFindNext (DB_FINDNEXT, opcode 7501) advances the DB cursor to
// the next row in DbRowQuery and pushes its id. Pushes -1 when the cursor
// is past the end. Errors when PtrFindDb is not set (i.e., no prior
// DB_LISTALL* / DB_FIND has populated the cursor). Mirrors TS DbOps.ts:82.
func handleDbFindNext(s *ScriptState) error {
	if s.Pointers&PtrFindDb == 0 {
		return fmt.Errorf("DB_FINDNEXT: find_db pointer not set")
	}
	if s.DbRow+1 >= len(s.DbRowQuery) {
		s.PushInt(-1)
		return nil
	}
	s.DbRow++
	rowID := s.DbRowQuery[s.DbRow]
	if err := checkDbRow(s, rowID, "DB_FINDNEXT"); err != nil {
		return err
	}
	s.PushInt(rowID)
	return nil
}

// handleDbFindByIndex (DB_FINDBYINDEX, opcode 7506) pops a non-negative
// index and pushes the row id at DbRowQuery[index]. Pushes -1 for any
// out-of-range index (negative or >= len). Does NOT move the DbRow
// cursor (random-access semantics). Errors when no table is selected.
// Mirrors TS DbOps.ts:152.
func handleDbFindByIndex(s *ScriptState) error {
	if s.DbTable == nil {
		return fmt.Errorf("DB_FINDBYINDEX: no table selected")
	}
	index := s.PopInt()
	if index < 0 || index >= len(s.DbRowQuery) {
		s.PushInt(-1)
		return nil
	}
	rowID := s.DbRowQuery[index]
	if err := checkDbRow(s, rowID, "DB_FINDBYINDEX"); err != nil {
		return err
	}
	s.PushInt(rowID)
	return nil
}

// dbFind is the shared implementation of DB_FIND / DB_FIND_WITH_COUNT.
// Pops an isString marker (==2 means string query), a query (int or
// string), and a packed tableColumnPacked; selects the table, resets
// DbRow to -1, populates DbRowQuery via DbTableIndex, and (for DB_FIND)
// sets PtrFindDb. For DB_FIND_WITH_COUNT, also pushes len(DbRowQuery).
// Pointer-set is asymmetric — DB_FIND_WITH_COUNT omits it per TS.
// Mirrors TS DbOps.ts:10-23.
func dbFind(s *ScriptState, withCount, setsPointer bool, op string) error {
	isString := s.PopInt() == 2

	var rowIDs []int
	if isString {
		q := s.PopString()
		packed := s.PopInt()
		tableID := (packed >> 12) & 0xffff
		if err := checkDbTable(s, tableID, op); err != nil {
			return err
		}
		s.DbTable = s.Configs.DbTableType(tableID)
		rowIDs = s.Configs.FindDbRowsStr(q, packed)
	} else {
		q := s.PopInt()
		packed := s.PopInt()
		tableID := (packed >> 12) & 0xffff
		if err := checkDbTable(s, tableID, op); err != nil {
			return err
		}
		s.DbTable = s.Configs.DbTableType(tableID)
		rowIDs = s.Configs.FindDbRowsInt(int32(q), packed)
	}

	s.DbRow = -1
	s.DbRowQuery = append(s.DbRowQuery[:0], rowIDs...)

	if setsPointer {
		s.Pointers |= PtrFindDb // TS: set: ['find_db']
	}
	// DB_FIND_WITH_COUNT intentionally omits the set (TS asymmetry — see preamble).

	if withCount {
		s.PushInt(len(s.DbRowQuery))
	}
	return nil
}

// handleDbFind (DB_FIND, opcode 7508).
func handleDbFind(s *ScriptState) error { return dbFind(s, false, true, "DB_FIND") }

// handleDbFindWithCount (DB_FIND_WITH_COUNT, opcode 7500).
func handleDbFindWithCount(s *ScriptState) error {
	return dbFind(s, true, false, "DB_FIND_WITH_COUNT")
}

// dbFindRefine is the shared implementation of DB_FIND_REFINE /
// DB_FIND_REFINE_WITH_COUNT. Requires PtrFindDb (for the plain variant
// only — DB_FIND_REFINE_WITH_COUNT omits the require per TS). Pops the
// same three args as dbFind, looks up the match set, intersects with
// the prev query (DbRowQuery). Intersection preserves prev-order:
// iteration is over prev, membership check against the found set.
// Allocates a fresh slice to avoid an aliasing trap on
// `append(s.DbRowQuery[:0], ...)` while iterating the same backing array.
// Resets DbRow to -1; pushes count if withCount. Mirrors TS DbOps.ts:42-63.
func dbFindRefine(s *ScriptState, withCount, requiresPointer bool, op string) error {
	if requiresPointer && s.Pointers&PtrFindDb == 0 {
		return fmt.Errorf("%s: find_db pointer not set", op)
	}
	// DB_FIND_REFINE_WITH_COUNT intentionally omits the require (TS asymmetry — see preamble).

	isString := s.PopInt() == 2
	var found []int
	if isString {
		q := s.PopString()
		packed := s.PopInt()
		found = s.Configs.FindDbRowsStr(q, packed)
	} else {
		q := s.PopInt()
		packed := s.PopInt()
		found = s.Configs.FindDbRowsInt(int32(q), packed)
	}

	foundSet := make(map[int]struct{}, len(found))
	for _, id := range found {
		foundSet[id] = struct{}{}
	}

	prev := s.DbRowQuery
	// fresh slice; do NOT use append(s.DbRowQuery[:0], ...) here — prev
	// aliases the same backing array and writes would corrupt the iteration.
	refined := make([]int, 0, len(prev))
	for _, id := range prev {
		if _, ok := foundSet[id]; ok {
			refined = append(refined, id)
		}
	}

	s.DbRow = -1
	s.DbRowQuery = refined

	if withCount {
		s.PushInt(len(refined))
	}
	return nil
}

// handleDbFindRefine (DB_FIND_REFINE, opcode 7509).
func handleDbFindRefine(s *ScriptState) error {
	return dbFindRefine(s, false, true, "DB_FIND_REFINE")
}

// handleDbFindRefineWithCount (DB_FIND_REFINE_WITH_COUNT, opcode 7507).
func handleDbFindRefineWithCount(s *ScriptState) error {
	return dbFindRefine(s, true, false, "DB_FIND_REFINE_WITH_COUNT")
}
