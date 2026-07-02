package friends

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	sqlitedriver "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

// docs/PORTING.md Arc 18 DB-2 — schema-decoupling note:
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
	if err := ensureDBParentDir(dsn); err != nil {
		return nil, fmt.Errorf("ensure db parent dir: %w", err)
	}
	db, err := sql.Open("sqlite", dsnWithPragmas(dsn))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// One writer at a time is SQLite's own model; serializing the pool
	// removes SQLITE_BUSY between our own transactions and matches the
	// TS engine's better-sqlite3 single-connection posture.
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite pragma journal_mode: %w", err)
	}
	if err = migrateDB(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

// dsnWithPragmas appends the per-connection pragmas every pooled
// connection must carry. busy_timeout and foreign_keys are
// per-connection settings (unlike journal_mode, which is persistent
// in the file), so they must ride the DSN — a db.Exec PRAGMA would
// only reach whichever single connection the pool hands out.
func dsnWithPragmas(dsn string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
}

// ensureDBParentDir creates the parent directory of the sqlite DSN's
// file path (no-op if it already exists or the DSN is in-memory).
// SQLite itself doesn't create missing parent directories — it returns
// SQLITE_CANTOPEN (error 14) on first query.
func ensureDBParentDir(dsn string) error {
	if dsn == ":memory:" || strings.HasPrefix(dsn, "file::memory:") {
		return nil
	}
	p := strings.TrimPrefix(dsn, "file:")
	if i := strings.IndexByte(p, '?'); i >= 0 {
		p = p[:i]
	}
	dir := filepath.Dir(p)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
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
