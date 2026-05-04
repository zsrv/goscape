# NAI-82: P_ARRIVEDELAY Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire opcode 2068 (`P_ARRIVEDELAY`) dispatch to a new `handlePArriveDelay` that suspends the script for 1 tick when the active player has moved within the past 2 ticks; lay TS-faithful `lastMovement` entity state on both `*Player` (with read accessor) and `*Npc` (write only — accessor deferred until the NPC-side opcode that reads it ships).

**Architecture:** Single feature touching `pkg/script` (interface + handler + dispatch + mock conformance + handler tests) and `modules/world` (Player field + accessor + write site + Npc field + write site + entity-side write-site tests). All changes additive — no existing tests should fail. The NPC-side accessor is intentionally NOT added in NAI-82 (no consumer; YAGNI per spec §6.1).

**Tech Stack:** Go 1.26+ (per `go_version.md`). Run all `go` commands with `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` prefix per the project CLAUDE.md.

**Spec:** `docs/superpowers/specs/2026-05-03-nai-82-p-arrivedelay-port-design.md` at HEAD.

---

## File Structure

| File | Change | Responsibility |
|---|---|---|
| `pkg/script/active.go` | Modify | Add `LastMovement() int` to `ActivePlayer` interface |
| `pkg/script/handlers.go` | Modify | Add `handlePArriveDelay` function + `OpPArriveDelay` dispatch entry |
| `pkg/script/handlers_test.go` | Modify | Add 6 handler tests covering all branches + boundaries |
| `pkg/script/runner_test.go` | Modify | Extend `mockPlayer` with `lastMovement int` field + `LastMovement() int` method |
| `modules/world/player.go` | Modify | Add `lastMovement int` to `*Player` field cluster |
| `modules/world/player_script.go` | Modify | Add `(p *Player) LastMovement() int` accessor |
| `modules/world/movement.go` | Modify | Add lastMovement write at tail of `resolveMovement` when `stepsTaken > 0` |
| `modules/world/movement_test.go` | Modify | Add 2 tests pinning the write site (step + idle) |
| `modules/world/npc.go` | Modify | Add `lastMovement int` to `*Npc` field cluster |
| `modules/world/npc_interaction.go` | Modify | Replace bare `return true` at tail of `updateMovement` with moved-check + write site |
| `modules/world/npc_movement_test.go` | **Create** | New file holding the 2 NPC write-site tests (avoids reorganising the existing `npc_interaction_test.go`) |

Total: 11 files (1 new, 10 modified).

---

## Task 1: Plumb `lastMovement` field + accessor + interface extension

**Files:**
- Modify: `pkg/script/active.go` (add interface method)
- Modify: `pkg/script/runner_test.go:97-318` (extend `mockPlayer` struct + add method)
- Modify: `modules/world/player.go:78-100` (add field to movement-state cluster)
- Modify: `modules/world/player_script.go:174-181` (add accessor next to `Playtime` / `LastItem`)
- Modify: `modules/world/npc.go:60-90` (add field to NPC field cluster)

This task lands all the type plumbing in one commit. No behavioral change. After this task, the project still compiles, all existing tests still pass, no new tests yet.

- [ ] **Step 1.1: Add `LastMovement()` to `ActivePlayer` interface**

In `pkg/script/active.go`, find the `SetDelayed` block (lines 10-13):

```go
	// SetDelayed marks the active player as suspended for `ticks` more
	// ticks starting next tick. Implementation must compute
	// resumeTick = currentTick + 1 + ticks.
	SetDelayed(ticks int)
```

Immediately after the `SetDelayed(ticks int)` line and its blank line, insert:

```go
	// LastMovement returns the absolute tick value stored on the player's
	// lastMovement field. The field is written to currentTick + 1 at the
	// end of any tick in which the player actually advanced (stepsTaken > 0),
	// matching TS Player.processMovement at Engine-TS/.../Player.ts:675-677.
	//
	// Consumed by P_ARRIVEDELAY (PlayerOps.ts:359), which suspends the
	// active script when the player moved within the past 2 ticks
	// (lastMovement >= currentTick) and is a no-op otherwise.
	//
	// Returns 0 when the player has never moved (zero-value of the field).
	LastMovement() int
```

