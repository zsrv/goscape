package world

import (
	"bytes"
	"strconv"
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// NAI-147 T4 — TS Player.ts:1076-1093 debug chat under NodeDebug.
// Numeric trigger-name fallback per NAI-147-D-TRIGGER-NAME-NUMERIC.

// makeDefaultOpFixture builds a server with all type configs seeded
// and a player wired with encryptor (required for MessageGame writes).
func makeDefaultOpFixture(t *testing.T) (*Server, *Player, <-chan []byte) {
	t.Helper()
	s := newTestServer(t)
	s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 100)}
	s.objTypes = &objtype.ObjTypeConfigs{Configs: make([]*objtype.ObjType, 100)}
	s.componentTypes = &objtype.ComponentTypeConfigs{Configs: make([]*objtype.ComponentType, 1000)}
	p, cc := newTestPlayer(t)
	p.client.server = s
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	received := drainConn(t, cc)
	return s, p, received
}

func TestDefaultOp_NoTriggerEmitsDebug_NodeDebugTrue(t *testing.T) {
	s, p, received := makeDefaultOpFixture(t)
	s.cfg.NodeDebug = true

	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	npc.typ = &objtype.NpcType{ConfigType: objtype.ConfigType{ID: npc.typeId, DebugName: "test_npc"}}
	p.SetInteraction(InteractionEngine, npc, 1, -1) // targetOp=1 → ApNpc1; +7 → OpNpc1

	defaultOp(p, nil, nil)
	p.client.flushWrite()
	got := <-received

	// Goscape stores targetOp as an op-slot (1-5), not a ServerTriggerType
	// enum value. The +7 offset is preserved from TS Player.ts:1092
	// (NAI-147-D-TRIGGER-NAME-NUMERIC). For op-slot=1, emitted value = 1+7=8.
	// The script import is kept to anchor the deviation documentation.
	_ = script.TriggerOpNpc1 // deviation doc anchor — see NAI-147-D-TRIGGER-NAME-NUMERIC
	wantDebug := []byte("No trigger for [" + strconv.Itoa(1+7) + ",test_npc]")
	if !bytes.Contains(got, wantDebug) {
		t.Errorf("missing debug message %q on wire; got %x", wantDebug, got)
	}
	if !bytes.Contains(got, []byte("Nothing interesting happens.")) {
		t.Errorf("missing NIH message on wire; got %x", got)
	}
}

func TestDefaultOp_NoTriggerSuppressed_NodeDebugFalse(t *testing.T) {
	s, p, received := makeDefaultOpFixture(t)
	s.cfg.NodeDebug = false

	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	npc.typ = &objtype.NpcType{ConfigType: objtype.ConfigType{ID: npc.typeId, DebugName: "test_npc"}}
	p.SetInteraction(InteractionEngine, npc, 1, -1)

	defaultOp(p, nil, nil)
	p.client.flushWrite()
	got := <-received

	if bytes.Contains(got, []byte("No trigger for")) {
		t.Errorf("debug message leaked under NodeDebug=false; got %x", got)
	}
	if !bytes.Contains(got, []byte("Nothing interesting happens.")) {
		t.Errorf("missing NIH message; got %x", got)
	}
}

func TestDefaultOp_DebugSuppressed_OpTriggerPresent(t *testing.T) {
	s, p, received := makeDefaultOpFixture(t)
	s.cfg.NodeDebug = true

	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	npc.typ = &objtype.NpcType{ConfigType: objtype.ConfigType{ID: npc.typeId, DebugName: "test_npc"}}
	p.SetInteraction(InteractionEngine, npc, 1, -1)

	stub := &script.ScriptFile{Name: "[opnpc1,test_npc]"}
	defaultOp(p, stub, nil)
	p.client.flushWrite()
	got := <-received

	if bytes.Contains(got, []byte("No trigger for")) {
		t.Errorf("debug message leaked when opTrigger non-nil; got %x", got)
	}
}

