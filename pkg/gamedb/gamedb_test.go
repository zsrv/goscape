package gamedb

import (
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// TestSQLite_TimeRoundTrip pins WHY the sqlite DDL's DATETIME decltype is
// load-bearing (Task 11's Step-1 discovery: migrations/sqlite/
// 000001_init.up.sql originally declared its time-bearing columns TEXT,
// and was changed to DATETIME because of the failure pinned here).
//
// Writing a time.Time param and scanning sql.NullTime does NOT round-trip
// on a TEXT-declared column, in EITHER direction.
// database/sql's convertAssign only accepts a src of time.Time (or nil) for
// a *time.Time/*sql.NullTime destination — see database/sql/convert.go. The
// modernc.org/sqlite driver only produces a time.Time driver.Value for a
// column when sqlite3_column_decltype reports "DATE", "DATETIME", or
// "TIMESTAMP" (rows.go Next(); conn.go parseTime/parseTimeFormats). For a
// column declared TEXT, the driver always returns a plain Go string,
// regardless of whether that string was produced by binding a time.Time
// query param (which the driver formats via time.Time.String(), e.g.
// "2026-07-05 12:30:00 +0000 UTC" — see conn.go formatTime) or by literal
// legacy text ("2026-07-05 12:30:00"). Scanning either string into
// sql.NullTime fails identically:
//
//	sql: Scan error on column index 0, name "at": unsupported Scan,
//	storing driver.Value type string into type *time.Time
//
// This test pins that failure (both directions, same error shape) so a
// TEXT-column DB stays a documented incompatibility rather than a silent
// behavior change. See TestSQLite_TimeRoundTrip_DeclaredTypeMatters below
// for the confirming positive case: the identical scan succeeds when the
// column is declared DATETIME instead of TEXT — decltype, not storage
// class, gates the driver's time auto-parsing. Schema principle:
// timestamptz in migrations/postgres ⇔ DATETIME in migrations/sqlite.
func TestSQLite_TimeRoundTrip(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec(`CREATE TABLE tt (id INTEGER PRIMARY KEY, at TEXT)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	want := time.Date(2026, 7, 5, 12, 30, 0, 0, time.UTC)
	if _, err := db.Exec(`INSERT INTO tt (id, at) VALUES (1, ?)`, want); err != nil {
		t.Fatalf("insert time.Time: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO tt (id, at) VALUES (2, '2026-07-05 12:30:00')`); err != nil {
		t.Fatalf("insert legacy text: %v", err)
	}
	const wantErrSubstr = "unsupported Scan, storing driver.Value type string into type *time.Time"
	for id := 1; id <= 2; id++ {
		var got sql.NullTime
		err := db.QueryRow(`SELECT at FROM tt WHERE id = ?`, id).Scan(&got)
		if err == nil {
			t.Fatalf("id=%d: scan into sql.NullTime unexpectedly succeeded (got %v, valid=%v) — modernc's TEXT-column"+
				" behavior changed; re-evaluate Task 11's sweep against the current schema", id, got.Time, got.Valid)
		}
		if !strings.Contains(err.Error(), wantErrSubstr) {
			t.Fatalf("id=%d: got error %q, want substring %q", id, err, wantErrSubstr)
		}
	}
}

// TestSQLite_TimeRoundTrip_DeclaredTypeMatters is the confirming companion
// to TestSQLite_TimeRoundTrip above: the identical write/scan pattern
// round-trips cleanly once the column's declared type is DATETIME instead
// of TEXT — sqlite3_column_decltype, not the underlying storage class, is
// what gates modernc.org/sqlite's time auto-parsing on read (rows.go
// Next()). Both a time.Time-written row and a legacy
// "2006-01-02 15:04:05" text row scan correctly into sql.NullTime under a
// DATETIME column. The legacy-text leg doubles as the compatibility pin
// for rows written by Phase 1 binaries via the old
// dbTimeFormat = "2006-01-02 15:04:05" string writes: they remain
// readable after the login module's switch to time.Time params.
func TestSQLite_TimeRoundTrip_DeclaredTypeMatters(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec(`CREATE TABLE tt_dt (id INTEGER PRIMARY KEY, at DATETIME)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	want := time.Date(2026, 7, 5, 12, 30, 0, 0, time.UTC)
	if _, err := db.Exec(`INSERT INTO tt_dt (id, at) VALUES (1, ?)`, want); err != nil {
		t.Fatalf("insert time.Time: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO tt_dt (id, at) VALUES (2, '2026-07-05 12:30:00')`); err != nil {
		t.Fatalf("insert legacy text: %v", err)
	}
	for id := 1; id <= 2; id++ {
		var got sql.NullTime
		if err := db.QueryRow(`SELECT at FROM tt_dt WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatalf("scan id=%d: %v", id, err)
		}
		if !got.Valid || !got.Time.UTC().Equal(want) {
			t.Errorf("id=%d: got %v, want %v", id, got.Time, want)
		}
	}
}

// TestSQLite_TimestampDefaultScansAsTime is the sqlite twin of
// TestPostgres_TimestampDefaultScansAsTime: a DATETIME column filled by
// its DEFAULT CURRENT_TIMESTAMP (sqlite emits 'YYYY-MM-DD HH:MM:SS' UTC
// text) must scan into sql.NullTime — i.e. modernc's decltype-gated
// parser must handle sqlite's own default-value format, not just values
// the driver wrote itself. Exercises the real migrated schema
// (friendlist.created) rather than a synthetic table.
func TestSQLite_TimestampDefaultScansAsTime(t *testing.T) {
	db := migratedTestDB(t)
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
