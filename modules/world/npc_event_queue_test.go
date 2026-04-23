package world

import (
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/buildarea"
	"github.com/zsrv/goscape/pkg/grid"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
)

func newNpcForLifecycleTest(t *testing.T) *Npc {
	t.Helper()
	typ := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 0, DebugName: "test_npc"},
		Stats:      []uint16{0, 0, 0, 10, 0, 0}, // HP=10 at NpcStatHitpoints (3)
		Category:   -1,
	}
	return NewNpc(1, 0, 3094, 3106, 0, typ)
}

func TestNewNpcSeedsBaseType(t *testing.T) {
	n := NewNpc(1, 42, 3094, 3106, 0, &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 42}})
	if n.baseType != 42 {
		t.Errorf("baseType: got %d, want 42 (seeded from typeId)", n.baseType)
	}
}

func TestNpcRevertTypeRestoresBaseType(t *testing.T) {
	n := newNpcForLifecycleTest(t)
	// Simulate a prior changetype: typeId now 99, uid recomputed.
	n.typeId = 99
	n.uid = (99 << 16) | n.nid

	n.revertType()

	if n.typeId != n.baseType {
		t.Errorf("typeId: got %d, want %d (baseType)", n.typeId, n.baseType)
	}
	wantUID := (n.baseType << 16) | n.nid
	if n.uid != wantUID {
		t.Errorf("uid: got %d, want %d", n.uid, wantUID)
	}
}

func TestNpcRevertTypeClearsQueue(t *testing.T) {
	n := newNpcForLifecycleTest(t)
	n.queue = []script.NpcQueueRequest{{Trigger: script.TriggerAiQueue1, Delay: 5, IntArg: 0}}

	n.revertType()

	if len(n.queue) != 0 {
		t.Errorf("queue: got %d entries, want 0 (cleared)", len(n.queue))
	}
}

func TestNpcRevertTypeClearsWaypoints(t *testing.T) {
	n := newNpcForLifecycleTest(t)
	n.waypointIndex = 3

	n.revertType()

	if n.waypointIndex != -1 {
		t.Errorf("waypointIndex: got %d, want -1 (cleared)", n.waypointIndex)
	}
}

func TestNpcRevertTypeRaisesTeleAndMask(t *testing.T) {
	n := newNpcForLifecycleTest(t)
	n.tele = false
	n.masks = 0

	n.revertType()

	if !n.tele {
		t.Errorf("tele: got false, want true")
	}
	if n.masks&rsbuf.NpcMaskChangeType == 0 {
		t.Errorf("masks: NpcMaskChangeType bit not set")
	}
}

func TestNpcTurnEventsRespawnPathAfterKill(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.respawnRate = 5
	n.lifecycle = NpcLifecycleRespawn
	n.x, n.z = n.startX+3, n.startZ+3 // moved away from spawn before death

	n.Kill() // sets n.dead=true, n.lifecycleTick=respawnRate=5

	// Tick respawnRate times; lifecycleTick goes 5→4→3→2→1→0 on the 5th call.
	for i := 0; i < 5; i++ {
		n.turn(s)
	}

	if n.dead {
		t.Errorf("dead: got true, want false (should have respawned)")
	}
	if n.x != n.startX || n.z != n.startZ {
		t.Errorf("pos: got (%d,%d), want (%d,%d) (should reset to spawn)", n.x, n.z, n.startX, n.startZ)
	}
	if !n.tele {
		t.Errorf("tele: got false, want true (revertType raises it)")
	}
}

func TestNpcTurnEventsDoesNotFireWhileDelayed(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.delayed = true
	n.delayedUntil = s.currentTick + 999
	n.lifecycleTick = 1
	n.lifecycle = NpcLifecycleRespawn

	for i := 0; i < 5; i++ {
		n.turn(s)
	}

	if n.lifecycleTick != 1 {
		t.Errorf("lifecycleTick: got %d, want 1 (no decrement while delayed)", n.lifecycleTick)
	}
}

