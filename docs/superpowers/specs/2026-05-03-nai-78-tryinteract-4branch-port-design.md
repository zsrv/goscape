# NAI-78 — Investigation+fix: tryInteract 4-branch port (Tutorial Island RS Guide door)

- **Sub-spec**: NAI-78
- **Date**: 2026-05-03
- **Scope label**: Investigation-and-fix sub-spec — Stage 1 short-circuited at brainstorm time (root cause concrete and grep-confirmed at HEAD `2fa3c17`: `modules/world/interaction.go::tryInteract` (lines 310-343) implements only 2 of TS `Player.tryInteract`'s 4 dispatch branches and does NOT gate the AP fire on `apTrigger != null`. Adjacent loc with `[oploc<n>]` registered but no `[aploc<n>]` — the canonical door pattern — falls into the AP block, the no-script path inside `fireApTriggerLoc` (`interaction_trigger.go:411-415`) calls `ClearInteraction` + `interactionFired = true`, and `tryInteract` returns `true`. Pre-step's `interacted=true` gates the post-step branch off, so OPLOC never fires and the player never paths). Stage 2 = single-file refactor of `tryInteract` to mirror TS Player.ts:1113-1184 4-branch dispatch + defaultOp NIH port + retire the now-dead AP/OP no-script branches in fire helpers. User-mediated Java-client smoke (per `smoke_test_server_handoff.md`) as binding feature-correctness gate. Bundle 3 conditional template (LOC opcode ports) drafted for the likely 2nd-order surface.
- **Predecessors**: NAI-77 (handleMoveClick port) — last on `main` as `2fa3c17`
- **Source root**:
  - `LostCityRS/Engine-TS` (TS canonical for `pkg/script` and `modules/world` per `ts_source_canonical_path.md`)
    - `src/engine/entity/Player.ts:1113-1184` — `tryInteract(allowOpScenery)` 4-branch dispatch
    - `src/engine/entity/Player.ts:966-998` — `getOpTrigger()` resolution (target type-switch + targetSubject.com override + ScriptProvider.getByTrigger)
    - `src/engine/entity/Player.ts:1000-1032` — `getApTrigger()` resolution (mirror of `getOpTrigger`, no `+7` offset)
    - `src/engine/entity/Player.ts:1072-1097` — `defaultOp()` (NIH "Nothing interesting happens." + clearWaypoints; optional dev-only debugname log NODE_PRODUCTION-gated)
    - `src/engine/entity/Player.ts:1200-1264` — `processInteraction()` caller (no change needed; pre/post-step tryInteract calls already mirror TS structure)
  - `LostCityRS/Content/scripts/tutorial/scripts/tut_doors_and_gates.rs2:39-50` — `[oploc1,newbie_door1]` (RS Guide door; downstream consumer)
  - `LostCityRS/Content/scripts/doors/scripts/open_and_close_doors.rs2:9-40` — `[proc,open_and_close_door]` (uses LOC_PARAM, LOC_COORD, LOC_ANGLE, LOC_SHAPE, LOC_CHANGE, LOC_ADD, MOVECOORD, P_TELEPORT, P_DELAY, SOUND_SYNTH — opcode-gap surface for Bundle 3)
  - `LostCityRS/Content/scripts/doors/scripts/door_procs.rs2:77-88` — `[proc,check_axis]` (uses LOC_COORD, LOC_ANGLE, COORDX, COORDZ — already-dispatched coord ops; door-side LOC_COORD/LOC_ANGLE gap)

## Motivation

NAI-77 closed the chatnpc click-away cascade (handleMoveClick port). The user re-smoked at HEAD `b9fb524` and confirmed:

- Symptom-2 (click-away modal dismiss): PASS — NAI-77 silenced cleanly.
- Symptom-1 (RS Guide door at Tutorial Island): FAIL, unchanged from NAI-76 smoke. **OPLOC1 packet received on the server, but no visible client effect (no walk, no chat, no action menu update). Player does NOT move at all.**

Per `nai_followups.md` "From NAI-77 → NAI-78 candidate" + `cascade_theory_smoke_binding.md`: the door is independent of the click-away cascade and warrants a fresh investigation+fix sub-spec.

Three hypotheses inherited from NAI-76/77 carry-forward:

