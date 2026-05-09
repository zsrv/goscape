package world

import "testing"

func TestDefaultMoveSpeed_RunZero(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.run = 0
	if got := p.defaultMoveSpeed(); got != MoveSpeedWalk {
		t.Errorf("defaultMoveSpeed(run=0): got %v, want MoveSpeedWalk", got)
	}
}

func TestDefaultMoveSpeed_RunOne(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.run = 1
	if got := p.defaultMoveSpeed(); got != MoveSpeedRun {
		t.Errorf("defaultMoveSpeed(run=1): got %v, want MoveSpeedRun", got)
	}
}

func TestResolveMovement_BridgeRunEnabled(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.run = 1
	p.runanim = 0
	p.tempRun = 0
	p.moveSpeed = MoveSpeedWalk
	p.waypointIndex = -1 // no movement; just verify bridge writes moveSpeed

	p.resolveMovement()

	if p.moveSpeed != MoveSpeedRun {
		t.Errorf("moveSpeed: got %v, want MoveSpeedRun (run=1, runanim>=0, tempRun=0)", p.moveSpeed)
	}
}

func TestResolveMovement_BridgeTempRunOverride(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.run = 0
	p.runanim = 0
	p.tempRun = 1
	p.moveSpeed = MoveSpeedWalk
	p.waypointIndex = -1

	p.resolveMovement()

	if p.moveSpeed != MoveSpeedRun {
		t.Errorf("moveSpeed: got %v, want MoveSpeedRun (tempRun=1 override)", p.moveSpeed)
	}
}

func TestResolveMovement_BridgeRunanimMinusOneForcesWalk(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.run = 1
	p.runanim = -1
	p.tempRun = 1
	p.moveSpeed = MoveSpeedWalk
	p.waypointIndex = -1

	p.resolveMovement()

	if p.moveSpeed != MoveSpeedWalk {
		t.Errorf("moveSpeed: got %v, want MoveSpeedWalk (runanim=-1 forces Walk per TS Player.ts:663-664)", p.moveSpeed)
	}
}

func TestResolveMovement_BridgeRunZero(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.run = 0
	p.runanim = 0
	p.tempRun = 0
	p.moveSpeed = MoveSpeedWalk
	p.waypointIndex = -1

	p.resolveMovement()

	if p.moveSpeed != MoveSpeedWalk {
		t.Errorf("moveSpeed: got %v, want MoveSpeedWalk (run=0, tempRun=0)", p.moveSpeed)
	}
}

func TestResolveMovement_BridgeInstantPreserved(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.run = 1
	p.runanim = 0
	p.tempRun = 1
	p.moveSpeed = MoveSpeedInstant
	p.waypointIndex = -1

	p.resolveMovement()

	if p.moveSpeed != MoveSpeedInstant {
		t.Errorf("moveSpeed: got %v, want MoveSpeedInstant (bridge skips when moveSpeed==Instant)", p.moveSpeed)
	}
}

func TestResolveMovement_TempRunResetOnIdle(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.tempRun = 1
	p.waypointIndex = -1 // no waypoints → no steps → idle

	p.resolveMovement()

	if p.tempRun != 0 {
		t.Errorf("tempRun: got %d, want 0 (reset on idle per TS Player.ts:670-673)", p.tempRun)
	}
}

func TestResolveMovement_TempRunPreservedDuringSteps(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0
	p.run = 1
	p.runanim = 0
	p.tempRun = 1
	p.waypoints[0] = packTestCoord(0, 3094, 3108)
	p.waypointIndex = 0

	p.resolveMovement()

	if p.stepsTaken == 0 {
		t.Fatalf("stepsTaken=0; expected at least one step")
	}
	if p.tempRun != 1 {
		t.Errorf("tempRun: got %d, want 1 (preserved when steps taken)", p.tempRun)
	}
}
