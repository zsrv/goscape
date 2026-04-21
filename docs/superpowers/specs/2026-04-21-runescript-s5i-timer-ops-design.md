# Sub-spec RuneScript S5i: Timer Ops — Design

**Status:** Draft → ready for plan
**Scope:** 5 handlers (SETTIMER, SOFTTIMER, CLEARTIMER, CLEARSOFTTIMER, GETTIMER), new `PlayerTimerType` enum, per-Player `timers` storage (map keyed by scriptID), new `processPlayerTimers` tick phase. Soft timers fire while player is delayed; normal timers wait for idle (same pattern as STRONG vs NORMAL queue).
**Out of scope:** AI_TIMER (needs active_npc — S6 territory). VARARG timer variants — this MVP supports a single int arg per timer (matches our `EnqueueScriptTyped` shape). World-wide timers. Multi-timer-per-scriptID (TS overwrites; we match).

---

## Goal

After S5i:

- Scripts can schedule repeating invocations of another script via `settimer(script_id, interval, arg)` (normal — fires only when idle) or `softtimer(script_id, interval, arg)` (fires regardless of busy state).
- Scripts can cancel a timer by id via `cleartimer(script_id)` / `clearsofttimer(script_id)`.
- Scripts can query remaining ticks via `gettimer(script_id)` — returns `-1` if the timer isn't set.
- The tick loop's new `processPlayerTimers` phase fires ready timers each tick, respecting type-based gating against `p.delayed`.
- Demo: a script that sets a timer to print "tick!" every 5 ticks runs for the player's session, visible in wire-level MessageGame packets.

## Architecture

```
pkg/script/
├── timer.go               (new) PlayerTimerType enum + String()
├── active.go              + SetTimer / ClearTimer / GetTimer methods
├── handlers_player.go     + 5 timer handlers
└── handlers.go            + 5 map entries

modules/world/
├── player.go              + timers map[uint32]*playerTimer
├── player_script.go       (new file player_timer.go OR inline) + playerTimer struct + Impl methods
└── tick.go                + processPlayerTimers phase before processPathing
```

## Components

### 1. `PlayerTimerType` enum — `pkg/script/timer.go`

```go
type PlayerTimerType uint8

const (
    TimerNormal PlayerTimerType = iota
    TimerSoft
)

func (t PlayerTimerType) String() string {
    switch t {
    case TimerNormal:
        return "Normal"
    case TimerSoft:
        return "Soft"
    default:
        return "Unknown"
    }
}
```

### 2. `ActivePlayer` extensions

```go
// S5i: timer ops.

// SetTimer registers a timer that re-runs the script at `scriptID`
// every `interval` ticks with `intArg` as the single int arg. A timer
// at the same scriptID is overwritten. type is TimerNormal (waits for
// idle) or TimerSoft (fires while busy).
SetTimer(scriptID uint32, interval int, intArg int, ttype PlayerTimerType)

// ClearTimer cancels any timer registered at scriptID, regardless of
// type. Silent no-op if no such timer.
ClearTimer(scriptID uint32)

// GetTimer returns the number of ticks until the timer at scriptID
// next fires, or -1 if no such timer is registered. May return
// negative if the timer is overdue but hasn't processed yet.
GetTimer(scriptID uint32) int
```

### 3. Handlers — `pkg/script/handlers_player.go` or new `handlers_timer.go`

Shared `enqueueTimer` helper to cut duplication:

```go
func enqueueTimer(s *ScriptState, ttype PlayerTimerType, op string) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return fmt.Errorf("%s: no active player", op)
    }
    // TS popInts(3) = [scriptID, interval, arg]; stack top = arg.
    // Implementer MUST verify exact pop order against TS PlayerOps.ts
    // lines 817-864.
    arg := int(s.PopInt())
    interval := int(s.PopInt())
    scriptID := uint32(s.PopInt())
    s.Self.SetTimer(scriptID, interval, arg, ttype)
    return nil
}

func handleSetTimer(s *ScriptState) error  { return enqueueTimer(s, TimerNormal, "SETTIMER") }
func handleSoftTimer(s *ScriptState) error { return enqueueTimer(s, TimerSoft, "SOFTTIMER") }

func handleClearTimer(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("CLEARTIMER: no active player")
    }
    scriptID := uint32(s.PopInt())
    s.Self.ClearTimer(scriptID)
    return nil
}

func handleClearSoftTimer(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("CLEARSOFTTIMER: no active player")
    }
    scriptID := uint32(s.PopInt())
    s.Self.ClearTimer(scriptID)
    return nil
}

func handleGetTimer(s *ScriptState) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return errors.New("GETTIMER: no active player")
    }
    scriptID := uint32(s.PopInt())
    s.PushInt(s.Self.GetTimer(scriptID))
    return nil
}
```

Note: `handleClearTimer` and `handleClearSoftTimer` both call `ClearTimer` — both cancel the timer at that ID regardless of type, matching TS.

### 4. Player-side state + impl

```go
// In modules/world/player.go
type playerTimer struct {
    ScriptID uint32
    Type     script.PlayerTimerType
    Interval int
    Clock    int // absolute tick of last fire (or creation)
    IntArg   int
}

type Player struct {
    // ... existing fields ...

    // timers holds per-player repeating scripts keyed by script lookup key.
    // nil until first SetTimer call.
    timers map[uint32]*playerTimer
}
```

```go
// In modules/world/player_script.go (or player_timer.go)

func (p *Player) SetTimer(scriptID uint32, interval, intArg int, ttype script.PlayerTimerType) {
    if p.timers == nil {
        p.timers = make(map[uint32]*playerTimer)
    }
    now := 0
    if p.client != nil && p.client.server != nil {
        now = p.client.server.currentTick
    }
    p.timers[scriptID] = &playerTimer{
        ScriptID: scriptID,
        Type:     ttype,
        Interval: interval,
        Clock:    now,
        IntArg:   intArg,
    }
}

func (p *Player) ClearTimer(scriptID uint32) {
    if p.timers == nil {
        return
    }
    delete(p.timers, scriptID)
}

func (p *Player) GetTimer(scriptID uint32) int {
    if p.timers == nil {
        return -1
    }
    t, ok := p.timers[scriptID]
    if !ok {
        return -1
    }
    now := 0
    if p.client != nil && p.client.server != nil {
        now = p.client.server.currentTick
    }
    // TS: clock + interval - currentTick
    return (t.Clock + t.Interval) - now
}
```

### 5. Tick phase — `modules/world/tick.go`

New `processPlayerTimers` phase, inserted between `processActiveScripts` and `processPathing`:

```go
// processPlayerTimers fires any timers whose clock + interval has
// elapsed. Soft timers fire regardless of p.delayed; normal timers
// skip while delayed.
func (s *Server) processPlayerTimers() {
    s.playersMu.RLock()
    players := make([]*Player, len(s.playerLoop))
    copy(players, s.playerLoop)
    s.playersMu.RUnlock()

    for _, p := range players {
        if len(p.timers) == 0 {
            continue
        }
        // Iterate stably by sorted scriptID (deterministic fire order).
        ids := make([]uint32, 0, len(p.timers))
        for id := range p.timers {
            ids = append(ids, id)
        }
        sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

        for _, id := range ids {
            t, ok := p.timers[id]
            if !ok {
                continue // timer was cleared mid-iteration
            }
            if s.currentTick < t.Clock+t.Interval {
                continue // not yet due
            }
            // Type gate: normal timers wait for idle.
            if t.Type == script.TimerNormal && p.delayed {
                continue
            }
            // Reset clock BEFORE firing so the script can cleartimer
            // or settimer without interfering.
            t.Clock = s.currentTick
            sf := s.scriptProvider.GetByLookupKey(id)
            if sf == nil {
                continue
            }
            s.runScript(sf, p, false, []int{t.IntArg}, nil)
        }
    }
}
```

