package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/objtype"
)

// TestUpdateMovement_LiveMoveRestrict pins npc-core-2: TS
// Npc.updateMovement (Npc.ts:337-341) reads moverestrict LIVE from
// `NpcType.get(this.type)` every tick. goscape's n.moveRestrict is a
// snapshot frozen at NewNpc, but n.typ IS refreshed on ChangeType
// (npc_masks.go:87). After a ChangeType to a NoMove type, the next
// updateMovement must observe NoMove on the LIVE typ even when the
// frozen n.moveRestrict still says Normal.
func TestUpdateMovement_LiveMoveRestrict(t *testing.T) {
	// Construct with MoveRestrict=Normal: NewNpc sets n.moveRestrict=Normal.
	typ := &objtype.NpcType{
		ID:           42,
		MoveRestrict: int(MoveRestrictNormal),
	}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.moveSpeed = MoveSpeedWalk
	n.waypoints[0] = coordgrid.PackCoord(0, 103, 100)
	n.waypointIndex = 0

	// Sanity: frozen n.moveRestrict matches the constructed typ.
	if n.moveRestrict != MoveRestrictNormal {
		t.Fatalf("precondition: n.moveRestrict=%v, want Normal", n.moveRestrict)
	}

	// Simulate a ChangeType refresh that updated n.typ but the
	// pre-fix frozen n.moveRestrict snapshot stayed Normal.
	n.typ.MoveRestrict = int(MoveRestrictNoMove)

	s := newTestServer(t)
	n.server = s

	moved := n.updateMovement(s)

	if moved {
		t.Errorf("updateMovement: returned true, want false (TS Npc.ts:337-341 reads NpcType.moverestrict LIVE → NoMove must short-circuit before the step). Pre-fix reads frozen n.moveRestrict=Normal and steps anyway.")
	}
	if n.walkDir != -1 {
		t.Errorf("walkDir: got %d, want -1 (NoMove path resets directions)", n.walkDir)
	}
}

// TestWanderMode_LiveMoveRestrict pins the wanderMode half of
// npc-core-2: TS Npc.wanderMode (Npc.ts:697-703) does
// `NpcType.get(this.type)` and gates the 1/8 roll on
// `type.moverestrict !== NOMOVE`. After ChangeType to a NoMove type
// the wander roll must no longer queue any random walk, even when the
// frozen n.moveRestrict snapshot still says Normal.
func TestWanderMode_LiveMoveRestrict(t *testing.T) {
	typ := &objtype.NpcType{
		ID:           1,
		MoveRestrict: int(MoveRestrictNormal),
		WanderRange:  3,
		DefaultMode:  objtype.NPCModeWander,
	}
	n := NewNpc(1, 0, 3094, 3106, 0, typ)

	if n.moveRestrict != MoveRestrictNormal {
		t.Fatalf("precondition: n.moveRestrict=%v, want Normal", n.moveRestrict)
	}

	// Refresh typ to NoMove without touching the frozen snapshot.
	n.typ.MoveRestrict = int(MoveRestrictNoMove)

	s := &Server{}

	// Run many ticks; expectation is that NO wander roll ever queues a
	// waypoint, because the live moverestrict is NoMove. Pre-fix reads
	// n.moveRestrict (still Normal) and the 1/8 roll fires ~12.5%.
	const iters = 400
	hits := 0
	for range iters {
		n.x = n.startX
		n.z = n.startZ
		n.waypointIndex = -1
		n.wanderCounter = 0
		n.wanderMode(s)
		if n.waypointIndex >= 0 {
			hits++
		}
	}

	if hits != 0 {
		t.Errorf("wanderMode under live NoMove: queued %d/%d ticks; want 0 (TS Npc.ts:701 gate `type.moverestrict !== NOMOVE` reads LIVE). Pre-fix reads frozen n.moveRestrict=Normal and rolls normally (~%d expected).", hits, iters, iters/8)
	}
}
