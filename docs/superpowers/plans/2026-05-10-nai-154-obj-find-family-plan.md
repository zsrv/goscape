# NAI-154 — OBJ_FIND family port (3505/3506/3507/3508/3509) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port 5 OBJ_FIND-family opcodes (OBJ_FIND 3505, OBJ_FINDALLZONE 3506, OBJ_FINDNEXT 3507, OBJ_NAME 3508, OBJ_PARAM 3509) from `Engine-TS/src/engine/script/handlers/ObjOps.ts:95-201`. Closes cascade-tail 34 → 29.

**Architecture:** Extend `pkg/script.ScriptState` with `OtherActiveObj` field, `objIterator` field, and `WorldVars.GetObj` + `WorldVars.ZoneObjs` interface methods. New `pkg/script.ObjIterator` type mirrors NAI-119 `LocIterator`. `setActiveObjSlot` helper mirrors `setActiveLocSlot`. Production-side `worldVarsView` delegates to existing `Server.GetObj` (`modules/world/obj_lookup.go:13`) and reads `Zone.Objs` directly. 5 handlers in `pkg/script/handlers_obj.go`; dispatch entries in `pkg/script/handlers.go`.

**Tech Stack:** Go 1.26+. Touches `pkg/script` (state, helpers, handlers, new iterator file, dispatch) and `modules/world/server_varp.go` (WorldVars impl). Pure additive; no protocol-layer changes; no new dependencies.

**Spec:** `docs/superpowers/specs/2026-05-10-nai-154-obj-find-family-design.md` (commit `ac0c9fd`).

**Cadence:** Cadence B (mid-band, ~350-450 LOC) — 5 sequential T-tasks dispatched as Sonnet implementer subagents per `superpowers:subagent-driven-development`; single combined Sonnet reviewer at end (NOT per-task two-stage). Implementer runs MUST be on Sonnet per `superpowers_code_reviewer_model`. `/clear` between plan and impl per `superpowers_clear_between_spec_and_impl`.

---

## File Structure

**New files:**
- `pkg/script/obj_iterator.go` — `ObjIterator` struct + `NewZoneObjIterator` constructor + `Stale` + `Next`. Mirrors `pkg/script/loc_iterator.go`.
- `pkg/script/obj_iterator_test.go` — 7 unit tests for the iterator. Mirrors `pkg/script/loc_iterator_test.go`.

**Modified files:**
- `pkg/script/state.go` — Add `OtherActiveObj ActiveObj` field after line 306; add `objIterator *ObjIterator` field after line 324 (adjacent to `locIterator`); add `GetObj` + `ZoneObjs` methods to `WorldVars` interface after `EnqueueObjDelayed` (around line 128).
- `pkg/script/handlers_obj.go` — Add `setActiveObjSlot` helper after `requireActiveObj`; add five new handler functions (`handleObjFind`, `handleObjFindAllZone`, `handleObjFindNext`, `handleObjName`, `handleObjParam`).
- `pkg/script/handlers.go` — Add 5 dispatch map entries.
- `pkg/script/handlers_obj_test.go` — Add ~27 new test functions and a `newObjFindState` / `newObjFindAllZoneState` / `newObjFindNextState` builder trio.
- `pkg/script/handlers_vars_test.go` — Add 2 default no-op stubs to `mockWorld` for `GetObj` and `ZoneObjs`.
- `modules/world/server_varp.go` — Add `GetObj` + `ZoneObjs` impls on `worldVarsView` (production-side `WorldVars` adapter).

**Test-helper extensions:**
- Extend `mockActiveObj` (if needed) — already has `ObjType()`, `Coords()`, `ObjCount()`, `IsValidFor` from NAI-153 at `pkg/script/handlers_npc_test.go:2422-2441`. No structural change expected.
- Extend `mockWorld` (`pkg/script/handlers_vars_test.go:11-89`) with `GetObj` + `ZoneObjs` default no-op stubs. Recording-wrapper types (`fakeWorldObjFind`, `fakeWorldZoneObjs`) defined per-test-task in `handlers_obj_test.go` mirroring existing `fakeWorldAddObj` precedent.

---

## Setup

- [ ] **Step S.1: Verify baseline at HEAD `5d07235`**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./modules/world/...
```

Expected: PASS. Capture any pre-existing failures for the reviewer to attribute correctly (per `verify_implementer_claims`).

- [ ] **Step S.2: Confirm spec anchors at HEAD**

Run these greps and verify each returns the cited line:
```bash
rg -n 'OpObjFind\s+Opcode\s*=\s*3505' pkg/script/opcode.go
rg -n 'PtrActiveObj2\s+Pointer\s*=\s*1 << 7' pkg/script/pointer.go
rg -n '^\s+ActiveObj ActiveObj$' pkg/script/state.go
rg -n 'func \(s \*Server\) GetObj' modules/world/obj_lookup.go
rg -n 'type ObjIterator' pkg/script/  # expect EMPTY (no file exists)
rg -n 'OtherActiveObj' pkg/script/state.go  # expect EMPTY
rg -n 'func \(w worldVarsView\) GetObj' modules/world/server_varp.go  # expect EMPTY
```

Expected:
- `OpObjFind` line ~313, `PtrActiveObj2` line 15, `ActiveObj ActiveObj` line 306, `Server.GetObj` line 13.
- Three empty greps confirming missing surface.

---

## Task T1: ScriptState surface + helper + WorldVars interface extension

**Why first:** All downstream tasks reference these surfaces. No new tests in T1 — they exist as compile-only changes plus a `mockWorld` stub extension to keep the existing test suite passing.

**Files:**
- Modify: `pkg/script/state.go` (3 additions)
- Modify: `pkg/script/handlers_obj.go` (`setActiveObjSlot` helper)
- Modify: `pkg/script/handlers_vars_test.go` (2 stub additions to `mockWorld`)
- Modify: `modules/world/server_varp.go` (2 no-op-or-delegating impls — full impls in T2 but stubs must compile in T1)

- [ ] **Step T1.1: Add `OtherActiveObj` field to `ScriptState`**

Open `pkg/script/state.go`. Find the line `ActiveObj ActiveObj` (around line 306). Add immediately after it:

```go
// OtherActiveObj is the secondary Obj slot, parallel to OtherActiveLoc
// (NAI-119) and OtherActiveNpc (NAI-11). Set by OBJ_FIND / OBJ_FINDNEXT
// when the bytecode IntOperand is 1 (.obj2 syntax). NAI-154.
//
// NAI-154-D-NO-DOWNSTREAM-OBJ2-CONSUMERS: no existing OBJ_* read handler
// reads from this slot at HEAD — they all read s.ActiveObj only. Tracked
// deviation, mirrors NAI-119-D-NO-DOWNSTREAM-LOC2-CONSUMERS. Closure
// when a `.obj2` content-script consumer surfaces.
OtherActiveObj ActiveObj
```

- [ ] **Step T1.2: Add `objIterator` field to `ScriptState`**

In the same file, find the `locIterator *LocIterator` field (around line 324). Add immediately after it:

```go
// objIterator holds the active OBJ_FIND iterator state. Set by
// OBJ_FINDALLZONE; consumed by OBJ_FINDNEXT. Lifetime is single-tick —
// Stale() check enforced at FINDNEXT against s.World.CurrentTick().
// Nil = no active iterator. Mirrors TS ScriptState.objIterator. NAI-154.
objIterator *ObjIterator
```

Note: this references `*ObjIterator` which doesn't exist until T2. T1 alone will not compile — that's expected; T2 adds the type. To keep T1 commits green, see step T1.7 below for the temporary forward declaration approach.

**Revised approach:** Skip the field add here; do it in T2 alongside the iterator type. (This is the cleaner path — keeps each task's commit green.)

**Replace this step with:** _Skip. T2 adds `objIterator` field together with the iterator type._

- [ ] **Step T1.3: Add `WorldVars.GetObj` and `WorldVars.ZoneObjs` method declarations**

In `pkg/script/state.go`, find the `WorldVars` interface (line 54). The `EnqueueObjDelayed` method ends around line 128 with closing `)` on its own line. Add the two new methods immediately after `EnqueueObjDelayed` and before `LookupPlayerByUID`:

```go
// GetObj returns the first ground obj at (level, x, z) whose type
// matches objId and is visible to the caller. receiverUID is the
// player UID gating private-receiver visibility (NAI-153-D2 — goscape
// uses player UID where TS uses hash64). Returns nil on miss. Mirrors
// TS World.getObj consumed via OBJ_FIND. NAI-154.
GetObj(level, x, z, objId, receiverUID int) ActiveObj

