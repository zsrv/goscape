# NAI-135 Run-Mode Visible-Effect Wiring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port TS `Player.ts:655-712` (`updateMovement` bridge + `defaultMoveSpeed` + `updateEnergy`) so toggling `p.run` produces a visible movement-speed change and run-energy drains/recovers per-tick at TS rates.

**Architecture:** Widen `(*Player).resolveMovement` with a `defaultMoveSpeed`-driven moveSpeed bridge + post-step `tempRun=0` reset. Add `(*Player).updateEnergy` (new file `player_run.go`) and a new `(*Server).processEnergy` tick step between `processWalkTriggerFallbacks` and `processNpcs`. Retire pre-existing per-step `drainRunEnergy` + per-step `runenergy > 0` gate in favor of TS-faithful per-tick drain.

**Tech Stack:** Go 1.26+. Files in `modules/world/`. No interface changes; no external library deps.

**Spec:** `docs/superpowers/specs/2026-05-09-nai-135-run-mode-visible-effect-design.md` (committed at `7aaa4c7`).

---

## File Structure

**Production:**
- Create: `modules/world/player_run.go` — `(*Player).updateEnergy` (T3).
- Modify: `modules/world/movement.go` — add `defaultMoveSpeed`, bridge block + idle reset in `resolveMovement`, retire `drainRunEnergy` + per-step gate (T1, T2, T4).
- Modify: `modules/world/tick.go` — add `(*Server).processEnergy`, wire into `runTickLoopWithRate` (T4).

**Tests:**
- Create: `modules/world/player_run_test.go` — `defaultMoveSpeed` (2 tests), `updateEnergy` (11 tests), and `resolveMovement` bridge + idle reset tests (7 tests). Total 20 tests across this new file (T1, T2, T3).
- Modify: `modules/world/movement_test.go` — update `TestResolveMovementAdvancesTwoTilesRunning` fixture to set `p.run=1; p.runanim=0` (T2). Delete `TestResolveMovementDrainsRunEnergy` (T4).
- Modify: `modules/world/nai101_fountain_test.go` — update `StepThroughDetour` subtest fixture to set `p.run=1; p.runanim=0` (T2).

---

## Task 1: `defaultMoveSpeed` helper + 2 unit tests

**Files:**
- Modify: `modules/world/movement.go` (add helper after line 133, where `drainRunEnergy` ends)
- Create: `modules/world/player_run_test.go`

**Why first:** No behavior change. The function exists but is unreferenced. Decouples the helper port from the bridge wiring (T2), keeping commits atomic.

- [ ] **Step 1.1: Write failing tests in new file**

Create `modules/world/player_run_test.go`:

```go
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
```

- [ ] **Step 1.2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestDefaultMoveSpeed -v`

Expected: compile error or FAIL — `defaultMoveSpeed` undefined on `*Player`.

- [ ] **Step 1.3: Add `defaultMoveSpeed` to movement.go**

Append to `modules/world/movement.go` (after the existing `drainRunEnergy` function at current lines 124-133, which is the end of the file pre-NAI-135):

```go
// defaultMoveSpeed maps p.run → MoveSpeed. Mirrors TS
// Engine-TS/src/engine/entity/Player.ts:710-712:
//
//	defaultMoveSpeed(): MoveSpeed {
//	    return this.run ? MoveSpeed.RUN : MoveSpeed.WALK;
//	}
//
// NAI-135.
func (p *Player) defaultMoveSpeed() MoveSpeed {
	if p.run != 0 {
		return MoveSpeedRun
	}
	return MoveSpeedWalk
}
```

- [ ] **Step 1.4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run TestDefaultMoveSpeed -v`

Expected: PASS for both `TestDefaultMoveSpeed_RunZero` and `TestDefaultMoveSpeed_RunOne`.

- [ ] **Step 1.5: Run full package to verify no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/`

Expected: PASS (no existing tests touch `defaultMoveSpeed`; this is purely additive).

- [ ] **Step 1.6: Commit**

```bash
git add modules/world/movement.go modules/world/player_run_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(nai-135): T1 — defaultMoveSpeed helper

Adds (*Player).defaultMoveSpeed() returning MoveSpeed from p.run.
Mirrors TS Player.ts:710-712. Pure helper — no callers yet (the
resolveMovement bridge that consumes it lands in T2).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `resolveMovement` bridge block + tempRun idle reset

