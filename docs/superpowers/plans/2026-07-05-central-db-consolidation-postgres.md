# Central Database Consolidation + PostgreSQL Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Merge the login and friends SQLite databases into one central database (TS-274-shaped friends tables, FK+CASCADE), with services as independent DB clients, then add PostgreSQL as a selectable backend.

**Architecture:** New `pkg/gamedb` client library owns backend selection, pool posture, placeholder rebinding, and the unified migration lineage. A new invisible `database` module migrates schema before `login`/`friends` start; each of those modules opens its **own** `gamedb` pool (no shared handles). Friends persistence re-keys from username37 columns to account-id JOINs mirroring TS 274 `FriendServerRepository.ts`, restoring the DB-2-blocked behaviors.

**Tech Stack:** Go 1.26, modernc.org/sqlite (existing), golang-migrate v4 (existing), jackc/pgx/v5 (new, Phase 2), dskit modules/services (existing).

**Spec:** `docs/superpowers/specs/2026-07-05-central-db-consolidation-postgres-design.md`

## Global Constraints

- Branch: `rev-274` only. Do NOT port to other rev branches.
- Go version: 1.26. Use modern idioms (`t.Context()`, `any`, `for i := range n`, `errors.Is`).
- All go commands: prefix `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`.
- All git commits: `git commit --no-gpg-sign`. Run `git status --short` before every commit; stage only files you changed.
- TS reference is ONLY `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/` and its `prisma/` (branch `274-GOSCAPE` @ `dee467c8`). No other LostCityRS paths.
- Clean break: no data-migration tooling; old `login.db`/`friends.db` files are never read or deleted.
- Fidelity: every TS-behavior claim in code comments cites the TS file:line. FK constraints are a documented goscape extension; `ignorelist.value` and `public_chat.session_uuid` deliberately get NO FK.
- Every new query string is wrapped in `db.Rebind(...)` at its call site (identity on SQLite; `$N` on Postgres) — the Phase 2 seam, built from day one.
- Tests must not require a network or a running Postgres except behind the `GOSCAPE_TEST_POSTGRES_DSN` env gate (Phase 2).

---

# Phase 1 — Consolidation (SQLite)

### Task 1: `pkg/gamedb` core — Config, Open (sqlite), Rebind

**Files:**
- Create: `pkg/gamedb/config.go`
- Create: `pkg/gamedb/gamedb.go`
- Create: `pkg/gamedb/config_test.go`
- Create: `pkg/gamedb/gamedb_test.go`

**Interfaces:**
- Consumes: nothing (leaf package).
- Produces (later tasks rely on these exact names):
  - `gamedb.Config{Backend string; SQLite SQLiteConfig; Postgres PostgresConfig}` with `RegisterFlagsAndApplyDefaults(*flag.FlagSet)` and `Validate() error`
  - `gamedb.SQLiteConfig{DSN string}`, `gamedb.PostgresConfig{DSN string; MaxOpenConns int}`
  - `gamedb.BackendSQLite = "sqlite"`, `gamedb.BackendPostgres = "postgres"`
  - `gamedb.Open(cfg Config, logger *slog.Logger) (*DB, error)`
  - `type DB struct { *sql.DB; ... }` with `(d *DB) Rebind(query string) string`

- [ ] **Step 1: Write failing config tests**

`pkg/gamedb/config_test.go`:

```go
package gamedb

import (
	"flag"
	"strings"
	"testing"
)

func defaultConfig() Config {
	var c Config
	fs := flag.NewFlagSet("", flag.PanicOnError)
	c.RegisterFlagsAndApplyDefaults(fs)
	return c
}

func TestConfig_Defaults(t *testing.T) {
	c := defaultConfig()
	if c.Backend != BackendSQLite {
		t.Errorf("Backend: got %q, want %q", c.Backend, BackendSQLite)
	}
	if c.SQLite.DSN != "data/goscape.db" {
		t.Errorf("SQLite.DSN: got %q, want data/goscape.db", c.SQLite.DSN)
	}
	if c.Postgres.MaxOpenConns != 8 {
		t.Errorf("Postgres.MaxOpenConns: got %d, want 8", c.Postgres.MaxOpenConns)
	}
	if err := c.Validate(); err != nil {
		t.Errorf("Validate on defaults: %v", err)
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string // "" = valid
	}{
		{"defaults valid", func(c *Config) {}, ""},
		{"unknown backend", func(c *Config) { c.Backend = "mysql" }, "database: backend"},
		{"empty sqlite dsn", func(c *Config) { c.SQLite.DSN = "" }, "database: sqlite.dsn"},
		// Phase 1: postgres is declared but not yet implemented.
		{"postgres not yet supported", func(c *Config) { c.Backend = BackendPostgres }, "not yet supported"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := defaultConfig()
			tt.mutate(&c)
			err := c.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate: unexpected error %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate: got %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamedb/`
Expected: FAIL (package does not compile — `Config` undefined).

- [ ] **Step 3: Implement `pkg/gamedb/config.go`**

```go
// Package gamedb is the central-database client library. Every service
// that needs persistent state (login, friends, future consumers) opens
// its OWN pool through this package — services are independent clients
// of one central database, mirroring the historical model of a
// standalone account database that login servers, the website, and the
// friend server each connected to directly. There is no handle sharing,
// even when modules are co-resident in one process.
//
// The package owns all dialect knowledge: backend selection
// (sqlite | postgres), per-dialect pool posture, placeholder rebinding
// (Rebind), and the unified schema migration lineage (migrations/).
//
// Spec: docs/superpowers/specs/2026-07-05-central-db-consolidation-postgres-design.md
package gamedb

import (
	"flag"
	"fmt"
)

const (
	BackendSQLite   = "sqlite"
	BackendPostgres = "postgres"
)

// Config selects and configures the central-database backend. It is a
// top-level config section (database:) shared by every DB-using module,
// analogous to TS Environment.db.backend (src/db/query.ts:12-28
// @dee467c8, sqlite | mysql there; goscape chooses postgres as its
// second backend instead of mysql — explicit user decision).
type Config struct {
	Backend  string         `yaml:"backend"`
	SQLite   SQLiteConfig   `yaml:"sqlite"`
	Postgres PostgresConfig `yaml:"postgres"`
}

type SQLiteConfig struct {
	DSN string `yaml:"dsn"`
}

type PostgresConfig struct {
	DSN          string `yaml:"dsn"`
	MaxOpenConns int    `yaml:"max_open_conns"`
}

func (c *Config) RegisterFlagsAndApplyDefaults(f *flag.FlagSet) {
	f.StringVar(&c.Backend, "database.backend", BackendSQLite, "Central database backend. Valid values: [sqlite, postgres].")
	f.StringVar(&c.SQLite.DSN, "database.sqlite-dsn", "data/goscape.db", "Central database SQLite DSN (file path).")
	f.StringVar(&c.Postgres.DSN, "database.postgres-dsn", "", "Central database PostgreSQL DSN, e.g. postgres://user:pass@host:5432/goscape?sslmode=disable. Required when database.backend=postgres.")
	f.IntVar(&c.Postgres.MaxOpenConns, "database.postgres-max-open-conns", 8, "Max open connections per service pool (postgres backend only; sqlite is always 1).")
}

// Validate enforces backend invariants. Errors self-prefix "database: "
// (matching the login/friends module convention consumed by
// cmd/goscape/app Config.Validate).
func (c *Config) Validate() error {
	switch c.Backend {
	case BackendSQLite:
		if c.SQLite.DSN == "" {
			return fmt.Errorf("database: sqlite.dsn must be non-empty when database.backend=sqlite")
		}
	case BackendPostgres:
		// Phase 2 lifts this and validates Postgres.DSN instead.
		return fmt.Errorf("database: backend %q is not yet supported (Phase 2)", c.Backend)
	default:
		return fmt.Errorf("database: backend must be one of [sqlite, postgres], got %q", c.Backend)
	}
	return nil
}
```

- [ ] **Step 4: Write failing Open/Rebind tests**

`pkg/gamedb/gamedb_test.go`:

```go
package gamedb

import (
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"path/filepath"
	"testing"
)

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// openTestDB opens an isolated in-memory sqlite DB (no migrations).
func openTestDB(t *testing.T) *DB {
	t.Helper()
	c := defaultConfig()
	c.SQLite.DSN = fmt.Sprintf("file:%s?mode=memory&cache=shared", url.PathEscape(t.Name()))
	db, err := Open(c, noopLogger())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpen_SQLitePragmasApplied(t *testing.T) {
	c := defaultConfig()
	c.SQLite.DSN = filepath.Join(t.TempDir(), "sub", "goscape.db") // sub: exercises parent-dir creation
	db, err := Open(c, noopLogger())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	var busy int
	if err := db.QueryRow(`PRAGMA busy_timeout`).Scan(&busy); err != nil {
		t.Fatalf("busy_timeout: %v", err)
	}
	if busy != 5000 {
		t.Errorf("busy_timeout: got %d, want 5000", busy)
	}
	var fk int
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk); err != nil {
		t.Fatalf("foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys: got %d, want 1", fk)
	}
	var mode string
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode: got %q, want wal", mode)
	}
}

func TestRebind_SQLiteIdentity(t *testing.T) {
	db := openTestDB(t)
	q := `SELECT id FROM account WHERE username = ? AND members = ?`
	if got := db.Rebind(q); got != q {
		t.Errorf("Rebind(sqlite): got %q, want identity", got)
	}
}

func TestRebind_PostgresNumbersPlaceholders(t *testing.T) {
	// Dialect is package-internal; construct directly (Open(postgres)
	// lands in Phase 2).
	d := &DB{dialect: dialectPostgres}
	got := d.Rebind(`INSERT INTO t (a, b, c) VALUES (?, ?, ?)`)
	want := `INSERT INTO t (a, b, c) VALUES ($1, $2, $3)`
	if got != want {
		t.Errorf("Rebind(postgres):\n got %q\nwant %q", got, want)
	}
	// No '?' anywhere: identity even on postgres.
	if q := d.Rebind(`DELETE FROM t`); q != `DELETE FROM t` {
		t.Errorf("Rebind(no placeholders): got %q", q)
	}
}
```

- [ ] **Step 5: Implement `pkg/gamedb/gamedb.go`**

The sqlite path reproduces the exact posture of the (to-be-deleted)
`modules/login/db.go` / `modules/friends/db.go` `openDB`/`dsnWithPragmas`/`ensureDBParentDir`:

```go
package gamedb

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

type dialect int

const (
	dialectSQLite dialect = iota
	dialectPostgres
)

// DB is one service's private connection pool to the central database.
// It embeds *sql.DB so call sites keep the standard query API, and adds
// the dialect-aware Rebind. Services never share a DB value — each
// module Opens its own (independent-clients model, spec §Design 1).
type DB struct {
	*sql.DB
	dialect dialect
}

// Open opens a pool for the configured backend. It does NOT migrate —
// schema lifecycle belongs to the database module (NewMigratorService)
// and to tests via (*DB).Migrate.
func Open(cfg Config, logger *slog.Logger) (*DB, error) {
	switch cfg.Backend {
	case BackendSQLite:
		return openSQLite(cfg.SQLite, logger)
	case BackendPostgres:
		return nil, fmt.Errorf("gamedb: backend %q is not yet supported (Phase 2)", cfg.Backend)
	default:
		return nil, fmt.Errorf("gamedb: unknown backend %q", cfg.Backend)
	}
}

func openSQLite(cfg SQLiteConfig, logger *slog.Logger) (*DB, error) {
	if err := ensureDBParentDir(cfg.DSN); err != nil {
		return nil, fmt.Errorf("gamedb: ensure db parent dir: %w", err)
	}
	db, err := sql.Open("sqlite", sqliteDSNWithPragmas(cfg.DSN))
	if err != nil {
		return nil, fmt.Errorf("gamedb: open sqlite: %w", err)
	}
	// One writer at a time is SQLite's own model; serializing the pool
	// removes SQLITE_BUSY between our own transactions and matches the
	// TS engine's single-connection posture (node:sqlite DatabaseSync,
	// src/db/query.ts:13-15).
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		db.Close()
		return nil, fmt.Errorf("gamedb: sqlite pragma journal_mode: %w", err)
	}
	logger.Debug("opened central database", "backend", BackendSQLite, "dsn", cfg.DSN)
	return &DB{DB: db, dialect: dialectSQLite}, nil
}

// sqliteDSNWithPragmas appends the per-connection pragmas every pooled
// connection must carry. busy_timeout and foreign_keys are
// per-connection settings (unlike journal_mode, which is persistent in
// the file), so they must ride the DSN — a db.Exec PRAGMA would only
// reach whichever single connection the pool hands out.
func sqliteDSNWithPragmas(dsn string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
}

// ensureDBParentDir creates the parent directory of the sqlite DSN's
// file path (no-op if it already exists or the DSN is in-memory).
// SQLite itself doesn't create missing parent directories — it returns
// SQLITE_CANTOPEN (error 14) on first query.
func ensureDBParentDir(dsn string) error {
	if dsn == ":memory:" || strings.HasPrefix(dsn, "file::memory:") {
		return nil
	}
	p := strings.TrimPrefix(dsn, "file:")
	if i := strings.IndexByte(p, '?'); i >= 0 {
		p = p[:i]
	}
	if strings.Contains(p, "mode=memory") {
		return nil
	}
	dir := filepath.Dir(p)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

// IsForeignKeyViolation reports whether err is an FK-constraint
// violation. Consumers (friends AddFriend/AddIgnore/LogPrivateMessage)
// map it to the TS "account missing" drop path: an account deleted
// between existence check and insert must not surface as an internal
// error (spec §Error handling). Phase 2 extends this for pgx error
// codes (23503).
func IsForeignKeyViolation(err error) bool {
	if err == nil {
		return false
	}
	// modernc.org/sqlite: SQLITE_CONSTRAINT_FOREIGNKEY surfaces as text.
	return strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}

// Rebind converts `?` placeholders to the dialect's form: identity on
// SQLite, $1..$N on Postgres. Call once per statement at the call site.
// Constraint (documented + tested): query strings must not contain '?'
// inside string literals — none of ours do.
func (d *DB) Rebind(query string) string {
	if d.dialect != dialectPostgres {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 8)
	n := 0
	for i := range len(query) {
		if query[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(query[i])
	}
	return b.String()
}
```

