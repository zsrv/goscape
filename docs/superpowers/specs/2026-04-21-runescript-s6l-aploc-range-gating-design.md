# S6l — APLOC Approach-Range Gating Design

> **Sub-spec context:** Twelfth sub-spec in the runescript-s* series. Closes S6j-D2 (APLOC fallback path) and partially closes S6j-D6 (apRange field made meaningful) for Loc targets. Adds the apRange state machine + `p_aprange` script opcode so APLOC triggers can run before the player reaches contact range, and scripts can extend the approach window.

> **TS-faithfulness gate:** User requires "true to TS." All behavioral claims cite TS line numbers in `$HOME/Code/github.com/LostCityRS/Engine-TS`. Five new documented deviations (S6l-D1 through S6l-D5), each with rationale + follow-up pointer.

> **Scope:** Approach 1 (Loc-only bundle). APNPC path explicitly deferred to a follow-up. LOS distance checks explicitly deferred. `ProtectedActivePlayer` access gate explicitly deferred.

## 1. Goal

Wire the approach-range trigger tier for Loc interactions. After this sub-spec, a player who clicks a loc 10 tiles away walks toward it; when within `apRange` tiles (default 10), `[aploc<n>,<locType>]` scripts fire. The script can call `p_aprange(N)` to narrow the approach window, causing the interaction to persist across ticks as the player continues walking.

Observable gameplay gain:
- Scripts that use `[aploc1,*]` hooks (common for NPC-attack-at-range, loc-examine-at-distance, and movement triggers) now fire correctly.
- `p_aprange(N)` is the canonical knob tutorial scripts use to customize per-object approach distance.
- OPLOC (contact) still fires exactly as it does today — S6l adds a NEW path in parallel, doesn't disturb the existing one.

## 2. Architecture

**Phase A — No handler changes.** `SetInteraction` from S6j already resets `apRange=10` and `apRangeCalled=false`. `handleOpLoc` stays as-is.

**Phase B — `processInteraction` gains an approach-distance branch.** Currently a 3-state machine (target-gone / at-contact-fire-OP / path-toward-target). S6l inserts a 4th state between "not at contact" and "need to path": "within apRange but not at contact → fire AP."

**Phase C — `tryFireApTrigger` dispatcher + `fireApTriggerLoc` helper.** New functions in `interaction_trigger.go` mirroring `tryFireOpTrigger` / `fireOpTriggerLoc` but with distinct post-fire persistence semantics: `apRangeCalled=true` keeps the interaction anchored; `apRangeCalled=false` clears it (matching TS `Player.ts:1261`).

**Phase D — `p_aprange(N)` script opcode.** Sets `player.apRange=N` and `player.apRangeCalled=true` atomically via a new `ActivePlayer.SetApRange(n int)` interface method.

### Data flow

```
click → handleOpLoc validates → SetInteraction(loc) with apRange=10, apRangeCalled=false
tick N:   player far            → processInteraction paths toward loc
tick N+K: within apRange         → processInteraction fires tryFireApTrigger
                                 → fireApTriggerLoc: apRangeCalled=false, run APLOC script
                                 → script calls p_aprange(5) → apRangeCalled=true, apRange=5
                                 → next tick retries (interaction not cleared)
tick N+K': within new apRange=5  → APLOC re-fires
                                 → script clears or sets new apRange
tick N+M: at contact             → processInteraction fires tryFireOpTrigger (existing OPLOC path)
```

Key invariant: `apRangeCalled` is **reset to `false` before every AP fire** (TS line 1141). Each AP fire is a fresh evaluation — the script must actively call `p_aprange` to persist.

## 3. File Map

| File | Action | Purpose |
|---|---|---|
| `pkg/script/trigger.go` | Modify | Add `TriggerApLoc1..5` constants (59..63) |
| `pkg/script/active.go` | Modify | Extend `ActivePlayer` with `SetApRange(n int)` |
| `pkg/script/opcode.go` | No change | `OpPApRange Opcode = 2067` ALREADY REGISTERED (plus its name-stringify case). Only the handler is missing |
| `pkg/script/handlers.go` | Modify | Wire `OpPApRange → handleApRange` (case currently missing; running `p_aprange` today hits a dispatch miss) |
| `pkg/script/handlers_player.go` | Modify | Add `handleApRange` helper |
| `pkg/script/handlers_player_test.go` | Modify | 4 `p_aprange` handler tests |
| `modules/world/player_script.go` | Modify | Implement `*Player.SetApRange(n int)` |
| `modules/world/interaction.go` | Modify | Add `inApproachDistance`; extend `processInteraction` with AP branch; fix `ClearInteraction` to reset `apRange=10` |
| `modules/world/interaction_test.go` | Modify | 5 state-machine tests |
| `modules/world/interaction_trigger.go` | Modify | Add `tryFireApTrigger` + `fireApTriggerLoc` (Task 3; Task 2 ships a stub for build-green) |
| `modules/world/interaction_trigger_test.go` | Modify | 7 AP-fire tests |

