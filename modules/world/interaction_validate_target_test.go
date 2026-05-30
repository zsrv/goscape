package world

import (
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/objtype"
)

// These tests pin the 2026-05-28 fresh-audit findings interaction-1 (player
// validateTarget omitted the changetype + isValid gates, porting only the
// level gate) and interaction-2 ((*Player).SetInteraction never snapshotted
// targetSubject.typ, so the changetype gate had no reference type for Npc
// targets). TS references: Player.validateTarget @ Player.ts:1186-1198 and
// PathingEntity.setInteraction @ PathingEntity.ts:510-526. The (*Npc) path
// (npc_interaction.go) is the in-codebase reference implementation.

// --- interaction-2: SetInteraction snapshots targetSubject.typ ---

// TestSetInteraction_SnapshotsNpcTyp pins the player counterpart of the NPC
// snapshot (npc_interaction.go:951-960): an Npc target records its typeId so
// validateTarget's changetype gate can detect a mid-interaction morph.
func TestSetInteraction_SnapshotsNpcTyp(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	npc := makeInteractionNpc(t, s, 1, 101, 100, 0)
	npc.typeId = 42

	p.SetInteraction(InteractionEngine, npc, 1, -1)

	if p.targetSubject.typ != 42 {
		t.Errorf("targetSubject.typ: got %d, want 42 (npc.typeId snapshot)", p.targetSubject.typ)
	}
}

// TestSetInteraction_SnapshotsLocTyp pins the Loc snapshot (TS PathingEntity.ts:523).
func TestSetInteraction_SnapshotsLocTyp(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	loc := entitypkg.NewLoc(0, 100, 100, 1, 1, entitypkg.LifecycleForever, 5, 10, 0)

	p.SetInteraction(InteractionEngine, loc, 1, -1)

	if p.targetSubject.typ != loc.Type() {
		t.Errorf("targetSubject.typ: got %d, want %d (loc.Type() snapshot)", p.targetSubject.typ, loc.Type())
	}
}

// TestSetInteraction_SnapshotsObjTyp pins the Obj snapshot (TS PathingEntity.ts:523).
func TestSetInteraction_SnapshotsObjTyp(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	obj := entitypkg.NewObj(0, 100, 100, entitypkg.LifecycleDespawn, 77, 1)

	p.SetInteraction(InteractionEngine, obj, 1, -1)

	if p.targetSubject.typ != 77 {
		t.Errorf("targetSubject.typ: got %d, want 77 (obj.Type snapshot)", p.targetSubject.typ)
	}
}

// TestSetInteraction_NonTypedTargetClearsTyp pins the else-branch (TS
// PathingEntity.ts:525): a non-Npc/Loc/Obj target stamps -1, overwriting any
// leftover typ from a prior interaction (the interaction-2 "leftover typ" bug).
func TestSetInteraction_NonTypedTargetClearsTyp(t *testing.T) {
	s := newTestServer(t)
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	p.targetSubject.typ = 999 // stale residue from a prior Loc/Obj interaction

	p.SetInteraction(InteractionEngine, fakeEntity{x: 100, z: 100, level: 0}, 1, -1)

	if p.targetSubject.typ != -1 {
		t.Errorf("targetSubject.typ: got %d, want -1 (non-typed target)", p.targetSubject.typ)
	}
}

// --- interaction-1: (*Player).validateTarget gates ---

// validateTargetTestPlayer builds a bare player at (100,100,0). validateTarget
// reads only p.target, p.level, p.targetSubject.typ, and p.UID() — no client.
func validateTargetTestPlayer() *Player {
	p := &Player{}
	p.x, p.z, p.level = 100, 100, 0
	return p
}

func TestValidateTarget_DifferentLevel_False(t *testing.T) {
	p := validateTargetTestPlayer()
	npc := NewNpc(1, 7, 100, 100, 1, &objtype.NpcType{}) // level 1, player on 0
	p.target = npc
	p.targetSubject.typ = 7
	if p.validateTarget() {
		t.Error("gate 1: validateTarget should be false for a different-level target")
	}
}

func TestValidateTarget_NpcChangetype_False(t *testing.T) {
	p := validateTargetTestPlayer()
	npc := NewNpc(1, 7, 101, 100, 0, &objtype.NpcType{})
	p.target = npc
	p.targetSubject.typ = 7
	if !p.validateTarget() {
		t.Fatal("precondition: validateTarget should be true while npc.typeId matches the snapshot")
	}
	npc.typeId = 9 // changetype mid-interaction
	if p.validateTarget() {
		t.Error("gate 2: validateTarget should be false after npc changetype (typeId != snapshot)")
	}
}

