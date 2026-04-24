# S7d — DB_GETFIELD family + DbTableType/DbRowType cache port Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the `DbTableType` and `DbRowType` caches into `pkg/objtype/` and implement seven RuneScript DB read-side opcodes (`DB_GETFIELD` 7502, `DB_GETFIELDCOUNT` 7503, `DB_GETROWTABLE` 7505, `DB_LISTALL` 7510, `DB_LISTALL_WITH_COUNT` 7504, `DB_FINDNEXT` 7501, `DB_FINDBYINDEX` 7506) so `[proc,combat_get_damagetype]` no longer aborts with `"no handler for DB_GETFIELD (opcode 7502) at pc=3"`.

**Architecture:** Layered introduction — (1) `DbTableType` in `pkg/objtype/` with decode + `GetDefault` + loader; (2) `DbRowType` in `pkg/objtype/` with decode + `GetValue` + loader (with pre-computed `RowsByTable`); (3) script-package foundation: three new `Configs` interface methods, three new `ScriptState` fields, two validators (`checkDbTable`, `checkDbRow`); (4) world-module wiring: `Server.dbTableTypes` / `dbRowTypes` fields, load calls, three `serverConfigsView` adapter methods; (5) `DB_GETFIELD` + `DB_GETFIELDCOUNT` (rewriting the existing stub); (6) `DB_GETROWTABLE` + `DB_LISTALL` + `DB_LISTALL_WITH_COUNT`; (7) `DB_FINDNEXT` + `DB_FINDBYINDEX` + cursor-reuse cross-handler test + S7d close commit. Column values use Approach 1 (parallel typed arrays `IntValues`/`StringValues`) per spec deviation S7d-D1.

**Tech Stack:** Go 1.26+. No new packages. Touches `pkg/objtype/dbtabletype.go` (new), `pkg/objtype/dbtabletype_test.go` (new), `pkg/objtype/dbrowtype.go` (new), `pkg/objtype/dbrowtype_test.go` (new), `pkg/script/configs.go`, `pkg/script/state.go`, `pkg/script/handlers_db.go`, `pkg/script/handlers_db_test.go` (new), `pkg/script/handlers.go`, `pkg/script/handlers_config_test.go`, `pkg/script/handlers_loc_test.go`, `modules/world/server.go`, `modules/world/server_configs.go`. Spec: `docs/superpowers/specs/2026-04-24-runescript-s7d-db-getfield-design.md`.

---

## Task 1: DbTableType cache + loader

**Files:**
- Create: `pkg/objtype/dbtabletype.go` — `DbTableType`, `DbTableTypeConfigs`, `NewDbTableType`, `Decode`, `GetDefault`, unexported `decodeDbValues` + `scriptVarTypeDefault`, `LoadDbTableTypes`, `parseDbTableTypes`
- Create: `pkg/objtype/dbtabletype_test.go` — loader + decode + `GetDefault` unit tests

Self-contained; introduces one new exported type. No consumers yet — `Configs` interface extension comes in Task 3.

- [ ] **Step 1: Create the DbTableType file with struct + constructor**

Create `pkg/objtype/dbtabletype.go`:

```go
package objtype

import (
	"fmt"
	"path/filepath"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

// Per-column flag bits used in DbTableType.Props (code 252). Mirrors TS
// DbTableType.ts:12-15. Constants are currently consumed only by future
// DB_FIND* handlers (not in S7d scope) — kept exported so the wire-level
// meaning is documented.
const (
	DbTableFlagIndexed    uint8 = 0x1
	DbTableFlagRequired   uint8 = 0x2
	DbTableFlagList       uint8 = 0x4
	DbTableFlagClientside uint8 = 0x8
)

// DbTableType describes a DB table schema parsed from server/dbtable.dat.
// Column values use Approach 1 (parallel typed arrays) per S7d-D1:
//
//   - Types[col][typeID] is the ScriptVarType of the typeID-th slot in the
//     column's tuple. Types[col] == nil for columns not declared in code 1.
//   - DefaultInts[col][typeID + fieldID*len(Types[col])] holds the stored
//     default int value where Types[col][typeID] != STRING; same stride for
//     DefaultStrs where the type == STRING. Both are nil if no default was
//     stored (use GetDefault to synthesize).
type DbTableType struct {
	ConfigType
	Types       [][]ScriptVarType
	DefaultInts [][]int32
	DefaultStrs [][]string
	ColumnNames []string
	Props       []uint8
}

// NewDbTableType returns a zero-valued DbTableType with the given id.
func NewDbTableType(id int) *DbTableType {
	return &DbTableType{
		ConfigType: ConfigType{ID: id},
	}
}
```

- [ ] **Step 2: Add the Decode method**

Append to `pkg/objtype/dbtabletype.go`:

```go
// Decode mirrors TS DbTableType.ts:72.
func (t *DbTableType) Decode(code uint8, dat *packet2.Packet) error {
	switch code {
	case 1:
		columnCount := int(dat.G1())
		t.Types = make([][]ScriptVarType, columnCount)
		t.DefaultInts = make([][]int32, columnCount)
		t.DefaultStrs = make([][]string, columnCount)

		for setting := dat.G1(); setting != 255; setting = dat.G1() {
			column := int(setting & 0x7f)
			hasDefault := setting&0x80 != 0

			typeCount := int(dat.G1())
			columnTypes := make([]ScriptVarType, typeCount)
			for i := range typeCount {
				columnTypes[i] = ScriptVarType(dat.G1())
			}
			t.Types[column] = columnTypes

			if hasDefault {
				ints, strs := decodeDbValues(dat, columnTypes)
				t.DefaultInts[column] = ints
				t.DefaultStrs[column] = strs
			}
		}
	case 250:
		t.DebugName = dat.GJStrLF()
	case 251:
		n := int(dat.G1())
		t.ColumnNames = make([]string, n)
		for i := range n {
			t.ColumnNames[i] = dat.GJStrLF()
		}
	case 252:
		n := int(dat.G1())
		t.Props = make([]uint8, n)
		for i := range n {
			t.Props[i] = dat.G1()
		}
	default:
		return fmt.Errorf("unrecognized dbtable config code %d", code)
	}
	return nil
}
```

- [ ] **Step 3: Add the shared decodeDbValues helper**

Append to `pkg/objtype/dbtabletype.go`. This helper is shared with `DbRowType` (Task 2); it lives in this file because `DbTableType` is the "primary" owner of the wire format.

```go
// decodeDbValues reads a field-count-prefixed tuple-values block from dat
// into parallel int/string slices. The layout is striped: the value at
// index (typeID + fieldID*len(types)) is an int32 read via G4 when
// types[typeID] != STRING, or a string read via GJStrLF when it is.
// Unused slots (opposite type) are zero-initialised. Length of both
// returned slices equals fieldCount * len(types).
func decodeDbValues(dat *packet2.Packet, types []ScriptVarType) (ints []int32, strs []string) {
	fieldCount := int(dat.G1())
	n := fieldCount * len(types)
	ints = make([]int32, n)
	strs = make([]string, n)
	for fieldID := range fieldCount {
		for typeID, t := range types {
			idx := typeID + fieldID*len(types)
			if t == ScriptVarTypeString {
				strs[idx] = dat.GJStrLF()
			} else {
				ints[idx] = int32(dat.G4())
			}
		}
	}
	return ints, strs
}
```

- [ ] **Step 4: Add GetDefault and the scriptVarTypeDefault helper**

Append to `pkg/objtype/dbtabletype.go`:

```go
// GetDefault returns per-tuple parallel arrays of length len(t.Types[column]).
// If a stored default exists, returns the column's stored slices verbatim.
// Otherwise synthesises per-slot defaults via scriptVarTypeDefault. The
// third return (types) echoes t.Types[column] for caller convenience;
// callers that already have the types in scope may ignore it.
func (t *DbTableType) GetDefault(column int) (ints []int32, strs []string, types []ScriptVarType) {
	types = t.Types[column]
	if t.DefaultInts[column] != nil {
		return t.DefaultInts[column], t.DefaultStrs[column], types
	}
	ints = make([]int32, len(types))
	strs = make([]string, len(types))
	for i, vt := range types {
		ints[i], strs[i] = scriptVarTypeDefault(vt)
	}
	return ints, strs, types
}

// scriptVarTypeDefault mirrors TS ScriptVarType.ts:172 — STRING → "",
// BOOLEAN → 0, else → -1. Unexported until a second consumer materialises
// (YAGNI).
func scriptVarTypeDefault(t ScriptVarType) (intVal int32, strVal string) {
	switch t {
	case ScriptVarTypeString:
		return 0, ""
	case ScriptVarTypeBoolean:
		return 0, ""
	default:
		return -1, ""
	}
}
```

- [ ] **Step 5: Add the DbTableTypeConfigs wrapper and loader**

Append to `pkg/objtype/dbtabletype.go`:

```go
// DbTableTypeConfigs is the loaded DbTableType catalogue.
type DbTableTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*DbTableType
}

// LoadDbTableTypes parses server/dbtable.dat from the given cache dir.
func LoadDbTableTypes(dir string) (*DbTableTypeConfigs, error) {
	server, err := packet2.Load(filepath.Join(dir, "server", "dbtable.dat"), false)
	if err != nil {
		return nil, err
	}
	return parseDbTableTypes(server)
}

func parseDbTableTypes(server *packet2.Packet) (*DbTableTypeConfigs, error) {
	count := int(server.G2())

	configs := make([]*DbTableType, count)
	configNames := make(map[string]int, count)

	for id := range count {
		config := NewDbTableType(id)
		if err := DecodeType(server, config); err != nil {
			return nil, err
		}
		configs[id] = config
		if config.DebugName != "" {
			configNames[config.DebugName] = id
		}
	}

	return &DbTableTypeConfigs{
		ConfigNames: configNames,
		Configs:     configs,
	}, nil
}
```

- [ ] **Step 6: Write the failing test file**

Create `pkg/objtype/dbtabletype_test.go`:

```go
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
```

- [ ] **Step 7: Run the tests and verify all pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/objtype/ -run 'DbTable'`
Expected:
```
ok  	github.com/zsrv/goscape/pkg/objtype
```

If any test fails, fix the implementation (not the test) and re-run.

- [ ] **Step 8: Verify the whole objtype package still passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/objtype/`
Expected: `ok	github.com/zsrv/goscape/pkg/objtype`.

