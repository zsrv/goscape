package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
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

func TestProcessMovementInteractionDelayedBails(t *testing.T) {
	s := newServerForScriptTest(t)
	typ := &objtype.NpcType{WanderRange: 5}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.server = s
	n.delayed = true

	n.processMovementInteraction(s)

	if n.x != 100 {
		t.Error("delayed bail: npc moved")
	}
}

func TestProcessMovementInteractionDeadBails(t *testing.T) {
	s := newServerForScriptTest(t)
	typ := &objtype.NpcType{WanderRange: 5}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.server = s
	n.dead = true

	n.processMovementInteraction(s)

	if n.x != 100 {
		t.Error("dead bail: npc moved")
	}
}

func TestProcessMovementInteractionNullFailsafeFallsToDefault(t *testing.T) {
	s := newServerForScriptTest(t)
	typ := &objtype.NpcType{WanderRange: 5}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.server = s
	n.targetOp = objtype.NPCModeNull

	n.processMovementInteraction(s)

	if n.targetOp != objtype.NPCModeWander {
		t.Errorf("Null failsafe: targetOp %d, want NPCModeWander", n.targetOp)
	}
}

func TestProcessMovementInteractionWanderInvokesWanderMode(t *testing.T) {
	s := newServerForScriptTest(t)
	typ := &objtype.NpcType{WanderRange: 5}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.server = s
	n.targetOp = objtype.NPCModeWander

	before := n.wanderCounter
	n.processMovementInteraction(s)
	if n.wanderCounter != before+1 {
		t.Errorf("wanderCounter: got %d, want %d", n.wanderCounter, before+1)
	}
}

func TestProcessMovementInteractionPlayerModesResetToDefault(t *testing.T) {
	s := newServerForScriptTest(t)
	typ := &objtype.NpcType{WanderRange: 5, MaxRange: 50}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.server = s

	target := &Npc{nid: 7, typeId: 99, x: 101, z: 100, level: 0}
	n.target = target
	n.targetOp = objtype.NPCModePlayerFollow
	n.targetSubject.typ = target.typeId

	n.processMovementInteraction(s)

	if n.targetOp != objtype.NPCModeWander {
		t.Errorf("PLAYER* mode: targetOp=%d, want NPCModeWander (resetDefaults)", n.targetOp)
	}
	if n.target != nil {
		t.Error("PLAYER* mode: target not cleared")
	}
}

func TestProcessMovementInteractionNilTargetResetsDefaults(t *testing.T) {
	s := newServerForScriptTest(t)
	typ := &objtype.NpcType{WanderRange: 5}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.server = s
	n.target = nil
	n.targetOp = objtype.NPCModeOpNpc1

	n.processMovementInteraction(s)

	if n.targetOp != objtype.NPCModeWander {
		t.Errorf("nil target with targeted mode: targetOp=%d, want NPCModeWander", n.targetOp)
	}
}

func TestNpcAiModeFiresOpBeforeMoveWhenInRange(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiOpNpc1, 99, "aimode-op"))

	n := newNpcAt100(t)
	n.server = s
	n.typ = &objtype.NpcType{AttackRange: 5, GiveChase: true}
	n.targetOp = objtype.NPCModeOpNpc1

	target := newNpcWithType(99, 0)
	target.x, target.z, target.level = 101, 100, 0
	n.target = target

	n.aiMode(s)

	if string(n.sayText) != "aimode-op" {
		t.Errorf("sayText: got %q, want %q (expected OP fire in contact range)", n.sayText, "aimode-op")
	}
}

func TestNpcAiModeGivechaseFalseClearsTargetAfterMove(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()

	n := newNpcAt100(t)
	n.server = s
	n.moveSpeed = MoveSpeedWalk
	n.typ = &objtype.NpcType{AttackRange: 5, GiveChase: false}
	n.targetOp = objtype.NPCModeOpNpc1

	target := newNpcWithType(99, 0)
	target.x, target.z, target.level = 110, 100, 0 // far — not in range pre-move
	n.target = target

	n.aiMode(s)

	if n.target != nil {
		t.Error("givechase=false + moved: target not cleared (resetDefaults should have run)")
	}
}

func TestNpcAiModeGivechaseTrueKeepsTarget(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()

	n := newNpcAt100(t)
	n.server = s
	n.moveSpeed = MoveSpeedWalk
	n.typ = &objtype.NpcType{AttackRange: 5, GiveChase: true}
	n.targetOp = objtype.NPCModeOpNpc1

	target := newNpcWithType(99, 0)
	target.x, target.z, target.level = 110, 100, 0
	n.target = target

	n.aiMode(s)

	if n.target == nil {
		t.Error("givechase=true + moved: target cleared (should persist)")
	}
}

