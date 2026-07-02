package login

import (
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	sqlitedriver "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"golang.org/x/crypto/bcrypt"
)

func createTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", url.PathEscape(t.Name()))
	db, err := openDB(dsn)
	if err != nil {
		t.Fatalf("createTestDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// insertTestAccount inserts a test account with bcrypt-hashed password (cost 4 for speed).
// Returns the inserted account ID.
func insertTestAccount(t *testing.T, db *sql.DB, username, password string) int64 {
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

func TestOpenDB_PragmasApplied(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "login.db")
	db, err := openDB(dsn)
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer db.Close()

	var busy int
	if err := db.QueryRow(`PRAGMA busy_timeout`).Scan(&busy); err != nil {
		t.Fatalf("query busy_timeout: %v", err)
	}
	if busy != 5000 {
		t.Errorf("busy_timeout: got %d, want 5000", busy)
	}
	var fk int
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatalf("query foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys: got %d, want 1", fk)
	}
}

// TestOpenDB_ConcurrentWriteTxs pins the pool-serialization half of the
// arch-28.1 fix contract: concurrent write transactions on ONE handle must
// serialize at the database/sql pool layer (SetMaxOpenConns(1)) instead of
// failing SQLITE_BUSY. Pre-fix this fails with "database is locked" almost
// every run (the unbounded pool handed each tx its own connection). The
// busy_timeout half — contention across SEPARATE handles, which the pool
// cap cannot serialize — is pinned by TestOpenDB_BusyTimeoutCrossHandle.
func TestOpenDB_ConcurrentWriteTxs(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "login.db")
	db, err := openDB(dsn)
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer db.Close()

	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := range 20 {
		wg.Go(func() {
			tx, err := db.BeginTx(t.Context(), nil)
			if err != nil {
				errs <- err
				return
			}
			_, err = tx.Exec(`INSERT INTO ipban (ip, added_by) VALUES (?, 'test')`,
				fmt.Sprintf("10.0.0.%d", i))
			if err != nil {
				tx.Rollback()
				errs <- err
				return
			}
			time.Sleep(5 * time.Millisecond) // widen the write-lock hold
			errs <- tx.Commit()
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent write tx: %v", err)
		}
	}
}

// TestOpenDB_BusyTimeoutCrossHandle pins the busy_timeout half of the
// arch-28.1 fix contract: contention across SEPARATE *sql.DB handles on the
// same database file — which SetMaxOpenConns(1) cannot serialize, because
// each handle owns its own single-connection pool (the real-world shape:
// goscape-cli or an operator's sqlite3 shell alongside the server). Handle A
// holds the write lock in an open transaction; handle B's INSERT must block
// on busy_timeout(5000) until A commits, then succeed. Pre-fix
// (busy_timeout=0) handle B fails immediately with SQLITE_BUSY
// ("database is locked").
func TestOpenDB_BusyTimeoutCrossHandle(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "login.db")
	dbA, err := openDB(dsn)
	if err != nil {
		t.Fatalf("openDB A: %v", err)
	}
	defer dbA.Close()
	dbB, err := openDB(dsn)
	if err != nil {
		t.Fatalf("openDB B: %v", err)
	}
	defer dbB.Close()

	// Handle A: take and hold the write lock.
	tx, err := dbA.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("begin tx A: %v", err)
	}
	if _, err := tx.Exec(`INSERT INTO ipban (ip, added_by) VALUES ('10.1.0.1', 'test')`); err != nil {
		tx.Rollback()
		t.Fatalf("insert A: %v", err)
	}
	commitErr := make(chan error, 1)
	go func() {
		time.Sleep(200 * time.Millisecond) // hold the lock while B contends
		commitErr <- tx.Commit()
	}()

	// Handle B: must block on busy_timeout until A commits, then succeed.
	if _, err := dbB.Exec(`INSERT INTO ipban (ip, added_by) VALUES ('10.1.0.2', 'test')`); err != nil {
		t.Fatalf("cross-handle insert B: %v", err)
	}
	if err := <-commitErr; err != nil {
		t.Fatalf("commit A: %v", err)
	}
}