// ZoneObjs returns every obj in the zone owning (level, zoneX, zoneZ),
// in storage order, without per-tile or per-receiver filtering. The
// caller (OBJ_FINDNEXT) applies its own validity gates as needed.
// Mirrors TS Zone.getAllObjsSafe(true) consumed by ObjIterator.generator
// (ScriptIterators.ts:400). Empty/nil slice on miss. NAI-154.
ZoneObjs(level, zoneX, zoneZ int) []ActiveObj
```

- [ ] **Step T1.4: Add `setActiveObjSlot` helper**

Open `pkg/script/handlers_obj.go`. Find the `requireActiveObj` function (around line 12). Add immediately after it:

```go
// setActiveObjSlot writes the obj to either ActiveObj (primary) or
// OtherActiveObj (secondary) based on the handler's IntOperand and sets
// the corresponding Pointer flag. Mirrors TS
// state.pointerAdd(ActiveObj[state.intOperand]) at ObjOps.ts:91, 181,
// 199, and the parallel setActiveLocSlot at handlers_loc.go:29-40.
//
// IntOperand==0 → ActiveObj/PtrActiveObj (.obj syntax).
// IntOperand==1 → OtherActiveObj/PtrActiveObj2 (.obj2 syntax).
// Any other value panics (compiler invariant — bytecode only emits 0/1).
func setActiveObjSlot(s *ScriptState, obj ActiveObj) {
    operand := s.Script.IntOperands[s.PC]
    switch operand {
    case 0:
        s.ActiveObj = obj
        s.Pointers |= PtrActiveObj
    case 1:
        s.OtherActiveObj = obj
        s.Pointers |= PtrActiveObj2
    default:
        panic(fmt.Sprintf("setActiveObjSlot: invalid IntOperand %d", operand))
    }
}
```

Verify `fmt` is already imported in `handlers_obj.go`. If not, add it to the import block.

- [ ] **Step T1.5: Add default no-op `GetObj` and `ZoneObjs` stubs to `mockWorld`**

Open `pkg/script/handlers_vars_test.go`. After the `EnqueueObjDelayed` stub (around line 89), add:

```go
// NAI-154: default no-op stubs for OBJ_FIND / OBJ_FINDALLZONE test
// fixtures. Tests exercising these override via fakeWorldObjFind /
// fakeWorldZoneObjs wrappers defined in handlers_obj_test.go.
func (m *mockWorld) GetObj(level, x, z, objId, receiverUID int) ActiveObj {
    return nil
}

func (m *mockWorld) ZoneObjs(level, zoneX, zoneZ int) []ActiveObj {
    return nil
}
```

- [ ] **Step T1.6: Add production-side stubs to `worldVarsView`**

Open `modules/world/server_varp.go`. After the existing `LookupNpcBySlot` method (around line 246, end of `func (w worldVarsView) LookupNpcBySlot`), add:

```go
// GetObj implements script.WorldVars.GetObj. Delegates to the existing
// Server.GetObj at modules/world/obj_lookup.go:13 (already consumed by
// modules/world/handler_opobj.go for OPOBJ-family handlers). The
// returned *entity.Obj implements script.ActiveObj via the NAI-153
// surface extension. Returns nil when no matching obj is at the tile
// or the caller lacks visibility. NAI-154.
func (w worldVarsView) GetObj(level, x, z, objId, receiverUID int) script.ActiveObj {
    if w.s == nil {
        return nil
    }
    o := w.s.GetObj(level, x, z, objId, receiverUID)
    if o == nil {
        return nil
    }
    return o
}

// ZoneObjs implements script.WorldVars.ZoneObjs. Reads the zone's Objs
// slice directly via zoneMap.Get and adapts each *entity.Obj to
// script.ActiveObj. Mirrors serverLocOps.AllLocsInZone at
// modules/world/script_loc_ops.go:85-92. Empty zone or out-of-range
// returns nil/empty. NAI-154.
func (w worldVarsView) ZoneObjs(level, zoneX, zoneZ int) []script.ActiveObj {
    if w.s == nil {
        return nil
    }
    z := w.s.zoneMap.Get(level, zoneX, zoneZ)
    if z == nil {
        return nil
    }
    out := make([]script.ActiveObj, 0, len(z.Objs))
    for _, o := range z.Objs {
        out = append(out, o)
    }
    return out
}
```

Verify `script` import (`github.com/zsrv/goscape/pkg/script`) is present in `server_varp.go` — it should be (used by the other methods).

- [ ] **Step T1.7: Build verification**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: PASS (clean build). The new `WorldVars.GetObj` and `WorldVars.ZoneObjs` methods are satisfied by both `mockWorld` (stub) and `worldVarsView` (delegating impl).

If any OTHER `WorldVars` implementer exists in the codebase, add stubs there too. Grep:
```bash
rg -n 'func \(.*\) CurrentTick\(\) int' --type go
```
Each impl that satisfies `WorldVars` needs the two new methods. Add no-op stubs to any test file's WorldVars implementer found.

- [ ] **Step T1.8: Run existing test suite**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./modules/world/...
```

Expected: PASS. No new tests yet; existing suite must remain green.

- [ ] **Step T1.9: Commit T1**

```bash
git add pkg/script/state.go pkg/script/handlers_obj.go pkg/script/handlers_vars_test.go modules/world/server_varp.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-154 T1 — OtherActiveObj field + setActiveObjSlot + WorldVars OBJ-find surface

Add OtherActiveObj field to ScriptState (parallel to OtherActiveLoc),
WorldVars.GetObj + ZoneObjs interface methods, setActiveObjSlot helper,
mockWorld default stubs, and worldVarsView production impls delegating
to Server.GetObj and Zone.Objs. Compile-only; handlers + iterator land
in T2-T5.

Tracks NAI-154-D-NO-DOWNSTREAM-OBJ2-CONSUMERS (mirrors NAI-119 parallel).
EOF
)"
```

---

## Task T2: ObjIterator type + tests

**Files:**
- Create: `pkg/script/obj_iterator.go`
- Create: `pkg/script/obj_iterator_test.go`
- Modify: `pkg/script/state.go` (add `objIterator *ObjIterator` field — deferred from T1)

- [ ] **Step T2.1: Write the failing iterator unit tests**

Create `pkg/script/obj_iterator_test.go`:

```go
package script

import "testing"

// objIterTestWorld is a mockWorld wrapper that returns a fixed slice
// from ZoneObjs. Independent of any inZone field so the iterator-test
// surface stays small. Mirrors locIterTestOps (loc_iterator_test.go).
type objIterTestWorld struct {
    *mockWorld
    zoneObjs []ActiveObj
}

func (w *objIterTestWorld) ZoneObjs(level, zoneX, zoneZ int) []ActiveObj {
    return w.zoneObjs
}

func newObjIterTestWorld(objs []ActiveObj) *objIterTestWorld {
    return &objIterTestWorld{
        mockWorld: newMockWorld(),
        zoneObjs:  objs,
    }
}

// TestNewZoneObjIteratorStoresFields pins the constructor: tick, level,
// x, z stored verbatim; iteration state (objs, idx, started) zeroed.
func TestNewZoneObjIteratorStoresFields(t *testing.T) {
    w := newObjIterTestWorld(nil)
    it := NewZoneObjIterator(w, 42, 0, 3200, 3300)
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
    if it.objs != nil {
        t.Error("objs: should be nil before first Next()")
    }
}

// TestObjIteratorStaleAtSameTick pins the strict-greater-than semantics
// of TS ScriptIterators.ts:401 (currentTick > creationTick).
func TestObjIteratorStaleAtSameTick(t *testing.T) {
    it := NewZoneObjIterator(newObjIterTestWorld(nil), 100, 0, 3200, 3300)
    if it.Stale(100) {
        t.Error("Stale(currentTick == creationTick): got true, want false")
    }
}

// TestObjIteratorStaleNextTick pins that any forward-tick advancement
// trips Stale().
func TestObjIteratorStaleNextTick(t *testing.T) {
    it := NewZoneObjIterator(newObjIterTestWorld(nil), 100, 0, 3200, 3300)
    if !it.Stale(101) {
        t.Error("Stale(currentTick > creationTick): got false, want true")
    }
}

// TestObjIteratorYieldsAllZoneObjs pins that Next() drains the snapshot
// in slice order and exhausts cleanly.
func TestObjIteratorYieldsAllZoneObjs(t *testing.T) {
    obj1 := &mockActiveObj{objType: 100, count: 1}
    obj2 := &mockActiveObj{objType: 101, count: 1}
    obj3 := &mockActiveObj{objType: 102, count: 1}
    w := newObjIterTestWorld([]ActiveObj{obj1, obj2, obj3})
    it := NewZoneObjIterator(w, 0, 0, 3200, 3300)

    got := []ActiveObj{}
    for {
        obj, ok := it.Next()
        if !ok {
            break
        }
        got = append(got, obj)
    }
    if len(got) != 3 {
        t.Fatalf("yield count: got %d, want 3", len(got))
    }
    if got[0].ObjType() != 100 || got[1].ObjType() != 101 || got[2].ObjType() != 102 {
        t.Errorf("yield order: got [%d, %d, %d], want [100, 101, 102]",
            got[0].ObjType(), got[1].ObjType(), got[2].ObjType())
    }
}

// TestObjIteratorEmptyZone pins that empty zones exhaust on first Next().
func TestObjIteratorEmptyZone(t *testing.T) {
    w := newObjIterTestWorld([]ActiveObj{})
    it := NewZoneObjIterator(w, 0, 0, 3200, 3300)
    if obj, ok := it.Next(); ok {
        t.Errorf("empty zone first Next: got (%v, true), want (nil, false)", obj)
    }
}

// TestObjIteratorExhaustionDoesNotClear pins TS LocIterator parallel:
// after exhaustion subsequent Next() calls keep returning (nil, false)
// without panic. Iterator is NOT nilled.
func TestObjIteratorExhaustionDoesNotClear(t *testing.T) {
    obj1 := &mockActiveObj{objType: 100, count: 1}
    w := newObjIterTestWorld([]ActiveObj{obj1})
    it := NewZoneObjIterator(w, 0, 0, 3200, 3300)

    if _, ok := it.Next(); !ok {
        t.Fatal("first Next: expected hit")
    }
    if _, ok := it.Next(); ok {
        t.Fatal("second Next: expected exhaustion")
    }
    for i := 0; i < 3; i++ {
        if obj, ok := it.Next(); ok || obj != nil {
            t.Errorf("post-exhaustion Next #%d: got (%v, %v), want (nil, false)", i, obj, ok)
        }
    }
}

// TestObjIteratorNilWorldDegrades pins the LocIterator parallel:
// nil WorldVars returns (nil, false) on first Next without panic.
func TestObjIteratorNilWorldDegrades(t *testing.T) {
    it := NewZoneObjIterator(nil, 0, 0, 3200, 3300)
    if obj, ok := it.Next(); ok || obj != nil {
        t.Errorf("nil world first Next: got (%v, %v), want (nil, false)", obj, ok)
    }
}
```

