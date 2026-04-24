package script

import (
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// fakeDbConfigs implements Configs with DB fixtures only; non-DB methods
// return nil. Used by all handlers_db_test.go tests.
type fakeDbConfigs struct {
	tables  map[int]*objtype.DbTableType
	rows    map[int]*objtype.DbRowType
	rowsByT map[int][]int
}

func (f *fakeDbConfigs) ObjType(id int) *objtype.ObjType       { return nil }
func (f *fakeDbConfigs) NpcType(id int) *objtype.NpcType       { return nil }
func (f *fakeDbConfigs) LocType(id int) *objtype.LocType       { return nil }
func (f *fakeDbConfigs) EnumType(id int) *objtype.EnumType     { return nil }
func (f *fakeDbConfigs) StructType(id int) *objtype.StructType { return nil }
func (f *fakeDbConfigs) ParamType(id int) *objtype.ParamType   { return nil }
func (f *fakeDbConfigs) InvType(id int) *objtype.InvType       { return nil }
func (f *fakeDbConfigs) DbTableType(id int) *objtype.DbTableType {
	return f.tables[id]
}
func (f *fakeDbConfigs) DbRowType(id int) *objtype.DbRowType { return f.rows[id] }
func (f *fakeDbConfigs) DbRowsInTable(tableID int) []int     { return f.rowsByT[tableID] }

// newDbState builds a ScriptState with Configs wired for DB tests.
func newDbState(cfg *fakeDbConfigs) *ScriptState {
	return &ScriptState{
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
		Configs:     cfg,
	}
}

// TestCheckDbTable exercises the DbTableType validator across id validity,
// out-of-range, and nil Configs.
func TestCheckDbTable(t *testing.T) {
	tbl := objtype.NewDbTableType(1)
	cfg := &fakeDbConfigs{
		tables: map[int]*objtype.DbTableType{1: tbl},
	}
	s := newDbState(cfg)

	if err := checkDbTable(s, 1, "DB_GETFIELD"); err != nil {
		t.Errorf("valid id: want nil, got %v", err)
	}
	if err := checkDbTable(s, -1, "DB_GETFIELD"); err == nil {
		t.Error("id=-1: want error, got nil")
	} else if !strings.Contains(err.Error(), "no DbTableType with value (-1)") {
		t.Errorf("id=-1: error message %q does not contain expected substring", err.Error())
	}
	if err := checkDbTable(s, 99, "DB_GETFIELD"); err == nil {
		t.Error("id=99 (not in fixture): want error, got nil")
	}

	// nil Configs.
	s.Configs = nil
	if err := checkDbTable(s, 1, "DB_GETFIELD"); err == nil {
		t.Error("nil Configs: want error, got nil")
	}
}

// TestCheckDbRow mirrors TestCheckDbTable for DbRowType.
func TestCheckDbRow(t *testing.T) {
	row := objtype.NewDbRowType(5)
	cfg := &fakeDbConfigs{
		rows: map[int]*objtype.DbRowType{5: row},
	}
	s := newDbState(cfg)

	if err := checkDbRow(s, 5, "DB_GETFIELD"); err != nil {
		t.Errorf("valid id: want nil, got %v", err)
	}
	if err := checkDbRow(s, -1, "DB_GETFIELD"); err == nil {
		t.Error("id=-1: want error, got nil")
	} else if !strings.Contains(err.Error(), "no DbRowType with value (-1)") {
		t.Errorf("id=-1: error message %q does not contain expected substring", err.Error())
	}
	if err := checkDbRow(s, 99, "DB_GETFIELD"); err == nil {
		t.Error("id=99: want error, got nil")
	}

	s.Configs = nil
	if err := checkDbRow(s, 5, "DB_GETFIELD"); err == nil {
		t.Error("nil Configs: want error, got nil")
	}
}

// buildDbFixture builds a tiny fakeDbConfigs with one table (id=7, columns
// [INT, STRING]) and two rows (id=0 and id=1, both in table 7).
func buildDbFixture() *fakeDbConfigs {
	tbl := objtype.NewDbTableType(7)
	tbl.Types = [][]objtype.ScriptVarType{
		{objtype.ScriptVarTypeInt},
		{objtype.ScriptVarTypeString},
	}
	tbl.DefaultInts = [][]int32{nil, {0}}
	tbl.DefaultStrs = [][]string{{""}, {"default_name"}}

	row0 := objtype.NewDbRowType(0)
	row0.TableID = 7
	row0.Types = [][]objtype.ScriptVarType{
		{objtype.ScriptVarTypeInt},
		{objtype.ScriptVarTypeString},
	}
	row0.IntValues = [][]int32{{42}, {0}}
	row0.StringValues = [][]string{{""}, {"hello"}}

	row1 := objtype.NewDbRowType(1)
	row1.TableID = 7
	row1.Types = [][]objtype.ScriptVarType{
		{objtype.ScriptVarTypeInt},
	}
	row1.IntValues = [][]int32{{99}}
	row1.StringValues = [][]string{{""}}

	return &fakeDbConfigs{
		tables:  map[int]*objtype.DbTableType{7: tbl},
		rows:    map[int]*objtype.DbRowType{0: row0, 1: row1},
		rowsByT: map[int][]int{7: {0, 1}},
	}
}

// pack builds the DB_GETFIELD/GETFIELDCOUNT packed key. tupleIndex=-1
// (i.e. "no tuple") maps to low-4-bits=0.
func pack(table, column, tupleIndex int) int {
	low := 0
	if tupleIndex >= 0 {
		low = (tupleIndex + 1) & 0xf
	}
	return (table&0xffff)<<12 | (column&0x7f)<<4 | low
}

// TestHandleDbGetField_Int verifies the INT column push path.
func TestHandleDbGetField_Int(t *testing.T) {
	s := newDbState(buildDbFixture())
	s.PushInt(0)              // row
	s.PushInt(pack(7, 0, -1)) // packed: table 7, column 0, no tuple
	s.PushInt(0)              // listIndex

	if err := handleDbGetField(s); err != nil {
		t.Fatalf("handleDbGetField: %v", err)
	}
	if s.ISP != 1 || s.IntStack[0] != 42 {
		t.Errorf("int stack: got ISP=%d, top=%d; want ISP=1, top=42", s.ISP, s.IntStack[0])
	}
}

// TestHandleDbGetField_String verifies the STRING column push path.
func TestHandleDbGetField_String(t *testing.T) {
	s := newDbState(buildDbFixture())
	s.PushInt(0)
	s.PushInt(pack(7, 1, -1))
	s.PushInt(0)

	if err := handleDbGetField(s); err != nil {
		t.Fatalf("handleDbGetField: %v", err)
	}
	if s.SSP != 1 || s.StringStack[0] != "hello" {
		t.Errorf("string stack: got SSP=%d, top=%q; want SSP=1, top=\"hello\"", s.SSP, s.StringStack[0])
	}
}

// TestHandleDbGetField_CrossTableFallsBackToDefault verifies the fallback
// when the row's TableID differs from the packed table.
func TestHandleDbGetField_CrossTableFallsBackToDefault(t *testing.T) {
	cfg := buildDbFixture()
	// Make row 0 belong to a different table.
	cfg.rows[0].TableID = 99

	s := newDbState(cfg)
	s.PushInt(0)
	s.PushInt(pack(7, 1, -1)) // STRING column 1
	s.PushInt(0)

	if err := handleDbGetField(s); err != nil {
		t.Fatalf("handleDbGetField: %v", err)
	}
	if s.SSP != 1 || s.StringStack[0] != "default_name" {
		t.Errorf("string stack: got SSP=%d, top=%q; want default fallback \"default_name\"", s.SSP, s.StringStack[0])
	}
}

// TestHandleDbGetField_ListIndexOutOfRangeFallsBack verifies that an
// out-of-range listIndex falls back to the table default (via GetValue).
func TestHandleDbGetField_ListIndexOutOfRangeFallsBack(t *testing.T) {
	s := newDbState(buildDbFixture())
	s.PushInt(0)
	s.PushInt(pack(7, 1, -1)) // STRING column 1
	s.PushInt(5)              // listIndex way out of range

	if err := handleDbGetField(s); err != nil {
		t.Fatalf("handleDbGetField: %v", err)
	}
	if s.SSP != 1 || s.StringStack[0] != "default_name" {
		t.Errorf("expected fallback to \"default_name\", got %q", s.StringStack[0])
	}
}

// TestHandleDbGetField_TupleIndex_SingleSlot verifies that a valid tupleIndex
// selects one slot only.
func TestHandleDbGetField_TupleIndex_SingleSlot(t *testing.T) {
	// Reshape: table 7 column 0 becomes a [INT, INT] tuple. Row 0 has
	// IntValues[0] = [10, 20].
	cfg := buildDbFixture()
	cfg.tables[7].Types[0] = []objtype.ScriptVarType{objtype.ScriptVarTypeInt, objtype.ScriptVarTypeInt}
	cfg.rows[0].Types[0] = []objtype.ScriptVarType{objtype.ScriptVarTypeInt, objtype.ScriptVarTypeInt}
	cfg.rows[0].IntValues[0] = []int32{10, 20}
	cfg.rows[0].StringValues[0] = []string{"", ""}

	s := newDbState(cfg)
	s.PushInt(0)
	s.PushInt(pack(7, 0, 1)) // tupleIndex=1 → low 4 bits = 2
	s.PushInt(0)

	if err := handleDbGetField(s); err != nil {
		t.Fatalf("handleDbGetField: %v", err)
	}
	if s.ISP != 1 || s.IntStack[0] != 20 {
		t.Errorf("expected single-slot push of 20, got ISP=%d top=%d", s.ISP, s.IntStack[0])
	}
}

// TestHandleDbGetField_TupleIndex_OutOfBounds returns an error.
func TestHandleDbGetField_TupleIndex_OutOfBounds(t *testing.T) {
	cfg := buildDbFixture()
	// Column 0 is still [INT] (length 1); packing tupleIndex=1 is OOB.
	s := newDbState(cfg)
	s.PushInt(0)
	s.PushInt(pack(7, 0, 1)) // tupleIndex=1 in a length-1 tuple
	s.PushInt(0)

	err := handleDbGetField(s)
	if err == nil {
		t.Fatal("expected tuple-out-of-bounds error, got nil")
	}
	if !strings.Contains(err.Error(), "tuple index out-of-bounds") {
		t.Errorf("error message %q missing \"tuple index out-of-bounds\"", err.Error())
	}
}

// TestHandleDbGetField_InvalidRow returns the validator error.
func TestHandleDbGetField_InvalidRow(t *testing.T) {
	s := newDbState(buildDbFixture())
	s.PushInt(99) // no such row
	s.PushInt(pack(7, 0, -1))
	s.PushInt(0)

	err := handleDbGetField(s)
	if err == nil {
		t.Fatal("expected validator error, got nil")
	}
	if !strings.Contains(err.Error(), "DB_GETFIELD: no DbRowType") {
		t.Errorf("error: %q", err.Error())
	}
}

// TestHandleDbGetField_InvalidTable returns the validator error.
func TestHandleDbGetField_InvalidTable(t *testing.T) {
	s := newDbState(buildDbFixture())
	s.PushInt(0)
	s.PushInt(pack(999, 0, -1)) // no such table
	s.PushInt(0)

	err := handleDbGetField(s)
	if err == nil {
		t.Fatal("expected validator error, got nil")
	}
	if !strings.Contains(err.Error(), "DB_GETFIELD: no DbTableType") {
		t.Errorf("error: %q", err.Error())
	}
}

// TestHandleDbGetField_MixedTupleFullPush verifies full-tuple push across a
// mixed INT/STRING column (both kinds of push in one call).
func TestHandleDbGetField_MixedTupleFullPush(t *testing.T) {
	cfg := buildDbFixture()
	cfg.tables[7].Types[0] = []objtype.ScriptVarType{objtype.ScriptVarTypeInt, objtype.ScriptVarTypeString}
	cfg.rows[0].Types[0] = []objtype.ScriptVarType{objtype.ScriptVarTypeInt, objtype.ScriptVarTypeString}
	cfg.rows[0].IntValues[0] = []int32{7, 0}
	cfg.rows[0].StringValues[0] = []string{"", "mixed"}

	s := newDbState(cfg)
	s.PushInt(0)
	s.PushInt(pack(7, 0, -1)) // whole-tuple push
	s.PushInt(0)

	if err := handleDbGetField(s); err != nil {
		t.Fatalf("handleDbGetField: %v", err)
	}
	if s.ISP != 1 || s.IntStack[0] != 7 {
		t.Errorf("int slot: got ISP=%d top=%d; want ISP=1 top=7", s.ISP, s.IntStack[0])
	}
	if s.SSP != 1 || s.StringStack[0] != "mixed" {
		t.Errorf("string slot: got SSP=%d top=%q; want SSP=1 top=\"mixed\"", s.SSP, s.StringStack[0])
	}
}

// TestHandleDbGetFieldCount_Basic verifies fieldCount=1 → push 1.
func TestHandleDbGetFieldCount_Basic(t *testing.T) {
	s := newDbState(buildDbFixture())
	s.PushInt(0)
	s.PushInt(pack(7, 0, -1))

	if err := handleDbGetFieldCount(s); err != nil {
		t.Fatalf("handleDbGetFieldCount: %v", err)
	}
	if s.ISP != 1 || s.IntStack[0] != 1 {
		t.Errorf("got ISP=%d top=%d; want ISP=1 top=1", s.ISP, s.IntStack[0])
	}
}

// TestHandleDbGetFieldCount_MultiTuple_Field3 verifies fieldCount=3 → push 3
// where the column's type count is 2 (total flat length 6, /2 = 3).
func TestHandleDbGetFieldCount_MultiTuple_Field3(t *testing.T) {
	cfg := buildDbFixture()
	cfg.tables[7].Types[0] = []objtype.ScriptVarType{objtype.ScriptVarTypeInt, objtype.ScriptVarTypeString}
	cfg.rows[0].Types[0] = []objtype.ScriptVarType{objtype.ScriptVarTypeInt, objtype.ScriptVarTypeString}
	cfg.rows[0].IntValues[0] = []int32{1, 0, 2, 0, 3, 0} // 6 entries, fieldCount=3
	cfg.rows[0].StringValues[0] = []string{"", "a", "", "b", "", "c"}

	s := newDbState(cfg)
	s.PushInt(0)
	s.PushInt(pack(7, 0, -1))

	if err := handleDbGetFieldCount(s); err != nil {
		t.Fatalf("handleDbGetFieldCount: %v", err)
	}
	if s.ISP != 1 || s.IntStack[0] != 3 {
		t.Errorf("got ISP=%d top=%d; want ISP=1 top=3", s.ISP, s.IntStack[0])
	}
}

// TestHandleDbGetFieldCount_CrossTableZero returns 0 when row.TableID !=
// packed table.
func TestHandleDbGetFieldCount_CrossTableZero(t *testing.T) {
	cfg := buildDbFixture()
	cfg.rows[0].TableID = 99
	s := newDbState(cfg)
	s.PushInt(0)
	s.PushInt(pack(7, 0, -1))

	if err := handleDbGetFieldCount(s); err != nil {
		t.Fatalf("handleDbGetFieldCount: %v", err)
	}
	if s.ISP != 1 || s.IntStack[0] != 0 {
		t.Errorf("got ISP=%d top=%d; want ISP=1 top=0", s.ISP, s.IntStack[0])
	}
}

// TestHandleDbGetFieldCount_InvalidRow returns validator error.
func TestHandleDbGetFieldCount_InvalidRow(t *testing.T) {
	s := newDbState(buildDbFixture())
	s.PushInt(99)
	s.PushInt(pack(7, 0, -1))
	if err := handleDbGetFieldCount(s); err == nil {
		t.Fatal("expected validator error, got nil")
	}
}

// TestHandleDbGetFieldCount_InvalidTable returns validator error.
func TestHandleDbGetFieldCount_InvalidTable(t *testing.T) {
	s := newDbState(buildDbFixture())
	s.PushInt(0)
	s.PushInt(pack(999, 0, -1))
	if err := handleDbGetFieldCount(s); err == nil {
		t.Fatal("expected validator error, got nil")
	}
}

// TestHandleDbGetRowTable_Basic verifies a valid row pushes its TableID.
func TestHandleDbGetRowTable_Basic(t *testing.T) {
	s := newDbState(buildDbFixture())
	s.PushInt(0)

	if err := handleDbGetRowTable(s); err != nil {
		t.Fatalf("handleDbGetRowTable: %v", err)
	}
	if s.ISP != 1 || s.IntStack[0] != 7 {
		t.Errorf("got ISP=%d top=%d; want ISP=1 top=7", s.ISP, s.IntStack[0])
	}
}

// TestHandleDbGetRowTable_InvalidRow returns validator error.
func TestHandleDbGetRowTable_InvalidRow(t *testing.T) {
	s := newDbState(buildDbFixture())
	s.PushInt(99)
	if err := handleDbGetRowTable(s); err == nil {
		t.Fatal("expected validator error, got nil")
	}
}

// TestHandleDbListAll_PopulatesState verifies state is set and no count
// is pushed.
func TestHandleDbListAll_PopulatesState(t *testing.T) {
	s := newDbState(buildDbFixture())
	s.PushInt(7)

	if err := handleDbListAll(s); err != nil {
		t.Fatalf("handleDbListAll: %v", err)
	}
	if s.DbTable == nil || s.DbTable.ID != 7 {
		t.Errorf("DbTable: got %v, want table id=7", s.DbTable)
	}
	if s.DbRow != -1 {
		t.Errorf("DbRow: got %d, want -1", s.DbRow)
	}
	if len(s.DbRowQuery) != 2 || s.DbRowQuery[0] != 0 || s.DbRowQuery[1] != 1 {
		t.Errorf("DbRowQuery: got %v, want [0 1]", s.DbRowQuery)
	}
	if s.ISP != 0 {
		t.Errorf("ISP: got %d, want 0 (no count pushed for DB_LISTALL)", s.ISP)
	}
}

// TestHandleDbListAll_EmptyTable verifies an empty table leaves the query
// empty and the cursor reset.
func TestHandleDbListAll_EmptyTable(t *testing.T) {
	cfg := buildDbFixture()
	cfg.rowsByT[7] = nil // make table 7 empty
	s := newDbState(cfg)
	s.PushInt(7)

	if err := handleDbListAll(s); err != nil {
		t.Fatalf("handleDbListAll: %v", err)
	}
	if len(s.DbRowQuery) != 0 {
		t.Errorf("DbRowQuery: got %v, want empty", s.DbRowQuery)
	}
}

// TestHandleDbListAll_InvalidTable returns validator error.
func TestHandleDbListAll_InvalidTable(t *testing.T) {
	s := newDbState(buildDbFixture())
	s.PushInt(999)
	if err := handleDbListAll(s); err == nil {
		t.Fatal("expected validator error, got nil")
	}
}

// TestHandleDbListAllWithCount_PushesCount verifies state population and
// count push.
func TestHandleDbListAllWithCount_PushesCount(t *testing.T) {
	s := newDbState(buildDbFixture())
	s.PushInt(7)

	if err := handleDbListAllWithCount(s); err != nil {
		t.Fatalf("handleDbListAllWithCount: %v", err)
	}
	if s.DbTable == nil || s.DbRow != -1 {
		t.Errorf("state: got DbTable=%v DbRow=%d", s.DbTable, s.DbRow)
	}
	if s.ISP != 1 || s.IntStack[0] != 2 {
		t.Errorf("count push: got ISP=%d top=%d; want ISP=1 top=2", s.ISP, s.IntStack[0])
	}
}

// TestHandleDbFindNext_AfterListAll_Advances verifies FINDNEXT advances
// the cursor from -1 → 0 and pushes the first row id.
func TestHandleDbFindNext_AfterListAll_Advances(t *testing.T) {
	s := newDbState(buildDbFixture())
	s.PushInt(7)
	if err := handleDbListAll(s); err != nil {
		t.Fatalf("handleDbListAll: %v", err)
	}

	if err := handleDbFindNext(s); err != nil {
		t.Fatalf("handleDbFindNext: %v", err)
	}
	if s.DbRow != 0 {
		t.Errorf("DbRow: got %d, want 0", s.DbRow)
	}
	if s.ISP != 1 || s.IntStack[0] != 0 {
		t.Errorf("push: got ISP=%d top=%d; want ISP=1 top=0", s.ISP, s.IntStack[0])
	}
}

// TestHandleDbFindNext_AtEnd_PushesNegativeOne verifies -1 is pushed when
// the cursor is past the last row.
func TestHandleDbFindNext_AtEnd_PushesNegativeOne(t *testing.T) {
	s := newDbState(buildDbFixture())
	s.PushInt(7)
	_ = handleDbListAll(s)
	s.DbRow = len(s.DbRowQuery) - 1 // simulate cursor at last row

	if err := handleDbFindNext(s); err != nil {
		t.Fatalf("handleDbFindNext: %v", err)
	}
	if s.ISP != 1 || s.IntStack[0] != -1 {
		t.Errorf("got ISP=%d top=%d; want ISP=1 top=-1", s.ISP, s.IntStack[0])
	}
}

// TestHandleDbFindNext_NoTableSelected returns an error.
func TestHandleDbFindNext_NoTableSelected(t *testing.T) {
	s := newDbState(buildDbFixture())
	if err := handleDbFindNext(s); err == nil {
		t.Fatal("expected \"no table selected\" error, got nil")
	}
}

// TestHandleDbFindNext_InvalidRowID returns the validator error.
func TestHandleDbFindNext_InvalidRowID(t *testing.T) {
	s := newDbState(buildDbFixture())
	s.PushInt(7)
	_ = handleDbListAll(s)
	s.DbRowQuery = []int{99} // inject an invalid id
	s.DbRow = -1

	if err := handleDbFindNext(s); err == nil {
		t.Fatal("expected validator error, got nil")
	}
}

// TestHandleDbFindByIndex_Basic pushes the row id at the given index.
func TestHandleDbFindByIndex_Basic(t *testing.T) {
	s := newDbState(buildDbFixture())
	s.PushInt(7)
	_ = handleDbListAll(s)

	s.PushInt(1)
	if err := handleDbFindByIndex(s); err != nil {
		t.Fatalf("handleDbFindByIndex: %v", err)
	}
	if s.ISP != 1 || s.IntStack[0] != 1 {
		t.Errorf("got ISP=%d top=%d; want ISP=1 top=1", s.ISP, s.IntStack[0])
	}
}

// TestHandleDbFindByIndex_Negative pushes -1.
func TestHandleDbFindByIndex_Negative(t *testing.T) {
	s := newDbState(buildDbFixture())
	s.PushInt(7)
	_ = handleDbListAll(s)

	s.PushInt(-1)
	if err := handleDbFindByIndex(s); err != nil {
		t.Fatalf("handleDbFindByIndex: %v", err)
	}
	if s.ISP != 1 || s.IntStack[0] != -1 {
		t.Errorf("got ISP=%d top=%d; want ISP=1 top=-1", s.ISP, s.IntStack[0])
	}
}

// TestHandleDbFindByIndex_BeyondEnd pushes -1.
func TestHandleDbFindByIndex_BeyondEnd(t *testing.T) {
	s := newDbState(buildDbFixture())
	s.PushInt(7)
	_ = handleDbListAll(s)

	s.PushInt(99)
	if err := handleDbFindByIndex(s); err != nil {
		t.Fatalf("handleDbFindByIndex: %v", err)
	}
	if s.ISP != 1 || s.IntStack[0] != -1 {
		t.Errorf("got ISP=%d top=%d; want ISP=1 top=-1", s.ISP, s.IntStack[0])
	}
}

// TestHandleDbFindByIndex_NoTableSelected returns an error.
func TestHandleDbFindByIndex_NoTableSelected(t *testing.T) {
	s := newDbState(buildDbFixture())
	s.PushInt(0)
	if err := handleDbFindByIndex(s); err == nil {
		t.Fatal("expected \"no table selected\" error, got nil")
	}
}

// TestHandleDbFindByIndex_InvalidRowID returns validator error.
func TestHandleDbFindByIndex_InvalidRowID(t *testing.T) {
	s := newDbState(buildDbFixture())
	s.PushInt(7)
	_ = handleDbListAll(s)
	s.DbRowQuery = []int{99}

	s.PushInt(0)
	if err := handleDbFindByIndex(s); err == nil {
		t.Fatal("expected validator error, got nil")
	}
}

// TestCursorReuse_FindByIndexDoesNotMoveFindNextCursor pins the invariant
// that FINDBYINDEX is random-access and doesn't advance the FINDNEXT cursor.
func TestCursorReuse_FindByIndexDoesNotMoveFindNextCursor(t *testing.T) {
	s := newDbState(buildDbFixture())
	s.PushInt(7)
	if err := handleDbListAll(s); err != nil {
		t.Fatalf("handleDbListAll: %v", err)
	}

	// FINDNEXT → 0
	if err := handleDbFindNext(s); err != nil {
		t.Fatalf("FINDNEXT #1: %v", err)
	}
	if s.DbRow != 0 {
		t.Fatalf("after FINDNEXT #1: DbRow=%d want 0", s.DbRow)
	}

	// FINDBYINDEX(1) → pushes 1; cursor unchanged
	s.PushInt(1)
	if err := handleDbFindByIndex(s); err != nil {
		t.Fatalf("FINDBYINDEX: %v", err)
	}
	if s.DbRow != 0 {
		t.Errorf("FINDBYINDEX moved DbRow: got %d, want 0 (unchanged)", s.DbRow)
	}

	// FINDNEXT → 1 (continues where prior FINDNEXT left off)
	if err := handleDbFindNext(s); err != nil {
		t.Fatalf("FINDNEXT #2: %v", err)
	}
	if s.DbRow != 1 {
		t.Errorf("FINDNEXT #2: DbRow=%d want 1", s.DbRow)
	}

	// FINDNEXT → -1 (past end)
	if err := handleDbFindNext(s); err != nil {
		t.Fatalf("FINDNEXT #3: %v", err)
	}
	if top := s.IntStack[s.ISP-1]; top != -1 {
		t.Errorf("FINDNEXT #3 top: got %d, want -1", top)
	}
}
