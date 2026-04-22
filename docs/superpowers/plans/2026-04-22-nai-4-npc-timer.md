# NAI-4 NPC Timer + `ai_timer` Trigger — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the NPC-side single-slot timer (`timerInterval`/`timerClock` fields on `*Npc`, `SetTimer` method, `processNpcTimer` tick-pass, `OpNpcSetTimer` opcode) so NPCs whose type has a non-zero `Timer` field fire their `ai_timer` trigger script every `interval` ticks.

**Architecture:** Mirror NAI-3's queue-pass layout but simpler (single slot, no per-entry delay). Fields live in `*Npc.{timerInterval,timerClock}` seeded from `NpcType.Timer`. `SetTimer` on `*Npc` implements `ActiveNpc.SetTimer` with a TS-faithful `-1` no-op guard. `processNpcTimer` helper in `npc_script.go` gates on `!n.delayed` internally (Go lacks TS's `isValid` early-return in `turn()`), increments `timerClock`, and fires + resets only on successful script dispatch. Wire call goes between resume and queue in `Npc.turn()` per TS order.

**Tech Stack:** Go 1.26+, existing `pkg/script` runtime, existing tick loop.

**Spec:** `docs/superpowers/specs/2026-04-22-nai-4-npc-timer-design.md`

**Roadmap:** `docs/superpowers/specs/2026-04-22-npc-ai-tick-decomposition-design.md`

---

## File Structure

**Modified:**
- `pkg/script/active.go` — `+SetTimer(interval int)` on `ActiveNpc`
- `pkg/script/handlers_npc.go` — `+handleNpcSetTimer`
- `pkg/script/handlers.go` — `+OpNpcSetTimer` registration
- `pkg/script/handlers_npc_test.go` — `+setTimerCalls []int` on `mockNpc`, `+SetTimer` recording method, 2 handler tests
- `pkg/script/handlers_player_test.go` — `+SetTimer` no-op stub on `mockActiveNpc`
- `modules/world/npc.go` — `+timerInterval`, `+timerClock` fields; `+SetTimer` method; `NewNpc` seeds `timerInterval = int(typ.Timer)`
- `modules/world/npc_script.go` — `+processNpcTimer` helper
- `modules/world/npc_ai.go` — `+s.processNpcTimer(n)` call inside existing `!n.dead` block, BEFORE `processNpcQueue`
- `modules/world/npc_script_test.go` — 3 tests (2 unit: SetTimer/NewNpc, 1 integration: delayed-gate)

Three tasks. Each task ends with a commit.

---

## Task 1: Npc fields + SetTimer method + ActiveNpc interface + NewNpc seeding + mock updates

**Files:**
- Modify: `pkg/script/active.go` (extend `ActiveNpc` with `SetTimer`)
- Modify: `pkg/script/handlers_npc_test.go` (add `setTimerCalls` + recording stub)
- Modify: `pkg/script/handlers_player_test.go` (add no-op stub)
- Modify: `modules/world/npc.go` (add fields + method + NewNpc seeding)
- Modify: `modules/world/npc_script_test.go` (add 2 unit tests)

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/npc_script_test.go`:

```go
func TestNpcSetTimer(t *testing.T) {
	n := newNpcForScriptTest(t)

	n.SetTimer(5)
	if n.timerInterval != 5 {
		t.Errorf("timerInterval after SetTimer(5): got %d, want 5", n.timerInterval)
	}

	// -1 is a TS-faithful no-op: must leave timerInterval at 5.
	n.SetTimer(-1)
	if n.timerInterval != 5 {
		t.Errorf("timerInterval after SetTimer(-1): got %d, want 5 (no-op expected)", n.timerInterval)
	}
}

