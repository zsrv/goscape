# NAI-135 — Run-mode visible-effect wiring (defaultMoveSpeed + tempRun + updateEnergy)

**Status:** spec
**Date:** 2026-05-09
**Predecessors:** NAI-117 (P_RUN/RUNENERGY handler ports — opcode-error silence) closed at `044f1bb`. NAI-117 spec §8 anticipated this follow-up.
**Tech stack:** Go 1.26+

## 1. Goal

Port TS `Player.ts:655-712` (`updateMovement` bridge + `defaultMoveSpeed` + `updateEnergy`) so that:

1. Toggling run-mode (via P_RUN, the run-mode UI button, or any other path that writes `p.run`) produces a **visible movement-speed change** the next time the player moves.
2. Run-energy drains while running and recovers while idle, at TS rates.
3. At runenergy=0, run-mode auto-disables (player reverts to walk + UI run-toggle clears via varp sync).
4. At runenergy<100, the one-shot `tempRun` (ctrl-held walk) auto-clears.

Closes the orphaned-write divergence: `p.run = v` is set by `handlePRun` at `pkg/script/handlers_player.go:641` → `(*Player).SetRun` at `modules/world/player_script.go:348` — but `p.run` is never read in production at HEAD `7d60cad`.

## 2. TS source — anchored

- **`updateMovement`**: `Engine-TS/src/engine/entity/Player.ts:655-680` (in particular the bridge block at `:661-668` and the post-step `tempRun=0` reset at `:670-673`).
- **`updateEnergy`**: `Engine-TS/src/engine/entity/Player.ts:682-704`.
- **`defaultMoveSpeed`**: `Engine-TS/src/engine/entity/Player.ts:710-712`.
- **Call sites**: `World.ts:731` calls `player.updateEnergy()` after `processInteraction` (which internally calls `updateMovement` at `Player.ts:1241`).

## 3. Non-goals

