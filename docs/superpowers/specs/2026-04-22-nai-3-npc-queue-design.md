# NAI-3 — NPC Queue + `ai_queue{1..20}` Dispatch

Add the NPC-side script queue: `NpcQueueRequest` type, `queue []NpcQueueRequest`
field on `*Npc`, a `processNpcQueue` pass inside `Npc.turn()`, and
`OpNpcQueue` opcode wiring so scripts can enqueue `ai_queueN` trigger
dispatches on an NPC.

Part of the NPC AI tick decomposition roadmap
(`docs/superpowers/specs/2026-04-22-npc-ai-tick-decomposition-design.md`).
Depends on NAI-2 (NPC script infrastructure).

## Goal

After NAI-3 ships, RuneScript can call `npc_queue <queueId, delay, arg>`
to schedule one of 20 trigger-based scripts on the active NPC. The
queued request counts down each tick only while the NPC is not
`delayed`, then fires by looking up the corresponding `ai_queueN`
trigger script via the NPC's type + category.

## Scope

Implements the NAI-3 row of the NAI roadmap with one additional
concern folded in per `nai_followups.md`:

- **Folded-in from NAI-2 follow-up:** tests for `resumeOrFinishNpc`'s
  error-return path and default-branch path. NAI-3 is the first
  sub-spec that can plausibly drive NPC scripts into these branches
  (via enqueue dispatch that might fail or return unexpected states).

## Non-goals

1. **`NumberNotNull` check on delay.** TS wraps the popped delay in
   `check(state.popInt(), NumberNotNull)`. Tracked as a
   future-audit item in `nai_followups.md`. Scripts don't pass
   negative delays in authentic content.
2. **STRONG/WEAK/LONG queue types.** TS's NPC queue has no
   type distinction — unlike the player queue. Preserving parity.
3. **Variadic `args []ScriptArgument` slice.** TS `NpcQueueRequest`
   has it but authentic content only uses the single `lastInt`
   argument. YAGNI.
4. **Per-NPC queue size cap.** No cap in TS; no cap in Go.
5. **`EnqueueScriptForTrigger` with concrete `*ScriptFile` variant.**
   Player side has `EnqueueScriptFile` for engine-dispatch
   convenience (e.g. changeStat). NPC side currently has no
   engine-dispatch callers; add the file-variant entry point when
   its first consumer arrives (Dead-API YAGNI).

## TS reference

- `Engine-TS/src/engine/entity/Npc.ts:180` (turn: processQueue call)
- `Engine-TS/src/engine/entity/Npc.ts:241-245` (enqueueScript)
- `Engine-TS/src/engine/entity/Npc.ts:538-560` (processQueue)
- `Engine-TS/src/engine/entity/NpcQueueRequest.ts`
- `Engine-TS/src/engine/script/handlers/NpcOps.ts:144-150` (NPC_QUEUE)

## Architecture

### File layout

**Modified:**
- `pkg/script/queue.go` — add `NpcQueueRequest` type
- `pkg/script/active.go` — extend `ActiveNpc` with
  `EnqueueScriptForTrigger`
- `pkg/script/handlers_npc.go` — add `handleNpcQueue`
- `pkg/script/handlers.go` — register `OpNpcQueue: handleNpcQueue`
- `pkg/script/handlers_npc_test.go` — 3 tests (basic enqueue,
  defensive nil, invalid queueID) + mock updates
- `pkg/script/handlers_player_test.go` — mock update (add
  `EnqueueScriptForTrigger` no-op to `mockActiveNpc`)
- `modules/world/npc.go` — add `queue []NpcQueueRequest` field +
  `EnqueueScriptForTrigger` method
- `modules/world/npc_script.go` — add `processNpcQueue` method on
  `*Server`
- `modules/world/npc_ai.go` — add `processNpcQueue` call inside
  existing `!n.dead` prefix block
- `modules/world/npc_script_test.go` — 5 tests (direct enqueue,
  dispatch, delayed-gate, re-entrant append, NAI-2 error/default
  follow-ups)

### Types

`pkg/script/queue.go` (append to existing file):

