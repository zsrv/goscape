package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

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

// updateEnergy: recover branch (stepsTaken<2) — agility=0, +8 baseline
func TestUpdateEnergy_RecoverBranchAgilityZero(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.varps = make([]int32, 1)
	p.stepsTaken = 0
	p.baseLevels[objtype.PlayerStatAgility] = 0
	p.runenergy = 5000

	p.updateEnergy()

	if p.runenergy != 5008 {
		t.Errorf("runenergy: got %d, want 5008 (5000 + 0/9 + 8)", p.runenergy)
	}
}

// updateEnergy: recover branch — agility=99, +19/tick (99/9=11; +8)
func TestUpdateEnergy_RecoverBranchAgilityNinetyNine(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.varps = make([]int32, 1)
	p.stepsTaken = 0
	p.baseLevels[objtype.PlayerStatAgility] = 99
	p.runenergy = 0

	p.updateEnergy()

	if p.runenergy != 19 {
		t.Errorf("runenergy: got %d, want 19 (99/9=11, +8)", p.runenergy)
	}
}

// updateEnergy: recover clamps at 10000
func TestUpdateEnergy_RecoverClampAt10000(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.varps = make([]int32, 1)
	p.stepsTaken = 0
	p.baseLevels[objtype.PlayerStatAgility] = 50
	p.runenergy = 9995

	p.updateEnergy()

	if p.runenergy != 10000 {
		t.Errorf("runenergy: got %d, want 10000 (clamped)", p.runenergy)
	}
}

// updateEnergy: drain branch — weight=0 → -67
func TestUpdateEnergy_DrainWeightZero(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.varps = make([]int32, 1)
	p.stepsTaken = 2
	p.runweight = 0
	p.runenergy = 10000

	p.updateEnergy()

	if p.runenergy != 9933 {
		t.Errorf("runenergy: got %d, want 9933 (10000 - 67)", p.runenergy)
	}
}

// updateEnergy: drain branch — weight=64kg (64000g) → -134
func TestUpdateEnergy_DrainWeight64kg(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.varps = make([]int32, 1)
	p.stepsTaken = 2
	p.runweight = 64000
	p.runenergy = 10000

	p.updateEnergy()

	if p.runenergy != 9866 {
		t.Errorf("runenergy: got %d, want 9866 (10000 - 134)", p.runenergy)
	}
}

// updateEnergy: drain clamps at 0
func TestUpdateEnergy_DrainClampAtZero(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.varps = make([]int32, 1)
	p.stepsTaken = 2
	p.runweight = 0
	p.runenergy = 10

	p.updateEnergy()

	if p.runenergy != 0 {
		t.Errorf("runenergy: got %d, want 0 (clamped)", p.runenergy)
	}
}

// updateEnergy: weight clamp negative → 0kg → -67
func TestUpdateEnergy_DrainWeightNegativeClampedToZero(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.varps = make([]int32, 1)
	p.stepsTaken = 2
	p.runweight = -1000 // -1kg
	p.runenergy = 10000

	p.updateEnergy()

	if p.runenergy != 9933 {
		t.Errorf("runenergy: got %d, want 9933 (negative weight clamped to 0kg → -67)", p.runenergy)
	}
}

// updateEnergy: weight clamp >64kg → 64kg → -134
func TestUpdateEnergy_DrainWeightOverflowClampedTo64kg(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.varps = make([]int32, 1)
	p.stepsTaken = 2
	p.runweight = 200000 // 200kg
	p.runenergy = 10000

	p.updateEnergy()

	if p.runenergy != 9866 {
		t.Errorf("runenergy: got %d, want 9866 (200kg clamped to 64kg → -134)", p.runenergy)
	}
}

// updateEnergy: runenergy=0 → run=0 + varp sync
func TestUpdateEnergy_EnergyZeroResetsRunAndVarp(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.varps = make([]int32, 1)
	p.varps[0] = 1 // pre-state: run is on
	p.run = 1
	p.stepsTaken = 2
	p.runweight = 0
	p.runenergy = 10 // will drain to 0

	p.updateEnergy()

	if p.runenergy != 0 {
		t.Errorf("runenergy: got %d, want 0", p.runenergy)
	}
	if p.run != 0 {
		t.Errorf("run: got %d, want 0 (reset on energy=0)", p.run)
	}
	if p.varps[script.VarPlayerRun] != 0 {
		t.Errorf("varps[VarPlayerRun]: got %d, want 0 (varp sync)", p.varps[script.VarPlayerRun])
	}
}

// updateEnergy: runenergy<100 → tempRun=0
func TestUpdateEnergy_EnergyBelow100ResetsTempRun(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.varps = make([]int32, 1)
	p.tempRun = 1
	p.stepsTaken = 2
	p.runweight = 0
	p.runenergy = 150 // 150 - 67 = 83 (<100)

	p.updateEnergy()

	if p.runenergy != 83 {
		t.Errorf("runenergy: got %d, want 83", p.runenergy)
	}
	if p.tempRun != 0 {
		t.Errorf("tempRun: got %d, want 0 (reset when energy<100)", p.tempRun)
	}
}

// updateEnergy: delayed → early return, no mutation
func TestUpdateEnergy_DelayedSkipsAll(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.varps = make([]int32, 1)
	p.delayed = true
	p.stepsTaken = 2
	p.runweight = 0
	p.runenergy = 10000
	p.tempRun = 1

	p.updateEnergy()

	if p.runenergy != 10000 {
		t.Errorf("runenergy: got %d, want 10000 (delayed skip)", p.runenergy)
	}
	if p.tempRun != 1 {
		t.Errorf("tempRun: got %d, want 1 (delayed skip)", p.tempRun)
	}
}
