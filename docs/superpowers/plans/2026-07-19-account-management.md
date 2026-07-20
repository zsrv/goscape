# Account Management System & Player Portal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the account-management system from `docs/superpowers/specs/2026-07-19-account-management-design.md`: a new `modules/account` (SSR portal + gRPC AccountService), central-DB migration `000003`, `login.auth_mode` delegation, and `goscape-cli account` admin verbs.

**Architecture:** One new dskit module `account` (deps: common, database) running two listeners — an `html/template` SSR portal on `net/http` and a gRPC `AccountService`. The login module gains `auth_mode: local|account`; in `account` mode it delegates password verification to `VerifyGameLogin` over gRPC. All portal data lives in new `portal_*` tables in the central DB (SQLite + Postgres). Zero new Go module dependencies: argon2id from the existing `golang.org/x/crypto`, hand-rolled OAuth2 code flow + token-bucket rate limiter, `net/smtp` for email.

**Tech Stack:** Go 1.26, `pkg/gamedb` (golang-migrate lineage), `pkg/dskit/services.BasicService`, grpc + protoc-gen-go via `make protos` (buf), `html/template` + `embed`, `x/crypto/argon2`, `net/smtp`.

## Global Constraints

- Go 1.26: use modern idioms (`any`, `min`/`max`, `for i := range n`, Go 1.22 `http.ServeMux` method patterns + `r.PathValue`, `t.Context()` in tests, `errors.AsType`, `slices`/`maps`/`cmp` packages).
- Every `go` invocation MUST be prefixed: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`
- Every commit: `git commit --no-gpg-sign`, message trailer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`. Stage ONLY the files named in the task (repo root has unrelated untracked noise — never `git add -A`).
- NO new entries in `go.mod`. argon2 comes from the already-present `golang.org/x/crypto`; OAuth2 and rate limiting are hand-rolled; email uses `net/smtp`.
- `login.auth_mode=local` (the default) must be byte-for-byte behavior-identical to today — existing login tests must pass unmodified.
- Config decoding is strict (unknown YAML key = fatal); every new key needs a struct field. Lists/secrets are YAML-only (no CLI flag) following the `ondemand.pub_pem` precedent; scalars get flags.
- Error self-prefix convention: `account:` prefix on `modules/account` Validate errors (matches login/friends, consumed unwrapped by `cmd/goscape/app.Config.Validate`).
- Timestamps: write `time.Now().UTC()`; SQLite columns MUST use `DATETIME` decltype (modernc scan trap, see header comment in `000001_init.up.sql`); Postgres uses `timestamptz`. Boolean-ish columns are `INTEGER` 0/1 in BOTH dialects (matches `account.members`) so Go scanning is uniform.
- SQL placeholders: write `?` and run through `db.Rebind(...)` for cross-dialect (see `modules/friends/repository.go` usage of `gamedb.DB`).
- Spec deviation (documented): the portal uses a module-managed `net/http.Server` instead of `pkg/dskit/server` — dskit server's flag namespace is single-instance-per-process (ondemand owns it). The module still binds listeners in `starting()` and drains in `stopping()`, preserving dskit lifecycle semantics.
- Spec deviation (documented): no `smtp.starttls` config key. `net/smtp.SendMail` negotiates STARTTLS automatically whenever the relay advertises it and cannot be told not to — a dead knob would misleadingly imply control. The spec's config listing has been amended to match.
- Test DB pattern: SQLite via `t.TempDir()` file DSN + `db.Migrate`; Postgres via `pkg/gamedb/gamedbtest.OpenTestSchema` gated on a DSN env/flag exactly like `modules/login/db_test.go` does. NEVER use `t.Context()` inside `t.Cleanup` bodies.
- Constants used across tasks (defined in Task 2 unless noted): `StatusActive = "active"`, `StatusDisabled = "disabled"`, `GroupManuallyApproved = "manually_approved"`, `GroupAdmin = "admin"`, `SentinelGamePassword = "!portal-managed!"` (Task 3), `TokenPurposeVerifyEmail = "verify_email"`, `TokenPurposeResetPassword = "reset_password"` (Task 6).

## File Map

| Path | Responsibility |
|---|---|
| `pkg/gamedb/migrations/{sqlite,postgres}/000003_portal_accounts.up.sql` | All `portal_*` tables + seeded groups |
| `modules/account/config.go` | `Config` + nested Gate/Argon2/Session/SMTP/Providers configs, flags, Validate |
| `modules/account/password.go` | argon2id PHC hash/verify, portal password policy, game-password sentinel |
| `modules/account/store.go` | `Store` — accounts, groups, audit (part 1) |
| `modules/account/store_identity.go` | identities (link/revoke/release), characters (+game-account tx), gate query |
| `modules/account/store_session.go` | portal sessions + email tokens |
| `proto/account/account.proto` → `pkg/accountpb/` | AccountService wire contract (generated via `make protos`) |
| `modules/account/grpc.go` | gRPC server: VerifyGameLogin, admin RPCs, bearer-token interceptor |
| `modules/account/account.go` | Module: `New`, starting/running/stopping (DB pool + both listeners) |
| `modules/account/portal.go` | portal struct, route table, template render helpers |
| `modules/account/middleware.go` | session load, CSRF, rate limiter, requireAuth/requireAdmin |
| `modules/account/handlers_auth.go` | register, login, logout, verify-email, forgot/reset password |
| `modules/account/handlers_app.go` | dashboard, character creation, settings, Discord link + callback |
| `modules/account/handlers_admin.go` | /admin search, account detail + actions, audit view |
| `modules/account/mail.go` | `Mailer` interface + `net/smtp` impl + templates |
| `modules/account/oauth.go` | hand-rolled Discord OAuth2 code flow client |
| `modules/account/templates/*.html`, `static/style.css` | embedded SSR assets |
| `modules/login/config.go`, `handler.go`, `db.go`, `login.go` | auth_mode knob, account client, delegation, accountByID |
| `cmd/goscape/app/{modules,config}.go` | register module, deps, validate fan-out |
| `cmd/goscape-cli/cmd_account.go` | admin CLI verb group |
| `examples/full-config-reference.yaml`, `CLAUDE.md` | document every new key + module |

---

### Task 1: Central-DB migration 000003 — portal tables

**Files:**
- Create: `pkg/gamedb/migrations/sqlite/000003_portal_accounts.up.sql`
- Create: `pkg/gamedb/migrations/postgres/000003_portal_accounts.up.sql`
- Test: `pkg/gamedb/migrate_portal_test.go`

**Interfaces:**
- Consumes: existing migration lineage (`000001`, `000002`), `gamedb.Open`, `DB.Migrate`.
- Produces: tables `portal_account`, `portal_identity`, `portal_character`, `portal_group` (seeded `manually_approved`, `admin`), `portal_group_member`, `portal_session`, `portal_token`, `portal_audit_log`. Every later task's SQL targets exactly these columns.

- [ ] **Step 1: Write the failing test**

