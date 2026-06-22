# RuneScript S5i: Timer Ops Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Register 5 timer handlers (SETTIMER, SOFTTIMER, CLEARTIMER, CLEARSOFTTIMER, GETTIMER). Add `PlayerTimerType` enum. Extend `ActivePlayer` with `SetTimer` / `ClearTimer` / `GetTimer`. Player gains a `timers` map + a tick phase to fire ready timers.

**Spec:** [`docs/superpowers/specs/2026-04-21-runescript-s5i-timer-ops-design.md`](../specs/2026-04-21-runescript-s5i-timer-ops-design.md)

---

## Task 1: PlayerTimerType + ActivePlayer + mockPlayer

**Files:**
- Create: `pkg/script/timer.go`
- Modify: `pkg/script/active.go`
- Modify: `pkg/script/runner_test.go`

- [ ] **Step 1: Create `pkg/script/timer.go`:**

```go
package script

// PlayerTimerType mirrors TS PlayerTimerType. Normal timers wait for
// idle before firing; Soft timers fire regardless of busy state.
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

- [ ] **Step 2: Add 3 methods to `ActivePlayer` in `pkg/script/active.go`** after the S5h action-clear block (StopAction/ClearPendingAction):

```go
// S5i: timer ops.

// SetTimer registers a timer that re-runs the script at scriptID every
// `interval` ticks with `intArg` as the single int arg. Overwrites any
// existing timer at the same scriptID. type = TimerNormal (waits for
// idle) or TimerSoft (fires while busy).
SetTimer(scriptID uint32, interval int, intArg int, ttype PlayerTimerType)

// ClearTimer cancels the timer at scriptID, regardless of type.
// Silent no-op if no such timer.
ClearTimer(scriptID uint32)

// GetTimer returns the number of ticks until the timer at scriptID
// fires next, or -1 if no such timer exists. May be negative if
// overdue but not yet processed.
GetTimer(scriptID uint32) int
```

- [ ] **Step 3: Extend `mockPlayer` in `pkg/script/runner_test.go`**:

```go
// S5i capture fields
lastSetTimer    struct{ scriptID uint32; interval, intArg int; ttype PlayerTimerType }
setTimerCalls   int
lastClearTimer  uint32
clearTimerCalls int
getTimerValue   int // pre-seed for GetTimer return
```

And methods:

```go
func (m *mockPlayer) SetTimer(scriptID uint32, interval, intArg int, ttype PlayerTimerType) {
    m.lastSetTimer = struct{ scriptID uint32; interval, intArg int; ttype PlayerTimerType }{scriptID, interval, intArg, ttype}
    m.setTimerCalls++
}
func (m *mockPlayer) ClearTimer(scriptID uint32) {
    m.lastClearTimer = scriptID
    m.clearTimerCalls++
}
func (m *mockPlayer) GetTimer(scriptID uint32) int { return m.getTimerValue }
```

- [ ] **Step 4: Build pkg/script**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/script/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```

Both pass. modules/world breaks temporarily — Task 3 fixes.

- [ ] **Step 5: Commit**

```bash
git add pkg/script/timer.go pkg/script/active.go pkg/script/runner_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S5i PlayerTimerType + ActivePlayer.Set/Clear/GetTimer

Normal vs Soft timer types mirror TS PlayerTimerType. Three new
ActivePlayer methods carry a single int arg (VARARG deferred). mockPlayer
fixture captures each call.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Handlers + tests

**Files:**
- Create: `pkg/script/handlers_timer.go`
- Create: `pkg/script/handlers_timer_test.go`
- Modify: `pkg/script/handlers.go`

- [ ] **Step 1: Verify opcode constants exist** in `pkg/script/opcode.go`. Grep for: OpSetTimer, OpSoftTimer, OpClearTimer, OpClearSoftTimer, OpGetTimer. Survey expects all 5 present; report any missing.

- [ ] **Step 2: Read TS `$HOME/Code/github.com/LostCityRS/Engine-TS/src/engine/script/handlers/PlayerOps.ts`** lines 817-864 for exact pop order per handler. Expected:
- SETTIMER / SOFTTIMER: pop `(scriptID, interval, arg)` via popInts(3) — stack top is arg.
- CLEARTIMER / CLEARSOFTTIMER: pop `scriptID`.
- GETTIMER: pop `scriptID`, push remaining or -1.

- [ ] **Step 3: Create `pkg/script/handlers_timer.go`**:

```go
package script

