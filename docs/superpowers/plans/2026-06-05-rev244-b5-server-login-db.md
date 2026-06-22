# rev-244 B5 — server/login/db Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the 225→244 server/login/db delta: login-server rate limiting (3-in-5s + 45s hop timer), real messageCount, friends multi-profile + public_chat re-key, logger report re-key, consumer-backed schema deltas, and the B5 decision rows.

**Architecture:** Spec `docs/superpowers/specs/2026-06-05-rev244-b5-server-login-db-design.md` (commit `6ab81c33`). Schema lands first (everything reads it); then login flow; then protos+wire; then friends; then logger; then docs+gates. All TS citations refer to the 244 pin `9aadcec4` in `$HOME/Code/github.com/LostCityRS/Engine-TS`.

**Tech Stack:** Go, SQLite (modernc.org/sqlite, golang-migrate iofs), gRPC/protobuf (`make protos` = buf generate), dskit modules.

**Worker ground rules (B2-B4-proven, bake into every implementer prompt):**
- Every go command: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache` prefix. Build with `CGO_ENABLED=0 go build -trimpath ./...`; `-race` needs `CGO_ENABLED=1`.
- Every commit: `git commit --no-gpg-sign` + `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>` trailer.
- Verify every `// TS <File>.ts:<lines>` citation against `git -C ../Engine-TS show 9aadcec4:<file> | cat -n` BEFORE writing.
- Sandbox `git status` shows phantom `??` dotfiles — never stage them; never `git add -A`. Stage explicit paths only.
- Stale LSP diagnostics routinely false-alarm whole files after interface changes — trust real build/vet/test runs only.
- Reject-path tests must seed earlier-gate prerequisites (e.g. a hop-timer test must use a valid password and pass the rate limit).
- Run the modules/world suite in any task that touches a contract world tests exercise (Tasks 3, 8, 10, 11).

---

### Task 1: Login migration 000005 — rate-limit/hop-timer/message/dormant schema

**Files:**
- Create: `modules/login/migrations/000005_rev244_b5.up.sql`
- Test: `modules/login/db_test.go` (append)

- [ ] **Step 1: Write the failing migration tests**

Append to `modules/login/db_test.go` (it already has `createTestDB(t)` which opens an in-memory DB and runs all migrations — check its exact name/shape first and reuse):

```go
// TestMigration000005_Schema pins the rev-244 B5 schema surface: the
// `login` attempts table (TS prisma model `login`), the per-profile
// account_login.logged_out/logout_time columns (TS prisma account_login),
// the message_thread/message/message_status tables backing
// getUnreadMessageCount (TS Messages.ts), the dormant account_session /
// wealth_event landing tables (user decision: schema-only, no Go writer),
// and the account.logout_time drop (login-server-7 closure step v).
func TestMigration000005_Schema(t *testing.T) {
	db := createTestDB(t)

	// New tables exist and are insertable with their full column sets.
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
	// Open a raw DB and migrate to version 4 only, seed 225-era rows,
	// then migrate to 5 and assert the backfill.
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
```

Add the needed imports to db_test.go if absent: `database/sql`, `fmt`, `github.com/golang-migrate/migrate/v4`, `sqlitedriver "github.com/golang-migrate/migrate/v4/database/sqlite"`, `github.com/golang-migrate/migrate/v4/source/iofs`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ -run 'TestMigration000005' -v`
Expected: FAIL — `no migration found for version 5` / `no such table: login`.

- [ ] **Step 3: Write the migration**

Create `modules/login/migrations/000005_rev244_b5.up.sql`:

```sql
-- rev-244 B5: login-server rate limit + hop timer + messageCount +
-- dormant logger landing tables. Mirrors the 244 prisma singleworld
-- schema delta (Engine-TS prisma/singleworld/schema.prisma at 9aadcec4).
-- Spec: docs/superpowers/specs/2026-06-05-rev244-b5-server-login-db-design.md.

