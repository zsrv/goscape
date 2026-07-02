package world

import (
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

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
	n.queue = []script.NpcQueueRequest{{Trigger: script.TriggerAiQueue1, Delay: 5, LastInt: 0}}

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

// TestNewNpcHoldsTypRegenRate: 244 has no regenInterval snapshot field;
// the rate is read live from n.typ.RegenRate each processNpcRegen call.
// Verify the NPC holds the type pointer with the expected RegenRate.
func TestNewNpcHoldsTypRegenRate(t *testing.T) {
	typ := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: 0, DebugName: "test_npc"},
		RegenRate:  7,
	}
	n := NewNpc(1, 0, 3094, 3106, 0, typ)

	if n.typ == nil || n.typ.RegenRate != 7 {
		var got int
		if n.typ != nil {
			got = n.typ.RegenRate
		}
		t.Errorf("n.typ.RegenRate: got %d, want 7 (live type rate; no snapshot field in 244)", got)
	}
}

// TestProcessNpcRegenFirstCallProcs: 244 countdown — clock init 0, first call
// pre-decrements to -1 which is <=0, so regen fires immediately (OSRS-accurate
// per TS comment Npc.ts:517-519). regenClock resets to regenrate.
func TestProcessNpcRegenFirstCallProcs(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.typ.RegenRate = 100
	n.regenClock = 0
	n.levels[objtype.NpcStatHitpoints], n.baseLevels[objtype.NpcStatHitpoints] = 8, 10

	s.processNpcRegen(n)

	if n.regenClock != 100 {
		t.Errorf("regenClock: got %d, want 100 (reset to regenrate after first-turn proc)", n.regenClock)
	}
	if n.CurHP() != 9 {
		t.Errorf("curHP: got %d, want 9 (first-turn proc fired, 8→9)", n.CurHP())
	}
}

// TestProcessNpcRegenCountdownCadence: 244 countdown — RegenRate=3 (ticks), clock
// init 0. Call 1: 0-1=-1<=0 → fires (HP 5→6, regenClock=3). Calls 2-3: count
// down 3→2→1 (no fire). Call 4: 1-1=0<=0 → fires (HP 6→7, regenClock=3).
// Rate is read live from n.typ.RegenRate (no snapshot field).
func TestProcessNpcRegenCountdownCadence(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.typ.RegenRate = 3
	n.regenClock = 0
	n.levels[objtype.NpcStatHitpoints], n.baseLevels[objtype.NpcStatHitpoints] = 5, 10

	// Call 1: first-turn proc. HP 5→6, regenClock→3.
	s.processNpcRegen(n)
	if n.regenClock != 3 {
		t.Fatalf("regenClock after call 1: got %d, want 3", n.regenClock)
	}
	if n.CurHP() != 6 {
		t.Fatalf("curHP after call 1: got %d, want 6 (first-turn proc)", n.CurHP())
	}

	// Calls 2-3: countdown 3→2→1; no fire yet.
	s.processNpcRegen(n)
	s.processNpcRegen(n)
	if n.regenClock != 1 {
		t.Fatalf("regenClock after call 3: got %d, want 1", n.regenClock)
	}
	if n.CurHP() != 6 {
		t.Fatalf("curHP after call 3: got %d, want 6 (no mid-stream proc)", n.CurHP())
	}

	// Call 4: countdown 1→0 → proc. HP 6→7, regenClock→3.
	// Rate read live from n.typ.RegenRate (244: no snapshot field).
	s.processNpcRegen(n)
	if n.regenClock != 3 {
		t.Errorf("regenClock after call 4: got %d, want 3 (reset to live regenrate)", n.regenClock)
	}
	if n.CurHP() != 7 {
		t.Errorf("curHP after call 4: got %d, want 7 (second proc)", n.CurHP())
	}
}