**Files:**
- Modify: `modules/world/movement.go:40-84` (`resolveMovement` body — bridge block insertion + idle reset insertion)
- Modify: `modules/world/movement_test.go:63-82` (`TestResolveMovementAdvancesTwoTilesRunning` fixture update)
- Modify: `modules/world/nai101_fountain_test.go:90-129` (`StepThroughDetour` subtest fixture update)
- Modify: `modules/world/player_run_test.go` (add 7 new tests)

**Why this order:** The bridge OVERWRITES `p.moveSpeed` based on `p.run`/`p.tempRun`/`p.runanim`. Existing tests that set `p.moveSpeed=MoveSpeedRun` directly (to bypass the absent bridge) will now have their setup clobbered to `MoveSpeedWalk` (since `runanim=-1` is the newPlayer default and TS line 663-664 forces Walk on `runanim==-1`). T2 must update those fixtures atomically with the bridge introduction.

- [ ] **Step 2.1: Write failing tests for the bridge + idle reset**

Append to `modules/world/player_run_test.go`:

```go
import (
	"testing"
)

// (already-present TestDefaultMoveSpeed_* tests above)

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
```

- [ ] **Step 2.2: Run new tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run "TestResolveMovement_(Bridge|TempRun)" -v`

Expected: ALL 7 FAIL with mismatched moveSpeed/tempRun expectations.

- [ ] **Step 2.3: Edit `resolveMovement` to add bridge block**

Open `modules/world/movement.go`. Find the `resolveMovement` function starting at line 40. After line 46 (`p.stepsTaken = 0`) and before line 48 (`p.lastTickX = p.x`), insert the bridge block:

```go
	// NAI-135: Bridge p.run → moveSpeed. Mirrors TS Player.ts:661-668.
	// moveSpeed==Instant skip preserves the teleport-jump invariant
	// from P_TELEJUMP / RebuildNormal (TS Player.ts:556).
	if p.moveSpeed != MoveSpeedInstant {
		p.moveSpeed = p.defaultMoveSpeed()
		if p.runanim == -1 {
			p.moveSpeed = MoveSpeedWalk
		} else if p.tempRun != 0 {
			p.moveSpeed = MoveSpeedRun
		}
	}
```

- [ ] **Step 2.4: Edit `resolveMovement` to add tempRun idle reset**

Still in `modules/world/movement.go`, in the same function, after the run-second-step block (which currently ends near line 73 with the closing brace of `if p.moveSpeed == MoveSpeedRun && p.runenergy > 0 ...`) and BEFORE the `// NAI-82: TS Player.processMovement at...` comment block at line 75, insert:

```go
	// NAI-135: Mirrors TS Player.ts:670-673 — TS resets tempRun when no
	// movement happened this tick. stepsTaken==0 is the equivalent of
	// TS's `if (!super.processMovement())` (false ⇒ no steps).
	if p.stepsTaken == 0 {
		p.tempRun = 0
	}
```

- [ ] **Step 2.5: Update `TestResolveMovementAdvancesTwoTilesRunning` fixture**

In `modules/world/movement_test.go`, find `TestResolveMovementAdvancesTwoTilesRunning` (around line 63-82). The current fixture sets only `p.moveSpeed = MoveSpeedRun`. Add the bridge-driving fields. Replace lines 65-67 (`p.x, p.z, p.level = ...; p.moveSpeed = MoveSpeedRun; p.runenergy = 10000`) with:

```go
	p.x, p.z, p.level = 3094, 3106, 0
	p.run = 1
	p.runanim = 0
	p.runenergy = 10000
```

(The `p.moveSpeed = MoveSpeedRun` line is removed — the bridge now writes moveSpeed from `p.run` and `p.runanim`.)

- [ ] **Step 2.6: Update `nai101_fountain_test.go` `StepThroughDetour` fixture**

In `modules/world/nai101_fountain_test.go`, find the `StepThroughDetour` subtest (around line 90-129). The current fixture sets `p.moveSpeed = MoveSpeedRun` at line 94. Replace lines 93-95 (`p.x, p.z, p.level = 3222, 3225, 0; p.moveSpeed = MoveSpeedRun; p.runenergy = 10000`) with:

```go
			p.x, p.z, p.level = 3222, 3225, 0
			p.run = 1
			p.runanim = 0
			p.runenergy = 10000
```

