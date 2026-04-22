# NAI-3 NPC Queue + `ai_queue{1..20}` Dispatch — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable `npc_queue` RuneScript opcode to enqueue `ai_queueN` trigger dispatches on an NPC, with tick-loop processing that mirrors TS's delay-decrement-only-when-not-delayed semantics and re-entrant-append "speedup quirk".

**Architecture:** `NpcQueueRequest` type in `pkg/script/queue.go` (co-located with `PlayerQueueType`). `queue []NpcQueueRequest` field on `*Npc`. Three-method `ActiveNpc` interface extension. `processNpcQueue` helper in `modules/world/npc_script.go` mirrors player-side `processPlayerQueue` with NPC-specific gating. `Npc.turn()` gets the queue pass inside the existing `!n.dead` block after the script-resume step. Folds in two NAI-2 follow-up tests for `resumeOrFinishNpc` defensive paths.

**Tech Stack:** Go 1.22+, `pkg/script` runtime, existing tick loop.

**Spec:** `docs/superpowers/specs/2026-04-22-nai-3-npc-queue-design.md`

**Roadmap:** `docs/superpowers/specs/2026-04-22-npc-ai-tick-decomposition-design.md`

**Memory (for context):** `nai_followups.md` — NAI-3 is the designated home for NAI-2's `resumeOrFinishNpc` error/default-branch test gaps.

---

## File Structure

**Modified:**
- `pkg/script/queue.go` — `+NpcQueueRequest` struct
- `pkg/script/active.go` — `+EnqueueScriptForTrigger` on `ActiveNpc`
- `pkg/script/handlers_npc.go` — `+handleNpcQueue`
- `pkg/script/handlers.go` — `+OpNpcQueue` registration
- `pkg/script/handlers_npc_test.go` — `+enqueueCalls` on `mockNpc` + 3 tests
- `pkg/script/handlers_player_test.go` — no-op stub on `mockActiveNpc`
- `modules/world/npc.go` — `+queue` field + `+EnqueueScriptForTrigger` method
- `modules/world/npc_script.go` — `+processNpcQueue` helper
- `modules/world/npc_ai.go` — `+processNpcQueue` call in `turn()` prefix
- `modules/world/npc_script_test.go` — 3 integration tests + 2 NAI-2 follow-up tests

Four tasks. Each task ends with a commit that leaves the tree green.

---

## Task 1: NpcQueueRequest type + ActiveNpc interface + *Npc field + method

**Files:**
- Modify: `pkg/script/queue.go` (append `NpcQueueRequest`)
- Modify: `pkg/script/active.go` (extend `ActiveNpc`)
- Modify: `pkg/script/handlers_npc_test.go` (add `enqueueCalls` + stub method to `mockNpc`)
- Modify: `pkg/script/handlers_player_test.go` (add no-op stub to `mockActiveNpc`)
- Modify: `modules/world/npc.go` (add field + method)
- Modify: `modules/world/npc_script_test.go` (add one unit test)

- [ ] **Step 1: Write the failing test**

Append to `modules/world/npc_script_test.go`:

```go
func TestNpcEnqueueScriptForTrigger(t *testing.T) {
	n := newNpcForScriptTest(t)

	n.EnqueueScriptForTrigger(script.TriggerAiQueue3, 5, 42)

	if len(n.queue) != 1 {
		t.Fatalf("queue len: got %d, want 1", len(n.queue))
	}
	req := n.queue[0]
	if req.Trigger != script.TriggerAiQueue3 {
		t.Errorf("Trigger: got %v, want TriggerAiQueue3", req.Trigger)
	}
	if req.Delay != 5 {
		t.Errorf("Delay: got %d, want 5", req.Delay)
	}
	if req.IntArg != 42 {
		t.Errorf("IntArg: got %d, want 42", req.IntArg)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcEnqueueScriptForTrigger -v
```

Expected: compile error — `n.EnqueueScriptForTrigger undefined`, or `n.queue undefined`, or `script.NpcQueueRequest undefined`.

- [ ] **Step 3: Add `NpcQueueRequest` to pkg/script/queue.go**

Append to `pkg/script/queue.go`:

