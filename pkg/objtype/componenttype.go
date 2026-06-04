package objtype

import (
	"errors"
	"os"
	"path/filepath"

	io "github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
)

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
// Mirrors Engine-TS/src/cache/config/Component.ts (fields at L277-318).
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
	Trans                int // TS Component.ts:280, new in 244
	OverLayer            int
	ScriptComparator     []uint8
	ScriptOperand        []uint16
	Scripts              [][]uint16
	Scroll               int
	Hide                 bool
	Draggable            bool
	Interactable         bool // TS Component.ts:288; renamed from operable in 244
	Usable               bool
	MarginX              int
	MarginY              int
	InventorySlotOffsetX []int16
	InventorySlotOffsetY []int16
	InventorySlotGraphic []string
	InventoryOptions     []string // TS Component.ts:295; renamed from iop in 244
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
		com.Trans = int(client.G1()) // TS Component.ts:66, new in 244

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
			childCount := int(client.G2()) // TS Component.ts:105, widened g1→g2 in 244
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
			com.Interactable = client.GBool() // TS Component.ts:124, renamed from operable in 244
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
			com.InventoryOptions = make([]string, 5) // TS Component.ts:141, renamed from iop in 244
			for i := range 5 {
				com.InventoryOptions[i] = client.GJStrLF() // TS Component.ts:143
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
			com.Interactable = client.GBool()        // TS Component.ts:208, renamed from operable in 244
			com.InventoryOptions = make([]string, 5) // TS Component.ts:209, renamed from iop in 244
			for i := range 5 {
				com.InventoryOptions[i] = client.GJStrLF() // TS Component.ts:211
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
			// TS Component.ts:243 reads overlay via gbool (g1() === 1), not a
			// non-zero test; GBool() mirrors that (==1). Matches the client-side
			// gbool reads above (e.g. line 300). L34.
			overlay := server.GBool()
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

// ByName returns the ComponentType matching the given debugname, or nil
// if no match exists. Mirrors TS Component.getByName. Uses the
// ConfigNames index built at load time — O(1) on name-indexed configs,
// O(N) only if ConfigNames is unpopulated (test fixtures) or stale.
// Consumed by ::openmain in modules/world/handlers_game.go (NAI-187).
func (c *ComponentTypeConfigs) ByName(name string) *ComponentType {
	if c == nil {
		return nil
	}
	if id, ok := c.ConfigNames[name]; ok {
		if id >= 0 && id < len(c.Configs) {
			return c.Configs[id]
		}
	}
	for _, t := range c.Configs {
		if t != nil && t.DebugName == name {
			return t
		}
	}
	return nil
}
