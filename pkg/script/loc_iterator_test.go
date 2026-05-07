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
