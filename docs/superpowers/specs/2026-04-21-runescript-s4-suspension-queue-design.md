# Sub-spec RuneScript S4: Suspension + Queue — Design

**Status:** Draft → ready for plan
**Scope:** Add cooperative script suspension via `p_delay`, resumption across ticks, and a single per-player script queue with a `queue` opcode. The LOGIN script (and any cache script using these primitives) now runs to completion across multiple ticks.
**Out of scope:** WEAK/STRONG/LONG queue variants, ENGINE queue, WORLD_DELAY, timers (SOFTTIMER/TIMER/AI_TIMER), NPC_SUSPENDED, COUNTDIALOG/PAUSEBUTTON, protected-access gating, VARARG queue opcodes. Each of these has its own later sub-spec.

---

## Goal

After S4:

- `p_delay N` pops `N`, marks the player delayed for `N+1` ticks, and suspends the script; the tick loop resumes the same `ScriptState` at the instruction after `p_delay` once the delay expires.
- `queue <scriptID> <delay> <arg>` enqueues a fresh-run request on the player's queue; when `delay` expires, the queued script runs as a brand-new `ScriptState`.
- A single per-player slot holds the one currently-suspended `ScriptState` (no stack of suspensions — matches TS).
- A single per-player queue holds pending fresh-run requests (just `normal` for MVP; other variants deferred).
- The tick loop gains a `processActiveScripts` phase that expires delays, resumes suspended scripts, and processes ready queue entries — inserted before `processPathing` so that movement happens after any script that set it up this tick.
- TS "speedup quirk" (a queue entry enqueued mid-iteration can fire the same tick) is preserved.

## Non-Goals

- No protected-access model. The `protect` argument exists on `runScript` already but S4 doesn't add any gating on top of suspension. (Later sub-specs add `p_arrivedelay`, OPHELD/OPLOC protect, and the associated gating.)
- No WEAK/STRONG/LONG/ENGINE queue separation. One list only, fired with same semantics.
- No queue VARARG variants (`queue_vararg`, etc.). Only the single-int-arg `queue` opcode.
- No NPC queues, loc queues, obj queues.
- No world queue (global fresh-run queue).
- No timers. (Timers are essentially recurring queues but scoped by player/npc/world; they share very little code with single-shot queues so are split cleanly.)

## Architecture

Three surgical edits to `pkg/script/` + one new file in `modules/world/` + one new tick phase.

The core design constraint is **dependency direction**: `pkg/script/` cannot import `modules/world/`. Per-player state (delay counters, active-script slot, queue list) lives on `Player`. The VM talks to `Player` via the existing `script.ActivePlayer` interface, which grows four methods.

```
pkg/script/
├── active.go                + SetDelayed, EnqueueScript, StoreActiveScript, ClearActiveScript on ActivePlayer
├── opcode.go                + OpPDelay, OpQueue constants (exact values verified from TS ScriptOpcode.ts)
├── handlers.go              + handlePDelay, handleQueue registered in handlers map
└── provider.go              + GetByLookupKey(uint32) *ScriptFile accessor (exposes byKey for queue dispatch)

modules/world/
├── player.go                + activeScript *script.ScriptState, queue []playerQueueRequest fields
├── player_script.go  (new)  Player impl of the 4 new ActivePlayer methods + playerQueueRequest type
├── script.go                runScript → runScriptAs + resumeOrFinish (shared post-Execute handler)
└── tick.go                  + processActiveScripts phase, inserted between processClientsIn and processPathing
```

Tests:

```
pkg/script/handlers_test.go       +TestPDelaySuspends, +TestQueueOpcode
modules/world/script_test.go      +TestPDelayStoresActiveScript, +TestResumeAfterDelayExpires,
                                  +TestResumedScriptEmitsMessageGame, +TestQueueFiresAtDelayExpiry,
                                  +TestQueueZeroDelayFiresSameTick
```

## Components

### 1. `pkg/script/active.go` — interface extension

```go
type ActivePlayer interface {
    MessageGame(msg string)
    Username() string

    // RuneScript S4 additions.

    // SetDelayed marks the active player as suspended for `ticks` more
    // ticks starting next tick. Implementation must compute
    // resume_tick = currentTick + 1 + ticks.
    SetDelayed(ticks int)

    // EnqueueScript appends a queued fresh-run request.
    // delay=0 fires same tick (authentic TS behavior).
    // For S4 only the single-int-arg variant is supported.
    EnqueueScript(scriptID uint32, delay int, intArg int)

    // StoreActiveScript saves a Suspended ScriptState so the tick loop
    // can resume it when the player's delay expires.
    StoreActiveScript(state *ScriptState)

    // ClearActiveScript discards any stored ScriptState. Called after
    // Finished/Aborted runs and on logout/cleanup.
    ClearActiveScript()
}
```

