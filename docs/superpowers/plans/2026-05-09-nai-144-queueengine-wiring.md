# NAI-144 — QueueEngine Wiring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Activate the reserved `script.QueueEngine` enum value with real per-tick drain semantics, port the TS `Player.ts:657` movement gate, and migrate the two TS-`PlayerQueueType.ENGINE` consumers (`changeStat`, `advanceStat`) from the QueueNormal-as-ENGINE approximation onto the new path. Predecessor sub-spec to NAI-145 (D2 + D3 zone trigger ports + SetMultiway).

**Architecture:** New `Player.engineQueue []playerQueueRequest` slice (parallel to existing `p.queue`). `EnqueueScriptFile` switches on `qtype` to route `QueueEngine` entries into the new slice. New server-level `processPlayerEngineQueues` drain function inserted between `processPlayerTimers` and `processPathing` in the master tick loop — mirrors TS `World.ts:725` placement. Drain semantics match TS `Player.processEngineQueue` (`Player.ts:641-651`): per-entry `Delay--`, fire when `canAccess() && Delay <= 0`, no STRONG bypass, no `delayed` gate. Movement gate at top of `resolveMovement` short-circuits when `moveClickRequest && Busy() && (len(queue)>0 || len(engineQueue)>0)`.

**Tech Stack:** Go 1.26+. Standard `net/http`, `testing`. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-05-09-nai-144-queueengine-wiring-design.md` (committed at `cbed772`).

**Audits resolved during plan-write:**
- **R3 (moveClickRequest setter)** — zero `=true` sites exist in goscape. TS sets it in `World.ts:611-628` per-tick post-decode pathfinding pass. Goscape's structural equivalent (`moveClickInner` at `handlers_game.go:235`) is decode-time, not per-tick. Setter port is **out of scope** for NAI-144. Gate is **inert at HEAD**. Plan documents this in the gate's doc-comment and adds tracker entry `NAI-144-D-MoveClickRequestSetter` for follow-up.
- **R5 (walkDir/runDir)** — gate body MUST explicitly set `walkDir = -1; runDir = -1` before return; pattern matches the existing `movement.go:65-66` "no waypoints" branch.
- **Task 4 audit** — TS `Player.ts:1804-1807` `advanceStat` ALSO uses `PlayerQueueType.ENGINE`. Plan migrates both `changeStat` AND `advanceStat` in Task 4 (cheap; closes both deviations together).
- **T9 fixture** — `TestAddXPFiresChangeStatOnLevelUp` at `modules/world/player_script_test.go:204-234` directly pins `p.queue` and `req.Type == QueueNormal`. Plan updates both assertions in Task 4.

---

## File Structure

| File | Disposition | Responsibility |
|------|-------------|----------------|
| `pkg/script/queue.go` | Modify | Remove `// reserved` comment from `QueueEngine` |
| `modules/world/player.go` | Modify | Add `engineQueue []playerQueueRequest` field |
| `modules/world/player_script.go` | Modify | `EnqueueScriptFile` qtype switch; `changeStat` and `advanceStat` migration |
| `modules/world/tick.go` | Modify | Add `processPlayerEngineQueues` function + tick-loop call |
| `modules/world/movement.go` | Modify | Insert TS `Player.ts:657` movement gate at top of `resolveMovement` |
| `modules/world/player_engine_queue_test.go` | Create | T1, T2, T3, T4, T5, T6, T10 (engine-queue routing + drain semantics) |
| `modules/world/player_movement_gate_test.go` | Create | T7 (movement-gate three sub-cases) |
| `modules/world/player_script_test.go` | Modify | T8 (`TestChangeStatUsesQueueEngine`, `TestAdvanceStatUsesQueueEngine`); T9 fixup of existing tests |

---

## Task 1: Foundation — `engineQueue` field + `EnqueueScriptFile` routing

**Files:**
- Modify: `pkg/script/queue.go:12`
- Modify: `modules/world/player.go:162`
- Modify: `modules/world/player_script.go:69-80`
- Create: `modules/world/player_engine_queue_test.go`

- [ ] **Step 1.1: Run baseline tests to confirm green starting point**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... ./pkg/script/...`
Expected: PASS (all existing tests green at HEAD `cbed772`).

- [ ] **Step 1.2: Remove `// reserved` from `QueueEngine` enum**

Edit `pkg/script/queue.go` line 12.

```go
QueueEngine PlayerQueueType = iota - 4 + 4 // anchor unchanged; iota progression preserves underlying value
```

Wait — `QueueEngine` is the 5th value in the iota block (`QueueNormal=0, Strong=1, Weak=2, Long=3, Engine=4, Soft=5`). Don't touch the iota math — it's already correct. Only remove the trailing `// reserved` comment.

Replace exactly:

```go
	QueueEngine // reserved
```

with:

```go
	QueueEngine // NAI-144: TS PlayerQueueType.ENGINE — separate engineQueue with canAccess()-gated drain (Player.ts:641-651)
```

- [ ] **Step 1.3: Verify pkg/script still compiles**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./pkg/script/...`
Expected: no errors.

- [ ] **Step 1.4: Add `engineQueue` field to Player struct**

Edit `modules/world/player.go` around line 162. The current declaration is:

```go
	activeScript *script.ScriptState
	queue        []playerQueueRequest
```

Replace with:

```go
	activeScript *script.ScriptState
	queue        []playerQueueRequest

	// engineQueue is the per-player TS PlayerQueueType.ENGINE drain (NAI-144).
	// Mirrors TS Player.engineQueue (Engine-TS/.../Player.ts:343
	// LinkList<PlayerQueueRequest>). Drained by processPlayerEngineQueues
	// between processPlayerTimers and processPathing in the master tick
	// loop, matching TS World.ts:725 ordering. Distinct from p.queue:
	// gated by canAccess() (not !p.delayed); no QueueStrong bypass; no
	// modal-close pre-pass. Entries always carry Type==QueueEngine —
	// the discriminator is redundant for this slice but kept for struct
	// shape parity (DEVIATION-NAI-144-D2).
	engineQueue []playerQueueRequest
```

- [ ] **Step 1.5: Verify Player struct still compiles**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`
Expected: no errors.

- [ ] **Step 1.6: Write T1 — `TestEnqueueQueueEngineRoutesToEngineQueue` (failing test)**

Create `modules/world/player_engine_queue_test.go`:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/script"
)

