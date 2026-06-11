package world

import (
	"fmt"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
)

// newActivePlayer builds a Player in the "live session" state for use as
// an npc-target: active, default visibility, attached client. Mirrors
// the post-NAI-11-Task-4 IsValid semantics.
func newActivePlayer(slot int) *Player {
	return &Player{
		slot:       slot,
		active:     true,
		visibility: rsbuf.VisibilityDefault,
		client:     &client{},
	}
}

type triggerMapCase struct {
	op   int
	want script.ServerTriggerType
}

func runTriggerMapTest(t *testing.T, name string, fn func(int) script.ServerTriggerType, cases []triggerMapCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s/op=%d", name, tc.op), func(t *testing.T) {
			if got := fn(tc.op); got != tc.want {
				t.Errorf("%s(%d) = %d, want %d", name, tc.op, got, tc.want)
			}
		})
	}
}

func TestAiApNpcTriggerForOp(t *testing.T) {
	cases := []triggerMapCase{
		{objtype.NPCModeApNpc1, script.TriggerAiApNpc1},
		{objtype.NPCModeApNpc2, script.TriggerAiApNpc2},
		{objtype.NPCModeApNpc3, script.TriggerAiApNpc3},
		{objtype.NPCModeApNpc4, script.TriggerAiApNpc4},
		{objtype.NPCModeApNpc5, script.TriggerAiApNpc5},
		{objtype.NPCModeApNpc5 + 1, 0}, // out-of-range high
		{objtype.NPCModeApNpc1 - 1, 0}, // out-of-range low
		{objtype.NPCModeApPlayer1, 0},  // wrong category
	}
	runTriggerMapTest(t, "aiApNpcTriggerForOp", aiApNpcTriggerForOp, cases)
}

func TestAiOpNpcTriggerForOp(t *testing.T) {
	cases := []triggerMapCase{
		{objtype.NPCModeOpNpc1, script.TriggerAiOpNpc1},
		{objtype.NPCModeOpNpc5, script.TriggerAiOpNpc5},
		{objtype.NPCModeOpNpc5 + 1, 0},
		{objtype.NPCModeApNpc1, 0},
	}
	runTriggerMapTest(t, "aiOpNpcTriggerForOp", aiOpNpcTriggerForOp, cases)
}

func TestAiApPlayerTriggerForOp(t *testing.T) {
	cases := []triggerMapCase{
		{objtype.NPCModeApPlayer1, script.TriggerAiApPlayer1},
		{objtype.NPCModeApPlayer5, script.TriggerAiApPlayer5},
		{objtype.NPCModeApPlayer5 + 1, 0},
		{objtype.NPCModeOpPlayer1, 0},
	}
	runTriggerMapTest(t, "aiApPlayerTriggerForOp", aiApPlayerTriggerForOp, cases)
}

func TestAiOpPlayerTriggerForOp(t *testing.T) {
	cases := []triggerMapCase{
		{objtype.NPCModeOpPlayer1, script.TriggerAiOpPlayer1},
		{objtype.NPCModeOpPlayer5, script.TriggerAiOpPlayer5},
		{objtype.NPCModeOpPlayer5 + 1, 0},
	}
	runTriggerMapTest(t, "aiOpPlayerTriggerForOp", aiOpPlayerTriggerForOp, cases)
}

func TestAiApLocTriggerForOp(t *testing.T) {
	cases := []triggerMapCase{
		{objtype.NPCModeApLoc1, script.TriggerAiApLoc1},
		{objtype.NPCModeApLoc5, script.TriggerAiApLoc5},
		{objtype.NPCModeApLoc5 + 1, 0},
	}
	runTriggerMapTest(t, "aiApLocTriggerForOp", aiApLocTriggerForOp, cases)
}

func TestAiOpLocTriggerForOp(t *testing.T) {
	cases := []triggerMapCase{
		{objtype.NPCModeOpLoc1, script.TriggerAiOpLoc1},
		{objtype.NPCModeOpLoc5, script.TriggerAiOpLoc5},
		{objtype.NPCModeOpLoc5 + 1, 0},
	}
	runTriggerMapTest(t, "aiOpLocTriggerForOp", aiOpLocTriggerForOp, cases)
}

func TestAiApObjTriggerForOp(t *testing.T) {
	cases := []triggerMapCase{
		{objtype.NPCModeApObj1, script.TriggerAiApObj1},
		{objtype.NPCModeApObj5, script.TriggerAiApObj5},
		{objtype.NPCModeApObj5 + 1, 0},
	}
	runTriggerMapTest(t, "aiApObjTriggerForOp", aiApObjTriggerForOp, cases)
}