The existing `MessageGame`/`Username` methods are preserved. The interface grows — `modules/world/message_game.go` still has its `var _ script.ActivePlayer = (*Player)(nil)` compile-time assertion, which will fail-fast if either side drifts.

### 2. `pkg/script/opcode.go` — no changes

Both opcodes already exist in `pkg/script/opcode.go` from S1:

- `OpPDelay Opcode = 2071`
- `OpQueue  Opcode = 2092`

No new constants needed. Both have disasm names (`P_DELAY`, `QUEUE`) already registered.

### 3. `pkg/script/handlers.go` — two new handlers

```go
// handlePDelay implements P_DELAY: pop int n, delay the active player by
// n+1 ticks, and suspend execution. TS:
//   state.delay = popInt();
//   state.execution = ScriptState.SUSPENDED;
// TS's `state.delay` is n; the +1 is applied downstream when the player
// checks (currentTick >= resumeTick). We bundle the +1 into SetDelayed
// so the Go Player stores resumeTick directly.
func handlePDelay(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("P_DELAY: no active player")
    }
    n := int(s.PopInt())
    s.Self.SetDelayed(n)
    s.Execution = Suspended
    return nil
}

// handleQueue implements QUEUE: enqueue a fresh-run request on the
// active player. TS verified (engine/script/handlers/PlayerOps.ts:148):
//   const [scriptId, delay, arg] = state.popInts(3);
// popInts fills i=n-1 down to 0 via PopInt, so the stack top is `arg`,
// then `delay`, then `scriptId`. Stack push order upstream is therefore
// scriptId, delay, arg. For MVP we support exactly one int arg.
func handleQueue(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("QUEUE: no active player")
    }
    arg := int(s.PopInt())
    delay := int(s.PopInt())
    scriptID := uint32(s.PopInt())
    s.Self.EnqueueScript(scriptID, delay, arg)
    return nil
}
```

Register in `handlers` map:

```go
OpPDelay: handlePDelay,
OpQueue:  handleQueue,
```

Both opcodes take int operands (verify via `isLargeOperand(op)` — QUEUE passes the scriptID as its operand, so it's large-operand; P_DELAY's operand is unused but is still present per TS format).

### 4. `pkg/script/provider.go` — one exported accessor

```go
// GetByLookupKey returns a script by its raw uint32 key (as used in
// byKey). Returns nil if unknown. Used by the world tick loop's queue
// dispatch (where the scriptID is the raw key written into the bytecode
// at compile time).
func (p *Provider) GetByLookupKey(key uint32) *ScriptFile {
    return p.byKey[key]
}
```

### 5. `modules/world/player.go` — two new fields

```go
// Inside the existing Player struct (near other script-related state):

// activeScript holds a Suspended ScriptState awaiting resumption.
// Nil when no script is suspended. Cleared on Finished/Aborted and
// logout.
activeScript *script.ScriptState

// queue holds pending fresh-run script requests. Processed each tick
// in processActiveScripts (after resumption, before movement).
queue []playerQueueRequest
```

### 6. `modules/world/player_script.go` (new) — Player impl of the interface extensions

```go
package world

import "github.com/zsrv/goscape/pkg/script"

// playerQueueRequest is one queued fresh-run request.
type playerQueueRequest struct {
    ScriptID uint32
    Delay    int
    IntArg   int
}

// SetDelayed marks the player as suspended for `ticks` ticks starting
// next tick. Implements the P_DELAY opcode contract: resume at
// currentTick + 1 + ticks.
func (p *Player) SetDelayed(ticks int) {
    if p.client == nil || p.client.server == nil {
        return
    }
    p.delayed = true
    p.delayedUntil = p.client.server.currentTick + 1 + ticks
}

// EnqueueScript appends a queued fresh-run request.
func (p *Player) EnqueueScript(scriptID uint32, delay int, intArg int) {
    p.queue = append(p.queue, playerQueueRequest{
        ScriptID: scriptID,
        Delay:    delay,
        IntArg:   intArg,
    })
}

// StoreActiveScript saves a Suspended state for tick-loop resumption.
func (p *Player) StoreActiveScript(state *script.ScriptState) {
    p.activeScript = state
}

// ClearActiveScript discards any stored state.
func (p *Player) ClearActiveScript() {
    p.activeScript = nil
}
```