```go
// NpcQueueRequest is an NPC-side enqueue entry. Unlike
// PlayerQueueRequest, it has no queue-type distinction — TS's NPC
// queue has no strong/weak/long variants. The Trigger is one of
// TriggerAiQueue1..TriggerAiQueue20 and identifies which script runs
// at fire time (resolved via scriptProvider.GetByTrigger on the
// NPC's type + category). Matches TS NpcQueueRequest at
// Engine-TS/src/engine/entity/NpcQueueRequest.ts.
type NpcQueueRequest struct {
	Trigger ServerTriggerType
	Delay   int
	IntArg  int
}
```

- [ ] **Step 4: Extend `ActiveNpc` interface in pkg/script/active.go**

In `pkg/script/active.go`, locate the `ActiveNpc` interface. After the three NAI-2 lifecycle methods (`StoreActiveScript`, `ClearActiveScript`, `SetDelayed`), append just before the closing `}`:

```go
	// EnqueueScriptForTrigger appends a queued ai_queueN dispatch to
	// the NPC. Matches TS Npc.enqueueScript at Npc.ts:241-245 — the
	// trigger (TriggerAiQueue1..TriggerAiQueue20) identifies which
	// script runs; lookup happens at fire time via
	// scriptProvider.GetByTrigger keyed on the NPC's type + category.
	EnqueueScriptForTrigger(trigger ServerTriggerType, delay int, intArg int)
```

- [ ] **Step 5: Add `queue` field + `EnqueueScriptForTrigger` method on `*Npc`**

In `modules/world/npc.go`, locate the `// === script state ===` block. Add a new field:

```go
	// === script state ===
	server       *Server          // back-reference; set by Server.addNpc
	activeScript *script.ScriptState
	delayed      bool
	delayedUntil int
	queue        []script.NpcQueueRequest
```

At the bottom of `modules/world/npc.go` (after `SetDelayed`), append:

```go
// EnqueueScriptForTrigger appends a queued ai_queueN dispatch.
// Implements script.ActiveNpc.EnqueueScriptForTrigger. Script
// resolution is deferred to fire time via
// scriptProvider.GetByTrigger — matches TS Npc.enqueueScript.
func (n *Npc) EnqueueScriptForTrigger(trigger script.ServerTriggerType, delay, intArg int) {
	n.queue = append(n.queue, script.NpcQueueRequest{
		Trigger: trigger,
		Delay:   delay,
		IntArg:  intArg,
	})
}
```

- [ ] **Step 6: Update mocks to satisfy extended ActiveNpc interface**

In `pkg/script/handlers_npc_test.go`, locate the `mockNpc` struct definition (around line 13). Add a new field after `damageCalls`:

```go
	damageCalls                        []struct{ amount, dmgType int }
	enqueueCalls                       []mockEnqueueCall
```

And add a type definition just before the `mockNpc` struct:

```go
type mockEnqueueCall struct {
	trigger ServerTriggerType
	delay   int
	intArg  int
}
```

Locate the three NAI-2 stubs at lines 77-79:

```go
func (m *mockNpc) StoreActiveScript(_ *ScriptState) {}
func (m *mockNpc) ClearActiveScript()               {}
func (m *mockNpc) SetDelayed(_ int)                 {}
```

Add a fourth method immediately after:

```go
func (m *mockNpc) EnqueueScriptForTrigger(trigger ServerTriggerType, delay, intArg int) {
	m.enqueueCalls = append(m.enqueueCalls, mockEnqueueCall{
		trigger: trigger,
		delay:   delay,
		intArg:  intArg,
	})
}
```

In `pkg/script/handlers_player_test.go`, locate the `mockActiveNpc` type (it has three NAI-2 no-op stubs). Add a fourth no-op stub:

```go
func (m *mockActiveNpc) EnqueueScriptForTrigger(_ ServerTriggerType, _, _ int) {}
```