func TestNpcTurnEventsDespawnEnqueuesEvent(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.lifecycle = NpcLifecycleDespawn
	n.lifecycleTick = 2

	// No scriptProvider registered → GetByTrigger returns nil → no enqueue,
	// but n.dead must flip true.
	n.turn(s)
	n.turn(s)

	if !n.dead {
		t.Errorf("dead: got false, want true (DESPAWN should have fired removeNpc)")
	}
	if len(s.npcEventQueue) != 0 {
		t.Errorf("npcEventQueue: got len %d, want 0 (no ai_despawn script registered)", len(s.npcEventQueue))
	}
}

func TestNewNpcSeedsRegenInterval(t *testing.T) {
	typ := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 0, DebugName: "test_npc"},
		RegenRate:  7,
	}
	n := NewNpc(1, 0, 3094, 3106, 0, typ)

	if n.regenInterval != 7 {
		t.Errorf("regenInterval: got %d, want 7 (seeded from typ.RegenRate)", n.regenInterval)
	}
}

func TestProcessNpcRegenIncrementsClock(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.regenInterval = 100
	n.regenClock = 0
	n.curHP, n.baseHP = 8, 10

	s.processNpcRegen(n)

	if n.regenClock != 1 {
		t.Errorf("regenClock: got %d, want 1", n.regenClock)
	}
	if n.curHP != 8 {
		t.Errorf("curHP: got %d, want 8 (no regen fire yet)", n.curHP)
	}
}

func TestProcessNpcRegenFiresAtInterval(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.regenInterval = 3
	n.regenClock = 0
	n.curHP, n.baseHP = 5, 10

	// Simulate a type-change that would set RegenRate=99 — the
	// Vorkath quirk means this new rate only takes effect on the
	// regen fire, not here. Before the fire, regenInterval is
	// still 3.
	n.typ.RegenRate = 99

	// 2 ticks: clock goes 0→1→2; no fire yet.
	s.processNpcRegen(n)
	s.processNpcRegen(n)
	if n.regenClock != 2 {
		t.Fatalf("regenClock after 2 ticks: got %d, want 2", n.regenClock)
	}
	if n.curHP != 5 {
		t.Fatalf("curHP after 2 ticks: got %d, want 5 (pre-fire)", n.curHP)
	}

	// 3rd tick: clock 2→3, fires. Interval reloads to 99; clock
	// resets to 0; curHP increments 5→6.
	s.processNpcRegen(n)
	if n.regenClock != 0 {
		t.Errorf("regenClock after fire: got %d, want 0 (reset)", n.regenClock)
	}
	if n.regenInterval != 99 {
		t.Errorf("regenInterval after fire: got %d, want 99 (reloaded from typ.RegenRate)", n.regenInterval)
	}
	if n.curHP != 6 {
		t.Errorf("curHP after fire: got %d, want 6 (incremented toward baseHP=10)", n.curHP)
	}
}

func TestProcessNpcRegenClampsAtBaseHP(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.regenInterval = 3
	n.regenClock = 0
	n.curHP, n.baseHP = 10, 10

	for i := 0; i < 3; i++ {
		s.processNpcRegen(n)
	}

	if n.curHP != 10 {
		t.Errorf("curHP: got %d, want 10 (no change at equal)", n.curHP)
	}
}

func TestProcessNpcRegenDecrementsWhenAboveBase(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.regenInterval = 3
	n.regenClock = 0
	n.curHP, n.baseHP = 12, 10

	for i := 0; i < 3; i++ {
		s.processNpcRegen(n)
	}

	if n.curHP != 11 {
		t.Errorf("curHP: got %d, want 11 (decremented toward baseHP=10)", n.curHP)
	}
}

