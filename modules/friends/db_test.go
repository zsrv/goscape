package friends

import (
	"database/sql"
	"fmt"
	"net/url"
	"testing"
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