func TestDefaultOp_DebugSuppressed_ApTriggerPresent(t *testing.T) {
	s, p, received := makeDefaultOpFixture(t)
	s.cfg.NodeDebug = true

	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	npc.typ = &objtype.NpcType{ConfigType: objtype.ConfigType{ID: npc.typeId, DebugName: "test_npc"}}
	p.SetInteraction(InteractionEngine, npc, 1, -1)

	stub := &script.ScriptFile{Name: "[apnpc1,test_npc]"}
	defaultOp(p, nil, stub)
	p.client.flushWrite()
	got := <-received

	if bytes.Contains(got, []byte("No trigger for")) {
		t.Errorf("debug message leaked when apTrigger non-nil; got %x", got)
	}
}

func TestDefaultOp_DebugnameNpc_FallbackToTypeId(t *testing.T) {
	s, p, received := makeDefaultOpFixture(t)
	s.cfg.NodeDebug = true

	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	npc.typ = &objtype.NpcType{ConfigType: objtype.ConfigType{ID: npc.typeId, DebugName: ""}} // empty
	p.SetInteraction(InteractionEngine, npc, 1, -1)

	defaultOp(p, nil, nil)
	p.client.flushWrite()
	got := <-received

	want := []byte("," + strconv.Itoa(npc.typeId) + "]")
	if !bytes.Contains(got, want) {
		t.Errorf("debug message debugname fallback to typeId: missing %q; got %x", want, got)
	}
}

func TestDefaultOp_DebugnameLoc(t *testing.T) {
	s, p, received := makeDefaultOpFixture(t)
	s.cfg.NodeDebug = true

	s.locTypes.Configs[42] = &objtype.LocType{
		ConfigType: objtype.ConfigType{ID: 42, DebugName: "newbie_door1"},
	}
	loc := entitypkg.NewLoc(0, 100, 100, 1, 1, entitypkg.LifecycleForever, 42, 10, 0)
	p.SetInteraction(InteractionEngine, loc, 1, -1)

	defaultOp(p, nil, nil)
	p.client.flushWrite()
	got := <-received

	if !bytes.Contains(got, []byte(",newbie_door1]")) {
		t.Errorf("debug message Loc debugname missing; got %x", got)
	}
}

func TestDefaultOp_DebugnameObj(t *testing.T) {
	s, p, received := makeDefaultOpFixture(t)
	s.cfg.NodeDebug = true

	s.objTypes.Configs[42] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: 42, DebugName: "bones"},
	}
	obj := entitypkg.NewObj(0, 100, 100, entitypkg.LifecycleForever, 42, 1)
	p.SetInteraction(InteractionEngine, obj, 1, -1)

	defaultOp(p, nil, nil)
	p.client.flushWrite()
	got := <-received

	if !bytes.Contains(got, []byte(",bones]")) {
		t.Errorf("debug message Obj debugname missing; got %x", got)
	}
}

func TestDefaultOp_DebugnameComOverride_TBranch(t *testing.T) {
	s, p, received := makeDefaultOpFixture(t)
	s.cfg.NodeDebug = true

	s.componentTypes.Configs[200] = &objtype.ComponentType{
		RootLayer: 200,
		ComName:   "spell_blast",
	}
	// T-branch fires only when target is NOT Npc/Loc/Obj (TS L1086 else-if
	// chain). Use a Player target with targetOpNpcT and com set so the
	// switch falls through and the T-trigger branch evaluates.
	other, otherWait := makeInteractionPlayer(t, s, 101, 100, 0)
	defer otherWait()
	p.SetInteraction(InteractionEngine, other, targetOpNpcT, 200)

	defaultOp(p, nil, nil)
	p.client.flushWrite()
	got := <-received

	if !bytes.Contains(got, []byte(",spell_blast]")) {
		t.Errorf("debug message com-name (T-branch) missing; got %x", got)
	}
}

// TestDefaultOp_DebugnameTBranch_PlayerTUnconditional pins TS L1086:
// the `com !== -1` guard applies only to APNPCT. APPLAYERT/APLOCT/APOBJT
// enter the com branch unconditionally and fall through to the numeric
// targetSubject form when com is -1.
func TestDefaultOp_DebugnameTBranch_PlayerTUnconditional(t *testing.T) {
	s, p, received := makeDefaultOpFixture(t)
	s.cfg.NodeDebug = true

	other, otherWait := makeInteractionPlayer(t, s, 101, 100, 0)
	defer otherWait()
	p.SetInteraction(InteractionEngine, other, targetOpPlayerT, -1)

	defaultOp(p, nil, nil)
	p.client.flushWrite()
	got := <-received

	if !bytes.Contains(got, []byte(",-1]")) {
		t.Errorf("debug message numeric com fallback (PlayerT, com=-1) missing; got %x", got)
	}
}