1. State-gate trip: tut_open chain now completes post-NAI-76 → some state advance now trips a `[proc]` gate that pre-NAI-76 was bypassed.
2. Pre-NAI-76 reading was imprecise: "doesn't move at all" is steady state; the earlier "walks past door" reading was an artifact.
3. Door's `[oploc1]` body invokes a script opcode goscape doesn't dispatch — the `no handler for X` error is now louder/different post-NAI-76.

Bundle 0 controller pre-flight (per `controller_preflight.md` + `investigation_subspec_cadence.md`) reads `tut_doors_and_gates.rs2:39-50` + transitive proc deps, traces goscape's OPLOC dispatch from `handler_oploc.go::handleOpLoc1` through `interaction.go::processInteraction` → `tryInteract` → `tryFireApTrigger`/`tryFireOpTrigger`, and diffs against TS `Player.ts:1113-1184`. Verdict: **smoking gun is a goscape engine divergence, not a content/script gap.** H1 is refuted (root cause is independent of `%tutorial`); H2 is adopted (steady-state behavior matches the engine bug); H3 remains possible as a 2nd-order surface that smoke will expose post-fix.

## Stage 1 short-circuit evidence

Re-grep at brainstorm time against HEAD `2fa3c17`.

### Goscape `tryInteract` shape (`modules/world/interaction.go:310-343`)

```go
func (p *Player) tryInteract(allowOpScenery bool) bool {
    tx, tz, _ := p.target.Coords()
    if inOperableDistance(p.x, p.z, tx, tz) {
        _, isNpc := p.target.(*Npc)
        _, isPlayer := p.target.(*Player)
        if isNpc || isPlayer || allowOpScenery {
            p.interacted = true
            if !p.interactionFired {
                tryFireOpTrigger(p)
            }
            return true
        }
        // Loc/Obj + !allowOpScenery: fall through to AP check.
    }
    if inApproachDistance(p.x, p.z, tx, tz, effectiveApRange(p)) {
        p.interacted = true
        if !p.interactionFired {
            tryFireApTrigger(p)
        }
        if p.nextTarget == nil && p.apRangeCalled {
            p.interactionFired = false
            return false
        }
        return true
    }
    return false
}
```

Two branches: OP (gated on `Npc/Player || allowOpScenery`) and AP (gated on `inApproachDistance` only). The AP branch fires `tryFireApTrigger` unconditionally on approach distance — does NOT gate on apTrigger existence.

### Goscape `fireApTriggerLoc` no-script path (`interaction_trigger.go:399-450`)

```go
sf := srv.scriptProvider.GetByTrigger(trigger, resolveTriggerTypeId(p, loc.Type()), category)
if sf == nil {
    p.ClearInteraction()
    p.interactionFired = true
    return
}
```

When no AP script registered, the helper consumes the interaction (target=nil) and marks fired=true. Identical no-script branches present in `fireApTriggerNpc` (`interaction_trigger.go:340-345`), `fireApTriggerObj` (`interaction_trigger.go:455-470` est.), and `fireApTriggerPlayer` (NAI-40-era).

### TS `tryInteract` shape (`Engine-TS/.../Player.ts:1113-1184`)

Resolves `opTrigger = getOpTrigger()` and `apTrigger = getApTrigger()` ONCE at entry (lines 1118-1119), then dispatches via 4 mutually-exclusive `if/else if` branches:

| Branch | Line | Gate | Body | Returns |
|---|---|---|---|---|
| 1 — OP fire | 1123 | `opTrigger && (PathingEntity \|\| allowOpScenery) && operable` | save target, clear waypoints, executeScript(opTrigger), restore target+nextTarget | `true` |
| 2 — AP fire | 1139 | `apTrigger && approach` | reset apRangeCalled, save target+waypoints, executeScript(apTrigger), restore; if apRangeCalled w/o nextTarget → restore waypoints + return `false` | `true` (or `false` on apRangeCalled retry) |
| 3 — default-AP no-op | 1173 | `approach` (apTrigger null) | `apRange = -1` | **`false`** |
| 4 — default-OP NIH | 1179 | `target && (PathingEntity \|\| allowOpScenery) && operable` (opTrigger null) | `defaultOp()` → MessageGame("Nothing interesting happens.") + clearWaypoints | `true` |

### Door symptom trace at HEAD

