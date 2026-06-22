# NPC_RANGE size-aware distance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port TS `CoordGrid.distanceTo + closest` semantics into `handleNpcRange` so size>1 NPCs compute Chebyshev distance from their nearest occupied tile rather than from origin. Size=1 NPCs remain byte-identical.

**Architecture:** Extend `ActiveNpc` interface with two new methods (`NpcWidth`, `NpcLength`); add delegating adapters on the production `*Npc` in `modules/world/`; add fields + default-zero-to-1 accessors on `mockNpc`; rewrite the body of `handleNpcRange` using closest-edge Chebyshev. Single commit, four files touched. Sibling-handler audit cleared `handleNpcHunt` as TS-faithful per spec §2.

**Tech Stack:** Go 1.26.3, `pkg/script` package (script handlers + test fixtures), `modules/world` package (production Npc adapter).

**Spec:** `docs/superpowers/specs/2026-05-21-npc-range-size-aware-distance-design.md` (committed at `ae9d22d4`).

---

## File Structure

```
pkg/script/active.go              MODIFY  interface ActiveNpc (+2 method declarations near line 813)
modules/world/npc_script.go       MODIFY  *Npc adapter methods (+2 methods after line 34)
pkg/script/handlers_npc_test.go   MODIFY  mockNpc fixture (+2 fields near :240, +2 accessors near :303, +3 test funcs after :3553)
pkg/script/handlers_npc.go        MODIFY  handleNpcRange body rewrite (:1128-1136) + doc-comment refresh (:1108-1113)
```

Single coherent change; no new files. The plumbing (interface + adapter + mock fields) must land together because the compile-time check `var _ script.ActiveNpc = (*Npc)(nil)` at `modules/world/npc_script.go:11` will fail if `*Npc` doesn't satisfy the extended interface. Test fixtures similarly fail to compile if mockNpc lacks the accessors.

## Validation gate prefix (per CLAUDE.md global)

All `go` commands MUST use:

```
GOROOT=$HOME/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache $HOME/go/go1.26.3/bin/go ...
GOROOT=$HOME/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache $HOME/go/go1.26.3/bin/gofmt -l ...
```

All commits use `git commit --no-gpg-sign`.

---

## Task 1: Plumb the interface — `ActiveNpc` + `*Npc` adapter + `mockNpc` accessors (compile-clean, no behavior change yet)

**Files:**
- Modify: `pkg/script/active.go:801-813` (add 2 interface method declarations)
- Modify: `modules/world/npc_script.go:29-34` (add 2 adapter methods after `NpcCategory`)
- Modify: `pkg/script/handlers_npc_test.go:239-303` (add 2 mockNpc fields + 2 accessors)

- [ ] **Step 1.1: Add NpcWidth + NpcLength to the ActiveNpc interface**

In `pkg/script/active.go`, find the existing `NpcCategory()` method declaration around line 813:

```go
	NpcCategory() int
	NpcUID() int // (typeId << 16) | nid
```

Replace with:

```go
	NpcCategory() int

	// NpcWidth returns the NPC's tile-footprint width. NPCs are square
	// in practice (NpcType.Size populates both width and length per
	// modules/world/npc.go:233-239), but the interface keeps them
	// distinct to mirror TS Npc.width/length semantics for
	// CoordGrid.distanceTo (read by NPC_RANGE per handlers_npc.go).
	NpcWidth() int

	// NpcLength returns the NPC's tile-footprint length. See NpcWidth.
	NpcLength() int

	NpcUID() int // (typeId << 16) | nid
```

- [ ] **Step 1.2: Verify the package no longer compiles**

Run:

```
GOROOT=$HOME/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache $HOME/go/go1.26.3/bin/go build ./pkg/script/... ./modules/world/... 2>&1 | head -30
```

Expected: build errors from `modules/world/npc_script.go:11` (`var _ script.ActiveNpc = (*Npc)(nil)`) AND from `pkg/script/handlers_npc_test.go` (mockNpc missing NpcWidth/NpcLength methods).

- [ ] **Step 1.3: Add NpcWidth + NpcLength adapter methods on *Npc**

In `modules/world/npc_script.go`, find the existing `NpcCategory` method around line 29-34:

```go
// NpcCategory returns the NPC's category, or -1 if its NpcType is nil.
func (n *Npc) NpcCategory() int {
	if n.typ == nil {
		return -1
	}
	return n.typ.Category
}
```

Add immediately after the closing brace:

```go

// NpcWidth returns the NPC's tile-footprint width. Delegates to the
// existing Width() method (npc.go:236) which returns n.size. NAI-120
// size-aware NPC_RANGE port.
func (n *Npc) NpcWidth() int { return n.Width() }

// NpcLength returns the NPC's tile-footprint length. Delegates to the
// existing Length() method (npc.go:239) which returns n.size. NAI-120
// size-aware NPC_RANGE port.
func (n *Npc) NpcLength() int { return n.Length() }
```

- [ ] **Step 1.4: Add width + length fields to mockNpc**

In `pkg/script/handlers_npc_test.go`, find the mockNpc field list around line 240:

```go
type mockNpc struct {
	typeID, x, z, level, uid, category int
	nid                                int
```

Replace with:

```go
type mockNpc struct {
	typeID, x, z, level, uid, category int
	width, length                      int
	nid                                int
```

- [ ] **Step 1.5: Add NpcWidth + NpcLength accessor methods on mockNpc with default-zero-to-1 fallback**

In `pkg/script/handlers_npc_test.go`, find the existing accessor block around line 294-303:

```go
func (m *mockNpc) NpcType() int        { return m.typeID }
func (m *mockNpc) NpcX() int           { return m.x }
func (m *mockNpc) NpcZ() int           { return m.z }
func (m *mockNpc) NpcLevel() int       { return m.level }
func (m *mockNpc) NpcUID() int         { return m.uid }
func (m *mockNpc) Nid() int            { return m.nid }
func (m *mockNpc) LastMovement() int   { return m.lastMovement }
func (m *mockNpc) Respawnrate() int    { return m.respawnrate }
func (m *mockNpc) TopContributor() int { return m.topContributor }
func (m *mockNpc) NpcCategory() int    { return m.category }
```

Add immediately after the `NpcCategory` accessor:

```go

// NpcWidth returns m.width, defaulting to 1 when unset. Preserves
// backward-compat with existing test fixtures that don't set width.
// The default-to-1 contract matches production semantics: NpcType.Size
// is initialized to 1 (npctype.go:310) and is never zero in production.
func (m *mockNpc) NpcWidth() int {
	if m.width == 0 {
		return 1
	}
	return m.width
}

// NpcLength returns m.length, defaulting to 1. See NpcWidth.
func (m *mockNpc) NpcLength() int {
	if m.length == 0 {
		return 1
	}
	return m.length
}
```

- [ ] **Step 1.6: Verify the package compiles + ALL existing tests still pass**

Run:

```
GOROOT=$HOME/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache $HOME/go/go1.26.3/bin/go test ./pkg/script/ ./modules/world/ -count=1 2>&1 | tail -10
```

Expected: PASS for both packages. The 5 existing NPC_RANGE tests (`TestNpcRange_SameLevel_Adjacent`/`_Diagonal`/`_DifferentLevel_Sentinel`/`_NoActiveNpc`/`_InvalidCoord`) pass unchanged because (a) the handler is still origin-based (no change yet) and (b) all 5 use mockNpc with zero width/length, which the default-zero-to-1 accessor maps to 1.

---

## Task 2: Write 3 failing size>1 tests (red phase)

**Files:**
- Modify: `pkg/script/handlers_npc_test.go` (add 3 test funcs immediately after `TestNpcRange_InvalidCoord` at `:3540-3553`)

- [ ] **Step 2.1: Add three size>1 test functions**

In `pkg/script/handlers_npc_test.go`, find `TestNpcRange_InvalidCoord` around line 3540 and its closing brace at line 3553:

```go
func TestNpcRange_InvalidCoord(t *testing.T) {
	npc := &mockNpc{x: 3222, z: 3218, level: 0}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	// Negative coord fails CoordValid.
	s.PushInt(-1)
	if err := handleNpcRange(s); err == nil {
		t.Error("NPC_RANGE invalid coord: want error")
	}
}
```

Immediately after the closing brace, append:

```go

// TestNpcRange_Size3_TargetInsideFootprint: size-3 NPC at origin (10, 10)
// occupies (10..12, 10..12). Target at (11, 11) is INSIDE the footprint —
// closest cell is (11, 11), distance 0. TS-faithful per
// CoordGrid.distanceTo + closest (CoordGrid.ts:60-72).
func TestNpcRange_Size3_TargetInsideFootprint(t *testing.T) {
	npc := &mockNpc{x: 10, z: 10, level: 0, width: 3, length: 3}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	// Packed coord at (11, 11, level 0).
	s.PushInt((0 << 28) | (11 << 14) | 11)
	if err := handleNpcRange(s); err != nil {
		t.Fatalf("NPC_RANGE size-3 inside: unexpected error %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("NPC_RANGE size-3 inside: got %d, want 0", got)
	}
}

// TestNpcRange_Size3_TargetEastOfFootprint: size-3 NPC at origin (10, 10)
// occupies (10..12, 10..12). Target at (15, 11). Closest cell = (12, 11),
// distance = max(|12-15|, |11-11|) = 3. Origin-based would erroneously
// return max(|10-15|, |10-11|) = 5; this test pins the divergence fix.
func TestNpcRange_Size3_TargetEastOfFootprint(t *testing.T) {
	npc := &mockNpc{x: 10, z: 10, level: 0, width: 3, length: 3}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	// Packed coord at (15, 11, level 0).
	s.PushInt((0 << 28) | (15 << 14) | 11)
	if err := handleNpcRange(s); err != nil {
		t.Fatalf("NPC_RANGE size-3 east: unexpected error %v", err)
	}
	if got := s.PopInt(); got != 3 {
		t.Errorf("NPC_RANGE size-3 east: got %d, want 3", got)
	}
}

// TestNpcRange_Size3_TargetSouthwestOfFootprint: size-3 NPC at origin
// (10, 10), target at (8, 8). Closest cell = (10, 10), distance =
// max(|10-8|, |10-8|) = 2. This case is byte-identical between origin
// and closest-edge formulas (SW of origin); pin for regression safety.
func TestNpcRange_Size3_TargetSouthwestOfFootprint(t *testing.T) {
	npc := &mockNpc{x: 10, z: 10, level: 0, width: 3, length: 3}
	s := &ScriptState{
		ActiveNpc:   npc,
		Pointers:    PtrActiveNpc,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	// Packed coord at (8, 8, level 0).
	s.PushInt((0 << 28) | (8 << 14) | 8)
	if err := handleNpcRange(s); err != nil {
		t.Fatalf("NPC_RANGE size-3 SW: unexpected error %v", err)
	}
	if got := s.PopInt(); got != 2 {
		t.Errorf("NPC_RANGE size-3 SW: got %d, want 2", got)
	}
}
```

- [ ] **Step 2.2: Run the new tests — confirm 2 of 3 fail**

Run:

```
GOROOT=$HOME/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache $HOME/go/go1.26.3/bin/go test ./pkg/script/ -run "TestNpcRange_Size3" -count=1 -v 2>&1 | tail -25
```

Expected:
- `TestNpcRange_Size3_TargetInsideFootprint` — **FAIL** with `"got 1, want 0"` (origin-based formula computes max(0, 1) = 1, not the correct 0 because (11,11) is inside footprint but origin is (10,10))
- `TestNpcRange_Size3_TargetEastOfFootprint` — **FAIL** with `"got 5, want 3"` (origin-based: max(5,1) = 5; correct: max(3,0) = 3)
- `TestNpcRange_Size3_TargetSouthwestOfFootprint` — **PASS** (byte-identical case; both formulas return 2)

This is the expected red state for TDD. Two failures + one byte-identical pass confirms the new tests are exercising the right code paths AND that the SW case is correctly identified as a regression-safety pin (no divergence to exploit).

---

## Task 3: Implement size-aware NPC_RANGE (green phase)

**Files:**
- Modify: `pkg/script/handlers_npc.go:1108-1138` (rewrite handler body + refresh doc-comment)

- [ ] **Step 3.1: Replace the doc-comment "size>1 audit deferred" framing**

In `pkg/script/handlers_npc.go`, find the comment block ending at line 1113:

```go
// Multi-tile NPCs (size > 1): the inner-ring call sites in
// player_combat.rs2 do not require size-aware distance -- sites pass
// `coord` (the player's own coord) and the active NPC is the combat
// target. This handler treats the NPC as a 1x1 source (matches TS
// behaviour for size=1 NPCs; size>1 audit deferred to a future sub-spec
// per NAI-120 Bundle 1 audit section 6 dependency note).
func handleNpcRange(s *ScriptState) error {
```

Replace with:

```go
// Multi-tile NPCs (size > 1): goscape ports the full TS
// CoordGrid.distanceTo + CoordGrid.closest semantics (CoordGrid.ts:60-72)
// — clamp the target cell into the NPC's occupied footprint
// [(npc.x, npc.z) .. (npc.x + npc.width - 1, npc.z + npc.length - 1)]
// and take the max-absolute-axis delta from the clamped point. For
// size=1 NPCs (width = length = 1), occupiedX/Z collapse to npc.x/z
// and the formula reduces to origin-based Chebyshev (byte-identical
// to the size=1 prior behavior). Closes the NAI-120 Bundle 1 audit
// section 6 deferral per docs/superpowers/specs/2026-05-21-npc-range-
// size-aware-distance-design.md.
func handleNpcRange(s *ScriptState) error {
```

