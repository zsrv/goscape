package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/gamemap"
	gameclient "github.com/zsrv/goscape/pkg/io/protocol/game/client"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
)

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
	p.run = 1
	p.runanim = 0
	p.moveSpeed = MoveSpeedWalk // bridge input; bridge will elevate to Run
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

// TestPathToMoveClick_FlyQueuesEndCoord pins TS PathingEntity.pathToMoveClick
// L408-420: any moveStrategy other than SMART (including FLY) takes the
// "queueWaypoint(last-coord)" branch. ::fly was the first runtime path
// that put MoveStrategyFly on a live Player (NAI-184 T3); the goscape
// switch in movement.go only had explicit Smart/Naive cases, dropping
// the player's clicks silently. NAI-184 T3 fix.
func TestPathToMoveClick_FlyQueuesEndCoord(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0
	p.moveStrategy = MoveStrategyFly

	packed := []int{packTestCoord(0, 3100, 3110), packTestCoord(0, 3105, 3115)}
	p.pathToMoveClick(packed, false)

	gotX := (p.waypoints[0] >> 14) & 0x3FFF
	gotZ := p.waypoints[0] & 0x3FFF
	if gotX != 3105 || gotZ != 3115 {
		t.Errorf("FLY should take input[-1]: got (%d,%d), want (3105,3115)", gotX, gotZ)
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

	// MOVE_GAMECLICK: opcode 63 (244), 1-byte length prefix.
	// Payload: ctrlHeld(1) + startX G2(2) + startZ G2(2) = 5 bytes
	// Move to (3094, 3107) — one tile north.
	payload := []byte{
		0,          // ctrlHeld
		0x0C, 0x16, // startX = 3094
		0x0C, 0x23, // startZ = 3107
	}
	buf := []byte{encryptOpcode(enc, gameclient.OpcMoveGameClick), byte(len(payload))}
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
	dir, status := p.stepOnce()
	if status != stepMoved {
		t.Fatalf("stepOnce status: got %v (dir=%d), want stepMoved", status, dir)
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
	if _, status := p.stepOnce(); status != stepMoved {
		t.Fatalf("stepOnce status: got %v, want stepMoved", status)
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

// TestQueueWaypointsReversesInputOrder pins TS PathingEntity.queueWaypoints
// (Engine-TS/src/engine/entity/PathingEntity.ts:248-254): packed arrives in
// src→dst order ([first_step, …, dest]); queueWaypoints reverses on copy so
// internal storage is [dest, …, first_step]. stepOnce's read of
// waypoints[waypointIndex=n-1] then returns first_step.
func TestQueueWaypointsReversesInputOrder(t *testing.T) {
	p, _ := newTestPlayer(t)

	a := packTestCoord(0, 3100, 3100) // first_step
	b := packTestCoord(0, 3105, 3105) // mid
	c := packTestCoord(0, 3110, 3110) // dest
	packed := []int{a, b, c}

	p.queueWaypoints(packed)

	if p.waypointIndex != 2 {
		t.Errorf("waypointIndex: got %d, want 2 (n-1)", p.waypointIndex)
	}
	if p.waypoints[0] != c {
		t.Errorf("waypoints[0]: got 0x%X, want 0x%X (= packed[2] = dest)", p.waypoints[0], c)
	}
	if p.waypoints[1] != b {
		t.Errorf("waypoints[1]: got 0x%X, want 0x%X (= packed[1] = mid)", p.waypoints[1], b)
	}
	if p.waypoints[2] != a {
		t.Errorf("waypoints[2]: got 0x%X, want 0x%X (= packed[0] = first_step)", p.waypoints[2], a)
	}
}

// TestQueueWaypointsTruncatesFarEntries pins TS PathingEntity.queueWaypoints
// truncation behavior (PathingEntity.ts:248-254 inner condition output <
// this.waypoints.length): when packed exceeds the waypoints buffer length,
// the entries closest to dest are preserved and far-from-dest entries are
// dropped. This matches TS because TS iterates input from length-1 down to
// 0 while output is bounded above by waypoints.length.
//
// Goscape's Player.waypoints is a fixed-size [25]int. With 30-element
// packed input, the 5 entries at packed[0..4] (closest to source) are
// dropped; packed[5..29] reversed are stored at waypoints[0..24].
func TestQueueWaypointsTruncatesFarEntries(t *testing.T) {
	p, _ := newTestPlayer(t)

	const inLen = 30
	if inLen <= len(p.waypoints) {
		t.Fatalf("test fixture broken: inLen=%d must exceed len(p.waypoints)=%d", inLen, len(p.waypoints))
	}
	packed := make([]int, inLen)
	for i := range packed {
		packed[i] = packTestCoord(0, 3000+i, 3000)
	}

	p.queueWaypoints(packed)

	bufLen := len(p.waypoints)
	if p.waypointIndex != bufLen-1 {
		t.Errorf("waypointIndex: got %d, want %d (buffer cap)", p.waypointIndex, bufLen-1)
	}
	// Storage[i] = packed[inLen-1-i] for i in [0, bufLen). The last
	// bufLen entries of packed (the dest-end) are preserved; packed[0..4]
	// (source-end) are dropped.
	for i := range bufLen {
		want := packed[inLen-1-i]
		if p.waypoints[i] != want {
			t.Errorf("waypoints[%d]: got 0x%X, want 0x%X (= packed[%d])", i, p.waypoints[i], want, inLen-1-i)
		}
	}
}

// TestStepOnceFollowsDirectionChangePoints is the regression pin for the
// NAI-101 root cause. Pre-fix, with packed=[first_step, mid, dest] stored
// natural-order, stepOnce reads waypoints[n-1] = dest and uses Face to head
// straight at dest, ignoring the routed mid waypoint. Post-fix, reversed
// storage means waypoints[n-1] = first_step; stepOnce iterates through
// each direction-change point in turn.
//
// Scenario: player at (3094, 3106). Route N to (3094, 3110), then E to
// (3097, 3110). Pre-fix Face from (3094, 3106) to dest (3097, 3110) returns
// DirectionNortheast (heads NE diagonally), bypassing the routed N→E shape.
// Post-fix Face from (3094, 3106) to first_step (3094, 3107) returns
// DirectionNorth (correct first step).
func TestStepOnceFollowsDirectionChangePoints(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0
	p.moveSpeed = MoveSpeedWalk

	firstStep := packTestCoord(0, 3094, 3107)
	mid := packTestCoord(0, 3094, 3110)
	dest := packTestCoord(0, 3097, 3110)
	p.queueWaypoints([]int{firstStep, mid, dest})

	// Tick 1: should step N (toward first_step), not NE (toward dest).
	p.resolveMovement()
	if p.x != 3094 || p.z != 3107 {
		t.Fatalf("tick 1: got (%d,%d), want (3094,3107) [N step toward first_step]; "+
			"pre-fix bug heads NE toward dest", p.x, p.z)
	}
}

// TestPlayerStepOnce_PlumbsBlockWalkFlag pins NAI-176 D4. TS Player.blockWalkFlag
// (Player.ts:706-708) is unconditional FlagBlockPlayers. Goscape pre-NAI-176
// passed extraFlag=0 to gamemap.CanTravel (movement.go:144), so a tile carrying
// only FlagBlockPlayers (e.g., one occupied by another player or a BlockWalkAll
// NPC) was traversable by the moving player. Post-fix: the same tile should
// block the step (status = stepBlocked).
func TestPlayerStepOnce_PlumbsBlockWalkFlag(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	// Allocate start + dest tiles so FlagMap defaults to FlagOpen (otherwise
	// FlagNull degenerate-blocks). Plant FlagBlockPlayers on the dest.
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3200, 3200, 0)
	s.gamemap.Pathfinder.Flags.Add(3201, 3200, 0, collision.FlagBlockPlayers)
	p.queueWaypoint(3201, 3200)

	wantWaypointIndex := p.waypointIndex
	dir, status := p.stepOnce()

	if status != stepBlocked {
		t.Fatalf("player step over FlagBlockPlayers tile: got status=%v dir=%d, want stepBlocked", status, dir)
	}
	if p.waypointIndex != wantWaypointIndex {
		t.Fatalf("waypointIndex after stepBlocked: got %d, want %d (D2: must NOT clear)",
			p.waypointIndex, wantWaypointIndex)
	}
	if p.x != 3200 || p.z != 3200 {
		t.Fatalf("position after blocked step: got (%d,%d), want (3200,3200) unchanged", p.x, p.z)
	}
}

// TestPlayerStepOnce_AxisFallback_XOnly pins NAI-176 D4 + D1 for Player.
// Direct diagonal blocked, X-only open → step east.
func TestPlayerStepOnce_AxisFallback_XOnly(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	// Allocate the 2x2 bounding box so unallocated FlagNull doesn't block.
	for x := 3200; x <= 3201; x++ {
		for z := 3200; z <= 3201; z++ {
			s.gamemap.Pathfinder.Flags.AllocateIfAbsent(x, z, 0)
		}
	}
	// Block NE-diagonal (3201, 3201); leave east (3201, 3200) open.
	s.gamemap.Pathfinder.Flags.Add(3201, 3201, 0, collision.FlagBlockWalk)
	p.queueWaypoint(3205, 3205)

	dir, status := p.stepOnce()

	if status != stepMoved {
		t.Fatalf("axis-fallback X: got status=%v, want stepMoved", status)
	}
	if dir != int(coordgrid.DirectionEast) {
		t.Fatalf("axis-fallback X: dir=%d, want East (%d)", dir, coordgrid.DirectionEast)
	}
	if p.x != 3201 || p.z != 3200 {
		t.Fatalf("axis-fallback X: stepped to (%d,%d), want (3201,3200)", p.x, p.z)
	}
}

// TestPlayerValidateAndAdvanceStep_NoMoveRestrict_ReturnsBlocked pins
// the wrapper's response to MoveRestrictNoMove: stepDone via cs==nil,
// wrapper decrements then sees waypointIndex<0 and returns (-1, false).
// waypointIndex transitions 0 → -1 (legitimate decrement, not a clear).
func TestPlayerValidateAndAdvanceStep_NoMoveRestrict_ReturnsBlocked(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	p.moveRestrict = MoveRestrictNoMove
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	p.queueWaypoint(3201, 3200)

	dir, advanced := p.validateAndAdvanceStep()

	if advanced {
		t.Fatalf("NoMove: got advanced=true (dir=%d), want false", dir)
	}
	if p.x != 3200 || p.z != 3200 {
		t.Fatalf("NoMove: position changed to (%d,%d), want (3200,3200)", p.x, p.z)
	}
}

// TestValidateDistanceWalked pins M3: a player whose net displacement from its
// start-of-tick position (lastTickX/Z) exceeds 2 tiles is flagged jump=true so
// the client renders a teleport rather than an impossible slide. Mirrors TS
// PathingEntity.validateDistanceWalked (PathingEntity.ts:303-315).
func TestValidateDistanceWalked(t *testing.T) {
	t.Run("over_two_tiles_jumps", func(t *testing.T) {
		p, _ := newTestPlayer(t)
		p.x, p.z, p.level = 3094, 3106, 0
		p.lastTickX, p.lastTickZ, p.lastLevel = 3094, 3100, 0 // moved 6 tiles
		p.jump = false
		p.validateDistanceWalked()
		if !p.jump {
			t.Error("net 6-tile move: jump=false, want true")
		}
	})

	t.Run("two_tiles_no_jump", func(t *testing.T) {
		p, _ := newTestPlayer(t)
		p.x, p.z, p.level = 3094, 3106, 0
		p.lastTickX, p.lastTickZ, p.lastLevel = 3094, 3104, 0 // moved 2 tiles (run)
		p.jump = false
		p.validateDistanceWalked()
		if p.jump {
			t.Error("net 2-tile move: jump=true, want false (a run covers 2 tiles)")
		}
	})
}

// TestProcessValidateDistanceWalked_ExactMoveGate pins that the per-tick pass
// skips the jump-snap when an EXACT_MOVE mask is already driving the
// displacement (TS World.ts:733 `(player.masks & EXACT_MOVE) == 0` guard).
func TestProcessValidateDistanceWalked_ExactMoveGate(t *testing.T) {
	s := newServerForScriptTest(t)

	moved := newTestPlayerAt(t, s, 1, 3094, 3106, 0)
	moved.lastTickX, moved.lastTickZ, moved.lastLevel = 3094, 3100, 0 // 6 tiles

	exact := newTestPlayerAt(t, s, 2, 3200, 3200, 0)
	exact.lastTickX, exact.lastTickZ, exact.lastLevel = 3200, 3194, 0 // 6 tiles
	exact.masks |= MaskExactMove

	s.players.set(moved.pid, moved)
	s.players.set(exact.pid, exact)
	s.processValidateDistanceWalked()

	if !moved.jump {
		t.Error("non-exact-move player moved 6 tiles: jump=false, want true")
	}
	if exact.jump {
		t.Error("EXACT_MOVE player: jump=true, want false (gate must skip it)")
	}
}