Player adjacent to `newbie_door1` (`[oploc1]` registered, `[aploc1]` NOT registered). `apRange = 10` (default from `SetInteraction`). `inOperableDistance(px,pz,tx,tz) = true` (Chebyshev ≤ 1). `inApproachDistance(...) = true` (1 ≤ 10).

Pre-step `tryInteract(allowOpScenery=false)`:

1. inOperableDistance=true; not Npc/Player and allowOpScenery=false → fall through to AP block (interaction.go:322 comment).
2. inApproachDistance=true → `tryFireApTrigger(p)`:
   - `fireApTriggerLoc` resolves `sf := GetByTrigger(TriggerApLoc1, …) == nil`.
   - Sets `ClearInteraction` (target=nil, apRange=10, apRangeCalled=false, …) + `interactionFired=true`. Returns.
3. Back in tryInteract AP arm: `nextTarget==nil && apRangeCalled==false` → fall to `return true`.

Pre-step returns `true` → `interacted=true` → post-step branch gated off (`if !interacted` at interaction.go:208).

Auto-clear at processInteraction tail: `interacted && !apRangeCalled` → `ClearInteraction()` (idempotent — already cleared by fire-helper).

**Result: zero waypoints generated, zero scripts dispatched, OPLOC1 trigger never fires, player never moves.** Symptom shape matches.

### TS path on the same scenario

Pre-step: branch 1 fails (Loc not PathingEntity, allowOpScenery=false). Branch 2 fails (`apTrigger=null`). **Branch 3 fires: `apRange=-1`, return `false`.** → `!interacted` → post-step runs.

Post-step: `pathToTarget(tx,tz)` (no-op when already at target), `walktrigger` no-op, `tryInteract(stepsTaken==0=true)` with `allowOpScenery=true`. Branch 1: `opTrigger && (Loc instanceof PathingEntity || true) && operable` → TRUE → fires OPLOC1. ✓

Conclusion: Stage 1 short-circuit conclusive. **No subagent audit dispatch needed.** Mirrors NAI-75/NAI-76 short-circuit pattern.

## Architecture

Single-file refactor of `modules/world/interaction.go::tryInteract` plus targeted retirements in `modules/world/interaction_trigger.go` fire helpers.

### A — `getOpTrigger` / `getApTrigger` resolution helpers (`modules/world/interaction_trigger.go`, new pkg-level funcs adjacent to existing `apLocTriggerForOp`)

Mirror TS `Player.ts:966-1032`. Single function each that:

1. Returns `nil` if `p.target == nil`.
2. Type-switches on `p.target` to derive `(typeId, categoryId)` from the entity's type registry (LocType / NpcType / ObjType). Player target → typeId=-1, categoryId=-1.
3. If `p.targetSubject.com != -1`, override `typeId = targetSubject.com` (TS `Player.ts:993-995`/`1027-1029`).
4. Resolve trigger constant per target+op via existing `apLocTriggerForOp`/`apNpcTriggerForOp`/`apObjTriggerForOp`/`apPlayerTriggerForOp` (or equivalents). For OP, add `+7` offset.
5. Call `srv.scriptProvider.GetByTrigger(trigger, typeId, categoryId)` and return.

```go
// getOpTrigger resolves the [op<entity><op>,<typeId>] script for the
// player's anchored target. Mirrors LostCityRS/Engine-TS Player.ts:966-998.
// Returns nil if no script registered or target is unsupported.
func getOpTrigger(p *Player, srv *Server) *script.ScriptFile { ... }

// getApTrigger resolves the [ap<entity><op>,<typeId>] script. Mirror of
// getOpTrigger without the +7 offset. Mirrors Player.ts:1000-1032.
func getApTrigger(p *Player, srv *Server) *script.ScriptFile { ... }
```

Both helpers must reproduce the existing per-target-type category resolution shape (Loc reads `srv.locTypes.Configs[typeID].Category`; Npc reads `npc.typ.Category` from the cached pointer; Obj/Player as the existing fire helpers do). Plan-author MUST grep `apLocTriggerForOp`/`apNpcTriggerForOp`/`apObjTriggerForOp`/`apPlayerTriggerForOp` + each fire helper's category-resolution block before codifying — `plan_grep_helper_patterns.md`.

### B — `tryInteract` 4-branch rewrite (`modules/world/interaction.go:310-343`)

Replace the existing 2-branch body with a TS-faithful 4-branch dispatch:

```go
func (p *Player) tryInteract(allowOpScenery bool) bool {
    if p.target == nil {
        return false
    }
    s := p.client.server

    opTrigger := getOpTrigger(p, s)
    apTrigger := getApTrigger(p, s)

    tx, tz, _ := p.target.Coords()
    operable := inOperableDistance(p.x, p.z, tx, tz)
    approach := inApproachDistance(p.x, p.z, tx, tz, effectiveApRange(p))

    isPathing := false
    switch p.target.(type) {
    case *Npc, *Player:
        isPathing = true
    }

    // Branch 1 — OP fire (TS Player.ts:1123).
    if opTrigger != nil && (isPathing || allowOpScenery) && operable {
        p.interacted = true
        if !p.interactionFired {
            tryFireOpTrigger(p, opTrigger)
        }
        return true
    }

    // Branch 2 — AP fire (TS Player.ts:1139).
    if apTrigger != nil && approach {
        p.interacted = true
        if !p.interactionFired {
            tryFireApTrigger(p, apTrigger)
        }
        // NAI-69 same-tick AP retry signal: aprange called, no p_op_*.
        if p.nextTarget == nil && p.apRangeCalled {
            p.interactionFired = false
            return false
        }
        return true
    }

    // Branch 3 — default-AP no-op (TS Player.ts:1173-1175).
    // Player is in approach distance but no [ap…] script exists.
    // Force apRange to -1 so the AP block can never re-enter, then
    // return false to let processInteraction's post-step branch run
    // (pathToTarget → walktrigger → post-step tryInteract with
    // allowOpScenery=true so branch 1 can fire OP).
    if approach {
        p.apRange = -1
        return false
    }

    // Branch 4 — default-OP NIH (TS Player.ts:1179-1182).
    // Player is in operable distance but no [op…] script exists.
    // Emit "Nothing interesting happens." + clear waypoints.
    if (isPathing || allowOpScenery) && operable {
        defaultOp(p)
        return true
    }

    return false
}
```

Key invariants preserved from HEAD:
- NAI-69 same-tick AP retry (`apRangeCalled` + `interactionFired=false` + `return false`) inside branch 2.
- The pre-step caller (`processInteraction:205`) passes `allowOpScenery=false` unchanged; post-step caller (`processInteraction:228`) passes `stepsTaken==0` unchanged.
- The `effectiveApRange(p)` call (S6l/NAI-69) feeds into `inApproachDistance` exactly once per tryInteract call.

### C — `defaultOp` helper (`modules/world/interaction.go`, new pkg-level func adjacent to tryInteract)

```go
// defaultOp implements the NIH (Not-Implemented-Here) fallback fired by
// tryInteract branch 4 when the player reaches operable distance but no
// [op…] script is registered. Mirrors LostCityRS/Engine-TS
// Player.ts:1072-1097 (defaultOp).
//
// Skips the NODE_PRODUCTION-gated dev "No trigger for [...]" debug
// message — goscape has no equivalent dev/prod flag and the chat-only
// path matches all known production-mode TS behavior.
func defaultOp(p *Player) {
    p.MessageGame("Nothing interesting happens.")
    p.waypoints = nil
    p.waypointIndex = -1
}
```

Plan-author MUST verify `Player.MessageGame` signature (`grep -n "func .* MessageGame" modules/world/`) and `waypoints`/`waypointIndex` field shape before codifying. The `waypoints = nil + waypointIndex = -1` pattern matches existing `clearWaypoints`-equivalent sites at `interaction_trigger.go:130,180,369,403`. If a `clearWaypoints()` helper has landed since prior NAI work, prefer it.

### D — Fire helper signature change + retire no-script branches (`modules/world/interaction_trigger.go`)

`tryFireOpTrigger` and `tryFireApTrigger` accept the resolved `*script.ScriptFile` from tryInteract:

```go
func tryFireOpTrigger(p *Player, sf *script.ScriptFile) { ... }
func tryFireApTrigger(p *Player, sf *script.ScriptFile) { ... }
```

