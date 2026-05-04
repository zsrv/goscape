package world

import "testing"

// NAI-82: TS Npc.updateMovement at Engine-TS/.../Npc.ts:362-366 writes
// lastMovement = World.currentTick + 1 when the NPC's position changed
// this tick. Read by AI_ARRIVEDELAY / AI_TARGETMOVED (deferred — see
// NAI-82 spec §6.1).
func TestNpcUpdateMovementWritesLastMovementOnStep(t *testing.T) {
	s := newTestServer(t)
	s.currentTick = 50

	n := newTestNpc(1)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.lastTickX, n.lastTickZ = n.x, n.z // mirror processMovementInteraction:162 snapshot
	n.QueueWaypoint(3094, 3107)

	moved := n.updateMovement(s)

	if !moved {
		t.Fatalf("updateMovement: got false, want true (one step queued)")
	}
	if n.x != 3094 || n.z != 3107 {
		t.Fatalf("position: got (%d,%d), want (3094,3107)", n.x, n.z)
	}
	if n.lastMovement != 51 {
		t.Errorf("lastMovement: got %d, want 51 (currentTick + 1)", n.lastMovement)
	}
}

// NAI-82: stationary tick (no waypoint) leaves lastMovement untouched.
func TestNpcUpdateMovementSkipsLastMovementWhenStationary(t *testing.T) {
	s := newTestServer(t)
	s.currentTick = 50

	n := newTestNpc(1)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.lastTickX, n.lastTickZ = n.x, n.z
	n.waypointIndex = -1

	moved := n.updateMovement(s)

	if moved {
		t.Errorf("updateMovement: got true, want false (no waypoint = no step)")
	}
	if n.lastMovement != 0 {
		t.Errorf("lastMovement: got %d, want 0 (unchanged from zero-value)", n.lastMovement)
	}
}
