# S7g — DB_FIND Family Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the four DB_FIND* opcodes (`DB_FIND_WITH_COUNT`=7500, `DB_FIND_REFINE_WITH_COUNT`=7507, `DB_FIND`=7508, `DB_FIND_REFINE`=7509) that S7d deferred; add the `DbTableIndex` cache-side precomputation over `DbTableType`+`DbRowType`; retrofit the `find_db` pointer gate across the DB family (new `PtrFindDb` flag; set in `DB_FIND` / `DB_LISTALL*`, require in `DB_FINDNEXT` / `DB_FIND_REFINE`). Preserves the TS asymmetry where WITH_COUNT variants and `DB_FINDBYINDEX` are intentionally un-gated. Unblocks `[label,music_playbyregion]` past `pc=21`.

**Architecture:** New type `DbTableIndex` lives in `pkg/objtype/` (alongside `DbTableType`/`DbRowType`); built once at world bootstrap via `BuildDbTableIndex` right after S7d's `LoadDbRowTypes`. Exposed to `pkg/script/` through two new `Configs` methods (`FindDbRowsInt`, `FindDbRowsStr`) to keep the `*DbTableIndex` pointer inside `modules/world/`. Handlers call `dbFind` / `dbFindRefine` shared helpers (mirroring S7d's `dbListAll` pattern). Pointer-gate enforcement is inline-per-handler (matches S7f's `setActiveNpcSlot` style; no central dispatcher). A forward-looking preamble comment on `handlers_db.go` anchors the TS pointer-gate asymmetry quirk.

**Tech Stack:** Go 1.26+. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` for verification. `git commit --no-gpg-sign` for commits. Branch: `main` (project convention — no worktree needed per established S-series workflow). Spec source: `docs/superpowers/specs/2026-04-24-runescript-s7g-db-find-family-design.md` (commit `bb7a885`).

---

## Task 1: `DbTableIndex` type + unit tests (`pkg/objtype/`)

Self-contained cache-layer type. No `pkg/script/` dependencies. Depth-first TDD: build tests → build impl → find tests → find impl.

**Files:**
- Create: `pkg/objtype/dbtableindex.go`
- Create: `pkg/objtype/dbtableindex_test.go`

---

- [ ] **Step 1.1: Create `pkg/objtype/dbtableindex_test.go` with scaffold + the two foundational build/find tests**

```go
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
```

- [ ] **Step 1.2: Run the tests — expect compile failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run TestBuild -v`

Expected: compile error — `BuildDbTableIndex`, `DbTableIndex`, `FindInt`, `FindStr` undefined.

- [ ] **Step 1.3: Create `pkg/objtype/dbtableindex.go` with the minimal production impl to pass Steps 1.1 tests**

```go
package objtype

// DbTableIndex is a build-time precomputed lookup over all INDEXED columns
// in DbTableType: packedKey → query → row IDs. Packed key is
// (tableID<<12) | (column<<4) | typeID, where typeID uses TS-build
// convention (0-based in the low nibble). Handlers pass the 1-based
// query form (matching bytecode's tupleIndex+1 encoding); DbTableIndex
// normalizes on Find by subtracting 1 from a non-zero low nibble.
//
// Int-valued and string-valued queries are split into parallel maps,
// consistent with the IntValues/StringValues split in DbRowType
// (deviation S7d-D1 applied consistently).
//
// Mirrors TS LostCityRS/Engine-TS/src/cache/config/DbTableIndex.ts.
type DbTableIndex struct {
	intRows map[int]map[int32][]int  // packedKey → (intQuery → rowIDs)
	strRows map[int]map[string][]int // packedKey → (strQuery → rowIDs)
}

// BuildDbTableIndex precomputes the lookup index over every INDEXED
// column across all tables. Called once at world bootstrap after
// LoadDbTableTypes / LoadDbRowTypes. Never fails; returns a non-nil
// *DbTableIndex even for empty configs.
func BuildDbTableIndex(tables *DbTableTypeConfigs, rows *DbRowTypeConfigs) *DbTableIndex {
	idx := &DbTableIndex{
		intRows: make(map[int]map[int32][]int),
		strRows: make(map[int]map[string][]int),
	}
	if tables == nil || rows == nil {
		return idx
	}

	for tableID, table := range tables.Configs {
		if table == nil {
			continue
		}
		// Skip tables with no INDEXED columns (matches TS early-return
		// at DbTableIndex.ts:24).
		anyIndexed := false
		for col := range table.Props {
			if table.Props[col]&DbTableFlagIndexed != 0 {
				anyIndexed = true
				break
			}
		}
		if !anyIndexed {
			continue
		}

		for _, rowID := range rows.RowsByTable[tableID] {
			if rowID < 0 || rowID >= len(rows.Configs) {
				continue
			}
			row := rows.Configs[rowID]
			if row == nil {
				continue
			}
			for col, types := range row.Types {
				if types == nil {
					continue
				}
				if col >= len(table.Props) || table.Props[col]&DbTableFlagIndexed == 0 {
					continue
				}
				if len(types) > 1 {
					idx.indexTuple(tableID, col, types, row, rowID)
				} else {
					idx.indexList(tableID, col, types[0], row, rowID)
				}
			}
		}
	}
	return idx
}

// indexTuple handles the multi-type (tuple) column path. packed key
// includes the 0-based typeID in the low nibble. fieldCount is the
// number of stored field-records per column; index lookup uses
// typeID + fieldID*len(types).
func (x *DbTableIndex) indexTuple(tableID, col int, types []ScriptVarType, row *DbRowType, rowID int) {
	fieldCount := len(row.IntValues[col]) / len(types)
	for fieldID := 0; fieldID < fieldCount; fieldID++ {
		for typeID, t := range types {
			packed := (tableID << 12) | (col << 4) | typeID
			valueIdx := typeID + fieldID*len(types)
			if t == ScriptVarTypeString {
				x.addStr(packed, row.StringValues[col][valueIdx], rowID)
			} else {
				x.addInt(packed, row.IntValues[col][valueIdx], rowID)
			}
		}
	}
}

// indexList handles the single-type-per-column path (including LIST
// columns with multiple stored values). packed key has tuple nibble = 0.
// Every stored value indexes to the same bucket.
func (x *DbTableIndex) indexList(tableID, col int, t ScriptVarType, row *DbRowType, rowID int) {
	packed := (tableID << 12) | (col << 4)
	if t == ScriptVarTypeString {
		for _, v := range row.StringValues[col] {
			x.addStr(packed, v, rowID)
		}
	} else {
		for _, v := range row.IntValues[col] {
			x.addInt(packed, v, rowID)
		}
	}
}

func (x *DbTableIndex) addInt(packed int, query int32, rowID int) {
	bucket, ok := x.intRows[packed]
	if !ok {
		bucket = make(map[int32][]int)
		x.intRows[packed] = bucket
	}
	bucket[query] = append(bucket[query], rowID)
}

func (x *DbTableIndex) addStr(packed int, query string, rowID int) {
	bucket, ok := x.strRows[packed]
	if !ok {
		bucket = make(map[string][]int)
		x.strRows[packed] = bucket
	}
	bucket[query] = append(bucket[query], rowID)
}

// FindInt returns row IDs whose indexed INT column contains query. packed
// uses the bytecode 1-based tuple-nibble convention; FindInt normalizes by
// subtracting 1 from a non-zero low nibble. Returns nil when the column
// is not INDEXED or no row matches. Returned slice is the map's
// underlying storage — callers must treat it as read-only.
func (x *DbTableIndex) FindInt(query int32, packed int) []int {
	key := packed
	if packed&0xf != 0 {
		key = packed - 1
	}
	bucket, ok := x.intRows[key]
	if !ok {
		return nil
	}
	return bucket[query]
}

// FindStr — symmetric to FindInt, over string-valued columns.
func (x *DbTableIndex) FindStr(query string, packed int) []int {
	key := packed
	if packed&0xf != 0 {
		key = packed - 1
	}
	bucket, ok := x.strRows[key]
	if !ok {
		return nil
	}
	return bucket[query]
}
```

- [ ] **Step 1.4: Run the tests from 1.1 — expect pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run TestBuild -v`

Expected: `TestBuildEmptyConfigs` and `TestBuildSingleIntColumn` PASS.

- [ ] **Step 1.5: Extend `pkg/objtype/dbtableindex_test.go` with the full test matrix**

Append to `pkg/objtype/dbtableindex_test.go`:

```go
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

	col0Packed := 0                       // (0<<12)|(0<<4)
	col1Packed := 1 << 4                  // (0<<12)|(1<<4)

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
```

- [ ] **Step 1.6: Run the full test suite — expect all pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -v`

Expected: all Task 1 tests PASS. S7d's `dbtabletype_test.go` and `dbrowtype_test.go` also continue to pass.

- [ ] **Step 1.7: Commit**

```bash
git add pkg/objtype/dbtableindex.go pkg/objtype/dbtableindex_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(objtype): S7g Task 1 — DbTableIndex cache-side lookup precomputation

Build-time precomputation of (packedKey → query → rowIDs) over every
INDEXED column across DbTableType / DbRowType. Split int/str maps match
the S7d-D1 IntValues/StringValues convention. FindInt / FindStr normalize
the bytecode 1-based tuple nibble to the 0-based build convention.

Mirrors TS LostCityRS/Engine-TS/src/cache/config/DbTableIndex.ts.
EOF
)"
```

---

## Task 2: `Configs` interface extension + world wiring + fake-stub sweep

Plumbs `FindDbRowsInt` / `FindDbRowsStr` through the `Configs` interface, the `serverConfigsView` adapter, and every test stub. After this task, downstream handler tasks can write `s.Configs.FindDbRowsInt(...)` without scaffolding.

**Files:**
- Modify: `pkg/script/configs.go` (add 2 methods to `Configs` interface)
- Modify: `modules/world/server.go` (add `dbTableIndex` field; call `BuildDbTableIndex`)
- Modify: `modules/world/server_configs.go` (add 2 adapter methods)
- Modify: `pkg/script/handlers_db_test.go` (add 2 methods to `fakeDbConfigs`)
- Modify: `pkg/script/handlers_loc_test.go` (add 2 methods to `fakeConfigs`)
- Modify: `pkg/script/handlers_config_test.go` (add 2 methods to `mockConfigs`)

---

- [ ] **Step 2.1: Verify fake-stub sites match expectations (paranoia check)**

Run: `grep -rln "Configs\b" /home/owner/Code/github.com/zsrv/goscape/pkg/script/ --include="*_test.go"`

Expected output: `handlers_db_test.go`, `handlers_inv_test.go`, `handlers_loc_test.go`, `handlers_player_test.go`, `handlers_config_test.go`, `handlers_npc_test.go`.

Then run: `grep -rln "^type\s\+\(mockConfigs\|fakeConfigs\|fakeDbConfigs\)\b" /home/owner/Code/github.com/zsrv/goscape/pkg/script/ --include="*.go"`

Expected: three canonical definitions — `handlers_config_test.go` (mockConfigs), `handlers_loc_test.go` (fakeConfigs), `handlers_db_test.go` (fakeDbConfigs). If any other `type .*Configs` stub is added later, this grep must include it. **If the second grep shows more than 3 types, STOP — the plan's stub sweep is incomplete and must be updated.**

- [ ] **Step 2.2: Write failing compile test — add `Configs` interface methods first**

Modify `pkg/script/configs.go`. Replace the entire file content:

```go
package script

import "github.com/zsrv/goscape/pkg/objtype"

// Configs is the config-type lookup surface for config-read opcodes
// (OC_*, NC_*, LC_*, ENUM, STRUCT_PARAM, DB_*). Implementations return nil
// when the type isn't loaded or the id is out of range. DbRowsInTable
// returns nil when no rows match or the catalogue is absent. FindDbRows*
// surface DbTableIndex-backed lookups for DB_FIND* handlers.
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

	// FindDbRowsInt returns row IDs whose indexed column (encoded in
	// packed) has any stored INT value equal to query. packed uses the
	// bytecode 1-based tuple-nibble convention; normalization happens
	// inside the *DbTableIndex. Returns nil if the column is not
	// INDEXED or no row matches.
	FindDbRowsInt(query int32, packed int) []int

	// FindDbRowsStr — string-valued variant of FindDbRowsInt.
	FindDbRowsStr(query string, packed int) []int
}
```

- [ ] **Step 2.3: Run `go build ./...` — expect compile errors from all `Configs` implementors**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: compile errors at every site implementing `Configs` (test-time stubs in `handlers_db_test.go`, `handlers_loc_test.go`, `handlers_config_test.go`; production-time `serverConfigsView` in `modules/world/server_configs.go`). The errors enumerate the exact sites to fix.

- [ ] **Step 2.4: Fix `fakeDbConfigs` in `pkg/script/handlers_db_test.go`**

Locate the method block at `handlers_db_test.go:18-29`. After the `DbRowsInTable` method (line 29), add:

```go
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
```

And modify the `fakeDbConfigs` struct at `handlers_db_test.go:12-16` to include the index:

```go
type fakeDbConfigs struct {
	tables  map[int]*objtype.DbTableType
	rows    map[int]*objtype.DbRowType
	rowsByT map[int][]int
	index   *objtype.DbTableIndex // nil-safe; nil means DB_FIND* tests can't run
}
```

- [ ] **Step 2.5: Fix `fakeConfigs` in `pkg/script/handlers_loc_test.go`**

After line 29 (`DbRowsInTable`), add:

```go
func (f *fakeConfigs) FindDbRowsInt(query int32, packed int) []int { return nil }
func (f *fakeConfigs) FindDbRowsStr(query string, packed int) []int { return nil }
```

- [ ] **Step 2.6: Fix `mockConfigs` in `pkg/script/handlers_config_test.go`**

After line 30 (`DbRowsInTable`), add:

```go
func (m *mockConfigs) FindDbRowsInt(query int32, packed int) []int { return nil }
func (m *mockConfigs) FindDbRowsStr(query string, packed int) []int { return nil }
```

- [ ] **Step 2.7: Wire `serverConfigsView` in `modules/world/server_configs.go`**

At the end of `server_configs.go` (after the `DbRowsInTable` adapter at line 104-108), add:

```go
// FindDbRowsInt delegates to the DbTableIndex built at world bootstrap.
// Returns nil if the server or index is uninitialized.
func (c *serverConfigsView) FindDbRowsInt(query int32, packed int) []int {
	if c == nil || c.s == nil || c.s.dbTableIndex == nil {
		return nil
	}
	return c.s.dbTableIndex.FindInt(query, packed)
}

// FindDbRowsStr — string-valued variant of FindDbRowsInt.
func (c *serverConfigsView) FindDbRowsStr(query string, packed int) []int {
	if c == nil || c.s == nil || c.s.dbTableIndex == nil {
		return nil
	}
	return c.s.dbTableIndex.FindStr(query, packed)
}
```

- [ ] **Step 2.8: Add `dbTableIndex` field to `*Server` in `modules/world/server.go`**

Find the existing field block (line 69-70 with `dbTableTypes` + `dbRowTypes`). Add immediately after:

```go
	dbTableIndex *objtype.DbTableIndex
```

Then find the `LoadDbRowTypes` call block (line 159 onward). After line 167 (`s.dbRowTypes = dbRowTypes`), add:

```go
	s.dbTableIndex = objtype.BuildDbTableIndex(dbTableTypes, dbRowTypes)
```

No error return — `BuildDbTableIndex` operates on in-memory configs and cannot fail.

- [ ] **Step 2.9: Run `go build ./...` — expect clean build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: clean build. If any `Configs` implementation site is still missing a method, the compiler flags it here.

- [ ] **Step 2.10: Run the full test suite — expect all pass (no new tests, just wiring)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all existing tests continue to pass. No DB_FIND handler behavior changes yet.

- [ ] **Step 2.11: Commit**

```bash
git add pkg/script/configs.go pkg/script/handlers_db_test.go pkg/script/handlers_loc_test.go pkg/script/handlers_config_test.go modules/world/server.go modules/world/server_configs.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script,world): S7g Task 2 — Configs.FindDbRows* + DbTableIndex wiring

Adds FindDbRowsInt / FindDbRowsStr to the Configs interface, wired
through serverConfigsView to the new DbTableIndex built at world
bootstrap. Test-time Configs stubs (fakeDbConfigs, fakeConfigs,
mockConfigs) satisfy the new methods; fakeDbConfigs adopts an index
field so later DB_FIND* handler tests can seed real lookups.
EOF
)"
```

---

## Task 3: `PtrFindDb` flag + S7d handler retrofit

Adds the new pointer flag and retrofits S7d's `dbListAll` / `handleDbFindNext` to set / require it. **`handleDbFindByIndex` is intentionally not touched** — TS leaves it un-gated (per `ScriptOpcodePointers.ts` omission).

**Files:**
- Modify: `pkg/script/pointer.go` (add `PtrFindDb` constant)
- Modify: `pkg/script/handlers_db.go` (set flag in `dbListAll`; replace `DbTable == nil` check with `PtrFindDb` gate in `handleDbFindNext`)
- Modify: `pkg/script/handlers_db_test.go` (augment S7d tests to assert flag state; add new gate-miss tests)

---

- [ ] **Step 3.1: Add failing tests for the retrofitted S7d handlers**

Append to `pkg/script/handlers_db_test.go`:

```go
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
```

- [ ] **Step 3.2: Run the new tests — expect FAIL**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestListAllSetsFindDbPointer|TestListAllWithCountSetsFindDbPointer|TestFindNextRequiresFindDbPointer|TestFindNextChainsFromListAll" -v`

Expected: the four new tests FAIL. `TestListAllSetsFindDbPointer` / `TestListAllWithCountSetsFindDbPointer` fail because `PtrFindDb` is undefined (compile error). `TestFindNextRequiresFindDbPointer` and `TestFindNextChainsFromListAll` also fail at the `PtrFindDb` reference.

- [ ] **Step 3.3: Add `PtrFindDb` to `pkg/script/pointer.go`**

Modify `pkg/script/pointer.go`. Replace the const block (lines 7-16) with:

```go
const (
	PtrActivePlayer  Pointer = 1 << 0
	PtrActivePlayer2 Pointer = 1 << 1
	PtrActiveNpc     Pointer = 1 << 2
	PtrActiveNpc2    Pointer = 1 << 3
	PtrActiveLoc     Pointer = 1 << 4
	PtrActiveLoc2    Pointer = 1 << 5
	PtrActiveObj     Pointer = 1 << 6
	PtrActiveObj2    Pointer = 1 << 7
	PtrFindDb        Pointer = 1 << 8 // S7g: DB_FIND* / DB_LISTALL* set; DB_FINDNEXT / DB_FIND_REFINE require.
)
```

- [ ] **Step 3.4: Retrofit `dbListAll` in `pkg/script/handlers_db.go`**

Find the `dbListAll` function (around line 129-143). Replace its body with:

```go
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
```

- [ ] **Step 3.5: Retrofit `handleDbFindNext` in `pkg/script/handlers_db.go`**

Find the function (around line 155-170). Replace its body with:

```go
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
```

- [ ] **Step 3.6: Run the four new tests — expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestListAllSetsFindDbPointer|TestListAllWithCountSetsFindDbPointer|TestFindNextRequiresFindDbPointer|TestFindNextChainsFromListAll" -v`

Expected: all four PASS.

- [ ] **Step 3.7: Run S7d's full handler_db test suite — expect continued PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestDb -v`

Expected: ALL existing S7d tests continue to pass. Particular concern: any S7d test that exercised `handleDbFindNext` with a directly-assigned `DbTable` (without going through `dbListAll`) will now fail the new gate. If this happens, fix the test to set `s.Pointers |= PtrFindDb` alongside the direct `DbTable` assignment — that's the new invariant. **If the fix is mechanical (add one line per test), apply it; if it's non-mechanical (test intent was "exercise nil-DbTable error path"), update the test to assert the new error message instead of the old one.**

- [ ] **Step 3.8: Run the full package test suite — expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -v`

Expected: all tests PASS.

- [ ] **Step 3.9: Commit**

```bash
git add pkg/script/pointer.go pkg/script/handlers_db.go pkg/script/handlers_db_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S7g Task 3 — PtrFindDb flag + DB_LISTALL*/DB_FINDNEXT retrofit

Adds PtrFindDb Pointer = 1 << 8 and wires set/require in S7d's DB handlers.
dbListAll sets the flag on success (shared by DB_LISTALL / DB_LISTALL_WITH_COUNT).
handleDbFindNext now requires it, replacing the S7d proxy check on DbTable == nil.
DB_FINDBYINDEX intentionally unchanged — TS ScriptOpcodePointers.ts omits it.
EOF
)"
```

---

## Task 4: `DB_FIND` + `DB_FIND_WITH_COUNT` handlers + preamble comment + dispatch wiring for both

**Files:**
- Modify: `pkg/script/handlers_db.go` (preamble comment + `dbFind` helper + 2 handlers)
- Modify: `pkg/script/handlers.go` (2 new dispatch rows; update deferred-comment)
- Modify: `pkg/script/handlers_db_test.go` (4 new tests)

---

- [ ] **Step 4.1: Write failing tests for `handleDbFind`**

Append to `pkg/script/handlers_db_test.go`:

```go
// buildTestDbIndex constructs a fakeDbConfigs + real *DbTableIndex for
// the DB_FIND* test matrix. Table 1 has two INDEXED columns:
//   - col 0 (INT): row 10 → 100, row 11 → 200, row 12 → 100 (duplicate)
//   - col 1 (STRING): row 10 → "a", row 11 → "b"
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
```

- [ ] **Step 4.2: Run the new tests — expect FAIL (compile error on `handleDbFind`, `handleDbFindWithCount`)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestDbFind" -v`