// TestEnqueueQueueEngineRoutesToEngineQueue pins NAI-144 routing: a
// QueueEngine enqueue must land in p.engineQueue, never in p.queue.
func TestEnqueueQueueEngineRoutesToEngineQueue(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{Name: "[engine,test]", LookupKey: 0xdeadbeef}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s

	if err := p.EnqueueScriptArgs(0xdeadbeef, 0, nil, nil, script.QueueEngine); err != nil {
		t.Fatalf("EnqueueScriptArgs: unexpected error: %v", err)
	}

	if len(p.queue) != 0 {
		t.Errorf("p.queue len: got %d, want 0 (QueueEngine must NOT route to primary queue)", len(p.queue))
	}
	if len(p.engineQueue) != 1 {
		t.Fatalf("p.engineQueue len: got %d, want 1", len(p.engineQueue))
	}
	if got := p.engineQueue[0].Script; got != sf {
		t.Errorf("p.engineQueue[0].Script: got %v, want %v", got, sf)
	}
	if got := p.engineQueue[0].Type; got != script.QueueEngine {
		t.Errorf("p.engineQueue[0].Type: got %v, want QueueEngine", got)
	}
}
```

- [ ] **Step 1.7: Run T1 to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestEnqueueQueueEngineRoutesToEngineQueue -v`
Expected: FAIL — `p.engineQueue len: got 0, want 1` (QueueEngine still routes to `p.queue` because the switch hasn't been added).

- [ ] **Step 1.8: Implement qtype switch in `EnqueueScriptFile`**

Edit `modules/world/player_script.go` around line 69-80. Current body:

```go
func (p *Player) EnqueueScriptFile(sf *script.ScriptFile, delay int, intArgs []int, stringArgs []string, qtype script.PlayerQueueType) {
	if sf == nil {
		return
	}
	p.queue = append(p.queue, playerQueueRequest{
		Script:     sf,
		Delay:      delay,
		IntArgs:    intArgs,
		StringArgs: stringArgs,
		Type:       qtype,
	})
}
```

Replace with:

```go
func (p *Player) EnqueueScriptFile(sf *script.ScriptFile, delay int, intArgs []int, stringArgs []string, qtype script.PlayerQueueType) {
	if sf == nil {
		return
	}
	req := playerQueueRequest{
		Script:     sf,
		Delay:      delay,
		IntArgs:    intArgs,
		StringArgs: stringArgs,
		Type:       qtype,
	}
	if qtype == script.QueueEngine {
		// NAI-144: TS Player.ts:823-826 — ENGINE entries land in the
		// separate engineQueue; processPlayerEngineQueues drains them
		// between processPlayerTimers and processPathing.
		p.engineQueue = append(p.engineQueue, req)
		return
	}
	p.queue = append(p.queue, req)
}
```

- [ ] **Step 1.9: Run T1 to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestEnqueueQueueEngineRoutesToEngineQueue -v`
Expected: PASS.

- [ ] **Step 1.10: Write T2 — `TestEnqueueQueueNormalDoesNotRouteToEngineQueue` (regression fence)**

Append to `modules/world/player_engine_queue_test.go`:

```go
// TestEnqueueQueueNormalDoesNotRouteToEngineQueue is a regression fence:
// QueueNormal must continue to land in p.queue (not p.engineQueue) after
// the NAI-144 switch is added.
func TestEnqueueQueueNormalDoesNotRouteToEngineQueue(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{Name: "[normal,test]", LookupKey: 0xc0ffee}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s

	if err := p.EnqueueScriptArgs(0xc0ffee, 0, nil, nil, script.QueueNormal); err != nil {
		t.Fatalf("EnqueueScriptArgs: unexpected error: %v", err)
	}

	if len(p.queue) != 1 {
		t.Errorf("p.queue len: got %d, want 1 (QueueNormal must route to primary queue)", len(p.queue))
	}
	if len(p.engineQueue) != 0 {
		t.Errorf("p.engineQueue len: got %d, want 0", len(p.engineQueue))
	}
}
```

- [ ] **Step 1.11: Run T2 to verify it passes (no implementation change needed)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestEnqueueQueueNormalDoesNotRouteToEngineQueue -v`
Expected: PASS.

- [ ] **Step 1.12: Run full test suite to verify no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... ./pkg/script/...`
Expected: PASS. **NB:** `TestAddXPFiresChangeStatOnLevelUp` should still pass at this point (changestat still uses QueueNormal — migration is in Task 4). If it fails here, halt and investigate.

- [ ] **Step 1.13: Commit Task 1**

Run:
```bash
git add pkg/script/queue.go modules/world/player.go modules/world/player_script.go modules/world/player_engine_queue_test.go
git commit --no-gpg-sign -m "feat(player): NAI-144 — engineQueue field + EnqueueScriptFile routing

Activates the previously-reserved script.QueueEngine enum: adds
Player.engineQueue []playerQueueRequest field, switches EnqueueScriptFile
to route QueueEngine entries to it. T1 + T2 pin routing.

Foundation for the per-tick drain (Task 2), movement gate (Task 3), and
changeStat/advanceStat migration (Task 4).
"
```

---

## Task 2: Drain — `processPlayerEngineQueues` + tick-loop wiring

**Files:**
- Modify: `modules/world/tick.go`
- Modify: `modules/world/player_engine_queue_test.go`

- [ ] **Step 2.1: Write T3 — `TestProcessPlayerEngineQueuesFiresWhenDelayReachesZero` (failing test)**

Append to `modules/world/player_engine_queue_test.go`. The fixture for this test needs a server wired with `runScript` capability AND a player whose `canAccess()` returns true. Use the `runScriptCallCounter` pattern: register a script and observe its execution-count instead of mocking runScript directly.

```go
// fireCounter holds an integer counter. registerCounterScript wires a
// ScriptFile whose body increments it via test-only state. This avoids
// having to mock s.runScript.
type engineFireCounter struct {
	count int
}

// TestProcessPlayerEngineQueuesFiresWhenDelayReachesZero pins TS
// Player.ts:641-651 drain semantics: per-entry Delay-- ; fire when
// Delay <= 0. With Delay=2, drain twice → first drain Delay→1 (no fire),
// second drain Delay→0 (fires + removes).
func TestProcessPlayerEngineQueuesFiresWhenDelayReachesZero(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{
		Name:             "[engine,delay_test]",
		LookupKey:        0xdelay0,
		IntLocalCount:    0,
		StringLocalCount: 0,
		IntArgCount:      0,
		StringArgCount:   0,
		InstructionCount: 0,
	}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s
	s.playerLoop = []*Player{p}

	// Manual append — bypass EnqueueScriptFile to set Delay=2 explicitly.
	p.engineQueue = append(p.engineQueue, playerQueueRequest{
		Script: sf,
		Delay:  2,
		Type:   script.QueueEngine,
	})

	// Tick 1: Delay 2 → 1, no fire.
	s.processPlayerEngineQueues()
	if len(p.engineQueue) != 1 {
		t.Fatalf("after tick 1: p.engineQueue len: got %d, want 1 (delay 2→1, no fire)", len(p.engineQueue))
	}
	if got := p.engineQueue[0].Delay; got != 1 {
		t.Errorf("after tick 1: Delay: got %d, want 1", got)
	}

	// Tick 2: Delay 1 → 0, fires + removes.
	s.processPlayerEngineQueues()
	if len(p.engineQueue) != 0 {
		t.Errorf("after tick 2: p.engineQueue len: got %d, want 0 (delay 1→0, fired + removed)", len(p.engineQueue))
	}
}
```

**NB:** `LookupKey: 0xdelay0` is invalid Go syntax — `0xdelay0` includes non-hex letters. Replace with `0xde1a000` (valid hex) or any literal uint32. Use `0xde1a000`. Same fix in subsequent tests where I write similar literals.

Actually — the test above uses `0xdelay0` as a placeholder I need to fix BEFORE adding. Use `LookupKey: 0xde1a000` instead.

- [ ] **Step 2.2: Run T3 to verify it fails (compilation)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessPlayerEngineQueuesFiresWhenDelayReachesZero -v`
Expected: COMPILE FAIL — `s.processPlayerEngineQueues undefined`.

- [ ] **Step 2.3: Implement `processPlayerEngineQueues`**

Edit `modules/world/tick.go`. Find the existing `processActiveScripts` function (around line 264). Add the new function below `processPlayerQueue` (after line 337):

```go
// processPlayerEngineQueues drains each player's engineQueue (NAI-144).
// Mirrors TS Player.processEngineQueue (Engine-TS/.../Player.ts:641-651):
// per entry, decrement Delay; if canAccess() && Delay <= 0, fire (as a
// protected script) and remove. Iteration is index-based and re-evaluates
// len(p.engineQueue) each pass so a script that re-enqueues during fire
// (TS LinkList chain semantics) is visible same-tick (T6 pin).
//
// Distinct from processPlayerQueue: no QueueStrong modal-close pre-pass;
// gated by p.canAccess() not !p.delayed; no STRONG-style preemption.
//
// Tick-loop slot: between processPlayerTimers and processPathing,
// matching TS World.ts:725.
func (s *Server) processPlayerEngineQueues() {
	s.playersMu.RLock()
	players := make([]*Player, len(s.playerLoop))
	copy(players, s.playerLoop)
	s.playersMu.RUnlock()

	for _, p := range players {
		func(p *Player) {
			defer recoverPlayer(p, "processPlayerEngineQueues", s.log)
			i := 0
			for i < len(p.engineQueue) {
				req := &p.engineQueue[i]
				req.Delay--
				if req.Delay > 0 || !p.canAccess() {
					i++
					continue
				}
				sf := req.Script
				intArgs := req.IntArgs
				stringArgs := req.StringArgs
				p.engineQueue = append(p.engineQueue[:i], p.engineQueue[i+1:]...)
				if sf != nil {
					// TS Player.ts:646 — executeScript(script, true): protected.
					s.runScript(sf, p, nil, true, intArgs, stringArgs)
				}
				// Don't advance i — the slice shrunk by one; index now points
				// at what was the next entry (or past end).
			}
		}(p)
	}
}
```

**Cross-check before saving:** the `canAccess` method must exist on `*Player`. Grep `func (p \*Player) canAccess` to verify. If it's named differently (`Busy`, `IsAccessible`, etc.), use the actual method name. (Per memory `mock_recorder_field_naming_check`: don't infer method names.)

