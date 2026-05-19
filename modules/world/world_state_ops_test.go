package world

import (
	"sync/atomic"
	"testing"
)

// TestRelayActionQueue_DrainExecutesOnTick pins that an action enqueued
// via enqueueRelayAction runs exactly once when drainRelayActions is
// invoked, and runs on the caller's goroutine (tick semantics).
func TestRelayActionQueue_DrainExecutesOnTick(t *testing.T) {
	s := newTestServer(t)

	var ran atomic.Int32
	s.enqueueRelayAction(func() { ran.Add(1) })

	if ran.Load() != 0 {
		t.Fatalf("action ran before drain: count=%d", ran.Load())
	}

	s.drainRelayActions()

	if got := ran.Load(); got != 1 {
		t.Fatalf("action did not run on drain: count=%d, want 1", got)
	}

	// Second drain with empty queue must be a no-op (no blocking).
	s.drainRelayActions()
	if got := ran.Load(); got != 1 {
		t.Fatalf("second drain re-ran action: count=%d, want 1", got)
	}
}

// TestRelayActionQueue_DropsOnFull pins that enqueueRelayAction is
// non-blocking and drops the action when the queue is at capacity.
// Mirrors slice-4a NAI-S4A-D-DROP-ON-FULL posture (drop-newest).
func TestRelayActionQueue_DropsOnFull(t *testing.T) {
	s := newTestServer(t)

	// Fill the queue to capacity with no-op closures.
	for i := 0; i < cap(s.relayActionQueue); i++ {
		s.enqueueRelayAction(func() {})
	}

	// The next enqueue must NOT block. If the implementation blocks,
	// the test will hang and fail on test timeout.
	var dropped atomic.Bool
	dropped.Store(true) // assume dropped; flipped to false if executed.
	s.enqueueRelayAction(func() { dropped.Store(false) })

	// Drain everything; only the first cap(queue) closures should run.
	// The over-cap closure was dropped, so dropped stays true.
	s.drainRelayActions()

	if dropped.Load() {
		// Got dropped — correct behavior.
		return
	}
	t.Fatal("over-cap enqueue was NOT dropped — drainRelayActions executed the over-cap closure")
}
