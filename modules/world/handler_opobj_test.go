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
	obj.IsActive = true
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

// TestHandleOpObj_DropperCanTakeOwnPrivateObj pins the reported bug: after
// dropping an item, the dropper must be able to pick it up immediately during
// the private (unrevealed) window — not only once the reveal timer expires.
//
// Private drops store ReceiverID = the dropper's UID (the drop path uses
// s.Self.UID(), the zone-visibility filter uses p.uid, reveal uses
// LookupPlayerByUID). The take handler must therefore query GetObj with p.uid,
// NOT p.slot. With p.slot, the dropper's own private obj never matches (uid !=
// slot) until reveal flips ReceiverID to PublicReceiver — exactly the bug.
// Mirrors TS: drop uses player.hash64, take uses player.hash64 (both sides
// identical by construction).
func TestHandleOpObj_DropperCanTakeOwnPrivateObj(t *testing.T) {
	_, p, obj, _ := makeOpObjFixture(t)

	// Production-shaped distinction: slot is a small index, uid is a
	// composeUID hash. If the handler used p.slot the lookup would miss.
	p.slot = 1
	p.uid = 0x4F3A0001 // composeUID-shaped, deliberately != slot

	// Make the obj a PRIVATE drop owned by this player, still unrevealed.
	obj.ReceiverID = p.uid
	obj.Reveal = entitypkg.ObjReveal

	if err := handleOpObj1(p, p2x3ObjPayload(100, 100, 42)); err != nil {
		t.Fatalf("handleOpObj1: %v", err)
	}

	if p.target != obj {
		t.Errorf("target: got %v, want the dropped obj (dropper must reach own private drop)", p.target)
	}
	if !p.opcalled {
		t.Error("opcalled: want true (dropper's take of own private obj must proceed before reveal)")
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

// TestHandleOpObjMissingObjRejected pins the 244 contract for a missing obj:
// interaction is NOT set and no UnsetMapFlag is written (the separate
// TestHandleOpObjMissingObjNoUnsetMapFlag test covers that side-effect in
// detail). TS OpObjHandler.ts:29-34 (244): no UnsetMapFlag write on missing obj.
func TestHandleOpObjMissingObjRejected(t *testing.T) {
	_, p, _, _ := makeOpObjFixture(t)

	_ = handleOpObj1(p, p2x3ObjPayload(100, 100, 999)) // wrong objId — missing

	if p.target != nil {
		t.Error("target should remain nil for missing obj")
	}
	if p.opcalled {
		t.Error("opcalled: want false for missing obj")
	}
}

func TestHandleOpObjMissingObjTypeRejected(t *testing.T) {
	s, p, _, cc := makeOpObjFixture(t)
	// Place an obj with typeID 77 but no registered ObjType.
	extra := entitypkg.NewObj(0, 100, 100, entitypkg.LifecycleDespawn, 77, 1)
	extra.IsActive = true
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

// TestHandleOpObjRejectsHiddenOpSlot — RE-OVERTURNED at 2e3bcf43: the
// explicit 'hidden' comparison is BACK (TS OpObjHandler.ts:38:
// `type.op[message.op - 1] === null || type.op[message.op - 1] === 'hidden'`).
// A 'hidden' op REJECTS with no interaction.
func TestHandleOpObjRejectsHiddenOpSlot(t *testing.T) {
	s, p, _, _ := makeOpObjFixture(t)
	s.objTypes.Configs[42].Op[0] = "hidden"

	if err := handleOpObj1(p, p2x3ObjPayload(100, 100, 42)); err != nil {
		t.Fatalf("handleOpObj1: %v", err)
	}
	if p.target != nil {
		t.Errorf("target: got %v, want nil ('hidden' op rejects at 2e3bcf43 — TS OpObjHandler.ts:38)", p.target)
	}
	if p.opcalled {
		t.Error("opcalled: want false ('hidden' op rejected)")
	}
}

// TestHandleOpObjOutOfViewportClearsPendingAction pins the 244 delta:
// the viewport gate now calls clearPendingAction() before returning.
// TS OpObjHandler.ts:23-27 (244).
func TestHandleOpObjOutOfViewportClearsPendingAction(t *testing.T) {
	_, p, obj, cc := makeOpObjFixture(t)
	// Pre-seed a stale pending action — must be cleared.
	p.target = obj
	p.targetOp = 5

	received := drainConn(t, cc)
	_ = handleOpObj1(p, p2x3ObjPayload(250, 100, 42)) // dx=150 > 52
	p.client.flushWrite()
	<-received // drain UnsetMapFlag

	if p.targetOp == -1 {
		t.Errorf("targetOp cleared to -1; 254 @2e3bcf43: rejection branches no longer clearPendingAction (was: clearPendingAction called on viewport reject — TS:25)")
	}
	if p.target == nil {
		t.Error("254 @2e3bcf43: rejection branches no longer clearPendingAction — sentinel target must survive")
	}
}

// TestHandleOpObjMissingObjNoUnsetMapFlag RE-PINNED at 2e3bcf43: a
// missing obj writes UnsetMapFlag like the rest of the family (TS
// OpObjHandler.ts:30-35) — the 244-era moveClickRequest=false +
// clearPendingAction + silent-drop branch is GONE. Test name kept for
// history greppability.
func TestHandleOpObjMissingObjNoUnsetMapFlag(t *testing.T) {
	_, p, obj, cc := makeOpObjFixture(t)
	// Pre-seed a stale pending action — must SURVIVE the reject.
	p.target = obj
	p.targetOp = 7
	p.moveClickRequest = true

	received := drainConn(t, cc)
	_ = handleOpObj1(p, p2x3ObjPayload(100, 100, 999)) // objId 999 → not found
	p.client.flushWrite()

	got := <-received
	if len(got) == 0 {
		t.Fatal("254 @2e3bcf43: missing obj must write UnsetMapFlag (TS OpObjHandler.ts:30-35)")
	}
	if p.targetOp == -1 {
		t.Errorf("targetOp cleared to -1; 254 @2e3bcf43: rejection branches no longer clearPendingAction")
	}
	if !p.moveClickRequest {
		t.Error("moveClickRequest: must be untouched at 2e3bcf43 (the 244 reset is gone)")
	}
}

// TestHandleOpObjGateOnlyOp1Op4 RE-PINNED at 2e3bcf43: EVERY op is
// gated on the type.op array (TS OpObjHandler.ts:38 — the 244-era
// op1/op4-only partial gate plus its "todo: validate all options" are
// gone). ops 2/3/5 with empty Op slots now REJECT; a populated slot
// passes. Test name kept for history greppability.
func TestHandleOpObjGateOnlyOp1Op4(t *testing.T) {
	opsUnderTest := []struct {
		op  int
		fn  func(*Player, []byte) error
		idx int // Op slot index
	}{
		{2, handleOpObj2, 1},
		{3, handleOpObj3, 2},
		{5, handleOpObj5, 4},
	}
	for _, c := range opsUnderTest {
		t.Run("op"+string(rune('0'+c.op)), func(t *testing.T) {
			s, p, obj, _ := makeOpObjFixture(t)
			s.objTypes.Configs[42].Op[c.idx] = "" // empty slot for this op
			if err := c.fn(p, p2x3ObjPayload(100, 100, 42)); err != nil {
				t.Fatalf("op%d: %v", c.op, err)
			}
			if p.target != nil {
				t.Errorf("op%d: target got %v, want nil (all ops gated at 2e3bcf43 — TS OpObjHandler.ts:38)", c.op, p.target)
			}
			if p.opcalled {
				t.Errorf("op%d: opcalled want false (rejected)", c.op)
			}

			// Populated slot passes the gate.
			s.objTypes.Configs[42].Op[c.idx] = "Use"
			if err := c.fn(p, p2x3ObjPayload(100, 100, 42)); err != nil {
				t.Fatalf("op%d (populated): %v", c.op, err)
			}
			if p.target != obj {
				t.Errorf("op%d (populated): target got %v, want obj", c.op, p.target)
			}
		})
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
	s, p, _, cc := makeOpObjFixture(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		7777: {RootLayer: 7777, ActionTarget: objtype.ComActionTargetObj},
	})
	p.tabs[0] = 7777

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
	s, p, _, cc := makeOpObjFixture(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		7777: {RootLayer: 7777, ActionTarget: objtype.ComActionTargetObj},
	})
	p.tabs[0] = 7777

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

// TestHandleOpObjTComponentCheckClearsPendingAction pins the 244 delta:
// the combined component check (nil || !visible || !actionTarget) now
// calls clearPendingAction(). TS OpObjTHandler.ts:19-24 (244).
func TestHandleOpObjTComponentCheckClearsPendingAction(t *testing.T) {
	s, p, obj, cc := makeOpObjFixture(t)
	// Register a component with wrong ActionTarget (not ComActionTargetObj).
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		7777: {RootLayer: 7777, ActionTarget: 0}, // 0 = no OBJ target bit
	})
	p.tabs[0] = 7777
	// Pre-seed stale pending action.
	p.target = obj
	p.targetOp = 5

	received := drainConn(t, cc)
	_ = handleOpObjT(p, p2x4ObjPayload(100, 100, 42, 7777))
	p.client.flushWrite()
	<-received // drain UnsetMapFlag

	if p.targetOp == -1 {
		t.Errorf("targetOp cleared to -1; 254 @2e3bcf43: rejection branches no longer clearPendingAction (was: clearPendingAction on component reject — TS:22)")
	}
	if p.target == nil {
		t.Error("target sentinel must survive the reject (no clearPendingAction at 2e3bcf43)")
	}
}

// TestHandleOpObjTOutOfViewportClearsPendingAction pins the 244 delta:
// viewport gate now calls clearPendingAction(). TS OpObjTHandler.ts:29-34 (244).
func TestHandleOpObjTOutOfViewportClearsPendingAction(t *testing.T) {
	s, p, obj, cc := makeOpObjFixture(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		7777: {RootLayer: 7777, ActionTarget: objtype.ComActionTargetObj},
	})
	p.tabs[0] = 7777
	p.target = obj
	p.targetOp = 5

	received := drainConn(t, cc)
	_ = handleOpObjT(p, p2x4ObjPayload(250, 100, 42, 7777)) // dx=150 > 52
	p.client.flushWrite()
	<-received

	if p.targetOp == -1 {
		t.Errorf("targetOp cleared to -1; 254 @2e3bcf43: rejection branches no longer clearPendingAction (was: clearPendingAction on viewport reject — TS:32)")
	}
	if p.target == nil {
		t.Error("target sentinel must survive the reject (no clearPendingAction at 2e3bcf43)")
	}
}