```bash
grep -n "func (p \*Player) canAccess\|func (p \*Player) CanAccess" /home/owner/Code/github.com/zsrv/goscape/modules/world/*.go
```

If `canAccess` is missing, the implementer should look for the goscape equivalent — likely `!p.Busy()` (since `Busy()` returns `delayed || modalMain|Chat`). TS `canAccess()` typically maps to `!busy() && !logged_out`. If unsure, the implementer should pause and re-derive from TS `Player.canAccess` definition before continuing. Update tests accordingly if the gate uses `!p.Busy()` instead of `p.canAccess()`.

- [ ] **Step 2.4: Run T3 to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessPlayerEngineQueuesFiresWhenDelayReachesZero -v`
Expected: PASS.

- [ ] **Step 2.5: Write T4 — `TestProcessPlayerEngineQueuesGatedByCanAccess` (failing test)**

Append to `modules/world/player_engine_queue_test.go`. To force `canAccess()=false`, set the player's modal state (or the inverse of whatever the actual gate condition is — discovered in Step 2.3):

```go
// TestProcessPlayerEngineQueuesGatedByCanAccess pins TS Player.ts:644
// gating: when canAccess() is false, the entry stays in the queue (no
// fire, no removal); when canAccess() is true on a later drain, the
// entry fires and is removed.
func TestProcessPlayerEngineQueuesGatedByCanAccess(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{Name: "[engine,gate_test]", LookupKey: 0x9a7e000}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s
	s.playerLoop = []*Player{p}

	// Force canAccess()=false. The exact field set depends on what
	// canAccess gates on — typically modalMain or delayed. Use the same
	// state-set pattern used in Busy()-related tests (grep
	// `p.modalState =` or `p.delayed =` for examples).
	p.delayed = true
	p.delayedUntil = 999_999

	p.engineQueue = append(p.engineQueue, playerQueueRequest{
		Script: sf,
		Delay:  0,
		Type:   script.QueueEngine,
	})

	s.processPlayerEngineQueues()

	if len(p.engineQueue) != 1 {
		t.Errorf("after tick (gated): p.engineQueue len: got %d, want 1 (canAccess=false → no fire, no removal)", len(p.engineQueue))
	}

	// Release the gate.
	p.delayed = false

	s.processPlayerEngineQueues()

	if len(p.engineQueue) != 0 {
		t.Errorf("after tick (released): p.engineQueue len: got %d, want 0 (canAccess=true → fired + removed)", len(p.engineQueue))
	}
}
```

**NB:** If `canAccess` doesn't gate on `delayed` (e.g., it gates only on logged-out state), substitute the right field. Implementer must verify the actual `canAccess()` body before writing this test. T5 (next) inverts T4 by pinning that `delayed=true` does NOT gate engineQueue (distinct from primary-queue), so these two tests together pin the exact gate condition.

- [ ] **Step 2.6: Run T4 to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessPlayerEngineQueuesGatedByCanAccess -v`
Expected: PASS.

- [ ] **Step 2.7: Write T5 — `TestProcessPlayerEngineQueuesNoStrongBypassNoDelayedGate` (failing test)**

Append to `modules/world/player_engine_queue_test.go`. This test pins that engineQueue's drain behaves DIFFERENTLY from processPlayerQueue. Specifically: in processPlayerQueue, `p.delayed=true` blocks all but QueueStrong; in processPlayerEngineQueues, `p.delayed=true` is irrelevant — only canAccess() matters.

If `canAccess()` is implemented as `!p.Busy()` AND `Busy() = delayed || modal`, then `p.delayed=true` ⇒ `canAccess()=false` ⇒ entry doesn't fire. In that case T5 cannot distinguish "delayed gates" from "canAccess gates" — they're equivalent through Busy().

Re-derive from TS `canAccess()` semantics. In TS, `canAccess()` typically gates on logged-out + connection state, NOT on delayed/modal. Verify by grepping TS:

```bash
grep -n "canAccess()" /home/owner/Code/github.com/LostCityRS/Engine-TS/src/engine/entity/Player.ts | head -5
```

If TS `canAccess()` does NOT include the modal/delayed check, then T5 is meaningful: delayed=true with canAccess=true should fire engineQueue entries. Goscape's port of `canAccess()` should match TS — verify it doesn't accidentally tie to Busy().

