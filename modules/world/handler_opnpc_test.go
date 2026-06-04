package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/inventory"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/objtype"
)

// makeOpNpcFixture creates a server + player + npc ready for handleOpNpc tests.
// Returns (server, player, clientConn, npc).
func makeOpNpcFixture(t *testing.T) (*Server, *Player, *Npc) {
	t.Helper()
	s := newTestServer(t)

	typ := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 0, DebugName: "test"},
		Op:          []string{"Attack", "Talk", "Examine", "Option4", "Option5"},
		WanderRange: 0,
		RespawnRate: 50,
	}
	npc := NewNpc(1, 0, 100, 100, 0, typ)
	npc.nid = 1
	s.npcs[1] = npc
	s.npcLoop = append(s.npcLoop, npc)

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 99, 100, 0

	p.slot = 1
	s.players[1] = p
	s.rsbuf.AddPlayer(1)
	s.rsbuf.SubscribeNpcForTest(1, int32(npc.nid)) // nid=1

	return s, p, npc
}

// p2Payload encodes a big-endian uint16 into a 2-byte slice.
func p2Payload(slot int) []byte {
	return []byte{byte(slot >> 8), byte(slot)}
}

// TestHandleOpNpc1SetsInteraction verifies a valid request sets interaction state.
func TestHandleOpNpc1SetsInteraction(t *testing.T) {
	_, p, npc := makeOpNpcFixture(t)

	if err := handleOpNpc1(p, p2Payload(1)); err != nil {
		t.Fatalf("handleOpNpc1: %v", err)
	}

	if p.target != npc {
		t.Errorf("target: got %v, want npc", p.target)
	}
	if p.targetOp != 1 {
		t.Errorf("targetOp: got %d, want 1", p.targetOp)
	}
	if p.interactionKind != InteractionEngine {
		t.Errorf("interactionKind: got %v, want InteractionEngine", p.interactionKind)
	}
}

// TestHandleOpNpc1InvalidSlotSendsUnsetMapFlag verifies out-of-bounds slot emits UnsetMapFlag.
func TestHandleOpNpc1InvalidSlotSendsUnsetMapFlag(t *testing.T) {
	s := newTestServer(t)

	p2, cc2 := newTestPlayer(t)
	p2.client.server = s
	p2.client.encryptor = io2.New([4]uint32{2, 3, 4, 5})

	received := drainConn(t, cc2)
	_ = handleOpNpc1(p2, p2Payload(9999)) // slot 9999 > 8191
	p2.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag byte, got nothing")
	}
	if p2.target != nil {
		t.Error("target should remain nil for invalid slot")
	}
}

// TestHandleOpNpc1DeadNpcSendsUnsetMapFlag verifies dead NPC emits UnsetMapFlag.
func TestHandleOpNpc1DeadNpcSendsUnsetMapFlag(t *testing.T) {
	s, _, _ := makeOpNpcFixture(t)

	p2, cc2 := newTestPlayer(t)
	p2.client.server = s
	p2.client.encryptor = io2.New([4]uint32{3, 4, 5, 6})

	// Mark the npc dead.
	s.npcs[1].dead = true

	received := drainConn(t, cc2)
	_ = handleOpNpc1(p2, p2Payload(1))
	p2.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for dead NPC, got nothing")
	}
	if p2.target != nil {
		t.Error("target should remain nil for dead NPC")
	}
}

// TestHandleOpNpc1HiddenOpAccepted244 pins the 244 op-gate change: at 225 the
// gate was `Op[i]=="" || Op[i]=="hidden"`; at 244 it is simply `Op[i]==""` (the
// explicit "hidden" comparison was removed from TS OpNpcHandler.ts:35).
// "hidden" is a non-empty truthy string → the gate passes and interaction is set.
// TS OpNpcHandler.ts:35 (244): `!npcType.op || !npcType.op[message.op - 1]`
func TestHandleOpNpc1HiddenOpAccepted244(t *testing.T) {
	s, _, npc := makeOpNpcFixture(t)
	s.npcs[1].typ.Op[0] = "hidden"

	p2, _ := newTestPlayer(t)
	p2.client.server = s
	p2.client.encryptor = io2.New([4]uint32{4, 5, 6, 7})
	p2.slot = 2
	rsbufSeesNpc(t, s, 2, 1)

	if err := handleOpNpc1(p2, p2Payload(1)); err != nil {
		t.Fatalf("handleOpNpc1: %v", err)
	}
	if p2.target != npc {
		t.Error("244: 'hidden' op is truthy — gate must pass and set interaction")
	}
}