- [ ] **Step 9: Commit**

```bash
git add pkg/objtype/dbtabletype.go pkg/objtype/dbtabletype_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): S7d Task 1 — DbTableType cache + loader

Ports TS cache/config/DbTableType.ts. Column values use parallel typed
arrays (DefaultInts/DefaultStrs, both nil when no default is stored)
per spec deviation S7d-D1. GetDefault synthesises -1/0/"" per
ScriptVarType.ts:172 semantics when nothing was stored.

No consumers yet — Task 3 wires into the Configs interface.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: DbRowType cache + loader (with RowsByTable pre-compute)

**Files:**
- Create: `pkg/objtype/dbrowtype.go` — `DbRowType`, `DbRowTypeConfigs` (with `RowsByTable`), `NewDbRowType`, `Decode`, `GetValue`, `LoadDbRowTypes`, `parseDbRowTypes`
- Create: `pkg/objtype/dbrowtype_test.go` — loader + decode + `GetValue` + `RowsByTable` tests

Depends on Task 1 (`decodeDbValues`, `DbTableType.GetDefault` for `GetValue`'s fallback path).

- [ ] **Step 1: Create the DbRowType file**

Create `pkg/objtype/dbrowtype.go`:

```go
package objtype

import (
	"fmt"
	"path/filepath"
	"sort"

	packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

// DbRowType describes a single DB row parsed from server/dbrow.dat.
// Like DbTableType, column values use parallel typed arrays
// (IntValues / StringValues, both allocated to the same flat length per
// column) per S7d-D1. A row may declare a subset of columns; undeclared
// slots remain nil.
type DbRowType struct {
	ConfigType
	TableID      int
	Types        [][]ScriptVarType
	IntValues    [][]int32
	StringValues [][]string
}

// NewDbRowType returns a zero-valued DbRowType with the given id.
func NewDbRowType(id int) *DbRowType {
	return &DbRowType{
		ConfigType: ConfigType{ID: id},
	}
}

// Decode mirrors TS DbRowType.ts:70.
func (r *DbRowType) Decode(code uint8, dat *packet2.Packet) error {
	switch code {
	case 3:
		numColumns := int(dat.G1())
		r.Types = make([][]ScriptVarType, numColumns)
		r.IntValues = make([][]int32, numColumns)
		r.StringValues = make([][]string, numColumns)

		for columnID := dat.G1(); columnID != 255; columnID = dat.G1() {
			typeCount := int(dat.G1())
			columnTypes := make([]ScriptVarType, typeCount)
			for i := range typeCount {
				columnTypes[i] = ScriptVarType(dat.G1())
			}
			r.Types[columnID] = columnTypes

			ints, strs := decodeDbValues(dat, columnTypes)
			r.IntValues[columnID] = ints
			r.StringValues[columnID] = strs
		}
	case 4:
		r.TableID = int(dat.G2())
	case 250:
		r.DebugName = dat.GJStrLF()
	default:
		return fmt.Errorf("unrecognized dbrow config code %d", code)
	}
	return nil
}

// GetValue mirrors TS DbRowType.ts:95. Returns per-tuple parallel slices
// for the given column and listIndex. On out-of-range listIndex (resulting
// in an empty slice), falls back to table.GetDefault(column). The caller
// (handler) passes *DbTableType explicitly since Go has no static registry
// (S7d-D2).
func (r *DbRowType) GetValue(column, listIndex int, table *DbTableType) (ints []int32, strs []string, types []ScriptVarType) {
	types = r.Types[column]
	tupLen := len(types)
	start := listIndex * tupLen
	end := start + tupLen

	if start < 0 || end > len(r.IntValues[column]) {
		return table.GetDefault(column)
	}
	return r.IntValues[column][start:end], r.StringValues[column][start:end], types
}
```

- [ ] **Step 2: Add the DbRowTypeConfigs wrapper, loader, and RowsByTable pre-compute**

Append to `pkg/objtype/dbrowtype.go`:

```go
// DbRowTypeConfigs is the loaded DbRowType catalogue. RowsByTable is
// pre-computed at load time (S7d-D4) so DB_LISTALL doesn't need to
// filter the full config slice per call.
type DbRowTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*DbRowType
	RowsByTable map[int][]int
}

// LoadDbRowTypes parses server/dbrow.dat from the given cache dir.
func LoadDbRowTypes(dir string) (*DbRowTypeConfigs, error) {
	server, err := packet2.Load(filepath.Join(dir, "server", "dbrow.dat"), false)
	if err != nil {
		return nil, err
	}
	return parseDbRowTypes(server)
}

