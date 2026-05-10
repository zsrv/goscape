package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// NAI-147 T5 — TS Player.ts:1114 3-part guard
// `!target || !hasInteraction() || !canAccess()`. Pins the new
// short-circuit branch + regression-fences existing happy paths.

func TestTryInteract_FollowOp_ShortCircuits(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	other, otherWait := makeInteractionPlayer(t, s, 101, 100, 0)
	defer otherWait()

	// Follow-op: targetOp=3, target=*Player. isFollowOp returns true,
	// HasInteraction returns false.
	p.SetInteraction(InteractionEngine, other, 3, -1)
	priorApRange := p.apRange

	got := p.tryInteract(false)

	if got {
		t.Errorf("tryInteract: got true, want false (follow-op short-circuit)")
	}
	if p.interactionFired {
		t.Errorf("interactionFired: got true, want false (no dispatch on short-circuit)")
	}
	if p.apRange != priorApRange {
		t.Errorf("apRange: got %d, want %d unchanged (no branch-3 mutation under guard)", p.apRange, priorApRange)
	}
}

func TestTryInteract_NotFollowOp_NotShortCircuited(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	npcType := &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}}
	noopScript := &script.ScriptFile{
		Name:      "[opnpc1,_]",
		LookupKey: script.LookupKeyForType(script.TriggerOpNpc1, 7),
	}
	s.scriptProvider.Register(noopScript)

	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	npc := makeInteractionNpc(t, s, 1, 101, 100, 0) // adjacent — inOperableDistance requires dx or dz > 0
	npc.typ = npcType
	npc.typeId = 7
	p.SetInteraction(InteractionEngine, npc, 1, -1)

	if !p.CanAccess() {
		t.Fatal("test setup invalid: CanAccess should be true")
	}

	got := p.tryInteract(false)
	if !got {
		t.Errorf("tryInteract: got false, want true (non-follow-op proceeds to branch 1)")
	}
	if !p.interacted {
		t.Errorf("interacted: got false, want true (branch 1 fired)")
	}
}

func TestTryInteract_Delayed_ShortCircuits(t *testing.T) {
	s := newTestServer(t)
	s.currentTick = 0
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	npc := makeInteractionNpc(t, s, 1, 101, 100, 0) // adjacent position
	npc.typ = &objtype.NpcType{ConfigType: objtype.ConfigType{ID: npc.typeId}}
	p.SetInteraction(InteractionEngine, npc, 1, -1)
	p.delayed = true
	p.delayedUntil = 1

	got := p.tryInteract(false)
	if got {
		t.Errorf("tryInteract: got true, want false (delayed → !CanAccess → short-circuit)")
	}
	if p.interactionFired {
		t.Errorf("interactionFired: got true, want false (no dispatch on guard short-circuit)")
	}
}

func TestTryInteract_NoTarget_ShortCircuits(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()

	got := p.tryInteract(false)
	if got {
		t.Errorf("tryInteract: got true, want false (no target)")
	}
}

func TestTryInteract_FollowOpDelayed_BothGatesGuard(t *testing.T) {
	s := newTestServer(t)
	s.currentTick = 0
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	other, otherWait := makeInteractionPlayer(t, s, 101, 100, 0)
	defer otherWait()
	p.SetInteraction(InteractionEngine, other, 3, -1)
	p.delayed = true
	p.delayedUntil = 1

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("tryInteract panicked under combined guard: %v", r)
		}
	}()

	got := p.tryInteract(false)
	if got {
		t.Errorf("tryInteract: got true, want false")
	}
}

func TestTryInteract_HasInteractionTrue_ProceedsToBranch1(t *testing.T) {
	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	noopScript := &script.ScriptFile{
		Name:      "[opnpc1,_]",
		LookupKey: script.LookupKeyForType(script.TriggerOpNpc1, 7),
	}
	s.scriptProvider.Register(noopScript)

	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	npc := makeInteractionNpc(t, s, 1, 101, 100, 0) // adjacent — inOperableDistance requires dx or dz > 0
	npc.typ = &objtype.NpcType{ConfigType: objtype.ConfigType{ID: 7}}
	npc.typeId = 7
	p.SetInteraction(InteractionEngine, npc, 1, -1)

	if !p.HasInteraction() {
		t.Fatal("test setup invalid: HasInteraction should be true for non-follow-op")
	}
	if !p.CanAccess() {
		t.Fatal("test setup invalid: CanAccess should be true")
	}

	got := p.tryInteract(false)
	if !got {
		t.Errorf("tryInteract: got false, want true (regression fence — guard must not break happy path)")
	}
}
