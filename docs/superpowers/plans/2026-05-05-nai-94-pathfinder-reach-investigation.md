# NAI-94 Pathfinder Reach Stage 1 Investigation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stage 1 audit + pinned reproducer tests for NAI-92's pathfinder-reach residual; identify root cause for "Hans cheb=2 returns no path" and "Survival Expert behind cabin wall returns short partial path", or document the diagnosis ceiling and route to NAI-95.

**Architecture:** Hybrid probe-then-diff. Write a minimum-distance reproducer (H1) first; branch the audit on its outcome. Diagnosis report compiled per-hypothesis. No production code changes — only tests under `pkg/pathfinder/routefinder/` and docs under `docs/superpowers/investigations/`.

**Tech Stack:** Go 1.26+. Reference upstream: `2004scape/rsmod-pathfinder` AssemblyScript HEAD at `/home/owner/Code/github.com/2004scape/rsmod-pathfinder/src/rsmod/`. Pinned production version is v5.0.4 (Rust + wasm-pack); reachable via `git checkout 8dd111e` in the rsmod-pathfinder repo if AS↔Rust algo divergence is suspected.

**Spec:** `docs/superpowers/specs/2026-05-05-nai-94-pathfinder-reach-investigation-design.md`

---

## File Structure

**Created:**
- `pkg/pathfinder/routefinder/nai94_repro_test.go` — pinned reproducer tests, `t.Skip` if anomaly reproduces, marked `// NAI-94:` for grep-discoverability.
- `docs/superpowers/investigations/2026-05-05-nai-94-diagnosis.md` — per-hypothesis verdict + file:line evidence + Stage 2 (NAI-95) handoff.

**Modified:**
- `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` — append "From NAI-94" section.

