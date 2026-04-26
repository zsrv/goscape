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
	if p.Chat == nil || len(p.Chat.Bytes) != 2 || p.Chat.Bytes[0] != 0x10 || p.Chat.Bytes[1] != 0x20 {
		t.Errorf("Chat.Bytes: got %v, want [0x10, 0x20]", p.Chat.Bytes)
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

func TestComputeNpc_WritesAllFields(t *testing.T) {
	b := New()
	b.AddNpc(50, 100)
	say := "rwar"

	b.ComputeNpc(50, 100,
		/*x*/ 60, /*level*/ 0, /*z*/ 70,
		/*tele*/ true,
		/*runDir*/ 1, /*walkDir*/ 2,
		/*active*/ true,
		/*masks*/ 0xff,
		/*faceEntity*/ 9, /*faceX*/ 10, /*faceZ*/ 11,
		/*orientationX*/ 12, /*orientationZ*/ 13,
		/*damageTaken*/ 7, /*damageType*/ 1,
		/*currentHitpoints*/ 90, /*baseHitpoints*/ 99,
		/*animID*/ 808, /*animDelay*/ 0,
		/*say*/ &say,
		/*graphicID*/ 200, /*graphicHeight*/ 92, /*graphicDelay*/ 0,
	)

	n := b.npcs[50]
	if n == nil {
		t.Fatal("ComputeNpc: slot nilled")
	}
	if n.Coord != coordgrid.PackCoord(0, 60, 70) {
		t.Errorf("Coord: got %d", n.Coord)
	}
	if n.NID != 50 || n.NType != 100 {
		t.Errorf("NID/NType: got (%d, %d)", n.NID, n.NType)
	}
	if !n.Tele {
		t.Error("Tele: got false")
	}
	if n.RunDir != 1 || n.WalkDir != 2 {
		t.Errorf("RunDir/WalkDir: got (%d, %d)", n.RunDir, n.WalkDir)
	}
	if !n.Active {
		t.Error("Active: got false")
	}
	if n.Masks != 0xff {
		t.Errorf("Masks: got %d", n.Masks)
	}
	if n.FaceEntity != 9 || n.FaceX != 10 || n.FaceZ != 11 {
		t.Errorf("Face*: got (%d,%d,%d)", n.FaceEntity, n.FaceX, n.FaceZ)
	}
	if n.OrientationX != 12 || n.OrientationZ != 13 {
		t.Errorf("Orientation*: got (%d,%d)", n.OrientationX, n.OrientationZ)
	}
	if n.DamageTaken != 7 || n.DamageType != 1 {
		t.Errorf("Damage*: got (%d,%d)", n.DamageTaken, n.DamageType)
	}
	if n.AnimID != 808 || n.AnimDelay != 0 {
		t.Errorf("Anim*: got (%d,%d)", n.AnimID, n.AnimDelay)
	}
	if n.Say == nil || *n.Say != "rwar" {
		t.Errorf("Say: got %v", n.Say)
	}
	if n.GraphicID != 200 || n.GraphicHeight != 92 || n.GraphicDelay != 0 {
		t.Errorf("Graphic*: got (%d,%d,%d)", n.GraphicID, n.GraphicHeight, n.GraphicDelay)
	}
}

func TestComputeNpc_NilSlotIsNoop(t *testing.T) {
	b := New()
	// nid 50 not added.
	b.ComputeNpc(50, 100, 60, 0, 70, false, -1, -1, false, 0,
		-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, nil, -1, -1, -1)
	if b.npcs[50] != nil {
		t.Error("ComputeNpc on nil slot allocated npc")
	}
}

func TestComputeNpc_NegativeIDsAreNoop(t *testing.T) {
	b := New()
	b.AddNpc(50, 100)
	b.ComputeNpc(-1, 100, 60, 0, 70, false, -1, -1, false, 0,
		-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, nil, -1, -1, -1)
	b.ComputeNpc(50, -1, 60, 0, 70, false, -1, -1, false, 0,
		-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, nil, -1, -1, -1)
	// Both should no-op. Pin that the negative-ntype call did NOT
	// overwrite NType=100 on the existing slot — without this assertion
	// the test would silently pass even if the ntype<0 guard were removed.
	if b.npcs[50] == nil {
		t.Fatal("slot 50 nilled")
	}
	if b.npcs[50].NType != 100 {
		t.Errorf("negative ntype: NType was overwritten, got %d, want 100", b.npcs[50].NType)
	}
}

func TestComputeNpc_CrossZoneMoveUpdatesZoneMap(t *testing.T) {
	b := New()
	b.AddNpc(50, 100)
	b.ComputeNpc(50, 100, 50, 0, 50, false, -1, -1, true, 0,
		-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, nil, -1, -1, -1)
	if _, ok := b.zoneMap.Zone(50, 0, 50).npcs[50]; !ok {
		t.Fatal("after first compute: zone (6,0,6) should contain nid 50")
	}

	b.ComputeNpc(50, 100, 64, 0, 50, false, -1, -1, true, 0,
		-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, nil, -1, -1, -1)
	if _, ok := b.zoneMap.Zone(50, 0, 50).npcs[50]; ok {
		t.Error("after cross-zone: old zone still contains nid 50")
	}
	if _, ok := b.zoneMap.Zone(64, 0, 50).npcs[50]; !ok {
		t.Error("after cross-zone: new zone missing nid 50")
	}
}

func TestCleanup_ClearsPlayerGridAndCallsEntityCleanup(t *testing.T) {
	b := New()
	b.AddPlayer(5)
	b.AddNpc(10, 100)
	// Compute populates state + playerGrid.
	b.ComputePlayer(5, 50, 0, 50, 48, 48, true, false, 1, 2,
		VisibilityDefault, true, 0xff, []byte{1}, -1, -1, -1, -1, -1, -1, -1,
		-1, -1, -1, 808, 0, nil, nil, 0, 0, 0, -1, -1, -1,
		-1, -1, -1, -1, -1, -1, -1)
	b.ComputeNpc(10, 100, 60, 0, 60, true, 1, 2, true, 0xff,
		-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, 808, nil, -1, -1, -1)
	if len(b.playerGrid) == 0 {
		t.Fatal("test setup: ComputePlayer did not populate playerGrid")
	}
	if !b.players[5].Tele || b.players[5].Masks != 0xff {
		t.Fatal("test setup: ComputePlayer didn't write fields")
	}
	if !b.npcs[10].Tele || b.npcs[10].Masks != 0xff {
		t.Fatal("test setup: ComputeNpc didn't write fields")
	}

	b.Cleanup()

	if len(b.playerGrid) != 0 {
		t.Errorf("Cleanup: playerGrid not cleared, len=%d", len(b.playerGrid))
	}
	if b.players[5].Tele {
		t.Error("Cleanup: player.Tele not reset")
	}
	if b.players[5].Masks != 0 {
		t.Errorf("Cleanup: player.Masks = %d, want 0", b.players[5].Masks)
	}
	if b.npcs[10].Tele {
		t.Error("Cleanup: npc.Tele not reset")
	}
	if b.npcs[10].Masks != 0 {
		t.Errorf("Cleanup: npc.Masks = %d, want 0", b.npcs[10].Masks)
	}
}

func TestCleanup_PreservesAppearanceAndOrientation(t *testing.T) {
	// Cleanup does NOT clear the persistent fields per upstream
	// player.rs/npc.rs commented-out cleanup lines.
	b := New()
	b.AddPlayer(5)
	b.AddNpc(10, 100)
	b.players[5].Appearance = []byte{1, 2, 3}
	b.players[5].LastAppearance = 100
	b.players[5].FaceEntity = 42
	b.players[5].OrientationX = 50
	b.npcs[10].FaceEntity = 99
	b.npcs[10].OrientationX = 33
	b.npcs[10].Observers = 4

	b.Cleanup()

	if len(b.players[5].Appearance) != 3 {
		t.Error("Cleanup CLEARED player.Appearance")
	}
	if b.players[5].LastAppearance != 100 {
		t.Errorf("Cleanup CLEARED player.LastAppearance")
	}
	if b.players[5].FaceEntity != 42 || b.players[5].OrientationX != 50 {
		t.Error("Cleanup CLEARED player FaceEntity / OrientationX")
	}
	if b.npcs[10].FaceEntity != 99 || b.npcs[10].OrientationX != 33 {
		t.Error("Cleanup CLEARED npc FaceEntity / OrientationX")
	}
	if b.npcs[10].Observers != 4 {
		t.Errorf("Cleanup CLEARED npc.Observers: got %d, want 4", b.npcs[10].Observers)
	}
}

func TestCleanup_NilSlotsAreSkipped(t *testing.T) {
	b := New()
	// No AddPlayer / AddNpc calls — all slots nil.
	b.ComputePlayer(5, 50, 0, 50, 48, 48, false, false, -1, -1,
		VisibilityDefault, false, 0, nil, -1, -1, -1, -1, -1, -1, -1,
		-1, -1, -1, -1, -1, nil, nil, 0, 0, 0, -1, -1, -1,
		-1, -1, -1, -1, -1, -1, -1)
	// playerGrid push from ComputePlayer was a no-op (nil slot guard).
	b.Cleanup() // must not panic on nil-slot iteration
	// Falsifiable assertion: Cleanup must not fabricate slots.
	if b.players[5] != nil {
		t.Error("nil-slot guard: slot 5 should still be nil after Cleanup")
	}
	if len(b.playerGrid) != 0 {
		t.Errorf("Cleanup: playerGrid not empty after nil-slot run, len=%d", len(b.playerGrid))
	}
}

func TestCleanupPlayerBuildArea_ClearsTrackingAndAppearances(t *testing.T) {
	b := New()
	b.AddPlayer(5)
	b.players[5].Build.Players.Insert(10)
	b.players[5].Build.Npcs.Insert(20)
	b.players[5].Build.SaveAppearance(7, 100)

	b.CleanupPlayerBuildArea(5)

	if b.players[5].Build.Players.Len() != 0 {
		t.Error("CleanupPlayerBuildArea: Players set not cleared")
	}
	if b.players[5].Build.Npcs.Len() != 0 {
		t.Error("CleanupPlayerBuildArea: Npcs set not cleared")
	}
	if b.players[5].Build.HasAppearance(7, 100) {
		t.Error("CleanupPlayerBuildArea: appearances not cleared")
	}
}

func TestCleanupPlayerBuildArea_NilSlotIsNoop(t *testing.T) {
	b := New()
	b.CleanupPlayerBuildArea(5) // never added
	b.CleanupPlayerBuildArea(-1)
	b.CleanupPlayerBuildArea(2048)
	// Falsifiable assertion: must not fabricate a slot via a side-effect.
	if b.players[5] != nil {
		t.Error("CleanupPlayerBuildArea(absent): unexpected slot allocation at pid=5")
	}
}

func TestHasPlayer_ChecksBuildArea(t *testing.T) {
	b := New()
	b.AddPlayer(5)
	b.players[5].Build.Players.Insert(10)
	if !b.HasPlayer(5, 10) {
		t.Error("HasPlayer(5, 10) false after Build.Players.Insert(10)")
	}
	if b.HasPlayer(5, 11) {
		t.Error("HasPlayer(5, 11) true (never inserted)")
	}
}

func TestHasPlayer_NegativeArgsAreFalse(t *testing.T) {
	b := New()
	b.AddPlayer(5)
	if b.HasPlayer(-1, 10) {
		t.Error("HasPlayer(-1, 10) returned true")
	}
	if b.HasPlayer(5, -1) {
		t.Error("HasPlayer(5, -1) returned true")
	}
}

func TestHasPlayer_NilSlotIsFalse(t *testing.T) {
	b := New()
	if b.HasPlayer(5, 10) { // pid 5 not added
		t.Error("HasPlayer on nil slot returned true")
	}
}

func TestHasNpc_ChecksBuildArea(t *testing.T) {
	b := New()
	b.AddPlayer(5)
	b.players[5].Build.Npcs.Insert(20)
	if !b.HasNpc(5, 20) {
		t.Error("HasNpc(5, 20) false after Build.Npcs.Insert(20)")
	}
	if b.HasNpc(5, 21) {
		t.Error("HasNpc(5, 21) true (never inserted)")
	}
}

func TestHasNpc_NegativeArgsAreFalse(t *testing.T) {
	b := New()
	b.AddPlayer(5)
	if b.HasNpc(-1, 20) {
		t.Error("HasNpc(-1, 20) returned true")
	}
	if b.HasNpc(5, -1) {
		t.Error("HasNpc(5, -1) returned true")
	}
}

func TestHasNpc_NilSlotIsFalse(t *testing.T) {
	b := New()
	if b.HasNpc(5, 20) { // pid 5 not added
		t.Error("HasNpc on nil slot returned true")
	}
}

func TestGetNpcObservers_ReadsCounter(t *testing.T) {
	b := New()
	b.AddNpc(50, 100)
	b.npcs[50].Observers = 7
	if b.GetNpcObservers(50) != 7 {
		t.Errorf("GetNpcObservers(50): got %d, want 7", b.GetNpcObservers(50))
	}
}

func TestGetNpcObservers_NilSlotIsZero(t *testing.T) {
	b := New()
	if b.GetNpcObservers(50) != 0 {
		t.Errorf("GetNpcObservers on nil slot: got %d, want 0", b.GetNpcObservers(50))
	}
	if got := b.GetNpcObservers(-1); got != 0 {
		t.Errorf("GetNpcObservers(-1): got %d, want 0", got)
	}
	if got := b.GetNpcObservers(8192); got != 0 {
		t.Errorf("GetNpcObservers(8192): got %d, want 0", got)
	}
}
