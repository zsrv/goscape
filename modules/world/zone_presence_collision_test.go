package world

// Collision-follow pin tests for the TS PathingEntity.refreshZonePresence
// port (Engine-TS@3c16994c PathingEntity.ts:163-188): an entity's collision
// footprint (FlagBlockNPCs, plus FlagBlockPlayers for blockWalk=ALL) must
// MOVE with the entity on every step and teleport — switch (this.blockWalk):
//
//	case NPC: changeNpcCollision(width, prev..., false) +
//	          changeNpcCollision(width, new..., true)        (TS :169-172)
//	case ALL: same PLUS the changePlayerCollision pair        (TS :173-178)
//
// Pre-fix goscape behavior: refreshPlayerZone/refreshNpcZone swapped zone
// membership only, so the flags seeded at spawn (addNpc, npc_registry.go)
// stayed frozen at the spawn tile forever (and a moving player never carried
// its NPC-blocking flag at all).

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
)

// newCollisionFollowServer builds a test server with a live gamemap so
// collision-flag probes via s.gamemap.Pathfinder.Flags are meaningful.
func newCollisionFollowServer(t *testing.T) *Server {
	t.Helper()
	s := newTestServer(t)
	s.gamemap = gamemap.New(discardLogger())
	return s
}

// blockingNpcType returns a synthetic 1x1 NpcType with the given blockWalk
// and a NORMAL moveRestrict (blockWalkFlag → FlagBlockNPCs, so the NPC's own
// pathing respects other NPC-blocking tiles — the Hans shape needs this).
func blockingNpcType(blockWalk int) *objtype.NpcType {
	return &objtype.NpcType{
		ConfigType:   objtype.ConfigType{ID: 1},
		MoveRestrict: int(MoveRestrictNormal),
		Size:         1,
		BlockWalk:    blockWalk,
	}
}

// --- 1. NPC step moves FlagBlockNPCs ---------------------------------------

func TestNpcStep_CollisionFlagFollows(t *testing.T) {
	s := newCollisionFollowServer(t)
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3200, 3200, 0)
	n := newRegisteredNpc(t, s, blockingNpcType(objtype.BlockWalkNPC), true) // spawns at (3200,3200,0)

	if !s.gamemap.Pathfinder.Flags.IsFlagged(3200, 3200, 0, collision.FlagBlockNPCs) {
		t.Fatalf("setup: spawn tile (3200,3200) missing FlagBlockNPCs after addNpc seed")
	}

	n.queueWaypoints([]int{coordgrid.PackCoord(0, 3201, 3200)})
	if dir := n.validateAndAdvanceStep(s); dir == -1 {
		t.Fatalf("setup: NPC step east did not advance")
	}
	if n.x != 3201 || n.z != 3200 {
		t.Fatalf("setup: NPC at (%d,%d), want (3201,3200)", n.x, n.z)
	}

	if s.gamemap.Pathfinder.Flags.IsFlagged(3200, 3200, 0, collision.FlagBlockNPCs) {
		t.Errorf("old tile (3200,3200) still carries FlagBlockNPCs after step; footprint must follow (TS PathingEntity.ts:170)")
	}
	if !s.gamemap.Pathfinder.Flags.IsFlagged(3201, 3200, 0, collision.FlagBlockNPCs) {
		t.Errorf("new tile (3201,3200) missing FlagBlockNPCs after step (TS PathingEntity.ts:171)")
	}
}

// --- 2. NPC Teleport moves the flag, using prevLevel for the removal -------

func TestNpcTeleport_CollisionFlagFollows_AcrossLevels(t *testing.T) {
	s := newCollisionFollowServer(t)
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3200, 3200, 0)
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3232, 3232, 2) // dest zone must be allocated (Teleport D2 gate)
	n := newRegisteredNpc(t, s, blockingNpcType(objtype.BlockWalkNPC), true)

	n.Teleport(3232, 3232, 2)
	if n.x != 3232 || n.z != 3232 || n.level != 2 {
		t.Fatalf("setup: NPC at (%d,%d,%d), want (3232,3232,2)", n.x, n.z, n.level)
	}

	if s.gamemap.Pathfinder.Flags.IsFlagged(3200, 3200, 0, collision.FlagBlockNPCs) {
		t.Errorf("source tile (3200,3200,0) still carries FlagBlockNPCs after teleport; removal must use prevX/prevZ/prevLevel (TS PathingEntity.ts:170,293)")
	}
	if !s.gamemap.Pathfinder.Flags.IsFlagged(3232, 3232, 2, collision.FlagBlockNPCs) {
		t.Errorf("dest tile (3232,3232,2) missing FlagBlockNPCs after teleport (TS PathingEntity.ts:171)")
	}
}

// --- 3. Player step moves FlagBlockNPCs (players block NPCs) ---------------