```go
// pkg/gamedb/migrate_portal_test.go
package gamedb_test

import (
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/gamedb"
)

// openMigratedSQLite mirrors the existing migrate_test.go helper style:
// fresh file DB in t.TempDir(), full lineage applied.
func openMigratedSQLite(t *testing.T) *gamedb.DB {
	t.Helper()
	var cfg gamedb.Config
	cfg.Backend = gamedb.BackendSQLite
	cfg.SQLite.DSN = filepath.Join(t.TempDir(), "test.db")
	db, err := gamedb.Open(cfg, slog.Default())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(t.Context()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestMigration000003_PortalTables(t *testing.T) {
	db := openMigratedSQLite(t)
	ctx := t.Context()

	for _, table := range []string{
		"portal_account", "portal_identity", "portal_character",
		"portal_group", "portal_group_member", "portal_session",
		"portal_token", "portal_audit_log",
	} {
		if _, err := db.ExecContext(ctx, "SELECT * FROM "+table+" WHERE 1=0"); err != nil {
			t.Errorf("table %s missing: %v", table, err)
		}
	}

	// Seeded groups.
	var n int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM portal_group WHERE name IN ('manually_approved', 'admin')`).Scan(&n)
	if err != nil || n != 2 {
		t.Fatalf("seeded groups: n=%d err=%v", n, err)
	}

	// One third-party identity can vouch for at most one portal account.
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, db.Rebind(q), args...); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	mustExec(`INSERT INTO portal_account (email, email_verified, password_hash, status, created_at, updated_at)
	          VALUES ('a@example.com', 1, 'x', 'active', '2026-07-19 00:00:00', '2026-07-19 00:00:00')`)
	mustExec(`INSERT INTO portal_account (email, email_verified, password_hash, status, created_at, updated_at)
	          VALUES ('b@example.com', 1, 'x', 'active', '2026-07-19 00:00:00', '2026-07-19 00:00:00')`)
	mustExec(`INSERT INTO portal_identity (account_id, provider, provider_user_id, provider_username, linked_at)
	          VALUES (1, 'discord', 'D1', 'alice', '2026-07-19 00:00:00')`)
	if _, err := db.ExecContext(ctx, db.Rebind(
		`INSERT INTO portal_identity (account_id, provider, provider_user_id, provider_username, linked_at)
		 VALUES (?, 'discord', 'D1', 'mallory', '2026-07-19 00:00:00')`), 2); err == nil {
		t.Fatal("duplicate (provider, provider_user_id) must violate UNIQUE")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamedb/ -run TestMigration000003 -v`
Expected: FAIL — `table portal_account missing` (migration file does not exist yet).

- [ ] **Step 3: Write the SQLite migration**

```sql
-- pkg/gamedb/migrations/sqlite/000003_portal_accounts.up.sql
-- Account-management system (portal): accounts as containers for game
-- characters, third-party identity links, admin groups, web sessions.
-- Spec: docs/superpowers/specs/2026-07-19-account-management-design.md
--
-- The existing `account` table remains per-character game state; these
-- tables are the identity layer above it. Boolean-ish columns are
-- INTEGER 0/1 (matches account.members) so Go scanning is uniform
-- across dialects. DATETIME decltype is load-bearing (see 000001).

CREATE TABLE portal_account (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    email TEXT NOT NULL UNIQUE,
    email_verified INTEGER NOT NULL DEFAULT 0,
    password_hash TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE TABLE portal_identity (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES portal_account(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    provider_user_id TEXT NOT NULL,
    provider_username TEXT NOT NULL DEFAULT '',
    linked_at DATETIME NOT NULL,
    -- Soft "burn": a revoked identity still occupies the UNIQUE below,
    -- so one Discord identity can never vouch for a second account.
    revoked_at DATETIME,
    UNIQUE (provider, provider_user_id),
    UNIQUE (account_id, provider)
);

CREATE TABLE portal_character (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES portal_account(id) ON DELETE CASCADE,
    username TEXT NOT NULL UNIQUE,
    game_account_id INTEGER NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    created_at DATETIME NOT NULL
);
CREATE INDEX idx_portal_character_account ON portal_character (account_id);

CREATE TABLE portal_group (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT ''
);
INSERT INTO portal_group (name, description) VALUES
    ('manually_approved', 'Bypasses the linked-identity character-creation gate.'),
    ('admin', 'Grants access to portal /admin pages and admin actions.');

CREATE TABLE portal_group_member (
    group_id INTEGER NOT NULL REFERENCES portal_group(id) ON DELETE CASCADE,
    account_id INTEGER NOT NULL REFERENCES portal_account(id) ON DELETE CASCADE,
    added_by INTEGER REFERENCES portal_account(id) ON DELETE SET NULL,
    added_at DATETIME NOT NULL,
    PRIMARY KEY (group_id, account_id)
);

CREATE TABLE portal_session (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    token_hash TEXT NOT NULL UNIQUE,
    account_id INTEGER NOT NULL REFERENCES portal_account(id) ON DELETE CASCADE,
    created_at DATETIME NOT NULL,
    expires_at DATETIME NOT NULL,
    ip TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_portal_session_account ON portal_session (account_id);

CREATE TABLE portal_token (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES portal_account(id) ON DELETE CASCADE,
    purpose TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at DATETIME NOT NULL,
    used_at DATETIME,
    created_at DATETIME NOT NULL
);
CREATE INDEX idx_portal_token_account ON portal_token (account_id, purpose);

CREATE TABLE portal_audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_account_id INTEGER REFERENCES portal_account(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    target TEXT NOT NULL DEFAULT '',
    details TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL
);
CREATE INDEX idx_portal_audit_created ON portal_audit_log (created_at);
```

- [ ] **Step 4: Write the Postgres migration**

Identical shape; dialect swaps only (`BIGINT GENERATED ALWAYS AS IDENTITY` PKs, `timestamptz`, same INTEGER 0/1 booleans):

```sql
-- pkg/gamedb/migrations/postgres/000003_portal_accounts.up.sql
-- Account-management system (portal). Mirror of the sqlite file; see
-- that file's header for design notes.
-- Spec: docs/superpowers/specs/2026-07-19-account-management-design.md

CREATE TABLE portal_account (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    email_verified INTEGER NOT NULL DEFAULT 0,
    password_hash TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE TABLE portal_identity (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES portal_account(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    provider_user_id TEXT NOT NULL,
    provider_username TEXT NOT NULL DEFAULT '',
    linked_at timestamptz NOT NULL,
    revoked_at timestamptz,
    UNIQUE (provider, provider_user_id),
    UNIQUE (account_id, provider)
);

CREATE TABLE portal_character (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES portal_account(id) ON DELETE CASCADE,
    username TEXT NOT NULL UNIQUE,
    game_account_id BIGINT NOT NULL REFERENCES account(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL
);
CREATE INDEX idx_portal_character_account ON portal_character (account_id);

CREATE TABLE portal_group (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT ''
);
INSERT INTO portal_group (name, description) VALUES
    ('manually_approved', 'Bypasses the linked-identity character-creation gate.'),
    ('admin', 'Grants access to portal /admin pages and admin actions.');

CREATE TABLE portal_group_member (
    group_id BIGINT NOT NULL REFERENCES portal_group(id) ON DELETE CASCADE,
    account_id BIGINT NOT NULL REFERENCES portal_account(id) ON DELETE CASCADE,
    added_by BIGINT REFERENCES portal_account(id) ON DELETE SET NULL,
    added_at timestamptz NOT NULL,
    PRIMARY KEY (group_id, account_id)
);

CREATE TABLE portal_session (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    account_id BIGINT NOT NULL REFERENCES portal_account(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    ip TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_portal_session_account ON portal_session (account_id);

CREATE TABLE portal_token (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES portal_account(id) ON DELETE CASCADE,
    purpose TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL
);
CREATE INDEX idx_portal_token_account ON portal_token (account_id, purpose);

CREATE TABLE portal_audit_log (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_account_id BIGINT REFERENCES portal_account(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    target TEXT NOT NULL DEFAULT '',
    details TEXT NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL
);
CREATE INDEX idx_portal_audit_created ON portal_audit_log (created_at);
```

- [ ] **Step 5: Run tests to verify they pass (and nothing regressed)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamedb/... -v -run 'Migration|Migrate'`
Expected: PASS including `TestMigration000003_PortalTables`. If a Postgres DSN is configured in the environment the existing postgres migrate tests must also pass.

- [ ] **Step 6: Commit**

```bash
git add pkg/gamedb/migrations/sqlite/000003_portal_accounts.up.sql \
        pkg/gamedb/migrations/postgres/000003_portal_accounts.up.sql \
        pkg/gamedb/migrate_portal_test.go
git commit --no-gpg-sign -m "feat(gamedb): migration 000003 — portal account tables

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: `modules/account` Config

**Files:**
- Create: `modules/account/config.go`
- Test: `modules/account/config_test.go`

**Interfaces:**
- Consumes: `pkg/util/log.Level` (per-module log level pattern from `modules/login/config.go`).
- Produces (used by every later task):

```go
package account

const (
	StatusActive           = "active"
	StatusDisabled         = "disabled"
	GroupManuallyApproved  = "manually_approved"
	GroupAdmin             = "admin"
)

type Config struct {
	LogLevel          *log.Level      `yaml:"log_level"`
	Enable            bool            `yaml:"enable"`
	HTTPListenAddress string          `yaml:"http_listen_address"` // default 127.0.0.1
	HTTPListenPort    int             `yaml:"http_listen_port"`    // default 8081
	GRPCListenAddress string          `yaml:"grpc_listen_address"` // default 127.0.0.1
	GRPCListenPort    int             `yaml:"grpc_listen_port"`    // default 2005
	PublicURL         string          `yaml:"public_url"`          // e.g. https://account.example.com — no trailing slash
	CharacterLimit    int             `yaml:"character_limit"`     // default 5
	AdminToken        string          `yaml:"admin_token"`         // YAML-only; empty = admin RPCs disabled
	Gate              GateConfig      `yaml:"gate"`
	Argon2            Argon2Config    `yaml:"argon2"`
	Session           SessionConfig   `yaml:"session"`
	SMTP              SMTPConfig      `yaml:"smtp"`
	Providers         ProvidersConfig `yaml:"providers"`
}

type GateConfig struct {
	// Providers whose non-revoked link satisfies the character-creation
	// gate. YAML-only. Empty slice = only manually_approved gates.
	Providers []string `yaml:"providers"`
}

type Argon2Config struct {
	MemoryKiB   int `yaml:"memory_kib"`  // default 65536
	Time        int `yaml:"time"`        // default 2
	Parallelism int `yaml:"parallelism"` // default 1
}

type SessionConfig struct {
	IdleTTL     time.Duration `yaml:"idle_ttl"`     // default 168h
	AbsoluteTTL time.Duration `yaml:"absolute_ttl"` // default 720h
}

type SMTPConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"` // default 587
	From     string `yaml:"from"`
	Username string `yaml:"username"` // YAML-only
	Password string `yaml:"password"` // YAML-only
}

type ProvidersConfig struct {
	Discord DiscordConfig `yaml:"discord"`
}

type DiscordConfig struct {
	ClientID     string `yaml:"client_id"`     // YAML-only
	ClientSecret string `yaml:"client_secret"` // YAML-only
	// Endpoint overrides for tests; empty = official Discord endpoints.
	AuthURL  string `yaml:"auth_url,omitempty"`
	TokenURL string `yaml:"token_url,omitempty"`
	APIBase  string `yaml:"api_base,omitempty"`
}

func (c *Config) RegisterFlagsAndApplyDefaults(f *flag.FlagSet)
func (c *Config) Validate() error
```

- [ ] **Step 1: Write the failing test**

```go
// modules/account/config_test.go
package account

import (
	"flag"
	"strings"
	"testing"
	"time"
)

func defaultConfig(t *testing.T) Config {
	t.Helper()
	var c Config
	fs := flag.NewFlagSet("", flag.PanicOnError)
	c.RegisterFlagsAndApplyDefaults(fs)
	return c
}

func TestConfig_Defaults(t *testing.T) {
	c := defaultConfig(t)
	if c.Enable {
		t.Error("Enable must default false")
	}
	if c.HTTPListenPort != 8081 || c.GRPCListenPort != 2005 {
		t.Errorf("ports: http=%d grpc=%d", c.HTTPListenPort, c.GRPCListenPort)
	}
	if c.CharacterLimit != 5 {
		t.Errorf("CharacterLimit = %d, want 5", c.CharacterLimit)
	}
	if c.Argon2.MemoryKiB != 65536 || c.Argon2.Time != 2 || c.Argon2.Parallelism != 1 {
		t.Errorf("argon2 defaults: %+v", c.Argon2)
	}
	if c.Session.IdleTTL != 168*time.Hour || c.Session.AbsoluteTTL != 720*time.Hour {
		t.Errorf("session defaults: %+v", c.Session)
	}
	if got := c.Gate.Providers; len(got) != 1 || got[0] != "discord" {
		t.Errorf("Gate.Providers = %v, want [discord]", got)
	}
	if c.SMTP.Port != 587 {
		t.Errorf("SMTP.Port = %d, want 587", c.SMTP.Port)
	}
}

func TestConfig_ValidateDisabledIsAlwaysOK(t *testing.T) {
	c := defaultConfig(t)
	c.Enable = false
	c.HTTPListenPort = -1 // nonsense values must not matter when disabled
	if err := c.Validate(); err != nil {
		t.Fatalf("disabled module must validate clean, got %v", err)
	}
}

func TestConfig_ValidateEnabled(t *testing.T) {
	base := func() Config {
		c := defaultConfig(t)
		c.Enable = true
		c.PublicURL = "http://127.0.0.1:8081"
		return c
	}
	if err := base().Validate(); err != nil {
		t.Fatalf("valid enabled config rejected: %v", err)
	}

	cases := []struct {
		name    string
		mutate  func(*Config)
		wantSub string
	}{
		{"bad http port", func(c *Config) { c.HTTPListenPort = 0 }, "http_listen_port"},
		{"bad grpc port", func(c *Config) { c.GRPCListenPort = 70000 }, "grpc_listen_port"},
		{"missing public url", func(c *Config) { c.PublicURL = "" }, "public_url"},
		{"trailing slash", func(c *Config) { c.PublicURL = "http://x/" }, "public_url"},
		{"bad limit", func(c *Config) { c.CharacterLimit = 0 }, "character_limit"},
		{"bad argon2 memory", func(c *Config) { c.Argon2.MemoryKiB = 1024 }, "argon2"},
		{"bad argon2 time", func(c *Config) { c.Argon2.Time = 0 }, "argon2"},
		{"bad idle ttl", func(c *Config) { c.Session.IdleTTL = 0 }, "session"},
		{"idle > absolute", func(c *Config) { c.Session.IdleTTL = 1000 * time.Hour }, "session"},
		{"unknown gate provider", func(c *Config) { c.Gate.Providers = []string{"myspace"} }, "gate.providers"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base()
			tc.mutate(&c)
			err := c.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("want error containing %q, got %v", tc.wantSub, err)
			}
			if err != nil && !strings.HasPrefix(err.Error(), "account: ") {
				t.Fatalf("errors must self-prefix 'account: ', got %v", err)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -v`
Expected: FAIL to build — package does not exist yet.

- [ ] **Step 3: Write config.go**

```go
// modules/account/config.go
package account

import (
	"flag"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/zsrv/goscape/pkg/util/log"
)

// Account status values (portal_account.status).
const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
)

// Seeded portal_group names (migration 000003).
const (
	GroupManuallyApproved = "manually_approved"
	GroupAdmin            = "admin"
)

// knownProviders is the closed set of third-party providers this build
// implements. gate.providers entries must come from it.
var knownProviders = []string{"discord"}

type Config struct {
	LogLevel          *log.Level      `yaml:"log_level"` // optional per-module override; nil = inherit global
	Enable            bool            `yaml:"enable"`
	HTTPListenAddress string          `yaml:"http_listen_address"`
	HTTPListenPort    int             `yaml:"http_listen_port"`
	GRPCListenAddress string          `yaml:"grpc_listen_address"`
	GRPCListenPort    int             `yaml:"grpc_listen_port"`
	// PublicURL is the externally-reachable base URL of the portal, used
	// in email links and the OAuth redirect URI. No trailing slash.
	PublicURL      string          `yaml:"public_url"`
	CharacterLimit int             `yaml:"character_limit"`
	// AdminToken guards the admin gRPC surface (bearer token in
	// metadata). Empty disables every admin RPC. YAML-only (secret).
	AdminToken string          `yaml:"admin_token"`
	Gate       GateConfig      `yaml:"gate"`
	Argon2     Argon2Config    `yaml:"argon2"`
	Session    SessionConfig   `yaml:"session"`
	SMTP       SMTPConfig      `yaml:"smtp"`
	Providers  ProvidersConfig `yaml:"providers"`
}

// GateConfig controls the character-creation gate. An account may create
// characters iff it is active, email-verified, under the character
// limit, AND (member of manually_approved OR holds a non-revoked
// identity whose provider is listed here). Empty = manual approval only.
type GateConfig struct {
	Providers []string `yaml:"providers"` // YAML-only (list)
}

// Argon2Config parameterizes argon2id password hashing (RFC 9106).
type Argon2Config struct {
	MemoryKiB   int `yaml:"memory_kib"`
	Time        int `yaml:"time"`
	Parallelism int `yaml:"parallelism"`
}

// SessionConfig bounds portal cookie sessions: a session expires
// IdleTTL after last use, and unconditionally AbsoluteTTL after login.
type SessionConfig struct {
	IdleTTL     time.Duration `yaml:"idle_ttl"`
	AbsoluteTTL time.Duration `yaml:"absolute_ttl"`
}

// SMTPConfig configures the mailer. net/smtp negotiates STARTTLS
// automatically when the server advertises it.
type SMTPConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	From     string `yaml:"from"`
	Username string `yaml:"username"` // YAML-only (secret)
	Password string `yaml:"password"` // YAML-only (secret)
}

type ProvidersConfig struct {
	Discord DiscordConfig `yaml:"discord"`
}

// DiscordConfig holds the OAuth2 app credentials. The URL overrides
// exist for tests (point at an httptest server); empty means the
// official Discord endpoints (see oauth.go).
type DiscordConfig struct {
	ClientID     string `yaml:"client_id"`     // YAML-only (secret-adjacent)
	ClientSecret string `yaml:"client_secret"` // YAML-only (secret)
	AuthURL      string `yaml:"auth_url,omitempty"`
	TokenURL     string `yaml:"token_url,omitempty"`
	APIBase      string `yaml:"api_base,omitempty"`
}

func (c *Config) RegisterFlagsAndApplyDefaults(f *flag.FlagSet) {
	f.BoolVar(&c.Enable, "account.enable", false, "Whether to run the account module.")
	f.StringVar(&c.HTTPListenAddress, "account.http-listen-address", "127.0.0.1", "Portal HTTP listen address.")
	f.IntVar(&c.HTTPListenPort, "account.http-listen-port", 8081, "Portal HTTP listen port.")
	f.StringVar(&c.GRPCListenAddress, "account.grpc-listen-address", "127.0.0.1", "AccountService gRPC listen address.")
	f.IntVar(&c.GRPCListenPort, "account.grpc-listen-port", 2005, "AccountService gRPC listen port.")
	f.StringVar(&c.PublicURL, "account.public-url", "", "Externally reachable portal base URL (email links, OAuth redirect). No trailing slash.")
	f.IntVar(&c.CharacterLimit, "account.character-limit", 5, "Maximum characters per portal account.")
	f.IntVar(&c.Argon2.MemoryKiB, "account.argon2-memory-kib", 65536, "argon2id memory cost in KiB.")
	f.IntVar(&c.Argon2.Time, "account.argon2-time", 2, "argon2id time cost (passes).")
	f.IntVar(&c.Argon2.Parallelism, "account.argon2-parallelism", 1, "argon2id parallelism (lanes).")
	f.DurationVar(&c.Session.IdleTTL, "account.session-idle-ttl", 168*time.Hour, "Portal session idle expiry.")
	f.DurationVar(&c.Session.AbsoluteTTL, "account.session-absolute-ttl", 720*time.Hour, "Portal session absolute expiry.")
	f.StringVar(&c.SMTP.Host, "account.smtp-host", "", "SMTP relay host for verification/reset email. Empty disables outbound mail.")
	f.IntVar(&c.SMTP.Port, "account.smtp-port", 587, "SMTP relay port.")
	f.StringVar(&c.SMTP.From, "account.smtp-from", "", "From address for portal email.")

	// YAML-only (no flags): admin_token, gate.providers, smtp
	// credentials, providers.discord.* — lists and secrets follow the
	// ondemand.pub_pem precedent.
	c.Gate.Providers = []string{"discord"}
}

// Validate enforces runtime invariants; errors self-prefix "account: "
// (consumed unwrapped by cmd/goscape/app Config.Validate). Disabled
// module short-circuits.
func (c *Config) Validate() error {
	if !c.Enable {
		return nil
	}
	if c.HTTPListenPort < 1 || c.HTTPListenPort > 65535 {
		return fmt.Errorf("account: http_listen_port must be in [1, 65535], got %d", c.HTTPListenPort)
	}
	if c.GRPCListenPort < 1 || c.GRPCListenPort > 65535 {
		return fmt.Errorf("account: grpc_listen_port must be in [1, 65535], got %d", c.GRPCListenPort)
	}
	if c.PublicURL == "" {
		return fmt.Errorf("account: public_url must be non-empty when account.enable=true")
	}
	if strings.HasSuffix(c.PublicURL, "/") {
		return fmt.Errorf("account: public_url must not end with a trailing slash, got %q", c.PublicURL)
	}
	if c.CharacterLimit < 1 {
		return fmt.Errorf("account: character_limit must be >= 1, got %d", c.CharacterLimit)
	}
	if c.Argon2.MemoryKiB < 8*1024 {
		return fmt.Errorf("account: argon2.memory_kib must be >= 8192, got %d", c.Argon2.MemoryKiB)
	}
	if c.Argon2.Time < 1 {
		return fmt.Errorf("account: argon2.time must be >= 1, got %d", c.Argon2.Time)
	}
	if c.Argon2.Parallelism < 1 {
		return fmt.Errorf("account: argon2.parallelism must be >= 1, got %d", c.Argon2.Parallelism)
	}
	if c.Session.IdleTTL <= 0 || c.Session.AbsoluteTTL <= 0 {
		return fmt.Errorf("account: session TTLs must be > 0, got idle=%v absolute=%v", c.Session.IdleTTL, c.Session.AbsoluteTTL)
	}
	if c.Session.IdleTTL > c.Session.AbsoluteTTL {
		return fmt.Errorf("account: session idle_ttl (%v) must be <= absolute_ttl (%v)", c.Session.IdleTTL, c.Session.AbsoluteTTL)
	}
	for _, p := range c.Gate.Providers {
		if !slices.Contains(knownProviders, p) {
			return fmt.Errorf("account: gate.providers entry %q is not a known provider %v", p, knownProviders)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -v`
Expected: PASS (all three test functions).

- [ ] **Step 5: Commit**

```bash
git add modules/account/config.go modules/account/config_test.go
git commit --no-gpg-sign -m "feat(account): module config with gate/argon2/session/smtp/provider sections

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: argon2id password hashing + policy

**Files:**
- Create: `modules/account/password.go`
- Test: `modules/account/password_test.go`

**Interfaces:**
- Consumes: `Argon2Config` (Task 2), `golang.org/x/crypto/argon2` (existing dependency).
- Produces:

```go
const SentinelGamePassword = "!portal-managed!" // stored in game account.password rows created by the portal

func HashPassword(password string, p Argon2Config) (string, error) // PHC string $argon2id$v=19$m=..,t=..,p=..$salt$key
func VerifyPassword(password, phc string) (bool, error)            // constant-time compare; false+nil on mismatch, err on malformed PHC
func ValidPortalPassword(password string) error                    // 8..20 chars, printable ASCII 0x21..0x7E (client-typable); nil = OK
```

- [ ] **Step 1: Write the failing test**

```go
// modules/account/password_test.go
package account

import (
	"strings"
	"testing"
)

func testArgon2() Argon2Config {
	// Small params for test speed; production defaults live in config.
	return Argon2Config{MemoryKiB: 8 * 1024, Time: 1, Parallelism: 1}
}

func TestHashVerifyRoundTrip(t *testing.T) {
	phc, err := HashPassword("Sw0rdfish!", testArgon2())
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(phc, "$argon2id$v=19$") {
		t.Fatalf("not a PHC argon2id string: %q", phc)
	}
	ok, err := VerifyPassword("Sw0rdfish!", phc)
	if err != nil || !ok {
		t.Fatalf("verify correct password: ok=%v err=%v", ok, err)
	}
	ok, err = VerifyPassword("sw0rdfish!", phc) // case matters
	if err != nil || ok {
		t.Fatalf("verify wrong-case password must fail: ok=%v err=%v", ok, err)
	}
}

func TestHashPassword_UniqueSalts(t *testing.T) {
	a, _ := HashPassword("same", testArgon2())
	b, _ := HashPassword("same", testArgon2())
	if a == b {
		t.Fatal("two hashes of the same password must differ (random salt)")
	}
}

func TestVerifyPassword_Malformed(t *testing.T) {
	for _, phc := range []string{"", "plaintext", SentinelGamePassword, "$argon2id$v=19$m=x$$"} {
		if ok, err := VerifyPassword("anything", phc); ok || err == nil {
			t.Errorf("VerifyPassword(_, %q) = %v, %v; want false, error", phc, ok, err)
		}
	}
}

func TestValidPortalPassword(t *testing.T) {
	cases := []struct {
		pw string
		ok bool
	}{
		{"abcd1234", true},
		{"exactly-twenty-chs20", true},
		{"short7!", false},                    // < 8
		{"this-is-way-longer-than-twenty", false}, // > 20 (client cap)
		{"has space", false},                  // space not client-typable in password field
		{"unicodé-pw", false},                 // non-ASCII
	}
	for _, tc := range cases {
		err := ValidPortalPassword(tc.pw)
		if (err == nil) != tc.ok {
			t.Errorf("ValidPortalPassword(%q) = %v, want ok=%v", tc.pw, err, tc.ok)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -run 'Password|HashVerify' -v`
Expected: FAIL to build — `HashPassword` undefined.

- [ ] **Step 3: Write password.go**

```go
// modules/account/password.go
package account

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// SentinelGamePassword is stored in the game `account.password` column
// for rows created by portal character creation. It is not a valid
// bcrypt hash, so if a deployment is ever flipped back to
// login.auth_mode=local, bcrypt comparison always fails — portal
// characters cannot be logged into through the legacy path.
const SentinelGamePassword = "!portal-managed!"

const (
	saltLen = 16
	keyLen  = 32
)

// HashPassword derives an argon2id hash and encodes it as a PHC string:
// $argon2id$v=19$m=<KiB>,t=<time>,p=<lanes>$<b64salt>$<b64key>.
func HashPassword(password string, p Argon2Config) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("account: salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, uint32(p.Time), uint32(p.MemoryKiB), uint8(p.Parallelism), keyLen)
	b64 := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.MemoryKiB, p.Time, p.Parallelism, b64(salt), b64(key)), nil
}

// VerifyPassword re-derives the key with the parameters embedded in the
// PHC string and compares in constant time. A well-formed PHC string
// that doesn't match returns (false, nil); a malformed string returns
// (false, error).
func VerifyPassword(password, phc string) (bool, error) {
	parts := strings.Split(phc, "$")
	// ["", "argon2id", "v=19", "m=..,t=..,p=..", salt, key]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, fmt.Errorf("account: malformed password hash")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, fmt.Errorf("account: unsupported argon2 version %q", parts[2])
	}
	var m, t, p int
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false, fmt.Errorf("account: malformed argon2 params %q", parts[3])
	}
	if m < 1 || t < 1 || p < 1 {
		return false, fmt.Errorf("account: invalid argon2 params %q", parts[3])
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("account: malformed salt: %w", err)
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("account: malformed key: %w", err)
	}
	got := argon2.IDKey([]byte(password), salt, uint32(t), uint32(m), uint8(p), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// ValidPortalPassword enforces the portal password policy. The account
// password is also typed into the Java client's login screen, which
// caps passwords at 20 characters and cannot enter spaces or
// non-ASCII — hence the unusual upper bound.
func ValidPortalPassword(password string) error {
	if len(password) < 8 || len(password) > 20 {
		return fmt.Errorf("password must be 8-20 characters (the game client caps passwords at 20)")
	}
	for _, r := range password {
		if r <= ' ' || r > '~' {
			return fmt.Errorf("password may only contain printable ASCII characters (no spaces) so it can be typed in the game client")
		}
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/account/password.go modules/account/password_test.go
git commit --no-gpg-sign -m "feat(account): argon2id PHC hashing + portal password policy

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: Store part 1 — accounts, groups, audit

**Files:**
- Create: `modules/account/store.go`
- Test: `modules/account/store_test.go`

**Interfaces:**
- Consumes: `pkg/gamedb` (`Open`, `DB.Rebind`, `DB.Migrate`), migration 000003 tables, constants from Task 2.
- Produces (later tasks call exactly these):

```go
type Store struct{ db *gamedb.DB }
func NewStore(db *gamedb.DB) *Store

type PortalAccount struct {
	ID            int64
	Email         string
	EmailVerified bool
	PasswordHash  string
	Status        string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type AuditEntry struct {
	ID        int64
	Actor     sql.NullInt64
	Action    string
	Target    string
	Details   string
	CreatedAt time.Time
}

var (
	ErrNotFound       = errors.New("account: not found")
	ErrEmailTaken     = errors.New("account: email already registered")
	ErrIdentityTaken  = errors.New("account: identity already linked to another account")
	ErrAlreadyLinked  = errors.New("account: a link for this provider already exists on this account")
	ErrNameTaken      = errors.New("account: character name already taken")
	ErrCharacterLimit = errors.New("account: character limit reached")
)

func (s *Store) CreateAccount(ctx context.Context, email, passwordHash string) (int64, error) // lowercases+trims email; ErrEmailTaken
func (s *Store) AccountByEmail(ctx context.Context, email string) (*PortalAccount, error)     // ErrNotFound
func (s *Store) AccountByID(ctx context.Context, id int64) (*PortalAccount, error)            // ErrNotFound
func (s *Store) SetEmailVerified(ctx context.Context, id int64) error
func (s *Store) SetPasswordHash(ctx context.Context, id int64, phc string) error
func (s *Store) SetAccountStatus(ctx context.Context, id int64, status string) error
func (s *Store) AddGroupMember(ctx context.Context, group string, accountID, addedBy int64) error // addedBy 0 ⇒ NULL; idempotent
func (s *Store) RemoveGroupMember(ctx context.Context, group string, accountID int64) error
func (s *Store) IsGroupMember(ctx context.Context, group string, accountID int64) (bool, error)
func (s *Store) AppendAudit(ctx context.Context, actor int64, action, target, details string) error // actor 0 ⇒ NULL
func (s *Store) RecentAudit(ctx context.Context, limit int, target string) ([]AuditEntry, error)    // target "" = all, newest first
```

- [ ] **Step 1: Write the failing test**

```go
// modules/account/store_test.go
package account

import (
	"errors"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/gamedb"
)

// openTestStore returns a Store over a fresh migrated DB: SQLite by
// default, or Postgres (schema-isolated via gamedbtest.OpenTestSchema)
// when the same Postgres-DSN gate the login/friends test suites use is
// configured — read modules/login/db_test.go FIRST and reuse its exact
// DSN flag/env convention here so one setting drives all suites. Every
// Store query goes through db.Rebind, and this dual-backend helper is
// what proves it (spec: "SQLite and Postgres both").
func openTestStore(t *testing.T) *Store {
	t.Helper()
	var cfg gamedb.Config
	cfg.Backend = gamedb.BackendSQLite
	cfg.SQLite.DSN = filepath.Join(t.TempDir(), "test.db")
	db, err := gamedb.Open(cfg, slog.Default())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(t.Context()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewStore(db)
}

func TestStore_CreateAndFetchAccount(t *testing.T) {
	s := openTestStore(t)
	ctx := t.Context()

	id, err := s.CreateAccount(ctx, "  Player@Example.COM ", "$argon2id$fake")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	acct, err := s.AccountByEmail(ctx, "player@example.com")
	if err != nil {
		t.Fatalf("by email: %v", err)
	}
	if acct.ID != id || acct.Email != "player@example.com" || acct.EmailVerified ||
		acct.Status != StatusActive || acct.PasswordHash != "$argon2id$fake" {
		t.Fatalf("bad row: %+v", acct)
	}

	if _, err := s.CreateAccount(ctx, "PLAYER@example.com", "x"); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("duplicate email: got %v, want ErrEmailTaken", err)
	}
	if _, err := s.AccountByEmail(ctx, "ghost@example.com"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing account: got %v, want ErrNotFound", err)
	}

	if err := s.SetEmailVerified(ctx, id); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := s.SetAccountStatus(ctx, id, StatusDisabled); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if err := s.SetPasswordHash(ctx, id, "$argon2id$new"); err != nil {
		t.Fatalf("set hash: %v", err)
	}
	acct, err = s.AccountByID(ctx, id)
	if err != nil {
		t.Fatalf("by id: %v", err)
	}
	if !acct.EmailVerified || acct.Status != StatusDisabled || acct.PasswordHash != "$argon2id$new" {
		t.Fatalf("updates not applied: %+v", acct)
	}
}

func TestStore_Groups(t *testing.T) {
	s := openTestStore(t)
	ctx := t.Context()
	id, err := s.CreateAccount(ctx, "a@example.com", "x")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	ok, err := s.IsGroupMember(ctx, GroupManuallyApproved, id)
	if err != nil || ok {
		t.Fatalf("fresh account must not be approved: ok=%v err=%v", ok, err)
	}
	if err := s.AddGroupMember(ctx, GroupManuallyApproved, id, 0); err != nil {
		t.Fatalf("add: %v", err)
	}
	// Idempotent re-add.
	if err := s.AddGroupMember(ctx, GroupManuallyApproved, id, 0); err != nil {
		t.Fatalf("re-add must be idempotent: %v", err)
	}
	ok, err = s.IsGroupMember(ctx, GroupManuallyApproved, id)
	if err != nil || !ok {
		t.Fatalf("membership: ok=%v err=%v", ok, err)
	}
	if err := s.RemoveGroupMember(ctx, GroupManuallyApproved, id); err != nil {
		t.Fatalf("remove: %v", err)
	}
	ok, _ = s.IsGroupMember(ctx, GroupManuallyApproved, id)
	if ok {
		t.Fatal("membership must be gone after remove")
	}
	if err := s.AddGroupMember(ctx, "no_such_group", id, 0); err == nil {
		t.Fatal("unknown group must error")
	}
}

func TestStore_Audit(t *testing.T) {
	s := openTestStore(t)
	ctx := t.Context()
	id, _ := s.CreateAccount(ctx, "a@example.com", "x")

	if err := s.AppendAudit(ctx, 0, "account.register", "account:1", "self-service"); err != nil {
		t.Fatalf("append (system actor): %v", err)
	}
	if err := s.AppendAudit(ctx, id, "group.add", "account:1", "manually_approved"); err != nil {
		t.Fatalf("append: %v", err)
	}
	entries, err := s.RecentAudit(ctx, 10, "")
	if err != nil || len(entries) != 2 {
		t.Fatalf("recent: n=%d err=%v", len(entries), err)
	}
	// Newest first.
	if entries[0].Action != "group.add" || !entries[0].Actor.Valid || entries[0].Actor.Int64 != id {
		t.Fatalf("bad newest entry: %+v", entries[0])
	}
	if entries[1].Actor.Valid {
		t.Fatalf("system entry must have NULL actor: %+v", entries[1])
	}
	only, err := s.RecentAudit(ctx, 10, "account:1")
	if err != nil || len(only) != 2 {
		t.Fatalf("target filter: n=%d err=%v", len(only), err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -run TestStore -v`
Expected: FAIL to build — `Store` undefined.

- [ ] **Step 3: Write store.go**

```go
// modules/account/store.go
package account

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zsrv/goscape/pkg/gamedb"
)

// Store is the repository over the portal_* tables. All SQL is written
// with ? placeholders and passed through db.Rebind for dialect safety.
type Store struct {
	db *gamedb.DB
}

func NewStore(db *gamedb.DB) *Store { return &Store{db: db} }

// Sentinel errors. Handlers map these to friendly page messages; the
// gRPC layer maps them to response statuses.
var (
	ErrNotFound       = errors.New("account: not found")
	ErrEmailTaken     = errors.New("account: email already registered")
	ErrIdentityTaken  = errors.New("account: identity already linked to another account")
	ErrAlreadyLinked  = errors.New("account: a link for this provider already exists on this account")
	ErrNameTaken      = errors.New("account: character name already taken")
	ErrCharacterLimit = errors.New("account: character limit reached")
)

type PortalAccount struct {
	ID            int64
	Email         string
	EmailVerified bool
	PasswordHash  string
	Status        string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type AuditEntry struct {
	ID        int64
	Actor     sql.NullInt64
	Action    string
	Target    string
	Details   string
	CreatedAt time.Time
}

// isUniqueViolation reports whether err is a UNIQUE-constraint failure
// on either backend (modernc sqlite: "UNIQUE constraint failed";
// pgx: SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") || strings.Contains(msg, "23505")
}

// NormalizeEmail is the single place email case/space normalization
// happens; every read and write path goes through it.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (s *Store) CreateAccount(ctx context.Context, email, passwordHash string) (int64, error) {
	email = NormalizeEmail(email)
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, s.db.Rebind(
		`INSERT INTO portal_account (email, email_verified, password_hash, status, created_at, updated_at)
		 VALUES (?, 0, ?, ?, ?, ?)`), email, passwordHash, StatusActive, now, now)
	if isUniqueViolation(err) {
		return 0, ErrEmailTaken
	}
	if err != nil {
		return 0, fmt.Errorf("account: create account: %w", err)
	}
	acct, err := s.AccountByEmail(ctx, email)
	if err != nil {
		return 0, err
	}
	return acct.ID, nil
}

func (s *Store) accountBy(ctx context.Context, where string, arg any) (*PortalAccount, error) {
	var (
		a        PortalAccount
		verified int
	)
	err := s.db.QueryRowContext(ctx, s.db.Rebind(
		`SELECT id, email, email_verified, password_hash, status, created_at, updated_at
		 FROM portal_account WHERE `+where), arg).
		Scan(&a.ID, &a.Email, &verified, &a.PasswordHash, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("account: fetch account: %w", err)
	}
	a.EmailVerified = verified == 1
	return &a, nil
}

func (s *Store) AccountByEmail(ctx context.Context, email string) (*PortalAccount, error) {
	return s.accountBy(ctx, "email = ?", NormalizeEmail(email))
}

func (s *Store) AccountByID(ctx context.Context, id int64) (*PortalAccount, error) {
	return s.accountBy(ctx, "id = ?", id)
}

func (s *Store) updateAccount(ctx context.Context, id int64, set string, args ...any) error {
	args = append(args, time.Now().UTC(), id)
	res, err := s.db.ExecContext(ctx, s.db.Rebind(
		`UPDATE portal_account SET `+set+`, updated_at = ? WHERE id = ?`), args...)
	if err != nil {
		return fmt.Errorf("account: update account: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetEmailVerified(ctx context.Context, id int64) error {
	return s.updateAccount(ctx, id, "email_verified = 1")
}

func (s *Store) SetPasswordHash(ctx context.Context, id int64, phc string) error {
	return s.updateAccount(ctx, id, "password_hash = ?", phc)
}

func (s *Store) SetAccountStatus(ctx context.Context, id int64, status string) error {
	if status != StatusActive && status != StatusDisabled {
		return fmt.Errorf("account: invalid status %q", status)
	}
	return s.updateAccount(ctx, id, "status = ?", status)
}

func (s *Store) groupID(ctx context.Context, group string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, s.db.Rebind(
		`SELECT id FROM portal_group WHERE name = ?`), group).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("account: unknown group %q", group)
	}
	if err != nil {
		return 0, fmt.Errorf("account: group lookup: %w", err)
	}
	return id, nil
}

// AddGroupMember is idempotent: re-adding an existing member is a no-op.
// addedBy 0 records a NULL actor (CLI/system).
func (s *Store) AddGroupMember(ctx context.Context, group string, accountID, addedBy int64) error {
	gid, err := s.groupID(ctx, group)
	if err != nil {
		return err
	}
	var actor sql.NullInt64
	if addedBy != 0 {
		actor = sql.NullInt64{Int64: addedBy, Valid: true}
	}
	_, err = s.db.ExecContext(ctx, s.db.Rebind(
		`INSERT INTO portal_group_member (group_id, account_id, added_by, added_at)
		 VALUES (?, ?, ?, ?)`), gid, accountID, actor, time.Now().UTC())
	if isUniqueViolation(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("account: add group member: %w", err)
	}
	return nil
}

func (s *Store) RemoveGroupMember(ctx context.Context, group string, accountID int64) error {
	gid, err := s.groupID(ctx, group)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, s.db.Rebind(
		`DELETE FROM portal_group_member WHERE group_id = ? AND account_id = ?`), gid, accountID); err != nil {
		return fmt.Errorf("account: remove group member: %w", err)
	}
	return nil
}

func (s *Store) IsGroupMember(ctx context.Context, group string, accountID int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, s.db.Rebind(
		`SELECT COUNT(*) FROM portal_group_member gm
		 JOIN portal_group g ON g.id = gm.group_id
		 WHERE g.name = ? AND gm.account_id = ?`), group, accountID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("account: group membership: %w", err)
	}
	return n > 0, nil
}

// AppendAudit records an admin/security event. actor 0 ⇒ NULL (system
// or CLI). Audit failures should not break the calling action — most
// call sites log-and-continue; only admin actions treat this as fatal.
func (s *Store) AppendAudit(ctx context.Context, actor int64, action, target, details string) error {
	var a sql.NullInt64
	if actor != 0 {
		a = sql.NullInt64{Int64: actor, Valid: true}
	}
	if _, err := s.db.ExecContext(ctx, s.db.Rebind(
		`INSERT INTO portal_audit_log (actor_account_id, action, target, details, created_at)
		 VALUES (?, ?, ?, ?, ?)`), a, action, target, details, time.Now().UTC()); err != nil {
		return fmt.Errorf("account: append audit: %w", err)
	}
	return nil
}

func (s *Store) RecentAudit(ctx context.Context, limit int, target string) ([]AuditEntry, error) {
	q := `SELECT id, actor_account_id, action, target, details, created_at
	      FROM portal_audit_log`
	args := []any{}
	if target != "" {
		q += ` WHERE target = ?`
		args = append(args, target)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, s.db.Rebind(q), args...)
	if err != nil {
		return nil, fmt.Errorf("account: recent audit: %w", err)
	}
	defer rows.Close()
	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.Actor, &e.Action, &e.Target, &e.Details, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("account: scan audit: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -run TestStore -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/account/store.go modules/account/store_test.go
git commit --no-gpg-sign -m "feat(account): store — portal accounts, groups, audit log

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: Store part 2 — identities, characters, gate

**Files:**
- Create: `modules/account/store_identity.go`
- Test: `modules/account/store_identity_test.go`

**Interfaces:**
- Consumes: Task 4 `Store` + errors, `pkg/util/jstring.ToSafeName`, `SentinelGamePassword` (Task 3), game `account` table (migration 000001).
- Produces:

```go
type Identity struct {
	ID               int64
	AccountID        int64
	Provider         string
	ProviderUserID   string
	ProviderUsername string
	LinkedAt         time.Time
	RevokedAt        sql.NullTime
}

type Character struct {
	ID            int64
	AccountID     int64
	GameAccountID int64
	Username      string
	CreatedAt     time.Time
}

func NormalizeCharacterName(raw string) (string, error) // lowercase, spaces→_, 1..12 [a-z0-9_], jstring.ToSafeName fixpoint
func (s *Store) LinkIdentity(ctx context.Context, accountID int64, provider, providerUserID, providerUsername string) error // ErrIdentityTaken / ErrAlreadyLinked
func (s *Store) IdentitiesByAccount(ctx context.Context, accountID int64) ([]Identity, error)
func (s *Store) IdentityByProviderUser(ctx context.Context, provider, providerUserID string) (*Identity, error) // ErrNotFound
func (s *Store) RevokeIdentity(ctx context.Context, accountID int64, provider string) error  // soft burn: sets revoked_at
func (s *Store) ReleaseIdentity(ctx context.Context, provider, providerUserID string) error  // hard delete
func (s *Store) GateEligible(ctx context.Context, accountID int64, providers []string) (bool, error)
func (s *Store) CreateCharacter(ctx context.Context, accountID int64, name string, limit int) (Character, error) // name pre-normalized; ErrNameTaken / ErrCharacterLimit
func (s *Store) CharactersByAccount(ctx context.Context, accountID int64) ([]Character, error)
func (s *Store) CharacterWithAccount(ctx context.Context, username string) (*Character, *PortalAccount, error) // ErrNotFound
```

- [ ] **Step 1: Write the failing test**

```go
// modules/account/store_identity_test.go
package account

import (
	"errors"
	"testing"
)

func TestNormalizeCharacterName(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"Zezima", "zezima", true},
		{" My Name ", "my_name", true},
		{"a", "a", true},
		{"exactly12chr", "exactly12chr", true},
		{"thirteenchars", "", false},
		{"", "", false},
		{"bad!char", "", false},
	}
	for _, tc := range cases {
		got, err := NormalizeCharacterName(tc.in)
		if (err == nil) != tc.ok || got != tc.want {
			t.Errorf("NormalizeCharacterName(%q) = %q, %v; want %q ok=%v", tc.in, got, err, tc.want, tc.ok)
		}
	}
}

func TestStore_LinkIdentityRules(t *testing.T) {
	s := openTestStore(t)
	ctx := t.Context()
	a, _ := s.CreateAccount(ctx, "a@example.com", "x")
	b, _ := s.CreateAccount(ctx, "b@example.com", "x")

	if err := s.LinkIdentity(ctx, a, "discord", "D1", "alice"); err != nil {
		t.Fatalf("link: %v", err)
	}
	// Same Discord identity on a second account: taken.
	if err := s.LinkIdentity(ctx, b, "discord", "D1", "mallory"); !errors.Is(err, ErrIdentityTaken) {
		t.Fatalf("cross-account relink: got %v, want ErrIdentityTaken", err)
	}
	// Second Discord on the same account: already linked.
	if err := s.LinkIdentity(ctx, a, "discord", "D2", "alice2"); !errors.Is(err, ErrAlreadyLinked) {
		t.Fatalf("second provider link: got %v, want ErrAlreadyLinked", err)
	}

	// Burn: revoked identity STILL blocks reuse by another account.
	if err := s.RevokeIdentity(ctx, a, "discord"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	ids, err := s.IdentitiesByAccount(ctx, a)
	if err != nil || len(ids) != 1 || !ids[0].RevokedAt.Valid {
		t.Fatalf("revoked identity row: %+v err=%v", ids, err)
	}
	if err := s.LinkIdentity(ctx, b, "discord", "D1", "mallory"); !errors.Is(err, ErrIdentityTaken) {
		t.Fatalf("burned identity must stay taken: got %v", err)
	}

	// Release: hard delete frees it.
	if err := s.ReleaseIdentity(ctx, "discord", "D1"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := s.LinkIdentity(ctx, b, "discord", "D1", "mallory"); err != nil {
		t.Fatalf("post-release link: %v", err)
	}
}

func TestStore_GateEligible(t *testing.T) {
	s := openTestStore(t)
	ctx := t.Context()
	id, _ := s.CreateAccount(ctx, "a@example.com", "x")
	providers := []string{"discord"}

	if ok, _ := s.GateEligible(ctx, id, providers); ok {
		t.Fatal("fresh account must not be eligible")
	}
	// Linked identity satisfies the gate.
	if err := s.LinkIdentity(ctx, id, "discord", "D1", "alice"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.GateEligible(ctx, id, providers); !ok {
		t.Fatal("linked account must be eligible")
	}
	// Revoked link no longer satisfies it.
	if err := s.RevokeIdentity(ctx, id, "discord"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.GateEligible(ctx, id, providers); ok {
		t.Fatal("revoked link must not satisfy the gate")
	}
	// manually_approved overrides.
	if err := s.AddGroupMember(ctx, GroupManuallyApproved, id, 0); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.GateEligible(ctx, id, providers); !ok {
		t.Fatal("manually_approved must satisfy the gate")
	}
	// Empty provider list: only the group counts.
	if err := s.RemoveGroupMember(ctx, GroupManuallyApproved, id); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.GateEligible(ctx, id, nil); ok {
		t.Fatal("empty providers + no group must not be eligible")
	}
}

func TestStore_CreateCharacter(t *testing.T) {
	s := openTestStore(t)
	ctx := t.Context()
	a, _ := s.CreateAccount(ctx, "a@example.com", "x")
	b, _ := s.CreateAccount(ctx, "b@example.com", "x")

	ch, err := s.CreateCharacter(ctx, a, "zezima", 2)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if ch.AccountID != a || ch.Username != "zezima" || ch.GameAccountID == 0 {
		t.Fatalf("bad character: %+v", ch)
	}

	// The game account row exists with the sentinel password.
	var pw string
	err = s.db.QueryRowContext(ctx, s.db.Rebind(
		`SELECT password FROM account WHERE id = ?`), ch.GameAccountID).Scan(&pw)
	if err != nil || pw != SentinelGamePassword {
		t.Fatalf("game account row: pw=%q err=%v", pw, err)
	}

	// Name uniqueness spans accounts (game account.username UNIQUE).
	if _, err := s.CreateCharacter(ctx, b, "zezima", 2); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("dup name: got %v, want ErrNameTaken", err)
	}

	// Limit.
	if _, err := s.CreateCharacter(ctx, a, "alt1", 2); err != nil {
		t.Fatalf("second char: %v", err)
	}
	if _, err := s.CreateCharacter(ctx, a, "alt2", 2); !errors.Is(err, ErrCharacterLimit) {
		t.Fatalf("over limit: got %v, want ErrCharacterLimit", err)
	}

	chars, err := s.CharactersByAccount(ctx, a)
	if err != nil || len(chars) != 2 {
		t.Fatalf("list: n=%d err=%v", len(chars), err)
	}

	gotCh, gotAcct, err := s.CharacterWithAccount(ctx, "zezima")
	if err != nil || gotCh.ID != ch.ID || gotAcct.ID != a {
		t.Fatalf("with account: %+v %+v %v", gotCh, gotAcct, err)
	}
	if _, _, err := s.CharacterWithAccount(ctx, "ghost"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing char: got %v, want ErrNotFound", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -run 'Normalize|LinkIdentity|Gate|CreateCharacter' -v`
Expected: FAIL to build — `NormalizeCharacterName` undefined.

- [ ] **Step 3: Write store_identity.go**

```go
// modules/account/store_identity.go
package account

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zsrv/goscape/pkg/util/jstring"
)

type Identity struct {
	ID               int64
	AccountID        int64
	Provider         string
	ProviderUserID   string
	ProviderUsername string
	LinkedAt         time.Time
	RevokedAt        sql.NullTime
}

type Character struct {
	ID            int64
	AccountID     int64
	GameAccountID int64
	Username      string
	CreatedAt     time.Time
}

// NormalizeCharacterName lowercases, maps spaces to underscores, and
// enforces the RS2 name rules (1-12 chars of [a-z0-9_], round-trips
// through jstring.ToSafeName). The returned name is what gets stored
// and what the game client must type at login.
func NormalizeCharacterName(raw string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(raw))
	name = strings.ReplaceAll(name, " ", "_")
	if name == "" || len(name) > 12 {
		return "", fmt.Errorf("character name must be 1-12 characters")
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return "", fmt.Errorf("character name may only contain letters, numbers, spaces and underscores")
		}
	}
	if safe := jstring.ToSafeName(name); safe != name {
		return "", fmt.Errorf("character name %q is not a valid game name", raw)
	}
	return name, nil
}

// LinkIdentity attaches a third-party identity. Two invariants, both
// enforced by migration-000003 UNIQUE constraints and mapped to
// sentinel errors here:
//   - (provider, provider_user_id) is globally unique — one Discord
//     identity vouches for at most one portal account, ever (a revoked
//     row still occupies the slot: the anti-bot "burn").
//   - (account_id, provider) is unique — one link per provider.
func (s *Store) LinkIdentity(ctx context.Context, accountID int64, provider, providerUserID, providerUsername string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(
		`INSERT INTO portal_identity (account_id, provider, provider_user_id, provider_username, linked_at)
		 VALUES (?, ?, ?, ?, ?)`), accountID, provider, providerUserID, providerUsername, time.Now().UTC())
	if isUniqueViolation(err) {
		// Disambiguate which constraint fired.
		if existing, lookErr := s.IdentityByProviderUser(ctx, provider, providerUserID); lookErr == nil && existing.AccountID != accountID {
			return ErrIdentityTaken
		}
		return ErrAlreadyLinked
	}
	if err != nil {
		return fmt.Errorf("account: link identity: %w", err)
	}
	return nil
}

func (s *Store) IdentityByProviderUser(ctx context.Context, provider, providerUserID string) (*Identity, error) {
	var id Identity
	err := s.db.QueryRowContext(ctx, s.db.Rebind(
		`SELECT id, account_id, provider, provider_user_id, provider_username, linked_at, revoked_at
		 FROM portal_identity WHERE provider = ? AND provider_user_id = ?`), provider, providerUserID).
		Scan(&id.ID, &id.AccountID, &id.Provider, &id.ProviderUserID, &id.ProviderUsername, &id.LinkedAt, &id.RevokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("account: identity lookup: %w", err)
	}
	return &id, nil
}

func (s *Store) IdentitiesByAccount(ctx context.Context, accountID int64) ([]Identity, error) {
	rows, err := s.db.QueryContext(ctx, s.db.Rebind(
		`SELECT id, account_id, provider, provider_user_id, provider_username, linked_at, revoked_at
		 FROM portal_identity WHERE account_id = ? ORDER BY provider`), accountID)
	if err != nil {
		return nil, fmt.Errorf("account: identities: %w", err)
	}
	defer rows.Close()
	var out []Identity
	for rows.Next() {
		var id Identity
		if err := rows.Scan(&id.ID, &id.AccountID, &id.Provider, &id.ProviderUserID, &id.ProviderUsername, &id.LinkedAt, &id.RevokedAt); err != nil {
			return nil, fmt.Errorf("account: scan identity: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// RevokeIdentity soft-deletes ("burns") a link: the row keeps occupying
// the (provider, provider_user_id) UNIQUE slot but no longer satisfies
// the gate. Admin-only operation; the caller audits it.
func (s *Store) RevokeIdentity(ctx context.Context, accountID int64, provider string) error {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(
		`UPDATE portal_identity SET revoked_at = ? WHERE account_id = ? AND provider = ? AND revoked_at IS NULL`),
		time.Now().UTC(), accountID, provider)
	if err != nil {
		return fmt.Errorf("account: revoke identity: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ReleaseIdentity hard-deletes a link, freeing the third-party identity
// for use on another account (support flow: player lost their old
// portal account). Admin-only; the caller audits it.
func (s *Store) ReleaseIdentity(ctx context.Context, provider, providerUserID string) error {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(
		`DELETE FROM portal_identity WHERE provider = ? AND provider_user_id = ?`), provider, providerUserID)
	if err != nil {
		return fmt.Errorf("account: release identity: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// GateEligible implements the spec's character-creation gate clause:
// member of manually_approved OR a non-revoked identity whose provider
// is in providers. It does NOT check status/verified/limit — the
// creation path (portal handler / admin RPC) composes those.
func (s *Store) GateEligible(ctx context.Context, accountID int64, providers []string) (bool, error) {
	approved, err := s.IsGroupMember(ctx, GroupManuallyApproved, accountID)
	if err != nil {
		return false, err
	}
	if approved {
		return true, nil
	}
	if len(providers) == 0 {
		return false, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(providers)), ",")
	args := []any{accountID}
	for _, p := range providers {
		args = append(args, p)
	}
	var n int
	err = s.db.QueryRowContext(ctx, s.db.Rebind(
		`SELECT COUNT(*) FROM portal_identity
		 WHERE account_id = ? AND revoked_at IS NULL AND provider IN (`+placeholders+`)`), args...).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("account: gate query: %w", err)
	}
	return n > 0, nil
}

// CreateCharacter reserves the name and creates BOTH rows in one
// transaction: the game `account` row (with the unusable sentinel
// password — auth is delegated in account mode) and the
// portal_character row pointing at it. The game account.username
// UNIQUE constraint is the single source of name reservation, so
// legacy rows are automatically respected. name must already be
// normalized via NormalizeCharacterName.
func (s *Store) CreateCharacter(ctx context.Context, accountID int64, name string, limit int) (Character, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Character{}, fmt.Errorf("account: begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var n int
	if err := tx.QueryRowContext(ctx, s.db.Rebind(
		`SELECT COUNT(*) FROM portal_character WHERE account_id = ?`), accountID).Scan(&n); err != nil {
		return Character{}, fmt.Errorf("account: count characters: %w", err)
	}
	if n >= limit {
		return Character{}, ErrCharacterLimit
	}

	if _, err := tx.ExecContext(ctx, s.db.Rebind(
		`INSERT INTO account (username, password, registration_ip) VALUES (?, ?, 'portal')`),
		name, SentinelGamePassword); err != nil {
		if isUniqueViolation(err) {
			return Character{}, ErrNameTaken
		}
		return Character{}, fmt.Errorf("account: insert game account: %w", err)
	}
	var gameID int64
	if err := tx.QueryRowContext(ctx, s.db.Rebind(
		`SELECT id FROM account WHERE username = ?`), name).Scan(&gameID); err != nil {
		return Character{}, fmt.Errorf("account: game account id: %w", err)
	}

	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, s.db.Rebind(
		`INSERT INTO portal_character (account_id, username, game_account_id, created_at)
		 VALUES (?, ?, ?, ?)`), accountID, name, gameID, now); err != nil {
		if isUniqueViolation(err) {
			return Character{}, ErrNameTaken
		}
		return Character{}, fmt.Errorf("account: insert character: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Character{}, fmt.Errorf("account: commit: %w", err)
	}
	committed = true

	var ch Character
	err = s.db.QueryRowContext(ctx, s.db.Rebind(
		`SELECT id, account_id, game_account_id, username, created_at
		 FROM portal_character WHERE username = ?`), name).
		Scan(&ch.ID, &ch.AccountID, &ch.GameAccountID, &ch.Username, &ch.CreatedAt)
	if err != nil {
		return Character{}, fmt.Errorf("account: reload character: %w", err)
	}
	return ch, nil
}

func (s *Store) CharactersByAccount(ctx context.Context, accountID int64) ([]Character, error) {
	rows, err := s.db.QueryContext(ctx, s.db.Rebind(
		`SELECT id, account_id, game_account_id, username, created_at
		 FROM portal_character WHERE account_id = ? ORDER BY id`), accountID)
	if err != nil {
		return nil, fmt.Errorf("account: characters: %w", err)
	}
	defer rows.Close()
	var out []Character
	for rows.Next() {
		var ch Character
		if err := rows.Scan(&ch.ID, &ch.AccountID, &ch.GameAccountID, &ch.Username, &ch.CreatedAt); err != nil {
			return nil, fmt.Errorf("account: scan character: %w", err)
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

// CharacterWithAccount resolves a game login: character name → owning
// portal account. Used by VerifyGameLogin.
func (s *Store) CharacterWithAccount(ctx context.Context, username string) (*Character, *PortalAccount, error) {
	var ch Character
	err := s.db.QueryRowContext(ctx, s.db.Rebind(
		`SELECT id, account_id, game_account_id, username, created_at
		 FROM portal_character WHERE username = ?`), username).
		Scan(&ch.ID, &ch.AccountID, &ch.GameAccountID, &ch.Username, &ch.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("account: character lookup: %w", err)
	}
	acct, err := s.AccountByID(ctx, ch.AccountID)
	if err != nil {
		return nil, nil, err
	}
	return &ch, acct, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -v`
Expected: PASS. If `TestNormalizeCharacterName` fails on the `jstring.ToSafeName` fixpoint check, read `pkg/util/jstring/jstring.go:66` and align the pre-check (do NOT weaken the fixpoint assertion — the stored name must be exactly what the game normalizes to).

- [ ] **Step 5: Commit**

```bash
git add modules/account/store_identity.go modules/account/store_identity_test.go
git commit --no-gpg-sign -m "feat(account): identities (link/burn/release), characters, creation gate

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 6: Store part 3 — portal sessions + email tokens

**Files:**
- Create: `modules/account/store_session.go`
- Test: `modules/account/store_session_test.go`

**Interfaces:**
- Consumes: Task 4 `Store` + errors, `SessionConfig` (Task 2).
- Produces:

```go
const (
	TokenPurposeVerifyEmail   = "verify_email"
	TokenPurposeResetPassword = "reset_password"
)

func NewRawToken() (string, error)  // 32 random bytes, base64url (cookie / email-link value)
func HashToken(raw string) string   // hex(sha256(raw)) — the only form stored

func (s *Store) CreateSession(ctx context.Context, accountID int64, tokenHash, ip, userAgent string, cfg SessionConfig) error
func (s *Store) SessionAccount(ctx context.Context, tokenHash string, cfg SessionConfig) (*PortalAccount, error) // ErrNotFound if missing/expired; touches idle expiry
func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error
func (s *Store) DeleteAccountSessions(ctx context.Context, accountID int64) error
func (s *Store) CreateToken(ctx context.Context, accountID int64, purpose, tokenHash string, ttl time.Duration) error
func (s *Store) ConsumeToken(ctx context.Context, purpose, tokenHash string) (int64, error) // single-use; ErrNotFound if used/expired/missing
```

- [ ] **Step 1: Write the failing test**

```go
// modules/account/store_session_test.go
package account

import (
	"errors"
	"testing"
	"time"
)

func TestTokenHelpers(t *testing.T) {
	a, err := NewRawToken()
	if err != nil || len(a) < 40 {
		t.Fatalf("NewRawToken: %q err=%v", a, err)
	}
	b, _ := NewRawToken()
	if a == b {
		t.Fatal("tokens must be unique")
	}
	if HashToken(a) == HashToken(b) || len(HashToken(a)) != 64 {
		t.Fatalf("HashToken: %q", HashToken(a))
	}
}

func TestStore_SessionLifecycle(t *testing.T) {
	s := openTestStore(t)
	ctx := t.Context()
	id, _ := s.CreateAccount(ctx, "a@example.com", "x")
	cfg := SessionConfig{IdleTTL: time.Hour, AbsoluteTTL: 24 * time.Hour}

	raw, _ := NewRawToken()
	if err := s.CreateSession(ctx, id, HashToken(raw), "1.2.3.4", "ua", cfg); err != nil {
		t.Fatalf("create: %v", err)
	}
	acct, err := s.SessionAccount(ctx, HashToken(raw), cfg)
	if err != nil || acct.ID != id {
		t.Fatalf("load: %+v err=%v", acct, err)
	}
	if _, err := s.SessionAccount(ctx, HashToken("wrong"), cfg); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bogus token: got %v", err)
	}

	if err := s.DeleteSession(ctx, HashToken(raw)); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.SessionAccount(ctx, HashToken(raw), cfg); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted session must be gone: got %v", err)
	}

	// Expired sessions are rejected.
	raw2, _ := NewRawToken()
	expired := SessionConfig{IdleTTL: -time.Hour, AbsoluteTTL: 24 * time.Hour}
	if err := s.CreateSession(ctx, id, HashToken(raw2), "", "", expired); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionAccount(ctx, HashToken(raw2), cfg); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired session: got %v", err)
	}

	// DeleteAccountSessions clears everything ("log out everywhere").
	raw3, _ := NewRawToken()
	_ = s.CreateSession(ctx, id, HashToken(raw3), "", "", cfg)
	if err := s.DeleteAccountSessions(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionAccount(ctx, HashToken(raw3), cfg); !errors.Is(err, ErrNotFound) {
		t.Fatalf("account sessions must be gone: got %v", err)
	}
}

func TestStore_TokenSingleUse(t *testing.T) {
	s := openTestStore(t)
	ctx := t.Context()
	id, _ := s.CreateAccount(ctx, "a@example.com", "x")

	raw, _ := NewRawToken()
	if err := s.CreateToken(ctx, id, TokenPurposeVerifyEmail, HashToken(raw), time.Hour); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := s.ConsumeToken(ctx, TokenPurposeVerifyEmail, HashToken(raw))
	if err != nil || got != id {
		t.Fatalf("consume: got=%d err=%v", got, err)
	}
	// Second consume fails (single-use).
	if _, err := s.ConsumeToken(ctx, TokenPurposeVerifyEmail, HashToken(raw)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("re-consume: got %v", err)
	}
	// Wrong purpose fails.
	raw2, _ := NewRawToken()
	_ = s.CreateToken(ctx, id, TokenPurposeResetPassword, HashToken(raw2), time.Hour)
	if _, err := s.ConsumeToken(ctx, TokenPurposeVerifyEmail, HashToken(raw2)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-purpose consume: got %v", err)
	}
	// Expired fails.
	raw3, _ := NewRawToken()
	_ = s.CreateToken(ctx, id, TokenPurposeVerifyEmail, HashToken(raw3), -time.Hour)
	if _, err := s.ConsumeToken(ctx, TokenPurposeVerifyEmail, HashToken(raw3)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired consume: got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -run 'TokenHelpers|SessionLifecycle|TokenSingleUse' -v`
Expected: FAIL to build — `NewRawToken` undefined.

- [ ] **Step 3: Write store_session.go**

```go
// modules/account/store_session.go
package account

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// Email-token purposes (portal_token.purpose).
const (
	TokenPurposeVerifyEmail   = "verify_email"
	TokenPurposeResetPassword = "reset_password"
)

// NewRawToken returns 32 random bytes base64url-encoded — the value
// that travels in a cookie or email link. Only its hash is stored.
func NewRawToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("account: token entropy: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashToken is the storage form of a raw token: hex(sha256(raw)). A DB
// leak therefore does not leak usable session/reset tokens.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (s *Store) CreateSession(ctx context.Context, accountID int64, tokenHash, ip, userAgent string, cfg SessionConfig) error {
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, s.db.Rebind(
		`INSERT INTO portal_session (token_hash, account_id, created_at, expires_at, ip, user_agent)
		 VALUES (?, ?, ?, ?, ?, ?)`),
		tokenHash, accountID, now, now.Add(cfg.IdleTTL), ip, userAgent); err != nil {
		return fmt.Errorf("account: create session: %w", err)
	}
	return nil
}

// SessionAccount validates a session token hash and returns the owning
// account. A hit slides the idle expiry forward, clamped to the
// absolute expiry (created_at + AbsoluteTTL).
func (s *Store) SessionAccount(ctx context.Context, tokenHash string, cfg SessionConfig) (*PortalAccount, error) {
	var (
		accountID int64
		createdAt time.Time
		expiresAt time.Time
	)
	err := s.db.QueryRowContext(ctx, s.db.Rebind(
		`SELECT account_id, created_at, expires_at FROM portal_session WHERE token_hash = ?`), tokenHash).
		Scan(&accountID, &createdAt, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("account: session lookup: %w", err)
	}
	now := time.Now().UTC()
	absolute := createdAt.Add(cfg.AbsoluteTTL)
	if now.After(expiresAt) || now.After(absolute) {
		// Expired: clean up eagerly and report missing.
		_, _ = s.db.ExecContext(ctx, s.db.Rebind(`DELETE FROM portal_session WHERE token_hash = ?`), tokenHash)
		return nil, ErrNotFound
	}
	newExpiry := now.Add(cfg.IdleTTL)
	if newExpiry.After(absolute) {
		newExpiry = absolute
	}
	if _, err := s.db.ExecContext(ctx, s.db.Rebind(
		`UPDATE portal_session SET expires_at = ? WHERE token_hash = ?`), newExpiry, tokenHash); err != nil {
		return nil, fmt.Errorf("account: session touch: %w", err)
	}
	return s.AccountByID(ctx, accountID)
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	if _, err := s.db.ExecContext(ctx, s.db.Rebind(
		`DELETE FROM portal_session WHERE token_hash = ?`), tokenHash); err != nil {
		return fmt.Errorf("account: delete session: %w", err)
	}
	return nil
}

// DeleteAccountSessions implements "log out everywhere" (used by
// password reset and admin disable).
func (s *Store) DeleteAccountSessions(ctx context.Context, accountID int64) error {
	if _, err := s.db.ExecContext(ctx, s.db.Rebind(
		`DELETE FROM portal_session WHERE account_id = ?`), accountID); err != nil {
		return fmt.Errorf("account: delete account sessions: %w", err)
	}
	return nil
}

func (s *Store) CreateToken(ctx context.Context, accountID int64, purpose, tokenHash string, ttl time.Duration) error {
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, s.db.Rebind(
		`INSERT INTO portal_token (account_id, purpose, token_hash, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?)`), accountID, purpose, tokenHash, now.Add(ttl), now); err != nil {
		return fmt.Errorf("account: create token: %w", err)
	}
	return nil
}

// ConsumeToken atomically marks a live token used and returns its
// account. The UPDATE's WHERE clause is the single-use guarantee — a
// second consume matches zero rows.
func (s *Store) ConsumeToken(ctx context.Context, purpose, tokenHash string) (int64, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, s.db.Rebind(
		`UPDATE portal_token SET used_at = ?
		 WHERE purpose = ? AND token_hash = ? AND used_at IS NULL AND expires_at > ?`),
		now, purpose, tokenHash, now)
	if err != nil {
		return 0, fmt.Errorf("account: consume token: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, ErrNotFound
	}
	var accountID int64
	if err := s.db.QueryRowContext(ctx, s.db.Rebind(
		`SELECT account_id FROM portal_token WHERE purpose = ? AND token_hash = ?`),
		purpose, tokenHash).Scan(&accountID); err != nil {
		return 0, fmt.Errorf("account: consumed token lookup: %w", err)
	}
	return accountID, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/account/store_session.go modules/account/store_session_test.go
git commit --no-gpg-sign -m "feat(account): portal sessions + single-use email tokens

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 7: AccountService proto + generated code

**Files:**
- Create: `proto/account/account.proto`
- Generated: `pkg/accountpb/account.pb.go`, `pkg/accountpb/account_grpc.pb.go` (via `make protos`)

**Interfaces:**
- Consumes: buf toolchain (`make protos` regenerates ALL protos — the diff must only add `pkg/accountpb/`; if other generated files churn, STOP and investigate before committing).
- Produces: `pkg/accountpb` package — `AccountServiceClient`, `AccountServiceServer`, `RegisterAccountServiceServer`, `VerifyResult_*` enum values, all messages below.

- [ ] **Step 1: Write the proto**

```proto
// proto/account/account.proto
syntax = "proto3";
package account.v1;

import "google/protobuf/empty.proto";

option go_package = "github.com/zsrv/goscape/pkg/accountpb";

// AccountService is the account-management module's wire contract.
// VerifyGameLogin is called by the login module (unauthenticated,
// internal network). Every other RPC is the admin surface used by
// `goscape-cli account` and requires
//   authorization: Bearer <account.admin_token>
// metadata; with no token configured the admin surface is disabled.
// Spec: docs/superpowers/specs/2026-07-19-account-management-design.md
service AccountService {
  rpc VerifyGameLogin(VerifyGameLoginRequest) returns (VerifyGameLoginResponse);

  rpc SearchAccounts(SearchAccountsRequest)         returns (SearchAccountsResponse);
  rpc GetAccount(GetAccountRequest)                 returns (GetAccountResponse);
  rpc SetGroupMembership(SetGroupMembershipRequest) returns (google.protobuf.Empty);
  rpc SetAccountStatus(SetAccountStatusRequest)     returns (google.protobuf.Empty);
  rpc UnlinkIdentity(UnlinkIdentityRequest)         returns (google.protobuf.Empty);
  rpc ReleaseIdentity(ReleaseIdentityRequest)       returns (google.protobuf.Empty);
  rpc AdminResetPassword(AdminResetPasswordRequest) returns (AdminResetPasswordResponse);
  rpc BootstrapAdmin(BootstrapAdminRequest)         returns (google.protobuf.Empty);
}

enum VerifyResult {
  VERIFY_RESULT_UNSPECIFIED         = 0;
  VERIFY_RESULT_OK                  = 1;
  VERIFY_RESULT_INVALID_CREDENTIALS = 2;
  VERIFY_RESULT_ACCOUNT_DISABLED    = 3;
  VERIFY_RESULT_EMAIL_UNVERIFIED    = 4;
}

message VerifyGameLoginRequest {
  string character_name = 1; // safe-name normalized by the login module
  string password       = 2; // verbatim from the client (post-RSA decode)
  string remote_address  = 3; // audit context
}

message VerifyGameLoginResponse {
  VerifyResult result            = 1;
  int64        game_account_id   = 2; // goscape `account` row for this character
  int64        portal_account_id = 3;
}

message AccountSummary {
  int64  id              = 1;
  string email           = 2;
  bool   email_verified  = 3;
  string status          = 4; // active | disabled
  int32  character_count = 5;
}

// query matches: email substring (case-insensitive), exact character
// name, or exact provider user id.
message SearchAccountsRequest  { string query = 1; }
message SearchAccountsResponse { repeated AccountSummary accounts = 1; }

message IdentityInfo {
  string provider          = 1;
  string provider_user_id  = 2;
  string provider_username = 3;
  bool   revoked           = 4;
}

message CharacterInfo {
  int64  id              = 1;
  string username        = 2;
  int64  game_account_id = 3;
}

// Exactly one of id / email must be set.
message GetAccountRequest { int64 id = 1; string email = 2; }
message GetAccountResponse {
  AccountSummary         account    = 1;
  repeated IdentityInfo  identities = 2;
  repeated CharacterInfo characters = 3;
  repeated string        groups     = 4;
}

message SetGroupMembershipRequest {
  int64  account_id = 1;
  string group      = 2; // manually_approved | admin
  bool   member     = 3;
}

message SetAccountStatusRequest {
  int64  account_id = 1;
  string status     = 2; // active | disabled
}

// UnlinkIdentity soft-revokes ("burns") — the identity can never vouch
// for another account. ReleaseIdentity hard-deletes, freeing it.
message UnlinkIdentityRequest  { int64 account_id = 1; string provider = 2; }
message ReleaseIdentityRequest { string provider = 1; string provider_user_id = 2; }

// AdminResetPassword mints a single-use reset token and returns the
// portal URL an admin can hand to the player out-of-band.
message AdminResetPasswordRequest  { int64 account_id = 1; }
message AdminResetPasswordResponse { string reset_url = 1; }

message BootstrapAdminRequest { string email = 1; }
```

- [ ] **Step 2: Generate**

Run: `make protos`
Expected: creates `pkg/accountpb/account.pb.go` and `pkg/accountpb/account_grpc.pb.go`. Run `git status --short` — ONLY `proto/account/account.proto` and `pkg/accountpb/` may be new/changed (plus regenerated-but-identical files showing no diff). Known trap: `make check-generated-files` is broken by buf-format drift — use `make protos`, not that target.

- [ ] **Step 3: Compile check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/accountpb/`
Expected: clean build.

- [ ] **Step 4: Commit**

```bash
git add proto/account/account.proto pkg/accountpb/
git commit --no-gpg-sign -m "feat(proto): AccountService — VerifyGameLogin + admin surface

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 8: gRPC server — VerifyGameLogin + admin-token interceptor

**Files:**
- Create: `modules/account/grpc.go`
- Test: `modules/account/grpc_test.go`

**Interfaces:**
- Consumes: `pkg/accountpb` (Task 7), `Store` (Tasks 4-6), `VerifyPassword` (Task 3).
- Produces:

```go
func newGRPCServer(cfg Config, store *Store, log *slog.Logger) *grpc.Server // registered handler + interceptor + reflection
// test helper reused by Task 9 and the e2e task:
func startBufconnServer(t *testing.T, cfg Config, store *Store) accountpb.AccountServiceClient
```

- [ ] **Step 1: Write the failing test**

```go
// modules/account/grpc_test.go
package account

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/zsrv/goscape/pkg/accountpb"
)

// startBufconnServer runs the account gRPC server over an in-memory
// listener and returns a connected client. Reused by admin + e2e tests.
func startBufconnServer(t *testing.T, cfg Config, store *Store) accountpb.AccountServiceClient {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := newGRPCServer(cfg, store, testLogger(t))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return accountpb.NewAccountServiceClient(conn)
}

// seedVerifiedAccountWithCharacter registers an account (argon2
// password "hunter22!"), verifies email, and creates character `name`.
func seedVerifiedAccountWithCharacter(t *testing.T, s *Store, email, name string) int64 {
	t.Helper()
	ctx := t.Context()
	phc, err := HashPassword("hunter22!", testArgon2())
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.CreateAccount(ctx, email, phc)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetEmailVerified(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateCharacter(ctx, id, name, 5); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestVerifyGameLogin(t *testing.T) {
	s := openTestStore(t)
	cfg := defaultConfig(t)
	client := startBufconnServer(t, cfg, s)
	ctx := t.Context()

	portalID := seedVerifiedAccountWithCharacter(t, s, "a@example.com", "zezima")

	verify := func(name, pw string) *accountpb.VerifyGameLoginResponse {
		t.Helper()
		resp, err := client.VerifyGameLogin(ctx, &accountpb.VerifyGameLoginRequest{
			CharacterName: name, Password: pw, RemoteAddress: "1.2.3.4:5",
		})
		if err != nil {
			t.Fatalf("rpc: %v", err)
		}
		return resp
	}

	// OK path.
	resp := verify("zezima", "hunter22!")
	if resp.Result != accountpb.VerifyResult_VERIFY_RESULT_OK ||
		resp.PortalAccountId != portalID || resp.GameAccountId == 0 {
		t.Fatalf("ok path: %+v", resp)
	}
	// Unknown character.
	if r := verify("ghost", "hunter22!"); r.Result != accountpb.VerifyResult_VERIFY_RESULT_INVALID_CREDENTIALS {
		t.Fatalf("unknown char: %v", r.Result)
	}
	// Wrong password (case-sensitive).
	if r := verify("zezima", "HUNTER22!"); r.Result != accountpb.VerifyResult_VERIFY_RESULT_INVALID_CREDENTIALS {
		t.Fatalf("wrong pw: %v", r.Result)
	}
	// Disabled account (correct password).
	if err := s.SetAccountStatus(ctx, portalID, StatusDisabled); err != nil {
		t.Fatal(err)
	}
	if r := verify("zezima", "hunter22!"); r.Result != accountpb.VerifyResult_VERIFY_RESULT_ACCOUNT_DISABLED {
		t.Fatalf("disabled: %v", r.Result)
	}
	// Unverified email (correct password, re-enabled).
	if err := s.SetAccountStatus(ctx, portalID, StatusActive); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, s.db.Rebind(
		`UPDATE portal_account SET email_verified = 0 WHERE id = ?`), portalID); err != nil {
		t.Fatal(err)
	}
	if r := verify("zezima", "hunter22!"); r.Result != accountpb.VerifyResult_VERIFY_RESULT_EMAIL_UNVERIFIED {
		t.Fatalf("unverified: %v", r.Result)
	}
}

func TestAdminRPCsRequireToken(t *testing.T) {
	s := openTestStore(t)

	// No token configured: admin surface is disabled outright.
	cfgNoToken := defaultConfig(t)
	client := startBufconnServer(t, cfgNoToken, s)
	_, err := client.SearchAccounts(t.Context(), &accountpb.SearchAccountsRequest{Query: "x"})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("no-token: got %v, want PermissionDenied", err)
	}

	// Token configured: wrong/missing bearer is Unauthenticated; right one passes.
	cfg := defaultConfig(t)
	cfg.AdminToken = "sekrit"
	client = startBufconnServer(t, cfg, s)

	_, err = client.SearchAccounts(t.Context(), &accountpb.SearchAccountsRequest{Query: "x"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("missing bearer: got %v, want Unauthenticated", err)
	}
	bad := metadata.AppendToOutgoingContext(t.Context(), "authorization", "Bearer wrong")
	_, err = client.SearchAccounts(bad, &accountpb.SearchAccountsRequest{Query: "x"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("bad bearer: got %v, want Unauthenticated", err)
	}
	good := metadata.AppendToOutgoingContext(t.Context(), "authorization", "Bearer sekrit")
	if _, err = client.SearchAccounts(good, &accountpb.SearchAccountsRequest{Query: "x"}); err != nil {
		t.Fatalf("good bearer: %v", err)
	}

	// VerifyGameLogin never needs the token.
	if _, err = client.VerifyGameLogin(t.Context(), &accountpb.VerifyGameLoginRequest{
		CharacterName: "nobody", Password: "x",
	}); err != nil {
		t.Fatalf("VerifyGameLogin must bypass admin auth: %v", err)
	}
}
```

Also add the shared test logger helper (same file or a small `testing_test.go`):

```go
// modules/account/testing_test.go
package account

import (
	"log/slog"
	"testing"
)

func testLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.Default()
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -run 'VerifyGameLogin|AdminRPCsRequireToken' -v`
Expected: FAIL to build — `newGRPCServer` undefined. (`SearchAccounts` exists as an auto-generated Unimplemented stub, so only the two functions above are missing.)

- [ ] **Step 3: Write grpc.go**

```go
// modules/account/grpc.go
package account

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	"github.com/zsrv/goscape/pkg/accountpb"
)

// grpcHandler implements accountpb.AccountServiceServer. Admin RPCs are
// guarded by adminAuthInterceptor, not per-method checks.
type grpcHandler struct {
	accountpb.UnimplementedAccountServiceServer

	cfg   Config
	store *Store
	log   *slog.Logger
}

func newGRPCServer(cfg Config, store *Store, log *slog.Logger) *grpc.Server {
	// Same keepalive posture as the login module (arch-29.2): permit
	// the world's/login's 30s probes.
	s := grpc.NewServer(
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             15 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.UnaryInterceptor(adminAuthInterceptor(cfg.AdminToken)),
	)
	accountpb.RegisterAccountServiceServer(s, &grpcHandler{cfg: cfg, store: store, log: log})
	reflection.Register(s)
	return s
}

// adminAuthInterceptor gates every RPC except VerifyGameLogin behind
// `authorization: Bearer <token>` metadata. Empty configured token =
// admin surface disabled (PermissionDenied), distinct from a bad
// credential (Unauthenticated).
func adminAuthInterceptor(token string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if info.FullMethod == accountpb.AccountService_VerifyGameLogin_FullMethodName {
			return handler(ctx, req)
		}
		if token == "" {
			return nil, status.Error(codes.PermissionDenied, "admin RPCs disabled: account.admin_token not configured")
		}
		md, _ := metadata.FromIncomingContext(ctx)
		vals := md.Get("authorization")
		if len(vals) != 1 || subtle.ConstantTimeCompare([]byte(vals[0]), []byte("Bearer "+token)) != 1 {
			return nil, status.Error(codes.Unauthenticated, "invalid admin token")
		}
		return handler(ctx, req)
	}
}

// VerifyGameLogin resolves character name → portal account and checks
// the account password. Password is verified BEFORE account-state
// checks so a caller without valid credentials cannot probe account
// status. Returns statuses, never gRPC errors, for auth outcomes;
// gRPC errors mean infrastructure failure (login maps them to
// login-server-offline).
func (h *grpcHandler) VerifyGameLogin(ctx context.Context, req *accountpb.VerifyGameLoginRequest) (*accountpb.VerifyGameLoginResponse, error) {
	fail := func(r accountpb.VerifyResult) *accountpb.VerifyGameLoginResponse {
		return &accountpb.VerifyGameLoginResponse{Result: r}
	}
	ch, acct, err := h.store.CharacterWithAccount(ctx, req.CharacterName)
	if errors.Is(err, ErrNotFound) {
		return fail(accountpb.VerifyResult_VERIFY_RESULT_INVALID_CREDENTIALS), nil
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "character lookup: %v", err)
	}
	ok, err := VerifyPassword(req.Password, acct.PasswordHash)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "password verify: %v", err)
	}
	if !ok {
		return fail(accountpb.VerifyResult_VERIFY_RESULT_INVALID_CREDENTIALS), nil
	}
	if acct.Status != StatusActive {
		return fail(accountpb.VerifyResult_VERIFY_RESULT_ACCOUNT_DISABLED), nil
	}
	if !acct.EmailVerified {
		return fail(accountpb.VerifyResult_VERIFY_RESULT_EMAIL_UNVERIFIED), nil
	}
	return &accountpb.VerifyGameLoginResponse{
		Result:          accountpb.VerifyResult_VERIFY_RESULT_OK,
		GameAccountId:   ch.GameAccountID,
		PortalAccountId: acct.ID,
	}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -v`
Expected: PASS. (`TestAdminRPCsRequireToken`'s "good bearer" case passes because the Unimplemented stub returns codes.Unimplemented — adjust the assertion to accept `codes.Unimplemented` OR nil error until Task 9 lands: `if c := status.Code(err); c != codes.OK && c != codes.Unimplemented { ... }`. Task 9 removes the Unimplemented allowance.)

- [ ] **Step 5: Commit**

```bash
git add modules/account/grpc.go modules/account/grpc_test.go modules/account/testing_test.go
git commit --no-gpg-sign -m "feat(account): gRPC server — VerifyGameLogin + admin bearer-token gate

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 9: gRPC admin RPCs

**Files:**
- Create: `modules/account/grpc_admin.go`
- Modify: `modules/account/store.go` (append two queries: `GroupsByAccount`, `SearchAccounts`)
- Test: `modules/account/grpc_admin_test.go`

**Interfaces:**
- Consumes: Tasks 4-8.
- Produces:

```go
// store.go additions
func (s *Store) GroupsByAccount(ctx context.Context, accountID int64) ([]string, error)
func (s *Store) SearchAccounts(ctx context.Context, query string) ([]PortalAccount, error) // email substring (lower), OR exact character username, OR exact provider_user_id; LIMIT 50
// grpc_admin.go: SearchAccounts, GetAccount, SetGroupMembership, SetAccountStatus,
// UnlinkIdentity, ReleaseIdentity, AdminResetPassword, BootstrapAdmin on *grpcHandler
```

- [ ] **Step 1: Write the failing test**

```go
// modules/account/grpc_admin_test.go
package account

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/zsrv/goscape/pkg/accountpb"
)

func TestAdminRPCs(t *testing.T) {
	s := openTestStore(t)
	cfg := defaultConfig(t)
	cfg.AdminToken = "sekrit"
	cfg.PublicURL = "http://portal.test"
	client := startBufconnServer(t, cfg, s)
	ctx := metadata.AppendToOutgoingContext(t.Context(), "authorization", "Bearer sekrit")

	portalID := seedVerifiedAccountWithCharacter(t, s, "a@example.com", "zezima")
	if err := s.LinkIdentity(t.Context(), portalID, "discord", "D1", "alice"); err != nil {
		t.Fatal(err)
	}

	// Search: by email substring, character name, provider user id.
	for _, q := range []string{"a@example", "zezima", "D1"} {
		resp, err := client.SearchAccounts(ctx, &accountpb.SearchAccountsRequest{Query: q})
		if err != nil || len(resp.Accounts) != 1 || resp.Accounts[0].Id != portalID {
			t.Fatalf("search %q: %+v err=%v", q, resp, err)
		}
	}

	// GetAccount by id: identities + characters + groups present.
	got, err := client.GetAccount(ctx, &accountpb.GetAccountRequest{Id: portalID})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Account.Email != "a@example.com" || got.Account.CharacterCount != 1 ||
		len(got.Identities) != 1 || got.Identities[0].ProviderUserId != "D1" ||
		len(got.Characters) != 1 || got.Characters[0].Username != "zezima" {
		t.Fatalf("get payload: %+v", got)
	}

	// Group membership round-trip.
	if _, err := client.SetGroupMembership(ctx, &accountpb.SetGroupMembershipRequest{
		AccountId: portalID, Group: GroupManuallyApproved, Member: true}); err != nil {
		t.Fatal(err)
	}
	got, _ = client.GetAccount(ctx, &accountpb.GetAccountRequest{Id: portalID})
	if len(got.Groups) != 1 || got.Groups[0] != GroupManuallyApproved {
		t.Fatalf("groups after add: %v", got.Groups)
	}
	if _, err := client.SetGroupMembership(ctx, &accountpb.SetGroupMembershipRequest{
		AccountId: portalID, Group: "bogus", Member: true}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("bogus group: %v", err)
	}

	// Status.
	if _, err := client.SetAccountStatus(ctx, &accountpb.SetAccountStatusRequest{
		AccountId: portalID, Status: StatusDisabled}); err != nil {
		t.Fatal(err)
	}

	// Unlink (burn) then release.
	if _, err := client.UnlinkIdentity(ctx, &accountpb.UnlinkIdentityRequest{
		AccountId: portalID, Provider: "discord"}); err != nil {
		t.Fatal(err)
	}
	got, _ = client.GetAccount(ctx, &accountpb.GetAccountRequest{Id: portalID})
	if !got.Identities[0].Revoked {
		t.Fatalf("identity not revoked: %+v", got.Identities)
	}
	if _, err := client.ReleaseIdentity(ctx, &accountpb.ReleaseIdentityRequest{
		Provider: "discord", ProviderUserId: "D1"}); err != nil {
		t.Fatal(err)
	}

	// AdminResetPassword returns a reset URL whose token consumes.
	rp, err := client.AdminResetPassword(ctx, &accountpb.AdminResetPasswordRequest{AccountId: portalID})
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "http://portal.test/reset-password?token="
	if len(rp.ResetUrl) <= len(prefix) || rp.ResetUrl[:len(prefix)] != prefix {
		t.Fatalf("reset url: %q", rp.ResetUrl)
	}
	raw := rp.ResetUrl[len(prefix):]
	if _, err := s.ConsumeToken(t.Context(), TokenPurposeResetPassword, HashToken(raw)); err != nil {
		t.Fatalf("reset token must consume: %v", err)
	}

	// BootstrapAdmin by email.
	if _, err := client.BootstrapAdmin(ctx, &accountpb.BootstrapAdminRequest{Email: "a@example.com"}); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.IsGroupMember(t.Context(), GroupAdmin, portalID); !ok {
		t.Fatal("bootstrap-admin must add admin group")
	}

	// Every admin action audited.
	entries, err := s.RecentAudit(t.Context(), 50, "")
	if err != nil || len(entries) < 6 {
		t.Fatalf("audit entries: n=%d err=%v", len(entries), err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -run TestAdminRPCs -v`
Expected: FAIL — RPCs return codes.Unimplemented.

- [ ] **Step 3: Write store additions + grpc_admin.go**

Append to `modules/account/store.go`:

```go
func (s *Store) GroupsByAccount(ctx context.Context, accountID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, s.db.Rebind(
		`SELECT g.name FROM portal_group_member gm
		 JOIN portal_group g ON g.id = gm.group_id
		 WHERE gm.account_id = ? ORDER BY g.name`), accountID)
	if err != nil {
		return nil, fmt.Errorf("account: groups by account: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("account: scan group: %w", err)
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// SearchAccounts finds accounts by email substring (case-insensitive),
// exact character name, or exact linked provider user id. Admin-only
// surface; capped at 50 rows.
func (s *Store) SearchAccounts(ctx context.Context, query string) ([]PortalAccount, error) {
	pattern := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"
	exact := strings.ToLower(strings.TrimSpace(query))
	rows, err := s.db.QueryContext(ctx, s.db.Rebind(
		`SELECT DISTINCT pa.id, pa.email, pa.email_verified, pa.password_hash, pa.status, pa.created_at, pa.updated_at
		 FROM portal_account pa
		 LEFT JOIN portal_character pc ON pc.account_id = pa.id
		 LEFT JOIN portal_identity pi ON pi.account_id = pa.id
		 WHERE pa.email LIKE ? OR pc.username = ? OR pi.provider_user_id = ?
		 ORDER BY pa.id LIMIT 50`), pattern, exact, query)
	if err != nil {
		return nil, fmt.Errorf("account: search: %w", err)
	}
	defer rows.Close()
	var out []PortalAccount
	for rows.Next() {
		var (
			a        PortalAccount
			verified int
		)
		if err := rows.Scan(&a.ID, &a.Email, &verified, &a.PasswordHash, &a.Status, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("account: scan search row: %w", err)
		}
		a.EmailVerified = verified == 1
		out = append(out, a)
	}
	return out, rows.Err()
}
```

Create `modules/account/grpc_admin.go`:

```go
// modules/account/grpc_admin.go
package account

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/zsrv/goscape/pkg/accountpb"
)

// audit records an admin action; the gRPC surface has no portal actor,
// so entries carry a NULL actor and a "grpc-admin" detail prefix.
func (h *grpcHandler) audit(ctx context.Context, action, target, details string) {
	if err := h.store.AppendAudit(ctx, 0, action, target, "grpc-admin: "+details); err != nil {
		h.log.Warn("audit append failed", slog.String("action", action), slog.Any("err", err))
	}
}

func accountTarget(id int64) string { return fmt.Sprintf("account:%d", id) }

func (h *grpcHandler) summary(ctx context.Context, a *PortalAccount) (*accountpb.AccountSummary, error) {
	chars, err := h.store.CharactersByAccount(ctx, a.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "characters: %v", err)
	}
	return &accountpb.AccountSummary{
		Id:             a.ID,
		Email:          a.Email,
		EmailVerified:  a.EmailVerified,
		Status:         a.Status,
		CharacterCount: int32(len(chars)),
	}, nil
}

func (h *grpcHandler) SearchAccounts(ctx context.Context, req *accountpb.SearchAccountsRequest) (*accountpb.SearchAccountsResponse, error) {
	accts, err := h.store.SearchAccounts(ctx, req.Query)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "search: %v", err)
	}
	resp := &accountpb.SearchAccountsResponse{}
	for i := range accts {
		sum, err := h.summary(ctx, &accts[i])
		if err != nil {
			return nil, err
		}
		resp.Accounts = append(resp.Accounts, sum)
	}
	return resp, nil
}

func (h *grpcHandler) GetAccount(ctx context.Context, req *accountpb.GetAccountRequest) (*accountpb.GetAccountResponse, error) {
	var (
		acct *PortalAccount
		err  error
	)
	switch {
	case req.Id != 0:
		acct, err = h.store.AccountByID(ctx, req.Id)
	case req.Email != "":
		acct, err = h.store.AccountByEmail(ctx, req.Email)
	default:
		return nil, status.Error(codes.InvalidArgument, "one of id or email is required")
	}
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "account not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "lookup: %v", err)
	}

	sum, serr := h.summary(ctx, acct)
	if serr != nil {
		return nil, serr
	}
	resp := &accountpb.GetAccountResponse{Account: sum}

	ids, err := h.store.IdentitiesByAccount(ctx, acct.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "identities: %v", err)
	}
	for _, id := range ids {
		resp.Identities = append(resp.Identities, &accountpb.IdentityInfo{
			Provider:         id.Provider,
			ProviderUserId:   id.ProviderUserID,
			ProviderUsername: id.ProviderUsername,
			Revoked:          id.RevokedAt.Valid,
		})
	}
	chars, err := h.store.CharactersByAccount(ctx, acct.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "characters: %v", err)
	}
	for _, ch := range chars {
		resp.Characters = append(resp.Characters, &accountpb.CharacterInfo{
			Id: ch.ID, Username: ch.Username, GameAccountId: ch.GameAccountID,
		})
	}
	groups, err := h.store.GroupsByAccount(ctx, acct.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "groups: %v", err)
	}
	resp.Groups = groups
	return resp, nil
}

func (h *grpcHandler) SetGroupMembership(ctx context.Context, req *accountpb.SetGroupMembershipRequest) (*emptypb.Empty, error) {
	if !slices.Contains([]string{GroupManuallyApproved, GroupAdmin}, req.Group) {
		return nil, status.Errorf(codes.InvalidArgument, "unknown group %q", req.Group)
	}
	var err error
	if req.Member {
		err = h.store.AddGroupMember(ctx, req.Group, req.AccountId, 0)
	} else {
		err = h.store.RemoveGroupMember(ctx, req.Group, req.AccountId)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "group membership: %v", err)
	}
	h.audit(ctx, "group.set", accountTarget(req.AccountId), fmt.Sprintf("%s=%v", req.Group, req.Member))
	return &emptypb.Empty{}, nil
}

func (h *grpcHandler) SetAccountStatus(ctx context.Context, req *accountpb.SetAccountStatusRequest) (*emptypb.Empty, error) {
	if err := h.store.SetAccountStatus(ctx, req.AccountId, req.Status); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, status.Error(codes.NotFound, "account not found")
		}
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if req.Status == StatusDisabled {
		// Disabling an account kills its portal sessions too.
		if err := h.store.DeleteAccountSessions(ctx, req.AccountId); err != nil {
			return nil, status.Errorf(codes.Internal, "clear sessions: %v", err)
		}
	}
	h.audit(ctx, "account.status", accountTarget(req.AccountId), req.Status)
	return &emptypb.Empty{}, nil
}

func (h *grpcHandler) UnlinkIdentity(ctx context.Context, req *accountpb.UnlinkIdentityRequest) (*emptypb.Empty, error) {
	if err := h.store.RevokeIdentity(ctx, req.AccountId, req.Provider); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, status.Error(codes.NotFound, "no active link for that provider")
		}
		return nil, status.Errorf(codes.Internal, "revoke: %v", err)
	}
	h.audit(ctx, "identity.unlink", accountTarget(req.AccountId), req.Provider)
	return &emptypb.Empty{}, nil
}

func (h *grpcHandler) ReleaseIdentity(ctx context.Context, req *accountpb.ReleaseIdentityRequest) (*emptypb.Empty, error) {
	if err := h.store.ReleaseIdentity(ctx, req.Provider, req.ProviderUserId); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, status.Error(codes.NotFound, "identity not found")
		}
		return nil, status.Errorf(codes.Internal, "release: %v", err)
	}
	h.audit(ctx, "identity.release", req.Provider+":"+req.ProviderUserId, "")
	return &emptypb.Empty{}, nil
}

// AdminResetPassword mints a 1h single-use reset token and returns the
// portal URL. The portal's /reset-password page (Task 17) consumes it.
func (h *grpcHandler) AdminResetPassword(ctx context.Context, req *accountpb.AdminResetPasswordRequest) (*accountpb.AdminResetPasswordResponse, error) {
	if _, err := h.store.AccountByID(ctx, req.AccountId); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, status.Error(codes.NotFound, "account not found")
		}
		return nil, status.Errorf(codes.Internal, "lookup: %v", err)
	}
	raw, err := NewRawToken()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "token: %v", err)
	}
	if err := h.store.CreateToken(ctx, req.AccountId, TokenPurposeResetPassword, HashToken(raw), time.Hour); err != nil {
		return nil, status.Errorf(codes.Internal, "create token: %v", err)
	}
	h.audit(ctx, "account.reset_password", accountTarget(req.AccountId), "reset link minted")
	return &accountpb.AdminResetPasswordResponse{
		ResetUrl: h.cfg.PublicURL + "/reset-password?token=" + raw,
	}, nil
}