func TestValidateTarget_LocChangetype_False(t *testing.T) {
	p := validateTargetTestPlayer()
	loc := entitypkg.NewLoc(0, 100, 100, 1, 1, entitypkg.LifecycleForever, 5, 10, 0)
	p.target = loc
	p.targetSubject.typ = loc.Type()
	if !p.validateTarget() {
		t.Fatal("precondition: validateTarget should be true while loc type matches the snapshot")
	}
	p.targetSubject.typ = loc.Type() + 1 // snapshot taken before a changetype
	if p.validateTarget() {
		t.Error("gate 2: validateTarget should be false when targetSubject.typ != loc.Type()")
	}
}

func TestValidateTarget_NpcDead_False(t *testing.T) {
	p := validateTargetTestPlayer()
	npc := NewNpc(1, 7, 101, 100, 0, &objtype.NpcType{})
	npc.dead = true
	p.target = npc
	p.targetSubject.typ = 7
	if p.validateTarget() {
		t.Error("gate 3: validateTarget should be false for a dead npc")
	}
}

func TestValidateTarget_NpcDelayed_False(t *testing.T) {
	p := validateTargetTestPlayer()
	npc := NewNpc(1, 7, 101, 100, 0, &objtype.NpcType{})
	npc.delayed = true
	p.target = npc
	p.targetSubject.typ = 7
	if p.validateTarget() {
		t.Error("gate 3: validateTarget should be false for a delayed npc (TS Npc.isValid)")
	}
}

// TestValidateTarget_ObjPrivateReveal pins gate 3's player-specific value: the
// hash64/UID-keyed private-obj reveal check (TS Obj.isValid(hash64), goscape
// Obj.IsValidFor) — something the NPC path cannot do.
func TestValidateTarget_ObjPrivateReveal(t *testing.T) {
	p := validateTargetTestPlayer()
	p.uid = 555
	obj := entitypkg.NewObj(0, 100, 100, entitypkg.LifecycleDespawn, 42, 1)
	obj.Reveal = 50      // private (reveal > -1)
	obj.ReceiverID = 999 // owned by a different player
	p.target = obj

	if p.validateTarget() {
		t.Error("gate 3: validateTarget should be false for a private obj owned by another player")
	}

	obj.ReceiverID = 555 // now owned by this player
	if !p.validateTarget() {
		t.Error("gate 3: validateTarget should be true once this player is the obj receiver")
	}
}

func TestValidateTarget_ObjDepleted_False(t *testing.T) {
	p := validateTargetTestPlayer()
	obj := entitypkg.NewObj(0, 100, 100, entitypkg.LifecycleDespawn, 42, 1)
	obj.Count = 0 // depleted
	p.target = obj
	if p.validateTarget() {
		t.Error("gate 3: validateTarget should be false for a depleted obj (count < 1)")
	}
}

// TestValidateTarget_PlayerLoggingOut_False pins gate 3's default-case
// delegation to the polymorphic Player.IsValid (loggingOut / visibility).
func TestValidateTarget_PlayerLoggingOut_False(t *testing.T) {
	s := newTestServer(t)
	p := validateTargetTestPlayer()
	target, wait := makeInteractionPlayer(t, s, 101, 100, 0)
	defer wait()
	target.active = true
	p.target = target

	if !p.validateTarget() {
		t.Fatal("precondition: validateTarget should be true for an active, default-visibility player target")
	}

	target.loggingOut = true
	if p.validateTarget() {
		t.Error("gate 3: validateTarget should be false for a logging-out player target")
	}
}

// TestProcessInteraction_Changetype_ClearsInteraction is the wiring proof:
// a far target (cheb=15) with the client routefinder enabled would otherwise
// be PRESERVED (the pre-step interact misses, the post-step head queues a path
// and hasWaypoints suppresses the "I can't reach" clear — see
// TestProcessInteractionRepathsAfterPathExhaustion). With validateTarget wired
// into the pre-step arm, a mid-interaction changetype clears the interaction
// before any pathing. Pre-fix: p.target stays non-nil (test fails).
func TestProcessInteraction_Changetype_ClearsInteraction(t *testing.T) {
	s := newTestServer(t)
	s.cfg.NodeClientRoutefinder = true
	p, wait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer wait()
	npc := makeInteractionNpc(t, s, 1, 115, 100, 0) // cheb=15
	npc.typeId = 7

	p.SetInteraction(InteractionEngine, npc, 1, -1)
	if p.targetSubject.typ != 7 {
		t.Fatalf("precondition: SetInteraction should snapshot targetSubject.typ=7, got %d", p.targetSubject.typ)
	}

	npc.typeId = 9 // changetype mid-interaction

	p.processInteraction()

	if p.target != nil {
		t.Fatal("changetype should clear the interaction via validateTarget gate 2; p.target is non-nil")
	}
}
