package world

import (
	"bytes"
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// opHeldPayload encodes a 6-byte OPHELD1-5 payload (obj:G2 slot:G2 com:G2).
func opHeldPayload(obj, slot, com int) []byte {
	return []byte{
		byte(obj >> 8), byte(obj),
		byte(slot >> 8), byte(slot),
		byte(com >> 8), byte(com),
	}
}

// setupOpHeldServer returns a Server + Player pre-wired with a world inv at
// invType=93, com=149, source=-1. ObjType 555 has IOp = ["op1","","","",""]
// so op=1 is allowed and op=2..5 reject. ComponentType 149 is Operable
// with RootLayer=149 (matches modalMain default).
//
// Item id=555 count=1 lives at inv slot 3.
func setupOpHeldServer(t *testing.T) (*Server, *Player) {
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

	s.objTypes = &objtype.ObjTypeConfigs{
		Configs: make([]*objtype.ObjType, 600),
	}
	s.objTypes.Configs[555] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: 555, DebugName: "test_held"},
		IOp:        []string{"op1", "", "", "", ""},
		Category:   -1,
	}

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.invListenOnCom(93, 149, -1)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Operable: true, Usable: true},
	})
	p.tabs[0] = 149
	p.modalMain = 149 // matches RootLayer ⇒ ClearPendingAction NOT called
	return s, p
}

// TestHandleOpHeld_Delayed pins that a delayed player drops the packet.
// Mirrors TS OpHeldHandler.ts:16-19.
func TestHandleOpHeld_Delayed(t *testing.T) {
	s, p := setupOpHeldServer(t)
	s.currentTick = 5
	p.delayed = true
	p.delayedUntil = 10

	_ = handleOpHeld1(p, opHeldPayload(555, 3, 149))

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (delayed must reject)", p.lastItem)
	}
}

// TestHandleOpHeld_ShortPayload pins that <6 bytes drops.
func TestHandleOpHeld_ShortPayload(t *testing.T) {
	s, p := setupOpHeldServer(t)
	_ = s // silence unused
	if err := handleOpHeld1(p, []byte{0, 0, 0}); err != nil {
		t.Fatalf("handleOpHeld1: %v", err)
	}
	if p.lastItem != -1 {
		t.Error("lastItem mutated on short payload")
	}
}

// TestHandleOpHeld_NilComponent pins that a comId not in the registry rejects.
func TestHandleOpHeld_NilComponent(t *testing.T) {
	s, p := setupOpHeldServer(t)
	delete(p.invListeners, 149)        // drop the listener so the registry-empty path is exercised cleanly
	p.invListenOnCom(93, 999, -1)      // listener for com=999 (not seeded)
	p.tabs[0] = 999
	_ = s

	_ = handleOpHeld1(p, opHeldPayload(555, 3, 999))

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (nil component must reject)", p.lastItem)
	}
}

// TestHandleOpHeld_NotOperable pins that com.Operable=false rejects.
// Mirrors TS OpHeldHandler.ts:21-23.
func TestHandleOpHeld_NotOperable(t *testing.T) {
	s, p := setupOpHeldServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Operable: false},
	})

	_ = handleOpHeld1(p, opHeldPayload(555, 3, 149))

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (Operable=false must reject)", p.lastItem)
	}
}

// TestHandleOpHeld_NotVisible pins !IsComponentVisible reject.
// Mirrors TS OpHeldHandler.ts:25-27.
func TestHandleOpHeld_NotVisible(t *testing.T) {
	s, p := setupOpHeldServer(t)
	_ = s
	p.tabs[0] = 0 // clear tab assignment so the component is not visible

	_ = handleOpHeld1(p, opHeldPayload(555, 3, 149))

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (not visible must reject)", p.lastItem)
	}
}

// TestHandleOpHeld_NoListener pins that a comId without an inv listener rejects.
// Mirrors TS OpHeldHandler.ts:30-35.
func TestHandleOpHeld_NoListener(t *testing.T) {
	s, p := setupOpHeldServer(t)
	_ = s
	delete(p.invListeners, 149)

	_ = handleOpHeld1(p, opHeldPayload(555, 3, 149))

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (no listener must reject)", p.lastItem)
	}
}

