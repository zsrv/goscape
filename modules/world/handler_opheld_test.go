package world

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/zsrv/goscape/pkg/inventory"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
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
// so op=1 is allowed and op=2..5 reject. ComponentType 149 is Interactable
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
		149: {RootLayer: 149, Interactable: true, Usable: true},
	})
	p.tabs[0] = 149
	p.modalMain = 149 // matches RootLayer ⇒ ClearPendingAction NOT called
	return s, p
}

// TestHandleOpHeld_Delayed pins that a delayed player drops the packet AFTER
// validation passes. In 244, delayed check moves to after all validation gates,
// and its reject does NOT call clearPendingAction.
// Mirrors TS OpHeldHandler.ts (244): delayed check after inv validation, returns
// false without calling clearPendingAction.
func TestHandleOpHeld_Delayed(t *testing.T) {
	s, p := setupOpHeldServer(t)
	s.currentTick = 5
	p.delayed = true
	p.delayedUntil = 10

	// Arm a sentinel target to detect whether ClearPendingAction is called.
	p.target = p
	p.opcalled = true

	_ = handleOpHeld1(p, opHeldPayload(555, 3, 149))

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (delayed must reject)", p.lastItem)
	}
	// 244: delayed reject must NOT call clearPendingAction (target survives).
	if p.target == nil {
		t.Error("244: delayed-only reject must NOT call clearPendingAction (target was cleared)")
	}
}

// TestHandleOpHeld_InvalidCom_WithDelayed_ClearsPending pins that invalid
// component validation rejects even when p.delayed=true AND calls
// clearPendingAction. In 244, validation runs before the delayed check.
// Mirrors TS OpHeldHandler.ts (244): com validation first, clearPendingAction on reject.
func TestHandleOpHeld_InvalidCom_WithDelayed_ClearsPending(t *testing.T) {
	s, p := setupOpHeldServer(t)
	s.currentTick = 5
	p.delayed = true
	p.delayedUntil = 10

	// Seed a non-interactable component.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: false},
	})

	p.target = p
	p.opcalled = true

	_ = handleOpHeld1(p, opHeldPayload(555, 3, 149))

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (invalid com must reject)", p.lastItem)
	}
	// 244: validation rejects call clearPendingAction even when delayed.
	if p.target != nil {
		t.Error("244: invalid com reject must call clearPendingAction (target should be nil)")
	}
}

// TestHandleOpHeld_NoListener_ClearsPendingAction pins that the listener-not-found
// reject calls clearPendingAction. In 244 this is explicit.
// Mirrors TS OpHeldHandler.ts (244): listener check calls clearPendingAction on reject.
func TestHandleOpHeld_NoListener_ClearsPendingAction(t *testing.T) {
	_, p := setupOpHeldServer(t)
	delete(p.invListeners, 149)

	p.target = p
	p.opcalled = true

	_ = handleOpHeld1(p, opHeldPayload(555, 3, 149))

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (no listener must reject)", p.lastItem)
	}
	if p.target != nil {
		t.Error("244: no-listener reject must call clearPendingAction (target should be nil)")
	}
}

// TestHandleOpHeld_NilInv_ClearsPendingAction pins that inv-unresolved reject
// calls clearPendingAction (244 change from 225).
func TestHandleOpHeld_NilInv_ClearsPendingAction(t *testing.T) {
	s, p := setupOpHeldServer(t)
	delete(s.invs, 93)

	p.target = p
	p.opcalled = true

	_ = handleOpHeld1(p, opHeldPayload(555, 3, 149))

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (nil inv must reject)", p.lastItem)
	}
	if p.target != nil {
		t.Error("244: nil-inv reject must call clearPendingAction (target should be nil)")
	}
}

// TestHandleOpHeld_HasAtFalse_ClearsPendingAction pins that HasAt-false reject
// calls clearPendingAction (244 change from 225).
func TestHandleOpHeld_HasAtFalse_ClearsPendingAction(t *testing.T) {
	_, p := setupOpHeldServer(t)

	p.target = p
	p.opcalled = true

	// Wrong slot — slot 3 has 555, slot 4 is empty.
	_ = handleOpHeld1(p, opHeldPayload(555, 4, 149))

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (HasAt false must reject)", p.lastItem)
	}
	if p.target != nil {
		t.Error("244: HasAt-false reject must call clearPendingAction (target should be nil)")
	}
}

// TestHandleOpHeld_Op5_SkipsIopValidation pins that op=5 bypasses the iop check.
// In 244, iop validation is skipped for op=5 (TS OpHeldHandler.ts: "message.op !== 5").
// IOp[4]="" but op=5 should still reach state mutation.
func TestHandleOpHeld_Op5_SkipsIopValidation(t *testing.T) {
	s, p := setupOpHeldServer(t)
	// IOp has all slots empty except slot 0; op=5 maps to IOp[4]="" but must NOT reject.
	s.objTypes.Configs[555].IOp = []string{"op1", "", "", "", ""}

	_ = handleOpHeld5(p, opHeldPayload(555, 3, 149))

	if p.lastItem != 555 {
		t.Errorf("lastItem: got %d, want 555 (op=5 must skip iop validation)", p.lastItem)
	}
}