```go
// NpcQueueRequest is an NPC-side enqueue entry. Unlike PlayerQueueRequest,
// it has no queue-type (TS NPC queue has no strong/weak/long
// distinction). The Trigger is one of TriggerAiQueue1..TriggerAiQueue20.
type NpcQueueRequest struct {
    Trigger ServerTriggerType
    Delay   int
    IntArg  int
}
```

### `ActiveNpc` interface extension (`pkg/script/active.go`)

Appended at the end of the `ActiveNpc` interface, after the NAI-2
script-lifecycle methods:

```go
// EnqueueScriptForTrigger appends a queued ai_queueN dispatch to the
// NPC. Matches TS Npc.enqueueScript at Npc.ts:241-245 — the trigger
// (TriggerAiQueue1..TriggerAiQueue20) identifies which script runs;
// lookup happens at fire time via scriptProvider.GetByTrigger keyed
// on the NPC's type + category.
EnqueueScriptForTrigger(trigger ServerTriggerType, delay int, intArg int)
```

### `*Npc` additions (`modules/world/npc.go`)

Field added to the `// === script state ===` block:

```go
queue []NpcQueueRequest
```

Method appended (after `SetDelayed`):

```go
// EnqueueScriptForTrigger appends a queued ai_queueN dispatch.
// Implements script.ActiveNpc. Script resolution deferred to fire
// time via scriptProvider.GetByTrigger — matches TS Npc.enqueueScript.
func (n *Npc) EnqueueScriptForTrigger(trigger script.ServerTriggerType, delay int, intArg int) {
    n.queue = append(n.queue, script.NpcQueueRequest{
        Trigger: trigger,
        Delay:   delay,
        IntArg:  intArg,
    })
}
```

### `processNpcQueue` helper (`modules/world/npc_script.go`)

Appended at bottom:

```go
// processNpcQueue walks the NPC's queue, decrementing delays and
// firing ready entries as fresh NPC-anchored script runs. Iterates
// by index so a request appended mid-pass (via a fired script calling
// EnqueueScriptForTrigger again) is visible in the same iteration —
// preserves TS's "speedup quirk" at Npc.ts:538-560.
//
// Removal happens BEFORE firing so a re-entrant enqueue doesn't
// collide with the index pointer. Matches the player-side pattern at
// modules/world/tick.go:219-242.
//
// Delay only decrements when the NPC is not delayed — TS Npc.ts:544-547
// "purposely only decrements the delay when the npc is not delayed".
func (s *Server) processNpcQueue(n *Npc) {
    if n.typ == nil {
        return
    }
    i := 0
    for i < len(n.queue) {
        req := &n.queue[i]
        if !n.delayed {
            req.Delay--
        }
        if n.delayed || req.Delay > 0 {
            i++
            continue
        }
        trigger := req.Trigger
        intArg := req.IntArg
        n.queue = append(n.queue[:i], n.queue[i+1:]...)
        if s.scriptProvider == nil {
            continue
        }
        sf := s.scriptProvider.GetByTrigger(trigger, n.typeId, n.typ.Category)
        s.runNpcScript(sf, n, []int{intArg}, nil)
        // Don't advance i — we removed the current element.
    }
}
```

### `Npc.turn()` prefix extension (`modules/world/npc_ai.go`)

Inside the existing `if !n.dead { ... }` block added in NAI-2, append
the queue pass after the script-resume pass:

```go
if !n.dead {
    // Delayed expiration. Matches TS Npc.ts:113.
    if n.delayed && s.currentTick >= n.delayedUntil {
        n.delayed = false
    }
    // Resume suspended script. Matches TS Npc.ts:116-118.
    if !n.delayed && n.activeScript != nil &&
        n.activeScript.Execution == script.NpcSuspended {
        state := n.activeScript
        state.Execution = script.Running
        s.resumeOrFinishNpc(state, n)
    }
    // Queue pass. Matches TS Npc.ts:180 (turn calls processQueue).
    s.processNpcQueue(n)
}
```

Ordering rationale: queue decrements happen AFTER potential resume
in the same tick, because a resumed script may enqueue new requests
that should NOT have their first tick immediately consumed. This
matches TS tick ordering where `processQueue` runs after the resume
step inside `turn()`.

### `handleNpcQueue` (`pkg/script/handlers_npc.go`)