// TestHandleOpNpc1EmptyOpClearsPendingAction pins the 244 behaviour for empty-string
// op: rejects with UnsetMapFlag AND calls clearPendingAction (target→nil).
// TS OpNpcHandler.ts:35-38 (244).
func TestHandleOpNpc1EmptyOpClearsPendingAction(t *testing.T) {
	s, _, _ := makeOpNpcFixture(t)
	s.npcs[1].typ.Op[0] = "" // falsy empty string — op gate rejects

	p2, _ := newTestPlayer(t)
	p2.client.server = s
	p2.client.encryptor = io2.New([4]uint32{4, 5, 6, 7})
	p2.slot = 2
	rsbufSeesNpc(t, s, 2, 1)
	p2.target = p2 // sentinel: clearPendingAction will nil this

	_ = handleOpNpc1(p2, p2Payload(1))

	if p2.target != nil {
		t.Error("244: empty-op reject must call clearPendingAction (target→nil)")
	}
}

// TestHandleOpNpc2RoutesToOp2 verifies opNpc2 sets targetOp=2.
func TestHandleOpNpc2RoutesToOp2(t *testing.T) {
	_, p, npc := makeOpNpcFixture(t)

	if err := handleOpNpc2(p, p2Payload(1)); err != nil {
		t.Fatalf("handleOpNpc2: %v", err)
	}

	if p.target != npc {
		t.Errorf("target: got %v, want npc", p.target)
	}
	if p.targetOp != 2 {
		t.Errorf("targetOp: got %d, want 2", p.targetOp)
	}
}

// TestHandleOpNpc3RoutesToOp3 verifies opNpc3 sets targetOp=3.
func TestHandleOpNpc3RoutesToOp3(t *testing.T) {
	_, p, _ := makeOpNpcFixture(t)

	if err := handleOpNpc3(p, p2Payload(1)); err != nil {
		t.Fatalf("handleOpNpc3: %v", err)
	}
	if p.targetOp != 3 {
		t.Errorf("targetOp: got %d, want 3", p.targetOp)
	}
}

// TestHandleOpNpcShortPayloadSendsUnsetMapFlag verifies < 2 byte payload emits UnsetMapFlag.
func TestHandleOpNpcShortPayloadSendsUnsetMapFlag(t *testing.T) {
	s := newTestServer(t)

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{5, 6, 7, 8})

	received := drainConn(t, cc)
	_ = handleOpNpc1(p, []byte{0x01}) // only 1 byte
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for short payload, got nothing")
	}
	if p.target != nil {
		t.Error("target should be nil for short payload")
	}
}

// TestHandleOpNpcDelayedPlayerRejected verifies delayed player gets UnsetMapFlag,
// no state change, and does NOT call clearPendingAction.
// TS OpNpcHandler.ts:16-19 (244): delayed → write(UnsetMapFlag) + return false,
// no clearPendingAction.
func TestHandleOpNpcDelayedPlayerRejected(t *testing.T) {
	s, _, _ := makeOpNpcFixture(t)

	p2, cc2 := newTestPlayer(t)
	p2.client.server = s
	p2.client.encryptor = io2.New([4]uint32{6, 7, 8, 9})
	p2.delayed = true
	p2.delayedUntil = 999
	s.currentTick = 0
	p2.target = p2 // sentinel: if clearPendingAction is called target→nil

	received := drainConn(t, cc2)
	_ = handleOpNpc1(p2, p2Payload(1))
	p2.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for delayed player, got nothing")
	}
	// 244: delayed player must NOT call clearPendingAction.
	if p2.target != p2 {
		t.Error("244: delayed-player reject must NOT call clearPendingAction (target sentinel cleared)")
	}
}

// TestHandleOpNpc1NilNpcClearsPendingAction pins the 244 merged reject:
// !npc || npc.delayed → UnsetMapFlag + clearPendingAction.
// TS OpNpcHandler.ts:21-25 (244).
func TestHandleOpNpc1NilNpcClearsPendingAction(t *testing.T) {
	s := newTestServer(t)
	// s.npcs[1] is nil

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{9, 8, 7, 6})
	p.target = p // sentinel

	_ = handleOpNpc1(p, p2Payload(1))

	if p.target != nil {
		t.Error("244: nil-npc reject must call clearPendingAction (target sentinel should be nil)")
	}
}

