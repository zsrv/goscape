# S6n — APNPC Approach-Range Gating Design

> **Sub-spec context:** Fourteenth sub-spec in the runescript-s* series. Closes S6l-D2 by wiring the NPC-side parallel to S6l's APLOC work. Player approaching an NPC within the NPC's per-type `AttackRange` fires `[apnpc<op>,<npcType>]` scripts. Unlike S6l's Player-mutable `apRange`, NPC approach-range is **fixed per NpcType** (`type.attackrange`) and there is no `apRangeCalled` persistence — TS NPC AP scripts complete and clear unconditionally.

> **TS-faithfulness gate:** User requires "true to TS." Three documented NPC-vs-Loc divergences inside the fire helper (lifecycle gate, category source, persistence model), one new deviation (S6n-D1: T/U sentinels skipped — OpNpcT/OpNpcU handlers don't exist yet).

> **Scope:** Approach 1 (core APNPC wiring, 1..5 ops). Defers DRY refactor of the 4 fire-helpers; defers T/U sentinel support; defers `fireOpTriggerNpc` refactor to use the new helper + 7.

## 1. Goal

Wire APNPC trigger dispatch so player approaches to NPCs fire the approach-range scripts at the NPC's per-type attackrange. After this sub-spec, NPC combat scripts that use `[apnpc1,*]` (attack-at-range, aggro detection, etc.) work correctly — today they never fire because `tryFireApTrigger` short-circuits via its default branch for non-Loc targets.

Observable gain:
- Archer/mage NPC scripts fire when player is within the NPC's attackrange
- `p.targetOp` = 1..5 (the op slot clicked via `handleOpNpc`) routes to `TriggerApNpc<op>` (3..7)
- Fire semantics are simpler than APLOC: no `apRangeCalled` persistence; terminal Execution always clears

## 2. Architecture

Two phases, 2 tasks:

**Phase A — target-type-specific `apRange` selection in `processInteraction`.** Currently `inApproachDistance(..., p.apRange)` unconditionally. For NPC targets, the correct range is `npc.typ.AttackRange` (fixed per-type), not the mutable `p.apRange`. A small `effectiveApRange(p *Player) int` helper picks the right value.

**Phase B — `fireApTriggerNpc` helper + `tryFireApTrigger` Npc-case dispatch.** Mirrors S6l's `fireApTriggerLoc` with three NPC-specific divergences (see §5.3). New `apNpcTriggerForOp` helper parallels `apLocTriggerForOp` for the 1..5 path (no T/U cases).

### Data flow

```
Click NPC 15 tiles away (NpcType.AttackRange = 5, op = 1):
click → handleOpNpc (existing, S6b) → SetInteraction(Engine, npc, 1, -1)
                                     → p.apRange = 10 (default; unused for NPC target)
tick N: still 15 tiles away → inApproachDistance(..., effectiveApRange(p))
                            → effectiveApRange(p) returns 5 (from npc.typ.AttackRange)
                            → 15 > 5, false → pathToTarget
tick N+K: player within 5 tiles → inApproachDistance returns true
                                → tryFireApTrigger → *Npc branch → fireApTriggerNpc
                                → apNpcTriggerForOp(1) → TriggerApNpc1
                                → script runs with ActiveNpc bound
                                → Execution terminal → ClearInteraction
                                → interactionFired = true
```

### Key TS divergences captured in code

| Concern | Loc (S6l) | NPC (S6n) |
|---|---|---|
| Approach range | mutable `p.apRange` (default 10; `p_aprange`-able) | fixed `npc.typ.AttackRange` |
| Lifecycle gate | `locStillValid` (zone + type match) | `npc.dead` flag |
| Category source | `srv.locTypes.Configs[locId].Category` | `npc.typ.Category` (cached pointer) |
| Persistence on Finished | `apRangeCalled`-gated (keep or clear) | unconditional `ClearInteraction` |
| `repathed=false` on persist | YES (p_aprange extends range) | N/A |

## 3. File Map

| File | Action | Purpose |
|---|---|---|
| `modules/world/interaction.go` | Modify | Add `effectiveApRange(p *Player) int` helper; swap `p.apRange` → `effectiveApRange(p)` in `processInteraction` AP branch |
| `modules/world/interaction_test.go` | Modify | 4 tests (3 helper unit + 1 integration) |
| `modules/world/interaction_trigger.go` | Modify | Add `apNpcTriggerForOp` helper + `fireApTriggerNpc`; wire `*Npc` case in `tryFireApTrigger` |
| `modules/world/interaction_trigger_test.go` | Modify | 2 helper unit tests + 5 fire tests |

**Existing infrastructure leveraged (no changes needed):**
- `TriggerApNpc1..5` (3..7), `TriggerOpNpc1..5` (10..14) — `pkg/script/trigger.go` (pre-enumerated)
- `NpcType.AttackRange uint16` field + decoder opcode 207 — `pkg/objtype/npctype.go:85`
- `Npc` satisfies `entity` interface — `modules/world/npc.go:145`
- `handleOpNpc` (S6b) passes `, -1` for com (S6m) — unchanged
- `inApproachDistance(px, pz, tx, tz, apRange int) bool` — S6l helper, generic over range param
- `apLocTriggerForOp` — S6m helper, template for new `apNpcTriggerForOp`
- `fireApTriggerLoc` structure — S6l template for new `fireApTriggerNpc`
- `tryFireApTrigger` — existing dispatcher (S6l); only Npc case gets wired

## 4. TS Reference Map

- **APNPC trigger enum:** `src/engine/script/ServerTriggerType.ts` — APNPC1..5, +7 offset to OPNPC preserved (verified: APNPC1 (3) + 7 = OPNPC1 (10); APNPC5 (7) + 7 = OPNPC5 (14))
- **`Npc.checkApTrigger`:** `src/engine/entity/Npc.ts:~861-883` — uses `type.attackrange`, not a mutable field
- **NPC script completion clears unconditionally:** `src/engine/entity/Npc.ts:~1064-1080` — no `apRangeCalled`-equivalent on NPCs

## 5. Component Details

### 5.1 `effectiveApRange` helper

Append to `modules/world/interaction.go` near `inApproachDistance`:

```go
// effectiveApRange returns the approach-range in tiles the player's
// current target should be checked against by inApproachDistance.
// For *Npc targets: the NPC's NpcType.AttackRange (fixed per-type,
// never mutated). For *Loc and all other targets: p.apRange (the
// mutable Player field, defaulted to 10 in SetInteraction and
// settable via p_aprange per S6l).
//
// Matches TS Npc.checkApTrigger (Npc.ts:~876) which reads
// type.attackrange, diverging from Player.tryInteract (Player.ts:~1139)
// which reads player.apRange.
//
// Returns 0 (which inApproachDistance rejects) if the target is an
// NPC with a nil NpcType — defensive guard; production cache always
// registers NpcType for any spawned NPC. Edge case: NpcType with
// AttackRange == 0 (uninitialized) will also yield 0 here, meaning
// APNPC never fires for that NPC. Intentional — production cache
// always sets attackrange for NPCs that have AP scripts.
func effectiveApRange(p *Player) int {
    if npc, ok := p.target.(*Npc); ok {
        if npc.typ == nil {
            return 0
        }
        return int(npc.typ.AttackRange)
    }
    return p.apRange
}
```

### 5.2 `processInteraction` AP branch swap

Currently at `modules/world/interaction.go:~81`:

```go
if inApproachDistance(p.x, p.z, tx, tz, p.apRange) {
```

Becomes:

```go
if inApproachDistance(p.x, p.z, tx, tz, effectiveApRange(p)) {
```

No other changes to `processInteraction`.

### 5.3 `apNpcTriggerForOp` helper

Append to `modules/world/interaction_trigger.go` near `apLocTriggerForOp`:

```go
// apNpcTriggerForOp returns the APNPC trigger for the player's
// targetOp. Returns ok=false if op is outside [1, 5]. fireOpTriggerNpc
// derives the OPNPC trigger by adding 7 to the returned APNPC (TS
// Player.ts:~997 offset convention):
//
//	APNPC1..5 (3..7) + 7 → OPNPC1..5 (10..14)
//
// NPC variant of apLocTriggerForOp. Does NOT handle T/U sentinels
// (DEVIATION S6n-D1) because OpNpcT/OpNpcU handlers are not wired in
// goscape yet — if those land, this helper's switch extends with
// matching cases.
func apNpcTriggerForOp(op int) (script.ServerTriggerType, bool) {
    if op >= 1 && op <= 5 {
        return script.TriggerApNpc1 + script.ServerTriggerType(op-1), true
    }
    return 0, false
}
```

### 5.4 `fireApTriggerNpc`

Append to `modules/world/interaction_trigger.go` after `fireApTriggerLoc`:

```go
// fireApTriggerNpc fires the [apnpc<op>,<npcType>] approach-trigger
// for the player's anchored NPC target when the player has reached
// the NPC's per-type attackrange. Matches TS Npc.ts:~861-883
// (checkApTrigger).
//
// Three divergences from fireApTriggerLoc (S6l):
//
//  1. Lifecycle gate is `npc.dead` (not locStillValid). NPCs have a
//     dedicated dead flag — no zone-membership pointer-stale check
//     needed because the *Npc reference itself is authoritative.
//
//  2. Category read from npc.typ.Category directly (the cached
//     pointer). fireApTriggerLoc does a locTypes.Configs[locId]
//     lookup because Loc has no cached LocType pointer, only a
//     packed Info bitfield.
//
//  3. NO apRangeCalled persistence contract. Per TS
//     (Npc.ts:~1064-1080): NPC AP scripts complete and clear
//     interaction unconditionally. The p_aprange persistence is
//     Player-side only; NPC attackrange is fixed per-type so
//     "extend the range" has no meaning. Simpler post-fire logic.
//
// DEVIATION S6n-D1: APNPC T/U sentinels not wired. OpNpcT/OpNpcU
// handlers don't exist in goscape yet; when they land,
// apNpcTriggerForOp gains matching cases and this fire function
// needs a sentinel-aware op-range gate update.
func fireApTriggerNpc(p *Player, srv *Server, npc *Npc) {
    if p.delayed && srv.currentTick < p.delayedUntil {
        return
    }

    if npc.dead {
        p.ClearInteraction()
        p.interactionFired = true
        return
    }

    trigger, ok := apNpcTriggerForOp(p.targetOp)
    if !ok {
        p.ClearInteraction()
        p.interactionFired = true
        return
    }

    category := 0
    if npc.typ != nil {
        category = npc.typ.Category
    }

    sf := srv.scriptProvider.GetByTrigger(trigger, npc.typeId, category)
    if sf == nil {
        p.ClearInteraction()
        p.interactionFired = true
        return
    }

    state := script.Init(sf, p, false, nil, nil)
    state.ActiveNpc = npc
    state.Pointers |= script.PtrActiveNpc
    state.Provider = srv.scriptProvider
    state.World = srv.worldVars
    state.Configs = srv.configsView
    state.Inv = srv.invLookup

    srv.resumeOrFinish(state, p)

    if state.Execution == script.Finished || state.Execution == script.Aborted {
        p.ClearInteraction()
    }
    p.interactionFired = true
}
```

### 5.5 `tryFireApTrigger` Npc case wire

Currently at `modules/world/interaction_trigger.go:~215-227`:

```go
func tryFireApTrigger(p *Player) {
    srv := p.client.server

    switch tgt := p.target.(type) {
    case *entitypkg.Loc:
        fireApTriggerLoc(p, srv, tgt)
    default:
        // *Npc, *Obj, etc. — AP branch not yet wired. Mark fired to
        // prevent same-tick retry; processInteraction's branch ordering
        // ensures OP still fires if player reaches contact next tick.
        p.interactionFired = true
    }
}
```

Becomes:

```go
func tryFireApTrigger(p *Player) {
    srv := p.client.server

    switch tgt := p.target.(type) {
    case *entitypkg.Loc:
        fireApTriggerLoc(p, srv, tgt)
    case *Npc:
        fireApTriggerNpc(p, srv, tgt)
    default:
        // *Obj, etc. — AP branch not yet wired. Mark fired to prevent
        // same-tick retry. Follow-up: APOBJ sub-spec.
        p.interactionFired = true
    }
}
```

### 5.6 Why S6n does NOT refactor `fireOpTriggerNpc`

`fireOpTriggerNpc` (S6j) uses inline `TriggerOpNpc1 + (op-1)` arithmetic and its own `op < 1 || op > 5` gate. Refactoring it to use `apNpcTriggerForOp + 7` would be byte-equivalent and symmetric with S6m's Loc OP refactor, but:
- S6n scope stays tight (no unrelated touches to S6j's OP path)
- The refactor becomes natural when OpNpcT/OpNpcU handlers land (forcing T/U sentinel support in `apNpcTriggerForOp` anyway)

Deferred as a follow-up — documented in §10 out-of-scope reminders.

## 6. Test Plan

### 6.1 `modules/world/interaction_test.go` — state machine (4 tests)

| # | Test | Asserts |
|---|---|---|
| 1 | `TestEffectiveApRangeNpcUsesTypeAttackrange` | NPC target with `AttackRange=5` → `effectiveApRange(p) == 5` regardless of `p.apRange` |
| 2 | `TestEffectiveApRangeLocUsesPlayerApRange` | Loc target → `effectiveApRange(p) == p.apRange` |
| 3 | `TestEffectiveApRangeNilNpcTypeReturnsZero` | NPC with `typ=nil` → `effectiveApRange(p) == 0` (defensive) |
| 4 | `TestProcessInteractionNpcUsesAttackrange` | Integration: NPC at dx=6 with `AttackRange=5` → pathing branch taken (not AP), proving attackrange (not p.apRange=10) is the gate |

### 6.2 `modules/world/interaction_trigger_test.go` — fire path (7 tests)

**Helper unit (2 tests):**

| # | Test | Asserts |
|---|---|---|
| 5 | `TestApNpcTriggerForOpValidValues` | Table: 1→TriggerApNpc1; 5→TriggerApNpc5; etc. |
| 6 | `TestApNpcTriggerForOpInvalidValues` | 0, 6, 7, 8, -1 → ok=false (no T/U sentinels) |

**Fire path (5 tests):**

| # | Test | Asserts |
|---|---|---|
| 7 | `TestFireApTriggerNpcNoScript` | No APNPC registered → `ClearInteraction`, `interactionFired=true` |
| 8 | `TestFireApTriggerNpcScriptFires` | APNPC1 script registered, npc alive → script fires, ActiveNpc bound, ClearInteraction after Finished |
| 9 | `TestFireApTriggerNpcDeadNpc` | `npc.dead=true` → silent clear (lifecycle gate) |
| 10 | `TestFireApTriggerNpcDeferredOnDelay` | `p.delayed=true` → no state change, `interactionFired` stays false |
| 11 | `TestFireApTriggerNpcOpOutOfRange` | `targetOp=0` → silent clear |

**Total: 11 new tests.**

## 7. Task Split

### Task 1 — `effectiveApRange` + `processInteraction` NPC-attackrange routing

- `modules/world/interaction.go` — helper + branch swap (~20 LOC)
- `modules/world/interaction_test.go` — 4 tests (~80 LOC)
- After commit: NPC approach-distance gate uses per-type attackrange. No APNPC fires yet because `tryFireApTrigger` default-branches for NPC targets (S6l state). Regression-free; observable only in unit tests.

Commit: `feat(world): effectiveApRange helper + NPC-attackrange routing in processInteraction (S6n-1)`

### Task 2 — `apNpcTriggerForOp` + `fireApTriggerNpc` + `tryFireApTrigger` Npc case

- `modules/world/interaction_trigger.go` — helper + fire function + dispatcher case (~110 LOC)
- `modules/world/interaction_trigger_test.go` — 7 tests (~150 LOC)
- After commit: APNPC scripts fire end-to-end. S6l-D2 CLOSED.

Commit: `feat(world): fireApTriggerNpc + APNPC trigger dispatch (S6n-2)`

## 8. Deviations from TS — Complete Summary

### New deviation introduced by S6n

| ID | TS behavior | goscape S6n | Reason | Follow-up |
|---|---|---|---|---|
| **S6n-D1** | `apNpcTriggerForOp` handles APNPC T/U sentinel paths (for OpNpcT/OpNpcU-clicked state) | Skip — only 1..5 cases wired | OpNpcT/OpNpcU click handlers don't exist yet | Bundle with "OpNpcT + OpNpcU handlers" sub-spec when scoped |

### S6l deviation status after S6n

| ID | Status | Notes |
|---|---|---|
| S6l-D1 | Still open (apRange=-1 sentinel) | Pure optimization; no gameplay impact |
| **S6l-D2** | ✅ **CLOSED in S6n** | APNPC approach-range gating wired |
| S6l-D3 | Still open | `ProtectedActivePlayer` gate on `p_aprange` |
| S6l-D4 | Still open | LOS/collision in distance checks |
| S6l-D5 | Still open | `p_op*` / `nextTarget` opcodes |

After S6n, S6l has 4 open deviations (D1/D3/D4/D5).

## 9. Scope Estimate

- **Implementation:** ~130 LOC across 2 files
- **Tests:** ~230 LOC (11 new tests)
- **Commits:** 2 (one per task)
- **Build/test green:** at every commit (Task 1 has no fire-path consumer yet; Task 2 wires it)
- **End-to-end gain:** APNPC triggers fire on player approach within NPC's per-type attackrange

## 10. Out-of-Scope Reminders

Explicitly NOT in S6n:

- DRY refactor of the 4 `fireXxxTriggerYyy` helpers (deferred — persistence-model divergence makes extraction awkward; revisit when OPOBJ/APOBJ lands)
- `fireOpTriggerNpc` refactor to use `apNpcTriggerForOp + 7` (symmetric with S6m's Loc OP refactor; bundle with OpNpcT/OpNpcU handlers)
- APNPC T/U sentinel support (S6n-D1; bundle with OpNpcT/OpNpcU handlers)
- LOS / collision gating in `inApproachDistance` (S6l-D4)
- S6l-D1 (apRange=-1 sentinel optimization; not meaningful for NPC since attackrange is fixed)
- S6l-D3 (ProtectedActivePlayer gate on p_aprange)
- S6l-D5 (p_op* opcodes)
