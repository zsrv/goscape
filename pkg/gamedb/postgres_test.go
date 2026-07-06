// Package gamedb_test (external test package, not internal gamedb) so
// this file can import gamedbtest, which itself imports gamedb — an
// internal (package gamedb) test file importing gamedbtest would be an
// import cycle ("gamedb test binary" -> gamedbtest -> gamedb). The
// external test package sidesteps that: it depends on both gamedb and
// gamedbtest as ordinary imports, same as any other consumer.
package gamedb_test

import (
	"database/sql"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/zsrv/goscape/pkg/gamedb"
	"github.com/zsrv/goscape/pkg/gamedb/gamedbtest"
)

// noopLogger returns a *slog.Logger that discards all output. Mirrors
// gamedb_test.go's internal noopLogger (duplicated here — this file is
// the external gamedb_test package and can't reach that unexported
// helper directly).
func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// seedAccount inserts a bare account row and returns its id. Mirrors
// migrate_test.go's internal seedAccount (duplicated for the same
// external-package reason as noopLogger above).
func seedAccount(t *testing.T, db *gamedb.DB, username string) int64 {
	t.Helper()
	var id int64
	err := db.QueryRowContext(t.Context(),
		db.Rebind(`INSERT INTO account (username, password) VALUES (?, '') RETURNING id`),
		username,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seedAccount(%s): %v", username, err)
	}
	return id
}

// postgresTestDB opens the env-configured Postgres with a UNIQUE
// throwaway schema per test (search_path isolation) and applies the
// postgres migration lineage. Skips unless GOSCAPE_TEST_POSTGRES_DSN
// is set, e.g.:
//
//	GOSCAPE_TEST_POSTGRES_DSN='postgres://goscape:goscape@localhost:5432/goscape_test?sslmode=disable'
//
// Thin wrapper around gamedbtest.OpenTestSchema (the shared
// schema-isolation harness also used by modules/login and
// modules/friends) that additionally skips when no DSN is configured —
// the module suites instead fall back to in-memory sqlite in that case.
func postgresTestDB(t *testing.T) *gamedb.DB {
	t.Helper()
	dsn := os.Getenv("GOSCAPE_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("GOSCAPE_TEST_POSTGRES_DSN not set")
	}
	return gamedbtest.OpenTestSchema(t, dsn, t.Name(), noopLogger())
}

func TestPostgres_MigrateAndCascade(t *testing.T) {
	db := postgresTestDB(t)
	ctx := t.Context()
	owner := seedAccount(t, db, "owner")
	friend := seedAccount(t, db, "friend")
	if _, err := db.ExecContext(ctx,
		db.Rebind(`INSERT INTO friendlist (profile, account_id, friend_account_id) VALUES ('main', ?, ?)`),
		owner, friend); err != nil {
		t.Fatalf("insert friendlist: %v", err)
	}
	if _, err := db.ExecContext(ctx, db.Rebind(`DELETE FROM account WHERE id = ?`), owner); err != nil {
		t.Fatalf("delete account: %v", err)
	}
	var n int
	if err := db.QueryRowContext(ctx, db.Rebind(`SELECT COUNT(*) FROM friendlist WHERE account_id = ?`), owner).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("cascade: got %d rows, want 0", n)
	}

	// FK violation surfaces through IsForeignKeyViolation (pgx SQLSTATE
	// 23503 path) exactly as sqlite's text-match path does.
	_, err := db.ExecContext(ctx,
		db.Rebind(`INSERT INTO friendlist (profile, account_id, friend_account_id) VALUES ('main', ?, ?)`),
		int64(999999), int64(999998))
	if err == nil {
		t.Fatal("insert with unknown account ids: got nil error, want FK violation")
	}
	if !gamedb.IsForeignKeyViolation(err) {
		t.Errorf("IsForeignKeyViolation(%v): got false, want true", err)
	}
}

func TestPostgres_OnConflictDoNothing(t *testing.T) {
	db := postgresTestDB(t)
	owner := seedAccount(t, db, "owner")
	q := db.Rebind(`INSERT INTO ignorelist (profile, account_id, value) VALUES ('main', ?, 'ghost') ON CONFLICT DO NOTHING`)
	for range 2 {
		if _, err := db.ExecContext(t.Context(), q, owner); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	var n int
	if err := db.QueryRowContext(t.Context(), db.Rebind(`SELECT COUNT(*) FROM ignorelist WHERE account_id = ?`), owner).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("on conflict: got %d rows, want 1", n)
	}
}

func TestPostgres_TimestampDefaultScansAsTime(t *testing.T) {
	db := postgresTestDB(t)
	owner := seedAccount(t, db, "owner")
	friend := seedAccount(t, db, "friend")
	if _, err := db.ExecContext(t.Context(),
		db.Rebind(`INSERT INTO friendlist (profile, account_id, friend_account_id) VALUES ('main', ?, ?)`),
		owner, friend); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var created sql.NullTime
	if err := db.QueryRowContext(t.Context(),
		db.Rebind(`SELECT created FROM friendlist WHERE account_id = ?`), owner).Scan(&created); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !created.Valid || created.Time.IsZero() {
		t.Errorf("created: got %+v, want valid non-zero time", created)
	}
}