- [ ] **Step T2.2: Run test to verify it fails**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestObjIterator|TestNewZoneObjIterator' -v
```

Expected: FAIL with "undefined: ObjIterator" / "undefined: NewZoneObjIterator". Build error is acceptable here — test is red because the type doesn't exist.

- [ ] **Step T2.3: Create `pkg/script/obj_iterator.go`**

```go
package script

// ObjIterator is the script-VM iterator state for the OBJ_FIND iterator
// family (currently OBJ_FINDALLZONE only — single-mode like LocIterator,
// unlike NpcIterator's DISTANCE/ZONE/HuntAll). Mirrors TS ObjIterator at
// ScriptIterators.ts:387-407.
//
// Lifetime: single-tick. Created by OBJ_FINDALLZONE; consumed by
// OBJ_FINDNEXT. Stale() check at FINDNEXT compares creationTick to
// World.CurrentTick(); on mismatch, the handler returns an error
// mirroring the LOC family pattern.
//
// Snapshot strategy: lazy on first Next() call via
// WorldVars.ZoneObjs(level, x, z). TS uses a generator over
// `getZone(...).getAllObjsSafe(true)` — equivalent because both produce
// a single point-in-time slice that the iterator drains independent of
// subsequent zone mutation.
//
// Ownership: held by ScriptState.objIterator. Nil = no active iterator.
type ObjIterator struct {
    creationTick int
    world        WorldVars
    level, x, z  int
    objs         []ActiveObj
    idx          int
    started      bool
}

// NewZoneObjIterator constructs a single-zone iterator for the zone
// containing (level, x, z). Mirrors TS ObjIterator constructor at
// ScriptIterators.ts:392-396. The snapshot is deferred to first Next();
// the constructor only stores center coords and tick.
func NewZoneObjIterator(world WorldVars, tick, level, x, z int) *ObjIterator {
    return &ObjIterator{
        creationTick: tick,
        world:        world,
        level:        level,
        x:            x,
        z:            z,
    }
}

// Stale reports whether the iterator was created in a prior tick. The
// FINDNEXT handler MUST check this before calling Next when single-tick
// lifetime matters. Mirrors TS strict-greater-than at
// ScriptIterators.ts:401 (World.currentTick > this.tick).
func (it *ObjIterator) Stale(currentTick int) bool {
    return currentTick > it.creationTick
}

// Next returns the next obj in the zone snapshot, or (nil, false) on
// exhaustion. Lazy-initializes the snapshot on first call.
//
// Nil-world degrades to immediate exhaustion (test stub or pre-wiring) —
// mirrors LocIterator.Next nil-ops handling (loc_iterator.go).
func (it *ObjIterator) Next() (ActiveObj, bool) {
    if !it.started {
        it.started = true
        if it.world != nil {
            it.objs = it.world.ZoneObjs(it.level, it.x, it.z)
        }
    }
    if it.idx >= len(it.objs) {
        return nil, false
    }
    obj := it.objs[it.idx]
    it.idx++
    return obj, true
}
```

- [ ] **Step T2.4: Add `objIterator *ObjIterator` field to `ScriptState`**

In `pkg/script/state.go`, find the `locIterator *LocIterator` field (around line 324, immediately preceded by the doc-comment paragraph for `locIterator`). Add immediately after it:

```go
// objIterator holds the active OBJ_FIND iterator state. Set by
// OBJ_FINDALLZONE; consumed by OBJ_FINDNEXT. Lifetime is single-tick —
// Stale() check enforced at FINDNEXT against s.World.CurrentTick().
// Nil = no active iterator. Mirrors TS ScriptState.objIterator. NAI-154.
objIterator *ObjIterator
```

- [ ] **Step T2.5: Run test to verify it passes**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestObjIterator|TestNewZoneObjIterator' -v
```

Expected: PASS — all 7 tests green.

- [ ] **Step T2.6: Run full test suite**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./modules/world/...
```

Expected: PASS.

- [ ] **Step T2.7: Commit T2**

```bash
git add pkg/script/obj_iterator.go pkg/script/obj_iterator_test.go pkg/script/state.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-154 T2 — ObjIterator type + objIterator state field

Add pkg/script/obj_iterator.go mirroring loc_iterator.go (lazy
zone-snapshot via WorldVars.ZoneObjs, Stale check on
currentTick > creationTick, nil-world degrades silently). 7 unit
tests pin constructor, stale semantics, yield order, empty zone,
exhaustion, nil-world degradation.

Add s.objIterator *ObjIterator field on ScriptState (parallel to
locIterator). Consumed by OBJ_FINDALLZONE / OBJ_FINDNEXT in T4.
EOF
)"
```

---

## Task T3: OBJ_FIND handler + dispatch + tests

**Files:**
- Modify: `pkg/script/handlers_obj.go` (`handleObjFind`)
- Modify: `pkg/script/handlers.go` (dispatch entry)
- Modify: `pkg/script/handlers_obj_test.go` (7 tests + `fakeWorldObjFind` recorder + `newObjFindState` builder)

- [ ] **Step T3.1: Write the failing OBJ_FIND tests**

Append to `pkg/script/handlers_obj_test.go`:

```go
// --- NAI-154: OBJ_FIND handler tests ---------------------------------

// objFindCall records a single WorldVars.GetObj invocation.
type objFindCall struct {
    level, x, z, objId, receiverUID int
}

// fakeWorldObjFind extends mockWorld to record GetObj calls and return a
// preset result. Mirrors fakeWorldAddObj.
type fakeWorldObjFind struct {
    *mockWorld
    result    ActiveObj
    calls     []objFindCall
}

func (w *fakeWorldObjFind) GetObj(level, x, z, objId, receiverUID int) ActiveObj {
    w.calls = append(w.calls, objFindCall{level, x, z, objId, receiverUID})
    return w.result
}

func newFakeWorldObjFind(result ActiveObj) *fakeWorldObjFind {
    return &fakeWorldObjFind{mockWorld: newMockWorld(), result: result}
}

// newObjFindState builds a ScriptState ready for handleObjFind: World
// wired, Configs wired with the given objId registered, Self+UID
// populated, IntOperand set for slot routing, and the coord+objId
// pre-pushed onto the int stack (bottom-up matches popInts semantics:
// coord pushed first, objId pushed second).
func newObjFindState(t *testing.T, w WorldVars, mc *mockConfigs, intOperand int32, coord, objId, uid int) *ScriptState {
    t.Helper()
    s := &ScriptState{
        Script:      &ScriptFile{IntOperands: []int32{intOperand}},
        PC:          0,
        World:       w,
        Configs:     mc,
        IntStack:    make([]int, StackCapacity),
        StringStack: make([]string, StackCapacity),
    }
    s.Self = &mockPlayer{uidValue: uid}
    s.Pointers |= PtrActivePlayer
    s.PushInt(coord)
    s.PushInt(objId)
    return s
}

