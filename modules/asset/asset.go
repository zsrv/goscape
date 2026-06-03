package asset

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/zsrv/goscape/pkg/dskit/middleware"
	"github.com/zsrv/goscape/pkg/dskit/server"
	"github.com/zsrv/goscape/pkg/dskit/services"
	"github.com/zsrv/goscape/pkg/world/connhandler"
)

// TODO: tracer

// Asset serves assets to game clients.
type Asset struct {
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
	// (see WebSocketHandler). May be nil when running asset-only (no world
	// module wired) — in which case the WebSocket route is not registered.
	worldConn connhandler.ConnHandler
}

// TODO: unused - reuse the code for other modules though
func New(cfg Config, logger *slog.Logger, serv *server.Server, worldConn connhandler.ConnHandler) (*Asset, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	//subservices := []services.Service(nil)

	sourceIPs, err := middleware.NewSourceIPs(cfg.Server.LogSourceIPsHeader, cfg.Server.LogSourceIPsRegex, cfg.Server.LogSourceIPsFull)
	if err != nil {
		return nil, fmt.Errorf("failed to configure source IP extractor: %w", err)
	}

	a := &Asset{
		cfg: cfg,
		log: logger,

		Server: serv,

		sourceIPs: sourceIPs,
		worldConn: worldConn,
	}

	// NOTE: Asset server doesn't have any subservices
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

func (a *Asset) starting(ctx context.Context) error {
	// NOTE: Asset server doesn't have any subservices
	// Only report success if all subservices start properly
	//err := services.StartManagerAndAwaitHealthy(ctx, a.subservices)
	//if err != nil {
	//	return fmt.Errorf("failed to start subservices: %w", err)
	//}

	return nil
}

func (a *Asset) running(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return nil
	case err := <-a.subservicesWatcher.Chan():
		// TODO: NewServerService does this differently in tempo
		return fmt.Errorf("asset subservices failed: %w", err)
	}
}

func (a *Asset) stopping(_ error) error {
	return services.StopManagerAndAwaitStopped(context.Background(), a.subservices)
}
