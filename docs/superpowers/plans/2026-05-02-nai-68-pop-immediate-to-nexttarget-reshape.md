# NAI-68 — `p_op*` immediate→nextTarget reshape Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Mirror TS Player.ts:1126-1163 + 1203 + 1255-1263 so that an OP/AP trigger script's `p_op_loc` / `p_op_npc` call survives auto-clear and applies on the next tick. Closes `NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET`. Opens `NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED`. Net deviation tally 13 → 13.

**Architecture:** Add `nextTarget entity` field on `*Player`; reset at `processInteraction` entry (TS L1203); pop at `processInteraction` tail (TS L1255-1263). Inline save/clear/exec/capture/restore at each of the 8 fire helpers (4 OP + 4 AP) — no extracted helpers. AP variants additionally save/restore waypoints with TS L1162's nextTarget-conditional clear. Drop the per-fire-helper Finished/Aborted ClearInteraction (subsumed by the tail's `else if interacted && !apRangeCalled`). Goscape's existing apRangeCalled across-tick re-fire (early-return-without-fired) preserved verbatim.

**Tech Stack:** Go 1.26+. TS source canonical path: `LostCityRS/Engine-TS`. Test harness: standard `testing` package + existing fixtures (`buildOpLocTriggerFixture`, `makeApLocTriggerFixture`, `buildNpcSayScript`, etc.).

**Spec:** `docs/superpowers/specs/2026-05-02-nai-68-pop-immediate-to-nexttarget-reshape-design.md`.

**Predecessor HEAD:** `3257bd2` (spec scope correction commit).

---

## File Structure

| File | Responsibility | T1 | T2 | T3 |
|---|---|---|---|---|
| `modules/world/player.go` | `Player` struct: add `nextTarget entity` field. | ✓ | | |
| `modules/world/interaction.go` | `processInteraction` entry reset + tail rewrite. | ✓ | | |
| `modules/world/interaction_test.go` | B1, B2 unit tests. | ✓ | | |
| `modules/world/interaction_trigger.go` | Inline OP+AP save/clear/restore at Loc/Npc/Obj fires; drop eager Finished/Aborted clears. | | ✓ | ✓ |
| `modules/world/player_interaction_trigger.go` | Same for Player×OP+AP fires. | | ✓ | ✓ |
| `modules/world/interaction_trigger_test.go` | B3 (Loc/Npc/Obj OP+AP), B5, B6 tests. | | ✓ | ✓ |
| `modules/world/player_interaction_trigger_test.go` | B3 (Player OP+AP) tests. | | ✓ | ✓ |

---

## Pre-flight Checks (controller, before each task dispatch)

Per `controller_preflight.md`, verify against HEAD before each task:

```bash
# T1 pre-flight
rg -n "nextTarget" modules/world/                                    # expect: zero hits
rg -n "if interacted && !p\.apRangeCalled" modules/world/interaction.go  # expect: line 245
grep -n "type Player struct" modules/world/player.go                 # expect: line 61

# T2 pre-flight
grep -n "func fireOpTrigger" modules/world/interaction_trigger.go modules/world/player_interaction_trigger.go
# expect (lines may shift after T1):
#   interaction_trigger.go: fireOpTriggerNpc ~line 53, fireOpTriggerLoc ~line 119,
#                           fireOpTriggerObj ~line 477
#   player_interaction_trigger.go: fireOpTriggerPlayer ~line 42

# T3 pre-flight
grep -n "func fireApTrigger" modules/world/interaction_trigger.go modules/world/player_interaction_trigger.go
# expect (lines may shift after T2):
#   interaction_trigger.go: fireApTriggerNpc ~line 298, fireApTriggerLoc ~line 359,
#                           fireApTriggerObj ~line 537
#   player_interaction_trigger.go: fireApTriggerPlayer ~line 79
rg -n "p\.apRangeCalled\s*=\s*false" modules/world/interaction_trigger.go modules/world/player_interaction_trigger.go
# expect: AP-Loc:401, AP-Obj:571 confirmed; verify AP-Npc + AP-Player presence (add if missing)
```

---

## Task 1: Framework — `nextTarget` field, entry reset, tail rewrite

**Closes:** B1 (nextTarget pop overrides auto-clear), B2 (entry reset).

**Files:**
- Modify: `modules/world/player.go:102-103` (add `nextTarget` field after `target`)
- Modify: `modules/world/interaction.go:184-200` (entry reset placement)
- Modify: `modules/world/interaction.go:239-247` (tail rewrite)
- Test: `modules/world/interaction_test.go` (extend with new test functions)

### Step 1.1: Write the failing test for B1 (nextTarget pop)

- [ ] Open `modules/world/interaction_test.go` and append (file already exists; locate end-of-file before adding):

```go
// TestProcessInteractionTailPopsNextTarget pins TS Player.ts:1255-1258.
// When p.nextTarget is non-nil at processInteraction tail, the auto-clear
// MUST be skipped and p.target MUST become p.nextTarget. Pre-fix this
// test fails because the tail's `if interacted && !p.apRangeCalled`
// unconditionally clears.
//
// NAI-68 B1.
func TestProcessInteractionTailPopsNextTarget(t *testing.T) {
	srv := newServerForTest(t)
	p := newClientedPlayer(t, srv, 3200, 3200, 0)

	// Two distinct stub targets at level 0 so the level-mismatch early-exit
	// in processInteraction does NOT fire — we want the full tail to run.
	locA := newStubLoc(3201, 3200, 0)
	npcB := newStubNpc(srv, 3201, 3201, 0)

	// Pre-state: pretend a fire just happened, the script set a nextTarget,
	// and we're now at the tail.
	p.target = locA
	p.nextTarget = npcB
	p.interacted = true
	p.apRangeCalled = false
	// Bypass pre-step / post-step arms entirely by parking the player
	// outside any operable/approach distance from locA — pre-step
	// tryInteract returns false (interacted local stays false), tail's
	// `if p.nextTarget != nil` branch still runs because nextTarget is
	// pre-set.
	// NOTE: this exercises the tail's nextTarget pop in isolation; the
	// test sets p.interacted directly so the auto-clear else-if would
	// fire if the pop weren't there.

	p.processInteraction()

	if p.target != npcB {
		t.Errorf("p.target after tail: got %v, want npcB (nextTarget pop)", p.target)
	}
	if p.nextTarget != npcB {
		// Tail does not reset nextTarget — only entry resets. After tail
		// the value was read into target; the field itself stays.
		t.Errorf("p.nextTarget after tail: got %v, want npcB (no field-reset at tail)", p.nextTarget)
	}
}

// TestProcessInteractionTailAutoClearsWithoutNextTarget pins the negative
// case — tail's else-if branch fires when nextTarget is nil and the
// pre-fix interacted+!apRangeCalled state is met.
//
// NAI-68 B1 dual-pin.
func TestProcessInteractionTailAutoClearsWithoutNextTarget(t *testing.T) {
	srv := newServerForTest(t)
	p := newClientedPlayer(t, srv, 3200, 3200, 0)

	locA := newStubLoc(3201, 3200, 0)

	p.target = locA
	p.nextTarget = nil
	p.interacted = true
	p.apRangeCalled = false

	p.processInteraction()

	if p.target != nil {
		t.Errorf("p.target after tail: got %v, want nil (else-if auto-clear)", p.target)
	}
}
```

