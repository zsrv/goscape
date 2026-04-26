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
	if b.npcs[50].Observers != 0 {
		t.Errorf("AddNpc(50, 100): Observers = %d, want 0 (persistent counter init)", b.npcs[50].Observers)
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
	b.RemoveNpc(8192) // out-of-range upper bound
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

func TestComputePlayer_WritesAllFields(t *testing.T) {
	b := New()
	b.AddPlayer(5)
	say := "hello"
	msgBytes := []byte{0x10, 0x20}

	b.ComputePlayer(5,
		/*x*/ 50, /*level*/ 0, /*z*/ 60,
		/*originX*/ 48, /*originZ*/ 56,
		/*tele*/ true, /*jump*/ false,
		/*runDir*/ 1, /*walkDir*/ 2,
		/*visibility*/ VisibilitySoft,
		/*active*/ true,
		/*masks*/ 0xff,
		/*appearance*/ []byte{0x01, 0x02, 0x03},
		/*lastAppearance*/ 100,
		/*faceEntity*/ 9, /*faceX*/ 10, /*faceZ*/ 11,
		/*orientationX*/ 12, /*orientationZ*/ 13,
		/*damageTaken*/ 7, /*damageType*/ 1,
		/*currentHitpoints*/ 90, /*baseHitpoints*/ 99,
		/*animID*/ 808, /*animDelay*/ 0,
		/*say*/ &say,
		/*message*/ msgBytes, /*color*/ 1, /*effect*/ 2, /*ignored*/ 3,
		/*graphicID*/ 200, /*graphicHeight*/ 92, /*graphicDelay*/ 0,
		/*exactStartX*/ 30, /*exactStartZ*/ 31,
		/*exactEndX*/ 32, /*exactEndZ*/ 33,
		/*exactMoveStart*/ 34, /*exactMoveEnd*/ 35, /*exactMoveDirection*/ 36,
	)

	p := b.players[5]
	if p == nil {
		t.Fatal("ComputePlayer: slot nilled")
	}
	expectCoord := coordgrid.PackCoord(0, 50, 60)
	if p.Coord != expectCoord {
		t.Errorf("Coord: got %d, want %d", p.Coord, expectCoord)
	}
	expectOrigin := coordgrid.PackCoord(0, 48, 56)
	if p.Origin != expectOrigin {
		t.Errorf("Origin: got %d, want %d", p.Origin, expectOrigin)
	}
	if !p.Tele || p.Jump {
		t.Errorf("Tele/Jump: got (%v, %v), want (true, false)", p.Tele, p.Jump)
	}
	if p.RunDir != 1 || p.WalkDir != 2 {
		t.Errorf("RunDir/WalkDir: got (%d, %d)", p.RunDir, p.WalkDir)
	}
	if p.Visibility != VisibilitySoft {
		t.Errorf("Visibility: got %d, want VisibilitySoft", p.Visibility)
	}
	if !p.Active {
		t.Error("Active: got false, want true")
	}
	if p.Masks != 0xff {
		t.Errorf("Masks: got %d", p.Masks)
	}
	if len(p.Appearance) != 3 {
		t.Errorf("Appearance: got %v", p.Appearance)
	}
	if p.LastAppearance != 100 {
		t.Errorf("LastAppearance: got %d", p.LastAppearance)
	}
	if p.FaceEntity != 9 || p.FaceX != 10 || p.FaceZ != 11 {
		t.Errorf("Face*: got (%d,%d,%d)", p.FaceEntity, p.FaceX, p.FaceZ)
	}
	if p.OrientationX != 12 || p.OrientationZ != 13 {
		t.Errorf("Orientation*: got (%d,%d)", p.OrientationX, p.OrientationZ)
	}
	if p.DamageTaken != 7 || p.DamageType != 1 {
		t.Errorf("Damage*: got (%d,%d)", p.DamageTaken, p.DamageType)
	}
	if p.CurrentHitpoints != 90 || p.BaseHitpoints != 99 {
		t.Errorf("Hitpoints: got (%d/%d)", p.CurrentHitpoints, p.BaseHitpoints)
	}
	if p.AnimID != 808 || p.AnimDelay != 0 {
		t.Errorf("Anim*: got (%d,%d)", p.AnimID, p.AnimDelay)
	}
	if p.Say == nil || *p.Say != "hello" {
		t.Errorf("Say: got %v", p.Say)
	}
	if p.Chat == nil || p.Chat.Color != 1 || p.Chat.Effect != 2 || p.Chat.Ignored != 3 {
		t.Errorf("Chat: got %+v", p.Chat)
	}
	if p.GraphicID != 200 || p.GraphicHeight != 92 || p.GraphicDelay != 0 {
		t.Errorf("Graphic*: got (%d,%d,%d)", p.GraphicID, p.GraphicHeight, p.GraphicDelay)
	}
	if p.ExactMove == nil || p.ExactMove.StartX != 30 || p.ExactMove.Dir != 36 {
		t.Errorf("ExactMove: got %+v", p.ExactMove)
	}
}

func TestComputePlayer_NilSlotIsNoop(t *testing.T) {
	b := New()
	// pid 5 not added — players[5] is nil.
	b.ComputePlayer(5, 50, 0, 60, 48, 56,
		false, false, -1, -1, VisibilityDefault, false, 0,
		nil, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1,
		nil, nil, 0, 0, 0, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1)
	if b.players[5] != nil {
		t.Error("ComputePlayer on nil slot allocated player")
	}
}

func TestComputePlayer_NegativePIDIsNoop(t *testing.T) {
	b := New()
	b.AddPlayer(5)
	b.ComputePlayer(-1, 50, 0, 60, 48, 56,
		false, false, -1, -1, VisibilityDefault, false, 0,
		nil, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1,
		nil, nil, 0, 0, 0, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1)
	// no panic
}

func TestComputePlayer_NilSayBytesAndMessageProduceNilSubstructs(t *testing.T) {
	b := New()
	b.AddPlayer(5)
	b.ComputePlayer(5, 50, 0, 60, 48, 56,
		false, false, -1, -1, VisibilityDefault, false, 0,
		nil, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1,
		nil /*say*/, nil /*message*/, 0, 0, 0, -1, -1, -1,
		-1 /*exactStartX*/, -1, -1, -1, -1, -1, -1)
	p := b.players[5]
	if p.Say != nil {
		t.Error("nil say argument produced non-nil Say")
	}
	if p.Chat != nil {
		t.Error("nil message argument produced non-nil Chat")
	}
	if p.ExactMove != nil {
		t.Error("exactStartX=-1 sentinel produced non-nil ExactMove (mirrors upstream lib.rs:90-103)")
	}
}

func TestComputePlayer_CrossZoneMoveUpdatesZoneMap(t *testing.T) {
	b := New()
	b.AddPlayer(5)
	// Tick 1: place at (50, 0, 50). Zone is (50>>3=6, 0, 50>>3=6).
	b.ComputePlayer(5, 50, 0, 50, 48, 48, false, false, -1, -1,
		VisibilityDefault, true, 0, nil, -1, -1, -1, -1, -1, -1, -1,
		-1, -1, -1, -1, -1, nil, nil, 0, 0, 0, -1, -1, -1,
		-1, -1, -1, -1, -1, -1, -1)
	if _, ok := b.zoneMap.Zone(50, 0, 50).players[5]; !ok {
		t.Fatal("after tick 1: zoneMap zone(50,0,50) should contain pid 5")
	}

	// Tick 2: cross-zone move to (64, 0, 50). Zone is (64>>3=8, 0, 6).
	b.ComputePlayer(5, 64, 0, 50, 48, 48, false, false, -1, -1,
		VisibilityDefault, true, 0, nil, -1, -1, -1, -1, -1, -1, -1,
		-1, -1, -1, -1, -1, nil, nil, 0, 0, 0, -1, -1, -1,
		-1, -1, -1, -1, -1, -1, -1)

	if _, ok := b.zoneMap.Zone(50, 0, 50).players[5]; ok {
		t.Error("after cross-zone move: old zone (50,0,50) still contains pid 5")
	}
	if _, ok := b.zoneMap.Zone(64, 0, 50).players[5]; !ok {
		t.Error("after cross-zone move: new zone (64,0,50) missing pid 5")
	}
}

func TestComputePlayer_SameZoneMoveDoesNotTouchZoneMap(t *testing.T) {
	b := New()
	b.AddPlayer(5)
	// Tick 1: place at (50, 0, 50). Zone (6, 0, 6).
	b.ComputePlayer(5, 50, 0, 50, 48, 48, false, false, -1, -1,
		VisibilityDefault, true, 0, nil, -1, -1, -1, -1, -1, -1, -1,
		-1, -1, -1, -1, -1, nil, nil, 0, 0, 0, -1, -1, -1,
		-1, -1, -1, -1, -1, -1, -1)

	// Tick 2: same-zone move to (55, 0, 50). 55>>3=6 — same zone.
	b.ComputePlayer(5, 55, 0, 50, 48, 48, false, false, -1, -1,
		VisibilityDefault, true, 0, nil, -1, -1, -1, -1, -1, -1, -1,
		-1, -1, -1, -1, -1, nil, nil, 0, 0, 0, -1, -1, -1,
		-1, -1, -1, -1, -1, -1, -1)

	if _, ok := b.zoneMap.Zone(50, 0, 50).players[5]; !ok {
		t.Error("after same-zone move: zone (6,0,6) lost pid 5 (zoneMap should be untouched)")
	}
	if b.players[5].Coord != coordgrid.PackCoord(0, 55, 50) {
		t.Errorf("Coord not updated: got %d, want %d", b.players[5].Coord, coordgrid.PackCoord(0, 55, 50))
	}
}

func TestComputePlayer_AlwaysPushesPlayerGrid(t *testing.T) {
	// Mirrors upstream lib.rs:151 — the player_grid push is unconditional;
	// it happens regardless of whether the move crossed a zone boundary.
	b := New()
	b.AddPlayer(5)
	b.ComputePlayer(5, 50, 0, 50, 48, 48, false, false, -1, -1,
		VisibilityDefault, true, 0, nil, -1, -1, -1, -1, -1, -1, -1,
		-1, -1, -1, -1, -1, nil, nil, 0, 0, 0, -1, -1, -1,
		-1, -1, -1, -1, -1, -1, -1)

	key := uint32(coordgrid.PackCoord(0, 50, 50))
	if got := b.playerGrid[key]; len(got) != 1 || got[0] != 5 {
		t.Errorf("playerGrid[%d]: got %v, want [5]", key, got)
	}

	// Same-zone move pushes the new tile too.
	b.ComputePlayer(5, 55, 0, 50, 48, 48, false, false, -1, -1,
		VisibilityDefault, true, 0, nil, -1, -1, -1, -1, -1, -1, -1,
		-1, -1, -1, -1, -1, nil, nil, 0, 0, 0, -1, -1, -1,
		-1, -1, -1, -1, -1, -1, -1)
	newKey := uint32(coordgrid.PackCoord(0, 55, 50))
	if got := b.playerGrid[newKey]; len(got) != 1 || got[0] != 5 {
		t.Errorf("playerGrid[%d] after second compute: got %v, want [5]", newKey, got)
	}
}