// TestHandleOpHeld_Op5_NilIop_SkipsValidation pins that op=5 also skips when iop is nil.
// Mirrors TS: "message.op !== 5" entirely bypasses the iop gate.
func TestHandleOpHeld_Op5_NilIop_SkipsValidation(t *testing.T) {
	s, p := setupOpHeldServer(t)
	s.objTypes.Configs[555].IOp = nil // nil iop

	_ = handleOpHeld5(p, opHeldPayload(555, 3, 149))

	if p.lastItem != 555 {
		t.Errorf("lastItem: got %d, want 555 (op=5 must skip nil iop validation)", p.lastItem)
	}
}

// TestHandleOpHeld_IopNil_Rejects_WithClearPendingAction pins that for op≠5,
// nil IOp rejects and calls clearPendingAction (244 iop condition change).
// TS: "(type.iop && !type.iop[message.op - 1]) || !type.iop"
func TestHandleOpHeld_IopNil_Rejects_WithClearPendingAction(t *testing.T) {
	s, p := setupOpHeldServer(t)
	s.objTypes.Configs[555].IOp = nil

	p.target = p
	p.opcalled = true

	_ = handleOpHeld1(p, opHeldPayload(555, 3, 149))

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (nil IOp op≠5 must reject)", p.lastItem)
	}
	if p.target != nil {
		t.Error("244: nil-IOp reject must call clearPendingAction")
	}
}

// TestHandleOpHeld_IopEmpty_ClearsPendingAction pins that op≠5 with empty iop entry
// rejects AND calls clearPendingAction (244 change — 225 did not call it here).
func TestHandleOpHeld_IopEmpty_ClearsPendingAction(t *testing.T) {
	_, p := setupOpHeldServer(t)

	p.target = p
	p.opcalled = true

	// op=2 → IOp[1] is "" in the fixture.
	_ = handleOpHeld2(p, opHeldPayload(555, 3, 149))

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (IOp[op-1]=='' must reject)", p.lastItem)
	}
	if p.target != nil {
		t.Error("244: empty-IOp reject must call clearPendingAction")
	}
}

// TestHandleOpHeld_IopShortSlice_Rejects pins that op≠5 rejects without panic when
// IOp is shorter than op (e.g. IOp length 2, op=3). The TS-faithful guard
// "len(IOp) < op" absorbs this case that the old "len(IOp) == 0" check missed.
// TS: "(type.iop && !type.iop[op-1]) || !type.iop" — JS undefined-falsy absorption.
func TestHandleOpHeld_IopShortSlice_Rejects(t *testing.T) {
	s, p := setupOpHeldServer(t)
	// IOp length 2: slots 0 and 1 only. op=3 would index IOp[2] — OOB without the fix.
	s.objTypes.Configs[555].IOp = []string{"op1", "op2"}

	p.target = p
	p.opcalled = true

	_ = handleOpHeld3(p, opHeldPayload(555, 3, 149))

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (short-slice IOp op=3 must reject)", p.lastItem)
	}
	if p.target != nil {
		t.Error("244: short-slice IOp reject must call clearPendingAction (no panic)")
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
	delete(p.invListeners, 149)   // drop the listener so the registry-empty path is exercised cleanly
	p.invListenOnCom(93, 999, -1) // listener for com=999 (not seeded)
	p.tabs[0] = 999
	_ = s

	_ = handleOpHeld1(p, opHeldPayload(555, 3, 999))

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (nil component must reject)", p.lastItem)
	}
}

// TestHandleOpHeld_NotInteractable pins that com.Interactable=false rejects.
// Mirrors TS OpHeldHandler.ts:21-23.
func TestHandleOpHeld_NotInteractable(t *testing.T) {
	s, p := setupOpHeldServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: false},
	})

	_ = handleOpHeld1(p, opHeldPayload(555, 3, 149))

	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (Interactable=false must reject)", p.lastItem)
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
		Name:        "[opheld1,555]",
		LookupKey:   script.LookupKeyForType(script.TriggerOpHeld1, 555),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
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
	// A8/ee28c1aa @2e3bcf43: the handler's `faceEntity = -1; masks |=
	// entitymask` pair was REMOVED upstream (TS OpHeldHandler.ts diff) —
	// the handler must no longer touch facing (the per-tick
	// setFaceEntity() derivation owns it; see face_entity.go).
	if p.faceEntity != 7 {
		t.Errorf("faceEntity: got %d, want 7 (handler must NOT touch facing)", p.faceEntity)
	}
	if p.masks&p.entitymask != 0 {
		t.Errorf("masks: entitymask bit must NOT be set by the handler (got %d)", p.masks)
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
		Name:        "[opheld2,555]",
		LookupKey:   script.LookupKeyForType(script.TriggerOpHeld2, 555),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
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
		Name:        "[opheld1,555]",
		LookupKey:   script.LookupKeyForType(script.TriggerOpHeld1, 555),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
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
		Name:        "[opheld1,555]",
		LookupKey:   script.LookupKeyForType(script.TriggerOpHeld1, 555),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
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
		149: {RootLayer: 149, Interactable: true, Usable: true},
		200: {RootLayer: 200, ActionTarget: objtype.ComActionTargetHeld},
	})
	p.tabs[1] = 200 // spell tab visible
	return s, p
}

