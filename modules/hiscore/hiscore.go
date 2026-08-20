package hiscore

import (
	"context"
	"fmt"
	"log/slog"

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
