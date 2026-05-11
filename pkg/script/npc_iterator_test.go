package script

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// stubLineValidator is a script.LineValidator test double that returns
// fixed responses for HasLineOfSight / HasLineOfWalk. NAI-35-T3.
type stubLineValidator struct {
	losReturn bool
	lowReturn bool
}

func (s *stubLineValidator) HasLineOfSight(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool {
	return s.losReturn
}

func (s *stubLineValidator) HasLineOfWalk(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool {
	return s.lowReturn
}

// recordingLineValidator captures the args of the most recent LoS/LoW
// call so tests can pin the src/dest argument order.
type recordingLineValidator struct {
	losLevel, losSrcX, losSrcZ, losDestX, losDestZ int
	losReturn                                      bool
	lowLevel, lowSrcX, lowSrcZ, lowDestX, lowDestZ int
	lowReturn                                      bool
}

func (r *recordingLineValidator) HasLineOfSight(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool {
	r.losLevel, r.losSrcX, r.losSrcZ, r.losDestX, r.losDestZ = level, srcX, srcZ, destX, destZ
	return r.losReturn
}
func (r *recordingLineValidator) HasLineOfWalk(level, srcX, srcZ, destX, destZ, srcSize, destWidth, destLength, extraFlag int) bool {
	r.lowLevel, r.lowSrcX, r.lowSrcZ, r.lowDestX, r.lowDestZ = level, srcX, srcZ, destX, destZ
	return r.lowReturn
}

func TestNpcIterator_StaleCheck(t *testing.T) {
	// TS uses strict `>` (ScriptIterators.ts:332,343): only forward
	// tick drift is stale. Past ticks are physically impossible
	// (script VM doesn't run backwards) but per TS we don't flag.
	it := &NpcIterator{creationTick: 100}
	if it.Stale(100) {
		t.Error("Stale(creationTick) should be false")
	}
	if !it.Stale(101) {
		t.Error("Stale(creationTick+1) should be true")
	}
	if it.Stale(99) {
		t.Error("Stale(creationTick-1) should be false (TS uses strict >, not !=)")
	}
}

func TestNpcIterator_DistanceMode_BoundsMath(t *testing.T) {
	cases := []struct {
		name                                                             string
		x, z, distance                                                   int
		wantMinZX, wantMaxZX, wantMinZZ, wantMaxZZ, wantCurZX, wantCurZZ int
	}{
		// centerX = x>>3, radius = 1 + distance/8, zone-bounds = center ± radius
		// curZone* starts at (max, max) per TS line 337-340
		{"distance=0 → radius 1", 3200, 3300, 0, 399, 401, 411, 413, 401, 413},
		{"distance=8 → radius 2", 3200, 3300, 8, 398, 402, 410, 414, 402, 414},
		{"distance=15 → radius 2 (15/8=1)", 3200, 3300, 15, 398, 402, 410, 414, 402, 414},
		{"distance=16 → radius 3 (16/8=2)", 3200, 3300, 16, 397, 403, 409, 415, 403, 415},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			it := NewDistanceNpcIterator(nil, 0, 0, tc.x, tc.z, tc.distance, 0, -1)
			if it.minZoneX != tc.wantMinZX || it.maxZoneX != tc.wantMaxZX {
				t.Errorf("X bounds: got [%d, %d], want [%d, %d]", it.minZoneX, it.maxZoneX, tc.wantMinZX, tc.wantMaxZX)
			}
			if it.minZoneZ != tc.wantMinZZ || it.maxZoneZ != tc.wantMaxZZ {
				t.Errorf("Z bounds: got [%d, %d], want [%d, %d]", it.minZoneZ, it.maxZoneZ, tc.wantMinZZ, tc.wantMaxZZ)
			}
			if it.curZoneX != tc.wantCurZX || it.curZoneZ != tc.wantCurZZ {
				t.Errorf("cursor: got (%d, %d), want (%d, %d) (start at max,max)", it.curZoneX, it.curZoneZ, tc.wantCurZX, tc.wantCurZZ)
			}
			if it.mode != NpcIteratorDistance {
				t.Errorf("mode: got %v, want NpcIteratorDistance", it.mode)
			}
		})
	}
}

func TestNpcIterator_ZoneMode_Construction(t *testing.T) {
	it := NewZoneNpcIterator(nil, 42, 3, 3200, 3300)
	if it.mode != NpcIteratorZone {
		t.Errorf("mode: got %v, want NpcIteratorZone", it.mode)
	}
	if it.creationTick != 42 {
		t.Errorf("creationTick: got %d, want 42", it.creationTick)
	}
	if it.level != 3 || it.x != 3200 || it.z != 3300 {
		t.Errorf("center: got (level=%d, x=%d, z=%d), want (3, 3200, 3300)", it.level, it.x, it.z)
	}
	if it.typeID != -1 {
		t.Errorf("typeID: got %d, want -1 (no filter in ZONE mode)", it.typeID)
	}
	if it.started {
		t.Error("started: should be false before first Next call")
	}
}

func TestNpcIterator_ZoneMode_SingleZone(t *testing.T) {
	npc1 := &mockNpc{typeID: 1, x: 3200, z: 3300, level: 0}
	npc2 := &mockNpc{typeID: 2, x: 3201, z: 3301, level: 0}
	lookup := &mockNpcLookup{
		byZone: map[uint64][]ActiveNpc{
			mockZoneKey(0, 3200&^7, 3300&^7): {npc1, npc2},
		},
	}
	it := NewZoneNpcIterator(lookup, 0, 0, 3200, 3300)

	got1, ok1 := it.Next()
	if !ok1 || got1 != npc1 {
		t.Errorf("first: got (%v, %v), want (npc1, true)", got1, ok1)
	}
	got2, ok2 := it.Next()
	if !ok2 || got2 != npc2 {
		t.Errorf("second: got (%v, %v), want (npc2, true)", got2, ok2)
	}
	got3, ok3 := it.Next()
	if ok3 || got3 != nil {
		t.Errorf("third (exhausted): got (%v, %v), want (nil, false)", got3, ok3)
	}

	// Pin: ZoneNpcs called exactly once with zone-aligned coords
	if lookup.zoneNpcsCalls != 1 {
		t.Errorf("zoneNpcsCalls: got %d, want 1 (lazy single fetch)", lookup.zoneNpcsCalls)
	}
	wantArgs := [3]int{0, (3200 >> 3) * 8, (3300 >> 3) * 8} // = (0, 3200, 3296)
	if lookup.zoneNpcsCallArgs[0] != wantArgs {
		t.Errorf("zoneNpcsCallArgs[0]: got %v, want %v", lookup.zoneNpcsCallArgs[0], wantArgs)
	}
}

func TestNpcIterator_ZoneMode_TerminatesAfterOneZone(t *testing.T) {
	// Empty zone, second Next is also (nil, false) — and ZoneNpcs called once
	lookup := &mockNpcLookup{} // byZone nil → returns nil
	it := NewZoneNpcIterator(lookup, 0, 0, 3200, 3300)
	if got, ok := it.Next(); ok || got != nil {
		t.Errorf("first on empty: got (%v, %v), want (nil, false)", got, ok)
	}
	if got, ok := it.Next(); ok || got != nil {
		t.Errorf("second on empty: got (%v, %v), want (nil, false)", got, ok)
	}
	if lookup.zoneNpcsCalls != 1 {
		t.Errorf("zoneNpcsCalls: got %d, want 1 (no re-fetch after exhaustion)", lookup.zoneNpcsCalls)
	}
}

func TestNpcIterator_DistanceMode_CursorOrder(t *testing.T) {
	// distance=0 → radius 1 → 9 zones (3x3) walked outer-X-desc, inner-Z-desc.
	// centerX=400, centerZ=412, bounds X=[399,401], Z=[411,413].
	// Expected zone visit order (in zone-aligned coord-grid coords, *8):
	// (401,413), (401,412), (401,411),  ← x=401 inner z desc
	// (400,413), (400,412), (400,411),  ← x=400
	// (399,413), (399,412), (399,411).  ← x=399
	// Per TS line 337-340: outer X descending, inner Z descending.
	lookup := &mockNpcLookup{} // byZone nil → returns nil per zone (empty)
	it := NewDistanceNpcIterator(lookup, 0, 0, 3200, 3300, 0, 0, -1)

	// Drain — Next loops until exhaustion. Empty zones produce no yields,
	// so we just drive Next() until it returns false.
	for {
		if _, ok := it.Next(); !ok {
			break
		}
	}

	want := [][3]int{
		{0, 401 * 8, 413 * 8},
		{0, 401 * 8, 412 * 8},
		{0, 401 * 8, 411 * 8},
		{0, 400 * 8, 413 * 8},
		{0, 400 * 8, 412 * 8},
		{0, 400 * 8, 411 * 8},
		{0, 399 * 8, 413 * 8},
		{0, 399 * 8, 412 * 8},
		{0, 399 * 8, 411 * 8},
	}
	if len(lookup.zoneNpcsCallArgs) != len(want) {
		t.Fatalf("zone visits: got %d, want %d. Sequence: %v", len(lookup.zoneNpcsCallArgs), len(want), lookup.zoneNpcsCallArgs)
	}
	for i := range want {
		if lookup.zoneNpcsCallArgs[i] != want[i] {
			t.Errorf("visit[%d]: got %v, want %v", i, lookup.zoneNpcsCallArgs[i], want[i])
		}
	}
}

func TestNpcIterator_DistanceMode_DistanceFilter(t *testing.T) {
	// Center (3200, 3300, lvl=0); distance=5. Zone-aligned coords for
	// the center zone are (3200, 3296) (3300>>3*8 = 3296). Place 3 NPCs
	// at: dist 0 (in), dist 5 (in, equal), dist 6 (out).
	// All three live in the SAME zone since they're within ~7 tiles.
	npcIn0 := &mockNpc{typeID: 1, x: 3200, z: 3300, level: 0}  // dist 0
	npcIn5 := &mockNpc{typeID: 1, x: 3205, z: 3300, level: 0}  // dist 5
	npcOut6 := &mockNpc{typeID: 1, x: 3206, z: 3300, level: 0} // dist 6 → filter out
	zoneKey := mockZoneKey(0, 3200, 3296)
	lookup := &mockNpcLookup{byZone: map[uint64][]ActiveNpc{zoneKey: {npcIn0, npcIn5, npcOut6}}}
	it := NewDistanceNpcIterator(lookup, 0, 0, 3200, 3300, 5, 0, -1)

	yielded := []ActiveNpc{}
	for {
		n, ok := it.Next()
		if !ok {
			break
		}
		yielded = append(yielded, n)
	}
	if len(yielded) != 2 || yielded[0] != npcIn0 || yielded[1] != npcIn5 {
		t.Errorf("yielded: got %v, want [npcIn0, npcIn5]", yielded)
	}
}

func TestNpcIterator_DistanceMode_TypeFilter(t *testing.T) {
	// 2 NPCs in same zone, different types. Filter on typeID=42 yields only the matching one.
	npcMatch := &mockNpc{typeID: 42, x: 3200, z: 3300, level: 0}
	npcMiss := &mockNpc{typeID: 99, x: 3201, z: 3300, level: 0}
	zoneKey := mockZoneKey(0, 3200, 3296)
	lookup := &mockNpcLookup{byZone: map[uint64][]ActiveNpc{zoneKey: {npcMiss, npcMatch}}}
	it := NewDistanceNpcIterator(lookup, 0, 0, 3200, 3300, 5, 0, 42)

	yielded := []ActiveNpc{}
	for {
		n, ok := it.Next()
		if !ok {
			break
		}
		yielded = append(yielded, n)
	}
	if len(yielded) != 1 || yielded[0] != npcMatch {
		t.Errorf("typeID=42: got %v, want [npcMatch only]", yielded)
	}

	// Negative-branch: typeID=-1 yields BOTH. Per test_passes_for_wrong_reason.md.
	lookup2 := &mockNpcLookup{byZone: map[uint64][]ActiveNpc{zoneKey: {npcMiss, npcMatch}}}
	it2 := NewDistanceNpcIterator(lookup2, 0, 0, 3200, 3300, 5, 0, -1)
	yielded2 := []ActiveNpc{}
	for {
		n, ok := it2.Next()
		if !ok {
			break
		}
		yielded2 = append(yielded2, n)
	}
	if len(yielded2) != 2 {
		t.Errorf("typeID=-1: got len=%d, want 2 (no filter)", len(yielded2))
	}
}

// --- NAI-35-T3: HuntAll-mode iterator tests ----------------------------

func TestNewHuntAllNpcIterator_Construction(t *testing.T) {
	// centerX = 3200>>3 = 400, centerZ = 3300>>3 = 412.
	// distance=10 → radius = 1 + 10/8 = 2.
	// Bounds: X=[398,402], Z=[410,414]; cursor at (max,max) = (402,414).
	it := NewHuntAllNpcIterator(nil, nil, 99, 0, 3200, 3300, 10, objtype.HuntVisLineOfSight)
	if it.mode != NpcIteratorHuntAll {
		t.Errorf("mode: got %v, want NpcIteratorHuntAll", it.mode)
	}
	if it.creationTick != 99 {
		t.Errorf("creationTick: got %d, want 99", it.creationTick)
	}
	if it.distance != 10 {
		t.Errorf("distance: got %d, want 10", it.distance)
	}
	if it.huntvis != objtype.HuntVisLineOfSight {
		t.Errorf("huntvis: got %d, want HuntVisLineOfSight (%d)", it.huntvis, objtype.HuntVisLineOfSight)
	}
	if it.typeID != -1 {
		t.Errorf("typeID: got %d, want -1 (HuntAll has no type filter)", it.typeID)
	}
	if it.minZoneX != 398 || it.maxZoneX != 402 {
		t.Errorf("X bounds: got [%d, %d], want [398, 402]", it.minZoneX, it.maxZoneX)
	}
	if it.minZoneZ != 410 || it.maxZoneZ != 414 {
		t.Errorf("Z bounds: got [%d, %d], want [410, 414]", it.minZoneZ, it.maxZoneZ)
	}
	if it.curZoneX != 402 || it.curZoneZ != 414 {
		t.Errorf("cursor: got (%d, %d), want (402, 414) (start at max,max)", it.curZoneX, it.curZoneZ)
	}
}

func TestPassesFilter_HuntAllMode_HuntVisOff_AdmitsInRange(t *testing.T) {
	npc := &mockNpc{x: 3203, z: 3300, level: 0}
	it := NewHuntAllNpcIterator(nil, nil, 0, 0, 3200, 3300, 5, objtype.HuntVisOff)
	if !it.passesFilter(npc) {
		t.Errorf("passesFilter(in-range, HuntVisOff): got false, want true")
	}
}

func TestPassesFilter_HuntAllMode_OutsideDistance_Rejected(t *testing.T) {
	// dist 6 > distance 5 → reject regardless of huntvis.
	npc := &mockNpc{x: 3206, z: 3300, level: 0}
	// Even with a stub validator that would otherwise approve LoS — distance gate fires first.
	stub := &stubLineValidator{losReturn: true, lowReturn: true}
	it := NewHuntAllNpcIterator(nil, stub, 0, 0, 3200, 3300, 5, objtype.HuntVisLineOfSight)
	if it.passesFilter(npc) {
		t.Errorf("passesFilter(out-of-range): got true, want false")
	}
}

func TestPassesFilter_HuntAllMode_LineOfSight_RejectsBlocked(t *testing.T) {
	// In-distance NPC, but LoS validator returns false → reject.
	npc := &mockNpc{x: 3203, z: 3300, level: 0}
	stub := &stubLineValidator{losReturn: false, lowReturn: true}
	it := NewHuntAllNpcIterator(nil, stub, 0, 0, 3200, 3300, 5, objtype.HuntVisLineOfSight)
	if it.passesFilter(npc) {
		t.Errorf("passesFilter(LoS-blocked): got true, want false")
	}
}

func TestPassesFilter_HuntAllMode_LineOfWalk_AdmitsClear(t *testing.T) {
	npc := &mockNpc{x: 3203, z: 3300, level: 0}
	stub := &stubLineValidator{losReturn: false, lowReturn: true}
	it := NewHuntAllNpcIterator(nil, stub, 0, 0, 3200, 3300, 5, objtype.HuntVisLineOfWalk)
	if !it.passesFilter(npc) {
		t.Errorf("passesFilter(LoW-clear): got false, want true")
	}
}

func TestPassesFilter_HuntAllMode_NilValidator_Allows(t *testing.T) {
	// Nil lineValidator → pessimistically allow even with huntvis=LoS/LoW.
	npc := &mockNpc{x: 3203, z: 3300, level: 0}
	it := NewHuntAllNpcIterator(nil, nil, 0, 0, 3200, 3300, 5, objtype.HuntVisLineOfSight)
	if !it.passesFilter(npc) {
		t.Errorf("passesFilter(nil-validator, LoS): got false, want true (pessimistic-allow)")
	}
	it2 := NewHuntAllNpcIterator(nil, nil, 0, 0, 3200, 3300, 5, objtype.HuntVisLineOfWalk)
	if !it2.passesFilter(npc) {
		t.Errorf("passesFilter(nil-validator, LoW): got false, want true (pessimistic-allow)")
	}
}

// TestNpcIterator_PassesFilter_HuntAllMode_LineOfSight_IteratorAsSrc pins
// the TS-asymmetric arg order: NpcHuntAllCommandIterator passes iterator-as-src
// + npc-as-dest (ScriptIterators.ts:284), REVERSE of PlayerHuntAll.
func TestNpcIterator_PassesFilter_HuntAllMode_LineOfSight_IteratorAsSrc(t *testing.T) {
	t.Parallel()
	rec := &recordingLineValidator{losReturn: true}
	it := NewHuntAllNpcIterator(nil, rec, 0, 0, 3200, 3200, 8, objtype.HuntVisLineOfSight)
	npc := &mockNpc{x: 3201, z: 3202, level: 0}
	_ = it.passesFilter(npc)
	// Iterator-center should be SRC (3200, 3200); NPC should be DEST (3201, 3202).
	if rec.losSrcX != 3200 || rec.losSrcZ != 3200 {
		t.Errorf("LoS src: got (%d,%d), want (3200,3200) — iterator-center coords", rec.losSrcX, rec.losSrcZ)
	}
	if rec.losDestX != 3201 || rec.losDestZ != 3202 {
		t.Errorf("LoS dest: got (%d,%d), want (3201,3202) — NPC coords", rec.losDestX, rec.losDestZ)
	}
}

// TestNpcIterator_LineValidatorArgShape pins the TS-canonical
// (srcSize=1, destWidth=1, destLength=1, extraFlag=0) arg tuple at both
// LOS and LOW branches of NpcIterator (npc_iterator.go lines 127, 139).
// Mirrors NAI-165-D-LOW-ARG-SHAPE-FIX semantics, applied to the iterator
// family. NAI-166-D-LOW-ARG-SHAPE-SWEEP.
//
// TS canonical: ScriptIterators.ts:284 (LOS), :287 (LOW) — both route
// through the GameMap.ts:425-431 wrappers (1, 1, 1, 1, 0). NpcHuntAll
// passes iterator-as-src + npc-as-dest (REVERSE of PlayerHuntAll).
func TestNpcIterator_LineValidatorArgShape(t *testing.T) {
	t.Parallel()
	// LOS branch
	stubLOS := &stubLineValidatorArgs{losReturn: true}
	itLOS := NewHuntAllNpcIterator(nil, stubLOS, 0, 0, 3200, 3200, 8, objtype.HuntVisLineOfSight)
	npc := &mockNpc{x: 3201, z: 3202, level: 0}
	_ = itLOS.passesFilter(npc)
	if len(stubLOS.losCalls) != 1 {
		t.Fatalf("LOS branch: expected 1 LV call, got %d", len(stubLOS.losCalls))
	}
	got := stubLOS.losCalls[0]
	want := losCall{level: 0, srcX: 3200, srcZ: 3200, destX: 3201, destZ: 3202, srcSize: 1, destWidth: 1, destLength: 1, extraFlag: 0}
	if got != want {
		t.Fatalf("LOS arg shape:\n got=%+v\nwant=%+v", got, want)
	}

	// LOW branch
	stubLOW := &stubLineValidatorArgs{lowReturn: true}
	itLOW := NewHuntAllNpcIterator(nil, stubLOW, 0, 0, 3200, 3200, 8, objtype.HuntVisLineOfWalk)
	_ = itLOW.passesFilter(npc)
	if len(stubLOW.lowCalls) != 1 {
		t.Fatalf("LOW branch: expected 1 LV call, got %d", len(stubLOW.lowCalls))
	}
	got = stubLOW.lowCalls[0]
	if got != want {
		t.Fatalf("LOW arg shape:\n got=%+v\nwant=%+v", got, want)
	}
}
