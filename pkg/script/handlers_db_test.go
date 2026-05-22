package script

import (
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/fonttype"
	"github.com/zsrv/goscape/pkg/objtype"
)

// fakeDbConfigs implements Configs with DB fixtures only; non-DB methods
// return nil. Used by all handlers_db_test.go tests.
type fakeDbConfigs struct {
	tables  map[int]*objtype.DbTableType
	rows    map[int]*objtype.DbRowType
	rowsByT map[int][]int
	index   *objtype.DbTableIndex // nil-safe; nil means DB_FIND* tests can't run
}

func (f *fakeDbConfigs) ObjType(id int) *objtype.ObjType           { return nil }
func (f *fakeDbConfigs) NpcType(id int) *objtype.NpcType           { return nil }
func (f *fakeDbConfigs) LocType(id int) *objtype.LocType           { return nil }
func (f *fakeDbConfigs) EnumType(id int) *objtype.EnumType         { return nil }
func (f *fakeDbConfigs) StructType(id int) *objtype.StructType     { return nil }
func (f *fakeDbConfigs) ParamType(id int) *objtype.ParamType       { return nil }
func (f *fakeDbConfigs) InvType(id int) *objtype.InvType           { return nil }
func (f *fakeDbConfigs) IdkType(id int) *objtype.IdkType           { return nil }
func (f *fakeDbConfigs) SpotAnimType(id int) *objtype.SpotanimType { return nil }
func (f *fakeDbConfigs) SeqType(id int) *objtype.SeqType           { return nil }
func (f *fakeDbConfigs) HuntType(id int) *objtype.HuntType         { return nil }
func (f *fakeDbConfigs) DbTableType(id int) *objtype.DbTableType {
	return f.tables[id]
}
func (f *fakeDbConfigs) DbRowType(id int) *objtype.DbRowType { return f.rows[id] }
func (f *fakeDbConfigs) DbRowsInTable(tableID int) []int     { return f.rowsByT[tableID] }
func (f *fakeDbConfigs) FindDbRowsInt(query int32, packed int) []int {
	if f.index == nil {
		return nil
	}
	return f.index.FindInt(query, packed)
}
func (f *fakeDbConfigs) FindDbRowsStr(query string, packed int) []int {
	if f.index == nil {
		return nil
	}
	return f.index.FindStr(query, packed)
}

func (f *fakeDbConfigs) VarpType(id int) (objtype.ScriptVarType, bool) {
	return objtype.ScriptVarTypeInt, false
}
func (f *fakeDbConfigs) VarnType(id int) objtype.ScriptVarType   { return objtype.ScriptVarTypeInt }
func (f *fakeDbConfigs) ObjByName(name string) *objtype.ObjType  { return nil }
func (f *fakeDbConfigs) MesanimType(id int) *objtype.MesanimType { return nil }
func (f *fakeDbConfigs) MesanimByName(name string) int           { return -1 }
func (f *fakeDbConfigs) FontType(id int) *fonttype.FontType      { return nil }

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
		t.Fatal("expected \"find_db pointer not set\" error, got nil")
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

// TestListAllSetsFindDbPointer pins that DB_LISTALL sets PtrFindDb on
// success (S7g retrofit — previously unset).
func TestListAllSetsFindDbPointer(t *testing.T) {
	tbl := objtype.NewDbTableType(1)
	cfg := &fakeDbConfigs{
		tables:  map[int]*objtype.DbTableType{1: tbl},
		rowsByT: map[int][]int{1: {10, 11}},
	}
	s := newDbState(cfg)
	s.PushInt(1) // table id

	if err := handleDbListAll(s); err != nil {
		t.Fatalf("handleDbListAll: unexpected error %v", err)
	}
	if s.Pointers&PtrFindDb == 0 {
		t.Error("DB_LISTALL: want PtrFindDb set, got unset")
	}
	if s.DbTable == nil {
		t.Error("DB_LISTALL: want DbTable set, got nil")
	}
}

// TestListAllWithCountSetsFindDbPointer — same for DB_LISTALL_WITH_COUNT.
func TestListAllWithCountSetsFindDbPointer(t *testing.T) {
	tbl := objtype.NewDbTableType(1)
	cfg := &fakeDbConfigs{
		tables:  map[int]*objtype.DbTableType{1: tbl},
		rowsByT: map[int][]int{1: {20, 21, 22}},
	}
	s := newDbState(cfg)
	s.PushInt(1)

	if err := handleDbListAllWithCount(s); err != nil {
		t.Fatalf("handleDbListAllWithCount: unexpected error %v", err)
	}
	if s.Pointers&PtrFindDb == 0 {
		t.Error("DB_LISTALL_WITH_COUNT: want PtrFindDb set, got unset")
	}
	if n := s.PopInt(); n != 3 {
		t.Errorf("count: want 3, got %d", n)
	}
}

