package objtype

import (
	"testing"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

type dbRowEntry struct {
	debugName string
	tableID   int
	columns   []dbRowColumn // which columns this row defines
}

type dbRowColumn struct {
	column     int
	types      []ScriptVarType
	fieldCount int
	ints       []int32  // flat: len(types)*fieldCount
	strs       []string // parallel to ints
}

// buildDbRowDat assembles a dbrow.dat wire blob.
func buildDbRowDat(entries []dbRowEntry) []byte {
	pkt := packet2.NewPacket(nil)
	pkt.P2(uint16(len(entries)))
	for _, e := range entries {
		// code 3: columns (emitted even when empty — matches real cache)
		pkt.P1(3)
		pkt.P1(uint8(numColumnsFor(e)))
		for _, c := range e.columns {
			pkt.P1(uint8(c.column))
			pkt.P1(uint8(len(c.types)))
			for _, tt := range c.types {
				pkt.P1(uint8(tt))
			}
			pkt.P1(uint8(c.fieldCount))
			for fieldID := range c.fieldCount {
				for typeID, tt := range c.types {
					idx := typeID + fieldID*len(c.types)
					if tt == ScriptVarTypeString {
						pkt.PJStrLF(c.strs[idx])
					} else {
						pkt.P4(uint32(c.ints[idx]))
					}
				}
			}
		}
		pkt.P1(255)

		if e.tableID != 0 {
			pkt.P1(4)
			pkt.P2(uint16(e.tableID))
		}
		if e.debugName != "" {
			pkt.P1(250)
			pkt.PJStrLF(e.debugName)
		}
		pkt.P1(0)
	}
	return pkt.Bytes()
}

// numColumnsFor returns the total declared column slot count for a row;
// in the real cache this is the highest column index + 1 or equal to the
// table's columnCount. For test simplicity we use len(columns) when every
// column index is contiguous, else max+1.
func numColumnsFor(e dbRowEntry) int {
	max := -1
	for _, c := range e.columns {
		if c.column > max {
			max = c.column
		}
	}
	return max + 1
}

// TestParseDbRowTypes exercises a two-row fixture covering codes 3, 4, 250.
func TestParseDbRowTypes(t *testing.T) {
	entries := []dbRowEntry{
		{
			debugName: "damage_normal",
			tableID:   7,
			columns: []dbRowColumn{
				{column: 0, types: []ScriptVarType{ScriptVarTypeInt}, fieldCount: 1, ints: []int32{1}, strs: []string{""}},
				{column: 1, types: []ScriptVarType{ScriptVarTypeString}, fieldCount: 1, ints: []int32{0}, strs: []string{"Normal"}},
			},
		},
		{
			debugName: "damage_magic",
			tableID:   7,
			columns: []dbRowColumn{
				{column: 0, types: []ScriptVarType{ScriptVarTypeInt}, fieldCount: 1, ints: []int32{2}, strs: []string{""}},
			},
		},
	}

	cfgs, err := parseDbRowTypes(packet2.NewPacket(buildDbRowDat(entries)))
	if err != nil {
		t.Fatalf("parseDbRowTypes: %v", err)
	}
	if len(cfgs.Configs) != 2 {
		t.Fatalf("configs: got %d, want 2", len(cfgs.Configs))
	}

	normal := cfgs.Configs[0]
	if normal.TableID != 7 {
		t.Errorf("TableID: got %d, want 7", normal.TableID)
	}
	if normal.DebugName != "damage_normal" {
		t.Errorf("DebugName: got %q", normal.DebugName)
	}
	if normal.IntValues[0][0] != 1 {
		t.Errorf("IntValues[0][0]: got %d, want 1", normal.IntValues[0][0])
	}
	if normal.StringValues[1][0] != "Normal" {
		t.Errorf("StringValues[1][0]: got %q, want Normal", normal.StringValues[1][0])
	}

	if got := cfgs.RowsByTable[7]; len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Errorf("RowsByTable[7]: got %v, want [0 1]", got)
	}
	if cfgs.ConfigNames["damage_normal"] != 0 {
		t.Errorf("ConfigNames[damage_normal]: got %d", cfgs.ConfigNames["damage_normal"])
	}
}