Expected: compile error — `handleDbFind`, `handleDbFindWithCount` undefined.

- [ ] **Step 4.3: Add preamble comment + `dbFind` helper + 2 handlers to `pkg/script/handlers_db.go`**

At the top of `pkg/script/handlers_db.go`, immediately after the `import` block, insert:

```go
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
```

Then at the bottom of the file (after `handleDbFindByIndex`), add:

```go
// dbFind is the shared implementation of DB_FIND / DB_FIND_WITH_COUNT.
// Pops an isString marker (==2 means string query), a query (int or
// string), and a packed tableColumnPacked; selects the table, resets
// DbRow to -1, populates DbRowQuery via DbTableIndex, and (for DB_FIND)
// sets PtrFindDb. For DB_FIND_WITH_COUNT, also pushes len(DbRowQuery).
// Pointer-set is asymmetric — DB_FIND_WITH_COUNT omits it per TS.
// Mirrors TS DbOps.ts:10-23.
func dbFind(s *ScriptState, withCount bool, op string) error {
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

	if op == "DB_FIND" {
		s.Pointers |= PtrFindDb // TS: set: ['find_db']
	}
	// DB_FIND_WITH_COUNT intentionally omits the set (TS asymmetry — see preamble).

	if withCount {
		s.PushInt(len(s.DbRowQuery))
	}
	return nil
}

// handleDbFind (DB_FIND, opcode 7508).
func handleDbFind(s *ScriptState) error { return dbFind(s, false, "DB_FIND") }

// handleDbFindWithCount (DB_FIND_WITH_COUNT, opcode 7500).
func handleDbFindWithCount(s *ScriptState) error { return dbFind(s, true, "DB_FIND_WITH_COUNT") }
```

