# NAI-58 — SpotanimType Config Port + NAI-36-D2 Retirement

## Motivation

`pkg/script/handlers_map.go:212` `checkSpotAnimType` falls back to
range-only validation (`id < 0` rejected) under deviation **NAI-36-D2**
because the `SpotanimType` config registry was unported when SPOTANIM_MAP
landed (NAI-36). TS `ServerOps.ts:88` wraps with
`check(spotanim, SpotAnimTypeValid)` — full presence-against-config-table
validation. NAI-57 just landed the directly analogous `SeqType` port;
SpotanimType is the next link in the IdkType (NAI-46) → SeqType (NAI-57)
→ SpotanimType chain.

**TS reference:** `Engine-TS/src/cache/config/SpotanimType.ts` (105 LOC,
filename `SpotanimType.ts`; struct named `SpotanimType` per file casing
even though the validator is named `SpotAnimTypeValid`); single existing
goscape consumer is `checkSpotAnimType` invoked from `handleSpotAnimMap`
at `pkg/script/handlers_map.go:233`.

## Tech Stack

**Go 1.26+** (per `go_version.md`; modern Go via `use-modern-go` skill).

## Deviations

| Tag | Status | Notes |
|-----|--------|-------|
| **NAI-36-D2** | **Retired** | `checkSpotAnimType` now mirrors TS `SpotAnimTypeValid`: rejects negative ids AND ids absent from the SpotanimType config table |

No new deviations opened. Decode codes 4-8 (`resizeh`, `resizev`,
`orientation`, `ambient`, `contrast`) and `recol_s/d` slots are parsed
and stored faithfully but currently have zero consumers — TS-faithful
struct shape, not a deviation.

**Tally:** 20 → 19.

## Scope

**In scope:**

- `pkg/objtype/spotanimtype.go` — `SpotanimType` struct + `Decode`
  opcode dispatch + `SpotanimTypeConfigs` registry +
  `LoadSpotanimTypes(dir)` loader
- `pkg/objtype/spotanimtype_test.go` — per-opcode decode + parse unit
  tests
- `modules/world/server.go` — add `spotanimTypes` field + load wire-in
- `modules/world/server_configs.go` — add `(c serverConfigsView).SpotAnimType(id)`
  accessor
- `pkg/script/configs.go` — extend `Configs` interface with
  `SpotAnimType(id int) *objtype.SpotanimType`
- `pkg/script/handlers_db_test.go` + `handlers_loc_test.go` +
  `handlers_config_test.go` — add `SpotAnimType` stub on each
  `Configs` test-mock implementor
- `pkg/script/handlers_map.go` — change `checkSpotAnimType` signature
  to `(s *ScriptState, id int, op string)`; add presence check via
  `s.Configs.SpotAnimType(id)`; retire NAI-36-D2 doc-comment
- `pkg/script/handlers_map_test.go` — extend with positive (registered
  id passes) + negative (unregistered id rejects) arms; seed
  `mockConfigs` with a `spotAnimTypes` map; retire NAI-36-D2 doc-comment

**Out of scope (deferred):**

- Handler ports for `SPOTANIM_NPC` (opcode 2547 declared but no handler
  registered) and `MAP_PROJANIM_PL/NPC/MAP` (opcodes that pop spotanim
  ids and call `World.mapProjAnim`). These each require dedicated
  porting work (entity-level method addition, 9-int pop semantics, zone
  routing) — separate sub-spec(s). NAI-58 only ports the registry +
  ties the existing `checkSpotAnimType` consumer.
- `SPOTANIM_PL` (PlayerOps.ts:589-593) is already TS-faithful: TS does
  no `SpotAnimTypeValid` check on the PL variant, only NumberNotNull on
  delay. Goscape's `handleSpotAnimPl` matches this. No tightening
  required.
- Encoding / wire-format use of SpotanimType beyond the validation gate.

---

## Pre-flight (verified at HEAD `dfaee46`)

