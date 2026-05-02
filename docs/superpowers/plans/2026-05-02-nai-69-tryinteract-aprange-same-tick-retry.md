# NAI-69 — `tryInteract` Same-Tick AP Retry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port TS Player.ts:1163-1170 same-tick AP retry by adding a `nextTarget==nil && apRangeCalled` guard in `tryInteract`'s AP branch and dropping the across-tick `interactionFired=false` scaffold from `fireApTriggerLoc` / `fireApTriggerObj`. Closes `NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED`.

**Architecture:** `tryInteract` becomes the sole owner of the same-tick retry signal. Fire helpers always set `interactionFired=true` at fire end. `tryInteract`'s AP branch checks the post-fire state and resets `interactionFired=false` colocated with `return false` when (and only when) `nextTarget==nil && apRangeCalled`. AP-Player gains real same-tick retry; AP-Npc gains structural-mechanism activation but stays a behavioral no-op (`effectiveApRange` reads `npc.typ.AttackRange`, not `p.apRange`).

**Tech Stack:** Go 1.26+. TS source canonical path: `LostCityRS/Engine-TS`. Spec: `docs/superpowers/specs/2026-05-02-nai-69-tryinteract-aprange-same-tick-retry-design.md`.

---

## Pre-flight Verification

Confirm at HEAD `dca2ff3` (NAI-69 spec commit, parent `95738e4` NAI-68 close):

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```
Expected: PASS.

```bash
rg -c "NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED" pkg/ modules/
```
Expected: 2 (`modules/world/interaction_trigger.go` — the two AP fire helpers).

```bash
rg -c "p\.repathed = false" modules/world/interaction_trigger.go
```
Expected: 2 (Loc + Obj fire helpers — both deleted in T2).

```bash
rg -n "func .*tryInteract" modules/world/interaction.go
```
Expected: 1 hit at line 310, `func (p *Player) tryInteract(allowOpScenery bool) bool`.

```bash
rg -n "TestTryFireApTriggerLocScriptCallsPApRange" modules/world/interaction_trigger_test.go
```
Expected: 1 hit at line 504. This test is reframed in T2.

```bash
rg -n "TestFireApTriggerObjScriptCallsPApRange" modules/world/
```
Expected: 0 hits (no AP-Obj parallel test exists at HEAD; T2 adds doc note only).

---

## File Map

| File | Role | Change scope |
|---|---|---|
| `modules/world/interaction.go` | `tryInteract`, `processInteraction`, `SetInteraction`, `ClearInteraction` | T1: 3-line addition in `tryInteract` AP branch. T5: 1 doc-comment rephrase at line 237. |
| `modules/world/interaction_trigger.go` | `fireApTriggerLoc`, `fireApTriggerObj`, `fireApTriggerNpc`, dispatch helpers | T2: ~14 LOC removed × 2 helpers + doc-comment header refresh × 3 helpers. Tag retirement at 2 sites. |
| `modules/world/player_interaction_trigger.go` | `fireApTriggerPlayer` | T5: doc-comment header refresh only. No production code change. |
| `modules/world/interaction_trigger_test.go` | Existing AP fire helper tests | T2: reframe `TestTryFireApTriggerLocScriptCallsPApRange` (3 assertion changes + doc-comment rephrase). |
| `modules/world/interaction_trigger_nai69_test.go` | NEW — same-tick retry tests for AP-Loc and AP-Obj | T1, T2: create. |
| `modules/world/player_interaction_trigger_test.go` | AP-Player tests | T3: append `TestFireApTriggerPlayer_SameTickRetry_*`. |
| `modules/world/npc_interaction_test.go` | NPC tests | T4: append `TestFireApTriggerNpc_ApRangeCalled_StructuralParityNoOp`. |

---

## Task 1: `tryInteract` AP-branch same-tick retry logic

**Files:**
- Modify: `modules/world/interaction.go:310-332` (`tryInteract` AP branch)
- Create: `modules/world/interaction_trigger_nai69_test.go` (new file for NAI-69 tests)
- Test: `modules/world/interaction_trigger_nai69_test.go`

- [ ] **Step 1: Create new test file with failing test**

Create `modules/world/interaction_trigger_nai69_test.go`:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/script"
)

// --- NAI-69 T1: tryInteract AP-branch same-tick retry pin ---

// TestTryInteract_ApRangeCalled_ReturnsFalseAndResetsFired pins the new
// TS L1163-1167 contract: when the AP script set apRangeCalled=true and
// nextTarget is nil, tryInteract resets interactionFired=false and
// returns false so processInteraction's walk-arm runs and the post-step
// tryInteract can re-fire AP.
//
// NAI-69 closes NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED.
func TestTryInteract_ApRangeCalled_ReturnsFalseAndResetsFired(t *testing.T) {
	s, p, loc, _ := makeApTriggerFixture(t)

	// Register an APLOC1 script that calls p_aprange(2).
	sf := scriptFileWithApRangeCall(t, script.TriggerApLoc1, loc.Type(), 2)
	s.scriptProvider.Register(sf)

	// Pre-state: in 10-range (apRange default), distance 5 (fixture
	// invariant from makeApTriggerFixture).
	result := p.tryInteract(false)

	if result {
		t.Error("tryInteract: got true, want false (TS L1167 — apRangeCalled triggers same-tick retry)")
	}
	if p.interactionFired {
		t.Error("interactionFired: got true, want false (reset by tryInteract for post-step re-fire)")
	}
	if !p.apRangeCalled {
		t.Error("apRangeCalled: got false, want true (script called p_aprange)")
	}
	if p.target != loc {
		t.Errorf("target: got %v, want loc (preserved across AP fire)", p.target)
	}
	if p.nextTarget != nil {
		t.Errorf("nextTarget: got %v, want nil (script did not call p_op_*)", p.nextTarget)
	}
}

// TestTryInteract_NoApRange_StillReturnsTrue pins that the new guard
// only triggers when apRangeCalled. A no-op AP script (no p_aprange)
// keeps the pre-NAI-69 contract: returns true, interactionFired stays
// true.
func TestTryInteract_NoApRange_StillReturnsTrue(t *testing.T) {
	s, p, loc, _ := makeApTriggerFixture(t)

	// Register a no-op APLOC1 script.
	sf := newNoopScriptFile(t, script.TriggerApLoc1, loc.Type(), -1)
	s.scriptProvider.Register(sf)

	result := p.tryInteract(false)

	if !result {
		t.Error("tryInteract: got false, want true (no apRangeCalled — original contract)")
	}
	if !p.interactionFired {
		t.Error("interactionFired: got false, want true (no retry signal)")
	}
	if p.apRangeCalled {
		t.Error("apRangeCalled: got true, want false (script did not call p_aprange)")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestTryInteract_ApRangeCalled_ReturnsFalseAndResetsFired -count=1 -v
```