// TestDbRowMultiTuple verifies multi-type column + multi-field decoding.
func TestDbRowMultiTuple(t *testing.T) {
	entries := []dbRowEntry{
		{
			debugName: "tuple_row",
			tableID:   1,
			columns: []dbRowColumn{
				{
					column:     0,
					types:      []ScriptVarType{ScriptVarTypeInt, ScriptVarTypeString},
					fieldCount: 3,
					ints:       []int32{10, 0, 20, 0, 30, 0},
					strs:       []string{"", "a", "", "b", "", "c"},
				},
			},
		},
	}
	cfgs, err := parseDbRowTypes(packet2.NewPacket(buildDbRowDat(entries)))
	if err != nil {
		t.Fatalf("parseDbRowTypes: %v", err)
	}
	row := cfgs.Configs[0]
	if n := len(row.IntValues[0]); n != 6 {
		t.Fatalf("IntValues[0]: got len %d, want 6", n)
	}
	if row.IntValues[0][0] != 10 || row.IntValues[0][2] != 20 || row.IntValues[0][4] != 30 {
		t.Errorf("IntValues[0]: got %v", row.IntValues[0])
	}
	if row.StringValues[0][1] != "a" || row.StringValues[0][3] != "b" || row.StringValues[0][5] != "c" {
		t.Errorf("StringValues[0]: got %v", row.StringValues[0])
	}
}

// TestDbRowUnknownCode verifies the loader rejects unknown codes.
func TestDbRowUnknownCode(t *testing.T) {
	pkt := packet2.NewPacket(nil)
	pkt.P2(1)
	pkt.P1(77) // bogus
	pkt.P1(0)
	_, err := parseDbRowTypes(packet2.NewPacket(pkt.Bytes()))
	if err == nil {
		t.Fatal("expected error on unknown dbrow code, got nil")
	}
}

// TestDbRowGetValue_InRange slices the requested tuple out of the flat layout.
func TestDbRowGetValue_InRange(t *testing.T) {
	tbl := NewDbTableType(0)
	tbl.Types = [][]ScriptVarType{{ScriptVarTypeInt, ScriptVarTypeString}}
	tbl.DefaultInts = [][]int32{nil}
	tbl.DefaultStrs = [][]string{nil}

	row := NewDbRowType(0)
	row.TableID = 0
	row.Types = [][]ScriptVarType{{ScriptVarTypeInt, ScriptVarTypeString}}
	row.IntValues = [][]int32{{10, 0, 20, 0}}
	row.StringValues = [][]string{{"", "a", "", "b"}}

	ints, strs, types := row.GetValue(0, 1, tbl)
	if len(ints) != 2 || ints[0] != 20 {
		t.Errorf("ints: got %v, want [20 0]", ints)
	}
	if len(strs) != 2 || strs[1] != "b" {
		t.Errorf("strs: got %v, want [\"\" b]", strs)
	}
	if len(types) != 2 {
		t.Errorf("types: got %v", types)
	}
}

// TestDbRowGetValue_OutOfRange_FallsBack verifies the default fallback path
// when listIndex exceeds the stored field count.
func TestDbRowGetValue_OutOfRange_FallsBack(t *testing.T) {
	tbl := NewDbTableType(0)
	tbl.Types = [][]ScriptVarType{{ScriptVarTypeInt}}
	tbl.DefaultInts = [][]int32{{99}}
	tbl.DefaultStrs = [][]string{{""}}

	row := NewDbRowType(0)
	row.TableID = 0
	row.Types = [][]ScriptVarType{{ScriptVarTypeInt}}
	row.IntValues = [][]int32{{5}} // fieldCount=1
	row.StringValues = [][]string{{""}}

	ints, _, _ := row.GetValue(0, 5, tbl) // listIndex way out of range
	if len(ints) != 1 || ints[0] != 99 {
		t.Errorf("expected fallback to default [99], got %v", ints)
	}
}