// TestObjFindHitPrimarySlot pins OBJ_FIND IntOperand=0: hit sets
// s.ActiveObj, sets PtrActiveObj, pushes 1.
func TestObjFindHitPrimarySlot(t *testing.T) {
    obj := &mockActiveObj{objType: 590, x: 3200, z: 3300, level: 0, count: 1}
    w := newFakeWorldObjFind(obj)
    mc := withObjForObjAdd(newTestConfigs(), 590, true, false, 0)
    coord := coordgrid.PackCoord(0, 3200, 3300)
    s := newObjFindState(t, w, mc, 0 /*intOperand*/, coord, 590, 12345)

    if err := handleObjFind(s); err != nil {
        t.Fatalf("handleObjFind: %v", err)
    }
    if got := s.PopInt(); got != 1 {
        t.Errorf("push: got %d, want 1 (hit)", got)
    }
    if s.ActiveObj != obj {
        t.Errorf("ActiveObj: got %v, want %v", s.ActiveObj, obj)
    }
    if s.Pointers&PtrActiveObj == 0 {
        t.Error("PtrActiveObj not set")
    }
    if s.OtherActiveObj != nil {
        t.Errorf("OtherActiveObj: got %v, want nil (primary slot)", s.OtherActiveObj)
    }
}

// TestObjFindHitSecondarySlot pins OBJ_FIND IntOperand=1: hit sets
// s.OtherActiveObj, sets PtrActiveObj2, pushes 1.
func TestObjFindHitSecondarySlot(t *testing.T) {
    obj := &mockActiveObj{objType: 590, x: 3200, z: 3300, level: 0, count: 1}
    w := newFakeWorldObjFind(obj)
    mc := withObjForObjAdd(newTestConfigs(), 590, true, false, 0)
    coord := coordgrid.PackCoord(0, 3200, 3300)
    s := newObjFindState(t, w, mc, 1 /*intOperand*/, coord, 590, 12345)

    if err := handleObjFind(s); err != nil {
        t.Fatalf("handleObjFind: %v", err)
    }
    if got := s.PopInt(); got != 1 {
        t.Errorf("push: got %d, want 1", got)
    }
    if s.OtherActiveObj != obj {
        t.Errorf("OtherActiveObj: got %v, want %v", s.OtherActiveObj, obj)
    }
    if s.Pointers&PtrActiveObj2 == 0 {
        t.Error("PtrActiveObj2 not set")
    }
    if s.ActiveObj != nil {
        t.Errorf("ActiveObj: got %v, want nil (secondary slot)", s.ActiveObj)
    }
}

// TestObjFindMissPushesZero pins the nil-result branch: pushes 0, leaves
// ActiveObj/OtherActiveObj untouched.
func TestObjFindMissPushesZero(t *testing.T) {
    w := newFakeWorldObjFind(nil)
    mc := withObjForObjAdd(newTestConfigs(), 590, true, false, 0)
    coord := coordgrid.PackCoord(0, 3200, 3300)
    s := newObjFindState(t, w, mc, 0, coord, 590, 12345)

    if err := handleObjFind(s); err != nil {
        t.Fatalf("handleObjFind: %v", err)
    }
    if got := s.PopInt(); got != 0 {
        t.Errorf("push: got %d, want 0 (miss)", got)
    }
    if s.ActiveObj != nil {
        t.Errorf("ActiveObj: got %v, want nil on miss", s.ActiveObj)
    }
}

// TestObjFindRequiresActivePlayer pins the requireActivePlayer guard:
// no Self → error.
func TestObjFindRequiresActivePlayer(t *testing.T) {
    w := newFakeWorldObjFind(nil)
    mc := withObjForObjAdd(newTestConfigs(), 590, true, false, 0)
    coord := coordgrid.PackCoord(0, 3200, 3300)
    s := newObjFindState(t, w, mc, 0, coord, 590, 12345)
    // Clear the player.
    s.Self = nil
    s.Pointers &^= PtrActivePlayer

    err := handleObjFind(s)
    if err == nil {
        t.Fatal("handleObjFind: want error (no active player), got nil")
    }
    if !strings.Contains(err.Error(), "OBJ_FIND") {
        t.Errorf("err: got %q, want substring %q", err.Error(), "OBJ_FIND")
    }
}

// TestObjFindUnknownObjId pins the Configs lookup guard: unknown objId
// → error.
func TestObjFindUnknownObjId(t *testing.T) {
    w := newFakeWorldObjFind(nil)
    mc := newTestConfigs() // no 590 registered
    coord := coordgrid.PackCoord(0, 3200, 3300)
    s := newObjFindState(t, w, mc, 0, coord, 590, 12345)

    err := handleObjFind(s)
    if err == nil {
        t.Fatal("handleObjFind: want error (unknown obj id), got nil")
    }
    if !strings.Contains(err.Error(), "unknown obj id") {
        t.Errorf("err: got %q, want substring %q", err.Error(), "unknown obj id")
    }
}

// TestObjFindInvalidCoord pins the checkCoord guard: -1 → error.
func TestObjFindInvalidCoord(t *testing.T) {
    w := newFakeWorldObjFind(nil)
    mc := withObjForObjAdd(newTestConfigs(), 590, true, false, 0)
    s := newObjFindState(t, w, mc, 0, -1 /*coord*/, 590, 12345)

    err := handleObjFind(s)
    if err == nil {
        t.Fatal("handleObjFind: want error (invalid coord), got nil")
    }
    if !strings.Contains(err.Error(), "OBJ_FIND") {
        t.Errorf("err: got %q, want substring %q", err.Error(), "OBJ_FIND")
    }
}

// TestObjFindUIDPropagation pins NAI-153-D2: receiverUID passed to
// WorldVars.GetObj is s.Self.UID() (goscape player UID, not TS hash64).
func TestObjFindUIDPropagation(t *testing.T) {
    obj := &mockActiveObj{objType: 590, count: 1}
    w := newFakeWorldObjFind(obj)
    mc := withObjForObjAdd(newTestConfigs(), 590, true, false, 0)
    coord := coordgrid.PackCoord(0, 3200, 3300)
    s := newObjFindState(t, w, mc, 0, coord, 590, 98765 /*uid*/)

    if err := handleObjFind(s); err != nil {
        t.Fatalf("handleObjFind: %v", err)
    }
    if len(w.calls) != 1 {
        t.Fatalf("GetObj calls: got %d, want 1", len(w.calls))
    }
    got := w.calls[0]
    want := objFindCall{level: 0, x: 3200, z: 3300, objId: 590, receiverUID: 98765}
    if got != want {
        t.Errorf("GetObj call: got %+v, want %+v", got, want)
    }
}
```

- [ ] **Step T3.2: Run test to verify failure**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestObjFind' -v
```

Expected: FAIL — "undefined: handleObjFind".

- [ ] **Step T3.3: Implement `handleObjFind`**

In `pkg/script/handlers_obj.go`, add the following handler. Placement: alphabetical or near the existing `handleObjAdd`. The handler follows the spec §4.5 exactly.

```go
// handleObjFind (OBJ_FIND, opcode 3505) pops [coord, objId], resolves
// the obj via WorldVars.GetObj, and either slot-routes it via
// setActiveObjSlot + pushes 1 on hit, or pushes 0 on miss. Mirrors TS
// ObjOps.ts:168-183.
//
// Pop order: objId is at the top of the stack (last pushed); coord
// below it. Matches TS `[coord, objId] = state.popInts(2)`.
//
// Receiver UID is s.Self.UID() per NAI-153-D2 (goscape UID vs TS hash64).
func handleObjFind(s *ScriptState) error {
    if err := requireActivePlayer(s, "OBJ_FIND"); err != nil {
        return err
    }
    if err := requireConfigs(s, "OBJ_FIND"); err != nil {
        return err
    }
    objId := s.PopInt()
    coord := s.PopInt()
    level, x, z, err := checkCoord(coord, "OBJ_FIND")
    if err != nil {
        return err
    }
    if s.Configs.ObjType(objId) == nil {
        return fmt.Errorf("OBJ_FIND: unknown obj id %d", objId)
    }
    if s.World == nil {
        s.PushInt(0)
        return nil
    }
    obj := s.World.GetObj(level, x, z, objId, s.Self.UID())
    if obj == nil {
        s.PushInt(0)
        return nil
    }
    setActiveObjSlot(s, obj)
    s.PushInt(1)
    return nil
}
```

- [ ] **Step T3.4: Wire dispatch in `handlers.go`**