// TestDefaultOp_DebugnameTBranch_NpcTGuarded pins the inverse: APNPCT
// with com=-1 SKIPS the T-branch (TS L1086 — the `com !== -1` guard
// applies only here). Falls through to subjectType then default `_`.
func TestDefaultOp_DebugnameTBranch_NpcTGuarded(t *testing.T) {
	s, p, received := makeDefaultOpFixture(t)
	s.cfg.NodeDebug = true

	other, otherWait := makeInteractionPlayer(t, s, 101, 100, 0)
	defer otherWait()
	p.SetInteraction(InteractionEngine, other, targetOpNpcT, -1)
	p.targetSubject.typ = -1

	defaultOp(p, nil, nil)
	p.client.flushWrite()
	got := <-received

	if !bytes.Contains(got, []byte(",_]")) {
		t.Errorf("debug message default underscore (NpcT, com=-1) missing; got %x", got)
	}
}

func TestDefaultOp_DebugnameSubjectTypeOverride(t *testing.T) {
	s, p, received := makeDefaultOpFixture(t)
	s.cfg.NodeDebug = true

	s.objTypes.Configs[42] = &objtype.ObjType{
		ConfigType: objtype.ConfigType{ID: 42, DebugName: "bones"},
	}
	other, otherWait := makeInteractionPlayer(t, s, 101, 100, 0)
	defer otherWait()
	p.SetInteraction(InteractionEngine, other, 1, -1)
	// Manually set targetSubject.typ — TS targetSubject.type analogue.
	p.targetSubject.typ = 42

	defaultOp(p, nil, nil)
	p.client.flushWrite()
	got := <-received

	if !bytes.Contains(got, []byte(",bones]")) {
		t.Errorf("debug message subjectType debugname missing; got %x", got)
	}
}

func TestDefaultOp_DebugnameDefault_Underscore(t *testing.T) {
	s, p, received := makeDefaultOpFixture(t)
	s.cfg.NodeDebug = true

	other, otherWait := makeInteractionPlayer(t, s, 101, 100, 0)
	defer otherWait()
	p.SetInteraction(InteractionEngine, other, 1, -1)
	p.targetSubject.com = -1
	p.targetSubject.typ = -1

	defaultOp(p, nil, nil)
	p.client.flushWrite()
	got := <-received

	if !bytes.Contains(got, []byte(",_]")) {
		t.Errorf("debug message default underscore missing; got %x", got)
	}
}

func TestDefaultOp_ClearWaypointsAlwaysFires(t *testing.T) {
	s, p, _ := makeDefaultOpFixture(t)
	s.cfg.NodeDebug = false

	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	npc.typ = &objtype.NpcType{ConfigType: objtype.ConfigType{ID: npc.typeId, DebugName: "test_npc"}}
	p.SetInteraction(InteractionEngine, npc, 1, -1)
	p.waypointIndex = 5

	defaultOp(p, nil, nil)

	if p.waypointIndex != -1 {
		t.Errorf("waypointIndex: got %d, want -1 (TS Player.ts:1096)", p.waypointIndex)
	}
}

func TestDefaultOp_NothingInteresting_AlwaysFires(t *testing.T) {
	for _, debug := range []bool{true, false} {
		debug := debug
		t.Run(strconv.FormatBool(debug), func(t *testing.T) {
			s, p, received := makeDefaultOpFixture(t)
			s.cfg.NodeDebug = debug

			npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
			npc.typ = &objtype.NpcType{ConfigType: objtype.ConfigType{ID: npc.typeId, DebugName: "test_npc"}}
			p.SetInteraction(InteractionEngine, npc, 1, -1)

			defaultOp(p, nil, nil)
			p.client.flushWrite()
			got := <-received

			if !bytes.Contains(got, []byte("Nothing interesting happens.")) {
				t.Errorf("NodeDebug=%v: missing NIH message; got %x", debug, got)
			}
		})
	}
}
