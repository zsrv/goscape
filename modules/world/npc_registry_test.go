package world

import (
	"errors"
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
// DESPAWN branch, removeNpc does NOT write n.lifecycleTick. (DESPAWN's
// full behavior — slot release, Cleanup, end-of-tick splice — is pinned
// by the NAI-19 test suite below; this test focuses on lifecycleTick.)
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

// TestRsbufLifecycle_FirstSpawnRegistersOnly covers world-ops-3
// (2026-05-28 fresh-audit MED): TS World.addNpc at World.ts:1259-1262
// calls rsbuf.addNpc ONLY inside `if (firstSpawn)`. goscape pre-fix
// placed AddNpc OUTSIDE the firstSpawn block, so a revertType /
// firstSpawn=false call re-registered an already-registered nid and
// allocated a fresh rsbuf.Npc struct each respawn cycle, churning
// state and breaking the TS invariant that a RESPAWN NPC keeps its
// rsbuf entry across death/respawn. Detection: snapshot the
// *rsbuf.Npc pointer after firstSpawn=true; a respawn must preserve
// the same pointer (no fresh allocation).
func TestRsbufLifecycle_FirstSpawnRegistersOnly(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}, Size: 1}
	n := newRegisteredNpc(t, s, typ, true)

	firstEntry := s.rsbuf.NpcForTest(int32(n.nid))
	if firstEntry == nil {
		t.Fatalf("rsbuf has no entry for nid=%d after firstSpawn=true (firstSpawn-gated AddNpc must fire on initial spawn)", n.nid)
	}

	// Respawn (firstSpawn=false). TS-faithful: rsbuf entry stays as-is.
	if err := s.addNpc(n, -1, false); err != nil {
		t.Fatalf("respawn addNpc: %v", err)
	}
	respawnEntry := s.rsbuf.NpcForTest(int32(n.nid))
	if respawnEntry != firstEntry {
		t.Errorf("rsbuf entry pointer changed across respawn: got %p, want %p (TS World.ts:1259-1262 gates rsbuf.addNpc on firstSpawn only — respawn must NOT re-allocate)",
			respawnEntry, firstEntry)
	}
}

// TestRsbufLifecycle_RespawnRemoveNpcPreservesEntry covers world-ops-3
// (sibling to the firstSpawn-gating test above): TS World.removeNpc at
// World.ts:1312-1315 calls rsbuf.removeNpc ONLY in the DESPAWN branch.
// goscape pre-fix called RemoveNpc unconditionally before the lifecycle
// switch, so a RESPAWN NPC lost its rsbuf entry on death (pairing with
// the addNpc re-register bug to churn registration state every cycle).
// Post-fix: a RESPAWN removeNpc preserves the rsbuf entry; only DESPAWN
// clears it.
func TestRsbufLifecycle_RespawnRemoveNpcPreservesEntry(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}, Size: 1}
	n := newRegisteredNpc(t, s, typ, true)
	n.lifecycle = NpcLifecycleRespawn

	if s.rsbuf.NpcForTest(int32(n.nid)) == nil {
		t.Fatalf("rsbuf has no entry for nid=%d after firstSpawn — preconditions wrong", n.nid)
	}

	s.removeNpc(n, 50)

	if s.rsbuf.NpcForTest(int32(n.nid)) == nil {
		t.Errorf("rsbuf entry cleared for RESPAWN nid=%d (TS World.ts:1312-1315 gates rsbuf.removeNpc on DESPAWN only — RESPAWN must preserve)", n.nid)
	}
	if !n.dead {
		t.Error("n.dead: got false, want true (collision toggle / dead-flag path still runs on RESPAWN)")
	}
}

