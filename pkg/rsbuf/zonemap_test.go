package rsbuf

import (
	"testing"
)

func TestZoneMap_ZoneCreatesOnMiss(t *testing.T) {
	m := newZoneMap()
	z := m.Zone(50, 0, 50)
	if z == nil {
		t.Fatal("Zone returned nil for unknown coord")
	}
	if len(z.players) != 0 || len(z.npcs) != 0 {
		t.Errorf("new zone: players=%d, npcs=%d, want both 0", len(z.players), len(z.npcs))
	}
}

func TestZoneMap_ZoneIsStable(t *testing.T) {
	m := newZoneMap()
	a := m.Zone(50, 0, 50)
	b := m.Zone(50, 0, 50)
	if a != b {
		t.Errorf("Zone for same coord returned different pointers: %p vs %p", a, b)
	}
}

func TestZoneMap_ZoneKeyedByZoneCoord(t *testing.T) {
	// All x=48..55 fall into the same zone (x>>3=6); zone differs only
	// when x crosses a zone boundary.
	m := newZoneMap()
	a := m.Zone(48, 0, 50)
	b := m.Zone(55, 0, 50) // same zone (55>>3=6)
	c := m.Zone(56, 0, 50) // different zone (56>>3=7)
	if a != b {
		t.Errorf("(48, 0, 50) and (55, 0, 50) should map to same zone")
	}
	if a == c {
		t.Errorf("(48, 0, 50) and (56, 0, 50) should map to different zones")
	}
}

func TestZoneMap_LevelDifferentiates(t *testing.T) {
	m := newZoneMap()
	a := m.Zone(50, 0, 50)
	b := m.Zone(50, 1, 50)
	if a == b {
		t.Errorf("zones at same (x,z) but different level returned same pointer")
	}
}

func TestZoneMap_AxisDifferentiates(t *testing.T) {
	// Confirm x and z are not transposed in packing.
	m := newZoneMap()
	a := m.Zone(8, 0, 0)  // zone (1, 0, 0)
	b := m.Zone(0, 0, 8)  // zone (0, 0, 1)
	if a == b {
		t.Errorf("(8,0,0) and (0,0,8) should map to different zones")
	}
}

func TestZone_PlayerNpcSetsIndependent(t *testing.T) {
	z := newZone()
	z.AddPlayer(5)
	z.AddNpc(5)
	if _, ok := z.players[5]; !ok {
		t.Error("AddPlayer(5): players[5] missing")
	}
	if _, ok := z.npcs[5]; !ok {
		t.Error("AddNpc(5): npcs[5] missing")
	}
	z.RemovePlayer(5)
	if _, ok := z.players[5]; ok {
		t.Error("RemovePlayer(5): players[5] still present")
	}
	if _, ok := z.npcs[5]; !ok {
		t.Error("RemovePlayer(5) leaked into npcs")
	}
}

func TestZone_RemoveAbsentIsNoop(t *testing.T) {
	z := newZone()
	z.RemovePlayer(99) // never added
	z.RemoveNpc(99)
	if len(z.players) != 0 || len(z.npcs) != 0 {
		t.Errorf("RemovePlayer/RemoveNpc(absent) populated empty sets")
	}
}

func TestZone_AddIdempotent(t *testing.T) {
	z := newZone()
	z.AddPlayer(7)
	z.AddPlayer(7)
	if len(z.players) != 1 {
		t.Errorf("double AddPlayer(7): len = %d, want 1", len(z.players))
	}
}
