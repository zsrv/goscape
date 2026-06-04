# NAI-101 — `queueWaypoints` missing TS-required input-reversal (path-around residual from NAI-98/100)

**Investigation+fix sub-spec.** Bundle 0 controller pre-flight done in this session; Stage 1 audit short-circuited (Bundle 0 produced binding TS-source diff verdict); Stage 2 fix; smoke-binding.

**Predecessor:** `nai_100_path_around_residual.md` (memory) + smoke 2026-05-05 (NAI-100 close).

**Tech Stack:** Go 1.26+; goscape engine; `LostCityRS/Engine-TS` canonical reference.

---

## 1. Problem

After NAI-100 correctly flagged the 4-tile Lumbridge fountain footprint with `FlagLoc` (verified by Bundle 0 probe), the user smoke on 2026-05-05 showed the player at `(3222, 3225)` stuck unable to walk around the fountain to reach the NPC at `(3219, 3230)`. The residual memory `nai_100_path_around_residual.md` enumerated three suspect hypotheses (FindPathPlain `moveNear` truncation, BFS expand limits, `TypeNormal`-vs-`FlagLoc` handling), all upstream of the BFS itself.

Bundle 0 probe at HEAD `a45c123` (post-NAI-100) **falsifies all three suspects** and surfaces the actual root cause downstream of the pathfinder.

## 2. Bundle 0 verdict — root cause identified

**Probe** (one-shot real-cache test, removed after capture):

```
FindPathToEntity(0, 3222,3225 → 3219,3230, 1,1,1):
  Success=true  Alternative=false  len(Waypoints)=3
    [0] (3220, 3225, 0)
    [1] (3220, 3228, 0)
    [2] (3219, 3229, 0)

FindPathPlain(0, 3222,3225 → 3219,3230):
  Success=true  Alternative=false  len(Waypoints)=3
    [0] (3220, 3225, 0)
    [1] (3220, 3229, 0)
    [2] (3219, 3230, 0)

Fountain flags (post-NAI-100): (3221,3226), (3221,3227), (3222,3226), (3222,3227) all = 0x100 (FlagLoc).
All other tiles in 3220-3223 × 3225-3228 = FlagOpen.
```

The pathfinder produces correct 3-waypoint detour routes. The bug is in the consumer.

**TS-source diff** (`Engine-TS/src/engine/entity/PathingEntity.ts:248-254`):

```ts
queueWaypoints(waypoints: ArrayLike<number>): void {
    let index: number = -1;
    for (let input: number = waypoints.length - 1, output: number = 0; input >= 0 && output < this.waypoints.length; input--, output++) {
        this.waypoints[output] = waypoints[input];
        index++;
    }
    this.waypointIndex = index;
}
```

TS **reverses** the input on copy — `waypoints[0] = input[length-1]`, `waypoints[length-1] = input[0]`. With input arriving in `[first_step, …, dest]` order (BFS-natural and protocol-natural per `MoveClickHandler.ts`), TS internal storage becomes `[dest, …, first_step]`. `waypointIndex = length-1` reads `waypoints[length-1] = first_step` first; `stepOnce` decrements toward `0` (= `dest`).

**goscape `(*Player).queueWaypoints`** at `modules/world/movement.go:14-29`:

```go
// queueWaypoints replaces the current path with the given packed coords.
// waypoints[0] is the final destination; the last element is the first step.
func (p *Player) queueWaypoints(packed []int) {
    if len(packed) == 0 {
        p.waypointIndex = -1
        return
    }
    n := len(packed)
    if n > len(p.waypoints) {
        n = len(p.waypoints)
    }
    for i := 0; i < n; i++ {
        p.waypoints[i] = packed[i]    // NATURAL ORDER — REVERSE MISSING
    }
    p.waypointIndex = n - 1
}
```

The doc-comment at lines 14-15 describes the **TS-faithful target contract** (`waypoints[0]` = destination), but the loop body stores in **natural order** (`waypoints[0] = packed[0] = first_step`). Comment ⊥ code inconsistent.

`(*Npc).queueWaypoints` at `modules/world/npc_ai.go:90-107` has the identical bug.

**Resulting symptom:** `stepOnce` reads `waypoints[waypointIndex=n-1] = dest` and uses `coordgrid.Face(player, dest)` to choose a single-tile direction toward the destination — heading **straight at it**, ignoring all intermediate direction-change points the BFS produced. When the direct route hits a blocker (the fountain's NW corner from the player's start tile), `gamemap.CanTravel` returns false, `waypointIndex = -1`, the path is lost.

**Why single-waypoint paths look fine pre-fix:** with `n=1`, `waypoints[0]` is unambiguously both first-step and destination; iteration produces correct `Face` direction.