Open `pkg/script/handlers.go`. Find an adjacent OBJ-family dispatch entry (look for `OpObjType:` or `OpObjCount:` or `OpObjTakeItem:`). Add:

```go
OpObjFind: handleObjFind,
```

The exact insertion line depends on the map's existing OBJ block; group with the existing OBJ entries to keep the file readable.

- [ ] **Step T3.5: Run tests to verify pass**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestObjFind' -v
```

Expected: PASS — all 7 OBJ_FIND tests green.

Run full suite:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./modules/world/...
```

Expected: PASS.

- [ ] **Step T3.6: Commit T3**

```bash
git add pkg/script/handlers_obj.go pkg/script/handlers.go pkg/script/handlers_obj_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-154 T3 — OBJ_FIND handler (3505) + dispatch

Port handleObjFind per TS ObjOps.ts:168-183: requireActivePlayer +
requireConfigs gates, checkCoord, ObjType-existence check, then
WorldVars.GetObj with s.Self.UID() as receiver (NAI-153-D2). On hit:
setActiveObjSlot routes ActiveObj (IntOperand=0) or OtherActiveObj
(IntOperand=1), pushes 1. On miss: pushes 0.

7 tests pin hit/miss × primary/secondary slot, requireActivePlayer
gate, unknown-objId guard, invalid-coord, and s.Self.UID()
propagation (NAI-153-D2 pin).
EOF
)"
```

---

## Task T4: OBJ_FINDALLZONE + OBJ_FINDNEXT handlers + dispatch + tests

**Files:**
- Modify: `pkg/script/handlers_obj.go` (`handleObjFindAllZone`, `handleObjFindNext`)
- Modify: `pkg/script/handlers.go` (2 dispatch entries)
- Modify: `pkg/script/handlers_obj_test.go` (8 tests + 2 builders)

- [ ] **Step T4.1: Write the failing iterator-handler tests**

Append to `pkg/script/handlers_obj_test.go`:

```go
// --- NAI-154: OBJ_FINDALLZONE + OBJ_FINDNEXT handler tests -----------

// newObjFindAllZoneState builds a ScriptState with a coord on the int
// stack, World wired (for CurrentTick), and IntOperands sized for the
// handler. Mirror of newLocFindAllZoneState (handlers_loc_test.go:876).
func newObjFindAllZoneState(t *testing.T, tick int, w WorldVars, coord int) *ScriptState {
    t.Helper()
    s := &ScriptState{
        Script:      &ScriptFile{IntOperands: []int32{0}},
        PC:          0,
        World:       w,
        IntStack:    make([]int, StackCapacity),
        StringStack: make([]string, StackCapacity),
    }
    s.PushInt(coord)
    return s
}

// newObjFindNextState builds a ScriptState with World wired (for
// CurrentTick), an optional objIterator pre-installed, and IntOperands
// supplied for setActiveObjSlot. Mirror of newLocFindNextState.
func newObjFindNextState(t *testing.T, tick int, iter *ObjIterator, intOperand int32) *ScriptState {
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
    s.objIterator = iter
    return s
}

// TestObjFindAllZoneStoresIterator pins OBJ_FINDALLZONE: pop coord →
// store iterator with creationTick from World.CurrentTick + level/x/z
// from coord.
func TestObjFindAllZoneStoresIterator(t *testing.T) {
    w := newObjIterTestWorld(nil)
    w.tick = 100
    coord := coordgrid.PackCoord(0, 3200, 3300)
    s := newObjFindAllZoneState(t, 100, w, coord)

    if err := handleObjFindAllZone(s); err != nil {
        t.Fatalf("handleObjFindAllZone: %v", err)
    }
    if s.objIterator == nil {
        t.Fatal("objIterator: got nil, want set")
    }
    if s.objIterator.creationTick != 100 {
        t.Errorf("creationTick: got %d, want 100 (from World.CurrentTick)",
            s.objIterator.creationTick)
    }
    if s.objIterator.level != 0 || s.objIterator.x != 3200 || s.objIterator.z != 3300 {
        t.Errorf("coord: got (%d, %d, %d), want (0, 3200, 3300)",
            s.objIterator.level, s.objIterator.x, s.objIterator.z)
    }
}

// TestObjFindAllZoneNilWorldDegrades pins the LocFindAllZone parallel:
// nil World → handler returns nil, objIterator stays nil.
func TestObjFindAllZoneNilWorldDegrades(t *testing.T) {
    coord := coordgrid.PackCoord(0, 3200, 3300)
    s := newObjFindAllZoneState(t, 100, nil, coord)
    s.World = nil

    if err := handleObjFindAllZone(s); err != nil {
        t.Fatalf("handleObjFindAllZone: got err %v, want nil (degrade silently)", err)
    }
    if s.objIterator != nil {
        t.Errorf("objIterator: got %v, want nil (no iterator on nil-world)", s.objIterator)
    }
}

// TestObjFindAllZoneCoordValid pins the checkCoord error path.
func TestObjFindAllZoneCoordValid(t *testing.T) {
    w := newObjIterTestWorld(nil)
    s := newObjFindAllZoneState(t, 100, w, -1)

    err := handleObjFindAllZone(s)
    if err == nil {
        t.Fatal("handleObjFindAllZone(-1): want error, got nil")
    }
    if !strings.Contains(err.Error(), "OBJ_FINDALLZONE") {
        t.Errorf("err: got %q, want substring %q", err.Error(), "OBJ_FINDALLZONE")
    }
}

// TestObjFindNextNoIterator pins the nil-iterator branch.
func TestObjFindNextNoIterator(t *testing.T) {
    s := newObjFindNextState(t, 100, nil, 0)

    if err := handleObjFindNext(s); err != nil {
        t.Fatalf("handleObjFindNext: %v", err)
    }
    if got := s.PopInt(); got != 0 {
        t.Errorf("nil iterator: got push %d, want 0", got)
    }
    if s.ActiveObj != nil {
        t.Error("ActiveObj should remain nil")
    }
    if s.OtherActiveObj != nil {
        t.Error("OtherActiveObj should remain nil")
    }
}

// TestObjFindNextHitPrimarySlot pins OBJ_FINDNEXT IntOperand=0: hit
// sets s.ActiveObj, sets PtrActiveObj, pushes 1.
func TestObjFindNextHitPrimarySlot(t *testing.T) {
    obj := &mockActiveObj{objType: 590, count: 1}
    w := newObjIterTestWorld([]ActiveObj{obj})
    iter := NewZoneObjIterator(w, 100, 0, 3200, 3300)
    s := newObjFindNextState(t, 100, iter, 0)

    if err := handleObjFindNext(s); err != nil {
        t.Fatalf("handleObjFindNext: %v", err)
    }
    if got := s.PopInt(); got != 1 {
        t.Errorf("push: got %d, want 1 (hit)", got)
    }
    if s.ActiveObj != obj {
        t.Errorf("ActiveObj: got %v, want %v", s.ActiveObj, obj)
    }
    if s.Pointers&PtrActiveObj == 0 {
        t.Error("PtrActiveObj not set")
    }
}

// TestObjFindNextHitSecondarySlot pins OBJ_FINDNEXT IntOperand=1: hit
// sets s.OtherActiveObj, sets PtrActiveObj2, pushes 1.
func TestObjFindNextHitSecondarySlot(t *testing.T) {
    obj := &mockActiveObj{objType: 590, count: 1}
    w := newObjIterTestWorld([]ActiveObj{obj})
    iter := NewZoneObjIterator(w, 100, 0, 3200, 3300)
    s := newObjFindNextState(t, 100, iter, 1)

    if err := handleObjFindNext(s); err != nil {
        t.Fatalf("handleObjFindNext: %v", err)
    }
    if got := s.PopInt(); got != 1 {
        t.Errorf("push: got %d, want 1", got)
    }
    if s.OtherActiveObj != obj {
        t.Errorf("OtherActiveObj: got %v, want %v", s.OtherActiveObj, obj)
    }
    if s.Pointers&PtrActiveObj2 == 0 {
        t.Error("PtrActiveObj2 not set")
    }
    if s.ActiveObj != nil {
        t.Error("ActiveObj should remain nil (secondary slot)")
    }
}

// TestObjFindNextExhaustionPushesZero pins the exhaustion path.
func TestObjFindNextExhaustionPushesZero(t *testing.T) {
    obj := &mockActiveObj{objType: 590, count: 1}
    w := newObjIterTestWorld([]ActiveObj{obj})
    iter := NewZoneObjIterator(w, 100, 0, 3200, 3300)
    // Drain.
    if _, ok := iter.Next(); !ok {
        t.Fatal("setup: iterator should yield once")
    }
    s := newObjFindNextState(t, 100, iter, 0)

    if err := handleObjFindNext(s); err != nil {
        t.Fatalf("handleObjFindNext: %v", err)
    }
    if got := s.PopInt(); got != 0 {
        t.Errorf("exhausted: got push %d, want 0", got)
    }
    if s.ActiveObj != nil {
        t.Error("ActiveObj should remain nil on exhaustion")
    }
}

// TestObjFindNextStaleErrors pins the stale-iterator guard: iterator
// created at tick=0, World.CurrentTick advanced to 1 → error.
func TestObjFindNextStaleErrors(t *testing.T) {
    obj := &mockActiveObj{objType: 590, count: 1}
    w := newObjIterTestWorld([]ActiveObj{obj})
    iter := NewZoneObjIterator(w, 0, 0, 3200, 3300) // tick=0
    s := newObjFindNextState(t, 1, iter, 0)         // World.tick=1

    err := handleObjFindNext(s)
    if err == nil {
        t.Fatal("handleObjFindNext on stale iterator: want error, got nil")
    }
    if !strings.Contains(err.Error(), "old iterator") {
        t.Errorf("err: got %q, want substring %q", err.Error(), "old iterator")
    }
}
```

