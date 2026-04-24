package objtype

import (
	"testing"
)

// indexedTable returns a fresh DbTableType with the given column types
// and indexed-column flags. flags[i]==true means column i is INDEXED.
// Used only in dbtableindex_test.go; kept local to avoid leaking fixture
// helpers into the package surface.
func indexedTable(id int, types [][]ScriptVarType, indexed []bool) *DbTableType {
	tbl := NewDbTableType(id)
	tbl.Types = types
	tbl.Props = make([]uint8, len(types))
	for i, idx := range indexed {
		if idx {
			tbl.Props[i] = DbTableFlagIndexed
		}
	}
	return tbl
}

// singleColRow returns a fresh DbRowType bound to the given table,
// with one column populated. Multi-type (tuple) columns are formed by
// passing types of length > 1 and values laid out as
// [typeID + fieldID*len(types)].
func singleColRow(id, tableID, col int, types []ScriptVarType, ints []int32, strs []string) *DbRowType {
	row := NewDbRowType(id)
	row.TableID = tableID
	// Size Types / IntValues / StringValues to col+1 so the zero-value
	// slots below col stay nil (matches a real loader's sparse output).
	row.Types = make([][]ScriptVarType, col+1)
	row.IntValues = make([][]int32, col+1)
	row.StringValues = make([][]string, col+1)
	row.Types[col] = types
	row.IntValues[col] = ints
	row.StringValues[col] = strs
	return row
}

// buildIndex is a test-only convenience that wraps BuildDbTableIndex for
// single-row / single-table fixtures.
func buildIndex(tbl *DbTableType, rows ...*DbRowType) *DbTableIndex {
	tables := &DbTableTypeConfigs{
		Configs: []*DbTableType{tbl},
	}
	rowConfigs := &DbRowTypeConfigs{
		Configs:     make([]*DbRowType, 0, len(rows)),
		RowsByTable: make(map[int][]int),
	}
	for _, r := range rows {
		for len(rowConfigs.Configs) <= r.ID {
			rowConfigs.Configs = append(rowConfigs.Configs, nil)
		}
		rowConfigs.Configs[r.ID] = r
		rowConfigs.RowsByTable[r.TableID] = append(rowConfigs.RowsByTable[r.TableID], r.ID)
	}
	return BuildDbTableIndex(tables, rowConfigs)
}

// TestBuildEmptyConfigs pins: nil or empty configs produce a non-nil
// *DbTableIndex that returns nil for every Find query.
func TestBuildEmptyConfigs(t *testing.T) {
	idx := BuildDbTableIndex(&DbTableTypeConfigs{}, &DbRowTypeConfigs{})
	if idx == nil {
		t.Fatal("BuildDbTableIndex: empty configs should return non-nil index")
	}
	if got := idx.FindInt(0, 0); got != nil {
		t.Errorf("FindInt on empty index: want nil, got %v", got)
	}
	if got := idx.FindStr("", 0); got != nil {
		t.Errorf("FindStr on empty index: want nil, got %v", got)
	}
}

// TestBuildSingleIntColumn pins: a single INT-indexed column is looked up
// correctly via FindInt; FindStr on the same packed key returns nil.
func TestBuildSingleIntColumn(t *testing.T) {
	tbl := indexedTable(0,
		[][]ScriptVarType{{ScriptVarTypeInt}},
		[]bool{true},
	)
	row := singleColRow(5, 0, 0,
		[]ScriptVarType{ScriptVarTypeInt},
		[]int32{42},
		[]string{""},
	)
	idx := buildIndex(tbl, row)

	packed := 0 // tableID=0, col=0, tuple=0
	if got := idx.FindInt(42, packed); len(got) != 1 || got[0] != 5 {
		t.Errorf("FindInt(42, 0): want [5], got %v", got)
	}
	if got := idx.FindInt(43, packed); got != nil {
		t.Errorf("FindInt(43, 0) [no match]: want nil, got %v", got)
	}
	if got := idx.FindStr("42", packed); got != nil {
		t.Errorf("FindStr on INT bucket: want nil, got %v", got)
	}
}

// TestBuildSingleStringColumn pins the STRING-indexed single-column path.
func TestBuildSingleStringColumn(t *testing.T) {
	tbl := indexedTable(0,
		[][]ScriptVarType{{ScriptVarTypeString}},
		[]bool{true},
	)
	row := singleColRow(3, 0, 0,
		[]ScriptVarType{ScriptVarTypeString},
		[]int32{0},
		[]string{"target"},
	)
	idx := buildIndex(tbl, row)

	packed := 0
	if got := idx.FindStr("target", packed); len(got) != 1 || got[0] != 3 {
		t.Errorf("FindStr: want [3], got %v", got)
	}
	if got := idx.FindInt(0, packed); got != nil {
		t.Errorf("FindInt on STRING bucket: want nil, got %v", got)
	}
}

