# NAI-46 — IdkType Config Port + NAI-45-D3 Retirement

## Motivation

`handleIdkSaveDesign` shipped with deviation NAI-45-D3: the
`IdkType.get(idkit[i])` disable+type validation loop is skipped because no
`IdkType` config registry exists. This sub-spec ports the registry and wires
the validation, retiring NAI-45-D3.

Bundled: `NAI-44-D-CONTINUEWALK-UNUSED` dead-API polish — `tryInteract`'s
`continueWalk bool` parameter has no reader; remove per `dead_api_polish.md`.

**TS reference:** `Engine-TS/src/cache/config/IdkType.ts` (config loader) +
`Engine-TS/src/network/game/client/handler/IdkSaveDesignHandler.ts` (handler).

## Tech Stack

**Go 1.26+** (per `go_version.md`; modern Go via `use-modern-go` skill).

## Deviations

| Tag | Status | Notes |
|-----|--------|-------|
| **NAI-45-D3** | **Retired** | IdkType.get(idkit[i]) disable+type checks now implemented |
| **NAI-44-D-CONTINUEWALK-UNUSED** | **Retired** | `continueWalk` param removed from `tryInteract` |

No new deviations opened.

## Scope

**In scope:**

- `pkg/objtype/idktype.go` — `IdkType` struct + `IdkTypeConfigs` + loader
- `pkg/objtype/idktype_test.go` — decode + parse unit tests
- `modules/world/server.go` — add `idkTypes` field + `LoadIdkTypes` call
- `modules/world/handler_interface.go` — `handleIdkSaveDesign` → `(*Server)` method + idk validation loop + retire NAI-45-D3 deviation comment
- `modules/world/handlers_game.go` — package-level adapter for `gameHandlers[52]` delegating to Server method
- `modules/world/handler_interface_test.go` — migrate 5 existing tests + add 6 new IdkType-validation tests
- `modules/world/interaction.go` — remove `continueWalk bool` from `tryInteract`, update 2 call sites, retire NAI-44-D-CONTINUEWALK-UNUSED comment

**Out of scope:**

- `SETIDKIT` script opcode handler (future compressed sub-spec once IdkType exists)
- `IdkType` exposure via `script.Configs` interface (needed only for SETIDKIT)

---

## Pre-flight (verified at HEAD `9929dd7`)

| Claim | Result |
|---|---|
| `handleIdkSaveDesign` free function in `handler_interface.go` | ✓ |
| `gameHandlers[52] = handleIdkSaveDesign` in `handlers_game.go:62` | ✓ |
| `tryInteract(continueWalk bool)` at `interaction.go:252` | ✓ |
| `_ = continueWalk` inside `tryInteract` at `interaction.go:268` | ✓ |
| `DEVIATION NAI-44-D-CONTINUEWALK-UNUSED` doc-comment at `interaction.go:249` | ✓ (1 site) |
| `tryInteract(false)` caller at `interaction.go:169` | ✓ |
| `tryInteract(p.stepsTaken == 0)` caller at `interaction.go:192` | ✓ |
| No `IdkType`, `idktype`, `IdkTypeConfigs` in `pkg/objtype/` | ✓ absent |
| `idkTypes` field absent from `Server` | ✓ absent |
| `NpcType`/`HuntType` loader patterns in `pkg/objtype/npctype.go` + `hunttype.go` | ✓ dual-source template ready |
| `io "github.com/zsrv/goscape/pkg/io/jagfile"` alias used in `npctype.go` | ✓ |

---

## Task 1 — `pkg/objtype/idktype.go`

Port `Engine-TS/src/cache/config/IdkType.ts` into a new `pkg/objtype/idktype.go`
following the `npctype.go` dual-source pattern.

### Struct + constructor

