# NAI-68 — `p_op*` immediate→nextTarget reshape

**Status:** Spec written 2026-05-02.
**Predecessor:** NAI-67 (HEAD `94f6dab`). Net deviation tally entering: 13.
**Closes:** `NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET`.
**Tech stack:** Go 1.26+. TS source canonical path: `LostCityRS/Engine-TS`.

## 1. Background

TS Player.ts:1126-1170 (the OP and AP arms of `tryInteract`) saves the
current `target` into a local, sets `this.target = null`, runs the trigger
script, then captures whatever `setInteraction(...)` the script may have
called via `this.nextTarget = this.target` and restores `this.target = target`.
At `processInteraction`'s tail (TS L1255-1263) `nextTarget` is popped:

```ts
if (this.nextTarget) {
    this.target = this.nextTarget;
}
else if (interacted && !this.apRangeCalled) {
    this.clearInteraction();
}
```

Goscape's current shape (`modules/world/interaction.go:187-191`) carries
the deviation:

```
// TS L1203 (this.nextTarget = null) — DEVIATION NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET:
// goscape's p_op* opcodes do immediate SetInteraction swaps rather
// than queueing a nextTarget for next-tick application. No nextTarget
// field exists on *Player; the reshape below has no nextTarget block.
// Closure: future p_op* opcode reshape sub-spec.
```

The bug surfaces when an OP/AP trigger script calls `p_op_loc` /
`p_op_npc`: goscape's handler calls `SetInteraction` immediately, which
writes the new target onto `p.target`. The fire helper's
`if Execution == Finished || Aborted { ClearInteraction() }` AND the
processInteraction tail's unconditional `if interacted && !apRangeCalled
{ ClearInteraction() }` then both clobber the script-set target. Net:
script-side `p_op_*` mid-trigger is a no-op in goscape today.

## 2. Goal

Mirror TS Player.ts:1113-1170 + 1203 + 1255-1263 verbatim so:

1. Script-set `nextTarget` survives the auto-clear and applies on the next
   tick.
2. AP scripts that call `p_aprange` propagate `apReverted` out of
   `tryInteract` and re-enable the post-step retry arm.
3. The OP branch's TS L1131 `clearWaypoints()` runs (currently absent in
   goscape).
4. AP branch saves+restores waypoints around script execution (currently
   absent).

Closes `NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET`. No new deviations expected.

## 3. Out of scope

- `NAI-44-D-CANACCESS-NO-STUN-CHECK` — stun system port, future sub-spec.
- `NAI-67-D-PLAYER-UNFOCUS-DEFERRED` — Player respawn/death sub-spec.
- Adding `handleP_OpPlayer`, `handleP_OpObj`, `handleP_OpNpcT`,
  `handleP_OpPlayerT` opcodes — these don't exist at HEAD; the reshape
  works correctly for the `p_op_loc` / `p_op_npc` handlers that DO exist.
  Future opcode-port sub-specs add the rest; the reshape supports them.
- `*Npc` side `tryInteract` — the deviation is Player-side only.

## 4. Design

### 4.1 Data shape

Add `nextTarget entity` field on `*Player` (`modules/world/player.go`).

Single field. TS only saves/restores `this.target`; the `targetOp`,
`targetSubject`, `apRange`, `apRangeCalled`, `interactionKind`,
`faceEntity`, `targetX/Z`, `interactionFired` left over from a script's
`SetInteraction` call are intentionally not snapshotted because the next
tick reads them as the new active interaction.

### 4.2 `processInteraction` entry (TS L1203)

After the `target == nil` early-return, after the `client/server` and
`delayed` guards, after the `followX/followZ` mirror writes, and BEFORE
the level-mismatch check, insert:

```go
// TS L1203.
p.nextTarget = nil
```

(This placement matches TS — TS does the reset after the canAccess
guards but before validateTarget. Goscape's level-mismatch check is the
nearest equivalent of TS validateTarget at this site.)

### 4.3 `processInteraction` tail (TS L1255-1263)

Replace `interaction.go:245-247`:

```go
if interacted && !p.apRangeCalled {
    p.ClearInteraction()
}
```

with:

```go
// TS L1255-1263.
if p.nextTarget != nil {
    p.target = p.nextTarget
} else if interacted && !p.apRangeCalled {
    p.ClearInteraction()
}
```

The mapflag-clear at `interaction.go:256-258` is unaffected.

### 4.4 Two new helpers

Both go in `modules/world/interaction.go`. Signatures use the existing
`entity` interface from `movement_consts.go:45`.

```go
// runOpTriggerScript wraps srv.resumeOrFinish with TS Player.ts:1129-1135's
// save/clear/capture/restore pattern. The OP arm unconditionally clears
// waypoints (TS L1131) and never reverts.
func runOpTriggerScript(p *Player, srv *Server, state *script.ScriptState, savedTarget entity) {
    p.target = nil
    p.waypointIndex = -1 // TS L1131: this.clearWaypoints()
    srv.resumeOrFinish(state, p)
    p.nextTarget = p.target
    p.target = savedTarget
}

