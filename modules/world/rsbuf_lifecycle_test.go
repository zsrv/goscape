package world

import (
	"testing"
)

// Smoke test for NAI-29 Bundle 4 Task 4.2 — AddPlayer / RemovePlayer
// hooks at login/logout sites. Verifies the round-trip doesn't panic
// and re-add works after remove. Implementation correctness verified
// by reading the diff at (*Server).addPlayer / removePlayer hook sites.
func TestServer_PlayerLifecycleRoundTripSmoke(t *testing.T) {
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	if p.slot < 1 {
		t.Fatalf("addPlayer didn't assign slot (got %d)", p.slot)
	}
	pid := p.slot

	s.removePlayer(p)

	// After removePlayer:
	//  - s.players[slot] should be nil (existing assertion)
	//  - s.rsbuf must not panic on follow-up queries (nil-slot guards in *Buf)
	if s.players[pid] != nil {
		t.Errorf("after removePlayer: s.players[%d] = %v, want nil", pid, s.players[pid])
	}
	// Smoke: query rsbuf — must not panic.
	_ = s.rsbuf.HasPlayer(int32(pid), 99)
	_ = s.rsbuf.GetNpcObservers(0)

	// Re-add at same slot must succeed.
	p2, _ := newTestPlayer(t)
	if err := s.addPlayer(p2); err != nil {
		t.Fatalf("re-add after removePlayer: %v", err)
	}
}
