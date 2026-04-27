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