- [ ] **Step 2.7: Run new tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run "TestResolveMovement_(Bridge|TempRun)" -v`

Expected: ALL 7 PASS.

- [ ] **Step 2.8: Run updated existing tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run "TestResolveMovementAdvancesTwoTilesRunning|TestNAI101FountainRoute" -v`

Expected: PASS. (`TestResolveMovementAdvancesTwoTilesRunning` still asserts the player advances to z=3108 — this works as long as moveSpeed=Run after the bridge runs, which it does given run=1, runanim=0.)

Note: `TestResolveMovementDrainsRunEnergy` at `movement_test.go:166-180` still passes at this point because `drainRunEnergy` is still called inside `resolveMovement`. It will be deleted in T4 when `drainRunEnergy` is retired.

- [ ] **Step 2.9: Run full package**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/`

Expected: PASS.

- [ ] **Step 2.10: Commit**

```bash
git add modules/world/movement.go modules/world/movement_test.go modules/world/nai101_fountain_test.go modules/world/player_run_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(nai-135): T2 — resolveMovement bridge + tempRun idle reset

Wires p.run + p.tempRun + p.runanim → p.moveSpeed via the bridge
block at the top of resolveMovement (TS Player.ts:661-668), and
adds the post-step tempRun=0 reset on idle (TS Player.ts:670-673).

Bridge skips moveSpeed==Instant to preserve the P_TELEJUMP /
RebuildNormal jump invariant (TS Player.ts:556).

Updates two existing fixtures (TestResolveMovementAdvancesTwoTilesRunning,
nai101_fountain_test.go StepThroughDetour) to drive moveSpeed via the
bridge: p.run=1; p.runanim=0 instead of p.moveSpeed=MoveSpeedRun.
runanim=-1 (newPlayer default) forces Walk per the bridge.

Adds 7 unit tests for bridge + idle reset coverage.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `(*Player).updateEnergy` + 11 unit tests

**Files:**
- Create: `modules/world/player_run.go`
- Modify: `modules/world/player_run_test.go` (append 11 tests)

**Why this order:** `updateEnergy` is self-contained — no caller wiring yet. T4 wires it into the tick loop. Splitting the function port from the wiring keeps the diff readable and lets tests pin behavior exhaustively before integration.

- [ ] **Step 3.1: Write failing tests**

Append to `modules/world/player_run_test.go`:

```go
import (
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

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
```

Note the new imports — append to the existing `import (...)` block at the top of `player_run_test.go`. The block should now read:

```go
import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)
```

- [ ] **Step 3.2: Run new tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run "TestUpdateEnergy_" -v`

Expected: compile error — `updateEnergy` undefined on `*Player`.

- [ ] **Step 3.3: Create `player_run.go`**

Create new file `modules/world/player_run.go`:

```go
package world