- [ ] **Step 1.2: Verify the build now fails on missing implementations**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```
Expected: build fails with errors of the form
`*Player does not implement ... (missing method LastMovement)` and
`*mockPlayer does not implement ... (missing method LastMovement)`.

This is the failing-test state for the interface addition: any consumer of `ActivePlayer` that doesn't add `LastMovement()` will fail to compile.

- [ ] **Step 1.3: Add `lastMovement int` field to `mockPlayer`**

In `pkg/script/runner_test.go` find the `mockPlayer` struct (starts around line 97). Locate the `setDelayedCalls []int` line in the "S4: captured calls from the suspension + queue methods" cluster (line 109):

```go
	// S4: captured calls from the suspension + queue methods.
	setDelayedCalls []int
```

Immediately before that line (so the new field sits with the suspension cluster), insert:

```go
	// NAI-82: seeded by handler tests to drive P_ARRIVEDELAY's gate.
	lastMovement int

```

(Leave a blank line between `lastMovement int` and the `// S4:` comment so the visual cluster boundaries stay clean.)

- [ ] **Step 1.4: Add `LastMovement()` method to `mockPlayer`**

In `pkg/script/runner_test.go`, find the `SetDelayed` method (around line 323):

```go
func (m *mockPlayer) SetDelayed(ticks int) {
	m.setDelayedCalls = append(m.setDelayedCalls, ticks)
}
```

Immediately after that function's closing brace, insert:

```go

func (m *mockPlayer) LastMovement() int { return m.lastMovement }
```

- [ ] **Step 1.5: Add `lastMovement int` field to `*Player`**

In `modules/world/player.go`, find the movement-state cluster (around lines 80-100). Locate:

```go
	lastTickX, lastTickZ, lastLevel int
	lastStepX, lastStepZ            int
```

Immediately after the `lastStepX, lastStepZ            int` line, insert:

```go
	// NAI-82: TS PathingEntity.lastMovement (Engine-TS/.../PathingEntity.ts:56).
	// Written to currentTick + 1 at end of resolveMovement when stepsTaken > 0;
	// read by P_ARRIVEDELAY's gate. Zero-value default matches TS init.
	lastMovement int
```

- [ ] **Step 1.6: Add `LastMovement()` accessor to `*Player`**

In `modules/world/player_script.go`, find the `Playtime` method (around line 174):

```go
func (p *Player) Playtime() int { return int(p.playtime) }
```

Immediately after that line, insert:

```go

// LastMovement returns the player's lastMovement field. See the
// pkg/script.ActivePlayer.LastMovement docstring for semantics.
func (p *Player) LastMovement() int { return p.lastMovement }
```

- [ ] **Step 1.7: Add `lastMovement int` field to `*Npc`**

In `modules/world/npc.go`, find the field cluster around `stepsTaken` (line 64). Locate:

```go
	stepsTaken      int
```

Immediately after that line (matching the indentation of the surrounding fields), insert:

```go
	// NAI-82: TS PathingEntity.lastMovement (Engine-TS/.../PathingEntity.ts:56).
	// Written to currentTick + 1 at end of updateMovement when position changed;
	// read by AI_ARRIVEDELAY / AI_TARGETMOVED (deferred — see NAI-82 spec §6.1).
	lastMovement int
```

- [ ] **Step 1.8: Verify the build now passes**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```
Expected: clean build.

- [ ] **Step 1.9: Run all tests as a regression check**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```
Expected: all packages PASS. The new field has zero-value default everywhere; no consumer reads it yet.

- [ ] **Step 1.10: Commit**

