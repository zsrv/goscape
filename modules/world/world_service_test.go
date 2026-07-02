package world

import (
	"context"
	"errors"
	"testing"
	"time"
)

// arch-28.4c: BasicService legally runs stoppingFn WITHOUT ever calling
// runFn when the service context is canceled between Starting and Running
// (pkg/dskit/services/basic_service.go ~178-182) — reachable here because
// NewWorldService's startingFn does slow work (CRC compute +
// WorldStartup/WorldConnect RPCs) before returning. stoppingFn blocks on
// <-serverDone, which only runFn's goroutine wrote pre-fix, so that
// interleaving deadlocked forever. The fix spawns the run() goroutine from
// starting (which always executes once startingBody succeeds), not run
// (which may never execute at all). Pre-fix this test hangs on the 5s
// guard because fns.run is deliberately never invoked below.
func TestWorldServiceStoppingWithoutRun(t *testing.T) {
	runCalled := make(chan struct{})
	fns := worldServiceFns(
		func() error { <-runCalled; return nil }, // run: blocks until shutdown fires
		func() { close(runCalled) },              // shutdown: unblocks run
		func() bool { return false },             // gracefulExit
		nil,                                      // lc close (disabled)
		nil,                                      // fc close (disabled)
		func(context.Context) error { return nil }, // starting body (CRC/RPCs stand-in)
		func() []terminationWaiter { return nil },  // servicesToWaitFor
		discardLogger(),
	)

	if err := fns.starting(t.Context()); err != nil {
		t.Fatalf("starting: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- fns.stopping(nil) }() // NOTE: run fn deliberately never invoked

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("stopping: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stoppingFn deadlocked without runFn (pre-fix behavior)")
	}
}

// TestWorldServiceFnsStartingBodyErrorSkipsRun guards against a regression
// where a failing starting body (CRC compute / WorldStartup / WorldConnect
// RPC stand-in) would still spawn the run() goroutine — leaving a dangling
// goroutine nobody will ever unblock, since stoppingFn is only reached via
// a successful starting.
func TestWorldServiceFnsStartingBodyErrorSkipsRun(t *testing.T) {
	wantErr := errors.New("boom")
	runInvoked := false
	fns := worldServiceFns(
		func() error { runInvoked = true; return nil },
		func() {},
		func() bool { return false },
		nil,
		nil,
		func(context.Context) error { return wantErr },
		func() []terminationWaiter { return nil },
		discardLogger(),
	)

	if err := fns.starting(t.Context()); !errors.Is(err, wantErr) {
		t.Fatalf("starting: got %v, want %v", err, wantErr)
	}
	if runInvoked {
		t.Fatal("run must not be invoked when the starting body fails")
	}
}

// TestWorldServiceFnsStartingBodyErrorClosesClients pins the arch-29.8 fix
// wave: BasicService skips stoppingFn entirely when startingFn returns an
// error ("if StartingFn returns error, no other functions are called" —
// pkg/dskit/services/basic_service.go:45), and the bridge client closes
// used to live only in stopping — so a startingBody failure (MakeCRCs,
// Listen, ...) leaked both gRPC connections. The starting closure must now
// close both clients exactly once before propagating the error.
func TestWorldServiceFnsStartingBodyErrorClosesClients(t *testing.T) {
	wantErr := errors.New("boom")
	lcCloses, fcCloses := 0, 0
	fns := worldServiceFns(
		func() error { return nil },
		func() {},
		func() bool { return false },
		func() error { lcCloses++; return nil },
		func() error { fcCloses++; return nil },
		func(context.Context) error { return wantErr },
		func() []terminationWaiter { return nil },
		discardLogger(),
	)

	if err := fns.starting(t.Context()); !errors.Is(err, wantErr) {
		t.Fatalf("starting: got %v, want %v", err, wantErr)
	}
	if lcCloses != 1 || fcCloses != 1 {
		t.Fatalf("close counts after starting failure: lc=%d fc=%d, want 1/1", lcCloses, fcCloses)
	}
	// stoppingFn is never invoked on this path (BasicService contract), so
	// there is no second call site to double-fire the closes.
}

// TestWorldServiceFnsSuccessPathClosesOnlyInStopping pins the exclusive
// counterpart: when starting succeeds, the closes must NOT fire in starting
// — stopping owns them, and fires each exactly once. Together with
// TestWorldServiceFnsStartingBodyErrorClosesClients this covers both sides
// of the state-machine exclusivity that makes a double-close impossible.
func TestWorldServiceFnsSuccessPathClosesOnlyInStopping(t *testing.T) {
	lcCloses, fcCloses := 0, 0
	fns := worldServiceFns(
		func() error { return nil }, // run: returns immediately; serverDone is fed
		func() {},
		func() bool { return true }, // graceful, irrelevant to closes
		func() error { lcCloses++; return nil },
		func() error { fcCloses++; return nil },
		func(context.Context) error { return nil },
		func() []terminationWaiter { return nil },
		discardLogger(),
	)

	if err := fns.starting(t.Context()); err != nil {
		t.Fatalf("starting: %v", err)
	}
	if lcCloses != 0 || fcCloses != 0 {
		t.Fatalf("close counts after successful starting: lc=%d fc=%d, want 0/0", lcCloses, fcCloses)
	}
	if err := fns.stopping(nil); err != nil {
		t.Fatalf("stopping: %v", err)
	}
	if lcCloses != 1 || fcCloses != 1 {
		t.Fatalf("close counts after stopping: lc=%d fc=%d, want 1/1", lcCloses, fcCloses)
	}
}

// TestWorldServiceFnsRunFnOutcomes pins runFn's three post-run branches —
// unchanged from NewWorldService's pre-restructure inline runFn: an error
// from run() propagates, a nil return under a graceful exit yields nil,
// and any other nil return is reported as an unexpected stop.
func TestWorldServiceFnsRunFnOutcomes(t *testing.T) {
	cases := []struct {
		name         string
		runErr       error
		gracefulExit bool
		wantErr      bool
	}{
		{name: "run error propagates", runErr: errors.New("boom"), wantErr: true},
		{name: "graceful exit returns nil", gracefulExit: true, wantErr: false},
		{name: "unexpected stop returns error", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fns := worldServiceFns(
				func() error { return tc.runErr },
				func() {},
				func() bool { return tc.gracefulExit },
				nil,
				nil,
				func(context.Context) error { return nil },
				func() []terminationWaiter { return nil },
				discardLogger(),
			)
			if err := fns.starting(t.Context()); err != nil {
				t.Fatalf("starting: %v", err)
			}
			err := fns.run(t.Context())
			if tc.wantErr && err == nil {
				t.Fatal("run: want error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("run: want nil, got %v", err)
			}
		})
	}
}
