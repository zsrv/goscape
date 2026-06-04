# NAI-68 — `p_op*` immediate→nextTarget reshape

**Status:** Spec written 2026-05-02.
**Predecessor:** NAI-67 (HEAD `bbaec8d`). Net deviation tally entering: 13.
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

Mirror TS Player.ts:1126-1163 + 1203 + 1255-1263 so:

1. Script-set `nextTarget` survives the auto-clear and applies on the next
   tick (TS L1133, L1155, L1255-1258).
2. The OP branch's TS L1131 `clearWaypoints()` runs (currently absent in
   goscape).
3. AP branch saves+restores waypoints around script execution and clears
   them when the script set a `nextTarget` (TS L1145-1162).
4. The fire-helper Finished/Aborted eager-clear is dropped in favor of
   the tail's `else if interacted && !apRangeCalled` block (consolidates
   clear-on-fire to one site).

**Out of scope for this sub-spec — TS L1166-1170 apRangeCalled revert
(same-tick post-step retry).** Goscape's existing AP fire helpers
simulate `apRangeCalled` persistence via an *across-tick* mechanism:
early-return without setting `interactionFired = true` so the AP re-fires
on a later tick after the player walks closer (interaction_trigger.go:415-422
for AP-Loc, mirrored in AP-Obj at line 586). Adopting TS L1166-1170's
same-tick retry path requires reworking goscape's `interactionFired`
guard (which is a goscape-specific re-fire prevention) to avoid same-tick
infinite loops. The two mechanisms are logically equivalent for player
experience but mutually exclusive at the state-machine level.

Closes `NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET`. Opens
`NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED` (TS L1166-1170 same-tick retry +
`interactionFired`-guard rework). Net tally: 13 → 13.

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

### 4.4 Inline save/clear/capture/restore at each fire helper

No extracted helpers. The save/clear/capture/restore pattern is
inlined at each of the 8 fire helpers because:

1. The pattern is short (4-5 lines) and TS-citation-anchored at each
   site, so inlining keeps the TS-line-mapping locally grep-discoverable.
2. The OP and AP shapes diverge (AP saves waypoints; OP doesn't), and
   inlining sidesteps a "shared helper that's wrong for half the callers"
   risk.
3. Goscape's existing AP-Loc apRangeCalled across-tick re-fire (early
   return without `interactionFired = true`) is preserved verbatim —
   trying to wrap that into a helper conflates two responsibilities.

The OP-side inline shape (TS L1129-1135):

```go
// Save target before script run (TS L1129).
savedTarget := p.target
p.target = nil
p.waypointIndex = -1 // TS L1131: this.clearWaypoints()

srv.resumeOrFinish(state, p)

// Capture script-set target into nextTarget; restore original (TS L1133-1134).
p.nextTarget = p.target
p.target = savedTarget

p.interactionFired = true
```

The AP-side inline shape (TS L1145-1162) — preserves goscape's existing
apRangeCalled across-tick re-fire branch:

```go
p.apRangeCalled = false // TS L1141 — pre-set required at all 4 AP sites.

// Save target + waypoints before script run (TS L1145, L1146).
savedTarget := p.target
savedWP := p.waypoints
savedIdx := p.waypointIndex
p.target = nil
p.waypointIndex = -1

srv.resumeOrFinish(state, p)

// Capture script-set target into nextTarget; restore target.
p.nextTarget = p.target
p.target = savedTarget

if p.nextTarget != nil {
    // TS L1162: clear destination so next-tick interaction starts fresh.
    p.waypointIndex = -1
} else {
    // No script-set target — restore waypoints (TS L1146 inverse).
    p.waypoints = savedWP
    p.waypointIndex = savedIdx
}

// Existing apRangeCalled across-tick re-fire branch — UNCHANGED.
// DEVIATION NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED: TS L1166-1170 does
// same-tick post-step retry; goscape uses early-return-without-fired
// for next-tick re-fire. Equivalent for player experience.
if state.Execution == script.Finished || state.Execution == script.Aborted {
    if p.apRangeCalled {
        p.repathed = false
        return // interactionFired stays false → re-fire next tick.
    }
}

p.interactionFired = true
```

