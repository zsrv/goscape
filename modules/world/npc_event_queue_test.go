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
