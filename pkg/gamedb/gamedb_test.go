package gamedb

import (
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"path/filepath"
	"testing"
)

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// openTestDB opens an isolated in-memory sqlite DB (no migrations).
func openTestDB(t *testing.T) *DB {
	t.Helper()
	c := defaultConfig()
	c.SQLite.DSN = fmt.Sprintf("file:%s?mode=memory&cache=shared", url.PathEscape(t.Name()))
	db, err := Open(c, noopLogger())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpen_SQLitePragmasApplied(t *testing.T) {
	c := defaultConfig()
	c.SQLite.DSN = filepath.Join(t.TempDir(), "sub", "goscape.db") // sub: exercises parent-dir creation
	db, err := Open(c, noopLogger())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	var busy int
	if err := db.QueryRow(`PRAGMA busy_timeout`).Scan(&busy); err != nil {
		t.Fatalf("busy_timeout: %v", err)
	}
	if busy != 5000 {
		t.Errorf("busy_timeout: got %d, want 5000", busy)
	}
	var fk int
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatalf("foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys: got %d, want 1", fk)
	}
	var mode string
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode: got %q, want wal", mode)
	}
}

func TestRebind_SQLiteIdentity(t *testing.T) {
	db := openTestDB(t)
	q := `SELECT id FROM account WHERE username = ? AND members = ?`
	if got := db.Rebind(q); got != q {
		t.Errorf("Rebind(sqlite): got %q, want identity", got)
	}
}

func TestRebind_PostgresNumbersPlaceholders(t *testing.T) {
	// Dialect is package-internal; construct directly (Open(postgres)
	// lands in Phase 2).
	d := &DB{dialect: dialectPostgres}
	got := d.Rebind(`INSERT INTO t (a, b, c) VALUES (?, ?, ?)`)
	want := `INSERT INTO t (a, b, c) VALUES ($1, $2, $3)`
	if got != want {
		t.Errorf("Rebind(postgres):\n got %q\nwant %q", got, want)
	}
	// No '?' anywhere: identity even on postgres.
	if q := d.Rebind(`DELETE FROM t`); q != `DELETE FROM t` {
		t.Errorf("Rebind(no placeholders): got %q", q)
	}
}
