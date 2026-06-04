# NAI-59 — ComponentType Config Port + IF_BUTTON Activation

## Motivation

`modules/world/handler_interface.go:24-44` `handleIfButton` ships under
two tracked deviations — **NAI-45-D1** (skips `buttonType == NO_BUTTON`
+ `isComponentVisible` checks) and **NAI-45-D2** (uses `protect=true`
unconditionally; TS uses `root.overlay == false`) — because the
`Component` config registry was unported when NAI-45 landed.
TS `IfButtonHandler.ts:12-41` has the full gate sequence
(component-lookup → buttonType → isComponentVisible → resumeButtons →
trigger script with protect derived from root.overlay).

NAI-46 (IdkType), NAI-57 (SeqType), and NAI-58 (SpotanimType) extended
the config-port chain. NAI-59 continues the chain with `Component`,
the most-referenced unported config registry — eight active deviations
across six handler files reference its absence:

| Deviation | Tag site | Cluster |
|---|---|---|
| NAI-45-D1 | `handler_interface.go:18` (IF_BUTTON) | this sub-spec |
| NAI-45-D2 | `handler_interface.go:22` (IF_BUTTON) | this sub-spec |
| S6o-D1 | `handler_opnpc.go:97` (OPNPCT spellCom) | NAI-60+ |
| S6o-D2 | `handler_opnpc.go:177` (OPNPCU useCom) | NAI-60+ |
| NAI-40-D-COMPONENT-REGISTRY-VALIDATION-SKIPPED | `handler_op_player.go:74,134` | NAI-60+ |
| NAI-48-D1 | `handler_inv_button.go:21,74` | NAI-60+ |
| NAI-50-D1 | `handler_opobj.go:95` (OPOBJT) | NAI-60+ |
| NAI-50-D2 | `handler_opobj.go:159` (OPOBJU) | NAI-60+ |
| NAI-53-D-CLEARCOMLISTENERS-PER-SLOT | `player_script.go:633` (CloseModal) | NAI-60+ |

NAI-59 closes only NAI-45-D1 + NAI-45-D2. Cluster siblings remain
tracked and become straightforward NAI-60+ follow-ups once the
registry + `IsComponentVisible` exist.

**TS reference:** `Engine-TS/src/cache/config/Component.ts` (327 LOC,
filename `Component.ts`, struct named `Component`, dual-source loader
reading `client/interface` jagfile + `server/interface.dat` packet);
gate references `Engine-TS/src/network/game/client/handler/IfButtonHandler.ts:12-41`
and `Engine-TS/src/engine/entity/Player.ts:2042-2049` (`setTab` +
`isComponentVisible`).

## Tech Stack

**Go 1.26+** (per `go_version.md`; modern Go via `use-modern-go` skill).

## Deviations

| Tag | Status | Notes |
|-----|--------|-------|
| **NAI-45-D1** | **Retired** | `handleIfButton` rejects `nil` component, `buttonType == ButtonNone`, and `!IsComponentVisible(com)` |
| **NAI-45-D2** | **Retired** | `handleIfButton` derives `protect = root == nil \|\| !root.Overlay` from the rootLayer component |
| **NAI-59-D-MODALTUTORIAL-NO-PRODUCER** | **New** | `Player.modalTutorial` field exists and is read by `IsComponentVisible`, but no goscape opcode handler writes it. TS `Player.openTutorial` (called from a script opcode at `Player.ts:2002`) is unported. Read-path consumer exists, so this is "stub-not-completed" per `protocol_stub_not_completed.md` rather than dead-API. Closure: future `IF_OPENTUT` (or equivalent) opcode handler sub-spec. Same shape applies to the unported tutorial branch in `(*Player).CloseModal` (TS `Player.ts:717-723`) — observed pre-existing gap, NOT introduced by NAI-59. |

**Tally:** 19 → 18 (-2 +1).

The `comType`-switched and `buttonType`-switched fields beyond what
the IF_BUTTON gate and IsComponentVisible read (e.g., `actionVerb`,
`actionTarget`, `inventorySlotOffsetX/Y`, `text`, `colour`, all model /
graphic / anim slots) are parsed and stored faithfully but currently
have zero consumers — TS-faithful struct shape per `true_to_ts_gate.md`,
not a deviation.

## Scope

**In scope:**

- `pkg/objtype/componenttype.go` — `ComponentType` struct + `Decode`
  (jagfile `data` entry) + `DecodeExtra` (server `interface.dat`) +
  `ComponentTypeConfigs` registry + `LoadComponentTypes(dir)` loader +
  comType / buttonType / ComActionTarget constants
- `pkg/objtype/componenttype_test.go` — per-comType-arm + per-buttonType-arm
  decode tests + `decodeExtra` round-trip + registry construction
- `modules/world/server.go` — add `componentTypes` field + bootstrap wire-in
- `modules/world/player.go` — add `tabs [14]int` + `modalTutorial int`
  fields; initialize both in `newPlayer` to all-`-1`