func TestPlayerStep_CollisionFlagFollows(t *testing.T) {
	s := newCollisionFollowServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3200, 3200, 0)
	// Emulate the carried state of a player who already moved at least once
	// (or SetVisibility(Default)): FlagBlockNPCs at the current tile. The
	// step must move it (Player constructs with BlockWalk.NPC, Player.ts:416).
	s.gamemap.ChangeNPCCollision(1, 3200, 3200, 0, true)

	p.queueWaypoint(3201, 3200)
	if dir := p.validateAndAdvanceStep(); dir == -1 {
		t.Fatal("setup: player step east did not move")
	}
	if p.x != 3201 || p.z != 3200 {
		t.Fatalf("setup: player at (%d,%d), want (3201,3200)", p.x, p.z)
	}

	if s.gamemap.Pathfinder.Flags.IsFlagged(3200, 3200, 0, collision.FlagBlockNPCs) {
		t.Errorf("old tile (3200,3200) still carries FlagBlockNPCs after player step (TS PathingEntity.ts:170)")
	}
	if !s.gamemap.Pathfinder.Flags.IsFlagged(3201, 3200, 0, collision.FlagBlockNPCs) {
		t.Errorf("new tile (3201,3200) missing FlagBlockNPCs after player step (TS PathingEntity.ts:171)")
	}
}

// --- 4. The Hans shape: NPC can WALK back onto its own spawn tile ----------

// TestNpcCanWalkBackOntoOwnSpawnTile: spawn an NPC at T, walk it off T, path
// it back onto T, and assert it arrives by walking. Pre-fix the FlagBlockNPCs
// seeded at T never moved, so the NPC's own frozen flag blocked the final
// step home (wandering NPCs like Hans could only return via the stuck-tele).
func TestNpcCanWalkBackOntoOwnSpawnTile(t *testing.T) {
	s := newCollisionFollowServer(t)
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3200, 3200, 0)
	n := newRegisteredNpc(t, s, blockingNpcType(objtype.BlockWalkNPC), true) // T = (3200,3200,0)

	// Walk off T: two steps east.
	n.queueWaypoints([]int{coordgrid.PackCoord(0, 3202, 3200)})
	for range 2 {
		if dir := n.validateAndAdvanceStep(s); dir == -1 {
			t.Fatalf("walk-off: step from (%d,%d) did not advance", n.x, n.z)
		}
	}
	if n.x != 3202 || n.z != 3200 {
		t.Fatalf("walk-off: NPC at (%d,%d), want (3202,3200)", n.x, n.z)
	}

	// Path back onto the spawn tile — must arrive by WALKING (no teleport).
	n.tele = false
	n.queueWaypoints([]int{coordgrid.PackCoord(0, 3200, 3200)})
	for range 2 {
		if dir := n.validateAndAdvanceStep(s); dir == -1 {
			t.Fatalf("walk-home: NPC blocked at (%d,%d) walking back onto its own spawn tile — frozen own FlagBlockNPCs (the Hans shape)", n.x, n.z)
		}
	}
	if n.x != 3200 || n.z != 3200 {
		t.Fatalf("walk-home: NPC at (%d,%d), want spawn (3200,3200)", n.x, n.z)
	}
	if n.tele {
		t.Error("walk-home: arrived with tele=true, want pure walking arrival")
	}
}

// --- 5. blockWalk NONE moves no flags; ALL moves both families -------------

func TestNpcBlockWalkNone_StepMovesNoFlags(t *testing.T) {
	s := newCollisionFollowServer(t)
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3200, 3200, 0)
	n := newRegisteredNpc(t, s, blockingNpcType(objtype.BlockWalkNone), true)

	both := collision.FlagBlockNPCs | collision.FlagBlockPlayers
	if s.gamemap.Pathfinder.Flags.IsFlagged(3200, 3200, 0, both) {
		t.Fatalf("setup: blockwalk=none spawn tile must carry no entity flags")
	}

	n.queueWaypoints([]int{coordgrid.PackCoord(0, 3201, 3200)})
	if dir := n.validateAndAdvanceStep(s); dir == -1 {
		t.Fatalf("setup: NPC step east did not advance")
	}

	if s.gamemap.Pathfinder.Flags.IsFlagged(3200, 3200, 0, both) {
		t.Errorf("blockwalk=none: old tile gained entity flags after step")
	}
	if s.gamemap.Pathfinder.Flags.IsFlagged(3201, 3200, 0, both) {
		t.Errorf("blockwalk=none: new tile gained entity flags after step (TS switch has no NONE case, PathingEntity.ts:168-179)")
	}
}

func TestNpcBlockWalkAll_StepMovesBothFlagFamilies(t *testing.T) {
	s := newCollisionFollowServer(t)
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3200, 3200, 0)
	n := newRegisteredNpc(t, s, blockingNpcType(objtype.BlockWalkAll), true)

	both := collision.FlagBlockNPCs | collision.FlagBlockPlayers
	if !s.gamemap.Pathfinder.Flags.IsFlagged(3200, 3200, 0, collision.FlagBlockNPCs) ||
		!s.gamemap.Pathfinder.Flags.IsFlagged(3200, 3200, 0, collision.FlagBlockPlayers) {
		t.Fatalf("setup: blockwalk=all spawn tile must carry BOTH flag families (addNpc seed)")
	}

	n.queueWaypoints([]int{coordgrid.PackCoord(0, 3201, 3200)})
	if dir := n.validateAndAdvanceStep(s); dir == -1 {
		t.Fatalf("setup: NPC step east did not advance")
	}

	if s.gamemap.Pathfinder.Flags.IsFlagged(3200, 3200, 0, both) {
		t.Errorf("blockwalk=all: old tile still carries entity flags after step (TS PathingEntity.ts:174-176)")
	}
	if !s.gamemap.Pathfinder.Flags.IsFlagged(3201, 3200, 0, collision.FlagBlockNPCs) ||
		!s.gamemap.Pathfinder.Flags.IsFlagged(3201, 3200, 0, collision.FlagBlockPlayers) {
		t.Errorf("blockwalk=all: new tile missing one or both flag families after step (TS PathingEntity.ts:175,177)")
	}
}

