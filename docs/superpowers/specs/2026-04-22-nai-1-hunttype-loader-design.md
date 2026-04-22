# NAI-1 — HuntType Cache Loader

Port the TypeScript `HuntType` cache-config loader
(`Engine-TS/src/cache/config/HuntType.ts`) into Go at
`pkg/objtype/hunttype.go`, and wire the result onto `Server` so later
sub-specs in the NAI series can consume it.

Part of the NPC AI tick decomposition roadmap
(`docs/superpowers/specs/2026-04-22-npc-ai-tick-decomposition-design.md`).
Downstream consumer is NAI-7 (hunt core), which reads `HuntType` records
by ID to drive NPC hunt behaviour. No consumers in NAI-1 itself.

## Goal

- Parse `server/hunt.dat` into a config registry (by ID, by debug name).
- Silently tolerate a missing `hunt.dat` — empty registry, nil error.
- Expose the registry on `*Server` via a `huntTypes` field, adjacent to
  the existing `npcTypes` field.
- Provide the four hunt-related enum families (`HuntModeType`, `HuntVis`,
  `HuntNobodyNear`, `HuntCheckNotTooStrong`) as top-level consts co-located
  with the `HuntType` struct, matching the `NPCMode*` / `MoveRestrict*` /
  `BlockWalk*` precedent at `pkg/objtype/npctype.go:24-48`.

## Non-goals

- No accessor method on `Server` for hunt lookups — NAI-7 adds that when
  it first needs to read by ID.
- No script-VM integration — `OpNpcSetHunt` / `OpNpcSetHuntMode` handler
  wiring is NAI-7's responsibility.
- No `checkHuntCondition` helper — TS defines it on `HuntType` at
  `HuntType.ts:63-75` but NAI-1 ships the data shape only; evaluation
  logic lives with its consumer (NAI-7/8/9) per Go single-responsibility.
- No golden-byte fidelity test vs. the TS loader's `.dat` output. Unit
  decode-per-opcode coverage plus an integration round-trip test gives
  equivalent guarantees without introducing a cross-project test fixture.

## TS reference

- `Engine-TS/src/cache/config/HuntType.ts` (full file, 148 lines) — data
  shape, defaults, decode switch, load path.
- `Engine-TS/src/engine/entity/hunt/HuntModeType.ts`,
  `HuntVis.ts`, `HuntNobodyNear.ts`, `HuntCheckNotTooStrong.ts` — the
  four enum families.

## Architecture

### New files

- `pkg/objtype/hunttype.go` — all production code
- `pkg/objtype/hunttype_test.go` — all tests

### Structure of `hunttype.go` (in file order)

1. Package-level enum consts for the four hunt-related enum families.
2. `HuntCheckVar` struct (one triple: var ID, condition operator, value).
3. `HuntType` struct, embedding `ConfigType`.
4. `Decode(code uint8, dat *packet.Packet) error` method.
5. `NewHuntType(id int) *HuntType` factory with defaults.
6. `HuntTypeConfigs` struct.
7. `LoadHuntTypes(dir string) (*HuntTypeConfigs, error)` entry point.
8. `parseHuntTypes(server *packet.Packet) (*HuntTypeConfigs, error)` — count-loop.

### Enum definitions

Inline consts at the top of `hunttype.go`:

```go
// HuntModeType values mirror TS HuntModeType.
const (
    HuntModeOff     = 0
    HuntModePlayer  = 1
    HuntModeNpc     = 2
    HuntModeObj     = 3
    HuntModeScenery = 4
)

// HuntVis values mirror TS HuntVis.
const (
    HuntVisOff         = 0
    HuntVisLineOfSight = 1
    HuntVisLineOfWalk  = 2
)

// HuntNobodyNear values mirror TS HuntNobodyNear.
const (
    HuntNobodyNearKeepHunting = 0
    HuntNobodyNearPauseHunt   = 1
)

// HuntCheckNotTooStrong values mirror TS HuntCheckNotTooStrong.
const (
    HuntCheckNotTooStrongOff               = 0
    HuntCheckNotTooStrongOutsideWilderness = 1
)
```

### Type definitions