-- 1. Per-attempt login log (TS prisma model `login`, "attempts
-- (monitoring abuse)"). uuid is goscape's per-attempt sessionUUID — the
-- stand-in for TS's per-socket uuid (LoginServer.ts:260 `uuid: socket`),
-- same one-row-per-attempt cardinality. The composite index backs the
-- 3-in-5s window scan (LoginServer.ts:235-242).
CREATE TABLE login (
    uuid       TEXT    NOT NULL PRIMARY KEY,
    account_id INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    world      INTEGER NOT NULL,
    timestamp  TEXT    NOT NULL,
    uid        INTEGER NOT NULL DEFAULT 0,
    ip         TEXT
);

CREATE INDEX idx_login_account_ip_time ON login (account_id, ip, timestamp);

-- 2. login-server-7 closure (steps i-iii): per-profile logged_out node
-- id + logout_time on account_login (TS prisma account_login
-- logged_out/logout_time; the 45s hop timer reads both,
-- LoginServer.ts:366-371). SQLite cannot ALTER TABLE ADD COLUMN with a
-- FK-preserving backfill in one statement set against the PK shape, so
-- re-create per the 000002/000003 precedent. logged_out backfills 0
-- (origin node never recorded pre-244; the hop timer's `!= 0` gate
-- treats it as no-block). logout_time backfills from the per-account
-- column it replaces.
CREATE TABLE account_login_new (
    account_id  INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    profile     TEXT    NOT NULL,
    node_id     INTEGER NOT NULL DEFAULT 0,
    logged_in   INTEGER NOT NULL DEFAULT 0,
    logged_out  INTEGER NOT NULL DEFAULT 0,
    logout_time TEXT,
    PRIMARY KEY (account_id, profile)
);

INSERT INTO account_login_new (account_id, profile, node_id, logged_in, logged_out, logout_time)
SELECT al.account_id, al.profile, al.node_id, al.logged_in, 0, a.logout_time
FROM account_login al
JOIN account a ON a.id = al.account_id;

DROP TABLE account_login;

ALTER TABLE account_login_new RENAME TO account_login;

-- 3. login-server-7 closure (step v): drop the per-account column.
-- SQLite >= 3.35 supports DROP COLUMN for plain unindexed columns.
ALTER TABLE account DROP COLUMN logout_time;

-- 4. Message-centre tables backing getUnreadMessageCount (TS
-- Messages.ts; prisma models message_thread / message / message_status).
-- message_tag and tag are website-only (no goscape consumer) — see the
-- B5 NOT-PORTED rows in PORTING.md.
CREATE TABLE message_thread (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    to_account_id     INTEGER,
    from_account_id   INTEGER NOT NULL,
    last_message_from INTEGER NOT NULL,
    subject           TEXT    NOT NULL,
    created           TEXT    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated           TEXT    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    messages          INTEGER NOT NULL DEFAULT 1,
    closed            TEXT,
    closed_by         INTEGER,
    marked_spam       TEXT,
    marked_spam_by    INTEGER
);

CREATE TABLE message (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    thread_id  INTEGER NOT NULL,
    sender_id  INTEGER NOT NULL,
    sender_ip  TEXT    NOT NULL,
    content    TEXT    NOT NULL,
    created    TEXT    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    edited     TEXT,
    edited_by  INTEGER,
    deleted    TEXT,
    deleted_by INTEGER
);

CREATE TABLE message_status (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    thread_id  INTEGER NOT NULL,
    account_id INTEGER NOT NULL,
    "read"     TEXT,
    deleted    TEXT
);

-- 5. Dormant logger landing tables (user decision, spec §User decisions
-- 1): account_session replaces TS session_log, wealth_event replaces
-- session_wealth (prisma models at 9aadcec4). NO Go reader or writer in
-- this public repo — the logger sink is slog-only
-- (modules/world/logger_bridge.go); the private sibling owns consumers.
CREATE TABLE account_session (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id   INTEGER NOT NULL,
    world        INTEGER NOT NULL DEFAULT 0,
    profile      TEXT    NOT NULL DEFAULT 'main',
    session_uuid TEXT    NOT NULL,
    timestamp    TEXT    NOT NULL,
    coord        INTEGER NOT NULL,
    event        TEXT    NOT NULL,
    event_type   INTEGER NOT NULL DEFAULT -1
);

CREATE TABLE wealth_event (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp         TEXT    NOT NULL,
    coord             INTEGER NOT NULL,
    world             INTEGER NOT NULL DEFAULT 0,
    profile           TEXT    NOT NULL DEFAULT 'main',
    event_type        INTEGER NOT NULL DEFAULT -1,
    account_id        INTEGER NOT NULL,
    account_session   TEXT    NOT NULL,
    account_items     TEXT    NOT NULL,
    account_value     INTEGER NOT NULL,
    recipient_id      INTEGER,
    recipient_session TEXT,
    recipient_items   TEXT,
    recipient_value   INTEGER
);

CREATE INDEX idx_wealth_event_recipient ON wealth_event (recipient_id);
```

NOTE: at this point `modules/login/db.go` still SELECTs `a.logout_time` in `accountByUsername` and UPDATEs it in `setLoggedOut` — those queries now FAIL against the migrated schema. Task 2 fixes them; to keep THIS task green run only the migration tests (step 4) and expect other package tests to break until Task 2. If the implementer prefers a green package per commit, Tasks 1+2 may land as ONE commit — that is the recommended path; the split here is for review clarity only.

- [ ] **Step 4: Run the migration tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ -run 'TestMigration000005' -v`
Expected: PASS (the two new tests). Other login tests may fail until Task 2 — verify the ONLY failures are logout_time-column queries, then proceed straight to Task 2 before committing (single commit covering Tasks 1+2).

### Task 2: db.go/handler.go — per-profile logged_out/logout_time (login-server-7 closure)

**Files:**
- Modify: `modules/login/db.go` (accountRow, accountByUsername, setLoggedOut; delete the login-server-7 marker block)
- Modify: `modules/login/handler.go` (M25 reject site ~line 232; ban-path no change; PlayerLogout passes NodeId; WorldStartup gains the startup-profile exception marker)
- Test: `modules/login/db_test.go`, `modules/login/handler_test.go`

- [ ] **Step 1: Write failing tests**

```go
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
	// The OTHER profile's row is untouched (per-profile stamp).
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

// TestPlayerLogin_M25_PerProfileLogoutTime pins the re-pointed M25
// safety reject (login-server-7 closure step iv): a missing save with a
// PER-PROFILE logout_time set rejects, while a different profile with
// NULL logout_time (legitimate first login) is admitted. This was the
// login-server-7 latent failure mode — now fixed.
func TestPlayerLogin_M25_PerProfileLogoutTime(t *testing.T) {
	h, _ := newTestHandler(t)
	// Register via a first login on profile "main" (creates the account).
	resp, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId: 10, Profile: "main", Username: "bob", Password: "pw",
		RemoteAddress: "1.2.3.4:5", Uid: 1,
	})
	if err != nil || resp.Result != loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER {
		t.Fatalf("first login: resp=%v err=%v", resp, err)
	}
	// Graceful logout on main with NO save written → logout_time stamped
	// per-profile on main only. (PlayerLogout's persistSaveIfValid skips
	// an invalid save without error.)
	if _, err := h.PlayerLogout(t.Context(), &loginpb.PlayerLogoutRequest{
		NodeId: 10, Profile: "main", Username: "bob", Save: []byte("bad"),
	}); err != nil {
		t.Fatalf("logout: %v", err)
	}
	// main: save missing + logout_time set → DataLoss reject (M25).
	if _, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId: 10, Profile: "main", Username: "bob", Password: "pw",
		RemoteAddress: "1.2.3.4:5", Uid: 1,
	}); status.Code(err) != codes.DataLoss {
		t.Errorf("main relogin: got err %v, want codes.DataLoss", err)
	}
	// beta: same account, no beta logout_time → NEW_PLAYER admitted.
	resp, err = h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId: 10, Profile: "beta", Username: "bob", Password: "pw",
		RemoteAddress: "1.2.3.4:5", Uid: 1,
	})
	if err != nil || resp.Result != loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER {
		t.Errorf("beta first login: resp=%v err=%v; want NEW_PLAYER", resp, err)
	}
}
```

Note: the rate limit (Task 4) does not exist yet, so 2-3 logins in this test stay under it once it lands (3 attempts allowed; this test makes 3 total for bob from one IP — when Task 4 lands, re-check this test still passes; if it makes a 4th attempt, space usernames or IPs).

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ -run 'TestSetLoggedOut_StampsPerProfile|TestPlayerLogin_M25_PerProfileLogoutTime' -v`
Expected: FAIL — `setLoggedOut` has no nodeID param / wrong column writes.

- [ ] **Step 3: Implement**

In `modules/login/db.go`:

1. `accountRow`: add `LoggedOut int` field; `LogoutTime` stays `sql.NullString` but is now sourced per-profile.
2. `accountByUsername`: replace `a.logout_time` with the per-profile columns:

```go
const query = `
SELECT a.id, a.username, a.password, a.staff_mod_level, a.members,
       a.banned_until, a.muted_until,
       al.logout_time,
       COALESCE(al.logged_out, 0),
       COALESCE(al.logged_in, 0),
       COALESCE(al.node_id, 0),
       CASE WHEN al.account_id IS NOT NULL THEN 1 ELSE 0 END as has_login_row
FROM account a
LEFT JOIN account_login al ON al.account_id = a.id AND al.profile = ?
WHERE a.username = ?`
```

Scan order updated to match (`&row.LogoutTime, &row.LoggedOut, &row.LoggedIn, &row.NodeID, &hasLoginRow`). `al.logout_time` is NULL when no login row exists — `sql.NullString` already models that.

3. `setLoggedOut`: new signature `setLoggedOut(ctx, db, accountID int, profile string, nodeID int)`. Replace the two-statement transaction with ONE statement (the per-account stamp is gone):

```go
// setLoggedOut clears the account_login flag and stamps the per-profile
// logged_out origin node + logout_time. Mirrors TS LoginServer.ts:484-496
// (player_logout): logged_in=0, login_time=null (goscape carries no
// login_time column — pre-existing), logged_out=nodeId, logout_time=now,
// keyed by (account_id, profile). The logout_time stamp arms the M25
// "save missing but logout_time set" safety reject on the next login for
// THIS profile, and the logged_out node id feeds the 45s hop timer
// (LoginServer.ts:366-371).
//
// login-server-7 CLOSED (rev-244 B5): logout_time moved from the
// per-account `account.logout_time` column to per-profile
// `account_login.logout_time` (migration 000005 backfilled and dropped
// the legacy column), eliminating the multi-profile spurious-M25-reject
// failure mode documented by the former PORTING-EXCEPTION here.
func setLoggedOut(ctx context.Context, db *sql.DB, accountID int, profile string, nodeID int) error {
	logoutTime := time.Now().UTC().Format(dbTimeFormat)
	if _, err := db.ExecContext(ctx,
		`UPDATE account_login
		 SET logged_in = 0, logged_out = ?, logout_time = ?
		 WHERE account_id = ? AND profile = ?`,
		nodeID, logoutTime, accountID, profile,
	); err != nil {
		return fmt.Errorf("setLoggedOut: %w", err)
	}
	return nil
}
```

Delete the old PORTING-EXCEPTION (login-server-7) comment block; the closure note above replaces it. Keep the "UPDATE matches by (account_id, profile) only" rationale line.

4. In `modules/login/handler.go` `PlayerLogout`: `setLoggedOut(ctx, h.db, account.ID, req.Profile, int(req.NodeId))` (the world already sends `NodeId` — modules/world/server.go:1495).

5. The M25 reject site (handler.go ~line 232) keeps reading `account.LogoutTime.Valid` — now per-profile via the query change; update its comment to note the per-profile source and login-server-7 closure.

6. `WorldStartup` (handler.go:56): add the exception marker above the `clearWorldSessions` call:

```go
// PORTING-EXCEPTION (rev244-b5-startup-profile, world_startup keeps
// profile): TS 244 dropped `profile` from the world_startup message
// (LoginClient.ts:18-26) but LoginServer.ts:160-171 still filters the
// account_login reset by the now-undefined profile — the upstream
// reset matches nothing at the pin. goscape keeps the field and the
// per-profile reset (correct behavior over broken-line fidelity;
// rev244-b3-ws-origin precedent). See PORTING.md.
```

- [ ] **Step 4: Run the full login suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/... -count=1`
Expected: PASS (including Task 1's migration tests and all pre-existing tests — `TestPlayerLogin_*`, hiscore, save tests).

- [ ] **Step 5: Commit (Tasks 1+2 together)**

```bash
git add modules/login/migrations/000005_rev244_b5.up.sql modules/login/db.go modules/login/handler.go modules/login/db_test.go modules/login/handler_test.go
git commit --no-gpg-sign -m "feat(login): 244 schema — login attempts table, per-profile logged_out/logout_time, message + dormant logger tables [rev-244 B5]" -m "Closes PORTING-EXCEPTION (login-server-7): migration 000005 backfills account_login.logout_time from account.logout_time and drops the legacy column; setLoggedOut stamps logged_out=nodeId + logout_time per (account, profile) (TS LoginServer.ts:484-496); the M25 safety reject now reads the per-profile column. New PORTING-EXCEPTION (rev244-b5-startup-profile). Message-centre + account_session/wealth_event tables land per spec user decision 1." -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

### Task 3: Login proto enum + world wire mapping

**Files:**
- Modify: `proto/login/login.proto` (LoginResult)
- Generate: `pkg/loginpb/*` via `make protos`
- Modify: `modules/world/server.go` (`loginResultToRS2`, ~line 1291)
- Test: `modules/world/server_test.go` (or the file holding existing loginResultToRS2 coverage — locate with `grep -rn "loginResultToRS2" modules/world --include="*_test.go"`; if none exists, add `TestLoginResultToRS2` to server_test.go covering ALL enum values)

- [ ] **Step 1: Write the failing test**

```go
// TestLoginResultToRS2_Rev244RateLimits pins the 244 reply→byte contract
// (World.ts:1871-1911): login-server response 8 (3-in-5s rate limit) →
// client byte 16 (TOO_MANY_ATTEMPTS, World.ts:1901-1906); response 6
// (45s hop timer) → client byte 9 (IP_LIMIT "login limit exceeded",
// World.ts:1891-1896).
func TestLoginResultToRS2_Rev244RateLimits(t *testing.T) {
	if got := loginResultToRS2(loginpb.LoginResult_LOGIN_RESULT_RATE_LIMITED); got != 16 {
		t.Errorf("RATE_LIMITED: got byte %d, want 16", got)
	}
	if got := loginResultToRS2(loginpb.LoginResult_LOGIN_RESULT_HOP_TIMER); got != 9 {
		t.Errorf("HOP_TIMER: got byte %d, want 9", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestLoginResultToRS2_Rev244RateLimits -v`
Expected: COMPILE FAIL — `LOGIN_RESULT_RATE_LIMITED` undefined.

- [ ] **Step 3: Extend the proto + regenerate**

In `proto/login/login.proto` append to `LoginResult`:

```proto
  // rev-244: login-server 3-in-5s same-account+IP rate limit
  // (TS LoginServer.ts:234-253, response 8 → client byte 16).
  LOGIN_RESULT_RATE_LIMITED        = 10;
  // rev-244: 45s world-hop timer (TS LoginServer.ts:366-379,
  // response 6 → client byte 9).
  LOGIN_RESULT_HOP_TIMER           = 11;
```

Run: `make protos`
Expected: regenerated `pkg/loginpb/login.pb.go` (and friendspb untouched). Stage only the login-related generated files.

- [ ] **Step 4: Extend loginResultToRS2**

Add to the switch in `modules/world/server.go` before `default:`:

```go
	case loginpb.LoginResult_LOGIN_RESULT_RATE_LIMITED:
		// TS response 8 → byte 16 "too many attempts" (World.ts:1901-1906).
		return loginresp.OpTooManyAttempts.Opcode
	case loginpb.LoginResult_LOGIN_RESULT_HOP_TIMER:
		// TS response 6 → byte 9 "login limit exceeded" (World.ts:1891-1896).
		return loginresp.OpIPLimit.Opcode
```

- [ ] **Step 5: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestLoginResultToRS2 -v && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...` (with CGO_ENABLED=0 -trimpath on the build)
Expected: PASS / clean build.

- [ ] **Step 6: Commit**

```bash
git add proto/login/login.proto pkg/loginpb modules/world/server.go modules/world/server_test.go
git commit --no-gpg-sign -m "feat(login): RATE_LIMITED + HOP_TIMER results with 244 reply bytes 16/9 [rev-244 B5]" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

### Task 4: 3-in-5s same-account+IP rate limit

**Files:**
- Modify: `modules/login/db.go` (two helpers)
- Modify: `modules/login/handler.go` (block between account resolution and password compare)
- Test: `modules/login/handler_test.go`

- [ ] **Step 1: Write failing tests**

```go
// TestPlayerLogin_RateLimit pins TS LoginServer.ts:234-268: per-attempt
// `login` rows keyed (account, ip); 3 rows inside 5s → response 8
// (LOGIN_RESULT_RATE_LIMITED) BEFORE the password compare; a rejected
// attempt does NOT insert a row.
func TestPlayerLogin_RateLimit(t *testing.T) {
	h, _ := newTestHandler(t)
	req := func(pw string) *loginpb.PlayerLoginRequest {
		return &loginpb.PlayerLoginRequest{
			NodeId: 10, Profile: "main", Username: "bob", Password: pw,
			RemoteAddress: "1.2.3.4:5", Uid: 1,
		}
	}
	// Attempt 1 registers the account (NEW_PLAYER) and logs row 1.
	resp, err := h.PlayerLogin(t.Context(), req("pw"))
	if err != nil || resp.Result != loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER {
		t.Fatalf("attempt 1: %v / %v", resp, err)
	}
	// Logout so attempts 2-3 are not ALREADY_LOGGED_IN. (Graceful logout
	// stamps logout_time but no save exists → use force-logout, which
	// does NOT stamp logout_time, so M25 stays unarmed.)
	if _, err := h.PlayerForceLogout(t.Context(), &loginpb.PlayerForceLogoutRequest{
		NodeId: 10, Profile: "main", Username: "bob",
	}); err != nil {
		t.Fatal(err)
	}
	// Attempts 2-3: wrong password — still insert attempt rows (the TS
	// insert precedes the bcrypt compare).
	for i := 2; i <= 3; i++ {
		resp, err = h.PlayerLogin(t.Context(), req("wrong"))
		if err != nil || resp.Result != loginpb.LoginResult_LOGIN_RESULT_INVALID_CREDENTIALS {
			t.Fatalf("attempt %d: %v / %v", i, resp, err)
		}
	}
	// Attempt 4 inside the window: rate limited, even with the right password.
	resp, err = h.PlayerLogin(t.Context(), req("pw"))
	if err != nil || resp.Result != loginpb.LoginResult_LOGIN_RESULT_RATE_LIMITED {
		t.Fatalf("attempt 4: got %v / %v, want RATE_LIMITED", resp, err)
	}
	// Exactly 3 rows (the rejected attempt did not insert).
	var n int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM login`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("login rows: got %d, want 3", n)
	}
}

// TestPlayerLogin_RateLimit_ScopedToAccountAndIP pins the window key:
// a different IP for the same account is NOT limited (TS keys the
// window by account_id AND ip, LoginServer.ts:238-239).
func TestPlayerLogin_RateLimit_ScopedToAccountAndIP(t *testing.T) {
	h, _ := newTestHandler(t)
	mk := func(addr string) *loginpb.PlayerLoginRequest {
		return &loginpb.PlayerLoginRequest{
			NodeId: 10, Profile: "main", Username: "bob", Password: "wrong",
			RemoteAddress: addr, Uid: 1,
		}
	}
	// Seed the account.
	if _, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId: 10, Profile: "main", Username: "bob", Password: "pw",
		RemoteAddress: "1.2.3.4:5", Uid: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.PlayerForceLogout(t.Context(), &loginpb.PlayerForceLogoutRequest{
		NodeId: 10, Profile: "main", Username: "bob",
	}); err != nil {
		t.Fatal(err)
	}
	// 2 more failed attempts from IP A (3 rows total for IP A).
	for i := 0; i < 2; i++ {
		if _, err := h.PlayerLogin(t.Context(), mk("1.2.3.4:5")); err != nil {
			t.Fatal(err)
		}
	}
	// Attempt from IP B: not limited (INVALID_CREDENTIALS, not RATE_LIMITED).
	resp, err := h.PlayerLogin(t.Context(), mk("9.9.9.9:5"))
	if err != nil || resp.Result != loginpb.LoginResult_LOGIN_RESULT_INVALID_CREDENTIALS {
		t.Errorf("other-IP attempt: got %v / %v, want INVALID_CREDENTIALS", resp, err)
	}
}

// TestPlayerLogin_RateLimit_WindowExpiry pins the 5s window edge: rows
// older than 5s do not count (TS timestamp >= now-5000,
// LoginServer.ts:240). Backdates the rows directly rather than sleeping.
func TestPlayerLogin_RateLimit_WindowExpiry(t *testing.T) {
	h, _ := newTestHandler(t)
	seed := &loginpb.PlayerLoginRequest{
		NodeId: 10, Profile: "main", Username: "bob", Password: "pw",
		RemoteAddress: "1.2.3.4:5", Uid: 1,
	}
	if _, err := h.PlayerLogin(t.Context(), seed); err != nil {
		t.Fatal(err)
	}
	if _, err := h.PlayerForceLogout(t.Context(), &loginpb.PlayerForceLogoutRequest{
		NodeId: 10, Profile: "main", Username: "bob",
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
			NodeId: 10, Profile: "main", Username: "bob", Password: "wrong",
			RemoteAddress: "1.2.3.4:5", Uid: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Backdate all 3 rows past the window.
	if _, err := h.db.Exec(`UPDATE login SET timestamp = '2000-01-01 00:00:00'`); err != nil {
		t.Fatal(err)
	}
	resp, err := h.PlayerLogin(t.Context(), seed)
	if err != nil || (resp.Result != loginpb.LoginResult_LOGIN_RESULT_OK &&
		resp.Result != loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER) {
		t.Errorf("post-window attempt: got %v / %v, want OK/NEW_PLAYER", resp, err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ -run 'TestPlayerLogin_RateLimit' -v`
Expected: FAIL — attempt 4 returns OK (no limiting), 0 `login` rows.

- [ ] **Step 3: Implement**

`modules/login/db.go` — two helpers:

```go
// countRecentLoginAttempts counts `login` rows for (accountID, ip) whose
// timestamp falls inside the trailing window. Mirrors the TS 3-in-5s
// window scan (LoginServer.ts:235-242; TS LIMITs at 3 and compares
// length === 3 — COUNT >= 3 is the same observable).
func countRecentLoginAttempts(ctx context.Context, db *sql.DB, accountID int, ip string, window time.Duration) (int, error) {
	cutoff := time.Now().UTC().Add(-window).Format(dbTimeFormat)
	var n int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM login
		 WHERE account_id = ? AND ip = ? AND timestamp >= ?`,
		accountID, ip, cutoff,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("countRecentLoginAttempts: %w", err)
	}
	return n, nil
}

// insertLoginAttempt records one per-attempt `login` row. Mirrors TS
// LoginServer.ts:255-267 (`uuid: socket` → goscape's per-attempt
// sessionUUID; `timestamp: toDbDate(nodeTime)` → server clock — goscape's
// PlayerLoginRequest carries no world clock; the window comparison uses
// the same clock on both sides, so the observable is unchanged).
func insertLoginAttempt(ctx context.Context, db *sql.DB, attemptUUID string, accountID, world, uid int, ip string) error {
	ts := time.Now().UTC().Format(dbTimeFormat)
	if _, err := db.ExecContext(ctx,
		`INSERT INTO login (uuid, account_id, world, timestamp, uid, ip)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		attemptUUID, accountID, world, ts, uid, ip,
	); err != nil {
		return fmt.Errorf("insertLoginAttempt: %w", err)
	}
	return nil
}
```

`modules/login/handler.go` — insert between step 3 (account resolution, after the auto-register block ends at ~line 118) and step 4 (password check):

```go
	// 3b. Per-attempt rate limit + attempt log. TS LoginServer.ts:234-268:
	// runs only when the account exists (auto-registered accounts
	// included), BEFORE the password compare; 3 rows for (account, ip)
	// inside 5s → response 8; a rate-limited attempt does NOT insert.
	// goscape's per-attempt sessionUUID stands in for TS's socket uuid.
	recent, err := countRecentLoginAttempts(ctx, h.db, account.ID, ip, 5*time.Second)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "countRecentLoginAttempts: %v", err)
	}
	if recent >= 3 {
		return &loginpb.PlayerLoginResponse{
			Result: loginpb.LoginResult_LOGIN_RESULT_RATE_LIMITED,
		}, nil
	}
	if err := insertLoginAttempt(ctx, h.db, sessionUUID, account.ID, int(req.NodeId), int(req.Uid), ip); err != nil {
		return nil, status.Errorf(codes.Internal, "insertLoginAttempt: %v", err)
	}
```

(No `if account != nil` guard needed — goscape's step 3 guarantees non-nil account past that point; TS's `if (account)` covers its no-auto-register fallthrough, which goscape returns early from. Note this in the comment if the reviewer asks.)

- [ ] **Step 4: Run the login suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/... -count=1`
Expected: PASS. If pre-existing tests now trip the limit (≥3 same-account+IP logins inside one test), fix the TEST fixtures (distinct IPs/usernames), not the limiter — and say so in the commit body.

- [ ] **Step 5: Commit**

```bash
git add modules/login/db.go modules/login/handler.go modules/login/handler_test.go
git commit --no-gpg-sign -m "feat(login): 3-in-5s same-account+IP rate limit over the new login attempts table [rev-244 B5]" -m "TS LoginServer.ts:234-268. Closes B3 tracker row 1 (world-side limiting removed in f4e7571e; this is the 244 replacement). Rejected attempts do not insert; window keyed (account_id, ip)." -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

### Task 5: 45s hop timer

**Files:**
- Modify: `modules/login/handler.go` (step-7 chain)
- Test: `modules/login/handler_test.go`

- [ ] **Step 1: Write failing tests**

```go
// hopTimerFixture registers `bob` (password "pw"), force-logs-out, then
// directly seeds account_login.logged_out/logout_time to simulate a
// graceful logout from another node. Returns the handler.
func hopTimerFixture(t *testing.T, loggedOut int, logoutAge time.Duration, staffLvl int) *handler {
	t.Helper()
	h, _ := newTestHandler(t)
	if _, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId: 11, Profile: "main", Username: "bob", Password: "pw",
		RemoteAddress: "1.2.3.4:5", Uid: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.PlayerForceLogout(t.Context(), &loginpb.PlayerForceLogoutRequest{
		NodeId: 11, Profile: "main", Username: "bob",
	}); err != nil {
		t.Fatal(err)
	}
	lt := time.Now().UTC().Add(-logoutAge).Format(dbTimeFormat)
	if _, err := h.db.Exec(`UPDATE account_login SET logged_out = ?, logout_time = ?
	                        WHERE account_id = 1 AND profile = 'main'`, loggedOut, lt); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.Exec(`UPDATE account SET staff_mod_level = ? WHERE id = 1`, staffLvl); err != nil {
		t.Fatal(err)
	}
	// Avoid arming the M25 missing-save reject: clear logout_time is NOT
	// possible (it IS the fixture), so write a valid save file instead.
	writeValidSaveFixture(t, h.cfg.SavePath, "main", "bob")
	return h
}

// TestPlayerLogin_HopTimer pins TS LoginServer.ts:366-379: a non-staff
// account that gracefully logged out of ANOTHER world < 45s ago is
// rejected with response 6 (LOGIN_RESULT_HOP_TIMER).
func TestPlayerLogin_HopTimer(t *testing.T) {
	attempt := func(h *handler) loginpb.LoginResult {
		t.Helper()
		resp, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
			NodeId: 10, Profile: "main", Username: "bob", Password: "pw",
			RemoteAddress: "5.6.7.8:5", Uid: 1, // distinct IP — stays under the Task 4 limit
		})
		if err != nil {
			t.Fatalf("PlayerLogin: %v", err)
		}
		return resp.Result
	}
	// Fires: other node (11 != 10), 10s ago, staff 0.
	if got := attempt(hopTimerFixture(t, 11, 10*time.Second, 0)); got != loginpb.LoginResult_LOGIN_RESULT_HOP_TIMER {
		t.Errorf("hop case: got %v, want HOP_TIMER", got)
	}
	// Bypass: same node (logged_out == nodeId 10).
	if got := attempt(hopTimerFixture(t, 10, 10*time.Second, 0)); got != loginpb.LoginResult_LOGIN_RESULT_OK {
		t.Errorf("same-node case: got %v, want OK", got)
	}
	// Bypass: logged_out == 0 (no recorded origin; backfill posture).
	if got := attempt(hopTimerFixture(t, 0, 10*time.Second, 0)); got != loginpb.LoginResult_LOGIN_RESULT_OK {
		t.Errorf("logged_out=0 case: got %v, want OK", got)
	}
	// Bypass: outside the 45s window.
	if got := attempt(hopTimerFixture(t, 11, 46*time.Second, 0)); got != loginpb.LoginResult_LOGIN_RESULT_OK {
		t.Errorf(">45s case: got %v, want OK", got)
	}
	// Bypass: staffmodlevel >= 2 (supermod tier, B3 T18).
	if got := attempt(hopTimerFixture(t, 11, 10*time.Second, 2)); got != loginpb.LoginResult_LOGIN_RESULT_OK {
		t.Errorf("staff case: got %v, want OK", got)
	}
}
```

`writeValidSaveFixture` — check `modules/login/save_test.go` / `handler_test.go` for the existing helper that produces a verify-passing save blob (the reconnect/M25 tests must already have one; reuse its exact name; only create a new helper if genuinely absent).

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ -run TestPlayerLogin_HopTimer -v`
Expected: FAIL — hop case returns OK.

- [ ] **Step 3: Implement**

In `modules/login/handler.go`, extend the step-7 chain (currently `if account.HasLoginRow && account.LoggedIn == 1 { ... }`) with an else-if arm:

```go
	reconnect := false
	if account.HasLoginRow && account.LoggedIn == 1 {
		if account.NodeID == int(req.NodeId) && req.Reconnecting {
			reconnect = true
		} else {
			return buildLoginResponse(loginpb.LoginResult_LOGIN_RESULT_ALREADY_LOGGED_IN, account, nil, sessionUUID), nil
		}
	} else if account.StaffModLevel < 2 &&
		account.LoggedOut != 0 &&
		account.LoggedOut != int(req.NodeId) &&
		account.LogoutTime.Valid {
		// 45s hop timer — TS LoginServer.ts:366-379: a non-staff account
		// that gracefully logged out of a DIFFERENT world less than 45s
		// ago is rejected with response 6. logged_out/logout_time are the
		// per-profile columns from migration 000005 (login-server-7
		// closure). TS's `logged_out !== null` collapses into != 0 (the
		// column is NOT NULL DEFAULT 0).
		if t, err := time.Parse(dbTimeFormat, account.LogoutTime.String); err == nil &&
			!t.Before(time.Now().UTC().Add(-45*time.Second)) {
			return &loginpb.PlayerLoginResponse{
				Result: loginpb.LoginResult_LOGIN_RESULT_HOP_TIMER,
			}, nil
		}
	}
```

- [ ] **Step 4: Run the login suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/... -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/login/handler.go modules/login/handler_test.go
git commit --no-gpg-sign -m "feat(login): 45s world-hop timer on the per-profile logged_out/logout_time columns [rev-244 B5]" -m "TS LoginServer.ts:366-379, response 6 -> client byte 9. Staff >=2 bypass; logged_out 0/same-node bypass; >45s bypass. Completes B3 tracker row 1's replacement pair with the 3-in-5s limit." -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

### Task 6: getUnreadMessageCount

**Files:**
- Create: `modules/login/messages.go`
- Modify: `modules/login/handler.go` (wire into reconnect + full-login responses)
- Test: `modules/login/messages_test.go`

- [ ] **Step 1: Write failing tests**

Create `modules/login/messages_test.go`:

```go
package login

import (
	"database/sql"
	"testing"
)

// mt inserts a message_thread row. from/to are account ids; lastFrom is
// last_message_from. Returns the thread id.
func mt(t *testing.T, db *sql.DB, from, to, lastFrom int) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO message_thread (to_account_id, from_account_id, last_message_from, subject)
	                     VALUES (?, ?, ?, 's')`, to, from, lastFrom)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

// msg inserts a message row with the given created stamp ('' = CURRENT_TIMESTAMP default).
func msg(t *testing.T, db *sql.DB, thread int64, created string, deleted bool) {
	t.Helper()
	del := sql.NullString{}
	if deleted {
		del = sql.NullString{String: created, Valid: true}
	}
	var err error
	if created == "" {
		_, err = db.Exec(`INSERT INTO message (thread_id, sender_id, sender_ip, content, deleted)
		                  VALUES (?, 1, '', 'm', ?)`, thread, del)
	} else {
		_, err = db.Exec(`INSERT INTO message (thread_id, sender_id, sender_ip, content, created, deleted)
		                  VALUES (?, 1, '', 'm', ?, ?)`, thread, created, del)
	}
	if err != nil {
		t.Fatal(err)
	}
}

// st inserts a message_status row for (thread, account) with optional
// read/deleted stamps ('' = NULL).
func st(t *testing.T, db *sql.DB, thread int64, account int, read, deleted string) {
	t.Helper()
	toNull := func(s string) sql.NullString {
		if s == "" {
			return sql.NullString{}
		}
		return sql.NullString{String: s, Valid: true}
	}
	if _, err := db.Exec(`INSERT INTO message_status (thread_id, account_id, "read", deleted)
	                      VALUES (?, ?, ?, ?)`, thread, account, toNull(read), toNull(deleted)); err != nil {
		t.Fatal(err)
	}
}

// TestGetUnreadMessageCount pins the TS Messages.ts:3-37 unread
// semantics, row by row of the fixture matrix in the spec §Testing.
// Viewer is account id 2; all threads from account 1 → account 2.
func TestGetUnreadMessageCount(t *testing.T) {
	const viewer = 2

	cases := []struct {
		name string
		seed func(t *testing.T, db *sql.DB)
		want int
	}{
		{"unread thread counted", func(t *testing.T, db *sql.DB) {
			th := mt(t, db, 1, viewer, 1)
			msg(t, db, th, "2026-06-05 10:00:00", false)
		}, 1},
		{"read after last message not counted", func(t *testing.T, db *sql.DB) {
			th := mt(t, db, 1, viewer, 1)
			msg(t, db, th, "2026-06-05 10:00:00", false)
			st(t, db, th, viewer, "2026-06-05 11:00:00", "")
		}, 0},
		{"read before last message counted", func(t *testing.T, db *sql.DB) {
			th := mt(t, db, 1, viewer, 1)
			msg(t, db, th, "2026-06-05 10:00:00", false)
			st(t, db, th, viewer, "2026-06-05 09:00:00", "")
		}, 1},
		{"status-deleted after last message not counted", func(t *testing.T, db *sql.DB) {
			th := mt(t, db, 1, viewer, 1)
			msg(t, db, th, "2026-06-05 10:00:00", false)
			st(t, db, th, viewer, "", "2026-06-05 11:00:00")
		}, 0},
		{"status-deleted before last message counted", func(t *testing.T, db *sql.DB) {
			th := mt(t, db, 1, viewer, 1)
			msg(t, db, th, "2026-06-05 10:00:00", false)
			st(t, db, th, viewer, "", "2026-06-05 09:00:00")
		}, 1},
		{"own-last-message thread excluded", func(t *testing.T, db *sql.DB) {
			th := mt(t, db, 1, viewer, viewer) // last_message_from = viewer
			msg(t, db, th, "2026-06-05 10:00:00", false)
		}, 0},
		{"deleted messages excluded from last-message", func(t *testing.T, db *sql.DB) {
			th := mt(t, db, 1, viewer, 1)
			msg(t, db, th, "2026-06-05 10:00:00", false)
			msg(t, db, th, "2026-06-05 12:00:00", true) // deleted newest
			st(t, db, th, viewer, "2026-06-05 11:00:00", "")
			// last non-deleted = 10:00 < read 11:00 → not unread
		}, 0},
		{"thread not involving viewer excluded", func(t *testing.T, db *sql.DB) {
			th := mt(t, db, 1, 3, 1)
			msg(t, db, th, "2026-06-05 10:00:00", false)
		}, 0},
		{"empty tables", func(t *testing.T, db *sql.DB) {}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := createTestDB(t)
			tc.seed(t, db)
			got, err := getUnreadMessageCount(t.Context(), db, viewer)
			if err != nil {
				t.Fatalf("getUnreadMessageCount: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}
```

Also add a handler-level wiring pin to `modules/login/handler_test.go`:

```go
// TestPlayerLogin_MessageCountWired pins that the unread count reaches
// PlayerLoginResponse.message_count on the full-login path (TS
// LoginServer.ts:395 + :433) — previously a stub 0.
func TestPlayerLogin_MessageCountWired(t *testing.T) {
	h, _ := newTestHandler(t)
	// First login auto-registers account 1.
	if _, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId: 10, Profile: "main", Username: "bob", Password: "pw",
		RemoteAddress: "1.2.3.4:5", Uid: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.PlayerForceLogout(t.Context(), &loginpb.PlayerForceLogoutRequest{
		NodeId: 10, Profile: "main", Username: "bob",
	}); err != nil {
		t.Fatal(err)
	}
	// One unread thread to account 1 from account 99.
	if _, err := h.db.Exec(`INSERT INTO message_thread (to_account_id, from_account_id, last_message_from, subject)
	                        VALUES (1, 99, 99, 's')`); err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.Exec(`INSERT INTO message (thread_id, sender_id, sender_ip, content)
	                        VALUES (1, 99, '', 'm')`); err != nil {
		t.Fatal(err)
	}
	resp, err := h.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId: 10, Profile: "main", Username: "bob", Password: "pw",
		RemoteAddress: "5.6.7.8:5", Uid: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.MessageCount != 1 {
		t.Errorf("MessageCount: got %d, want 1", resp.MessageCount)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ -run 'TestGetUnreadMessageCount|TestPlayerLogin_MessageCountWired' -v`
Expected: COMPILE FAIL — `getUnreadMessageCount` undefined.

- [ ] **Step 3: Implement**

Create `modules/login/messages.go`:

```go
package login

import (
	"context"
	"database/sql"
	"fmt"
)

// getUnreadMessageCount is the SQL port of TS Messages.ts:3-37
// (getUnreadMessageCount, Kysely): count threads the account
// participates in whose newest non-deleted message postdates the
// account's read/deleted status stamps, excluding threads where the
// account itself sent the last message. Returns 0 on empty tables —
// the same observable as the pre-B5 stub until message rows exist
// (goscape has no website writer; the tables are schema parity).
func getUnreadMessageCount(ctx context.Context, db *sql.DB, accountID int) (int, error) {
	const query = `
SELECT COUNT(*)
FROM message_thread thd
LEFT JOIN message_status s
       ON s.thread_id = thd.id AND s.account_id = ?
INNER JOIN (
    SELECT thread_id, MAX(created) AS last_message_date
    FROM message
    WHERE deleted IS NULL
    GROUP BY thread_id
) last_message ON last_message.thread_id = thd.id
WHERE (thd.from_account_id = ? OR thd.to_account_id = ?)
  AND (s.deleted IS NULL OR s.deleted < last_message.last_message_date)
  AND (s."read" IS NULL OR s."read" < last_message.last_message_date)
  AND thd.last_message_from != ?`
	var n int
	if err := db.QueryRowContext(ctx, query, accountID, accountID, accountID, accountID).Scan(&n); err != nil {
		return 0, fmt.Errorf("getUnreadMessageCount: %w", err)
	}
	return n, nil
}
```

Wire into `modules/login/handler.go`:

1. Reconnect path (TS LoginServer.ts:322): immediately before `return buildLoginResponse(loginpb.LoginResult_LOGIN_RESULT_RECONNECT_OK, ...)`:

```go
		mc, err := getUnreadMessageCount(ctx, h.db, account.ID)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "getUnreadMessageCount: %v", err)
		}
		resp := buildLoginResponse(loginpb.LoginResult_LOGIN_RESULT_RECONNECT_OK, account, saveBytes, sessionUUID)
		resp.MessageCount = int32(mc)
		return resp, nil
```

2. Full-login path (TS LoginServer.ts:395): immediately before the final `return buildLoginResponse(result, ...)` (after the tx commit):

```go
	mc, err := getUnreadMessageCount(ctx, h.db, account.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "getUnreadMessageCount: %v", err)
	}
	resp := buildLoginResponse(result, account, saveBytes, sessionUUID)
	resp.MessageCount = int32(mc)
	return resp, nil
```

(Reject paths keep messageCount 0 — TS reject responses carry none.)

- [ ] **Step 4: Run the login suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/... -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/login/messages.go modules/login/messages_test.go modules/login/handler.go modules/login/handler_test.go
git commit --no-gpg-sign -m "feat(login): real getUnreadMessageCount over the message-centre tables [rev-244 B5]" -m "SQL port of TS Messages.ts:3-37; wired on both the full-login (LoginServer.ts:395) and reconnect (:322) paths. Closes B3 tracker row 5." -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

### Task 7: Friends proto — profile fields + PublicMessage re-key

**Files:**
- Modify: `proto/friends/friends.proto`
- Generate: `pkg/friendspb/*` via `make protos`

No behavior yet (fields are additive; the rename is wire-compatible) — this is the foundation commit for Tasks 8-11. No test step; the compile gate is the check.

- [ ] **Step 1: Edit the proto**

In `proto/friends/friends.proto`, add `string profile` to each client→server request, mirroring the TS 244 message shapes (every FriendClient sender attaches `profile: this.profile` — FriendServer.ts:546-722; the server destructures `profile` in every opcode block including RELAY_*, FriendServer.ts:96-440):

| Message | New field |
|---|---|
| `PlayerLoginRequest` | `string profile = 5;` |
| `PlayerLogoutRequest` | `string profile = 3;` |
| `ChatSetModeRequest` | `string profile = 4;` |
| `FriendlistAddRequest` / `FriendlistDelRequest` | `string profile = 4;` |
| `IgnorelistAddRequest` / `IgnorelistDelRequest` | `string profile = 4;` |
| `PrivateMessageRequest` | `string profile = 8;` |
| `SubscribeUpdatesRequest` | `string profile = 3;` |
| `RelayMuteRequest` | `string profile = 4;` |
| `RelayKickRequest` | `string profile = 3;` |
| `RelayShutdownRequest` | `string profile = 3;` |
| `RelayBroadcastRequest` | `string profile = 3;` |
| `RelayTrackRequest` | `string profile = 4;` |
| `RelayReloadRequest` | `string profile = 2;` |
| `RelayClearLoginsRequest` | `string profile = 2;` |
| `RelayClearLogoutsRequest` | `string profile = 2;` |
| `RelayQueueScriptRequest` | `string profile = 4;` |
| `SubscribeWorldEventsRequest` | `string profile = 2;` |

(`WorldConnectRequest` already has `profile = 2`.)

And re-key `PublicMessageRequest` (TS PUBLIC_CHAT_LOG re-key, FriendServer.ts:287-305 + FriendClient publicMessage :704-722):

```proto
message PublicMessageRequest {
  int32  world_id = 1;
  // rev-244: re-keyed from session_uuid to the player username
  // (TS FriendServer.ts:287-305 / FriendClient.publicMessage :704-722).
  string username = 2;
  int32  coord    = 3;
  string chat     = 4;
  string profile  = 5;
}
```

Also update the PublicMessage RPC doc comment ("keyed by per-login session UUID" → "keyed by username + profile + world; rev-244 re-key") and the `SubscribeUpdates`/`SubscribeWorldEvents` comments to mention profile-scoped registries.

- [ ] **Step 2: Regenerate + compile**

Run: `make protos && CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath ./...`
Expected: build FAILS at `modules/friends/handler.go:472` (`req.SessionUuid` undefined) and at the world-side PublicMessage constructors. Fix ONLY the minimal compile breaks in this task by renaming the field accessor at the construction/read sites (`SessionUuid:` → `Username:`, `req.SessionUuid` → `req.Username`) WITHOUT behavior changes (the world still passes `p.session` for now; Task 10/11 re-key the values). Re-run the build until clean, then run both suites:

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/... ./modules/world/... -count=1`
Expected: PASS (world ~2.5 min — not hung).

- [ ] **Step 3: Commit**

```bash
git add proto/friends/friends.proto pkg/friendspb modules/friends modules/world
git commit --no-gpg-sign -m "feat(friends): profile on every request + PublicMessage username re-key (proto) [rev-244 B5]" -m "Mirrors the 244 per-message profile carriage (FriendClient senders FriendServer.ts:546-722; server destructure per opcode). Field values wired in the follow-up tasks; this is the additive proto foundation." -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

### Task 8: Friends server multi-profile conversion

**Files:**
- Modify: `modules/friends/repository.go` (add `repositories` container)
- Modify: `modules/friends/subscriptions.go` (key by (profile, username37))
- Modify: `modules/friends/world_subscriptions.go` (key by (profile, worldId))
- Modify: `modules/friends/handler.go` (profile threading; drop mismatch reject)
- Modify: `modules/friends/friends.go`, `modules/friends/server.go` (wiring)
- Modify: `modules/friends/config.go` (NodeProfile field/flag removed — server no longer validates; grep `cmd/goscape/app` for references)
- Test: `modules/friends/handler_test.go`, `modules/friends/repository_test.go`, `modules/friends/subscriptions_test.go`, `modules/friends/world_subscriptions_test.go` (mechanical re-point + new isolation pins)

- [ ] **Step 1: Write the failing isolation tests**

```go
// TestMultiProfile_WorldIsolation pins the 244 multi-profile server
// (FriendServer.ts:61-75 repositories[profile] +
// socketByWorld[profile][world]): the same world id under two profiles
// is two independent registries — registration, presence, and the
// player cap are all profile-scoped, and the WorldConnect
// profile-mismatch reject is GONE (TS deleted it at 244).
func TestMultiProfile_WorldIsolation(t *testing.T) {
	h := newTestHandler(t)
	// Two profiles connect the SAME world id — both accepted (no
	// mismatch reject).
	for _, profile := range []string{"main", "beta"} {
		if _, err := h.WorldConnect(t.Context(), &friendspb.WorldConnectRequest{
			WorldId: 1, Profile: profile,
		}); err != nil {
			t.Fatalf("WorldConnect %s: %v", profile, err)
		}
	}
	// Same username37 logs into world 1 under both profiles — these are
	// independent registrations (distinct repositories).
	for _, profile := range []string{"main", "beta"} {
		resp, err := h.PlayerLogin(t.Context(), &friendspb.PlayerLoginRequest{
			WorldId: 1, Profile: profile, Username37: 0xB0B,
		})
		if err != nil || !resp.Accepted {
			t.Fatalf("PlayerLogin %s: %v / %v", profile, resp, err)
		}
	}
	// Logout under beta only: main's registration survives.
	if _, err := h.PlayerLogout(t.Context(), &friendspb.PlayerLogoutRequest{
		WorldId: 1, Profile: "beta", Username37: 0xB0B,
	}); err != nil {
		t.Fatal(err)
	}
	if got := h.repos.get("main").GetWorld(0xB0B); got != 1 {
		t.Errorf("main presence after beta logout: world %d, want 1", got)
	}
	if got := h.repos.get("beta").GetWorld(0xB0B); got != 0 {
		t.Errorf("beta presence after beta logout: world %d, want 0", got)
	}
}
```

(Existing `TestHandler_WorldConnect_ProfileMismatch` pins the DELETED behavior — per hard-rule #2 verify against TS first: FriendServer.ts at 244 has no mismatch reject → DELETE that test, citing the TS removal in the commit body.)

- [ ] **Step 2: Run to verify failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run TestMultiProfile_WorldIsolation -v`
Expected: COMPILE FAIL (`h.repos` undefined).

- [ ] **Step 3: Implement the container + re-key**

`modules/friends/repository.go` — append:

```go
// repositories is the lazily-populated per-profile Repository registry.
// Mirrors TS 244 FriendServer.repositories[profile]
// (FriendServer.ts:64-67 declaration, :439-447 lazy creation in
// initializeWorld). All Repositories share one *sql.DB; profile scoping
// happens inside each Repository's SQL (r.profile) and in-memory maps.
type repositories struct {
	mu sync.Mutex
	db *sql.DB
	by map[string]*Repository
}

func newRepositories(db *sql.DB) *repositories {
	return &repositories{db: db, by: make(map[string]*Repository)}
}

// get returns the profile's Repository, creating it on first use
// (TS FriendServer.ts:443-445 `if (!this.repositories[profile])`).
func (rs *repositories) get(profile string) *Repository {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	r, ok := rs.by[profile]
	if !ok {
		r = NewRepository(rs.db, profile)
		rs.by[profile] = r
	}
	return r
}
```

`modules/friends/subscriptions.go` — key by (profile, username37):

```go
// subKey scopes the per-player registry by profile (rev-244 multi-
// profile server: the same username37 may be live under two profiles).
type subKey struct {
	profile    string
	username37 uint64
}
```

`subscriber` gains a `profile string` field (set in `newSubscriber(profile, worldId, username37)`); `subscriptions.by` becomes `map[subKey]*subscriber`; `register`/`deregister`/`send` take the key (`send(profile string, username37 uint64, u *friendspb.FriendsUpdate)`). Same transform for `world_subscriptions.go` with `wsubKey{profile string; worldId int32}` and `send(profile string, worldId int32, ev *friendspb.WorldEvent)` — mirrors TS `socketByWorld[profile][world]` (FriendServer.ts:69-75, :436-441).

`modules/friends/handler.go`:

1. Replace `repo *Repository` with `repos *repositories`.
2. `ensureWorld(profile string, worldId int32)` → `h.repos.get(profile).initializeWorldIfAbsent(worldId, h.cfg.WorldPlayerLimit)`.
3. `WorldConnect`: DELETE the mismatch reject (TS removed it — the 225 block at old FriendServer.ts:96-101 is gone at 244); body becomes `h.repos.get(req.Profile).InitializeWorld(req.WorldId, h.cfg.WorldPlayerLimit)`. Replace the L45 comment's profile-validation sentence accordingly.
4. Every handler method: `repo := h.repos.get(req.Profile)` then use `repo` (and pass `req.Profile` into `h.subs.send` / `h.worldSubs.send` / the broadcast helpers). The helpers `broadcastWorldToFollowers`, `sendPlayerWorldUpdate`, `worldIfVisible`, `sendInitialFriendlist`, `sendInitialIgnorelist` each gain a leading `profile string` parameter (mirroring the TS method-signature change in the same file, e.g. `broadcastWorldToFollowers(profile, username37)` FriendServer.ts:484-492).
5. Relay* handlers: `h.worldSubs.send(req.Profile, req.TargetWorldId, ...)`.
6. `SubscribeUpdates` / `SubscribeWorldEvents`: thread `req.Profile` into the subscriber ctor and the initial-snapshot helpers.
7. `PublicMessage`: `h.repos.get(req.Profile)` — the LogPublicMessage re-key itself is Task 9; for now keep passing `req.Username` into the existing signature.

`modules/friends/friends.go` `starting`: `repos := newRepositories(db)`; struct field + `newGRPCServer(cfg, repos, subs, worldSubs, log)`. `server.go`: ctor takes `repos *repositories`, handler literal sets `repos:`.

`modules/friends/config.go`: delete `NodeProfile` + its flag (the server no longer validates a configured profile — TS 244 deleted `private profile = Environment.NODE_PROFILE`, FriendServer.ts:63 at 225). Run `grep -rn "friends.node-profile\|NodeProfile" cmd modules/friends` and fix all references (config docs/tests).

Update existing tests mechanically: `newTestHandler` builds `repos: newRepositories(createTestDB(t))`; tests reading `h.repo.X(...)` become `h.repos.get("main").X(...)` — and every request literal in friends tests gains `Profile: "main"` (requests with empty profile would land in the "" profile's isolated registry and the assertions would miss).

- [ ] **Step 4: Run the friends suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/... -count=1`
Expected: PASS.

- [ ] **Step 5: Build + world suite (interface-cascade check)**

Run: `CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath ./... && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -count=1`
Expected: PASS (the world side only constructs request structs — additive fields don't break it).

- [ ] **Step 6: Commit**

```bash
git add modules/friends cmd
git commit --no-gpg-sign -m "feat(friends): multi-profile server — per-profile repositories + registries, mismatch reject removed [rev-244 B5]" -m "TS 244 FriendServer.ts:61-75 (repositories[profile], socketByWorld[profile][world]) + per-opcode profile destructure; the 225 WORLD_CONNECT profile-mismatch reject is deleted upstream. TestHandler_WorldConnect_ProfileMismatch pinned the deleted contract and is removed (verified against the pin). friends.node-profile config field retired." -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

### Task 9: public_chat re-key (friends DB + repository + handler)

**Files:**
- Create: `modules/friends/migrations/000004_public_chat_rev244.up.sql`
- Modify: `modules/friends/repository.go` (`LogPublicMessage`)
- Modify: `modules/friends/handler.go` (`PublicMessage`)
- Test: `modules/friends/repository_test.go`, `modules/friends/handler_test.go`

- [ ] **Step 1: Write failing tests**

```go
// TestLogPublicMessage_Rev244Shape pins the re-keyed public_chat row:
// (profile, world, username, coord, message). TS FriendServer.ts:287-305
// resolves username -> account_id against the shared account table;
// goscape's friends DB is username37/username-keyed by design (DB-2
// federation) — the username is stored directly (NO-LANDING-SITE row
// for the account_id resolution, see PORTING.md §B5).
func TestLogPublicMessage_Rev244Shape(t *testing.T) {
	r := NewRepository(createTestDB(t), "main")
	if err := r.LogPublicMessage(t.Context(), 10, "bob", 12345, "hello"); err != nil {
		t.Fatalf("LogPublicMessage: %v", err)
	}
	var profile, username, message string
	var world, coord int
	if err := r.db.QueryRow(`SELECT profile, world, username, coord, message FROM public_chat`).
		Scan(&profile, &world, &username, &coord, &message); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if profile != "main" || world != 10 || username != "bob" || coord != 12345 || message != "hello" {
		t.Errorf("row: %s/%d/%s/%d/%s", profile, world, username, coord, message)
	}
}
```

And a handler-level pin (in handler_test.go) that `PublicMessage` routes `req.Profile`/`req.WorldId`/`req.Username` through (insert then SELECT via `h.repos.get("main")`'s db).

- [ ] **Step 2: Run to verify failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run 'TestLogPublicMessage_Rev244Shape' -v`
Expected: FAIL (signature/columns).

- [ ] **Step 3: Implement**

`modules/friends/migrations/000004_public_chat_rev244.up.sql`:

```sql
-- rev-244: public_chat re-key session_uuid -> username (+ world).
-- TS FriendServer.ts:287-305 / prisma model public_chat
-- (account_id + profile + world); goscape stores the username directly —
-- the friends DB has no account table (DB-2 federation), so TS's
-- username->account_id resolution has no landing site (PORTING.md §B5).
-- Pre-244 rows are keyed by session_uuid with no username mapping in
-- this DB; the legacy table is preserved for audit rather than dropped.
ALTER TABLE public_chat RENAME TO public_chat_legacy_225;

CREATE TABLE public_chat (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    profile  TEXT    NOT NULL,
    world    INTEGER NOT NULL DEFAULT 0,
    username TEXT    NOT NULL,
    coord    INTEGER NOT NULL,
    message  TEXT    NOT NULL,
    created  TEXT    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_public_chat_username ON public_chat (profile, username, created);

CREATE INDEX idx_public_chat_recent ON public_chat (profile, created);
```

`repository.go`:

```go
// LogPublicMessage appends one row to public_chat under r.profile.
// rev-244 re-key: rows are keyed (profile, world, username) — TS
// FriendServer.ts:287-305 resolves username to account_id; goscape
// stores the username directly (federated DB, no account table —
// NO-LANDING-SITE row, PORTING.md §B5). Append-only, no dedupe, no
// validation; insert failure surfaces codes.Internal at the handler.
func (r *Repository) LogPublicMessage(ctx context.Context, world int32, username string, coord int32, message string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO public_chat (profile, world, username, coord, message)
		 VALUES (?, ?, ?, ?, ?)`,
		r.profile, world, username, coord, message,
	)
	if err != nil {
		return fmt.Errorf("LogPublicMessage: %w", err)
	}
	return nil
}
```

`handler.go` `PublicMessage`: `h.repos.get(req.Profile).LogPublicMessage(ctx, req.WorldId, req.Username, req.Coord, req.Chat)`; update its doc comment (world id is now persisted, not envelope-only — TS 244 inserts `world: nodeId`).

- [ ] **Step 4: Run friends suite, commit**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/... -count=1`
Expected: PASS.

```bash
git add modules/friends
git commit --no-gpg-sign -m "feat(friends): public_chat re-key session_uuid -> username + world [rev-244 B5]" -m "TS FriendServer.ts:287-305 / prisma public_chat. account_id resolution NO-LANDING-SITE (federated friends DB, no account table); legacy 225 rows preserved as public_chat_legacy_225." -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

### Task 10: World-side profile population + PublicMessage username re-key

**Files:**
- Modify: `modules/world/bridges.go` (FriendsBridge.PublicMessage signature + grpcFriendsBridge impls + noopBridges)
- Modify: `modules/world/handlers_game.go` (~line 425-440 call site)
- Modify: `modules/world/server.go` (friends request literals: login/logout), `modules/world/admin_bridge.go` (Relay* literals), plus every other `friendspb.*Request{` literal — enumerate with `grep -rn "friendspb\..*Request{" modules/world --include="*.go" | grep -v _test`
- Test: `modules/world/bridges_test.go` (recording fakes), plus the E2E fixtures that assert request contents

- [ ] **Step 1: Write/extend failing tests**

Extend the existing recording-fake assertions (find them via `grep -rn "PublicMessage" modules/world --include="*_test.go"`): the public-chat E2E must now record `username == p.username` (not the session UUID), and a new pin asserts every outbound friends request carries `Profile == s.cfg.NodeProfile`. Representative new pin:

```go
// TestPublicChatLog_UsernameKeyed pins the rev-244 re-key: the world
// sends username (not session uuid) and its profile on the public-chat
// audit log (TS World.ts:1620-1628 logPublicChat sends
// player.username; FriendClient.publicMessage attaches profile,
// FriendServer.ts:704-722). The 225-era session-validity gate is gone —
// TS 244 has no gate beyond logMessage != null (World.ts:677-679).
```

(Write the body against the package's existing public-chat test fixture — locate it first; it currently asserts the session-uuid value.)

- [ ] **Step 2: Run to verify failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run <the test names> -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

1. `bridges.go`: `PublicMessage(username string, coord int, message string)` (interface + grpcFriendsBridge + noopBridges + recordingBridges in tests). The grpc impl fills `&friendspb.PublicMessageRequest{WorldId: ..., Profile: b.profile (or s.cfg.NodeProfile via the bridge's existing config access — match how WorldId is sourced there), Username: username, Coord: int32(coord), Chat: message}`.
2. `handlers_game.go` call site: replace the session gate + call with:

```go
	// Audit-log to friends-server with the UNFILTERED decoded text —
	// rev-244 re-key: keyed by username, not session uuid
	// (TS World.ts:1620-1628 logPublicChat). TS's only gate is
	// logMessage != null (World.ts:677-679); the 225-era session gate
	// is removed with the re-key.
	s := p.client.server
	coord := coordgrid.PackCoord(p.level, p.x, p.z)
	s.friendsBridge.PublicMessage(p.username, coord, decoded)
```

3. Every other outbound friends request literal gains `Profile: s.cfg.NodeProfile` (or the local equivalent — `worldID := int32(s.cfg.NodeID)` shows the established sourcing pattern at server.go:1509). Sites (verify with the grep): server.go PlayerLogin/PlayerLogout/WorldConnect (WorldConnect already passes profile), tick.go (ChatSetMode/Friendlist/Ignorelist/PM senders if constructed there — follow the grep), admin_bridge.go Relay* literals, the SubscribeUpdates/SubscribeWorldEvents subscribe sites.

- [ ] **Step 4: Run the world suite (full — B4 lesson)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -count=1 -timeout 20m`
Expected: PASS (~2.5 min).

- [ ] **Step 5: Commit**

```bash
git add modules/world
git commit --no-gpg-sign -m "feat(world): send profile on friends RPCs; public-chat log keyed by username [rev-244 B5]" -m "TS World.ts:1620-1628 logPublicChat + FriendClient profile carriage (FriendServer.ts:546-722). Session-validity gate removed with the re-key (TS 244 gates only on logMessage != null, World.ts:677-679)." -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

### Task 11: Logger report re-key (slog seam)

**Files:**
- Modify: `modules/world/logger_bridge.go` (NotifyPlayerReport fields; ctor gains nodeID/profile)
- Modify: `modules/world/server.go:441` (ctor call)
- Test: `modules/world/logger_bridge_test.go`

- [ ] **Step 1: Extend the failing test**

`logger_bridge_test.go` already pins NotifyPlayerReport's emitted fields — extend it to require `world`, `profile`, `username`, `timestamp_ms` keys and the ABSENCE of the `session` key (TS 244 report shape: LoggerClient.ts:48-67 — `{type, world, profile, username, timestamp, coord, offender, reason}`).

- [ ] **Step 2: Run to verify failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestSlogLoggerBridge -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

```go
// slogLoggerBridge ... (existing doc) ...
type slogLoggerBridge struct {
	log     *slog.Logger
	nodeID  int
	profile string
}

// NewSlogLoggerBridge wraps parent in a child logger keyed
// component=logger_bridge. nodeID/profile are stamped on records whose
// TS message shapes carry world/profile (LoggerClient.ts:48-87).
func NewSlogLoggerBridge(parent *slog.Logger, nodeID int, profile string) LoggerBridge {
	return &slogLoggerBridge{
		log:     parent.With("component", "logger_bridge"),
		nodeID:  nodeID,
		profile: profile,
	}
}

// NotifyPlayerReport emits a 'report' record. Mirrors TS
// World.notifyPlayerReport's loggerThread.postMessage call; rev-244
// re-shape: keyed by username + world + profile + timestamp instead of
// the 225 session uuid (LoggerClient.ts:48-67). Proto message shapes
// stay with B5/private-sibling; this is the dev/debug slog seam only.
func (b *slogLoggerBridge) NotifyPlayerReport(p *Player, offender, reason string) {
	b.log.Info("player_report",
		"type", "report",
		"world", b.nodeID,
		"profile", b.profile,
		"username", p.username,
		"timestamp_ms", time.Now().UnixMilli(),
		"coord", coordgrid.PackCoord(p.level, p.x, p.z),
		"offender", offender,
		"reason", reason,
	)
}
```

`server.go:441`: `s.loggerBridge = NewSlogLoggerBridge(s.log, s.cfg.NodeID, s.cfg.NodeProfile)`. NOTE: the existing test calls `NotifyPlayerReport(nil, ...)` (bridges_test.go:158) — `p.username` on a nil player panics; check how the current impl handles the nil player (it reads p.session today, so the nil-call test must already construct a non-nil player or the bridge guards — match the existing posture exactly; if `recordingBridges` is what takes nil, only the slog impl needs a real player in ITS test).

- [ ] **Step 4: Run + commit**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -count=1 -timeout 20m`
Expected: PASS.

```bash
git add modules/world/logger_bridge.go modules/world/server.go modules/world/logger_bridge_test.go modules/world/bridges_test.go
git commit --no-gpg-sign -m "feat(world): report seam re-keyed to username + world/profile/timestamp [rev-244 B5]" -m "TS LoggerClient.ts:48-67 244 report shape. input_track was adapted in B3 (2f67fed2); proto/events shapes remain private-sibling-owned." -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

### Task 12: PORTING.md §B5 audit trail + decision rows

**Files:**
- Modify: `PORTING.md` (new `### B5 — server/login/db (2026-06-05)` subsection after §B4; tracker-row closures in §B3's list; Recent-audit-history line)

- [ ] **Step 1: Write the B5 subsection**

Follow the B4 subsection's exact structure (scope diff command, user decisions, decision rows, correspondence-audit table, tracker rows, gates). Content requirements:

**Decision rows:**
- `PORTING-EXCEPTION (rev244-b5-startup-profile)` NEW — marker at modules/login/handler.go WorldStartup; TS dropped profile from world_startup (LoginClient.ts:18-26) while LoginServer.ts:160-171 still filters by it (reset matches nothing at the pin); goscape keeps the field + per-profile reset.
- `PORTING-EXCEPTION (login-server-7)` **CLOSED** — migration 000005 + per-profile setLoggedOut + M25 re-point; closure notes remain in-code at db.go.
- `world_heartbeat` **NO-OP, dead-at-pin consumer** — World.ts:1251-1273 savePlayers posts it; LoginThread.ts:183-185 is `case 'world_heartbeat': break;` — never reaches the login server; producer not modeled (B1 DoublyLinkList precedent). Closes B3 tracker row 2.
- **Worker files NOT-PORTED** (formal rows, closing the eval deferral): `src/util/WorkerFactory.ts` (+11), `src/appWorker.ts` (+8), `src/server/worker/WorkerServer.ts` (+50), `src/server/worker/WorkerClientSocket.ts` (+24), and the STANDALONE_BUNDLE branches in LoginThread.ts/FriendThread.ts/LoggerThread.ts — platform-inapplicable browser-bundle mode, architecture-mapped to dskit (worker-eval verdict).
- **Website-only schema models NOT-PORTED** (user decision 1): newspost, tag, account_tag, message_tag, mod_action, input_report/input_report_event_raw, account 2FA/email/oauth/notes/password_updated columns, session re-shape (goscape's session table already diverged), hiscore PK reorder (index-only, no behavior; goscape keeps (profile, type, account_id)).
- **`account_session`/`wealth_event` dormant tables** — schema-only landing sites, no Go writer (user decision 1); the B4 TRADE recipient_items known-residual row is unchanged.
- **`LoginClient.remaining` drop** — already-aligned (goscape never carried it; eval §2).
- **friends `public_chat` account_id resolution NO-LANDING-SITE** — federated friends DB has no account table (DB-2); username stored directly.
- **FriendServerRepository internals** — orderBy `'f.created', 'asc'` → `'f.created asc'` Kysely-API form + addFriend select slimming + inline 100-cap: **NO-OP/N-A** against goscape's username37-keyed repository (verify each hunk before recording the verdict).
- **`TestHandler_WorldConnect_ProfileMismatch` deleted** — pinned the 225 reject TS removed at 244 (hard-rule #2 verification recorded).

**Correspondence audit table** — every file from the scope diff:

| TS surface | goscape commit / decision |
|---|---|
| `LoginServer.ts` (+59/−4) | rate limit (T4 SHA), hop timer (T5 SHA), messageCount wiring (T6 SHA), logged_out stamp (T2 SHA) |
| `Messages.ts` (+37) | T6 SHA |
| `LoginClient.ts` (10/9) | world_startup profile drop → rev244-b5-startup-profile exception; `remaining` drop already-aligned; rest is field-order churn NO-OP |
| `LoginThread.ts` (27/13) | STANDALONE_BUNDLE NOT-PORTED; `world_heartbeat: break` → NO-OP row |
| `login/index.d.ts` (−1) | covered by remaining-drop row |
| `FriendServer.ts` (136/101) | multi-profile (T8 SHA), public_chat re-key (T9 SHA), profile carriage (T7/T10 SHAs) |
| `FriendServerRepository.ts` (13/13) | NO-OP/N-A verdicts row |
| `FriendThread.ts` (28/14) | STANDALONE_BUNDLE NOT-PORTED; public_message username re-key (T7/T10 SHAs) |
| `LoggerClient.ts` (13/5) / `LoggerServer.ts` (48/16) / `LoggerThread.ts` (31/17) / `WealthEventType.ts` (1/1) | report seam re-key (T11 SHA); account_session/wealth_event dormant tables (T1 SHA); remainder private-sibling decision row |
| `WorkerServer.ts` / `WorkerClientSocket.ts` | NOT-PORTED rows |
| `prisma/*/schema.prisma` + migrations | T1 SHA (consumer-backed + dormant) + website-only NOT-PORTED row |

**Tracker rows:** B3 rows 1 (rate limiting), 2 (world_heartbeat), 5 (messageCount) closed; row 4 (logger/friends shapes) closed for the public repo (report seam + public_message re-key shipped; proto/events deltas remain private-sibling). The multiworld prisma schema is observably identical for these models (213/71 vs 214/71 — spot-verify) — record as covered-by-singleworld.

**Marker audit:** run `grep -rn "PORTING-EXCEPTION" modules pkg cmd internal | wc -l` and record the count delta (+1 rev244-b5-startup-profile code site, −1 login-server-7 marker → net depends on cross-refs; report actual numbers).

- [ ] **Step 2: Update the Recent audit history list + commit**

Add the one-line `rev-244 B5 — ...` summary to PORTING.md's Recent-audit-history section (B4 line as the template, with real SHAs).

```bash
git add PORTING.md
git commit --no-gpg-sign -m "docs(porting): rev-244 B5 audit trail — login/friends/logger correspondence, NOT-PORTED rows, tracker closures [rev-244 B5]" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

### Task 13: Bundle gates + final integration review

- [ ] **Step 1: Full gates**

```bash
CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...   # expect ONLY pkg/util/build self-assignments
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1 -timeout 20m
CGO_ENABLED=1 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/login/... ./modules/friends/... ./modules/world/... -count=1 -timeout 20m
```

Capture real exit codes (`; echo EXIT=$?`). Expected: build 0, vet pre-existing-only, tests 0, race 0.

- [ ] **Step 2: Whole-bundle integration review**

Dispatch a reviewer over `git diff <pre-B5-SHA>..HEAD` against the spec's section list: (1) every spec item has a commit, (2) every TS citation resolves at the pin, (3) the B5 PORTING.md table maps the whole scope diff, (4) no double-application of B3-shipped hunks (account_id threading, InputTracking re-shape), (5) migration is idempotent for fresh DBs (createTestDB path), (6) marker audit numbers match, (7) no `git add -A` staged phantom files. Fix-or-justify every finding (final-review "missing X" findings can be false positives — verify first).

- [ ] **Step 3: Update the resume-handoff doc**

Write `docs/superpowers/handoffs/2026-06-05-RESUME-rev244-port-b6.md` (B6 = pack pipeline re-baseline; carry forward: ALL windows close at B6, 244 reference-cache generation is the B6 prerequisite, B6 must not double-apply the B1 clientinterface writer pull-forward NOR re-bump jagFileVersion; B5 brief flags consumed). Commit.

---

## Self-review notes (already applied)

- Spec coverage: rate limit (T4), hop timer (T5), messageCount (T6), enum/wire (T3), schema incl. dormant tables (T1), login-server-7 closure (T1+T2), startup-profile exception (T2), friends multi-profile (T7+T8), public_chat re-key (T9+T10), logger report (T11), worker + website NOT-PORTED rows / world_heartbeat NO-OP (T12), gates+review (T13). No spec item unmapped.
- Type consistency: `setLoggedOut(ctx, db, accountID, profile, nodeID)` used in T2 tests and impl; `repos *repositories` + `h.repos.get(profile)` consistent across T8-T9; `LogPublicMessage(ctx, world int32, username string, coord int32, message string)` consistent across T9-T10.
- Known judgment calls the implementer must verify in-repo rather than trust the plan: exact existing test-helper names (`createTestDB`, save-fixture helper), the world-side request-literal inventory (grep-driven), the nil-player posture in the logger-bridge test.
