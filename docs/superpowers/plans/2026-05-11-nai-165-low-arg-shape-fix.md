# NAI-165 — `isLineOfWalk` wrapper + `handleLineOfWalk` arg-shape fix — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Widen the `isLineOfWalk` wrapper and the `handleLineOfWalk` direct call to pass `(1, 1, 1, 0)` to `HasLineOfWalk` — matching the TS canonical wrapper at `GameMap.ts:425-427` and symmetric to NAI-163-D-LOS-ARG-SHAPE-FIX.

**Architecture:** Two single-line production fixes at `pkg/script/handlers_map.go:175` and `:423`. Test infrastructure: extend the existing `stubLineValidatorArgs` fixture (built for NAI-163 B1 T0/T1) to record LineOfWalk calls in parallel with LineOfSight, then add two pins mirroring `TestIsLineOfSightWrapper_PassesTSFaithfulArgShape` and `TestHandleLineOfSight_ArgShape`. Doc-comments on both call sites are updated to cite the new deviation tag. Single TDD task, single commit.

**Tech Stack:** Go 1.26+ (`go_version.md`).

**Spec:** `docs/superpowers/specs/2026-05-11-nai-165-low-arg-shape-fix-design.md` (commit `74c7431`).

**HEAD at plan-write:** `74c7431`.

---

## File Map

**Modify:**
- `pkg/script/handlers_map.go` — flip two `HasLineOfWalk` arg shapes (lines 175 + 423); update doc-comments at lines 166-170 and 389-398.
- `pkg/script/handlers_map_test.go` — extend `stubLineValidatorArgs` (lines 971-988) to record LineOfWalk calls; add two new test functions adjacent to existing NAI-163 B1 T0/T1 pins.

**No new files.**

---

## Task 1: Extend `stubLineValidatorArgs` to record HasLineOfWalk calls

**Files:**
- Modify: `pkg/script/handlers_map_test.go:971-988`

Rationale: The existing fixture records `HasLineOfSight` calls into `losCalls []losCall` but `HasLineOfWalk` is a no-recording stub that returns `true`. To pin the LOW arg shape we need parallel recording.

- [ ] **Step 1.1: Read current fixture and confirm shape**

Run: `sed -n '965,990p' pkg/script/handlers_map_test.go`

Expected output includes:
```go
type stubLineValidatorArgs struct {
    losCalls  []losCall
    losReturn bool
}

type losCall struct {
    level, srcX, srcZ, destX, destZ          int
    srcSize, destWidth, destLength, extraFlag int
}

func (st *stubLineValidatorArgs) HasLineOfSight(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool {
    st.losCalls = append(st.losCalls, losCall{level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag})
    return st.losReturn
}

func (st *stubLineValidatorArgs) HasLineOfWalk(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool {
    return true
}
```

- [ ] **Step 1.2: Edit the fixture to add `lowCalls` slice + `lowReturn` field + record in `HasLineOfWalk`**

Use Edit on `pkg/script/handlers_map_test.go`.

Replace the struct + both method bodies with:

```go
// stubLineValidatorArgs records every (Has)LineOfSight and (Has)LineOfWalk
// call's full arg tuple for the NAI-163-D-LOS-ARG-SHAPE-FIX and
// NAI-165-D-LOW-ARG-SHAPE-FIX regressions. Distinct from the existing
// npc_iterator_test.go recordingLineValidator (which only captures level +
// src/dest, not srcSize/destWidth/destLength/extraFlag).
type stubLineValidatorArgs struct {
	losCalls  []losCall
	losReturn bool
	lowCalls  []losCall
	lowReturn bool
}

type losCall struct {
	level, srcX, srcZ, destX, destZ           int
	srcSize, destWidth, destLength, extraFlag int
}

func (st *stubLineValidatorArgs) HasLineOfSight(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool {
	st.losCalls = append(st.losCalls, losCall{level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag})
	return st.losReturn
}

func (st *stubLineValidatorArgs) HasLineOfWalk(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool {
	st.lowCalls = append(st.lowCalls, losCall{level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag})
	return st.lowReturn
}
```