- [ ] **Step 3.2: Replace the handler body (lines 1128-1136)**

In `pkg/script/handlers_npc.go`, find lines 1128-1136 in `handleNpcRange`:

```go
	dx := n.NpcX() - x
	if dx < 0 {
		dx = -dx
	}
	dz := n.NpcZ() - z
	if dz < 0 {
		dz = -dz
	}
	s.PushInt(max(dx, dz))
	return nil
}
```

Replace with:

```go
	// Closest-edge Chebyshev per TS CoordGrid.distanceTo + closest
	// (CoordGrid.ts:60-72): clamp the target cell into the NPC's
	// occupied footprint, then take the max-absolute-axis delta. For
	// size=1 NPCs (width=length=1), occupiedX = n.NpcX() and the
	// formula collapses to the prior origin-Chebyshev form
	// (byte-identical).
	nx := n.NpcX()
	nz := n.NpcZ()
	occupiedX := nx + n.NpcWidth() - 1
	occupiedZ := nz + n.NpcLength() - 1

	clampedX := x
	if x < nx {
		clampedX = nx
	} else if x > occupiedX {
		clampedX = occupiedX
	}
	clampedZ := z
	if z < nz {
		clampedZ = nz
	} else if z > occupiedZ {
		clampedZ = occupiedZ
	}

	dx := clampedX - x
	if dx < 0 {
		dx = -dx
	}
	dz := clampedZ - z
	if dz < 0 {
		dz = -dz
	}
	s.PushInt(max(dx, dz))
	return nil
}
```

- [ ] **Step 3.3: Run NPC_RANGE tests — confirm all 8 pass**

Run:

```
GOROOT=$HOME/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache $HOME/go/go1.26.3/bin/go test ./pkg/script/ -run "TestNpcRange" -count=1 -v 2>&1 | tail -25
```

Expected: 8 PASSes total
- `TestNpcRange_SameLevel_Adjacent` PASS (size=1 byte-identical)
- `TestNpcRange_SameLevel_Diagonal` PASS (size=1 byte-identical)
- `TestNpcRange_DifferentLevel_Sentinel` PASS (early -1 return unchanged)
- `TestNpcRange_NoActiveNpc` PASS (early-error guard unchanged)
- `TestNpcRange_InvalidCoord` PASS (early-error guard unchanged)
- `TestNpcRange_Size3_TargetInsideFootprint` PASS (got 0, want 0)
- `TestNpcRange_Size3_TargetEastOfFootprint` PASS (got 3, want 3)
- `TestNpcRange_Size3_TargetSouthwestOfFootprint` PASS (got 2, want 2)

---

## Task 4: Validation gates + audit-grep + single commit

**Files:** No new edits — verification only, then commit.

- [ ] **Step 4.1: Run full race test suite**

Run:

```
GOROOT=$HOME/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache $HOME/go/go1.26.3/bin/go test -race ./... -count=1 2>&1 | tail -5
```

Expected: All packages OK, 0 FAIL.

- [ ] **Step 4.2: Run pack-all smoke test**

Run:

```
GOROOT=$HOME/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache $HOME/go/go1.26.3/bin/go test ./pkg/packall/ -run TestPackAll_TwelveStageSmoke -count=1 2>&1 | tail -3
```

Expected: `ok  	github.com/zsrv/goscape/pkg/packall	<duration>s`

- [ ] **Step 4.3: Run gofmt check on touched files**

Run:

```
GOROOT=$HOME/go/go1.26.3 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache $HOME/go/go1.26.3/bin/gofmt -l pkg/script/active.go modules/world/npc_script.go pkg/script/handlers_npc_test.go pkg/script/handlers_npc.go
```

Expected: empty output (all 4 files clean).

- [ ] **Step 4.4: Audit-grep verification**

Run each grep and confirm the expected count:

```
grep -c "NpcWidth()\b" pkg/script/active.go modules/world/npc_script.go pkg/script/handlers_npc_test.go pkg/script/handlers_npc.go
```

Expected: at least one hit per file (interface decl, adapter, mock accessor, handler use). Total ≥ 4.

```
grep -c "NpcLength()\b" pkg/script/active.go modules/world/npc_script.go pkg/script/handlers_npc_test.go pkg/script/handlers_npc.go
```

Expected: at least one hit per file. Total ≥ 4.