func parseDbRowTypes(server *packet2.Packet) (*DbRowTypeConfigs, error) {
	count := int(server.G2())

	configs := make([]*DbRowType, count)
	configNames := make(map[string]int, count)
	rowsByTable := make(map[int][]int)

	for id := range count {
		config := NewDbRowType(id)
		if err := DecodeType(server, config); err != nil {
			return nil, err
		}
		configs[id] = config
		if config.DebugName != "" {
			configNames[config.DebugName] = id
		}
		rowsByTable[config.TableID] = append(rowsByTable[config.TableID], id)
	}

	// Ensure ascending-order invariant per column within each table (S7d-D4).
	// Append order is already ascending because id ranges 0..count, but
	// sorting is cheap and documents the invariant for any future refactor.
	for tableID := range rowsByTable {
		sort.Ints(rowsByTable[tableID])
	}

	return &DbRowTypeConfigs{
		ConfigNames: configNames,
		Configs:     configs,
		RowsByTable: rowsByTable,
	}, nil
}
```

- [ ] **Step 3: Write the failing test file**

Create `pkg/objtype/dbrowtype_test.go`:

```go
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
```

- [ ] **Step 4: Run the tests and verify all pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/objtype/ -run 'DbRow'`
Expected: `ok	github.com/zsrv/goscape/pkg/objtype`.

- [ ] **Step 5: Verify the whole objtype package still passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/objtype/`
Expected: `ok	github.com/zsrv/goscape/pkg/objtype`.

- [ ] **Step 6: Commit**

```bash
git add pkg/objtype/dbrowtype.go pkg/objtype/dbrowtype_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): S7d Task 2 — DbRowType cache + loader

Ports TS cache/config/DbRowType.ts. Parallel typed arrays
(IntValues/StringValues) per S7d-D1. GetValue falls back to
DbTableType.GetDefault when listIndex is out-of-range (S7d-D2: the
table is passed in rather than looked up via static registry).
DbRowTypeConfigs.RowsByTable is pre-computed at load time per S7d-D4.

No consumers yet — Task 3 wires into Configs.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Script-package foundation (Configs interface + state fields + validators)

**Files:**
- Modify: `pkg/script/configs.go` — add `DbTableType`, `DbRowType`, `DbRowsInTable` methods
- Modify: `pkg/script/state.go` — add `DbTable`, `DbRow`, `DbRowQuery` fields
- Modify: `pkg/script/handlers_config_test.go:11-27` — update `mockConfigs` stubs
- Modify: `pkg/script/handlers_loc_test.go:16-26` — update `fakeConfigs` stubs
- Modify: `pkg/script/handlers_db.go` — add `checkDbTable` + `checkDbRow` validators
- Modify: `pkg/script/handlers_db_test.go` — new file OR append; add validator tests

No handler dispatch yet — just plumbing. World module intentionally fails to build after this task until Task 4.

- [ ] **Step 1: Extend the Configs interface**

Modify `pkg/script/configs.go`. Replace the whole interface body with:

```go
package script

import "github.com/zsrv/goscape/pkg/objtype"

// Configs is the config-type lookup surface for config-read opcodes
// (OC_*, NC_*, LC_*, ENUM, STRUCT_PARAM, DB_*). Implementations return nil
// when the type isn't loaded or the id is out of range. DbRowsInTable
// returns nil when no rows match or the catalogue is absent.
type Configs interface {
	ObjType(id int) *objtype.ObjType
	NpcType(id int) *objtype.NpcType
	LocType(id int) *objtype.LocType
	EnumType(id int) *objtype.EnumType
	StructType(id int) *objtype.StructType
	ParamType(id int) *objtype.ParamType
	InvType(id int) *objtype.InvType
	DbTableType(id int) *objtype.DbTableType
	DbRowType(id int) *objtype.DbRowType
	DbRowsInTable(tableID int) []int
}
```

- [ ] **Step 2: Add the DB cursor fields to ScriptState**

Modify `pkg/script/state.go`. Inside the `ScriptState` struct, immediately after the `ActiveObj ActiveObj` / `OtherActiveNpc ActiveNpc` block (the "active target" section around line 127), add:

```go

	// DB cursor state — populated by DB_LISTALL* (and DB_FIND*, deferred to a
	// later sub-spec); consumed by DB_FINDNEXT, DB_FINDBYINDEX. DbTable == nil
	// means no LISTALL/FIND has selected a table yet; DbRow is the cursor index
	// into DbRowQuery (-1 after LISTALL before the first FINDNEXT advance).
	DbTable    *objtype.DbTableType
	DbRow      int
	DbRowQuery []int
```

Verify the file now imports `github.com/zsrv/goscape/pkg/objtype` (it already does in sibling files via `Configs`; the `script` package uses objtype in several places). If the state.go import block is missing it, add it to the top-of-file imports.

- [ ] **Step 3: Update mockConfigs in handlers_config_test.go**

Modify `pkg/script/handlers_config_test.go`. Find the `mockConfigs` struct (line 11) and add these three method implementations at the end of the method block (after the existing `InvType` method at line 27):

```go
func (m *mockConfigs) DbTableType(id int) *objtype.DbTableType { return nil }
func (m *mockConfigs) DbRowType(id int) *objtype.DbRowType     { return nil }
func (m *mockConfigs) DbRowsInTable(tableID int) []int         { return nil }
```

These stubs return nil; none of the existing `handlers_config_test.go` tests exercise DB opcodes.

- [ ] **Step 4: Update fakeConfigs in handlers_loc_test.go**

Modify `pkg/script/handlers_loc_test.go`. Find the `fakeConfigs` struct (line 16) and add these three method implementations after the existing `InvType` method at line 26:

```go
func (f *fakeConfigs) DbTableType(id int) *objtype.DbTableType { return nil }
func (f *fakeConfigs) DbRowType(id int) *objtype.DbRowType     { return nil }
func (f *fakeConfigs) DbRowsInTable(tableID int) []int         { return nil }
```

- [ ] **Step 5: Add the checkDbTable and checkDbRow validators**

Modify `pkg/script/handlers_db.go`. At the very top of the file (replace the existing file-level comment on the stub), insert the two validators ahead of `handleDbGetFieldCount`:

```go
package script

import "fmt"

// checkDbTable mirrors TS DbTableTypeValid (ScriptValidators.ts) — a
// ScriptInputConfigTypeValidator over DbTableType. Range + presence checks
// both collapse into "s.Configs.DbTableType(id) != nil" per the Configs
// interface contract. Follows the S7c checkInvType pattern.
func checkDbTable(s *ScriptState, id int, op string) error {
	if s.Configs == nil || s.Configs.DbTableType(id) == nil {
		return fmt.Errorf("%s: no DbTableType with value (%d) found", op, id)
	}
	return nil
}

// checkDbRow mirrors TS DbRowTypeValid.
func checkDbRow(s *ScriptState, id int, op string) error {
	if s.Configs == nil || s.Configs.DbRowType(id) == nil {
		return fmt.Errorf("%s: no DbRowType with value (%d) found", op, id)
	}
	return nil
}
```

**Do NOT touch** the existing `handleDbGetFieldCount` stub yet — Task 5 rewrites it as part of the `DB_GETFIELD`/`DB_GETFIELDCOUNT` pair.

- [ ] **Step 6: Create handlers_db_test.go with validator tests**

Create `pkg/script/handlers_db_test.go`:

```go
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
```

- [ ] **Step 7: Run the script package tests and verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/`
Expected: `ok	github.com/zsrv/goscape/pkg/script`.

If `handlers_config_test.go` or `handlers_loc_test.go` fail to compile because of a missing method, re-check Steps 3–4 for exact method signatures.

- [ ] **Step 8: Verify the world module intentionally fails to build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/`
Expected: FAIL with
```
modules/world/server_configs.go:... *serverConfigsView does not implement script.Configs (missing DbTableType method)
```

This failure is expected and confirms the interface change propagates. Task 4 resolves it. Do NOT work around the failure by stubbing on `serverConfigsView` yet — Task 4 does it properly with backing types on `*Server`.

- [ ] **Step 9: Commit**

```bash
git add pkg/script/configs.go pkg/script/state.go pkg/script/handlers_config_test.go pkg/script/handlers_loc_test.go pkg/script/handlers_db.go pkg/script/handlers_db_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S7d Task 3 — Configs interface + state fields + DB validators

Adds DbTableType/DbRowType/DbRowsInTable to the Configs interface,
DbTable/DbRow/DbRowQuery cursor state to ScriptState, and
checkDbTable/checkDbRow validators following the S7c checkInvType
pattern. mockConfigs and fakeConfigs stubs are updated to satisfy the
new interface (both return nil for DB methods).

modules/world does not build until Task 4 adds the serverConfigsView
adapter methods — intentional staging.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: World wiring (Server fields + load calls + serverConfigsView adapters)

**Files:**
- Modify: `modules/world/server.go` — add `dbTableTypes`/`dbRowTypes` fields on `*Server`; call `LoadDbTableTypes` + `LoadDbRowTypes` near line 146
- Modify: `modules/world/server_configs.go` — add three adapter methods mirroring the existing `InvType` pattern

Brings the repo back to a fully-building state. No handler dispatch yet.

- [ ] **Step 1: Add Server fields**

Modify `modules/world/server.go`. Find the `Server` struct and add two fields grouped with `invTypes`:

```go
	dbTableTypes *objtype.DbTableTypeConfigs
	dbRowTypes   *objtype.DbRowTypeConfigs
```

Exact placement: immediately after the existing `invTypes *objtype.InvTypeConfigs` field on the struct. If the struct uses tagged field groups (blank lines separating config-cache / world-state / etc.), keep the new fields inside the config-cache group.

- [ ] **Step 2: Wire the load calls**

Modify `modules/world/server.go`. Find the existing `invTypes, err := objtype.LoadInvTypes(cfg.CachePath)` call (around line 146). Immediately after the full invTypes error-check block, add:

```go
	dbTableTypes, err := objtype.LoadDbTableTypes(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load dbtable types: %w", err)
	}

	dbRowTypes, err := objtype.LoadDbRowTypes(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load dbrow types: %w", err)
	}
```

Then in the `&Server{...}` struct-literal initialisation further down, add the assignments alongside `invTypes`:

```go
		dbTableTypes: dbTableTypes,
		dbRowTypes:   dbRowTypes,
```

(Grep for `invTypes:` in server.go to find the exact literal-assignment line; add the new lines immediately after it, with matching indentation.)

- [ ] **Step 3: Add the three serverConfigsView adapter methods**

Modify `modules/world/server_configs.go`. Append at the end of the file (after the existing `InvType` method at line 79):

```go

func (c serverConfigsView) DbTableType(id int) *objtype.DbTableType {
	if c.s == nil || c.s.dbTableTypes == nil {
		return nil
	}
	if id < 0 || id >= len(c.s.dbTableTypes.Configs) {
		return nil
	}
	return c.s.dbTableTypes.Configs[id]
}