**Why detour paths break pre-fix:** the BFS's `[first_step, mid, dest]` shape is exactly the case where reading the LAST element as the "next step" diverges from reading the FIRST element.

## 3. Residual diagnostic ambiguity

The smoke evidence (`waypoint_idx=2 stable`, `steps_taken=0` for 22 ticks, `repathed=false`) does **not** perfectly match the reverse-bug-only theory: under that theory, `waypointIndex` would clamp to `-1` after one blocked step, not stay at `2`. Possible co-factors:

- Client/server/gamemap nil-guard in `stepOnce` skipping `CanTravel` (defensive code at `movement.go:94-99`); player may be silently stepping into FlagLoc tiles without server-side validation.
- An upstream re-queue site (not yet identified) restoring `waypointIndex=2` each tick despite `repathed=false` indicator.
- Smoke binary built before NAI-100 close commit `a45c123` (timing not verified).

The reverse-bug is **real per direct TS source comparison and unit semantics regardless of smoke-detail provenance**. Smoke is the binding test for "did the fix unblock the symptom" — see §8.

## 4. PRIMARY / SECONDARY commitments

Per `dispatch_correct_reach_blocked` cadence:

- **PRIMARY (TS-faithful port):** `(*Player).queueWaypoints` and `(*Npc).queueWaypoints` reverse-copy input on store, mirroring `PathingEntity.ts:248-254`. Closes on unit + integration test green + plan + reviews + smoke-binding.
- **SECONDARY (smoke target):** player walks W around the Lumbridge fountain footprint and reaches NPC at `(3219, 3230)`. Bound at smoke; if any residual surfaces, route per the §8 decision tree.

## 5. Approach

TS-faithful reverse-copy in `queueWaypoints` (Q3 option A, user-approved). Single edit at the storage-orientation boundary inside both `queueWaypoints` functions; no other layer changes.

Rejected alternatives:

- **Caller-side reverse in `routeToPacked`** — under-fixes: leaves client-supplied paths (`NodeClientRoutefinder=true` + multi-element packet) heading-direct-to-dest. `NodeClientRoutefinder` is config-flippable.
- **Both layers (belt-and-suspenders)** — over-engineered; TS does it once at the storage boundary.

## 6. Data-flow contract (post-fix)

| Layer                                                 | Order                                              |
| ----------------------------------------------------- | -------------------------------------------------- |
| `routefinder.RouteFinder.FindRoute` waypoints         | `[first_step, …, dest]` (src→dst; BFS-natural)     |
| `routeToPacked` output                                | `[first_step, …, dest]` (preserves)                |
| Client `MoveClick.path` (per `MoveClickHandler.ts`)   | `[first_step, …, dest]`                            |
| Input to `queueWaypoints`                             | `[first_step, …, dest]`                            |
| **Reverse-copy boundary (NAI-101 fix)**               | flips orientation                                  |
| Internal `p.waypoints[0..n-1]` storage                | `[dest, …, first_step]`                            |
| `waypointIndex` initial value                         | `n-1` → `waypoints[n-1] = first_step`              |
| `stepOnce` iteration                                  | decrements toward `0` (= `dest`)                   |

## 7. Changes

### 7.1 Production

**`modules/world/movement.go`** — replace the natural-order copy in `(*Player).queueWaypoints` (lines 16-29) with a TS-faithful reverse-copy. Keep the doc-comment's existing storage-contract sentence; add a citation to TS PathingEntity.ts:248-254.

```go
// queueWaypoints replaces the current path with the given packed coords.
// Mirrors TS PathingEntity.queueWaypoints (Engine-TS PathingEntity.ts:248-254):
// reverses the input on copy so that storage is [dest, …, first_step].
// stepOnce reads waypoints[waypointIndex] starting at n-1 (= first_step) and
// decrements toward 0 (= dest). Truncation drops far-from-dest entries when
// input exceeds the waypoint buffer (TS-faithful: TS truncates the same way
// via output < this.waypoints.length).
func (p *Player) queueWaypoints(packed []int) {
    if len(packed) == 0 {
        p.waypointIndex = -1
        return
    }
    index := -1
    for input, output := len(packed)-1, 0; input >= 0 && output < len(p.waypoints); input, output = input-1, output+1 {
        p.waypoints[output] = packed[input]
        index++
    }
    p.waypointIndex = index
}
```

**`modules/world/npc_ai.go`** — same change in `(*Npc).queueWaypoints` (lines 90-107). Same doc-comment shape; cross-reference `(*Player).queueWaypoints`.

### 7.2 Tests