Goscape's `[25]int` waypoints array is copied by value in the local —
the array is small and trivially copyable. Restoring both array+index
matches TS's `this.waypoints = wayPoints; this.waypointIndex = waypointIndex`
exactly.

The Finished/Aborted ClearInteraction in the existing AP-Loc helper
(line 426) and AP-Obj helper (corresponding site) is **dropped** — the
processInteraction tail's `else if interacted && !apRangeCalled` block
(Section 4.3) covers it.

### 4.5 Per-fire-helper changes (8 sites)

**4 OP fires** (`fireOpTriggerNpc` at interaction_trigger.go:53,
`fireOpTriggerLoc` at interaction_trigger.go:119, `fireOpTriggerPlayer`
at player_interaction_trigger.go:42, `fireOpTriggerObj` at
interaction_trigger.go:477):

Replace the existing tail block:

```go
srv.resumeOrFinish(state, p)
if state.Execution == script.Finished || state.Execution == script.Aborted {
    p.ClearInteraction()
}
p.interactionFired = true
```

with the OP inline shape from Section 4.4:

```go
savedTarget := p.target
p.target = nil
p.waypointIndex = -1 // TS L1131

srv.resumeOrFinish(state, p)

p.nextTarget = p.target
p.target = savedTarget

p.interactionFired = true
```

The Finished/Aborted clear is dropped — subsumed by the tail's
`else if interacted && !apRangeCalled` (which fires after every successful
trigger fire because `interacted=true` and goscape never sets
`apRangeCalled=true` in OP scripts).

**4 AP fires** (`fireApTriggerNpc` at interaction_trigger.go:298,
`fireApTriggerLoc` at interaction_trigger.go:359, `fireApTriggerPlayer`
at player_interaction_trigger.go:79, `fireApTriggerObj` at
interaction_trigger.go:537):

The pre-script `apRangeCalled = false` reset is currently explicit at
`interaction_trigger.go:401` (AP-Loc) and `interaction_trigger.go:571`
(AP-Obj). Verify presence on AP-Npc and AP-Player; add if missing.

Replace the existing tail block:

```go
srv.resumeOrFinish(state, p)
if state.Execution == script.Finished || state.Execution == script.Aborted {
    if p.apRangeCalled {
        p.repathed = false
        return
    }
    p.ClearInteraction()
}
p.interactionFired = true
```

with the AP inline shape from Section 4.4 (preserves the apRangeCalled
across-tick re-fire branch; drops the eager ClearInteraction).

`tryFireApTrigger` signature is **unchanged**. `tryInteract` AP-arm
is **unchanged**.

### 4.6 Lifecycle/lookup-failure clears (unchanged)

The `npc.dead`, `!locStillValid`, no-trigger, invalid-op, and
`delayed && currentTick < delayedUntil` early-clears inside each fire
helper run BEFORE script init. No script means no `nextTarget` to
preserve. Keep these as immediate `ClearInteraction()` + `interactionFired = true`.

## 5. Test plan

Four testable behaviors. Each pinned with new tests; existing tests
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

### B5 — OP branch's TS L1131 `clearWaypoints()` add (dual-pin)

Pre-set `p.waypointIndex >= 0` (active path). Run a `processInteraction`
that fires an OP trigger. Assert `p.waypointIndex == -1` post-call,
both when the script sets a `nextTarget` (B5a) and when it doesn't
(B5b). The dual-pin pre-empts a future regression that conditionalizes
the clear on script behavior.

### B6 — AP TS L1162 conditional waypoint clear

When an AP trigger script sets a `nextTarget` (calls `p_op_npc` etc.),
assert `p.waypointIndex == -1` post-call (TS L1162). When it does NOT
set a nextTarget AND does not call `p_aprange`, assert
`p.waypointIndex == savedIdx` (waypoints restored — TS L1146 inverse).
Two-test dual-pin.

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