**Existing infrastructure leveraged (no changes needed):**
- `Player.apRange int` — already reset in `SetInteraction` (interaction.go:28)
- `Player.apRangeCalled bool` — already reset in `SetInteraction`
- `TriggerOpLoc1..5` (66..70) — unchanged
- `Player.MessageGame` — defaultOp path (S6k)
- `locStillValid` — S6j lifecycle helper
- `requireActivePlayer` — existing active-guard pattern in handlers_player.go

## 4. TS Reference Map

- **apRange lifecycle:** `src/engine/entity/PathingEntity.ts:510-518` (setInteraction resets), `:554-555` (clearInteraction resets)
- **Two-path decision tree:** `src/engine/entity/Player.ts:1113-1184` (tryInteract)
- **apRangeCalled persistence gate:** `src/engine/entity/Player.ts:1261` (`if interacted && !apRangeCalled: clearInteraction`)
- **p_aprange opcode:** `src/engine/script/handlers/PlayerOpsHandler.ts` — pops int, sets `player.apRange = N; player.apRangeCalled = true`
- **Distance math:** `src/engine/entity/PathingEntity.ts:392-406` + `src/util/CoordGrid.ts:55-69` (Chebyshev + footprint closest-points + LOS)
- **defaultOp:** `src/engine/entity/Player.ts:1072-1097` — preserved from S6k

## 5. Component Details

### 5.1 `processInteraction` state machine