// TestHandleOpObjTMissingObjClearsPendingAction pins the 244 delta:
// missing obj now calls clearPendingAction(). TS OpObjTHandler.ts:36-41 (244).
func TestHandleOpObjTMissingObjClearsPendingAction(t *testing.T) {
	s, p, obj, cc := makeOpObjFixture(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		7777: {RootLayer: 7777, ActionTarget: objtype.ComActionTargetObj},
	})
	p.tabs[0] = 7777
	p.target = obj
	p.targetOp = 5

	received := drainConn(t, cc)
	_ = handleOpObjT(p, p2x4ObjPayload(100, 100, 999, 7777)) // objId 999 → not found
	p.client.flushWrite()
	<-received

	if p.targetOp == -1 {
		t.Errorf("targetOp cleared to -1; 254 @2e3bcf43: rejection branches no longer clearPendingAction (was: clearPendingAction on missing obj — TS:39)")
	}
	if p.target == nil {
		t.Error("target sentinel must survive the reject (no clearPendingAction at 2e3bcf43)")
	}
}

// --- handleOpObjU ---

func TestHandleOpObjUSetsInteraction(t *testing.T) {
	s, p, obj, _ := makeOpObjFixture(t)

	// Seed component so the component gate passes.
	// 244: gate uses com.interactable (was com.usable at 225). TS OpObjUHandler.ts:22.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true, Usable: true},
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
	// 244: gate uses com.interactable (was com.usable). TS OpObjUHandler.ts:22.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true, Usable: true},
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
	// 244: gate uses com.interactable (was com.usable). TS OpObjUHandler.ts:22.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true, Usable: true},
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

// TestHandleOpObjUMembersOnFreeWorldClearsPendingAction — ordering pin:
// ClearPendingAction must fire BEFORE the members-only check, so a stale
// pending action is cleared even when the members reject path fires.
// Matches TS OpObjUHandler.ts:59 (clearPendingAction before members check, 244).
func TestHandleOpObjUMembersOnFreeWorldClearsPendingAction(t *testing.T) {
	s, p, obj, cc := makeOpObjFixture(t)
	// 244: gate uses com.interactable (was com.usable). TS OpObjUHandler.ts:22.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true, Usable: true},
	})
	p.tabs[0] = 149
	s.cfg.NodeMembers = false
	if s.objTypes == nil {
		s.objTypes = &objtype.ObjTypeConfigs{Configs: make([]*objtype.ObjType, 2000)}
	}
	// Extend Configs slice to hold index 1511.
	if len(s.objTypes.Configs) <= 1511 {
		extended := make([]*objtype.ObjType, 1512)
		copy(extended, s.objTypes.Configs)
		s.objTypes.Configs = extended
	}
	s.objTypes.Configs[1511] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: 1511, DebugName: "members_item"},
		Members:    true,
	}
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 1511, Count: 1}
	s.invs[93] = inv
	p.invListenOnCom(93, 149, -1)

	// Pre-seed stale pending action — proves members reject clears it.
	p.targetOp = 99
	p.target = obj

	received := drainConn(t, cc)
	_ = handleOpObjU(p, p2x6ObjPayload(100, 100, 42, 1511, 3, 149))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected MessageGame + UnsetMapFlag for members-on-free, got nothing")
	}
	// Ordering pin: ClearPendingAction must have run before members reject
	// (TS OpObjUHandler.ts:69-75 @2e3bcf43 — unchanged at the 254 pin).
	if p.targetOp != -1 {
		t.Errorf("targetOp: got %d, want -1 (cleared by ClearPendingAction before members reject)", p.targetOp)
	}
	if p.target != nil {
		t.Error("target: want nil (ClearPendingAction precedes the members gate)")
	}
}

