# S7d — DB_GETFIELD family + DbTableType / DbRowType cache port

- **Sub-spec**: S7d
- **Date**: 2026-04-24
- **Scope label**: B+ (minimal DB read-side, with LISTALL cursor consumers)
- **Predecessors**: S7c (BUILDAPPEARANCE + checkInvType)
- **TS source root**: `LostCityRS/Engine-TS`

## Motivation

The S7c smoke confirmed `update_all` + `update_bas` complete end-to-end. The next script-VM stall is:

```
script=[proc,combat_get_damagetype] err="no handler for DB_GETFIELD (opcode 7502) at pc=3"
```

`DB_GETFIELD` has no Go implementation and no cache infra to back it. The existing
`handleDbGetFieldCount` at `pkg/script/handlers_db.go:15` is a pure stub that pops both
args and pushes `0`; its own comment labels it "deferred to a DB sub-spec." S7d is that
sub-spec.

## Tech stack

- Go 1.26+
- Existing packages: `pkg/objtype/`, `pkg/script/`, `pkg/io/packet`, `modules/world/`

## Scope (B+)

- Port `DbTableType` + `DbRowType` + their loaders into `pkg/objtype/`.
- Add loader wiring into `modules/world/server.go`.
- Extend `script.Configs` interface with `DbTableType(id)`, `DbRowType(id)`, `DbRowsInTable(tableID)`.
- Extend `ScriptState` with `DbTable *objtype.DbTableType`, `DbRow int`, `DbRowQuery []int`.
- Port seven handlers:
  - `DB_GETFIELD` (7502) — main unblocker
  - `DB_GETFIELDCOUNT` (7503) — rewrite the existing stub to real semantics
  - `DB_GETROWTABLE` (7505)
  - `DB_LISTALL` (7510)
  - `DB_LISTALL_WITH_COUNT` (7504)
  - `DB_FINDNEXT` (7501) — LISTALL cursor consumer
  - `DB_FINDBYINDEX` (7506) — LISTALL random-access consumer
- Add `checkDbTable` + `checkDbRow` validators (S7c `checkInvType` pattern).

## Explicitly out of scope

- `DB_FIND` family (7500, 7507, 7508, 7509) — requires `DbTableIndex` type. Deferred.
- `DbTableIndex` port — only needed by `DB_FIND*`. Deferred.
- Asset module wiring — asset is a pure HTTP file server with no config-cache
  dependency. Verified during exploration.
- ScriptState lifecycle changes — state is constructed fresh per script at
  `runner.go:13`. Zero-value defaults for new fields suffice.

## Architecture

### Files created

- `pkg/objtype/dbtabletype.go` — `DbTableType`, `DbTableTypeConfigs`, `LoadDbTableTypes`,
  `parseDbTableTypes`, `NewDbTableType`, `Decode`, `GetDefault`, unexported
  `decodeDbValues`, `scriptVarTypeDefault`.
- `pkg/objtype/dbtabletype_test.go` — loader + `GetDefault` unit tests.
- `pkg/objtype/dbrowtype.go` — `DbRowType`, `DbRowTypeConfigs` (with pre-computed
  `RowsByTable`), `LoadDbRowTypes`, `parseDbRowTypes`, `NewDbRowType`, `Decode`, `GetValue`.
- `pkg/objtype/dbrowtype_test.go` — loader + `GetValue` unit tests.
- `pkg/script/handlers_db_test.go` — handler + validator unit tests across all seven
  opcodes and both validators.

### Files modified

- `pkg/script/handlers_db.go` — rewrite stub; add 6 new handlers + 2 validators.
- `pkg/script/handlers.go` — register 6 new opcode → handler rows (GETFIELDCOUNT
  re-points at the real handler).
- `pkg/script/configs.go` — add 3 new methods on `Configs` interface.
- `pkg/script/state.go` — add 3 new fields on `ScriptState`.
- `modules/world/server.go` — call `LoadDbTableTypes` + `LoadDbRowTypes` after
  `LoadInvTypes`; store on `*Server`.
