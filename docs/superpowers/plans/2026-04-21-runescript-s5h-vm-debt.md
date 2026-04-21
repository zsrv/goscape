# RuneScript S5h: VM Debt Bundle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Register 7 handlers (JUMP / JUMP_WITH_PARAMS / WEAKQUEUE / STRONGQUEUE / LONGQUEUE / P_STOPACTION / P_CLEARPENDINGACTION). Rename `EnqueueScript` → `EnqueueScriptTyped` with a `PlayerQueueType` parameter. Add `ScriptState.JumpCall` for tail-calls. Gate STRONG queue entries to fire while delayed.

**Spec:** [`docs/superpowers/specs/2026-04-21-runescript-s5h-vm-debt-design.md`](../specs/2026-04-21-runescript-s5h-vm-debt-design.md)

---

## Task 1: Queue type enum + ActivePlayer rename + mockPlayer

**Files:**
- Create: `pkg/script/queue.go`
- Modify: `pkg/script/active.go` (rename EnqueueScript, add StopAction + ClearPendingAction)
- Modify: `pkg/script/runner_test.go` (update mockPlayer)
- Modify: `pkg/script/handlers_vars.go` (update handleQueue to pass QueueNormal)

- [ ] **Step 1: Create `pkg/script/queue.go`**:

```go
package script

// PlayerQueueType mirrors TS PlayerQueueType. Determines when a
// queued script fires relative to the player's busy state.
type PlayerQueueType uint8

const (
    QueueNormal PlayerQueueType = iota
    QueueStrong
    QueueWeak
    QueueLong
    QueueEngine // reserved
    QueueSoft   // reserved
)

func (q PlayerQueueType) String() string {
    switch q {
    case QueueNormal:
        return "Normal"
    case QueueStrong:
        return "Strong"
    case QueueWeak:
        return "Weak"
    case QueueLong:
        return "Long"
    case QueueEngine:
        return "Engine"
    case QueueSoft:
        return "Soft"
    default:
        return "Unknown"
    }
}
```

- [ ] **Step 2: In `pkg/script/active.go`** find the existing line:
```go
EnqueueScript(scriptID uint32, delay int, intArg int)
```
**Replace** with:
```go
// EnqueueScriptTyped appends a queued fresh-run request with the
// given queue type. Delay=0 fires same tick. STRONG-type entries
// fire even if the player is busy; others wait until idle.
// (S5h: renamed from EnqueueScript to carry type.)
EnqueueScriptTyped(scriptID uint32, delay int, intArg int, qtype PlayerQueueType)
```

Also add after the S5g block:
```go
// S5h: action-clear ops.

// StopAction clears the current interaction target + pending action.
// Matches TS Player.stopAction().
StopAction()

// ClearPendingAction clears the current interaction + pending action
// + closes any open modal. Walk queue is preserved.
ClearPendingAction()
```

- [ ] **Step 3: Update `mockPlayer`** in `runner_test.go`:
- Rename capture: `enqueueCalls` now has `Type` field. Update struct:
  ```go
  type mockEnqueue struct {
      ScriptID uint32
      Delay    int
      IntArg   int
      Type     PlayerQueueType
  }
  ```
- Rename method `EnqueueScript` → `EnqueueScriptTyped`. Append `{id, delay, arg, qtype}`.
- Add methods `StopAction()` and `ClearPendingAction()` with capture counters.

- [ ] **Step 4: Update `pkg/script/handlers_vars.go` handleQueue** to pass `QueueNormal`:

```go
s.Self.EnqueueScriptTyped(scriptID, delay, arg, QueueNormal)
```

- [ ] **Step 5: Build pkg/script**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/script/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```

Must pass. (modules/world will break — fixed in Task 4.)

- [ ] **Step 6: Commit**

```bash
git add pkg/script/queue.go pkg/script/active.go pkg/script/runner_test.go pkg/script/handlers_vars.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S5h PlayerQueueType + EnqueueScriptTyped + action-clear methods

Adds PlayerQueueType enum (Normal/Strong/Weak/Long/Engine/Soft).
Renames ActivePlayer.EnqueueScript to EnqueueScriptTyped with a
qtype parameter — the existing QUEUE handler now passes QueueNormal.
Adds StopAction + ClearPendingAction methods. mockPlayer fixture
updated.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `ScriptState.JumpCall` + JUMP handlers

**Files:**
- Modify: `pkg/script/state.go`
- Modify: `pkg/script/handlers.go` (or create handlers_core.go for JUMP handlers)
- Possibly refactor: `pkg/script/handlers.go` — factor out `popArgsForTarget` if not already done

- [ ] **Step 1: Add `JumpCall` to `ScriptState` in `pkg/script/state.go`**:

```go
// JumpCall performs a tail-call to target, discarding all saved frames.
// Distinct from GosubCall which saves the caller frame. TS reference:
// ScriptState.gotoFrame → setupNewScript.
func (s *ScriptState) JumpCall(target *ScriptFile, intArgs []int, stringArgs []string) {
    s.FrameSP = 0

    intLocals := make([]int, max(int(target.IntLocalCount), len(intArgs)))
    for i, v := range intArgs {
        intLocals[i] = v
    }
    stringLocals := make([]string, max(int(target.StringLocalCount), len(stringArgs)))
    for i, v := range stringArgs {
        stringLocals[i] = v
    }

    s.Script = target
    s.PC = -1
    s.IntLocals = intLocals
    s.StringLocals = stringLocals
}
```

- [ ] **Step 2: Factor out `popArgsForTarget` helper** if needed. Read `handleGosubWithParams` in `handlers.go`. If it has inline arg-popping logic, extract it into a helper in handlers.go:

```go
// popArgsForTarget pops int + string args in reverse order based on
// target.ParamTypes. Returns (intArgs, stringArgs) ready to pass to
// GosubCall / JumpCall.
func popArgsForTarget(s *ScriptState, target *ScriptFile) ([]int, []string) {
    // ... same logic as GOSUB_WITH_PARAMS ...
}
```

Then update `handleGosubWithParams` to use the helper.

- [ ] **Step 3: Add JUMP + JUMP_WITH_PARAMS handlers**. Create `pkg/script/handlers_core.go`:

```go
package script

import (
    "errors"
    "fmt"
)

// handleJump pops the target script id from the int stack and tail-
// calls it with no args. TS CoreOps.ts JUMP.
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
    intArgs, stringArgs := popArgsForTarget(s, target)
    s.JumpCall(target, intArgs, stringArgs)
    return nil
}
```

- [ ] **Step 4: Register both in `handlers.go`** at end of map:

```go
// S5h: tail-call.
OpJump:           handleJump,
OpJumpWithParams: handleJumpWithParams,
```

- [ ] **Step 5: Write JUMP tests** in a new `pkg/script/handlers_core_test.go`:

Test `TestJumpClearsFrameStack`:
1. Build script A: `[gosub B, return]`
2. Build script B: `[jump C, return]`
3. Build script C: `[push "done", mes, return]`
4. Seed provider with B and C.
5. Run A. Verify mockPlayer received "done" exactly once, Execution is Finished.
6. Verify FrameSP == 0 after JUMP (no A frame to return into).

Test `TestJumpWithParams`:
1. Build target: `[push_int_local 0, return]` (IntLocalCount=1).
2. Build caller: `[push_constant_int 77, jump_with_params target]` (operand = target id).
3. Run caller.
4. Verify state.PopInt() == 77.

- [ ] **Step 6: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestJump'
```

- [ ] **Step 7: Commit**

```bash
git add pkg/script/state.go pkg/script/handlers.go pkg/script/handlers_core.go pkg/script/handlers_core_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S5h JUMP + JUMP_WITH_PARAMS tail-call handlers

ScriptState.JumpCall discards the frame stack (unlike GosubCall which
saves) before re-initing the callee's locals and setting PC=-1. JUMP
pops target scriptID from the int stack; JUMP_WITH_PARAMS reads from
the operand and pops int+string args per target.ParamTypes via the
shared popArgsForTarget helper (factored out from handleGosubWithParams).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Queue variant + action-clear handlers

**Files:**
- Modify: `pkg/script/handlers_vars.go` (add queue variants + factor enqueueTyped helper)
- Modify: `pkg/script/handlers_player.go` (add P_STOPACTION + P_CLEARPENDINGACTION)
- Modify: `pkg/script/handlers.go` (register 5 handlers)
- Add tests: `pkg/script/handlers_vars_test.go` extend; `pkg/script/handlers_player_test.go` extend

- [ ] **Step 1: Refactor handleQueue + add variants** in `handlers_vars.go`:

```go
// enqueueTyped is the shared body for QUEUE / WEAKQUEUE / STRONGQUEUE /
// LONGQUEUE. Pops (scriptID, delay, arg) in TS popInts(3) order and
// calls Self.EnqueueScriptTyped with the requested type.
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

func handleQueue(s *ScriptState) error {
    return enqueueTyped(s, QueueNormal, "QUEUE")
}

func handleWeakQueue(s *ScriptState) error {
    return enqueueTyped(s, QueueWeak, "WEAKQUEUE")
}

func handleStrongQueue(s *ScriptState) error {
    return enqueueTyped(s, QueueStrong, "STRONGQUEUE")
}

func handleLongQueue(s *ScriptState) error {
    return enqueueTyped(s, QueueLong, "LONGQUEUE")
}
```