### Risk 2: `tryFireApTrigger` signature unchanged (de-risked)

Original spec considered changing `tryFireApTrigger` to return
`apReverted`. Final design leaves the signature alone — the
across-tick re-fire mechanism in goscape's existing AP fire helpers
already preserves `apRangeCalled` interaction state. No call-site
churn; tests at `interaction_trigger_test.go:439, 459, 500, 527, 545,
563, 578, 695, 714, 1114, 1136, 1185` and
`player_interaction_trigger_test.go:153` and `handler_opobj_test.go:720, 741`
all continue to compile unchanged.

### Risk 3: Implementer drifting inline save/clear/restore across 8 sites

Each fire helper inlines the same ~8-line save/clear/exec/capture/restore
shape, varying only by OP-vs-AP. Drift risk: implementer rewrites the
shape slightly differently at each site. Plan codifies the EXACT inline
text once for OP and once for AP (Section 4.4 + 4.5); per-site Edits
copy verbatim. Plan code blocks include adjacent TS L-citation comments;
reviewer checks one-to-one mapping at T2 and T3 close.

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

**Closes:** B1, B2.
**Files:** `modules/world/player.go`, `modules/world/interaction.go`,
`modules/world/interaction_test.go` (or extend an existing test file).

1. Add `nextTarget entity` field to `Player` struct.
2. `processInteraction` entry: insert `p.nextTarget = nil` after the
   followX/followZ writes and before the level-mismatch check.
3. `processInteraction` tail: replace `interaction.go:245-247` block
   with TS L1255-1263's `if/else if`.
4. Tests for B1, B2.

**Review:** code-reviewer audit against TS Player.ts:1200-1263 verbatim.

### T2 — Wire 4 OP fires (inline save/clear/restore)

**Closes:** B3 (OP variants), B5.
**Files:** `modules/world/interaction_trigger.go` (Loc, Npc, Obj OP
fires), `modules/world/player_interaction_trigger.go` (Player OP fire),
tests.

1. Inline OP save/clear/restore at `fireOpTriggerNpc` (interaction_trigger.go:53),
   `fireOpTriggerLoc` (interaction_trigger.go:119), `fireOpTriggerPlayer`
   (player_interaction_trigger.go:42), `fireOpTriggerObj`
   (interaction_trigger.go:477). Drop Finished/Aborted ClearInteraction.
2. Tests: per-entity-type B3 OP variant + B5 dual-pin.

**Review:** standard task review at T2 close.

### T3 — Wire 4 AP fires (inline save/clear/restore + waypoints)

**Closes:** B3 (AP variants), B6.
**Files:** `modules/world/interaction_trigger.go` (Loc, Npc, Obj AP
fires), `modules/world/player_interaction_trigger.go` (Player AP fire),
tests.

1. Verify `apRangeCalled = false` pre-reset on all 4 AP fire helpers
   (already at AP-Loc:401 and AP-Obj:571; add to AP-Npc and AP-Player
   if missing).
2. Inline AP save/clear/restore + nextTarget-conditional waypoint clear
   at `fireApTriggerNpc` (interaction_trigger.go:298), `fireApTriggerLoc`
   (interaction_trigger.go:359), `fireApTriggerPlayer`
   (player_interaction_trigger.go:79), `fireApTriggerObj`
   (interaction_trigger.go:537). Drop Finished/Aborted ClearInteraction
   (the Finished/Aborted-with-apRangeCalled early-return path stays).
3. Tests: per-entity-type B3 AP variant + B6 dual-pin.

**Review:** code-reviewer whole-impl audit against TS Player.ts:1140-1162.

### T4 — Close

Update `nai_followups.md`. Net deviation tally 13 → 13 (closes
NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET; opens
NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED). Close commit body cites TS
Player.ts:1126-1163 + 1203 + 1255-1263, all 8 fire-helper sites
touched, suspend-state behavior change (Risk 1), and carries
`Closes memory: NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET`.

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
