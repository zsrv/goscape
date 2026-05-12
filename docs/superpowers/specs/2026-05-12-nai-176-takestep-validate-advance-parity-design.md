# NAI-176 — NPC/Player takeStep + validateAndAdvanceStep parity

**Status:** Draft — design approved 2026-05-12.
**Predecessor:** NAI-175 (commit `5c5281d`) ported D0 strategy plumbing + D1 axis-fallback into `(*Npc).stepOnce`; D2/D3/D4 deferred behind NAI-175-D-* deviation tags.
**Memory context:** `memory/nai175_step_collision_strategy.md`.

## §1 — Goal & scope

Port the three NAI-175-deferred deviation arms to full TS PathingEntity parity:

- **D2 → `NAI-175-D-WAYPOINT-RETENTION`** — distinguish *transient block* (keep `waypointIndex`) from *waypoint reached / no-move* (decrement, recurse to next waypoint). Mirrors TS `PathingEntity.takeStep` returning `null` vs `-1` (`PathingEntity.ts:617-683`) consumed by `validateAndAdvanceStep` (`PathingEntity.ts:202-232`).
- **D3 → `NAI-175-D-SIZE-GT-1`** — `width > 1` arm that uses `Face(srcX, 0, x, 0)` / `Face(0, srcZ, 0, z)` axis-only checks (TS `PathingEntity.ts:642-651`).
- **D4 → `NAI-175-D-PLAYER-STEP-COLLISION`** — `(*Player).stepOnce` plumbs `p.blockWalkFlag()` (= `FlagBlockPlayers`) and `p.getCollisionStrategy()`, and gains D1 axis-fallback. Currently passes `(size=1, extraFlag=0, TypeNormal)` to `gamemap.CanTravel`.

**Stretch:** `MoveStrategyFly` enum + early-return (TS `PathingEntity.ts:663-665`). No content wires it; engine-fidelity only.

**Out of scope:** Touching `applyStep` semantics; pathfinder; `reorient`; `refreshZonePresence`; introducing `instance == Player` checks. TS-vestigial branches stay vestigial.

**Smoke binding:** none. All three arms are latent per NAI-175's Stage 1 audit. TS-fidelity-only sub-spec; closes on green tests + retired deviation tags.

## §2 — Architecture

Refactor lifts `stepOnce` from a one-shot `(advanced, dir)` to a *pure direction query* with a tri-state status, wrapped by `validateAndAdvanceStep` which owns `waypointIndex` bookkeeping and the recursive try-next-waypoint cascade.

```
modules/world/movement_consts.go
    + type stepStatus int
    + const ( stepMoved stepStatus = iota; stepDone; stepBlocked )
    + (stretch) extend the existing MoveStrategy const block with MoveStrategyFly
      (after MoveStrategyNaive at movement_consts.go:31)

modules/world/npc_interaction.go
    (*Npc).stepOnce(s) (int, stepStatus)            // mirrors TS takeStep
        - returns stepBlocked   = TS null (transient block; keep waypointIndex)
        - returns stepDone      = TS -1   (waypoint reached / no-move; decrement)
        - returns stepMoved+dir = TS dir  (position applied via applyStep)
        + width > 1 pre-branch (D3) at the equivalent of TS L642-651
        + (stretch) MoveStrategyFly early-return at the equivalent of TS L663-665
    + (*Npc).validateAndAdvanceStep(s) (int, bool)  // mirrors TS PathingEntity.validateAndAdvanceStep
        - returns (-1, false) on stepBlocked         (D2: waypointIndex preserved)
        - on stepDone: waypointIndex--, recurse if still ≥ 0
        - on stepMoved: returns (dir, true)

    (*Npc).updateMovement — calls validateAndAdvanceStep twice (walk + run);
        same walkDir/runDir bookkeeping; position update stays inside applyStep
        (called from the stepMoved arm of stepOnce). No behavior change for the
        moved arm.

modules/world/movement.go
    (*Player).stepOnce() (int, stepStatus)          // D4: full PathingEntity.takeStep parity
        + plumbs p.blockWalkFlag() / p.getCollisionStrategy()
        + adds D1 axis-fallback (X-only / Z-only retry)
        + (stretch) MoveStrategyFly early-return
        - keeps inline lastStepX/Z + refreshPlayerZone bookkeeping (no factor)
    + (*Player).validateAndAdvanceStep() (int, bool)
        — same shape as NPC wrapper
        — caller in updateMovement (movement.go:91 + L103 for run-arm) switches
          to the wrapper

(no other callers of stepOnce on either type; grep confirms)
```