Each `fireOpTriggerX` / `fireApTriggerX` (8 helpers total: Op×4 entity types + Ap×4 entity types) takes the resolved script as a parameter and **retires its internal `sf := scriptProvider.GetByTrigger(...) ; if sf == nil { ... return }` block**. The `apLocTriggerForOp`/`apNpcTriggerForOp`/`apObjTriggerForOp` `ok=false` branches (e.g. `interaction_trigger.go:64-69, 135-140, 326-331, 410-415`) are also retired — tryInteract pre-resolves and won't enter the fire path with an unsupported targetOp.

Lifecycle gates (`npc.dead` / `locStillValid` / `objStillValid` / Player target validity) are PRESERVED — these defend against between-tick state mutation that's invisible to tryInteract's resolution.

The `fireOpTriggerLoc` "Nothing interesting happens." NIH fallback at `interaction_trigger.go:156-164` (S6j-D7) is **retired** in favor of branch 4's `defaultOp(p)` — they produce identical observable behavior, but branch 4 is the TS-faithful site and avoids double-emission.

Plan-author MUST enumerate all 8 fire helpers + every test fixture that calls them with the old 1-arg signature; signature change is mechanical but blast-radius requires `grep -n "fireOpTrigger\|fireApTrigger\|tryFireOpTrigger\|tryFireApTrigger" modules/world/` before plan-write — `enumerate_all_sites.md`.

### Shape summary

```
modules/world/interaction.go         (~+50 LOC, ~-25 LOC tryInteract body)
  + tryInteract — 4-branch rewrite
  + defaultOp helper

modules/world/interaction_trigger.go (~+90 LOC helpers, ~-100 LOC retired branches)
  + getOpTrigger
  + getApTrigger
  ~ tryFireOpTrigger — accepts sf
  ~ tryFireApTrigger — accepts sf
  ~ fireOpTriggerLoc/Npc/Player/Obj — accept sf, retire no-script branch
  ~ fireApTriggerLoc/Npc/Player/Obj — accept sf, retire no-script branch
```

Net: ~150-200 LOC change. Test fixture updates: ~50 LOC. Net deviation tally: 15 → 14 (closes the untracked tryInteract divergence; defaultOp is in-scope so no NAI-78-D opens).

## Test strategy

### `modules/world/interaction_test.go` (new) — `tryInteract` 4-branch matrix

Six regression tests (one per TS branch + two retry edge cases). Fixture: spawn fresh server with one player + one Loc target adjacent (or in approach range). Use existing `makeOpLocFixture` family if present (grep `interaction_trigger_nai68_test.go`/`interaction_trigger_test.go` for the existing helper).

| Test | Setup | Expectation |
|---|---|---|
| `TestTryInteract_OpFires_AdjacentLoc_OpScriptOnly` | adjacent Loc; `[oploc1]` registered; no `[aploc1]`; pre-step `allowOpScenery=false` | pre-step returns `false`, `apRange == -1` (branch 3); post-step `allowOpScenery=true` returns `true` (branch 1); OP script ran exactly once |
| `TestTryInteract_OpFires_AdjacentLoc_BothScripts` | adjacent Loc; both `[oploc1]` + `[aploc1]` registered; pre-step `allowOpScenery=false` | pre-step returns `true` (branch 2 AP fires); post-step skipped; AP script ran exactly once |
| `TestTryInteract_OpFires_AdjacentNpc` | adjacent Npc; `[opnpc1]` registered; no `[apnpc1]`; pre-step `allowOpScenery=false` | pre-step returns `true` (branch 1, isPathing=true); OP script ran exactly once |
| `TestTryInteract_DefaultAp_NoScripts_PathingEntity` | adjacent Npc; no AP no OP; pre-step | branch 4 fires `defaultOp`; `MessageGame("Nothing interesting happens.")` emitted; `waypointIndex == -1` |
| `TestTryInteract_DefaultAp_NoScripts_Loc` | approach-range-only Loc (2 tiles away); no AP no OP; pre-step `allowOpScenery=false` | branch 3 fires; pre-step returns `false`; `apRange == -1` after; post-step `allowOpScenery=true` reaches branch 4; `defaultOp` emitted (or no-op if not in operable yet) |
| `TestTryInteract_AprangeRetry_PreservesNAI69` | adjacent Loc; `[aploc1]` script that calls `p_aprange(2)`; no OP | NAI-69 retry: pre-step returns `false` after branch 2 (apRangeCalled=true); waypoints preserved; processInteraction's post-step re-evaluates with new range |