func (h *grpcHandler) BootstrapAdmin(ctx context.Context, req *accountpb.BootstrapAdminRequest) (*emptypb.Empty, error) {
	acct, err := h.store.AccountByEmail(ctx, req.Email)
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "account not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "lookup: %v", err)
	}
	if err := h.store.AddGroupMember(ctx, GroupAdmin, acct.ID, 0); err != nil {
		return nil, status.Errorf(codes.Internal, "add admin: %v", err)
	}
	h.audit(ctx, "group.set", accountTarget(acct.ID), GroupAdmin+"=true (bootstrap)")
	return &emptypb.Empty{}, nil
}
```

Also update `modules/account/grpc_test.go`'s "good bearer" assertion from Task 8 to require plain success (`err != nil` fails the test) now that SearchAccounts is implemented.

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/account/grpc_admin.go modules/account/grpc_admin_test.go \
        modules/account/store.go modules/account/grpc_test.go
git commit --no-gpg-sign -m "feat(account): admin gRPC surface — search/get/groups/status/identity/reset/bootstrap

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 10: Module assembly + app wiring

**Files:**
- Create: `modules/account/account.go` (module lifecycle)
- Create: `modules/account/portal.go` (portal skeleton: struct + routes; extended in Tasks 13-20)
- Create: `modules/account/mail.go` (Mailer interface + logMailer; SMTP impl arrives in Task 16)
- Modify: `cmd/goscape/app/modules.go` (register module, deps, `all` target, database-anchor gate)
- Modify: `cmd/goscape/app/config.go` (Account section + validate fan-out)
- Modify: `cmd/goscape/app/app.go` (add `account *account.Account` field next to `login`/`friends`)
- Modify: `examples/full-config-reference.yaml` (full `account:` section at defaults)
- Modify: `CLAUDE.md` (module graph table + `account` target line)
- Test: `modules/account/account_test.go`

**Interfaces:**
- Consumes: Tasks 2-9, `pkg/dskit/services`, `pkg/gamedb`, app wiring patterns from `initLogin`.
- Produces:

```go
// modules/account
func New(cfg Config, dbCfg gamedb.Config, logger *slog.Logger) (*Account, error) // *Account embeds services.Service
type Mailer interface{ Send(to, subject, body string) error }
func newLogMailer(log *slog.Logger) Mailer // used when smtp.host is empty: logs instead of sending
func newPortal(cfg Config, store *Store, mailer Mailer, log *slog.Logger) (*portal, error)
func (p *portal) routes() *http.ServeMux
// app: const Account = "account"; deps Account:{Common, Database}; SingleBinary includes Account
```

- [ ] **Step 1: Write the failing test**

```go
// modules/account/account_test.go
package account

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/dskit/services"
	"github.com/zsrv/goscape/pkg/gamedb"
)