### 7. `modules/world/script.go` — `resumeOrFinish` split

The existing `runScript` currently runs, warns on non-Finished, and drops. S4 changes this to route `Suspended` into `StoreActiveScript` and keep the state alive.

```go
// runScript starts a fresh script execution. If the script suspends,
// the state is stored on the player for later resumption via
// processActiveScripts.
func (s *Server) runScript(sf *script.ScriptFile, self script.ActivePlayer,
    protect bool, intArgs []int, stringArgs []string) {
    if sf == nil {
        return
    }
    state := script.Init(sf, self, protect, intArgs, stringArgs)
    state.Provider = s.scriptProvider
    s.resumeOrFinish(state, self)
}

// resumeOrFinish runs Execute and routes the result. Used by both fresh
// runs (from runScript) and resumptions (from processActiveScripts).
func (s *Server) resumeOrFinish(state *script.ScriptState, self script.ActivePlayer) {
    if err := script.Execute(state); err != nil {
        s.log.Warn("script execute error",
            "script", state.Script.Name, "err", err)
        self.ClearActiveScript()
        return
    }
    switch state.Execution {
    case script.Finished, script.Aborted:
        self.ClearActiveScript()
    case script.Suspended:
        self.StoreActiveScript(state)
    default:
        // Unsupported suspension types (CountDialog, PauseButton,
        // NpcSuspended, WorldSuspended) until later sub-specs land.
        s.log.Warn("script in unsupported execution state",
            "script", state.Script.Name, "execution", state.Execution)
        self.ClearActiveScript()
    }
}
```

### 8. `modules/world/tick.go` — new phase

Tick-loop order changes:

```
processClientsIn
processActiveScripts   ← NEW
processPathing
processInteractions
processNpcs
processLogouts
processLogins
processInfo
processZones
processClientsOut
processCleanup
```

Rationale: per-player resumption and queue firing must happen before movement for this tick, so a resumed script that sets up a walk (or a queued script that cancels one) is visible this tick.

```go
// processActiveScripts expires delays, resumes suspended scripts, and
// fires ready queue entries. Inserted between processClientsIn and
// processPathing.
func (s *Server) processActiveScripts() {
    s.playersMu.RLock()
    players := make([]*Player, len(s.playerLoop))
    copy(players, s.playerLoop)
    s.playersMu.RUnlock()

    for _, p := range players {
        // (1) Expire delay.
        if p.delayed && s.currentTick >= p.delayedUntil {
            p.delayed = false
        }
        // (2) Resume suspended activeScript if delay has expired.
        if !p.delayed && p.activeScript != nil &&
            p.activeScript.Execution == script.Suspended {
            state := p.activeScript
            state.Execution = script.Running
            s.resumeOrFinish(state, p)
        }
        // (3) Process queue (fresh runs).
        s.processPlayerQueue(p)
    }
}

// processPlayerQueue walks the player's queue, decrementing delays and
// firing ready entries as fresh script runs. Iterates by index so that
// an entry appended mid-pass (via a fired script calling EnqueueScript
// again) is visible in the same iteration — this preserves TS's
// authentic "speedup quirk".
func (s *Server) processPlayerQueue(p *Player) {
    i := 0
    for i < len(p.queue) {
        req := &p.queue[i]
        req.Delay--
        if req.Delay > 0 || p.delayed {
            i++
            continue
        }
        // Remove BEFORE firing so a re-entrant EnqueueScript doesn't
        // collide with our index.
        scriptID := req.ScriptID
        intArg := req.IntArg
        p.queue = append(p.queue[:i], p.queue[i+1:]...)

        if s.scriptProvider != nil {
            if sf := s.scriptProvider.GetByLookupKey(scriptID); sf != nil {
                s.runScript(sf, p, false, []int{intArg}, nil)
            }
        }
        // Don't advance i: we just removed the current element, so
        // i now points to what was the next element (or past end).
    }
}
```

## Data flow

### p_delay round trip

