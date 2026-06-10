// Package clientinterface implements the binary unpack of the RS2 interface
// archive (client/interface jagfile, file "data").
//
// TS source: tools/unpack/interface/Unpack.ts (IfType.unpack, lines 86-288).
package clientinterface

import (
	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
)

// Component type constants — TS Unpack.ts:37-46 const enum ComponentType.
const (
	TypeLayer   = 0
	TypeUnused  = 1
	TypeInv     = 2
	TypeRect    = 3
	TypeText    = 4
	TypeGraphic = 5
	TypeModel   = 6
	TypeInvText = 7
)

// Button type constants — TS Unpack.ts:48-55 const enum ButtonType.
const (
	ButtonOK       = 1
	ButtonTarget   = 2
	ButtonClose    = 3
	ButtonToggle   = 4
	ButtonSelect   = 5
	ButtonContinue = 6
)

// Component mirrors the TS IfType instance fields (Unpack.ts:809-857).
// Zero values match TS class initializers where applicable; see field comments
// for cases where TS initializes to -1 (not Go zero).
//
// Fields read in the export half (Unpack.ts:359-807) but not the decode half
// are kept here so the next task (rename/export) can emit them correctly.
type Component struct {
	// ID and root identification
	ID        int // TS: id (initialized -1, overwritten at parse time)
	RootLayer int // TS: rootLayer (initialized -1)

	// Common header fields — TS lines 108-113
	ComType    int // TS: comType g1 (initialized -1)
	ButtonType int // TS: buttonType g1 (initialized -1)
	ClientCode int // TS: clientCode g2 (initialized 0)
	Width      int // TS: width g2 (initialized 0)
	Height     int // TS: height g2 (initialized 0)
	Trans      int // TS: trans g1 (initialized 0)

	// OverLayer — TS lines 115-120; -1 when wire byte is 0.
	OverLayer int // TS: overLayer (initialized -1)

	// Script conditionals — TS lines 122-131; nil when comparatorCount==0.
	ScriptComparator []uint8  // TS: scriptComparator Uint8Array
	ScriptOperand    []uint16 // TS: scriptOperand Uint16Array

	// Scripts — TS lines 133-146; nil when scriptCount==0.
	// Each element is a []uint16 of opcodeCount g2 words (inner count is g2).
	Script [][]uint16 // TS: script Array of Uint16Array

	// TYPE_LAYER tail — TS lines 148-162
	Scroll  int   // TS: scroll g2 (initialized 0)
	Hide    bool  // TS: hide gbool (initialized false)
	ChildID []int // TS: childId g2[]
	ChildX  []int // TS: childX g2s[] (signed)
	ChildY  []int // TS: childY g2s[] (signed)

	// TYPE_INV tail — TS lines 168-196
	Draggable      bool      // TS: draggable gbool
	Interactable   bool      // TS: interactable gbool
	Usable         bool      // TS: usable gbool
	Swappable      bool      // TS: swappable gbool (Unpack.ts:172)
	MarginX        int       // TS: marginX g1 (or g2s for TYPE_INV_TEXT)
	MarginY        int       // TS: marginY g1 (or g2s for TYPE_INV_TEXT)
	InvSlotOffsetX []int     // TS: invSlotOffsetX Int16Array (20 elements when set)
	InvSlotOffsetY []int     // TS: invSlotOffsetY Int16Array (20 elements when set)
	InvSlotSprite  []*string // TS: invSlotSprite string[] (20 elements; nil slot = not set)
	Iops           []*string // TS: iops (string|null)[] length 5; nil = was empty string

	// TYPE_RECT tail — TS line 199
	Fill bool // TS: fill gbool

	// TYPE_TEXT / TYPE_UNUSED / TYPE_INV_TEXT shared text-family fields
	Center   bool // TS: center gbool (initialized false)
	Font     int  // TS: font g1 (initialized 0 — TS class: `font: number | null = null`)
	Shadowed bool // TS: shadowed gbool (initialized false)

	// TYPE_TEXT strings — TS lines 209-211
	Text       string // TS: text gjstr (initialized null → Go empty string)
	ActiveText string // TS: activeText gjstr (initialized null → Go empty string)

	// Colour fields — TS g4s (signed 32-bit)
	Colour          int // TS: colour g4s (initialized 0)
	ActiveColour    int // TS: activeColour g4s (initialized 0)
	OverColour      int // TS: overColour g4s (initialized 0)
	ActiveOverColour int // TS: activeOverColour g4s (Unpack.ts:221)

	// TYPE_GRAPHIC — TS lines 223-225
	Graphic       string // TS: graphic gjstr (initialized null)
	ActiveGraphic string // TS: activeGraphic gjstr (initialized null)

	// TYPE_MODEL — TS lines 227-255
	// model/activeModel: wire byte 0 → value stays 0 (TS initializes model=-1
	// but overwrites with 0 on the wire-0 branch); non-zero → ((v-1)<<8)+g1.
	// anim/activeAnim: wire byte 0 → -1; non-zero → ((v-1)<<8)+g1.
	Model       int // TS: model (class init -1; wire-0 branch sets to 0)
	ActiveModel int // TS: activeModel (class init null; wire-0 branch sets to 0)
	Anim        int // TS: anim (class init -1; wire-0 branch keeps -1)
	ActiveAnim  int // TS: activeAnim (class init -1; wire-0 branch keeps -1)
	Zoom        int // TS: zoom g2
	Xan         int // TS: xan g2
	Yan         int // TS: yan g2

	// Button target/action tail — TS lines 278-282
	ActionVerb   string // TS: actionVerb gjstr (initialized null)
	Action       string // TS: action gjstr (initialized null)
	ActionTarget int    // TS: actionTarget g2 (initialized -1)

	// Button option — TS lines 284-286
	Option string // TS: option gjstr (initialized null)

	// FontSet tracks whether Font was actually read from the stream
	// (since both 0 and "not read" map to int zero). Used by export.
	FontSet bool
}

