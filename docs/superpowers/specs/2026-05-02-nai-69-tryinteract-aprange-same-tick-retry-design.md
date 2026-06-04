# NAI-69 — `tryInteract` same-tick AP retry (TS L1166-1170 port)

**Status:** Spec written 2026-05-02.
**Predecessor:** NAI-68 (HEAD `c5fcc0d`). Net deviation tally entering: 13.
**Closes:** `NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED`.
**Tech stack:** Go 1.26+. TS source canonical path: `LostCityRS/Engine-TS`.

## 1. Background

TS Player.ts:1139-1170 (the AP arm of `tryInteract`) closes with a
nextTarget-vs-apRangeCalled priority block:

```ts
this.nextTarget = this.target;
this.target = target;
if (this.nextTarget) {
    this.clearWaypoints();
}
// if aprange was called then we did not interact.
else if (this.apRangeCalled) {
    this.waypoints = wayPoints;
    this.waypointIndex = waypointIndex;
    this.target = target;
    return false;
}
return true;
```

The `else if (this.apRangeCalled) … return false` arm is the TS same-tick
retry mechanism: when the AP script called `p_aprange` (which sets
`apRangeCalled=true`), `tryInteract` returns `false`, restores the
saved waypoints/target, and lets `processInteraction`'s walk-arm and
post-step `tryInteract` retry within the same tick — typically firing
AP again with the new range or firing OP if walking reached contact
distance.

NAI-68 ported the OP/AP save-clear-exec-capture-restore framework + the
`nextTarget` priority arm but explicitly deferred the `apRangeCalled`
return-false arm under deviation tag `NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED`.
Goscape's two AP fire helpers that *do* support `p_aprange` mutability
(AP-Loc, AP-Obj) simulate the equivalent behavior via an
**across-tick** mechanism: leave `interactionFired = false` on the
`Finished/Aborted + apRangeCalled` exit path so the next tick's
`tryInteract` re-fires AP. The cost is one wasted tick of player
movement vs TS's same-tick re-evaluation.

The two mechanisms produce the same final interaction outcome, but the
tick-count divergence is observable to the client (one extra tick of
no movement when an AP script lowers range mid-fire). True-to-TS gate
calls for the same-tick path.

Two AP fire helpers that already conform to the new shape:

- **AP-Npc** (`fireApTriggerNpc`, `interaction_trigger.go:311`) —
  always sets `interactionFired=true` at end. `effectiveApRange` for
  Npc target reads `npc.typ.AttackRange`, not `p.apRange`, so
  `p_aprange` has no functional effect against an NPC target. The
  mechanism activates structurally but is a behavioral no-op.

- **AP-Player** (`fireApTriggerPlayer`,
  `player_interaction_trigger.go:88`) — always sets
  `interactionFired=true` at end. `effectiveApRange` for Player target
  reads `p.apRange`, so `p_aprange` IS functionally meaningful.
  AP-Player gains same-tick retry behavior with this port.

## 2. Goal

Mirror TS Player.ts:1163-1170 by closing `NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED`:

1. After `tryFireApTrigger(p)` in `tryInteract`, mirror TS's
   nextTarget-vs-apRangeCalled priority: when `p.nextTarget == nil &&
   p.apRangeCalled`, restore the per-tick re-fire gate (reset
   `interactionFired=false`) and return `false`. The waypoint/target
   restore is already done inside the AP fire helpers (NAI-68 T3
   bundled this work).

2. Drop the across-tick `apRangeCalled` early-return path in
   `fireApTriggerLoc` and `fireApTriggerObj`: remove the
   `Finished/Aborted + apRangeCalled` early-return that left
   `interactionFired = false`, and the accompanying `p.repathed = false`.
   Always set `interactionFired=true` at end of fire — uniform with
   AP-Npc and AP-Player.

