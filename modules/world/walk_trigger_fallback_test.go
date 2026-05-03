package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
)

// TestProcessWalkTriggerFallback_PlayerSetupFiresWhenNoOpCalled pins
// TS World.ts:638 — PLAYERSETUP fires walktrigger when !opcalled
// && hasWaypoints.
func TestProcessWalkTriggerFallback_PlayerSetupFiresWhenNoOpCalled(t *testing.T) {
	s := newTestServer(t)
	p := newTestPlayerAt(t, s, 1, 3200, 3200, 0)
	s.cfg.NodeWalktriggerSetting = WalkTriggerSettingPlayersetup
	p.opcalled = false
	p.walktrigger = 42
	p.userPath = []int{coordgrid.PackCoord(p.level, p.x+1, p.z)}
	p.pathToMoveClick(p.userPath, !s.cfg.NodeClientRoutefinder)
	if !p.hasWaypoints() {
		t.Fatalf("test precondition: hasWaypoints must be true")
	}

	processWalkTriggerFallback(p)

	if p.walktrigger != -1 {
		t.Errorf("walktrigger: got %d, want -1 (PLAYERSETUP fallback must consume)", p.walktrigger)
	}
}

// TestProcessWalkTriggerFallback_PlayerSetupSkipsWhenOpCalled is the
// absence-pin: with opcalled=true, fallback must NOT fire walktrigger.
func TestProcessWalkTriggerFallback_PlayerSetupSkipsWhenOpCalled(t *testing.T) {
	s := newTestServer(t)
	p := newTestPlayerAt(t, s, 1, 3200, 3200, 0)
	s.cfg.NodeWalktriggerSetting = WalkTriggerSettingPlayersetup
	p.opcalled = true
	p.walktrigger = 42
	p.userPath = []int{coordgrid.PackCoord(p.level, p.x+1, p.z)}
	p.pathToMoveClick(p.userPath, !s.cfg.NodeClientRoutefinder)

	processWalkTriggerFallback(p)

	if p.walktrigger != 42 {
		t.Errorf("walktrigger: got %d, want 42 (opcalled=true must skip)", p.walktrigger)
	}
}

// TestProcessWalkTriggerFallback_PlayerMovementSkipsWalktrigger pins
// TS World.ts:638 — PLAYERMOVEMENT re-paths but does NOT fire walktrigger.
func TestProcessWalkTriggerFallback_PlayerMovementSkipsWalktrigger(t *testing.T) {
	s := newTestServer(t)
	p := newTestPlayerAt(t, s, 1, 3200, 3200, 0)
	s.cfg.NodeWalktriggerSetting = WalkTriggerSettingPlayermovement
	p.walktrigger = 42
	p.userPath = []int{coordgrid.PackCoord(p.level, p.x+1, p.z)}

	processWalkTriggerFallback(p)

	if p.walktrigger != 42 {
		t.Errorf("walktrigger: got %d, want 42 (PLAYERMOVEMENT must NOT fire walktrigger)", p.walktrigger)
	}
}

// TestProcessWalkTriggerFallback_PlayerPacketSkipsBranch pins TS
// World.ts:635 — under PLAYERPACKET the entire fallback is skipped.
func TestProcessWalkTriggerFallback_PlayerPacketSkipsBranch(t *testing.T) {
	s := newTestServer(t)
	p := newTestPlayerAt(t, s, 1, 3200, 3200, 0)
	s.cfg.NodeWalktriggerSetting = WalkTriggerSettingPlayerpacket
	p.walktrigger = 42
	p.userPath = []int{coordgrid.PackCoord(p.level, p.x+1, p.z)}

	processWalkTriggerFallback(p)

	if p.walktrigger != 42 {
		t.Errorf("walktrigger: got %d, want 42 (PLAYERPACKET fallback must skip entirely)", p.walktrigger)
	}
}