**`modules/world/movement_test.go`** (new tests; existing tests untouched — verified zero collateral in §9):

- `TestQueueWaypointsReversesInputOrder` (red-first): queue 3-element packed `[A, B, C]`; assert `waypoints[0]==C && waypoints[1]==B && waypoints[2]==A` and `waypointIndex==2`. Pins the TS-faithful contract.
- `TestQueueWaypointsTruncatesFarEntries`: queue 30-element packed where each entry is uniquely identifiable (e.g., `packTestCoord(0, 3000+i, 3000)`); assert `waypoints[0..24]` correspond to **`packed[5..29]` reversed** (closest-to-dest entries preserved, far-from-dest dropped). Pins the truncation direction matches TS.
- `TestStepOnceFollowsDirectionChangePoints` (regression-pin for the bug): synthetic 3-waypoint scenario `[first_step=(3094, 3107), mid=(3094, 3110), dest=(3097, 3110)]` from player at `(3094, 3106)`, where direct `Face(player, dest)` would head NE but the routed path requires N then E. Tick `resolveMovement` 6 times; assert player ends at `dest` having traversed `mid`. Pre-fix: player heads NE on tick 1 from `(3094, 3106)` to `(3095, 3107)`, then iterates wrong / blocks; FAILS. Post-fix: player heads N to `(3094, 3107)` (= `first_step`), continues N to `mid`, then E to `dest`; PASSES.

**`modules/world/npc_ai_test.go`** (or appropriate `_test.go`): symmetric `TestNpc_QueueWaypointsReversesInputOrder` + `TestNpc_StepOnceFollowsDirectionChangePoints`.

**`modules/world/static_loc_collision_test.go`** (extend) OR new `modules/world/nai101_fountain_test.go` — full-stack real-cache regression test:

- Skip-if-absent for `data/pack/server/maps/m48_50` and `data/pack/server/loc.dat`.
- Setup: `gamemap.Init(cacheDir)` + `LoadLocTypes` + `populateStaticLocsIntoZones`.
- Pin fountain footprint flags `(3221..3222, 3226..3227) = FlagLoc` (sanity check NAI-100 still holds).
- Construct a `*Player` at `(3222, 3225)` via `newTestPlayer`; plug `s` into `p.client.server`.
- Set `p.target` = mock `*Npc` at `(3219, 3230)` (or call `pathToTarget` through dispatch).
- Tick `resolveMovement` up to 12 times or until `p.waypointIndex < 0`.
- Assert: `p.x == 3219 && p.z == 3229` (entity-reach adjacent S of NPC), `p.stepsTaken > 0`, total ticks `≤ 10`.
- Pre-fix would FAIL (player ends at start or near it after tripping CanTravel); post-fix PASSES.

## 8. Smoke + decision tree

**Smoke (out-of-band, user-launched per `smoke_test_server_handoff`):** user runs server with the post-fix binary; walks NW from Lumbridge spawn `(3221, 3218)` past `(3222, 3225)`; clicks NPC at `(3219, 3230)`. Watches for path-around behavior.

**Decision tree:**

- **Symptom resolved (player walks W around fountain, reaches NPC):** PRIMARY + SECONDARY both close. Bundle 3 not needed.
- **Symptom-shape changes (player walks some distance W but stalls before reaching NPC):** PRIMARY closes (reverse-fix is wired and observable in symptom-shape change); SECONDARY routes to NAI-102 with the new shape evidence. Per `smoke_unchanged_means_multiple_blockers`: this means a downstream second-blocker.
- **Symptom-shape unchanged (player still stuck at `(3222, 3225)` `waypoint_idx=2 stable`):** PRIMARY blocked; reverse-fix isn't taking effect at runtime. Bundle 3 investigates: was the binary built with the fix? Is there a re-queue site overwriting? Is `client/server/gamemap` nil at this player such that the nil-guard skips `CanTravel`?
- **Different symptom (e.g., player walks past target, oscillates, or pathfinder regression elsewhere):** unit `stepOnce` iteration semantics regressed under reversed storage. Bundle 3 re-audits `stepOnce`.

## 9. Audit — collateral check (controller pre-flight)

**Direct readers of `p.waypoints[*]` / `n.waypoints[*]` in production** (`grep -n "\.waypoints\[" --include="*.go" | grep -v _test.go`):

