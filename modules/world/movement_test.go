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

func TestMoveGameClickAdvancesPlayer(t *testing.T) {
	enc, dec := isaacPair([4]uint32{1, 2, 3, 4})
	p, _ := newTestPlayer(t)
	p.client.decryptor = dec
	p.client.encryptor = enc
	s := newTestServer(t)
	p.client.server = s
	p.x, p.z, p.level = 3094, 3106, 0
	p.moveSpeed = MoveSpeedWalk

	// MOVE_GAMECLICK: opcode 181, 1-byte length prefix.
	// Payload: ctrlHeld(1) + startX G2(2) + startZ G2(2) = 5 bytes
	// Move to (3094, 3107) — one tile north.
	payload := []byte{
		0,          // ctrlHeld
		0x0C, 0x16, // startX = 3094
		0x0C, 0x23, // startZ = 3107
	}
	buf := []byte{encryptOpcode(enc, 181), byte(len(payload))}
	buf = append(buf, payload...)
	p.client.in.Write(buf)

	p.processIn(0)

	if p.waypointIndex < 0 {
		t.Fatal("pathToMoveClick should have queued a waypoint")
	}

	p.resolveMovement()

	if p.z != 3107 {
		t.Errorf("after tick, z: got %d, want 3107", p.z)
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

func TestPlayerStepCrossZoneRefreshSubscription(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	// Start in zone (399, 400) at (3199, 3200).
	p.x, p.z, p.level = 3199, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	// Queue a step east into zone (400, 400).
	p.queueWaypoint(3200, 3200)
	dir, ok := p.stepOnce()
	if !ok {
		t.Fatalf("stepOnce ok: got false (dir=%d)", dir)
	}
	prevZ := s.zoneMap.Get(0, 3199, 3200)
	newZ := s.zoneMap.Get(0, 3200, 3200)
	if prevZ.PlayersCount() != 0 {
		t.Errorf("prev zone PlayersCount: got %d, want 0", prevZ.PlayersCount())
	}
	if newZ.PlayersCount() != 1 {
		t.Errorf("new zone PlayersCount: got %d, want 1", newZ.PlayersCount())
	}
}

// TestResolveMovementResetsStepsTaken — NAI-44 T3.
// stepsTaken must be reset at the start of each tick's movement cycle so
// processInteraction (which runs after processPathing) reads the per-tick
// step count, not a cumulative total. Goscape's stepsTaken increment at
// movement.go:88 has no consumer pre-NAI-44; T5's processInteraction port
// is the first reader.
func TestResolveMovementResetsStepsTaken(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.stepsTaken = 5 // simulate cumulative count from prior tick

	p.resolveMovement()

	// resolveMovement returns early on waypointIndex < 0 (no path), so
	// stepsTaken should be reset to 0 and stay there (no steps taken).
	if p.stepsTaken != 0 {
		t.Errorf("stepsTaken: got %d, want 0 (reset at top of resolveMovement)", p.stepsTaken)
	}
}

func TestPlayerStepIntraZoneNoSubscriptionChange(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	// Start at (3200, 3200) zone (400, 400). Step to (3201, 3201) — same zone.
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	p.queueWaypoint(3201, 3201)
	if _, ok := p.stepOnce(); !ok {
		t.Fatal("stepOnce ok: got false")
	}
	z := s.zoneMap.Get(0, 3200, 3200)
	if z.PlayersCount() != 1 {
		t.Errorf("intra-zone step PlayersCount: got %d, want 1", z.PlayersCount())
	}
}

// NAI-82: TS Player.processMovement at Engine-TS/.../Player.ts:675-677
// writes lastMovement = World.currentTick + 1 whenever stepsTaken > 0
// after the tick's movement resolves. Read by P_ARRIVEDELAY's gate.
func TestResolveMovementWritesLastMovementOnStep(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	s.currentTick = 50

	p.x, p.z, p.level = 3094, 3106, 0
	p.moveSpeed = MoveSpeedWalk
	p.queueWaypoint(3094, 3107)

	p.resolveMovement()

	if p.stepsTaken != 1 {
		t.Fatalf("stepsTaken: got %d, want 1 (sanity — pre-existing invariant)", p.stepsTaken)
	}
	if p.lastMovement != 51 {
		t.Errorf("lastMovement: got %d, want 51 (currentTick + 1)", p.lastMovement)
	}
}

// NAI-82: idle ticks (no waypoint) leave lastMovement untouched.
func TestResolveMovementSkipsLastMovementWhenIdle(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	s.currentTick = 50

	p.waypointIndex = -1

	p.resolveMovement()

	if p.stepsTaken != 0 {
		t.Errorf("stepsTaken: got %d, want 0 (no waypoint = no step)", p.stepsTaken)
	}
	if p.lastMovement != 0 {
		t.Errorf("lastMovement: got %d, want 0 (unchanged from zero-value)", p.lastMovement)
	}
}
