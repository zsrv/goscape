package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
)

// pathing-1 — the runtime MoveRestrict enum (movement_consts.go) must carry the
// same canonical 0..6 numbering as the config decoder (objtype.npctype.go) and
// Engine-TS MoveRestrict.ts, because npc.go casts the raw config byte straight
// across: moveRestrict = MoveRestrict(typ.MoveRestrict). Dropping BLOCKED_NORMAL
// shifted every value >= 2, so any npc with moverestrict >= 2 was misread.

// TestMoveRestrictEnumMatchesCanonical pins that the world enum named constants
// equal the canonical objtype constants 1:1 — the precondition that makes the
// raw-byte cast in npc.go (MoveRestrict(typ.MoveRestrict)) valid.
func TestMoveRestrictEnumMatchesCanonical(t *testing.T) {
	cases := []struct {
		name      string
		world     MoveRestrict
		canonical int
	}{
		{"NORMAL", MoveRestrictNormal, objtype.MoveRestrictNormal},
		{"BLOCKED", MoveRestrictBlocked, objtype.MoveRestrictBlocked},
		{"INDOORS", MoveRestrictIndoors, objtype.MoveRestrictIndoors},
		{"OUTDOORS", MoveRestrictOutdoors, objtype.MoveRestrictOutdoors},
		{"NOMOVE", MoveRestrictNoMove, objtype.MoveRestrictNoMove},
		{"PASSTHRU", MoveRestrictPassthru, objtype.MoveRestrictPassthru},
	}
	for _, c := range cases {
		if int(c.world) != c.canonical {
			t.Errorf("MoveRestrict %s: world enum = %d, canonical = %d (must match for the raw-byte cast in npc.go to be valid)", c.name, int(c.world), c.canonical)
		}
	}
}

// TestNpcMoveRestrictBehavior pins, per raw config value, the CollisionFlag the
// npc imposes (blockWalkFlag) and the pathfinder collision strategy
// (getCollisionStrategy) against the TS contract:
//
//	Npc.blockWalkFlag                  — Npc.ts:381-398
//	PathingEntity.getCollisionStrategy — PathingEntity.ts:558-575
//
// Driven by the RAW config value (objtype canonical constant) so it exercises
// the npc.go cast end-to-end, which is where the shift bug lived.
func TestNpcMoveRestrictBehavior(t *testing.T) {
	normal := collision.TypeNormal
	blocked := collision.TypeBlocked
	los := collision.TypeLineOfSight
	indoors := collision.TypeIndoors
	outdoors := collision.TypeOutdoors

	cases := []struct {
		name      string
		raw       int
		wantFlag  int
		wantStrat *collision.Type // nil => getCollisionStrategy returns nil
	}{
		{"NORMAL", objtype.MoveRestrictNormal, collision.FlagBlockNPCs, &normal},
		{"BLOCKED", objtype.MoveRestrictBlocked, collision.FlagOpen, &blocked},
		{"BLOCKED_NORMAL", objtype.MoveRestrictBlockedNormal, collision.FlagBlockNPCs, &los},
		{"INDOORS", objtype.MoveRestrictIndoors, collision.FlagBlockNPCs, &indoors},
		{"OUTDOORS", objtype.MoveRestrictOutdoors, collision.FlagBlockNPCs, &outdoors},
		{"NOMOVE", objtype.MoveRestrictNoMove, collision.FlagNull, nil},
		{"PASSTHRU", objtype.MoveRestrictPassthru, collision.FlagOpen, &normal},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			typ := &objtype.NpcType{
				ID: 0, DebugName: "mr",
				MoveRestrict: c.raw,
				RespawnRate:  50,
			}
			n := NewNpc(1, 0, 3094, 3106, 0, typ)

			if got := n.blockWalkFlag(); got != c.wantFlag {
				t.Errorf("blockWalkFlag() = %d, want %d", got, c.wantFlag)
			}
			assertCollisionStrategy(t, n.getCollisionStrategy(), c.wantStrat)
		})
	}
}

// TestPlayerMoveRestrictCollisionStrategy pins Player.getCollisionStrategy (its
// own copy of the PathingEntity.ts:558-575 switch), driving moveRestrict by the
// raw canonical value via cast so it catches the same enum shift.
func TestPlayerMoveRestrictCollisionStrategy(t *testing.T) {
	los := collision.TypeLineOfSight
	indoors := collision.TypeIndoors
	outdoors := collision.TypeOutdoors

	cases := []struct {
		name      string
		raw       int
		wantStrat *collision.Type
	}{
		{"BLOCKED_NORMAL", objtype.MoveRestrictBlockedNormal, &los},
		{"INDOORS", objtype.MoveRestrictIndoors, &indoors},
		{"OUTDOORS", objtype.MoveRestrictOutdoors, &outdoors},
		{"NOMOVE", objtype.MoveRestrictNoMove, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := &Player{moveRestrict: MoveRestrict(c.raw)}
			assertCollisionStrategy(t, p.getCollisionStrategy(), c.wantStrat)
		})
	}
}

func assertCollisionStrategy(t *testing.T, got, want *collision.Type) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Errorf("getCollisionStrategy() = %v, want nil", *got)
	case want != nil && got == nil:
		t.Errorf("getCollisionStrategy() = nil, want %v", *want)
	case want != nil && *got != *want:
		t.Errorf("getCollisionStrategy() = %v, want %v", *got, *want)
	}
}