| Claim | Result |
|---|---|
| `Engine-TS/src/cache/config/SpotanimType.ts` is 105 LOC, decode codes 1-8, 40-49, 50-59, 250 | ✓ |
| `pkg/script/handlers_map.go:212` `checkSpotAnimType` is range-only under NAI-36-D2 doc-comment | ✓ |
| Sole caller of `checkSpotAnimType`: `handleSpotAnimMap` at `handlers_map.go:233` | ✓ — single call site |
| `SPOTANIM_PL` handler at `pkg/script/handlers_player.go:557` does NOT call `checkSpotAnimType` (matches TS PlayerOps.ts:589-593 — no validation) | ✓ |
| `SPOTANIM_NPC` (opcode 2547) declared at `pkg/script/opcode.go:284` but NOT registered in gameHandlers (no `handleSpotAnimNpc` exists) | ✓ — out-of-scope confirmed |
| `MAP_PROJANIM_*` (3 variants) NOT ported as script handlers | ✓ — out-of-scope confirmed |
| Server data files staged: `data/pack/server/spotanim.{dat,idx}` | ✓ |
| `pkg/objtype/idktype.go` template available for ConfigType + dual-source loader pattern | ✓ |
| `pkg/objtype/configtype.go` exposes `ConfigType` base + `DecodeType(buf, decoder)` polymorphic helper | ✓ |
| `Configs` interface implementors at HEAD: `serverConfigsView` (production), `fakeDbConfigs` (handlers_db_test.go:26), `fakeConfigs` (handlers_loc_test.go:27), `mockConfigs` (handlers_config_test.go:29) | ✓ — 1 production + 3 mock |
| `mockConfigs` already has per-type stub-with-map pattern (e.g. `idks map[int]*objtype.IdkType`) | ✓ — `spotAnimTypes` map slot is the natural extension |

---

## Task 1 — `pkg/objtype/spotanimtype.go` + tests

Port `Engine-TS/src/cache/config/SpotanimType.ts` following the
dual-source (server + client jag) pattern from `idktype.go`.

### Struct + constructor

```go
package objtype

import (
    "errors"
    "fmt"
    "os"
    "path/filepath"

    io "github.com/zsrv/goscape/pkg/io/jagfile"
    "github.com/zsrv/goscape/pkg/io/packet"
)

// SpotanimType is a single spotanim.dat config record (graphic / spot animation).
// Mirrors Engine-TS/src/cache/config/SpotanimType.ts.
type SpotanimType struct {
    ConfigType
    Model       int      // -1 not used by TS default; TS default 0
    Anim        int      // -1 default
    HasAlpha    bool
    RecolS      [6]uint16
    RecolD      [6]uint16
    Resizeh     int      // 128 default
    Resizev     int      // 128 default
    Orientation int      // 0 default
    Ambient     int      // 0 default
    Contrast    int      // 0 default
}

// NewSpotanimType returns a SpotanimType with TS-faithful defaults.
// TS defaults: model=0, anim=-1, resizeh=128, resizev=128.
func NewSpotanimType(id int) *SpotanimType {
    return &SpotanimType{
        ConfigType: ConfigType{ID: id},
        Anim:       -1,
        Resizeh:    128,
        Resizev:    128,
    }
}
```

### Decode