- [ ] **Step 7: Run tests to verify passing**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcEnqueueScriptForTrigger -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
```

Expected: clean build; `TestNpcEnqueueScriptForTrigger` PASS; all `pkg/script/` and `modules/world/` existing tests PASS with zero regressions.

- [ ] **Step 8: Commit**

```bash
git add pkg/script/queue.go pkg/script/active.go pkg/script/handlers_npc_test.go pkg/script/handlers_player_test.go modules/world/npc.go modules/world/npc_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world,script): NAI-3 NpcQueueRequest + EnqueueScriptForTrigger

Add NpcQueueRequest struct to pkg/script/queue.go (Trigger, Delay,
IntArg — no queue-type variant since TS NPC queue has no strong/weak/
long distinction). Extend ActiveNpc interface with
EnqueueScriptForTrigger. Add queue []script.NpcQueueRequest field on
*Npc + EnqueueScriptForTrigger method. Update mockNpc + mockActiveNpc
to satisfy the extended interface.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: handleNpcQueue opcode handler + 3 handler tests

**Files:**
- Modify: `pkg/script/handlers_npc.go` (add `handleNpcQueue`)
- Modify: `pkg/script/handlers.go` (register `OpNpcQueue: handleNpcQueue`)
- Modify: `pkg/script/handlers_npc_test.go` (add 3 tests)

- [ ] **Step 1: Write the three failing tests**

Append to `pkg/script/handlers_npc_test.go`:

```go
// TestHandleNpcQueueEnqueues — NPC_QUEUE pops (delay, arg, queueID)
// in that order (top of stack = delay) and maps queueID (1-20) to
// TriggerAiQueue1 + queueID - 1.
func TestHandleNpcQueueEnqueues(t *testing.T) {
	npc := &mockNpc{}
	sf := &ScriptFile{
		Name: "test_npc_queue",
		Opcodes: []Opcode{
			OpPushConstantInt, // push queueID (3)
			OpPushConstantInt, // push arg (42)
			OpPushConstantInt, // push delay (5)
			OpNpcQueue,
			OpReturn,
		},
		IntOperands: []int32{3, 42, 5, 0, 0},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(npc.enqueueCalls) != 1 {
		t.Fatalf("enqueueCalls: got %d, want 1", len(npc.enqueueCalls))
	}
	call := npc.enqueueCalls[0]
	if call.trigger != TriggerAiQueue3 {
		t.Errorf("trigger: got %v, want TriggerAiQueue3", call.trigger)
	}
	if call.delay != 5 {
		t.Errorf("delay: got %d, want 5", call.delay)
	}
	if call.intArg != 42 {
		t.Errorf("intArg: got %d, want 42", call.intArg)
	}
}

// TestHandleNpcQueueWithoutActiveNpcErrors — defensive nil check.
func TestHandleNpcQueueWithoutActiveNpcErrors(t *testing.T) {
	sf := &ScriptFile{
		Name: "npc_queue_no_npc",
		Opcodes: []Opcode{
			OpPushConstantInt, OpPushConstantInt, OpPushConstantInt,
			OpNpcQueue, OpReturn,
		},
		IntOperands: []int32{1, 0, 0, 0, 0},
	}
	state := Init(sf, nil, false, nil, nil)
	// state.ActiveNpc intentionally nil.

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error, got nil")
	}
	want := "NPC_QUEUE: no active npc"
	if got := err.Error(); got != want {
		t.Errorf("error: got %q, want %q", got, want)
	}
}

// TestHandleNpcQueueInvalidQueueIDErrors — queueID out of [1,20].
func TestHandleNpcQueueInvalidQueueIDErrors(t *testing.T) {
	cases := []struct {
		name    string
		queueID int32
		wantErr string
	}{
		{"zero", 0, "NPC_QUEUE: invalid queueId 0 (want 1..20)"},
		{"twentyone", 21, "NPC_QUEUE: invalid queueId 21 (want 1..20)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			npc := &mockNpc{}
			sf := &ScriptFile{
				Name: "npc_queue_invalid_id",
				Opcodes: []Opcode{
					OpPushConstantInt, // queueID
					OpPushConstantInt, // arg
					OpPushConstantInt, // delay
					OpNpcQueue,
					OpReturn,
				},
				IntOperands: []int32{tc.queueID, 0, 0, 0, 0},
			}
			state := Init(sf, nil, false, nil, nil)
			state.ActiveNpc = npc
			state.Pointers |= PtrActiveNpc

			err := Execute(state)
			if err == nil {
				t.Fatalf("Execute: want error, got nil")
			}
			if got := err.Error(); got != tc.wantErr {
				t.Errorf("error: got %q, want %q", got, tc.wantErr)
			}
			if len(npc.enqueueCalls) != 0 {
				t.Errorf("enqueueCalls: got %d, want 0 (enqueue must not fire on invalid id)", len(npc.enqueueCalls))
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleNpcQueue -v
```