Expected: FAIL.
- `tryInteract: got true, want false` — the current AP branch returns true unconditionally after fire.
- `interactionFired: got true, want false` — the current fire helper's early-return (line 473-477) leaves it false BUT we're going to change that in T2; for now under HEAD code it's actually still false because of the early-return. Either way the first assertion fails first.

The second test (`TestTryInteract_NoApRange_StillReturnsTrue`) should PASS at HEAD — it's a regression baseline.

- [ ] **Step 3: Apply the tryInteract change**

Edit `modules/world/interaction.go` — locate the AP branch in `tryInteract` (around line 324):

```go
	if inApproachDistance(p.x, p.z, tx, tz, effectiveApRange(p)) {
		p.interacted = true
		if !p.interactionFired {
			tryFireApTrigger(p)
		}
		return true
	}
	return false
}
```

Replace the `return true` with the new TS L1163-1167 guard:

```go
	if inApproachDistance(p.x, p.z, tx, tz, effectiveApRange(p)) {
		p.interacted = true
		if !p.interactionFired {
			tryFireApTrigger(p)
		}
		// TS L1163-1167: same-tick AP retry. When the AP script called
		// p_aprange (sets apRangeCalled=true) and did NOT call p_op_*
		// (nextTarget nil), reset the per-tick re-fire gate and return
		// false so processInteraction's walk-arm runs and post-step
		// tryInteract can re-fire AP with the new range. nextTarget
		// priority mirrors TS L1158-1161 (nextTarget pop wins). Closes
		// NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED.
		if p.nextTarget == nil && p.apRangeCalled {
			p.interactionFired = false
			return false
		}
		return true
	}
	return false
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestTryInteract_ -count=1 -v
```

Expected: both NAI-69 tests PASS.

- [ ] **Step 5: Run the full test suite to verify no regression**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected: PASS. Note: the existing `TestTryFireApTriggerLocScriptCallsPApRange` at `interaction_trigger_test.go:504` STILL passes at this point because T1 doesn't touch the fire helper — `interactionFired=false` post-fire is still the across-tick-scaffold behavior. T2 will reframe that test.

- [ ] **Step 6: Commit**

