package hiscore

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/zsrv/goscape/pkg/dskit/server"
	"github.com/zsrv/goscape/pkg/dskit/services"
	"github.com/zsrv/goscape/pkg/gamedb"
)

// Hiscore is the module. Like login, friends and account it owns a
// private pool to the central database (independent-clients model). The
// HTTP listener is a dskit server owned by the caller, which is what
// supplies request logging, timeouts and source-IP extraction.
type Hiscore struct {
	services.Service

	cfg   Config
	dbCfg gamedb.Config
	log   *slog.Logger
	serv  *server.Server

	db *gamedb.DB
}

// New validates the config and prepares the module. It does not open the
// database or serve traffic — that happens in the service lifecycle.
func New(cfg Config, dbCfg gamedb.Config, logger *slog.Logger, serv *server.Server) (*Hiscore, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	h := &Hiscore{cfg: cfg, dbCfg: dbCfg, log: logger, serv: serv}
	h.Service = services.NewBasicService(h.starting, h.running, h.stopping)
	return h, nil
}

func (h *Hiscore) starting(context.Context) error {
	db, err := gamedb.Open(h.dbCfg, h.log)
	if err != nil {
		return fmt.Errorf("hiscore: open central database: %w", err)
	}

	a, err := newAPI(h.cfg, NewStore(db), h.log)
	if err != nil {
		// BasicService does not run stoppingFn when startingFn returns an
		// error (arch-29.8 follow-up, fadbfa6c, worked around this same
		// gap elsewhere) — anything opened earlier in starting must be
		// released here, on the spot, or it leaks on every failed start.
		db.Close()
		return fmt.Errorf("hiscore: build api: %w", err)
	}
	a.register(h.serv.HTTP)

	h.db = db
	h.log.Info("hiscore api registered", slog.String("profile", h.cfg.Profile))
	return nil
}

func (h *Hiscore) running(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (h *Hiscore) stopping(_ error) error {
	if h.db != nil {
		if err := h.db.Close(); err != nil {
			h.log.Warn("hiscore: closing database pool", slog.Any("err", err))
		}
		h.db = nil
	}
	return nil
}

// NewHiscoreService composes the module with the dskit server that
// carries its routes, so the server's Run/Shutdown lifecycle is driven
// by the same service the module manager supervises. Mirrors
// modules/ondemand.NewOndemandService.
//
// The module is started first (it registers routes and opens the pool),
// then the HTTP server runs; on shutdown the server stops before the
// module releases its pool.
func NewHiscoreService(h *Hiscore, serv *server.Server) services.Service {
	serverDone := make(chan error, 1)

	startingFn := func(ctx context.Context) error {
		return services.StartAndAwaitRunning(ctx, h)
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
			// A clean server exit while the context is still live means
			// the listener went away underneath us — a failure, not a
			// shutdown. Same posture as NewOndemandService.
			return fmt.Errorf("hiscore server stopped unexpectedly")
		}
	}

	stoppingFn := func(_ error) error {
		// Shut the HTTP server down first (this also unblocks runFn),
		// wait for the server goroutine, and only then let the module
		// release its database pool — no in-flight handler can still be
		// querying by that point.
		serv.Shutdown()
		<-serverDone
		serv.Log.Info("hiscore server stopped")

		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return services.StopAndAwaitTerminated(stopCtx, h)
	}

	return services.NewBasicService(startingFn, runFn, stoppingFn)
}
