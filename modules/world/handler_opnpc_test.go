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

// TestHandleOpNpc1HiddenOpSendsUnsetMapFlag verifies "hidden" Op emits UnsetMapFlag.
func TestHandleOpNpc1HiddenOpSendsUnsetMapFlag(t *testing.T) {
	s, _, _ := makeOpNpcFixture(t)
	s.npcs[1].typ.Op[0] = "hidden"

	p2, cc2 := newTestPlayer(t)
	p2.client.server = s
	p2.client.encryptor = io2.New([4]uint32{4, 5, 6, 7})
	p2.slot = 2
	rsbufSeesNpc(t, s, 2, 1)

	received := drainConn(t, cc2)
	_ = handleOpNpc1(p2, p2Payload(1))
	p2.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for hidden op, got nothing")
	}
	if p2.target != nil {
		t.Error("target should remain nil for hidden op")
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

// TestHandleOpNpcDelayedPlayerRejected verifies delayed player gets UnsetMapFlag, no state change.
func TestHandleOpNpcDelayedPlayerRejected(t *testing.T) {
	s, _, _ := makeOpNpcFixture(t)

	p2, cc2 := newTestPlayer(t)
	p2.client.server = s
	p2.client.encryptor = io2.New([4]uint32{6, 7, 8, 9})
	p2.delayed = true
	p2.delayedUntil = 999
	s.currentTick = 0

	received := drainConn(t, cc2)
	_ = handleOpNpc1(p2, p2Payload(1))
	p2.client.flushWrite()
	got := <-received

	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag for delayed player, got nothing")
	}
	if p2.target != nil {
		t.Error("target should remain nil for delayed player")
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
	_, p, npc := makeOpNpcFixture(t)

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

// TestHandleOpNpcTDelayedPlayerRejected verifies delayed → UnsetMapFlag.
func TestHandleOpNpcTDelayedPlayerRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	p.delayed = true
	p.delayedUntil = 999
	s.currentTick = 0

	_ = handleOpNpcT(p, p2x2Payload(1, 7777))

	if p.target != nil {
		t.Error("target should remain nil for delayed player")
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

// TestHandleOpNpcTDeadNpcRejected verifies dead NPC → UnsetMapFlag.
func TestHandleOpNpcTDeadNpcRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	s.npcs[1].dead = true

	_ = handleOpNpcT(p, p2x2Payload(1, 7777))

	if p.target != nil {
		t.Error("target should remain nil for dead NPC")
	}
}

// TestHandleOpNpcTMissingNpcTypeRejected verifies nil typ → UnsetMapFlag.
func TestHandleOpNpcTMissingNpcTypeRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	s.npcs[1].typ = nil

	_ = handleOpNpcT(p, p2x2Payload(1, 7777))

	if p.target != nil {
		t.Error("target should remain nil when NpcType is nil")
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
func TestHandleOpNpcUSetsInteraction(t *testing.T) {
	s, p, npc := makeOpNpcFixture(t)

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
// and that lastUseItem is NOT clobbered when validation fails (leak-
// prevention: state mutation happens only after all gates pass).
func TestHandleOpNpcUDelayedPlayerRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	p.delayed = true
	p.delayedUntil = 999
	s.currentTick = 0
	p.lastUseItem = 42 // sentinel: must stay unchanged on rejection

	_ = handleOpNpcU(p, p2x4NpcUPayload(1, 1511, 3, 149))

	if p.target != nil {
		t.Error("target should remain nil for delayed player")
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
	_, p, _ := makeOpNpcFixture(t)

	_ = handleOpNpcU(p, p2x4NpcUPayload(9999, 1511, 3, 149)) // slot 9999 OOB

	if p.target != nil {
		t.Error("target should remain nil for invalid slot")
	}
}

// TestHandleOpNpcUDeadNpcRejected verifies dead NPC → UnsetMapFlag.
func TestHandleOpNpcUDeadNpcRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	s.npcs[1].dead = true

	_ = handleOpNpcU(p, p2x4NpcUPayload(1, 1511, 3, 149))

	if p.target != nil {
		t.Error("target should remain nil for dead NPC")
	}
}

// TestHandleOpNpcUMissingNpcTypeRejected verifies nil typ → UnsetMapFlag.
func TestHandleOpNpcUMissingNpcTypeRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	s.npcs[1].typ = nil

	_ = handleOpNpcU(p, p2x4NpcUPayload(1, 1511, 3, 149))

	if p.target != nil {
		t.Error("target should remain nil when NpcType is nil")
	}
}

// TestHandleOpNpcUMissingListenerRejected — S6p closes S6o-D3. A useCom
// with no registered listener rejects with UnsetMapFlag.
func TestHandleOpNpcUMissingListenerRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
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

	other, _ := newTestPlayer(t)
	other.invs = map[int]*inventory.Inventory{}
	inv := inventory.New(93, 28, inventory.StackNormal)
	inv.Items[3] = &inventory.Item{Id: 1511, Count: 1}
	other.invs[93] = inv
	s.players[2] = other

	p.invListenOnCom(93, 149, 2)

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

// TestHandleOpNpcTDelayedNpcRejected verifies delayed NPC in handleOpNpcT sends UnsetMapFlag.
func TestHandleOpNpcTDelayedNpcRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	s.npcs[1].delayed = true
	s.npcs[1].delayedUntil = 999
	s.currentTick = 0

	_ = handleOpNpcT(p, p2x2Payload(1, 7777))

	if p.target != nil {
		t.Error("target should remain nil for delayed NPC")
	}
}

// TestHandleOpNpcUDelayedNpcRejected verifies delayed NPC in handleOpNpcU sends UnsetMapFlag.
func TestHandleOpNpcUDelayedNpcRejected(t *testing.T) {
	s, p, _ := makeOpNpcFixture(t)
	s.npcs[1].delayed = true
	s.npcs[1].delayedUntil = 999
	s.currentTick = 0
	// Register listener + populate inv so the test can reach the delayed-npc gate
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