func TestNewNpcSeedsTimerIntervalFromType(t *testing.T) {
	typ := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 0, DebugName: "test_npc"},
		Timer:      7,
	}
	n := NewNpc(1, 0, 3094, 3106, 0, typ)

	if n.timerInterval != 7 {
		t.Errorf("timerInterval from NewNpc: got %d, want 7 (seeded from typ.Timer)", n.timerInterval)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNpcSetTimer|TestNewNpcSeedsTimerInterval' -v
```

Expected: compile errors — `n.SetTimer undefined`, `n.timerInterval undefined`.

- [ ] **Step 3: Extend `ActiveNpc` interface**

In `pkg/script/active.go`, locate the `ActiveNpc` interface. After `EnqueueScriptForTrigger` (added in NAI-3), append just before the closing `}`:

```go
	// SetTimer sets the tick interval between ai_timer trigger fires
	// on the active NPC. interval == -1 is a silent no-op, matching
	// TS Npc.setTimer at Engine-TS/.../Npc.ts:210-214. Called by the
	// NPC_SETTIMER opcode.
	SetTimer(interval int)
```

- [ ] **Step 4: Add `timerInterval` / `timerClock` fields and `SetTimer` method on `*Npc`**

In `modules/world/npc.go`, locate the `// === script state ===` block. Current contents:

```go
	// === script state ===
	server       *Server          // back-reference; set by Server.addNpc
	activeScript *script.ScriptState
	delayed      bool
	delayedUntil int
	queue        []script.NpcQueueRequest
```

Add the two new fields at the bottom of the block:

```go
	// === script state ===
	server        *Server          // back-reference; set by Server.addNpc
	activeScript  *script.ScriptState
	delayed       bool
	delayedUntil  int
	queue         []script.NpcQueueRequest
	timerInterval int
	timerClock    int
```

(Column alignment may shift as gofmt re-pads — let gofmt win.)

Seed `timerInterval` in `NewNpc`. The current `NewNpc` return has a large struct literal (around lines 96-135). Locate the existing entry for `respawnRate: int(typ.RespawnRate),` and add a companion line right after it:

```go
		respawnRate:     int(typ.RespawnRate),
		timerInterval:   int(typ.Timer),
```

(The position is a stylistic choice — grouping with the other `typ.*`-sourced fields reads best. Let gofmt realign column widths.)

Append the `SetTimer` method at the bottom of `modules/world/npc.go`, AFTER `EnqueueScriptForTrigger` (added in NAI-3):

```go
// SetTimer sets the tick interval between ai_timer trigger fires.
// interval == -1 is a silent no-op, matching TS Npc.setTimer at
// Engine-TS/.../Npc.ts:210-214. Implements script.ActiveNpc.SetTimer.
func (n *Npc) SetTimer(interval int) {
	if interval == -1 {
		return
	}
	n.timerInterval = interval
}
```

- [ ] **Step 5: Update mocks to satisfy extended `ActiveNpc` interface**

In `pkg/script/handlers_npc_test.go`, locate the `mockNpc` struct. Add a new field at the end of the struct (after `enqueueCalls`):

```go
	damageCalls                        []struct{ amount, dmgType int }
	enqueueCalls                       []mockEnqueueCall
	setTimerCalls                      []int
```

Locate the existing NAI-3 method `EnqueueScriptForTrigger` and add a new method immediately after it:

```go
func (m *mockNpc) SetTimer(interval int) {
	m.setTimerCalls = append(m.setTimerCalls, interval)
}
```

In `pkg/script/handlers_player_test.go`, locate `mockActiveNpc` and its four NAI-2/NAI-3 no-op stubs. Add a fifth:

```go
func (m *mockActiveNpc) SetTimer(_ int) {}
```

- [ ] **Step 6: Run tests to verify they pass + full suites**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNpcSetTimer|TestNewNpcSeedsTimerInterval' -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
```

Expected: clean build; both unit tests PASS; all prior `pkg/script/` and `modules/world/` tests still PASS.

- [ ] **Step 7: Commit**

```bash
git add pkg/script/active.go pkg/script/handlers_npc_test.go pkg/script/handlers_player_test.go modules/world/npc.go modules/world/npc_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world,script): NAI-4 Npc timer fields + SetTimer + NewNpc seeding

Add timerInterval, timerClock fields to *Npc (script-state block).
Seed timerInterval from NpcType.Timer in NewNpc (default -1 from cache
means disabled — processNpcTimer will gate on <=0 when NAI-4 Task 3
lands). Add SetTimer method with TS-faithful -1-is-noop quirk. Extend
ActiveNpc interface with SetTimer. Update mockNpc (recording) and
mockActiveNpc (no-op) to satisfy the extended interface.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: handleNpcSetTimer opcode handler + 2 handler tests

