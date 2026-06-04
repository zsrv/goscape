# NAI-146 — Post-decode block port (TS World.ts:611-641)

**Date:** 2026-05-10
**Status:** spec — pending plan
**Predecessors:** NAI-144 (`1ac9816` — gate body wired, inert at HEAD), NAI-77 T3 (`processWalkTriggerFallbacks` — partial port at wrong slot)
**Tracker entries closed by this:** `NAI-144-D-MoveClickRequestSetter`, `NAI-77-D-WALKTRIGGER-FALLBACK-PHASE-CHOICE`

## 1. Scope

Port the per-tick post-decode player block at TS
`Engine-TS/src/engine/World.ts:611-641`. This block:

1. Sets `moveClickRequest` based on `busy()` + `opcalled` (TS L624-628) — the
   missing setter that activates the NAI-144 gate at
   `modules/world/movement.go:64`.
2. Resets `faceEntity` to `-1` for non-PathingEntity targets and OR-publishes
   the entity mask (TS L619-622).
3. Routes `delayed`-while-userPath/opcalled to `unsetMapFlag()` and skips the
   rest of the block (TS L614-617).
4. Calls `pathToTarget()` when op-driven and not following a player (TS
   L630-633).
5. Folds in the existing NAI-77 `processWalkTriggerFallbacks` re-path +
   PLAYERSETUP walktrigger phase (TS L635-641), retiring the standalone
   step and shifting it from "after `processPathing`" to "before
   `processPathing`" — TS-faithful slot.

## 2. Background

### 2.1 Current state at HEAD

- **Gate body wired** at `modules/world/movement.go:64` (NAI-144,
  `1ac9816`):
  ```go
  if p.moveClickRequest && p.Busy() && (len(p.queue) > 0 || len(p.engineQueue) > 0) {
      p.walkDir = -1
      p.runDir = -1
      return
  }
  ```
- **Setter sites: zero in production code.** `moveClickRequest` is only ever
  assigned `false` (in `handler_opheld.go` rejection paths). The gate is
  inert. Tracked as `NAI-144-D-MoveClickRequestSetter`.
- **NAI-77 partial port** at `modules/world/walk_trigger_fallback.go` —
  `processWalkTriggerFallbacks` runs at the wrong tick slot (after
  `processPathing` instead of before it). Tracked as
  `NAI-77-D-WALKTRIGGER-FALLBACK-PHASE-CHOICE`. PLAYERPACKET default makes
  it a per-player no-op today; the slot order is latent.
- **`Player.opcalled`**: set in 11 op-handler success paths
  (`handler_opnpc.go`, `handler_oploc.go`, `handler_opobj.go`,
  `handler_op_player.go`); reset to `false` at top of `processIn`
  (`player.go:1063`). Mirror of TS `NetworkPlayer.opcalled`.
- **`Player.userPath`**: populated in `moveClickInner` at decode-time
  (NAI-77). Cleared to `nil` on rejection paths.

### 2.2 TS canonical block

```typescript
// Engine-TS/src/engine/World.ts:611-641
if (isClientConnected(player) && player.decodeIn()) {
    const followingPlayer = player.targetOp === ServerTriggerType.APPLAYER3
                         || player.targetOp === ServerTriggerType.OPPLAYER3;
    if (player.userPath.length > 0 || player.opcalled) {
        if (player.delayed) {
            player.unsetMapFlag();
            continue;
        }

        if ((!player.target || player.target instanceof Loc || player.target instanceof Obj)
            && player.faceEntity !== -1) {
            player.faceEntity = -1;
            player.masks |= player.entitymask;
        }

        if (!player.busy() && player.opcalled) {
            player.moveClickRequest = false;
        } else {
            player.moveClickRequest = true;
        }

        if (!followingPlayer && player.opcalled
            && (player.userPath.length === 0 || !Environment.NODE_CLIENT_ROUTEFINDER)) {
            player.pathToTarget();
            continue;
        }

        if (Environment.NODE_WALKTRIGGER_SETTING !== WalkTriggerSetting.PLAYERPACKET) {
            player.pathToMoveClick(player.userPath, !Environment.NODE_CLIENT_ROUTEFINDER);

            if (Environment.NODE_WALKTRIGGER_SETTING === WalkTriggerSetting.PLAYERSETUP
                && !player.opcalled && player.hasWaypoints()) {
                player.processWalktrigger();
            }
        }
    }
}
```

