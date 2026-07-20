// Package account is the account-management module: portal accounts as
// containers for game characters, third-party identity linking, the
// character-creation gate, an SSR player portal, and the AccountService
// gRPC surface consumed by the login module and goscape-cli.
// Spec: docs/superpowers/specs/2026-07-19-account-management-design.md
package account

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"google.golang.org/grpc"

	"github.com/zsrv/goscape/pkg/dskit/services"
	"github.com/zsrv/goscape/pkg/gamedb"
)

// Account is the module. Like login, it owns a private pool to the
// central database; unlike login it runs two listeners (portal HTTP +
// AccountService gRPC).
type Account struct {
	services.Service

	cfg   Config
	dbCfg gamedb.Config
	log   *slog.Logger

	db      *gamedb.DB
	store   *Store
	grpcSrv *grpc.Server
	grpcLis net.Listener
	httpSrv *http.Server
	httpLis net.Listener
}

func New(cfg Config, dbCfg gamedb.Config, logger *slog.Logger) (*Account, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	a := &Account{cfg: cfg, dbCfg: dbCfg, log: logger}
	a.Service = services.NewBasicService(a.starting, a.running, a.stopping)
	return a, nil
}

func (a *Account) starting(ctx context.Context) error {
	db, err := gamedb.Open(a.dbCfg, a.log)
	if err != nil {
		return fmt.Errorf("open central database: %w", err)
	}
	store := NewStore(db)

	var mailer Mailer = newLogMailer(a.log)
	if a.cfg.SMTP.Host != "" {
		mailer = newSMTPMailer(a.cfg.SMTP)
	}
	p, err := newPortal(a.cfg, store, mailer, a.log)
	if err != nil {
		db.Close()
		return fmt.Errorf("portal: %w", err)
	}

	grpcSrv := newGRPCServer(a.cfg, store, a.log)
	grpcAddr := fmt.Sprintf("%s:%d", a.cfg.GRPCListenAddress, a.cfg.GRPCListenPort)
	grpcLis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		db.Close()
		return fmt.Errorf("grpc listen %s: %w", grpcAddr, err)
	}

	httpAddr := fmt.Sprintf("%s:%d", a.cfg.HTTPListenAddress, a.cfg.HTTPListenPort)
	httpLis, err := net.Listen("tcp", httpAddr)
	if err != nil {
		grpcLis.Close()
		db.Close()
		return fmt.Errorf("http listen %s: %w", httpAddr, err)
	}

	a.db = db
	a.store = store
	a.grpcSrv = grpcSrv
	a.grpcLis = grpcLis
	a.httpSrv = &http.Server{Handler: p.routes(), ReadHeaderTimeout: 10 * time.Second}
	a.httpLis = httpLis
	a.log.Info("account module listening",
		slog.String("http", httpAddr), slog.String("grpc", grpcAddr))
	return nil
}

func (a *Account) running(ctx context.Context) error {
	grpcDone := make(chan error, 1)
	httpDone := make(chan error, 1)
	grpcLis, httpLis := a.grpcLis, a.httpLis
	a.grpcLis, a.httpLis = nil, nil // servers own the listeners now

	go func() { grpcDone <- a.grpcSrv.Serve(grpcLis) }()
	go func() {
		if err := a.httpSrv.Serve(httpLis); !errors.Is(err, http.ErrServerClosed) {
			httpDone <- err
			return
		}
		httpDone <- nil
	}()

	shutdown := func() {
		shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.httpSrv.Shutdown(shCtx)
		a.grpcSrv.GracefulStop()
	}

	select {
	case <-ctx.Done():
		shutdown()
		<-grpcDone
		<-httpDone
		return nil
	case err := <-grpcDone:
		shutdown()
		<-httpDone
		if err != nil {
			return fmt.Errorf("account grpc server: %w", err)
		}
		return nil
	case err := <-httpDone:
		shutdown()
		<-grpcDone
		if err != nil {
			return fmt.Errorf("account http server: %w", err)
		}
		return nil
	}
}

func (a *Account) stopping(_ error) error {
	// Edge case: StopAsync between starting() and running() — the
	// servers never took listener ownership.
	if a.grpcLis != nil {
		a.grpcLis.Close()
	}
	if a.httpLis != nil {
		a.httpLis.Close()
	}
	if a.db != nil {
		a.db.Close()
	}
	return nil
}