Plan-author MUST verify the NAI-69 retry test at HEAD `b9fb524` — re-run `TestTryInteract_…AprangeRetry…` (if the test exists with another name) and ensure the new branch-2 shape (`apRangeCalled` check after `tryFireApTrigger` return, before `return true`) preserves NAI-69 semantics. `nai_followups.md` confirms NAI-69-D-APPLAYER-SELF2-REVERSED-NO-SAMETICK-RETRY is independent of this work.

### `modules/world/interaction_test.go` — `defaultOp` direct test

```go
func TestDefaultOp_EmitsNIHAndClearsWaypoints(t *testing.T) {
    // setup: player with non-empty waypoints
    p := …
    p.waypoints = []waypoint{…}
    p.waypointIndex = 0

    defaultOp(p)

    // assert MessageGame called with "Nothing interesting happens."
    // (drain client write buffer; pin payload bytes)
    // assert p.waypointIndex == -1
}
```

### Fire-helper signature regression

Every existing test that calls `tryFireOpTrigger(p)` / `tryFireApTrigger(p)` / `fireOpTriggerX` / `fireApTriggerX` with the 1-arg signature MUST be updated to pass an explicitly-constructed (or fixture-resolved) `*script.ScriptFile`. Plan-author enumerates: grep `modules/world/*_test.go` for `tryFire(Op|Ap)Trigger\|fire(Op|Ap)Trigger(Loc|Npc|Player|Obj)` and lists all call sites before T-N. `enumerate_all_sites.md`.

### Retired-branch test cleanup

Tests pinning the retired no-script branches inside fire helpers (e.g., `TestFireApTriggerLoc_NoScriptClearsInteraction` if it exists) MUST be retired or repointed to the new tryInteract-level branches 3/4. Plan-author MUST grep `_test.go` for `sf == nil`, `no.*script.*registered`, `ClearInteraction` assertions in fire-helper tests and enumerate.

### Cross-package regression

`go test ./... -count=1` plus `-race` per `verify_implementer_claims.md`. Stale IDE diagnostics ignored per failure-mode-1.

## Smoke matrix + decision tree

Server launch (per `smoke_test_server_handoff.md`, user-driven):

```
CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file config.yaml
```

**Items:**

1. **Door click registers movement** — log in fresh; talk to RS Guide enough to advance `%tutorial` past `^newbie_basics_instructor_interact_with_scenery` (existing chatnpc flow per NAI-75); click wooden door at the RS Guide room exit. Pass = player walks toward door (path generation visible) AND OPLOC1 trigger fires (script execution evidence in server log or in-game effect).
2. **Door full path** — same as item 1, but assert the full `[oploc1,newbie_door1]` body completes: door visually opens, player walks through, `~tutorial_step_moving_around` chatbox appears with `~chatnpcrange` rendering, `%tutorial` advances to `^newbie_basics_instructor_interacted_with_door`.
3. **No-trigger NIH fallback** — out-of-scope items (a random in-zone loc with no oploc/aploc registered) produce "Nothing interesting happens." chat instead of silent no-op.

**Decision tree at smoke close:**

| Outcome | Route |
|---|---|
| 1+2+3 all pass | NAI-78 closes single-fix. Mirror NAI-75/77 close-commit shape. |
| 1 passes, 2 fails with script error in log (no handler for LOC_PARAM/LOC_COORD/LOC_ANGLE/LOC_SHAPE/LOC_CHANGE/LOC_ADD/SOUND_SYNTH at known pc) | **Bundle 3 conditional materializes** — see §Bundle 3 template below. Per `investigation_subspec_cadence.md`, smoke surfaces 2nd-order layered bug; route to a fresh stage of fix work in this same sub-spec. |
| 1 passes, 2 fails with no script error (silent no-op, partial walk, etc.) | Bundle 3 alternate path: runtime instrumentation per `investigation_subspec_cadence.md` Stage 3 template. Add gated logs to fireOpTriggerLoc + the door's [oploc1] body entry; re-smoke. |
| 1 fails | Stage 2 fix has a defect (signature mismatch, branch 3 not actually returning false, NAI-69 retry broken). Re-investigate. |
| 3 fails alone | Branch 4 not wired correctly; in-scope-stretch fix (~5 LOC) per `smoke_surfaces_adjacent_divergences.md`. |

## Bundle 3 conditional template (LOC opcode ports)

