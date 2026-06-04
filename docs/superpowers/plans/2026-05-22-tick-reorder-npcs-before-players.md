# Tick-Reorder: processNpcs Before Player Block Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore TS-faithful tick order in `modules/world/tick.go` by running `processNpcs()` before the player-side block (between `processNpcEventQueue` and `processActiveScripts`). Fixes the in-game symptom "player follows wandering NPC but never attacks" by ensuring NPC has moved to its end-of-tick position BEFORE the player's `processInteraction` reads `inOperableDistance`.

**Architecture:** TS World.cycle (Engine-TS src/engine/World.ts:338-413) runs `processNpcs()` at L365 — strictly before `processPlayers()` at L376. Go currently runs `processNpcs()` at the end of the per-tick player block (tick.go:109), after `processPathing` (L106), `processInteractions` (L107), and `processEnergy` (L108). Move it to between `processNpcEventQueue` (L94) and `processActiveScripts` (L95). One-line move, plus comment updates, plus regression-test additions. The reorder is pure code motion — no per-method logic changes.

**Tech Stack:** Go 1.26.3, `go test -race`, in-repo `modules/world` test helpers.

---

## Scope Notes

**Affected NPC AI logic:** Only `huntPlayers()` in `modules/world/npc_hunt.go` reads player coordinates inside `processNpcs`. Post-reorder it sees last-tick-end player position (TS-faithful) instead of this-tick-after-pathing position. No algorithmic changes needed — the read is range-based and a one-tick stale read matches TS exactly.

**Existing pin to update:** `TestTickPhaseOrder_NpcEventQueueBeforeInteractions` in `modules/world/tick_order_test.go` is a source-text offset assertion. We extend it with a second invariant that pins `processNpcs` before `processActiveScripts`.

**NAI-122-D3 deviation marker** at `tick.go:84-93` documents `processNpcEventQueue`'s position relative to `processInteractions`. The new ordering closes a related-but-distinct gap; we leave NAI-122-D3 intact and write a new NAI marker for the `processNpcs` reorder.

---

## File Inventory

| Action | Path | Responsibility |
|---|---|---|
| Modify | `modules/world/tick.go:83-117` | Move `s.processNpcs()` call from after `processEnergy` to before `processActiveScripts`. Add NAI marker comment. |
| Modify | `modules/world/tick_order_test.go` | Add a new test pin asserting `processNpcs` < `processActiveScripts` source offsets. |
| (Optional) Modify | `modules/world/movement_test.go:223` | Update comment if it references the old ordering. |
| Add | `modules/world/tick_order_test.go` | New `TestProcessNpcsBeforePlayerBlock_TickFaithfulOrder` source-text pin. |

---

## Task 1: Pin the new ordering invariant (failing test)

**Files:**
- Modify: `modules/world/tick_order_test.go`

- [ ] **Step 1: Read current tick_order_test.go to see the existing pattern**

Run: `cat modules/world/tick_order_test.go | head -60`

Expected: see `TestTickPhaseOrder_NpcEventQueueBeforeInteractions` which scans `tick.go` source text for two call-site offsets and asserts the first is earlier than the second.

- [ ] **Step 2: Add a new sub-test for processNpcs-before-processActiveScripts**

Add this test function to `modules/world/tick_order_test.go` (place it directly after `TestTickPhaseOrder_NpcEventQueueBeforeInteractions`):