- [ ] **Step 4.4: Run the Step 4.1 tests — expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestDbFind" -v`

Expected: all six new tests PASS (`TestDbFindIntHit`, `TestDbFindStringPath`, `TestDbFindMultipleMatches`, `TestDbFindInvalidTable`, `TestDbFindWithCountHappyPath`, `TestDbFindWithCountFollowedByFindNextFails`).

- [ ] **Step 4.5: Wire DB_FIND / DB_FIND_WITH_COUNT into dispatch**

Modify `pkg/script/handlers.go`. Find the DB ops block (around line 112-120). Replace the comment and add two new rows:

```go
	// DB ops (7500-7510).
	// Pointer-gate asymmetry across this family — see preamble comment on handlers_db.go.
	OpDbFindWithCount:    handleDbFindWithCount, // 7500
	OpDbFindNext:         handleDbFindNext,      // 7501
	OpDbGetField:         handleDbGetField,      // 7502
	OpDbGetFieldCount:    handleDbGetFieldCount, // 7503
	OpDbListAllWithCount: handleDbListAllWithCount, // 7504
	OpDbGetRowTable:      handleDbGetRowTable,   // 7505
	OpDbFindByIndex:      handleDbFindByIndex,   // 7506
	OpDbFind:             handleDbFind,          // 7508
	OpDbListAll:          handleDbListAll,       // 7510
```

