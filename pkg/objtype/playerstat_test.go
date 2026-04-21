package objtype

import "testing"

func TestGetExpByLevelKnownValues(t *testing.T) {
	cases := []struct {
		level, want int
	}{
		{1, 0},          // base case (TS returns undefined; we return 0)
		{2, 830},        // first table entry: 83 × 10
		{3, 1740},       // 174 × 10
		{10, 11540},     // 1154 × 10 — RS2 canonical level-10 XP
		{50, 1013330},   // 101333 × 10 — mid-curve sanity
		{99, 130344310}, // 13034431 × 10 — top of curve
	}
	for _, tc := range cases {
		if got := GetExpByLevel(tc.level); got != tc.want {
			t.Errorf("GetExpByLevel(%d): got %d, want %d", tc.level, got, tc.want)
		}
	}
}

func TestGetExpByLevelClampsLow(t *testing.T) {
	for _, lvl := range []int{0, -1, -100} {
		if got := GetExpByLevel(lvl); got != 0 {
			t.Errorf("GetExpByLevel(%d): got %d, want 0 (low-clamp)", lvl, got)
		}
	}
}

func TestGetExpByLevelClampsHigh(t *testing.T) {
	want := GetExpByLevel(99)
	for _, lvl := range []int{100, 200, 1000} {
		if got := GetExpByLevel(lvl); got != want {
			t.Errorf("GetExpByLevel(%d): got %d, want %d (clamp to level-99)", lvl, got, want)
		}
	}
}

func TestPlayerStatCount(t *testing.T) {
	if PlayerStatCount != 21 {
		t.Errorf("PlayerStatCount: got %d, want 21 (matches TS PlayerStat enum)", PlayerStatCount)
	}
}

func TestPlayerStatHitpointsIsThree(t *testing.T) {
	if PlayerStatHitpoints != 3 {
		t.Errorf("PlayerStatHitpoints: got %d, want 3", PlayerStatHitpoints)
	}
}