// TestHandleOpHeldT_Delayed pins that delayed-only reject (all validation passes)
// does NOT call clearPendingAction. In 244, delayed check moves to after all
// validation gates. Mirrors TS OpHeldTHandler.ts (244).
func TestHandleOpHeldT_Delayed(t *testing.T) {
	s, p := setupOpHeldTServer(t)
	s.currentTick = 5
	p.delayed = true
	p.delayedUntil = 10

	// Sentinel: detect clearPendingAction.
	p.target = p
	p.opcalled = true

	_ = handleOpHeldT(p, opHeldTPayload(555, 3, 149, 200))
	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (delayed reject)", p.lastItem)
	}
	// 244: delayed-only reject must NOT call clearPendingAction.
	if p.target == nil {
		t.Error("244: delayed-only reject must NOT call clearPendingAction")
	}
}

// TestHandleOpHeldT_InvalidCom_WithDelayed_ClearsPending pins that invalid com
// validation rejects even when delayed, AND calls clearPendingAction (244 change).
func TestHandleOpHeldT_InvalidCom_WithDelayed_ClearsPending(t *testing.T) {
	s, p := setupOpHeldTServer(t)
	s.currentTick = 5
	p.delayed = true
	p.delayedUntil = 10
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: false}, // not interactable
		200: {RootLayer: 200, ActionTarget: objtype.ComActionTargetHeld},
	})
	p.target = p
	p.opcalled = true

	_ = handleOpHeldT(p, opHeldTPayload(555, 3, 149, 200))
	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (invalid com must reject even when delayed)", p.lastItem)
	}
	if p.target != nil {
		t.Error("244: invalid com reject must call clearPendingAction")
	}
}

func TestHandleOpHeldT_ShortPayload(t *testing.T) {
	_, p := setupOpHeldTServer(t)
	_ = handleOpHeldT(p, []byte{0, 0, 0, 0, 0, 0, 0}) // 7 bytes
	if p.lastItem != -1 {
		t.Error("lastItem mutated on short payload")
	}
}

// TS OpHeldTHandler.ts (244): com checked first with interactable, then spellCom.
// Mirrors the 244 gate order change: com.interactable before spellCom check.
func TestHandleOpHeldT_SpellComMissingHeldFlag(t *testing.T) {
	s, p := setupOpHeldTServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true, Usable: true},
		200: {RootLayer: 200, ActionTarget: 0}, // HELD flag clear
	})
	p.tabs[1] = 200
	_ = handleOpHeldT(p, opHeldTPayload(555, 3, 149, 200))
	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (spellCom missing HELD flag)", p.lastItem)
	}
}

// TestHandleOpHeldT_SpellComMissingHeldFlag_ClearsPending pins that the spellCom
// validation reject calls clearPendingAction (244 change).
func TestHandleOpHeldT_SpellComMissingHeldFlag_ClearsPending(t *testing.T) {
	s, p := setupOpHeldTServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true, Usable: true},
		200: {RootLayer: 200, ActionTarget: 0}, // HELD flag clear
	})
	p.tabs[1] = 200
	p.target = p
	p.opcalled = true
	_ = handleOpHeldT(p, opHeldTPayload(555, 3, 149, 200))
	if p.target != nil {
		t.Error("244: spellCom HELD-flag reject must call clearPendingAction")
	}
}

// TestHandleOpHeldT_ComNotInteractable pins that com.Interactable=false rejects.
// In 244, com is checked first (before spellCom) for Interactable (not Usable).
// TS OpHeldTHandler.ts (244): "!com.interactable" replaces "!com.usable".
func TestHandleOpHeldT_ComNotInteractable(t *testing.T) {
	s, p := setupOpHeldTServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: false, Usable: true}, // Interactable cleared
		200: {RootLayer: 200, ActionTarget: objtype.ComActionTargetHeld},
	})
	p.tabs[1] = 200
	p.target = p
	p.opcalled = true
	_ = handleOpHeldT(p, opHeldTPayload(555, 3, 149, 200))
	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (com not Interactable)", p.lastItem)
	}
	if p.target != nil {
		t.Error("244: com not-Interactable reject must call clearPendingAction")
	}
}

// TestHandleOpHeldT_NoListener_ClearsPending pins that listener-not-found
// calls clearPendingAction (244 change).
func TestHandleOpHeldT_NoListener_ClearsPending(t *testing.T) {
	_, p := setupOpHeldTServer(t)
	delete(p.invListeners, 149)
	p.target = p
	p.opcalled = true
	_ = handleOpHeldT(p, opHeldTPayload(555, 3, 149, 200))
	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (no listener must reject)", p.lastItem)
	}
	if p.target != nil {
		t.Error("244: no-listener reject must call clearPendingAction")
	}
}

// TestHandleOpHeldT_NilInv_ClearsPending pins that inv-unresolved calls clearPendingAction (244).
func TestHandleOpHeldT_NilInv_ClearsPending(t *testing.T) {
	s, p := setupOpHeldTServer(t)
	delete(s.invs, 93)
	p.target = p
	p.opcalled = true
	_ = handleOpHeldT(p, opHeldTPayload(555, 3, 149, 200))
	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (nil inv must reject)", p.lastItem)
	}
	if p.target != nil {
		t.Error("244: nil-inv reject must call clearPendingAction")
	}
}