(Leave the S5a string-ops block and subsequent rows unchanged.)

Note: `OpDbFindRefineWithCount` (7507) and `OpDbFindRefine` (7509) are **intentionally left out of Task 4**; they land in Task 5.

- [ ] **Step 4.6: Run the full package test suite — expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -v`

Expected: all tests PASS. Dispatch-layer sanity: opcode 7500 and 7508 now resolve.

- [ ] **Step 4.7: Commit**

```bash
git add pkg/script/handlers_db.go pkg/script/handlers.go pkg/script/handlers_db_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S7g Task 4 — DB_FIND + DB_FIND_WITH_COUNT handlers

Adds shared dbFind helper, handleDbFind (7508), and handleDbFindWithCount
(7500). Dispatch rows wired. Preamble comment on handlers_db.go anchors
the TS pointer-gate asymmetry (WITH_COUNT variants + DB_FINDBYINDEX
intentionally un-gated). Tests pin the TS-quirk: DB_FIND_WITH_COUNT
does NOT set find_db; follow-up DB_FINDNEXT fails the gate despite valid
cursor state. Mirrors TS DbOps.ts:10-23.
EOF
)"
```

---

## Task 5: `DB_FIND_REFINE` + `DB_FIND_REFINE_WITH_COUNT` handlers + dispatch + asymmetry pins

**Files:**
- Modify: `pkg/script/handlers_db.go` (`dbFindRefine` helper + 2 handlers)
- Modify: `pkg/script/handlers.go` (2 new dispatch rows)
- Modify: `pkg/script/handlers_db_test.go` (7 new tests)

---

- [ ] **Step 5.1: Write failing tests for `handleDbFindRefine` and the asymmetry pins**

Append to `pkg/script/handlers_db_test.go`:

```go
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
```

- [ ] **Step 5.2: Run the new tests — expect FAIL (handlers undefined)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestDbFindRefine" -v`