func freePort(t *testing.T) int {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()
	return lis.Addr().(*net.TCPAddr).Port
}

func TestAccountModule_StartServeStop(t *testing.T) {
	dir := t.TempDir()
	var dbCfg gamedb.Config
	dbCfg.Backend = gamedb.BackendSQLite
	dbCfg.SQLite.DSN = filepath.Join(dir, "test.db")

	// The database module migrates before dependents start; mirror that.
	db, err := gamedb.Open(dbCfg, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	db.Close()

	cfg := defaultConfig(t)
	cfg.Enable = true
	cfg.HTTPListenPort = freePort(t)
	cfg.GRPCListenPort = freePort(t)
	cfg.PublicURL = fmt.Sprintf("http://127.0.0.1:%d", cfg.HTTPListenPort)

	a, err := New(cfg, dbCfg, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	// StartAndAwaitRunning/StopAndAwaitTerminated are the dskit service
	// helpers; check pkg/dskit/services for the exact names in this port
	// before writing (grep 'func StartAndAwaitRunning').
	if err := services.StartAndAwaitRunning(t.Context(), a); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		if err := services.StopAndAwaitTerminated(t.Context(), a); err != nil {
			t.Fatalf("stop: %v", err)
		}
	}()

	// Portal answers.
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", cfg.HTTPListenPort))
	if err != nil {
		t.Fatalf("healthz: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status = %d", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -run TestAccountModule -v`
Expected: FAIL to build — `New` undefined.

- [ ] **Step 3: Write mail.go, portal.go skeleton, account.go**

```go
// modules/account/mail.go
package account

import "log/slog"

// Mailer sends portal email (verification, password reset). The SMTP
// implementation lands with the email flows (Task 16); logMailer is
// the no-SMTP fallback so a portal without smtp.host still functions —
// links are visible in the server log.
type Mailer interface {
	Send(to, subject, body string) error
}

type logMailer struct{ log *slog.Logger }

func newLogMailer(log *slog.Logger) Mailer { return &logMailer{log: log} }

func (m *logMailer) Send(to, subject, body string) error {
	m.log.Info("outbound mail (smtp.host not configured — logging instead)",
		slog.String("to", to), slog.String("subject", subject), slog.String("body", body))
	return nil
}
```

```go
// modules/account/portal.go
package account

import (
	"log/slog"
	"net/http"
)

// portal is the SSR web application. Handlers hang off this struct and
// are registered in routes(); later tasks add templates, session
// middleware, and the page handlers in sibling files.
type portal struct {
	cfg    Config
	store  *Store
	mailer Mailer
	log    *slog.Logger
}

func newPortal(cfg Config, store *Store, mailer Mailer, log *slog.Logger) (*portal, error) {
	return &portal{cfg: cfg, store: store, mailer: mailer, log: log}, nil
}

func (p *portal) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}
```

```go
// modules/account/account.go
// Package account is the account-management module: portal accounts as
// containers for game characters, third-party identity linking, the
// character-creation gate, an SSR player portal, and the AccountService
// gRPC surface consumed by the login module and goscape-cli.
// Spec: docs/superpowers/specs/2026-07-19-account-management-design.md
package account

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"google.golang.org/grpc"

	"github.com/zsrv/goscape/pkg/dskit/services"
	"github.com/zsrv/goscape/pkg/gamedb"
)

// Account is the module. Like login, it owns a private pool to the
// central database; unlike login it runs two listeners (portal HTTP +
// AccountService gRPC).
type Account struct {
	services.Service

	cfg   Config
	dbCfg gamedb.Config
	log   *slog.Logger

	db      *gamedb.DB
	store   *Store
	grpcSrv *grpc.Server
	grpcLis net.Listener
	httpSrv *http.Server
	httpLis net.Listener
}

func New(cfg Config, dbCfg gamedb.Config, logger *slog.Logger) (*Account, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	a := &Account{cfg: cfg, dbCfg: dbCfg, log: logger}
	a.Service = services.NewBasicService(a.starting, a.running, a.stopping)
	return a, nil
}

func (a *Account) starting(ctx context.Context) error {
	db, err := gamedb.Open(a.dbCfg, a.log)
	if err != nil {
		return fmt.Errorf("open central database: %w", err)
	}
	store := NewStore(db)

	mailer := newLogMailer(a.log)
	if a.cfg.SMTP.Host != "" {
		mailer = newSMTPMailer(a.cfg.SMTP) // Task 16; until then keep this branch commented out
	}
	p, err := newPortal(a.cfg, store, mailer, a.log)
	if err != nil {
		db.Close()
		return fmt.Errorf("portal: %w", err)
	}

	grpcSrv := newGRPCServer(a.cfg, store, a.log)
	grpcAddr := fmt.Sprintf("%s:%d", a.cfg.GRPCListenAddress, a.cfg.GRPCListenPort)
	grpcLis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		db.Close()
		return fmt.Errorf("grpc listen %s: %w", grpcAddr, err)
	}

	httpAddr := fmt.Sprintf("%s:%d", a.cfg.HTTPListenAddress, a.cfg.HTTPListenPort)
	httpLis, err := net.Listen("tcp", httpAddr)
	if err != nil {
		grpcLis.Close()
		db.Close()
		return fmt.Errorf("http listen %s: %w", httpAddr, err)
	}

	a.db = db
	a.store = store
	a.grpcSrv = grpcSrv
	a.grpcLis = grpcLis
	a.httpSrv = &http.Server{Handler: p.routes(), ReadHeaderTimeout: 10 * time.Second}
	a.httpLis = httpLis
	a.log.Info("account module listening",
		slog.String("http", httpAddr), slog.String("grpc", grpcAddr))
	return nil
}

func (a *Account) running(ctx context.Context) error {
	grpcDone := make(chan error, 1)
	httpDone := make(chan error, 1)
	grpcLis, httpLis := a.grpcLis, a.httpLis
	a.grpcLis, a.httpLis = nil, nil // servers own the listeners now

	go func() { grpcDone <- a.grpcSrv.Serve(grpcLis) }()
	go func() {
		if err := a.httpSrv.Serve(httpLis); !errors.Is(err, http.ErrServerClosed) {
			httpDone <- err
			return
		}
		httpDone <- nil
	}()

	shutdown := func() {
		shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.httpSrv.Shutdown(shCtx)
		a.grpcSrv.GracefulStop()
	}

	select {
	case <-ctx.Done():
		shutdown()
		<-grpcDone
		<-httpDone
		return nil
	case err := <-grpcDone:
		shutdown()
		<-httpDone
		if err != nil {
			return fmt.Errorf("account grpc server: %w", err)
		}
		return nil
	case err := <-httpDone:
		shutdown()
		<-grpcDone
		if err != nil {
			return fmt.Errorf("account http server: %w", err)
		}
		return nil
	}
}

func (a *Account) stopping(_ error) error {
	// Edge case: StopAsync between starting() and running() — the
	// servers never took listener ownership.
	if a.grpcLis != nil {
		a.grpcLis.Close()
	}
	if a.httpLis != nil {
		a.httpLis.Close()
	}
	if a.db != nil {
		a.db.Close()
	}
	return nil
}
```

Note: in THIS task, `newSMTPMailer` does not exist yet — write the starting() mailer block as:

```go
	var mailer Mailer = newLogMailer(a.log)
```

and Task 16 upgrades it to the conditional shown above.

- [ ] **Step 4: Wire the app**

`cmd/goscape/app/modules.go` — add the const, init func, registration, deps:

```go
// with the other target consts
	Account  string = "account"

// new init function, mirroring initLogin
func (g *App) initAccount() (services.Service, error) {
	if !g.cfg.Account.Enable {
		g.logger.Info("module disabled", "module", "account")
		return nil, nil
	}

	logLevel := slog.Level(g.cfg.LogLevel)
	if g.cfg.Account.LogLevel != nil {
		logLevel = slog.Level(*g.cfg.Account.LogLevel)
	}
	logger, err := log.NewLogger(logLevel, g.cfg.LogFormat, os.Stdout, log.WithSourceFormat(g.cfg.LogSource))
	if err != nil {
		return nil, fmt.Errorf("failed to create account logger: %w", err)
	}
	logger = logger.With("component", "account")

	a, err := account.New(g.cfg.Account, g.cfg.Database, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create account: %w", err)
	}
	g.account = a
	return g.account, nil
}
```

In `setupModuleManager`: `mm.RegisterModule(Account, g.initAccount)`; deps `Account: {Common, Database}`; `SingleBinary: {OnDemand, Friends, Login, World, Account}`.

In `initDatabase`, extend the no-consumer gate:

```go
	if !g.cfg.Login.Enable && !g.cfg.Friends.Enable && !g.cfg.Account.Enable {
```

`cmd/goscape/app/config.go` — add `Account account.Config \`yaml:"account,omitempty"\`` to `Config`, call `c.Account.RegisterFlagsAndApplyDefaults(f)` in RegisterFlagsAndApplyDefaults, and in Validate add (self-prefixing, so unwrapped):

```go
	if err := c.Account.Validate(); err != nil {
		return err
	}
```

`cmd/goscape/app/app.go` — add `account *account.Account` to the App struct fields next to `login *login.Login` (read the struct first; keep field order style).

`examples/full-config-reference.yaml` — append a full `account:` section with every key at its default and the same comment style as other sections (enable false, addresses 127.0.0.1, ports 8081/2005, public_url "", character_limit 5, admin_token "", gate.providers [discord], argon2 {memory_kib 65536, time 2, parallelism 1}, session {idle_ttl 168h, absolute_ttl 720h}, smtp {host "", port 587, from "", username "", password ""}, providers.discord {client_id "", client_secret ""}).

`CLAUDE.md` — add `account` to the `--target` list and to the module dependency table: `account   portal + AccountService gRPC   → common, database`, and note `all` now includes it.

- [ ] **Step 5: Run tests + boot verify**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ ./cmd/goscape/... -v -run 'TestAccountModule|TestConfig'`
Expected: PASS.
Run: `CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape --config.file examples/bundled/goscape.yaml --config.verify=true`
Expected: exits 0 (account disabled by default; existing config still valid).
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run '^$' ./...`
Expected: everything still compiles (compile-all gate).

- [ ] **Step 6: Commit**

```bash
git add modules/account/account.go modules/account/portal.go modules/account/mail.go \
        modules/account/account_test.go cmd/goscape/app/modules.go cmd/goscape/app/config.go \
        cmd/goscape/app/app.go examples/full-config-reference.yaml CLAUDE.md
git commit --no-gpg-sign -m "feat(account): module lifecycle + app wiring (portal HTTP + gRPC listeners)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 11: Login module — auth_mode delegation

**Files:**
- Modify: `modules/login/config.go` (AuthMode, AccountGRPCAddress + Validate)
- Modify: `modules/login/login.go` (dial account service in starting when mode=account)
- Modify: `modules/login/server.go` (pass account client into handler)
- Modify: `modules/login/db.go` (add `accountByID`)
- Modify: `modules/login/handler.go` (delegation branch in PlayerLogin)
- Test: `modules/login/handler_account_test.go`

**Interfaces:**
- Consumes: `pkg/accountpb` (Task 7). The account MODULE is not imported — only its proto package.
- Produces:

```go
const (
	AuthModeLocal   = "local"
	AuthModeAccount = "account"
)
// Config gains: AuthMode string `yaml:"auth_mode"`, AccountGRPCAddress string `yaml:"account_grpc_address"`
// handler gains: acct accountpb.AccountServiceClient (nil in local mode)
func accountByID(ctx context.Context, db *gamedb.DB, id int64, profile string) (*accountRow, error)
```

- [ ] **Step 1: Write the failing test**

```go
// modules/login/handler_account_test.go
package login

import (
	"context"
	"errors"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/zsrv/goscape/pkg/accountpb"
	"github.com/zsrv/goscape/pkg/loginpb"
)

// stubAccountService returns canned VerifyGameLogin responses and
// records the request the login handler sent.
type stubAccountService struct {
	accountpb.UnimplementedAccountServiceServer
	resp   *accountpb.VerifyGameLoginResponse
	err    error
	gotReq *accountpb.VerifyGameLoginRequest
}

func (s *stubAccountService) VerifyGameLogin(_ context.Context, req *accountpb.VerifyGameLoginRequest) (*accountpb.VerifyGameLoginResponse, error) {
	s.gotReq = req
	return s.resp, s.err
}

func stubAccountClient(t *testing.T, stub *stubAccountService) accountpb.AccountServiceClient {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	accountpb.RegisterAccountServiceServer(srv, stub)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return accountpb.NewAccountServiceClient(conn)
}

func TestConfig_AuthModeValidation(t *testing.T) {
	base := func() Config {
		c := newTestConfig(t) // reuse the existing test-config helper in this package;
		// if none exists, build via RegisterFlagsAndApplyDefaults exactly like
		// modules/account/config_test.go defaultConfig.
		c.Enable = true
		return c
	}
	c := base()
	c.AuthMode = "bogus"
	if err := c.Validate(); err == nil {
		t.Fatal("bogus auth_mode must fail validation")
	}
	c = base()
	c.AuthMode = AuthModeAccount
	c.AccountGRPCAddress = ""
	c.AutoRegister = false
	if err := c.Validate(); err == nil {
		t.Fatal("account mode without address must fail validation")
	}
	c = base()
	c.AuthMode = AuthModeAccount
	c.AccountGRPCAddress = "127.0.0.1:2005"
	c.AutoRegister = true
	if err := c.Validate(); err == nil {
		t.Fatal("account mode + auto_register must be a config conflict")
	}
	c = base()
	c.AuthMode = AuthModeAccount
	c.AccountGRPCAddress = "127.0.0.1:2005"
	c.AutoRegister = false
	if err := c.Validate(); err != nil {
		t.Fatalf("valid account mode rejected: %v", err)
	}
}

func TestPlayerLogin_AccountMode(t *testing.T) {
	// newTestHandler is the existing helper pattern in handler_test.go —
	// it builds handler{db, cfg, log} over a migrated sqlite DB and a
	// t.TempDir() save path. Reuse it; only the fields below differ.
	h, db := newTestHandler(t)
	h.cfg.AuthMode = AuthModeAccount
	h.cfg.AutoRegister = false
	ctx := t.Context()

	// Seed the game account row the way portal character creation does.
	if _, err := db.ExecContext(ctx, db.Rebind(
		`INSERT INTO account (username, password, registration_ip) VALUES ('zezima', '!portal-managed!', 'portal')`)); err != nil {
		t.Fatal(err)
	}
	var gameID int64
	if err := db.QueryRowContext(ctx, db.Rebind(`SELECT id FROM account WHERE username = 'zezima'`)).Scan(&gameID); err != nil {
		t.Fatal(err)
	}

	req := func() *loginpb.PlayerLoginRequest {
		return &loginpb.PlayerLoginRequest{
			NodeId: 10, Profile: "main", Username: "zezima",
			Password: "MixedCase!", RemoteAddress: "9.9.9.9:1", Uid: 7,
		}
	}

	// OK path: delegated verify succeeds → NEW_PLAYER (no save on disk),
	// and the password reaches the account service VERBATIM (no
	// lowercasing — that quirk is local-mode only).
	stub := &stubAccountService{resp: &accountpb.VerifyGameLoginResponse{
		Result: accountpb.VerifyResult_VERIFY_RESULT_OK, GameAccountId: gameID, PortalAccountId: 1}}
	h.acct = stubAccountClient(t, stub)
	resp, err := h.PlayerLogin(ctx, req())
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if resp.Result != loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER {
		t.Fatalf("result = %v, want NEW_PLAYER", resp.Result)
	}
	if stub.gotReq.Password != "MixedCase!" || stub.gotReq.CharacterName != "zezima" {
		t.Fatalf("delegated request: %+v", stub.gotReq)
	}

	// Result mapping table.
	cases := []struct {
		verify accountpb.VerifyResult
		want   loginpb.LoginResult
	}{
		{accountpb.VerifyResult_VERIFY_RESULT_INVALID_CREDENTIALS, loginpb.LoginResult_LOGIN_RESULT_INVALID_CREDENTIALS},
		{accountpb.VerifyResult_VERIFY_RESULT_ACCOUNT_DISABLED, loginpb.LoginResult_LOGIN_RESULT_ACCOUNT_DISABLED},
		{accountpb.VerifyResult_VERIFY_RESULT_EMAIL_UNVERIFIED, loginpb.LoginResult_LOGIN_RESULT_ACCOUNT_DISABLED},
	}
	for _, tc := range cases {
		stub.resp = &accountpb.VerifyGameLoginResponse{Result: tc.verify}
		resp, err := h.PlayerLogin(ctx, req())
		if err != nil {
			t.Fatalf("%v: %v", tc.verify, err)
		}
		if resp.Result != tc.want {
			t.Fatalf("%v → %v, want %v", tc.verify, resp.Result, tc.want)
		}
	}

	// Transport failure → gRPC error (world maps to login-server-offline).
	stub.resp = nil
	stub.err = status.Error(codes.Unavailable, "down")
	if _, err := h.PlayerLogin(ctx, req()); status.Code(err) != codes.Unavailable {
		t.Fatalf("transport failure: got %v, want Unavailable", err)
	}

	// IP ban still runs BEFORE delegation.
	stub.err = errors.New("must not be called")
	if _, err := db.ExecContext(ctx, db.Rebind(`INSERT INTO ipban (ip) VALUES ('9.9.9.9')`)); err != nil {
		t.Fatal(err)
	}
	stub.gotReq = nil
	resp, err = h.PlayerLogin(ctx, req())
	if err != nil || resp.Result != loginpb.LoginResult_LOGIN_RESULT_IP_BANNED {
		t.Fatalf("ip ban: %v %v", resp, err)
	}
	if stub.gotReq != nil {
		t.Fatal("account service must not be consulted for IP-banned logins")
	}
}
```

Adjust helper names (`newTestHandler`, `newTestConfig`) to whatever `modules/login/handler_test.go` actually provides — read that file first and reuse its fixtures; do NOT duplicate fixture code. The `ipban` insert must match the existing schema (check `000001_init.up.sql` for its columns; it may need more values).

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ -run 'AuthMode|AccountMode' -v`
Expected: FAIL to build — `AuthModeAccount`, `h.acct` undefined.

- [ ] **Step 3: Implement**

`modules/login/config.go` — add fields + flags + validation:

```go
// In Config struct:
	// AuthMode selects credential verification: "local" (default) is the
	// TS-faithful path — bcrypt against account.password with the
	// lowercase quirk, auto-register honored. "account" delegates to the
	// account module's VerifyGameLogin gRPC (character name + portal
	// account password, case-sensitive, no auto-register).
	AuthMode           string `yaml:"auth_mode"`
	AccountGRPCAddress string `yaml:"account_grpc_address"`

// In RegisterFlagsAndApplyDefaults:
	f.StringVar(&c.AuthMode, "login.auth-mode", AuthModeLocal, "Credential verification mode: local (TS-faithful bcrypt) or account (delegate to account module).")
	f.StringVar(&c.AccountGRPCAddress, "login.account-grpc-address", "", "AccountService gRPC address (host:port). Required when login.auth-mode=account.")

// In Validate (after the existing checks):
	switch c.AuthMode {
	case AuthModeLocal:
	case AuthModeAccount:
		if c.AccountGRPCAddress == "" {
			return fmt.Errorf("login: account_grpc_address must be set when auth_mode=account")
		}
		if c.AutoRegister {
			return fmt.Errorf("login: auto_register=true conflicts with auth_mode=account (accounts are created by the portal)")
		}
	default:
		return fmt.Errorf("login: auth_mode must be one of [local, account], got %q", c.AuthMode)
	}
```

And near the top of the file:

```go
const (
	AuthModeLocal   = "local"
	AuthModeAccount = "account"
)
```

`modules/login/login.go` — dial in starting, close in stopping (mirror how the world module dials login; use the same dial options as `stubAccountClient` minus bufconn):

```go
// struct fields added to Login:
	acctConn *grpc.ClientConn

// in starting(), after opening the DB and before newGRPCServer:
	var acct accountpb.AccountServiceClient
	if l.cfg.AuthMode == AuthModeAccount {
		conn, err := grpc.NewClient(l.cfg.AccountGRPCAddress,
			grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			db.Close()
			return fmt.Errorf("dial account service: %w", err)
		}
		l.acctConn = conn
		acct = accountpb.NewAccountServiceClient(conn)
	}
	srv := newGRPCServer(l.cfg, db, acct, l.log)

// in stopping(), before db.Close():
	if l.acctConn != nil {
		l.acctConn.Close()
	}
```

`modules/login/server.go` — thread the client through:

```go
func newGRPCServer(cfg Config, db *gamedb.DB, acct accountpb.AccountServiceClient, log *slog.Logger) *grpcServer {
	...
	loginpb.RegisterLoginServiceServer(s, &handler{
		db:   db,
		cfg:  cfg,
		acct: acct,
		log:  log,
	})
```

`modules/login/db.go` — add:

```go
// accountByID is accountByUsername keyed on account.id — used by the
// account-mode login path, which learns the row id from
// VerifyGameLogin instead of trusting the wire username.
func accountByID(ctx context.Context, db *gamedb.DB, id int64, profile string) (*accountRow, error) {
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
WHERE a.id = ?`
	// Scan block identical to accountByUsername — extract the shared
	// row-scan into a small scanAccountRow helper rather than
	// duplicating it.
	...
}
```

(Refactor `accountByUsername` and `accountByID` to share one `scanAccountRow(row *sql.Row) (*accountRow, error)` helper.)

`modules/login/handler.go` — add the field and the branch. Replace the section between the IP-ban check (step 2) and the ban check (step 5) as follows:

```go
// handler struct gains:
	acct accountpb.AccountServiceClient // non-nil only when cfg.AuthMode == AuthModeAccount

// PlayerLogin: steps 3-4 become mode-dispatched.
	var account *accountRow
	if h.cfg.AuthMode == AuthModeAccount {
		// 3/4 (account mode): delegate credential verification to the
		// account module. Password travels VERBATIM — the TS lowercase
		// quirk is local-mode-only (spec: config-gated divergence).
		vctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		vresp, verr := h.acct.VerifyGameLogin(vctx, &accountpb.VerifyGameLoginRequest{
			CharacterName: req.Username,
			Password:      req.Password,
			RemoteAddress: req.RemoteAddress,
		})
		cancel()
		if verr != nil {
			// Transport/deadline failure: surface a gRPC error so the
			// world maps it to login-server-offline, exactly like a dead
			// login server.
			return nil, status.Errorf(codes.Unavailable, "account service: %v", verr)
		}
		switch vresp.Result {
		case accountpb.VerifyResult_VERIFY_RESULT_OK:
			// fall through to row load below
		case accountpb.VerifyResult_VERIFY_RESULT_INVALID_CREDENTIALS:
			return &loginpb.PlayerLoginResponse{
				Result: loginpb.LoginResult_LOGIN_RESULT_INVALID_CREDENTIALS,
			}, nil
		case accountpb.VerifyResult_VERIFY_RESULT_ACCOUNT_DISABLED,
			accountpb.VerifyResult_VERIFY_RESULT_EMAIL_UNVERIFIED:
			// Both render as the client's "account disabled" screen; the
			// portal dashboard is where the player learns which it was.
			return &loginpb.PlayerLoginResponse{
				Result: loginpb.LoginResult_LOGIN_RESULT_ACCOUNT_DISABLED,
			}, nil
		default:
			return nil, status.Errorf(codes.Internal, "account service returned %v", vresp.Result)
		}
		var aerr error
		account, aerr = accountByID(ctx, h.db, vresp.GameAccountId, req.Profile)
		if aerr != nil {
			return nil, status.Errorf(codes.Internal, "accountByID: %v", aerr)
		}
		if account == nil {
			// Portal creation inserts the game row in the same tx as the
			// character, so this indicates DB divergence — refuse.
			return nil, status.Errorf(codes.Internal, "account row %d missing for verified character %q", vresp.GameAccountId, req.Username)
		}
	} else {
		// 3/4 (local mode): UNCHANGED existing lookup / auto-register /
		// bcrypt-compare block, verbatim.
		...existing code...
	}
```

Everything from step 5 (`// 5. Ban check`) onward is untouched and uses `account`.

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ -v`
Expected: PASS — including every pre-existing test unmodified (local-mode regression gate). If any existing test needed changes beyond the `newGRPCServer` signature threading, that is a defect in this task.

- [ ] **Step 5: Update examples + commit**

Add to `examples/full-config-reference.yaml` under `login:`: `auth_mode: local` and `account_grpc_address: ""` with comments explaining both modes and the auto_register conflict.

```bash
git add modules/login/config.go modules/login/login.go modules/login/server.go \
        modules/login/db.go modules/login/handler.go modules/login/handler_account_test.go \
        examples/full-config-reference.yaml
git commit --no-gpg-sign -m "feat(login): auth_mode=account — delegate game-login auth to account module

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 12: goscape-cli `account` verb group

**Files:**
- Create: `cmd/goscape-cli/cmd_account.go`
- Modify: `cmd/goscape-cli/main.go` (append one entry to `verbs`)
- Test: `cmd/goscape-cli/cmd_account_test.go`

**Interfaces:**
- Consumes: `pkg/accountpb` (Task 7).
- Produces: `runAccount(args []string, stdout, stderr io.Writer) int` following the existing verbHandler signature.

CLI shape (global flags first, then subcommand):

```
goscape-cli account -addr 127.0.0.1:2005 [-token T] <subcommand> [args]
  search <query>                                  list matching accounts
  show <account-id|email>                         full account detail
  approve <account-id>    / unapprove <account-id>   manually_approved on/off
  disable <account-id>    / enable <account-id>      status
  unlink <account-id> <provider>                  soft-revoke (burn) a linked identity
  release-identity <provider> <provider-user-id>  hard-delete (free) an identity
  reset-password <account-id>                     mint + print a reset URL
  bootstrap-admin <email>                         add account to the admin group
Token: -token flag, else GOSCAPE_ACCOUNT_TOKEN env var.
```

- [ ] **Step 1: Write the failing test**

```go
// cmd/goscape-cli/cmd_account_test.go
package main

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/zsrv/goscape/pkg/accountpb"
)

// recordingAccountService captures metadata + requests, returns canned data.
type recordingAccountService struct {
	accountpb.UnimplementedAccountServiceServer
	gotAuth []string
}

func (s *recordingAccountService) record(ctx context.Context) {
	md, _ := metadata.FromIncomingContext(ctx)
	s.gotAuth = md.Get("authorization")
}

func (s *recordingAccountService) SearchAccounts(ctx context.Context, req *accountpb.SearchAccountsRequest) (*accountpb.SearchAccountsResponse, error) {
	s.record(ctx)
	return &accountpb.SearchAccountsResponse{Accounts: []*accountpb.AccountSummary{{
		Id: 7, Email: "a@example.com", EmailVerified: true, Status: "active", CharacterCount: 2,
	}}}, nil
}

func (s *recordingAccountService) SetGroupMembership(ctx context.Context, req *accountpb.SetGroupMembershipRequest) (*emptypb.Empty, error) {
	s.record(ctx)
	return &emptypb.Empty{}, nil
}

func (s *recordingAccountService) AdminResetPassword(ctx context.Context, req *accountpb.AdminResetPasswordRequest) (*accountpb.AdminResetPasswordResponse, error) {
	s.record(ctx)
	return &accountpb.AdminResetPasswordResponse{ResetUrl: "http://portal/reset-password?token=abc"}, nil
}

func startStubAccountServer(t *testing.T) (addr string, stub *recordingAccountService) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stub = &recordingAccountService{}
	srv := grpc.NewServer()
	accountpb.RegisterAccountServiceServer(srv, stub)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String(), stub
}

func TestAccountVerb_SearchAndAuth(t *testing.T) {
	addr, stub := startStubAccountServer(t)
	var out, errOut bytes.Buffer
	code := runAccount([]string{"-addr", addr, "-token", "sekrit", "search", "a@"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "a@example.com") || !strings.Contains(out.String(), "7") {
		t.Fatalf("output: %q", out.String())
	}
	if len(stub.gotAuth) != 1 || stub.gotAuth[0] != "Bearer sekrit" {
		t.Fatalf("auth metadata: %v", stub.gotAuth)
	}
}

func TestAccountVerb_ApproveAndResetPassword(t *testing.T) {
	addr, _ := startStubAccountServer(t)
	var out, errOut bytes.Buffer
	if code := runAccount([]string{"-addr", addr, "-token", "x", "approve", "7"}, &out, &errOut); code != 0 {
		t.Fatalf("approve exit=%d stderr=%s", code, errOut.String())
	}
	out.Reset()
	if code := runAccount([]string{"-addr", addr, "-token", "x", "reset-password", "7"}, &out, &errOut); code != 0 {
		t.Fatalf("reset exit=%d stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "http://portal/reset-password?token=abc") {
		t.Fatalf("reset output: %q", out.String())
	}
}

func TestAccountVerb_UsageErrors(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runAccount(nil, &out, &errOut); code != 2 {
		t.Fatalf("no args: exit=%d", code)
	}
	if code := runAccount([]string{"-addr", "127.0.0.1:1", "frobnicate"}, &out, &errOut); code != 2 {
		t.Fatalf("unknown sub: exit=%d", code)
	}
	if code := runAccount([]string{"-addr", "127.0.0.1:1", "approve", "not-a-number"}, &out, &errOut); code != 2 {
		t.Fatalf("bad id: exit=%d", code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/ -run TestAccountVerb -v`
Expected: FAIL to build — `runAccount` undefined.

- [ ] **Step 3: Implement cmd_account.go**

```go
// cmd/goscape-cli/cmd_account.go
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/zsrv/goscape/pkg/accountpb"
)

// runAccount is the `account` verb: a thin client over the
// AccountService admin gRPC surface (modules/account). Requires the
// server's account.admin_token via -token or GOSCAPE_ACCOUNT_TOKEN.
func runAccount(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("account", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addr := fs.String("addr", "127.0.0.1:2005", "AccountService gRPC address")
	token := fs.String("token", "", "admin bearer token (default: GOSCAPE_ACCOUNT_TOKEN env)")
	fs.Usage = func() {
		fmt.Fprintln(stderr, `usage: goscape-cli account [-addr host:port] [-token T] <subcommand> [args]

subcommands:
  search <query>                                  list matching accounts
  show <account-id|email>                         full account detail
  approve <account-id> | unapprove <account-id>   manually_approved on/off
  disable <account-id> | enable <account-id>      account status
  unlink <account-id> <provider>                  soft-revoke (burn) a linked identity
  release-identity <provider> <provider-user-id>  hard-delete (free) an identity
  reset-password <account-id>                     mint + print a reset URL
  bootstrap-admin <email>                         add account to the admin group`)
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *token == "" {
		*token = os.Getenv("GOSCAPE_ACCOUNT_TOKEN")
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fs.Usage()
		return 2
	}
	sub, subArgs := rest[0], rest[1:]

	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Fprintf(stderr, "account: dial %s: %v\n", *addr, err)
		return 1
	}
	defer conn.Close()
	client := accountpb.NewAccountServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if *token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+*token)
	}

	parseID := func(s string) (int64, bool) {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil || id < 1 {
			fmt.Fprintf(stderr, "account: %q is not an account id\n", s)
			return 0, false
		}
		return id, true
	}
	need := func(n int) bool {
		if len(subArgs) != n {
			fs.Usage()
			return false
		}
		return true
	}
	setGroup := func(group string, member bool) int {
		if !need(1) {
			return 2
		}
		id, ok := parseID(subArgs[0])
		if !ok {
			return 2
		}
		if _, err := client.SetGroupMembership(ctx, &accountpb.SetGroupMembershipRequest{
			AccountId: id, Group: group, Member: member}); err != nil {
			fmt.Fprintf(stderr, "account: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "account %d: %s=%v\n", id, group, member)
		return 0
	}
	setStatus := func(status string) int {
		if !need(1) {
			return 2
		}
		id, ok := parseID(subArgs[0])
		if !ok {
			return 2
		}
		if _, err := client.SetAccountStatus(ctx, &accountpb.SetAccountStatusRequest{
			AccountId: id, Status: status}); err != nil {
			fmt.Fprintf(stderr, "account: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "account %d: status=%s\n", id, status)
		return 0
	}

	switch sub {
	case "search":
		if !need(1) {
			return 2
		}
		resp, err := client.SearchAccounts(ctx, &accountpb.SearchAccountsRequest{Query: subArgs[0]})
		if err != nil {
			fmt.Fprintf(stderr, "account: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "%-8s %-32s %-9s %-9s %s\n", "ID", "EMAIL", "VERIFIED", "STATUS", "CHARS")
		for _, a := range resp.Accounts {
			fmt.Fprintf(stdout, "%-8d %-32s %-9v %-9s %d\n", a.Id, a.Email, a.EmailVerified, a.Status, a.CharacterCount)
		}
		return 0

	case "show":
		if !need(1) {
			return 2
		}
		req := &accountpb.GetAccountRequest{}
		if strings.Contains(subArgs[0], "@") {
			req.Email = subArgs[0]
		} else {
			id, ok := parseID(subArgs[0])
			if !ok {
				return 2
			}
			req.Id = id
		}
		resp, err := client.GetAccount(ctx, req)
		if err != nil {
			fmt.Fprintf(stderr, "account: %v\n", err)
			return 1
		}
		a := resp.Account
		fmt.Fprintf(stdout, "id: %d\nemail: %s (verified=%v)\nstatus: %s\ngroups: %s\n",
			a.Id, a.Email, a.EmailVerified, a.Status, strings.Join(resp.Groups, ", "))
		for _, id := range resp.Identities {
			state := "linked"
			if id.Revoked {
				state = "REVOKED"
			}
			fmt.Fprintf(stdout, "identity: %s %s (%s) [%s]\n", id.Provider, id.ProviderUserId, id.ProviderUsername, state)
		}
		for _, ch := range resp.Characters {
			fmt.Fprintf(stdout, "character: %s (id=%d game_account=%d)\n", ch.Username, ch.Id, ch.GameAccountId)
		}
		return 0

	case "approve":
		return setGroup("manually_approved", true)
	case "unapprove":
		return setGroup("manually_approved", false)
	case "disable":
		return setStatus("disabled")
	case "enable":
		return setStatus("active")

	case "unlink":
		if !need(2) {
			return 2
		}
		id, ok := parseID(subArgs[0])
		if !ok {
			return 2
		}
		if _, err := client.UnlinkIdentity(ctx, &accountpb.UnlinkIdentityRequest{
			AccountId: id, Provider: subArgs[1]}); err != nil {
			fmt.Fprintf(stderr, "account: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "account %d: %s identity revoked (burned — use release-identity to free it)\n", id, subArgs[1])
		return 0

	case "release-identity":
		if !need(2) {
			return 2
		}
		if _, err := client.ReleaseIdentity(ctx, &accountpb.ReleaseIdentityRequest{
			Provider: subArgs[0], ProviderUserId: subArgs[1]}); err != nil {
			fmt.Fprintf(stderr, "account: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "identity %s:%s released\n", subArgs[0], subArgs[1])
		return 0

	case "reset-password":
		if !need(1) {
			return 2
		}
		id, ok := parseID(subArgs[0])
		if !ok {
			return 2
		}
		resp, err := client.AdminResetPassword(ctx, &accountpb.AdminResetPasswordRequest{AccountId: id})
		if err != nil {
			fmt.Fprintf(stderr, "account: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "%s\n", resp.ResetUrl)
		return 0

	case "bootstrap-admin":
		if !need(1) {
			return 2
		}
		if _, err := client.BootstrapAdmin(ctx, &accountpb.BootstrapAdminRequest{Email: subArgs[0]}); err != nil {
			fmt.Fprintf(stderr, "account: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "%s is now an admin\n", subArgs[0])
		return 0

	default:
		fmt.Fprintf(stderr, "account: unknown subcommand %q\n", sub)
		fs.Usage()
		return 2
	}
}
```

Append to `verbs` in `cmd/goscape-cli/main.go`:

```go
	{"account", runAccount, "Administer portal accounts (search | show | approve | disable | unlink | reset-password | ...)."},
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/ -run TestAccountVerb -v`
Expected: PASS. Also run `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape-cli/` (full package) — the main_test.go usage/dispatch tests must still pass with the new verb row.

- [ ] **Step 5: Commit**

```bash
git add cmd/goscape-cli/cmd_account.go cmd/goscape-cli/cmd_account_test.go cmd/goscape-cli/main.go
git commit --no-gpg-sign -m "feat(cli): account verb — admin client for AccountService

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 13: Portal foundation — embedded templates, render pipeline, home page

**Files:**
- Modify: `modules/account/portal.go` (assets embed, template cache, render, route helpers)
- Create: `modules/account/templates/base.html`, `modules/account/templates/pages/home.html`, `modules/account/templates/pages/message.html`
- Create: `modules/account/static/style.css`
- Test: `modules/account/portal_test.go`

**Interfaces:**
- Consumes: Task 10 portal struct.
- Produces:

```go
//go:embed templates static
var assetsFS embed.FS

type pageData struct {
	Account *PortalAccount // nil when unauthenticated
	CSRF    string         // per-session CSRF token, "" when no session
	Msg     string         // flash message from ?msg= query param
	Data    any            // page-specific payload
}

func (p *portal) render(w http.ResponseWriter, r *http.Request, page string, data any) // page = file name under templates/pages/
// portal struct gains: pages map[string]*template.Template (parsed in newPortal)
// test helper reused by Tasks 14-20:
func newTestPortal(t *testing.T) (*portal, *Store) // portal over openTestStore + recordingMailer
type recordingMailer struct{ mu sync.Mutex; sent []sentMail } // sentMail{To, Subject, Body string}
```

- [ ] **Step 1: Write the failing test**

```go
// modules/account/portal_test.go
package account

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type sentMail struct{ To, Subject, Body string }

// recordingMailer captures outbound mail for flow tests (Tasks 16-17
// pull verification/reset links out of Body).
type recordingMailer struct {
	mu   sync.Mutex
	sent []sentMail
}

func (m *recordingMailer) Send(to, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, sentMail{to, subject, body})
	return nil
}

