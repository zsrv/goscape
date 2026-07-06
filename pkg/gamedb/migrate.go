package gamedb

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
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
	subdir := "migrations/sqlite"
	if d.dialect == dialectPostgres {
		subdir = "migrations/postgres"
	}
	sub, err := fs.Sub(migrationsFS, subdir)
	if err != nil {
		return fmt.Errorf("gamedb: migrations subdir: %w", err)
	}
	src, err := iofs.New(sub, ".")
	if err != nil {
		return fmt.Errorf("gamedb: iofs source: %w", err)
	}
	var (
		drvName string
		drv     database.Driver
	)
	if d.dialect == dialectPostgres {
		drvName = "pgx5"
		drv, err = pgxmigrate.WithInstance(d.DB, &pgxmigrate.Config{})
	} else {
		drvName = "sqlite"
		drv, err = sqlitedriver.WithInstance(d.DB, &sqlitedriver.Config{})
	}
	if err != nil {
		return fmt.Errorf("gamedb: %s driver: %w", drvName, err)
	}
	m, err := migrate.NewWithInstance("iofs", src, drvName, drv)
	if err != nil {
		return fmt.Errorf("gamedb: migrate instance: %w", err)
	}
	if err = m.Up(); errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}
