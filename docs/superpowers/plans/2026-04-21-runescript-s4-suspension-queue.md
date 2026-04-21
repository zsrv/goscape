# RuneScript S4: Suspension + Queue Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add cooperative script suspension (`p_delay`) with cross-tick resumption and a single per-player script queue (`queue` opcode), so LOGIN and other cache scripts can time-sequence operations across multiple ticks.

**Architecture:** Extend `script.ActivePlayer` with four methods (`SetDelayed`, `EnqueueScript`, `StoreActiveScript`, `ClearActiveScript`). Add two opcode handlers (`OpPDelay = 2071`, `OpQueue = 2092`) to `pkg/script/handlers.go`. Add `activeScript` + `queue` fields to `Player`. Add a `processActiveScripts` tick phase between `processClientsIn` and `processPathing` that expires delays, resumes suspended scripts, and fires ready queue entries. TS speedup quirk preserved via index-based queue iteration.

**Tech Stack:** Go 1.22+, existing `pkg/script/` VM, existing `modules/world/` server, existing test infra (`newTestServer`, `newTestPlayer`, `drainConn`, `isaacPair`).

**Spec:** [`docs/superpowers/specs/2026-04-21-runescript-s4-suspension-queue-design.md`](../specs/2026-04-21-runescript-s4-suspension-queue-design.md)

---

## Task 1: Extend `ActivePlayer` interface

**Files:**
- Modify: `pkg/script/active.go`

- [ ] **Step 1: Edit `pkg/script/active.go` to add the four new methods**

Replace the current interface body with:

```go
package script

// ActivePlayer is the minimal surface RuneScript needs from a Player.
// Sub-spec S2 wires modules/world.Player to this interface. S4 adds
// suspension + queue methods.
type ActivePlayer interface {
	MessageGame(msg string)
	Username() string

	// SetDelayed marks the active player as suspended for `ticks` more
	// ticks starting next tick. Implementation must compute
	// resumeTick = currentTick + 1 + ticks.
	SetDelayed(ticks int)

	// EnqueueScript appends a queued fresh-run request with one int arg.
	// delay=0 fires same tick (authentic TS behavior).
	EnqueueScript(scriptID uint32, delay int, intArg int)

	// StoreActiveScript saves a Suspended ScriptState so the tick loop
	// can resume it when the player's delay expires.
	StoreActiveScript(state *ScriptState)

	// ClearActiveScript discards any stored ScriptState. Called after
	// Finished/Aborted runs and on logout/cleanup.
	ClearActiveScript()
}

// Stubs for later sub-specs; defined now to avoid interface churn in S6.
type ActiveNpc interface{}
type ActiveLoc interface{}
type ActiveObj interface{}
```

- [ ] **Step 2: Run package build to verify the interface compiles**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/script/...`
Expected: build succeeds.

- [ ] **Step 3: Run full build to confirm which call sites fail (expected)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: `modules/world` fails to build because `*Player` no longer satisfies `script.ActivePlayer`. This is expected — Task 3 adds the methods.

- [ ] **Step 4: Commit**

```bash
git add pkg/script/active.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): extend ActivePlayer with S4 suspension + queue methods

SetDelayed, EnqueueScript, StoreActiveScript, ClearActiveScript allow
the VM to signal suspension requests and queue-enqueue operations to
the embedding player implementation without pkg/script reaching into
modules/world.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Add P_DELAY + QUEUE handlers

**Files:**
- Modify: `pkg/script/handlers.go`
- Test: `pkg/script/handlers_test.go`

- [ ] **Step 1: Add an `OpPDelay` test in `pkg/script/handlers_test.go`**

Append to the end of the file:

