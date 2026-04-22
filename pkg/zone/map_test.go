package zone

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/entity"
)

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
	idx := coordgrid.ZoneIndex(3094, 3106, 0)
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
	g0a := m.Grid(0)
	g0b := m.Grid(0)
	if g0a != g0b {
		t.Error("Grid(0) called twice should return the same cached instance")
	}
}

func TestZoneMapCountsAggregateAcrossZones(t *testing.T) {
	m := NewZoneMap()
	zA := m.Get(0, 0, 0)
	zB := m.Get(0, 100, 100)
	// Inject Locs/Objs directly; we're testing aggregation, not mutation APIs.
	zA.Locs = make([]*entity.Loc, 2)
	zB.Objs = make([]*entity.Obj, 3)
	if got := m.ZoneCount(); got != 2 {
		t.Errorf("ZoneCount: got %d, want 2", got)
	}
	if got := m.LocCount(); got != 2 {
		t.Errorf("LocCount: got %d, want 2", got)
	}
	if got := m.ObjCount(); got != 3 {
		t.Errorf("ObjCount: got %d, want 3", got)
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

func TestNearbyZonesRadius0ReturnsCenter(t *testing.T) {
	m := NewZoneMap()
	center := m.Get(0, 3094, 3106) // materialise center zone
	zones := m.NearbyZones(0, 3094, 3106, 0)
	if len(zones) != 1 || zones[0] != center {
		t.Errorf("NearbyZones radius 0: got %d zones, want [center]", len(zones))
	}
}

func TestNearbyZonesRadius1ReturnsUpTo9(t *testing.T) {
	m := NewZoneMap()
	// Materialise 3x3 around (3094, 3106) level 0.
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			m.Get(0, 3094+dx*8, 3106+dz*8)
		}
	}
	zones := m.NearbyZones(0, 3094, 3106, 1)
	if len(zones) != 9 {
		t.Errorf("NearbyZones radius 1: got %d zones, want 9", len(zones))
	}
}

func TestNearbyZonesSkipsUnmaterialisedZones(t *testing.T) {
	m := NewZoneMap()
	center := m.Get(0, 3094, 3106)
	east := m.Get(0, 3094+8, 3106) // one east, zoneRadius 1 neighbour
	// The other 7 neighbours are NOT materialised.
	zones := m.NearbyZones(0, 3094, 3106, 1)
	if len(zones) != 2 {
		t.Errorf("NearbyZones: got %d zones, want 2 (only materialised)", len(zones))
	}
	// Order is deterministic (dx outer, dz inner, both ascending).
	have := map[*Zone]bool{zones[0]: true, zones[1]: true}
	if !have[center] || !have[east] {
		t.Errorf("NearbyZones: missing center or east from result")
	}
}

func TestNearbyZonesLevelFilter(t *testing.T) {
	m := NewZoneMap()
	z0 := m.Get(0, 3094, 3106)
	z1 := m.Get(1, 3094, 3106)
	level0 := m.NearbyZones(0, 3094, 3106, 0)
	level1 := m.NearbyZones(1, 3094, 3106, 0)
	if len(level0) != 1 || level0[0] != z0 {
		t.Errorf("NearbyZones level 0: got %v, want [z0]", level0)
	}
	if len(level1) != 1 || level1[0] != z1 {
		t.Errorf("NearbyZones level 1: got %v, want [z1]", level1)
	}
}

func TestNearbyZonesClampsNegativeCoords(t *testing.T) {
	m := NewZoneMap()
	m.Get(0, 0, 0) // materialise origin
	// Center at (0,0) radius 1 would naively probe (-1, 0..1) etc;
	// helper must skip negatives to avoid malformed zone indexes.
	zones := m.NearbyZones(0, 0, 0, 1)
	// Only materialised neighbour is the origin itself.
	if len(zones) != 1 {
		t.Errorf("NearbyZones near origin: got %d zones, want 1 (just origin)", len(zones))
	}
}