func TestNewNpcSeedsHuntFromType(t *testing.T) {
	typ := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 0, DebugName: "test_npc"},
		HuntMode:   3,
		HuntRange:  5,
	}
	n := NewNpc(1, 0, 3094, 3106, 0, typ)

	if n.huntMode != 3 {
		t.Errorf("huntMode: got %d, want 3 (seeded from typ.HuntMode)", n.huntMode)
	}
	if n.huntRange != 5 {
		t.Errorf("huntRange: got %d, want 5 (seeded from typ.HuntRange)", n.huntRange)
	}
}

func TestNpcSetHuntRangeAndMode(t *testing.T) {
	n := newNpcForLifecycleTest(t)

	n.SetHuntRange(7)
	if n.huntRange != 7 {
		t.Errorf("huntRange after SetHuntRange(7): got %d, want 7", n.huntRange)
	}

	n.SetHuntMode(2)
	if n.huntMode != 2 {
		t.Errorf("huntMode after SetHuntMode(2): got %d, want 2", n.huntMode)
	}

	// -1 is a valid clear value (not a no-op like SetTimer).
	n.SetHuntMode(-1)
	if n.huntMode != -1 {
		t.Errorf("huntMode after SetHuntMode(-1): got %d, want -1 (clear)", n.huntMode)
	}
}

func TestNpcRevertTypeResetsHuntFields(t *testing.T) {
	// Use a typ with explicit HuntMode=2, HuntRange=4 so we can
	// verify the reset brings fields BACK to those values after
	// scripts mutate them.
	typ := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 0, DebugName: "test_npc"},
		Stats:      []uint16{0, 0, 0, 10, 0, 0},
		Category:   -1,
		HuntMode:   2,
		HuntRange:  4,
	}
	n := NewNpc(1, 0, 3094, 3106, 0, typ)

	// Mutate all 4 hunt fields (simulating live hunt state).
	n.huntRange = 99
	n.huntMode = 0
	n.huntClock = 42
	n.huntTarget = nil // already nil; just documenting the expected reset

	n.revertType()

	if n.huntRange != 4 {
		t.Errorf("huntRange: got %d, want 4 (reset from typ.HuntRange)", n.huntRange)
	}
	if n.huntMode != 2 {
		t.Errorf("huntMode: got %d, want 2 (reset from typ.HuntMode)", n.huntMode)
	}
	if n.huntClock != 0 {
		t.Errorf("huntClock: got %d, want 0 (reset)", n.huntClock)
	}
	if n.huntTarget != nil {
		t.Errorf("huntTarget: got %v, want nil (reset)", n.huntTarget)
	}
}

func TestProcessNpcHuntSkipsWhenHuntModeNegative(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.huntMode = -1
	n.huntClock = 0

	s.processNpcHunt(n)

	if n.huntClock != 0 {
		t.Errorf("huntClock: got %d, want 0 (no-op when huntMode=-1)", n.huntClock)
	}
}

func TestProcessNpcHuntIncrementsClockWhenHuntModeValid(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	// Seed a HuntTypeConfigs with index 0 being a "always-gate-open"
	// HuntType. Type=Off means huntAll short-circuits at the
	// `HuntModeOff || huntRange < 1` check; NobodyNear=KeepHunting
	// means the observer gate passes. Net effect: gate passes, clock
	// increments, huntAll is a no-op.
	s.huntTypes = &objtype.HuntTypeConfigs{
		Configs: []*objtype.HuntType{
			{
				Type:       objtype.HuntModeOff,
				NobodyNear: objtype.HuntNobodyNearKeepHunting,
				Rate:       1,
			},
		},
	}
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.huntMode = 0
	n.huntClock = 0

	s.processNpcHunt(n)

	if n.huntClock != 1 {
		t.Errorf("huntClock: got %d, want 1 (gate passes, clock increments)", n.huntClock)
	}
}

