package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// TestTargetWithinMaxRangePlayerFollowAlwaysTrue — NAI-13 Task 2.
// Mirrors TS Npc.ts:633-635 where PLAYERFOLLOW returns true unconditionally
// at the top of targetWithinMaxRange. Rationale: PLAYERFOLLOW has no
// retreat-range semantics; the player is free to roam arbitrarily far.
func TestTargetWithinMaxRangePlayerFollowAlwaysTrue(t *testing.T) {
	typ := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 0, DebugName: "follow_npc"},
		MaxRange:    2, // deliberately tiny
		AttackRange: 1,
	}
	n := NewNpc(1, 0, 3094, 3106, 0, typ)
	n.startX, n.startZ = 3094, 3106

	// Player target 100 tiles away on both axes.
	target := &Npc{nid: 2, x: 3194, z: 3206, level: 0, typ: typ}
	n.target = target
	n.targetOp = objtype.NPCModePlayerFollow

	if !n.targetWithinMaxRange() {
		t.Errorf("targetWithinMaxRange: got false, want true (PLAYERFOLLOW must always return true per TS Npc.ts:633-635)")
	}
}

// TestTargetWithinMaxRangePlayerEscapeRetreatBound — NAI-13 Task 2.
// Mirrors TS Npc.ts:657-673: when targetOp is PLAYERESCAPE, the range
// test uses the same Chebyshev `maxAxis > maxrange+1` shape as the
// OP-trigger branch. Tests both pass (at maxrange+1) and fail (at
// maxrange+2) on a single axis.
func TestTargetWithinMaxRangePlayerEscapeRetreatBound(t *testing.T) {
	typ := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 0, DebugName: "escape_npc"},
		MaxRange:    5,
		AttackRange: 1,
	}

	// Case 1: target at +5 on one axis from start → maxAxis = 5 < maxrange+1 = 6 → true.
	n := NewNpc(1, 0, 100, 100, 0, typ)
	n.startX, n.startZ = 100, 100
	t1 := &Npc{nid: 2, x: 105, z: 100, level: 0, typ: typ}
	n.target = t1
	n.targetOp = objtype.NPCModePlayerEscape
	if !n.targetWithinMaxRange() {
		t.Errorf("targetWithinMaxRange (dx=5): got false, want true (within maxrange+1)")
	}

	// Case 2: target at +7 on one axis → maxAxis = 7 > maxrange+1 = 6 → false.
	t2 := &Npc{nid: 3, x: 107, z: 100, level: 0, typ: typ}
	n.target = t2
	if n.targetWithinMaxRange() {
		t.Errorf("targetWithinMaxRange (dx=7): got true, want false (exceeds maxrange+1)")
	}
}

// TestTargetWithinMaxRangePlayerEscapeCornerQuirk — NAI-13 Task 2.
// Mirrors TS Npc.ts:670-672 corner-removal quirk (shared with OP-trigger
// branch at :645-648): when both dx AND dz equal maxrange+1, the target
// is rejected. This excludes the exact diagonal-corner tile of the
// retreat box even though its max-axis value is within maxrange+1.
func TestTargetWithinMaxRangePlayerEscapeCornerQuirk(t *testing.T) {
	typ := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 0, DebugName: "escape_npc"},
		MaxRange:    5,
		AttackRange: 1,
	}
	n := NewNpc(1, 0, 100, 100, 0, typ)
	n.startX, n.startZ = 100, 100
	// Target at (+6, +6) from start: dx = dz = 6 = maxrange+1 → corner-reject.
	target := &Npc{nid: 2, x: 106, z: 106, level: 0, typ: typ}
	n.target = target
	n.targetOp = objtype.NPCModePlayerEscape

	if n.targetWithinMaxRange() {
		t.Errorf("targetWithinMaxRange (dx=dz=maxrange+1): got true, want false (corner-removal quirk per TS :670-672)")
	}
}

// TestTargetWithinMaxRangeOpTriggerUnchanged — NAI-13 Task 2 regression guard.
// Confirms the existing OP-trigger branch at targetWithinMaxRange lines
// 425-435 still fires for OP modes (PLAYERFOLLOW/PLAYERESCAPE must NOT
// capture these). Uses an OP NPC mode and verifies the maxrange+1
// Chebyshev shape still works.
func TestTargetWithinMaxRangeOpTriggerUnchanged(t *testing.T) {
	typ := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 0, DebugName: "op_npc"},
		MaxRange:    5,
		AttackRange: 1,
	}
	n := NewNpc(1, 0, 100, 100, 0, typ)
	n.startX, n.startZ = 100, 100
	target := &Npc{nid: 2, x: 105, z: 100, level: 0, typ: typ} // dx=5
	n.target = target
	n.targetOp = objtype.NPCModeOpNpc1 // OP-trigger band

	if !n.targetWithinMaxRange() {
		t.Errorf("targetWithinMaxRange (OP, dx=5): got false, want true — OP-branch regression")
	}
}