```bash
git add modules/world/interaction.go modules/world/interaction_trigger_nai69_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-69 T1 — tryInteract AP-branch same-tick retry guard

Add TS L1163-1167 guard in tryInteract's AP branch: when AP script
called p_aprange (sets apRangeCalled=true) AND did not call p_op_*
(nextTarget nil), reset interactionFired=false and return false so
processInteraction's walk-arm runs and post-step tryInteract can
re-fire AP with the new range.

Pre-existing across-tick re-fire scaffold in fireApTriggerLoc /
fireApTriggerObj is now redundant — to be removed in T2. Existing
TestTryFireApTriggerLocScriptCallsPApRange still passes at this point
because the fire helper isn't touched yet.

NAI-69 closes NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED.

Spec: docs/superpowers/specs/2026-05-02-nai-69-tryinteract-aprange-same-tick-retry-design.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Drop across-tick early-return in `fireApTriggerLoc` and `fireApTriggerObj`

**Files:**
- Modify: `modules/world/interaction_trigger.go:468-484` (`fireApTriggerLoc` post-fire block)
- Modify: `modules/world/interaction_trigger.go:660-676` (`fireApTriggerObj` post-fire block)
- Modify: `modules/world/interaction_trigger.go:290-310` (`fireApTriggerNpc` doc-comment header)
- Modify: `modules/world/interaction_trigger.go:383-393` (`fireApTriggerLoc` doc-comment header)
- Modify: `modules/world/interaction_trigger.go` near line 523 (`fireApTriggerObj` doc-comment header)
- Modify: `modules/world/interaction_trigger_test.go:500-530` (`TestTryFireApTriggerLocScriptCallsPApRange` reframe)
- Test: extend `modules/world/interaction_trigger_nai69_test.go`

- [ ] **Step 1: Write a failing test pinning the new fire-helper contract**

Append to `modules/world/interaction_trigger_nai69_test.go`:

```go
// --- NAI-69 T2: fire-helper uniform interactionFired=true contract ---

// TestFireApTriggerLoc_ApRangeCalled_SetsInteractionFiredTrue pins the
// post-NAI-69 contract: fireApTriggerLoc always sets
// interactionFired=true at fire end, regardless of apRangeCalled state.
// The pre-NAI-69 across-tick re-fire scaffold (early-return on
// Finished/Aborted+apRangeCalled leaving interactionFired=false) is
// dropped — same-tick retry is now signaled via apRangeCalled and
// handled by tryInteract (see T1).
func TestFireApTriggerLoc_ApRangeCalled_SetsInteractionFiredTrue(t *testing.T) {
	s, p, loc, _ := makeApTriggerFixture(t)

	sf := scriptFileWithApRangeCall(t, script.TriggerApLoc1, loc.Type(), 2)
	s.scriptProvider.Register(sf)

	tryFireApTrigger(p)

	if !p.interactionFired {
		t.Error("interactionFired: got false, want true (NAI-69: fire helper uniform exit)")
	}
	if !p.apRangeCalled {
		t.Error("apRangeCalled: got false, want true (script called p_aprange)")
	}
	if p.apRange != 2 {
		t.Errorf("apRange: got %d, want 2 (script set new range)", p.apRange)
	}
	if p.target != loc {
		t.Errorf("target: got %v, want loc (restored after fire)", p.target)
	}
}

// TestFireApTriggerObj_ApRangeCalled_SetsInteractionFiredTrue — AP-Obj
// parity. fireApTriggerObj's post-NAI-69 contract is identical to
// fireApTriggerLoc.
func TestFireApTriggerObj_ApRangeCalled_SetsInteractionFiredTrue(t *testing.T) {
	s, p, obj, _ := makeApObjTriggerFixture(t)

	// Register an APOBJ1 script that calls p_aprange(2).
	sf := scriptFileWithApRangeCall(t, script.TriggerApObj1, obj.Type, 2)
	s.scriptProvider.Register(sf)

	fireApTriggerObj(p, s, obj)

	if !p.interactionFired {
		t.Error("interactionFired: got false, want true (NAI-69: fire helper uniform exit)")
	}
	if !p.apRangeCalled {
		t.Error("apRangeCalled: got false, want true (script called p_aprange)")
	}
	if p.apRange != 2 {
		t.Errorf("apRange: got %d, want 2 (script set new range)", p.apRange)
	}
	if p.target != obj {
		t.Errorf("target: got %v, want obj (restored after fire)", p.target)
	}
}
```

- [ ] **Step 2: Run new tests to verify they fail**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestFireApTriggerLoc_ApRangeCalled_SetsInteractionFiredTrue|TestFireApTriggerObj_ApRangeCalled_SetsInteractionFiredTrue" -count=1 -v
```

Expected: both FAIL with `interactionFired: got false, want true` — current fire helpers leave it false on the apRangeCalled early-return.

- [ ] **Step 3: Drop the early-return in `fireApTriggerLoc`**

In `modules/world/interaction_trigger.go`, locate the AP-Loc fire helper post-fire block (around line 468-484):

```go
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

Replace with the new uniform-exit shape:

```go
	// TS L1163-1167 same-tick AP retry: when state.Execution is
	// Finished/Aborted AND apRangeCalled is true, tryInteract sees the
	// flag, restores interactionFired=false, and returns false so
	// processInteraction's walk-arm runs and post-step tryInteract
	// re-fires AP with the new range. Suspended scripts (P_DELAY /
	// P_PAUSEBUTTON / P_COUNTDIALOG) leave apRangeCalled false and
	// keep the anchor across ticks via the suspended ScriptState. NAI-69
	// closes NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED.
	p.interactionFired = true
}
```

- [ ] **Step 4: Drop the early-return in `fireApTriggerObj`**

In `modules/world/interaction_trigger.go`, locate the AP-Obj fire helper post-fire block (around line 660-676) — identical shape to `fireApTriggerLoc`:

```go
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

