package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// NAI-147 T3 — TS Player.ts:986-988 / :1020-1022 null-type guard.
// `triggerTypeAndCategory` returns ok=false when the target's type
// config is unresolvable; getOpTrigger/getApTrigger short-circuit to
// nil before reaching GetByTrigger.

func TestTriggerTypeAndCategory_NpcWithNilType_OkFalse(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	npc.typ = nil
	p.SetInteraction(InteractionEngine, npc, 1, -1)

	_, _, ok := triggerTypeAndCategory(p, s)
	if ok {
		t.Errorf("ok: got true, want false (Npc{typ:nil} → ok=false per TS L986-988)")
	}
}

func TestTriggerTypeAndCategory_LocOutOfRange_OkFalse(t *testing.T) {
	s := newTestServer(t)
	s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 10)}
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	loc := entitypkg.NewLoc(0, 100, 100, 1, 1, entitypkg.LifecycleForever, 999999, 10, 0)
	p.SetInteraction(InteractionEngine, loc, 1, -1)

	_, _, ok := triggerTypeAndCategory(p, s)
	if ok {
		t.Errorf("ok: got true, want false (Loc{Type:OOB} → ok=false)")
	}
}

func TestTriggerTypeAndCategory_LocNilConfig_OkFalse(t *testing.T) {
	s := newTestServer(t)
	s.locTypes = &objtype.LocTypeConfigs{Configs: make([]*objtype.LocType, 10)}
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	loc := entitypkg.NewLoc(0, 100, 100, 1, 1, entitypkg.LifecycleForever, 5, 10, 0)
	p.SetInteraction(InteractionEngine, loc, 1, -1)

	_, _, ok := triggerTypeAndCategory(p, s)
	if ok {
		t.Errorf("ok: got true, want false (Loc{Type:5,Configs[5]:nil} → ok=false)")
	}
}

func TestTriggerTypeAndCategory_ObjOutOfRange_OkFalse(t *testing.T) {
	s := newTestServer(t)
	s.objTypes = &objtype.ObjTypeConfigs{Configs: make([]*objtype.ObjType, 10)}
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	obj := entitypkg.NewObj(0, 100, 100, entitypkg.LifecycleForever, 999999, 1)
	p.SetInteraction(InteractionEngine, obj, 1, -1)

	_, _, ok := triggerTypeAndCategory(p, s)
	if ok {
		t.Errorf("ok: got true, want false (Obj{Type:OOB} → ok=false)")
	}
}

func TestTriggerTypeAndCategory_ObjNilConfig_OkFalse(t *testing.T) {
	s := newTestServer(t)
	s.objTypes = &objtype.ObjTypeConfigs{Configs: make([]*objtype.ObjType, 10)}
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	obj := entitypkg.NewObj(0, 100, 100, entitypkg.LifecycleForever, 5, 1)
	p.SetInteraction(InteractionEngine, obj, 1, -1)

	_, _, ok := triggerTypeAndCategory(p, s)
	if ok {
		t.Errorf("ok: got true, want false (Obj{Type:5,Configs[5]:nil} → ok=false)")
	}
}

func TestTriggerTypeAndCategory_NpcOk(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	npc.typ = &objtype.NpcType{ID: npc.typeId, Category: 7}
	p.SetInteraction(InteractionEngine, npc, 1, -1)

	typeId, categoryId, ok := triggerTypeAndCategory(p, s)
	if !ok {
		t.Errorf("ok: got false, want true")
	}
	if typeId != npc.typeId {
		t.Errorf("typeId: got %d, want %d", typeId, npc.typeId)
	}
	if categoryId != 7 {
		t.Errorf("categoryId: got %d, want 7", categoryId)
	}
}

func TestTriggerTypeAndCategory_PlayerOk(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	other, otherWait := makeInteractionPlayer(t, s, 101, 100, 0)
	defer otherWait()
	p.SetInteraction(InteractionEngine, other, 1, -1)

	typeId, categoryId, ok := triggerTypeAndCategory(p, s)
	if !ok {
		t.Errorf("ok: got false, want true (Player target has no type lookup)")
	}
	if typeId != -1 {
		t.Errorf("typeId: got %d, want -1 (Player branch)", typeId)
	}
	if categoryId != -1 {
		t.Errorf("categoryId: got %d, want -1 (Player branch)", categoryId)
	}
}

func TestGetOpTrigger_NilTypeReturnsNil(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	globalScript := &script.ScriptFile{
		Name:      "[opnpc1,_global]",
		LookupKey: script.LookupKeyForGlobal(script.TriggerOpNpc1),
	}
	s.scriptProvider.Register(globalScript)

	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	npc.typ = nil
	p.SetInteraction(InteractionEngine, npc, 1, -1)

	got := getOpTrigger(p, s)
	if got != nil {
		t.Errorf("getOpTrigger: got %v, want nil (Npc{typ:nil} short-circuits)", got)
	}
}

func TestGetApTrigger_NilTypeReturnsNil(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	globalScript := &script.ScriptFile{
		Name:      "[apnpc1,_global]",
		LookupKey: script.LookupKeyForGlobal(script.TriggerApNpc1),
	}
	s.scriptProvider.Register(globalScript)

	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	npc.typ = nil
	p.SetInteraction(InteractionEngine, npc, 1, -1)

	got := getApTrigger(p, s)
	if got != nil {
		t.Errorf("getApTrigger: got %v, want nil", got)
	}
}

func TestGetOpTrigger_TypeKnownResolvesAtCategoryFallback(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()

	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	npc.typ = &objtype.NpcType{ID: npc.typeId, Category: 0}

	categoryScript := &script.ScriptFile{
		Name:      "[opnpc1,_category0]",
		LookupKey: script.LookupKeyForCategory(script.TriggerOpNpc1, 0),
	}
	s.scriptProvider.Register(categoryScript)

	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	p.SetInteraction(InteractionEngine, npc, 1, -1)

	got := getOpTrigger(p, s)
	if got == nil {
		t.Errorf("getOpTrigger: got nil, want categoryScript (type known → 3-tier fallback fires)")
	}
}
