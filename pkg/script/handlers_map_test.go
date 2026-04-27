package script

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
)

// MAP_PLAYERCOUNT (opcode 1015) — NAI-35-T2.
// Mirrors TS ServerOps.ts:27-45. Pops two coords (rect bounds) and pushes
// the count of players whose (x, z) falls inside the rect on from.level.

func TestHandleMapPlayerCount_EmptyRect(t *testing.T) {
	sf := newSingleOp("map_playercount_empty", OpMapPlayerCount)
	state := Init(sf, nil, false, nil, nil)
	state.PlayerLookup = &mockPlayerLookup{}
	state.PushInt(coordgrid.PackCoord(0, 100, 100)) // c1
	state.PushInt(coordgrid.PackCoord(0, 110, 110)) // c2 (top of stack)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 0 {
		t.Errorf("count: got %d, want 0", got)
	}
}

func TestHandleMapPlayerCount_SinglePlayerInRect(t *testing.T) {
	p := &mockPlayer{x: 105, z: 105}
	lookup := &mockPlayerLookup{
		byZone: map[zoneKey][]ActivePlayer{
			{0, (105 >> 3) << 3, (105 >> 3) << 3}: {p},
		},
	}
	sf := newSingleOp("map_playercount_one", OpMapPlayerCount)
	state := Init(sf, nil, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(coordgrid.PackCoord(0, 100, 100))
	state.PushInt(coordgrid.PackCoord(0, 110, 110))
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 1 {
		t.Errorf("count: got %d, want 1", got)
	}
}

func TestHandleMapPlayerCount_PlayerAtRectBoundary(t *testing.T) {
	// Inclusive boundary at fromX (TS line 36: x >= from.x).
	p := &mockPlayer{x: 100, z: 105}
	lookup := &mockPlayerLookup{
		byZone: map[zoneKey][]ActivePlayer{
			{0, (100 >> 3) << 3, (105 >> 3) << 3}: {p},
		},
	}
	sf := newSingleOp("map_playercount_boundary", OpMapPlayerCount)
	state := Init(sf, nil, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(coordgrid.PackCoord(0, 100, 100))
	state.PushInt(coordgrid.PackCoord(0, 110, 110))
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 1 {
		t.Errorf("inclusive-boundary count: got %d, want 1", got)
	}
}

func TestHandleMapPlayerCount_PlayerOutsideRect(t *testing.T) {
	p := &mockPlayer{x: 95, z: 95}
	lookup := &mockPlayerLookup{
		byZone: map[zoneKey][]ActivePlayer{
			{0, (95 >> 3) << 3, (95 >> 3) << 3}: {p},
		},
	}
	sf := newSingleOp("map_playercount_outside", OpMapPlayerCount)
	state := Init(sf, nil, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(coordgrid.PackCoord(0, 100, 100))
	state.PushInt(coordgrid.PackCoord(0, 110, 110))
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 0 {
		t.Errorf("count: got %d, want 0", got)
	}
}

func TestHandleMapPlayerCount_CrossLevelRectIgnoresToLevel(t *testing.T) {
	// NAI-35-D1: TS uses from.level only; to.level is silently ignored.
	// Player on level 1, from.level=0 → NOT counted (level-0 zones are
	// empty in this fixture).
	p := &mockPlayer{x: 105, z: 105}
	lookup := &mockPlayerLookup{
		byZone: map[zoneKey][]ActivePlayer{
			{1, (105 >> 3) << 3, (105 >> 3) << 3}: {p},
		},
	}
	sf := newSingleOp("map_playercount_d1", OpMapPlayerCount)
	state := Init(sf, nil, false, nil, nil)
	state.PlayerLookup = lookup
	state.PushInt(coordgrid.PackCoord(0, 100, 100)) // from.level = 0
	state.PushInt(coordgrid.PackCoord(1, 110, 110)) // to.level = 1 (ignored)
	if err := Execute(state); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := state.PopInt(); got != 0 {
		t.Errorf("cross-level count (D1): got %d, want 0", got)
	}
}
