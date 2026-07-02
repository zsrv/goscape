package world

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/zsrv/goscape/pkg/cache"
	"github.com/zsrv/goscape/pkg/dskit/services"
	"github.com/zsrv/goscape/pkg/dskit/signals"
	"github.com/zsrv/goscape/pkg/tapper"
)

// TODO: tracer

// World represents an instance of a game world.
type World struct {
	services.Service
	log                *slog.Logger
	subservices        *services.Manager
	subservicesWatcher *services.FailureWatcher
	Server             *Server
	loginClient        LoginClient
	friendsClient      FriendsClient
	cfg                Config
}

func New(cfg Config, logger *slog.Logger, tap tapper.Tapper) (*World, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	//subservices := []services.Service(nil)

	w := &World{
		cfg: cfg,
		log: logger.With("component", compWorld),
	}

	// NOTE: World server doesn't have any subservices
	//var err error
	//w.subservices, err = services.NewManager(subservices...)
	//if err != nil {
	//	return nil, fmt.Errorf("failed to create subservices: %w", err)
	//}
	//w.subservicesWatcher = services.NewFailureWatcher()
	//w.subservicesWatcher.WatchManager(w.subservices)

	handler := cfg.SignalHandler
	if handler == nil {
		handler = signals.NewHandler(logger)
	}

	var loginClient LoginClient
	if cfg.LoginServerEnabled {
		lc, err := NewLoginClient(cfg.LoginServerAddress, logger.With("component", compLogin))
		if err != nil {
			// Log the error but don't fail startup — the world should run even if login is unreachable.
			w.log.Warn("failed to create login client", slog.Any("err", err))
		} else {
			loginClient = lc
		}
	}
	w.loginClient = loginClient

	var friendsClient FriendsClient
	if cfg.FriendsServerEnabled {
		fc, err := NewFriendsClient(cfg.FriendsServerAddress, logger.With("component", compFriends))
		if err != nil {
			w.log.Warn("failed to create friends client", slog.Any("err", err))
		} else {
			friendsClient = fc
		}
	}
	w.friendsClient = friendsClient

	server, err := NewServer(cfg, loginClient, friendsClient, logger, tap)
	if err != nil {
		return nil, fmt.Errorf("failed to create server: %w", err)
	}
	w.Server = server

	return w, nil
}

// GetLoginClient returns the LoginClient for this world (may be nil if disabled).
func (w *World) GetLoginClient() LoginClient { return w.loginClient }

// GetFriendsClient returns the FriendsClient for this world (may be nil if disabled).
func (w *World) GetFriendsClient() FriendsClient { return w.friendsClient }

// terminationWaiter is the minimal surface worldServiceFns needs from a
// dependency service in stoppingFn — narrowed from services.Service so
// unit tests can inject fakes without implementing the full Service
// interface.
type terminationWaiter interface {
	AwaitTerminated(ctx context.Context) error
}

// svcFns bundles the three services.BasicService closures produced by
// worldServiceFns.
type svcFns struct {
	starting func(ctx context.Context) error
	run      func(ctx context.Context) error
	stopping func(err error) error
}