```go
// mockActivePlayer captures ActivePlayer calls for tests.
type mockActivePlayer struct {
	messages []string
	username string

	setDelayedCalls []int
	enqueueCalls    []mockEnqueue
	stored          *ScriptState
	cleared         int
}

type mockEnqueue struct {
	ScriptID uint32
	Delay    int
	IntArg   int
}

func (m *mockActivePlayer) MessageGame(s string)                  { m.messages = append(m.messages, s) }
func (m *mockActivePlayer) Username() string                      { return m.username }
func (m *mockActivePlayer) SetDelayed(ticks int)                  { m.setDelayedCalls = append(m.setDelayedCalls, ticks) }
func (m *mockActivePlayer) EnqueueScript(id uint32, d, a int)     { m.enqueueCalls = append(m.enqueueCalls, mockEnqueue{id, d, a}) }
func (m *mockActivePlayer) StoreActiveScript(state *ScriptState)  { m.stored = state }
func (m *mockActivePlayer) ClearActiveScript()                    { m.cleared++ }

func TestPDelaySuspends(t *testing.T) {
	sf := &ScriptFile{
		Name: "test_pdelay",
		Opcodes: []Opcode{
			OpPushConstantInt, // push 5
			OpPDelay,          // delay 5
			OpReturn,
		},
		IntOperands:      []int32{5, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	mp := &mockActivePlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.Execution != Suspended {
		t.Errorf("Execution: got %v, want Suspended", state.Execution)
	}
	if len(mp.setDelayedCalls) != 1 || mp.setDelayedCalls[0] != 5 {
		t.Errorf("setDelayedCalls: got %v, want [5]", mp.setDelayedCalls)
	}
}

func TestPDelayRequiresActivePlayer(t *testing.T) {
	sf := &ScriptFile{
		Name: "test_pdelay_noself",
		Opcodes: []Opcode{
			OpPushConstantInt,
			OpPDelay,
			OpReturn,
		},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	}
	state := Init(sf, nil, false, nil, nil)
	err := Execute(state)
	if err == nil {
		t.Fatal("Execute: want error, got nil")
	}
}

func TestQueueOpcode(t *testing.T) {
	sf := &ScriptFile{
		Name: "test_queue",
		Opcodes: []Opcode{
			OpPushConstantInt, // push scriptID 77
			OpPushConstantInt, // push delay 3
			OpPushConstantInt, // push arg 42
			OpQueue,
			OpReturn,
		},
		IntOperands:      []int32{77, 3, 42, 0, 0},
		StringOperands:   []string{"", "", "", "", ""},
		InstructionCount: 5,
	}
	mp := &mockActivePlayer{}
	state := Init(sf, mp, false, nil, nil)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.Execution != Finished {
		t.Errorf("Execution: got %v, want Finished", state.Execution)
	}
	if len(mp.enqueueCalls) != 1 {
		t.Fatalf("enqueueCalls: got %d, want 1", len(mp.enqueueCalls))
	}
	got := mp.enqueueCalls[0]
	want := mockEnqueue{ScriptID: 77, Delay: 3, IntArg: 42}
	if got != want {
		t.Errorf("enqueue: got %+v, want %+v", got, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestPDelaySuspends|TestPDelayRequiresActivePlayer|TestQueueOpcode' -v`
Expected: FAIL. `OpPDelay` / `OpQueue` not registered in the `handlers` map; Execute will return `unknown opcode` errors.

- [ ] **Step 3: Implement both handlers in `pkg/script/handlers.go`**

Append after `handleConsole` (or near the other suspended-player ops):

