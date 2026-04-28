# NAI-42 — Tick-wide panic-recovery convention Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wrap five TS-faithful tick sites in panic-recovery helpers so a panic during one player's tick step (or one world-script execution) cannot abort the tick goroutine. Closes `NAI-37-D-WORLDQUEUE-NO-PANIC-RECOVERY`.

**Architecture:** Two helpers in a new file (`tick_recovery.go`): `recoverPlayer` (per-player, force-disconnects) and `recoverWorldScript` (logs only). Each wrap site uses `defer <helper>(...)` inside a per-iteration closure. Recovery action mirrors TS `World.ts:651-657` / `736-742` (set `requestLogout` + close `client.conn`).

**Tech Stack:** Go 1.26+ (per `go_version.md`; use `use-modern-go` skill). TS source: `LostCityRS/Engine-TS` only per `ts_source_canonical_path.md`. HEAD baseline: `d0b897c` (NAI-42 spec commit).

---

## Spec reference

Spec at `docs/superpowers/specs/2026-04-28-nai-42-tick-panic-recovery-design.md`.

| Spec section | Plan task |
|--------------|-----------|
| §4.1 helpers + §5 helper tests | T1 |
| §4.2 + §4.3 sites 1-4 (tick.go) | T2 |
| §4.3 site 5 + §7 deviation closure | T3 |

## File structure

| File | Status | Responsibility |
|------|--------|----------------|
| `modules/world/tick_recovery.go` | **create** | `recoverPlayer`, `recoverWorldScript` helpers |
| `modules/world/tick_recovery_test.go` | **create** | helper unit + integration tests |
| `modules/world/tick.go` | modify | wrap 4 per-player iteration sites (lines 70, 202, 261, 428) |
| `modules/world/world_script_queue.go` | modify | wrap `resumeOrFinishWorld` call; retire `NAI-37-D-WORLDQUEUE-NO-PANIC-RECOVERY` deviation comment |

## Pre-flight checks (controller)

Per `controller_preflight.md`: re-grep each premise against HEAD before dispatching each task.

| Check | Command | Expected at HEAD `d0b897c` |
|-------|---------|----------------------------|
| `Server.log` field | `rg -n '^\s+log\s+\*slog\.Logger' modules/world/server.go` | `server.go:48` |
| `Player.requestLogout` field | `rg -n '\brequestLogout\b' modules/world/player.go` | `player.go:180` |
| `Player.client` + `client.conn` | `rg -n '^\s+client\s+\*client\b\|^\s+conn\s+net\.Conn' modules/world/player.go modules/world/client.go` | `player.go:?`, `client.go:36` |
| `ScriptFile.Name` | `rg -n '^\s+Name\s+string' pkg/script/file.go` | `file.go:15` |
| `ScriptState.Script` | `rg -n '^\s+Script\s+\*ScriptFile' pkg/script/state.go` | `state.go:137` |
| 4 wrap sites in tick.go | `rg -n 'for _, p := range players \{' modules/world/tick.go` | lines 70, 146, 187, 202, 261, 302, 318, 342, 428, 438 (we wrap 70, 202, 261, 428) |
| `processWorldQueue` shape | `sed -n '60,86p' modules/world/world_script_queue.go` | matches §3 of spec |
| `slog.DiscardHandler` available | `go doc log/slog.DiscardHandler` | exists (Go 1.24+) |
| no existing `recoverPlayer`/`recoverWorldScript` | `rg -n 'recoverPlayer\|recoverWorldScript' modules/` | empty |

Halt and report if any check fails.

---

### Task 1: Helpers + helper tests

**Goal:** Land `tick_recovery.go` and `tick_recovery_test.go`. Production wrap sites untouched. After T1, helpers exist and are tested but no production code calls them.

**Files:**
- Create: `modules/world/tick_recovery.go`
- Create: `modules/world/tick_recovery_test.go`

#### Step 1.1: Write failing tests

Create `modules/world/tick_recovery_test.go`:

```go
package world

import (
	"errors"
	"log/slog"
	"net"
	"sync/atomic"
	"testing"

	"github.com/zsrv/goscape/pkg/script"
)

// mockConn satisfies net.Conn; only Close is observable. Read/Write etc.
// will nil-panic if called — tests must not exercise them.
type mockConn struct {
	net.Conn
	closed atomic.Bool
}

func (m *mockConn) Close() error {
	m.closed.Store(true)
	return nil
}

func newDiscardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// TestRecoverPlayer_NoPanic: when no panic, recoverPlayer is a no-op.
// requestLogout stays false, the conn stays open.
func TestRecoverPlayer_NoPanic(t *testing.T) {
	mc := &mockConn{}
	p := &Player{username: "alice", client: &client{conn: mc}}
	log := newDiscardLogger()

	func() {
		defer recoverPlayer(p, "test", log)
		// no panic
	}()

	if p.requestLogout {
		t.Error("requestLogout: want false on clean run, got true")
	}
	if mc.closed.Load() {
		t.Error("conn.closed: want false on clean run, got true")
	}
}

// TestRecoverPlayer_PanicSetsLogout: a panic inside the deferred frame
// must set requestLogout = true (mirrors TS player.logout()).
func TestRecoverPlayer_PanicSetsLogout(t *testing.T) {
	mc := &mockConn{}
	p := &Player{username: "alice", client: &client{conn: mc}}
	log := newDiscardLogger()

	func() {
		defer recoverPlayer(p, "test", log)
		panic("boom")
	}()

	if !p.requestLogout {
		t.Error("requestLogout: want true after panic, got false")
	}
}

// TestRecoverPlayer_PanicClosesConn: a panic must close the player's
// client connection (mirrors TS player.client.close()).
func TestRecoverPlayer_PanicClosesConn(t *testing.T) {
	mc := &mockConn{}
	p := &Player{username: "alice", client: &client{conn: mc}}
	log := newDiscardLogger()

	func() {
		defer recoverPlayer(p, "test", log)
		panic("boom")
	}()

	if !mc.closed.Load() {
		t.Error("conn.closed: want true after panic, got false")
	}
}

// TestRecoverPlayer_NilClientSafe: recovery must not panic when
// p.client is nil (test players often have no wire connection).
func TestRecoverPlayer_NilClientSafe(t *testing.T) {
	p := &Player{username: "alice"} // client is nil
	log := newDiscardLogger()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("recoverPlayer should not propagate; got: %v", r)
		}
	}()

	func() {
		defer recoverPlayer(p, "test", log)
		panic("boom")
	}()

	if !p.requestLogout {
		t.Error("requestLogout: want true even with nil client")
	}
}

// TestRecoverPlayer_PanicWithErrorValue: panics with an error value
// must be recovered (Go panic value can be any).
func TestRecoverPlayer_PanicWithErrorValue(t *testing.T) {
	p := &Player{username: "alice"}
	log := newDiscardLogger()

	func() {
		defer recoverPlayer(p, "test", log)
		panic(errors.New("typed error"))
	}()

	if !p.requestLogout {
		t.Error("requestLogout: want true after error-typed panic")
	}
}

// TestRecoverWorldScript_NoPanic: no-op when the deferred frame
// returns normally.
func TestRecoverWorldScript_NoPanic(t *testing.T) {
	state := &script.ScriptState{Script: &script.ScriptFile{Name: "[world,demo]"}}
	log := newDiscardLogger()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("clean run should not propagate; got: %v", r)
		}
	}()

	func() {
		defer recoverWorldScript(state, log)
		// no panic
	}()
}

// TestRecoverWorldScript_PanicSwallowed: a panic during world-script
// execution must be swallowed (caller's loop continues).
func TestRecoverWorldScript_PanicSwallowed(t *testing.T) {
	state := &script.ScriptState{Script: &script.ScriptFile{Name: "[world,demo]"}}
	log := newDiscardLogger()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("recoverWorldScript should swallow; got: %v", r)
		}
	}()

	func() {
		defer recoverWorldScript(state, log)
		panic("boom")
	}()
}

// TestRecoverWorldScript_NilStateSafe: nil state must not nil-panic
// inside the recovery (defensive; production callers always pass non-nil).
func TestRecoverWorldScript_NilStateSafe(t *testing.T) {
	log := newDiscardLogger()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("recoverWorldScript should not nil-panic; got: %v", r)
		}
	}()

	func() {
		defer recoverWorldScript(nil, log)
		panic("boom")
	}()
}

// TestRecoverPlayer_ThreePlayers_OnePanics: integration-style. Three
// players in a per-player iteration; the second panics; the first and
// third must run cleanly and only the second is force-disconnected.
// Mirrors TS World.processClients per-iteration recovery scope.
func TestRecoverPlayer_ThreePlayers_OnePanics(t *testing.T) {
	mc1, mc2, mc3 := &mockConn{}, &mockConn{}, &mockConn{}
	p1 := &Player{username: "p1", client: &client{conn: mc1}}
	p2 := &Player{username: "p2", client: &client{conn: mc2}}
	p3 := &Player{username: "p3", client: &client{conn: mc3}}
	players := []*Player{p1, p2, p3}
	log := newDiscardLogger()

	var ran [3]bool
	for i, p := range players {
		func(i int, p *Player) {
			defer recoverPlayer(p, "test", log)
			ran[i] = true
			if p.username == "p2" {
				panic("boom")
			}
		}(i, p)
	}

	if !ran[0] || !ran[1] || !ran[2] {
		t.Errorf("ran: want all three reached fn body, got %v", ran)
	}
	if p1.requestLogout || p3.requestLogout {
		t.Errorf("requestLogout: want only p2, got p1=%v p3=%v",
			p1.requestLogout, p3.requestLogout)
	}
	if !p2.requestLogout {
		t.Error("requestLogout: want true for panicking player p2")
	}
	if mc1.closed.Load() || mc3.closed.Load() {
		t.Error("conn.closed: only p2's conn should close")
	}
	if !mc2.closed.Load() {
		t.Error("conn.closed: want true for p2's conn after panic")
	}
}
```