func TestAiOpObjTriggerForOp(t *testing.T) {
	cases := []triggerMapCase{
		{objtype.NPCModeOpObj1, script.TriggerAiOpObj1},
		{objtype.NPCModeOpObj5, script.TriggerAiOpObj5},
		{objtype.NPCModeOpObj5 + 1, 0},
	}
	runTriggerMapTest(t, "aiOpObjTriggerForOp", aiOpObjTriggerForOp, cases)
}

// newNpcWithType builds a fresh target NPC with a given typeId and
// category (category defaults to 0). Mirrors the setup pattern used
// by the Npc-target fire tests.
func newNpcWithType(typeId, category int) *Npc {
	typ := &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: typeId},
		Category:   category,
	}
	return NewNpc(2, typeId, 101, 100, 0, typ)
}

// Top-level dispatcher tests ----------------------------------------------

func TestFireAiOpTriggerDispatchesPlayer(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiOpPlayer1, 0, "disp-player"))

	n := newNpcForScriptTest(t)
	n.server = s
	n.targetOp = objtype.NPCModeOpPlayer1
	p := newActivePlayer(3)
	n.target = p

	n.fireAiOpTrigger(s)

	if string(n.sayText) != "disp-player" {
		t.Errorf("sayText: got %q, want %q (player branch)", n.sayText, "disp-player")
	}
}

func TestFireAiApTriggerDispatchesNpc(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiApNpc1, 0, "disp-npc")) // keyed on the acting npc (type 0)

	n := newNpcForScriptTest(t)
	n.server = s
	n.targetOp = objtype.NPCModeApNpc1
	target := newNpcWithType(99, 0)
	n.target = target

	n.fireAiApTrigger(s)

	if string(n.sayText) != "disp-npc" {
		t.Errorf("sayText: got %q, want %q (npc branch)", n.sayText, "disp-npc")
	}
}

func TestFireAiOpTriggerDispatchesLoc(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiOpLoc1, 0, "disp-loc")) // keyed on the acting npc (type 0)

	n := newNpcForScriptTest(t)
	n.server = s
	n.targetOp = objtype.NPCModeOpLoc1

	loc := addLocToZone(t, s, 0, 101, 100, 77, 0)
	n.target = loc
	n.targetSubject.typ = loc.Type()

	n.fireAiOpTrigger(s)

	if string(n.sayText) != "disp-loc" {
		t.Errorf("sayText: got %q, want %q (loc branch)", n.sayText, "disp-loc")
	}
}

func TestFireAiApTriggerDispatchesObj(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiApObj1, 0, "disp-obj")) // keyed on the acting npc (type 0)

	n := newNpcForScriptTest(t)
	n.server = s
	n.targetOp = objtype.NPCModeApObj1

	obj := addObjToZone(t, s, 0, 101, 100, 88, 0)
	n.target = obj

	n.fireAiApTrigger(s)

	if string(n.sayText) != "disp-obj" {
		t.Errorf("sayText: got %q, want %q (obj branch)", n.sayText, "disp-obj")
	}
}

func TestFireAiOpTriggerNilTargetNoOp(t *testing.T) {
	s := newServerForScriptTest(t)
	n := newNpcForScriptTest(t)
	n.server = s
	n.target = nil // no crash expected

	n.fireAiOpTrigger(s)
}

// Obj-target fire helper tests --------------------------------------------
// addObjToZone is defined in npc_hunt_entities_test.go and reused here.

func TestFireAiOpTriggerObjHappyPath(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiOpObj1, 0, "aiop-obj")) // keyed on the acting npc (type 0)

	n := newNpcForScriptTest(t)
	n.server = s
	n.targetOp = objtype.NPCModeOpObj1

	obj := addObjToZone(t, s, 0, 101, 100, 88, 0)
	n.target = obj
	n.targetSubject.typ = obj.Type

	n.fireAiOpTriggerObj(s, obj)

	if string(n.sayText) != "aiop-obj" {
		t.Errorf("sayText: got %q, want %q", n.sayText, "aiop-obj")
	}
}

