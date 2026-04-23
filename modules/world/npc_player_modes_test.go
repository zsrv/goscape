package world

// Player-mode-specific targeting behavior tests (PLAYERFOLLOW / PLAYERESCAPE /
// PLAYERFACE / PLAYERFACECLOSE). Cross-mode matrix tests that exercise the
// default branch or the OP/AP bands stay in npc_interaction_test.go.

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
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

// playerModeFixture builds a minimal Server + Npc + Player target ready
// for processMovementInteraction dispatch tests. NPC at (3094, 3106);
// player at same tile (caller should move as needed). Players and NPCs
// are registered in s.grid / s.npcs / s.players. The returned Player has
// p.active = true so Player.IsValid() returns true and validateTarget's
// Gate 4 passes. s.gamemap is NOT wired by default — callers that need
// wall-flag seeding for PLAYERESCAPE add it via
// `s.gamemap = gamemap.New(...)` after calling this helper.
func playerModeFixture(t *testing.T) (*Server, *Npc, *Player) {
	t.Helper()
	s := newServerForScriptTest(t)
	typ := &objtype.NpcType{
		ConfigType:  objtype.ConfigType{ID: 0, DebugName: "test_npc"},
		MaxRange:    10,
		AttackRange: 1,
		WanderRange: 1, // so defaultMode() returns NPCModeWander (not None)
	}
	n := NewNpc(1, 0, 3094, 3106, 0, typ)
	n.server = s
	p := addPlayerToServer(t, s, 1, 3094, 3106, 0)
	p.active = true // required for Player.IsValid() → validateTarget Gate 4
	return s, n, p
}

// TestProcessMovementInteractionDispatchPlayerFace — NAI-13 Task 3.
// PLAYERFACE is a no-op mode (type guard only) — after dispatch, the NPC's
// target MUST still be set (resetDefaults-stub behavior from NAI-11 clears
// target; NAI-13 dispatch must route to playerFaceMode which leaves state
// alone). Mask-wise, the faceEntity bit comes from the earlier
// SetInteraction call, not from playerFaceMode itself.
func TestProcessMovementInteractionDispatchPlayerFace(t *testing.T) {
	s, n, p := playerModeFixture(t)
	p.x, p.z = 3094, 3108 // close enough for validateTarget
	n.SetInteraction(InteractionScript, p, objtype.NPCModePlayerFace, 0)

	n.processMovementInteraction(s)

	if n.target == nil {
		t.Error("target: got nil, want non-nil — PLAYERFACE dispatch must NOT reset (this is the NAI-11 stub behavior)")
	}
	if n.targetOp != objtype.NPCModePlayerFace {
		t.Errorf("targetOp: got %d, want NPCModePlayerFace (%d)", n.targetOp, objtype.NPCModePlayerFace)
	}
	if n.masks&rsbuf.NpcMaskFaceEntity == 0 {
		t.Error("masks & NpcMaskFaceEntity: got 0, want nonzero (was set by SetInteraction earlier)")
	}
}

// TestPlayerFaceNonPlayerTargetLogsAndReturns — NAI-13 Task 3.
// TS throws on type mismatch (Npc.ts:816-818); Go logs + returns.
// Verifies the method does not panic and leaves state unchanged when
// the target is unexpectedly non-Player.
func TestPlayerFaceNonPlayerTargetLogsAndReturns(t *testing.T) {
	s, n, _ := playerModeFixture(t)
	other := newTestNpc(2)
	n.target = other
	n.targetOp = objtype.NPCModePlayerFace

	// Direct method call — not via dispatch.
	n.playerFaceMode(s)

	if n.target != other {
		t.Error("target: mutated on non-Player input — expected log-and-return")
	}
}

// TestPlayerFaceCloseWithinRangeNoops — NAI-13 Task 4.
// Chebyshev distance ≤ 1 → state unchanged, target preserved. TS Npc.ts:826
// inverts this: `> 1 → resetDefaults`.
func TestPlayerFaceCloseWithinRangeNoops(t *testing.T) {
	s, n, p := playerModeFixture(t)
	p.x, p.z = 3095, 3106 // Chebyshev 1
	n.SetInteraction(InteractionScript, p, objtype.NPCModePlayerFaceClose, 0)
	origTarget := n.target

	n.playerFaceCloseMode(s)

	if n.target != origTarget {
		t.Error("target: mutated despite within-range (Chebyshev=1)")
	}
	if n.targetOp != objtype.NPCModePlayerFaceClose {
		t.Errorf("targetOp: got %d, want NPCModePlayerFaceClose", n.targetOp)
	}
}

// TestPlayerFaceCloseBeyondRangeResetsDefaults — NAI-13 Task 4.
// TS Npc.ts:826-828: `distanceTo(this, target) > 1 → resetDefaults`.
func TestPlayerFaceCloseBeyondRangeResetsDefaults(t *testing.T) {
	s, n, p := playerModeFixture(t)
	p.x, p.z = 3096, 3106 // Chebyshev 2
	n.SetInteraction(InteractionScript, p, objtype.NPCModePlayerFaceClose, 0)

	n.playerFaceCloseMode(s)

	if n.target != nil {
		t.Errorf("target: got %v, want nil (resetDefaults should clear target)", n.target)
	}
	if n.targetOp != n.defaultMode() {
		t.Errorf("targetOp: got %d, want defaultMode (%d)", n.targetOp, n.defaultMode())
	}
}