Note: `openTestDB` uses `mode=memory` in a `file:` DSN — that's why `ensureDBParentDir` also skips `mode=memory` DSNs (small extension over the old modules' helper, which only handled `file::memory:`).

- [ ] **Step 6: Run tests to verify pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamedb/ -v`
Expected: PASS (all of TestConfig_Defaults, TestConfig_Validate, TestOpen_SQLitePragmasApplied, TestRebind_*).

- [ ] **Step 7: Commit**

```bash
git add pkg/gamedb/
git commit --no-gpg-sign -m "feat(gamedb): central-database client core (config, sqlite open, rebind)"
```

---

### Task 2: Unified migration lineage + `Migrate` + FK/cascade tests

**Files:**
- Create: `pkg/gamedb/migrations/sqlite/000001_init.up.sql`
- Create: `pkg/gamedb/migrate.go`
- Create: `pkg/gamedb/migrate_test.go`

**Interfaces:**
- Consumes: `gamedb.Open`, `*gamedb.DB` (Task 1).
- Produces: `(d *DB) Migrate(ctx context.Context) error`. Schema tables/columns exactly as in the DDL below — Tasks 4–6 write queries against these names.

- [ ] **Step 1: Write the full sqlite DDL**

`pkg/gamedb/migrations/sqlite/000001_init.up.sql`. The login-side tables reproduce the **cumulative** result of the retired `modules/login/migrations` chain (000001–000005) with one clean-break change: `session.session_uuid` drops the legacy `''` allowance (a fresh lineage has no pre-slice-7 rows). The friends-side tables are re-keyed to TS 274 shape (`prisma/singleworld/schema.prisma` @dee467c8: friendlist:71-79, ignorelist:81-89, private_chat:141-151, public_chat:131-139) plus goscape FK+CASCADE extensions.

```sql
-- Unified central-database schema (fresh lineage; clean break from the
-- retired modules/login/migrations + modules/friends/migrations chains).
-- Spec: docs/superpowers/specs/2026-07-05-central-db-consolidation-postgres-design.md
--
-- FK posture: account-referencing columns that goscape itself
-- reads/writes carry REFERENCES account(id) ON DELETE CASCADE — a
-- goscape extension over TS (the 274 prisma schemas declare zero
-- @relation fields). Deliberate exceptions:
--   * ignorelist.value — raw username string, NO FK: TS addIgnore
--     (FriendServerRepository.ts:247-296) never checks the target, so
--     you can ignore usernames that don't exist.
--   * public_chat.session_uuid — NO FK: headless players emit uuids
--     with no session row (world sends p.sessionOrHeadless()).
--   * message_* / account_session / wealth_event — dormant landing
--     tables, NO FKs, mirroring their prisma-generated DDL.

CREATE TABLE account (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password TEXT NOT NULL,
    registration_ip TEXT NOT NULL DEFAULT '',
    staff_mod_level INTEGER NOT NULL DEFAULT 0,
    members INTEGER NOT NULL DEFAULT 0,
    banned_until TEXT,
    muted_until TEXT
);

CREATE TABLE account_login (
    account_id  INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    profile     TEXT    NOT NULL,
    node_id     INTEGER NOT NULL DEFAULT 0,
    logged_in   INTEGER NOT NULL DEFAULT 0,
    logged_out  INTEGER NOT NULL DEFAULT 0,
    logout_time TEXT,
    PRIMARY KEY (account_id, profile)
);