> **Note for the implementer:** the helpers `newServerForTest`, `newClientedPlayer`, `newStubLoc`, `newStubNpc` may already exist under different names in the test files. Check `interaction_test.go`, `interaction_trigger_test.go`, and `player_interaction_trigger_test.go` for the canonical fixture names BEFORE writing — adopt the existing names. If a helper doesn't exist that creates a level-0 player with a non-nil `client.server`, write a minimal one and document why. The two `newStubLoc` / `newStubNpc` factories should return values that satisfy the `entity` interface (`Slot`, `Coords`, `IsValid`).

- [ ] **Step 1.1 commit:** none yet — test won't compile (`nextTarget` field doesn't exist). Move to Step 1.2.

### Step 1.2: Run the new tests; expect compile failure

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessInteractionTail
```

Expected: build failure with `p.nextTarget undefined (type *Player has no field or method nextTarget)`.

### Step 1.3: Add `nextTarget entity` field to `*Player`

- [ ] Edit `modules/world/player.go`:

Find:
```go
	// === interaction target ===
	target   entity
	targetOp int
```

Replace with:
```go
	// === interaction target ===
	target   entity
	// nextTarget queues a script-set interaction target for next-tick
	// application. Written by the OP/AP fire helpers (interaction_trigger.go,
	// player_interaction_trigger.go) capturing whatever a trigger script
	// stored via SetInteraction; popped at processInteraction tail
	// (interaction.go) per TS Player.ts:1255-1258. Nil between ticks.
	nextTarget entity
	targetOp   int
```

(The `targetOp` field declaration is reformatted to align with `target`/`nextTarget` per gofmt — gofmt should auto-handle the alignment after the edit; if not, manually align.)

### Step 1.4: Add entry reset to `processInteraction` (TS L1203)

- [ ] Edit `modules/world/interaction.go`:

Find:
```go
	// TS L1201-1202.
	p.followX = p.lastStepX
	p.followZ = p.lastStepZ
	// TS L1203 (this.nextTarget = null) — DEVIATION NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET:
	// goscape's p_op* opcodes do immediate SetInteraction swaps rather
	// than queueing a nextTarget for next-tick application. No nextTarget
	// field exists on *Player; the reshape below has no nextTarget block.
	// Closure: future p_op* opcode reshape sub-spec.

	followOp := isFollowOp(p)
```

Replace with:
```go
	// TS L1201-1202.
	p.followX = p.lastStepX
	p.followZ = p.lastStepZ
	// TS L1203.
	p.nextTarget = nil

	followOp := isFollowOp(p)
```

### Step 1.5: Rewrite `processInteraction` tail with TS L1255-1263 if/else-if

- [ ] Edit `modules/world/interaction.go`:

Find:
```go
	// Auto-clear (TS L1261-1263). NAI-44 closure of
	// NAI-40-D-OPPLAYER3-FOLLOWOP-NOT-PORTED's auto-clear gap.
	// Note: followOp paths can still reach this when tryInteract returned
	// true at the pre-step arm (contact range with target=*Player op=3).
	// TS does the same — followOp gates SKIP_post-step-interact, not
	// the auto-clear itself.
	if interacted && !p.apRangeCalled {
		p.ClearInteraction()
	}
```

Replace with:
```go
	// nextTarget pop + auto-clear (TS L1255-1263). NAI-68 closes
	// NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET: when an OP/AP trigger
	// script called p_op_* mid-trigger, the fire helpers captured the
	// script-set target into p.nextTarget; pop it here. Otherwise,
	// auto-clear the interaction (NAI-44 closure of
	// NAI-40-D-OPPLAYER3-FOLLOWOP-NOT-PORTED's auto-clear gap).
	// followOp paths can still reach the else-if when tryInteract
	// returned true at the pre-step arm (contact range with
	// target=*Player op=3); TS does the same — followOp gates SKIP
	// post-step-interact, not the auto-clear itself.
	if p.nextTarget != nil {
		p.target = p.nextTarget
	} else if interacted && !p.apRangeCalled {
		p.ClearInteraction()
	}
```

### Step 1.6: Add B2 test (entry reset)

- [ ] Append to `modules/world/interaction_test.go`:

```go
// TestProcessInteractionEntryResetsNextTarget pins TS Player.ts:1203.
// p.nextTarget MUST be reset to nil on every processInteraction call,
// even on the level-mismatch early-exit path (TS L1203 runs before
// validateTarget at TS L1207).
//
// NAI-68 B2.
func TestProcessInteractionEntryResetsNextTarget(t *testing.T) {
	srv := newServerForTest(t)
	p := newClientedPlayer(t, srv, 3200, 3200, 0)

	// Stale nextTarget from a hypothetical previous tick.
	npcStale := newStubNpc(srv, 3201, 3201, 0)
	locA := newStubLoc(3201, 3200, 0)

	p.target = locA
	p.nextTarget = npcStale

	p.processInteraction()

	if p.nextTarget != nil {
		// After tail: pop happens, but the field STAYS at the popped
		// value (we don't post-pop reset). However, the entry reset
		// runs FIRST, so the stale value is wiped before the pop reads
		// it. The pop reads nil → no swap → tail's else-if runs.
		// Final state: nextTarget=nil because the value was reset on
		// entry, never re-set during this tick.
		t.Errorf("p.nextTarget after processInteraction: got %v, want nil (entry reset per TS L1203)", p.nextTarget)
	}
}

