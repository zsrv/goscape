# Friends-server bridge slice 3 — SQLite persistence implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Swap the slice-1 in-memory friends/ignores maps for a SQLite-backed persistence layer mirroring `modules/login/db.go`. Presence (worlds/players/chatMode/staffLvl) stays in-memory.

**Architecture:** Two new tables (`friendlist`, `ignorelist`) keyed on `(profile, owner_username37, target_username37)`. `Repository` keeps its public method set (signatures gain `context.Context` + `error` on SQL paths), constructor becomes `NewRepository(db *sql.DB, profile string)`. Lifecycle in `Friends.starting()` opens the DB before constructing the repo.

**Tech Stack:** Go 1.24, `modernc.org/sqlite`, `github.com/golang-migrate/migrate/v4` (`source/iofs` + `database/sqlite` drivers), `database/sql`. Already vendored — `modules/login/db.go` uses them.

**Spec:** `docs/superpowers/specs/2026-05-19-friends-server-bridge-slice3-design.md`

**Conventions (CLAUDE.md):**
- All `go` invocations: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`
- All commits: `git commit --no-gpg-sign`
- Use `use-modern-go` skill conventions when writing Go (slog, `t.Context()`, `errors.Is`, `for range N` where applicable, `any` over `interface{}`).

---

### Task 1: Add SQLite schema migration

**Files:**
- Create: `modules/friends/migrations/000001_init.up.sql`

- [ ] **Step 1: Create the migrations directory and migration file**

```sql
CREATE TABLE friendlist (
    profile TEXT NOT NULL,
    owner_username37 INTEGER NOT NULL,
    target_username37 INTEGER NOT NULL,
    created TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (profile, owner_username37, target_username37)
);

CREATE INDEX idx_friendlist_target
    ON friendlist (profile, target_username37);

CREATE TABLE ignorelist (
    profile TEXT NOT NULL,
    owner_username37 INTEGER NOT NULL,
    target_username37 INTEGER NOT NULL,
    created TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (profile, owner_username37, target_username37)
);
```

- [ ] **Step 2: Verify the file exists**

Run: `ls modules/friends/migrations/`
Expected: `000001_init.up.sql`

- [ ] **Step 3: Commit**

```bash
git add modules/friends/migrations/000001_init.up.sql
git commit --no-gpg-sign -m "friends: add SQLite schema migration"
```

---

### Task 2: Add `db.go` openDB + migrateDB

**Files:**
- Create: `modules/friends/db.go`

**Reference:** `modules/login/db.go:1-76` — verbatim shape, only the package name and embed path change.

- [ ] **Step 1: Create `modules/friends/db.go`**

```go
package friends

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	sqlitedriver "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

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

// migrateDB applies all pending up-migrations. m.Close() is intentionally
// omitted: the sqlite driver's Close() closes the *sql.DB passed to
// WithInstance, which would invalidate all subsequent queries.
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

- [ ] **Step 2: Verify build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/friends/...`
Expected: no output (clean build)

- [ ] **Step 3: Commit**

```bash
git add modules/friends/db.go
git commit --no-gpg-sign -m "friends: add openDB + migration runner"
```

---

### Task 3: Add `db_test.go` covering openDB, migration, pragmas

**Files:**
- Create: `modules/friends/db_test.go`

**Reference:** `modules/login/db_test.go:15-44` (createTestDB helper + noopLogger).

- [ ] **Step 1: Write the failing tests**

```go
package friends

