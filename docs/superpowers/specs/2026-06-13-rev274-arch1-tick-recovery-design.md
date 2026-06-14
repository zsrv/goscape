# ARCH-1: Faithful tick error-recovery (rev-274) — Design

**Status:** APPROVED (design) — ready for implementation plan
**Date:** 2026-06-13
**Branch:** `rev-274`
**Closes:** PORTING.md backlog row ARCH-1 (the sole remaining open row)

---

## 1. Problem

goscape's per-tick error recovery diverges from the rev-274 TS engine
(`Engine-TS @dee467c8`) in how it handles a fault during two tick steps. TS
uses `try/catch` and **retries the offending work on the next tick**; goscape
evicts/drops it.

Three sites were audited:

| Site | TS reference | TS on fault | goscape today | Verdict |
|------|--------------|-------------|---------------|---------|
| **(A)** NPC lifecycle transition (respawn / type-revert / despawn) | `Npc.ts:122-150` | `catch { … this.setLifeCycle(1) }` → retry next tick | panic bubbles to `recoverNpc` → `removeNpc(n,-1)` (evicts the NPC) | **divergent** |
| **(B)** World-script queue fire | `World.ts:534-559` | `unlink()` runs *after* `execute()`; a throw skips it → entry retries next tick | removes entry *before* fire → a panicking entry is gone (no retry) | **divergent** |
| **(C)** objDelayed queue fire | `World.ts:566-572` | `unlink()` runs *before* `addObj()` → no retry | removes before fire → no retry | **already matches — no change** |

The user has chosen the **"Faithful TS at both"** posture: restore retry-next-tick
at (A) and (B), leave (C) untouched. This closes ARCH-1 as fully FIXED with no
remaining deviation.

---

## 2. Key insight (scopes the change)

The naive reading of "TS retries on any error" would force goscape to re-run
buggy content scripts forever. That is **not** what TS does, and verifying the
actual TS control flow narrows the change dramatically.

**TS `ScriptRunner.execute` (`ScriptRunner.ts:120-227`) catches script errors
internally.** Its `try/catch` wraps the entire opcode loop; on a thrown error it
messages the player / removes the npc / logs to console, sets
`state.execution = ScriptState.ABORTED`, and **returns** — it does **not**
re-throw. So a normal script runtime error (bad PC, unhandled opcode, opcount
limit, a throwing handler) never reaches the world-queue or lifecycle `catch`.
The queue simply unlinks the `ABORTED` entry → **no retry**.

goscape mirrors this already: `script.Execute` (`pkg/script/runner.go:55`)
**returns** the error; `resumeOrFinishWorld` (`modules/world/script.go:246`)
logs it via `logScriptExecuteError` + routes `handlePlayerScriptError` /
`handleNpcScriptError`, then returns normally. The world-queue entry is then
removed on the normal path. **Normal script errors already behave identically
across TS and goscape** (handled inline, entry removed, no retry).

Therefore the *only* fault the outer TS `try/catch` actually retries on is a
**throw that escapes `execute`** — which in goscape is a **Go panic**. So the
faithful change is precisely:

- **(A)** On a *panic* during the lifecycle transition → re-arm `lifecycleTick = 1`
  (TS `setLifeCycle(1)`, retry next tick) instead of evicting.
- **(B)** On a *panic* during world-script fire → leave the entry queued (retry
  next tick) instead of removing it before fire.

Because goscape returns content-script errors (they do **not** panic — confirmed
by the live boot log, where "opcount limit exceeded" and similar surfaced as
WARN log lines, an error-return path), the "infinite retry of a buggy content
script" footgun is **not** triggered by content bugs. Only a genuine Go panic
(a nil-deref or an invariant `panic()` in engine code — i.e. a goscape bug)
takes the retry path, which is exactly the fault class TS-faithful retry is for.

### 2.1 Architecture note (why ARCH-1 was deferred so long)

