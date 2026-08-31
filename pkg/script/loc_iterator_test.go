package script

import "testing"

// locIterTestOps is a fakeLocOps wrapper that returns a fixed slice
// from AllLocsSafe (LocIterator's iteration source post-h-loc-4).
// Independent of fakeLocOps.inZone (the AllLocsInZone slice, shared
// across other tests for MAP_LOCADDUNSAFE-style consumers).
type locIterTestOps struct {
	*fakeLocOps
	zoneLocs []ActiveLoc
}

// AllLocsSafe returns the test fixture verbatim. The fixture is
// authored in the order the iterator should yield (i.e., the test
// pre-applies any reverse / IsActive transform expected of the
// production serverLocOps.AllLocsSafe implementation). Mirrors the
// fake-snapshot pattern from the pre-h-loc-4 AllLocsInZone version.
func (o *locIterTestOps) AllLocsSafe(level, x, z int, reverse bool) []ActiveLoc {
	o.allLocsSafeCalls = append(o.allLocsSafeCalls, allLocsSafeCall{level, x, z, reverse})
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
	for i := range 3 {
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

// TestLocIteratorRoutesThroughAllLocsSafeWithReverseTrue pins the
// h-loc-4 fix: LocIterator must source its snapshot from
// LocOps.AllLocsSafe(level, x, z, reverse=true), mirroring TS
// ScriptIterators.ts:378 (getAllLocsSafe(true)). Filter + reverse are
// delegated to the impl (verified separately in
// modules/world/script_loc_ops_test.go), so this test only asserts the
// iterator's contract: it calls AllLocsSafe exactly once on first
// Next() with reverse=true and the right coord triple.
//
// Toggle-off proof: revert pkg/script/loc_iterator.go's lazy-init from
// AllLocsSafe(...) back to AllLocsInZone(...) → this test fails with
// "AllLocsSafe call count: got 0, want 1".
func TestLocIteratorRoutesThroughAllLocsSafeWithReverseTrue(t *testing.T) {
	// Populate BOTH AllLocsSafe (the post-h-loc-4 source) and AllLocsInZone
	// (the pre-h-loc-4 source) with the same fixture, so that if the
	// iterator regresses to calling AllLocsInZone the failure surfaces as
	// the routing-contract assertion below rather than an upstream
	// empty-iterator path.
	ops := newLocIterTestOps([]ActiveLoc{fakeActiveLoc{id: 100}})
	ops.fakeLocOps.inZone = []ActiveLoc{fakeActiveLoc{id: 100}}
	it := NewZoneLocIterator(ops, 0, 5, 3200, 3300)

	if _, ok := it.Next(); !ok {
		t.Fatal("first Next: expected hit")
	}

	if got := len(ops.allLocsSafeCalls); got != 1 {
		t.Fatalf("AllLocsSafe call count: got %d, want 1 (LocIterator must source from AllLocsSafe, not AllLocsInZone)", got)
	}
	call := ops.allLocsSafeCalls[0]
	if call.level != 5 || call.x != 3200 || call.z != 3300 {
		t.Errorf("AllLocsSafe coords: got (%d,%d,%d), want (5,3200,3300)", call.level, call.x, call.z)
	}
	if !call.reverse {
		t.Errorf("AllLocsSafe reverse arg: got false, want true (TS ScriptIterators.ts:378 passes true)")
	}
}