```go
// handlePDelay implements P_DELAY (opcode 2071): pop int n, delay the
// active player by n+1 ticks, and suspend execution. TS PlayerOps.ts
// sets state.delay = n and delayedUntil = currentTick + 1 + n; we push
// the whole calculation into the ActivePlayer.SetDelayed implementation.
func handlePDelay(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("P_DELAY: no active player")
	}
	n := int(s.PopInt())
	s.Self.SetDelayed(n)
	s.Execution = Suspended
	return nil
}

// handleQueue implements QUEUE (opcode 2092): enqueue a fresh-run
// script request on the active player.
//
// TS (engine/script/handlers/PlayerOps.ts:148):
//
//	const [scriptId, delay, arg] = state.popInts(3);
//
// popInts(n) fills ints[n-1] down to ints[0] via PopInt, so the stack
// top is `arg`, then `delay`, then `scriptId`. For S4 we support only
// the single-int-arg variant (QUEUEVARARG is deferred).
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

- [ ] **Step 4: Register the handlers in the `handlers` map**

In `pkg/script/handlers.go`, extend the existing `handlers` map literal with the two new entries:

```go
var handlers = map[Opcode]func(*ScriptState) error{
	OpPushConstantInt:    handlePushConstantInt,
	OpPushConstantString: handlePushConstantString,
	OpReturn:             handleReturn,
	OpPushIntLocal:       handlePushIntLocal,
	OpPopIntLocal:        handlePopIntLocal,
	OpPushStringLocal:    handlePushStringLocal,
	OpPopStringLocal:     handlePopStringLocal,
	OpBranch:             handleBranch,
	OpBranchEquals:       handleBranchEquals,
	OpBranchNot:          handleBranchNot,
	OpPopIntDiscard:      handlePopIntDiscard,
	OpPopStringDiscard:   handlePopStringDiscard,
	OpJoinString:         handleJoinString,
	OpAdd:                handleAdd,
	OpSub:                handleSub,
	OpToString:           handleToString,
	OpGosubWithParams:    handleGosubWithParams,
	OpMes:                handleMes,
	OpName:               handleName,
	OpConsole:            handleConsole,
	OpPDelay:             handlePDelay,
	OpQueue:              handleQueue,
}
```

- [ ] **Step 5: Run the new tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestPDelaySuspends|TestPDelayRequiresActivePlayer|TestQueueOpcode' -v`
Expected: PASS.

- [ ] **Step 6: Run the full `pkg/script/` test suite to confirm no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/script/handlers.go pkg/script/handlers_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): handlePDelay + handleQueue opcode handlers

P_DELAY (2071) pops n, calls Self.SetDelayed(n), sets Execution=Suspended.
QUEUE (2092) pops [scriptID, delay, arg] in TS popInts(3) order and
calls Self.EnqueueScript. Unit tests cover both handlers against a mock
ActivePlayer.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Expose provider lookup by key

**Files:**
- Modify: `pkg/script/provider.go`
- Test: `pkg/script/provider_test.go`

- [ ] **Step 1: Add a failing test in `pkg/script/provider_test.go`**

Append at the end of the file:

```go
func TestGetByLookupKey(t *testing.T) {
	p := NewProvider()
	f := &ScriptFile{Name: "[test,key]", LookupKey: 0x1234}
	p.Register(f)

	if got := p.GetByLookupKey(0x1234); got != f {
		t.Errorf("GetByLookupKey(0x1234): got %v, want %v", got, f)
	}
	if got := p.GetByLookupKey(0x9999); got != nil {
		t.Errorf("GetByLookupKey(missing): got %v, want nil", got)
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestGetByLookupKey -v`
Expected: FAIL. `p.GetByLookupKey undefined`.

- [ ] **Step 3: Add the accessor to `pkg/script/provider.go`**

Append after `GetByName`:

```go
// GetByLookupKey returns a script by its raw uint32 key (as stored in
// byKey). Returns nil if unknown. Used by the world tick loop's queue
// dispatch, where the scriptID comes from the bytecode stream as a raw
// key.
func (p *Provider) GetByLookupKey(key uint32) *ScriptFile {
	return p.byKey[key]
}
```

- [ ] **Step 4: Run the test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestGetByLookupKey -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/script/provider.go pkg/script/provider_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): Provider.GetByLookupKey accessor

Exposes byKey lookup by uint32 so the world tick loop's queue dispatch
can resolve queued scriptIDs directly without reimplementing the trigger
machinery.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Add `Player.activeScript` + `Player.queue` fields and the 4 method impls

**Files:**
- Modify: `modules/world/player.go`
- Create: `modules/world/player_script.go`