**Files:**
- Modify: `pkg/script/handlers_npc.go` (add `handleNpcSetTimer`)
- Modify: `pkg/script/handlers.go` (register `OpNpcSetTimer: handleNpcSetTimer`)
- Modify: `pkg/script/handlers_npc_test.go` (add 2 tests)

- [ ] **Step 1: Write the failing tests**

Append to `pkg/script/handlers_npc_test.go`:

```go
// TestHandleNpcSetTimer — NPC_SETTIMER pops interval and calls
// ActiveNpc.SetTimer. Mirrors TS NpcOps.ts:278-280.
func TestHandleNpcSetTimer(t *testing.T) {
	npc := &mockNpc{}
	sf := &ScriptFile{
		Name: "test_npc_settimer",
		Opcodes: []Opcode{
			OpPushConstantInt, // push interval (42)
			OpNpcSetTimer,
			OpReturn,
		},
		IntOperands: []int32{42, 0, 0},
	}
	state := Init(sf, nil, false, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= PtrActiveNpc

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(npc.setTimerCalls) != 1 {
		t.Fatalf("setTimerCalls: got %d, want 1", len(npc.setTimerCalls))
	}
	if npc.setTimerCalls[0] != 42 {
		t.Errorf("setTimerCalls[0]: got %d, want 42", npc.setTimerCalls[0])
	}
}

// TestHandleNpcSetTimerWithoutActiveNpcErrors — defensive nil check.
func TestHandleNpcSetTimerWithoutActiveNpcErrors(t *testing.T) {
	sf := &ScriptFile{
		Name: "npc_settimer_no_npc",
		Opcodes: []Opcode{
			OpPushConstantInt, OpNpcSetTimer, OpReturn,
		},
		IntOperands: []int32{5, 0, 0},
	}
	state := Init(sf, nil, false, nil, nil)
	// state.ActiveNpc intentionally nil.

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error, got nil")
	}
	want := "NPC_SETTIMER: no active npc"
	if got := err.Error(); got != want {
		t.Errorf("error: got %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleNpcSetTimer -v
```

Expected: both tests FAIL because `OpNpcSetTimer` has no registered handler. Error shape from the runner: `script "...": no handler for NPC_SETTIMER (opcode 2536) at pc=1` or similar.

- [ ] **Step 3: Add `handleNpcSetTimer` to `pkg/script/handlers_npc.go`**

Append (after `handleNpcQueue` added in NAI-3):

```go
// handleNpcSetTimer (NPC_SETTIMER, opcode 2536) sets the active
// NPC's ai_timer tick interval. Pop order: interval. Mirrors TS
// NpcOps.ts:278-280. No NumberNotNull check — tracked as future
// fidelity-audit item in nai_followups memory.
func handleNpcSetTimer(s *ScriptState) error {
	if err := requireActiveNpc(s, "NPC_SETTIMER"); err != nil {
		return err
	}
	interval := s.PopInt()
	s.ActiveNpc.SetTimer(interval)
	return nil
}
```

(`fmt` is already imported from earlier NAI tasks in this file; no import change needed.)

- [ ] **Step 4: Register the handler**

In `pkg/script/handlers.go`, locate the NPC-mutating-ops block that contains `OpNpcQueue: handleNpcQueue,` (added in NAI-3). Add `OpNpcSetTimer: handleNpcSetTimer,` in alphabetical position — between `OpNpcQueue` and `OpNpcType` (or whatever existing OpNpc* entries alphabetically surround it; gofmt realigns column widths).

- [ ] **Step 5: Run tests to verify they pass + full package**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleNpcSetTimer -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -count=1
```

Expected: both new tests PASS; no regressions.

- [ ] **Step 6: Commit**

```bash
git add pkg/script/handlers_npc.go pkg/script/handlers.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-4 handleNpcSetTimer (NPC_SETTIMER, opcode 2536)

