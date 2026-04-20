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