func TestNpcNoModeCallsUpdateMovement(t *testing.T) {
	s := newServerForScriptTest(t)
	typ := &objtype.NpcType{}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.server = s
	n.moveSpeed = MoveSpeedWalk
	n.waypoints[0] = coordgrid.PackCoord(0, 103, 100)
	n.waypointIndex = 0

	n.noMode(s)

	if n.x != 101 {
		t.Errorf("noMode did not advance: x=%d, want 101", n.x)
	}
}

// newNpcAt100 builds a test NPC positioned at (100,100,0) — convenient
// for tryInteract range tests where the target sits at 101/103/etc.
func newNpcAt100(t *testing.T) *Npc {
	t.Helper()
	n := newNpcForScriptTest(t)
	n.x, n.z, n.level = 100, 100, 0
	n.startX, n.startZ, n.startLevel = 100, 100, 0
	return n
}

func TestNpcTryInteractOpBranchPlayer(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiOpPlayer1, 0, "op-fired"))

	n := newNpcAt100(t)
	n.server = s
	n.typ = &objtype.NpcType{AttackRange: 5}
	n.targetOp = objtype.NPCModeOpPlayer1

	p := newActivePlayer(3)
	p.x, p.z, p.level = 101, 100, 0 // adjacent
	n.target = p

	if !n.tryInteract(s, true) {
		t.Error("tryInteract: false, want true (OP in contact range)")
	}
	if string(n.sayText) != "op-fired" {
		t.Errorf("sayText: got %q, want %q", n.sayText, "op-fired")
	}
}

func TestNpcTryInteractApBranchPlayer(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiApPlayer1, 0, "ap-fired"))

	n := newNpcAt100(t)
	n.server = s
	n.typ = &objtype.NpcType{AttackRange: 5}
	n.targetOp = objtype.NPCModeApPlayer1

	p := newActivePlayer(3)
	p.x, p.z, p.level = 103, 100, 0 // AP range (not contact)
	n.target = p

	if !n.tryInteract(s, false) {
		t.Error("tryInteract: false, want true (AP in approach range)")
	}
	if string(n.sayText) != "ap-fired" {
		t.Errorf("sayText: got %q, want %q", n.sayText, "ap-fired")
	}
}

func TestNpcTryInteractOutOfRange(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiOpPlayer1, 0, "op-fired"))

	n := newNpcAt100(t)
	n.server = s
	n.typ = &objtype.NpcType{AttackRange: 5}
	n.targetOp = objtype.NPCModeOpPlayer1

	p := newActivePlayer(3)
	p.x, p.z, p.level = 200, 100, 0 // far out
	n.target = p

	if n.tryInteract(s, true) {
		t.Error("tryInteract: true, want false (target out of range)")
	}
	if string(n.sayText) == "op-fired" {
		t.Error("sayText: script ran despite out-of-range target")
	}
}

func TestNpcTryInteractOpLocRequiresAllowOpScenery(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiOpLoc1, 77, "loc-fired"))

	n := newNpcAt100(t)
	n.server = s
	n.typ = &objtype.NpcType{AttackRange: 5}
	n.targetOp = objtype.NPCModeOpLoc1

	loc := addLocToZone(t, s, 0, 101, 100, 77, 0)
	n.target = loc
	n.targetSubject.typ = loc.Type()

	// allowOpScenery=false — short-circuit, no fire.
	if n.tryInteract(s, false) {
		t.Error("Loc OP fired with allowOpScenery=false")
	}
	if string(n.sayText) == "loc-fired" {
		t.Error("Loc script ran despite allowOpScenery=false")
	}

	// allowOpScenery=true — should fire.
	if !n.tryInteract(s, true) {
		t.Error("Loc OP did not fire with allowOpScenery=true")
	}
	if string(n.sayText) != "loc-fired" {
		t.Errorf("sayText: got %q, want %q", n.sayText, "loc-fired")
	}
}

func TestNpcUpdateMovementWalk(t *testing.T) {
	s := newServerForScriptTest(t)
	typ := &objtype.NpcType{}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.server = s
	n.moveSpeed = MoveSpeedWalk
	n.waypoints[0] = coordgrid.PackCoord(0, 103, 100)
	n.waypointIndex = 0

	moved := n.updateMovement(s)

	if !moved {
		t.Error("moved: false, want true")
	}
	if n.x != 101 {
		t.Errorf("x: got %d, want 101 (one step east)", n.x)
	}
	if n.walkDir < 0 {
		t.Errorf("walkDir: got %d, want set", n.walkDir)
	}
	if n.runDir != -1 {
		t.Errorf("runDir: got %d, want -1 (walk mode)", n.runDir)
	}
}