// TestHandleOpNpc1NotVisibleClearsPendingAction pins the 244 rsbuf-reject change:
// !hasNpc(player.pid, npc.nid) → UnsetMapFlag + clearPendingAction.
// TS OpNpcHandler.ts:27-30 (244).
func TestHandleOpNpc1NotVisibleClearsPendingAction(t *testing.T) {
	s, _, _ := makeOpNpcFixture(t)

	// Fresh player NOT subscribed to npc nid=1.
	p2, _ := newTestPlayer(t)
	p2.client.server = s
	p2.client.encryptor = io2.New([4]uint32{10, 11, 12, 13})
	p2.slot = 2
	s.players[2] = p2
	s.rsbuf.AddPlayer(2) // registered but NOT subscribed to npc nid=1
	p2.target = p2       // sentinel

	_ = handleOpNpc1(p2, p2Payload(1))

	if p2.target != nil {
		t.Error("244: rsbuf-invisible reject must call clearPendingAction (target sentinel should be nil)")
	}
}

// TestHandleOpNpcNilNpcSendsUnsetMapFlag verifies nil npc slot emits UnsetMapFlag.
func TestHandleOpNpcNilNpcSendsUnsetMapFlag(t *testing.T) {
	s := newTestServer(t)
	// s.npcs[1] is nil (no npc registered)

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{9, 8, 7, 6})

	received := drainConn(t, cc)
	_ = handleOpNpc1(p, p2Payload(1))
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for nil npc, got nothing")
	}
}

// p2x2Payload encodes (a: u16, b: u16) into 4 bytes big-endian.
// Used by OpNpcT payload construction: slot + spellCom.
func p2x2Payload(a, b int) []byte {
	return []byte{
		byte(a >> 8), byte(a),
		byte(b >> 8), byte(b),
	}
}

// TestHandleOpNpcTSetsInteraction verifies a valid OpNpcT request sets
// interaction state with targetOp=targetOpNpcT and targetSubject.com
// carrying the spellCom.
func TestHandleOpNpcTSetsInteraction(t *testing.T) {
	s, p, npc := makeOpNpcFixture(t)

	// gate satisfaction: register spellCom with passing ActionTarget bit and visibility.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		7777: {RootLayer: 7777, ActionTarget: objtype.ComActionTargetNpc},
	})
	p.tabs[0] = 7777

	if err := handleOpNpcT(p, p2x2Payload(1, 7777)); err != nil {
		t.Fatalf("handleOpNpcT: %v", err)
	}

	if p.target != npc {
		t.Errorf("target: got %v, want npc", p.target)
	}
	if p.targetOp != targetOpNpcT {
		t.Errorf("targetOp: got %d, want targetOpNpcT (%d)", p.targetOp, targetOpNpcT)
	}
	if p.targetSubject.com != 7777 {
		t.Errorf("targetSubject.com: got %d, want 7777 (spellCom)", p.targetSubject.com)
	}
	if p.interactionKind != InteractionEngine {
		t.Errorf("interactionKind: got %v, want InteractionEngine", p.interactionKind)
	}
}

// TestHandleOpNpcTDelayedPlayerRejected verifies delayed → UnsetMapFlag, no clearPendingAction.
// TS OpNpcTHandler.ts:16-19 (244): delayed → write(UnsetMapFlag) + return false,
// no clearPendingAction.
func TestHandleOpNpcTDelayedPlayerRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	p.delayed = true
	p.delayedUntil = 999
	s.currentTick = 0
	p.target = p // sentinel: clearPendingAction would nil this

	_ = handleOpNpcT(p, p2x2Payload(1, 7777))

	// 244: delayed player must NOT call clearPendingAction.
	if p.target != p {
		t.Error("244: delayed-player reject must NOT call clearPendingAction (target sentinel cleared)")
	}
}

// TestHandleOpNpcTShortPayloadRejected verifies <4 bytes → UnsetMapFlag.
func TestHandleOpNpcTShortPayloadRejected(t *testing.T) {
	_, p, _ := makeOpNpcFixture(t)

	_ = handleOpNpcT(p, []byte{0x00, 0x01}) // only 2 bytes

	if p.target != nil {
		t.Error("target should remain nil for short payload")
	}
}