import (
    "errors"
    "fmt"
)

// enqueueTimer is the shared body for SETTIMER / SOFTTIMER.
func enqueueTimer(s *ScriptState, ttype PlayerTimerType, op string) error {
    if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
        return fmt.Errorf("%s: no active player", op)
    }
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

- [ ] **Step 4: Register 5 handlers in `pkg/script/handlers.go`** at end of map with `// S5i: timer ops.` comment block.

- [ ] **Step 5: Create `pkg/script/handlers_timer_test.go`** via Edit tool (no heredoc):

```go
package script

import "testing"

func TestSetTimerCapturesArgs(t *testing.T) {
    sf := &ScriptFile{
        Name: "set_timer",
        Opcodes: []Opcode{
            OpPushConstantInt, // scriptID
            OpPushConstantInt, // interval
            OpPushConstantInt, // arg
            OpSetTimer,
            OpReturn,
        },
        IntOperands:      []int32{0x12345678, 5, 42, 0, 0},
        StringOperands:   []string{"", "", "", "", ""},
        InstructionCount: 5,
    }
    mp := &mockPlayer{}
    state := Init(sf, mp, false, nil, nil)
    if err := Execute(state); err != nil {
        t.Fatalf("Execute: %v", err)
    }
    if mp.setTimerCalls != 1 {
        t.Fatalf("setTimerCalls: got %d, want 1", mp.setTimerCalls)
    }
    got := mp.lastSetTimer
    if got.scriptID != 0x12345678 || got.interval != 5 || got.intArg != 42 || got.ttype != TimerNormal {
        t.Errorf("lastSetTimer: got %+v, want scriptID=0x12345678 interval=5 intArg=42 type=Normal", got)
    }
}

func TestSoftTimerSetsSoftType(t *testing.T) {
    sf := &ScriptFile{
        Name: "soft_timer",
        Opcodes: []Opcode{
            OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
            OpSoftTimer, OpReturn,
        },
        IntOperands:      []int32{0xABCDEF00, 3, 7, 0, 0},
        StringOperands:   []string{"", "", "", "", ""},
        InstructionCount: 5,
    }
    mp := &mockPlayer{}
    state := Init(sf, mp, false, nil, nil)
    if err := Execute(state); err != nil {
        t.Fatalf("Execute: %v", err)
    }
    if mp.lastSetTimer.ttype != TimerSoft {
        t.Errorf("ttype: got %v, want TimerSoft", mp.lastSetTimer.ttype)
    }
}

func TestClearTimerCapturesID(t *testing.T) {
    sf := &ScriptFile{
        Name: "clear_timer",
        Opcodes: []Opcode{
            OpPushConstantInt, OpClearTimer, OpReturn,
        },
        IntOperands:      []int32{0x11111111, 0, 0},
        StringOperands:   []string{"", "", ""},
        InstructionCount: 3,
    }
    mp := &mockPlayer{}
    state := Init(sf, mp, false, nil, nil)
    if err := Execute(state); err != nil {
        t.Fatalf("Execute: %v", err)
    }
    if mp.lastClearTimer != 0x11111111 || mp.clearTimerCalls != 1 {
        t.Errorf("ClearTimer: got %#x (x%d calls), want 0x11111111 (x1)", mp.lastClearTimer, mp.clearTimerCalls)
    }
}

func TestGetTimer(t *testing.T) {
    sf := &ScriptFile{
        Name: "get_timer",
        Opcodes: []Opcode{
            OpPushConstantInt, OpGetTimer, OpReturn,
        },
        IntOperands:      []int32{0x22222222, 0, 0},
        StringOperands:   []string{"", "", ""},
        InstructionCount: 3,
    }
    mp := &mockPlayer{getTimerValue: 99}
    state := Init(sf, mp, false, nil, nil)
    if err := Execute(state); err != nil {
        t.Fatalf("Execute: %v", err)
    }
    if got := state.PopInt(); got != 99 {
        t.Errorf("GETTIMER push: got %d, want 99", got)
    }
}

func TestTimerOpsRequireActivePlayer(t *testing.T) {
    for _, op := range []Opcode{OpSetTimer, OpSoftTimer, OpClearTimer, OpClearSoftTimer, OpGetTimer} {
        t.Run(op.String(), func(t *testing.T) {
            sf := &ScriptFile{
                Name: "no_self",
                Opcodes: []Opcode{
                    OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
                    op, OpReturn,
                },
                IntOperands:      []int32{0, 0, 0, 0, 0},
                StringOperands:   []string{"", "", "", "", ""},
                InstructionCount: 5,
            }
            state := Init(sf, nil, false, nil, nil)
            if err := Execute(state); err == nil {
                t.Errorf("%v: want error with nil Self", op)
            }
        })
    }
}
```

