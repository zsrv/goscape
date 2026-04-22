# NAI-2 NPC Script Infrastructure — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable NPC-anchored RuneScript programs to suspend via `npc_delay`, store on the NPC, and resume from the tick loop on delay expiry — the prerequisite for every behavioural NAI sub-spec downstream.

**Architecture:** Mirror the existing player-side layout (`modules/world/script.go`, `pkg/script/active.go`, `Npc.turn()` at `modules/world/npc_ai.go`) for NPC-anchored scripts. Add three lifecycle methods (`StoreActiveScript`, `ClearActiveScript`, `SetDelayed`) to the `ActiveNpc` interface, three matching fields (`activeScript`, `delayed`, `delayedUntil`) + one back-reference (`server *Server`) on `*Npc`, a new handler (`handleNpcDelay`), and two new world-side helpers (`runNpcScript`, `resumeOrFinishNpc`). Add a delayed-expiration + suspended-script resume prefix to `Npc.turn()`.

**Tech Stack:** Go 1.22+ (uses `for id := range count` idiom), existing `pkg/script` runtime, existing `modules/world` tick loop.

**Spec:** `docs/superpowers/specs/2026-04-22-nai-2-npc-script-infra-design.md`

**Roadmap:** `docs/superpowers/specs/2026-04-22-npc-ai-tick-decomposition-design.md`

---

## File Structure

**Created:**
- `modules/world/npc_script.go` — `runNpcScript`, `resumeOrFinishNpc` world-side helpers
- `modules/world/npc_script_test.go` — Tasks 1, 2, 4, 5 unit + integration tests

**Modified:**
- `modules/world/npc.go` — `+server`, `+activeScript`, `+delayed`, `+delayedUntil` fields; `+StoreActiveScript`, `+ClearActiveScript`, `+SetDelayed` methods
- `modules/world/npc_registry.go` — `addNpc` sets `n.server = s`
- `modules/world/npc_ai.go` — prepend delayed-expiration + resume block in `Npc.turn()`
- `pkg/script/active.go` — `ActiveNpc` interface grows three methods
- `pkg/script/handlers_npc.go` — `+handleNpcDelay`
- `pkg/script/handlers.go` — register `OpNpcDelay: handleNpcDelay`
- `pkg/script/handlers_npc_test.go` — Task 3 unit test

Five tasks. Each one ships a commit that leaves the tree green.

---

## Task 1: Npc struct fields + server back-ref + Store/ClearActiveScript + addNpc wiring

**Files:**
- Modify: `modules/world/npc.go` (add 4 fields + 2 methods)
- Modify: `modules/world/npc_registry.go` (set `n.server` in `addNpc`)
- Create: `modules/world/npc_script_test.go` (first test)

- [ ] **Step 1: Write the failing test**

Create `modules/world/npc_script_test.go`:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

func newNpcForScriptTest(t *testing.T) *Npc {
	t.Helper()
	typ := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 0, DebugName: "test_npc"},
	}
	return NewNpc(1, 0, 3094, 3106, 0, typ)
}