// newComponent returns a Component with the TS class field defaults applied.
// TS Unpack.ts:809-857.
func newComponent() *Component {
	return &Component{
		ID:           -1,
		RootLayer:    -1,
		ComType:      -1,
		ButtonType:   -1,
		OverLayer:    -1,
		Anim:         -1,
		ActiveAnim:   -1,
		ActionTarget: -1,
		// Model class-default is -1 per TS:843; ActiveModel class-default is null.
		// After the wire-read they may be set to 0 (wire-byte 0 path) or to a
		// packed value. We start at the TS class defaults.
		Model:       -1,
		ActiveModel: 0, // TS `activeModel: number | null = null` treated as 0 when unset
	}
}

// Decoded is the full result of a Decode call.
type Decoded struct {
	// Components is indexed by component ID; slots for IDs that were never
	// written are nil. TS: IfType.instances[].
	Components []*Component

	// Order records component IDs in the order they appeared in the binary
	// stream. TS: IfType.order[].
	Order []int

	// Count is the component count read from the file header.
	// TS: IfType.count = dat.g2() at line 92.
	// NOTE: len(Components) may exceed Count when stream IDs are >= Count
	// (sparse layout — the slice grows to fit). Always range over Components
	// directly; do not use Count as an upper bound.
	Count int
}

// Decode reads the interface binary from jag's "data" member and returns the
// full decode result.
//
// Mirrors TS IfType.unpack (Unpack.ts:86-288).
func Decode(jag *jagfile.Jagfile) (*Decoded, error) {
	dat, err := jag.Read("data")
	if err != nil {
		return nil, err
	}

	return DecodePacket(dat)
}