```bash
git add pkg/script/active.go pkg/script/runner_test.go modules/world/player.go modules/world/player_script.go modules/world/npc.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-82 T1 — plumb lastMovement field + ActivePlayer accessor

Add LastMovement() int to ActivePlayer interface, wire field +
accessor on *Player, mirror field on *Npc (accessor deferred per
spec §6.1), extend mockPlayer with field + method. Pure plumbing —
no behavioral change; subsequent tasks add write sites + handler.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Player write site (`resolveMovement` tail) + tests

**Files:**
- Modify: `modules/world/movement.go:34-68` (add write site at tail of `resolveMovement`)
- Modify: `modules/world/movement_test.go` (append 2 tests)

TDD cycle: red test → green write site → second test (idle) → all green → commit.

- [ ] **Step 2.1: Write the failing test for the step path**

In `modules/world/movement_test.go`, append at the end of the file:

```go
// NAI-82: TS Player.processMovement at Engine-TS/.../Player.ts:675-677
// writes lastMovement = World.currentTick + 1 whenever stepsTaken > 0
// after the tick's movement resolves. Read by P_ARRIVEDELAY's gate.
func TestResolveMovementWritesLastMovementOnStep(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	s.currentTick = 50

	p.x, p.z, p.level = 3094, 3106, 0
	p.moveSpeed = MoveSpeedWalk
	p.queueWaypoint(3094, 3107)

	p.resolveMovement()

	if p.stepsTaken != 1 {
		t.Fatalf("stepsTaken: got %d, want 1 (sanity — pre-existing invariant)", p.stepsTaken)
	}
	if p.lastMovement != 51 {
		t.Errorf("lastMovement: got %d, want 51 (currentTick + 1)", p.lastMovement)
	}
}
```

- [ ] **Step 2.2: Run the new test; expect FAIL**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestResolveMovementWritesLastMovementOnStep -v
```
Expected: FAIL with `lastMovement: got 0, want 51 (currentTick + 1)`.

If the test FAILS at the `stepsTaken` Fatalf instead, the gamemap on `newTestServer` is rejecting the step. Check `newTestServer` returns a server with `gamemap` allowing free travel at (3094, 3106). Adjust the waypoint to a tile the test server permits; the goal is `stepsTaken == 1`.

- [ ] **Step 2.3: Add the write site at the tail of `resolveMovement`**

In `modules/world/movement.go`, find `resolveMovement` (line 34-68). The function ends with the run-step block at lines 61-67:

```go
	if p.moveSpeed == MoveSpeedRun && p.runenergy > 0 && p.waypointIndex >= 0 {
		dir2, ok2 := p.stepOnce()
		if ok2 {
			p.runDir = int(dir2)
			p.drainRunEnergy()
		}
	}
}
```

Immediately before the closing `}` of `resolveMovement` (after the run-step block), insert:

```go

	// NAI-82: TS Player.processMovement at Engine-TS/.../Player.ts:675-677
	// writes lastMovement = World.currentTick + 1 whenever stepsTaken > 0
	// after the tick's movement resolves. The defensive client/server nil
	// guard mirrors the established stepOnce convention (movement.go:84) —
	// fixture tests that construct a bare *Player with no client get a
	// silent skip.
	if p.stepsTaken > 0 && p.client != nil && p.client.server != nil {
		p.lastMovement = p.client.server.currentTick + 1
	}
```

- [ ] **Step 2.4: Run the test; expect PASS**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestResolveMovementWritesLastMovementOnStep -v
```
Expected: PASS.

- [ ] **Step 2.5: Add the idle-path test**

In `modules/world/movement_test.go`, append after the previous test:

```go
// NAI-82: idle ticks (no waypoint) leave lastMovement untouched.
func TestResolveMovementSkipsLastMovementWhenIdle(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	s.currentTick = 50

	p.waypointIndex = -1

	p.resolveMovement()

	if p.stepsTaken != 0 {
		t.Errorf("stepsTaken: got %d, want 0 (no waypoint = no step)", p.stepsTaken)
	}
	if p.lastMovement != 0 {
		t.Errorf("lastMovement: got %d, want 0 (unchanged from zero-value)", p.lastMovement)
	}
}
```

- [ ] **Step 2.6: Run both new tests; expect PASS**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestResolveMovement(WritesLastMovementOnStep|SkipsLastMovementWhenIdle)' -v
```
Expected: both PASS.

- [ ] **Step 2.7: Run the full `modules/world` package as a regression check**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```
Expected: full PASS — no existing test should regress.

- [ ] **Step 2.8: Commit**

```bash
git add modules/world/movement.go modules/world/movement_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-82 T2 — Player.lastMovement write site at resolveMovement tail

Match TS Player.processMovement at Engine-TS/.../Player.ts:675-677:
when stepsTaken > 0 after a tick's movement resolves, set
lastMovement = currentTick + 1. Defensive client/server nil-guard
mirrors stepOnce convention (movement.go:84). Two tests pin step
+ idle paths.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: NPC write site (`updateMovement` tail) + tests

