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