- `modules/world/player_interface.go` — `IfSetTab` reshape (state-write
  before wire emit); new `(p *Player) IsComponentVisible(com *objtype.ComponentType) bool`
  method
- `modules/world/handler_interface.go` — `handleIfButton` gates +
  `protect = !root.Overlay` derivation + retire NAI-45-D1/D2 doc-comments
- `modules/world/handler_interface_test.go` + new
  `modules/world/player_interface_test.go` — gate-arm tests +
  IsComponentVisible branch tests + IfSetTab state+wire tests

**Out of scope (deferred):**

- Activating Component validation in any non-IF_BUTTON handler. The
  cluster-sibling deviations (S6o-D1/D2, NAI-40-D-COMPONENT-REGISTRY-
  VALIDATION-SKIPPED ×2, NAI-48-D1 ×2, NAI-50-D1/D2,
  NAI-53-D-CLEARCOMLISTENERS-PER-SLOT) remain tagged. Each is a
  small NAI-60+ follow-up: import `objtype` if needed, call
  `s.Configs.Component(id)`, apply the same gate pattern as
  NAI-59 T6.
- `Player.openTutorial` opcode handler (writes `modalTutorial`) and
  the `CloseModal` tutorial branch (TS `Player.ts:717-723`). Tracked
  under NAI-59-D-MODALTUTORIAL-NO-PRODUCER; closure: future tutorial-
  opcode sub-spec.
- Login-path bulk `IfSetTab` emission for all 14 tabs (TS `Player.ts:546`
  loop). Goscape's login flow already differs from TS in tab-restore
  semantics; this is a separate scope.
- Component child-coordinate plumbing (`childId`, `childX`, `childY`).
  Decoded into the struct for full TS-faithfulness but no goscape
  consumer reads them.

---

## Pre-flight (verified at HEAD `56315d1`)

| Claim | Result |
|---|---|
| `Engine-TS/src/cache/config/Component.ts` is 327 LOC; `decode` (L43-234) + `decodeExtra` (L237-250) + `get`/`getId`/`getByName` (L252-267); fields at L270-318; ComActionTarget enum at L321-327 | ✓ |
| `modules/world/handler_interface.go:24` `handleIfButton` is the IF_BUTTON entry-point with NAI-45-D1/D2 doc-comments at L18-23 | ✓ |
| `modules/world/handler_interface.go` has no `_test.go` sibling holding new IF_BUTTON tests; existing tests in `handler_interface_test.go` cover SetLastCom + ResumePauseButton + PauseButtonNotInResumeButtons (L131-218) | ✓ |
| `modules/world/player.go:200` declares `modalMain, modalChat, modalSide int` (no `tabs`, no `modalTutorial` at HEAD) | ✓ — confirmed via `grep -nE "tabs\\b\|modalTutorial" player.go` returns no matches |
| `modules/world/player.go:35-37` defines `modalStateMain=0x1`, `modalStateChat=0x2`, `modalStateSide=0x4` bitmap; modal-presence is gated via `modalState`, not `modal* != -1` (goscape divergence from TS-default-(-1) sentinel) | ✓ — IsComponentVisible must gate each modal-arm on `modalState&...` not raw equality, since `modalMain/Chat/Side` are NOT initialized to -1 in `newPlayer` (zero-valued by Go default) |
| `modules/world/player_interface.go:67-72` `IfSetTab` is wire-only; no server-side state | ✓ |
| `modules/world/server.go:87-89` Server holds `idkTypes`, `seqTypes`, `spotanimTypes` config fields; bootstrap loads them at L237/248/254 | ✓ |
| Server-side client-game handlers (handler_*.go) access typed registries via direct field reads (e.g., `s.idkTypes.Configs[i]` at `handler_interface.go:83-86`), NOT via the `Configs` interface (which is for script-state dispatch only) | ✓ — confirmed via `grep "s.idkTypes.Configs\[" modules/world/handler_*.go`; pattern: server-side handler reads registry field directly, script-side handler reads `state.Configs.IdkType(id)` |
| `pkg/objtype/spotanimtype.go` is the most-recent sibling template (NAI-58) for ConfigType + dual-source loader pattern | ✓ |
| `pkg/io/jagfile` package exposes `LoadJagfile` (returns `*Jagfile`) + `(*Jagfile).Read(name)` returning `*packet.Packet`; `Available()` (or equivalent) supports the TS `dat.available > 0` while-loop | needs T1 pre-flight grep — verify `Read` return shape and available-bytes API on `packet.Packet` before T2 dispatch |
| Cache data files staged: `data/pack/server/interface.dat` + `data/pack/client/interface` (jagfile) | needs T2 pre-flight `ls` |
| `pkg/objtype/configtype.go` exposes `ConfigType` base + `DecodeType(buf, decoder)` polymorphic helper | ✓ — used by every sibling type port |

---

## Task 1 — `pkg/objtype/componenttype.go` struct + constants

Port the data model (no decoder yet; T2 adds decoders). Mirror the
field set in `Engine-TS/src/cache/config/Component.ts:269-318` and the
constants at L7-22 + L321-327.

### Constants