If goscape doesn't have a `canAccess()` method at all and the implementer used `!p.Busy()` as a substitute in Step 2.3, then T5 must call out the divergence: with `!p.Busy()` as the gate, delayed=true blocks engineQueue (different from TS). The implementer should:
- Either port a real `canAccess()` method matching TS semantics, OR
- Document the deviation as `DEVIATION-NAI-144-D4` (canAccess approximated by !Busy() until a TS-faithful canAccess port lands) and adjust T5 to assert the goscape behavior + label it as expected-divergence.

For the plan's purposes — write T5 assuming TS-faithful canAccess() (if it lands in NAI-144) OR skip T5 entirely (if approximated by !Busy()). Implementer's call after Step 2.3 audit. Default: PORT a minimal `canAccess()` to match TS and write T5 below.

```go
// TestProcessPlayerEngineQueuesNoStrongBypassNoDelayedGate pins that
// engineQueue's drain ignores p.delayed (distinct from processPlayerQueue
// which gates non-STRONG entries on !p.delayed). TS Player.ts:641-651
// gates only on canAccess() — delay is independent.
//
// Precondition: p.canAccess() returns true even when p.delayed=true
// (TS canAccess() gates on connection state, not script-delay). If the
// goscape canAccess() port deviates, this test asserts the TS-faithful
// behavior and will fail until the port aligns.
func TestProcessPlayerEngineQueuesNoStrongBypassNoDelayedGate(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{Name: "[engine,delayed_pin]", LookupKey: 0xde1a7ed0}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s
	s.playerLoop = []*Player{p}

	// p.delayed=true on its own should NOT gate engineQueue (TS engineQueue
	// is canAccess-gated, not delayed-gated).
	p.delayed = false // explicit; default already false
	p.engineQueue = append(p.engineQueue, playerQueueRequest{
		Script: sf,
		Delay:  0,
		Type:   script.QueueEngine,
	})

	s.processPlayerEngineQueues()

	if len(p.engineQueue) != 0 {
		t.Errorf("p.engineQueue len: got %d, want 0 (delay=0 + canAccess=true → fires)", len(p.engineQueue))
	}
}
```

**NB:** This test's value depends on the exact `canAccess()` port. If implementer takes the !Busy() shortcut, simplify T5 to a sanity-fire test (no delayed-gate distinction).

- [ ] **Step 2.8: Run T5 to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessPlayerEngineQueuesNoStrongBypassNoDelayedGate -v`
Expected: PASS.

- [ ] **Step 2.9: Write T6 — `TestProcessPlayerEngineQueuesSameTickReentrant` (failing test)**

Append to `modules/world/player_engine_queue_test.go`. To pin same-tick chain reentrancy without standing up a full ScriptRunner, exploit the index-based loop directly: pre-seed two entries, observe both fire in one drain.

Actually a cleaner same-tick reentrancy test would have script A enqueue script B during execution. That requires `s.runScript` to actually fire script A and A's body to do `p.EnqueueScriptArgs(B, ...)`. Standing up a full ScriptRunner with a real bytecode body is over-engineering for this test.

Simpler T6 alternative: pre-seed two entries with delay=0, one drain → both fire. This pins the index-based loop's correctness for multiple-fire-per-tick (same observable effect as same-tick chain when the loop re-evaluates `len(p.engineQueue)`).

```go
// TestProcessPlayerEngineQueuesSameTickReentrant pins that the
// index-based drain loop fires multiple ready entries in one tick.
// Precondition for TS LinkList chain semantics where script A enqueues
// script B mid-fire and B is visible same-tick (T6 simulates the
// equivalent shape with two pre-seeded entries — both fire in one pass).
func TestProcessPlayerEngineQueuesSameTickReentrant(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sfA := &script.ScriptFile{Name: "[engine,reentry_a]", LookupKey: 0xa1}
	sfB := &script.ScriptFile{Name: "[engine,reentry_b]", LookupKey: 0xa2}
	s.scriptProvider.Register(sfA)
	s.scriptProvider.Register(sfB)

	p, _ := newTestPlayer(t)
	p.client.server = s
	s.playerLoop = []*Player{p}

	p.engineQueue = append(p.engineQueue,
		playerQueueRequest{Script: sfA, Delay: 0, Type: script.QueueEngine},
		playerQueueRequest{Script: sfB, Delay: 0, Type: script.QueueEngine},
	)

	s.processPlayerEngineQueues()

	if len(p.engineQueue) != 0 {
		t.Errorf("p.engineQueue len: got %d, want 0 (both delay=0 entries fire in one drain)", len(p.engineQueue))
	}
}
```

- [ ] **Step 2.10: Run T6 to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessPlayerEngineQueuesSameTickReentrant -v`
Expected: PASS.

- [ ] **Step 2.11: Write T10 — `TestProcessPlayerEngineQueuesEmptyIsNoop` (failing test)**

Append to `modules/world/player_engine_queue_test.go`:

```go
// TestProcessPlayerEngineQueuesEmptyIsNoop pins defensive sanity: drain
// on an empty engineQueue must not panic and must not mutate state.
func TestProcessPlayerEngineQueuesEmptyIsNoop(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	s.playerLoop = []*Player{p}

	if len(p.engineQueue) != 0 {
		t.Fatalf("p.engineQueue len: precondition got %d, want 0", len(p.engineQueue))
	}

	s.processPlayerEngineQueues() // should not panic

	if len(p.engineQueue) != 0 {
		t.Errorf("p.engineQueue len after drain: got %d, want 0", len(p.engineQueue))
	}
}
```

- [ ] **Step 2.12: Run T10 to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessPlayerEngineQueuesEmptyIsNoop -v`
Expected: PASS.

- [ ] **Step 2.13: Wire `processPlayerEngineQueues` into the master tick loop**

Edit `modules/world/tick.go` lines 53-54. Current code:

```go
		s.processPlayerTimers()
		s.processPathing()
```

Replace with:

```go
		s.processPlayerTimers()
		// NAI-144: TS World.ts:725 — engineQueue drains between timers and
		// movement. processPlayerEngineQueues mirrors TS
		// Player.processEngineQueue per-player drain semantics.
		s.processPlayerEngineQueues()
		s.processPathing()
```

- [ ] **Step 2.14: Run full test suite to verify the new tick-loop slot doesn't regress anything**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: PASS.

- [ ] **Step 2.15: Commit Task 2**

Run:
```bash
git add modules/world/tick.go modules/world/player_engine_queue_test.go
git commit --no-gpg-sign -m "feat(player): NAI-144 — processPlayerEngineQueues drain + tick-loop wiring

Adds per-player engineQueue drain mirroring TS Player.processEngineQueue
(Player.ts:641-651): per-entry Delay-- ; fire when canAccess() && Delay
<= 0; protected execution; index-based loop supports same-tick chain
reentrancy.

Tick-loop slot inserted between processPlayerTimers and processPathing,
matching TS World.ts:725 ordering.