3. Doc-comment refresh at four sites:
   - `fireApTriggerLoc` and `fireApTriggerObj` doc-comment headers:
     drop "across-tick re-fire" narration; cite TS L1163-1170 same-tick
     mechanism with cross-reference to `tryInteract`.
   - `fireApTriggerNpc` doc-comment header: rephrase the
     "NO apRangeCalled persistence contract" subsection to "structurally
     active per TS-uniform AP block; behaviorally no-op because
     `effectiveApRange` reads `npc.typ.AttackRange` instead of
     `p.apRange` (goscape design choice)." Cite the relevant TS line.
   - `tryInteract`'s AP branch: cite TS L1163-1170 alongside the new
     return-false logic.

4. Doc-retirement (carry-forward from NAI-68 close): at
   `interaction.go:237`, the `// nextTarget pop + auto-clear …
   NAI-68 closes // NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET:` lead-in is
   technically a closure narration but reads as if `NAI-44-D-…` were
   still active. Rephrase as a single closure sentence per
   `retire_deviation_grep_all_comments.md`.

5. Tag retirement: remove `DEVIATION NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED`
   inline doc-comments at `interaction_trigger.go:470` and `:662`.

Closes `NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED`. Opens nothing.
**Net tally: 13 → 12.**

## 3. Out of scope

- **Drop `interactionFired` field entirely.** TS has no equivalent;
  goscape's per-tick guard is structurally redundant given
  `processInteraction`'s `if !interacted` discipline (only two
  `tryInteract` calls per tick: pre-step + post-step). The reset-with-
  return-false (this sub-spec) is sufficient for behavioral TS-fidelity.
  Field removal is its own audit (~30 test assertions pin
  `interactionFired`); deferred as a future cleanup candidate.
- `NAI-44-D-CANACCESS-NO-STUN-CHECK` — stun system port.
- `NAI-67-D-PLAYER-UNFOCUS-DEFERRED` — Player respawn/death sub-spec.
- AP-Npc functional retry — blocked on
  `effectiveApRange`-divergence reframe (goscape reads
  `npc.typ.AttackRange`; TS reads `this.apRange` uniformly). This is a
  separate goscape design choice predating NAI-69; the mechanism
  activation here is structurally TS-faithful but the no-op behavior
  for NPC targets is preserved per the existing divergence.

## 4. Behavior delta (the change a player observes)

**Before NAI-69:** AP script targets a Loc 5 tiles away. AP fires,
script calls `p_aprange(2)` (lower allowed range from 10 to 2). Script
finishes. `apRangeCalled=true`. Goscape leaves `interactionFired=false`
and returns from the fire helper. `tryInteract` returns true.
processInteraction's tail else-if doesn't clear (apRangeCalled gates
it off). Player skips this tick's walk arm (the `if !interacted` guard
short-circuits). Next tick: `tryInteract` re-fires AP with the new
2-tile range; the player still hasn't moved, AP fires again, and so on
until the player walks within 2 tiles. **One wasted tick of movement
per `p_aprange` call.**

**After NAI-69:** Same setup. AP fires, `p_aprange(2)` sets
`apRangeCalled=true`. `tryInteract` checks `nextTarget == nil &&
apRangeCalled` → resets `interactionFired=false`, returns `false`.
processInteraction's `if !interacted` arm runs: recalc path, walk-arm
moves the player one step closer, post-step `tryInteract` re-fires AP
with the new range (or fires OP if the walk reached contact distance).
**No wasted tick.**

**For AP-Player target:** Same behavior change applies (`p.apRange` is
the relevant field).

**For AP-Npc target:** Same code path activates; `apRangeCalled` is
true; `tryInteract` returns false; walk arm runs; post-step retry.
But `effectiveApRange` reads `npc.typ.AttackRange` (unchanged), so
post-step AP fires with the same range. No infinite loop because
`apRangeCalled` is reset at fire start (mirrors TS L1141), so each
fire is a fresh evaluation. Behaviorally indistinguishable from the
pre-NAI-69 state for NPC targets.

