# NAI-175 — NPC stepOnce collision-strategy plumbing (duck-wander symptom)

**Tech Stack:** Go 1.26+

**Cadence:** investigation sub-spec (Stage 1 audit → Stage 2 fix → smoke handoff → conditional Stage 3 probe).

**TS source:** `LostCityRS/Engine-TS` (canonical) — `src/engine/entity/PathingEntity.ts` `takeStep` (lines 617–683) and `validateAndAdvanceStep` (lines 202–232). `src/engine/entity/Npc.ts` `blockWalkFlag` (381–398) and `getCollisionStrategy` (inherited from PathingEntity:558–575). Already ported on the goscape side — see `modules/world/npc.go:245-289`.

## 1. Symptom

Adult ducks (`[duck]` typeId=44, `[duck_female]` typeId=45) at Lumbridge river don't move. User-supplied server logs:

```
... msg=nai128.npc.enqueue npc=2888412 typeId=44 trigger=121 delay=0 lastInt=0 queueLen=1
... msg=nai128.npc.queuefire npc=2888412 typeId=44 trigger=121 sf=[ai_queue5,_duck] lastInt=0
... msg=nai128.npc.enqueue npc=2951288 typeId=45 ...
... msg=nai128.npc.queuefire npc=2951288 typeId=45 ... sf=[ai_queue5,_duck]
```

The log lines are **unrelated** to the stuck-movement symptom. `[ai_timer,_duck]` re-queues itself every 50–99 ticks; `[ai_queue5,_duck]` only `npc_say("Quack!")` + `~sound_area(quack, 0, npc_coord, 5)` (`Content/scripts/general/scripts/flavour_text/ducks.rs2:5-7,35-37`). They prove the AI loop runs, nothing more.

Engine-driven wandering for `[duck]` / `[duck_female]` (config: `wanderrange=35`, `moverestrict=blocked`, no `defaultmode` line → defaults to `wander` via `(*Npc).defaultMode` at `modules/world/npc_interaction.go:955-966`) is what should move ducks. It doesn't.

## 2. Bundle 0 root cause (controller pre-flight)

`(*Npc).stepOnce` at `modules/world/npc_interaction.go:344-370`:

```go
if s != nil && s.gamemap != nil && !s.gamemap.CanTravel(n.level, n.x, n.z, dx, dz) {
    n.waypointIndex = -1
    return false, -1
}
```

`(*GameMap).CanTravel` at `pkg/gamemap/gamemap.go:136-140`:

```go
func (gm *GameMap) CanTravel(level, x, z, offsetX, offsetZ int) bool {
    return gm.Pathfinder.StepValidator.CanTravel(
        level, x, z, offsetX, offsetZ, 1, 0, collision.TypeNormal,
    )
}
```

**Hardcoded** `(size=1, extraFlag=0, TypeNormal)`. The NPC's `getCollisionStrategy()` (returns `TypeBlocked` for `MoveRestrictBlocked`) and `blockWalkFlag()` (returns `FlagOpen` for blocked) — `modules/world/npc.go:245-289` — are **never plumbed through**.

**Why this stops ducks.** Per-direction masks like `FlagBlockNorth` resolve via `FlagBlockNorth = FlagWallSouth | FlagWalkBlocked` where `FlagWalkBlocked = FlagLoc | FlagFloorBlocked` and `FlagFloorBlocked = FlagBlockWalk | FlagGroundDecor` (`pkg/pathfinder/collision/flag.go:59,61,67-68`). So `FlagBlockNorth` includes `FlagBlockWalk`.

Under `TypeNormal` (`pkg/pathfinder/collision/strategies.go:25-26`): `tileFlag & blockFlag == 0`. A water tile carries `FlagBlockWalk`, so the AND is non-zero → step rejected.

Under `TypeBlocked` (`strategies.go:27-29`): strips `FlagBlockWalk` from the mask, then requires `FlagBlockWalk` *set* on the destination — i.e. ducks step *only* on water. This is the TS predicate.

Then `stepOnce` sets `n.waypointIndex = -1` (path abandoned). `wanderMode` rolls 1/8 ticks (`modules/world/npc_interaction.go:85`); every rolled waypoint hits the same wall.

**Prior knowledge.** The same hardcoding was flagged 5 days ago in `nai_followups.md` Sub-H7 under NAI-98:
> "`pkg/gamemap/gamemap.go:97-101` hardcodes srcSize=1, extraFlag=0; BFS uses per-direction `clipFlag` from `routefinder.go:187, 197`."

NAI-98 noted it as a *sub-hypothesis for a different cascade*; the per-step plumbing was never the target of a fix. NAI-175 closes that thread.

## 3. Secondary divergences from TS `PathingEntity.takeStep`

Enumerated for Stage 1 to rate; not all must ship in Stage 2.