Expected: tests FAIL because `OpNpcQueue` has no registered handler. Error shape: `script "...": no handler for NPC_QUEUE (opcode 2530) at pc=3` or similar.

- [ ] **Step 3: Add `handleNpcQueue` to `pkg/script/handlers_npc.go`**

Append (after `handleNpcDelay`):

```go
// handleNpcQueue (NPC_QUEUE, opcode 2530) enqueues an ai_queueN
// dispatch on the active NPC. Pop order: delay (top), arg, queueId
// (bottom). queueId ∈ [1, 20] maps to TriggerAiQueue1..20 via
// arithmetic: trigger = TriggerAiQueue1 + queueId - 1. Mirrors TS
// NpcOps.ts:144-150.
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

- [ ] **Step 4: Register the handler**

In `pkg/script/handlers.go`, locate the NPC-mutating-ops block that contains `OpNpcDelay: handleNpcDelay,` (added in NAI-2). Add `OpNpcQueue: handleNpcQueue,` in alphabetical position. Ordering: `OpNpcDelay` (D) → `OpNpcFaceSquare` (F)  → ... → `OpNpcQueue` (Q) would slot between `OpNpcName` or similar and `OpNpcRange`. The exact alphabetical position depends on the registered siblings; `gofmt` will realign column widths.

If unsure about alphabetical position, place the new line immediately after any existing `OpNpc*` entry — the lint check in Task 4 will catch misalignment via test output if any.

- [ ] **Step 5: Run tests to verify they pass**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleNpcQueue -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -count=1
```

Expected: all three new tests PASS (including the 2 subtests of `TestHandleNpcQueueInvalidQueueIDErrors`); no regressions in the package.

- [ ] **Step 6: Commit**

```bash
git add pkg/script/handlers_npc.go pkg/script/handlers.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-3 handleNpcQueue (NPC_QUEUE, opcode 2530)

Pop (delay, arg, queueId) — queueId ∈ [1,20] maps to TriggerAiQueue1..20
via TriggerAiQueue1 + queueId - 1. Defensive checks: requireActiveNpc
gate, and invalid-queueId error with explicit range. Matches TS
NpcOps.ts:144-150. No NumberNotNull check on delay — tracked as
future fidelity-audit item in nai_followups memory.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: processNpcQueue helper + turn() wire + integration tests

**Files:**
- Modify: `modules/world/npc_script.go` (add `processNpcQueue`)
- Modify: `modules/world/npc_ai.go` (wire call in `turn()`)
- Modify: `modules/world/npc_script_test.go` (add 3 integration tests)

- [ ] **Step 1: Write the three failing tests**

Append to `modules/world/npc_script_test.go`:

```go
// buildNpcForIntegration builds an NPC wired to a server, with typ
// set so processNpcQueue can read n.typ.Category.
func buildNpcForIntegration(t *testing.T) (*Server, *Npc) {
	t.Helper()
	s := newServerForScriptTest(t)
	s.currentTick = 100
	typ := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 0, DebugName: "test_npc"},
		Category:   -1,
	}
	n := NewNpc(1, 0, 3094, 3106, 0, typ)
	n.server = s
	return s, n
}

