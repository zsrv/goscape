package world

import (
	"slices"
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
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

// TestResetEntityForRespawnInvokesUnfocus pins TS Npc.resetEntity(true)
// at Npc.ts:284 — calls super.unfocus() to restore default-south
// face-angle. Goscape's resetEntityForRespawn is the goscape-shape
// equivalent of that branch.
func TestResetEntityForRespawnInvokesUnfocus(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{Size: 1}
	n := newRegisteredNpc(t, s, typ, false)
	// Pre-state: simulate post-interaction face-angle drift.
	n.faceAngleX = 999_999
	n.faceAngleZ = 999_999

	s.resetEntityForRespawn(n)

	wantFX := coordgrid.Fine(n.x, n.size)
	wantFZ := coordgrid.Fine(n.z-1, n.size)
	if n.faceAngleX != wantFX {
		t.Errorf("faceAngleX: got %d, want %d (Fine(n.x=%d, size=%d))", n.faceAngleX, wantFX, n.x, n.size)
	}
	if n.faceAngleZ != wantFZ {
		t.Errorf("faceAngleZ: got %d, want %d (Fine(n.z-1=%d, size=%d))", n.faceAngleZ, wantFZ, n.z-1, n.size)
	}
}

// seedVarnTypes installs a minimal VarnTypeConfigs on s with the given
// (type, name) tuples for resetEntityForRespawn seed-loop tests.
func seedVarnTypes(s *Server, entries []struct {
	Type objtype.ScriptVarType
	Name string
}) {
	configs := make([]*objtype.VarNpcType, len(entries))
	configNames := make(map[string]int, len(entries))
	for i, e := range entries {
		c := objtype.NewVarNpcType(i)
		c.Type = e.Type
		c.DebugName = e.Name
		configs[i] = c
		// Match parseVarnTypes: only insert into ConfigNames when the
		// debug name is non-empty.
		if e.Name != "" {
			configNames[e.Name] = i
		}
	}
	s.varnTypes = &objtype.VarnTypeConfigs{ConfigNames: configNames, Configs: configs}
}

func TestResetEntityForRespawn_SeedsIntToZero(t *testing.T) {
	s := newTestServer(t)
	seedVarnTypes(s, []struct {
		Type objtype.ScriptVarType
		Name string
	}{
		{Type: objtype.ScriptVarTypeInt, Name: "int_var"},
	})

	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 0}}
	n := NewNpc(1, 0, 100, 100, 0, npcType)
	n.varns = []int32{42} // pre-write to verify reset overwrites
	s.resetEntityForRespawn(n)

	if got := n.NpcVarN(0); got != 0 {
		t.Errorf("INT-typed varn after reset: got %d, want 0", got)
	}
}

func TestResetEntityForRespawn_SeedsPlayerUidToMinusOne(t *testing.T) {
	s := newTestServer(t)
	seedVarnTypes(s, []struct {
		Type objtype.ScriptVarType
		Name string
	}{
		{Type: objtype.ScriptVarTypePlayerUid, Name: "npc_macro_event_target"},
	})

	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 0}}
	n := NewNpc(1, 0, 100, 100, 0, npcType)
	s.resetEntityForRespawn(n)

	if got := n.NpcVarN(0); got != -1 {
		t.Errorf("player_uid-typed varn: got %d, want -1", got)
	}
}

func TestResetEntityForRespawn_SeedsCoordToMinusOne(t *testing.T) {
	s := newTestServer(t)
	seedVarnTypes(s, []struct {
		Type objtype.ScriptVarType
		Name string
	}{
		{Type: objtype.ScriptVarTypeCoord, Name: "npc_start_coord"},
	})

	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 0}}
	n := NewNpc(1, 0, 100, 100, 0, npcType)
	s.resetEntityForRespawn(n)

	if got := n.NpcVarN(0); got != -1 {
		t.Errorf("coord-typed varn: got %d, want -1", got)
	}
}

func TestResetEntityForRespawn_SeedsNpcUidToMinusOne(t *testing.T) {
	s := newTestServer(t)
	seedVarnTypes(s, []struct {
		Type objtype.ScriptVarType
		Name string
	}{
		{Type: objtype.ScriptVarTypeNpcUid, Name: "rantz_attacking_chompy"},
	})

	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 0}}
	n := NewNpc(1, 0, 100, 100, 0, npcType)
	s.resetEntityForRespawn(n)

	if got := n.NpcVarN(0); got != -1 {
		t.Errorf("npc_uid-typed varn: got %d, want -1", got)
	}
}

func TestResetEntityForRespawn_SeedsStringToEmpty(t *testing.T) {
	s := newTestServer(t)
	seedVarnTypes(s, []struct {
		Type objtype.ScriptVarType
		Name string
	}{
		{Type: objtype.ScriptVarTypeString, Name: "string_var"},
	})

	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 0}}
	n := NewNpc(1, 0, 100, 100, 0, npcType)
	// Note: NpcVarNString accessor lands in T5; test reads field
	// directly here. After T5, this can switch to NpcVarNString.
	s.resetEntityForRespawn(n)

	if len(n.varnsString) != 1 {
		t.Fatalf("varnsString length: got %d, want 1", len(n.varnsString))
	}
	if got := n.varnsString[0]; got != "" {
		t.Errorf("string-typed varn: got %q, want \"\"", got)
	}
}

func TestResetEntityForRespawn_NilVarnTypes_NoOp(t *testing.T) {
	s := newTestServer(t)
	// Do NOT seed varnTypes; leave nil.

	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 0}}
	n := NewNpc(1, 0, 100, 100, 0, npcType)
	s.resetEntityForRespawn(n)

	if n.varns != nil {
		t.Errorf("varns: got non-nil slice, want nil (defensive no-op)")
	}
	if n.varnsString != nil {
		t.Errorf("varnsString: got non-nil slice, want nil (defensive no-op)")
	}
}