// TestHandleOpNpcTInvalidSlotRejected verifies slot >= len(s.npcs) → UnsetMapFlag.
func TestHandleOpNpcTInvalidSlotRejected(t *testing.T) {
	_, p, _ := makeOpNpcFixture(t)

	_ = handleOpNpcT(p, p2x2Payload(9999, 7777)) // slot 9999 > len(s.npcs)

	if p.target != nil {
		t.Error("target should remain nil for invalid slot")
	}
}

// TestHandleOpNpcTDeadNpcRejected verifies dead NPC → UnsetMapFlag + clearPendingAction.
// TS OpNpcTHandler.ts:28-32 (244).
func TestHandleOpNpcTDeadNpcRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	// Register a component so the component gate (gate 3) passes and the
	// dead-npc gate (gate 4) is actually reached.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		7777: {RootLayer: 7777, ActionTarget: objtype.ComActionTargetNpc},
	})
	p.tabs[0] = 7777
	s.npcs[1].dead = true
	p.target = p // sentinel

	_ = handleOpNpcT(p, p2x2Payload(1, 7777))

	if p.target != nil {
		t.Error("244: dead-npc reject must call clearPendingAction (target sentinel should be nil)")
	}
}

// TestHandleOpNpcTComponentClearsPendingAction pins the 244 combined component
// check: undefined component → UnsetMapFlag + clearPendingAction.
// TS OpNpcTHandler.ts:21-26 (244): all component sub-gates merged into one block
// that now calls clearPendingAction on any component failure.
func TestHandleOpNpcTComponentClearsPendingAction(t *testing.T) {
	_, p, _ := makeOpNpcFixture(t)
	// No component seeded → lookupComponent returns nil → gate rejects.
	p.target = p // sentinel

	_ = handleOpNpcT(p, p2x2Payload(1, 7777))

	if p.target != nil {
		t.Error("244: component-fail reject must call clearPendingAction (target sentinel should be nil)")
	}
}

// TestHandleOpNpcTNilNpcClearsPendingAction pins the 244 merged npc reject:
// !npc || npc.delayed → UnsetMapFlag + clearPendingAction.
// TS OpNpcTHandler.ts:28-32 (244).
func TestHandleOpNpcTNilNpcClearsPendingAction(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	// Register a component so the component gate passes.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		7777: {RootLayer: 7777, ActionTarget: objtype.ComActionTargetNpc},
	})
	p.tabs[0] = 7777
	// NPC slot 1 exists but make it nil.
	s.npcs[1] = nil
	p.target = p // sentinel

	_ = handleOpNpcT(p, p2x2Payload(1, 7777))

	if p.target != nil {
		t.Error("244: nil-npc reject must call clearPendingAction (target sentinel should be nil)")
	}

}

// p2x4NpcUPayload encodes 4 u16 values into 8 bytes big-endian.
// Used by OpNpcU payload construction: slot + useObj + useSlot + useCom.
// Named p2x4NpcUPayload (not p2x4Payload) to avoid collision with
// handler_oploc_test.go's p2x4Payload which has a different semantic.
func p2x4NpcUPayload(a, b, c, d int) []byte {
	return []byte{
		byte(a >> 8), byte(a),
		byte(b >> 8), byte(b),
		byte(c >> 8), byte(c),
		byte(d >> 8), byte(d),
	}
}

