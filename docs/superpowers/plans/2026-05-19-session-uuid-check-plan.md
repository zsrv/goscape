# `session.session_uuid` CHECK Constraint Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Tighten the login server's `session.session_uuid` column with a shape-level CHECK constraint (UUID-or-empty) via a forward-only `golang-migrate` table-rebuild migration, coercing pre-slice-7 `RemoteAddr().String()` rows to `""` inline.

**Architecture:** Single new migration file `modules/login/migrations/000002_session_uuid_check.up.sql` that rebuilds the table (CREATE new + INSERT SELECT with CASE coerce + DROP old + RENAME) inside the implicit golang-migrate transaction. Zero application-code changes — `insertSession` already writes UUIDs from slice 7. Three new tests pin the CHECK behavior + legacy-data path; one existing test (`TestInsertSession`) gets a one-line compatibility update from `"uuid-abc-123"` to a real UUID-shaped value.

**Tech Stack:** SQLite (`modernc.org/sqlite` driver), `golang-migrate/migrate/v4` with `iofs` source + embedded `migrations/*.sql`, Go `database/sql`, existing `createTestDB`/`insertTestAccount` helpers in `modules/login/db_test.go`.

**Spec:** `docs/superpowers/specs/2026-05-19-session-uuid-check-design.md`

**Verification gates** (run after T6):
- `unset GOROOT; export PATH="$HOME/go/current/bin:$PATH"; GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... -count=1 -timeout 600s` — zero FAIL across all packages.
- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape-cli smoke-pack --content-dir $HOME/Code/github.com/LostCityRS/content` — 12 OK / 0 ERR / 0 SKIP.

**Discipline:** Per global CLAUDE.md, every `go` invocation prefixes `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`; every commit uses `--no-gpg-sign`; pre-commit `git status`, post-commit `git show --stat HEAD`. Test helper files use `_test.go` suffix (these are existing files — no new test helpers added in this plan).

---

## File structure

**Create:**
- `modules/login/migrations/000002_session_uuid_check.up.sql` — the new migration.

**Modify:**
- `modules/login/db_test.go` — add three new tests at the end of the file; update `TestInsertSession` to use a UUID-shaped value.

**No application code changes.** `modules/login/db.go`, `modules/login/handler.go`, `modules/login/login.go` are all unchanged. `insertSession` already writes `uuid.NewString()` values from slice 7.

---

## Task 1: Add `TestSessionUUIDCheckRejectsNonUUID` (TDD red)

**Goal:** Write the failing test first. Before the migration exists, a raw `INSERT INTO session` with `session_uuid = "not-a-uuid"` succeeds (no CHECK exists yet), so the test asserts an error and fails. After T2 adds the migration, this same test passes.

**Files:**
- Modify: `modules/login/db_test.go` (append to end of file)

- [ ] **Step 1.1: Read the end of the file to find the right insertion point**

Run: `unset GOROOT; export PATH="$HOME/go/current/bin:$PATH"; wc -l modules/login/db_test.go`

Note the last line number; the new test goes at the end of the file (after the existing last test).

- [ ] **Step 1.2: Append the failing test**

Add this code at the end of `modules/login/db_test.go`:

```go
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
```

- [ ] **Step 1.3: Add the `strings` import if not already present**

At the top of `modules/login/db_test.go`, the import block needs `"strings"`. Check the existing imports first; if missing, add it. The current import block looks like:

```go
import (
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)
```

Update to:

```go
import (
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)
```

- [ ] **Step 1.4: Run the new test — expect FAIL**

Run:

```bash
unset GOROOT; export PATH="$HOME/go/current/bin:$PATH"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ -run TestSessionUUIDCheckRejectsNonUUID -count=1 -v
```

Expected output (FAIL):

```
=== RUN   TestSessionUUIDCheckRejectsNonUUID
    db_test.go:NNN: INSERT with non-UUID session_uuid succeeded; expected CHECK constraint failure
--- FAIL: TestSessionUUIDCheckRejectsNonUUID (0.0Xs)
FAIL
```

This is the RED phase of TDD — the migration doesn't exist yet, so the raw insert succeeds.

- [ ] **Step 1.5: Commit the failing test**

Run:

```bash
git status   # confirm only modules/login/db_test.go is modified
git add modules/login/db_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
login: failing test for session_uuid CHECK (B3 T1 red)

Pins the schema-level CHECK constraint that migration 000002 will
add: a raw INSERT INTO session with session_uuid = "not-a-uuid"
must fail. Currently FAILS as expected — no CHECK exists yet.

