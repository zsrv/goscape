# NAI-44 — Full TS executeScript binding semantics + followOp chase port

## Motivation

Three deviations clustered around the player/script lifecycle have been deferred since their respective sub-specs landed. NAI-44 closes all three by re-aligning goscape's script-resume dispatch with TS, and by porting the chase-the-target shape of TS `Player.processInteraction`.

Closures targeted:

- **`NAI-37-D-WORLDQUEUE-CROSS-CONTEXT-DROP`** (`modules/world/script.go:171-177`). When a world-queued script transitions to `Suspended` / `NpcSuspended` / `PauseButton` / `CountDialog`, goscape currently warns and drops. The deviation comment claims TS rebinds these "implicitly to the corresponding entity's activeScript (Player.ts:2137-2141, Npc.ts:221-225)". That framing is **mis-attributed**: TS `World.processWorld` (World.ts:530-560) does NOT call `Player.executeScript`; it dispatches inline with only 3 explicit arms (`SUSPENDED`/`NPC_SUSPENDED`/`WORLD_SUSPENDED`), and `PauseButton`/`CountDialog` fall through silently after `request.unlink()`. Per `tracker_entry_framing_can_be_incomplete.md`, the deviation is fact-correct (we drop) but framing-wrong; the actual TS shape requires only 2 rebinds (Suspended→Self, NpcSuspended→ActiveNpc), with Pause/Count matching TS's silent fall-through.

- **`NAI-37-D-WORLDSUSPEND-CLEARS-ACTIVE-SCRIPT`** (`modules/world/script.go:124-130`, `modules/world/npc_script.go:312`). The `WorldSuspended` arm of `resumeOrFinish` and `resumeOrFinishNpc` defensively calls `ClearActiveScript()`. TS Player.executeScript (Player.ts:2143-2150) and Npc.executeScript (Npc.ts:226-228) only null `activeScript` on `FINISHED` / `ABORTED` (and only when `script === this.activeScript`). The defensive clear is removed.

- **`NAI-40-D-OPPLAYER3-FOLLOWOP-NOT-PORTED`** (`modules/world/handler_op_player.go:26`). TS Player.processInteraction (Player.ts:1200-1264) computes `followOp = (targetOp == APPLAYER3 || targetOp == OPPLAYER3)` and gates four branches on it: pre-step walktrigger skip (L1219-1221), post-step waypoint-exhaustion clear (L1237-1239), and post-step interact skip (L1244-1252). Goscape's `processInteraction` (interaction.go:104-150) lacks the followOp predicate and the structural shape that hosts it; it also lacks the global auto-clear at TS L1261-1263 (`if interacted && !apRangeCalled → clearInteraction`).

The OPPLAYER3 closure forces a re-shape of `processInteraction` to mirror TS Player.ts:1200-1264 structurally. This is the **TS-faithful** approach (Approach 2 from brainstorming, vs. a surgical-followOp Approach 1) chosen because: (a) the global auto-clear lays clean rails for `NAI-40-SB1` (OPCALLED) and the eventual walktrigger consumer; (b) goscape's processInteraction has accumulated shape-divergence that compounds as more triggers land; (c) the cascade-audit cost is bounded (~10 existing tests, all in-tree).

Pre-NAI-44 behavior:
- World-queued scripts that transition to Suspended/NpcSuspended/Pause/Count are dropped with a warn (cross-context resume unsupported).
- WorldSuspended branches of player/npc resume defensively null the entity's `activeScript` slot (vs. TS's only-Finished/Aborted clear).
- OPPLAYER3 op-clicks fire the trigger once and forget; no chase semantics; the player loses interaction-anchor state inconsistently with TS.
- `processInteraction` never auto-clears post-fire — interactions stay anchored until something external (script opcode, level mismatch, packet) clears them.

Post-NAI-44 behavior:
- World-queued scripts rebind cross-context per TS World.processWorld L547-559 (Suspended→Self, NpcSuspended→ActiveNpc, Pause/Count silent drop matching TS).
- WorldSuspended branches preserve `activeScript` per TS executeScript.
- OPPLAYER3 op-clicks enter chase mode: walktrigger-skipped, post-step-interact-skipped, waypoint-exhaustion clears interaction.
- `processInteraction` auto-clears post-successful-fire per TS L1261-1263 (gated by `!apRangeCalled` and `!followOp`-via-skipped-post-step-interact-arm).