#### Step 1.2: Run test to verify they fail

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestRecover -v`

Expected: compile error (`recoverPlayer`/`recoverWorldScript` undefined).

#### Step 1.3: Create the helpers

Create `modules/world/tick_recovery.go`:

```go
package world

import (
	"log/slog"
	"runtime/debug"

	"github.com/zsrv/goscape/pkg/script"
)

// recoverPlayer recovers from panics during a per-player tick step.
//
// Mirrors TS World.processClients (World.ts:651-657) and World.processPlayers
// (World.ts:736-742) catch action: structured log + force-disconnect. Sets
// p.requestLogout so the existing processLogouts loop (tick.go:140) picks
// the player up next tick, and closes the TCP connection immediately so
// any further decode attempt fails fast.
//
// Must be called from inside a `defer recoverPlayer(...)` registered as
// the FIRST deferred call in a per-iteration closure — Go semantics
// require recover() to run inside the deferred frame.
//
// op identifies the tick step ("processIn", "processInteraction", etc.)
// for log readability; pass a constant string per call site.
func recoverPlayer(p *Player, op string, log *slog.Logger) {
	r := recover()
	if r == nil {
		return
	}
	username := ""
	if p != nil {
		username = p.username
	}
	log.Error("panic in tick step",
		"op", op,
		"player", username,
		"err", r,
		"stack", string(debug.Stack()))
	if p == nil {
		return
	}
	p.requestLogout = true
	if p.client != nil && p.client.conn != nil {
		_ = p.client.conn.Close()
	}
}

// recoverWorldScript recovers from panics during world-script-queue
// execution. The world queue has no Player to disconnect; the offending
// entry was already removed before fire (per processWorldQueue's
// remove-before-fire ordering at world_script_queue.go:75), so recovery
// only logs.
//
// Mirrors TS World.ts:534-559 catch action.
func recoverWorldScript(state *script.ScriptState, log *slog.Logger) {
	r := recover()
	if r == nil {
		return
	}
	scriptName := ""
	if state != nil && state.Script != nil {
		scriptName = state.Script.Name
	}
	log.Error("panic in world script execution",
		"script", scriptName,
		"err", r,
		"stack", string(debug.Stack()))
}
```

#### Step 1.4: Run tests to verify they pass

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestRecover -v`

Expected: 9 tests pass.