Replace with the same uniform-exit shape:

```go
	// TS L1163-1167 same-tick AP retry: when state.Execution is
	// Finished/Aborted AND apRangeCalled is true, tryInteract sees the
	// flag, restores interactionFired=false, and returns false so
	// processInteraction's walk-arm runs and post-step tryInteract
	// re-fires AP with the new range. Suspended scripts leave
	// apRangeCalled false and keep the anchor across ticks via the
	// suspended ScriptState. NAI-69 closes
	// NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED.
	p.interactionFired = true
}
```

- [ ] **Step 5: Refresh `fireApTriggerLoc` doc-comment header**

In `modules/world/interaction_trigger.go`, locate the doc-comment block above `fireApTriggerLoc` (around line 383-393):

```go
// fireApTriggerLoc fires the [aploc<op>,<locType>] trigger with the
// persistence contract: apRangeCalled=true keeps the interaction
// anchored across ticks; apRangeCalled=false clears it after a
// terminal Execution. Matches TS Player.ts:1139-1170 + :1261.
//
// Lifecycle gate: locStillValid (same helper from S6j) — catches
// in-place Info mutation and zone removal.
//
// Script lookup: TriggerApLoc1 + (op-1). No APLOC→OPLOC fallthrough
// at approach distance — OPLOC fires only when the player reaches
// contact on a later processInteraction tick.
```

Replace with:

```go
// fireApTriggerLoc fires the [aploc<op>,<locType>] trigger. Matches
// TS Player.ts:1139-1170. Always sets interactionFired=true at exit;
// the same-tick retry signal is apRangeCalled (set by p_aprange via
// the ActivePlayer.SetApRange interface). tryInteract owns the
// retry-vs-pop decision (see interaction.go AP branch).
//
// Lifecycle gate: locStillValid (same helper from S6j) — catches
// in-place Info mutation and zone removal.
//
// Script lookup: TriggerApLoc1 + (op-1). No APLOC→OPLOC fallthrough
// at approach distance — OPLOC fires only when the player reaches
// contact on a later processInteraction tick.
```

- [ ] **Step 6: Refresh `fireApTriggerNpc` doc-comment header**

In `modules/world/interaction_trigger.go`, locate `fireApTriggerNpc`'s doc-comment header (around line 290-310). The current header includes the "NO apRangeCalled persistence contract" subsection (numbered item 3):

```go
//  3. NO apRangeCalled persistence contract. Per TS
//     (Npc.ts:~1064-1080): NPC AP scripts complete and clear
//     interaction unconditionally. The p_aprange persistence is
//     Player-side only; NPC attackrange is fixed per-type so
//     "extend the range" has no meaning. Simpler post-fire logic.
```

Replace with the NAI-69 reframe:

```go
//  3. apRangeCalled mechanism is structurally active per the uniform
//     TS Player.ts:1139-1170 AP block, but behaviorally a no-op for
//     NPC targets. effectiveApRange (interaction.go:393) reads
//     npc.typ.AttackRange (fixed per-type), not p.apRange — so a
//     script calling p_aprange against an NPC target sets
//     p.apRangeCalled=true but doesn't change the in-range check on
//     post-step retry. NAI-69 preserves this preexisting goscape
//     divergence. Closure: future "AP-Npc effectiveApRange parity"
//     audit if upstream TS NPC AP behavior changes.
```

- [ ] **Step 7: Refresh `fireApTriggerObj` doc-comment header**

In `modules/world/interaction_trigger.go`, locate `fireApTriggerObj`'s doc-comment header (around line 523-528):

```go
// fireApTriggerObj fires the [apobj<op>,<objType>] approach-trigger for the
// player's anchored Obj target. Mirrors fireApTriggerLoc with three
// substitutions:
//  1. Lifecycle gate: objStillValid.
//  2. ScriptState: ActiveObj + PtrActiveObj.
//  3. No-script path: apRange=-1 sentinel (OP trigger takes over on contact).
```

This header is correct as-is; no change needed beyond the post-fire block already updated in Step 4. Verify with:

```bash
rg -n "fireApTriggerObj fires" modules/world/interaction_trigger.go
```

Confirm one hit and the doc-comment text matches the above.

- [ ] **Step 8: Reframe `TestTryFireApTriggerLocScriptCallsPApRange`**

In `modules/world/interaction_trigger_test.go`, locate the test at line 500-530.

Replace the doc-comment header (lines 500-503):