func TestProcessNpcRegenClampsAtBaseHP(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 100
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.typ.RegenRate = 3
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
	n.typ.RegenRate = 3
	n.regenClock = 0
	n.levels[objtype.NpcStatHitpoints], n.baseLevels[objtype.NpcStatHitpoints] = 12, 10

	// Call 1: first-turn proc (regenClock 0→-1→0≤0), HP 12→11.
	// Calls 2-3: countdown 3→2→1 (no proc).
	for i := 0; i < 3; i++ {
		s.processNpcRegen(n)
	}

	if n.CurHP() != 11 {
		t.Errorf("curHP: got %d, want 11 (decremented toward baseHP=10)", n.CurHP())
	}
}

// --- 244 regen countdown pins (Npc.ts:514-532 @ 9aadcec4) ---
// These tests encode the 244 contract:
//   - countdown clock (pre-decrement then <= 0 check)
//   - clock init 0 → first-turn-alive proc
//   - regenrate 0 disables entirely (no decrement, no proc)
//   - regenClock resets to regenrate (not 0) after proc
//   - rate read from n.typ.RegenRate each call (no regenInterval snapshot)

// TestProcessNpcRegen244FirstTurnProc: fresh NPC (regenClock=0, RegenRate=100)
// must proc on the very first processNpcRegen call (244 countdown: 0-1=-1 <=0).
// After: levels[HP]=6, regenClock=100.
func TestProcessNpcRegen244FirstTurnProc(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.typ.RegenRate = 100
	n.regenClock = 0
	n.levels[objtype.NpcStatHitpoints] = 5
	n.baseLevels[objtype.NpcStatHitpoints] = 10

	s.processNpcRegen(n)

	if n.CurHP() != 6 {
		t.Errorf("curHP after first call: got %d, want 6 (first-turn proc)", n.CurHP())
	}
	if n.regenClock != 100 {
		t.Errorf("regenClock after first call: got %d, want 100 (reset to regenrate)", n.regenClock)
	}
}

// TestProcessNpcRegen244SteadyStateCadence: after first-turn proc (regenClock=100),
// next proc happens exactly 100 calls later (call 101 total; level goes 5→6 at
// call 1, then 6→7 at call 101). Verifies no premature fire and exact cadence.
func TestProcessNpcRegen244SteadyStateCadence(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.typ.RegenRate = 100
	n.regenClock = 0
	n.levels[objtype.NpcStatHitpoints] = 5
	n.baseLevels[objtype.NpcStatHitpoints] = 10

	// Call 1: first-turn proc → level 5→6, regenClock reset to 100.
	s.processNpcRegen(n)
	if n.CurHP() != 6 {
		t.Fatalf("call 1: curHP got %d, want 6", n.CurHP())
	}
	if n.regenClock != 100 {
		t.Fatalf("call 1: regenClock got %d, want 100", n.regenClock)
	}

	// Calls 2..100: countdown ticks 99 down to 1; no proc yet.
	for i := 2; i <= 100; i++ {
		s.processNpcRegen(n)
	}
	if n.CurHP() != 6 {
		t.Fatalf("after call 100: curHP got %d, want 6 (no mid-stream proc)", n.CurHP())
	}
	if n.regenClock != 1 {
		t.Fatalf("after call 100: regenClock got %d, want 1", n.regenClock)
	}

	// Call 101: countdown 1-1=0 <=0 → proc; level 6→7, regenClock reset to 100.
	s.processNpcRegen(n)
	if n.CurHP() != 7 {
		t.Errorf("call 101: curHP got %d, want 7 (second proc)", n.CurHP())
	}
	if n.regenClock != 100 {
		t.Errorf("call 101: regenClock got %d, want 100", n.regenClock)
	}
}

// TestProcessNpcRegen244RegenRateZeroDisables: RegenRate=0 must disable regen
// entirely — the clock must not underflow, the level must stay damaged, even
// after many calls. (225 countup logic: ++regenClock >= regenInterval=0 was always true, so regen fired EVERY tick instead of being disabled; 244 short-circuits on regenrate==0 before any decrement.)
func TestProcessNpcRegen244RegenRateZeroDisables(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.typ.RegenRate = 0
	n.regenClock = 0
	n.levels[objtype.NpcStatHitpoints] = 5
	n.baseLevels[objtype.NpcStatHitpoints] = 10

	for i := 0; i < 150; i++ {
		s.processNpcRegen(n)
	}

	if n.CurHP() != 5 {
		t.Errorf("curHP after 150 calls (regenrate=0): got %d, want 5 (regen disabled)", n.CurHP())
	}
	// Clock must remain 0 (no decrement applied when regenrate=0).
	if n.regenClock != 0 {
		t.Errorf("regenClock after 150 calls (regenrate=0): got %d, want 0 (no decrement)", n.regenClock)
	}
}

