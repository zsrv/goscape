package objtype

import (
	"os"
	"path/filepath"
	"testing"

	jag "github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
)

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
			0, 0, // scroll = 0
			0,    // hide = false
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

func TestComponentDecode_RootLayerSentinel(t *testing.T) {
	p := packet.NewPacket(nil)
	p.P2(0)     // count header
	p.P2(65535) // sentinel
	p.P2(99)    // rootLayer
	p.P2(10)    // real id
	p.P1(ComTypeLayer)
	p.P1(ButtonNone)
	p.P2(0)
	p.P2(0)
	p.P2(0) // clientCode/width/height
	p.P1(0) // overLayer
	p.P1(0) // comparatorCount
	p.P1(0) // scriptCount
	// TYPE_LAYER body: scroll/hide/childCount=0
	p.P2(0)
	p.P1(0)
	p.P1(0)

	cfg, err := parseComponentTypes(p, nil)
	if err != nil {
		t.Fatalf("parseComponentTypes: %v", err)
	}
	if len(cfg.Configs) <= 10 || cfg.Configs[10] == nil {
		t.Fatalf("Configs[10]: missing")
	}
	if cfg.Configs[10].RootLayer != 99 {
		t.Errorf("RootLayer: got %d, want 99", cfg.Configs[10].RootLayer)
	}
}

func TestComponentDecode_OverLayerZero(t *testing.T) {
	// overLayer byte = 0 → OverLayer must be -1 (no follow-up byte consumed)
	client := minimalComponentRecord(t, 3, ComTypeLayer, ButtonNone,
		[]byte{0, 0, 0, 0}, nil)
	cfg, err := parseComponentTypes(client, nil)
	if err != nil {
		t.Fatalf("parseComponentTypes: %v", err)
	}
	if cfg.Configs[3].OverLayer != -1 {
		t.Errorf("OverLayer: got %d, want -1", cfg.Configs[3].OverLayer)
	}
}

func TestComponentDecode_OverLayerNonZero(t *testing.T) {
	// overLayer byte = 2, next byte = 5 → OverLayer = ((2-1)<<8)+5 = 261
	p := packet.NewPacket(nil)
	p.P2(0)  // count header
	p.P2(3)  // id
	p.P1(ComTypeLayer)
	p.P1(ButtonNone)
	p.P2(0)
	p.P2(0)
	p.P2(0) // clientCode/width/height
	p.P1(2) // overLayer hi = 2
	p.P1(5) // overLayer lo = 5
	p.P1(0) // comparatorCount
	p.P1(0) // scriptCount
	// TYPE_LAYER body
	p.P2(0)
	p.P1(0)
	p.P1(0)

	cfg, err := parseComponentTypes(p, nil)
	if err != nil {
		t.Fatalf("parseComponentTypes: %v", err)
	}
	want := ((2 - 1) << 8) + 5 // 261
	if cfg.Configs[3].OverLayer != want {
		t.Errorf("OverLayer: got %d, want %d", cfg.Configs[3].OverLayer, want)
	}
}

func TestComponentDecode_ScriptComparator(t *testing.T) {
	p := packet.NewPacket(nil)
	p.P2(0) // count header
	p.P2(1) // id
	p.P1(ComTypeLayer)
	p.P1(ButtonNone)
	p.P2(0)
	p.P2(0)
	p.P2(0) // clientCode/width/height
	p.P1(0) // overLayer
	p.P1(3) // comparatorCount = 3
	p.P1(1)
	p.P2(100)
	p.P1(2)
	p.P2(200)
	p.P1(3)
	p.P2(300)
	p.P1(0) // scriptCount = 0
	// TYPE_LAYER body
	p.P2(0)
	p.P1(0)
	p.P1(0)

	cfg, err := parseComponentTypes(p, nil)
	if err != nil {
		t.Fatalf("parseComponentTypes: %v", err)
	}
	c := cfg.Configs[1]
	if len(c.ScriptComparator) != 3 {
		t.Fatalf("ScriptComparator len: got %d, want 3", len(c.ScriptComparator))
	}
	if c.ScriptComparator[0] != 1 || c.ScriptComparator[1] != 2 || c.ScriptComparator[2] != 3 {
		t.Errorf("ScriptComparator: got %v, want [1 2 3]", c.ScriptComparator)
	}
	if len(c.ScriptOperand) != 3 {
		t.Fatalf("ScriptOperand len: got %d, want 3", len(c.ScriptOperand))
	}
	if c.ScriptOperand[0] != 100 || c.ScriptOperand[1] != 200 || c.ScriptOperand[2] != 300 {
		t.Errorf("ScriptOperand: got %v, want [100 200 300]", c.ScriptOperand)
	}
}

