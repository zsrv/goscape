package zone

import "testing"

func TestZoneIndexRoundTrip(t *testing.T) {
	// Tile coord (3094, 3106, 0) → zone (386, 388, 0) → index.
	// UnpackIndex returns tile-unit coords at the zone's SW corner: (386<<3, 388<<3, 0) = (3088, 3104, 0).
	idx := ZoneIndex(3094, 3106, 0)
	x, z, level := UnpackIndex(idx)
	if x != 3088 || z != 3104 || level != 0 {
		t.Errorf("roundtrip: got (%d,%d,%d), want (3088,3104,0)", x, z, level)
	}
}

func TestZoneIndexLevelMatters(t *testing.T) {
	if ZoneIndex(0, 0, 0) == ZoneIndex(0, 0, 1) {
		t.Error("zones at different levels must have different indexes")
	}
}

func TestZoneMapGetCreatesOnce(t *testing.T) {
	m := NewZoneMap()
	z1 := m.Get(0, 3094, 3106)
	z2 := m.Get(0, 3094, 3106)
	if z1 != z2 {
		t.Error("two Gets at the same coord should return the same Zone pointer")
	}
	if z1 == nil {
		t.Fatal("Get should never return nil")
	}
	if z1.X != 386 || z1.Z != 388 || z1.Level != 0 {
		t.Errorf("zone coords: got (%d,%d,%d), want (386,388,0)", z1.X, z1.Z, z1.Level)
	}
}

func TestZoneMapGetByIndex(t *testing.T) {
	m := NewZoneMap()
	idx := ZoneIndex(3094, 3106, 0)
	z := m.GetByIndex(idx)
	if z.Index != idx {
		t.Errorf("zone.Index: got %d, want %d", z.Index, idx)
	}
}

func TestZoneMapGridPerLevel(t *testing.T) {
	m := NewZoneMap()
	if m.Grid(0) == m.Grid(1) {
		t.Error("Grid(0) and Grid(1) should be distinct instances")
	}
	if m.Grid(0) != m.Grid(0) {
		t.Error("Grid(0) called twice should return the same instance")
	}
}

func TestZoneMapZoneCount(t *testing.T) {
	m := NewZoneMap()
	m.Get(0, 0, 0)
	m.Get(0, 100, 100)
	m.Get(0, 0, 0) // same as first
	if m.ZoneCount() != 2 {
		t.Errorf("ZoneCount: got %d, want 2", m.ZoneCount())
	}
}