func TestNpcUpdateMovementRunConsumesTwoSteps(t *testing.T) {
	s := newServerForScriptTest(t)
	typ := &objtype.NpcType{}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.server = s
	n.moveSpeed = MoveSpeedRun
	n.waypoints[0] = coordgrid.PackCoord(0, 105, 100)
	n.waypointIndex = 0

	moved := n.updateMovement(s)

	if !moved {
		t.Error("moved: false, want true")
	}
	if n.x != 102 {
		t.Errorf("x: got %d, want 102 (two steps east in run mode)", n.x)
	}
	if n.walkDir < 0 {
		t.Error("walkDir: not set")
	}
	if n.runDir < 0 {
		t.Error("runDir: not set (run mode with multi-step waypoint)")
	}
}

func TestNpcUpdateMovementRunWithOneWaypointStep(t *testing.T) {
	s := newServerForScriptTest(t)
	typ := &objtype.NpcType{}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.server = s
	n.moveSpeed = MoveSpeedRun
	n.waypoints[0] = coordgrid.PackCoord(0, 101, 100) // arrives after 1 step
	n.waypointIndex = 0

	moved := n.updateMovement(s)

	if !moved {
		t.Error("moved: false, want true")
	}
	if n.x != 101 {
		t.Errorf("x: got %d, want 101", n.x)
	}
	if n.runDir != -1 {
		t.Errorf("runDir: got %d, want -1 (no second step available)", n.runDir)
	}
}

func TestNpcUpdateMovementNoMoveRestrict(t *testing.T) {
	s := newServerForScriptTest(t)
	typ := &objtype.NpcType{}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.server = s
	n.moveRestrict = MoveRestrictNoMove
	n.waypoints[0] = coordgrid.PackCoord(0, 105, 100)
	n.waypointIndex = 0

	moved := n.updateMovement(s)

	if moved {
		t.Error("moved: true, want false (NoMove restrict)")
	}
	if n.x != 100 {
		t.Errorf("x: got %d, want 100 (no step)", n.x)
	}
	if n.walkDir != -1 || n.runDir != -1 {
		t.Errorf("dirs: walkDir=%d runDir=%d, want both -1", n.walkDir, n.runDir)
	}
}

func TestNpcPathToTarget(t *testing.T) {
	typ := &objtype.NpcType{}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.target = &Npc{x: 105, z: 108, level: 0}

	n.pathToTarget()

	if n.waypointIndex < 0 {
		t.Fatal("waypointIndex: got < 0, want >= 0 after path set")
	}
	got := coordgrid.UnpackCoord(n.waypoints[n.waypointIndex])
	if got.X != 105 || got.Z != 108 {
		t.Errorf("waypoint: got (%d,%d), want (105,108)", got.X, got.Z)
	}
}

func TestNpcPathToTargetNilTargetNoOp(t *testing.T) {
	typ := &objtype.NpcType{}
	n := NewNpc(1, 42, 100, 100, 0, typ)
	n.target = nil

	n.pathToTarget()

	if n.waypointIndex != -1 {
		t.Errorf("waypointIndex: got %d, want -1 (no-op)", n.waypointIndex)
	}
}

