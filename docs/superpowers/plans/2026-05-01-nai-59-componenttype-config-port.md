# NAI-59 — ComponentType Config Port + IF_BUTTON Activation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port `Engine-TS/src/cache/config/Component.ts` into `pkg/objtype/componenttype.go`; add `Player.tabs[14]` + `Player.modalTutorial` + `IsComponentVisible`; activate the buttonType + IsComponentVisible + protect-from-overlay gates in `handleIfButton`. Closes NAI-45-D1 + NAI-45-D2.

**Architecture:** ComponentType is parsed from a dual-source loader — `client/interface` jagfile + `server/interface.dat` raw packet — into a sparse `[]*ComponentType` registry on `*Server`. Server-side handlers access the registry directly via `s.lookupComponent(id)` (no `Configs` interface extension; isComponentVisible has no script-side caller). Player gains `tabs[14]int` + `modalTutorial int` to mirror the modal-slot set TS `isComponentVisible` reads; `IfSetTab` writes into `tabs[]` before emitting the wire packet. `handleIfButton` adds three gates and derives `protect` from the rootLayer component's `Overlay` flag.

**Tech Stack:** Go 1.26+ (per `go_version.md`; modern Go via `use-modern-go`).

**Spec:** `docs/superpowers/specs/2026-05-01-nai-59-componenttype-config-port-design.md`.

---

## File Structure

| Path | Action | Responsibility |
|---|---|---|
| `pkg/objtype/componenttype.go` | Create | `ComponentType` struct + `comType`/`buttonType`/`ComActionTarget` constants + `NewComponentType` constructor + dual-source `parseComponentTypes` decoder + `LoadComponentTypes` loader + `ComponentTypeConfigs` registry |
| `pkg/objtype/componenttype_test.go` | Create | Per-comType-arm + per-buttonType-arm decode + decodeExtra round-trip + LoadComponentTypes file-IO |
| `modules/world/server.go` | Modify | Add `componentTypes *objtype.ComponentTypeConfigs` field; bootstrap `LoadComponentTypes` |
| `modules/world/player.go` | Modify | Add `tabs [14]int` + `modalTutorial int` fields; init both in `newPlayer` |
| `modules/world/player_interface.go` | Modify | Reshape `IfSetTab` to write `p.tabs[tab]` before wire emit; add `IsComponentVisible` method |
| `modules/world/player_interface_test.go` | Create | `IfSetTab` state-write tests + `IsComponentVisible` per-branch tests + `newPlayer` defaults test |
| `modules/world/handler_interface.go` | Modify | Add `lookupComponent` helper; reshape `handleIfButton` with 3 gates + `protect = !root.Overlay`; retire NAI-45-D1/D2 doc-comments |
| `modules/world/handler_interface_test.go` | Modify | New IF_BUTTON gate tests + fixture-update existing IF_BUTTON tests for the registry seed |
| `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` | Modify | Append NAI-59 close section; mark NAI-45-D1 + NAI-45-D2 Resolved |

---

## Pre-flight controller checklist (run before each implementer dispatch)

| Task | Pre-flight grep / Read |
|---|---|
| **Before T1** | `cat pkg/objtype/spotanimtype.go \| head -40` to confirm sibling-port template; `cat pkg/objtype/configtype.go` to confirm `ConfigType{ID, DebugName}` base shape |
| **Before T2** | `grep -nE "func \(p \*Packet\) (G1\|G2\|G2S\|G4\|GBool\|GJStrLF\|Len)" pkg/io/packet/packet.go pkg/io/packet/buffer.go` — confirm reader API surface (signed: `G2S` only; no `G4S` — colour fields use `int32(p.G4())`); confirm `Len()` (NOT `Available()`) is the read-loop terminator per `packet_rw_pointer_gotcha.md` |
| **Before T2** | `grep -n "func LoadJagfile\|func (.*Jagfile) Read" pkg/io/jagfile/jagfile.go` — confirm `LoadJagfile(path) (*Jagfile, error)` and `Read(name) (*packet.Packet, error)` shapes |
| **Before T2** | `ls data/pack/server/interface.dat data/pack/client/interface 2>&1` — confirm cache files staged or that the loader gracefully handles their absence |
| **Before T3** | `grep -nE "(idkTypes\|seqTypes\|spotanimTypes)" modules/world/server.go \| head -10` — confirm field-block ordering at HEAD |
| **Before T4** | `grep -nE "modalMain.*int\|tabs\b\|modalTutorial\b" modules/world/player.go` — confirm tabs+modalTutorial absent; confirm modal field block at L200 |
| **Before T4** | Re-read `modules/world/player.go:344-431` `newPlayer` constructor — confirm field-init style (struct literal with named fields) |
| **Before T5** | `grep -nE "modalState(Main\|Chat\|Side)" modules/world/player.go` — confirm bitmap constants at L35-37 |
| **Before T6** | `rg -nE "NAI-45-D[12]" pkg/ modules/ cmd/` — snapshot exact line numbers (expected `handler_interface.go:18,22`) |
| **Before T6** | `grep -nE "s\.scriptProvider\." modules/world/handler_*.go \| head -20` — survey sibling sites; check whether they `s.scriptProvider != nil` guard before calling per `plan_sibling_site_guard_audit.md` |
| **Before T6** | `grep -nE "func \(s \*Server\) runScript" modules/world/*.go` — confirm `runScript` 6-arg signature |
| **Before T7** | `rg -nE "NAI-45-D[12]" pkg/ modules/ cmd/` — expect zero hits |

---

## Task 1: ComponentType struct + constants + constructor

**Files:**
- Create: `pkg/objtype/componenttype.go`
- Create: `pkg/objtype/componenttype_test.go`

**Test list:**
- `TestNewComponentTypeDefaults`

- [ ] **Step 1: Write the failing test**

```go
// pkg/objtype/componenttype_test.go
package objtype

import "testing"

func TestNewComponentTypeDefaults(t *testing.T) {
	c := NewComponentType(7)
	if c.ID != 7 {
		t.Errorf("ID: got %d, want 7", c.ID)
	}
	if c.RootLayer != -1 {
		t.Errorf("RootLayer: got %d, want -1", c.RootLayer)
	}
	if c.ComType != -1 {
		t.Errorf("ComType: got %d, want -1", c.ComType)
	}
	if c.ButtonType != -1 {
		t.Errorf("ButtonType: got %d, want -1", c.ButtonType)
	}
	if c.OverLayer != -1 {
		t.Errorf("OverLayer: got %d, want -1", c.OverLayer)
	}
	if c.Model != -1 {
		t.Errorf("Model: got %d, want -1", c.Model)
	}
	if c.ActiveModel != -1 {
		t.Errorf("ActiveModel: got %d, want -1", c.ActiveModel)
	}
	if c.Anim != -1 {
		t.Errorf("Anim: got %d, want -1", c.Anim)
	}
	if c.ActiveAnim != -1 {
		t.Errorf("ActiveAnim: got %d, want -1", c.ActiveAnim)
	}
	if c.ActionTarget != -1 {
		t.Errorf("ActionTarget: got %d, want -1", c.ActionTarget)
	}
	if c.ComName != "" {
		t.Errorf("ComName: got %q, want empty", c.ComName)
	}
	if c.Overlay {
		t.Errorf("Overlay: got true, want false")
	}
	if len(c.ScriptComparator) != 0 {
		t.Errorf("ScriptComparator: got %d, want 0", len(c.ScriptComparator))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run TestNewComponentTypeDefaults -v`