func (c serverConfigsView) DbRowType(id int) *objtype.DbRowType {
	if c.s == nil || c.s.dbRowTypes == nil {
		return nil
	}
	if id < 0 || id >= len(c.s.dbRowTypes.Configs) {
		return nil
	}
	return c.s.dbRowTypes.Configs[id]
}

// DbRowsInTable returns the pre-computed row IDs for the given table
// (S7d-D4). Returns nil when the catalogue is absent or no rows match.
func (c serverConfigsView) DbRowsInTable(tableID int) []int {
	if c.s == nil || c.s.dbRowTypes == nil {
		return nil
	}
	return c.s.dbRowTypes.RowsByTable[tableID]
}
```

- [ ] **Step 4: Verify the world module builds**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/`
Expected: no output (successful build).

- [ ] **Step 5: Verify the full module tree still tests cleanly**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...`
Expected: all `ok	github.com/...` lines; no FAIL lines.

Per the `verify_implementer_claims` memory, the full-tree run (not just `./pkg/script` / `./modules/world`) is the authoritative gate. If any package fails, investigate before moving to the next task.

- [ ] **Step 6: Commit**

```bash
git add modules/world/server.go modules/world/server_configs.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): S7d Task 4 — DbTableType/DbRowType load calls + Configs adapters

Wires dbtable.dat and dbrow.dat into Server via LoadDbTableTypes /
LoadDbRowTypes, stores them on *Server, and exposes the new Configs
methods (DbTableType / DbRowType / DbRowsInTable) through the existing
serverConfigsView adapter. No handler dispatch yet — Task 5 onward.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: DB_GETFIELD + DB_GETFIELDCOUNT (close stub)

**Files:**
- Modify: `pkg/script/handlers_db.go` — replace the stub `handleDbGetFieldCount`; add `handleDbGetField`
- Modify: `pkg/script/handlers.go` — register `OpDbGetField` (opcode 7502)
- Modify: `pkg/script/handlers_db_test.go` — append handler tests

This pair is the main stall-buster for `combat_get_damagetype`. `DB_GETFIELDCOUNT` is reachable via the already-wired `OpDbGetFieldCount` dispatch entry — no new `handlers.go` row for it.

- [ ] **Step 1: Replace the handleDbGetFieldCount stub with the real implementation**

Modify `pkg/script/handlers_db.go`. **Delete** the existing file-level comment block (lines 3-14) and the stub body (lines 15-20 in the pre-S7d file, but lines will have shifted after Task 3 added validators at the top). Identify the current `handleDbGetFieldCount` function; replace its whole body with:

```go
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
```

- [ ] **Step 2: Add handleDbGetField**

Append to `pkg/script/handlers_db.go` (after `handleDbGetFieldCount`):

```go
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
```

Add `"github.com/zsrv/goscape/pkg/objtype"` to the file's import block if not already present.

- [ ] **Step 3: Register OpDbGetField in handlers.go**

Modify `pkg/script/handlers.go`. Find the dispatch table entry for `OpDbGetFieldCount: handleDbGetFieldCount` (grep: `OpDbGetFieldCount`). Add **immediately above** it (to keep opcode numeric order):

```go
	OpDbGetField:      handleDbGetField,
```

Verify the registration format matches the existing pattern (field alignment, trailing comma).

- [ ] **Step 4: Append handler tests for DB_GETFIELD**

Append to `pkg/script/handlers_db_test.go`:

```go
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
	s.PushInt(0)                     // row
	s.PushInt(pack(7, 0, -1))        // packed: table 7, column 0, no tuple
	s.PushInt(0)                     // listIndex

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
	cfg.rows[0].IntValues[0] = []int32{1, 0, 2, 0, 3, 0}     // 6 entries, fieldCount=3
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
```

- [ ] **Step 5: Run handler tests and verify all pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/ -run 'DbGetField'`
Expected: `ok	github.com/zsrv/goscape/pkg/script`.

- [ ] **Step 6: Run full-tree regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...`
Expected: all ok.

- [ ] **Step 7: Commit**

```bash
git add pkg/script/handlers_db.go pkg/script/handlers.go pkg/script/handlers_db_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S7d Task 5 — DB_GETFIELD (7502) + DB_GETFIELDCOUNT (7503)

Ports TS DbOps.ts:97 (DB_GETFIELD) and replaces the handleDbGetFieldCount
stub with real semantics mirroring DbOps.ts:135. Unblocks the
combat_get_damagetype proc stall at pc=3.

Handler tests cover INT/STRING/mixed-tuple pushes, tuple-index selection,
tuple-index out-of-bounds error, listIndex-OOB default fallback,
cross-table fallback, and both validator paths.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: DB_GETROWTABLE + DB_LISTALL + DB_LISTALL_WITH_COUNT

**Files:**
- Modify: `pkg/script/handlers_db.go` — append `handleDbGetRowTable`, `dbListAll`, `handleDbListAll`, `handleDbListAllWithCount`
- Modify: `pkg/script/handlers.go` — register three new opcodes
- Modify: `pkg/script/handlers_db_test.go` — append tests

`dbListAll` is the shared helper mirroring TS `db_listall(state, withCount)` at `DbOps.ts:25`.

- [ ] **Step 1: Append the handlers to handlers_db.go**

Append to `pkg/script/handlers_db.go`:

```go
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
```

- [ ] **Step 2: Register the three opcodes in handlers.go**

Modify `pkg/script/handlers.go`. Find the existing `OpDbGetField: handleDbGetField,` row added in Task 5. Add **after** it (preserving ascending opcode order — GetRowTable is 7505, ListAll is 7510, ListAllWithCount is 7504):

```go
	OpDbListAllWithCount: handleDbListAllWithCount,
	OpDbGetRowTable:      handleDbGetRowTable,
	OpDbListAll:          handleDbListAll,