// TestBuildListColumnMultipleValues pins: a LIST column (multiple stored
// values per row) indexes each value to the same packed key; Find on any
// of the values returns the row.
func TestBuildListColumnMultipleValues(t *testing.T) {
	tbl := indexedTable(0,
		[][]ScriptVarType{{ScriptVarTypeInt}},
		[]bool{true},
	)
	tbl.Props[0] |= DbTableFlagList

	row := singleColRow(1, 0, 0,
		[]ScriptVarType{ScriptVarTypeInt},
		[]int32{10, 20, 30},
		[]string{},
	)
	idx := buildIndex(tbl, row)

	packed := 0
	for _, v := range []int32{10, 20, 30} {
		if got := idx.FindInt(v, packed); len(got) != 1 || got[0] != 1 {
			t.Errorf("FindInt(%d): want [1], got %v", v, got)
		}
	}
	if got := idx.FindInt(40, packed); got != nil {
		t.Errorf("FindInt(40): want nil, got %v", got)
	}
}

// TestBuildTupleColumn pins: an (INT, STRING) multi-type column stores
// each typeID's value at a distinct packed key (typeID=0 bucket for INT,
// typeID=1 bucket for STRING).
func TestBuildTupleColumn(t *testing.T) {
	tbl := indexedTable(0,
		[][]ScriptVarType{{ScriptVarTypeInt, ScriptVarTypeString}},
		[]bool{true},
	)
	row := singleColRow(7, 0, 0,
		[]ScriptVarType{ScriptVarTypeInt, ScriptVarTypeString},
		[]int32{100, 0}, // typeID=0 INT=100, typeID=1 STRING slot unused (0)
		[]string{"", "alpha"}, // typeID=0 STRING slot unused, typeID=1 STRING="alpha"
	)
	idx := buildIndex(tbl, row)

	// Build stores at typeID=0 (INT bucket) and typeID=1 (STRING bucket).
	// Tests call Find with 1-based nibble (bytecode convention).
	intBuildKey := 0                   // (0<<12)|(0<<4)|0
	strBuildKey := 1                   // (0<<12)|(0<<4)|1
	intFindKey := intBuildKey + 1      // 1-based: typeID 0 -> 1
	strFindKey := strBuildKey + 1      // 1-based: typeID 1 -> 2

	if got := idx.FindInt(100, intFindKey); len(got) != 1 || got[0] != 7 {
		t.Errorf("FindInt INT slot (nibble=1 → bucket=0): want [7], got %v", got)
	}
	if got := idx.FindStr("alpha", strFindKey); len(got) != 1 || got[0] != 7 {
		t.Errorf("FindStr STRING slot (nibble=2 → bucket=1): want [7], got %v", got)
	}
}

// TestBuildNonIndexedColumnSkipped pins: columns without the INDEXED
// flag are not indexed at all; Find* on their packed key returns nil.
func TestBuildNonIndexedColumnSkipped(t *testing.T) {
	tbl := indexedTable(0,
		[][]ScriptVarType{{ScriptVarTypeInt}, {ScriptVarTypeInt}},
		[]bool{false, true}, // col 0 not indexed, col 1 indexed
	)
	row := NewDbRowType(1)
	row.TableID = 0
	row.Types = [][]ScriptVarType{
		{ScriptVarTypeInt},
		{ScriptVarTypeInt},
	}
	row.IntValues = [][]int32{
		{11},
		{22},
	}
	row.StringValues = [][]string{{""}, {""}}
	idx := buildIndex(tbl, row)

	col0Packed := 0      // (0<<12)|(0<<4)
	col1Packed := 1 << 4 // (0<<12)|(1<<4)

	if got := idx.FindInt(11, col0Packed); got != nil {
		t.Errorf("col 0 (not indexed): want nil, got %v", got)
	}
	if got := idx.FindInt(22, col1Packed); len(got) != 1 || got[0] != 1 {
		t.Errorf("col 1 (indexed): want [1], got %v", got)
	}
}

// TestBuildMultipleRowsSameValue pins: multiple rows sharing the same
// query value are returned as a slice in RowsByTable ascending order.
func TestBuildMultipleRowsSameValue(t *testing.T) {
	tbl := indexedTable(0,
		[][]ScriptVarType{{ScriptVarTypeInt}},
		[]bool{true},
	)
	r1 := singleColRow(2, 0, 0, []ScriptVarType{ScriptVarTypeInt}, []int32{99}, []string{""})
	r2 := singleColRow(5, 0, 0, []ScriptVarType{ScriptVarTypeInt}, []int32{99}, []string{""})
	r3 := singleColRow(7, 0, 0, []ScriptVarType{ScriptVarTypeInt}, []int32{99}, []string{""})
	idx := buildIndex(tbl, r1, r2, r3)

	got := idx.FindInt(99, 0)
	want := []int{2, 5, 7}
	if len(got) != len(want) {
		t.Fatalf("FindInt(99): want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("FindInt(99)[%d]: want %d, got %d (full: %v)", i, want[i], got[i], got)
		}
	}
}

