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
