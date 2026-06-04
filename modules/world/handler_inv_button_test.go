package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/inventory"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// invButtonPayload encodes a 6-byte INV_BUTTON1-5 payload (obj:G2 slot:G2 com:G2).
func invButtonPayload(obj, slot, com int) []byte {
	return []byte{
		byte(obj >> 8), byte(obj),
		byte(slot >> 8), byte(slot),
		byte(com >> 8), byte(com),
	}
}

// invButtonDPayload encodes a 6-byte INV_BUTTOND payload (com:G2 slot:G2 targetSlot:G2).
func invButtonDPayload(com, slot, targetSlot int) []byte {
	return []byte{
		byte(com >> 8), byte(com),
		byte(slot >> 8), byte(slot),
		byte(targetSlot >> 8), byte(targetSlot),
	}
}

// setupInvButtonServer returns a Server + Player pre-wired with a world inv at
// invType=93, com=149, source=-1. Item id=555, count=1 lives at slot=3.
func setupInvButtonServer(t *testing.T) (*Server, *Player) {
	t.Helper()
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}
	s.invs = make(map[int]*inventory.Inventory)
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 555, Count: 1}
	s.invs[93] = inv
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.invListenOnCom(93, 149, -1)
	return s, p
}

// --- INV_BUTTON1-5 tests ---

// TestHandleInvButtonDelayed pins that a delayed player causes an early drop
// with no state mutation (mirrors TS InvButtonHandler.ts:14-17).
func TestHandleInvButtonDelayed(t *testing.T) {
	s, p := setupInvButtonServer(t)
	s.currentTick = 5
	p.delayed = true
	p.delayedUntil = 10

	_ = s.handleInvButton(p, invButtonPayload(555, 3, 149), 1)

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (not set when delayed)", p.lastItem)
	}
	if p.lastSlot != -1 {
		t.Errorf("lastSlot: got %d, want -1 (not set when delayed)", p.lastSlot)
	}
}

// TestHandleInvButtonShortPayload pins that payloads under 6 bytes are dropped.
func TestHandleInvButtonShortPayload(t *testing.T) {
	s, p := setupInvButtonServer(t)

	_ = s.handleInvButton(p, []byte{0, 0, 0, 0}, 1)

	if p.lastItem != -1 || p.lastSlot != -1 {
		t.Error("state mutated on short payload")
	}
}

// TestHandleInvButtonNoListener pins that a comId absent from invListeners
// causes a drop (mirrors TS InvButtonHandler.ts:30-36).
func TestHandleInvButtonNoListener(t *testing.T) {
	s, p := setupInvButtonServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		999: {RootLayer: 999, InventoryOptions: []string{"option1", "", "", "", ""}},
	})
	p.tabs[0] = 999

	// com=999 not registered as inv listener
	_ = s.handleInvButton(p, invButtonPayload(555, 3, 999), 1)

	if p.lastItem != -1 {
		t.Error("lastItem mutated despite no listener")
	}
}

// TestHandleInvButtonNilInv pins that a listener whose inv cannot be resolved
// causes a drop (mirrors TS InvButtonHandler.ts:37-41).
func TestHandleInvButtonNilInv(t *testing.T) {
	s, p := setupInvButtonServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, InventoryOptions: []string{"option1", "", "", "", ""}},
	})
	p.tabs[0] = 149
	delete(s.invs, 93) // break the world-inv so resolveListenerInv returns nil

	_ = s.handleInvButton(p, invButtonPayload(555, 3, 149), 1)

	if p.lastItem != -1 {
		t.Error("lastItem mutated despite nil inventory")
	}
}

// TestHandleInvButtonItemMismatch pins that HasAt(slot, obj) false causes a
// drop (mirrors TS InvButtonHandler.ts:43-47: validSlot + hasAt checks).
func TestHandleInvButtonItemMismatch(t *testing.T) {
	s, p := setupInvButtonServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, InventoryOptions: []string{"option1", "", "", "", ""}},
	})
	p.tabs[0] = 149

	// obj=9999 is not at slot 3 (inv has id=555)
	_ = s.handleInvButton(p, invButtonPayload(9999, 3, 149), 1)

	if p.lastItem != -1 {
		t.Error("lastItem mutated despite item mismatch")
	}
}

