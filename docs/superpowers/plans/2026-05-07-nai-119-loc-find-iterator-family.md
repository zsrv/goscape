# NAI-119 LOC_FIND Iterator Family Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire LOC_FINDALLZONE (opcode 3008) and LOC_FINDNEXT (opcode 3009) so `[proc,tut_open_mining_gate]` (`LostCityRS/Content/scripts/tutorial/scripts/tut_doors_and_gates.rs2:131`) executes successfully and the Tutorial Island mining-area exit gate opens.

**Architecture:** Mirror the NPC_FIND iterator family at narrower scope. New `LocIterator` type (single-zone, no filtering, lazy snapshot via existing `LocOps.AllLocsInZone`); two new fields on `ScriptState` (`OtherActiveLoc`, `locIterator`); `setActiveLocSlot` helper threading IntOperand 0/1 to primary/secondary slot; two new handlers + dispatch entries. Cadence B: combined single review at end (not per-task two-stage).

**Tech Stack:** Go 1.26+, package `pkg/script`. References: `pkg/script/npc_iterator.go`, `pkg/script/handlers_npc.go:64-83, 704-795`. TS source: `Engine-TS/src/engine/script/ScriptIterators.ts:365-385`, `LocOps.ts:96-112`.

**Spec:** `docs/superpowers/specs/2026-05-07-nai-119-loc-find-iterator-family-design.md` (commit `f0454ef`).

**HEAD at plan-writing:** `f0454ef`. All file:line anchors below verified at this HEAD.

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `pkg/script/state.go` | Modify | Add `OtherActiveLoc ActiveLoc` field + `locIterator *LocIterator` field |
| `pkg/script/loc_iterator.go` | Create | `LocIterator` type, `NewZoneLocIterator`, `Stale`, `Next` |
| `pkg/script/loc_iterator_test.go` | Create | Iterator-level unit tests |
| `pkg/script/handlers_loc.go` | Modify | Add `setActiveLocSlot`, `handleLocFindAllZone`, `handleLocFindNext` |
| `pkg/script/handlers.go` | Modify | Add dispatch entries for `OpLocFindAllZone` + `OpLocFindNext` |
| `pkg/script/handlers_loc_test.go` | Modify | Handler-level tests |

---

## Task 1: Iterator infrastructure

**Files:**
- Modify: `pkg/script/state.go` (add 2 fields)
- Create: `pkg/script/loc_iterator.go`
- Create: `pkg/script/loc_iterator_test.go`

This task adds the `LocIterator` type and its supporting `ScriptState` fields, with full unit-test coverage for the iterator itself (no handler-level tests yet — those come in Task 2).

### - [ ] Step 1.1: Add `OtherActiveLoc` field to `ScriptState`

In `pkg/script/state.go`, find this block (currently at lines 229-234):

```go
	// ActiveLoc is the Loc that LOC_* ops target. Nil if no Loc is
	// bound to this script's execution. Set by callers (test fixtures,
	// OPLOC trigger routing). Type is the package-local ActiveLoc
	// interface (currently empty — handlers_loc.go will populate
	// methods in a follow-up sub-spec).
	ActiveLoc ActiveLoc
```

Insert immediately after that block (before `ActiveObj`):

```go
	// OtherActiveLoc is the secondary Loc slot, parallel to OtherActiveNpc.
	// Set by LOC_FINDNEXT (and any future LOC_FIND family handler) when
	// the bytecode IntOperand is 1 (.loc2 syntax). NAI-119.
	//
	// NAI-119-D-NO-DOWNSTREAM-LOC2-CONSUMERS: no existing LOC_* read
	// handler reads from this slot at HEAD — they all read s.ActiveLoc
	// only. Tracked deviation; closure when a `.loc2` content-script
	// consumer surfaces.
	OtherActiveLoc ActiveLoc
```

### - [ ] Step 1.2: Add `locIterator` field to `ScriptState`

In `pkg/script/state.go`, find this block (currently at lines 244-250):

```go
	// npcIterator holds the active NPC_FIND iterator state. Set by
	// FINDALL/FINDALLANY/FINDALLZONE; consumed by FINDNEXT. Lifetime is
	// single-tick — Stale() check enforced at FINDNEXT against
	// s.World.CurrentTick(). Nil = no active iterator. Mirrors TS
	// ScriptState.npcIterator (ScriptState.ts:125). Lowercase = package-
	// private; handlers in pkg/script access directly. NAI-33.
	npcIterator *NpcIterator
```

