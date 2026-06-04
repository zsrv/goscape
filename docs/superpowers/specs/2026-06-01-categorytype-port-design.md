# CategoryType subsystem port — design

**Status:** approved 2026-06-01
**Closes (cluster, merged-alias single closure):**
- `gap-world-reload-events-8` — `World.reload` omits the `CategoryType.load` arm
- `cfg-var-9` — no `pkg/objtype/categorytype.go` loader / no Provider accessor
- `h-npc-3` — `checkCategoryType` drops the count-bound rejection (only -1 sentinel survives)

## Background

TS `CategoryType` (`Engine-TS/src/cache/config/CategoryType.ts`) is a "virtual"
`ConfigType` — every entry carries only a debugname. The static class exposes
`load(dir)`, `get(id)`, `count`, `getId(name)`, `getByName(name)`. It is loaded
at `World.ts:216` alongside the rest of the cache types and is consumed for
input validation at:

- `NpcOps.ts:373` (`NPC_FINDCAT`) — `check(npcCategory, CategoryTypeValid)`
- `InvOps.ts:638` (`INV_TOTALCAT`) — `check(category, CategoryTypeValid)`
- `OpHeldUHandler.ts:106,112` — `CategoryType.get(objType.category)` (lookup, not validation)

`CategoryTypeValid` is a `ScriptInputConfigTypeValidator` that passes
`input >= 0 && input < CategoryType.count`.

Goscape currently has **no `CategoryType` subsystem at all**: no loader, no
`*Server` field, no `Configs` accessor, no `Reload` arm. The audit ledger at
`docs/superpowers/audits/2026-05-28-ts-parity-audit-fresh.md:974` lists this
cluster as `STALE-DEFER`, and the 2026-06-01 sweep confirmed it stays
deferred until the loader is ported. Both `NPC_FINDCAT` and `INV_TOTALCAT`
already call a stub `checkCategoryType(v, op)` that rejects only the `-1`
null sentinel — any other negative or out-of-range int admits silently and
flows into the downstream lookup (`FindClosestNpcByCategory` / `invTotalCat`),
which returns the empty result without raising. `OpHeldU` already routes its
`objType.Category` int directly to the trigger lookup
(`handler_opheld.go:381-391`) without a `CategoryType.get` round-trip, so the
loader is not on its critical path.

## Decisions taken during brainstorming

1. **Sibling loader pattern.** Mirror `pkg/objtype/varntype.go` exactly: tiny
   struct embedding `ConfigType`, `LoadXxxTypes`/`parseXxxTypes` pair,
   single-field `Decode(code, dat)` dispatch. The TS on-disk format
   (`g2`/count + per-entry `[code=1, gjstrLF, end=0]` repeat) matches what
   `DecodeType` consumes verbatim.
2. **Fail-soft on missing `category.dat`.** TS's `CategoryType.load` is the
   only cache loader guarded by `fs.existsSync` (silent no-op). Goscape
   already has the fail-soft precedent at `fonttype.Load`
   (`modules/world/server.go:537-545`). On `os.ErrNotExist` we return an
   empty `*CategoryTypeConfigs` and a warn-log. Other I/O / decode errors
   propagate up as `*Server.New` / `Reload` errors, matching siblings.
3. **`Configs.CategoryType(id) *objtype.CategoryType`** — sibling-consistent
   shape returning nil out-of-range. `Count`/`HasID` variants rejected; the
   nil-check at the validator naturally covers the bound check because
   `parseCategoryTypes` populates every slot in `[0, count)`.
4. **`checkCategoryType(s *ScriptState, id int, op string)`** — signature
   change to thread `s` so the validator can consult `Configs`. Matches
   `checkNpcType` / `checkHuntType` / `checkObjType`. Two call sites
   updated (NPC_FINDCAT, INV_TOTALCAT).
5. **Reload position.** Insert between `spotanim` and `enum` in
   `modules/world/reload.go` (TS World.ts:216 reloads CategoryType among the
   other config types at the same step). Add `s.categoryTypes = categoryTypes_`
   to the step-3 atomic swap. Drop `D4-NO-CATEGORYTYPES` from the
   function-level deviation list.
