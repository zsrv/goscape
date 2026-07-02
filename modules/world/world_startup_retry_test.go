package world

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// arch-29.3: a failed WorldStartup at boot must retry in the background
// until it succeeds (it is an idempotent UPDATE) instead of stranding
// logged_in=1 rows forever.
func TestRetryBridgeRegistrationRetriesUntilSuccess(t *testing.T) {
	s := newTestServer(t)
	s.bridgeRetryDelay = time.Millisecond
	var calls atomic.Int32
	done := make(chan struct{})
	s.retryBridgeRegistration("login WorldStartup", func(ctx context.Context) error {
		if calls.Add(1) < 3 {
			return errors.New("login restarting")
		}
		close(done)
		return nil
	})
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("registration never succeeded")
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("calls: got %d, want 3", got)
	}
}

func TestRetryBridgeRegistrationStopsOnShutdown(t *testing.T) {
	s := newTestServer(t)
	s.bridgeRetryDelay = time.Millisecond
	var calls atomic.Int32
	s.retryBridgeRegistration("friends WorldConnect", func(ctx context.Context) error {
		calls.Add(1)
		return errors.New("always down")
	})
	time.Sleep(20 * time.Millisecond) // let a few attempts happen
	s.bridgesCancel()
	n := calls.Load()
	time.Sleep(20 * time.Millisecond)
	if calls.Load() > n+1 { // at most one straggler attempt racing the cancel
		t.Fatalf("retry loop kept running after bridgesCancel: %d -> %d", n, calls.Load())
	}
}

// arch-29.3 fix wave: the login gate (worldStartupDone) must open only
// after the WorldStartup registration call has succeeded — i.e. strictly
// after the blanket logged_in wipe inside that RPC — and must stay closed
// while the retry loop is still failing.
func TestRetryBridgeRegistrationOpensLoginGate(t *testing.T) {
	s := newTestServer(t)
	s.bridgeRetryDelay = time.Millisecond
	s.worldStartupDone.Store(false)
	fake := newFakeLoginClient()
	fake.worldStartupErr = errors.New("login restarting")

	s.retryBridgeRegistration("login WorldStartup", s.worldStartupCall(fake))

	// Let several failing attempts happen; the gate must stay closed.
	time.Sleep(10 * time.Millisecond)
	if s.worldStartupDone.Load() {
		t.Fatal("gate opened while WorldStartup was still failing")
	}

	fake.mu.Lock()
	fake.worldStartupErr = nil
	fake.mu.Unlock()

	deadline := time.After(5 * time.Second)
	for !s.worldStartupDone.Load() {
		select {
		case <-deadline:
			t.Fatal("gate never opened after WorldStartup succeeded")
		case <-time.After(time.Millisecond):
		}
	}
}

// arch-29.3 fix wave (reviewer Important): Shutdown must be able to join
// the registration retry goroutines after cancelling bridgesCtx so it
// never returns with live goroutines still running.
func TestRetryBridgeRegistrationShutdownJoins(t *testing.T) {
	s := newTestServer(t)
	s.bridgeRetryDelay = time.Millisecond
	s.retryBridgeRegistration("friends WorldConnect", func(ctx context.Context) error {
		return errors.New("always down")
	})
	time.Sleep(5 * time.Millisecond) // let the loop start spinning
	s.bridgesCancel()
	done := make(chan struct{})
	go func() { s.bridgeWg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("bridgeWg.Wait did not return promptly after bridgesCancel")
	}
}