```go
package objtype

// ComType discriminator values per TS Component.ts:7-14.
const (
    ComTypeLayer         = 0
    ComTypeUnused        = 1
    ComTypeInventory     = 2
    ComTypeRect          = 3
    ComTypeText          = 4
    ComTypeSprite        = 5
    ComTypeModel         = 6
    ComTypeInventoryText = 7
)

// Button discriminator values per TS Component.ts:16-22.
const (
    ButtonNone   = 0
    Button       = 1
    ButtonTarget = 2
    ButtonClose  = 3
    ButtonToggle = 4
    ButtonSelect = 5
    ButtonPause  = 6
)

// ComActionTarget bitmask per TS Component.ts:321-327. Currently no
// goscape consumer reads these; ported for true_to_ts_gate parity.
const (
    ComActionTargetObj    = 1
    ComActionTargetNpc    = 2
    ComActionTargetLoc    = 4
    ComActionTargetPlayer = 8
    ComActionTargetHeld   = 16
)
```

### Struct + constructor

```go
// ComponentType is a single interface component (widget) config record.
// Mirrors Engine-TS/src/cache/config/Component.ts (fields at L270-318).
type ComponentType struct {
    ConfigType
    RootLayer            int
    ComName              string
    Overlay              bool
    ComType              int
    ButtonType           int
    ClientCode           int
    Width                int
    Height               int
    OverLayer            int
    ScriptComparator     []uint8
    ScriptOperand        []uint16
    Scripts              [][]uint16
    Scroll               int
    Hide                 bool
    Draggable            bool
    Operable             bool
    Usable               bool
    MarginX              int
    MarginY              int
    InventorySlotOffsetX []uint16
    InventorySlotOffsetY []uint16
    InventorySlotGraphic []string
    Iop                  []string
    Fill                 bool
    Center               bool
    Font                 int
    Shadowed             bool
    Text                 string
    ActiveText           string
    Colour               int
    ActiveColour         int
    OverColour           int
    Graphic              string
    ActiveGraphic        string
    Model                int
    ActiveModel          int
    Anim                 int
    ActiveAnim           int
    Zoom                 int
    Xan                  int
    Yan                  int
    ActionVerb           string
    Action               string
    ActionTarget         int
    Option               string
    ChildId              []uint16
    ChildX               []uint16
    ChildY               []uint16
}

// NewComponentType returns a ComponentType with TS-faithful defaults.
// TS defaults at Component.ts:270-318: id=-1 (set by caller),
// rootLayer=-1, comType=-1, buttonType=-1, overLayer=-1, model=-1,
// activeModel=-1, anim=-1, activeAnim=-1, actionTarget=-1.
func NewComponentType(id int) *ComponentType {
    return &ComponentType{
        ConfigType:   ConfigType{ID: id},
        RootLayer:    -1,
        ComType:      -1,
        ButtonType:   -1,
        OverLayer:    -1,
        Model:        -1,
        ActiveModel:  -1,
        Anim:         -1,
        ActiveAnim:   -1,
        ActionTarget: -1,
    }
}
```

### Test list

`pkg/objtype/componenttype_test.go`:

- `TestNewComponentTypeDefaults` — `NewComponentType(7)` → ID=7,
  RootLayer/ComType/ButtonType/OverLayer/Model/ActiveModel/Anim/ActiveAnim/ActionTarget = -1,
  all other ints/bools/strings/slices zero-valued (no panics on later
  decode-loop nil-slice append paths).

(No decoder exercised yet; T1 ships the type only.)

---

## Task 2 — `pkg/objtype/componenttype.go` decoders

Port `Component.decode` (TS L43-234) and `Component.decodeExtra` (TS
L237-250) + the registry + loader. The decoder structure does not fit
the per-opcode `Decode(code uint8, dat *packet.Packet)` shape used by
`spotanimtype.go` — Component decodes a fixed-shape record per id
(no opcode dispatch), with two nested switches (comType then
buttonType) at TS L99-230. Departure from the sibling pattern is
TS-faithful.

### Top-level loader

```go
// LoadComponentTypes reads the dual-source Component config:
//   - dir/client/interface (jagfile, "data" entry; ~half the fields)
//   - dir/server/interface.dat (raw packet; debugname + overlay)
// Mirrors TS Component.load (Component.ts:27-41). Silent-on-missing
// for the client jagfile (returns empty registry) matching TS.
func LoadComponentTypes(dir string) (*ComponentTypeConfigs, error) {
    clientJag, err := io.LoadJagfile(filepath.Join(dir, "client", "interface"))
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return &ComponentTypeConfigs{ConfigNames: map[string]int{}}, nil
        }
        return nil, err
    }
    clientData, err := clientJag.Read("data")
    if err != nil {
        // TS: `if (!client.has('data'))` → return without populating
        return &ComponentTypeConfigs{ConfigNames: map[string]int{}}, nil
    }

    server, err := packet.Load(filepath.Join(dir, "server", "interface.dat"), false)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            // Decode-only path (no debugname / overlay extension).
            return parseComponentTypes(clientData, nil)
        }
        return nil, err
    }
    return parseComponentTypes(clientData, server)
}
```

