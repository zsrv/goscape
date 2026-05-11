# NAI-167 — NPC `resetPathingEntity` parity (NAI-157-FU bundle)

> Combined spec + plan per `compressed_cadence.md` (≤100 LOC band; ~25 production LOC, ~50 test LOC).

## 1. Scope

Complete the NAI-157 pattern by porting the remaining `PathingEntity.resetPathingEntity()` fields TS resets at end-of-tick but goscape currently misses (or resets in the wrong place, behind an early-return).

Bundles the two queued follow-ups from `nai_followups.md` "From NAI-157":

- **NAI-157-FU-PATHING-ENTITY-FULL-RESET** — `n.tele`, `n.lastTickX/Z/lastLevel` reset.
- **NAI-157-FU-NPC-STEPSTAKEN-RESET** — `n.stepsTaken` reset.

## 2. TS source

`Engine-TS/src/engine/entity/PathingEntity.ts:577-587` (relevant subset):

```ts
protected resetPathingEntity(): void {
    this.moveSpeed = this.defaultMoveSpeed();
    this.walkDir = -1;
    this.runDir = -1;
    this.jump = false;
    this.tele = false;
    this.lastTickX = this.x;
    this.lastTickZ = this.z;
    this.lastLevel = this.level;
    this.stepsTaken = 0;
    this.interacted = false;
    this.apRangeCalled = false;
    // ...remaining fields not in port scope (most don't exist on goscape Npc; see §6)
}
```

Called from `Entity.resetEntity(refresh)` → invoked per-NPC in `World.processCleanup` (TS `World.ts:1152`).

## 3. Current goscape state at HEAD `5471a99`

- `n.walkDir = -1; n.runDir = -1` — ALREADY in `(*Npc).ResetMasks()` (NAI-157, `npc_masks.go:224-225`).
- `n.lastTickX, n.lastTickZ, n.lastLevel = n.x, n.z, n.level` — at `processMovementInteraction:163`, **behind** `n.delayed || n.dead` early-return at `:159-161`. Delayed/dead NPCs miss the write.
- `n.tele = false` — at `processMovementInteraction:164`, same early-return hazard.
- `n.stepsTaken` — incremented at `:363`, never reset (only `NewNpc` initializes to zero). Forever-broken `n.targetX != -1 && n.stepsTaken == 0` reorient-gate at `:932`.
- `n.jump`, `n.interacted`, `n.apRangeCalled` — not yet in goscape's NPC field set; out of scope (see §6).

## 4. Approach (Approach B per brainstorm)

Extract a new `(*Npc).resetPathingEntity()` method mirroring TS structure. The method owns the **PathingEntity-level** end-of-tick reset; `ResetMasks` continues to own goscape-specific mask payload + faceEntity trailing-clear.

### File map

- **Modify** `modules/world/npc_masks.go`:
  - Delete `n.walkDir = -1` / `n.runDir = -1` lines from `ResetMasks` (lines 224-225). Update the surrounding NAI-157 comment to point to `resetPathingEntity` instead.
  - Add new `(*Npc).resetPathingEntity()` method below `ResetMasks` body. Resets: `walkDir`, `runDir`, `tele`, `lastTickX`, `lastTickZ`, `lastLevel`, `stepsTaken`. Doc-comment cites TS PathingEntity.ts:577-587 + lists the four fields not yet ported (jump, interacted, apRangeCalled, moveSpeed — see §6).
- **Modify** `modules/world/npc_interaction.go`:
  - Delete lines 163-164 (`n.lastTickX, n.lastTickZ, n.lastLevel = n.x, n.z, n.level` + `n.tele = false`). These now live in the end-of-tick reset.
  - Update surrounding comment (lines 150-157 dispatch-order list) to drop the "Last-tick coord bookkeeping + tele flag reset" bullet.
- **Modify** `modules/world/tick.go`:
  - Inside the NPC loop at `processCleanup` (line 654-656), call `n.resetPathingEntity()` immediately before (or after) `n.ResetMasks()`. Order: `resetPathingEntity()` first (mirrors TS, which calls `resetPathingEntity` from inside `resetEntity` before mask-payload work), then `ResetMasks()` for goscape mask-specific cleanup.

### Test pins

All in **`modules/world/npc_masks_test.go`** (existing NAI-157 file; pattern at lines 283-330):

1. **`TestNpc_ResetPathingEntity_AdvancesLastTickCoords`** — set `n.x/z/level = 5/6/0`; preset `n.lastTickX/Z/lastLevel = -1/-1/-1`; call `resetPathingEntity`; verify lastTick fields now `5/6/0`.
2. **`TestNpc_ResetPathingEntity_ClearsTele`** — set `n.tele = true`; call; verify `false`.
3. **`TestNpc_ResetPathingEntity_ResetsStepsTaken`** — set `n.stepsTaken = 5`; call; verify `0`.
4. **`TestNpc_ResetPathingEntity_ResetsWalkRunDir`** — set `n.walkDir = 4; n.runDir = 7`; call; verify both `-1`. (Migration of the existing NAI-157 pin away from ResetMasks; the original `TestNpcResetMasksClearsWalkDirRunDir` test is **inverted/relocated**: ResetMasks no longer clears these, resetPathingEntity does. Update or replace.)
5. **`TestNpc_DelayedNpc_GetsLastTickAdvancedAtCleanup`** — preset `n.delayed = true`, `n.x/z = 5/6`, `n.lastTickX/Z = -1/-1`. Simulate end-of-tick processCleanup call to `resetPathingEntity`. Verify `lastTickX/Z = 5/6` — the regression pin for the delayed-NPC reset gap. Mirrors the existing `TestNpcDelayedAfterStepClearsStaleWalkDir` shape from NAI-157.
6. **`TestNpc_StepsTakenResetEnablesReorientGate_AcrossTicks`** — covers the forever-broken reorient gate. Construct an NPC with `targetX = 3, targetZ = 4, target = nil, stepsTaken = 1` (post-tick-1 state). Call `resetPathingEntity` (end of tick 1). Call `n.reorient()` (start of tick 2). Verify `n.targetX/Z = -1/-1` (gate fired) and `n.dir`/face state updated by focus. Without the fix, stepsTaken stays at 1 → gate never fires.

