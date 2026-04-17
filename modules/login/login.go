package login

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/zsrv/goscape/internal/dskit/services"
)

// Login is the login server module. It owns the SQLite DB and the gRPC server.
type Login struct {
	services.Service

	cfg Config
	log *slog.Logger
}

// New validates the config and constructs the Login module.
func New(cfg Config, logger *slog.Logger) (*Login, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	l := &Login{cfg: cfg, log: logger}
	l.Service = services.NewBasicService(l.starting, l.running, l.stopping)
	return l, nil
}

// NewLoginService is the factory used by the dskit module manager.
func NewLoginService(cfg Config, logger *slog.Logger) (services.Service, error) {
	return New(cfg, logger)
}

func (l *Login) starting(ctx context.Context) error {
	_ = ctx
	return nil
}

func (l *Login) running(ctx context.Context) error {
	db, err := openDB(l.cfg.SQLiteDSN)
	if err != nil {
		return fmt.Errorf("open login db: %w", err)
	}

	srv := newGRPCServer(l.cfg, db, l.log)
	serverDone := make(chan error, 1)
	go func() { serverDone <- srv.run(l.cfg) }()

	select {
	case <-ctx.Done():
		srv.shutdown()
		<-serverDone
		db.Close()
		return nil
	case err := <-serverDone:
		db.Close()
		if err != nil {
			return fmt.Errorf("grpc server: %w", err)
		}
		return nil
	}
}

func (l *Login) stopping(_ error) error {
	return nil
}