// TestHandleOpHeldT_HappyPath pins the success path: state mutated,
// ClearPendingAction unconditional, facing UNTOUCHED (A8/ee28c1aa
// @2e3bcf43 removed the handler's faceEntity=-1 + mask pair).
// Mirrors TS OpHeldTHandler.ts:57-73.
func TestHandleOpHeldT_HappyPath(t *testing.T) {
	s, p := setupOpHeldTServer(t)
	sf := &script.ScriptFile{
		Name:        "[opheldt,200]",
		LookupKey:   script.LookupKeyForType(script.TriggerOpHeldT, 200),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
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
	// A8/ee28c1aa @2e3bcf43: handler must no longer touch facing.
	if p.faceEntity != 7 {
		t.Errorf("faceEntity: got %d, want 7 (handler must NOT touch facing)", p.faceEntity)
	}
	if p.masks&p.entitymask != 0 {
		t.Error("masks: entitymask bit must NOT be set by the handler")
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
		149: {RootLayer: 149, Interactable: true, Usable: true},
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

// opHeldUPayload encodes a 12-byte OPHELDU payload.
// Wire format: obj:G2 | slot:G2 | com:G2 | useObj:G2 | useSlot:G2 | useCom:G2.
func opHeldUPayload(obj, slot, com, useObj, useSlot, useCom int) []byte {
	return []byte{
		byte(obj >> 8), byte(obj),
		byte(slot >> 8), byte(slot),
		byte(com >> 8), byte(com),
		byte(useObj >> 8), byte(useObj),
		byte(useSlot >> 8), byte(useSlot),
		byte(useCom >> 8), byte(useCom),
	}
}

// setupOpHeldUServer extends setupOpHeldServer with a second item at
// slot 5 (id=777) so item-on-item swaps can be pinned. Both 555 and 777
// have Category=-1 so category-fallback arms are inactive by default.
// The ObjType slice is grown to 800 to accommodate id=777.
func setupOpHeldUServer(t *testing.T) (*Server, *Player) {
	t.Helper()
	s, p := setupOpHeldServer(t)
	// Grow the configs slice so id=777 is in range.
	grown := make([]*objtype.ObjType, 800)
	copy(grown, s.objTypes.Configs)
	s.objTypes.Configs = grown
	s.invs[93].Items[5] = &inventory.Item{Id: 777, Count: 1}
	s.objTypes.Configs[777] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: 777, DebugName: "test_held2"},
		IOp:        []string{"op1", "", "", "", ""},
		Category:   -1,
	}
	return s, p
}

func TestHandleOpHeldU_Delayed(t *testing.T) {
	s, p := setupOpHeldUServer(t)
	s.currentTick = 5
	p.delayed = true
	p.delayedUntil = 10
	_ = handleOpHeldU(p, opHeldUPayload(555, 3, 149, 777, 5, 149))
	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (delayed)", p.lastItem)
	}
}

func TestHandleOpHeldU_ShortPayload(t *testing.T) {
	_, p := setupOpHeldUServer(t)
	_ = handleOpHeldU(p, []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}) // 11 bytes
	if p.lastItem != -1 {
		t.Error("lastItem mutated on short payload")
	}
}

// TestHandleOpHeldU_ComMismatch_244_Allowed pins that 244 REMOVED the comId==useComId
// check. In 225 this was a hard reject; in 244 (TS OpHeldUHandler.ts 9aadcec4)
// the check is gone entirely — different components are now valid.
// We verify the packet proceeds past this former gate (com=149, useCom=200;
// both must be seeded and visible for progress — the test verifies no early reject
// from the mismatch check, though later gates may still reject on visibility).
func TestHandleOpHeldU_ComMismatch_244_Allowed(t *testing.T) {
	s, p := setupOpHeldUServer(t)
	// Seed useCom=200 as interactable and visible so the packet can proceed past
	// the com/useCom gates and reach later validation.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true, Usable: true},
		200: {RootLayer: 200, Interactable: true, Usable: true},
	})
	p.invListenOnCom(93, 200, -1) // listener for useCom=200
	p.tabs[1] = 200               // make com=200 visible

	// com=149, useCom=200 — different components. In 244 this must NOT be rejected by a mismatch check.
	// (The packet may still fail at later gates; we just assert the mismatch gate is gone
	// by checking that at minimum no immediate drop happens on the mismatch itself.)
	// Since both items are in the same inv and both comIds have listeners + items,
	// the packet should reach state mutation.
	_ = handleOpHeldU(p, opHeldUPayload(555, 3, 149, 777, 5, 200))
	// Both comId and useComId are valid interactable components with listeners and items;
	// 244 removes comId==useComId gate so state mutation must occur.
	if p.lastItem != 555 && p.lastItem != 777 {
		t.Errorf("244: comId!=useComId must NOT be rejected (lastItem=%d, want 555 or 777)", p.lastItem)
	}
}

// TestHandleOpHeldU_ComNotInteractable_ClearsPending pins that com.Interactable=false
// rejects and calls clearPendingAction. In 244, Interactable replaces Usable.
// Mirrors TS OpHeldUHandler.ts (244): "!com.interactable".
func TestHandleOpHeldU_ComNotInteractable_ClearsPending(t *testing.T) {
	s, p := setupOpHeldUServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: false, Usable: true}, // Interactable cleared
	})
	p.target = p
	p.opcalled = true
	_ = handleOpHeldU(p, opHeldUPayload(555, 3, 149, 777, 5, 149))
	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (com not Interactable must reject)", p.lastItem)
	}
	if p.target != nil {
		t.Error("244: com not-Interactable reject must call clearPendingAction")
	}
}

