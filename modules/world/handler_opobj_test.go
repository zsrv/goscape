package world

import (
	"bytes"
	"net"
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/inventory"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
	"github.com/zsrv/goscape/pkg/zone"
)

// makeOpObjFixture creates a server + player + obj adjacent to the player,
// with an ObjType registered, ready for handleOpObj tests.
// Player at (99, 100, 0); obj at (100, 100, 0) — Chebyshev=1.
// Player originX/originZ = (100, 100) so viewport gate accepts coords
// within [-52, +52] of (100, 100).
// ObjType 42, Op = ["op1","op2","op3","op4","op5"].
// Returns (server, player, obj, clientConn).
func makeOpObjFixture(t *testing.T) (*Server, *Player, *entitypkg.Obj, net.Conn) {
	t.Helper()
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()

	s.objTypes = &objtype.ObjTypeConfigs{
		Configs: make([]*objtype.ObjType, 43),
	}
	s.objTypes.Configs[42] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: 42, DebugName: "test_obj"},
		Op:         []string{"op1", "op2", "op3", "op4", "op5"},
	}

	obj := entitypkg.NewObj(0, 100, 100, entitypkg.LifecycleDespawn, 42, 1)
	zn := s.zoneMap.Get(0, 100, 100)
	zn.Objs = append(zn.Objs, obj)

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 99, 100, 0
	p.originX, p.originZ = 100, 100

	return s, p, obj, cc
}

// p2x3ObjPayload encodes (x: u16, z: u16, objId: u16) into 6 bytes big-endian.
func p2x3ObjPayload(x, z, objId int) []byte {
	return []byte{
		byte(x >> 8), byte(x),
		byte(z >> 8), byte(z),
		byte(objId >> 8), byte(objId),
	}
}

// p2x4ObjPayload encodes (x: u16, z: u16, objId: u16, com: u16) into 8 bytes.
func p2x4ObjPayload(x, z, objId, com int) []byte {
	return []byte{
		byte(x >> 8), byte(x),
		byte(z >> 8), byte(z),
		byte(objId >> 8), byte(objId),
		byte(com >> 8), byte(com),
	}
}

// p2x6ObjPayload encodes (x, z, objId, useObj, useSlot, useCom) into 12 bytes.
func p2x6ObjPayload(x, z, objId, useObj, useSlot, useCom int) []byte {
	return []byte{
		byte(x >> 8), byte(x),
		byte(z >> 8), byte(z),
		byte(objId >> 8), byte(objId),
		byte(useObj >> 8), byte(useObj),
		byte(useSlot >> 8), byte(useSlot),
		byte(useCom >> 8), byte(useCom),
	}
}

// --- handleOpObj (OPOBJ1-5) ---

func TestHandleOpObj1SetsInteraction(t *testing.T) {
	_, p, obj, _ := makeOpObjFixture(t)

	if err := handleOpObj1(p, p2x3ObjPayload(100, 100, 42)); err != nil {
		t.Fatalf("handleOpObj1: %v", err)
	}

	if p.target != obj {
		t.Errorf("target: got %v, want obj", p.target)
	}
	if p.targetOp != 1 {
		t.Errorf("targetOp: got %d, want 1", p.targetOp)
	}
	if p.interactionKind != InteractionEngine {
		t.Errorf("interactionKind: got %v, want InteractionEngine", p.interactionKind)
	}
	if !p.opcalled {
		t.Error("opcalled: want true")
	}
	if p.targetSubject.typ != 42 {
		t.Errorf("targetSubject.typ: got %d, want 42", p.targetSubject.typ)
	}
	if p.targetSubject.x != 100 || p.targetSubject.z != 100 || p.targetSubject.level != 0 {
		t.Errorf("targetSubject coords: got (%d,%d,%d), want (100,100,0)",
			p.targetSubject.x, p.targetSubject.z, p.targetSubject.level)
	}
}

func TestHandleOpObjAllFiveOpsRouteIndependently(t *testing.T) {
	type opCase struct {
		op int
		fn func(*Player, []byte) error
	}
	cases := []opCase{
		{1, handleOpObj1}, {2, handleOpObj2}, {3, handleOpObj3},
		{4, handleOpObj4}, {5, handleOpObj5},
	}
	for _, c := range cases {
		t.Run("op"+string(rune('0'+c.op)), func(t *testing.T) {
			_, p, _, _ := makeOpObjFixture(t)
			if err := c.fn(p, p2x3ObjPayload(100, 100, 42)); err != nil {
				t.Fatalf("op%d: %v", c.op, err)
			}
			if p.targetOp != c.op {
				t.Errorf("targetOp: got %d, want %d", p.targetOp, c.op)
			}
		})
	}
}