// TestHandleInvButtonSetsStateAndRunsScript pins the happy path: valid payload
// sets lastItem/lastSlot and fires the matching [inv_button1,<com>] script
// (mirrors TS InvButtonHandler.ts:49-58).
func TestHandleInvButtonSetsStateAndRunsScript(t *testing.T) {
	s, p := setupInvButtonServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, InventoryOptions: []string{"option1", "", "", "", ""}},
	})
	p.tabs[0] = 149
	sf := &script.ScriptFile{
		Name:        "[inv_button1,149]",
		LookupKey:   script.LookupKeyForType(script.TriggerInvButton1, 149),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(sf)

	_ = s.handleInvButton(p, invButtonPayload(555, 3, 149), 1)

	if p.lastItem != 555 {
		t.Errorf("lastItem: got %d, want 555", p.lastItem)
	}
	if p.lastSlot != 3 {
		t.Errorf("lastSlot: got %d, want 3", p.lastSlot)
	}
	// Script returns immediately — no suspension.
	if p.activeScript != nil {
		t.Error("activeScript: want nil after RETURN, got non-nil")
	}
}

// TestHandleInvButtonOpVariant pins that op=2 looks up TriggerInvButton2
// (not TriggerInvButton1). Registers a Button2-specific script and
// confirms it fires for op=2.
func TestHandleInvButtonOpVariant(t *testing.T) {
	s, p := setupInvButtonServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, InventoryOptions: []string{"o1", "o2", "", "", ""}},
	})
	p.tabs[0] = 149
	sf := &script.ScriptFile{
		Name:        "[inv_button2,149]",
		LookupKey:   script.LookupKeyForType(script.TriggerInvButton2, 149),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(sf)

	_ = s.handleInvButton(p, invButtonPayload(555, 3, 149), 2)

	// Confirm handler reached the dispatch block (state was mutated).
	if p.lastItem != 555 {
		t.Errorf("lastItem: got %d, want 555 (handler must reach dispatch branch)", p.lastItem)
	}
	if p.lastSlot != 3 {
		t.Errorf("lastSlot: got %d, want 3 (handler must reach dispatch branch)", p.lastSlot)
	}
	// Script for Button2 fired and returned — no suspension.
	if p.activeScript != nil {
		t.Error("activeScript: want nil after RETURN (Button2 script should have fired)")
	}
}

// --- INV_BUTTOND tests ---

// TestHandleInvButtonDNoListener pins that a comId absent from invListeners
// causes a drop (mirrors TS InvButtonDHandler.ts:18-22).
func TestHandleInvButtonDNoListener(t *testing.T) {
	s, p := setupInvButtonServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		999: {RootLayer: 999, Draggable: true},
	})
	p.tabs[0] = 999

	_ = s.handleInvButtonD(p, invButtonDPayload(999, 3, 5))

	if p.lastSlot != -1 {
		t.Error("lastSlot mutated despite no listener")
	}
}

// TestHandleInvButtonDNilInv pins that an unresolvable inv causes a drop.
func TestHandleInvButtonDNilInv(t *testing.T) {
	s, p := setupInvButtonServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Draggable: true},
	})
	p.tabs[0] = 149
	delete(s.invs, 93)

	_ = s.handleInvButtonD(p, invButtonDPayload(149, 3, 5))

	if p.lastSlot != -1 {
		t.Error("lastSlot mutated despite nil inventory")
	}
}

// TestHandleInvButtonDSlotOOB pins that a slot or targetSlot outside
// inv.Capacity causes a drop (mirrors TS InvButtonDHandler.ts:31-35:
// validSlot(slot) || validSlot(targetSlot) false).
func TestHandleInvButtonDSlotOOB(t *testing.T) {
	s, p := setupInvButtonServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Draggable: true},
	})
	p.tabs[0] = 149

	// inv capacity=28; slot=28 is OOB
	_ = s.handleInvButtonD(p, invButtonDPayload(149, 28, 5))

	if p.lastSlot != -1 {
		t.Error("lastSlot mutated despite OOB slot")
	}
}

// TestHandleInvButtonDSourceEmpty pins that an empty source slot causes a drop
// (mirrors TS InvButtonDHandler.ts:36-39: inv.get(slot) falsy).
func TestHandleInvButtonDSourceEmpty(t *testing.T) {
	s, p := setupInvButtonServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Draggable: true},
	})
	p.tabs[0] = 149

	// slot=10 has no item (only slot 3 is populated)
	_ = s.handleInvButtonD(p, invButtonDPayload(149, 10, 5))

	if p.lastSlot != -1 {
		t.Error("lastSlot mutated despite empty source slot")
	}
}