// runApTriggerScript wraps srv.resumeOrFinish with TS Player.ts:1145-1170's
// save/clear/capture/restore pattern. AP additionally saves+restores the
// waypoint queue and reverts waypoints when the script called p_aprange
// (apRangeCalled=true) without setting a nextTarget.
//
// Returns apReverted=true when the AP script asked for an extended range
// (TS L1167-1170 path); the caller propagates !apReverted as the
// tryInteract return value so the post-step arm gets a chance to retry.
func runApTriggerScript(p *Player, srv *Server, state *script.ScriptState, savedTarget entity) (apReverted bool) {
    savedWP := p.waypoints
    savedIdx := p.waypointIndex
    p.target = nil
    p.waypointIndex = -1
    srv.resumeOrFinish(state, p)
    p.nextTarget = p.target
    p.target = savedTarget
    if p.nextTarget != nil {
        // TS L1162: clear destination so the next-tick interaction starts fresh.
        p.waypointIndex = -1
        return false
    }
    if p.apRangeCalled {
        // TS L1167-1170: revert waypoints and re-enable post-step retry.
        p.waypoints = savedWP
        p.waypointIndex = savedIdx
        return true
    }
    return false
}
```

Goscape's `[25]int` waypoints array is copied by value in the local —
the array is small and trivially copyable. Restoring both array+index
matches TS's `this.waypoints = wayPoints; this.waypointIndex = waypointIndex`
exactly.

### 4.5 Per-fire-helper changes (8 sites)

**4 OP fires** (`fireOpTriggerNpc`, `fireOpTriggerLoc`,
`fireOpTriggerPlayer`, `fireOpTriggerObj`):

Replace the existing tail block:

```go
srv.resumeOrFinish(state, p)
if state.Execution == script.Finished || state.Execution == script.Aborted {
    p.ClearInteraction()
}
p.interactionFired = true
```

with:

```go
runOpTriggerScript(p, srv, state, p.target)
p.interactionFired = true
```

(`p.target` is the saved target value at the call site — `p` still
points to the entity locked in by the lifecycle/lookup-passes earlier
in the helper. `runOpTriggerScript` reads it before clearing.)

The Finished/Aborted clear is dropped — subsumed by the tail's
`else if interacted && !apRangeCalled` (which fires after every successful
trigger fire because `interacted=true` and goscape never sets
`apRangeCalled=true` in OP scripts).

**4 AP fires** (`fireApTriggerNpc`, `fireApTriggerLoc`,
`fireApTriggerPlayer`, `fireApTriggerObj`):

Same treatment, but the helper returns `apReverted` which propagates
out via `tryFireApTrigger`. The pre-script `apRangeCalled = false`
reset (currently explicit at `interaction_trigger.go:401` for AP-Loc)
must be present at all 4 AP sites — verify and add to AP-Npc, AP-Player,
AP-Obj if missing. Replace:

```go
p.apRangeCalled = false  // already present at AP-Loc; verify others
srv.resumeOrFinish(state, p)
if state.Execution == script.Finished || state.Execution == script.Aborted {
    p.ClearInteraction()
}
p.interactionFired = true
```

with:

```go
p.apRangeCalled = false  // TS L1141 — pre-set at all 4 AP sites
apReverted = runApTriggerScript(p, srv, state, p.target)
p.interactionFired = true
return apReverted
```

(`apReverted` becomes a return value of each per-entity AP fire.)

### 4.6 `tryFireApTrigger` signature change

`interaction_trigger.go:258` changes from:

```go
func tryFireApTrigger(p *Player) {
```

to:

```go
func tryFireApTrigger(p *Player) (apReverted bool) {
```

The type-switch routes to per-entity helpers; collect each's
`apReverted` return into the named return.

### 4.7 `tryInteract` AP arm

`interaction.go:321-327` changes from:

```go
if inApproachDistance(p.x, p.z, tx, tz, effectiveApRange(p)) {
    p.interacted = true
    if !p.interactionFired {
        tryFireApTrigger(p)
    }
    return true
}
```

to:

```go
if inApproachDistance(p.x, p.z, tx, tz, effectiveApRange(p)) {
    p.interacted = true
    if !p.interactionFired {
        if apReverted := tryFireApTrigger(p); apReverted {
            // TS L1167-1170: AP script called p_aprange; defer to post-step.
            return false
        }
    }
    return true
}
```

The OP arm at `interaction.go:307-320` is unchanged in return-value
shape — TS L1136 unconditionally returns true.

### 4.8 Lifecycle/lookup-failure clears (unchanged)

The `npc.dead`, `!locStillValid`, no-trigger, invalid-op, and
`delayed && currentTick < delayedUntil` early-clears inside each fire
helper run BEFORE script init. No script means no `nextTarget` to
preserve. Keep these as immediate `ClearInteraction()` + `interactionFired = true`.

## 5. Test plan

Five testable behaviors. Each pinned with new tests; existing tests
audited against the new contract.

### B1 — `nextTarget` pop overrides auto-clear

Direct unit test on `processInteraction`. Pre-set `p.target = locA`,
`p.nextTarget = npcB`, `p.interacted = true`, `p.apRangeCalled = false`.
Call `processInteraction`. Assert `p.target == npcB` post-call.
Counter-pin: with `p.nextTarget = nil` and same other state, assert
`p.target == nil` (auto-clear ran).

### B2 — `nextTarget` reset at processInteraction entry

Direct unit test. Pre-set `p.nextTarget = npcB`, `p.target = locA`.
Run a single `processInteraction` cycle that doesn't fire any trigger
(no script registered). Assert `p.nextTarget == nil` post-call. The
test also pins that the early `target.level != p.level` exit path
honors the reset (call once with mismatched level; assert reset still
happened — TS L1203 runs before validateTarget).

### B3 — OP/AP save-clear-restore (presence + dual-pin per `ts_asymmetry_dual_pin.md`)

Per-entity (Loc + Npc) test for both arms (OP + AP) where the trigger
script calls `p_op_npc` mid-execution. Setup uses the existing
`buildNpcSayScript` / `buildOpPlayerHintPlScript` test fixture pattern;
extend to a new fixture that pushes `s.ActiveNpc.nid` (or activeLoc) +
`op` + emits `OpPOpNpc` (or `OpPOpLoc`). After `processInteraction`,
assert `p.target == newTarget` AND `p.targetOp == newOp`. Counter-pin:
when the script does NOT call `p_op_*`, assert `p.target == nil`
(auto-clear via tail's else-if).

### B4 — apRangeCalled revert returns false from tryInteract

Test that an AP script calling `p_aprange(N)` causes the post-step arm
to execute. Setup: target outside operable range, AP script registered
for `[aploc1,<typeID>]` whose body is `push N; p_aprange; ret`. After
`processInteraction`: assert `p.repathed == true` (post-step recalc
ran) AND `p.apRangeCalled == true` (preserved into next tick). Pre-fix
this test would fail because pre-step returned true and short-circuited
post-step.

### B5 — OP branch's TS L1131 `clearWaypoints()` add (dual-pin)

Pre-set `p.waypointIndex >= 0` (active path). Run a `processInteraction`
that fires an OP trigger. Assert `p.waypointIndex == -1` post-call,
both when the script sets a `nextTarget` (B5a) and when it doesn't
(B5b). The dual-pin pre-empts a future regression that conditionalizes
the clear on script behavior.

### Existing tests — audit pass

- `interaction_trigger_test.go:451, 488-509` (apRangeCalled preserves
  interaction): must still pass — the tail's `!apRangeCalled` gate
  preserves `p.target` exactly as before.
- All `[oploc1,*]` / `[opnpc1,*]` trigger tests where the script does
  NOT call `p_op_*`: still pass — `nextTarget == nil` falls through
  to the tail's else-if auto-clear, identical to pre-fix behavior.
- AP-Loc tests pinning `apRangeCalled = false` reset behavior: still
  pass — the reset moves into the per-fire-site pre-set (already there
  for AP-Loc per `interaction_trigger.go:398-401`; T3 ensures all 4
  have it).

## 6. Risk register

### Risk 1: Suspended-script interaction state cleared at tail

Pre-reshape, fire helpers KEEP interaction across `Suspended` script
state (only clear on `Finished`/`Aborted`). Post-reshape, the per-helper
clear is dropped; the tail's `else if interacted && !p.apRangeCalled`
runs unconditionally (interacted=true after fire; apRangeCalled=false
unless script called `p_aprange`).

So **a script that suspends (P_DELAY/P_PAUSEBUTTON/P_COUNTDIALOG) inside
an OP/AP trigger will have `p.target` cleared at this tick's tail**,
where pre-reshape it stayed anchored.

This matches TS exactly — TS's tail clears on `interacted &&
!apRangeCalled` regardless of script suspend state. Suspended scripts
hold their own ActiveLoc/ActiveNpc/Self pointers internal to the
`ScriptState`; the player-side `p.target` clear is cosmetic for the
player's interaction state machine, not for the script's resumption.

T1's commit body explicitly notes this change. T3 adds a regression
test pinning "OP-script suspends → `p.target` cleared at tail" so the
intent is grep-discoverable.

### Risk 2: `tryFireApTrigger` signature change blast radius

Quick grep at plan-write time confirms `tryFireApTrigger` is package-private
(`world` package); only call site is `tryInteract` AP arm at
`interaction.go:323`. Pre-flight: `rg "tryFireApTrigger" --glob '*.go' modules/world/`
should yield exactly the definition + 1 call site + the 4 per-entity
delegations.

### Risk 3: Implementer drifting `runOpTriggerScript` / `runApTriggerScript`

The two helpers are TS-anchored — the body lines correspond 1:1 to
specific TS source lines (cited in 4.4's doc-comments). Plan-doc
includes the TS lines verbatim adjacent to each Go statement; reviewer
checks one-to-one mapping at T2 and T3 close.

### Risk 4: Pre-existing AP fire helpers missing `apRangeCalled = false` pre-set

AP-Loc explicitly pre-sets at `interaction_trigger.go:401`. AP-Npc
(line 298), AP-Player, AP-Obj must also pre-set per TS L1141 — but
the per-helper code may already do this implicitly via `SetInteraction`
clearing it (which it does at line 84 of `interaction.go`), if the
fire happens immediately after `SetInteraction`. However the AP fire
runs at the post-step arm where `SetInteraction` may be many ticks
old. Plan T3 verifies and adds the pre-set where missing. Pre-flight:
`rg "p\.apRangeCalled\s*=\s*false" --glob '*.go' modules/world/`.

### Risk 5: Waypoints array copy correctness

`waypoints [25]int` is a value-type array (not slice). `savedWP :=
p.waypoints` is a value copy — the 25 ints are duplicated. Restore via
`p.waypoints = savedWP` overwrites all 25 entries. This is a complete
snapshot+restore semantically identical to TS's `this.waypoints =
wayPoints` (TS waypoints is a number array; TS rebinds the reference,
goscape copies the array; same observable result).

## 7. Cadence & task split

Three implementation tasks + close commit. Two-stage review at T1
(framework + tail) and T3 (AP arm — most complex, surfaces
apRangeCalled-revert + waypoint save/restore).

### T1 — Framework + tail rewrite

**Closes:** B1, B2, partial B5.
**Files:** `modules/world/player.go`, `modules/world/interaction.go`,
`modules/world/interaction_test.go` (or extend an existing test file).

1. Add `nextTarget entity` field to `Player` struct.
2. `processInteraction` entry: insert `p.nextTarget = nil` after the
   followX/followZ writes and before the level-mismatch check.
3. `processInteraction` tail: replace `interaction.go:245-247` block
   with TS L1255-1263's `if/else if`.
4. Tests for B1, B2.

**Review:** code-reviewer audit against TS Player.ts:1200-1263 verbatim.

### T2 — OP helper + wire 4 OP fires

**Closes:** B5 fully, partial B3 (OP variants).
**Files:** `modules/world/interaction.go` (new helper),
`modules/world/interaction_trigger.go` (Loc, Npc, Obj OP fires),
`modules/world/player_interaction_trigger.go` (Player OP fire), tests.

1. Add `runOpTriggerScript` helper to `interaction.go`.
2. Wire into `fireOpTriggerNpc`, `fireOpTriggerLoc`, `fireOpTriggerPlayer`,
   `fireOpTriggerObj`. Drop each helper's Finished/Aborted clear block.
3. Tests: per-entity-type B3 OP variant + B5 dual-pin.

**Review:** standard task review at T2 close.

### T3 — AP helper + wire 4 AP fires + tryInteract AP-branch return

**Closes:** B3 fully, B4.
**Files:** `modules/world/interaction.go` (new helper + tryInteract AP
arm), `modules/world/interaction_trigger.go` (Loc, Npc, Obj AP fires +
`tryFireApTrigger` signature), `modules/world/player_interaction_trigger.go`
(Player AP fire), tests.

1. Add `runApTriggerScript` helper to `interaction.go`.
2. Add `apRangeCalled = false` pre-reset to all 4 AP fire helpers (verify
   AP-Loc already has it, add to AP-Npc/AP-Player/AP-Obj if missing).
3. Wire helper into all 4 AP fires; each returns `apReverted bool`.
4. Change `tryFireApTrigger` to `(apReverted bool)`; collect from
   per-entity helpers.
5. `tryInteract` AP arm: `return !apReverted` propagation.
6. Tests: per-entity-type B3 AP variant + B4.

**Review:** code-reviewer whole-impl audit against TS Player.ts:1140-1170.

### T4 — Close

Update `nai_followups.md`. Net deviation tally 13 → 12.
Close commit body cites TS Player.ts:1113-1170 + 1203 + 1255-1263, all
8 fire-helper sites touched, suspend-state behavior change (Risk 1),
and carries `Closes memory: NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET`.

## 8. Memory entries to apply at plan-write and dispatch

- `runescript_cadence.md` — full cadence, 3 implementation tasks + close.
- `true_to_ts_gate.md` — every behavioral change cited against TS source.
- `enumerate_all_sites.md` — pre-flight grep all 8 fire-helper sites
  + `tryFireApTrigger` call sites + `apRangeCalled = false` pre-set
  occurrences; re-grep post-T2 and post-T3.
- `controller_preflight.md` — verify against HEAD: 8 fire-helper line
  numbers, `apRangeCalled = false` reset placement at
  `interaction_trigger.go:401`, `interaction.go:245-247` tail block,
  `clearWaypoints` absence (we use `p.waypointIndex = -1` directly).
- `plan_grep_helper_patterns.md` — `runOpTriggerScript`/`runApTriggerScript`
  are NEW; verify before T2 no prior helper covers save/clear/restore.
- `plan_helper_coverage.md` — both helpers cross-cut 4 sites each;
  cross-check call-site test coverage before dispatch.
- `plan_test_coverage_crosscheck.md` — diff B1..B5 against each task's
  code block at plan-write time.
- `ts_asymmetry_dual_pin.md` — B3 dual-pins script-set vs no-script-set;
  B4 dual-pins apRangeCalled=true vs false; B5 dual-pins waypoints clear
  presence regardless of nextTarget.
- `audit_full_method_against_ts.md` — at T1 review, audit
  `processInteraction` AND both `tryInteract` arms vs TS
  Player.ts:1113-1180+1255-1263.
- `defensive_gate_doc_comment_label.md` — any goscape-only checks added
  inside the helpers get the "(goscape defensive; TS skips this check)"
  comment.
- `close_commit_memory_trailer.md` — close commit carries
  `Closes memory: NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET`.
- `plan_doc_replaceall_timeline.md` — per-instance Edits across the
  8 fire helpers, never `replace_all`.
- `plan_var_name_collision.md` — `savedTarget`/`savedWP`/`savedIdx`/
  `apReverted` locals checked against the existing `state`/`sf`/
  `category`/`trigger` body locals at each site.
- `verify_implementer_claims.md` — fresh `go test ./...` after each
  task; never trust package-scoped green.
- `feedback_subagent_wt_path.md` — subagent worktree-vs-mainline check
  at every merge.
- `tracker_entry_framing_can_be_incomplete.md` — re-derive the deviation
  framing at brainstorm (done above).
- `latent_bug_at_migration_boundary.md` — keep T1's old-tail-block
  semantics live until T2 wires the helper; nextTarget stays nil
  through T1 because no producer exists yet, so the tail's new
  `if p.nextTarget != nil` branch never fires until T2 lands.

## 9. Spec self-review notes

- **Placeholder scan:** none.
- **Internal consistency:** Section 4.5's per-fire-helper change shape
  matches Section 4.4's helper signatures matches Section 7's task
  decomposition.
- **Scope check:** focused on `*Player`-side `tryInteract` reshape.
  `*Npc.tryInteract` (npc_interaction.go:247) is explicitly out of
  scope per Section 3.
- **Ambiguity check:** "save waypoints" is concrete — `savedWP, savedIdx
  := p.waypoints, p.waypointIndex` (value-copy of `[25]int` array,
  Section 4.4 + Risk 5).
