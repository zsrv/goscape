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