- `modules/world/server_configs.go` — add 3 new adapter methods on
  `serverConfigsView`.
- Every `_test.go` file that declares a stub implementing `Configs` — add stub
  methods for the 3 new interface methods. (Plan phase enumerates all sites via
  `grep -rn "Configs\b" pkg/script/ --include="*_test.go"`.)

### Dependency graph

```
objtype.DbTableType  ───┐
                        ├──► world.Server ──► serverConfigsView ──► script.Configs
objtype.DbRowType  ─────┘                                                   │
                                                                            ▼
                                                      handlers_db.go (7 handlers + 2 validators)
                                                                            │
                                                                            ▼
                                                   ScriptState{DbTable, DbRow, DbRowQuery}
```

## Cache-type design (Approach 1: parallel typed arrays)

### `DbTableType` (`pkg/objtype/dbtabletype.go`)

```go
const (
    DbTableFlagIndexed    uint8 = 0x1
    DbTableFlagRequired   uint8 = 0x2
    DbTableFlagList       uint8 = 0x4
    DbTableFlagClientside uint8 = 0x8
)

type DbTableType struct {
    ConfigType
    Types       [][]ScriptVarType  // nil for unset columns
    DefaultInts [][]int32          // DefaultInts[col][typeID + fieldID*len(Types[col])]; nil if no stored default
    DefaultStrs [][]string         // parallel to DefaultInts (same stride, same length)
    ColumnNames []string
    Props       []uint8            // flags per column
}
```

**Decode loop** (mirrors `DbTableType.ts:72`):

- **Code 1** (schema + optional defaults):
  1. `columnCount := dat.G1()`; pre-alloc `Types`, `DefaultInts`, `DefaultStrs` to
     length `columnCount`.
  2. Loop: `setting := dat.G1()`; break on `255`.
  3. `column := setting & 0x7f`; `hasDefault := setting & 0x80 != 0`.
  4. `typeCount := dat.G1()`; read `typeCount` `G1`s into `Types[column]`.
  5. If `hasDefault`: call `decodeDbValues(dat, Types[column])` →
     `DefaultInts[column]`, `DefaultStrs[column]`.
- **Code 250**: `DebugName = dat.GJStrLF()`.
- **Code 251**: `colNameCount := dat.G1()`; read `colNameCount` `GJStrLF`s into
  `ColumnNames`.
- **Code 252**: `propsCount := dat.G1()`; read `propsCount` `G1`s into `Props`.
- Default: `return fmt.Errorf("unrecognized dbtable config code %d", code)`.

### `DbRowType` (`pkg/objtype/dbrowtype.go`)

```go
type DbRowType struct {
    ConfigType
    TableID      int
    Types        [][]ScriptVarType
    IntValues    [][]int32
    StringValues [][]string
}
```

**Decode loop** (mirrors `DbRowType.ts:70`):

- **Code 3**: `numColumns := dat.G1()`; pre-alloc `Types`, `IntValues`, `StringValues`.
  Loop: `columnID := dat.G1()`; break on `255`. Read `typeCount := dat.G1()` and
  `typeCount` types; call `decodeDbValues(dat, Types[columnID])` to populate
  `IntValues[columnID]` + `StringValues[columnID]`.
- **Code 4**: `TableID = int(dat.G2())`.
- **Code 250**: `DebugName = dat.GJStrLF()`.
- Default: `return fmt.Errorf("unrecognized dbrow config code %d", code)`.

### Shared helper (`decodeDbValues`, unexported in `dbtabletype.go`)