### `parseComponentTypes` shape

The full body is large (~150 LOC) — abbreviated structure here; T2's
plan-stage code block will enumerate every comType / buttonType arm
field-by-field per TS L99-230.

```go
func parseComponentTypes(client *packet.Packet, server *packet.Packet) (*ComponentTypeConfigs, error) {
    // configs is sized large because component IDs are sparse and
    // can exceed the leading count. TS uses Component.components[id]
    // as a sparse array; goscape uses a slice with capacity sized
    // to the maximum observed id. We pre-scan or grow on assignment.
    var configs []*ComponentType
    configNames := make(map[string]int)

    client.G2() // count header (advisory; TS reads then ignores)

    rootLayer := -1
    for client.Available() > 0 {
        id := int(client.G2())
        if id == 65535 {
            rootLayer = int(client.G2())
            id = int(client.G2())
        }
        com := NewComponentType(id)
        com.RootLayer = rootLayer
        com.ComType = int(client.G1())
        com.ButtonType = int(client.G1())
        com.ClientCode = int(client.G2())
        com.Width = int(client.G2())
        com.Height = int(client.G2())

        overLayer := int(client.G1())
        if overLayer == 0 {
            com.OverLayer = -1
        } else {
            com.OverLayer = ((overLayer - 1) << 8) + int(client.G1())
        }

        comparatorCount := int(client.G1())
        if comparatorCount > 0 {
            com.ScriptComparator = make([]uint8, comparatorCount)
            com.ScriptOperand = make([]uint16, comparatorCount)
            for i := range comparatorCount {
                com.ScriptComparator[i] = client.G1()
                com.ScriptOperand[i] = client.G2()
            }
        }

        scriptCount := int(client.G1())
        if scriptCount > 0 {
            com.Scripts = make([][]uint16, scriptCount)
            for i := range scriptCount {
                opcodeCount := int(client.G2())
                com.Scripts[i] = make([]uint16, opcodeCount)
                for j := range opcodeCount {
                    com.Scripts[i][j] = client.G2()
                }
            }
        }

        switch com.ComType {
        case ComTypeLayer:
            // TS L100-104: scroll, hide, childId/X/Y arrays
            // ... per-arm field reads
        case ComTypeUnused:
            // TS L107-115: fill, center, font, shadowed, text/activeText, colour/active/over
        case ComTypeInventory:
            // TS L118-148: draggable, operable, usable, marginX/Y, slot offsets, slot graphics, iop
        case ComTypeRect:
            // TS L151-156
        case ComTypeText:
            // TS L159-176
        case ComTypeSprite:
            // TS L179-184
        case ComTypeModel:
            // TS L187-198
        case ComTypeInventoryText:
            // TS L201-213
        }

        switch com.ButtonType {
        case ButtonNone:
            // no extra fields
        case ButtonTarget:
            com.ActionVerb = client.GJStrLF()
            com.Action = client.GJStrLF()
            com.ActionTarget = int(client.G2())
        case Button, ButtonToggle, ButtonSelect, ButtonPause:
            com.Option = client.GJStrLF()
        }

        // Grow configs slice if id exceeds current cap.
        if id >= len(configs) {
            grown := make([]*ComponentType, id+1)
            copy(grown, configs)
            configs = grown
        }
        configs[id] = com
    }

    if server != nil {
        server.G2() // count header (advisory)
        for server.Available() > 0 {
            id := int(server.G2())
            debugName := server.GJStrLF()
            overlay := server.G1() != 0
            if id < len(configs) && configs[id] != nil {
                configs[id].ComName = debugName
                configs[id].Overlay = overlay
                configs[id].DebugName = debugName
                configNames[debugName] = id
            }
        }
    }

    return &ComponentTypeConfigs{ConfigNames: configNames, Configs: configs}, nil
}

// ComponentTypeConfigs is the parsed registry of all component records.
type ComponentTypeConfigs struct {
    ConfigNames map[string]int
    Configs     []*ComponentType
}
```

**Plan-stage requirement:** the writing-plans skill MUST enumerate
every TS L99-230 per-arm field read explicitly in the T2 task block
(not the abbreviated `// ... per-arm field reads` shorthand above).
Per `audit_full_method_against_ts.md` and `spec_ts_source_read.md`,
plan-author shall read TS line-by-line, not by analogy.

### Test list

`pkg/objtype/componenttype_test.go`:

- `TestComponentDecode_TypeLayer` — id=10, comType=ComTypeLayer +
  scroll/hide/childCount=2 fixture → ChildId/X/Y arrays populated
- `TestComponentDecode_TypeInventory` — comType=ComTypeInventory with
  draggable/operable/usable/marginX/Y/slotCount/iop fixture