```go
// Decode dispatches on the spotanim config opcode, matching TS
// SpotanimType.decode at Engine-TS/src/cache/config/SpotanimType.ts:78-104.
func (t *SpotanimType) Decode(code uint8, dat *packet.Packet) error {
    switch code {
    case 1:
        t.Model = int(dat.G2())
    case 2:
        t.Anim = int(dat.G2())
    case 3:
        t.HasAlpha = true
    case 4:
        t.Resizeh = int(dat.G2())
    case 5:
        t.Resizev = int(dat.G2())
    case 6:
        t.Orientation = int(dat.G2())
    case 7:
        t.Ambient = int(dat.G1())
    case 8:
        t.Contrast = int(dat.G1())
    case 40, 41, 42, 43, 44, 45, 46, 47, 48, 49:
        // TS recol_s is 6-element; codes 46-49 are out-of-range. Guard
        // matches TS Uint16Array silent-discard behavior. Same shape as
        // idktype.go.
        slot := code - 40
        v := dat.G2()
        if slot < 6 {
            t.RecolS[slot] = v
        }
    case 50, 51, 52, 53, 54, 55, 56, 57, 58, 59:
        slot := code - 50
        v := dat.G2()
        if slot < 6 {
            t.RecolD[slot] = v
        }
    case 250:
        t.DebugName = dat.GJStrLF()
    default:
        return fmt.Errorf("unrecognized spotanim config code %d", code)
    }
    return nil
}
```

### Registry + loader

```go
// SpotanimTypeConfigs is the parsed registry of all spotanim records.
type SpotanimTypeConfigs struct {
    ConfigNames map[string]int
    Configs     []*SpotanimType
}

// LoadSpotanimTypes parses server/spotanim.dat + client/config jag →
// spotanim.dat into a SpotanimTypeConfigs registry. Returns an empty
// registry with nil error when server/spotanim.dat is absent
// (silent-on-missing, matching TS SpotanimType.load).
func LoadSpotanimTypes(dir string) (*SpotanimTypeConfigs, error) {
    server, err := packet.Load(filepath.Join(dir, "server", "spotanim.dat"), false)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return &SpotanimTypeConfigs{ConfigNames: map[string]int{}}, nil
        }
        return nil, err
    }

    clientJag, err := io.LoadJagfile(filepath.Join(dir, "client", "config"))
    if err != nil {
        return nil, err
    }

    return parseSpotanimTypes(server, clientJag)
}

func parseSpotanimTypes(server *packet.Packet, clientJag *io.Jagfile) (*SpotanimTypeConfigs, error) {
    count := int(server.G2())
    configs := make([]*SpotanimType, count)
    configNames := make(map[string]int, count)

    client, err := clientJag.Read("spotanim.dat")
    if err != nil {
        return nil, err
    }
    client.Pos = 2 // skip client-side count header (matches idktype.go pattern)

    for id := range count {
        config := NewSpotanimType(id)
        if err := DecodeType(server, config); err != nil {
            return nil, err
        }
        if err := DecodeType(client, config); err != nil {
            return nil, err
        }
        configs[id] = config
        if config.DebugName != "" {
            configNames[config.DebugName] = id
        }
    }

    return &SpotanimTypeConfigs{ConfigNames: configNames, Configs: configs}, nil
}
```

### Test list

`pkg/objtype/spotanimtype_test.go`:

- `TestNewSpotanimTypeDefaults` — `NewSpotanimType(7)` → ID=7, Anim=-1,
  Resizeh=128, Resizev=128, all other ints/bool zero, RecolS/RecolD
  zero-arrays
- `TestSpotanimTypeDecode_Model` — code 1 + g2 → Model
- `TestSpotanimTypeDecode_Anim` — code 2 + g2 → Anim
- `TestSpotanimTypeDecode_HasAlpha` — code 3 → HasAlpha=true
- `TestSpotanimTypeDecode_Resizeh` — code 4 + g2 → Resizeh
- `TestSpotanimTypeDecode_Resizev` — code 5 + g2 → Resizev
- `TestSpotanimTypeDecode_Orientation` — code 6 + g2 → Orientation
- `TestSpotanimTypeDecode_Ambient` — code 7 + g1 → Ambient
- `TestSpotanimTypeDecode_Contrast` — code 8 + g1 → Contrast
- `TestSpotanimTypeDecode_RecolSInRange` — codes 40, 45 + g2 each → RecolS[0], RecolS[5]
- `TestSpotanimTypeDecode_RecolSOutOfRangeDiscarded` — code 47 + g2 →
  silent-discard, packet cursor advances (idktype-pattern)
