# NAI-10 — `consumeHuntTarget` + Target Glue

Convert a hunt-phase result (`huntTarget`) into a consumable interaction
state (`target`, `targetOp`) exactly once per tick, matching TS
`Npc.consumeHuntTarget` at `Engine-TS/src/engine/entity/Npc.ts:887-919`.
Also port the `QUEUE1..QUEUE20` direct-dispatch branch of the same TS
method.

Part of the NPC AI tick decomposition roadmap. Blocker: NAI-7 (hunt
core; any variant — NAI-8 or NAI-9 — is sufficient since this sub-spec
only reads `n.huntTarget` and doesn't care which variant populated it).
Roadmap fidelity risk: Low — one-line glue; the main foot-gun is the
`huntTarget = nil` + `huntClock = 0` nil-out step, which tests guard
explicitly.

**Tech Stack:** Go 1.26+, existing `pkg/script` (`TriggerAiQueue1`,
`ServerTriggerType`, `ScriptProvider.GetByTrigger`), existing
`(*Server).runNpcScript` (NAI-2), existing `pkg/objtype.HuntType` fields
(`FindNewMode`, `FindKeepHunting`, `Type`).

## Goal

After NAI-10 ships:

1. `(*Server).consumeHuntTarget(n *Npc)` exists and is called from
   `Npc.turn()` between `processNpcHunt` and `processNpcRegen`, matching
   TS call order at `Npc.ts:174`.
2. When `huntTarget != nil` and `hunt.FindNewMode ∈ [NPCModeQueue1, NPCModeQueue20]`
   (`[47, 66]`), the corresponding `TriggerAiQueueN` script fires
   immediately via `runNpcScript` (NOT via the NAI-3 queue system).
3. When `huntTarget != nil` and `FindNewMode` is outside the QUEUE
   range, `n.target = n.huntTarget` and `n.targetOp = hunt.FindNewMode`
   are written — ready for NAI-11 to consume.
4. In both branches: `n.huntTarget = nil`, `n.huntClock = 0`, and
   `n.huntMode = -1` when `hunt.FindKeepHunting == false`.
5. `pkg/objtype` exposes `NPCModeQueue1` through `NPCModeQueue20`
   constants (values 47 through 66).

## Scope — what's IN

1. **`consumeHuntTarget` method** at the bottom of
   `modules/world/npc_hunt.go`, adjacent to `huntAll`. Mirrors TS layout
   (`Npc.ts:887-919`, immediately after `huntAll` at `:249-277`).

2. **Single call site** in `modules/world/npc_ai.go` inside `Npc.turn()`,
   placed between `s.processNpcHunt(n)` and `s.processNpcRegen(n)`.

3. **NpcMode QUEUE constants** in `pkg/objtype/npctype.go`,
   co-located with existing `NPCModeNull/None/Wander`:

   ```go
   NPCModeQueue1  = 47
   NPCModeQueue2  = 48
   ...
   NPCModeQueue20 = 66
   ```

   All 20 values are emitted even though only `NPCModeQueue1` and
   `NPCModeQueue20` are used by the range check — matches the
   enumeration shape in TS `NpcMode.ts:76-95` and avoids magic
   numbers in any future consumer.

4. **Unit and integration tests** in new file
   `modules/world/npc_hunt_test.go` covering: interaction branch,
   QUEUE branch (including boundary QUEUE20), no-op guards (nil
   huntTarget / HuntModeOff / invalid huntMode), FindKeepHunting
   toggle, and one tick-level placement test. See "Test strategy"
   below.

## Scope — what's OUT (non-goals)

1. **`apRange`, `apRangeCalled`, `targetSubject.com/type` fields.** TS
   `setInteraction` at `PathingEntity.ts:510-530` writes these, but
   they're consumed exclusively by `processMovementInteraction` —
   NAI-11's scope. Per memory "Dead-API YAGNI polish pattern", we
   don't ship struct fields with zero NAI-10 consumers. NAI-11 adds
   them when it needs them.

2. **`Interaction.SCRIPT` / `Interaction.ENGINE` enum** (TS
   `Interaction.ts`). No consumer in NAI-10; NAI-11 introduces it.

3. **`target.isValid()` pre-check** (TS `PathingEntity.ts:511-513`).
   In the NAI-10 window (same tick as hunt found the target, no
   cleanup between), this check is always true. NAI-11's interaction
   processing will add a real validity gate when it actually paths
   toward the target.

4. **`target` back-population from other opcode handlers.** `n.target`
   today is already set by `handleNpcSetInteraction`-equivalent paths
   (if any exist) or defaulted to nil. NAI-10 only adds the
   hunt-driven write path.

5. **Observer/hunt-gate changes.** NAI-7 + NAI-9 fully closed the
   PAUSEHUNT path; NAI-10 is downstream of all hunt gating.

## Architecture

### Call placement in `Npc.turn()`

```
// turn() ordering after NAI-10 (matches TS Npc.ts:110-185):
//   [script prefix: delayed-expire, resume-suspended]
//   [Events: lifecycle / despawn / revert]
//   [isValid gate: return if dead || delayed]
//   s.processNpcHunt(n)       // NAI-7..9 — populates n.huntTarget
//   s.consumeHuntTarget(n)    // NAI-10 — converts huntTarget → target/targetOp or fires queueN
//   s.processNpcRegen(n)      // NAI-6
//   s.processNpcTimer(n)      // NAI-4
//   s.processNpcQueue(n)      // NAI-3
//   [movement / wander / patrol]
```

Exact TS line mapping: `Npc.ts:158-171` = processNpcHunt; `:174` =
consumeHuntTarget; `:176` = processRegen; `:178` = processTimers;
`:180` = processQueue.

### Control flow of `consumeHuntTarget`

```
1. Entry guard:
     return if huntTarget == nil
     return if huntTypes nil || huntMode out of [0, len(Configs))
     return if Configs[huntMode] == nil || Configs[huntMode].Type == HuntModeOff

2. Branch on hunt.FindNewMode:
     QUEUE1..QUEUE20:
       if n.typ != nil && scriptProvider != nil:
         trigger = TriggerAiQueue1 + (FindNewMode - NPCModeQueue1)
         sf = scriptProvider.GetByTrigger(trigger, n.typeId, n.typ.Category)
         runNpcScript(sf, n, nil, nil)   // safe with nil sf
       // target/targetOp NOT set — script owns subsequent state

     else:
       n.target = n.huntTarget
       n.targetOp = hunt.FindNewMode

3. Common tail (both branches):
     n.huntTarget = nil
     n.huntClock = 0

4. Stop-hunting clause:
     if !hunt.FindKeepHunting:
       n.huntMode = -1
```

### Why the bounds check is not redundant with `processNpcHunt`

`processNpcHunt` validates `n.huntMode` before `huntAll` runs. Between
the two calls, `n.huntMode` cannot change from the NAI-3 queue path
(those enqueues execute on a *later* tick's `processNpcQueue`). It
technically *could* be mutated by a script fired inline during
`processNpcHunt` — but no existing NAI-7/8/9 code does inline script
dispatch from the hunt phase. Still, the defensive check mirrors TS's
`typeof hunt === 'undefined'` guard and costs four lines. Cheaper than
reasoning about re-entry invariants at every future edit.

## Implementation details

### `modules/world/npc_hunt.go` — add at bottom

```go
// consumeHuntTarget converts a hunt result into interaction state.
// Matches TS Npc.consumeHuntTarget at Engine-TS/.../Npc.ts:887-919.
//
// QUEUE1..QUEUE20 branch: fires AiQueueN directly (not enqueued) and
// leaves target/targetOp untouched — the script owns subsequent state.
// Else branch: writes n.target + n.targetOp for NAI-11 to consume.
//
// DEVIATION from TS setInteraction: apRange, apRangeCalled, and
// targetSubject fields are not written — NAI-11 scope, not yet on *Npc.
func (s *Server) consumeHuntTarget(n *Npc) {
    if n.huntTarget == nil {
        return
    }
    if s.huntTypes == nil ||
        n.huntMode < 0 ||
        n.huntMode >= len(s.huntTypes.Configs) {
        return
    }
    hunt := s.huntTypes.Configs[n.huntMode]
    if hunt == nil || hunt.Type == objtype.HuntModeOff {
        return
    }

    if hunt.FindNewMode >= objtype.NPCModeQueue1 &&
        hunt.FindNewMode <= objtype.NPCModeQueue20 {
        if n.typ != nil && s.scriptProvider != nil {
            trigger := script.TriggerAiQueue1 +
                script.ServerTriggerType(hunt.FindNewMode-objtype.NPCModeQueue1)
            sf := s.scriptProvider.GetByTrigger(trigger, n.typeId, n.typ.Category)
            s.runNpcScript(sf, n, nil, nil)
        }
    } else {
        n.target = n.huntTarget
        n.targetOp = hunt.FindNewMode
    }

    n.huntTarget = nil
    n.huntClock = 0

    if !hunt.FindKeepHunting {
        n.huntMode = -1
    }
}
```

### `modules/world/npc_hunt.go` — add `script` import

The existing imports are `math/rand/v2`, `pkg/objtype`, `pkg/rsbuf`.
Add `github.com/zsrv/goscape/pkg/script`.

### `modules/world/npc_ai.go` — call-site insertion

In `Npc.turn()` after line 73 (`s.processNpcHunt(n)`), before line 74
(`s.processNpcRegen(n)`):

```go
s.processNpcHunt(n)     // NAI-7..9
s.consumeHuntTarget(n)  // NAI-10 — matches TS Npc.ts:174
s.processNpcRegen(n)    // NAI-6
```

### `pkg/objtype/npctype.go` — add constants

Extend the existing `const (...)` block at line 42-48:

```go
// NPCMode values (subset of rs-server-225/entity.NPCMode constants relevant
// to the current scope of the port).
const (
    NPCModeNull   = -1
    NPCModeNone   = 0
    NPCModeWander = 1

    // QUEUE1..QUEUE20 are `ai_queueN`-dispatch modes, consumed by
    // Npc.consumeHuntTarget (NAI-10). See TS NpcMode.ts:76-95.
    NPCModeQueue1  = 47
    NPCModeQueue2  = 48
    NPCModeQueue3  = 49
    NPCModeQueue4  = 50
    NPCModeQueue5  = 51
    NPCModeQueue6  = 52
    NPCModeQueue7  = 53
    NPCModeQueue8  = 54
    NPCModeQueue9  = 55
    NPCModeQueue10 = 56
    NPCModeQueue11 = 57
    NPCModeQueue12 = 58
    NPCModeQueue13 = 59
    NPCModeQueue14 = 60
    NPCModeQueue15 = 61
    NPCModeQueue16 = 62
    NPCModeQueue17 = 63
    NPCModeQueue18 = 64
    NPCModeQueue19 = 65
    NPCModeQueue20 = 66
)
```

## Test strategy

All in new file `modules/world/npc_hunt_test.go`. Use existing
fixtures:

- `newServerForScriptTest`, `newNpcForLifecycleTest`,
  `addNpcToServerAt` — from `npc_hunt_entities_test.go` and
  `npc_script_test.go`.
- `newNoopScriptFile(t, trigger, typeID, -1)` + `s.scriptProvider =
  script.NewProvider()` + `s.scriptProvider.Register(sf)` — the
  registered-script pattern established in
  `interaction_trigger_test.go:258-273`. A fired `OpReturn`-only
  script reaches `script.Finished` without side effects; observe
  that `runNpcScript` was reached by asserting the script's
  `Execution == script.Finished` via the stored state, or (simpler)
  by registering a script whose opcodes include a test-observable
  mutation (`NpcSetTimer` with a unique interval is the cheapest
  available side-effect; see `npc_script_test.go:88-92` for the
  `OpPushConstantInt, OpNpcSetTimer, OpReturn` pattern).

### Unit tests — `consumeHuntTarget` direct

1. **`TestConsumeHuntTargetInteractionBranchSetsTarget`** —
   huntTarget = some player; hunt valid with `FindNewMode = 4`
   (PLAYERFOLLOW, non-QUEUE); call consumeHuntTarget; assert
   `n.target == player` and `n.targetOp == 4`.

2. **`TestConsumeHuntTargetInteractionBranchClearsHuntState`** —
   same setup as #1; assert `n.huntTarget == nil` and `n.huntClock == 0`
   post-call.

3. **`TestConsumeHuntTargetQueueBranchFiresScript`** —
   huntTarget set; hunt with `FindNewMode = 49` (QUEUE3); register a
   script for `(TriggerAiQueue3, n.typeId)` using `newNoopScriptFile`
   + `s.scriptProvider.Register` (pattern at
   `interaction_trigger_test.go:258-273`); call consumeHuntTarget.
   Observable proof of dispatch: use the `OpPushConstantInt,
   OpNpcSetTimer, OpReturn` body pattern from
   `npc_script_test.go:88-92` with a unique timerInterval value;
   assert `n.timerInterval` equals that value post-call (proves the
   script ran and the script engine reached the NPC back-reference).

4. **`TestConsumeHuntTargetQueueBranchDoesNotSetTarget`** —
   pre-set `n.target = someOtherEntity` and `n.targetOp = 999`; set
   huntTarget + hunt with `FindNewMode = QUEUE3`; call
   consumeHuntTarget; assert `n.target == someOtherEntity` and
   `n.targetOp == 999` (unchanged). Guards the "nil-out step that's
   easy to forget" the roadmap flagged.

5. **`TestConsumeHuntTargetQueueBranchBoundaryQueue20`** —
   `FindNewMode = 66`; register fixture for `TriggerAiQueue20`; assert
   dispatched trigger is `TriggerAiQueue20`. Guards the `+19` offset
   arithmetic (off-by-one would dispatch Queue19 or a out-of-range
   trigger).

6. **`TestConsumeHuntTargetNilHuntTargetNoOp`** —
   `n.huntTarget == nil`; pre-set `n.target`, `n.targetOp`,
   `n.huntClock`, `n.huntMode`; call consumeHuntTarget; assert all
   four unchanged (no silent state mutation).

7. **`TestConsumeHuntTargetHuntModeOffNoOp`** —
   huntTarget set; `hunt.Type == HuntModeOff`; call; assert
   `n.huntTarget` and `n.huntClock` unchanged (entry guard holds).

8. **`TestConsumeHuntTargetInvalidHuntModeNoOp`** —
   run twice: once with `n.huntMode == -1`, once with
   `n.huntMode == len(Configs)`; assert both return without panic and
   without clearing huntTarget.

9. **`TestConsumeHuntTargetFindKeepHuntingFalseClearsHuntMode`** —
   huntTarget set; `hunt.FindKeepHunting == false`; original
   `n.huntMode == 5`; call; assert `n.huntMode == -1` post-call.

10. **`TestConsumeHuntTargetFindKeepHuntingTrueKeepsHuntMode`** —
    same as #9 but `FindKeepHunting == true`; assert
    `n.huntMode == 5` (preserved).

### Integration test — call placement in `turn()`

11. **`TestNpcTurnHuntAndConsumeSetsTarget`** —
    end-to-end: seed a huntable player in range, register a PLAYER
    hunt type with `FindNewMode = PLAYERFOLLOW (4)` and
    `FindKeepHunting = true`; ensure hunt will fire this tick
    (`huntClock >= rate-1`, observers > 0); call `n.turn(s)`; assert
    `n.target == player`, `n.targetOp == 4`, `n.huntTarget == nil`,
    `n.huntClock == 0`, `n.huntMode` unchanged. Proves consumeHuntTarget
    runs **after** processNpcHunt (so it sees huntTarget) and **before**
    any later phase could clobber it.

### Test-count justification

11 tests against ~30 LOC of production code is a high ratio but
consistent with NAI-7/8/9 density. Each test guards a distinct branch
or invariant (including the roadmap-flagged nil-out step), so dropping
any of them would leave a specific failure mode uncovered.

## Deviations tracked

Per memory "True-to-TS fidelity gate", behavioural divergences are
recorded with rationale + follow-up.

1. **`setInteraction` partial port.** `apRange=10`, `apRangeCalled=false`,
   `targetSubject.com=-1`, `targetSubject.type=target.type` are NOT
   written. **Rationale:** consumers are NAI-11 scope; per "Dead-API
   YAGNI polish pattern", struct fields with zero current consumers
   aren't shipped. **Follow-up:** NAI-11 adds these fields on *Npc
   and writes them from an expanded consumeHuntTarget (or a dedicated
   setInteraction helper).

2. **`target.isValid()` pre-check omitted.** **Rationale:** same-tick
   as hunt found target; no cleanup path runs between. **Follow-up:**
   NAI-11 adds validity gating at the interaction-processing site,
   which is where stale-target handling actually matters.

3. **`Interaction` enum not introduced.** **Rationale:** no current
   consumer distinguishes SCRIPT from ENGINE modes. **Follow-up:**
   NAI-11 introduces the enum when its dispatch path needs to branch
   on it.

Each deviation gets its own inline breadcrumb (`// DEVIATION: ...`) in
the production code pointing to the follow-up sub-spec.

## TS reference

Primary: `Engine-TS/src/engine/entity/Npc.ts:887-919` (`consumeHuntTarget`).

Supporting:
- `Npc.ts:174` — call site in `turn()`.
- `PathingEntity.ts:510-530` — `setInteraction` (partial-port source).
- `Interaction.ts` — `SCRIPT`/`ENGINE` enum (deferred).
- `NpcMode.ts:76-95` — QUEUE1..QUEUE20 constant values.
- `rsbuf.d.ts:13` — `getNpcObservers` (upstream only; no change here).

## Files touched

**Modified (3):**
- `modules/world/npc_hunt.go` — add `consumeHuntTarget` + `script` import.
- `modules/world/npc_ai.go` — add single call site in `Npc.turn()`.
- `pkg/objtype/npctype.go` — add `NPCModeQueue1..Queue20` constants.

**Created (1):**
- `modules/world/npc_hunt_test.go` — 11 tests (unit + one integration).

## Post-commit verification

Per memory "Enumerate ALL call sites when propagating through a shared
file":

- Grep `processNpcHunt|consumeHuntTarget` across `modules/world/` —
  confirm `consumeHuntTarget` has exactly one call site (inside
  `Npc.turn()`) and sits between `processNpcHunt` and `processNpcRegen`.
- Grep `NPCModeQueue` across the repo — confirm constants used only in
  `modules/world/npc_hunt.go` (production) plus `modules/world/npc_hunt_test.go`
  (tests).
- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` run at
  HEAD (not package-scoped) per memory "Verify implementer claims with
  fresh independent runs" — package-local green can mask cross-package
  breakage.

## Rough LOC estimate

- Production: ~30 LOC (method body + imports + call site + constants).
- Tests: ~200 LOC (11 tests × ~18 LOC average).
- Docs: ~15 LOC of doc comments.

Within the roadmap's "~40 LOC prod+test" framing when accounting for
test volume (roadmap estimates are prod-oriented).