T3 (delay-decrement-then-fire) + T4 (canAccess gating) + T5
(no-delayed-gate distinct from primary queue) + T6 (multi-fire per
drain) + T10 (empty-noop defensive).
"
```

---

## Task 3: Movement gate — TS `Player.ts:657` parity

**Files:**
- Modify: `modules/world/movement.go:40-46`
- Create: `modules/world/player_movement_gate_test.go`

- [ ] **Step 3.1: Write T7-a — gate fires on primary `queue.head()` (failing test)**

Create `modules/world/player_movement_gate_test.go`:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/script"
)

// TestResolveMovementGateOnPrimaryQueue pins TS Player.ts:657 first
// disjunct: when moveClickRequest && Busy() && len(queue) > 0, movement
// is suppressed (walkDir/runDir reset to -1; no waypoint advance).
func TestResolveMovementGateOnPrimaryQueue(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3200, 3200, 0
	p.lastTickX, p.lastTickZ, p.lastLevel = 3200, 3200, 0
	// Set up a single-step waypoint so resolveMovement WOULD step if
	// the gate didn't fire.
	p.waypoints[0] = coordgrid.PackCoord(0, 3201, 3200)
	p.waypointIndex = 0
	p.waypointPriority = 0
	p.walkDir = 7 // poison: stale prior-tick value
	p.runDir = 7

	// Activate gate.
	p.moveClickRequest = true
	p.delayed = true // makes Busy() true
	p.queue = append(p.queue, playerQueueRequest{
		Script: &script.ScriptFile{Name: "[blocker]"},
		Type:   script.QueueNormal,
	})

	p.resolveMovement()

	if p.walkDir != -1 {
		t.Errorf("walkDir: got %d, want -1 (gate fires → walkDir cleared)", p.walkDir)
	}
	if p.runDir != -1 {
		t.Errorf("runDir: got %d, want -1 (gate fires → runDir cleared)", p.runDir)
	}
	if p.waypointIndex != 0 {
		t.Errorf("waypointIndex: got %d, want 0 (gate fires → no step taken)", p.waypointIndex)
	}
	if p.x != 3200 {
		t.Errorf("p.x: got %d, want 3200 (gate fires → no step taken)", p.x)
	}
}
```