Insert immediately after that block (before `playerIterator`):

```go
	// locIterator holds the active LOC_FIND iterator state. Set by
	// LOC_FINDALLZONE; consumed by LOC_FINDNEXT. Lifetime is single-tick
	// — Stale() check enforced at FINDNEXT against s.World.CurrentTick().
	// Nil = no active iterator. Mirrors TS ScriptState.locIterator. NAI-119.
	locIterator *LocIterator
```

### - [ ] Step 1.3: Verify the file still compiles

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/script/...`

Expected: FAIL with `undefined: LocIterator` — that's correct, we add the type next.

### - [ ] Step 1.4: Write the failing iterator tests (red)

Create `pkg/script/loc_iterator_test.go`:

```go
package script

import "testing"

// locIterTestOps is a fakeLocOps wrapper that returns a fixed slice
// from AllLocsInZone. Independent of fakeLocOps.inZone (which is
// shared across other tests).
type locIterTestOps struct {
	*fakeLocOps
	zoneLocs []ActiveLoc
}

func (o *locIterTestOps) AllLocsInZone(level, x, z int) []ActiveLoc {
	return o.zoneLocs
}

func newLocIterTestOps(locs []ActiveLoc) *locIterTestOps {
	return &locIterTestOps{
		fakeLocOps: &fakeLocOps{},
		zoneLocs:   locs,
	}
}

// TestNewZoneLocIteratorStoresFields pins the constructor: tick, level,
// x, z stored verbatim; iteration state (locs, idx, started) zeroed.
func TestNewZoneLocIteratorStoresFields(t *testing.T) {
	ops := newLocIterTestOps(nil)
	it := NewZoneLocIterator(ops, 42, 0, 3200, 3300)
	if it.creationTick != 42 {
		t.Errorf("creationTick: got %d, want 42", it.creationTick)
	}
	if it.level != 0 {
		t.Errorf("level: got %d, want 0", it.level)
	}
	if it.x != 3200 {
		t.Errorf("x: got %d, want 3200", it.x)
	}
	if it.z != 3300 {
		t.Errorf("z: got %d, want 3300", it.z)
	}
	if it.idx != 0 {
		t.Errorf("idx: got %d, want 0", it.idx)
	}
	if it.started {
		t.Error("started: got true, want false (lazy snapshot)")
	}
	if it.locs != nil {
		t.Error("locs: should be nil before first Next()")
	}
}

// TestLocIteratorStaleAtSameTick pins the strict-greater-than semantics
// of TS ScriptIterators.ts:379 (currentTick > creationTick).
func TestLocIteratorStaleAtSameTick(t *testing.T) {
	it := NewZoneLocIterator(newLocIterTestOps(nil), 100, 0, 3200, 3300)
	if it.Stale(100) {
		t.Error("Stale(currentTick == creationTick): got true, want false")
	}
}

// TestLocIteratorStaleNextTick pins that any forward-tick advancement
// trips Stale().
func TestLocIteratorStaleNextTick(t *testing.T) {
	it := NewZoneLocIterator(newLocIterTestOps(nil), 100, 0, 3200, 3300)
	if !it.Stale(101) {
		t.Error("Stale(currentTick > creationTick): got false, want true")
	}
}

// TestLocIteratorYieldsAllZoneLocs pins that Next() drains the snapshot
// in slice order and exhausts cleanly.
func TestLocIteratorYieldsAllZoneLocs(t *testing.T) {
	loc1 := fakeActiveLoc{id: 100}
	loc2 := fakeActiveLoc{id: 101}
	loc3 := fakeActiveLoc{id: 102}
	ops := newLocIterTestOps([]ActiveLoc{loc1, loc2, loc3})
	it := NewZoneLocIterator(ops, 0, 0, 3200, 3300)

	got := []ActiveLoc{}
	for {
		loc, ok := it.Next()
		if !ok {
			break
		}
		got = append(got, loc)
	}
	if len(got) != 3 {
		t.Fatalf("yield count: got %d, want 3", len(got))
	}
	if got[0].LocType() != 100 || got[1].LocType() != 101 || got[2].LocType() != 102 {
		t.Errorf("yield order: got [%d, %d, %d], want [100, 101, 102]",
			got[0].LocType(), got[1].LocType(), got[2].LocType())
	}
}