The `losCall` struct is reused for both opcodes (identical 9-int tuple shape). Field name `lowCalls` (lowercase "low") mirrors `losCalls`; `lowReturn` defaults to false if unset.

- [ ] **Step 1.3: Build to confirm fixture change compiles cleanly**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/script/...`

Expected: no errors. No tests run yet.

---

## Task 2: Add the two RED arg-shape pins (LOW wrapper + LOW opcode end-to-end)

**Files:**
- Modify: `pkg/script/handlers_map_test.go` — append two new test functions immediately after `TestHandleLineOfSight_ArgShape` (currently ends at line 1129; new tests insert before the `// MAP_MULTIWAY` section at line 1131).

- [ ] **Step 2.1: Insert two new test functions**

Use Edit on `pkg/script/handlers_map_test.go`. Locate the end of `TestHandleLineOfSight_ArgShape` (closing `}` at line 1129) — the next line currently is a blank line followed by `// MAP_MULTIWAY (opcode 1014) — NAI-120 Bundle 2A.`. Insert the two new tests in that gap.

Edit:
- `old_string`:

```go
func TestHandleLineOfSight_ArgShape(t *testing.T) {
	// Pins the TS-faithful arg tuple passed to HasLineOfSight by handleLineOfSight
	// via the isLineOfSight wrapper. NAI-163-D-LOS-ARG-SHAPE-FIX.
	st := &stubLineValidatorArgs{losReturn: true}
	s := newTestState(minimalScript(OpReturn))
	mw := newMockWorld()
	mw.mapMembers = 1
	s.World = mw
	s.LineValidator = st
	s.PushInt(coordgrid.PackCoord(0, 3200, 3300)) // from (c1)
	s.PushInt(coordgrid.PackCoord(0, 3210, 3305)) // to (c2)
	if err := handleLineOfSight(s); err != nil {
		t.Fatalf("handleLineOfSight: %v", err)
	}
	_ = s.PopInt()
	if len(st.losCalls) != 1 {
		t.Fatalf("expected 1 LV call, got %d", len(st.losCalls))
	}
	got := st.losCalls[0]
	want := losCall{level: 0, srcX: 3200, srcZ: 3300, destX: 3210, destZ: 3305, srcSize: 1, destWidth: 1, destLength: 1, extraFlag: 0}
	if got != want {
		t.Fatalf("handleLineOfSight arg shape mismatch:\n got=%+v\nwant=%+v", got, want)
	}
}

// MAP_MULTIWAY (opcode 1014) — NAI-120 Bundle 2A.
```

- `new_string`:

```go
func TestHandleLineOfSight_ArgShape(t *testing.T) {
	// Pins the TS-faithful arg tuple passed to HasLineOfSight by handleLineOfSight
	// via the isLineOfSight wrapper. NAI-163-D-LOS-ARG-SHAPE-FIX.
	st := &stubLineValidatorArgs{losReturn: true}
	s := newTestState(minimalScript(OpReturn))
	mw := newMockWorld()
	mw.mapMembers = 1
	s.World = mw
	s.LineValidator = st
	s.PushInt(coordgrid.PackCoord(0, 3200, 3300)) // from (c1)
	s.PushInt(coordgrid.PackCoord(0, 3210, 3305)) // to (c2)
	if err := handleLineOfSight(s); err != nil {
		t.Fatalf("handleLineOfSight: %v", err)
	}
	_ = s.PopInt()
	if len(st.losCalls) != 1 {
		t.Fatalf("expected 1 LV call, got %d", len(st.losCalls))
	}
	got := st.losCalls[0]
	want := losCall{level: 0, srcX: 3200, srcZ: 3300, destX: 3210, destZ: 3305, srcSize: 1, destWidth: 1, destLength: 1, extraFlag: 0}
	if got != want {
		t.Fatalf("handleLineOfSight arg shape mismatch:\n got=%+v\nwant=%+v", got, want)
	}
}

// --- NAI-165: isLineOfWalk wrapper + handleLineOfWalk arg-shape regression ---
// NAI-165-D-LOW-ARG-SHAPE-FIX: widens isLineOfWalk from (1, 0, 0, 0) to
// (1, 1, 1, 0) to match TS GameMap.ts:425-427. Symmetric mirror of
// NAI-163-D-LOS-ARG-SHAPE-FIX.

func TestIsLineOfWalkWrapper_PassesTSFaithfulArgShape(t *testing.T) {
	// Regression pin: TS GameMap.ts:426 calls
	//   rsmod.hasLineOfWalk(level, sX, sZ, dX, dZ, 1, 1, 1, 1, 0)
	// goscape's srcSize expands to srcWidth=srcLength=1 inside RayCast
	// (linevalidator.go:21), so the TS-faithful arg tuple at the wrapper
	// level is srcSize=1, destWidth=1, destLength=1, extraFlag=0.
	// Pre-NAI-165-D-LOW-ARG-SHAPE-FIX the wrapper was (1, 0, 0, 0).
	st := &stubLineValidatorArgs{lowReturn: true}
	s := &ScriptState{LineValidator: st}
	_ = isLineOfWalk(s, 0, 3200, 3300, 3210, 3305)
	if len(st.lowCalls) != 1 {
		t.Fatalf("expected 1 LineValidator call, got %d", len(st.lowCalls))
	}
	got := st.lowCalls[0]
	want := losCall{level: 0, srcX: 3200, srcZ: 3300, destX: 3210, destZ: 3305, srcSize: 1, destWidth: 1, destLength: 1, extraFlag: 0}
	if got != want {
		t.Fatalf("isLineOfWalk arg shape mismatch:\n got=%+v\nwant=%+v", got, want)
	}
}

func TestHandleLineOfWalk_ArgShape(t *testing.T) {
	// Pins the TS-faithful arg tuple passed to HasLineOfWalk by handleLineOfWalk
	// at the opcode 1006 dispatch site (direct call, NOT via the wrapper —
	// see handlers_map.go:423). NAI-165-D-LOW-ARG-SHAPE-FIX.
	st := &stubLineValidatorArgs{lowReturn: true}
	s := newTestState(minimalScript(OpReturn))
	mw := newMockWorld()
	mw.mapMembers = 1
	s.World = mw
	s.LineValidator = st
	s.PushInt(coordgrid.PackCoord(0, 3200, 3300)) // from (c1)
	s.PushInt(coordgrid.PackCoord(0, 3210, 3305)) // to (c2)
	if err := handleLineOfWalk(s); err != nil {
		t.Fatalf("handleLineOfWalk: %v", err)
	}
	_ = s.PopInt()
	if len(st.lowCalls) != 1 {
		t.Fatalf("expected 1 LV call, got %d", len(st.lowCalls))
	}
	got := st.lowCalls[0]
	want := losCall{level: 0, srcX: 3200, srcZ: 3300, destX: 3210, destZ: 3305, srcSize: 1, destWidth: 1, destLength: 1, extraFlag: 0}
	if got != want {
		t.Fatalf("handleLineOfWalk arg shape mismatch:\n got=%+v\nwant=%+v", got, want)
	}
}

// MAP_MULTIWAY (opcode 1014) — NAI-120 Bundle 2A.
```

