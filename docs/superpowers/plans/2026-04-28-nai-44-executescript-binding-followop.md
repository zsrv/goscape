# NAI-44 — Full TS executeScript binding semantics + followOp Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close 3 deferred deviations by aligning goscape's script-resume dispatch with TS World.processWorld and porting TS Player.processInteraction L1200-1264 shape (with followOp chase semantics + auto-clear).

**Architecture:** Two-section sub-spec. Section A (T1-T2) edits `resumeOrFinish*` functions in script.go / npc_script.go to (a) drop defensive `ClearActiveScript` at WorldSuspended arms and (b) replace warn+drop with TS-faithful 3-arm cross-context rebind in `resumeOrFinishWorld`. Section B (T3-T5) reshapes `processInteraction` in interaction.go to mirror TS Player.ts:1200-1264 — `followOp` predicate, pre/post-step interact arms via new `tryInteract` helper, no-op `processWalktrigger` stub, and global auto-clear at TS L1261-1263. T6 closes the sub-spec with the cascade audit + deviation retirement + memory bump.

**Tech Stack:**
- Go 1.26+ (per `go_version.md`; modern Go syntax via the `use-modern-go` skill).
- TS source: `Engine-TS` only.
  - `src/engine/entity/Player.ts:1200-1264` (processInteraction reshape target).
  - `src/engine/entity/Player.ts:2125-2151` (executeScript reference; goscape's analogue is `resumeOrFinish`).
  - `src/engine/entity/Npc.ts:216-239` (Npc.executeScript reference).
  - `src/engine/World.ts:530-560` (`processWorld` world-queue dispatch — canonical shape for `resumeOrFinishWorld`).

**Pre-flight verified at HEAD `fe84be5`:**

| Spec audit | Result |
|---|---|
| `lastStepX`, `lastStepZ` exist on `*Player` | ✓ player.go:79; written at movement.go:84-85 |
| `stepsTaken` exists on `*Player` | ✓ player.go:96; incremented at movement.go:88; **never reset → T3 adds reset** |
| `followX`, `followZ` exist on `*Player` | ✓ player.go:97 (currently unused) |
| `targetX`, `targetZ` exist on `*Player` | ✓ player.go:98 (currently unused; not consumed in NAI-44) |
| `hasWaypoints()` helper | ✗ does not exist; goscape uses `p.waypointIndex >= 0`; **T3 adds helper** |
| `(p *Player).MessageGame(text string)` | ✓ used at handler_op_player.go:201 |
| `nextTarget` field on `*Player` | ✗ does not exist; **NAI-44 does NOT add it** — tag deviation inline |
| `script.ActivePlayer.StoreActiveScript(state)` interface method | ✓ used at script.go:116 |
| `script.ActiveNpc.StoreActiveScript(state)` interface method | ✓ used at npc_script.go:307 |
| Tick order: `processWorldQueue → processActiveScripts → processPathing → processInteractions` | ✓ tick.go:35-39 |
| `processActiveScripts` resume-gate is `Execution == Suspended` only | ✓ tick.go:213-214 (so cross-context rebound states won't double-fire from world queue + active loop) |
| Execution enum order | Running=0, Finished=1, Aborted=2, Suspended=3, CountDialog=4, PauseButton=5, NpcSuspended=6, WorldSuspended=7 (`pkg/script/execution.go:8-17`) |

`NAI-44-D-NO-LAST-STEP-COORDS` is **not opened** — fields exist. `NAI-44-D-PLAYER-WALKTRIGGER-NOOP`, `NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET`, `NAI-44-D-CONTINUEWALK-UNUSED` proceed as planned. `NAI-44-D-CANACCESS-NO-STUN-CHECK` and `NAI-44-D-NO-MODAL-CLOSE-ON-SCRIPT-FINISH` remain conditional, evaluated at T5 / T1 respectively.

---

## File structure

**Modified:**

- `modules/world/script.go` — T1 (delete defensive clear + dev-comment block in `resumeOrFinish` WorldSuspended arm) + T2 (replace warn+drop in `resumeOrFinishWorld` with 3-arm switch).
- `modules/world/npc_script.go` — T1 symmetric delete in `resumeOrFinishNpc` WorldSuspended arm.
- `modules/world/movement.go` — T3 (`stepsTaken = 0` reset at top of `resolveMovement`).
- `modules/world/interaction.go` — T3 (`hasWaypoints` helper + `processWalktrigger` stub) + T4 (`tryInteract` helper extraction) + T5 (`processInteraction` reshape).
- `modules/world/handler_op_player.go` — T6 (retire `NAI-40-D-OPPLAYER3-FOLLOWOP-NOT-PORTED` doc-comment block).

**Test files modified:**

- `modules/world/script_test.go` — T1 (player WorldSuspended no-clear test) + T2 (R5 regression).
- `modules/world/npc_script_test.go` — T1 (npc WorldSuspended no-clear test).
- `modules/world/world_script_queue_test.go` — T2 (4 cross-context rebind tests; existing test rename).
- `modules/world/interaction_test.go` — T3 (B7 stub test) + T4 (B5 regression) + T5 (B1-B4) + T6 (cascade-audit fixes).

**No new files.**

---

## Task 1: Drop defensive ClearActiveScript in WorldSuspended arms

**Files:**
- Modify: `modules/world/script.go:117-133`
- Modify: `modules/world/npc_script.go:303-319`
- Test: `modules/world/script_test.go`
- Test: `modules/world/npc_script_test.go`

Closes `NAI-37-D-WORLDSUSPEND-CLEARS-ACTIVE-SCRIPT`. Pure deletion of `ClearActiveScript()` calls + their preceding deviation-comment blocks at both player- and npc-path WorldSuspended arms.

- [ ] **Step 1: Write the failing test (player path)**

Add to `modules/world/script_test.go` (place after the last existing test in the file; if existing tests use a `setupServerForScript()`-style helper, mirror that).

```go
// TestResumeOrFinishWorldSuspendedDoesNotClearActiveScript pins NAI-44 T1:
// when a player-anchored script transitions to WorldSuspended, the
// player's activeScript slot retains the state pointer (TS Player.ts:2143-2150
// only nulls activeScript on FINISHED/ABORTED).
func TestResumeOrFinishWorldSuspendedDoesNotClearActiveScript(t *testing.T) {
	s := setupServerForScript(t)
	p := newTestPlayer(t, s, 1)

	sf := newWorldDelayScript(t, 5) // helper that builds a script popping delay=5 then setting Execution=WorldSuspended
	state := script.Init(sf, p, false, nil, nil)
	state.Provider = s.scriptProvider
	state.World = s.worldVars
	// Pre-condition: pretend the script was already stored as activeScript
	// (mimics the Suspended → WorldSuspended transition path).
	p.activeScript = state

	// Push delay onto the int stack so resumeOrFinish's WorldSuspended arm
	// can pop it (the WORLD_DELAY handler does this in production).
	state.PushInt(5)
	state.Execution = script.WorldSuspended

	s.resumeOrFinish(state, p)

	if p.activeScript != state {
		t.Errorf("activeScript: got %p, want %p (WorldSuspended must NOT clear)", p.activeScript, state)
	}
	if len(s.worldScriptQueue) != 1 {
		t.Errorf("worldScriptQueue length: got %d, want 1 (state should have been enqueued)", len(s.worldScriptQueue))
	}
}
```

If `setupServerForScript` / `newTestPlayer` / `newWorldDelayScript` helpers don't exist in the file, find the existing pattern by inspecting the first few tests in `script_test.go` and `world_script_queue_test.go`. Reuse, don't reinvent.

- [ ] **Step 2: Run test to verify it fails**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestResumeOrFinishWorldSuspendedDoesNotClearActiveScript ./modules/world/...
```

Expected: FAIL — `activeScript: got <nil>, want <ptr>` (current behavior calls `ClearActiveScript()`, nilling the slot).

- [ ] **Step 3: Apply the deletion at script.go:124-133**

Modify `modules/world/script.go`. The current WorldSuspended arm reads (L117-133):

```go
	case script.WorldSuspended:
		// NAI-37 T10: player-bound script suspended to world queue.
		// Pop the wakeup-tick (which the script's bytecode pushed
		// before WORLD_DELAY — see handlers_server.go:87-108) and
		// enqueue. The player no longer owns this script; it now
		// belongs to the world queue. Mirrors TS Player.ts:2135-2136.
		//
		// DEVIATION NAI-37-D-WORLDSUSPEND-CLEARS-ACTIVE-SCRIPT: TS does
		// NOT clear activePlayer.activeScript in this branch (see
		// Player.ts:2143-2150 — only Finished/Aborted clears). Goscape's
		// ClearActiveScript() is defensive against stale-pointer
		// double-execution if a previously-stored Suspended script
		// transitions to WorldSuspended. Closure when goscape ports the
		// full TS executeScript binding semantics.
		delay := state.PopInt()
		s.EnqueueWorldScript(state, delay)
		self.ClearActiveScript()
```

Replace with:

```go
	case script.WorldSuspended:
		// NAI-37 T10: player-bound script suspended to world queue.
		// Pop the wakeup-tick (which the script's bytecode pushed
		// before WORLD_DELAY — see handlers_server.go:87-108) and
		// enqueue. The player no longer owns this script's execution;
		// it now belongs to the world queue. Mirrors TS Player.ts:2135-2136.
		//
		// NAI-44: TS Player.executeScript (L2143-2150) only nulls
		// this.activeScript on FINISHED/ABORTED. Goscape's previous
		// defensive clear here was untracked-divergent; processActiveScripts
		// gates resume on Execution == Suspended (tick.go:213-214), so
		// holding the pointer is safe — the player's resume loop will
		// not re-fire a WorldSuspended state.
		delay := state.PopInt()
		s.EnqueueWorldScript(state, delay)
```

- [ ] **Step 4: Run player-path test to verify it passes**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestResumeOrFinishWorldSuspendedDoesNotClearActiveScript ./modules/world/...
```

Expected: PASS.

- [ ] **Step 5: Write the failing test (npc path)**

Add to `modules/world/npc_script_test.go`:

```go
// TestResumeOrFinishNpcWorldSuspendedDoesNotClearActiveScript — NAI-44 T1
// symmetric pin for the npc-path. TS Npc.ts:226-228 only nulls activeScript
// on FINISHED/ABORTED.
func TestResumeOrFinishNpcWorldSuspendedDoesNotClearActiveScript(t *testing.T) {
	s := setupServerForScript(t)
	n := newTestNpc(t, s, 1, 100)

	sf := newWorldDelayScript(t, 5)
	state := script.Init(sf, n, false, nil, nil)
	state.Provider = s.scriptProvider
	state.World = s.worldVars
	n.activeScript = state

	state.PushInt(5)
	state.Execution = script.WorldSuspended

	s.resumeOrFinishNpc(state, n)

	if n.activeScript != state {
		t.Errorf("activeScript: got %p, want %p (WorldSuspended must NOT clear)", n.activeScript, state)
	}
	if len(s.worldScriptQueue) != 1 {
		t.Errorf("worldScriptQueue length: got %d, want 1", len(s.worldScriptQueue))
	}
}
```

- [ ] **Step 6: Run npc-path test to verify it fails**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestResumeOrFinishNpcWorldSuspendedDoesNotClearActiveScript ./modules/world/...
```

Expected: FAIL.

- [ ] **Step 7: Apply the deletion at npc_script.go:308-319**

Current code (L308-319):

```go
	case script.WorldSuspended:
		// NAI-37: npc-bound script suspended to world queue. Symmetric
		// to resumeOrFinish (player path, T10). Mirrors TS Npc.ts:219-220.
		//
		// DEVIATION NAI-37-D-WORLDSUSPEND-CLEARS-ACTIVE-SCRIPT: TS does
		// NOT clear activeNpc.activeScript in this branch (see Npc.ts:226-228
		// — only Finished/Aborted clears via the script === this.activeScript
		// check). Goscape's ClearActiveScript() is defensive; same closure
		// as the player-path divergence.
		delay := state.PopInt()
		s.EnqueueWorldScript(state, delay)
		npc.ClearActiveScript()
```

Replace with:

```go
	case script.WorldSuspended:
		// NAI-37: npc-bound script suspended to world queue. Symmetric
		// to resumeOrFinish (player path). Mirrors TS Npc.ts:219-220.
		//
		// NAI-44: TS Npc.executeScript (L226-228) only nulls activeScript
		// on FINISHED/ABORTED. Same logic as the player-path: holding
		// the pointer is safe because Npc.turn() does not re-fire
		// WorldSuspended states.
		delay := state.PopInt()
		s.EnqueueWorldScript(state, delay)
```

- [ ] **Step 8: Run npc-path test to verify it passes**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestResumeOrFinishNpcWorldSuspendedDoesNotClearActiveScript ./modules/world/...
```

Expected: PASS.

- [ ] **Step 9: Run full module test to catch any regressions**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: PASS. If any pre-existing test fails, it will be one that asserts `p.activeScript == nil` or `n.activeScript == nil` after a WorldSuspended cycle — those are the stale defensive-clear assertions and need re-framing in T6 (cascade audit). For T1, expect zero failures (per `runescript_cadence.md` cleanup-as-you-go and `latent_bug_at_migration_boundary.md` we surface them now if any). If any DO fail, list them in the commit body and let T6's cascade audit close them.

- [ ] **Step 10: Commit**

```bash
git add modules/world/script.go modules/world/npc_script.go modules/world/script_test.go modules/world/npc_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-44 T1 — drop defensive ClearActiveScript in WorldSuspended arms

Closes NAI-37-D-WORLDSUSPEND-CLEARS-ACTIVE-SCRIPT. Player- and npc-path
resume functions (resumeOrFinish, resumeOrFinishNpc) no longer clear the
entity's activeScript slot when the script transitions to WorldSuspended;
matches TS Player.executeScript L2143-2150 / Npc.executeScript L226-228
which only null activeScript on FINISHED/ABORTED.

Holding the pointer is safe: processActiveScripts gates resume on
Execution == Suspended (tick.go:213-214), so a WorldSuspended state in
the player slot will not be re-fired by the active-script loop.

Two regression tests pin the new behavior (player + npc paths).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Port World-queue cross-context dispatch (TS-faithful 3-arm)

**Files:**
- Modify: `modules/world/script.go:142-183` (`resumeOrFinishWorld`)
- Test: `modules/world/world_script_queue_test.go` (4 new cross-context rebind tests; rename existing deviation-citing test)
- Test: `modules/world/script_test.go` (R5 Suspended-then-WorldSuspended regression)

Closes `NAI-37-D-WORLDQUEUE-CROSS-CONTEXT-DROP`. Replaces the warn+drop block at L171-177 with the TS-faithful 3-arm switch (`Suspended → state.Self.StoreActiveScript`, `NpcSuspended → state.ActiveNpc.StoreActiveScript`, `PauseButton/CountDialog → silent fall-through matching TS World.processWorld L530-560`).

- [ ] **Step 1: Write the failing tests (4 cross-context branches)**

Add to `modules/world/world_script_queue_test.go`. The file already has a `TestResumeOrFinishWorldXxx` family — mirror their setup pattern (find the existing helper for building a `*ScriptState` with a stubbed transition).

```go
// TestResumeOrFinishWorldSuspendedRebindsToSelf — NAI-44 T2.
// TS World.processWorld L547-549: world-queue script transitions to
// SUSPENDED → bind script.activePlayer.activeScript = script.
func TestResumeOrFinishWorldSuspendedRebindsToSelf(t *testing.T) {
	s := setupServerForScript(t)
	p := newTestPlayer(t, s, 1)

	state := newSuspendingScriptState(t, p, script.Suspended)

	s.resumeOrFinishWorld(state)

	if p.activeScript != state {
		t.Errorf("p.activeScript: got %p, want %p (Suspended must rebind to Self)", p.activeScript, state)
	}
	if len(s.worldScriptQueue) != 0 {
		t.Errorf("worldScriptQueue length: got %d, want 0 (entry already removed by caller)", len(s.worldScriptQueue))
	}
}

// TestResumeOrFinishWorldNpcSuspendedRebindsToActiveNpc — NAI-44 T2.
// TS World.processWorld L550-552: world-queue script transitions to
// NPC_SUSPENDED → bind script.activeNpc.activeScript = script.
func TestResumeOrFinishWorldNpcSuspendedRebindsToActiveNpc(t *testing.T) {
	s := setupServerForScript(t)
	n := newTestNpc(t, s, 1, 100)

	state := newSuspendingScriptStateWithActiveNpc(t, n, script.NpcSuspended)

	s.resumeOrFinishWorld(state)

	if n.activeScript != state {
		t.Errorf("n.activeScript: got %p, want %p (NpcSuspended must rebind to ActiveNpc)", n.activeScript, state)
	}
	if len(s.worldScriptQueue) != 0 {
		t.Errorf("worldScriptQueue length: got %d, want 0", len(s.worldScriptQueue))
	}
}

// TestResumeOrFinishWorldPauseButtonDropsSilently — NAI-44 T2.
// TS World.processWorld (World.ts:530-560) has NO branch for PauseButton;
// request.unlink() at L545 already removed the entry, so they fall through
// silently with no rebind and no warn.
func TestResumeOrFinishWorldPauseButtonDropsSilently(t *testing.T) {
	s, logs := setupServerForScriptWithLogCapture(t)
	p := newTestPlayer(t, s, 1)

	state := newSuspendingScriptState(t, p, script.PauseButton)

	s.resumeOrFinishWorld(state)

	if p.activeScript == state {
		t.Errorf("p.activeScript: got rebind, want nil (PauseButton must NOT rebind cross-context per TS)")
	}
	if len(s.worldScriptQueue) != 0 {
		t.Errorf("worldScriptQueue length: got %d, want 0", len(s.worldScriptQueue))
	}
	if got := logs.warnCount(); got != 0 {
		t.Errorf("warn log count: got %d, want 0 (silent drop matches TS)", got)
	}
}

// TestResumeOrFinishWorldCountDialogDropsSilently — NAI-44 T2. Same
// rationale as PauseButton.
func TestResumeOrFinishWorldCountDialogDropsSilently(t *testing.T) {
	s, logs := setupServerForScriptWithLogCapture(t)
	p := newTestPlayer(t, s, 1)

	state := newSuspendingScriptState(t, p, script.CountDialog)

	s.resumeOrFinishWorld(state)

	if p.activeScript == state {
		t.Errorf("p.activeScript: got rebind, want nil (CountDialog must NOT rebind cross-context per TS)")
	}
	if len(s.worldScriptQueue) != 0 {
		t.Errorf("worldScriptQueue length: got %d, want 0", len(s.worldScriptQueue))
	}
	if got := logs.warnCount(); got != 0 {
		t.Errorf("warn log count: got %d, want 0 (silent drop matches TS)", got)
	}
}
```

If a log-capture helper (`setupServerForScriptWithLogCapture`, returning a `*logCapture` with a `warnCount()` method) doesn't exist, build one as a 20-line helper at the bottom of the test file using `slog.NewTextHandler(&buf, ...)` and a count of substring `"level=WARN"` lines in `buf.String()`. Mirror the existing `discardLogger` test helper if present (search `rg "discardLogger" modules/world/`). Look for `tick_recovery_test.go` per the NAI-42 polish trail — it likely has the right pattern.

`newSuspendingScriptState(t, p, exec)` and `newSuspendingScriptStateWithActiveNpc(t, n, exec)` build a `*ScriptState` with `Self=p` (or `ActiveNpc=n`), `Pointers` set appropriately, and a synthetic `Script` whose Execute body sets `state.Execution = exec`. Look for the closest existing helper in the file; if none fits, build them from `script.Init(...)` + manual `Execute = exec` assignment without running the script (the `resumeOrFinishWorld` first calls `script.Execute(state)`, so we need the synthetic Execute to set the desired state — easiest approach is to pre-build the script with a single bytecode that sets Execution and returns).

If the synthetic script approach is awkward, a simpler shape: directly bypass `script.Execute` by stubbing the script's state transition. Look at how existing `world_script_queue_test.go:257-266` (the existing `WORLDQUEUE-CROSS-CONTEXT-DROP` test, per the spec memory) builds its synthetic script — replicate that pattern exactly.

- [ ] **Step 2: Run tests to verify all 4 fail**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestResumeOrFinishWorld(Suspended|NpcSuspended|PauseButton|CountDialog)" ./modules/world/...
```

Expected: FAIL. Suspended/NpcSuspended fail because no rebind. PauseButton/CountDialog fail because the existing warn+drop emits a WARN log (test expects warnCount() == 0).

- [ ] **Step 3: Replace `resumeOrFinishWorld`'s default branch**

In `modules/world/script.go`, the current `resumeOrFinishWorld` (L142-183) ends:

```go
	case script.WorldSuspended:
		delay := state.PopInt()
		s.EnqueueWorldScript(state, delay)
	case script.Suspended, script.NpcSuspended, script.PauseButton, script.CountDialog:
		// DEVIATION NAI-37-D-WORLDQUEUE-CROSS-CONTEXT-DROP: cross-context
		// resume from a world-queued script is not supported. TS would
		// re-bind to the corresponding entity's activeScript; goscape
		// drops with a warn until broader script-lifecycle alignment.
		s.log.Warn("world-queue script transitioned to cross-context state; resume unsupported",
			"script", state.Script.Name, "execution", state.Execution)
	default:
		// Running, or any future-added Execution value.
		s.log.Warn("world-queue script in unexpected execution state",
			"script", state.Script.Name, "execution", state.Execution)
	}
}
```

Also update the doc-comment at L142-158 (the dispatch table reference). Full replacement of the function body's switch arms:

```go
	case script.Finished, script.Aborted:
		// Clean exit; nothing to do (entry already removed by caller).
	case script.WorldSuspended:
		delay := state.PopInt()
		s.EnqueueWorldScript(state, delay)
	case script.Suspended:
		// TS World.ts:548-549 — bind to script.activePlayer.activeScript.
		// The "(probably not needed)" TS comment notes this case isn't
		// expected from world-queued scripts in practice, but the binding
		// exists for completeness.
		if state.Self != nil {
			state.Self.StoreActiveScript(state)
		} else {
			s.log.Warn("world-queue script Suspended with nil Self; dropping",
				"script", state.Script.Name)
		}
	case script.NpcSuspended:
		// TS World.ts:550-552 — bind to script.activeNpc.activeScript.
		if state.ActiveNpc != nil {
			state.ActiveNpc.StoreActiveScript(state)
		} else {
			s.log.Warn("world-queue script NpcSuspended with nil ActiveNpc; dropping",
				"script", state.Script.Name)
		}
	case script.PauseButton, script.CountDialog:
		// TS World.processWorld (World.ts:530-560) has NO branch for these
		// states. request.unlink() at L545 already removed the entry, so
		// they are silently dropped. Match TS by intentionally falling
		// through with no rebind and no warn.
	default:
		// Running, or any future-added Execution value.
		s.log.Warn("world-queue script in unexpected execution state",
			"script", state.Script.Name, "execution", state.Execution)
	}
}
```

Also update the function-level comment at L142-158 to remove the deviation citation. The new comment:

```go
// resumeOrFinishWorld dispatches the post-Execute state for a script
// run from the world-script queue (called by processWorldQueue after
// removing the entry).
//
// Dispatch table mirrors TS World.processWorld (World.ts:530-560):
//   - Finished, Aborted: clean exit (entry already unlink()'d).
//   - WorldSuspended: re-enqueue (self-loop). Pops the wakeup-tick from
//     the script's int stack and re-appends to worldScriptQueue.
//   - Suspended: rebind to state.Self.activeScript (TS L548-549).
//   - NpcSuspended: rebind to state.ActiveNpc.activeScript (TS L550-552).
//   - PauseButton, CountDialog: silent fall-through (TS World.processWorld
//     has no branch for these; matches TS behavior).
//   - default (Running, future-added): warn+drop.
//
// NAI-44 closure of NAI-37-D-WORLDQUEUE-CROSS-CONTEXT-DROP.
```

- [ ] **Step 4: Run the 4 tests to verify all pass**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run "TestResumeOrFinishWorld(Suspended|NpcSuspended|PauseButton|CountDialog)" ./modules/world/...
```

Expected: PASS.

- [ ] **Step 5: Update existing deviation-tagged test**

Find the existing test at `world_script_queue_test.go:257-266` that asserts the warn+drop behavior (per spec memory: `"warn+drop per NAI-37-D-WORLDQUEUE-CROSS-CONTEXT-DROP"`). Re-frame it as a positive-rebind test or delete it if redundant with the 4 new tests. Read the test first; if it covers a unique state-transition path not in the 4 new tests, retain it with updated assertions. If it's a pure duplicate, delete.

- [ ] **Step 6: Write R5 regression test (Suspended-then-WorldSuspended)**

Add to `modules/world/script_test.go`:

```go
// TestSuspendedThenWorldSuspendedNoDoubleFire — NAI-44 T2 R5 regression.
// Pre-NAI-44, the defensive ClearActiveScript at the WorldSuspended arm
// guarded against double-fire if the same state pointer was held by both
// the player slot and the world queue. NAI-44 deletes that clear.
//
// Verify the gating logic still prevents double-fire: a state with
// Execution == WorldSuspended in the player's activeScript slot is NOT
// re-fired by processActiveScripts (which gates on Execution == Suspended
// only, tick.go:213-214).
func TestSuspendedThenWorldSuspendedNoDoubleFire(t *testing.T) {
	s := setupServerForScript(t)
	p := newTestPlayer(t, s, 1)

	state := newWorldDelaySuspendedState(t, p) // helper: state.Execution=WorldSuspended, delay=5
	p.activeScript = state // simulate prior Suspended → A1's no-clear leaves slot pointing here
	s.EnqueueWorldScript(state, 5)

	beforeExecuteCallCount := state.executeCallCount() // helper or use a counter on the synthetic script

	s.processActiveScripts() // tick.go:36

	if got := state.executeCallCount(); got != beforeExecuteCallCount {
		t.Errorf("processActiveScripts re-fired a WorldSuspended state: callCount %d → %d, want unchanged", beforeExecuteCallCount, got)
	}
	if p.activeScript != state {
		t.Errorf("p.activeScript: got %p, want %p (slot must remain pointing at state)", p.activeScript, state)
	}
}
```

If `newWorldDelaySuspendedState` and the executeCallCount infrastructure don't exist, simplify by checking `state.Execution` is unchanged after processActiveScripts (a re-fire would change it). Adjust the assertion accordingly — the goal is "processActiveScripts did NOT call resumeOrFinish on this state."

- [ ] **Step 7: Run R5 regression to verify it passes**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestSuspendedThenWorldSuspendedNoDoubleFire ./modules/world/...
```

Expected: PASS (gating logic at tick.go:213-214 already filters; the test confirms the invariant holds without the defensive clear).

- [ ] **Step 8: Run full module test to catch regressions**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: PASS. If existing tests fail, see Step 9 of T1 for triage.

- [ ] **Step 9: Commit**

```bash
git add modules/world/script.go modules/world/world_script_queue_test.go modules/world/script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-44 T2 — TS-faithful world-queue cross-context dispatch

Closes NAI-37-D-WORLDQUEUE-CROSS-CONTEXT-DROP. Replaces resumeOrFinishWorld's
warn+drop block for Suspended/NpcSuspended/PauseButton/CountDialog with
TS World.processWorld L530-560's actual 3-arm shape:

  - Suspended → state.Self.StoreActiveScript(state)  (TS L548-549)
  - NpcSuspended → state.ActiveNpc.StoreActiveScript(state)  (TS L550-552)
  - PauseButton, CountDialog → silent fall-through (TS has no branch)

Re-frames the original deviation: TS World.processWorld does NOT route
through Player.executeScript / Npc.executeScript (only ScriptRunner.execute
inline). The earlier comment cited Player.ts:2137-2141 + Npc.ts:221-225 as
the rebind reference, but those are executeScript-path; the world-queue
path drops Pause/Count with no rebind.

Per tracker_entry_framing_can_be_incomplete.md.

4 cross-context rebind tests + 1 R5 regression (Suspended-then-WorldSuspended
no-double-fire) pin the new behavior.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Foundation — hasWaypoints + processWalktrigger + stepsTaken reset

**Files:**
- Modify: `modules/world/movement.go:34` (add `p.stepsTaken = 0` at top of `resolveMovement`)
- Modify: `modules/world/interaction.go` (add `hasWaypoints` and `processWalktrigger`)
- Test: `modules/world/interaction_test.go` (B7 + stepsTaken reset test)

Pure foundation: no behavior change to existing trigger flows. Lays the helpers + invariants T4 and T5 depend on.

- [ ] **Step 1: Write failing test for stepsTaken reset**

Add to `modules/world/interaction_test.go` (or `movement_test.go` if more appropriate; pick by reading the existing test files). Look for existing `resolveMovement` tests first; if a test file `movement_test.go` exists, add there.

```go
// TestResolveMovementResetsStepsTaken — NAI-44 T3.
// stepsTaken must be reset at the start of each tick's movement cycle so
// processInteraction (which runs after processPathing) reads the per-tick
// step count, not a cumulative total. Goscape's stepsTaken increment at
// movement.go:88 has no consumer pre-NAI-44; T5's processInteraction port
// is the first reader.
func TestResolveMovementResetsStepsTaken(t *testing.T) {
	p := newPlayerForMovementTest(t) // existing helper; or build minimal
	p.stepsTaken = 5 // simulate cumulative count from prior tick

	p.resolveMovement()

	// resolveMovement returns early on waypointIndex < 0 (no path), so
	// stepsTaken should be reset to 0 and stay there (no steps taken).
	if p.stepsTaken != 0 {
		t.Errorf("stepsTaken: got %d, want 0 (reset at top of resolveMovement)", p.stepsTaken)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestResolveMovementResetsStepsTaken ./modules/world/...
```

Expected: FAIL — `stepsTaken: got 5, want 0` (no reset at HEAD).

- [ ] **Step 3: Add the reset at top of `resolveMovement`**

Modify `modules/world/movement.go`. Current `resolveMovement` (L34-61):

```go
func (p *Player) resolveMovement() {
	p.lastTickX = p.x
	p.lastTickZ = p.z
	p.lastLevel = p.level

	if p.waypointIndex < 0 {
		p.walkDir = -1
		p.runDir = -1
		return
	}
	...
```

Insert the reset BEFORE the existing body:

```go
func (p *Player) resolveMovement() {
	// NAI-44 T3: stepsTaken accumulates per-step in stepOnce (movement.go:88).
	// Reset at start of each tick's movement cycle so processInteraction
	// (which runs after processPathing in tick.go:38-39) reads the
	// per-tick step count. TS Player.processInteraction reads
	// stepsTaken === 0 to gate post-step retry timing (Player.ts:1245).
	p.stepsTaken = 0

	p.lastTickX = p.x
	p.lastTickZ = p.z
	p.lastLevel = p.level

	if p.waypointIndex < 0 {
		p.walkDir = -1
		p.runDir = -1
		return
	}
	...
```

- [ ] **Step 4: Run reset test to verify it passes**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestResolveMovementResetsStepsTaken ./modules/world/...
```

Expected: PASS.

- [ ] **Step 5: Write failing test for hasWaypoints helper**

Add to `modules/world/interaction_test.go`:

```go
// TestHasWaypoints — NAI-44 T3 helper. Returns true iff the player has
// active waypoints; goscape's existing convention is waypointIndex == -1
// for "no waypoints" (vs >= 0 for "active path").
func TestHasWaypoints(t *testing.T) {
	tests := []struct {
		name          string
		waypointIndex int
		want          bool
	}{
		{"no path", -1, false},
		{"single step path", 0, true},
		{"multi-step path", 5, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Player{waypointIndex: tt.waypointIndex}
			if got := p.hasWaypoints(); got != tt.want {
				t.Errorf("hasWaypoints: got %v, want %v (waypointIndex=%d)", got, tt.want, tt.waypointIndex)
			}
		})
	}
}
```

- [ ] **Step 6: Run test to verify it fails (compile error)**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestHasWaypoints ./modules/world/...
```

Expected: FAIL — compile error `p.hasWaypoints undefined`.

- [ ] **Step 7: Add `hasWaypoints` method**

Append to `modules/world/interaction.go` (after `processInteraction`'s closing brace, before `inOperableDistance`):

```go
// hasWaypoints reports whether the player has an active waypoint queue.
// Goscape's convention: waypointIndex == -1 means no path; >= 0 means
// active. Mirrors TS Player.hasWaypoints(); the predicate is consumed by
// processInteraction's pre/post-step arms.
func (p *Player) hasWaypoints() bool {
	return p.waypointIndex >= 0
}
```

- [ ] **Step 8: Run hasWaypoints test to verify it passes**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestHasWaypoints ./modules/world/...
```

Expected: PASS (all 3 subtests).

- [ ] **Step 9: Write failing test for processWalktrigger no-op stub**

Add to `modules/world/interaction_test.go`:

```go
// TestProcessWalktriggerNoOp — NAI-44 T3 / B7. processWalktrigger is a
// stub for TS-faithful processInteraction shape (TS Player.ts:1219-1234).
// Goscape has no walktrigger consumer (NAI-37-D-WALKTRIGGER-NOREADER on
// the Npc side; NAI-44-D-PLAYER-WALKTRIGGER-NOOP on the Player side).
// The empty stub must not panic and must not mutate Player state.
func TestProcessWalktriggerNoOp(t *testing.T) {
	p := newPlayerForInteractionTest(t)
	beforeX, beforeZ, beforeLevel := p.x, p.z, p.level
	beforeWaypointIndex := p.waypointIndex
	beforeTarget := p.target

	p.processWalktrigger()

	if p.x != beforeX || p.z != beforeZ || p.level != beforeLevel {
		t.Errorf("processWalktrigger: coords mutated: was (%d,%d,%d), got (%d,%d,%d)",
			beforeX, beforeZ, beforeLevel, p.x, p.z, p.level)
	}
	if p.waypointIndex != beforeWaypointIndex {
		t.Errorf("processWalktrigger: waypointIndex mutated: was %d, got %d", beforeWaypointIndex, p.waypointIndex)
	}
	if p.target != beforeTarget {
		t.Errorf("processWalktrigger: target mutated: was %v, got %v", beforeTarget, p.target)
	}
}
```

`newPlayerForInteractionTest(t)` — find the existing helper in `interaction_test.go`. Likely something like `newPlayerAt(t, 3200, 3200, 0)`.

- [ ] **Step 10: Run test to verify it fails (compile error)**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestProcessWalktriggerNoOp ./modules/world/...
```

Expected: FAIL — `p.processWalktrigger undefined`.

- [ ] **Step 11: Add `processWalktrigger` stub**

Append to `modules/world/interaction.go` (after `hasWaypoints`):

```go
// processWalktrigger is the per-tick walktrigger consumption hook
// invoked by processInteraction's pre-step and post-step arms.
//
// DEVIATION NAI-44-D-PLAYER-WALKTRIGGER-NOOP: TS Player.ts:1219-1234
// calls processWalktrigger which dispatches the player's queued
// walktrigger script. Goscape has no walktrigger consumer yet (sibling
// to NAI-37-D-WALKTRIGGER-NOREADER on the Npc side at npc.go:92).
// Empty no-op preserves TS-faithful processInteraction shape so a
// future consumer can wire here without further reshape.
func (p *Player) processWalktrigger() {}
```

- [ ] **Step 12: Run no-op test to verify it passes**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestProcessWalktriggerNoOp ./modules/world/...
```

Expected: PASS.

- [ ] **Step 13: Run full module test**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: PASS. If `stepsTaken=0` reset breaks any existing test, that test was depending on accumulation across ticks, which was unintentional. Re-frame at T6.

- [ ] **Step 14: Commit**

```bash
git add modules/world/movement.go modules/world/interaction.go modules/world/interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-44 T3 — foundation: hasWaypoints + processWalktrigger + stepsTaken reset

T3 lays the helpers and invariants T4/T5 depend on:

  - hasWaypoints() bool — goscape uses waypointIndex == -1 for "no path";
    helper wraps the convention. TS Player.hasWaypoints() equivalent.
  - processWalktrigger() — empty no-op stub. NAI-44-D-PLAYER-WALKTRIGGER-NOOP:
    sibling to NAI-37-D-WALKTRIGGER-NOREADER on the Npc side; preserves
    TS-faithful processInteraction shape so a consumer can wire here later.
  - stepsTaken = 0 reset at top of resolveMovement (movement.go:34).
    stepsTaken increments per-step at movement.go:88; TS Player.ts:1245
    reads stepsTaken === 0 to gate post-step retry timing. Pre-NAI-44,
    stepsTaken accumulated indefinitely with no reader; T5's processInteraction
    port is the first consumer.

3 tests pin the foundation. No behavior change to existing trigger flows.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Extract `tryInteract` helper from processInteraction

**Files:**
- Modify: `modules/world/interaction.go` (extract `tryInteract`; processInteraction unchanged behaviorally)
- Test: `modules/world/interaction_test.go` (B5 baseline — existing tests must continue to pass)

Pure refactor. `tryInteract` consolidates the operable/approach distance dispatch from the current processInteraction body into a shared helper. processInteraction temporarily delegates to it; T5 then reshapes processInteraction around it.

- [ ] **Step 1: Add `tryInteract` to `interaction.go`**

Append to `modules/world/interaction.go` (after `processWalktrigger` from T3):

```go
// tryInteract is the contact/approach-distance dispatch unifying the
// OP and AP arms that processInteraction previously inlined.
// Returns true when an OP or AP trigger fired this tick.
//
// continueWalk is reserved for TS Player.ts:1245's stepsTaken-aware
// retry timing. Goscape's per-tick movement order makes it currently a
// no-op (the post-step arm only runs once anyway).
//
// DEVIATION NAI-44-D-CONTINUEWALK-UNUSED: parameter kept for symmetry
// with TS shape; closure is dead-API-polish at next sub-spec close per
// dead_api_polish.md if no consumer materializes.
func (p *Player) tryInteract(continueWalk bool) bool {
	tx, tz, _ := p.target.Coords()
	if inOperableDistance(p.x, p.z, tx, tz) {
		p.interacted = true
		if !p.interactionFired {
			tryFireOpTrigger(p)
		}
		return true
	}
	if inApproachDistance(p.x, p.z, tx, tz, effectiveApRange(p)) {
		p.interacted = true
		if !p.interactionFired {
			tryFireApTrigger(p)
		}
		return true
	}
	_ = continueWalk
	return false
}
```

- [ ] **Step 2: Replace processInteraction's distance branches with `tryInteract` calls**

Modify `modules/world/interaction.go`. Current `processInteraction` body (L98-150):

```go
func (p *Player) processInteraction() {
	if p.target == nil {
		return
	}
	if p.client == nil || p.client.server == nil {
		return
	}
	s := p.client.server
	if p.delayed && s.currentTick < p.delayedUntil {
		return
	}

	tx, tz, tlevel := p.target.Coords()
	if tlevel != p.level {
		p.ClearInteraction()
		sendUnsetMapFlag(p)
		return
	}

	if inOperableDistance(p.x, p.z, tx, tz) {
		// Contact range — fire OP. Matches TS Player.ts:1123-1135 (OP
		// checked before AP at contact). NAI-41 moved the faceEntity
		// write to SetInteraction time; no contact-time write needed.
		p.interacted = true
		if !p.interactionFired {
			tryFireOpTrigger(p)
		}
		return
	}

	if inApproachDistance(p.x, p.z, tx, tz, effectiveApRange(p)) {
		// Approach range — fire AP. Matches TS Player.ts:1139-1170.
		// S6l-D1 closed in S6r: when fireApTriggerLoc finds no script,
		// it sets p.apRange = -1. Next tick's inApproachDistance sees
		// apRange <= 0 and returns false, skipping re-lookup.
		p.interacted = true
		if !p.interactionFired {
			tryFireApTrigger(p)
		}
		return
	}

	if !p.repathed {
		p.pathToTarget(tx, tz)
		p.repathed = true
	}
}
```

Replace with the **transitional shape** (T5 reshapes again):

```go
func (p *Player) processInteraction() {
	if p.target == nil {
		return
	}
	if p.client == nil || p.client.server == nil {
		return
	}
	s := p.client.server
	if p.delayed && s.currentTick < p.delayedUntil {
		return
	}

	_, _, tlevel := p.target.Coords()
	if tlevel != p.level {
		p.ClearInteraction()
		sendUnsetMapFlag(p)
		return
	}

	if p.tryInteract(false) {
		return
	}

	if !p.repathed {
		tx, tz, _ := p.target.Coords()
		p.pathToTarget(tx, tz)
		p.repathed = true
	}
}
```

- [ ] **Step 3: Run full module test to verify zero regression**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: PASS (pure refactor; tryInteract returns the same bool that the inlined branches' `return` would have communicated; flow is preserved). If any existing test fails, the refactor introduced a bug — **do not proceed to T5**; debug `tryInteract` until parity is restored.

- [ ] **Step 4: Run with race detector**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/interaction.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(world): NAI-44 T4 — extract tryInteract helper from processInteraction

Pure refactor; no behavior change. Extracts the operable/approach distance
dispatch from processInteraction's inlined branches into a shared helper:

  tryInteract(continueWalk bool) bool

T5 reshapes processInteraction around this helper to add followOp +
auto-clear semantics. Splitting the extraction into its own task keeps
T5's diff focused on the actual behavior change.

continueWalk is reserved for TS Player.ts:1245 stepsTaken-aware retry
timing; tagged NAI-44-D-CONTINUEWALK-UNUSED for symmetry.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Reshape processInteraction with followOp + auto-clear

**Files:**
- Modify: `modules/world/interaction.go` (`processInteraction` reshape — TS Player.ts:1200-1264 structural port)
- Test: `modules/world/interaction_test.go` (B1 + B2 + B3 + B4: followOp predicate + chase + waypoint exhaustion + contact fire)

Closes `NAI-40-D-OPPLAYER3-FOLLOWOP-NOT-PORTED`. **Highest blast-radius task.** Controller pre-flight verification per `controller_preflight.md` mandatory before dispatch.

**Pre-dispatch controller checks:**
- Re-grep: `rg "p\.target\s*!=\s*nil|p\.target\s*==\s*nil" modules/world/*_test.go` — list every assertion site for the cascade audit at T6.
- Verify `effectiveApRange(p)`, `inOperableDistance`, `inApproachDistance` signatures unchanged from T4.
- Verify `p.MessageGame(text string)` callable (not `messageGame`).
- Verify `p.target` typed as `entity` (interface) — type-switch `_, ok := p.target.(*Player)` works.

- [ ] **Step 1: Write the followOp predicate test (B1)**

Add to `modules/world/interaction_test.go`:

```go
// TestFollowOpPredicate — NAI-44 T5 / B1. followOp = (targetOp == 3 &&
// target is *Player). TS Player.ts:1205 uses ServerTriggerType enum
// (APPLAYER3/OPPLAYER3 are sibling values); goscape stores raw op slot
// 1..4, so a single equality check covers both AP and OP variants.
func TestFollowOpPredicate(t *testing.T) {
	tests := []struct {
		name      string
		targetOp  int
		buildTarget func(t *testing.T, s *Server) entity
		wantFollow bool
	}{
		{"OPPLAYER3 → followOp",
			3,
			func(t *testing.T, s *Server) entity { return newTestPlayer(t, s, 2) },
			true,
		},
		{"OPPLAYER1 → not followOp",
			1,
			func(t *testing.T, s *Server) entity { return newTestPlayer(t, s, 2) },
			false,
		},
		{"OPNPC3 (op=3, *Npc target) → not followOp",
			3,
			func(t *testing.T, s *Server) entity { return newTestNpc(t, s, 1, 100) },
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := setupServerForInteractionTest(t)
			p := newTestPlayer(t, s, 1)
			target := tt.buildTarget(t, s)
			p.SetInteraction(InteractionEngine, target, tt.targetOp, -1)

			got := isFollowOp(p)

			if got != tt.wantFollow {
				t.Errorf("followOp: got %v, want %v (targetOp=%d, target type=%T)", got, tt.wantFollow, tt.targetOp, target)
			}
		})
	}
}
```

The test references a package-level helper `isFollowOp(p *Player) bool` — exporting the predicate as a tiny function makes it independently testable. The reshaped `processInteraction` calls `isFollowOp(p)`.

- [ ] **Step 2: Run test to verify it fails (compile error)**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestFollowOpPredicate ./modules/world/...
```

Expected: FAIL — `isFollowOp undefined`.

- [ ] **Step 3: Reshape `processInteraction` with followOp + auto-clear**

Modify `modules/world/interaction.go`. Replace the **entire** `processInteraction` body (the transitional shape from T4) with the TS-faithful reshape, and add `isFollowOp` as a package-level helper:

```go
// isFollowOp reports whether the current interaction is in chase-the-target
// mode. TS Player.ts:1205: followOp = targetOp == APPLAYER3 || OPPLAYER3.
// Goscape's targetOp is the raw op slot 1..4 (interaction.go:56), so a
// single equality check covers both AP and OP variants of slot 3. Player
// targets only — OPLOC/OPNPC/OPOBJ slot-3 ops are unrelated to the
// player→player chase semantics.
func isFollowOp(p *Player) bool {
	if p.targetOp != 3 {
		return false
	}
	_, ok := p.target.(*Player)
	return ok
}

// processInteraction runs once per tick per player after pathing.
// Mirrors TS Player.processInteraction (Player.ts:1200-1264).
//
// Branch summary:
//   - No target / no client / delayed: no-op.
//   - Target on different level: clear + UnsetMapFlag (subset of TS
//     validateTarget; goscape has no isValid()-style alive/visible
//     registry).
//   - Pre-step arm: walktrigger (skipped when followOp) + tryInteract.
//   - If pre-step did not interact: repath, post-step walktrigger (if
//     waypoints), waypoint-exhaustion clear (if followOp), post-step
//     tryInteract (skipped when followOp).
//   - Auto-clear: interacted && !apRangeCalled → ClearInteraction
//     (TS L1261-1263).
//
// Goscape's updateMovement runs in processPathing (tick.go:38), BEFORE
// processInteractions (tick.go:39). TS embeds it inline at L1241; the
// order-of-operations difference is by goscape design.
func (p *Player) processInteraction() {
	if p.target == nil {
		return
	}
	if p.client == nil || p.client.server == nil {
		return
	}
	s := p.client.server
	if p.delayed && s.currentTick < p.delayedUntil {
		return
	}

	// TS L1201-1202.
	p.followX = p.lastStepX
	p.followZ = p.lastStepZ
	// TS L1203 (this.nextTarget = null) — DEVIATION NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET:
	// goscape's p_op* opcodes do immediate SetInteraction swaps rather
	// than queueing a nextTarget for next-tick application. No nextTarget
	// field exists on *Player; the reshape below has no nextTarget block.
	// Closure: future p_op* opcode reshape sub-spec.

	followOp := isFollowOp(p)

	_, _, tlevel := p.target.Coords()
	if tlevel != p.level {
		p.ClearInteraction()
		sendUnsetMapFlag(p)
		return
	}

	interacted := false

	// Pre-step interact arm (TS L1209-1224).
	// canAccess() is approximated by the delayed-gate already passed
	// above. DEVIATION NAI-44-D-CANACCESS-NO-STUN-CHECK: TS canAccess()
	// also tests stun/freeze; goscape has no stun system, so the
	// !p.delayed subset is the in-tree approximation.
	if !followOp {
		p.processWalktrigger()
	}
	interacted = p.tryInteract(false)

	// Post-step arm (TS L1227-1252). Skipped when pre-step interacted.
	if !interacted {
		// Recalc path (TS L1228-1229).
		if !p.repathed {
			tx, tz, _ := p.target.Coords()
			p.pathToTarget(tx, tz)
			p.repathed = true
		}

		if p.hasWaypoints() {
			p.processWalktrigger()
		}

		// followOp + waypoint exhaustion → clear (TS L1237-1239).
		if !p.hasWaypoints() && followOp {
			p.ClearInteraction()
		}

		// Post-step interact (TS L1244-1252). Skipped when followOp
		// (the chase keeps interaction anchored across steps).
		if p.target != nil && !followOp {
			interacted = p.tryInteract(p.stepsTaken == 0)
			if !interacted && !p.hasWaypoints() && p.stepsTaken == 0 {
				p.MessageGame("I can't reach that!")
				p.ClearInteraction()
			}
		}
	}

	// Auto-clear (TS L1261-1263). NAI-44 closure of
	// NAI-40-D-OPPLAYER3-FOLLOWOP-NOT-PORTED's auto-clear gap.
	// Note: followOp paths can still reach this when tryInteract returned
	// true at the pre-step arm (contact range with target=*Player op=3).
	// TS does the same — followOp gates SKIP_post-step-interact, not
	// the auto-clear itself.
	if interacted && !p.apRangeCalled {
		p.ClearInteraction()
	}
}
```

- [ ] **Step 4: Run B1 followOp predicate test to verify it passes**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestFollowOpPredicate ./modules/world/...
```

Expected: PASS (all 3 subtests).

- [ ] **Step 5: Write B2 — followOp anchored chase test**

Add to `modules/world/interaction_test.go`:

```go
// TestFollowOpAnchoredChase — NAI-44 T5 / B2. When OPPLAYER3 fires with
// the target out of operable/approach range, the player path-walks toward
// the target. processInteraction must NOT clear the interaction in this
// scenario (followOp keeps interaction anchored across steps).
func TestFollowOpAnchoredChase(t *testing.T) {
	s := setupServerForInteractionTest(t)
	clicker := newTestPlayerAt(t, s, 1, 3200, 3200, 0)
	target := newTestPlayerAt(t, s, 2, 3210, 3200, 0) // 10 tiles east — out of range

	clicker.SetInteraction(InteractionEngine, target, 3, -1)

	clicker.processInteraction()

	if clicker.target != target {
		t.Errorf("target: got %v, want %v (followOp must NOT auto-clear when chasing)", clicker.target, target)
	}
	if clicker.targetOp != 3 {
		t.Errorf("targetOp: got %d, want 3", clicker.targetOp)
	}
	if !clicker.hasWaypoints() {
		t.Error("hasWaypoints: got false, want true (path should be set toward target)")
	}
}
```

- [ ] **Step 6: Run B2 to verify it passes**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestFollowOpAnchoredChase ./modules/world/...
```

Expected: PASS.

- [ ] **Step 7: Write B3 — followOp waypoint exhaustion test**

```go
// TestFollowOpWaypointExhaustion — NAI-44 T5 / B3. When followOp is
// active and pathToTarget yields no waypoints (e.g. target unreachable),
// the post-step arm clears the interaction (TS L1237-1239).
func TestFollowOpWaypointExhaustion(t *testing.T) {
	s := setupServerForInteractionTest(t)
	clicker := newTestPlayerAt(t, s, 1, 3200, 3200, 0)
	target := newTestPlayerAt(t, s, 2, 3210, 3200, 0)

	clicker.SetInteraction(InteractionEngine, target, 3, -1)
	// Force waypoint exhaustion: stub pathToTarget to leave waypointIndex < 0.
	// In practice, pathfinder returns no route on unreachable targets; the
	// test setup must produce that condition. Easiest: use a server with no
	// gamemap (Pathfinder.FindPathDefault returns no route → queueWaypoints
	// receives empty slice → waypointIndex set to -1).
	// setupServerForInteractionTest may already produce such a server;
	// if not, add a `setupServerWithoutGamemap(t)` variant.
	clicker.waypointIndex = -1
	clicker.repathed = true // skip the repath in processInteraction; force post-step arm to see no waypoints

	clicker.processInteraction()

	if clicker.target != nil {
		t.Errorf("target: got %v, want nil (followOp + no waypoints must ClearInteraction)", clicker.target)
	}
}
```

The `clicker.repathed = true` shortcut sidesteps the `pathToTarget` call inside processInteraction. If the test feels brittle, an alternative is a helper `setupServerWithoutGamemap` that returns a pathfinder-less server so `pathToTarget` becomes a no-op leaving `waypointIndex == -1`. Use whichever produces a cleaner test; both are valid.

- [ ] **Step 8: Run B3 to verify it passes**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestFollowOpWaypointExhaustion ./modules/world/...
```

Expected: PASS.

- [ ] **Step 9: Write B4 — followOp contact fire test**

```go
// TestFollowOpContactFire — NAI-44 T5 / B4. OPPLAYER3 with target in
// operable distance: pre-step tryInteract fires the OP trigger, sets
// p.interacted=true, p.interactionFired=true. The auto-clear gate at
// L1261-1263 evaluates `interacted && !apRangeCalled` → ClearInteraction.
// followOp does NOT gate the auto-clear; it only gates post-step-interact.
func TestFollowOpContactFire(t *testing.T) {
	s := setupServerForInteractionTest(t)
	clicker := newTestPlayerAt(t, s, 1, 3200, 3200, 0)
	target := newTestPlayerAt(t, s, 2, 3201, 3200, 0) // adjacent — operable distance

	clicker.SetInteraction(InteractionEngine, target, 3, -1)

	clicker.processInteraction()

	if !clicker.interactionFired {
		t.Error("interactionFired: got false, want true (OP trigger should fire at contact)")
	}
	// Auto-clear gate fires (interacted && !apRangeCalled).
	if clicker.target != nil {
		t.Errorf("target: got %v, want nil (auto-clear at TS L1261-1263)", clicker.target)
	}
}
```

Note: `tryFireOpTrigger` may not actually find a registered OPPLAYER3 script in the test fixture; verify the test's expected `interactionFired=true` against the existing trigger-firing test pattern. If goscape sets `interactionFired` only when a script IS found, register a stub script in the test fixture (look for `registerTestScript` or similar in the existing test file). If goscape sets `interactionFired = true` unconditionally on trigger-fire-attempt regardless of script existence, the test as-written is fine.

- [ ] **Step 10: Run B4 to verify it passes**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestFollowOpContactFire ./modules/world/...
```

Expected: PASS.

- [ ] **Step 11: Run full module test**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

**Expected: A non-zero number of existing tests will FAIL.** Per the spec's R1 risk: adding TS L1261-1263 auto-clear changes when goscape clears interactions globally; tests that pin `p.target != nil` after a single `processInteraction` cycle on a successful contact-fire WILL fail. **Do NOT fix them in T5** — that's T6's cascade audit. Capture the failing-test list (run with `-v` and grep `--- FAIL`) and embed it in the T5 commit body for T6 to triage.

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -v ./modules/world/... 2>&1 | grep -E "^--- FAIL|^FAIL\s" > /tmp/t5_failures.txt || true
cat /tmp/t5_failures.txt
```

If the failure list is empty (zero existing tests rely on the now-changed anchor lifetime), great — T6's cascade audit becomes a doc-and-dev-tag-retirement task. If non-empty, list each in the T5 commit body.

- [ ] **Step 12: Commit**

```bash
git add modules/world/interaction.go modules/world/interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-44 T5 — processInteraction TS-faithful reshape with followOp + auto-clear

Closes NAI-40-D-OPPLAYER3-FOLLOWOP-NOT-PORTED. Reshapes processInteraction
to mirror TS Player.processInteraction (Player.ts:1200-1264):

  - followOp predicate: isFollowOp() = targetOp == 3 && target is *Player.
    Covers both APPLAYER3 and OPPLAYER3 (TS uses ServerTriggerType enum
    siblings; goscape's targetOp is raw op slot 1..4).
  - Pre-step interact arm: walktrigger (skipped when followOp) + tryInteract.
  - Post-step interact arm: repath + walktrigger + waypoint-exhaustion
    clear (followOp-only) + post-step tryInteract (skipped when followOp).
  - Auto-clear (TS L1261-1263): interacted && !apRangeCalled → ClearInteraction.
    Closes the global gap where goscape interactions stayed anchored
    indefinitely after firing.

New tests:
  - TestFollowOpPredicate (B1): 3 subtests pinning the predicate.
  - TestFollowOpAnchoredChase (B2): out-of-range chase keeps target.
  - TestFollowOpWaypointExhaustion (B3): no-waypoints + followOp clears.
  - TestFollowOpContactFire (B4): contact-fire auto-clears via L1261-1263.

Opens NAI-44-D-PLAYER-WALKTRIGGER-NOOP, NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET,
NAI-44-D-CONTINUEWALK-UNUSED, NAI-44-D-CANACCESS-NO-STUN-CHECK at their
respective in-code sites.

EXISTING TEST FAILURES (deferred to T6 cascade audit per
enumerate_all_sites.md):

  <list from /tmp/t5_failures.txt; empty if zero failures>

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

If the failure list is empty, the body section reads `<no existing test failures>` and T6 is doc-only.

---

## Task 6: Cascade audit + deviation retirement + close commit

**Files:**
- Modify: `modules/world/interaction_test.go`, `modules/world/npc_test.go`, others — re-frame any tests broken by T5's auto-clear semantic change.
- Modify: `modules/world/handler_op_player.go:21-30` — retire `NAI-40-D-OPPLAYER3-FOLLOWOP-NOT-PORTED` doc-comment block.
- Modify: `modules/world/world_script_queue_test.go:257-266` — retire `NAI-37-D-WORLDQUEUE-CROSS-CONTEXT-DROP` test-comment references.
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` — append NAI-44 close section.

Closes the sub-spec. **Two phases:** (a) cascade-audit fix-up of any failing tests from T5; (b) deviation-tag retirement + memory bump + close commit.

- [ ] **Step 1: Enumerate cascade-audit sites**

Run the prescribed grep per `enumerate_all_sites.md`:

```
rg -n "p\.target\s*(==|!=)\s*nil" modules/world/*_test.go > /tmp/cascade_sites.txt
cat /tmp/cascade_sites.txt
```

Cross-reference with the T5 failure list at `/tmp/t5_failures.txt`. Each FAILing test should appear in the cascade-sites list. Each cascade-site that does NOT appear in failures was either (a) already failing pre-T5 (unrelated), or (b) testing a code path that doesn't exercise the auto-clear (out-of-range, level-mismatch, delayed). For each FAILing site:

- Is the test asserting "interaction stays anchored after fire"? **Re-frame** to assert `p.interactionFired == true` instead of `p.target != nil`.
- Is the test asserting "interaction clears after some non-fire path"? **Likely already correct** — T5 didn't change non-fire paths. Investigate.
- Is the test asserting AP-script-extends-apRange semantics? Check `apRangeCalled` is properly set; the auto-clear gate is `interacted && !apRangeCalled`, so AP-extension tests should auto-clear-EXEMPT correctly.

- [ ] **Step 2: Fix each failing test**

Apply per-test re-framing. Show one canonical example here for guidance; apply the same shape to each failure.

Example: a hypothetical test currently asserting `p.target != nil` after `processInteraction` with target in operable range would change from:

```go
// BEFORE (broken at HEAD post-T5)
p.processInteraction()
if p.target == nil {
	t.Error("target: got nil, want non-nil after fire")
}
if !p.interactionFired {
	t.Error("interactionFired: got false, want true")
}
```

…to:

```go
// AFTER (NAI-44 auto-clear semantics)
p.processInteraction()
if !p.interactionFired {
	t.Error("interactionFired: got false, want true")
}
// NAI-44 T5: auto-clear at TS L1261-1263 fires when interacted &&
// !apRangeCalled, so target is nil after a successful contact-fire.
// (Prior to NAI-44, goscape interactions stayed anchored indefinitely.)
if p.target != nil {
	t.Errorf("target: got %v, want nil (auto-clear after successful fire)", p.target)
}
```

Apply, run the affected test, verify PASS, move to next failure.

- [ ] **Step 3: Run full module test to verify zero remaining failures**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: PASS across all packages. If any test still fails, repeat Step 2 for that test.

- [ ] **Step 4: Run race detector + cross-package**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./...
```

Expected: PASS across all 30 packages.

- [ ] **Step 5: Retire deviation doc-comment in handler_op_player.go**

Modify `modules/world/handler_op_player.go`. Current L21-30:

```go
// DEVIATION NAI-40-D-OPCALLED-MISSING: TS sets player.opcalled = true
// at handler exit; goscape uses interactionFired (set by trigger fire)
// instead. Pre-existing S6a-era convention. Closure: NAI-40-SB1
// (cross-cutting opcalled-flag convergence).
//
// DEVIATION NAI-40-D-OPPLAYER3-FOLLOWOP-NOT-PORTED: TS Player.ts:1115
// special-cases targetOp == APPLAYER3 || OPPLAYER3 to keep the
// interaction anchored while chasing the target. Goscape fires-and-
// forgets. Tag-only; closure when player-script-lifecycle alignment
// sub-spec ports follow-op semantics.
```

Replace with (keep `OPCALLED-MISSING`, retire `OPPLAYER3-FOLLOWOP-NOT-PORTED`):

```go
// DEVIATION NAI-40-D-OPCALLED-MISSING: TS sets player.opcalled = true
// at handler exit; goscape uses interactionFired (set by trigger fire)
// instead. Pre-existing S6a-era convention. Closure: NAI-40-SB1
// (cross-cutting opcalled-flag convergence).
//
// NAI-40-D-OPPLAYER3-FOLLOWOP-NOT-PORTED closed by NAI-44 T5
// (processInteraction reshape with followOp + auto-clear).
```

- [ ] **Step 6: Re-grep ALL deviation tag references per `retire_deviation_grep_all_comments.md`**

```
rg "NAI-37-D-WORLDQUEUE-CROSS-CONTEXT-DROP|NAI-37-D-WORLDSUSPEND-CLEARS-ACTIVE-SCRIPT|NAI-40-D-OPPLAYER3-FOLLOWOP-NOT-PORTED" pkg/ modules/ docs/
```

Expected matches:
- `docs/superpowers/specs/2026-04-28-nai-44-executescript-binding-followop-design.md` — keep (history).
- `docs/superpowers/plans/2026-04-28-nai-44-executescript-binding-followop.md` — keep (history).
- `docs/superpowers/specs/2026-04-27-nai-37-walktrigger-hintnpc-worlddelay-design.md` — keep (history).
- `docs/superpowers/plans/2026-04-27-nai-37-walktrigger-hintnpc-worlddelay.md` — keep (history).
- `docs/superpowers/plans/2026-04-27-nai-40-opplayer-producer.md` — keep (history).
- `modules/world/world_script_queue_test.go` — re-frame any retained test-comment references (the existing test that cited the deviation; per T2 Step 5 should already be updated).

If any production-code (`pkg/`, `modules/`) site STILL references the retired tags, update it. Goal: zero production-code matches; doc/plan history preserved.

- [ ] **Step 7: Update memory `nai_followups.md`**

Append to `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (use Write or Edit; per `memory_write_sandbox_quirk.md`, do NOT use Bash echo/printf):

```markdown

## NAI-44 (CLOSED 2026-04-28)

### NAI-44 close — Full TS executeScript binding semantics + followOp

**Closes:**
- `NAI-37-D-WORLDQUEUE-CROSS-CONTEXT-DROP` — `resumeOrFinishWorld` now ports TS World.processWorld (World.ts:530-560) 3-arm dispatch (Suspended→Self, NpcSuspended→ActiveNpc, PauseButton/CountDialog→silent fall-through). Re-framed in close commit: TS World.processWorld does NOT route through Player/Npc.executeScript; the cross-context rebind for Pause/Count was never a TS behavior to mirror.
- `NAI-37-D-WORLDSUSPEND-CLEARS-ACTIVE-SCRIPT` — defensive `ClearActiveScript()` calls deleted at `script.go:` (player path) and `npc_script.go:` (npc path) WorldSuspended arms. Holding the pointer is safe: processActiveScripts gates resume on `Execution == Suspended` only (tick.go:213-214).
- `NAI-40-D-OPPLAYER3-FOLLOWOP-NOT-PORTED` — `processInteraction` reshaped to mirror TS Player.ts:1200-1264 with followOp branches at all 4 TS sites (pre-step walktrigger skip, post-step waypoint-exhaustion clear, post-step interact skip) and global auto-clear at TS L1261-1263.

**Implementation shape:** 6 TDD tasks (T1-T6), single sub-spec; ~330-500 LOC.
- T1 `<sha>`: drop defensive ClearActiveScript at WorldSuspended arms.
- T2 `<sha>`: TS-faithful 3-arm world-queue cross-context dispatch.
- T3 `<sha>`: foundation (hasWaypoints + processWalktrigger + stepsTaken reset).
- T4 `<sha>`: tryInteract helper extraction (pure refactor).
- T5 `<sha>`: processInteraction reshape with followOp + auto-clear.
- T6 `<sha>`: cascade-audit fix-up + deviation retirement + close.

**NAI-44 deviations (introduced + tracked):**

- `NAI-44-D-PLAYER-WALKTRIGGER-NOOP` — empty `(p *Player).processWalktrigger()` stub for TS-shape parity. Closure: bundles with NAI-37-D-WALKTRIGGER-NOREADER.
- `NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET` — goscape's `p_op*` opcodes do immediate `SetInteraction` swaps; TS Player.ts:1255-1258 queues `nextTarget` for next-tick. Closure: future `p_op*` opcode reshape sub-spec.
- `NAI-44-D-CONTINUEWALK-UNUSED` — `tryInteract`'s `continueWalk` parameter reserved for TS L1245 stepsTaken-aware retry timing; goscape's per-tick movement order makes it a no-op. Closure: dead-API-polish at next sub-spec close per dead_api_polish.md.
- `NAI-44-D-CANACCESS-NO-STUN-CHECK` — `!p.delayed` is the in-tree subset of TS canAccess() (which also tests stun/freeze; goscape has no stun system). Closure: stun system port (no current sub-spec planned).

**Net deviation tally:** 3 closed + 4 opened = +1 net.

**Pre-flight controller catches (saved an implementer cycle):**
- T1+T2 setup: pre-flight verified `lastStepX/Z`, `stepsTaken`, `followX/Z`, `targetX/Z` all already exist on `*Player` (player.go:79, 96-98). Spec's audit-conditional `NAI-44-D-NO-LAST-STEP-COORDS` collapsed; `nextTarget` field was confirmed absent (skipped per IMMEDIATE-POP-VS-NEXTTARGET tag).
- T1: tick ordering + processActiveScripts gate confirmed (`Execution == Suspended` only) so the no-clear deletion is verifiably safe.

**Items deferred (still open after NAI-44):**

Carryovers:
1. `pathing-entity-focus-and-step-tracking` sub-spec — closes NAI-34-D3, D4, D5-NPC, NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ. Conditional on consumer materializing.
2. `NAI-35-T3-D1` audit — conditional on smoke signal.
3. `Configs.SpotAnimType(id)` config-port (NAI-36-D2) — conditional on need.
4. AI-tick walktrigger consumption (NAI-37-D-WALKTRIGGER-NOREADER + NAI-44-D-PLAYER-WALKTRIGGER-NOOP) — bundle.
5. `NAI-40-SB1` OPCALLED — RECLASSIFIED blocked on World.ts:613-642 port.
6. `NAI-40-SB2` FINDHERO + BOTH_HEROPOINTS — needs HeroPoints + hash64 infra.
7. `NAI-40-SB4` Slot-reuse / target-logout — defensive-only.

New from NAI-44:
8. `NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET` closure — future `p_op*` opcode reshape sub-spec.
9. `NAI-44-D-CONTINUEWALK-UNUSED` closure — dead-API-polish at next sub-spec close if no consumer.
10. `NAI-44-D-CANACCESS-NO-STUN-CHECK` closure — stun system port (no current sub-spec planned).

**Memory entries seeded by NAI-44:** none (established patterns worked).

**Memory entries applied by NAI-44 (provenance):**
- `runescript_cadence.md` — full cadence with auto-mode collapse.
- `tracker_entry_framing_can_be_incomplete.md` — drove the WORLDQUEUE-CROSS-CONTEXT-DROP re-frame in T2 close commit.
- `controller_preflight.md` — pre-flight grep at plan-write time collapsed 4 audit-conditionals to confirmed-existing.
- `enumerate_all_sites.md` — drove T6 cascade-audit.
- `latent_bug_at_migration_boundary.md` — drove the clean-cutover-then-fix shape (T5 ships breakage; T6 fixes).
- `audit_full_method_against_ts.md` — drove the full TS L1200-1264 method audit at brainstorm (vs. just porting the followOp predicate).
- `dead_api_polish.md` — drove conditional-deviation closure plans (CONTINUEWALK-UNUSED).
- `retire_deviation_grep_all_comments.md` — drove T6 Step 6 enumeration.
- `close_commit_memory_trailer.md` — close commit carries `Closes memory:` trailer.
- `memory_write_sandbox_quirk.md` — used Write/Edit for memory dir (not Bash) per the documented quirk.
```

Replace `<sha>` placeholders with the actual commit hashes from T1-T6 once T6 commits.

- [ ] **Step 8: Update `MEMORY.md` index if any new entries were seeded**

NAI-44 seeded no new entries (per Step 7's note). Skip this step. If the implementer DID seed something during T1-T5, add the corresponding line to `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md`.

- [ ] **Step 9: Final verification**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -count=1 ./...
rg "NAI-37-D-WORLDQUEUE-CROSS-CONTEXT-DROP|NAI-37-D-WORLDSUSPEND-CLEARS-ACTIVE-SCRIPT|NAI-40-D-OPPLAYER3-FOLLOWOP-NOT-PORTED" pkg/ modules/
```

Expected:
- `go vet` clean.
- All tests PASS (race-free).
- The grep returns 0 hits in `pkg/` and `modules/` (history-only references in `docs/` are fine).

- [ ] **Step 10: Close commit**

```bash
git add modules/world/ docs/
# AND the memory file (must be added explicitly because it's outside the repo)
git status

git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(world,docs): NAI-44 closed — full TS executeScript binding semantics + followOp

Sub-spec close. Closes 3 deferred deviations:

  - NAI-37-D-WORLDQUEUE-CROSS-CONTEXT-DROP (T2): resumeOrFinishWorld
    ports TS World.processWorld L530-560 3-arm dispatch.
  - NAI-37-D-WORLDSUSPEND-CLEARS-ACTIVE-SCRIPT (T1): defensive
    ClearActiveScript calls deleted at player + npc WorldSuspended arms.
  - NAI-40-D-OPPLAYER3-FOLLOWOP-NOT-PORTED (T5): processInteraction
    reshape with followOp branches + auto-clear at TS L1261-1263.

Opens 4 tracked deviations (NAI-44-D-PLAYER-WALKTRIGGER-NOOP,
NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET, NAI-44-D-CONTINUEWALK-UNUSED,
NAI-44-D-CANACCESS-NO-STUN-CHECK).

Net deviation tally: 3 closed + 4 opened = +1 net.

T6 cascade-audit fix-up: <N> existing tests re-framed to assert
interactionFired-and-target-nil per the new auto-clear semantics.

Verification at HEAD:
  - go vet ./... clean.
  - go test -race -count=1 ./... PASS across all 30 packages.
  - rg <retired tags> pkg/ modules/ → 0 hits.

Closes memory: nai_followups.md (NAI-44 close section appended)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Replace `<N>` with the actual cascade-audit fix count from Step 2.

---

## Self-review (post-plan)

**1. Spec coverage:**
- Section A.1 (drop defensive ClearActiveScript) → T1. ✓
- Section A.2 (port World-queue 3-arm dispatch) → T2. ✓
- Section A.3 (re-frame in close commit) → T2 commit body + T6 close commit. ✓
- Section B.1 (Player struct fields) → pre-flight collapsed; only `stepsTaken` reset needed → T3. ✓
- Section B.2 (processWalktrigger stub) → T3. ✓
- Section B.3 (tryInteract helper) → T4. ✓
- Section B.4 (processInteraction reshape) → T5. ✓
- Section B.5 (retire deviation tags) → T6. ✓
- Section C.1 (cascade audit) → T6. ✓
- All test buckets (A1, A2, A3, B1, B2, B3, B4, B5, B6, B7) covered.

**2. Placeholder scan:**
- `<list from /tmp/t5_failures.txt; empty if zero failures>` in T5 commit — instruction to embed. ✓ (operational placeholder, fillable by implementer)
- `<sha>` in T7 memory append — operational placeholder. ✓
- `<N>` in T6 close commit — operational placeholder. ✓
- No semantic placeholders ("TBD", "implement later", "handle edge cases"). ✓

**3. Type consistency:**
- `script.ActivePlayer.StoreActiveScript(state)` and `script.ActiveNpc.StoreActiveScript(state)` — both verified to exist at HEAD. ✓
- `state.Self` (type `script.ActivePlayer`) and `state.ActiveNpc` (type `script.ActiveNpc`) — used consistently. ✓
- `p.targetOp == 3` predicate — consistent across T5 (Step 3) and T5 Step 1 (B1 test). ✓
- `isFollowOp(p)` helper — exported at T5 Step 3, called from `processInteraction` body, tested in T5 Step 1. ✓
- `tryInteract(continueWalk bool) bool` signature — T4 Step 1 + T5 Step 3 calls match. ✓
- `hasWaypoints() bool` — T3 Step 7 + T5 Step 3 calls match. ✓
- `processWalktrigger()` no-args — T3 Step 11 + T5 Step 3 calls match. ✓
