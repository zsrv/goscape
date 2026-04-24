# S7g — DB_FIND family + `DbTableIndex` cache port + `PtrFindDb` retrofit

- **Sub-spec**: S7g
- **Date**: 2026-04-24
- **Scope label**: B+ (minimal DB lookup-side; closes S7d's deferred DB_FIND* cluster and retrofits the `find_db` pointer gate across the DB family)
- **Predecessors**: S7f (NPC_FIND family) — last on `main` as `f5612b0`
- **TS source root**: `LostCityRS/Engine-TS`

## Motivation

The S7f smoke confirmed the NPC_FIND cluster unblocked `[proc,set_hint_newbie_basics_instructor]` past `pc=8`. The tutorial-entry flow has advanced; the next stall is:

```
script=[label,music_playbyregion]
err="no handler for DB_FIND (opcode 7508) at pc=21"
```

This is a music-region-resolution script fired during world-load / login. `DB_FIND` has no Go implementation and no `DbTableIndex` cache infra to back it. S7d deliberately left the DB_FIND* cluster deferred (see S7d `## Deferred follow-ups`) pending the `DbTableIndex` port. S7g delivers that port plus four handlers sharing it, and retrofits the `find_db` pointer gate that S7d punted on.

## Tech stack

- Go 1.26+
- Existing packages: `pkg/objtype/`, `pkg/script/`, `modules/world/`

## Scope (B+)

- Port `DbTableIndex` — a new `pkg/objtype/` cache-side precomputation over `DbTableType` + `DbRowType`.
- Extend `script.Configs` with two delegating methods: `FindDbRowsInt`, `FindDbRowsStr`.
- Wire `BuildDbTableIndex` into `modules/world/server.go` after S7d's loaders.
- Port four handlers — all sharing the same infrastructure:
    - `DB_FIND` (7508) — the confirmed blocker
    - `DB_FIND_WITH_COUNT` (7500)
    - `DB_FIND_REFINE` (7509)
    - `DB_FIND_REFINE_WITH_COUNT` (7507)
- Add `PtrFindDb Pointer = 1 << 8` flag.
- Retrofit S7d's `handleDbListAll`, `handleDbListAllWithCount`, `handleDbFindNext` to honor the gate. Set the flag in new `DB_FIND` / `DB_LISTALL*`; require the flag in `DB_FINDNEXT` / `DB_FIND_REFINE`. Strictly preserve TS asymmetry: `DB_FIND_WITH_COUNT`, `DB_FIND_REFINE_WITH_COUNT`, `DB_FINDBYINDEX` remain un-gated.

## Explicitly out of scope

- `DB_FINDINDEX` (if it exists as an ECS-range opcode) — separate audit, later sub-spec.
- `printWarning`-equivalent on non-indexed lookup — observability-only; no behavior impact.
- Hot-reload support for cache configs — no existing infrastructure; out of S7g scope.

## Architecture

### Files created

- `pkg/objtype/dbtableindex.go` — `DbTableIndex` struct, `BuildDbTableIndex`, `FindInt`, `FindStr`.
- `pkg/objtype/dbtableindex_test.go` — build + find unit tests.

### Files modified

- `pkg/script/handlers_db.go` — 4 new handlers (`handleDbFind`, `handleDbFindWithCount`, `handleDbFindRefine`, `handleDbFindRefineWithCount`) plus shared helpers `dbFind` / `dbFindRefine`; pointer-gate set/require retrofit on S7d's `dbListAll` and `handleDbFindNext`; preamble comment anchoring TS pointer-gate asymmetry quirk.
- `pkg/script/handlers_db_test.go` — new DB_FIND* handler tests; pointer-gate retrofit tests; asymmetry pins.
- `pkg/script/handlers.go` — 4 new dispatch rows; update the `7500/7507-7509 deferred` comment.
- `pkg/script/configs.go` — 2 new methods on `Configs` interface.
- `pkg/script/pointer.go` — `PtrFindDb Pointer = 1 << 8` constant.
- `modules/world/server.go` — call `BuildDbTableIndex` after `LoadDbRowTypes`; store `*objtype.DbTableIndex` on `*Server`.
- `modules/world/server_configs.go` — 2 new adapter methods on `serverConfigsView`.
- Every `_test.go` that declares a local fake-Configs stub — add the 2 new methods. Plan phase enumerates sites via `grep -rn "Configs\b" pkg/script/ --include="*_test.go"`.

### Dependency graph

```
objtype.DbTableType  ───┐
                        ├──► BuildDbTableIndex ──► objtype.DbTableIndex
objtype.DbRowType  ─────┘                                     │
                                                              ▼
                                             world.Server.dbTableIndex ──► serverConfigsView ──► script.Configs
                                                                                                        │
                                                                                                        ▼
                                                                          handlers_db.go (4 new handlers)
                                                                                                        │
                                                                                                        ▼
                                                                 ScriptState{DbTable, DbRow, DbRowQuery} + Pointers|=PtrFindDb
```

## `DbTableIndex` type (`pkg/objtype/dbtableindex.go`)

```go
// DbTableIndex is a build-time precomputed lookup over all INDEXED columns
// in DbTableType: packedKey → query → row IDs. Packed key is
// (tableID<<12) | (column<<4) | typeID, where typeID uses TS-build convention
// (0-based in the low nibble). Handlers pass the 1-based query form
// (matching bytecode's tupleIndex+1 encoding) and DbTableIndex normalizes.
//
// Int-valued and string-valued queries are split into parallel maps to
// match the IntValues/StringValues split in DbRowType (house style;
// deviation S7d-D1 applied consistently).
type DbTableIndex struct {
    intRows map[int]map[int32][]int  // packedKey → (intQuery → rowIDs)
    strRows map[int]map[string][]int // packedKey → (strQuery → rowIDs)
}
```

### `BuildDbTableIndex(tables *DbTableTypeConfigs, rows *DbRowTypeConfigs) *DbTableIndex`

Called once at world bootstrap, after `LoadDbTableTypes` and `LoadDbRowTypes`. Mirrors TS `DbTableIndex.ts:10-73`:

1. Allocate empty `intRows`, `strRows`.
2. For each `tableID, table := range tables.Configs`:
    - Skip nil tables.
    - Scan `table.Props[col]` for any column with `DbTableFlagIndexed` set. If none, `continue`.
    - For each `rowID := range rows.RowsByTable[tableID]`:
        - `row := rows.Configs[rowID]`
        - For each `col := range row.Types` where `row.Types[col] != nil`:
            - If `table.Props[col] & DbTableFlagIndexed == 0`, continue.
            - If `len(row.Types[col]) > 1` — **tuple path**:
                - For each `(fieldID, typeID, t)` in the row's stored values, index each per-type slot separately at packed key `(tableID<<12) | (col<<4) | typeID`.
            - Else — **list/single path**:
                - Packed key: `(tableID<<12) | (col<<4)` (tuple nibble = 0).
                - Index every stored value in the column (LIST columns have multiple values per row; each indexes to the same key).
3. Return `&DbTableIndex{intRows, strRows}`.

### `FindInt(query int32, packed int) []int` / `FindStr(query string, packed int) []int`

```go
func (x *DbTableIndex) FindStr(query string, packed int) []int {
    key := packed
    if packed & 0xf != 0 {
        key = packed - 1
    }
    if bucket, ok := x.strRows[key]; ok {
        return bucket[query]
    }
    return nil
}
// FindInt — symmetric, over intRows.
```

**On miss** (non-indexed column): returns `nil`. No TS-equivalent `printWarning` emitted (see S7g-D3 deviation).

### Invariant pins

- **Non-destructive reads**: returned slice is the map's underlying `[]int`. Handlers must not mutate; they iterate it (for refine).
- **Tuple-nibble normalization**: 1-based query nibble (from bytecode) → 0-based bucket key. Pinned in a test.
- **List column**: all stored values in a LIST column index to the same packed key.
- **Non-indexed column**: `nil` return, no panic.
- **Determinism**: `RowsByTable` ascending-ID order propagates into `Find*` results.

## `Configs` interface extension (`pkg/script/configs.go`)

```go
type Configs interface {
    // ... existing methods (ObjType, NpcType, ..., DbRowsInTable) ...

    // FindDbRowsInt returns row IDs whose indexed column (encoded in packed)
    // has any stored value equal to query. packed uses the 1-based tuple-
    // nibble convention from bytecode; normalization is handled internally.
    // Returns nil if the column is not INDEXED or no row matches.
    FindDbRowsInt(query int32, packed int) []int // NEW

    // FindDbRowsStr — string-valued variant.
    FindDbRowsStr(query string, packed int) []int // NEW
}
```

Two methods (rather than a single `DbTableIndex() *objtype.DbTableIndex`) keep the `script` → `objtype` surface small; prevent the `*DbTableIndex` pointer from leaking into `pkg/script/`; match the `DbRowsInTable` delegation precedent.

## World wiring (`modules/world/`)

### `server.go`

After the existing `LoadDbRowTypes` call (S7d), insert:

```go
dbTableIndex := objtype.BuildDbTableIndex(dbTableTypes, dbRowTypes)
```

No error return — `BuildDbTableIndex` operates on already-loaded in-memory configs. Store on `*Server`:

```go
type Server struct {
    // ... existing fields ...
    dbTableIndex *objtype.DbTableIndex  // NEW
    // ...
}
```

Assign in the constructor alongside `dbTableTypes` / `dbRowTypes`.

### `server_configs.go`

```go
func (v *serverConfigsView) FindDbRowsInt(query int32, packed int) []int {
    if v == nil || v.s == nil || v.s.dbTableIndex == nil {
        return nil
    }
    return v.s.dbTableIndex.FindInt(query, packed)
}

func (v *serverConfigsView) FindDbRowsStr(query string, packed int) []int {
    if v == nil || v.s == nil || v.s.dbTableIndex == nil {
        return nil
    }
    return v.s.dbTableIndex.FindStr(query, packed)
}
```

Nil-guard pattern mirrors existing adapter methods.

### Data dependency

No new `.dat` files. `dbtable.dat` + `dbrow.dat` already loaded in S7d.

## `Pointer` extension (`pkg/script/pointer.go`)

```go
const (
    PtrActivePlayer  Pointer = 1 << 0
    // ... existing through PtrActiveObj2 = 1 << 7 ...
    PtrFindDb        Pointer = 1 << 8  // NEW — DB_FIND* / DB_LISTALL* set; DB_FINDNEXT / DB_FIND_REFINE require.
)
```

No re-numbering of existing flags. Placement after `PtrActiveObj2` keeps the bit layout stable.

## Handlers (`pkg/script/handlers_db.go`)

### Preamble comment (forward-looking anchor)

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

### `dbFind` helper (shared by DB_FIND / DB_FIND_WITH_COUNT)

Pseudo-code mirrors TS `DbOps.ts:10-23`:

```go
func dbFind(s *ScriptState, withCount bool, op string) error {
    isString := s.PopInt() == 2
    var rowIDs []int
    if isString {
        q      := s.PopString()
        packed := s.PopInt()
        tableID := (packed >> 12) & 0xffff
        if err := checkDbTable(s, tableID, op); err != nil { return err }
        s.DbTable = s.Configs.DbTableType(tableID)
        rowIDs = s.Configs.FindDbRowsStr(q, packed)
    } else {
        q      := s.PopInt()
        packed := s.PopInt()
        tableID := (packed >> 12) & 0xffff
        if err := checkDbTable(s, tableID, op); err != nil { return err }
        s.DbTable = s.Configs.DbTableType(tableID)
        rowIDs = s.Configs.FindDbRowsInt(int32(q), packed)
    }
    s.DbRow      = -1
    s.DbRowQuery = append(s.DbRowQuery[:0], rowIDs...)

    if op == "DB_FIND" {
        s.Pointers |= PtrFindDb   // TS: set: ['find_db']
    }
    // DB_FIND_WITH_COUNT intentionally omits set (TS asymmetry — see preamble).

    if withCount {
        s.PushInt(len(s.DbRowQuery))
    }
    return nil
}

func handleDbFind(s *ScriptState) error          { return dbFind(s, false, "DB_FIND") }
func handleDbFindWithCount(s *ScriptState) error { return dbFind(s, true,  "DB_FIND_WITH_COUNT") }
```

**Pop-order fidelity**: top of stack is `isString`, then `query`, then `packed` — matches TS `state.popInt() → popInt()/popString() → popInt()` sequence.

### `dbFindRefine` helper (shared by DB_FIND_REFINE / DB_FIND_REFINE_WITH_COUNT)

Mirrors TS `DbOps.ts:42-63`:

```go
func dbFindRefine(s *ScriptState, withCount bool, op string) error {
    if op == "DB_FIND_REFINE" && s.Pointers&PtrFindDb == 0 {
        return fmt.Errorf("%s: find_db pointer not set", op)
    }
    // DB_FIND_REFINE_WITH_COUNT intentionally omits require (TS asymmetry).

    isString := s.PopInt() == 2
    var found []int
    if isString {
        q      := s.PopString()
        packed := s.PopInt()
        found = s.Configs.FindDbRowsStr(q, packed)
    } else {
        q      := s.PopInt()
        packed := s.PopInt()
        found = s.Configs.FindDbRowsInt(int32(q), packed)
    }

    foundSet := make(map[int]struct{}, len(found))
    for _, id := range found {
        foundSet[id] = struct{}{}
    }

    prev := s.DbRowQuery
    refined := make([]int, 0, len(prev)) // fresh slice — prev may alias DbRowQuery
    for _, id := range prev {
        if _, ok := foundSet[id]; ok {
            refined = append(refined, id)
        }
    }

    s.DbRow      = -1
    s.DbRowQuery = refined

    if withCount {
        s.PushInt(len(refined))
    }
    return nil
}

func handleDbFindRefine(s *ScriptState) error          { return dbFindRefine(s, false, "DB_FIND_REFINE") }
func handleDbFindRefineWithCount(s *ScriptState) error { return dbFindRefine(s, true,  "DB_FIND_REFINE_WITH_COUNT") }
```

**Intersection-order fidelity**: TS outer-loops `prevQuery`, inner `.includes(found)` — output is `prev`-ordered. Go port uses a set for O(n+m) but iterates `prev` (outer) to preserve order.

**Fresh-slice-vs-reuse rationale**: `prev := s.DbRowQuery` captures the current backing-array pointer; `append(s.DbRowQuery[:0], …)` while iterating `prev` would create a subtle aliasing write. Allocating a fresh slice sidesteps it entirely.

### Pointer-gate retrofit on S7d handlers

#### `handleDbFindNext` — require PtrFindDb, remove the `DbTable == nil` check

```go
func handleDbFindNext(s *ScriptState) error {
    if s.Pointers&PtrFindDb == 0 {
        return fmt.Errorf("DB_FINDNEXT: find_db pointer not set")
    }
    if s.DbRow+1 >= len(s.DbRowQuery) {
        s.PushInt(-1); return nil
    }
    s.DbRow++
    rowID := s.DbRowQuery[s.DbRow]
    if err := checkDbRow(s, rowID, "DB_FINDNEXT"); err != nil { return err }
    s.PushInt(rowID)
    return nil
}
```

The old `s.DbTable == nil` check was S7d's proxy for the missing pointer gate. Replaced by the real gate. Test #21 pins the equivalence: PtrFindDb ⇒ DbTable != nil.

#### `dbListAll` — set PtrFindDb on success

```go
func dbListAll(s *ScriptState, withCount bool) error {
    table := s.PopInt()
    if err := checkDbTable(s, table, "DB_LISTALL"); err != nil { return err }

    s.DbTable    = s.Configs.DbTableType(table)
    s.DbRow      = -1
    s.DbRowQuery = append(s.DbRowQuery[:0], s.Configs.DbRowsInTable(table)...)
    s.Pointers  |= PtrFindDb                       // NEW — TS: set: ['find_db']

    if withCount { s.PushInt(len(s.DbRowQuery)) }
    return nil
}
```

Both `handleDbListAll` and `handleDbListAllWithCount` delegate here, so both set the flag.

#### `handleDbFindByIndex` — **no change**

TS `ScriptOpcodePointers.ts` omits DB_FINDBYINDEX. The existing `s.DbTable == nil` check is the correct (TS-faithful) gate for this opcode alone.

### Dispatch wiring (`pkg/script/handlers.go`)

Replace:

```
// DB ops (7501-7506, 7510; 7500/7507-7509 deferred).
```

with:

```
// DB ops (7500-7510).
// Pointer-gate asymmetry across this family — see preamble comment on handlers_db.go.
```

Add four rows alongside the existing seven:

```go
OpDbFindWithCount:        handleDbFindWithCount,        // 7500
OpDbFindRefineWithCount:  handleDbFindRefineWithCount,  // 7507
OpDbFind:                 handleDbFind,                 // 7508
OpDbFindRefine:           handleDbFindRefine,           // 7509
```

### Error-handling invariants

- Validators return errors; never panic.
- Pop-from-empty tolerated (S7d convention; returns 0/"").
- Nil `s.Configs` guarded via existing `checkDbTable`.
- Empty FindDbRows return (non-indexed column or no match) → empty DbRowQuery; DB_FINDNEXT cleanly returns -1.
- Refine-without-prior-find blocked by gate before state mutation.

## Testing strategy

### Layer 1 — `pkg/objtype/dbtableindex_test.go`

Synthetic `DbTableTypeConfigs` + `DbRowTypeConfigs` fixtures (no `.dat` reliance).

**Build tests:**
1. Empty configs → empty index.
2. One INT-indexed single-column table, one row → `FindInt(val, packed)` returns `[rowID]`.
3. One STRING-indexed column → `FindStr` hits; `FindInt` returns nil.
4. LIST column → `FindInt` on any stored value returns the row.
5. Tuple column (INT, STRING) → per-type buckets.
6. Non-INDEXED column → `Find*` returns nil.
7. Multiple rows share a query → result in `RowsByTable` ascending order.
8. Packed key with column=127 (0x7f edge) — preserved.
9. Packed key with tableID=0xffff (0xffff edge) — preserved.

**Find tests:**
10. `packed & 0xf == 0` → lookup uses packed directly.
11. `packed & 0xf == 1` → lookup uses `packed - 1`. **The 1-based-vs-0-based-nibble pin.**
12. Query value absent in bucket → returns nil.

**Determinism pin:**
13. Two `BuildDbTableIndex` calls on identical input → identical `Find*` output.

### Layer 2 — `pkg/script/handlers_db_test.go` (S7d file, augmented)

Fake-Configs stub's `FindDbRowsInt` / `FindDbRowsStr` are backed by a real `DbTableIndex` built from test fixtures (not a second mock — avoids consistency drift).

**DB_FIND tests:**
1. INT query, one match → DbRowQuery, DbTable, DbRow=-1, PtrFindDb all set; no push.
2. STRING query path (isString=2) → same invariants.
3. No matches → empty DbRowQuery, DbTable set, PtrFindDb set.
4. Invalid table → checkDbTable error; no state mutation; PtrFindDb not set.

**DB_FIND_WITH_COUNT tests:**
5. Populates state + pushes count.
6. **Asymmetry pin**: after DB_FIND_WITH_COUNT, `Pointers&PtrFindDb == 0`; follow-up DB_FIND_REFINE errors on the gate; follow-up DB_FINDNEXT errors on the gate.

**DB_FIND_REFINE tests:**
7. Prior DB_FIND → intersecting query → intersection in prev-order.
8. Prior DB_FIND → disjoint query → empty DbRowQuery.
9. Prior DB_LISTALL → DB_FIND_REFINE works (LISTALL sets flag).
10. No prior find → gate error.
11. Order preserved across prev-set and found-set permutations.

**DB_FIND_REFINE_WITH_COUNT tests:**
12. Matches DB_FIND_REFINE + pushes count.
13. **Asymmetry pin**: does NOT require PtrFindDb; called without a prior find, operates on empty prev, pushes 0, does not error.

**Pointer-gate retrofit tests:**
14. `handleDbListAll` on success → `Pointers&PtrFindDb != 0`.
15. `handleDbListAllWithCount` on success → same.
16. `handleDbFindNext` without PtrFindDb → error.
17. `handleDbFindNext` with PtrFindDb + valid DbRowQuery → advances + pushes.
18. Cross-handler chain LISTALL → FINDNEXT → FINDNEXT preserved (flag carried).
19. `handleDbFindByIndex` behavior unchanged; "no table selected" via `DbTable == nil` still fires.

**Invariant pin:**
20. After any successful `dbFind` / `dbListAll` / `dbFindRefine`: `DbTable != nil` whenever `PtrFindDb` is set. Formalizes the removed proxy check in `handleDbFindNext`.

### Layer 3 — Cross-file audit

- Fake-Configs sweep: `grep -rn "Configs\b" pkg/script/ --include="*_test.go"` enumerates every test file with a local stub; each gets the two new methods.
- Regression: `go test -race ./...` top-level; per `verify_implementer_claims.md`, package-scoped green can mask cross-package breakage.

### Excluded from test scope

- Benchmarks / performance — load-time build, per-op call; no pressure.
- Concurrency — index is immutable post-build.
- `printWarning`-equivalent (S7g-D3) — no behavior, no test.

## Smoke-test closure criterion

User-launched Java client against user-launched server (per `smoke_test_server_handoff.md`):

1. **Must pass**: login + update_all + update_bas (S7c), NPC_FIND cluster (S7f), existing DB infra (S7d).
2. **Must pass**: `[label,music_playbyregion]` executes past `pc=21` without `no handler for DB_FIND (opcode 7508)`.
3. **Observation**: next `no handler for …` log line (if any) is the S7h entry point. Also watch whether `combat_get_damagetype` finally reaches DB_GETFIELD cleanly (S7d inferential confirmation).
4. **TS-asymmetry probe**: if any script chains DB_FIND_WITH_COUNT → DB_FIND_REFINE and hits the gate error, the preserved quirk has been exercised — escalation vector to TS upstream.

## Deviations

| ID | Subject | Rationale | Follow-up |
|---|---|---|---|
| **S7g-D1** | `PtrFindDb` retrofit applied across DB family: new S7g handlers + S7d's DB_LISTALL / DB_LISTALL_WITH_COUNT / DB_FINDNEXT | TS-fidelity gap closure — S7d deferred, S7g fulfills. WITH_COUNT variants + DB_FINDBYINDEX preserved un-gated to match TS asymmetry exactly. | None — closes the gap. |
| **S7g-D2** | Split `intRows` / `strRows` maps in `DbTableIndex` replace TS `Map<string\|number, number[]>` | Matches S7d-D1 (IntValues/StringValues split); avoids `any`-key type-promotion bugs in Go. | None — structural only. |
| **S7g-D3** | `DbTableIndex` build/find omits TS `printWarning` on non-indexed lookup | Observability-only; script-author diagnostic, not runtime behavior. | Promote to logger invocation if a second consumer emerges. |

**Forward-looking comment** on `handlers_db.go` anchoring the TS pointer-gate asymmetry — documentation, not a deviation. Reminder for the implementer: this comment is load-bearing; preserve it through future polish passes.

### Pre-existing deviations carried

S7a-D1, S7a-D2, S7b-D1, S7c-D1, S7d-D1, S7d-D2, S7d-D3, S7d-D4, S7e-D1, S7f-D1, S7f-D2, S7f-D3. Active count after S7g: **15** (12 carried + 3 new).

## Deferred follow-ups (post-S7g)

- **DB_FINDINDEX** (if it exists as an ECS-range opcode) — audit the remaining DB opcode space; separate sub-spec.
- **`printWarning`-equivalent logging** — promote S7g-D3 if a second consumer appears.
- **S7d DB_GETFIELD combat-path confirmation** — smoke re-check after S7g lands; still pending as of S7g close.
- **Next smoke re-stall point** — S7h entry.

## Estimated scope

- **Production LOC**: 220-280 (`dbtableindex.go` ~100, `handlers_db.go` additions + retrofit ~100, `pointer.go` + `configs.go` + server wiring ~20).
- **Test LOC**: 450-600 (`dbtableindex_test.go` ~250, `handlers_db_test.go` additions ~300).
- **Files touched**: 7 modified, 2 created (`handlers_db.go`, `handlers_db_test.go`, `handlers.go`, `configs.go`, `pointer.go`, `modules/world/server.go`, `modules/world/server_configs.go` modified; `pkg/objtype/dbtableindex.go` + `_test.go` created). Plus fake-Configs stub sweep across `pkg/script/*_test.go` (~6-10 files, 2-line additions each).
