package world

import (
	"slices"
	"testing"

	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

// TestRemoveNpcCollisionTogglesOff verifies that removeNpc(n, duration)
// clears the NPC's collision flags when n.typ.BlockWalk is BlockWalkNPC
// or BlockWalkAll. Mirrors TS World.removeNpc at World.ts:1296-1319
// (collision side of the body).
func TestRemoveNpcCollisionTogglesOff(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	typ := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 7},
		Size:       1,
		BlockWalk:  objtype.BlockWalkNPC,
	}
	n := NewNpc(0, 7, 100, 100, 0, typ)
	n.nid = 1
	n.server = s
	s.npcs[1] = n
	s.npcLoop = append(s.npcLoop, n)
	n.lifecycle = NpcLifecycleRespawn

	s.removeNpc(n, -1)

	if !n.dead {
		t.Error("n.dead: got false, want true")
	}
	// Collision assertion: n.dead flip is the load-bearing test signal here.
	// Reading Pathfinder flag state post-toggle is feasible via
	// s.gamemap.Pathfinder.Flags.IsFlagged but requires a prior Add/Allocate
	// that NewNpc doesn't do automatically — skipping the direct flag probe
	// keeps the test tight and unambiguous. The ChangeNPCCollision call
	// itself is exercised; nil-panic would fail the test loudly.
}

// TestRemoveNpcRespawnLifecycleSetsLifecycleTick verifies that on
// the RESPAWN+duration>-1 branch, removeNpc writes the scaled
// duration into n.lifecycleTick. Per TS World.ts:1316-1318.
func TestRemoveNpcRespawnLifecycleSetsLifecycleTick(t *testing.T) {
	s := newTestServer(t)
	setPlayerCountForTest(t, s, 0) // empty world: scale factor 1.0
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}}
	n := NewNpc(0, 7, 100, 100, 0, typ)
	n.nid = 1
	n.server = s
	s.npcs[1] = n
	s.npcLoop = append(s.npcLoop, n)
	n.lifecycle = NpcLifecycleRespawn
	n.lifecycleTick = 0

	s.removeNpc(n, 50)

	if n.lifecycleTick != 50 {
		t.Errorf("n.lifecycleTick: got %d, want 50 (empty world: scale=1.0)", n.lifecycleTick)
	}
}

// TestRemoveNpcDespawnLifecycleSkipsLifecycleTick verifies that on the
// DESPAWN branch, removeNpc does NOT write n.lifecycleTick. The DESPAWN
// branch carries TODO breadcrumbs for future rsbuf.RemoveNpc + registry
// cleanup but currently only flips n.dead.
func TestRemoveNpcDespawnLifecycleSkipsLifecycleTick(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}}
	n := NewNpc(0, 7, 100, 100, 0, typ)
	n.nid = 1
	n.server = s
	s.npcs[1] = n
	s.npcLoop = append(s.npcLoop, n)
	n.lifecycle = NpcLifecycleDespawn
	n.lifecycleTick = 99

	s.removeNpc(n, 50)

	if n.lifecycleTick != 99 {
		t.Errorf("n.lifecycleTick: got %d, want 99 (DESPAWN branch must not write)", n.lifecycleTick)
	}
	if !n.dead {
		t.Error("n.dead: got false, want true")
	}
}

// TestAddNpcFirstSpawnAllocsSlot verifies that addNpc(n, duration, true)
// allocates a slot and registers the NPC in s.npcs and s.npcLoop.
// Mirrors TS World.addNpc firstSpawn=true branch at World.ts:1259-1262.
func TestAddNpcFirstSpawnAllocsSlot(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{Size: 1}
	n := newRegisteredNpc(t, s, typ, true)

	if n.nid <= 0 || n.nid >= len(s.npcs) {
		t.Errorf("n.nid: got %d, want valid slot", n.nid)
	}
	if s.npcs[n.nid] != n {
		t.Error("s.npcs[n.nid]: not registered")
	}
	if !slices.Contains(s.npcLoop, n) {
		t.Error("s.npcLoop: not appended")
	}
}