Spec: docs/superpowers/specs/2026-05-19-session-uuid-check-design.md
EOF
)"
git show --stat HEAD | head -10
```

---

## Task 2: Add migration `000002_session_uuid_check.up.sql` (TDD green)

**Goal:** Add the migration file. The failing test from T1 now passes.

**Files:**
- Create: `modules/login/migrations/000002_session_uuid_check.up.sql`

- [ ] **Step 2.1: Create the migration file**

Create `modules/login/migrations/000002_session_uuid_check.up.sql` with this exact content:

```sql
-- Tighten session.session_uuid: enforce UUID-shape-or-empty at the
-- schema level. Pre-slice-7 rows hold RemoteAddr().String() (e.g.
-- "127.0.0.1:42193") in this column; that same value lives in the
-- separate remote_address column, so coercing session_uuid to '' on
-- legacy rows loses no audit data. Going forward, insertSession
-- (slice 7) only writes uuid.NewString() values so the CHECK is
-- defensive against future regressions.

CREATE TABLE session_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_uuid TEXT NOT NULL CHECK (
        session_uuid = ''
        OR session_uuid GLOB '????????-????-????-????-????????????'
    ),
    account_id INTEGER NOT NULL REFERENCES account(id),
    profile TEXT NOT NULL,
    world INTEGER NOT NULL DEFAULT 0,
    uid INTEGER NOT NULL DEFAULT 0,
    login_time TEXT NOT NULL,
    remote_address TEXT NOT NULL DEFAULT ''
);

INSERT INTO session_new (id, session_uuid, account_id, profile, world, uid, login_time, remote_address)
SELECT
    id,
    CASE
        WHEN session_uuid GLOB '????????-????-????-????-????????????' THEN session_uuid
        ELSE ''
    END,
    account_id, profile, world, uid, login_time, remote_address
FROM session;

DROP TABLE session;

ALTER TABLE session_new RENAME TO session;
```

- [ ] **Step 2.2: Re-run the T1 test — expect PASS**

Run:

```bash
unset GOROOT; export PATH="$HOME/go/current/bin:$PATH"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ -run TestSessionUUIDCheckRejectsNonUUID -count=1 -v
```

Expected output (PASS):

```
=== RUN   TestSessionUUIDCheckRejectsNonUUID
--- PASS: TestSessionUUIDCheckRejectsNonUUID (0.0Xs)
PASS
ok      github.com/zsrv/goscape/modules/login   0.0XXs
```

This is the GREEN phase of TDD — the migration applies via golang-migrate's `iofs` source (which uses `//go:embed migrations/*.sql` at `modules/login/db.go:17-18`), and the CHECK rejects the non-UUID insert.

- [ ] **Step 2.3: Commit the migration**

Run:

```bash
git status   # confirm only modules/login/migrations/000002_session_uuid_check.up.sql is new
git add modules/login/migrations/000002_session_uuid_check.up.sql
git commit --no-gpg-sign -m "$(cat <<'EOF'
login: migration 000002 — session_uuid CHECK + legacy coerce (B3 T2 green)

SQLite table-rebuild pattern (CREATE new + INSERT SELECT with CASE +
DROP old + RENAME) inside the implicit golang-migrate transaction.
CHECK pins session.session_uuid to UUID-shape (8-4-4-4-12 with dashes)
or empty string. Pre-slice-7 rows holding RemoteAddr().String() get
coerced to '' in the INSERT SELECT; the same forensic IP:port value
is independently preserved in the remote_address column.

Spec: docs/superpowers/specs/2026-05-19-session-uuid-check-design.md
EOF
)"
git show --stat HEAD | head -10
```

---

## Task 3: Update `TestInsertSession` to use UUID-shaped value

**Goal:** Existing `TestInsertSession` writes `"uuid-abc-123"` which fails the new CHECK. Update it to use a real UUID-shaped value so the existing test stays green.

**Files:**
- Modify: `modules/login/db_test.go` (lines 229-262, function `TestInsertSession`)

- [ ] **Step 3.1: Confirm the breakage exists by running the existing test**

Run:

```bash
unset GOROOT; export PATH="$HOME/go/current/bin:$PATH"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ -run TestInsertSession -count=1 -v
```

Expected output (FAIL): `insertSession: ... CHECK constraint failed: session_uuid = '' OR session_uuid GLOB ...`.

This documents that T2 broke a previously-passing test — fixable in one line.

- [ ] **Step 3.2: Update both call site and assertion**

In `modules/login/db_test.go`, replace the two occurrences of `"uuid-abc-123"` in `TestInsertSession` with a UUID-shaped literal. The relevant lines are 233 and 247-248:

Find:

```go
	err := insertSession(t.Context(), db, "uuid-abc-123", int(id), "main", 2, 42, "192.168.0.1")
```

Replace with:

```go
	err := insertSession(t.Context(), db, "11111111-2222-3333-4444-555555555555", int(id), "main", 2, 42, "192.168.0.1")
```

Find:

```go
	if sessionUUID != "uuid-abc-123" {
		t.Errorf("session_uuid: got %q, want %q", sessionUUID, "uuid-abc-123")
	}
```

Replace with:

```go
	if sessionUUID != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("session_uuid: got %q, want %q", sessionUUID, "11111111-2222-3333-4444-555555555555")
	}
```

- [ ] **Step 3.3: Re-run the existing test — expect PASS**

Run:

```bash
unset GOROOT; export PATH="$HOME/go/current/bin:$PATH"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ -run TestInsertSession -count=1 -v
```

Expected output (PASS):

```
=== RUN   TestInsertSession
--- PASS: TestInsertSession (0.0Xs)
PASS
```

- [ ] **Step 3.4: Commit**

Run:

```bash
git status   # confirm only modules/login/db_test.go is modified
git add modules/login/db_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
login: TestInsertSession uses UUID-shaped value (B3 T3)

Existing test wrote "uuid-abc-123" which fails the new CHECK
constraint added in T2. Replace with a deterministic UUID-shaped
literal.

Spec: docs/superpowers/specs/2026-05-19-session-uuid-check-design.md
EOF
)"
git show --stat HEAD | head -10
```

---

## Task 4: Add `TestSessionUUIDCheckAcceptsEmpty`

**Goal:** Pin that the empty-string carve-out (used by the legacy-row coercion) is permitted. Documents the schema's design intent.

**Files:**
- Modify: `modules/login/db_test.go` (append after `TestSessionUUIDCheckRejectsNonUUID`)

- [ ] **Step 4.1: Append the test**

Add this code at the end of `modules/login/db_test.go`, after `TestSessionUUIDCheckRejectsNonUUID`:

```go
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
```

- [ ] **Step 4.2: Run the new test — expect PASS**

Run:

```bash
unset GOROOT; export PATH="$HOME/go/current/bin:$PATH"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ -run TestSessionUUIDCheckAcceptsEmpty -count=1 -v
```

Expected output (PASS):

```
=== RUN   TestSessionUUIDCheckAcceptsEmpty
--- PASS: TestSessionUUIDCheckAcceptsEmpty (0.0Xs)
PASS
```

- [ ] **Step 4.3: Commit**

Run:

```bash
git status   # confirm only modules/login/db_test.go is modified
git add modules/login/db_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
login: pin empty-string carve-out on session_uuid CHECK (B3 T4)

Empty string is the coercion target for legacy rows in migration
000002. Pin that the CHECK allows it.

Spec: docs/superpowers/specs/2026-05-19-session-uuid-check-design.md
EOF
)"
git show --stat HEAD | head -10
```

---

## Task 5: Add `TestMigration002CoercesLegacyRows`

**Goal:** Exercise the legacy-data path explicitly. Open a DB at schema version 1, write a pre-slice-7-style row directly, advance to version 2, and verify the row is preserved with `session_uuid = ""` and the `id` stays continuous for subsequent inserts.

**Files:**
- Modify: `modules/login/db_test.go` (append after `TestSessionUUIDCheckAcceptsEmpty`)

- [ ] **Step 5.1: Append the test**

Add this code at the end of `modules/login/db_test.go`, after `TestSessionUUIDCheckAcceptsEmpty`. The test opens a fresh DB, manually constructs a `migrate.Migrate` instance, runs only the first migration (`Steps(1)`), inserts a pre-slice-7-style row, then runs `Up()` to apply the rest:

```go
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
```

- [ ] **Step 5.2: Add the migrate-related imports**

The new test uses `migrate`, `iofs`, and `sqlitedriver` packages that the production `db.go` file imports but `db_test.go` doesn't yet. Add to the existing `db_test.go` import block:

```go
import (
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	sqlitedriver "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"golang.org/x/crypto/bcrypt"
)
```

(`time` may now be unused depending on what the rest of the file does — leave it alone if other tests use it; remove only if `go vet` flags it. Same applies to `slog` / `io` from `noopLogger`.)

- [ ] **Step 5.3: Run the new test — expect PASS**

Run:

```bash
unset GOROOT; export PATH="$HOME/go/current/bin:$PATH"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ -run TestMigration002CoercesLegacyRows -count=1 -v
```

Expected output (PASS):

```
=== RUN   TestMigration002CoercesLegacyRows
--- PASS: TestMigration002CoercesLegacyRows (0.0Xs)
PASS
```