// TestLocIteratorEmptyZone pins that empty zones exhaust on first Next().
func TestLocIteratorEmptyZone(t *testing.T) {
	ops := newLocIterTestOps([]ActiveLoc{})
	it := NewZoneLocIterator(ops, 0, 0, 3200, 3300)
	if loc, ok := it.Next(); ok {
		t.Errorf("empty zone first Next: got (%v, true), want (nil, false)", loc)
	}
}

// TestLocIteratorExhaustionDoesNotClear pins TS NpcIterator parallel:
// after exhaustion subsequent Next() calls keep returning (nil, false)
// without panic. Iterator is NOT nilled (matches NPC family — see
// pkg/script/handlers_npc.go:769-771 doc comment).
func TestLocIteratorExhaustionDoesNotClear(t *testing.T) {
	loc1 := fakeActiveLoc{id: 100}
	ops := newLocIterTestOps([]ActiveLoc{loc1})
	it := NewZoneLocIterator(ops, 0, 0, 3200, 3300)

	// Drain.
	if _, ok := it.Next(); !ok {
		t.Fatal("first Next: expected hit")
	}
	if _, ok := it.Next(); ok {
		t.Fatal("second Next: expected exhaustion")
	}
	// Subsequent calls must continue to return (nil, false).
	for i := 0; i < 3; i++ {
		if loc, ok := it.Next(); ok || loc != nil {
			t.Errorf("post-exhaustion Next #%d: got (%v, %v), want (nil, false)", i, loc, ok)
		}
	}
}

// TestLocIteratorNilOpsDegrades pins the NpcIterator.Next:238-240
// parallel: nil ops returns (nil, false) on first Next without panic.
func TestLocIteratorNilOpsDegrades(t *testing.T) {
	it := NewZoneLocIterator(nil, 0, 0, 3200, 3300)
	if loc, ok := it.Next(); ok || loc != nil {
		t.Errorf("nil ops first Next: got (%v, %v), want (nil, false)", loc, ok)
	}
}
```

### - [ ] Step 1.5: Run the new tests to confirm they fail to compile

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestLocIterator|TestNewZoneLocIterator' 2>&1 | head -20`

Expected: FAIL with `undefined: NewZoneLocIterator` and/or `undefined: LocIterator`. Test file does not yet have the implementation to call.

### - [ ] Step 1.6: Implement `LocIterator`

Create `pkg/script/loc_iterator.go`:

```go
package script

// LocIterator is the script-VM iterator state for the LOC_FIND iterator
// family (currently LOC_FINDALLZONE only — the LOC iterator family is
// single-mode, unlike NpcIterator's DISTANCE/ZONE/HuntAll). Mirrors TS
// LocIterator at ScriptIterators.ts:365-385.
//
// Lifetime: single-tick. Created by LOC_FINDALLZONE; consumed by
// LOC_FINDNEXT. Stale() check at FINDNEXT compares creationTick to
// World.CurrentTick(); on mismatch the handler returns an error
// mirroring the NPC family pattern (npc_script.go log-warn +
// ClearActiveScript path runs).
//
// Snapshot strategy: lazy on first Next() call via
// LocOps.AllLocsInZone(level, x, z). Subsequent calls drain the
// snapshot. TS uses a generator over `getZone(...).getAllLocsSafe(true)`
// — equivalent because both produce a single point-in-time slice that
// the iterator drains independent of subsequent zone mutation.
//
// Ownership: held by ScriptState.locIterator. Nil = no active iterator.
type LocIterator struct {
	creationTick int
	ops          LocOps
	level, x, z  int
	locs         []ActiveLoc
	idx          int
	started      bool
}

// NewZoneLocIterator constructs a single-zone iterator for the zone
// containing (level, x, z). Mirrors TS LocIterator constructor at
// ScriptIterators.ts:370-374. The snapshot is deferred to first Next();
// the constructor only stores center coords and tick.
func NewZoneLocIterator(ops LocOps, tick, level, x, z int) *LocIterator {
	return &LocIterator{
		creationTick: tick,
		ops:          ops,
		level:        level,
		x:            x,
		z:            z,
	}
}

// Stale reports whether the iterator was created in a prior tick. The
// FINDNEXT handler MUST check this before calling Next when single-tick
// lifetime matters. Mirrors TS strict-greater-than at
// ScriptIterators.ts:379 (World.currentTick > this.tick).
func (it *LocIterator) Stale(currentTick int) bool {
	return currentTick > it.creationTick
}

// Next returns the next loc in the zone snapshot, or (nil, false) on
// exhaustion. Lazy-initializes the snapshot on first call.
//
// Nil-ops degrades to immediate exhaustion (test stub or pre-wiring) —
// mirrors NpcIterator.Next nil-lookup handling at npc_iterator.go:238-240.
func (it *LocIterator) Next() (ActiveLoc, bool) {
	if !it.started {
		it.started = true
		if it.ops != nil {
			it.locs = it.ops.AllLocsInZone(it.level, it.x, it.z)
		}
	}
	if it.idx >= len(it.locs) {
		return nil, false
	}
	loc := it.locs[it.idx]
	it.idx++
	return loc, true
}
```

