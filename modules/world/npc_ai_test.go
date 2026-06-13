package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

func newWanderNpc(t *testing.T) *Npc {
	t.Helper()
	typ := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 0, DebugName: "wanderer"},
		WanderRange: 5,
		RespawnRate: 50,
		// defaultMode reads this stored field (TS Npc.ts:414); a wander NPC
		// has defaultmode=wander. NewNpcType defaults it to Wander, but this
		// bare literal must set it explicitly.
		DefaultMode: objtype.NPCModeWander,
	}
	return NewNpc(1, 0, 3094, 3106, 0, typ)
}

func TestKillSetsDeadAndLifecycleTick(t *testing.T) {
	n := newWanderNpc(t)
	n.Kill()
	if !n.dead {
		t.Error("Kill should set dead=true")
	}
	if n.lifecycleTick != 50 {
		t.Errorf("lifecycleTick: got %d, want 50", n.lifecycleTick)
	}
}

func TestTeleportHomeAfterStuck(t *testing.T) {
	n := newWanderNpc(t)
	n.x, n.z = 3094+10, 3106+10
	// MoveRestrictNoMove makes this a deterministically *stuck* NPC: wanderMode
	// skips the 1/8 random-walk roll and updateMovement can't step, so the
	// NPC's position never changes and stuckCounter is never reset on-move.
	// Without it the random roll occasionally moves the NPC, resetting the
	// counter and making this teleport-home assertion flaky — which is exactly
	// the correct stuck-recovery semantics. NoMove must live on the TYPE:
	// wanderMode/updateMovement read moverestrict live from NpcType
	// (2787f1fb removed the PathingEntity field).
	n.typ.MoveRestrict = int(MoveRestrictNoMove)
	// rev-254: updateMovement's moved-check is positional vs lastTick;
	// seed the snapshot so the fresh-NPC -1 sentinel doesn't read as
	// "moved" and reset stuckCounter.
	n.lastTickX, n.lastTickZ = n.x, n.z
	// rev-274: TS wanderMode @dee467c8 is `if (this.stuckCounter++ > 500)`,
	// so the teleport fires the tick stuckCounter is 501 (pre-increment > 500).
	n.stuckCounter = 501
	s := &Server{}
	n.turn(s)
	if n.x != n.startX || n.z != n.startZ {
		t.Errorf("teleport home: got (%d,%d), want (%d,%d)", n.x, n.z, n.startX, n.startZ)
	}
	if !n.tele {
		t.Error("tele flag should be set after teleport home")
	}
}

// TestWanderModeStuckTeleportBoundary pins the rev-274 off-by-one against TS
// wanderMode @dee467c8: `if (this.stuckCounter++ > 500)`. The PRE-increment
// value is compared to 500, so the teleport-home fires when stuckCounter is
// 501 and NOT when it is 500 (the old `++; >= 500` form fired at 500 — one
// tick early). Both halves are pinned: 500 → no tele (counter advances to
// 501), 501 → tele (counter reset to 0).
func TestWanderModeStuckTeleportBoundary(t *testing.T) {
	mk := func(t *testing.T) (*Npc, *Server) {
		n := newWanderNpc(t)
		n.x, n.z = 3094+10, 3106+10
		n.typ.MoveRestrict = int(MoveRestrictNoMove)
		n.lastTickX, n.lastTickZ = n.x, n.z
		return n, &Server{}
	}

	// stuckCounter == 500 → comparison 500 > 500 is false → no teleport;
	// counter increments to 501.
	n, s := mk(t)
	n.stuckCounter = 500
	n.wanderMode(s)
	if n.x == n.startX && n.z == n.startZ {
		t.Error("counter 500: NPC teleported home, want NO teleport (500 > 500 is false)")
	}
	if n.stuckCounter != 501 {
		t.Errorf("counter 500: stuckCounter after tick = %d, want 501 (incremented, no reset)", n.stuckCounter)
	}

	// stuckCounter == 501 → comparison 501 > 500 is true → teleport; reset 0.
	n, s = mk(t)
	n.stuckCounter = 501
	n.wanderMode(s)
	if n.x != n.startX || n.z != n.startZ {
		t.Errorf("counter 501: coords (%d,%d), want home (%d,%d) (501 > 500 fires)", n.x, n.z, n.startX, n.startZ)
	}
	if n.stuckCounter != 0 {
		t.Errorf("counter 501: stuckCounter after tele = %d, want 0 (reset)", n.stuckCounter)
	}
}

func TestRespawnAfterKill(t *testing.T) {
	n := newWanderNpc(t)
	n.x, n.z = 3094+5, 3106+5
	n.Kill()
	s := &Server{}
	// NAI-19 Task 5e: revertType's heavy path now calls through
	// n.server.removeNpc + n.server.addNpc, so n.server must be wired.
	n.server = s
	for range n.respawnRate {
		n.turn(s)
	}
	if n.dead {
		t.Error("should have respawned by now")
	}
	if n.x != n.startX || n.z != n.startZ {
		t.Errorf("respawn coords: got (%d,%d), want (%d,%d)", n.x, n.z, n.startX, n.startZ)
	}
	// NAI-20 Task 2: NpcMaskChangeType is only raised when typeId != baseType
	// (morph-revert path). Normal respawn after kill (typeId == baseType) does
	// NOT raise the mask per TS resetEntity(true) semantics.
	if n.masks&rsbuf.NpcMaskChangeType != 0 {
		t.Error("normal respawn (no changetype) must NOT raise NpcMaskChangeType (NAI-20 gate)")
	}
}