Also update **`TestNpcResetMasksClearsWalkDirRunDir`** at line 288 — invert assertion (ResetMasks **no longer** touches walkDir/runDir; new home is resetPathingEntity). Either rename to `TestNpcResetMasksDoesNotTouchWalkDir` (preset to 4/7, verify still 4/7 after ResetMasks), or delete in favor of the new resetPathingEntity test #4. Recommend: delete + supersede (the assertion is now load-bearing on the new method).

Update **`TestNpcDelayedAfterStepClearsStaleWalkDir`** at line 312 — change the call from `n.ResetMasks()` to `n.resetPathingEntity()` (or both, since processCleanup calls both). The end-state assertion (`walkDir/runDir == -1` after end-of-tick cleanup) is unchanged.

## 5. TDD cadence

Single TDD bundle:

- **T1 (RED)** — author all six new test pins + the two updates. Run `go test ./modules/world/... -run 'TestNpc_ResetPathing|TestNpcResetMasksClearsWalkDir|TestNpcDelayedAfter'`; expect compile failure (`resetPathingEntity` undefined) → 6 failures after stubbing the method as a no-op.
- **T2 (GREEN)** — implement `resetPathingEntity`, delete the now-stale assignments at `npc_interaction.go:163-164` and `npc_masks.go:224-225`, wire into `tick.go:654-656`. Run focused tests → all green. Run full repo `go test ./...` → green.
- **T3 (VERIFY)** — `go vet ./...` clean. Confirm no new lint warnings.

Compressed cadence (≤100 LOC band): single end-of-impl Sonnet reviewer subagent OR controller-only review per `compressed_cadence.md`. Close commit cites this combined doc.

## 6. Out-of-scope / deferred

- **`moveSpeed = defaultMoveSpeed()`** at TS L578. goscape NPCs do not currently expose a `moveSpeed` field on `*Npc` (only `*Player` per NAI-135). When NPC move-speed plumbing lands (likely as part of `npc_walk_player`/`npc_walk_mode` future port), the call site for `defaultMoveSpeed()` on NPC will be added then.
- **`jump = false`** at TS L581. goscape `*Npc` has no `jump` field; teleport-jump is player-only at HEAD.
- **`interacted = false`** at TS L587 and **`apRangeCalled = false`** at TS L588. Used by TS's per-tick "did we already fire an AP-range trigger" gate; goscape's AP-range path uses a different snapshot mechanism. Open audit when AP-range / interaction-completion fidelity sub-spec surfaces.
- The remaining ~15 fields at TS L591-614 (`exactStartX/Z`, `exactEndX/Z`, `exactMoveStart/End/Facing`, `animId/Delay`, `sayMessage`, `hitmarkDamage/Type`, `spotanimId/Height/Time`, `faceSquareX/Z`) — some already reset in `ResetMasks` (sayText, damageAmt/Type, spotanimID/Height/Delay), some not yet a goscape field (exactMove*), some persistent goscape-side (animID per NAI-17 — explicitly retained across ticks).
- **faceEntity trailing-clear** at TS L611-614 — already in `ResetMasks` (NAI-157 commentary at `npc_masks.go:198-206`). Stays there; not duplicated into `resetPathingEntity`.

## 7. Risk register

- **R1**: Moving `n.lastTickX/Z = n.x/z` from tick-start (pre-step) to tick-end (post-step) shifts when the snapshot is captured. The `lastMovement` calc at `npc_interaction.go:333` (`if n.x != n.lastTickX || n.z != n.lastTickZ`) reads it *after* the step in the same tick. After the move: tick N starts with `lastTickX` = tick N-1's post-step x (from N-1's cleanup). Tick N step → x changes. The :333 diff is unchanged in semantics. **No regression expected; covered by existing `npc_interaction_test.go` lastMovement coverage if any, plus the new `TestNpc_DelayedNpc_GetsLastTickAdvancedAtCleanup` test.**
- **R2**: `n.stepsTaken = 0` at processCleanup means the field will be 0 at the start of tick N's processInfo/reorient pass for *every* NPC, after any tick where `processCleanup` has run. Currently the field accumulates forever; production AI code reads `stepsTaken == 0` only at `npc_interaction.go:932` (`reorient` gate). No other reads. **Behaviorally identical to TS; no other gate affected.**
- **R3**: Existing `npc_reorient_test.go` (6 tests) may rely on the no-reset behavior (e.g., constructs an NPC with `stepsTaken` left at zero, then asserts the gate fires). If any test sets `stepsTaken` to a non-zero value pre-call to simulate "NPC has moved before this tick", that test now needs to set it explicitly INSIDE the test (which it should anyway, since reorient runs same-tick after step). **Audit npc_reorient_test.go for stepsTaken touch points before T1.**

## 8. Deviations

None anticipated. Pure TS-fidelity port; matching field reset placement to TS's resetPathingEntity boundary.

## 9. Acceptance

- All six new test pins green.
- Existing NAI-157 pins (`TestNpcResetMasksClearsWalkDirRunDir`, `TestNpcDelayedAfterStepClearsStaleWalkDir`) green after the inversion/relocation update.
- `go test ./...` green.
- `go vet ./...` clean.
- No new deviation entries.