import (
	"database/sql"
	"fmt"
	"io"
	"log/slog"
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

// noopLogger returns a *slog.Logger that discards all output.
func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestOpenDB_AppliesMigrations(t *testing.T) {
	db := createTestDB(t)

	wantTables := []string{"friendlist", "ignorelist"}
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
	// or URI), so an obviously malformed URI is the most reliable failure.
	_, err := openDB("file:?_pragma=garbage(bogus)")
	if err == nil {
		t.Fatalf("openDB with malformed pragma URI: got nil error, want failure")
	}
}
```

- [ ] **Step 2: Run tests, confirm they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run TestOpenDB -v`
Expected: 4 PASS. If `TestOpenDB_BadDSN` does not fail the open (some DSN forms are tolerated), substitute a clearly broken DSN — e.g. set the file path to a non-writable directory like `/proc/self/mem/x.db`.

- [ ] **Step 3: Run with -race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/ -run TestOpenDB`
Expected: PASS, no race-detector output.

- [ ] **Step 4: Commit**

```bash
git add modules/friends/db_test.go
git commit --no-gpg-sign -m "friends: add db_test.go covering openDB + migration"
```

---

### Task 4: Add `SQLiteDSN` to friends Config

**Files:**
- Modify: `modules/friends/config.go`

- [ ] **Step 1: Edit `modules/friends/config.go`**

Current `Config` struct (read first to confirm field order):

```go
type Config struct {
	GRPCListenAddress       string        `yaml:"grpc_listen_address"`
	NodeProfile             string        `yaml:"node_profile"`
	GRPCListenPort          int           `yaml:"grpc_listen_port"`
	WorldPlayerLimit        int           `yaml:"world_player_limit"`
	Enable                  bool          `yaml:"enable"`
	GracefulShutdownTimeout time.Duration `yaml:"graceful_shutdown_timeout"`
}
```

Add `SQLiteDSN string` after `NodeProfile` (keeps related text fields adjacent):

```go
type Config struct {
	GRPCListenAddress       string        `yaml:"grpc_listen_address"`
	NodeProfile             string        `yaml:"node_profile"`
	SQLiteDSN               string        `yaml:"sqlite_dsn"`
	GRPCListenPort          int           `yaml:"grpc_listen_port"`
	WorldPlayerLimit        int           `yaml:"world_player_limit"`
	Enable                  bool          `yaml:"enable"`
	GracefulShutdownTimeout time.Duration `yaml:"graceful_shutdown_timeout"`
}
```

Register the flag inside `RegisterFlagsAndApplyDefaults`, just after the `NodeProfile` flag (matches `login.sqlite-dsn` pattern):

```go
f.StringVar(&c.SQLiteDSN, "friends.sqlite-dsn", "data/friends.db", "Friends server SQLite DSN.")
```

- [ ] **Step 2: Verify build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/friends/...`
Expected: no output

- [ ] **Step 3: Commit**

```bash
git add modules/friends/config.go
git commit --no-gpg-sign -m "friends: add SQLiteDSN config + friends.sqlite-dsn flag"
```

---

### Task 5: Change `NewRepository` signature + add `newTestRepo` fixture

This is the load-bearing structural change. Constructor now accepts `(db *sql.DB, profile string)`. `friends` and `ignores` maps drop out of the struct entirely (replaced by SQL in tasks 6–8). Presence maps (`worlds`, `players`) stay.

**Files:**
- Modify: `modules/friends/repository.go`
- Modify: `modules/friends/repository_test.go`

- [ ] **Step 1: Edit `modules/friends/repository.go`**

Replace the `Repository` struct, the `NewRepository` function, and **stub out** the four SQL-bound methods (`AddFriend`, `DeleteFriend`, `GetFriends`, `AddIgnore`, `DeleteIgnore`, `GetIgnores`, `GetFollowers`) and `IsVisibleTo` to return zero values + nil error. Tasks 6–8 fill these in.

Keep `worldState`, `playerState`, `GetWorld`, `InitializeWorld`, `initializeWorldIfAbsent`, `Register`, `Unregister`, `SetChatMode`, `GetChatMode` exactly as they are today. They touch only in-memory presence state.

New struct + constructor:

```go
package friends

import (
	"context"
	"database/sql"
	"sync"
)

// Repository is the friends/ignores/presence store. Presence (worlds,
// players, privateChat, staffLvl) lives in-memory and is guarded by mu.
// Friends and ignores persist to SQLite via db. profile scopes every
// SQL operation, mirroring the TS FriendServerRepository(profile) ctor.
type Repository struct {
	mu      sync.RWMutex
	db      *sql.DB
	profile string
	worlds  map[int32]*worldState
	players map[uint64]*playerState
}

func NewRepository(db *sql.DB, profile string) *Repository {
	return &Repository{
		db:      db,
		profile: profile,
		worlds:  make(map[int32]*worldState),
		players: make(map[uint64]*playerState),
	}
}
```

Stubs (delete the existing in-memory bodies for these eight methods, replace with):

```go
// AddFriend is wired in Task 6.
func (r *Repository) AddFriend(ctx context.Context, owner, target uint64) error {
	return nil
}

// DeleteFriend is wired in Task 6.
func (r *Repository) DeleteFriend(ctx context.Context, owner, target uint64) error {
	return nil
}

// GetFriends is wired in Task 6.
func (r *Repository) GetFriends(ctx context.Context, owner uint64) ([]uint64, error) {
	return nil, nil
}

// AddIgnore is wired in Task 7.
func (r *Repository) AddIgnore(ctx context.Context, owner, target uint64) error {
	return nil
}

// DeleteIgnore is wired in Task 7.
func (r *Repository) DeleteIgnore(ctx context.Context, owner, target uint64) error {
	return nil
}

// GetIgnores is wired in Task 7.
func (r *Repository) GetIgnores(ctx context.Context, owner uint64) ([]uint64, error) {
	return nil, nil
}

// GetFollowers is wired in Task 8.
//
// NAI-S1-D-NO-FOLLOWER-BROADCAST — handlers don't call this in slice 1.
// Retired by slice 4.
func (r *Repository) GetFollowers(ctx context.Context, target uint64) ([]uint64, error) {
	return nil, nil
}

// IsVisibleTo is wired in Task 8.
func (r *Repository) IsVisibleTo(ctx context.Context, viewer, other uint64) (bool, error) {
	return false, nil
}
```

Also delete the package-doc reference to "in-memory friend/ignore". The presence-only docstring is fine; the slice-3 SQL story will land in Task 8's final retirement.

- [ ] **Step 2: Edit `modules/friends/repository_test.go`**

Add the `newTestRepo` helper near the top (after the existing `noopLogger` — actually `noopLogger` will conflict with Task 3's identical helper in `db_test.go`, so **delete** the one in `repository_test.go`):

```go
// At top of repository_test.go, after imports. Delete the existing
// noopLogger func — Task 3 added an identical one in db_test.go.

// newTestRepo returns a Repository backed by a fresh in-memory SQLite
// database. The DB is closed via t.Cleanup. profile defaults to "test".
func newTestRepo(t *testing.T) (*Repository, *sql.DB) {
	t.Helper()
	db := createTestDB(t)
	return NewRepository(db, "test"), db
}
```

Then mechanically replace **every** `r := NewRepository()` in the 15 existing tests with:

```go
r, _ := newTestRepo(t)
```

The discard `_` is the `*sql.DB`. Existing tests don't call SQL methods directly so they don't need it. (Tests for SQL methods come in tasks 6–8.)

Tests that compare results of `AddFriend`/`DeleteFriend`/`GetFriends`/etc. — these will TEMPORARILY fail their behavioral assertions because the methods are stubs. That is the planned red state for tasks 6–8.

To keep the tree compiling and the test suite green at the end of task 5, you may need to **comment out the bodies** of any test that calls a stubbed method. Tag each commented-out block with `// TODO(slice3 task N)` so tasks 6–8 know what to re-enable. Specifically:
- Tests calling `AddFriend`, `DeleteFriend`, `GetFriends` → uncomment in **Task 6** (and thread `t.Context()`)
- Tests calling `AddIgnore`, `DeleteIgnore`, `GetIgnores` → uncomment in **Task 7**
- Tests calling `GetFollowers`, `IsVisibleTo` → uncomment in **Task 8**

Identify the affected tests by grep:

```bash
grep -n "AddFriend\|DeleteFriend\|GetFriends\|AddIgnore\|DeleteIgnore\|GetIgnores\|GetFollowers\|IsVisibleTo" modules/friends/repository_test.go
```

For each affected test, wrap the body in `t.Skip("re-enabled in slice 3 task N")` with N appropriate. This is cleaner than commenting and produces visible skip output.

- [ ] **Step 3: Edit `modules/friends/handler_test.go`**

Three call sites use `NewRepository()`:

```bash
grep -n "NewRepository\|repo:" modules/friends/handler_test.go
```

Replace each `NewRepository()` with `NewRepository(createTestDB(t), "test")`. The handler tests will need ctx threading too — defer to Task 9.

Same skip-treatment for handler tests that depend on stubbed methods: wrap their body in `t.Skip("re-enabled in slice 3 task 9")`.

- [ ] **Step 4: Update `modules/friends/friends.go` to match the new signature**

The `starting` method has `repo := NewRepository()`. The full lifecycle change lands in Task 10, but the constructor signature changing here will fail compilation immediately. Temporary minimal change: pass `nil, ""` to make it compile. Task 10 replaces this with the real DB lifecycle.

```go
// In friends.go starting():
repo := NewRepository(nil, "") // TODO(slice3 task 10): real DB
```

- [ ] **Step 5: Run the full friends package test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/...`
Expected: all tests PASS or SKIP. No FAIL. No build error.

- [ ] **Step 6: Commit**

```bash
git add modules/friends/repository.go modules/friends/repository_test.go modules/friends/handler_test.go modules/friends/friends.go
git commit --no-gpg-sign -m "friends: change NewRepository to (*sql.DB, profile); stub SQL methods"
```

---

### Task 6: Implement SQL-backed `AddFriend`, `DeleteFriend`, `GetFriends`

**Files:**
- Modify: `modules/friends/repository.go` (replace the three stubs from Task 5)
- Modify: `modules/friends/repository_test.go` (unskip + thread ctx)

- [ ] **Step 1: Write/unskip the failing tests in `modules/friends/repository_test.go`**

Locate every test that currently has `t.Skip("re-enabled in slice 3 task 6")`. Remove the `t.Skip` call. Thread `t.Context()` and error returns through the existing assertions. Pattern:

```go
// Before:
r.AddFriend(0xAAAA, 0xBBBB)

// After:
if err := r.AddFriend(t.Context(), 0xAAAA, 0xBBBB); err != nil {
    t.Fatalf("AddFriend: %v", err)
}
```

```go
// Before:
got := r.GetFriends(0xAAAA)

// After:
got, err := r.GetFriends(t.Context(), 0xAAAA)
if err != nil {
    t.Fatalf("GetFriends: %v", err)
}
```

- [ ] **Step 2: Run, confirm they fail with the expected `got nil, want [0xBBBB]` pattern (stub returns nil)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run TestRepository -v`
Expected: tests touching AddFriend/DeleteFriend/GetFriends FAIL with empty-result assertions.

- [ ] **Step 3: Replace the three stubs in `repository.go`**

```go
func (r *Repository) AddFriend(ctx context.Context, owner, target uint64) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO friendlist (profile, owner_username37, target_username37)
		 VALUES (?, ?, ?)`,
		r.profile, int64(owner), int64(target),
	)
	if err != nil {
		return fmt.Errorf("AddFriend: %w", err)
	}
	return nil
}