```go
// TestTryFireApTriggerLocScriptCallsPApRange verifies an APLOC script
// that calls p_aprange sets apRangeCalled=true, which causes the
// interaction to PERSIST past the tick (no ClearInteraction). repathed
// is reset to force a fresh path on the next tick.
```

With:

```go
// TestTryFireApTriggerLocScriptCallsPApRange verifies an APLOC script
// that calls p_aprange completes the fire cleanly with apRangeCalled=true
// and interactionFired=true (post-NAI-69 uniform-exit contract). The
// same-tick retry decision happens in tryInteract, not the fire helper
// (see TestTryInteract_ApRangeCalled_ReturnsFalseAndResetsFired).
```

In the test body, delete the `p.repathed = true` setup line (line 511) — `repathed` is no longer touched by the fire helper, so the pre-test setup is irrelevant:

```go
	p.repathed = true // verify it gets reset to false post-fire
```

Delete this line.

Update the post-fire assertions (lines 524-529). Replace:

```go
	if p.repathed {
		t.Error("repathed: want false (reset post-p_aprange for fresh path)")
	}
	if p.interactionFired {
		t.Error("interactionFired: want false (allow re-fire next tick)")
	}
```

With:

```go
	if !p.interactionFired {
		t.Error("interactionFired: want true (NAI-69: fire helper uniform exit)")
	}
```

(The `repathed` assertion is dropped entirely — `repathed` is no longer fire-helper concern post-NAI-69.)

- [ ] **Step 9: Verify the new tests pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestFireApTriggerLoc_ApRangeCalled_SetsInteractionFiredTrue|TestFireApTriggerObj_ApRangeCalled_SetsInteractionFiredTrue|TestTryFireApTriggerLocScriptCallsPApRange" -count=1 -v
```

Expected: all three PASS.

- [ ] **Step 10: Run the full test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 11: Verify the deviation tag is gone from production code**

```bash
rg "NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED" modules/world/interaction_trigger.go
```

Expected: 0 hits (the inline `DEVIATION NAI-68-D-…` tag was inside the early-return block deleted in Step 3 and Step 4).

```bash
rg "p\.repathed = false" modules/world/interaction_trigger.go
```

Expected: 0 hits.

- [ ] **Step 12: Commit**

```bash
git add modules/world/interaction_trigger.go modules/world/interaction_trigger_nai69_test.go modules/world/interaction_trigger_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-69 T2 — drop across-tick scaffold in AP-Loc/AP-Obj fire helpers

Fire helpers now always set interactionFired=true at exit. The same-tick
retry signal moves entirely to apRangeCalled, owned by tryInteract's
AP branch (NAI-69 T1). Drops the early-return on Finished/Aborted +
apRangeCalled and the redundant p.repathed=false in fireApTriggerLoc
and fireApTriggerObj.

Doc-comment refresh at fireApTriggerLoc + fireApTriggerNpc; AP-Obj
header was already accurate. Reframes the existing
TestTryFireApTriggerLocScriptCallsPApRange to pin the new uniform-exit
contract (interactionFired=true post-fire). Adds AP-Loc and AP-Obj
parity tests pinning interactionFired=true + apRangeCalled=true +
apRange=2 + target restored.

Inline DEVIATION NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED tag retired at
both fire helpers.

NAI-69 closes NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: AP-Player same-tick retry pin (test-only)

**Files:**
- Modify: `modules/world/player_interaction_trigger.go` (`fireApTriggerPlayer` doc-comment header — informational refresh only, no production code change)
- Test: `modules/world/player_interaction_trigger_test.go` (append)

- [ ] **Step 1: Locate AP-Player fixture**

`newPlayerTriggerFixture(t)` at `modules/world/player_interaction_trigger_test.go` returns `(s *Server, clicker *Player, target *Player, conn1)`. Used for AP-Player tests already (e.g. `TestFireApTriggerPlayerRestoresTargetAndWaypoints` at line 257).

- [ ] **Step 2: Write the failing test**

Append to `modules/world/player_interaction_trigger_test.go`:

```go
// --- NAI-69 T3: AP-Player same-tick retry pin ---

// TestFireApTriggerPlayer_ApRangeCalled_SetsInteractionFiredTrue pins
// the post-NAI-69 contract for AP-Player: fire helper sets
// interactionFired=true uniformly. AP-Player gains real same-tick
// retry behavior because effectiveApRange reads p.apRange for Player
// targets (interaction.go:399).
func TestFireApTriggerPlayer_ApRangeCalled_SetsInteractionFiredTrue(t *testing.T) {
	s, clicker, target, _ := newPlayerTriggerFixture(t)

	// Register an APPLAYER1 script that calls p_aprange(2).
	s.scriptProvider.Register(&script.ScriptFile{
		Name:      "[applayer1,_]_aprange",
		LookupKey: script.LookupKeyForGlobal(script.TriggerApPlayer1),
		Opcodes: []script.Opcode{
			script.OpPushConstantInt,
			script.OpPApRange,
			script.OpReturn,
		},
		IntOperands:      []int32{2, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	})

	s.players[target.slot] = target
	s.players[clicker.slot] = clicker

	fireApTriggerPlayer(clicker, s, target)

	if !clicker.interactionFired {
		t.Error("clicker.interactionFired: got false, want true (NAI-69: fire helper uniform exit)")
	}
	if !clicker.apRangeCalled {
		t.Error("clicker.apRangeCalled: got false, want true (script called p_aprange)")
	}
	if clicker.apRange != 2 {
		t.Errorf("clicker.apRange: got %d, want 2 (script set new range)", clicker.apRange)
	}
	if clicker.target != target {
		t.Errorf("clicker.target: got %v, want target (restored after fire)", clicker.target)
	}
}

// TestTryInteract_ApPlayer_ApRangeCalled_ReturnsFalseAndResetsFired —
// AP-Player end-to-end pin via tryInteract (the same-tick retry
// mechanism activates for Player targets identically to Loc/Obj).
// Verifies effectiveApRange reads p.apRange (mutable), not a fixed
// per-type field.
func TestTryInteract_ApPlayer_ApRangeCalled_ReturnsFalseAndResetsFired(t *testing.T) {
	s, clicker, target, _ := newPlayerTriggerFixture(t)

	// Position clicker 5 tiles from target.
	clicker.x, clicker.z = target.x-5, target.z

	clicker.SetInteraction(InteractionEngine, target, 1, -1)

	// Register the p_aprange(2) script.
	s.scriptProvider.Register(&script.ScriptFile{
		Name:      "[applayer1,_]_aprange",
		LookupKey: script.LookupKeyForGlobal(script.TriggerApPlayer1),
		Opcodes: []script.Opcode{
			script.OpPushConstantInt,
			script.OpPApRange,
			script.OpReturn,
		},
		IntOperands:      []int32{2, 0, 0},
		StringOperands:   []string{"", "", ""},
		InstructionCount: 3,
	})

	s.players[target.slot] = target
	s.players[clicker.slot] = clicker

	result := clicker.tryInteract(false)

	if result {
		t.Error("tryInteract: got true, want false (apRangeCalled triggers same-tick retry)")
	}
	if clicker.interactionFired {
		t.Error("interactionFired: got true, want false (reset for post-step re-fire)")
	}
	if !clicker.apRangeCalled {
		t.Error("apRangeCalled: got false, want true")
	}
}
```

- [ ] **Step 3: Run the new tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run "TestFireApTriggerPlayer_ApRangeCalled_SetsInteractionFiredTrue|TestTryInteract_ApPlayer_ApRangeCalled_ReturnsFalseAndResetsFired" -count=1 -v
```

Expected: both PASS (T1+T2 changes already cover the AP-Player path because the fire helper already sets `interactionFired=true` uniformly and `tryInteract`'s guard is target-type-agnostic).

- [ ] **Step 4: Refresh AP-Player doc-comment header**

In `modules/world/player_interaction_trigger.go`, locate the doc-comment above `fireApTriggerPlayer` (around line 84-87):

```go
// fireApTriggerPlayer fires the [applayer<op>,_] trigger at approach
// distance. On no-script-found: sets p.apRange = -1 to skip re-lookup
// next tick (matches fireApTriggerLoc behaviour at S6r). Self2 binding
// is the same as fireOpTriggerPlayer.
```

Replace with:

```go
// fireApTriggerPlayer fires the [applayer<op>,_] trigger at approach
// distance. On no-script-found: sets p.apRange = -1 to skip re-lookup
// next tick (matches fireApTriggerLoc behaviour at S6r). Self2 binding
// is the same as fireOpTriggerPlayer.
//
// Same-tick retry: AP-Player gains real TS L1163-1167 same-tick retry
// with NAI-69 because effectiveApRange (interaction.go:399) reads
// p.apRange for Player targets. Always sets interactionFired=true at
// exit; tryInteract owns the retry-vs-pop decision via the
// nextTarget==nil && apRangeCalled guard.
```

In the body, also drop the now-misleading internal comment at line 111-113:

```go
	// TS Player.ts:1145-1162 AP save/clear/exec/capture/restore. NAI-68.
	// AP-Player has no apRangeCalled persistence (uses runScript which
	// hides state.Execution; same NPC-class semantic).
```

Replace with:

```go
	// TS Player.ts:1145-1162 AP save/clear/exec/capture/restore. NAI-68
	// framework; NAI-69 activates same-tick retry by routing the
	// post-fire apRangeCalled signal through tryInteract.