```go
func decodeDbValues(dat *packet.Packet, types []ScriptVarType) (ints []int32, strs []string) {
    fieldCount := int(dat.G1())
    n := fieldCount * len(types)
    ints = make([]int32, n)
    strs = make([]string, n)
    for fieldID := 0; fieldID < fieldCount; fieldID++ {
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

### Default-synthesis helper

```go
func scriptVarTypeDefault(t ScriptVarType) (intVal int32, strVal string) {
    switch t {
    case ScriptVarTypeString:  return 0, ""
    case ScriptVarTypeBoolean: return 0, ""
    default:                   return -1, ""
    }
}
```

Unexported in `dbtabletype.go`. Mirrors `ScriptVarType.ts:172` — STRING → `""`,
BOOLEAN → `0`, else → `-1`. Promotion to exported `objtype.ScriptVarTypeDefault`
deferred until a second consumer appears (YAGNI).

### `DbTableType.GetDefault`

```go
// Returns per-tuple parallel arrays of length len(t.Types[column]).
// If a stored default exists (DefaultInts/DefaultStrs non-nil for column),
// returns it verbatim; otherwise synthesizes per-slot defaults via
// scriptVarTypeDefault.
func (t *DbTableType) GetDefault(column int) (ints []int32, strs []string, types []ScriptVarType)
```

### `DbRowType.GetValue`

```go
// GetValue mirrors DbRowType.ts:95. Returns the per-tuple parallel slices for
// the given column and listIndex. On out-of-range listIndex (slice empty),
// falls back to table.GetDefault(column). Caller (handler) passes in the
// *DbTableType it already resolved — avoids static-registry coupling.
func (r *DbRowType) GetValue(column, listIndex int, table *DbTableType) (ints []int32, strs []string, types []ScriptVarType)
```

### `DbRowTypeConfigs.RowsByTable`

Pre-computed at load time in `parseDbRowTypes`:

```go
type DbRowTypeConfigs struct {
    ConfigNames map[string]int
    Configs     []*DbRowType
    RowsByTable map[int][]int  // tableID → row IDs in ascending order
}
```

One pass after parsing: walk `Configs`, for each non-nil row append `row.ID` to
`RowsByTable[row.TableID]`.

## `Configs` interface extension (`pkg/script/configs.go`)

```go
type Configs interface {
    ObjType(id int) *objtype.ObjType
    NpcType(id int) *objtype.NpcType
    LocType(id int) *objtype.LocType
    EnumType(id int) *objtype.EnumType
    StructType(id int) *objtype.StructType
    ParamType(id int) *objtype.ParamType
    InvType(id int) *objtype.InvType
    DbTableType(id int) *objtype.DbTableType     // NEW
    DbRowType(id int) *objtype.DbRowType         // NEW
    DbRowsInTable(tableID int) []int             // NEW
}
```

Naming matches the sibling convention (repeat the type name in the method name).
Returns follow the existing contract: nil (or nil slice) on out-of-range / not-loaded.

## World wiring (`modules/world/`)

### `server.go`

After the existing `LoadInvTypes` call (line 146), add:

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

Store on `*Server`:

```go
dbTableTypes *objtype.DbTableTypeConfigs
dbRowTypes   *objtype.DbRowTypeConfigs
```

### `server_configs.go`

Three new adapter methods on `serverConfigsView`, mirroring the existing
`ObjType`/`InvType`/etc. pattern — nil-guard the server and the configs slice,
range-check the id, return the indexed entry. `DbRowsInTable` delegates to
`dbRowTypes.RowsByTable[tableID]` (nil-safe).

### Data dependency

`data/pack/server/dbtable.dat` and `data/pack/server/dbrow.dat` are both present
in the packed cache tree. No `config.yaml` changes required.

## `ScriptState` surface (`pkg/script/state.go`)

Three new fields, grouped:

```go
// DB cursor state — populated by DB_LISTALL* (and DB_FIND*, deferred to a
// later sub-spec); consumed by DB_FINDNEXT, DB_FINDBYINDEX. DbTable == nil
// means no LISTALL/FIND has selected a table yet; DbRow is the cursor index
// into DbRowQuery (-1 after LISTALL before the first FINDNEXT advance).
DbTable    *objtype.DbTableType
DbRow      int
DbRowQuery []int
```

No change to `Init()` in `runner.go` — zero-value defaults suffice.

## Handlers (`pkg/script/handlers_db.go`)

### Validators

```go
func checkDbTable(s *ScriptState, id int, op string) error {
    if s.Configs == nil || s.Configs.DbTableType(id) == nil {
        return fmt.Errorf("%s: no DbTableType with value (%d) found", op, id)
    }
    return nil
}