func (r *Repository) DeleteFriend(ctx context.Context, owner, target uint64) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM friendlist
		 WHERE profile = ? AND owner_username37 = ? AND target_username37 = ?`,
		r.profile, int64(owner), int64(target),
	)
	if err != nil {
		return fmt.Errorf("DeleteFriend: %w", err)
	}
	return nil
}

func (r *Repository) GetFriends(ctx context.Context, owner uint64) ([]uint64, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT target_username37 FROM friendlist
		 WHERE profile = ? AND owner_username37 = ?`,
		r.profile, int64(owner),
	)
	if err != nil {
		return nil, fmt.Errorf("GetFriends: %w", err)
	}
	defer rows.Close()

	var out []uint64
	for rows.Next() {
		var t int64
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("GetFriends scan: %w", err)
		}
		out = append(out, uint64(t))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetFriends rows: %w", err)
	}
	return out, nil
}
```

Add `"fmt"` to the import block if not already present.

- [ ] **Step 4: Run tests, confirm green**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run TestRepository -v`
Expected: all unskipped tests PASS.

- [ ] **Step 5: -race pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/...`
Expected: PASS, no race-detector output.

- [ ] **Step 6: Commit**

```bash
git add modules/friends/repository.go modules/friends/repository_test.go
git commit --no-gpg-sign -m "friends: implement SQL-backed AddFriend/DeleteFriend/GetFriends"
```

---

### Task 7: Implement SQL-backed `AddIgnore`, `DeleteIgnore`, `GetIgnores`

Mirror Task 6's shape against `ignorelist`.

**Files:**
- Modify: `modules/friends/repository.go`
- Modify: `modules/friends/repository_test.go`

- [ ] **Step 1: Unskip the affected tests** (those tagged `slice 3 task 7`)

Same ctx-threading pattern as Task 6.

- [ ] **Step 2: Run, confirm they fail with stub-shaped empties**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run TestRepository -v`

