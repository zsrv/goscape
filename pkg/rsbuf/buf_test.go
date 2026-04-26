package rsbuf

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
)

// packPlayerCoord wraps pkg/coordgrid.PackCoord with the test's preferred
// argument order (x, level, z) for symmetry with rsbuf's internal *Buf.Zone
// argument order. Test-only.
func packPlayerCoord(x, level, z int) int {
	return coordgrid.PackCoord(level, x, z)
}

func TestNew_ZeroInit(t *testing.T) {
	b := New()
	if b == nil {
		t.Fatal("New returned nil")
	}
	for pid := range int32(2048) {
		if b.players[pid] != nil {
			t.Errorf("New: players[%d] non-nil", pid)
			break
		}
	}
	for nid := range int32(8192) {
		if b.npcs[nid] != nil {
			t.Errorf("New: npcs[%d] non-nil", nid)
			break
		}
	}
	if b.zoneMap == nil {
		t.Error("New: zoneMap nil")
	}
	if b.playerGrid == nil {
		t.Error("New: playerGrid nil")
	}
}

func TestAddPlayer_AllocatesSlot(t *testing.T) {
	b := New()
	b.AddPlayer(5)
	if b.players[5] == nil {
		t.Fatal("AddPlayer(5): slot still nil")
	}
	if b.players[5].PID != 5 {
		t.Errorf("AddPlayer(5): players[5].PID = %d, want 5", b.players[5].PID)
	}
	if b.players[5].Build == nil {
		t.Error("AddPlayer(5): players[5].Build nil — should be initialized BuildArea")
	}
	if b.players[5].RunDir != -1 {
		t.Errorf("AddPlayer(5): players[5].RunDir = %d, want -1 (sentinel default)", b.players[5].RunDir)
	}
}

func TestAddPlayer_NegativeIDIsNoop(t *testing.T) {
	b := New()
	b.AddPlayer(-1)
	// no panic; no observable side effect
}

func TestAddPlayer_OutOfRangeIsNoop(t *testing.T) {
	b := New()
	b.AddPlayer(2048) // >= len
	b.AddPlayer(99999)
	// no panic; no observable side effect
}

func TestAddPlayer_DoubleAddOverwrites(t *testing.T) {
	// Mirrors upstream lib.rs:179-184 — assignment, not insertion check.
	b := New()
	b.AddPlayer(5)
	first := b.players[5]
	b.AddPlayer(5)
	second := b.players[5]
	if first == second {
		t.Error("double AddPlayer(5): expected new *Player, got same pointer")
	}
	if second.PID != 5 {
		t.Errorf("after re-add: players[5].PID = %d, want 5", second.PID)
	}
}

func TestRemovePlayer_NilsSlot(t *testing.T) {
	b := New()
	b.AddPlayer(5)
	b.RemovePlayer(5)
	if b.players[5] != nil {
		t.Error("after RemovePlayer(5): slot still non-nil")
	}
}

func TestRemovePlayer_AbsentIsNoop(t *testing.T) {
	b := New()
	b.RemovePlayer(5) // never added
	b.RemovePlayer(-1)
	b.RemovePlayer(2048)
	if b.players[5] != nil {
		t.Error("RemovePlayer(absent): slot mutated")
	}
}

func TestRemovePlayer_DecrementsObserverForTrackedNpcs(t *testing.T) {
	// Mirrors upstream lib.rs:194-198 — RemovePlayer iterates the
	// player's BuildArea.npcs set and decrements each npc's observer
	// count (floor 0).
	b := New()
	b.AddPlayer(5)
	b.AddNpc(10, 100)
	b.AddNpc(20, 100)
	b.npcs[10].Observers = 3
	b.npcs[20].Observers = 1
	// Hand-seed the player's tracking set with these npcs.
	b.players[5].Build.Npcs.Insert(10)
	b.players[5].Build.Npcs.Insert(20)

	b.RemovePlayer(5)

	if b.npcs[10].Observers != 2 {
		t.Errorf("npcs[10].Observers: got %d, want 2 (3-1)", b.npcs[10].Observers)
	}
	if b.npcs[20].Observers != 0 {
		t.Errorf("npcs[20].Observers: got %d, want 0 (1-1, floored)", b.npcs[20].Observers)
	}
}

func TestRemovePlayer_ObserverFloorsAtZero(t *testing.T) {
	b := New()
	b.AddPlayer(5)
	b.AddNpc(10, 100)
	b.npcs[10].Observers = 0 // already 0
	b.players[5].Build.Npcs.Insert(10)

	b.RemovePlayer(5)

	if b.npcs[10].Observers != 0 {
		t.Errorf("Observers: got %d, want 0 (floor)", b.npcs[10].Observers)
	}
}

func TestRemovePlayer_RemovesFromZoneMap(t *testing.T) {
	// Mirrors upstream lib.rs:193 — RemovePlayer removes pid from
	// the zone at the player's last coord.
	b := New()
	b.AddPlayer(5)
	// Manually set a coord so the zoneMap remove targets a specific zone.
	// (ComputePlayer would do this; we hand-set for unit isolation.)
	b.players[5].Coord = packPlayerCoord(50, 0, 50) // helper: pkg/coordgrid.PackCoord
	b.zoneMap.Zone(50, 0, 50).AddPlayer(5)

	b.RemovePlayer(5)

	if _, ok := b.zoneMap.Zone(50, 0, 50).players[5]; ok {
		t.Error("RemovePlayer: pid still in zoneMap")
	}
}

func TestAddNpc_AllocatesSlot(t *testing.T) {
	b := New()
	b.AddNpc(50, 100)
	if b.npcs[50] == nil {
		t.Fatal("AddNpc(50, 100): slot nil")
	}
	if b.npcs[50].NID != 50 {
		t.Errorf("AddNpc(50, 100): NID = %d, want 50", b.npcs[50].NID)
	}
	if b.npcs[50].NType != 100 {
		t.Errorf("AddNpc(50, 100): NType = %d, want 100", b.npcs[50].NType)
	}
	if b.npcs[50].WalkDir != -1 {
		t.Errorf("AddNpc(50, 100): WalkDir = %d, want -1 (sentinel)", b.npcs[50].WalkDir)
	}
}

func TestAddNpc_NegativeIsNoop(t *testing.T) {
	b := New()
	b.AddNpc(-1, 100)
	b.AddNpc(50, -1)
	for i := range int32(8192) {
		if b.npcs[i] != nil {
			t.Errorf("AddNpc with negative arg populated npcs[%d]", i)
			break
		}
	}
}

func TestRemoveNpc_NilsSlot(t *testing.T) {
	b := New()
	b.AddNpc(50, 100)
	b.RemoveNpc(50)
	if b.npcs[50] != nil {
		t.Error("after RemoveNpc(50): slot still non-nil")
	}
}

func TestRemoveNpc_AbsentIsNoop(t *testing.T) {
	b := New()
	b.RemoveNpc(50) // never added
	b.RemoveNpc(-1)
	if b.npcs[50] != nil {
		t.Error("RemoveNpc(absent): slot mutated")
	}
}

func TestRemoveNpc_RemovesFromZoneMap(t *testing.T) {
	b := New()
	b.AddNpc(50, 100)
	// Hand-set coord so zoneMap remove targets a specific zone.
	b.npcs[50].Coord = coordgrid.PackCoord(0, 50, 50)
	b.zoneMap.Zone(50, 0, 50).AddNpc(50)

	b.RemoveNpc(50)

	if _, ok := b.zoneMap.Zone(50, 0, 50).npcs[50]; ok {
		t.Error("RemoveNpc: nid still in zoneMap")
	}
}