```go
// IdkType is a single idk.dat record (identity-kit config).
// Mirrors Engine-TS/src/cache/config/IdkType.ts.
type IdkType struct {
    ConfigType
    Type    int       // body-part slot; -1 = unset
    Models  []uint16  // nil = no models
    Heads   [5]uint16 // 0xFFFF = unset (TS Uint16Array.fill(-1))
    RecolS  [6]uint16
    RecolD  [6]uint16
    Disable bool
}

func NewIdkType(id int) *IdkType {
    return &IdkType{
        ConfigType: ConfigType{ID: id},
        Type:       -1,
        Heads:      [5]uint16{0xFFFF, 0xFFFF, 0xFFFF, 0xFFFF, 0xFFFF},
    }
}
```

### Decode

Mirrors `IdkType.decode` at `Engine-TS/src/cache/config/IdkType.ts:62-89`.

```go
func (t *IdkType) Decode(code uint8, dat *packet.Packet) error {
    switch code {
    case 1:
        t.Type = int(dat.G1())
    case 2:
        count := dat.G1()
        t.Models = make([]uint16, count)
        for i := range count {
            t.Models[i] = dat.G2()
        }
    case 3:
        t.Disable = true
    case 40, 41, 42, 43, 44, 45, 46, 47, 48, 49:
        t.RecolS[code-40] = dat.G2()
    case 50, 51, 52, 53, 54, 55, 56, 57, 58, 59:
        t.RecolD[code-50] = dat.G2()
    case 60, 61, 62, 63, 64, 65, 66, 67, 68, 69:
        // TS declares heads as 5-element; codes 65-69 are technically reachable
        // but would be out-of-bounds writes in TS too. Guard in Go to avoid panic.
        slot := code - 60
        v := dat.G2()
        if slot < 5 {
            t.Heads[slot] = v
        }
    case 250:
        t.DebugName = dat.GJStrLF()
    default:
        return fmt.Errorf("unrecognized idk config code %d", code)
    }
    return nil
}
```

### Registry + loader

```go
type IdkTypeConfigs struct {
    ConfigNames map[string]int
    Configs     []*IdkType
}

// LoadIdkTypes parses server/idk.dat + client/config jag → idk.dat.
// Returns an empty registry with nil error when server/idk.dat is absent
// (silent-on-missing, matching TS IdkType.load).
func LoadIdkTypes(dir string) (*IdkTypeConfigs, error) {
    server, err := packet.Load(filepath.Join(dir, "server", "idk.dat"), false)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return &IdkTypeConfigs{ConfigNames: map[string]int{}}, nil
        }
        return nil, err
    }

    clientJag, err := io.LoadJagfile(filepath.Join(dir, "client", "config"))
    if err != nil {
        return nil, err
    }

    return parseIdkTypes(server, clientJag)
}

func parseIdkTypes(server *packet.Packet, clientJag *io.Jagfile) (*IdkTypeConfigs, error) {
    count := int(server.G2())
    configs := make([]*IdkType, count)
    configNames := make(map[string]int, count)

    client, err := clientJag.Read("idk.dat")
    if err != nil {
        return nil, err
    }
    client.Pos = 2 // skip client-side count header

    for id := range count {
        config := NewIdkType(id)
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

    return &IdkTypeConfigs{ConfigNames: configNames, Configs: configs}, nil
}
```

Required imports: `"errors"`, `"fmt"`, `"os"`, `"path/filepath"`,
`io "github.com/zsrv/goscape/pkg/io/jagfile"`,
`packet "github.com/zsrv/goscape/pkg/io/packet"`.

---

## Task 2 — `pkg/objtype/idktype_test.go`

Unit tests using `parseIdkTypes` with hand-crafted byte buffers (same technique
as `hunttype_test.go` / `npctype_test.go`).

### Test list

- `TestNewIdkTypeDefaults` — `NewIdkType(5)` → `Type=-1`, `Disable=false`,
  `Heads=[0xFFFF,0xFFFF,0xFFFF,0xFFFF,0xFFFF]`, `Models=nil`, `ID=5`
