package gamedb

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"

	"github.com/golang-migrate/migrate/v4"
	sqlitedriver "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations
var migrationsFS embed.FS

// Migrate applies all pending up-migrations for the active dialect.
// m.Close() is intentionally omitted: the sqlite driver's Close()
// closes the *sql.DB passed to WithInstance, which would invalidate
// all subsequent queries on this pool.
//
// A schema AHEAD of this binary (rollback scenario) surfaces here as a
// golang-migrate version error → the caller (database module or test)
// fails fast.
func (d *DB) Migrate(_ context.Context) error {
	// Guarded up front: migrations/postgres does not exist yet, and
	// letting fs.Sub/iofs.New touch it first would surface a confusing
	// "open .: file does not exist" instead of this message (Phase 2
	// replaces this branch with the pgx driver).
	if d.dialect == dialectPostgres {
		return errors.New("gamedb: postgres migrations not yet supported (Phase 2)")
	}
	sub, err := fs.Sub(migrationsFS, "migrations/sqlite")
	if err != nil {
		return fmt.Errorf("gamedb: migrations subdir: %w", err)
	}
	src, err := iofs.New(sub, ".")
	if err != nil {
		return fmt.Errorf("gamedb: iofs source: %w", err)
	}
	drv, err := sqlitedriver.WithInstance(d.DB, &sqlitedriver.Config{})
	if err != nil {
		return fmt.Errorf("gamedb: sqlite driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "sqlite", drv)
	if err != nil {
		return fmt.Errorf("gamedb: migrate instance: %w", err)
	}
	if err = m.Up(); errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}