// TestHandleOpObjUUsesInteractableNotUsable pins the 244 delta:
// component gate now checks com.Interactable (renamed from Usable/operable
// at 244). A component with Usable=true but Interactable=false must be
// REJECTED. TS OpObjUHandler.ts:22 (244): `!com.interactable`.
func TestHandleOpObjUUsesInteractableNotUsable(t *testing.T) {
	s, p, _, cc := makeOpObjFixture(t)
	// Usable=true but Interactable=false — should FAIL the gate at 244.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Usable: true, Interactable: false},
	})
	p.tabs[0] = 149

	received := drainConn(t, cc)
	_ = handleOpObjU(p, p2x6ObjPayload(100, 100, 42, 1511, 3, 149))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for Usable=true/Interactable=false component")
	}
	if p.target != nil {
		t.Error("target should be nil when Interactable=false (gate must use Interactable, not Usable)")
	}
}

// TestHandleOpObjUComponentCheckClearsPendingAction pins the 244 delta:
// the combined component check now calls clearPendingAction().
// TS OpObjUHandler.ts:21-26 (244).
func TestHandleOpObjUComponentCheckClearsPendingAction(t *testing.T) {
	_, p, obj, cc := makeOpObjFixture(t)
	// Nil component (not registered) → gate fires.
	p.target = obj
	p.targetOp = 9

	received := drainConn(t, cc)
	_ = handleOpObjU(p, p2x6ObjPayload(100, 100, 42, 1511, 3, 9999)) // comId 9999 not registered
	p.client.flushWrite()
	<-received

	if p.targetOp == -1 {
		t.Errorf("targetOp cleared to -1; 254 @2e3bcf43: rejection branches no longer clearPendingAction")
	}
	if p.target == nil {
		t.Error("target sentinel must survive the reject (no clearPendingAction at 2e3bcf43)")
	}
}

