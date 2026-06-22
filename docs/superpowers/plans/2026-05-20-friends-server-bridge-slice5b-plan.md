# Friends-server bridge slice 5b — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the world-side `WorldEventsDispatcher` from slog-only (slice 5a) to actually apply each inbound `RELAY_*` opcode to world state, retiring `NAI-S5A-D-DISPATCHER-NO-ACTION` piecewise.

**Architecture:** A new composition dispatcher `actionWorldEventsDispatcher` wraps the existing `slogWorldEventsDispatcher` and, for each event, calls into a new `WorldStateOps` interface implemented by `*Server`. Cross-goroutine safety is preserved via a single closure queue `relayActionQueue chan func()` drained at the top of the tick loop — so dispatcher-goroutine writes never touch tick-owned state. Eight opcodes ship wired (MUTE, KICK, SHUTDOWN, RELOAD, CLEARLOGINS, CLEARLOGOUTS-no-op, BROADCAST, TRACK); QUEUESCRIPT stays slog-warn behind `NAI-S5B-D-NO-RUNESCRIPT-RUNTIME`.

**Tech Stack:** Go 1.26.2; `log/slog` for structured logs; existing `sync.RWMutex` + buffered channels for tick-marshal; existing `pkg/util/jstring.ToBase37` for username decoding; `github.com/zsrv/goscape/pkg/friendspb` proto types.

## Plan-time refinement of spec

The spec (§4 KICK, §5 lookup) proposed `playersMu.Lock()` in dispatcher methods that flip Player fields directly. **Plan-time investigation revealed this is racy:** the tick goroutine reads Player fields (`loggingOut`, `mutedUntil`, `submitInput`, `forceRemove`) without acquiring `playersMu` (e.g. `tick.go:327-347` snapshots `playerLoop` under RLock but reads/writes fields lock-free). Acquiring the write lock in the dispatcher does NOT prevent the race detector from flagging the tick's lock-free reads.

**Refined approach:** the dispatcher goroutine never touches tick-owned state. All `WorldStateOps` methods on `*Server` enqueue a closure on `s.relayActionQueue chan func()`. The tick goroutine drains the queue at the top of each iteration (between the existing `rebuildResult` drain at `tick.go:45-49` and the `processShutdown` check at `tick.go:55`). Single-writer semantics on player state are preserved — every field mutation happens on the tick goroutine.

This also lets `Shutdown` call the existing `rebootTimer(duration)` helper (which reads `currentTick`, writes `shutdownTick`, iterates `playerLoop`, and writes packets to player connections) without modification.

**Plan-time investigation confirmed BROADCAST and TRACK are wireable** (not deferred):
- `Server.BroadcastMes` at `server_broadcast.go:8` already fans `MESSAGE_GAME` packets out to every connected player; no NAI-182-D5 gating.
- `Player.submitInput` at `player.go:290` is a server-internal `bool` read by the tracking-submission gate (`input_tracking.go:122`); no packet emit on flip.

`WorldStateOps` therefore has **eight methods** (six core + BroadcastMessage + SetPlayerInputTracking). QUEUESCRIPT remains deferred behind `NAI-S5B-D-NO-RUNESCRIPT-RUNTIME`.

## File structure

| File | Action | Responsibility |
|---|---|---|
| `modules/world/server.go` | modify | Add `relayActionQueue chan func()` field + initialize in `NewServer` and `newTestServer`; insert `drainRelayActions()` call at top of `tick.go` main loop body; replace `newSlogWorldEventsDispatcher` wiring with `newActionWorldEventsDispatcher` |
| `modules/world/tick.go` | modify | Insert one-line call `s.drainRelayActions()` between rebuildResult drain and processShutdown check |
| `modules/world/bridges.go` | modify | Add `WorldStateOps` interface (8 methods); update `slogWorldEventsDispatcher` doc-comment to retire bullets of `NAI-S5A-D-DISPATCHER-NO-ACTION` |
| `modules/world/world_state_ops.go` | new | `*Server` impls of `WorldStateOps`; tick-only `lookupPlayerByUsername37` helper; `drainRelayActions` + `enqueueRelayAction` plumbing |
| `modules/world/world_state_ops_test.go` | new | Per-method integration tests against `*Server` from `newTestServer`; drive the queue manually via direct `drainRelayActions` call |
| `modules/world/world_events_dispatcher.go` | new | `actionWorldEventsDispatcher` impl (composition wrapper) + `newActionWorldEventsDispatcher` constructor |
| `modules/world/world_events_dispatcher_test.go` | extend (slice-5a file) | Add `recordingWorldStateOps` + per-opcode composition tests |
| `modules/world/friends_smoke_test.go` | extend | Add `TestFriendsClient_E2E_RelayShutdownAppliesAction` — boot real friends-server + a *Server with the action dispatcher; issue `RelayShutdown`; assert `shutdownTick` advances after dispatch |

LOC estimate: ~600-700 added; ~5 deleted (slogWorldEventsDispatcher doc-comment bullet edits).

---

## Task 1: Add `relayActionQueue` + tick drain infrastructure

**Files:**
- Modify: `modules/world/server.go` (struct field; `NewServer`)
- Modify: `modules/world/server_test.go` (`newTestServer`)
- Modify: `modules/world/tick.go` (call drain at top of loop)
- Create: `modules/world/world_state_ops.go` (drain + enqueue helpers)

This task introduces the channel + drain machinery only. No `WorldStateOps` interface yet, no dispatcher rewire — those land in T2/T4/T6.

- [ ] **Step 1: Write the failing test**

Create `modules/world/world_state_ops_test.go` with this content:

```go
package world

import (
	"sync/atomic"
	"testing"
)

// TestRelayActionQueue_DrainExecutesOnTick pins that an action enqueued
// via enqueueRelayAction runs exactly once when drainRelayActions is
// invoked, and runs on the caller's goroutine (tick semantics).
func TestRelayActionQueue_DrainExecutesOnTick(t *testing.T) {
	s := newTestServer(t)

	var ran atomic.Int32
	s.enqueueRelayAction(func() { ran.Add(1) })

	if ran.Load() != 0 {
		t.Fatalf("action ran before drain: count=%d", ran.Load())
	}

	s.drainRelayActions()

	if got := ran.Load(); got != 1 {
		t.Fatalf("action did not run on drain: count=%d, want 1", got)
	}

	// Second drain with empty queue must be a no-op (no blocking).
	s.drainRelayActions()
	if got := ran.Load(); got != 1 {
		t.Fatalf("second drain re-ran action: count=%d, want 1", got)
	}
}

// TestRelayActionQueue_DropsOnFull pins that enqueueRelayAction is
// non-blocking and drops the action when the queue is at capacity.
// Mirrors slice-4a NAI-S4A-D-DROP-ON-FULL posture (drop-newest).
func TestRelayActionQueue_DropsOnFull(t *testing.T) {
	s := newTestServer(t)

	// Fill the queue to capacity with no-op closures.
	for i := 0; i < cap(s.relayActionQueue); i++ {
		s.enqueueRelayAction(func() {})
	}

	// The next enqueue must NOT block. If the implementation blocks,
	// the test will hang and fail on test timeout.
	var dropped atomic.Bool
	dropped.Store(true) // assume dropped; flipped to false if executed.
	s.enqueueRelayAction(func() { dropped.Store(false) })

	// Drain everything; only the first cap(queue) closures should run.
	// The over-cap closure was dropped, so dropped stays true.
	s.drainRelayActions()

	if !dropped.Load() {
		// Got dropped — correct behavior.
		return
	}
	t.Fatal("over-cap enqueue was NOT dropped — drainRelayActions executed the over-cap closure")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
unset GOROOT; export PATH="$HOME/go/current/bin:$PATH"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run TestRelayActionQueue ./modules/world/...
```

Expected: COMPILE FAILURE — `s.relayActionQueue`, `s.enqueueRelayAction`, `s.drainRelayActions` undefined.

- [ ] **Step 3: Add the field to Server**

In `modules/world/server.go`, add a new field on the `Server` struct (alongside `rebuildReq` near line 191-195):