- `movement.go:10`  `p.waypoints[0] = …` — `queueWaypoint` (single, both orders equivalent for n=1) ✓
- `movement.go:26`  `p.waypoints[i] = packed[i]` — **THE BUG** ⇐ FIX
- `movement.go:85`  `dest := UnpackCoord(p.waypoints[p.waypointIndex])` — `stepOnce`. Becomes correct after `queueWaypoints` reverses (iteration was already correct under the post-fix contract).
- `npc_ai.go:86`    `n.waypoints[0] = …` — `QueueWaypoint` single ✓
- `npc_ai.go:104`   `n.waypoints[i] = packed[i]` — **THE BUG** ⇐ FIX
- `npc_interaction.go:348` `dest := UnpackCoord(n.waypoints[n.waypointIndex])` — Npc `stepOnce`. Becomes correct.

No other production reader. Audit clean.

**Existing tests** (`grep -rn "waypoints\[[0-9]\] *==\|waypoints\[[0-9]\] *!=" --include="*_test.go"`):

- `movement_test.go:14` — `if p.waypoints[0] == 0` for single-element packed; n=1 → reverse is identity. ✓
- `movement_test.go:111` — `p.waypoints[0] != packed[0]` for single-element packed; n=1 → identity. ✓
- `interaction_trigger_nai68_test.go:323,429` — sentinel-preservation tests; values planted directly into `waypoints[2]` without going through `queueWaypoints`; preserved across interaction state-clears. Order-orthogonal. ✓
- `TestQueueWaypointsReplacesExisting` (movement_test.go:19-35) — asserts `waypointIndex` value only after queueing. Unchanged. ✓

Zero existing-test collateral.

## 10. Plan cadence

Per `investigation_subspec_cadence`:

- **Bundle 0 (done in this session):** controller pre-flight probe; conclusive root-cause identified at the line-of-code level via TS-source diff + real-cache pathfinder probe.
- **Bundle 1 (Stage 1 audit):** **SKIP.** Bundle 0's verdict is binding; no audit subagent needed (would only re-verify what the TS diff already shows). NAI-101 compresses vs. NAI-31's full Stage-1 audit because the diagnosis is line-level certain.
- **Bundle 2 (Stage 2 fix):** TDD red→green per task:
  - T1: Player `queueWaypoints` reverse-copy + unit tests (red-first).
  - T2: Npc `queueWaypoints` reverse-copy + symmetric unit tests.
  - T3: integration full-stack fountain test (real-cache).
  - T4: rollup close commit.
- **Smoke handoff:** out-of-band, user-launched.
- **Bundle 3 (conditional):** materialized only on smoke failure per §8 decision tree.

## 11. Out of scope

- The `NodeClientRoutefinder=false` config branch (handled by single-coord `userPath`, not affected by this bug).
- `pkg/pathfinder/routefinder` BFS internals (Bundle 0 confirmed correct).
- Any refactor to consolidate Player+Npc duplicated `queueWaypoints` logic (separate concern; risk-register R2 in `interaction.go` already documents the asymmetric-server-access reason for the duplication).
- The `gameMapBlockMapSquare = 0x2 vs TS BLOCK_MAP_SQUARE = 0x1` divergence noted at `static_loc_collision_test.go:65-71` (NAI-96+ followup).

## 12. Risks

- **R1 (low):** TS-truncation direction confirmed (closest-to-dest preserved). Pinned by `TestQueueWaypointsTruncatesFarEntries`.
- **R2 (low):** A future refactor adds a `waypoints[i]` reader assuming natural order. Mitigated by the doc-comment citation to `PathingEntity.ts:248-254`.
- **R3 (residual):** smoke `waypoint_idx=2 stable` not fully explained by reverse-bug alone. If post-fix smoke shows a similar stall, Bundle 3 investigates the auxiliary co-factor.
- **R4 (low):** the integration test depends on `data/pack/server/maps/m48_50` cache presence; CI without that data falls through `t.Skip`. Acceptable per existing pattern (`TestNAI95_StaticLocCollision_HansArea`).

## 13. Memory reads

- `nai_100_path_around_residual.md` — residual seed.
- `pathfinder_api_loc_aware.md` — FindPathPlain / FindPathToEntity / FindPathToLoc dispatch.
- `smoke_unchanged_means_multiple_blockers.md` — multi-blocker smoke pattern.
- `dispatch_correct_reach_blocked.md` — PRIMARY/SECONDARY split at smoke close.
- `cascade_theory_smoke_binding.md` — residuals route forward when cascade smokes refuted.
- `investigation_subspec_cadence.md` — Bundle 0/1/2/3 cadence.
- `controller_preflight.md` — Bundle 0 grep+read discipline.
- `audit_subagent_fabrication.md` — Bundle 0 probe is independent verification.
- `true_to_ts_gate.md` — TS-faithful is the project hard requirement.
- `superpowers_clear_between_spec_and_impl.md` — `/clear` boundary after spec → before plan.
