package gamedb

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
)

// postgresTestDB opens the env-configured Postgres with a UNIQUE
// throwaway schema per test (search_path isolation) and applies the
// postgres migration lineage. Skips unless GOSCAPE_TEST_POSTGRES_DSN
// is set, e.g.:
//
//	GOSCAPE_TEST_POSTGRES_DSN='postgres://goscape:goscape@localhost:5432/goscape_test?sslmode=disable'
func postgresTestDB(t *testing.T) *DB {
	t.Helper()
	dsn := os.Getenv("GOSCAPE_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("GOSCAPE_TEST_POSTGRES_DSN not set")
	}
	schema := fmt.Sprintf("t_%x", sha256.Sum256([]byte(t.Name())))[:32]

	cfg := defaultConfig()
	cfg.Backend = BackendPostgres
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	cfg.Postgres.DSN = dsn + sep + "search_path=" + schema

	admin, err := Open(cfg, noopLogger())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := admin.ExecContext(t.Context(), `CREATE SCHEMA IF NOT EXISTS `+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.ExecContext(t.Context(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
		admin.Close()
	})
	if err := admin.Migrate(t.Context()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return admin
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
	if !IsForeignKeyViolation(err) {
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