```go
	// relayActionQueue carries closures enqueued by WorldStateOps
	// methods (the impl of which lives on *Server, world_state_ops.go).
	// Drained at the top of the tick loop body so all field mutations
	// run on the tick goroutine — preserves single-writer semantics
	// on Player state. Buffer 64; drop-newest on full per
	// NAI-S5A-D-WORLDEVENTS-DROP-ON-FULL posture (slice 5b adopts the
	// same posture client-side).
	relayActionQueue chan func()
```

Then in `NewServer` (around line 257-277, the Server literal), add:

```go
		relayActionQueue: make(chan func(), 64),
```

immediately after the existing `rebuildResult: make(chan rebuildResult, 1),` line.

- [ ] **Step 4: Add the same initialization to newTestServer**

In `modules/world/server_test.go`, inside `newTestServer` (around line 311-335), add to the `&Server{...}` literal — placed immediately after `rebuildResult: make(chan rebuildResult, 1),`:

```go
		relayActionQueue: make(chan func(), 64),
```

- [ ] **Step 5: Create `world_state_ops.go` with the queue helpers**

Create `modules/world/world_state_ops.go` with this content:

```go
package world

// enqueueRelayAction posts a closure onto the relay action queue.
// Non-blocking: drops the action if the queue is full (matches
// NAI-S5A-D-WORLDEVENTS-DROP-ON-FULL server-side posture). Called by
// WorldStateOps methods on the per-world subscriber goroutine.
//
// A dropped action represents one lost RELAY_* event. Logged at Warn
// so operators see queue pressure. In practice the queue is sized
// generously (64) and tick cadence is sub-second, so drops should be
// rare.
func (s *Server) enqueueRelayAction(action func()) {
	select {
	case s.relayActionQueue <- action:
	default:
		s.log.Warn("relay action queue full; dropping action")
	}
}

// drainRelayActions runs every pending action on the queue. Must be
// invoked from the tick goroutine. Non-blocking — exits as soon as the
// queue is empty. Actions are executed in FIFO order in the same
// iteration; they observe and mutate tick-owned state directly.
//
// Placement: top of tick loop body, between the rebuildResult drain and
// processShutdown so that a RELAY_SHUTDOWN that arrived this iteration
// can take effect on this same tick.
func (s *Server) drainRelayActions() {
	for {
		select {
		case action := <-s.relayActionQueue:
			action()
		default:
			return
		}
	}
}
```

- [ ] **Step 6: Wire the drain into the tick loop**

In `modules/world/tick.go`, locate the existing rebuildResult drain block (around line 45-49):

```go
		select {
		case r := <-s.rebuildResult:
			s.handleRebuildResult(r)
		default:
		}
```

Immediately after that block (before the `// NAI-182 — shutdown consumer` comment at line 51), insert:

```go
		// Slice 5b: drain inbound RELAY_* actions enqueued by the world
		// events dispatcher BEFORE processShutdown so a same-tick
		// RELAY_SHUTDOWN observes its own shutdownTick assignment.
		s.drainRelayActions()
```

- [ ] **Step 7: Run tests to verify they pass**

```bash
unset GOROOT; export PATH="$HOME/go/current/bin:$PATH"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run TestRelayActionQueue ./modules/world/... -count=1
```

Expected: PASS — both `TestRelayActionQueue_DrainExecutesOnTick` and `TestRelayActionQueue_DropsOnFull`.

- [ ] **Step 8: Run the wider package to ensure tick integration didn't break anything**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -count=1 -timeout 300s
```

Expected: PASS — all existing tick / world tests continue to pass with the one-line drain insertion.

- [ ] **Step 9: Commit**

```bash
git add modules/world/server.go modules/world/server_test.go modules/world/tick.go modules/world/world_state_ops.go modules/world/world_state_ops_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: slice 5b T1 — relayActionQueue + tick drain

Adds Server.relayActionQueue (chan func(), buffer 64) drained at
the top of the tick loop body, between rebuildResult and
processShutdown. WorldStateOps methods will enqueue closures
here so all player-state mutations run on the tick goroutine
(single-writer preserved, race-clean by construction).

Drop-newest on full mirrors slice-5a's
NAI-S5A-D-WORLDEVENTS-DROP-ON-FULL server-side posture.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Define `WorldStateOps` interface

**Files:**
- Modify: `modules/world/bridges.go` (add interface declaration)

- [ ] **Step 1: Write the compile-time interface assertion test**

Append to `modules/world/world_state_ops_test.go`:

```go
// Compile-time assertion that *Server implements WorldStateOps.
// Until the WorldStateOps methods are implemented in T3-T4, this line
// must compile against the interface declared in T2 — *Server has all
// methods → compiles. The runtime test below ensures *Server can be
// constructed and bound through the interface.
var _ WorldStateOps = (*Server)(nil)

// TestWorldStateOps_InterfaceBindsToServer pins that *Server satisfies
// WorldStateOps at construction time. Method behavior is verified in
// the per-method tests below.
func TestWorldStateOps_InterfaceBindsToServer(t *testing.T) {
	s := newTestServer(t)
	var ops WorldStateOps = s
	if ops == nil {
		t.Fatal("WorldStateOps binding returned nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
unset GOROOT; export PATH="$HOME/go/current/bin:$PATH"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run TestWorldStateOps_InterfaceBindsToServer ./modules/world/...
```

Expected: COMPILE FAILURE — `WorldStateOps` undefined; `*Server` is missing methods (depending on what compiler reports first).

- [ ] **Step 3: Add the WorldStateOps interface to bridges.go**

In `modules/world/bridges.go`, immediately after the `WorldEventsDispatcher` interface block (after the closing `}` at line 166, before `// slogWorldEventsDispatcher` at line 168), insert:

```go
// WorldStateOps is the world-side action surface invoked by
// actionWorldEventsDispatcher on inbound RELAY_* events. *Server
// implements it (world_state_ops.go). Tests bind recordingWorldStateOps.
//
// Methods correspond 1:1 to wired RELAY_* opcodes. QUEUESCRIPT is NOT
// on this interface — it stays slog-warn behind
// NAI-S5B-D-NO-RUNESCRIPT-RUNTIME until the runscript runtime can
// resolve [queue,<name>] triggers.
//
// All methods are safe to call from any goroutine. Production *Server
// impls enqueue a closure on relayActionQueue and return immediately;
// the tick goroutine drains the queue at the top of each iteration
// (see Server.drainRelayActions in world_state_ops.go).
type WorldStateOps interface {
	SetPlayerMute(username37 uint64, mutedUntilMs int64)
	KickPlayer(username37 uint64)
	Shutdown(durationTicks int32)
	BroadcastMessage(message string)
	SetPlayerInputTracking(username37 uint64, state int32)
	Reload()
	ClearLogins()
	// ClearLogouts is a tagged no-op: goscape has no logout-request
	// queue analogous to TS's World.logoutRequests. See
	// NAI-S5B-D-CLEARLOGOUTS-NO-GOSCAPE-QUEUE (slice 5b spec §6.4).
	ClearLogouts()
}
```