- [ ] **Step 2: Add action-clear handlers** in `handlers_player.go` (append to file):

```go
// S5h: action-clear.

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

- [ ] **Step 3: Register 5 handlers in `handlers.go`**:

Under S5h block:
```go
OpWeakQueue:           handleWeakQueue,
OpStrongQueue:         handleStrongQueue,
OpLongQueue:           handleLongQueue,
OpPStopAction:         handlePStopAction,
OpPClearPendingAction: handlePClearPendingAction,
```

- [ ] **Step 4: Add tests** — extend existing `handlers_vars_test.go` with:

```go
func TestQueueVariants(t *testing.T) {
    cases := []struct {
        name  string
        op    Opcode
        qtype PlayerQueueType
    }{
        {"weak", OpWeakQueue, QueueWeak},
        {"strong", OpStrongQueue, QueueStrong},
        {"long", OpLongQueue, QueueLong},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            sf := &ScriptFile{
                Name: "q_" + tc.name,
                Opcodes: []Opcode{
                    OpPushConstantInt, // scriptID 77
                    OpPushConstantInt, // delay 3
                    OpPushConstantInt, // arg 42
                    tc.op,
                    OpReturn,
                },
                IntOperands:      []int32{77, 3, 42, 0, 0},
                StringOperands:   []string{"", "", "", "", ""},
                InstructionCount: 5,
            }
            mp := &mockPlayer{}
            state := Init(sf, mp, false, nil, nil)
            if err := Execute(state); err != nil {
                t.Fatalf("Execute: %v", err)
            }
            if len(mp.enqueueCalls) != 1 {
                t.Fatalf("enqueueCalls: got %d, want 1", len(mp.enqueueCalls))
            }
            got := mp.enqueueCalls[0]
            if got.ScriptID != 77 || got.Delay != 3 || got.IntArg != 42 || got.Type != tc.qtype {
                t.Errorf("enqueue: got %+v, want type=%v", got, tc.qtype)
            }
        })
    }
}
```

And in `handlers_player_test.go`:

```go
func TestPStopAction(t *testing.T) {
    sf := &ScriptFile{
        Name:             "stop",
        Opcodes:          []Opcode{OpPStopAction, OpReturn},
        IntOperands:      []int32{0, 0},
        StringOperands:   []string{"", ""},
        InstructionCount: 2,
    }
    mp := &mockPlayer{}
    state := Init(sf, mp, false, nil, nil)
    if err := Execute(state); err != nil {
        t.Fatalf("Execute: %v", err)
    }
    if mp.stopActionCalls != 1 {
        t.Errorf("stopActionCalls: got %d, want 1", mp.stopActionCalls)
    }
}

func TestPClearPendingAction(t *testing.T) {
    sf := &ScriptFile{
        Name:             "clear",
        Opcodes:          []Opcode{OpPClearPendingAction, OpReturn},
        IntOperands:      []int32{0, 0},
        StringOperands:   []string{"", ""},
        InstructionCount: 2,
    }
    mp := &mockPlayer{}
    state := Init(sf, mp, false, nil, nil)
    if err := Execute(state); err != nil {
        t.Fatalf("Execute: %v", err)
    }
    if mp.clearPendingActionCalls != 1 {
        t.Errorf("clearPendingActionCalls: got %d, want 1", mp.clearPendingActionCalls)
    }
}
```

- [ ] **Step 5: Run tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```

- [ ] **Step 6: Commit**

```bash
git add pkg/script/handlers_vars.go pkg/script/handlers_player.go pkg/script/handlers.go \
        pkg/script/handlers_vars_test.go pkg/script/handlers_player_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): S5h queue variants + action-clear handlers

WEAK / STRONG / LONG QUEUE handlers delegate to a shared enqueueTyped
helper that routes QueueType to EnqueueScriptTyped. P_STOPACTION calls
Self.StopAction; P_CLEARPENDINGACTION calls Self.ClearPendingAction.
Existing QUEUE handler refactored to pass QueueNormal via the same
helper.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Player impls + STRONG-queue tick gating

**Files:**
- Modify: `modules/world/player_script.go` (rename, add methods)
- Modify: `modules/world/tick.go` (processPlayerQueue)
- Modify: `modules/world/script_test.go` (update existing tests + add E2E)

- [ ] **Step 1: Rename playerQueueRequest field + EnqueueScriptTyped** in `player_script.go`:

```go
type playerQueueRequest struct {
    ScriptID uint32
    Delay    int
    IntArg   int
    Type     script.PlayerQueueType
}