```
grep -c "occupiedX\|occupiedZ" pkg/script/handlers_npc.go
```

Expected: 4 (two declarations + two reads inside the clamp logic).

```
grep "size>1 audit deferred" pkg/script/handlers_npc.go
```

Expected: empty output (carry-forward retired).

- [ ] **Step 4.5: Stage only the touched files (NOT config.yaml standing drift)**

Run:

```
git status --short
```

Expected to show:
- `M  pkg/script/active.go`
- `M  modules/world/npc_script.go`
- `M  pkg/script/handlers_npc_test.go`
- `M  pkg/script/handlers_npc.go`
- `M  config.yaml` (standing drift — DO NOT stage)

Stage only the 4 task files:

```
git add pkg/script/active.go modules/world/npc_script.go pkg/script/handlers_npc_test.go pkg/script/handlers_npc.go
```

Confirm:

```
git diff --cached --stat
```

Expected: 4 files changed, with `config.yaml` NOT listed.

- [ ] **Step 4.6: Commit (single commit per spec §12)**

Run:

```
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(script): size-aware NPC_RANGE Chebyshev distance per TS

Port TS CoordGrid.distanceTo + closest semantics into handleNpcRange
so size>1 NPCs compute Chebyshev distance from their nearest occupied
tile rather than from origin. Size=1 NPCs are byte-identical; size>1
diverged east/north of origin by (size-1) per axis.

Adds NpcWidth/NpcLength to the ActiveNpc interface (one method per
field, matching the existing NpcX/NpcZ pattern) with delegating
adapters on *Npc (Width/Length already exist) and default-zero-to-1
accessors on mockNpc (preserves the &ScriptState{} test-fixture
convention from state.go:277). NPC_HUNT's origin-Euclidean ranking is
TS-faithful per CoordGrid.euclideanSquaredDistance and stays unchanged.

Closes the size>1 deferral at handlers_npc.go:1108-1113 noted in
NAI-120 Bundle 1 audit section 6. See
docs/superpowers/specs/2026-05-21-npc-range-size-aware-distance-design.md.
EOF
)"
```

- [ ] **Step 4.7: Confirm post-commit state**

Run:

```
git log --oneline -3 && git status --short
```

Expected:
- New commit at HEAD with the message above
- Previous commit `ae9d22d4 docs(spec): NPC_RANGE size-aware distance design`
- `git status` shows only `M  config.yaml` (standing drift unchanged)

---

## Post-implementation: close-memo + MEMORY.md index entry

After the impl commit lands, write a close memo at `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/npc_range_size_aware_distance_close.md` following the established session pattern, and prepend a one-line index entry to `MEMORY.md`. This is post-impl bookkeeping, not part of the task checklist; the controlling agent handles it after Task 4 completes.

---

## Self-review checklist (run before declaring plan complete)

**1. Spec coverage:**

| Spec section | Plan task |
|---|---|
| §3 architecture (3-layer change) | Task 1 (interface + adapter + mock), Task 3 (handler) |
| §4 interface extension | Task 1 Steps 1.1, 1.2 |
| §5 production adapter | Task 1 Step 1.3 |
| §6 mockNpc fixture | Task 1 Steps 1.4, 1.5 |
| §7 handler rewrite | Task 3 Steps 3.1, 3.2 |
| §7.1 inline-vs-helper rationale | Embodied in Task 3 Step 3.2 (inline) |
| §8.1 preserved 5 tests | Task 1 Step 1.6 + Task 3 Step 3.3 |
| §8.2 new 3 tests | Task 2 Step 2.1 |
| §8.3 no dedicated accessor tests | Embodied in Task 2 (integration coverage via size>1 tests) |
| §9 gates + audit-greps | Task 4 Steps 4.1-4.4 |
| §10 risk mitigations | Embodied in Task 1.6 (existing-tests-still-pass risk), Task 4.5 (config.yaml drift risk) |
| §11 out-of-scope | Not in plan (correctly) |
| §12 single commit | Task 4 Step 4.6 |
| §13 cadence (in-thread XS) | Plan structure (4 tasks, single commit) |

All spec sections accounted for.

**2. Placeholder scan:** No "TBD", "TODO", "fill in", or "similar to Task N" markers. All code blocks are complete and self-contained.

**3. Type consistency:** Method names `NpcWidth`/`NpcLength` used consistently across spec, interface decl (Step 1.1), adapter (Step 1.3), mock accessor (Step 1.5), and handler use (Step 3.2). Field names `width`/`length` used consistently across mockNpc field decl (Step 1.4), test construction (Step 2.1), and accessor reads (Step 1.5).
