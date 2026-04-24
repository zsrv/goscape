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