```go
// TestTickPhaseOrder_NpcsBeforePlayerBlock pins the TS-faithful ordering
// where processNpcs() runs BEFORE the player-side block (starting at
// processActiveScripts). Mirrors TS World.cycle ordering at
// Engine-TS/src/engine/World.ts:365 (processNpcs) → :376 (processPlayers).
//
// Without this invariant, the player's processInteraction reads an NPC
// position from end-of-previous-tick (since processNpcs hasn't run yet
// this tick). For a wandering NPC moving toward the player during
// processNpcs, the player's inOperableDistance check measures distance
// to the NPC's stale position — one tile further than where the NPC
// will actually end up this tick — so branch 1 (OP fire) skips even
// though the player ends the tick visually adjacent. Symptom in-game:
// "player follows wandering NPC but never swings".
func TestTickPhaseOrder_NpcsBeforePlayerBlock(t *testing.T) {
	src, err := os.ReadFile("tick.go")
	if err != nil {
		t.Fatalf("read tick.go: %v", err)
	}
	body := string(src)

	npcsIdx := strings.Index(body, "s.processNpcs()")
	if npcsIdx < 0 {
		t.Fatal("tick.go does not contain s.processNpcs() call")
	}
	scriptsIdx := strings.Index(body, "s.processActiveScripts()")
	if scriptsIdx < 0 {
		t.Fatal("tick.go does not contain s.processActiveScripts() call")
	}

	if npcsIdx >= scriptsIdx {
		t.Errorf("s.processNpcs() at offset %d must appear BEFORE s.processActiveScripts() at offset %d in tick.go (TS World.cycle L365→L376 ordering). Player's processInteraction must see this-tick NPC position, not last-tick-end.",
			npcsIdx, scriptsIdx)
	}
}
```

Also verify imports include `os` and `strings`. If `tick_order_test.go` already imports them (for the existing `TestTickPhaseOrder_NpcEventQueueBeforeInteractions`), no changes needed.

- [ ] **Step 3: Run the new test pre-fix — must fail**

Run: `GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test ./modules/world/ -run TestTickPhaseOrder_NpcsBeforePlayerBlock -count=1 -v`

Expected output (FAIL):
```
--- FAIL: TestTickPhaseOrder_NpcsBeforePlayerBlock (0.00s)
    tick_order_test.go:XX: s.processNpcs() at offset NNNN must appear BEFORE s.processActiveScripts() at offset MMMM in tick.go ...
FAIL
```

If it PASSES pre-fix, something is wrong — investigate before proceeding. (Probably `tick.go` was already changed; revert and retry.)

---

## Task 2: Move processNpcs() to the new position

**Files:**
- Modify: `modules/world/tick.go:83-117`

- [ ] **Step 1: Read the current tick.go cycle body**

Run: `sed -n '83,117p' modules/world/tick.go`

Expected: see the call sequence from `s.processClientsIn()` through `s.processCleanup()`.

- [ ] **Step 2: Apply the reorder**

Delete the `s.processNpcs()` call currently at the line between `s.processEnergy()` and `s.processLogouts()`. Insert it as a new call between `s.processNpcEventQueue()` and `s.processActiveScripts()`, with a new NAI-XXX comment block documenting the TS-parity rationale. Reuse the next free NAI number — at the time of writing this plan that is **NAI-217**; bump if a parallel commit has claimed it.

Use Edit to replace the block:

```go
		s.processNpcEventQueue()
		s.processActiveScripts()
```

with:

```go
		s.processNpcEventQueue()
		// NAI-217: processNpcs moved up to mirror TS World.cycle order
		// (Engine-TS/src/engine/World.ts:365 processNpcs → :376
		// processPlayers). Player-side processInteraction at L107 must
		// see this-tick NPC positions (after the NPC moved THIS cycle),
		// not the stale end-of-previous-tick positions that resulted
		// when processNpcs ran later. Pre-NAI-217 symptom: when the
		// player chases a wandering NPC, inOperableDistance measures
		// against the NPC's last-tick-end position, so branch-1 OP
		// fire skips even though the NPC will end this tick visually
		// adjacent. processNpcs internally drives NPC ai_spawn resume,
		// stat regen, timer, queue, movement, and modes — all of which
		// must settle before the per-player block reads NPC state.
		s.processNpcs()
		s.processActiveScripts()
```

Then, lower in the same function body, delete the now-redundant `s.processNpcs()` call AND `s.processEnergy()`'s relationship — actually `processEnergy` stays in place (it's player-side). Just remove the bare `s.processNpcs()` line.

Use Edit to replace the block:

```go
		s.processInteractions()
		s.processEnergy() // NAI-135: TS World.ts:731 per-player updateEnergy
		s.processNpcs()
		s.processLogouts()
```

