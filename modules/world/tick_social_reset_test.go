package world

import "testing"

// TestProcessCleanupResetsSocialFlags pins NAI-72: processCleanup must
// reset socialProtect and reportAbuseProtect to false on every player
// each tick. Mirrors TS Player.resetEntity(false) at Player.ts:466-467,
// called from World.ts:1138.
func TestProcessCleanupResetsSocialFlags(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	s.playersMu.Lock()
	s.playerLoop = append(s.playerLoop, p)
	s.playersMu.Unlock()

	p.socialProtect = true
	p.reportAbuseProtect = true

	s.processCleanup()

	if p.socialProtect {
		t.Error("socialProtect: not reset by processCleanup")
	}
	if p.reportAbuseProtect {
		t.Error("reportAbuseProtect: not reset by processCleanup")
	}
}

// TestProcessCleanupPreservesStaffModLevel pins that staffModLevel
// is NOT reset per-tick (it's set once at login per TS World.ts:1895).
func TestProcessCleanupPreservesStaffModLevel(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	s.playersMu.Lock()
	s.playerLoop = append(s.playerLoop, p)
	s.playersMu.Unlock()

	p.staffModLevel = 2

	s.processCleanup()

	if p.staffModLevel != 2 {
		t.Errorf("staffModLevel: got %d after cleanup, want 2 (not reset)", p.staffModLevel)
	}
}