- [ ] **Step 3: Replace the three stubs in `repository.go`**

```go
func (r *Repository) AddIgnore(ctx context.Context, owner, target uint64) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO ignorelist (profile, owner_username37, target_username37)
		 VALUES (?, ?, ?)`,
		r.profile, int64(owner), int64(target),
	)
	if err != nil {
		return fmt.Errorf("AddIgnore: %w", err)
	}
	return nil
}

func (r *Repository) DeleteIgnore(ctx context.Context, owner, target uint64) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM ignorelist
		 WHERE profile = ? AND owner_username37 = ? AND target_username37 = ?`,
		r.profile, int64(owner), int64(target),
	)
	if err != nil {
		return fmt.Errorf("DeleteIgnore: %w", err)
	}
	return nil
}

func (r *Repository) GetIgnores(ctx context.Context, owner uint64) ([]uint64, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT target_username37 FROM ignorelist
		 WHERE profile = ? AND owner_username37 = ?`,
		r.profile, int64(owner),
	)
	if err != nil {
		return nil, fmt.Errorf("GetIgnores: %w", err)
	}
	defer rows.Close()

	var out []uint64
	for rows.Next() {
		var t int64
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("GetIgnores scan: %w", err)
		}
		out = append(out, uint64(t))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetIgnores rows: %w", err)
	}
	return out, nil
}
```

- [ ] **Step 4: Run + -race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/friends/repository.go modules/friends/repository_test.go
git commit --no-gpg-sign -m "friends: implement SQL-backed AddIgnore/DeleteIgnore/GetIgnores"
```

