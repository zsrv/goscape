package ondemand

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/zsrv/goscape/pkg/dskit/middleware"
	"github.com/zsrv/goscape/pkg/dskit/server"
	"github.com/zsrv/goscape/pkg/dskit/services"
	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/world/connhandler"
)

// TODO: tracer

// OnDemand serves assets to game clients.
type OnDemand struct {
	//services.Service

	cfg Config
	log *slog.Logger

	// Subservices manager
	subservices        *services.Manager
	subservicesWatcher *services.FailureWatcher

	Server *server.Server // TODO: mine

	// sourceIPs extracts the client IP from request headers. Mirrors the
	// dskit server's own extractor (see pkg/dskit/server.BuildHTTPMiddleware)
	// so handler-level log lines like the unmatched-path debug log surface
	// the same source IP that the request-logging middleware records.
	sourceIPs *middleware.SourceIPExtractor

	// worldConn is the destination for accepted WebSocket-framed connections
	// (see WebSocketHandler). May be nil when running ondemand-only (no world
	// module wired) — in which case the WebSocket route is not registered.
	worldConn connhandler.ConnHandler

	// cache is the read-only FileStream opened from cfg.CachePath. HTTP
	// handlers call cache.Read under cacheMu because FileStream is not safe
	// for concurrent use. May be nil if CachePath is not configured.
	cache   *filestream.FileStream
	cacheMu sync.Mutex
}

// New constructs the OnDemand module. Called from cmd/goscape/app
// modules.go initOnDemand; the service wrapper is NewOndemandService.
func New(cfg Config, logger *slog.Logger, serv *server.Server, worldConn connhandler.ConnHandler) (*OnDemand, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	//subservices := []services.Service(nil)

	sourceIPs, err := middleware.NewSourceIPs(cfg.Server.LogSourceIPsHeader, cfg.Server.LogSourceIPsRegex, cfg.Server.LogSourceIPsFull)
	if err != nil {
		return nil, fmt.Errorf("failed to configure source IP extractor: %w", err)
	}

	a := &OnDemand{
		cfg: cfg,
		log: logger,

		Server: serv,

		sourceIPs: sourceIPs,
		worldConn: worldConn,
	}

	// Open the read-only FileStream used by the archive HTTP routes (web.ts:65-80
	// at Engine-TS 9aadcec4). The ondemand module opens its own FileStream so it
	// can run as a separate process (--target ondemand) without the world module.
	// CachePath defaults to "./data/pack", matching the world module's flag idiom.
	if cfg.CachePath != "" {
		cache, err := filestream.New(cfg.CachePath, false, true)
		if err != nil {
			return nil, fmt.Errorf("failed to open cache at %s: %w", cfg.CachePath, err)
		}
		a.cache = cache
	}

	// NOTE: OnDemand server doesn't have any subservices
	//var err error
	//a.subservices, err = services.NewManager(subservices...)
	//if err != nil {
	//	return nil, fmt.Errorf("failed to create subservices: %w", err)
	//}
	//a.subservicesWatcher = services.NewFailureWatcher()
	//a.subservicesWatcher.WatchManager(a.subservices)

	//a.Service = services.NewBasicService(a.starting, a.running, a.stopping)
	//a.Service = services.NewBasicService(nil, runFn, stoppingFn)
	return a, nil
}

// Close releases the FileStream opened in New. It is called by the service
// lifecycle stopping function (modules.go) after the HTTP server has shut down.
func (a *OnDemand) Close() error {
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	if a.cache != nil {
		if err := a.cache.Close(); err != nil {
			return err
		}
		a.cache = nil
	}
	return nil
}

// NewOndemandService constructs a services.Service from an OnDemand + its HTTP
// server. The stopping function shuts the HTTP server down, then closes the
// FileStream so file handles are released cleanly. Mirrors the pattern used by
// NewWorldService in modules/world/world.go.
func NewOndemandService(a *OnDemand, serv *server.Server, servicesToWaitFor func() []services.Service) services.Service {
	serverDone := make(chan error, 1)

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
			return fmt.Errorf("ondemand server stopped unexpectedly")
		}
	}

	stoppingFn := func(_ error) error {
		// wait until all modules are done, then shut the HTTP server down
		for _, s := range servicesToWaitFor() {
			_ = s.AwaitTerminated(context.Background())
		}

		// shut the HTTP server down (also unblocks runFn)
		serv.Shutdown()

		// wait until the server goroutine exits
		<-serverDone
		serv.Log.Info("ondemand server stopped")

		// release the FileStream file handles after the HTTP server is done
		// (no in-flight handlers can call a.cache.Read any more)
		if err := a.Close(); err != nil {
			a.log.Warn("failed to close ondemand cache", slog.Any("err", err))
		}
		return nil
	}

	return services.NewBasicService(nil, runFn, stoppingFn)
}