### - [ ] Step 1.7: Run the iterator tests to confirm they pass (green)

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestLocIterator|TestNewZoneLocIterator' -v 2>&1 | tail -20`

Expected: All 7 tests PASS.

### - [ ] Step 1.8: Run the full pkg/script test suite to confirm no regressions

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...`

Expected: `ok  github.com/zsrv/goscape/pkg/script` — clean.

### - [ ] Step 1.9: Run vet

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./pkg/script/...`

Expected: clean (no output).

### - [ ] Step 1.10: Commit Task 1

```bash
git add pkg/script/state.go pkg/script/loc_iterator.go pkg/script/loc_iterator_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-119 T1 — LocIterator + ScriptState fields

Adds LocIterator type (pkg/script/loc_iterator.go) for the
LOC_FINDALLZONE / LOC_FINDNEXT pair. Single-zone, no filtering, lazy
snapshot via the existing LocOps.AllLocsInZone surface (already used by
NAI-114 MAP_LOCADDUNSAFE). Mirrors TS ScriptIterators.ts:365-385 and
the goscape NpcIterator ZONE-mode template (pkg/script/npc_iterator.go).

Adds ScriptState.OtherActiveLoc (parallel to OtherActiveNpc) and
ScriptState.locIterator. Tracked deviation
NAI-119-D-NO-DOWNSTREAM-LOC2-CONSUMERS: no existing LOC_* read handler
reads from OtherActiveLoc; closure when a .loc2 content-script consumer
surfaces.

Tests pin: constructor field-storage, strict-> Stale semantics,
multi-loc yield order, empty-zone exhaustion, post-exhaustion stability,
nil-ops degradation.

Spec: docs/superpowers/specs/2026-05-07-nai-119-loc-find-iterator-family-design.md
Plan: docs/superpowers/plans/2026-05-07-nai-119-loc-find-iterator-family.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Handler dispatch

**Files:**
- Modify: `pkg/script/handlers_loc.go`
- Modify: `pkg/script/handlers.go`
- Modify: `pkg/script/handlers_loc_test.go`

This task wires LOC_FINDALLZONE and LOC_FINDNEXT through the dispatch table, with full handler-level test coverage (including the dual-slot pin).

### - [ ] Step 2.1: Verify Task 1 commit landed

Run: `git log --oneline -1`

Expected: HEAD shows `feat(script): NAI-119 T1 ...`. If not, return to Task 1.

### - [ ] Step 2.2: Write the failing handler tests (red)

First, add the `strings` import to `pkg/script/handlers_loc_test.go`. The current import block is:

```go
import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/objtype"
)
```

Update it to:

```go
import (
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/objtype"
)
```

Then append to `pkg/script/handlers_loc_test.go`:

```go
// --- NAI-119: LOC_FINDALLZONE handler tests --------------------------

// newLocFindAllZoneState builds a ScriptState with a coord on the int
// stack, World wired (for CurrentTick), LocOps wired. Mirror of
// newNpcFindNextState (handlers_npc_test.go) plus a stack-prepushed coord.
func newLocFindAllZoneState(t *testing.T, tick int, ops LocOps, coord int) *ScriptState {
	t.Helper()
	mw := newMockWorld()
	mw.tick = tick
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{0}},
		PC:          0,
		World:       mw,
		LocOps:      ops,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.PushInt(coord)
	return s
}

