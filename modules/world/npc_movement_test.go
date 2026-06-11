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

// TestNpcQueueWaypointsReversesInputOrder is the Npc-side analogue of
// TestQueueWaypointsReversesInputOrder. Per Engine-TS PathingEntity.ts:248-254,
// the reverse-copy semantic is shared between Player and Npc (both inherit
// queueWaypoints from PathingEntity in TS).
func TestNpcQueueWaypointsReversesInputOrder(t *testing.T) {
	n := newTestNpc(1)

	a := packTestCoord(0, 3100, 3100) // first_step
	b := packTestCoord(0, 3105, 3105) // mid
	c := packTestCoord(0, 3110, 3110) // dest
	n.queueWaypoints([]int{a, b, c})

	if n.waypointIndex != 2 {
		t.Errorf("waypointIndex: got %d, want 2 (n-1)", n.waypointIndex)
	}
	if n.waypoints[0] != c {
		t.Errorf("waypoints[0]: got 0x%X, want 0x%X (= packed[2] = dest)", n.waypoints[0], c)
	}
	if n.waypoints[1] != b {
		t.Errorf("waypoints[1]: got 0x%X, want 0x%X (= packed[1] = mid)", n.waypoints[1], b)
	}
	if n.waypoints[2] != a {
		t.Errorf("waypoints[2]: got 0x%X, want 0x%X (= packed[0] = first_step)", n.waypoints[2], a)
	}
}

// TestNpcStepFollowsDirectionChangePoints is the Npc-side regression
// pin for NAI-101. Mirror of TestStepFollowsDirectionChangePoints.
//
// NPC at (3094, 3106). Route N to (3094, 3110), then E to (3097, 3110).
// Pre-fix Face from (3094, 3106) to dest (3097, 3110) returns NE diagonal,
// skipping the mid waypoint. Post-fix iterates first_step → mid → dest.
func TestNpcStepFollowsDirectionChangePoints(t *testing.T) {
	s := newTestServer(t)

	n := newTestNpc(1)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.lastTickX, n.lastTickZ = n.x, n.z

	firstStep := packTestCoord(0, 3094, 3107)
	mid := packTestCoord(0, 3094, 3110)
	dest := packTestCoord(0, 3097, 3110)
	n.queueWaypoints([]int{firstStep, mid, dest})

	// One updateMovement tick should step N (toward first_step), not NE.
	moved := n.updateMovement(s)
	if !moved {
		t.Fatalf("updateMovement: got false, want true (one step queued)")
	}
	if n.x != 3094 || n.z != 3107 {
		t.Fatalf("tick 1: got (%d,%d), want (3094,3107) [N step toward first_step]; "+
			"pre-fix bug heads NE toward dest", n.x, n.z)
	}
}
