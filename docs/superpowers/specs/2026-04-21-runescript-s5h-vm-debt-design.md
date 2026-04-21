# Sub-spec RuneScript S5h: VM Debt Bundle — Design

**Status:** Draft → ready for plan
**Scope:** 7 handlers. JUMP / JUMP_WITH_PARAMS (tail-call), WEAKQUEUE / STRONGQUEUE / LONGQUEUE (typed queue variants — new `PlayerQueueType` field on queue entries, STRONG fires even when delayed), P_STOPACTION / P_CLEARPENDINGACTION (player-action clearing). Extends `ActivePlayer` with: `StopAction()`, `ClearPendingAction()`, replaces `EnqueueScript(...)` with `EnqueueScriptTyped(..., type PlayerQueueType)`.
**Out of scope:** VARARG queue variants (QUEUEVARARG / WEAKQUEUEVARARG / STRONGQUEUEVARARG / LONGQUEUEVARARG — separate sub-spec once we wire ARG_VARARG packing). ENGINE queue (needs engine-script concept). SOFTTIMER / TIMER / AI_TIMER (timer sub-spec). `canAccess()` protect-flag gating (protect semantics still deferred). LONG-queue "logout-action arg" behavior (strip arg[0] on fire — edge case, document TODO).

---

## Goal

After S5h:

- Scripts can tail-call another script via `jump target` / `jump_with_params target args`, discarding the frame stack and resuming at PC=-1 in the target.
- Scripts can enqueue WEAK / STRONG / LONG variants in addition to the existing NORMAL queue. Each queue entry carries its `Type` field. STRONG fires even when `p.delayed == true`; NORMAL/WEAK/LONG fire only when idle.
- Scripts can call `p_stopaction` to abort the current interaction + pending action, and `p_clearpendingaction` for a partial clear that preserves movement but drops the pending action and modal.
- Demo: a script `jump target_script` hands control off without leaving a resumable frame; a follow-up `return` in the target finishes the whole chain.

## Architecture

```
pkg/script/
├── queue.go                       (new) PlayerQueueType enum + String()
├── active.go                      + StopAction, ClearPendingAction, replace EnqueueScript with EnqueueScriptTyped
├── state.go                       + JumpCall(target, intArgs, stringArgs) helper on ScriptState
├── handlers.go                    + 7 map entries
├── handlers_core.go (new OR extend handlers.go) handleJump, handleJumpWithParams
├── handlers_player.go             + handlePStopAction, handlePClearPendingAction
├── handlers_vars.go OR new        + handleWeakQueue, handleStrongQueue, handleLongQueue
└── tests for each

modules/world/
├── player_script.go               playerQueueRequest gains Type field; EnqueueScript → EnqueueScriptTyped; + StopAction + ClearPendingAction impls
├── player.go                      (no new fields needed — reuses interaction state from S5a/S5c)
├── tick.go                        processPlayerQueue checks Type: STRONG fires when delayed; others only when !delayed
└── script_test.go                 + E2E JumpChain + StrongQueueFiresWhileDelayed tests
```

## Components

### 1. `PlayerQueueType` enum — `pkg/script/queue.go`

```go
// PlayerQueueType mirrors TS PlayerQueueType. Determines when a queued
// script fires relative to the player's busy state.
type PlayerQueueType uint8

const (
    QueueNormal PlayerQueueType = iota
    QueueStrong                 // fires even if player is busy (delayed)
    QueueWeak                   // fires when idle; historically a separate list
    QueueLong                   // fires when idle; carries a logout-action arg (deferred)
    QueueEngine                 // reserved — engine-script use; not implemented here
    QueueSoft                   // reserved
)
```

### 2. `ActivePlayer` interface updates

**Replace** the existing `EnqueueScript` with a typed version:

```go
// EnqueueScriptTyped appends a queued fresh-run request with the given
// queue type. Delay=0 fires same tick (per-type gating on whether "same
// tick" honors the player's busy state — see docs).
// S5h replaces the untyped EnqueueScript.
EnqueueScriptTyped(scriptID uint32, delay int, intArg int, qtype PlayerQueueType)
```

**Add:**

```go
// StopAction clears the current interaction target + pending action
// + map-flag. Called by P_STOPACTION. Matches TS Player.stopAction().
StopAction()

// ClearPendingAction clears the current interaction + pending action
// + closes any open modal. Walk queue is preserved.
ClearPendingAction()
```

**Existing callers of `EnqueueScript`** (the QUEUE handler in `handlers_vars.go`) update to call `EnqueueScriptTyped(..., QueueNormal)`.

### 3. `ScriptState.JumpCall` helper — `pkg/script/state.go`

