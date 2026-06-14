# ARCH-1 Faithful Tick Error-Recovery — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: subagent-driven-development or executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Make goscape's per-tick error recovery retry-next-tick on a panic at the NPC lifecycle transition (A) and the world-script queue (B), matching rev-274 TS (`Engine-TS @dee467c8`), closing PORTING.md row ARCH-1.

**Architecture:** TS `ScriptRunner.execute` catches script errors internally (→ ABORTED, no rethrow); the outer `try/catch` only retries on *escaped* throws = goscape panics. So the change is panic-only retry. (A) wrap the lifecycle transition in a recover that re-arms `lifecycleTick=1`; (B) fire-then-remove-on-clean, leave-on-panic. (C) objDelayed already matches — untouched. Full rationale: `docs/superpowers/specs/2026-06-13-rev274-arch1-tick-recovery-design.md`.

**Tech Stack:** Go; `modules/world` (Server tick loop), `pkg/script`. Tests are stdlib `testing`. Go commands prefixed `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`. Commits `--no-gpg-sign`, suffix `[rev-274]`, Co-Authored-By line. Never `git add -A` (add explicit paths).

---

### Task 1: (A) NPC lifecycle retry — `fireNpcLifecycle`

**Files:**
- Modify: `modules/world/npc_ai.go` (extract transition; remove PORTING-EXCEPTION; add `runtime/debug` import)
- Test: `modules/world/arch1_tick_recovery_test.go` (new)

- [ ] **Step 1: Write the failing tests** (new file `modules/world/arch1_tick_recovery_test.go`)

```go
package world

import "testing"

// ARCH-1 (A): a panic during the lifecycle transition must re-arm
// lifecycleTick=1 (TS Npc.ts:144-150 setLifeCycle(1) retry), NOT evict.
// Deterministic panic: s.npcs is a fixed [16384]*Npc array, so an
// out-of-bounds nid forces an index-out-of-range panic inside removeNpc's
// despawn branch (s.npcs[n.nid] = nil) — standing in for any transition fault.
func TestFireNpcLifecycle_DespawnPanicRetries(t *testing.T) {
	s := &Server{log: discardLogger()}
	n := &Npc{nid: 1 << 20, typeId: 42, lifecycle: NpcLifecycleDespawn}

	fired := s.fireNpcLifecycle(n)

	if !fired {
		t.Error("fired: want true (transition attempted), got false")
	}
	if n.lifecycleTick != 1 {
		t.Errorf("lifecycleTick: want 1 (TS setLifeCycle(1) retry), got %d", n.lifecycleTick)
	}
	// Reaching here proves the panic did not propagate.
}

// ARCH-1 (A): a clean transition must NOT re-arm lifecycleTick (no retry).
func TestFireNpcLifecycle_DespawnCleanNoRetry(t *testing.T) {
	s := &Server{log: discardLogger()} // scriptProvider nil → no trigger
	n := &Npc{nid: 7, typeId: 42, lifecycle: NpcLifecycleDespawn, lifecycleTick: 0}

	fired := s.fireNpcLifecycle(n)

	if !fired {
		t.Error("fired: want true, got false")
	}
	if n.lifecycleTick != 0 {
		t.Errorf("lifecycleTick: want 0 (clean path does not retry), got %d", n.lifecycleTick)
	}
	if !n.dead {
		t.Error("n.dead: want true after clean despawn, got false")
	}
}

// ARCH-1 (A): the inner recover pre-empts the outer recoverNpc eviction.
// Run the npc through the same closure shape processNpcs uses; the inner
// recover handles the panic so recoverNpc never fires.
func TestNpcLifecyclePanic_InnerRecoverPreemptsEviction(t *testing.T) {
	s := &Server{log: discardLogger()}
	n := &Npc{nid: 1 << 20, typeId: 42, lifecycle: NpcLifecycleDespawn, lifecycleTick: 1}

	func(n *Npc) {
		defer recoverNpc(n, s, "processNpcTurn", s.log)
		n.turn(s)
	}(n)

	if n.lifecycleTick != 1 {
		t.Errorf("lifecycleTick: want 1 (inner recover re-armed; outer evict pre-empted), got %d", n.lifecycleTick)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail to compile** (`fireNpcLifecycle` undefined)

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'FireNpcLifecycle|InnerRecoverPreempts' 2>&1 | head`
Expected: build error — `s.fireNpcLifecycle undefined`.