- [ ] **Step T4.2: Run test to verify failure**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestObjFindAllZone|TestObjFindNext' -v
```

Expected: FAIL — "undefined: handleObjFindAllZone" / "undefined: handleObjFindNext".

- [ ] **Step T4.3: Implement the two handlers**

Add to `pkg/script/handlers_obj.go` (near `handleObjFind` from T3):

```go
// handleObjFindAllZone (OBJ_FINDALLZONE, opcode 3506) pops a coord and
// stores a single-zone ObjIterator targeting the zone containing that
// coord. Mirrors TS ObjOps.ts:185-189.
//
// Nil-World degrades silently (matches LOC_FINDALLZONE convention at
// handlers_loc.go).
func handleObjFindAllZone(s *ScriptState) error {
    coord := s.PopInt()
    level, x, z, err := checkCoord(coord, "OBJ_FINDALLZONE")
    if err != nil {
        return err
    }
    if s.World == nil {
        return nil
    }
    s.objIterator = NewZoneObjIterator(s.World, s.World.CurrentTick(), level, x, z)
    return nil
}

// handleObjFindNext (OBJ_FINDNEXT, opcode 3507) advances the active
// ObjIterator and either sets the active obj slot + pushes 1 on hit, or
// pushes 0 on miss / nil-iterator. Mirrors TS ObjOps.ts:191-201.
//
// Stale-iterator semantics mirror LOC_FINDNEXT — return error on stale.
// Pointer-set: setActiveObjSlot threads IntOperand 0/1 per TS
// state.pointerAdd(ActiveObj[intOperand]).
func handleObjFindNext(s *ScriptState) error {
    it := s.objIterator
    if it == nil {
        s.PushInt(0)
        return nil
    }
    if it.Stale(s.World.CurrentTick()) {
        return fmt.Errorf("OBJ_FINDNEXT: tried to use an old iterator. Create a new iterator instead.")
    }
    obj, ok := it.Next()
    if !ok {
        s.PushInt(0)
        return nil
    }
    setActiveObjSlot(s, obj)
    s.PushInt(1)
    return nil
}
```

- [ ] **Step T4.4: Wire dispatch**

In `pkg/script/handlers.go`, near the existing `OpObjFind: handleObjFind` from T3, add:

```go
OpObjFindAllZone: handleObjFindAllZone,
OpObjFindNext:    handleObjFindNext,
```

- [ ] **Step T4.5: Run tests to verify pass**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestObjFindAllZone|TestObjFindNext' -v
```

Expected: PASS — all 8 tests green.

Run full suite:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/... ./modules/world/...
```

Expected: PASS.

- [ ] **Step T4.6: Commit T4**

```bash
git add pkg/script/handlers_obj.go pkg/script/handlers.go pkg/script/handlers_obj_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-154 T4 — OBJ_FINDALLZONE (3506) + OBJ_FINDNEXT (3507) + dispatch

Port iterator-pair handlers per TS ObjOps.ts:185-201. OBJ_FINDALLZONE
pops coord, installs single-zone ObjIterator with World.CurrentTick
as creationTick. OBJ_FINDNEXT advances iterator, slot-routes ActiveObj
via setActiveObjSlot, pushes 1/0; returns error on stale iterator
mirroring LOC_FINDNEXT.

8 tests pin: iterator install + creationTick + level/x/z (FINDALLZONE
storage), nil-World degradation, invalid-coord; nil-iterator,
hit×slot routing, exhaustion, stale-error (FINDNEXT).
EOF
)"
```

---

## Task T5: OBJ_NAME + OBJ_PARAM handlers + dispatch + tests + close

**Files:**
- Modify: `pkg/script/handlers_obj.go` (`handleObjName`, `handleObjParam`)
- Modify: `pkg/script/handlers.go` (2 dispatch entries)
- Modify: `pkg/script/handlers_obj_test.go` (12 tests + minor builder)

- [ ] **Step T5.1: Write the failing OBJ_NAME and OBJ_PARAM tests**

Append to `pkg/script/handlers_obj_test.go`:

```go
// --- NAI-154: OBJ_NAME + OBJ_PARAM handler tests ---------------------

// newObjNameOrParamState builds a state with s.ActiveObj installed,
// Configs wired, and (for OBJ_PARAM) paramID pre-pushed.
func newObjNameOrParamState(t *testing.T, obj ActiveObj, mc *mockConfigs) *ScriptState {
    t.Helper()
    s := &ScriptState{
        Script:      &ScriptFile{IntOperands: []int32{0}},
        PC:          0,
        Configs:     mc,
        IntStack:    make([]int, StackCapacity),
        StringStack: make([]string, StackCapacity),
    }
    s.ActiveObj = obj
    return s
}

// TestObjNameNamePresent pins the Name branch.
func TestObjNameNamePresent(t *testing.T) {
    obj := &mockActiveObj{objType: 590, count: 1}
    mc := newTestConfigs()
    ot := objtype.NewObjType(590)
    ot.Name = "rune sword"
    mc.objs[590] = ot
    s := newObjNameOrParamState(t, obj, mc)

    if err := handleObjName(s); err != nil {
        t.Fatalf("handleObjName: %v", err)
    }
    if got := s.PopString(); got != "rune sword" {
        t.Errorf("name: got %q, want %q", got, "rune sword")
    }
}

// TestObjNameDebugFallback pins the DebugName fallback (Name empty).
func TestObjNameDebugFallback(t *testing.T) {
    obj := &mockActiveObj{objType: 590, count: 1}
    mc := newTestConfigs()
    ot := objtype.NewObjType(590)
    ot.Name = ""
    ot.DebugName = "sword_t1"
    mc.objs[590] = ot
    s := newObjNameOrParamState(t, obj, mc)

    if err := handleObjName(s); err != nil {
        t.Fatalf("handleObjName: %v", err)
    }
    if got := s.PopString(); got != "sword_t1" {
        t.Errorf("debugname fallback: got %q, want %q", got, "sword_t1")
    }
}

// TestObjNameNullFallback pins the "null" fallback (both empty).
func TestObjNameNullFallback(t *testing.T) {
    obj := &mockActiveObj{objType: 590, count: 1}
    mc := newTestConfigs()
    ot := objtype.NewObjType(590)
    ot.Name = ""
    ot.DebugName = ""
    mc.objs[590] = ot
    s := newObjNameOrParamState(t, obj, mc)

    if err := handleObjName(s); err != nil {
        t.Fatalf("handleObjName: %v", err)
    }
    if got := s.PopString(); got != "null" {
        t.Errorf("null fallback: got %q, want %q", got, "null")
    }
}

// TestObjNameRequiresActiveObj pins the requireActiveObj guard.
func TestObjNameRequiresActiveObj(t *testing.T) {
    s := newObjNameOrParamState(t, nil, newTestConfigs())

    err := handleObjName(s)
    if err == nil {
        t.Fatal("handleObjName(nil ActiveObj): want error, got nil")
    }
    if !strings.Contains(err.Error(), "OBJ_NAME") {
        t.Errorf("err: got %q, want substring %q", err.Error(), "OBJ_NAME")
    }
}

// TestObjNameRequiresConfigs pins the requireConfigs guard.
func TestObjNameRequiresConfigs(t *testing.T) {
    obj := &mockActiveObj{objType: 590, count: 1}
    s := newObjNameOrParamState(t, obj, nil)
    s.Configs = nil

    err := handleObjName(s)
    if err == nil {
        t.Fatal("handleObjName(nil Configs): want error, got nil")
    }
    if !strings.Contains(err.Error(), "OBJ_NAME") {
        t.Errorf("err: got %q, want substring %q", err.Error(), "OBJ_NAME")
    }
}