If the AUTOINCREMENT continuity assertion fails: the SQLite version in use does not rename `sqlite_sequence` rows on `ALTER TABLE RENAME TO`. The fix is to add an explicit `UPDATE sqlite_sequence SET name = 'session' WHERE name = 'session_new';` step to the migration file (before the final `ALTER TABLE`). Stop and report the failure before patching — it would surface a behavioral surprise worth a separate plan-deviation note.

- [ ] **Step 5.4: Commit**

Run:

```bash
git status   # confirm only modules/login/db_test.go is modified
git add modules/login/db_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
login: pin legacy-row coercion + autoincrement continuity (B3 T5)

Manually drives golang-migrate to version 1 only, writes a pre-slice-7-
style row directly (session_uuid = RemoteAddr().String()), advances to
version 2 via the new migration, and asserts the legacy session_uuid
is coerced to "" while all other audit columns are preserved. Also
pins that a fresh insertSession on the upgraded table yields a larger
id than the legacy row (sqlite_sequence continuity through the
table-rebuild).

Spec: docs/superpowers/specs/2026-05-19-session-uuid-check-design.md
EOF
)"
git show --stat HEAD | head -10
```

---

## Task 6: Full gate + memory close

**Goal:** Confirm the full suite stays green under `-race`, smoke-pack holds, and write the memory close memo retiring B3.

**Files:**
- Create: `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/post_friends_arc_cleanup_b3_close.md`
- Modify: `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md`

- [ ] **Step 6.1: Run full `-race` suite**

Run:

```bash
unset GOROOT; export PATH="$HOME/go/current/bin:$PATH"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... -count=1 -timeout 600s 2>&1 | tail -30
```

Expected: every line either `ok` / `PASS` or `[no test files]`. Zero `FAIL`. If anything fails, stop and investigate — the failure is likely related to the migration (some other test path inserts non-UUID session rows).

- [ ] **Step 6.2: Run smoke-pack**

Run:

```bash
unset GOROOT; export PATH="$HOME/go/current/bin:$PATH"
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape-cli smoke-pack --content-dir $HOME/Code/github.com/LostCityRS/content 2>&1 | tail -20
```

Expected: `Result: 12 OK, 0 ERR, 0 SKIP` in the final summary line. This gate confirms the login module still builds into the full binary.

- [ ] **Step 6.3: Write memory close memo**

Create `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/post_friends_arc_cleanup_b3_close.md` with this content (replace the commit-hash placeholders with actuals from `git log --oneline af55430b..HEAD`):

```markdown
---
name: post-friends-arc-cleanup-b3-close
description: B3 (session_uuid CHECK constraint) shipped post-deferral on top of [[post-friends-arc-cleanup-b-close]]; tightens schema with shape-level CHECK + legacy coercion.
metadata:
  type: project
---

B3 (deferred during the post-friends-arc cleanup batch) shipped 2026-05-19 across N commits `<first-hash>..<last-hash>` on top of [[post-friends-arc-cleanup-b-close]].

**What changed:**
- New `modules/login/migrations/000002_session_uuid_check.up.sql` — SQLite table-rebuild (CREATE new + INSERT SELECT with CASE + DROP old + RENAME) inside the implicit golang-migrate transaction. Pre-slice-7 rows holding `RemoteAddr().String()` in `session_uuid` are coerced to `""` inline; new schema's CHECK pins `session_uuid` to UUID-shape (8-4-4-4-12 with dashes via `GLOB '????????-????-????-????-????????????'`) or empty string.
- Three new tests in `modules/login/db_test.go`: `TestSessionUUIDCheckRejectsNonUUID` (TDD red→green), `TestSessionUUIDCheckAcceptsEmpty` (empty-string carve-out), `TestMigration002CoercesLegacyRows` (legacy-data path + AUTOINCREMENT continuity through the table-rebuild via `migrate.Steps(1)` → write legacy row → `migrate.Up()` → assertions).
- One-line update to existing `TestInsertSession` — `"uuid-abc-123"` → UUID-shaped literal (the old value would fail the new CHECK).
- Zero application code changes. `insertSession` already writes UUIDs from slice 7; the CHECK is purely defensive against future regressions.

**Verification gates:**
- `go test -race ./... -count=1 -timeout 600s` clean.
- `cmd/goscape-cli smoke-pack` → 12 OK / 0 ERR / 0 SKIP.

**Retires:** the B3 deferred-item line in [[post-friends-arc-cleanup-b-close]]. No new deviation tags opened. No tags retired (none were associated with B3).

**Design notes carried forward:**
- Friends DB chat-audit tables (`public_chat.session_uuid`, `private_chat.session_uuid`) were not touched — those tables came into existence at/after slice 7 and hold only real UUIDs by construction.
- The CHECK is shape-level, not strict UUID-v4 hex validation — sufficient to catch the actual drift class (IP:port strings) and avoids tying the schema to a specific UUID variant.
- AUTOINCREMENT continuity through `DROP TABLE session` + `ALTER TABLE session_new RENAME TO session` is verified by `TestMigration002CoercesLegacyRows` (post-migration `insertSession` must yield `id > legacyID`).

**Cluster posture:** friends-server bridge arc + public_chat follow-up + post-arc cleanup batch B1/B2/B3/B4 all closed. Zero active conditional tags in friends/login surface. Next directions per the resume: Candidate A (NAI-182-D5 social-cluster ServerGameProt port — retires `NAI-S4A-D-NO-INGAME-PACKET-EMIT` + `NAI-S4B-D-NO-INGAME-PM-EMIT`) or general world/runescript engine work.
```