- [ ] **Step 3: Add `runtime/debug` import to `npc_ai.go`**

Change the import block from:
```go
import (
	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/script"
)
```
to:
```go
import (
	"runtime/debug"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/script"
)
```

- [ ] **Step 4: Extract the transition into `fireNpcLifecycle` and rewire `turn`**

In `modules/world/npc_ai.go`, replace the events block (the `if !n.delayed { n.lifecycleTick--; if n.lifecycleTick == 0 { switch n.lifecycle { ... } } }`, currently lines ~26-75) with:

```go
	// === Events block (NAI-5 — matches TS Npc.ts:121-151) ===
	if !n.delayed {
		n.lifecycleTick--
		if n.lifecycleTick == 0 {
			if s.fireNpcLifecycle(n) {
				return
			}
		}
	}
```

Then add the new method (place it directly after `turn`):

```go
// fireNpcLifecycle runs the once-per-cycle lifecycle transition (respawn /
// type-revert / despawn) under a recover that retries next tick on panic,
// mirroring TS Npc.ts:122-150 (try { … } catch { … this.setLifeCycle(1) }).
//
// Returns fired=true when a transition ran (respawn or despawn) so turn()
// skips this tick's movement — preserving goscape's behavior of not
// overwriting a teleport with a walk path on the transition tick.
//
// On panic the transition is logged and lifecycleTick is re-armed to 1 (TS
// setLifeCycle(1) — retry next tick) instead of letting the panic bubble to
// recoverNpc, which would evict the NPC via removeNpc(n,-1). This is the
// INNER of TS's two recovery layers: inner retry (Npc.ts:144-150) pre-empts
// outer evict (World.ts:681-690 → goscape recoverNpc). ARCH-1.
func (s *Server) fireNpcLifecycle(n *Npc) (fired bool) {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("panic in npc lifecycle transition (retrying next tick)",
				"nid", n.nid,
				"typeId", n.typeId,
				"lifecycle", n.lifecycle,
				"err", r,
				"stack", string(debug.Stack()))
			n.lifecycleTick = 1 // TS setLifeCycle(1): retry next tick
			fired = true        // skip movement this tick
		}
	}()
	switch n.lifecycle {
	case NpcLifecycleRespawn:
		if n.dead {
			// Respawn: flip dead, reset position, revert type.
			n.dead = false
			prevX, prevZ, prevLevel := n.x, n.z, n.level
			n.x, n.z, n.level = n.startX, n.startZ, n.startLevel
			// Zone-only refresh — deliberately NOT the collision-following
			// refreshNpcZonePresence: TS routes respawn through World.addNpc
			// (World.ts:1295-1316) which seeds collision at the start tile;
			// the death-tile flags were already cleared by removeNpc at
			// death, so a presence-move here would phantom-remove at the
			// death tile.
			refreshNpcZone(s, n, prevX, prevZ, prevLevel)
			n.revertType()
		} else {
			// Revert morphed NPC (post-changetype).
			n.revertType()
		}
		return true
	case NpcLifecycleDespawn:
		if !n.dead {
			s.removeNpc(n, -1)
			if s.scriptProvider != nil && n.typ != nil {
				sf := s.scriptProvider.GetByTrigger(
					script.TriggerAiDespawn, n.typeId, n.typ.Category)
				if sf != nil {
					s.npcEventQueue = append(s.npcEventQueue,
						NpcEventRequest{
							Type:   NpcEventDespawn,
							Script: sf,
							Npc:    n,
						})
				}
			}
		}
		return true
	}
	return false
}
```