// registerTrivialAiQueueScript registers a do-nothing script for a
// TriggerAiQueueN on the NPC's type, via the scriptProvider's
// RegisterTrigger (or equivalent). Returns a pointer to a counter
// that increments each time the script fires.
//
// The script body is [OpReturn], so it runs to Finished immediately.
//
// NB: `scriptProvider` must be seeded on `s` — call this AFTER
// buildNpcForIntegration.
func registerTrivialAiQueueScript(t *testing.T, s *Server, trigger script.ServerTriggerType, typeID int) *int {
	t.Helper()
	counter := 0
	// Sentinel script that counts fires via a closure-captured
	// counter by side-effecting through a provider hook. Simplest
	// approach: use a script body of just OpReturn and let the
	// wrapping runNpcScript log do the accounting; but we need the
	// counter for assertions. Use a custom ScriptFile registered
	// directly on a minimal provider.
	//
	// For NAI-3, the cleanest fixture is:
	//   - Create a bare *script.Provider
	//   - Register a sentinel ScriptFile for the trigger
	//   - The ScriptFile's Opcodes are [OpReturn] so it finishes
	//     immediately on fire
	//   - Assert fires by checking n.queue length + downstream side
	//     effects (e.g. that Execute was called)
	//
	// Since we don't have a simple way to spy on fires through the
	// provider, these tests will instead assert on observable
	// side-effects: n.queue shrinks after fire, n.activeScript stays
	// nil (script finished immediately), and we can use a sentinel
	// mutation op to prove the script ran.

	// For this plan, use the counter approach via a mock provider
	// helper. If `script.NewProvider` doesn't support direct file
	// registration without cache bytes, skip the counter and rely
	// on queue-shrinkage assertions.
	_ = counter
	return &counter
}
```

**NB to implementer:** the helper above is illustrative. The actual implementation of `registerTrivialAiQueueScript` depends on whether `*script.Provider` exposes a `RegisterScript(trigger ServerTriggerType, typeID, catID int, sf *ScriptFile)` method or similar. If no such method exists, INSTEAD of the closure-counter fixture, use one of these two fallback fixtures — whichever compiles most easily on the first try:

**Fallback A (provider test-hook)** — if `*script.Provider` has a public `RegisterForTest` or `triggerMap` field exposed via test-only export:

```go
func registerTrivialAiQueueScript(t *testing.T, s *Server, trigger script.ServerTriggerType, typeID int) {
	sf := &script.ScriptFile{
		Name:    "trivial_ai_queue",
		Opcodes: []script.Opcode{script.OpReturn},
	}
	if s.scriptProvider == nil {
		s.scriptProvider = script.NewProvider()
	}
	// Use whatever test-only registration path the provider exposes.
	// If no such path, use Fallback B.
	s.scriptProvider.RegisterForTest(trigger, typeID, -1, sf)
}
```

**Fallback B (provider nil, assert queue-only)** — if no registration hook exists, test only the queue-side observable behavior:

```go
// Leave s.scriptProvider nil. processNpcQueue will short-circuit on
// the GetByTrigger call (nil provider) and remove the queue entry
// without firing anything. Tests assert that queue shrinks on
// delay-zero ticks but NOT while delayed.
```

Before writing the tests, grep for `RegisterForTest`, `triggerMap`, or similar test hooks in `pkg/script/provider*.go` and pick the fixture that works. Report which fallback was used.

Now the three actual tests — these work with EITHER fallback (assertions focus on queue mutation, not script side-effects):

```go
// TestNpcTurnFiresQueuedEntryWhenDelayZero — enqueue at delay=1,
// advance two ticks, assert queue drains.
func TestNpcTurnFiresQueuedEntryWhenDelayZero(t *testing.T) {
	s, n := buildNpcForIntegration(t)

	n.EnqueueScriptForTrigger(script.TriggerAiQueue1, 1, 0)
	if len(n.queue) != 1 {
		t.Fatalf("setup: queue should have 1 entry, got %d", len(n.queue))
	}

	// Tick 1: decrements to 0, does not fire yet (TS: fires when delay <= 0).
	n.turn(s)
	// Wait — TS fires when delay <= 0 AFTER decrement. So at delay=1 → 0,
	// the same tick that decrements also fires. Queue should be empty.
	if len(n.queue) != 0 {
		t.Fatalf("after first turn: queue should be empty (delay went 1→0 and fired), got %d", len(n.queue))
	}
}

