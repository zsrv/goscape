package world

// Player-mode-specific targeting behavior tests (PLAYERFOLLOW / PLAYERESCAPE /
// PLAYERFACE / PLAYERFACECLOSE). Cross-mode matrix tests that exercise the
// default branch or the OP/AP bands stay in npc_interaction_test.go.

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

// TestTargetWithinMaxRangePlayerEscapeRejectsOnlyWhenBothExceed — NAI-13 Task 2.
// Mirrors TS Npc.ts:657-673: PLAYERESCAPE rejects only when BOTH the NPC's
// and the target's distance-from-start exceed maxrange. Threshold is `>
// maxrange` (strict, no +1, no corner quirk). This lets the NPC flee away
// from start while the target is still inside the retreat box, and vice
// versa.
func TestTargetWithinMaxRangePlayerEscapeRejectsOnlyWhenBothExceed(t *testing.T) {
	typ := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 0, DebugName: "escape_npc"},
		MaxRange:    5,
		AttackRange: 1,
	}
	n := NewNpc(1, 0, 100, 100, 0, typ)
	n.startX, n.startZ = 100, 100
	// NPC at (107, 100) — distanceToEscape = 7 > maxrange (5).
	n.x, n.z = 107, 100
	// Target at (108, 100) — targetDistanceFromStart = 8 > maxrange (5).
	target := &Npc{nid: 2, x: 108, z: 100, level: 0, typ: typ}
	n.target = target
	n.targetOp = objtype.NPCModePlayerEscape

	if n.targetWithinMaxRange() {
		t.Errorf("targetWithinMaxRange: got true, want false — BOTH NPC(d=7) AND target(d=8) exceed maxrange=5")
	}
}

// TestTargetWithinMaxRangePlayerEscapeAllowsWhenOnlyTargetExceeds — NAI-13 Task 2.
// TS :671: `targetDistanceFromStart > maxrange && distanceToEscape >
// maxrange`. AND-gated — if the NPC is still inside its retreat box,
// validateTarget lets the interaction continue even though the target
// drifted outside. This is the critical semantic difference vs. OP/default
// branches which reject on EITHER side exceeding.
func TestTargetWithinMaxRangePlayerEscapeAllowsWhenOnlyTargetExceeds(t *testing.T) {
	typ := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 0, DebugName: "escape_npc"},
		MaxRange:    5,
		AttackRange: 1,
	}
	n := NewNpc(1, 0, 100, 100, 0, typ)
	n.startX, n.startZ = 100, 100
	n.x, n.z = 100, 100 // NPC on its spawn tile → distanceToEscape = 0 (not > 5)
	// Target at (108, 100) — targetDistanceFromStart = 8 > maxrange (5).
	target := &Npc{nid: 2, x: 108, z: 100, level: 0, typ: typ}
	n.target = target
	n.targetOp = objtype.NPCModePlayerEscape

	if !n.targetWithinMaxRange() {
		t.Errorf("targetWithinMaxRange: got false, want true — only target exceeds; NPC still in retreat box")
	}
}

// TestTargetWithinMaxRangePlayerEscapeAllowsWhenOnlyNpcExceeds — NAI-13 Task 2.
// Mirror of the above: NPC has fled outside the retreat box but the target
// is still nearby. AND-gate keeps the interaction alive.
func TestTargetWithinMaxRangePlayerEscapeAllowsWhenOnlyNpcExceeds(t *testing.T) {
	typ := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 0, DebugName: "escape_npc"},
		MaxRange:    5,
		AttackRange: 1,
	}
	n := NewNpc(1, 0, 100, 100, 0, typ)
	n.startX, n.startZ = 100, 100
	n.x, n.z = 107, 100 // NPC fled 7 tiles → distanceToEscape = 7 > 5
	// Target at (102, 100) — targetDistanceFromStart = 2 (not > 5)
	target := &Npc{nid: 2, x: 102, z: 100, level: 0, typ: typ}
	n.target = target
	n.targetOp = objtype.NPCModePlayerEscape

	if !n.targetWithinMaxRange() {
		t.Errorf("targetWithinMaxRange: got false, want true — only NPC exceeds; target still in retreat box")
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
