package world

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/grid"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/objtype"
)

// makeInteractionNpc builds a live NPC registered in s.npcs at the given slot.
func makeInteractionNpc(t *testing.T, s *Server, slot, x, z, level int) *Npc {
	t.Helper()
	typ := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 0, DebugName: "test"},
		Op:          []string{"Attack"},
		WanderRange: 0,
		RespawnRate: 50,
	}
	n := NewNpc(slot, 0, x, z, level, typ)
	n.nid = slot
	s.npcs[slot] = n
	s.npcLoop = append(s.npcLoop, n)
	return n
}

// makeInteractionPlayer wires a Player to the server with ISAAC pair and coords.
func makeInteractionPlayer(t *testing.T, s *Server, x, z, level int) (*Player, func()) {
	t.Helper()
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = x, z, level
	drain := drainConn(t, cc)
	return p, func() { <-drain }
}

// TestSetInteractionPopulatesFields checks that SetInteraction stores all fields.
func TestSetInteractionPopulatesFields(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)

	p, wait := makeInteractionPlayer(t, s, 99, 100, 0)
	defer wait()

	p.SetInteraction(InteractionEngine, npc, 3)

	if p.target != npc {
		t.Errorf("target: got %v, want npc", p.target)
	}
	if p.targetOp != 3 {
		t.Errorf("targetOp: got %d, want 3", p.targetOp)
	}
	if p.interactionKind != InteractionEngine {
		t.Errorf("interactionKind: got %v, want InteractionEngine", p.interactionKind)
	}
	if p.apRange != 10 {
		t.Errorf("apRange: got %d, want 10", p.apRange)
	}
	if p.apRangeCalled {
		t.Error("apRangeCalled should be false")
	}
	if p.interacted {
		t.Error("interacted should be false")
	}
	if p.repathed {
		t.Error("repathed should be false")
	}
}

// TestClearInteractionResetsAll verifies all fields return to idle.
func TestClearInteractionResetsAll(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)

	p, wait := makeInteractionPlayer(t, s, 99, 100, 0)
	defer wait()

	p.SetInteraction(InteractionEngine, npc, 1)
	p.interacted = true
	p.repathed = true
	p.apRangeCalled = true

	p.ClearInteraction()

	if p.target != nil {
		t.Errorf("target: got %v, want nil", p.target)
	}
	if p.targetOp != -1 {
		t.Errorf("targetOp: got %d, want -1", p.targetOp)
	}
	if p.apRangeCalled {
		t.Error("apRangeCalled should be false")
	}
	if p.interacted {
		t.Error("interacted should be false")
	}
	if p.repathed {
		t.Error("repathed should be false")
	}
}

// TestProcessInteractionNoTargetNoop verifies nil target is a no-op.
func TestProcessInteractionNoTargetNoop(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	// no target set

	p.processInteraction()

	if p.interacted {
		t.Error("interacted should remain false with no target")
	}
	if p.waypointIndex >= 0 {
		t.Error("no waypoint should be set with no target")
	}
}

// TestProcessInteractionInRangeFacesTarget verifies adjacent target triggers face + interacted.
func TestProcessInteractionInRangeFacesTarget(t *testing.T) {
	s := newTestServer(t)
	s.grid = grid.New()
	npc := makeInteractionNpc(t, s, 1, 101, 100, 0)

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 100, 100, 0

	p.SetInteraction(InteractionEngine, npc, 1)

	received := drainConn(t, cc)
	p.processInteraction()
	p.client.flushWrite()
	<-received

	if !p.interacted {
		t.Error("interacted should be true when adjacent to target")
	}
	if p.faceEntity != npc.nid {
		t.Errorf("faceEntity: got %d, want %d", p.faceEntity, npc.nid)
	}
	if p.masks&MaskFaceEntity == 0 {
		t.Error("MaskFaceEntity bit should be set")
	}
}

// TestProcessInteractionOutOfRangePaths verifies a distant target causes pathing.
func TestProcessInteractionOutOfRangePaths(t *testing.T) {
	s := newTestServer(t)
	s.cfg.NodeClientRoutefinder = true // use direct-step mode
	s.grid = grid.New()
	npc := makeInteractionNpc(t, s, 1, 105, 100, 0) // 5 tiles away

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 100, 100, 0

	p.SetInteraction(InteractionEngine, npc, 1)

	received := drainConn(t, cc)
	p.processInteraction()
	p.client.flushWrite()
	<-received

	if p.waypointIndex < 0 {
		t.Error("waypointIndex should be >= 0 after pathToTarget")
	}
	if !p.repathed {
		t.Error("repathed should be true after first out-of-range tick")
	}
	if p.interacted {
		t.Error("interacted should be false when out of range")
	}
}