```go
type HuntCheckVar struct {
    VarID     int
    Condition string
    Val       int
}

type HuntType struct {
    ConfigType
    Type                 int
    CheckVis             int
    CheckNotTooStrong    int
    CheckNotBusy         bool
    FindKeepHunting      bool
    FindNewMode          int
    NobodyNear           int
    CheckNotCombat       int
    CheckNotCombatSelf   int
    CheckAfk             bool
    Rate                 int
    CheckCategory        int
    CheckNpc             int
    CheckObj             int
    CheckLoc             int
    CheckInv             int
    CheckObjParam        int
    CheckInvCondition    string
    CheckInvVal          int
    CheckVars            []HuntCheckVar
}

type HuntTypeConfigs struct {
    ConfigNames map[string]int
    Configs     []*HuntType
}
```

### `NewHuntType` defaults

Mirrors TS field-initialiser defaults at `HuntType.ts:78-97`:

| Field | Default |
|---|---|
| `Type` | `HuntModeOff` |
| `CheckVis` | `HuntVisOff` |
| `CheckNotTooStrong` | `HuntCheckNotTooStrongOff` |
| `CheckNotBusy` | `false` |
| `FindKeepHunting` | `false` |
| `FindNewMode` | `NPCModeNull` (-1, already in `npctype.go`) |
| `NobodyNear` | `HuntNobodyNearPauseHunt` |
| `CheckNotCombat` | `-1` |
| `CheckNotCombatSelf` | `-1` |
| `CheckAfk` | `true` |
| `Rate` | `1` |
| `CheckCategory`, `CheckNpc`, `CheckObj`, `CheckLoc`, `CheckInv`, `CheckObjParam`, `CheckInvVal` | `-1` |
| `CheckInvCondition` | `""` |
| `CheckVars` | `nil` (lazy-allocated on first append; matches TS `[]`-init behaviour given no-presence-check downstream) |

### `Decode` cases

Port of TS switch at `HuntType.ts:99-147`:

| Code | Field(s) | Read order |
|---|---|---|
| 1 | `Type` | `G1` |
| 2 | `CheckVis` | `G1` |
| 3 | `CheckNotTooStrong` | `G1` |
| 4 | `CheckNotBusy = true` | (no read) |
| 5 | `FindKeepHunting = true` | (no read) |
| 6 | `FindNewMode` | `G1` |
| 7 | `NobodyNear` | `G1` |
| 8 | `CheckNotCombat` | `G2` |
| 9 | `CheckNotCombatSelf` | `G2` |
| 10 | `CheckAfk = false` | (no read) |
| 11 | `Rate` | `G2` |
| 12 | `CheckCategory` | `G2` |
| 13 | `CheckNpc` | `G2` |
| 14 | `CheckObj` | `G2` |
| 15 | `CheckLoc` | `G2` |
| 16 | `CheckInv`, `CheckObj`, `CheckInvCondition`, `CheckInvVal` | `G2, G2, GJStrLF, int32(G4())` |
| 17 | `CheckInv`, `CheckObjParam`, `CheckInvCondition`, `CheckInvVal` | `G2, G2, GJStrLF, int32(G4())` |
| 18, 19, 20 | append `HuntCheckVar{G2, GJStrLF, int32(G4())}` to `CheckVars` | `G2, GJStrLF, int32(G4())` |

**Note:** The TS loader uses `g4s()` (signed read) but the Go `packet` package
has no `G4S`; established precedent at `pkg/objtype/enumtype.go:31, 35, 41, 42`
and `pkg/objtype/invtype.go:49` is `int32(dat.G4())`. Matches TS bit-for-bit
(both are a 4-byte big-endian read reinterpreted as signed).
| 250 | `DebugName` (via embedded `ConfigType`) | `GJStrLF` |
| default | `return fmt.Errorf("unrecognized hunt config code %d", code)` | — |

Cases 18/19/20 use `case 18, 19, 20:` — single branch, identical behaviour
to TS `code > 17 && code < 21`.

### `LoadHuntTypes` / `parseHuntTypes`

```go
func LoadHuntTypes(dir string) (*HuntTypeConfigs, error) {
    server, err := packet.Load(filepath.Join(dir, "server", "hunt.dat"), false)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return &HuntTypeConfigs{
                ConfigNames: map[string]int{},
                Configs:     nil,
            }, nil
        }
        return nil, err
    }
    return parseHuntTypes(server)
}
```

`parseHuntTypes` follows `parseNPCTypes` at `npctype.go:284-319`,
minus the client-jag leg:

```go
func parseHuntTypes(server *packet.Packet) (*HuntTypeConfigs, error) {
    count := int(server.G2())
    configs := make([]*HuntType, count)
    configNames := make(map[string]int, count)

    for id := range count {
        config := NewHuntType(id)
        if err := DecodeType(server, config); err != nil {
            return nil, err
        }
        configs[id] = config
        if config.DebugName != "" {
            configNames[config.DebugName] = id
        }
    }

    return &HuntTypeConfigs{
        ConfigNames: configNames,
        Configs:     configs,
    }, nil
}
```

