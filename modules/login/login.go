package login

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net"

	"github.com/zsrv/goscape/pkg/dskit/services"
	"github.com/zsrv/goscape/pkg/telemetry"
)

// Login is the login server module. It owns the SQLite DB and the gRPC server.
type Login struct {
	services.Service

	cfg Config
	log *slog.Logger

	db       *sql.DB
	srv      *grpcServer
	lis      net.Listener
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
	db, err := openDB(l.cfg.SQLiteDSN)
	if err != nil {
		return fmt.Errorf("open login db: %w", err)
	}


	lis, err := srv.listen(l.cfg)
	if err != nil {
		db.Close()
		return err
	}

	l.db = db
	l.srv = srv
	l.lis = lis
	return nil
}

func (l *Login) running(ctx context.Context) error {
	serverDone := make(chan error, 1)
	lis := l.lis
	l.lis = nil // gRPC now owns the listener
	go func() { serverDone <- l.srv.serve(lis) }()

	select {
	case <-ctx.Done():
		l.srv.shutdown()
		<-serverDone
		return nil
	case err := <-serverDone:
		if err != nil {
			return fmt.Errorf("grpc server: %w", err)
		}
		return nil
	}
}

func (l *Login) stopping(_ error) error {
	// Covers the edge case where StopAsync is called between starting() returning
	// and running() being invoked — gRPC never took ownership of the listener.
	if l.lis != nil {
		l.lis.Close()
	}
	}
	if l.db != nil {
		l.db.Close()
	}
	return nil
}