// TestProcessInteractionEntryResetsNextTargetEvenOnLevelMismatch pins
// TS L1203's placement BEFORE the validateTarget level-check. A target
// at a different level triggers the early-exit at interaction.go:196-199,
// but the entry reset must already have run.
//
// NAI-68 B2 placement-pin.
func TestProcessInteractionEntryResetsNextTargetEvenOnLevelMismatch(t *testing.T) {
	srv := newServerForTest(t)
	p := newClientedPlayer(t, srv, 3200, 3200, 0)

	npcStale := newStubNpc(srv, 3201, 3201, 0)
	// Target on a DIFFERENT level — triggers level-mismatch early-exit.
	locOtherLevel := newStubLoc(3201, 3200, 1)

	p.target = locOtherLevel
	p.nextTarget = npcStale

	p.processInteraction()

	if p.nextTarget != nil {
		t.Errorf("p.nextTarget after level-mismatch exit: got %v, want nil (TS L1203 runs before L1207)", p.nextTarget)
	}
}
```

### Step 1.7: Run all interaction tests; expect pass

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessInteraction -v
```

Expected: all four new tests PASS. Existing `TestProcessInteraction*` tests (audit list) should also still pass — they don't use `nextTarget`, so the new field with default zero-value `nil` is invisible.

### Step 1.8: Run the full module test suite for regressions

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: PASS. If any existing test fails, audit and fix before commit (likely culprit: a test that pre-set `interacted=true` with a non-nil target and expected ClearInteraction to fire — verify the test's intent matches the new tail semantics).

### Step 1.9: Run cross-package tests for regressions

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS. Per `verify_implementer_claims.md`, never trust package-scoped green — full ./... is the reproducible verification.

### Step 1.10: Commit

```bash
git add modules/world/player.go modules/world/interaction.go modules/world/interaction_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-68 T1 — Player.nextTarget field + processInteraction entry reset and tail pop

Adds the nextTarget mechanism per TS Player.ts:1203 + 1255-1263 so an
OP/AP trigger script's p_op_* call can survive the auto-clear. Field
reset at processInteraction entry; popped at tail before auto-clear.
T2 wires the OP fire helpers to capture into nextTarget; T3 wires AP.

Tests: B1 (tail pop overrides auto-clear, dual-pin), B2 (entry reset
on normal + level-mismatch early-exit paths).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Step 1.11: T1 review checkpoint

Per `runescript_cadence.md` two-stage review at T1, dispatch a code-review subagent with:

```
Review the T1 commit's adherence to NAI-68 spec Section 4.1-4.3 and TS
Player.ts:1200-1263. Specifically verify:
  1. p.nextTarget = nil is placed AFTER followX/followZ writes and
     BEFORE the level-mismatch check (TS L1203 placement).
  2. The tail's `if/else if` mirrors TS L1255-1263 verbatim (no
     additional branches; nextTarget pop takes precedence over auto-clear).
  3. The new field's doc-comment describes the producer (T2/T3 fire
     helpers) and consumer (tail pop) without forward-referencing
     specific implementer details.
  4. B1, B2 tests pin the right contract (B1: pop overrides clear; B2:
     entry reset even on early-exit).
  5. No new deviation tags introduced.
  6. gofmt applied — Player struct field alignment is consistent.
Report findings under Critical / Important / Nice-to-have / Praise.
```

Block T2 dispatch on critical/important findings; address inline before T2 starts.

---

## Task 2: Wire 4 OP fires (inline save/clear/restore + TS L1131)

**Closes:** B3 OP variants, B5 (TS L1131 clearWaypoints add).

**Files:**
- Modify: `modules/world/interaction_trigger.go` (`fireOpTriggerNpc`, `fireOpTriggerLoc`, `fireOpTriggerObj`)
- Modify: `modules/world/player_interaction_trigger.go` (`fireOpTriggerPlayer`)
- Test: `modules/world/interaction_trigger_test.go` (B3 OP variants for Loc/Npc/Obj, B5)
- Test: `modules/world/player_interaction_trigger_test.go` (B3 OP variant for Player)

### Step 2.1: Write the failing B3 test for OP-Loc (script calls `p_op_npc` mid-trigger)

- [ ] Append to `modules/world/interaction_trigger_test.go`:

```go
// TestFireOpTriggerLocCapturesNextTargetFromScript pins TS Player.ts:1129-1134.
// An OP-Loc trigger script that calls p_op_npc mid-execution must have
// the new NPC target captured into p.nextTarget; p.target must be
// restored to the original loc post-script (the tail's pop applies the
// nextTarget on the next tick).
//
// NAI-68 B3 OP-Loc variant.
func TestFireOpTriggerLocCapturesNextTargetFromScript(t *testing.T) {
	fx := makeOpLocTriggerFixture(t)
	srv, p, loc, _ := fx.srv, fx.p, fx.loc, fx.npc
	npcB := newStubNpc(srv, p.x+1, p.z, p.level) // distinct from loc

	// Register an [oploc1, locType] script whose body queues p_op_npc(npcB, 2)
	// and returns. The fixture's buildPOpNpcScript helper compiles to:
	//   push npcB.nid; .npc_finduid; push 2; p_op_npc; ret
	// (Adopt the existing fixture builder; if no buildPOpNpcScript exists,
	// model it on buildOpPlayerHintPlScript.)
	srv.scriptProvider.Register(buildPOpNpcScript("[oploc1,fixturetype]", npcB.nid, 2))

	// Active script context: ActiveNpc set to npcB so npc_finduid resolves.
	// The fixture supplies this via fx.npc; but our target nextTarget
	// is npcB, not fx.npc — overwrite.
	fx.npc = npcB

	fireOpTriggerLoc(p, srv, loc)

	// Tail HAS NOT run (we're testing the fire helper in isolation).
	// Expected: nextTarget captured, target restored to the original loc.
	if p.nextTarget != npcB {
		t.Errorf("p.nextTarget: got %v, want npcB (script-set target captured)", p.nextTarget)
	}
	if p.target != loc {
		t.Errorf("p.target: got %v, want loc (restored after capture)", p.target)
	}
}

// TestFireOpTriggerLocClearsWaypoints pins TS Player.ts:1131. Every
// OP fire MUST clear waypoints before script execution, regardless
// of whether the script sets a nextTarget.
//
// NAI-68 B5 dual-pin: counterpart with no script-side p_op_* runs in
// TestFireOpTriggerLocClearsWaypointsNoNextTarget below.
func TestFireOpTriggerLocClearsWaypoints(t *testing.T) {
	fx := makeOpLocTriggerFixture(t)
	srv, p, loc := fx.srv, fx.p, fx.loc

	// Pre-state: active waypoint queue.
	p.waypointIndex = 3
	p.waypoints[3] = 0xDEADBEEF

	srv.scriptProvider.Register(buildNpcSayScript(script.TriggerOpLoc1, fx.locType, "any-msg-fixture"))

	fireOpTriggerLoc(p, srv, loc)

	if p.waypointIndex != -1 {
		t.Errorf("p.waypointIndex: got %d, want -1 (TS L1131)", p.waypointIndex)
	}
}

