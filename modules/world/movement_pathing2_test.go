package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
)

// TestResolveMovement_InstantMoveSpeed_SuppressesStep pins pathing-2 (2026-05-28
// fresh-audit MED). TS PathingEntity.processMovement (PathingEntity.ts:134-137)
// early-returns when moveSpeed === MoveSpeed.INSTANT (the teleport-jump state
// set by P_TELEJUMP / RebuildNormal — player_script.go:600,
// login_resync.go:98). Without that gate, a queued waypoint from a prior
// pathToMoveClick is still stepped on the teleport tick, producing an animated
// walk-step inside the same tick as the jump.
//
// Player.updateMovement (TS Player.ts:670-673) resets tempRun in the
// !super.processMovement() branch; we mirror that here. lastTickX/Z must be
// captured BEFORE the gate so the next tick's validateDistanceWalked compares
// against the post-teleport position, not the pre-teleport coords from the
// previous tick (which would re-flag jump=true on the next tick).
func TestResolveMovement_InstantMoveSpeed_SuppressesStep(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.x, p.z, p.level = 3094, 3106, 0
	// A queued waypoint one tile north — would step here if the gate is missing.
	p.waypoints[0] = coordgrid.PackCoord(0, 3094, 3107)
	p.waypointIndex = 0
	// Teleport state: P_TELEJUMP / RebuildNormal set moveSpeed = Instant for
	// the tick they fire. The bridge in resolveMovement preserves it.
	p.moveSpeed = MoveSpeedInstant
	// Poison values the gate must reset (mirror the no-waypoints branch at
	// movement.go:83-89: walkDir/runDir=-1, tempRun=0).
	p.walkDir = 7
	p.runDir = 7
	p.tempRun = 1

	p.resolveMovement()

	if p.waypointIndex != 0 {
		t.Errorf("waypointIndex: got %d, want 0 (Instant gate fires → no step consumed)", p.waypointIndex)
	}
	if p.x != 3094 || p.z != 3106 {
		t.Errorf("position: got (%d,%d), want (3094,3106) (Instant gate fires → no step)", p.x, p.z)
	}
	if p.walkDir != -1 {
		t.Errorf("walkDir: got %d, want -1 (Instant gate fires → cleared)", p.walkDir)
	}
	if p.runDir != -1 {
		t.Errorf("runDir: got %d, want -1 (Instant gate fires → cleared)", p.runDir)
	}
	if p.tempRun != 0 {
		t.Errorf("tempRun: got %d, want 0 (Instant gate fires → mirrors TS Player.ts:670-673 reset)", p.tempRun)
	}
	if p.stepsTaken != 0 {
		t.Errorf("stepsTaken: got %d, want 0 (Instant gate fires → no step)", p.stepsTaken)
	}
	// Gate placement check: lastTickX/Z must be captured BEFORE the new gate
	// (movement.go:79-81) so post-teleport position is the baseline next tick.
	if p.lastTickX != 3094 || p.lastTickZ != 3106 {
		t.Errorf("lastTickX/Z: got (%d,%d), want (3094,3106) (gate fires AFTER lastTick capture)", p.lastTickX, p.lastTickZ)
	}
}