**Materializes only if smoke item 2 fails with a script-error log line.** Pre-templated here so the controller can dispatch quickly without re-architecture.

The `[oploc1,newbie_door1]` body + transitive procs (`~check_axis`, `~open_and_close_door`, `~door_open`) require these script opcodes that are NOT in `pkg/script/handlers.go` dispatch map at HEAD `2fa3c17`:

| Opcode | Const | TS handler | Goscape gap |
|---|---|---|---|
| 3000 | `OpLocAdd` | `LocOps.ts::LOC_ADD` | constant only |
| 3001 | `OpLocAngle` | `LocOps.ts::LOC_ANGLE` | constant only |
| 3003 | `OpLocCategory` | `LocOps.ts::LOC_CATEGORY` | constant only |
| 3004 | `OpLocChange` | `LocOps.ts::LOC_CHANGE` | constant only |
| 3005 | `OpLocCoord` | `LocOps.ts::LOC_COORD` | constant only |
| 3006 | `OpLocDel` | `LocOps.ts::LOC_DEL` | constant only |
| 3008 | `OpLocFindAllZone` | `LocOps.ts::LOC_FINDALLZONE` | constant only |
| 3009 | `OpLocFindNext` | `LocOps.ts::LOC_FINDNEXT` | constant only |
| 3011 | `OpLocParam` | `LocOps.ts::LOC_PARAM` | constant only |
| 3012 | `OpLocShape` | `LocOps.ts::LOC_SHAPE` | constant only |
| 3013 | `OpLocType` | `LocOps.ts::LOC_TYPE` | constant only |
| 2104 | `OpSoundSynth` | `ServerOps.ts::SOUND_SYNTH` | constant only |

Direct minimum for the RS Guide door's gate-pass path (`%tutorial >= ^newbie_basics_instructor_interact_with_scenery`):

- `LOC_PARAM` (`loc_param(next_loc_stage)`)
- `LOC_COORD` / `LOC_ANGLE` / `LOC_SHAPE` (read `~check_axis` + `~open_and_close_door`)
- `LOC_CHANGE` (`loc_change(inviswall, 3)`)
- `LOC_ADD` (`loc_add(...)`)
- `SOUND_SYNTH` (`sound_synth(door_open, 0, 0)`)

Bundle 3 task split:

- **B3-T1**: read-only opcodes (`LOC_PARAM`, `LOC_COORD`, `LOC_ANGLE`, `LOC_SHAPE`, `LOC_CATEGORY`, `LOC_TYPE`) — pure stack-ops over `ScriptState.ActiveLoc`. ~80 LOC + ~60 LOC tests.
- **B3-T2**: `LOC_CHANGE` (in-place Info bitfield mutation; consumer of existing `loc_lookup.go` mechanisms) + `LOC_ADD` / `LOC_DEL` (zoneMap.Add/Remove + duration-based revert) — touches `modules/world/loc_lookup.go` + new `Server.AddLoc`/`Server.ChangeLoc`/`Server.DelLoc` infra. ~120 LOC + ~80 LOC tests. **Highest risk** — `loc_add` with duration semantics needs a tick-revert scheduler (TS `World.ts` `LocList`/`zone.locs` tick-watcher equivalent). Plan-author must `grep -n "locList\|locReverts\|zoneTick" modules/world/` to enumerate existing scheduler infra at Bundle 3 time.
- **B3-T3**: `LOC_FINDALLZONE` / `LOC_FINDNEXT` (zone iterator pattern per `iterator_state_pattern.md`) — only needed for `~tut_island_survial_gate` and `~tut_open_mining_gate` procs, NOT `[oploc1,newbie_door1]` directly. **Defer** unless smoke item 2 hits a `LOC_FINDALLZONE` error specifically.
- **B3-T4**: `SOUND_SYNTH` (server-emit OpSoundSynth wire packet to player). ~20 LOC + ~15 LOC tests.

If Bundle 3 materializes, expect a ~250-300 LOC sub-task running over 1-2 implementer dispatches. May warrant rolling forward to NAI-79 if scope explodes — controller-level decision at smoke-fail time per `cadence_compression_for_tiny_subspecs.md`/scope-gate discipline.

## Out of scope (deferred)