Expected: FAIL — `undefined: NewComponentType` and `undefined: ComponentType`.

- [ ] **Step 3: Create `pkg/objtype/componenttype.go` with struct + constants + constructor**

```go
// pkg/objtype/componenttype.go
package objtype

// ComType discriminator values per Engine-TS/src/cache/config/Component.ts:7-14.
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

// Button discriminator values per Engine-TS/src/cache/config/Component.ts:16-22.
const (
	ButtonNone   = 0
	Button       = 1
	ButtonTarget = 2
	ButtonClose  = 3
	ButtonToggle = 4
	ButtonSelect = 5
	ButtonPause  = 6
)

// ComActionTarget bitmask per Engine-TS/src/cache/config/Component.ts:321-327.
// Currently no goscape consumer reads these; ported for true_to_ts_gate parity.
const (
	ComActionTargetObj    = 1
	ComActionTargetNpc    = 2
	ComActionTargetLoc    = 4
	ComActionTargetPlayer = 8
	ComActionTargetHeld   = 16
)

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
	InventorySlotOffsetX []int16
	InventorySlotOffsetY []int16
	InventorySlotGraphic []string
	Iop                  []string
	Fill                 bool
	Center               bool
	Font                 int
	Shadowed             bool
	Text                 string
	ActiveText           string
	Colour               int32
	ActiveColour         int32
	OverColour           int32
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
	ChildX               []int16
	ChildY               []int16
}

// NewComponentType returns a ComponentType with TS-faithful defaults.
// TS defaults at Component.ts:270-318: rootLayer=-1, comType=-1,
// buttonType=-1, overLayer=-1, model=-1, activeModel=-1, anim=-1,
// activeAnim=-1, actionTarget=-1.
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

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run TestNewComponentTypeDefaults -v`
Expected: PASS.

- [ ] **Step 5: Run full pkg/objtype test suite to confirm no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add pkg/objtype/componenttype.go pkg/objtype/componenttype_test.go
git commit --no-gpg-sign -m "feat(objtype): NAI-59 T1 — ComponentType struct + constants"
```

---

## Task 2: ComponentType decoder + loader

**Files:**
- Modify: `pkg/objtype/componenttype.go` (append parseComponentTypes + LoadComponentTypes + ComponentTypeConfigs)
- Modify: `pkg/objtype/componenttype_test.go` (append decoder tests)

**Test list (in dispatch order):**
- `TestComponentDecode_HeaderFields` — id, comparator-zero, scripts-zero, comType=0 minimal record
- `TestComponentDecode_RootLayerSentinel` — id=65535 marker reads next G2 as rootLayer + the real id
- `TestComponentDecode_OverLayerZero` → OverLayer = -1
- `TestComponentDecode_OverLayerNonZero` → OverLayer = ((b-1)<<8) + next
- `TestComponentDecode_ScriptComparator` — comparatorCount=3 fixture
- `TestComponentDecode_ScriptsArray` — scriptCount=2, opcodeCount=4 each
- `TestComponentDecode_TypeLayer` — scroll/hide/childCount=2
- `TestComponentDecode_TypeUnused` — 10-byte skip
- `TestComponentDecode_TypeInventory` — draggable/operable/usable/marginX/Y/20-slot loop (sparse) + iop[5] + actionVerb/action/actionTarget
- `TestComponentDecode_TypeRect` — fill + 3 colours
- `TestComponentDecode_TypeText` — center/font/shadowed/text/activeText + 3 colours
- `TestComponentDecode_TypeSprite` — graphic/activeGraphic
- `TestComponentDecode_TypeModel` — model/activeModel packed-byte expansion + anim/activeAnim 0→-1 + zoom/xan/yan
- `TestComponentDecode_TypeInventoryText` — center/font/shadowed/colour/marginX/Y/operable/iop[5]
- `TestComponentDecode_ButtonNoneNoExtra`
- `TestComponentDecode_ButtonTarget` — actionVerb/action/actionTarget
- `TestComponentDecode_Button_ToggleSelectPause_Option` — table-driven over 4 button types
- `TestParseComponentTypes_DecodeExtraSetsOverlayAndComName`
- `TestParseComponentTypes_DecodeExtraOnUnknownIdSilentlyDiscarded`
- `TestLoadComponentTypes_MissingClientJagfileReturnsEmpty`
- `TestLoadComponentTypes_NoDataEntryInJagfileReturnsEmpty`
- `TestLoadComponentTypes_MissingServerInterfaceDatStillDecodes`

The full per-arm test code is verbose; the steps below show the production code first (because the decoder is monolithic), then the test stubs use a shared `decodeOneComponent` helper at the test-only level.

- [ ] **Step 1: Write a small failing test that proves the decoder entry-point exists**

```go
// pkg/objtype/componenttype_test.go (append)

import "github.com/zsrv/goscape/pkg/io/packet"

// minimalComponentRecord builds a single-record client.interface payload
// with the supplied comType/buttonType and no comparator/scripts/extra
// fields beyond what the type-switches read. Returns a packet ready for
// parseComponentTypes' client arg.
func minimalComponentRecord(t *testing.T, id int, comType, buttonType uint8, typeBody, buttonBody []byte) *packet.Packet {
	t.Helper()
	p := packet.NewPacket(nil)
	p.P2(0) // count header (advisory)
	p.P2(uint16(id))
	p.P1(comType)
	p.P1(buttonType)
	p.P2(0) // clientCode
	p.P2(0) // width
	p.P2(0) // height
	p.P1(0) // overLayer = 0 → -1, no follow-up byte
	p.P1(0) // comparatorCount = 0
	p.P1(0) // scriptCount = 0
	p.Data = append(p.Data, typeBody...)
	p.Data = append(p.Data, buttonBody...)
	return p
}

func TestComponentDecode_HeaderFields(t *testing.T) {
	client := minimalComponentRecord(t, 10, ComTypeLayer, ButtonNone,
		[]byte{
			0, 0, // scroll = 0 (g2)
			0,    // hide = false (gbool / g1)
			0,    // childCount = 0
		},
		nil,
	)
	cfg, err := parseComponentTypes(client, nil)
	if err != nil {
		t.Fatalf("parseComponentTypes: %v", err)
	}
	if len(cfg.Configs) <= 10 || cfg.Configs[10] == nil {
		t.Fatalf("Configs[10]: missing")
	}
	c := cfg.Configs[10]
	if c.ID != 10 {
		t.Errorf("ID: got %d, want 10", c.ID)
	}
	if c.ComType != ComTypeLayer {
		t.Errorf("ComType: got %d, want %d", c.ComType, ComTypeLayer)
	}
	if c.ButtonType != ButtonNone {
		t.Errorf("ButtonType: got %d, want %d", c.ButtonType, ButtonNone)
	}
	if c.OverLayer != -1 {
		t.Errorf("OverLayer: got %d, want -1", c.OverLayer)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run TestComponentDecode_HeaderFields -v`
Expected: FAIL — `undefined: parseComponentTypes` and `undefined: ComponentTypeConfigs`.

- [ ] **Step 3: Append the decoder + loader to `pkg/objtype/componenttype.go`**

Append to the existing `pkg/objtype/componenttype.go` (add imports as needed):

```go
import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	io "github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
)

// ComponentTypeConfigs is the parsed registry of all component records.
type ComponentTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*ComponentType
}

