package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/dskit/modules"
	"github.com/zsrv/goscape/pkg/dskit/services"
	"github.com/zsrv/goscape/pkg/dskit/signals"
)

// discardLogger returns a logger that discards output, suitable for tests
// that don't care about log assertions.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeSignalHandler is a signal handler whose Loop blocks until Stop is
// called (mirrors signals.Handler's quit-channel semantics).
type fakeSignalHandler struct {
	quit chan struct{}
	once sync.Once
}

func newFakeSignalHandler() *fakeSignalHandler {
	return &fakeSignalHandler{quit: make(chan struct{})}
}

func (f *fakeSignalHandler) Loop() { <-f.quit }
func (f *fakeSignalHandler) Stop() { f.once.Do(func() { close(f.quit) }) }
func (f *fakeSignalHandler) Stopped() bool {
	select {
	case <-f.quit:
		return true
	default:
		return false
	}
}

// newAppForTest returns an App with all modules disabled (so initX uses the
// IdleService branch and no network/disk side effects are exercised), plus
// an injected fake signal handler so Run terminates on the test's command.
func newAppForTest(t *testing.T, target string) (*App, *fakeSignalHandler) {
	t.Helper()
	cfg := *NewDefaultConfig()
	cfg.Target = target
	// All Enable=false ⇒ each initX returns NewIdleService(nil, nil).
	cfg.OnDemand.Enable = false
	cfg.Friends.Enable = false
	cfg.Login.Enable = false
	cfg.World.Enable = false

	a, err := New(discardLogger(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	fh := newFakeSignalHandler()
	a.newSignalHandler = func(*slog.Logger) signalHandler { return fh }
	return a, fh
}

// TestOnDemandDependsOnWorld pins the architectural invariant that the ondemand
// module is dependency-ordered after world in the dskit module graph.
// OnDemand's /crc handler reads cache.CRC(), which is populated by
// world.startingFn calling cache.MakeCRCs(). Removing this edge would
// silently regress --target=ondemand (empty buffer served) and introduce a
// startup race in --target=all (SingleBinary). NAI-19 B1 added this edge.
func TestOnDemandDependsOnWorld(t *testing.T) {
	g := &App{}
	if err := g.setupModuleManager(discardLogger()); err != nil {
		t.Fatalf("setupModuleManager: %v", err)
	}
	if g.deps == nil {
		t.Fatal("g.deps not populated by setupModuleManager")
	}
	got := g.deps[OnDemand]
	if !slices.Contains(got, World) {
		t.Errorf("ondemand dependencies = %v, want to include %q", got, World)
	}
}

// TestSetupModuleManager_RegistersAllExpectedModules confirms every named
// target is registered with the manager so the --target flag accepts each.
// COV-1 (Arc 18).
func TestSetupModuleManager_RegistersAllExpectedModules(t *testing.T) {
	g := &App{}
	if err := g.setupModuleManager(discardLogger()); err != nil {
		t.Fatalf("setupModuleManager: %v", err)
	}
	for _, mod := range []string{OnDemand, Friends, Login, World, SingleBinary, "common"} {
		if !g.ModuleManager.IsModuleRegistered(mod) {
			t.Errorf("module %q not registered", mod)
		}
	}
}

// TestSetupModuleManager_CommonIsInvisible confirms common is marked
// UserInvisibleModule (per modules.go) so --target=common emits the
// "internal module" warning at App.Run.
// COV-1 (Arc 18).
func TestSetupModuleManager_CommonIsInvisible(t *testing.T) {
	g := &App{}
	if err := g.setupModuleManager(discardLogger()); err != nil {
		t.Fatalf("setupModuleManager: %v", err)
	}
	if g.ModuleManager.IsUserVisibleModule("common") {
		t.Errorf("common is UserVisible, want invisible")
	}
	// OnDemand / Friends / Login / World / SingleBinary should be visible.
	for _, mod := range []string{OnDemand, Friends, Login, World, SingleBinary} {
		if !g.ModuleManager.IsUserVisibleModule(mod) {
			t.Errorf("module %q is not UserVisible, want visible", mod)
		}
	}
}

// TestSetupModuleManager_DAGTopology pins the exact dependency edges
// declared in modules.go — Arc 11 sanctioned OnDemand:{Common,World};
// Database (task 3, database module) is the migration anchor and sits
// between Common and both DB-using modules: Friends:{Common,Database},
// Login:{Common,Database}.
// Changing
// any edge here is load-bearing and should be a deliberate decision.
// COV-1 (Arc 18).
func TestSetupModuleManager_DAGTopology(t *testing.T) {
	g := &App{}
	if err := g.setupModuleManager(discardLogger()); err != nil {
		t.Fatalf("setupModuleManager: %v", err)
	}
	want := map[string][]string{
		"common":     {},
		Database:     {"common"},
		OnDemand:     {"common", World},
		Friends:      {"common", Database},
		Login:        {"common", Database},
		World:        {"common", Login, Friends},
		SingleBinary: {OnDemand, Friends, Login, World},
	}
	for mod, expected := range want {
		got := g.deps[mod]
		if !slices.Equal(sortedCopy(got), sortedCopy(expected)) {
			t.Errorf("deps[%q] = %v, want %v", mod, got, expected)
		}
	}
}

func sortedCopy(s []string) []string {
	out := slices.Clone(s)
	slices.Sort(out)
	return out
}

// TestApp_New_OnDemand confirms App.New succeeds with --target=ondemand and that
// the resulting ModuleManager recognises that target as a visible module.
// COV-1 (Arc 18).
func TestApp_New_OnDemand(t *testing.T) {
	cfg := *NewDefaultConfig()
	cfg.Target = OnDemand
	a, err := New(discardLogger(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a == nil {
		t.Fatal("New returned nil app")
	}
	if !a.ModuleManager.IsUserVisibleModule(OnDemand) {
		t.Errorf("OnDemand target not user-visible after New")
	}
}

// TestApp_New_World confirms App.New succeeds with --target=world.
// COV-1 (Arc 18).
func TestApp_New_World(t *testing.T) {
	cfg := *NewDefaultConfig()
	cfg.Target = World
	a, err := New(discardLogger(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !a.ModuleManager.IsUserVisibleModule(World) {
		t.Errorf("World target not user-visible after New")
	}
}

// TestApp_New_All confirms App.New succeeds with the default --target=all.
// COV-1 (Arc 18).
func TestApp_New_All(t *testing.T) {
	cfg := *NewDefaultConfig()
	cfg.Target = SingleBinary
	a, err := New(discardLogger(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !a.ModuleManager.IsUserVisibleModule(SingleBinary) {
		t.Errorf("SingleBinary target not user-visible after New")
	}
}

// TestApp_Run_GracefulStop confirms App.Run starts cleanly and returns nil
// after Stop() is invoked. Exercises the happy-path Run loop:
// InitModuleServices → NewManager → StartAsync → AwaitStopped.
//
// arch-29.8: this used to rely on all four production modules being
// disabled (each returning an IdleService) to populate the service map with
// harmless no-op services. Disabled modules now contribute NO service (see
// TestDisabledModulesYieldNoService in modules_disabled_test.go), so an
// all-disabled --target=all now correctly yields zero services and
// NewManager fails fast with "no services" — that degenerate case is not
// what this test is about. A scoped ModuleManager with one explicit idle
// module stands in for "a module that starts, runs, and stops cleanly on
// signal," independent of the disabled-module masquerade this task removes.
// COV-1 (Arc 18).
func TestApp_Run_GracefulStop(t *testing.T) {
	a, fh := newAppForTest(t, "idle")
	mm := modules.NewManager(discardLogger())
	mm.RegisterModule("idle", func() (services.Service, error) {
		return services.NewIdleService(nil, nil), nil
	})
	if err := mm.AddDependency("idle"); err != nil {
		t.Fatalf("AddDependency: %v", err)
	}
	a.ModuleManager = mm

	runDone := make(chan error, 1)
	go func() { runDone <- a.Run() }()

	// Trigger graceful shutdown by closing the signal handler's Loop.
	// Small sleep lets Run reach the AwaitStopped point so the test
	// exercises the full path rather than racing past it.
	time.Sleep(10 * time.Millisecond)
	fh.Stop()

	select {
	case err := <-runDone:
		if err != nil {
			t.Errorf("Run returned %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return within 5s")
	}
}

// TestApp_Run_OnDemandTarget exercises Run with --target=ondemand, confirming
// target selection narrows the service map to just the requested module.
//
// arch-29.8: previously relied on the disabled-module IdleService masquerade
// to populate serviceMap[OnDemand] with a harmless no-op (see
// TestApp_Run_GracefulStop's comment for why that masquerade is gone). Uses
// an explicit stand-in "ondemand" module on a scoped ModuleManager instead
// of exercising the real (disabled-by-default) production initOnDemand.
// COV-1 (Arc 18).
func TestApp_Run_OnDemandTarget(t *testing.T) {
	a, fh := newAppForTest(t, OnDemand)
	mm := modules.NewManager(discardLogger())
	mm.RegisterModule(OnDemand, func() (services.Service, error) {
		return services.NewIdleService(nil, nil), nil
	})
	if err := mm.AddDependency(OnDemand); err != nil {
		t.Fatalf("AddDependency: %v", err)
	}
	a.ModuleManager = mm

	runDone := make(chan error, 1)
	go func() { runDone <- a.Run() }()
	time.Sleep(10 * time.Millisecond)
	fh.Stop()

	select {
	case err := <-runDone:
		if err != nil {
			t.Errorf("Run returned %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return within 5s")
	}

	if _, ok := a.serviceMap[OnDemand]; !ok {
		t.Errorf("serviceMap missing OnDemand")
	}
}

// TestApp_Run_UnknownTarget confirms App.Run returns a wrapped error when
// the requested target is not registered — InitModuleServices fails fast.
// COV-1 (Arc 18).
func TestApp_Run_UnknownTarget(t *testing.T) {
	a, _ := newAppForTest(t, "nonexistent-module")
	err := a.Run()
	if err == nil {
		t.Fatal("Run returned nil, want unrecognised-module error")
	}
	if !contains(err.Error(), "failed to init module services") {
		t.Errorf("Run err = %v, want wrapped init-services error", err)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// TestApp_Run_ModuleStartFailure swaps in a fresh ModuleManager that
// registers a failing module so App.Run's serviceFailed listener fires
// and StopAsync cascades, causing Run to return after FailureWatcher
// reports the failure. The wrapped manager's AwaitStopped intentionally
// returns nil on a clean stop (even one triggered by a failure-driven
// StopAsync), so we assert termination + bounded latency, not a non-nil
// return value.
// COV-1 (Arc 18).
func TestApp_Run_ModuleStartFailure(t *testing.T) {
	a, fh := newAppForTest(t, "failing")
	mm := modules.NewManager(discardLogger())
	mm.RegisterModule("failing", func() (services.Service, error) {
		return services.NewBasicService(
			func(_ context.Context) error { return errors.New("intentional startup failure") },
			func(_ context.Context) error { return nil },
			func(_ error) error { return nil },
		), nil
	})
	if err := mm.AddDependency("failing"); err != nil {
		t.Fatalf("AddDependency: %v", err)
	}
	a.ModuleManager = mm

	runDone := make(chan error, 1)
	go func() { runDone <- a.Run() }()

	select {
	case <-runDone:
		// Run returned: serviceFailed listener fired, StopAsync was
		// invoked, AwaitStopped released. Either nil or non-nil return
		// is acceptable here — what we're pinning is that App.Run does
		// not hang when a module fails to start.
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return within 5s after module start failure")
	}
	fh.Stop() // belt-and-braces
}

// TestApp_Run_ModuleRuntimeFailure confirms a module that errors during
// runFn (after StartAsync) triggers serviceFailed and Run terminates.
// COV-1 (Arc 18).
func TestApp_Run_ModuleRuntimeFailure(t *testing.T) {
	a, fh := newAppForTest(t, "failing")
	mm := modules.NewManager(discardLogger())
	mm.RegisterModule("failing", func() (services.Service, error) {
		return services.NewBasicService(
			nil,
			func(_ context.Context) error {
				return fmt.Errorf("intentional runtime failure")
			},
			nil,
		), nil
	})
	if err := mm.AddDependency("failing"); err != nil {
		t.Fatalf("AddDependency: %v", err)
	}
	a.ModuleManager = mm

	runDone := make(chan error, 1)
	go func() { runDone <- a.Run() }()

	select {
	case <-runDone:
		// Termination is the assertion; see TestApp_Run_ModuleStartFailure.
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return within 5s after module runtime failure")
	}
	fh.Stop()
}

// TestApp_Stop_PanicsBeforeRun confirms Stop() panics when called before
// Run() (signalsHandler is nil). Pinned because the panic is the contract.
// COV-1 (Arc 18).
func TestApp_Stop_PanicsBeforeRun(t *testing.T) {
	a, _ := newAppForTest(t, SingleBinary)
	defer func() {
		if r := recover(); r == nil {
			t.Error("Stop() did not panic when called before Run()")
		}
	}()
	a.Stop()
}

// TestApp_Stop_DispatchesToSignalHandler confirms App.Stop() forwards to
// signalsHandler.Stop. We pre-assign a fake handler (bypassing Run) so we
// can assert dispatch without racing against Run's concurrent write to the
// same field — the production race window already exists in app.go but is
// outside this test's scope.
// COV-1 (Arc 18).
func TestApp_Stop_DispatchesToSignalHandler(t *testing.T) {
	a := &App{}
	fh := newFakeSignalHandler()
	a.signalsHandler = fh
	a.Stop()
	if !fh.Stopped() {
		t.Error("App.Stop() did not invoke signalsHandler.Stop")
	}
}

// TestApp_Stop_DoubleCallIsSafe pins arch-29.8: calling Stop() twice with a
// live (non-nil) handler must not panic. Before signals.Handler.Stop grew
// its own sync.Once, a second Stop() closed an already-closed channel.
// Uses the real *signals.Handler (not the test-package fake, which already
// carried its own Once) so the fix is exercised at the layer it lives in.
func TestApp_Stop_DoubleCallIsSafe(t *testing.T) {
	a := &App{}
	a.signalsHandler = signals.NewHandler(discardLogger())
	a.Stop()
	a.Stop() // must not panic
}
