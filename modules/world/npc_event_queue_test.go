package world

import (
	"testing"

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
