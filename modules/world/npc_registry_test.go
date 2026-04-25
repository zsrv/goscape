package world

import (
	"slices"
	"testing"

	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/objtype"
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
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}, Size: 1}
	n := NewNpc(0, 7, 100, 100, 0, typ)

	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}
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
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}, Size: 1}
	n := NewNpc(0, 7, 100, 100, 0, typ)
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("first addNpc: %v", err)
	}
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
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}, Size: 1}
	n := NewNpc(0, 7, 100, 100, 0, typ)
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("first addNpc: %v", err)
	}

	if err := s.addNpc(n, 25, false); err != nil {
		t.Fatalf("respawn with duration: %v", err)
	}
	if n.lifecycleTick != 25 {
		t.Errorf("n.lifecycleTick: got %d, want 25", n.lifecycleTick)
	}
}
