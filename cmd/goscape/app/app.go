package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/zsrv/goscape/modules/account"
	"github.com/zsrv/goscape/modules/friends"
	"github.com/zsrv/goscape/modules/hiscore"
	"github.com/zsrv/goscape/modules/login"
	"github.com/zsrv/goscape/modules/ondemand"
	"github.com/zsrv/goscape/modules/world"
	"github.com/zsrv/goscape/pkg/dskit/modules"
	"github.com/zsrv/goscape/pkg/dskit/services"
	"github.com/zsrv/goscape/pkg/dskit/signals"
)

// signalHandler is the narrow surface App needs from a signal-handling
// component. Defined locally so tests can inject a fake (e.g. a handler that
// returns immediately from Loop) without pulling in the real OS-signal wiring.
// COV-1 (Arc 18): minimal refactor for App.Run testability.
type signalHandler interface {
	Loop()
	Stop()
}

// App is the root data structure.
type App struct {
	cfg Config

	logger *slog.Logger // my addition; global logger, should only be used for app init, each module should make its own logger!

	ondemand *ondemand.OnDemand
	friends  *friends.Friends
	login    *login.Login
	world    *world.World
	account  *account.Account
	hiscore  *hiscore.Hiscore

	// signalsHandlerMu guards signalsHandler against the Run/Stop race
	// surfaced by COV-1's race detector: Run() writes signalsHandler at
	// line ~125 on the main goroutine, Stop() reads it at line ~150 from
	// whichever caller drives shutdown. Both accesses go through the mutex.
	// R5 (Arc 22).
	signalsHandlerMu sync.Mutex
	signalsHandler   signalHandler

	// newSignalHandler constructs the signal handler used by Run. Defaults
	// to signals.NewHandler; tests override to inject a no-op fake.
	// COV-1 (Arc 18): minimal hook for App.Run testability.
	newSignalHandler func(*slog.Logger) signalHandler

	ModuleManager *modules.Manager
	serviceMap    map[string]services.Service
	deps          map[string][]string
}

// New makes a new app.
func New(logger *slog.Logger, cfg Config) (*App, error) {
	app := &App{
		cfg:    cfg,
		logger: logger,
	}

	if err := app.setupModuleManager(logger); err != nil {
		return nil, fmt.Errorf("failed to set up module manager: %w", err)
	}

	return app, nil
}

// resolveModuleName maps a service back to its module key ("unknown"
// if unregistered) — shared by the failure listener and the post-stop
// exit-code check so their classifications can't drift (arch-29.9).
func resolveModuleName(serviceMap map[string]services.Service, svc services.Service) string {
	for m, s := range serviceMap {
		if s == svc {
			return m
		}
	}
	return "unknown"
}

// isRequestedStop reports whether a FailureCase represents a requested
// shutdown rather than a failure (ErrStopProcess, context.Canceled).
func isRequestedStop(err error) bool {
	return errors.Is(err, modules.ErrStopProcess) || errors.Is(err, context.Canceled)
}

// failedServicesError maps any Failed services back to their module
// names and returns a joined error, or nil if everything stopped
// cleanly. ErrStopProcess (a module requesting shutdown) and
// context.Canceled (normal stop signal) are not failures. Without
// this check App.Run returned AwaitStopped's nil regardless of how
// services ended, so a crashed module exited the process with status
// 0 — invisible to systemd Restart=on-failure and orchestrators.
// (Upstream Loki/Tempo perform the same post-stop inspection.)
func failedServicesError(sm *services.Manager, serviceMap map[string]services.Service) error {
	var errs []error
	for _, s := range sm.ServicesByState()[services.Failed] {
		fc := s.FailureCase()
		if isRequestedStop(fc) {
			continue
		}
		errs = append(errs, fmt.Errorf("module %s failed: %w", resolveModuleName(serviceMap, s), fc))
	}
	return errors.Join(errs...)
}

// Run starts, and blocks until a signal is received or Stop is called.
func (g *App) Run() error {
	if !g.ModuleManager.IsUserVisibleModule(g.cfg.Target) {
		g.logger.Warn("selected target is an internal module, is this intended?", "target", g.cfg.Target)
	}

	serviceMap, err := g.ModuleManager.InitModuleServices(g.cfg.Target)
	if err != nil {
		return fmt.Errorf("failed to init module services: %w", err)
	}
	g.serviceMap = serviceMap

	svcs := []services.Service(nil)
	for _, s := range serviceMap {
		svcs = append(svcs, s)
	}

	sm, err := services.NewManager(svcs...)
	if err != nil {
		return fmt.Errorf("failed to start service manager: %w", err)
	}

	// listen for events from this manager and log them
	healthy := func() { g.logger.Info("goscape started") }
	stopped := func() { g.logger.Info("goscape stopped") }
	serviceFailed := func(service services.Service) {
		// if any service fails, stop everything
		sm.StopAsync()

		m := resolveModuleName(serviceMap, service)
		err := service.FailureCase()
		switch {
		case errors.Is(err, modules.ErrStopProcess):
			g.logger.Info("received stop signal via return error", "module", m, "err", err)
		case isRequestedStop(err):
			// context.Canceled: normal stop signal, nothing to log.
		case err != nil:
			g.logger.Error("module failed", "module", m, "err", err)
		}
	}
	sm.AddListener(services.NewManagerListener(healthy, stopped, serviceFailed))

	// Set up signal handler. If signal arrives, we stop the manager, which stops all the services.
	if g.newSignalHandler == nil {
		g.newSignalHandler = func(l *slog.Logger) signalHandler { return signals.NewHandler(l) }
	}
	handler := g.newSignalHandler(g.logger)
	g.signalsHandlerMu.Lock()
	g.signalsHandler = handler
	g.signalsHandlerMu.Unlock()
	// arch-29.8: guarantee handler.Stop() runs on every exit path out of
	// Run, not just the signal-driven one below. Without this, a non-signal
	// exit (sm.StartAsync failing, AwaitStopped erroring, a module failing
	// before ever receiving a signal, ...) leaves the handler.Loop()
	// goroutine parked forever waiting for a signal that will never come.
	// Wrapped in sync.OnceFunc because Loop may already have returned via a
	// real OS signal by the time this defer runs; signals.Handler.Stop is
	// independently idempotent too (belt-and-braces against an external
	// App.Stop() call racing this defer).
	defer sync.OnceFunc(handler.Stop)()
	go func() {
		handler.Loop()
		sm.StopAsync()
	}()

	// Start all services. This can really only fail if some service is already
	// in a state other than New, which should not be the case.
	err = sm.StartAsync(context.Background())
	if err != nil {
		return fmt.Errorf("failed to start service manager: %w", err)
	}

	if err := sm.AwaitStopped(context.Background()); err != nil {
		return err
	}
	return failedServicesError(sm, serviceMap)
}

// Stop the app. It panics if the app is not running.
func (g *App) Stop() {
	g.signalsHandlerMu.Lock()
	h := g.signalsHandler
	g.signalsHandlerMu.Unlock()
	if h == nil {
		panic("app is not running")
	}
	h.Stop()
}