```
Script:  mes "A"; p_delay 2; mes "B"
Tick N:
  processLogins or queue dispatch calls s.runScript(sf, p, ...)
  → state = Init(sf, ...)
  → resumeOrFinish(state, p)
    → Execute(state)
      → handleMes:    p.MessageGame("A")
      → handlePDelay: n=2, p.SetDelayed(2), state.Execution=Suspended
                      Player.delayed=true, Player.delayedUntil=N+3
                      dispatch loop exits (Execution != Running)
    → state.Execution == Suspended
    → p.StoreActiveScript(state)  → Player.activeScript = state

Tick N+1: processActiveScripts
  Player.delayed=true, currentTick=N+1, delayedUntil=N+3: still delayed

Tick N+2: same, still delayed

Tick N+3: processActiveScripts
  Player.delayed && N+3 >= N+3 → delayed=false
  Player.activeScript != nil && Execution==Suspended
    state.Execution = Running
    → resumeOrFinish(state, p)
      → Execute(state) resumes at PC after p_delay
        → handleMes:  p.MessageGame("B")
        → handleReturn: state.Execution = Finished
      → state.Execution == Finished
      → p.ClearActiveScript() → Player.activeScript = nil
```

### queue round trip

```
Script A: queue <scriptB> 0 42
Tick N: Script A runs
  → handleQueue: p.EnqueueScript(scriptB_id, 0, 42)
    Player.queue = [{scriptB, 0, 42}]

Tick N: same tick, later in processActiveScripts (scripts fired INSIDE
  processActiveScripts during resumption get their queue adds fired this
  tick; scripts fired OUTSIDE processActiveScripts — e.g. LOGIN in
  processLogins — get their queue adds fired next tick)

Tick N+1: processActiveScripts
  walk queue: req.Delay = 0, pre-decrement → -1
  -1 <= 0 && !delayed → fire
  remove entry, runScript(scriptB, p, false, [42], nil)
```

### Queue speedup quirk (TS authentic)

```
Script B is queued with delay=1 at tick N.
Script C is queued with delay=0 at tick N.
Tick N+1: processActiveScripts
  walk queue by index:
    i=0: Script B. Delay 1 → 0. 0 <= 0 → fire.
         Script B's handler calls queue(scriptD, 0, ...).
         Player.queue is now [scriptC (delay=0), scriptD (delay=0)]
         (with scriptC at i=0 after our removal).
    Don't advance i (we removed B).
    i=0: Script C. Delay 0 → -1. Fire.
    i=0: Script D. Delay 0 → -1. Fire.
  All three fire same tick.
```

This is the "authentic bug" TS replicates. Real cache scripts may rely on this timing.

## Error handling

- **`scriptProvider == nil`**: tick phase still runs delay-expiry + resumption (but resumption can still work since `activeScript` is the full state, not a lookup). Queue dispatch silently drops since there's no provider to look up scripts.
- **Queue scriptID not found**: silent skip. (TS logs a warning; we can log at `slog.Debug` if needed but do NOT spam at Info — a runaway bug could queue a missing script 50x/tick.)
- **Resumed script Execute error**: logged at Warn; `ClearActiveScript` called. Player is not left stuck.
- **Queue entry with negative initial delay**: pre-decrement still fires immediately. That matches TS.
- **Re-entrant `EnqueueScript` during a fire**: safe — we remove before firing, iterate by index, and don't advance after removal.
- **Player logout with activeScript or non-empty queue**: `Server.removePlayer` already clears the player. Since there's no dedicated cleanup yet, no explicit cleanup is required — the GC reclaims the Player including its activeScript and queue. (If logout cleanup gains a script-cancel hook later, it can call `ClearActiveScript` for symmetry.)

## Testing

### `pkg/script/handlers_test.go`

- `TestPDelaySuspends`: build `[PUSH 5, P_DELAY, RETURN]`, run with a mock `ActivePlayer` that records `SetDelayed` calls. Assert `SetDelayed(5)` called exactly once, `state.Execution == Suspended` after Execute.
- `TestPDelayRequiresActivePlayer`: build the same script, run without `PtrActivePlayer` set (`s.Self = nil`). Assert Execute returns an error.
- `TestQueueOpcode`: build `[PUSH delay, PUSH arg, QUEUE scriptID, RETURN]`, run with a mock that records `EnqueueScript`. Assert correct scriptID/delay/arg.

### `modules/world/script_test.go`

Existing infrastructure (`newTestServer`, `newTestPlayer`, `drainConn`, `buildLoginScript`) gives us a working player + scripted wire capture.