// worldServiceFns builds the three services.BasicService closures for the
// world service from injectable primitives, so the seam below is
// unit-testable without a real *Server, LoginClient, or FriendsClient
// (arch-28.4c).
//
// run blocks until the server stops; shutdown stops it; gracefulExit
// reports whether the stop was an intentional ::reboot/::slowreboot vs. an
// unexpected exit; lcClose/fcClose are the login/friends client Close
// funcs (nil when that client is disabled); startingBody is the slow
// startup work (CRC compute, WorldStartup/WorldConnect RPCs, content-watch
// wiring) that must complete before Run can usefully proceed;
// servicesToWaitFor lists the dependency services stoppingFn must await
// before shutting the server down.
func worldServiceFns(
	run func() error,
	shutdown func(),
	gracefulExit func() bool,
	lcClose func() error,
	fcClose func() error,
	startingBody func(ctx context.Context) error,
	servicesToWaitFor func() []terminationWaiter,
	log *slog.Logger,
) svcFns {
	serverDone := make(chan error, 1)

	starting := func(ctx context.Context) error {
		if err := startingBody(ctx); err != nil {
			return err
		}
		// Spawn Run here, not in runFn: BasicService legally runs stoppingFn
		// WITHOUT runFn (service context canceled between Starting and
		// Running — reachable because this startingFn does slow work: CRC
		// compute + WorldStartup/WorldConnect RPCs). stoppingFn blocks on
		// <-serverDone, so the goroutine that feeds serverDone must be alive
		// by the time startingFn returns nil (arch-28.4c).
		go func() {
			defer close(serverDone)
			serverDone <- run()
		}()
		return nil
	}

	runFn := func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return nil
		case err := <-serverDone:
			if err != nil {
				return err
			}
			if gracefulExit() {
				return nil // NAI-182 — ::reboot / ::slowreboot graceful exit
			}
			return fmt.Errorf("server stopped unexpectedly")
		}
	}

	stopping := func(_ error) error {
		// wait until all modules are done, and then shut the server down
		for _, s := range servicesToWaitFor() {
			_ = s.AwaitTerminated(context.Background())
		}

		// shut the TCP server down (this also unblocks Run)
		shutdown()

		// if not closed yet, wait until the server stops
		<-serverDone
		log.Info("world server stopped")

		if lcClose != nil {
			if err := lcClose(); err != nil {
				log.Warn("failed to close login client", slog.Any("err", err))
			}
		}
		if fcClose != nil {
			if err := fcClose(); err != nil {
				log.Warn("failed to close friends client", slog.Any("err", err))
			}
		}
		return nil
	}

	return svcFns{starting: starting, run: runFn, stopping: stopping}
}

// NewWorldService constructs a services.Service from a Server component.
// The Server should not react to signals. Early return from Run function
// is considered to be an error.
func NewWorldService(serv *Server, lc LoginClient, fc FriendsClient, servicesToWaitFor func() []services.Service) services.Service {
	startingBody := func(ctx context.Context) error {
		cachePath := serv.cfg.CachePath
		cache.MakeCRCs(cachePath)
		// arch-29.3: WorldStartup/WorldConnect are idempotent registration
		// calls (the former also clears stale account_login.logged_in rows
		// from an ungraceful shutdown). Retry them in the background on
		// serv.bridgesCtx instead of making one blocking attempt on the
		// startingFn ctx — a failed attempt at boot (e.g. login mid-restart)
		// must not strand crashed-out players at ALREADY_LOGGED_IN, nor
		// block boot on the ~20s gRPC min-connect timeout.
		if lc != nil {
			serv.retryBridgeRegistration("login WorldStartup", func(ctx context.Context) error {
				return lc.WorldStartup(ctx, int32(serv.cfg.NodeID), serv.cfg.NodeProfile)
			})
		}
		if fc != nil {
			serv.retryBridgeRegistration("friends WorldConnect", func(ctx context.Context) error {
				return fc.WorldConnect(ctx, int32(serv.cfg.NodeID), serv.cfg.NodeProfile)
			})
		}
		// NAI-REBUILD-ASYNC: spawn long-lived pack worker + optional
		// fsnotify watcher when ContentPath is configured. Both exit
		// when serv.quit closes (via stoppingFn → Shutdown).
		if serv.cfg.ContentPath != "" {
			go serv.runRebuildWorker()
			if serv.cfg.ContentWatch {
				go serv.runContentWatcher()
			}
		} else if serv.cfg.ContentWatch {
			serv.log.Warn("world: --world.content-watch is set but --world.content-path is empty; auto-rebuild disabled")
		}
		return nil
	}

	waitFor := func() []terminationWaiter {
		ss := servicesToWaitFor()
		out := make([]terminationWaiter, len(ss))
		for i, s := range ss {
			out[i] = s
		}
		return out
	}

	var lcClose, fcClose func() error
	if lc != nil {
		lcClose = lc.Close
	}
	if fc != nil {
		fcClose = fc.Close
	}

	fns := worldServiceFns(
		serv.Run,
		serv.Shutdown,
		func() bool { return serv.shutdownGraceful },
		lcClose,
		fcClose,
		startingBody,
		waitFor,
		serv.log,
	)

	return services.NewBasicService(fns.starting, fns.run, fns.stopping)
}

// DisableSignalHandling puts a dummy signal handler
func DisableSignalHandling(config *Config) {
	config.SignalHandler = make(ignoreSignalHandler)
}

type ignoreSignalHandler chan struct{}

func (dh ignoreSignalHandler) Loop() {
	<-dh
}

func (dh ignoreSignalHandler) Stop() {
	close(dh)
}
