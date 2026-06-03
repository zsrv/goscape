package friends

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net"

	"github.com/zsrv/goscape/pkg/dskit/services"
)

// Friends is the friends-server module. It owns the gRPC server and the
// in-memory repository.
type Friends struct {
	services.Service

	cfg Config
	log *slog.Logger

	db        *sql.DB
	repo      *Repository
	subs      *subscriptions
	worldSubs *worldSubscriptions
	srv       *grpcServer
	lis       net.Listener
}

// New validates the config and constructs the Friends module.
func New(cfg Config, logger *slog.Logger) (*Friends, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	f := &Friends{cfg: cfg, log: logger}
	f.Service = services.NewBasicService(f.starting, f.running, f.stopping)
	return f, nil
}

// NewFriendsService is the factory used by the dskit module manager.
func NewFriendsService(cfg Config, logger *slog.Logger) (services.Service, error) {
	return New(cfg, logger)
}

func (f *Friends) starting(_ context.Context) error {
	db, err := openDB(f.cfg.SQLiteDSN)
	if err != nil {
		return fmt.Errorf("open friends db: %w", err)
	}
	repo := NewRepository(db, f.cfg.NodeProfile)
	subs := newSubscriptions(f.log)
	worldSubs := newWorldSubscriptions(f.log)
	srv := newGRPCServer(f.cfg, repo, subs, worldSubs, f.log)
	lis, err := srv.listen(f.cfg)
	if err != nil {
		db.Close()
		return err
	}
	f.db = db
	f.repo = repo
	f.subs = subs
	f.worldSubs = worldSubs
	f.srv = srv
	f.lis = lis
	return nil
}

func (f *Friends) running(ctx context.Context) error {
	serverDone := make(chan error, 1)
	lis := f.lis
	f.lis = nil // gRPC now owns the listener
	go func() { serverDone <- f.srv.serve(lis) }()

	select {
	case <-ctx.Done():
		f.srv.shutdown()
		<-serverDone
		return nil
	case err := <-serverDone:
		if err != nil {
			return fmt.Errorf("grpc server: %w", err)
		}
		return nil
	}
}

func (f *Friends) stopping(_ error) error {
	// Covers the edge case where StopAsync is called between starting()
	// returning and running() being invoked — gRPC never took ownership
	// of the listener.
	if f.lis != nil {
		f.lis.Close()
	}
	if f.db != nil {
		f.db.Close()
	}
	return nil
}