### 2.3 Goscape mapping table

| TS line | TS expression | Goscape mapping |
|---|---|---|
| 611 | `isClientConnected(p) && p.decodeIn()` | `p.client != nil && p.client.state == ClientStateGame && p.decodedThisTick` (T1 NEW field; tight gate per design choice) |
| 612 | `targetOp === APPLAYER3 \|\| OPPLAYER3` | `p.targetOp == 3` — per `modules/world/interaction.go:140-146` doc-comment, goscape's `targetOp` is the raw op slot 1..4 |
| 613 | `userPath.length > 0 \|\| opcalled` | `len(p.userPath) > 0 \|\| p.opcalled` |
| 614 | `delayed` | `p.delayed` |
| 615 | `unsetMapFlag()` | `p.unsetMapFlag()` (T2 NEW helper bundling `waypointIndex=-1` + `sendUnsetMapFlag(p)`; per `ts_helper_method_bundles.md`) |
| 619 | `!target \|\| target instanceof Loc \|\| target instanceof Obj` | `p.target == nil \|\| p.target.(*entitypkg.Loc) \|\| p.target.(*entitypkg.Obj)` (Go type assertions) |
| 620 | `faceEntity !== -1` | `p.faceEntity != -1` |
| 621 | `faceEntity = -1` | `p.faceEntity = -1` |
| 622 | `masks \|= entitymask` | `p.masks \|= p.entitymask` |
| 624-628 | moveClickRequest setter | direct port (3-case truth table) |
| 630-633 | pathToTarget when op-driven, not-following, no-userPath-or-non-routefinder | direct port; calls existing `(p *Player) pathToTarget()` at `interaction.go:671` |
| 635-641 | non-PLAYERPACKET re-path + PLAYERSETUP walktrigger | folded from existing `processWalkTriggerFallback` (`walk_trigger_fallback.go:28-43`) |

## 3. Architecture

```
processClientsIn / processIn (per-player)
├── ClientStateGame gate                      ← unchanged
├── lastConnected = currentTick               ← unchanged
├── reset: opcalled=false, *Limit=0           ← unchanged
├── *p.decodedThisTick = false                ← T1 NEW (R2 mitigation)
├── decode loop (readPacket × N)              ← unchanged
├── if readAny: lastResponse = currentTick    ← unchanged
├── *p.decodedThisTick = readAny              ← T1 NEW
├── *p.processPostDecode()                    ← T3 NEW (TS L611-641)
└── processInputTracking()                    ← unchanged

Tick-loop: processWalkTriggerFallbacks step retired (T4)
```

**New surfaces**:
- `Player.decodedThisTick bool` — zero-value-correct.
- `(p *Player) unsetMapFlag()` — bundles `p.waypointIndex = -1` +
  `sendUnsetMapFlag(p)`.
- `(p *Player) processPostDecode()` — TS L611-641 port.

**Retired surfaces**:
- `processWalkTriggerFallback` / `processWalkTriggerFallbacks` in
  `modules/world/walk_trigger_fallback.go` — folded into
  `processPostDecode`.
- `s.processWalkTriggerFallbacks()` step in `modules/world/tick.go:60`.

## 4. Implementation reference

### 4.1 T1 — `decodedThisTick`

`modules/world/player.go` — field declaration:
```go
afkEventReady, moveClickRequest, decodedThisTick bool
```