**Read-only references:**
- `pkg/pathfinder/routefinder/routefinder.go` (656 lines; goscape's BFS port)
- `pkg/pathfinder/routefinder/stepvalidator.go` (235 lines; collision-flag step validation)
- `pkg/pathfinder/routefinder/api.go` (354 lines; `FindPathPlain`/`FindPathToEntity`/`FindPathToLoc` wrappers)
- `pkg/pathfinder/collision/` (FlagMap + flag bit layout)
- `/home/owner/Code/github.com/2004scape/rsmod-pathfinder/src/rsmod/PathFinder.ts` (695 lines; AS reference algo)
- `/home/owner/Code/github.com/2004scape/rsmod-pathfinder/src/rsmod/StepValidator.ts` (244 lines; AS reference step-validation)

---

## Conventions for this plan

- **Reproducer disposition:** Each reproducer test is written as a *real* assertion against expected behavior. Run it. If it FAILS (anomaly reproduces), wrap the assertion in a `t.Skip` block immediately above with `// NAI-94: ...` and pin the OBSERVED behavior in the skip body so NAI-95 has a precise diff target. If it PASSES, leave it as a passing test and note the elimination in the diagnosis report.
- **No production code changes.** If a task's audit surfaces a "smoking gun" one-line fix opportunity, document it in the diagnosis report's Stage 2 handoff section — do **not** apply it in NAI-94.
- **Subagent fabrication guard** (`audit_subagent_fabrication`, `verify_implementer_claims`): if any audit task is delegated, controller verifies every claimed file:line citation with `git show HEAD -- <file>` / `rg` / `Read` before merging into the diagnosis report.
- **`go` invocation prefix** (per global CLAUDE.md): every `go test`/`go build` runs as `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`.

---

## Task 1: H1 — Hans cheb=2 trivial-path reproducer

**Hypothesis:** BFS / waypoint return is broken at minimum-distance paths.

**Files:**
- Create: `pkg/pathfinder/routefinder/nai94_repro_test.go`

- [ ] **Step 1.1: Write the reproducer test (asserts the *expected* behavior; will fail if H1 fires)**

```go
package routefinder

import (
	"testing"
)

// TestNAI94_HansCheb2_StraightLineMustReach is the H1 reproducer for NAI-94.
// World coords: src=(3219, 3224), dest=(3219, 3222). Cheb=2, straight-line N→S
// move with empty FlagMap (no walls). Smoke (2026-05-05) showed real-game
// dispatch returns waypoint_idx=-1 for this exact shape. This unit test pins
// the same coords against the actual RouteFinder API to determine whether
// the bug is in the pathfinder algo itself or in something upstream.
//
// Disposition (per NAI-94 plan §"Conventions"):
//   - If FAILS (anomaly reproduces here): wrap in t.Skip, pin observed
//     behavior, route to NAI-95.
//   - If PASSES: H1 is eliminated against the unit-level pathfinder; the
//     real-game bug is upstream of the pathfinder API. Document in diagnosis.
func TestNAI94_HansCheb2_StraightLineMustReach(t *testing.T) {
	pf := NewPathFinderAPI()

	const (
		level = 0
		srcX  = 3219
		srcZ  = 3224
		dstX  = 3219
		dstZ  = 3222
	)

	route := pf.FindPathPlain(level, srcX, srcZ, dstX, dstZ)

	if !route.Success {
		t.Fatalf("Route.Success=false; expected pathfinder to succeed on cheb=2 straight-line with empty FlagMap. Route=%+v", route)
	}
	if len(route.Waypoints) == 0 {
		t.Fatalf("Route.Waypoints empty; expected at least one waypoint reaching (%d, %d)", dstX, dstZ)
	}
	last := route.Waypoints[len(route.Waypoints)-1]
	if last.X() != dstX || last.Z() != dstZ {
		t.Fatalf("last waypoint = (%d, %d); want (%d, %d). Full waypoints: %+v", last.X(), last.Z(), dstX, dstZ, route.Waypoints)
	}
}
```

- [ ] **Step 1.2: Run the test**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestNAI94_HansCheb2_StraightLineMustReach -v ./pkg/pathfinder/routefinder/
```

Expected outcome: **either** PASS (H1 eliminated at unit level — real-game bug is upstream) **or** FAIL (H1 confirmed). Record the actual `Route` value from the failure message verbatim — that's the pin.

- [ ] **Step 1.3: Disposition based on Step 1.2 result**

If the test PASSED: leave it as a passing assertion. Add a comment block at the top of the file noting the unit-level path succeeded, so the diagnosis report can cite this elimination.

If the test FAILED: edit the test to wrap the body in a skip-with-pin pattern. Replace the body with:

```go
func TestNAI94_HansCheb2_StraightLineMustReach(t *testing.T) {
	t.Skip("NAI-94: H1 reproducer — pathfinder returns no/short path on cheb=2 straight-line with empty FlagMap. " +
		"Observed Route at NAI-94 audit time: <PASTE EXACT ROUTE VALUE FROM STEP 1.2>. " +
		"Lift this skip in NAI-95 once the fix lands.")

	pf := NewPathFinderAPI()
	// ... unchanged body above ...
}
```

- [ ] **Step 1.4: Run the test once more to verify final state**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestNAI94_HansCheb2_StraightLineMustReach -v ./pkg/pathfinder/routefinder/
```

Expected: PASS (either as a real pass, or as `--- SKIP: TestNAI94_HansCheb2_StraightLineMustReach`).

- [ ] **Step 1.5: Commit**

```bash
git add pkg/pathfinder/routefinder/nai94_repro_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(routefinder): NAI-94 T1 — H1 Hans cheb=2 reproducer

Pins NAI-92 smoke shape: FindPathPlain(0, 3219, 3224, 3219, 3222)
on empty FlagMap. Disposition recorded inline.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: H2 — `useRouteBlockerFlags` audit

**Hypothesis:** The `useRouteBlockerFlags bool` field at `routefinder.go:43` is wired but never consulted in BFS step expansion. If goscape never branches on it where the AS reference does, route-blocker semantics (closed-door tiles) are entirely missing.

**Files:**
- Modify: `pkg/pathfinder/routefinder/nai94_repro_test.go` (add subtests)
- (Read-only) `pkg/pathfinder/routefinder/routefinder.go`, `pkg/pathfinder/routefinder/stepvalidator.go`
- (Read-only) `/home/owner/Code/github.com/2004scape/rsmod-pathfinder/src/rsmod/PathFinder.ts`, `StepValidator.ts`

- [ ] **Step 2.1: Grep goscape for `useRouteBlockerFlags` consumers**

```bash
rg -n "useRouteBlockerFlags|RouteBlocker|FlagWallWestRouteBlocker|FlagLocRouteBlocker" pkg/pathfinder/ pkg/grid/ pkg/zone/ modules/world/ 2>/dev/null
```

Record every match with file:line. Note specifically whether `useRouteBlockerFlags` is read anywhere outside its declaration at `routefinder.go:43`.

- [ ] **Step 2.2: Grep AS reference for the equivalent flag/branch**

```bash
rg -n "RouteBlocker|routeblocker|breakRouteFinding" /home/owner/Code/github.com/2004scape/rsmod-pathfinder/src/rsmod/
```

Record which functions in `PathFinder.ts` and `StepValidator.ts` consult route-blocker flags, and on which BFS expansion branches.

- [ ] **Step 2.3: Diff: enumerate every AS site that consults route-blocker, then check each one's goscape counterpart**

For each AS site found in Step 2.2, identify the corresponding goscape function (likely in `routefinder.go` or `stepvalidator.go`), Read the relevant lines, and record:
- AS path: file:line + the route-blocker-reading expression
- Goscape path: file:line + whether the equivalent expression exists or is missing/inverted/dead

The output is a 2-column table you'll paste into the diagnosis report at Task 6.

- [ ] **Step 2.4: Add H2 reproducer subtests to `nai94_repro_test.go`**

Append the following test:

```go
// TestNAI94_RouteBlockerFlag_Consulted is the H2 reproducer. Builds a 5×5
// synthetic grid with a single FlagWallWestRouteBlocker at (3, 2). With
// useRouteBlockerFlags=true (NPC pathing in TS), the route should refuse to
// step W→E across that tile boundary. With useRouteBlockerFlags=false
// (player pathing in TS), the route should pass through.
//
// In goscape, RouteFinder is constructed via NewRouteFinderDefault with
// useRouteBlockerFlags=false (per api.go:28). If the field is unconsulted
// (the // TODO at routefinder.go:43), both subtests behave identically and
// H2 is confirmed.
func TestNAI94_RouteBlockerFlag_Consulted(t *testing.T) {
	const (
		level = 0
		// Use synthetic local coords centered well away from real mapsquares
		// to avoid accidentally seeding any pre-existing flag state.
		srcX = 3000
		srcZ = 3000
		dstX = 3004
		dstZ = 3000
	)

	for _, tc := range []struct {
		name                 string
		useRouteBlockerFlags bool
		wantSuccess          bool
		wantReachesDest      bool
	}{
		{name: "BlockerHonored_RefusesToCross", useRouteBlockerFlags: true, wantSuccess: false, wantReachesDest: false},
		{name: "BlockerIgnored_PassesThrough", useRouteBlockerFlags: false, wantSuccess: true, wantReachesDest: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flags := collision.NewFlagMap()
			// Plant a route-blocker on tile (3002, 3000) blocking westward step
			// from (3003, 3000) → (3002, 3000). FlagWallWestRouteBlocker on the
			// destination tile blocks entry from the east side.
			flags.Add(3002, 3000, level, collision.FlagWallWestRouteBlocker)

			rf := NewRouteFinder(flags, routefinderDefaultSearchMapSize, routefinderDefaultRingBufferSize, tc.useRouteBlockerFlags)

			route := rf.FindRoute(level, srcX, srcZ, dstX, dstZ, 1, 1, 1, 0, -1, true, 0, 25, collision.TypeNormal)

			if route.Success != tc.wantSuccess {
				t.Errorf("Route.Success = %v; want %v. Route=%+v", route.Success, tc.wantSuccess, route)
			}
			if tc.wantReachesDest && len(route.Waypoints) > 0 {
				last := route.Waypoints[len(route.Waypoints)-1]
				if last.X() != dstX || last.Z() != dstZ {
					t.Errorf("last waypoint = (%d, %d); want (%d, %d) [BlockerIgnored expects passage]", last.X(), last.Z(), dstX, dstZ)
				}
			}
		})
	}
}
```

Add the import line if not already present:

```go
import (
	"testing"

	"github.com/zsrv/goscape/pkg/pathfinder/collision"
)
```

- [ ] **Step 2.5: Run the H2 subtests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNAI94_RouteBlockerFlag_Consulted' -v ./pkg/pathfinder/routefinder/
```

