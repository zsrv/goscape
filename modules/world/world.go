package world

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/zsrv/goscape/internal/dskit/services"
	"github.com/zsrv/goscape/internal/dskit/signals"
)

// TODO: tracer

// World represents an instance of a game world.
type World struct {
	services.Service
	log                *slog.Logger
	subservices        *services.Manager
	subservicesWatcher *services.FailureWatcher
	Server             *Server
	cfg                Config
}

func New(cfg Config, logger *slog.Logger) (*World, error) {
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

	server, err := NewServer(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create server: %w", err)
	}
	w.Server = server

	return w, nil
}

// NewWorldService constructs a services.Service from a Server component.
// The Server should not react to signals. Early return from Run function
// is considered to be an error.
func NewWorldService(serv *Server, servicesToWaitFor func() []services.Service) services.Service {
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
		return nil
	}

	return services.NewBasicService(nil, runFn, stoppingFn)
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