Expected: compile error — `handleDbFindRefine`, `handleDbFindRefineWithCount` undefined.

- [ ] **Step 5.3: Add `dbFindRefine` helper + 2 handlers to `pkg/script/handlers_db.go`**

At the bottom of `pkg/script/handlers_db.go` (after `handleDbFindWithCount` from Task 4), add:

```go
// dbFindRefine is the shared implementation of DB_FIND_REFINE /
// DB_FIND_REFINE_WITH_COUNT. Requires PtrFindDb (for the plain variant
// only — DB_FIND_REFINE_WITH_COUNT omits the require per TS). Pops the
// same three args as dbFind, looks up the match set, intersects with
// the prev query (DbRowQuery). Intersection preserves prev-order:
// iteration is over prev, membership check against the found set.
// Allocates a fresh slice to avoid an aliasing trap on
// `append(s.DbRowQuery[:0], ...)` while iterating the same backing array.
// Resets DbRow to -1; pushes count if withCount. Mirrors TS DbOps.ts:42-63.
func dbFindRefine(s *ScriptState, withCount bool, op string) error {
	if op == "DB_FIND_REFINE" && s.Pointers&PtrFindDb == 0 {
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
	refined := make([]int, 0, len(prev)) // fresh slice — prev aliases DbRowQuery backing array
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
func handleDbFindRefine(s *ScriptState) error { return dbFindRefine(s, false, "DB_FIND_REFINE") }

// handleDbFindRefineWithCount (DB_FIND_REFINE_WITH_COUNT, opcode 7507).
func handleDbFindRefineWithCount(s *ScriptState) error {
	return dbFindRefine(s, true, "DB_FIND_REFINE_WITH_COUNT")
}
```