func TestHandleOpObjDelayedPlayerRejected(t *testing.T) {
	s, p, _, cc := makeOpObjFixture(t)
	p.delayed = true
	p.delayedUntil = 999
	s.currentTick = 0

	received := drainConn(t, cc)
	_ = handleOpObj1(p, p2x3ObjPayload(100, 100, 42))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for delayed player")
	}
	if p.target != nil {
		t.Error("target should remain nil for delayed player")
	}
}

func TestHandleOpObjShortPayloadRejected(t *testing.T) {
	_, p, _, cc := makeOpObjFixture(t)

	received := drainConn(t, cc)
	_ = handleOpObj1(p, []byte{0x00, 0x64, 0x00}) // 3 bytes
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for short payload")
	}
	if p.target != nil {
		t.Error("target should remain nil for short payload")
	}
}

func TestHandleOpObjOutOfViewportRejected(t *testing.T) {
	_, p, _, cc := makeOpObjFixture(t)

	received := drainConn(t, cc)
	_ = handleOpObj1(p, p2x3ObjPayload(250, 100, 42)) // dx = 150 > 52
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for out-of-viewport click")
	}
	if p.target != nil {
		t.Error("target should remain nil for out-of-viewport click")
	}
}

func TestHandleOpObjMissingObjRejected(t *testing.T) {
	_, p, _, cc := makeOpObjFixture(t)

	received := drainConn(t, cc)
	_ = handleOpObj1(p, p2x3ObjPayload(100, 100, 999)) // wrong objId
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing obj")
	}
	if p.target != nil {
		t.Error("target should remain nil for missing obj")
	}
}

func TestHandleOpObjMissingObjTypeRejected(t *testing.T) {
	s, p, _, cc := makeOpObjFixture(t)
	// Place an obj with typeID 77 but no registered ObjType.
	extra := entitypkg.NewObj(0, 100, 100, entitypkg.LifecycleDespawn, 77, 1)
	s.zoneMap.Get(0, 100, 100).Objs = append(s.zoneMap.Get(0, 100, 100).Objs, extra)

	received := drainConn(t, cc)
	_ = handleOpObj1(p, p2x3ObjPayload(100, 100, 77)) // ObjType 77 not registered
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing ObjType")
	}
	if p.target != nil {
		t.Error("target should remain nil for missing ObjType")
	}
}

func TestHandleOpObjRejectsEmptyOpSlot(t *testing.T) {
	s, p, _, cc := makeOpObjFixture(t)
	s.objTypes.Configs[42].Op[0] = "" // op=1 slot cleared

	received := drainConn(t, cc)
	_ = handleOpObj1(p, p2x3ObjPayload(100, 100, 42))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for empty Op slot")
	}
	if p.target != nil {
		t.Error("target should remain nil when Op slot is empty")
	}
}

// --- handleOpObjT ---

func TestHandleOpObjTSetsInteraction(t *testing.T) {
	s, p, obj, _ := makeOpObjFixture(t)

	// gate satisfaction: register spellCom with passing ActionTarget bit and visibility.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		7777: {RootLayer: 7777, ActionTarget: objtype.ComActionTargetObj},
	})
	p.tabs[0] = 7777

	if err := handleOpObjT(p, p2x4ObjPayload(100, 100, 42, 7777)); err != nil {
		t.Fatalf("handleOpObjT: %v", err)
	}

	if p.target != obj {
		t.Errorf("target: got %v, want obj", p.target)
	}
	if p.targetOp != targetOpObjT {
		t.Errorf("targetOp: got %d, want targetOpObjT (%d)", p.targetOp, targetOpObjT)
	}
	if p.targetSubject.com != 7777 {
		t.Errorf("targetSubject.com: got %d, want 7777 (spellCom)", p.targetSubject.com)
	}
	if p.targetSubject.typ != 42 || p.targetSubject.x != 100 || p.targetSubject.z != 100 {
		t.Errorf("targetSubject snapshot: got (typ=%d,x=%d,z=%d), want (42,100,100)",
			p.targetSubject.typ, p.targetSubject.x, p.targetSubject.z)
	}
	if !p.opcalled {
		t.Error("opcalled: want true")
	}
}