func TestComponentDecode_ScriptsArray(t *testing.T) {
	// scriptCount=2, opcodeCount=4 each
	p := packet.NewPacket(nil)
	p.P2(0) // count header
	p.P2(2) // id
	p.P1(ComTypeLayer)
	p.P1(ButtonNone)
	p.P2(0)
	p.P2(0)
	p.P2(0) // clientCode/width/height
	p.P1(0) // overLayer
	p.P1(0) // comparatorCount
	p.P1(2) // scriptCount = 2
	p.P2(4) // script[0] opcodeCount = 4
	p.P2(10)
	p.P2(20)
	p.P2(30)
	p.P2(40)
	p.P2(4) // script[1] opcodeCount = 4
	p.P2(50)
	p.P2(60)
	p.P2(70)
	p.P2(80)
	// TYPE_LAYER body
	p.P2(0)
	p.P1(0)
	p.P1(0)

	cfg, err := parseComponentTypes(p, nil)
	if err != nil {
		t.Fatalf("parseComponentTypes: %v", err)
	}
	c := cfg.Configs[2]
	if len(c.Scripts) != 2 {
		t.Fatalf("Scripts len: got %d, want 2", len(c.Scripts))
	}
	if len(c.Scripts[0]) != 4 {
		t.Fatalf("Scripts[0] len: got %d, want 4", len(c.Scripts[0]))
	}
	if c.Scripts[0][0] != 10 || c.Scripts[0][3] != 40 {
		t.Errorf("Scripts[0]: got %v, want [10 20 30 40]", c.Scripts[0])
	}
	if c.Scripts[1][0] != 50 || c.Scripts[1][3] != 80 {
		t.Errorf("Scripts[1]: got %v, want [50 60 70 80]", c.Scripts[1])
	}
}

func TestComponentDecode_TypeLayer(t *testing.T) {
	// scroll=500, hide=true, childCount=2 with full child fields
	p := packet.NewPacket(nil)
	p.P2(0) // count header
	p.P2(4) // id
	p.P1(ComTypeLayer)
	p.P1(ButtonNone)
	p.P2(0)
	p.P2(0)
	p.P2(0) // clientCode/width/height
	p.P1(0) // overLayer
	p.P1(0) // comparatorCount
	p.P1(0) // scriptCount
	p.P2(500) // scroll
	p.P1(1)   // hide = true
	p.P1(2)   // childCount = 2
	// child[0]: id=10, x=20, y=30
	p.P2(10)
	p.P2(uint16(int16(20)))
	p.P2(uint16(int16(30)))
	// child[1]: id=11, x=-5, y=-10
	p.P2(11)
	neg5 := int16(-5)
	neg10 := int16(-10)
	p.P2(uint16(neg5))
	p.P2(uint16(neg10))

	cfg, err := parseComponentTypes(p, nil)
	if err != nil {
		t.Fatalf("parseComponentTypes: %v", err)
	}
	c := cfg.Configs[4]
	if c.Scroll != 500 {
		t.Errorf("Scroll: got %d, want 500", c.Scroll)
	}
	if !c.Hide {
		t.Errorf("Hide: got false, want true")
	}
	if len(c.ChildId) != 2 {
		t.Fatalf("ChildId len: got %d, want 2", len(c.ChildId))
	}
	if c.ChildId[0] != 10 || c.ChildX[0] != 20 || c.ChildY[0] != 30 {
		t.Errorf("child[0]: id=%d x=%d y=%d, want 10/20/30", c.ChildId[0], c.ChildX[0], c.ChildY[0])
	}
	if c.ChildId[1] != 11 || c.ChildX[1] != -5 || c.ChildY[1] != -10 {
		t.Errorf("child[1]: id=%d x=%d y=%d, want 11/-5/-10", c.ChildId[1], c.ChildX[1], c.ChildY[1])
	}
}