// TestHandleOpHeldU_UseComNotInteractable_ClearsPending pins that useCom.Interactable=false
// rejects and calls clearPendingAction (244 change).
func TestHandleOpHeldU_UseComNotInteractable_ClearsPending(t *testing.T) {
	s, p := setupOpHeldUServer(t)
	// We need a second component for useCom. Seed useCom=200 as non-interactable.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true, Usable: true},
		200: {RootLayer: 200, Interactable: false, Usable: true}, // not interactable
	})
	p.invListenOnCom(93, 200, -1)
	p.tabs[1] = 200
	p.target = p
	p.opcalled = true
	_ = handleOpHeldU(p, opHeldUPayload(555, 3, 149, 777, 5, 200))
	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (useCom not Interactable must reject)", p.lastItem)
	}
	if p.target != nil {
		t.Error("244: useCom not-Interactable reject must call clearPendingAction")
	}
}

// TestHandleOpHeldU_NoListener_ClearsPending pins that listener-not-found for com
// calls clearPendingAction in 244 (change from 225 which just returned nil).
func TestHandleOpHeldU_NoListener_ClearsPending(t *testing.T) {
	_, p := setupOpHeldUServer(t)
	delete(p.invListeners, 149)
	p.target = p
	p.opcalled = true
	_ = handleOpHeldU(p, opHeldUPayload(555, 3, 149, 777, 5, 149))
	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (no com listener must reject)", p.lastItem)
	}
	if p.target != nil {
		t.Error("244: no-listener reject must call clearPendingAction")
	}
}

// TestHandleOpHeldU_NoUseListener_ClearsPending pins that listener-not-found for useCom
// calls clearPendingAction (244 change).
func TestHandleOpHeldU_NoUseListener_ClearsPending(t *testing.T) {
	s, p := setupOpHeldUServer(t)
	// useCom=200 with no listener.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true, Usable: true},
		200: {RootLayer: 200, Interactable: true, Usable: true},
	})
	p.tabs[1] = 200
	// No listener for useCom=200 — leave p.invListeners only having 149.
	p.target = p
	p.opcalled = true
	_ = handleOpHeldU(p, opHeldUPayload(555, 3, 149, 777, 5, 200))
	if p.lastItem != -1 {
		t.Errorf("lastItem: got %d, want -1 (no useCom listener must reject)", p.lastItem)
	}
	if p.target != nil {
		t.Error("244: no-useListener reject must call clearPendingAction")
	}
}

// TestHandleOpHeldU_SlotMismatch_ClearsAndUnsetsMoveClick pins gate 6:
// inv.HasAt(slot, obj)=false → moveClickRequest=false + ClearPendingAction.
// TS OpHeldUHandler.ts:39-43.
func TestHandleOpHeldU_SlotMismatch_ClearsAndUnsetsMoveClick(t *testing.T) {
	_, p := setupOpHeldUServer(t)
	// Pre-arm the sentinel state.
	p.moveClickRequest = true
	p.target = p // ClearPendingAction sets target=nil

	// slot=10 is empty in the fixture (inv has items at 3 and 5 only).
	_ = handleOpHeldU(p, opHeldUPayload(555, 10, 149, 777, 5, 149))

	if p.moveClickRequest {
		t.Error("moveClickRequest: want false after slot-mismatch reject (gate 6)")
	}
	if p.target != nil {
		t.Error("ClearPendingAction: want called (target nil) after slot-mismatch reject (gate 6)")
	}
}

// TestHandleOpHeldU_UseSlotMismatch_ClearsAndUnsetsMoveClick pins gate 8:
// useInv.HasAt(useSlot, useObj)=false → moveClickRequest=false + ClearPendingAction.
// TS OpHeldUHandler.ts:54-58.
func TestHandleOpHeldU_UseSlotMismatch_ClearsAndUnsetsMoveClick(t *testing.T) {
	_, p := setupOpHeldUServer(t)
	// Pre-arm the sentinel state.
	p.moveClickRequest = true
	p.target = p // ClearPendingAction sets target=nil

	// slot=3 has id=555 (HasAt true); useSlot=10 is empty (HasAt false).
	_ = handleOpHeldU(p, opHeldUPayload(555, 3, 149, 777, 10, 149))

	if p.moveClickRequest {
		t.Error("moveClickRequest: want false after useSlot-mismatch reject (gate 8)")
	}
	if p.target != nil {
		t.Error("ClearPendingAction: want called (target nil) after useSlot-mismatch reject (gate 8)")
	}
}