Open `modules/world/interaction.go`. Current shape (lines 51-85) has OP-at-contact and path-toward-target branches. Insert a middle AP branch:

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

    tx, tz, tlevel := p.target.Coords()
    if tlevel != p.level {
        p.ClearInteraction()
        sendUnsetMapFlag(p)
        return
    }

    if inOperableDistance(p.x, p.z, tx, tz) {
        // Contact range — fire OP. Matches TS Player.ts:1123-1135
        // (OP checked before AP at contact).
        if npc, ok := p.target.(*Npc); ok {
            p.SetFaceEntity(npc.nid)
        }
        p.interacted = true
        if p.interactionKind == InteractionEngine && !p.interactionFired {
            tryFireOpTrigger(p)
        }
        return
    }

    if inApproachDistance(p.x, p.z, tx, tz, p.apRange) {
        // Approach range — fire AP. Matches TS Player.ts:1139-1170.
        // S6l-D1: goscape skips TS's apRange=-1 sentinel.
        p.interacted = true
        if p.interactionKind == InteractionEngine && !p.interactionFired {
            tryFireApTrigger(p)
        }
        return
    }

    if !p.repathed {
        p.pathToTarget(tx, tz)
        p.repathed = true
    }
}
```

Order matters: operable distance ≤ approach distance, so the operable check MUST come first. Matches TS line 1123.

### 5.2 `inApproachDistance` helper

Pure Chebyshev, mirrors `inOperableDistance`. Append to `interaction.go`:

```go
// inApproachDistance returns true when (px,pz) is within apRange
// Chebyshev tiles of (tx,tz), excluding the same tile. Range-portion
// of TS PathingEntity.inApproachDistance, sans LOS (S6l-D4).
// apRange <= 0 always returns false.
func inApproachDistance(px, pz, tx, tz, apRange int) bool {
    if apRange <= 0 {
        return false
    }
    dx := px - tx
    if dx < 0 {
        dx = -dx
    }
    dz := pz - tz
    if dz < 0 {
        dz = -dz
    }
    if dx > apRange || dz > apRange {
        return false
    }
    return !(dx == 0 && dz == 0)
}
```

### 5.3 `ClearInteraction` apRange reset

Currently missing. Fix at `interaction.go:36-43`:

```go
func (p *Player) ClearInteraction() {
    p.target = nil
    p.targetOp = -1
    p.apRange = 10         // NEW: match TS PathingEntity.ts:554
    p.apRangeCalled = false
    p.interacted = false
    p.repathed = false
    p.interactionFired = false
}
```

### 5.4 `TriggerApLoc1..5` constants

Append to `pkg/script/trigger.go` (existing `TriggerOpLoc1..5` live at 66..70):

```go
// APLOC1..5 — approach-range triggers (59..70 split: APLOC=59..63,
// OPLOC=66..70, +7 offset between AP and OP per TS ServerTriggerType.ts).
TriggerApLoc1 ServerTriggerType = 59
TriggerApLoc2 ServerTriggerType = 60
TriggerApLoc3 ServerTriggerType = 61
TriggerApLoc4 ServerTriggerType = 62
TriggerApLoc5 ServerTriggerType = 63
```

### 5.5 `ActivePlayer.SetApRange` + `*Player.SetApRange`

**Interface extension** (`pkg/script/active.go`):

```go
// SetApRange sets the approach-range-in-tiles for the active
// interaction AND marks apRangeCalled=true. Called by p_aprange
// script opcode when an APLOC trigger wants to extend the range
// the player should approach before re-firing. Matches TS
// PlayerOps.ts:P_APRANGE — both fields are set atomically.
SetApRange(n int)
```

**Implementation** (`modules/world/player_script.go`):

```go
// SetApRange implements script.ActivePlayer.SetApRange. Atomically
// sets apRange and marks apRangeCalled=true to persist the
// interaction past the current tick.
func (p *Player) SetApRange(n int) {
    p.apRange = n
    p.apRangeCalled = true
}
```

### 5.6 `handleApRange` script handler

Append to `pkg/script/handlers_player.go`:

```go
// handleApRange pops the approach range (in tiles) and sets it on
// the active player along with apRangeCalled=true. Called from
// APLOC trigger scripts to extend the approach-distance at which
// the trigger re-fires. Matches TS PlayerOps.ts:P_APRANGE.
//
// No clamping or bounds check: TS is permissive. Negative values
// functionally disable the trigger (inApproachDistance returns
// false for apRange<=0) — scripts passing negative are
// misconfigured, not a security concern.
//
// S6l-D3: no ProtectedActivePlayer gate; goscape lacks the
// protected-access model.
func handleApRange(s *ScriptState) error {
    if err := requireActivePlayer(s, "P_APRANGE"); err != nil {
        return err
    }
    n := s.PopInt()
    s.Self.SetApRange(n)
    return nil
}
```

### 5.7 `tryFireApTrigger` + `fireApTriggerLoc`

Append to `modules/world/interaction_trigger.go` (alongside existing `tryFireOpTrigger` / `fireOpTriggerNpc` / `fireOpTriggerLoc`):

```go
// tryFireApTrigger fires the [aploc<op>,<locType>] approach-trigger
// for the player's anchored target when the player has just reached
// apRange. Matches TS Player.ts:1139-1170 for the Loc branch.
// S6l-D2: APNPC branch intentionally deferred.
//
// Preconditions (guaranteed by caller — Player.processInteraction):
//   - p.interacted == true
//   - p.interactionKind == InteractionEngine
//   - p.target != nil
//   - p.interactionFired == false
//   - player is in approach range but NOT operable distance
func tryFireApTrigger(p *Player) {
    srv := p.client.server

    switch tgt := p.target.(type) {
    case *entitypkg.Loc:
        fireApTriggerLoc(p, srv, tgt)
    default:
        // *Npc, *Obj, etc. — AP branch not yet wired. Mark fired to
        // prevent same-tick retry.
        p.interactionFired = true
    }
}

