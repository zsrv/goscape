package script

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// --- NAI-35-T4: PlayerIterator (HuntAll-mode) tests --------------------

func TestPlayerIterator_Stale_StrictGreaterThan(t *testing.T) {
	t.Parallel()
	it := &PlayerIterator{creationTick: 10}
	if it.Stale(10) {
		t.Error("currentTick == creationTick: should NOT be stale (TS-faithful)")
	}
	if !it.Stale(11) {
		t.Error("currentTick > creationTick: should be stale")
	}
	if it.Stale(9) {
		t.Error("currentTick < creationTick: should NOT be stale")
	}
}

func TestNewHuntAllPlayerIterator_Construction(t *testing.T) {
	t.Parallel()
	const tick = 42
	const level, x, z = 0, 3200, 3200
	const distance = 16
	const huntvis = objtype.HuntVisLineOfWalk

	it := NewHuntAllPlayerIterator(nil, nil, tick, level, x, z, distance, huntvis)

	if it.mode != PlayerIteratorHuntAll {
		t.Errorf("mode: got %d, want PlayerIteratorHuntAll", it.mode)
	}
	if it.creationTick != tick {
		t.Errorf("creationTick: got %d, want %d", it.creationTick, tick)
	}
	if it.distance != distance {
		t.Errorf("distance: got %d, want %d", it.distance, distance)
	}
	if it.huntvis != huntvis {
		t.Errorf("huntvis: got %d, want %d", it.huntvis, huntvis)
	}
	expectedRadius := 1 + distance/8
	if it.maxZoneX != (x>>3)+expectedRadius {
		t.Errorf("maxZoneX: got %d, want %d", it.maxZoneX, (x>>3)+expectedRadius)
	}
	if it.curZoneX != it.maxZoneX || it.curZoneZ != it.maxZoneZ {
		t.Errorf("cursor: got (%d,%d), want (%d,%d)", it.curZoneX, it.curZoneZ, it.maxZoneX, it.maxZoneZ)
	}
}

func TestPlayerIterator_PassesFilter_HuntVisOff_AdmitsInRange(t *testing.T) {
	t.Parallel()
	it := NewHuntAllPlayerIterator(nil, nil, 0, 0, 3200, 3200, 8, objtype.HuntVisOff)
	p := &mockPlayer{x: 3201, z: 3201}
	if !it.passesFilter(p) {
		t.Error("HuntVisOff in-range: should pass")
	}
}

func TestPlayerIterator_PassesFilter_OutsideDistanceRejected(t *testing.T) {
	t.Parallel()
	it := NewHuntAllPlayerIterator(nil, nil, 0, 0, 3200, 3200, 4, objtype.HuntVisOff)
	p := &mockPlayer{x: 3300, z: 3300}
	if it.passesFilter(p) {
		t.Error("beyond distance: should fail regardless of huntvis")
	}
}

func TestPlayerIterator_PassesFilter_LineOfSight_RejectsBlocked(t *testing.T) {
	t.Parallel()
	stub := &stubLineValidator{losReturn: false, lowReturn: true}
	it := NewHuntAllPlayerIterator(nil, stub, 0, 0, 3200, 3200, 8, objtype.HuntVisLineOfSight)
	p := &mockPlayer{x: 3201, z: 3201}
	if it.passesFilter(p) {
		t.Error("LoS=false stub: in-range player should be rejected")
	}
}

func TestPlayerIterator_PassesFilter_LineOfWalk_AdmitsClear(t *testing.T) {
	t.Parallel()
	stub := &stubLineValidator{losReturn: false, lowReturn: true}
	it := NewHuntAllPlayerIterator(nil, stub, 0, 0, 3200, 3200, 8, objtype.HuntVisLineOfWalk)
	p := &mockPlayer{x: 3201, z: 3201}
	if !it.passesFilter(p) {
		t.Error("LoW=true stub: in-range player should pass")
	}
}

func TestPlayerIterator_NilLookup_NextReturnsFalse(t *testing.T) {
	t.Parallel()
	it := NewHuntAllPlayerIterator(nil, nil, 0, 0, 3200, 3200, 8, objtype.HuntVisOff)
	p, ok := it.Next()
	if ok || p != nil {
		t.Errorf("nil lookup: got (%v, %t), want (nil, false)", p, ok)
	}
}
