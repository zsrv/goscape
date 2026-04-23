package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestCheckOpTrigger(t *testing.T) {
	ops := []struct {
		name string
		op   int
		want bool
	}{
		{"OpPlayer1", objtype.NPCModeOpPlayer1, true},
		{"OpPlayer5", objtype.NPCModeOpPlayer5, true},
		{"OpLoc1", objtype.NPCModeOpLoc1, true},
		{"OpLoc5", objtype.NPCModeOpLoc5, true},
		{"OpObj1", objtype.NPCModeOpObj1, true},
		{"OpObj5", objtype.NPCModeOpObj5, true},
		{"OpNpc1", objtype.NPCModeOpNpc1, true},
		{"OpNpc5", objtype.NPCModeOpNpc5, true},
		{"ApPlayer1 — NOT op", objtype.NPCModeApPlayer1, false},
		{"ApNpc5 — NOT op", objtype.NPCModeApNpc5, false},
		{"PatrolMode", objtype.NPCModePatrol, false},
		{"NoneMode", objtype.NPCModeNone, false},
		{"Queue1", objtype.NPCModeQueue1, false},
		{"Null", objtype.NPCModeNull, false},
	}
	for _, tc := range ops {
		t.Run(tc.name, func(t *testing.T) {
			if got := checkOpTrigger(tc.op); got != tc.want {
				t.Errorf("checkOpTrigger(%d) = %t, want %t", tc.op, got, tc.want)
			}
		})
	}
}

func TestNpcResetDefaultsClearsTargetKeepsOtherState(t *testing.T) {
	typ := &objtype.NpcType{WanderRange: 5}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.target = &Npc{nid: 99}
	n.targetOp = objtype.NPCModeOpNpc1
	n.faceEntity = 99
	n.masks = 0xff
	n.apRange = 5
	n.apRangeCalled = true

	n.resetDefaults()

	if n.target != nil {
		t.Error("target: not nil")
	}
	if n.targetOp != objtype.NPCModeWander {
		t.Errorf("targetOp: got %d, want NPCModeWander", n.targetOp)
	}
	// These stay untouched — next SetInteraction call will overwrite.
	if n.faceEntity != 99 {
		t.Errorf("faceEntity: got %d, want 99 (resetDefaults must not clear)", n.faceEntity)
	}
	if n.masks != 0xff {
		t.Errorf("masks: got 0x%x, want 0xff (resetDefaults must not clear)", n.masks)
	}
	if n.apRange != 5 {
		t.Errorf("apRange: got %d, want 5 (resetDefaults must not clear)", n.apRange)
	}
	if !n.apRangeCalled {
		t.Error("apRangeCalled: not preserved")
	}
}

func TestNpcClearInteractionResetsState(t *testing.T) {
	typ := &objtype.NpcType{WanderRange: 5}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.target = &Npc{nid: 99}
	n.targetOp = objtype.NPCModeOpNpc1
	n.apRange = 5
	n.apRangeCalled = true
	n.targetSubject = npcTargetSubject{com: 42, typ: 1}

	n.clearInteraction()

	if n.target != nil {
		t.Error("target: not nil")
	}
	if n.targetOp != -1 {
		t.Errorf("targetOp: got %d, want -1", n.targetOp)
	}
	if n.apRange != 10 {
		t.Errorf("apRange: got %d, want 10 (reset to default)", n.apRange)
	}
	if n.apRangeCalled {
		t.Error("apRangeCalled: got true, want false")
	}
	if n.targetSubject.com != -1 || n.targetSubject.typ != -1 {
		t.Errorf("targetSubject: got %+v, want {-1,-1}", n.targetSubject)
	}
}

func TestNpcDefaultMode(t *testing.T) {
	tests := []struct {
		name string
		typ  *objtype.NpcType
		want int
	}{
		{"patrol config", &objtype.NpcType{PatrolCoord: []uint32{100}}, objtype.NPCModePatrol},
		{"wander config", &objtype.NpcType{WanderRange: 5}, objtype.NPCModeWander},
		{"neither", &objtype.NpcType{}, objtype.NPCModeNone},
		{"both — patrol wins", &objtype.NpcType{PatrolCoord: []uint32{100}, WanderRange: 5}, objtype.NPCModePatrol},
		{"nil typ", nil, objtype.NPCModeNone},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n := &Npc{typ: tc.typ}
			if got := n.defaultMode(); got != tc.want {
				t.Errorf("defaultMode: got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestCheckApTrigger(t *testing.T) {
	ops := []struct {
		name string
		op   int
		want bool
	}{
		{"ApPlayer1", objtype.NPCModeApPlayer1, true},
		{"ApPlayer5", objtype.NPCModeApPlayer5, true},
		{"ApLoc1", objtype.NPCModeApLoc1, true},
		{"ApLoc5", objtype.NPCModeApLoc5, true},
		{"ApObj1", objtype.NPCModeApObj1, true},
		{"ApObj5", objtype.NPCModeApObj5, true},
		{"ApNpc1", objtype.NPCModeApNpc1, true},
		{"ApNpc5", objtype.NPCModeApNpc5, true},
		{"OpPlayer1 — NOT ap", objtype.NPCModeOpPlayer1, false},
		{"OpNpc5 — NOT ap", objtype.NPCModeOpNpc5, false},
		{"PatrolMode", objtype.NPCModePatrol, false},
		{"Queue20", objtype.NPCModeQueue20, false},
	}
	for _, tc := range ops {
		t.Run(tc.name, func(t *testing.T) {
			if got := checkApTrigger(tc.op); got != tc.want {
				t.Errorf("checkApTrigger(%d) = %t, want %t", tc.op, got, tc.want)
			}
		})
	}
}