TS's lifecycle `try/catch` primarily guards `World.addNpc` throwing on NPC-ID
exhaustion (the catch comment: *"ex: server is full on npc IDs"*). goscape's
respawn architecture cannot hit that case: respawn revives the NPC **in place**
(it reuses the existing `nid`; `removeNpc` only frees the slot in the DESPAWN
branch — see `npc_registry.go` NAI-19), and `addNpc` returns an `error` rather
than throwing. So the faithful port reduces to **panic-retry parity** — the
residual difference ARCH-1 names — not a re-architecting of respawn. This is
why the change is small once the TS control flow is understood.

---

## 3. Design

### 3.1 (A) NPC lifecycle retry — `modules/world/npc_ai.go`

Extract the RESPAWN/DESPAWN transition (currently inline in `turn`, lines 30-73)
into a method wrapped in a `recover` that retries on panic. The existing
PORTING-EXCEPTION comment block (npc_ai.go:53-55) is removed (deviation closed).

New method:

```go
// fireNpcLifecycle runs the once-per-cycle lifecycle transition (respawn /
// type-revert / despawn) under a recover that retries next tick on panic,
// mirroring TS Npc.ts:122-150 (try { … } catch { … this.setLifeCycle(1) }).
//
// Returns fired=true when a transition ran (respawn or despawn) so turn()
// skips this tick's movement — preserving the existing goscape behavior of
// not overwriting a teleport with a walk path on the transition tick.
//
// On panic the transition is logged and lifecycleTick is re-armed to 1 (TS
// setLifeCycle(1) — retry on the next tick) instead of letting the panic
// bubble to recoverNpc, which would evict the NPC via removeNpc(n,-1). This
// is the INNER of TS's two recovery layers: inner retry (Npc.ts:144-150)
// pre-empts outer evict (World.ts:681-690 → goscape recoverNpc). ARCH-1.
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
			n.dead = false
			prevX, prevZ, prevLevel := n.x, n.z, n.level
			n.x, n.z, n.level = n.startX, n.startZ, n.startLevel
			refreshNpcZone(s, n, prevX, prevZ, prevLevel)
			n.revertType()
		} else {
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
					s.npcEventQueue = append(s.npcEventQueue, NpcEventRequest{
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

`turn` (npc_ai.go) collapses the inline switch to a call:

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

The inline comments that documented the respawn-zone-refresh rationale
(npc_ai.go:37-43) and the skip-movement rationale (npc_ai.go:50-51) move into
`fireNpcLifecycle` (the zone-refresh note stays adjacent to `refreshNpcZone`;
the skip-movement note is captured by the `fired` doc comment).

**Import:** add `"runtime/debug"` to npc_ai.go (for `debug.Stack()`).

**Two-layer interaction (unchanged outer):** the `processNpcs` call site
(`tick.go:1158-1161`) keeps its `defer recoverNpc(...)`. After this change a
lifecycle panic is caught by `fireNpcLifecycle`'s inner recover and never
reaches `recoverNpc`; a panic anywhere else in `turn()` still bubbles to
`recoverNpc` and evicts (unchanged). This is the faithful TS two-layer shape.

### 3.2 (B) World-script queue retry — `modules/world/world_script_queue.go`

Change `processWorldQueue` from **remove-before-fire** to **fire, then remove on
clean (non-panicking) return; on panic leave queued for next-tick retry.** This
mirrors TS `World.ts:542-558`, where `request.unlink()` runs *after*
`ScriptRunner.execute(script)` and a throw skips the unlink.

```go
func (s *Server) processWorldQueue() {
	i := 0
	for i < len(s.worldScriptQueue) {
		entry := &s.worldScriptQueue[i]
		// POST-decrement (unchanged): capture current, then decrement.
		// Mirrors TS World.ts:535 `const delay = request.delay--`.
		delay := entry.delay
		entry.delay--
		if delay > 0 {
			i++
			continue
		}
		state := entry.script
		// Reset Execution=Running so script.Execute resumes the loop from
		// the post-WORLD_DELAY PC (unchanged convention; see prior comment).
		state.Execution = script.Running

		// Fire first; remove only on a clean (non-panicking) return.
		// Mirrors TS World.ts:542-558 where request.unlink() runs AFTER
		// ScriptRunner.execute — a throw that escapes execute skips the
		// unlink, leaving the entry to retry next tick. Normal script
		// errors are caught INSIDE execute (→ ABORTED → resumeOrFinishWorld
		// returns cleanly → removed here); only a genuine Go panic takes
		// the retry path. ARCH-1.
		if s.fireWorldScript(state) {
			// Panicked: leave the entry queued so it retries next tick.
			// Advance past it for the remainder of this pass.
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

**Re-entrancy / mid-pass append (preserved):** `resumeOrFinishWorld`'s
`WorldSuspended` branch calls `EnqueueWorldScript`, which `append`s a new entry
at the end of the slice. That happens during `fireWorldScript` (a clean return),
so the caller then removes the *old* entry at index `i`; the new entry sits at
the tail and is visited later in the same pass (the existing "speedup quirk").
Re-entrant appends never collide with the `[:i]/[i+1:]` splice because they land
at indices `>= len-before-append`. The slice may reallocate during fire, so
`state` is captured *before* the call (it already is); the `entry` pointer is
not used after the fire.

**Why advancing `i` on panic is safe (no infinite in-tick loop):** a panicking
entry is left in place and `i` advances past it, so the current pass terminates.
On the *next* tick the entry's `delay` has been decremented again (now negative,
so `delay > 0` is false) and it re-fires once — a per-tick retry, exactly like
TS, not a busy-loop.

### 3.3 (B) recovery helper — `modules/world/tick_recovery.go`

Replace `recoverWorldScript(state, log)` (which calls `recover()` itself and
swallows) with a value-taking logger, because the recover now lives in
`fireWorldScript` (it must, to set the named return `panicked`):

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

The old `recoverWorldScript` PORTING-EXCEPTION comment (tick_recovery.go:55-58)
is removed (deviation closed). `recoverObjDelayed` (C) and `recoverPlayer` and
`recoverNpc` are untouched.

### 3.4 (C) objDelayed — no change

`recoverObjDelayed` + `processObjDelayedQueue`'s remove-before-fire already
match TS `World.ts:566-572` (unlink before `addObj` → no retry). Leave as-is.
A one-line note is added to `recoverObjDelayed`'s comment clarifying that this
"remove-before-fire, no retry" is the TS-faithful behavior for objDelayed
(contrasting it with the world-queue's new retry), so a future reader does not
"fix" it to match the world queue.

---

## 4. Tests

New file: `modules/world/arch1_tick_recovery_test.go`. Plus targeted updates to
two existing tests whose names/comments become inaccurate.

### 4.1 (A) lifecycle retry

Deterministic panic source: `s.npcs` is a fixed `[16384]*Npc` **array** (always
present — a nil/empty fixture does NOT panic on `s.npcs[n.nid] = nil`). To force
a real Go panic inside the transition, use an **out-of-bounds `nid`**: the
DESPAWN branch of `removeNpc` executes `s.npcs[n.nid] = nil`, so `nid = 1 << 20`
(≥ 16384) raises an index-out-of-range panic. A bare
`&Server{log: discardLogger()}` fixture (matching the existing `TestRecoverNpc_*`
style; `zoneMap`/`gamemap`/`rsbuf`/`scriptProvider` all nil → their guards
short-circuit) supplies it. The out-of-range index stands in for *any*
transition-time fault — what is under test is the recover/retry wiring, not the
fault's origin.

1. **`TestFireNpcLifecycle_DespawnPanicRetries`** — `s := &Server{log: discardLogger()}`,
   `n := &Npc{nid: 1 << 20, typeId: 42, lifecycle: NpcLifecycleDespawn}`
   (dead=false). Call `fired := s.fireNpcLifecycle(n)`. Assert: `fired == true`,
   `n.lifecycleTick == 1` (re-armed = TS `setLifeCycle(1)` retry), and the test
   reaching its assertions proves the panic did **not** propagate. **This is the
   load-bearing ARCH-1 assertion: panic → retry-next-tick, not eviction.**

2. **`TestFireNpcLifecycle_DespawnCleanNoRetry`** — `s := &Server{log: discardLogger()}`,
   `n := &Npc{nid: 7, typeId: 42, lifecycle: NpcLifecycleDespawn}` (in-bounds nid,
   `s.scriptProvider` nil so no trigger fires). Pre-set `n.lifecycleTick = 0`
   (as it is at the call site after the decrement-to-0). Call
   `fired := s.fireNpcLifecycle(n)`. Assert: `fired == true`,
   `n.lifecycleTick == 0` (NOT re-armed — the clean path does not retry),
   `n.dead == true`. Proves the clean path is unaffected, and contrasts
   `lifecycleTick == 0` (clean) vs `== 1` (panic-retry).

3. **`TestNpcLifecyclePanic_InnerRecoverPreemptsEviction`** — run the npc through
   the same closure shape `processNpcs` uses, with the out-of-bounds-nid panic
   fixture from test 1 and `n.lifecycleTick = 1`:
   ```go
   func(n *Npc) {
       defer recoverNpc(n, s, "processNpcTurn", s.log)
       n.turn(s)
   }(n)
   ```
   Assert `n.lifecycleTick == 1`. Proves the inner recover handled the panic so
   `turn()` returned cleanly and `recoverNpc` (outer evict) never fired —
   pinning the two-layer interaction.

   (Note for the implementer: in the despawn-nil-map path `removeNpc` sets
   `n.dead = true` before the panic, so on retry the `if !n.dead` guard short-
   circuits the second attempt — this partial-execution-on-retry is inherent to
   the panic-retry model and matches TS; the test only asserts `lifecycleTick`.)

### 4.2 (B) world-queue retry

Reuse the world-queue test infra (`newTestServer`, `newReturnImmediatelyScript`,
`script.Init`, bytecode `ScriptFile`s) from `world_script_queue_test.go`.

Deterministic panic source: a real opcode that reaches `setActiveNpcSlot` /
`setActiveLocSlot` / `setActiveObjSlot` with `IntOperand == 2` (the
non-{0,1} value that those slot-setters `panic` on — see
`handlers_npc.go:183` etc.). The simplest reliable trigger is `NPC_FINDEXACT`
(or `NPC_FIND`) against an npc placed by `buildNpcForIntegration(t)`
(`npc_script_test.go:232`), with the find opcode's `IntOperand` set to `2`:

```
Opcodes:     PUSH_CONSTANT_INT(coord), PUSH_CONSTANT_INT(npcTypeId), NPC_FINDEXACT
IntOperands: coord,                    npcTypeId,                     2   // ← panics in setActiveNpcSlot
```

(Pop order for `NPC_FINDEXACT` is `npcTypeID := PopInt(); coord := PopInt()`, so
push coord first.) The find succeeds (npc present), then `setActiveNpcSlot` with
operand 2 panics inside `script.Execute` — a genuine escaped panic.

1. **`TestFireWorldScript_PanicReported`** — build the panicking state, call
   `panicked := s.fireWorldScript(state)`. Assert `panicked == true` and the
   call did not propagate. Mirror of the old `TestRecoverWorldScript_PanicSwallowed`,
   re-pointed at the new seam.

2. **`TestFireWorldScript_CleanReturnsFalse`** — `newReturnImmediatelyScript(t)`
   (OpReturn → Finished). Assert `s.fireWorldScript(state) == false`.

3. **`TestProcessWorldQueue_PanicRetriesNextTick`** — enqueue the panicking
   script with delay 0 (fires on the 2nd drain). Drain until it fires. Assert:
   after the firing drain the queue **still contains the entry** (len unchanged
   for that entry — retry), in contrast to a clean script which would be removed.
   Drain once more and assert it fires **again** (still queued). This is the
   load-bearing ARCH-1 (B) assertion.

4. **`TestProcessWorldQueue_CleanEntryRemovedAfterFire`** — a clean script is
   removed on the firing drain (queue length drops). Confirms remove-on-success.

### 4.3 Existing-test updates (names/comments now inaccurate)

- **`world_script_queue_test.go` `TestProcessWorldQueue_RemovedBeforeFire`**
  (lines 117-131): rename → `TestProcessWorldQueue_RemovedAfterCleanFire` and
  rewrite the comment to describe remove-after-clean-fire. Its assertion
  (queue length 0 after a clean fire) still holds unchanged.

- **`tick_recovery_test.go` `TestRecoverWorldScript_*`** (lines 196-247): these
  three tests target the deleted `recoverWorldScript` helper. Replace them with
  the `fireWorldScript` tests in 4.2 (the no-op/clean and panic cases), or
  delete them in favor of the new file's coverage. `TestRecoverWorldScript_NilStateSafe`'s
  nil-state defensiveness is preserved by `logWorldScriptPanic`'s `state != nil`
  guard — add a `TestLogWorldScriptPanic_NilStateSafe` if keeping that coverage.

### 4.4 Verification

- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...`
- Full suite: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
- Confirm no test pins the literal `PORTING-EXCEPTION` marker count (a repo grep
  found no numeric count assertion; verify after removing the two ARCH-1 markers).

---

## 5. Docs & bookkeeping

- **`PORTING.md`**: move the ARCH-1 open-deviations row (line ~32) and the
  audit-history row (line ~49) to reflect **FIXED** status; per project
  convention, relocate to `docs/PORTING-CLOSED.md` with the fix commit SHA(s),
  TS references (`Npc.ts:122-150`, `World.ts:534-558`), and the "panic-only
  retry; normal errors already matched" rationale from §2.
- **PORTING-EXCEPTION markers**: removed at `npc_ai.go:53` and
  `tick_recovery.go:55` (both ARCH-1). No other ARCH-1 markers exist.
- **Memory**: after landing, update `rev274_port_started.md` / `MEMORY.md` to
  note ARCH-1 closed (the last open rev-274 backlog row).

---

## 6. Scope & backport

Implement on **`rev-274`** only. ARCH-1 is a fidelity-**restoring** fix (goscape
diverged from its own pinned TS), so backporting to `rev-225/244/245.2/254` is
permitted by the no-forward-port policy (`[[no-forward-port-deviations]]`) —
each of those branches' pinned TS has the same `try/catch` retry shape. Backport
is an **optional follow-up** offered after rev-274 lands; it is **out of scope**
for this plan. (Note: the exact retry mechanics may differ slightly per branch
if an older pin's `Npc.ts` / `World.ts` differs — each backport must be verified
against that branch's own pin, not assumed identical.)

---

## 7. Files touched

| File | Change |
|------|--------|
| `modules/world/npc_ai.go` | extract `fireNpcLifecycle` with retry-on-panic; `turn` calls it; remove ARCH-1 PORTING-EXCEPTION; add `runtime/debug` import |
| `modules/world/world_script_queue.go` | `processWorldQueue` fire-then-remove-on-clean; add `fireWorldScript`; rewrite the remove-before-fire doc comment |
| `modules/world/tick_recovery.go` | replace `recoverWorldScript` → `logWorldScriptPanic` (value-taking); remove ARCH-1 PORTING-EXCEPTION; clarify `recoverObjDelayed` no-retry-is-faithful note |
| `modules/world/arch1_tick_recovery_test.go` | **new** — (A) and (B) retry tests |
| `modules/world/world_script_queue_test.go` | rename/recomment `TestProcessWorldQueue_RemovedBeforeFire` |
| `modules/world/tick_recovery_test.go` | replace `TestRecoverWorldScript_*` with `fireWorldScript` / `logWorldScriptPanic` tests |
| `PORTING.md` / `docs/PORTING-CLOSED.md` | ARCH-1 → FIXED/closed |
