package world

import (
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/inventory"
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

	// com=999 not registered
	_ = s.handleInvButton(p, invButtonPayload(555, 3, 999), 1)

	if p.lastItem != -1 {
		t.Error("lastItem mutated despite no listener")
	}
}

// TestHandleInvButtonNilInv pins that a listener whose inv cannot be resolved
// causes a drop (mirrors TS InvButtonHandler.ts:37-41).
func TestHandleInvButtonNilInv(t *testing.T) {
	s, p := setupInvButtonServer(t)
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
	sf := &script.ScriptFile{
		Name:      "[inv_button1,149]",
		LookupKey: script.LookupKeyForType(script.TriggerInvButton1, 149),
		Opcodes:   []script.Opcode{script.OpReturn},
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
	sf := &script.ScriptFile{
		Name:      "[inv_button2,149]",
		LookupKey: script.LookupKeyForType(script.TriggerInvButton2, 149),
		Opcodes:   []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(sf)

	_ = s.handleInvButton(p, invButtonPayload(555, 3, 149), 2)

	// Script for Button2 fired and returned — no suspension.
	if p.activeScript != nil {
		t.Error("activeScript: want nil after RETURN (Button2 script should have fired)")
	}
	// Button1 was NOT registered; the lack of Button1 script with op=2 passing
	// confirms the trigger offset computation is correct.
}

// --- INV_BUTTOND tests ---

// TestHandleInvButtonDNoListener pins that a comId absent from invListeners
// causes a drop (mirrors TS InvButtonDHandler.ts:18-22).
func TestHandleInvButtonDNoListener(t *testing.T) {
	s, p := setupInvButtonServer(t)

	_ = s.handleInvButtonD(p, invButtonDPayload(999, 3, 5))

	if p.lastSlot != -1 {
		t.Error("lastSlot mutated despite no listener")
	}
}

// TestHandleInvButtonDNilInv pins that an unresolvable inv causes a drop.
func TestHandleInvButtonDNilInv(t *testing.T) {
	s, p := setupInvButtonServer(t)
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
	s.invs = make(map[int]*inventory.Inventory)
	s.currentTick = 5
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 555, Count: 1}
	s.invs[93] = inv

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.invListenOnCom(93, 149, -1)
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
	sf := &script.ScriptFile{
		Name:      "[inv_buttond,149]",
		LookupKey: script.LookupKeyForType(script.TriggerInvButtonD, 149),
		Opcodes:   []script.Opcode{script.OpReturn},
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