Tail-call: discard the frame stack, re-init locals with the given args, set PC = -1 (so the runner's `PC++` lands at 0).

```go
// JumpCall performs a tail-call to target, discarding all saved frames.
// Distinct from GosubCall which saves the caller frame for later return.
// TS reference: ScriptState.gotoFrame → setupNewScript.
func (s *ScriptState) JumpCall(target *ScriptFile, intArgs []int, stringArgs []string) {
    // Discard frame stack (no caller to return to).
    s.FrameSP = 0

    // Allocate new locals for the target.
    intLocals := make([]int, max(int(target.IntLocalCount), len(intArgs)))
    for i, v := range intArgs {
        intLocals[i] = v
    }
    stringLocals := make([]string, max(int(target.StringLocalCount), len(stringArgs)))
    for i, v := range stringArgs {
        stringLocals[i] = v
    }

    // Switch context; PC = -1 so dispatch's PC++ lands at 0.
    s.Script = target
    s.PC = -1
    s.IntLocals = intLocals
    s.StringLocals = stringLocals
}
```

### 4. Handlers

**JUMP** (`handlers.go` or `handlers_core.go`):
```go
// handleJump pops the target script id from the int stack and tail-
// calls it (no saved frame). No args are popped — target script must
// accept zero int/string args.
func handleJump(s *ScriptState) error {
    if s.Provider == nil {
        return errors.New("JUMP: no provider")
    }
    scriptID := uint32(s.PopInt())
    target := s.Provider.GetByLookupKey(scriptID)
    if target == nil {
        return fmt.Errorf("JUMP: unknown script id %d", scriptID)
    }
    s.JumpCall(target, nil, nil)
    return nil
}

// handleJumpWithParams reads target script id from the instruction's
// int operand and pops int/string args per the target's ParamTypes.
// Mirrors handleGosubWithParams.
func handleJumpWithParams(s *ScriptState) error {
    if s.Provider == nil {
        return errors.New("JUMP_WITH_PARAMS: no provider")
    }
    scriptID := uint32(s.Script.IntOperands[s.PC])
    target := s.Provider.GetByLookupKey(scriptID)
    if target == nil {
        return fmt.Errorf("JUMP_WITH_PARAMS: unknown script id %d", scriptID)
    }
    // Pop args in reverse order per ParamTypes (same as GOSUB_WITH_PARAMS).
    intArgs, stringArgs := popArgsForTarget(s, target)
    s.JumpCall(target, intArgs, stringArgs)
    return nil
}
```

Where `popArgsForTarget` is a shared helper already used by `handleGosubWithParams`. If not already factored out, factor it during this sub-spec.

**Queue variants** — one-liners that call EnqueueScriptTyped with the right type:
```go
func handleWeakQueue(s *ScriptState) error {
    return enqueueTyped(s, QueueWeak, "WEAKQUEUE")
}
func handleStrongQueue(s *ScriptState) error {
    return enqueueTyped(s, QueueStrong, "STRONGQUEUE")
}
func handleLongQueue(s *ScriptState) error {
    return enqueueTyped(s, QueueLong, "LONGQUEUE")
}

// enqueueTyped is the shared body (pop scriptID + delay + arg, call
// Self.EnqueueScriptTyped). Factor handleQueue to use this too.
func enqueueTyped(s *ScriptState, qtype PlayerQueueType, op string) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return fmt.Errorf("%s: no active player", op)
    }
    arg := int(s.PopInt())
    delay := int(s.PopInt())
    scriptID := uint32(s.PopInt())
    s.Self.EnqueueScriptTyped(scriptID, delay, arg, qtype)
    return nil
}
```

**Action-clear** — one-liners:
```go
func handlePStopAction(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("P_STOPACTION: no active player")
    }
    s.Self.StopAction()
    return nil
}
func handlePClearPendingAction(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("P_CLEARPENDINGACTION: no active player")
    }
    s.Self.ClearPendingAction()
    return nil
}
```

### 5. `modules/world` updates

**`playerQueueRequest.Type`** — add the type field:
```go
type playerQueueRequest struct {
    ScriptID uint32
    Delay    int
    IntArg   int
    Type     script.PlayerQueueType
}
```

**`EnqueueScriptTyped` impl** on `*Player`:
```go
func (p *Player) EnqueueScriptTyped(scriptID uint32, delay, intArg int, qtype script.PlayerQueueType) {
    p.queue = append(p.queue, playerQueueRequest{
        ScriptID: scriptID, Delay: delay, IntArg: intArg, Type: qtype,
    })
}
```

**Old `EnqueueScript` removed** from the interface and from Player.

**`StopAction` + `ClearPendingAction` impls:**
```go
// StopAction implements script.ActivePlayer.StopAction.
func (p *Player) StopAction() {
    p.ClearInteraction()
    p.ClearPendingAction()
}

// ClearPendingAction implements script.ActivePlayer.ClearPendingAction.
// Preserves walk queue.
func (p *Player) ClearPendingAction() {
    p.interactionKind = InteractionKindNone
    p.target = nil
    p.targetOp = 0
    p.CloseModal()
}
```

Verify the existing `ClearInteraction` method exists — it may already be in `modules/world/interaction.go` from S6a-era work. If not, add it inline in `ClearPendingAction`.

**`processPlayerQueue` update** in `modules/world/tick.go`:

Replace the `p.delayed` check with a per-entry gate:

```go
req := &p.queue[i]
req.Delay--
fires := req.Delay <= 0
if fires {
    // STRONG queue fires even if delayed; NORMAL/WEAK/LONG wait for idle.
    if p.delayed && req.Type != script.QueueStrong {
        fires = false
    }
}
if !fires {
    i++
    continue
}
// ... existing remove + runScript ...
```

### 6. Tests

**Script unit tests** (`pkg/script/handlers_debt_test.go` or extend existing):
- `TestJumpClearsFrameStack`: GOSUB into A, then A calls JUMP to B, B returns — expect Finished (no frame stack to pop back into A).
- `TestJumpWithParams`: JUMP_WITH_PARAMS into target with 2 int args; target's IntLocals[0] and [1] match.
- `TestQueueVariants`: call each of WEAK/STRONG/LONG; verify mockPlayer captures Type correctly.
- `TestStopAction`: run script `[p_stopaction, return]`, assert `mp.stopActionCalls == 1`.
- `TestClearPendingAction`: similar.

**E2E tests** (`modules/world/script_test.go`):
- `TestStrongQueueFiresWhileDelayed`: set `p.delayed = true, p.delayedUntil = s.currentTick + 99`. Enqueue a STRONG script. `processActiveScripts` → assert it fires despite delay. Similar with NORMAL — assert it does NOT fire.
- `TestJumpChain`: pure pkg/script test — scripts A→B→C via JUMP, final return finishes cleanly.

### 7. LOC estimate

| File | LOC |
|---|---|
| `pkg/script/queue.go` | 30 |
| `pkg/script/active.go` (diff) | +8 (rename + 2 new methods) |
| `pkg/script/state.go` (diff) | +25 (JumpCall) |
| `pkg/script/handlers_core.go` or diff | +50 (JUMP + JUMP_WITH_PARAMS + popArgsForTarget factor-out) |
| `pkg/script/handlers_vars.go` (diff) | +30 (queue variants + enqueueTyped) |
| `pkg/script/handlers_player.go` (diff) | +20 (stopaction + clear) |
| `pkg/script/handlers.go` (diff) | +9 (register 7) |
| `pkg/script/runner_test.go` (diff) | +40 (mockPlayer Type capture) |
| `pkg/script/*_test.go` (new cases) | +180 |
| `modules/world/player_script.go` (diff) | +30 (rename + 2 methods) |
| `modules/world/tick.go` (diff) | +8 (STRONG gating) |
| `modules/world/script_test.go` (diff) | +90 |
| **Total** | **~520** |

## Key design calls

- **Rename `EnqueueScript` → `EnqueueScriptTyped`.** Forces all callers (the single QUEUE handler in handlers_vars.go) to pass a type. Clean, no backwards-compat alias.
- **`JumpCall` discards frames entirely.** Unlike GOSUB, there's no "return to caller" — the caller was tail-called away from. Implementer notes: set FrameSP = 0, allocate fresh locals, PC = -1.
- **STRONG fires while delayed; others don't.** One-line gate in `processPlayerQueue`. This is the main semantic difference between queue types we implement.
- **WEAK queue is NOT on a separate list.** TS keeps a separate `weakQueue` list for iteration-order isolation; we merge onto the same list with a Type tag. Observable difference: if a script-chain modifies both lists mid-pass, TS and Go might iterate differently. We accept this as a pragmatic simplification; revisit if telemetry reveals a problem.
- **LONG queue's "strip args[0]" quirk deferred.** TS treats LONG's first arg as a logout-action indicator; we don't strip. Document as TODO — affects only very specific cache scripts.
- **`StopAction` as `ClearInteraction + ClearPendingAction`.** Two existing Go player methods (from S5c/S5f work) do the right things; `StopAction` just chains them.

## Gotchas

- **`popArgsForTarget` helper**: if not already factored out of `handleGosubWithParams`, JUMP_WITH_PARAMS has to duplicate the logic. Prefer factoring.
- **`max` builtin**: Go 1.21+ has `max(a, b int) int` as a builtin. Already used in existing `GosubCall`. JumpCall can use it directly.
- **Interface rename**: renaming `EnqueueScript` to `EnqueueScriptTyped` breaks `mockPlayer` — update its impl + field names. The compile-time assertion `var _ script.ActivePlayer = (*Player)(nil)` catches any missed site.
- **Existing `ClearInteraction` / `ClearPendingAction` methods** — check whether `modules/world/interaction.go` or `player_script.go` already has these. If yes, reuse. If no, inline the field mutations.
- **Test fixture `playerQueueRequest.Type` default** — `QueueNormal` is the zero value (iota = 0). Existing queue tests should still work after the field is added.
- **Heredoc `!=` bug**: use Edit/Write for test files.