// Happy path arm (a): [opheldu,objType.id] hits — no swap.
// TS OpHeldUHandler.ts:96-97 ("[opheldu,b]" in TS labelling but lookup
// is on objType.id which is the dragged item).
func TestHandleOpHeldU_ArmA_NoSwap(t *testing.T) {
	s, p := setupOpHeldUServer(t)
	sf := &script.ScriptFile{
		Name:        "[opheldu,555]",
		LookupKey:   script.LookupKeyForType(script.TriggerOpHeldU, 555),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(sf)

	_ = handleOpHeldU(p, opHeldUPayload(555, 3, 149, 777, 5, 149))

	if p.lastItem != 555 {
		t.Errorf("lastItem (a): got %d, want 555 (no swap)", p.lastItem)
	}
	if p.lastSlot != 3 {
		t.Errorf("lastSlot (a): got %d, want 3", p.lastSlot)
	}
	if p.lastUseItem != 777 {
		t.Errorf("lastUseItem (a): got %d, want 777", p.lastUseItem)
	}
	if p.lastUseSlot != 5 {
		t.Errorf("lastUseSlot (a): got %d, want 5", p.lastUseSlot)
	}
}

// Arm (b): [opheldu,useObjType.id] hits when (a) misses — SWAP both pairs.
// TS OpHeldUHandler.ts:99-103.
func TestHandleOpHeldU_ArmB_SwapsItemAndSlot(t *testing.T) {
	s, p := setupOpHeldUServer(t)
	sf := &script.ScriptFile{
		Name:        "[opheldu,777]",
		LookupKey:   script.LookupKeyForType(script.TriggerOpHeldU, 777),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(sf)

	_ = handleOpHeldU(p, opHeldUPayload(555, 3, 149, 777, 5, 149))

	// Pre-swap: lastItem=555, lastSlot=3, lastUseItem=777, lastUseSlot=5
	// Post-swap: lastItem=777, lastSlot=5, lastUseItem=555, lastUseSlot=3
	if p.lastItem != 777 {
		t.Errorf("lastItem (b): got %d, want 777 (swapped)", p.lastItem)
	}
	if p.lastSlot != 5 {
		t.Errorf("lastSlot (b): got %d, want 5 (swapped)", p.lastSlot)
	}
	if p.lastUseItem != 555 {
		t.Errorf("lastUseItem (b): got %d, want 555 (swapped)", p.lastUseItem)
	}
	if p.lastUseSlot != 3 {
		t.Errorf("lastUseSlot (b): got %d, want 3 (swapped)", p.lastUseSlot)
	}
}

// Arm (c): [opheldu,-1,objType.Category] hits — INHERITS the b-block swap.
// objType.Category=100 activates the category-fallback arm (c).
//
// Path: (a) misses → (b) entered → b-swap fires (UNCONDITIONAL) → (b)'s
// lookup misses (no script for useObjType.id=777) → (c) entered
// (objType.Category=100) → (c) hits → no further swap.
// Final state: b-block swap took effect, so lastItem=777, lastUseItem=555.
func TestHandleOpHeldU_ArmC_CategoryB_Inherits_BSwap(t *testing.T) {
	s, p := setupOpHeldUServer(t)
	s.objTypes.Configs[555].Category = 100 // category set so arm (c) is active
	sf := &script.ScriptFile{
		Name:        "[opheldu,_,100]",
		LookupKey:   script.LookupKeyForCategory(script.TriggerOpHeldU, 100),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(sf)

	_ = handleOpHeldU(p, opHeldUPayload(555, 3, 149, 777, 5, 149))

	// Pre-snapshot: lastItem=555, lastSlot=3, lastUseItem=777, lastUseSlot=5.
	// After b-block swap: lastItem=777, lastSlot=5, lastUseItem=555, lastUseSlot=3.
	// Arm (c) fires with no additional swap.
	if p.lastItem != 777 {
		t.Errorf("lastItem (c): got %d, want 777 (inherits b-block swap)", p.lastItem)
	}
	if p.lastSlot != 5 {
		t.Errorf("lastSlot (c): got %d, want 5 (inherits b-block swap)", p.lastSlot)
	}
	if p.lastUseItem != 555 {
		t.Errorf("lastUseItem (c): got %d, want 555 (inherits b-block swap)", p.lastUseItem)
	}
	if p.lastUseSlot != 3 {
		t.Errorf("lastUseSlot (c): got %d, want 3 (inherits b-block swap)", p.lastUseSlot)
	}
}

// Arm (d): [opheldu,-1,useObjType.Category] hits — b-swap + d-swap = net identity.
// useObjType.Category=200 activates arm (d).
//
// Path: (a) misses → (b) entered → b-swap fires (UNCONDITIONAL) → (b)'s
// lookup misses (no script for useObjType.id=777) → (c) skipped
// (objType.Category=-1) → (d) entered (useObjType.Category=200) → d-swap
// fires (UNCONDITIONAL) → (d)'s lookup hits.
// b-swap + d-swap = double swap = net identity (original state restored).
func TestHandleOpHeldU_ArmD_CategoryA_DoubleSwap_NetIdentity(t *testing.T) {
	s, p := setupOpHeldUServer(t)
	s.objTypes.Configs[777].Category = 200
	sf := &script.ScriptFile{
		Name:        "[opheldu,_,200]",
		LookupKey:   script.LookupKeyForCategory(script.TriggerOpHeldU, 200),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	}
	s.scriptProvider.Register(sf)

	_ = handleOpHeldU(p, opHeldUPayload(555, 3, 149, 777, 5, 149))

	// Pre-snapshot: lastItem=555, lastSlot=3, lastUseItem=777, lastUseSlot=5.
	// After b-block swap: lastItem=777, lastSlot=5, lastUseItem=555, lastUseSlot=3.
	// After d-block swap: lastItem=555, lastSlot=3, lastUseItem=777, lastUseSlot=5.
	// Double swap = net identity; final state equals original snapshot.
	if p.lastItem != 555 {
		t.Errorf("lastItem (d): got %d, want 555 (b-swap+d-swap=identity)", p.lastItem)
	}
	if p.lastSlot != 3 {
		t.Errorf("lastSlot (d): got %d, want 3 (b-swap+d-swap=identity)", p.lastSlot)
	}
	if p.lastUseItem != 777 {
		t.Errorf("lastUseItem (d): got %d, want 777 (b-swap+d-swap=identity)", p.lastUseItem)
	}
	if p.lastUseSlot != 5 {
		t.Errorf("lastUseSlot (d): got %d, want 5 (b-swap+d-swap=identity)", p.lastUseSlot)
	}
}

// TestHandleOpHeldU_AllMiss_NothingInteresting pins that when all four
// trigger arms miss, "Nothing interesting happens." is sent to the client.
func TestHandleOpHeldU_AllMiss_NothingInteresting(t *testing.T) {
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
	inv.Items[5] = &inventory.Item{Id: 777, Count: 1}
	s.invs[93] = inv
	s.objTypes = &objtype.ObjTypeConfigs{Configs: make([]*objtype.ObjType, 800)}
	s.objTypes.Configs[555] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: 555},
		IOp:        []string{"op1", "", "", "", ""},
		Category:   -1,
	}
	s.objTypes.Configs[777] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: 777},
		IOp:        []string{"op1", "", "", "", ""},
		Category:   -1,
	}

	p.client.server = s
	p.invListenOnCom(93, 149, -1)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true, Usable: true},
	})
	p.tabs[0] = 149

	received := drainConn(t, cc)
	_ = handleOpHeldU(p, opHeldUPayload(555, 3, 149, 777, 5, 149))
	p.client.flushWrite()
	got := <-received
	if !bytes.Contains(got, []byte("Nothing interesting happens.")) {
		t.Errorf("want \"Nothing interesting happens.\" in drained bytes, got %x", got)
	}
}