// TestHandleInvButtonDDelayedRevert pins that a delayed player triggers
// an UpdateInvPartial revert packet and does NOT set lastSlot/lastTargetSlot
// (mirrors TS InvButtonDHandler.ts:41-44: UpdateInvPartial + return false).
func TestHandleInvButtonDDelayedRevert(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.configsView = serverConfigsView{s: s}
	s.invs = make(map[int]*inventory.Inventory)
	s.currentTick = 5
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 555, Count: 1}
	s.invs[93] = inv

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.invListenOnCom(93, 149, -1)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Draggable: true},
	})
	p.tabs[0] = 149
	p.delayed = true
	p.delayedUntil = 10

	received := drainConn(t, cc)
	_ = s.handleInvButtonD(p, invButtonDPayload(149, 3, 5))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Error("delayed INV_BUTTOND: want UpdateInvPartial revert packet, got none")
	}
	if p.lastSlot != -1 {
		t.Errorf("lastSlot: got %d, want -1 (not set when delayed)", p.lastSlot)
	}
	if p.lastTargetSlot != -1 {
		t.Errorf("lastTargetSlot: got %d, want -1 (not set when delayed)", p.lastTargetSlot)
	}
}

// TestHandleInvButtonDSetsStateAndRunsScript pins the happy path: valid payload
// sets lastSlot/lastTargetSlot and fires [inv_buttond,<com>]
// (mirrors TS InvButtonDHandler.ts:46-55).
func TestHandleInvButtonDSetsStateAndRunsScript(t *testing.T) {
	s, p := setupInvButtonServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Draggable: true},
	})
	p.tabs[0] = 149
	sf := &script.ScriptFile{
		Name:        "[inv_buttond,149]",
		LookupKey:   script.LookupKeyForType(script.TriggerInvButtonD, 149),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(sf)

	_ = s.handleInvButtonD(p, invButtonDPayload(149, 3, 5))

	if p.lastSlot != 3 {
		t.Errorf("lastSlot: got %d, want 3", p.lastSlot)
	}
	if p.lastTargetSlot != 5 {
		t.Errorf("lastTargetSlot: got %d, want 5", p.lastTargetSlot)
	}
	if p.activeScript != nil {
		t.Error("activeScript: want nil after RETURN, got non-nil")
	}
}

// --- InvButton component gate + protect tests ---

// TestHandleInvButton_NilComponentRejects pins that registry-empty for comId
// causes the handler to bail before reading lastItem/lastSlot.
func TestHandleInvButton_NilComponentRejects(t *testing.T) {
	s, p := setupInvButtonServer(t)
	// no seedComponentTypes call → registry empty for com=149
	if err := s.handleInvButton(p, invButtonPayload(555, 3, 149), 1); err != nil {
		t.Fatalf("handleInvButton: %v", err)
	}
	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (nil component should reject)", p.lastItem)
	}
}

// TestHandleInvButton_NoIopAtOpRejects pins that com.InventoryOptions[op-1]=="" or out
// of bounds rejects.
func TestHandleInvButton_NoIopAtOpRejects(t *testing.T) {
	s, p := setupInvButtonServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, InventoryOptions: []string{"option1", "", "", "", ""}},
	})
	p.tabs[0] = 149

	// op=2 → InventoryOptions[1]="" → reject
	if err := s.handleInvButton(p, invButtonPayload(555, 3, 149), 2); err != nil {
		t.Fatalf("handleInvButton: %v", err)
	}
	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (InventoryOptions[1]=\"\" should reject)", p.lastItem)
	}
}

// TestHandleInvButton_NotVisibleRejects pins that root-not-in-tabs rejects.
func TestHandleInvButton_NotVisibleRejects(t *testing.T) {
	s, p := setupInvButtonServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 999, InventoryOptions: []string{"option1", "", "", "", ""}},
	})
	// p.tabs left at default — 999 not visible

	if err := s.handleInvButton(p, invButtonPayload(555, 3, 149), 1); err != nil {
		t.Fatalf("handleInvButton: %v", err)
	}
	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (not visible should reject)", p.lastItem)
	}
}