// TestHandleOpNpcUSetsInteraction verifies a valid OpNpcU request sets
// interaction state with targetOp=targetOpNpcU, stores useObj/useSlot
// in p.lastUseItem/lastUseSlot, and passes -1 for com (useCom discarded).
// 244: component gate now checks Interactable (not Usable).
// TS OpNpcUHandler.ts:23-28 (244).
func TestHandleOpNpcUSetsInteraction(t *testing.T) {
	s, p, npc := makeOpNpcFixture(t)

	// Seed component so the component gate passes.
	// 244: gate checks com.interactable (TS OpNpcUHandler.ts:24), not com.usable.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true},
	})
	p.tabs[0] = 149

	// Register listener + populate the inv with the claimed item.
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 1511, Count: 1}
	s.invs[93] = inv
	p.invListenOnCom(93, 149, -1)

	if err := handleOpNpcU(p, p2x4NpcUPayload(1, 1511, 3, 149)); err != nil {
		t.Fatalf("handleOpNpcU: %v", err)
	}

	if p.target != npc {
		t.Errorf("target: got %v, want npc", p.target)
	}
	if p.targetOp != targetOpNpcU {
		t.Errorf("targetOp: got %d, want targetOpNpcU (%d)", p.targetOp, targetOpNpcU)
	}
	if p.lastUseItem != 1511 {
		t.Errorf("lastUseItem: got %d, want 1511 (useObj)", p.lastUseItem)
	}
	if p.lastUseSlot != 3 {
		t.Errorf("lastUseSlot: got %d, want 3", p.lastUseSlot)
	}
	if p.targetSubject.com != -1 {
		t.Errorf("targetSubject.com: got %d, want -1 (OpNpcU passes -1)", p.targetSubject.com)
	}
}

// TestHandleOpNpcUDelayedPlayerRejected verifies delayed → UnsetMapFlag,
// no clearPendingAction, and lastUseItem is NOT clobbered.
// TS OpNpcUHandler.ts:17-20 (244): delayed → write(UnsetMapFlag) + return false,
// no clearPendingAction.
func TestHandleOpNpcUDelayedPlayerRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	p.delayed = true
	p.delayedUntil = 999
	s.currentTick = 0
	p.lastUseItem = 42 // sentinel: must stay unchanged on rejection
	p.target = p       // sentinel: clearPendingAction would nil this; must stay p

	_ = handleOpNpcU(p, p2x4NpcUPayload(1, 1511, 3, 149))

	// 244: delayed player must NOT call clearPendingAction.
	if p.target != p {
		t.Error("244: delayed-player reject must NOT call clearPendingAction (target sentinel cleared)")
	}
	if p.lastUseItem != 42 {
		t.Errorf("lastUseItem leaked through rejected handler: got %d, want 42", p.lastUseItem)
	}
}

// TestHandleOpNpcUShortPayloadRejected verifies <8 bytes → UnsetMapFlag.
func TestHandleOpNpcUShortPayloadRejected(t *testing.T) {
	_, p, _ := makeOpNpcFixture(t)

	_ = handleOpNpcU(p, []byte{0x00, 0x01, 0x00, 0x02}) // only 4 bytes

	if p.target != nil {
		t.Error("target should remain nil for short payload")
	}
}

// TestHandleOpNpcUInvalidSlotRejected verifies slot OOB → UnsetMapFlag.
func TestHandleOpNpcUInvalidSlotRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	// Seed component and listener so gates pass and the NPC slot OOB gate fires.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true},
	})
	p.tabs[0] = 149
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 1511, Count: 1}
	s.invs[93] = inv
	p.invListenOnCom(93, 149, -1)

	_ = handleOpNpcU(p, p2x4NpcUPayload(9999, 1511, 3, 149)) // slot 9999 OOB

	if p.target != nil {
		t.Error("target should remain nil for invalid slot")
	}
}

// TestHandleOpNpcUDeadNpcRejected verifies dead NPC → UnsetMapFlag + clearPendingAction.
// TS OpNpcUHandler.ts:44-48 (244).
func TestHandleOpNpcUDeadNpcRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	// Seed component and listener so gates pass and the dead-NPC gate fires.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true},
	})
	p.tabs[0] = 149
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 1511, Count: 1}
	s.invs[93] = inv
	p.invListenOnCom(93, 149, -1)
	s.npcs[1].dead = true
	p.target = p // sentinel

	_ = handleOpNpcU(p, p2x4NpcUPayload(1, 1511, 3, 149))

	if p.target != nil {
		t.Error("244: dead-npc reject must call clearPendingAction (target sentinel should be nil)")
	}
}

// TestHandleOpNpcUComponentInteractableClearsPendingAction pins the 244 component-gate
// change: com.usable replaced by com.interactable, and all sub-gate failures now call
// clearPendingAction. TS OpNpcUHandler.ts:23-27 (244).
func TestHandleOpNpcUComponentInteractableClearsPendingAction(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	// Seed component with Interactable=false (explicitly not interactable).
	// Gate: !com.interactable → reject + clearPendingAction.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: false},
	})
	p.tabs[0] = 149
	p.target = p // sentinel

	_ = handleOpNpcU(p, p2x4NpcUPayload(1, 1511, 3, 149))

	if p.target != nil {
		t.Error("244: !interactable reject must call clearPendingAction (target sentinel should be nil)")
	}
}