// TestLocFindAllZoneStoresIterator pins LOC_FINDALLZONE: pop coord →
// store iterator with creationTick from World.CurrentTick + level/x/z
// from coord.
func TestLocFindAllZoneStoresIterator(t *testing.T) {
	ops := newLocIterTestOps(nil)
	coord := coordgrid.PackCoord(0, 3200, 3300)
	s := newLocFindAllZoneState(t, 100, ops, coord)

	if err := handleLocFindAllZone(s); err != nil {
		t.Fatalf("handleLocFindAllZone: %v", err)
	}
	if s.locIterator == nil {
		t.Fatal("locIterator: got nil, want set")
	}
	if s.locIterator.creationTick != 100 {
		t.Errorf("creationTick: got %d, want 100 (from World.CurrentTick)",
			s.locIterator.creationTick)
	}
	if s.locIterator.level != 0 || s.locIterator.x != 3200 || s.locIterator.z != 3300 {
		t.Errorf("coord: got (%d, %d, %d), want (0, 3200, 3300)",
			s.locIterator.level, s.locIterator.x, s.locIterator.z)
	}
}

// TestLocFindAllZoneNilLocOpsDegrades pins the parallel-NPC nil-ops
// degradation: handler returns nil, locIterator stays nil.
func TestLocFindAllZoneNilLocOpsDegrades(t *testing.T) {
	coord := coordgrid.PackCoord(0, 3200, 3300)
	s := newLocFindAllZoneState(t, 100, nil, coord)
	// LocOps is nil — explicitly set to confirm.
	s.LocOps = nil

	if err := handleLocFindAllZone(s); err != nil {
		t.Fatalf("handleLocFindAllZone: got err %v, want nil (degrade silently)", err)
	}
	if s.locIterator != nil {
		t.Errorf("locIterator: got %v, want nil (no iterator on nil-ops)", s.locIterator)
	}
}

// TestLocFindAllZoneCoordValid pins the checkCoord error path: invalid
// coord (-1) yields the wrapped error.
func TestLocFindAllZoneCoordValid(t *testing.T) {
	ops := newLocIterTestOps(nil)
	s := newLocFindAllZoneState(t, 100, ops, -1)

	err := handleLocFindAllZone(s)
	if err == nil {
		t.Fatal("handleLocFindAllZone(-1): want error, got nil")
	}
	// Error string should be checkCoord's format; assert opcode tag.
	if want := "LOC_FINDALLZONE"; !strings.Contains(err.Error(), want) {
		t.Errorf("err: got %q, want substring %q", err.Error(), want)
	}
}

// --- NAI-119: LOC_FINDNEXT handler tests -----------------------------

// newLocFindNextState builds a ScriptState with World wired (for
// CurrentTick), an optional locIterator pre-installed, and IntOperands
// supplied for setActiveLocSlot to read. Mirror of newNpcFindNextState.
func newLocFindNextState(t *testing.T, tick int, iter *LocIterator, intOperand int32) *ScriptState {
	t.Helper()
	mw := newMockWorld()
	mw.tick = tick
	s := &ScriptState{
		Script:      &ScriptFile{IntOperands: []int32{intOperand}},
		PC:          0,
		World:       mw,
		IntStack:    make([]int, StackCapacity),
		StringStack: make([]string, StackCapacity),
	}
	s.locIterator = iter
	return s
}