// runInvButtonProtectScript wraps runProtectScript with InvButton specifics.
func runInvButtonProtectScript(t *testing.T, op int, rootOverlay, includeRoot bool) bool {
	t.Helper()
	const comId = 149
	const rootLayer = 100
	return runProtectScript(t,
		script.TriggerInvButton1+script.ServerTriggerType(op-1), comId,
		rootLayer, rootOverlay, includeRoot,
		func(s *Server, p *Player) {
			if s.invs == nil {
				s.invs = make(map[int]*inventory.Inventory)
			}
			inv := inventory.New(93, 28, inventory.StackNormal)
			inv.Items[3] = &inventory.Item{Id: 555, Count: 1}
			s.invs[93] = inv
			p.invListenOnCom(93, comId, -1)
		},
		func(s *Server, p *Player) error {
			return s.handleInvButton(p, invButtonPayload(555, 3, comId), op)
		},
		&objtype.ComponentType{InventoryOptions: []string{"option1", "", "", "", ""}},
	)
}

func TestHandleInvButton_OverlayRootSetsProtectFalse(t *testing.T) {
	if got := runInvButtonProtectScript(t, 1, true, true); got {
		t.Errorf("script suspended: got true, want false (Overlay=true → protect=false)")
	}
}

func TestHandleInvButton_NonOverlayRootSetsProtectTrue(t *testing.T) {
	if got := runInvButtonProtectScript(t, 1, false, true); !got {
		t.Errorf("script suspended: got false, want true (Overlay=false → protect=true)")
	}
}

func TestHandleInvButton_NilRootSetsProtectTrue(t *testing.T) {
	if got := runInvButtonProtectScript(t, 1, false, false); !got {
		t.Errorf("script suspended: got false, want true (nil root → protect=true)")
	}
}

// --- InvButtonD component gate + protect tests ---

func TestHandleInvButtonD_NilComponentRejects(t *testing.T) {
	s, p := setupInvButtonServer(t)
	if err := s.handleInvButtonD(p, invButtonDPayload(149, 3, 5)); err != nil {
		t.Fatalf("handleInvButtonD: %v", err)
	}
	if p.lastSlot != -1 {
		t.Errorf("lastSlot: got %d, want -1 (nil component should reject)", p.lastSlot)
	}
}

func TestHandleInvButtonD_NotDraggableRejects(t *testing.T) {
	s, p := setupInvButtonServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Draggable: false},
	})
	p.tabs[0] = 149
	if err := s.handleInvButtonD(p, invButtonDPayload(149, 3, 5)); err != nil {
		t.Fatalf("handleInvButtonD: %v", err)
	}
	if p.lastSlot != -1 {
		t.Errorf("lastSlot: got %d, want -1 (Draggable=false should reject)", p.lastSlot)
	}
}

func TestHandleInvButtonD_NotVisibleRejects(t *testing.T) {
	s, p := setupInvButtonServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 999, Draggable: true},
	})
	if err := s.handleInvButtonD(p, invButtonDPayload(149, 3, 5)); err != nil {
		t.Fatalf("handleInvButtonD: %v", err)
	}
	if p.lastSlot != -1 {
		t.Errorf("lastSlot: got %d, want -1 (not visible should reject)", p.lastSlot)
	}
}

func runInvButtonDProtectScript(t *testing.T, rootOverlay, includeRoot bool) bool {
	t.Helper()
	const comId = 149
	const rootLayer = 100
	return runProtectScript(t,
		script.TriggerInvButtonD, comId,
		rootLayer, rootOverlay, includeRoot,
		func(s *Server, p *Player) {
			if s.invs == nil {
				s.invs = make(map[int]*inventory.Inventory)
			}
			inv := inventory.New(93, 28, inventory.StackNormal)
			inv.Items[3] = &inventory.Item{Id: 555, Count: 1}
			s.invs[93] = inv
			p.invListenOnCom(93, comId, -1)
		},
		func(s *Server, p *Player) error {
			return s.handleInvButtonD(p, invButtonDPayload(comId, 3, 5))
		},
		&objtype.ComponentType{Draggable: true},
	)
}

func TestHandleInvButtonD_OverlayRootSetsProtectFalse(t *testing.T) {
	if got := runInvButtonDProtectScript(t, true, true); got {
		t.Errorf("script suspended: got true, want false (Overlay=true → protect=false)")
	}
}

func TestHandleInvButtonD_NonOverlayRootSetsProtectTrue(t *testing.T) {
	if got := runInvButtonDProtectScript(t, false, true); !got {
		t.Errorf("script suspended: got false, want true (Overlay=false → protect=true)")
	}
}

func TestHandleInvButtonD_NilRootSetsProtectTrue(t *testing.T) {
	if got := runInvButtonDProtectScript(t, false, false); !got {
		t.Errorf("script suspended: got false, want true (nil root → protect=true)")
	}
}