- `TestSpotanimTypeDecode_RecolDInRange` — codes 50, 55 + g2 each → RecolD[0], RecolD[5]
- `TestSpotanimTypeDecode_RecolDOutOfRangeDiscarded` — code 57 + g2 →
  silent-discard
- `TestSpotanimTypeDecode_DebugName` — code 250 + null-terminated string → DebugName
- `TestSpotanimTypeDecode_UnknownCode` — code 99 → error returned
- `TestParseSpotanimTypes_EmptyServerCount` — server G2=0, dummy client
  jag → empty Configs, nil error
- `TestLoadSpotanimTypes_MissingFileSilent` — tmp dir without
  `server/spotanim.dat` → empty registry, nil error

---

## Task 2 — `modules/world/server.go` wire-in

Add field alongside `idkTypes`/`seqTypes`/`seqFrames`:

```go
spotanimTypes *objtype.SpotanimTypeConfigs
```

In the bootstrap path that already loads `seqTypes` (currently around
`server.go:247`), append:

```go
spotanimTypes, err := objtype.LoadSpotanimTypes(cfg.CachePath)
if err != nil {
    return nil, fmt.Errorf("load spotanim types: %w", err)
}
s.spotanimTypes = spotanimTypes
```

No `Player.spotanimTypes` field needed — unlike SeqType (which gates a
hot-path entity method), SpotanimType lookups go through the
`script.Configs` interface only.

---

## Task 3 — `Configs.SpotAnimType` accessor

### 3a. `pkg/script/configs.go`

Extend the `Configs` interface (alphabetised insertion alongside
`IdkType`, `InvType`, etc.):

```go
SpotAnimType(id int) *objtype.SpotanimType
```

The interface method name uses **`SpotAnimType`** (TS validator name
`SpotAnimTypeValid`) for consistency with the other typed accessors;
the underlying struct retains its file casing `SpotanimType`. This
mirrors how TS ServerOps imports `SpotanimType` but calls the validator
`SpotAnimTypeValid`.

### 3b. `modules/world/server_configs.go`

Add the production accessor mirroring the existing `IdkType` accessor
shape at lines 81-89:

```go
func (c serverConfigsView) SpotAnimType(id int) *objtype.SpotanimType {
    if c.s == nil || c.s.spotanimTypes == nil {
        return nil
    }
    if id < 0 || id >= len(c.s.spotanimTypes.Configs) {
        return nil
    }
    return c.s.spotanimTypes.Configs[id]
}
```

### 3c. Test-mock stubs

Add `SpotAnimType` to each of the 3 Configs implementors in
`pkg/script/`:

- `handlers_db_test.go:26` — `func (f *fakeDbConfigs) SpotAnimType(id int) *objtype.SpotanimType { return nil }`
- `handlers_loc_test.go:27` — `func (f *fakeConfigs) SpotAnimType(id int) *objtype.SpotanimType { return nil }`
- `handlers_config_test.go:29` — `func (m *mockConfigs) SpotAnimType(id int) *objtype.SpotanimType { return m.spotAnimTypes[id] }`
  + add `spotAnimTypes map[int]*objtype.SpotanimType` field to the
  `mockConfigs` struct definition (alongside `idks`, etc.).

### 3d. Pre-flight re-check at T3 dispatch

Re-grep `IdkType\(id ` against HEAD before dispatching T3 to enumerate
`Configs` implementors. The 3-mock list above is current at HEAD
`dfaee46`; an implementor added between spec-write and T3 will surface
as a compile error if missed, but pre-flight catches it earlier.

---

## Task 4 — Tighten `checkSpotAnimType`

### 4a. Edit `pkg/script/handlers_map.go`

Replace the current 4-line range-only validator with a presence-aware
form. Signature gains a `*ScriptState` parameter to access `Configs`:

```go
// checkSpotAnimType validates a spotanim type id by mirroring TS
// SpotAnimTypeValid (ScriptValidators.ts). Rejects negatives and
// any id not present in the SpotanimType config registry. Closes
// deviation NAI-36-D2.
func checkSpotAnimType(s *ScriptState, id int, op string) error {
    if id < 0 {
        return fmt.Errorf("%s: invalid spotanim id (%d)", op, id)
    }
    if s.Configs.SpotAnimType(id) == nil {
        return fmt.Errorf("%s: invalid spotanim id (%d)", op, id)
    }
    return nil
}
```

Update the sole caller at `handleSpotAnimMap`:

```go
// before:
if err := checkSpotAnimType(spotanim, "SPOTANIM_MAP"); err != nil {

// after:
if err := checkSpotAnimType(s, spotanim, "SPOTANIM_MAP"); err != nil {
```

Drop the multi-line `NAI-36-D2:` doc-comment block (currently lines
207-211) and replace with the single-line "Closes deviation NAI-36-D2"
attribution shown above.

### 4b. Pre-flight re-check at T4 dispatch

Re-grep `\bcheckSpotAnimType\(` against HEAD before T4. Sole-caller
invariant must hold; any new caller introduced between spec-write and
T4 dispatch needs the same `s, ` prefix added at the call site.

---

## Task 5 — Extend `handlers_map_test.go`

### 5a. Wire `mockConfigs` into the SPOTANIM_MAP test path

Existing SPOTANIM_MAP tests at `handlers_map_test.go:425-507` build
their `ScriptState` without setting `state.Configs`:

- `TestSpotAnimMap_PopsValidatesAndDelegates` — uses `runMapOp` (line
  324) which constructs `ScriptState` without `Configs:`
- `TestSpotAnimMap_InvalidCoordErrors` — direct `&ScriptState{...}` no
  `Configs:`
- `TestSpotAnimMap_NegativeSpotanimIDErrors` — direct, no `Configs:`
  (passes today on the `id < 0` short-circuit before any Configs deref)
- `TestSpotAnimMap_ZeroDelayPassesThrough` — uses `runMapOp`, no
  `Configs:`

After T4 tightens `checkSpotAnimType`, all three positive-arm tests
(PopsValidatesAndDelegates pushes spotanim=200; ZeroDelayPassesThrough
pushes spotanim=200) will fail because `s.Configs` is nil → nil
interface deref panic OR `s.Configs.SpotAnimType(200)` returns nil →
validation error.

Fix:

1. Extend `runMapOp` signature to accept an optional `Configs`:
   ```go
   func runMapOp(t *testing.T, w WorldVars, c Configs, op Opcode, intInputs []int) *ScriptState {
       // ...
       state := &ScriptState{
           Script:      sf,
           World:       w,
           Configs:     c,
           IntStack:    make([]int, StackCapacity),
           StringStack: make([]string, StackCapacity),
       }
       // ... unchanged
   }
   ```

2. Update the 4 existing `runMapOp` callers in this file (the
   MAP_BLOCKED tests at `handlers_map_test.go:359-408` plus the two
   SPOTANIM_MAP tests above) to pass an appropriate `Configs`:
   - MAP_BLOCKED tests don't touch Configs → pass `nil`
   - SPOTANIM_MAP positive-arm tests pass a `&mockConfigs{spotAnimTypes:
     map[int]*objtype.SpotanimType{200: objtype.NewSpotanimType(200)}}`
     fixture

3. Update the two direct `ScriptState{...}` constructors in
   `TestSpotAnimMap_InvalidCoordErrors` and
   `TestSpotAnimMap_NegativeSpotanimIDErrors`:
   - InvalidCoord: errors on coord BEFORE spotanim check → can pass
     `Configs: nil` (the coord check fires first); but TS-faithfully
     wire `&mockConfigs{}` for hygiene
   - NegativeSpotanim: errors on `id < 0` BEFORE Configs deref → safe
     with `Configs: nil`, but wire `&mockConfigs{}` for hygiene

### 5b. New test arms