- `TestIdkTypeDecode_Type` — code 1 + g1 value → `Type` updated
- `TestIdkTypeDecode_Models` — code 2 + count=2 + two g2 values → `Models` slice
- `TestIdkTypeDecode_Disable` — code 3 → `Disable=true`
- `TestIdkTypeDecode_RecolS` — code 40 → `RecolS[0]` updated
- `TestIdkTypeDecode_RecolD` — code 50 → `RecolD[0]` updated
- `TestIdkTypeDecode_Heads` — code 60 → `Heads[0]` updated; code 64 → `Heads[4]`; code 65 → `Heads` unchanged (out-of-range guard)
- `TestIdkTypeDecode_DebugName` — code 250 + null-terminated string → `DebugName`
- `TestIdkTypeDecode_UnknownCode` — code 99 → error returned
- `TestParseIdkTypes_EmptyCache` — server dat with G2=0 + empty client jag →
  empty `Configs`, nil error

---

## Task 3 — `modules/world/server.go`

Add `idkTypes` field and load call.

```go
// In Server struct (alongside npcTypes / huntTypes):
idkTypes *objtype.IdkTypeConfigs

// In NewServer, after huntTypes:
idkTypes, err := objtype.LoadIdkTypes(cfg.CachePath)
if err != nil {
    return nil, fmt.Errorf("load idk types: %w", err)
}
s.idkTypes = idkTypes
```

---

## Task 4 — `modules/world/handler_interface.go`

**4a. Promote `handleIdkSaveDesign` to a Server method.**

Replace:
```go
func handleIdkSaveDesign(p *Player, payload []byte) error {
```
with:
```go
func (s *Server) handleIdkSaveDesign(p *Player, payload []byte) error {
```

**4b. Insert idk validation loop BEFORE the color loop** (TS-faithful order:
`IdkSaveDesignHandler.ts` runs idk loop at lines 18-33 before color loop at
lines 35-40).

After the gender check (`if gender > 1 { return nil }`), insert:

```go
// IdkType validation — mirrors TS IdkSaveDesignHandler.ts:18-33.
if s.idkTypes != nil {
    for i := range 7 {
        typ := i + gender*7
        if typ == 8 && idkit[i] == -1 { // female jaw exception (TS comment)
            continue
        }
        if idkit[i] < 0 || idkit[i] >= len(s.idkTypes.Configs) {
            return nil
        }
        idk := s.idkTypes.Configs[idkit[i]]
        if idk.Disable || idk.Type != typ {
            return nil
        }
    }
}
```

`s.idkTypes == nil` guard preserves pre-registry behavior for tests that don't
seed the registry.

**4c. Remove the `DEVIATION NAI-45-D3` doc-comment lines** (both the comment
body and the `// no IdkType config registry` line above it).

---

## Task 5 — `modules/world/handlers_game.go`

**5a. Replace** `gameHandlers[52] = handleIdkSaveDesign` with:
```go
gameHandlers[52] = handleIdkSaveDesignGame // IDK_SAVEDESIGN
```

**5b. Add** a package-level adapter following the `handleTutClickSide` /
`handleIfButton` pattern:
```go
func handleIdkSaveDesignGame(p *Player, payload []byte) error {
    if p.client == nil || p.client.server == nil {
        return nil
    }
    return p.client.server.handleIdkSaveDesign(p, payload)
}
```

Note on naming: the adapter is `handleIdkSaveDesignGame` (not `handleIdkSaveDesign`)
to avoid a collision with the `(s *Server) handleIdkSaveDesign` method in the same
package. Go allows same-named package functions and methods, but `handleTutClickSide`
and `handleIfButton` happen to use the same name because the Server method is in a
different file — this is legal Go. Either naming works; the spec chooses the
explicit suffix to keep handlers_game.go self-documenting.

---

## Task 6 — `modules/world/handler_interface_test.go`

### 6a. Migrate 5 existing tests

All 5 existing tests call `handleIdkSaveDesign(p, ...)` as a free function. After
Task 4+5, that symbol becomes the `handleIdkSaveDesignGame` adapter which returns
nil early when `p.client.server == nil`. Tests would pass for the wrong reason
(nil-server guard, not the actual validation). Each must be migrated to call
`s.handleIdkSaveDesign(p, payload)` directly on a `newTestServer(t)` server,
with `p.client.server = s`.

