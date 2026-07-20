package login

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/zsrv/goscape/pkg/accountpb"
	"github.com/zsrv/goscape/pkg/dskit/services"
	"github.com/zsrv/goscape/pkg/gamedb"
)

// Login is the login server module. It owns its private pool to the
// central database and the gRPC server.
type Login struct {
	services.Service

	cfg   Config
	dbCfg gamedb.Config
	log   *slog.Logger

	db       *gamedb.DB
	srv      *grpcServer
	lis      net.Listener
	acctConn *grpc.ClientConn
}

// New validates the config and constructs the Login module. dbCfg is
// the shared database: section — the module opens its OWN pool with it
// in starting() (independent-clients model; schema is migrated by the
// database module, which login depends on in the app graph).
func New(cfg Config, dbCfg gamedb.Config, logger *slog.Logger) (*Login, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	l := &Login{cfg: cfg, dbCfg: dbCfg, log: logger}
	l.Service = services.NewBasicService(l.starting, l.running, l.stopping)
	return l, nil
}

func (l *Login) starting(ctx context.Context) error {
	db, err := gamedb.Open(l.dbCfg, l.log)
	if err != nil {
		return fmt.Errorf("open central database: %w", err)
	}

	var acct accountpb.AccountServiceClient
	if l.cfg.AuthMode == AuthModeAccount {
		conn, err := grpc.NewClient(l.cfg.AccountGRPCAddress,
			grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			db.Close()
			return fmt.Errorf("dial account service: %w", err)
		}
		l.acctConn = conn
		acct = accountpb.NewAccountServiceClient(conn)
	}

	srv := newGRPCServer(l.cfg, db, acct, l.log)
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
	if l.acctConn != nil {
		l.acctConn.Close()
	}
	if l.db != nil {
		l.db.Close()
	}
	return nil
}