with:

```go
		s.processInteractions()
		s.processEnergy() // NAI-135: TS World.ts:731 per-player updateEnergy
		s.processLogouts()
```

- [ ] **Step 3: Verify the structural reorder visually**

Run: `sed -n '83,117p' modules/world/tick.go`

Expected: `s.processNpcs()` appears between `s.processNpcEventQueue()` (with its NAI-122 comment block above it) and `s.processActiveScripts()`. There is NO `s.processNpcs()` between `s.processEnergy()` and `s.processLogouts()` anymore.

- [ ] **Step 4: Run the new ordering pin — must pass now**

Run: `GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test ./modules/world/ -run TestTickPhaseOrder_NpcsBeforePlayerBlock -count=1 -v`

Expected: `--- PASS: TestTickPhaseOrder_NpcsBeforePlayerBlock`

- [ ] **Step 5: Verify the existing NAI-122 pin still passes**

Run: `GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test ./modules/world/ -run TestTickPhaseOrder_NpcEventQueueBeforeInteractions -count=1 -v`

Expected: PASS. `processNpcEventQueue` is still BEFORE `processInteractions` (its position didn't change; we only moved `processNpcs`).

---

## Task 3: Sweep stale ordering comments

**Files:**
- Modify: `modules/world/movement_test.go:221-225` (if it documents old order)
- Modify: any other `*.go` files with stale "processNpcs runs after processInteractions" comments

- [ ] **Step 1: Grep for stale ordering claims**

Run: `grep -rn "processNpcs.*after\|processNpcs.*later\|processInteractions.*before processNpcs\|processPathing.*before processNpcs\|processEnergy.*before processNpcs" modules/world/ --include="*.go"`

Expected: a small set of comment hits (likely zero or one). The Explore agent's scope flagged `movement_test.go:223` ("processInteraction (which runs after processPathing) reads the per-tick step count") — this claim is unaffected by the reorder (processPathing still runs before processInteractions) and should stay as is. Hits elsewhere need case-by-case review.

- [ ] **Step 2: Update any comment that explicitly claims processNpcs runs AFTER the player block**

If any comment claims this, replace the relevant phrase with a TS-parity citation. Example template for a comment update:

> // Pre-NAI-217: processNpcs ran AFTER processInteractions; the player saw
> // stale NPC positions. Post-NAI-217: processNpcs runs BEFORE the player
> // block (mirrors TS World.cycle L365 → L376).

Skip any comment that's already correct or unrelated.

If the grep returns no hits beyond `movement_test.go:223` (which stays unchanged), proceed to Task 4.

---

## Task 4: Full repo test gate

**Files:** (none — verification only)

- [ ] **Step 1: Run the full modules/world race-test suite**

Run: `GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test -race ./modules/world/ -count=1 2>&1 | tail -10`

Expected: `ok  github.com/zsrv/goscape/modules/world  ~155s` (one OK line).

If failures occur, the most likely candidates per the scoping agent's risk assessment are:
1. `TestTickPhaseOrder_*` — verify the new test passes (Task 2 step 4); update assertions if they conflict with the new layout.
2. `TestHuntPlayers*` in `npc_hunt_test.go` — these call `huntPlayers()` directly and should NOT be affected by the tick-order move, but if a test set up a "player just moved this tick" fixture state, the test should now reflect the new TS-faithful contract.
3. Any integration test that simulated a full tick and asserted player-NPC distance progression — re-derive the expected distance using the new ordering.

For each failure, READ the test fully, REASON about what the test pins (the old ordering convention or a semantic invariant), and update minimally. Do NOT mask failures by changing assertions to match the new behavior unless the new behavior is correct per TS.

- [ ] **Step 2: Run the full repo race-test suite**

Run: `GOROOT=/home/owner/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache /home/owner/go/go1.26.3/bin/go test -race ./... -count=1 2>&1 | grep -E "^(FAIL|ok\s)" | head -25`

Expected: every line starts with `ok  ` (no FAIL). Timeouts in 5-minute range are acceptable for modules/world.

- [ ] **Step 3: Run gofmt**

Run: `GOROOT=/home/owner/go/go1.26.3 /home/owner/go/go1.26.3/bin/gofmt -l modules pkg cmd internal`

Expected: empty output (no files need re-formatting).

---

## Task 5: Commit

**Files:** all staged from Tasks 1-3.

- [ ] **Step 1: Confirm git status shows only intentional changes**

Run: `git status --porcelain`

Expected output (paths):
```
 M modules/world/tick.go
 M modules/world/tick_order_test.go
```

Plus possibly `modules/world/movement_test.go` if Task 3 found a hit. No other modules/world/*.go modified unless Task 4 surfaced a real test fix.

- [ ] **Step 2: Inspect the diff one more time**

Run: `git diff modules/world/tick.go modules/world/tick_order_test.go | head -80`

Expected: see the `s.processNpcs()` line moved up; the new test function added; no unrelated edits.

- [ ] **Step 3: Commit with a thorough message**

Run:

```bash
git add modules/world/tick.go modules/world/tick_order_test.go && git -c commit.gpgsign=false commit -m "$(cat <<'EOF'
fix(world): reorder tick so processNpcs runs before the player block

User-reported in-game bug: when attacking a wandering NPC, the player
follows the NPC visually adjacent at end-of-tick but never swings.
Pre-fix root cause: Go's tick cycle ran processNpcs AFTER the
player-side block (processPathing + processInteractions + processEnergy
at modules/world/tick.go:106-108, with processNpcs at L109), so the
player's inOperableDistance check inside processInteraction measured
distance to the NPC's last-tick-end position. When the NPC then moved
TOWARD the player during processNpcs, the visual end-of-tick state
showed them adjacent — but the engine had already passed the OP fire
opportunity.

TS Engine-TS World.cycle (src/engine/World.ts:338-413) runs
processNpcs at L365 strictly BEFORE processPlayers at L376. The Go
port inverted this. NAI-122 had already moved processNpcEventQueue up
to match TS L356; the broader processNpcs move was not done at that
time.

Fix: move `s.processNpcs()` from between processEnergy and
processLogouts to between processNpcEventQueue and processActiveScripts
(the start of the per-tick player-side block). One-line code motion.

Affected NPC AI logic: huntPlayers() at modules/world/npc_hunt.go:113-218
is the only consumer of player coords inside processNpcs. It now sees
last-tick-end player position (TS-faithful) instead of this-tick-after-
pathing position. No algorithmic changes — the read is range/LoS-based
and a one-tick stale read matches TS exactly.

New regression pin TestTickPhaseOrder_NpcsBeforePlayerBlock in
modules/world/tick_order_test.go scans tick.go source text and asserts
the offset of `s.processNpcs()` is strictly less than the offset of
`s.processActiveScripts()`. Mirrors the existing NAI-122 pin
(TestTickPhaseOrder_NpcEventQueueBeforeInteractions). Fails on the
pre-reorder tree; passes after.

Verified: `go test -race ./... -count=1` passes (modules/world full
race suite included). gofmt clean.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)" && git log --oneline -5
```

Expected: the new HEAD commit lands cleanly, `git log --oneline -5` shows the new commit on top of `9b03fd10`, `f9b4cc20`, `7fc5e8c5`, `0b20de20`.

---

## Self-Review Notes

**Spec coverage:**
- Reorder code change → Task 2.
- New regression pin → Task 1.
- Existing NAI-122 pin still works → Task 2 step 5.
- Stale comment sweep → Task 3.
- Full test gate → Task 4.
- Commit → Task 5.

**Placeholder scan:** No "TODO", "implement later", or "see also" placeholders. Every step has the actual command or code block needed.

**Type consistency:** Function names referenced (`s.processNpcs()`, `s.processActiveScripts()`, etc.) are exact matches to `tick.go` symbol names verified during scoping. NAI-217 is the proposed marker number — agent should bump if a parallel commit landed in the interim (`grep -rn "NAI-217" modules pkg cmd internal docs` should be empty pre-execution).
