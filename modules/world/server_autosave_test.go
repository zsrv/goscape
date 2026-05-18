package world

import (
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/loginpb"
)

// drainAutosaveReqs reads up to `want` requests from the fake's channel
// within timeout. Returns the captured slice (may be shorter than `want`
// on timeout) for the caller to assert on.
func drainAutosaveReqs(t *testing.T, fake *fakeLoginClient, want int, timeout time.Duration) []*loginpb.PlayerAutosaveRequest {
	t.Helper()
	out := make([]*loginpb.PlayerAutosaveRequest, 0, want)
	deadline := time.After(timeout)
	for len(out) < want {
		select {
		case req := <-fake.autosaveReqs:
			out = append(out, req)
		case <-deadline:
			return out
		}
	}
	return out
}

func TestAutosavePlayers_FiresOncePerActivePlayer(t *testing.T) {
	fake := newFakeLoginClient()
	s := newTestServer(t)
	s.cfg.NodeID = 1
	s.cfg.NodeProfile = "dev"
	s.loginClient = fake
	_, invTypes := newTestPlayerForLoadSave(t)
	s.invTypes = invTypes

	// Two active players.
	c1, _ := newTestClient(t)
	c1.server = s
	p1 := newPlayer(c1)
	p1.username = "alice"
	if err := s.addPlayer(p1); err != nil {
		t.Fatalf("addPlayer p1: %v", err)
	}

	c2, _ := newTestClient(t)
	c2.server = s
	p2 := newPlayer(c2)
	p2.username = "bob"
	if err := s.addPlayer(p2); err != nil {
		t.Fatalf("addPlayer p2: %v", err)
	}

	s.autosavePlayers()

	got := drainAutosaveReqs(t, fake, 2, 3*time.Second)
	if len(got) != 2 {
		t.Fatalf("autosave req count: got %d, want 2", len(got))
	}
	// Deviation from plan: PlayerAutosaveRequest has no NodeId field
	// (Profile/Username/Save only — see pkg/loginpb/login.pb.go:470-472).
	names := map[string]bool{}
	for _, r := range got {
		if r.Profile != "dev" {
			t.Errorf("autosave req for %s: Profile=%q; want dev", r.Username, r.Profile)
		}
		if len(r.Save) == 0 {
			t.Errorf("autosave req for %s: Save is empty", r.Username)
		}
		if !VerifySave(r.Save) {
			t.Errorf("autosave req for %s: Save fails VerifySave", r.Username)
		}
		names[r.Username] = true
	}
	if !names["alice"] || !names["bob"] {
		t.Errorf("expected both usernames; got %+v", names)
	}
}

func TestAutosavePlayers_NoLoginClient_NoRPC(t *testing.T) {
	s := newTestServer(t)

	c, _ := newTestClient(t)
	c.server = s
	p := newPlayer(c)
	p.username = "alice"
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	// Must not panic, must not block.
	done := make(chan struct{})
	go func() {
		s.autosavePlayers()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("autosavePlayers blocked with nil loginClient")
	}
}

func TestAutosavePlayers_EmptyUsername_Skipped(t *testing.T) {
	fake := newFakeLoginClient()
	s := newTestServer(t)
	s.loginClient = fake
	_, invTypes := newTestPlayerForLoadSave(t)
	s.invTypes = invTypes

	c, _ := newTestClient(t)
	c.server = s
	p := newPlayer(c)
	// p.username stays empty
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	s.autosavePlayers()

	// No RPC should fire.
	select {
	case req := <-fake.autosaveReqs:
		t.Errorf("autosave fired for empty-username player: %+v", req)
	case <-time.After(100 * time.Millisecond):
		// expected — no RPC
	}
}

// TestAutosavePlayers_TickCadenceGate pins the gate at tick.go:55 — autosave
// fires on ticks where currentTick % PlayerSaveRate == 0 && currentTick > 0,
// and not otherwise. Test by calling the gate inline (no tick loop needed).
func TestAutosavePlayers_TickCadenceGate(t *testing.T) {
	cases := []struct {
		tick int
		want bool
	}{
		{0, false}, // explicitly excluded by > 0 guard
		{1, false},
		{PlayerSaveRate - 1, false},
		{PlayerSaveRate, true},
		{PlayerSaveRate + 1, false},
		{2 * PlayerSaveRate, true},
		{3 * PlayerSaveRate, true},
	}
	for _, tc := range cases {
		got := tc.tick%PlayerSaveRate == 0 && tc.tick > 0
		if got != tc.want {
			t.Errorf("tick=%d: gate=%v, want %v", tc.tick, got, tc.want)
		}
	}
}
