package script

import (
	"testing"
)

func TestNpcIterator_StaleCheck(t *testing.T) {
	it := &NpcIterator{creationTick: 100}
	if it.Stale(100) {
		t.Error("Stale(creationTick) should be false")
	}
	if !it.Stale(101) {
		t.Error("Stale(creationTick+1) should be true")
	}
	if !it.Stale(99) {
		t.Error("Stale(creationTick-1) should be true (any !=)")
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