**Files:**
- Modify: `modules/world/npc_interaction.go:279-326` (replace bare `return true` at tail of `updateMovement`)
- Create: `modules/world/npc_movement_test.go` (new file holding NPC write-site tests)

- [ ] **Step 3.1: Write the failing tests in a new file**

Create `modules/world/npc_movement_test.go` with this content:

```go
package world

import "testing"

// NAI-82: TS Npc.updateMovement at Engine-TS/.../Npc.ts:362-366 writes
// lastMovement = World.currentTick + 1 when the NPC's position changed
// this tick. Read by AI_ARRIVEDELAY / AI_TARGETMOVED (deferred — see
// NAI-82 spec §6.1).
func TestNpcUpdateMovementWritesLastMovementOnStep(t *testing.T) {
	s := newTestServer(t)
	s.currentTick = 50

	n := newTestNpc(1)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.lastTickX, n.lastTickZ = n.x, n.z // mirror processMovementInteraction:162 snapshot
	n.queueWaypoint(3094, 3107)

	moved := n.updateMovement(s)

	if !moved {
		t.Fatalf("updateMovement: got false, want true (one step queued)")
	}
	if n.x != 3094 || n.z != 3107 {
		t.Fatalf("position: got (%d,%d), want (3094,3107)", n.x, n.z)
	}
	if n.lastMovement != 51 {
		t.Errorf("lastMovement: got %d, want 51 (currentTick + 1)", n.lastMovement)
	}
}

// NAI-82: stationary tick (no waypoint) leaves lastMovement untouched.
func TestNpcUpdateMovementSkipsLastMovementWhenStationary(t *testing.T) {
	s := newTestServer(t)
	s.currentTick = 50

	n := newTestNpc(1)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.lastTickX, n.lastTickZ = n.x, n.z
	n.waypointIndex = -1

	moved := n.updateMovement(s)

	if moved {
		t.Errorf("updateMovement: got true, want false (no waypoint = no step)")
	}
	if n.lastMovement != 0 {
		t.Errorf("lastMovement: got %d, want 0 (unchanged from zero-value)", n.lastMovement)
	}
}
```

- [ ] **Step 3.2: Run the new tests; expect mixed FAIL / PASS**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcUpdateMovement -v
```
Expected:
- `TestNpcUpdateMovementWritesLastMovementOnStep`: **FAIL** (lastMovement is 0, want 51).
- `TestNpcUpdateMovementSkipsLastMovementWhenStationary`: **PASS** (lastMovement stays 0 because the function early-returns false before any write).

If either of the `n.queueWaypoint` / `n.server = s` assignments fails to compile, the NPC type's exposed fields don't match the assumed shape — verify against `modules/world/npc.go` and adjust accessors. (Both `queueWaypoint` and the `server` field are referenced elsewhere in the package per existing tests.)

- [ ] **Step 3.3: Add the write site at the tail of `updateMovement`**

In `modules/world/npc_interaction.go`, find the tail of `updateMovement` (around line 322-326):

```go
	if n.moveSpeed == MoveSpeedRun && n.waypointIndex >= 0 {
		advanced2, dir2 := n.stepOnce(s)
		if advanced2 {
			n.runDir = dir2
		} else {
			n.runDir = -1
		}
	} else {
		n.runDir = -1
	}
	return true
}
```

Replace the final `return true` line (and only that line) with:

```go
	// NAI-82: TS Npc.updateMovement at Engine-TS/.../Npc.ts:362-366 writes
	// lastMovement = World.currentTick + 1 when the NPC's position changed
	// this tick. Read by AI_ARRIVEDELAY / AI_TARGETMOVED (deferred — see
	// NAI-82 spec §6.1). Position-vs-snapshot check (rather than
	// stepsTaken > 0) mirrors TS exactly. The s != nil guard handles the
	// existing test fixture pattern at npc_reorient_test.go:85 where
	// updateMovement is exercised with a nil server.
	if (n.x != n.lastTickX || n.z != n.lastTickZ) && s != nil {
		n.lastMovement = s.currentTick + 1
	}
	return true
}
```

- [ ] **Step 3.4: Run the new tests; expect both PASS**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestNpcUpdateMovement -v
```
Expected: both PASS.

