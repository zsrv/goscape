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
	"github.com/zsrv/goscape/pkg/zone"
)

func newNpcForLifecycleTest(t *testing.T) *Npc {
	t.Helper()
	typ := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 0, DebugName: "test_npc"},
		Size:       1, // match production NewNpcType default (npctype.go:310);
		// NAI-18: fixture was silently Size=0 (uint8 zero value), which would
		// collide with HasLineOfSight's lineCoordinate(a, b, 0) → a-1 off-by-one
		// in inApproachDistance's LoS gate. Production NPCs always have Size>=1.
		Stats:    []uint16{0, 0, 0, 10, 0, 0}, // HP=10 at NpcStatHitpoints (3)
		Category: -1,
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
	// NAI-19 Task 5e: revertType's heavy path now calls through
	// n.server.removeNpc + n.server.addNpc; wire the server.
	n.server = newTestServer(t)
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
	n.server = newTestServer(t) // NAI-19 Task 5e: heavy path needs server.
	n.queue = []script.NpcQueueRequest{{Trigger: script.TriggerAiQueue1, Delay: 5, IntArg: 0}}

	n.revertType()

	if len(n.queue) != 0 {
		t.Errorf("queue: got %d entries, want 0 (cleared)", len(n.queue))
	}
}

func TestNpcRevertTypeClearsWaypoints(t *testing.T) {
	n := newNpcForLifecycleTest(t)
	n.server = newTestServer(t) // NAI-19 Task 5e: heavy path needs server.
	n.waypointIndex = 3

	n.revertType()

	if n.waypointIndex != -1 {
		t.Errorf("waypointIndex: got %d, want -1 (cleared)", n.waypointIndex)
	}
}