---

### Task 8: Implement `GetFollowers` + hybrid `IsVisibleTo`

**Files:**
- Modify: `modules/friends/repository.go`
- Modify: `modules/friends/repository_test.go`

`GetFollowers` is a SQL scan over `friendlist` keyed by `target_username37`. The `idx_friendlist_target` index from Task 1 makes this O(log n).

`IsVisibleTo` is hybrid:
1. Read `players[other].privateChat` from in-memory (presence is in-memory; takes `r.mu.RLock`).
2. If `FRIENDS` mode, look up SQL `friendlist` to check if viewer is in other's friend set.

Locking discipline: release `r.mu` before the SQL call to avoid holding the in-memory lock across I/O.

- [ ] **Step 1: Unskip affected tests** (those tagged `slice 3 task 8`)

- [ ] **Step 2: Run, confirm fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run TestRepository -v`

- [ ] **Step 3: Replace the two stubs in `repository.go`**

```go
func (r *Repository) GetFollowers(ctx context.Context, target uint64) ([]uint64, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT owner_username37 FROM friendlist
		 WHERE profile = ? AND target_username37 = ?`,
		r.profile, int64(target),
	)
	if err != nil {
		return nil, fmt.Errorf("GetFollowers: %w", err)
	}
	defer rows.Close()

	var out []uint64
	for rows.Next() {
		var o int64
		if err := rows.Scan(&o); err != nil {
			return nil, fmt.Errorf("GetFollowers scan: %w", err)
		}
		out = append(out, uint64(o))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetFollowers rows: %w", err)
	}
	return out, nil
}

// IsVisibleTo applies TS visibility rules:
//
//	other.privateChat 0 (ON)      -> always visible
//	other.privateChat 1 (FRIENDS) -> visible only if viewer is in other's friend set
//	other.privateChat 2 (OFF)     -> never visible
//
// If other is not registered (no presence row), returns (false, nil).
func (r *Repository) IsVisibleTo(ctx context.Context, viewer, other uint64) (bool, error) {
	r.mu.RLock()
	ps, ok := r.players[other]
	if !ok {
		r.mu.RUnlock()
		return false, nil
	}
	mode := ps.privateChat
	r.mu.RUnlock()

	switch mode {
	case 0: // ON
		return true, nil
	case 1: // FRIENDS
		var count int
		err := r.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM friendlist
			 WHERE profile = ? AND owner_username37 = ? AND target_username37 = ?`,
			r.profile, int64(other), int64(viewer),
		).Scan(&count)
		if err != nil {
			return false, fmt.Errorf("IsVisibleTo: %w", err)
		}
		return count > 0, nil
	default: // OFF or unknown
		return false, nil
	}
}
```

- [ ] **Step 4: Run + -race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/friends/repository.go modules/friends/repository_test.go
git commit --no-gpg-sign -m "friends: implement GetFollowers + hybrid IsVisibleTo"
```

---

### Task 9: Add SQL-concern tests

Four new tests covering profile-boundary correctness, SQL idempotency, follower index, and result-set determinism.