(The old inline PORTING-EXCEPTION comment block and the skip-movement comment are dropped — the rationale now lives in the method doc / inline notes above.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'FireNpcLifecycle|InnerRecoverPreempts' -v 2>&1 | tail -20`
Expected: all three PASS.

- [ ] **Step 6: Confirm no NPC lifecycle regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'Npc|Lifecycle|Respawn|Despawn|Turn' 2>&1 | tail -20`
Expected: PASS (the extraction is behavior-preserving for the clean path).

- [ ] **Step 7: Commit**

```bash
git add modules/world/npc_ai.go modules/world/arch1_tick_recovery_test.go
git commit --no-gpg-sign -m "fix(world): ARCH-1 (A) NPC lifecycle retry-next-tick on panic [rev-274]

Extract the respawn/despawn transition into fireNpcLifecycle, wrapped in a
recover that re-arms lifecycleTick=1 (TS Npc.ts:144-150 setLifeCycle(1))
instead of bubbling to recoverNpc (evict). Inner retry pre-empts outer evict.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: (B) world-queue retry — `fireWorldScript` + `logWorldScriptPanic`

**Files:**
- Modify: `modules/world/world_script_queue.go` (`processWorldQueue` restructure; add `fireWorldScript`)
- Modify: `modules/world/tick_recovery.go` (replace `recoverWorldScript` → `logWorldScriptPanic`; remove ARCH-1 PORTING-EXCEPTION; clarify `recoverObjDelayed` note)
- Modify: `modules/world/tick_recovery_test.go` (replace `TestRecoverWorldScript_*`)
- Test: `modules/world/arch1_tick_recovery_test.go` (append (B) tests)

- [ ] **Step 1: Write the failing (B) tests** — append to `modules/world/arch1_tick_recovery_test.go`

```go
// newPanickingWorldScript builds a ScriptState whose Execute panics: an
// NPC find opcode with IntOperand=2 reaches setActiveNpcSlot, which panics
// on any operand ∉ {0,1} (handlers_npc.go:183). The find must succeed, so
// the npc placed by buildNpcForIntegration is targeted by exact coord+type.
// (If the exact opcode/coord packing differs, adapt to NPC_FIND / LOC_ADD /
// OBJ — the only requirement is a real panic escaping script.Execute.)
//
// IMPLEMENTER: verify OpNpcFindExact constant name, coord packing helper,
// and that buildNpcForIntegration's npc is findable at its coord before
// finalizing. See spec §4.2.

func TestFireWorldScript_PanicReported(t *testing.T) {
	s, n := buildNpcForIntegration(t)
	state := newPanickingWorldScript(t, s, n)

	panicked := s.fireWorldScript(state)

	if !panicked {
		t.Error("panicked: want true for a panicking world script, got false")
	}
}

func TestFireWorldScript_CleanReturnsFalse(t *testing.T) {
	s := newTestServer(t)
	state := newReturnImmediatelyScript(t)

	if s.fireWorldScript(state) {
		t.Error("panicked: want false for a clean script, got true")
	}
}

// ARCH-1 (B): a panicking world-queue entry is LEFT queued (retry next tick),
// mirroring TS World.ts:542-558 (unlink runs after execute; a throw skips it).
func TestProcessWorldQueue_PanicRetriesNextTick(t *testing.T) {
	s, n := buildNpcForIntegration(t)
	state := newPanickingWorldScript(t, s, n)
	s.EnqueueWorldScript(state, 0) // stored=1; fires on the 2nd drain

	s.processWorldQueue() // tick 1: skip
	s.processWorldQueue() // tick 2: fire → panics → LEFT queued
	if got := len(s.worldScriptQueue); got != 1 {
		t.Fatalf("after panic-fire: queue length got %d, want 1 (entry retained for retry)", got)
	}
	s.processWorldQueue() // tick 3: fires again, still panics, still queued
	if got := len(s.worldScriptQueue); got != 1 {
		t.Errorf("after 2nd panic-fire: queue length got %d, want 1 (still retrying)", got)
	}
}

// ARCH-1 (B): a clean world-queue entry is removed after firing.
func TestProcessWorldQueue_CleanEntryRemovedAfterFire(t *testing.T) {
	s := newTestServer(t)
	state := newReturnImmediatelyScript(t)
	s.EnqueueWorldScript(state, 0)

	s.processWorldQueue() // tick 1: skip
	s.processWorldQueue() // tick 2: fire → clean → removed
	if got := len(s.worldScriptQueue); got != 0 {
		t.Errorf("after clean fire: queue length got %d, want 0 (removed)", got)
	}
}
```

- [ ] **Step 2: Run to verify failure** (`fireWorldScript`, `newPanickingWorldScript` undefined)

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'FireWorldScript|ProcessWorldQueue_Panic|ProcessWorldQueue_Clean' 2>&1 | head`
Expected: build error.

- [ ] **Step 3: Replace `recoverWorldScript` with `logWorldScriptPanic` in `tick_recovery.go`**

Delete `recoverWorldScript` (and its ARCH-1 PORTING-EXCEPTION comment, lines ~47-72). Add:

```go
// logWorldScriptPanic emits the structured error log for a world-script-queue
// panic. The recover() itself is performed by fireWorldScript (it must run in
// that deferred frame to set the panicked return); this helper only formats
// the log. r is the recovered panic value.
//
// ARCH-1: the panicking entry is intentionally LEFT in the queue by the caller
// so it retries on the next tick, mirroring TS World.ts:542-558 (unlink runs
// after execute; a throw skips it). The prior remove-before-fire behavior
// (swallow, no retry) is closed.
//
// Mirrors TS World.ts:534-559 catch logging.
func logWorldScriptPanic(state *script.ScriptState, r any, log *slog.Logger) {
	scriptName := ""
	if state != nil && state.Script != nil {
		scriptName = state.Script.Name
	}
	log.Error("panic in world script execution (retrying next tick)",
		"script", scriptName,
		"err", r,
		"stack", string(debug.Stack()))
}
```

In `recoverObjDelayed`'s doc comment, append a one-line clarification that its remove-before-fire/no-retry is the TS-faithful behavior for objDelayed (World.ts:566-572), contrasting it with the world-queue's new retry — so a reader does not "fix" it to match the queue.

- [ ] **Step 4: Restructure `processWorldQueue` + add `fireWorldScript` in `world_script_queue.go`**

Replace the doc comment + loop body of `processWorldQueue` (lines ~38-88) and add `fireWorldScript`:

```go
// processWorldQueue drains ready entries from s.worldScriptQueue, firing each
// via fireWorldScript (script.Execute through resumeOrFinishWorld) and
// dispatching the post-execute state.
//
// Iteration uses an index-based slice walk with mid-pass append visibility
// (re-reads len each iteration) — the TS-authentic "speedup quirk" where a
// script that re-enqueues during Execute is processed the same tick.
//
// Fire-then-remove-on-clean: an entry is removed only after a non-panicking
// fire. On a panic the entry is LEFT queued so it retries next tick. This
// mirrors TS World.ts:542-558, where request.unlink() runs AFTER
// ScriptRunner.execute and a throw skips the unlink. Normal script errors are
// caught INSIDE execute (→ ABORTED → resumeOrFinishWorld returns cleanly →
// removed here); only a genuine Go panic takes the retry path. ARCH-1.
func (s *Server) processWorldQueue() {
	i := 0
	for i < len(s.worldScriptQueue) {
		entry := &s.worldScriptQueue[i]
		// POST-decrement: capture current, then decrement. Mirrors TS
		// World.ts:535 `const delay = request.delay--`.
		delay := entry.delay
		entry.delay--
		if delay > 0 {
			i++
			continue
		}
		state := entry.script
		// Reset Execution=Running so script.Execute resumes from the
		// post-WORLD_DELAY PC. Mirrors the player-path resume at tick.go:211.
		state.Execution = script.Running

		if s.fireWorldScript(state) {
			// Panicked: leave the entry queued so it retries next tick;
			// advance past it for the remainder of this pass.
			i++
			continue
		}
		// Clean return (incl. an inline-handled script error): remove.
		s.worldScriptQueue = append(s.worldScriptQueue[:i], s.worldScriptQueue[i+1:]...)
		// Don't advance i: we removed the current element, so i now points
		// to what was the next element (or past end).
	}
}

// fireWorldScript resumes one world-queued script under a recover. Returns
// panicked=true when execution panicked (the caller leaves the entry queued
// for a next-tick retry, mirroring TS World.ts where a throw skips unlink).
// A normal return — including an inline-handled script error that
// resumeOrFinishWorld logged and routed — yields panicked=false (caller
// removes the entry). ARCH-1.
func (s *Server) fireWorldScript(state *script.ScriptState) (panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			panicked = true
			logWorldScriptPanic(state, r, s.log)
		}
	}()
	s.resumeOrFinishWorld(state)
	return false
}
```

- [ ] **Step 5: Replace the obsolete `TestRecoverWorldScript_*` tests in `tick_recovery_test.go`**

Delete `TestRecoverWorldScript_NoPanic`, `TestRecoverWorldScript_PanicSwallowed`, `TestRecoverWorldScript_NilStateSafe` (lines ~196-247). Add a nil-state guard test for the new helper:

```go
// TestLogWorldScriptPanic_NilStateSafe: nil state must not nil-panic the logger.
func TestLogWorldScriptPanic_NilStateSafe(t *testing.T) {
	log := discardLogger()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("logWorldScriptPanic should not nil-panic; got: %v", r)
		}
	}()
	logWorldScriptPanic(nil, "boom", log)
}
```

(The panic-detection + clean-return coverage now lives in the `fireWorldScript` tests in Task 2 Step 1. The `script` import in `tick_recovery_test.go` is still used by other tests; verify it isn't orphaned.)

- [ ] **Step 6: Implement `newPanickingWorldScript` helper** (in `arch1_tick_recovery_test.go`)

Build the panicking script per spec §4.2 (NPC find opcode, `IntOperand=2`, targeting `buildNpcForIntegration`'s npc by exact coord+type). Verify the opcode constant, coord packing, and findability against the actual `buildNpcForIntegration` setup; adapt the opcode if needed. The helper returns a `*script.ScriptState` whose `script.Execute` panics.

- [ ] **Step 7: Run the (B) tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'FireWorldScript|ProcessWorldQueue' -v 2>&1 | tail -30`
Expected: all PASS (including the pre-existing `TestProcessWorldQueue_*` scheduler tests, which are behavior-preserving on the clean path).

- [ ] **Step 8: Rename the now-inaccurate existing test**

In `world_script_queue_test.go`, rename `TestProcessWorldQueue_RemovedBeforeFire` → `TestProcessWorldQueue_RemovedAfterCleanFire` and rewrite its comment to describe remove-after-clean-fire (the assertion — queue length 0 after a clean fire — is unchanged).

- [ ] **Step 9: Commit**

```bash
git add modules/world/world_script_queue.go modules/world/tick_recovery.go modules/world/tick_recovery_test.go modules/world/world_script_queue_test.go modules/world/arch1_tick_recovery_test.go
git commit --no-gpg-sign -m "fix(world): ARCH-1 (B) world-queue retry-next-tick on panic [rev-274]

processWorldQueue now fires-then-removes-on-clean; a panicking entry is left
queued to retry next tick (TS World.ts:542-558 unlink-after-execute). New
fireWorldScript owns the recover; recoverWorldScript replaced by
logWorldScriptPanic. objDelayed (C) unchanged (already TS-faithful).

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Close ARCH-1 docs + full verification

**Files:**
- Modify: `PORTING.md`, `docs/PORTING-CLOSED.md`
- Modify: memory `rev274_port_started.md` / `MEMORY.md` (after verification)

- [ ] **Step 1: Verify no leftover ARCH-1 PORTING-EXCEPTION markers**

Run: `grep -rn "PORTING-EXCEPTION (ARCH-1" modules pkg cmd internal`
Expected: no output (both markers removed in Tasks 1-2).

Run: `grep -rn "PORTING-EXCEPTION" --include='*_test.go' . | grep -iE "count|== [0-9]|wc"`
Expected: no numeric count assertion (confirms removing 2 markers breaks no test).

- [ ] **Step 2: Move the ARCH-1 rows in `PORTING.md` to FIXED**

Edit the open-deviations row (~line 32) and the audit row (~line 49): change status `🚧 ARCH-1` / "deferred indefinitely" to **FIXED** with the two fix-commit SHAs (from Tasks 1-2), TS refs (`Npc.ts:122-150`, `World.ts:534-558`), and the "panic-only retry; normal errors already matched" rationale. Per convention, relocate the closed row to `docs/PORTING-CLOSED.md`.

- [ ] **Step 3: Full test suite + race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... 2>&1 | tail -30`
Expected: all PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... 2>&1 | tail -20`
Expected: PASS, no race.

- [ ] **Step 4: Commit docs**

```bash
git add PORTING.md docs/PORTING-CLOSED.md
git commit --no-gpg-sign -m "docs(porting): ARCH-1 CLOSED — tick error-recovery now TS-faithful (panic retry) [rev-274]

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

- [ ] **Step 5: Update memory** — note ARCH-1 closed (last open rev-274 backlog row) in `rev274_port_started.md` and the `MEMORY.md` index line.

---

## Self-review

- **Spec coverage:** (A) Task 1; (B) Task 2; (C) no-op noted in Task 2 Step 3; docs/PORTING Task 3; tests §4 across Tasks 1-2; backport explicitly out of scope (spec §6).
- **Placeholders:** none — all code is concrete except `newPanickingWorldScript`'s exact bytecode, which is flagged for implementer verification against real `buildNpcForIntegration` setup (an inherent verify-at-impl detail, not a hand-wave).
- **Type consistency:** `fireNpcLifecycle(n *Npc) (fired bool)`, `fireWorldScript(state *script.ScriptState) (panicked bool)`, `logWorldScriptPanic(state *script.ScriptState, r any, log *slog.Logger)` used consistently across plan + tests.