// LoadComponentTypes reads the dual-source Component config:
//   - dir/client/interface (jagfile, "data" entry)
//   - dir/server/interface.dat (raw packet; debugname + overlay)
//
// Mirrors TS Component.load (Engine-TS/src/cache/config/Component.ts:27-41).
// Silent-on-missing for the client jagfile (returns empty registry, nil err).
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
		return &ComponentTypeConfigs{ConfigNames: map[string]int{}}, nil
	}

	server, err := packet.Load(filepath.Join(dir, "server", "interface.dat"), false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return parseComponentTypes(clientData, nil)
		}
		return nil, err
	}
	return parseComponentTypes(clientData, server)
}

// parseComponentTypes decodes the dual-source body. client is the jagfile
// "data" entry (per-id record stream); server is the server interface.dat
// (debugname + overlay extension). server may be nil.
//
// Mirrors TS Component.decode (Component.ts:43-234) + decodeExtra (L237-250).
func parseComponentTypes(client *packet.Packet, server *packet.Packet) (*ComponentTypeConfigs, error) {
	var configs []*ComponentType
	configNames := make(map[string]int)

	client.G2() // count header (advisory; TS reads then ignores)

	rootLayer := -1
	for client.Len() > 0 {
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
			com.Scroll = int(client.G2())
			com.Hide = client.GBool()
			childCount := int(client.G1())
			com.ChildId = make([]uint16, childCount)
			com.ChildX = make([]int16, childCount)
			com.ChildY = make([]int16, childCount)
			for i := range childCount {
				com.ChildId[i] = client.G2()
				com.ChildX[i] = client.G2S()
				com.ChildY[i] = client.G2S()
			}
		case ComTypeUnused:
			// TS L116-120: client reads 10 bytes "seems unused though".
			client.Pos += 10
		case ComTypeInventory:
			com.Draggable = client.GBool()
			com.Operable = client.GBool()
			com.Usable = client.GBool()
			com.MarginX = int(client.G1())
			com.MarginY = int(client.G1())
			com.InventorySlotOffsetX = make([]int16, 20)
			com.InventorySlotOffsetY = make([]int16, 20)
			com.InventorySlotGraphic = make([]string, 20)
			for i := range 20 {
				if client.GBool() {
					com.InventorySlotOffsetX[i] = client.G2S()
					com.InventorySlotOffsetY[i] = client.G2S()
					com.InventorySlotGraphic[i] = client.GJStrLF()
				}
			}
			com.Iop = make([]string, 5)
			for i := range 5 {
				com.Iop[i] = client.GJStrLF()
			}
			com.ActionVerb = client.GJStrLF()
			com.Action = client.GJStrLF()
			com.ActionTarget = int(client.G2())
		case ComTypeRect:
			com.Fill = client.GBool()
			com.Colour = int32(client.G4())
			com.ActiveColour = int32(client.G4())
			com.OverColour = int32(client.G4())
		case ComTypeText:
			com.Center = client.GBool()
			com.Font = int(client.G1())
			com.Shadowed = client.GBool()
			com.Text = client.GJStrLF()
			com.ActiveText = client.GJStrLF()
			com.Colour = int32(client.G4())
			com.ActiveColour = int32(client.G4())
			com.OverColour = int32(client.G4())
		case ComTypeSprite:
			com.Graphic = client.GJStrLF()
			com.ActiveGraphic = client.GJStrLF()
		case ComTypeModel:
			modelHi := int(client.G1())
			if modelHi != 0 {
				com.Model = ((modelHi - 1) << 8) + int(client.G1())
			} else {
				com.Model = 0 // TS: stays 0, not -1
			}
			activeModelHi := int(client.G1())
			if activeModelHi != 0 {
				com.ActiveModel = ((activeModelHi - 1) << 8) + int(client.G1())
			} else {
				com.ActiveModel = 0 // TS: stays 0, not -1
			}
			animHi := int(client.G1())
			if animHi == 0 {
				com.Anim = -1
			} else {
				com.Anim = ((animHi - 1) << 8) + int(client.G1())
			}
			activeAnimHi := int(client.G1())
			if activeAnimHi == 0 {
				com.ActiveAnim = -1
			} else {
				com.ActiveAnim = ((activeAnimHi - 1) << 8) + int(client.G1())
			}
			com.Zoom = int(client.G2())
			com.Xan = int(client.G2())
			com.Yan = int(client.G2())
		case ComTypeInventoryText:
			com.Center = client.GBool()
			com.Font = int(client.G1())
			com.Shadowed = client.GBool()
			com.Colour = int32(client.G4())
			com.MarginX = int(client.G2S())
			com.MarginY = int(client.G2S())
			com.Operable = client.GBool()
			com.Iop = make([]string, 5)
			for i := range 5 {
				com.Iop[i] = client.GJStrLF()
			}
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

		if id >= len(configs) {
			grown := make([]*ComponentType, id+1)
			copy(grown, configs)
			configs = grown
		}
		configs[id] = com
	}

	if server != nil {
		server.G2() // count header (advisory)
		for server.Len() > 0 {
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

	_ = fmt.Sprintf // silence unused-import warning if no errors path uses fmt
	return &ComponentTypeConfigs{ConfigNames: configNames, Configs: configs}, nil
}
```

The `_ = fmt.Sprintf` is a placeholder — drop the `fmt` import if no
error-formatting paths remain. Implementer trims as needed before commit.

- [ ] **Step 4: Run header test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run TestComponentDecode_HeaderFields -v`
Expected: PASS.

- [ ] **Step 5: Add the remaining decoder tests** (per "Test list" above)

Each test follows the same pattern: build a `minimalComponentRecord` with a tailored typeBody/buttonBody, call `parseComponentTypes`, assert the decoded fields. The implementer enumerates one assertion per TS source line for that arm. Examples:

```go
func TestComponentDecode_RootLayerSentinel(t *testing.T) {
	p := packet.NewPacket(nil)
	p.P2(0)        // count header
	p.P2(65535)    // sentinel
	p.P2(99)       // rootLayer
	p.P2(10)       // real id
	p.P1(ComTypeLayer)
	p.P1(ButtonNone)
	p.P2(0); p.P2(0); p.P2(0) // clientCode/width/height
	p.P1(0)        // overLayer
	p.P1(0)        // comparatorCount
	p.P1(0)        // scriptCount
	// TYPE_LAYER body: scroll/hide/childCount=0
	p.P2(0); p.P1(0); p.P1(0)

	cfg, err := parseComponentTypes(p, nil)
	if err != nil {
		t.Fatalf("parseComponentTypes: %v", err)
	}
	if cfg.Configs[10].RootLayer != 99 {
		t.Errorf("RootLayer: got %d, want 99", cfg.Configs[10].RootLayer)
	}
}

func TestComponentDecode_TypeModel(t *testing.T) {
	// Tests the packed-byte expansion: model = ((hi-1)<<8) + lo when hi != 0.
	// Use model=257 → hi=2, lo=1 (2-1)<<8 + 1 = 257.
	// activeModel=0: hi=0, no follow-up byte, ActiveModel stays 0.
	// anim=-1: hi=0, ActiveAnim becomes -1.
	// activeAnim=257: hi=2, lo=1.
	body := []byte{
		2, 1,           // model: hi=2, lo=1 → 257
		0,              // activeModel: hi=0 → 0
		0,              // anim: hi=0 → -1
		2, 1,           // activeAnim: hi=2, lo=1 → 257
		0, 100,         // zoom = 100
		0, 50,          // xan = 50
		0, 25,          // yan = 25
	}
	client := minimalComponentRecord(t, 5, ComTypeModel, ButtonNone, body, nil)
	cfg, err := parseComponentTypes(client, nil)
	if err != nil {
		t.Fatalf("parseComponentTypes: %v", err)
	}
	c := cfg.Configs[5]
	if c.Model != 257 {
		t.Errorf("Model: got %d, want 257", c.Model)
	}
	if c.ActiveModel != 0 {
		t.Errorf("ActiveModel: got %d, want 0", c.ActiveModel)
	}
	if c.Anim != -1 {
		t.Errorf("Anim: got %d, want -1", c.Anim)
	}
	if c.ActiveAnim != 257 {
		t.Errorf("ActiveAnim: got %d, want 257", c.ActiveAnim)
	}
	if c.Zoom != 100 {
		t.Errorf("Zoom: got %d, want 100", c.Zoom)
	}
	if c.Xan != 50 {
		t.Errorf("Xan: got %d, want 50", c.Xan)
	}
	if c.Yan != 25 {
		t.Errorf("Yan: got %d, want 25", c.Yan)
	}
}

func TestComponentDecode_ButtonTarget(t *testing.T) {
	// Layer body (empty children) + button-target trailing fields.
	typeBody := []byte{0, 0, 0, 0} // scroll=0, hide=false, childCount=0
	// ButtonTarget body: "att" + 0x0a + "Attack" + 0x0a + p2(99) → actionTarget=99
	buttonBody := []byte{
		'a', 't', 't', 0x0a,
		'A', 't', 't', 'a', 'c', 'k', 0x0a,
		0, 99,
	}
	client := packet.NewPacket(nil)
	client.P2(0)
	client.P2(7)
	client.P1(ComTypeLayer)
	client.P1(ButtonTarget)
	client.P2(0); client.P2(0); client.P2(0)
	client.P1(0); client.P1(0); client.P1(0)
	client.Data = append(client.Data, typeBody...)
	client.Data = append(client.Data, buttonBody...)
	cfg, err := parseComponentTypes(client, nil)
	if err != nil {
		t.Fatalf("parseComponentTypes: %v", err)
	}
	c := cfg.Configs[7]
	if c.ActionVerb != "att" {
		t.Errorf("ActionVerb: got %q, want %q", c.ActionVerb, "att")
	}
	if c.Action != "Attack" {
		t.Errorf("Action: got %q, want %q", c.Action, "Attack")
	}
	if c.ActionTarget != 99 {
		t.Errorf("ActionTarget: got %d, want 99", c.ActionTarget)
	}
}

func TestParseComponentTypes_DecodeExtraSetsOverlayAndComName(t *testing.T) {
	client := minimalComponentRecord(t, 10, ComTypeLayer, ButtonNone,
		[]byte{0, 0, 0, 0}, nil) // scroll/hide/childCount=0

	server := packet.NewPacket(nil)
	server.P2(0)              // count
	server.P2(10)             // id
	server.PJStrLF("foo")     // debugname
	server.P1(1)              // overlay = true

	cfg, err := parseComponentTypes(client, server)
	if err != nil {
		t.Fatalf("parseComponentTypes: %v", err)
	}
	if cfg.Configs[10].ComName != "foo" {
		t.Errorf("ComName: got %q, want %q", cfg.Configs[10].ComName, "foo")
	}
	if !cfg.Configs[10].Overlay {
		t.Errorf("Overlay: got false, want true")
	}
	if cfg.ConfigNames["foo"] != 10 {
		t.Errorf("ConfigNames[foo]: got %d, want 10", cfg.ConfigNames["foo"])
	}
}

func TestLoadComponentTypes_MissingClientJagfileReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadComponentTypes(dir)
	if err != nil {
		t.Fatalf("LoadComponentTypes: %v", err)
	}
	if len(cfg.Configs) != 0 {
		t.Errorf("Configs: got %d entries, want 0", len(cfg.Configs))
	}
}
```

Implementer enumerates the remaining test bodies (TypeLayer, TypeUnused, TypeInventory, TypeRect, TypeText, TypeSprite, TypeInventoryText, ButtonNoneNoExtra, Button_ToggleSelectPause_Option (table-driven), DecodeExtraOnUnknownIdSilentlyDiscarded, NoDataEntryInJagfileReturnsEmpty, MissingServerInterfaceDatStillDecodes) following the same pattern. Each tests **only** the fields its arm reads, comparing assertion values to the byte-pattern fixture.

- [ ] **Step 6: Run all decoder tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/ -run TestComponentDecode -v`
Expected: PASS for every per-arm test.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/objtype/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/objtype/componenttype.go pkg/objtype/componenttype_test.go
git commit --no-gpg-sign -m "feat(objtype): NAI-59 T2 — ComponentType decoder + dual-source loader"
```

---

## Task 3: Server registry wiring

**Files:**
- Modify: `modules/world/server.go` (add field; bootstrap wire-in)

**Test list:**
- (deferred to T6 fixture wiring; T3 ships infrastructure only)

- [ ] **Step 1: Add `componentTypes` field to Server struct**

In `modules/world/server.go`, locate the field block at L87-89 (the
`idkTypes`/`seqTypes`/`spotanimTypes` declarations) and append a new
field on a new line:

```go
componentTypes *objtype.ComponentTypeConfigs
```

- [ ] **Step 2: Wire `LoadComponentTypes` into Server bootstrap**

In `modules/world/server.go`, locate the `spotanimTypes` load block
at approximately L254 (verified at HEAD `0ee2e7a`). Append after it:

```go
componentTypes, err := objtype.LoadComponentTypes(cfg.CachePath)
if err != nil {
	return nil, fmt.Errorf("load component types: %w", err)
}
s.componentTypes = componentTypes
```

- [ ] **Step 3: Verify server compiles + tests still pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/`
Expected: success (no symbol errors).

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count 1`
Expected: PASS for all existing tests (Server bootstrap only changes by adding the new field; all other tests should remain green).

- [ ] **Step 4: Commit**

```bash
git add modules/world/server.go
git commit --no-gpg-sign -m "feat(world): NAI-59 T3 — wire ComponentType registry into Server"
```

---

## Task 4: Player.tabs[14] + Player.modalTutorial + IfSetTab state-write

**Files:**
- Modify: `modules/world/player.go` (add fields; init in `newPlayer`)
- Modify: `modules/world/player_interface.go` (reshape `IfSetTab`)
- Create: `modules/world/player_interface_test.go`

**Test list:**
- `TestNewPlayer_TabsAndModalTutorialDefaults`
- `TestIfSetTab_WritesTabsState`
- `TestIfSetTab_OutOfRangeTabSilentlyDropped`
- `TestIfSetTab_NegativeTabSilentlyDropped`

- [ ] **Step 1: Write the failing tests**

Create `modules/world/player_interface_test.go`:

```go
package world

import "testing"

func TestNewPlayer_TabsAndModalTutorialDefaults(t *testing.T) {
	p := newTestPlayer(1)
	if p.modalTutorial != -1 {
		t.Errorf("modalTutorial: got %d, want -1", p.modalTutorial)
	}
	for i, tab := range p.tabs {
		if tab != -1 {
			t.Errorf("tabs[%d]: got %d, want -1", i, tab)
		}
	}
}

func TestIfSetTab_WritesTabsState(t *testing.T) {
	p := newTestPlayer(1)
	p.IfSetTab(100, 3)
	if p.tabs[3] != 100 {
		t.Errorf("tabs[3]: got %d, want 100", p.tabs[3])
	}
}

func TestIfSetTab_OutOfRangeTabSilentlyDropped(t *testing.T) {
	p := newTestPlayer(1)
	p.IfSetTab(100, 99) // out of range
	for i, tab := range p.tabs {
		if tab != -1 {
			t.Errorf("tabs[%d]: got %d, want -1 (out-of-range tab should not write)", i, tab)
		}
	}
}

func TestIfSetTab_NegativeTabSilentlyDropped(t *testing.T) {
	p := newTestPlayer(1)
	p.IfSetTab(100, -1)
	for i, tab := range p.tabs {
		if tab != -1 {
			t.Errorf("tabs[%d]: got %d, want -1", i, tab)
		}
	}
}
```

If `newTestPlayer` doesn't exist or doesn't return a Player suitable
for these tests, use the helper pattern from `modules/world/handler_interface_test.go`
where it constructs a `*Player` with a `*client` having a discard writer.
Pre-flight grep `grep -nE "func newTestPlayer\b" modules/world/*.go` to
confirm. If the helper requires a server for `writeOut`, set
`p.client = &client{conn: &discardConn{}}` minimally.

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestNewPlayer_TabsAndModalTutorialDefaults|TestIfSetTab_" -v`
Expected: FAIL — `p.modalTutorial undefined`, `p.tabs undefined`.

- [ ] **Step 3: Add fields to Player struct**

In `modules/world/player.go`, locate L200:

```go
modalMain, modalChat, modalSide                    int
```

Replace with:

```go
modalMain, modalChat, modalSide, modalTutorial     int
tabs                                               [14]int
```

(Verify column-alignment of the field block; if other adjacent declarations use a different alignment, match the file's existing style.)

- [ ] **Step 4: Initialize fields in `newPlayer`**

In `modules/world/player.go`, locate `func newPlayer(c *client) *Player`
at L344. Inside the struct literal, before the closing `}` at L431, add:

```go
modalTutorial: -1,
tabs:          [14]int{-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1},
```

- [ ] **Step 5: Reshape `IfSetTab` to write state before wire emit**

In `modules/world/player_interface.go`, locate `IfSetTab` at L67-72.
Replace with:

```go
// IfSetTab emits IF_SETTAB (com u16, tab u8). 3-byte payload. Also
// writes p.tabs[tab] = com so IsComponentVisible's tab check sees the
// same set of root-layers the client sees. Mirrors TS Player.setTab
// (Player.ts:2042-2044) which performs the array write before writing
// the wire packet.
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

- [ ] **Step 6: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestNewPlayer_TabsAndModalTutorialDefaults|TestIfSetTab_" -v`
Expected: PASS.

- [ ] **Step 7: Run full modules/world test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count 1`
Expected: PASS for all existing tests. Any new failure indicates the field-block edit or `newPlayer` init created a regression.

- [ ] **Step 8: Commit**

```bash
git add modules/world/player.go modules/world/player_interface.go modules/world/player_interface_test.go
git commit --no-gpg-sign -m "feat(world): NAI-59 T4 — Player.tabs[14] + modalTutorial + IfSetTab state-write"
```

---

## Task 5: `(p *Player) IsComponentVisible` method

**Files:**
- Modify: `modules/world/player_interface.go` (append method)
- Modify: `modules/world/player_interface_test.go` (append branch tests)

**Test list:**
- `TestIsComponentVisible_NilComponentReturnsFalse`
- `TestIsComponentVisible_MatchesModalMain`
- `TestIsComponentVisible_MainBitOffEvenWithMatchingId`
- `TestIsComponentVisible_MatchesModalChat`
- `TestIsComponentVisible_MatchesModalSide`
- `TestIsComponentVisible_MatchesTab`
- `TestIsComponentVisible_MatchesTabAtIndexZero`
- `TestIsComponentVisible_MatchesTabAtIndexThirteen`
- `TestIsComponentVisible_TabAllNegOneMisses`
- `TestIsComponentVisible_MatchesModalTutorial`
- `TestIsComponentVisible_NoMatchReturnsFalse`

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/player_interface_test.go`:

```go
import (
	"github.com/zsrv/goscape/pkg/objtype"
)

func TestIsComponentVisible_NilComponentReturnsFalse(t *testing.T) {
	p := newTestPlayer(1)
	if p.IsComponentVisible(nil) {
		t.Errorf("IsComponentVisible(nil): got true, want false")
	}
}

func TestIsComponentVisible_MatchesModalMain(t *testing.T) {
	p := newTestPlayer(1)
	p.modalState = modalStateMain
	p.modalMain = 200
	com := &objtype.ComponentType{RootLayer: 200, ButtonType: objtype.Button}
	if !p.IsComponentVisible(com) {
		t.Errorf("IsComponentVisible: got false, want true (modalMain=200, RootLayer=200)")
	}
}

func TestIsComponentVisible_MainBitOffEvenWithMatchingId(t *testing.T) {
	p := newTestPlayer(1)
	p.modalState = 0 // Main bit off
	p.modalMain = 200
	com := &objtype.ComponentType{RootLayer: 200}
	if p.IsComponentVisible(com) {
		t.Errorf("IsComponentVisible: got true, want false (modalState=0)")
	}
}

func TestIsComponentVisible_MatchesModalChat(t *testing.T) {
	p := newTestPlayer(1)
	p.modalState = modalStateChat
	p.modalChat = 300
	com := &objtype.ComponentType{RootLayer: 300}
	if !p.IsComponentVisible(com) {
		t.Errorf("IsComponentVisible: got false, want true")
	}
}

func TestIsComponentVisible_MatchesModalSide(t *testing.T) {
	p := newTestPlayer(1)
	p.modalState = modalStateSide
	p.modalSide = 400
	com := &objtype.ComponentType{RootLayer: 400}
	if !p.IsComponentVisible(com) {
		t.Errorf("IsComponentVisible: got false, want true")
	}
}

func TestIsComponentVisible_MatchesTab(t *testing.T) {
	p := newTestPlayer(1)
	p.tabs[5] = 42
	com := &objtype.ComponentType{RootLayer: 42}
	if !p.IsComponentVisible(com) {
		t.Errorf("IsComponentVisible: got false, want true (tabs[5]=42)")
	}
}

func TestIsComponentVisible_MatchesTabAtIndexZero(t *testing.T) {
	p := newTestPlayer(1)
	p.tabs[0] = 99
	com := &objtype.ComponentType{RootLayer: 99}
	if !p.IsComponentVisible(com) {
		t.Errorf("IsComponentVisible: got false, want true (tabs[0]=99)")
	}
}

func TestIsComponentVisible_MatchesTabAtIndexThirteen(t *testing.T) {
	p := newTestPlayer(1)
	p.tabs[13] = 77
	com := &objtype.ComponentType{RootLayer: 77}
	if !p.IsComponentVisible(com) {
		t.Errorf("IsComponentVisible: got false, want true (tabs[13]=77)")
	}
}

func TestIsComponentVisible_TabAllNegOneMisses(t *testing.T) {
	p := newTestPlayer(1)
	// All tabs default to -1; com.RootLayer=10 should not match.
	com := &objtype.ComponentType{RootLayer: 10}
	if p.IsComponentVisible(com) {
		t.Errorf("IsComponentVisible: got true, want false (all tabs default -1)")
	}
}

func TestIsComponentVisible_MatchesModalTutorial(t *testing.T) {
	p := newTestPlayer(1)
	p.modalTutorial = 99
	com := &objtype.ComponentType{RootLayer: 99}
	if !p.IsComponentVisible(com) {
		t.Errorf("IsComponentVisible: got false, want true (modalTutorial=99)")
	}
}

func TestIsComponentVisible_NoMatchReturnsFalse(t *testing.T) {
	p := newTestPlayer(1)
	p.modalState = modalStateMain | modalStateChat | modalStateSide
	p.modalMain = 1
	p.modalChat = 2
	p.modalSide = 3
	p.modalTutorial = 4
	for i := range p.tabs {
		p.tabs[i] = 100 + i
	}
	com := &objtype.ComponentType{RootLayer: 999}
	if p.IsComponentVisible(com) {
		t.Errorf("IsComponentVisible: got true, want false (no slot matches)")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestIsComponentVisible -v`
Expected: FAIL — `p.IsComponentVisible undefined`.

- [ ] **Step 3: Append `IsComponentVisible` to `modules/world/player_interface.go`**

Add `"github.com/zsrv/goscape/pkg/objtype"` to the import block, then
append after the existing `IfSetTab` (or anywhere in the file):

```go
// IsComponentVisible reports whether the given component's rootLayer
// is currently in any of the player's visible-modal slots. Mirrors TS
// Player.isComponentVisible (Player.ts:2047-2049).
//
// Goscape divergence from TS: TS gates each modal slot via raw
// equality against -1-defaulted fields; goscape uses the modalState
// bitmap (modalStateMain/Chat/Side) because modalMain/Chat/Side
// fields are not initialized to -1 (zero-valued by Go default).
// Functionally equivalent: a slot is "active" iff the corresponding
// bit is set, and only then is its component-id read.
//
// modalTutorial IS initialized to -1 (see newPlayer); the !=-1 guard
// is direct because the field is write-empty until the IF_OPENTUT-
// equivalent opcode lands (DEVIATION NAI-59-D-MODALTUTORIAL-NO-PRODUCER).
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

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestIsComponentVisible -v`
Expected: PASS for all 11 sub-tests.

- [ ] **Step 5: Run full modules/world test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count 1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add modules/world/player_interface.go modules/world/player_interface_test.go
git commit --no-gpg-sign -m "feat(world): NAI-59 T5 — Player.IsComponentVisible method"
```

---

## Task 6: `handleIfButton` gates + retire NAI-45-D1/D2

**Files:**
- Modify: `modules/world/handler_interface.go` (add `lookupComponent` helper; reshape `handleIfButton`; drop NAI-45-D1/D2 doc-comments)
- Modify: `modules/world/handler_interface_test.go` (new tests + fixture-update existing IF_BUTTON tests)

**Test list:**
- `TestHandleIfButton_NilComponentRejects`
- `TestHandleIfButton_NoButtonTypeRejects`
- `TestHandleIfButton_NotVisibleRejects`
- `TestHandleIfButton_OverlayRootSetsProtectFalse`
- `TestHandleIfButton_NonOverlayRootSetsProtectTrue`
- `TestHandleIfButton_NilRootSetsProtectTrue`
- Update existing `TestHandleIfButtonSetsLastCom` (handler_interface_test.go:131)
- Update existing `TestHandleIfButtonResumesPauseButton` (handler_interface_test.go:146)
- Update existing `TestHandleIfButtonPauseButtonNotInResumeButtons` (handler_interface_test.go:183)

- [ ] **Step 1: Pre-flight grep for NAI-45-D1/D2 sites**

Run: `rg -nE "NAI-45-D[12]" pkg/ modules/ cmd/`
Expected at HEAD `0ee2e7a`: exactly 2 hits — `handler_interface.go:18` and `:22`. If any other site surfaces, note it for T6 retirement; if line numbers drift, update the Step 5 edit accordingly.

- [ ] **Step 2: Write failing tests for the three new gates + protect derivation**

In `modules/world/handler_interface_test.go`, append new tests. These
require a Server with a `componentTypes` registry seeded with specific
fixtures. Pre-flight grep `grep -nE "func newTestServer\b" modules/world/*.go`
to find the helper; reuse its construction pattern.

```go
// Helper to seed a component registry inline.
func seedComponentTypes(t *testing.T, s *Server, components map[int]*objtype.ComponentType) {
	t.Helper()
	maxId := 0
	for id := range components {
		if id > maxId {
			maxId = id
		}
	}
	configs := make([]*ComponentType, maxId+1)
	for id, c := range components {
		configs[id] = c
	}
	s.componentTypes = &objtype.ComponentTypeConfigs{
		ConfigNames: map[string]int{},
		Configs:     configs,
	}
}

func TestHandleIfButton_NilComponentRejects(t *testing.T) {
	s, p := newTestServerAndPlayer(t)
	// componentTypes left nil (or empty) — Component(42) returns nil
	payload := []byte{0, 42}
	if err := s.handleIfButton(p, payload); err != nil {
		t.Fatalf("handleIfButton: %v", err)
	}
	if p.lastCom != -1 {
		t.Errorf("lastCom: got %d, want -1 (handler should reject before setting lastCom)", p.lastCom)
	}
}

func TestHandleIfButton_NoButtonTypeRejects(t *testing.T) {
	s, p := newTestServerAndPlayer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		42: {RootLayer: 100, ButtonType: objtype.ButtonNone},
	})
	payload := []byte{0, 42}
	if err := s.handleIfButton(p, payload); err != nil {
		t.Fatalf("handleIfButton: %v", err)
	}
	if p.lastCom != -1 {
		t.Errorf("lastCom: got %d, want -1 (ButtonNone should reject)", p.lastCom)
	}
}

func TestHandleIfButton_NotVisibleRejects(t *testing.T) {
	s, p := newTestServerAndPlayer(t)
	// Component visible only if rootLayer matches a modal/tab slot.
	// Component has rootLayer=100; player has no modal open and tabs all -1.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		42: {RootLayer: 100, ButtonType: objtype.Button},
	})
	payload := []byte{0, 42}
	if err := s.handleIfButton(p, payload); err != nil {
		t.Fatalf("handleIfButton: %v", err)
	}
	if p.lastCom != -1 {
		t.Errorf("lastCom: got %d, want -1 (not visible should reject)", p.lastCom)
	}
}

func TestHandleIfButton_OverlayRootSetsProtectFalse(t *testing.T) {
	s, p := newTestServerAndPlayer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		42:  {RootLayer: 100, ButtonType: objtype.Button},
		100: {RootLayer: 100, ButtonType: objtype.ButtonNone, Overlay: true},
	})
	p.tabs[0] = 100 // make rootLayer visible via tab slot
	// Wire a script for trigger IF_BUTTON,42 so we can observe the protect arg.
	registerTestScript(t, s, script.TriggerIfButton, 42, -1)
	payload := []byte{0, 42}
	if err := s.handleIfButton(p, payload); err != nil {
		t.Fatalf("handleIfButton: %v", err)
	}
	// Assert runScript was called with protect=false.
	// (Implementer plumbs a recorder via the existing script provider's
	// test seam; or asserts post-run state that distinguishes protect=true
	// from protect=false. The simplest seam: a test-only TriggerIfButton
	// script whose body sets a flag we can read; but that doesn't show
	// protect. Use a recording mock by overriding s.scriptProvider with
	// one that captures the (sf, protect) tuple.)
	if got := lastRunScriptProtect(t, s); got != false {
		t.Errorf("runScript protect: got %v, want false", got)
	}
}

func TestHandleIfButton_NonOverlayRootSetsProtectTrue(t *testing.T) {
	s, p := newTestServerAndPlayer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		42:  {RootLayer: 100, ButtonType: objtype.Button},
		100: {RootLayer: 100, ButtonType: objtype.ButtonNone, Overlay: false},
	})
	p.tabs[0] = 100
	registerTestScript(t, s, script.TriggerIfButton, 42, -1)
	payload := []byte{0, 42}
	if err := s.handleIfButton(p, payload); err != nil {
		t.Fatalf("handleIfButton: %v", err)
	}
	if got := lastRunScriptProtect(t, s); got != true {
		t.Errorf("runScript protect: got %v, want true", got)
	}
}

func TestHandleIfButton_NilRootSetsProtectTrue(t *testing.T) {
	s, p := newTestServerAndPlayer(t)
	// rootLayer=999 is visible (tab match) but not registered.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		42: {RootLayer: 999, ButtonType: objtype.Button},
		// no entry for id 999
	})
	p.tabs[0] = 999
	registerTestScript(t, s, script.TriggerIfButton, 42, -1)
	payload := []byte{0, 42}
	if err := s.handleIfButton(p, payload); err != nil {
		t.Fatalf("handleIfButton: %v", err)
	}
	if got := lastRunScriptProtect(t, s); got != true {
		t.Errorf("runScript protect: got %v, want true (nil root falls through to protect)", got)
	}
}
```

The `lastRunScriptProtect` and `registerTestScript` helpers reflect
how existing handler_interface_test.go captures runScript invocations.
Pre-flight `grep -nE "registerTest\b|runScript.*recorder\|lastRunScript" modules/world/handler_interface_test.go modules/world/script_test.go`
to find the project's existing test seam pattern; if no seam exists,
the implementer adds a small recording-script-provider for T6.

Update the three existing tests at handler_interface_test.go:131, 146,
183 to seed a visible Button-typed component for `comId` BEFORE calling
`handleIfButton`. Without the seed, the new gates reject and the tests
that assert `lastCom == comId` will fail.

Example for `TestHandleIfButtonSetsLastCom`:

```go
func TestHandleIfButtonSetsLastCom(t *testing.T) {
	s, p := newTestServerAndPlayer(t)
	const comId = 100
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		comId: {RootLayer: comId, ButtonType: objtype.Button},
	})
	p.tabs[0] = comId // make visible
	payload := []byte{byte(comId >> 8), byte(comId & 0xff)}
	if err := s.handleIfButton(p, payload); err != nil {
		t.Fatalf("handleIfButton: %v", err)
	}
	if p.lastCom != comId {
		t.Errorf("lastCom: got %d, want %d", p.lastCom, comId)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleIfButton -v`
Expected: FAIL on the new tests + FAIL on the three updated existing tests (until production code is updated). The failure message should be assertion-mismatch (post-Step 4) not compile-error — verify the test file compiles before proceeding.

- [ ] **Step 4: Add `lookupComponent` helper + reshape `handleIfButton`**

In `modules/world/handler_interface.go`:

1. Add `"github.com/zsrv/goscape/pkg/objtype"` to the import block.
2. Drop the doc-comment block at L18-23 (the `// DEVIATION NAI-45-D1:` and `// DEVIATION NAI-45-D2:` blocks). Keep the top-of-function summary intact.
3. Add `lookupComponent` helper above `handleIfButton`:

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

4. Replace the body of `handleIfButton`:

```go
// handleIfButton handles client opcode 155 (IF_BUTTON).
// Body: u16 component-id.
//
// Gates per TS IfButtonHandler.ts:14-22:
//   - Component must be registered AND have buttonType != NO_BUTTON
//   - Component must be IsComponentVisible to the player
// On pass, sets lastCom and either resumes a PauseButton-suspended
// script or fires [if_button,<comId>]. The trigger fires with
// protect = !root.Overlay (root = rootLayer's component).
func (s *Server) handleIfButton(p *Player, payload []byte) error {
	if len(payload) < 2 {
		return nil
	}
	comId := int(uint16(payload[0])<<8 | uint16(payload[1]))

	com := s.lookupComponent(comId)
	if com == nil || com.ButtonType == objtype.ButtonNone {
		return nil
	}
	if !p.IsComponentVisible(com) {
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

The `s.scriptProvider != nil` guard mirrors the post-NAI-51 fixup
pattern (commit `d5e7c19`); if the controller's Step 1 pre-flight finds
all sibling sites omit this guard at HEAD, the implementer may drop it
to reduce noise. Default: keep it, per `plan_sibling_site_guard_audit.md`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestHandleIfButton -v`
Expected: PASS for all new tests + the three updated existing tests.

- [ ] **Step 6: Verify NAI-45-D1/D2 doc-comments fully retired**

Run: `rg -nE "NAI-45-D[12]" pkg/ modules/ cmd/`
Expected: zero hits. If any site surfaces, edit it out before commit.

- [ ] **Step 7: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count 1`
Expected: PASS across all packages.

- [ ] **Step 8: Commit**

```bash
git add modules/world/handler_interface.go modules/world/handler_interface_test.go
git commit --no-gpg-sign -m "feat(world): NAI-59 T6 — handleIfButton gates close NAI-45-D1 + NAI-45-D2"
```

---

## Task 7: Close commit + nai_followups update

**Files:**
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`

- [ ] **Step 1: Stale-deviation grep**

Run: `rg -nE "NAI-45-D[12]" pkg/ modules/ cmd/`
Expected: zero hits. If non-zero, edit out before committing.

- [ ] **Step 2: Append NAI-59 close section to nai_followups.md**

Append after the NAI-58 section (analogous to NAI-58's closing block):

```markdown
## NAI-59 — CLOSED 2026-05-01

**Scope:** Port `ComponentType` (Engine-TS/.../Component.ts) into
`pkg/objtype`, wire registry into Server, add `Player.tabs[14]` +
`Player.modalTutorial` + `IsComponentVisible`, activate buttonType +
visibility + protect-from-overlay gates in `handleIfButton`.

**Cadence:** Full sub-spec, single bundle, 6 implementation tasks +
1 close. NAI-46 IdkType / NAI-57 SeqType / NAI-58 SpotanimType-shaped
but with T2 carrying ~180 production LOC (Component.ts decode is the
largest sibling sub-spec to date).

**Close commit:** `<HEAD>` (T1: `<sha1>`, T2: `<sha2>`, T3: `<sha3>`,
T4: `<sha4>`, T5: `<sha5>`, T6: `<sha6>`).

**Spec:** `docs/superpowers/specs/2026-05-01-nai-59-componenttype-config-port-design.md`.
**Plan:** `docs/superpowers/plans/2026-05-01-nai-59-componenttype-config-port.md`.

**Follow-ups closed:**
- NAI-45-D1 (IF_BUTTON skipped buttonType + isComponentVisible checks).
- NAI-45-D2 (IF_BUTTON used protect=true unconditionally).

**Deviations opened:**
- **NAI-59-D-MODALTUTORIAL-NO-PRODUCER** — `Player.modalTutorial`
  field exists and is read by `IsComponentVisible`, but no goscape
  opcode handler writes it. TS `Player.openTutorial` (called from a
  script opcode at `Player.ts:2002`) is unported. Read-path consumer
  exists, so this is "stub-not-completed" per
  `protocol_stub_not_completed.md` rather than dead-API. Closure:
  future `IF_OPENTUT` (or equivalent) opcode handler sub-spec. Same
  shape applies to the unported tutorial branch in `(*Player).CloseModal`
  (TS `Player.ts:717-723`) — observed pre-existing gap, NOT introduced
  by NAI-59.

**Deviations closed:** NAI-45-D1, NAI-45-D2.

**Deviation tally:** 19 → 18 (-2 +1).

**Cluster status update:** Component-registry cluster at NAI-59 close —
8 active deviations remain open across 6 handler files (S6o-D1, S6o-D2,
NAI-40-D-COMPONENT-REGISTRY-VALIDATION-SKIPPED ×2, NAI-48-D1 ×2,
NAI-50-D1, NAI-50-D2, NAI-53-D-CLEARCOMLISTENERS-PER-SLOT). Each is
now a straightforward NAI-60+ follow-up: import objtype, call
`s.lookupComponent(id)`, apply the same gate pattern as NAI-59 T6.
The cluster cleanup will likely span 2-3 small sub-specs grouped by
handler family (OpNpc, OpObj, OpPlayer, InvButton, CloseModal).

**Follow-up candidates:**
- **NAI-60 candidate (cluster cleanup):** Activate Component validation
  in OpNpc (S6o-D1, S6o-D2), OpObj (NAI-50-D1, NAI-50-D2), OpPlayer
  (NAI-40-D-COMPONENT-REGISTRY-VALIDATION-SKIPPED ×2), InvButton
  (NAI-48-D1 ×2). Bundle by handler family or single sub-spec depending
  on scope budget.
- **`IF_OPENTUT` (or equivalent) opcode handler** — closes
  NAI-59-D-MODALTUTORIAL-NO-PRODUCER. Conditional on a tutorial-content
  driver materializing.
- **`(*Player).CloseModal` tutorial branch port (TS L717-723)** —
  observed pre-existing gap during NAI-59 brainstorm; pairs with the
  IF_OPENTUT closure.

**Plan-author misses caught by controller pre-flight:**
- (To be filled in by the controller at T7-time after running through
  the full sequence.)

**Memory entries seeded by NAI-59:** (To be filled in if any new
failure modes surface during implementation.)
```

- [ ] **Step 3: Mark NAI-45-D1 + NAI-45-D2 as Resolved in their original `From NAI-45` section**

Locate the "From NAI-45" section in nai_followups.md and append a
"Resolved" note to the NAI-45-D1 and NAI-45-D2 entries pointing to NAI-59.

(Pre-flight grep `grep -n "NAI-45-D1\|NAI-45-D2" /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` to find exact lines.)

- [ ] **Step 4: Run full test suite once more for the close-commit dignity**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count 1`
Expected: PASS.

- [ ] **Step 5: Commit close**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-59 — ComponentType config port; closes NAI-45-D1 + NAI-45-D2

Closes memory: NAI-45-D1, NAI-45-D2
EOF
)"
```

(If memory file is part of the same git tree, `git add` it before
committing; otherwise the memory edits stay outside the goscape repo
and the commit is empty-but-trailered. Pre-flight `git -C /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory status` to determine.)

---

## Self-review checklist (controller, post-T6, pre-T7)

- [ ] All NAI-45-D1 + NAI-45-D2 doc-comment sites retired (`rg "NAI-45-D[12]" pkg/ modules/ cmd/` returns zero hits).
- [ ] All new tests green; existing test suite green.
- [ ] `s.lookupComponent` helper returns nil safely under all inputs (registry-nil / id-negative / id-too-large).
- [ ] `IsComponentVisible` returns false for nil component.
- [ ] `IfSetTab` writes to `tabs[]` only within bounds.
- [ ] No new untracked files outside `pkg/objtype/componenttype*.go` + `modules/world/player_interface_test.go`.
- [ ] Deviation tally bookkept in T7 close (19 → 18, -2 +1).

---

## Memory entries that apply

- `runescript_cadence.md`
- `execution_mode_default.md`
- `controller_preflight.md` (Pre-flight section above)
- `enumerate_all_sites.md` (T6 deviation-tag grep)
- `retire_deviation_grep_all_comments.md` (T6/T7 NAI-45-D1/D2 sweep)
- `dead_api_polish.md` (modalTutorial framing per `protocol_stub_not_completed.md`)
- `protocol_stub_not_completed.md` (modalTutorial deviation type)
- `plan_runnable_test_fixtures.md` (T2 byte-precise fixtures; T6 component-registry seed fixture)
- `audit_full_method_against_ts.md` (T2 plan codifies every TS L99-230 per-arm field read)
- `spec_ts_source_read.md` (plan-author read TS line-by-line)
- `mock_recorder_field_naming_check.md` (T6 fixture wiring; grep `Server.componentTypes` field shape)
- `close_commit_memory_trailer.md` (T7 close commit `Closes memory:` trailer)
- `true_to_ts_gate.md` (full ComponentType field set ported even though only a subset is read)
- `packet_rw_pointer_gotcha.md` (T2 `client.Len()` not `client.Available()` for read-loop terminator)
- `plan_sibling_site_guard_audit.md` (T6 `s.scriptProvider != nil` guard reproduction)
- `compressed_cadence.md` (explicitly NOT applied)