// TestBuildPackedKeyEdges pins that tableID=0xffff and column=0x7f both
// round-trip — no silent truncation in the packed-key bit layout.
func TestBuildPackedKeyEdges(t *testing.T) {
	tbl := indexedTable(0xffff,
		make([][]ScriptVarType, 0x80), // 128 columns to reach col 127
		make([]bool, 0x80),
	)
	tbl.Types[0x7f] = []ScriptVarType{ScriptVarTypeInt}
	tbl.Props[0x7f] = DbTableFlagIndexed

	row := NewDbRowType(1)
	row.TableID = 0xffff
	row.Types = make([][]ScriptVarType, 0x80)
	row.Types[0x7f] = []ScriptVarType{ScriptVarTypeInt}
	row.IntValues = make([][]int32, 0x80)
	row.IntValues[0x7f] = []int32{555}
	row.StringValues = make([][]string, 0x80)
	row.StringValues[0x7f] = []string{""}

	idx := buildIndex(tbl, row)

	packed := (0xffff << 12) | (0x7f << 4)
	if got := idx.FindInt(555, packed); len(got) != 1 || got[0] != 1 {
		t.Errorf("edge-bit packed key: want [1], got %v", got)
	}
}

// TestFindNonIndexedReturnsNil pins the "non-indexed column Find returns
// nil, no panic" contract.
func TestFindNonIndexedReturnsNil(t *testing.T) {
	idx := &DbTableIndex{
		intRows: map[int]map[int32][]int{},
		strRows: map[int]map[string][]int{},
	}
	if got := idx.FindInt(0, 0x12345); got != nil {
		t.Errorf("FindInt on missing bucket: want nil, got %v", got)
	}
	if got := idx.FindStr("x", 0x12345); got != nil {
		t.Errorf("FindStr on missing bucket: want nil, got %v", got)
	}
}

// TestFindNibbleNormalization pins the 1-based-query → 0-based-build
// normalization — the single most error-prone corner of the port.
// Build stores at packed key K; query with packed K+1 must also hit.
func TestFindNibbleNormalization(t *testing.T) {
	tbl := indexedTable(0,
		[][]ScriptVarType{{ScriptVarTypeInt, ScriptVarTypeString}},
		[]bool{true},
	)
	row := singleColRow(9, 0, 0,
		[]ScriptVarType{ScriptVarTypeInt, ScriptVarTypeString},
		[]int32{77, 0},
		[]string{"", "z"},
	)
	idx := buildIndex(tbl, row)

	// Build stored INT at packed=0 (typeID=0); find with nibble=1 must hit 0.
	if got := idx.FindInt(77, 1); len(got) != 1 || got[0] != 9 {
		t.Errorf("nibble=1 normalization: want [9], got %v", got)
	}
	// Build stored STRING at packed=1 (typeID=1); find with nibble=2 must hit 1.
	if got := idx.FindStr("z", 2); len(got) != 1 || got[0] != 9 {
		t.Errorf("nibble=2 normalization: want [9], got %v", got)
	}
}

// TestFindMissingQueryValueReturnsNil pins that a bucket present but
// query absent yields nil (not an empty slice).
func TestFindMissingQueryValueReturnsNil(t *testing.T) {
	tbl := indexedTable(0,
		[][]ScriptVarType{{ScriptVarTypeInt}},
		[]bool{true},
	)
	row := singleColRow(1, 0, 0, []ScriptVarType{ScriptVarTypeInt}, []int32{5}, []string{""})
	idx := buildIndex(tbl, row)

	if got := idx.FindInt(999, 0); got != nil {
		t.Errorf("absent query in present bucket: want nil, got %v", got)
	}
}

// TestBuildDeterministic pins that the same input produces the same
// output across two independent builds. Important because Find iterates
// the map's returned slice; determinism requires the slice to match.
func TestBuildDeterministic(t *testing.T) {
	build := func() *DbTableIndex {
		tbl := indexedTable(0,
			[][]ScriptVarType{{ScriptVarTypeInt}},
			[]bool{true},
		)
		r1 := singleColRow(2, 0, 0, []ScriptVarType{ScriptVarTypeInt}, []int32{50}, []string{""})
		r2 := singleColRow(4, 0, 0, []ScriptVarType{ScriptVarTypeInt}, []int32{50}, []string{""})
		r3 := singleColRow(6, 0, 0, []ScriptVarType{ScriptVarTypeInt}, []int32{50}, []string{""})
		return buildIndex(tbl, r1, r2, r3)
	}
	a, b := build(), build()
	got1, got2 := a.FindInt(50, 0), b.FindInt(50, 0)
	if len(got1) != len(got2) {
		t.Fatalf("determinism: len mismatch %v vs %v", got1, got2)
	}
	for i := range got1 {
		if got1[i] != got2[i] {
			t.Errorf("determinism: index %d differs %d vs %d", i, got1[i], got2[i])
		}
	}
}