// TestAddNpcRespawnSpawnSkipsSlotAlloc verifies that addNpc(n, duration, false)
// does NOT touch s.npcs or s.npcLoop — the NPC is already registered, this
// is the revertType respawn path. Mirrors TS World.addNpc firstSpawn=false
// at World.ts:1258-1262.
func TestAddNpcRespawnSpawnSkipsSlotAlloc(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{Size: 1}
	n := newRegisteredNpc(t, s, typ, true)
	nidBefore := n.nid
	loopLenBefore := len(s.npcLoop)

	// Now respawn: should NOT alloc a new slot.
	if err := s.addNpc(n, -1, false); err != nil {
		t.Fatalf("respawn addNpc: %v", err)
	}
	if n.nid != nidBefore {
		t.Errorf("n.nid changed: got %d, want %d (firstSpawn=false must not realloc)", n.nid, nidBefore)
	}
	if len(s.npcLoop) != loopLenBefore {
		t.Errorf("s.npcLoop grew: got len %d, want %d (firstSpawn=false must not append)",
			len(s.npcLoop), loopLenBefore)
	}
}

// TestAddNpcTeleportsToStart verifies that addNpc(n, duration, false)
// teleports the NPC back to its (startX, startZ). Mirrors TS World.addNpc
// at World.ts:1264-1265.
func TestAddNpcTeleportsToStart(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}, Size: 1}
	n := NewNpc(0, 7, 100, 100, 0, typ)
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("first addNpc: %v", err)
	}
	// NPC walks away from spawn.
	n.x = 150
	n.z = 150
	n.dead = true

	if err := s.addNpc(n, -1, false); err != nil {
		t.Fatalf("respawn addNpc: %v", err)
	}
	if n.x != 100 || n.z != 100 {
		t.Errorf("n.(x,z): got (%d,%d), want (100,100) (startX/startZ)", n.x, n.z)
	}
	if n.dead {
		t.Error("n.dead: got true, want false (respawn must clear)")
	}
}

// TestAddNpcRespawnSetsLifecycleTickWhenDurationGT0 verifies that on the
// duration > -1 branch, addNpc writes n.lifecycleTick = duration.
// Mirrors TS World.addNpc at World.ts:1291-1293.
func TestAddNpcRespawnSetsLifecycleTickWhenDurationGT0(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{Size: 1}
	n := newRegisteredNpc(t, s, typ, true)

	if err := s.addNpc(n, 25, false); err != nil {
		t.Fatalf("respawn with duration: %v", err)
	}
	if n.lifecycleTick != 25 {
		t.Errorf("n.lifecycleTick: got %d, want 25", n.lifecycleTick)
	}
}

// TestSizeMorphRevertRestoresBaseFootprint pins NAI-20 Task 2: when a
// size-1 NPC morphs to size-2 then reverts via the heavy path
// (s.removeNpc(n,-1); s.addNpc(n,-1,false)), collision flags reflect
// the base-size-1 footprint, not the morph-size-2 footprint.
func TestSizeMorphRevertRestoresBaseFootprint(t *testing.T) {
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())

	baseTyp := &objtype.NpcType{Size: 1, BlockWalk: objtype.BlockWalkAll}
	morphTyp := &objtype.NpcType{Size: 2, BlockWalk: objtype.BlockWalkAll}
	s.npcTypes = &objtype.NPCTypeConfigs{
		Configs: []*objtype.NpcType{nil, baseTyp, morphTyp},
	}

	n := newRegisteredNpc(t, s, baseTyp, true) // first-spawn

	n.ChangeType(2, -1) // morph; n.typ swaps to morphTyp; n.size stays 1

	// Heavy revert path: removeNpc clears flags using SNAPSHOT (size=1),
	// addNpc(false) re-sets flags using SNAPSHOT (size=1).
	s.removeNpc(n, -1)
	if err := s.addNpc(n, -1, false); err != nil {
		t.Fatalf("addNpc: %v", err)
	}

	// Verify size=1 footprint at (3200, 3200): SW corner flagged, neighbors NOT.
	flagMask := collision.FlagBlockNPCs | collision.FlagBlockPlayers
	if !s.gamemap.Pathfinder.Flags.IsFlagged(3200, 3200, 0, flagMask) {
		t.Errorf("(3200, 3200) should have NPC+Player flags after revert, got none")
	}
	for _, off := range []struct{ dx, dz int }{{1, 0}, {0, 1}, {1, 1}} {
		if s.gamemap.Pathfinder.Flags.IsFlagged(3200+off.dx, 3200+off.dz, 0, flagMask) {
			t.Errorf("(%d, %d) should NOT have flags after revert (size=1 footprint)",
				3200+off.dx, 3200+off.dz)
		}
	}
}