// DecodePacket decodes directly from a Packet (useful for tests that build
// synthetic fixtures without a real jagfile).
func DecodePacket(dat *packet.Packet) (*Decoded, error) {
	// TS line 92: IfType.count = dat.g2()
	count := int(dat.G2())

	dec := &Decoded{
		Count:      count,
		Components: make([]*Component, count),
		Order:      make([]int, 0, count),
	}

	layer := -1 // TS line 95: let layer: number = -1

	// TS line 96: while (dat.pos < dat.length)
	for dat.Pos < len(dat.Data) {
		// TS lines 97-101: read id; if 65535 then read new layer + new id
		id := int(dat.G2())
		if id == 65535 {
			layer = int(dat.G2())
			id = int(dat.G2())
		}

		// TS line 103: IfType.order.push(id)
		dec.Order = append(dec.Order, id)

		// TS line 105-107: new IfType; store in instances[id]
		com := newComponent()
		com.ID = id
		com.RootLayer = layer

		// TS lines 108-113: common header
		com.ComType = int(dat.G1())
		com.ButtonType = int(dat.G1())
		com.ClientCode = int(dat.G2())
		com.Width = int(dat.G2())
		com.Height = int(dat.G2())
		com.Trans = int(dat.G1())

		// TS lines 115-120: overLayer encoding
		// Wire byte 0 → -1; non-zero → ((v-1)<<8) + next byte.
		overLayerHi := int(dat.G1())
		if overLayerHi == 0 {
			com.OverLayer = -1
		} else {
			com.OverLayer = ((overLayerHi - 1) << 8) + int(dat.G1())
		}

		// TS lines 122-131: comparator/operand arrays
		comparatorCount := int(dat.G1())
		if comparatorCount > 0 {
			com.ScriptComparator = make([]uint8, comparatorCount)
			com.ScriptOperand = make([]uint16, comparatorCount)
			for i := range comparatorCount {
				com.ScriptComparator[i] = dat.G1()
				com.ScriptOperand[i] = dat.G2()
			}
		}

		// TS lines 133-146: script array
		// Outer count g1 = number of scripts.
		// Per script: opcodeCount g2 THEN opcodeCount × g2 words.
		scriptCount := int(dat.G1())
		if scriptCount > 0 {
			com.Script = make([][]uint16, scriptCount)
			for i := range scriptCount {
				opcodeCount := int(dat.G2())
				words := make([]uint16, opcodeCount)
				for j := range opcodeCount {
					words[j] = dat.G2()
				}
				com.Script[i] = words
			}
		}

		// TS lines 148-162: TYPE_LAYER tail
		if com.ComType == TypeLayer {
			com.Scroll = int(dat.G2())
			com.Hide = dat.GBool()

			childCount := int(dat.G2())
			com.ChildID = make([]int, childCount)
			com.ChildX = make([]int, childCount)
			com.ChildY = make([]int, childCount)
			for i := range childCount {
				com.ChildID[i] = int(dat.G2())
				com.ChildX[i] = int(dat.G2S())
				com.ChildY[i] = int(dat.G2S())
			}
		}

		// TS lines 164-166: TYPE_UNUSED — skip 3 bytes
		if com.ComType == TypeUnused {
			dat.Pos += 3
		}

		// TS lines 168-196: TYPE_INV tail
		if com.ComType == TypeInv {
			com.Draggable = dat.GBool()
			com.Interactable = dat.GBool()
			com.Usable = dat.GBool()
			com.Swappable = dat.GBool() // TS Unpack.ts:172
			com.MarginX = int(dat.G1())
			com.MarginY = int(dat.G1())

			com.InvSlotOffsetX = make([]int, 20)
			com.InvSlotOffsetY = make([]int, 20)
			com.InvSlotSprite = make([]*string, 20)

			for i := range 20 {
				if dat.GBool() {
					x := int(dat.G2S())
					y := int(dat.G2S())
					s := dat.GJStrLF()
					com.InvSlotOffsetX[i] = x
					com.InvSlotOffsetY[i] = y
					com.InvSlotSprite[i] = &s
				}
			}

			com.Iops = make([]*string, 5)
			for i := range 5 {
				iop := dat.GJStrLF()
				if len(iop) > 0 {
					s := iop
					com.Iops[i] = &s
				}
				// empty string → nil (TS: if (iop.length === 0) { com.iops[i] = null })
			}
		}

		// TS lines 198-200: TYPE_RECT tail
		if com.ComType == TypeRect {
			com.Fill = dat.GBool()
		}

		// TS lines 202-206: TYPE_TEXT or TYPE_UNUSED — text-family header
		if com.ComType == TypeText || com.ComType == TypeUnused {
			com.Center = dat.GBool()
			com.Font = int(dat.G1())
			com.FontSet = true
			com.Shadowed = dat.GBool()
		}

		// TS lines 208-211: TYPE_TEXT strings
		if com.ComType == TypeText {
			com.Text = dat.GJStrLF()
			com.ActiveText = dat.GJStrLF()
		}

		// TS lines 213-215: colour for TYPE_UNUSED / TYPE_RECT / TYPE_TEXT
		if com.ComType == TypeUnused || com.ComType == TypeRect || com.ComType == TypeText {
			com.Colour = int(int32(dat.G4()))
		}

		// TS lines 217-222: activeColour / overColour / activeOverColour for TYPE_RECT / TYPE_TEXT
		if com.ComType == TypeRect || com.ComType == TypeText {
			com.ActiveColour = int(int32(dat.G4()))
			com.OverColour = int(int32(dat.G4()))
			com.ActiveOverColour = int(int32(dat.G4())) // TS Unpack.ts:221
		}

		// TS lines 222-225: TYPE_GRAPHIC strings
		if com.ComType == TypeGraphic {
			com.Graphic = dat.GJStrLF()
			com.ActiveGraphic = dat.GJStrLF()
		}

		// TS lines 227-255: TYPE_MODEL tail
		// model/activeModel: wire-0 → leave as 0 (TS overwrites class-init -1
		//   with 0 when wire byte is 0 via direct assignment `com.model = dat.g1()`
		//   then the if-nonzero branch optionally replaces it with packed value).
		// anim/activeAnim: wire-0 → -1; non-zero → ((v-1)<<8)+g1.
		if com.ComType == TypeModel {
			modelHi := int(dat.G1())
			if modelHi != 0 {
				com.Model = ((modelHi - 1) << 8) + int(dat.G1())
			} else {
				com.Model = 0
			}

			activeModelHi := int(dat.G1())
			if activeModelHi != 0 {
				com.ActiveModel = ((activeModelHi - 1) << 8) + int(dat.G1())
			} else {
				com.ActiveModel = 0
			}

			animHi := int(dat.G1())
			if animHi == 0 {
				com.Anim = -1
			} else {
				com.Anim = ((animHi - 1) << 8) + int(dat.G1())
			}

			activeAnimHi := int(dat.G1())
			if activeAnimHi == 0 {
				com.ActiveAnim = -1
			} else {
				com.ActiveAnim = ((activeAnimHi - 1) << 8) + int(dat.G1())
			}

			com.Zoom = int(dat.G2())
			com.Xan = int(dat.G2())
			com.Yan = int(dat.G2())
		}

		// TS lines 257-276: TYPE_INV_TEXT tail
		if com.ComType == TypeInvText {
			com.Center = dat.GBool()
			com.Font = int(dat.G1())
			com.FontSet = true
			com.Shadowed = dat.GBool()
			com.Colour = int(int32(dat.G4()))
			com.MarginX = int(dat.G2S())
			com.MarginY = int(dat.G2S())
			com.Interactable = dat.GBool()

			com.Iops = make([]*string, 5)
			for i := range 5 {
				iop := dat.GJStrLF()
				if len(iop) > 0 {
					s := iop
					com.Iops[i] = &s
				}
			}
		}

		// TS lines 278-282: BUTTON_TARGET or TYPE_INV → actionVerb/action/actionTarget
		if com.ButtonType == ButtonTarget || com.ComType == TypeInv {
			com.ActionVerb = dat.GJStrLF()
			com.Action = dat.GJStrLF()
			com.ActionTarget = int(dat.G2())
		}

		// TS lines 284-286: BUTTON_OK / BUTTON_TOGGLE / BUTTON_SELECT / BUTTON_CONTINUE → option
		if com.ButtonType == ButtonOK || com.ButtonType == ButtonToggle ||
			com.ButtonType == ButtonSelect || com.ButtonType == ButtonContinue {
			com.Option = dat.GJStrLF()
		}

		// Store in the sparse Components slice, growing if needed.
		for id >= len(dec.Components) {
			dec.Components = append(dec.Components, nil)
		}
		dec.Components[id] = com
	}

	return dec, nil
}
