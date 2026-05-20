package objtype

import "testing"

// TestHitTypeConstants pins the three wire values + count sentinel.
// Mirrors TS Engine-TS/src/engine/entity/HitType.ts:1-5
// (BLOCK=0, DAMAGE=1, POISON=2).
func TestHitTypeConstants(t *testing.T) {
	cases := []struct {
		name string
		got  int
		want int
	}{
		{"HitTypeBlock", HitTypeBlock, 0},
		{"HitTypeDamage", HitTypeDamage, 1},
		{"HitTypePoison", HitTypePoison, 2},
		{"HitTypeCount", HitTypeCount, 3},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, c.got, c.want)
		}
	}
}
