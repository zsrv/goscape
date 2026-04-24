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
	n.wanderCounter = 501
	s := &Server{}
	n.turn(s)
	if n.x != n.startX || n.z != n.startZ {
		t.Errorf("teleport home: got (%d,%d), want (%d,%d)", n.x, n.z, n.startX, n.startZ)
	}
	if !n.tele {
		t.Error("tele flag should be set after teleport home")
	}
}

func TestRespawnAfterKill(t *testing.T) {
	n := newWanderNpc(t)
	n.x, n.z = 3094+5, 3106+5
	n.Kill()
	s := &Server{}
	for range n.respawnRate {
		n.turn(s)
	}
	if n.dead {
		t.Error("should have respawned by now")
	}
	if n.x != n.startX || n.z != n.startZ {
		t.Errorf("respawn coords: got (%d,%d), want (%d,%d)", n.x, n.z, n.startX, n.startZ)
	}
	if n.masks&rsbuf.NpcMaskChangeType == 0 {
		t.Error("respawn should raise NpcMaskChangeType")
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
