package world

import (
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/objtype"
)

// TestProcessInteractionsRunsPerPlayer verifies that processInteractions drives
// every active player's interaction state machine in a single call.
func TestProcessInteractionsRunsPerPlayer(t *testing.T) {
	s := newTestServer(t)

	typ := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 0, DebugName: "test"},
		Op:          []string{"Attack"},
		WanderRange: 0,
		RespawnRate: 50,
	}

	// Player 1: adjacent to npc1 at slot 1
	npc1 := NewNpc(1, 0, 101, 100, 0, typ)
	npc1.nid = 1
	s.npcs[1] = npc1
	s.npcLoop = append(s.npcLoop, npc1)

	p1, cc1 := newTestPlayer(t)
	p1.client.server = s
	p1.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p1.x, p1.z, p1.level = 100, 100, 0
	p1.SetInteraction(InteractionEngine, npc1, 1, -1)
	p1.pid = 1
	s.playersMu.Lock()
	s.players.set(1, p1)
	s.playersMu.Unlock()

	// Player 2: adjacent to npc2 at slot 2
	npc2 := NewNpc(2, 0, 200, 200, 0, typ)
	npc2.nid = 2
	s.npcs[2] = npc2
	s.npcLoop = append(s.npcLoop, npc2)

	p2, cc2 := newTestPlayer(t)
	p2.client.server = s
	p2.client.encryptor = io2.New([4]uint32{5, 6, 7, 8})
	p2.x, p2.z, p2.level = 199, 200, 0
	p2.SetInteraction(InteractionEngine, npc2, 1, -1)
	p2.pid = 2
	s.playersMu.Lock()
	s.players.set(2, p2)
	s.playersMu.Unlock()

	// Drain both conns before calling processInteractions (avoids net.Pipe deadlock).
	drain1 := drainConn(t, cc1)
	drain2 := drainConn(t, cc2)

	// Interaction is split around movement (TS-faithful): the pre-step
	// interact runs in the pre-move pass, post-step + auto-clear in the
	// post-move pass. Both players are already adjacent, so the pre-step OP
	// fires; no movement pass is needed between for this contact-range case.
	s.processInteractionsPreMove()
	s.processInteractions()

	p1.client.flushWrite()
	p2.client.flushWrite()

	<-drain1
	<-drain2

	// NAI-44 T6 cascade: interacted is cleared by the post-fire auto-clear
	// (TS L1261-1263: interacted && !apRangeCalled → ClearInteraction).
	// target==nil proves each player's interaction fired and auto-cleared.
	if p1.target != nil {
		t.Errorf("p1.target: got %v, want nil (auto-clear after contact-fire with npc1)", p1.target)
	}
	if p2.target != nil {
		t.Errorf("p2.target: got %v, want nil (auto-clear after contact-fire with npc2)", p2.target)
	}
	if p1.faceEntity != npc1.nid {
		t.Errorf("p1.faceEntity: got %d, want %d", p1.faceEntity, npc1.nid)
	}
	if p2.faceEntity != npc2.nid {
		t.Errorf("p2.faceEntity: got %d, want %d", p2.faceEntity, npc2.nid)
	}
}

// TestProcessInteractionsNoTargetNoOp verifies that players without a target are untouched.
func TestProcessInteractionsNoTargetNoOp(t *testing.T) {
	s := newTestServer(t)

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.x, p.z, p.level = 100, 100, 0
	// No target set.
	p.pid = 1
	s.playersMu.Lock()
	s.players.set(1, p)
	s.playersMu.Unlock()

	drain := drainConn(t, cc)
	s.processInteractions()
	p.client.flushWrite()
	got := <-drain

	if len(got) != 0 {
		t.Errorf("expected no wire bytes for player with no target, got %d", len(got))
	}
	if p.interacted {
		t.Error("interacted should remain false")
	}
}