- [ ] **Step 5.4: Run the Step 5.1 tests — expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run "TestDbFindRefine" -v`

Expected: all seven new tests PASS.

- [ ] **Step 5.5: Wire DB_FIND_REFINE / DB_FIND_REFINE_WITH_COUNT into dispatch**

Modify `pkg/script/handlers.go`. Find the DB ops block from Task 4. Add two rows (insert in opcode-ascending order):

```go
	// DB ops (7500-7510).
	// Pointer-gate asymmetry across this family — see preamble comment on handlers_db.go.
	OpDbFindWithCount:        handleDbFindWithCount,        // 7500
	OpDbFindNext:             handleDbFindNext,             // 7501
	OpDbGetField:             handleDbGetField,             // 7502
	OpDbGetFieldCount:        handleDbGetFieldCount,        // 7503
	OpDbListAllWithCount:     handleDbListAllWithCount,     // 7504
	OpDbGetRowTable:          handleDbGetRowTable,          // 7505
	OpDbFindByIndex:          handleDbFindByIndex,          // 7506
	OpDbFindRefineWithCount:  handleDbFindRefineWithCount,  // 7507
	OpDbFind:                 handleDbFind,                 // 7508
	OpDbFindRefine:           handleDbFindRefine,           // 7509
	OpDbListAll:              handleDbListAll,              // 7510