// TestNpcTurnDoesNotDecrementQueueWhileDelayed — NPC delayed; queue
// delay must not decrement.
func TestNpcTurnDoesNotDecrementQueueWhileDelayed(t *testing.T) {
	s, n := buildNpcForIntegration(t)

	n.EnqueueScriptForTrigger(script.TriggerAiQueue1, 3, 0)
	n.delayed = true
	n.delayedUntil = s.currentTick + 100 // far future

	n.turn(s)
	n.turn(s)
	n.turn(s)

	if len(n.queue) != 1 {
		t.Fatalf("queue len: got %d, want 1 (no fire while delayed)", len(n.queue))
	}
	if n.queue[0].Delay != 3 {
		t.Errorf("queue[0].Delay: got %d, want 3 (no decrement while delayed)", n.queue[0].Delay)
	}
}

// TestNpcTurnReentryQueueAppendDuringIteration — if a fired script
// enqueues another entry, that entry is visible to the same
// processNpcQueue pass (TS "speedup quirk"). Without a fixture that
// spies on fires, this test is limited to proving the append-during-
// iteration mechanism via direct mutation of n.queue within a scope.
//
// Approach: use a hand-built helper that inserts an entry mid-pass,
// bypassing the script runner. This tests the iteration-robustness
// not the trigger-fire behavior.
func TestNpcTurnReentryQueueAppendDuringIteration(t *testing.T) {
	s, n := buildNpcForIntegration(t)

	// Two entries, both ready (delay=0). The iteration should
	// process both in one turn() call.
	n.EnqueueScriptForTrigger(script.TriggerAiQueue1, 0, 0)
	n.EnqueueScriptForTrigger(script.TriggerAiQueue2, 0, 0)

	n.turn(s)

	if len(n.queue) != 0 {
		t.Errorf("queue len: got %d, want 0 (both entries should fire in one pass)", len(n.queue))
	}
}
```

**NB to implementer:** The re-entrant append test as spec'd is WEAKER than the spec described — it only proves "multiple ready entries fire in one pass" rather than "a script that enqueues during its own fire triggers the newly-appended entry in the same iteration." The stronger test requires a fixture that hooks into script execution to append mid-fire. If Fallback A (provider hook) allows registering a real script that calls `EnqueueScriptForTrigger` via a test-NPC method, implement the stronger version. Otherwise, the weaker version above is acceptable for NAI-3 and can be strengthened in a future audit pass.

- [ ] **Step 2: Run tests to verify they fail**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNpcTurn(Fires|DoesNotDecrement|Reentry)' -v
```

Expected: all three tests FAIL because `n.turn(s)` doesn't call `processNpcQueue` yet. `TestNpcTurnFiresQueuedEntryWhenDelayZero` fails on the queue-empty assertion; others may pass spuriously.

- [ ] **Step 3: Add `processNpcQueue` to `modules/world/npc_script.go`**

Append at bottom:

```go
// processNpcQueue walks the NPC's queue, decrementing delays and
// firing ready entries as fresh NPC-anchored script runs. Iterates
// by index so a request appended mid-pass (via a fired script calling
// EnqueueScriptForTrigger again) is visible in the same iteration —
// preserves TS's "speedup quirk" at Npc.ts:538-560.
//
// Delay only decrements when the NPC is not delayed (TS Npc.ts:544-547
// "purposely only decrements the delay when the npc is not delayed").
// Removal happens BEFORE firing so a re-entrant enqueue doesn't
// collide with the index pointer. Matches the player-side pattern at
// modules/world/tick.go:219-242.
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
		// Don't advance i — removed current element.
	}
}
```

- [ ] **Step 4: Wire `processNpcQueue` call in `Npc.turn()`**

In `modules/world/npc_ai.go`, locate the `!n.dead` prefix block added in NAI-2:

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
	}
```

Add the queue-pass call immediately after the resume block, still inside the `!n.dead` guard:

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

- [ ] **Step 5: Run tests to verify they pass**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNpcTurn' -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
```

Expected: all three new tests PASS; all prior NAI-2 `TestNpcTurn*` tests still PASS (the queue pass is a no-op when `n.queue` is empty, which is the default state for every NAI-2 test fixture).