- [ ] **Step 6.4: Update MEMORY.md**

In `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md`, insert a new entry ABOVE the existing `post-friends-arc-cleanup-b-close` line (which is currently the second line, just below `git-pre-commit-status-check`). The new entry must be one line, under ~200 chars per the file's `>` warning style. Suggested text:

```
- [post-friends-arc cleanup B3 close](post_friends_arc_cleanup_b3_close.md) — session_uuid CHECK shipped 2026-05-19 across N commits <first>..<last> on top of [[post-friends-arc-cleanup-b-close]]; new migration 000002 rebuilds session table with shape-level CHECK ('' OR `GLOB '????????-????-????-????-????????????'`) + inline coerces pre-slice-7 IP:port rows to ""; 3 new tests (rejection / empty-accept / legacy-coerce-with-AUTOINCREMENT-continuity) + 1 existing test fixup (TestInsertSession uses UUID literal); zero app-code change (insertSession already writes UUIDs); friends DB unaffected; -race clean; smoke-pack 12 OK; retires B3 deferred-item line
```

Replace `N`, `<first>`, `<last>` with actuals from `git log --oneline af55430b..HEAD | wc -l` and the first/last short hashes.

- [ ] **Step 6.5: Verify the memory entry length**

Run:

```bash
awk 'NR==2 {print length($0)}' $HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md
```

If under ~1500 chars (the existing memory lines are 1000+ chars; the warning is about total file line count, not per-entry length), it's fine. If you want it tighter for index readability, trim — but matching neighbor entries' detail level is acceptable.

- [ ] **Step 6.6: Final pre-commit status check**

Run:

```bash
git status
```

Expected: only `.claude/projects/.../memory/MEMORY.md` and `.claude/projects/.../memory/post_friends_arc_cleanup_b3_close.md` modified/new. **Memory files are NOT in the goscape repo** — they live in `~/.claude/...` (a separate location). So `git status` inside the goscape repo should show "nothing to commit, working tree clean" (other than the standing untracked-file noise from the user's local `.bash_profile` etc.). The memory updates themselves do not get committed to the goscape repo.

No git commit is needed for T6 — memory file updates are out-of-tree.

---

## Self-review pass

(Performed inline by the plan author after the task list was complete. Findings + fixes folded back into the plan above:)

- **Spec coverage:** ✅ All four spec sections have tasks. Architecture → T2. CHECK shape → T2 (literal SQL). Migration file content → T2. Data flow → T2 + T5. Application code "zero changes" → T3 (the only modification is a test value update, not application code). Three new tests → T1+T4+T5. Rollout → covered by the implicit golang-migrate flow used in tests. Verification gates → T6. Memory close → T6.
- **Placeholder scan:** ✅ No TBDs, no "implement later", no "add appropriate error handling". The two `<first>`/`<last>` hash placeholders in T6.3/T6.4 are unavoidable — they're values the engineer derives from `git log` at the moment of close.
- **Type consistency:** ✅ The UUID-shaped literals across T3 (`"11111111-2222-3333-4444-555555555555"`) and T5 (`"22222222-3333-4444-5555-666666666666"`) match the spec's GLOB shape (36 chars, dashes at positions 9/14/19/24). The CHECK constraint text in T2 matches verbatim what T1's error-message assertion expects (`"CHECK"` substring). Migration filename `000002_session_uuid_check.up.sql` is consistent across T2, T5, T6.3, T6.4.
- **One added clarification:** T5 has a fallback paragraph if AUTOINCREMENT continuity fails — instructs the engineer to stop and report rather than silently patching the migration. This guards against the spec's noted SQLite-version-specific risk.
