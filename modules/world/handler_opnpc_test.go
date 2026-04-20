package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/grid"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/objtype"
)

// makeOpNpcFixture creates a server + player + npc ready for handleOpNpc tests.
// Returns (server, player, clientConn, npc).
func makeOpNpcFixture(t *testing.T) (*Server, *Player, *Npc) {
	t.Helper()
	s := newTestServer(t)
	s.grid = grid.New()

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
	s.grid = grid.New()

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
	s.grid = grid.New()

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
	s.grid = grid.New()
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

// TestHandleOpNpcOpIndexOutOfRange verifies NpcType with fewer Op entries emits UnsetMapFlag.
func TestHandleOpNpcOpIndexOutOfRange(t *testing.T) {
	s := newTestServer(t)
	s.grid = grid.New()

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