// TestHandleOpHeldSessionLogPushOp1Through4 pins NAI-71-D close: every
// successful op != 5 dispatch pushes one MODERATOR session-log record
// formatted as "<iop> <debugname>". Exercises ops 1..4.
func TestHandleOpHeldSessionLogPushOp1Through4(t *testing.T) {
	cases := []struct {
		op       int
		iop      string
		wantSkip bool
	}{
		{1, "op1", false},
		{2, "op2", false},
		{3, "op3", false},
		{4, "op4", false},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("op%d", tc.op), func(t *testing.T) {
			s, p := setupOpHeldServer(t)
			// Seed all 5 IOp slots so each op is allowed.
			s.objTypes.Configs[555].IOp = []string{"op1", "op2", "op3", "op4", "op5"}
			p.session = "test-sess"
			p.level, p.x, p.z = 0, 3200, 3200

			err := handleOpHeld(p, opHeldPayload(555, 3, 149), tc.op)
			if err != nil {
				t.Fatalf("handleOpHeld op=%d: %v", tc.op, err)
			}

			if got := len(s.sessionLogs); got != 1 {
				t.Fatalf("sessionLogs after op=%d: got %d, want 1", tc.op, got)
			}
			lg := s.sessionLogs[0]
			wantEvent := tc.iop + " test_held"
			if lg.Event != wantEvent {
				t.Errorf("Event: got %q, want %q", lg.Event, wantEvent)
			}
			if lg.EventType != LoggerEventTypeModerator {
				t.Errorf("EventType: got %d, want MODERATOR(%d)", lg.EventType, LoggerEventTypeModerator)
			}
			if lg.SessionUUID != "test-sess" {
				t.Errorf("SessionUUID: got %q, want test-sess", lg.SessionUUID)
			}
		})
	}
}

// TestHandleOpHeldOp5NoSessionLog pins the TS wealth-log carve-out:
// op == 5 must NOT push a session-log (TS OpHeldHandler.ts:63).
func TestHandleOpHeldOp5NoSessionLog(t *testing.T) {
	s, p := setupOpHeldServer(t)
	s.objTypes.Configs[555].IOp = []string{"op1", "op2", "op3", "op4", "op5"}

	if err := handleOpHeld(p, opHeldPayload(555, 3, 149), 5); err != nil {
		t.Fatalf("handleOpHeld op=5: %v", err)
	}

	if got := len(s.sessionLogs); got != 0 {
		t.Errorf("sessionLogs: got %d, want 0 (op=5 must skip session-log)", got)
	}
}

// TestHandleOpHeldSessionLogBeforeScript pins that the session-log push
// happens unconditionally on the gates-passed path — even when no script
// is registered for the trigger. Mirrors TS unconditional addSessionLog
// at OpHeldHandler.ts:62-65 (line 64 runs before the line-69 dispatch).
func TestHandleOpHeldSessionLogBeforeScript(t *testing.T) {
	s, p := setupOpHeldServer(t)
	s.objTypes.Configs[555].IOp = []string{"op1", "", "", "", ""}
	// Do NOT register any script for the OPHELD1 trigger — runScript(nil) is no-op.

	if err := handleOpHeld(p, opHeldPayload(555, 3, 149), 1); err != nil {
		t.Fatalf("handleOpHeld op=1: %v", err)
	}

	if got := len(s.sessionLogs); got != 1 {
		t.Errorf("sessionLogs: got %d, want 1 (push must fire regardless of script presence)", got)
	}
}