- [ ] **Step 6: Commit**

```bash
git add modules/world/npc_script.go modules/world/npc_ai.go modules/world/npc_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-3 processNpcQueue + Npc.turn() queue pass

Add processNpcQueue helper: index-based iteration preserves TS's
"speedup quirk" (re-entrant append during iteration visible in the
same pass). Delay decrements ONLY when !n.delayed, matching TS
Npc.ts:544-547. Removal-before-fire prevents index collision with
re-entrant enqueue. Wire the call inside the !n.dead prefix block of
Npc.turn(), after the script-resume step — matches TS Npc.ts:180
ordering. Three integration tests: fire-at-delay-zero, no-decrement-
while-delayed, and multiple-ready-in-one-pass (weaker form of the
re-entrant-append quirk — strengthening requires a script-fire
fixture out of NAI-3 scope).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: NAI-2 follow-up tests — closes NAI-3

**Files:**
- Modify: `modules/world/npc_script_test.go` (add 2 tests)

These close the NAI-2 `resumeOrFinishNpc` test gaps documented in
`nai_followups.md`.

- [ ] **Step 1: Write the two failing tests**

Append to `modules/world/npc_script_test.go`:

```go
// TestResumeOrFinishNpcErrorPathClearsScript — NAI-2 follow-up.
// When script.Execute returns an error, resumeOrFinishNpc must
// clear n.activeScript (matching the player-side resumeOrFinish
// error-path at modules/world/script.go:31-35).
func TestResumeOrFinishNpcErrorPathClearsScript(t *testing.T) {
	s, n := buildNpcForIntegration(t)

	// Pre-store a dummy script to prove it gets cleared.
	n.activeScript = &script.ScriptState{}

	// Build a state whose Execute will error. Opcode 0xFFFF has no
	// registered handler; Execute returns "no handler for ..." error.
	sf := &script.ScriptFile{
		Name:    "err_script",
		Opcodes: []script.Opcode{script.Opcode(0xFFFF)},
	}
	errState := script.Init(sf, nil, false, nil, nil)
	errState.ActiveNpc = n

	s.resumeOrFinishNpc(errState, n)

	if n.activeScript != nil {
		t.Errorf("activeScript: got %v, want nil (cleared on Execute error)", n.activeScript)
	}
}

// TestResumeOrFinishNpcDefaultBranchClearsScript — NAI-2 follow-up.
// Synthetic: pre-set Execution to a value that hits the default:
// branch (not Finished/Aborted/NpcSuspended). Execute's hot loop
// exits immediately when Execution != Running, so the pre-set value
// survives untouched.
//
// This path is unreachable from authentic content (all non-
// NpcSuspended non-terminal Execution values require an ActivePlayer,
// and runNpcScript passes nil Self), but the test proves the
// defensive clear fires if future code accidentally drives an NPC-
// anchored script into one of these states.
func TestResumeOrFinishNpcDefaultBranchClearsScript(t *testing.T) {
	s, n := buildNpcForIntegration(t)

	n.activeScript = &script.ScriptState{}

	sf := &script.ScriptFile{
		Name:    "default_branch_script",
		Opcodes: []script.Opcode{script.OpReturn},
	}
	state := script.Init(sf, nil, false, nil, nil)
	state.ActiveNpc = n
	state.Execution = script.CountDialog // synthetic non-Running, non-terminal state

	s.resumeOrFinishNpc(state, n)

	if n.activeScript != nil {
		t.Errorf("activeScript: got %v, want nil (cleared on default branch)", n.activeScript)
	}
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestResumeOrFinishNpc -v
```

Expected: both tests PASS on first run. The implementation being tested (resumeOrFinishNpc) already landed in NAI-2; these tests merely exercise previously-uncovered branches. No code changes needed for pass.

If either test FAILS, it indicates a real bug in NAI-2's `resumeOrFinishNpc` — the branch-to-clear behavior isn't working as documented. In that case: stop, diagnose, report.

- [ ] **Step 3: Run full race-enabled suite**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: full repo PASS with race detector.

- [ ] **Step 4: Grep sanity check**

Run:
```
rg -n '\bNpcQueueRequest\b|\bEnqueueScriptForTrigger\b|\bprocessNpcQueue\b' modules/world/ pkg/script/
```

Expected: matches appear in `pkg/script/queue.go`, `pkg/script/active.go`, `pkg/script/handlers_npc.go`, `pkg/script/handlers_npc_test.go`, `pkg/script/handlers_player_test.go`, `modules/world/npc.go`, `modules/world/npc_script.go`, `modules/world/npc_script_test.go`, `modules/world/npc_ai.go`. No stray references.

- [ ] **Step 5: Commit, closing NAI-3**

```bash
git add modules/world/npc_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): NAI-3 resumeOrFinishNpc defensive-branch tests — closes NAI-3

