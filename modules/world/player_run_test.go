package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
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
	// pathing-2: newTestPlayer leaves moveSpeed = MoveSpeedInstant (player.go
	// constructor default). The bridge in resolveMovement preserves Instant, and
	// pathing-2's new early-return (TS PathingEntity.ts:134-137) suppresses
	// stepping when moveSpeed is Instant. Set MoveSpeedWalk explicitly so the
	// bridge fires and elevates to Run (runanim==0 && tempRun==1 → Run path).
	p.moveSpeed = MoveSpeedWalk
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
		t.Errorf("runenergy: got %d, want 5008 (5000 + 0/6 + 8; agility=0 is unchanged by /6 vs /9)", p.runenergy)
	}
}

// updateEnergy: recover branch — agility=99, +24/tick (99/6=16; +8).
// 225 was /9: 99/9=11, +8=19. 244 formula: TS Player.ts:692.
func TestUpdateEnergy_RecoverBranchAgilityNinetyNine(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.varps = make([]int32, 1)
	p.stepsTaken = 0
	p.baseLevels[objtype.PlayerStatAgility] = 99
	p.runenergy = 0

	p.updateEnergy()

	if p.runenergy != 24 {
		t.Errorf("runenergy: got %d, want 24 (99/6=16, +8; 244 formula)", p.runenergy)
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

// player-core-2: TS Player.updateEnergy (Player.ts:690-693) computes
// weightKg as float (`this.runweight / 1000`) and clamps the float
// BEFORE the 67*weightKg/64 math; only the final `loss` expression is
// truncated via `| 0`. goscape's int division `weightKg :=
// p.runweight/1000` drops the kg fraction BEFORE the drain math,
// producing systematically lower drain for any partial-kg encumbrance
// (the common in-game case).
//
// With runweight=32500 (32.5 kg):
//
//	TS:     (67 + 67*32.5/64) | 0 = (67 + 34.023) | 0 = 101 drain
//	bug:    67 + 67*32/64 (int div) = 67 + 33 = 100 drain
//
// After the float port, both yield 101.
func TestUpdateEnergy_DrainFractionalKgUsesFloatMath(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.varps = make([]int32, 1)
	p.stepsTaken = 2
	p.runweight = 32500 // 32.5 kg — falls between two integer-kg bins
	p.runenergy = 10000

	p.updateEnergy()

	if p.runenergy != 9899 {
		t.Errorf("runenergy: got %d, want 9899 (10000 - (67 + 67*32.5/64 | 0) = 10000 - 101 — int-divide bug would give 9900)", p.runenergy)
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

// updateEnergy: runenergy=0 → run=0 + varp sync to clientcode-7-resolved id
func TestUpdateEnergy_EnergyZeroResetsRunAndVarp(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := newTestServer(t)
	s.varpTypes = &objtype.VarpTypeConfigs{
		Configs: make([]*objtype.VarPlayerType, 174),
		RunID:   173,
	}
	p.client.server = s

	p.varps = make([]int32, 174)
	p.varps[173] = 1 // pre-state: run-mode varp is on
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
	if p.varps[173] != 0 {
		t.Errorf("varps[173] (RunID=173): got %d, want 0", p.varps[173])
	}
	if p.varps[0] != 0 {
		t.Errorf("varps[0]: got %d, want 0 (sanity: no write hit hardcoded id 0)", p.varps[0])
	}
}

// updateEnergy: runenergy=0 + RunID=0 → write still lands at the TS
// placeholder default id (0). Sanity-pins parity with TS behavior when
// the cache lacks a clientcode-7 config (Engine-TS/src/cache/config/VarPlayerType.ts:18 default).
func TestUpdateEnergy_EnergyZeroNoEmitWhenRunIDZero(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := newTestServer(t)
	s.varpTypes = &objtype.VarpTypeConfigs{
		Configs: make([]*objtype.VarPlayerType, 1),
		RunID:   0,
	}
	p.client.server = s

	p.varps = make([]int32, 1)
	p.varps[0] = 1
	p.run = 1
	p.stepsTaken = 2
	p.runweight = 0
	p.runenergy = 10

	p.updateEnergy()

	if p.varps[0] != 0 {
		t.Errorf("varps[0]: got %d, want 0 (RunID=0 default lands at id 0)", p.varps[0])
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

// updateEnergy: recover branch — agility=60, +18/tick per 244 contract (60/6=10; +8).
// 225 formula was /9: 60/9=6, +8=14. Pins TS Player.ts:692 rev-244 change.
func TestUpdateEnergy_RecoverBranchAgility60_244Formula(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.varps = make([]int32, 1)
	p.stepsTaken = 0
	p.baseLevels[objtype.PlayerStatAgility] = 60
	p.runenergy = 0

	p.updateEnergy()

	if p.runenergy != 18 {
		t.Errorf("runenergy: got %d, want 18 (60/6=10, +8; 244 formula /6, was /9=14)", p.runenergy)
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
