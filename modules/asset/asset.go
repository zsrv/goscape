package asset

import (
	"context"
	"fmt"

	"github.com/zsrv/goscape/internal/dskit/services"
	"github.com/zsrv/goscape/pkg/util/log"
)

// TODO: tracer

// Asset serves assets to game clients.
type Asset struct {
	services.Service

	cfg Config

	// Manager for subservices
	subservices        *services.Manager
	subservicesWatcher *services.FailureWatcher

	logger log.Logger
}

func New(cfg Config, logger log.Logger) (*Asset, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	//subservices := []services.Service(nil)

	a := &Asset{
		cfg:    cfg,
		logger: logger,
	}

	// NOTE: Asset server doesn't have any subservices
	//var err error
	//a.subservices, err = services.NewManager(subservices...)
	//if err != nil {
	//	return nil, fmt.Errorf("failed to create subservices: %w", err)
	//}
	//a.subservicesWatcher = services.NewFailureWatcher()
	//a.subservicesWatcher.WatchManager(a.subservices)

	a.Service = services.NewBasicService(a.starting, a.running, a.stopping)
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
		return fmt.Errorf("asset subservices failed: %w", err)
	}
}

func (a *Asset) stopping(_ error) error {
	return services.StopManagerAndAwaitStopped(context.Background(), a.subservices)
}
