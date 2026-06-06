package clientinterface

import (
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// -----------------------------------------------------------------------
// Fixture helpers — build the binary stream that Decode reads.
// -----------------------------------------------------------------------

// buf is a small write helper so test fixtures don't need to import packet
// write methods directly (they just append raw bytes).
type buf struct {
	data []byte
}

func (b *buf) p1(v uint8) {
	b.data = append(b.data, v)
}

func (b *buf) p2(v uint16) {
	b.data = append(b.data, byte(v>>8), byte(v))
}

func (b *buf) p2s(v int16) {
	b.p2(uint16(v))
}

func (b *buf) p4s(v int32) {
	b.data = append(b.data, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

func (b *buf) pbool(v bool) {
	if v {
		b.p1(1)
	} else {
		b.p1(0)
	}
}

// pjstr writes a LF-terminated JagString.
func (b *buf) pjstr(s string) {
	b.data = append(b.data, []byte(s)...)
	b.data = append(b.data, 10) // LF terminator (TS Packet.gjstr default)
}

// pOverLayer encodes the TS overLayer wire format:
//   - -1 → single byte 0
//   - otherwise → hi = (v>>8)+1; lo = v & 0xFF → two bytes
func (b *buf) pOverLayer(v int) {
	if v == -1 {
		b.p1(0)
		return
	}
	hi := byte((v >> 8) + 1)
	lo := byte(v & 0xFF)
	b.p1(hi)
	b.p1(lo)
}

// pModelOrActiveModel encodes the model/activeModel wire format:
//   - 0 → single byte 0
//   - otherwise → hi = (v>>8)+1; lo = v & 0xFF
func (b *buf) pModelOrActiveModel(v int) {
	if v == 0 {
		b.p1(0)
		return
	}
	hi := byte((v >> 8) + 1)
	lo := byte(v & 0xFF)
	b.p1(hi)
	b.p1(lo)
}

// pAnim encodes the anim/activeAnim wire format:
//   - -1 → single byte 0
//   - otherwise → hi = (v>>8)+1; lo = v & 0xFF
func (b *buf) pAnim(v int) {
	if v == -1 {
		b.p1(0)
		return
	}
	hi := byte((v >> 8) + 1)
	lo := byte(v & 0xFF)
	b.p1(hi)
	b.p1(lo)
}

// pCommonHeader writes the common header fields (after the ID/layer marker).
// TS lines 108-113.
func (b *buf) pCommonHeader(comType, buttonType, clientCode, width, height, trans int) {
	b.p1(uint8(comType))
	b.p1(uint8(buttonType))
	b.p2(uint16(clientCode))
	b.p2(uint16(width))
	b.p2(uint16(height))
	b.p1(uint8(trans))
}

// pComparators writes the comparator/operand arrays (TS lines 122-131).
func (b *buf) pComparators(comparators []uint8, operands []uint16) {
	b.p1(uint8(len(comparators)))
	for i := range len(comparators) {
		b.p1(comparators[i])
		b.p2(operands[i])
	}
}

// pScripts writes the script array (TS lines 133-146).
// Each entry: g2 opcodeCount then that many g2 words.
func (b *buf) pScripts(scripts [][]uint16) {
	b.p1(uint8(len(scripts)))
	for _, script := range scripts {
		b.p2(uint16(len(script)))
		for _, w := range script {
			b.p2(w)
		}
	}
}

// pNoComparatorsNoScripts writes zero comparators and zero scripts.
func (b *buf) pNoComparatorsNoScripts() {
	b.p1(0) // comparatorCount = 0
	b.p1(0) // scriptCount = 0
}

// makePacket wraps buf.data in a packet.Packet for DecodePacket.
// It prepends the 2-byte count header (TS line 92).
func makePacket(count uint16, body []byte) *packet.Packet {
	full := make([]byte, 2+len(body))
	full[0] = byte(count >> 8)
	full[1] = byte(count)
	copy(full[2:], body)
	return packet.NewPacket(full)
}

// -----------------------------------------------------------------------
// Tests
// -----------------------------------------------------------------------

// TestDecode_TypeLayer verifies a full TYPE_LAYER component decode, including
// the 65535 layer-marker path and ChildX/ChildY signedness.
func TestDecode_TypeLayer(t *testing.T) {
	b := &buf{}

	// Emit the 65535 layer-marker: sets layer=10, then component id=10.
	// TS lines 98-100.
	b.p2(65535)
	b.p2(10) // new layer = 10
	b.p2(10) // component id = 10

	b.pCommonHeader(TypeLayer, 0, 42, 512, 334, 0)
	b.pOverLayer(-1)
	b.pNoComparatorsNoScripts()

	// TYPE_LAYER tail: scroll, hide, children
	b.p2(99) // scroll
	b.pbool(true)
	b.p2(2)     // childCount = 2
	b.p2(11)    // childId[0]
	b.p2s(-5)   // childX[0] negative — signedness test
	b.p2s(20)   // childY[0]
	b.p2(12)    // childId[1]
	b.p2s(100)  // childX[1]
	b.p2s(-300) // childY[1] negative

	pkt := makePacket(11, b.data)
	dec, err := DecodePacket(pkt)
	if err != nil {
		t.Fatalf("DecodePacket error: %v", err)
	}

	if dec.Count != 11 {
		t.Errorf("Count: got %d, want 11", dec.Count)
	}
	if len(dec.Order) != 1 || dec.Order[0] != 10 {
		t.Errorf("Order: got %v, want [10]", dec.Order)
	}

	com := dec.Components[10]
	if com == nil {
		t.Fatal("Components[10] is nil")
	}
	if com.ID != 10 {
		t.Errorf("ID: got %d, want 10", com.ID)
	}
	if com.RootLayer != 10 {
		t.Errorf("RootLayer: got %d, want 10", com.RootLayer)
	}
	if com.ComType != TypeLayer {
		t.Errorf("ComType: got %d, want %d", com.ComType, TypeLayer)
	}
	if com.ButtonType != 0 {
		t.Errorf("ButtonType: got %d, want 0", com.ButtonType)
	}
	if com.ClientCode != 42 {
		t.Errorf("ClientCode: got %d, want 42", com.ClientCode)
	}
	if com.Width != 512 {
		t.Errorf("Width: got %d, want 512", com.Width)
	}
	if com.Height != 334 {
		t.Errorf("Height: got %d, want 334", com.Height)
	}
	if com.OverLayer != -1 {
		t.Errorf("OverLayer: got %d, want -1", com.OverLayer)
	}
	if com.Scroll != 99 {
		t.Errorf("Scroll: got %d, want 99", com.Scroll)
	}
	if !com.Hide {
		t.Errorf("Hide: got false, want true")
	}
	if len(com.ChildID) != 2 {
		t.Fatalf("ChildID len: got %d, want 2", len(com.ChildID))
	}
	if com.ChildID[0] != 11 || com.ChildX[0] != -5 || com.ChildY[0] != 20 {
		t.Errorf("Child[0]: id=%d x=%d y=%d, want id=11 x=-5 y=20",
			com.ChildID[0], com.ChildX[0], com.ChildY[0])
	}
	if com.ChildID[1] != 12 || com.ChildX[1] != 100 || com.ChildY[1] != -300 {
		t.Errorf("Child[1]: id=%d x=%d y=%d, want id=12 x=100 y=-300",
			com.ChildID[1], com.ChildX[1], com.ChildY[1])
	}
}

// TestDecode_LayerMarkerTwoRoots verifies that two separate 65535 markers each
// set the layer correctly and both components appear in Order.
func TestDecode_LayerMarkerTwoRoots(t *testing.T) {
	b := &buf{}

	// First root: layer=0, id=0
	b.p2(65535)
	b.p2(0)
	b.p2(0)
	b.pCommonHeader(TypeLayer, 0, 0, 100, 100, 0)
	b.pOverLayer(-1)
	b.pNoComparatorsNoScripts()
	// TYPE_LAYER tail with 0 children
	b.p2(0) // scroll
	b.pbool(false)
	b.p2(0) // childCount = 0

	// Second root: layer=200, id=200
	b.p2(65535)
	b.p2(200)
	b.p2(200)
	b.pCommonHeader(TypeLayer, 0, 0, 50, 50, 0)
	b.pOverLayer(-1)
	b.pNoComparatorsNoScripts()
	b.p2(0) // scroll
	b.pbool(false)
	b.p2(0) // childCount = 0

	pkt := makePacket(201, b.data)
	dec, err := DecodePacket(pkt)
	if err != nil {
		t.Fatalf("DecodePacket error: %v", err)
	}

	if len(dec.Order) != 2 {
		t.Fatalf("Order len: got %d, want 2", len(dec.Order))
	}
	if dec.Order[0] != 0 || dec.Order[1] != 200 {
		t.Errorf("Order: got %v, want [0 200]", dec.Order)
	}

	com0 := dec.Components[0]
	if com0 == nil || com0.RootLayer != 0 {
		t.Errorf("Components[0].RootLayer: got %v, want 0", com0)
	}

	com200 := dec.Components[200]
	if com200 == nil || com200.RootLayer != 200 {
		t.Errorf("Components[200].RootLayer: want 200")
	}
}

// TestDecode_TypeInv verifies all TYPE_INV fields including InvSlotSprite,
// actionVerb/action/actionTarget (TYPE_INV always reads the button-target tail).
func TestDecode_TypeInv(t *testing.T) {
	b := &buf{}

	b.p2(5) // id = 5 (no layer marker — layer stays -1)
	b.pCommonHeader(TypeInv, 0, 0, 64, 64, 0)
	b.pOverLayer(-1)
	b.pNoComparatorsNoScripts()

	// TYPE_INV tail
	b.pbool(true)  // draggable
	b.pbool(true)  // interactable
	b.pbool(false) // usable
	b.p1(3)        // marginX
	b.p1(7)        // marginY

	// 20 inv slots — set only slot 0 and slot 19
	for i := range 20 {
		if i == 0 {
			b.pbool(true)
			b.p2s(-10) // offsetX
			b.p2s(15)  // offsetY
			b.pjstr("sprite0")
		} else if i == 19 {
			b.pbool(true)
			b.p2s(0)
			b.p2s(0)
			b.pjstr("sprite19")
		} else {
			b.pbool(false)
		}
	}

	// 5 iops: first is non-empty, rest empty
	b.pjstr("Take")
	b.pjstr("") // → nil
	b.pjstr("")
	b.pjstr("")
	b.pjstr("")

	// TYPE_INV always reads actionVerb/action/actionTarget (TS line 278)
	b.pjstr("Use")
	b.pjstr("with")
	b.p2(0x0003) // actionTarget

	pkt := makePacket(20, b.data)
	dec, err := DecodePacket(pkt)
	if err != nil {
		t.Fatalf("DecodePacket error: %v", err)
	}

	com := dec.Components[5]
	if com == nil {
		t.Fatal("Components[5] is nil")
	}
	if com.ComType != TypeInv {
		t.Errorf("ComType: got %d, want %d", com.ComType, TypeInv)
	}
	if !com.Draggable {
		t.Error("Draggable: want true")
	}
	if !com.Interactable {
		t.Error("Interactable: want true")
	}
	if com.Usable {
		t.Error("Usable: want false")
	}
	if com.MarginX != 3 || com.MarginY != 7 {
		t.Errorf("Margin: got %d,%d, want 3,7", com.MarginX, com.MarginY)
	}
	if com.InvSlotSprite == nil || len(com.InvSlotSprite) != 20 {
		t.Fatal("InvSlotSprite: nil or wrong length")
	}
	if com.InvSlotSprite[0] == nil || *com.InvSlotSprite[0] != "sprite0" {
		t.Errorf("InvSlotSprite[0]: got %v, want sprite0", com.InvSlotSprite[0])
	}
	if com.InvSlotOffsetX[0] != -10 || com.InvSlotOffsetY[0] != 15 {
		t.Errorf("InvSlotOffset[0]: got %d,%d, want -10,15",
			com.InvSlotOffsetX[0], com.InvSlotOffsetY[0])
	}
	if com.InvSlotSprite[19] == nil || *com.InvSlotSprite[19] != "sprite19" {
		t.Errorf("InvSlotSprite[19]: got %v, want sprite19", com.InvSlotSprite[19])
	}
	if com.InvSlotSprite[1] != nil {
		t.Errorf("InvSlotSprite[1]: got non-nil, want nil")
	}
	if com.Iops == nil || len(com.Iops) != 5 {
		t.Fatal("Iops: nil or wrong length")
	}
	if com.Iops[0] == nil || *com.Iops[0] != "Take" {
		t.Errorf("Iops[0]: got %v, want Take", com.Iops[0])
	}
	if com.Iops[1] != nil {
		t.Errorf("Iops[1]: got non-nil, want nil (empty string → nil)")
	}
	if com.ActionVerb != "Use" {
		t.Errorf("ActionVerb: got %q, want Use", com.ActionVerb)
	}
	if com.Action != "with" {
		t.Errorf("Action: got %q, want with", com.Action)
	}
	if com.ActionTarget != 3 {
		t.Errorf("ActionTarget: got %d, want 3", com.ActionTarget)
	}
}

// TestDecode_TypeRect verifies TYPE_RECT: fill, colour, activeColour, overColour.
// Pins negative colour (g4s signedness).
func TestDecode_TypeRect(t *testing.T) {
	b := &buf{}

	b.p2(3) // id = 3
	b.pCommonHeader(TypeRect, 0, 0, 10, 10, 0)
	b.pOverLayer(-1)
	b.pNoComparatorsNoScripts()

	// TYPE_RECT tail
	b.pbool(true) // fill

	// colour (TYPE_UNUSED | TYPE_RECT | TYPE_TEXT): use a negative value
	b.p4s(-1) // colour = -1 (0xFFFFFFFF in hex → signed -1)

	// activeColour, overColour (TYPE_RECT | TYPE_TEXT)
	b.p4s(0xFF0000) // red
	b.p4s(0)        // none

	pkt := makePacket(10, b.data)
	dec, err := DecodePacket(pkt)
	if err != nil {
		t.Fatalf("DecodePacket error: %v", err)
	}

	com := dec.Components[3]
	if com == nil {
		t.Fatal("Components[3] is nil")
	}
	if !com.Fill {
		t.Error("Fill: want true")
	}
	if com.Colour != -1 {
		t.Errorf("Colour: got %d, want -1 (signedness check)", com.Colour)
	}
	if com.ActiveColour != 0xFF0000 {
		t.Errorf("ActiveColour: got %d, want 0xFF0000", com.ActiveColour)
	}
	if com.OverColour != 0 {
		t.Errorf("OverColour: got %d, want 0", com.OverColour)
	}
}

// TestDecode_TypeText verifies TYPE_TEXT: center, font, shadowed, text/activeText,
// colour, activeColour, overColour; plus ButtonOK → option tail.
func TestDecode_TypeText(t *testing.T) {
	b := &buf{}

	b.p2(7) // id = 7
	b.pCommonHeader(TypeText, ButtonOK, 0, 100, 20, 0)
	b.pOverLayer(-1)
	b.pNoComparatorsNoScripts()

	// TYPE_TEXT / TYPE_UNUSED header (lines 202-206)
	b.pbool(true) // center
	b.p1(2)       // font = 2 (b12)
	b.pbool(true) // shadowed

	// TYPE_TEXT strings
	b.pjstr("Hello")
	b.pjstr("World")

	// colour (TYPE_UNUSED | TYPE_RECT | TYPE_TEXT)
	b.p4s(0x00FF00)

	// activeColour, overColour (TYPE_RECT | TYPE_TEXT)
	b.p4s(0xFF0000)
	b.p4s(0x0000FF)

	// ButtonOK → option (lines 284-286)
	b.pjstr("Click here")

	pkt := makePacket(10, b.data)
	dec, err := DecodePacket(pkt)
	if err != nil {
		t.Fatalf("DecodePacket error: %v", err)
	}

	com := dec.Components[7]
	if com == nil {
		t.Fatal("Components[7] is nil")
	}
	if !com.Center {
		t.Error("Center: want true")
	}
	if com.Font != 2 {
		t.Errorf("Font: got %d, want 2", com.Font)
	}
	if !com.FontSet {
		t.Error("FontSet: want true")
	}
	if !com.Shadowed {
		t.Error("Shadowed: want true")
	}
	if com.Text != "Hello" {
		t.Errorf("Text: got %q, want Hello", com.Text)
	}
	if com.ActiveText != "World" {
		t.Errorf("ActiveText: got %q, want World", com.ActiveText)
	}
	if com.Colour != 0x00FF00 {
		t.Errorf("Colour: got %d, want 0x00FF00", com.Colour)
	}
	if com.ActiveColour != 0xFF0000 {
		t.Errorf("ActiveColour: got %d, want 0xFF0000", com.ActiveColour)
	}
	if com.OverColour != 0x0000FF {
		t.Errorf("OverColour: got %d, want 0x0000FF", com.OverColour)
	}
	if com.Option != "Click here" {
		t.Errorf("Option: got %q, want 'Click here'", com.Option)
	}
}

// TestDecode_TypeGraphic verifies TYPE_GRAPHIC.
func TestDecode_TypeGraphic(t *testing.T) {
	b := &buf{}

	b.p2(20)
	b.pCommonHeader(TypeGraphic, 0, 0, 32, 32, 0)
	b.pOverLayer(-1)
	b.pNoComparatorsNoScripts()

	b.pjstr("coins.dat")
	b.pjstr("coins2.dat")

	pkt := makePacket(21, b.data)
	dec, err := DecodePacket(pkt)
	if err != nil {
		t.Fatalf("DecodePacket error: %v", err)
	}

	com := dec.Components[20]
	if com == nil {
		t.Fatal("Components[20] is nil")
	}
	if com.Graphic != "coins.dat" {
		t.Errorf("Graphic: got %q, want coins.dat", com.Graphic)
	}
	if com.ActiveGraphic != "coins2.dat" {
		t.Errorf("ActiveGraphic: got %q, want coins2.dat", com.ActiveGraphic)
	}
}

// TestDecode_TypeModel verifies TYPE_MODEL with the model vs. anim encoding difference.
//
//   - model/activeModel: wire-byte 0 → decoded value is 0 (NOT -1).
//   - anim/activeAnim:   wire-byte 0 → decoded value is -1.
//   - Non-zero both: ((hi-1)<<8) + lo.
func TestDecode_TypeModel(t *testing.T) {
	b := &buf{}

	b.p2(30)
	b.pCommonHeader(TypeModel, 0, 0, 128, 128, 0)
	b.pOverLayer(-1)
	b.pNoComparatorsNoScripts()

	// model = 256 = (1<<8)+0 → wire: hi=(1)+1=2, lo=0
	// Encode: hi=(256>>8)+1=2, lo=256&0xFF=0
	b.pModelOrActiveModel(256)
	// activeModel = 0 (wire byte 0)
	b.pModelOrActiveModel(0)
	// anim = 5 → ((5>>8)+1)=1, lo=5 → wire: 1,5
	b.pAnim(5)
	// activeAnim = -1 (wire byte 0)
	b.pAnim(-1)

	b.p2(500) // zoom
	b.p2(100) // xan
	b.p2(200) // yan

	pkt := makePacket(31, b.data)
	dec, err := DecodePacket(pkt)
	if err != nil {
		t.Fatalf("DecodePacket error: %v", err)
	}

	com := dec.Components[30]
	if com == nil {
		t.Fatal("Components[30] is nil")
	}
	if com.Model != 256 {
		t.Errorf("Model: got %d, want 256", com.Model)
	}
	if com.ActiveModel != 0 {
		t.Errorf("ActiveModel: got %d, want 0 (wire-0 branch, NOT -1)", com.ActiveModel)
	}
	if com.Anim != 5 {
		t.Errorf("Anim: got %d, want 5", com.Anim)
	}
	if com.ActiveAnim != -1 {
		t.Errorf("ActiveAnim: got %d, want -1 (wire-0 branch for anim)", com.ActiveAnim)
	}
	if com.Zoom != 500 {
		t.Errorf("Zoom: got %d, want 500", com.Zoom)
	}
	if com.Xan != 100 {
		t.Errorf("Xan: got %d, want 100", com.Xan)
	}
	if com.Yan != 200 {
		t.Errorf("Yan: got %d, want 200", com.Yan)
	}
}

// TestDecode_TypeInvText verifies TYPE_INV_TEXT: center, font, shadowed, colour,
// marginX/marginY (g2s — signed), interactable, iops.
func TestDecode_TypeInvText(t *testing.T) {
	b := &buf{}

	b.p2(50)
	b.pCommonHeader(TypeInvText, 0, 0, 80, 16, 0)
	b.pOverLayer(-1)
	b.pNoComparatorsNoScripts()

	b.pbool(false)   // center
	b.p1(1)          // font = 1 (p12)
	b.pbool(false)   // shadowed
	b.p4s(-16711936) // colour = -16711936 (negative test)
	b.p2s(-2)        // marginX negative
	b.p2s(3)         // marginY
	b.pbool(true)    // interactable

	// 5 iops
	b.pjstr("Examine")
	b.pjstr("")
	b.pjstr("")
	b.pjstr("")
	b.pjstr("")

	pkt := makePacket(51, b.data)
	dec, err := DecodePacket(pkt)
	if err != nil {
		t.Fatalf("DecodePacket error: %v", err)
	}

	com := dec.Components[50]
	if com == nil {
		t.Fatal("Components[50] is nil")
	}
	if com.ComType != TypeInvText {
		t.Errorf("ComType: got %d, want %d", com.ComType, TypeInvText)
	}
	if com.Font != 1 {
		t.Errorf("Font: got %d, want 1", com.Font)
	}
	if !com.FontSet {
		t.Error("FontSet: want true")
	}
	if com.Colour != -16711936 {
		t.Errorf("Colour: got %d, want -16711936 (signedness test)", com.Colour)
	}
	if com.MarginX != -2 {
		t.Errorf("MarginX: got %d, want -2 (signed g2s)", com.MarginX)
	}
	if com.MarginY != 3 {
		t.Errorf("MarginY: got %d, want 3", com.MarginY)
	}
	if !com.Interactable {
		t.Error("Interactable: want true")
	}
	if com.Iops == nil || com.Iops[0] == nil || *com.Iops[0] != "Examine" {
		t.Errorf("Iops[0]: want Examine")
	}
	if com.Iops[1] != nil {
		t.Error("Iops[1]: want nil")
	}
}

// TestDecode_OverLayerEncoding tests both branches of the overLayer encoding.
func TestDecode_OverLayerEncoding(t *testing.T) {
	// Non-zero overLayer: value = ((hi-1)<<8) + lo
	// E.g. overLayer = 0x0105 = 261:
	//   hi = (261>>8)+1 = 2, lo = 261&0xFF = 5

	b2 := &buf{}
	b2.p2(1) // id = 1
	b2.pCommonHeader(TypeUnused, 0, 0, 10, 10, 0)
	// overLayer = 261
	b2.p1(2) // hi
	b2.p1(5) // lo
	b2.p1(0) // comparatorCount
	b2.p1(0) // scriptCount
	// TYPE_UNUSED: skip 3 bytes (already in the stream)
	b2.p1(7) // skipped byte 1
	b2.p1(8) // skipped byte 2
	b2.p1(9) // skipped byte 3
	// TYPE_TEXT or TYPE_UNUSED header
	b2.pbool(true) // center
	b2.p1(3)       // font
	b2.pbool(true) // shadowed
	// colour (g4s)
	b2.p4s(12345)

	pkt := makePacket(5, b2.data)
	dec, err := DecodePacket(pkt)
	if err != nil {
		t.Fatalf("DecodePacket error: %v", err)
	}
	com := dec.Components[1]
	if com == nil {
		t.Fatal("Components[1] nil")
	}
	if com.OverLayer != 261 {
		t.Errorf("OverLayer: got %d, want 261", com.OverLayer)
	}
	if com.Center != true {
		t.Error("Center: want true")
	}
	if com.Font != 3 {
		t.Errorf("Font: got %d, want 3", com.Font)
	}
	if com.Colour != 12345 {
		t.Errorf("Colour: got %d, want 12345", com.Colour)
	}
}

// TestDecode_ScriptsAndComparators exercises comparator/operand and script
// array round-trips, including multi-script and non-trivial opcodeCount.
func TestDecode_ScriptsAndComparators(t *testing.T) {
	b := &buf{}

	b.p2(99)
	b.pCommonHeader(TypeLayer, ButtonToggle, 0, 100, 100, 0)
	b.pOverLayer(-1)

	// 2 comparators
	b.pComparators(
		[]uint8{1, 3},
		[]uint16{100, 200},
	)

	// 3 scripts
	// script[0]: opcodeCount=2, words=[10, 20]
	// script[1]: opcodeCount=1, words=[5]
	// script[2]: opcodeCount=3, words=[7, 8, 9]
	b.pScripts([][]uint16{
		{10, 20},
		{5},
		{7, 8, 9},
	})

	// TYPE_LAYER tail
	b.p2(0) // scroll
	b.pbool(false)
	b.p2(0) // childCount

	// ButtonToggle → option
	b.pjstr("Toggle me")

	pkt := makePacket(100, b.data)
	dec, err := DecodePacket(pkt)
	if err != nil {
		t.Fatalf("DecodePacket error: %v", err)
	}

	com := dec.Components[99]
	if com == nil {
		t.Fatal("Components[99] nil")
	}
	if len(com.ScriptComparator) != 2 {
		t.Fatalf("ScriptComparator len: got %d, want 2", len(com.ScriptComparator))
	}
	if com.ScriptComparator[0] != 1 || com.ScriptComparator[1] != 3 {
		t.Errorf("ScriptComparator: got %v, want [1 3]", com.ScriptComparator)
	}
	if com.ScriptOperand[0] != 100 || com.ScriptOperand[1] != 200 {
		t.Errorf("ScriptOperand: got %v, want [100 200]", com.ScriptOperand)
	}
	if len(com.Script) != 3 {
		t.Fatalf("Script len: got %d, want 3", len(com.Script))
	}
	if len(com.Script[0]) != 2 || com.Script[0][0] != 10 || com.Script[0][1] != 20 {
		t.Errorf("Script[0]: got %v, want [10 20]", com.Script[0])
	}
	if len(com.Script[1]) != 1 || com.Script[1][0] != 5 {
		t.Errorf("Script[1]: got %v, want [5]", com.Script[1])
	}
	if len(com.Script[2]) != 3 {
		t.Errorf("Script[2] len: got %d, want 3", len(com.Script[2]))
	}
	if com.Option != "Toggle me" {
		t.Errorf("Option: got %q, want 'Toggle me'", com.Option)
	}
}

// TestDecode_ButtonTarget verifies that BUTTON_TARGET reads actionVerb/action/actionTarget.
func TestDecode_ButtonTarget(t *testing.T) {
	b := &buf{}

	b.p2(60)
	b.pCommonHeader(TypeText, ButtonTarget, 0, 100, 20, 0)
	b.pOverLayer(-1)
	b.pNoComparatorsNoScripts()

	// TYPE_TEXT / TYPE_UNUSED header
	b.pbool(false) // center
	b.p1(0)        // font
	b.pbool(false) // shadowed

	// TYPE_TEXT strings
	b.pjstr("Drop")
	b.pjstr("")

	// colour (TYPE_UNUSED | TYPE_RECT | TYPE_TEXT)
	b.p4s(0)
	// activeColour, overColour (TYPE_RECT | TYPE_TEXT)
	b.p4s(0)
	b.p4s(0)

	// BUTTON_TARGET → actionVerb/action/actionTarget (TS line 278)
	b.pjstr("Wield")
	b.pjstr("weapon")
	b.p2(0x0005) // actionTarget = 5

	pkt := makePacket(61, b.data)
	dec, err := DecodePacket(pkt)
	if err != nil {
		t.Fatalf("DecodePacket error: %v", err)
	}

	com := dec.Components[60]
	if com == nil {
		t.Fatal("Components[60] nil")
	}
	if com.ActionVerb != "Wield" {
		t.Errorf("ActionVerb: got %q, want Wield", com.ActionVerb)
	}
	if com.Action != "weapon" {
		t.Errorf("Action: got %q, want weapon", com.Action)
	}
	if com.ActionTarget != 5 {
		t.Errorf("ActionTarget: got %d, want 5", com.ActionTarget)
	}
}

// TestDecode_ButtonContinue verifies ButtonContinue reads the option string.
func TestDecode_ButtonContinue(t *testing.T) {
	b := &buf{}

	b.p2(70)
	b.pCommonHeader(TypeText, ButtonContinue, 0, 100, 20, 0)
	b.pOverLayer(-1)
	b.pNoComparatorsNoScripts()

	b.pbool(false) // center
	b.p1(0)        // font
	b.pbool(false) // shadowed
	b.pjstr("Click to continue")
	b.pjstr("")
	b.p4s(0) // colour
	b.p4s(0) // activeColour
	b.p4s(0) // overColour

	// ButtonContinue → option
	b.pjstr("Click to continue")

	pkt := makePacket(71, b.data)
	dec, err := DecodePacket(pkt)
	if err != nil {
		t.Fatalf("DecodePacket error: %v", err)
	}

	com := dec.Components[70]
	if com == nil {
		t.Fatal("Components[70] nil")
	}
	if com.Option != "Click to continue" {
		t.Errorf("Option: got %q, want 'Click to continue'", com.Option)
	}
}

// TestDecode_TypeUnused verifies that TYPE_UNUSED skips 3 bytes and then reads
// the text-family header (center, font, shadowed) and colour.
func TestDecode_TypeUnused(t *testing.T) {
	b := &buf{}

	b.p2(80)
	b.pCommonHeader(TypeUnused, 0, 0, 10, 10, 0)
	b.pOverLayer(-1)
	b.pNoComparatorsNoScripts()

	// TYPE_UNUSED: skip 3 bytes (content irrelevant)
	b.p1(0xFF)
	b.p1(0xFF)
	b.p1(0xFF)

	// TYPE_TEXT || TYPE_UNUSED header (lines 202-206)
	b.pbool(true) // center
	b.p1(1)       // font
	b.pbool(true) // shadowed

	// colour (TYPE_UNUSED | TYPE_RECT | TYPE_TEXT)
	b.p4s(999)

	pkt := makePacket(81, b.data)
	dec, err := DecodePacket(pkt)
	if err != nil {
		t.Fatalf("DecodePacket error: %v", err)
	}

	com := dec.Components[80]
	if com == nil {
		t.Fatal("Components[80] nil")
	}
	if com.ComType != TypeUnused {
		t.Errorf("ComType: got %d, want %d", com.ComType, TypeUnused)
	}
	if !com.Center {
		t.Error("Center: want true")
	}
	if com.Font != 1 {
		t.Errorf("Font: got %d, want 1", com.Font)
	}
	if com.Colour != 999 {
		t.Errorf("Colour: got %d, want 999", com.Colour)
	}
}

// TestDecode_Defaults verifies that fields NOT written for a TYPE_LAYER
// component retain their TS class-default values.
func TestDecode_Defaults(t *testing.T) {
	b := &buf{}

	b.p2(5)
	b.pCommonHeader(TypeLayer, 0, 0, 0, 0, 0)
	b.pOverLayer(-1)
	b.pNoComparatorsNoScripts()
	b.p2(0) // scroll = 0
	b.pbool(false)
	b.p2(0) // childCount = 0

	pkt := makePacket(6, b.data)
	dec, err := DecodePacket(pkt)
	if err != nil {
		t.Fatalf("DecodePacket error: %v", err)
	}

	com := dec.Components[5]
	if com == nil {
		t.Fatal("Components[5] nil")
	}

	// TS class defaults:
	// anim = -1, activeAnim = -1 (TS:845-846)
	if com.Anim != -1 {
		t.Errorf("Anim default: got %d, want -1", com.Anim)
	}
	if com.ActiveAnim != -1 {
		t.Errorf("ActiveAnim default: got %d, want -1", com.ActiveAnim)
	}
	// actionTarget = -1 (TS:852)
	if com.ActionTarget != -1 {
		t.Errorf("ActionTarget default: got %d, want -1", com.ActionTarget)
	}
	// overLayer = -1 (TS:817)
	if com.OverLayer != -1 {
		t.Errorf("OverLayer default: got %d, want -1", com.OverLayer)
	}
	// ScriptComparator and Script nil when not written
	if com.ScriptComparator != nil {
		t.Errorf("ScriptComparator: want nil")
	}
	if com.Script != nil {
		t.Errorf("Script: want nil")
	}
	// FontSet false (font was not read)
	if com.FontSet {
		t.Error("FontSet: want false for TYPE_LAYER (font never read)")
	}
}

// TestDecode_ModelAnimEncodingBoundary specifically pins the model==0 vs anim==-1
// distinction for the wire-byte-0 path (the key encode difference).
func TestDecode_ModelAnimEncodingBoundary(t *testing.T) {
	// Wire byte 0 for model → model stays 0 (TS: com.model = dat.g1() → 0, if nonzero not entered)
	// Wire byte 0 for anim  → anim = -1  (TS: if (com.anim === 0) { com.anim = -1 })
	b := &buf{}
	b.p2(1)
	b.pCommonHeader(TypeModel, 0, 0, 128, 128, 0)
	b.pOverLayer(-1)
	b.pNoComparatorsNoScripts()

	b.pModelOrActiveModel(0) // model = 0 (wire byte 0)
	b.pModelOrActiveModel(0) // activeModel = 0
	b.pAnim(-1)              // anim = -1 (wire byte 0)
	b.pAnim(-1)              // activeAnim = -1
	b.p2(0)
	b.p2(0)
	b.p2(0)

	pkt := makePacket(2, b.data)
	dec, err := DecodePacket(pkt)
	if err != nil {
		t.Fatalf("DecodePacket: %v", err)
	}

	com := dec.Components[1]
	if com == nil {
		t.Fatal("Components[1] nil")
	}
	// model wire-0 → 0, NOT -1
	if com.Model != 0 {
		t.Errorf("Model wire-0 branch: got %d, want 0 (NOT -1)", com.Model)
	}
	// anim wire-0 → -1
	if com.Anim != -1 {
		t.Errorf("Anim wire-0 branch: got %d, want -1", com.Anim)
	}
}

// TestDecode_MultipleComponents verifies that two sequential components are
// decoded and Order captures both IDs.
func TestDecode_MultipleComponents(t *testing.T) {
	b := &buf{}

	// Component id=0 (no layer marker — layer = -1)
	b.p2(0)
	b.pCommonHeader(TypeRect, 0, 0, 10, 10, 0)
	b.pOverLayer(-1)
	b.pNoComparatorsNoScripts()
	b.pbool(false) // fill
	b.p4s(0)       // colour
	b.p4s(0)       // activeColour
	b.p4s(0)       // overColour

	// Component id=1
	b.p2(1)
	b.pCommonHeader(TypeRect, 0, 0, 20, 20, 0)
	b.pOverLayer(-1)
	b.pNoComparatorsNoScripts()
	b.pbool(true) // fill
	b.p4s(255)    // colour
	b.p4s(0)
	b.p4s(0)

	pkt := makePacket(2, b.data)
	dec, err := DecodePacket(pkt)
	if err != nil {
		t.Fatalf("DecodePacket error: %v", err)
	}

	if len(dec.Order) != 2 {
		t.Fatalf("Order len: got %d, want 2", len(dec.Order))
	}
	if dec.Order[0] != 0 || dec.Order[1] != 1 {
		t.Errorf("Order: got %v, want [0 1]", dec.Order)
	}
	if dec.Components[0] == nil || dec.Components[1] == nil {
		t.Fatal("Components[0] or [1] nil")
	}
	if dec.Components[1].Fill != true {
		t.Error("Components[1].Fill: want true")
	}
	if dec.Components[1].Colour != 255 {
		t.Errorf("Components[1].Colour: got %d, want 255", dec.Components[1].Colour)
	}
}
