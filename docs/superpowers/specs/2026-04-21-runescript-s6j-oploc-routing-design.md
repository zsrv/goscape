# S6j — OPLOC Routing Design

> **Sub-spec context:** Tenth sub-spec in the runescript-s* series. Switches threads from the player-stats trio (S6g/h/i) to gameplay interactivity. OPLOC click routing — the sibling to OPNPC (S6b) — for object/loc clicks (trees, doors, banks, etc.). Cumulative-deferred since S6c (six sub-specs).

> **TS-faithfulness gate:** User explicitly required "true to TS." All behavioral claims cite TS line numbers in `/home/owner/Code/github.com/LostCityRS/Engine-TS`. Documented deviations are listed in §6 with rationale + follow-up sub-spec pointer.

> **Scope:** Approach 1 (minimal mirror of S6b OPNPC routing). Routing layer only — no LocType.Op, no script-side `loc_op` opcode, no apRange/opRange semantics, no OpLocT/OpLocU sibling opcodes, no default-op message.

## 1. Goal

Wire the OPLOC1..5 client click opcodes to fire `[oploc1..5,<locType>]` script triggers on the next tick. After this sub-spec, a player clicking on a static loc (tree, door, signpost) routes through the standard interaction state machine and fires its registered op-trigger script.

## 2. Architecture

OPLOC routing has two phases — exactly the shape S6b's OPNPC routing established as canonical.

**Phase A — Click handler** (synchronous, network thread, in `handleOpLoc1..5`):
1. Decode payload from wire (`x`, `z`, `locId` — all `G2`)
2. Validate four sequential gates (delayed → viewport → loc-exists → locType-exists)
3. Mutate player interaction state (clear existing → `SetInteraction(InteractionEngine, loc, op)` → record `targetSubject`)
4. Return — **no script fires here**

**Phase B — Tick interaction step** (next tick, world thread, in `tryFireOpTrigger`):
1. `locStillValid` check — was the loc despawned, type-changed, or removed from zone?
2. Trigger lookup via existing `Provider.GetByTrigger` (3-tier fallback: type → category → global)
3. Trigger script executes if found; interaction clears on Finished/Aborted

This decomposition gives test isolation: handler tests cover Phase A in pure validation table form; tick-fire tests cover Phase B by manipulating the post-handler state directly.

## 3. File Map

| File | Action | Purpose |
|---|---|---|
| `pkg/entity/loc.go` | Modify | Add `Slot() int` (returns -1) + `Coords() (x,z,level int)` to satisfy `entity` interface |
| `pkg/script/state.go` | Modify | Add `ActiveLoc *Loc`-equivalent + `PtrActiveLoc` constant if not present |
| `modules/world/server.go` (or new `loc_lookup.go`) | Modify | Add `Server.GetLoc(level, x, z, locId int) *Loc` — iterates `zoneMap.Get(level,x,z).Locs` matching `typeID` |
| `modules/world/player.go` | Modify | Add `targetSubject struct{Type, X, Z, Level int}` field |
| `modules/world/handler_oploc.go` | Create | New file: `handleOpLoc1..5` — mirror of `handler_opnpc.go` shape |
| `modules/world/handlers_game.go` | Modify | Wire 5 new opcode → handler entries |
| `modules/world/interaction_trigger.go` | Modify | Append `*Loc` switch case to `tryFireOpTrigger` |
| `pkg/entity/loc_test.go` | Create or modify | 4 entity-interface conformance tests |
| `modules/world/handler_oploc_test.go` | Create | 8 handler validation + state-change tests |
| `modules/world/interaction_trigger_test.go` | Modify | Append 6 Loc-branch fire tests |

## 4. TS Reference Map

The TS implementation lives in:

- **Handler:** `src/network/game/client/handler/OpLocHandler.ts:14-46` (validation + state mutation)
- **Sibling for comparison:** `src/network/game/client/handler/OpNpcHandler.ts` (S6b's reference)
- **State mutation target:** `src/engine/entity/PathingEntity.ts:510-548` (`setInteraction`)
- **Tick fire site:** `src/engine/entity/Player.ts:1119` (`tryInteract`), `Player.ts:1031` (`getApTrigger`), `Player.ts:997` (`getOpTrigger` with `+7` APLOC→OPLOC conversion)
- **Lifecycle safeguard:** `src/engine/entity/Player.ts:1186-1197` (`validateTarget`)
- **Loc lifecycle:** `src/engine/entity/Loc.ts:58-59` (despawn → `World.removeLoc`)
- **Script lookup tier:** `src/engine/script/ScriptProvider.ts:124-134` (matches our existing `Provider.GetByTrigger` exactly — no provider changes needed)

## 5. Phase A — Click Handler (`handleOpLoc1..5`)

### 5.1 Packet decode

All five opcodes use the same 6-byte payload (per `prot.go` registration matching TS `OpLocDecoder.ts:14-20`):

```
x        : G2  (absolute world coord)
z        : G2  (absolute world coord)
locId    : G2  (LocType ID)
```

`OpLocT` (8 bytes, +`spellCom: G2`) and `OpLocU` (12 bytes, +`useObj/useSlot/useCom: G2`) are **out of scope** (deviation S6j-D5).

### 5.2 Validation gates (mirrors `OpLocHandler.ts:14-42`)

Five sequential gates. Failure on any: `player.UnsetMapFlag()` + return.

| # | Gate | TS line | goscape impl |
|---|---|---|---|
| 1 | `player.delayed` rejection | `OpLocHandler.ts:14-18` | `if p.delayed && srv.currentTick < p.delayedUntil { p.UnsetMapFlag(); return }` |
| 2 | Viewport: `|dx|<=52 && |dz|<=52` from player origin | `OpLocHandler.ts:20-28` | Inline check using `p.origin{X,Z}` (52 = client render distance constant) |
| 3 | Loc exists: `World.getLoc(x, z, level, locId)` | `OpLocHandler.ts:30-35` | New `Server.GetLoc(level, x, z, locId)` returns nil → `UnsetMapFlag` |
| 4 | LocType exists | `OpLocHandler.ts:37` | `srv.locTypes.Get(loc.Type())` returns nil → `UnsetMapFlag` |
| 5 | **`locType.op[op-1] != null && != 'hidden'`** | `OpLocHandler.ts:38-42` | **DEFERRED — see deviation S6j-D1** |

### 5.3 State mutation (mirrors `OpLocHandler.ts:45-46` + `PathingEntity.ts:510-548`)

After all gates pass, the handler executes (in order):

1. **Clear pending action.** Whatever `clearPendingAction` equivalent S6b's `handleOpNpc` uses for the same effect — clear existing interaction, close modals. Replicated identically here.
2. **Set interaction.** `p.SetInteraction(InteractionEngine, loc, op)` where `op` is the opcode-derived 1..5 number (NOT the trigger value — see deviation S6j-D3).
3. **Record `targetSubject`.** New struct field on `Player`:
   ```go
   p.targetSubject = targetSubject{
       Type:  loc.Type(),
       X:     loc.X,
       Z:     loc.Z,
       Level: loc.Level,
   }
   ```
   Note: `Loc.Type()` is a method (returns `int` from packed Info bitfield); `X/Z/Level` are direct exported fields on the embedded `entity.Entity`. This is the TS `targetSubject.type` lifecycle safeguard, extended with X/Z/Level for goscape's stale-pointer concern (see deviation S6j-D4).

### 5.4 Fields NOT mutated (deferred decoratives)

| TS field | TS line | Skipped because |
|---|---|---|
| `apRange = 10` | `PathingEntity.ts:517` | No approach-vs-operate logic in this sub-spec (deviation S6j-D6) |
| `apRangeCalled = false` | same | Paired with `apRange` |
| `faceEntity` / `focus` | `PathingEntity.ts:528-543` | Cosmetic; no test-observable difference for trigger fire |

## 6. Phase B — Tick-Deferred Trigger Fire

The existing `tryFireOpTrigger` (`interaction_trigger.go:28-81`) is exactly the right shape to extend. The new `*Loc` switch case is appended after the existing `*Npc` case. The default no-op tail (`p.interactionFired = true`) becomes the catch-all for any remaining unhandled target type (e.g., future `*Obj`).

### 6.1 New switch case structure

```go
loc, ok := p.target.(*Loc)
if ok {
    if p.delayed && srv.currentTick < p.delayedUntil {
        return  // defer; retry next tick
    }

    // Lifecycle gate — parallels npc.dead at line 41.
    // (1) Loc type changed since click (e.g., tree → stump): silent clear.
    // (2) Loc removed from zone (e.g., axed): silent clear.
    if !locStillValid(srv, loc, p.targetSubject) {
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

    trigger := script.TriggerOpLoc1 + script.ServerTriggerType(op-1)
    // Loc has no cached LocType pointer (only a packed Info bitfield);
    // resolve category through the LocType registry. Unlike the *Npc branch
    // which reads npc.typ.Category from a cached pointer.
    category := 0
    if lt := srv.locTypes.Get(loc.Type()); lt != nil {
        category = lt.Category
    }

    sf := srv.scriptProvider.GetByTrigger(trigger, loc.Type(), category)
    if sf == nil {
        p.ClearInteraction()
        p.interactionFired = true
        return
    }

    state := script.Init(sf, p, false, nil, nil)
    state.ActiveLoc = loc
    state.Pointers |= script.PtrActiveLoc
    state.Provider = srv.scriptProvider
    state.World = srv.worldVars
    state.Configs = srv.configsView
    state.Inv = srv.invLookup

    srv.resumeOrFinish(state, p)

    if state.Execution == script.Finished || state.Execution == script.Aborted {
        p.ClearInteraction()
    }
    p.interactionFired = true
    return
}
```

### 6.2 `locStillValid` semantics

Signature: `locStillValid(srv *Server, loc *entity.Loc, ts targetSubject) bool`. Two checks combined:

1. **Zone membership.** Look up `srv.zoneMap.Get(ts.Level, ts.X, ts.Z)`. Iterate `Zone.Locs` searching for the held `*Loc` pointer. Returns false if not found.
2. **Type match.** `loc.Type() == ts.Type`. Returns false if mismatched (e.g., tree was replaced by stump in place via `Info` bitfield mutation).

Both checks are necessary — type-only misses pointer-removal cases; pointer-only misses in-place mutation cases (recall Loc's docstring: *"Returns a pointer so callers can mutate Info in place (shape changes, angle changes) without re-allocating."*).

### 6.3 Comment update

The existing default-branch comment (`interaction_trigger.go:18-19`) currently says:

> *Target not \*Npc: no-op; set interactionFired so we don't retry. (OPLOC/OPOBJ branches will extend this switch in a later sub-spec.)*

Updates to:

> *Target neither \*Npc nor \*Loc: no-op; set interactionFired so we don't retry. (OPOBJ branch will extend this switch in a later sub-spec.)*

## 7. Loc Entity Interface (`pkg/entity/loc.go`)

`Loc` does not currently implement the `entity` interface (defined in `modules/world/movement_consts.go`) — missing `Slot() int` and `Coords() (x, z, level int)`. Without these, `p.target = loc` cannot type-check.

### 7.1 New methods

```go
// Slot returns -1 because locs are not slot-indexed (unlike Players and Npcs
// which live in server-wide slot registries). Required for the entity
// interface.
func (l *Loc) Slot() int { return -1 }

// Coords returns the loc's tile position. Required for the entity interface.
func (l *Loc) Coords() (x, z, level int) {
    return l.X, l.Z, l.Level
}
```

`X`, `Z`, `Level` are direct exported fields on the embedded `entity.Entity` (see `pkg/entity/entity.go:6-12`); no accessor methods needed.

### 7.2 Compile-time conformance

Add an interface assertion in `loc_test.go` (or wherever the `entity` interface assertion lives):

```go
var _ entity = (*Loc)(nil)  // adjust import as needed
```

## 8. `ScriptState.ActiveLoc` + `PtrActiveLoc`

The `state.ActiveLoc = loc` and `state.Pointers |= script.PtrActiveLoc` lines reference fields that may already exist in `pkg/script/state.go` (parallel to `ActiveNpc` / `PtrActiveNpc`). Quick verification needed during implementation. If absent, this becomes a small additive change in Task 1, mirroring whatever shape `ActiveNpc` takes.

## 9. Test Plan

### 9.1 `pkg/entity/loc_test.go` — entity interface conformance (3 tests)

| # | Test | Asserts |
|---|---|---|
| 1 | `TestLocSlotReturnsMinusOne` | `loc.Slot() == -1` |
| 2 | `TestLocCoordsReturnsXZLevel` | `loc.Coords()` matches construction values |
| 3 | `TestLocSatisfiesEntityInterface` | Compile-time `var _ entity = (*Loc)(nil)` |

### 9.2 `modules/world/handler_oploc_test.go` — Phase A validation (8 tests)

| # | Test | Setup | Asserts |
|---|---|---|---|
| 1 | `TestHandleOpLocRejectsDelayedPlayer` | `p.delayed=true`, valid payload | `UnsetMapFlag` called, `p.target == nil` |
| 2 | `TestHandleOpLocRejectsOutOfViewport` | Coords > 52 from `p.origin` | `UnsetMapFlag` called, no state change |
| 3 | `TestHandleOpLocRejectsMissingLoc` | `Server.GetLoc` returns nil | `UnsetMapFlag` called, no state change |
| 4 | `TestHandleOpLocRejectsMissingLocType` | LocType not loaded | `UnsetMapFlag` called, no state change |
| 5 | `TestHandleOpLocSetsInteractionState` | Valid click, op=1 | `p.target == loc`, `p.targetOp == 1`, `p.targetSubject` populated correctly |
| 6 | `TestHandleOpLocAllFiveOpsRouteIndependently` | Table test, op ∈ {1,2,3,4,5} | Each routes through with `p.targetOp == op` |
| 7 | `TestHandleOpLocClearsExistingInteraction` | Pre-existing interaction state | Cleared before new state set |
| 8 | `TestHandleOpLocCoordValidationBoundary` | Exactly 52-tile distance | Accepted; 53-tile rejected |

### 9.3 `modules/world/interaction_trigger_test.go` — Phase B fire (6 new tests)

Append to existing test file. Each test sets up a Loc-target interaction state directly (not through `handleOpLoc`) for isolation.

| # | Test | Setup | Asserts |
|---|---|---|---|
| 1 | `TestTryFireOpTriggerLocNoScript` | Valid Loc target, no `[oploc1,*]` registered | Silent `ClearInteraction`, `interactionFired=true` |
| 2 | `TestTryFireOpTriggerLocScriptFires` | Valid Loc target, `[oploc1,<typeID>]` registered | Script fires, `state.ActiveLoc == loc`, `ClearInteraction` after `Finished` |
| 3 | `TestTryFireOpTriggerLocDeferredOnDelay` | `p.delayed=true`, `srv.currentTick < p.delayedUntil` | No-op, `interactionFired` stays false |
| 4 | `TestTryFireOpTriggerLocTypeChanged` | `targetSubject.Type != loc.Type()` | Silent `ClearInteraction` |
| 5 | `TestTryFireOpTriggerLocRemoved` | Loc not in `zone.Locs` | Silent `ClearInteraction` |
| 6 | `TestTryFireOpTriggerLocOpOutOfRange` | `p.targetOp == 0` (or 6) | Silent `ClearInteraction` (mirrors NPC test) |

**Total: ~18 new tests** (3 entity + 1 GetLoc + 8 handler + 6 fire). Mirrors S6b coverage density.

## 10. Task Split

Three tasks, mirrors S6i cadence (infra → handler → fire wire).

### Task 1 — Loc entity interface + `Server.GetLoc` + ScriptState plumbing

**Pure additive — build green throughout, no behavior change to existing flows.**

- Files: `pkg/entity/loc.go`, `modules/world/server.go` (or new `modules/world/loc_lookup.go`), `pkg/script/state.go`
- Add `Loc.Slot()`, `Loc.Coords()`, interface assertion
- Add `Server.GetLoc(level, x, z, locId int) *Loc`
- Add `ScriptState.ActiveLoc` + `PtrActiveLoc` if not present
- Tests: 3 entity tests + 1 GetLoc test (finds existing, returns nil for missing)
- Commit: `feat(world): Loc.Slot/Coords + Server.GetLoc + ScriptState.ActiveLoc plumbing`

### Task 2 — `handler_oploc.go` + handler tests

**Phase A — synchronous click routing.**

- Files: `modules/world/handler_oploc.go` (new), `modules/world/handlers_game.go` (wire opcodes), `modules/world/player.go` (add `targetSubject` field)
- Implement `handleOpLoc1`, `handleOpLoc2`, `handleOpLoc3`, `handleOpLoc4`, `handleOpLoc5` (or one `handleOpLoc(p, payload, op int)` with five thin wrappers per S6b's pattern)
- Wire 5 entries in `gameHandlers` map in `handlers_game.go`
- Add 5 validation gates per §5.2 (skipping op-validation per S6j-D1)
- Add state mutation per §5.3
- Tests: 8 handler tests per §9.2
- Build green; depends on Task 1's `Server.GetLoc`
- Commit: `feat(world): handleOpLoc1..5 + 8 validation tests`

### Task 3 — Extend `tryFireOpTrigger` with `*Loc` branch

**Phase B — tick-deferred trigger fire. End-to-end OPLOC click → trigger fire path live after this task.**

- Files: `modules/world/interaction_trigger.go`, `modules/world/interaction_trigger_test.go`
- Append `*Loc` switch case per §6.1
- Implement `locStillValid` per §6.2 (small helper, possibly in same file or `loc_lookup.go`)
- Update existing default-branch comment per §6.3
- Tests: 6 fire tests per §9.3
- Build green; depends on Task 1 (Server.GetLoc, Loc entity interface) and Task 2 (handleOpLoc for integration test setup)
- Commit: `feat(world): tryFireOpTrigger Loc branch + 6 fire tests`

## 11. Deviations from TS — Complete Summary

| ID | TS behavior | goscape S6j | Reason | Follow-up |
|---|---|---|---|---|
| **S6j-D1** | Reject if `locType.op[op-1] == null \|\| 'hidden'` (`OpLocHandler.ts:38-42`) | Skip per-op validation; accept all 5 slots | `LocType.Op []string` deferred from this scope | "LocType.Op + loc_op script opcode" sub-spec |
| **S6j-D2** | Set `targetOp = APLOC1+(op-1)`; engine fires APLOC at approach range, OPLOC at contact | Set `targetOp = op` (1-5); fire `TriggerOpLoc1+(op-1)` directly without APLOC fallback | Inherits S6b OPNPC convention; APLOC requires apRange/opRange machinery | "approach-vs-operate range gating + APLOC fallback" sub-spec |
| **S6j-D3** | `targetOp` stores trigger enum value | Stores op number 1-5 | Goscape codebase-wide convention from S6b | None — pure storage convention |
| **S6j-D4** | `validateTarget` checks only `targetSubject.type !== target.type` | `locStillValid` adds zone-membership check | Goscape uses raw `*Loc` pointers; needs stale-ref guard | None — defensive addition |
| **S6j-D5** | Handler decodes & registers `OpLocT` (8 bytes) and `OpLocU` (12 bytes) | Skip — only OpLoc1..5 routed | Sibling opcodes; not core routing | "OpLocT/U handlers" sub-spec |
| **S6j-D6** | `setInteraction` sets `apRange=10`, calls `focus()` for facing | Skip — both decorative without range/facing tick logic | Out of scope | "approach-range" + "loc facing" sub-specs |
| **S6j-D7** | Default "Nothing interesting happens" message when no script | Skip — silent no-op via `ClearInteraction` | Needs message infrastructure | "default-op message" sub-spec |

## 12. Scope Estimate

- **Implementation:** ~280-340 LOC
- **Tests:** ~250-300 LOC
- **Commits:** 3 (one per task)
- **Build/test green:** at every commit
- **End-to-end gain:** after Task 3, `[oploc1,object_xyz]` scripts in test fixtures fire on player click

## 13. Out-of-Scope Reminders

These are explicitly NOT in S6j (each tracked in §11 as a follow-up):

- LocType.Op []string field — handler op-validation gate stays disabled
- Script `loc_op` opcode wiring (currently reads non-existent `LocType.Op`)
- OpLocT (target-spell-on-loc) and OpLocU (use-item-on-loc) handlers
- apRange / opRange semantics + APLOC1..5 trigger constants + APLOC→OPLOC fallback
- Loc facing / camera focus
- "Nothing interesting happens" defaultOp message
- Loc operate-distance check (`reachedLoc()` in TS — uses loc shape/angle/forceapproach)