- No script-side opcode work — this is engine-layer only. NAI-117 already ported P_RUN (opcode 2085) and RUNENERGY (opcode 2096).
- No new VarPlayerType definitions — `script.VarPlayerRun = 0` already exists at `pkg/script/active.go:7` (NAI-117).
- No agility-skill XP wiring or skill training — just reading `baseLevels[Agility]` for the recovery formula.
- No NPC run-mode (NPCs don't have `tempRun`/`runenergy`; this is a player-only subsystem).
- No broader stat-recovery framework — `updateEnergy` is self-contained.
- Smoke harness work is user-driven per `smoke_test_server_handoff`.

## 4. Architecture

Goscape's `resolveMovement` (`modules/world/movement.go:40`) is the structural parallel of TS `updateMovement` — both run before interactions resolve, both step the player along their waypoint queue, both write `walkDir`/`runDir`/`stepsTaken`. NAI-135 widens `resolveMovement` to include the `defaultMoveSpeed` bridge (currently absent) and the post-step `tempRun=0` reset (currently absent).

Goscape has no parallel of TS `updateEnergy`. NAI-135 introduces `(*Player).updateEnergy` and a new tick step `(*Server).processEnergy` that iterates `s.playerLoop` and dispatches the per-player call. Tick placement: between `processWalkTriggerFallbacks` and `processNpcs`, mirroring TS's `World.ts:731` placement (post-`processInteraction`).

Goscape's existing per-step `drainRunEnergy` (`movement.go:124-133`) and the per-step `runenergy > 0` gate (`movement.go:67`) are pre-NAI-135 divergences (TS doesn't drain per-step and doesn't gate per-step on energy). Both are RETIRED in NAI-135 in favor of the TS-faithful per-tick drain in `updateEnergy`.

```
Per-tick (per player) sequence
──────────────────────────────
processPathing
  resolveMovement
    bridge:  if moveSpeed != Instant:
               moveSpeed = defaultMoveSpeed()         ← TS Player.ts:662
               if runanim == -1: moveSpeed = Walk     ← TS Player.ts:663-664
               elif tempRun:     moveSpeed = Run      ← TS Player.ts:665-666
    step loop:  walk + (run-second-step if moveSpeed==Run)
    idle reset: if stepsTaken == 0: tempRun = 0       ← TS Player.ts:670-673
    lastMovement: if stepsTaken > 0: lastMovement = currentTick+1

processInteractions
processWalkTriggerFallbacks
processEnergy                                          ← NEW (TS World.ts:731)
  for p in playerLoop:
    p.updateEnergy()
      if delayed: return                               ← TS Player.ts:683-685
      if stepsTaken < 2:
        recovered = baseLevels[Agility]/9 + 8          ← TS Player.ts:687
        runenergy = min(runenergy + recovered, 10000)
      else:
        weightKg = runweight / 1000
        clampWeight = clamp(weightKg, 0, 64)
        loss = 67 + 67*clampWeight/64                  ← TS Player.ts:692
        runenergy = max(runenergy - loss, 0)
      if runenergy == 0:
        run = 0
        SetVarp(VarPlayerRun, 0)                       ← TS Player.ts:697-699
      if runenergy < 100:
        tempRun = 0                                    ← TS Player.ts:701-703

processNpcs ...
```

## 5. Component contracts

### 5.1 `(*Player).defaultMoveSpeed() MoveSpeed`

**Location:** `modules/world/movement.go` (next to `resolveMovement`).

**TS reference:** Player.ts:710-712.

**Body:**
```go
func (p *Player) defaultMoveSpeed() MoveSpeed {
    if p.run != 0 {
        return MoveSpeedRun
    }
    return MoveSpeedWalk
}
```

**Doc-comment:** "defaultMoveSpeed maps p.run → MoveSpeed. Mirrors TS Player.ts:710-712."

### 5.2 `resolveMovement` widening (movement.go:40)

**Insertion 1** — bridge block, after `p.stepsTaken = 0` (current line 46), before `p.lastTickX = p.x` (current line 48):

```go
// Bridge p.run → moveSpeed. Mirrors TS Player.ts:661-668.
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

**Modification** — line 67 gate. Drop the `&& p.runenergy > 0` clause:
```go
if p.moveSpeed == MoveSpeedRun && p.waypointIndex >= 0 {
```

**Modification** — remove the `p.drainRunEnergy()` call inside the run-second-step block (current line 71). The block becomes:
```go
if p.moveSpeed == MoveSpeedRun && p.waypointIndex >= 0 {
    dir2, ok2 := p.stepOnce()
    if ok2 {
        p.runDir = int(dir2)
    }
}
```

**Insertion 2** — idle reset, after the run-second-step block, before the `lastMovement` write at current line 81:
```go
// Mirrors TS Player.ts:670-673 — TS resets tempRun when no movement
// happened this tick. stepsTaken==0 is the equivalent of TS's
// `if (!super.processMovement())` (false ⇒ no steps).
if p.stepsTaken == 0 {
    p.tempRun = 0
}
```

### 5.3 `(*Player).updateEnergy()` (new)

**Location:** new file `modules/world/player_run.go`.

**TS reference:** Player.ts:682-704.

**Body:**
```go
package world

import (
    "github.com/zsrv/goscape/pkg/objtype"
    "github.com/zsrv/goscape/pkg/script"
)

// updateEnergy drains or recovers run energy for one tick, and
// disables run-mode at low-energy thresholds.
//
// Mirrors TS Player.ts:682-704 line-for-line. Called once per
// player per tick from Server.processEnergy after movement +
// interactions resolve.
//
// stepsTaken < 2 is the TS "idle / single-step" recovery branch;
// stepsTaken >= 2 is the running-this-tick drain branch.
//
// runweight is in grams; TS divides by 1000 to convert to kg, then
// clamps to [0, 64]. Loss formula = floor(67 + 67*kg/64).
//
// At runenergy==0: clear p.run AND propagate via SetVarp(VarPlayerRun)
// so the client's varp-driven run-toggle UI updates.
//
// At runenergy<100: clear p.tempRun.
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

### 5.4 `drainRunEnergy` retirement

Delete from `movement.go`:
- The function definition at lines 124-133 (10 lines).
- The single call site at line 71 (1 line).

### 5.5 `(*Server).processEnergy()` (new)

**Location:** `modules/world/tick.go`, next to `processPathing` (around line 234).

**Body:**
```go
// processEnergy drives one tick of per-player run-energy
// drain/recovery + run-mode auto-disable. Mirrors TS World.ts:731
// (player.updateEnergy() per-player iteration).
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

### 5.6 Tick-loop integration (tick.go runTickLoopWithRate)

Insert one new step after `s.processWalkTriggerFallbacks()` (current line 56), before `s.processNpcs()` (current line 57):

```go
s.processWalkTriggerFallbacks() // NAI-77 T3
s.processEnergy()               // NAI-135 — TS World.ts:731
s.processNpcs()
```

## 6. Error handling

- `updateEnergy` early-returns on `p.delayed` (matches TS).
- No nil-guards needed: `(*Player).baseLevels` is `[21]uint8` (value type, always present); `p.runweight` is `int`; `p.runenergy` is `int`.
- `SetVarp(VarPlayerRun, 0)` is a value-method on `*Player` and already in production use (handlers_player.go:641 NAI-117). It internally writes `p.varps[id]` and emits the varp packet to the client — the client guard handles disconnected players.
- `processEnergy` follows the established `processFoo()` pattern: snapshot `s.playerLoop` under read lock, then iterate without lock held — same shape as `processPathing`.

## 7. Testing strategy

### 7.1 New `modules/world/player_run_test.go`

**`defaultMoveSpeed`** (2 tests):
- `TestDefaultMoveSpeed_RunZero` — `run=0 → Walk`
- `TestDefaultMoveSpeed_RunOne` — `run=1 → Run`

**`resolveMovement` bridge** (5 tests, fixtures use bare *Player with `client=nil` per existing convention; assert post-`resolveMovement` values):
- `TestResolveMovement_BridgeRunEnabled` — `run=1, runanim=0, tempRun=0 → moveSpeed=Run`
- `TestResolveMovement_BridgeTempRunOverride` — `run=0, tempRun=1, runanim=0 → moveSpeed=Run`
- `TestResolveMovement_BridgeRunanimMinusOne` — `run=1, runanim=-1, tempRun=1 → moveSpeed=Walk` (runanim==-1 wins per TS line 663-664 structure)
- `TestResolveMovement_BridgeRunZero` — `run=0, tempRun=0, runanim=0 → moveSpeed=Walk`
- `TestResolveMovement_BridgeInstantPreserved` — `moveSpeed=Instant pre-call → moveSpeed=Instant post-call`

**`tempRun` idle reset** (2 tests):
- `TestResolveMovement_TempRunResetOnIdle` — `tempRun=1, no waypoints → tempRun=0` post-tick
- `TestResolveMovement_TempRunPreservedDuringSteps` — `tempRun=1, waypointIndex>=0, takes step → tempRun=1` post-tick

**`updateEnergy`** (11 tests):
- `TestUpdateEnergy_RecoverBranchAgilityZero` — `stepsTaken=0, baseLevels[Agility]=0, runenergy=5000 → 5008`
- `TestUpdateEnergy_RecoverBranchAgilityNinetyNine` — `stepsTaken=0, baseLevels[Agility]=99, runenergy=0 → 19` (99/9=11, +8)
- `TestUpdateEnergy_RecoverClampAt10000` — `stepsTaken=0, runenergy=9995 → 10000`
- `TestUpdateEnergy_DrainWeightZero` — `stepsTaken=2, runweight=0, runenergy=10000 → 9933` (-67)
- `TestUpdateEnergy_DrainWeight64kg` — `stepsTaken=2, runweight=64000, runenergy=10000 → 9866` (-134)
- `TestUpdateEnergy_DrainClampAtZero` — `stepsTaken=2, runenergy=10 → 0`
- `TestUpdateEnergy_DrainWeightNegativeClampedToZero` — `stepsTaken=2, runweight=-1000 → -67`
- `TestUpdateEnergy_DrainWeightOverflowClampedTo64kg` — `stepsTaken=2, runweight=200000 → -134`
- `TestUpdateEnergy_EnergyZeroResetsRunAndVarp` — drain to 0, assert `p.run==0` AND `p.varps[VarPlayerRun]==0` (varp recorder pattern from existing tests)
- `TestUpdateEnergy_EnergyBelow100ResetsTempRun` — `runenergy=99 → tempRun=0`
- `TestUpdateEnergy_DelayedSkipsAll` — `delayed=true, stepsTaken=2, runenergy=10000 → unchanged`

### 7.2 Existing test updates

- `movement_test.go` `TestDrainRunEnergy` (around line 170-178) — the function under test no longer exists. Either:
  (a) **Delete the test** (preferred — the per-step drain semantics are gone), OR
  (b) Convert to a `TestUpdateEnergy_DrainBranch` smoke (but the new `player_run_test.go` already covers this).
  Per `test_sut_vs_setup_distinction`: this test was pinning the per-step drain SUT. With SUT retired, delete is correct.
- `movement_test.go` other run-step tests (around lines 140, 169, 257, 372) — verify they still pass under no-per-step-drain. Most set `runenergy=10000` and assert post-step coords; energy is incidental.
- `nai101_fountain_test.go:95` — pre-tick `runenergy=10000`; teleport assertion. No per-step drain change to fountain test outcome.

### 7.3 Tick-order regression

Run full repo `go test ./...` and `go test -race ./modules/world/...` to verify no NAI-122/NAI-127 tick-order regression. The new `processEnergy` step is purely additive between two existing steps; no existing call moves.

### 7.4 Smoke

User-launched per `smoke_test_server_handoff`. Two binding criteria:

1. **PRIMARY:** Tutorial Island, post-Master-Chef, toggle run-mode UI button → walk to next tile → player visibly runs (covers two tiles per tick instead of one). Pressing the button again → walks.
2. **SECONDARY:** Run continuously across a long span → `runenergy` UI bar drains visibly → at 0, player auto-reverts to walk + run-toggle UI clears.

Per `cascade_theory_smoke_binding`: PRIMARY closure on visible-effect bind. SECONDARY is observable but not strictly required for close (energy magnitudes match TS by construction; if SECONDARY drift is visible, it routes to a follow-up).

## 8. Risk register

| Risk | Mitigation |
|---|---|
| NAI-101 fountain teleport invariant breaks | nai101_fountain_test.go covered in §7.2; pre-tick fixture sets runenergy=10000, post-tick assertion is teleport-coord based — no per-step decrement assumed |
| Energy-zero player keeps running mid-tick | TS-faithful — TS player runs the full tick on stale energy; energy hits 0 in updateEnergy at end of tick; next tick defaultMoveSpeed sees run=0 and reverts to Walk. Documented in §4 architecture |
| Tick-order placement misses interactions that re-trigger movement | Goscape's processInteractions doesn't add steps within the same tick (verified — interactions resolve intent for next tick). Mirror TS placement at World.ts:731 |
| baseLevels[Agility] uninitialized for fresh chars (zero-init) | Fresh char agility level 0 → recovered = 0/9 + 8 = 8/tick. TS at agility level 1 (the actual minimum) → 1/9 + 8 = 8/tick. Behavior equivalent to ±1 LSB; no defensive guard needed |
| min/max generic resolution | go 1.21+ built-in `min`/`max`; all operands are `int`; no ambiguity |
| SetVarp side effect on disconnected client | SetVarp internally guards client-write per existing handlers_player.go:641 NAI-117 path; updateEnergy inherits the same safety |
| processEnergy under playersMu | Snapshot-then-iterate pattern matches processPathing/processObjDelayedQueue siblings; no new lock contention |

## 9. Open questions

None.

## 10. Acceptance

- All new tests in `modules/world/player_run_test.go` green.
- `movement_test.go` `TestDrainRunEnergy` deleted; other run-related tests still green.
- `go test ./...` passes.
- `go test -race ./...` passes.
- `go build ./...` clean.
- `go vet ./...` clean.
- No regression in NAI-101 / NAI-117 / NAI-122 / NAI-127 / NAI-130 / NAI-131 / NAI-132 / NAI-133 / NAI-134 tests.
- Doc-comment cross-references to `Player.ts:655-712` present in `defaultMoveSpeed`, `resolveMovement` (bridge + idle blocks), `updateEnergy`, `processEnergy`.
- No DEVIATION-NAI-135-* tags introduced. NAI-135 retires two pre-existing unflagged divergences (per-step drain, per-step energy gate).

## 11. Carry-forward routing

NAI-136+ candidates (unchanged from NAI-134 close):
- **NAI-127 carryover:** range-across-fence (line-of-walk / projectile rules). Investigation cadence.
- **NAI-127 carryover:** arrows-not-consumed (missile lifecycle / inv-decrement). Investigation cadence.
- **NAI-115 P1:** firemaking ashes-no-drop after fire despawn (investigation).
- **NAI-115 P2:** LOWMEM byte-alignment trace (investigation).
- **NAI-119 carryover:** weapon-equip rendering (multi-file ≥100 LOC).
- **NAI-111:** P_TELEJUMP `[label,tutorial_complete]` (investigation).

Parked deviations: NAI-115-D1, NAI-115-D2, NAI-119-D-NO-DOWNSTREAM-LOC2-CONSUMERS, NAI-120 stale-comment polish, NAI-121-D4-debugname, NAI-122-paramtype-boundary, NAI-123-D1, NAI-127-D1/D2, NAI-130-D1..D4, NAI-134-D1.