import (
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// updateEnergy drains or recovers run energy for one tick, and
// disables run-mode at low-energy thresholds.
//
// Mirrors TS Engine-TS/src/engine/entity/Player.ts:682-704
// line-for-line. Called once per player per tick from
// (*Server).processEnergy after movement + interactions resolve.
//
// stepsTaken < 2 is the TS "idle / single-step" recovery branch;
// stepsTaken >= 2 is the running-this-tick drain branch (a clean
// run-step emits walk + run for stepsTaken==2).
//
// runweight is in grams; TS divides by 1000 to convert to kg, then
// clamps to [0, 64]. Loss formula = floor(67 + 67*kg/64).
//
// At runenergy==0: clear p.run AND propagate via SetVarp(VarPlayerRun)
// so the client's varp-driven run-toggle UI updates. Mirrors TS
// Player.ts:697-699.
//
// At runenergy<100: clear p.tempRun (TS Player.ts:701-703).
//
// NAI-135.
func (p *Player) updateEnergy() {
	if p.delayed {
		return
	}
	if p.stepsTaken < 2 {
		agility := int(p.baseLevels[objtype.PlayerStatAgility])
		recovered := agility/9 + 8
		p.runenergy = min(p.runenergy+recovered, 10000)
	} else {
		weightKg := p.runweight / 1000
		clampWeight := max(min(weightKg, 64), 0)
		loss := 67 + 67*clampWeight/64
		p.runenergy = max(p.runenergy-loss, 0)
	}
	if p.runenergy == 0 {
		p.run = 0
		p.SetVarp(script.VarPlayerRun, 0)
	}
	if p.runenergy < 100 {
		p.tempRun = 0
	}
}
```

- [ ] **Step 3.4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/ -run "TestUpdateEnergy_" -v`

Expected: ALL 11 PASS.

- [ ] **Step 3.5: Run full package**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/`

Expected: PASS. updateEnergy is not yet called from anywhere in production, so no behavior change in other tests.

- [ ] **Step 3.6: Commit**

```bash
git add modules/world/player_run.go modules/world/player_run_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(nai-135): T3 — (*Player).updateEnergy

Ports TS Player.ts:682-704 line-for-line. Per-tick run-energy drain
(stepsTaken≥2) or recovery (stepsTaken<2). Drain formula uses
runweight/1000 → kg with [0,64] clamp, then loss = 67 + 67*kg/64.
Recovery formula = baseLevels[Agility]/9 + 8. Clamps runenergy to
[0, 10000].

At runenergy=0: clears p.run + SetVarp(VarPlayerRun, 0) for
varp-driven UI sync. At runenergy<100: clears p.tempRun.

Function not yet called from production — wiring lands in T4.

11 unit tests cover both branches + clamps + reset gates + delayed
early-return.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Wire `processEnergy` into tick + retire `drainRunEnergy` + retire per-step gate

**Files:**
- Modify: `modules/world/tick.go` — add `(*Server).processEnergy`, wire into `runTickLoopWithRate` between `processWalkTriggerFallbacks` and `processNpcs`
- Modify: `modules/world/movement.go` — drop `&& p.runenergy > 0` from line 67 gate, drop `p.drainRunEnergy()` call at line 71, delete `drainRunEnergy` function at lines 124-133
- Modify: `modules/world/movement_test.go` — delete `TestResolveMovementDrainsRunEnergy` (lines 166-180)

**Why this is one commit:** Splitting the wire-in (additive drain) from the per-step retirement (subtractive drain) creates either (a) a double-drain interim (after wire-in, before retirement) or (b) a no-drain interim (after retirement, before wire-in). Both poison `git bisect` and any test run between commits. Single atomic swap.

- [ ] **Step 4.1: Add `processEnergy` to tick.go**

Open `modules/world/tick.go`. Find `(*Server).processPathing` at line 234. After its closing brace (line 243), add `processEnergy` as a sibling:

```go
// processEnergy drives one tick of per-player run-energy
// drain/recovery + run-mode auto-disable. Mirrors TS World.ts:731
// (player.updateEnergy() per-player iteration). NAI-135.
func (s *Server) processEnergy() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()

	for _, p := range players {
		p.updateEnergy()
	}
}
```

- [ ] **Step 4.2: Wire `processEnergy` into the tick loop**

In `modules/world/tick.go`, find `runTickLoopWithRate`. Locate the `s.processWalkTriggerFallbacks()` call (current line 56). Insert `s.processEnergy()` immediately after it, before `s.processNpcs()` (current line 57):

```go
		s.processWalkTriggerFallbacks() // NAI-77 T3: TS World.ts:635-641 per-tick re-path + PLAYERSETUP walktrigger
		s.processEnergy()               // NAI-135: TS World.ts:731 per-player updateEnergy
		s.processNpcs()
```

- [ ] **Step 4.3: Drop `&& p.runenergy > 0` from per-step gate**

In `modules/world/movement.go`, find line 67:

```go
	if p.moveSpeed == MoveSpeedRun && p.runenergy > 0 && p.waypointIndex >= 0 {
```

Change to:

```go
	if p.moveSpeed == MoveSpeedRun && p.waypointIndex >= 0 {
```

(TS doesn't gate per-step on energy; the energy=0 → walk transition happens in `updateEnergy` at end of tick, taking effect next tick via `defaultMoveSpeed`.)

- [ ] **Step 4.4: Remove `p.drainRunEnergy()` call**

In `modules/world/movement.go`, inside the run-second-step block (current line 67-73), remove the `p.drainRunEnergy()` line. The block becomes:

```go
	if p.moveSpeed == MoveSpeedRun && p.waypointIndex >= 0 {
		dir2, ok2 := p.stepOnce()
		if ok2 {
			p.runDir = int(dir2)
		}
	}
```

- [ ] **Step 4.5: Delete `drainRunEnergy` function**

In `modules/world/movement.go`, delete the entire `drainRunEnergy` function (current lines 123-133, including the `// drainRunEnergy applies the TS run-energy decay formula once per running step.` doc-comment at line 123):

```go
// DELETE THIS BLOCK
// drainRunEnergy applies the TS run-energy decay formula once per running step.
func (p *Player) drainRunEnergy() {
	decay := (67 + 67*p.runweight/64) / 100
	if decay < 1 {
		decay = 1
	}
	p.runenergy -= decay
	if p.runenergy < 0 {
		p.runenergy = 0
	}
}
```

- [ ] **Step 4.6: Delete `TestResolveMovementDrainsRunEnergy`**

In `modules/world/movement_test.go`, delete the entire `TestResolveMovementDrainsRunEnergy` function at lines 166-180:

```go
// DELETE THIS BLOCK
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
```

The SUT (`drainRunEnergy`) is retired; the test would fail by referring to a deleted function and is irrelevant under the new model. `updateEnergy` drain coverage lives in `player_run_test.go` (T3).

- [ ] **Step 4.7: Build to verify no compile errors**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: clean (no references to deleted `drainRunEnergy` or deleted test function remain).

- [ ] **Step 4.8: Run modules/world tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./modules/world/`

Expected: PASS. All NAI-135 unit tests still green; all updated existing tests still green; no new failures.

- [ ] **Step 4.9: Run full repo + race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...`

Expected: PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -race ./...`

Expected: PASS. (NAI-122 / NAI-127 tick-order tests must remain green — `processEnergy` is purely additive between `processWalkTriggerFallbacks` and `processNpcs`; no existing call moves.)

- [ ] **Step 4.10: Run vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: clean.

- [ ] **Step 4.11: Commit**

```bash
git add modules/world/tick.go modules/world/movement.go modules/world/movement_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(nai-135): T4 — wire processEnergy + retire drainRunEnergy

Adds (*Server).processEnergy iterating s.playerLoop and dispatching
(*Player).updateEnergy per-player. Wired into runTickLoopWithRate
between processWalkTriggerFallbacks and processNpcs, mirroring TS
World.ts:731 placement (post-processInteraction).

Retires two pre-existing unflagged divergences in the same atomic
swap so no commit produces a double-drain or no-drain interim:

  1. Per-step `runenergy > 0` gate at movement.go:67 — TS doesn't
     gate per-step on energy. Energy=0 transition now propagates
     via updateEnergy → defaultMoveSpeed next tick.
  2. (*Player).drainRunEnergy + its caller in resolveMovement —
     replaced by per-tick drain in updateEnergy.

Deletes TestResolveMovementDrainsRunEnergy: SUT retired, drain
coverage lives in player_run_test.go (T3).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Verification + close commit

**Files:** none (verification + close-memo commit only)

- [ ] **Step 5.1: Full repo verification battery**

Run, in order:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=1 -race ./...
```

Expected: all four GREEN.

- [ ] **Step 5.2: Verify deleted symbols are gone**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache grep -nE "drainRunEnergy|TestResolveMovementDrainsRunEnergy" modules/world/`

Expected: no output. Confirms no stale references survive (per `verify_implementer_claims`).

- [ ] **Step 5.3: Verify `processEnergy` is wired**

Run: `grep -nE "processEnergy" modules/world/tick.go`

Expected: at least 2 hits — one for the function definition, one for the call site in `runTickLoopWithRate`.

- [ ] **Step 5.4: Smoke handoff to user**

Per `smoke_test_server_handoff`: emit a message in the conversation asking the user to launch the server and verify two binding criteria from spec §9:

1. **PRIMARY:** Toggle run-mode UI button → walk to next tile → player visibly runs (covers two tiles per tick instead of one). Toggle off → player walks.
2. **SECONDARY:** Run continuously across a long span → `runenergy` UI bar drains visibly → at 0, player auto-reverts to walk + run-toggle UI clears.

Wait for user smoke result. If PRIMARY binds, proceed to Step 5.5. If PRIMARY fails, do NOT close — investigate per `cascade_theory_smoke_binding`.

- [ ] **Step 5.5: Close commit**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-135 — run-mode visible-effect wiring; smoke confirmed PRIMARY met

Ports TS Player.ts:655-712 (updateMovement bridge + defaultMoveSpeed
+ updateEnergy) closing the orphan-write divergence where p.run was
set by NAI-117 P_RUN handler but never read in production. Toggling
run-mode now produces a visible movement-speed change.

T1 (7aaa4c7 → next): defaultMoveSpeed helper.
T2: resolveMovement bridge block + tempRun idle reset; 7 tests +
    2 existing fixture updates (p.run/runanim instead of moveSpeed).
T3: (*Player).updateEnergy ported line-for-line from
    Player.ts:682-704; 11 tests covering both branches + clamps +
    reset gates + delayed early-return.
T4: (*Server).processEnergy wired between processWalkTriggerFallbacks
    and processNpcs (TS World.ts:731 placement); per-step
    drainRunEnergy + per-step runenergy>0 gate retired in the same
    atomic swap.
T5: full-repo green; smoke bound visible run-mode effect + energy
    drain + energy-zero auto-disable.

No interface changes. No DEVIATION-NAI-135-* tags introduced. Two
pre-existing unflagged divergences (per-step drain magnitude, per-step
energy gate) retired.

Carry-forward routing (NAI-136+): NAI-127 carryovers (range-across-fence,
arrows-not-consumed), NAI-115 P1/P2, NAI-119 weapon-equip, NAI-111
P_TELEJUMP. All prior parked deviations unchanged.

Closes memory:
  - true_to_ts_gate (full TS-fidelity port; no deviations opened)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

(Empty close commit — all production diffs already landed in T1-T4. The close-memo body documents the bundle.)

---

## Self-Review

**Spec coverage:**
- §1 Goal: covered by T1 (defaultMoveSpeed), T2 (bridge), T3 (updateEnergy), T4 (wiring + retirements). ✓
- §2 TS source anchors: each task cites exact TS line ranges. ✓
- §3 Non-goals: no opcode work, no new VarPlayerType, no agility XP, no NPC run-mode, no smoke harness — none of these surfaces in the plan. ✓
- §4 Architecture (tick placement post-processWalkTriggerFallbacks pre-processNpcs): T4 Step 4.2. ✓
- §5.1 defaultMoveSpeed: T1. ✓
- §5.2 resolveMovement widening: T2 (Step 2.3 bridge, Step 2.4 idle reset). ✓
- §5.3 updateEnergy: T3. ✓
- §5.4 drainRunEnergy retirement: T4 (Steps 4.3, 4.4, 4.5). ✓
- §5.5 processEnergy: T4 (Step 4.1). ✓
- §5.6 Tick-loop integration: T4 (Step 4.2). ✓
- §6 Error handling: delayed early-return covered by `TestUpdateEnergy_DelayedSkipsAll` (T3). ✓
- §7.1 New tests (2 + 5 + 2 + 11 = 20): T1 (2), T2 (7), T3 (11). ✓
- §7.2 Existing test updates: T2 Steps 2.5/2.6 (fixture updates), T4 Step 4.6 (delete). ✓
- §7.3 Tick-order regression: T4 Step 4.9. ✓
- §7.4 Smoke: T5 Step 5.4. ✓
- §8 Risks (per-step retirement, energy=0 mid-tick, tick-order, agility uninit, min/max, SetVarp client-nil, processEnergy locking): all addressed by spec text + corresponding test coverage in T3. ✓
- §10 Acceptance (`go test ./...`, race, vet, no regression in NAI-101/117/122/127/130-134, doc-comments cite Player.ts:655-712, no DEVIATION tags): T5 Steps 5.1-5.3 + T1/T3/T4 doc-comments. ✓

No gaps.

**Placeholder scan:** none (no TBD/TODO/"appropriate"/"similar to" patterns).

**Type consistency:**
- `MoveSpeed` constants (`MoveSpeedWalk`, `MoveSpeedRun`, `MoveSpeedInstant`): consistent across T1, T2, T3.
- `objtype.PlayerStatAgility = 16`: used identically in §5.3 spec body and T3 Step 3.3.
- `script.VarPlayerRun = 0`: used identically in §5.3 spec body and T3 Step 3.3.
- `(*Player).SetVarp(id int, val int32)`: signature matches existing definition at `player_script.go:317`.
- `p.varps []int32`: tests size with `make([]int32, 1)` per existing convention at `script_test.go:399`/`:427`/`:455`.
- `p.baseLevels [21]uint8`: drained via `int(p.baseLevels[...])` cast (uint8→int).
- `p.runweight int`, `p.runenergy int`: scalar arithmetic with go 1.21+ built-in min/max on `int`.
- `(*Server).processEnergy`: signature `func (s *Server) processEnergy()` consistent across T4 Step 4.1 (definition) and Step 4.2 (call site).
- `p.delayed bool`, `p.tempRun int`, `p.run int`, `p.runanim int`: all field types verified at HEAD against `player.go:137`, `:201`, `:323`.

No inconsistencies.