// TestFireOpTriggerLocClearsWaypointsNoNextTarget — counter-pin: same
// clear semantics even when no nextTarget is set.
func TestFireOpTriggerLocClearsWaypointsNoNextTarget(t *testing.T) {
	fx := makeOpLocTriggerFixture(t)
	srv, p, loc := fx.srv, fx.p, fx.loc

	p.waypointIndex = 3
	p.waypoints[3] = 0xDEADBEEF

	// No script registered — no-script path still goes through the OP
	// fire's pre-script clear? No — wait: no-script path is a
	// lifecycle-fail with early-return (interaction_trigger.go:151-159
	// "Nothing interesting happens."). That path does NOT clear
	// waypoints; only the script-execution path does. Adjust the test:
	// register a no-op script so we hit the script-execution path with
	// no nextTarget capture.
	srv.scriptProvider.Register(buildNoopScript(script.TriggerOpLoc1, fx.locType))

	fireOpTriggerLoc(p, srv, loc)

	if p.waypointIndex != -1 {
		t.Errorf("p.waypointIndex: got %d, want -1 (TS L1131 — fires regardless of nextTarget)", p.waypointIndex)
	}
	if p.nextTarget != nil {
		t.Errorf("p.nextTarget: got %v, want nil (no-op script didn't set)", p.nextTarget)
	}
}
```

> **Implementer note:** if `buildPOpNpcScript` and `buildNoopScript` helpers don't exist in the test file, write minimal versions adjacent to existing builders like `buildNpcSayScript`. They must produce a `*script.ScriptFile` with the appropriate trigger metadata. Re-use the bytecode-emit pattern of the existing fixtures verbatim. If `npc_finduid` opcode is unavailable in tests, use `s.PlayerLookup.LookupPlayerByUID` substitute paths or replace with a direct `s.ActiveNpc` reference if the test fixture pre-sets it.

### Step 2.2: Run the new tests; expect compile/test failure

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestFireOpTriggerLoc -v
```

Expected: tests run but fail because the OP fire helper still does immediate-target-overwrite (`p.target = npcB` after `SetInteraction` in the script) and then ClearInteraction wipes back to nil. `p.nextTarget` remains nil; `p.target` may be nil or original depending on path.

### Step 2.3: Inline save/clear/restore at `fireOpTriggerLoc`

- [ ] Edit `modules/world/interaction_trigger.go`:

Find:
```go
	state := script.Init(sf, p, true, nil, nil)
	state.ActiveLoc = loc
	state.Pointers |= script.PtrActiveLoc
	state.Provider = srv.scriptProvider
	state.World = srv.worldVars
	state.Configs = srv.configsView
	state.Inv = srv.invLookup
	state.Npcs = srv.npcLookup
	state.LineValidator = srv.scriptLineValidator()

	srv.resumeOrFinish(state, p)

	if state.Execution == script.Finished || state.Execution == script.Aborted {
		p.ClearInteraction()
	}
	p.interactionFired = true
}
```

Replace with:
```go
	state := script.Init(sf, p, true, nil, nil)
	state.ActiveLoc = loc
	state.Pointers |= script.PtrActiveLoc
	state.Provider = srv.scriptProvider
	state.World = srv.worldVars
	state.Configs = srv.configsView
	state.Inv = srv.invLookup
	state.Npcs = srv.npcLookup
	state.LineValidator = srv.scriptLineValidator()

	// TS Player.ts:1129-1134 OP save/clear/exec/capture/restore.
	// NAI-68 closes NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET.
	savedTarget := p.target
	p.target = nil
	p.waypointIndex = -1 // TS L1131 — this.clearWaypoints()

	srv.resumeOrFinish(state, p)

	p.nextTarget = p.target
	p.target = savedTarget

	// Finished/Aborted ClearInteraction dropped — subsumed by
	// processInteraction tail's else-if at interaction.go (TS L1261-1263).
	p.interactionFired = true
}
```

### Step 2.4: Run B3 OP-Loc + B5 tests; expect pass

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestFireOpTriggerLoc -v
```

Expected: PASS. If a test fails because the script can't dispatch (e.g., `npc_finduid` unavailable), simplify the test fixture to set `state.ActiveNpc = npcB` directly via a fixture helper before the fire, bypassing the script's lookup. The contract under test is the save/clear/capture/restore — not the full opcode dispatch.

### Step 2.5: Apply the same inline shape to `fireOpTriggerNpc`

- [ ] Edit `modules/world/interaction_trigger.go` at `fireOpTriggerNpc` (around line 95-101):

Find:
```go
	state := script.Init(sf, p, true, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= script.PtrActiveNpc
	state.Provider = srv.scriptProvider
	state.World = srv.worldVars
	state.Configs = srv.configsView
	state.Inv = srv.invLookup
	state.Npcs = srv.npcLookup
	state.LineValidator = srv.scriptLineValidator()

	srv.resumeOrFinish(state, p)

	if state.Execution == script.Finished || state.Execution == script.Aborted {
		p.ClearInteraction()
	}
	p.interactionFired = true
}
```

Replace with:
```go
	state := script.Init(sf, p, true, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= script.PtrActiveNpc
	state.Provider = srv.scriptProvider
	state.World = srv.worldVars
	state.Configs = srv.configsView
	state.Inv = srv.invLookup
	state.Npcs = srv.npcLookup
	state.LineValidator = srv.scriptLineValidator()

	// TS Player.ts:1129-1134 OP save/clear/exec/capture/restore. NAI-68.
	savedTarget := p.target
	p.target = nil
	p.waypointIndex = -1 // TS L1131

	srv.resumeOrFinish(state, p)

	p.nextTarget = p.target
	p.target = savedTarget

	p.interactionFired = true
}
```

### Step 2.6: Apply the same inline shape to `fireOpTriggerObj`

- [ ] Edit `modules/world/interaction_trigger.go` at `fireOpTriggerObj` (around lines 510-528). Use the same find/replace pattern as Step 2.5 — locate the trailing block:

```go
	srv.resumeOrFinish(state, p)

	if state.Execution == script.Finished || state.Execution == script.Aborted {
		p.ClearInteraction()
	}
	p.interactionFired = true
}
```

inside `fireOpTriggerObj` and replace with the same inline shape from Step 2.5 (with the per-helper TS-L comment header `TS Player.ts:1129-1134 OP save/clear/exec/capture/restore. NAI-68.`).

> **Implementer note:** per `plan_doc_replaceall_timeline.md`, do NOT use `replace_all`. Each fire helper has slightly different state.Active* assignments above the swap site; verify the find-string is unique to that helper before each Edit (include 2-3 lines of context above to disambiguate).

### Step 2.7: Apply the same inline shape to `fireOpTriggerPlayer`

- [ ] Edit `modules/world/player_interaction_trigger.go` at `fireOpTriggerPlayer` (around line 42-72). Same find/replace pattern.

### Step 2.8: Add per-entity B3 OP variant tests

- [ ] Append B3 tests for OP-Npc (in `interaction_trigger_test.go`), OP-Obj (`interaction_trigger_test.go`), OP-Player (`player_interaction_trigger_test.go`), each modeled on `TestFireOpTriggerLocCapturesNextTargetFromScript`.

The Loc test from Step 2.1 is the template; per-entity tests vary only in:
- Fixture builder (`makeOpNpcTriggerFixture`, `makeOpObjTriggerFixture`, `makePlayerInteractionFixture` — adopt existing names; check before writing).
- Active* state.Active{Npc,Obj,Player} setup matches the fire helper.
- The script-side p_op target (still `p_op_npc(npcB, 2)` works for all four — switching from any source to an Npc target).

### Step 2.9: Run all OP fire tests; expect pass

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestFireOpTrigger -v
```

