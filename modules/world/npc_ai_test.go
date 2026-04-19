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