```

- [ ] **Step 5: Run the full test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add modules/world/player_interaction_trigger.go modules/world/player_interaction_trigger_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): NAI-69 T3 — AP-Player same-tick retry pin + doc refresh

Pins that AP-Player gains real same-tick retry behavior with NAI-69
because effectiveApRange reads p.apRange for Player targets (mutable
via p_aprange). No production code change to fireApTriggerPlayer —
the helper already sets interactionFired=true uniformly; tryInteract's
NAI-69 T1 guard is target-type-agnostic.

Doc-comment refresh at fireApTriggerPlayer header + internal comment
cites NAI-69 same-tick retry mechanism.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: AP-Npc structural parity pin (test-only)

**Files:**
- Test: `modules/world/interaction_trigger_nai69_test.go` (append)

- [ ] **Step 1: Write the failing/passing test**

Append to `modules/world/interaction_trigger_nai69_test.go`:

```go
// --- NAI-69 T4: AP-Npc structural parity (no-op) pin ---

// TestFireApTriggerNpc_ApRangeCalled_SetsInteractionFiredTrueStructural
// pins the post-NAI-69 contract for AP-Npc: the fire helper sets
// interactionFired=true at exit and apRangeCalled is set by p_aprange
// (mechanism activates structurally) — but the next tryInteract call
// would re-evaluate using effectiveApRange = npc.typ.AttackRange
// (fixed per-type, not p.apRange), so the retry path is a behavioral
// no-op for NPC targets. This preserves the preexisting goscape
// divergence at interaction.go:393.
func TestFireApTriggerNpc_ApRangeCalled_SetsInteractionFiredTrueStructural(t *testing.T) {
	s, p, npc := newApTriggerNpcFixture(t)

	// Register an APNPC1 script for npc.typeId that calls p_aprange(2).
	sf := scriptFileWithApRangeCall(t, script.TriggerApNpc1, npc.typeId, 2)
	s.scriptProvider.Register(sf)

	fireApTriggerNpc(p, s, npc)

	// Mechanism activates structurally:
	if !p.interactionFired {
		t.Error("interactionFired: got false, want true (NAI-69: uniform exit)")
	}
	if !p.apRangeCalled {
		t.Error("apRangeCalled: got false, want true (script called p_aprange)")
	}
	if p.apRange != 2 {
		t.Errorf("apRange: got %d, want 2 (script set new range — but effectiveApRange reads npc.typ.AttackRange for NPC targets)", p.apRange)
	}

	// effectiveApRange divergence pin: for NPC targets, the retry
	// decision uses npc.typ.AttackRange, NOT p.apRange. Verify the
	// in-range check still uses the NPC's AttackRange.
	if effectiveApRange(p) != int(npc.typ.AttackRange) {
		t.Errorf("effectiveApRange: got %d, want %d (NPC AttackRange, not p.apRange — preexisting divergence)",
			effectiveApRange(p), npc.typ.AttackRange)
	}
}
```

- [ ] **Step 2: Run the test**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestFireApTriggerNpc_ApRangeCalled_SetsInteractionFiredTrueStructural -count=1 -v
```

Expected: PASS (T1+T2 changes already cover; AP-Npc fire helper was already uniform).

- [ ] **Step 3: Run the full test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add modules/world/interaction_trigger_nai69_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): NAI-69 T4 — AP-Npc structural parity (no-op) pin

Pins that AP-Npc fire helper exits with interactionFired=true and
apRangeCalled=true (mechanism activates structurally), AND that
effectiveApRange for NPC targets reads npc.typ.AttackRange (not
p.apRange) — so the same-tick retry path is a behavioral no-op for
NPC targets. Preserves the preexisting goscape divergence at
interaction.go:393.

NAI-69 T2 doc-comment header rephrase already documents this.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Doc-retirement at `interaction.go:237`

**Files:**
- Modify: `modules/world/interaction.go:236-241` (NAI-44 closure narration cleanup)

- [ ] **Step 1: Read the current doc-comment**

In `modules/world/interaction.go` at lines 236-245, the current doc-comment reads:

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
```

The lead-in `NAI-68 closes // NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET:` reads as if the deviation were still active (the second line opens with the tag-style identifier as if introducing it). Per `retire_deviation_grep_all_comments.md`, closure narrations should be unambiguously past-tense.

- [ ] **Step 2: Replace with a clean closure narration**

Edit `modules/world/interaction.go` — replace the doc-comment block at lines 236-245 with:

```go
	// nextTarget pop + auto-clear (TS L1255-1263). When an OP/AP
	// trigger script called p_op_* mid-trigger, the fire helpers
	// captured the script-set target into p.nextTarget; pop it here.
	// Otherwise, auto-clear the interaction. followOp paths can still
	// reach the else-if when tryInteract returned true at the pre-step
	// arm (contact range with target=*Player op=3); TS does the same —
	// followOp gates SKIP post-step-interact, not the auto-clear
	// itself. NAI-68 closed NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET via
	// this reshape; NAI-69 closes NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED
	// by routing the same-tick retry signal through tryInteract.
```

- [ ] **Step 3: Verify the closure narration is grep-stable**