func TestWanderModeFrequency(t *testing.T) {
	n := newWanderNpc(t)
	s := &Server{}
	hits := 0
	for range 8000 {
		n.waypointIndex = -1
		n.wanderMode(s)
		if n.waypointIndex >= 0 {
			hits++
		}
	}
	// Expect ~1000; allow +/-25%.
	if hits < 750 || hits > 1250 {
		t.Errorf("wander hit rate: got %d/8000, want ~1000 (12.5%%)", hits)
	}
}

// TestWanderMode_ZeroRange_DisplacedNpc_QueuesSpawnReturn pins the 2026-05-28
// audit row npc-ai-4. TS wanderMode (Npc.ts:697-715) gates ONLY on
// `moverestrict !== NOMOVE && Math.random() < 0.125`, then calls
// `randomWalk(type.wanderrange)` UNCONDITIONALLY. TS randomWalk
// (Npc.ts:682-691) with range=0 computes dx=dz=0; if the NPC has drifted
// off-spawn, it queues a waypoint BACK to (startX, startZ).
//
// goscape pre-fix added an extra `&& n.typ.WanderRange > 0` clause to
// the gate, suppressing the wander roll entirely for 0-range NPCs.
// A drifted 0-range NPC could therefore only return home via the
// 500-tick teleport-to-spawn counter, not via the natural 1/8 wander
// re-queue.
func TestWanderMode_ZeroRange_DisplacedNpc_QueuesSpawnReturn(t *testing.T) {
	typ := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 0, DebugName: "zero_range"},
		WanderRange: 0,
		DefaultMode: objtype.NPCModeWander,
	}
	n := NewNpc(1, 0, 3094, 3106, 0, typ)
	s := &Server{}

	hits := 0
	const iters = 800
	for range iters {
		// Re-seed displaced state each iter. waypointIndex reset and
		// wanderCounter reset isolate the wander-roll signal from
		// (a) prior-iter waypoint leftover, (b) the 500-tick home-tele.
		n.x = n.startX + 1
		n.z = n.startZ
		n.waypointIndex = -1
		n.stuckCounter = 0
		n.wanderMode(s)
		// Hit signal: NPC returned to spawn this tick. wanderMode's
		// updateMovement (with nil gamemap, test-fixture path at
		// npc_interaction.go:423) auto-applies the queued step, so the
		// queued waypoint (startX, startZ) is consumed in-call and the
		// NPC ends at spawn iff the 1/8 wander roll fired AND randomWalk
		// queued the return.
		if n.x == n.startX && n.z == n.startZ {
			hits++
		}
	}

	// Expect ~100 hits (12.5% of 800). Use the same generous tolerance as
	// TestWanderModeFrequency (~±60%) to keep the test non-flaky under
	// math/rand/v2's per-process seed.
	if hits < 40 {
		t.Errorf("WanderRange=0 displaced NPC: %d/%d ticks returned to spawn; expected ~100 (1/8 cadence per TS Npc.ts:701). Pre-fix gate `&& WanderRange > 0` suppressed every wander roll for 0-range NPCs.", hits, iters)
	}
}

// TestNpcTurnRespawnAliveMorphReverts directly exercises the
// `lifecycle=Respawn && !dead` branch at npc_ai.go:37-40: when an
// alive morphed NPC's lifecycleTick hits 0, revertType() fires and
// typeId is restored to baseType.
//
// NAI-5 originally covered this branch only indirectly through
// TestNpcTurnEventsRespawnPathAfterKill (which tests the dead-npc
// respawn path). This direct test isolates the alive-morph branch
// and its revertType() invocation.
//
// DELIBERATELY does NOT use (*Npc).ChangeType to set up the morph —
// the point is to assert revertType()'s post-condition without
// depending on ChangeType's semantics. See NAI-16 spec § 4.
func TestNpcTurnRespawnAliveMorphReverts(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForLifecycleTest(t)
	n.server = s
	// Simulate post-changetype state: typeId mutated, uid recomputed,
	// lifecycleTick scheduling a revert in 3 ticks.
	n.typeId = 99
	n.uid = (99 << 16) | n.nid
	n.lifecycle = NpcLifecycleRespawn
	n.dead = false
	n.lifecycleTick = 3
	n.masks = 0 // clear mask so we can assert revertType raises it

	for range 3 {
		n.turn(s)
	}

	if n.typeId != n.baseType {
		t.Errorf("typeId: got %d, want baseType %d (revertType should restore)", n.typeId, n.baseType)
	}
	wantUID := (n.baseType << 16) | n.nid
	if n.uid != wantUID {
		t.Errorf("uid: got %d, want %d (recomputed from baseType)", n.uid, wantUID)
	}
	if n.masks&rsbuf.NpcMaskChangeType == 0 {
		t.Error("masks: NpcMaskChangeType bit not set (revertType should raise it)")
	}
	if !n.tele {
		t.Error("tele: got false, want true (revertType raises it)")
	}
}