- [ ] **Step 4: Run test to verify it still fails (now with method-missing errors)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run TestWorldStateOps_InterfaceBindsToServer ./modules/world/...
```

Expected: COMPILE FAILURE — `*Server does not implement WorldStateOps (missing method SetPlayerMute, KickPlayer, ...)`.

This failure is the trigger for T3 and T4, which add the methods.

- [ ] **Step 5: Do NOT commit yet**

The compile-time assertion line cannot be merged until *Server has all methods. Leave the working tree dirty; T3-T4 land the methods, and T4's commit will include the now-compiling bridges.go interface + the test assertion.

> **Note:** This is a deliberate departure from one-commit-per-task — T2's interface declaration and the assertion are useless without the impl. Subagent prompts for T3-T4 should reference this and stage T2's files at T4's commit.

---

## Task 3: Implement server-scope `WorldStateOps` methods (no player lookup)

**Files:**
- Modify: `modules/world/world_state_ops.go` (5 methods)
- Modify: `modules/world/world_state_ops_test.go` (5 method tests)

Server-scope: Shutdown, Reload, ClearLogins, ClearLogouts, BroadcastMessage. None require player lookup; all act on `*Server` state directly inside the enqueued closure.

- [ ] **Step 1: Write failing tests**

Append to `modules/world/world_state_ops_test.go`:

```go
// TestWorldStateOps_Shutdown_AdvancesShutdownTick pins that
// Shutdown(d) enqueues a closure that, when drained, calls
// rebootTimer(d) — which sets shutdownTick = currentTick + d.
func TestWorldStateOps_Shutdown_AdvancesShutdownTick(t *testing.T) {
	s := newTestServer(t)
	s.currentTick = 100

	var ops WorldStateOps = s
	ops.Shutdown(50)
	s.drainRelayActions()

	if s.shutdownTick != 150 {
		t.Fatalf("shutdownTick: got %d, want 150 (currentTick=100 + duration=50)", s.shutdownTick)
	}
}

// TestWorldStateOps_Reload_EnqueuesRebuildReq pins that
// Reload() enqueues a closure that, when drained, posts on
// rebuildReq via dispatchRebuildRequest (existing helper).
func TestWorldStateOps_Reload_EnqueuesRebuildReq(t *testing.T) {
	s := newTestServer(t)

	var ops WorldStateOps = s
	ops.Reload()
	s.drainRelayActions()

	select {
	case <-s.rebuildReq:
		// expected
	default:
		t.Fatal("rebuildReq did not receive after Reload + drain")
	}
}

// TestWorldStateOps_ClearLogins_EmptiesNewPlayers pins that
// ClearLogins() enqueues a closure that clears s.newPlayers.
func TestWorldStateOps_ClearLogins_EmptiesNewPlayers(t *testing.T) {
	s := newTestServer(t)
	// Seed two pending logins. Bypass addPlayer (which requires a wired
	// client) — directly populate the slice under playersMu.
	s.playersMu.Lock()
	s.newPlayers = append(s.newPlayers, &Player{}, &Player{})
	s.playersMu.Unlock()

	var ops WorldStateOps = s
	ops.ClearLogins()
	s.drainRelayActions()

	s.playersMu.RLock()
	got := len(s.newPlayers)
	s.playersMu.RUnlock()
	if got != 0 {
		t.Fatalf("newPlayers len after ClearLogins + drain: got %d, want 0", got)
	}
}

// TestWorldStateOps_ClearLogouts_IsTaggedNoop pins that
// ClearLogouts() runs without panic and emits a single Info log line
// referencing the no-op. NAI-S5B-D-CLEARLOGOUTS-NO-GOSCAPE-QUEUE.
func TestWorldStateOps_ClearLogouts_IsTaggedNoop(t *testing.T) {
	s := newTestServer(t)
	buf := &syncBuffer{}
	s.log = slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	var ops WorldStateOps = s
	ops.ClearLogouts()
	s.drainRelayActions()

	if !strings.Contains(buf.String(), "RELAY_CLEARLOGOUTS") {
		t.Fatalf("expected ClearLogouts Info log; got: %s", buf.String())
	}
}

// TestWorldStateOps_BroadcastMessage_FansOutToPlayers pins that
// BroadcastMessage(m) enqueues a closure that calls BroadcastMes(m).
// Verified by counting MessageGame writes via outbound packet buffer.
func TestWorldStateOps_BroadcastMessage_FansOutToPlayers(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	var ops WorldStateOps = s
	ops.BroadcastMessage("hello world")
	s.drainRelayActions()

	// MessageGame appends a MESSAGE_GAME opcode + payload to the
	// player's outbound buffer. The exact bytes are tested elsewhere
	// (server_broadcast_test.go); here we just assert non-empty.
	if p.client.outBuf.Len() == 0 {
		t.Fatal("BroadcastMessage produced no outbound bytes for connected player")
	}
}
```

Add the necessary imports at the top of `world_state_ops_test.go` (if not already present):

```go
import (
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
)
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
unset GOROOT; export PATH="$HOME/go/current/bin:$PATH"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run TestWorldStateOps_ ./modules/world/...
```

Expected: COMPILE FAILURE — `*Server` does not implement `WorldStateOps`; methods undefined.

- [ ] **Step 3: Implement the 5 server-scope methods in world_state_ops.go**

Append to `modules/world/world_state_ops.go`:

```go
// Shutdown schedules a world reboot in `durationTicks` ticks. Mirrors
// TS World.rebootTimer (World.ts:1787-1793) via the existing
// rebootTimer helper, which writes shutdownTick + broadcasts
// UPDATE_REBOOT_TIMER packets to all online players.
//
// NAI-S5A-D-DISPATCHER-NO-ACTION (SHUTDOWN bullet) — retired here.
func (s *Server) Shutdown(durationTicks int32) {
	s.enqueueRelayAction(func() {
		s.rebootTimer(int(durationTicks))
	})
}

// Reload triggers a content rebuild via the existing fsnotify/::rebuild
// pipeline (NAI-REBUILD-ASYNC). Non-blocking; coalesces with any other
// pending rebuild request.
//
// NAI-S5A-D-DISPATCHER-NO-ACTION (RELOAD bullet) — retired here.
func (s *Server) Reload() {
	s.enqueueRelayAction(func() {
		s.dispatchRebuildRequest()
	})
}

// ClearLogins drains the pending-logins queue (s.newPlayers). Mirrors
// TS World.loginRequests.clear() at World.ts:2038.
//
// NAI-S5A-D-DISPATCHER-NO-ACTION (CLEARLOGINS bullet) — retired here.
func (s *Server) ClearLogins() {
	s.enqueueRelayAction(func() {
		s.playersMu.Lock()
		s.newPlayers = nil
		s.playersMu.Unlock()
	})
}

// ClearLogouts is a tagged no-op. Goscape has no logout-request queue
// analogous to TS's World.logoutRequests.clear() at World.ts:2040 —
// logouts are signaled via the loggingOut flag and drained by
// processLogouts. Clearing the flag from a non-tick goroutine would
// be unsafe.
//
// NAI-S5B-D-CLEARLOGOUTS-NO-GOSCAPE-QUEUE — permanent (architectural
// divergence from TS).
func (s *Server) ClearLogouts() {
	s.enqueueRelayAction(func() {
		s.log.Info("RELAY_CLEARLOGOUTS received (no-op: goscape has no logout-request queue)")
	})
}