// TestNpcRevertTypeNonMorphedRaisesTeleNotMask: revertType on an NPC
// where typeId == baseType (non-morphed) raises tele but NOT
// NpcMaskChangeType. NAI-20 Task 2 gated the mask raise inside
// resetEntityForRespawn's morph-detect block per TS resetEntity(true)
// semantics; the morph-revert mask-positive case is pinned by
// TestResetEntityForRespawnRevertRaisesChangeTypeMask in
// npc_registry_test.go.
func TestNpcRevertTypeNonMorphedRaisesTeleNotMask(t *testing.T) {
	n := newNpcForLifecycleTest(t)
	n.server = newTestServer(t) // NAI-19 Task 5e: heavy path needs server.
	n.tele = false
	n.masks = 0

	n.revertType()

	if !n.tele {
		t.Errorf("tele: got false, want true")
	}
	// NAI-20 Task 2: NpcMaskChangeType is only raised when typeId != baseType
	// (morph-revert path). When typeId == baseType (normal respawn), the mask
	// is NOT raised per TS resetEntity(true) semantics. This NPC has not been
	// morphed so the mask should remain clear.
	if n.masks&rsbuf.NpcMaskChangeType != 0 {
		t.Errorf("masks: NpcMaskChangeType should NOT be set on non-morphed respawn (NAI-20 gate)")
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
	n.levels[objtype.NpcStatHitpoints], n.baseLevels[objtype.NpcStatHitpoints] = 8, 10

	s.processNpcRegen(n)

	if n.regenClock != 1 {
		t.Errorf("regenClock: got %d, want 1", n.regenClock)
	}
	if n.CurHP() != 8 {
		t.Errorf("curHP: got %d, want 8 (no regen fire yet)", n.CurHP())
	}
}

func TestProcessNpcRegenFiresAtInterval(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.regenInterval = 3
	n.regenClock = 0
	n.levels[objtype.NpcStatHitpoints], n.baseLevels[objtype.NpcStatHitpoints] = 5, 10

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
	if n.CurHP() != 5 {
		t.Fatalf("curHP after 2 ticks: got %d, want 5 (pre-fire)", n.CurHP())
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
	if n.CurHP() != 6 {
		t.Errorf("curHP after fire: got %d, want 6 (incremented toward baseHP=10)", n.CurHP())
	}
}

func TestProcessNpcRegenClampsAtBaseHP(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.regenInterval = 3
	n.regenClock = 0
	n.levels[objtype.NpcStatHitpoints], n.baseLevels[objtype.NpcStatHitpoints] = 10, 10

	for i := 0; i < 3; i++ {
		s.processNpcRegen(n)
	}

	if n.CurHP() != 10 {
		t.Errorf("curHP: got %d, want 10 (no change at equal)", n.CurHP())
	}
}

func TestProcessNpcRegenDecrementsWhenAboveBase(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.regenInterval = 3
	n.regenClock = 0
	n.levels[objtype.NpcStatHitpoints], n.baseLevels[objtype.NpcStatHitpoints] = 12, 10

	for i := 0; i < 3; i++ {
		s.processNpcRegen(n)
	}

	if n.CurHP() != 11 {
		t.Errorf("curHP: got %d, want 11 (decremented toward baseHP=10)", n.CurHP())
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
	n.server = newTestServer(t) // NAI-19 Task 5e: heavy path needs server.

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
// Observer count is seeded via s.rsbuf.SetObserverForTest.
func TestProcessNpcHuntPauseHuntBailsWithNoObservers(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t) // nid = 1 per NewNpc's arg
	s.rsbuf.AddNpc(int32(n.nid), 0)
	s.rsbuf.SetObserverForTest(int32(n.nid), 0)
	defer s.rsbuf.SetObserverForTest(int32(n.nid), 0)
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
	s.rsbuf.AddNpc(int32(n.nid), 0)
	s.rsbuf.SetObserverForTest(int32(n.nid), 1)       // seed one observer
	defer s.rsbuf.SetObserverForTest(int32(n.nid), 0) // cleanup
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

// TestProcessNpcEventQueueHappyPathFire guards the fire+remove path
// of processNpcEventQueue at modules/world/npc_event_queue.go:36-48.
// A non-delayed NPC with a queued event runs through runNpcScript →
// resumeOrFinishNpc → (on Finished) ClearActiveScript. Observability:
// queue drained + activeScript cleared.
//
// Closes NAI-5 test-gap #1. Complement to
// TestProcessNpcEventQueueSkipsDelayedNpcs (skip branch) and
// TestNpcTurnEventsDespawnEnqueuesEvent (enqueue-no-fire).
func TestProcessNpcEventQueueHappyPathFire(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	// Pre-seed activeScript so the post-call nil check is a positive
	// witness that ClearActiveScript dispatched (matches the pattern
	// in TestResumeOrFinishNpcErrorPathClearsScript at
	// modules/world/npc_script_test.go:306).
	n.activeScript = &script.ScriptState{}

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

	if len(s.npcEventQueue) != 0 {
		t.Errorf("npcEventQueue: got len %d, want 0 (queue drained after fire)", len(s.npcEventQueue))
	}
	if n.activeScript != nil {
		t.Error("activeScript: got non-nil, want nil (Finished execution should have cleared the pre-seeded state via ClearActiveScript)")
	}
}

// addPlayerToServer seeds s.players[slot], s.grid, and pkg/zone with a
// minimal *Player at the given coords. Used by NAI-8 huntPlayers tests.
// Slot 0 is reserved per existing convention.
//
// Sets p.active=true so PlayersSafe(false).IsValid() passes (post-NAI-28
// huntPlayers reads from Zone subscription, not s.grid).
func addPlayerToServer(t *testing.T, s *Server, slot, x, z, level int) *Player {
	t.Helper()
	if s.grid == nil {
		s.grid = grid.New()
	}
	if s.zoneMap == nil {
		s.zoneMap = zone.NewZoneMap()
	}
	p := &Player{
		slot:   slot,
		x:      x,
		z:      z,
		level:  level,
		active: true,
	}
	s.players[slot] = p
	s.grid.Add(slot, x, z, level)
	zn := s.zoneMap.Get(level, x, z)
	p.zoneListElement = zn.EnterPlayer(p, nil)
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

	hunt := &objtype.HuntType{CheckNotCombat: -1, CheckNotCombatSelf: -1, CheckInv: -1}
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

	hunt := &objtype.HuntType{CheckNotCombat: -1, CheckNotCombatSelf: -1, CheckInv: -1}
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
	huntWithAfk := &objtype.HuntType{CheckAfk: true, CheckNotCombat: -1, CheckNotCombatSelf: -1, CheckInv: -1}
	hunted := n.huntPlayers(s, huntWithAfk)
	if len(hunted) != 1 {
		t.Fatalf("CheckAfk=true: got %d, want 1 (AFK filtered)", len(hunted))
	}
	if hunted[0].Slot() != pActive.slot {
		t.Errorf("CheckAfk=true: got slot %d, want slot %d (active)", hunted[0].Slot(), pActive.slot)
	}

	// With CheckAfk=false, both players returned.
	huntNoAfk := &objtype.HuntType{CheckAfk: false, CheckNotCombat: -1, CheckNotCombatSelf: -1, CheckInv: -1}
	hunted = n.huntPlayers(s, huntNoAfk)
	if len(hunted) != 2 {
		t.Errorf("CheckAfk=false: got %d, want 2 (filter inactive, both returned)", len(hunted))
	}
}

func TestHuntPlayersReturnsEmptyWhenNoCandidates(t *testing.T) {
	s := newServerForScriptTest(t)
	s.zoneMap = zone.NewZoneMap()
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.huntRange = 10

	hunt := &objtype.HuntType{CheckNotCombat: -1, CheckNotCombatSelf: -1, CheckInv: -1}
	hunted := n.huntPlayers(s, hunt)

	if len(hunted) != 0 {
		t.Errorf("hunted: got %d, want 0 (empty zone)", len(hunted))
	}
}

func TestProcessLogoutsDecrementsSubscribedNpcObservers(t *testing.T) {
	s := newServerForScriptTest(t)
	s.rsbuf.AddNpc(101, 0) // cleanup — ensure slots exist
	s.rsbuf.AddNpc(102, 0)

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
	s.rsbuf.SetObserverForTest(101, 1)
	s.rsbuf.SetObserverForTest(102, 1)

	// Trigger logout: set loggingOut flag (force logout regardless of timing).
	p.loggingOut = true
	p.preventLogoutUntil = 0

	s.processLogouts()

	// processLogouts calls rsbuf.RemovePlayer, which operates on the
	// package-level shim (not migrated until T4.5). Verify the decrements
	// through that shim.
	if got := rsbuf.GetNpcObservers(101); got != 0 {
		t.Errorf("GetNpcObservers(101) after logout: got %d, want 0", got)
	}
	if got := rsbuf.GetNpcObservers(102); got != 0 {
		t.Errorf("GetNpcObservers(102) after logout: got %d, want 0", got)
	}
}

// TestAddNpcQueuesSpawnEventOnFirstSpawn pins NAI-22 Bundle 1: addNpc
// queues an NpcEventSpawn entry when a SPAWN script is registered for
// the NPC's typeId/category. Mirrors TS World.ts:1284-1289.
func TestAddNpcQueuesSpawnEventOnFirstSpawn(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	n := newNpcForLifecycleTest(t)
	n.server = s

	spawnScript := &script.ScriptFile{
		Name:      "ai_spawn_global",
		LookupKey: script.LookupKeyForGlobal(script.TriggerAiSpawn),
	}
	s.scriptProvider.Register(spawnScript)

	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}

	if len(s.npcEventQueue) != 1 {
		t.Fatalf("npcEventQueue: got len %d, want 1 (SPAWN script registered, must enqueue)", len(s.npcEventQueue))
	}
	got := s.npcEventQueue[0]
	if got.Type != NpcEventSpawn {
		t.Errorf("npcEventQueue[0].Type: got %d, want NpcEventSpawn (%d)", got.Type, NpcEventSpawn)
	}
	if got.Script != spawnScript {
		t.Errorf("npcEventQueue[0].Script: got %p, want %p", got.Script, spawnScript)
	}
	if got.Npc != n {
		t.Errorf("npcEventQueue[0].Npc: got %p, want %p", got.Npc, n)
	}
}

// TestAddNpcQueuesSpawnEventOnRespawn pins NAI-22 Bundle 1: addNpc with
// firstSpawn=false (revertType heavy path) ALSO queues SPAWN. Matches TS
// World.ts:1258-1294, which has no firstSpawn guard around the queue
// insertion (lines 1284-1289).
func TestAddNpcQueuesSpawnEventOnRespawn(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	n := newNpcForLifecycleTest(t)
	n.server = s
	// Pre-register the NPC at a slot (firstSpawn=true would do this; here
	// we simulate revertType path: NPC keeps its slot, addNpc(firstSpawn=false)).
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("first addNpc setup: %v", err)
	}
	s.npcEventQueue = nil // reset queue from setup; we only want to observe the second call

	spawnScript := &script.ScriptFile{
		Name:      "ai_spawn_global",
		LookupKey: script.LookupKeyForGlobal(script.TriggerAiSpawn),
	}
	s.scriptProvider.Register(spawnScript)

	if err := s.addNpc(n, -1, false); err != nil {
		t.Fatalf("addNpc(firstSpawn=false): %v", err)
	}

	if len(s.npcEventQueue) != 1 {
		t.Fatalf("npcEventQueue: got len %d, want 1 (SPAWN must fire on respawn too)", len(s.npcEventQueue))
	}
}

// TestAddNpcNoSpawnScriptNoQueue pins the script != nil short-circuit:
// when no SPAWN script is registered, addNpc must NOT enqueue.
func TestAddNpcNoSpawnScriptNoQueue(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider() // empty provider
	n := newNpcForLifecycleTest(t)
	n.server = s

	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}

	if len(s.npcEventQueue) != 0 {
		t.Errorf("npcEventQueue: got len %d, want 0 (no SPAWN script registered)", len(s.npcEventQueue))
	}
}

// TestAddNpcNilScriptProviderNoQueue pins the s.scriptProvider != nil
// defensive guard. The DESPAWN producer at npc_ai.go:47 uses the same
// guard; SPAWN must mirror.
func TestAddNpcNilScriptProviderNoQueue(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = nil // explicit
	n := newNpcForLifecycleTest(t)
	n.server = s

	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}

	if len(s.npcEventQueue) != 0 {
		t.Errorf("npcEventQueue: got len %d, want 0 (nil scriptProvider must short-circuit)", len(s.npcEventQueue))
	}
}

// TestProcessNpcEventQueueDispatchesSpawn pins end-to-end SPAWN dispatch:
// addNpc enqueues, processNpcEventQueue drains AND fires the script. The
// type-agnostic processor at npc_event_queue.go:36-48 already handles
// SPAWN identically to DESPAWN; this test pins that contract.
func TestProcessNpcEventQueueDispatchesSpawn(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	n := newNpcForLifecycleTest(t)
	n.server = s

	spawnScript := &script.ScriptFile{
		Name:      "ai_spawn_stub",
		Opcodes:   []script.Opcode{script.OpReturn},
		LookupKey: script.LookupKeyForGlobal(script.TriggerAiSpawn),
	}
	s.scriptProvider.Register(spawnScript)

	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}
	if len(s.npcEventQueue) != 1 {
		t.Fatalf("setup: queue len %d, want 1", len(s.npcEventQueue))
	}

	s.processNpcEventQueue()

	if len(s.npcEventQueue) != 0 {
		t.Errorf("npcEventQueue: got len %d, want 0 (queue must drain after dispatch)", len(s.npcEventQueue))
	}
}
