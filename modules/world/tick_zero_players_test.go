package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

// rev-274 perf gates (TS World.ts @dee467c8): when zero players are online,
// the world skips the npc-hunt pass (World.ts:576 `if (this.getTotalPlayers()
// > 0)`) and the info-processing pass (World.ts:979-981 early return). These
// are pure perf short-circuits — an empty world has no observers, so neither
// pass can produce visible work — but pinning them guards against the gate
// being dropped on a future merge.

// TestProcessInfo_ZeroPlayers_SkipsBody pins the rev-274 processInfo gate:
// with getTotalPlayers()==0 the function returns immediately, before the
// per-NPC reorient loop runs. Observable via Npc.reorient: an NPC with a
// cached targetX and stepsTaken==0 has its targetX cleared to -1 by the
// reorient loop — but ONLY if processInfo's body executes. With the gate the
// targetX survives.
func TestProcessInfo_ZeroPlayers_SkipsBody(t *testing.T) {
	s := newTestServer(t)
	s.renderer = rsbuf.NewRenderer()

	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.target = nil
	n.targetX, n.targetZ = 99, 99 // cached focus coord; reorient would clear to -1
	n.stepsTaken = 0
	s.npcLoop = append(s.npcLoop, n)

	if got := s.getTotalPlayers(); got != 0 {
		t.Fatalf("precondition: getTotalPlayers()=%d, want 0", got)
	}

	s.processInfo()

	if n.targetX != 99 || n.targetZ != 99 {
		t.Errorf("processInfo ran its body at 0 players (targetX=%d,targetZ=%d, want 99,99); "+
			"the World.ts:979-981 0-player early return is missing", n.targetX, n.targetZ)
	}
}

// The positive complement (processInfo runs its body with a player online) is
// covered by the broader tick integration tests, which set up the full player
// state (buildArea, etc.) that processInfo's rebuildNormal pass requires —
// reconstructing that here just to re-prove the gate is conditional would
// duplicate fragile setup. The hunt-pass positive case IS pinned directly by
// TestProcessNpcHuntPlayers_AcquiresObservedPlayer (npc_hunt_test.go).

// TestProcessNpcHuntPlayers_ZeroPlayers_NoHunt pins the rev-274 hunt-pass
// gate (TS World.ts:576 `if (this.getTotalPlayers() > 0)`): with zero players
// online the world-level player-hunt pass acquires no target. The fixture
// seeds an observed, in-hunt-range HuntModePlayer NPC, then removes its player
// so getTotalPlayers()==0 — observers remain >0 (they are a persistent
// counter), so the ONLY thing keeping huntAll from scanning is the
// 0-player short-circuit.
func TestProcessNpcHuntPlayers_ZeroPlayers_NoHunt(t *testing.T) {
	s := newServerForScriptTest(t)
	s.gamemap = gamemap.New(discardLogger())

	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10
	n.huntMode = 0
	n.huntClock = 0
	s.npcLoop = append(s.npcLoop, n)
	s.rsbuf.AddNpc(int32(n.nid), 0)
	s.rsbuf.SetObserverForTest(int32(n.nid), 1) // observers persist even with 0 players

	s.huntTypes = &objtype.HuntTypeConfigs{
		Configs: []*objtype.HuntType{
			{
				Type:               objtype.HuntModePlayer,
				Rate:               1,
				CheckVis:           objtype.HuntVisLineOfSight,
				CheckNotCombat:     -1,
				CheckNotCombatSelf: -1,
				CheckInv:           -1,
			},
		},
	}

	if got := s.getTotalPlayers(); got != 0 {
		t.Fatalf("precondition: getTotalPlayers()=%d, want 0", got)
	}

	s.processNpcHuntPlayers()

	if n.huntTarget != nil {
		t.Errorf("huntTarget: got %v, want nil (0-player hunt-pass gate must short-circuit)", n.huntTarget)
	}
}