Expected: all OP-fire tests pass — the 3 new B3 OP tests + B5 dual-pin (Loc).

### Step 2.10: Run full module + cross-package tests

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS. Watch for: existing OP-fire tests that asserted `p.target == nil` post-fire (under the dropped Finished/Aborted clear semantic) — those should still pass because the *processInteraction tail* now does the clearing, but the test is calling the fire helper in isolation. Test fix: swap the assertion from `p.target == nil` to `p.target == originalTarget` since the helper now restores rather than clears.

If an existing test breaks due to this assertion-flip, fix the test to assert `p.target == originalTarget && p.nextTarget == nil` (the new contract: helper preserves; tail clears).

### Step 2.11: Commit

```bash
git add modules/world/interaction_trigger.go modules/world/player_interaction_trigger.go \
        modules/world/interaction_trigger_test.go modules/world/player_interaction_trigger_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-68 T2 — wire 4 OP fire helpers to nextTarget mechanism

Inlines TS Player.ts:1129-1134's save/clear/exec/capture/restore at
fireOpTriggerNpc, fireOpTriggerLoc, fireOpTriggerObj, fireOpTriggerPlayer.
Adds TS L1131 clearWaypoints(). Drops the per-helper Finished/Aborted
ClearInteraction (subsumed by processInteraction tail's else-if at
interaction.go).

Tests: B3 OP variants (4 entity types), B5 dual-pin
(waypoints clear regardless of nextTarget set).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Step 2.12: T2 review checkpoint (standard task review)

Standard task review (single-stage) verifying TS-line-mapping and absence of drift across the 4 OP fire helpers. Block T3 on critical/important findings.

---

## Task 3: Wire 4 AP fires (inline save/clear/restore + waypoints + TS L1162)

**Closes:** B3 AP variants, B6 (waypoint conditional clear).

**Files:**
- Modify: `modules/world/interaction_trigger.go` (`fireApTriggerNpc`, `fireApTriggerLoc`, `fireApTriggerObj`)
- Modify: `modules/world/player_interaction_trigger.go` (`fireApTriggerPlayer`)
- Test: `modules/world/interaction_trigger_test.go` (B3 AP variants Loc/Npc/Obj, B6)
- Test: `modules/world/player_interaction_trigger_test.go` (B3 AP variant Player)

### Step 3.1: Verify `apRangeCalled = false` pre-reset on AP-Npc and AP-Player

Run:
```bash
grep -n "p\.apRangeCalled\s*=\s*false" modules/world/interaction_trigger.go modules/world/player_interaction_trigger.go
```

Expected pre-T3: AP-Loc:401, AP-Obj:571 confirmed; AP-Npc and AP-Player MUST NOT yet have the pre-reset (or they may already — verify and skip Step 3.2 if they do).

### Step 3.2: Add `apRangeCalled = false` pre-reset to AP-Npc and AP-Player if missing

For each AP fire helper missing the reset:

- [ ] In `fireApTriggerNpc` (interaction_trigger.go ~line 298), find:

```go
	state := script.Init(sf, p, true, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= script.PtrActiveNpc
```

(this is the line BEFORE state init.)

Insert `apRangeCalled = false` BEFORE state init:

```go
	// Reset apRangeCalled BEFORE exec (TS Player.ts:1141). Each AP fire
	// is a fresh evaluation — script must actively call p_aprange to
	// persist the interaction.
	p.apRangeCalled = false

	state := script.Init(sf, p, true, nil, nil)
	state.ActiveNpc = npc
	state.Pointers |= script.PtrActiveNpc
```

- [ ] Repeat for `fireApTriggerPlayer` (player_interaction_trigger.go).

### Step 3.3: Write the failing B3 AP-Loc test (script calls `p_op_npc` mid-AP-trigger)

- [ ] Append to `modules/world/interaction_trigger_test.go`:

```go
// TestFireApTriggerLocCapturesNextTargetFromScript pins TS Player.ts:1145-1162.
// An AP-Loc trigger script that calls p_op_npc must:
//   - capture new target into p.nextTarget,
//   - restore p.target to the original loc,
//   - clear waypoints (TS L1162 — nextTarget != nil branch).
//
// NAI-68 B3 AP-Loc variant + B6 nextTarget-set sub-pin.
func TestFireApTriggerLocCapturesNextTargetFromScript(t *testing.T) {
	fx := makeApLocTriggerFixture(t)
	srv, p, loc := fx.srv, fx.p, fx.loc
	npcB := newStubNpc(srv, p.x+1, p.z, p.level)

	// Pre-state: active waypoint queue (must be cleared per TS L1162).
	p.waypointIndex = 3
	p.waypoints[3] = 0xDEADBEEF

	srv.scriptProvider.Register(buildPOpNpcScript("[aploc1,fixturetype]", npcB.nid, 2))

	fireApTriggerLoc(p, srv, loc)

	if p.nextTarget != npcB {
		t.Errorf("p.nextTarget: got %v, want npcB", p.nextTarget)
	}
	if p.target != loc {
		t.Errorf("p.target: got %v, want loc (restored)", p.target)
	}
	if p.waypointIndex != -1 {
		t.Errorf("p.waypointIndex: got %d, want -1 (TS L1162 nextTarget != nil clears)", p.waypointIndex)
	}
}

// TestFireApTriggerLocRestoresWaypointsWhenNoNextTarget pins TS L1146 inverse.
// When the AP script does NOT set a nextTarget AND does not call p_aprange,
// waypoints must be RESTORED to pre-fire state (the L1146 clear is reverted).
//
// NAI-68 B6 nextTarget-nil sub-pin.
func TestFireApTriggerLocRestoresWaypointsWhenNoNextTarget(t *testing.T) {
	fx := makeApLocTriggerFixture(t)
	srv, p, loc := fx.srv, fx.p, fx.loc

	// Pre-state: active waypoint queue.
	p.waypointIndex = 3
	p.waypoints[3] = 0xDEADBEEF
	p.waypoints[2] = 0xCAFEBABE

	// No-op AP script: no p_op_*, no p_aprange.
	srv.scriptProvider.Register(buildNoopScript(script.TriggerApLoc1, fx.locType))

	fireApTriggerLoc(p, srv, loc)

	if p.waypointIndex != 3 {
		t.Errorf("p.waypointIndex: got %d, want 3 (no-script-target preserves waypoints)", p.waypointIndex)
	}
	if p.waypoints[3] != 0xDEADBEEF {
		t.Errorf("p.waypoints[3]: got 0x%X, want 0xDEADBEEF", p.waypoints[3])
	}
	if p.waypoints[2] != 0xCAFEBABE {
		t.Errorf("p.waypoints[2]: got 0x%X, want 0xCAFEBABE", p.waypoints[2])
	}
	if p.nextTarget != nil {
		t.Errorf("p.nextTarget: got %v, want nil", p.nextTarget)
	}
}
```

### Step 3.4: Run new AP-Loc tests; expect failure

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestFireApTriggerLoc -v
```

Expected: tests run but fail — the AP fire helper does not yet capture nextTarget or save/restore waypoints.

### Step 3.5: Inline AP save/clear/restore at `fireApTriggerLoc`

- [ ] Edit `modules/world/interaction_trigger.go` at `fireApTriggerLoc`. Find the block from `srv.resumeOrFinish(state, p)` through `p.interactionFired = true`:

```go
	srv.resumeOrFinish(state, p)

	if state.Execution == script.Finished || state.Execution == script.Aborted {
		if p.apRangeCalled {
			// Script requested a new approach range. Persist interaction
			// for next-tick re-evaluation at updated apRange.
			p.repathed = false
			// interactionFired stays false → processInteraction re-enters
			// next tick; APLOC re-fires if still in range.
			return
		}
		// apRangeCalled=false → script didn't extend range; TS line 1261
		// clears the interaction.
		p.ClearInteraction()
	}
	// Reached by: (a) Finished/Aborted + !apRangeCalled (after
	// ClearInteraction above), or (b) Suspended/P_DELAY/P_PAUSEBUTTON/
	// P_COUNTDIALOG (anchor intact, resume flow re-enters on resume tick).
	p.interactionFired = true
}
```

Replace with:
```go
	// TS Player.ts:1145-1162 AP save/clear/exec/capture/restore +
	// nextTarget-conditional waypoint clear. NAI-68.
	savedTarget := p.target
	savedWP := p.waypoints
	savedIdx := p.waypointIndex
	p.target = nil
	p.waypointIndex = -1

	srv.resumeOrFinish(state, p)

	p.nextTarget = p.target
	p.target = savedTarget
	if p.nextTarget != nil {
		// TS L1162: clear destination so next-tick interaction starts fresh.
		p.waypointIndex = -1
	} else {
		// No script-set target — restore waypoints (TS L1146 inverse).
		p.waypoints = savedWP
		p.waypointIndex = savedIdx
	}

	// Existing apRangeCalled across-tick re-fire branch — UNCHANGED.
	// DEVIATION NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED: TS L1166-1170 does
	// same-tick post-step retry; goscape uses early-return-without-fired
	// for next-tick re-fire. Equivalent for player experience.
	if state.Execution == script.Finished || state.Execution == script.Aborted {
		if p.apRangeCalled {
			p.repathed = false
			return // interactionFired stays false → re-fire next tick.
		}
		// Finished/Aborted + !apRangeCalled: ClearInteraction dropped —
		// subsumed by processInteraction tail's else-if (TS L1261-1263).
	}
	// Reached by: (a) Finished/Aborted + !apRangeCalled (no-op here, tail
	// handles), or (b) Suspended (anchor intact, resume on next tick).
	p.interactionFired = true
}
```

### Step 3.6: Run B3+B6 AP-Loc tests; expect pass

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestFireApTriggerLoc -v
```

Expected: PASS for both new tests + existing AP-Loc tests (which assert apRangeCalled across-tick re-fire — preserved verbatim).

### Step 3.7: Apply the same AP inline shape to `fireApTriggerNpc`

- [ ] Edit `modules/world/interaction_trigger.go` at `fireApTriggerNpc` (around lines 340-345). Find:

```go
	srv.resumeOrFinish(state, p)

	if state.Execution == script.Finished || state.Execution == script.Aborted {
		p.ClearInteraction()
	}
	p.interactionFired = true
}
```

(AP-Npc has the SIMPLER post-script structure — per existing comment "NO apRangeCalled persistence contract" at lines 293-297. NPCs don't extend approach range.)

Replace with:
```go
	// TS Player.ts:1145-1162 AP save/clear/exec/capture/restore. NAI-68.
	// AP-Npc has no apRangeCalled persistence (NPC attackrange is fixed
	// per-type per pre-existing doc-comment at fireApTriggerNpc:293-297).
	savedTarget := p.target
	savedWP := p.waypoints
	savedIdx := p.waypointIndex
	p.target = nil
	p.waypointIndex = -1

	srv.resumeOrFinish(state, p)

	p.nextTarget = p.target
	p.target = savedTarget
	if p.nextTarget != nil {
		p.waypointIndex = -1
	} else {
		p.waypoints = savedWP
		p.waypointIndex = savedIdx
	}

	// Finished/Aborted ClearInteraction dropped — subsumed by
	// processInteraction tail's else-if (TS L1261-1263).
	p.interactionFired = true
}
```

### Step 3.8: Apply the AP inline shape to `fireApTriggerObj`

- [ ] Edit `modules/world/interaction_trigger.go` at `fireApTriggerObj`. The body around lines 580-592 has the apRangeCalled across-tick branch (mirror of AP-Loc). Use the SAME replacement as Step 3.5 — Obj has the same persistence contract as Loc.

### Step 3.9: Apply the AP inline shape to `fireApTriggerPlayer`

- [ ] Edit `modules/world/player_interaction_trigger.go` at `fireApTriggerPlayer` (around line 79). Player's body is closest to AP-Loc — apply the SAME replacement as Step 3.5.

> **Implementer note:** AP-Loc and AP-Obj both have the apRangeCalled across-tick branch (preserve it). AP-Npc lacks it. AP-Player — verify by reading the function body before editing; if it has the apRangeCalled branch, use the AP-Loc replacement; if not (matches AP-Npc shape), use the AP-Npc replacement.

### Step 3.10: Add per-entity B3 AP variant tests

- [ ] Append B3 tests for AP-Npc, AP-Obj (in `interaction_trigger_test.go`), AP-Player (`player_interaction_trigger_test.go`), each modeled on `TestFireApTriggerLocCapturesNextTargetFromScript`.

### Step 3.11: Run all AP fire tests; expect pass

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestFireApTrigger -v
```

