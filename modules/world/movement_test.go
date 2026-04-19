package world

import "testing"

func TestQueueWaypointSetsFirstEntry(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0

	p.queueWaypoint(3100, 3110)

	if p.waypointIndex != 0 {
		t.Errorf("waypointIndex: got %d, want 0", p.waypointIndex)
	}
	if p.waypoints[0] == 0 {
		t.Error("waypoints[0] should be set")
	}
}

func TestQueueWaypointsReplacesExisting(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0

	first := []int{packTestCoord(0, 3100, 3110)}
	second := []int{packTestCoord(0, 3200, 3210), packTestCoord(0, 3205, 3215)}

	p.queueWaypoints(first)
	if p.waypointIndex != 0 {
		t.Errorf("after first queueWaypoints, waypointIndex = %d, want 0", p.waypointIndex)
	}

	p.queueWaypoints(second)
	if p.waypointIndex != 1 {
		t.Errorf("after second queueWaypoints (2 entries), waypointIndex = %d, want 1", p.waypointIndex)
	}
}

func packTestCoord(level, x, z int) int {
	return (z & 0x3fff) | ((x & 0x3fff) << 14) | ((level & 0x3) << 28)
}

func TestResolveMovementAdvancesOneTileWalking(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0
	p.moveSpeed = MoveSpeedWalk
	p.queueWaypoint(3094, 3107)

	p.resolveMovement()

	if p.z != 3107 {
		t.Errorf("after walk step z: got %d, want 3107", p.z)
	}
	if p.walkDir == -1 {
		t.Error("walkDir should be set after a step")
	}
	if p.runDir != -1 {
		t.Errorf("runDir: got %d, want -1 (walking)", p.runDir)
	}
	if p.lastTickX != 3094 || p.lastTickZ != 3106 {
		t.Errorf("lastTick: got (%d,%d), want (3094,3106)", p.lastTickX, p.lastTickZ)
	}
}

func TestResolveMovementAdvancesTwoTilesRunning(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0
	p.moveSpeed = MoveSpeedRun
	p.runenergy = 10000
	p.waypoints[0] = packTestCoord(0, 3094, 3108)
	p.waypointIndex = 0

	p.resolveMovement()

	if p.z != 3108 {
		t.Errorf("after run z: got %d, want 3108 (two steps)", p.z)
	}
	if p.walkDir == -1 {
		t.Error("walkDir should be set")
	}
	if p.runDir == -1 {
		t.Error("runDir should be set when running")
	}
}

func TestResolveMovementNoPathClearsDirections(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.waypointIndex = -1
	p.walkDir = 5
	p.runDir = 3

	p.resolveMovement()

	if p.walkDir != -1 {
		t.Errorf("walkDir with no path: got %d, want -1", p.walkDir)
	}
	if p.runDir != -1 {
		t.Errorf("runDir with no path: got %d, want -1", p.runDir)
	}
}

func TestPathToMoveClickSmartTrustClient(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0
	p.moveStrategy = MoveStrategySmart

	packed := []int{packTestCoord(0, 3100, 3110)}
	p.pathToMoveClick(packed, false)

	if p.waypointIndex != 0 {
		t.Errorf("waypointIndex: got %d, want 0", p.waypointIndex)
	}
	if p.waypoints[0] != packed[0] {
		t.Error("waypoints[0] should equal input")
	}
}

func TestPathToMoveClickNaiveTakesLastCoord(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0
	p.moveStrategy = MoveStrategyNaive

	packed := []int{packTestCoord(0, 3100, 3110), packTestCoord(0, 3105, 3115)}
	p.pathToMoveClick(packed, false)

	gotX := (p.waypoints[0] >> 14) & 0x3FFF
	gotZ := p.waypoints[0] & 0x3FFF
	if gotX != 3105 || gotZ != 3115 {
		t.Errorf("NAIVE should take input[-1]: got (%d,%d), want (3105,3115)", gotX, gotZ)
	}
}

func TestResolveMovementDrainsRunEnergy(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0
	p.moveSpeed = MoveSpeedRun
	p.runenergy = 10000
	p.runweight = 0
	p.waypoints[0] = packTestCoord(0, 3094, 3108)
	p.waypointIndex = 0

	p.resolveMovement()

	if p.runenergy >= 10000 {
		t.Errorf("run energy should have drained, got %d", p.runenergy)
	}
}