func checkDbRow(s *ScriptState, id int, op string) error {
    if s.Configs == nil || s.Configs.DbRowType(id) == nil {
        return fmt.Errorf("%s: no DbRowType with value (%d) found", op, id)
    }
    return nil
}
```

### Handler pseudo-code

**`handleDbGetField`** (opcode 7502) — TS `DbOps.ts:97`:

```
listIndex := s.PopInt()
packed    := s.PopInt()
row       := s.PopInt()

fieldTable  := (packed >> 12) & 0xffff
fieldColumn := (packed >> 4)  & 0x7f
tupleIndex  := (packed & 0xf) - 1

if err := checkDbRow(s, row, "DB_GETFIELD"); err != nil { return err }
if err := checkDbTable(s, fieldTable, "DB_GETFIELD"); err != nil { return err }

rowType   := s.Configs.DbRowType(row)
tableType := s.Configs.DbTableType(fieldTable)
valueTypes := tableType.Types[fieldColumn]

off, length := 0, len(valueTypes)
if tupleIndex >= 0 {
    if tupleIndex >= length {
        return fmt.Errorf("DB_GETFIELD: tuple index out-of-bounds. Requested: %d, Max: %d", tupleIndex, length)
    }
    off = tupleIndex; length = tupleIndex + 1
}

var ints []int32; var strs []string
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
```

Fidelity pins: type-directed push reads **table** types (not row types); cross-table
fallback triggers on `row.TableID != fieldTable` (not on a missing column); `listIndex`
out-of-range is swallowed by `GetValue` → default fallback.

**`handleDbGetFieldCount`** (opcode 7503) — TS `DbOps.ts:135`:

```
tableColumnPacked := s.PopInt()
row               := s.PopInt()
table  := (tableColumnPacked >> 12) & 0xffff
column := (tableColumnPacked >> 4)  & 0x7f

if err := checkDbRow(s, row, "DB_GETFIELDCOUNT"); err != nil { return err }
if err := checkDbTable(s, table, "DB_GETFIELDCOUNT"); err != nil { return err }

rowType   := s.Configs.DbRowType(row)
tableType := s.Configs.DbTableType(table)

if rowType.TableID != table {
    s.PushInt(0); return nil
}
s.PushInt(len(rowType.IntValues[column]) / len(tableType.Types[column]))
return nil
```

The existing file-level stub comment and stub body in `handlers_db.go:3–20` are
removed.

**`handleDbGetRowTable`** (opcode 7505) — TS `DbOps.ts:175`:

```
row := s.PopInt()
if err := checkDbRow(s, row, "DB_GETROWTABLE"); err != nil { return err }
s.PushInt(s.Configs.DbRowType(row).TableID)
return nil
```

**`handleDbListAll`** / **`handleDbListAllWithCount`** (opcodes 7510 / 7504) —
TS `DbOps.ts:25`:

```
func dbListAll(s *ScriptState, withCount bool) error {
    table := s.PopInt()
    if err := checkDbTable(s, table, "DB_LISTALL"); err != nil { return err }

    s.DbTable    = s.Configs.DbTableType(table)
    s.DbRow      = -1
    s.DbRowQuery = append(s.DbRowQuery[:0], s.Configs.DbRowsInTable(table)...)

    if withCount {
        s.PushInt(len(s.DbRowQuery))
    }
    return nil
}

