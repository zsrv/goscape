package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/zsrv/goscape/modules/account"
	"github.com/zsrv/goscape/modules/friends"
	"github.com/zsrv/goscape/modules/hiscore"
	"github.com/zsrv/goscape/modules/login"
	"github.com/zsrv/goscape/modules/ondemand"
	packetcapturemodule "github.com/zsrv/goscape/modules/packetcapture"
	telemetrymodule "github.com/zsrv/goscape/modules/telemetry"
	"github.com/zsrv/goscape/modules/world"
	"github.com/zsrv/goscape/pkg/admin"
	"github.com/zsrv/goscape/pkg/dskit/grpcutil"
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

	telemetry     *telemetrymodule.Telemetry
	packetcapture *packetcapturemodule.Module

	// shutdownRequested is set the moment a stop is requested, before the
	// modules are asked to stop. It is what makes /readyz and the gRPC
	// health service report not-ready during --shutdown-delay, so an
	// orchestrator can drain this pod from its Service endpoints while it
	// is still serving. Read from HTTP and gRPC goroutines, hence atomic.
	shutdownRequested atomic.Bool

	// signalsHandlerMu guards signalsHandler against the Run/Stop race
	// surfaced by COV-1's race detector: Run() writes signalsHandler at
	// line ~125 on the main goroutine, Stop() reads it at line ~150 from
	// whichever caller drives shutdown. Both accesses go through the mutex.
	// R5 (Arc 22). It also guards abortShutdownDelay, which Run publishes
	// alongside the handler and Stop reads.
	signalsHandlerMu sync.Mutex
	signalsHandler   signalHandler
	// abortShutdownDelay cuts the --shutdown-delay wait short. Published by
	// Run; called by a second Stop() and by the service-failure listener,
	// so neither an impatient operator nor a crashed module has to sit out
	// the full delay. Idempotent (sync.OnceFunc).
	abortShutdownDelay func()

	// newSignalHandler constructs the signal handler used by Run. Defaults
	// to signals.NewHandler; tests override to inject a no-op fake.
	// COV-1 (Arc 18): minimal hook for App.Run testability.
	newSignalHandler func(*slog.Logger) signalHandler

	ModuleManager *modules.Manager

	// stateMu guards serviceMap, sm and adminAddr. Run writes all three on
	// the main goroutine while Ready reads the first two from HTTP and gRPC
	// goroutines, so every access goes through this mutex.
	stateMu    sync.Mutex
	serviceMap map[string]services.Service
	sm         *services.Manager
	// adminAddr is the address the admin listener actually bound, published
	// here because a :0 port is only known after the bind. Empty when the
	// listener is disabled or Run has not got that far.
	adminAddr string

	deps map[string][]string
}

// adminAddress returns the address the admin listener bound, or "" when it is
// disabled or not yet bound.
func (g *App) adminAddress() string {
	g.stateMu.Lock()
	defer g.stateMu.Unlock()
	return g.adminAddr
}

// adminShutdownTimeout bounds the graceful stop of the admin listener on the
// way out of Run, so a wedged probe connection cannot delay process exit.
const adminShutdownTimeout = 5 * time.Second

// Ready is the one process-wide readiness definition, served over HTTP by the
// admin listener's /readyz and over the gRPC Health Checking Protocol by every
// gRPC server this process runs. It reports readiness, a short reason when not
// ready, and the state of every module in this target.
//
// The checks are ordered, and the first failure wins:
//
//  1. a stop has been requested — during --shutdown-delay this process is
//     still serving, but it must be drained;
//  2. the service manager is not built yet, or not every module is Running;
//  3. the world (when wired) is not ticking. A world inside its boot grace
//     (world.ErrStarting) counts as NOT ready here: a pod must not receive
//     players before its first tick. Only the legacy ondemand /healthz route
//     treats that case as 200, for backward compatibility.
//
// Modelled on Loki's readyHandler. Safe to call at any time, including before
// and after Run.
func (g *App) Ready(ctx context.Context) (bool, string, map[string]string) {
	g.stateMu.Lock()
	sm := g.sm
	states := make(map[string]string, len(g.serviceMap))
	for name, svc := range g.serviceMap {
		states[name] = svc.State().String()
	}
	g.stateMu.Unlock()

	switch {
	case g.shutdownRequested.Load():
		return false, "shutting down", states
	case sm == nil || !sm.IsHealthy():
		return false, "modules not running", states
	}
	if g.world != nil && g.world.Server != nil {
		if err := g.world.Server.CheckReady(ctx); err != nil {
			return false, "world: " + err.Error(), states
		}
	}
	return true, "", states
}

