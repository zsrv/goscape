# Login DB Migrations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add golang-migrate with embedded SQL so the login server auto-creates its SQLite schema on first run and tracks future schema changes.

**Architecture:** SQL migration files live in `modules/login/migrations/` and are embedded into the binary with `//go:embed`. `openDB` calls `migrateDB` after setting pragmas; golang-migrate applies any unapplied migrations in version order and owns a `schema_migrations` table to track state. Tests are updated to use real migrations instead of an inline schema constant, making them a true regression check.

**Tech Stack:** `github.com/golang-migrate/migrate/v4`, `golang-migrate/migrate/v4/source/iofs`, `golang-migrate/migrate/v4/database/sqlite` (modernc, CGO-free), Go `embed` package.

---

## File Map

| Action | Path | Purpose |
|---|---|---|
| Create | `modules/login/migrations/000001_init.up.sql` | Authoritative schema source |
| Modify | `modules/login/db.go` | Add embed var, `migrateDB`, call from `openDB` |
| Modify | `modules/login/db_test.go` | Remove `testSchema`; simplify `createTestDB` |

---

## Task 1: Verify baseline

**Files:**
- (read-only) `modules/login/`

- [ ] **Step 1: Run the existing login tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/... -v -count=1
```

Expected: all tests PASS. If any fail before changes, stop and investigate — do not proceed with a broken baseline.

---

## Task 2: Create the migration SQL file

**Files:**
- Create: `modules/login/migrations/000001_init.up.sql`

- [ ] **Step 1: Create the migrations directory and SQL file**

Create `modules/login/migrations/000001_init.up.sql` with this exact content — plain `CREATE TABLE`, no `IF NOT EXISTS` (golang-migrate's version tracking makes idempotent guards unnecessary and they mask bugs):

```sql
CREATE TABLE account (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password TEXT NOT NULL,
    registration_ip TEXT NOT NULL DEFAULT '',
    staff_mod_level INTEGER NOT NULL DEFAULT 0,
    members INTEGER NOT NULL DEFAULT 0,
    banned_until TEXT,
    muted_until TEXT,
    logout_time TEXT
);

CREATE TABLE account_login (
    account_id INTEGER NOT NULL REFERENCES account(id),
    profile TEXT NOT NULL,
    node_id INTEGER NOT NULL DEFAULT 0,
    logged_in INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (account_id, profile)
);

CREATE TABLE session (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_uuid TEXT NOT NULL,
    account_id INTEGER NOT NULL REFERENCES account(id),
    profile TEXT NOT NULL,
    world INTEGER NOT NULL DEFAULT 0,
    uid INTEGER NOT NULL DEFAULT 0,
    login_time TEXT NOT NULL,
    remote_address TEXT NOT NULL DEFAULT ''
);

CREATE TABLE ipban (
    ip TEXT NOT NULL PRIMARY KEY,
    added_by TEXT NOT NULL DEFAULT '',
    added_on TEXT NOT NULL DEFAULT ''
);
```

- [ ] **Step 2: Confirm the file exists**

```bash
ls modules/login/migrations/
```

Expected output: `000001_init.up.sql`

---

## Task 3: Add golang-migrate dependency

**Files:**
- Modify: `go.mod`, `go.sum` (via `go get`)

- [ ] **Step 1: Fetch the module**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go get github.com/golang-migrate/migrate/v4@latest
```

Expected: go.mod and go.sum updated, no errors.

- [ ] **Step 2: Tidy**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go mod tidy
```

Expected: exits 0, no errors.

- [ ] **Step 3: Confirm the dependency is present**

```bash
grep "golang-migrate" go.mod
```

Expected: a line containing `github.com/golang-migrate/migrate/v4`.

---

## Task 4: Update db.go

**Files:**
- Modify: `modules/login/db.go` (lines 1–43)

- [ ] **Step 1: Replace the import block and add the embed var**

Replace the existing package declaration + import block + blank line at the top of `modules/login/db.go` with:

```go
package login

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	sqlitedriver "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS
```

- [ ] **Step 2: Replace openDB**

Replace the existing `openDB` function (currently ends after `return db, nil`) with:

```go
func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err = db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite pragma journal_mode: %w", err)
	}
	if _, err = db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite pragma foreign_keys: %w", err)
	}
	if err = migrateDB(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}
```

- [ ] **Step 3: Add migrateDB immediately after openDB**

Insert this function right after `openDB`:

```go
func migrateDB(db *sql.DB) error {
	src, err := iofs.New(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("iofs source: %w", err)
	}
	drv, err := sqlitedriver.WithInstance(db, &sqlitedriver.Config{})
	if err != nil {
		return fmt.Errorf("sqlite driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "sqlite", drv)
	if err != nil {
		return fmt.Errorf("migrate instance: %w", err)
	}
	if err = m.Up(); errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}
```

- [ ] **Step 4: Verify it compiles**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/login/...
```

Expected: exits 0, no output.

---

## Task 5: Update db_test.go

**Files:**
- Modify: `modules/login/db_test.go`

- [ ] **Step 1: Delete the testSchema constant**

Remove the entire `testSchema` constant (the `const testSchema = \`...\`` block including all SQL inside it). It spans from the line `const testSchema = \`` to the closing backtick on the line after `);`.

- [ ] **Step 2: Replace createTestDB**

Replace the existing `createTestDB` function with this simpler version that delegates to `openDB` (which now applies migrations):

```go
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
```

- [ ] **Step 3: Verify it compiles**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/login/...
```

Expected: exits 0.

---

## Task 6: Run all tests and commit

**Files:**
- All modified files from Tasks 2–5.

- [ ] **Step 1: Run the full login test suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/... -v -count=1
```

Expected: all tests PASS. The tests now exercise the real migration path — `createTestDB` calls `openDB`, which calls `migrateDB`, which applies `000001_init.up.sql` to the in-memory DB before any test runs.

If any test fails with a schema error (e.g., `no such table: account`), the migration SQL is not being applied — check that the `//go:embed` directive is present and that the `migrations/` directory path is correct relative to `db.go`.

- [ ] **Step 2: Run with the race detector**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/login/... -count=1
```

Expected: PASS, no race conditions reported.

- [ ] **Step 3: Commit**

```bash
git add modules/login/migrations/000001_init.up.sql \
        modules/login/db.go \
        modules/login/db_test.go \
        go.mod \
        go.sum
git commit --no-gpg-sign -m "feat(login): add golang-migrate with embedded SQL schema"
```