- **APLOC/APNPC fallback for non-default scripts** — branch 2 AP fire mechanics already match TS; not touched.
- **`hasInteraction()` / `canAccess()` TS gates** at TS Player.ts:1114 — goscape's `tryInteract` entry has no equivalent (target-nil check elsewhere). Audit deferred — preserve current shape; revisit if a smoke surfaces a missing-gate symptom.
- **Bundle 3 LOC ops** — conditional, only on smoke item 2 fail.
- **TUT_CLOSE (opcode 2120) handler + `Player.closeTutorial()`** — still deferred from NAI-76.
- **`World.ts:624-628 moveClickRequest` per-tick assignment** — still deferred from NAI-77.
- **Carry-forwards from NAI-74/75/76/77** — `NAI-75-D-FONT-WRAP-NAIVE`, `NAI-75-D-MESANIM-NOT-PORTED`, `NAI-77-D-WALKTRIGGER-FALLBACK-PHASE-CHOICE`, `NAI-72-D-FRIENDS-SERVER-BRIDGE`, `NAI-72-D-LOGIN-SERVER-BRIDGE-MOD`, `NAI-69-D-APPLAYER-SELF2-REVERSED-NO-SAMETICK-RETRY`, `NAI-67-D-PLAYER-UNFOCUS-DEFERRED`, `NAI-34-D4-NPC` + `NAI-34-D5-NPC`, `NAI-35-T3-D1`. All untouched.

## Deviations

**Closed (untracked at HEAD; closed-as-fix):**

- `goscape engine: tryInteract 2-branch instead of TS 4-branch` (untracked at HEAD; closed by §B). Symptom-bearer for NAI-78. Fix mechanism: branch 2 gated on `apTrigger != nil`; branches 3 + 4 added; defaultOp NIH ported. Net: 1 untracked divergence closed.
- `goscape engine: AP-no-script consumes interaction` (untracked at HEAD; closed by §D retirement of fire-helper no-script branches). Symptom-bearer (joint with above). Net: 1 untracked divergence closed (subsumed in the above; 1 logical fix).

**Opened:** None (defaultOp NIH is in-scope per §B branch 4 + §C; no new tracked deviation).

**Net deviation tally:** 15 → 14 (close the untracked tryInteract+AP-fire divergence; +0 new).

If the controller decides to defer defaultOp NIH at plan-write time (e.g., to compress the change), open `NAI-78-D-DEFAULTOP-NIH-NOT-PORTED` with closure path = "Add 4-line `defaultOp(p)` helper + branch 4 wiring; closed at NAI-N+1". Tally would be 15 → 15 in that variant.

## Cadence reminders for plan-author + controller

- `controller_preflight.md`: re-grep + Read every file/line/symbol assertion in §A/B/C/D against HEAD before T1 dispatch. Especially: `apLocTriggerForOp`/`apNpcTriggerForOp` line numbers, `fireOpTriggerLoc`/`fireApTriggerLoc` body shape, `ClearInteraction` semantics, `Player.MessageGame` signature, `waypoints`/`waypointIndex` field names.
- `enumerate_all_sites.md`: list every caller of `tryFireOpTrigger`/`tryFireApTrigger`/`fireOpTriggerX`/`fireApTriggerX` (production + tests) BEFORE plan dispatch. Signature change is mechanical but exhaustive.
- `plan_helper_coverage.md`: the new `getOpTrigger`/`getApTrigger` helpers must replicate every per-target-type branch in the existing fire-helper resolution code; cross-check Loc/Npc/Player/Obj against the 4 fire-helper resolution blocks at §A.
- `verify_implementer_claims.md`: every commit verified via `git show --stat` + fresh `go test ./... -count=1` + `-race` from a clean shell.
- `cascade_theory_smoke_binding.md`: smoke item 2 may PARTIALLY pass (player walks to door, no further effect) — that's the Bundle 3 trigger, not a NAI-78 close gate.
- `investigation_subspec_cadence.md`: 4th instance (after NAI-31, NAI-75, NAI-76) of the Stage-1-short-circuit + Stage-2-fix + smoke pattern. Bundle 3 conditional template pre-drafted (§Bundle 3 template).
- `latent_bug_at_migration_boundary.md`: tryInteract is on the engine path that fires every interaction; smoke must check more than the door (existing OPLOC/OPNPC/OPOBJ/OPPLAYER paths must regress green). NAI-69 retry semantics are the highest-risk regression vector.