func (m *recordingMailer) last(t *testing.T) sentMail {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sent) == 0 {
		t.Fatal("no mail sent")
	}
	return m.sent[len(m.sent)-1]
}

func newTestPortal(t *testing.T) (*portal, *Store) {
	t.Helper()
	s := openTestStore(t)
	cfg := defaultConfig(t)
	cfg.Enable = true
	cfg.PublicURL = "http://portal.test"
	cfg.Argon2 = testArgon2()
	p, err := newPortal(cfg, s, &recordingMailer{}, testLogger(t))
	if err != nil {
		t.Fatalf("newPortal: %v", err)
	}
	return p, s
}

func TestPortal_HomeRenders(t *testing.T) {
	p, _ := newTestPortal(t)
	srv := httptest.NewServer(p.routes())
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	body := readBody(t, resp)
	for _, want := range []string{"goscape", "/register", "/login"} {
		if !strings.Contains(body, want) {
			t.Errorf("home missing %q", want)
		}
	}

	// Static assets served from the embed.
	css, err := http.Get(srv.URL + "/static/style.css")
	if err != nil || css.StatusCode != http.StatusOK {
		t.Fatalf("style.css: %v %d", err, css.StatusCode)
	}
	css.Body.Close()

	// Unknown path is a 404, not the home page (the GET /{$} pattern).
	nf, _ := http.Get(srv.URL + "/no-such-page")
	nf.Body.Close()
	if nf.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown path: %d", nf.StatusCode)
	}
}
```

And a shared body helper in `testing_test.go`:

```go
func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -run TestPortal_Home -v`
Expected: FAIL — home route missing / templates absent.

- [ ] **Step 3: Write templates, css, and the render pipeline**

`modules/account/templates/base.html`:

```html
{{define "base"}}<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{block "title" .}}goscape{{end}}</title>
<link rel="stylesheet" href="/static/style.css">
</head>
<body>
<header>
  <a class="brand" href="/">goscape</a>
  <nav>
    {{if .Account}}
      <a href="/dashboard">Dashboard</a>
      <a href="/settings/password">Settings</a>
      <form class="inline" method="post" action="/logout">
        <input type="hidden" name="csrf" value="{{.CSRF}}">
        <button type="submit">Log out</button>
      </form>
    {{else}}
      <a href="/login">Log in</a>
      <a href="/register">Register</a>
    {{end}}
  </nav>