```

Alphabetical ordering of the dispatch table would put these in a different order; follow the existing convention (grep for how `OpDbGetFieldCount` is grouped in the table — if the existing entries are listed by opcode number, match that; if by name, match that). If in doubt, group all `OpDb*` entries together in the same order as the const block in `opcode.go:472-482`.

- [ ] **Step 3: Append handler tests**

Append to `pkg/script/handlers_db_test.go`:

```go
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
```

- [ ] **Step 4: Run handler tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/ -run 'DbGetRowTable|DbListAll'`
Expected: all ok.

- [ ] **Step 5: Run full-tree regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...`
Expected: all ok.

- [ ] **Step 6: Commit**

```bash
git add pkg/script/handlers_db.go pkg/script/handlers.go pkg/script/handlers_db_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S7d Task 6 — DB_GETROWTABLE + DB_LISTALL family

Ports TS DbOps.ts:175 (DB_GETROWTABLE), DbOps.ts:74 (DB_LISTALL), and
DbOps.ts:78 (DB_LISTALL_WITH_COUNT). Shared dbListAll(state, withCount)
helper mirrors the TS shape. Populates ScriptState.DbTable/DbRow/DbRowQuery
for consumption by DB_FINDNEXT and DB_FINDBYINDEX (Task 7).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: DB_FINDNEXT + DB_FINDBYINDEX + cursor-reuse test + S7d close

**Files:**
- Modify: `pkg/script/handlers_db.go` — append `handleDbFindNext`, `handleDbFindByIndex`
- Modify: `pkg/script/handlers.go` — register two new opcodes
- Modify: `pkg/script/handlers_db_test.go` — append handler tests + cross-handler cursor test

Closes out the LISTALL consumer pair and finalises S7d.

- [ ] **Step 1: Append the two cursor handlers**

Append to `pkg/script/handlers_db.go`:

```go
// handleDbFindNext (DB_FINDNEXT, opcode 7501) advances the DB cursor to
// the next row in DbRowQuery and pushes its id. Pushes -1 when the cursor
// is past the end. Errors when no table has been selected by a prior
// DB_LISTALL* (or, later, DB_FIND*). Mirrors TS DbOps.ts:82.
func handleDbFindNext(s *ScriptState) error {
	if s.DbTable == nil {
		return fmt.Errorf("DB_FINDNEXT: no table selected")
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
	index := s.PopInt()
	if s.DbTable == nil {
		return fmt.Errorf("DB_FINDBYINDEX: no table selected")
	}
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
```

- [ ] **Step 2: Register the two opcodes in handlers.go**

Modify `pkg/script/handlers.go`. Add entries for `OpDbFindNext` (7501) and `OpDbFindByIndex` (7506) to the dispatch table, keeping the existing grouping convention established in Tasks 5–6:

```go
	OpDbFindNext:    handleDbFindNext,
	OpDbFindByIndex: handleDbFindByIndex,
```

- [ ] **Step 3: Append handler tests**

Append to `pkg/script/handlers_db_test.go`:

```go
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
```

- [ ] **Step 4: Run handler tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./pkg/script/ -run 'DbFindNext|DbFindByIndex|CursorReuse'`
Expected: all ok.

- [ ] **Step 5: Run full-tree regression with race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./...`
Expected: all ok. Per the spec's Layer 3 requirement and the `verify_implementer_claims` memory, this is the authoritative close gate.

- [ ] **Step 6: Verify every spec test case is covered**

Walk the spec's test matrix (`docs/superpowers/specs/2026-04-24-runescript-s7d-db-getfield-design.md` §"Testing strategy") and confirm a corresponding test exists for each row. Grep the test files for expected test names:

```bash
grep -h "^func Test" pkg/objtype/dbtabletype_test.go pkg/objtype/dbrowtype_test.go pkg/script/handlers_db_test.go | sort
```

