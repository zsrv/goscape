package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// npc-ai-1 regression. TS Npc.tryInteract resolves the AI trigger ONCE,
// target-independently, via getTrigger(type) where
// `type = NpcType.get(this.type)` — the ACTING npc's own type id and
// category (src/engine/entity/Npc.ts:865-866,989-995):
//
//	return ScriptProvider.getByTrigger(trigger, this.type, type.category)
//
// The lookup subject+category are the acting npc's for EVERY target kind
// (player/npc/loc/obj), never the target's. Pre-fix the Go fireAi*
// helpers keyed GetByTrigger on the TARGET's type/category (and a
// hardcoded (0,0) for players), so the wrong script subject was
// dispatched. These tests fail until the helpers key on the acting npc.
const (
	aiActingType     = 42
	aiActingCategory = 5
	aiNpcTargetType  = 99
	aiLocTargetType  = 77
	aiObjTargetType  = 88
)

// newActingNpcForAiTest returns an acting npc wired to s with a known,
// distinct type+category so a script keyed on the acting npc is
// distinguishable from one keyed on the target. newNpcForScriptTest
// defaults the type to 0, so we override both fields explicitly.
func newActingNpcForAiTest(t *testing.T, s *Server, op int) *Npc {
	t.Helper()
	n := newNpcForScriptTest(t)
	n.server = s
	n.targetOp = op
	n.typeId = aiActingType
	n.typ = &objtype.NpcType{
		ConfigType: objtype.ConfigType{ID: aiActingType},
		Category:   aiActingCategory,
	}
	return n
}

func TestFireAiTrigger_NpcTarget_KeyedByActingNpc(t *testing.T) {
	t.Run("ActingTypeScriptFires", func(t *testing.T) {
		s := newServerForScriptTest(t)
		s.scriptProvider = script.NewProvider()
		s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiOpNpc1, aiActingType, "by-acting"))

		n := newActingNpcForAiTest(t, s, objtype.NPCModeOpNpc1)
		target := newNpcWithType(aiNpcTargetType, 7)
		n.target = target

		n.fireAiOpTriggerNpc(s, target)

		if string(n.sayText) != "by-acting" {
			t.Errorf("sayText = %q, want %q (lookup must key on acting npc type %d, not target type %d)",
				n.sayText, "by-acting", aiActingType, aiNpcTargetType)
		}
	})

	t.Run("TargetTypeScriptIgnored", func(t *testing.T) {
		s := newServerForScriptTest(t)
		s.scriptProvider = script.NewProvider()
		s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiOpNpc1, aiNpcTargetType, "by-target"))

		n := newActingNpcForAiTest(t, s, objtype.NPCModeOpNpc1)
		target := newNpcWithType(aiNpcTargetType, 7)
		n.target = target

		n.fireAiOpTriggerNpc(s, target)

		if string(n.sayText) == "by-target" {
			t.Errorf("a script keyed on the TARGET type %d fired; TS keys getByTrigger on the acting npc type only",
				aiNpcTargetType)
		}
	})
}

// CategoryKeyedByActingNpc pins the SECOND argument of getByTrigger
// (type.category) to the acting npc's category. No type-specific script
// exists, so resolution falls back to the category tier; it must use the
// acting npc's category (5), not the target's (7).
func TestFireAiTrigger_NpcTarget_CategoryKeyedByActingNpc(t *testing.T) {
	t.Run("ActingCategoryScriptFires", func(t *testing.T) {
		s := newServerForScriptTest(t)
		s.scriptProvider = script.NewProvider()
		sc := buildNpcSayScript(script.TriggerAiOpNpc1, 0, "by-acting-cat")
		sc.LookupKey = script.LookupKeyForCategory(script.TriggerAiOpNpc1, aiActingCategory)
		s.scriptProvider.Register(sc)

		n := newActingNpcForAiTest(t, s, objtype.NPCModeOpNpc1)
		target := newNpcWithType(aiNpcTargetType, 7)
		n.target = target

		n.fireAiOpTriggerNpc(s, target)

		if string(n.sayText) != "by-acting-cat" {
			t.Errorf("sayText = %q, want %q (category fallback must use acting category %d)",
				n.sayText, "by-acting-cat", aiActingCategory)
		}
	})

	t.Run("TargetCategoryScriptIgnored", func(t *testing.T) {
		s := newServerForScriptTest(t)
		s.scriptProvider = script.NewProvider()
		sc := buildNpcSayScript(script.TriggerAiOpNpc1, 0, "by-target-cat")
		sc.LookupKey = script.LookupKeyForCategory(script.TriggerAiOpNpc1, 7) // target category
		s.scriptProvider.Register(sc)

		n := newActingNpcForAiTest(t, s, objtype.NPCModeOpNpc1)
		target := newNpcWithType(aiNpcTargetType, 7)
		n.target = target

		n.fireAiOpTriggerNpc(s, target)

		if string(n.sayText) == "by-target-cat" {
			t.Errorf("a script keyed on the TARGET category fired; getByTrigger must use the acting npc category")
		}
	})
}

