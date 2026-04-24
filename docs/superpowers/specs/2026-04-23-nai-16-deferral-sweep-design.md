# NAI-16 — NAI-5 + NAI-8 Deferral Sweep (NPC Vars Filter + ChangeType Duration + Test Backfills)

Close four deferrals accumulated across the NAI-5 and NAI-8 reviews, all
touching NPC instance state. Two items are production-code fixes that
unblock documented `DEFERRED` comment sites; two are test-coverage
backfills flagged by the NAI-5 final reviewer. Bundled because the items
share a subsystem (NPC instance state / hunt-AI) and independently are
too small to justify their own sub-specs.

**Bundle items:**

1. **Item A — `checkNotCombatSelf` filter** (closes NAI-8 deferral #4 in
   `nai_followups.md`). Wire the filter at the existing `DEFERRED` site
   inside the NAI-15 combat guard using the `NpcVarN` infrastructure
   that already landed in S6a. The `nai_followups.md` claim that
   "no NPC-vars infrastructure yet" was stale: `Npc.varns`,
   `NpcVarN(id) int32`, and `SetNpcVarN(id, val)` all ship at HEAD.

2. **Item B — `NPC_CHANGETYPE` duration wiring** (closes NAI-5 "unassigned
   small fix"). TS `Npc.changeType(type, duration)` writes the new type,
   recomputes the uid, and schedules a revert via `lifecycleTick`. Go's
   current `*Npc.ChangeType(newType)` only writes the mask-payload field
   and raises the mask bit — it never updates `n.typeId`, which is a
   latent correctness bug (any post-changetype `NPC_TYPE` read returns
   the OLD type). Extend `ActiveNpc.ChangeType` to `(newType, duration
   int)` and port the TS guard + writes.

3. **Item C — RESPAWN+alive morph-revert direct test** (closes NAI-5 test
   gap #2). Direct unit test of the `npc_ai.go:37-40` branch that fires
   `revertType()` on `lifecycle=Respawn && !dead && lifecycleTick==0`.
   Uses manual state setup — deliberately NOT routed through Item B's
   `ChangeType` so the test isolates the revert code path.

4. **Item D — `processNpcEventQueue` happy-path fire test** (closes
   NAI-5 test gap #1). Non-delayed NPC with a registered `ai_despawn`
   script fixture gets dispatched and removed from the queue.

**Roadmap:** NAI-16 is the first "deferral sweep" bundle in the NAI
series. Fidelity risk: **Low**. Each item is line-for-line symmetric
with an existing Go pattern OR a direct TS port. No new subsystems, no
new abstractions, no new packages. One interface signature change
(`ActiveNpc.ChangeType`) with 5 enumerated call sites.

**Tech Stack:** Go 1.26+. No new packages. Existing `pkg/script`
(`ActiveNpc` interface, `handlers_npc.go`), `pkg/objtype` (`HuntType`),
`modules/world` (`Npc`, `Server`, `npcEventQueue`, `npc_hunt.go`,
`npc_ai.go`, `npc_masks.go`, `npc_script.go`).

## Goal

After NAI-16 ships:

1. **`(*Npc).huntPlayers`** applies `checkNotCombatSelf` inside the
   NAI-15 combat guard, mirroring TS `Npc.ts:946-948`. The filter uses
   the existing `(*Npc).NpcVarN(id)` reader.
2. **`ActiveNpc.ChangeType`** interface extends to `ChangeType(newType,
   duration int)`. `*Npc.ChangeType` early-returns on `duration < 1 ||
   n.dead`, then writes `typeId`, recomputes `uid`, sets
   `lifecycleTick = duration`, and finally sets the mask payload.
   `handleNpcChangeType` passes duration through instead of discarding
   it.
3. **`npc_ai_test.go`** gains `TestNpcTurnRespawnAliveMorphReverts`
   directly exercising the morph-revert branch.
4. **`npc_event_queue_test.go`** gains
   `TestProcessNpcEventQueueHappyPathFire` directly exercising script
   dispatch + queue removal.
5. **`nai_followups.md`** updated: Item A entry marked Resolved with
   a note that the "no NPC-vars infra" claim was stale (S6a shipped
   it); Item B entry marked Resolved; Items C/D entries marked
   Resolved.
6. **Deferred-filter doc comment** in `npc_hunt.go:98-103` rewritten:
   `checkNotCombatSelf` line removed; surviving deferrals
   (`checkNotBusy`, `checkNotTooStrong`, `checkInv`) stay.
7. **New DEFERRED comment** added at `*Npc.ChangeType` pointing at
   three items left out of scope:
   - TS `changeType`'s optional `reset` parameter and the stats-array
     reset branch (TS `Npc.ts:436-443`).
   - `NPC_CHANGETYPE_KEEPALL` opcode handler (currently a reserved
     constant at `pkg/script/opcode.go:243` with no dispatch).
   - TS `type === baseType && lifecycle === RESPAWN → setLifeCycle(-1)`
     fast-path (TS `Npc.ts:444-448`).

## Architecture

### §1. File-by-file delta

| File | Change | Item |
|------|--------|------|
| `modules/world/npc_hunt.go` | Insert `checkNotCombatSelf` filter block after existing `checkNotCombat` block (lines 172-177). Rewrite deferred-filter doc-comment block (lines 98-103) | A |
| `modules/world/npc_hunt_test.go` | + `TestHuntPlayersCheckNotCombatSelf` (3 sub-cases) + `TestHuntPlayersCheckNotCombatSelfOutsideGuard` (1 sub-case) | A |
| `pkg/script/active.go` | `ChangeType(newType int)` → `ChangeType(newType, duration int)` on `ActiveNpc` interface. Update doc comment | B |
| `pkg/script/handlers_npc.go` | Rewrite `handleNpcChangeType`: pop duration, pass to interface call. Update doc comment (drop "S6c discards" phrasing) | B |
| `modules/world/npc_masks.go` | Replace `*Npc.ChangeType` body: `duration < 1 \|\| n.dead` early return; then set `typeId`, recompute `uid`, set `lifecycleTick`, set `changeTypeID`, raise mask. Add DEFERRED doc comment | B |
| `pkg/script/handlers_npc_test.go` | `mockNpc.ChangeType` takes two ints; `changeTypeCalls` becomes `[]struct{ newType, duration int }`. + `TestHandleNpcChangeTypePassesDuration` | B |
| `pkg/script/handlers_player_test.go` | `mockActiveNpc.ChangeType(newType, duration int)` stub signature update | B |
| `modules/world/npc_test.go` | Update `TestNpcChangeTypeSetsMask` to new signature + add `typeId`/`lifecycleTick` assertions. + `TestNpcChangeTypeDurationZeroNoOp` + `TestNpcChangeTypeDeadNoOp` | B |
| `modules/world/npc_ai_test.go` | + `TestNpcTurnRespawnAliveMorphReverts` | C |
| `modules/world/npc_event_queue_test.go` | + `TestProcessNpcEventQueueHappyPathFire` | D |
| `~/.claude/projects/.../memory/nai_followups.md` | Add Resolved preambles to "From NAI-5 → npc_changetype duration wiring" and "From NAI-5 → test gaps" and "From NAI-8 → Deferred filters (#4 checkNotCombatSelf)" | All |

No new files. No new packages.

### §2. `checkNotCombatSelf` filter (Item A)

Insert directly after the `checkNotCombat` block at
`modules/world/npc_hunt.go:172-177`, inside the `if applyCombatGuard`
block. Replaces the existing DEFERRED comment at lines 178-179.

```go
// checkNotCombatSelf (TS:946-948): skip candidate if this NPC's own
// combat-tracker varn was written within the past 8 ticks. Symmetric
// to checkNotCombat above, but reads the NPC side (n.NpcVarN) instead
// of the player side (p.Varp). Gated by the same outer combat guard.
if hunt.CheckNotCombatSelf != -1 &&
    int(n.NpcVarN(hunt.CheckNotCombatSelf))+8 > s.currentTick {
    continue
}
```

The doc-comment block at `npc_hunt.go:98-103` is rewritten from:

```
// Filters DEFERRED (infra missing; each TS line cited):
//   - checkNotBusy             (TS:931-933)       — no Player.Busy()
//   - checkNotTooStrong        (TS:939-941)       — wilderness + combat-level
//   - checkNotCombatSelf       (TS:946-948)       — needs NPC-vars infra
//                                                   (VarNpcType, Npc.vars, Npc.Varp)
//   - checkInv                 (TS:959-969)       — inventory queries
```

to:

```
// Filters DEFERRED (infra missing; each TS line cited):
//   - checkNotBusy             (TS:931-933)       — no Player.Busy()
//   - checkNotTooStrong        (TS:939-941)       — wilderness + combat-level
//   - checkInv                 (TS:959-969)       — inventory queries
```

And the "Filter coverage" list at `npc_hunt.go:87-93` gets one new line:

```
//   - checkNotCombatSelf       (NAI-16, TS:946-948)
```

### §3. `ChangeType` duration wiring (Item B)

**TS source** (verified, `Engine-TS/.../Npc.ts:427-449`):

```ts
changeType(type: number, duration: number, reset: boolean = true) {
    if (!this.isActive || duration < 1) {
        return;
    }
    this.type = type;
    this.masks |= NpcInfoProt.CHANGE_TYPE;
    this.uid = (type << 16) | this.nid;
    this.resetOnRevert = reset;

    if (reset) {
        // ...stats-reset branch (DEFERRED — needs baseLevels/levels arrays)...
    }
    if (type === this.baseType && this.lifecycle === EntityLifeCycle.RESPAWN) {
        this.setLifeCycle(-1);   // DEFERRED — fast-path
    } else {
        this.setLifeCycle(duration);
    }
}
```

**Go port** (replace entire body at `modules/world/npc_masks.go:16-19`):

```go
// ChangeType morphs the NPC to newType and schedules a revert to
// baseType after `duration` ticks. Mirrors TS Npc.changeType at
// Engine-TS/.../Npc.ts:427-449.
//
// Semantics:
//   - No-op when duration < 1 (TS guard; rejects 0 and negatives in
//     one check) OR when the NPC is dead (TS `!this.isActive`).
//   - On success: writes typeId, recomputes uid, writes lifecycleTick
//     (consumed by the Events block at npc_ai.go:27-43 to fire
//     revertType when it hits 0 on RESPAWN+alive), writes the mask
//     payload field changeTypeID, raises NpcMaskChangeType.
//
// DEFERRED (TS parity gaps, left for a follow-up sub-spec):
//   - Stats-reset branch (TS:436-443) — requires baseLevels/levels
//     arrays on *Npc which don't exist yet. Current engine has only
//     curHP/baseHP; a full 6-stat array port is a separate concern.
//   - The optional `reset=false` flag and its NPC_CHANGETYPE_KEEPALL
//     opcode (opcode 2506 is a reserved constant at
//     pkg/script/opcode.go:243 with no handler). Wiring KEEPALL
//     requires the stats-array infra above, so both land together.
//   - The `type === baseType && RESPAWN → setLifeCycle(-1)`
//     fast-path (TS:444-445) — minor corner case; current behavior
//     writes lifecycleTick=duration unconditionally, which fires a
//     harmless no-op revert at tick 0 (revertType is idempotent when
//     typeId == baseType).
func (n *Npc) ChangeType(newType, duration int) {
    if duration < 1 || n.dead {
        return
    }
    n.typeId = newType
    n.uid = (n.typeId << 16) | n.nid
    n.lifecycleTick = duration
    n.changeTypeID = newType
    n.masks |= rsbuf.NpcMaskChangeType
}
```

**`ActiveNpc` interface** at `pkg/script/active.go:341`:

```go
// ChangeType morphs the NPC to newType and schedules a revert after
// `duration` ticks. No-op when duration < 1 or when the NPC is dead.
// Mirrors TS Npc.changeType at Engine-TS/.../Npc.ts:427-449.
// DEFERRED: `reset=false` (NPC_CHANGETYPE_KEEPALL opcode 2506) — see
// implementation for the full list.
ChangeType(newType, duration int)
```

**Handler** at `pkg/script/handlers_npc.go:176-184`:

```go
// handleNpcChangeType pops (newType, duration) in TS order (duration
// on top). Matches TS NpcOps.ts:457-462.
func handleNpcChangeType(s *ScriptState) error {
    if err := requireActiveNpc(s, "NPC_CHANGETYPE"); err != nil {
        return err
    }
    duration := s.PopInt()
    newType := s.PopInt()
    s.ActiveNpc.ChangeType(newType, duration)
    return nil
}
```

**Call-site enumeration** (per the `enumerate_all_sites.md` memory —
every `ChangeType(` site that needs updating):

| # | Site | Kind |
|---|------|------|
| 1 | `pkg/script/active.go:341` | interface decl |
| 2 | `pkg/script/handlers_npc.go:182` | handler call |
| 3 | `modules/world/npc_masks.go:16` | concrete impl |
| 4 | `pkg/script/handlers_npc_test.go:79` | `mockNpc` stub |
| 5 | `pkg/script/handlers_player_test.go:31` | `mockActiveNpc` stub |
| 6 | `modules/world/npc_test.go:43` | existing test call |

One additional reference is a **non-ChangeType** site that uses `n.typeId
= 99` to simulate a changetype directly in `npc_event_queue_test.go:37`
— unaffected by the signature change.

### §4. RESPAWN+alive morph-revert test (Item C)

Unit-tests the `lifecycle=Respawn && !dead` branch at
`modules/world/npc_ai.go:37-40`. Must manually set morphed state (do
NOT route through `ChangeType` — memory's "simulate a changetype"
phrasing explicitly isolates this branch from Item B).

Test outline:

```go
func TestNpcTurnRespawnAliveMorphReverts(t *testing.T) {
    s := newTestServer(t)
    n := newNpcForLifecycleTest(t)  // existing fixture
    n.server = s
    // Manually morph: typeId != baseType simulates post-changetype state.
    n.typeId = 99
    n.uid = (99 << 16) | n.nid
    n.lifecycle = NpcLifecycleRespawn
    n.dead = false
    n.lifecycleTick = 3

    for i := 0; i < 3; i++ { n.turn(s) }

    if n.typeId != n.baseType {
        t.Errorf("typeId: got %d, want baseType %d", n.typeId, n.baseType)
    }
    if n.masks & rsbuf.NpcMaskChangeType == 0 {
        t.Error("revertType should raise NpcMaskChangeType")
    }
}
```

### §5. `processNpcEventQueue` happy-path test (Item D)

Unit-tests the fire-and-remove path of `processNpcEventQueue` at
`modules/world/npc_event_queue.go:36-48`. Non-delayed NPC, registered
`TriggerAiDespawn` script fixture, manually enqueued request.

**Fixture choice:** `ScriptFile{OpReturn}` is the minimal opcode that
executes cleanly. Observability: `runNpcScript` → `resumeOrFinishNpc`
→ `script.Execute` returns `Finished`, then `npc.ClearActiveScript()`.
The test asserts two side effects:

1. `len(s.npcEventQueue) == 0` after the call (removal).
2. The script ran to completion without leaving `n.activeScript` set.

Fallback if `OpReturn` is not sufficient as an observation point: wrap
`s.runNpcScript` under a test-only counter via the existing
`scriptProvider.RegisterForTest`-style registration path. Pin down at
plan-authoring time.

Test outline:

```go
func TestProcessNpcEventQueueHappyPathFire(t *testing.T) {
    s := newTestServer(t)
    n := newNpcForLifecycleTest(t)
    n.server = s

    sf := &script.ScriptFile{Name: "ai_despawn_fixture",
        Opcodes: []script.Opcode{script.OpReturn}}
    s.scriptProvider = script.NewProviderForTest(
        map[script.TriggerKey]*script.ScriptFile{
            {Trigger: script.TriggerAiDespawn, TypeID: n.typeId}: sf,
        })

    s.npcEventQueue = []NpcEventRequest{
        {Type: NpcEventDespawn, Script: sf, Npc: n},
    }

    s.processNpcEventQueue()

    if len(s.npcEventQueue) != 0 {
        t.Errorf("queue should be empty after fire; got %d entries",
            len(s.npcEventQueue))
    }
    if n.activeScript != nil {
        t.Error("activeScript should be cleared after Finished run")
    }
}
```

## Testing strategy

Cross-check per task-code-block (per `plan_test_coverage_crosscheck.md`
memory):

| Task | Production change | Test(s) ensuring it |
|------|-------------------|---------------------|
| A | checkNotCombatSelf filter body | `TestHuntPlayersCheckNotCombatSelf` (3 sub-cases: `-1 accepts`, `var+8 > tick skips`, `var+8 <= tick accepts`) |
| A | Filter gated by outer combat guard | `TestHuntPlayersCheckNotCombatSelfOutsideGuard` (target==player → filter doesn't fire regardless of varn) |
| B | ChangeType writes typeId, uid, lifecycleTick, mask | `TestNpcChangeTypeSetsMask` (updated: `n.ChangeType(42, 100)`; asserts typeId==42, uid recomputed, lifecycleTick==100, mask raised) |
| B | ChangeType is no-op when duration < 1 | `TestNpcChangeTypeDurationZeroNoOp` (ChangeType(42, 0) leaves typeId, mask, lifecycleTick unchanged) |
| B | ChangeType is no-op when dead | `TestNpcChangeTypeDeadNoOp` (n.dead=true; ChangeType(42, 100) no-op) |
| B | Handler pops duration + threads it through | `TestHandleNpcChangeTypePassesDuration` (mock assertion) |
| C | npc_ai Events-block RESPAWN+alive fires revertType | `TestNpcTurnRespawnAliveMorphReverts` |
| D | processNpcEventQueue fires script + removes from queue | `TestProcessNpcEventQueueHappyPathFire` |

**Total tests touched:** 8 test functions — 7 new + 1 updated.
Breakdown: A = 2 new; B = 3 new + 1 updated (`TestNpcChangeTypeSetsMask`
gets new assertions for typeId/uid/lifecycleTick); C = 1 new; D = 1 new.

## Error handling / edge cases

| Scenario | Behavior |
|----------|----------|
| `hunt.CheckNotCombatSelf == -1` | Filter short-circuits; candidate accepted (TS disable sentinel) |
| `hunt.CheckNotCombatSelf` points at a never-written varn | `NpcVarN` returns 0; `0+8 > s.currentTick` fires only in ticks 0-7. Matches TS default-0 semantics |
| Outer combat guard false (target==player OR multi zone) | Neither `checkNotCombat` NOR `checkNotCombatSelf` fires |
| `ChangeType(newType, 0)` | Total no-op. No typeId write, no mask. Matches TS `duration < 1` guard |
| `ChangeType(newType, -1)` | Total no-op (same guard as above) |
| `ChangeType` when `n.dead` | No-op. Matches TS `!this.isActive` guard |
| `ChangeType(baseType, duration)` | Writes `typeId=baseType`, schedules revert. Revert at tick 0 is a no-op (revertType only acts when `typeId != baseType`). DEFERRED: TS fast-path that calls `setLifeCycle(-1)` |
| `ChangeType` called twice in one tick | Second call wins: typeId/mask/lifecycleTick all overwritten. Matches TS (no debounce) |
| `processNpcEventQueue` with delayed NPC | Skipped, stays in queue (existing behavior; not tested here — NAI-5 already covers via `TestProcessNpcEventQueueSkipsDelayedNpcs`) |
| `processNpcEventQueue` with nil Script | Would crash `runNpcScript` — but producer path (`npc_ai.go:51-57`) only enqueues when `sf != nil` via `GetByTrigger`. Not a user-reachable state; no test needed |

## Gotchas / tracked deviations

1. **`Npc.varns` lazy allocation unchanged.** `SetNpcVarN` grows the
   slice on first write; `NpcVarN` returns 0 for unwritten ids. Item A
   relies on this: a cold NPC reading `NpcVarN(X)` before any script
   has written varn X returns 0, and `0 + 8 > currentTick` is only true
   for `currentTick < 8`. In practice the NPC must have engaged in
   combat (via a script that calls `npc_setvar`) for this filter to
   skip anyone. Matches TS semantics where `this.vars[id] === undefined`
   coerces to 0 via the `as number` cast.

2. **`n.typeId` is `int` in Go; TS uses `number`.** No overflow concern:
   newType values come from `NpcTypeValid.check` upstream (in the
   handler's TS counterpart, `NpcOps.ts:459`). Go's handler doesn't
   currently re-validate — this is a pre-existing gap, out of scope for
   NAI-16 but worth a breadcrumb (see Out-of-scope below).

3. **`n.uid` recomputation.** Writing `n.typeId = newType` without
   recomputing `n.uid` would leave `uid` inconsistent. Both `ChangeType`
   and `revertType` update `uid`; keeping them in sync is a fragile
   invariant that a future refactor might break. Consider a private
   `setTypeId(newType)` helper in a later pass — out of scope here
   (single call site per direction).

4. **Test-fixture `OpReturn` observability.** Item D's test uses the
   absence of `n.activeScript` + empty queue as proof of dispatch. If
   fixture investigation at plan-authoring time reveals this is
   insufficient (e.g., `OpReturn` doesn't route through
   `resumeOrFinishNpc`'s clear-activeScript path), the test falls back
   to a custom test-only opcode or a counter on `scriptProvider`.

5. **The `nai_followups.md` "blocker" claim was stale.** Item A's
   resolution preamble explicitly calls this out so future memory
   verification grep-finds the lesson: the memory entry claimed
   `VarNpcType` + `Npc.vars` + `Npc.Varp` were all missing; HEAD ships
   all three (under the names `varns` + `NpcVarN`). Per
   `verify_implementer_claims.md`: memories that name specific symbols
   claim those symbols exist *when the memory was written*, not *now*.

## Out of scope (tracked)

These items are deliberately NOT addressed by NAI-16. Add to
`nai_followups.md` if not already tracked:

1. **`NPC_CHANGETYPE_KEEPALL` handler** (opcode 2506, `pkg/script/
   opcode.go:243` is a reserved constant with no `handle*` function).
   Requires: the `reset=false` variant of `ChangeType` → baseLevels/
   levels arrays for stats-preservation semantics. Suggested follow-up
   sub-spec: "NPC stat arrays + KEEPALL".
2. **Stats reset on `ChangeType`** (TS `Npc.ts:436-443`). Requires the
   same baseLevels/levels arrays. Bundle with #1.
3. **`type === baseType && RESPAWN → setLifeCycle(-1)` fast-path**
   (TS `Npc.ts:444-445`). Minor corner case; current behavior is
   harmless (revertType idempotent when `typeId == baseType`).
   Low-priority polish.
4. **`NpcTypeValid` check on handler input** (pre-existing gap; the TS
   handler at NpcOps.ts:459 validates the popped id, Go does not). Part
   of the broader "NumberNotNull / *Valid opcode gate" audit track
   tracked in `nai_followups.md` "From NAI-2".
5. **Surviving `huntPlayers` filter deferrals** (`checkNotBusy`,
   `checkNotTooStrong`, `checkInv`). Unchanged by NAI-16.

## References

- TS source:
  - `LostCityRS/Engine-TS/src/engine/entity/Npc.ts:195-198` (NPC `getVar`)
  - `LostCityRS/Engine-TS/src/engine/entity/Npc.ts:427-449` (`changeType`)
  - `LostCityRS/Engine-TS/src/engine/entity/Npc.ts:946-948` (filter consumer)
  - `LostCityRS/Engine-TS/src/engine/script/handlers/NpcOps.ts:457-462` (handler)
- Go source at HEAD:
  - `modules/world/npc_hunt.go:98-180` (huntPlayers + DEFERRED site)
  - `modules/world/npc_masks.go:16-19` (current ChangeType impl)
  - `modules/world/npc_script.go:53-77` (existing NpcVarN/SetNpcVarN)
  - `modules/world/npc_ai.go:26-63` (Events block + revertType fire)
  - `modules/world/npc_event_queue.go:36-48` (processNpcEventQueue)
  - `pkg/script/active.go:339-341` (ActiveNpc.ChangeType decl)
  - `pkg/script/handlers_npc.go:173-184` (current handler with discard)
- Prior specs:
  - NAI-5 (`docs/superpowers/specs/2026-04-22-nai-5-npc-events-block-design.md` — revertType + Events block)
  - NAI-8 (`docs/superpowers/specs/2026-04-22-nai-8-hunt-players-design.md` — huntPlayers scaffolding)
  - NAI-15 (`docs/superpowers/specs/2026-04-23-nai-15-varp-filter-bundle-design.md` — outer combat guard + checkNotCombat)
  - S6a (`docs/superpowers/specs/2026-04-21-runescript-s6a-active-npc-reads-design.md` — `NpcVarN`/`SetNpcVarN`/`varns` infrastructure)