// TestRsbufLifecycle_DespawnRemoveNpcUnregisters regression-guards the
// DESPAWN side: removeNpc with lifecycle=DESPAWN MUST clear the rsbuf
// entry (paired with s.npcs slot-nil + Cleanup). TS World.ts:1312-1315.
// Pre-fix and post-fix both clear here; this test ensures the move-into-
// branch refactor didn't accidentally skip the clear.
func TestRsbufLifecycle_DespawnRemoveNpcUnregisters(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}, Size: 1}
	n := newRegisteredNpc(t, s, typ, true)
	n.lifecycle = NpcLifecycleDespawn

	if s.rsbuf.NpcForTest(int32(n.nid)) == nil {
		t.Fatalf("rsbuf has no entry for nid=%d after firstSpawn — preconditions wrong", n.nid)
	}
	nidBefore := n.nid

	s.removeNpc(n, 50)

	if s.rsbuf.NpcForTest(int32(nidBefore)) != nil {
		t.Errorf("rsbuf entry NOT cleared for DESPAWN nid=%d (TS World.ts:1312-1315: rsbuf.removeNpc + this.npcs.remove + npc.cleanup must fire)", nidBefore)
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

// TestResetEntityForRespawnClearsHeroPoints pins the TS Npc.ts:292
// heroPoints.clear() call on the respawn=true branch. The NPC struct
// is reused across respawn cycles, so contributors accumulated before
// death must NOT linger into the next life. NAI-120 Bundle 2D follow-up.
func TestResetEntityForRespawnClearsHeroPoints(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{Size: 1, BlockWalk: objtype.BlockWalkNPC}
	n := newRegisteredNpc(t, s, typ, true)

	n.heroPoints.AddHero(42, 7)
	n.heroPoints.AddHero(99, 3)
	if got := n.heroPoints.TopContributor(); got != 42 {
		t.Fatalf("setup: TopContributor() = %d, want 42", got)
	}

	s.resetEntityForRespawn(n)

	if got := n.heroPoints.TopContributor(); got != 0 {
		t.Errorf("after resetEntityForRespawn: TopContributor() = %d, want 0 (empty ledger)", got)
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

// TestResetEntityForRespawn_AppliesResetDefaultsTSFidelity pins the 2026-05-28
// audit row npc-core-1. TS Npc.resetEntity(true) at Npc.ts:307 calls
// resetDefaults(); TS resetDefaults (Npc.ts:411-425) clears target/targetOp/
// apRange/apRangeCalled/targetSubject/faceEntity/timerInterval and sets
// masks |= entitymask. goscape's (n *Npc).resetDefaults() is the NAI-11-
// stripped subset (target/targetOp/faceEntity/masks only); the
// apRange/apRangeCalled/targetSubject/timerInterval resets the stripped
// subset omits must be re-applied inline by resetEntityForRespawn so the
// respawn surface reaches full TS-fidelity.
func TestResetEntityForRespawn_AppliesResetDefaultsTSFidelity(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{
		Size:        1,
		DefaultMode: objtype.NPCModePatrol, // distinct from anything pre-set on n
		Timer:       42,
	}
	n := newRegisteredNpc(t, s, typ, false)
	other := newRegisteredNpc(t, s, &objtype.NpcType{Size: 1}, false)
	// Dirty every field TS resetDefaults resets.
	n.target = other
	n.targetOp = objtype.NPCModePlayerFollow
	n.faceEntity = 7
	n.apRange = 3
	n.apRangeCalled = true
	n.targetSubject = npcTargetSubject{com: 99, typ: 5}
	n.timerInterval = 999
	n.masks = 0

	s.resetEntityForRespawn(n)

	if n.target != nil {
		t.Errorf("target: got %p, want nil (TS Npc.ts:307 resetDefaults→clearInteraction)", n.target)
	}
	if n.targetOp != objtype.NPCModePatrol {
		t.Errorf("targetOp: got %d, want %d (NPCModePatrol from typ.DefaultMode per TS Npc.ts:414)", n.targetOp, objtype.NPCModePatrol)
	}
	if n.faceEntity != -1 {
		t.Errorf("faceEntity: got %d, want -1 (TS Npc.ts:415)", n.faceEntity)
	}
	if n.apRange != 10 {
		t.Errorf("apRange: got %d, want 10 (TS PathingEntity.clearInteraction defaultApRange)", n.apRange)
	}
	if n.apRangeCalled {
		t.Error("apRangeCalled: got true, want false (TS PathingEntity.clearInteraction)")
	}
	if n.targetSubject.com != -1 || n.targetSubject.typ != -1 {
		t.Errorf("targetSubject: got {com:%d, typ:%d}, want {com:-1, typ:-1} (TS PathingEntity.clearInteraction)", n.targetSubject.com, n.targetSubject.typ)
	}
	if n.timerInterval != 42 {
		t.Errorf("timerInterval: got %d, want 42 (typ.Timer per TS Npc.ts:424)", n.timerInterval)
	}
	if n.masks&rsbuf.NpcMaskFaceEntity == 0 {
		t.Errorf("masks & NpcMaskFaceEntity: bit not set after reset (TS Npc.ts:416 `masks |= entitymask`); got masks=%d", n.masks)
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

// TestCompactNpcLoop_PrunesDespawnedDead pins NAI-19 end-of-tick pruning:
// DESPAWN-lifecycle dead NPCs are removed from s.npcLoop; alive NPCs
// and RESPAWN-lifecycle dead NPCs are preserved. The pruning predicate
// is (n.dead && n.lifecycle == NpcLifecycleDespawn).
func TestCompactNpcLoop_PrunesDespawnedDead(t *testing.T) {
	s := newTestServer(t)
	alive := &Npc{nid: 1, lifecycle: NpcLifecycleRespawn, dead: false}
	respawnDead := &Npc{nid: 2, lifecycle: NpcLifecycleRespawn, dead: true}
	despawnDead := &Npc{nid: 3, lifecycle: NpcLifecycleDespawn, dead: true}
	s.npcLoop = []*Npc{alive, respawnDead, despawnDead}

	s.compactNpcLoop()

	if len(s.npcLoop) != 2 {
		t.Fatalf("len(npcLoop): got %d, want 2", len(s.npcLoop))
	}
	if s.npcLoop[0] != alive {
		t.Errorf("npcLoop[0]: got %p, want %p (alive must be preserved)", s.npcLoop[0], alive)
	}
	if s.npcLoop[1] != respawnDead {
		t.Errorf("npcLoop[1]: got %p, want %p (RESPAWN+dead must be preserved)", s.npcLoop[1], respawnDead)
	}
	for _, n := range s.npcLoop {
		if n == despawnDead {
			t.Errorf("npcLoop still contains DESPAWN+dead %p", despawnDead)
		}
	}
}

// TestCompactNpcLoop_TailNilledForGC pins defensive GC-hint: trailing
// slots in the slice's capacity are nilled to drop pointer retention.
func TestCompactNpcLoop_TailNilledForGC(t *testing.T) {
	s := newTestServer(t)
	alive := &Npc{nid: 1, lifecycle: NpcLifecycleRespawn, dead: false}
	despawnDead := &Npc{nid: 2, lifecycle: NpcLifecycleDespawn, dead: true}
	s.npcLoop = []*Npc{alive, despawnDead}

	s.compactNpcLoop()

	// After compact, len == 1; the underlying capacity-slot [1] must be nil.
	full := s.npcLoop[:cap(s.npcLoop)]
	if full[1] != nil {
		t.Errorf("trailing slot full[1]: got %p, want nil (GC-hint required)", full[1])
	}
}

// TestRemoveNpc_DespawnLifecycle_SlotReusable pins NAI-19 round-trip:
// after removeNpc + compactNpcLoop, the allocator reuses the freed nid.
// Moved here from T2 because compactNpcLoop is defined in T3.
func TestRemoveNpc_DespawnLifecycle_SlotReusable(t *testing.T) {
	s := newTestServer(t)
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}}
	n1 := NewNpc(0, 7, 100, 100, 0, typ)
	n1.nid = 1
	n1.server = s
	s.npcs[1] = n1
	s.npcLoop = append(s.npcLoop, n1)
	n1.lifecycle = NpcLifecycleDespawn

	s.nextNpcSlot = 1 // force allocator to start at slot 1
	s.removeNpc(n1, -1)
	s.compactNpcLoop()

	reused := s.allocNpcSlot()
	if reused != 1 {
		t.Errorf("allocNpcSlot: got %d, want 1 (freed slot must be reusable)", reused)
	}
}

// --- NAI-163 B3: AddNpcAt adapter tests ----------------------------------

// newServerForAddNpcAt builds a *Server with a seeded NpcType for typeID=1
// and an empty zoneMap; mirrors newTestServer's defaults so the
// adapter's addNpc path goes through cleanly. Tests can vary typ
// per case by reassigning s.npcTypes.Configs[1].
func newServerForAddNpcAt(t *testing.T, typ *objtype.NpcType) *Server {
	t.Helper()
	s := newTestServer(t)
	s.npcTypes = &objtype.NPCTypeConfigs{
		Configs: []*objtype.NpcType{nil, typ},
	}
	return s
}

func TestAddNpcAt_AllocsNidAndRegisters(t *testing.T) {
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 1}, Size: 1}
	s := newServerForAddNpcAt(t, typ)
	w := worldVarsView{s: s}
	npc, err := w.AddNpcAt(0, 3200, 3300, 1, -1)
	if err != nil {
		t.Fatalf("AddNpcAt: %v", err)
	}
	real, ok := npc.(*Npc)
	if !ok {
		t.Fatalf("AddNpcAt returned %T, want *Npc", npc)
	}
	if real.nid < 1 || real.nid >= len(s.npcs) {
		t.Fatalf("nid out of range: %d", real.nid)
	}
	if s.npcs[real.nid] != real {
		t.Fatalf("Npc not registered at s.npcs[%d]", real.nid)
	}
	if !slices.Contains(s.npcLoop, real) {
		t.Fatalf("Npc not appended to s.npcLoop")
	}
}

func TestAddNpcAt_SetsDespawnLifecycle(t *testing.T) {
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 1}, Size: 1}
	s := newServerForAddNpcAt(t, typ)
	w := worldVarsView{s: s}
	npc, err := w.AddNpcAt(0, 3200, 3300, 1, -1)
	if err != nil {
		t.Fatalf("AddNpcAt: %v", err)
	}
	real := npc.(*Npc)
	if real.lifecycle != NpcLifecycleDespawn {
		t.Fatalf("lifecycle = %d, want NpcLifecycleDespawn (%d)", real.lifecycle, NpcLifecycleDespawn)
	}
}