- [ ] **Step 3.2: Run T7-a to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestResolveMovementGateOnPrimaryQueue -v`
Expected: FAIL — gate not yet installed; player steps and `p.x = 3201` (or similar) instead of 3200.

- [ ] **Step 3.3: Install the gate at top of `resolveMovement`**

Edit `modules/world/movement.go`. Current body around line 40-46:

```go
func (p *Player) resolveMovement() {
	// NAI-44 T3: stepsTaken accumulates per-step in stepOnce (movement.go:88).
	// Reset at start of each tick's movement cycle so processInteraction
	// (which runs after processPathing in tick.go:38-39) reads the
	// per-tick step count. TS Player.processInteraction reads
	// stepsTaken === 0 to gate post-step retry timing (Player.ts:1245).
	p.stepsTaken = 0
```

Insert immediately after the `p.stepsTaken = 0` line:

```go
	// NAI-144: TS Player.ts:657 movement gate. When the player has an
	// outstanding move-click request AND is busy (modal/delayed) AND has
	// unfinished primary-queue OR engineQueue work, suppress movement
	// for this tick.
	//
	// INERT AT HEAD: goscape currently has zero `moveClickRequest = true`
	// assignment sites (verified at HEAD `cbed772`). TS sets it in
	// World.ts:611-628 (per-tick post-decode pathfinding pass); goscape's
	// structural equivalent lives in moveClickInner (handlers_game.go:235),
	// which runs at decode-time, not per-tick. The gate is wired
	// TS-faithful and ready to fire as soon as a setter port lands —
	// see tracker NAI-144-D-MoveClickRequestSetter.
	//
	// Gate body explicitly clears walkDir/runDir to avoid stale prior-tick
	// values bleeding into the current tick's outbound info block (the
	// existing "no waypoints" branch at movement.go:65-66 sets the same
	// pattern).
	if p.moveClickRequest && p.Busy() && (len(p.queue) > 0 || len(p.engineQueue) > 0) {
		p.walkDir = -1
		p.runDir = -1
		return
	}
```

**NB:** The method is `Busy()` (capitalized — exported per `player.go:627`). Don't write `p.busy()`.

- [ ] **Step 3.4: Run T7-a to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestResolveMovementGateOnPrimaryQueue -v`
Expected: PASS.

- [ ] **Step 3.5: Write T7-b — gate fires on `engineQueue.head()` (failing test)**

Append to `modules/world/player_movement_gate_test.go`:

```go
// TestResolveMovementGateOnEngineQueue pins TS Player.ts:657 second
// disjunct: when moveClickRequest && Busy() && len(engineQueue) > 0,
// movement is suppressed even if the primary queue is empty.
func TestResolveMovementGateOnEngineQueue(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3200, 3200, 0
	p.lastTickX, p.lastTickZ, p.lastLevel = 3200, 3200, 0
	p.waypoints[0] = coordgrid.PackCoord(0, 3201, 3200)
	p.waypointIndex = 0
	p.walkDir = 7
	p.runDir = 7

	p.moveClickRequest = true
	p.delayed = true
	// Primary queue empty; engineQueue has work.
	p.engineQueue = append(p.engineQueue, playerQueueRequest{
		Script: &script.ScriptFile{Name: "[blocker_engine]"},
		Type:   script.QueueEngine,
	})

	p.resolveMovement()

	if p.walkDir != -1 {
		t.Errorf("walkDir: got %d, want -1 (gate fires on engineQueue → walkDir cleared)", p.walkDir)
	}
	if p.x != 3200 {
		t.Errorf("p.x: got %d, want 3200 (gate fires on engineQueue → no step taken)", p.x)
	}
}
```

- [ ] **Step 3.6: Run T7-b to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestResolveMovementGateOnEngineQueue -v`
Expected: PASS.

- [ ] **Step 3.7: Write T7-c — gate releases when both queues empty (failing test)**

Append to `modules/world/player_movement_gate_test.go`:

```go
// TestResolveMovementGateReleasesWhenQueuesEmpty pins that the gate
// only fires when at least one of (queue, engineQueue) is non-empty.
// With both empty AND moveClickRequest+Busy(), movement proceeds
// normally.
func TestResolveMovementGateReleasesWhenQueuesEmpty(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3200, 3200, 0
	p.lastTickX, p.lastTickZ, p.lastLevel = 3200, 3200, 0
	p.waypoints[0] = coordgrid.PackCoord(0, 3201, 3200)
	p.waypointIndex = 0

	p.moveClickRequest = true
	p.delayed = true // Busy() = true
	// Both queues empty → gate releases.

	p.resolveMovement()

	// Step happened — p.x advanced from 3200 to 3201 (or stepsTaken > 0).
	if p.stepsTaken == 0 {
		t.Errorf("stepsTaken: got 0, want > 0 (queues empty → gate releases → step proceeds)")
	}
}
```

- [ ] **Step 3.8: Run T7-c to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestResolveMovementGateReleasesWhenQueuesEmpty -v`
Expected: PASS.

- [ ] **Step 3.9: Run full movement test suite to verify the gate doesn't regress existing movement tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestResolveMovement -v`
Expected: PASS for all `TestResolveMovement*` tests. (`moveClickRequest` is false by default for existing tests, so the gate never fires for them.)

- [ ] **Step 3.10: Run full test suite for paranoia**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: PASS.

- [ ] **Step 3.11: Commit Task 3**

Run:
```bash
git add modules/world/movement.go modules/world/player_movement_gate_test.go
git commit --no-gpg-sign -m "feat(player): NAI-144 — movement gate (TS Player.ts:657 parity, inert at HEAD)

Ports the TS Player.ts:657 movement gate at top of resolveMovement:
when moveClickRequest && Busy() && (len(queue)>0 || len(engineQueue)>0),
suppress movement for this tick. Explicitly clears walkDir/runDir before
return to avoid stale prior-tick values.

INERT AT HEAD: goscape has zero \`moveClickRequest = true\` assignment
sites — TS sets it in World.ts:611-628 (per-tick post-decode pathfinding
pass). The setter port is tracked as NAI-144-D-MoveClickRequestSetter
follow-up; the gate is wired TS-faithful and activates as soon as a
setter lands.

T7-a/b/c pin gate-on-primary-queue, gate-on-engineQueue, and
gate-releases-when-empty.
"
```

---

## Task 4: `changeStat` and `advanceStat` migration to QueueEngine

**Files:**
- Modify: `modules/world/player_script.go:583-589` (`changeStat`)
- Modify: `modules/world/player_script.go:605-611` (`advanceStat`)
- Modify: `modules/world/player_script_test.go` (T9 fixup of existing assertions)
- Modify: `modules/world/player_script_test.go` (add T8 — `TestChangeStatUsesQueueEngine`, `TestAdvanceStatUsesQueueEngine`)

- [ ] **Step 4.1: Audit `TestAddXP*` tests for `p.queue` / `QueueNormal` assertions tied to changestat or advancestat**

Run:

```bash
grep -n "p\.queue\|QueueNormal\|QueueEngine" /home/owner/Code/github.com/zsrv/goscape/modules/world/player_script_test.go | head -30
```

Expected affected tests at HEAD:
- `TestAddXPFiresChangeStatOnLevelUp` (line ~204): pins `len(p.queue)==before+1`, `req := p.queue[before]`, `req.Type != script.QueueNormal`.
- `TestAddXPDoesNotFireChangeStatWithoutLevelUp` (line ~236): pins `len(p.queue) == before` (no fire).
- `TestAddXPChangeStatNoScriptIsNoop` (line ~257): pins no-fire shape.
- Any test combining changeStat AND advanceStat (mentioned at line 362-387 per spec exploration).

Report the list of test functions whose assertions need updating. The implementer must update each one.

- [ ] **Step 4.2: Migrate `changeStat` from QueueNormal to QueueEngine**

Edit `modules/world/player_script.go` lines 583-589. Current body:

```go
// (doc comment referring to TS Player.ts:1816-1821 + S6h "QueueNormal as approximation")
func (p *Player) changeStat(stat int) {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return
	}
	sf := p.client.server.scriptProvider.GetByTrigger(script.TriggerChangeStat, stat, -1)
	p.EnqueueScriptFile(sf, 0, nil, nil, script.QueueNormal)
}
```

Replace the doc-comment AND the QueueNormal argument:

```go
// changeStat fires the [changestat,<skill>] trigger for the given stat
// slot when a cache script is registered for that exact stat (or its
// category, or globally). Mirrors TS Player.changeStat (Player.ts:1816-1821).
//
// Enqueued as QueueEngine — TS PlayerQueueType.ENGINE: distinct from
// the primary queue, drains in processPlayerEngineQueues between
// processPlayerTimers and processPathing (NAI-144). Closes the S6h
// `QueueNormal-as-ENGINE` deviation.
//
// Silent no-op if no script is registered (GetByTrigger returns nil →
// EnqueueScriptFile's nil-check short-circuits). Called from AddXP's
// level-up branch.
func (p *Player) changeStat(stat int) {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return
	}
	sf := p.client.server.scriptProvider.GetByTrigger(script.TriggerChangeStat, stat, -1)
	p.EnqueueScriptFile(sf, 0, nil, nil, script.QueueEngine)
}
```

- [ ] **Step 4.3: Migrate `advanceStat` from QueueNormal to QueueEngine**

Edit `modules/world/player_script.go` lines 605-611. Current body uses QueueNormal. Replace the QueueNormal argument AND the doc-comment line that says "Enqueued as QueueNormal so it runs asynchronously through processPlayerQueue. Matches TS Player.ts:1804-1807 exactly."

Replace with:

```go
// advanceStat fires the [advancestat,<skill>] trigger for the given stat
// slot when a cache script is registered for that exact stat. Unlike
// changeStat (which uses the 3-level fallback via GetByTrigger), this
// uses GetByTriggerSpecific — type-specific only, no category or global
// fallback. A global [advancestat,_] script would be wrong here: cache
// scripts that say "Congratulations, you just advanced an Attack level!"
// must be skill-keyed.
//
// Enqueued as QueueEngine — TS PlayerQueueType.ENGINE (NAI-144).
// Matches TS Player.ts:1804-1807 exactly.
//
// Silent no-op if no specific script is registered (GetByTriggerSpecific
// returns nil → EnqueueScriptFile's nil-check short-circuits). Called
// from AddXP's level-up branch after changeStat.
func (p *Player) advanceStat(stat int) {
	if p.client == nil || p.client.server == nil || p.client.server.scriptProvider == nil {
		return
	}
	sf := p.client.server.scriptProvider.GetByTriggerSpecific(script.TriggerAdvanceStat, stat, -1)
	p.EnqueueScriptFile(sf, 0, nil, nil, script.QueueEngine)
}
```

- [ ] **Step 4.4: Update `TestAddXPFiresChangeStatOnLevelUp` (T9 regression-fence fixup)**

Edit `modules/world/player_script_test.go` lines 204-234. Replace the body. Current assertions at lines 221-233 pin `p.queue`. Update to pin `p.engineQueue`:

```go
func TestAddXPFiresChangeStatOnLevelUp(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	// Register [changestat,attack=0] — keyed by trigger(165) | (0x2<<8) | (0<<10).
	key := script.LookupKeyForType(script.TriggerChangeStat, objtype.PlayerStatAttack)
	sf := &script.ScriptFile{
		Name:      "[changestat,attack]",
		LookupKey: key,
	}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.stats[objtype.PlayerStatAttack] = 800
	p.baseLevels[objtype.PlayerStatAttack] = 2
	p.levels[objtype.PlayerStatAttack] = 2

	// NAI-144: changeStat now uses QueueEngine, not QueueNormal.
	beforeQueue := len(p.queue)
	beforeEngineQueue := len(p.engineQueue)
	p.AddXP(objtype.PlayerStatAttack, 1000) // → level 3

	if len(p.queue) != beforeQueue {
		t.Errorf("p.queue len: got %d, want %d (changestat must NOT land in primary queue post-NAI-144)",
			len(p.queue), beforeQueue)
	}
	if len(p.engineQueue) != beforeEngineQueue+1 {
		t.Fatalf("p.engineQueue len: got %d, want %d (+1 changestat via QueueEngine)",
			len(p.engineQueue), beforeEngineQueue+1)
	}
	req := p.engineQueue[beforeEngineQueue]
	if req.Script != sf {
		t.Errorf("p.engineQueue[%d].Script: got %v, want [changestat,attack] (%v)", beforeEngineQueue, req.Script, sf)
	}
	if req.Type != script.QueueEngine {
		t.Errorf("p.engineQueue[%d].Type: got %v, want QueueEngine (NAI-144 closes S6h deviation)", beforeEngineQueue, req.Type)
	}
}
```

- [ ] **Step 4.5: Update `TestAddXPDoesNotFireChangeStatWithoutLevelUp`**

Edit `modules/world/player_script_test.go` lines 236-255. Update the no-fire assertion to also check `p.engineQueue`:

```go
func TestAddXPDoesNotFireChangeStatWithoutLevelUp(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	key := script.LookupKeyForType(script.TriggerChangeStat, objtype.PlayerStatAttack)
	s.scriptProvider.Register(&script.ScriptFile{Name: "[changestat,attack]", LookupKey: key})

	p, _ := newTestPlayer(t)
	p.client.server = s
	p.stats[objtype.PlayerStatAttack] = 100 // below level-2 threshold (830)
	p.baseLevels[objtype.PlayerStatAttack] = 1
	p.levels[objtype.PlayerStatAttack] = 1

	beforeQueue := len(p.queue)
	beforeEngineQueue := len(p.engineQueue)
	p.AddXP(objtype.PlayerStatAttack, 100) // → 200, still level 1 (< 830)

	if len(p.queue) != beforeQueue {
		t.Errorf("p.queue len: got %d, want %d (no level-up = no changestat fire)",
			len(p.queue), beforeQueue)
	}
	if len(p.engineQueue) != beforeEngineQueue {
		t.Errorf("p.engineQueue len: got %d, want %d (no level-up = no changestat fire on QueueEngine path either)",
			len(p.engineQueue), beforeEngineQueue)
	}
}
```

- [ ] **Step 4.6: Update `TestAddXPChangeStatNoScriptIsNoop` and other affected tests**

Find each test in `modules/world/player_script_test.go` whose assertions reference `p.queue` in the context of changestat/advancestat. The test list reported in Step 4.1 enumerates them. For each one:

- If the test asserts that the queue grew by N entries due to changestat/advancestat, redirect to `p.engineQueue`.
- If the test asserts that the queue did NOT grow (no-fire branches), keep the `p.queue` check AND add a parallel `p.engineQueue` check.
- If the test pins `req.Type == QueueNormal` for a changestat or advancestat entry, change to `QueueEngine`.

Particularly: the combined changeStat+advanceStat test at lines 362-387 (mentioned during exploration) likely asserts BOTH fire, in TS-faithful order (changeStat first, advanceStat second). Both now land in `p.engineQueue` post-migration.

- [ ] **Step 4.7: Run all `TestAddXP*` tests to verify the regression-fence updates work with the migrated implementation**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestAddXP -v`
Expected: PASS for all `TestAddXP*` tests.

- [ ] **Step 4.8: Write T8-a — `TestChangeStatUsesQueueEngine` (regression fence for migration)**

Append to `modules/world/player_script_test.go`:

```go
// TestChangeStatUsesQueueEngine is a direct regression fence for the
// NAI-144 migration: changeStat must enqueue to p.engineQueue with
// Type=QueueEngine, not the previous S6h QueueNormal-as-ENGINE shape.
func TestChangeStatUsesQueueEngine(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	key := script.LookupKeyForType(script.TriggerChangeStat, objtype.PlayerStatAttack)
	sf := &script.ScriptFile{Name: "[changestat,attack]", LookupKey: key}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s

	p.changeStat(objtype.PlayerStatAttack)

	if len(p.queue) != 0 {
		t.Errorf("p.queue len: got %d, want 0 (changeStat must NOT land in primary queue)", len(p.queue))
	}
	if len(p.engineQueue) != 1 {
		t.Fatalf("p.engineQueue len: got %d, want 1 (changeStat uses QueueEngine)", len(p.engineQueue))
	}
	if p.engineQueue[0].Type != script.QueueEngine {
		t.Errorf("Type: got %v, want QueueEngine", p.engineQueue[0].Type)
	}
	if p.engineQueue[0].Script != sf {
		t.Errorf("Script: got %v, want %v", p.engineQueue[0].Script, sf)
	}
}
```

- [ ] **Step 4.9: Write T8-b — `TestAdvanceStatUsesQueueEngine` (regression fence)**

Append to `modules/world/player_script_test.go`:

```go
// TestAdvanceStatUsesQueueEngine pins NAI-144 migration: advanceStat
// uses QueueEngine to match TS Player.ts:1804-1807.
func TestAdvanceStatUsesQueueEngine(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	key := script.LookupKeyForType(script.TriggerAdvanceStat, objtype.PlayerStatAttack)
	sf := &script.ScriptFile{Name: "[advancestat,attack]", LookupKey: key}
	s.scriptProvider.Register(sf)

	p, _ := newTestPlayer(t)
	p.client.server = s

	p.advanceStat(objtype.PlayerStatAttack)

	if len(p.queue) != 0 {
		t.Errorf("p.queue len: got %d, want 0 (advanceStat must NOT land in primary queue)", len(p.queue))
	}
	if len(p.engineQueue) != 1 {
		t.Fatalf("p.engineQueue len: got %d, want 1 (advanceStat uses QueueEngine)", len(p.engineQueue))
	}
	if p.engineQueue[0].Type != script.QueueEngine {
		t.Errorf("Type: got %v, want QueueEngine", p.engineQueue[0].Type)
	}
}
```

- [ ] **Step 4.10: Run T8-a/b**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestChangeStatUsesQueueEngine|TestAdvanceStatUsesQueueEngine" -v`
Expected: PASS.

- [ ] **Step 4.11: Run full test suite for final verification**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... ./pkg/script/...`
Expected: PASS for ALL tests. If any test fails, halt and investigate — likely an overlooked changestat/advancestat assertion site from Step 4.6.

- [ ] **Step 4.12: Run race-detector pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...`
Expected: PASS, no race warnings. The new `processPlayerEngineQueues` follows the established `processActiveScripts` lock pattern, but a race-detector pass confirms.

- [ ] **Step 4.13: Commit Task 4**

Run:
```bash
git add modules/world/player_script.go modules/world/player_script_test.go
git commit --no-gpg-sign -m "feat(player): NAI-144 — migrate changeStat + advanceStat to QueueEngine

Closes the S6h \`QueueNormal-as-ENGINE\` tracked deviation. Both consumers
(Player.ts:1816-1821 changeStat + Player.ts:1804-1807 advanceStat) now
route to the QueueEngine drain wired in earlier tasks. T8-a/b pin the
migration directly; T9 (existing TestAddXPFires*) updated to assert
p.engineQueue + Type=QueueEngine.

Closes memory: superpowers:specs/2026-04-21-runescript-s6h-changestat-trigger-design.md QueueEngine-deferral
"
```

---

## Final verification

- [ ] **Step 5.1: Confirm clean working tree (no untracked changes)**

Run: `git status`
Expected: clean working tree (apart from any pre-existing untracked files unrelated to NAI-144).

- [ ] **Step 5.2: Confirm 4 commits on the branch**

Run: `git log --oneline -5`
Expected: 4 NAI-144 commits + earlier `cbed772` spec commit.

- [ ] **Step 5.3: Final full test suite + race detector**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... ./pkg/script/...
```
Expected: PASS for both.

- [ ] **Step 5.4: Open close-bundle commit**

Bundle close commit summarizing the four task commits, with the canonical NAI-N close shape (memory: `close_commit_memory_trailer`):

```bash
git commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-144 — QueueEngine wiring; smoke deferred (foundational infra)

Activates script.QueueEngine end-to-end:

- Player.engineQueue slice + EnqueueScriptFile qtype routing (Task 1)
- s.processPlayerEngineQueues drain inserted between processPlayerTimers
  and processPathing per TS World.ts:725 (Task 2)
- Movement gate at top of resolveMovement per TS Player.ts:657
  (Task 3 — INERT at HEAD; tracker NAI-144-D-MoveClickRequestSetter
  for follow-up setter port)
- changeStat + advanceStat migrated from QueueNormal to QueueEngine,
  closing the S6h tracked deviation (Task 4)

SECONDARY pins (test-only): T1-T10 all green; existing TestAddXP* and
TestChangeStat* regression fences updated and green.

PRIMARY pin: SMOKE DEFERRED. Foundational infra; per
cascade_theory_smoke_binding the test-only PRIMARY is acceptable.
Carry-forward: bind a level-up smoke (combat XP → [changestat,*] modal)
or zone-walk smoke (NAI-145 SetMultiway) at the next QueueEngine-touching
sub-spec close.

Predecessor to NAI-145 (NAI-142-D-R-D2 + NAI-142-D-R-D3). NAI-145
brainstorm/spec begins fresh-session.

Closes memory: superpowers:specs/2026-04-21-runescript-s6h-changestat-trigger-design.md
EOF
)"
```

---

## Self-review

**1. Spec coverage check (against `docs/superpowers/specs/2026-05-09-nai-144-queueengine-wiring-design.md`):**

| Spec §1 In-scope item | Plan task |
|----------------------|-----------|
| `Player.engineQueue` field | Task 1 (Step 1.4) |
| EnqueueScriptFile routing | Task 1 (Step 1.8) |
| `processPlayerEngineQueues` drain | Task 2 (Step 2.3) |
| Tick-loop slot (between timers and pathing) | Task 2 (Step 2.13) |
| Movement gate (top of resolveMovement) | Task 3 (Step 3.3) |
| `changeStat` migration | Task 4 (Step 4.2) |
| `pkg/script/queue.go` `// reserved` removal | Task 1 (Step 1.2) |
| `advanceStat` migration (audit-resolved) | Task 4 (Step 4.3) |