func TestNpcValidateTarget(t *testing.T) {
	typ := &objtype.NpcType{MaxRange: 10, AttackRange: 2}

	t.Run("different level", func(t *testing.T) {
		n := NewNpc(1, 42, 100, 100, 0, typ)
		n.targetOp = objtype.NPCModeOpNpc1
		n.target = &Npc{x: 101, z: 100, level: 1}
		n.targetSubject.typ = n.target.(*Npc).typeId
		if n.validateTarget() {
			t.Error("different level: got true, want false")
		}
	})

	t.Run("out of maxrange", func(t *testing.T) {
		n := NewNpc(1, 42, 100, 100, 0, typ)
		n.targetOp = objtype.NPCModeOpNpc1
		n.target = &Npc{x: 200, z: 100, level: 0}
		n.targetSubject.typ = n.target.(*Npc).typeId
		if n.validateTarget() {
			t.Error("far target: got true, want false")
		}
	})

	t.Run("npc typeId changed mid-interaction", func(t *testing.T) {
		n := NewNpc(1, 42, 100, 100, 0, typ)
		n.targetOp = objtype.NPCModeOpNpc1
		target := &Npc{nid: 7, typeId: 99, x: 105, z: 100, level: 0}
		n.target = target
		n.targetSubject.typ = 99

		target.typeId = 100 // simulate changetype

		if n.validateTarget() {
			t.Error("changetyped target: got true, want false")
		}
	})

	t.Run("dead npc target", func(t *testing.T) {
		n := NewNpc(1, 42, 100, 100, 0, typ)
		n.targetOp = objtype.NPCModeOpNpc1
		target := &Npc{nid: 7, typeId: 99, x: 105, z: 100, level: 0, dead: true}
		n.target = target
		n.targetSubject.typ = 99
		if n.validateTarget() {
			t.Error("dead target: got true, want false")
		}
	})

	t.Run("delayed npc target", func(t *testing.T) {
		n := NewNpc(1, 42, 100, 100, 0, typ)
		n.targetOp = objtype.NPCModeOpNpc1
		target := &Npc{nid: 7, typeId: 99, x: 105, z: 100, level: 0, delayed: true}
		n.target = target
		n.targetSubject.typ = 99
		if n.validateTarget() {
			t.Error("delayed target: got true, want false (TS isActive = !dead && !delayed)")
		}
	})

	t.Run("valid npc target", func(t *testing.T) {
		n := NewNpc(1, 42, 100, 100, 0, typ)
		n.targetOp = objtype.NPCModeOpNpc1
		target := &Npc{nid: 7, typeId: 99, x: 105, z: 100, level: 0}
		n.target = target
		n.targetSubject.typ = 99
		if !n.validateTarget() {
			t.Error("valid target: got false, want true")
		}
	})
}

func TestNpcTargetWithinMaxRange(t *testing.T) {
	typ := &objtype.NpcType{MaxRange: 5, AttackRange: 2}

	tests := []struct {
		name     string
		targetOp int
		tx, tz   int
		want     bool
	}{
		// OP branch (maxrange+1=6, with corner-removal)
		{"OP within +1", objtype.NPCModeOpNpc1, 106, 100, true},
		{"OP at +2", objtype.NPCModeOpNpc1, 107, 100, false},
		{"OP corner at (+1,+1)", objtype.NPCModeOpNpc1, 106, 106, false},
		{"OP non-corner edge (+1,0)", objtype.NPCModeOpNpc1, 106, 100, true},

		// AP branch (maxrange + attackrange = 7)
		{"AP at +7", objtype.NPCModeApNpc1, 107, 100, true},
		{"AP at +8", objtype.NPCModeApNpc1, 108, 100, false},

		// Default branch (targetless targeted mode — maxrange+1).
		// Uses PLAYERFACE (not PLAYERFOLLOW) because PLAYERFOLLOW now
		// short-circuits to always-true per TS Npc.ts:633-635 (NAI-13 Task 2).
		{"Default at +6", objtype.NPCModePlayerFace, 106, 100, true},
		{"Default at +7", objtype.NPCModePlayerFace, 107, 100, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n := NewNpc(1, 42, 100, 100, 0, typ)
			n.targetOp = tc.targetOp
			n.target = &Npc{x: tc.tx, z: tc.tz, level: 0}
			if got := n.targetWithinMaxRange(); got != tc.want {
				t.Errorf("got %t, want %t", got, tc.want)
			}
		})
	}

	t.Run("nil target returns true", func(t *testing.T) {
		n := NewNpc(1, 42, 100, 100, 0, typ)
		n.target = nil
		if !n.targetWithinMaxRange() {
			t.Error("nil target: got false, want true")
		}
	})
}

func TestNpcInOperableDistance(t *testing.T) {
	typ := &objtype.NpcType{}
	n := NewNpc(1, 42, 100, 100, 0, typ)

	tests := []struct {
		name   string
		tx, tz int
		want   bool
	}{
		{"same tile", 100, 100, false},
		{"adjacent N", 100, 101, true},
		{"adjacent NE (diagonal)", 101, 101, true},
		{"two tiles away", 102, 100, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			target := &Npc{x: tc.tx, z: tc.tz, level: 0}
			if got := n.inOperableDistance(target); got != tc.want {
				t.Errorf("got %t, want %t", got, tc.want)
			}
		})
	}

	t.Run("different level", func(t *testing.T) {
		target := &Npc{x: 101, z: 100, level: 1}
		if n.inOperableDistance(target) {
			t.Error("different level should return false")
		}
	})
}