Expected: all AP-fire tests pass — the 4 new B3 AP tests + B6 dual-pin (Loc) + existing apRangeCalled persistence tests.

### Step 3.12: Run full module + cross-package tests

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS. Watch for: existing AP-fire tests that asserted `p.target == nil` post-fire — same fix as T2 step 2.10 (assertion flip to `p.target == originalTarget && p.nextTarget == nil`).

### Step 3.13: Commit

```bash
git add modules/world/interaction_trigger.go modules/world/player_interaction_trigger.go \
        modules/world/interaction_trigger_test.go modules/world/player_interaction_trigger_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-68 T3 — wire 4 AP fire helpers to nextTarget mechanism + waypoint save/restore

Inlines TS Player.ts:1145-1162's save/clear/exec/capture/restore +
nextTarget-conditional waypoint clear at fireApTriggerNpc,
fireApTriggerLoc, fireApTriggerObj, fireApTriggerPlayer. Adds TS L1141
apRangeCalled pre-reset to AP-Npc and AP-Player (already present at
AP-Loc and AP-Obj). Drops per-helper Finished/Aborted+!apRangeCalled
ClearInteraction (subsumed by processInteraction tail's else-if).
Preserves existing AP-Loc/AP-Obj apRangeCalled across-tick re-fire
branch (tagged NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED — TS L1166-1170
same-tick retry not ported in this sub-spec; equivalent across-tick
mechanism).

Tests: B3 AP variants (4 entity types), B6 dual-pin (waypoints clear
when nextTarget set; restore when not).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Step 3.14: T3 review checkpoint (whole-impl review)

Per `runescript_cadence.md` two-stage review at T3, dispatch a code-review subagent (Sonnet per `superpowers_code_reviewer_model.md`) with:

```
Review the T3 commit's adherence to NAI-68 spec Section 4.1-4.6 and TS
Player.ts:1140-1170 + 1255-1263. This is a whole-impl review (T1 + T2 +
T3 commits taken together). Specifically verify:

  1. TS-line-mapping at each of the 8 fire helpers' inline blocks matches
     the spec verbatim (savedTarget, savedWP, savedIdx, target=nil,
     waypointIndex=-1, resumeOrFinish, nextTarget=p.target, target=savedTarget,
     and AP-side conditional waypoint clear/restore).
  2. AP-Loc and AP-Obj preserve the apRangeCalled across-tick re-fire
     early-return; AP-Npc and AP-Player don't have one (consistent with
     pre-existing semantics). New deviation tag
     NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED has a doc-comment at AP-Loc
     and AP-Obj.
  3. apRangeCalled = false pre-reset present at all 4 AP sites (TS L1141).
  4. Per-helper Finished/Aborted ClearInteraction DROPPED at all 8 sites;
     processInteraction tail's else-if is the sole auto-clear.
  5. B3 (4 entity × 2 arms = 8 tests), B5 (dual-pin), B6 (dual-pin) all
     pin the right contracts.
  6. No drift across the 8 inline shapes — same variable names, same TS-L
     comments, same line ordering.
  7. Existing tests still pin the apRangeCalled across-tick re-fire
     contract (no regressions on
     interaction_trigger_test.go:439-510 and similar).
  8. `go test ./...` passes.