- **D1 — axis-fallback.** TS (`PathingEntity.ts:668-682`): if diagonal blocked, try X-only `dx, 0` then Z-only `0, dz`. goscape `stepOnce` returns blocked after the first failed check. Not strictly required to unstick ducks (any TypeBlocked-passing direction works), but TS-fidelity gate.
- **D2 — blocked-step waypoint semantics.** TS `validateAndAdvanceStep:202-213` returns `-1` *without* mutating `waypointIndex` when `takeStep` returns null (stuck). goscape sets `n.waypointIndex = -1` (path abandoned). Affects how aggressively wandering NPCs retry their current goal.
- **D3 — size>1 branch.** TS `takeStep:642-651` has a separate `this.width > 1` arm. goscape `stepOnce` always single-tile. Ducks are size 1; not duck-binding, but a real gap for large NPCs.
- **D4 — Player parallel.** `(*Player).stepOnce` (`modules/world/movement.go:120-154`) has the same `gamemap.CanTravel` hardcoding. Players are `MoveRestrictNormal` so `TypeNormal` is correct, **but** `extraFlag` should be `FlagBlockNPCs` per `blockWalkFlag` parity — currently 0, so players walk *through* tiles occupied by NPCs (latent bug, not duck-binding).

## 4. Approach

Three options considered:

**Option A — change `(*GameMap).CanTravel` signature.** Add `(size, extraFlag int, collisionType collision.Type)` params; update all callers. ~5 callers, all in `modules/world`. Minimal indirection; matches TS where `canTravel` is also fully parameterised. **Recommended.**

**Option B — keep `CanTravel`, add `CanTravelStrategy`.** Two wrappers. More API surface, but the existing wrapper stays usable for non-NPC contexts. Rejected: there are no real non-NPC contexts; the wrapper exists *because* stepOnce and friends are the only callers, and they all need the strategy.

**Option C — bypass the wrapper; call `Pathfinder.StepValidator.CanTravel` directly from stepOnce.** Smallest diff, but accumulates a stepOnce → routefinder coupling that other call sites don't share. Rejected.

**Picked: Option A.**

### 4.1 In-scope (Stage 2)

1. Change `(*GameMap).CanTravel` signature to `(level, x, z, offsetX, offsetZ, size, extraFlag int, collisionType collision.Type)`. Existing test callers update to pass `(1, 0, collision.TypeNormal)` for backward parity. (~5 callers; grep to enumerate at plan time.)
2. Port `(*Npc).stepOnce` to TS `takeStep` parity:
   - Pull `cs := n.getCollisionStrategy()`; if `cs == nil`, treat as TS "-1 = nomove returns -1" → return `(false, -1)` and `waypointIndex--` like TS.
   - Pull `extraFlag := n.blockWalkFlag()`; if `extraFlag == collision.FlagNull`, same handling.
   - Pass `n.Width()` for size (1 for ducks; ≥1 for size>1 NPCs).
   - **Axis-fallback (D1):** on direct-direction block, try X-only then Z-only; return null-equivalent only if all three fail.
   - **Blocked-step semantics (D2):** when all three fail, return `(false, -1)` *without* setting `waypointIndex = -1` — leave path intact for next-tick retry.
   - **Size>1 branch (D3):** mirror TS — at width>1, use `Face(srcX, 0, x, 0)` / `Face(0, srcZ, 0, z)` separately, with the same fallback shape.
3. **Player.stepOnce (D4) — DECISION GATE AT STAGE 1.** If Stage 1 audit concludes Option A's signature change forces the Player call site to update anyway, port `(*Player).stepOnce` to the same shape (pulling player's `blockWalkFlag` / `getCollisionStrategy`). Otherwise, carve out a `NAI-175-D-PLAYER-STEP-COLLISION` deviation tag and defer Player parity to NAI-176.

### 4.2 Out of scope

- Hunt RNG seam, `npc_findall` script-side, `[ai_queue5,_duck]` script behaviour — all confirmed not duck-binding.
- `Player.takeStep` axis-fallback if Stage 1 says Player port can be deferred (still tracked as D4).
- Pathfinder BFS strategy — already handled per-direction (`pkg/pathfinder/routefinder/routefinder.go:187,197` per `nai_followups.md` Sub-H7).

## 5. Architecture / data flow

Unchanged at the system level. The dataflow shape is:

```
processMovementInteraction (npc_interaction.go:161)
  → wanderMode (:81)         // 1/8 RNG: QueueWaypoint(startX±rand, startZ±rand)
  → updateMovement (:280)
    → stepOnce (:344)        // <— THIS FUNCTION; calls (*GameMap).CanTravel
      → GameMap.CanTravel    // <— THIS WRAPPER; now strategy-parameterised
        → StepValidator.CanTravel  // already strategy-aware (no change)
          → collision.CanMove        // already strategy-aware (no change)
```

Player path symmetric via `movement.go:120 (*Player).stepOnce`.

## 6. Tests