#### Step 1.5: Run full package to verify no regression

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/`

Expected: PASS (helpers are dead code at this point — no production callers — but tests are green).

#### Step 1.6: Commit

```bash
git add modules/world/tick_recovery.go modules/world/tick_recovery_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-42 T1 — tick panic-recovery helpers

recoverPlayer + recoverWorldScript helpers in tick_recovery.go.
Mirrors TS World.processClients (World.ts:651-657) and
World.processPlayers (World.ts:736-742) catch shape: structured log
+ force-disconnect (set requestLogout + close client.conn). World-queue
recovery logs only.

No production wire-up yet; T2/T3 wrap the call sites.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Wrap 4 per-player iteration sites in `tick.go`

**Goal:** Apply `defer recoverPlayer(...)` to the 4 TS-faithful per-player sites. Existing tests must stay green (wrappers are pass-through).

**Files:**
- Modify: `modules/world/tick.go` (4 wrap sites: lines 70-72, 202-216, 261-293, 428-430)

#### Step 2.1: Wrap `processClientsIn` (tick.go:70-72)

**Replace** lines 70-72:

```go
	for _, p := range players {
		p.processIn(s.currentTick)
	}
```

with:

```go
	for _, p := range players {
		func(p *Player) {
			defer recoverPlayer(p, "processIn", s.log)
			p.processIn(s.currentTick)
		}(p)
	}
```

#### Step 2.2: Wrap `processActiveScripts` (tick.go:202-216)

**Replace** the loop body at lines 202-216:

```go
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
```

with:

```go
	for _, p := range players {
		func(p *Player) {
			defer recoverPlayer(p, "processActiveScripts", s.log)
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
		}(p)
	}
```

#### Step 2.3: Wrap `processPlayerTimers` (tick.go:261-293)

**Replace** the loop body at lines 261-293:

```go
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
			sf := s.scriptProvider.GetByID(id)
			if sf == nil {
				continue
			}
			s.runScript(sf, p, nil, false, t.IntArgs, t.StringArgs)
		}
	}
```

with:

```go
	for _, p := range players {
		func(p *Player) {
			defer recoverPlayer(p, "processPlayerTimers", s.log)
			if len(p.timers) == 0 {
				return
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
				sf := s.scriptProvider.GetByID(id)
				if sf == nil {
					continue
				}
				s.runScript(sf, p, nil, false, t.IntArgs, t.StringArgs)
			}
		}(p)
	}
```

Note: the loop body's `continue` becomes `return` inside the closure (we're now in a function, not a for-loop body — `return` is the closure-equivalent of skipping to next iteration).

#### Step 2.4: Wrap `processInteractions` (tick.go:428-430)

**Replace** lines 428-430:

```go
	for _, p := range players {
		p.processInteraction()
	}
```

with:

```go
	for _, p := range players {
		func(p *Player) {
			defer recoverPlayer(p, "processInteraction", s.log)
			p.processInteraction()
		}(p)
	}
```

#### Step 2.5: Run full package to verify no regression

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/`

Expected: PASS. Wrappers are pass-through when no panic occurs; existing green tests stay green.

#### Step 2.6: Run race detector

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/`

Expected: PASS. The closures capture `p` and `s.log` by value/pointer; no new concurrency hazards.

#### Step 2.7: Commit