Pop interval, call s.ActiveNpc.SetTimer. Defensive requireActiveNpc
gate. Mirrors TS NpcOps.ts:278-280. No NumberNotNull check on the
interval — tracked as future fidelity-audit item in nai_followups
memory. Happy path test verifies mock records SetTimer(42);
defensive test asserts "NPC_SETTIMER: no active npc" on nil anchor.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: processNpcTimer helper + turn() wire + integration test — closes NAI-4

**Files:**
- Modify: `modules/world/npc_script.go` (add `processNpcTimer`)
- Modify: `modules/world/npc_ai.go` (wire call in `turn()`)
- Modify: `modules/world/npc_script_test.go` (add 1 integration test)

- [ ] **Step 1: Write the failing test**

Append to `modules/world/npc_script_test.go`:

```go
// TestNpcTurnDoesNotTickTimerWhileDelayed — timer must not increment
// while the NPC is delayed. TS gates via the isValid early-return in
// turn(); Go gates internally inside processNpcTimer. Matches TS
// Npc.ts:154 (isValid) + :527-536 (processTimers).
func TestNpcTurnDoesNotTickTimerWhileDelayed(t *testing.T) {
	s, n := buildNpcForIntegration(t)
	n.timerInterval = 3
	n.delayed = true
	n.delayedUntil = s.currentTick + 100 // far future

	n.turn(s)
	n.turn(s)
	n.turn(s)

	if n.timerClock != 0 {
		t.Errorf("timerClock after 3 turns while delayed: got %d, want 0 (no tick while delayed)", n.timerClock)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcTurnDoesNotTickTimerWhileDelayed -v
```

Expected: this test may PASS by coincidence because `n.turn()` currently has no timer-pass call at all — `timerClock` stays at 0 naturally. That's acceptable. The test becomes a real regression guard once Task 3 Step 3 adds the call.

(Optional stronger red-then-green: temporarily add an unconditional `n.timerClock++` line into turn() to verify the test catches it. Not required; plan proceeds with the standard order.)

- [ ] **Step 3: Add `processNpcTimer` to `modules/world/npc_script.go`**

Append at the bottom of the file (after `processNpcQueue` added in NAI-3):

```go
// processNpcTimer fires the ai_timer trigger script when timerClock
// reaches timerInterval. Matches TS Npc.processTimers at
// Engine-TS/.../Npc.ts:527-536.
//
// Behaviour:
//   - No-op while delayed (TS gates via the isValid return in
//     turn(); Go gates internally).
//   - No-op when timerInterval <= 0 (unset or explicitly disabled
//     via SetTimer with a non-positive value).
//   - timerClock increments once per call when conditions pass.
//   - timerClock resets to 0 ONLY after a successful script fire.
//     If no ai_timer trigger script is registered for the NPC's
//     type, timerClock stays at threshold and retries every tick —
//     matches TS's "script may be registered later" semantics.
func (s *Server) processNpcTimer(n *Npc) {
	if n.delayed || n.timerInterval <= 0 {
		return
	}
	n.timerClock++
	if n.timerClock < n.timerInterval {
		return
	}
	if n.typ == nil || s.scriptProvider == nil {
		return
	}
	sf := s.scriptProvider.GetByTrigger(script.TriggerAiTimer, n.typeId, n.typ.Category)
	if sf == nil {
		return
	}
	s.runNpcScript(sf, n, nil, nil)
	n.timerClock = 0
}
```

- [ ] **Step 4: Wire `processNpcTimer` call in `Npc.turn()`**

In `modules/world/npc_ai.go`, locate the `!n.dead` prefix block. After NAI-3 it looks like:

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

Add the timer-pass call IMMEDIATELY BEFORE the existing `s.processNpcQueue(n)` line (TS order: regen → timer → queue; regen is NAI-6, not yet present). Final result:

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
		// Timer pass. Matches TS Npc.ts:178 (turn calls processTimers).
		s.processNpcTimer(n)
		// Queue pass. Matches TS Npc.ts:180 (turn calls processQueue).
		s.processNpcQueue(n)
	}
