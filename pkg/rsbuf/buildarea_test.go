package rsbuf

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
)

func packCoordTest(level, x, z int) int { return coordgrid.PackCoord(level, x, z) }

func TestNewBuildArea_ZeroInit(t *testing.T) {
	b := newBuildArea()
	if b == nil {
		t.Fatal("newBuildArea returned nil")
	}
	if b.Players == nil || b.Npcs == nil {
		t.Errorf("newBuildArea: Players=%v, Npcs=%v, want both non-nil", b.Players, b.Npcs)
	}
	if b.Players.Len() != 0 || b.Npcs.Len() != 0 {
		t.Errorf("newBuildArea: Players.Len=%d, Npcs.Len=%d, want both 0", b.Players.Len(), b.Npcs.Len())
	}
	if b.ViewDistance != preferredViewDistance {
		t.Errorf("newBuildArea: ViewDistance=%d, want %d", b.ViewDistance, preferredViewDistance)
	}
	for i := range b.appearances {
		if b.appearances[i] != 0 {
			t.Errorf("newBuildArea: appearances[%d]=%d, want 0", i, b.appearances[i])
			break
		}
	}
}

func TestBuildArea_HasAppearance_FreshIsFalse(t *testing.T) {
	b := newBuildArea()
	// tick=0 is NOT tested here: appearances is zero-initialized, so
	// HasAppearance(pid, 0) returns true on a fresh BuildArea (0 == 0).
	// This matches upstream Rust has_appearance at build.rs:151-153.
	// In practice, callers guard with last_appearance != -1 before calling
	// HasAppearance, so tick=0 is never passed by the engine (info.rs:305).
	for _, tick := range []uint32{1, 100} {
		if b.HasAppearance(7, tick) {
			t.Errorf("fresh BuildArea: HasAppearance(7, %d) = true, want false", tick)
		}
	}
}

func TestBuildArea_SaveAppearance_RoundTrip(t *testing.T) {
	b := newBuildArea()
	b.SaveAppearance(7, 100)
	if !b.HasAppearance(7, 100) {
		t.Error("after SaveAppearance(7, 100), HasAppearance(7, 100) is false")
	}
	if b.HasAppearance(7, 99) {
		t.Error("HasAppearance(7, 99) is true after SaveAppearance(7, 100) — should be false (tick mismatch)")
	}
	if b.HasAppearance(7, 101) {
		t.Error("HasAppearance(7, 101) is true after SaveAppearance(7, 100) — should be false")
	}
	if b.HasAppearance(8, 100) {
		t.Error("SaveAppearance(7, 100) leaked into pid=8")
	}
}

func TestBuildArea_Cleanup_ClearsAll(t *testing.T) {
	b := newBuildArea()
	b.Players.Insert(5)
	b.Players.Insert(10)
	b.Npcs.Insert(3)
	b.SaveAppearance(7, 100)
	b.SaveAppearance(8, 200)

	b.Cleanup()

	if b.Players.Len() != 0 {
		t.Errorf("Cleanup: Players.Len=%d, want 0", b.Players.Len())
	}
	if b.Npcs.Len() != 0 {
		t.Errorf("Cleanup: Npcs.Len=%d, want 0", b.Npcs.Len())
	}
	if b.HasAppearance(7, 100) || b.HasAppearance(8, 200) {
		t.Error("Cleanup: appearances not cleared")
	}
}

func TestBuildArea_GetNearbyPlayers_EmptyZoneMapReturnsEmpty(t *testing.T) {
	ba := newBuildArea()
	zm := newZoneMap()
	var players [2048]*Player
	got := ba.GetNearbyPlayers(&players, zm, 1, 100, 0, 100)
	if len(got) != 0 {
		t.Errorf("empty zoneMap: got %v, want []", got)
	}
}

func TestBuildArea_GetNearbyPlayers_WindowMath(t *testing.T) {
	ba := newBuildArea()
	zm := newZoneMap()
	var players [2048]*Player
	// Place player 5 at (96, 0, 96): zone (12, 12). Search center (100, 0, 100): zone (12, 12).
	// preferredViewDistance=15: (100-15)>>3=10 to (100+15)>>3=14 — zone (12,12) is in range.
	players[5] = newPlayer(5)
	players[5].Coord = packCoordTest(0, 96, 96)
	players[5].PID = 5
	zm.Zone(96, 0, 96).AddPlayer(5)

	got := ba.GetNearbyPlayers(&players, zm, 1, 100, 0, 100)
	if len(got) != 1 || got[0] != 5 {
		t.Errorf("got %v, want [5]", got)
	}
}

func TestBuildArea_GetNearbyPlayers_FiltersAlreadyTracked(t *testing.T) {
	ba := newBuildArea()
	zm := newZoneMap()
	var players [2048]*Player
	players[5] = newPlayer(5)
	players[5].Coord = packCoordTest(0, 100, 100)
	players[5].PID = 5
	zm.Zone(100, 0, 100).AddPlayer(5)
	ba.Players.Insert(5) // already tracked

	got := ba.GetNearbyPlayers(&players, zm, 1, 100, 0, 100)
	if len(got) != 0 {
		t.Errorf("already-tracked excluded; got %v, want []", got)
	}
}