## Tech stack

- **Go 1.26+** (per `go_version.md`; use modern Go syntax via the `use-modern-go` skill).
- TS source: `Engine-TS` only per `ts_source_canonical_path.md`.
  - `src/engine/entity/Player.ts:1200-1264` (processInteraction with followOp branches).
  - `src/engine/entity/Player.ts:2125-2151` (executeScript dispatch table — reference only; goscape's resumeOrFinish is the analogue, NOT a direct port).
  - `src/engine/entity/Npc.ts:216-239` (Npc.executeScript dispatch — reference for the Npc-path resume).
  - `src/engine/World.ts:530-560` (`processWorld` world-queue dispatch — the canonical shape for `resumeOrFinishWorld`).
- Existing infrastructure (verified at HEAD `8451cb0`):
  - `(p *Player).targetOp int` field set in `SetInteraction` (interaction.go:56), reset to -1 in `ClearInteraction` (interaction.go:90). The followOp predicate reads this directly: `p.targetOp == 3` for OPPLAYER3-class clicks.
  - `(p *Player).apRangeCalled bool` field set/reset by `SetInteraction`/`ClearInteraction` (interaction.go:60, 92) and mutated by AP-trigger handlers when the script extends `apRange`. Used by the auto-clear gate at TS L1261-1263.
  - `(p *Player).interacted bool` and `(p *Player).interactionFired bool` (player.go:117, set in interaction.go:61, 63 + 93-95). `interactionFired` is per-anchor-cycle (blocks re-fire); `interacted` is the per-tick flag goscape currently sets in operable/approach branches.
  - `tryFireOpTrigger(p)` and `tryFireApTrigger(p)` exist (`player_interaction_trigger.go`); `tryInteract` will fold their callers into a shared helper.
  - `inOperableDistance` (interaction.go:156), `inApproachDistance` (interaction.go:176), `effectiveApRange` (interaction.go:211), `pathToTarget` (interaction.go:223) — kept as-is.
  - `(p *Player).pathToMoveClick` (modules/world/movement.go area, plan-author confirms exact location) — the per-tick step consumer; the `stepsTaken` increment site.
  - `(s *Server).resumeOrFinishWorld` at `modules/world/script.go:159-183` — the rewrite target for Section 1.
  - `(p *Player).StoreActiveScript(state)` and `(n *Npc).StoreActiveScript(state)` already exist (npc.go:217). The world-queue rebind dispatches through these.
  - `state.Self` (interface `script.ActivePlayer`) and `state.ActiveNpc` (interface `script.ActiveNpc`) carry the bind targets through the world queue.
  - `(p *Player).MessageGame(text string)` for the "I can't reach that!" message at TS L1249.
  - `(p *Player).hasWaypoints()` — **plan-author audit**: confirm helper exists or add a 2-line one (`return len(p.waypoints) > 0` or whatever the queue field is named).
- Memory entries applied:
  - `tracker_entry_framing_can_be_incomplete.md` — drives the re-frame of `NAI-37-D-WORLDQUEUE-CROSS-CONTEXT-DROP` in the close commit body.
  - `enumerate_all_sites.md` — drives the cascade-audit task (T6).
  - `controller_preflight.md` — drives the pre-T5 grep verification (the highest-blast-radius task).
  - `latent_bug_at_migration_boundary.md` — drives the clean-cutover-then-fix shape for T5/T6 (vs. dual-path migration).
  - `plan_enumerate_struct_literals.md` — drives the field-default audit if `stepsTaken`/`nextTarget`/`followX`/`followZ` get added (any new field needs all `Player{` literal sites enumerated).
  - `dead_api_polish.md` — drives the `NAI-44-D-CONTINUEWALK-UNUSED` tag (close at next sub-spec polish if no consumer materializes).

## Scope

**In scope:**

### A. Script-lifecycle alignment

A1. **Drop defensive `ClearActiveScript()` in `WorldSuspended` arms.**
- `modules/world/script.go:117-133` (`resumeOrFinish` WorldSuspended arm): delete `self.ClearActiveScript()` at L133 and the entire `// DEVIATION NAI-37-D-WORLDSUSPEND-CLEARS-ACTIVE-SCRIPT: ...` comment block at L124-130.
- `modules/world/npc_script.go:307-318` (`resumeOrFinishNpc` WorldSuspended arm): symmetric delete.
- TS reference: Player.ts:2143-2150 only nulls `this.activeScript` on FINISHED/ABORTED (and only when `script === this.activeScript`).

A2. **Port TS World.processWorld L547-559 dispatch into `resumeOrFinishWorld`.**

Replace `modules/world/script.go:171-177` with a TS-faithful 3-arm switch:

```go
case script.Suspended:
    // TS World.ts:548-549 — bind to script.activePlayer.activeScript.
    // The "(probably not needed)" TS comment notes this case isn't
    // expected from world-queued scripts in practice, but the binding
    // exists for completeness.
    if state.Self != nil {
        state.Self.StoreActiveScript(state)
    } else {
        s.log.Warn("world-queue script Suspended with nil Self; dropping",
            "script", state.Script.Name)
    }
case script.NpcSuspended:
    // TS World.ts:550-552 — bind to script.activeNpc.activeScript.
    if state.ActiveNpc != nil {
        state.ActiveNpc.StoreActiveScript(state)
    } else {
        s.log.Warn("world-queue script NpcSuspended with nil ActiveNpc; dropping",
            "script", state.Script.Name)
    }
case script.PauseButton, script.CountDialog:
    // TS World.processWorld (World.ts:530-560) has NO branch for these
    // states. request.unlink() at L545 already removed the entry, so
    // they are silently dropped. Match TS by intentionally falling
    // through with no rebind and no warn.
default:
    // Running, or any future-added Execution value.
    s.log.Warn("world-queue script in unexpected execution state",
        "script", state.Script.Name, "execution", state.Execution)
```

A3. **Re-frame `NAI-37-D-WORLDQUEUE-CROSS-CONTEXT-DROP` in the close commit body.** The deviation tag is retired; the close commit notes that the original framing ("TS rebinds Pause/Count cross-context") was incorrect — TS World.processWorld also drops Pause/Count silently.

### B. TS-faithful processInteraction port

B1. **Add Player struct fields (foundation, no behavior change).**

Three new fields on `*Player` (modules/world/player.go):

- `stepsTaken int` — count of waypoints consumed this tick. Reset at tick start, incremented per step. Plan-author audit: locate the per-tick step consumer (likely `pathToMoveClick` or per-tick movement step) and add `p.stepsTaken++` at the consumption site. Reset point: tick-start in `server.go`/`tick.go`.
- `nextTarget entity` — set by `p_op*` script opcodes for next-tick target swap. **Goscape currently does immediate swap** in `p_op_loc`/`p_op_npc` (player_script.go:623 area). This field is added for TS-shape parity and read by the new `processInteraction`'s nextTarget block, but no producer wires it; tag `NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET`.
- `followX int`, `followZ int` — set from `lastStepX/Z` at tick start (TS L1201-1202). Plan-author audit: if goscape lacks `lastStepX/Z` fields, stub these as `p.x, p.z` and tag `NAI-44-D-NO-LAST-STEP-COORDS`.

Per `plan_enumerate_struct_literals.md`: enumerate all `Player{` struct literals at plan-author time and add explicit zero-values where needed (Go's zero-value default covers `int`/interface but explicit init at test fixtures avoids surprise).

B2. **Add `processWalktrigger` no-op stub on `*Player`** (modules/world/interaction.go):

```go
// processWalktrigger is the per-tick walktrigger consumption hook
// invoked by processInteraction on the pre-step and post-step arms.
//
// DEVIATION NAI-44-D-PLAYER-WALKTRIGGER-NOOP: TS Player.ts:1219-1234
// calls processWalktrigger which dispatches to the player's queued
// walktrigger script. Goscape has no walktrigger consumer yet
// (sibling to NAI-37-D-WALKTRIGGER-NOREADER on the Npc side). Empty
// no-op preserves TS-faithful processInteraction shape so the
// consumer can be wired here without further reshape.
func (p *Player) processWalktrigger() {}
```

B3. **Add `tryInteract` helper consolidating the OP/AP distance dispatch:**

```go
// tryInteract is the contact/approach-distance dispatch unifying the
// OP and AP arms that processInteraction previously inlined. Returns
// true when an OP or AP trigger fired this tick.
//
// continueWalk is reserved for TS Player.ts:1245's stepsTaken-aware
// retry timing; goscape's per-tick movement order makes this a no-op
// (the post-step arm only runs once anyway). Tagged
// NAI-44-D-CONTINUEWALK-UNUSED for symmetry-with-TS provenance.
func (p *Player) tryInteract(continueWalk bool) bool {
    tx, tz, _ := p.target.Coords()
    if inOperableDistance(p.x, p.z, tx, tz) {
        p.interacted = true
        if !p.interactionFired {
            tryFireOpTrigger(p)
        }
        return true
    }
    if inApproachDistance(p.x, p.z, tx, tz, effectiveApRange(p)) {
        p.interacted = true
        if !p.interactionFired {
            tryFireApTrigger(p)
        }
        return true
    }
    _ = continueWalk
    return false
}
```

B4. **Reshape `processInteraction`** to mirror TS Player.ts:1200-1264:

```go
func (p *Player) processInteraction() {
    if p.target == nil {
        return
    }
    if p.client == nil || p.client.server == nil {
        return
    }
    s := p.client.server
    if p.delayed && s.currentTick < p.delayedUntil {
        return
    }

    // TS L1201-1202.
    p.followX = p.lastStepX // or p.x if NAI-44-D-NO-LAST-STEP-COORDS
    p.followZ = p.lastStepZ
    p.nextTarget = nil

    // TS L1205. Goscape predicate uses the raw op slot since p.targetOp
    // is 1..4 (the op slot, NOT a ServerTriggerType enum). APPLAYER3
    // and OPPLAYER3 share op slot 3 in TS; goscape stores op slot
    // directly, so a single equality check covers both.
    _, targetIsPlayer := p.target.(*Player)
    followOp := p.targetOp == 3 && targetIsPlayer

    // Existing level-mismatch guard (subset of TS validateTarget).
    _, _, tlevel := p.target.Coords()
    if tlevel != p.level {
        p.ClearInteraction()
        sendUnsetMapFlag(p)
        return
    }

    interacted := false

    // Pre-step interact arm (TS L1209-1224).
    // canAccess() is approximated by !p.delayed; goscape has no
    // stun/freeze. DEVIATION NAI-44-D-CANACCESS-NO-STUN-CHECK
    // (conditional, plan-author confirms).
    if !p.delayed {
        if !followOp {
            p.processWalktrigger()
        }
        interacted = p.tryInteract(false)
    }

    // Post-step arm (TS L1227-1252).
    if !interacted {
        // Recalc path (TS L1228-1229).
        if !p.repathed {
            tx, tz, _ := p.target.Coords()
            p.pathToTarget(tx, tz)
            p.repathed = true
        }

        if p.hasWaypoints() && !p.delayed {
            p.processWalktrigger()
        }

        // followOp + waypoint exhaustion → clear (TS L1237-1239).
        if !p.hasWaypoints() && followOp {
            p.ClearInteraction()
        }

        // updateMovement: TS embeds inline at L1241; goscape's
        // updateMovement is called from server.go's tick loop. Order-
        // of-operations differs by design.

        if p.target != nil && !p.delayed && !followOp {
            interacted = p.tryInteract(p.stepsTaken == 0)
            if !interacted && !p.hasWaypoints() && p.stepsTaken == 0 {
                p.MessageGame("I can't reach that!")
                p.ClearInteraction()
            }
        }
    }

    // nextTarget swap (TS L1255-1258).
    // DEVIATION NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET: goscape's p_op*
    // opcodes do immediate SetInteraction swaps; nextTarget is always
    // nil here. Block kept for TS-shape parity.
    if p.nextTarget != nil {
        p.target = p.nextTarget
    } else if interacted && !p.apRangeCalled {
        // Auto-clear after successful interaction (TS L1261-1263).
        p.ClearInteraction()
    }
}
```

B5. **Retire deviation tags at completion:**
- `NAI-37-D-WORLDQUEUE-CROSS-CONTEXT-DROP` doc-comment block at `modules/world/script.go:153-157` and `:172-177`.
- `NAI-37-D-WORLDSUSPEND-CLEARS-ACTIVE-SCRIPT` doc-comment block at `modules/world/script.go:124-130` and `modules/world/npc_script.go:312-` (along with the deletion of the call itself).
- `NAI-40-D-OPPLAYER3-FOLLOWOP-NOT-PORTED` doc-comment block at `modules/world/handler_op_player.go:26-30`.
- Cross-reference matches in `world_script_queue_test.go:257-266` (existing test comments) and any plan-doc references — re-grep `rg "NAI-37-D-WORLDQUEUE-CROSS-CONTEXT-DROP|NAI-37-D-WORLDSUSPEND-CLEARS-ACTIVE-SCRIPT|NAI-40-D-OPPLAYER3-FOLLOWOP-NOT-PORTED" pkg/ modules/` at retirement time per `retire_deviation_grep_all_comments.md`.

### C. Cascade audit

C1. **Auto-clear blast-radius enumeration.** Adding TS L1261-1263 auto-clear changes the post-fire interaction-anchor lifetime for ALL existing op triggers (OpLoc, OpNpc, OpObj, OpPlayer1/2/4). Plan-author T6 grep:

```
rg -l "p\.target\s*!=\s*nil|p\.target\s*==\s*nil" modules/world/*_test.go
```

…and re-validates each assertion against the new auto-clear behavior. Per `enumerate_all_sites.md`, enumerate in the plan, verify post-commit by re-running the grep at HEAD.

Expected affected sites: tests that assert `p.target != nil` after a single `processInteraction` cycle. Likely fixes:
- Re-frame to assert `p.interactionFired == true` and `p.target == nil` (the new post-cycle shape).
- For tests pinning the anchored-target behavior on out-of-range targets, no change (auto-clear gate is `interacted && !apRangeCalled` — only fires on successful contact/approach).

**Out of scope:**

- **OPCALLED flag convergence (`NAI-40-SB1`)** — consumer-port-blocked on World.ts:613-642 userPath/opcalled branch.
- **Walktrigger consumer port** (`NAI-37-D-WALKTRIGGER-NOREADER`) — Npc-side and Player-side (the new `NAI-44-D-PLAYER-WALKTRIGGER-NOOP`); both are stubbed only.
- **`p_op*` opcode reshape** to use `nextTarget` — touches every script-side opcode handler (`p_op_loc`, `p_op_npc`, `p_op_obj`, etc.).
- **Modal close on script finish** (TS Player.ts:2146-2149) — goscape's modal/dialog substrate is incomplete; if plan-author confirms no closeModal-on-finish path elsewhere, tag `NAI-44-D-NO-MODAL-CLOSE-ON-SCRIPT-FINISH` (deferral, not closure).
- **canAccess stun/freeze checks** — goscape has no stun system; the `!p.delayed` approximation is the in-tree subset. Tag `NAI-44-D-CANACCESS-NO-STUN-CHECK` if confirmed absent.
- **Loc/Obj `targetX/Z` (`NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ`)** — orthogonal; closed by the focus/step-tracking sub-spec.
- **`updateMovement` re-ordering to TS-inline** — goscape's tick-loop architecture differs by design.

## Tracked deviations

**Closed by NAI-44 (3):**

| Tag | Site at HEAD `8451cb0` | Closure mechanism |
|---|---|---|
| `NAI-37-D-WORLDQUEUE-CROSS-CONTEXT-DROP` | `modules/world/script.go:171-177` + comment at `:153-157` | A2: TS-faithful 3-arm switch in `resumeOrFinishWorld`. Re-framed in close commit body (TS World.processWorld also drops Pause/Count). |
| `NAI-37-D-WORLDSUSPEND-CLEARS-ACTIVE-SCRIPT` | `modules/world/script.go:124-130`, `modules/world/npc_script.go:312` | A1: delete defensive `ClearActiveScript()` at both sites. |
| `NAI-40-D-OPPLAYER3-FOLLOWOP-NOT-PORTED` | `modules/world/handler_op_player.go:26-30` | B4: followOp predicate + chase-the-target branches in `processInteraction`. |

**Opened by NAI-44 (4 tracked + up to 2 conditional):**

| Tag | Site | Closure plan |
|---|---|---|
| `NAI-44-D-PLAYER-WALKTRIGGER-NOOP` | `(p *Player).processWalktrigger()` in `interaction.go` | Bundles with existing `NAI-37-D-WALKTRIGGER-NOREADER` future sub-spec (deferred item #4). |
| `NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET` | `nextTarget`-block in `processInteraction` | Future `p_op*` opcode reshape sub-spec — touches every `p_op_loc/npc/obj` handler. |
| `NAI-44-D-NO-LAST-STEP-COORDS` (conditional) | `followX`/`followZ` assignment in `processInteraction` | Conditional on `lastStepX/Z` not existing at HEAD. Bundles with `pathing-entity-focus-and-step-tracking` (deferred item #1). |
| `NAI-44-D-CONTINUEWALK-UNUSED` | `_ = continueWalk` in `tryInteract` | Conditional. Closes if a TS L1245 retry-after-step consumer surfaces; otherwise dead-API-polish at next sub-spec close per `dead_api_polish.md`. |
| `NAI-44-D-CANACCESS-NO-STUN-CHECK` (conditional) | `if !p.delayed { ... }` arm in `processInteraction` | Opened only if plan-author confirms goscape has no stun/freeze check. Likely yes. |
| `NAI-44-D-NO-MODAL-CLOSE-ON-SCRIPT-FINISH` (conditional) | `resumeOrFinish` Finished/Aborted arm | Opened only if plan-author confirms no closeModal-on-script-finish substrate elsewhere. |

**Net deviation tally:** 3 closed + 4 to 6 opened = +1 to +3 net. Aligns with `true_to_ts_gate.md`: every divergence has a tracked tag with rationale + future closure path.

## Test plan

| Bucket | Tests | File |
|---|---|---|
| **A1 — WorldSuspended no-clear** | 2 (player branch + npc branch); pin `self.activeScript == state` after `EnqueueWorldScript` | `script_test.go`, `npc_script_test.go` |
| **A2 — World-queue cross-context rebind** | 4 (Suspended→Self bind; NpcSuspended→ActiveNpc bind; PauseButton silent drop; CountDialog silent drop) | `world_script_queue_test.go` |
| **A3 — Suspended-then-WorldSuspended regression** | 1 — pin no double-execution from same state-pointer being held by both player slot and world queue | `script_test.go` |
| **B1 — followOp predicate** | 3 — `targetOp=3 + *Player` → followOp; `targetOp=3 + *Npc` → not followOp; `targetOp=1 + *Player` → not followOp | `interaction_test.go` |
| **B2 — followOp anchored chase** | 1 — out-of-range OPPLAYER3, after one cycle: `p.target != nil`, waypoints set, no auto-clear | `interaction_test.go` |
| **B3 — followOp waypoint exhaustion** | 1 — no waypoints + followOp → `ClearInteraction` called | `interaction_test.go` |
| **B4 — followOp contact fire** | 1 — operable range + OPPLAYER3 → OP trigger fires; auto-clear gate evaluates per `interacted && !apRangeCalled` (re-read TS L1261-1263 at test-write time) | `interaction_test.go` |
| **B5 — non-followOp baseline (regression)** | 2 — OPPLAYER1 (op=1) and OPLOC1 (different target type), pin behavior under new shape | `interaction_test.go` |
| **B6 — auto-clear cascade** | TBD by plan-author grep — prescribed list of `processInteraction`-touching existing tests, each re-validated; expected ≤10 | `interaction_test.go`, `npc_test.go`, others |
| **B7 — processWalktrigger no-op** | 1 — call doesn't panic, returns immediately, has no observable effect on Player state | `interaction_test.go` |

Roughly **15-20 new tests + ≤10 cascade-audit updates**.

## File map

**Modified:**

- `modules/world/script.go` — A1 (delete defensive clear + comment block) + A2 (replace warn+drop with 3-arm switch).
- `modules/world/npc_script.go` — A1 symmetric delete.
- `modules/world/interaction.go` — B2 (`processWalktrigger`), B3 (`tryInteract`), B4 (reshape `processInteraction`).
- `modules/world/player.go` — B1 (3-4 fields: `stepsTaken`, `nextTarget`, `followX`, `followZ`).
- `modules/world/handler_op_player.go` — B5 (retire deviation doc-comment block).

**Possibly modified (audit-conditional):**

- `modules/world/server.go` or `modules/world/tick.go` — `stepsTaken` per-tick reset + `nextTarget = nil` if not in `processInteraction`.
- `modules/world/movement.go` (or wherever per-tick step consumption lives) — `stepsTaken++`.
- `modules/world/player.go` — `hasWaypoints()` helper if absent.

**Test files modified:**

- `modules/world/script_test.go`
- `modules/world/npc_script_test.go`
- `modules/world/world_script_queue_test.go`
- `modules/world/interaction_test.go`

## Cadence shape (commit plan)

Following `runescript_cadence.md`'s 2-5 commits / sub-spec band. Six tasks proposed for the plan doc (T1-T6); the plan-author may consolidate T1+T2 into a single Area-A task at their discretion.

- **T1** — Area A.1: drop defensive `ClearActiveScript` (player + npc branches). Tests A1. ~30 LOC.
- **T2** — Area A.2: port World-queue cross-context rebind (TS-faithful 3-arm switch). Tests A2 + A3. ~80 LOC.
- **T3** — Area B.1: Player struct fields + `processWalktrigger` stub + `hasWaypoints` helper if needed. Tests B7. ~50 LOC. (Foundation, no behavior change.)
- **T4** — Area B.2: `tryInteract` extraction (refactor `processInteraction`'s distance branches into shared helper). Tests in B5. ~80 LOC. (Refactor, no behavior change.)
- **T5** — Area B.3: `processInteraction` reshape with followOp + auto-clear (the real behavior change). Tests B1 + B2 + B3 + B4. ~120 LOC. **Highest blast-radius task — controller pre-flight grep mandatory** per `controller_preflight.md`.
- **T6** — Cascade-audit + close commit. Plan-author grep + per-affected-test update; retire deviation comments at `modules/world/handler_op_player.go` + `script.go`; bump `nai_followups.md`; close-commit with `Closes memory:` trailer per `close_commit_memory_trailer.md`. Tests B6.

Two-stage review per task (spec-compliance → code-quality, both opus) per `runescript_cadence.md`. Auto-mode collapses self-approvals to autonomous.

**Estimated total LOC:** ~330-500 (production: ~150-200; tests: ~180-300). Sits at the upper edge of the cadence's target band — runs ~2-2.5h of subagent work per `runescript_cadence.md`.

## Risks

- **R1: Auto-clear cascade.** T5 changes when goscape clears interactions; existing OpLoc/OpNpc/OpObj tests may assume "anchored after fire." Mitigation: T6 enumerate-and-fix per `enumerate_all_sites.md`; clean-cutover-then-fix per `latent_bug_at_migration_boundary.md`.
- **R2: `stepsTaken` integration site.** If goscape's per-tick step accounting lacks a clean increment site, B1 ships a write-zero-only field. Mitigation: plan-author audit; downgrade to tagged stub if necessary.
- **R3: `lastStepX/Z` absence.** If goscape lacks these fields, `followX/Z` get stubbed; tag `NAI-44-D-NO-LAST-STEP-COORDS`. Mitigation: tag-and-defer per `dead_api_polish.md`.
- **R4: `hasWaypoints` helper absence.** Trivial 2-line port; flagged for plan-author audit.
- **R5: Suspended-then-WorldSuspended double-fire.** Section A1's regression test must verify that the same `state` pointer being held by both the player slot and the world queue does NOT cause double-execution. Plan-author confirms goscape's tick-loop ordering: does `processActiveScripts` run before or after `processWorldQueue`?
- **R6: Re-framing the `WORLDQUEUE-CROSS-CONTEXT-DROP` deviation.** The original deviation comment misattributed the TS shape. Close commit body must re-derive the actual TS dispatch from primary source per `tracker_entry_framing_can_be_incomplete.md`.

## References

- `tracker_entry_framing_can_be_incomplete.md` — drives R6 re-frame.
- `enumerate_all_sites.md` — drives T6 cascade-audit.
- `controller_preflight.md` — drives T5 pre-dispatch verification.
- `latent_bug_at_migration_boundary.md` — drives clean-cutover shape.
- `plan_enumerate_struct_literals.md` — drives field-default audit.
- `dead_api_polish.md` — drives conditional-deviation closure plans.
- `retire_deviation_grep_all_comments.md` — drives B5 retirement enumeration.
- `close_commit_memory_trailer.md` — drives T6 trailer convention.
- `runescript_cadence.md` — drives the 6-task / two-stage-review shape.
- `ts_source_canonical_path.md` — `Engine-TS` only.
- `go_version.md` — Go 1.26+ + `use-modern-go` skill.
- `audit_full_method_against_ts.md` — drives the full TS L1200-1264 method audit (vs. just porting the followOp predicate).
- `ts_base_class_read_for_inherited_behavior.md` — drives the executeScript-vs-World.processWorld distinction (the cross-context dispatch lives in World, not Player.executeScript).