// TestHandleOpHeld_NilInv pins that resolveListenerInv→nil rejects.
// Mirrors TS OpHeldHandler.ts:30-35 second arm.
func TestHandleOpHeld_NilInv(t *testing.T) {
	s, p := setupOpHeldServer(t)
	delete(s.invs, 93)

	_ = handleOpHeld1(p, opHeldPayload(555, 3, 149))

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (nil inv must reject)", p.lastItem)
	}
}

// TestHandleOpHeld_HasAtFalse pins inv.HasAt(slot, obj)=false reject.
// Mirrors TS OpHeldHandler.ts:37-43.
func TestHandleOpHeld_HasAtFalse(t *testing.T) {
	s, p := setupOpHeldServer(t)
	_ = s

	// Wrong slot — slot 3 has 555, slot 4 is empty.
	_ = handleOpHeld1(p, opHeldPayload(555, 4, 149))

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (HasAt false must reject)", p.lastItem)
	}
}

// TestHandleOpHeld_ObjTypeNil pins that an obj id outside the loaded ObjType
// table rejects (goscape defensive; TS throws).
func TestHandleOpHeld_ObjTypeNil(t *testing.T) {
	s, p := setupOpHeldServer(t)
	s.objTypes.Configs[555] = nil

	_ = handleOpHeld1(p, opHeldPayload(555, 3, 149))

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (nil ObjType must reject)", p.lastItem)
	}
}

// TestHandleOpHeld_IOpEmpty pins that objType.IOp[op-1] == "" rejects.
// Mirrors TS OpHeldHandler.ts:45-48.
func TestHandleOpHeld_IOpEmpty(t *testing.T) {
	s, p := setupOpHeldServer(t)
	_ = s

	// op=2 → IOp[1] is "" in the fixture.
	_ = handleOpHeld2(p, opHeldPayload(555, 3, 149))

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (IOp[op-1]=='' must reject)", p.lastItem)
	}
}

// TestHandleOpHeld_HappyPath pins the success path: state mutated,
// trigger registered, script fired, mask emitted.
// Mirrors TS OpHeldHandler.ts:51-73.
func TestHandleOpHeld_HappyPath(t *testing.T) {
	s, p := setupOpHeldServer(t)
	sf := &script.ScriptFile{
		Name:             "[opheld1,555]",
		LookupKey:        script.LookupKeyForType(script.TriggerOpHeld1, 555),
		Opcodes:          []script.Opcode{script.OpReturn},
		IntOperands:      []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(sf)

	// Sentinel pre-state.
	p.faceEntity = 7 // any non-(-1) value
	p.moveClickRequest = true
	p.masks = 0

	_ = handleOpHeld1(p, opHeldPayload(555, 3, 149))

	if p.lastItem != 555 {
		t.Errorf("lastItem: got %d, want 555", p.lastItem)
	}
	if p.lastSlot != 3 {
		t.Errorf("lastSlot: got %d, want 3", p.lastSlot)
	}
	if p.moveClickRequest {
		t.Error("moveClickRequest: want false post-fire")
	}
	if p.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1", p.faceEntity)
	}
	if p.masks&p.entitymask == 0 {
		t.Errorf("masks: entitymask bit not set (got %d)", p.masks)
	}
	if p.activeScript != nil {
		t.Error("activeScript: want nil after RETURN, got non-nil")
	}
}