// registerGRPCHealth adds grpc.health.v1 to every gRPC server this process
// runs, reporting the health of the WHOLE process (the service field of the
// request is ignored, as in dskit). It must be called before the modules
// start, because each of them calls Serve from its running fn and a gRPC
// server accepts no new service registrations once it is serving.
//
// The registration is unconditional: it adds a service to servers that already
// exist and opens no port of its own.
func (g *App) registerGRPCHealth(sm *services.Manager) {
	hc := grpcutil.NewHealthCheckFrom(
		// Upstream deliberately treats Stopping as healthy — a module can
		// still be fully functional while shutting down. Keep that; the
		// shutdown flag below is what drains this process.
		grpcutil.WithManager(sm),
		grpcutil.WithShutdownRequested(&g.shutdownRequested),
	)
	register := func(r grpc.ServiceRegistrar) { grpc_health_v1.RegisterHealthServer(r, hc) }

	if g.login != nil {
		g.login.RegisterGRPCService(register)
	}
	if g.friends != nil {
		g.friends.RegisterGRPCService(register)
	}
	if g.account != nil {
		g.account.RegisterGRPCService(register)
	}
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

	// Start the admin listener BEFORE the modules are initialised, so
	// /healthz answers while a slow cache load is still running and /readyz
	// can say 503 with a reason instead of refusing the connection. The bind
	// is synchronous: a port clash fails the boot here, before any module
	// has acquired a resource.
	adminSrv := admin.NewServer(g.cfg.Admin.Listen, g.Ready)
	if err := adminSrv.Start(); err != nil {
		return fmt.Errorf("admin listener: %w", err)
	}
	g.stateMu.Lock()
	g.adminAddr = adminSrv.Addr()
	g.stateMu.Unlock()
	if addr := adminSrv.Addr(); addr != "" {
		g.logger.Info("admin listener started", "addr", addr, "liveness", "GET /healthz", "readiness", "GET /readyz")
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), adminShutdownTimeout)
		defer cancel()
		if err := adminSrv.Shutdown(ctx); err != nil {
			g.logger.Warn("admin listener did not shut down cleanly", "err", err)
		}
	}()

	serviceMap, err := g.ModuleManager.InitModuleServices(g.cfg.Target)
	if err != nil {
		return fmt.Errorf("failed to init module services: %w", err)
	}
	g.stateMu.Lock()
	g.serviceMap = serviceMap
	g.stateMu.Unlock()

	svcs := []services.Service(nil)
	for _, s := range serviceMap {
		svcs = append(svcs, s)
	}

	sm, err := services.NewManager(svcs...)
	if err != nil {
		return fmt.Errorf("failed to start service manager: %w", err)
	}
	g.stateMu.Lock()
	g.sm = sm
	g.stateMu.Unlock()

	// Register grpc.health.v1 on every gRPC-serving module AFTER the manager
	// exists (the health check reports its state) and BEFORE StartAsync:
	// every goscape module calls Serve from its running fn, and a serving
	// gRPC server accepts no further registrations.
	g.registerGRPCHealth(sm)

	// stopNow cuts the shutdown delay short; see the signal goroutine below.
	stopNow := make(chan struct{})
	abortShutdownDelay := sync.OnceFunc(func() { close(stopNow) })

	// listen for events from this manager and log them
	healthy := func() { g.logger.Info("goscape started") }
	stopped := func() { g.logger.Info("goscape stopped") }
	serviceFailed := func(service services.Service) {
		// if any service fails, stop everything
		sm.StopAsync()
		// Nothing is left to drain, so don't sit out the shutdown delay.
		abortShutdownDelay()

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
	g.abortShutdownDelay = abortShutdownDelay
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

		// A stop has been requested: /readyz and the gRPC health service
		// report not-ready from here on, whether or not a delay follows.
		g.shutdownRequested.Store(true)

		if g.cfg.ShutdownDelay > 0 {
			g.logger.Info("waiting before shutting down services", "shutdown_delay", g.cfg.ShutdownDelay)
			timer := time.NewTimer(g.cfg.ShutdownDelay)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-stopNow: // a second Stop, or a module that failed meanwhile
			}
		}

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

// Stop the app. It panics if the app is not running. Calling it a second time
// while a --shutdown-delay is running cuts the delay short: the first stop has
// already flipped every probe to not-ready, so a caller asking again wants out
// now.
func (g *App) Stop() {
	g.signalsHandlerMu.Lock()
	h := g.signalsHandler
	abort := g.abortShutdownDelay
	g.signalsHandlerMu.Unlock()
	if h == nil {
		panic("app is not running")
	}
	if abort != nil && g.shutdownRequested.Load() {
		abort()
	}
	h.Stop()
}
