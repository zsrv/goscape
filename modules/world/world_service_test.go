package world

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/dskit/modules"
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

// TestWorldServiceFnsRunFnOutcomes pins runFn's three post-run branches: an
// error from run() propagates; a graceful ::reboot/::slowreboot exit returns
// modules.ErrStopProcess (so the manager's failure listener tears down every
// sibling module and the process exits — TS World.ts processShutdown calls
// process.exit(0)); and any other run() return is reported as an unexpected
// stop (a plain error, distinct from the ErrStopProcess sentinel).
func TestWorldServiceFnsRunFnOutcomes(t *testing.T) {
	cases := []struct {
		name            string
		runErr          error
		gracefulExit    bool
		wantErr         bool
		wantStopProcess bool
	}{
		{name: "run error propagates", runErr: errors.New("boom"), wantErr: true},
		{name: "graceful exit returns ErrStopProcess", gracefulExit: true, wantErr: true, wantStopProcess: true},
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
			if tc.wantStopProcess && !errors.Is(err, modules.ErrStopProcess) {
				t.Fatalf("run: want modules.ErrStopProcess, got %v", err)
			}
			if !tc.wantStopProcess && errors.Is(err, modules.ErrStopProcess) {
				t.Fatalf("run: unexpected ErrStopProcess, got %v", err)
			}
		})
	}
}