// TestPlayerBlockWalkNone_StepMovesNoFlags pins the invisibility path:
// SetVisibility(non-default) sets p.blockWalk = BlockWalkNone (TS
// Player.ts:1903); a subsequent step must not move/plant any flags.
func TestPlayerBlockWalkNone_StepMovesNoFlags(t *testing.T) {
	s := newCollisionFollowServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	p.blockWalk = BlockWalkNone
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3200, 3200, 0)

	p.queueWaypoint(3201, 3200)
	if dir := p.validateAndAdvanceStep(); dir == -1 {
		t.Fatal("setup: player step east did not move")
	}

	both := collision.FlagBlockNPCs | collision.FlagBlockPlayers
	if s.gamemap.Pathfinder.Flags.IsFlagged(3201, 3200, 0, both) {
		t.Errorf("blockwalk=none player: new tile gained entity flags after step")
	}
}

// --- 6. Seed balance: spawn/despawn and step/logout round-trips -------------

// TestNpcSpawnDespawnCollisionSeedBalance extends TestRemoveNpcCollisionTogglesOff
// (which deliberately skipped the flag probe) with direct flag assertions:
// addNpc seeds the flag (TS World.ts:1308-1316), removeNpc clears it at the
// CURRENT position (TS World.ts:1339-1347).
func TestNpcSpawnDespawnCollisionSeedBalance(t *testing.T) {
	s := newCollisionFollowServer(t)
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3200, 3200, 0)
	n := newRegisteredNpc(t, s, blockingNpcType(objtype.BlockWalkNPC), true)

	if !s.gamemap.Pathfinder.Flags.IsFlagged(3200, 3200, 0, collision.FlagBlockNPCs) {
		t.Fatalf("spawn: tile missing FlagBlockNPCs after addNpc")
	}

	// Walk one tile so the despawn-side removal exercises the CURRENT
	// (post-move) position, not the spawn tile.
	n.queueWaypoints([]int{coordgrid.PackCoord(0, 3201, 3200)})
	if dir := n.validateAndAdvanceStep(s); dir == -1 {
		t.Fatalf("setup: NPC step east did not advance")
	}

	n.lifecycle = NpcLifecycleDespawn
	s.removeNpc(n, -1)

	for _, tile := range [][2]int{{3200, 3200}, {3201, 3200}} {
		if s.gamemap.Pathfinder.Flags.IsFlagged(tile[0], tile[1], 0, collision.FlagBlockNPCs) {
			t.Errorf("despawn: tile (%d,%d) still carries FlagBlockNPCs after removeNpc", tile[0], tile[1])
		}
	}
}

// TestPlayerStepThenLogout_CollisionSeedBalance: per TS@3c16994c there is NO
// login-time player collision seed — the flag materialises on the first
// refreshZonePresence move (PathingEntity.ts:171) and is cleared
// unconditionally at logout (World.ts:1642, goscape removePlayerInternal).
func TestPlayerStepThenLogout_CollisionSeedBalance(t *testing.T) {
	s := newCollisionFollowServer(t)
	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.client.server = s
	p.x, p.z, p.level = 3200, 3200, 0
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(3200, 3200, 0)

	// No login-time seed: addPlayer must NOT have planted the flag (TS has
	// no changeNpcCollision in its player-add path at 3c16994c).
	if s.gamemap.Pathfinder.Flags.IsFlagged(3200, 3200, 0, collision.FlagBlockNPCs) {
		t.Fatalf("login: tile (3200,3200) carries FlagBlockNPCs before the first move — TS has no login-time seed")
	}

	p.queueWaypoint(3201, 3200)
	if dir := p.validateAndAdvanceStep(); dir == -1 {
		t.Fatal("setup: player step east did not move")
	}
	if !s.gamemap.Pathfinder.Flags.IsFlagged(3201, 3200, 0, collision.FlagBlockNPCs) {
		t.Fatalf("step: tile (3201,3200) missing FlagBlockNPCs (first move materialises the flag, TS PathingEntity.ts:171)")
	}

	s.removePlayerInternal(p)

	if s.gamemap.Pathfinder.Flags.IsFlagged(3201, 3200, 0, collision.FlagBlockNPCs) {
		t.Errorf("logout: tile (3201,3200) still carries FlagBlockNPCs after removePlayerInternal (TS World.ts:1642)")
	}
}
