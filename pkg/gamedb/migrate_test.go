package gamedb

import (
	"fmt"
	"path/filepath"
	"testing"
)

// migratedTestDB opens an isolated in-memory DB and applies the full
// lineage.
func migratedTestDB(t *testing.T) *DB {
	t.Helper()
	db := openTestDB(t)
	if err := db.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return db
}

func TestMigrate_CreatesAllTables(t *testing.T) {
	db := migratedTestDB(t)
	for _, table := range []string{
		"account", "account_login", "session", "ipban",
		"hiscore", "hiscore_large", "login",
		"message_thread", "message", "message_status",
		"account_session", "wealth_event",
		"friendlist", "ignorelist",
	} {
		var n int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table,
		).Scan(&n); err != nil {
			t.Fatalf("sqlite_master(%s): %v", table, err)
		}
		if n != 1 {
			t.Errorf("table %s: not created", table)
		}
	}
}

// TestMigrate_ChatTablesDropped pins migration 000002: chat is
// Kafka-only (spec docs/superpowers/specs/2026-07-07-chat-kafka-only-design.md),
// so a fully-migrated schema has neither public_chat nor private_chat.
func TestMigrate_ChatTablesDropped(t *testing.T) {
	db := migratedTestDB(t) // same helper TestMigrate_CreatesAllTables uses
	for _, tbl := range []string{"public_chat", "private_chat"} {
		var n int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, tbl,
		).Scan(&n); err != nil {
			t.Fatalf("sqlite_master query: %v", err)
		}
		if n != 0 {
			t.Errorf("table %s still exists, want dropped", tbl)
		}
	}
}

// seedAccount inserts a bare account row and returns its id.
// INSERT ... RETURNING is dialect-portable (SQLite >= 3.35, Postgres).
func seedAccount(t *testing.T, db *DB, username string) int64 {
	t.Helper()
	var id int64
	err := db.QueryRowContext(t.Context(),
		db.Rebind(`INSERT INTO account (username, password) VALUES (?, '') RETURNING id`),
		username,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seedAccount(%s): %v", username, err)
	}
	return id
}