// fireApTriggerLoc fires the [aploc<op>,<locType>] trigger. The
// post-fire persistence contract diverges from OP triggers:
//
//   - Before fire: reset apRangeCalled=false (TS line 1141).
//   - After fire:
//     - apRangeCalled=true (script called p_aprange): keep interaction
//       anchored; reset repathed=false and interactionFired=false so
//       next tick re-evaluates at the new apRange.
//     - apRangeCalled=false AND Execution is Finished/Aborted:
//       clear interaction (TS line 1261).
//     - Execution is Suspended (P_DELAY etc.): keep interaction
//       anchored; resume tick will re-enter; clear happens when
//       script ultimately finishes without p_aprange.
//
// Lifecycle gate: locStillValid (same helper from S6j).
// Script lookup: TriggerApLoc1 + (op-1). No APLOC→OPLOC fallthrough
// at this distance — OPLOC only fires when player reaches contact
// (later processInteraction tick).
func fireApTriggerLoc(p *Player, srv *Server, loc *entitypkg.Loc) {
    if p.delayed && srv.currentTick < p.delayedUntil {
        return
    }

    if !locStillValid(srv, loc, p.targetSubject.typ, p.targetSubject.x, p.targetSubject.z, p.targetSubject.level) {
        p.ClearInteraction()
        p.interactionFired = true
        return
    }

    op := p.targetOp
    if op < 1 || op > 5 {
        p.ClearInteraction()
        p.interactionFired = true
        return
    }

    trigger := script.TriggerApLoc1 + script.ServerTriggerType(op-1)
    category := 0
    if locId := loc.Type(); locId >= 0 && locId < len(srv.locTypes.Configs) {
        if lt := srv.locTypes.Configs[locId]; lt != nil {
            category = lt.Category
        }
    }

    sf := srv.scriptProvider.GetByTrigger(trigger, loc.Type(), category)
    if sf == nil {
        // No AP script registered. S6l-D1: skip TS apRange=-1
        // sentinel; interaction stays anchored. Next tick will
        // re-evaluate: player at contact fires OP (or defaultOp);
        // still-in-apRange re-enters this branch (idempotent).
        p.interactionFired = true
        return
    }

    // Reset apRangeCalled before exec (TS line 1141).
    p.apRangeCalled = false

    state := script.Init(sf, p, false, nil, nil)
    state.ActiveLoc = loc
    state.Pointers |= script.PtrActiveLoc
    state.Provider = srv.scriptProvider
    state.World = srv.worldVars
    state.Configs = srv.configsView
    state.Inv = srv.invLookup

    srv.resumeOrFinish(state, p)

    if state.Execution == script.Finished || state.Execution == script.Aborted {
        if p.apRangeCalled {
            // Script requested a new approach range. Persist interaction;
            // allow re-evaluation next tick at the updated apRange.
            p.repathed = false
            // interactionFired stays false → processInteraction re-enters.
            return
        }
        // apRangeCalled=false → script didn't extend range; clear.
        p.ClearInteraction()
    }
    // Suspended (P_DELAY etc.): keep interaction anchored; resume
    // flow re-enters on resume tick.
    p.interactionFired = true
}
```

## 6. Test Plan

### 6.1 `pkg/script/handlers_player_test.go` — p_aprange (4 tests)

| # | Test | Asserts |
|---|---|---|
| 1 | `TestHandleApRangeRequiresActivePlayer` | `Self == nil` → error tagged `"P_APRANGE"` |
| 2 | `TestHandleApRangeSetsBothFields` | `p_aprange(5)` → `apRange == 5`, `apRangeCalled == true` |
| 3 | `TestHandleApRangeAcceptsNegative` | `p_aprange(-1)` → fields accepted (TS permissive) |
| 4 | `TestHandleApRangeDefaultInitialState` | Fresh player → `apRange == 10`, `apRangeCalled == false` |

### 6.2 `modules/world/interaction_test.go` — state machine (5 tests)

| # | Test | Asserts |
|---|---|---|
| 1 | `TestInApproachDistanceSameTile` | Same tile → `false` (matches `inOperableDistance`) |
| 2 | `TestInApproachDistanceAtRange` | Chebyshev exactly `apRange` → `true` |
| 3 | `TestInApproachDistanceBeyondRange` | `apRange+1` away → `false` |
| 4 | `TestInApproachDistanceZeroRange` | `apRange=0` → `false` for all positions |
| 5 | `TestClearInteractionResetsApRange` | After mutation, `ClearInteraction` resets `apRange=10` |

### 6.3 `modules/world/interaction_trigger_test.go` — AP fire (7 tests)

| # | Test | Asserts |
|---|---|---|
| 1 | `TestTryFireApTriggerLocNoScript` | APLOC not registered → no clear, `interactionFired=true` |
| 2 | `TestTryFireApTriggerLocScriptFiresNoApRangeCalled` | Script runs, no p_aprange → `ClearInteraction` runs |
| 3 | `TestTryFireApTriggerLocScriptCallsPApRange` | Script runs + p_aprange(5) → no clear, `apRange=5`, `repathed=false`, `interactionFired=false` |
| 4 | `TestTryFireApTriggerLocDeferredOnDelay` | Player delayed → no fire, no state change |
| 5 | `TestTryFireApTriggerLocTypeChanged` | `locStillValid` fails on type mismatch → silent clear |
| 6 | `TestTryFireApTriggerLocRemoved` | Loc removed from zone → silent clear |
| 7 | `TestTryFireApTriggerLocOpOutOfRange` | `targetOp=0` → silent clear |

**Total: 16 new tests.**

## 7. Task Split

Three tasks.

### Task 1 — `pkg/script` additions + `Player.SetApRange`

Cross-boundary but minimal (ActivePlayer method implementation on *Player is 2 lines).

- `pkg/script/trigger.go` — `TriggerApLoc1..5`
- `pkg/script/active.go` — `ActivePlayer.SetApRange(n int)`
- `pkg/script/opcode.go` — no change (`OpPApRange = 2067` already registered at line 167)
- `pkg/script/handlers.go` — dispatch wiring
- `pkg/script/handlers_player.go` — `handleApRange`
- `pkg/script/handlers_player_test.go` — 4 handler tests
- `modules/world/player_script.go` — `*Player.SetApRange` impl

Build green throughout. Commit: `feat(script): TriggerApLoc + p_aprange opcode + ActivePlayer.SetApRange (S6l-1)`

### Task 2 — `processInteraction` state machine

Pure `modules/world/interaction.go` changes. Ships a **stub** `tryFireApTrigger` (sets `interactionFired=true`, returns) so `processInteraction` compiles; Task 3 replaces the stub.

- `modules/world/interaction.go` — `inApproachDistance` + `processInteraction` AP branch + `ClearInteraction` fix
- `modules/world/interaction_trigger.go` — stub `tryFireApTrigger`
- `modules/world/interaction_test.go` — 5 state-machine tests

Commit: `feat(world): processInteraction apRange gating + inApproachDistance (S6l-2)`

### Task 3 — `tryFireApTrigger` + `fireApTriggerLoc`

Replaces Task 2's stub. Pure `modules/world/interaction_trigger.go`.

- `modules/world/interaction_trigger.go` — full `tryFireApTrigger` + `fireApTriggerLoc`
- `modules/world/interaction_trigger_test.go` — 7 AP-fire tests

Commit: `feat(world): tryFireApTrigger Loc branch + p_aprange persistence (S6l-3)`

## 8. Deviations from TS — Complete Summary

### New deviations introduced by S6l

| ID | TS behavior | goscape S6l | Reason | Follow-up |
|---|---|---|---|---|
| **S6l-D1** | `apRange = -1` sentinel after failed AP lookup (TS line 1174) | Skip sentinel; re-evaluate each tick | Provider lookup is cheap; no observable difference | None — pure optimization |
| **S6l-D2** | APNPC path exists in parallel (uses `NpcType.attackrange`, not `apRange`) | Only APLOC wired | Scope narrowed to match S6k thread | "APNPC apRange gating" sub-spec (with NPC AI) |
| **S6l-D3** | `p_aprange` uses `checkedHandler(ProtectedActivePlayer, …)` gate | Callable from any script | Goscape has no protected-access model yet | "multiplayer script access model" sub-spec |
| **S6l-D4** | `inApproachDistance` does Chebyshev + LOS | Pure Chebyshev, no LOS | Mirrors existing `inOperableDistance`; LOS is separable | "LOS / collision gating" sub-spec |
| **S6l-D5** | APLOC can set `nextTarget` via `p_op*` to re-anchor on new entity | Not wired (`p_op*` opcodes don't exist yet) | Pre-existing gap | Bundled with `p_op*` sub-spec |

### S6j deviation status after S6l

| ID | Status | Notes |
|---|---|---|
| S6j-D1 | ✅ CLOSED in S6k | Per-op validation gate |
| **S6j-D2** | ✅ **CLOSED FOR LOC** in S6l | APLOC path wired; apRange consumed |
| S6j-D3 | Still open | `targetOp` 1-5 storage convention (no follow-up planned) |
| S6j-D4 | Still open | `locStillValid` defensive zone check (no follow-up needed) |
| S6j-D5 | Still open | OpLocT / OpLocU sibling handlers |
| **S6j-D6** | ✅ **PARTIALLY CLOSED** in S6l | apRange meaningful; focus/camera deferred |
| S6j-D7 | ✅ CLOSED in S6k | defaultOp message |

After S6l, S6j has 3 open deviations (D3, D4, D5 + D6's focus half).

## 9. Scope Estimate

- **Implementation:** ~280-320 LOC across 7 files
- **Tests:** ~200 LOC (16 new tests)
- **Commits:** 3 (one per task)
- **Build/test green:** at every commit (Task 2 stub keeps build green)
- **End-to-end gain:** APLOC fires at approach range; `p_aprange` extends interactions; S6j-D2 closed for Loc

## 10. Out-of-Scope Reminders

Explicitly NOT in S6l:

- APNPC (NPC.attackrange) approach-trigger path (S6l-D2)
- LOS / collision gating in `inApproachDistance` (S6l-D4)
- `ProtectedActivePlayer` access gate on `p_aprange` (S6l-D3)
- `p_op*` / `nextTarget` re-anchor opcodes (S6l-D5)
- Focus / camera mutations on `setInteraction` (S6j-D6 focus half)
- OpLocT / OpLocU sibling handlers (S6j-D5)
