package friends

import (
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// createTestDB opens an in-memory SQLite, applies migrations, registers
// cleanup, and returns the *sql.DB. Mirrors modules/login/db_test.go.
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

func TestOpenDB_PragmasApplied(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "friends.db")
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
	dsn := filepath.Join(t.TempDir(), "friends.db")
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
			_, err = tx.Exec(`INSERT INTO friendlist (profile, owner_username37, target_username37) VALUES ('main', ?, 1)`,
				i)
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
	dsn := filepath.Join(t.TempDir(), "friends.db")
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
	if _, err := tx.Exec(`INSERT INTO friendlist (profile, owner_username37, target_username37) VALUES ('main', 100, 1)`); err != nil {
		tx.Rollback()
		t.Fatalf("insert A: %v", err)
	}
	commitErr := make(chan error, 1)
	go func() {
		time.Sleep(200 * time.Millisecond) // hold the lock while B contends
		commitErr <- tx.Commit()
	}()

	// Handle B: must block on busy_timeout until A commits, then succeed.
	if _, err := dbB.Exec(`INSERT INTO friendlist (profile, owner_username37, target_username37) VALUES ('main', 200, 1)`); err != nil {
		t.Fatalf("cross-handle insert B: %v", err)
	}
	if err := <-commitErr; err != nil {
		t.Fatalf("commit A: %v", err)
	}
}

func TestDSNWithPragmas(t *testing.T) {
	got := dsnWithPragmas("data/friends.db")
	want := "data/friends.db?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	if got != want {
		t.Errorf("plain dsn: got %q, want %q", got, want)
	}
	got = dsnWithPragmas("file:x?mode=memory&cache=shared")
	want = "file:x?mode=memory&cache=shared&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	if got != want {
		t.Errorf("param dsn: got %q, want %q", got, want)
	}
}

func TestOpenDB_AppliesMigrations(t *testing.T) {
	db := createTestDB(t)

	wantTables := []string{"friendlist", "ignorelist", "private_chat", "public_chat"}
	for _, name := range wantTables {
		var got string
		err := db.QueryRow(
			`SELECT name FROM sqlite_schema WHERE type='table' AND name=?`,
			name,
		).Scan(&got)
		if err != nil {
			t.Errorf("table %q missing: %v", name, err)
		}
	}
}

func TestOpenDB_Idempotent(t *testing.T) {
	// Open twice against the same in-memory DSN; the second open should
	// hit the migrate.ErrNoChange branch and return nil.
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", url.PathEscape(t.Name()))
	db1, err := openDB(dsn)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	defer db1.Close()
	db2, err := openDB(dsn)
	if err != nil {
		t.Fatalf("second open (idempotent): %v", err)
	}
	defer db2.Close()
}

func TestOpenDB_SetsPragmas(t *testing.T) {
	db := createTestDB(t)

	// foreign_keys should be on (returns 1).
	var fk int
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatalf("query foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("PRAGMA foreign_keys: got %d, want 1", fk)
	}

	// journal_mode for in-memory databases reports "memory" (SQLite
	// rejects WAL on :memory: variants). We assert the PRAGMA returns a
	// non-empty string — sufficient to prove the Exec call did not error.
	var jm string
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&jm); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if jm == "" {
		t.Errorf("PRAGMA journal_mode: got empty, want non-empty")
	}
}

func TestOpenDB_BadDSN(t *testing.T) {
	// modernc.org/sqlite accepts almost any string as a DSN (file path
	// or URI), so we use a path under a non-existent parent directory to
	// guarantee an open failure (the directory /nonexistent-goscape-XXXX
	// will never exist on this host). The malformed pragma URI form
	// "file:?_pragma=garbage(bogus)" is tolerated by modernc.org/sqlite,
	// so a non-writable/missing-parent path is the reliable failure mode.
	_, err := openDB("/nonexistent-goscape-friends-XXXX/x.db")
	if err == nil {
		t.Fatalf("openDB with non-existent parent dir: got nil error, want failure")
	}
}