- [ ] **Step 1: Add two fields to `Player` in `modules/world/player.go`**

Locate the block containing `delayed bool` and `delayedUntil int` (currently lines 80–81) and extend:

```go
	delayed         bool
	delayedUntil    int
	activeScript    *script.ScriptState
	queue           []playerQueueRequest
```

If `"github.com/zsrv/goscape/pkg/script"` is not yet imported in `player.go`, add it.

- [ ] **Step 2: Create `modules/world/player_script.go` with the four method impls**

Create the file with contents:

```go
package world

import "github.com/zsrv/goscape/pkg/script"

// playerQueueRequest is one queued fresh-run script request with a
// single int arg. Queue entries are processed in processActiveScripts;
// when Delay reaches zero (or below) the target script runs as a brand-
// new ScriptState.
type playerQueueRequest struct {
	ScriptID uint32
	Delay    int
	IntArg   int
}

// SetDelayed marks the player as suspended for `ticks` ticks starting
// next tick, per the P_DELAY opcode contract: the player resumes at
// currentTick + 1 + ticks.
//
// No-op if the player is not wired to a server (e.g. in fixtures that
// create a player without calling newTestServer + wiring).
func (p *Player) SetDelayed(ticks int) {
	if p.client == nil || p.client.server == nil {
		return
	}
	p.delayed = true
	p.delayedUntil = p.client.server.currentTick + 1 + ticks
}

// EnqueueScript appends a queued fresh-run request to the player's
// normal queue. Delay=0 fires on the next processActiveScripts pass.
func (p *Player) EnqueueScript(scriptID uint32, delay int, intArg int) {
	p.queue = append(p.queue, playerQueueRequest{
		ScriptID: scriptID,
		Delay:    delay,
		IntArg:   intArg,
	})
}

// StoreActiveScript saves a Suspended ScriptState so the tick loop can
// resume it when the player's delay expires.
func (p *Player) StoreActiveScript(state *script.ScriptState) {
	p.activeScript = state
}

// ClearActiveScript discards any stored ScriptState. Called after
// Finished/Aborted runs and on logout cleanup.
func (p *Player) ClearActiveScript() {
	p.activeScript = nil
}
```

- [ ] **Step 3: Build the full tree to confirm the interface is satisfied**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: build succeeds. The compile-time assertion `var _ script.ActivePlayer = (*Player)(nil)` in `modules/world/message_game.go` confirms all four new methods exist.

- [ ] **Step 4: Commit**

```bash
git add modules/world/player.go modules/world/player_script.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): Player.activeScript, queue, and ActivePlayer S4 methods

Adds activeScript (pending-resume state) and queue (pending fresh-run
requests) fields to Player. Implements SetDelayed, EnqueueScript,
StoreActiveScript, ClearActiveScript to satisfy the extended
script.ActivePlayer interface. SetDelayed computes resumeTick =
server.currentTick + 1 + ticks per TS semantics.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Split `runScript` into `runScript` + `resumeOrFinish`

**Files:**
- Modify: `modules/world/script.go`

- [ ] **Step 1: Replace the body of `modules/world/script.go` with the split version**

```go
package world

import (
	"github.com/zsrv/goscape/pkg/script"
)

// runScript initialises a ScriptState for a fresh invocation and routes
// the result via resumeOrFinish. Safe to call with a nil scriptFile
// (no-op) so callers don't have to nil-check the trigger lookup.
//
// If the script suspends (Execution == Suspended), the state is stored
// on the active player and the tick loop will resume it when the
// player's delay expires via processActiveScripts.
func (s *Server) runScript(sf *script.ScriptFile, self script.ActivePlayer, protect bool, intArgs []int, stringArgs []string) {
	if sf == nil {
		return
	}
	state := script.Init(sf, self, protect, intArgs, stringArgs)
	state.Provider = s.scriptProvider
	s.resumeOrFinish(state, self)
}