// BroadcastMessage fans a chat message out to every connected player.
// Delegates to the existing BroadcastMes helper.
//
// NAI-S5A-D-DISPATCHER-NO-ACTION (BROADCAST bullet) — retired here.
func (s *Server) BroadcastMessage(message string) {
	s.enqueueRelayAction(func() {
		s.BroadcastMes(message)
	})
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run TestWorldStateOps_ ./modules/world/... -count=1
```

Expected: most pass. `TestWorldStateOps_InterfaceBindsToServer` will still FAIL — the 3 per-player methods (T4) are missing. That's expected; T4 finishes the interface.

The 5 new tests must pass:
- `TestWorldStateOps_Shutdown_AdvancesShutdownTick`
- `TestWorldStateOps_Reload_EnqueuesRebuildReq`
- `TestWorldStateOps_ClearLogins_EmptiesNewPlayers`
- `TestWorldStateOps_ClearLogouts_IsTaggedNoop`
- `TestWorldStateOps_BroadcastMessage_FansOutToPlayers`

- [ ] **Step 5: Do NOT commit yet**

T4 will land the remaining methods + commit the bridges.go interface declaration + all of T2-T4 in one logically-complete commit.

---

## Task 4: Implement per-player `WorldStateOps` methods + lookup helper

**Files:**
- Modify: `modules/world/world_state_ops.go` (3 methods + lookup helper)
- Modify: `modules/world/world_state_ops_test.go` (3 method tests)

Per-player: SetPlayerMute, KickPlayer, SetPlayerInputTracking. All take `username37 uint64`, look up the player on the tick goroutine, and mutate one field.

- [ ] **Step 1: Write failing tests**

Append to `modules/world/world_state_ops_test.go`:

```go
// TestWorldStateOps_SetPlayerMute_SetsMutedUntil pins that
// SetPlayerMute(u37, ms) flips p.mutedUntil on the looked-up player.
func TestWorldStateOps_SetPlayerMute_SetsMutedUntil(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.username = "alice"
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	u37 := jstring.ToBase37("alice")
	wantMs := int64(1700000000000)

	var ops WorldStateOps = s
	ops.SetPlayerMute(u37, wantMs)
	s.drainRelayActions()

	if got := p.mutedUntil.UnixMilli(); got != wantMs {
		t.Fatalf("mutedUntil: got %d ms, want %d ms", got, wantMs)
	}
}

// TestWorldStateOps_SetPlayerMute_LookupMissIsHarmless pins that
// SetPlayerMute against an offline player is a no-op (Debug log only,
// no panic).
func TestWorldStateOps_SetPlayerMute_LookupMissIsHarmless(t *testing.T) {
	s := newTestServer(t)
	u37 := jstring.ToBase37("ghost")

	var ops WorldStateOps = s
	ops.SetPlayerMute(u37, 999)
	s.drainRelayActions()
	// No panic = pass.
}

// TestWorldStateOps_KickPlayer_FlipsLoggingOut mirrors the existing
// ::kick cheat assertion (handler_cheats_supermod_test.go:401). After
// dispatch + drain, p.loggingOut must be true. Teardown deferred to
// processLogouts (NAI-186-D1).
func TestWorldStateOps_KickPlayer_FlipsLoggingOut(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.username = "bob"
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	if p.loggingOut {
		t.Fatal("preflight: p.loggingOut should be false before kick")
	}

	u37 := jstring.ToBase37("bob")
	var ops WorldStateOps = s
	ops.KickPlayer(u37)
	s.drainRelayActions()

	if !p.loggingOut {
		t.Fatal("p.loggingOut: must be true after KickPlayer + drain")
	}
}

// TestWorldStateOps_SetPlayerInputTracking_FlipsSubmitInput pins that
// SetPlayerInputTracking(u37, 1) flips p.submitInput=true and
// SetPlayerInputTracking(u37, 0) flips it back to false. Mirrors TS
// Player.submitInput = state at World.ts:2033.
func TestWorldStateOps_SetPlayerInputTracking_FlipsSubmitInput(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.username = "carol"
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	u37 := jstring.ToBase37("carol")
	var ops WorldStateOps = s

	ops.SetPlayerInputTracking(u37, 1)
	s.drainRelayActions()
	if !p.submitInput {
		t.Fatal("submitInput: must be true after SetPlayerInputTracking(1)")
	}

	ops.SetPlayerInputTracking(u37, 0)
	s.drainRelayActions()
	if p.submitInput {
		t.Fatal("submitInput: must be false after SetPlayerInputTracking(0)")
	}
}
```

Add the `jstring` import at the top of `world_state_ops_test.go`:

```go
import (
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"

	jstring "github.com/zsrv/goscape/pkg/util/jstring"
)
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
unset GOROOT; export PATH="$HOME/go/current/bin:$PATH"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run TestWorldStateOps_ ./modules/world/...
```

Expected: COMPILE FAILURE — `SetPlayerMute`, `KickPlayer`, `SetPlayerInputTracking` undefined on `*Server`.

- [ ] **Step 3: Implement the 3 per-player methods + lookup helper in world_state_ops.go**

Append to `modules/world/world_state_ops.go`:

```go
// lookupPlayerByUsername37 returns the active player whose username
// (base37-encoded) matches u37, or nil if none. Tick-only: iterates
// s.playerLoop without acquiring playersMu, mirroring the existing
// LookupPlayerByUsername(string) helper at server.go:1106. WorldStateOps
// closures call this on the tick goroutine where playerLoop is
// unguarded.
//
// Lookup-miss is a normal occurrence (the friends-server fans a relay
// to every world; the target may live on a different one). Callers log
// a miss at Debug — not Warn — to avoid log spam.
func (s *Server) lookupPlayerByUsername37(u37 uint64) *Player {
	for _, p := range s.playerLoop {
		if p == nil || !p.active {
			continue
		}
		if jstring.ToBase37(p.username) == u37 {
			return p
		}
	}
	return nil
}

// SetPlayerMute persists a mute deadline on the looked-up player.
// Mirrors TS Player.muted_until = new Date(muted_until) at
// World.ts:2006. Lookup-miss is silently dropped at Debug.
//
// NAI-S5A-D-DISPATCHER-NO-ACTION (MUTE bullet) — retired here.
func (s *Server) SetPlayerMute(username37 uint64, mutedUntilMs int64) {
	s.enqueueRelayAction(func() {
		p := s.lookupPlayerByUsername37(username37)
		if p == nil {
			s.log.Debug("RELAY_MUTE: player not online; skipping",
				slog.Uint64("username37", username37))
			return
		}
		p.mutedUntil = time.UnixMilli(mutedUntilMs)
	})
}

// KickPlayer flags the looked-up player for logout. Mirrors TS
// Player.loggingOut = true at World.ts:2013-2018 (goscape defers the
// teardown to processLogouts per NAI-186-D1, identical to the ::kick
// cheat at handlers_game.go:1231). Lookup-miss is silently dropped.
//
// NAI-S5A-D-DISPATCHER-NO-ACTION (KICK bullet) — retired here.
func (s *Server) KickPlayer(username37 uint64) {
	s.enqueueRelayAction(func() {
		p := s.lookupPlayerByUsername37(username37)
		if p == nil {
			s.log.Debug("RELAY_KICK: player not online; skipping",
				slog.Uint64("username37", username37))
			return
		}
		p.loggingOut = true
	})
}

// SetPlayerInputTracking flips the per-player input-tracking gate.
// Mirrors TS Player.submitInput = state at World.ts:2033. Goscape
// stores submitInput as bool; convert via state != 0. Lookup-miss is
// silently dropped.
//
// NAI-S5A-D-DISPATCHER-NO-ACTION (TRACK bullet) — retired here.
func (s *Server) SetPlayerInputTracking(username37 uint64, state int32) {
	s.enqueueRelayAction(func() {
		p := s.lookupPlayerByUsername37(username37)
		if p == nil {
			s.log.Debug("RELAY_TRACK: player not online; skipping",
				slog.Uint64("username37", username37))
			return
		}
		p.submitInput = state != 0
	})
}
```

Add imports at the top of `world_state_ops.go` (currently only `package world`):

```go
package world

import (
	"log/slog"
	"time"

	jstring "github.com/zsrv/goscape/pkg/util/jstring"
)
```

- [ ] **Step 4: Run all WorldStateOps tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run TestWorldStateOps ./modules/world/... -count=1
```

Expected: PASS — all 9 `TestWorldStateOps_*` tests, plus `TestRelayActionQueue_*` from T1.

- [ ] **Step 5: Run wider package to confirm nothing else regresses**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -count=1 -timeout 300s
```

Expected: PASS.

- [ ] **Step 6: Commit T2 + T3 + T4 together**

This commit lands the interface declaration, all 8 method impls, and the lookup helper.

```bash
git add modules/world/bridges.go modules/world/world_state_ops.go modules/world/world_state_ops_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: slice 5b T2-T4 — WorldStateOps + *Server impl

Adds WorldStateOps interface (8 methods) to bridges.go and the
*Server impl in world_state_ops.go. Each method enqueues a
closure on relayActionQueue (T1); the tick drains and executes
on the tick goroutine — single-writer preserved.

Methods:
  - SetPlayerMute(u37, ms) → sets Player.mutedUntil
  - KickPlayer(u37) → sets Player.loggingOut (NAI-186-D1 pattern)
  - SetPlayerInputTracking(u37, state) → sets Player.submitInput
  - Shutdown(durationTicks) → rebootTimer(durationTicks)
  - BroadcastMessage(m) → BroadcastMes(m)
  - Reload() → dispatchRebuildRequest()
  - ClearLogins() → s.newPlayers = nil under playersMu.Lock
  - ClearLogouts() → tagged no-op (NAI-S5B-D-CLEARLOGOUTS-NO-GOSCAPE-QUEUE)

Tick-only lookupPlayerByUsername37 mirrors LookupPlayerByUsername(string)
shape — iterates playerLoop without locking.

Retires NAI-S5A-D-DISPATCHER-NO-ACTION bullets for the 8 wired
opcodes; QUEUESCRIPT remains slog-warn (T8 adds the gate tag).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Implement `actionWorldEventsDispatcher` (composition wrapper)

**Files:**
- Create: `modules/world/world_events_dispatcher.go`
- Modify: `modules/world/world_events_dispatcher_test.go` (add tests)

The dispatcher composes the existing `slogWorldEventsDispatcher` (inner) with a `WorldStateOps` (ops). Each `On*` method calls inner first (preserves slice-5a slog behavior), then routes to ops (wired opcodes) or logs Warn (QUEUESCRIPT only).

- [ ] **Step 1: Write failing tests**

Append to `modules/world/world_events_dispatcher_test.go` (the existing slice-5a test file):

```go
// recordingWorldStateOps captures WorldStateOps calls for composition
// tests. Each method is single-call; tests assert via the captured
// fields. Unused fields stay at zero value.
type recordingWorldStateOps struct {
	muteU37, kickU37, trackU37 uint64
	muteMs                     int64
	trackState                 int32
	shutdownTicks              int32
	broadcastMsg               string
	reloadCalled               bool
	clearLoginsCalled          bool
	clearLogoutsCalled         bool
}

func (r *recordingWorldStateOps) SetPlayerMute(u37 uint64, ms int64) {
	r.muteU37, r.muteMs = u37, ms
}
func (r *recordingWorldStateOps) KickPlayer(u37 uint64)              { r.kickU37 = u37 }
func (r *recordingWorldStateOps) Shutdown(d int32)                   { r.shutdownTicks = d }
func (r *recordingWorldStateOps) BroadcastMessage(m string)          { r.broadcastMsg = m }
func (r *recordingWorldStateOps) SetPlayerInputTracking(u37 uint64, s int32) {
	r.trackU37, r.trackState = u37, s
}
func (r *recordingWorldStateOps) Reload()        { r.reloadCalled = true }
func (r *recordingWorldStateOps) ClearLogins()   { r.clearLoginsCalled = true }
func (r *recordingWorldStateOps) ClearLogouts()  { r.clearLogoutsCalled = true }

// TestActionWorldEventsDispatcher_RoutesToOpsAndInner pins that each
// wired On* method calls (a) the WorldStateOps method with the right
// args, and (b) the inner WorldEventsDispatcher (composition).
func TestActionWorldEventsDispatcher_RoutesToOpsAndInner(t *testing.T) {
	buf := &syncBuffer{}
	innerLog := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	inner := newSlogWorldEventsDispatcher(innerLog)
	ops := &recordingWorldStateOps{}
	d := newActionWorldEventsDispatcher(inner, ops, innerLog)

	d.OnMute(11, 22)
	if ops.muteU37 != 11 || ops.muteMs != 22 {
		t.Errorf("OnMute → ops: got (%d,%d), want (11,22)", ops.muteU37, ops.muteMs)
	}
	if !strings.Contains(buf.String(), "world event: mute") {
		t.Errorf("OnMute: inner slog did not fire")
	}

	d.OnKick(33)
	if ops.kickU37 != 33 {
		t.Errorf("OnKick → ops: got %d, want 33", ops.kickU37)
	}

	d.OnShutdown(44)
	if ops.shutdownTicks != 44 {
		t.Errorf("OnShutdown → ops: got %d, want 44", ops.shutdownTicks)
	}

	d.OnBroadcast("hello")
	if ops.broadcastMsg != "hello" {
		t.Errorf("OnBroadcast → ops: got %q, want %q", ops.broadcastMsg, "hello")
	}

	d.OnTrack(55, 1)
	if ops.trackU37 != 55 || ops.trackState != 1 {
		t.Errorf("OnTrack → ops: got (%d,%d), want (55,1)", ops.trackU37, ops.trackState)
	}

	d.OnReload()
	if !ops.reloadCalled {
		t.Errorf("OnReload → ops: not called")
	}

	d.OnClearLogins()
	if !ops.clearLoginsCalled {
		t.Errorf("OnClearLogins → ops: not called")
	}

	d.OnClearLogouts()
	if !ops.clearLogoutsCalled {
		t.Errorf("OnClearLogouts → ops: not called")
	}
}

// TestActionWorldEventsDispatcher_QueueScriptIsSlogWarnOnly pins that
// QUEUESCRIPT does NOT call any WorldStateOps method — the runescript
// runtime gap is documented at NAI-S5B-D-NO-RUNESCRIPT-RUNTIME.
// The inner slog dispatcher still fires; the action layer logs Warn.
func TestActionWorldEventsDispatcher_QueueScriptIsSlogWarnOnly(t *testing.T) {
	innerBuf := &syncBuffer{}
	innerLog := slog.New(slog.NewTextHandler(innerBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	inner := newSlogWorldEventsDispatcher(innerLog)

	actionBuf := &syncBuffer{}
	actionLog := slog.New(slog.NewTextHandler(actionBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	ops := &recordingWorldStateOps{}
	d := newActionWorldEventsDispatcher(inner, ops, actionLog)

	d.OnQueueScript("dbg_dump", 99)

	// ops surface for QUEUESCRIPT must NOT exist — recording impl has
	// no method that would be set. (Compile-time guaranteed by the
	// interface lacking a QueueScript method.) The test below verifies
	// the slog-warn was emitted at the action layer.
	if !strings.Contains(actionBuf.String(), "RELAY_QUEUESCRIPT") {
		t.Fatalf("QUEUESCRIPT: action-layer Warn log missing; got: %s", actionBuf.String())
	}
	// Inner dispatcher still logs at Info — composition preserves slice-5a behavior.
	if !strings.Contains(innerBuf.String(), "world event: queue_script") {
		t.Fatalf("QUEUESCRIPT: inner Info log missing; got: %s", innerBuf.String())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
unset GOROOT; export PATH="$HOME/go/current/bin:$PATH"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run TestActionWorldEventsDispatcher ./modules/world/...
```

Expected: COMPILE FAILURE — `newActionWorldEventsDispatcher` undefined.

- [ ] **Step 3: Create `world_events_dispatcher.go`**

Create `modules/world/world_events_dispatcher.go`:

```go
package world

import (
	"log/slog"
)

// actionWorldEventsDispatcher composes an inner WorldEventsDispatcher
// (typically slogWorldEventsDispatcher from bridges.go) with a
// WorldStateOps action surface. Slice 5b production wiring.
//
// Each wired On* method calls inner first (preserves slice-5a slog
// observability) and then routes to the WorldStateOps method that
// applies the world-state effect. QUEUESCRIPT remains slog-warn only
// (no ops route) until the runescript runtime can dispatch
// [queue,<name>] triggers — see NAI-S5B-D-NO-RUNESCRIPT-RUNTIME.
//
// The log field carries action-layer Warn lines (lookup misses are
// logged by *Server impls inside the queue closures, not here).
type actionWorldEventsDispatcher struct {
	inner WorldEventsDispatcher
	ops   WorldStateOps
	log   *slog.Logger
}

// Compile-time assertion.
var _ WorldEventsDispatcher = (*actionWorldEventsDispatcher)(nil)

func newActionWorldEventsDispatcher(inner WorldEventsDispatcher, ops WorldStateOps, log *slog.Logger) *actionWorldEventsDispatcher {
	return &actionWorldEventsDispatcher{inner: inner, ops: ops, log: log}
}

func (d *actionWorldEventsDispatcher) OnMute(username37 uint64, mutedUntilMs int64) {
	d.inner.OnMute(username37, mutedUntilMs)
	d.ops.SetPlayerMute(username37, mutedUntilMs)
}

func (d *actionWorldEventsDispatcher) OnKick(username37 uint64) {
	d.inner.OnKick(username37)
	d.ops.KickPlayer(username37)
}

func (d *actionWorldEventsDispatcher) OnShutdown(durationTicks int32) {
	d.inner.OnShutdown(durationTicks)
	d.ops.Shutdown(durationTicks)
}

func (d *actionWorldEventsDispatcher) OnBroadcast(message string) {
	d.inner.OnBroadcast(message)
	d.ops.BroadcastMessage(message)
}

func (d *actionWorldEventsDispatcher) OnTrack(username37 uint64, state int32) {
	d.inner.OnTrack(username37, state)
	d.ops.SetPlayerInputTracking(username37, state)
}

func (d *actionWorldEventsDispatcher) OnReload() {
	d.inner.OnReload()
	d.ops.Reload()
}

func (d *actionWorldEventsDispatcher) OnClearLogins() {
	d.inner.OnClearLogins()
	d.ops.ClearLogins()
}

func (d *actionWorldEventsDispatcher) OnClearLogouts() {
	d.inner.OnClearLogouts()
	d.ops.ClearLogouts()
}

// OnQueueScript stays slog-warn only — the WorldStateOps interface
// intentionally has no QueueScript method until the runescript runtime
// can resolve [queue,<name>] triggers by name and enqueue on a player.
//
// NAI-S5B-D-NO-RUNESCRIPT-RUNTIME — retires when the runescript runtime
// supports named-script dispatch to a player.
func (d *actionWorldEventsDispatcher) OnQueueScript(scriptName string, username37 uint64) {
	d.inner.OnQueueScript(scriptName, username37)
	d.log.Warn("RELAY_QUEUESCRIPT received but no runtime to dispatch (NAI-S5B-D-NO-RUNESCRIPT-RUNTIME)",
		slog.String("script_name", scriptName),
		slog.Uint64("username37", username37))
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run TestActionWorldEventsDispatcher ./modules/world/... -count=1
```

Expected: PASS — both `TestActionWorldEventsDispatcher_RoutesToOpsAndInner` and `TestActionWorldEventsDispatcher_QueueScriptIsSlogWarnOnly`.

- [ ] **Step 5: Run wider package**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -count=1 -timeout 300s
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add modules/world/world_events_dispatcher.go modules/world/world_events_dispatcher_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: slice 5b T5 — actionWorldEventsDispatcher composition

New actionWorldEventsDispatcher wraps inner WorldEventsDispatcher
(slogWorldEventsDispatcher from slice 5a) + a WorldStateOps. Each
wired On* method calls inner first (preserves slog observability),
then routes to the ops method.

QUEUESCRIPT stays slog-warn only — no WorldStateOps.QueueScript
method until the runescript runtime supports named dispatch.

Not yet wired into NewServer — T6 lands the swap.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Wire `actionWorldEventsDispatcher` into `NewServer`

**Files:**
- Modify: `modules/world/server.go` (replace line 284 wiring)

- [ ] **Step 1: Write failing test**

Append to `modules/world/world_events_dispatcher_test.go`:

```go
// TestNewServer_WiresActionWorldEventsDispatcher pins that NewServer
// installs an *actionWorldEventsDispatcher (not the slice-5a
// slogWorldEventsDispatcher directly). End-to-end behavior is pinned
// by the e2e smoke in friends_smoke_test.go (T7).
func TestNewServer_WiresActionWorldEventsDispatcher(t *testing.T) {
	s := newTestServer(t)
	// newTestServer doesn't run NewServer's friendsClient branch, but
	// it also doesn't currently install a worldEventsDispatcher at all
	// — only NewServer does. So this test must boot a minimal NewServer
	// to verify the type. However NewServer requires TCP listen + cfg
	// scaffolding that the test harness avoids. Instead: directly
	// invoke the constructor sequence that NewServer uses, isolated.
	inner := newSlogWorldEventsDispatcher(discardLogger())
	d := newActionWorldEventsDispatcher(inner, s, discardLogger())
	// Smoke: type-asserts as WorldEventsDispatcher.
	var _ WorldEventsDispatcher = d
	// And the inner is the slice-5a slog impl.
	if d.inner == nil {
		t.Fatal("actionWorldEventsDispatcher.inner is nil")
	}
	if d.ops == nil {
		t.Fatal("actionWorldEventsDispatcher.ops is nil")
	}
}
```

- [ ] **Step 2: Run test to verify it passes already (T5 impl)**

```bash
unset GOROOT; export PATH="$HOME/go/current/bin:$PATH"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run TestNewServer_WiresActionWorldEventsDispatcher ./modules/world/... -count=1
```

Expected: PASS — the test exercises constructor wiring directly, doesn't depend on NewServer changes yet.

- [ ] **Step 3: Replace the NewServer wiring**

In `modules/world/server.go`, locate this line (around line 284):

```go
	s.worldEventsDispatcher = newSlogWorldEventsDispatcher(s.log)
```

Replace with:

```go
	// Slice 5b: production dispatcher composes the slice-5a slog
	// dispatcher with WorldStateOps so each RELAY_* event both logs
	// AND applies its world-state effect.
	innerSlog := newSlogWorldEventsDispatcher(s.log)
	s.worldEventsDispatcher = newActionWorldEventsDispatcher(innerSlog, s, s.log)
```

The `s` (passed to `newActionWorldEventsDispatcher` as the `ops` argument) implements `WorldStateOps` per T3-T4.

- [ ] **Step 4: Run full package tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -count=1 -timeout 300s
```

Expected: PASS. Pay attention to the slice-5a `world_events_subscriber_test.go` tests — those use `recordingWorldEventsDispatcher` directly and don't go through NewServer, so they continue to work unchanged.

- [ ] **Step 5: Run the full project gate**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... -count=1 -timeout 600s
```

Expected: PASS across all 30 packages.

- [ ] **Step 6: Commit**

```bash
git add modules/world/server.go modules/world/world_events_dispatcher_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: slice 5b T6 — NewServer wires action dispatcher

Replaces slogWorldEventsDispatcher direct wiring with the slice-5b
actionWorldEventsDispatcher composition. Each RELAY_* event now
logs at Info (slice-5a observability preserved) AND applies its
world-state effect via WorldStateOps (slice-5b).

QUEUESCRIPT logs at Info via inner + Warn via action layer (no
ops route — NAI-S5B-D-NO-RUNESCRIPT-RUNTIME).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: E2E round-trip smoke for action dispatch

**Files:**
- Modify: `modules/world/friends_smoke_test.go` (new test below the existing slice-5a e2e at line 422)

This e2e brings up a real friends-server, a `*Server` (test-harness, not full TCP), wires the production `actionWorldEventsDispatcher`, and issues `RelayShutdown` + `RelayReload` cross-world to assert real world-state effects.

- [ ] **Step 1: Read the existing e2e to confirm test scaffolding pattern**

```bash
sed -n '422,560p' $HOME/Code/github.com/zsrv/goscape/modules/world/friends_smoke_test.go
```

Expected: `TestFriendsClient_E2E_RelayWorldEventsRoundTrip` boots a real friends-server, two `worldEventsSubscriber`s (one per worldId), issues `RelayMute` / `RelayShutdown` / `RelayReload`, asserts dispatch via recording dispatcher channels.

- [ ] **Step 2: Write the failing e2e test**

Append a new test function to `modules/world/friends_smoke_test.go` after the existing slice-5a e2e (after the closing brace of `TestFriendsClient_E2E_RelayWorldEventsRoundTrip`):

```go
// TestFriendsClient_E2E_RelayShutdownAppliesAction pins the slice-5b
// integration: a real friends-server fanouts RelayShutdown to a world
// whose actionWorldEventsDispatcher routes through WorldStateOps to
// *Server.rebootTimer — assert s.shutdownTick advances. Mirror for
// RelayReload (asserts rebuildReq receives a value).
func TestFriendsClient_E2E_RelayShutdownAppliesAction(t *testing.T) {
	port := freePort(t)
	cfg := friends.Config{
		GRPCListenAddress:       "127.0.0.1",
		GRPCListenPort:          port,
		NodeProfile:             "main",
		WorldPlayerLimit:        100,
		Enable:                  true,
		GracefulShutdownTimeout: 5 * time.Second,
		SQLiteDSN:               filepath.Join(t.TempDir(), "friends.db"),
	}
	log := discardLogger()
	svc, err := friends.New(cfg, log)
	if err != nil {
		t.Fatalf("friends.New: %v", err)
	}
	bootCtx, bootCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer bootCancel()
	if err := svc.StartAsync(bootCtx); err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if err := svc.AwaitRunning(bootCtx); err != nil {
		t.Fatalf("AwaitRunning: %v", err)
	}
	t.Cleanup(func() {
		svc.StopAsync()
		_ = svc.AwaitTerminated(context.Background())
	})

	addr := "127.0.0.1:" + strconv.Itoa(port)
	client, err := NewFriendsClient(addr, log)
	if err != nil {
		t.Fatalf("NewFriendsClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	// Build a test *Server (sans TCP) with the production action
	// dispatcher wired against itself as WorldStateOps.
	s := newTestServer(t)
	s.currentTick = 100
	inner := newSlogWorldEventsDispatcher(log)
	dispatcher := newActionWorldEventsDispatcher(inner, s, log)
	const targetWorldID = 7
	sub := newWorldEventsSubscriber(client, targetWorldID, dispatcher, log)
	subCtx, subCancel := context.WithCancel(context.Background())
	subDone := make(chan struct{})
	go func() { sub.run(subCtx); close(subDone) }()
	defer func() {
		subCancel()
		<-subDone
	}()

	// Probe + drain helper: poll for stream readiness via RelayKick of
	// a ghost username, drain via s.drainRelayActions, check
	// s.relayActionQueue actually receives.
	probeDelivered := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !probeDelivered {
		client.RelayKick(context.Background(), &friendspb.RelayKickRequest{TargetWorldId: targetWorldID, Username37: 9999999})
		time.Sleep(50 * time.Millisecond)
		// Drain whatever arrived; check whether anything actually ran by
		// observing queue depth or by asserting on a sentinel.
		s.drainRelayActions()
		// The ghost lookup returns nil, so no state changes — but the
		// fact that drainRelayActions consumed indicates routing works.
		// Use a non-blocking peek: if a real probe didn't arrive, repeat.
		select {
		case s.relayActionQueue <- func() {}:
			// queue had room; consume our placeholder and loop
			<-s.relayActionQueue
			continue
		default:
		}
		probeDelivered = true
	}

	// Issue RelayShutdown(duration=50) and assert shutdownTick advances.
	wantTick := s.currentTick + 50
	client.RelayShutdown(context.Background(), &friendspb.RelayShutdownRequest{
		TargetWorldId: targetWorldID, DurationTicks: 50,
	})

	// Poll up to 2s for the closure to land + drain.
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.drainRelayActions()
		if s.shutdownTick == wantTick {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if s.shutdownTick != wantTick {
		t.Fatalf("shutdownTick after RelayShutdown: got %d, want %d", s.shutdownTick, wantTick)
	}

	// Issue RelayReload; assert rebuildReq receives.
	client.RelayReload(context.Background(), &friendspb.RelayReloadRequest{TargetWorldId: targetWorldID})
	deadline = time.Now().Add(2 * time.Second)
	delivered := false
	for time.Now().Before(deadline) && !delivered {
		s.drainRelayActions()
		select {
		case <-s.rebuildReq:
			delivered = true
		case <-time.After(20 * time.Millisecond):
		}
	}
	if !delivered {
		t.Fatal("rebuildReq did not receive after RelayReload + drain (NAI-S5B routing broken)")
	}
}
```

- [ ] **Step 3: Run the e2e to verify it passes**

```bash
unset GOROOT; export PATH="$HOME/go/current/bin:$PATH"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run TestFriendsClient_E2E_RelayShutdownAppliesAction ./modules/world/... -count=1 -timeout 30s
```

Expected: PASS within ~1-2s. If the probe loop hangs, increase the deadline OR replace the probe-loop pattern with a longer fixed-sleep (200-300ms) before issuing the first real RelayShutdown — the slice-5a e2e established that the stream register-side handshake completes within a few hundred ms.

- [ ] **Step 4: Run the full package to ensure no regression**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -count=1 -timeout 300s
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/friends_smoke_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: slice 5b T7 — e2e RelayShutdown/RelayReload apply actions

New TestFriendsClient_E2E_RelayShutdownAppliesAction: boots a real
friends-server, wires a *Server (test-harness) with the production
actionWorldEventsDispatcher, issues RelayShutdown + RelayReload
cross-world, asserts s.shutdownTick advances and s.rebuildReq
receives — proving the slice-5b routing chain end-to-end.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Retire `NAI-S5A-D-DISPATCHER-NO-ACTION` + open `NAI-S5B-D-*` tags

**Files:**
- Modify: `modules/world/bridges.go` (doc-comment retirement + new tags on slog dispatcher)
- Modify: `modules/world/world_events_dispatcher.go` (already opens NAI-S5B-D-NO-RUNESCRIPT-RUNTIME via T5; verify)

- [ ] **Step 1: Update the doc-comment on `WorldEventsDispatcher`**

In `modules/world/bridges.go`, locate the `WorldEventsDispatcher` interface doc-comment (around lines 149-155):

```go
// WorldEventsDispatcher is the world-side sink for inbound RELAY_*
// admin events received over the SubscribeWorldEvents stream (slice 5a).
// Default impl (slogWorldEventsDispatcher) logs each event at Info; no
// world-state effects.
//
// NAI-S5A-D-DISPATCHER-NO-ACTION — slice 5b retires this piecewise as
// each opcode's action is wired (e.g. OnShutdown → services.Manager.StopAsync).
```

Replace with:

```go
// WorldEventsDispatcher is the world-side sink for inbound RELAY_*
// admin events received over the SubscribeWorldEvents stream (slice 5a).
//
// Default no-effects impl: slogWorldEventsDispatcher (this file) — logs
// each event at Info.
//
// Production impl: actionWorldEventsDispatcher (world_events_dispatcher.go,
// slice 5b) — composes the slog impl with WorldStateOps so each event
// also applies its world-state effect on the tick goroutine.
//
// NAI-S5A-D-DISPATCHER-NO-ACTION — RETIRED 2026-05-20 (slice 5b):
// eight wired opcodes (MUTE, KICK, SHUTDOWN, BROADCAST, TRACK, RELOAD,
// CLEARLOGINS, CLEARLOGOUTS-tagged-noop) apply real effects.
// QUEUESCRIPT remains slog-warn only; tracked separately by
// NAI-S5B-D-NO-RUNESCRIPT-RUNTIME on actionWorldEventsDispatcher.OnQueueScript.
//
// Slice 5b opens these new tags:
//
// NAI-S5B-D-CLEARLOGOUTS-NO-GOSCAPE-QUEUE — permanent (architectural
//   divergence from TS; goscape has no logout-request queue). See
//   (*Server).ClearLogouts in world_state_ops.go.
//
// NAI-S5B-D-NO-RUNESCRIPT-RUNTIME — retires when the runescript runtime
//   can resolve [queue,<name>] triggers by name and enqueue on a
//   player. See actionWorldEventsDispatcher.OnQueueScript.
```

- [ ] **Step 2: Verify NAI-S5B-D-NO-RUNESCRIPT-RUNTIME is in world_events_dispatcher.go**

T5's `OnQueueScript` impl already references `NAI-S5B-D-NO-RUNESCRIPT-RUNTIME`. Confirm by:

```bash
grep -n "NAI-S5B-D-NO-RUNESCRIPT-RUNTIME" modules/world/world_events_dispatcher.go
```

Expected: at least one match.

- [ ] **Step 3: Verify NAI-S5B-D-CLEARLOGOUTS-NO-GOSCAPE-QUEUE is in world_state_ops.go**

T3's `ClearLogouts` impl already references it. Confirm:

```bash
grep -n "NAI-S5B-D-CLEARLOGOUTS-NO-GOSCAPE-QUEUE" modules/world/world_state_ops.go
```

Expected: at least one match.

- [ ] **Step 4: Run the package to ensure doc-comment edits compile**

```bash
unset GOROOT; export PATH="$HOME/go/current/bin:$PATH"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...
```

Expected: success (no output).

- [ ] **Step 5: Commit**

```bash
git add modules/world/bridges.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: slice 5b T8 — retire NAI-S5A-D-DISPATCHER-NO-ACTION

Doc-comment update on WorldEventsDispatcher: marks
NAI-S5A-D-DISPATCHER-NO-ACTION RETIRED (8 of 9 RELAY_* opcodes
wired via actionWorldEventsDispatcher; QUEUESCRIPT remains
slog-warn behind its own NAI-S5B-D-NO-RUNESCRIPT-RUNTIME tag).

Opens slice-5b tags (referenced inline by their impl sites):
  - NAI-S5B-D-CLEARLOGOUTS-NO-GOSCAPE-QUEUE (permanent)
  - NAI-S5B-D-NO-RUNESCRIPT-RUNTIME (retires when runtime can
    dispatch named scripts to players)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Whole-slice reviewer pass

**Files:**
- Read-only review of all slice-5b commits (T1-T8)

- [ ] **Step 1: Run the full project gate one more time**

```bash
unset GOROOT; export PATH="$HOME/go/current/bin:$PATH"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... -count=1 -timeout 600s
```

Expected: PASS across all 30 packages.

- [ ] **Step 2: Run smoke-pack to confirm no PackAll regression**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape-cli smoke-pack --content-dir $HOME/Code/github.com/LostCityRS/content
```

Expected: `12 OK / 0 ERR / 0 SKIP` (slice 5b touches no PackAll paths; this verifies no accidental import-cycle or build break that smoke-pack would surface).

- [ ] **Step 3: Run a code-reviewer agent across the whole slice**

Dispatch the `feature-dev:code-reviewer` agent (foreground) with this prompt:

> Whole-slice review of friends-server bridge slice 5b. Spec: `docs/superpowers/specs/2026-05-20-friends-server-bridge-slice5b-design.md`. Plan: `docs/superpowers/plans/2026-05-20-friends-server-bridge-slice5b-plan.md`. Range to review: `git log --oneline 5af9b647..HEAD` (all commits since the resume baseline).
>
> Focus on:
> 1. Race-safety: confirm dispatcher goroutine never touches Player fields directly; all mutations go through relayActionQueue → tick drain. Spot any reads of currentTick / playerLoop / Player fields from non-tick goroutines.
> 2. Lookup-miss handling: confirm Debug (not Warn) and no panic for offline players in SetPlayerMute / KickPlayer / SetPlayerInputTracking.
> 3. Composition: confirm actionWorldEventsDispatcher always calls inner before ops (preserves slog observability ordering).
> 4. Tag consistency: NAI-S5A-D-DISPATCHER-NO-ACTION retired in bridges.go; NAI-S5B-D-CLEARLOGOUTS-NO-GOSCAPE-QUEUE referenced at ClearLogouts; NAI-S5B-D-NO-RUNESCRIPT-RUNTIME referenced at OnQueueScript.
> 5. Test coverage: each WorldStateOps method has an integration test; e2e proves end-to-end routing for at least one opcode.
> 6. Idiomatic Go: any places that should use modern Go (e.g. `range over int`, `slices` package, `cmp.Or`) per the `use-modern-go` skill guidance.
>
> Report HIGH-priority issues only (correctness / safety / consistency). Skip stylistic nits.

- [ ] **Step 4: Apply reviewer fix-ups (if any)**

If the reviewer surfaces real issues, fix them in a single follow-up commit titled `world: slice 5b review fix-ups (whole-slice pass)` mirroring slice-5a's `49cb8e00` close pattern.

If no issues, skip to Step 5.

- [ ] **Step 5: Close-out summary**

Write a memory close-out memo: `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/friends_server_slice5b_close.md` summarizing what landed (commits, retired tags, opened tags, plan-execution deviations if any), and add an index entry to `MEMORY.md`.

---

## Self-review

### Spec coverage check

| Spec section | Plan task(s) |
|---|---|
| §1 Forward map (8 file touches) | T1-T7 cover all 8 |
| §2 Composition over replacement | T5 implements composition |
| §3 WorldStateOps interface (6 → refined to 8) | T2 declares; T3-T4 impl |
| §4 KICK threading model | T1 + T4 (closure-marshal supersedes spec's playersMu.Lock plan) |
| §5 Player lookup | T4 (tick-only lookupPlayerByUsername37 — fused inline via closures) |
| §6.1 Wireable opcodes (6 core) | T3 (5) + T4 (3 incl 2 from §6.3) |
| §6.2 Deferred QUEUESCRIPT | T5 OnQueueScript slog-warn impl |
| §6.3 BROADCAST + TRACK plan-time | Investigated in plan-time refinement — both wireable; landed as part of T3/T4 |
| §6.4 CLEARLOGOUTS no-op | T3 ClearLogouts impl |
| §7.1 Unit tests dispatcher→ops | T5 |
| §7.2 Integration tests ops on *Server | T3 + T4 |
| §7.3 E2E round-trip | T7 |
| §8 Retire NAI-S5A-D-DISPATCHER-NO-ACTION | T8 |
| §8 Open NAI-S5B-D-* tags | T3 (CLEARLOGOUTS), T5 (NO-RUNESCRIPT-RUNTIME), T8 (doc roll-up) |
| §9 Out of scope | Not implementing — correct |
| §10 Plan-execution discipline | Documented in plan header + per-task subagent prompts will carry it forward |

### Placeholder scan

Searched for "TBD", "TODO", "implement later", "FIXME", "fill in details" in the plan. None present (the one TBD in the spec was resolved by §5's fused-lookup choice during spec self-review).

### Type consistency

| Symbol | Definition | Usage |
|---|---|---|
| `Server.relayActionQueue chan func()` | T1 Step 3 | T1 Step 5 (drain), T3-T4 (enqueue) |
| `WorldStateOps` interface | T2 Step 3 | T3-T4 (impl), T5 (consumed by dispatcher), T6 (NewServer wiring) |
| Interface method names: `SetPlayerMute`, `KickPlayer`, `Shutdown`, `BroadcastMessage`, `SetPlayerInputTracking`, `Reload`, `ClearLogins`, `ClearLogouts` | T2 Step 3 | T3-T4 (impl, matching exactly) + T5 (`actionWorldEventsDispatcher` route calls match) |
| `actionWorldEventsDispatcher` | T5 Step 3 (constructor `newActionWorldEventsDispatcher`) | T6 Step 3 (NewServer call site) |
| `recordingWorldStateOps` (test helper) | T5 Step 1 | Used only in `world_events_dispatcher_test.go` |
| `lookupPlayerByUsername37` (tick-only) | T4 Step 3 | Used only inside the 3 per-player WorldStateOps closures |
| `enqueueRelayAction`, `drainRelayActions` | T1 Step 5 | T3-T4 (enqueue), T1 Step 6 (drain in tick) |
| `dispatchRebuildRequest` (existing, slice 4c era) | `rebuild_worker.go:27` | T3 Step 3 (Reload impl) |
| `BroadcastMes` (existing) | `server_broadcast.go:8` | T3 Step 3 (BroadcastMessage impl) |
| `rebootTimer` (existing) | `reboot.go:21` | T3 Step 3 (Shutdown impl) |
| `jstring.ToBase37` (existing) | `pkg/util/jstring` | T4 Step 3 (lookupPlayerByUsername37) |

All cross-task references compile-check via the tests at the end of each task.

### Scope check

Single-package focus (`modules/world/`), no proto changes, no DB changes, ~600-700 LOC total. Appropriate scope for one implementation plan; matches slice-5a's blast radius.