func TestHandleOpObjTDelayedPlayerRejected(t *testing.T) {
	s, p, _, cc := makeOpObjFixture(t)
	p.delayed = true
	p.delayedUntil = 999
	s.currentTick = 0

	received := drainConn(t, cc)
	_ = handleOpObjT(p, p2x4ObjPayload(100, 100, 42, 7777))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for delayed player")
	}
	if p.target != nil {
		t.Error("target should remain nil for delayed player")
	}
}

func TestHandleOpObjTShortPayloadRejected(t *testing.T) {
	_, p, _, cc := makeOpObjFixture(t)

	received := drainConn(t, cc)
	_ = handleOpObjT(p, []byte{0x00, 0x64, 0x00, 0x64}) // 4 bytes, need 8
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for short payload")
	}
	if p.target != nil {
		t.Error("target should remain nil for short payload")
	}
}

func TestHandleOpObjTOutOfViewportRejected(t *testing.T) {
	_, p, _, cc := makeOpObjFixture(t)

	received := drainConn(t, cc)
	_ = handleOpObjT(p, p2x4ObjPayload(250, 100, 42, 7777)) // dx = 150
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for out-of-viewport")
	}
	if p.target != nil {
		t.Error("target should remain nil for out-of-viewport")
	}
}

func TestHandleOpObjTMissingObjRejected(t *testing.T) {
	_, p, _, cc := makeOpObjFixture(t)

	received := drainConn(t, cc)
	_ = handleOpObjT(p, p2x4ObjPayload(100, 100, 999, 7777))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing obj")
	}
	if p.target != nil {
		t.Error("target should remain nil for missing obj")
	}
}

// --- handleOpObjU ---

func TestHandleOpObjUSetsInteraction(t *testing.T) {
	s, p, obj, _ := makeOpObjFixture(t)

	// Seed component so the component gate passes.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Usable: true},
	})
	p.tabs[0] = 149

	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 1511, Count: 1}
	s.invs[93] = inv
	p.invListenOnCom(93, 149, -1)

	if err := handleOpObjU(p, p2x6ObjPayload(100, 100, 42, 1511, 3, 149)); err != nil {
		t.Fatalf("handleOpObjU: %v", err)
	}

	if p.target != obj {
		t.Errorf("target: got %v, want obj", p.target)
	}
	if p.targetOp != targetOpObjU {
		t.Errorf("targetOp: got %d, want targetOpObjU (%d)", p.targetOp, targetOpObjU)
	}
	if p.lastUseItem != 1511 {
		t.Errorf("lastUseItem: got %d, want 1511", p.lastUseItem)
	}
	if p.lastUseSlot != 3 {
		t.Errorf("lastUseSlot: got %d, want 3", p.lastUseSlot)
	}
	if p.targetSubject.com != -1 {
		t.Errorf("targetSubject.com: got %d, want -1 (OPOBJU passes -1)", p.targetSubject.com)
	}
	if !p.opcalled {
		t.Error("opcalled: want true")
	}
}

func TestHandleOpObjUDelayedPlayerRejected(t *testing.T) {
	s, p, _, cc := makeOpObjFixture(t)
	p.delayed = true
	p.delayedUntil = 999
	s.currentTick = 0
	p.lastUseItem = 42 // sentinel: must stay unchanged

	received := drainConn(t, cc)
	_ = handleOpObjU(p, p2x6ObjPayload(100, 100, 42, 1511, 3, 149))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for delayed player")
	}
	if p.target != nil {
		t.Error("target should remain nil for delayed player")
	}
	if p.lastUseItem != 42 {
		t.Errorf("lastUseItem leaked through rejected handler: got %d, want 42", p.lastUseItem)
	}
}

func TestHandleOpObjUShortPayloadRejected(t *testing.T) {
	_, p, _, cc := makeOpObjFixture(t)

	received := drainConn(t, cc)
	_ = handleOpObjU(p, []byte{0x00, 0x64, 0x00, 0x64, 0x00, 0x2a, 0x05, 0xe7}) // 8 bytes
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for short payload")
	}
	if p.target != nil {
		t.Error("target should remain nil for short payload")
	}
}