**Bundle ordering (small-blast-radius first):**

- **B1** — Refactor NPC `stepOnce` signature → `(int, stepStatus)`; introduce `(*Npc).validateAndAdvanceStep`; switch `updateMovement` to wrapper; land D2. Retires `NAI-175-D-WAYPOINT-RETENTION`.
- **B2** — Add `n.size > 1` pre-branch (D3) inside the new NPC `stepOnce`. Retires `NAI-175-D-SIZE-GT-1`.
- **B3** — Mirror everything on Player + axis-fallback (D4). Retires `NAI-175-D-PLAYER-STEP-COLLISION`.
- **B4 (stretch)** — `MoveStrategyFly` enum + early-return in both bodies. Carves `NAI-176-D-FLY-NO-CONTENT-WIRES` deviation tag (no content callsite assigns `MoveStrategyFly`).

## §3 — Data flow & semantics

### Tri-state contract

| TS `takeStep` return | goscape `stepOnce` return         | wrapper action                                          |
| -------------------- | --------------------------------- | ------------------------------------------------------- |
| `null`               | `(_, stepBlocked)`                | return `(-1, false)`; **waypointIndex preserved (D2)**  |
| `-1`                 | `(_, stepDone)`                   | `waypointIndex--`; if ≥ 0 recurse, else return `(-1, false)` |
| `number` (dir)       | `(dir, stepMoved)` + applyStep    | return `(dir, true)`                                    |

### TS-to-goscape branch mapping

```
TS PathingEntity.takeStep:617-683          → (*Npc).stepOnce / (*Player).stepOnce
  L620-623 waypointIndex===-1 → null       → (_, stepBlocked) at entry guard
                                              (existing goscape returns (false,-1);
                                               re-classified as stepBlocked —
                                               wrapper's caller already short-circuits
                                               on waypointIndex<0 in updateMovement,
                                               so no observable diff)
  L625-628 strategy null → -1              → (_, stepDone)
  L630-635 extraFlag NULL → -1             → (_, stepDone)
  L640    unpackCoord(waypoint)            → unchanged
  L642-651 width>1 arm                     → D3: new pre-branch on n.size>1
                                              (Player skips — Width() ≡ 1)
  L654-661 Face/delta + zero-delta guard   → (_, stepDone) on Face==-1
                                              (covers TS L659-661 dx==0 && dz==0
                                              via existing goscape Face short-circuit)
  L663-665 FLY early-return                → stretch: MoveStrategyFly → stepMoved+dir
  L668-670 direct travel                   → applyStep → (dir, stepMoved)
  L672-675 X-only fallback                 → applyStep → (axisDir, stepMoved)
  L677-680 Z-only fallback                 → applyStep → (axisDir, stepMoved)
  L682    null                             → (_, stepBlocked)   ← D2 binding line
```

### Entry-guard reclassification

Existing `(*Npc).stepOnce` returns `(false, -1)` when `n.waypointIndex < 0` — in the new tri-state, that becomes `(_, stepBlocked)` so the wrapper does *not* try to decrement further. Behaviorally identical (the existing `updateMovement` early-returns when `waypointIndex < 0` before reaching the wrapper anyway); the classification is semantically cleaner.

### Player divergence retired without factoring