## Test strategy

File: `pkg/objtype/hunttype_test.go`.

1. **`TestHuntTypeDefaults`** — `NewHuntType(0)` assertions against every
   default listed in the table above. Catches silent default drift.
2. **`TestHuntTypeDecodeAllOpcodes`** — table-driven, one row per opcode
   (1–17, 18, 19, 20, 250). Each row builds a packet with one `[code,
   payload]` pair, calls `DecodeType`, asserts the resulting field(s).
   Single-opcode granularity isolates regressions.
3. **`TestHuntTypeDecodeUnknownOpcode`** — packet with code 42, asserts
   error containing `"unrecognized hunt config code 42"`.
4. **`TestHuntTypeDecodeCheckVarsAppend`** — single record with codes 18,
   19, 20 in sequence, asserts `CheckVars` has 3 entries in insertion order.
5. **`TestLoadHuntTypesTwoRecords`** — build temp dir, write a handcrafted
   `server/hunt.dat` containing two records (count=2), load, assert both
   appear in `Configs` with populated fields, and `ConfigNames` maps
   debug names to IDs.
6. **`TestLoadHuntTypesMissingFile`** — temp dir with no `hunt.dat`,
   asserts `LoadHuntTypes` returns `(cfgs, nil)` where
   `cfgs.Configs == nil` and `cfgs.ConfigNames` is empty (not nil, not
   error).

**Note on parse-error testing:** An earlier revision of this spec
included a seventh test for truncated-payload parse errors, which
required `parseHuntTypes` to recover from `packet.Packet` EOF panics
via `defer recover()`. This was removed after discovery during
implementation (commit `36df706`): no other loader in `pkg/objtype`
does panic recovery, and TS `HuntType.load` at
`cache/config/HuntType.ts:16-22` has no equivalent. `parseHuntTypes`
matches the straight-through shape of `parseVarpTypes` /
`parseEnumTypes`; truncated input will panic, which is consistent
with project convention.

Test data uses the existing `packet.Packet` builder methods (`P1`, `P2`,
`PJStrLF`, `P4`) per the `varptype_test.go:19-37` `buildVarpDat` helper
pattern. Signed writes use `pkt.P4(uint32(signedVal))`.

## Server wiring

File: `modules/world/server.go`.

1. Add field adjacent to `npcTypes` at `server.go:82`:
   ```go
   huntTypes   *objtype.HuntTypeConfigs
   ```

2. In `NewServer`, immediately after the existing `LoadNPCTypes` block
   that ends at `server.go:191`, insert:
   ```go
   huntTypes, err := objtype.LoadHuntTypes(cfg.CachePath)
   if err != nil {
       return nil, fmt.Errorf("load hunt types: %w", err)
   }
   s.huntTypes = huntTypes
   ```

3. No test changes to existing server-startup tests; the load path is
   silent on missing files so existing test fixtures (which don't have
   `hunt.dat`) continue to pass.

## Fidelity notes

- **Missing `hunt.dat` is silent.** TS `HuntType.load` at `HuntType.ts:17-19`
  explicitly uses `if (!fs.existsSync(...)) return;`. Go matches by
  catching `os.ErrNotExist` and returning an empty registry. This is a
  deliberate TS-authoritative choice — hunt-less caches are a supported
  scenario in the reference implementation.
- **Field naming uses Go idiom** (`CheckNotBusy` vs TS `checkNotBusy`).
  This is a stylistic divergence, not behavioural — no downstream
  consumer depends on field casing because Go `json`-tagged output isn't
  involved.
- **`CheckVars` is `nil` by default** vs TS `[]` by default. In Go,
  appending to a `nil` slice is valid and yields identical results to
  appending to `[]`; the only observable difference is `len == 0` in both
  cases. No behavioural divergence.

## Rough LOC

Matches roadmap estimate of ~150 prod+test.

- `hunttype.go`: ~150 lines (enums + type + defaults + decode + load).
- `hunttype_test.go`: ~200 lines (7 tests, most table-driven).
- `server.go` diff: ~8 lines.

## Dependencies

- **Blocks:** NAI-7 (hunt core dispatcher) — consumer of the registry.
- **Blocked by:** nothing. Can ship first in the NAI series.