// TestHandleOpHeldTSessionLogPush pins NAI-71-D close for OPHELDT:
// successful dispatch pushes one MODERATOR record formatted as
// "Cast <comName> on <debugname>" (TS OpHeldTHandler.ts:61).
func TestHandleOpHeldTSessionLogPush(t *testing.T) {
	s, p := setupOpHeldTServer(t)
	// Set ComName on spell component 200 — re-seed because seedComponentTypes is full-replace.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true, Usable: true},
		200: {RootLayer: 200, ActionTarget: objtype.ComActionTargetHeld, ComName: "spell_blast"},
	})
	// Register a no-op script to prevent the "Nothing interesting happens."
	// fallback from reaching MessageGame (which requires an encryptor).
	s.scriptProvider.Register(&script.ScriptFile{
		Name:        "[opheldt,200]",
		LookupKey:   script.LookupKeyForType(script.TriggerOpHeldT, 200),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	})
	p.session = "wizard-sess"

	if err := handleOpHeldT(p, opHeldTPayload(555, 3, 149, 200)); err != nil {
		t.Fatalf("handleOpHeldT: %v", err)
	}

	if got := len(s.sessionLogs); got != 1 {
		t.Fatalf("sessionLogs: got %d, want 1", got)
	}
	lg := s.sessionLogs[0]
	wantEvent := "Cast spell_blast on test_held"
	if lg.Event != wantEvent {
		t.Errorf("Event: got %q, want %q", lg.Event, wantEvent)
	}
	if lg.EventType != LoggerEventTypeModerator {
		t.Errorf("EventType: got %d, want MODERATOR(%d)", lg.EventType, LoggerEventTypeModerator)
	}
	if lg.SessionUUID != "wizard-sess" {
		t.Errorf("SessionUUID: got %q, want wizard-sess", lg.SessionUUID)
	}
}

// TestHandleOpHeldTSessionLogMissingObjType pins the goscape-defensive
// guard: when the obj has no registered ObjType, the session-log is
// skipped (no panic). TS would throw at ObjType.get(obj).debugname.
func TestHandleOpHeldTSessionLogMissingObjType(t *testing.T) {
	s, p := setupOpHeldTServer(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true, Usable: true},
		200: {RootLayer: 200, ActionTarget: objtype.ComActionTargetHeld, ComName: "spell_blast"},
	})
	// Register a no-op script to prevent the "Nothing interesting happens."
	// fallback from reaching MessageGame (which requires an encryptor).
	s.scriptProvider.Register(&script.ScriptFile{
		Name:        "[opheldt,200]",
		LookupKey:   script.LookupKeyForType(script.TriggerOpHeldT, 200),
		Opcodes:     []script.Opcode{script.OpReturn},
		IntOperands: []int32{0}, StringOperands: []string{""}, InstructionCount: 1,
	})
	// Place an item with id=999 in inv slot 3 so HasAt passes, but use an obj id
	// that's out-of-bounds for the Configs slice (Configs len is 600 from
	// setupOpHeldServer; 999 trips the bounds-guard in the new ObjType lookup).
	s.invs[93].Items[3] = &inventory.Item{Id: 999, Count: 1}

	if err := handleOpHeldT(p, opHeldTPayload(999, 3, 149, 200)); err != nil {
		t.Fatalf("handleOpHeldT: %v", err)
	}

	// No panic; no session-log pushed because ObjType is nil/out-of-bounds.
	if got := len(s.sessionLogs); got != 0 {
		t.Errorf("sessionLogs: got %d, want 0 (missing ObjType must skip session-log)", got)
	}
}

// TestHandleOpHeldU_MembersOnFreeWorld_Rejects pins that members-only items
// on a free world send the members message. Per TS OpHeldUHandler.ts:90-93,
// state is mutated BEFORE the members check (TS:78-81 ordering).
func TestHandleOpHeldU_MembersOnFreeWorld_Rejects(t *testing.T) {
	p, cc := newTestPlayer(t)
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}
	s.cfg.NodeMembers = false
	s.invs = make(map[int]*inventory.Inventory)
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 555, Count: 1}
	inv.Items[5] = &inventory.Item{Id: 777, Count: 1}
	s.invs[93] = inv
	s.objTypes = &objtype.ObjTypeConfigs{Configs: make([]*objtype.ObjType, 800)}
	s.objTypes.Configs[555] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: 555},
		IOp:        []string{"op1", "", "", "", ""},
		Category:   -1,
		Members:    true, // triggers members gate
	}
	s.objTypes.Configs[777] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: 777},
		IOp:        []string{"op1", "", "", "", ""},
		Category:   -1,
	}

	p.client.server = s
	p.invListenOnCom(93, 149, -1)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true, Usable: true},
	})
	p.tabs[0] = 149

	received := drainConn(t, cc)
	_ = handleOpHeldU(p, opHeldUPayload(555, 3, 149, 777, 5, 149))
	p.client.flushWrite()
	got := <-received
	if !bytes.Contains(got, []byte("To use this item please login to a members' server.")) {
		t.Errorf("want members-message in drained bytes, got %x", got)
	}
	// Per TS: state set pre-members-check (TS:78-81); lastItem must equal obj=555.
	if p.lastItem != 555 {
		t.Errorf("lastItem: got %d, want 555 (TS sets state pre-members-check)", p.lastItem)
	}
}