func TestDSNWithPragmas(t *testing.T) {
	got := dsnWithPragmas("data/login.db")
	want := "data/login.db?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	if got != want {
		t.Errorf("plain dsn: got %q, want %q", got, want)
	}
	got = dsnWithPragmas("file:x?mode=memory&cache=shared")
	want = "file:x?mode=memory&cache=shared&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	if got != want {
		t.Errorf("param dsn: got %q, want %q", got, want)
	}
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
		`INSERT INTO account_login (account_id, profile, node_id, logged_in) VALUES (?, ?, ?, ?)`,
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
		`INSERT INTO ipban (ip, added_by, added_on) VALUES (?, ?, ?)`,
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
		`SELECT logged_in, node_id FROM account_login WHERE account_id = ? AND profile = ?`,
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
		`INSERT INTO account_login (account_id, profile, node_id, logged_in) VALUES (?, ?, ?, ?)`,
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
		`SELECT logged_in, node_id FROM account_login WHERE account_id = ? AND profile = ?`,
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

// TestUpsertAccountLogin_PreservesLogoutState pins that a re-login
// leaves account_login.logged_out/logout_time intact: TS's login-success
// update writes only logged_in + login_time (LoginServer.ts:438-457),
// so the hop-timer inputs survive until the next graceful logout
// overwrites them.
func TestUpsertAccountLogin_PreservesLogoutState(t *testing.T) {
	db := createTestDB(t)
	if _, err := db.Exec(`INSERT INTO account (username, password) VALUES ('bob', 'x')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO account_login (account_id, profile, node_id, logged_in, logged_out, logout_time)
	                      VALUES (1, 'main', 11, 0, 11, '2026-06-01 12:00:00')`); err != nil {
		t.Fatal(err)
	}
	if err := upsertAccountLogin(t.Context(), db, 1, "main", 10); err != nil {
		t.Fatalf("upsertAccountLogin: %v", err)
	}
	var loggedIn, nodeID, loggedOut int
	var logoutTime sql.NullString
	if err := db.QueryRow(`SELECT logged_in, node_id, logged_out, logout_time FROM account_login
	                       WHERE account_id = 1 AND profile = 'main'`).
		Scan(&loggedIn, &nodeID, &loggedOut, &logoutTime); err != nil {
		t.Fatal(err)
	}
	if loggedIn != 1 || nodeID != 10 {
		t.Errorf("upsert: logged_in=%d node_id=%d; want 1, 10", loggedIn, nodeID)
	}
	if loggedOut != 11 || !logoutTime.Valid || logoutTime.String != "2026-06-01 12:00:00" {
		t.Errorf("logout state clobbered: logged_out=%d logout_time=%v; want 11, 2026-06-01 12:00:00", loggedOut, logoutTime)
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
		`SELECT session_uuid, profile, world, uid, remote_address FROM session WHERE account_id = ?`,
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
		`INSERT INTO account_login (account_id, profile, node_id, logged_in) VALUES (?, ?, ?, ?)`,
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
		`SELECT logged_in FROM account_login WHERE account_id = ? AND profile = ?`,
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

	err = setLoggedOut(t.Context(), db, int(id), "main", 3)
	if err != nil {
		t.Fatalf("setLoggedOut: %v", err)
	}

	var loggedIn, loggedOut int
	var logoutTime sql.NullString
	err = db.QueryRowContext(t.Context(),
		`SELECT logged_in, logged_out, logout_time FROM account_login WHERE account_id = ? AND profile = ?`,
		id, "main",
	).Scan(&loggedIn, &loggedOut, &logoutTime)
	if err != nil {
		t.Fatalf("query account_login: %v", err)
	}
	if loggedIn != 0 {
		t.Errorf("logged_in: got %d, want 0", loggedIn)
	}
	if loggedOut != 3 {
		t.Errorf("logged_out: got %d, want 3 (nodeID passed to setLoggedOut)", loggedOut)
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

	if err := setLoggedOut(t.Context(), db, int(id), "main", 0); err != nil {
		t.Fatalf("setLoggedOut: %v", err)
	}

	var loggedIn int
	if err := db.QueryRowContext(t.Context(),
		`SELECT logged_in FROM account_login WHERE account_id = ? AND profile = ?`,
		id, "main",
	).Scan(&loggedIn); err != nil {
		t.Fatalf("query account_login: %v", err)
	}
	if loggedIn != 0 {
		t.Errorf("logged_in: got %d, want 0; TS LoginServer.ts:484-496 clears WHERE (account_id, profile) only — setLoggedOut must NOT gate on node_id (login-server-3)", loggedIn)
	}
}

// TestSetLoggedOut_StampsPerProfile pins login-server-7 closure step iii:
// setLoggedOut writes account_login.logged_out = nodeID and
// account_login.logout_time for THIS (account, profile) row only.
// TS LoginServer.ts:484-496.
func TestSetLoggedOut_StampsPerProfile(t *testing.T) {
	db := createTestDB(t)
	if _, err := db.Exec(`INSERT INTO account (username, password) VALUES ('bob', 'x')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO account_login (account_id, profile, node_id, logged_in)
	                      VALUES (1, 'main', 10, 1), (1, 'beta', 11, 1)`); err != nil {
		t.Fatal(err)
	}
	if err := setLoggedOut(t.Context(), db, 1, "main", 10); err != nil {
		t.Fatalf("setLoggedOut: %v", err)
	}
	var loggedIn, loggedOut int
	var logoutTime sql.NullString
	if err := db.QueryRow(`SELECT logged_in, logged_out, logout_time FROM account_login
	                       WHERE account_id = 1 AND profile = 'main'`).
		Scan(&loggedIn, &loggedOut, &logoutTime); err != nil {
		t.Fatal(err)
	}
	if loggedIn != 0 || loggedOut != 10 || !logoutTime.Valid {
		t.Errorf("main row: logged_in=%d logged_out=%d logout_time.Valid=%v; want 0, 10, true",
			loggedIn, loggedOut, logoutTime.Valid)
	}
	if err := db.QueryRow(`SELECT logged_in, logged_out, logout_time FROM account_login
	                       WHERE account_id = 1 AND profile = 'beta'`).
		Scan(&loggedIn, &loggedOut, &logoutTime); err != nil {
		t.Fatal(err)
	}
	if loggedIn != 1 || loggedOut != 0 || logoutTime.Valid {
		t.Errorf("beta row touched: logged_in=%d logged_out=%d logout_time.Valid=%v; want 1, 0, false",
			loggedIn, loggedOut, logoutTime.Valid)
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

	var bannedUntil sql.NullString
	err = db.QueryRowContext(t.Context(),
		`SELECT banned_until FROM account WHERE username = ?`,
		"banneduser",
	).Scan(&bannedUntil)
	if err != nil {
		t.Fatalf("query account: %v", err)
	}
	if !bannedUntil.Valid {
		t.Fatal("banned_until should be set")
	}
	expected := until.Format(dbTimeFormat)
	if bannedUntil.String != expected {
		t.Errorf("banned_until: got %q, want %q", bannedUntil.String, expected)
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

	var mutedUntil sql.NullString
	err = db.QueryRowContext(t.Context(),
		`SELECT muted_until FROM account WHERE username = ?`,
		"muteduser",
	).Scan(&mutedUntil)
	if err != nil {
		t.Fatalf("query account: %v", err)
	}
	if !mutedUntil.Valid {
		t.Fatal("muted_until should be set")
	}
	expected := until.Format(dbTimeFormat)
	if mutedUntil.String != expected {
		t.Errorf("muted_until: got %q, want %q", mutedUntil.String, expected)
	}
}

// TestSessionUUIDCheckRejectsNonUUID pins the schema-level CHECK
// constraint on session.session_uuid added by migration 000002.
// Pre-migration, a raw INSERT with a non-UUID-shaped value succeeds
// (no CHECK exists); post-migration, the same insert errors with a
// CHECK constraint failure. Closes B3 from the post-friends-arc
// cleanup batch.
func TestSessionUUIDCheckRejectsNonUUID(t *testing.T) {
	db := createTestDB(t)
	accountID := insertTestAccount(t, db, "checkrejecttest", "pass")

	_, err := db.ExecContext(t.Context(),
		`INSERT INTO session (session_uuid, account_id, profile, world, uid, login_time, remote_address)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"not-a-uuid", accountID, "main", 0, 0, "2026-05-19T00:00:00Z", "127.0.0.1:1234",
	)
	if err == nil {
		t.Fatalf("INSERT with non-UUID session_uuid succeeded; expected CHECK constraint failure")
	}
	if !strings.Contains(err.Error(), "CHECK") && !strings.Contains(err.Error(), "constraint") {
		t.Errorf("error message did not mention CHECK or constraint: %v", err)
	}
}

// TestSessionUUIDCheckAcceptsEmpty pins that the schema CHECK
// constraint permits the empty string. This is the coercion target
// used by migration 000002 for pre-slice-7 rows whose session_uuid
// held RemoteAddr().String() instead of a UUID.
func TestSessionUUIDCheckAcceptsEmpty(t *testing.T) {
	db := createTestDB(t)
	accountID := insertTestAccount(t, db, "checkemptytest", "pass")

	_, err := db.ExecContext(t.Context(),
		`INSERT INTO session (session_uuid, account_id, profile, world, uid, login_time, remote_address)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"", accountID, "main", 0, 0, "2026-05-19T00:00:00Z", "127.0.0.1:1234",
	)
	if err != nil {
		t.Fatalf("INSERT with empty session_uuid failed: %v", err)
	}
}

// TestMigration002CoercesLegacyRows exercises the legacy-data path
// in migration 000002. Open a fresh DB at schema version 1 only
// (so the session table has no CHECK), insert a pre-slice-7-style
// row with session_uuid = RemoteAddr().String(), then advance to
// version 2 and assert:
//   - the legacy session_uuid is coerced to ""
//   - other columns are preserved
//   - the id is preserved (AUTOINCREMENT pass-through)
//   - a fresh insertSession on the upgraded table yields a larger id
//     (sqlite_sequence continuity)
func TestMigration002CoercesLegacyRows(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", url.PathEscape(t.Name()))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	src, err := iofs.New(migrations, "migrations")
	if err != nil {
		t.Fatalf("iofs.New: %v", err)
	}
	drv, err := sqlitedriver.WithInstance(db, &sqlitedriver.Config{})
	if err != nil {
		t.Fatalf("driver: %v", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "sqlite", drv)
	if err != nil {
		t.Fatalf("migrate instance: %v", err)
	}
	// Advance exactly one step from the empty starting state — applies
	// 000001_init only. The session table now exists with NO CHECK.
	if err := m.Steps(1); err != nil {
		t.Fatalf("migrate.Steps(1): %v", err)
	}

	// Set up the account row that the legacy session row's FK
	// references.
	hashed, err := bcrypt.GenerateFromPassword([]byte("pw"), 4)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	accountID, err := insertAccount(t.Context(), db, "legacyuser", string(hashed), "127.0.0.1")
	if err != nil {
		t.Fatalf("insertAccount: %v", err)
	}

	// Insert a pre-slice-7-style row directly: session_uuid holds an
	// IP:port string (what RemoteAddr().String() produces).
	legacyUUIDValue := "127.0.0.1:42193"
	legacyRemoteAddr := "127.0.0.1:42193"
	legacyLoginTime := "2024-12-01T00:00:00Z"
	res, err := db.ExecContext(t.Context(),
		`INSERT INTO session (session_uuid, account_id, profile, world, uid, login_time, remote_address)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		legacyUUIDValue, accountID, "main", 3, 99, legacyLoginTime, legacyRemoteAddr,
	)
	if err != nil {
		t.Fatalf("legacy INSERT: %v", err)
	}
	legacyID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}

	// Now advance to version 2 — applies 000002 which rebuilds the
	// table with CHECK and coerces the legacy row.
	if err := m.Up(); err != nil {
		t.Fatalf("migrate.Up: %v", err)
	}

	// Read the legacy row back.
	var (
		gotUUID, gotProfile, gotRemoteAddr, gotLoginTime string
		gotWorld, gotUID                                 int
		gotAccountID                                     int64
	)
	err = db.QueryRowContext(t.Context(),
		`SELECT session_uuid, account_id, profile, world, uid, login_time, remote_address
		   FROM session WHERE id = ?`,
		legacyID,
	).Scan(&gotUUID, &gotAccountID, &gotProfile, &gotWorld, &gotUID, &gotLoginTime, &gotRemoteAddr)
	if err != nil {
		t.Fatalf("SELECT post-migration: %v", err)
	}
	if gotUUID != "" {
		t.Errorf("legacy session_uuid: got %q, want \"\"", gotUUID)
	}
	if gotAccountID != accountID {
		t.Errorf("account_id: got %d, want %d", gotAccountID, accountID)
	}
	if gotProfile != "main" {
		t.Errorf("profile: got %q, want %q", gotProfile, "main")
	}
	if gotWorld != 3 {
		t.Errorf("world: got %d, want 3", gotWorld)
	}
	if gotUID != 99 {
		t.Errorf("uid: got %d, want 99", gotUID)
	}
	if gotLoginTime != legacyLoginTime {
		t.Errorf("login_time: got %q, want %q", gotLoginTime, legacyLoginTime)
	}
	if gotRemoteAddr != legacyRemoteAddr {
		t.Errorf("remote_address: got %q, want %q", gotRemoteAddr, legacyRemoteAddr)
	}

	// AUTOINCREMENT continuity: a fresh insertSession on the upgraded
	// table must yield an id strictly greater than legacyID.
	err = insertSession(t.Context(), db, "22222222-3333-4444-5555-666666666666", int(accountID), "main", 1, 1, "127.0.0.1:1")
	if err != nil {
		t.Fatalf("insertSession post-migration: %v", err)
	}
	var newID int64
	err = db.QueryRowContext(t.Context(),
		`SELECT id FROM session WHERE session_uuid = ?`,
		"22222222-3333-4444-5555-666666666666",
	).Scan(&newID)
	if err != nil {
		t.Fatalf("SELECT new row id: %v", err)
	}
	if newID <= legacyID {
		t.Errorf("AUTOINCREMENT continuity broken: new id %d <= legacy id %d", newID, legacyID)
	}
}

// TestMigration000005_Schema pins the rev-244 B5 schema surface: the
// `login` attempts table (TS prisma model `login`), the per-profile
// account_login.logged_out/logout_time columns (TS prisma account_login),
// the message_thread/message/message_status tables backing
// getUnreadMessageCount (TS Messages.ts), the dormant account_session /
// wealth_event landing tables (user decision: schema-only, no Go writer),
// and the account.logout_time drop (login-server-7 closure step v).
func TestMigration000005_Schema(t *testing.T) {
	db := createTestDB(t)

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	mustExec(`INSERT INTO account (username, password) VALUES ('a', 'x')`)
	mustExec(`INSERT INTO login (uuid, account_id, world, timestamp, uid, ip)
	          VALUES ('u-1', 1, 10, '2026-06-05 00:00:00', 7, '1.2.3.4')`)
	mustExec(`INSERT INTO account_login (account_id, profile, node_id, logged_in, logged_out, logout_time)
	          VALUES (1, 'main', 10, 0, 10, '2026-06-05 00:00:00')`)
	mustExec(`INSERT INTO message_thread (to_account_id, from_account_id, last_message_from, subject)
	          VALUES (2, 1, 1, 's')`)
	mustExec(`INSERT INTO message (thread_id, sender_id, sender_ip, content)
	          VALUES (1, 1, '1.2.3.4', 'hello')`)
	mustExec(`INSERT INTO message_status (thread_id, account_id, "read", deleted)
	          VALUES (1, 2, NULL, NULL)`)
	mustExec(`INSERT INTO account_session (account_id, world, profile, session_uuid, timestamp, coord, event, event_type)
	          VALUES (1, 10, 'main', 's-1', '2026-06-05 00:00:00', 0, 'e', -1)`)
	mustExec(`INSERT INTO wealth_event (timestamp, coord, world, profile, event_type,
	          account_id, account_session, account_items, account_value)
	          VALUES ('2026-06-05 00:00:00', 0, 10, 'main', -1, 1, 's-1', '[]', 0)`)

	// account.logout_time is GONE (login-server-7 step v).
	if _, err := db.Exec(`SELECT logout_time FROM account`); err == nil {
		t.Errorf("account.logout_time still exists; migration 000005 must drop it")
	}

	// FK integrity after the migration.
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Errorf("foreign_key_check reported violations")
	}
}

// TestMigration000005_Backfill pins login-server-7 closure steps i-iii:
// pre-migration account.logout_time values land on EVERY existing
// (account_id, profile) account_login row; logged_out backfills 0 (the
// origin node was never recorded; the hop timer's `logged_out != 0`
// gate treats 0 as no-block, TS LoginServer.ts:368).
func TestMigration000005_Backfill(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	for _, pragma := range []string{`PRAGMA journal_mode=WAL`, `PRAGMA foreign_keys=ON`} {
		if _, err := db.Exec(pragma); err != nil {
			t.Fatalf("%s: %v", pragma, err)
		}
	}
	src, err := iofs.New(migrations, "migrations")
	if err != nil {
		t.Fatalf("iofs: %v", err)
	}
	drv, err := sqlitedriver.WithInstance(db, &sqlitedriver.Config{})
	if err != nil {
		t.Fatalf("driver: %v", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "sqlite", drv)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := m.Migrate(4); err != nil {
		t.Fatalf("migrate to 4: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO account (username, password, logout_time)
	                      VALUES ('bob', 'x', '2026-06-01 12:00:00')`); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO account_login (account_id, profile, node_id, logged_in)
	                      VALUES (1, 'main', 10, 0), (1, 'beta', 11, 0)`); err != nil {
		t.Fatalf("seed account_login: %v", err)
	}
	if err := m.Migrate(5); err != nil {
		t.Fatalf("migrate to 5: %v", err)
	}
	rows, err := db.Query(`SELECT profile, logged_out, COALESCE(logout_time, '') FROM account_login ORDER BY profile`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	got := map[string][2]string{}
	for rows.Next() {
		var profile, lt string
		var lo int
		if err := rows.Scan(&profile, &lo, &lt); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[profile] = [2]string{fmt.Sprint(lo), lt}
	}
	for _, profile := range []string{"beta", "main"} {
		want := [2]string{"0", "2026-06-01 12:00:00"}
		if got[profile] != want {
			t.Errorf("backfill %s: got %v, want %v", profile, got[profile], want)
		}
	}
}