**Files:**
- Modify: `modules/friends/repository_test.go`

- [ ] **Step 1: Append the four tests**

```go
func TestRepository_AddFriend_Idempotent_SQL(t *testing.T) {
	r, db := newTestRepo(t)
	const owner = uint64(0xAAAA)
	const target = uint64(0xBBBB)

	for i := 0; i < 3; i++ {
		if err := r.AddFriend(t.Context(), owner, target); err != nil {
			t.Fatalf("AddFriend iter %d: %v", i, err)
		}
	}

	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM friendlist WHERE profile=? AND owner_username37=?`,
		"test", int64(owner),
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("rows after 3 AddFriend calls: got %d, want 1", count)
	}
}

func TestRepository_AddFriend_RespectsProfileBoundary(t *testing.T) {
	db := createTestDB(t)
	rMain := NewRepository(db, "main")
	rAlt := NewRepository(db, "alt")

	const owner = uint64(0xAAAA)
	const target = uint64(0xBBBB)

	if err := rMain.AddFriend(t.Context(), owner, target); err != nil {
		t.Fatalf("rMain AddFriend: %v", err)
	}

	gotAlt, err := rAlt.GetFriends(t.Context(), owner)
	if err != nil {
		t.Fatalf("rAlt GetFriends: %v", err)
	}
	if len(gotAlt) != 0 {
		t.Errorf("rAlt GetFriends: got %v, want empty (profile boundary)", gotAlt)
	}

	gotMain, err := rMain.GetFriends(t.Context(), owner)
	if err != nil {
		t.Fatalf("rMain GetFriends: %v", err)
	}
	if len(gotMain) != 1 || gotMain[0] != target {
		t.Errorf("rMain GetFriends: got %v, want [%#x]", gotMain, target)
	}
}

func TestRepository_GetFollowers_FindsAllOwners(t *testing.T) {
	r, _ := newTestRepo(t)
	const target = uint64(0xBBBB)
	owners := []uint64{0xA1, 0xA2, 0xA3, 0xA4}

	for _, o := range owners {
		if err := r.AddFriend(t.Context(), o, target); err != nil {
			t.Fatalf("AddFriend %#x: %v", o, err)
		}
	}

	got, err := r.GetFollowers(t.Context(), target)
	if err != nil {
		t.Fatalf("GetFollowers: %v", err)
	}
	if len(got) != len(owners) {
		t.Errorf("GetFollowers len: got %d (%v), want %d", len(got), got, len(owners))
	}
	gotSet := make(map[uint64]bool, len(got))
	for _, o := range got {
		gotSet[o] = true
	}
	for _, o := range owners {
		if !gotSet[o] {
			t.Errorf("GetFollowers missing owner %#x", o)
		}
	}
}

func TestRepository_GetFriends_OrderIsStable(t *testing.T) {
	r, _ := newTestRepo(t)
	const owner = uint64(0xAAAA)
	targets := []uint64{0xB1, 0xB2, 0xB3}
	for _, t37 := range targets {
		if err := r.AddFriend(t.Context(), owner, t37); err != nil {
			t.Fatalf("AddFriend %#x: %v", t37, err)
		}
	}

	first, err := r.GetFriends(t.Context(), owner)
	if err != nil {
		t.Fatalf("GetFriends 1: %v", err)
	}
	for i := 0; i < 5; i++ {
		again, err := r.GetFriends(t.Context(), owner)
		if err != nil {
			t.Fatalf("GetFriends %d: %v", i, err)
		}
		if !slices.Equal(first, again) {
			t.Errorf("GetFriends iter %d: got %v, want %v (PK ordering)", i, again, first)
		}
	}
}
```

`slices` is already imported in `repository_test.go` from existing tests. If not, add to imports.

- [ ] **Step 2: Run**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/ -run TestRepository -v`
Expected: 4 new tests PASS.

- [ ] **Step 3: Commit**

```bash
git add modules/friends/repository_test.go
git commit --no-gpg-sign -m "friends: add SQL-concern tests (idempotency, profile boundary, followers)"
```

---

### Task 10: Thread `ctx` through `handler.go` + update `handler_test.go`

`handler.go` calls every Repository method. Each call now requires a `ctx` argument and many return an `error`. The handler functions already receive `ctx context.Context` from the gRPC stack — thread it through.