// TestObjNameUnknownType pins the unknown-objId guard.
func TestObjNameUnknownType(t *testing.T) {
    obj := &mockActiveObj{objType: 999, count: 1}
    s := newObjNameOrParamState(t, obj, newTestConfigs()) // 999 not registered

    err := handleObjName(s)
    if err == nil {
        t.Fatal("handleObjName(unknown objId): want error, got nil")
    }
    if !strings.Contains(err.Error(), "unknown obj id") {
        t.Errorf("err: got %q, want substring %q", err.Error(), "unknown obj id")
    }
}

// TestObjParamIntBranch pins the int-param branch.
func TestObjParamIntBranch(t *testing.T) {
    obj := &mockActiveObj{objType: 590, count: 1}
    mc := newTestConfigs()
    ot := objtype.NewObjType(590)
    ot.Params = make(objtype.ParamMap)
    ot.Params[7] = int32(42)
    mc.objs[590] = ot
    paramID := 7
    pt := objtype.NewParamType(paramID)
    pt.Type = objtype.ParamTypeInt
    pt.DefaultInt = 0
    mc.params[paramID] = pt
    s := newObjNameOrParamState(t, obj, mc)
    s.PushInt(paramID)

    if err := handleObjParam(s); err != nil {
        t.Fatalf("handleObjParam: %v", err)
    }
    if got := s.PopInt(); got != 42 {
        t.Errorf("int param: got %d, want 42", got)
    }
}

// TestObjParamStringBranch pins the string-param branch.
func TestObjParamStringBranch(t *testing.T) {
    obj := &mockActiveObj{objType: 590, count: 1}
    mc := newTestConfigs()
    ot := objtype.NewObjType(590)
    ot.Params = make(objtype.ParamMap)
    ot.Params[8] = "hello"
    mc.objs[590] = ot
    paramID := 8
    pt := objtype.NewParamType(paramID)
    pt.Type = objtype.ParamTypeString
    pt.DefaultString = ""
    mc.params[paramID] = pt
    s := newObjNameOrParamState(t, obj, mc)
    s.PushInt(paramID)

    if err := handleObjParam(s); err != nil {
        t.Fatalf("handleObjParam: %v", err)
    }
    if got := s.PopString(); got != "hello" {
        t.Errorf("string param: got %q, want %q", got, "hello")
    }
}

// TestObjParamIntDefaultFallback pins the int-default branch with
// sign-extension preserved (NAI-125 paramint default convention).
func TestObjParamIntDefaultFallback(t *testing.T) {
    obj := &mockActiveObj{objType: 590, count: 1}
    mc := newTestConfigs()
    ot := objtype.NewObjType(590)
    ot.Params = make(objtype.ParamMap) // empty — fallback path
    mc.objs[590] = ot
    paramID := 9
    pt := objtype.NewParamType(paramID)
    pt.Type = objtype.ParamTypeInt
    pt.DefaultInt = int32(-7)
    mc.params[paramID] = pt
    s := newObjNameOrParamState(t, obj, mc)
    s.PushInt(paramID)

    if err := handleObjParam(s); err != nil {
        t.Fatalf("handleObjParam: %v", err)
    }
    if got := s.PopInt(); got != -7 {
        t.Errorf("int default: got %d, want -7 (sign-extended)", got)
    }
}

// TestObjParamStringDefaultFallback pins the string-default branch.
func TestObjParamStringDefaultFallback(t *testing.T) {
    obj := &mockActiveObj{objType: 590, count: 1}
    mc := newTestConfigs()
    ot := objtype.NewObjType(590)
    ot.Params = make(objtype.ParamMap) // empty
    mc.objs[590] = ot
    paramID := 10
    pt := objtype.NewParamType(paramID)
    pt.Type = objtype.ParamTypeString
    pt.DefaultString = "def"
    mc.params[paramID] = pt
    s := newObjNameOrParamState(t, obj, mc)
    s.PushInt(paramID)

    if err := handleObjParam(s); err != nil {
        t.Fatalf("handleObjParam: %v", err)
    }
    if got := s.PopString(); got != "def" {
        t.Errorf("string default: got %q, want %q", got, "def")
    }
}

// TestObjParamRequiresActiveObj pins the requireActiveObj guard.
func TestObjParamRequiresActiveObj(t *testing.T) {
    s := newObjNameOrParamState(t, nil, newTestConfigs())
    s.PushInt(7)

    err := handleObjParam(s)
    if err == nil {
        t.Fatal("handleObjParam(nil ActiveObj): want error, got nil")
    }
    if !strings.Contains(err.Error(), "OBJ_PARAM") {
        t.Errorf("err: got %q, want substring %q", err.Error(), "OBJ_PARAM")
    }
}

// TestObjParamRequiresConfigs pins the requireConfigs guard.
func TestObjParamRequiresConfigs(t *testing.T) {
    obj := &mockActiveObj{objType: 590, count: 1}
    s := newObjNameOrParamState(t, obj, nil)
    s.Configs = nil
    s.PushInt(7)

    err := handleObjParam(s)
    if err == nil {
        t.Fatal("handleObjParam(nil Configs): want error, got nil")
    }
    if !strings.Contains(err.Error(), "OBJ_PARAM") {
        t.Errorf("err: got %q, want substring %q", err.Error(), "OBJ_PARAM")
    }
}

// TestObjParamUnknownType pins the unknown-objId guard.
func TestObjParamUnknownType(t *testing.T) {
    obj := &mockActiveObj{objType: 999, count: 1}
    s := newObjNameOrParamState(t, obj, newTestConfigs()) // 999 not registered
    s.PushInt(7)

    err := handleObjParam(s)
    if err == nil {
        t.Fatal("handleObjParam(unknown objId): want error, got nil")
    }
    if !strings.Contains(err.Error(), "unknown obj id") {
        t.Errorf("err: got %q, want substring %q", err.Error(), "unknown obj id")
    }
}
```

**Plan-author check:** The test bodies above assume the test fixture builders `newTestConfigs`, `mockConfigs.objs`, `mockConfigs.params`, and helpers `objtype.NewObjType`, `objtype.NewParamType`, `objtype.ParamMap`, `objtype.ParamTypeInt`, `objtype.ParamTypeString` exist with the exact field/constant names shown. Implementer: if any name differs (e.g., `mockConfigs` uses `paramTypes` instead of `params`, or `ParamType.Type` is named differently), match the existing convention in `handlers_config_test.go` for OC_PARAM tests — that's the closest reference. Update fixture seed code to match without changing the test's pin intent.

Reference: `pkg/script/handlers_config_test.go` for `TestOcParam*` patterns (they pin the same `paramLookup` shared path that OBJ_PARAM delegates to).

- [ ] **Step T5.2: Run test to verify failure**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestObjName|TestObjParam' -v
```

Expected: FAIL — "undefined: handleObjName" / "undefined: handleObjParam".

- [ ] **Step T5.3: Implement `handleObjName` and `handleObjParam`**

Add to `pkg/script/handlers_obj.go`:

```go
// handleObjName (OBJ_NAME, opcode 3508) pushes the active obj's name
// (or debugname fallback; "null" when both are empty). Mirrors TS
// ObjOps.ts:106-110 and the existing handleOcName at
// handlers_config.go:429-446.
func handleObjName(s *ScriptState) error {
    if err := requireActiveObj(s, "OBJ_NAME"); err != nil {
        return err
    }
    if err := requireConfigs(s, "OBJ_NAME"); err != nil {
        return err
    }
    ot := s.Configs.ObjType(s.ActiveObj.ObjType())
    if ot == nil {
        return fmt.Errorf("OBJ_NAME: unknown obj id %d", s.ActiveObj.ObjType())
    }
    if ot.Name != "" {
        s.PushString(ot.Name)
    } else if ot.DebugName != "" {
        s.PushString(ot.DebugName)
    } else {
        s.PushString("null")
    }
    return nil
}

// handleObjParam (OBJ_PARAM, opcode 3509) pops a paramID and delegates
// to paramLookup using the active obj's type Params. Mirrors TS
// ObjOps.ts:95-104 and the existing handleOcParam at
// handlers_config.go:448-460.
func handleObjParam(s *ScriptState) error {
    if err := requireActiveObj(s, "OBJ_PARAM"); err != nil {
        return err
    }
    if err := requireConfigs(s, "OBJ_PARAM"); err != nil {
        return err
    }
    paramID := s.PopInt()
    ot := s.Configs.ObjType(s.ActiveObj.ObjType())
    if ot == nil {
        return fmt.Errorf("OBJ_PARAM: unknown obj id %d", s.ActiveObj.ObjType())
    }
    return paramLookup(s, ot.Params, paramID)
}
```

- [ ] **Step T5.4: Wire dispatch**