// TestPlayerFaceCloseSymmetricDiagonalQuirk — NAI-13 Task 4.
// Quirk guard: the Chebyshev gate MUST reject targets that exceed on
// BOTH axes simultaneously (dx=2, dz=2). Complements
// TestPlayerFaceCloseBeyondRangeResetsDefaults (dx=2, dz=0) — together
// the two cover the single-axis and symmetric-diagonal branches of the
// `max(|dx|, |dz|) > 1` gate.
func TestPlayerFaceCloseSymmetricDiagonalQuirk(t *testing.T) {
	s, n, p := playerModeFixture(t)
	p.x, p.z = 3096, 3108 // dx=2, dz=2 — symmetric diagonal beyond-range
	n.SetInteraction(InteractionScript, p, objtype.NPCModePlayerFaceClose, 0)

	n.playerFaceCloseMode(s)

	if n.target != nil {
		t.Errorf("target: got %v, want nil — symmetric diagonal dx=dz=2 must be beyond Chebyshev-1 range", n.target)
	}
}

// TestProcessMovementInteractionDispatchPlayerFaceClose — NAI-13 Task 4.
// Player beyond Chebyshev-1 → the mode resetDefaults-clears target.
// Proves the dispatch switch routes to playerFaceCloseMode (not the
// resetDefaults stub — which would also nil target but wouldn't trigger
// the distance-gate logic we just wrote).
func TestProcessMovementInteractionDispatchPlayerFaceClose(t *testing.T) {
	s, n, p := playerModeFixture(t)
	p.x, p.z = 3094, 3109 // Chebyshev 3 — beyond face-close range
	n.SetInteraction(InteractionScript, p, objtype.NPCModePlayerFaceClose, 0)

	n.processMovementInteraction(s)

	if n.target != nil {
		t.Errorf("target: got %v, want nil (playerFaceCloseMode distance gate should fire)", n.target)
	}
}

// TestPlayerFollowQueuesWaypointAtTarget — NAI-13 Task 5.
// TS Npc.ts:801-812: `pathToTarget(); updateMovement()`. Naive-path port
// inherited from NAI-11: pathToTarget queues a single waypoint at the
// player's current tile. The SMART branch is still deferred (NAI-11).
func TestPlayerFollowQueuesWaypointAtTarget(t *testing.T) {
	s, n, p := playerModeFixture(t)
	p.x, p.z = 3100, 3112
	n.SetInteraction(InteractionScript, p, objtype.NPCModePlayerFollow, 0)

	n.playerFollowMode(s)

	// queueWaypoint writes waypoints[0] = coordgrid.PackCoord(level, x, z).
	// Round-trip via UnpackCoord for a clean assertion.
	if n.waypointIndex != 0 {
		t.Fatalf("waypointIndex: got %d, want 0 (waypoint should be queued)", n.waypointIndex)
	}
	pos := coordgrid.UnpackCoord(n.waypoints[0])
	if pos.X != p.x || pos.Z != p.z || pos.Level != p.level {
		t.Errorf("waypoint: got (level=%d, x=%d, z=%d), want (level=%d, x=%d, z=%d)",
			pos.Level, pos.X, pos.Z, p.level, p.x, p.z)
	}
}

// TestPlayerFollowAdvancesOneTile — NAI-13 Task 5.
// Proves updateMovement actually runs (not just pathToTarget). After one
// tick the NPC should be one tile closer to the player (typ.MoveSpeed
// defaults to Instant/Walk = 1 step/tick via NpcType.MoveRestrict).
func TestPlayerFollowAdvancesOneTile(t *testing.T) {
	s, n, p := playerModeFixture(t)
	n.x, n.z = 3094, 3106
	p.x, p.z = 3094, 3112 // +6 Z
	n.SetInteraction(InteractionScript, p, objtype.NPCModePlayerFollow, 0)

	startZ := n.z
	n.playerFollowMode(s)

	if n.z == startZ {
		t.Errorf("z: got %d, want != %d (updateMovement should have stepped)", n.z, startZ)
	}
}

// TestPlayerFollowNonPlayerTargetLogsAndReturns — NAI-13 Task 5.
// Type-guard behavior. TS throws (Npc.ts:804-806); Go logs + returns.
func TestPlayerFollowNonPlayerTargetLogsAndReturns(t *testing.T) {
	s, n, _ := playerModeFixture(t)
	other := newTestNpc(2)
	n.target = other
	n.targetOp = objtype.NPCModePlayerFollow

	n.playerFollowMode(s)

	// No waypoint should have been queued.
	if n.waypointIndex != -1 {
		t.Errorf("waypointIndex: got %d, want -1 (no waypoint on non-Player target)", n.waypointIndex)
	}
	if n.target != other {
		t.Error("target: mutated on non-Player input")
	}
}

// TestProcessMovementInteractionDispatchPlayerFollow — NAI-13 Task 5.
// Proves the dispatch switch now routes PLAYERFOLLOW to playerFollowMode
// (rather than the resetDefaults stub). Observable effect: waypoint
// queued at the player's tile.
func TestProcessMovementInteractionDispatchPlayerFollow(t *testing.T) {
	s, n, p := playerModeFixture(t)
	p.x, p.z = 3099, 3111
	n.SetInteraction(InteractionScript, p, objtype.NPCModePlayerFollow, 0)

	n.processMovementInteraction(s)

	if n.target == nil {
		t.Fatal("target: got nil, want non-nil (PLAYERFOLLOW should preserve target, not resetDefaults-stub)")
	}
	if n.waypointIndex != 0 {
		t.Errorf("waypointIndex: got %d, want 0 (pathToTarget should have queued a waypoint)", n.waypointIndex)
	}
}
