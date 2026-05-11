package collision

import "testing"

// TestIsIndoors pins the predicate against the FlagRoof bit. Mirrors
// TS isIndoors (GameMap.ts:417-419) which calls isFlagged(...,
// CollisionFlag.ROOF).
//
// Adaptation vs plan: flags are typed as int (not uint32) per the
// package convention at pkg/pathfinder/collision/flag.go.
func TestIsIndoors(t *testing.T) {
	cases := []struct {
		name string
		flag int
		want bool
	}{
		{"open-tile-no-roof", FlagOpen, false},
		{"roof-only", FlagRoof, true},
		{"roof-plus-blockwalk", FlagRoof | FlagBlockWalk, true},
		{"blockwalk-only", FlagBlockWalk, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsIndoors(tc.flag); got != tc.want {
				t.Errorf("IsIndoors(%#x) = %v, want %v", tc.flag, got, tc.want)
			}
		})
	}
}