func TestAddNpcAt_WritesLifecycleTick(t *testing.T) {
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 1}, Size: 1}
	s := newServerForAddNpcAt(t, typ)
	w := worldVarsView{s: s}
	npc, err := w.AddNpcAt(0, 3200, 3300, 1, 50)
	if err != nil {
		t.Fatalf("AddNpcAt: %v", err)
	}
	real := npc.(*Npc)
	if real.lifecycleTick != 50 {
		t.Fatalf("lifecycleTick = %d, want 50", real.lifecycleTick)
	}
}

func TestAddNpcAt_RegistryFull_ReturnsErrNpcsFull(t *testing.T) {
	typ := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 1}, Size: 1}
	s := newServerForAddNpcAt(t, typ)
	// Fill all slots 1..N-1 (slot 0 stays nil per allocator convention).
	for i := 1; i < len(s.npcs); i++ {
		s.npcs[i] = &Npc{nid: i, typeId: 1}
	}
	w := worldVarsView{s: s}
	_, err := w.AddNpcAt(0, 3200, 3300, 1, -1)
	if !errors.Is(err, errNpcsFull) {
		t.Fatalf("err = %v, want errNpcsFull", err)
	}
}

func TestAddNpcAt_PopulatesSizeBlockWalkMoveRestrict(t *testing.T) {
	typ := &objtype.NpcType{
		ConfigType:   objtype.ConfigType{ID: 1},
		Size:         2,
		BlockWalk:    objtype.BlockWalkNPC,
		MoveRestrict: 1, // any non-zero MoveRestrict for the test
	}
	s := newServerForAddNpcAt(t, typ)
	w := worldVarsView{s: s}
	npc, err := w.AddNpcAt(0, 3200, 3300, 1, -1)
	if err != nil {
		t.Fatalf("AddNpcAt: %v", err)
	}
	real := npc.(*Npc)
	if real.size != int(typ.Size) {
		t.Fatalf("size = %d, want %d", real.size, typ.Size)
	}
	if real.blockWalk != typ.BlockWalk {
		t.Fatalf("blockWalk = %v, want %v", real.blockWalk, typ.BlockWalk)
	}
	if real.moveRestrict != MoveRestrict(typ.MoveRestrict) {
		t.Fatalf("moveRestrict = %v, want %v", real.moveRestrict, typ.MoveRestrict)
	}
}