// TestHandleOpObjUOutOfViewportClearsPendingAction pins the 244 delta:
// viewport gate now calls clearPendingAction().
// TS OpObjUHandler.ts:31-36 (244).
func TestHandleOpObjUOutOfViewportClearsPendingAction(t *testing.T) {
	s, p, obj, cc := makeOpObjFixture(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true, Usable: true},
	})
	p.tabs[0] = 149
	p.target = obj
	p.targetOp = 9

	received := drainConn(t, cc)
	_ = handleOpObjU(p, p2x6ObjPayload(250, 100, 42, 1511, 3, 149)) // dx=150 > 52
	p.client.flushWrite()
	<-received

	if p.targetOp == -1 {
		t.Errorf("targetOp cleared to -1; 254 @2e3bcf43: rejection branches no longer clearPendingAction")
	}
	if p.target == nil {
		t.Error("target sentinel must survive the reject (no clearPendingAction at 2e3bcf43)")
	}
}

// TestHandleOpObjUMissingObjClearsPendingAction pins the 244 delta:
// missing obj now calls clearPendingAction() (and still sends UnsetMapFlag).
// 244 order: component → viewport → listener → inv → obj.
// TS OpObjUHandler.ts:52-57 (244).
func TestHandleOpObjUMissingObjClearsPendingAction(t *testing.T) {
	s, p, obj, cc := makeOpObjFixture(t)
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true, Usable: true},
	})
	p.tabs[0] = 149
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 1511, Count: 1}
	s.invs[93] = inv
	p.invListenOnCom(93, 149, -1)
	p.target = obj
	p.targetOp = 9

	received := drainConn(t, cc)
	_ = handleOpObjU(p, p2x6ObjPayload(100, 100, 999, 1511, 3, 149)) // objId 999 → not found
	p.client.flushWrite()
	<-received

	if p.targetOp == -1 {
		t.Errorf("targetOp cleared to -1; 254 @2e3bcf43: rejection branches no longer clearPendingAction")
	}
	if p.target == nil {
		t.Error("target sentinel must survive the reject (no clearPendingAction at 2e3bcf43)")
	}
}