6. **`OpHeldU` is out of scope.** Goscape already dispatches the
   held-on-held trigger via `objType.Category` (the int) directly, skipping
   the `CategoryType.get` round-trip TS does at
   `OpHeldUHandler.ts:106,112`. The behaviour at the wire boundary is
   identical; no wiring change.

## Architecture

```
                 ┌────────────────────────────────────────────┐
                 │  NPC_FINDCAT (handlers_npc.go:810)         │
                 │  INV_TOTALCAT (handlers_inv.go:372)        │
                 └─────────────────┬──────────────────────────┘
                                   │ checkCategoryType(s, id, op)
                                   ▼
                 ┌────────────────────────────────────────────┐
                 │  s.Configs.CategoryType(id)                │
                 │  [pkg/script.Configs interface]            │
                 └─────────────────┬──────────────────────────┘
                                   │
                                   ▼
                 ┌────────────────────────────────────────────┐
                 │  serverConfigsView.CategoryType(id)        │
                 │  [modules/world/server_configs.go]         │
                 └─────────────────┬──────────────────────────┘
                                   │
                                   ▼
                 ┌────────────────────────────────────────────┐
                 │  *Server.categoryTypes.Configs[id]         │
                 └─────────────────┬──────────────────────────┘
                                   ▲
                                   │
                 ┌─────────────────┴──────────────────────────┐
                 │  Bootstrap: LoadCategoryTypes(cachePath)   │
                 │  Reload:    LoadCategoryTypes + swap       │
                 └────────────────────────────────────────────┘
```

## Components

### `pkg/objtype/categorytype.go` (new)

```go
package objtype

import (
    "errors"
    "log/slog"
    "os"
    "path/filepath"

    packet2 "github.com/zsrv/goscape/pkg/io/packet"
)

type CategoryType struct {
    ConfigType
}

func (c *CategoryType) Decode(code uint8, dat *packet2.Packet) error {
    switch code {
    case 1:
        c.DebugName = dat.GJStrLF()
    default:
        // TS CategoryType.decode (CategoryType.ts:62-66) handles only
        // code 1; any other code falls through silently. Mirror sibling
        // varntype.go: warn-log and continue so DecodeType keeps reading.
        slog.Warn("objtype: unrecognized category config code", "code", code)
    }
    return nil
}

func NewCategoryType(id int) *CategoryType {
    return &CategoryType{ConfigType: ConfigType{ID: id}}
}

type CategoryTypeConfigs struct {
    ConfigNames map[string]int
    Configs     []*CategoryType
}

// LoadCategoryTypes mirrors TS CategoryType.load (CategoryType.ts:12-19).
// Missing data/pack/server/category.dat returns an empty registry
// (TS guards with fs.existsSync; goscape sibling precedent: fonttype.Load
// at modules/world/server.go:537-545). Other I/O / decode errors propagate.
func LoadCategoryTypes(dir string) (*CategoryTypeConfigs, error) {
    server, err := packet2.Load(filepath.Join(dir, "server", "category.dat"), false)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            slog.Warn("objtype: category.dat missing; CategoryType registry empty",
                "dir", dir)
            return &CategoryTypeConfigs{
                ConfigNames: map[string]int{},
                Configs:     nil,
            }, nil
        }
        return nil, err
    }
    return parseCategoryTypes(server)
}

func parseCategoryTypes(server *packet2.Packet) (*CategoryTypeConfigs, error) {
    count := int(server.G2())
    configs := make([]*CategoryType, count)
    configNames := make(map[string]int, count)
    for id := range count {
        config := NewCategoryType(id)
        if err := DecodeType(server, config); err != nil {
            return nil, err
        }
        configs[id] = config
        if config.DebugName != "" {
            configNames[config.DebugName] = id
        }
    }
    return &CategoryTypeConfigs{
        ConfigNames: configNames,
        Configs:     configs,
    }, nil
}
```

### `pkg/objtype/categorytype_test.go` (new)

- `TestParseCategoryTypes_RoundTrip`: synthesise a `*Packet` with count=3
  and 3 named entries (incl. one empty-debugname entry to pin the
  conditional `configNames` population), assert `len(Configs)==3`, each
  `ConfigType.ID == idx`, populated `DebugName`, and `ConfigNames` size +
  contents.