func handleDbListAll(s *ScriptState) error          { return dbListAll(s, false) }
func handleDbListAllWithCount(s *ScriptState) error { return dbListAll(s, true) }
```

**`handleDbFindNext`** (opcode 7501) — TS `DbOps.ts:82`:

```
if s.DbTable == nil {
    return fmt.Errorf("DB_FINDNEXT: no table selected")
}
if s.DbRow+1 >= len(s.DbRowQuery) {
    s.PushInt(-1); return nil
}
s.DbRow++
rowID := s.DbRowQuery[s.DbRow]
if err := checkDbRow(s, rowID, "DB_FINDNEXT"); err != nil { return err }
s.PushInt(rowID)
return nil
```

**`handleDbFindByIndex`** (opcode 7506) — TS `DbOps.ts:152`:

```
index := s.PopInt()
if s.DbTable == nil {
    return fmt.Errorf("DB_FINDBYINDEX: no table selected")
}
if index < 0 || index >= len(s.DbRowQuery) {
    s.PushInt(-1); return nil
}
rowID := s.DbRowQuery[index]
if err := checkDbRow(s, rowID, "DB_FINDBYINDEX"); err != nil { return err }
s.PushInt(rowID)
return nil
```

### Dispatch wiring (`pkg/script/handlers.go`)

Six new rows; `OpDbGetFieldCount` stays pointed at `handleDbGetFieldCount` (now
real). The plan phase will confirm the existing registration of `OpDbGetFieldCount`
still resolves correctly after the stub rewrite.

### Error-handling invariants

- Validators return errors; never panic.
- Pop-from-empty is deliberately tolerated (returns 0); mirrors TS
  `toInt32(null) === 0`. Documented at `state.go:149`.
- Nil `s.Configs` is guarded via the validators; handler unit tests with bare
  state never panic.
- A missing column in a well-formed row (`rowType.IntValues[column] == nil`) is
  a loader bug, not a handler bug — not explicitly guarded.

## Testing strategy

### Layer 1 — `pkg/objtype/dbtabletype_test.go`, `dbrowtype_test.go`

Synthetic packets built in-test via `packet.Packet`. No reliance on real
`data/pack/server/*.dat` in unit tests.

**`DbTableType.Decode`**:

1. Minimal: one INT column, no defaults.
2. With stored default: INT column, `fieldCount=1`.
3. STRING column with stored default.
4. Mixed tuple `[INT, STRING, BOOLEAN]`, `fieldCount=2`.
5. Sparse columns: `columnCount=4`, only columns 0 and 2 defined.
6. `setting=255` terminates the schema loop.
7. Code 250 → `DebugName`.
8. Code 251 → `ColumnNames`.
9. Code 252 → `Props`; each flag (`INDEXED`, `REQUIRED`, `LIST`, `CLIENTSIDE`)
   round-trips.
10. Unknown code → error.

**`DbTableType.GetDefault`**:

11. Stored default returned verbatim.
12. No stored default, INT column → synthesized `-1`.
13. No stored default, STRING column → synthesized `""`.
14. No stored default, BOOLEAN column → synthesized `0` (distinct from INT).
15. Mixed tuple, no stored default → per-type synthesis.

**`DbRowType.Decode`**:

16. Code 3, minimal.
17. Code 3, multi-tuple column.
18. Code 3, multi-field row.
19. Code 4 → `TableID` (`G2`).
20. Code 250 → `DebugName`.
21. Unknown code → error.
22. Sparse columns.

**`DbRowType.GetValue`**:

23. Valid `listIndex=0`.
24. `listIndex=1` with `fieldCount=3` → correct offset.
25. `listIndex` out-of-range → falls back to `table.GetDefault`.
26. Empty column values → falls back.

**Loader end-to-end**:

27. Two-config fixture → `Configs` slice length; `ConfigNames` map.
28. `RowsByTable` pre-computed; ascending-order invariant pinned.

### Layer 2 — `pkg/script/handlers_db_test.go`

Fake Configs stub (`fakeDbConfigs`) holding `tables map[int]*DbTableType`,
`rows map[int]*DbRowType`, `rowsByT map[int][]int`. Non-DB Configs methods
return nil. One file; covers all seven opcodes and both validators.

Per-handler matrix:

| Handler | Cases |
|---|---|
| `checkDbTable` | valid; id=-1 → error; id large → error; nil Configs → error |
| `checkDbRow` | mirror |
| `handleDbGetField` | INT single-slot no-tuple; STRING single-slot no-tuple; mixed tuple full push; `tupleIndex=N` single slot; `tupleIndex` OOB → error; `row.TableID ≠ fieldTable` → default; listIndex OOB → default via `GetValue`; invalid row → validator error; invalid table → validator error |
| `handleDbGetFieldCount` | `fieldCount=1` → 1; `fieldCount=3` multi-tuple → 3; `row.TableID ≠ table` → 0; invalid row → error; invalid table → error |
| `handleDbGetRowTable` | valid → `TableID`; invalid row → error |
| `handleDbListAll` | populates state; empty table → empty query; invalid table → error |
| `handleDbListAllWithCount` | state populated + count pushed |
| `handleDbFindNext` | after LISTALL advances cursor and pushes; end of query → -1; nil `DbTable` → error; query with invalid row ID → validator error |
| `handleDbFindByIndex` | valid index pushes ID; `<0` → -1; `≥len` → -1; nil `DbTable` → error; invalid row ID → validator error |

**Cursor-reuse cross-handler test**: LISTALL → FINDNEXT → FINDNEXT → FINDBYINDEX(0)
→ FINDNEXT → FINDNEXT past end. Pins that `FINDBYINDEX` does not advance the
`FINDNEXT` cursor.

### Layer 3 — Cross-file audit & wiring

- **Fake-Configs sweep**: grep `Configs\b` under `pkg/script/ --include="*_test.go"`
  to enumerate every file with a local stub; add the three new methods. Plan
  task-list pre-enumerates sites.
- **Regression**: `go test ./...` with `-race` must pass, not just package-scoped
  runs. Per the `verify_implementer_claims` memory, package-scoped green can
  mask cross-package breakage in `modules/world/`.

### Excluded from unit-test scope

- No `DbTableIndex` tests (type doesn't exist).
- No `DB_FIND*` handler tests (handlers don't exist).
- Automated integration smoke is not authored; user-launched Java-client smoke
  closes the sub-spec.

## Smoke-test closure criterion

User-launched Java client against a user-launched server (per
`smoke_test_server_handoff` memory):

1. **Must pass**: login → `update_all` + `update_bas` regression (S7c pins).
2. **Must pass**: any script invoking `combat_get_damagetype` proc executes past
   `pc=3` without `no handler for DB_GETFIELD (opcode 7502)`.
3. **Observation**: next `no handler for …` log line (if any) is recorded as the
   S7e entry point.

## Deviations

| ID | Subject | Rationale | Follow-up |
|---|---|---|---|
| **S7d-D1** | Parallel typed arrays (`IntValues`/`StringValues`, `DefaultInts`/`DefaultStrs`) replace TS `(string\|number)[]` | Matches `InvType.StockObj/StockCount/StockRate` convention in `objtype/`; zero boxing; type-safety. | None — structural only. |
| **S7d-D2** | `DbRowType.GetValue` takes `*DbTableType` param | No static registries in Go; handler already has the pointer. | None. |
| **S7d-D3** | `DB_FINDNEXT` / `DB_FINDBYINDEX` drop TS `check(...).id` round-trip | Post-validation equivalence; tests pin. | None. |
| **S7d-D4** | `DbRowsInTable` pre-computed at load time | TS filters per call; Go pre-computes once in `LoadDbRowTypes`. | None. |

Pre-existing: S7a-D1, S7a-D2, S7b-D1, S7c-D1 carry forward unchanged. No new
interactions expected.

## Deferred follow-ups (post-S7d)

- **DB_FIND family sub-spec**: `DB_FIND` (7508), `DB_FIND_WITH_COUNT` (7500),
  `DB_FIND_REFINE` (7509), `DB_FIND_REFINE_WITH_COUNT` (7507). Requires
  `DbTableIndex` type with tuple-aware `tableColumnPacked` encoding.
- **`ScriptVarType` default helper promotion**: lift `scriptVarTypeDefault` from
  unexported to exported `objtype.ScriptVarTypeDefault` if a second consumer
  appears (YAGNI, `dead_api_polish` pattern).
- **`GetDefault` `types` return**: if no caller consumes the third return value,
  drop it at next touch.

## Estimated scope

- Production LOC: ≈ 500–600.
- Test LOC: ≈ 500–700 (loader fixtures + handler matrix).
- Files touched: 8 modified, 5 created.