Add two tests closing NAI-2 follow-ups tracked in nai_followups memory:

1. TestResumeOrFinishNpcErrorPathClearsScript — drives Execute to
   error via an unregistered opcode, asserts n.activeScript is
   cleared.
2. TestResumeOrFinishNpcDefaultBranchClearsScript — synthetic test
   pre-setting Execution=CountDialog before resumeOrFinishNpc
   dispatch, asserts default-branch clear. Unreachable from authentic
   content but proves defensive behavior.

Closes NAI-3 (NPC queue + ai_queue{1..20} dispatch) and resolves the
NAI-2 whole-impl reviewer's test-coverage gap.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-review checklist results

**1. Spec coverage:**

| Spec section | Task |
|---|---|
| `NpcQueueRequest` type in `pkg/script/queue.go` | Task 1 |
| `ActiveNpc.EnqueueScriptForTrigger` interface method | Task 1 |
| `*Npc.queue` field + `EnqueueScriptForTrigger` method | Task 1 |
| Mock updates (mockNpc + mockActiveNpc) | Task 1 |
| `handleNpcQueue` handler + registration | Task 2 |
| `processNpcQueue` helper | Task 3 |
| `Npc.turn()` queue-pass wiring | Task 3 |
| Test: `TestNpcEnqueueScriptForTrigger` | Task 1 |
| Test: `TestHandleNpcQueueEnqueues` | Task 2 |
| Test: `TestHandleNpcQueueWithoutActiveNpcErrors` | Task 2 |
| Test: `TestHandleNpcQueueInvalidQueueIDErrors` | Task 2 |
| Test: `TestNpcTurnFiresQueuedEntryWhenDelayZero` | Task 3 |
| Test: `TestNpcTurnDoesNotDecrementQueueWhileDelayed` | Task 3 |
| Test: `TestNpcTurnReentryQueueAppendDuringIteration` | Task 3 (weaker form — noted) |
| Test: `TestResumeOrFinishNpcErrorPathClearsScript` (NAI-2 follow-up) | Task 4 |
| Test: `TestResumeOrFinishNpcDefaultBranchClearsScript` (NAI-2 follow-up) | Task 4 |

All 9 spec tests have tasks. One test (re-entrant append) ships a weaker form because strengthening requires a fixture out of scope.

**2. Placeholder scan:** The `registerTrivialAiQueueScript` helper in Task 3 has multiple fallback paths depending on what test-hook the existing `*script.Provider` exposes. This is not a plan failure — it's an explicit contingency tree. All actual test code is concrete; the fixture helper is a guided decision point for the implementer.

**3. Type consistency:** `NpcQueueRequest` (exported), `EnqueueScriptForTrigger` (exported method + interface), `processNpcQueue` (unexported), `ServerTriggerType` (existing), `TriggerAiQueue1..20` (existing) — all match across Tasks 1-4.

---

## Commit trail (for reference)

Four commits close NAI-3:

1. `feat(world,script): NAI-3 NpcQueueRequest + EnqueueScriptForTrigger`
2. `feat(script): NAI-3 handleNpcQueue (NPC_QUEUE, opcode 2530)`
3. `feat(world): NAI-3 processNpcQueue + Npc.turn() queue pass`
4. `test(world): NAI-3 resumeOrFinishNpc defensive-branch tests — closes NAI-3`

Each commit leaves the tree green; the final one closes NAI-3 and resolves two NAI-2 follow-up items.