**Files:**
- Modify: `modules/friends/handler.go`
- Modify: `modules/friends/handler_test.go`

- [ ] **Step 1: Audit the handler call sites**

Run: `grep -n "repo\." modules/friends/handler.go`

For each call:
- `repo.AddFriend(a, b)` → `repo.AddFriend(ctx, a, b)` + log-and-continue (or return) on error.
- `repo.GetFriends(a)` → `friends, err := repo.GetFriends(ctx, a)` + handle err.
- ... same for the rest.

Error policy: if the gRPC handler can return `(*pb.Resp, error)`, return `status.Errorf(codes.Internal, "...")` on SQL error. If the RPC is fire-and-forget (no response payload), log via `h.log.Error("op failed", "err", err)` and continue — matches `grpcFriendsClient` log-and-swallow precedent (`[[friends-server-slice2-close]]`).

- [ ] **Step 2: Update the handler call sites**

Apply the audit's findings to `handler.go`. The exact set of edits depends on what's there — read the file in full before editing.

- [ ] **Step 3: Build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/friends/...`
Expected: clean.

- [ ] **Step 4: Unskip and update `handler_test.go`**

For each test that was `t.Skip("re-enabled in slice 3 task 9")` (a typo for task 10 — the skip in Task 5 said task 9; if so, unskip those too):
- Remove the `t.Skip`.
- Add `ctx := t.Context()` and thread into Repository call sites used by the test setup.
- If the handler under test takes a `ctx context.Context`, pass `t.Context()` instead of `context.Background()` so the test integrates with cancellation correctly.

- [ ] **Step 5: Run the full friends suite -race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/...`
Expected: PASS, no skips remain.

- [ ] **Step 6: Commit**

```bash
git add modules/friends/handler.go modules/friends/handler_test.go
git commit --no-gpg-sign -m "friends: thread ctx + error returns through handler call sites"
```

---

### Task 11: Wire DB lifecycle in `friends.go`

Open DB in `starting()`, close in `stopping()`, construct Repository with the real DB.

**Files:**
- Modify: `modules/friends/friends.go`

- [ ] **Step 1: Edit `friends.go`**

Replace the `Friends` struct and `starting`/`stopping` methods:

```go
type Friends struct {
	services.Service

	cfg Config
	log *slog.Logger

	db   *sql.DB
	repo *Repository
	srv  *grpcServer
	lis  net.Listener
}

func (f *Friends) starting(_ context.Context) error {
	db, err := openDB(f.cfg.SQLiteDSN)
	if err != nil {
		return fmt.Errorf("open friends db: %w", err)
	}
	repo := NewRepository(db, f.cfg.NodeProfile)
	srv := newGRPCServer(f.cfg, repo, f.log)
	lis, err := srv.listen(f.cfg)
	if err != nil {
		db.Close()
		return err
	}
	f.db = db
	f.repo = repo
	f.srv = srv
	f.lis = lis
	return nil
}

func (f *Friends) stopping(_ error) error {
	if f.lis != nil {
		f.lis.Close()
	}
	if f.db != nil {
		f.db.Close()
	}
	return nil
}
```

Add `"database/sql"` to the import block.

- [ ] **Step 2: Build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/friends/...`
Expected: clean.

- [ ] **Step 3: Run the whole project -race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`
Expected: PASS across all packages (~150s).

- [ ] **Step 4: Re-run the end-to-end smoke against a live friends-server**

The smoke test is at `modules/world/friends_smoke_test.go` (added in slice 2). It already exercises every RPC against an in-process friends-server. After this change, the friends-server will write to a real (temp-file) SQLite DB. If the smoke harness uses an in-memory DSN or runs the friends module via `Friends.starting`, it picks up the DSN from `Config`.