- `TestComponentDecode_TypeRect` — fill/colour/activeColour/overColour
- `TestComponentDecode_TypeText` — center/font/shadowed/text/activeText/colour/active/over
- `TestComponentDecode_TypeSprite` — graphic/activeGraphic/colour/active/over
- `TestComponentDecode_TypeModel` — model/activeModel/anim/activeAnim/zoom/xan/yan/colour/active/over
- `TestComponentDecode_TypeInventoryText` — center/font/shadowed/colour/marginX/marginY/operable/iop
- `TestComponentDecode_ButtonNoneNoExtra` — buttonType=ButtonNone, no
  trailing reads
- `TestComponentDecode_ButtonTarget` — buttonType=ButtonTarget →
  ActionVerb/Action/ActionTarget populated
- `TestComponentDecode_Button_ToggleSelectPause_Option` — table-driven,
  one case per of {Button, ButtonToggle, ButtonSelect, ButtonPause} →
  Option populated
- `TestComponentDecode_RootLayerSentinel` — id=65535 marker reads next
  G2 as `rootLayer` and continues with the real id
- `TestComponentDecode_ScriptComparator` — comparatorCount=3 →
  ScriptComparator + ScriptOperand both length 3
- `TestComponentDecode_ScriptsArray` — scriptCount=2 with opcodeCount=4
  each → Scripts[2][4] populated
- `TestComponentDecode_OverLayerZero` → OverLayer=-1
- `TestComponentDecode_OverLayerNonZero` → OverLayer = ((b-1)<<8)+next
- `TestParseComponentTypes_DecodeExtraSetsOverlayAndComName` —
  decodeExtra round-trip: server packet with id=10 + debugname="foo" +
  overlay=1 → configs[10].ComName="foo", Overlay=true,
  ConfigNames["foo"]=10
- `TestParseComponentTypes_DecodeExtraOnUnknownIdSilentlyDiscarded` —
  server-only id (not in client jag) → no panic, ConfigNames unchanged
- `TestLoadComponentTypes_MissingClientJagfileReturnsEmpty` — tmp dir
  without `client/interface` → empty registry, nil error
- `TestLoadComponentTypes_MissingServerInterfaceDatStillDecodes` — tmp
  dir with `client/interface` only → registry populated, ComName/Overlay
  zero-valued

---

## Task 3 — Server registry wiring

No `Configs` interface extension. Server-side `handleIfButton` accesses
the registry directly via `s.componentTypes.Configs[id]`, mirroring the
sibling pattern in `handleIdkSaveDesign` at `handler_interface.go:83-86`
(`s.idkTypes.Configs[idkit[i]]`). The `Configs` interface (in
`pkg/script/`) is for script-state-side dispatch (handler_*.go files in
`pkg/script/`), and NAI-59 has no script-side consumer of Component —
all TS `isComponentVisible` callers (IfButtonHandler, OpHeldHandler,
OpObjU, OpLocT, etc.) live in `network/game/client/handler/` (= goscape's
`modules/world/handler_*.go`), all server-side. Adding `Configs.Component`
would be dead-API per `dead_api_polish.md`.

### 3a. `modules/world/server.go`

Add field alongside `idkTypes`/`seqTypes`/`spotanimTypes` (existing
block at server.go:87-89):

```go
componentTypes *objtype.ComponentTypeConfigs
```

In the bootstrap path that already loads `spotanimTypes` (currently
around `server.go:254`), append:

```go
componentTypes, err := objtype.LoadComponentTypes(cfg.CachePath)
if err != nil {
    return nil, fmt.Errorf("load component types: %w", err)
}
s.componentTypes = componentTypes
```

### Test list

- `TestNewServer_ComponentTypesLoaded` — server bootstrap with cache
  path containing `client/interface` + `server/interface.dat` → `s.componentTypes != nil`
  + at least one entry retrievable.
- `TestNewServer_ComponentTypesEmptyOnMissingCache` — server bootstrap
  with empty cache path → `s.componentTypes != nil` (empty registry, not
  nil) so handlers can index without nil-checking the wrapper.

---

## Task 4 — `Player.tabs[14]` + `Player.modalTutorial` + `IfSetTab` state-write

### 4a. Player field declarations

`modules/world/player.go:200` block currently reads:

```go
modalMain, modalChat, modalSide                    int
```

Replace with (insertion in the same field block):

```go
modalMain, modalChat, modalSide, modalTutorial     int
tabs                                               [14]int
```

### 4b. `newPlayer` initialization

`modules/world/player.go:344-431` `newPlayer` constructor. Add inside
the struct literal (before the closing `}`):

```go
modalTutorial: -1,
tabs:          [14]int{-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1},
```

(Slot count `14` from TS Player.ts:346 `new Array(14).fill(-1)`. The
14-element array literal is verbose but clearer than a `for` loop in
the constructor; matches the goscape convention of explicit literal
init for fixed-shape sentinels.)

### 4c. `IfSetTab` reshape

`modules/world/player_interface.go:67-72`:

```go
// IfSetTab emits IF_SETTAB (com u16, tab u8). 3-byte payload. Also
// writes p.tabs[tab] = com so IsComponentVisible's tab check sees
// the same set of root-layers the client sees. Mirrors TS
// Player.setTab (Player.ts:2042-2044) which performs the array
// write before writing the wire packet.
func (p *Player) IfSetTab(com, tab int) {
    if tab >= 0 && tab < len(p.tabs) {
        p.tabs[tab] = com
    }
    buf := packet.NewPacket(nil)
    buf.P2(uint16(com))
    buf.P1(uint8(tab))
    p.writeOut(gameserver.OpIfSetTab, buf.Bytes())
}
```

The `tab >= 0 && tab < len(p.tabs)` guard handles malformed callers
defensively. TS doesn't bounds-check (would silently extend the
JavaScript array); Go panics on OOB write. Defensive guard is
goscape-idiomatic and consistent with similar guards in `IfSetTab`
callers (`pkg/script/handlers_interface.go` already wraps tab in
`checkNotNull` at script-handler entry).

### Test list

`modules/world/player_interface_test.go` (new file):

- `TestIfSetTab_WritesTabsState` — `p.IfSetTab(100, 3)` → `p.tabs[3] == 100`
- `TestIfSetTab_OutOfRangeTabSilentlyDropped` — `p.IfSetTab(100, 99)` →
  no panic, `p.tabs` unchanged
- `TestIfSetTab_NegativeTabSilentlyDropped` — `p.IfSetTab(100, -1)` →
  no panic, `p.tabs` unchanged
- `TestNewPlayer_TabsAndModalTutorialDefaults` — new player has
  `modalTutorial == -1` and every `tabs[i] == -1`

---

## Task 5 — `(p *Player) IsComponentVisible` method

### 5a. Method body

`modules/world/player_interface.go` (append after `IfSetTab`):

```go
// IsComponentVisible reports whether the given component's rootLayer
// is currently in any of the player's visible-modal slots. Mirrors TS
// Player.isComponentVisible (Player.ts:2047-2049).
//
// Goscape divergence from TS: TS gates each modal slot via raw
// equality against -1-defaulted fields; goscape uses the modalState
// bitmap (modalStateMain/Chat/Side) because modalMain/Chat/Side
// fields are not initialized to -1 (see player.go:200, 344-441).
// Functionally equivalent: a slot is "active" iff the corresponding
// bit is set, and only then is its component-id read.
//
// The tabs[] check is a linear scan over 14 elements; tabs are
// defaulted to -1 so unmatched entries reject naturally. The
// modalTutorial check is direct equality because the field IS
// initialized to -1 (see Task 4) and write-empty until the
// IF_OPENTUT-equivalent opcode lands (DEVIATION
// NAI-59-D-MODALTUTORIAL-NO-PRODUCER).
func (p *Player) IsComponentVisible(com *objtype.ComponentType) bool {
    if com == nil {
        return false
    }
    if p.modalState&modalStateMain != 0 && com.RootLayer == p.modalMain {
        return true
    }
    if p.modalState&modalStateChat != 0 && com.RootLayer == p.modalChat {
        return true
    }
    if p.modalState&modalStateSide != 0 && com.RootLayer == p.modalSide {
        return true
    }
    for _, t := range p.tabs {
        if t == com.RootLayer {
            return true
        }
    }
    if p.modalTutorial != -1 && com.RootLayer == p.modalTutorial {
        return true
    }
    return false
}
```

### 5b. Test list

`modules/world/player_interface_test.go`:

- `TestIsComponentVisible_NilComponentReturnsFalse`
- `TestIsComponentVisible_MatchesModalMain` — `p.modalState=Main`,
  `p.modalMain=200`, `com.RootLayer=200` → true
- `TestIsComponentVisible_MainBitOffEvenWithMatchingId` — same as
  above but `p.modalState=0` → false (gates the bitmap-not-equality
  divergence)
- `TestIsComponentVisible_MatchesModalChat`
- `TestIsComponentVisible_MatchesModalSide`
- `TestIsComponentVisible_MatchesTab` — `p.tabs[5]=42`,
  `com.RootLayer=42` → true
- `TestIsComponentVisible_MatchesTabAtIndexZero` — covers the linear-
  scan first-element case
- `TestIsComponentVisible_MatchesTabAtIndexThirteen` — last-element case
- `TestIsComponentVisible_TabAllNegOneMisses` — fresh player with all
  tabs at -1, com.RootLayer=10 → false (negative-default-doesnt-match
  zero-rootLayer)
- `TestIsComponentVisible_MatchesModalTutorial` — `p.modalTutorial=99`,
  `com.RootLayer=99` → true
- `TestIsComponentVisible_NoMatchReturnsFalse` — every slot mismatched

---

## Task 6 — `handleIfButton` gates + retire NAI-45-D1/D2

### 6a. Edit `modules/world/handler_interface.go:24-44`

Add a tiny in-package helper (above `handleIfButton`):

```go
// lookupComponent returns the registered component for id, or nil if
// the registry is unloaded or the id is out of range. Mirrors TS
// Component.get (Component.ts:252-254) which reads sparse-array slots
// returning undefined on miss.
func (s *Server) lookupComponent(id int) *objtype.ComponentType {
    if s.componentTypes == nil || id < 0 || id >= len(s.componentTypes.Configs) {
        return nil
    }
    return s.componentTypes.Configs[id]
}
```