func TestHandleOpObjUOutOfViewportRejected(t *testing.T) {
	_, p, _, cc := makeOpObjFixture(t)

	received := drainConn(t, cc)
	_ = handleOpObjU(p, p2x6ObjPayload(250, 100, 42, 1511, 3, 149)) // dx=150
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for out-of-viewport")
	}
	if p.target != nil {
		t.Error("target should remain nil for out-of-viewport")
	}
}

func TestHandleOpObjUMissingObjRejected(t *testing.T) {
	_, p, _, cc := makeOpObjFixture(t)

	received := drainConn(t, cc)
	_ = handleOpObjU(p, p2x6ObjPayload(100, 100, 999, 1511, 3, 149))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing obj")
	}
	if p.target != nil {
		t.Error("target should remain nil for missing obj")
	}
}

func TestHandleOpObjUMissingListenerRejected(t *testing.T) {
	s, p, _, cc := makeOpObjFixture(t)
	// Seed component so the component gate passes; listener-missing gate fires next.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Usable: true},
	})
	p.tabs[0] = 149
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	s.invs[93] = inventory.New(93, 28, inventory.StackNormal)
	// No invListenOnCom call — listener absent.
	p.lastUseItem = 77 // sentinel

	received := drainConn(t, cc)
	_ = handleOpObjU(p, p2x6ObjPayload(100, 100, 42, 1511, 3, 149))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for missing listener")
	}
	if p.target != nil {
		t.Error("target should remain nil for missing listener")
	}
	if p.lastUseItem != 77 {
		t.Errorf("lastUseItem leaked: got %d, want 77", p.lastUseItem)
	}
}

func TestHandleOpObjUItemMismatchRejected(t *testing.T) {
	s, p, _, cc := makeOpObjFixture(t)
	// Seed component so the component gate passes; item-mismatch gate fires next.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Usable: true},
	})
	p.tabs[0] = 149
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 9999, Count: 1} // NOT 1511
	s.invs[93] = inv
	p.invListenOnCom(93, 149, -1)
	p.lastUseItem = 77 // sentinel

	received := drainConn(t, cc)
	_ = handleOpObjU(p, p2x6ObjPayload(100, 100, 42, 1511, 3, 149)) // claims 1511
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for item mismatch")
	}
	if p.target != nil {
		t.Error("target should remain nil for item mismatch")
	}
	if p.lastUseItem != 77 {
		t.Errorf("lastUseItem leaked: got %d, want 77", p.lastUseItem)
	}
}

// --- Trigger dispatch tests (fireOpTriggerObj / fireApTriggerObj) ---

// makeOpObjTriggerFixture creates a fixture for tryFireOpTrigger Obj-branch
// tests: server + player anchored on an obj with valid targetSubject,
// positioned at contact distance (player at (99,100), obj at (100,100)).
func makeOpObjTriggerFixture(t *testing.T) (*Server, *Player, *entitypkg.Obj, net.Conn) {
	t.Helper()
	s, p, obj, cc := makeOpObjFixture(t)
	s.scriptProvider = script.NewProvider()
	p.SetInteraction(InteractionEngine, obj, 1, -1)
	p.targetSubject.typ = obj.Type
	p.targetSubject.x = obj.X
	p.targetSubject.z = obj.Z
	p.targetSubject.level = obj.Level
	return s, p, obj, cc
}

// TestTryFireOpTriggerObjNoScript verifies a *Obj target with no registered
// trigger emits "Nothing interesting happens." and clears the interaction.
func TestTryFireOpTriggerObjNoScript(t *testing.T) {
	_, p, _, cc := makeOpObjTriggerFixture(t)

	received := drainConn(t, cc)
	tryFireOpTrigger(p)
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected MessageGame packet for default-op, got nothing")
	}
	if p.target != nil {
		t.Errorf("target: got %v, want nil after default-op clear", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after default-op clear")
	}
	if !bytes.Contains(got, []byte("Nothing interesting happens.")) {
		t.Errorf("expected \"Nothing interesting happens.\" in drained bytes, got %x", got)
	}
}

// TestTryFireOpTriggerObjScriptFires verifies a registered [opobj1,<typeID>]
// script fires, and ClearInteraction runs after Finished.
func TestTryFireOpTriggerObjScriptFires(t *testing.T) {
	s, p, obj, _ := makeOpObjTriggerFixture(t)

	sf := newNoopScriptFile(t, script.TriggerOpObj1, obj.Type, -1)
	s.scriptProvider.Register(sf)

	tryFireOpTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil after Finished", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after script fire")
	}
}