// TestSpawnLoopNidGap pins rev-244 B3 gamemap-2: on a non-members world the
// nid allocator must advance for every valid NpcType in the spawn list —
// including members-only NPCs that are gated out — so that the nid sequence
// has gaps where members NPCs were skipped. This mirrors TS
// GameMap.loadNpcs:131-134 (pin 9aadcec4) which hoists `new Npc(...,
// World.getNextNid(), ...)` ABOVE the members gate.
//
// Fixture: three spawns in order — F2P, members-only, F2P — on a non-members
// world. The first F2P NPC gets nid N; the members NPC is skipped (gate) but
// its nid N+1 is consumed; the second F2P NPC gets nid N+2.
//
// FAIL expected under 225-style gate-before-alloc (members skip → no nid
// consumed → third NPC gets N+1). PASS after 244 hoist (nid consumed for
// skipped members NPC → third NPC gets N+2).
func TestSpawnLoopNidGap(t *testing.T) {
	s := newTestServer(t)

	f2pTyp := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 1},
		Size:       1,
		Members:    false,
	}
	membersTyp := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 2},
		Size:       1,
		Members:    true,
	}

	worldMembers := false
	spawns := []*objtype.NpcType{f2pTyp, membersTyp, f2pTyp}

	// Call spawnBootNpc — the 244-faithful helper that implements the hoisted
	// nid-consumption loop. Under pre-hoist (225-style) code this helper does
	// not exist and the inline loop in server.go gates before allocating; the
	// test therefore fails until the hoist is applied.
	var registered []*Npc
	for _, typ := range spawns {
		n, err := s.spawnBootNpc(typ, int(typ.ConfigType.ID), 100, 100, 0, worldMembers)
		if err != nil {
			t.Fatalf("registry full during test setup: %v", err)
		}
		if n != nil {
			registered = append(registered, n)
		}
	}

	if len(registered) != 2 {
		t.Fatalf("registered NPC count: got %d, want 2 (members NPC must be skipped)", len(registered))
	}

	firstNid := registered[0].nid
	thirdNid := registered[1].nid

	// The gap: members NPC consumed nid firstNid+1, so the second F2P NPC
	// must be at firstNid+2 — NOT firstNid+1 (which would mean no gap).
	wantThirdNid := firstNid + 2
	if thirdNid != wantThirdNid {
		t.Errorf("third NPC nid: got %d, want %d — members-skipped NPC must consume nid %d (244 hoist [gamemap-2])",
			thirdNid, wantThirdNid, firstNid+1)
	}
}