Expected: at least one subtest fails. Record exact output. The expected diagnostic finding is that BOTH subtests behave identically (route always passes through OR route always blocks) — proving H2 — but record the actual behavior, don't assume.

- [ ] **Step 2.6: Disposition: skip with pin**

If the test reveals H2 (both subtests behave identically), wrap each subtest body inside the `t.Run` closure with `t.Skip("NAI-94: H2 reproducer — useRouteBlockerFlags appears unconsulted; observed behavior: <fill>")` and keep the structural assertions for NAI-95 to lift.

If H2 is eliminated (the two subtests differentiate), keep the test as a passing pin and document at the top of the test the verified behavior. This is unexpected per the audit but possible.

- [ ] **Step 2.7: Run final state**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run 'TestNAI94_RouteBlockerFlag_Consulted' -v ./pkg/pathfinder/routefinder/
```

Expected: green (passing or skipping cleanly).

- [ ] **Step 2.8: Commit**

```bash
git add pkg/pathfinder/routefinder/nai94_repro_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(routefinder): NAI-94 T2 — H2 useRouteBlockerFlags reproducer

Probes whether the field at routefinder.go:43 (// TODO: unused) is
consulted by FindRoute. Subtests differentiate on the field value;
identical behavior confirms H2.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: H3 — Survival Expert blocked-passage reproducer

**Hypothesis:** Ring buffer wrap (`routefinderDefaultRingBufferSize=4096`) or `maxWaypoints=25` truncation halts BFS before destination is reached. **Or** the cabin-wall flags simply leave no traversable route, in which case "gets within 6 tiles" is the *correct* moveNear closest-approach result and the bug is elsewhere.

**Files:**
- Modify: `pkg/pathfinder/routefinder/nai94_repro_test.go`

- [ ] **Step 3.1: Append the Survival Expert reproducer**

```go
// TestNAI94_SurvivalExpert_BlockedPassage is the H3 reproducer. Smoke shape
// from 2026-05-05 NAI-92 run: player at (3101, 3103) → NPC typeId=943 at
// (3103, 3095). Cheb=8. Real-game observation: player gets within ~6 tiles,
// no closer.
//
// This unit test uses an EMPTY FlagMap to isolate the question: does the
// pathfinder return a clean reaching path when there's no obstacle? If yes,
// the "gets within 6 tiles" symptom is downstream of FlagMap state (real
// cabin wall flags); if no, the truncation/algo issue reproduces even
// without walls. A second subtest plants a synthetic minimal cabin-wall to
// see whether moveNear closest-approach matches the in-game ~6-tile result.
//
// Real-mapsquare m48_50 fixture loading is OUT of scope for this plan
// (would drag world wiring into a unit test). NAI-95 may revisit this.
func TestNAI94_SurvivalExpert_BlockedPassage(t *testing.T) {
	const (
		level = 0
		srcX  = 3101
		srcZ  = 3103
		dstX  = 3103
		dstZ  = 3095
	)

	t.Run("EmptyFlagMap_MustReach", func(t *testing.T) {
		pf := NewPathFinderAPI()

		route := pf.FindPathPlain(level, srcX, srcZ, dstX, dstZ)

		if !route.Success {
			t.Fatalf("Route.Success=false on empty FlagMap; cheb=8 unobstructed must succeed. Route=%+v", route)
		}
		if len(route.Waypoints) == 0 {
			t.Fatalf("Route.Waypoints empty on empty FlagMap")
		}
		last := route.Waypoints[len(route.Waypoints)-1]
		if last.X() != dstX || last.Z() != dstZ {
			t.Fatalf("last waypoint = (%d, %d); want (%d, %d). Full waypoints: %+v",
				last.X(), last.Z(), dstX, dstZ, route.Waypoints)
		}
	})

	t.Run("SyntheticCabinWall_MoveNearReports", func(t *testing.T) {
		pf := NewPathFinderAPI()

		// Synthetic horizontal wall at z=3099 spanning x=[3100..3105], blocking
		// north→south traversal. Player at z=3103 must detour around it. With
		// no detour available within search bounds, moveNear=true should yield
		// closest-approach. This is NOT the real m48_50 layout — it's a
		// minimal repro shape. Document divergence at the test site.
		level0 := 0
		for x := 3100; x <= 3105; x++ {
			pf.Flags.Add(x, 3099, level0, collision.FlagWallNorth)
			pf.Flags.Add(x, 3100, level0, collision.FlagWallSouth)
		}

		route := pf.FindPathPlain(level, srcX, srcZ, dstX, dstZ)

		// Just record the result — no assertion. The diagnosis report
		// captures the observed behavior. (Use t.Logf so -v shows it.)
		t.Logf("synthetic-wall route: Success=%v Alternative=%v len(Waypoints)=%d",
			route.Success, route.Alternative, len(route.Waypoints))
		if len(route.Waypoints) > 0 {
			last := route.Waypoints[len(route.Waypoints)-1]
			t.Logf("last waypoint: (%d, %d)", last.X(), last.Z())
		}
	})
}
```

- [ ] **Step 3.2: Run the test**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestNAI94_SurvivalExpert_BlockedPassage -v ./pkg/pathfinder/routefinder/
```

Record output of both subtests verbatim — the `EmptyFlagMap_MustReach` PASS/FAIL is signal, and the `SyntheticCabinWall_MoveNearReports` t.Logf lines are diagnostic input.

- [ ] **Step 3.3: Disposition**

If `EmptyFlagMap_MustReach` FAILED: same skip-with-pin treatment as Task 1 Step 1.3 (wrap with `t.Skip("NAI-94: H3 — empty-flagmap cheb=8 path failed; observed: <pin>")`).

If `EmptyFlagMap_MustReach` PASSED: leave it. The cheb=8 unobstructed case works — H3's truncation hypothesis is eliminated for short distances. Capture for diagnosis.

The `SyntheticCabinWall_MoveNearReports` subtest stays as a `t.Logf`-only diagnostic; no assertion, never fails.

- [ ] **Step 3.4: Run final state**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run TestNAI94_SurvivalExpert_BlockedPassage -v ./pkg/pathfinder/routefinder/
```

Expected: green.

- [ ] **Step 3.5: Commit**

```bash
git add pkg/pathfinder/routefinder/nai94_repro_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(routefinder): NAI-94 T3 — H3 Survival Expert reproducer

EmptyFlagMap subtest pins cheb=8 unobstructed reach; synthetic-wall
subtest captures moveNear closest-approach behavior diagnostically.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: H4 — systematic AS↔goscape `findPath1` diff

**Hypothesis:** Behavioral divergence in goscape's BFS step expansion vs the AS reference. Tasks 1-3 may already root-cause; if so, this task narrows to confirming the smoking-gun area. If they don't, this is the systematic backstop.

**Files:**
- (Read-only) `pkg/pathfinder/routefinder/routefinder.go`
- (Read-only) `/home/owner/Code/github.com/2004scape/rsmod-pathfinder/src/rsmod/PathFinder.ts`
- Output recorded inline as audit notes; final form lands in Task 6's diagnosis report.

- [ ] **Step 4.1: Read AS findPath1 in full**

```bash
sed -n '135,266p' /home/owner/Code/github.com/2004scape/rsmod-pathfinder/src/rsmod/PathFinder.ts
```

Read the entire `findPath1` body (8-direction step expansion, BFS exit conditions, ring buffer wrap). Note line ranges of: queue pop, 8-direction conditional blocks (W/E/S/N/SW/SE/NW/NE), each branch's `collision.canMove(...)` guard, the early-exit condition.

- [ ] **Step 4.2: Read goscape's equivalent**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go doc -all ./pkg/pathfinder/routefinder/ | head -100
```

Then Read the goscape equivalent function in `routefinder.go` (search for the BFS loop — likely inside `FindRoute` or a private `findPath1`-equivalent). Identify the matching 8-direction blocks.

- [ ] **Step 4.3: Build the diff register**

For each of the 8 direction-step blocks (W/E/S/N/SW/SE/NW/NE), record in a markdown table:
- AS file:line
- Goscape file:line
- Divergence summary (or "match" with a 1-line confirmation)

Pay particular attention to:
- The collision-flag mask used (`BLOCK_WEST` vs `FlagWallWest` etc.)
- The early-exit condition after appendDirection
- Off-by-one bounds (`< relativeSearchSize` vs `<= relativeSearchSize`)
- Whether route-blocker flags are masked into / out of the canMove check (links to H2)

- [ ] **Step 4.4: Spot-check Rust v5.0.4 if AS↔goscape look identical**

If the H4 diff register shows AS and goscape match on every direction-step block AND H1/H2/H3 didn't smoke-gun, run:

```bash
cd /home/owner/Code/github.com/2004scape/rsmod-pathfinder && git stash --include-untracked && git checkout 8dd111e
```

(`8dd111e` = `chore: release for 5.0.4`.) Read the Rust pathfinder source under `src/` (post-Rust-rewrite layout). Spot-check whether the Rust version of `find_path1` introduces any branch missing from AS — that's the AS-line-of-history blind spot.

After spot-check:

```bash
cd /home/owner/Code/github.com/2004scape/rsmod-pathfinder && git checkout - && git stash pop
```

Record cwd return — the original shell pwd should be `/home/owner/Code/github.com/zsrv/goscape`.

- [ ] **Step 4.5: Commit nothing (read-only audit)**

This task produces no file changes. Hold the diff register notes for inclusion in Task 6's diagnosis report.

---

## Task 5: H5 — closest-approach / moveNear audit (conditional)

**Hypothesis:** When the destination is unreachable (real cabin wall), `moveNear=true` triggers a closest-approach selection. If goscape's closest-approach picks a tile farther from the destination than the AS reference would, that explains "player gets within 6 tiles, no closer."

**Trigger:** Run this task only if Tasks 1-4 do not surface a smoking gun for the partial-path symptom. If Task 1 reveals "no path returned at all" as the root pattern, H5 is moot — move directly to Task 6.

**Files:**
- (Read-only) `pkg/pathfinder/routefinder/routefinder.go` (search for "moveNear", "closest", "alternative")
- (Read-only) `/home/owner/Code/github.com/2004scape/rsmod-pathfinder/src/rsmod/PathFinder.ts` lines ~535-695 (`findClosestApproachPoint` analog)

- [ ] **Step 5.1: Locate goscape's closest-approach analog**

```bash
rg -n "moveNear|closestApproach|Alternative|alternativeRoute" pkg/pathfinder/routefinder/
```

Read each match. Identify which function is equivalent to AS's `findClosestApproachPoint`.

- [ ] **Step 5.2: Read AS reference**

```bash
sed -n '535,695p' /home/owner/Code/github.com/2004scape/rsmod-pathfinder/src/rsmod/PathFinder.ts
```

- [ ] **Step 5.3: Diff: cost function + selection criteria**

Build a 2-column table comparing AS vs goscape on:
- Cost function for ranking candidate closest-approach tiles
- Selection ordering (does AS prefer tiles by `dist²` while goscape uses `cheb`? etc.)
- Tie-breaking
- Bound constants (`MaxAlternativeRouteLowestCost = 1000`, `MaxAlternativeRouteSeekRange = 100`, `MaxAlternativeRouteDistanceFromDestination = 10` — all present in goscape's `routefinder.go:17-19`)

Record the divergence register for the diagnosis report.

- [ ] **Step 5.4: No commit (read-only audit)**

Hold notes for Task 6.

---

## Task 6: Compile diagnosis report

**Files:**
- Create: `docs/superpowers/investigations/2026-05-05-nai-94-diagnosis.md`

- [ ] **Step 6.1: Verify the directory exists**

```bash
ls docs/superpowers/ 2>/dev/null
mkdir -p docs/superpowers/investigations 2>/dev/null
ls docs/superpowers/investigations/ 2>/dev/null
```

- [ ] **Step 6.2: Write the diagnosis report**

Create `docs/superpowers/investigations/2026-05-05-nai-94-diagnosis.md` with the following structure (fill from the audit findings collected in Tasks 1-5; do NOT leave any section empty — write "ELIMINATED — see §X for evidence" or "UNDETERMINED — diagnosis ceiling" if the audit didn't produce a verdict):

```markdown
# NAI-94 — Pathfinder Reach Stage 1 Diagnosis

**Spec:** `docs/superpowers/specs/2026-05-05-nai-94-pathfinder-reach-investigation-design.md`
**Plan:** `docs/superpowers/plans/2026-05-05-nai-94-pathfinder-reach-investigation.md`
**Audit date:** 2026-05-05

## Summary

[Single paragraph: which hypothesis fired (if any), root cause if identified, OR diagnosis ceiling.]

## Reproducer test results

| Test | Result | Disposition |
|---|---|---|
| TestNAI94_HansCheb2_StraightLineMustReach | [PASS / FAIL+pinned] | [skipped or passing] |
| TestNAI94_RouteBlockerFlag_Consulted/BlockerHonored_RefusesToCross | [PASS / FAIL+pinned] | [skipped or passing] |
| TestNAI94_RouteBlockerFlag_Consulted/BlockerIgnored_PassesThrough | [PASS / FAIL+pinned] | [skipped or passing] |
| TestNAI94_SurvivalExpert_BlockedPassage/EmptyFlagMap_MustReach | [PASS / FAIL+pinned] | [skipped or passing] |
| TestNAI94_SurvivalExpert_BlockedPassage/SyntheticCabinWall_MoveNearReports | (diagnostic) | [logged values: ...] |

## Per-hypothesis verdicts

### H1 — BFS / waypoint return broken at minimum-distance paths

**Verdict:** [CONFIRMED / ELIMINATED / PARTIAL / UNDETERMINED]

**Evidence:**
- [file:line citation] [observed behavior]
- ...

### H2 — `useRouteBlockerFlags` declared but unconsulted

**Verdict:** [CONFIRMED / ELIMINATED / PARTIAL / UNDETERMINED]

**Diff register (from Task 2 Step 2.3):**

| AS site | Goscape site | Status |
|---|---|---|
| ... | ... | match / divergent / missing |

**Evidence:**
- ...

### H3 — Ring buffer wrap or maxWaypoints truncation

**Verdict:** [CONFIRMED / ELIMINATED / PARTIAL / UNDETERMINED]

**Evidence:** [empty-flagmap cheb=8 result; synthetic-wall log lines]

### H4 — `findPath1` step-expansion divergence

**Verdict:** [CONFIRMED / ELIMINATED / PARTIAL / UNDETERMINED]

**Diff register (from Task 4 Step 4.3):**

| Direction block | AS line | Goscape line | Divergence |
|---|---|---|---|
| W | ... | ... | ... |
| E | ... | ... | ... |
| S | ... | ... | ... |
| N | ... | ... | ... |
| SW | ... | ... | ... |
| SE | ... | ... | ... |
| NW | ... | ... | ... |
| NE | ... | ... | ... |

**AS↔Rust spot-check:** [if performed in Step 4.4: result; otherwise "not performed (H1-H3 surfaced root cause)"]

### H5 — Closest-approach / moveNear divergence

**Verdict:** [CONFIRMED / ELIMINATED / PARTIAL / UNDETERMINED / NOT-RUN]

**Diff register (from Task 5 Step 5.3):**

| Aspect | AS behavior | Goscape behavior | Divergent? |
|---|---|---|---|
| Cost function | ... | ... | ... |
| Selection ordering | ... | ... | ... |
| Tie-breaking | ... | ... | ... |
| Bound constants | ... | ... | ... |

## Root cause

[Single paragraph naming the bug with file:line evidence, OR "Diagnosis ceiling: NAI-95 needs <X> to break through. Specifically: ..."]

## Stage 2 (NAI-95) handoff

- **Root cause:** [file:line + 1-2 sentence summary]
- **Repro tests to lift skip on:** [list of t.Skip-wrapped tests + expected post-fix behavior]
- **Files NAI-95 will touch:** [exact list]
- **Estimated LOC for fix:** [ballpark]
- **Residual hypotheses for NAI-96+:** [any H4 divergences not in NAI-95 scope; any AS↔Rust deltas worth a separate audit]
```

- [ ] **Step 6.3: Verify report has no template placeholders**

```bash
rg -n "\[CONFIRMED / ELIMINATED|\[file:line|\[exact list|\[ballpark|\[empty-flagmap|\.\.\." docs/superpowers/investigations/2026-05-05-nai-94-diagnosis.md
```

Expected: zero matches against the literal template tokens (square-bracket placeholders, `...`). Every `[...]` cell in the tables must be filled with real evidence.

If matches surface, fix inline.

- [ ] **Step 6.4: Commit**

```bash
git add docs/superpowers/investigations/2026-05-05-nai-94-diagnosis.md
git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(investigation): NAI-94 diagnosis report

Per-hypothesis verdicts H1-H5; reproducer test results table;
root cause / diagnosis ceiling; Stage 2 (NAI-95) handoff.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Followups update + close commit

**Files:**
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`

- [ ] **Step 7.1: Append "From NAI-94" section to followups**

Read the current tail of `nai_followups.md`:

```bash
tail -40 /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md
```

Append (using Edit on the file's end-of-file marker) a new section:

```markdown

---

## From NAI-94

**Why:** NAI-94 was a Stage 1 audit of the pathfinder-reach residual NAI-92 surfaced. Stage 2 fix is NAI-95.
**How to apply:** When opening NAI-95, read `docs/superpowers/investigations/2026-05-05-nai-94-diagnosis.md` first; the Stage 2 handoff section there is the spec input.

- **Root cause / diagnosis ceiling (verbatim from diagnosis report §"Root cause"):** [paste]
- **Reproducer tests awaiting lift:** [paste from §"Stage 2 handoff"]
- **Files NAI-95 will touch:** [paste]
- **Residuals for NAI-96+:** [paste]
```

Replace each `[paste]` with the actual content from `docs/superpowers/investigations/2026-05-05-nai-94-diagnosis.md`.

- [ ] **Step 7.2: Verify the followups entry has no `[paste]` placeholders left**

```bash
rg -n "\[paste\]" /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md
```

Expected: zero matches.

- [ ] **Step 7.3: Pre-close verification — full pathfinder test suite + repository smoke**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/pathfinder/routefinder/... -v
```

Expected: all tests PASS (including the new NAI-94 tests, whether passing or skipping). No FAIL.

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: green at the repo level (no broken tests; baseline preserved).

- [ ] **Step 7.4: Verify `git status` clean of stray artifacts (per `feedback_subagent_wt_path`)**

```bash
git status
```

Expected: only intended changes. The unstaged `.bash_profile`, `.bashrc`, etc. shown at session start are pre-existing untracked dotfiles — NOT NAI-94 artifacts; leave untouched.

- [ ] **Step 7.5: Close commit**

```bash
git add docs/superpowers/specs/2026-05-05-nai-94-pathfinder-reach-investigation-design.md \
        docs/superpowers/plans/2026-05-05-nai-94-pathfinder-reach-investigation.md \
        docs/superpowers/investigations/2026-05-05-nai-94-diagnosis.md \
        pkg/pathfinder/routefinder/nai94_repro_test.go
# memory file lives outside the repo; commit only the in-repo changes.

git commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
chore(close): NAI-94 — pathfinder reach Stage 1 investigation

Stage 1 audit complete. Hypotheses H1-H5 verdicted in diagnosis
report; reproducer tests pinned; Stage 2 (NAI-95) handoff in
nai_followups.md.

[Add 1-2 line summary of root cause / diagnosis ceiling here.]

Closes memory: <name-of-new-followup-entry-if-any>

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Replace the bracketed summary line with the actual root cause / ceiling result. Replace `<name-of-new-followup-entry-if-any>` with the followups-section anchor name (or omit the trailer if no new memory entry was created — only an updated one).

- [ ] **Step 7.6: Verify clean close**

```bash
git log -1 --stat
git status
```

Expected: `git status` clean; latest commit message contains the summary; the close commit's `--stat` lists only the docs files (no production code).

---

## Self-Review Checklist (controller, before task dispatch)

- [ ] **Spec coverage:** every §1-§9 spec section maps to a task above. (§1 motivation → Task 1 + §"Goal"; §2 in/out scope → all tasks honor "no production change"; §3 H1-H5 → Tasks 1, 2, 3, 4, 5; §4 reproducer matrix → Tasks 1, 2, 3; §5 methodology hybrid probe-then-diff → Task ordering; §6 deliverables → Tasks 1, 2, 3, 6, 7; §7 exit criteria → Task 7 Step 7.3 + Task 6 §"Stage 2 handoff"; §8 risks honored in Task 4 Step 4.4 + Task 3 Step 3.1's "real-mapsquare fixture out of scope" note + Task 1 Step 1.3's audit-fabrication-guard skip-with-pin; §9 cadence references → Task 7 close-commit format.)
- [ ] **No placeholders:** every step has the actual command / code. Template `[paste]` and `[fill]` markers in Tasks 6 and 7 are intentional fill-in-from-audit-output sites, gated by explicit "expected: zero matches" verification steps (6.3, 7.2).
- [ ] **Type consistency:** test names (`TestNAI94_HansCheb2_StraightLineMustReach`, etc.) referenced consistently in Tasks 1, 6, 7. Field names (`Route.Success`, `Route.Waypoints`, `RouteCoordinates.X()`) match the actual goscape API verified at plan-author time (`pkg/pathfinder/routefinder/route.go:1-7`, `routecoordinates.go:9-19`). `NewRouteFinder` 4-arg signature matches `routefinder.go:39` checked at plan-author time.
- [ ] **Conditional task gating:** Task 5 explicitly gated on Tasks 1-4 outcome; Task 4 Step 4.4 spot-check explicitly gated on AS↔goscape "identical" finding. Each gate has an explicit if/then in the task header.
- [ ] **Audit-fabrication guard wired:** Task 4 and Task 5 are read-only / no-commit; their outputs feed Task 6, where the controller (per `audit_subagent_fabrication`, `verify_implementer_claims`) verifies every file:line citation independently before merging into the diagnosis report.

---
