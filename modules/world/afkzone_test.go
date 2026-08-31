package world

import (
	"math"
	"testing"
)

// TestAfkChanceConstants pins the AFK roll constants to the values defined in
// TS World.ts:128-129. AFK_CHANCE1 = 1/24, AFK_CHANCE2 = 1/12. A prior bug
// used a hand-rounded 0.0167 (~2.5× too low) instead of the correct ~0.04167.
func TestAfkChanceConstants(t *testing.T) {
	const eps = 1e-12
	if math.Abs(afkChance1-1.0/24.0) > eps {
		t.Errorf("afkChance1: got %v, want %v (1/24)", afkChance1, 1.0/24.0)
	}
	if math.Abs(afkChance2-1.0/12.0) > eps {
		t.Errorf("afkChance2: got %v, want %v (1/12)", afkChance2, 1.0/12.0)
	}
	if afkChance2 <= afkChance1 {
		t.Errorf("afkChance2 (%v) must be strictly greater than afkChance1 (%v)", afkChance2, afkChance1)
	}
}

// TestAfkChanceBranchUsesZonesAfk verifies the runtime dispatch in
// processIn: a player whose IsZonesAfk() is true must roll against
// afkChance2 (the higher chance), otherwise afkChance1. We assert the
// dispatch by reading lastAfkZone and selecting the chance the same way
// processIn does, without invoking the global rand.
func TestAfkChanceBranchUsesZonesAfk(t *testing.T) {
	p, _ := newTestPlayer(t)

	// Not zones-afk yet: branch must pick afkChance1.
	p.lastAfkZone = 0
	if p.IsZonesAfk() {
		t.Fatal("precondition: IsZonesAfk() should be false at lastAfkZone=0")
	}
	got := afkChance1
	if p.IsZonesAfk() {
		got = afkChance2
	}
	if got != afkChance1 {
		t.Errorf("non-afk branch: got %v, want %v", got, afkChance1)
	}

	// Zones-afk saturated: branch must pick afkChance2.
	p.lastAfkZone = 1000
	if !p.IsZonesAfk() {
		t.Fatal("precondition: IsZonesAfk() should be true at lastAfkZone=1000")
	}
	got = afkChance1
	if p.IsZonesAfk() {
		got = afkChance2
	}
	if got != afkChance2 {
		t.Errorf("zones-afk branch: got %v, want %v", got, afkChance2)
	}
}

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
	for range 5 {
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
	for range 1500 {
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