// TestProcessInteractionDifferentLevelClears verifies level mismatch clears and emits UnsetMapFlag.
func TestProcessInteractionDifferentLevelClears(t *testing.T) {
	s := newTestServer(t)
	s.grid = grid.New()
	npc := makeInteractionNpc(t, s, 1, 100, 100, 1) // level 1

	p, cc := newTestPlayer(t)
	p.client.server = s
	enc := io2.New([4]uint32{1, 2, 3, 4})
	refEnc := io2.New([4]uint32{1, 2, 3, 4})
	p.client.encryptor = enc
	p.x, p.z, p.level = 100, 100, 0 // player on level 0

	p.SetInteraction(InteractionEngine, npc, 1)

	received := drainConn(t, cc)
	p.processInteraction()
	p.client.flushWrite()
	got := <-received

	// Expect UnsetMapFlag (opcode 19, 0 payload = just the encrypted opcode byte).
	want := byte((int(gameserver.OpUnsetMapFlag.Opcode) + int(refEnc.GetNext())) & 0xff)
	if len(got) == 0 {
		t.Fatal("expected UnsetMapFlag byte on wire, got nothing")
	}
	if got[0] != want {
		t.Errorf("wire byte: got %d, want %d (UnsetMapFlag)", got[0], want)
	}
	if p.target != nil {
		t.Error("target should be nil after level mismatch")
	}
}

// TestProcessInteractionDelayedPlayerSkipped verifies a delayed player skips interaction.
func TestProcessInteractionDelayedPlayerSkipped(t *testing.T) {
	s := newTestServer(t)
	s.grid = grid.New()
	npc := makeInteractionNpc(t, s, 1, 101, 100, 0)

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 100, 100, 0

	p.SetInteraction(InteractionEngine, npc, 1)
	p.delayed = true
	p.delayedUntil = 999 // far future
	s.currentTick = 0

	received := drainConn(t, cc)
	p.processInteraction()
	p.client.flushWrite()
	got := <-received

	if len(got) != 0 {
		t.Errorf("delayed player: expected no wire bytes, got %d", len(got))
	}
	if p.interacted {
		t.Error("interacted should be false for delayed player")
	}
}

func TestSetInteractionResetsInteractionFired(t *testing.T) {
	p := &Player{}
	p.interactionFired = true
	npc := &Npc{nid: 0, typeId: 7}
	p.SetInteraction(InteractionEngine, npc, 1)
	if p.interactionFired {
		t.Error("SetInteraction: interactionFired should be reset to false")
	}
}

func TestClearInteractionResetsInteractionFired(t *testing.T) {
	p := &Player{}
	p.interactionFired = true
	p.ClearInteraction()
	if p.interactionFired {
		t.Error("ClearInteraction: interactionFired should be reset to false")
	}
}

// TestInOperableDistanceTable checks adjacency logic for various offsets.
func TestInOperableDistanceTable(t *testing.T) {
	cases := []struct {
		dx, dz int
		want   bool
	}{
		{0, 0, false}, // same tile
		{1, 0, true},  // N/S/E/W adjacent
		{0, 1, true},
		{-1, 0, true},
		{0, -1, true},
		{1, 1, true},   // diagonal adjacent
		{-1, -1, true}, // diagonal adjacent
		{2, 0, false},  // 2 away
		{0, 2, false},
		{2, 1, false},
	}
	for _, tc := range cases {
		got := inOperableDistance(0, 0, tc.dx, tc.dz)
		if got != tc.want {
			t.Errorf("inOperableDistance(0,0,%d,%d) = %v, want %v", tc.dx, tc.dz, got, tc.want)
		}
	}
}

// TestSendUnsetMapFlagWireFormat verifies the encrypted opcode byte.
func TestSendUnsetMapFlagWireFormat(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc := io2.New([4]uint32{7, 8, 9, 10})
	refEnc := io2.New([4]uint32{7, 8, 9, 10})
	p.client.encryptor = enc

	want := byte((int(gameserver.OpUnsetMapFlag.Opcode) + int(refEnc.GetNext())) & 0xff)

	received := drainConn(t, cc)
	sendUnsetMapFlag(p)
	p.client.flushWrite()
	got := <-received

	if len(got) != 1 {
		t.Fatalf("UnsetMapFlag: got %d bytes, want 1", len(got))
	}
	if !bytes.Equal(got, []byte{want}) {
		t.Errorf("UnsetMapFlag wire: got %v, want %v", got, []byte{want})
	}
}
