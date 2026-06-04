# NAI-77 — `handleMoveClick` port: close modals on walk-click

**Date:** 2026-05-03
**Status:** Spec
**Branch:** main
**HEAD at spec-write:** `1505bfa` (NAI-76 close)

---

## 1. Motivation

NAI-76 silenced the per-login `[proc,tutorialstep_page]` runtime error
(item 1 PASS) but partially refuted the cascade theory: post-NAI-76
smoke confirmed two residual symptoms. Per
`cascade_theory_smoke_binding.md`, each symptom is treated independently
and routed via the decision tree.

NAI-77 covers the **click-away modal-dismiss** symptom, which has a
grep-confirmed concrete root cause and does NOT require a Stage-1 audit.
The door-interaction symptom (different root) is routed to NAI-78.

**Symptom (smoke evidence):** With a chatnpc dialog open, clicking
ground tiles does not close the dialog. NAI-75 chatnpc rendering works
and NAI-76 tut_open succeeds, yet click-away is broken.

**Root cause:** `modules/world/handlers_game.go:193` `handleMoveClick`
decodes the wire payload and calls `pathToMoveClick`, but is missing
the body of TS `MoveClickHandler.ts:43-55` — specifically the
`!opClick` branch that calls `player.clearPendingAction()`. In
goscape, `ClearPendingAction` (`player_script.go:824-829`) calls
`CloseModal(true)`, which is the trigger that dismisses the chatnpc
dialog on walk-click. Adding the missing body fires that trigger.

**Scope discipline:** True-to-TS-fidelity scope (per
`true_to_ts_gate.md`). Full port of `MoveClickHandler.ts` semantics
including the three currently-missing gates (delayed, ctrlHeld bound,
distance bound), opClick differentiation via wrapper pattern, and
`WalkTriggerSetting` enum + per-tick fallback site in `World.ts:635-641`.

---

## 2. Tech Stack

- Go 1.26+ (per `go_version.md`)
- Test framework: standard `testing` package
- TS reference: `LostCityRS/Engine-TS` (per `ts_source_canonical_path.md`)
  - Primary: `src/network/game/client/handler/MoveClickHandler.ts`
  - Adjacent: `src/network/game/client/codec/MoveClickDecoder.ts`,
    `src/engine/entity/WalkTriggerSetting.ts`,
    `src/engine/entity/Player.ts:741-794` (closeModal),
    `src/engine/World.ts:624-641` (per-tick fallback)
  - Constants: `src/util/Environment.ts:53` (default
    `WalkTriggerSetting.PLAYERPACKET`)

---

## 3. Scope

### In scope

1. Port `MoveClickHandler.ts` body to goscape via wrapper pattern:
   - New inner `moveClickInner(p *Player, payload []byte, opClick bool)`
   - Two thin wrappers: `handleMoveGameClick` (opClick=false) and
     `handleMoveOpClick` (opClick=true)
   - Rewire `gameHandlers[181]` and `gameHandlers[93]` accordingly
2. Three gates (currently absent): delayed → UnsetMapFlag; ctrlHeld
   range + distance > 104 → unsetMapFlag + clear userPath;
3. `!opClick` body: `ClearPendingAction()` (which fires `CloseModal`),
   tempRun update with `runenergy<100 && ctrlHeld==1` → 0 sub-rule,
   `processWalktrigger` gated on PLAYERPACKET + hasWaypoints.
4. `WalkTriggerSetting` enum (3 values: PLAYERPACKET=0, PLAYERSETUP=1,
   PLAYERMOVEMENT=2) + new world config field defaulting to PLAYERPACKET.
5. Per-tick fallback site (TS `World.ts:635-641`): when
   `cfg.WalkTriggerSetting != PLAYERPACKET`, re-path from `p.userPath`
   each tick; PLAYERSETUP additionally fires `processWalktrigger` when
   `!opcalled && hasWaypoints()`. Insertion phase is plan-author's
   choice per R3 — most likely a new per-player phase invoked from
   the tick driver, NOT inside `processInteraction` (which is target-
   gated and would skip players without an active target).

### Out of scope

- Door-interaction OPLOC1 → no-movement symptom (routes to NAI-78
  investigation+fix).
- TS `World.ts:624-628` per-tick `moveClickRequest = !busy && opcalled
  ? false : true` assignment. Orthogonal to the WalkTriggerSetting
  port; tracked for a future sub-spec if it surfaces.
- Any Tutorial-Island content-script changes.
- `MOVE_MINIMAPCLICK` (opcode 165). Different code path; left untouched.

---

## 4. Architecture / Files Touched

### Production

