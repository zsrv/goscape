package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
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

func TestNpcSetInteraction(t *testing.T) {
	typ := &objtype.NpcType{}
	targetNpc := &Npc{nid: 7, typeId: 99, x: 105, z: 105, level: 0}
	targetPlayer := &Player{
		slot:       3,
		active:     true,
		visibility: rsbuf.VisibilityDefault,
		client:     &client{},
		x:          105, z: 105, level: 0,
	}
	targetLoc := entitypkg.NewLoc(0, 105, 105, 1, 1, entitypkg.LifecycleRespawn, 42, 10, 0)
	targetObj := entitypkg.NewObj(0, 105, 105, entitypkg.LifecycleRespawn, 88, 1)

	type row struct {
		name       string
		target     entity
		kind       InteractionKind
		op         int
		com        int
		wantOK     bool
		wantFace   int // faceEntity; -1 if not applicable
		wantTX     int // targetX; -1 if not applicable
		wantTZ     int
		wantSubCom int
		wantSubTyp int
	}

	rows := []row{
		{
			name: "Player target", target: targetPlayer, kind: InteractionScript,
			op: objtype.NPCModeOpPlayer1, com: -1, wantOK: true,
			wantFace: 3 + 32768, wantTX: -1, wantTZ: -1,
			wantSubCom: -1, wantSubTyp: -1,
		},
		{
			name: "Npc target", target: targetNpc, kind: InteractionScript,
			op: objtype.NPCModeOpNpc1, com: -1, wantOK: true,
			wantFace: 7, wantTX: -1, wantTZ: -1,
			wantSubCom: -1, wantSubTyp: 99,
		},
		{
			name: "Loc target", target: targetLoc, kind: InteractionEngine,
			op: objtype.NPCModeOpLoc1, com: 5, wantOK: true,
			wantFace:   -1,
			wantTX:     coordgrid.Fine(105, 1),
			wantTZ:     coordgrid.Fine(105, 1),
			wantSubCom: 5, wantSubTyp: 42,
		},
		{
			name: "Obj target", target: targetObj, kind: InteractionEngine,
			op: objtype.NPCModeOpObj1, com: -1, wantOK: true,
			wantFace:   -1,
			wantTX:     coordgrid.Fine(105, 1),
			wantTZ:     coordgrid.Fine(105, 1),
			wantSubCom: -1, wantSubTyp: 88,
		},
		{
			name: "com==0 → subject.com==-1 (TS quirk)", target: targetNpc, kind: InteractionScript,
			op: objtype.NPCModeOpNpc1, com: 0, wantOK: true,
			wantFace: 7, wantTX: -1, wantTZ: -1,
			wantSubCom: -1, wantSubTyp: 99,
		},
	}

	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			n := NewNpc(1, 42, 100, 100, 0, typ)
			ok := n.SetInteraction(r.kind, r.target, r.op, r.com)
			if ok != r.wantOK {
				t.Errorf("return: got %t, want %t", ok, r.wantOK)
			}
			if n.target != r.target {
				t.Error("target not set")
			}
			if n.targetOp != r.op {
				t.Errorf("targetOp: got %d, want %d", n.targetOp, r.op)
			}
			if n.apRange != 10 {
				t.Errorf("apRange: got %d, want 10", n.apRange)
			}
			if n.apRangeCalled {
				t.Error("apRangeCalled: got true, want false")
			}
			if n.targetSubject.com != r.wantSubCom {
				t.Errorf("subject.com: got %d, want %d", n.targetSubject.com, r.wantSubCom)
			}
			if n.targetSubject.typ != r.wantSubTyp {
				t.Errorf("subject.typ: got %d, want %d", n.targetSubject.typ, r.wantSubTyp)
			}
			if r.wantFace != -1 && n.faceEntity != r.wantFace {
				t.Errorf("faceEntity: got %d, want %d", n.faceEntity, r.wantFace)
			}
			if r.wantTX != -1 && n.targetX != r.wantTX {
				t.Errorf("targetX: got %d, want %d", n.targetX, r.wantTX)
			}
			if r.wantTZ != -1 && n.targetZ != r.wantTZ {
				t.Errorf("targetZ: got %d, want %d", n.targetZ, r.wantTZ)
			}
		})
	}
}

func TestNpcSetInteractionTargetInvalidReturnsFalse(t *testing.T) {
	typ := &objtype.NpcType{}
	n := NewNpc(1, 42, 100, 100, 0, typ)

	deadNpc := &Npc{nid: 7, typeId: 99, dead: true}
	originalTarget := n.target

	ok := n.SetInteraction(InteractionScript, deadNpc, objtype.NPCModeOpNpc1, -1)

	if ok {
		t.Error("return: got true, want false")
	}
	if n.target != originalTarget {
		t.Error("target changed despite IsValid()==false")
	}
	if n.targetOp != n.defaultMode() {
		t.Errorf("targetOp changed: got %d, want %d", n.targetOp, n.defaultMode())
	}
}

func TestNpcFocusSetsFaceAngleCoords(t *testing.T) {
	typ := &objtype.NpcType{}
	n := NewNpc(1, 42, 100, 100, 0, typ)

	n.focus(6431, 6431, false)
	if n.faceAngleX != 6431 {
		t.Errorf("faceAngleX: got %d, want 6431", n.faceAngleX)
	}
	if n.faceAngleZ != 6431 {
		t.Errorf("faceAngleZ: got %d, want 6431", n.faceAngleZ)
	}

	// instant flag is write-only (per the DEVIATION note) — test merely
	// confirms no panic and that coords still update on a subsequent call.
	n.focus(1000, 2000, true)
	if n.faceAngleX != 1000 || n.faceAngleZ != 2000 {
		t.Errorf("focus(instant=true) did not update coords: got (%d,%d)", n.faceAngleX, n.faceAngleZ)
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
