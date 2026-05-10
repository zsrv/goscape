package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/script"
)

// TestResolveMovementGateOnPrimaryQueue pins TS Player.ts:657 first
// disjunct: when moveClickRequest && Busy() && len(queue) > 0, movement
// is suppressed (walkDir/runDir reset to -1; no waypoint advance).
func TestResolveMovementGateOnPrimaryQueue(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3200, 3200, 0
	p.lastTickX, p.lastTickZ, p.lastLevel = 3200, 3200, 0
	// Set up a single-step waypoint so resolveMovement WOULD step if
	// the gate didn't fire.
	p.waypoints[0] = coordgrid.PackCoord(0, 3201, 3200)
	p.waypointIndex = 0
	p.walkDir = 7 // poison: stale prior-tick value
	p.runDir = 7

	// Activate gate.
	p.moveClickRequest = true
	p.delayed = true // makes Busy() true
	p.queue = append(p.queue, playerQueueRequest{
		Script: &script.ScriptFile{Name: "[blocker]"},
		Type:   script.QueueNormal,
	})

	p.resolveMovement()

	if p.walkDir != -1 {
		t.Errorf("walkDir: got %d, want -1 (gate fires → walkDir cleared)", p.walkDir)
	}
	if p.runDir != -1 {
		t.Errorf("runDir: got %d, want -1 (gate fires → runDir cleared)", p.runDir)
	}
	if p.waypointIndex != 0 {
		t.Errorf("waypointIndex: got %d, want 0 (gate fires → no step taken)", p.waypointIndex)
	}
	if p.x != 3200 {
		t.Errorf("p.x: got %d, want 3200 (gate fires → no step taken)", p.x)
	}
}

// TestResolveMovementGateOnEngineQueue pins TS Player.ts:657 second
// disjunct: when moveClickRequest && Busy() && len(engineQueue) > 0,
// movement is suppressed even if the primary queue is empty.
func TestResolveMovementGateOnEngineQueue(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3200, 3200, 0
	p.lastTickX, p.lastTickZ, p.lastLevel = 3200, 3200, 0
	p.waypoints[0] = coordgrid.PackCoord(0, 3201, 3200)
	p.waypointIndex = 0
	p.walkDir = 7
	p.runDir = 7

	p.moveClickRequest = true
	p.delayed = true
	// Primary queue empty; engineQueue has work.
	p.engineQueue = append(p.engineQueue, playerQueueRequest{
		Script: &script.ScriptFile{Name: "[blocker_engine]"},
		Type:   script.QueueEngine,
	})

	p.resolveMovement()

	if p.walkDir != -1 {
		t.Errorf("walkDir: got %d, want -1 (gate fires on engineQueue → walkDir cleared)", p.walkDir)
	}
	if p.x != 3200 {
		t.Errorf("p.x: got %d, want 3200 (gate fires on engineQueue → no step taken)", p.x)
	}
}

// TestResolveMovementGateReleasesWhenQueuesEmpty pins that the gate
// only fires when at least one of (queue, engineQueue) is non-empty.
// With both empty AND moveClickRequest+Busy(), movement proceeds
// past the gate (verified by checking that lastTickX is overwritten
// from its poison value).
func TestResolveMovementGateReleasesWhenQueuesEmpty(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3200, 3200, 0
	p.waypoints[0] = coordgrid.PackCoord(0, 3201, 3200)
	p.waypointIndex = 0

	p.moveClickRequest = true
	p.delayed = true // Busy() = true
	// Both queues empty → gate releases.

	p.lastTickX = 9999 // poison; gets overwritten by resolveMovement past gate
	p.lastTickZ = 9999

	p.resolveMovement()

	if p.lastTickX != 3200 {
		t.Errorf("lastTickX: got %d, want 3200 (queues empty → gate releases → resolveMovement progressed past gate)", p.lastTickX)
	}
}

// TestProcessPostDecodeActivatesGate is the end-to-end pin closing
// NAI-144-D-MoveClickRequestSetter: with a busy player who issued a
// move-click (userPath set) and has queued script work, the
// post-decode block sets moveClickRequest=true and the gate at
// resolveMovement returns early.
func TestProcessPostDecodeActivatesGate(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.client.server = &Server{
		log: discardLogger(),
		cfg: Config{
			NodeWalktriggerSetting: WalkTriggerSettingPlayerpacket,
			NodeClientRoutefinder:  true,
		},
	}
	p.x, p.z, p.level = 3200, 3200, 0
	p.lastTickX, p.lastTickZ, p.lastLevel = 3200, 3200, 0

	// Stage gate-firing prerequisites.
	p.userPath = []int{coordgrid.PackCoord(p.level, p.x+1, p.z)}
	p.waypoints[0] = coordgrid.PackCoord(0, 3201, 3200)
	p.waypointIndex = 0
	// Use modalState (not delayed) to make Busy() true: p.delayed=true would
	// trigger the TS L614-617 early-return arm in processPostDecode (which
	// calls unsetMapFlag and bypasses the moveClickRequest setter), defeating
	// the test. modalState=Main satisfies Busy() via the modal disjunct
	// (interaction.go Busy()) without entering the delayed branch.
	p.modalState = modalStateMain
	p.opcalled = false
	p.queue = append(p.queue, playerQueueRequest{
		Script: &script.ScriptFile{Name: "[blocker]"},
		Type:   script.QueueNormal,
	})
	p.decodedThisTick = true
	p.walkDir = 7
	p.runDir = 7
	p.moveClickRequest = false // sentinel — will be flipped by processPostDecode

	// Drive the post-decode block.
	p.processPostDecode()

	if !p.moveClickRequest {
		t.Fatalf("moveClickRequest after processPostDecode: got false, want true (Busy + !opcalled + userPath set)")
	}

	// Drive movement; gate should fire and suppress.
	p.resolveMovement()

	if p.walkDir != -1 {
		t.Errorf("walkDir: got %d, want -1 (gate fires → walkDir cleared)", p.walkDir)
	}
	if p.runDir != -1 {
		t.Errorf("runDir: got %d, want -1 (gate fires → runDir cleared)", p.runDir)
	}
	if p.x != 3200 {
		t.Errorf("p.x: got %d, want 3200 (gate fires → no step taken)", p.x)
	}
}
