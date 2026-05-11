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

// TestPlayerIterator_PassesFilter_LineOfSight_PlayerAsSrc pins the
// TS-asymmetric arg order: PlayerHuntAllCommandIterator passes player-as-src
// + iterator-as-dest (ScriptIterators.ts:216), REVERSE of NpcHuntAll.
func TestPlayerIterator_PassesFilter_LineOfSight_PlayerAsSrc(t *testing.T) {
	t.Parallel()
	rec := &recordingLineValidator{losReturn: true}
	it := NewHuntAllPlayerIterator(nil, rec, 0, 0, 3200, 3200, 8, objtype.HuntVisLineOfSight)
	p := &mockPlayer{x: 3201, z: 3202}
	_ = it.passesFilter(p)
	// Player should be the SRC (3201, 3202); iterator-center should be the DEST (3200, 3200).
	if rec.losSrcX != 3201 || rec.losSrcZ != 3202 {
		t.Errorf("LoS src: got (%d,%d), want (3201,3202) — player coords", rec.losSrcX, rec.losSrcZ)
	}
	if rec.losDestX != 3200 || rec.losDestZ != 3200 {
		t.Errorf("LoS dest: got (%d,%d), want (3200,3200) — iterator-center coords", rec.losDestX, rec.losDestZ)
	}
}

// TestPlayerIterator_PassesFilter_LineOfWalk_PlayerAsSrc — companion to LoS.
func TestPlayerIterator_PassesFilter_LineOfWalk_PlayerAsSrc(t *testing.T) {
	t.Parallel()
	rec := &recordingLineValidator{lowReturn: true}
	it := NewHuntAllPlayerIterator(nil, rec, 0, 0, 3200, 3200, 8, objtype.HuntVisLineOfWalk)
	p := &mockPlayer{x: 3201, z: 3202}
	_ = it.passesFilter(p)
	if rec.lowSrcX != 3201 || rec.lowSrcZ != 3202 {
		t.Errorf("LoW src: got (%d,%d), want (3201,3202)", rec.lowSrcX, rec.lowSrcZ)
	}
	if rec.lowDestX != 3200 || rec.lowDestZ != 3200 {
		t.Errorf("LoW dest: got (%d,%d), want (3200,3200)", rec.lowDestX, rec.lowDestZ)
	}
}

// TestPlayerIterator_LineValidatorArgShape pins the TS-canonical
// (srcSize=1, destWidth=1, destLength=1, extraFlag=0) arg tuple at both
// LOS and LOW branches of PlayerIterator.passesFilter (player_iterator.go
// lines 71, 77). Mirrors NAI-165-D-LOW-ARG-SHAPE-FIX semantics, applied
// to the iterator family. NAI-166-D-LOW-ARG-SHAPE-SWEEP.
//
// TS canonical: ScriptIterators.ts:216 (LOS), :220 (LOW) — both route
// through the GameMap.ts:425-431 wrappers (1, 1, 1, 1, 0). goscape's
// srcSize collapses TS srcWidth+srcHeight into one arg via RayCast
// (linevalidator.go:21), so TS-faithful tuple at goscape's LV iface is
// (srcSize=1, destWidth=1, destLength=1, extraFlag=0).
func TestPlayerIterator_LineValidatorArgShape(t *testing.T) {
	t.Parallel()
	// LOS branch
	stubLOS := &stubLineValidatorArgs{losReturn: true}
	itLOS := NewHuntAllPlayerIterator(nil, stubLOS, 0, 0, 3200, 3200, 8, objtype.HuntVisLineOfSight)
	p := &mockPlayer{x: 3201, z: 3202}
	_ = itLOS.passesFilter(p)
	if len(stubLOS.losCalls) != 1 {
		t.Fatalf("LOS branch: expected 1 LV call, got %d", len(stubLOS.losCalls))
	}
	got := stubLOS.losCalls[0]
	want := losCall{level: 0, srcX: 3201, srcZ: 3202, destX: 3200, destZ: 3200, srcSize: 1, destWidth: 1, destLength: 1, extraFlag: 0}
	if got != want {
		t.Fatalf("LOS arg shape:\n got=%+v\nwant=%+v", got, want)
	}

	// LOW branch
	stubLOW := &stubLineValidatorArgs{lowReturn: true}
	itLOW := NewHuntAllPlayerIterator(nil, stubLOW, 0, 0, 3200, 3200, 8, objtype.HuntVisLineOfWalk)
	_ = itLOW.passesFilter(p)
	if len(stubLOW.lowCalls) != 1 {
		t.Fatalf("LOW branch: expected 1 LV call, got %d", len(stubLOW.lowCalls))
	}
	got = stubLOW.lowCalls[0]
	if got != want {
		t.Fatalf("LOW arg shape:\n got=%+v\nwant=%+v", got, want)
	}
}