- `TestLoadCategoryTypes_MissingFileReturnsEmpty`: temp dir with no
  `server/category.dat`, assert `nil err`, empty registry, no panic.
- `TestLoadCategoryTypes_LoadsRealFixture` (if `data/pack/server/category.dat`
  is reachable from the test): assert count == 232 and a few well-known
  debugnames resolve via `ConfigNames`.

### `pkg/script/configs.go` — interface addition

Insert near the other `ConfigType` accessors (alphabetically grouped with
the per-type lookups, e.g. between `DbRowType` and `DbRowsInTable`, or
after `SpotAnimType` — the existing file groups loosely by domain):

```go
// CategoryType returns the category config for id, or nil when out of
// range or the registry is empty (TS-faithful fail-soft on missing
// data/pack/server/category.dat). Mirrors TS CategoryType.get
// (CategoryType.ts:39-41). Consumed by checkCategoryType for NPC_FINDCAT
// (NpcOps.ts:373) and INV_TOTALCAT (InvOps.ts:638) bound validation.
CategoryType(id int) *objtype.CategoryType
```

### `modules/world/server.go` — field + bootstrap

Add field next to siblings (line ~170):
```go
categoryTypes *objtype.CategoryTypeConfigs
```

Add bootstrap load in `(*Server).New` near the other type loads, in TS
World.ts:216 order (between spotanim/seq and enum if following Reload's
order, or grouped with the trivial virtual types — match the Reload-step
position for consistency):

```go
categoryTypes, err := objtype.LoadCategoryTypes(cfg.CachePath)
if err != nil {
    return nil, fmt.Errorf("load category types: %w", err)
}
s.categoryTypes = categoryTypes
```

### `modules/world/server_configs.go` — adapter impl

```go
func (c serverConfigsView) CategoryType(id int) *objtype.CategoryType {
    if c.s == nil || c.s.categoryTypes == nil {
        return nil
    }
    if id < 0 || id >= len(c.s.categoryTypes.Configs) {
        return nil
    }
    return c.s.categoryTypes.Configs[id]
}
```

### `modules/world/reload.go`

- Remove the `D4-NO-CATEGORYTYPES` bullet from the function-level
  DEVIATIONs comment (lines 110-118).
- Delete the `PORTING-EXCEPTION` block at lines 161-172.
- In its place, between the `spotanim_` load and the `enumTypes_` load,
  insert:

```go
categoryTypes_, err := objtype.LoadCategoryTypes(cachePath)
if err != nil {
    return nil, fmt.Errorf("reload: category types: %w", err)
}
```

- In step 3 (atomic swap), add:

```go
s.categoryTypes = categoryTypes_
```

at the matching position.

### `pkg/script/handlers_npc.go`

Replace lines 154-174 (the EXCEPTION docstring + 4-line stub body) with the
sibling-shaped version:

```go
// checkCategoryType mirrors TS CategoryTypeValid (ScriptValidators.ts:123)
// — ScriptInputConfigTypeValidator(CategoryType.get, 0 <= n < count),
// collapsed into a single Configs.CategoryType(id) nil check per the
// checkNpcType / checkHuntType pattern. Used by NPC_FINDCAT
// (NpcOps.ts:373) and INV_TOTALCAT (InvOps.ts:638).
func checkCategoryType(s *ScriptState, id int, op string) error {
    if s.Configs == nil || s.Configs.CategoryType(id) == nil {
        return fmt.Errorf("%s: no CategoryType with value (%d) found", op, id)
    }
    return nil
}
```

Call-site changes (2 sites):

- `pkg/script/handlers_npc.go:810` — `checkCategoryType(category, "NPC_FINDCAT")` → `checkCategoryType(s, category, "NPC_FINDCAT")`.
- `pkg/script/handlers_inv.go:372` — `checkCategoryType(category, "INV_TOTALCAT")` → `checkCategoryType(s, category, "INV_TOTALCAT")`.

### Mock cascade

Per the resume-doc `NEW-INTERFACE-METHOD-COMPILE-CASCADE LESSON`: every
`Configs` mock in test files needs a `CategoryType(id int) *objtype.CategoryType`
method. Initial scan suggests three sites:

- `pkg/script/...` — any `fakeConfigs` / `mockConfigs` / `fakeDbConfigs`
  that already stubs `NpcType` / `HuntType` / etc.
- `modules/world/...` — equivalent test fixtures, if any.

Default impl: return nil. Fixtures that need positive lookups gain a
`categories map[int]*objtype.CategoryType` field paralleling the existing
`npcs` / `varnTypes` / etc. accessors.

## Data flow

Inbound (per-tick script execution → category-bounded handler):

1. Script bytecode pushes `category` int onto the operand stack (compiled
   from `find_category` lookup or hard-coded ID).
2. Handler dispatch reaches `handleNpcFindCat` or `handleInvTotalCat`.
3. Handler pops `category` (and other args) and calls
   `checkCategoryType(s, category, op)`.
4. `s.Configs.CategoryType(category)` resolves through
   `serverConfigsView` → `s.categoryTypes.Configs[category]`.
5. Non-nil → handler proceeds to `FindClosestNpcByCategory` /
   `invTotalCat`. Nil → handler returns the validator error;
   `ScriptRunner` catches it via the post-`script-core-1` error path and
   logs / aborts.

Boot / reload:

1. `(*Server).New` calls `LoadCategoryTypes(cfg.CachePath)`; result is
   stashed on `s.categoryTypes`.
2. `(*Server).Reload(clearInvs)` loads a new registry into local
   `categoryTypes_`, then swaps `s.categoryTypes = categoryTypes_` in
   step 3.
3. `serverConfigsView` reads the field through pointer indirection on the
   embedded `*Server` — no atomicity concerns because Reload runs
   synchronously on the tick goroutine (DEVIATIONs in `reload.go`
   function doc-comment).

## Error handling

| Scenario | Behaviour |
|---|---|
| `category.dat` missing | Fail-soft: empty registry, warn-log. Subsequent validator calls reject every id (count=0 → all OOB). |
| `category.dat` corrupt (truncated, decoder I/O error) | `*Server.New` / `Reload` returns error; matches every other sibling loader. |
| `Configs == nil` at validator | Error (`"%s: no CategoryType with value (N) found"`). Test-fixture posture; production always wires `Configs`. |
| `categoryTypes == nil` at validator | Error (same message). |
| `id < 0` (incl. `-1` null sentinel) | Error (same message). Subsumes the old `-1`-only rejection. |
| `id >= count` | Error (same message). New TS-fidelity behaviour. |
| `0 <= id < count` | Pass; handler proceeds. |

## Testing strategy (TDD per the established arc pattern)

### `pkg/objtype/categorytype_test.go`
- `TestParseCategoryTypes_RoundTrip`
- `TestLoadCategoryTypes_MissingFileReturnsEmpty`
- (optional, if the real fixture is reachable from the test sandbox)
  `TestLoadCategoryTypes_LoadsRealFixture`

### `pkg/script/handlers_npc_test.go` (extend)
- `TestCheckCategoryType_NilConfigs` — `s.Configs == nil` → error.
- `TestCheckCategoryType_NilRegistry` — Configs returns nil for every id → error.
- `TestCheckCategoryType_OOB` — `id == count`, `id < -1`, `id == -1` → error.
- `TestCheckCategoryType_InRange` — `0 <= id < count` → nil.
- `TestNpcFindCat_OOBCategoryRejects` — RED→GREEN regression bite at the
  handler boundary: pre-fix (toggle-revert) admits and queries
  `FindClosestNpcByCategory` with the OOB int; post-fix rejects with the
  cited "no CategoryType with value (N) found" message.

### `pkg/script/handlers_inv_test.go` (extend)
- `TestInvTotalCat_OOBCategoryRejects` — same pattern, INV_TOTALCAT side.

### `modules/world/server_configs_test.go` (extend or create)
- `TestServerConfigsView_CategoryType_NilServer`
- `TestServerConfigsView_CategoryType_NilRegistry`
- `TestServerConfigsView_CategoryType_OOB`
- `TestServerConfigsView_CategoryType_InRange`

### Toggle-revert proof

