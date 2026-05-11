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