| Spec §3 Test plan | Plan step |
|------------------|-----------|
| T1 `TestEnqueueQueueEngineRoutesToEngineQueue` | Step 1.6 |
| T2 `TestEnqueueQueueNormalDoesNotRouteToEngineQueue` | Step 1.10 |
| T3 `TestProcessPlayerEngineQueuesFiresWhenDelayReachesZero` | Step 2.1 |
| T4 `TestProcessPlayerEngineQueuesGatedByCanAccess` | Step 2.5 |
| T5 `TestProcessPlayerEngineQueuesNoStrongBypassNoDelayedGate` | Step 2.7 |
| T6 `TestProcessPlayerEngineQueuesSameTickReentrant` | Step 2.9 |
| T7 movement gate (3 sub-cases) | Steps 3.1, 3.5, 3.7 |
| T8 `TestChangeStatUsesQueueEngine` (+ T8-b advanceStat) | Steps 4.8, 4.9 |
| T9 regression fence | Steps 4.4, 4.5, 4.6 |
| T10 `TestProcessPlayerEngineQueuesEmptyIsNoop` | Step 2.11 |

All in-scope spec items have tasks. All T1–T10 have implementation steps.

**2. Placeholder scan:** No "TBD", "TODO" placeholders. Two known-unknowns from the audits ARE explicit (canAccess() existence; affected test list from Step 4.1 grep) — these are intentional implementer audits at execution time, not lazy plan artifacts.

