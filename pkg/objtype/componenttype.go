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