- [ ] **Step 3.5: Run the full `modules/world` package as a regression check**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```
Expected: full PASS. Pay attention to `npc_reorient_test.go:85` — that test uses `npc.updateMovement` with a nil server; the new `s != nil` guard makes the new write a no-op there, so behavior is unchanged.

- [ ] **Step 3.6: Commit**

```bash
git add modules/world/npc_interaction.go modules/world/npc_movement_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-82 T3 — Npc.lastMovement write site at updateMovement tail

Match TS Npc.updateMovement at Engine-TS/.../Npc.ts:362-366: when
position changed this tick (lastTickX/Z != x/z), set
lastMovement = currentTick + 1. Position-vs-snapshot check mirrors
TS exactly. The accessor + ActiveNpc interface method are deferred
until AI_ARRIVEDELAY / AI_TARGETMOVED port (spec §6.1). s != nil
guard preserves existing nil-server test fixtures.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: `handlePArriveDelay` + dispatch + handler tests

**Files:**
- Modify: `pkg/script/handlers.go` (add handler function; add `OpPArriveDelay` dispatch entry)
- Modify: `pkg/script/handlers_test.go` (append 6 tests after the existing `TestPDelay*` cluster ending around line 410)

TDD cycle: write all 6 tests → run → all FAIL ("opcode … has no handler" via dispatch) → add handler + dispatch → all PASS → commit.

- [ ] **Step 4.1: Append the 6 handler tests to `handlers_test.go`**

In `pkg/script/handlers_test.go`, append at the end of the file (the existing `TestPDelay*` precedent cluster ends around line 410; new tests can sit anywhere convenient — append-at-EOF keeps the diff localised):