func TestNpcInApproachDistance(t *testing.T) {
	typ := &objtype.NpcType{}
	n := NewNpc(1, 42, 100, 100, 0, typ)

	tests := []struct {
		name   string
		rng    int
		tx, tz int
		want   bool
	}{
		{"range 5, at 103", 5, 103, 100, true},
		{"range 5, at 106", 5, 106, 100, false},
		{"range 0 — always false", 0, 101, 100, false},
		{"same tile — false", 5, 100, 100, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			target := &Npc{x: tc.tx, z: tc.tz, level: 0}
			if got := n.inApproachDistance(tc.rng, target); got != tc.want {
				t.Errorf("got %t, want %t", got, tc.want)
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

// approachDistanceFixture builds a *Server + *Npc + target *Player
// positioned 2 tiles apart on level 0, ready to exercise
// inApproachDistance. NPC at (3094, 3106); target player at (3094, 3108).
// s.gamemap is wired. n.server is set. Returns everything the caller
// needs. The target Player is registered via addPlayerToServer which
// also indexes into s.grid; grid registration is harmless for
// inApproachDistance (only target.Coords() and target.level are read).
func approachDistanceFixture(t *testing.T) (*Server, *Npc, *Player) {
	t.Helper()
	s := newServerForScriptTest(t)
	s.gamemap = gamemap.New(discardLogger())
	n := newNpcForLifecycleTest(t)
	n.server = s
	n.x, n.z, n.level = 3094, 3106, 0
	p := addPlayerToServer(t, s, 1, 3094, 3108, 0)
	return s, n, p
}

func TestNpcInApproachDistanceLosPasses(t *testing.T) {
	s, n, p := approachDistanceFixture(t)
	// Seed entity (implicit via fixture), then allocate so empty tiles
	// read FlagOpen instead of FlagNull.
	s.gamemap.Pathfinder.Flags.AllocateIfAbsent(n.x, n.z, n.level)

	if !n.inApproachDistance(5, p) {
		t.Error("inApproachDistance: got false, want true (range ok + LoS clear)")
	}
}

func TestNpcInApproachDistanceLosBlocks(t *testing.T) {
	s, n, p := approachDistanceFixture(t)
	withBlockingWall(t, s, 3094, 3107, 0) // mid-tile blocker

	if n.inApproachDistance(5, p) {
		t.Error("inApproachDistance: got true, want false (LoS blocked by mid-tile)")
	}
}

// TestNpcInApproachDistanceNpcBackwardArgsQuirk guards the TS
// target-as-source + self-as-dest ordering at PathingEntity.ts:402-405.
// Uses an asymmetric directional wall flag that blocks target->NPC
// direction but would pass NPC->target.
//
// Fixture rationale (target north of NPC at +2 z):
//
//	Target->NPC direction: travelSouth — ray checks FlagWallNorth-bit
//	when entering each new tile. FlagWallNorthProjBlocker at mid-tile
//	(3094, 3107) blocks this direction.
//	NPC->target (un-swap): travelNorth — checks FlagWallSouth-bit.
//	FlagWallNorthProjBlocker is NOT in the south mask, so un-swap
//	would pass.
//
//	If implementer reverses to self-as-source (forward LoS), the ray
//	passes and inApproachDistance returns true; this test flips red.
func TestNpcInApproachDistanceNpcBackwardArgsQuirk(t *testing.T) {
	s, n, p := approachDistanceFixture(t)
	s.gamemap.Pathfinder.Flags.Add(3094, 3107, 0, collision.FlagWallNorthProjBlocker)

	if n.inApproachDistance(5, p) {
		t.Error("inApproachDistance: got true, want false — target-as-source LoS " +
			"blocked; if true, the TS NPC-backward arg order is reverted (bug)")
	}
}

// TestNpcInApproachDistancePlayerFlagIsRespected guards the
// CollisionFlag.PLAYER extraFlag wiring at GameMap.ts:433-435. Places
// only FlagBlockPlayers at a mid-tile (no wall, no proj-blocker). The
// ray would PASS if extraFlag=0, but BLOCK if extraFlag=FlagBlockPlayers.
// Proves inApproachDistance actually passes FlagBlockPlayers through.
func TestNpcInApproachDistancePlayerFlagIsRespected(t *testing.T) {
	s, n, p := approachDistanceFixture(t)
	s.gamemap.Pathfinder.Flags.Add(3094, 3107, 0, collision.FlagBlockPlayers)

	if n.inApproachDistance(5, p) {
		t.Error("inApproachDistance: got true, want false — FlagBlockPlayers " +
			"mid-tile; if true, extraFlag=FlagBlockPlayers is not wired (bug)")
	}
}