func TestFireAiOpTriggerObjNoScriptClears(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()

	n := newNpcForScriptTest(t)
	n.server = s
	n.targetOp = objtype.NPCModeOpObj1

	obj := addObjToZone(t, s, 0, 101, 100, 88, 0)
	n.target = obj

	n.fireAiOpTriggerObj(s, obj)

	if n.target != nil {
		t.Error("target: expected nil after no-script-found clearInteraction")
	}
}

func TestFireAiOpTriggerObjZoneStaleClears(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiOpObj1, 0, "aiop-obj")) // keyed on the acting npc (type 0)

	n := newNpcForScriptTest(t)
	n.server = s
	n.targetOp = objtype.NPCModeOpObj1

	obj := addObjToZone(t, s, 0, 101, 100, 88, 0)
	n.target = obj

	// Evict from zone.
	zn := s.zoneMap.Get(0, 101, 100)
	zn.Objs = nil

	n.fireAiOpTriggerObj(s, obj)

	if n.target != nil {
		t.Error("target: expected nil after zone-stale clearInteraction")
	}
}

func TestFireAiApTriggerObjHappyPath(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiApObj1, 0, "aiap-obj")) // keyed on the acting npc (type 0)

	n := newNpcForScriptTest(t)
	n.server = s
	n.targetOp = objtype.NPCModeApObj1

	obj := addObjToZone(t, s, 0, 101, 100, 88, 0)
	n.target = obj

	n.fireAiApTriggerObj(s, obj)

	if string(n.sayText) != "aiap-obj" {
		t.Errorf("sayText: got %q, want %q", n.sayText, "aiap-obj")
	}
}

// Loc-target fire helper tests --------------------------------------------
// addLocToZone is defined in npc_hunt_entities_test.go and reused here.

func TestFireAiOpTriggerLocHappyPath(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiOpLoc1, 0, "aiop-loc")) // keyed on the acting npc (type 0)

	n := newNpcForScriptTest(t)
	n.server = s
	n.targetOp = objtype.NPCModeOpLoc1

	loc := addLocToZone(t, s, 0, 101, 100, 77, 0)
	n.target = loc
	n.targetSubject.typ = loc.Type()

	n.fireAiOpTriggerLoc(s, loc)

	if string(n.sayText) != "aiop-loc" {
		t.Errorf("sayText: got %q, want %q", n.sayText, "aiop-loc")
	}
}

func TestFireAiOpTriggerLocNoScriptClears(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()

	n := newNpcForScriptTest(t)
	n.server = s
	n.targetOp = objtype.NPCModeOpLoc1

	loc := addLocToZone(t, s, 0, 101, 100, 77, 0)
	n.target = loc
	n.targetSubject.typ = loc.Type()

	n.fireAiOpTriggerLoc(s, loc)

	if n.target != nil {
		t.Error("target: expected nil after no-script-found clearInteraction")
	}
}

func TestFireAiOpTriggerLocZoneStaleClears(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiOpLoc1, 0, "aiop-loc")) // keyed on the acting npc (type 0)

	n := newNpcForScriptTest(t)
	n.server = s
	n.targetOp = objtype.NPCModeOpLoc1

	loc := addLocToZone(t, s, 0, 101, 100, 77, 0)
	n.target = loc
	n.targetSubject.typ = loc.Type()

	// Evict from zone to simulate stale pointer.
	zn := s.zoneMap.Get(0, 101, 100)
	zn.Locs = nil

	n.fireAiOpTriggerLoc(s, loc)

	if n.target != nil {
		t.Error("target: expected nil after zone-stale clearInteraction")
	}
	if string(n.sayText) == "aiop-loc" {
		t.Error("sayText: script ran despite zone-stale target")
	}
}

func TestFireAiApTriggerLocHappyPath(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiApLoc1, 0, "aiap-loc")) // keyed on the acting npc (type 0)

	n := newNpcForScriptTest(t)
	n.server = s
	n.targetOp = objtype.NPCModeApLoc1

	loc := addLocToZone(t, s, 0, 101, 100, 77, 0)
	n.target = loc
	n.targetSubject.typ = loc.Type()

	n.fireAiApTriggerLoc(s, loc)

	if string(n.sayText) != "aiap-loc" {
		t.Errorf("sayText: got %q, want %q", n.sayText, "aiap-loc")
	}
}

// Npc-target fire helper tests --------------------------------------------