```

- [ ] **Step 5.6: Run the full `pkg/script/` test suite — expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -v`

Expected: all tests PASS.

- [ ] **Step 5.7: Commit**

```bash
git add pkg/script/handlers_db.go pkg/script/handlers.go pkg/script/handlers_db_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S7g Task 5 — DB_FIND_REFINE + DB_FIND_REFINE_WITH_COUNT handlers

Adds shared dbFindRefine helper, handleDbFindRefine (7509), and
handleDbFindRefineWithCount (7507). DB_FIND_REFINE requires find_db;
DB_FIND_REFINE_WITH_COUNT omits the require per TS asymmetry (tests pin).
Intersection preserves prev-order via fresh-slice allocation to avoid
aliasing the DbRowQuery backing array. Mirrors TS DbOps.ts:42-63.
EOF
)"
```

---

## Task 6: Full regression sweep + smoke-handoff prep

**Files:**
- No code changes. Verification + handoff only.

---

- [ ] **Step 6.1: Run `go test -race ./...` top-level**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`

Expected: all tests PASS across all packages. Per `verify_implementer_claims.md` memory, package-scoped green can mask cross-package breakage; top-level is authoritative. If `modules/world/` tests fail with nil-deref on `dbTableIndex`, the Task 2 wiring skipped an init path — revisit server bootstrap.

- [ ] **Step 6.2: Confirm the opcode audit grep lines up**

Run: `grep -nE "OpDbFind|OpDbListAll|OpDbGetField|OpDbGetRowTable" /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers.go`

Expected: all 11 DB opcodes (7500-7510) appear exactly once in the dispatch table. If any are missing or duplicated, the dispatch wiring is wrong.

Also run: `grep -nE "deferred" /home/owner/Code/github.com/zsrv/goscape/pkg/script/handlers.go`

Expected: **no matches** referencing DB opcodes. If the old `7500/7507-7509 deferred` comment is still in place, remove it. The new comment should read `DB ops (7500-7510).` + the asymmetry pointer-line.

- [ ] **Step 6.3: Verify fake-Configs stub sweep is complete**

Run: `grep -rLn "FindDbRowsInt\|FindDbRowsStr" /home/owner/Code/github.com/zsrv/goscape/pkg/script/ --include="*_test.go" | xargs -r grep -l "^type\s\+.*Configs\s\+struct\b" 2>/dev/null`

Expected: empty output. If any file prints, it declares a `*Configs` stub but is missing the two new methods. Fix it before closure.

- [ ] **Step 6.4: Prepare smoke handoff message to the user**

Per `smoke_test_server_handoff.md` memory, the user launches the server themselves. Compose a closure message listing:

1. **What to start**: `CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape --config.file config.yaml`
2. **What to watch for in smoke logs**:
    - **Must pass**: login + update_all + update_bas (S7c), NPC_FIND cluster (S7f), DB infra (S7d).
    - **Must pass**: `[label,music_playbyregion]` executes past `pc=21` without `no handler for DB_FIND (opcode 7508)`.
    - **Observation**: next `no handler for …` log line (if any) → S7h entry point.
    - **Observation**: whether `combat_get_damagetype` finally reaches DB_GETFIELD cleanly (S7d inferential confirmation).
    - **TS-asymmetry probe**: if any script chains DB_FIND_WITH_COUNT → DB_FIND_REFINE or calls DB_FINDNEXT after DB_FIND_WITH_COUNT, the preserved quirk fires as a gate error → escalation signal to TS upstream.

- [ ] **Step 6.5: Close commit with memory-trailer**

Per `close_commit_memory_trailer.md` memory, the S-series close commits carry a `Closes memory:` trailer enumerating memory entries that informed the work. Draft:

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(script): S7g closed — DB_FIND family + DbTableIndex + PtrFindDb retrofit

Four handlers (7500, 7507, 7508, 7509), DbTableIndex cache type, and
PtrFindDb gate retrofit landed across five code-bearing tasks.
[label,music_playbyregion] stall at pc=21 unblocked. Three new
deviations S7g-D1/D2/D3; 15 active after this sub-spec. Preamble
comment on handlers_db.go anchors the preserved TS pointer-gate
asymmetry (WITH_COUNT variants + DB_FINDBYINDEX intentionally un-gated).

Closes memory: compressed_cadence, consume_reserved_constant, enumerate_all_sites, plan_helper_coverage, plan_test_coverage_crosscheck, spec_ts_source_read, true_to_ts_gate, verify_implementer_claims.
EOF
)"
```