// TestProcessNpcRegen244LiveTypeRate: regenClock resets to n.typ.RegenRate as
// read at proc time, with no snapshot field. After a type change (n.typ.RegenRate
// mutated), the NEXT proc adopts the new rate automatically.
func TestProcessNpcRegen244LiveTypeRate(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.typ.RegenRate = 5
	n.regenClock = 0
	n.levels[objtype.NpcStatHitpoints] = 5
	n.baseLevels[objtype.NpcStatHitpoints] = 10

	// First-turn proc → regenClock=5.
	s.processNpcRegen(n)
	if n.regenClock != 5 {
		t.Fatalf("after first proc: regenClock got %d, want 5", n.regenClock)
	}

	// Change rate live (simulates changeType).
	n.typ.RegenRate = 20

	// Count down 5 ticks to next proc: regenClock goes 5→4→3→2→1→0.
	// The 5th call (regenClock 1→0) fires: HP 6→7, regenClock resets to 20.
	for i := 0; i < 5; i++ {
		s.processNpcRegen(n)
	}

	if n.regenClock != 20 {
		t.Errorf("after rate-change proc: regenClock got %d, want 20 (live type rate)", n.regenClock)
	}
	if n.CurHP() != 7 {
		t.Errorf("after rate-change proc: curHP got %d, want 7 (6 from first proc + 1 from second proc)", n.CurHP())
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
// resumeOrFinishNpc → (on Finished) OnScriptFinishedOrAborted.
// Observability: queue drained + unrelated pre-seeded activeScript
// preserved (NAI-54 guard: Npc.ts:226 `if (script === this.activeScript)`).
//
// Closes NAI-5 test-gap #1. Complement to
// TestProcessNpcEventQueueSkipsDelayedNpcs (skip branch) and
// TestNpcTurnEventsDespawnEnqueuesEvent (enqueue-no-fire).
func TestProcessNpcEventQueueHappyPathFire(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForLifecycleTest(t)
	n.server = s
	// Pre-seed activeScript with an UNRELATED state. Under NAI-54's
	// TS-faithful Finished/Aborted tail (Npc.ts:226 `if (script ===
	// this.activeScript)`), the executing event-queue script is a
	// different ScriptState instance, so the pre-seeded activeScript
	// must be PRESERVED, not nilled. Observability of the fire path
	// remains via the queue-drained assertion below.
	stored := &script.ScriptState{}
	n.activeScript = stored

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
	if n.activeScript != stored {
		t.Errorf("activeScript: got %p, want preserved %p (NAI-54 guard: fresh event-queue script finishing must not null unrelated stored state)",
			n.activeScript, stored)
	}
}

// addPlayerToServer seeds s.players[slot] and pkg/zone with a minimal
// *Player at the given coords. Used by NAI-8 huntPlayers tests.
// Slot 0 is reserved per existing convention.
//
// Sets p.active=true so PlayersSafe(false).IsValid() passes
// (huntPlayers reads from Zone subscription post-NAI-28).
func addPlayerToServer(t *testing.T, s *Server, slot, x, z, level int) *Player {
	t.Helper()
	if s.zoneMap == nil {
		s.zoneMap = zone.NewZoneMap()
	}
	p := &Player{
		pid:    slot,
		x:      x,
		z:      z,
		level:  level,
		active: true,
	}
	s.players.set(slot, p)
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
	if hunted[0].Slot() != pInRange.pid {
		t.Errorf("hunted[0]: got slot %d, want slot %d", hunted[0].Slot(), pInRange.pid)
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
	if hunted[0].Slot() != pSameLevel.pid {
		t.Errorf("hunted[0]: got slot %d, want slot %d", hunted[0].Slot(), pSameLevel.pid)
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
	if hunted[0].Slot() != pActive.pid {
		t.Errorf("CheckAfk=true: got slot %d, want slot %d (active)", hunted[0].Slot(), pActive.pid)
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
	// dropConnRef, not a raw c.in.Release(): this test drives the player
	// through processLogouts -> removePlayerOnTick, which calls
	// dropTickRef and may already have pool-returned the buffers. A second
	// unconditional Release here would double-return the same pooled
	// object (arch-28.4b).
	t.Cleanup(func() { c.dropConnRef() })
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

	// Subscribe the player's BuildArea to two NPCs and set observer counts to 1.
	s.rsbuf.SubscribeNpcForTest(int32(p.pid), 101)
	s.rsbuf.SubscribeNpcForTest(int32(p.pid), 102)
	s.rsbuf.SetObserverForTest(101, 1)
	s.rsbuf.SetObserverForTest(102, 1)

	// Trigger logout: set loggingOut flag (force logout regardless of timing).
	p.loggingOut = true
	p.preventLogoutUntil = 0

	s.processLogouts()

	// processLogouts -> removePlayerOnTick -> removePlayerInternal -> s.rsbuf.RemovePlayer iterates
	// player.Build.Npcs and decrements per-Buf observer counts. Verify
	// via the per-Buf accessor (NAI-30 Bundle 4 retired the package-level
	// rsbuf.GetNpcObservers shim consumer here).
	if got := s.rsbuf.GetNpcObservers(101); got != 0 {
		t.Errorf("GetNpcObservers(101) after logout: got %d, want 0", got)
	}
	if got := s.rsbuf.GetNpcObservers(102); got != 0 {
		t.Errorf("GetNpcObservers(102) after logout: got %d, want 0", got)
	}
}

// newLoggingOutPlayer builds a minimal in-game player attached to s with a
// live (discarded) client connection and the loggingOut flag set, ready for
// processLogouts. Test servers have nil login/friends RPC clients, so removal
// performs no network I/O.
func newLoggingOutPlayer(t *testing.T, s *Server) *Player {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() { serverConn.Close(); clientConn.Close() })
	c := newClient(serverConn, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	// dropConnRef: see newTestClient's comment (server_test.go) — this
	// helper's callers all drive processLogouts -> removePlayerOnTick,
	// which may already have released the buffers via dropTickRef.
	t.Cleanup(func() { c.dropConnRef() })
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
	p.loggingOut = true
	p.preventLogoutUntil = 0
	return p
}

func playerInLoop(s *Server, p *Player) bool {
	for lp := range s.players.all() {
		if lp == p {
			return true
		}
	}
	return false
}

// TestProcessLogouts_NonDiscardableQueueBlocks pins H5: a logging-out player
// whose primary queue holds a non-discardable entry (anything other than a
// LONG marked logoutAction==1) is NOT removed this tick. TS World.ts:776-788.
func TestProcessLogouts_NonDiscardableQueueBlocks(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 1
	p := newLoggingOutPlayer(t, s)
	p.queue = append(p.queue, playerQueueRequest{Type: script.QueueNormal})

	s.processLogouts()

	if !playerInLoop(s, p) {
		t.Error("player removed despite a non-discardable queue entry (H5 gate)")
	}
}

// TestProcessLogouts_DiscardableQueueRemoves pins that a fully discardable
// queue (LONG with logoutAction==1) does not block logout.
func TestProcessLogouts_DiscardableQueueRemoves(t *testing.T) {
	s := newServerForScriptTest(t)
	s.currentTick = 1
	p := newLoggingOutPlayer(t, s)
	p.queue = append(p.queue, playerQueueRequest{Type: script.QueueLong, IntArgs: []int{1}})

	s.processLogouts()

	if playerInLoop(s, p) {
		t.Error("player not removed with a discardable LONG-discard queue (H5 gate)")
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