```bash
rg -n "NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET" modules/world/ pkg/script/
```

Expected: 2 hits, both as closure narrations (`NAI-68 closed NAI-44-D-…` or `NAI-68 closes NAI-44-D-…`):
- `modules/world/interaction.go:~244` — the new text from this task.
- `modules/world/interaction_trigger.go:177` — pre-existing `// NAI-68 closes NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET.` line.

```bash
rg -n "NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED" modules/world/ pkg/script/
```

Expected: 1 hit (only the new closure narration in `interaction.go`). Production tag sites were already retired in T2.

- [ ] **Step 4: Run the full test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
```

Expected: PASS (doc-only change).

- [ ] **Step 5: Commit**

```bash
git add modules/world/interaction.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(world): NAI-69 T5 — doc-retirement sweep at interaction.go:237

Rephrases the nextTarget-pop doc-comment to make the NAI-44 closure
narration unambiguously past-tense (per retire_deviation_grep_all_comments
memory) and folds in the new NAI-69 closure mention. No behavioral
change.

Carry-forward cleanup from NAI-68 close.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Close commit

**Files:**
- (no file change — close commit only)

- [ ] **Step 1: Final pre-close grep verification**

```bash
rg -c "NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED" modules/ pkg/
```

Expected: 1 hit (the closure narration in `interaction.go`, not a production tag).

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1 -race
```

Expected: PASS with `-race`.

- [ ] **Step 2: Verify the commit chain since NAI-69 spec**

```bash
git log --oneline 95738e4..HEAD
```

Expected: 6 commits, in order:
1. `dca2ff3` docs(spec): NAI-69 spec
2. T1 commit
3. T2 commit
4. T3 commit
5. T4 commit
6. T5 commit

- [ ] **Step 3: Compose and push the close commit**

```bash
git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(close): NAI-69 — tryInteract same-tick AP retry (TS L1166-1170 port)

Closes NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED. AP-Loc / AP-Obj /
AP-Player scripts that call p_aprange mid-fire now apply the new
range and retry within the same tick (TS Player.ts:1163-1170),
replacing the pre-NAI-69 across-tick re-fire scaffold. AP-Npc gains
structural mechanism activation but stays a behavioral no-op
(effectiveApRange reads npc.typ.AttackRange — preexisting goscape
divergence preserved).

Mechanism: tryInteract's AP branch checks `nextTarget == nil &&
apRangeCalled` after the fire and resets interactionFired=false +
returns false. Fire helpers (Loc/Obj) drop the across-tick early-return;
all four AP fire helpers now exit with interactionFired=true uniformly.

Net deviation tally: 13 → 12.

Implementation timeline:
  T1 tryInteract guard + isolated tests
  T2 AP-Loc/AP-Obj fire-helper uniform exit + existing-test reframe
  T3 AP-Player same-tick retry pin + doc refresh
  T4 AP-Npc structural-parity (no-op) pin
  T5 interaction.go:237 doc-retirement

Spec: docs/superpowers/specs/2026-05-02-nai-69-tryinteract-aprange-same-tick-retry-design.md
Plan: docs/superpowers/plans/2026-05-02-nai-69-tryinteract-aprange-same-tick-retry.md

Closes memory: NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 4: Verify clean state**

```bash
git status
git log --oneline -10
```

Expected: clean working tree, last 7 commits are the NAI-69 chain (spec + 5 impl tasks + close).

---

## Self-Review Checklist (run before declaring plan complete)

- [ ] **Spec coverage:** All §5 code-map entries in the spec map to a task. ✅ (T1: tryInteract; T2: AP-Loc/Obj fire helpers; T3: AP-Player; T4: AP-Npc; T5: interaction.go:237 doc).
- [ ] **All §7 tests have a task:** ✅ (T1: TryInteract_ApRangeCalled / NoApRange; T2: FireApTriggerLoc / Obj uniform-exit + reframed existing; T3: AP-Player parity; T4: AP-Npc structural parity).
- [ ] **No placeholders.** ✅
- [ ] **Type/method consistency.** ✅ (`tryInteract`, `tryFireApTrigger`, `fireApTriggerLoc`, `fireApTriggerObj`, `fireApTriggerPlayer`, `fireApTriggerNpc`, `effectiveApRange`, `scriptFileWithApRangeCall`, `makeApTriggerFixture`, `makeApObjTriggerFixture`, `newApTriggerNpcFixture`, `newPlayerTriggerFixture` — all verified against HEAD).
- [ ] **Pre-flight verification step:** ✅ at top.
- [ ] **Closes memory trailer in T6 close commit:** ✅
- [ ] **Spec §3 out-of-scope items NOT mistakenly included:** ✅ (`interactionFired` field-removal deferred; `NAI-44-D-CANACCESS-NO-STUN-CHECK` untouched; AP-Npc functional retry untouched).