### 6.1 Stage 2 — new tests

`modules/world/npc_interaction_test.go` (add):

- `TestNpcStepOnce_BlockedNpcStepsOntoWaterTile` — flagmap with two adjacent `FlagBlockWalk` tiles at (3221, 3220) and (3222, 3220), one NPC of `MoveRestrictBlocked` at (3221, 3220) with waypoint at (3222, 3220), `stepOnce` returns `(true, East)` and NPC at (3222, 3220). Inverse: a `MoveRestrictNormal` NPC at the same coord with the same waypoint gets `(false, -1)`.
- `TestNpcStepOnce_NoMoveReturnsMinusOne` — `MoveRestrictNoMove` NPC with a valid waypoint returns `(false, -1)`; waypointIndex unchanged.
- `TestNpcStepOnce_BlockedTransientLeavesWaypointIntact` (D2 fixture) — NPC at (x,z), wall on all four sides via flagmap, `stepOnce` returns `(false, -1)` AND `n.waypointIndex` is unchanged (still 0, dest still in waypoints[0]).
- `TestNpcStepOnce_AxisFallback_X` (D1) — diagonal blocked, X-only open: returns `(true, East)` toward dest. Variant `_Z` for Z-only.
- `TestNpcStepOnce_Size2Width` (D3) — size=2 NPC with the size>1 branch covered; minimal flagmap that distinguishes direct vs single-axis.

`pkg/gamemap/gamemap_test.go` — confirm new signature works with all (extraFlag, collisionType) combinations for size=1 and size=2. Inverse-table fixture per the TS strategies (Normal vs Blocked vs Indoors vs Outdoors).

### 6.2 Stage 1 — no new tests; verdict-only audit

### 6.3 Existing tests to update

- All `gamemap.CanTravel` callers: pass `(1, 0, collision.TypeNormal)` to preserve behaviour where the test doesn't intend to exercise strategy. Plan enumerates.
- `npc_test.go` `TestNpc_GetCollisionStrategy_PerMoveRestrict`, `TestNpc_BlockWalkFlag_PerMoveRestrict` — unchanged.

### 6.4 Smoke gate

User launches the server with the new binary, runs Lumbridge in the Java client. Stand near the river south of the castle for ~30s. Pass = adult ducks visibly drift between water tiles. Fail = still stationary (proceed to Stage 3).

## 7. Stage 3 (conditional, on smoke failure)

Inline `slog.Info` probe in `(*Npc).stepOnce` reporting `(typeId, x, z, dx, dz, extraFlag, cs, canTravel_result, reason)` where `reason` ∈ {`face_minus_one`, `cs_nil`, `extraFlag_null`, `direct_blocked`, `axis_x_blocked`, `axis_z_blocked`, `all_blocked`, `stepped`}. Tee+grep on a 60-second smoke window. Gate the probe behind the existing NodeDebug pattern (see `nodedebug_gateway_probe_pattern.md` in memory) so it lands as a permanent diagnostic, not a one-off.

## 8. Risks

- **R1 — Player parity scope creep.** If the audit pushes Player.stepOnce into scope, Stage 2 grows. Mitigation: explicit decision gate at end of Stage 1; defer Player to NAI-176 if duck-symptom-orthogonal. R1 priority: medium.
- **R2 — Test lock-in.** Existing tests may assume `gamemap.CanTravel(Normal/0/1)` semantics for NPCs that aren't `MoveRestrictNormal`. Stage 1 grep for `CanTravel\(` + `WanderRange` + `MoveRestrict` to enumerate. R2 priority: low; most NPC tests use `WanderRange: 0` (stationary).
- **R3 — Axis-fallback divergence triples step volume.** D1 means up to 3 `CanTravel` calls per step. Negligible (FlagMap lookup is O(1) array index), but worth a perf-smell mental note. R3 priority: trivial.
- **R4 — NAI-92 plan stale reference.** `docs/superpowers/plans/2026-05-04-nai-92-smart-pathfinding-port.md:545,558` enumerates the same strategy mapping. NAI-92 already ported pathfinder-side; the per-step wrapper was the orphan. Mitigation: no plan-doc cross-reference needed; cite from `npc.go:245-289` (production source of truth).

## 9. Deviations from TS

None *expected* if the full takeStep parity (D1+D2+D3) ships in Stage 2. If Player.stepOnce port defers, open `NAI-175-D-PLAYER-STEP-COLLISION` with rationale "Player MoveRestrictNormal path unaffected by duck symptom; FlagBlockNPCs latent for normal walking; defer to NAI-176". Otherwise no deviation tags expected.

## 10. Acceptance

- All Stage 2 new tests green.
- `go test ./...` green at HEAD with the strategy-parameterised wrapper.
- Smoke: adult ducks at Lumbridge visibly move within 30s of standing nearby.
- Memory entry added recording Sub-H7 retirement and any Stage 1 surprises.