```go
// -- P_ARRIVEDELAY tests (NAI-82) ----------------------------------------
//
// TS PlayerOps.ts:357-366: if state.activePlayer.lastMovement < World.currentTick
// then return (no-op); else SetDelayed(0) + Suspended. The 2-tick window arises
// because lastMovement is written to currentTick + 1 after a moving tick.

// TestPArriveDelaySuspendsWhenMovedThisTick: lastMovement = currentTick + 1
// (the value written this tick by Player.resolveMovement).
// Gate condition: 101 < 100 is false ⇒ suspend.
func TestPArriveDelaySuspendsWhenMovedThisTick(t *testing.T) {
	mp := &mockPlayer{lastMovement: 101}
	w := &mockWorld{tick: 100}
	sf := newSingleOp("p_arrivedelay_moved_this_tick", OpPArriveDelay)
	state := Init(sf, mp, true, nil, nil)
	state.World = w

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.Execution != Suspended {
		t.Errorf("Execution: got %v, want Suspended", state.Execution)
	}
	if len(mp.setDelayedCalls) != 1 || mp.setDelayedCalls[0] != 0 {
		t.Errorf("setDelayedCalls: got %v, want [0]", mp.setDelayedCalls)
	}
}

// TestPArriveDelaySuspendsWhenMovedLastTick: lastMovement = currentTick (the
// boundary case — moved on tick T-1 means lastMovement was set to T-1+1 = T).
// Gate condition: 100 < 100 is false ⇒ suspend. Pins the inclusive boundary.
func TestPArriveDelaySuspendsWhenMovedLastTick(t *testing.T) {
	mp := &mockPlayer{lastMovement: 100}
	w := &mockWorld{tick: 100}
	sf := newSingleOp("p_arrivedelay_moved_last_tick", OpPArriveDelay)
	state := Init(sf, mp, true, nil, nil)
	state.World = w

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.Execution != Suspended {
		t.Errorf("Execution: got %v, want Suspended", state.Execution)
	}
	if len(mp.setDelayedCalls) != 1 || mp.setDelayedCalls[0] != 0 {
		t.Errorf("setDelayedCalls: got %v, want [0]", mp.setDelayedCalls)
	}
}

// TestPArriveDelayNoOpWhenMovedTwoTicksAgo: lastMovement = currentTick - 1
// (the first tick on which the gate becomes a no-op).
// Gate condition: 99 < 100 is true ⇒ return early.
func TestPArriveDelayNoOpWhenMovedTwoTicksAgo(t *testing.T) {
	mp := &mockPlayer{lastMovement: 99}
	w := &mockWorld{tick: 100}
	sf := newSingleOp("p_arrivedelay_moved_two_ticks_ago", OpPArriveDelay)
	state := Init(sf, mp, true, nil, nil)
	state.World = w

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.Execution != Finished {
		t.Errorf("Execution: got %v, want Finished (no-op should let OpReturn complete)", state.Execution)
	}
	if len(mp.setDelayedCalls) != 0 {
		t.Errorf("setDelayedCalls: got %v, want [] (no-op must not call SetDelayed)", mp.setDelayedCalls)
	}
}

// TestPArriveDelayNoOpWhenNeverMoved: lastMovement = 0 (zero-value, never
// moved). Gate condition: 0 < 100 is true ⇒ return early. Pins zero-value.
func TestPArriveDelayNoOpWhenNeverMoved(t *testing.T) {
	mp := &mockPlayer{lastMovement: 0}
	w := &mockWorld{tick: 100}
	sf := newSingleOp("p_arrivedelay_never_moved", OpPArriveDelay)
	state := Init(sf, mp, true, nil, nil)
	state.World = w

	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if state.Execution != Finished {
		t.Errorf("Execution: got %v, want Finished", state.Execution)
	}
	if len(mp.setDelayedCalls) != 0 {
		t.Errorf("setDelayedCalls: got %v, want []", mp.setDelayedCalls)
	}
}

// TestPArriveDelayUnprotectedRejected: TS uses checkedHandler(ProtectedActivePlayer);
// scripts started with protect=false must reject. Mirrors TestPDelayUnprotectedRejected.
func TestPArriveDelayUnprotectedRejected(t *testing.T) {
	mp := &mockPlayer{}
	sf := newSingleOp("p_arrivedelay_unprotected", OpPArriveDelay)
	state := Init(sf, mp, false, nil, nil) // protect=false

	err := Execute(state)
	if err == nil || err.Error() != "P_ARRIVEDELAY: script not protected" {
		t.Errorf("expected 'P_ARRIVEDELAY: script not protected', got %v", err)
	}
	if len(mp.setDelayedCalls) != 0 {
		t.Errorf("setDelayedCalls: got %v, want [] (rejection must not mutate)", mp.setDelayedCalls)
	}
}

// TestPArriveDelayRequiresActivePlayer: no Self ⇒ requireProtectedActivePlayer
// chains through requireActivePlayer's "no active player" message.
// Mirrors TestPDelayRequiresActivePlayer.
func TestPArriveDelayRequiresActivePlayer(t *testing.T) {
	sf := newSingleOp("p_arrivedelay_no_self", OpPArriveDelay)
	state := Init(sf, nil, false, nil, nil)

	err := Execute(state)
	if err == nil || err.Error() != "P_ARRIVEDELAY: no active player" {
		t.Errorf("expected 'P_ARRIVEDELAY: no active player', got %v", err)
	}
}
```

- [ ] **Step 4.2: Run the new tests; expect ALL FAIL**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestPArriveDelay -v
```
Expected: 6 tests, all FAIL with errors of the form
`script "p_arrivedelay_…": opcode 2068 has no handler at pc=0` (or similar — the exact text comes from the dispatch path's "no handler" branch). The rejection tests will fail because they expect specific error strings, but receive the "no handler" error instead.

- [ ] **Step 4.3: Add `handlePArriveDelay`**

In `pkg/script/handlers.go`, find `handlePDelay` (lines 649-670):

```go
// handlePDelay implements P_DELAY (opcode 2071): pop int n
// (NumberNotNull-checked), delay the active player by n+1 ticks, and
// suspend execution. ...
func handlePDelay(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_DELAY"); err != nil {
		return err
	}
	n := s.PopInt()
	if err := checkNotNull(n, "P_DELAY"); err != nil {
		return err
	}
	s.Self.SetDelayed(n)
	s.Execution = Suspended
	return nil
}
```

Immediately after the closing `}` of `handlePDelay`, insert:

```go