**Suspended AP** (script calls `p_delay`, `p_pausebutton`,
`p_countdialog`): suspended scripts don't reach the AP block's tail
(executeScript returns early on suspend). `apRangeCalled` was
pre-reset at fire start and is never set during suspend. `tryInteract`
sees apRangeCalled=false → returns true. Tail's else-if fires
ClearInteraction. Resumed script runs from its preserved ScriptState
(holds Self/ActiveLoc/etc.) and cleared `p.target` doesn't affect it.
**Same as TS — preserved.**

**`nextTarget` priority**: if a script calls both `p_aprange` AND
`p_op_loc` (or another `p_op_*`), `nextTarget` is non-nil after the
fire. The new check `if p.nextTarget == nil && p.apRangeCalled` skips
the same-tick retry path; `tryInteract` returns true; tail's
`if p.nextTarget != nil { p.target = p.nextTarget }` arm pops.
**Mirrors TS L1158-1161 priority.**

## 5. Code map

| File | Change | LOC delta |
|---|---|---|
| `modules/world/interaction.go` (`tryInteract` AP branch ~L324-330) | Add `if p.nextTarget == nil && p.apRangeCalled { p.interactionFired = false; return false }` after `tryFireApTrigger(p)`. | +3 prod |
| `modules/world/interaction.go:237` | Rephrase "NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET:" lead-in as a clean "NAI-68 closes …" closure narration. | doc only |
| `modules/world/interaction_trigger.go` (`fireApTriggerLoc` ~L468-484) | Remove the `Finished/Aborted + apRangeCalled` early-return block + `p.repathed = false` line. Always set `interactionFired = true` at end. Drop the `DEVIATION NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED` inline tag. Doc-comment header: cite TS L1163-1170 same-tick mechanism via `tryInteract`. | -14 prod, +2 doc |
| `modules/world/interaction_trigger.go` (`fireApTriggerObj` ~L660-676) | Same as `fireApTriggerLoc`. | -14 prod, +2 doc |
| `modules/world/interaction_trigger.go` (`fireApTriggerNpc` doc header ~L290-310) | Rephrase "NO apRangeCalled persistence contract" subsection. | doc only |
| `modules/world/player_interaction_trigger.go` (`fireApTriggerPlayer` doc header) | Add note that AP-Player gains same-tick retry behavior with NAI-69. | doc only |
| `modules/world/interaction_trigger_test.go` + `interaction_test.go` + `player_interaction_trigger_test.go` | New tests + adjust pre-NAI-69 across-tick-re-fire pinning tests. | +~250 test |

## 6. Pre-flight grep targets (controller_preflight)

Verified at HEAD `c5fcc0d`:

- `rg "NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED" pkg/ modules/` → 2 hits
  (interaction_trigger.go:470, :662). Both retire in T2.
- `rg "interactionFired" modules/world/interaction.go` → 4 hits
  (lines 86 SetInteraction reset, 134 ClearInteraction reset, 317 OP gate,
  326 AP gate). T1 adds a 5th reset inside the new AP-branch
  return-false arm.
- `rg "p\.repathed = false" modules/world/interaction_trigger.go` → 2
  hits (Loc and Obj fire helpers). T2 deletes both.
- `rg "Finished \|\| state\.Execution == script\.Aborted" modules/world/interaction_trigger.go`
  → 2 hits (Loc and Obj). T2 removes both blocks entirely.
- `rg "NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET" modules/ pkg/` → 2 hits
  (interaction.go:237 needs rephrase; interaction_trigger.go:177 is
  a clean "NAI-68 closes …" line that stays).
- `rg "func .*tryFireApTrigger\b" modules/world/` → 1 hit
  (interaction_trigger.go:271). The dispatch switch has 4 arms (Loc,
  Npc, Player, Obj) — confirmed in `enumerate_all_sites.md` discipline.
- `rg "p\.apRangeCalled" modules/world/ pkg/script/` → confirms set
  sites (`pkg/script/active.go:342` p_aprange handler) and reset
  sites match expectations.
- **§7.5 reframe targets:**
  `rg "TestTryFireApTriggerLocScriptCallsPApRange|TestTryFireApTriggerObjScriptCallsPApRange" modules/world/`
  → identifies the existing across-tick re-fire pinning tests that
  T2 must reframe. T2 verifies presence/absence of the AP-Obj mirror
  before edit.