Only run this step if a close-commit convention calls for it and the plan's prior commits don't already mark closure. If Task 5's commit is sufficient closure, skip 6.5.

---

## Self-review checklist (author's notes — not for runtime)

- **Spec coverage**: 7 spec sections → 6 tasks. Section 1 (scope) + section 2 (DbTableIndex) → Task 1. Section 3 (Configs interface + world wiring) → Task 2. Section 4 (pointer gate + preamble + handlers + retrofit) → Tasks 3-5. Section 5 (testing strategy) → distributed across Tasks 1-5 as per-task test blocks. Section 6 (deviations + closure) → Task 6. No gap.
- **Placeholder scan**: no TBD / TODO / "add error handling" / "similar to task N". Every code block is complete.
- **Type consistency**: `DbTableIndex.FindInt(query int32, packed int)`, `Configs.FindDbRowsInt(query int32, packed int)`, `serverConfigsView.FindDbRowsInt(query int32, packed int)` — consistent across tasks. `dbFind` / `dbFindRefine` take `(s, withCount, op)` — consistent.
- **TS-asymmetry coverage**: Task 4 (DB_FIND_WITH_COUNT no set), Task 5 (DB_FIND_REFINE_WITH_COUNT no require) — explicitly pinned with dedicated tests.
- **Memory pins applied**: `compressed_cadence` (ruled out, standard cadence chosen), `consume_reserved_constant` (S7d's DbTable/DbRow/DbRowQuery consumed correctly), `enumerate_all_sites` (fake-Configs stub sweep), `plan_helper_coverage` (shared `dbFind` / `dbFindRefine` helpers cross-checked against all four consumers), `plan_test_coverage_crosscheck` (each test block pins a spec-listed behavior), `spec_ts_source_read` (TS source read for every handler body), `true_to_ts_gate` (asymmetry preserved + pinned + commented), `verify_implementer_claims` (Task 6 Step 6.1 top-level go test -race).