// TestHandleOpHeld_Op2VariantFires pins that op=2 looks up TriggerOpHeld2.
// IOp[0]=="op1", IOp[1]=="op2" — both populated for this test.
func TestHandleOpHeld_Op2VariantFires(t *testing.T) {
	s, p := setupOpHeldServer(t)
	s.objTypes.Configs[555].IOp = []string{"op1", "op2", "", "", ""}
	sf := &script.ScriptFile{
		Name:             "[opheld2,555]",
		LookupKey:        script.LookupKeyForType(script.TriggerOpHeld2, 555),
		Opcodes:          []script.Opcode{script.OpReturn},
		IntOperands:      []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(sf)

	_ = handleOpHeld2(p, opHeldPayload(555, 3, 149))

	if p.lastItem != 555 {
		t.Errorf("lastItem: got %d, want 555 (op=2 dispatch must reach state mutation)", p.lastItem)
	}
}

// TestHandleOpHeld_RootLayerMatchesModal_NoClearPending pins that when
// com.RootLayer == p.modalMain, ClearPendingAction is NOT called.
// Sentinel: pre-set p.target to a non-nil entity and verify it survives
// the handler call. Mirrors TS OpHeldHandler.ts:54-56 negative arm.
func TestHandleOpHeld_RootLayerMatchesModal_NoClearPending(t *testing.T) {
	s, p := setupOpHeldServer(t)
	// setupOpHeldServer already sets p.modalMain = 149 (matches com.RootLayer = 149)
	sf := &script.ScriptFile{
		Name:             "[opheld1,555]",
		LookupKey:        script.LookupKeyForType(script.TriggerOpHeld1, 555),
		Opcodes:          []script.Opcode{script.OpReturn},
		IntOperands:      []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(sf)

	// Sentinel: a stale interaction target. ClearPendingAction would
	// null this; we verify it survives.
	p.target = p // any non-nil entity
	p.opcalled = true

	_ = handleOpHeld1(p, opHeldPayload(555, 3, 149))

	if p.target == nil {
		t.Error("ClearPendingAction was called but rootLayer matched modalMain (should be no-op)")
	}
}

// TestHandleOpHeld_RootLayerMismatch_ClearsPending pins that when
// com.RootLayer != p.modalMain, ClearPendingAction IS called.
// Mirrors TS OpHeldHandler.ts:54-56 positive arm.
func TestHandleOpHeld_RootLayerMismatch_ClearsPending(t *testing.T) {
	s, p := setupOpHeldServer(t)
	p.modalMain = 999 // != com.RootLayer (149)
	sf := &script.ScriptFile{
		Name:             "[opheld1,555]",
		LookupKey:        script.LookupKeyForType(script.TriggerOpHeld1, 555),
		Opcodes:          []script.Opcode{script.OpReturn},
		IntOperands:      []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(sf)

	p.target = p
	p.opcalled = true

	_ = handleOpHeld1(p, opHeldPayload(555, 3, 149))

	if p.target != nil {
		t.Error("ClearPendingAction was NOT called but rootLayer mismatched modalMain (should clear)")
	}
}

// opHeldTPayload encodes an 8-byte OPHELDT payload
// (obj:G2 slot:G2 com:G2 spellCom:G2).
func opHeldTPayload(obj, slot, com, spellCom int) []byte {
	return []byte{
		byte(obj >> 8), byte(obj),
		byte(slot >> 8), byte(slot),
		byte(com >> 8), byte(com),
		byte(spellCom >> 8), byte(spellCom),
	}
}

// setupOpHeldTServer extends setupOpHeldServer with a spell component
// at id=200 that has ActionTarget&HELD set and is visible.
func setupOpHeldTServer(t *testing.T) (*Server, *Player) {
	t.Helper()
	s, p := setupOpHeldServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Operable: true, Usable: true},
		200: {RootLayer: 200, ActionTarget: objtype.ComActionTargetHeld},
	})
	p.tabs[1] = 200 // spell tab visible
	return s, p
}

func TestHandleOpHeldT_Delayed(t *testing.T) {
	s, p := setupOpHeldTServer(t)
	s.currentTick = 5
	p.delayed = true
	p.delayedUntil = 10
	_ = handleOpHeldT(p, opHeldTPayload(555, 3, 149, 200))
	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (delayed reject)", p.lastItem)
	}
}