// TestHandleOpNpcUMissingListenerRejected — S6p closes S6o-D3. A useCom
// with no registered listener rejects with UnsetMapFlag.
func TestHandleOpNpcUMissingListenerRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	// Seed component so the component gate passes; listener-missing gate fires next.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true},
	})
	p.tabs[0] = 149
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	s.invs[93] = inventory.New(93, 28, inventory.StackNormal)
	// NO invListenOnCom.

	p.lastUseItem = 77 // sentinel: must stay unchanged
	_ = handleOpNpcU(p, p2x4NpcUPayload(1, 1511, 3, 149))

	if p.target != nil {
		t.Error("target should remain nil for missing listener")
	}
	if p.lastUseItem != 77 {
		t.Errorf("lastUseItem leaked through rejected handler: got %d, want 77", p.lastUseItem)
	}
}

// TestHandleOpNpcUInvalidInvSlotRejected verifies useSlot OOB of the
// registered inv → UnsetMapFlag.
func TestHandleOpNpcUInvalidInvSlotRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	// Seed component so the component gate passes.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true},
	})
	p.tabs[0] = 149
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	s.invs[93] = inventory.New(93, 28, inventory.StackNormal)
	p.invListenOnCom(93, 149, -1)

	p.lastUseItem = 77 // sentinel: must stay unchanged
	// useSlot=99, OOB.
	_ = handleOpNpcU(p, p2x4NpcUPayload(1, 1511, 99, 149))

	if p.target != nil {
		t.Error("target should remain nil for invalid slot")
	}
	if p.lastUseItem != 77 {
		t.Errorf("lastUseItem leaked through rejected handler: got %d, want 77", p.lastUseItem)
	}
}

// TestHandleOpNpcUItemMismatchRejected verifies slot N holds a
// different id than useObj → UnsetMapFlag.
func TestHandleOpNpcUItemMismatchRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	// Seed component so the component gate passes.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true},
	})
	p.tabs[0] = 149
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 9999, Count: 1} // NOT 1511
	s.invs[93] = inv
	p.invListenOnCom(93, 149, -1)

	p.lastUseItem = 77 // sentinel: must stay unchanged
	_ = handleOpNpcU(p, p2x4NpcUPayload(1, 1511, 3, 149))

	if p.target != nil {
		t.Error("target should remain nil for item mismatch")
	}
	if p.lastUseItem != 77 {
		t.Errorf("lastUseItem leaked through rejected handler: got %d, want 77", p.lastUseItem)
	}
}

// TestHandleOpNpcUHappyPathWithOtherPlayerInv verifies Source != -1
// path works through resolveListenerInv.
func TestHandleOpNpcUHappyPathWithOtherPlayerInv(t *testing.T) {
	s, p, npc := makeOpNpcFixture(t)

	// Seed component so the component gate passes.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true},
	})
	p.tabs[0] = 149

	other, _ := newTestPlayer(t)
	other.slot = 2
	other.uid = 0xDEADBEEF // arbitrary UID; must use UID not slot in invListenOnCom
	other.active = true
	other.invs = map[int]*inventory.Inventory{}
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 1511, Count: 1}
	other.invs[93] = inv
	s.players[2] = other
	s.playerLoop = append(s.playerLoop, other)

	p.invListenOnCom(93, 149, 0xDEADBEEF)

	if err := handleOpNpcU(p, p2x4NpcUPayload(1, 1511, 3, 149)); err != nil {
		t.Fatalf("handleOpNpcU: %v", err)
	}
	if p.target != npc {
		t.Errorf("target: got %v, want npc", p.target)
	}
}

// TestHandleOpNpcUMembersOnFreeWorldRejected — S6z closes S6o-D4.
func TestHandleOpNpcUMembersOnFreeWorldRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	// Seed component so the component gate passes.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true},
	})
	p.tabs[0] = 149
	s.cfg.NodeMembers = false
	if s.objTypes == nil {
		s.objTypes = &objtype.ObjTypeConfigs{Configs: make([]*objtype.ObjType, 2000)}
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

	_ = handleOpNpcU(p, p2x4NpcUPayload(1, 1511, 3, 149))

	if p.target != nil {
		t.Error("target should remain nil for members-on-free rejection")
	}
}

