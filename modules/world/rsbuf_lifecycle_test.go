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

func TestServer_AddNpcWiresRsbufSlot(t *testing.T) {
	s := newTestServer(t)
	n := newTestNpc(50)
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}
	// Smoke: GetNpcObservers must not panic on the new slot.
	if got := s.rsbuf.GetNpcObservers(int32(n.nid)); got != 0 {
		t.Errorf("GetNpcObservers fresh: got %d, want 0", got)
	}
}

func TestServer_RemoveNpcCleansRsbufSlot(t *testing.T) {
	s := newTestServer(t)
	n := newTestNpc(50)
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}
	s.removeNpc(n, -1)
	// Smoke: post-remove queries must not panic.
	if got := s.rsbuf.GetNpcObservers(int32(n.nid)); got != 0 {
		t.Errorf("GetNpcObservers post-remove: got %d, want 0", got)
	}
}
