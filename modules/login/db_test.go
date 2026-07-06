package login

import (
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/zsrv/goscape/pkg/gamedb"
	"github.com/zsrv/goscape/pkg/gamedb/gamedbtest"
)

// createTestDB opens an isolated central test DB: in-memory sqlite by
// default; the env-configured Postgres (unique schema per test, dropped
// on cleanup) when GOSCAPE_TEST_POSTGRES_DSN is set — the whole module
// suite then runs against the real backend.
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

// insertTestAccount inserts a test account with bcrypt-hashed password (cost 4 for speed).
// Returns the inserted account ID.
func insertTestAccount(t *testing.T, db *gamedb.DB, username, password string) int64 {
	t.Helper()
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), 4)
	if err != nil {
		t.Fatalf("insertTestAccount: bcrypt: %v", err)
	}
	id, err := insertAccount(t.Context(), db, username, string(hashed), "127.0.0.1")
	if err != nil {
		t.Fatalf("insertTestAccount: insertAccount: %v", err)
	}
	return id
}

// noopLogger returns a *slog.Logger that discards all output.
func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestAccountByUsername_NotFound(t *testing.T) {
	db := createTestDB(t)
	row, err := accountByUsername(t.Context(), db, "nonexistent", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if row != nil {
		t.Fatalf("expected nil row, got %+v", row)
	}
}

func TestAccountByUsername_Found(t *testing.T) {
	db := createTestDB(t)
	id := insertTestAccount(t, db, "testuser", "hunter2")

	row, err := accountByUsername(t.Context(), db, "testuser", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if row == nil {
		t.Fatal("expected non-nil row")
	}
	if row.ID != int(id) {
		t.Errorf("ID: got %d, want %d", row.ID, id)
	}
	if row.Username != "testuser" {
		t.Errorf("Username: got %q, want %q", row.Username, "testuser")
	}
	if row.Password == "" {
		t.Error("Password should not be empty")
	}
	if row.HasLoginRow {
		t.Error("HasLoginRow should be false when no account_login row exists")
	}
}

func TestAccountByUsername_WithLoginRow(t *testing.T) {
	db := createTestDB(t)
	id := insertTestAccount(t, db, "loginuser", "secret")

	// Insert an account_login row with logged_in=1
	_, err := db.ExecContext(t.Context(),
		db.Rebind(`INSERT INTO account_login (account_id, profile, node_id, logged_in) VALUES (?, ?, ?, ?)`),
		id, "main", 3, 1,
	)
	if err != nil {
		t.Fatalf("insert account_login: %v", err)
	}

	row, err := accountByUsername(t.Context(), db, "loginuser", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if row == nil {
		t.Fatal("expected non-nil row")
	}
	if !row.HasLoginRow {
		t.Error("HasLoginRow should be true")
	}
	if row.LoggedIn != 1 {
		t.Errorf("LoggedIn: got %d, want 1", row.LoggedIn)
	}
	if row.NodeID != 3 {
		t.Errorf("NodeID: got %d, want 3", row.NodeID)
	}
}

func TestIPBanned_NotBanned(t *testing.T) {
	db := createTestDB(t)
	banned, err := ipBanned(t.Context(), db, "192.168.1.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if banned {
		t.Error("expected not banned")
	}
}

func TestIPBanned_Banned(t *testing.T) {
	db := createTestDB(t)
	_, err := db.ExecContext(t.Context(),
		db.Rebind(`INSERT INTO ipban (ip, added_by, added_on) VALUES (?, ?, ?)`),
		"10.0.0.1", "admin", "2026-01-01 00:00:00",
	)
	if err != nil {
		t.Fatalf("insert ipban: %v", err)
	}

	banned, err := ipBanned(t.Context(), db, "10.0.0.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !banned {
		t.Error("expected banned")
	}
}

func TestInsertAccount(t *testing.T) {
	db := createTestDB(t)
	hashed, err := bcrypt.GenerateFromPassword([]byte("password123"), 4)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}

	id, err := insertAccount(t.Context(), db, "newuser", string(hashed), "10.0.0.2")
	if err != nil {
		t.Fatalf("insertAccount: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive ID, got %d", id)
	}

	row, err := accountByUsername(t.Context(), db, "newuser", "main")
	if err != nil {
		t.Fatalf("accountByUsername: %v", err)
	}
	if row == nil {
		t.Fatal("expected account to be queryable after insert")
	}
	if row.Username != "newuser" {
		t.Errorf("Username: got %q, want %q", row.Username, "newuser")
	}
}

func TestUpsertAccountLogin_Insert(t *testing.T) {
	db := createTestDB(t)
	id := insertTestAccount(t, db, "upsertuser", "pass")

	err := upsertAccountLogin(t.Context(), db, int(id), "main", 5)
	if err != nil {
		t.Fatalf("upsertAccountLogin: %v", err)
	}

	var loggedIn, nodeID int
	err = db.QueryRowContext(t.Context(),
		db.Rebind(`SELECT logged_in, node_id FROM account_login WHERE account_id = ? AND profile = ?`),
		id, "main",
	).Scan(&loggedIn, &nodeID)
	if err != nil {
		t.Fatalf("query account_login: %v", err)
	}
	if loggedIn != 1 {
		t.Errorf("logged_in: got %d, want 1", loggedIn)
	}
	if nodeID != 5 {
		t.Errorf("node_id: got %d, want 5", nodeID)
	}
}

func TestUpsertAccountLogin_Update(t *testing.T) {
	db := createTestDB(t)
	id := insertTestAccount(t, db, "updateuser", "pass")

	// Pre-insert a row with logged_in=0
	_, err := db.ExecContext(t.Context(),
		db.Rebind(`INSERT INTO account_login (account_id, profile, node_id, logged_in) VALUES (?, ?, ?, ?)`),
		id, "main", 0, 0,
	)
	if err != nil {
		t.Fatalf("insert account_login: %v", err)
	}

	err = upsertAccountLogin(t.Context(), db, int(id), "main", 7)
	if err != nil {
		t.Fatalf("upsertAccountLogin: %v", err)
	}

	var loggedIn, nodeID int
	err = db.QueryRowContext(t.Context(),
		db.Rebind(`SELECT logged_in, node_id FROM account_login WHERE account_id = ? AND profile = ?`),
		id, "main",
	).Scan(&loggedIn, &nodeID)
	if err != nil {
		t.Fatalf("query account_login: %v", err)
	}
	if loggedIn != 1 {
		t.Errorf("logged_in: got %d, want 1", loggedIn)
	}
	if nodeID != 7 {
		t.Errorf("node_id: got %d, want 7", nodeID)
	}
}

func TestInsertSession(t *testing.T) {
	db := createTestDB(t)
	id := insertTestAccount(t, db, "sessionuser", "pass")

	err := insertSession(t.Context(), db, "11111111-2222-3333-4444-555555555555", int(id), "main", 2, 42, "192.168.0.1")
	if err != nil {
		t.Fatalf("insertSession: %v", err)
	}

	var sessionUUID, profile, remoteAddr string
	var world, uid int
	err = db.QueryRowContext(t.Context(),
		db.Rebind(`SELECT session_uuid, profile, world, uid, remote_address FROM session WHERE account_id = ?`),
		id,
	).Scan(&sessionUUID, &profile, &world, &uid, &remoteAddr)
	if err != nil {
		t.Fatalf("query session: %v", err)
	}
	if sessionUUID != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("session_uuid: got %q, want %q", sessionUUID, "11111111-2222-3333-4444-555555555555")
	}
	if profile != "main" {
		t.Errorf("profile: got %q, want %q", profile, "main")
	}
	if world != 2 {
		t.Errorf("world: got %d, want 2", world)
	}
	if uid != 42 {
		t.Errorf("uid: got %d, want 42", uid)
	}
	if remoteAddr != "192.168.0.1" {
		t.Errorf("remote_address: got %q, want %q", remoteAddr, "192.168.0.1")
	}
}

func TestClearWorldSessions(t *testing.T) {
	db := createTestDB(t)
	id := insertTestAccount(t, db, "clearuser", "pass")

	_, err := db.ExecContext(t.Context(),
		db.Rebind(`INSERT INTO account_login (account_id, profile, node_id, logged_in) VALUES (?, ?, ?, ?)`),
		id, "main", 4, 1,
	)
	if err != nil {
		t.Fatalf("insert account_login: %v", err)
	}

	err = clearWorldSessions(t.Context(), db, 4, "main")
	if err != nil {
		t.Fatalf("clearWorldSessions: %v", err)
	}

	var loggedIn int
	err = db.QueryRowContext(t.Context(),
		db.Rebind(`SELECT logged_in FROM account_login WHERE account_id = ? AND profile = ?`),
		id, "main",
	).Scan(&loggedIn)
	if err != nil {
		t.Fatalf("query account_login: %v", err)
	}
	if loggedIn != 0 {
		t.Errorf("logged_in: got %d, want 0", loggedIn)
	}
}

func TestSetLoggedOut(t *testing.T) {
	db := createTestDB(t)
	id := insertTestAccount(t, db, "logoutuser", "pass")

	// Insert login row with logged_in=1
	err := upsertAccountLogin(t.Context(), db, int(id), "main", 3)
	if err != nil {
		t.Fatalf("upsertAccountLogin: %v", err)
	}

	err = setLoggedOut(t.Context(), db, int(id), "main")
	if err != nil {
		t.Fatalf("setLoggedOut: %v", err)
	}

	var loggedIn int
	err = db.QueryRowContext(t.Context(),
		db.Rebind(`SELECT logged_in FROM account_login WHERE account_id = ? AND profile = ?`),
		id, "main",
	).Scan(&loggedIn)
	if err != nil {
		t.Fatalf("query account_login: %v", err)
	}
	if loggedIn != 0 {
		t.Errorf("logged_in: got %d, want 0", loggedIn)
	}

	var logoutTime sql.NullTime
	err = db.QueryRowContext(t.Context(),
		db.Rebind(`SELECT logout_time FROM account WHERE id = ?`),
		id,
	).Scan(&logoutTime)
	if err != nil {
		t.Fatalf("query account: %v", err)
	}
	if !logoutTime.Valid {
		t.Error("logout_time: expected non-NULL timestamp after setLoggedOut")
	}
}

// TestSetLoggedOut_ClearsRowRegardlessOfNodeId pins login-server-3: the UPDATE
// keys by (account_id, profile) ONLY. TS LoginServer.ts:438-439,484-485 keeps
// the WHERE clause node-agnostic so a logout originating from a different node
// (e.g. force-logout from a sibling world) can still clear a stale login row.
// Pre-fix goscape filtered `AND node_id = ?` and silently no-op'd when the
// caller's nodeID differed from the row's, leaving logged_in=1 forever and
// arming spurious "account is logged in elsewhere" rejects on the next login.
//
// Toggle-revert RED proof: reintroduce `AND node_id = 0` (literal mismatch
// vs the row's node_id=99 seeded below) into the UPDATE; this test then
// reads logged_in=1 and fails with the cited assertion message.
func TestSetLoggedOut_ClearsRowRegardlessOfNodeId(t *testing.T) {
	db := createTestDB(t)
	id := insertTestAccount(t, db, "noderoamuser", "pass")

	// Row seeded by node_id=99 — a different world from the one initiating
	// the logout. Pre-fix WHERE clause demanded the bound nodeID match.
	if err := upsertAccountLogin(t.Context(), db, int(id), "main", 99); err != nil {
		t.Fatalf("upsertAccountLogin: %v", err)
	}

	if err := setLoggedOut(t.Context(), db, int(id), "main"); err != nil {
		t.Fatalf("setLoggedOut: %v", err)
	}

	var loggedIn int
	if err := db.QueryRowContext(t.Context(),
		db.Rebind(`SELECT logged_in FROM account_login WHERE account_id = ? AND profile = ?`),
		id, "main",
	).Scan(&loggedIn); err != nil {
		t.Fatalf("query account_login: %v", err)
	}
	if loggedIn != 0 {
		t.Errorf("logged_in: got %d, want 0; TS LoginServer.ts:438-439,484-485 clears WHERE (account_id, profile) only — setLoggedOut must NOT gate on node_id (login-server-3)", loggedIn)
	}
}

func TestSetAccountBanned(t *testing.T) {
	db := createTestDB(t)
	insertTestAccount(t, db, "banneduser", "pass")

	until := time.Date(2027, 1, 15, 12, 0, 0, 0, time.UTC)
	err := setAccountBanned(t.Context(), db, "banneduser", until)
	if err != nil {
		t.Fatalf("setAccountBanned: %v", err)
	}

	var bannedUntil sql.NullTime
	err = db.QueryRowContext(t.Context(),
		db.Rebind(`SELECT banned_until FROM account WHERE username = ?`),
		"banneduser",
	).Scan(&bannedUntil)
	if err != nil {
		t.Fatalf("query account: %v", err)
	}
	if !bannedUntil.Valid {
		t.Fatal("banned_until should be set")
	}
	if !bannedUntil.Time.UTC().Equal(until) {
		t.Errorf("banned_until: got %v, want %v", bannedUntil.Time, until)
	}
}

func TestSetAccountMuted(t *testing.T) {
	db := createTestDB(t)
	insertTestAccount(t, db, "muteduser", "pass")

	until := time.Date(2027, 3, 20, 8, 30, 0, 0, time.UTC)
	err := setAccountMuted(t.Context(), db, "muteduser", until)
	if err != nil {
		t.Fatalf("setAccountMuted: %v", err)
	}

	var mutedUntil sql.NullTime
	err = db.QueryRowContext(t.Context(),
		db.Rebind(`SELECT muted_until FROM account WHERE username = ?`),
		"muteduser",
	).Scan(&mutedUntil)
	if err != nil {
		t.Fatalf("query account: %v", err)
	}
	if !mutedUntil.Valid {
		t.Fatal("muted_until should be set")
	}
	if !mutedUntil.Time.UTC().Equal(until) {
		t.Errorf("muted_until: got %v, want %v", mutedUntil.Time, until)
	}
}

// TestSessionUUIDCheckRejectsNonUUID pins the schema-level CHECK
// constraint on session.session_uuid (pkg/gamedb's unified schema).
// A raw INSERT with a non-UUID-shaped value must be rejected.
func TestSessionUUIDCheckRejectsNonUUID(t *testing.T) {
	db := createTestDB(t)
	accountID := insertTestAccount(t, db, "checkrejecttest", "pass")

	_, err := db.ExecContext(t.Context(),
		db.Rebind(`INSERT INTO session (session_uuid, account_id, profile, world, uid, login_time, remote_address)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`),
		"not-a-uuid", accountID, "main", 0, 0, time.Now().UTC(), "127.0.0.1:1234",
	)
	if err == nil {
		t.Fatalf("INSERT with non-UUID session_uuid succeeded; expected CHECK constraint failure")
	}
	if !strings.Contains(err.Error(), "CHECK") && !strings.Contains(err.Error(), "constraint") {
		t.Errorf("error message did not mention CHECK or constraint: %v", err)
	}
}