func TestNpcStoreAndClearActiveScript(t *testing.T) {
	n := newNpcForScriptTest(t)
	state := &script.ScriptState{}

	n.StoreActiveScript(state)
	if n.activeScript != state {
		t.Errorf("StoreActiveScript: got %v, want %v", n.activeScript, state)
	}

	n.ClearActiveScript()
	if n.activeScript != nil {
		t.Errorf("ClearActiveScript: got %v, want nil", n.activeScript)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcStoreAndClearActiveScript -v
```

Expected: compile error — `n.activeScript undefined`, `n.StoreActiveScript undefined`, `n.ClearActiveScript undefined`.

- [ ] **Step 3: Add fields + methods to Npc**

In `modules/world/npc.go`, locate the existing `// === AI ===` section in the Npc struct (around line 54):

```go
	// === AI ===
	targetOp        int
	wanderCounter   int
	nextPatrolTick  int
	nextPatrolPoint int
	delayedPatrol   bool
```

Insert a new section immediately before it:

```go
	// === script state ===
	server       *Server          // back-reference; set by Server.addNpc
	activeScript *script.ScriptState
	delayed      bool
	delayedUntil int

	// === AI ===
	targetOp        int
	wanderCounter   int
	nextPatrolTick  int
	nextPatrolPoint int
	delayedPatrol   bool
```

Add the `script` import at the top of `modules/world/npc.go`. The current imports are:

```go
import (
	"github.com/zsrv/goscape/pkg/objtype"
)
```

Change to:

```go
import (
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)
```

Add the two methods at the bottom of `modules/world/npc.go` (after `Slot()`):

```go
// StoreActiveScript saves a Suspended ScriptState so Npc.turn() can
// resume it when the NPC's delay expires. Part of the ActiveNpc
// interface; mirrors *Player.StoreActiveScript.
func (n *Npc) StoreActiveScript(state *script.ScriptState) {
	n.activeScript = state
}

// ClearActiveScript discards any stored ScriptState. Called after
// Finished/Aborted runs. Part of the ActiveNpc interface; mirrors
// *Player.ClearActiveScript.
func (n *Npc) ClearActiveScript() {
	n.activeScript = nil
}
```

- [ ] **Step 4: Wire `n.server = s` in addNpc**

In `modules/world/npc_registry.go`, locate:

```go
func (s *Server) addNpc(n *Npc) error {
	nid := s.allocNpcSlot()
	if nid < 0 {
		return errNpcsFull
	}
	n.nid = nid
	s.npcs[nid] = n
	s.npcLoop = append(s.npcLoop, n)
	return nil
}
```

Change to:

```go
func (s *Server) addNpc(n *Npc) error {
	nid := s.allocNpcSlot()
	if nid < 0 {
		return errNpcsFull
	}
	n.nid = nid
	n.server = s
	s.npcs[nid] = n
	s.npcLoop = append(s.npcLoop, n)
	return nil
}
```

- [ ] **Step 5: Run test to verify it passes + existing tests still pass**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcStoreAndClearActiveScript -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
```

Expected: `TestNpcStoreAndClearActiveScript` PASS; all other modules/world tests still PASS.

- [ ] **Step 6: Commit**

```bash
git add modules/world/npc.go modules/world/npc_registry.go modules/world/npc_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-2 Npc script-state fields + Store/ClearActiveScript

Add server back-reference + activeScript + delayed + delayedUntil
fields on *Npc. Add StoreActiveScript / ClearActiveScript methods
mirroring the *Player pattern. Wire n.server = s in Server.addNpc so
SetDelayed (next task) can reach currentTick via the back-reference.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: SetDelayed method + ActiveNpc interface extension

**Files:**
- Modify: `modules/world/npc.go` (add `SetDelayed`)
- Modify: `pkg/script/active.go` (extend `ActiveNpc` interface)
- Modify: `modules/world/npc_script_test.go` (add one test)

- [ ] **Step 1: Write the failing test**

Append to `modules/world/npc_script_test.go`:

```go
func TestNpcSetDelayed(t *testing.T) {
	n := newNpcForScriptTest(t)
	s := &Server{}
	s.currentTick = 100
	n.server = s

	n.SetDelayed(5)

	if !n.delayed {
		t.Errorf("delayed: got false, want true")
	}
	want := 100 + 1 + 5
	if n.delayedUntil != want {
		t.Errorf("delayedUntil: got %d, want %d", n.delayedUntil, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcSetDelayed -v
```

Expected: compile error — `n.SetDelayed undefined`.

- [ ] **Step 3: Add SetDelayed to Npc**

Append to `modules/world/npc.go` (after `ClearActiveScript`):

```go
// SetDelayed marks the NPC as suspended for `ticks` more ticks starting
// next tick. delayedUntil = currentTick + 1 + ticks, matching TS
// Npc.delay() and ActivePlayer.SetDelayed semantics.
func (n *Npc) SetDelayed(ticks int) {
	n.delayed = true
	n.delayedUntil = n.server.currentTick + 1 + ticks
}
```

- [ ] **Step 4: Extend ActiveNpc interface**

In `pkg/script/active.go`, locate the existing `ActiveNpc` interface (around line 310):

```go
// ActiveNpc is the per-NPC surface that NPC_* opcodes and VARN
// handlers read/write. Set on ScriptState before Execute by callers
// that target a specific NPC (test fixtures, OPNPC routing, etc.).
type ActiveNpc interface {
	NpcType() int // returns NpcType.id
	NpcX() int
	NpcZ() int
	NpcLevel() int
	NpcStat(stat int) int     // current (boosted) level — S6a: only HP (id 0) is real
	NpcBaseStat(stat int) int // base level — S6a: only HP (id 0) is real
	NpcCategory() int
	NpcUID() int // (typeId << 16) | nid
	NpcVarN(id int) int32
	SetNpcVarN(id int, val int32)
```

Find the end of the `ActiveNpc` interface block (closing `}`) and insert these three methods just before it:

```go
	// StoreActiveScript saves a NpcSuspended ScriptState so Npc.turn()
	// can resume it when the NPC's delay expires. Mirrors
	// ActivePlayer.StoreActiveScript at active.go:22-24.
	StoreActiveScript(state *ScriptState)

	// ClearActiveScript discards any stored ScriptState. Called after
	// Finished/Aborted runs. Mirrors ActivePlayer.ClearActiveScript.
	ClearActiveScript()

	// SetDelayed marks the NPC as suspended for `ticks` more ticks
	// starting next tick. Implementations compute delayedUntil =
	// currentTick + 1 + ticks. Mirrors ActivePlayer.SetDelayed at
	// active.go:13-14.
	SetDelayed(ticks int)
```

- [ ] **Step 5: Run test to verify it passes + everything still compiles**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcSetDelayed -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -count=1
```

Expected: clean build; `TestNpcSetDelayed` PASS; all `pkg/script/` tests still PASS (the interface extension is additive — all existing ActiveNpc implementations now need the three new methods, but the only production implementer is `*Npc` which has them).

**If the build fails with "does not implement ActiveNpc":** Some test-only mock in `pkg/script` may need the three methods. If so, find the mock (grep for `ActiveNpc` in `pkg/script/*_test.go`) and add no-op implementations to the mock. Report which mock needed the addition in the commit message.

- [ ] **Step 6: Commit**

```bash
git add modules/world/npc.go pkg/script/active.go modules/world/npc_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world,script): NAI-2 Npc.SetDelayed + ActiveNpc interface extension

Add SetDelayed(ticks) on *Npc computing delayedUntil = currentTick
+ 1 + ticks via the n.server back-reference. Extend ActiveNpc
interface with StoreActiveScript, ClearActiveScript, SetDelayed —
symmetric with ActivePlayer for the script-lifecycle surface.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: handleNpcDelay opcode handler

**Files:**
- Modify: `pkg/script/handlers_npc.go` (add `handleNpcDelay`)
- Modify: `pkg/script/handlers.go` (register `OpNpcDelay: handleNpcDelay`)
- Modify: `pkg/script/handlers_npc_test.go` (add one test)

- [ ] **Step 1: Write the failing test**

Append to `pkg/script/handlers_npc_test.go`:

```go
// TestHandleNpcDelayWithoutActiveNpcErrors — defensive check when
// NPC_DELAY runs with no active npc anchored.
func TestHandleNpcDelayWithoutActiveNpcErrors(t *testing.T) {
	sf := &ScriptFile{
		Name:     "npc_delay_no_npc",
		Opcodes:  []Opcode{OpPushConstantInt, OpNpcDelay, OpReturn},
		IntOperands: []int{3},
	}

	state := Init(sf, nil, false, nil, nil)
	// state.ActiveNpc intentionally left nil.

	err := Execute(state)
	if err == nil {
		t.Fatalf("Execute: want error, got nil")
	}
	want := "NPC_DELAY: no active npc"
	if got := err.Error(); got != want {
		t.Errorf("error: got %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleNpcDelayWithoutActiveNpcErrors -v
```

Expected: either compile-pass + test-FAIL with something like "unknown opcode 2511" (because OpNpcDelay has no handler registered), or PASS-for-wrong-reason if the Execute machinery coincidentally surfaces a different error.

**If the expected error format doesn't match the "unknown opcode" text:** Check what `Execute` returns for an unregistered opcode by grepping `pkg/script/runner.go` for the dispatch loop. The actual "fails for the right eventual reason" here is: error is non-nil because no handler is registered. Adjust the test's expected-error string if needed.

- [ ] **Step 3: Add handleNpcDelay**

In `pkg/script/handlers_npc.go`, append (after existing handlers, but before any trailing closing brace if the file ends with a registration block — check structure first):

```go
// handleNpcDelay (NPC_DELAY, opcode 2511) suspends the active NPC's
// script for N ticks. Transitions the script to NpcSuspended and
// records the wake tick on the NPC via SetDelayed. The tick loop
// resumes the script from Npc.turn() when delayedUntil expires.
// Mirrors TS NpcOps.ts NPC_DELAY.
func handleNpcDelay(s *ScriptState) error {
	if s.ActiveNpc == nil {
		return fmt.Errorf("NPC_DELAY: no active npc")
	}
	ticks := s.PopInt()
	s.ActiveNpc.SetDelayed(ticks)
	s.Execution = NpcSuspended
	return nil
}
```

Verify `handlers_npc.go` imports `fmt`. Search at the top of the file — most existing handlers return fmt.Errorf, so it's likely already imported.

- [ ] **Step 4: Register the handler**

In `pkg/script/handlers.go`, locate the registration block that includes `OpNpcAnim, OpNpcFaceSquare, OpNpcChangeType, OpNpcDamage` (the NPC mutating ops, around the S6c line-range). Add one new line to that block:

```go
OpNpcDelay:    handleNpcDelay,
```

If there are multiple NPC-adjacent registration blocks, pick the one that groups mutating-state NPC ops (i.e. the block with `OpNpcAnim` and friends). Consistency: alphabetical within block is the local convention — place `OpNpcDelay` between `OpNpcDamage` and `OpNpcFaceSquare`.

- [ ] **Step 5: Run test to verify it passes**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestHandleNpcDelayWithoutActiveNpcErrors -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -count=1
```

Expected: target test PASS; entire `pkg/script` suite PASS with zero regressions.

- [ ] **Step 6: Commit**

```bash
git add pkg/script/handlers_npc.go pkg/script/handlers.go pkg/script/handlers_npc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-2 handleNpcDelay (NPC_DELAY, opcode 2511)

Pop int N, call s.ActiveNpc.SetDelayed(N), set Execution =
NpcSuspended. Defensive error when ActiveNpc is nil matches the
requireActiveNpc pattern used elsewhere in handlers_npc.go. Mirrors
TS NpcOps.ts NPC_DELAY, which is the only opcode that produces the
NpcSuspended state. No Protect gate — NPC scripts don't use the
ProtectedActivePlayer concept (that's a player-re-entrancy guard,
not an NPC concern).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: runNpcScript + resumeOrFinishNpc helpers

**Files:**
- Create: `modules/world/npc_script.go`
- Modify: `modules/world/npc_script_test.go` (add two tests)

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/npc_script_test.go`:

```go
// newServerForScriptTest builds a minimal *Server wired for running
// NPC-anchored scripts. Reuses the pattern from script_test.go:939-952.
func newServerForScriptTest(t *testing.T) *Server {
	t.Helper()
	return &Server{
		log: newTestLogger(t),
	}
}

func TestRunNpcScriptFiresAndFinishes(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForScriptTest(t)
	n.server = s

	sf := &script.ScriptFile{
		Name:    "trivial_return",
		Opcodes: []script.Opcode{script.OpReturn},
	}

	s.runNpcScript(sf, n, nil, nil)

	if n.activeScript != nil {
		t.Errorf("activeScript: got %v, want nil (script finished)", n.activeScript)
	}
	if n.delayed {
		t.Errorf("delayed: got true, want false")
	}
}

func TestRunNpcScriptSuspendsOnNpcDelay(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForScriptTest(t)
	n.server = s

	sf := &script.ScriptFile{
		Name:        "npc_delay_3_return",
		Opcodes:     []script.Opcode{script.OpPushConstantInt, script.OpNpcDelay, script.OpReturn},
		IntOperands: []int{3},
	}

	s.runNpcScript(sf, n, nil, nil)

	if n.activeScript == nil {
		t.Fatalf("activeScript: got nil, want stored state")
	}
	if n.activeScript.Execution != script.NpcSuspended {
		t.Errorf("Execution: got %v, want NpcSuspended", n.activeScript.Execution)
	}
	if !n.delayed {
		t.Errorf("delayed: got false, want true")
	}
	want := 100 + 1 + 3
	if n.delayedUntil != want {
		t.Errorf("delayedUntil: got %d, want %d", n.delayedUntil, want)
	}
}
```

**Note on `newTestLogger`:** if no such helper exists in the test tree, inline one at the top of the file:

```go
import (
	"io"
	"log/slog"
	// ... existing imports
)

func newTestLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
```

Also check if `s.log` is the right field name — grep for `s\.log` in existing tests. If the actual field name differs (e.g., `logger`), use that.

- [ ] **Step 2: Run tests to verify they fail**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestRunNpcScript' -v
```

Expected: compile error — `s.runNpcScript undefined`.

- [ ] **Step 3: Create `modules/world/npc_script.go`**

Create `modules/world/npc_script.go`:

```go
package world

import (
	"github.com/zsrv/goscape/pkg/script"
)

// runNpcScript initialises a ScriptState anchored on npc (not a
// player) and routes the result via resumeOrFinishNpc. Safe to call
// with a nil scriptFile (no-op) so callers don't have to nil-check
// the trigger lookup. Mirrors runScript at modules/world/script.go:14.
//
// If the script suspends (Execution == NpcSuspended), the state is
// stored on the NPC and Npc.turn() resumes it when the NPC's delay
// expires via the prefix block added in NAI-2.
func (s *Server) runNpcScript(sf *script.ScriptFile, npc script.ActiveNpc, intArgs []int, stringArgs []string) {
	if sf == nil {
		return
	}
	state := script.Init(sf, nil, false, intArgs, stringArgs)
	state.ActiveNpc = npc
	state.Provider = s.scriptProvider
	state.World = s.worldVars
	state.Configs = s.configsView
	state.Inv = s.invLookup
	s.resumeOrFinishNpc(state, npc)
}

// resumeOrFinishNpc is the shared post-Execute handler for both fresh
// NPC-anchored runs (from runNpcScript) and resumed runs (from
// Npc.turn()). Mirrors resumeOrFinish at modules/world/script.go:30
// but routes via the ActiveNpc interface instead of ActivePlayer.
func (s *Server) resumeOrFinishNpc(state *script.ScriptState, npc script.ActiveNpc) {
	if err := script.Execute(state); err != nil {
		s.log.Warn("npc script execute error",
			"script", state.Script.Name, "err", err)
		npc.ClearActiveScript()
		return
	}
	switch state.Execution {
	case script.Finished, script.Aborted:
		npc.ClearActiveScript()
	case script.NpcSuspended:
		npc.StoreActiveScript(state)
	default:
		// Suspended / PauseButton / CountDialog / WorldSuspended —
		// not reachable via npc_delay alone, but defensively clear.
		s.log.Warn("npc script in unexpected execution state",
			"script", state.Script.Name, "execution", state.Execution)
		npc.ClearActiveScript()
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestRunNpcScript' -v
```

Expected: both `TestRunNpcScriptFiresAndFinishes` and `TestRunNpcScriptSuspendsOnNpcDelay` PASS.

Also run the full world suite:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
```

Expected: no regressions.

- [ ] **Step 5: Commit**

```bash
git add modules/world/npc_script.go modules/world/npc_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-2 runNpcScript + resumeOrFinishNpc helpers

Mirror the player-side runScript / resumeOrFinish pair for NPC-anchored
scripts. runNpcScript calls script.Init with nil ActivePlayer (Self =
nil, no PtrActivePlayer), sets state.ActiveNpc = npc, and routes via
resumeOrFinishNpc which dispatches on Execution state: Finished/
Aborted clear, NpcSuspended stores, everything else warns and clears.
Integration tests cover both the trivial-return path and the
npc_delay-suspends path end to end.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Npc.turn() prefix — closes NAI-2

**Files:**
- Modify: `modules/world/npc_ai.go` (prepend delayed-expiration + resume block)
- Modify: `modules/world/npc_script_test.go` (add two end-to-end tests)

- [ ] **Step 1: Write the failing tests**

Append to `modules/world/npc_script_test.go`:

```go
func TestNpcTurnResumesSuspendedScriptAfterDelay(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForScriptTest(t)
	n.server = s

	sf := &script.ScriptFile{
		Name:        "npc_delay_3_return",
		Opcodes:     []script.Opcode{script.OpPushConstantInt, script.OpNpcDelay, script.OpReturn},
		IntOperands: []int{3},
	}

	// Suspend: after this, delayedUntil = 104.
	s.runNpcScript(sf, n, nil, nil)
	if n.activeScript == nil || !n.delayed {
		t.Fatalf("setup: expected suspended state")
	}

	// Advance to delayedUntil and call turn.
	s.currentTick = 104
	n.turn(s)

	if n.activeScript != nil {
		t.Errorf("activeScript: got %v, want nil (resumed and finished)", n.activeScript)
	}
	if n.delayed {
		t.Errorf("delayed: got true, want false (delay expired)")
	}
}

func TestNpcTurnDoesNotResumeWhileDelayed(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForScriptTest(t)
	n.server = s

	sf := &script.ScriptFile{
		Name:        "npc_delay_3_return",
		Opcodes:     []script.Opcode{script.OpPushConstantInt, script.OpNpcDelay, script.OpReturn},
		IntOperands: []int{3},
	}

	// Suspend: delayedUntil = 104.
	s.runNpcScript(sf, n, nil, nil)

	// Advance to one tick BEFORE delayedUntil.
	s.currentTick = 103
	n.turn(s)

	if n.activeScript == nil {
		t.Errorf("activeScript: got nil, want still-suspended state")
	}
	if n.activeScript != nil && n.activeScript.Execution != script.NpcSuspended {
		t.Errorf("Execution: got %v, want still NpcSuspended", n.activeScript.Execution)
	}
	if !n.delayed {
		t.Errorf("delayed: got false, want true (still within delay window)")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNpcTurn(Resumes|DoesNotResume)' -v
```

Expected: either tests FAIL (because turn() currently has no resume block, so the script stays suspended forever — the "Resumes" test fails the "got nil, want nil" check, the "DoesNotResume" test might accidentally pass by coincidence). At minimum `TestNpcTurnResumesSuspendedScriptAfterDelay` MUST fail.

- [ ] **Step 3: Prepend resume block in Npc.turn()**

In `modules/world/npc_ai.go`, locate the start of the `turn` method:

```go
// turn runs once per tick from processNpcs.
func (n *Npc) turn(s *Server) {
	if n.dead {
		n.lifecycleTick--
```

Change to:

```go
// turn runs once per tick from processNpcs.
func (n *Npc) turn(s *Server) {
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

	if n.dead {
		n.lifecycleTick--
```

Ensure `modules/world/npc_ai.go` imports `script`. Current imports:

```go
import (
	"math/rand/v2"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/rsbuf"
)
```

Add `"github.com/zsrv/goscape/pkg/script"`:

```go
import (
	"math/rand/v2"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
)
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestNpcTurn' -v
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1
```

Expected: both new tests PASS; all prior NPC tick tests (`TestKillSetsDeadAndLifecycleTick`, `TestTeleportHomeAfterStuck`, `TestRespawnAfterKill`, etc.) still PASS — the prefix block is a no-op when `delayed=false` and `activeScript=nil`, which is the default post-`NewNpc` state.

- [ ] **Step 5: Run the full race-enabled suite**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: full repo passes with race detector.

- [ ] **Step 6: Grep sanity check**

Run:
```
rg -n '\bactiveScript\b|\bdelayedUntil\b' modules/world/ pkg/script/
```

Expected: references appear in `modules/world/npc.go`, `modules/world/npc_script.go`, `modules/world/npc_ai.go`, `modules/world/npc_script_test.go`, and `pkg/script/handlers_npc.go` (the `ActiveNpc.SetDelayed` interface doc mentions `delayedUntil` conceptually). No stray references in files that shouldn't know about NPC script state.

- [ ] **Step 7: Commit, closing NAI-2**

```bash
git add modules/world/npc_ai.go modules/world/npc_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-2 Npc.turn() resume block — closes NAI-2

Prepend delayed-expiration + suspended-script resume to Npc.turn().
When delayed expires and an NpcSuspended activeScript is stored, flip
Execution back to Running and route through resumeOrFinishNpc — which
re-enters Execute, potentially re-suspending or finishing. Matches TS
Npc.ts:113-118. End-to-end tests prove: (1) a script that calls
npc_delay 3 at tick 100 is resumed at tick 104, (2) the same script
is not prematurely resumed at tick 103.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-review checklist results

**1. Spec coverage:**

| Spec section | Task |
|---|---|
| `Npc` struct: `server`, `activeScript`, `delayed`, `delayedUntil` fields | Task 1 |
| `Npc` methods: `StoreActiveScript`, `ClearActiveScript` | Task 1 |
| `Npc` methods: `SetDelayed` | Task 2 |
| `Server.addNpc` wires `n.server = s` | Task 1 |
| `ActiveNpc` interface: 3 new methods | Task 2 |
| `handleNpcDelay` + opcode registration | Task 3 |
| `runNpcScript`, `resumeOrFinishNpc` helpers | Task 4 |
| `Npc.turn()` resume prefix | Task 5 |
| Tests 1-6 + test 7 | Tasks 1, 2, 3, 4, 5 |

No gaps. Open-verification items from spec:
- Protected-pointer cleanup — verified during plan-write: TS's cleanup pattern handles player-side state on scripts that touch both active-player + npc-delay. Go's NPC scripts start with `Self=nil, Protect=false` (see Task 4's `script.Init(sf, nil, false, ...)`), so there is nothing to clean up. No code needed.
- `Protect` check on `handleNpcDelay` — verified: `handlePDelay` uses `requireProtectedActivePlayer`, but that gate is a player-re-entrancy guard, not an NPC concern. Task 3 correctly omits it.
- `addNpc` as single entry point — verified at `modules/world/npc_registry.go:30`. Task 1 covers it.

**2. Placeholder scan:** No TBDs, TODOs, or vague steps. Every step contains the exact code and every run step contains the exact command with expected output.

**3. Type consistency:** `activeScript` (lowercase), `StoreActiveScript` / `ClearActiveScript` / `SetDelayed` (exported methods), `NpcSuspended` (script package), `ActiveNpc` (interface), `runNpcScript` / `resumeOrFinishNpc` (unexported world-side helpers) — all match across Tasks 1-5.

---

## Commit trail (for reference)

Five commits close NAI-2:

1. `feat(world): NAI-2 Npc script-state fields + Store/ClearActiveScript`
2. `feat(world,script): NAI-2 Npc.SetDelayed + ActiveNpc interface extension`
3. `feat(script): NAI-2 handleNpcDelay (NPC_DELAY, opcode 2511)`
4. `feat(world): NAI-2 runNpcScript + resumeOrFinishNpc helpers`
5. `feat(world): NAI-2 Npc.turn() resume block — closes NAI-2`

Each commit leaves the tree green (build + all tests pass) and ships self-contained work.