## 7. Test plan

Single TDD bundle. Tests pin the behavior delta at every fire-helper
target type, and pin the priority order vs `nextTarget`.

### 7.1 AP-Loc same-tick retry (new file `interaction_trigger_aprange_test.go` or extend existing)

- **`TestApTriggerLoc_SameTickRetry_RangeLowered`** — script calls
  `p_aprange(2)`. Player at distance 5 (in original 10-range). Pre-step
  AP fires. `tryInteract` returns false. Walk-arm runs (assert
  `p.repathed == true` post-tick, assert `p.stepsTaken == 1`).
  Post-step AP doesn't fire (still out of 2-range). Tail doesn't
  auto-clear. interaction preserved for next tick.

- **`TestApTriggerLoc_SameTickRetry_AcrossToOp`** — script calls
  `p_aprange(0)` (effectively disable AP). Player adjacent (would-be
  OP range). Pre-step AP fires (last AP in 10-range). post-step
  `tryInteract(allowOpScenery: stepsTaken==0)` — but stepsTaken==1 so
  allowOpScenery=false. So OP doesn't fire for Loc. Test verifies the
  same-tick walk-then-no-AP-no-OP outcome (matches TS — Loc OP only
  fires same-tick if walking didn't happen).

- **`TestApTriggerLoc_SameTickRetry_StepsZeroOpFires`** — Player
  adjacent to Loc, AP script lowers range, but stepsTaken stays 0
  because path was already exhausted. Post-step uses
  `allowOpScenery=true`. OP fires same tick.

- **`TestApTriggerLoc_NextTargetPriorityOverApRange`** — script calls
  BOTH `p_aprange(0)` AND `p_op_loc`. tryInteract checks
  `nextTarget == nil && apRangeCalled` — nextTarget is set, so the
  retry path is SKIPPED. Returns true. Tail's `if p.nextTarget != nil`
  arm pops the new target. (Pins TS L1158-1161 priority.)

- **`TestApTriggerLoc_SuspendedNoRetry`** — script calls `p_delay`.
  Suspends. `apRangeCalled` stays false (never set). tryInteract
  returns true. Tail clears. (Pins suspended-path behavior unchanged.)

### 7.2 AP-Obj same-tick retry

- **`TestApTriggerObj_SameTickRetry_RangeLowered`** — symmetric to AP-Loc
  range-lowered case.
- **`TestApTriggerObj_NextTargetPriorityOverApRange`** — symmetric.
- **`TestApTriggerObj_SuspendedNoRetry`** — symmetric.

### 7.3 AP-Player same-tick retry (new — gains behavior with NAI-69)

- **`TestApTriggerPlayer_SameTickRetry_RangeLowered`** — script
  targeting another player calls `p_aprange(2)`. Verifies
  `effectiveApRange` reads `p.apRange` for Player target (range now 2).
  Same-tick retry mechanism activates.
- **`TestApTriggerPlayer_NextTargetPriorityOverApRange`** — same shape.

### 7.4 AP-Npc structural parity (no-op behavior preserved)

- **`TestApTriggerNpc_ApRangeCalled_NoOp`** — script targeting an NPC
  calls `p_aprange(0)`. tryInteract returns false (mechanism activates).
  Walk-arm runs. Post-step uses `effectiveApRange = npc.typ.AttackRange`
  (unchanged) — so range check is unchanged. No infinite loop (test
  runs for 3 ticks, asserts no panic / no excessive script invocations).
  Pin that the mechanism is structurally active but functionally a
  no-op for NPC targets.

### 7.5 Pre-existing across-tick re-fire tests — reframe

One existing test pins the across-tick re-fire mechanism directly:

**`TestTryFireApTriggerLocScriptCallsPApRange`** at
`interaction_trigger_test.go:504-530`. Three of its post-fire
assertions encode the across-tick mechanism and must change in T2:

- `if p.repathed { t.Error("repathed: want false …") }` — DELETE.
  T2 removes the `p.repathed = false` line from `fireApTriggerLoc`;
  the test's `p.repathed = true` setup line at `:511` should also be
  deleted (no longer relevant).
- `if p.interactionFired { t.Error("interactionFired: want false (allow
  re-fire next tick)") }` — INVERT to `if !p.interactionFired { … "want
  true" }`. The new contract: fire helper sets `interactionFired=true`
  uniformly; the reset happens in `tryInteract`'s return-false arm
  (covered by the new T1 test).
- Doc-comment header at `:500-503` ("…interaction to PERSIST past the
  tick. repathed is reset…") — REPHRASE to describe the new contract:
  fire helper completes cleanly with `apRangeCalled=true` and
  `interactionFired=true`; same-tick retry is the responsibility of
  `tryInteract`.

The `target`, `apRange`, and `apRangeCalled` assertions survive
unchanged — those reflect script-handler effects that NAI-69 doesn't
touch.

**Parallel AP-Obj test:** if a `TestTryFireApTriggerObjScriptCallsPApRange`
mirror exists at HEAD (T2 verifies via grep), apply the same three
edits.

Other `interactionFired == false`-pinning tests (`TestTryFireOpTrigger_PlayerDelayed`,
`TestTryFireApTriggerLocDeferredOnDelay`, `TestTryFireOpTriggerLocDeferredOnDelay`,
`TestFireApTriggerNpcDeferredOnDelay`, plus the OP-Obj deferred test
at `handler_opobj_test.go:639`) all pin the **delayed-player early-
return** path (`p.delayed && currentTick < delayedUntil`), NOT the
across-tick re-fire. NAI-69 doesn't touch that path; these tests are
unaffected.

### 7.6 `interactionFired`-state regression checks

Pre-NAI-69, AP-Loc + apRangeCalled exit left `interactionFired=false`.
Post-NAI-69, the fire helper sets `interactionFired=true` always; the
reset to false happens inside `tryInteract`'s return-false arm. New
tests should pin:

- **`TestApTriggerLoc_FireHelperSetsInteractionFiredOnFinishApRangeCalled`** —
  isolated test of the fire helper (not via tryInteract) confirming
  `interactionFired=true` post-fire even when apRangeCalled=true.
- **`TestTryInteract_ResetsInteractionFiredOnApRangeCalledReturnFalse`** —
  isolated test of `tryInteract` confirming `interactionFired=false`
  after the AP-branch returns false.

## 8. Implementation tasks (TDD bundle)

### T1 — `tryInteract` AP-branch return-false logic (red→green)

Order:
1. Write `TestTryInteract_ResetsInteractionFiredOnApRangeCalledReturnFalse`
   — fails (current code returns true).
2. Write `TestApTriggerLoc_NextTargetPriorityOverApRange` — fails or
   passes accidentally; confirm fail-on-correct-reason.
3. Update `tryInteract` AP branch (3 LOC + doc).
4. Re-run; both pass.

### T2 — drop fire-helper across-tick early-return (red→green)

Order:
1. Write `TestApTriggerLoc_FireHelperSetsInteractionFiredOnFinishApRangeCalled`
   — fails (current code leaves false).
2. Drop the `if state.Execution == script.Finished || ...` block in
   `fireApTriggerLoc` (~14 LOC). Drop `p.repathed = false`.
3. Same for `fireApTriggerObj`.
4. Update doc-comment headers at both helpers + `fireApTriggerNpc` +
   `fireApTriggerPlayer`.
5. Re-run; passes.

### T3 — same-tick retry behavior bundle (red→green)

Order:
1. Write `TestApTriggerLoc_SameTickRetry_RangeLowered`.
2. Write `TestApTriggerLoc_SameTickRetry_AcrossToOp`.
3. Write `TestApTriggerLoc_SameTickRetry_StepsZeroOpFires`.
4. Write `TestApTriggerLoc_SuspendedNoRetry`.
5. All should pass after T1+T2 land. (T3 is verification + coverage,
   not a code change task.)

### T4 — AP-Obj parity tests

Add the AP-Obj tests from §7.2.

### T5 — AP-Player + AP-Npc parity tests

Add the AP-Player tests (§7.3, gains behavior) and AP-Npc tests (§7.4,
no-op pinning).

### T6 — doc-retirement sweep

1. Rephrase `interaction.go:237` lead-in.
2. Drop `DEVIATION NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED` inline tag at
   `interaction_trigger.go:470` and `:662`.
3. Verify `rg "NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED" pkg/ modules/`
   returns 0 hits.
4. Spot-check other deviation tags for similar staleness; minimal
   touches only.

### T7 — close commit

Per `close_commit_memory_trailer.md`: include `Closes memory:
NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED` trailer.

## 9. Risk register

- **Infinite loop risk** — guarded by `apRangeCalled` reset at fire
  entry (TS L1141 mirror, already in place at three of four sites).
  At most 2 AP fires per tick (pre-step + post-step). Mitigation in
  `TestApTriggerNpc_ApRangeCalled_NoOp` runs 3 ticks to assert
  bounded execution.

- **AP-Player retry path verified** — AP-Player fires through
  `srv.runScript`, which is a thin wrapper around `srv.resumeOrFinish`
  (`modules/world/script.go:86-99`). Both paths invoke
  `script.Execute`, which routes the `p_aprange` opcode through the
  shared `ActivePlayer.SetApRange` interface (`pkg/script/active.go:341-347`).
  `p.apRangeCalled` is set on the same `*Player` regardless of dispatch
  path. AP-Player same-tick retry works structurally identical to
  AP-Loc/AP-Obj.

- **Test fixture interaction with NAI-68 framework** — the NAI-68
  save/clear/exec/capture/restore framework saved waypoints and
  restored on no-nextTarget. The new return-false arm in `tryInteract`
  doesn't re-clear waypoints (they were restored by the fire helper).
  Verify that `tryInteract`'s walk-arm receives waypoints intact in
  the AP-retry test.

- **`interaction.go:237` doc-comment is functional, not just doc** —
  the rephrase MUST preserve all behavior: nextTarget pop priority,
  followOp narration, the auto-clear gate. Use surgical Edit, not
  rewrite.

## 10. Acceptance criteria

1. `rg "NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED" pkg/ modules/` returns 0
   hits at HEAD.
2. `go test ./...` -count=1 -race passes.
3. New tests in §7 all pass.
4. No existing test regresses (modulo the pre-existing across-tick
   re-fire pin if any was added — adjust per §7.5).
5. Doc-comments at `interaction.go:237`, `fireApTriggerLoc`,
   `fireApTriggerObj`, `fireApTriggerNpc`, `fireApTriggerPlayer`,
   and `tryInteract` AP branch all cite TS L1163-1170 with the
   same-tick mechanism narrative.
6. Net deviation tally in close commit body: **13 → 12**.

## 11. Memory entries reinforced

- `runescript_cadence.md` — full sub-spec → plan → TDD bundle → close.
- `true_to_ts_gate.md` — every behavioral change cited against TS
  Player.ts:1163-1170.
- `controller_preflight.md` — pre-flight grep targets in §6 verified
  against HEAD.
- `enumerate_all_sites.md` — all 4 AP fire helpers enumerated; tests
  pin all 4 paths.
- `retire_deviation_grep_all_comments.md` — T6 explicitly grep-verifies
  zero residual hits.
- `defensive_gate_doc_comment_label.md` — `effectiveApRange`
  Npc-divergence preserved as a labeled goscape design choice.
- `ts_asymmetry_dual_pin.md` — AP-Npc structural-parity-but-no-op test
  dual-pins both the activation and the absence of behavior change.
- `dead_api_polish.md` — `interactionFired` field flagged as future
  cleanup candidate (reference §3 out-of-scope).
- `close_commit_memory_trailer.md` — close commit carries
  `Closes memory:` trailer.
- `plan_test_coverage_crosscheck.md` — every code change in §5 has a
  matching test in §7.