- `TestPDelayStoresActiveScript`: build `[PUSH 2, P_DELAY, RETURN]`, runScript. Assert `p.activeScript != nil`, `p.delayed == true`, `p.delayedUntil == s.currentTick + 3`.
- `TestResumeAfterDelayExpires`: same setup. Advance `s.currentTick += 3`. Call `s.processActiveScripts()`. Assert `p.activeScript == nil`, `p.delayed == false`.
- `TestResumedScriptEmitsMessageGame`: build `[PUSH_STR "before", MES, PUSH 1, P_DELAY, PUSH_STR "after", MES, RETURN]`. Run. Drain conn: expect "before\n" packet. Advance tick by 2; call `processActiveScripts`; flushWrite; drain: expect "after\n" packet.
- `TestQueueFiresAtDelayExpiry`: seed provider with a "greet" script that emits `mes "g"`. Directly call `p.EnqueueScript(greet_key, 1, 0)` (skipping the opcode test). Advance tick; call `processActiveScripts`; drain: expect "g\n".
- `TestQueueZeroDelayFiresSameTick`: enqueue with `delay=0`. One pass of `processActiveScripts`. Assert fired.
- `TestQueueMultipleEntriesPreservesOrder`: enqueue greet1 (delay=0) and greet2 (delay=0). One pass. Assert both wire packets arrive in order.

## LOC estimate

| File | LOC |
|---|---|
| `pkg/script/active.go` | +24 |
| `pkg/script/handlers.go` | +35 |
| `pkg/script/provider.go` | +5 |
| `pkg/script/handlers_test.go` | +75 |
| `modules/world/player.go` | +4 |
| `modules/world/player_script.go` (new) | ~50 |
| `modules/world/script.go` | +25 |
| `modules/world/tick.go` | +50 |
| `modules/world/script_test.go` | +180 |
| **Total** | **~450** |

One sub-spec, implementable in one pass.

## Key design calls

- **`resumeOrFinish` is the single post-Execute handler**. Fresh and resumed runs share it; the Store/Clear decisions live in one place.
- **`StoreActiveScript` / `ClearActiveScript` on `ActivePlayer`** avoids runtime type assertions (`self.(*Player)`) inside `runScript`. Keeps dependency direction clean.
- **`processActiveScripts` before `processPathing`**: a resumed script that calls `p_walk` produces movement this tick.
- **Index-based queue iteration** preserves TS's "speedup quirk" — queue entries added mid-iteration become visible without needing a rescan.
- **Remove-before-fire**: prevents index drift when a fired script re-enters the queue.
- **Single queue for MVP**: weak/strong/long/engine get their own sub-specs. TS's queue tag is a 1-word change once those land.
- **Interface grows; compile-time assertion catches drift**: `var _ script.ActivePlayer = (*Player)(nil)` in `message_game.go` will fail the build if the `Player` methods go missing.

## Gotchas (from survey + TS cross-read)

- **`+1` in `SetDelayed`**: `delayedUntil = currentTick + 1 + ticks`. Off-by-one produces one-tick-early resumes.
- **`Execution = Running` before resume**: the dispatch loop exits immediately on anything other than `Running`. A resumed state MUST have its execution flipped back to `Running` before `Execute` is called again.
- **Opcode numeric values**: `OpPDelay = 2071`, `OpQueue = 2092` — both verified in our `pkg/script/opcode.go`.
- **QUEUE pop order**: verified against TS `PlayerOps.ts:148` — `popInts(3)` fills top-down, so the Go handler pops `arg`, then `delay`, then `scriptID`.
- **`p.client.server` nil in tests**: `SetDelayed` reaches through `p.client.server` for the current tick. Tests must use `newTestPlayer` which wires these up, or the test must set them manually.
- **`isLargeOperand` for OpPDelay / OpQueue**: both opcodes (2071, 2092) are > 100, so `isLargeOperand` returns true for both — the decoder reads a 4-byte int operand. Neither handler reads from `IntOperands` (P_DELAY ignores its operand; QUEUE pops its scriptID from the stack).

## Demo

After S4 lands:

1. A synthetic test `mes "A"; p_delay 1; mes "B"` produces exactly one wire packet this tick, then one more packet on the second tick after.
2. A synthetic LOGIN script that uses `p_delay` runs to completion across multiple ticks — the welcome message appears on login, a deferred message appears 1 tick later.
3. A synthetic `queue greet 0 42` inside a LOGIN script fires `greet` on the next tick (not same tick, because LOGIN runs outside `processActiveScripts`).
4. All tick-loop invariants still hold: movement, zone tracking, and PlayerInfo remain correct.