// handlePArriveDelay implements P_ARRIVEDELAY (opcode 2068): if the
// active player has moved within the past 2 ticks, mark them delayed for
// 1 tick and suspend the script; otherwise no-op. TS PlayerOps.ts:357-366.
//
// The 2-tick window arises from the TS lastMovement contract (written to
// currentTick + 1 after a moving tick): the gate accepts moves from this
// tick (lastMovement = T+1) and last tick (lastMovement = T) but rejects
// moves from 2+ ticks ago (lastMovement = T-1; T-1 < T ⇒ return).
//
// Requires ProtectedActivePlayer pointer.
func handlePArriveDelay(s *ScriptState) error {
	if err := requireProtectedActivePlayer(s, "P_ARRIVEDELAY"); err != nil {
		return err
	}
	if s.Self.LastMovement() < s.World.CurrentTick() {
		return nil
	}
	s.Self.SetDelayed(0)
	s.Execution = Suspended
	return nil
}
```

- [ ] **Step 4.4: Add `OpPArriveDelay` to the dispatch map**

In `pkg/script/handlers.go`, find the existing P_* dispatch cluster. Locate the `OpPApRange` entry at line 332:

```go
	OpPApRange: handlePApRange,
```

Immediately before that line, insert:

```go
	OpPArriveDelay: handlePArriveDelay,
```

(Lexical order among OpP* entries: `OpPAnimProtect` (line 399, separate cluster) → `OpPApRange` (332) → `OpPArriveDelay` belongs alphabetically between them. Inserting before `OpPApRange` may break strict lexical order with respect to `OpPAnimProtect`, but the cluster as a whole already isn't strictly alphabetical — this matches the established convention. If implementer prefers strict alpha, place between `OpPAnimProtect` and `OpPApRange` wherever those siblings sit; either placement is acceptable.)

- [ ] **Step 4.5: Run the handler tests; expect ALL PASS**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestPArriveDelay -v
```
Expected: 6 tests, all PASS.

- [ ] **Step 4.6: Run the full `pkg/script` package as a regression check**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```
Expected: full PASS.

- [ ] **Step 4.7: Commit**

```bash
git add pkg/script/handlers.go pkg/script/handlers_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-82 T4 — handlePArriveDelay + dispatch wiring

Wire opcode 2068 to a new handler that suspends the script when the
active player moved within the past 2 ticks (lastMovement >=
currentTick), otherwise no-ops. ProtectedActivePlayer-gated. TS
PlayerOps.ts:357-366. Six handler tests pin all branches incl.
boundary cases of the 2-tick window.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Final verification + close commit

This is the audit + close step. No new code. Confirms whole-tree green, clean vet, then retires the seed memory.

- [ ] **Step 5.1: Run the full test suite**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```
Expected: all packages PASS.

- [ ] **Step 5.2: Run vet**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```
Expected: no output (clean).

- [ ] **Step 5.3: Verify the cumulative diff matches the spec**

Run:
```bash
git --no-pager log --oneline -4
git --no-pager diff --stat HEAD~4 HEAD
```
Expected: 4 commits (T1, T2, T3, T4) covering exactly the 11 files enumerated in the File Structure table at the top of this plan. No collateral file changes.

If any unintended file appears in the cumulative diff, investigate before proceeding to close commit.

- [ ] **Step 5.4: Retire the seed memory file**

The seed memory `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai81_seed_loc_coord_p_arrivedelay.md` carried both NAI-81 and NAI-82 items. NAI-81's close commit referenced it; NAI-82 retires it. Delete the file:

```bash
rm /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai81_seed_loc_coord_p_arrivedelay.md
```

Then remove its index entry from `MEMORY.md`:

```bash
grep -n "nai81_seed" /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md
```

Use the Edit tool to delete the matching line (the entry titled "NAI-82 seed — P_ARRIVEDELAY (NAI-81 closed)").

- [ ] **Step 5.5: Stage and create the close commit (memory-only changes)**

Note: the memory directory is OUTSIDE the goscape repo, so the `rm` and `MEMORY.md` edit do NOT show up in `git status` of the goscape working tree. The close commit is purely a `git commit --allow-empty` carrying the trailer:

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
close: NAI-82 — P_ARRIVEDELAY ported; lastMovement entity state landed