```go
// handleNpcQueue (NPC_QUEUE, opcode 2530) enqueues an ai_queueN
// dispatch on the active NPC. Pop order: delay, arg, queueId (queueId
// is 1-20, maps to TriggerAiQueue{1..20}). Mirrors TS NpcOps.ts:144-150.
func handleNpcQueue(s *ScriptState) error {
    if err := requireActiveNpc(s, "NPC_QUEUE"); err != nil {
        return err
    }
    delay := s.PopInt()
    arg := s.PopInt()
    queueID := s.PopInt()
    if queueID < 1 || queueID > 20 {
        return fmt.Errorf("NPC_QUEUE: invalid queueId %d (want 1..20)", queueID)
    }
    trigger := TriggerAiQueue1 + ServerTriggerType(queueID-1)
    s.ActiveNpc.EnqueueScriptForTrigger(trigger, delay, arg)
    return nil
}
```

**Opcode registration (`pkg/script/handlers.go`):**

Add `OpNpcQueue: handleNpcQueue,` in the NPC-mutating-ops block in
alphabetical position (between `OpNpcHunt` / `OpNpcHuntAll` and
`OpNpcRange`; the exact line depends on current gofmt alignment).
Note: `OpNpcHunt`/`OpNpcHuntAll` are NOT YET wired (NAI-7 wires
them); place `OpNpcQueue` at whatever alphabetical position gofmt
produces.

## Test strategy

### World-side tests (`modules/world/npc_script_test.go`)

1. **`TestNpcEnqueueScriptForTrigger`** — unit: build *Npc, call
   `n.EnqueueScriptForTrigger(TriggerAiQueue3, 5, 42)`, assert
   `n.queue` has one entry with those fields. No server needed.

2. **`TestNpcTurnFiresQueuedScriptWhenDelayZero`** — integration:
   build *Server with `scriptProvider` seeded with a trivial ai_queueN
   script registered for the NPC type; call
   `n.EnqueueScriptForTrigger(trigger, 1, 99)`; advance one tick via
   `n.turn(s)` twice (first tick decrements delay to 0, second tick
   fires). Assert queue empty after fire. Use a sentinel script that
   writes to a shared var to detect the fire.

3. **`TestNpcTurnDoesNotDecrementQueueWhileDelayed`** — enqueue with
   delay=1; set `n.delayed = true` and `n.delayedUntil = 999`;
   call `n.turn(s)` multiple times; assert `n.queue[0].Delay` remains
   1 (did not decrement). Matches TS Npc.ts:544-547.

4. **`TestNpcTurnReentryQueueAppendDuringIteration`** — enqueue entry
   A at delay=1; register script for A's trigger that in turn enqueues
   entry B at delay=0; advance tick; assert BOTH A and B fired in the
   same `turn()` call (B appended by A's script, consumed by the same
   `processNpcQueue` iteration). Matches TS "speedup quirk" + Go
   player-side precedent at `tick.go:212-216`.

5. **`TestResumeOrFinishNpcErrorPathClearsScript`** (NAI-2 follow-up):
   construct a state that will error from `script.Execute` (e.g. via
   a script with an unknown opcode, or a malformed opcode array);
   call `resumeOrFinishNpc`; assert `n.activeScript == nil` and a
   warn-level log is emitted. Use a log buffer to verify.

6. **`TestResumeOrFinishNpcDefaultBranchClearsScript`** (NAI-2
   follow-up): synthetic test. Construct a `ScriptState` via
   `script.Init(sf, nil, false, nil, nil)`, manually set
   `state.Execution = script.CountDialog` *before* calling
   `s.resumeOrFinishNpc(state, n)`. Since `script.Execute`'s hot loop
   only runs while `Execution == Running`, the pre-set value survives
   untouched; `resumeOrFinishNpc` dispatches on it and hits the
   `default:` branch. Assert `n.activeScript == nil` after dispatch.
   This path is unreachable from authentic content (all
   non-`NpcSuspended` non-terminal `Execution` values require an
   `ActivePlayer`, and `runNpcScript` passes `nil`), but the test
   proves the defensive clear fires if future code accidentally
   drives an NPC-anchored script there.

### Script-side tests (`pkg/script/handlers_npc_test.go`)

