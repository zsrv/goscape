package gamedb

import (
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
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

func TestEnsureDBParentDir_ModeMemorySkipsMkdir(t *testing.T) {
	// Regression: mode=memory lives in the DSN's query string, so the
	// guard must run against the full DSN — checking after truncating at
	// '?' can never fire, and a path-shaped in-memory DSN would
	// spuriously mkdir. openTestDB masks this because url.PathEscape
	// removes path separators.
	dir := filepath.Join(t.TempDir(), "nonexistent-dir")
	dsn := "file:" + filepath.Join(dir, "x.db") + "?mode=memory&cache=shared"
	if err := ensureDBParentDir(dsn); err != nil {
		t.Fatalf("ensureDBParentDir(mode=memory): unexpected error %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("ensureDBParentDir(mode=memory): parent dir was created; want no-op (stat err: %v)", err)
	}

	// Companion positive case: same DSN without mode=memory must mkdir.
	dsnDisk := "file:" + filepath.Join(dir, "x.db") + "?cache=shared"
	if err := ensureDBParentDir(dsnDisk); err != nil {
		t.Fatalf("ensureDBParentDir(disk): unexpected error %v", err)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Errorf("ensureDBParentDir(disk): parent dir not created (err: %v)", err)
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
	// Dialect is package-internal; construct directly rather than
	// through Open (Rebind needs no live connection).
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