```bash
git add modules/world/tick.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-42 T2 — wrap 4 per-player tick iterations

processClientsIn, processActiveScripts, processPlayerTimers, and
processInteractions wrap their per-player loop bodies in
`defer recoverPlayer(p, op, s.log)`. Maps onto TS World.processClients
+ World.processPlayers try/catch coverage (World.ts:603-658, 703-743).

Wrappers are pass-through when no panic occurs; existing tests stay
green. The 6 unwrapped tick sites (processLogouts, processPathing,
processClientsOut, processInfo×2, processCleanup) have no TS
counterpart in try/catch and remain bare per spec §4.4.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Wrap `processWorldQueue` + retire deviation

**Goal:** Wrap the per-entry script-execute call in `processWorldQueue` with `defer recoverWorldScript(...)` and retire the `NAI-37-D-WORLDQUEUE-NO-PANIC-RECOVERY` deviation comment.

T1's unit tests already prove `recoverWorldScript` swallows panics correctly. T3's wrap is a 4-line transformation visible in the diff; no integration test is added because the script runner returns errors (not panics) on malformed scripts (`pkg/script/runner.go:54-78`), so a deterministic panic trigger isn't trivial without test-only opcode infrastructure that's not warranted at this scope.

**Files:**
- Modify: `modules/world/world_script_queue.go` (wrap fire site at line 82; rewrite deviation comment block at lines 55-59)

#### Step 3.1: Pre-flight grep for stale deviation references

Run: `rg -n 'NAI-37-D-WORLDQUEUE-NO-PANIC-RECOVERY' .`

Expected: hits only in `modules/world/world_script_queue.go:55` (the comment we're retiring) and the spec/plan/follow-ups under `docs/` + `.claude/`. Note the production hit count — if more production hits exist, list them in the commit body.

#### Step 3.2: Wrap the fire site in `processWorldQueue`

**Replace** the comment block at `world_script_queue.go:38-86` (the function header comment + body):

```go
// processWorldQueue drains ready entries from s.worldScriptQueue,
// firing each by calling script.Execute (via resumeOrFinishWorld) and
// dispatching the post-execute state.
//
// Iteration uses index-based slice walk with mid-pass append visibility
// (re-reads len(s.worldScriptQueue) each loop iteration) — this
// preserves the same TS-authentic "speedup quirk" already present
// in processPlayerQueue (tick.go:222) where a script that re-enqueues
// itself or another script during Execute will see the new entry
// processed in the same tick.
//
// Removal happens BEFORE firing (matching processPlayerQueue:243-249)
// so a re-entrant Execute that calls EnqueueWorldScript doesn't
// collide with the index pointer.
//
// Mirrors TS World.processWorld world-queue iteration at World.ts:534-559.
//
// DEVIATION NAI-37-D-WORLDQUEUE-NO-PANIC-RECOVERY: TS wraps the
// world-queue iteration body in try/catch (World.ts:557-559) to
// swallow per-script panics. goscape leaves panics to propagate up
// the tick goroutine — closure when the project adopts a tick-wide
// panic-recovery convention.
func (s *Server) processWorldQueue() {
	i := 0
	for i < len(s.worldScriptQueue) {
		entry := &s.worldScriptQueue[i]
		// POST-decrement: capture current, then decrement. Mirrors TS
		// World.processWorld at World.ts:535 (`const delay = request.delay--`).
		// With delay=delay+1 stored at enqueue, this means user world_delay N
		// fires on the (N+2)-th processWorldQueue call after suspend (matching TS).
		delay := entry.delay
		entry.delay--
		if delay > 0 {
			i++
			continue
		}
		state := entry.script
		s.worldScriptQueue = append(s.worldScriptQueue[:i], s.worldScriptQueue[i+1:]...)
		// Reset Execution=Running so script.Execute resumes the loop
		// from the post-WORLD_DELAY PC. Mirrors the player-path resume
		// convention at tick.go:211. TS ScriptRunner.execute resets
		// internally (ScriptRunner.ts:130); goscape leaves the reset to
		// callers, matching processActiveScripts.
		state.Execution = script.Running
		s.resumeOrFinishWorld(state)
		// Don't advance i: we just removed the current element, so i
		// now points to what was the next element (or past end).
	}
}
```

with:

```go
// processWorldQueue drains ready entries from s.worldScriptQueue,
// firing each by calling script.Execute (via resumeOrFinishWorld) and
// dispatching the post-execute state.
//
// Iteration uses index-based slice walk with mid-pass append visibility
// (re-reads len(s.worldScriptQueue) each loop iteration) — this
// preserves the same TS-authentic "speedup quirk" already present
// in processPlayerQueue (tick.go:222) where a script that re-enqueues
// itself or another script during Execute will see the new entry
// processed in the same tick.
//
// Removal happens BEFORE firing (matching processPlayerQueue:243-249)
// so a re-entrant Execute that calls EnqueueWorldScript doesn't
// collide with the index pointer. This ordering also matters for
// panic recovery: a panicking script has already been removed from
// the queue when recoverWorldScript fires, so the next iteration sees
// the next entry (NAI-42).
//
// Mirrors TS World.processWorld world-queue iteration at World.ts:534-559,
// including the per-iteration try/catch (NAI-42; closes
// NAI-37-D-WORLDQUEUE-NO-PANIC-RECOVERY).
func (s *Server) processWorldQueue() {
	i := 0
	for i < len(s.worldScriptQueue) {
		entry := &s.worldScriptQueue[i]
		// POST-decrement: capture current, then decrement. Mirrors TS
		// World.processWorld at World.ts:535 (`const delay = request.delay--`).
		// With delay=delay+1 stored at enqueue, this means user world_delay N
		// fires on the (N+2)-th processWorldQueue call after suspend (matching TS).
		delay := entry.delay
		entry.delay--
		if delay > 0 {
			i++
			continue
		}
		state := entry.script
		s.worldScriptQueue = append(s.worldScriptQueue[:i], s.worldScriptQueue[i+1:]...)
		// Reset Execution=Running so script.Execute resumes the loop
		// from the post-WORLD_DELAY PC. Mirrors the player-path resume
		// convention at tick.go:211. TS ScriptRunner.execute resets
		// internally (ScriptRunner.ts:130); goscape leaves the reset to
		// callers, matching processActiveScripts.
		state.Execution = script.Running
		func(state *script.ScriptState) {
			defer recoverWorldScript(state, s.log)
			s.resumeOrFinishWorld(state)
		}(state)
		// Don't advance i: we just removed the current element, so i
		// now points to what was the next element (or past end).
	}
}
```

#### Step 3.3: Run existing world-queue tests

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run WorldQueue -v`

