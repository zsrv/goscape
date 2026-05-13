//go:build !race

package world

import (
	"testing"
	"time"
)

// TestTickLoopHonoursFieldRate pins NAI-188: mid-loop mutations to
// s.tickRate take effect on the next iteration. Starts at a 3ms
// cadence, runs ~30ms (expects ~10 ticks), mutates s.tickRate to 30ms,
// runs ~60ms (expects ~2 ticks). The post-mutation second-window tick
// delta must be strictly less than the pre-mutation first-window delta.
//
// Build-tagged !race because the test deliberately reads s.currentTick
// and writes s.tickRate from the test goroutine while the tick
// goroutine is running. Spec §6 documents that production mutations
// happen on the tick goroutine (cheat dispatch path) — no atomics
// required. The test simulates the mutation cross-goroutine, which is
// principled for regression detection but not race-clean. Functional
// correctness is verified without the race detector; the cheat-handler
// tests in T3 cover the production code path race-cleanly.
func TestTickLoopHonoursFieldRate(t *testing.T) {
	s := newTestServer(t)

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runTickLoopWithRate(3 * time.Millisecond)
	}()

	time.Sleep(30 * time.Millisecond)
	firstWindow := s.currentTick

	s.tickRate = 30 * time.Millisecond

	time.Sleep(60 * time.Millisecond)
	secondWindow := s.currentTick - firstWindow

	close(s.quit)
	<-done

	if firstWindow < 5 {
		t.Errorf("first window (3ms rate, 30ms): currentTick = %d, want >= 5", firstWindow)
	}
	if secondWindow >= firstWindow {
		t.Errorf("rate mutation did not slow the loop: first window = %d ticks (3ms rate), second window = %d ticks (30ms rate after mutation). Loop is still reading the captured parameter, not s.tickRate.",
			firstWindow, secondWindow)
	}
}
