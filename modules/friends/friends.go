package friends

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/zsrv/goscape/pkg/dskit/services"
	"github.com/zsrv/goscape/pkg/gamedb"
)

// Friends is the friends-server module. It owns its private pool to the
// central database, the gRPC server, and the repository.
type Friends struct {
	services.Service

	cfg   Config
	dbCfg gamedb.Config
	log   *slog.Logger

	db        *gamedb.DB
	repo      *Repository
	subs      *subscriptions
	worldSubs *worldSubscriptions
	srv       *grpcServer
	lis       net.Listener
}

// New validates the config and constructs the Friends module. dbCfg is
// the shared database: section — the module opens its OWN pool with it
// in starting() (independent-clients model; schema is migrated by the
// database module, which friends depends on in the app graph).
func New(cfg Config, dbCfg gamedb.Config, logger *slog.Logger) (*Friends, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	f := &Friends{cfg: cfg, dbCfg: dbCfg, log: logger}
	f.Service = services.NewBasicService(f.starting, f.running, f.stopping)
	return f, nil
}

func (f *Friends) starting(_ context.Context) error {
	db, err := gamedb.Open(f.dbCfg, f.log)
	if err != nil {
		return fmt.Errorf("open central database: %w", err)
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
		// arch-29.4: release every open subscriber stream before asking
		// gRPC to stop. SubscribeUpdates/SubscribeWorldEvents loops exit
		// only on client-driven ctx.Done or sub.done; with nothing else
		// server-side ending them, GracefulStop would otherwise wait
		// forever for a subscriber a client never closes — the
		// standalone `--target friends` SIGTERM hang while worlds are
		// attached. closeAll leaves both registries empty but usable:
		// a Subscribe landing after this point (the narrow race before
		// GracefulStop actually stops accepting) registers normally and
		// exits via its stream ctx once shutdown's grace window forces
		// Stop (see grpcServer.shutdown / defaultGracefulStopBound).
		f.subs.closeAll()
		f.worldSubs.closeAll()
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