// TestHandleOpNpcUMembersOnMembersWorldAllowed — gate only fires on free.
func TestHandleOpNpcUMembersOnMembersWorldAllowed(t *testing.T) {
	s, p, npc := makeOpNpcFixture(t)
	// Seed component so the component gate passes.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true},
	})
	p.tabs[0] = 149
	s.cfg.NodeMembers = true
	if s.objTypes == nil {
		s.objTypes = &objtype.ObjTypeConfigs{Configs: make([]*objtype.ObjType, 2000)}
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

	if err := handleOpNpcU(p, p2x4NpcUPayload(1, 1511, 3, 149)); err != nil {
		t.Fatalf("handleOpNpcU: %v", err)
	}
	if p.target != npc {
		t.Errorf("target: got %v, want npc (members world should allow)", p.target)
	}
}

// rsbufSeesNpc makes s.rsbuf.HasNpc(playerSlot, nid) return true.
func rsbufSeesNpc(t *testing.T, s *Server, playerSlot, nid int) {
	t.Helper()
	s.rsbuf.AddPlayer(int32(playerSlot))
	s.rsbuf.SubscribeNpcForTest(int32(playerSlot), int32(nid))
}

// TestHandleOpNpcDelayedNpcRejected verifies delayed NPC sends UnsetMapFlag.
func TestHandleOpNpcDelayedNpcRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	s.npcs[1].delayed = true
	s.npcs[1].delayedUntil = 999
	s.currentTick = 0

	_ = handleOpNpc1(p, p2Payload(1))

	if p.target != nil {
		t.Error("target should remain nil for delayed NPC")
	}
}

// TestHandleOpNpcTDelayedNpcRejected verifies delayed NPC in handleOpNpcT sends UnsetMapFlag
// and calls clearPendingAction. TS OpNpcTHandler.ts:28-33 (244).
func TestHandleOpNpcTDelayedNpcRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	// Register a component so the component gate (gate 3) passes and the
	// delayed-npc gate (gate 4) is actually reached.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		7777: {RootLayer: 7777, ActionTarget: objtype.ComActionTargetNpc},
	})
	p.tabs[0] = 7777
	s.npcs[1].delayed = true
	s.npcs[1].delayedUntil = 999
	s.currentTick = 0
	p.target = p // sentinel: clearPendingAction will nil this

	_ = handleOpNpcT(p, p2x2Payload(1, 7777))

	if p.target != nil {
		t.Error("244: delayed-npc reject must call clearPendingAction (target sentinel should be nil)")
	}
}

// TestHandleOpNpcUDelayedNpcRejected verifies delayed NPC in handleOpNpcU sends UnsetMapFlag.
func TestHandleOpNpcUDelayedNpcRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	s.npcs[1].delayed = true
	s.npcs[1].delayedUntil = 999
	s.currentTick = 0
	// Seed component so the component gate passes; register listener + populate inv
	// so the delayed-npc gate fires (not the component/listener gate).
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true},
	})
	p.tabs[0] = 149
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 1511, Count: 1}
	s.invs[93] = inv
	p.invListenOnCom(93, 149, -1)

	_ = handleOpNpcU(p, p2x4NpcUPayload(1, 1511, 3, 149))

	if p.target != nil {
		t.Error("target should remain nil for delayed NPC")
	}
}

// TestHandleOpNpcNpcNotVisibleRejected verifies NPC not in rsbuf sends UnsetMapFlag.
func TestHandleOpNpcNpcNotVisibleRejected(t *testing.T) {
	s, _, _ := makeOpNpcFixture(t)

	// Create a fresh player NOT subscribed to the NPC via rsbuf.
	p2, cc2 := newTestPlayer(t)
	p2.client.server = s
	p2.client.encryptor = io2.New([4]uint32{10, 11, 12, 13})
	p2.slot = 2
	s.players[2] = p2
	s.rsbuf.AddPlayer(2) // registered but NOT subscribed to npc nid=1

	received := drainConn(t, cc2)
	_ = handleOpNpc1(p2, p2Payload(1))
	p2.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for invisible NPC, got nothing")
	}
	if p2.target != nil {
		t.Error("target should remain nil for invisible NPC")
	}
}