`(*Player).stepOnce` currently has inline zone-refresh + `lastStepX/Z` bookkeeping (movement.go:150-158). That stays inside the moved arm — no factoring across types (Npc.applyStep and Player's inline body diverge intentionally per the `modules/world` cross-type pattern; the duplication is by design).

## §4 — Tests

Per the "per-arm pin + cross-arm integration" depth choice. New tests live alongside existing siblings.

### B1 (D2 waypoint retention) — `modules/world/npc_interaction_test.go`

- `TestNpcStepOnce_TransientBlock_PreservesWaypointIndex` — set NPC to `MoveRestrictNormal` next to a blocking NPC (`FlagBlockNPCs` on adjacent tile via zone-tracking); call `stepOnce`; assert returned status == `stepBlocked`, `waypointIndex` unchanged from setup.
- `TestNpcValidateAndAdvanceStep_DoneCascade_TriesNextWaypoint` — queue two waypoints where `waypoints[1]` is at NPC's tile (Face == -1 → stepDone) and `waypoints[0]` is one tile east; call wrapper; assert NPC steps east and returned dir matches the second waypoint.
- *(refactor)* Update existing axis-fallback / direct-walk tests (`TestNpcStepOnce_*` ~L2153-2240) to assert the new tri-state return where they previously asserted `(advanced, dir)`.

### B2 (D3 size>1) — `modules/world/npc_interaction_test.go`

- `TestNpcStepOnce_WidthGt1_PrefersXAxis` — width=2 NPC, target diagonal NE, only X axis is open; call `stepOnce`; assert returned dir == East, status == `stepMoved`. Pin uses `Face(srcX, 0, x, 0)` (the TS L643 form).
- `TestNpcStepOnce_WidthGt1_FallsThroughToZ` — width=2 NPC, X blocked, Z open; assert dir == North/South, status == `stepMoved`.
- `TestNpcStepOnce_WidthGt1_BothBlocked` — width=2 NPC, both axes blocked; assert status == `stepBlocked` (TS L651 `return null`).

### B3 (D4 Player parity) — `modules/world/movement_test.go`

- `TestPlayerStepOnce_PlumbsBlockWalkFlag` — mock `gamemap.CanTravel` (recording mock); call `stepOnce`; assert recorded `extraFlag == FlagBlockPlayers` and `strategy == TypeNormal` for a `MoveRestrictNormal` player.
- `TestPlayerStepOnce_AxisFallback_XOnly` — direct blocked, X-only open; assert dir == east (or matching axis), status == `stepMoved`.
- `TestPlayerStepOnce_TransientBlock_PreservesWaypointIndex` — symmetric of the NPC D2 pin.
- `TestPlayerValidateAndAdvanceStep_NoMoveRestrict_ReturnsBlocked` — `MoveRestrictNoMove` player; assert wrapper returns `(-1, false)` and `waypointIndex` untouched.

### Cross-arm integration — `modules/world/npc_interaction_test.go`

- `TestNpcUpdateMovement_RunSpeed_RecursesThroughDoneWaypoint` — running NPC at waypoint with Face == -1 on first step; assert that within one tick the wrapper recursion advances `waypointIndex` AND a run-step lands on the next waypoint (`walkDir` AND `runDir` both populated). Pins that the run-arm in `updateMovement` correctly re-enters the wrapper.

### B4 (stretch, FLY)

No test. Deviation tag captures "no content wires it." If desired, a 5-line fixture player with `moveStrategy = MoveStrategyFly` walking through a `FlagBlockWalk` tile is easy to add.

### Existing tests to update

- `TestNpcStepOnce_*` (`npc_interaction_test.go` ~L2153-2240) — all assert `(advanced bool, dir int)`. Convert to `(dir int, status stepStatus)`.
- `TestPlayerStepOnce_*` (`movement_test.go` ~L181, L225, L346) — same conversion.
- `nai101_fountain_test.go` — uses `stepOnce` indirectly via tick; plan-author re-runs to confirm unaffected.

## §5 — Risks & deviations

### Risks

- **R1 — Signature flip ripple.** All existing `stepOnce` callers (`updateMovement` for Npc; `movement.go:91`/`L103` for Player; ~6 test sites) change return shape simultaneously. Mitigation: B1 is a single-bundle atomic refactor with no parallel-impl window; all callers convert in one commit. `stepStatus` is a `modules/world` internal type — no consumer outside the package. Priority: **medium**.

- **R2 — Recursion depth.** `validateAndAdvanceStep` recurses on stepDone while `waypointIndex ≥ 0`. Worst case is `len(waypoints) == 25` stack frames (`[25]int` per `plan_waypoint_api_shape.md`). Trivial; Go default stack handles it. TS does the same recursion. Calling it out so reviewers don't flag it. Priority: **none (informational)**.

- **R3 — Width=2 fixture availability.** D3 tests need a width=2 NPC. Need to confirm `newTestNpc` / `newRegisteredNpc` allow setting `n.size` directly or via `NpcType.Size`. **Plan-author pre-flight:** grep `newTestNpc` / `newRegisteredNpc` / direct `n.size =` writes in tests; if no path exists, add a test-only setter (mirrors `SetLandBytesForTest` per `test_export_underscore_test_visibility.md`). Priority: **low (mechanical)**.

- **R4 — Player `lastStepX/Z` bookkeeping in moved arm.** TS PathingEntity.takeStep doesn't write `lastStepX/Z`; goscape's `(*Player).stepOnce` writes them at L150-151. **Plan-author pre-flight:** grep `lastStepX|lastStepZ` consumers (player.go + tests); confirm consumer expectations. Mitigation: keep the write where it is (inside the moved arm of stepOnce, before applyStep equivalent) and document as a goscape-specific bookkeeping detail. Priority: **low**.

- **R5 — Smoke binding still latent.** All three arms are unobserved-in-the-wild. No symptom-binding smoke for NAI-176. Verdict per `cascade_theory_smoke_binding.md`: TS-fidelity-only sub-spec, closes on green tests + retired deviation tags. No smoke handoff expected. Priority: **explicitly accepted, not mitigated**.

### Deviations expected at close

- `NAI-176-D-FLY-NO-CONTENT-WIRES` (stretch B4) — `MoveStrategyFly` enum + early-return ported, but no `NpcType.MoveStrategy` / Player content assigns it. Rationale: no flying entity in cache; engine-fidelity only. To retire: when first FLY-moveStrategy content (e.g., wyvern, dragon) ports.

### Deviations retired

- `NAI-175-D-WAYPOINT-RETENTION` (in `npc_interaction.go`) — closes at end of B1.
- `NAI-175-D-SIZE-GT-1` (in `npc_interaction.go`) — closes at end of B2.
- `NAI-175-D-PLAYER-STEP-COLLISION` (in `movement.go`) — closes at end of B3.

Per `retire_deviation_grep_all_comments.md`: `rg "NAI-175-D-(WAYPOINT-RETENTION|SIZE-GT-1|PLAYER-STEP-COLLISION)" pkg/ modules/ cmd/ docs/` before close to enumerate ALL comment + doc-tag references (not just the production touch points).

## §6 — Cadence & close

**Cadence:** runescript-s per `runescript_cadence.md`. Subagent-driven TDD per `execution_mode_default.md`.

**Bundle close criteria:**

- **B1** closes when: `stepStatus` type lands; `(*Npc).validateAndAdvanceStep` lands; `updateMovement` switches to wrapper; D2 pin tests green; `NAI-175-D-WAYPOINT-RETENTION` tag retired; `go test ./modules/world/...` green; `go test ./...` green (full repo).
- **B2** closes when: width>1 pre-branch lands; 3 D3 pin tests green; `NAI-175-D-SIZE-GT-1` tag retired; both test suites green.
- **B3** closes when: Player.stepOnce + Player.validateAndAdvanceStep land with full takeStep parity; 4 D4 pin tests green; `NAI-175-D-PLAYER-STEP-COLLISION` tag retired; both test suites green.
- **B4 (stretch)** closes when: `MoveStrategyFly` constant + early-return in both bodies; `NAI-176-D-FLY-NO-CONTENT-WIRES` tag carved.

**Final close — memory + commit trailer:**

- Update `memory/nai175_step_collision_strategy.md`: mark D2/D3/D4 as ported in NAI-176; trim the deviation-tag tail.
- Add new memory entry if anything non-derivable surfaces during impl (e.g., a surprising `stepStatus` consumer, a width=2 fixture gotcha).
- Close commit per `close_commit_memory_trailer.md`: `chore(close): NAI-176 — port NAI-175 D2/D3/D4 deviation arms` with `Closes memory: nai175_step_collision_strategy.md` trailer.

**Smoke handoff:** none expected (R5). If implementer observes adjacent symptom via test surface, route per `smoke_surfaces_adjacent_divergences.md` (in-scope stretch ≤ 30 LOC, else NAI-177).

## §7 — TS-fidelity references

- TS `PathingEntity.takeStep` — `Engine-TS/src/engine/entity/PathingEntity.ts:617-683`.
- TS `PathingEntity.validateAndAdvanceStep` — `Engine-TS/src/engine/entity/PathingEntity.ts:202-232`.
- TS `Player.blockWalkFlag` — `Engine-TS/src/engine/entity/Player.ts:706-708` (unconditional `FlagBlockPlayers`).
- TS `Npc.blockWalkFlag` — `Engine-TS/src/engine/entity/Npc.ts:381-398`.
- TS `PathingEntity.getCollisionStrategy` — `Engine-TS/src/engine/entity/PathingEntity.ts:558-575` (shared by Npc + Player; goscape duplicates per type since Go doesn't inherit).
- NAI-175 spec — `docs/superpowers/specs/2026-05-12-nai-175-npc-step-collision-strategy-design.md`.
- NAI-175 plan (deviation tag carve points) — `docs/superpowers/plans/2026-05-12-nai-175-npc-step-collision-strategy.md` (esp. §"D4 deferral fallback").