Wire into the tick loop:
```go
s.processClientsIn()
s.processActiveScripts()
s.processPlayerTimers()   // ← NEW
s.processPathing()
...
```

## Testing

**Handler unit tests** (`pkg/script/handlers_player_test.go` append, or new file):

- `TestSetTimerCapturesArgs`: script `[push id, push interval, push arg, settimer, return]`, mockPlayer captures `SetTimer` call with all four fields.
- `TestSoftTimerSetsSoftType`: same, verify qtype == TimerSoft.
- `TestClearTimer`: mock captures ClearTimer(scriptID).
- `TestGetTimer`: mock returns a known remaining; handler pushes it.

**E2E tests** (`modules/world/script_test.go`):

- `TestSetTimerFiresAfterInterval`: seed mainProvider with a greet script at id 0xDEADBEEF. Call `p.SetTimer(0xDEADBEEF, 5, 0, script.TimerNormal)`. Advance `s.currentTick += 5`. Call `s.processPlayerTimers()`. Drain conn; expect "g\n" on wire. Then advance by 5 more ticks; assert it fires again (interval semantics).
- `TestSoftTimerFiresWhileDelayed`: similar but `p.delayed = true`; TimerSoft fires anyway; TimerNormal doesn't.
- `TestClearTimerStopsFiring`: set a timer, clear it, advance 100 ticks, assert no wire bytes.

## LOC estimate

| File | LOC |
|---|---|
| `pkg/script/timer.go` | 20 |
| `pkg/script/active.go` (diff) | +10 |
| `pkg/script/handlers_player.go` (diff) or new handlers_timer.go | +80 |
| `pkg/script/handlers.go` (diff) | +7 (register 5) |
| `pkg/script/runner_test.go` (diff) | +30 (mockPlayer captures) |
| `pkg/script/handlers_player_test.go` (diff) | +140 |
| `modules/world/player.go` (diff) | +3 (struct + field) |
| `modules/world/player_timer.go` (new) | ~60 |
| `modules/world/tick.go` (diff) | +55 |
| `modules/world/script_test.go` (diff) | +150 |
| **Total** | **~555** |

## Key design calls

- **Single int arg** (no vararg): matches our `EnqueueScriptTyped` shape. VARARG timer support is deferred with the other VARARG opcodes.
- **Sorted iteration** for deterministic fire order. Go maps are unordered; tests flake without a stable order.
- **Clock reset before fire** matches TS — the script can `cleartimer(self)` mid-run safely (we check ok-from-map after reset). Actually: TS does the clock reset before Execute, which we match.
- **`ClearTimer` works across types**: both CLEARTIMER and CLEARSOFTTIMER cancel regardless of stored type. Matches TS.
- **No per-player timer cap**: risk of runaway script-spam. Acceptable since scripts control their own timer registration and each replaces at the same scriptID.
- **Timer on disconnect**: `Player.timers` goes with the GC'd Player. `processPlayerTimers` only iterates `s.playerLoop` so logged-out players don't fire. No explicit cleanup needed.

## Gotchas

- **Pop order**: TS `popInts(3)` fills top-down → stack top is `arg`, middle `interval`, bottom `scriptID`. Verify during implementation.
- **Timer fires at same tick when `currentTick == Clock + Interval`** (>=, not just ==). First fire happens at `Clock + Interval` (i.e. `Interval` ticks after creation).
- **Interval 0**: fires every tick. Document as "caller beware" but don't cap.
- **newTestServer() path**: existing tests seed `s.currentTick = 0` and `p.client.server = s`. The test helpers pattern in `script_test.go` is already established for S4/S5h. Follow.
- **Heredoc `!=` bug**: use Edit/Write for test files.