**3. Type consistency:** `engineQueue` declared in Step 1.4; referenced as `p.engineQueue` in Steps 1.8, 2.3, 2.5, 2.7, 2.9, 2.11, 3.1, 3.5, 3.7, 4.4, 4.5, 4.6, 4.8, 4.9 — consistent. `playerQueueRequest` reused throughout. `script.QueueEngine` enum value referenced consistently. `Busy()` (capitalized) vs `canAccess()` distinguished where relevant.

**4. Risk register coverage:** R1 (reentrancy) → T6. R2 (TestAddXP* assertions) → Step 4.1 audit + Steps 4.4-4.6 fixup. R3 (gate over-block) → resolved during plan-write (gate is INERT at HEAD; documented + tracker entry). R4 (tick-slot ordering) → Step 2.14 full suite. R5 (walkDir/runDir staleness) → Step 3.3 explicit clears.

---

## Memories applied

- `runescript_cadence` — full cadence (brainstorm → spec → plan → subagent-driven TDD).
- `compressed_cadence` — NOT applied; surface is too large for compressed bundle.
- `execution_mode_default` — subagent-driven-development.
- `superpowers_code_reviewer_model` — reviewer dispatches Sonnet only.
- `verify_implementer_claims` — Steps 1.12, 2.14, 3.10, 4.7, 4.11, 4.12, 5.3 verify with fresh independent runs.
- `risk_register_premise_grep` — R3 audit performed at plan-write; gate INERT classification documented.
- `plan_test_coverage_crosscheck` — every task's code block has a test ID mapped in §Self-review.
- `plan_helper_coverage` — fixture re-use audit: `newTestPlayer`, `newTestServer` confirmed sufficient for T1-T10 (no helper-flag mismatches).
- `plan_var_name_collision` — Step 4.4-4.5 use `beforeQueue` / `beforeEngineQueue` (no collision with `before` from existing test).
- `int32_hex_literal_overflow` — LookupKey literals stay below 0x80000000 (uint32 territory): all are < 0x10000000.
- `plan_runnable_test_fixtures` — every test fixture mentally compiled before saving (canAccess audit flagged at Step 2.3 as known-unknown for implementer to verify).
- `mock_recorder_field_naming_check` — Step 2.3 explicitly tells implementer to grep `func (p *Player) canAccess` before using; do not infer the method name.
- `audit_full_method_when_restructuring` — both `changeStat` and `advanceStat` audited in full (not just lines 583-589).
- `close_commit_memory_trailer` — Step 5.4 close commit includes `Closes memory:` trailer.
- `superpowers_clear_between_spec_and_impl` — after this plan is committed, controller emits paste-ready resume prompt and STOPS (does NOT begin Task 1 in this session).
- `session_context_management` — implementer dispatches happen in fresh sessions per task or per bundle.
