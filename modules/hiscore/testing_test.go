package hiscore

import (
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/gamedb"
	"github.com/zsrv/goscape/pkg/gamedb/gamedbtest"
)

func noopLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

// createTestDB opens an isolated migrated central test DB: in-memory
// sqlite by default; the env-configured Postgres (unique schema per
// test, dropped on cleanup) when GOSCAPE_TEST_POSTGRES_DSN is set, so
// the whole suite can run against the real backend. Mirrors
// modules/login/db_test.go:createTestDB.
func createTestDB(t *testing.T) *gamedb.DB {
	t.Helper()
	if dsn := os.Getenv("GOSCAPE_TEST_POSTGRES_DSN"); dsn != "" {
		return gamedbtest.OpenTestSchema(t, dsn, t.Name(), noopLogger())
	}

	var cfg gamedb.Config
	fs := flag.NewFlagSet("", flag.PanicOnError)
	cfg.RegisterFlagsAndApplyDefaults(fs)
	cfg.SQLite.DSN = fmt.Sprintf("file:%s?mode=memory&cache=shared", url.PathEscape(t.Name()))
	db, err := gamedb.Open(cfg, noopLogger())
	if err != nil {
		t.Fatalf("createTestDB: open: %v", err)
	}
	if err := db.Migrate(t.Context()); err != nil {
		db.Close()
		t.Fatalf("createTestDB: migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// testClock is the fixed "now" every store/handler test uses, so
// banned_until comparisons and Last-Modified values are deterministic.
var testClock = time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)

// insertAccount inserts an account row and returns its id. staffModLevel
// > 1 and a bannedUntil at or after the test clock both make the account
// invisible to the API.
func insertAccount(t *testing.T, db *gamedb.DB, username string, staffModLevel int, bannedUntil *time.Time) int64 {
	t.Helper()
	res, err := db.ExecContext(t.Context(), db.Rebind(
		`INSERT INTO account (username, password, registration_ip, staff_mod_level, members, banned_until)
		 VALUES (?, ?, ?, ?, ?, ?)`),
		username, "x", "127.0.0.1", staffModLevel, 0, bannedUntil)
	if err != nil {
		t.Fatalf("insertAccount(%s): %v", username, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		// Postgres' pgx driver does not support LastInsertId; read it back.
		var back int64
		if qerr := db.QueryRowContext(t.Context(), db.Rebind(
			`SELECT id FROM account WHERE username = ?`), username).Scan(&back); qerr != nil {
			t.Fatalf("insertAccount(%s): id lookup: %v", username, qerr)
		}
		return back
	}
	return id
}

// insertHiscore writes one leaderboard row. valueX10 is the raw
// fixed-point tenths value, exactly as modules/login stores it.
func insertHiscore(t *testing.T, db *gamedb.DB, table string, accountID int64, profile string, typ, level int, valueX10 int64, date time.Time) {
	t.Helper()
	q := `INSERT INTO hiscore (account_id, profile, type, level, value, date) VALUES (?, ?, ?, ?, ?, ?)`
	if table == "hiscore_large" {
		q = `INSERT INTO hiscore_large (account_id, profile, type, level, value, date) VALUES (?, ?, ?, ?, ?, ?)`
	}
	if _, err := db.ExecContext(t.Context(), db.Rebind(q),
		accountID, profile, typ, level, valueX10, date); err != nil {
		t.Fatalf("insertHiscore(%s, acct=%d, type=%d): %v", table, accountID, typ, err)
	}
}