Replace `handleIfButton` body:

```go
func (s *Server) handleIfButton(p *Player, payload []byte) error {
    if len(payload) < 2 {
        return nil
    }
    comId := int(uint16(payload[0])<<8 | uint16(payload[1]))

    com := s.lookupComponent(comId)
    if com == nil || com.ButtonType == objtype.ButtonNone {
        // bad client: component is unknown OR not button-typed
        return nil
    }
    if !p.IsComponentVisible(com) {
        // bad client or lag: component not currently visible
        return nil
    }

    p.lastCom = comId

    for _, b := range p.resumeButtons {
        if b == comId {
            if p.activeScript != nil && p.activeScript.Execution == script.PauseButton {
                p.activeScript.Execution = script.Running
                s.resumeOrFinish(p.activeScript, p)
            }
            return nil
        }
    }

    if s.scriptProvider == nil {
        return nil
    }
    sf := s.scriptProvider.GetByTriggerSpecific(script.TriggerIfButton, comId, -1)
    root := s.lookupComponent(com.RootLayer)
    protect := root == nil || !root.Overlay
    s.runScript(sf, p, nil, protect, nil, nil)
    return nil
}
```

Drop the `DEVIATION NAI-45-D1` and `DEVIATION NAI-45-D2` doc-comment
blocks above the function (currently L18-23). Top-of-function summary
remains.

The `s.scriptProvider != nil` guard mirrors the sibling pattern in
`handleNpcWalkTrigger` (post-NAI-51 fixup at commit `23ec2a2`); per
`plan_sibling_site_guard_audit.md`. The pre-NAI-59 `handleIfButton`
already enters `GetByTriggerSpecific` without guarding, but T6 adds
the guard since the surrounding handler structure is being reshaped
(the post-condition check is the right time to introduce the guard).
If pre-flight finds `s.scriptProvider != nil` is invariant in all test
fixtures and production, T6 can drop the new guard — to be decided at
T6 plan-write.

### 6b. `objtype` import

`handler_interface.go` currently imports only `"github.com/zsrv/goscape/pkg/script"`.
T6 adds `"github.com/zsrv/goscape/pkg/objtype"` for `objtype.ButtonNone`.

### 6c. Cross-file audit

Per `enumerate_all_sites.md` and `retire_deviation_grep_all_comments.md`,
re-grep `rg "NAI-45-D[12]" pkg/ modules/ cmd/` at T6 close. Expected
post-T6: zero hits. If any other site surfaces (e.g., test-side
attribution comments), edit it out before the T6 commit lands.

### Test list

`modules/world/handler_interface_test.go` (extend existing file):

- `TestHandleIfButton_NilComponentRejects` — `s.Configs.Component(42)
  → nil`; payload pushes 42 → handler returns without touching `lastCom`
  or firing trigger
- `TestHandleIfButton_NoButtonTypeRejects` — Component with
  `ButtonType=ButtonNone` → reject
- `TestHandleIfButton_NotVisibleRejects` — Component with
  `ButtonType=Button` but rootLayer not in any modal/tab slot → reject
- `TestHandleIfButton_OverlayRootSetsProtectFalse` — Component (id=42)
  with rootLayer=100, root component (id=100) has `Overlay=true`,
  `p.tabs[0]=100` (so visible) → `runScript` called with `protect=false`
- `TestHandleIfButton_NonOverlayRootSetsProtectTrue` — same as above
  but root.Overlay=false → `protect=true`
- `TestHandleIfButton_NilRootSetsProtectTrue` — rootLayer=999 (no
  component registered) but root-component-itself-visible cannot
  happen here, so this branch may need a pre-condition tweak: actually
  the handler reaches `Configs.Component(com.RootLayer)` only AFTER
  `IsComponentVisible(com)` passes. Set up so that `com.RootLayer=999`
  AND `p.tabs[0]=999` (i.e., the rootLayer is "visible" because it
  matches a tab) BUT `Configs.Component(999) == nil` (the root entry
  itself isn't registered — possible in a sparse registry). Defensive
  fall-through to `protect=true`.
- Existing `TestHandleIfButtonSetsLastCom` (handler_interface_test.go:131)
  needs Configs.Component fixture-wiring update — pre-T6 the test
  passed without any registry; post-T6 it must seed
  `mockConfigs.components` with a Button-typed visible component.
- Existing `TestHandleIfButtonResumesPauseButton` (L146) needs same
  fixture extension.
- Existing `TestHandleIfButtonPauseButtonNotInResumeButtons` (L183)
  needs same fixture extension.

### 6d. Pre-flight re-check at T6 dispatch

- Re-grep `rg -nE "NAI-45-D[12]" pkg/ modules/ cmd/` and snapshot the
  current line numbers — expected at `handler_interface.go:18,22` per
  Pre-flight section above. Any drift means controller updates the T6
  task block before dispatch.
- Re-read TS `IfButtonHandler.ts:12-41` line-by-line to confirm gate
  ordering hasn't drifted from the four-step sequence (component-lookup
  → buttonType → isComponentVisible → resumeButtons → trigger script
  with protect).