For the two RED→GREEN handler tests, the toggle-revert test (`git apply -R`
on the production hunk in `pkg/script/handlers_npc.go`) must reproduce the
RED state with the cited TS-source reference in the assertion message
(`TS NpcOps.ts:373 check(npcCategory, CategoryTypeValid) (h-npc-3)` for
NPC_FINDCAT; `TS InvOps.ts:638 check(category, CategoryTypeValid)
(h-npc-3)` for INV_TOTALCAT).

## Risk surface

1. **Existing tests passing arbitrary category ints.** Any current test
   that exercises `NPC_FINDCAT` or `INV_TOTALCAT` with a category ≥ 232
   (or `< 0` but not `-1`) will now reject. Audit during implementation:
   `grep -rn "NPC_FINDCAT\|INV_TOTALCAT\|FindClosestNpcByCategory\|invTotalCat" --include='*_test.go'`.
   Real categories from the cache file fall in `[0, 232)` so well-formed
   fixtures are unaffected.
2. **Fixtures without `category.dat`.** Fail-soft means count=0 →
   validator rejects every id. Tests that load other types but not
   categories and exercise the validator need either to load categories
   too or to construct a `mockConfigs` that returns non-nil for the test's
   chosen id.
3. **Mock cascade.** Compile errors at every `Configs`-implementing test
   double until they grow a `CategoryType` method. Resume-doc precedent
   from `h-core-3`'s `VarsType` addition (`med-bundle-15` `591039eb`)
   confirms this is a bounded cost (one parallel-shape impl per consumer).

## Build & ship

- **Branch:** `fix/categorytype-port` from current main `86b368b3`.
- **Commit shape:** dedicated code commit (per `script-core-1` precedent
  for architectural ports — *not* bundle-feasible), then docs commit, then
  FF (or rebase-then-FF if main advances).
- **Code commit subject:** `fix(world): port CategoryType subsystem [h-npc-3]`.
- **Docs commit subject:** `docs(porting): close gap-world-reload-events-8 / cfg-var-9 / h-npc-3 cluster`.
- **Closure shape:** 3-row merged-alias single closure (cite `gap-world-reload-events-8 / cfg-var-9 / h-npc-3` in the PORTING-CLOSED.md row, body cites this spec).
- **Docs to update:**
  - `docs/PORTING-CLOSED.md` — promote the existing `✅ EXCEPTION-DOCUMENTED` row (line 74, `fix/med-bundle-18` `207496e9`) to `✅ FIXED` citing the new commit SHA. Body updated to reflect the actual port (loader + Provider + reload arm + tightened validator) rather than the pre-port deferral rationale.
  - `PORTING.md` — extend the "Recent audit history" / Arc 28 line with this closure.
  - `docs/superpowers/audits/2026-05-28-ts-parity-audit-fresh.md:974` — mark the STALE-DEFER cluster as closed by this commit.
- **#274 flip prediction:** no-op — this work touches neither `deploy/bundled/goscape.yaml` nor `pkg/util/build/build.go`. Pre-FF md5sum snapshot both; post-FF empty-delta confirmation.
- **#288/#289 carries:** if main advances during implementation, use
  `git -c commit.gpgsign=false rebase main` (vanilla `git rebase` ignores
  `--no-gpg-sign`). Verify zero file-overlap via
  `git diff --name-only $(git merge-base main HEAD) HEAD` before rebase.

## Out of scope

- **`OpHeldU` `CategoryType.get` round-trip.** Goscape already dispatches
  via `objType.Category` int directly; no behavioural difference at the
  wire boundary.
- **`getByName` / `getId` resolution path.** No goscape consumer uses
  category debugname → id resolution at runtime. `ConfigNames` is built
  for future-proofing parallel to siblings but isn't surfaced through
  `Configs` until a consumer emerges.
- **Strict `VarSharedValid`-style validator.** Goscape's silent-default
  posture on unknown ids in adjacent surfaces (VarpType / VarnType /
  VarsType — `DEVIATION-NAI-121-D3`) stays in place for those. CategoryType
  gets the strict-validator treatment because it has no fallback semantic
  (unlike a typed read where the int-default makes degraded sense).