</header>
<main>
{{if .Msg}}<p class="flash">{{.Msg}}</p>{{end}}
{{block "content" .}}{{end}}
</main>
</body>
</html>{{end}}
```

`modules/account/templates/pages/home.html`:

```html
{{define "title"}}goscape — account portal{{end}}
{{define "content"}}
<h1>goscape account portal</h1>
<p>Create an account, link your Discord, and create game characters.</p>
{{if not .Account}}
<p><a class="button" href="/register">Create an account</a></p>
{{else}}
<p><a class="button" href="/dashboard">Go to your dashboard</a></p>
{{end}}
{{end}}
```

`modules/account/templates/pages/message.html` (generic outcome page):

```html
{{define "title"}}goscape{{end}}
{{define "content"}}
<h1>{{.Data}}</h1>
<p><a href="/">Back to the portal</a></p>
{{end}}
```

`modules/account/static/style.css`:

```css
:root { color-scheme: light dark; font-family: system-ui, sans-serif; }
body { margin: 0; }
header { display: flex; justify-content: space-between; align-items: center;
         padding: 0.75rem 1.25rem; border-bottom: 1px solid #8884; }
header .brand { font-weight: 700; text-decoration: none; }
header nav { display: flex; gap: 1rem; align-items: center; }
header nav form.inline { display: inline; }
main { max-width: 46rem; margin: 1.5rem auto; padding: 0 1.25rem; }
.flash { padding: 0.6rem 0.9rem; border: 1px solid #8886; border-radius: 6px; }
.error { color: #c0392b; }
form.stack { display: grid; gap: 0.75rem; max-width: 24rem; }
form.stack label { display: grid; gap: 0.25rem; }
table { border-collapse: collapse; width: 100%; }
th, td { text-align: left; padding: 0.4rem 0.6rem; border-bottom: 1px solid #8883; }
button, .button { padding: 0.45rem 0.9rem; cursor: pointer; }
.muted { opacity: 0.7; }
```

Update `modules/account/portal.go`:

```go
// modules/account/portal.go
package account

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
)

//go:embed templates static
var assetsFS embed.FS

type portal struct {
	cfg    Config
	store  *Store
	mailer Mailer
	log    *slog.Logger
	pages  map[string]*template.Template
	rl     *rateLimiter // Task 14
}

type pageData struct {
	Account *PortalAccount
	CSRF    string
	Msg     string
	Data    any
}

func newPortal(cfg Config, store *Store, mailer Mailer, log *slog.Logger) (*portal, error) {
	pageFiles, err := fs.Glob(assetsFS, "templates/pages/*.html")
	if err != nil {
		return nil, fmt.Errorf("glob pages: %w", err)
	}
	pages := make(map[string]*template.Template, len(pageFiles))
	for _, f := range pageFiles {
		t, err := template.ParseFS(assetsFS, "templates/base.html", f)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		pages[path.Base(f)] = t
	}
	return &portal{
		cfg: cfg, store: store, mailer: mailer, log: log,
		pages: pages,
		rl:    newRateLimiter(), // Task 14; until then omit this field entirely
	}, nil
}

// render executes a page template inside base.html. Errors after the
// header is written are unrecoverable, so pages render to a buffer
// first.
func (p *portal) render(w http.ResponseWriter, r *http.Request, page string, data any) {
	pd := pageData{Account: ctxAccount(r), Msg: r.URL.Query().Get("msg"), Data: data}
	if c, err := r.Cookie(sessionCookieName); err == nil {
		pd.CSRF = csrfToken(c.Value)
	}
	tmpl, ok := p.pages[page]
	if !ok {
		p.log.Error("unknown page template", slog.String("page", page))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "base", pd); err != nil {
		p.log.Error("render failed", slog.String("page", page), slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}

func (p *portal) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("GET /static/", http.FileServerFS(assetsFS))
	mux.HandleFunc("GET /{$}", p.public(p.handleHome))
	// Tasks 14-20 register the remaining routes here.
	return mux
}

func (p *portal) handleHome(w http.ResponseWriter, r *http.Request) {
	p.render(w, r, "home.html", nil)
}
```

NOTE for this task: `ctxAccount`, `sessionCookieName`, `csrfToken`, `p.public`, and `newRateLimiter` arrive in Task 14. To keep THIS task compiling standalone, add temporary minimal forms in portal.go — `func ctxAccount(*http.Request) *PortalAccount { return nil }`, `const sessionCookieName = "goscape_session"`, `func csrfToken(string) string { return "" }`, `func (p *portal) public(h http.HandlerFunc) http.HandlerFunc { return h }` — and omit the `rl` field. Task 14 replaces all four with the real implementations (they move to middleware.go).

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -v`
Expected: PASS (including the Task 10 module test — the embedded assets ship in the binary).

- [ ] **Step 5: Commit**

```bash
git add modules/account/portal.go modules/account/portal_test.go modules/account/testing_test.go \
        modules/account/templates modules/account/static
git commit --no-gpg-sign -m "feat(account): portal foundation — embedded SSR templates + render pipeline

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 14: Middleware — sessions, CSRF, rate limiting, auth guards

**Files:**
- Create: `modules/account/middleware.go`
- Modify: `modules/account/portal.go` (remove the Task-13 temporary stubs; add `rl` field)
- Test: `modules/account/middleware_test.go`

**Interfaces:**
- Consumes: Tasks 6, 13.
- Produces:

```go
const sessionCookieName = "goscape_session"

func ctxAccount(r *http.Request) *PortalAccount            // nil if unauthenticated
func csrfToken(rawSessionToken string) string              // hex(sha256("goscape-csrf|" + raw)) — session-bound, stateless
func (p *portal) public(h http.HandlerFunc) http.HandlerFunc  // loads account if a valid session cookie exists
func (p *portal) authed(h http.HandlerFunc) http.HandlerFunc  // public + requires login (302 → /login) + POST CSRF check
func (p *portal) admin(h http.HandlerFunc) http.HandlerFunc   // authed + admin group (else 404) 
func (p *portal) setSessionCookie(w http.ResponseWriter, raw string)
func (p *portal) clearSessionCookie(w http.ResponseWriter)

type rateLimiter struct{ ... } // fixed-window in-memory counter
func newRateLimiter() *rateLimiter
func (rl *rateLimiter) allow(key string, limit int, window time.Duration) bool
```

- [ ] **Step 1: Write the failing test**

```go
// modules/account/middleware_test.go
package account

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// loginTestAccount creates an account + a live session, returning the
// raw cookie value. Reused by later portal flow tests.
func loginTestAccount(t *testing.T, p *portal, s *Store, email string) (int64, *http.Cookie) {
	t.Helper()
	ctx := t.Context()
	phc, _ := HashPassword("hunter22!", testArgon2())
	id, err := s.CreateAccount(ctx, email, phc)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := NewRawToken()
	if err := s.CreateSession(ctx, id, HashToken(raw), "t", "t", p.cfg.Session); err != nil {
		t.Fatal(err)
	}
	return id, &http.Cookie{Name: sessionCookieName, Value: raw}
}

func TestMiddleware_AuthGuards(t *testing.T) {
	p, s := newTestPortal(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /open", p.public(func(w http.ResponseWriter, r *http.Request) {
		if a := ctxAccount(r); a != nil {
			w.Write([]byte("hello " + a.Email))
			return
		}
		w.Write([]byte("anonymous"))
	}))
	mux.HandleFunc("GET /private", p.authed(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("secret"))
	}))
	mux.HandleFunc("POST /private", p.authed(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("posted"))
	}))
	mux.HandleFunc("GET /adminz", p.admin(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("admin"))
	}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	get := func(path string, cookie *http.Cookie) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	// Anonymous.
	if resp := get("/open", nil); readBody(t, resp) != "anonymous" {
		t.Fatal("public without session must pass through as anonymous")
	}
	if resp := get("/private", nil); resp.StatusCode != http.StatusFound ||
		!strings.HasPrefix(resp.Header.Get("Location"), "/login") {
		t.Fatalf("authed without session: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	if resp := get("/adminz", nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("admin route must 404 for anonymous, got %d", resp.StatusCode)
	}

	// Authenticated.
	id, cookie := loginTestAccount(t, p, s, "a@example.com")
	if body := readBody(t, get("/open", cookie)); body != "hello a@example.com" {
		t.Fatalf("public with session: %q", body)
	}
	if resp := get("/private", cookie); resp.StatusCode != http.StatusOK {
		t.Fatalf("authed with session: %d", resp.StatusCode)
	}
	if resp := get("/adminz", cookie); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("admin route must 404 for non-admin, got %d", resp.StatusCode)
	}

	// Admin.
	if err := s.AddGroupMember(t.Context(), GroupAdmin, id, 0); err != nil {
		t.Fatal(err)
	}
	if resp := get("/adminz", cookie); resp.StatusCode != http.StatusOK {
		t.Fatalf("admin route for admin: %d", resp.StatusCode)
	}

	// CSRF: POST without token 403; with token passes.
	post := func(form url.Values) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/private", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(cookie)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	if resp := post(url.Values{}); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("POST without csrf: %d", resp.StatusCode)
	}
	if resp := post(url.Values{"csrf": {csrfToken(cookie.Value)}}); resp.StatusCode != http.StatusOK {
		t.Fatalf("POST with csrf: %d", resp.StatusCode)
	}
}

func TestRateLimiter(t *testing.T) {
	rl := newRateLimiter()
	for i := range 3 {
		if !rl.allow("k", 3, time.Minute) {
			t.Fatalf("attempt %d must be allowed", i)
		}
	}
	if rl.allow("k", 3, time.Minute) {
		t.Fatal("4th attempt must be denied")
	}
	if !rl.allow("other", 3, time.Minute) {
		t.Fatal("different key must be independent")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -run 'Middleware|RateLimiter' -v`
Expected: FAIL — `p.authed`/`p.admin`/`newRateLimiter` undefined.

- [ ] **Step 3: Write middleware.go (and strip the Task-13 stubs from portal.go)**

```go
// modules/account/middleware.go
package account

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const sessionCookieName = "goscape_session"

type ctxKey int

const ctxKeyAccount ctxKey = 0

// ctxAccount returns the logged-in account attached by the middleware,
// or nil.
func ctxAccount(r *http.Request) *PortalAccount {
	a, _ := r.Context().Value(ctxKeyAccount).(*PortalAccount)
	return a
}

// csrfToken derives the per-session CSRF token from the RAW session
// cookie value: hex(sha256("goscape-csrf|" + raw)). Stateless and
// session-bound — the server never stores it, and it is worthless
// without the (HttpOnly) session cookie it is derived from.
func csrfToken(rawSessionToken string) string {
	sum := sha256.Sum256([]byte("goscape-csrf|" + rawSessionToken))
	return hex.EncodeToString(sum[:])
}

// public loads the session account (if any) into the request context.
func (p *portal) public(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(sessionCookieName); err == nil {
			acct, err := p.store.SessionAccount(r.Context(), HashToken(c.Value), p.cfg.Session)
			switch {
			case err == nil && acct.Status == StatusActive:
				r = r.WithContext(context.WithValue(r.Context(), ctxKeyAccount, acct))
			case err == nil:
				// Disabled account with a live cookie: kill the session.
				_ = p.store.DeleteSession(r.Context(), HashToken(c.Value))
				p.clearSessionCookie(w)
			case errors.Is(err, ErrNotFound):
				p.clearSessionCookie(w)
			default:
				p.log.Error("session load failed", slog.Any("err", err))
			}
		}
		h(w, r)
	}
}

// authed requires a logged-in account and enforces CSRF on
// state-changing methods.
func (p *portal) authed(h http.HandlerFunc) http.HandlerFunc {
	return p.public(func(w http.ResponseWriter, r *http.Request) {
		acct := ctxAccount(r)
		if acct == nil {
			http.Redirect(w, r, "/login?msg=Please+log+in", http.StatusFound)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			c, err := r.Cookie(sessionCookieName)
			if err != nil || r.FormValue("csrf") != csrfToken(c.Value) {
				http.Error(w, "invalid csrf token", http.StatusForbidden)
				return
			}
		}
		h(w, r)
	})
}

// admin is authed + admin-group membership. Non-admins get 404 (the
// admin surface is not advertised), matching the spec.
func (p *portal) admin(h http.HandlerFunc) http.HandlerFunc {
	return p.authed(func(w http.ResponseWriter, r *http.Request) {
		acct := ctxAccount(r)
		ok, err := p.store.IsGroupMember(r.Context(), GroupAdmin, acct.ID)
		if err != nil {
			p.log.Error("admin check failed", slog.Any("err", err))
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		h(w, r)
	})
}

func (p *portal) setSessionCookie(w http.ResponseWriter, raw string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    raw,
		Path:     "/",
		MaxAge:   int(p.cfg.Session.AbsoluteTTL / time.Second),
		HttpOnly: true,
		Secure:   strings.HasPrefix(p.cfg.PublicURL, "https://"),
		SameSite: http.SameSiteLaxMode,
	})
}

func (p *portal) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}

// clientIP extracts the remote IP for rate-limit keys and audit rows.
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// rateLimiter is a fixed-window in-memory counter: cheap, per-process,
// good enough for portal abuse control (spec: in-memory token buckets).
type rateLimiter struct {
	mu      sync.Mutex
	windows map[string]*rlWindow
}

type rlWindow struct {
	start time.Time
	count int
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{windows: make(map[string]*rlWindow)}
}

func (rl *rateLimiter) allow(key string, limit int, window time.Duration) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	w, ok := rl.windows[key]
	if !ok || now.Sub(w.start) >= window {
		rl.windows[key] = &rlWindow{start: now, count: 1}
		return true
	}
	w.count++
	return w.count <= limit
}
```

In `portal.go`: delete the temporary `ctxAccount`/`sessionCookieName`/`csrfToken`/`public` stubs, add `rl *rateLimiter` to the struct, and set `rl: newRateLimiter()` in `newPortal`.

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/account/middleware.go modules/account/middleware_test.go modules/account/portal.go
git commit --no-gpg-sign -m "feat(account): portal middleware — sessions, CSRF, rate limiting, auth guards

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 15: Registration, login, logout

**Files:**
- Create: `modules/account/handlers_auth.go`
- Create: `modules/account/templates/pages/register.html`, `templates/pages/login.html`
- Modify: `modules/account/portal.go` (register routes)
- Test: `modules/account/handlers_auth_test.go`

**Interfaces:**
- Consumes: Tasks 3-6, 13, 14.
- Produces:

```go
// routes added:
//   GET/POST /register     GET/POST /login     POST /logout
func (p *portal) sendVerificationEmail(ctx context.Context, acct *PortalAccount) error // 24h verify_email token + mail via p.mailer
// register.html / login.html form field names: email, password, password2 (register only), csrf (login only — pre-auth POSTs are rate-limited instead of CSRF-checked; register has no session yet)
```

Rate limits (constants in handlers_auth.go): register 5/hour/IP; login 10 per 5min per IP+email key.

- [ ] **Step 1: Write the failing test**

```go
// modules/account/handlers_auth_test.go
package account

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// portalClient wraps httptest with a cookie jar and no-redirect policy.
func portalClient(t *testing.T, p *portal) (*httptest.Server, *http.Client) {
	t.Helper()
	srv := httptest.NewServer(p.routes())
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	return srv, &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func postForm(t *testing.T, c *http.Client, u string, form url.Values) *http.Response {
	t.Helper()
	resp, err := c.PostForm(u, form)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestRegisterFlow(t *testing.T) {
	p, s := newTestPortal(t)
	mailer := p.mailer.(*recordingMailer)
	srv, client := portalClient(t, p)

	// GET form renders.
	resp, _ := client.Get(srv.URL + "/register")
	if body := readBody(t, resp); !strings.Contains(body, `name="password2"`) {
		t.Fatalf("register form: %q", body)
	}

	// Successful registration creates the account and sends a
	// verification mail.
	resp = postForm(t, client, srv.URL+"/register", url.Values{
		"email": {"new@example.com"}, "password": {"hunter22!"}, "password2": {"hunter22!"},
	})
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("register: %d", resp.StatusCode)
	}
	acct, err := s.AccountByEmail(t.Context(), "new@example.com")
	if err != nil || acct.EmailVerified {
		t.Fatalf("created account: %+v err=%v", acct, err)
	}
	mail := mailer.last(t)
	if mail.To != "new@example.com" || !strings.Contains(mail.Body, "http://portal.test/verify-email?token=") {
		t.Fatalf("verification mail: %+v", mail)
	}

	// Password mismatch / policy violations re-render with an error.
	for _, form := range []url.Values{
		{"email": {"x@example.com"}, "password": {"hunter22!"}, "password2": {"different1!"}},
		{"email": {"x@example.com"}, "password": {"short"}, "password2": {"short"}},
		{"email": {"not-an-email"}, "password": {"hunter22!"}, "password2": {"hunter22!"}},
	} {
		resp = postForm(t, client, srv.URL+"/register", form)
		if resp.StatusCode != http.StatusOK || !strings.Contains(readBody(t, resp), "error") {
			t.Fatalf("bad form %v must re-render with error, got %d", form, resp.StatusCode)
		}
	}

	// Duplicate email surfaces a friendly error.
	resp = postForm(t, client, srv.URL+"/register", url.Values{
		"email": {"new@example.com"}, "password": {"hunter22!"}, "password2": {"hunter22!"},
	})
	if !strings.Contains(readBody(t, resp), "already registered") {
		t.Fatal("duplicate email must be reported")
	}
}

func TestLoginLogoutFlow(t *testing.T) {
	p, s := newTestPortal(t)
	srv, client := portalClient(t, p)
	phc, _ := HashPassword("hunter22!", testArgon2())
	id, _ := s.CreateAccount(t.Context(), "a@example.com", phc)

	// Wrong password.
	resp := postForm(t, client, srv.URL+"/login", url.Values{
		"email": {"a@example.com"}, "password": {"wrong"},
	})
	if resp.StatusCode != http.StatusOK || !strings.Contains(readBody(t, resp), "error") {
		t.Fatalf("wrong password: %d", resp.StatusCode)
	}

	// Correct password: session cookie set, redirect to /dashboard.
	resp = postForm(t, client, srv.URL+"/login", url.Values{
		"email": {"a@example.com"}, "password": {"hunter22!"},
	})
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/dashboard" {
		t.Fatalf("login: %d → %s", resp.StatusCode, resp.Header.Get("Location"))
	}

	// Session works: home shows the logged-in nav.
	resp, _ = client.Get(srv.URL + "/")
	if !strings.Contains(readBody(t, resp), "/dashboard") {
		t.Fatal("nav must show dashboard when logged in")
	}

	// Disabled accounts cannot log in.
	if err := s.SetAccountStatus(t.Context(), id, StatusDisabled); err != nil {
		t.Fatal(err)
	}
	resp = postForm(t, client, srv.URL+"/login", url.Values{
		"email": {"a@example.com"}, "password": {"hunter22!"},
	})
	if resp.StatusCode != http.StatusOK || !strings.Contains(readBody(t, resp), "disabled") {
		t.Fatal("disabled login must be rejected")
	}
	_ = s.SetAccountStatus(t.Context(), id, StatusActive)

	// Logout: needs CSRF, clears the session.
	var raw string
	u, _ := url.Parse(srv.URL)
	for _, c := range client.Jar.Cookies(u) {
		if c.Name == sessionCookieName {
			raw = c.Value
		}
	}
	resp = postForm(t, client, srv.URL+"/logout", url.Values{"csrf": {csrfToken(raw)}})
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("logout: %d", resp.StatusCode)
	}
	resp, _ = client.Get(srv.URL + "/")
	if strings.Contains(readBody(t, resp), "/dashboard") {
		t.Fatal("logout must drop the session")
	}
}

func TestLoginRateLimit(t *testing.T) {
	p, _ := newTestPortal(t)
	srv, client := portalClient(t, p)
	form := url.Values{"email": {"rl@example.com"}, "password": {"wrong"}}
	var last *http.Response
	for range 11 {
		last = postForm(t, client, srv.URL+"/login", form)
	}
	if last.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("11th attempt: %d, want 429", last.StatusCode)
	}
}
```

(add `"net/http/cookiejar"` to imports)

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -run 'Register|LoginLogout|LoginRateLimit' -v`
Expected: FAIL — routes return 404.

- [ ] **Step 3: Write templates + handlers**

`modules/account/templates/pages/register.html`:

```html
{{define "title"}}Register — goscape{{end}}
{{define "content"}}
<h1>Create your account</h1>
{{if .Data}}<p class="flash error">{{.Data}}</p>{{end}}
<form class="stack" method="post" action="/register">
  <label>Email <input type="email" name="email" required maxlength="255"></label>
  <label>Password
    <input type="password" name="password" required minlength="8" maxlength="20">
    <span class="muted">8-20 characters, no spaces — this password is also used at the game login screen, which caps passwords at 20 characters.</span>
  </label>
  <label>Repeat password <input type="password" name="password2" required maxlength="20"></label>
  <button type="submit">Register</button>
</form>
<p>Already have an account? <a href="/login">Log in</a>.</p>
{{end}}
```

`modules/account/templates/pages/login.html`:

```html
{{define "title"}}Log in — goscape{{end}}
{{define "content"}}
<h1>Log in</h1>
{{if .Data}}<p class="flash error">{{.Data}}</p>{{end}}
<form class="stack" method="post" action="/login">
  <label>Email <input type="email" name="email" required maxlength="255"></label>
  <label>Password <input type="password" name="password" required maxlength="20"></label>
  <button type="submit">Log in</button>
</form>
<p><a href="/forgot-password">Forgot your password?</a> · <a href="/register">Create an account</a></p>
{{end}}
```

`modules/account/handlers_auth.go`:

```go
// modules/account/handlers_auth.go
package account

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Portal abuse limits (fixed windows, per process).
const (
	registerLimit  = 5
	registerWindow = time.Hour
	loginLimit     = 10
	loginWindow    = 5 * time.Minute
	mailLimit      = 3
	mailWindow     = time.Hour
)

func validEmail(email string) bool {
	email = NormalizeEmail(email)
	at := strings.IndexByte(email, '@')
	return len(email) <= 255 && at > 0 && at < len(email)-1 && !strings.ContainsAny(email, " \t\r\n")
}

// sendVerificationEmail mints a 24h verify_email token and mails the
// link. Callers decide whether a failure is fatal (registration keeps
// the account and lets the player resend).
func (p *portal) sendVerificationEmail(ctx context.Context, acct *PortalAccount) error {
	raw, err := NewRawToken()
	if err != nil {
		return err
	}
	if err := p.store.CreateToken(ctx, acct.ID, TokenPurposeVerifyEmail, HashToken(raw), 24*time.Hour); err != nil {
		return err
	}
	link := p.cfg.PublicURL + "/verify-email?token=" + raw
	body := fmt.Sprintf("Welcome to goscape!\r\n\r\nVerify your email address by opening:\r\n\r\n%s\r\n\r\nThe link expires in 24 hours. If you didn't register, ignore this mail.\r\n", link)
	return p.mailer.Send(acct.Email, "Verify your goscape account", body)
}

func (p *portal) handleRegisterForm(w http.ResponseWriter, r *http.Request) {
	p.render(w, r, "register.html", nil)
}

func (p *portal) handleRegister(w http.ResponseWriter, r *http.Request) {
	if !p.rl.allow("register:"+clientIP(r), registerLimit, registerWindow) {
		http.Error(w, "too many registrations from this address — try again later", http.StatusTooManyRequests)
		return
	}
	fail := func(msg string) { p.render(w, r, "register.html", msg) } // .Data doubles as the error line (class="error")
	email := r.FormValue("email")
	password := r.FormValue("password")
	if !validEmail(email) {
		fail("error: that email address doesn't look valid")
		return
	}
	if password != r.FormValue("password2") {
		fail("error: the passwords don't match")
		return
	}
	if err := ValidPortalPassword(password); err != nil {
		fail("error: " + err.Error())
		return
	}
	phc, err := HashPassword(password, p.cfg.Argon2)
	if err != nil {
		p.log.Error("hash failed", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	id, err := p.store.CreateAccount(r.Context(), email, phc)
	if errors.Is(err, ErrEmailTaken) {
		fail("error: that email is already registered — try logging in or resetting your password")
		return
	}
	if err != nil {
		p.log.Error("create account failed", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := p.store.AppendAudit(r.Context(), 0, "account.register", fmt.Sprintf("account:%d", id), "ip="+clientIP(r)); err != nil {
		p.log.Warn("audit failed", slog.Any("err", err))
	}
	acct, err := p.store.AccountByID(r.Context(), id)
	if err == nil {
		if err := p.sendVerificationEmail(r.Context(), acct); err != nil {
			p.log.Warn("verification mail failed", slog.Any("err", err))
			http.Redirect(w, r, "/login?msg=Account+created,+but+the+verification+email+failed+to+send.+Log+in+and+use+Resend.", http.StatusFound)
			return
		}
	}
	http.Redirect(w, r, "/login?msg=Account+created.+Check+your+email+for+a+verification+link.", http.StatusFound)
}

func (p *portal) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	p.render(w, r, "login.html", nil)
}

func (p *portal) handleLogin(w http.ResponseWriter, r *http.Request) {
	email := NormalizeEmail(r.FormValue("email"))
	if !p.rl.allow("login:"+clientIP(r)+":"+email, loginLimit, loginWindow) {
		http.Error(w, "too many login attempts — try again later", http.StatusTooManyRequests)
		return
	}
	fail := func(msg string) { p.render(w, r, "login.html", msg) }
	acct, err := p.store.AccountByEmail(r.Context(), email)
	if errors.Is(err, ErrNotFound) {
		fail("error: unknown email or wrong password")
		return
	}
	if err != nil {
		p.log.Error("login lookup failed", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	ok, err := VerifyPassword(r.FormValue("password"), acct.PasswordHash)
	if err != nil || !ok {
		fail("error: unknown email or wrong password")
		return
	}
	if acct.Status != StatusActive {
		fail("error: this account is disabled — contact an admin")
		return
	}
	raw, err := NewRawToken()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := p.store.CreateSession(r.Context(), acct.ID, HashToken(raw), clientIP(r), r.UserAgent(), p.cfg.Session); err != nil {
		p.log.Error("create session failed", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	p.setSessionCookie(w, raw)
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

func (p *portal) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil {
		_ = p.store.DeleteSession(r.Context(), HashToken(c.Value))
	}
	p.clearSessionCookie(w)
	http.Redirect(w, r, "/?msg=Logged+out", http.StatusFound)
}
```

Add to `routes()` in portal.go:

```go
	mux.HandleFunc("GET /register", p.public(p.handleRegisterForm))
	mux.HandleFunc("POST /register", p.public(p.handleRegister))
	mux.HandleFunc("GET /login", p.public(p.handleLoginForm))
	mux.HandleFunc("POST /login", p.public(p.handleLogin))
	mux.HandleFunc("POST /logout", p.authed(p.handleLogout))
```

Note: `/dashboard` doesn't exist until Task 19 — the login test asserts only the redirect Location, not the target page.

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/account/handlers_auth.go modules/account/handlers_auth_test.go \
        modules/account/templates/pages/register.html modules/account/templates/pages/login.html \
        modules/account/portal.go
git commit --no-gpg-sign -m "feat(account): portal registration, login, logout

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 16: SMTP mailer + email verification flow

**Files:**
- Modify: `modules/account/mail.go` (add smtpMailer)
- Modify: `modules/account/account.go` (wire smtpMailer when smtp.host set)
- Modify: `modules/account/handlers_auth.go` (verify-email + resend handlers)
- Modify: `modules/account/portal.go` (routes)
- Test: `modules/account/handlers_verify_test.go` (+ a unit test for the mail message format in `mail_test.go`)

**Interfaces:**
- Consumes: Tasks 6, 15.
- Produces:

```go
func newSMTPMailer(cfg SMTPConfig) Mailer // net/smtp.SendMail; PlainAuth when username set; STARTTLS negotiated automatically
func buildMailMessage(from, to, subject, body string) []byte // RFC-5322 headers + CRLF body (unit-testable without a server)
// routes added: GET /verify-email    POST /resend-verification (authed)
```

- [ ] **Step 1: Write the failing tests**

```go
// modules/account/mail_test.go
package account

import (
	"strings"
	"testing"
)

func TestBuildMailMessage(t *testing.T) {
	msg := string(buildMailMessage("noreply@x", "player@y", "Subject line", "Line1\r\nLine2\r\n"))
	for _, want := range []string{
		"From: noreply@x\r\n", "To: player@y\r\n", "Subject: Subject line\r\n",
		"MIME-Version: 1.0\r\n", "Content-Type: text/plain; charset=utf-8\r\n",
		"\r\n\r\nLine1\r\n",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
}
```

```go
// modules/account/handlers_verify_test.go
package account

import (
	"net/url"
	"strings"
	"testing"
)

func extractLink(t *testing.T, body, marker string) string {
	t.Helper()
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatalf("no %q link in mail body:\n%s", marker, body)
	}
	link := body[i:]
	if j := strings.IndexAny(link, "\r\n \t"); j >= 0 {
		link = link[:j]
	}
	return link
}

func TestVerifyEmailFlow(t *testing.T) {
	p, s := newTestPortal(t)
	mailer := p.mailer.(*recordingMailer)
	srv, client := portalClient(t, p)

	// Register → mail carries the verify link.
	postForm(t, client, srv.URL+"/register", url.Values{
		"email": {"v@example.com"}, "password": {"hunter22!"}, "password2": {"hunter22!"},
	})
	link := extractLink(t, mailer.last(t).Body, "http://portal.test/verify-email?token=")
	local := strings.Replace(link, "http://portal.test", srv.URL, 1)

	// Following the link verifies the account.
	resp, err := client.Get(local)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("verify: %v %d", err, resp.StatusCode)
	}
	if !strings.Contains(readBody(t, resp), "verified") {
		t.Fatal("verify page must confirm")
	}
	acct, _ := s.AccountByEmail(t.Context(), "v@example.com")
	if !acct.EmailVerified {
		t.Fatal("account must be verified")
	}

	// Token is single-use.
	resp, _ = client.Get(local)
	if !strings.Contains(readBody(t, resp), "invalid or expired") {
		t.Fatal("second use must fail")
	}

	// Resend: log in (unverified users can), request resend, new link works.
	p2, s2 := newTestPortal(t)
	mailer2 := p2.mailer.(*recordingMailer)
	srv2, client2 := portalClient(t, p2)
	postForm(t, client2, srv2.URL+"/register", url.Values{
		"email": {"r@example.com"}, "password": {"hunter22!"}, "password2": {"hunter22!"},
	})
	postForm(t, client2, srv2.URL+"/login", url.Values{
		"email": {"r@example.com"}, "password": {"hunter22!"},
	})
	var raw string
	u2, _ := url.Parse(srv2.URL)
	for _, c := range client2.Jar.Cookies(u2) {
		if c.Name == sessionCookieName {
			raw = c.Value
		}
	}
	before := len(mailer2.sent)
	resp = postForm(t, client2, srv2.URL+"/resend-verification", url.Values{"csrf": {csrfToken(raw)}})
	if resp.StatusCode != http.StatusFound || len(mailer2.sent) != before+1 {
		t.Fatalf("resend: %d mails=%d", resp.StatusCode, len(mailer2.sent))
	}
	link2 := extractLink(t, mailer2.last(t).Body, "http://portal.test/verify-email?token=")
	resp, _ = client2.Get(strings.Replace(link2, "http://portal.test", srv2.URL, 1))
	if !strings.Contains(readBody(t, resp), "verified") {
		t.Fatal("resent link must verify")
	}
	acct2, _ := s2.AccountByEmail(t.Context(), "r@example.com")
	if !acct2.EmailVerified {
		t.Fatal("account 2 must be verified")
	}
	_ = s
}
```

(imports: `net/http`)

- [ ] **Step 2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -run 'BuildMail|VerifyEmail' -v`
Expected: FAIL — `buildMailMessage` undefined, /verify-email 404.

- [ ] **Step 3: Implement**

Append to `modules/account/mail.go`:

```go
import (
	"fmt"
	"net/smtp"
	"strings"
)

// buildMailMessage assembles a minimal RFC-5322 text/plain message.
func buildMailMessage(from, to, subject, body string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("\r\n\r\n")
	b.WriteString(body)
	return []byte(b.String())
}

// smtpMailer sends through a single relay. net/smtp negotiates
// STARTTLS automatically when the server advertises it; PlainAuth is
// used only when a username is configured.
type smtpMailer struct{ cfg SMTPConfig }

func newSMTPMailer(cfg SMTPConfig) Mailer { return &smtpMailer{cfg: cfg} }

func (m *smtpMailer) Send(to, subject, body string) error {
	addr := fmt.Sprintf("%s:%d", m.cfg.Host, m.cfg.Port)
	var auth smtp.Auth
	if m.cfg.Username != "" {
		auth = smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)
	}
	return smtp.SendMail(addr, auth, m.cfg.From, []string{to}, buildMailMessage(m.cfg.From, to, subject, body))
}
```

In `modules/account/account.go` starting(), replace the mailer line with the conditional:

```go
	var mailer Mailer = newLogMailer(a.log)
	if a.cfg.SMTP.Host != "" {
		mailer = newSMTPMailer(a.cfg.SMTP)
	}