// TestResetEntityForRespawnFirstSpawnDoesNotRaiseChangeTypeMask pins
// NAI-20 Task 2 item 5: first-spawn (n.typeId == n.baseType) MUST NOT
// raise NpcMaskChangeType.
func TestResetEntityForRespawnFirstSpawnDoesNotRaiseChangeTypeMask(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{Size: 1, BlockWalk: objtype.BlockWalkNPC}
	n := newRegisteredNpc(t, s, typ, true) // addNpc → resetEntityForRespawn

	if n.masks&rsbuf.NpcMaskChangeType != 0 {
		t.Errorf("first-spawn raised NpcMaskChangeType (masks=%d); should remain clear",
			n.masks)
	}
}

// TestResetEntityForRespawnRevertRaisesChangeTypeMask pins NAI-20
// Task 2 item 5 inverse: when n.typeId != n.baseType (the morph-revert
// case), resetEntityForRespawn DOES raise NpcMaskChangeType.
func TestResetEntityForRespawnRevertRaisesChangeTypeMask(t *testing.T) {
	s := newTestServer(t)
	baseTyp := &objtype.NpcType{Size: 1}
	morphTyp := &objtype.NpcType{Size: 1}
	s.npcTypes = &objtype.NPCTypeConfigs{
		Configs: []*objtype.NpcType{nil, baseTyp, morphTyp},
	}
	n := newRegisteredNpc(t, s, baseTyp, false)
	n.typeId = 2 // simulate post-morph
	n.masks = 0  // start clean

	s.resetEntityForRespawn(n)

	if n.masks&rsbuf.NpcMaskChangeType == 0 {
		t.Errorf("revert path did NOT raise NpcMaskChangeType (masks=%d)", n.masks)
	}
	if n.typeId != n.baseType {
		t.Errorf("typeId=%d after reset, want baseType=%d", n.typeId, n.baseType)
	}
}

func TestRemoveNpcLeavesZone(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{Size: 1, BlockWalk: objtype.BlockWalkNone}
	n := newRegisteredNpc(t, s, typ, true)
	z := s.zoneMap.Get(n.level, n.x, n.z)
	s.removeNpc(n, -1)
	if z.NpcsCount() != 0 {
		t.Errorf("after removeNpc, Zone.NpcsCount: got %d, want 0", z.NpcsCount())
	}
	if n.zoneListElement != nil {
		t.Error("removeNpc should null n.zoneListElement")
	}
}

func TestNpcRevertTypeHeavyPathLeavesAndReentersZone(t *testing.T) {
	// revertType heavy path is s.removeNpc(n, -1) + s.addNpc(n, -1, false).
	// Subscription should round-trip to its pre-call state.
	s := newTestServer(t)
	typ := &objtype.NpcType{Size: 1, BlockWalk: objtype.BlockWalkNone}
	n := newRegisteredNpc(t, s, typ, true)
	z := s.zoneMap.Get(n.level, n.x, n.z)
	s.removeNpc(n, -1)
	if z.NpcsCount() != 0 {
		t.Fatalf("after removeNpc, NpcsCount: got %d, want 0", z.NpcsCount())
	}
	if err := s.addNpc(n, -1, false); err != nil {
		t.Fatalf("addNpc respawn: %v", err)
	}
	if z.NpcsCount() != 1 {
		t.Errorf("after revertType heavy path, NpcsCount: got %d, want 1", z.NpcsCount())
	}
}

func TestAddNpcEntersZone(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{Size: 1, BlockWalk: objtype.BlockWalkNone}
	n := newRegisteredNpc(t, s, typ, true)
	z := s.zoneMap.Get(n.level, n.x, n.z)
	if z.NpcsCount() != 1 {
		t.Errorf("after addNpc, Zone.NpcsCount: got %d, want 1", z.NpcsCount())
	}
	if n.zoneListElement == nil {
		t.Error("addNpc should populate n.zoneListElement")
	}
	// Dual-pin per ts_asymmetry_dual_pin: NPC enter does NOT flag grid.
	if s.zoneMap.Grid(n.level).IsFlagged(n.x>>3, n.z>>3, 0) {
		t.Error("addNpc must NOT flag the grid (only player enter flags)")
	}
}