// TestLocFindNextNoIterator pins the nil-iterator branch: pushes 0,
// no error, ActiveLoc/OtherActiveLoc untouched.
func TestLocFindNextNoIterator(t *testing.T) {
	s := newLocFindNextState(t, 100, nil, 0)

	if err := handleLocFindNext(s); err != nil {
		t.Fatalf("handleLocFindNext: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("nil iterator: got push %d, want 0", got)
	}
	if s.ActiveLoc != nil {
		t.Error("ActiveLoc should remain nil")
	}
	if s.OtherActiveLoc != nil {
		t.Error("OtherActiveLoc should remain nil")
	}
}

// TestLocFindNextHitPrimarySlot pins LOC_FINDNEXT IntOperand=0:
// pushes 1, sets ActiveLoc + PtrActiveLoc.
func TestLocFindNextHitPrimarySlot(t *testing.T) {
	loc := fakeActiveLoc{id: 100}
	ops := newLocIterTestOps([]ActiveLoc{loc})
	iter := NewZoneLocIterator(ops, 100, 0, 3200, 3300)
	s := newLocFindNextState(t, 100, iter, 0)

	if err := handleLocFindNext(s); err != nil {
		t.Fatalf("handleLocFindNext: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("hit: got push %d, want 1", got)
	}
	if s.ActiveLoc == nil || s.ActiveLoc.LocType() != 100 {
		t.Errorf("ActiveLoc: got %v, want id=100", s.ActiveLoc)
	}
	if s.OtherActiveLoc != nil {
		t.Error("OtherActiveLoc should remain nil for IntOperand=0")
	}
	if s.Pointers&PtrActiveLoc == 0 {
		t.Error("PtrActiveLoc should be set")
	}
	if s.Pointers&PtrActiveLoc2 != 0 {
		t.Error("PtrActiveLoc2 should NOT be set for IntOperand=0")
	}
}

// TestLocFindNextHitSecondarySlot pins LOC_FINDNEXT IntOperand=1:
// pushes 1, sets OtherActiveLoc + PtrActiveLoc2 (primary slot
// untouched). Closes NAI-119 dual-slot decision.
func TestLocFindNextHitSecondarySlot(t *testing.T) {
	loc := fakeActiveLoc{id: 200}
	ops := newLocIterTestOps([]ActiveLoc{loc})
	iter := NewZoneLocIterator(ops, 100, 0, 3200, 3300)
	s := newLocFindNextState(t, 100, iter, 1)

	if err := handleLocFindNext(s); err != nil {
		t.Fatalf("handleLocFindNext: %v", err)
	}
	if got := s.PopInt(); got != 1 {
		t.Errorf("hit: got push %d, want 1", got)
	}
	if s.OtherActiveLoc == nil || s.OtherActiveLoc.LocType() != 200 {
		t.Errorf("OtherActiveLoc: got %v, want id=200", s.OtherActiveLoc)
	}
	if s.ActiveLoc != nil {
		t.Error("ActiveLoc should remain nil for IntOperand=1")
	}
	if s.Pointers&PtrActiveLoc2 == 0 {
		t.Error("PtrActiveLoc2 should be set")
	}
	if s.Pointers&PtrActiveLoc != 0 {
		t.Error("PtrActiveLoc should NOT be set for IntOperand=1")
	}
}

// TestLocFindNextExhaustionPushesZero pins post-exhaustion behavior:
// FINDNEXT pushes 0, leaves ActiveLoc/OtherActiveLoc untouched.
func TestLocFindNextExhaustionPushesZero(t *testing.T) {
	ops := newLocIterTestOps([]ActiveLoc{}) // empty zone
	iter := NewZoneLocIterator(ops, 100, 0, 3200, 3300)
	s := newLocFindNextState(t, 100, iter, 0)

	if err := handleLocFindNext(s); err != nil {
		t.Fatalf("handleLocFindNext: %v", err)
	}
	if got := s.PopInt(); got != 0 {
		t.Errorf("exhaustion: got push %d, want 0", got)
	}
	if s.ActiveLoc != nil {
		t.Error("ActiveLoc should remain nil on exhaustion")
	}
}

// TestLocFindNextStaleErrors pins the stale-iterator error path:
// creationTick=99, currentTick=100 → handler returns the canonical
// "tried to use an old iterator" error matching the NPC family wording.
func TestLocFindNextStaleErrors(t *testing.T) {
	loc := fakeActiveLoc{id: 100}
	ops := newLocIterTestOps([]ActiveLoc{loc})
	iter := NewZoneLocIterator(ops, 99, 0, 3200, 3300) // creationTick=99
	s := newLocFindNextState(t, 100, iter, 0)          // currentTick=100

	err := handleLocFindNext(s)
	if err == nil {
		t.Fatal("stale: want error, got nil")
	}
	want := "LOC_FINDNEXT: tried to use an old iterator. Create a new iterator instead."
	if err.Error() != want {
		t.Errorf("err: got %q, want %q", err.Error(), want)
	}
}
```

### - [ ] Step 2.3: Run new tests to verify they fail to compile

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestLocFindAllZone|TestLocFindNext' 2>&1 | head -25`

Expected: FAIL with `undefined: handleLocFindAllZone` / `undefined: handleLocFindNext`.

### - [ ] Step 2.4: Add `setActiveLocSlot` helper and the two handlers

In `pkg/script/handlers_loc.go`, find this block (currently at lines 13-18):

```go
func requireActiveLoc(s *ScriptState, op string) error {
	if s.ActiveLoc == nil {
		return fmt.Errorf("%s: no active loc", op)
	}
	return nil
}
```

Insert immediately after that block (and before the existing `handleLocFind`):

```go
// setActiveLocSlot writes the loc to either ActiveLoc (primary) or
// OtherActiveLoc (secondary) based on the handler's IntOperand and sets
// the corresponding Pointer flag. Mirrors TS
// state.pointerAdd(ActiveLoc[state.intOperand]) at LocOps.ts:110, and
// the parallel setActiveNpcSlot at handlers_npc.go:64-83.
//
// IntOperand==0 → ActiveLoc/PtrActiveLoc (.loc syntax).
// IntOperand==1 → OtherActiveLoc/PtrActiveLoc2 (.loc2 syntax).
// Any other value panics (compiler invariant — bytecode only emits 0/1).
func setActiveLocSlot(s *ScriptState, loc ActiveLoc) {
	operand := s.Script.IntOperands[s.PC]
	switch operand {
	case 0:
		s.ActiveLoc = loc
		s.Pointers |= PtrActiveLoc
	case 1:
		s.OtherActiveLoc = loc
		s.Pointers |= PtrActiveLoc2
	default:
		panic(fmt.Sprintf("setActiveLocSlot: invalid IntOperand %d", operand))
	}
}

// handleLocFindAllZone (LOC_FINDALLZONE, opcode 3008) pops a coord,
// validates, and stores a single-zone LocIterator targeting the zone
// containing that coord. Mirrors TS LocOps.ts:96-100. No
// distance/category/type filtering (TS LocIterator is single-mode).
//
// Nil-LocOps degrades silently (matches NPC_FINDALLZONE convention at
// handlers_npc.go:714-716).
func handleLocFindAllZone(s *ScriptState) error {
	coord := s.PopInt()
	level, x, z, err := checkCoord(coord, "LOC_FINDALLZONE")
	if err != nil {
		return err
	}
	if s.LocOps == nil {
		return nil
	}
	s.locIterator = NewZoneLocIterator(s.LocOps, s.World.CurrentTick(), level, x, z)
	return nil
}

// handleLocFindNext (LOC_FINDNEXT, opcode 3009) advances the active
// LocIterator and either sets active_loc + pushes 1 on hit, or pushes 0
// on miss / nil-iterator. Mirrors TS LocOps.ts:102-112.
//
// Stale-iterator semantics: mirror NPC_FINDNEXT (handlers_npc.go:778-795)
// — return error on stale; existing runtime path catches and clears the
// active script (parallel to npc_script.go:167-172).
//
// Pointer-set: setActiveLocSlot threads IntOperand 0/1 to choose
// primary/secondary slot per TS state.pointerAdd(ActiveLoc[intOperand]).
//
// Exhaustion does NOT clear s.locIterator (matches NPC family —
// handlers_npc.go:769-771). Subsequent FINDNEXT calls continue to
// return push-0.
func handleLocFindNext(s *ScriptState) error {
	it := s.locIterator
	if it == nil {
		s.PushInt(0)
		return nil
	}
	if it.Stale(s.World.CurrentTick()) {
		return fmt.Errorf("LOC_FINDNEXT: tried to use an old iterator. Create a new iterator instead.")
	}
	loc, ok := it.Next()
	if !ok {
		s.PushInt(0)
		return nil
	}
	setActiveLocSlot(s, loc)
	s.PushInt(1)
	return nil
}
```

### - [ ] Step 2.5: Add dispatch entries

In `pkg/script/handlers.go`, find this line (currently line 138):

```go
	OpLocFind:  handleLocFind,
```

Replace it with these three lines (preserving alignment):

```go
	OpLocFind:        handleLocFind,
	OpLocFindAllZone: handleLocFindAllZone,
	OpLocFindNext:    handleLocFindNext,
```

### - [ ] Step 2.6: Run handler tests to verify green

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... -run 'TestLocFindAllZone|TestLocFindNext' -v 2>&1 | tail -30`

Expected: All 8 new handler tests PASS.

### - [ ] Step 2.7: Run full pkg/script suite for regressions

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...`

Expected: `ok  github.com/zsrv/goscape/pkg/script` — clean.

### - [ ] Step 2.8: Run full repo test suite

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: All packages clean.

### - [ ] Step 2.9: Run vet

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: clean.

### - [ ] Step 2.10: Commit Task 2

```bash
git add pkg/script/handlers_loc.go pkg/script/handlers.go pkg/script/handlers_loc_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-119 T2 — LOC_FINDALLZONE + LOC_FINDNEXT handlers

Wires opcodes 3008 (LOC_FINDALLZONE) and 3009 (LOC_FINDNEXT) through the
dispatch table. Mirrors TS LocOps.ts:96-112 and the parallel goscape
NPC_FINDALLZONE / NPC_FINDNEXT pair (handlers_npc.go:704-795).

Adds setActiveLocSlot helper threading bytecode IntOperand 0/1 to the
ActiveLoc/OtherActiveLoc primary/secondary slot, parallel to
setActiveNpcSlot (handlers_npc.go:64-83). Closes NAI-119's dual-slot
plan.

Tests pin: iterator-storage, nil-LocOps degradation, checkCoord error,
nil-iterator FINDNEXT, primary/secondary slot population, exhaustion
push-0, stale-iterator error matching NPC family wording.

Spec: docs/superpowers/specs/2026-05-07-nai-119-loc-find-iterator-family-design.md
Plan: docs/superpowers/plans/2026-05-07-nai-119-loc-find-iterator-family.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Combined final review

This is a single review pass across both Task 1 and Task 2 commits — NOT a per-task two-stage review (per cadence B).

### - [ ] Step 3.1: Verify both Task commits landed

Run: `git log --oneline -3`

Expected:
```
<sha>  feat(script): NAI-119 T2 — LOC_FINDALLZONE + LOC_FINDNEXT handlers
<sha>  feat(script): NAI-119 T1 — LocIterator + ScriptState fields
f0454ef docs(spec): NAI-119 — LOC_FIND iterator family port (LOC_FINDALLZONE + LOC_FINDNEXT)
```

### - [ ] Step 3.2: Verify aggregate file scope

Run: `git diff --stat f0454ef..HEAD -- pkg/`

Expected (approximate):
```
 pkg/script/handlers.go            |  3 ++-
 pkg/script/handlers_loc.go        | ~70 +++++++++++++++++
 pkg/script/handlers_loc_test.go   | ~180 ++++++++++++++++++++++++++++
 pkg/script/loc_iterator.go        | ~70 +++++++++++++
 pkg/script/loc_iterator_test.go   | ~120 ++++++++++++++++++
 pkg/script/state.go               | ~16 ++++
 6 files changed, ~459 insertions(+), 1 deletion(-)
```

Tolerate ±20% on line counts (comments may render with different wrapping). If a file outside `pkg/script/` shows up, surface that as a finding.

### - [ ] Step 3.3: Code-level review against the spec

Read the spec at `docs/superpowers/specs/2026-05-07-nai-119-loc-find-iterator-family-design.md` and confirm:

- §3.2 missing pieces: all 7 items present in the diff.
- §4.1 LocIterator code: matches the new `pkg/script/loc_iterator.go` verbatim (modulo final-formatting).
- §4.2 ScriptState fields: both fields present at the spec'd insertion points.
- §4.3 handlers + helper: present in `handlers_loc.go`.
- §4.4 dispatch: both entries present in `handlers.go`.
- §5 tests: all 7 iterator-level + 8 handler-level tests present (15 total).
- §6 deviation `NAI-119-D-NO-DOWNSTREAM-LOC2-CONSUMERS`: documented in the `OtherActiveLoc` field doc comment in `state.go`.

Surface any discrepancies as findings (do not fix in this step — finding-only).

### - [ ] Step 3.4: Fresh full-repo verification

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all packages clean.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: clean.

### - [ ] Step 3.5: Self-review summary

Output a final summary:

- ✅ / ❌ Spec coverage (per Step 3.3 checklist)
- ✅ / ❌ Test count (15 new tests; both files)
- ✅ / ❌ `go test ./...` clean (per Step 3.4)
- ✅ / ❌ `go vet ./...` clean (per Step 3.4)
- ✅ / ❌ File scope matches expectation (per Step 3.2)
- Any concerns or DONE_WITH_CONCERNS escalations.

If all five pass: report DONE; controller hands off to user for the §8 smoke (mining-area gate opens; player walks to combat area).

If any fail: report DONE_WITH_CONCERNS with specifics; controller decides whether to dispatch a fix or escalate to the user.

---

## Post-execution: smoke handoff

After Task 3 passes, the controller hands off to the user for the spec §8 smoke per `smoke_test_server_handoff`. Expected smoke result:

- `[proc,tut_open_mining_gate]` no longer errors with `no handler for LOC_FINDALLZONE`.
- The mining-area exit gate visually opens and emits the `door_open` sound.
- Player can walk through to the combat area.

On smoke bind: close commit `chore(close): NAI-119 — final close after smoke binding` per `close_commit_memory_trailer` with `Closes memory:` trailer.

On smoke residual: route per `smoke_surfaces_adjacent_divergences` (in-scope-stretch ≤30 LOC, else NAI-120).
