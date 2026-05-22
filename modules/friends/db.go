package friends

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	sqlitedriver "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

// PORTING.md Arc 18 DB-2 — schema-decoupling note:
//
// friendlist / ignorelist / private_chat / public_chat store bare
// username37 integers without referential integrity against an
// accounts table. The friends-server is intentionally federated
// from the login/account store (separate SQLite DB, separate gRPC
// service), so cross-DB FK constraints are not expressible at the
// schema level. Orphan rows from deleted accounts are accepted as
// a federation trade-off; downstream readers (Get{Friends,Ignores,
// Followers}, IsVisibleTo) treat unknown username37s as
// not-currently-online which matches the legacy TS behaviour.
//
// The AddFriend / AddIgnore "INSERT OR IGNORE" race against
// concurrent DeleteFriend / DeleteIgnore is closed in repository.go
// by wrapping the recheck-then-insert in a per-call BeginTx.

func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err = db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite pragma journal_mode: %w", err)
	}
	if _, err = db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite pragma foreign_keys: %w", err)
	}
	if err = migrateDB(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

// migrateDB applies all pending up-migrations. m.Close() is intentionally
// omitted: the sqlite driver's Close() closes the *sql.DB passed to
// WithInstance, which would invalidate all subsequent queries.
func migrateDB(db *sql.DB) error {
	src, err := iofs.New(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("iofs source: %w", err)
	}
	drv, err := sqlitedriver.WithInstance(db, &sqlitedriver.Config{})
	if err != nil {
		return fmt.Errorf("sqlite driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "sqlite", drv)
	if err != nil {
		return fmt.Errorf("migrate instance: %w", err)
	}
	if err = m.Up(); errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}