7. **`TestHandleNpcQueueEnqueues`** — construct a mockNpc, push
   `queueID=3, arg=42, delay=5` (remember pop order: top=delay,
   middle=arg, bottom=queueID, so push in reverse), execute OpNpcQueue,
   assert mockNpc recorded one enqueue call with
   `(TriggerAiQueue3, 5, 42)`.

8. **`TestHandleNpcQueueWithoutActiveNpcErrors`** — state with nil
   ActiveNpc, execute OpNpcQueue, assert error
   `"NPC_QUEUE: no active npc"` via `requireActiveNpc`.

9. **`TestHandleNpcQueueInvalidQueueIDErrors`** — push queueID=0 and
   queueID=21 (boundaries); assert error
   `"NPC_QUEUE: invalid queueId 0 (want 1..20)"` and equivalent for 21.

### Mock updates

- `mockNpc` in `handlers_npc_test.go` gets a new method recording
  enqueue calls:
  ```go
  enqueues []enqueueCall  // slice of {trigger, delay, intArg}
  func (m *mockNpc) EnqueueScriptForTrigger(t ServerTriggerType, d, a int) {
      m.enqueues = append(m.enqueues, enqueueCall{t, d, a})
  }
  ```
- `mockActiveNpc` in `handlers_player_test.go` gets a no-op stub.

## Fidelity notes

1. **Delay decrement gating.** TS `Npc.ts:544-547` decrements ONLY
   when `!this.delayed`. Go preserves this exactly in `processNpcQueue`.
2. **Re-entrant append during iteration.** TS `Npc.ts:543` iterates
   `this.queue.all()` (a linked list — new appends become visible).
   Go mirrors with slice-index iteration + explicit removal-before-fire,
   matching the player-side quirk at `tick.go:212-216` (explicitly
   documented there).
3. **Script lookup by trigger.** TS uses
   `ScriptProvider.getByTrigger(queueId, type.id, type.category)`.
   Go calls `s.scriptProvider.GetByTrigger(trigger, n.typeId, n.typ.Category)`.
   Nil `*scriptProvider` is defensively handled (short-circuit return).
4. **Missing script silent-drop.** TS checks `if (script)` before
   firing. Go's `runNpcScript` has the equivalent nil-check at
   `modules/world/npc_script.go:89`. The removed queue entry does NOT
   get re-added on missing script — matches TS.

## Rough LOC

- `pkg/script/queue.go`: +10 lines (`NpcQueueRequest` struct + docs)
- `pkg/script/active.go`: +8 lines (interface method)
- `pkg/script/handlers_npc.go`: +18 lines (handleNpcQueue)
- `pkg/script/handlers.go`: +1 line (registration)
- `pkg/script/handlers_npc_test.go`: +90 lines (3 tests + mock update)
- `pkg/script/handlers_player_test.go`: +3 lines (mock stub)
- `modules/world/npc.go`: +10 lines (field + method)
- `modules/world/npc_script.go`: +30 lines (processNpcQueue)
- `modules/world/npc_ai.go`: +3 lines (queue-pass call)
- `modules/world/npc_script_test.go`: +180 lines (5 tests including
  two NAI-2 follow-ups)

Total ≈ 350 LOC. Slightly above the roadmap's ~100 prod+test estimate
because NAI-2 follow-ups add ~80 LOC of test coverage, and the
re-entrant-append test needs full script-fire harness setup.

## Dependencies

- **Blocks:** NAI-5 (lifecycle) uses `queue.clear()` on revertType.
  NAI-7 (hunt core) will eventually interact with queue via
  `OpNpcSetHunt` / `OpNpcSetHuntMode`, but not directly.
- **Blocked by:** NAI-2 (`runNpcScript`, `resumeOrFinishNpc`,
  `ActiveNpc` interface script-lifecycle methods, `*Npc.delayed`).

## Verifications resolved during spec-write

1. TS `NPC_QUEUE` pop order confirmed via
   `Engine-TS/.../NpcOps.ts:144-147`: `delay, arg, queueId`.
2. `TriggerAiQueue1..20` already defined in
   `pkg/script/trigger.go:117-136` — no new trigger constants needed.
3. Player-side quirk documented at `modules/world/tick.go:212-216`
   (re-entrant enqueue during iteration) confirmed as the template
   to mirror.