Report findings under Critical / Important / Nice-to-have / Praise.
```

Block T4 close on critical/important findings; address inline.

---

## Task 4: Close

**Closes:** sub-spec close.

**Files:**
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (append NAI-68 close section)
- (No code changes.)

### Step 4.1: Audit close-state

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS.

```bash
rg -n "NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET" --glob '*.go' modules/ pkg/
```

Expected: zero hits — the deviation tag is fully retired from production code (the tag was at `interaction.go:187-191` before T1 and was removed in Step 1.4).

```bash
rg -n "NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED" --glob '*.go' modules/ pkg/
```

Expected: at least 2 hits — at AP-Loc and AP-Obj inline doc-comments per Step 3.5/3.8 instructions.

### Step 4.2: Append NAI-68 close section to `nai_followups.md`

- [ ] Append to `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`:

```markdown
---

## NAI-68 — CLOSED 2026-05-XX

**Scope:** `p_op*` immediate→nextTarget reshape. Adds `nextTarget entity`
field on `*Player`; resets at processInteraction entry (TS L1203); pops
at processInteraction tail (TS L1255-1263). Inlines save/clear/exec/
capture/restore at all 8 fire helpers (4 OP + 4 AP); AP variants add
waypoint save/restore with TS L1162 nextTarget-conditional clear. Drops
per-fire-helper Finished/Aborted ClearInteraction in favor of the tail's
else-if. Preserves goscape's existing AP-Loc/AP-Obj apRangeCalled
across-tick re-fire (early-return-without-interactionFired).

**Cadence:** Full sub-spec, 3 implementation tasks + close. Two-stage
review at T1 (framework + tail) and T3 (AP whole-impl).

**Spec:** `docs/superpowers/specs/2026-05-02-nai-68-pop-immediate-to-nexttarget-reshape-design.md`.
**Plan:** `docs/superpowers/plans/2026-05-02-nai-68-pop-immediate-to-nexttarget-reshape.md`.

**Close commit:** (this commit). T1: `<sha>`. T2: `<sha>`. T3: `<sha>`.

**Follow-ups closed:**
- `NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET` — script-side `p_op_loc` /
  `p_op_npc` mid-trigger now applies on next tick via nextTarget pop.

**Deviations opened:** `NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED` — TS
Player.ts:1166-1170 same-tick post-step retry NOT ported; goscape's
existing across-tick re-fire mechanism (interactionFired stays false on
apRangeCalled early-return) is the equivalent. Closure: future "tryInteract
structural alignment" sub-spec that reworks the interactionFired guard
to support same-tick retry without infinite-loop risk.