func TestComponentDecode_TypeUnused(t *testing.T) {
	// ComTypeUnused skips 10 bytes; next record (id=1) must decode correctly.
	p := packet.NewPacket(nil)
	p.P2(0) // count header
	// record id=0, ComTypeUnused
	p.P2(0)
	p.P1(ComTypeUnused)
	p.P1(ButtonNone)
	p.P2(0)
	p.P2(0)
	p.P2(0) // clientCode/width/height
	p.P1(0) // overLayer
	p.P1(0) // comparatorCount
	p.P1(0) // scriptCount
	// 10 unused bytes
	p.Data = append(p.Data, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	// record id=1, ComTypeLayer
	p.P2(1)
	p.P1(ComTypeLayer)
	p.P1(ButtonNone)
	p.P2(0)
	p.P2(0)
	p.P2(0) // clientCode/width/height
	p.P1(0) // overLayer
	p.P1(0) // comparatorCount
	p.P1(0) // scriptCount
	p.P2(0)
	p.P1(0)
	p.P1(0) // scroll/hide/childCount=0

	cfg, err := parseComponentTypes(p, nil)
	if err != nil {
		t.Fatalf("parseComponentTypes: %v", err)
	}
	if cfg.Configs[0] == nil {
		t.Fatalf("Configs[0]: missing")
	}
	if cfg.Configs[0].ComType != ComTypeUnused {
		t.Errorf("Configs[0].ComType: got %d, want ComTypeUnused", cfg.Configs[0].ComType)
	}
	if len(cfg.Configs) <= 1 || cfg.Configs[1] == nil {
		t.Fatalf("Configs[1]: missing — 10-byte skip consumed too many bytes")
	}
	if cfg.Configs[1].ComType != ComTypeLayer {
		t.Errorf("Configs[1].ComType: got %d, want ComTypeLayer", cfg.Configs[1].ComType)
	}
}

func TestComponentDecode_TypeInventory(t *testing.T) {
	// 2 populated slots (indices 0 and 3), 18 empty; iop[5]; action fields
	p := packet.NewPacket(nil)
	p.P2(0) // count header
	p.P2(6) // id
	p.P1(ComTypeInventory)
	p.P1(ButtonNone)
	p.P2(0)
	p.P2(0)
	p.P2(0) // clientCode/width/height
	p.P1(0) // overLayer
	p.P1(0) // comparatorCount
	p.P1(0) // scriptCount
	p.P1(1) // draggable = true
	p.P1(0) // operable = false
	p.P1(1) // usable = true
	p.P1(3) // marginX = 3
	p.P1(4) // marginY = 4
	// 20 slots: slot 0 populated, slot 3 populated, rest empty
	for i := range 20 {
		if i == 0 {
			p.P1(1) // GBool=true
			p.P2(uint16(int16(10)))
			p.P2(uint16(int16(20)))
			p.PJStrLF("sprite0")
		} else if i == 3 {
			p.P1(1) // GBool=true
			negOne := int16(-1)
			negTwo := int16(-2)
			p.P2(uint16(negOne))
			p.P2(uint16(negTwo))
			p.PJStrLF("sprite3")
		} else {
			p.P1(0) // GBool=false
		}
	}
	// iop[5]
	p.PJStrLF("use1")
	p.PJStrLF("use2")
	p.PJStrLF("use3")
	p.PJStrLF("use4")
	p.PJStrLF("use5")
	// actionVerb, action, actionTarget
	p.PJStrLF("Wield")
	p.PJStrLF("wield")
	p.P2(7)

	cfg, err := parseComponentTypes(p, nil)
	if err != nil {
		t.Fatalf("parseComponentTypes: %v", err)
	}
	c := cfg.Configs[6]
	if !c.Draggable {
		t.Errorf("Draggable: want true")
	}
	if c.Operable {
		t.Errorf("Operable: want false")
	}
	if !c.Usable {
		t.Errorf("Usable: want true")
	}
	if c.MarginX != 3 {
		t.Errorf("MarginX: got %d, want 3", c.MarginX)
	}
	if c.MarginY != 4 {
		t.Errorf("MarginY: got %d, want 4", c.MarginY)
	}
	if c.InventorySlotOffsetX[0] != 10 || c.InventorySlotOffsetY[0] != 20 || c.InventorySlotGraphic[0] != "sprite0" {
		t.Errorf("slot[0]: offsetX=%d offsetY=%d graphic=%q, want 10/20/sprite0", c.InventorySlotOffsetX[0], c.InventorySlotOffsetY[0], c.InventorySlotGraphic[0])
	}
	if c.InventorySlotOffsetX[3] != -1 || c.InventorySlotOffsetY[3] != -2 || c.InventorySlotGraphic[3] != "sprite3" {
		t.Errorf("slot[3]: offsetX=%d offsetY=%d graphic=%q, want -1/-2/sprite3", c.InventorySlotOffsetX[3], c.InventorySlotOffsetY[3], c.InventorySlotGraphic[3])
	}
	if len(c.Iop) != 5 || c.Iop[0] != "use1" || c.Iop[4] != "use5" {
		t.Errorf("Iop: got %v, want [use1..use5]", c.Iop)
	}
	if c.ActionVerb != "Wield" {
		t.Errorf("ActionVerb: got %q, want Wield", c.ActionVerb)
	}
	if c.Action != "wield" {
		t.Errorf("Action: got %q, want wield", c.Action)
	}
	if c.ActionTarget != 7 {
		t.Errorf("ActionTarget: got %d, want 7", c.ActionTarget)
	}
}

func TestComponentDecode_TypeRect(t *testing.T) {
	typeBody := []byte{
		1,                // fill = true
		0x00, 0x11, 0x22, 0x33, // colour = 0x00112233
		0x00, 0xAA, 0xBB, 0xCC, // activeColour
		0xFF, 0x00, 0x00, 0x01, // overColour
	}
	client := minimalComponentRecord(t, 5, ComTypeRect, ButtonNone, typeBody, nil)
	cfg, err := parseComponentTypes(client, nil)
	if err != nil {
		t.Fatalf("parseComponentTypes: %v", err)
	}
	c := cfg.Configs[5]
	if !c.Fill {
		t.Errorf("Fill: want true")
	}
	if c.Colour != int32(0x00112233) {
		t.Errorf("Colour: got %08x, want 00112233", c.Colour)
	}
	if c.ActiveColour != int32(0x00AABBCC) {
		t.Errorf("ActiveColour: got %08x, want 00AABBCC", c.ActiveColour)
	}
	// 0xFF000001 interpreted as int32 (two's complement = -16777215)
	wantOverColour := int32(-16777215) // int32(uint32(0xFF000001))
	if c.OverColour != wantOverColour {
		t.Errorf("OverColour: got %08x, want ff000001", c.OverColour)
	}
}

func TestComponentDecode_TypeText(t *testing.T) {
	p := packet.NewPacket(nil)
	p.P2(0) // count header
	p.P2(8) // id
	p.P1(ComTypeText)
	p.P1(ButtonNone)
	p.P2(0)
	p.P2(0)
	p.P2(0) // clientCode/width/height
	p.P1(0) // overLayer
	p.P1(0) // comparatorCount
	p.P1(0) // scriptCount
	p.P1(1) // center = true
	p.P1(2) // font = 2
	p.P1(1) // shadowed = true
	p.PJStrLF("Hello")
	p.PJStrLF("World")
	p.P4(0x001122FF) // colour
	p.P4(0x002244FF) // activeColour
	p.P4(0x003366FF) // overColour

	cfg, err := parseComponentTypes(p, nil)
	if err != nil {
		t.Fatalf("parseComponentTypes: %v", err)
	}
	c := cfg.Configs[8]
	if !c.Center {
		t.Errorf("Center: want true")
	}
	if c.Font != 2 {
		t.Errorf("Font: got %d, want 2", c.Font)
	}
	if !c.Shadowed {
		t.Errorf("Shadowed: want true")
	}
	if c.Text != "Hello" {
		t.Errorf("Text: got %q, want Hello", c.Text)
	}
	if c.ActiveText != "World" {
		t.Errorf("ActiveText: got %q, want World", c.ActiveText)
	}
	if c.Colour != int32(0x001122FF) {
		t.Errorf("Colour: got %08x, want 001122ff", c.Colour)
	}
}

func TestComponentDecode_TypeSprite(t *testing.T) {
	p := packet.NewPacket(nil)
	p.P2(0) // count header
	p.P2(9) // id
	p.P1(ComTypeSprite)
	p.P1(ButtonNone)
	p.P2(0)
	p.P2(0)
	p.P2(0) // clientCode/width/height
	p.P1(0) // overLayer
	p.P1(0) // comparatorCount
	p.P1(0) // scriptCount
	p.PJStrLF("sword")
	p.PJStrLF("sword_active")

	cfg, err := parseComponentTypes(p, nil)
	if err != nil {
		t.Fatalf("parseComponentTypes: %v", err)
	}
	c := cfg.Configs[9]
	if c.Graphic != "sword" {
		t.Errorf("Graphic: got %q, want sword", c.Graphic)
	}
	if c.ActiveGraphic != "sword_active" {
		t.Errorf("ActiveGraphic: got %q, want sword_active", c.ActiveGraphic)
	}
}

func TestComponentDecode_TypeModel(t *testing.T) {
	body := []byte{
		2, 1, // model: hi=2, lo=1 → ((2-1)<<8)+1 = 257
		0,    // activeModel: hi=0 → 0
		0,    // anim: hi=0 → -1
		2, 1, // activeAnim: hi=2, lo=1 → 257
		0, 100, // zoom = 100
		0, 50,  // xan = 50
		0, 25,  // yan = 25
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

func TestComponentDecode_TypeInventoryText(t *testing.T) {
	p := packet.NewPacket(nil)
	p.P2(0)  // count header
	p.P2(12) // id
	p.P1(ComTypeInventoryText)
	p.P1(ButtonNone)
	p.P2(0)
	p.P2(0)
	p.P2(0) // clientCode/width/height
	p.P1(0) // overLayer
	p.P1(0) // comparatorCount
	p.P1(0) // scriptCount
	p.P1(1) // center = true
	p.P1(3) // font = 3
	p.P1(0) // shadowed = false
	p.P4(0x00CCFFAA) // colour
	marginX5 := int16(5)
	marginYNeg3 := int16(-3)
	p.P2(uint16(marginX5))   // marginX = 5
	p.P2(uint16(marginYNeg3)) // marginY = -3
	p.P1(1) // operable = true
	p.PJStrLF("op1")
	p.PJStrLF("op2")
	p.PJStrLF("op3")
	p.PJStrLF("op4")
	p.PJStrLF("op5")

	cfg, err := parseComponentTypes(p, nil)
	if err != nil {
		t.Fatalf("parseComponentTypes: %v", err)
	}
	c := cfg.Configs[12]
	if !c.Center {
		t.Errorf("Center: want true")
	}
	if c.Font != 3 {
		t.Errorf("Font: got %d, want 3", c.Font)
	}
	if c.Shadowed {
		t.Errorf("Shadowed: want false")
	}
	if c.Colour != int32(0x00CCFFAA) {
		t.Errorf("Colour: got %08x, want 00ccffaa", c.Colour)
	}
	if c.MarginX != 5 {
		t.Errorf("MarginX: got %d, want 5", c.MarginX)
	}
	if c.MarginY != -3 {
		t.Errorf("MarginY: got %d, want -3", c.MarginY)
	}
	if !c.Operable {
		t.Errorf("Operable: want true")
	}
	if len(c.Iop) != 5 || c.Iop[0] != "op1" || c.Iop[4] != "op5" {
		t.Errorf("Iop: got %v, want [op1..op5]", c.Iop)
	}
}

func TestComponentDecode_ButtonTarget(t *testing.T) {
	typeBody := []byte{0, 0, 0, 0} // scroll=0, hide=false, childCount=0
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
	client.P2(0)
	client.P2(0)
	client.P2(0)
	client.P1(0)
	client.P1(0)
	client.P1(0)
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

func TestComponentDecode_Button_ToggleSelectPause_Option(t *testing.T) {
	tests := []struct {
		name       string
		buttonType uint8
	}{
		{"Button", Button},
		{"ButtonToggle", ButtonToggle},
		{"ButtonSelect", ButtonSelect},
		{"ButtonPause", ButtonPause},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := packet.NewPacket(nil)
			p.P2(0)
			p.P2(1)
			p.P1(ComTypeLayer)
			p.P1(tt.buttonType)
			p.P2(0)
			p.P2(0)
			p.P2(0)
			p.P1(0)
			p.P1(0)
			p.P1(0)
			// TYPE_LAYER body
			p.P2(0)
			p.P1(0)
			p.P1(0)
			// button option
			p.PJStrLF("MyOption")

			cfg, err := parseComponentTypes(p, nil)
			if err != nil {
				t.Fatalf("parseComponentTypes: %v", err)
			}
			c := cfg.Configs[1]
			if c.Option != "MyOption" {
				t.Errorf("Option: got %q, want MyOption", c.Option)
			}
		})
	}
}

func TestParseComponentTypes_DecodeExtraSetsOverlayAndComName(t *testing.T) {
	client := minimalComponentRecord(t, 10, ComTypeLayer, ButtonNone,
		[]byte{0, 0, 0, 0}, nil)

	server := packet.NewPacket(nil)
	server.P2(0)
	server.P2(10)
	server.PJStrLF("foo")
	server.P1(1)

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

func TestParseComponentTypes_DecodeExtraOnUnknownIdSilentlyDiscarded(t *testing.T) {
	// client has id=10, server refers to id=99 which doesn't exist
	client := minimalComponentRecord(t, 10, ComTypeLayer, ButtonNone,
		[]byte{0, 0, 0, 0}, nil)

	server := packet.NewPacket(nil)
	server.P2(0)
	server.P2(99) // unknown id
	server.PJStrLF("unknown")
	server.P1(0)

	cfg, err := parseComponentTypes(client, server)
	if err != nil {
		t.Fatalf("parseComponentTypes: %v", err)
	}
	if _, ok := cfg.ConfigNames["unknown"]; ok {
		t.Errorf("ConfigNames[unknown]: should not exist")
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

// buildMinimalJagfile constructs a valid single-entry jagfile binary
// and returns its bytes. nameHash is the genHash of the entry name; fileData
// is the raw entry content, which is BZip2-compressed for per-file storage.
// The outer unpackedSize == packedSize so NewJagfile takes the Unpacked=false
// path; each file is BZip2-compressed individually (removeHeader=true so
// BZip2Decompress(prependHeader=true) can reconstruct the stream).
func buildMinimalJagfile(t *testing.T, nameHash uint32, fileData []byte) []byte {
	t.Helper()
	compressed, err := jag.BZip2Compress(fileData, false, true, 1, 0)
	if err != nil {
		t.Fatalf("BZip2Compress: %v", err)
	}

	p := packet.NewPacket(nil)
	p.P3(1)                          // unpackedSize (equal → Unpacked=false path)
	p.P3(1)                          // packedSize
	p.P2(1)                          // fileCount = 1
	p.P4(nameHash)                   // file hash
	p.P3(uint32(len(fileData)))      // file unpackedSize
	p.P3(uint32(len(compressed)))    // file packedSize (compressed size)
	p.Data = append(p.Data, compressed...)
	return p.Data
}

func TestLoadComponentTypes_NoDataEntryInJagfileReturnsEmpty(t *testing.T) {
	// hash("other") = 662708686 (0x278021CE), computed via genHash
	const hashOther = uint32(662708686)
	jagBytes := buildMinimalJagfile(t, hashOther, []byte{0x00})

	dir := t.TempDir()
	clientDir := filepath.Join(dir, "client")
	if err := os.MkdirAll(clientDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(clientDir, "interface"), jagBytes, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := LoadComponentTypes(dir)
	if err != nil {
		t.Fatalf("LoadComponentTypes: %v", err)
	}
	if len(cfg.Configs) != 0 {
		t.Errorf("Configs: got %d entries, want 0 (no data entry)", len(cfg.Configs))
	}
}

func TestLoadComponentTypes_MissingServerInterfaceDatStillDecodes(t *testing.T) {
	// Build a minimal component payload: count header + 1 record (id=0, ComTypeLayer, ButtonNone)
	comPayload := func() []byte {
		p := packet.NewPacket(nil)
		p.P2(0)            // count header
		p.P2(0)            // id = 0
		p.P1(ComTypeLayer) // comType
		p.P1(ButtonNone)   // buttonType
		p.P2(0)            // clientCode
		p.P2(0)            // width
		p.P2(0)            // height
		p.P1(0)            // overLayer = 0 → -1
		p.P1(0)            // comparatorCount = 0
		p.P1(0)            // scriptCount = 0
		// ComTypeLayer body: scroll/hide/childCount
		p.P2(0)
		p.P1(0)
		p.P1(0)
		return p.Data
	}()

	// hash("data") = 8297314 (0x007E9B62), computed via genHash
	const hashData = uint32(8297314)
	jagBytes := buildMinimalJagfile(t, hashData, comPayload)

	dir := t.TempDir()
	clientDir := filepath.Join(dir, "client")
	if err := os.MkdirAll(clientDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(clientDir, "interface"), jagBytes, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// NOTE: no server/interface.dat — LoadComponentTypes must still decode client data.

	cfg, err := LoadComponentTypes(dir)
	if err != nil {
		t.Fatalf("LoadComponentTypes: %v", err)
	}
	if len(cfg.Configs) == 0 || cfg.Configs[0] == nil {
		t.Fatalf("Configs[0]: missing — server absence should not block client decode")
	}
	if cfg.Configs[0].ComType != ComTypeLayer {
		t.Errorf("Configs[0].ComType: got %d, want ComTypeLayer", cfg.Configs[0].ComType)
	}
}

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