```

Append handlers to `modules/account/handlers_auth.go`:

```go
func (p *portal) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("token")
	if raw == "" {
		p.render(w, r, "message.html", "That verification link is invalid or expired.")
		return
	}
	accountID, err := p.store.ConsumeToken(r.Context(), TokenPurposeVerifyEmail, HashToken(raw))
	if errors.Is(err, ErrNotFound) {
		p.render(w, r, "message.html", "That verification link is invalid or expired.")
		return
	}
	if err != nil {
		p.log.Error("verify token failed", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := p.store.SetEmailVerified(r.Context(), accountID); err != nil {
		p.log.Error("set verified failed", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	p.render(w, r, "message.html", "Your email is verified. You can now create characters (once your account passes the gate).")
}

func (p *portal) handleResendVerification(w http.ResponseWriter, r *http.Request) {
	acct := ctxAccount(r)
	if acct.EmailVerified {
		http.Redirect(w, r, "/dashboard?msg=Your+email+is+already+verified", http.StatusFound)
		return
	}
	if !p.rl.allow("mail:"+acct.Email, mailLimit, mailWindow) {
		http.Error(w, "too many emails requested — try again later", http.StatusTooManyRequests)
		return
	}
	if err := p.sendVerificationEmail(r.Context(), acct); err != nil {
		p.log.Warn("resend failed", slog.Any("err", err))
		http.Redirect(w, r, "/dashboard?msg=Sending+failed+—+try+again+later", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/dashboard?msg=Verification+email+sent", http.StatusFound)
}
```

Routes:

```go
	mux.HandleFunc("GET /verify-email", p.public(p.handleVerifyEmail))
	mux.HandleFunc("POST /resend-verification", p.authed(p.handleResendVerification))
```

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/account/mail.go modules/account/mail_test.go modules/account/account.go \
        modules/account/handlers_auth.go modules/account/handlers_verify_test.go modules/account/portal.go
git commit --no-gpg-sign -m "feat(account): SMTP mailer + email verification flow

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 17: Password reset flow

**Files:**
- Modify: `modules/account/handlers_auth.go` (forgot + reset handlers, `sendResetEmail` helper)
- Create: `modules/account/templates/pages/forgot.html`, `templates/pages/reset.html`
- Modify: `modules/account/portal.go` (routes)
- Test: `modules/account/handlers_reset_test.go`

**Interfaces:**
- Consumes: Tasks 6, 15, 16.
- Produces:

```go
func (p *portal) sendResetEmail(ctx context.Context, acct *PortalAccount) error // 1h reset_password token; reused by admin send-reset (Task 20)
// routes: GET/POST /forgot-password    GET/POST /reset-password
```

- [ ] **Step 1: Write the failing test**

```go
// modules/account/handlers_reset_test.go
package account

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestPasswordResetFlow(t *testing.T) {
	p, s := newTestPortal(t)
	mailer := p.mailer.(*recordingMailer)
	srv, client := portalClient(t, p)
	phc, _ := HashPassword("oldpass99!", testArgon2())
	id, _ := s.CreateAccount(t.Context(), "a@example.com", phc)

	// Live session that must die after the reset.
	raw, _ := NewRawToken()
	if err := s.CreateSession(t.Context(), id, HashToken(raw), "", "", p.cfg.Session); err != nil {
		t.Fatal(err)
	}

	// Enumeration-safe: unknown email gets the SAME response and no mail.
	before := len(mailer.sent)
	resp := postForm(t, client, srv.URL+"/forgot-password", url.Values{"email": {"ghost@example.com"}})
	unknownBody := readBody(t, resp)
	if len(mailer.sent) != before {
		t.Fatal("unknown email must not send mail")
	}
	resp = postForm(t, client, srv.URL+"/forgot-password", url.Values{"email": {"a@example.com"}})
	if knownBody := readBody(t, resp); knownBody != unknownBody {
		t.Fatal("known/unknown email responses must be identical (enumeration)")
	}
	if len(mailer.sent) != before+1 {
		t.Fatal("known email must send mail")
	}

	// Follow the link, set a new password.
	link := extractLink(t, mailer.last(t).Body, "http://portal.test/reset-password?token=")
	local := strings.Replace(link, "http://portal.test", srv.URL, 1)
	resp, _ = client.Get(local)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reset form: %d", resp.StatusCode)
	}
	tokenParam := strings.SplitN(link, "token=", 2)[1]
	resp = postForm(t, client, srv.URL+"/reset-password", url.Values{
		"token": {tokenParam}, "password": {"newpass22!"}, "password2": {"newpass22!"},
	})
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("reset submit: %d", resp.StatusCode)
	}

	// New password works, old doesn't, all sessions invalidated.
	acct, _ := s.AccountByID(t.Context(), id)
	if ok, _ := VerifyPassword("newpass22!", acct.PasswordHash); !ok {
		t.Fatal("new password must verify")
	}
	if ok, _ := VerifyPassword("oldpass99!", acct.PasswordHash); ok {
		t.Fatal("old password must not verify")
	}
	if _, err := s.SessionAccount(t.Context(), HashToken(raw), p.cfg.Session); err == nil {
		t.Fatal("existing sessions must be invalidated by reset")
	}

	// Token is single-use.
	resp = postForm(t, client, srv.URL+"/reset-password", url.Values{
		"token": {tokenParam}, "password": {"another11!"}, "password2": {"another11!"},
	})
	if resp.StatusCode != http.StatusOK || !strings.Contains(readBody(t, resp), "invalid or expired") {
		t.Fatal("token reuse must fail")
	}

	// A weak new password does NOT burn the token.
	resp = postForm(t, client, srv.URL+"/forgot-password", url.Values{"email": {"a@example.com"}})
	link2 := extractLink(t, mailer.last(t).Body, "http://portal.test/reset-password?token=")
	token2 := strings.SplitN(link2, "token=", 2)[1]
	resp = postForm(t, client, srv.URL+"/reset-password", url.Values{
		"token": {token2}, "password": {"short"}, "password2": {"short"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("weak password: %d", resp.StatusCode)
	}
	resp = postForm(t, client, srv.URL+"/reset-password", url.Values{
		"token": {token2}, "password": {"goodpass33!"}, "password2": {"goodpass33!"},
	})
	if resp.StatusCode != http.StatusFound {
		t.Fatal("token must survive a failed policy check")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -run PasswordResetFlow -v`
Expected: FAIL — routes 404.

- [ ] **Step 3: Implement**

`modules/account/templates/pages/forgot.html`:

```html
{{define "title"}}Forgot password — goscape{{end}}
{{define "content"}}
<h1>Forgot your password?</h1>
<form class="stack" method="post" action="/forgot-password">
  <label>Email <input type="email" name="email" required maxlength="255"></label>
  <button type="submit">Send reset link</button>
</form>
{{end}}
```

`modules/account/templates/pages/reset.html`:

```html
{{define "title"}}Reset password — goscape{{end}}
{{define "content"}}
<h1>Choose a new password</h1>
{{if .Data.Error}}<p class="flash error">{{.Data.Error}}</p>{{end}}
<form class="stack" method="post" action="/reset-password">
  <input type="hidden" name="token" value="{{.Data.Token}}">
  <label>New password
    <input type="password" name="password" required minlength="8" maxlength="20">
    <span class="muted">8-20 characters, no spaces (game-client compatible).</span>
  </label>
  <label>Repeat new password <input type="password" name="password2" required maxlength="20"></label>
  <button type="submit">Set password</button>
</form>
{{end}}
```

Append to `modules/account/handlers_auth.go`:

```go
type resetPageData struct {
	Token string
	Error string
}

// sendResetEmail mints a 1h reset_password token and mails the link.
// Shared by the self-service forgot flow and the admin "send reset"
// action (Task 20).
func (p *portal) sendResetEmail(ctx context.Context, acct *PortalAccount) error {
	raw, err := NewRawToken()
	if err != nil {
		return err
	}
	if err := p.store.CreateToken(ctx, acct.ID, TokenPurposeResetPassword, HashToken(raw), time.Hour); err != nil {
		return err
	}
	link := p.cfg.PublicURL + "/reset-password?token=" + raw
	body := fmt.Sprintf("A password reset was requested for your goscape account.\r\n\r\nSet a new password here:\r\n\r\n%s\r\n\r\nThe link expires in 1 hour and can be used once. If you didn't request this, ignore this mail.\r\n", link)
	return p.mailer.Send(acct.Email, "Reset your goscape password", body)
}

func (p *portal) handleForgotForm(w http.ResponseWriter, r *http.Request) {
	p.render(w, r, "forgot.html", nil)
}

func (p *portal) handleForgot(w http.ResponseWriter, r *http.Request) {
	if !p.rl.allow("mail:"+clientIP(r), mailLimit, mailWindow) {
		http.Error(w, "too many emails requested — try again later", http.StatusTooManyRequests)
		return
	}
	// Enumeration safety: identical response whether or not the account
	// exists; only the mail differs.
	if acct, err := p.store.AccountByEmail(r.Context(), r.FormValue("email")); err == nil {
		if err := p.sendResetEmail(r.Context(), acct); err != nil {
			p.log.Warn("reset mail failed", slog.Any("err", err))
		}
	}
	p.render(w, r, "message.html", "If that email has an account, a reset link is on its way.")
}

func (p *portal) handleResetForm(w http.ResponseWriter, r *http.Request) {
	p.render(w, r, "reset.html", resetPageData{Token: r.URL.Query().Get("token")})
}

func (p *portal) handleReset(w http.ResponseWriter, r *http.Request) {
	token := r.FormValue("token")
	password := r.FormValue("password")
	fail := func(msg string) {
		p.render(w, r, "reset.html", resetPageData{Token: token, Error: msg})
	}
	// Validate the new password BEFORE consuming the single-use token,
	// so a policy failure doesn't burn the link.
	if password != r.FormValue("password2") {
		fail("the passwords don't match")
		return
	}
	if err := ValidPortalPassword(password); err != nil {
		fail(err.Error())
		return
	}
	accountID, err := p.store.ConsumeToken(r.Context(), TokenPurposeResetPassword, HashToken(token))
	if errors.Is(err, ErrNotFound) {
		p.render(w, r, "message.html", "That reset link is invalid or expired.")
		return
	}
	if err != nil {
		p.log.Error("reset consume failed", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	phc, err := HashPassword(password, p.cfg.Argon2)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := p.store.SetPasswordHash(r.Context(), accountID, phc); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := p.store.DeleteAccountSessions(r.Context(), accountID); err != nil {
		p.log.Warn("session sweep failed", slog.Any("err", err))
	}
	if err := p.store.AppendAudit(r.Context(), 0, "account.password_reset", fmt.Sprintf("account:%d", accountID), "self-service"); err != nil {
		p.log.Warn("audit failed", slog.Any("err", err))
	}
	http.Redirect(w, r, "/login?msg=Password+changed.+Log+in+with+your+new+password.", http.StatusFound)
}
```

Routes:

```go
	mux.HandleFunc("GET /forgot-password", p.public(p.handleForgotForm))
	mux.HandleFunc("POST /forgot-password", p.public(p.handleForgot))
	mux.HandleFunc("GET /reset-password", p.public(p.handleResetForm))
	mux.HandleFunc("POST /reset-password", p.public(p.handleReset))
```

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/account/handlers_auth.go modules/account/handlers_reset_test.go \
        modules/account/templates/pages/forgot.html modules/account/templates/pages/reset.html \
        modules/account/portal.go
git commit --no-gpg-sign -m "feat(account): self-service password reset (enumeration-safe, single-use)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 18: Discord OAuth linking

**Files:**
- Create: `modules/account/oauth.go`
- Create: `modules/account/handlers_app.go` (link + callback handlers; dashboard etc. join in Task 19)
- Modify: `modules/account/portal.go` (disc field + routes)
- Test: `modules/account/oauth_test.go`

**Interfaces:**
- Consumes: Tasks 5, 14.
- Produces:

```go
type discordClient struct{ cfg DiscordConfig; hc *http.Client }
func newDiscordClient(cfg DiscordConfig) *discordClient
func (d *discordClient) configured() bool
func (d *discordClient) authorizeURL(state, redirectURI string) string
func (d *discordClient) exchangeCode(ctx context.Context, code, redirectURI string) (string, error)
func (d *discordClient) identify(ctx context.Context, accessToken string) (id, username string, err error)
// portal gains: disc *discordClient (set in newPortal from cfg.Providers.Discord)
// routes: GET /link/discord (authed)   GET /oauth/discord/callback (authed)
const oauthStateCookie = "goscape_oauth_state"
```

- [ ] **Step 1: Write the failing test**

```go
// modules/account/oauth_test.go
package account

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// fakeDiscord stands in for discord.com: token exchange + identify.
func fakeDiscord(t *testing.T) (*httptest.Server, *string) {
	t.Helper()
	var lastCode string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", 400)
			return
		}
		lastCode = r.FormValue("code")
		if r.FormValue("grant_type") != "authorization_code" || r.FormValue("client_id") == "" {
			http.Error(w, "bad request", 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok-" + lastCode, "token_type": "Bearer"})
	})
	mux.HandleFunc("GET /api/users/@me", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer tok-") {
			http.Error(w, "unauthorized", 401)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "D-42", "username": "alice"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &lastCode
}

func newDiscordTestPortal(t *testing.T) (*portal, *Store, *httptest.Server) {
	t.Helper()
	disc, _ := fakeDiscord(t)
	s := openTestStore(t)
	cfg := defaultConfig(t)
	cfg.Enable = true
	cfg.PublicURL = "http://portal.test"
	cfg.Argon2 = testArgon2()
	cfg.Providers.Discord = DiscordConfig{
		ClientID: "cid", ClientSecret: "sec",
		AuthURL:  disc.URL + "/oauth2/authorize",
		TokenURL: disc.URL + "/api/oauth2/token",
		APIBase:  disc.URL + "/api",
	}
	p, err := newPortal(cfg, s, &recordingMailer{}, testLogger(t))
	if err != nil {
		t.Fatal(err)
	}
	return p, s, disc
}

func TestDiscordLinkFlow(t *testing.T) {
	p, s, _ := newDiscordTestPortal(t)
	srv, client := portalClient(t, p)
	id, cookie := loginTestAccount(t, p, s, "a@example.com")
	u, _ := url.Parse(srv.URL)
	client.Jar.SetCookies(u, []*http.Cookie{cookie})

	// Start: redirects to Discord with client_id + state; state cookie set.
	resp, err := client.Get(srv.URL + "/link/discord")
	if err != nil || resp.StatusCode != http.StatusFound {
		t.Fatalf("link start: %v %d", err, resp.StatusCode)
	}
	loc, _ := url.Parse(resp.Header.Get("Location"))
	if loc.Query().Get("client_id") != "cid" || loc.Query().Get("state") == "" ||
		loc.Query().Get("scope") != "identify" {
		t.Fatalf("authorize url: %s", loc)
	}
	state := loc.Query().Get("state")

	// Callback with matching state links the identity.
	resp, err = client.Get(srv.URL + "/oauth/discord/callback?code=abc&state=" + url.QueryEscape(state))
	if err != nil || resp.StatusCode != http.StatusFound ||
		!strings.HasPrefix(resp.Header.Get("Location"), "/dashboard") {
		t.Fatalf("callback: %v %d %s", err, resp.StatusCode, resp.Header.Get("Location"))
	}
	ids, err := s.IdentitiesByAccount(t.Context(), id)
	if err != nil || len(ids) != 1 || ids[0].ProviderUserID != "D-42" || ids[0].ProviderUsername != "alice" {
		t.Fatalf("identity: %+v err=%v", ids, err)
	}

	// State mismatch is rejected and links nothing.
	p2, s2, _ := newDiscordTestPortal(t)
	srv2, client2 := portalClient(t, p2)
	id2, cookie2 := loginTestAccount(t, p2, s2, "b@example.com")
	u2, _ := url.Parse(srv2.URL)
	client2.Jar.SetCookies(u2, []*http.Cookie{cookie2})
	if _, err := client2.Get(srv2.URL + "/link/discord"); err != nil {
		t.Fatal(err)
	}
	resp, _ = client2.Get(srv2.URL + "/oauth/discord/callback?code=abc&state=forged")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("forged state: %d", resp.StatusCode)
	}
	if ids, _ := s2.IdentitiesByAccount(t.Context(), id2); len(ids) != 0 {
		t.Fatal("forged state must not link")
	}
}

func TestDiscordLink_TakenIdentity(t *testing.T) {
	p, s, _ := newDiscordTestPortal(t)
	srv, client := portalClient(t, p)
	// D-42 already belongs (burned) to another account.
	other, _ := s.CreateAccount(t.Context(), "other@example.com", "x")
	if err := s.LinkIdentity(t.Context(), other, "discord", "D-42", "bob"); err != nil {
		t.Fatal(err)
	}
	_, cookie := loginTestAccount(t, p, s, "a@example.com")
	u, _ := url.Parse(srv.URL)
	client.Jar.SetCookies(u, []*http.Cookie{cookie})

	resp, _ := client.Get(srv.URL + "/link/discord")
	loc, _ := url.Parse(resp.Header.Get("Location"))
	resp, _ = client.Get(srv.URL + "/oauth/discord/callback?code=abc&state=" + url.QueryEscape(loc.Query().Get("state")))
	if resp.StatusCode != http.StatusFound ||
		!strings.Contains(resp.Header.Get("Location"), "already+linked") {
		t.Fatalf("taken identity: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestDiscordLink_NotConfigured(t *testing.T) {
	p, s := newTestPortal(t) // no discord credentials
	srv, client := portalClient(t, p)
	_, cookie := loginTestAccount(t, p, s, "a@example.com")
	u, _ := url.Parse(srv.URL)
	client.Jar.SetCookies(u, []*http.Cookie{cookie})
	resp, _ := client.Get(srv.URL + "/link/discord")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unconfigured provider: %d, want 404", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -run Discord -v`
Expected: FAIL — routes 404 / `newDiscordClient` undefined.

- [ ] **Step 3: Implement oauth.go + handlers**

```go
// modules/account/oauth.go
package account

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Hand-rolled OAuth2 authorization-code flow (spec decision: zero new
// dependencies). Discord's flow is the plain RFC 6749 shape: redirect
// to authorize, POST the code to the token endpoint, GET /users/@me.
const (
	defaultDiscordAuthURL  = "https://discord.com/oauth2/authorize"
	defaultDiscordTokenURL = "https://discord.com/api/oauth2/token"
	defaultDiscordAPIBase  = "https://discord.com/api"
)

type discordClient struct {
	cfg DiscordConfig
	hc  *http.Client
}

func newDiscordClient(cfg DiscordConfig) *discordClient {
	return &discordClient{cfg: cfg, hc: &http.Client{Timeout: 10 * time.Second}}
}

// configured reports whether the operator supplied app credentials;
// unconfigured providers hide their routes (404).
func (d *discordClient) configured() bool {
	return d.cfg.ClientID != "" && d.cfg.ClientSecret != ""
}

func (d *discordClient) authorizeURL(state, redirectURI string) string {
	q := url.Values{
		"client_id":     {d.cfg.ClientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {"identify"},
		"state":         {state},
	}
	return cmp.Or(d.cfg.AuthURL, defaultDiscordAuthURL) + "?" + q.Encode()
}

func (d *discordClient) exchangeCode(ctx context.Context, code, redirectURI string) (string, error) {
	form := url.Values{
		"client_id":     {d.cfg.ClientID},
		"client_secret": {d.cfg.ClientSecret},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		cmp.Or(d.cfg.TokenURL, defaultDiscordTokenURL), strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := d.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("token exchange: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("token exchange: HTTP %d: %s", resp.StatusCode, b)
	}
	var payload struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil || payload.AccessToken == "" {
		return "", fmt.Errorf("token exchange: bad payload (%v)", err)
	}
	return payload.AccessToken, nil
}

func (d *discordClient) identify(ctx context.Context, accessToken string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		cmp.Or(d.cfg.APIBase, defaultDiscordAPIBase)+"/users/@me", nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := d.hc.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("identify: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("identify: HTTP %d", resp.StatusCode)
	}
	var payload struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil || payload.ID == "" {
		return "", "", fmt.Errorf("identify: bad payload (%v)", err)
	}
	return payload.ID, payload.Username, nil
}
```

```go
// modules/account/handlers_app.go
package account

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

const oauthStateCookie = "goscape_oauth_state"

func (p *portal) discordRedirectURI() string {
	return p.cfg.PublicURL + "/oauth/discord/callback"
}

// handleLinkDiscord starts the OAuth dance: random state bound to the
// browser via a short-lived cookie, then redirect to Discord.
func (p *portal) handleLinkDiscord(w http.ResponseWriter, r *http.Request) {
	if !p.disc.configured() {
		http.NotFound(w, r)
		return
	}
	state, err := NewRawToken()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: oauthStateCookie, Value: state, Path: "/oauth/",
		MaxAge: int((10 * time.Minute) / time.Second), HttpOnly: true,
		Secure: strings.HasPrefix(p.cfg.PublicURL, "https://"), SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, p.disc.authorizeURL(state, p.discordRedirectURI()), http.StatusFound)
}

func (p *portal) handleDiscordCallback(w http.ResponseWriter, r *http.Request) {
	if !p.disc.configured() {
		http.NotFound(w, r)
		return
	}
	acct := ctxAccount(r)
	c, err := r.Cookie(oauthStateCookie)
	if err != nil || c.Value == "" || r.URL.Query().Get("state") != c.Value {
		http.Error(w, "oauth state mismatch — restart the linking flow", http.StatusForbidden)
		return
	}
	// One-shot state: clear immediately.
	http.SetCookie(w, &http.Cookie{Name: oauthStateCookie, Value: "", Path: "/oauth/", MaxAge: -1})

	code := r.URL.Query().Get("code")
	if code == "" {
		// Player denied the authorization on Discord's side.
		http.Redirect(w, r, "/dashboard?msg=Discord+linking+was+cancelled", http.StatusFound)
		return
	}
	token, err := p.disc.exchangeCode(r.Context(), code, p.discordRedirectURI())
	if err != nil {
		p.log.Warn("discord exchange failed", slog.Any("err", err))
		http.Redirect(w, r, "/dashboard?msg=Discord+linking+failed+—+try+again", http.StatusFound)
		return
	}
	discordID, discordName, err := p.disc.identify(r.Context(), token)
	if err != nil {
		p.log.Warn("discord identify failed", slog.Any("err", err))
		http.Redirect(w, r, "/dashboard?msg=Discord+linking+failed+—+try+again", http.StatusFound)
		return
	}
	err = p.store.LinkIdentity(r.Context(), acct.ID, "discord", discordID, discordName)
	switch {
	case errors.Is(err, ErrIdentityTaken):
		http.Redirect(w, r, "/dashboard?msg=That+Discord+account+is+already+linked+to+a+different+account", http.StatusFound)
		return
	case errors.Is(err, ErrAlreadyLinked):
		http.Redirect(w, r, "/dashboard?msg=Your+account+already+has+a+linked+Discord", http.StatusFound)
		return
	case err != nil:
		p.log.Error("link identity failed", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := p.store.AppendAudit(r.Context(), acct.ID, "identity.link",
		fmt.Sprintf("account:%d", acct.ID), "discord:"+discordID); err != nil {
		p.log.Warn("audit failed", slog.Any("err", err))
	}
	http.Redirect(w, r, "/dashboard?msg=Discord+linked", http.StatusFound)
}
```

(import `"strings"` in handlers_app.go)

In `portal.go`: add `disc *discordClient` to the struct; in `newPortal` set `disc: newDiscordClient(cfg.Providers.Discord)`. Routes:

```go
	mux.HandleFunc("GET /link/discord", p.authed(p.handleLinkDiscord))
	mux.HandleFunc("GET /oauth/discord/callback", p.authed(p.handleDiscordCallback))
```

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/account/oauth.go modules/account/oauth_test.go modules/account/handlers_app.go \
        modules/account/portal.go
git commit --no-gpg-sign -m "feat(account): Discord OAuth linking (hand-rolled code flow, state-bound)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 19: Dashboard, character creation, settings

**Files:**
- Modify: `modules/account/handlers_app.go` (dashboard, characters, settings handlers)
- Create: `templates/pages/dashboard.html`, `templates/pages/character_new.html`, `templates/pages/settings.html`
- Modify: `modules/account/portal.go` (routes)
- Test: `modules/account/handlers_app_test.go`

**Interfaces:**
- Consumes: Tasks 3, 5, 14, 16, 18.
- Produces:

```go
type dashboardData struct {
	Characters        []Character
	Identities        []Identity
	Eligible          bool
	CharacterLimit    int
	DiscordConfigured bool
	DiscordLinked     bool
}
// routes: GET /dashboard   GET/POST /characters/new   GET/POST /settings/password (all authed)
```

Gate logic at the single choke point (`handleCharacterCreate`): active status is implied by `authed` (public() drops disabled accounts' sessions); requires `acct.EmailVerified`, `GateEligible(providers)`, and `CreateCharacter` enforces the limit + name reservation atomically.

- [ ] **Step 1: Write the failing test**

```go
// modules/account/handlers_app_test.go
package account

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// authedPortal returns a portal + client already logged in as a fresh
// verified account, plus the raw session value for CSRF.
func authedPortal(t *testing.T) (*portal, *Store, *httptest.Server, *http.Client, int64, string) {
	t.Helper()
	p, s := newTestPortal(t)
	srv, client := portalClient(t, p)
	id, cookie := loginTestAccount(t, p, s, "a@example.com")
	if err := s.SetEmailVerified(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(srv.URL)
	client.Jar.SetCookies(u, []*http.Cookie{cookie})
	return p, s, srv, client, id, cookie.Value
}

func TestDashboard(t *testing.T) {
	_, s, srv, client, id, _ := authedPortal(t)
	if _, err := s.CreateCharacter(t.Context(), id, "zezima", 5); err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(srv.URL + "/dashboard")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("dashboard: %v %d", err, resp.StatusCode)
	}
	body := readBody(t, resp)
	for _, want := range []string{"zezima", "a@example.com", "not eligible"} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard missing %q", want)
		}
	}
}

func TestCharacterCreation_GateAndLimit(t *testing.T) {
	p, s, srv, client, id, raw := authedPortal(t)
	form := func(name string) url.Values {
		return url.Values{"name": {name}, "csrf": {csrfToken(raw)}}
	}

	// Not eligible: no link, not approved.
	resp := postForm(t, client, srv.URL+"/characters/new", form("zezima"))
	if resp.StatusCode != http.StatusOK || !strings.Contains(readBody(t, resp), "not eligible") {
		t.Fatalf("ineligible create must re-render with error: %d", resp.StatusCode)
	}
	if chars, _ := s.CharactersByAccount(t.Context(), id); len(chars) != 0 {
		t.Fatal("no character may be created while ineligible")
	}

	// manually_approved unlocks it.
	if err := s.AddGroupMember(t.Context(), GroupManuallyApproved, id, 0); err != nil {
		t.Fatal(err)
	}
	resp = postForm(t, client, srv.URL+"/characters/new", form("Zezima"))
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("eligible create: %d", resp.StatusCode)
	}
	chars, _ := s.CharactersByAccount(t.Context(), id)
	if len(chars) != 1 || chars[0].Username != "zezima" {
		t.Fatalf("created: %+v", chars)
	}

	// Bad name re-renders.
	resp = postForm(t, client, srv.URL+"/characters/new", form("bad!name!!"))
	if resp.StatusCode != http.StatusOK || !strings.Contains(readBody(t, resp), "may only contain") {
		t.Fatal("invalid name must re-render with the validation error")
	}

	// Duplicate name.
	resp = postForm(t, client, srv.URL+"/characters/new", form("zezima"))
	if !strings.Contains(readBody(t, resp), "already taken") {
		t.Fatal("dup name must be reported")
	}

	// Limit: cfg default is 5; drop it to 1 for the test portal.
	p.cfg.CharacterLimit = 1
	resp = postForm(t, client, srv.URL+"/characters/new", form("alt1"))
	if !strings.Contains(readBody(t, resp), "character limit") {
		t.Fatal("limit must be reported")
	}

	// Unverified email blocks creation even when approved.
	if _, err := s.db.ExecContext(t.Context(), s.db.Rebind(
		`UPDATE portal_account SET email_verified = 0 WHERE id = ?`), id); err != nil {
		t.Fatal(err)
	}
	resp = postForm(t, client, srv.URL+"/characters/new", form("alt2"))
	if !strings.Contains(readBody(t, resp), "verify your email") {
		t.Fatal("unverified must be blocked")
	}
}

func TestSettings_ChangePassword(t *testing.T) {
	p, s, srv, client, id, raw := authedPortal(t)
	resp := postForm(t, client, srv.URL+"/settings/password", url.Values{
		"csrf": {csrfToken(raw)}, "current": {"hunter22!"},
		"password": {"newpass33!"}, "password2": {"newpass33!"},
	})
	if resp.StatusCode != http.StatusFound || !strings.HasPrefix(resp.Header.Get("Location"), "/login") {
		t.Fatalf("change password: %d → %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	acct, _ := s.AccountByID(t.Context(), id)
	if ok, _ := VerifyPassword("newpass33!", acct.PasswordHash); !ok {
		t.Fatal("new password must verify")
	}
	if _, err := s.SessionAccount(t.Context(), HashToken(raw), p.cfg.Session); err == nil {
		t.Fatal("all sessions must be invalidated")
	}

	// Wrong current password is rejected.
	_, cookie2 := loginTestAccount(t, p, s, "b@example.com")
	u, _ := url.Parse(srv.URL)
	client.Jar.SetCookies(u, []*http.Cookie{cookie2})
	resp = postForm(t, client, srv.URL+"/settings/password", url.Values{
		"csrf": {csrfToken(cookie2.Value)}, "current": {"wrong"},
		"password": {"newpass44!"}, "password2": {"newpass44!"},
	})
	if resp.StatusCode != http.StatusOK || !strings.Contains(readBody(t, resp), "current password") {
		t.Fatal("wrong current password must be rejected")
	}
}
```

(import `"net/http/httptest"`)

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -run 'Dashboard|CharacterCreation|Settings' -v`
Expected: FAIL — routes 404.

- [ ] **Step 3: Implement templates + handlers**

`templates/pages/dashboard.html`:

```html
{{define "title"}}Dashboard — goscape{{end}}
{{define "content"}}
<h1>Your account</h1>
<p>{{.Account.Email}}
{{if .Account.EmailVerified}} <span class="muted">(verified)</span>
{{else}} <span class="error">(unverified)</span>
<form class="inline" method="post" action="/resend-verification">
  <input type="hidden" name="csrf" value="{{.CSRF}}"><button>Resend verification email</button>
</form>
{{end}}</p>

<h2>Linked accounts</h2>
{{if .Data.Identities}}
<table><tr><th>Provider</th><th>Account</th><th>Status</th></tr>
{{range .Data.Identities}}
<tr><td>{{.Provider}}</td><td>{{.ProviderUsername}} ({{.ProviderUserID}})</td>
<td>{{if .RevokedAt.Valid}}revoked{{else}}linked{{end}}</td></tr>
{{end}}</table>
<p class="muted">Linked accounts cannot be removed. Contact an admin if you need a link changed.</p>
{{else}}
<p>No linked accounts.
{{if .Data.DiscordConfigured}}<a class="button" href="/link/discord">Link Discord</a>{{end}}</p>
{{end}}

<h2>Characters ({{len .Data.Characters}}/{{.Data.CharacterLimit}})</h2>
{{if .Data.Characters}}
<table><tr><th>Name</th><th>Created</th></tr>
{{range .Data.Characters}}<tr><td>{{.Username}}</td><td>{{.CreatedAt.Format "2006-01-02"}}</td></tr>{{end}}
</table>
{{else}}<p>No characters yet.</p>{{end}}
{{if .Data.Eligible}}
<p><a class="button" href="/characters/new">Create a character</a></p>
{{else}}
<p class="muted">Your account is not eligible to create characters yet — link a Discord account, or ask an admin for manual approval.</p>
{{end}}
{{end}}
```

`templates/pages/character_new.html`:

```html
{{define "title"}}New character — goscape{{end}}
{{define "content"}}
<h1>Create a character</h1>
{{if .Data}}<p class="flash error">{{.Data}}</p>{{end}}
<form class="stack" method="post" action="/characters/new">
  <input type="hidden" name="csrf" value="{{.CSRF}}">
  <label>Character name
    <input name="name" required maxlength="12" pattern="[A-Za-z0-9_ ]{1,12}">
    <span class="muted">1-12 characters: letters, numbers, spaces or underscores. This is the name you log into the game with (using your account password).</span>
  </label>
  <button type="submit">Create</button>
</form>
{{end}}
```

`templates/pages/settings.html`:

```html
{{define "title"}}Settings — goscape{{end}}
{{define "content"}}
<h1>Change password</h1>
{{if .Data}}<p class="flash error">{{.Data}}</p>{{end}}
<p class="muted">Changing your password logs you out everywhere. The new password is also your game login password.</p>
<form class="stack" method="post" action="/settings/password">
  <input type="hidden" name="csrf" value="{{.CSRF}}">
  <label>Current password <input type="password" name="current" required maxlength="20"></label>
  <label>New password <input type="password" name="password" required minlength="8" maxlength="20"></label>
  <label>Repeat new password <input type="password" name="password2" required maxlength="20"></label>
  <button type="submit">Change password</button>
</form>
{{end}}
```

Append to `handlers_app.go`:

```go
type dashboardData struct {
	Characters        []Character
	Identities        []Identity
	Eligible          bool
	CharacterLimit    int
	DiscordConfigured bool
}

func (p *portal) handleDashboard(w http.ResponseWriter, r *http.Request) {
	acct := ctxAccount(r)
	chars, err := p.store.CharactersByAccount(r.Context(), acct.ID)
	if err != nil {
		p.log.Error("dashboard characters", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	ids, err := p.store.IdentitiesByAccount(r.Context(), acct.ID)
	if err != nil {
		p.log.Error("dashboard identities", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	eligible, err := p.store.GateEligible(r.Context(), acct.ID, p.cfg.Gate.Providers)
	if err != nil {
		p.log.Error("dashboard gate", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	p.render(w, r, "dashboard.html", dashboardData{
		Characters:        chars,
		Identities:        ids,
		Eligible:          eligible && acct.EmailVerified,
		CharacterLimit:    p.cfg.CharacterLimit,
		DiscordConfigured: p.disc.configured(),
	})
}

func (p *portal) handleCharacterForm(w http.ResponseWriter, r *http.Request) {
	p.render(w, r, "character_new.html", nil)
}

// handleCharacterCreate is the gate choke point (spec: single place).
func (p *portal) handleCharacterCreate(w http.ResponseWriter, r *http.Request) {
	acct := ctxAccount(r)
	fail := func(msg string) { p.render(w, r, "character_new.html", msg) }
	if !acct.EmailVerified {
		fail("verify your email address before creating characters")
		return
	}
	eligible, err := p.store.GateEligible(r.Context(), acct.ID, p.cfg.Gate.Providers)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !eligible {
		fail("your account is not eligible yet — link a Discord account or ask an admin for approval")
		return
	}
	name, err := NormalizeCharacterName(r.FormValue("name"))
	if err != nil {
		fail(err.Error())
		return
	}
	ch, err := p.store.CreateCharacter(r.Context(), acct.ID, name, p.cfg.CharacterLimit)
	switch {
	case errors.Is(err, ErrNameTaken):
		fail("that name is already taken")
		return
	case errors.Is(err, ErrCharacterLimit):
		fail(fmt.Sprintf("you've reached the character limit (%d)", p.cfg.CharacterLimit))
		return
	case err != nil:
		p.log.Error("create character", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := p.store.AppendAudit(r.Context(), acct.ID, "character.create",
		fmt.Sprintf("account:%d", acct.ID), "name="+ch.Username); err != nil {
		p.log.Warn("audit failed", slog.Any("err", err))
	}
	http.Redirect(w, r, "/dashboard?msg=Character+"+ch.Username+"+created.+Log+into+the+game+with+that+name+and+your+account+password.", http.StatusFound)
}

func (p *portal) handleSettingsForm(w http.ResponseWriter, r *http.Request) {
	p.render(w, r, "settings.html", nil)
}

func (p *portal) handleSettingsPassword(w http.ResponseWriter, r *http.Request) {
	acct := ctxAccount(r)
	fail := func(msg string) { p.render(w, r, "settings.html", msg) }
	ok, err := VerifyPassword(r.FormValue("current"), acct.PasswordHash)
	if err != nil || !ok {
		fail("your current password is wrong")
		return
	}
	newPW := r.FormValue("password")
	if newPW != r.FormValue("password2") {
		fail("the new passwords don't match")
		return
	}
	if err := ValidPortalPassword(newPW); err != nil {
		fail(err.Error())
		return
	}
	phc, err := HashPassword(newPW, p.cfg.Argon2)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := p.store.SetPasswordHash(r.Context(), acct.ID, phc); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := p.store.DeleteAccountSessions(r.Context(), acct.ID); err != nil {
		p.log.Warn("session sweep failed", slog.Any("err", err))
	}
	if err := p.store.AppendAudit(r.Context(), acct.ID, "account.password_change",
		fmt.Sprintf("account:%d", acct.ID), ""); err != nil {
		p.log.Warn("audit failed", slog.Any("err", err))
	}
	p.clearSessionCookie(w)
	http.Redirect(w, r, "/login?msg=Password+changed+—+log+in+again+(this+is+also+your+game+password)", http.StatusFound)
}
```

Routes:

```go
	mux.HandleFunc("GET /dashboard", p.authed(p.handleDashboard))
	mux.HandleFunc("GET /characters/new", p.authed(p.handleCharacterForm))
	mux.HandleFunc("POST /characters/new", p.authed(p.handleCharacterCreate))
	mux.HandleFunc("GET /settings/password", p.authed(p.handleSettingsForm))
	mux.HandleFunc("POST /settings/password", p.authed(p.handleSettingsPassword))
```

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/account/handlers_app.go modules/account/handlers_app_test.go \
        modules/account/templates/pages/dashboard.html modules/account/templates/pages/character_new.html \
        modules/account/templates/pages/settings.html modules/account/portal.go
git commit --no-gpg-sign -m "feat(account): dashboard, gated character creation, password settings

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 20: Admin portal pages

**Files:**
- Create: `modules/account/handlers_admin.go`
- Create: `templates/pages/admin_search.html`, `templates/pages/admin_account.html`, `templates/pages/admin_audit.html`
- Modify: `modules/account/portal.go` (routes)
- Test: `modules/account/handlers_admin_test.go`

**Interfaces:**
- Consumes: Tasks 4-6, 9 (store search/groups), 14, 17 (`sendResetEmail`), 16 (`sendVerificationEmail`).
- Produces routes (all `p.admin`):

```
GET  /admin                       search form + results (?q=)
GET  /admin/accounts/{id}         detail: status, identities, characters, groups, recent audit
POST /admin/accounts/{id}/group   form: group, member ("on"/"")   → add/remove manually_approved or admin
POST /admin/accounts/{id}/status  form: status                    → active/disabled (+ session sweep on disable)
POST /admin/accounts/{id}/unlink  form: provider                  → soft burn
POST /admin/accounts/{id}/release form: provider, provider_user_id → hard delete
POST /admin/accounts/{id}/resend-verification
POST /admin/accounts/{id}/send-reset
GET  /admin/audit                 recent audit (?target=)
```

Every action calls `AppendAudit` with `actor = admin.ID` and redirects back to `/admin/accounts/{id}?msg=...`.

- [ ] **Step 1: Write the failing test**

```go
// modules/account/handlers_admin_test.go
package account

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func adminPortal(t *testing.T) (*portal, *Store, *httptest.Server, *http.Client, string) {
	t.Helper()
	p, s := newTestPortal(t)
	srv, client := portalClient(t, p)
	adminID, cookie := loginTestAccount(t, p, s, "admin@example.com")
	if err := s.AddGroupMember(t.Context(), GroupAdmin, adminID, 0); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(srv.URL)
	client.Jar.SetCookies(u, []*http.Cookie{cookie})
	return p, s, srv, client, cookie.Value
}

func TestAdminPages(t *testing.T) {
	_, s, srv, client, raw := adminPortal(t)
	targetID := seedVerifiedAccountWithCharacter(t, s, "player@example.com", "zezima")
	if err := s.LinkIdentity(t.Context(), targetID, "discord", "D9", "bob"); err != nil {
		t.Fatal(err)
	}
	detail := fmt.Sprintf("%s/admin/accounts/%d", srv.URL, targetID)
	csrf := csrfToken(raw)

	// Search finds the player.
	resp, err := client.Get(srv.URL + "/admin?q=player@")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("search: %v %d", err, resp.StatusCode)
	}
	if body := readBody(t, resp); !strings.Contains(body, "player@example.com") {
		t.Fatalf("search results: %q", body)
	}

	// Detail shows identities + characters.
	resp, _ = client.Get(detail)
	body := readBody(t, resp)
	for _, want := range []string{"zezima", "discord", "D9", "manually_approved"} {
		if !strings.Contains(body, want) {
			t.Errorf("detail missing %q", want)
		}
	}

	// Approve → gate satisfied.
	resp = postForm(t, client, detail+"/group", url.Values{
		"csrf": {csrf}, "group": {GroupManuallyApproved}, "member": {"on"},
	})
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("approve: %d", resp.StatusCode)
	}
	if ok, _ := s.IsGroupMember(t.Context(), GroupManuallyApproved, targetID); !ok {
		t.Fatal("approve must add group")
	}

	// Disable → status flips and sessions die.
	resp = postForm(t, client, detail+"/status", url.Values{"csrf": {csrf}, "status": {StatusDisabled}})
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("disable: %d", resp.StatusCode)
	}
	acct, _ := s.AccountByID(t.Context(), targetID)
	if acct.Status != StatusDisabled {
		t.Fatal("disable must persist")
	}

	// Unlink burns; release frees.
	resp = postForm(t, client, detail+"/unlink", url.Values{"csrf": {csrf}, "provider": {"discord"}})
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("unlink: %d", resp.StatusCode)
	}
	ids, _ := s.IdentitiesByAccount(t.Context(), targetID)
	if len(ids) != 1 || !ids[0].RevokedAt.Valid {
		t.Fatal("unlink must revoke")
	}
	resp = postForm(t, client, detail+"/release", url.Values{
		"csrf": {csrf}, "provider": {"discord"}, "provider_user_id": {"D9"},
	})
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("release: %d", resp.StatusCode)
	}
	if ids, _ := s.IdentitiesByAccount(t.Context(), targetID); len(ids) != 0 {
		t.Fatal("release must delete")
	}

	// Audit page shows the admin actions with the admin as actor.
	resp, _ = client.Get(srv.URL + "/admin/audit")
	body = readBody(t, resp)
	for _, want := range []string{"group.set", "account.status", "identity.unlink", "identity.release"} {
		if !strings.Contains(body, want) {
			t.Errorf("audit missing %q", want)
		}
	}
}

func TestAdminMailActions(t *testing.T) {
	p, s, srv, client, raw := adminPortal(t)
	mailer := p.mailer.(*recordingMailer)
	targetID, _ := s.CreateAccount(t.Context(), "player@example.com", "x")
	detail := fmt.Sprintf("%s/admin/accounts/%d", srv.URL, targetID)

	resp := postForm(t, client, detail+"/resend-verification", url.Values{"csrf": {csrfToken(raw)}})
	if resp.StatusCode != http.StatusFound || !strings.Contains(mailer.last(t).Body, "/verify-email?token=") {
		t.Fatalf("resend-verification: %d", resp.StatusCode)
	}
	resp = postForm(t, client, detail+"/send-reset", url.Values{"csrf": {csrfToken(raw)}})
	if resp.StatusCode != http.StatusFound || !strings.Contains(mailer.last(t).Body, "/reset-password?token=") {
		t.Fatalf("send-reset: %d", resp.StatusCode)
	}
}
```

(import `"net/http/httptest"`)

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -run TestAdmin -v`
Expected: FAIL — /admin routes 404 (note: `TestAdminRPCs` from Task 9 still passes; the -run pattern also matches it, which is fine).

- [ ] **Step 3: Implement templates + handlers**

`templates/pages/admin_search.html`:

```html
{{define "title"}}Admin — goscape{{end}}
{{define "content"}}
<h1>Admin — accounts</h1>
<form method="get" action="/admin">
  <input name="q" value="{{.Data.Query}}" placeholder="email, character name, or Discord id">
  <button type="submit">Search</button>
  <a href="/admin/audit">Audit log</a>
</form>
{{if .Data.Results}}
<table><tr><th>ID</th><th>Email</th><th>Verified</th><th>Status</th></tr>
{{range .Data.Results}}
<tr><td><a href="/admin/accounts/{{.ID}}">{{.ID}}</a></td>
<td>{{.Email}}</td><td>{{.EmailVerified}}</td><td>{{.Status}}</td></tr>
{{end}}</table>
{{else if .Data.Query}}<p>No matches.</p>{{end}}
{{end}}
```

`templates/pages/admin_account.html`:

```html
{{define "title"}}Admin — account{{end}}
{{define "content"}}
<h1>Account {{.Data.Acct.ID}} — {{.Data.Acct.Email}}</h1>
<p>Status: <b>{{.Data.Acct.Status}}</b> · Verified: {{.Data.Acct.EmailVerified}} · Groups: {{range .Data.Groups}}{{.}} {{end}}</p>

<h2>Actions</h2>
<form class="inline" method="post" action="/admin/accounts/{{.Data.Acct.ID}}/group">
  <input type="hidden" name="csrf" value="{{.CSRF}}">
  <input type="hidden" name="group" value="manually_approved">
  {{if .Data.Approved}}<button type="submit">Remove manual approval</button>
  {{else}}<input type="hidden" name="member" value="on"><button type="submit">Manually approve</button>{{end}}
</form>
<form class="inline" method="post" action="/admin/accounts/{{.Data.Acct.ID}}/status">
  <input type="hidden" name="csrf" value="{{.CSRF}}">
  {{if eq .Data.Acct.Status "active"}}<input type="hidden" name="status" value="disabled"><button type="submit">Disable account</button>
  {{else}}<input type="hidden" name="status" value="active"><button type="submit">Re-enable account</button>{{end}}
</form>
<form class="inline" method="post" action="/admin/accounts/{{.Data.Acct.ID}}/resend-verification">
  <input type="hidden" name="csrf" value="{{.CSRF}}"><button type="submit">Resend verification</button>
</form>
<form class="inline" method="post" action="/admin/accounts/{{.Data.Acct.ID}}/send-reset">
  <input type="hidden" name="csrf" value="{{.CSRF}}"><button type="submit">Send password reset</button>
</form>

<h2>Linked identities</h2>
<table><tr><th>Provider</th><th>User id</th><th>Name</th><th>State</th><th></th></tr>
{{range .Data.Identities}}
<tr><td>{{.Provider}}</td><td>{{.ProviderUserID}}</td><td>{{.ProviderUsername}}</td>
<td>{{if .RevokedAt.Valid}}REVOKED{{else}}linked{{end}}</td>
<td>
{{if not .RevokedAt.Valid}}
<form class="inline" method="post" action="/admin/accounts/{{$.Data.Acct.ID}}/unlink">
  <input type="hidden" name="csrf" value="{{$.CSRF}}">
  <input type="hidden" name="provider" value="{{.Provider}}">
  <button type="submit">Unlink (burn)</button>
</form>
{{end}}
<form class="inline" method="post" action="/admin/accounts/{{$.Data.Acct.ID}}/release">
  <input type="hidden" name="csrf" value="{{$.CSRF}}">
  <input type="hidden" name="provider" value="{{.Provider}}">
  <input type="hidden" name="provider_user_id" value="{{.ProviderUserID}}">
  <button type="submit">Release identity</button>
</form>
</td></tr>
{{end}}</table>

<h2>Characters</h2>
<table><tr><th>Name</th><th>Game account</th><th>Created</th></tr>
{{range .Data.Characters}}<tr><td>{{.Username}}</td><td>{{.GameAccountID}}</td><td>{{.CreatedAt.Format "2006-01-02"}}</td></tr>{{end}}
</table>

<h2>Recent audit</h2>
<table><tr><th>When</th><th>Actor</th><th>Action</th><th>Details</th></tr>
{{range .Data.Audit}}
<tr><td>{{.CreatedAt.Format "2006-01-02 15:04"}}</td>
<td>{{if .Actor.Valid}}{{.Actor.Int64}}{{else}}system{{end}}</td>
<td>{{.Action}}</td><td>{{.Details}}</td></tr>
{{end}}</table>
{{end}}
```

`templates/pages/admin_audit.html`:

```html
{{define "title"}}Admin — audit{{end}}
{{define "content"}}
<h1>Audit log</h1>
<form method="get" action="/admin/audit">
  <input name="target" value="{{.Data.Target}}" placeholder="target filter, e.g. account:7">
  <button type="submit">Filter</button>
</form>
<table><tr><th>When</th><th>Actor</th><th>Action</th><th>Target</th><th>Details</th></tr>
{{range .Data.Entries}}
<tr><td>{{.CreatedAt.Format "2006-01-02 15:04:05"}}</td>
<td>{{if .Actor.Valid}}{{.Actor.Int64}}{{else}}system{{end}}</td>
<td>{{.Action}}</td><td>{{.Target}}</td><td>{{.Details}}</td></tr>
{{end}}</table>
{{end}}
```

`modules/account/handlers_admin.go`:

```go
// modules/account/handlers_admin.go
package account

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
)

type adminSearchData struct {
	Query   string
	Results []PortalAccount
}

type adminAccountData struct {
	Acct       *PortalAccount
	Groups     []string
	Approved   bool
	Identities []Identity
	Characters []Character
	Audit      []AuditEntry
}

type adminAuditData struct {
	Target  string
	Entries []AuditEntry
}

func (p *portal) handleAdminSearch(w http.ResponseWriter, r *http.Request) {
	data := adminSearchData{Query: r.URL.Query().Get("q")}
	if data.Query != "" {
		results, err := p.store.SearchAccounts(r.Context(), data.Query)
		if err != nil {
			p.log.Error("admin search", slog.Any("err", err))
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		data.Results = results
	}
	p.render(w, r, "admin_search.html", data)
}

// adminTarget loads the {id} path account or writes 404.
func (p *portal) adminTarget(w http.ResponseWriter, r *http.Request) (*PortalAccount, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return nil, false
	}
	acct, err := p.store.AccountByID(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		http.NotFound(w, r)
		return nil, false
	}
	if err != nil {
		p.log.Error("admin target", slog.Any("err", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return nil, false
	}
	return acct, true
}

func (p *portal) handleAdminAccount(w http.ResponseWriter, r *http.Request) {
	acct, ok := p.adminTarget(w, r)
	if !ok {
		return
	}
	groups, err := p.store.GroupsByAccount(r.Context(), acct.ID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	ids, err := p.store.IdentitiesByAccount(r.Context(), acct.ID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	chars, err := p.store.CharactersByAccount(r.Context(), acct.ID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	audit, err := p.store.RecentAudit(r.Context(), 25, fmt.Sprintf("account:%d", acct.ID))
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	approved := false
	for _, g := range groups {
		if g == GroupManuallyApproved {
			approved = true
		}
	}
	p.render(w, r, "admin_account.html", adminAccountData{
		Acct: acct, Groups: groups, Approved: approved,
		Identities: ids, Characters: chars, Audit: audit,
	})
}

// adminAction wraps the shared shape of the POST actions: resolve
// target, run the mutation, audit as the acting admin, bounce back.
func (p *portal) adminAction(action string, mutate func(r *http.Request, target *PortalAccount) (details string, err error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		target, ok := p.adminTarget(w, r)
		if !ok {
			return
		}
		details, err := mutate(r, target)
		if err != nil {
			p.log.Error("admin action failed", slog.String("action", action), slog.Any("err", err))
			http.Redirect(w, r, fmt.Sprintf("/admin/accounts/%d?msg=Action+failed:+%s", target.ID, action), http.StatusFound)
			return
		}
		admin := ctxAccount(r)
		if err := p.store.AppendAudit(r.Context(), admin.ID, action,
			fmt.Sprintf("account:%d", target.ID), details); err != nil {
			p.log.Warn("audit failed", slog.Any("err", err))
		}
		http.Redirect(w, r, fmt.Sprintf("/admin/accounts/%d?msg=Done", target.ID), http.StatusFound)
	}
}

func (p *portal) handleAdminAudit(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("target")
	entries, err := p.store.RecentAudit(r.Context(), 100, target)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	p.render(w, r, "admin_audit.html", adminAuditData{Target: target, Entries: entries})
}
```

Routes (the closures give each action its audit name):

```go
	mux.HandleFunc("GET /admin", p.admin(p.handleAdminSearch))
	mux.HandleFunc("GET /admin/accounts/{id}", p.admin(p.handleAdminAccount))
	mux.HandleFunc("GET /admin/audit", p.admin(p.handleAdminAudit))
	mux.HandleFunc("POST /admin/accounts/{id}/group", p.admin(p.adminAction("group.set",
		func(r *http.Request, target *PortalAccount) (string, error) {
			group := r.FormValue("group")
			if group != GroupManuallyApproved && group != GroupAdmin {
				return "", fmt.Errorf("unknown group %q", group)
			}
			member := r.FormValue("member") == "on"
			admin := ctxAccount(r)
			if member {
				return group + "=true", p.store.AddGroupMember(r.Context(), group, target.ID, admin.ID)
			}
			return group + "=false", p.store.RemoveGroupMember(r.Context(), group, target.ID)
		})))
	mux.HandleFunc("POST /admin/accounts/{id}/status", p.admin(p.adminAction("account.status",
		func(r *http.Request, target *PortalAccount) (string, error) {
			status := r.FormValue("status")
			if err := p.store.SetAccountStatus(r.Context(), target.ID, status); err != nil {
				return "", err
			}
			if status == StatusDisabled {
				return status, p.store.DeleteAccountSessions(r.Context(), target.ID)
			}
			return status, nil
		})))
	mux.HandleFunc("POST /admin/accounts/{id}/unlink", p.admin(p.adminAction("identity.unlink",
		func(r *http.Request, target *PortalAccount) (string, error) {
			provider := r.FormValue("provider")
			return provider, p.store.RevokeIdentity(r.Context(), target.ID, provider)
		})))
	mux.HandleFunc("POST /admin/accounts/{id}/release", p.admin(p.adminAction("identity.release",
		func(r *http.Request, target *PortalAccount) (string, error) {
			provider, uid := r.FormValue("provider"), r.FormValue("provider_user_id")
			return provider + ":" + uid, p.store.ReleaseIdentity(r.Context(), provider, uid)
		})))
	mux.HandleFunc("POST /admin/accounts/{id}/resend-verification", p.admin(p.adminAction("account.resend_verification",
		func(r *http.Request, target *PortalAccount) (string, error) {
			return "", p.sendVerificationEmail(r.Context(), target)
		})))
	mux.HandleFunc("POST /admin/accounts/{id}/send-reset", p.admin(p.adminAction("account.reset_password",
		func(r *http.Request, target *PortalAccount) (string, error) {
			return "reset link mailed", p.sendResetEmail(r.Context(), target)
		})))
```

- [ ] **Step 4: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/account/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/account/handlers_admin.go modules/account/handlers_admin_test.go \
        modules/account/templates/pages/admin_search.html modules/account/templates/pages/admin_account.html \
        modules/account/templates/pages/admin_audit.html modules/account/portal.go
git commit --no-gpg-sign -m "feat(account): admin portal — search, account detail actions, audit view

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 21: Cross-module e2e + final verification

**Files:**
- Create: `modules/login/e2e_account_test.go`
- Test: full-repo gates

**Interfaces:**
- Consumes: everything. `modules/login` test imports `modules/account` (no cycle: account never imports login) plus `loginpb`/`accountpb`.

- [ ] **Step 1: Write the e2e test**

```go
// modules/login/e2e_account_test.go
package login_test

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/zsrv/goscape/modules/account"
	"github.com/zsrv/goscape/modules/login"
	"github.com/zsrv/goscape/pkg/dskit/services"
	"github.com/zsrv/goscape/pkg/gamedb"
	"github.com/zsrv/goscape/pkg/loginpb"
)

func freePort(t *testing.T) int {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()
	return lis.Addr().(*net.TCPAddr).Port
}

// TestE2E_PortalToGameLogin runs the whole story on real listeners:
// register → verify (link from log-mailer is unavailable, so verify via
// DB) → approve → create character in the portal → PlayerLogin through
// the login module in account mode → NEW_PLAYER.
func TestE2E_PortalToGameLogin(t *testing.T) {
	dir := t.TempDir()
	var dbCfg gamedb.Config
	dbCfg.Backend = gamedb.BackendSQLite
	dbCfg.SQLite.DSN = filepath.Join(dir, "e2e.db")
	logger := slog.Default()

	// Migrate (the app's database module normally does this).
	db, err := gamedb.Open(dbCfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	// Account module on real ports.
	acctHTTP, acctGRPC := freePort(t), freePort(t)
	acctCfg := account.NewTestConfig() // see note below
	acctCfg.Enable = true
	acctCfg.HTTPListenPort = acctHTTP
	acctCfg.GRPCListenPort = acctGRPC
	acctCfg.PublicURL = fmt.Sprintf("http://127.0.0.1:%d", acctHTTP)
	acctMod, err := account.New(acctCfg, dbCfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := services.StartAndAwaitRunning(t.Context(), acctMod); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = services.StopAndAwaitTerminated(t.Context(), acctMod) })

	// Login module in account mode on a real port.
	loginPort := freePort(t)
	loginCfg := login.NewTestConfig() // see note below
	loginCfg.Enable = true
	loginCfg.GRPCListenPort = loginPort
	loginCfg.SavePath = filepath.Join(dir, "players")
	loginCfg.AuthMode = login.AuthModeAccount
	loginCfg.AccountGRPCAddress = fmt.Sprintf("127.0.0.1:%d", acctGRPC)
	loginCfg.AutoRegister = false
	loginMod, err := login.New(loginCfg, dbCfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := services.StartAndAwaitRunning(t.Context(), loginMod); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = services.StopAndAwaitTerminated(t.Context(), loginMod) })

	// --- Portal: register + create character ---
	base := fmt.Sprintf("http://127.0.0.1:%d", acctHTTP)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	if _, err := client.PostForm(base+"/register", url.Values{
		"email": {"e2e@example.com"}, "password": {"hunter22!"}, "password2": {"hunter22!"},
	}); err != nil {
		t.Fatal(err)
	}
	// Verify + approve directly in the DB (mail went to the log mailer).
	if _, err := db.ExecContext(t.Context(),
		`UPDATE portal_account SET email_verified = 1 WHERE email = 'e2e@example.com'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `
		INSERT INTO portal_group_member (group_id, account_id, added_by, added_at)
		SELECT g.id, a.id, NULL, '2026-07-19 00:00:00'
		FROM portal_group g, portal_account a
		WHERE g.name = 'manually_approved' AND a.email = 'e2e@example.com'`); err != nil {
		t.Fatal(err)
	}
	// Log in and create the character through the real portal.
	if _, err := client.PostForm(base+"/login", url.Values{
		"email": {"e2e@example.com"}, "password": {"hunter22!"},
	}); err != nil {
		t.Fatal(err)
	}
	var raw string
	u, _ := url.Parse(base)
	for _, c := range jar.Cookies(u) {
		if c.Name == "goscape_session" {
			raw = c.Value
		}
	}
	if raw == "" {
		t.Fatal("no portal session")
	}
	resp, err := client.PostForm(base+"/characters/new", url.Values{
		"name": {"e2ehero"}, "csrf": {account.CSRFTokenForTest(raw)}, // see note below
	})
	if err != nil || resp.StatusCode != http.StatusFound {
		t.Fatalf("character create: %v %d", err, resp.StatusCode)
	}

	// --- Game side: PlayerLogin over the login module's real gRPC ---
	conn, err := grpc.NewClient(fmt.Sprintf("127.0.0.1:%d", loginPort),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	lc := loginpb.NewLoginServiceClient(conn)

	lresp, err := lc.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
		NodeId: 10, Profile: "main", Username: "e2ehero",
		Password: "hunter22!", RemoteAddress: "127.0.0.1:9", Uid: 1,
	})
	if err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if lresp.Result != loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER {
		t.Fatalf("result = %v, want NEW_PLAYER", lresp.Result)
	}

	// Wrong password fails; wrong case fails (case-sensitive in account mode).
	for _, pw := range []string{"wrong", "HUNTER22!"} {
		lresp, err = lc.PlayerLogin(t.Context(), &loginpb.PlayerLoginRequest{
			NodeId: 10, Profile: "main", Username: "e2ehero",
			Password: pw, RemoteAddress: "127.0.0.1:9", Uid: 1,
		})
		if err != nil || lresp.Result != loginpb.LoginResult_LOGIN_RESULT_INVALID_CREDENTIALS {
			t.Fatalf("pw %q: %v %v", pw, lresp.Result, err)
		}
	}
}
```

Two tiny exported test hooks are needed in `modules/account` (add in this task, file `modules/account/export_test_helpers.go` — deliberately exported, documented as test support):

```go
// modules/account/export_test_helpers.go
package account

import "flag"

// NewTestConfig returns a Config at flag defaults. Exported for
// cross-module integration tests (modules/login e2e); production code
// builds Config through the app's flag/YAML pipeline instead.
func NewTestConfig() Config {
	var c Config
	fs := flag.NewFlagSet("", flag.PanicOnError)
	c.RegisterFlagsAndApplyDefaults(fs)
	return c
}

// CSRFTokenForTest derives the portal CSRF token for a raw session
// cookie value. Exported for cross-module integration tests only.
func CSRFTokenForTest(rawSessionToken string) string { return csrfToken(rawSessionToken) }
```

Add the mirror `login.NewTestConfig()` in `modules/login/export_test_helpers.go` the same way (defaults via RegisterFlagsAndApplyDefaults). If `modules/login` already exposes an equivalent helper in its test files, reuse that instead of adding a new one — check first.

- [ ] **Step 2: Run the e2e test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/login/ -run TestE2E_PortalToGameLogin -v`
Expected: PASS.

- [ ] **Step 3: Full-repo gates**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run '^$' ./...   # compile-all
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...             # full suite
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/account/ ./modules/login/ ./cmd/...
gofmt -l modules/account modules/login cmd pkg/gamedb | tee /dev/stderr | wc -l   # must print 0
CGO_ENABLED=1 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/account/ ./modules/login/
```
Expected: all green; gofmt lists nothing. (Known pre-existing failure on this branch: `TestNAI128_RatLootCascade` — if it appears, it is NOT caused by this work; everything else must pass.)

- [ ] **Step 4: Commit**

```bash
git add modules/login/e2e_account_test.go modules/account/export_test_helpers.go \
        modules/login/export_test_helpers.go
git commit --no-gpg-sign -m "test(login): e2e — portal registration through account-mode game login

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Post-implementation

- **Smoke test:** hand off to the user to launch `--target all` with an account-enabled config (smoke-test servers are user-launched in this project). Walk-through: register → verify (log-mailer link in server log) → `goscape-cli account bootstrap-admin` → approve → create character → Java client login with character name + account password.
- **Not in scope** (spec's out-of-scope list): TOTP/2FA, guild checks, self-unlink, legacy migration, JSON API, Helm chart wiring for the new ports (follow-up if wanted).
- **Fidelity note:** `login.auth_mode` defaults to `local`; no parity baseline touches any of this.

