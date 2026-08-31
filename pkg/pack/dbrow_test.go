package pack

import (
	"bytes"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// buildDbTableTypes constructs a *DbTableTypeConfigs with one DbTableType
// having the given column tuples.
func buildDbTableTypes(t *testing.T, tableID int, types [][]objtype.ScriptVarType, colNames []string, props []uint8) *objtype.DbTableTypeConfigs {
	t.Helper()
	cfgs := make([]*objtype.DbTableType, tableID+1)
	cfgs[tableID] = &objtype.DbTableType{
		ID: tableID, DebugName: "t_test",
		Types:       types,
		DefaultInts: make([][]int32, len(types)),
		DefaultStrs: make([][]string, len(types)),
		ColumnNames: colNames,
		Props:       props,
	}
	return &objtype.DbTableTypeConfigs{
		ConfigNames: map[string]int{"t_test": tableID},
		Configs:     cfgs,
	}
}

// buildParamLookupsForDbRowTest constructs a paramLookups with empty PackFiles for
// the type lookups not exercised by the test.
func buildParamLookupsForDbRowTest(t *testing.T) *paramLookups {
	t.Helper()
	lk := &paramLookups{}
	for _, dst := range []**PackFile{
		&lk.enumPF, &lk.objPF, &lk.locPF, &lk.interfacePF, &lk.structPF,
		&lk.categoryPF, &lk.spotanimPF, &lk.npcPF, &lk.invPF, &lk.synthPF,
		&lk.seqPF, &lk.varpPF, &lk.dbrowPF,
	} {
		*dst = newTestPF("dummy", map[int]string{})
	}
	return lk
}

func TestPackDbRowConfigs_RowWithSingleColumn(t *testing.T) {
	// table id=0, single column "col_name" of type [int], no props
	dbtableTypes := buildDbTableTypes(t, 0,
		[][]objtype.ScriptVarType{{objtype.ScriptVarTypeInt}},
		[]string{"col_name"},
		[]uint8{0},
	)
	pf := newTestPF("dbrow", map[int]string{0: "r_one"})
	configs := map[string][]ConfigLine{
		"r_one": {
			{Key: "table", Value: 0}, // already resolved by parser
			{Key: "data", Value: "col_name,42"},
		},
	}
	pd, err := packDbRowConfigs(configs, pf, dbtableTypes, buildParamLookupsForDbRowTest(t))
	if err != nil {
		t.Fatal(err)
	}
	// Layout:
	//   2-byte count header: 0x00, 0x01
	//   opcode 3: 3 | col-count=1 | col-id=0 | type-len=1 | 'i'(105) | field-count=1 | P4(42) | end=255
	//   opcode 4: 4 | P2(0)
	//   opcode 250: 250 | "r_one"\n
	//   Next() terminator: 0x00
	want := []byte{
		0x00, 0x01, // count header
		3, 1, 0, 1, 105, 1, 0, 0, 0, 42, 255, // opcode 3
		4, 0, 0, // opcode 4
		250, 'r', '_', 'o', 'n', 'e', 0x0a, // opcode 250
		0x00, // Next() terminator
	}
	if !bytes.Equal(pd.Dat.Data, want) {
		t.Fatalf("got % x\nwant % x", pd.Dat.Data, want)
	}
}

func TestPackDbRowConfigs_NoTableDefinedError(t *testing.T) {
	dbtableTypes := buildDbTableTypes(t, 0,
		[][]objtype.ScriptVarType{{objtype.ScriptVarTypeInt}},
		[]string{"col_name"},
		[]uint8{0},
	)
	pf := newTestPF("dbrow", map[int]string{0: "r_notbl"})
	// No "table" line in config
	configs := map[string][]ConfigLine{
		"r_notbl": {
			{Key: "data", Value: "col_name,42"},
		},
	}
	_, err := packDbRowConfigs(configs, pf, dbtableTypes, buildParamLookupsForDbRowTest(t))
	if err == nil {
		t.Fatal("want error for missing table, got nil")
	}
	if !strings.Contains(err.Error(), "r_notbl") {
		t.Fatalf("err=%q, want debugname 'r_notbl'", err)
	}
	if !strings.Contains(err.Error(), "No table defined for dbrow") {
		t.Fatalf("err=%q, want 'No table defined for dbrow'", err)
	}
}

func TestPackDbRowConfigs_RequiredColumnMissingError(t *testing.T) {
	// Table has two columns: column 0 (REQUIRED) and column 1 (optional).
	// Config provides data for the optional column only → REQUIRED fires.
	// The TS REQUIRED check sits inside the if (data.length > 0) block, so
	// at least one data= line is needed to enter that block; the error fires
	// when iterating the REQUIRED column and finding zero matching fields.
	dbtableTypes := buildDbTableTypes(t, 0,
		[][]objtype.ScriptVarType{
			{objtype.ScriptVarTypeInt}, // col 0: REQUIRED
			{objtype.ScriptVarTypeInt}, // col 1: optional
		},
		[]string{"required_col", "optional_col"},
		[]uint8{objtype.DbTableFlagRequired, 0},
	)
	pf := newTestPF("dbrow", map[int]string{0: "r_req"})
	configs := map[string][]ConfigLine{
		"r_req": {
			{Key: "table", Value: 0},
			// data for optional_col only; required_col has no data
			{Key: "data", Value: "optional_col,99"},
		},
	}
	_, err := packDbRowConfigs(configs, pf, dbtableTypes, buildParamLookupsForDbRowTest(t))
	if err == nil {
		t.Fatal("want error for REQUIRED column missing data, got nil")
	}
	if !strings.Contains(err.Error(), "r_req") {
		t.Fatalf("err=%q, want debugname 'r_req'", err)
	}
	if !strings.Contains(err.Error(), "REQUIRED") {
		t.Fatalf("err=%q, want 'REQUIRED'", err)
	}
}

func TestPackDbRowConfigs_NonListColumnWithMultipleDataError(t *testing.T) {
	// column has no LIST flag, but two data lines for same column
	dbtableTypes := buildDbTableTypes(t, 0,
		[][]objtype.ScriptVarType{{objtype.ScriptVarTypeInt}},
		[]string{"plain_col"},
		[]uint8{0},
	)
	pf := newTestPF("dbrow", map[int]string{0: "r_multi"})
	configs := map[string][]ConfigLine{
		"r_multi": {
			{Key: "table", Value: 0},
			{Key: "data", Value: "plain_col,1"},
			{Key: "data", Value: "plain_col,2"},
		},
	}
	_, err := packDbRowConfigs(configs, pf, dbtableTypes, buildParamLookupsForDbRowTest(t))
	if err == nil {
		t.Fatal("want error for non-LIST column with multiple data, got nil")
	}
	if !strings.Contains(err.Error(), "r_multi") {
		t.Fatalf("err=%q, want debugname 'r_multi'", err)
	}
	if !strings.Contains(err.Error(), "LIST") {
		t.Fatalf("err=%q, want 'LIST'", err)
	}
}

func TestPackDbRowConfigs_OnlyTableNoData(t *testing.T) {
	// table= line only, no data= → opcode 3 NOT emitted; opcode 4 + 250 emitted
	dbtableTypes := buildDbTableTypes(t, 0,
		[][]objtype.ScriptVarType{{objtype.ScriptVarTypeInt}},
		[]string{"col_name"},
		[]uint8{0},
	)
	pf := newTestPF("dbrow", map[int]string{0: "r_nodata"})
	configs := map[string][]ConfigLine{
		"r_nodata": {
			{Key: "table", Value: 0},
		},
	}
	pd, err := packDbRowConfigs(configs, pf, dbtableTypes, buildParamLookupsForDbRowTest(t))
	if err != nil {
		t.Fatal(err)
	}
	// Opcode 3 must NOT appear
	if bytes.Contains(pd.Dat.Data, []byte{3}) {
		t.Fatalf("opcode 3 must not be emitted when data is empty; got % x", pd.Dat.Data)
	}
	// Opcode 4 must appear
	if !bytes.Contains(pd.Dat.Data, []byte{4, 0, 0}) {
		t.Fatalf("opcode 4 (table-id) must be emitted; got % x", pd.Dat.Data)
	}
	// Opcode 250 must appear
	if !bytes.Contains(pd.Dat.Data, []byte{250}) {
		t.Fatalf("opcode 250 (debugname) must be emitted; got % x", pd.Dat.Data)
	}
}

func TestParseDbRowConfigFor_TableResolution(t *testing.T) {
	dbtablePF := newTestPF("dbtable", map[int]string{7: "t_table"})
	parseFn := parseDbRowConfigFor(dbtablePF)

	v, claimed, err := parseFn("table", "t_table")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !claimed {
		t.Fatal("expected claimed=true for 'table' key")
	}
	if v.(int) != 7 {
		t.Fatalf("got %v, want 7", v)
	}
}

func TestParseDbRowConfigFor_UnknownTableRejected(t *testing.T) {
	dbtablePF := newTestPF("dbtable", map[int]string{7: "t_table"})
	parseFn := parseDbRowConfigFor(dbtablePF)

	v, claimed, err := parseFn("table", "t_unknown")
	if err == nil {
		t.Fatal("expected error for unknown table name, got nil")
	}
	if !claimed {
		t.Fatal("expected claimed=true even on error (key is recognized)")
	}
	if v != nil {
		t.Fatalf("expected nil value on error, got %v", v)
	}
}
