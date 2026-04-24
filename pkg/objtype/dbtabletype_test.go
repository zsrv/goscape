package objtype

import (
	"testing"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

// dbTableEntry is the test-side description of one DbTableType record.
// Only the fields used by the test are populated; zero values are skipped.
type dbTableEntry struct {
	debugName   string
	columnCount int // total column slots (types array length)
	columns     []dbTableColumn
	columnNames []string
	props       []uint8
}

type dbTableColumn struct {
	column       int
	types        []ScriptVarType
	hasDefault   bool
	defaultInts  []int32  // flat layout, length fieldCount*len(types)
	defaultStrs  []string // parallel to defaultInts
	defaultCount int      // fieldCount
}

// buildDbTableDat assembles a dbtable.dat wire blob matching parseDbTableTypes.
func buildDbTableDat(entries []dbTableEntry) []byte {
	pkt := packet2.NewPacket(nil)
	pkt.P2(uint16(len(entries)))
	for _, e := range entries {
		if e.columnCount > 0 {
			pkt.P1(1)
			pkt.P1(uint8(e.columnCount))
			for _, c := range e.columns {
				setting := uint8(c.column & 0x7f)
				if c.hasDefault {
					setting |= 0x80
				}
				pkt.P1(setting)
				pkt.P1(uint8(len(c.types)))
				for _, tt := range c.types {
					pkt.P1(uint8(tt))
				}
				if c.hasDefault {
					pkt.P1(uint8(c.defaultCount))
					for fieldID := range c.defaultCount {
						for typeID, tt := range c.types {
							idx := typeID + fieldID*len(c.types)
							if tt == ScriptVarTypeString {
								pkt.PJStrLF(c.defaultStrs[idx])
							} else {
								pkt.P4(uint32(c.defaultInts[idx]))
							}
						}
					}
				}
			}
			pkt.P1(255) // terminator
		}
		if len(e.columnNames) > 0 {
			pkt.P1(251)
			pkt.P1(uint8(len(e.columnNames)))
			for _, s := range e.columnNames {
				pkt.PJStrLF(s)
			}
		}
		if len(e.props) > 0 {
			pkt.P1(252)
			pkt.P1(uint8(len(e.props)))
			for _, p := range e.props {
				pkt.P1(p)
			}
		}
		if e.debugName != "" {
			pkt.P1(250)
			pkt.PJStrLF(e.debugName)
		}
		pkt.P1(0)
	}
	return pkt.Bytes()
}

// TestParseDbTableTypes exercises parseDbTableTypes end-to-end with a
// two-config fixture covering code 1 (schema), 250 (DebugName), 251
// (ColumnNames), 252 (Props).
func TestParseDbTableTypes(t *testing.T) {
	entries := []dbTableEntry{
		{
			debugName:   "damagetype",
			columnCount: 2,
			columns: []dbTableColumn{
				{column: 0, types: []ScriptVarType{ScriptVarTypeInt}},
				{column: 1, types: []ScriptVarType{ScriptVarTypeString}, hasDefault: true, defaultCount: 1, defaultInts: []int32{0}, defaultStrs: []string{"normal"}},
			},
			columnNames: []string{"id", "name"},
			props:       []uint8{DbTableFlagIndexed, 0},
		},
		{
			debugName:   "simple",
			columnCount: 1,
			columns: []dbTableColumn{
				{column: 0, types: []ScriptVarType{ScriptVarTypeInt}},
			},
		},
	}

	cfgs, err := parseDbTableTypes(packet2.NewPacket(buildDbTableDat(entries)))
	if err != nil {
		t.Fatalf("parseDbTableTypes: %v", err)
	}
	if len(cfgs.Configs) != 2 {
		t.Fatalf("configs: got %d, want 2", len(cfgs.Configs))
	}

	dmg := cfgs.Configs[0]
	if dmg.DebugName != "damagetype" {
		t.Errorf("DebugName[0]: got %q, want %q", dmg.DebugName, "damagetype")
	}
	if len(dmg.Types) != 2 {
		t.Fatalf("Types[0]: got len %d, want 2", len(dmg.Types))
	}
	if len(dmg.Types[0]) != 1 || dmg.Types[0][0] != ScriptVarTypeInt {
		t.Errorf("Types[0][0]: got %v, want [INT]", dmg.Types[0])
	}
	if len(dmg.Types[1]) != 1 || dmg.Types[1][0] != ScriptVarTypeString {
		t.Errorf("Types[0][1]: got %v, want [STRING]", dmg.Types[1])
	}
	if dmg.DefaultStrs[1] == nil || dmg.DefaultStrs[1][0] != "normal" {
		t.Errorf("DefaultStrs[1]: got %v, want [normal]", dmg.DefaultStrs[1])
	}
	if dmg.DefaultInts[0] != nil {
		t.Errorf("DefaultInts[0]: got %v, want nil (no stored default)", dmg.DefaultInts[0])
	}
	if got := dmg.ColumnNames; len(got) != 2 || got[0] != "id" || got[1] != "name" {
		t.Errorf("ColumnNames: got %v", got)
	}
	if got := dmg.Props; len(got) != 2 || got[0] != DbTableFlagIndexed || got[1] != 0 {
		t.Errorf("Props: got %v", got)
	}

	if cfgs.ConfigNames["damagetype"] != 0 {
		t.Errorf("ConfigNames[damagetype]: got %d, want 0", cfgs.ConfigNames["damagetype"])
	}
	if cfgs.ConfigNames["simple"] != 1 {
		t.Errorf("ConfigNames[simple]: got %d, want 1", cfgs.ConfigNames["simple"])
	}
}

// TestDbTableMixedTuple covers a multi-type column with multiple fields
// (the tuple + list shape).
func TestDbTableMixedTuple(t *testing.T) {
	entries := []dbTableEntry{
		{
			debugName:   "tuple_mix",
			columnCount: 1,
			columns: []dbTableColumn{
				{
					column:       0,
					types:        []ScriptVarType{ScriptVarTypeInt, ScriptVarTypeString, ScriptVarTypeBoolean},
					hasDefault:   true,
					defaultCount: 2,
					defaultInts:  []int32{10, 0, 1, 20, 0, 0},
					defaultStrs:  []string{"", "a", "", "", "b", ""},
				},
			},
		},
	}

	cfgs, err := parseDbTableTypes(packet2.NewPacket(buildDbTableDat(entries)))
	if err != nil {
		t.Fatalf("parseDbTableTypes: %v", err)
	}
	got := cfgs.Configs[0]
	if n := len(got.DefaultInts[0]); n != 6 {
		t.Fatalf("DefaultInts[0]: got len %d, want 6", n)
	}
	if got.DefaultInts[0][0] != 10 || got.DefaultInts[0][3] != 20 {
		t.Errorf("DefaultInts[0]: got %v", got.DefaultInts[0])
	}
	if got.DefaultStrs[0][1] != "a" || got.DefaultStrs[0][4] != "b" {
		t.Errorf("DefaultStrs[0]: got %v", got.DefaultStrs[0])
	}
	if got.DefaultInts[0][2] != 1 || got.DefaultInts[0][5] != 0 {
		t.Errorf("DefaultInts[0] BOOLEAN slots: got %v", got.DefaultInts[0])
	}
}

// TestDbTableSparseColumns verifies that a declared columnCount of 4 with
// only columns 0 and 2 provided leaves columns 1 and 3 as nil.
func TestDbTableSparseColumns(t *testing.T) {
	entries := []dbTableEntry{
		{
			debugName:   "sparse",
			columnCount: 4,
			columns: []dbTableColumn{
				{column: 0, types: []ScriptVarType{ScriptVarTypeInt}},
				{column: 2, types: []ScriptVarType{ScriptVarTypeString}},
			},
		},
	}
	cfgs, err := parseDbTableTypes(packet2.NewPacket(buildDbTableDat(entries)))
	if err != nil {
		t.Fatalf("parseDbTableTypes: %v", err)
	}
	got := cfgs.Configs[0]
	if got.Types[0] == nil || got.Types[2] == nil {
		t.Errorf("expected Types[0] and Types[2] populated, got %v and %v", got.Types[0], got.Types[2])
	}
	if got.Types[1] != nil || got.Types[3] != nil {
		t.Errorf("expected Types[1] and Types[3] nil, got %v and %v", got.Types[1], got.Types[3])
	}
}

// TestDbTableUnknownCode verifies the loader rejects unknown codes.
func TestDbTableUnknownCode(t *testing.T) {
	pkt := packet2.NewPacket(nil)
	pkt.P2(1)
	pkt.P1(77) // bogus
	pkt.P1(0)
	_, err := parseDbTableTypes(packet2.NewPacket(pkt.Bytes()))
	if err == nil {
		t.Fatal("expected error on unknown dbtable code, got nil")
	}
}

// TestDbTableGetDefault_Stored verifies GetDefault returns the stored slices
// verbatim when a default was decoded.
func TestDbTableGetDefault_Stored(t *testing.T) {
	tbl := NewDbTableType(0)
	tbl.Types = [][]ScriptVarType{{ScriptVarTypeInt, ScriptVarTypeString}}
	tbl.DefaultInts = [][]int32{{42, 0}}
	tbl.DefaultStrs = [][]string{{"", "hi"}}

	ints, strs, types := tbl.GetDefault(0)
	if len(ints) != 2 || ints[0] != 42 {
		t.Errorf("ints: got %v, want [42 0]", ints)
	}
	if len(strs) != 2 || strs[1] != "hi" {
		t.Errorf("strs: got %v, want [\"\" hi]", strs)
	}
	if len(types) != 2 {
		t.Errorf("types: got %v", types)
	}
}

// TestDbTableGetDefault_Synthesized verifies GetDefault synthesises
// per-type defaults when nothing is stored: STRING → "", BOOLEAN → 0,
// else → -1.
func TestDbTableGetDefault_Synthesized(t *testing.T) {
	tbl := NewDbTableType(0)
	tbl.Types = [][]ScriptVarType{{ScriptVarTypeInt, ScriptVarTypeString, ScriptVarTypeBoolean}}
	tbl.DefaultInts = [][]int32{nil}
	tbl.DefaultStrs = [][]string{nil}

	ints, strs, _ := tbl.GetDefault(0)
	if ints[0] != -1 {
		t.Errorf("INT default: got %d, want -1", ints[0])
	}
	if strs[1] != "" {
		t.Errorf("STRING default: got %q, want \"\"", strs[1])
	}
	if ints[2] != 0 {
		t.Errorf("BOOLEAN default: got %d, want 0 (distinct from INT)", ints[2])
	}
}