// TestProcessNpcHuntPauseHuntBailsWithNoObservers validates that
// PAUSEHUNT gates short-circuit (skip huntAll and clock increment)
// when observer count is zero and hunt type is not HuntModePlayer.
// Observer count is seeded via rsbuf.SetObserverForTest.
func TestProcessNpcHuntPauseHuntBailsWithNoObservers(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t) // nid = 1 per NewNpc's arg
	rsbuf.SetObserverForTest(n.nid, 0)
	defer rsbuf.SetObserverForTest(n.nid, 0)
	n.server = s
	n.huntMode = 0 // index into huntTypes
	n.huntRange = 10
	n.huntClock = 0
	s.huntTypes = &objtype.HuntTypeConfigs{
		Configs: []*objtype.HuntType{
			{
				Type:       objtype.HuntModeNpc,
				NobodyNear: objtype.HuntNobodyNearPauseHunt,
				Rate:       1,
			},
		},
	}

	s.processNpcHunt(n)

	if n.huntClock != 0 {
		t.Errorf("huntClock: got %d, want 0 (PAUSEHUNT gate short-circuited)", n.huntClock)
	}
}

func TestProcessNpcHuntPauseHuntRunsWithObservers(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	rsbuf.SetObserverForTest(n.nid, 1)       // seed one observer
	defer rsbuf.SetObserverForTest(n.nid, 0) // cleanup
	n.server = s
	n.huntMode = 0
	n.huntRange = 10
	n.huntClock = 0
	s.huntTypes = &objtype.HuntTypeConfigs{
		Configs: []*objtype.HuntType{
			{
				Type:       objtype.HuntModeNpc,
				NobodyNear: objtype.HuntNobodyNearPauseHunt,
				Rate:       1,
			},
		},
	}

	s.processNpcHunt(n)

	if n.huntClock != 1 {
		t.Errorf("huntClock: got %d, want 1 (gate passed, huntClock advanced)", n.huntClock)
	}
}

func TestProcessNpcEventQueueSkipsDelayedNpcs(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.delayed = true
	n.delayedUntil = s.currentTick + 999

	sf := &script.ScriptFile{
		Name:    "ai_despawn_stub",
		Opcodes: []script.Opcode{script.OpReturn},
	}
	s.npcEventQueue = append(s.npcEventQueue, NpcEventRequest{
		Type:   NpcEventDespawn,
		Script: sf,
		Npc:    n,
	})

	s.processNpcEventQueue()

	if len(s.npcEventQueue) != 1 {
		t.Errorf("npcEventQueue: got len %d, want 1 (delayed NPC's event must be skipped, not removed)", len(s.npcEventQueue))
	}
}

// addPlayerToServer seeds s.players[slot] + s.grid with a minimal
// *Player at the given coords. Used by NAI-8 huntPlayers tests.
// Slot 0 is reserved per existing convention.
func addPlayerToServer(t *testing.T, s *Server, slot, x, z, level int) *Player {
	t.Helper()
	if s.grid == nil {
		s.grid = grid.New()
	}
	p := &Player{
		slot:  slot,
		x:     x,
		z:     z,
		level: level,
	}
	s.players[slot] = p
	s.grid.Add(slot, x, z, level)
	return p
}

func TestHuntPlayersInRange(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	pInRange := addPlayerToServer(t, s, 1, n.x+3, n.z+3, n.level)
	_ = addPlayerToServer(t, s, 2, n.x+20, n.z+20, n.level) // out of range

	hunt := &objtype.HuntType{CheckNotCombat: -1}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 1 {
		t.Fatalf("hunted: got %d players, want 1 (in-range only)", len(hunted))
	}
	if hunted[0].Slot() != pInRange.slot {
		t.Errorf("hunted[0]: got slot %d, want slot %d", hunted[0].Slot(), pInRange.slot)
	}
}

