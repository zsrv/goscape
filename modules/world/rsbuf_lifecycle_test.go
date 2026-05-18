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

	s.removePlayerInternal(p)

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

// NAI-150 in-scope-stretch: pins that addNpc(firstSpawn=true) refreshes
// n.uid to match the freshly-allocated n.nid. Pre-fix: production
// spawn site (server.go:312) constructs NewNpc(0, ...) so the
// constructor-computed uid carried slot=0; addNpc allocated a real
// slot ≥1 but never recomputed uid, leaving every spawned NPC with a
// stale slot in its uid. Surfaced by NAI-150 smoke (PROJANIM_NPC
// errored with "invalid npc uid" because npc_uid → slot 0 → s.npcs[0]
// = nil); also silently broke NAI-120's FindNpcByUID.
func TestServer_AddNpc_UidRefreshedAfterSlotAlloc(t *testing.T) {
	s := newTestServer(t)
	// Construct with nid=0 to mirror production server.go:312 NewNpc(0, ...).
	n := newTestNpc(0)
	if err := s.addNpc(n, -1, true); err != nil {
		t.Fatalf("addNpc: %v", err)
	}
	want := (n.typeId << 16) | n.nid
	if n.uid != want {
		t.Errorf("Npc.uid after addNpc(firstSpawn=true): got %d, want %d (typeId=%d, nid=%d)",
			n.uid, want, n.typeId, n.nid)
	}
	// Roundtrip pin: extracting slot from uid must yield n.nid (the
	// inverse of the staleness bug).
	if got := n.uid & 0xffff; got != n.nid {
		t.Errorf("uid slot extraction: got %d, want %d (n.nid)", got, n.nid)
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
