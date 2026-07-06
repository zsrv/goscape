package gamedb

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/zsrv/goscape/pkg/dskit/services"
)

// NewMigratorService wraps schema migration in a dskit service. It is
// the `database` module: it opens a short-lived connection, applies all
// pending migrations, closes that connection, then idles until stopped.
// Modules that use the DB (login, friends) depend on this module in the
// dskit graph, so the topological start order guarantees schema exists
// before any dependent service accepts work — in every target
// combination. The migrator holds NO runtime connection: services are
// independent clients and open their own pools (spec §Design 2).
//
// In split deployments each process runs its own migrator at boot;
// on SQLite the processes share a file (same host) mediated by
// busy_timeout, on Postgres golang-migrate takes an advisory lock.
func NewMigratorService(cfg Config, logger *slog.Logger) services.Service {
	starting := func(ctx context.Context) error {
		db, err := Open(cfg, logger)
		if err != nil {
			return fmt.Errorf("database: open: %w", err)
		}
		if err := db.Migrate(ctx); err != nil {
			db.Close()
			return fmt.Errorf("database: migrate: %w", err)
		}
		logger.Info("central database schema up to date", "backend", cfg.Backend)
		return db.Close()
	}
	running := func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	}
	return services.NewBasicService(starting, running, nil)
}