// TestHandleOpObjUComponentCheckBeforeObj RE-PINNED at 2e3bcf43: the
// 254 pin reorders the gates again — viewport and obj lookup now fire
// BEFORE the component check (TS OpObjUHandler.ts:23-49). With a
// missing obj AND an unregistered component, the obj gate rejects
// first; either way no clearPendingAction runs on a reject. Test name
// kept for history greppability.
func TestHandleOpObjUComponentCheckBeforeObj(t *testing.T) {
	_, p, obj, cc := makeOpObjFixture(t)
	// Do NOT register component 9999.
	p.target = obj
	p.targetOp = 9

	received := drainConn(t, cc)
	// objId=999 doesn't exist AND comId=9999 not registered — the obj
	// gate (TS:33-38) fires before the component gate (TS:40-49).
	_ = handleOpObjU(p, p2x6ObjPayload(100, 100, 999, 1511, 3, 9999))
	p.client.flushWrite()
	<-received // UnsetMapFlag from the obj gate

	if p.targetOp == -1 {
		t.Errorf("targetOp cleared to -1; 254 @2e3bcf43: rejection branches no longer clearPendingAction")
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
	if !bytes.Contains(got, []byte("Nothing interesting happens.")) {
		t.Errorf("expected \"Nothing interesting happens.\" in drained bytes, got %x", got)
	}
}

// TestTryFireOpTriggerObjScriptFires verifies a registered [opobj1,<typeID>]
// script fires, and target is restored after Finished. NAI-68: dropped
// Finished/Aborted ClearInteraction — target preserved until tail clears.
func TestTryFireOpTriggerObjScriptFires(t *testing.T) {
	s, p, obj, _ := makeOpObjTriggerFixture(t)

	sf := newNoopScriptFile(t, script.TriggerOpObj1, obj.Type, -1)
	s.scriptProvider.Register(sf)

	tryFireOpTrigger(p)

	// NAI-68: target restored to obj (not nil); processInteraction tail clears.
	if p.target != obj {
		t.Errorf("target: got %v, want obj (restored after Finished — NAI-68)", p.target)
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
}

// TestTryFireOpTriggerObjFiresObjUTrigger verifies targetOpObjU → OPOBJU dispatch.
// NAI-68: target restored after Finished (not nil).
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

	// NAI-68: target restored to obj (not nil); processInteraction tail clears.
	if p.target != obj {
		t.Errorf("target: got %v, want obj (restored after OPOBJU fire — NAI-68)", p.target)
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
	if p.apRange != -1 {
		t.Errorf("apRange: got %d, want -1 (sentinel for no-AP-script)", p.apRange)
	}
}

// TestTryFireApTriggerObjScriptFiresNoApRangeCalled verifies an APOBJ script
// that runs but doesn't call p_aprange leaves p.target as the original obj
// (ClearInteraction deferred to processInteraction tail per NAI-68). nextTarget nil.
func TestTryFireApTriggerObjScriptFiresNoApRangeCalled(t *testing.T) {
	s, p, obj, _ := makeApObjTriggerFixture(t)

	sf := newNoopScriptFile(t, script.TriggerApObj1, obj.Type, -1)
	s.scriptProvider.Register(sf)

	tryFireApTrigger(p)

	// NAI-68: ClearInteraction dropped from fire helper; p.target restored to
	// original obj. Auto-clear happens in processInteraction tail's else-if.
	if p.target != obj {
		t.Errorf("target: got %v, want obj (restored — NAI-68)", p.target)
	}
	if p.nextTarget != nil {
		t.Errorf("nextTarget: got %v, want nil (no p_op_* in script)", p.nextTarget)
	}
}

// TestHandleOpObjReachesInteractionWithExplicitTakeOp pins that an obj whose
// type has Op[2]="Take" (set via cache decode code 32) reaches the interaction
// — i.e., the op_slot_empty gate passes. This is the post-NAI-152-B1 regression
// for the static-obj pickup symptom.
//
// Note: TS ObjType.ts:147 (244) changed the class-default for op from
// [null,null,'Take',null,null] to null. Items only have "Take" when the packer
// explicitly emits code 32 for them. This test sets Op explicitly to mirror an
// item that has code 32 in the cache.
func TestHandleOpObjReachesInteractionWithExplicitTakeOp(t *testing.T) {
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()

	s.objTypes = &objtype.ObjTypeConfigs{
		Configs: make([]*objtype.ObjType, 559),
	}
	ot := objtype.NewObjType(558)
	ot.DebugName = "mindrune"
	// Explicit Op[2]="Take" — mirrors a cache that wrote code 32 for this item.
	// TS ObjType.ts:147 (244): op default is null; "Take" only appears when the
	// packer writes it via decode code 32.
	ot.Op = []string{"", "", "Take", "", ""}
	s.objTypes.Configs[558] = ot

	obj := entitypkg.NewObj(0, 100, 100, entitypkg.LifecycleRespawn, 558, 1)
	obj.IsActive = true // L9: GetObj now filters !IsActive; production AddObj sets this.
	zn := s.zoneMap.Get(0, 100, 100)
	zn.Objs = append(zn.Objs, obj)

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 99, 100, 0
	p.originX, p.originZ = 100, 100

	if err := handleOpObj3(p, p2x3ObjPayload(100, 100, 558)); err != nil {
		t.Fatalf("handleOpObj3: %v", err)
	}

	if p.target != obj {
		t.Errorf("target: got %v, want obj (gate must pass when Op[2]=\"Take\")", p.target)
	}
	if p.targetOp != 3 {
		t.Errorf("targetOp: got %d, want 3", p.targetOp)
	}
	if p.interactionKind != InteractionEngine {
		t.Errorf("interactionKind: got %v, want InteractionEngine", p.interactionKind)
	}
	if !p.opcalled {
		t.Error("opcalled: want true")
	}
}

// TestHandleOpObjRejectsWhenOpNil — UPDATED for rev-244 (B2).
//
// At 225 op3 defaulted to "Take" and was validated, so nil Op on op3 rejected.
// At 244 TS OpObjHandler.ts:36-42 only gates op1 (index 0) and op4 (index 3);
// op3 is completely ungated ("todo: validate all options"). A nil Op slice
// with op3 therefore PASSES the gate at 244.
//
// The companion TestHandleOpObjRejectsEmptyOpSlot still pins op1 rejection
// (index 0 gated), and TestHandleOpObjGateOnlyOp1Op4 pins ops 2/3/5 passing
// even with empty Op slots.
func TestHandleOpObjRejectsWhenOpNil(t *testing.T) {
	s := newTestServer(t)
	s.zoneMap = zone.NewZoneMap()

	s.objTypes = &objtype.ObjTypeConfigs{
		Configs: make([]*objtype.ObjType, 559),
	}
	ot := objtype.NewObjType(558)
	ot.DebugName = "no_take_item"
	// Op remains nil — no cache codes. At 244 op3 is ungated, so nil Op passes.
	s.objTypes.Configs[558] = ot

	obj := entitypkg.NewObj(0, 100, 100, entitypkg.LifecycleRespawn, 558, 1)
	obj.IsActive = true
	zn := s.zoneMap.Get(0, 100, 100)
	zn.Objs = append(zn.Objs, obj)

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 99, 100, 0
	p.originX, p.originZ = 100, 100

	if err := handleOpObj3(p, p2x3ObjPayload(100, 100, 558)); err != nil {
		t.Fatalf("handleOpObj3: %v", err)
	}

	// 244: op3 is ungated → gate passes even with nil Op.
	if p.target != obj {
		t.Errorf("target: got %v, want obj (op3 ungated at 244 — TS OpObjHandler.ts:36-42)", p.target)
	}
	if !p.opcalled {
		t.Error("opcalled: want true (op3 ungated at 244)")
	}
}