- [ ] **Step 6: Run**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestSetTimer|TestSoftTimer|TestClearTimer|TestGetTimer|TestTimerOps'
```

- [ ] **Step 7: Commit**

```bash
git add pkg/script/handlers_timer.go pkg/script/handlers_timer_test.go pkg/script/handlers.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S5i timer handlers (SET/SOFT/CLEAR/CLEAR-SOFT/GET TIMER)

SETTIMER and SOFTTIMER pop (scriptID, interval, arg) and call
Self.SetTimer with the appropriate type. CLEARTIMER and CLEARSOFTTIMER
both delegate to Self.ClearTimer (cancel by id regardless of type).
GETTIMER pops scriptID and pushes Self.GetTimer(scriptID).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Player state + impl + tick phase + E2E

**Files:**
- Modify: `modules/world/player.go` (+timers field + playerTimer struct)
- Create: `modules/world/player_timer.go` (Set/Clear/GetTimer impls)
- Modify: `modules/world/tick.go` (processPlayerTimers phase)
- Modify: `modules/world/script_test.go` (E2E)

- [ ] **Step 1: Add `timers` field + struct** in `modules/world/player.go`. Find the section near `queue` / S5g fields:

```go
// playerTimer is a per-player repeating script registration.
// S5i: identified by target scriptID (TS semantics: setTimer at same
// id overwrites).
type playerTimer struct {
    ScriptID uint32
    Type     script.PlayerTimerType
    Interval int
    Clock    int // last-fired (or creation) absolute tick
    IntArg   int
}
```

Add field in Player struct (near `queue`):
```go
// timers is a per-player repeating-script map keyed by script lookup
// key. Allocated lazily on first SetTimer call.
timers map[uint32]*playerTimer
```

- [ ] **Step 2: Create `modules/world/player_timer.go`** with Set/Clear/GetTimer impls:

```go
package world

import "github.com/zsrv/goscape/pkg/script"

// SetTimer implements script.ActivePlayer.SetTimer.
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

// ClearTimer implements script.ActivePlayer.ClearTimer.
func (p *Player) ClearTimer(scriptID uint32) {
    if p.timers == nil {
        return
    }
    delete(p.timers, scriptID)
}

// GetTimer implements script.ActivePlayer.GetTimer. Returns -1 if no
// timer is registered at scriptID.
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
    return (t.Clock + t.Interval) - now
}
```

- [ ] **Step 3: Add `processPlayerTimers` in `modules/world/tick.go`** and wire into the loop:

```go
// processPlayerTimers fires any ready timers. Soft timers fire even
// while p.delayed; normal timers wait for idle.
func (s *Server) processPlayerTimers() {
    s.playersMu.RLock()
    players := make([]*Player, len(s.playerLoop))
    copy(players, s.playerLoop)
    s.playersMu.RUnlock()

    for _, p := range players {
        if len(p.timers) == 0 {
            continue
        }
        // Deterministic fire order (maps are unordered).
        ids := make([]uint32, 0, len(p.timers))
        for id := range p.timers {
            ids = append(ids, id)
        }
        sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

        for _, id := range ids {
            t, ok := p.timers[id]
            if !ok {
                continue
            }
            if s.currentTick < t.Clock+t.Interval {
                continue
            }
            if t.Type == script.TimerNormal && p.delayed {
                continue
            }
            t.Clock = s.currentTick
            if s.scriptProvider == nil {
                continue
            }
            sf := s.scriptProvider.GetByLookupKey(id)
            if sf == nil {
                continue
            }
            s.runScript(sf, p, false, []int{t.IntArg}, nil)
        }
    }
}
```

Add `"sort"` to tick.go's imports.

Wire into the tick loop — locate the `runTickLoopWithRate` loop body, insert after `processActiveScripts`:

```go
s.processClientsIn()
s.processActiveScripts()
s.processPlayerTimers()    // ← NEW
s.processPathing()
// ... rest ...
```

- [ ] **Step 4: Full build + test**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Clean. The `var _ script.ActivePlayer = (*Player)(nil)` check confirms interface compliance.