**Deviations closed:** `NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET`.

**Net deviation tally:** -1 closure, +1 open = 13 → 13 (unchanged).

**Wire-behaviour delta at HEAD:**
- An OP/AP trigger script calling `p_op_loc` / `p_op_npc` mid-execution
  now correctly transitions to the new target on next tick. Pre-fix the
  call was a silent no-op (Finished/Aborted ClearInteraction wiped the
  script-set target).
- Suspended scripts (P_DELAY/P_PAUSEBUTTON/P_COUNTDIALOG) inside an
  OP/AP trigger now have their player-side `p.target` cleared at the
  processInteraction tail (was preserved by the dropped per-helper
  Finished/Aborted gate). Suspended scripts hold their own
  ActiveLoc/ActiveNpc/Self pointers internally — resumption is
  unaffected. This matches TS exactly.

**Lessons confirmed:**
- `runescript_cadence.md` — full 3-task cadence + two-stage review at T1
  and T3.
- `true_to_ts_gate.md` — every behavioral change cited against TS
  Player.ts source lines.
- `controller_preflight.md` — pre-flight grep gates surfaced a scope
  conflict between TS L1166-1170 and goscape's existing across-tick
  re-fire mechanism BEFORE plan-write; spec corrected at commit
  `3257bd2`.
- `tracker_entry_framing_can_be_incomplete.md` — reframed mid-plan when
  the original spec's TS L1166-1170 inclusion conflicted with goscape's
  state machine.
- `dead_api_polish.md` — N/A this sub-spec; nextTarget gets a producer
  in the same sub-spec that adds it.
- `enumerate_all_sites.md` — pre-flight enumeration of all 8 fire-helper
  sites + `tryFireApTrigger` callers; re-grep post-T2 and post-T3.
- `plan_grep_helper_patterns.md` — verified no prior helper covers the
  save/clear/restore pattern; chose inline-per-site over extracted
  helper.
- `plan_doc_replaceall_timeline.md` — per-site Edits across the 8 fire
  helpers, never `replace_all`.
- `plan_var_name_collision.md` — `savedTarget`, `savedWP`, `savedIdx`
  locals checked against existing `state`, `sf`, `category`, `trigger`
  body locals at each site.
- `verify_implementer_claims.md` — fresh `go test ./...` pass after each
  task.
- `audit_full_method_against_ts.md` — T3 whole-impl review audited
  processInteraction + both tryInteract arms vs TS L1113-1180+1255-1263.
- `ts_asymmetry_dual_pin.md` — B3 dual-pinned script-set vs no-script-set;
  B5 dual-pinned waypoints clear regardless of nextTarget; B6 dual-pinned
  L1162 (clear when nextTarget set) vs L1146 inverse (restore when not).
- `defensive_gate_doc_comment_label.md` — NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED
  doc-comments at AP-Loc and AP-Obj early-return sites.
- `close_commit_memory_trailer.md` — close commit carries
  `Closes memory: NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET`.

**Memory entries reinforced (no edits needed):** all of the above.

**Carry-forwards (still open after NAI-68):**
- `NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED` (new) — TS L1166-1170 same-tick
  retry, blocked on interactionFired-guard rework.
- `NAI-67-D-PLAYER-UNFOCUS-DEFERRED` — Player respawn/death sub-spec.
- `NAI-34-D4-NPC` + `NAI-34-D5-NPC` — permanent dead-API skip.
- `NAI-35-T3-D1` op[1] operability gate audit.
- All other deferred carry-forwards from NAI-65 / NAI-66 / NAI-67
  (NAI-40-SB1/SB2/SB4, NAI-44-D-CANACCESS-NO-STUN-CHECK,
  NAI-59-D-MODALTUTORIAL-NO-PRODUCER) unchanged.
```

> **Implementer note:** replace `2026-05-XX` with today's date when committing T4. Replace `<sha>` placeholders with the actual T1/T2/T3 commit SHAs from `git log --oneline`.

### Step 4.3: Update memory MEMORY.md if any new entry was added

Per `superpowers_clear_between_spec_and_impl.md` defaults, no new memory entries are expected from this sub-spec — all lessons reinforce existing entries. Skip MEMORY.md edit unless a genuinely new and non-derivable lesson surfaced during implementation.

### Step 4.4: Commit close

```bash
git add /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-68 — p_op* immediate→nextTarget reshape

Closes NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET. Opens
NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED (TS L1166-1170 same-tick retry
not ported; goscape's across-tick re-fire is the equivalent).
Net deviation tally: 13 → 13 (1 closure, 1 new open).

Spec: docs/superpowers/specs/2026-05-02-nai-68-pop-immediate-to-nexttarget-reshape-design.md
Plan: docs/superpowers/plans/2026-05-02-nai-68-pop-immediate-to-nexttarget-reshape.md

Closes memory: NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Step 4.5: Final verification

Run:
```bash
git log --oneline -5
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: 4 new commits at HEAD (T1, T2, T3, T4), tests all green.

---

## Self-Review

**Spec coverage:**
- Section 4.1 (data shape) → Task 1 Step 1.3.
- Section 4.2 (entry reset) → Task 1 Step 1.4.
- Section 4.3 (tail rewrite) → Task 1 Step 1.5.
- Section 4.4 (inline OP shape) → Task 2 Steps 2.3, 2.5, 2.6, 2.7.
- Section 4.4 (inline AP shape) → Task 3 Steps 3.5, 3.7, 3.8, 3.9.
- Section 4.5 (per-fire-helper changes, 8 sites) → Tasks 2 + 3.
- Section 4.6 (lifecycle clears unchanged) → no task; intentionally not modified.
- Section 5 (B1) → Task 1 Step 1.1.
- Section 5 (B2) → Task 1 Step 1.6.
- Section 5 (B3 OP) → Task 2 Steps 2.1, 2.8.
- Section 5 (B3 AP) → Task 3 Steps 3.3, 3.10.
- Section 5 (B5) → Task 2 Step 2.1.
- Section 5 (B6) → Task 3 Step 3.3.
- Section 6 risks → addressed in T1/T2/T3 commit bodies and review prompts.
- Section 7 cadence → matches T1/T2/T3/T4 task split.

**Placeholder scan:** none — all code blocks complete, all expected
outputs concrete, all commit messages drafted.

**Type consistency:** `nextTarget entity` in Step 1.3 matches usage in
Steps 1.4, 1.5, all of T2, all of T3. `savedTarget` / `savedWP` /
`savedIdx` consistent across all 8 fire-helper inline blocks.