Confirm the smoke test still creates a fresh DB per test (likely via `t.TempDir() + "/friends.db"`). If the existing smoke test bypassed lifecycle and constructed `Repository` directly, it now needs a DB. Update accordingly.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -run FriendsSmoke -v`
Expected: PASS (originally 7ms; with SQLite startup overhead expect < 100ms).

- [ ] **Step 5: Commit**

```bash
git add modules/friends/friends.go modules/world/friends_smoke_test.go  # if smoke changed
git commit --no-gpg-sign -m "friends: open SQLite DB in starting(); close in stopping()"
```

---

### Task 12: Retire `NAI-S1-D-INMEMORY-REPO` + final verification

**Files:**
- Modify: `modules/friends/repository.go` (package doc + retired tag reference)

- [ ] **Step 1: Find every reference to the retired tag**

Run: `grep -rn "NAI-S1-D-INMEMORY-REPO" modules/ docs/`

Expected hits: `modules/friends/repository.go` package doc (the slice-1 docstring saying "Retired by slice 3").

- [ ] **Step 2: Update the package doc**

Replace the slice-1 docstring (currently the first 6 lines of `repository.go`) with:

```go
// Package friends hosts the friends-server gRPC module. The Repository
// keeps presence state (worlds, players, privateChat, staffLvl) in
// memory and persists friend / ignore lists to SQLite via *sql.DB. The
// schema lives at modules/friends/migrations/000001_init.up.sql.
package friends
```

Remove the `NAI-S1-D-INMEMORY-REPO` tag-line from the doc.

- [ ] **Step 3: Run smoke-pack to confirm 12 OK / 0 ERR baseline holds**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run ./cmd/goscape-cli smoke-pack --reference-dir <ref>` (or equivalent project invocation — check Makefile if unsure).

Expected: 12 OK / 0 ERR / 0 SKIP. Pass means slice 3 introduced no regression in unrelated stages.

- [ ] **Step 4: Final whole-slice review**

Re-read every commit added by this plan:

```bash
git log --oneline e4f05e41..HEAD
```

Verify:
- Each commit is independently buildable + testable.
- No `TODO(slice3 task N)` markers remain in the tree.
- `grep -rn "NAI-S1-D-INMEMORY-REPO" modules/` returns no hits.
- `grep -rn "NAI-S3-D-" modules/` shows the 3 new tags (`-USERNAME37-NOT-ACCOUNTID`, `-NO-IN-MEMORY-CACHE`, `-NO-LIST-CAPS`) referenced where they apply.
- `go test -race ./...` clean.

- [ ] **Step 5: Commit the retirement**

```bash
git add modules/friends/repository.go
git commit --no-gpg-sign -m "friends: retire NAI-S1-D-INMEMORY-REPO (slice 3 closes)"
```

---

## Self-review

**Spec coverage:** Every section of the spec maps to at least one task:
- §0 forward map → tasks 1-11 cover each row
- §1 persistence model → §3 + §5 collectively
- §2 schema → Task 1
- §3 API → Tasks 5-8 (signature change + per-method SQL impl)
- §4 config → Task 4
- §5 lifecycle → Task 11
- §6.1 existing tests → Task 5 fixture swap; tasks 6-8 unskip
- §6.2 new db_test.go → Task 3
- §6.3 new SQL-concern tests → Task 9
- §6.4 e2e smoke → Task 11 step 4
- §7 deviation tags → Task 12 retires `NAI-S1-D-INMEMORY-REPO`. The 3 new tags get referenced inside docstrings during tasks 5/6/8 as appropriate (each task plants its own tag where the design constraint lives).
- §9 build sequence → tasks 1-12 follow the sequence

**Placeholder scan:** No "TBD", "TODO" except the deliberate `TODO(slice3 task N)` skip markers introduced in Task 5 and removed by tasks 6-10. These are explicit, scoped, and retired. No "handle edge cases" / "implement later" / "appropriate error handling" phrasing.

**Type consistency:**
- `NewRepository(db *sql.DB, profile string)` — same signature in Task 5, Task 9, Task 11, Task 12.
- `AddFriend(ctx, owner, target uint64) error` — same in Tasks 5/6/9.
- `GetFriends(ctx, owner uint64) ([]uint64, error)` — same in Tasks 5/6.
- `GetFollowers(ctx, target uint64) ([]uint64, error)` — same in Tasks 5/8/9.
- `IsVisibleTo(ctx, viewer, other uint64) (bool, error)` — same in Tasks 5/8.
- `newTestRepo(t) (*Repository, *sql.DB)` — same in Tasks 5/9.
- `createTestDB(t) *sql.DB` — same in Tasks 3/5/9.

Open gap: Task 10 "skip task 9" typo cross-reference resolved inside the task ("if so, unskip those too") — flagged. Not a plan bug, just a hand-off note to the executing subagent.

---

## Execution

Use **superpowers:subagent-driven-development**. Twelve tasks, each independent enough for a fresh subagent, with review checkpoints between tasks. Default cadence.