// TestFindNextRequiresFindDbPointer pins that DB_FINDNEXT errors when
// PtrFindDb is unset — the S7g gate replacing S7d's DbTable-nil proxy.
func TestFindNextRequiresFindDbPointer(t *testing.T) {
	s := newDbState(&fakeDbConfigs{})
	// Pointers zero; DbTable nil.
	err := handleDbFindNext(s)
	if err == nil {
		t.Fatal("DB_FINDNEXT without PtrFindDb: want error, got nil")
	}
	if !strings.Contains(err.Error(), "find_db pointer not set") {
		t.Errorf("error message %q: want mention of find_db pointer", err.Error())
	}
}

// TestFindNextChainsFromListAll pins that after DB_LISTALL sets the flag,
// a chained DB_FINDNEXT advances the cursor. This is the regression-pin
// for S7d's cross-handler cursor-reuse test expanded to the new gate.
func TestFindNextChainsFromListAll(t *testing.T) {
	tbl := objtype.NewDbTableType(1)
	row10 := objtype.NewDbRowType(10)
	row10.TableID = 1
	row11 := objtype.NewDbRowType(11)
	row11.TableID = 1
	cfg := &fakeDbConfigs{
		tables:  map[int]*objtype.DbTableType{1: tbl},
		rows:    map[int]*objtype.DbRowType{10: row10, 11: row11},
		rowsByT: map[int][]int{1: {10, 11}},
	}
	s := newDbState(cfg)

	s.PushInt(1)
	if err := handleDbListAll(s); err != nil {
		t.Fatalf("handleDbListAll: %v", err)
	}
	if err := handleDbFindNext(s); err != nil {
		t.Fatalf("handleDbFindNext #1: %v", err)
	}
	if n := s.PopInt(); n != 10 {
		t.Errorf("FINDNEXT #1: want 10, got %d", n)
	}
	if err := handleDbFindNext(s); err != nil {
		t.Fatalf("handleDbFindNext #2: %v", err)
	}
	if n := s.PopInt(); n != 11 {
		t.Errorf("FINDNEXT #2: want 11, got %d", n)
	}
	// Past end → -1.
	if err := handleDbFindNext(s); err != nil {
		t.Fatalf("handleDbFindNext #3: %v", err)
	}
	if n := s.PopInt(); n != -1 {
		t.Errorf("FINDNEXT past end: want -1, got %d", n)
	}
}

// buildTestDbIndex constructs a fakeDbConfigs + real *DbTableIndex for
// the DB_FIND* test matrix. Table 1 has two INDEXED columns:
//   - col 0 (INT): row 10 → 100, row 11 → 200, row 12 → 100 (duplicate)
//   - col 1 (STRING): row 10 → "a", row 11 → "b", row 12 → "c"
//
// Used by DB_FIND / DB_FIND_WITH_COUNT / DB_FIND_REFINE* tests.
func buildTestDbIndex(t *testing.T) *fakeDbConfigs {
	t.Helper()
	tbl := objtype.NewDbTableType(1)
	tbl.Types = [][]objtype.ScriptVarType{
		{objtype.ScriptVarTypeInt},
		{objtype.ScriptVarTypeString},
	}
	tbl.Props = []uint8{objtype.DbTableFlagIndexed, objtype.DbTableFlagIndexed}

	mkRow := func(id int, intVal int32, strVal string) *objtype.DbRowType {
		r := objtype.NewDbRowType(id)
		r.TableID = 1
		r.Types = [][]objtype.ScriptVarType{
			{objtype.ScriptVarTypeInt},
			{objtype.ScriptVarTypeString},
		}
		r.IntValues = [][]int32{{intVal}, {0}}
		r.StringValues = [][]string{{""}, {strVal}}
		return r
	}
	r10 := mkRow(10, 100, "a")
	r11 := mkRow(11, 200, "b")
	r12 := mkRow(12, 100, "c")

	tables := &objtype.DbTableTypeConfigs{Configs: []*objtype.DbTableType{nil, tbl}}
	rows := &objtype.DbRowTypeConfigs{
		Configs:     []*objtype.DbRowType{nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, r10, r11, r12},
		RowsByTable: map[int][]int{1: {10, 11, 12}},
	}
	idx := objtype.BuildDbTableIndex(tables, rows)

	return &fakeDbConfigs{
		tables:  map[int]*objtype.DbTableType{1: tbl},
		rows:    map[int]*objtype.DbRowType{10: r10, 11: r11, 12: r12},
		rowsByT: map[int][]int{1: {10, 11, 12}},
		index:   idx,
	}
}