- [ ] **Step 2.2: Run the two new tests — confirm they FAIL with current production code**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestIsLineOfWalkWrapper_PassesTSFaithfulArgShape|TestHandleLineOfWalk_ArgShape' -v
```

Expected: BOTH tests FAIL with arg-shape mismatch. The diff should report:
- `got` has `destWidth: 0, destLength: 0`
- `want` has `destWidth: 1, destLength: 1`

This confirms RED — the production code at `handlers_map.go:175` and `:423` is still on the pre-fix `(1, 0, 0, 0)` shape.

If a test passes here, STOP — the fixture change in Task 1 may have a bug. Investigate before continuing.

---

## Task 3: Flip the two production sites to `(1, 1, 1, 0)`

**Files:**
- Modify: `pkg/script/handlers_map.go:175` (wrapper)
- Modify: `pkg/script/handlers_map.go:423` (handleLineOfWalk direct call)
- Modify: doc-comments at `pkg/script/handlers_map.go:166-170` and `:389-398`

- [ ] **Step 3.1: Update the `isLineOfWalk` wrapper + its doc-comment**

Use Edit on `pkg/script/handlers_map.go`.

- `old_string`:

```go
// isLineOfWalk delegates to s.LineValidator. Pessimistic-allow on nil
// validator (matches NpcIterator passesFilter HuntAll behavior). Calls
// HasLineOfWalk with src=(srcX,srcZ), dest=(destX,destZ); the goscape
// convention uses srcSize=1, destWidth=0, destLength=0, extraFlag=0,
// matching player_iterator.go and npc_iterator.go. NAI-35-T6.
func isLineOfWalk(s *ScriptState, level, srcX, srcZ, destX, destZ int) bool {
	if s.LineValidator == nil {
		return true
	}
	return s.LineValidator.HasLineOfWalk(level, srcX, srcZ, destX, destZ, 1, 0, 0, 0)
}
```

- `new_string`:

```go
// isLineOfWalk delegates to s.LineValidator. Mirrors TS
// GameMap.ts:425-427: rsmod.hasLineOfWalk(level, sX, sZ, dX, dZ, 1, 1, 1, 1, 0).
// goscape's srcSize collapses TS srcWidth+srcHeight (both 1) into a single
// arg via RayCast's `srcSize, srcSize` (linevalidator.go:21); destWidth and
// destLength are passed verbatim. NAI-165-D-LOW-ARG-SHAPE-FIX widens this
// wrapper from the pre-fix (1, 0, 0, 0) shape to TS-faithful (1, 1, 1, 0);
// existing MapFindSquareLineOfWalk callers at lines 117, 147 inherit the
// corrected endpoint semantics. Pessimistic-allow on nil validator.
// NAI-35-T6 (NAI-165). The iterator/hunt-site sweep at player_iterator.go,
// npc_iterator.go, npc_hunt_entities.go, and npc_hunt.go (still on
// (1, 0, 0, 0)) is tracked separately as a NAI-166 candidate.
func isLineOfWalk(s *ScriptState, level, srcX, srcZ, destX, destZ int) bool {
	if s.LineValidator == nil {
		return true
	}
	return s.LineValidator.HasLineOfWalk(level, srcX, srcZ, destX, destZ, 1, 1, 1, 0)
}
```

- [ ] **Step 3.2: Update the `handleLineOfWalk` direct call + its doc-comment**

Use Edit on `pkg/script/handlers_map.go`.

- `old_string`:

```go
// handleLineOfWalk (LINEOFWALK, opcode 1006) reports whether a 1-tile
// entity at c1 has line-of-walk to c2. Pop order: top-of-stack is c2,
// c1 below. Pushes 1 on success, 0 on fail.
//
// Same-level guard: differing levels push 0 immediately.
// F2P short-circuit: in a non-members world, destination tile not in
// an F2P zone pushes 0.
// Nil-LineValidator: pushes 0 (fail closed) when no validator wired.
//
// Mirrors TS ServerOps.ts:65-82.
func handleLineOfWalk(s *ScriptState) error {
	c2 := s.PopInt()
	c1 := s.PopInt()

	fromLevel, fromX, fromZ, err := checkCoord(c1, "LINEOFWALK")
	if err != nil {
		return err
	}
	toLevel, toX, toZ, err := checkCoord(c2, "LINEOFWALK")
	if err != nil {
		return err
	}
	if fromLevel != toLevel {
		s.PushInt(0)
		return nil
	}
	if s.World.MapMembers() == 0 && !s.World.IsFreeToPlay(toX, toZ) {
		s.PushInt(0)
		return nil
	}
	if s.LineValidator == nil {
		s.PushInt(0)
		return nil
	}
	if s.LineValidator.HasLineOfWalk(fromLevel, fromX, fromZ, toX, toZ, 1, 0, 0, 0) {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	return nil
}
```

- `new_string`:

```go
// handleLineOfWalk (LINEOFWALK, opcode 1006) reports whether a 1-tile
// entity at c1 has line-of-walk to c2. Pop order: top-of-stack is c2,
// c1 below. Pushes 1 on success, 0 on fail.
//
// Same-level guard: differing levels push 0 immediately.
// F2P short-circuit: in a non-members world, destination tile not in
// an F2P zone pushes 0.
// Nil-LineValidator: pushes 0 (fail closed) when no validator wired
// (goscape defensive; TS routes through isLineOfWalk wrapper which is
// pessimistic-ALLOW on nil — pre-existing asymmetry vs handleLineOfSight
// at line 230, tracked separately as a NAI-166 candidate).
//
// Arg shape: HasLineOfWalk(..., 1, 1, 1, 0) per NAI-165-D-LOW-ARG-SHAPE-FIX;
// matches the isLineOfWalk wrapper at line 171 and TS GameMap.ts:425-427.
//
// Mirrors TS ServerOps.ts:65-82.
func handleLineOfWalk(s *ScriptState) error {
	c2 := s.PopInt()
	c1 := s.PopInt()

	fromLevel, fromX, fromZ, err := checkCoord(c1, "LINEOFWALK")
	if err != nil {
		return err
	}
	toLevel, toX, toZ, err := checkCoord(c2, "LINEOFWALK")
	if err != nil {
		return err
	}
	if fromLevel != toLevel {
		s.PushInt(0)
		return nil
	}
	if s.World.MapMembers() == 0 && !s.World.IsFreeToPlay(toX, toZ) {
		s.PushInt(0)
		return nil
	}
	if s.LineValidator == nil {
		s.PushInt(0)
		return nil
	}
	if s.LineValidator.HasLineOfWalk(fromLevel, fromX, fromZ, toX, toZ, 1, 1, 1, 0) {
		s.PushInt(1)
	} else {
		s.PushInt(0)
	}
	return nil
}
```

- [ ] **Step 3.3: Run the two new tests — confirm GREEN**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestIsLineOfWalkWrapper_PassesTSFaithfulArgShape|TestHandleLineOfWalk_ArgShape' -v
```

Expected: both PASS.

- [ ] **Step 3.4: Run the full `pkg/script` test suite — confirm no regressions**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
```

Expected: PASS (`ok pkg/script ...`).

Particular focus: the existing NAI-163-B1-T1 LOW handler family (`TestHandleLineOfWalk_*` for level-mismatch / F2P-gate / ray-clear / ray-blocked / nil-LV) must remain green. Those tests use the coarse `stubLineValidator` (different fixture, no arg recording) so they're insensitive to the arg-shape flip.

- [ ] **Step 3.5: Run the full repo test suite for safety**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: all packages PASS. The fix is localized to `pkg/script/handlers_map.go`; `modules/world` callers don't go through these two functions so cross-package impact is nil. If any test fails, investigate before committing.

---

## Task 4: Single combined commit + close

**Files:**
- `pkg/script/handlers_map.go`
- `pkg/script/handlers_map_test.go`

- [ ] **Step 4.1: Stage the two modified files**

Run:
```
git add pkg/script/handlers_map.go pkg/script/handlers_map_test.go
```

- [ ] **Step 4.2: Sanity-check the staged diff**

Run: `git diff --staged --stat`

Expected: 2 files changed, ~50 insertions, ~15 deletions (approximate).

Run: `git diff --staged pkg/script/handlers_map.go | grep -E '^[-+].*HasLineOfWalk'`

Expected output (4 lines — two `-` removals and two `+` additions):
```
-	return s.LineValidator.HasLineOfWalk(level, srcX, srcZ, destX, destZ, 1, 0, 0, 0)
+	return s.LineValidator.HasLineOfWalk(level, srcX, srcZ, destX, destZ, 1, 1, 1, 0)
-	if s.LineValidator.HasLineOfWalk(fromLevel, fromX, fromZ, toX, toZ, 1, 0, 0, 0) {
+	if s.LineValidator.HasLineOfWalk(fromLevel, fromX, fromZ, toX, toZ, 1, 1, 1, 0) {
```

- [ ] **Step 4.3: Commit**

Run:
```
git commit --no-gpg-sign -m "$(cat <<'EOF'
fix(script): NAI-165 — isLineOfWalk + handleLineOfWalk arg-shape (1, 1, 1, 0)

Two production sites flipped from (1, 0, 0, 0) → (1, 1, 1, 0):
- pkg/script/handlers_map.go:175  isLineOfWalk wrapper
                                  (MAP_FINDSQUARE LOW arms inherit)
- pkg/script/handlers_map.go:423  handleLineOfWalk direct call
                                  (LINEOFWALK opcode 1006)

Matches TS canonical isLineOfWalk wrapper at GameMap.ts:425-427:
  rsmod.hasLineOfWalk(level, sX, sZ, dX, dZ, 1, 1, 1, 1, 0)
(goscape collapses TS srcWidth+srcHeight into srcSize=1 via RayCast).

Symmetric mirror of NAI-163-D-LOS-ARG-SHAPE-FIX. Two pins added,
mirroring NAI-163 B1 T0/T1 siblings:
  TestIsLineOfWalkWrapper_PassesTSFaithfulArgShape
  TestHandleLineOfWalk_ArgShape

stubLineValidatorArgs (handlers_map_test.go) extended to record
HasLineOfWalk calls in parallel to HasLineOfSight.

Deviation NAI-165-D-LOW-ARG-SHAPE-FIX (narrative-only; closed in this
commit) tagged at both doc-comment sites.

Out of scope, routed to NAI-166:
- Iterator/hunt-site LOW+LOS sweep (player_iterator, npc_iterator,
  npc_hunt_entities, npc_hunt) — same (1, 0, 0, 0) divergence with
  stale doc-comments claiming "mirrors TS isLineOfWalk wrapper"
- handleLineOfWalk wrapper-routing + pessimistic-deny-on-nil asymmetry
  vs handleLineOfSight wrapper-routing + pessimistic-allow-on-nil

Spec: docs/superpowers/specs/2026-05-11-nai-165-low-arg-shape-fix-design.md
Plan: docs/superpowers/plans/2026-05-11-nai-165-low-arg-shape-fix.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 4.4: Verify the commit landed**

Run: `git log -1 --format='%h %s'`

Expected: a single line beginning with `<short-sha> fix(script): NAI-165 — isLineOfWalk + handleLineOfWalk arg-shape (1, 1, 1, 0)`.

Run: `git status`

Expected: clean working tree (other than pre-existing `M Dockerfile`, `M Makefile`, `M build.sh`, `M config.yaml`, `.dockerignore`, `.claude/` from the session start — those are unrelated and stay untouched).

---

## Task 5: Append NAI-165 entry to `nai_followups.md` and queue NAI-166

**Files:**
- Modify: `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (append at end)

Per `close_commit_memory_trailer.md` and the standing close-protocol pattern visible in NAI-150/154/19 close sections.

- [ ] **Step 5.1: Append a NAI-165 close section to the followups memory**

Use Edit on `nai_followups.md` to insert a new section at end-of-file. The file is 6929 lines; the last section is the NAI-19 DESPAWN close (line 6916+). Append:

```markdown

---

## From NAI-165 (2026-05-11) — `isLineOfWalk` + `handleLineOfWalk` arg-shape fix (CLOSED)

NAI-165 spec at `docs/superpowers/specs/2026-05-11-nai-165-low-arg-shape-fix-design.md` (commit `74c7431`); plan at `docs/superpowers/plans/2026-05-11-nai-165-low-arg-shape-fix.md`. Closed at `<COMMIT-SHA>` (single TDD commit).

Symmetric mirror of NAI-163-D-LOS-ARG-SHAPE-FIX. Two production sites flipped from `(1, 0, 0, 0)` → `(1, 1, 1, 0)`:
- `pkg/script/handlers_map.go:175` — `isLineOfWalk` wrapper (MAP_FINDSQUARE LOW arms inherit)
- `pkg/script/handlers_map.go:423` — `handleLineOfWalk` direct call (LINEOFWALK opcode 1006)

`stubLineValidatorArgs` extended to record `HasLineOfWalk` calls; two pins added (`TestIsLineOfWalkWrapper_PassesTSFaithfulArgShape`, `TestHandleLineOfWalk_ArgShape`).

### NAI-165-FOLLOWUP-NAI-166 — iterator/hunt-site LOW+LOS sweep

Same `(1, 0, 0, 0)` divergence still present at:
- `pkg/script/player_iterator.go:71, 77`
- `pkg/script/npc_iterator.go:127, 139`
- `modules/world/npc_hunt_entities.go:68/73/137/142/214/219`
- `modules/world/npc_hunt.go:163`

Plus stale doc-comments (e.g. `npc_iterator.go:133-139`) claiming the broken shape "mirrors TS isLineOfWalk wrapper" — these comments need joint correction. TS source (`ScriptIterators.ts:88, 92, 113, 116, 137, 140, 160, 163, 216, 220, 284, 287, 348, 351`) confirms all sites flow through the canonical wrappers.

**Why:** Pre-existing TS-fidelity divergence; same root cause as NAI-165 but spread across iterator/hunt code paths. Per `runescript_cadence.md` queued as a separate sub-spec to keep diffs reviewable and avoid bundling with the simpler NAI-165 wrapper fix.

**How to apply:** When opening NAI-166 brainstorm, scope as ~10 production sites + ~6 doc-comment fixes + arg-recording in existing iterator/hunt test fixtures. Audit each site's TS counterpart in `ScriptIterators.ts` to confirm the canonical wrapper call. Mock-LineValidator tests in `pkg/pathfinder/routefinder/linevalidator_test.go` are unit tests of the validator itself, NOT call-site shape assertions — they should NOT be changed.

### NAI-165-FOLLOWUP-NAI-166-WRAPPER-ROUTING — `handleLineOfWalk` asymmetry

`handleLineOfWalk` (`handlers_map.go:419-421`) has explicit nil-guard → pessimistic-DENY on nil validator. `handleLineOfSight` (`handlers_map.go:230`) routes through `isLineOfSight` wrapper → pessimistic-ALLOW on nil. TS routes both through their respective `isLineOfWalk`/`isLineOfSight` wrappers (both pessimistic-allow). Pre-existing divergence; not introduced by NAI-165 but worth retiring in the same sweep.

**How to apply:** Refactor `handleLineOfWalk` to call the `isLineOfWalk` wrapper instead of `s.LineValidator.HasLineOfWalk` directly; delete the explicit nil guard at lines 419-421. The change flips nil-LV semantics from pessimistic-deny (push 0) to pessimistic-allow (push 1) — invert the existing `TestHandleLineOfWalk_NilLineValidator` pin (mirrors NAI-155 inversion pattern in `nai_followups.md` NAI-156 close).
```

Substitute the actual close-commit SHA for `<COMMIT-SHA>` after Task 4 lands. Use the output of `git log -1 --format='%h'` from Step 4.4.

- [ ] **Step 5.2: Verify the append**

Run: `tail -50 $HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`

Expected: shows the new NAI-165 section at end-of-file.

Note: the memory `MEMORY.md` index does NOT need a new entry — `nai_followups.md` is already indexed there as a generic pointer. No update needed.

---

## Verification checklist (post-execution)

After Task 5 completes, sanity-check:

- [ ] `git log -1 --format='%B'` shows the fix commit with the correct two-line arg-shape change in the body.
- [ ] `grep -n 'HasLineOfWalk.*1, 0, 0, 0' pkg/script/handlers_map.go` returns **zero matches** (both sites flipped).
- [ ] `grep -n 'HasLineOfWalk.*1, 1, 1, 0' pkg/script/handlers_map.go` returns **two matches** at line ~175 and ~423.
- [ ] `grep -rn 'NAI-165-D-LOW-ARG-SHAPE-FIX' pkg/` returns **at least 3 matches** — the wrapper doc-comment, the handler doc-comment, and the test fixture/test doc-comment.
- [ ] `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...` is fully green.
- [ ] `tail -1 $HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` shows the new NAI-166 routing block.

---

## Risks and mitigations

1. **Iterator/hunt-site test breakage (cross-package).** Unlikely — NAI-165 only touches `handlers_map.go` + `handlers_map_test.go`. Step 3.5 runs `./...` to catch any cross-package surprise.
2. **Pop-order or parameter-name confusion in `handleLineOfWalk` fixup.** The `TestHandleLineOfWalk_ArgShape` end-to-end pin catches mis-application (e.g. swapped from/to or wrong field). Step 3.3 makes this the gate before Task 4 commit.
3. **Fixture-rename collision.** Task 1 reuses `losCall` for both LOW and LOS calls (identical 9-int tuple shape) to avoid introducing a parallel type. If a future sub-spec needs to distinguish them, the slice name (`losCalls` vs `lowCalls`) is enough.
