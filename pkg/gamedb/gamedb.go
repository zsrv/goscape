package gamedb

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

type dialect int

const (
	dialectSQLite dialect = iota
	dialectPostgres
)

// DB is one service's private connection pool to the central database.
// It embeds *sql.DB so call sites keep the standard query API, and adds
// the dialect-aware Rebind. Services never share a DB value — each
// module Opens its own (independent-clients model, spec §Design 1).
type DB struct {
	*sql.DB
	dialect dialect
}

// Open opens a pool for the configured backend. It does NOT migrate —
// schema lifecycle belongs to the database module (NewMigratorService)
// and to tests via (*DB).Migrate. A nil logger is tolerated (falls back
// to a no-op logger) so library callers outside the module wiring can't
// panic the open path.
func Open(cfg Config, logger *slog.Logger) (*DB, error) {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	switch cfg.Backend {
	case BackendSQLite:
		return openSQLite(cfg.SQLite, logger)
	case BackendPostgres:
		return openPostgres(cfg.Postgres, logger)
	default:
		return nil, fmt.Errorf("gamedb: unknown backend %q", cfg.Backend)
	}
}

func openSQLite(cfg SQLiteConfig, logger *slog.Logger) (*DB, error) {
	if err := ensureDBParentDir(cfg.DSN); err != nil {
		return nil, fmt.Errorf("gamedb: ensure db parent dir: %w", err)
	}
	db, err := sql.Open("sqlite", sqliteDSNWithPragmas(cfg.DSN))
	if err != nil {
		return nil, fmt.Errorf("gamedb: open sqlite: %w", err)
	}
	// One writer at a time is SQLite's own model; serializing the pool
	// removes SQLITE_BUSY between our own transactions and matches the
	// TS engine's single-connection posture (node:sqlite DatabaseSync,
	// src/db/query.ts:13-15).
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		db.Close()
		return nil, fmt.Errorf("gamedb: sqlite pragma journal_mode: %w", err)
	}
	logger.Debug("opened central database", "backend", BackendSQLite, "dsn", cfg.DSN)
	return &DB{DB: db, dialect: dialectSQLite}, nil
}

// openPostgres opens a pooled connection to a Postgres central database.
// sql.Open never dials — connectivity errors (bad host, auth failure,
// missing database) surface on first use (Migrate or the first query),
// which the database module turns into a fail-fast boot error.
func openPostgres(cfg PostgresConfig, logger *slog.Logger) (*DB, error) {
	db, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("gamedb: open postgres: %w", err)
	}
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	logger.Debug("opened central database", "backend", BackendPostgres)
	return &DB{DB: db, dialect: dialectPostgres}, nil
}

// sqliteDSNWithPragmas appends the per-connection pragmas every pooled
// connection must carry. busy_timeout and foreign_keys are
// per-connection settings (unlike journal_mode, which is persistent in
// the file), so they must ride the DSN — a db.Exec PRAGMA would only
// reach whichever single connection the pool hands out.
func sqliteDSNWithPragmas(dsn string) string {
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
	// mode=memory is checked against the full DSN: it lives in the query
	// string, which is stripped below before the path is examined.
	if dsn == ":memory:" || strings.HasPrefix(dsn, "file::memory:") || strings.Contains(dsn, "mode=memory") {
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

// IsForeignKeyViolation reports whether err is an FK-constraint
// violation. Consumers (friends AddFriend/AddIgnore) map it to the TS
// "account missing" drop path: an account deleted
// between existence check and insert must not surface as an internal
// error (spec §Error handling).
func IsForeignKeyViolation(err error) bool {
	if err == nil {
		return false
	}
	// postgres: SQLSTATE 23503 foreign_key_violation.
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code == "23503"
	}
	// modernc.org/sqlite: SQLITE_CONSTRAINT_FOREIGNKEY surfaces as text.
	return strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}

// Rebind converts `?` placeholders to the dialect's form: identity on
// SQLite, $1..$N on Postgres. Call once per statement at the call site.
// Constraint (documented + tested): query strings must not contain '?'
// inside string literals — none of ours do.
func (d *DB) Rebind(query string) string {
	if d.dialect != dialectPostgres {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 8)
	n := 0
	for i := range len(query) {
		if query[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(query[i])
	}
	return b.String()
}