func TestFireAiTrigger_LocTarget_KeyedByActingNpc(t *testing.T) {
	t.Run("ActingTypeScriptFires", func(t *testing.T) {
		s := newServerForScriptTest(t)
		s.scriptProvider = script.NewProvider()
		s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiOpLoc1, aiActingType, "by-acting"))

		n := newActingNpcForAiTest(t, s, objtype.NPCModeOpLoc1)
		loc := addLocToZone(t, s, 0, 101, 100, aiLocTargetType, 0)
		n.target = loc
		n.targetSubject.typ = loc.Type()

		n.fireAiOpTriggerLoc(s, loc)

		if string(n.sayText) != "by-acting" {
			t.Errorf("sayText = %q, want %q (loc lookup must key on acting npc type %d, not loc type %d)",
				n.sayText, "by-acting", aiActingType, aiLocTargetType)
		}
	})

	t.Run("TargetTypeScriptIgnored", func(t *testing.T) {
		s := newServerForScriptTest(t)
		s.scriptProvider = script.NewProvider()
		s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiOpLoc1, aiLocTargetType, "by-target"))

		n := newActingNpcForAiTest(t, s, objtype.NPCModeOpLoc1)
		loc := addLocToZone(t, s, 0, 101, 100, aiLocTargetType, 0)
		n.target = loc
		n.targetSubject.typ = loc.Type()

		n.fireAiOpTriggerLoc(s, loc)

		if string(n.sayText) == "by-target" {
			t.Errorf("a script keyed on the loc (target) type %d fired; TS keys on the acting npc", aiLocTargetType)
		}
	})
}

func TestFireAiTrigger_ObjTarget_KeyedByActingNpc(t *testing.T) {
	t.Run("ActingTypeScriptFires", func(t *testing.T) {
		s := newServerForScriptTest(t)
		s.scriptProvider = script.NewProvider()
		s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiOpObj1, aiActingType, "by-acting"))

		n := newActingNpcForAiTest(t, s, objtype.NPCModeOpObj1)
		obj := addObjToZone(t, s, 0, 101, 100, aiObjTargetType, 0)
		n.target = obj
		n.targetSubject.typ = obj.Type

		n.fireAiOpTriggerObj(s, obj)

		if string(n.sayText) != "by-acting" {
			t.Errorf("sayText = %q, want %q (obj lookup must key on acting npc type %d, not obj type %d)",
				n.sayText, "by-acting", aiActingType, aiObjTargetType)
		}
	})

	t.Run("TargetTypeScriptIgnored", func(t *testing.T) {
		s := newServerForScriptTest(t)
		s.scriptProvider = script.NewProvider()
		s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiOpObj1, aiObjTargetType, "by-target"))

		n := newActingNpcForAiTest(t, s, objtype.NPCModeOpObj1)
		obj := addObjToZone(t, s, 0, 101, 100, aiObjTargetType, 0)
		n.target = obj
		n.targetSubject.typ = obj.Type

		n.fireAiOpTriggerObj(s, obj)

		if string(n.sayText) == "by-target" {
			t.Errorf("a script keyed on the obj (target) type %d fired; TS keys on the acting npc", aiObjTargetType)
		}
	})
}

// PlayerTarget: pre-fix the player helper hardcoded GetByTrigger(.., 0, 0).
// TS keys on the acting npc's type+category just like the other kinds, so
// a script registered under the acting npc type must fire.
func TestFireAiTrigger_PlayerTarget_KeyedByActingNpc(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiOpPlayer1, aiActingType, "by-acting"))

	n := newActingNpcForAiTest(t, s, objtype.NPCModeOpPlayer1)
	p := newActivePlayer(3)
	n.target = p

	n.fireAiOpTriggerPlayer(s, p)

	if string(n.sayText) != "by-acting" {
		t.Errorf("sayText = %q, want %q (player lookup must key on acting npc type %d, not hardcoded 0)",
			n.sayText, "by-acting", aiActingType)
	}
}

// ApBand pins the approach-distance band through the same acting-npc
// contract (the OP tests cover the contact band).
func TestFireAiTrigger_ApBand_KeyedByActingNpc(t *testing.T) {
	s := newServerForScriptTest(t)
	s.scriptProvider = script.NewProvider()
	s.scriptProvider.Register(buildNpcSayScript(script.TriggerAiApNpc1, aiActingType, "by-acting"))

	n := newActingNpcForAiTest(t, s, objtype.NPCModeApNpc1)
	target := newNpcWithType(aiNpcTargetType, 7)
	n.target = target

	n.fireAiApTriggerNpc(s, target)

	if string(n.sayText) != "by-acting" {
		t.Errorf("sayText = %q, want %q (ap-band lookup must key on acting npc type %d)",
			n.sayText, "by-acting", aiActingType)
	}
}