func TestBuildArea_GetNearbyPlayers_FiltersOutOfDistance(t *testing.T) {
	ba := newBuildArea()
	zm := newZoneMap()
	var players [2048]*Player
	players[5] = newPlayer(5)
	// (116, 0, 100): zone 14 is inside the [10,14] zone-walk window for
	// center (100, 0, 100), so filterPlayer IS called for this candidate.
	// Tile distance is max(16, 0) = 16 > preferredViewDistance(15), so
	// the !withinDistanceSW branch in filterPlayer fires.
	players[5].Coord = packCoordTest(0, 116, 100)
	players[5].PID = 5
	zm.Zone(116, 0, 100).AddPlayer(5)

	got := ba.GetNearbyPlayers(&players, zm, 1, 100, 0, 100)
	if len(got) != 0 {
		t.Errorf("out-of-distance excluded; got %v, want []", got)
	}
}

func TestBuildArea_GetNearbyPlayers_FiltersNegativePid(t *testing.T) {
	ba := newBuildArea()
	zm := newZoneMap()
	var players [2048]*Player
	players[5] = newPlayer(5)
	players[5].Coord = packCoordTest(0, 100, 100)
	players[5].PID = -1 // empty-slot marker
	zm.Zone(100, 0, 100).AddPlayer(5)

	got := ba.GetNearbyPlayers(&players, zm, 1, 100, 0, 100)
	if len(got) != 0 {
		t.Errorf("pid=-1 excluded; got %v, want []", got)
	}
}

func TestBuildArea_GetNearbyPlayers_FiltersSelf(t *testing.T) {
	ba := newBuildArea()
	zm := newZoneMap()
	var players [2048]*Player
	players[1] = newPlayer(1)
	players[1].Coord = packCoordTest(0, 100, 100)
	players[1].PID = 1
	zm.Zone(100, 0, 100).AddPlayer(1)

	got := ba.GetNearbyPlayers(&players, zm, 1, 100, 0, 100) // self pid=1
	if len(got) != 0 {
		t.Errorf("self excluded; got %v, want []", got)
	}
}

func TestBuildArea_GetNearbyPlayers_FiltersDifferentLevel(t *testing.T) {
	ba := newBuildArea()
	zm := newZoneMap()
	var players [2048]*Player
	players[5] = newPlayer(5)
	players[5].Coord = packCoordTest(1, 100, 100) // level 1
	players[5].PID = 5
	zm.Zone(100, 1, 100).AddPlayer(5)

	got := ba.GetNearbyPlayers(&players, zm, 1, 100, 0, 100) // self at level 0
	if len(got) != 0 {
		t.Errorf("different-level excluded; got %v, want []", got)
	}
}

func TestBuildArea_GetNearbyPlayers_RespectsPreferredCap(t *testing.T) {
	ba := newBuildArea()
	zm := newZoneMap()
	var players [2048]*Player
	// Insert 251 candidates in the same zone (preferredPlayers=250).
	for i := int32(2); i < 253; i++ {
		players[i] = newPlayer(i)
		players[i].Coord = packCoordTest(0, 100, 100)
		players[i].PID = i
		zm.Zone(100, 0, 100).AddPlayer(i)
	}
	got := ba.GetNearbyPlayers(&players, zm, 1, 100, 0, 100)
	if len(got) != int(preferredPlayers) {
		t.Errorf("cap respected: got len %d, want %d", len(got), preferredPlayers)
	}
}

func TestWithinDistanceSW(t *testing.T) {
	tests := []struct {
		name           string
		ax, az, bx, bz int
		radius         int
		want           bool
	}{
		{"identical", 100, 100, 100, 100, 15, true},
		{"dx_eq_radius", 115, 100, 100, 100, 15, true},
		{"dx_gt_radius", 116, 100, 100, 100, 15, false},
		{"dz_eq_radius", 100, 115, 100, 100, 15, true},
		{"dz_gt_radius", 100, 116, 100, 100, 15, false},
		{"negative_dx_within", 90, 100, 100, 100, 15, true},
		{"negative_dx_outside", 84, 100, 100, 100, 15, false},
		{"both_axes_at_limit", 115, 115, 100, 100, 15, true},
		{"chebyshev_max_governs", 115, 116, 100, 100, 15, false}, // dz=16 > 15 even though dx=15
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := withinDistanceSW(tt.ax, tt.az, tt.bx, tt.bz, tt.radius)
			if got != tt.want {
				t.Errorf("withinDistanceSW(%d,%d,%d,%d,%d) = %v, want %v",
					tt.ax, tt.az, tt.bx, tt.bz, tt.radius, got, tt.want)
			}
		})
	}
}