Player.lastMovement field + accessor + per-tick write site, Npc-side
field + write site (accessor deferred per spec §6.1), handler +
dispatch + 6 branch tests all green. Cascade attribution
("[oploc1, _bookcase]" no-handler error silenced) pending next
NAI-N+1 user-driven smoke.

Closes memory: nai81_seed_loc_coord_p_arrivedelay.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 5.6: Confirm commit history shape**

Run:
```bash
git --no-pager log --oneline -5
```
Expected: top of log shows
- `<sha> close: NAI-82 — P_ARRIVEDELAY ported; lastMovement entity state landed`
- `<sha> feat(script): NAI-82 T4 — handlePArriveDelay + dispatch wiring`
- `<sha> feat(world): NAI-82 T3 — Npc.lastMovement write site at updateMovement tail`
- `<sha> feat(world): NAI-82 T2 — Player.lastMovement write site at resolveMovement tail`
- `<sha> feat(script): NAI-82 T1 — plumb lastMovement field + ActivePlayer accessor`

Below that should be `ab4d8ef docs(spec): NAI-82 — port P_ARRIVEDELAY opcode handler ...` and `0b3fd3e feat(script): NAI-81 — port LOC_COORD opcode handler`.

---

## Out-of-band notes for the implementer

1. **Test execution order:** Always run package-scoped tests first (`./pkg/script/` or `./modules/world/`) for fast feedback before the full `./...` regression.
2. **The `s != nil` guard at NPC tail:** Don't be tempted to remove it "for cleanliness" — `npc_reorient_test.go:85` exercises `npc.updateMovement` with a nil server and would crash without it.
3. **Mock conformance order:** Task 1 deliberately splits the interface addition from the mock additions across steps to demonstrate the failing-build state. If you prefer, all six insertions can be done back-to-back; the tests at Task 4 are what actually verify behavior.
4. **No coverage for the "moved exactly currentTick - 1 ticks ago" boundary at the entity-side write tests:** the boundary tests live on the handler-side (Task 4), where the gate arithmetic is the SUT. Entity-side tests pin the write-site invariant only. This split is intentional.
5. **Per `superpowers_clear_between_spec_and_impl.md`:** the controller dispatching this plan should `/clear` between the spec-write session and the implementation session. The implementer subagent receives this plan with no prior conversation context — every step is self-contained.

---

## Self-review

**Spec coverage:**
- Spec §4.1 (interface extension) → Task 1 step 1.1
- Spec §4.2 (Player field + accessor) → Task 1 steps 1.5-1.6
- Spec §4.3 (Player write site) → Task 2 step 2.3
- Spec §4.4 (NPC field + write site) → Task 1 step 1.7 (field) + Task 3 step 3.3 (write site)
- Spec §4.5 (handler) → Task 4 step 4.3
- Spec §4.6 (dispatch wiring) → Task 4 step 4.4
- Spec §4.7 (mock updates) → Task 1 steps 1.3-1.4
- Spec §5.1 (6 handler tests) → Task 4 step 4.1 (all 6)
- Spec §5.2 (Player write-site tests, 2) → Task 2 steps 2.1, 2.5
- Spec §5.3 (NPC write-site tests, 2) → Task 3 step 3.1
- Spec §6.2 (NAI-82-D1 deviation) → tracker entry in spec; NO code action (correctly)
- Spec §10 (close protocol with memory retirement + Closes memory trailer) → Task 5 steps 5.4-5.5

All spec sections accounted for.

**Placeholder scan:** No "TBD", "TODO", "implement later", or vague handwaving in any step. Every code block is complete and runnable.

**Type consistency:**
- `LastMovement() int` — same signature in interface (1.1), mock method (1.4), `*Player` method (1.6).
- `lastMovement int` field name — same on `mockPlayer` (1.3), `*Player` (1.5), `*Npc` (1.7).
- `handlePArriveDelay` function name — consistent in handler def (4.3) + dispatch entry (4.4) + (implicit) all 6 test names.
- `OpPArriveDelay` constant — matches `pkg/script/opcode.go:168` (verified at brainstorm).
- Error string `"P_ARRIVEDELAY: no active player"` and `"P_ARRIVEDELAY: script not protected"` — match `requireActivePlayer` / `requireProtectedActivePlayer` format strings (verified at handlers_player.go:37, :63).

No type mismatches.