// EnqueueScriptTyped implements script.ActivePlayer.EnqueueScriptTyped.
func (p *Player) EnqueueScriptTyped(scriptID uint32, delay, intArg int, qtype script.PlayerQueueType) {
    p.queue = append(p.queue, playerQueueRequest{
        ScriptID: scriptID,
        Delay:    delay,
        IntArg:   intArg,
        Type:     qtype,
    })
}
```

Remove the old `EnqueueScript` method entirely.

- [ ] **Step 2: Add StopAction + ClearPendingAction** to `player_script.go`:

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

Verify `ClearInteraction` exists in `modules/world/interaction.go`. If yes, reuse. If not, inline the same field resets.

Verify `InteractionKindNone` exists; grep to find the right constant name.

- [ ] **Step 3: STRONG-queue gating** in `modules/world/tick.go` `processPlayerQueue`:

Find:
```go
if req.Delay > 0 || p.delayed {
    i++
    continue
}
```

Replace with:
```go
if req.Delay > 0 {
    i++
    continue
}
// STRONG queue fires even when delayed; others wait for idle.
if p.delayed && req.Type != script.QueueStrong {
    i++
    continue
}
```

- [ ] **Step 4: Full build + test**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: existing tests that called `p.EnqueueScript(...)` will need updating to `p.EnqueueScriptTyped(..., script.QueueNormal)`. Grep for `EnqueueScript` in tests and update.

- [ ] **Step 5: Add E2E test `TestStrongQueueFiresWhileDelayed`** in `script_test.go` (Edit tool, not heredoc):

```go
func TestStrongQueueFiresWhileDelayed(t *testing.T) {
    s := newTestServer(t)
    s.scriptProvider = script.NewProvider()
    s.scriptProvider.Register(buildGreetScript(0xBEEF, "s"))
    s.configsView = serverConfigsView{s: s}
    s.invLookup = invLookupView{s: s}

    p, cc := newTestPlayer(t)
    p.client.server = s
    p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
    s.playerLoop = append(s.playerLoop, p)

    // Force the player into a busy (delayed) state.
    p.delayed = true
    p.delayedUntil = s.currentTick + 99

    received := drainConn(t, cc)

    // Enqueue a STRONG script with delay=0 — should fire even though delayed.
    p.EnqueueScriptTyped(0xBEEF, 0, 0, script.QueueStrong)
    s.processActiveScripts()
    p.client.flushWrite()
    got := <-received

    if len(got) != 4 {
        t.Fatalf("STRONG fire: got %d bytes, want 4", len(got))
    }
    if p.delayed == false {
        // the handleGreet script shouldn't have cleared delayed; assert intact
        t.Error("player should still be delayed after STRONG fire")
    }
}

func TestNormalQueueWaitsForIdle(t *testing.T) {
    s := newTestServer(t)
    s.scriptProvider = script.NewProvider()
    s.scriptProvider.Register(buildGreetScript(0xBEE2, "n"))
    s.configsView = serverConfigsView{s: s}
    s.invLookup = invLookupView{s: s}

    p, cc := newTestPlayer(t)
    p.client.server = s
    p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
    s.playerLoop = append(s.playerLoop, p)

    p.delayed = true
    p.delayedUntil = s.currentTick + 99

    received := drainConn(t, cc)
    p.EnqueueScriptTyped(0xBEE2, 0, 0, script.QueueNormal)
    s.processActiveScripts()
    p.client.flushWrite()

    // Use a short timeout-style check: try to read; expect 0 bytes.
    select {
    case got := <-received:
        if len(got) > 0 {
            t.Errorf("NORMAL: got %d bytes fired while delayed; want 0", len(got))
        }
    case <-time.After(50 * time.Millisecond):
        // expected path: nothing fired
    }
}
```

The `buildGreetScript` helper already exists in `script_test.go`.

- [ ] **Step 6: Run tests + race + vet + handler count**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestStrongQueue|TestNormalQueue' -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
grep -cE "^\s+Op[A-Z].*handle" pkg/script/handlers.go
```

Expected handler count: **180** (173 + 7).

- [ ] **Step 7: Commit**

```bash
git add modules/world/player_script.go modules/world/tick.go modules/world/script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): Player EnqueueScriptTyped + StopAction + STRONG queue firing while delayed

playerQueueRequest gains Type field. Renames EnqueueScript to
EnqueueScriptTyped. StopAction chains ClearInteraction +
ClearPendingAction; ClearPendingAction resets interaction + closes
modal (walk queue preserved). processPlayerQueue gates NORMAL/WEAK/LONG
to fire only when !delayed; STRONG fires regardless.

E2E tests: TestStrongQueueFiresWhileDelayed confirms STRONG-tagged
queued scripts run through even with p.delayed=true;
TestNormalQueueWaitsForIdle confirms the others don't.

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
- [ ] Handler count = 180