- [ ] **Step 5: Add E2E tests** in `modules/world/script_test.go` (Edit tool, no heredoc):

```go
func TestSetTimerFiresAfterInterval(t *testing.T) {
    s := newTestServer(t)
    s.scriptProvider = script.NewProvider()
    s.scriptProvider.Register(buildGreetScript(0xTIMER, "t"))
    s.configsView = serverConfigsView{s: s}
    s.invLookup = invLookupView{s: s}

    p, cc := newTestPlayer(t)
    p.client.server = s
    p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
    s.playerLoop = append(s.playerLoop, p)

    // Register a timer at interval=5, starting at current tick 0.
    p.SetTimer(0xTIMER, 5, 0, script.TimerNormal)

    received := drainConn(t, cc)

    // Tick 0..4: no fire.
    for i := 0; i < 5; i++ {
        s.processPlayerTimers()
        s.currentTick++
    }
    // Now currentTick = 5, timer fires.
    s.processPlayerTimers()
    p.client.flushWrite()
    got := <-received
    if len(got) != 4 {
        t.Fatalf("fire at interval: got %d bytes, want 4", len(got))
    }
    if string(got[2:3]) != "t" {
        t.Errorf("payload: got %q, want 't'", got[2:])
    }
}

func TestSoftTimerFiresWhileDelayed(t *testing.T) {
    s := newTestServer(t)
    s.scriptProvider = script.NewProvider()
    s.scriptProvider.Register(buildGreetScript(0xSOFT, "s"))
    s.configsView = serverConfigsView{s: s}
    s.invLookup = invLookupView{s: s}

    p, cc := newTestPlayer(t)
    p.client.server = s
    p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
    s.playerLoop = append(s.playerLoop, p)
    p.delayed = true
    p.delayedUntil = s.currentTick + 99

    p.SetTimer(0xSOFT, 1, 0, script.TimerSoft)

    received := drainConn(t, cc)
    s.currentTick = 1
    s.processPlayerTimers()
    p.client.flushWrite()
    got := <-received
    if len(got) != 4 {
        t.Errorf("Soft timer while delayed: got %d bytes, want 4 (fire)", len(got))
    }
}

func TestClearTimerStopsFiring(t *testing.T) {
    s := newTestServer(t)
    s.scriptProvider = script.NewProvider()
    s.scriptProvider.Register(buildGreetScript(0xSTOP, "x"))
    s.configsView = serverConfigsView{s: s}
    s.invLookup = invLookupView{s: s}

    p, cc := newTestPlayer(t)
    p.client.server = s
    p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
    s.playerLoop = append(s.playerLoop, p)

    p.SetTimer(0xSTOP, 1, 0, script.TimerNormal)
    p.ClearTimer(0xSTOP)

    received := drainConn(t, cc)
    for i := 0; i < 10; i++ {
        s.currentTick++
        s.processPlayerTimers()
    }
    p.client.flushWrite()

    select {
    case got := <-received:
        if len(got) > 0 {
            t.Errorf("cleared timer fired: got %d bytes, want 0", len(got))
        }
    case <-time.After(50 * time.Millisecond):
        // expected
    }
}
```

**Note**: The literal `0xTIMER` etc. are not valid Go hex — use actual hex like `0xA1` or decimal. Adapt the test constants to be valid numbers.

- [ ] **Step 6: Run + race + vet + handler count**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestSetTimerFires|TestSoftTimerFires|TestClearTimerStops' -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
grep -cE "^\s+Op[A-Z].*handle" pkg/script/handlers.go
```

Handler count: **185** (180 + 5).

- [ ] **Step 7: Commit**

```bash
git add modules/world/player.go modules/world/player_timer.go modules/world/tick.go modules/world/script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): Player timers + processPlayerTimers tick phase

Adds Player.timers map (keyed by scriptID) with SetTimer/ClearTimer/
GetTimer impls. New processPlayerTimers tick phase (inserted between
processActiveScripts and processPathing) iterates timers in sorted-id
order, fires any whose Clock + Interval has elapsed, and re-runs the
target script via runScript with the stored int arg. Soft timers fire
regardless of p.delayed; normal timers skip while delayed.

E2E tests cover interval firing, soft-while-delayed, and
clear-stops-firing.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review

- [ ] `go build ./...` clean
- [ ] `go test ./...` passes
- [ ] `go test -race ./...` clean
- [ ] `go vet ./...` clean
- [ ] Handler count = 185