func TestHandleOpHeldT_ShortPayload(t *testing.T) {
	_, p := setupOpHeldTServer(t)
	_ = handleOpHeldT(p, []byte{0, 0, 0, 0, 0, 0, 0}) // 7 bytes
	if p.lastItem != -1 {
		t.Error("lastItem mutated on short payload")
	}
}

// TS OpHeldTHandler.ts:21-23 — spellCom: nil or actionTarget&HELD == 0.
func TestHandleOpHeldT_SpellComMissingHeldFlag(t *testing.T) {
	s, p := setupOpHeldTServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Operable: true, Usable: true},
		200: {RootLayer: 200, ActionTarget: 0}, // HELD flag clear
	})
	p.tabs[1] = 200
	_ = handleOpHeldT(p, opHeldTPayload(555, 3, 149, 200))
	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (spellCom missing HELD flag)", p.lastItem)
	}
}

// TS OpHeldTHandler.ts:30-32 — com: nil or !Usable.
func TestHandleOpHeldT_ComNotUsable(t *testing.T) {
	s, p := setupOpHeldTServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Operable: true, Usable: false}, // Usable cleared
		200: {RootLayer: 200, ActionTarget: objtype.ComActionTargetHeld},
	})
	p.tabs[1] = 200
	_ = handleOpHeldT(p, opHeldTPayload(555, 3, 149, 200))
	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (com not Usable)", p.lastItem)
	}
}

// TestHandleOpHeldT_HappyPath pins the success path: state mutated,
// ClearPendingAction unconditional, faceEntity=-1, mask emitted.
// Mirrors TS OpHeldTHandler.ts:57-73.
func TestHandleOpHeldT_HappyPath(t *testing.T) {
	s, p := setupOpHeldTServer(t)
	sf := &script.ScriptFile{
		Name:             "[opheldt,200]",
		LookupKey:        script.LookupKeyForType(script.TriggerOpHeldT, 200),
		Opcodes:          []script.Opcode{script.OpReturn},
		IntOperands:      []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(sf)

	p.faceEntity = 7
	p.target = p // sentinel for unconditional ClearPendingAction
	p.opcalled = true
	p.masks = 0

	_ = handleOpHeldT(p, opHeldTPayload(555, 3, 149, 200))

	if p.lastItem != 555 {
		t.Errorf("lastItem: got %d, want 555", p.lastItem)
	}
	if p.lastSlot != 3 {
		t.Errorf("lastSlot: got %d, want 3", p.lastSlot)
	}
	if p.target != nil {
		t.Error("ClearPendingAction must be unconditional in OPHELDT (target should be nil)")
	}
	if p.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1", p.faceEntity)
	}
	if p.masks&p.entitymask == 0 {
		t.Error("masks: entitymask bit not set")
	}
}

// TestHandleOpHeldT_NoScript_NothingInteresting pins that when no
// [opheldt,spellComId] script is registered, "Nothing interesting happens."
// is sent to the client. Mirrors TS OpHeldTHandler.ts:71-73.
func TestHandleOpHeldT_NoScript_NothingInteresting(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}
	s.invs = make(map[int]*inventory.Inventory)
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 555, Count: 1}
	s.invs[93] = inv
	s.objTypes = &objtype.ObjTypeConfigs{Configs: make([]*objtype.ObjType, 600)}
	s.objTypes.Configs[555] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: 555},
		IOp:        []string{"op1", "", "", "", ""},
		Category:   -1,
	}

	p.client.server = s
	p.invListenOnCom(93, 149, -1)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Operable: true, Usable: true},
		200: {RootLayer: 200, ActionTarget: objtype.ComActionTargetHeld},
	})
	p.tabs[0] = 149
	p.tabs[1] = 200

	received := drainConn(t, cc)
	_ = handleOpHeldT(p, opHeldTPayload(555, 3, 149, 200))
	p.client.flushWrite()
	got := <-received
	if !bytes.Contains(got, []byte("Nothing interesting happens.")) {
		t.Errorf("want \"Nothing interesting happens.\" in drained bytes, got %x", got)
	}
}
