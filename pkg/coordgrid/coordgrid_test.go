package coordgrid

import "testing"

func TestPackUnpackRoundTrip(t *testing.T) {
	cases := []struct{ level, x, z int }{
		{0, 3094, 3106},
		{3, 16383, 16383},
		{1, 0, 0},
		{2, 100, 200},
	}
	for _, tc := range cases {
		packed := PackCoord(tc.level, tc.x, tc.z)
		got := UnpackCoord(packed)
		if got.Level != tc.level || got.X != tc.x || got.Z != tc.z {
			t.Errorf("round-trip (%d,%d,%d): got (%d,%d,%d)", tc.level, tc.x, tc.z, got.Level, got.X, got.Z)
		}
	}
}

func TestFaceDirections(t *testing.T) {
	cases := []struct {
		name                   string
		srcX, srcZ, dstX, dstZ int
		want                   Direction
	}{
		{"north", 0, 0, 0, 1, DirectionNorth},
		{"south", 0, 1, 0, 0, DirectionSouth},
		{"east", 0, 0, 1, 0, DirectionEast},
		{"west", 1, 0, 0, 0, DirectionWest},
		{"northeast", 0, 0, 1, 1, DirectionNortheast},
		{"northwest", 1, 0, 0, 1, DirectionNorthwest},
		{"southeast", 0, 1, 1, 0, DirectionSoutheast},
		{"southwest", 1, 1, 0, 0, DirectionSouthwest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Face(tc.srcX, tc.srcZ, tc.dstX, tc.dstZ)
			if got != tc.want {
				t.Errorf("Face(%d,%d,%d,%d) = %d, want %d", tc.srcX, tc.srcZ, tc.dstX, tc.dstZ, got, tc.want)
			}
		})
	}
}

func TestDeltaXZ(t *testing.T) {
	if DeltaX(DirectionEast) != 1 || DeltaX(DirectionWest) != -1 || DeltaX(DirectionNorth) != 0 {
		t.Error("DeltaX wrong")
	}
	if DeltaZ(DirectionNorth) != 1 || DeltaZ(DirectionSouth) != -1 || DeltaZ(DirectionEast) != 0 {
		t.Error("DeltaZ wrong")
	}
}