In `pkg/script/handlers.go`, near the existing OBJ-family entries from T3/T4, add:

```go
OpObjName:  handleObjName,
OpObjParam: handleObjParam,
```

- [ ] **Step T5.5: Run tests to verify pass**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run 'TestObjName|TestObjParam' -v
```

Expected: PASS — all 12 tests green.

- [ ] **Step T5.6: Run final full test suite + vet**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Expected: PASS, clean. Capture any unexpected output for reviewer.

- [ ] **Step T5.7: Verify all 5 opcodes are now dispatched**

Run the missing-handler audit:
```bash
rg -n '^\s+(Op\w+)\s+Opcode\s*=' -or '$1' pkg/script/opcode.go | sort -u > $TMPDIR/declared
rg -no 'Op[A-Za-z0-9_]+:\s*handle' pkg/script/handlers.go | sed 's/:.*//' | sort -u > $TMPDIR/dispatched
comm -23 $TMPDIR/declared $TMPDIR/dispatched | wc -l
comm -23 $TMPDIR/declared $TMPDIR/dispatched | grep -E 'OpObj(Find|FindAllZone|FindNext|Name|Param)'
```

Expected: count = 29 (down from 34); the second grep returns 0 hits (all 5 dispatched).

- [ ] **Step T5.8: Commit T5**

```bash
git add pkg/script/handlers_obj.go pkg/script/handlers.go pkg/script/handlers_obj_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-154 T5 — OBJ_NAME (3508) + OBJ_PARAM (3509) + dispatch

Port handleObjName per TS ObjOps.ts:106-110 (mirrors handleOcName:
Name → DebugName → "null" fallback chain). Port handleObjParam per TS
ObjOps.ts:95-104 (mirrors handleOcParam, delegates to paramLookup
shared path with s.ActiveObj.ObjType() as the obj-id source).

12 tests pin: Name/DebugName/null fallback chain × require guards ×
unknown objId (OBJ_NAME, 6 tests); int/string branch × int/string
default fallback × require guards × unknown objId (OBJ_PARAM, 6
tests). Sign-extension of negative DefaultInt pinned per NAI-125.

Cascade-tail: 34 → 29 unhandled opcodes. All 5 NAI-154 opcodes
dispatched.
EOF
)"
```

---

## Final Review Task (Sonnet reviewer subagent, NOT inline)

After T5 commits, dispatch a single Sonnet reviewer subagent per `superpowers:requesting-code-review` (or equivalent superpowers code-reviewer agent — per `superpowers_code_reviewer_model`, **must be Sonnet, not Opus**).

Reviewer scope: all 5 T-task commits (`HEAD~5..HEAD`) + spec compliance check against `docs/superpowers/specs/2026-05-10-nai-154-obj-find-family-design.md`. Reviewer should verify:

1. All 5 handlers wired in `handlers.go`.
2. `OtherActiveObj` and `objIterator` fields present and doc-commented.
3. `setActiveObjSlot` IntOperand 0/1 routing matches `setActiveLocSlot` precedent.
4. `worldVarsView.GetObj` / `ZoneObjs` delegate correctly (nil-guard, type assert path).
5. TS-fidelity check on each handler against spec §2 excerpts.
6. Test coverage matches spec §5 (7 iterator + 7 OBJ_FIND + 8 iterator-handler + 12 OBJ_NAME/OBJ_PARAM = 34 tests).
7. No new pre-existing-failure attribution drift (per `verify_implementer_claims`).
8. Modern-Go style per `use-modern-go` skill.

Reviewer report: DONE / DONE_WITH_CONCERNS / BLOCKED.

---

## Post-Review: Smoke Handoff (User-Launched)

Per `smoke_test_server_handoff`: hand off to user with a paste-ready resume prompt.

**Smoke procedure:**
1. User runs `CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file config.yaml` (or build + run).
2. Connect Java client; play a normal session (Tutorial Island → Lumbridge spawn → general loot pickup procs).
3. Watch server log for any `script: no handler for OBJ_FIND|OBJ_FINDALLZONE|OBJ_FINDNEXT|OBJ_NAME|OBJ_PARAM` WARN classes — these should NOT appear.
4. Regression-check NAI-152/153 mindrune pickup (ground tile clears, item enters inventory).

**Smoke result branch:**
- **CLEAN** → close commit per `close_commit_memory_trailer` with `Closes memory:` trailer.
- **ADJACENT-OP NO-HANDLER WARN ≤30 LOC** → in-scope-stretch per `smoke_surfaces_adjacent_divergences`.
- **ADJACENT-OP NO-HANDLER WARN >30 LOC** → route to NAI-155.
- **NEW SEMANTIC DIVERGENCE** → investigation sub-spec per `investigation_subspec_cadence`.

---

## Self-Review (plan author)

**1. Spec coverage:**
| Spec §  | Coverage |
|---|---|
| §3.2 missing #1 (ObjIterator type) | T2.3 |
| §3.2 missing #2 (OtherActiveObj field) | T1.1 |
| §3.2 missing #3 (objIterator field) | T2.4 |
| §3.2 missing #4 (setActiveObjSlot) | T1.4 |
| §3.2 missing #5 (WorldVars.GetObj) | T1.3 (decl) + T1.6 (impl) |
| §3.2 missing #6 (WorldVars.ZoneObjs) | T1.3 (decl) + T1.6 (impl) |
| §3.2 missing #7 (worldVarsView.GetObj) | T1.6 |
| §3.2 missing #8 (worldVarsView.ZoneObjs) | T1.6 |
| §3.2 missing #9 (5 handler functions) | T3.3 (OBJ_FIND) + T4.3 (FINDALLZONE/FINDNEXT) + T5.3 (NAME/PARAM) |
| §3.2 missing #10 (5 dispatch entries) | T3.4 + T4.4 + T5.4 |
| §3.2 missing #11 (tests) | T2.1 (iterator), T3.1 (FIND), T4.1 (FINDALLZONE/NEXT), T5.1 (NAME/PARAM) |
| §6 deviation tracking | T1.1 doc-comment cites NAI-154-D-NO-DOWNSTREAM-OBJ2-CONSUMERS |
| §7 verification | T1.8 / T2.6 / T3.5 / T4.5 / T5.6 |
| §8 smoke | "Post-Review" section |

All spec missing-items mapped to a task step. ✓

**2. Placeholder scan:** Searched the plan for `TBD`, `TODO`, `implement later`, `fill in details`, `appropriate`, `handle edge cases`, `similar to Task N`. None present. ✓

**3. Type consistency:**
- `ObjIterator` struct + `NewZoneObjIterator(world, tick, level, x, z)` constructor: consistent across T2.3, T4.1, T4.3.
- `setActiveObjSlot(s *ScriptState, obj ActiveObj)` signature: consistent across T1.4, T3.3, T4.3.
- `WorldVars.GetObj(level, x, z, objId, receiverUID int) ActiveObj` and `WorldVars.ZoneObjs(level, zoneX, zoneZ int) []ActiveObj`: consistent across T1.3, T1.5, T1.6, T2.1, T3.1.
- `s.Self.UID()` receiver convention: consistent across T3.1 (test), T3.3 (impl), spec §3.3.
- `s.objIterator` field name: consistent across T2.4 (decl), T4.1 (test fixture), T4.3 (impl).
- `s.OtherActiveObj` field name: consistent across T1.1 (decl), T3.1/T4.1 (tests), T1.4/T4.3 (impl via setActiveObjSlot).

No type/name drift detected. ✓

**4. Plan-runnable check (per `plan_runnable_test_fixtures`):**
- Each test step's fixture is mentally executable: state builder → push args → call handler → assert pops/state.
- The OBJ_PARAM tests assume `mockConfigs.params` and `objtype.NewParamType` shapes — flagged in T5.1's plan-author check note. Implementer follows existing OC_PARAM test precedent if names differ.
- T1 has an inversion issue: `objIterator *ObjIterator` was originally in T1.2 but references a type from T2.3 — moved to T2.4 to keep each task's commit green. Fixed. ✓

**5. Dependency ordering:**
- T1 → T2 (T2 needs `OtherActiveObj` field, `setActiveObjSlot`, `WorldVars.GetObj`/`ZoneObjs` declarations).
- T2 → T3 (T3 tests reference `mockActiveObj` which is pre-existing; T3 handler implementation uses surface from T1).
- T2 → T4 (T4 needs `ObjIterator` type + `objIterator` field).
- T3 → T4 (cohesive ordering; T4 wires FINDALLZONE/NEXT which T3's FIND has prepared via setActiveObjSlot precedent in tests).
- T5 standalone after T1 (only needs `requireActiveObj`, `requireConfigs`, `paramLookup` which all exist at HEAD).

All forward references resolved. ✓
