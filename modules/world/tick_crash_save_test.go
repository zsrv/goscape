package world

import (
	"testing"
	"time"
)

// SEC1 M-1 / DEVIATION SEC1-D2: an unrecovered tick-loop panic fires one
// PlayerAutosave per online player before the process dies. TS cycle()
// exits without saving.
func TestCrashSaveAll_AutosavesEveryPlayer(t *testing.T) {
	s := newTestServer(t)
	fake := newFakeLoginClient()
	s.loginClient = fake
	for _, name := range []string{"alice", "bob"} {
		c, _ := newTestClient(t)
		p := newPlayer(c)
		p.username = name
		if err := s.addPlayer(p); err != nil {
			t.Fatal(err)
		}
	}

	s.crashSaveAll("boom")

	got := map[string]bool{}
	for range 2 {
		select {
		case req := <-fake.autosaveReqs:
			got[req.Username] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for autosaves; got %v", got)
		}
	}
	if !got["alice"] || !got["bob"] {
		t.Fatalf("expected autosave for alice and bob, got %v", got)
	}
}

// runTickLoop must still die (re-panic) after saving — supervisors rely
// on the crash.
func TestRunTickLoop_RepanicsAfterCrashSave(t *testing.T) {
	s := newTestServer(t)
	s.loginClient = newFakeLoginClient()
	s.tickBodyFn = func() { panic("tick boom") }
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("runTickLoop must re-panic after crash save")
		}
	}()
	s.tickRate = time.Millisecond
	s.runTickLoop()
}

// SEC1 M-1 review round 1: crashSaveAll's per-player save must be
// isolated — a single player's panicking Save (e.g. corrupt in-memory
// state, plausibly the very thing that crashed the tick loop) must not
// abort the pass and strand every player behind them unsaved.
func TestCrashSavePlayers_SkipsPanickingPlayerContinues(t *testing.T) {
	s := newTestServer(t)
	fake := newFakeLoginClient()
	s.loginClient = fake

	var alice *Player
	for _, name := range []string{"alice", "bob"} {
		c, _ := newTestClient(t)
		p := newPlayer(c)
		p.username = name
		if err := s.addPlayer(p); err != nil {
			t.Fatal(err)
		}
		if name == "alice" {
			alice = p
		}
	}

	saveFn := func(p *Player) []byte {
		if p == alice {
			panic("corrupt alice state")
		}
		return p.Save(s.invTypes, s.varpTypes)
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("crashSavePlayers must not let a per-player panic escape: %v", r)
			}
		}()
		s.crashSavePlayers(saveFn)
	}()

	select {
	case req := <-fake.autosaveReqs:
		if req.Username != "bob" {
			t.Fatalf("expected bob's autosave to survive alice's panicking save, got %q", req.Username)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for bob's autosave after alice's save panicked")
	}
}

// SEC1 M-1 review round 1: crashSaveAll must actually wait for in-flight
// autosave RPCs to flush (via waitForSaveFlush), not race ahead of them.
// The fake's PlayerAutosave blocks on a gate the test controls; crashSaveAll
// must not return before the gate is released, and must return promptly
// (not merely after playerSaveFlushTimeout) once it is.
func TestCrashSaveAll_WaitsForSaveFlush(t *testing.T) {
	s := newTestServer(t)
	fake := newFakeLoginClient()
	fake.autosaveGate = make(chan struct{})
	s.loginClient = fake

	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.username = "carol"
	if err := s.addPlayer(p); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		s.crashSaveAll("boom")
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("crashSaveAll returned before the in-flight autosave RPC was released")
	case <-time.After(150 * time.Millisecond):
	}

	close(fake.autosaveGate)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("crashSaveAll did not return promptly after the autosave RPC unblocked")
	}
}
