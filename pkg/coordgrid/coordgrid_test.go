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

func TestPackZoneCoordCorners(t *testing.T) {
	if got := PackZoneCoord(0, 0); got != 0x00 {
		t.Errorf("(0,0): got %#x, want 0x00", got)
	}
	if got := PackZoneCoord(7, 7); got != 0x77 {
		t.Errorf("(7,7): got %#x, want 0x77", got)
	}
}

func TestPackZoneCoordWorldAbsolute(t *testing.T) {
	// (3094 & 7) == 6; (3106 & 7) == 2; byte = (6<<4) | 2 = 0x62.
	if got := PackZoneCoord(3094, 3106); got != 0x62 {
		t.Errorf("(3094,3106): got %#x, want 0x62", got)
	}
}

func TestPackZoneCoordDiscardsHighBits(t *testing.T) {
	if PackZoneCoord(3200, 3200) != PackZoneCoord(0, 0) {
		t.Error("PackZoneCoord should only look at the low 3 bits of x and z")
	}
}

func TestZoneIndexRoundTrip(t *testing.T) {
	// (3094, 3106, 0) → zone (386, 388) → packs; unpacks to tile SW corner (3088, 3104).
	idx := ZoneIndex(3094, 3106, 0)
	x, z, level := UnpackZoneIndex(idx)
	if x != 3088 || z != 3104 || level != 0 {
		t.Errorf("roundtrip: got (%d,%d,%d), want (3088,3104,0)", x, z, level)
	}
}

func TestZoneIndexDistinguishesLevels(t *testing.T) {
	if ZoneIndex(0, 0, 0) == ZoneIndex(0, 0, 1) {
		t.Error("same x/z at different levels must have distinct indexes")
	}
}

func TestFine(t *testing.T) {
	// TS CoordGrid.fine(coord, size): coord*64 + (size*64 - 1) / 2.
	// For a 1x1 entity at tile 100: fine = 100*64 + 31 = 6431.
	tests := []struct {
		name       string
		coord, siz int
		want       int
	}{
		{"1x1 at 0", 0, 1, 31},
		{"1x1 at 100", 100, 1, 6431},
		{"2x2 at 0", 0, 2, 63},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Fine(tc.coord, tc.siz); got != tc.want {
				t.Errorf("Fine(%d,%d) = %d, want %d", tc.coord, tc.siz, got, tc.want)
			}
		})
	}
}

func TestIntersects(t *testing.T) {
	cases := []struct {
		name           string
		sx, sz, sw, sl int
		dx, dz, dw, dl int
		want           bool
	}{
		{"identical 1x1", 5, 5, 1, 1, 5, 5, 1, 1, true},
		{"adjacent east 1x1", 5, 5, 1, 1, 6, 5, 1, 1, false},
		{"adjacent north 1x1", 5, 5, 1, 1, 5, 6, 1, 1, false},
		{"src contains dest", 5, 5, 3, 3, 6, 6, 1, 1, true},
		{"dest contains src", 5, 5, 1, 1, 4, 4, 3, 3, true},
		{"overlap NE corner", 5, 5, 2, 2, 6, 6, 2, 2, true},
		{"disjoint far east", 5, 5, 1, 1, 10, 5, 1, 1, false},
		{"disjoint far north", 5, 5, 1, 1, 5, 10, 1, 1, false},
		{"touching edge east", 5, 5, 2, 1, 7, 5, 1, 1, false}, // src right edge at 7, dest at 7 → touching, NOT overlap
		{"touching edge north", 5, 5, 1, 2, 5, 7, 1, 1, false},
		{"2x2 vs 2x2 overlap one tile", 5, 5, 2, 2, 6, 6, 2, 2, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Intersects(tc.sx, tc.sz, tc.sw, tc.sl, tc.dx, tc.dz, tc.dw, tc.dl)
			if got != tc.want {
				t.Errorf("Intersects(%d,%d,%d,%d, %d,%d,%d,%d) = %v, want %v",
					tc.sx, tc.sz, tc.sw, tc.sl, tc.dx, tc.dz, tc.dw, tc.dl, got, tc.want)
			}
		})
	}
}