// resumeOrFinish is the shared post-Execute handler for both fresh runs
// (from runScript) and resumed runs (from processActiveScripts). It
// drives the state-store / state-clear decision in one place so the
// tick loop doesn't need to type-assert back to *Player.
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
		// CountDialog, PauseButton, NpcSuspended, WorldSuspended are
		// handled by later sub-specs; drop the state for now.
		s.log.Warn("script in unsupported execution state",
			"script", state.Script.Name, "execution", state.Execution)
		self.ClearActiveScript()
	}
}
```

- [ ] **Step 2: Run existing world tests to confirm S3 still works**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestRunScript'`
Expected: PASS (the existing three `TestRunScript*` tests still compile and pass; they don't exercise the new suspension path).

- [ ] **Step 3: Commit**

```bash
git add modules/world/script.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(world): split runScript into runScript + resumeOrFinish

Post-Execute handling now lives in one place used by both fresh runs
and resumption. Routes Suspended to StoreActiveScript instead of
warn+drop. Finished/Aborted/Error paths all call ClearActiveScript so
the player never retains a stale state across runs.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Add `processActiveScripts` tick phase

**Files:**
- Modify: `modules/world/tick.go`

- [ ] **Step 1: Add `processActiveScripts` and `processPlayerQueue` after `processPathing`**

Append the two new methods near the other `processX` methods in `modules/world/tick.go`:

```go
// processActiveScripts expires any elapsed delay, resumes suspended
// scripts, and fires ready queue entries. Runs between processClientsIn
// and processPathing so that a resumed or queued script that sets up
// movement has its movement applied this tick.
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
// firing ready entries as fresh script runs. Iterates by index so an
// entry appended mid-pass (via a fired script calling EnqueueScript
// again) is visible in the same iteration — this preserves TS's
// authentic "speedup quirk" where queue-chain reactions cascade.
//
// Removal happens BEFORE firing so a re-entrant EnqueueScript doesn't
// collide with the index pointer.
func (s *Server) processPlayerQueue(p *Player) {
	i := 0
	for i < len(p.queue) {
		req := &p.queue[i]
		req.Delay--
		if req.Delay > 0 || p.delayed {
			i++
			continue
		}
		scriptID := req.ScriptID
		intArg := req.IntArg
		p.queue = append(p.queue[:i], p.queue[i+1:]...)

		if s.scriptProvider != nil {
			if sf := s.scriptProvider.GetByLookupKey(scriptID); sf != nil {
				s.runScript(sf, p, false, []int{intArg}, nil)
			}
		}
		// Don't advance i: we just removed the current element, so i
		// now points to what was the next element (or past end).
	}
}
```

- [ ] **Step 2: Wire the phase into the tick loop**

In `runTickLoopWithRate`, insert `s.processActiveScripts()` between `s.processClientsIn()` and `s.processPathing()`:

```go
			s.processClientsIn()
			s.processActiveScripts()
			s.processPathing()
			s.processInteractions()
			s.processNpcs()
			s.processLogouts()
			s.processLogins()
			s.processInfo()
			s.processZones()       // compute ComputeShared before delivery
			s.processClientsOut()
			s.processCleanup()
			s.currentTick++
```

- [ ] **Step 3: Build the world package**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`
Expected: build succeeds.

- [ ] **Step 4: Run existing tests to confirm no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/tick.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): processActiveScripts tick phase for S4 suspension + queue

Inserted between processClientsIn and processPathing so resumed scripts
and fired queue entries see movement applied this tick. Expires
Player.delayed, resumes Player.activeScript via resumeOrFinish, then
walks the queue by index with remove-before-fire semantics to preserve
TS's authentic "speedup quirk" for cascading queue additions.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: End-to-end tests for suspension and queue

**Files:**
- Modify: `modules/world/script_test.go`

- [ ] **Step 1: Add suspension + queue tests**

Append to `modules/world/script_test.go` (after `TestRunScriptHandlesError`):

