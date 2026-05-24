package routefinder

import "testing"

// TestLineScaleDown_NonNegativeMatchesLogicalShift pins the L43 invariant for
// lineScaleDown: over the non-negative domain it can actually be reached with
// (a coordinate interpolated between two non-negative map tiles), Go's
// arithmetic `>>` produces the same result as the JS transliteration's
// logical `>>> 16`. The two would diverge only for a negative operand, which
// RayCast never produces.
func TestLineScaleDown_NonNegativeMatchesLogicalShift(t *testing.T) {
	values := []int{
		0,
		lineHalfTile,
		1 << 16,
		(1 << 16) + lineHalfTile,
		100 << 16,
		3200 << 16,
		16383 << 16,
		(16383 << 16) + lineHalfTile,
	}
	for _, v := range values {
		got := lineScaleDown(v)
		want := int(uint32(v) >> 16) // logical >>> 16
		if got != want {
			t.Errorf("lineScaleDown(%d) = %d, logical >>>16 = %d", v, got, want)
		}
	}
}

// TestLineScaleUpDownRoundTrip confirms scaling a tile coordinate up and back
// down is the identity for non-negative tile counts (the reachable domain).
func TestLineScaleUpDownRoundTrip(t *testing.T) {
	for tiles := 0; tiles <= 16383; tiles += 257 {
		if got := lineScaleDown(lineScaleUp(tiles)); got != tiles {
			t.Errorf("lineScaleDown(lineScaleUp(%d)) = %d, want %d", tiles, got, tiles)
		}
	}
}
