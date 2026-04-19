package grid

import "testing"

func TestAddAndNearbyReturnsPlayer(t *testing.T) {
	g := New()
	g.Add(5, 3094, 3106, 0)

	near := g.NearbyPlayers(3094, 3106, 0, 1)
	if len(near) != 1 || near[0] != 5 {
		t.Errorf("NearbyPlayers: got %v, want [5]", near)
	}
}

func TestRemoveRemovesPlayer(t *testing.T) {
	g := New()
	g.Add(5, 3094, 3106, 0)
	g.Remove(5, 3094, 3106, 0)

	near := g.NearbyPlayers(3094, 3106, 0, 1)
	if len(near) != 0 {
		t.Errorf("after remove: got %v, want empty", near)
	}
}

func TestLevelFilter(t *testing.T) {
	g := New()
	g.Add(5, 3094, 3106, 0)
	g.Add(6, 3094, 3106, 1)

	level0 := g.NearbyPlayers(3094, 3106, 0, 1)
	if len(level0) != 1 || level0[0] != 5 {
		t.Errorf("level 0: got %v, want [5]", level0)
	}
	level1 := g.NearbyPlayers(3094, 3106, 1, 1)
	if len(level1) != 1 || level1[0] != 6 {
		t.Errorf("level 1: got %v, want [6]", level1)
	}
}

func TestRadiusBoundary(t *testing.T) {
	g := New()
	// Add player exactly 3 zones east (24 tiles).
	g.Add(5, 3094+24, 3106, 0)

	in := g.NearbyPlayers(3094, 3106, 0, 3)
	if len(in) != 1 {
		t.Errorf("radius 3 should include: got %v", in)
	}
	out := g.NearbyPlayers(3094, 3106, 0, 2)
	if len(out) != 0 {
		t.Errorf("radius 2 should exclude: got %v", out)
	}
}