- Confirm `s.runScript` signature at HEAD — it currently takes
  `(sf *ScriptFile, p *Player, n *Npc, protect bool, intArgs []int, stringArgs []string)`
  per `npc_script.go` (5-th param is `protect`). Verify before the
  T6 plan codifies the call.

---

## Task 7 — Close commit + nai_followups update

After all tests green:

1. **Stale-deviation grep** — `rg "NAI-45-D[12]" pkg/ modules/ cmd/`
   per `retire_deviation_grep_all_comments.md`. Expected zero hits.
   If non-zero, edit out before committing.
2. **Tally update** — verify final deviation count via the active
   deviation list (currently at 19 entering NAI-59); expected
   post-close: 18 (-2 retired + 1 new = -1 net).
3. **Memory updates** — append NAI-59 entry to `nai_followups.md`
   summarising the close (analogous to NAI-58's section). Mark
   NAI-45-D1 + NAI-45-D2 as Resolved with closure pointer to NAI-59.
   Add NAI-59-D-MODALTUTORIAL-NO-PRODUCER as a new active deviation
   under "From NAI-59" with closure pointer to "future tutorial-opcode
   sub-spec".
4. **Close commit message** — include trailer:
   ```
   Closes memory: NAI-45-D1, NAI-45-D2
   ```

---

## Bundle / cadence

Single bundle, 6 implementation tasks + 1 close. Full sub-spec (not
compressed; LOC > 15 by an order of magnitude). NAI-46 / NAI-57 /
NAI-58-shaped, but with T2 carrying ~150 LOC (Component.ts decode is
the largest sibling to date by ~2-3×).

Estimated LOC at close:
- T1: ~80 production (struct + constants + constructor)
- T2: ~180 production (decoder + loader) + ~150 test
- T3: ~10 production (server field + bootstrap wire-in) + ~20 test
- T4: ~30 production (player fields + IfSetTab reshape) + ~40 test
- T5: ~40 production (IsComponentVisible) + ~80 test
- T6: ~55 production (handler reshape + lookupComponent helper + retire deviations) + ~120 test
- T7: trailer-only

Total: ~395 production + ~410 test = ~805 LOC.

Implementation mode: **subagent-driven-development** (per
`execution_mode_default.md`).

Per-task review pattern: T1 (struct), T3 (wiring), T4 (Player state),
T5 (method), T6 (handler) receive full per-task two-stage review;
T2 (decoder, largest-LOC) receives the most stringent line-by-line
TS comparison in spec review. T7 is a memo + close commit, no formal
review.

## Memory entries that apply

- `runescript_cadence.md` — full sub-spec cadence with two-stage review
- `execution_mode_default.md` — subagent-driven-development by default
- `controller_preflight.md` — pre-T1/T3/T4/T5/T6 grep + Read verification
  of plan premises against HEAD
- `enumerate_all_sites.md` — T6 deviation-tag grep + T6 sibling-handler comparison (lookupComponent vs lookupSpotanim-style helpers)
- `retire_deviation_grep_all_comments.md` — T6/T7 NAI-45-D1/D2 sweep
- `dead_api_polish.md` — guards against shipping any helper with zero
  consumers; modalTutorial framing per `protocol_stub_not_completed.md`
- `protocol_stub_not_completed.md` — modalTutorial deviation framing
- `plan_runnable_test_fixtures.md` — codified test fixtures (T1, T2,
  T5, T6) must be mentally traced before dispatch; Component decoder
  fixtures are byte-precise and the largest fixture surface to date
- `audit_full_method_against_ts.md` — T2 plan-stage requirement to
  enumerate every TS L99-230 per-arm field read explicitly
- `spec_ts_source_read.md` — plan-author MUST read TS Component.ts
  L99-230 line-by-line, not by analogy
- `mock_recorder_field_naming_check.md` — T6 fixture wiring; grep
  actual `Server.componentTypes` field shape before referencing in tests
- `close_commit_memory_trailer.md` — T7 close commit carries
  `Closes memory:` trailer
- `true_to_ts_gate.md` — every behavioural change tracked; tally
  bookkept; full ComponentType field set ported even though only
  `RootLayer`, `ComName`, `Overlay`, `ComType`, `ButtonType` are read
  by NAI-59 consumers
- `compressed_cadence.md` — explicitly NOT applied (port size exceeds
  threshold)
- `controller_preflight.md` (re-listed for emphasis) — T6 specifically
  must verify (a) `runScript` signature shape, (b) `s.Configs` non-nil
  invariant in test fixtures, (c) `s.scriptProvider != nil` guard
  around `GetByTriggerSpecific` call per `plan_sibling_site_guard_audit.md`
- `plan_sibling_site_guard_audit.md` — T6 must reproduce the
  `s.scriptProvider != nil` guard pattern from sibling
  `s.scriptProvider.GetByTrigger` sites