CREATE TABLE session (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_uuid TEXT NOT NULL CHECK (
        session_uuid GLOB '????????-????-????-????-????????????'
    ),
    account_id INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    profile TEXT NOT NULL,
    world INTEGER NOT NULL DEFAULT 0,
    uid INTEGER NOT NULL DEFAULT 0,
    login_time TEXT NOT NULL,
    remote_address TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_session_account_profile ON session (account_id, profile);

CREATE TABLE ipban (
    ip TEXT NOT NULL PRIMARY KEY,
    added_by TEXT NOT NULL DEFAULT '',
    added_on TEXT NOT NULL DEFAULT ''
);

CREATE TABLE hiscore (
    account_id INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    profile    TEXT    NOT NULL DEFAULT 'main',
    type       INTEGER NOT NULL,
    level      INTEGER NOT NULL,
    value      INTEGER NOT NULL,
    date       TEXT    NOT NULL,
    PRIMARY KEY (profile, type, account_id)
);

CREATE TABLE hiscore_large (
    account_id INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    profile    TEXT    NOT NULL DEFAULT 'main',
    type       INTEGER NOT NULL,
    level      INTEGER NOT NULL,
    value      INTEGER NOT NULL,
    date       TEXT    NOT NULL,
    PRIMARY KEY (profile, type, account_id)
);

CREATE TABLE login (
    uuid       TEXT    NOT NULL PRIMARY KEY,
    account_id INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    world      INTEGER NOT NULL,
    timestamp  TEXT    NOT NULL,
    uid        INTEGER NOT NULL DEFAULT 0,
    ip         TEXT
);

CREATE INDEX idx_login_account_ip_time ON login (account_id, ip, timestamp);

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

-- ==== friends tables: TS 274 shape + goscape FK extensions ====

CREATE TABLE friendlist (
    profile           TEXT    NOT NULL,
    account_id        INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    friend_account_id INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    created           TEXT    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (profile, account_id, friend_account_id)
);

-- Backs GetFollowers / IsVisibleTo reverse lookups (friend-side scan).
CREATE INDEX idx_friendlist_friend ON friendlist (profile, friend_account_id);

CREATE TABLE ignorelist (
    profile    TEXT    NOT NULL,
    account_id INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    value      TEXT    NOT NULL,
    created    TEXT    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (profile, account_id, value)
);

CREATE TABLE private_chat (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id    INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    profile       TEXT    NOT NULL,
    timestamp     TEXT    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    coord         INTEGER NOT NULL,
    to_account_id INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    message       TEXT    NOT NULL
);

CREATE INDEX idx_private_chat_to   ON private_chat (profile, to_account_id, timestamp);
CREATE INDEX idx_private_chat_from ON private_chat (profile, account_id, timestamp);

CREATE TABLE public_chat (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    session_uuid TEXT    NOT NULL,
    timestamp    TEXT    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    coord        INTEGER NOT NULL,
    message      TEXT    NOT NULL
);

CREATE INDEX idx_public_chat_session ON public_chat (session_uuid, timestamp);
```

- [ ] **Step 2: Implement `pkg/gamedb/migrate.go`**

```go
package gamedb

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"

	"github.com/golang-migrate/migrate/v4"
	sqlitedriver "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations
var migrationsFS embed.FS

// Migrate applies all pending up-migrations for the active dialect.
// m.Close() is intentionally omitted: the sqlite driver's Close()
// closes the *sql.DB passed to WithInstance, which would invalidate
// all subsequent queries on this pool.
//
// A schema AHEAD of this binary (rollback scenario) surfaces here as a
// golang-migrate version error → the caller (database module or test)
// fails fast.
func (d *DB) Migrate(_ context.Context) error {
	dir := "migrations/sqlite"
	if d.dialect == dialectPostgres {
		dir = "migrations/postgres"
	}
	sub, err := fs.Sub(migrationsFS, dir)
	if err != nil {
		return fmt.Errorf("gamedb: migrations subdir: %w", err)
	}
	src, err := iofs.New(sub, ".")
	if err != nil {
		return fmt.Errorf("gamedb: iofs source: %w", err)
	}
	if d.dialect == dialectPostgres {
		return errors.New("gamedb: postgres migrations not yet supported (Phase 2)")
	}
	drv, err := sqlitedriver.WithInstance(d.DB, &sqlitedriver.Config{})
	if err != nil {
		return fmt.Errorf("gamedb: sqlite driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "sqlite", drv)
	if err != nil {
		return fmt.Errorf("gamedb: migrate instance: %w", err)
	}
	if err = m.Up(); errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}
```

- [ ] **Step 3: Write migration tests (FK cascade, CHECK, two independent pools)**

`pkg/gamedb/migrate_test.go`:

```go
package gamedb

import (
	"fmt"
	"net/url"
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
		"friendlist", "ignorelist", "private_chat", "public_chat",
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
	mustExec(`INSERT INTO private_chat (account_id, profile, coord, to_account_id, message) VALUES (?, 'main', 0, ?, 'hi')`, owner, friend)
	mustExec(`INSERT INTO hiscore (account_id, type, level, value, date) VALUES (?, 0, 3, 1154, '2026-07-05 00:00:00')`, owner)

	mustExec(`DELETE FROM account WHERE id = ?`, owner)

	for _, tc := range []struct {
		table, where string
		arg          int64
		want         int
	}{
		{"friendlist", "account_id", owner, 0},
		{"ignorelist", "account_id", owner, 0},
		{"private_chat", "account_id", owner, 0},
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
	// (FriendServerRepository.ts:247-296 never resolves the target).
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
```

(Import only what the final file uses — `fmt`, `path/filepath`, `testing`.)

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamedb/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/gamedb/
git commit --no-gpg-sign -m "feat(gamedb): unified central-db schema (TS-274 friends shape, FK+CASCADE) + Migrate"
```

---

### Task 3: `database` module — migration anchor in the app graph

**Files:**
- Create: `pkg/gamedb/service.go`
- Create: `pkg/gamedb/service_test.go`
- Modify: `cmd/goscape/app/config.go` (add `Database` section)
- Modify: `cmd/goscape/app/modules.go` (register module + deps)

**Interfaces:**
- Consumes: `gamedb.Open`, `(*DB).Migrate`, `gamedb.Config` (Tasks 1–2); `services.NewBasicService`, `modules.UserInvisibleModule` (existing dskit).
- Produces: `gamedb.NewMigratorService(cfg Config, logger *slog.Logger) services.Service`; app config field `Config.Database gamedb.Config` (yaml key `database`); module name const `Database = "database"`; graph deps `Login → Database`, `Friends → Database` (wired here, consumed implicitly by Tasks 4–5).

- [ ] **Step 1: Write failing service test**

`pkg/gamedb/service_test.go`:

```go
package gamedb

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/dskit/services"
)

func TestMigratorService_MigratesThenIdles(t *testing.T) {
	cfg := defaultConfig()
	cfg.SQLite.DSN = filepath.Join(t.TempDir(), "goscape.db")

	svc := NewMigratorService(cfg, noopLogger())
	if err := services.StartAndAwaitRunning(t.Context(), svc); err != nil {
		t.Fatalf("StartAndAwaitRunning: %v", err)
	}

	// Schema must exist for an independent client by the time the
	// service reports Running.
	db, err := Open(cfg, noopLogger())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='friendlist'`).Scan(&n); err != nil {
		t.Fatalf("probe: %v", err)
	}
	if n != 1 {
		t.Error("friendlist table missing after migrator Running")
	}

	svc.StopAsync()
	if err := svc.AwaitTerminated(t.Context()); err != nil {
		t.Fatalf("AwaitTerminated: %v", err)
	}
	_ = time.Second // no timing assertions; ctx from t.Context()
}

func TestMigratorService_FailsOnUnknownBackend(t *testing.T) {
	cfg := defaultConfig()
	cfg.Backend = "bogus"
	svc := NewMigratorService(cfg, noopLogger())
	if err := services.StartAndAwaitRunning(t.Context(), svc); err == nil {
		t.Fatal("StartAndAwaitRunning: got nil error, want failure (unknown backend)")
	}
}
```

Check `pkg/dskit/services` for the exact helper name: if `StartAndAwaitRunning` does not exist, use the pattern found in existing module tests (`svc.StartAsync(ctx)` + `svc.AwaitRunning(ctx)` — grep `AwaitRunning` in `modules/*/friends_shutdown_test.go` for the established idiom) and adjust both tests. Delete the `_ = time.Second` line if unused.

- [ ] **Step 2: Implement `pkg/gamedb/service.go`**

```go
package gamedb

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/zsrv/goscape/pkg/dskit/services"
)

// NewMigratorService wraps schema migration in a dskit service. It is
// the `database` module: it opens a short-lived connection, applies all
// pending migrations, closes that connection, then idles until stopped.
// Modules that use the DB (login, friends) depend on this module in the
// dskit graph, so the topological start order guarantees schema exists
// before any dependent service accepts work — in every target
// combination. The migrator holds NO runtime connection: services are
// independent clients and open their own pools (spec §Design 2).
//
// In split deployments each process runs its own migrator at boot;
// on SQLite the processes share a file (same host) mediated by
// busy_timeout, on Postgres golang-migrate takes an advisory lock.
func NewMigratorService(cfg Config, logger *slog.Logger) services.Service {
	starting := func(ctx context.Context) error {
		db, err := Open(cfg, logger)
		if err != nil {
			return fmt.Errorf("database: open: %w", err)
		}
		if err := db.Migrate(ctx); err != nil {
			db.Close()
			return fmt.Errorf("database: migrate: %w", err)
		}
		logger.Info("central database schema up to date", "backend", cfg.Backend)
		return db.Close()
	}
	running := func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	}
	return services.NewBasicService(starting, running, nil)
}
```

If `services.NewBasicService` rejects a nil stopping fn (check its implementation in `pkg/dskit/services/basic_service.go`), pass `func(_ error) error { return nil }` instead.

- [ ] **Step 3: Run gamedb tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamedb/ -v`
Expected: PASS.

- [ ] **Step 4: Wire the app config**

In `cmd/goscape/app/config.go`:

1. Add import `"github.com/zsrv/goscape/pkg/gamedb"`.
2. Add the field to `Config` (after `LogSource`, before the module sections):

```go
	Database gamedb.Config `yaml:"database,omitempty"`
```

3. In `RegisterFlagsAndApplyDefaults`, alongside the other module calls:

```go
	c.Database.RegisterFlagsAndApplyDefaults(f)
```

4. In `Validate()`, before the module fan-out (gamedb errors self-prefix `database: `):

```go
	if err := c.Database.Validate(); err != nil {
		return err
	}
```

- [ ] **Step 5: Register the module**

In `cmd/goscape/app/modules.go`:

1. Add const with the other individual targets:

```go
	Database string = "database"
```

2. Add the init function (after `initFriends`):

```go
// initDatabase is the migration anchor: it brings the central-database
// schema up to date before any DB-using module starts (login and
// friends both depend on it in the graph). It holds no runtime
// connection — login and friends each open their own pool
// (independent-clients model, pkg/gamedb doc).
func (g *App) initDatabase() (services.Service, error) {
	if !g.cfg.Login.Enable && !g.cfg.Friends.Enable {
		// No DB consumer in this target — contribute no service
		// (arch-29.8 posture: a disabled module must not masquerade
		// as Running).
		g.logger.Info("module disabled", "module", "database")
		return nil, nil
	}

	logger, err := log.NewLogger(slog.Level(g.cfg.LogLevel), g.cfg.LogFormat, os.Stdout, log.WithSourceFormat(g.cfg.LogSource))
	if err != nil {
		return nil, fmt.Errorf("failed to create database logger: %w", err)
	}
	logger = logger.With("component", "database")

	return gamedb.NewMigratorService(g.cfg.Database, logger), nil
}
```

3. Add import `"github.com/zsrv/goscape/pkg/gamedb"`.
4. In `setupModuleManager`, register it as user-invisible (it is not a runnable `--target`):

```go
	mm.RegisterModule(Database, g.initDatabase, modules.UserInvisibleModule)
```

5. Update the deps map:

```go
	deps := map[string][]string{
		Common: {},

		Database: {Common},
		OnDemand: {Common, World},
		Friends:  {Common, Database},
		Login:    {Common, Database},
		World:    {Common, Login, Friends},

		SingleBinary: {OnDemand, Friends, Login, World},
	}
```

- [ ] **Step 6: Build + full test sweep**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./... && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/... ./pkg/gamedb/`
Expected: build OK; tests PASS. (login/friends still run on their own DSNs at this point — the database module coexists.)

- [ ] **Step 7: Commit**

```bash
git add pkg/gamedb/ cmd/goscape/app/
git commit --no-gpg-sign -m "feat(app): database module — central-db migration anchor + database: config section"
```

---

### Task 4: Login module → gamedb client

**Files:**
- Modify: `modules/login/login.go` (New signature, starting)
- Modify: `modules/login/config.go` (drop SQLiteDSN)
- Modify: `modules/login/db.go` (delete open/migrate helpers + embed; thread `*gamedb.DB`; Rebind)
- Delete: `modules/login/migrations/` (entire directory)
- Modify: `modules/login/server.go`, `modules/login/handler.go`, `modules/login/hiscore.go`, `modules/login/emit.go` (thread `*gamedb.DB` where `*sql.DB` appears)
- Modify: `modules/login/db_test.go`, other `modules/login/*_test.go` (test helper)
- Modify: `cmd/goscape/app/modules.go` (initLogin passes `g.cfg.Database`)

**Interfaces:**
- Consumes: `gamedb.Open`, `*gamedb.DB`, `(*DB).Rebind`, `(*DB).Migrate` (Tasks 1–2).
- Produces: `login.New(cfg Config, dbCfg gamedb.Config, logger *slog.Logger) (*Login, error)` — Task 3's app wiring is updated here to pass `g.cfg.Database`.

- [ ] **Step 1: Change the module wiring**

`modules/login/login.go` — struct gains `dbCfg`, `db` becomes `*gamedb.DB`:

```go
// Login is the login server module. It owns its private pool to the
// central database and the gRPC server.
type Login struct {
	services.Service

	cfg   Config
	dbCfg gamedb.Config
	log   *slog.Logger

	db  *gamedb.DB
	srv *grpcServer
	lis net.Listener
}

// New validates the config and constructs the Login module. dbCfg is
// the shared database: section — the module opens its OWN pool with it
// in starting() (independent-clients model; schema is migrated by the
// database module, which login depends on in the app graph).
func New(cfg Config, dbCfg gamedb.Config, logger *slog.Logger) (*Login, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	l := &Login{cfg: cfg, dbCfg: dbCfg, log: logger}
	l.Service = services.NewBasicService(l.starting, l.running, l.stopping)
	return l, nil
}

func (l *Login) starting(ctx context.Context) error {
	db, err := gamedb.Open(l.dbCfg, l.log)
	if err != nil {
		return fmt.Errorf("open central database: %w", err)
	}

	srv := newGRPCServer(l.cfg, db, l.log)
	lis, err := srv.listen(l.cfg)
	if err != nil {
		db.Close()
		return err
	}

	l.db = db
	l.srv = srv
	l.lis = lis
	return nil
}
```

Imports: add `"github.com/zsrv/goscape/pkg/gamedb"`, drop `"database/sql"`.

- [ ] **Step 2: Slim `modules/login/db.go`**

Delete: the `//go:embed migrations/*.sql` block, `openDB`, `dsnWithPragmas`, `ensureDBParentDir`, `migrateDB`, and the now-unused imports (`embed`, `errors`, `os`, `path/filepath`, `strings`, golang-migrate imports, the modernc blank import). Delete the whole `modules/login/migrations/` directory.

Then thread `*gamedb.DB` through every remaining function that takes `db *sql.DB`, wrapping each query in `db.Rebind(...)`. Worked example — `accountByUsername` (apply the identical mechanical pattern to all listed functions):

```go
// before
func accountByUsername(ctx context.Context, db *sql.DB, username, profile string) (*accountRow, error) {
	row := db.QueryRowContext(ctx, `SELECT ... WHERE a.username = ? ...`, username, profile)
// after
func accountByUsername(ctx context.Context, db *gamedb.DB, username, profile string) (*accountRow, error) {
	row := db.QueryRowContext(ctx, db.Rebind(`SELECT ... WHERE a.username = ? ...`), username, profile)
```

Functions to convert in `modules/login/db.go` (from the current function inventory): `accountByUsername`, `ipBanned`, `insertAccount`, `setAccountMembers`, `upsertAccountLogin`, `insertSession`, `clearWorldSessions`, `setLoggedOut`, `clearLoggedInFlag`, `setAccountBanned`, `setAccountMuted`. The tx-scoped helpers `upsertAccountLoginTx` and `insertSessionTx` keep their `execer` param but gain a `db *gamedb.DB` first-class param purely for `db.Rebind`:

```go
func upsertAccountLoginTx(ctx context.Context, db *gamedb.DB, ex execer, accountID int, profile string, nodeID int) error {
	_, err := ex.ExecContext(ctx, db.Rebind(`INSERT INTO account_login (...) VALUES (?, ?, ?) ON CONFLICT ...`), ...)
	...
}
```

(Update their callers accordingly.) Also convert `db *sql.DB` fields/params in `modules/login/server.go` (`newGRPCServer`), `modules/login/handler.go`, `modules/login/hiscore.go`, and `modules/login/emit.go` to `*gamedb.DB`, wrapping every SQL string in `db.Rebind(...)`. Enumerate all sites first:

Run: `grep -n "sql.DB\|QueryRowContext\|QueryContext\|ExecContext" modules/login/*.go | grep -v _test`
Convert every hit. Keep `database/sql` imported only where `sql.NullString`/`sql.ErrNoRows`/`execer` still need it.

- [ ] **Step 3: Drop the config key**

`modules/login/config.go`: remove the `SQLiteDSN` field, its `f.StringVar(... "login.sqlite-dsn" ...)` line, and its `Validate` check. Everything else stays.

- [ ] **Step 4: Update app wiring**

`cmd/goscape/app/modules.go` `initLogin`: change the constructor call to

```go
	l, err := login.New(g.cfg.Login, g.cfg.Database, logger)
```

- [ ] **Step 5: Update the login test helper**

`modules/login/db_test.go`: replace `createTestDB` with:

```go
// createTestDB opens an isolated in-memory central DB via gamedb and
// applies the unified migration lineage.
func createTestDB(t *testing.T) *gamedb.DB {
	t.Helper()
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
```

Then run the login suite and fix fallout mechanically:
- Tests asserting old pragma helpers (`TestOpenDB_PragmasApplied` etc. in `db_test.go`) — DELETE; that behavior is now pinned in `pkg/gamedb` (Task 1).
- Tests inserting `session_uuid = ''` rows (the legacy allowance is gone) — update to well-formed UUIDs.
- Any test constructing `login.New(cfg, logger)` — add the `gamedb.Config` argument (build one with `RegisterFlagsAndApplyDefaults` + temp-dir DSN, and pre-migrate it via `gamedb.Open`+`Migrate`+`Close` if the test exercises `starting()`).

- [ ] **Step 6: Run the suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ ./cmd/... -count=1`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add modules/login/ cmd/goscape/app/modules.go
git commit --no-gpg-sign -m "refactor(login): independent gamedb client of the central database; retire private lineage"
```

---

### Task 5: Friends repository re-key to TS 274 shape

**Files:**
- Modify: `modules/friends/friends.go` (New signature, starting)
- Modify: `modules/friends/config.go` (drop SQLiteDSN)
- Modify: `modules/friends/repository.go` (all persistence re-keyed)
- Delete: `modules/friends/db.go`, `modules/friends/db_test.go`, `modules/friends/migrations/`
- Modify: `modules/friends/repository_test.go`, `modules/friends/handler_test.go`, `modules/friends/friends_shutdown_test.go` (helper + seeding)
- Modify: `cmd/goscape/app/modules.go` (initFriends passes `g.cfg.Database`)

**Interfaces:**
- Consumes: `gamedb.Open`, `*gamedb.DB`, `Rebind`, schema from Task 2; `jstring.FromBase37`/`jstring.ToBase37` (existing `pkg/util/jstring`).
- Produces (Task 6 relies on): `errAccountMissing` sentinel; `(r *Repository) LogPrivateMessage(ctx, from, to uint64, coord int32, message string) error` (returns `errAccountMissing`); `(r *Repository) LogPublicMessage(ctx context.Context, sessionUUID string, coord int32, message string) error` (world param GONE). Public repository API otherwise keeps its username37-based signatures (TS's own API shape).

**Behavior contract (each function mirrors its TS 274 counterpart):**

| Go method | TS reference | Restored behavior |
|---|---|---|
| `AddFriend` | `FriendServerRepository.ts:204-245` | resolves owner (`id`,`members`) + target; either missing → silent no-op; cap `members ? 200 : 100`; cap counts across ALL profiles (TS quirk: no profile filter on the count) |
| `DeleteFriend` | `:182-202` | delete via account-subqueries |
| `GetFriends` | `loadFriends :356-370` | double `INNER JOIN account`, `ORDER BY f.created ASC` |
| `AddIgnore` | `:247-296` | resolves owner only; target stored as raw username string (`value`), unchecked; cap 100, ALL profiles; `ON CONFLICT DO NOTHING` |
| `DeleteIgnore` | `:298-316` | delete by (profile, value, owner-subquery) |
| `GetIgnores` | `loadIgnores :372-386` | join owner account, select `i.value`, `ORDER BY i.created ASC` |
| `GetFollowers` | `:176-180` (in-memory in TS; goscape keeps its SQL mechanism, now id-keyed) | reverse join |
| `IsVisibleTo`/`IsVisibleToMany` | `:331-354` | same semantics, id-keyed SQL |
| `LogPrivateMessage` | `FriendServer.ts:266-284` | resolve both endpoints, missing → `errAccountMissing` (TS throw → catch → drop) |
| `LogPublicMessage` | `FriendServer.ts:286-297` | TS-exact row `{session_uuid, timestamp(default), coord, message}` |

- [ ] **Step 1: Write the failing new-behavior tests**

Append to `modules/friends/repository_test.go` (these compile against the new API; the suite goes red until Step 3):

```go
// seedAccount inserts an account whose username is the canonical
// FromBase37 form of username37, mirroring how the login module and TS
// both key accounts by username. Returns the account id.
func seedAccount(t *testing.T, db *gamedb.DB, username37 uint64) int64 {
	t.Helper()
	var id int64
	err := db.QueryRowContext(t.Context(),
		db.Rebind(`INSERT INTO account (username, password) VALUES (?, '') RETURNING id`),
		jstring.FromBase37(username37),
	).Scan(&id)
	if err != nil {
		t.Fatalf("seedAccount(%d): %v", username37, err)
	}
	return id
}

// seedMemberAccount is seedAccount with members=1 (the TS 274
// members-aware friend cap, FriendServerRepository.ts:229).
func seedMemberAccount(t *testing.T, db *gamedb.DB, username37 uint64) int64 {
	t.Helper()
	var id int64
	err := db.QueryRowContext(t.Context(),
		db.Rebind(`INSERT INTO account (username, password, members) VALUES (?, '', 1) RETURNING id`),
		jstring.FromBase37(username37),
	).Scan(&id)
	if err != nil {
		t.Fatalf("seedMemberAccount(%d): %v", username37, err)
	}
	return id
}

func TestAddFriend_MissingTarget_NoInsert(t *testing.T) {
	// TS FriendServerRepository.ts:219-222: `if (!account || !friendAccount) return`.
	r, db := newTestRepo(t)
	seedAccount(t, db, 1)
	if err := r.AddFriend(t.Context(), 1, 2); err != nil { // 2 has no account
		t.Fatalf("AddFriend: %v", err)
	}
	friends, err := r.GetFriends(t.Context(), 1)
	if err != nil {
		t.Fatalf("GetFriends: %v", err)
	}
	if len(friends) != 0 {
		t.Errorf("friend of missing account persisted: %v", friends)
	}
}

func TestAddFriend_MissingOwner_NoInsert(t *testing.T) {
	r, db := newTestRepo(t)
	seedAccount(t, db, 2)
	if err := r.AddFriend(t.Context(), 1, 2); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	if friends, _ := r.GetFriends(t.Context(), 1); len(friends) != 0 {
		t.Errorf("friend row for missing owner persisted: %v", friends)
	}
}

func TestAddFriend_BothExist_Persists(t *testing.T) {
	// Dual-pin: presence AND absence (ts_asymmetry posture).
	r, db := newTestRepo(t)
	seedAccount(t, db, 1)
	seedAccount(t, db, 2)
	if err := r.AddFriend(t.Context(), 1, 2); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	friends, err := r.GetFriends(t.Context(), 1)
	if err != nil {
		t.Fatalf("GetFriends: %v", err)
	}
	if len(friends) != 1 || friends[0] != 2 {
		t.Errorf("GetFriends: got %v, want [2]", friends)
	}
}

func TestAddFriend_NonMemberCap100(t *testing.T) {
	// TS: limit = account.members ? 200 : 100 (FriendServerRepository.ts:229).
	r, db := newTestRepo(t)
	seedAccount(t, db, 1)
	for i := uint64(2); i < 103; i++ {
		seedAccount(t, db, i)
	}
	for i := uint64(2); i < 102; i++ { // 100 friends
		if err := r.AddFriend(t.Context(), 1, i); err != nil {
			t.Fatalf("AddFriend #%d: %v", i, err)
		}
	}
	if err := r.AddFriend(t.Context(), 1, 102); err != nil {
		t.Fatalf("AddFriend #101: %v", err)
	}
	friends, _ := r.GetFriends(t.Context(), 1)
	if len(friends) != 100 {
		t.Errorf("non-member cap: got %d friends, want 100", len(friends))
	}
}

func TestAddFriend_MemberCap200(t *testing.T) {
	r, db := newTestRepo(t)
	seedMemberAccount(t, db, 1)
	for i := uint64(2); i < 104; i++ {
		seedAccount(t, db, i)
	}
	for i := uint64(2); i < 104; i++ { // 102 friends — over the non-member cap
		if err := r.AddFriend(t.Context(), 1, i); err != nil {
			t.Fatalf("AddFriend #%d: %v", i, err)
		}
	}
	friends, _ := r.GetFriends(t.Context(), 1)
	if len(friends) != 102 {
		t.Errorf("member cap: got %d friends, want 102 (limit 200)", len(friends))
	}
}

func TestAddIgnore_NonexistentTarget_Succeeds(t *testing.T) {
	// TS never resolves the ignore target (FriendServerRepository.ts:247-296):
	// ignoring a player who doesn't exist works.
	r, db := newTestRepo(t)
	seedAccount(t, db, 1)
	if err := r.AddIgnore(t.Context(), 1, 999); err != nil {
		t.Fatalf("AddIgnore(nonexistent target): %v", err)
	}
	ignores, err := r.GetIgnores(t.Context(), 1)
	if err != nil {
		t.Fatalf("GetIgnores: %v", err)
	}
	if len(ignores) != 1 || ignores[0] != 999 {
		t.Errorf("GetIgnores: got %v, want [999]", ignores)
	}
}

func TestAddIgnore_MissingOwner_NoOp(t *testing.T) {
	// TS resolves the OWNER and returns on miss (FriendServerRepository.ts:256-260).
	r, _ := newTestRepo(t)
	if err := r.AddIgnore(t.Context(), 1, 2); err != nil {
		t.Fatalf("AddIgnore: %v", err)
	}
	if ignores, _ := r.GetIgnores(t.Context(), 1); len(ignores) != 0 {
		t.Errorf("ignore row for missing owner persisted: %v", ignores)
	}
}

func TestLogPrivateMessage_MissingEndpoint_ErrAccountMissing(t *testing.T) {
	// TS FriendServer.ts:270-271 executeTakeFirstOrThrow → outer catch →
	// PM dropped with no insert.
	r, db := newTestRepo(t)
	seedAccount(t, db, 1)
	err := r.LogPrivateMessage(t.Context(), 1, 2, 0, "hello")
	if !errors.Is(err, errAccountMissing) {
		t.Fatalf("LogPrivateMessage(missing to): got %v, want errAccountMissing", err)
	}
	err = r.LogPrivateMessage(t.Context(), 3, 1, 0, "hello")
	if !errors.Is(err, errAccountMissing) {
		t.Fatalf("LogPrivateMessage(missing from): got %v, want errAccountMissing", err)
	}
	var n int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM private_chat`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("private_chat rows: got %d, want 0", n)
	}
}

func TestLogPrivateMessage_BothExist_PersistsIDKeyedRow(t *testing.T) {
	r, db := newTestRepo(t)
	fromID := seedAccount(t, db, 1)
	toID := seedAccount(t, db, 2)
	if err := r.LogPrivateMessage(t.Context(), 1, 2, 12345, "hello"); err != nil {
		t.Fatalf("LogPrivateMessage: %v", err)
	}
	var gotFrom, gotTo int64
	var gotCoord int32
	var gotMsg, gotProfile string
	err := db.QueryRowContext(t.Context(),
		`SELECT account_id, to_account_id, coord, message, profile FROM private_chat`,
	).Scan(&gotFrom, &gotTo, &gotCoord, &gotMsg, &gotProfile)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if gotFrom != fromID || gotTo != toID || gotCoord != 12345 || gotMsg != "hello" || gotProfile != "test" {
		t.Errorf("row: got (%d,%d,%d,%q,%q), want (%d,%d,12345,\"hello\",\"test\")",
			gotFrom, gotTo, gotCoord, gotMsg, gotProfile, fromID, toID)
	}
}

func TestLogPublicMessage_TSRowShape(t *testing.T) {
	// TS FriendServer.ts:286-297 inserts {session_uuid, timestamp, coord,
	// message} — no profile, no world (recovered by joining session).
	r, db := newTestRepo(t)
	if err := r.LogPublicMessage(t.Context(), "01234567-89ab-cdef-0123-456789abcdef", 99, "gday"); err != nil {
		t.Fatalf("LogPublicMessage: %v", err)
	}
	var uuid, msg string
	var coord int32
	err := db.QueryRowContext(t.Context(),
		`SELECT session_uuid, coord, message FROM public_chat`,
	).Scan(&uuid, &coord, &msg)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if uuid != "01234567-89ab-cdef-0123-456789abcdef" || coord != 99 || msg != "gday" {
		t.Errorf("row: got (%q,%d,%q)", uuid, coord, msg)
	}
}
```

Also update the test-DB helper. `modules/friends/db_test.go` is deleted with `db.go`; move `createTestDB` (gamedb flavor, identical to Task 4 Step 5's version) and `noopLogger` into `modules/friends/repository_test.go`, and change `newTestRepo`:

```go
func newTestRepo(t *testing.T) (*Repository, *gamedb.DB) {
	t.Helper()
	db := createTestDB(t)
	return NewRepository(db, "test"), db
}
```

- [ ] **Step 2: Run to verify red**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -count=1`
Expected: FAIL (compile errors: `gamedb` types, `errAccountMissing`, changed signatures).

- [ ] **Step 3: Re-key `modules/friends/repository.go`**

Header changes:

```go
import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/zsrv/goscape/pkg/gamedb"
	"github.com/zsrv/goscape/pkg/util/jstring"
)

// errAccountMissing is LogPrivateMessage's sentinel for "an endpoint
// account does not exist". Mirrors TS FriendServer.ts:270-271, where
// executeTakeFirstOrThrow throws and the outer catch drops the PM.
// The handler maps it to a silent drop (no delivery, success RPC).
var errAccountMissing = errors.New("account missing")

// friendListLimit / membersFriendListLimit cap the friend list per
// owner: TS 274 is members-aware — `account.members ? 200 : 100`
// (FriendServerRepository.ts:229). ignoreListLimit stays a flat 100
// (FriendServerRepository.ts:270).
const (
	friendListLimit        = 100
	membersFriendListLimit = 200
	ignoreListLimit        = 100
)
```

`repositories` and `Repository` swap `db *sql.DB` → `db *gamedb.DB` (`newRepositories(db *gamedb.DB)`, `NewRepository(db *gamedb.DB, profile string)`). The in-memory presence code (`GetWorld`, `InitializeWorld`, `initializeWorldIfAbsent`, `Register`, `Unregister`, `SetChatMode`, `GetChatMode`, `isStaffLocked`) is untouched.

New shared resolver:

```go
// accountID resolves username37 to its account row id via the central
// database — the friend server verifying a username IS this query
// (TS FriendServerRepository.ts:216-217/256; FriendServer.ts:270-271).
// ok=false with nil error when the account does not exist.
func (r *Repository) accountID(ctx context.Context, username37 uint64) (int64, bool, error) {
	var id int64
	err := r.db.QueryRowContext(ctx,
		r.db.Rebind(`SELECT id FROM account WHERE username = ? LIMIT 1`),
		jstring.FromBase37(username37),
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("accountID: %w", err)
	}
	return id, true, nil
}
```

Replace `AddFriend` + delete `atomicUpsertList` (the two lists' flows diverge now; each owns its tx). The per-call BeginTx posture is retained — it closes the recheck-vs-concurrent-delete race the old DB-2 fix documented:

```go
// AddFriend adds target to owner's friend list, resolving both accounts
// against the central database like TS FriendServerRepository.addFriend
// (FriendServerRepository.ts:204-245): either account missing → silent
// no-op; duplicate → no-op; cap members ? 200 : 100. TS counts the cap
// across ALL profiles (no profile filter on the count query,
// FriendServerRepository.ts:224-228) — quirk mirrored. The whole
// read-modify-write runs in one tx so a concurrent DeleteFriend cannot
// interleave (retained from the DB-2-era fix).
func (r *Repository) AddFriend(ctx context.Context, owner, target uint64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("AddFriend: begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var ownerID, members int64
	err = tx.QueryRowContext(ctx,
		r.db.Rebind(`SELECT id, members FROM account WHERE username = ? LIMIT 1`),
		jstring.FromBase37(owner),
	).Scan(&ownerID, &members)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // TS :219-222 — missing owner, drop silently
	}
	if err != nil {
		return fmt.Errorf("AddFriend: resolve owner: %w", err)
	}

	var targetID int64
	err = tx.QueryRowContext(ctx,
		r.db.Rebind(`SELECT id FROM account WHERE username = ? LIMIT 1`),
		jstring.FromBase37(target),
	).Scan(&targetID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // TS :219-222 — missing target, drop silently
	}
	if err != nil {
		return fmt.Errorf("AddFriend: resolve target: %w", err)
	}

	var dup int
	err = tx.QueryRowContext(ctx,
		r.db.Rebind(`SELECT COUNT(*) FROM friendlist WHERE profile = ? AND account_id = ? AND friend_account_id = ?`),
		r.profile, ownerID, targetID,
	).Scan(&dup)
	if err != nil {
		return fmt.Errorf("AddFriend: dup check: %w", err)
	}
	if dup == 0 {
		var total int
		err = tx.QueryRowContext(ctx,
			r.db.Rebind(`SELECT COUNT(*) FROM friendlist WHERE account_id = ?`),
			ownerID,
		).Scan(&total)
		if err != nil {
			return fmt.Errorf("AddFriend: cap check: %w", err)
		}
		limit := friendListLimit
		if members != 0 {
			limit = membersFriendListLimit
		}
		if total >= limit {
			if err := tx.Commit(); err != nil {
				return fmt.Errorf("AddFriend: commit: %w", err)
			}
			committed = true
			return nil
		}
		if _, err = tx.ExecContext(ctx,
			r.db.Rebind(`INSERT INTO friendlist (profile, account_id, friend_account_id) VALUES (?, ?, ?)`),
			r.profile, ownerID, targetID,
		); err != nil {
			if gamedb.IsForeignKeyViolation(err) {
				// Account deleted between resolve and insert (possible
				// under Postgres read-committed) — same outcome as the
				// TS missing-account path: drop silently (spec §Error
				// handling). Deferred rollback cleans up.
				return nil
			}
			return fmt.Errorf("AddFriend: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("AddFriend: commit: %w", err)
	}
	committed = true
	return nil
}
```

```go
// DeleteFriend removes target from owner's friend list via account
// subqueries (TS FriendServerRepository.deleteFriend,
// FriendServerRepository.ts:196-201). No-op when either username has
// no account or the row does not exist.
func (r *Repository) DeleteFriend(ctx context.Context, owner, target uint64) error {
	_, err := r.db.ExecContext(ctx,
		r.db.Rebind(`DELETE FROM friendlist
		 WHERE profile = ?
		   AND account_id IN (SELECT id FROM account WHERE username = ?)
		   AND friend_account_id IN (SELECT id FROM account WHERE username = ?)`),
		r.profile, jstring.FromBase37(owner), jstring.FromBase37(target),
	)
	if err != nil {
		return fmt.Errorf("DeleteFriend: %w", err)
	}
	return nil
}
```

```go
// GetFriends returns owner's friend list as username37s, oldest entry
// first. Mirrors TS loadFriends' double INNER JOIN + orderBy f.created
// asc (FriendServerRepository.ts:356-370).
func (r *Repository) GetFriends(ctx context.Context, owner uint64) ([]uint64, error) {
	rows, err := r.db.QueryContext(ctx,
		r.db.Rebind(`SELECT a.username FROM account AS a
		 INNER JOIN friendlist AS f ON a.id = f.friend_account_id
		 INNER JOIN account AS local ON local.id = f.account_id
		 WHERE local.username = ? AND f.profile = ?
		 ORDER BY f.created ASC`),
		jstring.FromBase37(owner), r.profile,
	)
	if err != nil {
		return nil, fmt.Errorf("GetFriends: %w", err)
	}
	defer rows.Close()

	var out []uint64
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, fmt.Errorf("GetFriends scan: %w", err)
		}
		out = append(out, jstring.ToBase37(u))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetFriends rows: %w", err)
	}
	return out, nil
}
```

```go
// AddIgnore mirrors TS addIgnore (FriendServerRepository.ts:247-296):
// resolves the OWNER only (missing → no-op); the target is stored as a
// raw username string with NO existence check — ignoring a player who
// doesn't exist is allowed. Cap 100, counted across ALL profiles (TS
// quirk, :264-268). ON CONFLICT DO NOTHING matches TS's sqlite branch
// (:284-285) and is valid on both goscape backends.
func (r *Repository) AddIgnore(ctx context.Context, owner, target uint64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("AddIgnore: begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var ownerID int64
	err = tx.QueryRowContext(ctx,
		r.db.Rebind(`SELECT id FROM account WHERE username = ? LIMIT 1`),
		jstring.FromBase37(owner),
	).Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // TS :258-260
	}
	if err != nil {
		return fmt.Errorf("AddIgnore: resolve owner: %w", err)
	}

	var total int
	err = tx.QueryRowContext(ctx,
		r.db.Rebind(`SELECT COUNT(*) FROM ignorelist WHERE account_id = ?`),
		ownerID,
	).Scan(&total)
	if err != nil {
		return fmt.Errorf("AddIgnore: cap check: %w", err)
	}
	if total >= ignoreListLimit {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("AddIgnore: commit: %w", err)
		}
		committed = true
		return nil
	}

	if _, err = tx.ExecContext(ctx,
		r.db.Rebind(`INSERT INTO ignorelist (profile, account_id, value) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`),
		r.profile, ownerID, jstring.FromBase37(target),
	); err != nil {
		if gamedb.IsForeignKeyViolation(err) {
			return nil // owner deleted mid-flight — TS missing-owner outcome
		}
		return fmt.Errorf("AddIgnore: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("AddIgnore: commit: %w", err)
	}
	committed = true
	return nil
}
```

Note the cap-before-dup ordering: TS checks in-memory `includes()` first, then cap; the DB dup case is absorbed by `ON CONFLICT DO NOTHING`, and the cap check runs before insert exactly like TS (`:264-272`). A duplicate add at the cap is a no-op either way — semantics identical.

```go
// DeleteIgnore removes value from owner's ignore list
// (TS FriendServerRepository.deleteIgnore, :310-315: where profile,
// value, account-subquery).
func (r *Repository) DeleteIgnore(ctx context.Context, owner, target uint64) error {
	_, err := r.db.ExecContext(ctx,
		r.db.Rebind(`DELETE FROM ignorelist
		 WHERE profile = ? AND value = ?
		   AND account_id IN (SELECT id FROM account WHERE username = ?)`),
		r.profile, jstring.FromBase37(target), jstring.FromBase37(owner),
	)
	if err != nil {
		return fmt.Errorf("DeleteIgnore: %w", err)
	}
	return nil
}

// GetIgnores returns owner's ignore list as username37s, oldest first
// (TS loadIgnores, :372-386: join owner account, select i.value,
// orderBy i.created asc; values round-trip through toBase37).
func (r *Repository) GetIgnores(ctx context.Context, owner uint64) ([]uint64, error) {
	rows, err := r.db.QueryContext(ctx,
		r.db.Rebind(`SELECT i.value FROM account AS local
		 INNER JOIN ignorelist AS i ON local.id = i.account_id
		 WHERE local.username = ? AND i.profile = ?
		 ORDER BY i.created ASC`),
		jstring.FromBase37(owner), r.profile,
	)
	if err != nil {
		return nil, fmt.Errorf("GetIgnores: %w", err)
	}
	defer rows.Close()

	var out []uint64
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("GetIgnores scan: %w", err)
		}
		out = append(out, jstring.ToBase37(v))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetIgnores rows: %w", err)
	}
	return out, nil
}

// GetFollowers returns the username37s of all players who have target
// in their friend list. TS computes this from its in-memory cache
// (FriendServerRepository.ts:176-180); goscape keeps its established
// SQL mechanism, now id-keyed, backed by idx_friendlist_friend.
func (r *Repository) GetFollowers(ctx context.Context, target uint64) ([]uint64, error) {
	rows, err := r.db.QueryContext(ctx,
		r.db.Rebind(`SELECT local.username FROM friendlist AS f
		 INNER JOIN account AS local ON local.id = f.account_id
		 INNER JOIN account AS a ON a.id = f.friend_account_id
		 WHERE f.profile = ? AND a.username = ?`),
		r.profile, jstring.FromBase37(target),
	)
	if err != nil {
		return nil, fmt.Errorf("GetFollowers: %w", err)
	}
	defer rows.Close()

	var out []uint64
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, fmt.Errorf("GetFollowers scan: %w", err)
		}
		out = append(out, jstring.ToBase37(u))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetFollowers rows: %w", err)
	}
	return out, nil
}
```

`IsVisibleTo`: keep structure/locking; replace the two SQL probes:

```go
	// isIgnoredBy probe (unchanged semantics), and the FRIENDS-mode
	// membership check becomes:
	case 1: // FRIENDS
		var count int
		err := r.db.QueryRowContext(ctx,
			r.db.Rebind(`SELECT COUNT(*) FROM friendlist AS f
			 INNER JOIN account AS local ON local.id = f.account_id
			 INNER JOIN account AS a ON a.id = f.friend_account_id
			 WHERE f.profile = ? AND local.username = ? AND a.username = ?`),
			r.profile, jstring.FromBase37(other), jstring.FromBase37(viewer),
		).Scan(&count)
		if err != nil {
			return false, fmt.Errorf("IsVisibleTo: %w", err)
		}
		return count > 0, nil
```

```go
// isIgnoredBy reports whether owner has target on its ignore list
// (TS playerIgnores[other].includes(viewer), FriendServerRepository.ts:339).
func (r *Repository) isIgnoredBy(ctx context.Context, owner, target uint64) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		r.db.Rebind(`SELECT COUNT(*) FROM ignorelist AS i
		 INNER JOIN account AS local ON local.id = i.account_id
		 WHERE i.profile = ? AND local.username = ? AND i.value = ?`),
		r.profile, jstring.FromBase37(owner), jstring.FromBase37(target),
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("isIgnoredBy: %w", err)
	}
	return count > 0, nil
}
```

`IsVisibleToMany`: keep the algorithm; replace `targetsAmong` with two shape-specific helpers and update the two call sites (`r.targetsAmong(ctx, "ignorelist", …)` → `r.ignoreValuesAmong(ctx, other, viewers)`; `"friendlist"` → `r.friendTargetsAmong(ctx, other, viewers)`):

```go
// friendTargetsAmong returns the subset of candidates present in
// owner's friend list, via one IN query over usernames (id-keyed
// analogue of the old username37 IN probe; avoids N+1).
func (r *Repository) friendTargetsAmong(ctx context.Context, owner uint64, candidates []uint64) (map[uint64]bool, error) {
	found := make(map[uint64]bool, len(candidates))
	if len(candidates) == 0 {
		return found, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(candidates)), ",")
	args := make([]any, 0, 2+len(candidates))
	args = append(args, r.profile, jstring.FromBase37(owner))
	for _, c := range candidates {
		args = append(args, jstring.FromBase37(c))
	}
	rows, err := r.db.QueryContext(ctx,
		r.db.Rebind(`SELECT a.username FROM friendlist AS f
		 INNER JOIN account AS local ON local.id = f.account_id
		 INNER JOIN account AS a ON a.id = f.friend_account_id
		 WHERE f.profile = ? AND local.username = ? AND a.username IN (`+placeholders+`)`),
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("friendTargetsAmong: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, fmt.Errorf("friendTargetsAmong scan: %w", err)
		}
		found[jstring.ToBase37(u)] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("friendTargetsAmong rows: %w", err)
	}
	return found, nil
}

// ignoreValuesAmong is friendTargetsAmong against ignorelist.value
// (raw username strings, no target join).
func (r *Repository) ignoreValuesAmong(ctx context.Context, owner uint64, candidates []uint64) (map[uint64]bool, error) {
	found := make(map[uint64]bool, len(candidates))
	if len(candidates) == 0 {
		return found, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(candidates)), ",")
	args := make([]any, 0, 2+len(candidates))
	args = append(args, r.profile, jstring.FromBase37(owner))
	for _, c := range candidates {
		args = append(args, jstring.FromBase37(c))
	}
	rows, err := r.db.QueryContext(ctx,
		r.db.Rebind(`SELECT i.value FROM ignorelist AS i
		 INNER JOIN account AS local ON local.id = i.account_id
		 WHERE i.profile = ? AND local.username = ? AND i.value IN (`+placeholders+`)`),
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("ignoreValuesAmong: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("ignoreValuesAmong scan: %w", err)
		}
		found[jstring.ToBase37(v)] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ignoreValuesAmong rows: %w", err)
	}
	return found, nil
}
```

```go
// LogPrivateMessage persists a PM keyed by resolved account ids
// (TS FriendServer.ts:266-284: resolve from + to via
// executeTakeFirstOrThrow, insert {account_id, profile, to_account_id,
// timestamp, coord, message}). Either endpoint missing →
// errAccountMissing: the handler drops the PM silently, matching the
// TS throw-and-catch. timestamp uses the column DEFAULT (equivalent to
// TS's explicit toDbDate(Date.now())).
func (r *Repository) LogPrivateMessage(ctx context.Context, from, to uint64, coord int32, message string) error {
	fromID, ok, err := r.accountID(ctx, from)
	if err != nil {
		return fmt.Errorf("LogPrivateMessage: %w", err)
	}
	if !ok {
		return fmt.Errorf("LogPrivateMessage from %d: %w", from, errAccountMissing)
	}
	toID, ok, err := r.accountID(ctx, to)
	if err != nil {
		return fmt.Errorf("LogPrivateMessage: %w", err)
	}
	if !ok {
		return fmt.Errorf("LogPrivateMessage to %d: %w", to, errAccountMissing)
	}
	if _, err := r.db.ExecContext(ctx,
		r.db.Rebind(`INSERT INTO private_chat (account_id, profile, coord, to_account_id, message)
		 VALUES (?, ?, ?, ?, ?)`),
		fromID, r.profile, coord, toID, message,
	); err != nil {
		if gamedb.IsForeignKeyViolation(err) {
			// Endpoint deleted between resolve and insert — same
			// outcome as the TS missing-account throw: drop.
			return fmt.Errorf("LogPrivateMessage: %w", errAccountMissing)
		}
		return fmt.Errorf("LogPrivateMessage: %w", err)
	}
	return nil
}

// LogPublicMessage appends one row to public_chat in the exact TS 274
// shape {session_uuid, timestamp, coord, message}
// (FriendServer.ts:286-297). No profile/world columns — those are
// recovered by joining session on session_uuid, which the central
// database can now do; headless uuids simply have no session row (TS
// accepts those too — deliberately NO FK here). timestamp uses the
// column DEFAULT (TS writes toDbDate(nodeTime); goscape's proto does
// not carry nodeTime — established, accepted deviation).
func (r *Repository) LogPublicMessage(ctx context.Context, sessionUUID string, coord int32, message string) error {
	if _, err := r.db.ExecContext(ctx,
		r.db.Rebind(`INSERT INTO public_chat (session_uuid, coord, message) VALUES (?, ?, ?)`),
		sessionUUID, coord, message,
	); err != nil {
		return fmt.Errorf("LogPublicMessage: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Rewire `modules/friends/friends.go` and config**

Mirror Task 4 exactly: `Friends` struct gets `dbCfg gamedb.Config` and `db *gamedb.DB`; `New(cfg Config, dbCfg gamedb.Config, logger *slog.Logger)`; `starting` opens via `gamedb.Open(f.dbCfg, f.log)` (error text `"open central database: %w"`). Delete `modules/friends/db.go` (including the DB-2 federation doc-comment — its rationale is retired), `modules/friends/db_test.go`, and `modules/friends/migrations/`. Remove `SQLiteDSN` from `modules/friends/config.go` (field, flag, Validate check). Update `cmd/goscape/app/modules.go` `initFriends`:

```go
	f, err := friends.New(g.cfg.Friends, g.cfg.Database, logger)
```

- [ ] **Step 5: Fix the existing friends test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -count=1 2>&1 | head -80`

Mechanical fallout, in order:
1. Package-header comment in `repository.go` still says "persists … to SQLite via *sql.DB" — reword to "persists friend / ignore lists to the central database via *gamedb.DB. The schema lives at pkg/gamedb/migrations/."
2. Every existing test that adds/queries lists for username37s must seed accounts first (`seedAccount(t, db, N)` for each 37 used). Enumerate: `grep -n "AddFriend\|AddIgnore\|GetFriends\|GetIgnores\|GetFollowers\|IsVisibleTo\|LogPrivateMessage" modules/friends/repository_test.go modules/friends/handler_test.go modules/friends/subscriptions_test.go modules/friends/world_subscriptions_test.go`.
3. Tests that pinned the OLD federation behavior (adding friends without accounts persisting; PMs to unknown accounts persisting) now contradict restored TS behavior. Find them: `grep -ln "NAI-S4A-D-FED\|no.account.existence\|no existence check" modules/friends/*_test.go` plus any test failing with "got 0 rows, want 1"-style asserts after seeding was considered. DELETE each and note its name in the commit message — the new dual-pin tests from Step 1 are their replacements.
4. `friends_shutdown_test.go` builds a `friends.Config` with `SQLiteDSN`; replace with a `gamedb.Config` (temp-dir DSN) passed to `friends.New`, pre-migrated:

```go
	dbCfg := testGamedbConfig(t) // helper: defaults + DSN filepath.Join(t.TempDir(), "goscape.db")
	pre, err := gamedb.Open(dbCfg, noopLogger())
	if err != nil { t.Fatalf("pre-open: %v", err) }
	if err := pre.Migrate(t.Context()); err != nil { t.Fatalf("pre-migrate: %v", err) }
	if err := pre.Close(); err != nil { t.Fatalf("pre-close: %v", err) }
	f, err := friends.New(cfg, dbCfg, logger)
```

(Define `testGamedbConfig` once next to `createTestDB`.)

- [ ] **Step 6: Run the suite green**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ ./cmd/... -count=1`
Expected: PASS — except `handler_test.go` PM tests that assert delivery-to-unknown-accounts; those are Task 6's subject. If any remain red, mark them with `t.Skip("Task 6: PM existence-check restoration")` ONLY if they cannot be seeded-and-kept; prefer seeding accounts to keep them meaningful.

- [ ] **Step 7: Commit**

```bash
git add -A modules/friends/ cmd/goscape/app/modules.go
git commit --no-gpg-sign -m "feat(friends): re-key persistence to TS 274 account-id schema via central database

Restores TS behaviors the DB-2 federation blocked: addFriend/addIgnore
account resolution, members-aware 200 cap, TS-shape public_chat.
Deleted federation-era tests: <list names here>"
```

---

### Task 6: Friends handler restorations (PM existence check)

**Files:**
- Modify: `modules/friends/handler.go` (PrivateMessage, PublicMessage)
- Modify: `modules/friends/handler_test.go`

**Interfaces:**
- Consumes: `errAccountMissing`, `LogPrivateMessage`, `LogPublicMessage` (Task 5).
- Produces: final wire behavior — PM to a missing account returns success with no delivery and no row.

- [ ] **Step 1: Write failing handler tests**

Append to `modules/friends/handler_test.go` (adapt the harness construction to the file's existing helpers — it already builds a handler around `newTestRepo`-style plumbing; follow the established pattern there):

```go
func TestPrivateMessage_MissingTarget_DroppedSilently(t *testing.T) {
	// TS FriendServer.ts:270-284: executeTakeFirstOrThrow on either
	// endpoint throws → outer catch → no insert, no delivery, socket
	// stays healthy. goscape: RPC succeeds, no row, no subscriber send.
	h, db := newTestHandler(t) // use the file's existing harness helper name
	seedAccount(t, db, 1)      // sender exists; target 2 does not

	sub := subscribeForTest(t, h, 1, 2) // target's would-be stream, per existing helpers

	_, err := h.PrivateMessage(t.Context(), &friendspb.PrivateMessageRequest{
		WorldId: 1, Username37: 1, TargetUsername37: 2, Coord: 0, Chat: "psst",
	})
	if err != nil {
		t.Fatalf("PrivateMessage: got %v, want nil (silent drop)", err)
	}
	var n int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM private_chat`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("private_chat rows: got %d, want 0", n)
	}
	assertNoDelivery(t, sub) // per existing helpers: nothing on the channel
}

func TestPrivateMessage_BothExist_PersistedAndDelivered(t *testing.T) {
	// Dual-pin (presence side).
	h, db := newTestHandler(t)
	seedAccount(t, db, 1)
	seedAccount(t, db, 2)
	sub := subscribeForTest(t, h, 1, 2)

	_, err := h.PrivateMessage(t.Context(), &friendspb.PrivateMessageRequest{
		WorldId: 1, Username37: 1, TargetUsername37: 2, Coord: 7, Chat: "hello",
	})
	if err != nil {
		t.Fatalf("PrivateMessage: %v", err)
	}
	var n int
	if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM private_chat`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("private_chat rows: got %d, want 1", n)
	}
	assertDelivered(t, sub, "hello")
}
```

The helper names `newTestHandler`, `subscribeForTest`, `assertNoDelivery`, `assertDelivered` are stand-ins for whatever `handler_test.go` already uses to build a handler and observe subscriber sends — reuse those exact existing helpers (grep the file); do NOT invent a parallel harness. If no delivery-observation helper exists, assert via the `subscriptions` registry the file already exercises.

- [ ] **Step 2: Run to verify red**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run 'TestPrivateMessage' -count=1 -v`
Expected: FAIL — missing-target case currently persists and delivers.

- [ ] **Step 3: Restore TS behavior in the handler**

`modules/friends/handler.go` — replace `PrivateMessage`'s doc comment and body. The entire `NAI-S4A-D-FED-NO-ACCOUNT-EXISTENCE-CHECK` block is deleted (its structural rationale — no account table to join — is gone):

```go
// PrivateMessage persists the PM to private_chat (account-id-keyed)
// and routes a PrivateMessageDelivery to the target's open stream.
// Mirrors TS FriendServer.ts:266-284: both endpoint accounts are
// resolved against the central database first; if either is missing
// the PM is dropped silently — no insert, no delivery, successful
// result (TS throws inside the message handler and the outer catch
// swallows it). Other insert failures keep the codes.Internal posture.
//
// req.Coord is server-side-persisted (and otherwise unused for
// routing). req.WorldId is unused for routing because the registry is
// keyed solely by (profile, username37); cross-world routing therefore
// falls out for free.
func (h *handler) PrivateMessage(ctx context.Context, req *friendspb.PrivateMessageRequest) (*emptypb.Empty, error) {
	repo := h.repo()
	h.ensureWorld(req.WorldId)
	if err := repo.LogPrivateMessage(ctx, req.Username37, req.TargetUsername37, req.Coord, req.Chat); err != nil {
		if errors.Is(err, errAccountMissing) {
			return &emptypb.Empty{}, nil
		}
		return nil, status.Errorf(codes.Internal, "LogPrivateMessage: %v", err)
	}
	h.subs.send(h.profile(), req.TargetUsername37, &friendspb.FriendsUpdate{
		Update: &friendspb.FriendsUpdate_PrivateMessage{
			PrivateMessage: &friendspb.PrivateMessageDelivery{
				FromUsername37: req.Username37,
				StaffLvl:       req.StaffLvl,
				PmId:           req.PmId,
				Chat:           req.Chat,
			},
		},
	})
	return &emptypb.Empty{}, nil
}
```

Add `"errors"` to the imports. Update `PublicMessage` for the Task 5 signature (and trim its comment's federation references):

```go
// PublicMessage persists one row to public_chat in the TS 274 shape
// (FriendServer.ts:286-297 — append-only, no delivery, no validation).
// Insert error → codes.Internal (established mutation-handler posture).
func (h *handler) PublicMessage(ctx context.Context, req *friendspb.PublicMessageRequest) (*emptypb.Empty, error) {
	repo := h.repo()
	if err := repo.LogPublicMessage(ctx, req.SessionUuid, req.Coord, req.Chat); err != nil {
		return nil, status.Errorf(codes.Internal, "LogPublicMessage: %v", err)
	}
	return &emptypb.Empty{}, nil
}
```

Also delete the cross-link paragraph referencing the exception block from `LogPrivateMessage`'s old comment if any remnant survived Task 5, and un-skip anything skipped in Task 5 Step 6.

- [ ] **Step 4: Run the full friends suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -count=1`
Expected: PASS, zero skips.

- [ ] **Step 5: Commit**

```bash
git add modules/friends/
git commit --no-gpg-sign -m "feat(friends): restore TS PM account-existence check; retire NAI-S4A-D-FED exception"
```

---

### Task 7: Examples, docs, spec corrections

**Files:**
- Modify: `examples/bundled/goscape.yaml`
- Modify: `examples/full-config-reference.yaml`
- Modify: `CLAUDE.md` (module graph + config notes)
- Modify: `docs/PORTING.md` (retire DB-2 exceptions)
- Modify: `docs/superpowers/specs/2026-07-05-central-db-consolidation-postgres-design.md` (2 corrections)

- [ ] **Step 1: Update `examples/bundled/goscape.yaml`**

Replace the `login:` block's `sqlite_dsn: data/login.db` line (delete it) and add the `database:` section after `target: all`:

```yaml
# The game server is self-contained: login and friends persist to one
# local SQLite central database, so no external services are required.
target: all

database:
  backend: sqlite
  sqlite:
    dsn: data/goscape.db
```

(Also update the comment block at the top as shown.) `friends:` block has no dsn line to remove; `login:` keeps `save_path`, `node_profile`, etc.

- [ ] **Step 2: Update `examples/full-config-reference.yaml`**

Remove `sqlite_dsn` from the `friends:` (line ~191) and `login:` (line ~217) sections. Add a new documented top-level section (alphabetical placement consistent with the file's existing ordering; every key at its default):

```yaml
# Central database shared by the login and friends services. Both are
# independent clients of it — with the sqlite backend they must share a
# filesystem; postgres (Phase 2) lifts that to a network database.
database:
  # Backend: sqlite | postgres.
  backend: sqlite
  sqlite:
    # SQLite file path (parent directories are created automatically).
    dsn: data/goscape.db
  postgres:
    # PostgreSQL DSN, e.g. postgres://user:pass@host:5432/goscape?sslmode=disable
    # Required when backend=postgres. Not yet supported (Phase 2).
    dsn: ""
    # Max open connections per service pool.
    max_open_conns: 8
```

Fix the stale example at line ~30: `sqlite_dsn: ${GOSCAPE_DB:-data/login.db}` → `dsn: ${GOSCAPE_DB:-data/goscape.db}`.

- [ ] **Step 3: Verify both configs boot-parse**

Run:
```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file examples/bundled/goscape.yaml --config.verify=true
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file examples/full-config-reference.yaml --config.verify=true
```
Expected: both exit 0. Also pin the clean break: temporarily add `sqlite_dsn: x` under `login:` in a scratch copy and confirm `--config.verify=true` fails with an unknown-field error naming `sqlite_dsn`.

- [ ] **Step 4: Update `CLAUDE.md`**

In the module-dependency block, add the database row and update world's line context:

```
common    invisible; no deps — exists only to anchor the graph
database  invisible; central-DB migration anchor (pkg/gamedb)  → common
friends   friends server                                       → common, database
login     gRPC login service                                   → common, database
world     TCP game server (world.Server)                       → common, login, friends
ondemand  HTTP OnDemand server (dskit server + OnDemand)       → common, world
all       composite "run everything" target                    → ondemand, friends, login, world
```

Remove "(SQLite)" annotations from friends/login lines; add one sentence to the Configuration section: "The `database:` section selects the central database backend (`sqlite` default; both login and friends are independent clients of one DB)."

- [ ] **Step 5: Update `docs/PORTING.md`**

Find every open DB-2 reference: `grep -n "DB-2\|NAI-S4A-D-FED" docs/PORTING.md`. Add a dated entry in the current arc's section (follow the file's existing entry style):

```
- ✅ **DB-2 federation RETIRED (2026-07-05)** — login+friends now share
  one central database (spec 2026-07-05-central-db-consolidation-postgres);
  friends tables re-keyed to TS 274 account-id shape with FK+CASCADE
  goscape extensions. Restored TS behaviors previously excepted:
  addFriend/addIgnore owner+target account resolution
  (FriendServerRepository.ts:204-296) incl. members-aware 200 cap,
  PM account-existence check (FriendServer.ts:270-284 — the
  NAI-S4A-D-FED-NO-ACCOUNT-EXISTENCE-CHECK exception blocks are
  deleted), TS-shape public_chat {session_uuid, timestamp, coord,
  message} (profile/world recovered via session join). Deliberate
  non-FKs: ignorelist.value (TS allows ignoring nonexistent players),
  public_chat.session_uuid (headless uuids have no session row).
```

Mark the superseded 🚧/ℹ rows (public_chat no-landing-site, PM exception) as retired with a pointer to this entry, per the file's established convention for closed items.

- [ ] **Step 6: Correct the spec (2 items found during planning)**

In `docs/superpowers/specs/2026-07-05-central-db-consolidation-postgres-design.md`:
1. §Behavior restorations item 1: replace "Keeps the 100-cap and `ON CONFLICT DO NOTHING`" with "Cap is members-aware at 274: `account.members ? 200 : 100` (`FriendServerRepository.ts:229`), counted across ALL profiles (TS quirk). `ON CONFLICT DO NOTHING` applies to ignorelist only; friendlist keeps the tx recheck."
2. §Config & deployment: replace "NetworkPolicy gains a postgres egress rule when enabled" with "The chart's NetworkPolicy is ingress-only (`policyTypes: [Ingress]`), so postgres egress is already unrestricted — no NetworkPolicy change needed."

- [ ] **Step 7: Commit**

```bash
git add examples/ CLAUDE.md docs/
git commit --no-gpg-sign -m "docs: central-db config in examples/CLAUDE.md; retire DB-2 in PORTING.md; spec corrections"
```

---

### Task 8: Helm chart — unified config (still SQLite)

**Files:**
- Modify: `production/helm/goscape/templates/_helpers.tpl` (baseConfig)
- Modify: `production/helm/goscape/values.yaml` (comment only)

- [ ] **Step 1: Update `goscape.baseConfig` in `_helpers.tpl`**

Add the `database:` section right after `log_format:` (gated to the modes that run DB-using modules), and delete the two `sqlite_dsn:` lines from the `login:`/`friends:` blocks:

```
target: all
log_level: {{ $g.logLevel | quote }}
log_format: {{ $g.logFormat | quote }}
{{- if or (eq $mode "SingleBinary") (eq $mode "Management") }}
database:
  backend: sqlite
  sqlite:
    dsn: {{ printf "%s/goscape.db" $g.dataPath | quote }}
{{- end }}
```

- [ ] **Step 2: Update `values.yaml` dataPath comment**

`# -- Data dir for stateful modes (login/friends SQLite + player saves)` → `# -- Data dir for stateful modes (central sqlite DB + player saves)`.

- [ ] **Step 3: Render-verify**

Run (for each mode):
```
helm template test production/helm/goscape --set deploymentMode=SingleBinary | grep -A4 "database:"
helm template test production/helm/goscape --set deploymentMode=Management | grep -A4 "database:"
helm template test production/helm/goscape --set deploymentMode=World --set goscape.loginServerAddress=l:2004 --set goscape.friendsServerAddress=f:2005 | grep -c "database:" || true
```
Expected: SingleBinary/Management render the `database:` block with `dsn: "/var/lib/goscape/goscape.db"`; World renders none; NO `sqlite_dsn` anywhere (`helm template ... | grep sqlite_dsn` → empty). If `helm` is unavailable on this box, note that in the commit and rely on Task 9's config.verify instead — do not skip silently.

- [ ] **Step 4: Commit**

```bash
git add production/helm/goscape/
git commit --no-gpg-sign -m "feat(helm): render unified database: section; drop per-module sqlite DSNs"
```

---

### Task 9: Phase 1 verification

- [ ] **Step 1: Full test sweep**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1`
Expected: ALL PASS. Then race-check the touched packages:

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=1 go test -race ./pkg/gamedb/ ./modules/login/ ./modules/friends/ -count=1`
Expected: PASS. (If gcc is missing, report `-race unavailable` explicitly — do not claim it ran.)

- [ ] **Step 2: Compile-all gate + stale-artifact check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./... && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go build -trimpath ./...`
Also confirm no references to the deleted lineages remain: `grep -rn "login.db\|friends.db\|sqlite_dsn\|SQLiteDSN" --include='*.go' --include='*.yaml' --include='*.tpl' . | grep -v docs/ | grep -v '_test.go:.*goscape.db'`
Expected: no hits outside docs/history.

- [ ] **Step 3: Boot smoke (user-launched)**

Per the standing handoff protocol, the smoke server must be launched by the user, not the agent. Report Phase 1 complete and ask the user to run:

```
CGO_ENABLED=0 go run -trimpath ./cmd/goscape --config.file examples/bundled/goscape.yaml
```

and verify: clean boot (database module logs "central database schema up to date" before login/friends), `data/goscape.db` created, client login works, friend add/remove + PM between two accounts work, PM to a nonexistent name is silently dropped.

- [ ] **Step 4: Commit any verification fixes; do NOT start Phase 2 until smoke passes.**

---

# Phase 2 — PostgreSQL backend

### Task 10: gamedb Postgres backend + migrations + gated integration tests

**Files:**
- Modify: `go.mod` (add `github.com/jackc/pgx/v5`)
- Modify: `pkg/gamedb/gamedb.go` (openPostgres)
- Modify: `pkg/gamedb/config.go` (lift Validate restriction)
- Modify: `pkg/gamedb/migrate.go` (postgres driver)
- Create: `pkg/gamedb/migrations/postgres/000001_init.up.sql`
- Create: `pkg/gamedb/postgres_test.go`
- Modify: `pkg/gamedb/config_test.go` (postgres validation cases)

**Interfaces:**
- Consumes: everything from Tasks 1–2.
- Produces: `Open` works for `BackendPostgres`; `Migrate` applies `migrations/postgres/`; test helper `postgresTestDB(t)` (env-gated) for Task 12.

- [ ] **Step 1: Add the dependency**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go get github.com/jackc/pgx/v5@latest && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go mod tidy`
(Module proxy goes through the configured artifactory; if the fetch is blocked, stop and report — do not vendor by hand.)

- [ ] **Step 2: Update config validation + tests**

`config.go` `Validate()` postgres arm becomes:

```go
	case BackendPostgres:
		if c.Postgres.DSN == "" {
			return fmt.Errorf("database: postgres.dsn must be non-empty when database.backend=postgres")
		}
		if c.Postgres.MaxOpenConns < 1 {
			return fmt.Errorf("database: postgres.max_open_conns must be >= 1, got %d", c.Postgres.MaxOpenConns)
		}
```

`config_test.go`: replace the `postgres not yet supported` case with:

```go
		{"postgres without dsn", func(c *Config) { c.Backend = BackendPostgres }, "postgres.dsn"},
		{"postgres with dsn valid", func(c *Config) {
			c.Backend = BackendPostgres
			c.Postgres.DSN = "postgres://u:p@localhost:5432/goscape?sslmode=disable"
		}, ""},
		{"postgres bad pool size", func(c *Config) {
			c.Backend = BackendPostgres
			c.Postgres.DSN = "postgres://u:p@localhost:5432/goscape"
			c.Postgres.MaxOpenConns = 0
		}, "max_open_conns"},
```

- [ ] **Step 3: Implement `openPostgres`**

In `gamedb.go` (imports add `_ "github.com/jackc/pgx/v5/stdlib"`):

```go
func openPostgres(cfg PostgresConfig, logger *slog.Logger) (*DB, error) {
	db, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("gamedb: open postgres: %w", err)
	}
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	logger.Debug("opened central database", "backend", BackendPostgres)
	return &DB{DB: db, dialect: dialectPostgres}, nil
}
```

and the `Open` switch's postgres arm returns `openPostgres(cfg.Postgres, logger)`. Note `sql.Open` doesn't dial — first use (Migrate/query) surfaces connectivity errors, which the database module turns into a fail-fast boot error.

Extend `IsForeignKeyViolation` for pgx (import `"github.com/jackc/pgx/v5/pgconn"`):

```go
func IsForeignKeyViolation(err error) bool {
	if err == nil {
		return false
	}
	// postgres: SQLSTATE 23503 foreign_key_violation.
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code == "23503"
	}
	// modernc.org/sqlite: SQLITE_CONSTRAINT_FOREIGNKEY surfaces as text.
	return strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}
```

and add the postgres side to the gated tests (Step 6): assert `IsForeignKeyViolation` is true for an FK-violating insert in `TestPostgres_MigrateAndCascade` (insert a friendlist row with ids 999999/999998 and check the returned error).

- [ ] **Step 4: Postgres migration driver**

`migrate.go`: remove the phase-1 postgres error; add the driver import `pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"` and branch:

```go
	var (
		drvName string
		drv     database.Driver
	)
	if d.dialect == dialectPostgres {
		drvName = "pgx5"
		drv, err = pgxmigrate.WithInstance(d.DB, &pgxmigrate.Config{})
	} else {
		drvName = "sqlite"
		drv, err = sqlitedriver.WithInstance(d.DB, &sqlitedriver.Config{})
	}
	if err != nil {
		return fmt.Errorf("gamedb: %s driver: %w", drvName, err)
	}
	m, err := migrate.NewWithInstance("iofs", src, drvName, drv)
```

(`database` here is `"github.com/golang-migrate/migrate/v4/database"`. Run `go mod tidy` after — the pgx/v5 migrate driver is part of the existing golang-migrate module.)

- [ ] **Step 5: Write `migrations/postgres/000001_init.up.sql`**

Translate the sqlite DDL 1:1 with these mappings (everything else — table names, column names, constraints, indexes, FK clauses — identical):
- `INTEGER PRIMARY KEY AUTOINCREMENT` → `BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY`
- every other `INTEGER` column that references or is referenced by `account.id` (all `*_account_id`, `account_id`, `closed_by`, `edited_by`, `deleted_by`, `sender_id`, `recipient_id`, `last_message_from`, `from_account_id`, `to_account_id`) → `BIGINT`
- plain small integers (`world`, `uid`, `coord`, `type`, `level`, `node_id`, `logged_in`, `logged_out`, `staff_mod_level`, `members`, `event_type`, `messages`) → `INTEGER`; `value` in hiscore/hiscore_large and `account_value`/`recipient_value` → `BIGINT`
- timestamp-bearing columns (`banned_until`, `muted_until`, `logout_time`, `login_time`, `timestamp`, `date`, `created`, `updated`, `closed`, `marked_spam`, `edited`, `deleted`, `"read"`, `added_on` stays TEXT) → `timestamptz` (nullable ones stay nullable); `DEFAULT CURRENT_TIMESTAMP` → `DEFAULT now()`
- `session.session_uuid` CHECK: `GLOB '????????-????-????-????-????????????'` → `session_uuid ~ '^.{8}-.{4}-.{4}-.{4}-.{12}$'`
- SQLite `TEXT` → `TEXT` (unchanged)

Example of the pattern (first two tables; apply consistently to all):

```sql
CREATE TABLE account (
    id BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    password TEXT NOT NULL,
    registration_ip TEXT NOT NULL DEFAULT '',
    staff_mod_level INTEGER NOT NULL DEFAULT 0,
    members INTEGER NOT NULL DEFAULT 0,
    banned_until timestamptz,
    muted_until timestamptz
);

CREATE TABLE account_login (
    account_id  BIGINT  NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    profile     TEXT    NOT NULL,
    node_id     INTEGER NOT NULL DEFAULT 0,
    logged_in   INTEGER NOT NULL DEFAULT 0,
    logged_out  INTEGER NOT NULL DEFAULT 0,
    logout_time timestamptz,
    PRIMARY KEY (account_id, profile)
);
```

- [ ] **Step 6: Gated integration tests**

`pkg/gamedb/postgres_test.go`:

```go
package gamedb

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"
)

// postgresTestDB opens the env-configured Postgres with a UNIQUE
// throwaway schema per test (search_path isolation) and applies the
// postgres migration lineage. Skips unless GOSCAPE_TEST_POSTGRES_DSN
// is set, e.g.:
//	GOSCAPE_TEST_POSTGRES_DSN='postgres://goscape:goscape@localhost:5432/goscape_test?sslmode=disable'
func postgresTestDB(t *testing.T) *DB {
	t.Helper()
	dsn := os.Getenv("GOSCAPE_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("GOSCAPE_TEST_POSTGRES_DSN not set")
	}
	schema := fmt.Sprintf("t_%x", sha256.Sum256([]byte(t.Name())))[:32]

	cfg := defaultConfig()
	cfg.Backend = BackendPostgres
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	cfg.Postgres.DSN = dsn + sep + "search_path=" + schema

	admin, err := Open(cfg, noopLogger())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := admin.ExecContext(t.Context(), `CREATE SCHEMA IF NOT EXISTS `+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.ExecContext(t.Context(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
		admin.Close()
	})
	if err := admin.Migrate(t.Context()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return admin
}

func TestPostgres_MigrateAndCascade(t *testing.T) {
	db := postgresTestDB(t)
	ctx := t.Context()
	owner := seedAccount(t, db, "owner")
	friend := seedAccount(t, db, "friend")
	if _, err := db.ExecContext(ctx,
		db.Rebind(`INSERT INTO friendlist (profile, account_id, friend_account_id) VALUES ('main', ?, ?)`),
		owner, friend); err != nil {
		t.Fatalf("insert friendlist: %v", err)
	}
	if _, err := db.ExecContext(ctx, db.Rebind(`DELETE FROM account WHERE id = ?`), owner); err != nil {
		t.Fatalf("delete account: %v", err)
	}
	var n int
	if err := db.QueryRowContext(ctx, db.Rebind(`SELECT COUNT(*) FROM friendlist WHERE account_id = ?`), owner).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("cascade: got %d rows, want 0", n)
	}
}

func TestPostgres_OnConflictDoNothing(t *testing.T) {
	db := postgresTestDB(t)
	owner := seedAccount(t, db, "owner")
	q := db.Rebind(`INSERT INTO ignorelist (profile, account_id, value) VALUES ('main', ?, 'ghost') ON CONFLICT DO NOTHING`)
	for range 2 {
		if _, err := db.ExecContext(t.Context(), q, owner); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	var n int
	if err := db.QueryRowContext(t.Context(), db.Rebind(`SELECT COUNT(*) FROM ignorelist WHERE account_id = ?`), owner).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("on conflict: got %d rows, want 1", n)
	}
}

func TestPostgres_TimestampDefaultScansAsTime(t *testing.T) {
	db := postgresTestDB(t)
	owner := seedAccount(t, db, "owner")
	friend := seedAccount(t, db, "friend")
	if _, err := db.ExecContext(t.Context(),
		db.Rebind(`INSERT INTO friendlist (profile, account_id, friend_account_id) VALUES ('main', ?, ?)`),
		owner, friend); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var created sql.NullTime
	if err := db.QueryRowContext(t.Context(),
		db.Rebind(`SELECT created FROM friendlist WHERE account_id = ?`), owner).Scan(&created); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !created.Valid || created.Time.IsZero() {
		t.Errorf("created: got %+v, want valid non-zero time", created)
	}
}
```

Adjust `seedAccount` (Task 2's version takes a username string — matches this usage). Add `"database/sql"` import.

- [ ] **Step 7: Run both modes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamedb/ -count=1` — Expected: PASS, postgres tests SKIP.
If a Postgres instance is available (ask the user; e.g. `podman run --rm -d -p 5432:5432 -e POSTGRES_PASSWORD=goscape -e POSTGRES_DB=goscape_test postgres:17`), run with `GOSCAPE_TEST_POSTGRES_DSN` set — Expected: PASS. Record in the commit message whether the gated suite actually ran.

- [ ] **Step 8: Commit**

```bash
git add go.mod go.sum pkg/gamedb/
git commit --no-gpg-sign -m "feat(gamedb): PostgreSQL backend (pgx/v5) + postgres migration lineage + gated integration tests"
```

---

### Task 11: Login time-handling → `time.Time` (dialect-uniform)

**Files:**
- Modify: `modules/login/db.go`, `modules/login/handler.go`, `modules/login/hiscore.go`
- Modify: `pkg/gamedb/gamedb_test.go` (modernc time round-trip pins)
- Modify: affected `modules/login/*_test.go`

**Why:** Postgres `timestamptz` scans as `time.Time`; the login module currently formats/parses `"2006-01-02 15:04:05"` strings (sqlite TEXT). Uniform contract: Go passes and scans `time.Time` on BOTH dialects.

- [ ] **Step 1: Pin modernc's time behavior first**

Add to `pkg/gamedb/gamedb_test.go`:

```go
func TestSQLite_TimeRoundTrip(t *testing.T) {
	// The login module writes time.Time params and scans sql.NullTime.
	// Pin modernc's storage + parse behavior for both directions, and
	// for legacy "2006-01-02 15:04:05" text (rows written by Phase 1
	// binaries must remain readable).
	db := openTestDB(t)
	if _, err := db.Exec(`CREATE TABLE tt (id INTEGER PRIMARY KEY, at TEXT)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	want := time.Date(2026, 7, 5, 12, 30, 0, 0, time.UTC)
	if _, err := db.Exec(`INSERT INTO tt (id, at) VALUES (1, ?)`, want); err != nil {
		t.Fatalf("insert time.Time: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO tt (id, at) VALUES (2, '2026-07-05 12:30:00')`); err != nil {
		t.Fatalf("insert legacy text: %v", err)
	}
	for id := 1; id <= 2; id++ {
		var got sql.NullTime
		if err := db.QueryRow(`SELECT at FROM tt WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatalf("scan id=%d: %v", id, err)
		}
		if !got.Valid || !got.Time.UTC().Equal(want) {
			t.Errorf("id=%d: got %v, want %v", id, got.Time, want)
		}
	}
}
```

Run it. **If modernc does not round-trip one of the directions**, STOP and record the actual behavior, then adjust the sweep to the workaround the test reveals (e.g. keep writing UTC `time.Time` but scan via a small `nullTime` shim in gamedb that falls back to parsing the known text formats). The test is the decision record either way.

- [ ] **Step 2: Sweep the login module**

Exact sites:
- `modules/login/db.go:23` — delete `const dbTimeFormat`.
- `accountRow` (db.go:25-38): `BannedUntil`, `MutedUntil`, `LogoutTime` become `sql.NullTime`.
- db.go:223 (`insertSessionTx`): `loginTime := time.Now().UTC()` — pass the `time.Time` directly.
- db.go:264 (`setLoggedOut`): same change.
- db.go:306, 317 (`setAccountBanned`/`setAccountMuted`): pass `until` (already `time.Time`) directly, drop `.Format(...)`.
- handler.go:149, 206, 485: replace `time.Parse(dbTimeFormat, account.X.String)` blocks with direct `account.X.Time` use guarded by `account.X.Valid`.
- hiscore.go:41: same pattern.
- hiscore.go:65: `date := now` (pass `time.Time`).
- `insertAccount` (db.go:166): convert `res.LastInsertId()` to `INSERT ... RETURNING id` + `QueryRowContext(...).Scan(&id)` — dialect-portable (SQLite ≥3.35 and Postgres both support RETURNING; sqlite's LastInsertId works but RETURNING is the uniform form).

Update every login test that inserts or asserts formatted time strings (grep `dbTimeFormat\|2006-01-02` in `modules/login/*_test.go`) to use `time.Time` values.

- [ ] **Step 3: Run**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ ./pkg/gamedb/ -count=1`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add modules/login/ pkg/gamedb/
git commit --no-gpg-sign -m "refactor(login): time.Time end-to-end (dialect-uniform); pin modernc time round-trip"
```

---

### Task 12: Module suites against Postgres (env-gated)

**Files:**
- Modify: `modules/friends/repository_test.go` (createTestDB gains postgres mode)
- Modify: `modules/login/db_test.go` (same)

- [ ] **Step 1: Extend the two createTestDB helpers**

Same change in both files:

```go
// createTestDB opens an isolated central test DB: in-memory sqlite by
// default; the env-configured Postgres (unique schema per test, dropped
// on cleanup) when GOSCAPE_TEST_POSTGRES_DSN is set — the whole module
// suite then runs against the real backend.
func createTestDB(t *testing.T) *gamedb.DB {
	t.Helper()
	var cfg gamedb.Config
	fs := flag.NewFlagSet("", flag.PanicOnError)
	cfg.RegisterFlagsAndApplyDefaults(fs)

	if dsn := os.Getenv("GOSCAPE_TEST_POSTGRES_DSN"); dsn != "" {
		schema := fmt.Sprintf("t_%x", sha256.Sum256([]byte(t.Name())))[:32]
		cfg.Backend = gamedb.BackendPostgres
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		cfg.Postgres.DSN = dsn + sep + "search_path=" + schema
		db, err := gamedb.Open(cfg, noopLogger())
		if err != nil {
			t.Fatalf("createTestDB(postgres): %v", err)
		}
		if _, err := db.ExecContext(t.Context(), `CREATE SCHEMA IF NOT EXISTS `+schema); err != nil {
			t.Fatalf("create schema: %v", err)
		}
		t.Cleanup(func() {
			_, _ = db.ExecContext(t.Context(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
			db.Close()
		})
		if err := db.Migrate(t.Context()); err != nil {
			t.Fatalf("createTestDB(postgres): migrate: %v", err)
		}
		return db
	}

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
```

(Consider extracting the shared postgres-schema logic into an exported `gamedb.OpenTestSchema(t, dsn)` helper in a `pkg/gamedb/testutil.go` if duplication across three files bothers the reviewer — acceptable either way; if extracting, move the Task 10 helper onto it too.)

- [ ] **Step 2: Run both modes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ ./modules/friends/ -count=1` — PASS (sqlite).
With a live server: `GOSCAPE_TEST_POSTGRES_DSN=... go test ./modules/login/ ./modules/friends/ -count=1` — PASS. Tests that hardcode sqlite-only SQL (e.g. `sqlite_master` probes) must guard: `if os.Getenv("GOSCAPE_TEST_POSTGRES_DSN") != "" { t.Skip("sqlite-specific") }`. Record in the commit whether the pg run happened.

- [ ] **Step 3: Commit**

```bash
git add modules/ pkg/gamedb/
git commit --no-gpg-sign -m "test: run login/friends suites against Postgres via GOSCAPE_TEST_POSTGRES_DSN"
```

---

### Task 13: Helm — Postgres backend values

**Files:**
- Modify: `production/helm/goscape/values.yaml`
- Modify: `production/helm/goscape/templates/_helpers.tpl`

- [ ] **Step 1: Add values**

In `values.yaml` under the `goscape:` section:

```yaml
  database:
    # -- Central database backend: sqlite | postgres
    backend: sqlite
    postgres:
      # -- PostgreSQL host (required when backend=postgres)
      host: ""
      # -- PostgreSQL port
      port: 5432
      # -- Database name
      database: goscape
      # -- Database user
      user: goscape
      # -- Existing Secret holding the database password
      existingSecret: ""
      # -- Key in existingSecret containing the password
      secretKey: password
      # -- sslmode for the DSN
      sslmode: disable
```

- [ ] **Step 2: Render backend-aware config in `_helpers.tpl`**

Replace Task 8's `database:` block in `goscape.baseConfig`:

```
{{- if or (eq $mode "SingleBinary") (eq $mode "Management") }}
database:
{{- if eq $g.database.backend "postgres" }}
  backend: postgres
  postgres:
    dsn: {{ printf "postgres://%s:${GOSCAPE_DB_PASSWORD}@%s:%d/%s?sslmode=%s" $g.database.postgres.user (required "goscape.database.postgres.host is required when backend=postgres" $g.database.postgres.host) (int $g.database.postgres.port) $g.database.postgres.database $g.database.postgres.sslmode | quote }}
{{- else }}
  backend: sqlite
  sqlite:
    dsn: {{ printf "%s/goscape.db" $g.dataPath | quote }}
{{- end }}
{{- end }}
```

In `goscape.podTemplate` (or wherever the container args/env are defined — locate with `grep -n "args\|env" production/helm/goscape/templates/_helpers.tpl`): when `backend=postgres` AND the mode runs DB modules, append the container arg `--config.expand-env=true` and the env var:

```
{{- if and (eq $g.database.backend "postgres") (or (eq $mode "SingleBinary") (eq $mode "Management")) }}
- name: GOSCAPE_DB_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ required "goscape.database.postgres.existingSecret is required when backend=postgres" $g.database.postgres.existingSecret }}
      key: {{ $g.database.postgres.secretKey }}
{{- end }}
```

Note the config-file env syntax is `${GOSCAPE_DB_PASSWORD}` (expand-env), which is why the DSN template writes it literally.

- [ ] **Step 3: Render-verify**

```
helm template test production/helm/goscape --set deploymentMode=Management \
  --set goscape.database.backend=postgres \
  --set goscape.database.postgres.host=pg.example \
  --set goscape.database.postgres.existingSecret=goscape-db | grep -B2 -A6 "database:\|GOSCAPE_DB_PASSWORD\|expand-env"
```
Expected: postgres DSN with `${GOSCAPE_DB_PASSWORD}` placeholder in the ConfigMap; the secretKeyRef env + `--config.expand-env=true` arg on the container; sqlite default still renders when backend unset; `--set deploymentMode=Management` WITHOUT host fails with the `required` message.

- [ ] **Step 4: Commit**

```bash
git add production/helm/goscape/
git commit --no-gpg-sign -m "feat(helm): postgres central-db backend (secret-based DSN via expand-env)"
```

---

### Task 14: Phase 2 docs + final verification

- [ ] **Step 1: Docs**

- `examples/full-config-reference.yaml`: update the `database.postgres.dsn` comment — remove "Not yet supported (Phase 2)".
- `CLAUDE.md` Configuration section: extend the database sentence: "…(`sqlite` default or `postgres` via `database.backend`; postgres enables running login and friends on different hosts against one network central DB)."
- `docs/PORTING.md`: append to the Task 7 entry: "Phase 2 (postgres backend via pgx/v5, timestamptz schema, gated pg test suites, Helm postgres values) landed <date>."

- [ ] **Step 2: Full verification**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=1 go test -race ./pkg/gamedb/ ./modules/login/ ./modules/friends/ -count=1
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go build -trimpath ./...
```
All green. If a Postgres server is available, run the gated suites and a full boot smoke with `database.backend: postgres` (user-launched, both `--target all` and split `--target login` / `--target friends` processes against one DB). Report honestly which verifications ran.

- [ ] **Step 3: Commit**

```bash
git add examples/ CLAUDE.md docs/
git commit --no-gpg-sign -m "docs: postgres backend documented; phase 2 complete"
```

---

## Plan Self-Review Notes (resolved during writing)

- TS 274 `addFriend` cap is members-aware (200) — spec §5.1 corrected in Task 7.
- Chart NetworkPolicy is ingress-only — no egress work needed; spec corrected in Task 7.
- `public_chat` at 274 is `session_uuid`-keyed with NO profile/world (unlike the 244/254-era notes) — DDL and `LogPublicMessage` reflect the 274 shape.
- `ensureDBParentDir` gains a `mode=memory` guard (test DSNs); pinned by `openTestDB` usage.
- `ignorelist` cap check has no dup-branch (ON CONFLICT absorbs it) — ordering matches TS `:264-272`.