| File | Change | Line anchor |
|------|--------|-------------|
| `modules/world/walk_trigger_setting.go` | NEW. `WalkTriggerSetting` enum + parse helper. | — |
| `modules/world/config.go` | ADD `WalkTriggerSetting` field (yaml + flag, default PLAYERPACKET). | After line ~40 (sibling of NodeClientRoutefinder). |
| `modules/world/handlers_game.go` | REPLACE `handleMoveClick` body (lines 193-218). REWIRE dispatch table at lines 21-22. ADD `moveClickInner` + `handleMoveGameClick` + `handleMoveOpClick`. | 21-22 (dispatch), 193-218 (handler). |
| `modules/world/interaction.go` (likely) OR `modules/world/tick.go` (alternative) | ADD per-tick fallback branch. Insertion phase is open per R3: TS L635-641 sits in the per-player tick after `pathToTarget` and before post-step interact, BUT only fires for players whose `pathToTarget` branch did NOT `continue` (i.e., players without an active target chase). Goscape's `processInteraction` (interaction.go:169) is gated on having a target, so the fallback may live OUTSIDE it — likely a new per-player phase invoked from the tick driver alongside or just after `processInteractions`. **Plan-author re-reads `processInteraction` end-to-end + the surrounding tick phases before codifying the insertion line.** |

### Tests

| File | New tests |
|------|-----------|
| `modules/world/handlers_game_test.go` (extend) | `TestMoveGameClickRunsClearPendingActionAndClosesModal`, `TestMoveOpClickSkipsClearPendingAction`, `TestMoveClickDelayedSendsUnsetMapFlag`, `TestMoveClickInvalidCtrlHeldClearsUserPath`, `TestMoveClickStartTooFarClearsUserPath`, `TestMoveClickGameClickSetsTempRunFromCtrlHeld`, `TestMoveClickRunenergyLowSuppressesTempRun`, `TestMoveGameClickFiresProcessWalktriggerWhenPlayerpacketAndHasWaypoints`, `TestMoveGameClickSkipsWalktriggerWhenSettingNotPlayerpacket` |
| `modules/world/walk_trigger_setting_test.go` (new) | Enum parse tests + default = PLAYERPACKET pin. |
| `modules/world/interaction_test.go` (extend) | `TestPerTickWalkTriggerFallbackPlayerSetupFiresWhenNoOpCalled`, `TestPerTickWalkTriggerFallbackPlayerSetupSkipsWhenOpCalled`, `TestPerTickWalkTriggerFallbackPlayerMovementSkipsWalktrigger`, `TestPerTickWalkTriggerFallbackPlayerPacketSkipsBranch` |

---

## 5. Data Flow

### Symptom-2 path (MOVE_GAMECLICK with chatnpc open)

```
Java client click on tile
  → MOVE_GAMECLICK (opcode 181)
  → goscape gameHandlers[181] = handleMoveGameClick
  → moveClickInner(p, payload, opClick=false)
      1. delayed gate                      → UnsetMapFlag, return
      2. decode ctrlHeld + startX + startZ
      3. validate ctrlHeld in [0,1]
         AND distanceToSW(player, start) ≤ 104
                                           → fail: unsetMapFlag,
                                                    p.userPath = nil,
                                                    return
      4. set userPath:
           cfg.NodeClientRoutefinder=true  → pack full path
           cfg.NodeClientRoutefinder=false → pack only dest
      5. cfg.WalkTriggerSetting==PLAYERPACKET
         → pathToMoveClick(userPath, !cfg.NodeClientRoutefinder)
      6. !opClick (i.e. GAMECLICK):
         a. p.ClearPendingAction()         ← FIRES CloseModal(true)
                                              → modalState=None
                                              → IF_CLOSE triggers fire
                                              → modalChat=-1
         b. tempRun = ctrlHeld
            (override: runenergy<100 && ctrlHeld==1 → tempRun=0)
         c. cfg.WalkTriggerSetting==PLAYERPACKET
            && p.hasWaypoints()
            → p.processWalktrigger()
```

### MOVE_OPCLICK (opcode 93) path

Identical decode + path setup; **skips step 6 entirely**. Wire payload
is identical between 181 and 93 per TS `MoveClickDecoder.ts:28`; the
opClick bit comes from the originating opcode, not the payload.

### Per-tick fallback (TS `World.ts:635-641`)

Insertion phase per R3 — most likely a new per-player phase in the
tick driver (alongside or just after `processInteractions` invocation
in `tick.go`), NOT inside the target-gated `processInteraction` body.
Pseudocode:

```go
if s.cfg.WalkTriggerSetting != WalkTriggerSettingPlayerpacket {
    p.pathToMoveClick(p.userPath, !s.cfg.NodeClientRoutefinder)
    if s.cfg.WalkTriggerSetting == WalkTriggerSettingPlayersetup &&
       !p.opcalled && p.hasWaypoints() {
        p.processWalktrigger()
    }
}
```