// TestTryFireOpTriggerObjDeferredOnDelay verifies delayed player defers fire.
func TestTryFireOpTriggerObjDeferredOnDelay(t *testing.T) {
	s, p, obj, _ := makeOpObjTriggerFixture(t)
	p.delayed = true
	p.delayedUntil = 999
	s.currentTick = 0

	tryFireOpTrigger(p)

	if p.target != obj {
		t.Errorf("target: got %v, want obj (deferred)", p.target)
	}
	if p.interactionFired {
		t.Error("interactionFired: want false (deferred)")
	}
}

// TestTryFireOpTriggerObjRemoved verifies removing the obj from its zone
// clears interaction silently.
func TestTryFireOpTriggerObjRemoved(t *testing.T) {
	s, p, _, _ := makeOpObjTriggerFixture(t)

	// Remove all objs from the zone.
	zn := s.zoneMap.Get(0, 100, 100)
	zn.Objs = nil

	tryFireOpTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil (obj removed)", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after removal clear")
	}
}

// TestTryFireOpTriggerObjFiresObjTTrigger verifies targetOpObjT → OPOBJT dispatch.
func TestTryFireOpTriggerObjFiresObjTTrigger(t *testing.T) {
	s, p, obj, _ := makeOpObjFixture(t)
	s.scriptProvider = script.NewProvider()
	p.SetInteraction(InteractionEngine, obj, targetOpObjT, 7777)
	p.targetSubject.typ = obj.Type
	p.targetSubject.x = obj.X
	p.targetSubject.z = obj.Z
	p.targetSubject.level = obj.Level

	sf := newNoopScriptFile(t, script.TriggerOpObjT, obj.Type, -1)
	s.scriptProvider.Register(sf)

	tryFireOpTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil after Finished", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after OPOBJT fire")
	}
}

// TestTryFireOpTriggerObjFiresObjUTrigger verifies targetOpObjU → OPOBJU dispatch.
func TestTryFireOpTriggerObjFiresObjUTrigger(t *testing.T) {
	s, p, obj, _ := makeOpObjFixture(t)
	s.scriptProvider = script.NewProvider()
	p.SetInteraction(InteractionEngine, obj, targetOpObjU, -1)
	p.targetSubject.typ = obj.Type
	p.targetSubject.x = obj.X
	p.targetSubject.z = obj.Z
	p.targetSubject.level = obj.Level

	sf := newNoopScriptFile(t, script.TriggerOpObjU, obj.Type, -1)
	s.scriptProvider.Register(sf)

	tryFireOpTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil after Finished", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after OPOBJU fire")
	}
}

// makeApObjTriggerFixture creates a fixture for tryFireApTrigger Obj-branch tests:
// player at (95, 100) — 5 tiles from obj at (100, 100), within apRange=10.
func makeApObjTriggerFixture(t *testing.T) (*Server, *Player, *entitypkg.Obj, net.Conn) {
	t.Helper()
	s, p, obj, cc := makeOpObjTriggerFixture(t)
	p.x, p.z = 95, 100 // move out of contact, within approach range
	return s, p, obj, cc
}

// TestTryFireApTriggerObjNoScript verifies a *Obj target with no APOBJ
// trigger leaves the interaction anchored, sets apRange=-1, interactionFired=true.
func TestTryFireApTriggerObjNoScript(t *testing.T) {
	_, p, obj, _ := makeApObjTriggerFixture(t)

	tryFireApTrigger(p)

	if p.target != obj {
		t.Errorf("target: got %v, want obj (no-AP-script should not clear)", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after no-AP-script mark")
	}
	if p.apRange != -1 {
		t.Errorf("apRange: got %d, want -1 (sentinel for no-AP-script)", p.apRange)
	}
}

// TestTryFireApTriggerObjScriptFiresNoApRangeCalled verifies an APOBJ script
// that runs but doesn't call p_aprange causes ClearInteraction.
func TestTryFireApTriggerObjScriptFiresNoApRangeCalled(t *testing.T) {
	s, p, obj, _ := makeApObjTriggerFixture(t)

	sf := newNoopScriptFile(t, script.TriggerApObj1, obj.Type, -1)
	s.scriptProvider.Register(sf)

	tryFireApTrigger(p)

	if p.target != nil {
		t.Errorf("target: got %v, want nil after no-p_aprange clear", p.target)
	}
	if !p.interactionFired {
		t.Error("interactionFired: want true after clear")
	}
}