func TestMigrate_AccountDeleteCascades(t *testing.T) {
	db := migratedTestDB(t)
	ctx := t.Context()
	owner := seedAccount(t, db, "owner")
	friend := seedAccount(t, db, "friend")

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, db.Rebind(q), args...); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	mustExec(`INSERT INTO friendlist (profile, account_id, friend_account_id) VALUES ('main', ?, ?)`, owner, friend)
	mustExec(`INSERT INTO ignorelist (profile, account_id, value) VALUES ('main', ?, 'ghost')`, owner)
	mustExec(`INSERT INTO hiscore (account_id, type, level, value, date) VALUES (?, 0, 3, 1154, '2026-07-05 00:00:00')`, owner)

	mustExec(`DELETE FROM account WHERE id = ?`, owner)

	for _, tc := range []struct {
		table, where string
		arg          int64
		want         int
	}{
		{"friendlist", "account_id", owner, 0},
		{"ignorelist", "account_id", owner, 0},
		{"hiscore", "account_id", owner, 0},
		// friend still exists; the friend-side row died with owner.
		{"friendlist", "friend_account_id", friend, 0},
	} {
		var n int
		q := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE %s = ?`, tc.table, tc.where)
		if err := db.QueryRowContext(ctx, db.Rebind(q), tc.arg).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", tc.table, err)
		}
		if n != tc.want {
			t.Errorf("%s.%s=%d: got %d rows, want %d (ON DELETE CASCADE)", tc.table, tc.where, tc.arg, n, tc.want)
		}
	}
}

func TestMigrate_FriendlistRejectsUnknownAccount(t *testing.T) {
	db := migratedTestDB(t)
	_, err := db.ExecContext(t.Context(),
		db.Rebind(`INSERT INTO friendlist (profile, account_id, friend_account_id) VALUES ('main', 999, 998)`))
	if err == nil {
		t.Fatal("insert with unknown account ids: got nil error, want FK violation")
	}
	if !IsForeignKeyViolation(err) {
		t.Errorf("IsForeignKeyViolation(%v): got false, want true", err)
	}
	if IsForeignKeyViolation(nil) {
		t.Error("IsForeignKeyViolation(nil): got true, want false")
	}
}

func TestMigrate_IgnorelistValueIsFreeString(t *testing.T) {
	// TS lets you ignore usernames that don't exist
	// (FriendServerRepository.ts:249-294 never resolves the target).
	db := migratedTestDB(t)
	owner := seedAccount(t, db, "owner")
	if _, err := db.ExecContext(t.Context(),
		db.Rebind(`INSERT INTO ignorelist (profile, account_id, value) VALUES ('main', ?, 'nonexistent player')`),
		owner,
	); err != nil {
		t.Fatalf("ignore of nonexistent username must succeed: %v", err)
	}
}

func TestMigrate_SessionUUIDShapeEnforced(t *testing.T) {
	db := migratedTestDB(t)
	acc := seedAccount(t, db, "owner")
	// Clean break: the legacy '' allowance is gone.
	_, err := db.ExecContext(t.Context(),
		db.Rebind(`INSERT INTO session (session_uuid, account_id, profile, login_time) VALUES ('', ?, 'main', '2026-07-05 00:00:00')`), acc)
	if err == nil {
		t.Fatal("empty session_uuid: got nil error, want CHECK violation")
	}
	if _, err := db.ExecContext(t.Context(),
		db.Rebind(`INSERT INTO session (session_uuid, account_id, profile, login_time) VALUES ('01234567-89ab-cdef-0123-456789abcdef', ?, 'main', '2026-07-05 00:00:00')`), acc); err != nil {
		t.Fatalf("well-formed session_uuid rejected: %v", err)
	}
}

// TestIndependentClients pins the architecture model: two separate
// pools (as the login and friends services would each hold) against
// one central database file — writes through one visible to the other,
// WAL + busy_timeout mediating.
func TestIndependentClients_TwoPoolsOneDatabase(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "goscape.db")
	cfg := defaultConfig()
	cfg.SQLite.DSN = dsn

	migrator, err := Open(cfg, noopLogger())
	if err != nil {
		t.Fatalf("Open(migrator): %v", err)
	}
	if err := migrator.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := migrator.Close(); err != nil {
		t.Fatalf("Close(migrator): %v", err)
	}

	loginPool, err := Open(cfg, noopLogger())
	if err != nil {
		t.Fatalf("Open(login pool): %v", err)
	}
	defer loginPool.Close()
	friendsPool, err := Open(cfg, noopLogger())
	if err != nil {
		t.Fatalf("Open(friends pool): %v", err)
	}
	defer friendsPool.Close()

	// "login" creates the account …
	id := seedAccount(t, loginPool, "adventurer")
	// … "friends" resolves it through its own pool.
	var got int64
	if err := friendsPool.QueryRowContext(t.Context(),
		friendsPool.Rebind(`SELECT id FROM account WHERE username = ?`), "adventurer",
	).Scan(&got); err != nil {
		t.Fatalf("friends pool read: %v", err)
	}
	if got != id {
		t.Errorf("cross-pool read: got id %d, want %d", got, id)
	}
}

func TestMigrate_SecondRunNoChange(t *testing.T) {
	db := migratedTestDB(t)
	if err := db.Migrate(t.Context()); err != nil {
		t.Fatalf("second Migrate: %v (want ErrNoChange swallowed)", err)
	}
}

// TestMigrate_HiscoreIndexes pins the ranking indexes added in 000004.
// The hiscore API's leaderboard ordering and rank counting both depend
// on them; without them every leaderboard page is a full sort.
func TestMigrate_HiscoreIndexes(t *testing.T) {
	db := migratedTestDB(t)

	want := []string{
		"idx_hiscore_rank",
		"idx_hiscore_account",
		"idx_hiscore_large_rank",
		"idx_hiscore_large_account",
	}
	for _, name := range want {
		var got string
		err := db.QueryRowContext(t.Context(), db.Rebind(
			`SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`), name).Scan(&got)
		if err != nil {
			t.Fatalf("index %s missing after migrate: %v", name, err)
		}
	}
}