Migration template:
```go
s := newTestServer(t)
p, _ := newTestPlayer(t)
p.client.server = s
p.allowDesign = true

_ = s.handleIdkSaveDesign(p, payload)
```

For the 4 rejection tests (`AllowDesignFalse`, `InvalidGender`, `ColorOutOfBounds`,
`Idkit255ConvertedToMinus1`), the idkTypes registry can stay nil on `s` because
these tests return before the idk loop.

**`TestHandleIdkSaveDesignIdkit255ConvertedToMinus1` requires special handling:**
With the new validation, `idkit[0]=-1` for `gender=0` is now *rejected* (only
female jaw can be -1). The 255→-1 decode still happens, but the test fixture
(`gender=0, idkit[0]=255`) now maps to a rejected path. Restructure as
`TestHandleIdkSaveDesignFemaleJaw255Accepted` — use `gender=1`, `idkit[1]=255`
(wire 255 → -1, type=8, female jaw exception → continue). Confirm `body[1]=-1`
after the call. Requires the registry to have valid entries for all non-jaw
female slots (i≠1).

**`TestHandleIdkSaveDesignSuccess`** passes `gender=1`, `body=[3,4,5,6,7,8,9]`.
With the new idk loop, this needs a seeded registry. Rewrite using a helper
(see §6b) or seed inline.

### 6b. New tests (seeded registry fixture)

Define a helper to build a minimal valid registry:

```go
// buildIdkTypes returns an IdkTypeConfigs seeded with entries 0..N-1.
// Entry i has Type=i and Disable=false (nil = male types 0..6, female types 7..13).
func buildIdkTypes(count int) *objtype.IdkTypeConfigs {
    configs := make([]*objtype.IdkType, count)
    for i := range count {
        c := objtype.NewIdkType(i)
        c.Type = i
        configs[i] = c
    }
    return &objtype.IdkTypeConfigs{
        ConfigNames: map[string]int{},
        Configs:     configs,
    }
}
```

New tests (all use `s.idkTypes = buildIdkTypes(14)` for ids 0..13, types 0..13):

- `TestHandleIdkSaveDesignValidMale` — gender=0, idkit=[0,1,2,3,4,5,6] (types 0-6),
  valid colors → body+colors updated, `MaskAppearance` set
- `TestHandleIdkSaveDesignValidFemaleWithJaw` — gender=1, idkit=[7,8,9,10,11,12,13],
  valid colors → accepted
- `TestHandleIdkSaveDesignFemaleJawMinus1Accepted` — gender=1, idkit[1]=255→-1
  (type=8 female jaw exception), other slots valid → accepted; verify `body[1]=-1`
- `TestHandleIdkSaveDesignDisabledIdk` — seed entry 0 with `Disable=true`, use
  idkit[0]=0 → rejected
- `TestHandleIdkSaveDesignWrongType` — seed entry 0 with `Type=99`, use gender=0
  idkit[0]=0 (expected type=0, got 99) → rejected
- `TestHandleIdkSaveDesignOutOfRangeIdkit` — idkit[0] = len(configs) → rejected

---

## Task 7 — `modules/world/interaction.go`

Remove `continueWalk bool` dead API.

**7a. Function signature** — change:
```go
func (p *Player) tryInteract(continueWalk bool) bool {
```
to:
```go
func (p *Player) tryInteract() bool {
```

**7b. Remove body dead-API line:**
```go
_ = continueWalk
```

**7c. Update call sites:**
- `interaction.go:169`: `p.tryInteract(false)` → `p.tryInteract()`
- `interaction.go:192`: `p.tryInteract(p.stepsTaken == 0)` → `p.tryInteract()`

**7d. Retire deviation comment** — remove `// DEVIATION NAI-44-D-CONTINUEWALK-UNUSED`
doc-comment block at `interaction.go:249` (1 site).

---

## Deviation tally

- Retired: NAI-45-D3, NAI-44-D-CONTINUEWALK-UNUSED
- Opened: none
- Net: −2

---

## Close commit trailer

```
Closes memory: NAI-45-D3, NAI-44-D-CONTINUEWALK-UNUSED
```
