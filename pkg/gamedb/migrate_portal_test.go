package gamedb_test

import (
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/gamedb"
)

// openMigratedSQLite mirrors the existing migrate_test.go helper style:
// fresh file DB in t.TempDir(), full lineage applied.
func openMigratedSQLite(t *testing.T) *gamedb.DB {
	t.Helper()
	var cfg gamedb.Config
	cfg.Backend = gamedb.BackendSQLite
	cfg.SQLite.DSN = filepath.Join(t.TempDir(), "test.db")
	db, err := gamedb.Open(cfg, slog.Default())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(t.Context()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestMigration000003_PortalTables(t *testing.T) {
	db := openMigratedSQLite(t)
	ctx := t.Context()

	for _, table := range []string{
		"portal_account", "portal_identity", "portal_character",
		"portal_group", "portal_group_member", "portal_session",
		"portal_token", "portal_audit_log",
	} {
		if _, err := db.ExecContext(ctx, "SELECT * FROM "+table+" WHERE 1=0"); err != nil {
			t.Errorf("table %s missing: %v", table, err)
		}
	}

	// Seeded groups.
	var n int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM portal_group WHERE name IN ('manually_approved', 'admin')`).Scan(&n)
	if err != nil || n != 2 {
		t.Fatalf("seeded groups: n=%d err=%v", n, err)
	}

	// One third-party identity can vouch for at most one portal account.
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, db.Rebind(q), args...); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	mustExec(`INSERT INTO portal_account (email, email_verified, password_hash, status, created_at, updated_at)
	          VALUES ('a@example.com', 1, 'x', 'active', '2026-07-19 00:00:00', '2026-07-19 00:00:00')`)
	mustExec(`INSERT INTO portal_account (email, email_verified, password_hash, status, created_at, updated_at)
	          VALUES ('b@example.com', 1, 'x', 'active', '2026-07-19 00:00:00', '2026-07-19 00:00:00')`)
	mustExec(`INSERT INTO portal_identity (account_id, provider, provider_user_id, provider_username, linked_at)
	          VALUES (1, 'discord', 'D1', 'alice', '2026-07-19 00:00:00')`)
	if _, err := db.ExecContext(ctx, db.Rebind(
		`INSERT INTO portal_identity (account_id, provider, provider_user_id, provider_username, linked_at)
		 VALUES (?, 'discord', 'D1', 'mallory', '2026-07-19 00:00:00')`), 2); err == nil {
		t.Fatal("duplicate (provider, provider_user_id) must violate UNIQUE")
	}
}
