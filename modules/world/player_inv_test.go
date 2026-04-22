package world

import (
	"testing"
)

// TestInvListenOnComRegistersNewListener verifies a fresh call on a
// Player with no prior listeners creates a map entry with the expected
// Type, Com, Source, and FirstSeen=true.
func TestInvListenOnComRegistersNewListener(t *testing.T) {
	p, _ := newTestPlayer(t)

	p.invListenOnCom(93, 149, -1)

	if len(p.invListeners) != 1 {
		t.Fatalf("len: got %d, want 1", len(p.invListeners))
	}
	got, ok := p.invListeners[149]
	if !ok {
		t.Fatal("listener at com=149 should exist")
	}
	if got.Type != 93 {
		t.Errorf("Type: got %d, want 93", got.Type)
	}
	if got.Com != 149 {
		t.Errorf("Com: got %d, want 149", got.Com)
	}
	if got.Source != -1 {
		t.Errorf("Source: got %d, want -1", got.Source)
	}
	if !got.FirstSeen {
		t.Error("FirstSeen should be true for new listener")
	}
}

// TestInvListenOnComReplacesExisting verifies that a second call with
// the same com overwrites the first entry and resets FirstSeen to true,
// matching TS Player.ts:1441-1462 add-or-replace semantics.
func TestInvListenOnComReplacesExisting(t *testing.T) {
	p, _ := newTestPlayer(t)

	p.invListenOnCom(93, 149, -1)
	// Simulate a first-seen emit flipping FirstSeen to false.
	l := p.invListeners[149]
	l.FirstSeen = false
	p.invListeners[149] = l

	// Re-register with a different Type/Source at the same com.
	p.invListenOnCom(100, 149, 2)

	if len(p.invListeners) != 1 {
		t.Fatalf("len: got %d, want 1 (re-register should not add a second entry)", len(p.invListeners))
	}
	got := p.invListeners[149]
	if got.Type != 100 {
		t.Errorf("Type: got %d, want 100 (replace should overwrite)", got.Type)
	}
	if got.Source != 2 {
		t.Errorf("Source: got %d, want 2 (replace should overwrite)", got.Source)
	}
	if !got.FirstSeen {
		t.Error("FirstSeen should reset to true on replace")
	}
}

// TestInvListenOnComLazyInitializesMap verifies that the first call on
// a Player whose invListeners field is nil allocates the map.
func TestInvListenOnComLazyInitializesMap(t *testing.T) {
	p, _ := newTestPlayer(t)
	if p.invListeners != nil {
		t.Fatalf("precondition: invListeners should start nil, got %v", p.invListeners)
	}

	p.invListenOnCom(93, 149, -1)

	if p.invListeners == nil {
		t.Fatal("invListenOnCom should allocate the map on first call")
	}
}

// TestInvStopListenOnComRemovesListener verifies that calling stop on
// a registered com deletes the entry and decreases len by 1.
func TestInvStopListenOnComRemovesListener(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.invListenOnCom(93, 149, -1)
	p.invListenOnCom(100, 200, -1)
	if len(p.invListeners) != 2 {
		t.Fatalf("setup: len should be 2, got %d", len(p.invListeners))
	}

	p.invStopListenOnCom(149)

	if len(p.invListeners) != 1 {
		t.Errorf("len: got %d, want 1", len(p.invListeners))
	}
	if _, ok := p.invListeners[149]; ok {
		t.Error("listener at 149 should be removed")
	}
	if _, ok := p.invListeners[200]; !ok {
		t.Error("listener at 200 should remain")
	}
}

// TestInvStopListenOnComNoopForMissingKey verifies calling stop on a
// com that was never registered is a no-op (does not panic, does not
// mutate map).
func TestInvStopListenOnComNoopForMissingKey(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.invListenOnCom(93, 149, -1)

	p.invStopListenOnCom(999) // never registered

	if len(p.invListeners) != 1 {
		t.Errorf("len: got %d, want 1 (unrelated listener should remain)", len(p.invListeners))
	}
}

// TestInvStopListenOnComNoopForNilMap verifies calling stop on a Player
// whose map is still nil does not panic (Go's delete-on-nil semantic).
func TestInvStopListenOnComNoopForNilMap(t *testing.T) {
	p, _ := newTestPlayer(t)
	if p.invListeners != nil {
		t.Fatalf("precondition: invListeners should start nil")
	}

	p.invStopListenOnCom(149) // must not panic

	if p.invListeners != nil {
		t.Error("stop on nil map should not cause an allocation")
	}
}