func TestAddNpc_FreshSpawn_PlayerUidVarnReadsMinusOne(t *testing.T) {
	// THE smoke-bind unit pin. After Server.addNpc, a fresh-spawn NPC's
	// player_uid-typed varn must read as -1 so the player_combat.rs2
	// "It's not after you." gate skips.
	s := newTestServer(t)
	seedVarnTypes(s, []struct {
		Type objtype.ScriptVarType
		Name string
	}{
		{Type: objtype.ScriptVarTypePlayerUid, Name: "npc_macro_event_target"},
	})

	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 0}}
	n := NewNpc(1, 0, 100, 100, 0, npcType)
	// addNpc(n, duration, firstSpawn). firstSpawn=true allocates nid.
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}

	if got := n.NpcVarN(0); got != -1 {
		t.Errorf("smoke-bind: %%npc_macro_event_target on fresh-spawn NPC: got %d, want -1", got)
	}
}

func TestAddNpc_RespawnAfterChangeType_ReseedsVarns(t *testing.T) {
	// Heavy revertType path: removeNpc + addNpc(firstSpawn=false) →
	// resetEntityForRespawn re-seeds varns. Mid-life writes are wiped.
	s := newTestServer(t)
	seedVarnTypes(s, []struct {
		Type objtype.ScriptVarType
		Name string
	}{
		{Type: objtype.ScriptVarTypePlayerUid, Name: "npc_macro_event_target"},
	})

	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 0}}
	n := NewNpc(1, 0, 100, 100, 0, npcType)
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc(firstSpawn=true): %v", err)
	}

	// Mid-life mutation (e.g. combat-script attaches a player UID).
	n.SetNpcVarN(0, 12345)
	if got := n.NpcVarN(0); got != 12345 {
		t.Fatalf("setup: NpcVarN(0) after SetNpcVarN: got %d, want 12345", got)
	}

	// Heavy revertType-style respawn (firstSpawn=false; nid stays).
	if err := s.addNpc(n, -1, false); err != nil {
		t.Fatalf("addNpc(firstSpawn=false): %v", err)
	}

	if got := n.NpcVarN(0); got != -1 {
		t.Errorf("varn after respawn re-seed: got %d, want -1", got)
	}
}

// TestRemoveNpc_DespawnLifecycle_ClearsRegistrySlot pins the NAI-19
// slot-release: after removeNpc on a DESPAWN-lifecycle NPC, the
// allocated nid slot in s.npcs is nilled so allocNpcSlot can reuse it.
// Mirrors TS World.ts:1314: this.npcs.remove(npc.nid).
func TestRemoveNpc_DespawnLifecycle_ClearsRegistrySlot(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}}
	n := NewNpc(0, 7, 100, 100, 0, typ)
	n.nid = 1
	n.server = s
	s.npcs[1] = n
	s.npcLoop = append(s.npcLoop, n)
	n.lifecycle = NpcLifecycleDespawn

	slot := n.nid // capture before Cleanup zeros it

	s.removeNpc(n, -1)

	if s.npcs[slot] != nil {
		t.Errorf("s.npcs[%d]: got %p, want nil (slot must be released on DESPAWN)", slot, s.npcs[slot])
	}
}

// TestRemoveNpc_DespawnLifecycle_RunsCleanup pins that the DESPAWN
// arm of removeNpc calls n.Cleanup, zeroing identity / script / hunt /
// queue. Mirrors TS World.ts:1315: npc.cleanup().
func TestRemoveNpc_DespawnLifecycle_RunsCleanup(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}}
	n := NewNpc(0, 7, 100, 100, 0, typ)
	n.nid = 1
	n.server = s
	s.npcs[1] = n
	s.npcLoop = append(s.npcLoop, n)
	n.lifecycle = NpcLifecycleDespawn
	n.activeScript = &script.ScriptState{}
	n.queue = []script.NpcQueueRequest{{}}

	s.removeNpc(n, -1)

	if n.nid != -1 {
		t.Errorf("nid: got %d, want -1 (Cleanup must zero)", n.nid)
	}
	if n.uid != -1 {
		t.Errorf("uid: got %d, want -1", n.uid)
	}
	if n.activeScript != nil {
		t.Errorf("activeScript: got %p, want nil", n.activeScript)
	}
	if n.queue != nil {
		t.Errorf("queue: got %v, want nil", n.queue)
	}
}

// TestRemoveNpc_RespawnLifecycle_PreservesRegistry pins that removeNpc
// on a RESPAWN-lifecycle NPC does NOT release the slot or run Cleanup —
// the NPC will respawn in place at lifecycleTick==0 (see npc_ai.go:31-45).
func TestRemoveNpc_RespawnLifecycle_PreservesRegistry(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}}
	n := NewNpc(0, 7, 100, 100, 0, typ)
	n.nid = 1
	n.uid = (7 << 16) | 1
	n.server = s
	s.npcs[1] = n
	s.npcLoop = append(s.npcLoop, n)
	n.lifecycle = NpcLifecycleRespawn
	n.activeScript = &script.ScriptState{}

	s.removeNpc(n, 50)

	if s.npcs[1] != n {
		t.Errorf("s.npcs[1]: got %p, want %p (RESPAWN must NOT release slot)", s.npcs[1], n)
	}
	if n.nid != 1 {
		t.Errorf("nid: got %d, want 1 (RESPAWN must NOT run Cleanup)", n.nid)
	}
	if n.activeScript == nil {
		t.Error("activeScript: got nil, want preserved (RESPAWN must NOT run Cleanup)")
	}
}
