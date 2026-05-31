package world

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/zsrv/goscape/internal/dskit/services"
	"github.com/zsrv/goscape/internal/dskit/signals"
	"github.com/zsrv/goscape/pkg/cache"
	tapper "github.com/zsrv/goscape/pkg/tapper"
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

func New(cfg Config, logger *slog.Logger, capture *tapper.Capture) (*World, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	//subservices := []services.Service(nil)

	w := &World{
		cfg: cfg,
		log: logger,
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
		lc, err := NewLoginClient(cfg.LoginServerAddress, logger)
		if err != nil {
			// Log the error but don't fail startup — the world should run even if login is unreachable.
			logger.Warn("failed to create login client", slog.Any("err", err))
		} else {
			loginClient = lc
		}
	}
	w.loginClient = loginClient

	var friendsClient FriendsClient
	if cfg.FriendsServerEnabled {
		fc, err := NewFriendsClient(cfg.FriendsServerAddress, logger)
		if err != nil {
			logger.Warn("failed to create friends client", slog.Any("err", err))
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

// NewWorldService constructs a services.Service from a Server component.
// The Server should not react to signals. Early return from Run function
// is considered to be an error.
func NewWorldService(serv *Server, lc LoginClient, fc FriendsClient, servicesToWaitFor func() []services.Service) services.Service {
	serverDone := make(chan error, 1)

	startingFn := func(ctx context.Context) error {
		cachePath := serv.cfg.CachePath
		if err := cache.PreloadClient(filepath.Join(cachePath, "client")); err != nil {
			return fmt.Errorf("world: preload client assets: %w", err)
		}
		cache.MakeCRCs(cachePath)
		if lc != nil {
			lc.WorldStartup(ctx, int32(serv.cfg.NodeID), serv.cfg.NodeProfile)
		}
		if fc != nil {
			fc.WorldConnect(ctx, int32(serv.cfg.NodeID), serv.cfg.NodeProfile)
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

	runFn := func(ctx context.Context) error {
		go func() {
			defer close(serverDone)
			serverDone <- serv.Run()
		}()

		select {
		case <-ctx.Done():
			return nil
		case err := <-serverDone:
			if err != nil {
				return err
			}
			if serv.shutdownGraceful {
				return nil // NAI-182 — ::reboot / ::slowreboot graceful exit
			}
			return fmt.Errorf("server stopped unexpectedly")
		}
	}

	stoppingFn := func(_ error) error {
		// wait until all modules are done, and then shut the server down
		for _, s := range servicesToWaitFor() {
			_ = s.AwaitTerminated(context.Background())
		}

		// shut the TCP server down (this also unblocks Run)
		serv.Shutdown()

		// if not closed yet, wait until the server stops
		<-serverDone
		serv.log.Info("world server stopped")

		if lc != nil {
			if err := lc.Close(); err != nil {
				serv.log.Warn("failed to close login client", slog.Any("err", err))
			}
		}
		if fc != nil {
			if err := fc.Close(); err != nil {
				serv.log.Warn("failed to close friends client", slog.Any("err", err))
			}
		}
		return nil
	}

	return services.NewBasicService(startingFn, runFn, stoppingFn)
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