```go
// buildDelayScript returns a synthetic ScriptFile equivalent to:
//   mes "before"
//   p_delay 1
//   mes "after"
//   return
func buildDelayScript() *script.ScriptFile {
	return &script.ScriptFile{
		Name: "[delay,test]",
		Opcodes: []script.Opcode{
			script.OpPushConstantString, // push "before"
			script.OpMes,
			script.OpPushConstantInt, // push 1
			script.OpPDelay,
			script.OpPushConstantString, // push "after"
			script.OpMes,
			script.OpReturn,
		},
		IntOperands:      []int32{0, 0, 1, 0, 0, 0, 0},
		StringOperands:   []string{"before", "", "", "", "after", "", ""},
		InstructionCount: 7,
	}
}

// buildGreetScript is a tiny queued-target script that emits "g".
func buildGreetScript(key uint32) *script.ScriptFile {
	return &script.ScriptFile{
		Name:             "[greet,test]",
		LookupKey:        key,
		Opcodes:          []script.Opcode{script.OpPushConstantString, script.OpMes, script.OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"g", "", ""},
		InstructionCount: 3,
	}
}

func TestPDelayStoresActiveScript(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	startTick := s.currentTick

	sf := buildDelayScript()
	s.runScript(sf, p, true, nil, nil)

	if p.activeScript == nil {
		t.Fatal("activeScript: got nil, want non-nil")
	}
	if p.activeScript.Execution != script.Suspended {
		t.Errorf("Execution: got %v, want Suspended", p.activeScript.Execution)
	}
	if !p.delayed {
		t.Error("delayed: got false, want true")
	}
	if p.delayedUntil != startTick+2 {
		t.Errorf("delayedUntil: got %d, want %d", p.delayedUntil, startTick+2)
	}
}

func TestResumeAfterDelayExpires(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	s.runScript(buildDelayScript(), p, true, nil, nil)
	if p.activeScript == nil {
		t.Fatal("precondition: activeScript should be non-nil after suspension")
	}

	// Advance enough ticks for the delay to expire.
	s.currentTick += 3
	s.processActiveScripts()

	if p.activeScript != nil {
		t.Errorf("activeScript after expiry: got %v, want nil", p.activeScript)
	}
	if p.delayed {
		t.Error("delayed after expiry: got true, want false")
	}
}

func TestResumedScriptEmitsMessageGame(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	received := drainConn(t, cc)
	s.runScript(buildDelayScript(), p, true, nil, nil)
	p.client.flushWrite()
	first := <-received

	// First packet should be the "before" MessageGame.
	// Wire = opcode(1) + len(1) + PJStrLF("before") = 1+1+7 = 9 bytes.
	if len(first) != 9 {
		t.Fatalf("first packet: got %d bytes, want 9", len(first))
	}
	if string(first[2:8]) != "before" || first[8] != 0x0a {
		t.Errorf("first payload: got %q, want 'before\\n'", first[2:])
	}

	// Advance ticks and resume. Should emit the "after" MessageGame.
	received2 := drainConn(t, cc)
	s.currentTick += 3
	s.processActiveScripts()
	p.client.flushWrite()
	second := <-received2

	// Wire = opcode(1) + len(1) + PJStrLF("after") = 1+1+6 = 8 bytes.
	if len(second) != 8 {
		t.Fatalf("second packet: got %d bytes, want 8", len(second))
	}
	if string(second[2:7]) != "after" || second[7] != 0x0a {
		t.Errorf("second payload: got %q, want 'after\\n'", second[2:])
	}
}

func TestQueueFiresAtDelayExpiry(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	greet := buildGreetScript(0xAAAA)
	s.scriptProvider.Register(greet)

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	received := drainConn(t, cc)
	p.EnqueueScript(0xAAAA, 1, 0)

	// Tick 1: decrement from 1 -> 0, NOT yet ready (delay > 0 branch
	// keeps it). Wait — actually pre-decrement semantics: 1 -> 0, and
	// 0 <= 0 fires. So it SHOULD fire on the first pass.
	s.processActiveScripts()
	p.client.flushWrite()
	got := <-received

	// "g\n" wire = 1 + 1 + 2 = 4 bytes.
	if len(got) != 4 {
		t.Fatalf("queue fire: got %d bytes, want 4", len(got))
	}
	if string(got[2:3]) != "g" || got[3] != 0x0a {
		t.Errorf("queue payload: got %q, want 'g\\n'", got[2:])
	}
	if len(p.queue) != 0 {
		t.Errorf("queue after fire: len=%d, want 0", len(p.queue))
	}
}

func TestQueueZeroDelayFiresSameTick(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	greet := buildGreetScript(0xBBBB)
	s.scriptProvider.Register(greet)

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	received := drainConn(t, cc)
	p.EnqueueScript(0xBBBB, 0, 0)
	s.processActiveScripts()
	p.client.flushWrite()
	got := <-received

	if len(got) != 4 {
		t.Fatalf("zero-delay fire: got %d bytes, want 4", len(got))
	}
	if len(p.queue) != 0 {
		t.Errorf("queue not drained")
	}
}

func TestQueueMultipleEntriesPreservesOrder(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(&script.ScriptFile{
		Name:             "[g1]",
		LookupKey:        0xCCC1,
		Opcodes:          []script.Opcode{script.OpPushConstantString, script.OpMes, script.OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"1", "", ""},
		InstructionCount: 3,
	})
	s.scriptProvider.Register(&script.ScriptFile{
		Name:             "[g2]",
		LookupKey:        0xCCC2,
		Opcodes:          []script.Opcode{script.OpPushConstantString, script.OpMes, script.OpReturn},
		IntOperands:      []int32{0, 0, 0},
		StringOperands:   []string{"2", "", ""},
		InstructionCount: 3,
	})

	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	received := drainConn(t, cc)
	p.EnqueueScript(0xCCC1, 0, 0)
	p.EnqueueScript(0xCCC2, 0, 0)
	s.processActiveScripts()
	p.client.flushWrite()

	// Collect both packets. Each is opcode + len + 1-char + LF = 4 bytes.
	got := <-received
	// drainConn writes one buffer per read; both packets may coalesce
	// into a single Read depending on timing. Accept either.
	if len(got) == 4 {
		// First packet only; second is in the next drain.
		got2 := <-received
		if len(got2) != 4 {
			t.Fatalf("second packet: got %d, want 4", len(got2))
		}
		if got[2] != '1' || got2[2] != '2' {
			t.Errorf("order: got %c,%c; want 1,2", got[2], got2[2])
		}
	} else if len(got) == 8 {
		// Both coalesced.
		if got[2] != '1' || got[6] != '2' {
			t.Errorf("coalesced order: got %c,%c; want 1,2", got[2], got[6])
		}
	} else {
		t.Fatalf("unexpected packet length: %d", len(got))
	}
}
```

- [ ] **Step 2: Run all new tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestPDelay|TestResume|TestQueue' -v`
Expected: PASS for all six new tests.

- [ ] **Step 3: Run the full module test suite to confirm no regression**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: PASS.

- [ ] **Step 4: Run the full repository test suite with race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): S4 end-to-end tests for p_delay + queue

Covers:
- p_delay stores activeScript and sets delayedUntil to currentTick+2
  for p_delay(1)
- Delay expires and activeScript clears after advancing past
  delayedUntil
- Resumed script emits its post-p_delay MessageGame on the wire
- Queue with delay=1 fires on first processActiveScripts pass (pre-
  decrement semantics)
- Queue with delay=0 fires same tick
- Multiple queue entries fire in insertion order

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review Checklist

After completing all tasks, run:

- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...` — clean build
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` — all tests pass
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...` — no race warnings
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...` — no vet issues
- [ ] `CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape --config.verify --config.file config.yaml` — config verifies (optional sanity check)

The LOGIN script that emits "Welcome to RuneScape." continues to display, and any cache script containing `p_delay` or `queue` now runs to completion across ticks.