`s.cfg` access: server pointer is reachable via `p.client.server.cfg`
(matches existing `handler_oploc.go:29` pattern).

---

## 6. Test Strategy

### Symptom-pin tests (handler-level, integration-style)

- **`TestMoveGameClickRunsClearPendingActionAndClosesModal`**: setup
  Player with `modalChat=N` (positive component), `modalState |= modalStateChat`,
  client wired through `newTestPlayer` helper. Send a synthesized
  MOVE_GAMECLICK payload. Assert post-handler: `modalChat == -1`,
  `modalState == modalStateNone`. This is the symptom-2 behavioral pin.
- **`TestMoveOpClickSkipsClearPendingAction`**: same setup, route via
  `handleMoveOpClick`. Assert `modalChat` UNCHANGED. Pins the !opClick
  branch gate (TS-asymmetry dual-pin per `ts_asymmetry_dual_pin.md`).

### Gate tests

- **Delayed gate**: set `p.delayed=true` + `p.delayedUntil > currentTick`.
  Assert UnsetMapFlag emitted (drain client out-pipe and look for the
  byte sequence; existing helper `assertUnsetMapFlag` may exist — if
  not, drain-pattern in `handler_oploc_test.go` is the template).
- **ctrlHeld out of range**: payload with ctrlHeld=2. Assert userPath
  cleared and unsetMapFlag emitted.
- **distance > 104**: payload with start far from p.x, p.z. Same
  assertion shape.

### Body-line tests

- **tempRun assignment**: send GAMECLICK with ctrlHeld=1. Assert
  `p.tempRun == 1`.
- **runenergy<100 suppression**: set `p.runenergy=50`, ctrlHeld=1.
  Assert `p.tempRun == 0`.

### Walktrigger-firing tests

- **Walktrigger fires when PLAYERPACKET + hasWaypoints**: register a
  walktrigger script (mock); send GAMECLICK with non-empty path; assert
  `p.walktrigger == -1` post-handler (consumed) and the script ran.
  Helper template at `interaction_test.go:694`
  `TestProcessWalktrigger_FiresAndClears`.
- **Walktrigger skipped when setting != PLAYERPACKET**: same setup,
  set `cfg.WalkTriggerSetting = WalkTriggerSettingPlayermovement`.
  Assert `p.walktrigger` unchanged.

### Per-tick fallback tests

- **`TestPerTickWalkTriggerFallbackPlayerSetupFiresWhenNoOpCalled`**:
  set cfg=PLAYERSETUP, p.opcalled=false, hasWaypoints=true. Invoke
  the new per-player fallback phase directly (function name TBD by
  plan-author; conventional candidate: `(*Player).processWalktriggerFallback`
  or a free function in interaction.go / tick.go). Assert
  `processWalktrigger` ran (use the walktrigger-cleared signal).
- **`TestPerTickWalkTriggerFallbackPlayerSetupSkipsWhenOpCalled`**:
  same but p.opcalled=true. Assert walktrigger NOT fired.
- **`TestPerTickWalkTriggerFallbackPlayerMovementSkipsWalktrigger`**:
  cfg=PLAYERMOVEMENT. Assert `pathToMoveClick` ran (re-path) but
  walktrigger NOT fired.
- **`TestPerTickWalkTriggerFallbackPlayerPacketSkipsBranch`**:
  cfg=PLAYERPACKET (default). Assert NEITHER re-path NOR walktrigger
  fired from the fallback path. Negative-pin per
  `ts_asymmetry_dual_pin.md`.

### Test fixture notes

- `newTestPlayer` and the `client.outChan` drain pattern are the
  established fixture for handler-level tests; templates at
  `handlers_game_test.go` and `handler_oploc_test.go`.
- For per-tick fallback tests, the walktrigger-clear signal at
  `interaction.go:282-290` is a clean assertion target (compare
  `interaction_test.go:694-735` `TestProcessWalktrigger_FiresAndClears`).

---

## 7. Risk Register