Expected: PASS. The wrapper is transparent for the existing tests (none of them inject panics).

#### Step 3.4: Run full package

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/`

Expected: PASS.

#### Step 3.5: Run full repo

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: PASS.

#### Step 3.6: Re-verify deviation tag retired

Run: `rg -n 'NAI-37-D-WORLDQUEUE-NO-PANIC-RECOVERY' .`

Expected: zero hits in `modules/`, `pkg/`, `cmd/`. Hits in `docs/superpowers/` and `.claude/projects/` are expected (spec/plan/memory references).

Per `retire_deviation_grep_all_comments.md`: enumerate every doc-comment site, not just production touch points. The retired tag should leave NO production-code hit.

#### Step 3.7: Commit

```bash
git add modules/world/world_script_queue.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-42 T3 — world-queue panic recovery

processWorldQueue wraps resumeOrFinishWorld in
`defer recoverWorldScript(state, s.log)`. A panicking script entry has
already been removed pre-fire (matches the existing remove-before-fire
ordering at world_script_queue.go:75), so the next iteration sees the
next entry cleanly. Mirrors TS World.ts:557-559.

Closes NAI-37-D-WORLDQUEUE-NO-PANIC-RECOVERY.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Final whole-impl review checklist

After T1+T2+T3 land, the controller runs a single whole-impl review (per `compressed_cadence.md` for sub-specs under ~250 LOC):

1. `rg -n 'NAI-37-D-WORLDQUEUE-NO-PANIC-RECOVERY' modules/ pkg/ cmd/` → 0 hits.
2. `rg -n 'recover\(\)' modules/world/` → only in `tick_recovery.go` and `tick_recovery_test.go` (the test's `defer recover()` watchdog).
3. `rg -n 'recoverPlayer\b' modules/world/tick.go` → 4 hits (one per wrapped site).
4. `rg -n 'recoverWorldScript\b' modules/world/world_script_queue.go` → 1 hit.
5. Run `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...` → PASS.
6. Verify the 6 unwrapped sites listed in spec §4.4 remain bare (no accidental wrap).
7. Confirm no new deviations introduced (spec §7).

## Closing commit

After whole-impl review passes:

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(world,docs): NAI-42 closed — tick-wide panic-recovery convention

Closes NAI-37-D-WORLDQUEUE-NO-PANIC-RECOVERY. Five sites wrapped
(processClientsIn, processActiveScripts, processPlayerTimers,
processInteractions, processWorldQueue); 6 sites left bare per spec
§4.4 TS-faithful asymmetry. No new deviations.

Closes memory: <memory entries seeded if any>

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```
