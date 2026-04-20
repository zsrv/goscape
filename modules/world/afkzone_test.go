package world

import "testing"

func TestPackUnpackAfkCoord(t *testing.T) {
	got := packAfkCoord(0, 3084, 3096)
	x, z := unpackAfkCoord(got)
	if x != 3084 || z != 3096 {
		t.Errorf("roundtrip: got (%d,%d), want (3084,3096)", x, z)
	}
}

func TestRectsIntersect(t *testing.T) {
	if !rectsIntersect(100, 100, 1, 1, 95, 95, 21, 21) {
		t.Error("point (100,100) should intersect 21×21 rect at (95,95)")
	}
	if rectsIntersect(200, 200, 1, 1, 95, 95, 21, 21) {
		t.Error("point (200,200) should NOT intersect 21×21 rect at (95,95)")
	}
}

func TestAfkZoneIncrementsWhileStill(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0
	for i := 0; i < 5; i++ {
		p.updateAfkZones()
	}
	if p.lastAfkZone != 5 {
		t.Errorf("lastAfkZone: got %d, want 5", p.lastAfkZone)
	}
}

func TestAfkZoneRecentersOnLeave(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0
	p.updateAfkZones()
	oldZone0 := p.afkZones[0]
	// Move 25 tiles — outside the 21×21 window.
	p.x = 3094 + 25
	p.updateAfkZones()
	if p.afkZones[0] == oldZone0 {
		t.Error("afkZones[0] should have been recentered after moving out")
	}
	if p.afkZones[1] != oldZone0 {
		t.Errorf("afkZones[1] should have received the old zone[0]; got %d want %d", p.afkZones[1], oldZone0)
	}
	if p.lastAfkZone != 0 {
		t.Errorf("lastAfkZone should reset to 0 on recenter; got %d", p.lastAfkZone)
	}
}

func TestAfkZoneSaturatesAt1000(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0
	for i := 0; i < 1500; i++ {
		p.updateAfkZones()
	}
	if p.lastAfkZone != 1000 {
		t.Errorf("lastAfkZone: got %d, want 1000", p.lastAfkZone)
	}
	if !p.IsZonesAfk() {
		t.Error("IsZonesAfk() should return true at 1000")
	}
}

func TestAfkZoneInstantJumpUsesNewCoord(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0
	p.updateAfkZones()
	// Move out.
	p.x = 3094 + 25
	p.moveSpeed = MoveSpeedInstant
	p.jump = true
	p.updateAfkZones()
	if p.afkZones[0] != p.afkZones[1] {
		t.Errorf("instant+jump should put the same new coord in both slots; got [%d, %d]", p.afkZones[0], p.afkZones[1])
	}
}