func TestFireAiOpTriggerNpcHappyPath(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiOpNpc1, 0, "aiop-npc")) // keyed on the acting npc (type 0)

	n := newNpcForScriptTest(t)
	n.server = s
	n.targetOp = objtype.NPCModeOpNpc1

	target := newNpcWithType(99, 0)
	n.target = target

	n.fireAiOpTriggerNpc(s, target)

	if string(n.sayText) != "aiop-npc" {
		t.Errorf("sayText: got %q, want %q", n.sayText, "aiop-npc")
	}
}

func TestFireAiOpTriggerNpcNoScriptClears(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()

	n := newNpcForScriptTest(t)
	n.server = s
	n.targetOp = objtype.NPCModeOpNpc1

	target := newNpcWithType(99, 0)
	n.target = target

	n.fireAiOpTriggerNpc(s, target)

	if n.target != nil {
		t.Error("target: expected nil after no-script-found clearInteraction")
	}
}

func TestFireAiOpTriggerNpcDeadTargetClears(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiOpNpc1, 0, "aiop-npc")) // keyed on the acting npc (type 0)

	n := newNpcForScriptTest(t)
	n.server = s
	n.targetOp = objtype.NPCModeOpNpc1

	target := newNpcWithType(99, 0)
	target.dead = true
	n.target = target

	n.fireAiOpTriggerNpc(s, target)

	if n.target != nil {
		t.Error("target: expected nil after dead-target clearInteraction")
	}
	if string(n.sayText) == "aiop-npc" {
		t.Error("sayText: script ran despite dead target")
	}
}

func TestFireAiApTriggerNpcHappyPath(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiApNpc1, 0, "aiap-npc")) // keyed on the acting npc (type 0)

	n := newNpcForScriptTest(t)
	n.server = s
	n.targetOp = objtype.NPCModeApNpc1

	target := newNpcWithType(99, 0)
	n.target = target

	n.fireAiApTriggerNpc(s, target)

	if string(n.sayText) != "aiap-npc" {
		t.Errorf("sayText: got %q, want %q", n.sayText, "aiap-npc")
	}
}

// Player-target fire helper tests ------------------------------------------

func TestFireAiOpTriggerPlayerHappyPath(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiOpPlayer1, 0, "aiop-fired"))

	n := newNpcForScriptTest(t)
	n.server = s
	n.targetOp = objtype.NPCModeOpPlayer1
	p := newActivePlayer(3)
	n.target = p

	n.fireAiOpTriggerPlayer(s, p)

	if string(n.sayText) != "aiop-fired" {
		t.Errorf("sayText: got %q, want %q", n.sayText, "aiop-fired")
	}
}

func TestFireAiOpTriggerPlayerNoScriptClears(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider() // empty

	n := newNpcForScriptTest(t)
	n.server = s
	n.targetOp = objtype.NPCModeOpPlayer1
	p := newActivePlayer(3)
	n.target = p

	n.fireAiOpTriggerPlayer(s, p)

	if n.target != nil {
		t.Error("target: expected nil after no-script-found clearInteraction")
	}
}

func TestFireAiOpTriggerPlayerInvalidClears(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiOpPlayer1, 0, "aiop-fired"))

	n := newNpcForScriptTest(t)
	n.server = s
	n.targetOp = objtype.NPCModeOpPlayer1
	p := newActivePlayer(3)
	p.active = false // makes IsValid return false
	n.target = p

	n.fireAiOpTriggerPlayer(s, p)

	if n.target != nil {
		t.Error("target: expected nil after IsValid-false clearInteraction")
	}
	if string(n.sayText) == "aiop-fired" {
		t.Error("sayText: script ran despite invalid target")
	}
}

func TestFireAiOpTriggerPlayerOutOfRangeOpClears(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()

	n := newNpcForScriptTest(t)
	n.server = s
	n.targetOp = objtype.NPCModeApPlayer1 // wrong category for the Op helper
	p := newActivePlayer(3)
	n.target = p

	n.fireAiOpTriggerPlayer(s, p)

	if n.target != nil {
		t.Error("target: expected nil after out-of-range-op clearInteraction")
	}
}

func TestFireAiApTriggerPlayerHappyPath(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiApPlayer1, 0, "aiap-fired"))

	n := newNpcForScriptTest(t)
	n.server = s
	n.targetOp = objtype.NPCModeApPlayer1
	p := newActivePlayer(3)
	n.target = p

	n.fireAiApTriggerPlayer(s, p)

	if string(n.sayText) != "aiap-fired" {
		t.Errorf("sayText: got %q, want %q", n.sayText, "aiap-fired")
	}
}