- `TestSpotAnimMap_RegisteredIdPasses` — seeded `mockConfigs.spotAnimTypes
  = {7: NewSpotanimType(7)}`; bytecode pushes 7; `World.AnimMap` invoked
  with spotanim=7. Run via `runMapOp(t, w, m, OpSpotAnimMap, ...)`.
- `TestSpotAnimMap_UnregisteredIdRejects` — `mockConfigs.spotAnimTypes
  = {7: ...}`; bytecode pushes 8 (unregistered); error returned;
  `World.AnimMap` NOT invoked. Direct `ScriptState{Configs: m, ...}`
  construction since `runMapOp` calls `Execute` which would `t.Fatalf`
  on the validation error.
- `TestSpotAnimMap_NilEntryRejects` — `mockConfigs.spotAnimTypes =
  {7: nil}` (explicit nil); bytecode pushes 7; error returned (covers
  the registry-has-key-but-nil-value edge — `mockConfigs.SpotAnimType`
  reads from the map and the entry is nil).

The existing `TestSpotAnimMap_NegativeSpotanimIDErrors` already covers
the negative-id branch — keep it unchanged (only the `Configs:` wiring
above changes).

### 5c. Retire NAI-36-D2 doc-comment

Drop the `// NAI-36-D2: SpotAnimType config-port absent at HEAD.
Falling back to ...` block at `handlers_map_test.go:471`. Either
delete the comment outright or replace with a one-line "Pins
post-NAI-58 negative-id rejection" attribution.

### 5d. Cross-file audit

Per `enumerate_all_sites.md` and `retire_deviation_grep_all_comments.md`,
re-grep `rg "NAI-36-D2" pkg/ modules/ cmd/` at T5 close. Expected
post-T5: zero hits (T4 retired the production doc-comment; T5 retires
the test doc-comment). If any other site surfaces, edit it out before
the T5 commit lands.

---

## Task 6 — Close commit

After all tests green:

1. **Stale-deviation grep** — `rg "NAI-36-D2" pkg/ modules/ cmd/` per
   `retire_deviation_grep_all_comments.md`. Expected zero hits. If
   non-zero, edit out before committing.
2. **Tally update** — verify final deviation count via the active
   deviation list (currently at 20 entering NAI-58); expected
   post-close: 19.
3. **Memory updates** — append NAI-58 entry to `nai_followups.md`
   summarising the close (analogous to NAI-57's section). Mark
   NAI-36-D2 as Resolved with closure pointer to NAI-58.
4. **Close commit message** — include trailer:
   ```
   Closes memory: NAI-36-D2
   ```

---

## Bundle / cadence

Single bundle, 6 tasks. Full sub-spec (not compressed; LOC > 15 by an
order of magnitude). NAI-46/NAI-57-shaped.

Estimated LOC at close: ~110 production (port) + ~30 wire-in/accessor +
~10 consumer tightening + ~120 tests. Each task ships green at HEAD;
no inter-task rebases planned.

Implementation mode: **subagent-driven-development** (per
`execution_mode_default.md` memory).

## Memory entries that apply

- `runescript_cadence.md` — full sub-spec cadence with two-stage review
- `execution_mode_default.md` — subagent-driven-development by default
- `controller_preflight.md` — pre-T1/T3/T4 grep verification of plan
  premises against HEAD
- `enumerate_all_sites.md` — T3 mock enumeration, T5/T6 deviation-tag
  grep
- `retire_deviation_grep_all_comments.md` — T5/T6 NAI-36-D2 sweep
- `dead_api_polish.md` — watch for any helper shipped with zero
  consumers at T6 close (current scope has no such risk)
- `plan_runnable_test_fixtures.md` — codified test fixtures (T1, T5)
  must be mentally traced before dispatch
- `close_commit_memory_trailer.md` — T6 close commit carries
  `Closes memory:` trailer
- `true_to_ts_gate.md` — every behavioural change tracked; tally
  bookkept
- `compressed_cadence.md` — explicitly NOT applied (port size exceeds
  threshold)
