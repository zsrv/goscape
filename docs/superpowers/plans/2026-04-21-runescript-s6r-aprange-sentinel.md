# S6r — apRange=-1 Sentinel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Set `p.apRange = -1` when `fireApTriggerLoc` finds no registered AP script, caching the "no-AP" result so subsequent ticks short-circuit out of the AP branch. Closes S6l-D1.

**Architecture:** One production assignment + two comment-block updates (deviation → closure form) + 3 new tests.

**Tech Stack:** Go 1.26, standard `testing`.

**Spec:** `docs/superpowers/specs/2026-04-21-runescript-s6r-aprange-sentinel-design.md` (commit `c488c64`).

---

## Single task

### Task 1: Sentinel + tests + deviation closure

**Files:**
- Modify: `modules/world/interaction_trigger.go` — `fireApTriggerLoc`'s `if sf == nil` branch
- Modify: `modules/world/interaction.go` — `processInteraction`'s AP-branch comment
- Modify: `modules/world/interaction_trigger_test.go` — 3 new tests; update any stale S6l-D1 doc-strings

### TDD context

Red-green cycle: write 3 tests → verify they fail (well, one might pass pre-impl because the current code just doesn't set apRange — tests probing "apRange remains 10" would pass) → implement → green.

Actually the cleanest failing assertion is: "after `fireApTriggerLoc` with no script, `p.apRange == -1`." Pre-impl, apRange stays at its prior value (10 after SetInteraction). The assertion `p.apRange == -1` fails. Post-impl, passes.

- [ ] **Step 1: Capture test baseline.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Record the PASS count (expected ~310+ based on post-S6p state).

- [ ] **Step 2: Write 3 failing tests.**

Append to `modules/world/interaction_trigger_test.go`:

```go
// TestFireApTriggerLocNoScriptSetsApRangeSentinel verifies that when
// fireApTriggerLoc finds no registered AP script for (trigger,
// locType, category), it sets p.apRange = -1 as a sentinel. Closes
// S6l-D1: matches TS Player.ts:~1139-1170 apRange=-1 semantics.
func TestFireApTriggerLocNoScriptSetsApRangeSentinel(t *testing.T) {
	s, p, loc, _ := makeOpLocFixture(t)
	// Anchor an OpLoc1 interaction. makeOpLocFixture registers
	// LocType 42 but NO AP script for it.
	p.SetInteraction(InteractionEngine, loc, 1, -1)
	p.targetSubject.typ = loc.Type()
	p.targetSubject.x = loc.X
	p.targetSubject.z = loc.Z
	p.targetSubject.level = loc.Level

	// Sanity: apRange starts at 10 (SetInteraction default).
	if p.apRange != 10 {
		t.Fatalf("apRange pre-fire: got %d, want 10", p.apRange)
	}

	fireApTriggerLoc(p, s, loc)

	if p.apRange != -1 {
		t.Errorf("apRange post no-script fire: got %d, want -1 (S6l-D1 sentinel)", p.apRange)
	}
}

// TestApRangeSentinelShortCircuitsApproachGate verifies that with
// p.apRange = -1, inApproachDistance returns false regardless of
// actual player-to-target distance. This is how the sentinel skips
// re-lookup on subsequent ticks.
func TestApRangeSentinelShortCircuitsApproachGate(t *testing.T) {
	// Player at (100, 100), target at (101, 100) — distance 1 tile.
	// With apRange=-1, should return false even though distance <
	// any positive apRange.
	if inApproachDistance(100, 100, 101, 100, -1) {
		t.Error("inApproachDistance should return false when apRange=-1 (sentinel)")
	}

	// Control: with apRange=5, same positions should return true.
	if !inApproachDistance(100, 100, 101, 100, 5) {
		t.Error("control: inApproachDistance should return true when apRange=5 and distance=1")
	}
}

// TestSetInteractionResetsApRangeSentinel verifies that starting a
// fresh interaction clears the -1 sentinel. Codifies the contract
// so future refactors can't regress it silently.
func TestSetInteractionResetsApRangeSentinel(t *testing.T) {
	_, p, loc, _ := makeOpLocFixture(t)
	p.apRange = -1 // simulate a prior sentinel state

	p.SetInteraction(InteractionEngine, loc, 3, -1)

	if p.apRange != 10 {
		t.Errorf("apRange post SetInteraction: got %d, want 10 (sentinel should be reset)", p.apRange)
	}
}
```

- [ ] **Step 3: Run tests to verify 1/3 fails.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestFireApTriggerLocNoScriptSetsApRangeSentinel|TestApRangeSentinelShortCircuitsApproachGate|TestSetInteractionResetsApRangeSentinel' -v`
Expected:
- `TestFireApTriggerLocNoScriptSetsApRangeSentinel` — **FAIL** (apRange stays 10 pre-impl)
- `TestApRangeSentinelShortCircuitsApproachGate` — PASS (inApproachDistance already guards `apRange <= 0`)
- `TestSetInteractionResetsApRangeSentinel` — PASS (SetInteraction already sets apRange=10)

- [ ] **Step 4: Apply the sentinel + deviation-comment closure in `interaction_trigger.go`.**

Locate the block in `fireApTriggerLoc` (approximately lines 355-362). Current:

```go
sf := srv.scriptProvider.GetByTrigger(trigger, loc.Type(), category)
if sf == nil {
    // No AP script registered. DEVIATION S6l-D1: skip TS apRange=-1
    // sentinel. Interaction stays anchored; next tick re-evaluates.
    // If player has reached contact, OP/defaultOp takes over.
    p.interactionFired = true
    return
}
```

Replace with:

```go
sf := srv.scriptProvider.GetByTrigger(trigger, loc.Type(), category)
if sf == nil {
    // S6l-D1 closed in S6r: cache "no AP script for this (trigger,
    // locType, category) triple" via the apRange=-1 sentinel so
    // inApproachDistance short-circuits on subsequent ticks.
    // Matches TS Player.ts:~1139-1170 behavior: apRange=-1 means
    // "AP path permanently disabled for this interaction;
    // anchor stays — contact (OP) takes over on a later tick."
    p.apRange = -1
    p.interactionFired = true
    return
}
```

- [ ] **Step 5: Update the `processInteraction` comment in `interaction.go`.**

Locate the AP-branch in `processInteraction` (approximately lines 103-112). Current:

```go
if inApproachDistance(p.x, p.z, tx, tz, effectiveApRange(p)) {
    // Approach range — fire AP. Matches TS Player.ts:1139-1170.
    // DEVIATION S6l-D1: goscape skips TS's apRange=-1 sentinel
    // optimization; each tick does a fresh provider lookup.
    p.interacted = true
```

Replace the 3-line deviation comment with:

```go
if inApproachDistance(p.x, p.z, tx, tz, effectiveApRange(p)) {
    // Approach range — fire AP. Matches TS Player.ts:1139-1170.
    // S6l-D1 closed in S6r: when fireApTriggerLoc finds no script,
    // it sets p.apRange = -1. Next tick's inApproachDistance sees
    // apRange <= 0 and returns false, skipping re-lookup.
    p.interacted = true
```

- [ ] **Step 6: Run the new tests.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestFireApTriggerLocNoScriptSetsApRangeSentinel|TestApRangeSentinelShortCircuitsApproachGate|TestSetInteractionResetsApRangeSentinel' -v`
Expected: PASS × 3.

- [ ] **Step 7: Check for any pre-existing tests whose S6l-D1 doc-comments are now stale.**

Grep: `modules/world/interaction_trigger_test.go` for `S6l-D1`. Any docstring mentioning the deviation-open form should be updated to reference "closed in S6r" OR left alone if the surrounding test semantics still hold. Judgment call — prefer minimal churn (leave if unclear).

Earlier Grep showed one such reference at `interaction_trigger_test.go:423`:

```go
// DEVIATION S6l-D1: goscape skips TS's apRange=-1 sentinel. The
```

Read the test body around that line — if the test is still describing behavior that now differs (i.e., it tests "we don't set apRange=-1" and that's now false), it's a regression target. If it tests tangential behavior and just happens to mention D1 in passing, a comment swap suffices. Read context before editing.

- [ ] **Step 8: Run full world suite + vet.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: PASS (baseline + 3). No existing tests should regress because the sentinel only fires on "no AP script" paths, which existing tests don't exercise in a way that depends on `apRange` staying at 10.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`
Expected: clean.

- [ ] **Step 9: Run race detector.**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...`
Expected: PASS.

- [ ] **Step 10: Commit.**

```bash
git add modules/world/interaction_trigger.go \
        modules/world/interaction.go \
        modules/world/interaction_trigger_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): apRange=-1 sentinel for fireApTriggerLoc no-AP-script path — closes S6l-D1 (S6r)

When fireApTriggerLoc finds no registered AP script for the current
(trigger, locType, category) triple, set p.apRange = -1 as a sentinel.
Subsequent ticks see apRange <= 0 in inApproachDistance (guard at
interaction.go:145), which returns false, skipping the re-lookup and
falling through to path-to-target or contact-detection (OP).

Matches TS Player.ts:~1139-1170 behavior exactly: apRange=-1 means
"AP path permanently disabled for this interaction; anchor stays."

SetInteraction and ClearInteraction already reset apRange to 10, so
the sentinel cleanly scopes to a single interaction. Scope is Loc-
only — the NPC path already calls ClearInteraction on sf==nil so has
no next-tick AP re-entry to optimize.

Tests: 3 new (sentinel-sets-on-no-script, apRange<=0 short-circuits,
SetInteraction resets). No regressions in existing tests.

Closes S6l-D1. No new deviations.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```