`modules/world/player.go` `processIn`:
```go
// existing ClientStateGame gate, lastConnected, opcalled=false, *Limit=0 …

p.decodedThisTick = false           // NAI-146 T1: reset before decode

c.inMu.Lock()
defer c.inMu.Unlock()

readAny := false
for /* existing decode loop */ { /* … */ }

if readAny {
    p.lastResponse = currentTick
}
p.decodedThisTick = readAny         // NAI-146 T1: TS decodeIn() return value

p.processPostDecode()               // NAI-146 T3: TS World.ts:611-641
p.processInputTracking(currentTick) // unchanged
```

### 4.2 T2 — `(p *Player) unsetMapFlag()`

`modules/world/interaction.go` (sibling of `sendUnsetMapFlag`):
```go
// unsetMapFlag clears the player's waypoint queue and emits the
// OpUnsetMapFlag packet. Mirrors TS Player.unsetMapFlag (Player.ts:2169)
// — the bundled clearWaypoints + write helper. Distinct from the
// wire-only sendUnsetMapFlag(p), which is preserved for decode-time
// handler call sites that already manage waypoint state inline.
//
// Per memory ts_helper_method_bundles.md: TS Player.unsetMapFlag is a
// helper bundle (clearWaypoints + write), so when porting a TS site
// that calls unsetMapFlag(), use this method, not sendUnsetMapFlag.
func (p *Player) unsetMapFlag() {
    p.waypointIndex = -1
    sendUnsetMapFlag(p)
}
```

### 4.3 T3 — `(p *Player) processPostDecode()`

New file `modules/world/player_post_decode.go`:
```go
package world

import (
    entitypkg "github.com/zsrv/goscape/pkg/entity"
)

// processPostDecode runs the per-tick post-decode block at TS
// Engine-TS/src/engine/World.ts:611-641. Called from end of processIn,
// before processInputTracking (matching TS L611-646 ordering).
//
// Activates the NAI-144 moveClickRequest gate at movement.go:64 by
// porting the L624-628 setter. Folds in NAI-77 walktrigger fallback
// (L635-641), retiring processWalkTriggerFallbacks; this also closes
// NAI-77-D-WALKTRIGGER-FALLBACK-PHASE-CHOICE by shifting the fallback
// from after-processPathing to before-processPathing (TS-faithful).
func (p *Player) processPostDecode() {
    if !p.decodedThisTick {
        return
    }
    if len(p.userPath) == 0 && !p.opcalled {
        return
    }
    if p.client == nil || p.client.server == nil {
        return // (goscape defensive; TS skips this check)
    }
    s := p.client.server

    // TS L614-617: delayed-while-pending → unsetMapFlag and skip.
    if p.delayed {
        p.unsetMapFlag()
        return
    }

    // TS L619-622: faceEntity reset for non-PathingEntity targets.
    if p.faceEntity != -1 {
        switch p.target.(type) {
        case nil, *entitypkg.Loc, *entitypkg.Obj:
            p.faceEntity = -1
            p.masks |= p.entitymask
        }
    }

    // TS L624-628: moveClickRequest setter — activates gate at
    // movement.go:64.
    if !p.Busy() && p.opcalled {
        p.moveClickRequest = false
    } else {
        p.moveClickRequest = true
    }

    // TS L630-633: pathToTarget when op-driven and not following a player.
    // followingPlayer = targetOp == 3 per interaction.go:140-146.
    followingPlayer := p.targetOp == 3
    if !followingPlayer && p.opcalled &&
        (len(p.userPath) == 0 || !s.cfg.NodeClientRoutefinder) {
        p.pathToTarget()
        return
    }

    // TS L635-641: non-PLAYERPACKET re-path + PLAYERSETUP walktrigger.
    // Folded from NAI-77 processWalkTriggerFallbacks; shifts back to
    // TS-faithful slot (before processPathing). Closes
    // NAI-77-D-WALKTRIGGER-FALLBACK-PHASE-CHOICE.
    if s.cfg.NodeWalktriggerSetting != WalkTriggerSettingPlayerpacket {
        p.pathToMoveClick(p.userPath, !s.cfg.NodeClientRoutefinder)
        if s.cfg.NodeWalktriggerSetting == WalkTriggerSettingPlayersetup &&
            !p.opcalled && p.hasWaypoints() {
            p.processWalktrigger()
        }
    }
}
```