// pushDbFindArgs pushes the three DB_FIND args in RS2 stack order:
// packed (deepest), query, isString (topmost). Matches TS DbOps.ts:10-14.
func pushDbFindArgs(s *ScriptState, packed int, queryInt int, queryStr string, isString bool) {
	s.PushInt(packed)
	if isString {
		s.PushString(queryStr)
		s.PushInt(2) // TS: isString marker is 2
	} else {
		s.PushInt(queryInt)
		s.PushInt(1) // anything != 2 means int path
	}
}

// TestDbFindIntHit pins DB_FIND INT happy path: one-match query.
func TestDbFindIntHit(t *testing.T) {
	cfg := buildTestDbIndex(t)
	s := newDbState(cfg)

	packedCol0 := (1 << 12) | (0 << 4) // table 1, col 0, tuple=0
	pushDbFindArgs(s, packedCol0, 200, "", false)

	if err := handleDbFind(s); err != nil {
		t.Fatalf("handleDbFind: %v", err)
	}
	if len(s.DbRowQuery) != 1 || s.DbRowQuery[0] != 11 {
		t.Errorf("DbRowQuery: want [11], got %v", s.DbRowQuery)
	}
	if s.DbRow != -1 {
		t.Errorf("DbRow: want -1, got %d", s.DbRow)
	}
	if s.DbTable == nil || s.DbTable.ID != 1 {
		t.Errorf("DbTable: want id=1, got %v", s.DbTable)
	}
	if s.Pointers&PtrFindDb == 0 {
		t.Error("DB_FIND: want PtrFindDb set, got unset")
	}
	// No value pushed.
	if s.ISP != 0 {
		t.Errorf("int stack pointer: want 0 pushed, got ISP=%d", s.ISP)
	}
}

// TestDbFindStringPath pins isString=2 routing + FindDbRowsStr delegation.
func TestDbFindStringPath(t *testing.T) {
	cfg := buildTestDbIndex(t)
	s := newDbState(cfg)

	packedCol1 := (1 << 12) | (1 << 4)
	pushDbFindArgs(s, packedCol1, 0, "a", true)

	if err := handleDbFind(s); err != nil {
		t.Fatalf("handleDbFind: %v", err)
	}
	if len(s.DbRowQuery) != 1 || s.DbRowQuery[0] != 10 {
		t.Errorf("DbRowQuery: want [10], got %v", s.DbRowQuery)
	}
	if s.Pointers&PtrFindDb == 0 {
		t.Error("DB_FIND: want PtrFindDb set")
	}
}

// TestDbFindMultipleMatches pins that duplicate query values return all
// matching rows in RowsByTable ascending order.
func TestDbFindMultipleMatches(t *testing.T) {
	cfg := buildTestDbIndex(t)
	s := newDbState(cfg)

	packedCol0 := 1 << 12
	pushDbFindArgs(s, packedCol0, 100, "", false) // rows 10 + 12 share value 100

	if err := handleDbFind(s); err != nil {
		t.Fatalf("handleDbFind: %v", err)
	}
	want := []int{10, 12}
	if len(s.DbRowQuery) != len(want) {
		t.Fatalf("DbRowQuery: want %v, got %v", want, s.DbRowQuery)
	}
	for i := range want {
		if s.DbRowQuery[i] != want[i] {
			t.Errorf("DbRowQuery[%d]: want %d, got %d", i, want[i], s.DbRowQuery[i])
		}
	}
}

// TestDbFindInvalidTable pins that an unloaded table id errors and
// leaves state untouched (DbTable remains nil, PtrFindDb unset).
func TestDbFindInvalidTable(t *testing.T) {
	cfg := buildTestDbIndex(t)
	s := newDbState(cfg)

	packedBadTable := (99 << 12)
	pushDbFindArgs(s, packedBadTable, 100, "", false)

	err := handleDbFind(s)
	if err == nil {
		t.Fatal("DB_FIND with invalid table: want error, got nil")
	}
	if !strings.Contains(err.Error(), "DB_FIND") || !strings.Contains(err.Error(), "99") {
		t.Errorf("error %q: want mention of DB_FIND and id 99", err.Error())
	}
	if s.Pointers&PtrFindDb != 0 {
		t.Error("DB_FIND failed: PtrFindDb must NOT be set")
	}
	if s.DbTable != nil {
		t.Error("DB_FIND failed: DbTable must remain nil")
	}
}