Expected functions (at minimum):
- `TestParseDbTableTypes`, `TestDbTableMixedTuple`, `TestDbTableSparseColumns`, `TestDbTableUnknownCode`, `TestDbTableGetDefault_Stored`, `TestDbTableGetDefault_Synthesized`
- `TestParseDbRowTypes`, `TestDbRowMultiTuple`, `TestDbRowUnknownCode`, `TestDbRowGetValue_InRange`, `TestDbRowGetValue_OutOfRange_FallsBack`
- `TestCheckDbTable`, `TestCheckDbRow`
- `TestHandleDbGetField_Int`, `TestHandleDbGetField_String`, `TestHandleDbGetField_CrossTableFallsBackToDefault`, `TestHandleDbGetField_ListIndexOutOfRangeFallsBack`, `TestHandleDbGetField_TupleIndex_SingleSlot`, `TestHandleDbGetField_TupleIndex_OutOfBounds`, `TestHandleDbGetField_InvalidRow`, `TestHandleDbGetField_InvalidTable`, `TestHandleDbGetField_MixedTupleFullPush`
- `TestHandleDbGetFieldCount_Basic`, `TestHandleDbGetFieldCount_MultiTuple_Field3`, `TestHandleDbGetFieldCount_CrossTableZero`, `TestHandleDbGetFieldCount_InvalidRow`, `TestHandleDbGetFieldCount_InvalidTable`
- `TestHandleDbGetRowTable_Basic`, `TestHandleDbGetRowTable_InvalidRow`
- `TestHandleDbListAll_PopulatesState`, `TestHandleDbListAll_EmptyTable`, `TestHandleDbListAll_InvalidTable`, `TestHandleDbListAllWithCount_PushesCount`
- `TestHandleDbFindNext_AfterListAll_Advances`, `TestHandleDbFindNext_AtEnd_PushesNegativeOne`, `TestHandleDbFindNext_NoTableSelected`, `TestHandleDbFindNext_InvalidRowID`
- `TestHandleDbFindByIndex_Basic`, `TestHandleDbFindByIndex_Negative`, `TestHandleDbFindByIndex_BeyondEnd`, `TestHandleDbFindByIndex_NoTableSelected`, `TestHandleDbFindByIndex_InvalidRowID`
- `TestCursorReuse_FindByIndexDoesNotMoveFindNextCursor`

If any are missing, add them before the close commit. If any test is present but doesn't assert what its name implies, fix it.

- [ ] **Step 7: Commit Task 7 body**

```bash
git add pkg/script/handlers_db.go pkg/script/handlers.go pkg/script/handlers_db_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S7d Task 7 — DB_FINDNEXT (7501) + DB_FINDBYINDEX (7506)

Ports TS DbOps.ts:82 (DB_FINDNEXT) and DbOps.ts:152 (DB_FINDBYINDEX).
Both consume the DbRowQuery populated by DB_LISTALL (Task 6) and validate
the resolved row id via checkDbRow. FINDBYINDEX is random-access and
does not move the FINDNEXT cursor — pinned by the cross-handler
TestCursorReuse test.

Closes the LISTALL consumer pair as scoped in S7d-B+.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 8: S7d close commit**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(script): S7d closed — DB_GETFIELD family + cache port

All seven DB read-side opcodes (7501, 7502, 7503, 7504, 7505, 7506, 7510)
land with full handler+loader test coverage. DbTableType and DbRowType
cache types are ported with parallel typed arrays (S7d-D1), the
Configs interface exposes DbTableType/DbRowType/DbRowsInTable, and
ScriptState carries DbTable/DbRow/DbRowQuery cursor state.

User-launched smoke (Java client vs. user-launched server) pending to
confirm combat_get_damagetype proc advances past pc=3 without the
"no handler for DB_GETFIELD" stall; next stall (if any) entry-points
the S7e scoping conversation.

DB_FIND family + DbTableIndex deferred.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-review (run before handing this plan to subagent-driven-development)

**Spec coverage:** Every requirement in the spec's scope list maps to a task:

- `DbTableType` + loader → Task 1
- `DbRowType` + loader (with `RowsByTable`) → Task 2
- `Configs` interface extension (3 methods) → Task 3, Step 1
- `ScriptState` fields (3 fields) → Task 3, Step 2
- Fake-Configs sweep (2 files) → Task 3, Steps 3–4
- `checkDbTable` / `checkDbRow` validators → Task 3, Steps 5–6
- `modules/world/server.go` load calls + Server fields → Task 4, Steps 1–2
- `modules/world/server_configs.go` adapters → Task 4, Step 3
- `DB_GETFIELD` handler → Task 5, Step 2
- `DB_GETFIELDCOUNT` rewrite → Task 5, Step 1
- `DB_GETROWTABLE` handler → Task 6, Step 1
- `DB_LISTALL` / `DB_LISTALL_WITH_COUNT` → Task 6, Step 1
- `DB_FINDNEXT` / `DB_FINDBYINDEX` → Task 7, Step 1
- Dispatch wiring for all 6 new opcodes → Tasks 5/6/7 (Steps 3/2/2 respectively)
- Cursor-reuse cross-handler test → Task 7, Step 3
- Layer 3 full-tree regression with `-race` → Task 7, Step 5

**Placeholder scan:** no "TBD", "TODO", "implement later", "similar to Task N", or "add appropriate error handling" anywhere in the task bodies. Every code step includes the actual Go code to write.

**Type consistency:** `DbTable` / `DbRow` / `DbRowQuery` field names are used consistently across Tasks 3, 6, 7. `checkDbTable` / `checkDbRow` signatures `(s *ScriptState, id int, op string) error` are consistent across all call sites. `GetDefault` returns `(ints, strs, types)` uniformly; `GetValue` returns the same triple. `DbRowTypeConfigs.RowsByTable` is used with identical type `map[int][]int` in the constructor, interface adapter, and tests.