### 4.4 T4 — Retire `processWalkTriggerFallbacks`

- Delete `modules/world/walk_trigger_fallback.go`.
- Delete `s.processWalkTriggerFallbacks()` invocation at
  `modules/world/tick.go:60`.
- Update / migrate any tests in `walk_trigger_fallback_test.go` (if any
  exist) to drive `p.processPostDecode()` instead. Plan-author audit:
  grep `processWalkTriggerFallback` at HEAD to enumerate consumers.

### 4.5 T5 — End-to-end gate-activation test

Extend `modules/world/player_movement_gate_test.go` with one
end-to-end test that drives the full path:
- delayed=false (so processPostDecode doesn't short-circuit)
- userPath set + Busy()=true (modal main open) + queue non-empty
- opcalled=false (so setter takes the `else` branch → `true`)
- decodedThisTick=true (gate satisfied)

Call `p.processPostDecode()` then `p.resolveMovement()`. Assert
`p.moveClickRequest == true` after post-decode AND
`p.walkDir == -1 && p.runDir == -1` after resolveMovement.

### 4.6 T6 — Tracker retirement

- `modules/world/movement.go:53-59` — replace the "INERT AT HEAD"
  paragraph with a closed-tracker reference to NAI-146 + this spec
  path.
- Tracker entry close in `nai_followups.md`:
  `NAI-144-D-MoveClickRequestSetter` → CLOSED with sha and bundle
  pointer; `NAI-77-D-WALKTRIGGER-FALLBACK-PHASE-CHOICE` → CLOSED.

## 5. Test Matrix

| Task | Test | Pin |
|--|--|--|
| T1 | `TestProcessIn_DecodedThisTickResetAndSet` | (a) reset to false at start of processIn; (b) set to true after readAny; (c) remains false on no-read tick |
| T2 | `TestPlayer_UnsetMapFlag_ClearsWaypointAndEmitsPacket` | `waypointIndex == -1` + `OpUnsetMapFlag` byte on out buffer |
| T3a | `TestProcessPostDecode_OuterGateSkipsWhenNotDecoded` | `decodedThisTick=false` → no state change even with userPath/opcalled set |
| T3a | `TestProcessPostDecode_OuterGateSkipsWhenIdle` | `decodedThisTick=true + len(userPath)==0 + !opcalled` → no state change |
| T3b | `TestProcessPostDecode_DelayedFiresUnsetMapFlagAndReturns` | `delayed=true + userPath set` → unsetMapFlag fires (waypointIndex=-1 + OpUnsetMapFlag), faceEntity untouched, moveClickRequest unchanged |
| T3c | `TestProcessPostDecode_FaceEntityResetForLocTarget` | `target=*Loc + faceEntity=42 + opcalled` → `faceEntity=-1`, `masks |= entitymask` |
| T3c | `TestProcessPostDecode_FaceEntityResetForObjTarget` | same for `*Obj` |
| T3c | `TestProcessPostDecode_FaceEntityResetForNilTarget` | nil target + faceEntity=42 + opcalled → reset |
| T3c | `TestProcessPostDecode_FaceEntityPreservedForPlayerTarget` | `target=*Player + opcalled` → faceEntity unchanged, masks untouched |
| T3c | `TestProcessPostDecode_FaceEntityNoOpWhenAlreadyMinusOne` | masks NOT touched when faceEntity already -1 (TS guard L620) |
| T3d | `TestProcessPostDecode_MoveClickRequest_NotBusyOpcalled` | `!Busy + opcalled` → `false` |
| T3d | `TestProcessPostDecode_MoveClickRequest_BusyOpcalled` | `Busy + opcalled` → `true` |
| T3d | `TestProcessPostDecode_MoveClickRequest_NotBusyNotOpcalled_Path` | `!Busy + !opcalled + userPath set` → `true` |
| T3e | `TestProcessPostDecode_PathToTargetFiresAndReturns` | `opcalled + targetOp!=3 + !routefinder` → `pathToTarget` called, walktrigger fallback NOT entered |
| T3e | `TestProcessPostDecode_PathToTargetSkippedForFollowingPlayer` | `opcalled + targetOp==3` → `pathToTarget` NOT called, walktrigger fallback proceeds |
| T3e | `TestProcessPostDecode_PathToTargetSkippedWhenRoutefinderAndUserPath` | `opcalled + routefinder + len(userPath)>0` → `pathToTarget` NOT called, walktrigger fallback proceeds |
| T3f | `TestProcessPostDecode_WalktriggerFallback_PlayerpacketNoOp` | default cfg → fallback inert (no `pathToMoveClick` re-path; no `processWalktrigger`) |
| T3f | `TestProcessPostDecode_WalktriggerFallback_Playersetup_FiresWhenNotOpcalled` | PLAYERSETUP + !opcalled + hasWaypoints → `processWalktrigger` fires |
| T3f | `TestProcessPostDecode_WalktriggerFallback_Playersetup_SkipsWhenOpcalled` | PLAYERSETUP + opcalled → re-path runs but `processWalktrigger` does NOT (TS L638 `!opcalled` guard) |
| T4 | (build pass) | tick.go no longer references `processWalkTriggerFallbacks`; `walk_trigger_fallback.go` deleted |
| T5 | `TestGateActivation_DelayedClickWalkSuppressesMovement` | end-to-end: post-decode sets `moveClickRequest=true` → `resolveMovement` returns early (`walkDir/runDir=-1`) |
| T6 | (doc-comment retire) | grep `MoveClickRequestSetter` returns no production sites; movement.go:53-59 paragraph replaced |

**T3 fixture sharing**: a `newPostDecodeTestPlayer` helper sets up
`p.client`, `p.client.server` with cfg, and the `decodedThisTick=true`
+ outer-gate satisfied baseline. Per
`scriptstate_test_fixture_idioms.md` precedent for ScriptState
fixtures.

## 6. Risk Register

- **R1 — Walktrigger fallback slot order shift** *(low at HEAD, latent)*.
  Folding into `processPostDecode` puts re-path BEFORE `processPathing`
  (TS-faithful), where today it runs AFTER. Default cfg = PLAYERPACKET
  → no production-visible change. Closes
  `NAI-77-D-WALKTRIGGER-FALLBACK-PHASE-CHOICE`. *Mitigation*: T3f
  covers PLAYERSETUP path; PLAYERPACKET no-op asserted.
- **R2 — `decodedThisTick` reset placement** *(low)*. Must reset to
  `false` BEFORE the decode loop (so a `return early` at
  `ClientStateGame` gate doesn't leak a stale `true` from prior tick).
  Per spec §4.1 reset slot is the line BEFORE `c.inMu.Lock()`.
  *Mitigation*: T1a pins reset at top of processIn.
- **R3 — `unsetMapFlag` waypoint clear semantics at decode-time
  callers** *(plan-author audit required)*. Per
  `ts_helper_method_bundles.md` TS bundles `clearWaypoints + write`.
  Goscape's `sendUnsetMapFlag` is wire-only. NAI-77 `moveClickInner`
  manually writes `p.waypointIndex = -1` inline alongside
  `sendUnsetMapFlag(p)`. Other call sites (`handler_opnpc.go` x7,
  others) emit packet WITHOUT clearing waypoints — possibly a
  pre-existing divergence from TS handlers that call
  `player.unsetMapFlag()`.

  **Plan-author audit required**: grep ALL `sendUnsetMapFlag(p)` sites
  at HEAD; cross-reference each TS counterpart handler to confirm
  whether it calls `unsetMapFlag()` (bundle) or `write(new
  UnsetMapFlag())` (wire-only). Per
  `enumerate_all_sites.md`. Route any divergences as in-scope-stretch
  (≤30 LOC) or NAI-N+1 deviation entries; do NOT in-scope rewrite the
  decode-time call sites in this bundle without explicit user
  approval.
- **R4 — `targetOp == 3` followingPlayer mapping** *(verified at
  HEAD)*. Per `interaction.go:140-146` doc-comment, goscape's
  `targetOp` is the raw op slot 1..4 → `followingPlayer = (targetOp
  == 3)` is the established mapping. Cross-reference for grep:
  `interaction.go:146` uses `if p.targetOp != 3` for the same TS
  expression.
- **R5 — `userPath` empty representation** *(low)*. TS `userPath = []`
  → goscape `len(p.userPath) == 0` (slice can be nil OR zero-len).
  `moveClickInner` uses both `nil` and `:0` slicing depending on cfg
  branch. *Mitigation*: tests use `len(...)==0` guard, never
  `== nil`. Spec uses `len(p.userPath) > 0` / `len(p.userPath) == 0`
  uniformly.
- **R6 — `processPostDecode` panic isolation** *(verified)*. processIn
  is invoked under `defer recoverPlayer(p, "processIn", s.log)` at
  `tick.go:94`. New phase inherits this. No additional isolation
  needed.
- **R7 — `entitymask` field name** *(verified at HEAD)*. Field is
  named `entitymask` (lowercase) at `modules/world/player.go:197`,
  initialised to `rsbuf.MaskFaceEntity` at `player.go:560`. Spec uses
  this form throughout.
- **R8 — `target` type assertions** *(verified)*. `p.target` is an
  `entitypkg.Pathable` / equivalent interface. Type-switching on
  `nil`, `*entitypkg.Loc`, `*entitypkg.Obj` is the established pattern
  per `pathToTargetSmart` at `interaction.go:695-702`.

## 7. Tracked deviations

- (none anticipated; full TS-faithful port — but plan-author may add
  during cross-grep audit)

## 8. Smoke

**Deferred** per `cascade_theory_smoke_binding.md` and per the answered
brainstorm question. PRIMARY pin = test-only; this extends the
deferred-smoke batch alongside NAI-143 / NAI-144 / NAI-145.

**Carry-forward**: bind a smoke at the next user-facing tick-touching
sub-spec close. Sample script for whoever runs it:
1. Login; verify normal walk works (regression fence).
2. Open chatnpc dialog (any NPC with `chat_*` script). Click to walk
   while chat modal is open. Player should NOT move (gate fires:
   `moveClickRequest=true && Busy() && len(queue)>0`). After dialog
   closes, click-to-walk should resume.
3. Without modal: click NPC for op1 → player walks to NPC and
   interacts (regression fence — `pathToTarget` branch).
4. NAI-145 zone-walk regression fence (≥3 zone boundaries).

## 9. Cadence

Per `runescript_cadence.md`: brainstorm → spec → plan → subagent-driven
TDD with two-stage review. Per
`superpowers_clear_between_spec_and_impl.md`: emit resume prompt and
stop after plan-write; user `/clear` before implementing.

Per `execution_mode_default.md`: dispatch via subagent-driven-development.

## 10. Tech stack

- Go 1.26+ per `go_version.md`.
- TS source canonical path: `Engine-TS` only per
  `ts_source_canonical_path.md`.

## 11. Pattern memories applicable

- `ts_helper_method_bundles.md` — T2 unsetMapFlag bundle (clearWaypoints + write)
- `enumerate_all_sites.md` — R3 audit
- `mock_recorder_field_naming_check.md` — R7 entitymask verified at spec-write
- `compressed_cadence.md` — does NOT apply (multi-task plan, ~80-150 LOC)
- `verify_implementer_claims.md` — controller pre-flight verification
- `cascade_theory_smoke_binding.md` — smoke deferral honest-noting
- `close_commit_memory_trailer.md` — `Closes memory:` trailer in close commit
- `superpowers_code_reviewer_model.md` — Sonnet for both reviewer + impl