// TestDbFindWithCountHappyPath pins DB_FIND_WITH_COUNT populates state
// AND pushes count. Crucially, does NOT set PtrFindDb (TS asymmetry).
func TestDbFindWithCountHappyPath(t *testing.T) {
	cfg := buildTestDbIndex(t)
	s := newDbState(cfg)

	packedCol0 := 1 << 12
	pushDbFindArgs(s, packedCol0, 100, "", false)

	if err := handleDbFindWithCount(s); err != nil {
		t.Fatalf("handleDbFindWithCount: %v", err)
	}
	if n := s.PopInt(); n != 2 {
		t.Errorf("count: want 2, got %d", n)
	}
	if len(s.DbRowQuery) != 2 {
		t.Errorf("DbRowQuery: want len 2, got %v", s.DbRowQuery)
	}
	// TS-asymmetry pin: DB_FIND_WITH_COUNT mutates state but does NOT set the flag.
	if s.Pointers&PtrFindDb != 0 {
		t.Error("DB_FIND_WITH_COUNT: TS omits set find_db; goscape must match")
	}
}

// TestDbFindWithCountFollowedByFindNextFails pins the TS-asymmetry quirk:
// even though DB_FIND_WITH_COUNT populated the cursor, DB_FINDNEXT fails
// because the flag is not set.
func TestDbFindWithCountFollowedByFindNextFails(t *testing.T) {
	cfg := buildTestDbIndex(t)
	s := newDbState(cfg)

	packedCol0 := 1 << 12
	pushDbFindArgs(s, packedCol0, 100, "", false)
	if err := handleDbFindWithCount(s); err != nil {
		t.Fatalf("setup: %v", err)
	}
	_ = s.PopInt() // discard count

	// DB_FINDNEXT should fail the gate despite valid cursor state.
	err := handleDbFindNext(s)
	if err == nil {
		t.Fatal("DB_FINDNEXT after DB_FIND_WITH_COUNT: want gate error, got nil")
	}
	if !strings.Contains(err.Error(), "find_db pointer not set") {
		t.Errorf("error %q: want mention of find_db pointer", err.Error())
	}
}

// TestDbFindRefineRequiresFindDbPointer pins the gate on DB_FIND_REFINE.
func TestDbFindRefineRequiresFindDbPointer(t *testing.T) {
	cfg := buildTestDbIndex(t)
	s := newDbState(cfg)

	packedCol0 := 1 << 12
	pushDbFindArgs(s, packedCol0, 100, "", false)

	err := handleDbFindRefine(s)
	if err == nil {
		t.Fatal("DB_FIND_REFINE without prior find: want error, got nil")
	}
	if !strings.Contains(err.Error(), "find_db pointer not set") {
		t.Errorf("error %q: want mention of find_db pointer", err.Error())
	}
}

// TestDbFindRefineIntersects pins basic refine behavior: after DB_FIND,
// a refining query returns the intersection of match-sets.
func TestDbFindRefineIntersects(t *testing.T) {
	cfg := buildTestDbIndex(t)
	s := newDbState(cfg)

	// First: DB_FIND on col 0 = 100 → rows {10, 12}.
	packedCol0 := 1 << 12
	pushDbFindArgs(s, packedCol0, 100, "", false)
	if err := handleDbFind(s); err != nil {
		t.Fatalf("setup DB_FIND: %v", err)
	}

	// Refine: col 1 = "c" → bucket has {12}; intersection with {10,12} = {12}.
	packedCol1 := (1 << 12) | (1 << 4)
	pushDbFindArgs(s, packedCol1, 0, "c", true)
	if err := handleDbFindRefine(s); err != nil {
		t.Fatalf("DB_FIND_REFINE: %v", err)
	}
	if len(s.DbRowQuery) != 1 || s.DbRowQuery[0] != 12 {
		t.Errorf("refined query: want [12], got %v", s.DbRowQuery)
	}
	if s.DbRow != -1 {
		t.Error("DbRow: want -1 after refine")
	}
}

// TestDbFindRefineDisjoint pins empty intersection produces empty query.
func TestDbFindRefineDisjoint(t *testing.T) {
	cfg := buildTestDbIndex(t)
	s := newDbState(cfg)

	// DB_FIND col 0 = 100 → rows {10, 12}.
	packedCol0 := 1 << 12
	pushDbFindArgs(s, packedCol0, 100, "", false)
	if err := handleDbFind(s); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Refine: col 1 = "b" → bucket has {11}; intersection = {}.
	packedCol1 := (1 << 12) | (1 << 4)
	pushDbFindArgs(s, packedCol1, 0, "b", true)
	if err := handleDbFindRefine(s); err != nil {
		t.Fatalf("DB_FIND_REFINE: %v", err)
	}
	if len(s.DbRowQuery) != 0 {
		t.Errorf("disjoint refine: want empty, got %v", s.DbRowQuery)
	}
}