| # | Risk | Status | Mitigation |
|---|------|--------|------------|
| R1 | `ClearPendingAction → CloseModal(true)` semantics: TS uses immediate close (not deferred via `requestModalClose`). Goscape's `CloseModal` (player_script.go:674) IS immediate per inspection at spec-write — modalState set to None synchronously and IF_CLOSE triggers fire inline. | **VERIFIED at spec-write** (HEAD `1505bfa`) | Re-verify at plan-author + implementer-dispatch. No code change. |
| R2 | Wire payload between MOVE_GAMECLICK and MOVE_OPCLICK is identical (opClick comes from prot, not payload). | **VERIFIED at spec-write** via `MoveClickDecoder.ts:28`. | None. |
| R3 | Per-tick fallback insertion phase choice: TS L635-641 sits in `World.ts` per-player tick after `pathToTarget` and before post-step interact, but only fires for players whose `pathToTarget` branch did NOT `continue`. In TS this means it runs whether or not the player has a target. Goscape's `processInteraction` is target-gated (returns early when `p.target == nil` per `interaction.go:162-166`), so embedding the fallback there would skip target-less players — that's WRONG for click-away+walk where there's no target. The fallback likely needs to be a new per-player phase in `tick.go`, called alongside or just after `processInteractions`. | OPEN | Plan-author re-reads `processInteraction` end-to-end + the tick-driver phase ordering before codifying. If natural goscape phase doesn't align with TS, open `NAI-77-D-WALKTRIGGER-FALLBACK-PHASE-CHOICE` deviation with a doc-comment cross-cite. |
| R4 | `processWalktrigger` is a method on `*Player` and only fires under specific gates (delayed, !protectedScriptActive, etc. per `interaction.go:282`). Tests must respect these gates. | LOW | Test fixtures use the `interaction_test.go:694` template which sets up a non-delayed, non-protected player. |
| R5 | `s.cfg.NodeClientRoutefinder` access pattern: handler must reach config via `p.client.server.cfg`. Per-tick fallback must reach via the equivalent — `p.client.server.cfg` works in both contexts (verified via `handler_oploc.go:29` and `interaction.go` already accesses `s.currentTick` via similar path). | **VERIFIED at spec-write** | None. |
| R6 | `CloseModal(true)` clears `weakQueue`. If any in-flight walktrigger script lives on weakQueue, it would be dropped. TS does the same per `Player.ts:742-744`. | **TS-FIDELITY MATCHES**, no risk. | None. |

---

## 8. Open Questions

None at spec-time. All premises grep-verified at HEAD `1505bfa`.

---

## 9. Cadence + Close Criteria

**Cadence:** Standard sub-spec (NOT compressed) per
`runescript_cadence.md`. Stage-1 short-circuited at brainstorm via
grep evidence (no investigation phase). Stage-2 = single Plan + dispatch
via `subagent-driven-development` per `execution_mode_default.md`.

**Plan structure (anticipated):**
- T1: enum + config field (foundation, ~15 LOC, compressed-cadence
  candidate or merged into T2 at plan-author discretion)
- T2: `moveClickInner` + wrapper pair + dispatch rewire (TDD red→green)
- T3: per-tick fallback in `processInteraction` (TDD red→green)

**Close criteria:**
1. All new tests pass; existing test suite green (`go test ./...
   -count=1` from clean shell).
2. Smoke test by user (per `smoke_test_server_handoff.md`): launch
   server, log in, open chatnpc dialog, click ground tile → dialog
   closes. Pin smoke result in close commit body.
3. Close commit carries `Closes memory:` trailer per
   `close_commit_memory_trailer.md`.
4. Net deviation tally update in close commit message.
5. Door-interaction symptom NOT expected to resolve (NAI-78 territory);
   close on click-away resolution alone per
   `cascade_theory_smoke_binding.md`.

---

## 10. Deviations Expected

None expected. The port is a straight TS-fidelity application at
default `WalkTriggerSetting=PLAYERPACKET`. R3 (insertion-phase choice)
is the only candidate for opening a tracked deviation.

---

## 11. Follow-ups

- **NAI-78 candidate (still open):** Door interaction OPLOC1 → no
  movement, post-NAI-76 symptom-shape change. Investigation cadence
  with Stage-1 audit per `nai_followups.md` "From NAI-76" §2.
- **`World.ts:624-628` moveClickRequest per-tick assignment** — not
  ported. If symptom surfaces in any future smoke, route to a
  dedicated sub-spec.
- **TUT_CLOSE (opcode 2120) handler + `Player.closeTutorial()`** —
  carry-forward from NAI-76. Single content site, not loud at current
  smoke matrix.
- `NAI-75-D-FONT-WRAP-NAIVE`, `NAI-75-D-MESANIM-NOT-PORTED`,
  `NAI-72-D-FRIENDS-SERVER-BRIDGE`, `NAI-72-D-LOGIN-SERVER-BRIDGE-MOD`
  — still open, unrelated.

---

## 12. Lessons Anticipated

- `cascade_theory_smoke_binding.md` — second instance of the routing
  pattern (NAI-76 was first). The two cascade-residual symptoms split
  cleanly into two sub-specs because their roots are in independent
  network-handler dispatch paths.
- `controller_preflight.md` — all premises grep-verified at HEAD
  `1505bfa` BEFORE spec-write commit, not just at plan dispatch.
- `plan_grep_helper_patterns.md` — `ClearPendingAction` (helper) used
  rather than inlining `CloseModal(true)` directly.
- `true_to_ts_gate.md` — full-port scope chosen over symptom-minimum;
  no new deviations expected; per-tick fallback ported even though
  default-cfg path doesn't exercise it.