func TestHuntPlayersFiltersByLevel(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	pSameLevel := addPlayerToServer(t, s, 1, n.x+2, n.z+2, n.level)
	_ = addPlayerToServer(t, s, 2, n.x+2, n.z+2, n.level+1) // wrong level

	hunt := &objtype.HuntType{CheckNotCombat: -1}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 1 {
		t.Fatalf("hunted: got %d, want 1 (same-level only)", len(hunted))
	}
	if hunted[0].Slot() != pSameLevel.slot {
		t.Errorf("hunted[0]: got slot %d, want slot %d", hunted[0].Slot(), pSameLevel.slot)
	}
}

func TestHuntPlayersSkipsAfkZonedPlayers(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	n.huntRange = 10

	pActive := addPlayerToServer(t, s, 1, n.x+2, n.z+2, n.level)
	pAfk := addPlayerToServer(t, s, 2, n.x+3, n.z+3, n.level)
	pAfk.lastAfkZone = 1000 // IsZonesAfk saturates at 1000

	// With CheckAfk=true, AFK player is filtered.
	huntWithAfk := &objtype.HuntType{CheckAfk: true, CheckNotCombat: -1}
	hunted := n.huntPlayers(s, huntWithAfk)
	if len(hunted) != 1 {
		t.Fatalf("CheckAfk=true: got %d, want 1 (AFK filtered)", len(hunted))
	}
	if hunted[0].Slot() != pActive.slot {
		t.Errorf("CheckAfk=true: got slot %d, want slot %d (active)", hunted[0].Slot(), pActive.slot)
	}

	// With CheckAfk=false, both players returned.
	huntNoAfk := &objtype.HuntType{CheckAfk: false, CheckNotCombat: -1}
	hunted = n.huntPlayers(s, huntNoAfk)
	if len(hunted) != 2 {
		t.Errorf("CheckAfk=false: got %d, want 2 (filter inactive, both returned)", len(hunted))
	}
}

func TestHuntPlayersReturnsEmptyWhenNoCandidates(t *testing.T) {
	s := newServerForScriptTest(t)
	s.grid = grid.New()
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.huntRange = 10

	hunt := &objtype.HuntType{CheckNotCombat: -1}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 0 {
		t.Errorf("hunted: got %d, want 0 (empty grid)", len(hunted))
	}
}

func TestProcessLogoutsDecrementsSubscribedNpcObservers(t *testing.T) {
	rsbuf.SetObserverForTest(101, 0) // cleanup — ensure clean state
	rsbuf.SetObserverForTest(102, 0)

	s := newServerForScriptTest(t)
	s.currentTick = 1

	// Create a minimal player with a client and buildArea.
	// This mirrors the server_test pattern.
	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() {
		serverConn.Close()
		clientConn.Close()
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	c := newClient(serverConn, time.Second, logger)
	t.Cleanup(func() { c.in.Release() })
	c.server = s
	c.state = ClientStateGame
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	c.encryptor = enc
	go io.Copy(io.Discard, clientConn)

	p := newPlayer(c)
	c.player = p
	if err := s.addPlayer(p); err != nil {
		t.Fatal(err)
	}

	// Seed a buildArea subscribing to two NPCs and set observer counts to 1.
	p.buildArea = buildarea.New()
	p.buildArea.Npcs[101] = struct{}{}
	p.buildArea.Npcs[102] = struct{}{}
	rsbuf.SetObserverForTest(101, 1)
	rsbuf.SetObserverForTest(102, 1)

	// Trigger logout: set loggingOut flag (force logout regardless of timing).
	p.loggingOut = true
	p.preventLogoutUntil = 0

	s.processLogouts()

	if got := rsbuf.GetNpcObservers(101); got != 0 {
		t.Errorf("GetNpcObservers(101) after logout: got %d, want 0", got)
	}
	if got := rsbuf.GetNpcObservers(102); got != 0 {
		t.Errorf("GetNpcObservers(102) after logout: got %d, want 0", got)
	}
}