// TestDbFindRefineFromListAll pins that DB_LISTALL's set flag enables
// DB_FIND_REFINE — the full gate path through the "listall" populator.
func TestDbFindRefineFromListAll(t *testing.T) {
	cfg := buildTestDbIndex(t)
	s := newDbState(cfg)

	s.PushInt(1)
	if err := handleDbListAll(s); err != nil {
		t.Fatalf("setup DB_LISTALL: %v", err)
	}
	// DbRowQuery = {10, 11, 12}; flag set.

	packedCol0 := 1 << 12
	pushDbFindArgs(s, packedCol0, 200, "", false)
	if err := handleDbFindRefine(s); err != nil {
		t.Fatalf("DB_FIND_REFINE: %v", err)
	}
	if len(s.DbRowQuery) != 1 || s.DbRowQuery[0] != 11 {
		t.Errorf("refine after listall: want [11], got %v", s.DbRowQuery)
	}
}

// TestDbFindRefinePreservesOrder pins: refined output retains prev-order,
// not found-order. Here prev is {12, 10, 11} (manually set to diverge from
// ascending); found is {11, 12} (also non-ascending). Intersection
// preserves prev order: {12, 11}.
func TestDbFindRefinePreservesOrder(t *testing.T) {
	cfg := buildTestDbIndex(t)
	s := newDbState(cfg)
	s.Pointers |= PtrFindDb // satisfy gate
	s.DbTable = cfg.tables[1]
	s.DbRowQuery = []int{12, 10, 11}

	// col 0 = 100 → found {10, 12} (ascending from index); intersect with
	// prev {12, 10, 11}: walking prev, keep {12, 10}.
	packedCol0 := 1 << 12
	pushDbFindArgs(s, packedCol0, 100, "", false)
	if err := handleDbFindRefine(s); err != nil {
		t.Fatalf("DB_FIND_REFINE: %v", err)
	}
	want := []int{12, 10}
	if len(s.DbRowQuery) != len(want) {
		t.Fatalf("refined: want %v, got %v", want, s.DbRowQuery)
	}
	for i := range want {
		if s.DbRowQuery[i] != want[i] {
			t.Errorf("refined[%d]: want %d, got %d (full: %v)", i, want[i], s.DbRowQuery[i], s.DbRowQuery)
		}
	}
}

// TestDbFindRefineWithCountAsymmetry pins the TS asymmetry: the
// _WITH_COUNT variant does NOT require PtrFindDb. Calling it on empty
// state operates on empty prev, pushes 0, no error.
func TestDbFindRefineWithCountAsymmetry(t *testing.T) {
	cfg := buildTestDbIndex(t)
	s := newDbState(cfg)
	// No prior find; Pointers zero; DbRowQuery nil.

	packedCol0 := 1 << 12
	pushDbFindArgs(s, packedCol0, 100, "", false)

	err := handleDbFindRefineWithCount(s)
	if err != nil {
		t.Fatalf("DB_FIND_REFINE_WITH_COUNT on empty: want no error (TS asymmetry), got %v", err)
	}
	if n := s.PopInt(); n != 0 {
		t.Errorf("count: want 0, got %d", n)
	}
}

// TestDbFindRefineWithCountHappyPath pins _WITH_COUNT variant matches
// DB_FIND_REFINE's state mutation AND pushes count.
func TestDbFindRefineWithCountHappyPath(t *testing.T) {
	cfg := buildTestDbIndex(t)
	s := newDbState(cfg)

	// Setup: DB_FIND col 0 = 100 → {10, 12}.
	packedCol0 := 1 << 12
	pushDbFindArgs(s, packedCol0, 100, "", false)
	if err := handleDbFind(s); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Refine col 1 = "c" → {12}, count = 1.
	packedCol1 := (1 << 12) | (1 << 4)
	pushDbFindArgs(s, packedCol1, 0, "c", true)
	if err := handleDbFindRefineWithCount(s); err != nil {
		t.Fatalf("DB_FIND_REFINE_WITH_COUNT: %v", err)
	}
	if n := s.PopInt(); n != 1 {
		t.Errorf("count: want 1, got %d", n)
	}
	if len(s.DbRowQuery) != 1 || s.DbRowQuery[0] != 12 {
		t.Errorf("refined: want [12], got %v", s.DbRowQuery)
	}
}