// TestHandleOpNpcTNpcNotVisibleRejected verifies NPC not visible in handleOpNpcT.
func TestHandleOpNpcTNpcNotVisibleRejected(t *testing.T) {
	s, _, _ := makeOpNpcFixture(t)

	p2, _ := newTestPlayer(t)
	p2.client.server = s
	p2.client.encryptor = io2.New([4]uint32{14, 15, 16, 17})
	p2.slot = 2
	s.players[2] = p2
	s.rsbuf.AddPlayer(2)

	_ = handleOpNpcT(p2, p2x2Payload(1, 7777))

	if p2.target != nil {
		t.Error("target should remain nil for invisible NPC (handleOpNpcT)")
	}
}

// TestHandleOpNpcUNpcNotVisibleRejected verifies NPC not visible in handleOpNpcU.
func TestHandleOpNpcUNpcNotVisibleRejected(t *testing.T) {
	s, _, _ := makeOpNpcFixture(t)

	p2, _ := newTestPlayer(t)
	p2.client.server = s
	p2.client.encryptor = io2.New([4]uint32{18, 19, 20, 21})
	p2.slot = 2
	s.players[2] = p2
	s.rsbuf.AddPlayer(2)
	// Seed component so the component gate passes; rsbuf-visibility gate fires next.
	seedComponentTypes(t, s, map[int]*objtype.ComponentType{
		149: {RootLayer: 149, Interactable: true},
	})
	p2.tabs[0] = 149
	if s.invs == nil {
		s.invs = make(map[int]*inventory.Inventory)
	}
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 1511, Count: 1}
	s.invs[93] = inv
	p2.invListenOnCom(93, 149, -1)

	_ = handleOpNpcU(p2, p2x4NpcUPayload(1, 1511, 3, 149))

	if p2.target != nil {
		t.Error("target should remain nil for invisible NPC (handleOpNpcU)")
	}
}

// TestHandleOpNpc1SetsOpcalled verifies success path sets p.opcalled=true.
func TestHandleOpNpc1SetsOpcalled(t *testing.T) {
	_, p, _ := makeOpNpcFixture(t)

	if err := handleOpNpc1(p, p2Payload(1)); err != nil {
		t.Fatalf("handleOpNpc1: %v", err)
	}

	if !p.opcalled {
		t.Error("opcalled: want true after successful handleOpNpc1, got false")
	}
}

// TestHandleOpNpcRejectedDoesNotSetOpcalled verifies rejection leaves p.opcalled=false.
func TestHandleOpNpcRejectedDoesNotSetOpcalled(t *testing.T) {
	s, _, _ := makeOpNpcFixture(t)

	p2, _ := newTestPlayer(t)
	p2.client.server = s
	p2.client.encryptor = io2.New([4]uint32{99, 98, 97, 96})
	// p2 has no rsbuf subscription → HasNpc gate rejects

	_ = handleOpNpc1(p2, p2Payload(1))

	if p2.opcalled {
		t.Error("opcalled: want false after rejection, got true")
	}
}

// TestProcessInResetsOpcalled verifies processIn sets p.opcalled to false.
func TestProcessInResetsOpcalled(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	p.opcalled = true // pre-set to true

	// Run processIn with an empty inbox — no packets, just the reset logic.
	p.processIn(s.currentTick)

	if p.opcalled {
		t.Error("opcalled: want false after processIn, got true")
	}
}

// TestHandleOpNpcOpIndexOutOfRange verifies NpcType with fewer Op entries emits UnsetMapFlag.
func TestHandleOpNpcOpIndexOutOfRange(t *testing.T) {
	s := newTestServer(t)

	typ := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 0, DebugName: "test"},
		Op:          []string{"Attack"}, // only 1 op
		WanderRange: 0,
		RespawnRate: 50,
	}
	npc := NewNpc(1, 0, 100, 100, 0, typ)
	npc.nid = 1
	s.npcs[1] = npc

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 1, 1, 1})
	p.slot = 1
	rsbufSeesNpc(t, s, 1, 1)

	received := drainConn(t, cc)
	_ = handleOpNpc2(p, p2Payload(1)) // op=2 but only Op[0] exists
	p.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for out-of-range op index, got nothing")
	}
	if p.target != nil {
		t.Error("target should remain nil")
	}
}