```

- [ ] **Step 5: Run tests to verify they pass + full suite + race**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNpc|TestRunNpcScript|TestResumeOrFinishNpc' -v -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: `TestNpcTurnDoesNotTickTimerWhileDelayed` PASS (now with the real timer-pass call in place, the test actually asserts delayed-gating); all prior NPC-script tests still PASS (the new call is a no-op when `timerInterval <= 0`, which is the default post-`newNpcForScriptTest` state since `newNpcForScriptTest` doesn't seed a Timer); race suite clean.

- [ ] **Step 6: Grep sanity check**

Run:
```
rg -n '\btimerInterval\b|\btimerClock\b|\bprocessNpcTimer\b|\bSetTimer\b' modules/ pkg/
```

Expected: matches appear in:
- `pkg/script/active.go` (interface method)
- `pkg/script/handlers_npc.go` (handler + call)
- `pkg/script/handlers_npc_test.go` (mock + tests)
- `pkg/script/handlers_player_test.go` (mock stub)
- `modules/world/npc.go` (fields + NewNpc seed + method)
- `modules/world/npc_script.go` (processNpcTimer)
- `modules/world/npc_ai.go` (processNpcTimer call)
- `modules/world/npc_script_test.go` (tests)

Report match count. No stray references expected.

- [ ] **Step 7: Commit, closing NAI-4**

```bash
git add modules/world/npc_script.go modules/world/npc_ai.go modules/world/npc_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-4 processNpcTimer + Npc.turn() timer pass — closes NAI-4

Add processNpcTimer helper: gate on !n.delayed + timerInterval > 0;
increment timerClock; fire via scriptProvider.GetByTrigger(TriggerAiTimer,
typeId, category); reset timerClock = 0 ONLY on successful fire
(matches TS Npc.ts:527-536 — if no script registered, timerClock
stays at threshold and retries each tick). Wire the call inside the
!n.dead prefix block of Npc.turn(), between script-resume and
processNpcQueue — matches TS regen→timer→queue order.

Integration test proves delayed-gate: 3 turns with n.delayed=true
leaves timerClock at 0.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-review checklist results

**1. Spec coverage:**

| Spec section | Task |
|---|---|
| `timerInterval`, `timerClock` fields on `*Npc` | Task 1 |
| `NewNpc` seeds `timerInterval = int(typ.Timer)` | Task 1 |
| `*Npc.SetTimer` method with -1 no-op | Task 1 |
| `ActiveNpc.SetTimer` interface method | Task 1 |
| `handleNpcSetTimer` opcode handler | Task 2 |
| `OpNpcSetTimer` registration | Task 2 |
| `processNpcTimer` helper (delayed-gate + unset-gate + timerClock-reset-only-on-fire) | Task 3 |
| `Npc.turn()` wire between resume and queue | Task 3 |
| Test: `TestNpcSetTimer` (unit) | Task 1 |
| Test: `TestNewNpcSeedsTimerIntervalFromType` (unit) | Task 1 |
| Test: `TestHandleNpcSetTimer` (handler happy path) | Task 2 |
| Test: `TestHandleNpcSetTimerWithoutActiveNpcErrors` (handler defensive) | Task 2 |
| Test: `TestNpcTurnDoesNotTickTimerWhileDelayed` (integration) | Task 3 |

All 5 spec-listed tests have tasks. No gaps.

**2. Placeholder scan:** No TBDs/TODOs/vague steps. Every code step contains complete code. Every run step has exact command + expected output. Task 2 Step 4 refers to "alphabetical position" without pinning the exact line number — this is an informed-positioning instruction, not a placeholder (the exact line depends on current gofmt column widths).

**3. Type consistency:** `timerInterval` / `timerClock` (lowercase, unexported) used consistently. `SetTimer(interval int)` signature matches across `*Npc` method, `ActiveNpc` interface, and `mockNpc` recording stub. `TriggerAiTimer` matches the existing `pkg/script/trigger.go:140` symbol. `int(typ.Timer)` cast matches the `Timer int` field on `NpcType`.

---

## Commit trail (for reference)

Three commits close NAI-4:

1. `feat(world,script): NAI-4 Npc timer fields + SetTimer + NewNpc seeding`
2. `feat(script): NAI-4 handleNpcSetTimer (NPC_SETTIMER, opcode 2536)`
3. `feat(world): NAI-4 processNpcTimer + Npc.turn() timer pass — closes NAI-4`

Each commit leaves the tree green; the final one closes NAI-4.
