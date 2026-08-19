# Hiscores Web API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `modules/hiscore`, a read-only JSON API over the existing `hiscore` / `hiscore_large` central-database tables, designed to sit behind a Kong OSS gateway.

**Architecture:** A new dskit module beside `account`, deps `{Common, Database}`, with its own `gamedb` pool and an HTTP listener built on `pkg/dskit/server`. Ranks are computed on read in SQL against new indexes; the module is anonymous-safe and treats all gateway-supplied headers as untrusted. The Helm chart gains optional Kong Ingress Controller *configuration* (Kong itself stays a prerequisite).

**Tech Stack:** Go 1.26+, `pkg/dskit/server` + `pkg/dskit/services`, `pkg/gamedb` (`database/sql`, modernc SQLite + pgx Postgres), `pkg/objtype`, `pkg/util/jstring`, golang-migrate (embedded), Helm.

**Spec:** `docs/superpowers/specs/2026-08-19-hiscore-api-design.md`

## Global Constraints

- **Branch:** `rev-274`. Backport to rev-254 / rev-245.2 / rev-244 / rev-225 is a separate follow-up effort, out of scope here.
- **Go invocations must be prefixed:** `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...` (per CLAUDE.md).
- **All commits use `git commit --no-gpg-sign`** (per CLAUDE.md).
- **XP is stored as fixed-point tenths (`×10`).** The store layer carries the raw `×10` value in fields named `ValueX10`. Conversion to whole XP (`/10`) happens **only** in the HTTP encoding layer. Never divide in the store; never expose `×10` over HTTP.
- **`type` encoding:** `0` = overall (table `hiscore_large`); `stat + 1` = per-stat (table `hiscore`).
- **Visibility rule, used identically everywhere:** a row is visible iff its account has `staff_mod_level <= 1` AND (`banned_until IS NULL` OR `banned_until < now`). This must appear in the leaderboard query, the rank-counting subquery, and the account lookup — a mismatch produces ranks that disagree between endpoints.
- **Total ordering, used identically everywhere:** `value DESC, date ASC, account_id ASC`. Ranks are 1-based and unique.
- **Table names are compile-time constants only.** `hiscore` / `hiscore_large` are selected by a Go `switch` on type, never interpolated from request input.
- **SQL is written with `?` placeholders and passed through `db.Rebind(...)`** so it runs on both backends. Argument order must match placeholder order **including placeholders inside subqueries**.
- **No production code may import `pkg/gamedb/gamedbtest`.**
- **Never call `t.Context()` inside `t.Cleanup`** (documented schema-leak trap in `gamedbtest`).
- **This subsystem is a goscape extension with no Engine-TS counterpart.** It is outside the TS fidelity ledger.

---

## File Structure

**Created:**

| File | Responsibility |
|---|---|
| `pkg/gamedb/migrations/sqlite/000004_hiscore_indexes.up.sql` | ranking + lookup indexes (SQLite) |
| `pkg/gamedb/migrations/postgres/000004_hiscore_indexes.up.sql` | same (Postgres) |
| `modules/hiscore/skills.go` | `type` ↔ skill-name mapping derived from `objtype` |
| `modules/hiscore/config.go` | `Config`, flags/defaults, `Validate` |
| `modules/hiscore/store.go` | every SQL query; the only file that writes SQL |
| `modules/hiscore/cursor.go` | opaque cursor encode/decode |
| `modules/hiscore/gateway.go` | caller identity + backstop limiter |
| `modules/hiscore/handlers.go` | routing, JSON encoding, caps, cache headers |
| `modules/hiscore/hiscore.go` | module struct, `New`, dskit service wiring |
| `modules/hiscore/*_test.go` | per-file test suites |
| `modules/hiscore/testing_test.go` | shared test helpers (DB, fixtures) |
| `production/helm/goscape/templates/hiscore-kong.yaml` | Kong config objects |
| `docs/guides/hiscores-api.md` | user-facing guide (path confirmed in Task 15) |

**Modified:** `cmd/goscape/app/modules.go`, `cmd/goscape/app/config.go`, `cmd/goscape/app/app.go`, `examples/full-config-reference.yaml`, `examples/bundled/goscape.yaml`, `production/helm/goscape/values.yaml`, `production/helm/goscape/values.schema.json`, `docs/PORTING.md`.

## Shared Type Reference

Every task below uses these exact names. Defined across Tasks 2–8; repeated here so any task read in isolation has them.

```go
// skills.go
const SkillOverall = "overall"
type Skill struct {
    Type int    // 0 = overall, else stat+1
    Name string // canonical lowercase
}
func Skills() []Skill                        // the 19 enabled stats, excludes overall
func SkillByName(name string) (Skill, bool)  // accepts "overall" too
func TableForType(typ int) string            // "hiscore_large" for 0, else "hiscore"

// store.go
var ErrNotFound = errors.New("hiscore: not found")
type Store struct{ db *gamedb.DB }
func NewStore(db *gamedb.DB) *Store
type Account struct {
    ID       int64
    Username string // base37 safe name as stored
}
type Entry struct {
    Type      int
    Rank      int64
    Level     int
    ValueX10  int64
    UpdatedAt time.Time
}
type Row struct {
    AccountID int64
    Username  string
    Rank      int64
    Level     int
    ValueX10  int64
    UpdatedAt time.Time
}
type Card struct {
    Account Account
    Overall *Entry  // nil when no hiscore_large row
    Skills  []Entry // sparse — only stats with rows
}
func (s *Store) LookupAccountByName(ctx context.Context, safeName string, now time.Time) (Account, error)
func (s *Store) PlayerCard(ctx context.Context, profile string, accountID int64, now time.Time) (Card, error)
func (s *Store) LeaderboardByOffset(ctx context.Context, profile string, typ, offset, limit int, now time.Time) ([]Row, error)
func (s *Store) LeaderboardByCursor(ctx context.Context, profile string, typ int, cur Cursor, limit int, now time.Time) ([]Row, error)

// cursor.go
var ErrBadCursor = errors.New("hiscore: malformed cursor")
type Cursor struct {
    ValueX10  int64
    UpdatedAt time.Time
    AccountID int64
    Rank      int64 // rank of the next row to return; 0 = start of board
}
func (c Cursor) IsStart() bool
func (c Cursor) Encode() string
func DecodeCursor(s string) (Cursor, error)

// gateway.go
type caller struct {
    Consumer  string // "" when unknown
    Anonymous bool
    IP        string
}
func (c caller) limiterKey() string
func consumerFromHeaders(r *http.Request, trust bool) caller
func (a *api) identify(r *http.Request) caller
type backstop struct{ /* unexported */ }
func newBackstop(perMinute int) *backstop
func (b *backstop) allow(key string, now time.Time) bool

// handlers.go
type api struct {
    cfg       Config
    store     *Store
    sourceIPs *middleware.SourceIPExtractor
    limiter   *backstop
    now       func() time.Time
    log       *slog.Logger
}
func newAPI(cfg Config, store *Store, log *slog.Logger) (*api, error)
func (a *api) register(mux *http.ServeMux)
```

---

## Task 1: Migration — ranking indexes

**Files:**
- Create: `pkg/gamedb/migrations/sqlite/000004_hiscore_indexes.up.sql`
- Create: `pkg/gamedb/migrations/postgres/000004_hiscore_indexes.up.sql`
- Test: `pkg/gamedb/migrate_test.go` (modify — add one test)

**Interfaces:**
- Consumes: nothing.
- Produces: four indexes — `idx_hiscore_rank`, `idx_hiscore_account`, `idx_hiscore_large_rank`, `idx_hiscore_large_account`.

Note: this repo's migration lineage has **no `.down.sql` files**. Do not create any.

- [ ] **Step 1: Write the failing test**

Append to `pkg/gamedb/migrate_test.go`:

```go
// TestMigrate_HiscoreIndexes pins the ranking indexes added in 000004.
// The hiscore API's leaderboard ordering and rank counting both depend
// on them; without them every leaderboard page is a full sort.
func TestMigrate_HiscoreIndexes(t *testing.T) {
	db := newTestDB(t)

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
```

If `newTestDB` is not the helper name already present in `pkg/gamedb/migrate_test.go`, use whatever local helper that file already uses to obtain a migrated `*gamedb.DB`; do not add a second helper.

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamedb/ -run TestMigrate_HiscoreIndexes -v`

Expected: FAIL — `index idx_hiscore_rank missing after migrate: sql: no rows in result set`

- [ ] **Step 3: Write the SQLite migration**

Create `pkg/gamedb/migrations/sqlite/000004_hiscore_indexes.up.sql`:

```sql
-- Ranking + lookup indexes for the hiscore read API (modules/hiscore).
-- Spec: docs/superpowers/specs/2026-08-19-hiscore-api-design.md
--
-- goscape extension over TS: Engine-TS has no hiscore serving endpoint,
-- so its prisma schemas declare no such indexes. Purely additive — no
-- behavioural change to the write path in modules/login.
--
-- The *_rank indexes match the API's total ordering exactly
-- (value DESC, date ASC, account_id ASC) so leaderboard pages and the
-- rank COUNT are served by an index range scan rather than a sort.
--
-- The *_account indexes serve the player-card lookup: the existing PK is
-- (profile, type, account_id), which cannot serve a query that knows
-- profile + account_id but not type.

CREATE INDEX idx_hiscore_rank
    ON hiscore (profile, type, value DESC, date ASC, account_id ASC);

CREATE INDEX idx_hiscore_account
    ON hiscore (profile, account_id);

CREATE INDEX idx_hiscore_large_rank
    ON hiscore_large (profile, type, value DESC, date ASC, account_id ASC);

CREATE INDEX idx_hiscore_large_account
    ON hiscore_large (profile, account_id);
```

- [ ] **Step 4: Write the Postgres migration**

Create `pkg/gamedb/migrations/postgres/000004_hiscore_indexes.up.sql` with **identical content** (the DDL above is valid on both engines verbatim, including the `DESC`/`ASC` column modifiers).

- [ ] **Step 5: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamedb/ -run TestMigrate_HiscoreIndexes -v`

Expected: PASS

- [ ] **Step 6: Run the whole gamedb suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamedb/...`

Expected: PASS. If a test asserts a specific migration count or latest version, update it to 4.

- [ ] **Step 7: Commit**

```bash
git add pkg/gamedb/migrations/sqlite/000004_hiscore_indexes.up.sql \
        pkg/gamedb/migrations/postgres/000004_hiscore_indexes.up.sql \
        pkg/gamedb/migrate_test.go
git commit --no-gpg-sign -m "feat(gamedb): migration 000004 — hiscore ranking indexes"
```

---

## Task 2: Skill mapping

**Files:**
- Create: `modules/hiscore/skills.go`
- Test: `modules/hiscore/skills_test.go`

**Interfaces:**
- Consumes: `objtype.PlayerStatNames`, `objtype.PlayerStatEnabled`, `objtype.PlayerStatCount`.
- Produces: `SkillOverall`, `Skill`, `Skills()`, `SkillByName(string) (Skill, bool)`, `TableForType(int) string`.

- [ ] **Step 1: Write the failing test**

Create `modules/hiscore/skills_test.go`:

```go
package hiscore

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestSkills_MatchesEnabledStats(t *testing.T) {
	got := Skills()
	if len(got) != 19 {
		t.Fatalf("Skills(): got %d entries, want 19 enabled stats", len(got))
	}
	for _, s := range got {
		stat := s.Type - 1
		if stat < 0 || stat >= objtype.PlayerStatCount {
			t.Errorf("skill %q: type %d out of stat range", s.Name, s.Type)
			continue
		}
		if !objtype.PlayerStatEnabled[stat] {
			t.Errorf("skill %q: type %d maps to disabled stat %d", s.Name, s.Type, stat)
		}
		if s.Name != objtype.PlayerStatNames[stat] {
			t.Errorf("skill type %d: name %q, want %q", s.Type, s.Name, objtype.PlayerStatNames[stat])
		}
	}
}

func TestSkills_ExcludesOverall(t *testing.T) {
	for _, s := range Skills() {
		if s.Type == 0 || s.Name == SkillOverall {
			t.Fatalf("Skills() must not contain overall, got %+v", s)
		}
	}
}

func TestSkillByName(t *testing.T) {
	tests := []struct {
		name     string
		wantType int
		wantOK   bool
	}{
		{"overall", 0, true},
		{"attack", objtype.PlayerStatAttack + 1, true},
		{"runecraft", objtype.PlayerStatRunecraft + 1, true},
		{"stat18", 0, false},  // disabled reserved slot
		{"stat19", 0, false},  // disabled reserved slot
		{"Attack", 0, false},  // case-sensitive: callers normalize
		{"nonsense", 0, false},
		{"", 0, false},
	}
	for _, tc := range tests {
		got, ok := SkillByName(tc.name)
		if ok != tc.wantOK {
			t.Errorf("SkillByName(%q): ok = %v, want %v", tc.name, ok, tc.wantOK)
			continue
		}
		if ok && got.Type != tc.wantType {
			t.Errorf("SkillByName(%q): type = %d, want %d", tc.name, got.Type, tc.wantType)
		}
	}
}

func TestTableForType(t *testing.T) {
	if got := TableForType(0); got != "hiscore_large" {
		t.Errorf("TableForType(0) = %q, want hiscore_large", got)
	}
	for _, typ := range []int{1, 5, 21} {
		if got := TableForType(typ); got != "hiscore" {
			t.Errorf("TableForType(%d) = %q, want hiscore", typ, got)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/hiscore/ -run TestSkill -v`

Expected: FAIL — build error, `undefined: Skills`

- [ ] **Step 3: Write the implementation**

Create `modules/hiscore/skills.go`:

```go
// Package hiscore is the hiscores read API: a public, anonymous-safe
// JSON surface over the hiscore / hiscore_large central-database tables
// that modules/login populates on logout.
//
// This subsystem is a goscape extension. Engine-TS has no hiscore
// serving endpoint at any pinned revision, so it sits outside the TS
// fidelity ledger.
//
// Spec: docs/superpowers/specs/2026-08-19-hiscore-api-design.md
package hiscore

import (
	"sync"

	"github.com/zsrv/goscape/pkg/objtype"
)

// SkillOverall is the path/name selector for the aggregate board
// (hiscore type 0, stored in hiscore_large).
const SkillOverall = "overall"

// Skill is one selectable board.
type Skill struct {
	// Type is the hiscore `type` column: 0 for overall, stat+1 otherwise.
	Type int `json:"type"`
	// Name is the canonical lowercase selector.
	Name string `json:"name"`
}

var (
	skillsOnce sync.Once
	skillList  []Skill
	skillIndex map[string]Skill
)

// initSkills derives the board list from objtype rather than restating
// it, so the API and the write path in modules/login cannot drift.
func initSkills() {
	skillsOnce.Do(func() {
		skillIndex = map[string]Skill{SkillOverall: {Type: 0, Name: SkillOverall}}
		for stat := range objtype.PlayerStatCount {
			if !objtype.PlayerStatEnabled[stat] {
				continue
			}
			s := Skill{Type: stat + 1, Name: objtype.PlayerStatNames[stat]}
			skillList = append(skillList, s)
			skillIndex[s.Name] = s
		}
	})
}

// Skills returns the enabled per-stat boards in stat order. It excludes
// overall, which is not a stat.
func Skills() []Skill {
	initSkills()
	out := make([]Skill, len(skillList))
	copy(out, skillList)
	return out
}

// SkillByName resolves a selector, accepting "overall" in addition to
// the enabled stat names. Matching is exact and case-sensitive; callers
// normalize request input before calling.
func SkillByName(name string) (Skill, bool) {
	initSkills()
	s, ok := skillIndex[name]
	return s, ok
}

// TableForType returns the table holding rows of the given type. The
// result is a compile-time constant string, never request-derived, and
// is the only way a table name reaches a query.
func TableForType(typ int) string {
	if typ == 0 {
		return "hiscore_large"
	}
	return "hiscore"
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/hiscore/ -run TestSkill -v`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add modules/hiscore/skills.go modules/hiscore/skills_test.go
git commit --no-gpg-sign -m "feat(hiscore): skill mapping derived from objtype"
```

---

## Task 3: Config

**Files:**
- Create: `modules/hiscore/config.go`
- Test: `modules/hiscore/config_test.go`

**Interfaces:**
- Consumes: `server.Config` from `pkg/dskit/server`.
- Produces: `Config`, `(*Config).RegisterFlagsAndApplyDefaults(*flag.FlagSet)`, `(*Config).Validate() error`.

Note: log level lives in the embedded `server.Config.LogLevel` (the `ondemand` pattern), **not** as a separate field.

- [ ] **Step 1: Write the failing test**

Create `modules/hiscore/config_test.go`:

```go
package hiscore

import (
	"flag"
	"strings"
	"testing"
	"time"
)

func defaultConfig(t *testing.T) Config {
	t.Helper()
	var cfg Config
	fs := flag.NewFlagSet("test", flag.PanicOnError)
	cfg.RegisterFlagsAndApplyDefaults(fs)
	return cfg
}

func TestConfig_Defaults(t *testing.T) {
	cfg := defaultConfig(t)

	if cfg.Enable {
		t.Error("Enable: got true, want false (module off unless asked for)")
	}
	if cfg.Profile != "main" {
		t.Errorf("Profile: got %q, want main", cfg.Profile)
	}
	if cfg.CacheMaxAge != 60*time.Second {
		t.Errorf("CacheMaxAge: got %v, want 60s", cfg.CacheMaxAge)
	}
	if cfg.DefaultLimit != 25 {
		t.Errorf("DefaultLimit: got %d, want 25", cfg.DefaultLimit)
	}
	if cfg.MaxLimit != 100 {
		t.Errorf("MaxLimit: got %d, want 100", cfg.MaxLimit)
	}
	if cfg.LeaderboardMaxRank != 500000 {
		t.Errorf("LeaderboardMaxRank: got %d, want 500000", cfg.LeaderboardMaxRank)
	}
	if cfg.TrustGatewayHeaders {
		t.Error("TrustGatewayHeaders: got true, want false (safe default)")
	}
	if cfg.BackstopRate != 120 {
		t.Errorf("BackstopRate: got %d, want 120", cfg.BackstopRate)
	}
	if cfg.Server.HTTPListenPort != 8082 {
		t.Errorf("Server.HTTPListenPort: got %d, want 8082 (must not collide with portal 8081 or ondemand 8080)", cfg.Server.HTTPListenPort)
	}
	// Both blank selects dskit's built-in proxy-header chain, which is
	// what Kong populates.
	if cfg.Server.LogSourceIPsHeader != "" || cfg.Server.LogSourceIPsRegex != "" {
		t.Errorf("source IP header/regex = %q/%q, want both empty",
			cfg.Server.LogSourceIPsHeader, cfg.Server.LogSourceIPsRegex)
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{"defaults are valid", func(*Config) {}, ""},
		{"empty profile", func(c *Config) { c.Profile = "" }, "profile"},
		{"zero default limit", func(c *Config) { c.DefaultLimit = 0 }, "default_limit"},
		{"default above max", func(c *Config) { c.DefaultLimit = 500 }, "default_limit"},
		{"zero max limit", func(c *Config) { c.MaxLimit = 0 }, "max_limit"},
		{"zero max rank", func(c *Config) { c.LeaderboardMaxRank = 0 }, "leaderboard_max_rank"},
		{"negative cache age", func(c *Config) { c.CacheMaxAge = -time.Second }, "cache_max_age"},
		{"negative backstop", func(c *Config) { c.BackstopRate = -1 }, "backstop_rate"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig(t)
			tc.mutate(&cfg)
			err := cfg.Validate()
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("Validate(): unexpected error %v", err)
			case tc.wantErr != "" && err == nil:
				t.Fatalf("Validate(): got nil, want error mentioning %q", tc.wantErr)
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Fatalf("Validate(): error %q does not mention %q", err, tc.wantErr)
			}
		})
	}
}

// Zero disables the backstop limiter entirely and must stay valid.
func TestConfig_BackstopZeroIsValid(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.BackstopRate = 0
	if err := cfg.Validate(); err != nil {
		t.Fatalf("BackstopRate=0 must be valid (disables limiter), got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/hiscore/ -run TestConfig -v`

Expected: FAIL — build error, `undefined: Config`

- [ ] **Step 3: Write the implementation**

Create `modules/hiscore/config.go`:

```go
package hiscore

import (
	"errors"
	"flag"
	"fmt"
	"time"

	"github.com/zsrv/goscape/pkg/dskit/server"
)

// Config is the hiscore module's configuration. The embedded
// server.Config supplies the listener, timeouts, request logging and
// source-IP extraction (log_source_ips_header / _regex), which is how
// the real client IP is recovered from behind the gateway.
type Config struct {
	Server server.Config `yaml:",inline"`
	Enable bool          `yaml:"enable"`

	// Profile is the default profile queried when a request does not
	// specify one. Boards are per-profile, matching the write path.
	Profile string `yaml:"profile"`

	// CacheMaxAge drives Cache-Control: public, max-age=N. Responses are
	// also ETag'd; this is what makes an edge cache (Kong proxy-cache)
	// effective and is the largest single lever on database load.
	CacheMaxAge time.Duration `yaml:"cache_max_age"`

	DefaultLimit int `yaml:"default_limit"`
	MaxLimit     int `yaml:"max_limit"`

	// LeaderboardMaxRank bounds offset paging (offset+limit must not
	// exceed it). This is a product boundary — the depth of board the
	// hiscores display — not a safety valve; cursor paging is the
	// mechanism for cheap deep reads.
	LeaderboardMaxRank int `yaml:"leaderboard_max_rank"`

	// TrustGatewayHeaders enables reading Kong's X-Consumer-* headers
	// for logging. Default false: nothing is ever authorized by them, so
	// this only controls whether an unverified header can reach a log
	// line. Enable it where a gateway actually fronts the module.
	TrustGatewayHeaders bool `yaml:"trust_gateway_headers"`

	// BackstopRate is a coarse in-process request ceiling per caller per
	// minute, for the case where the module is reached without a
	// gateway. It is not the quota system — Kong's per-consumer
	// rate-limiting is. 0 disables it.
	BackstopRate int `yaml:"backstop_rate"`
}

func (c *Config) RegisterFlagsAndApplyDefaults(f *flag.FlagSet) {
	// server.Config has no flag-registration method of its own — each
	// module registers the server fields it uses under its own prefix.
	// modules/ondemand/config.go is the reference implementation.
	f.StringVar(&c.Server.HTTPListenAddress, "hiscore.http-listen-address", "127.0.0.1", "Hiscore API listen address.")
	f.StringVar(&c.Server.HTTPListenNetwork, "hiscore.http-listen-network", server.DefaultNetwork, "Hiscore API listen network.")
	f.IntVar(&c.Server.HTTPListenPort, "hiscore.http-listen-port", 8082, "Hiscore API listen port.")
	f.DurationVar(&c.Server.ServerGracefulShutdownTimeout, "hiscore.graceful-shutdown-timeout", 30*time.Second, "Timeout for graceful shutdowns.")
	f.DurationVar(&c.Server.HTTPServerReadTimeout, "hiscore.http-read-timeout", 30*time.Second, "Read timeout for the hiscore HTTP server.")
	f.DurationVar(&c.Server.HTTPServerWriteTimeout, "hiscore.http-write-timeout", 30*time.Second, "Write timeout for the hiscore HTTP server.")
	f.DurationVar(&c.Server.HTTPServerIdleTimeout, "hiscore.http-idle-timeout", 120*time.Second, "Idle timeout for the hiscore HTTP server.")

	// Source-IP extraction is how the real client IP is recovered from
	// behind the gateway. Both blank selects dskit's built-in
	// Forwarded / X-Real-IP / X-Forwarded-For chain, which is what Kong
	// populates — deliberately unlike ondemand, which defaults to
	// cf-connecting-ip for its Cloudflare-fronted deployment.
	f.BoolVar(&c.Server.LogSourceIPs, "hiscore.log-source-ips-enabled", true, "Log source IPs on hiscore API requests.")
	f.StringVar(&c.Server.LogSourceIPsHeader, "hiscore.log-source-ips-header", "", "Header holding the source IP. Blank uses the built-in Forwarded/X-Real-IP/X-Forwarded-For chain.")
	f.StringVar(&c.Server.LogSourceIPsRegex, "hiscore.log-source-ips-regex", "", "Regex for extracting the source IP from the configured header. Must be set together with the header.")
	f.BoolVar(&c.Server.LogSourceIPsFull, "hiscore.log-source-ips-full", false, "Log all source IPs instead of the first match.")

	f.BoolVar(&c.Enable, "hiscore.enable", false, "Whether to run the hiscore API module.")
	f.StringVar(&c.Profile, "hiscore.profile", "main", "Default profile queried by the hiscore API.")
	f.DurationVar(&c.CacheMaxAge, "hiscore.cache-max-age", 60*time.Second, "Cache-Control max-age on hiscore API responses.")
	f.IntVar(&c.DefaultLimit, "hiscore.default-limit", 25, "Default leaderboard page size.")
	f.IntVar(&c.MaxLimit, "hiscore.max-limit", 100, "Maximum leaderboard page size.")
	f.IntVar(&c.LeaderboardMaxRank, "hiscore.leaderboard-max-rank", 500000, "Deepest rank reachable by offset paging.")
	f.BoolVar(&c.TrustGatewayHeaders, "hiscore.trust-gateway-headers", false, "Read gateway-supplied X-Consumer-* headers for logging.")
	f.IntVar(&c.BackstopRate, "hiscore.backstop-rate", 120, "In-process requests/minute per caller when no gateway limits apply. 0 disables.")
}

func (c *Config) Validate() error {
	if c.Profile == "" {
		return errors.New("hiscore: profile must not be empty")
	}
	if c.MaxLimit < 1 {
		return fmt.Errorf("hiscore: max_limit must be >= 1, got %d", c.MaxLimit)
	}
	if c.DefaultLimit < 1 || c.DefaultLimit > c.MaxLimit {
		return fmt.Errorf("hiscore: default_limit must be in [1, max_limit=%d], got %d", c.MaxLimit, c.DefaultLimit)
	}
	if c.LeaderboardMaxRank < 1 {
		return fmt.Errorf("hiscore: leaderboard_max_rank must be >= 1, got %d", c.LeaderboardMaxRank)
	}
	if c.CacheMaxAge < 0 {
		return fmt.Errorf("hiscore: cache_max_age must not be negative, got %v", c.CacheMaxAge)
	}
	if c.BackstopRate < 0 {
		return fmt.Errorf("hiscore: backstop_rate must not be negative, got %d", c.BackstopRate)
	}
	return nil
}
```

Import `"github.com/zsrv/goscape/pkg/dskit/server"` for `server.DefaultNetwork`.

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/hiscore/ -run TestConfig -v`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add modules/hiscore/config.go modules/hiscore/config_test.go
git commit --no-gpg-sign -m "feat(hiscore): module config with flags, defaults and validation"
```

---

## Task 4: Test fixtures + account lookup

**Files:**
- Create: `modules/hiscore/testing_test.go`
- Create: `modules/hiscore/store.go`
- Test: `modules/hiscore/store_test.go`

**Interfaces:**
- Consumes: `gamedb.DB`, `gamedbtest.OpenTestSchema`.
- Produces: `Store`, `NewStore`, `ErrNotFound`, `Account`, `LookupAccountByName`; test helpers `createTestDB`, `insertAccount`, `insertHiscore`.

- [ ] **Step 1: Write the test fixtures**

Create `modules/hiscore/testing_test.go`:

```go
package hiscore

import (
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/gamedb"
	"github.com/zsrv/goscape/pkg/gamedb/gamedbtest"
)

func noopLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

// createTestDB opens an isolated migrated central test DB: in-memory
// sqlite by default; the env-configured Postgres (unique schema per
// test, dropped on cleanup) when GOSCAPE_TEST_POSTGRES_DSN is set, so
// the whole suite can run against the real backend. Mirrors
// modules/login/db_test.go:createTestDB.
func createTestDB(t *testing.T) *gamedb.DB {
	t.Helper()
	if dsn := os.Getenv("GOSCAPE_TEST_POSTGRES_DSN"); dsn != "" {
		return gamedbtest.OpenTestSchema(t, dsn, t.Name(), noopLogger())
	}

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

// testClock is the fixed "now" every store/handler test uses, so
// banned_until comparisons and Last-Modified values are deterministic.
var testClock = time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)

// insertAccount inserts an account row and returns its id. staffModLevel
// > 1 and a bannedUntil at or after the test clock both make the account
// invisible to the API.
func insertAccount(t *testing.T, db *gamedb.DB, username string, staffModLevel int, bannedUntil *time.Time) int64 {
	t.Helper()
	res, err := db.ExecContext(t.Context(), db.Rebind(
		`INSERT INTO account (username, password, registration_ip, staff_mod_level, members, banned_until)
		 VALUES (?, ?, ?, ?, ?, ?)`),
		username, "x", "127.0.0.1", staffModLevel, 0, bannedUntil)
	if err != nil {
		t.Fatalf("insertAccount(%s): %v", username, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		// Postgres' pgx driver does not support LastInsertId; read it back.
		var back int64
		if qerr := db.QueryRowContext(t.Context(), db.Rebind(
			`SELECT id FROM account WHERE username = ?`), username).Scan(&back); qerr != nil {
			t.Fatalf("insertAccount(%s): id lookup: %v", username, qerr)
		}
		return back
	}
	return id
}

// insertHiscore writes one leaderboard row. valueX10 is the raw
// fixed-point tenths value, exactly as modules/login stores it.
func insertHiscore(t *testing.T, db *gamedb.DB, table string, accountID int64, profile string, typ, level int, valueX10 int64, date time.Time) {
	t.Helper()
	q := `INSERT INTO hiscore (account_id, profile, type, level, value, date) VALUES (?, ?, ?, ?, ?, ?)`
	if table == "hiscore_large" {
		q = `INSERT INTO hiscore_large (account_id, profile, type, level, value, date) VALUES (?, ?, ?, ?, ?, ?)`
	}
	if _, err := db.ExecContext(t.Context(), db.Rebind(q),
		accountID, profile, typ, level, valueX10, date); err != nil {
		t.Fatalf("insertHiscore(%s, acct=%d, type=%d): %v", table, accountID, typ, err)
	}
}
```

- [ ] **Step 2: Write the failing test**

Create `modules/hiscore/store_test.go`:

```go
package hiscore

import (
	"errors"
	"testing"
	"time"
)

func TestLookupAccountByName(t *testing.T) {
	db := createTestDB(t)
	store := NewStore(db)

	past := testClock.Add(-24 * time.Hour)
	future := testClock.Add(24 * time.Hour)

	insertAccount(t, db, "zezima", 0, nil)
	insertAccount(t, db, "modash", 2, nil)              // staff — hidden
	insertAccount(t, db, "cheater", 0, &future)         // banned — hidden
	insertAccount(t, db, "reformed", 0, &past)          // ban expired — visible

	tests := []struct {
		name    string
		lookup  string
		wantErr error
	}{
		{"plain account", "zezima", nil},
		{"staff hidden", "modash", ErrNotFound},
		{"active ban hidden", "cheater", ErrNotFound},
		{"expired ban visible", "reformed", nil},
		{"unknown", "nobody", ErrNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := store.LookupAccountByName(t.Context(), tc.lookup, testClock)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("LookupAccountByName(%q): err = %v, want %v", tc.lookup, err, tc.wantErr)
			}
			if tc.wantErr == nil && got.Username != tc.lookup {
				t.Errorf("username = %q, want %q", got.Username, tc.lookup)
			}
			if tc.wantErr == nil && got.ID == 0 {
				t.Error("ID = 0, want a real account id")
			}
		})
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/hiscore/ -run TestLookupAccountByName -v`

Expected: FAIL — build error, `undefined: NewStore`

- [ ] **Step 4: Write the implementation**

Create `modules/hiscore/store.go`:

```go
package hiscore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/zsrv/goscape/pkg/gamedb"
)

// ErrNotFound means the requested account or board row is absent, or is
// hidden by the visibility rules. The two are deliberately
// indistinguishable to callers: a hidden account must not be
// detectable through the API.
var ErrNotFound = errors.New("hiscore: not found")

// Store is the module's only SQL surface.
type Store struct{ db *gamedb.DB }

func NewStore(db *gamedb.DB) *Store { return &Store{db: db} }

// Account is a visible account resolved from its safe name.
type Account struct {
	ID       int64
	Username string
}

// visibleAccount is the visibility predicate, shared verbatim by every
// query in this file. A row counts iff its account is not staff and is
// not currently banned. The write path (modules/login updateHiscores)
// applies the same rule, but only at logout — applying it again at read
// time is what makes a ban take effect on the boards immediately
// instead of at the offender's next logout.
//
// Placeholder: banned_until cutoff (a time.Time "now").
const visibleAccount = `%[1]s.staff_mod_level <= 1
	AND (%[1]s.banned_until IS NULL OR %[1]s.banned_until < ?)`

// LookupAccountByName resolves a base37 safe name to a visible account.
// The caller normalizes the name (jstring.ToSafeName) before calling.
func (s *Store) LookupAccountByName(ctx context.Context, safeName string, now time.Time) (Account, error) {
	q := fmt.Sprintf(`SELECT a.id, a.username FROM account a
		WHERE a.username = ? AND `+visibleAccount, "a")

	var acct Account
	err := s.db.QueryRowContext(ctx, s.db.Rebind(q), safeName, now).Scan(&acct.ID, &acct.Username)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("hiscore: lookup account: %w", err)
	}
	return acct, nil
}
```

Note the argument order: `safeName` binds the `username = ?` placeholder, `now` binds the one inside `visibleAccount`. `visibleAccount` is a `fmt` template over a **table alias**, never over request input.

- [ ] **Step 5: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/hiscore/ -run TestLookupAccountByName -v`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add modules/hiscore/testing_test.go modules/hiscore/store.go modules/hiscore/store_test.go
git commit --no-gpg-sign -m "feat(hiscore): store scaffolding and visible-account lookup"
```

---

## Task 5: Player card with ranks

**Files:**
- Modify: `modules/hiscore/store.go`
- Test: `modules/hiscore/store_test.go` (append)

**Interfaces:**
- Consumes: `Store`, `visibleAccount`, `ErrNotFound`, `TableForType`.
- Produces: `Entry`, `Card`, `(*Store).PlayerCard`.

- [ ] **Step 1: Write the failing test**

Append to `modules/hiscore/store_test.go`:

```go
// TestPlayerCard_RanksAndTiebreaks pins the total ordering:
// value DESC, then date ASC (first to reach it wins), then account_id
// ASC. Ranks must be 1-based, unique and gapless across visible rows.
func TestPlayerCard_RanksAndTiebreaks(t *testing.T) {
	db := createTestDB(t)
	store := NewStore(db)

	early := testClock.Add(-48 * time.Hour)
	late := testClock.Add(-1 * time.Hour)

	// Three players tied on XP; `early` beats `late`, and the pair tied
	// on both value and date falls back to account_id.
	top := insertAccount(t, db, "topper", 0, nil)     // higher XP  -> rank 1
	first := insertAccount(t, db, "firstly", 0, nil)  // tie, early  -> rank 2
	lowID := insertAccount(t, db, "aardvark", 0, nil) // tie, late   -> rank 3
	highID := insertAccount(t, db, "zulu", 0, nil)    // tie, late   -> rank 4

	const attack = 1
	insertHiscore(t, db, "hiscore", top, "main", attack, 99, 20_000_000, late)
	insertHiscore(t, db, "hiscore", first, "main", attack, 90, 10_000_000, early)
	insertHiscore(t, db, "hiscore", lowID, "main", attack, 90, 10_000_000, late)
	insertHiscore(t, db, "hiscore", highID, "main", attack, 90, 10_000_000, late)

	want := map[int64]int64{top: 1, first: 2, lowID: 3, highID: 4}
	for acctID, wantRank := range want {
		card, err := store.PlayerCard(t.Context(), "main", acctID, testClock)
		if err != nil {
			t.Fatalf("PlayerCard(%d): %v", acctID, err)
		}
		if len(card.Skills) != 1 {
			t.Fatalf("PlayerCard(%d): got %d skill entries, want 1", acctID, len(card.Skills))
		}
		if got := card.Skills[0].Rank; got != wantRank {
			t.Errorf("account %d: rank = %d, want %d", acctID, got, wantRank)
		}
	}
}

// TestPlayerCard_HiddenRowsDoNotConsumeRanks is the bug this design is
// most exposed to: if the rank subquery omits the visibility filter,
// hidden accounts silently occupy ranks and the card disagrees with the
// leaderboard.
func TestPlayerCard_HiddenRowsDoNotConsumeRanks(t *testing.T) {
	db := createTestDB(t)
	store := NewStore(db)

	future := testClock.Add(24 * time.Hour)
	staff := insertAccount(t, db, "modash", 2, nil)
	banned := insertAccount(t, db, "cheater", 0, &future)
	player := insertAccount(t, db, "zezima", 0, nil)

	const attack = 1
	insertHiscore(t, db, "hiscore", staff, "main", attack, 99, 30_000_000, testClock)
	insertHiscore(t, db, "hiscore", banned, "main", attack, 99, 25_000_000, testClock)
	insertHiscore(t, db, "hiscore", player, "main", attack, 90, 10_000_000, testClock)

	card, err := store.PlayerCard(t.Context(), "main", player, testClock)
	if err != nil {
		t.Fatalf("PlayerCard: %v", err)
	}
	if got := card.Skills[0].Rank; got != 1 {
		t.Errorf("rank = %d, want 1 — two hidden rows above must not consume ranks", got)
	}
}

// TestPlayerCard_SparseSkillsAndOverall pins that per-stat rows are
// sparse (written only at base level >= 15) while overall always exists,
// and that raw x10 values survive the store untouched.
func TestPlayerCard_SparseSkillsAndOverall(t *testing.T) {
	db := createTestDB(t)
	store := NewStore(db)

	acct := insertAccount(t, db, "zezima", 0, nil)
	insertHiscore(t, db, "hiscore_large", acct, "main", 0, 150, 5_000_000, testClock)
	insertHiscore(t, db, "hiscore", acct, "main", 1, 60, 3_000_000, testClock)
	insertHiscore(t, db, "hiscore", acct, "main", 4, 55, 2_000_000, testClock)

	card, err := store.PlayerCard(t.Context(), "main", acct, testClock)
	if err != nil {
		t.Fatalf("PlayerCard: %v", err)
	}
	if card.Overall == nil {
		t.Fatal("Overall = nil, want the hiscore_large row")
	}
	if card.Overall.ValueX10 != 5_000_000 {
		t.Errorf("Overall.ValueX10 = %d, want 5000000 (raw x10, undivided)", card.Overall.ValueX10)
	}
	if card.Overall.Rank != 1 {
		t.Errorf("Overall.Rank = %d, want 1", card.Overall.Rank)
	}
	if len(card.Skills) != 2 {
		t.Fatalf("got %d skill entries, want 2 — absent rows must not be synthesized", len(card.Skills))
	}
	if card.Skills[0].Type != 1 || card.Skills[1].Type != 4 {
		t.Errorf("skill types = %d,%d, want 1,4 in ascending order", card.Skills[0].Type, card.Skills[1].Type)
	}
	if card.Skills[0].ValueX10 != 3_000_000 {
		t.Errorf("Skills[0].ValueX10 = %d, want 3000000 (raw x10)", card.Skills[0].ValueX10)
	}
}

// A visible account that has never been exported has no rows at all.
func TestPlayerCard_NeverExported(t *testing.T) {
	db := createTestDB(t)
	store := NewStore(db)

	acct := insertAccount(t, db, "freshman", 0, nil)
	card, err := store.PlayerCard(t.Context(), "main", acct, testClock)
	if err != nil {
		t.Fatalf("PlayerCard: %v", err)
	}
	if card.Overall != nil || len(card.Skills) != 0 {
		t.Fatalf("got overall=%v skills=%d, want an empty card", card.Overall, len(card.Skills))
	}
}

// Boards are per-profile; a row under another profile is invisible here.
func TestPlayerCard_ProfileIsolation(t *testing.T) {
	db := createTestDB(t)
	store := NewStore(db)

	acct := insertAccount(t, db, "zezima", 0, nil)
	insertHiscore(t, db, "hiscore", acct, "beta", 1, 99, 13_000_000, testClock)

	card, err := store.PlayerCard(t.Context(), "main", acct, testClock)
	if err != nil {
		t.Fatalf("PlayerCard: %v", err)
	}
	if len(card.Skills) != 0 {
		t.Fatalf("got %d entries under profile main, want 0 — beta rows must not leak", len(card.Skills))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/hiscore/ -run TestPlayerCard -v`

Expected: FAIL — build error, `card.Skills undefined` / `undefined: (*Store).PlayerCard`

- [ ] **Step 3: Write the implementation**

Append to `modules/hiscore/store.go`:

```go
// Entry is one board row belonging to a specific player.
type Entry struct {
	Type      int
	Rank      int64
	Level     int
	ValueX10  int64 // raw fixed-point tenths, exactly as stored
	UpdatedAt time.Time
}

// Card is a player's full hiscore standing. Skills is sparse: it holds
// only the stats that actually have rows (the write path exports a stat
// only at base level >= 15), in ascending type order. Overall is nil
// when the player has never been exported.
type Card struct {
	Account Account
	Overall *Entry
	Skills  []Entry
}

// cardQuery ranks every row a player holds in one table. The correlated
// subquery counts visible rows strictly ahead under the total ordering
// (value DESC, date ASC, account_id ASC), so rank is 1-based and unique.
//
// The subquery repeats the visibility filter deliberately: without it,
// hidden accounts would consume ranks here but not on the leaderboard,
// and the two endpoints would disagree.
//
// %[1]s is the table name, always from TableForType (a compile-time
// constant), never from request input.
//
// Placeholder order: (1) visibility cutoff inside the subquery,
// (2) profile, (3) account_id.
const cardQuery = `
SELECT h.type, h.level, h.value, h.date,
       1 + (SELECT COUNT(*)
              FROM %[1]s r
              JOIN account ra ON ra.id = r.account_id
             WHERE r.profile = h.profile
               AND r.type = h.type
               AND ra.staff_mod_level <= 1
               AND (ra.banned_until IS NULL OR ra.banned_until < ?)
               AND (r.value > h.value
                 OR (r.value = h.value
                     AND (r.date < h.date
                       OR (r.date = h.date AND r.account_id < h.account_id)))))
  FROM %[1]s h
 WHERE h.profile = ? AND h.account_id = ?
 ORDER BY h.type ASC`

// PlayerCard returns the player's overall standing plus every per-stat
// row they hold, each with its rank. Two queries — one per table —
// rather than one per stat.
func (s *Store) PlayerCard(ctx context.Context, profile string, accountID int64, now time.Time) (Card, error) {
	card := Card{}

	overall, err := s.entriesFor(ctx, "hiscore_large", profile, accountID, now)
	if err != nil {
		return Card{}, err
	}
	if len(overall) > 0 {
		card.Overall = &overall[0]
	}

	skills, err := s.entriesFor(ctx, "hiscore", profile, accountID, now)
	if err != nil {
		return Card{}, err
	}
	card.Skills = skills
	return card, nil
}

// entriesFor runs cardQuery against one table. table must be a
// compile-time constant.
func (s *Store) entriesFor(ctx context.Context, table, profile string, accountID int64, now time.Time) ([]Entry, error) {
	q := fmt.Sprintf(cardQuery, table)
	rows, err := s.db.QueryContext(ctx, s.db.Rebind(q), now, profile, accountID)
	if err != nil {
		return nil, fmt.Errorf("hiscore: card query %s: %w", table, err)
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.Type, &e.Level, &e.ValueX10, &e.UpdatedAt, &e.Rank); err != nil {
			return nil, fmt.Errorf("hiscore: card scan %s: %w", table, err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("hiscore: card rows %s: %w", table, err)
	}
	return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/hiscore/ -run TestPlayerCard -v`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add modules/hiscore/store.go modules/hiscore/store_test.go
git commit --no-gpg-sign -m "feat(hiscore): player card with visibility-consistent ranks"
```

---

## Task 6: Leaderboard by offset

**Files:**
- Modify: `modules/hiscore/store.go`
- Test: `modules/hiscore/store_test.go` (append)

**Interfaces:**
- Consumes: `Store`, `ErrNotFound`.
- Produces: `Row`, `(*Store).LeaderboardByOffset`.

- [ ] **Step 1: Write the failing test**

Append to `modules/hiscore/store_test.go`:

```go
// seedBoard inserts n visible players on one board, descending in XP so
// that account "p0" is rank 1. Returns usernames in rank order.
func seedBoard(t *testing.T, db *gamedb.DB, profile string, typ, n int) []string {
	t.Helper()
	names := make([]string, 0, n)
	table := TableForType(typ)
	for i := range n {
		name := fmt.Sprintf("p%d", i)
		id := insertAccount(t, db, name, 0, nil)
		insertHiscore(t, db, table, id, profile, typ, 99-i, int64(1_000_000-i*1000), testClock)
		names = append(names, name)
	}
	return names
}

func TestLeaderboardByOffset(t *testing.T) {
	db := createTestDB(t)
	store := NewStore(db)
	names := seedBoard(t, db, "main", 1, 10)

	rows, err := store.LeaderboardByOffset(t.Context(), "main", 1, 3, 4, testClock)
	if err != nil {
		t.Fatalf("LeaderboardByOffset: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("got %d rows, want 4", len(rows))
	}
	for i, r := range rows {
		wantRank := int64(3 + i + 1)
		if r.Rank != wantRank {
			t.Errorf("row %d: rank = %d, want %d", i, r.Rank, wantRank)
		}
		if r.Username != names[3+i] {
			t.Errorf("row %d: username = %q, want %q", i, r.Username, names[3+i])
		}
	}
}

func TestLeaderboardByOffset_ExcludesHidden(t *testing.T) {
	db := createTestDB(t)
	store := NewStore(db)

	future := testClock.Add(24 * time.Hour)
	staff := insertAccount(t, db, "modash", 2, nil)
	banned := insertAccount(t, db, "cheater", 0, &future)
	player := insertAccount(t, db, "zezima", 0, nil)

	insertHiscore(t, db, "hiscore", staff, "main", 1, 99, 30_000_000, testClock)
	insertHiscore(t, db, "hiscore", banned, "main", 1, 99, 25_000_000, testClock)
	insertHiscore(t, db, "hiscore", player, "main", 1, 90, 10_000_000, testClock)

	rows, err := store.LeaderboardByOffset(t.Context(), "main", 1, 0, 10, testClock)
	if err != nil {
		t.Fatalf("LeaderboardByOffset: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1 visible", len(rows))
	}
	if rows[0].Username != "zezima" || rows[0].Rank != 1 {
		t.Errorf("got %q at rank %d, want zezima at rank 1", rows[0].Username, rows[0].Rank)
	}
}

// The card and the leaderboard must agree on rank for the same player,
// including when hidden accounts sit above them.
func TestRankAgreement_CardVsLeaderboard(t *testing.T) {
	db := createTestDB(t)
	store := NewStore(db)

	future := testClock.Add(24 * time.Hour)
	hidden := insertAccount(t, db, "cheater", 0, &future)
	insertHiscore(t, db, "hiscore", hidden, "main", 1, 99, 99_000_000, testClock)
	names := seedBoard(t, db, "main", 1, 5)

	rows, err := store.LeaderboardByOffset(t.Context(), "main", 1, 0, 10, testClock)
	if err != nil {
		t.Fatalf("LeaderboardByOffset: %v", err)
	}
	for _, r := range rows {
		card, err := store.PlayerCard(t.Context(), "main", r.AccountID, testClock)
		if err != nil {
			t.Fatalf("PlayerCard(%d): %v", r.AccountID, err)
		}
		if len(card.Skills) != 1 {
			t.Fatalf("player %s: got %d entries, want 1", r.Username, len(card.Skills))
		}
		if card.Skills[0].Rank != r.Rank {
			t.Errorf("player %s: card rank %d != leaderboard rank %d",
				r.Username, card.Skills[0].Rank, r.Rank)
		}
	}
	if len(rows) != len(names) {
		t.Errorf("got %d rows, want %d", len(rows), len(names))
	}
}

func TestLeaderboardByOffset_Overall(t *testing.T) {
	db := createTestDB(t)
	store := NewStore(db)
	seedBoard(t, db, "main", 0, 3)

	rows, err := store.LeaderboardByOffset(t.Context(), "main", 0, 0, 10, testClock)
	if err != nil {
		t.Fatalf("LeaderboardByOffset(overall): %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows from hiscore_large, want 3", len(rows))
	}
}

func TestLeaderboardByOffset_PastEnd(t *testing.T) {
	db := createTestDB(t)
	store := NewStore(db)
	seedBoard(t, db, "main", 1, 3)

	rows, err := store.LeaderboardByOffset(t.Context(), "main", 1, 100, 10, testClock)
	if err != nil {
		t.Fatalf("LeaderboardByOffset: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("got %d rows past the end, want 0 and no error", len(rows))
	}
}
```

Add `"fmt"` and `"github.com/zsrv/goscape/pkg/gamedb"` to `store_test.go`'s imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/hiscore/ -run 'TestLeaderboardByOffset|TestRankAgreement' -v`

Expected: FAIL — build error, `undefined: (*Store).LeaderboardByOffset`

- [ ] **Step 3: Write the implementation**

Append to `modules/hiscore/store.go`:

```go
// Row is one leaderboard entry.
type Row struct {
	AccountID int64
	Username  string
	Rank      int64
	Level     int
	ValueX10  int64
	UpdatedAt time.Time
}

// boardSelect is the shared projection and ordering for both paging
// modes. %[1]s is the table name (compile-time constant).
//
// The ORDER BY matches idx_%[1]s_rank exactly, so the engine serves it
// as an index range scan rather than a sort.
const boardSelect = `
SELECT h.account_id, a.username, h.level, h.value, h.date
  FROM %[1]s h
  JOIN account a ON a.id = h.account_id
 WHERE h.profile = ? AND h.type = ?
   AND a.staff_mod_level <= 1
   AND (a.banned_until IS NULL OR a.banned_until < ?)`

const boardOrder = `
 ORDER BY h.value DESC, h.date ASC, h.account_id ASC`

// LeaderboardByOffset returns one page starting at a zero-based offset.
// Rank is offset+index+1: the page is already in rank order and the
// visibility filter is applied inside the same query, so the offset
// counts only visible rows.
//
// OFFSET is O(offset) in every SQL engine. This mode exists for random
// access ("jump to page N"); bulk readers use LeaderboardByCursor.
func (s *Store) LeaderboardByOffset(ctx context.Context, profile string, typ, offset, limit int, now time.Time) ([]Row, error) {
	q := fmt.Sprintf(boardSelect, TableForType(typ)) + boardOrder + `
 LIMIT ? OFFSET ?`

	rows, err := s.db.QueryContext(ctx, s.db.Rebind(q), profile, typ, now, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("hiscore: leaderboard offset query: %w", err)
	}
	defer rows.Close()

	return scanBoard(rows, int64(offset)+1)
}

// scanBoard reads board rows, assigning consecutive ranks from firstRank.
func scanBoard(rows *sql.Rows, firstRank int64) ([]Row, error) {
	var out []Row
	rank := firstRank
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.AccountID, &r.Username, &r.Level, &r.ValueX10, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("hiscore: leaderboard scan: %w", err)
		}
		r.Rank = rank
		rank++
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("hiscore: leaderboard rows: %w", err)
	}
	return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/hiscore/ -run 'TestLeaderboardByOffset|TestRankAgreement' -v`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add modules/hiscore/store.go modules/hiscore/store_test.go
git commit --no-gpg-sign -m "feat(hiscore): offset-paged leaderboard queries"
```

---

## Task 7: Cursor paging

**Files:**
- Create: `modules/hiscore/cursor.go`
- Modify: `modules/hiscore/store.go`
- Test: `modules/hiscore/cursor_test.go`
- Test: `modules/hiscore/store_test.go` (append)

**Interfaces:**
- Consumes: `Store`, `boardSelect`, `boardOrder`, `scanBoard`, `Row`.
- Produces: `Cursor`, `ErrBadCursor`, `(Cursor).IsStart`, `(Cursor).Encode`, `DecodeCursor`, `(*Store).LeaderboardByCursor`.

- [ ] **Step 1: Write the failing cursor test**

Create `modules/hiscore/cursor_test.go`:

```go
package hiscore

import (
	"errors"
	"testing"
	"time"
)

func TestCursor_RoundTrip(t *testing.T) {
	want := Cursor{
		ValueX10:  13_034_431_0,
		UpdatedAt: time.Date(2026, 8, 19, 10, 30, 0, 0, time.UTC),
		AccountID: 4242,
		Rank:      101,
	}
	got, err := DecodeCursor(want.Encode())
	if err != nil {
		t.Fatalf("DecodeCursor: %v", err)
	}
	if got.ValueX10 != want.ValueX10 || got.AccountID != want.AccountID || got.Rank != want.Rank {
		t.Errorf("round trip: got %+v, want %+v", got, want)
	}
	if !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Errorf("UpdatedAt: got %v, want %v", got.UpdatedAt, want.UpdatedAt)
	}
}

func TestDecodeCursor_Rejects(t *testing.T) {
	tests := []string{
		"",                      // empty
		"!!!not-base64!!!",      // bad encoding
		"YWJjZGVm",              // valid base64, not a cursor
	}
	for _, in := range tests {
		if _, err := DecodeCursor(in); !errors.Is(err, ErrBadCursor) {
			t.Errorf("DecodeCursor(%q): err = %v, want ErrBadCursor", in, err)
		}
	}
}

func TestCursor_IsStart(t *testing.T) {
	if !(Cursor{}).IsStart() {
		t.Error("zero Cursor must report IsStart")
	}
	if (Cursor{Rank: 1}).IsStart() {
		t.Error("Cursor with a real rank must not report IsStart")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/hiscore/ -run TestCursor -v`

Expected: FAIL — build error, `undefined: Cursor`

- [ ] **Step 3: Write the cursor implementation**

Create `modules/hiscore/cursor.go`:

```go
package hiscore

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrBadCursor marks a cursor that is absent, malformed, or not
// self-consistent. It always maps to 400, never to a panic or a 500.
var ErrBadCursor = errors.New("hiscore: malformed cursor")

// Cursor is a keyset position on a board: the sort key of the last row
// returned, plus the rank the next row will carry.
//
// Cursors are deliberately unsigned. No privilege attaches to one, and
// the query clamps to a single (profile, type) board, so a forged cursor
// can only produce a wrong page for whoever sent it.
type Cursor struct {
	ValueX10  int64     `json:"v"`
	UpdatedAt time.Time `json:"d"`
	AccountID int64     `json:"a"`
	// Rank is the rank of the NEXT row to return. Because the ordering
	// is total, carrying it forward yields true absolute ranks rather
	// than position-within-page.
	Rank int64 `json:"r"`
}

// IsStart reports whether this is the implicit start-of-board position.
// Ranks are 1-based, so a zero Rank can only mean "no cursor supplied".
func (c Cursor) IsStart() bool { return c.Rank == 0 }

// Encode renders the cursor as an opaque base64url token.
func (c Cursor) Encode() string {
	b, err := json.Marshal(c)
	if err != nil {
		// Cursor holds only fixed scalar types; Marshal cannot fail.
		panic(fmt.Sprintf("hiscore: encoding cursor: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeCursor parses a token produced by Encode.
func DecodeCursor(s string) (Cursor, error) {
	if s == "" {
		return Cursor{}, ErrBadCursor
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, ErrBadCursor
	}
	var c Cursor
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return Cursor{}, ErrBadCursor
	}
	// A decoded cursor must name a real position: ranks are 1-based, so
	// rank 0 here means the token was not produced by Encode.
	if c.Rank < 1 || c.AccountID < 1 {
		return Cursor{}, ErrBadCursor
	}
	return c, nil
}
```

- [ ] **Step 4: Run cursor test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/hiscore/ -run TestCursor -v`

Expected: PASS

- [ ] **Step 5: Write the failing store test**

Append to `modules/hiscore/store_test.go`:

```go
// TestLeaderboard_OffsetCursorEquivalence is the test that keeps two
// paging modes honest: walking the same board both ways must yield
// identical rows in identical order with identical absolute ranks.
func TestLeaderboard_OffsetCursorEquivalence(t *testing.T) {
	db := createTestDB(t)
	store := NewStore(db)
	seedBoard(t, db, "main", 1, 17)

	const page = 5

	var viaOffset []Row
	for off := 0; ; off += page {
		rows, err := store.LeaderboardByOffset(t.Context(), "main", 1, off, page, testClock)
		if err != nil {
			t.Fatalf("LeaderboardByOffset(%d): %v", off, err)
		}
		if len(rows) == 0 {
			break
		}
		viaOffset = append(viaOffset, rows...)
	}

	var viaCursor []Row
	cur := Cursor{}
	for {
		rows, err := store.LeaderboardByCursor(t.Context(), "main", 1, cur, page, testClock)
		if err != nil {
			t.Fatalf("LeaderboardByCursor(%+v): %v", cur, err)
		}
		if len(rows) == 0 {
			break
		}
		viaCursor = append(viaCursor, rows...)
		last := rows[len(rows)-1]
		cur = Cursor{
			ValueX10:  last.ValueX10,
			UpdatedAt: last.UpdatedAt,
			AccountID: last.AccountID,
			Rank:      last.Rank + 1,
		}
	}

	if len(viaOffset) != len(viaCursor) {
		t.Fatalf("offset walk got %d rows, cursor walk got %d", len(viaOffset), len(viaCursor))
	}
	for i := range viaOffset {
		o, c := viaOffset[i], viaCursor[i]
		if o.AccountID != c.AccountID || o.Rank != c.Rank || o.ValueX10 != c.ValueX10 {
			t.Errorf("row %d diverges: offset %+v vs cursor %+v", i, o, c)
		}
	}
	if len(viaOffset) != 17 {
		t.Errorf("walked %d rows, want 17", len(viaOffset))
	}
}

// A cursor sitting on a tie must resume after the exact row it names,
// not before it and not after the whole tie group.
func TestLeaderboardByCursor_TieBoundary(t *testing.T) {
	db := createTestDB(t)
	store := NewStore(db)

	a := insertAccount(t, db, "aaa", 0, nil)
	b := insertAccount(t, db, "bbb", 0, nil)
	c := insertAccount(t, db, "ccc", 0, nil)
	for _, id := range []int64{a, b, c} {
		insertHiscore(t, db, "hiscore", id, "main", 1, 90, 10_000_000, testClock)
	}

	first, err := store.LeaderboardByCursor(t.Context(), "main", 1, Cursor{}, 1, testClock)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first) != 1 || first[0].AccountID != a {
		t.Fatalf("first page = %+v, want the lowest account_id of the tie group", first)
	}

	next, err := store.LeaderboardByCursor(t.Context(), "main", 1, Cursor{
		ValueX10:  first[0].ValueX10,
		UpdatedAt: first[0].UpdatedAt,
		AccountID: first[0].AccountID,
		Rank:      2,
	}, 10, testClock)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(next) != 2 {
		t.Fatalf("got %d rows after the tie boundary, want 2", len(next))
	}
	if next[0].AccountID != b || next[1].AccountID != c {
		t.Errorf("got accounts %d,%d, want %d,%d", next[0].AccountID, next[1].AccountID, b, c)
	}
	if next[0].Rank != 2 || next[1].Rank != 3 {
		t.Errorf("got ranks %d,%d, want 2,3", next[0].Rank, next[1].Rank)
	}
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/hiscore/ -run 'TestLeaderboard_OffsetCursor|TestLeaderboardByCursor' -v`

Expected: FAIL — build error, `undefined: (*Store).LeaderboardByCursor`

- [ ] **Step 7: Write the implementation**

Append to `modules/hiscore/store.go`:

```go
// LeaderboardByCursor returns one page starting at a keyset position.
// Unlike OFFSET, the seek is O(limit) at any depth, so a full-board walk
// is linear rather than quadratic. Ranks come from the cursor, which is
// exact because the ordering is total.
//
// The keyset predicate mirrors the ORDER BY term for term: strictly
// worse value, or equal value and strictly later date, or all three
// equal and a strictly greater account_id.
func (s *Store) LeaderboardByCursor(ctx context.Context, profile string, typ int, cur Cursor, limit int, now time.Time) ([]Row, error) {
	q := fmt.Sprintf(boardSelect, TableForType(typ))
	args := []any{profile, typ, now}

	firstRank := int64(1)
	if !cur.IsStart() {
		q += `
   AND (h.value < ?
     OR (h.value = ? AND (h.date > ?
       OR (h.date = ? AND h.account_id > ?))))`
		args = append(args, cur.ValueX10, cur.ValueX10, cur.UpdatedAt, cur.UpdatedAt, cur.AccountID)
		firstRank = cur.Rank
	}
	q += boardOrder + `
 LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, s.db.Rebind(q), args...)
	if err != nil {
		return nil, fmt.Errorf("hiscore: leaderboard cursor query: %w", err)
	}
	defer rows.Close()

	return scanBoard(rows, firstRank)
}
```

- [ ] **Step 8: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/hiscore/ -v`

Expected: PASS (all tests so far)

- [ ] **Step 9: Commit**

```bash
git add modules/hiscore/cursor.go modules/hiscore/cursor_test.go \
        modules/hiscore/store.go modules/hiscore/store_test.go
git commit --no-gpg-sign -m "feat(hiscore): keyset cursor paging with absolute ranks"
```

---

## Task 8: Caller identity and backstop limiter

**Files:**
- Create: `modules/hiscore/gateway.go`
- Test: `modules/hiscore/gateway_test.go`

**Interfaces:**
- Consumes: `Config`.
- Produces: `caller`, `backstop`, `newBackstop`, `(*backstop).allow`, `consumerFromHeaders`.

`(*api).identify` is added in Task 9, once `api` exists; this task builds the pieces it uses.

- [ ] **Step 1: Write the failing test**

Create `modules/hiscore/gateway_test.go`:

```go
package hiscore

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestConsumerFromHeaders_Untrusted(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	r.Header.Set("X-Consumer-Username", "partner-a")
	r.Header.Set("X-Anonymous-Consumer", "false")

	got := consumerFromHeaders(r, false)
	if got.Consumer != "" {
		t.Errorf("Consumer = %q, want empty — headers must be ignored when untrusted", got.Consumer)
	}
	if !got.Anonymous {
		t.Error("Anonymous = false, want true when headers are not trusted")
	}
}

func TestConsumerFromHeaders_Trusted(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	r.Header.Set("X-Consumer-Username", "partner-a")
	r.Header.Set("X-Anonymous-Consumer", "false")

	got := consumerFromHeaders(r, true)
	if got.Consumer != "partner-a" {
		t.Errorf("Consumer = %q, want partner-a", got.Consumer)
	}
	if got.Anonymous {
		t.Error("Anonymous = true, want false")
	}
}

func TestConsumerFromHeaders_TrustedAnonymous(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	r.Header.Set("X-Consumer-Username", "anonymous-consumer")
	r.Header.Set("X-Anonymous-Consumer", "true")

	got := consumerFromHeaders(r, true)
	if !got.Anonymous {
		t.Error("Anonymous = false, want true — X-Anonymous-Consumer: true is authoritative")
	}
}

func TestBackstop_AllowsUnderLimitThenBlocks(t *testing.T) {
	b := newBackstop(3)
	now := testClock

	for i := range 3 {
		if !b.allow("k", now) {
			t.Fatalf("request %d: blocked under the limit", i+1)
		}
	}
	if b.allow("k", now) {
		t.Error("request 4: allowed above the limit of 3")
	}
}

func TestBackstop_WindowRolls(t *testing.T) {
	b := newBackstop(1)
	if !b.allow("k", testClock) {
		t.Fatal("first request blocked")
	}
	if b.allow("k", testClock.Add(30*time.Second)) {
		t.Error("second request inside the window was allowed")
	}
	if !b.allow("k", testClock.Add(61*time.Second)) {
		t.Error("request after the window rolled was blocked")
	}
}

func TestBackstop_KeysAreIndependent(t *testing.T) {
	b := newBackstop(1)
	if !b.allow("a", testClock) || !b.allow("b", testClock) {
		t.Error("distinct keys must not share a budget")
	}
}

func TestBackstop_ZeroDisables(t *testing.T) {
	b := newBackstop(0)
	for i := range 1000 {
		if !b.allow("k", testClock) {
			t.Fatalf("request %d blocked, but rate 0 disables the limiter", i)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/hiscore/ -run 'TestConsumerFrom|TestBackstop' -v`

Expected: FAIL — build error, `undefined: consumerFromHeaders`

- [ ] **Step 3: Write the implementation**

Create `modules/hiscore/gateway.go`:

```go
package hiscore

import (
	"net/http"
	"sync"
	"time"
)

// caller is who we believe sent a request. It is used for log lines and
// for keying the backstop limiter — never for authorization. Nothing in
// this module grants anything on the strength of these fields, which is
// what makes trusting the gateway optional.
type caller struct {
	Consumer  string // gateway consumer name; "" when unknown
	Anonymous bool
	IP        string
}

// Kong sets these on the upstream request after key-auth runs.
const (
	hdrConsumerUsername = "X-Consumer-Username"
	hdrAnonymousConsumer = "X-Anonymous-Consumer"
)

// consumerFromHeaders reads the gateway's identity headers. When trust
// is false the headers are ignored entirely and every caller is
// anonymous — the safe default, since an unverified header should not
// reach a log line and be mistaken for an identity.
func consumerFromHeaders(r *http.Request, trust bool) caller {
	if !trust {
		return caller{Anonymous: true}
	}
	if r.Header.Get(hdrAnonymousConsumer) == "true" {
		return caller{Anonymous: true}
	}
	name := r.Header.Get(hdrConsumerUsername)
	return caller{Consumer: name, Anonymous: name == ""}
}

// backstop is a coarse fixed-window limiter for the case where the
// module is reached without a gateway in front of it. It is not the
// quota system — Kong's per-consumer rate-limiting is — so a fixed
// window (rather than a token bucket) is deliberate: it is cheap,
// allocation-free per request, and precise enough for a safety net.
type backstop struct {
	perMinute int

	mu      sync.Mutex
	windows map[string]*window
}

type window struct {
	start time.Time
	count int
}

func newBackstop(perMinute int) *backstop {
	return &backstop{perMinute: perMinute, windows: make(map[string]*window)}
}

// allow reports whether this request fits in the caller's current
// window. A perMinute of 0 disables the limiter entirely.
func (b *backstop) allow(key string, now time.Time) bool {
	if b.perMinute <= 0 {
		return true
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Evict windows that rolled over, so a long-lived process does not
	// accumulate an entry per distinct caller forever.
	for k, w := range b.windows {
		if now.Sub(w.start) >= time.Minute {
			delete(b.windows, k)
		}
	}

	w, ok := b.windows[key]
	if !ok {
		b.windows[key] = &window{start: now, count: 1}
		return true
	}
	if w.count >= b.perMinute {
		return false
	}
	w.count++
	return true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/hiscore/ -run 'TestConsumerFrom|TestBackstop' -v`

Expected: PASS

- [ ] **Step 5: Run with the race detector**

Run: `CGO_ENABLED=1 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/hiscore/ -run TestBackstop`

Expected: PASS, no race reports.

- [ ] **Step 6: Commit**

```bash
git add modules/hiscore/gateway.go modules/hiscore/gateway_test.go
git commit --no-gpg-sign -m "feat(hiscore): untrusted-by-default gateway identity and backstop limiter"
```

---

## Task 9: HTTP scaffolding and `/v1/skills`

**Files:**
- Create: `modules/hiscore/handlers.go`
- Test: `modules/hiscore/handlers_test.go`

**Interfaces:**
- Consumes: `Config`, `Store`, `caller`, `backstop`, `Skills()`.
- Produces: `api`, `newAPI`, `(*api).register`, `(*api).identify`, `writeJSON`, `writeError`, error-code constants.

- [ ] **Step 1: Write the failing test**

Create `modules/hiscore/handlers_test.go`:

```go
package hiscore

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/gamedb"
)

// newTestAPI builds an api over a fresh DB with a frozen clock.
func newTestAPI(t *testing.T) (*api, *gamedb.DB) {
	t.Helper()
	db := createTestDB(t)
	cfg := defaultConfig(t)
	cfg.Enable = true

	a, err := newAPI(cfg, NewStore(db), noopLogger())
	if err != nil {
		t.Fatalf("newAPI: %v", err)
	}
	a.now = func() time.Time { return testClock }
	return a, db
}

func doGET(t *testing.T, a *api, target string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	a.register(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestSkillsEndpoint(t *testing.T) {
	a, _ := newTestAPI(t)
	rec := doGET(t, a, "/v1/skills")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if rec.Header().Get("ETag") == "" {
		t.Error("ETag missing")
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "max-age=60") {
		t.Errorf("Cache-Control = %q, want max-age=60", cc)
	}

	var body struct {
		Skills []Skill `json:"skills"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Skills) != 19 {
		t.Fatalf("got %d skills, want 19", len(body.Skills))
	}
	if body.Skills[0].Name != "attack" || body.Skills[0].Type != 1 {
		t.Errorf("first skill = %+v, want {Type:1 Name:attack}", body.Skills[0])
	}
}

func TestConditionalRequest_304(t *testing.T) {
	a, _ := newTestAPI(t)
	first := doGET(t, a, "/v1/skills")
	etag := first.Header().Get("ETag")

	mux := http.NewServeMux()
	a.register(mux)
	req := httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	req.Header.Set("If-None-Match", etag)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want 304", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("304 carried a %d-byte body, want empty", rec.Body.Len())
	}
}

func TestETagIsStableAcrossIdenticalRequests(t *testing.T) {
	a, _ := newTestAPI(t)
	first := doGET(t, a, "/v1/skills").Header().Get("ETag")
	second := doGET(t, a, "/v1/skills").Header().Get("ETag")
	if first != second {
		t.Errorf("ETag changed between identical requests: %q vs %q", first, second)
	}
}

func TestUnknownRoute_404JSON(t *testing.T) {
	a, _ := newTestAPI(t)
	rec := doGET(t, a, "/v1/nonsense")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	var body errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v; body %s", err, rec.Body.String())
	}
	if body.Error.Code != codeNotFound {
		t.Errorf("code = %q, want %q", body.Error.Code, codeNotFound)
	}
}

func TestBackstopLimiter_429(t *testing.T) {
	db := createTestDB(t)
	cfg := defaultConfig(t)
	cfg.BackstopRate = 1
	a, err := newAPI(cfg, NewStore(db), noopLogger())
	if err != nil {
		t.Fatalf("newAPI: %v", err)
	}
	a.now = func() time.Time { return testClock }

	if rec := doGET(t, a, "/v1/skills"); rec.Code != http.StatusOK {
		t.Fatalf("first request: status %d, want 200", rec.Code)
	}
	rec := doGET(t, a, "/v1/skills")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: status %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("429 missing Retry-After")
	}
}
```

Add `"time"` to `handlers_test.go`'s imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/hiscore/ -run 'TestSkillsEndpoint|TestConditional|TestETag|TestUnknownRoute|TestBackstopLimiter' -v`

Expected: FAIL — build error, `undefined: newAPI`

- [ ] **Step 3: Write the implementation**

Create `modules/hiscore/handlers.go`:

```go
package hiscore

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/zsrv/goscape/pkg/dskit/middleware"
)

// Error codes. These are part of the public contract.
const (
	codeNotFound       = "not_found"
	codeInvalidRequest = "invalid_request"
	codeRateLimited    = "rate_limited"
	codeInternal       = "internal"
)

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// api holds the HTTP surface. now is injectable so tests can freeze the
// clock that drives the visibility filter and the limiter.
type api struct {
	cfg       Config
	store     *Store
	sourceIPs *middleware.SourceIPExtractor
	limiter   *backstop
	now       func() time.Time
	log       *slog.Logger
}

func newAPI(cfg Config, store *Store, log *slog.Logger) (*api, error) {
	// Always construct the extractor: with header and regex both blank
	// it uses dskit's built-in Forwarded / X-Real-IP / X-Forwarded-For
	// chain, which is the default and is what a gateway populates.
	// NewSourceIPs only errors when exactly one of the pair is set.
	sourceIPs, err := middleware.NewSourceIPs(
		cfg.Server.LogSourceIPsHeader, cfg.Server.LogSourceIPsRegex, cfg.Server.LogSourceIPsFull)
	if err != nil {
		return nil, fmt.Errorf("hiscore: source IP extractor: %w", err)
	}
	return &api{
		cfg:       cfg,
		store:     store,
		sourceIPs: sourceIPs,
		limiter:   newBackstop(cfg.BackstopRate),
		now:       time.Now,
		log:       log,
	}, nil
}

// register wires the routes. Every route goes through guard, which
// applies the backstop limiter.
func (a *api) register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/skills", a.guard(a.handleSkills))
	// A catch-all so unmatched paths get the JSON error shape rather
	// than net/http's text 404.
	mux.HandleFunc("/", a.guard(func(w http.ResponseWriter, r *http.Request) {
		a.writeError(w, http.StatusNotFound, codeNotFound, "no such resource")
	}))
}

// identify resolves the caller for logging and limiter keying.
func (a *api) identify(r *http.Request) caller {
	c := consumerFromHeaders(r, a.cfg.TrustGatewayHeaders)
	c.IP = a.clientIP(r)
	return c
}

// clientIP prefers the configured proxy header (dskit's extractor,
// which is how the real client IP is recovered from behind a gateway)
// and falls back to the socket peer.
func (a *api) clientIP(r *http.Request) string {
	if a.sourceIPs != nil {
		if ip := a.sourceIPs.Get(r); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// limiterKey buckets by gateway consumer when we have one, else by IP.
func (c caller) limiterKey() string {
	if c.Consumer != "" {
		return "consumer:" + c.Consumer
	}
	return "ip:" + c.IP
}

// guard applies the backstop limiter to a handler.
func (a *api) guard(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := a.identify(r)
		if !a.limiter.allow(c.limiterKey(), a.now()) {
			w.Header().Set("Retry-After", "60")
			a.writeError(w, http.StatusTooManyRequests, codeRateLimited,
				"too many requests; retry after 60s")
			return
		}
		h(w, r)
	}
}

func (a *api) handleSkills(w http.ResponseWriter, r *http.Request) {
	a.writeJSON(w, r, struct {
		Skills []Skill `json:"skills"`
	}{Skills: Skills()}, time.Time{})
}

// writeJSON encodes v, sets caching headers, and honours If-None-Match.
// lastMod is written as Last-Modified when non-zero; build-static
// responses pass the zero time and carry an ETag only.
func (a *api) writeJSON(w http.ResponseWriter, r *http.Request, v any, lastMod time.Time) {
	body, err := json.Marshal(v)
	if err != nil {
		a.log.Error("hiscore: encoding response", slog.Any("err", err))
		a.writeError(w, http.StatusInternalServerError, codeInternal, "internal error")
		return
	}

	sum := sha256.Sum256(body)
	etag := `"` + base64.RawURLEncoding.EncodeToString(sum[:16]) + `"`

	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("ETag", etag)
	h.Set("Cache-Control", "public, max-age="+strconv.Itoa(int(a.cfg.CacheMaxAge.Seconds())))
	if !lastMod.IsZero() {
		h.Set("Last-Modified", lastMod.UTC().Format(http.TimeFormat))
	}

	if match := r.Header.Get("If-None-Match"); match != "" && match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(body); err != nil {
		a.log.Debug("hiscore: writing response", slog.Any("err", err))
	}
}

func (a *api) writeError(w http.ResponseWriter, status int, code, msg string) {
	var env errorEnvelope
	env.Error.Code = code
	env.Error.Message = msg

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// Errors are per-caller and must not be cached at the edge.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(env); err != nil {
		a.log.Debug("hiscore: writing error response", slog.Any("err", err))
	}
}
```

`(SourceIPExtractor).Get(*http.Request) string` is the confirmed extraction method (`pkg/dskit/middleware/source_ips.go:80`, value receiver).

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/hiscore/ -run 'TestSkillsEndpoint|TestConditional|TestETag|TestUnknownRoute|TestBackstopLimiter' -v`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add modules/hiscore/handlers.go modules/hiscore/handlers_test.go
git commit --no-gpg-sign -m "feat(hiscore): HTTP scaffolding, caching headers and /v1/skills"
```

---

## Task 10: `/v1/players/{name}`

**Files:**
- Modify: `modules/hiscore/handlers.go`
- Test: `modules/hiscore/handlers_test.go` (append)

**Interfaces:**
- Consumes: `api`, `Store.LookupAccountByName`, `Store.PlayerCard`, `Skills()`, `jstring.ToSafeName`, `jstring.ToDisplayName`.
- Produces: `(*api).handlePlayer`, `playerResponse`, `skillEntry`.

- [ ] **Step 1: Write the failing test**

Append to `modules/hiscore/handlers_test.go`:

```go
func TestPlayerEndpoint(t *testing.T) {
	a, db := newTestAPI(t)

	acct := insertAccount(t, db, "zezima", 0, nil)
	// 13,034,431 whole XP is stored as 130,344,310 tenths.
	insertHiscore(t, db, "hiscore_large", acct, "main", 0, 1893, 130_344_310, testClock)
	insertHiscore(t, db, "hiscore", acct, "main", 1, 99, 130_344_310, testClock)

	rec := doGET(t, a, "/v1/players/Zezima")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}

	var body playerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body.Name != "Zezima" {
		t.Errorf("name = %q, want display form Zezima", body.Name)
	}
	if body.Profile != "main" {
		t.Errorf("profile = %q, want main", body.Profile)
	}
	if body.Overall == nil {
		t.Fatal("overall = nil, want the aggregate entry")
	}
	if *body.Overall.XP != 13_034_431 {
		t.Errorf("overall xp = %d, want 13034431 (whole XP, x10 divided out)", *body.Overall.XP)
	}
	if len(body.Skills) != 19 {
		t.Fatalf("got %d skill entries, want all 19 enabled stats", len(body.Skills))
	}

	var attack, defence *skillEntry
	for i := range body.Skills {
		switch body.Skills[i].Name {
		case "attack":
			attack = &body.Skills[i]
		case "defence":
			defence = &body.Skills[i]
		}
	}
	if attack == nil || defence == nil {
		t.Fatal("attack and defence entries must both be present")
	}
	if !attack.Ranked {
		t.Error("attack: ranked = false, want true")
	}
	if attack.XP == nil || *attack.XP != 13_034_431 {
		t.Errorf("attack xp = %v, want 13034431", attack.XP)
	}
	if defence.Ranked {
		t.Error("defence: ranked = true, want false — no row below level 15")
	}
	if defence.XP != nil || defence.Rank != nil || defence.Level != nil {
		t.Errorf("defence: got xp=%v rank=%v level=%v, want all null",
			defence.XP, defence.Rank, defence.Level)
	}
}

// Name normalization: base37 safe-name round trip means these all
// address the same account.
func TestPlayerEndpoint_NameNormalization(t *testing.T) {
	a, db := newTestAPI(t)
	acct := insertAccount(t, db, "ze_zima", 0, nil)
	insertHiscore(t, db, "hiscore_large", acct, "main", 0, 100, 1_000_000, testClock)

	for _, name := range []string{"ze_zima", "Ze_Zima", "Ze%20zima", "ZE ZIMA"} {
		rec := doGET(t, a, "/v1/players/"+name)
		if rec.Code != http.StatusOK {
			t.Errorf("GET /v1/players/%s: status %d, want 200", name, rec.Code)
		}
	}
}

func TestPlayerEndpoint_NotFound(t *testing.T) {
	a, db := newTestAPI(t)
	future := testClock.Add(24 * time.Hour)
	insertAccount(t, db, "cheater", 0, &future)
	insertAccount(t, db, "modash", 2, nil)

	// Unknown, banned, and staff must all be indistinguishable 404s.
	for _, name := range []string{"nobody", "cheater", "modash"} {
		rec := doGET(t, a, "/v1/players/"+name)
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET /v1/players/%s: status %d, want 404", name, rec.Code)
		}
	}
}

func TestPlayerEndpoint_NeverExportedIs404(t *testing.T) {
	a, db := newTestAPI(t)
	insertAccount(t, db, "freshman", 0, nil)

	rec := doGET(t, a, "/v1/players/freshman")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for an account with no exported rows", rec.Code)
	}
}

func TestPlayerEndpoint_LastModified(t *testing.T) {
	a, db := newTestAPI(t)
	acct := insertAccount(t, db, "zezima", 0, nil)
	newest := testClock.Add(-2 * time.Hour)
	insertHiscore(t, db, "hiscore_large", acct, "main", 0, 100, 1_000_000, testClock.Add(-5*time.Hour))
	insertHiscore(t, db, "hiscore", acct, "main", 1, 99, 900_000, newest)

	rec := doGET(t, a, "/v1/players/zezima")
	got := rec.Header().Get("Last-Modified")
	if want := newest.UTC().Format(http.TimeFormat); got != want {
		t.Errorf("Last-Modified = %q, want %q (newest row in the response)", got, want)
	}
}

// A dead database must produce an opaque 500, not a panic and not a
// leak of SQL or internal identifiers.
func TestPlayerEndpoint_DatabaseFailure(t *testing.T) {
	a, db := newTestAPI(t)
	insertAccount(t, db, "zezima", 0, nil)

	if err := db.Close(); err != nil {
		t.Fatalf("closing db: %v", err)
	}

	rec := doGET(t, a, "/v1/players/zezima")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}

	var body errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v; body %s", err, rec.Body.String())
	}
	if body.Error.Code != codeInternal {
		t.Errorf("code = %q, want %q", body.Error.Code, codeInternal)
	}
	for _, leak := range []string{"SELECT", "hiscore", "account_id", "sql"} {
		if strings.Contains(body.Error.Message, leak) {
			t.Errorf("error message %q leaks internals (%q)", body.Error.Message, leak)
		}
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store — errors must not be cached at the edge", cc)
	}
}
```

Note: `createTestDB` also registers a `db.Close()` cleanup; closing twice is safe (`sql.DB.Close` is idempotent).

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/hiscore/ -run TestPlayerEndpoint -v`

Expected: FAIL — build error, `undefined: playerResponse`

- [ ] **Step 3: Write the implementation**

Append to `modules/hiscore/handlers.go`:

```go
// skillEntry is one row of a player card. Pointer fields are null in
// JSON when the player has no row for that stat — the write path
// exports a stat only at base level >= 15, so a card is deliberately
// sparse. Every enabled stat is still listed, so a consumer can render
// a fixed table without special-casing absence.
type skillEntry struct {
	Type      int        `json:"type"`
	Name      string     `json:"name"`
	Ranked    bool       `json:"ranked"`
	Rank      *int64     `json:"rank"`
	Level     *int       `json:"level"`
	XP        *int64     `json:"xp"`
	UpdatedAt *time.Time `json:"updated_at"`
}

type playerResponse struct {
	Name    string       `json:"name"`
	Profile string       `json:"profile"`
	Overall *skillEntry  `json:"overall"`
	Skills  []skillEntry `json:"skills"`
}

// wholeXP converts the stored fixed-point tenths to whole XP. This is
// the ONLY place the x10 representation is divided out.
func wholeXP(valueX10 int64) int64 { return valueX10 / 10 }

func (a *api) handlePlayer(w http.ResponseWriter, r *http.Request) {
	profile := a.profileParam(r)
	safeName := jstring.ToSafeName(r.PathValue("name"))
	if safeName == "" {
		a.writeError(w, http.StatusBadRequest, codeInvalidRequest, "player name is required")
		return
	}

	now := a.now()
	acct, err := a.store.LookupAccountByName(r.Context(), safeName, now)
	if errors.Is(err, ErrNotFound) {
		a.writeError(w, http.StatusNotFound, codeNotFound, "player not found")
		return
	}
	if err != nil {
		a.internal(w, "lookup account", err)
		return
	}

	card, err := a.store.PlayerCard(r.Context(), profile, acct.ID, now)
	if err != nil {
		a.internal(w, "player card", err)
		return
	}
	// A visible account that has never been exported is reported the
	// same as an unknown one: there is no standing to show.
	if card.Overall == nil && len(card.Skills) == 0 {
		a.writeError(w, http.StatusNotFound, codeNotFound, "player not found")
		return
	}

	byType := make(map[int]Entry, len(card.Skills))
	var newest time.Time
	for _, e := range card.Skills {
		byType[e.Type] = e
		if e.UpdatedAt.After(newest) {
			newest = e.UpdatedAt
		}
	}

	resp := playerResponse{
		Name:    jstring.ToDisplayName(acct.Username),
		Profile: profile,
		Skills:  make([]skillEntry, 0, len(Skills())),
	}
	if card.Overall != nil {
		e := entryToJSON(0, SkillOverall, *card.Overall)
		resp.Overall = &e
		if card.Overall.UpdatedAt.After(newest) {
			newest = card.Overall.UpdatedAt
		}
	}
	for _, s := range Skills() {
		if e, ok := byType[s.Type]; ok {
			resp.Skills = append(resp.Skills, entryToJSON(s.Type, s.Name, e))
			continue
		}
		resp.Skills = append(resp.Skills, skillEntry{Type: s.Type, Name: s.Name, Ranked: false})
	}

	a.writeJSON(w, r, resp, newest)
}

func entryToJSON(typ int, name string, e Entry) skillEntry {
	rank, level, xp, at := e.Rank, e.Level, wholeXP(e.ValueX10), e.UpdatedAt
	return skillEntry{
		Type: typ, Name: name, Ranked: true,
		Rank: &rank, Level: &level, XP: &xp, UpdatedAt: &at,
	}
}

// profileParam returns the requested profile, defaulting to config.
func (a *api) profileParam(r *http.Request) string {
	if p := r.URL.Query().Get("profile"); p != "" {
		return p
	}
	return a.cfg.Profile
}

// internal logs the real cause and returns an opaque 500. Callers never
// see SQL, table names, or account ids.
func (a *api) internal(w http.ResponseWriter, what string, err error) {
	a.log.Error("hiscore: "+what, slog.Any("err", err))
	a.writeError(w, http.StatusInternalServerError, codeInternal, "internal error")
}
```

Add to `handlers.go` imports: `"errors"` and `"github.com/zsrv/goscape/pkg/util/jstring"`.

Register the route in `(*api).register`, **before** the `/` catch-all:

```go
	mux.HandleFunc("GET /v1/players/{name}", a.guard(a.handlePlayer))
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/hiscore/ -run TestPlayerEndpoint -v`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add modules/hiscore/handlers.go modules/hiscore/handlers_test.go
git commit --no-gpg-sign -m "feat(hiscore): /v1/players/{name} card endpoint"
```

---

## Task 11: `/v1/leaderboards/{skill}`

**Files:**
- Modify: `modules/hiscore/handlers.go`
- Test: `modules/hiscore/handlers_test.go` (append)

**Interfaces:**
- Consumes: `api`, `Store.LeaderboardByOffset`, `Store.LeaderboardByCursor`, `SkillByName`, `Cursor`, `DecodeCursor`.
- Produces: `(*api).handleLeaderboard`, `leaderboardResponse`, `boardEntry`.

- [ ] **Step 1: Write the failing test**

Append to `modules/hiscore/handlers_test.go`:

```go
func TestLeaderboardEndpoint(t *testing.T) {
	a, db := newTestAPI(t)
	seedBoard(t, db, "main", 1, 10)

	rec := doGET(t, a, "/v1/leaderboards/attack?limit=3&offset=2")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}

	var body leaderboardResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Skill != "attack" {
		t.Errorf("skill = %q, want attack", body.Skill)
	}
	if len(body.Entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(body.Entries))
	}
	if body.Entries[0].Rank != 3 {
		t.Errorf("first rank = %d, want 3 (offset 2)", body.Entries[0].Rank)
	}
	if body.NextCursor == "" {
		t.Error("next_cursor empty, want a token — more rows remain")
	}
}

func TestLeaderboardEndpoint_Overall(t *testing.T) {
	a, db := newTestAPI(t)
	seedBoard(t, db, "main", 0, 3)

	rec := doGET(t, a, "/v1/leaderboards/overall")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	var body leaderboardResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Entries) != 3 {
		t.Errorf("got %d entries from hiscore_large, want 3", len(body.Entries))
	}
}

func TestLeaderboardEndpoint_XPIsWhole(t *testing.T) {
	a, db := newTestAPI(t)
	acct := insertAccount(t, db, "zezima", 0, nil)
	insertHiscore(t, db, "hiscore", acct, "main", 1, 99, 130_344_310, testClock)

	rec := doGET(t, a, "/v1/leaderboards/attack")
	var body leaderboardResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Entries[0].XP != 13_034_431 {
		t.Errorf("xp = %d, want 13034431 (whole XP)", body.Entries[0].XP)
	}
}

func TestLeaderboardEndpoint_NextCursorEmptyAtEnd(t *testing.T) {
	a, db := newTestAPI(t)
	seedBoard(t, db, "main", 1, 3)

	rec := doGET(t, a, "/v1/leaderboards/attack?limit=10")
	var body leaderboardResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.NextCursor != "" {
		t.Errorf("next_cursor = %q, want empty at end of board", body.NextCursor)
	}
}

func TestLeaderboardEndpoint_CursorWalkMatchesOffsetWalk(t *testing.T) {
	a, db := newTestAPI(t)
	seedBoard(t, db, "main", 1, 12)

	var viaCursor []boardEntry
	target := "/v1/leaderboards/attack?limit=5"
	for {
		rec := doGET(t, a, target)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: status %d", target, rec.Code)
		}
		var body leaderboardResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		viaCursor = append(viaCursor, body.Entries...)
		if body.NextCursor == "" {
			break
		}
		target = "/v1/leaderboards/attack?limit=5&cursor=" + url.QueryEscape(body.NextCursor)
	}

	if len(viaCursor) != 12 {
		t.Fatalf("cursor walk returned %d entries, want 12", len(viaCursor))
	}
	for i, e := range viaCursor {
		if e.Rank != int64(i+1) {
			t.Errorf("entry %d: rank = %d, want %d", i, e.Rank, i+1)
		}
	}
}

func TestLeaderboardEndpoint_BadRequests(t *testing.T) {
	a, _ := newTestAPI(t)

	tests := []struct {
		name   string
		target string
	}{
		{"unknown skill", "/v1/leaderboards/nonsense"},
		{"disabled stat", "/v1/leaderboards/stat18"},
		{"limit above max", "/v1/leaderboards/attack?limit=101"},
		{"zero limit", "/v1/leaderboards/attack?limit=0"},
		{"negative limit", "/v1/leaderboards/attack?limit=-1"},
		{"non-numeric limit", "/v1/leaderboards/attack?limit=abc"},
		{"negative offset", "/v1/leaderboards/attack?offset=-1"},
		{"offset past max rank", "/v1/leaderboards/attack?offset=500000&limit=25"},
		{"offset and cursor together", "/v1/leaderboards/attack?offset=5&cursor=abc"},
		{"malformed cursor", "/v1/leaderboards/attack?cursor=!!!"},
		{"empty profile", "/v1/leaderboards/attack?profile="},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := doGET(t, a, tc.target)
			// "empty profile" falls back to the configured default and is valid.
			if tc.name == "empty profile" {
				if rec.Code != http.StatusOK {
					t.Fatalf("status = %d, want 200 (empty profile falls back to default)", rec.Code)
				}
				return
			}
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body %s", rec.Code, rec.Body.String())
			}
			var body errorEnvelope
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.Error.Code != codeInvalidRequest {
				t.Errorf("code = %q, want %q", body.Error.Code, codeInvalidRequest)
			}
		})
	}
}

// An unknown profile is a valid query that simply has no rows.
func TestLeaderboardEndpoint_UnknownProfileIsEmpty(t *testing.T) {
	a, db := newTestAPI(t)
	seedBoard(t, db, "main", 1, 3)

	rec := doGET(t, a, "/v1/leaderboards/attack?profile=nosuchprofile")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body leaderboardResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Entries) != 0 {
		t.Errorf("got %d entries, want 0", len(body.Entries))
	}
}
```

Add `"net/url"` to `handlers_test.go`'s imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/hiscore/ -run TestLeaderboardEndpoint -v`

Expected: FAIL — build error, `undefined: leaderboardResponse`

- [ ] **Step 3: Write the implementation**

Append to `modules/hiscore/handlers.go`:

```go
type boardEntry struct {
	Rank      int64     `json:"rank"`
	Name      string    `json:"name"`
	Level     int       `json:"level"`
	XP        int64     `json:"xp"`
	UpdatedAt time.Time `json:"updated_at"`
}

type leaderboardResponse struct {
	Skill   string       `json:"skill"`
	Profile string       `json:"profile"`
	Entries []boardEntry `json:"entries"`
	// NextCursor is empty when the page reached the end of the board.
	// Bulk readers should follow it rather than incrementing offset:
	// OFFSET is O(offset), so an offset walk of the whole board is
	// quadratic while a cursor walk is linear.
	NextCursor string `json:"next_cursor"`
}

func (a *api) handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	skill, ok := SkillByName(r.PathValue("skill"))
	if !ok {
		a.writeError(w, http.StatusBadRequest, codeInvalidRequest, "unknown skill")
		return
	}

	q := r.URL.Query()
	profile := a.profileParam(r)

	limit, err := a.intParam(q, "limit", a.cfg.DefaultLimit)
	if err != nil || limit < 1 || limit > a.cfg.MaxLimit {
		a.writeError(w, http.StatusBadRequest, codeInvalidRequest,
			fmt.Sprintf("limit must be an integer in [1, %d]", a.cfg.MaxLimit))
		return
	}

	rawCursor := q.Get("cursor")
	hasOffset := q.Has("offset")
	if rawCursor != "" && hasOffset {
		a.writeError(w, http.StatusBadRequest, codeInvalidRequest,
			"offset and cursor are mutually exclusive")
		return
	}

	now := a.now()
	var rows []Row

	if rawCursor != "" {
		cur, cerr := DecodeCursor(rawCursor)
		if cerr != nil {
			a.writeError(w, http.StatusBadRequest, codeInvalidRequest, "malformed cursor")
			return
		}
		rows, err = a.store.LeaderboardByCursor(r.Context(), profile, skill.Type, cur, limit, now)
	} else {
		offset, oerr := a.intParam(q, "offset", 0)
		if oerr != nil || offset < 0 {
			a.writeError(w, http.StatusBadRequest, codeInvalidRequest,
				"offset must be a non-negative integer")
			return
		}
		if offset+limit > a.cfg.LeaderboardMaxRank {
			a.writeError(w, http.StatusBadRequest, codeInvalidRequest,
				fmt.Sprintf("offset+limit must not exceed %d; use cursor paging for deep reads",
					a.cfg.LeaderboardMaxRank))
			return
		}
		rows, err = a.store.LeaderboardByOffset(r.Context(), profile, skill.Type, offset, limit, now)
	}
	if err != nil {
		a.internal(w, "leaderboard", err)
		return
	}

	resp := leaderboardResponse{
		Skill:   skill.Name,
		Profile: profile,
		Entries: make([]boardEntry, 0, len(rows)),
	}
	var newest time.Time
	for _, row := range rows {
		resp.Entries = append(resp.Entries, boardEntry{
			Rank:      row.Rank,
			Name:      jstring.ToDisplayName(row.Username),
			Level:     row.Level,
			XP:        wholeXP(row.ValueX10),
			UpdatedAt: row.UpdatedAt,
		})
		if row.UpdatedAt.After(newest) {
			newest = row.UpdatedAt
		}
	}

	// A short page means the board is exhausted; only hand out a cursor
	// when there is plausibly more to read.
	if len(rows) == limit {
		last := rows[len(rows)-1]
		resp.NextCursor = Cursor{
			ValueX10:  last.ValueX10,
			UpdatedAt: last.UpdatedAt,
			AccountID: last.AccountID,
			Rank:      last.Rank + 1,
		}.Encode()
	}

	a.writeJSON(w, r, resp, newest)
}

// intParam parses an optional integer query parameter. A present but
// empty value is treated as absent.
func (a *api) intParam(q url.Values, name string, def int) (int, error) {
	raw := q.Get(name)
	if raw == "" {
		return def, nil
	}
	return strconv.Atoi(raw)
}
```

Add `"net/url"` to `handlers.go`'s imports.

Register the route in `(*api).register`, **before** the `/` catch-all:

```go
	mux.HandleFunc("GET /v1/leaderboards/{skill}", a.guard(a.handleLeaderboard))
```

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/hiscore/ -run TestLeaderboardEndpoint -v`

Expected: PASS

- [ ] **Step 5: Run the whole package with the race detector**

Run: `CGO_ENABLED=1 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/hiscore/`

Expected: PASS, no race reports.

- [ ] **Step 6: Commit**

```bash
git add modules/hiscore/handlers.go modules/hiscore/handlers_test.go
git commit --no-gpg-sign -m "feat(hiscore): /v1/leaderboards/{skill} with offset and cursor paging"
```

---

## Task 12: Module and service wiring

**Files:**
- Create: `modules/hiscore/hiscore.go`
- Test: `modules/hiscore/hiscore_test.go`

**Interfaces:**
- Consumes: `Config`, `NewStore`, `newAPI`, `gamedb.Open`, `server.New`, `services.NewBasicService`.
- Produces: `Hiscore`, `New(cfg Config, dbCfg gamedb.Config, logger *slog.Logger, serv *server.Server) (*Hiscore, error)`.

- [ ] **Step 1: Write the failing test**

Create `modules/hiscore/hiscore_test.go`:

```go
package hiscore

import (
	"flag"
	"net/http"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/dskit/server"
	"github.com/zsrv/goscape/pkg/dskit/services"
	"github.com/zsrv/goscape/pkg/gamedb"
)

// testServer builds a dskit server bound to an ephemeral port. Port 0
// avoids collisions when tests run in parallel; the tests drive
// serv.HTTP directly rather than dialing, because Server exposes no
// accessor for the bound listener address.
func testServer(t *testing.T, cfg *Config) *server.Server {
	t.Helper()
	cfg.Server.HTTPListenAddress = "127.0.0.1"
	cfg.Server.HTTPListenPort = 0
	cfg.Server.Log = noopLogger()
	server.DisableSignalHandling(&cfg.Server)

	serv, err := server.New(cfg.Server)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	t.Cleanup(func() { _ = serv.Close() })
	return serv
}

func TestHiscore_StartsAndRegistersRoutes(t *testing.T) {
	dbCfg := testGameDBConfig(t)

	cfg := defaultConfig(t)
	cfg.Enable = true
	serv := testServer(t, &cfg)

	h, err := New(cfg, dbCfg, noopLogger(), serv)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := services.StartAndAwaitRunning(t.Context(), h); err != nil {
		t.Fatalf("StartAndAwaitRunning: %v", err)
	}
	t.Cleanup(func() {
		// Own context: t.Context() is already canceled by cleanup time.
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := services.StopAndAwaitTerminated(stopCtx, h); err != nil {
			t.Errorf("StopAndAwaitTerminated: %v", err)
		}
	})

	// starting() registered the routes on the server's mux.
	rec := httptest.NewRecorder()
	serv.HTTP.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/skills", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("GET /v1/skills after start: status %d, want 200; body %s", rec.Code, rec.Body.String())
	}
}

func TestHiscore_RejectsInvalidConfig(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.Enable = true
	cfg.Profile = ""
	serv := testServer(t, &cfg)

	if _, err := New(cfg, testGameDBConfig(t), noopLogger(), serv); err == nil {
		t.Fatal("New: got nil error for an invalid config, want validation failure")
	}
}
```

Imports for `hiscore_test.go`: `context`, `net/http`, `net/http/httptest`, `testing`, `time`, plus the three goscape packages. `flag` and `gamedb` are **not** needed here (they belong to `testing_test.go`).

Add to `modules/hiscore/testing_test.go`:

```go
// testGameDBConfig returns a gamedb.Config pointing at a private
// in-memory sqlite database for this test.
func testGameDBConfig(t *testing.T) gamedb.Config {
	t.Helper()
	var cfg gamedb.Config
	fs := flag.NewFlagSet("", flag.PanicOnError)
	cfg.RegisterFlagsAndApplyDefaults(fs)
	cfg.SQLite.DSN = fmt.Sprintf("file:%s-mod?mode=memory&cache=shared", url.PathEscape(t.Name()))

	// The module opens its own pool but never migrates; bring the schema
	// up first through a throwaway pool, and hold it open for the test's
	// duration so the shared-cache in-memory DB survives.
	db, err := gamedb.Open(cfg, noopLogger())
	if err != nil {
		t.Fatalf("testGameDBConfig: open: %v", err)
	}
	if err := db.Migrate(t.Context()); err != nil {
		db.Close()
		t.Fatalf("testGameDBConfig: migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return cfg
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/hiscore/ -run TestHiscore_ -v`

Expected: FAIL — build error, `undefined: New`

- [ ] **Step 3: Write the implementation**

Create `modules/hiscore/hiscore.go`:

```go
package hiscore

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/zsrv/goscape/pkg/dskit/server"
	"github.com/zsrv/goscape/pkg/dskit/services"
	"github.com/zsrv/goscape/pkg/gamedb"
)

// Hiscore is the module. Like login, friends and account it owns a
// private pool to the central database (independent-clients model). The
// HTTP listener is a dskit server owned by the caller, which is what
// supplies request logging, timeouts and source-IP extraction.
type Hiscore struct {
	services.Service

	cfg   Config
	dbCfg gamedb.Config
	log   *slog.Logger
	serv  *server.Server

	db *gamedb.DB
}

// New validates the config and prepares the module. It does not open the
// database or serve traffic — that happens in the service lifecycle.
func New(cfg Config, dbCfg gamedb.Config, logger *slog.Logger, serv *server.Server) (*Hiscore, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	h := &Hiscore{cfg: cfg, dbCfg: dbCfg, log: logger, serv: serv}
	h.Service = services.NewBasicService(h.starting, h.running, h.stopping)
	return h, nil
}

func (h *Hiscore) starting(context.Context) error {
	db, err := gamedb.Open(h.dbCfg, h.log)
	if err != nil {
		return fmt.Errorf("hiscore: open central database: %w", err)
	}

	a, err := newAPI(h.cfg, NewStore(db), h.log)
	if err != nil {
		db.Close()
		return fmt.Errorf("hiscore: build api: %w", err)
	}
	a.register(h.serv.HTTP)

	h.db = db
	h.log.Info("hiscore api registered", slog.String("profile", h.cfg.Profile))
	return nil
}

func (h *Hiscore) running(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (h *Hiscore) stopping(_ error) error {
	if h.db != nil {
		if err := h.db.Close(); err != nil {
			h.log.Warn("hiscore: closing database pool", slog.Any("err", err))
		}
		h.db = nil
	}
	return nil
}
```

Note: the dskit `server.Server`'s own `Run`/`Shutdown` lifecycle is driven by the app wiring in Task 13 (mirroring how `ondemand` composes `NewOndemandService`), not by this module.

- [ ] **Step 4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/hiscore/ -run TestHiscore_ -v`

Expected: PASS

- [ ] **Step 5: Run the whole package**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/hiscore/...`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add modules/hiscore/hiscore.go modules/hiscore/hiscore_test.go modules/hiscore/testing_test.go
git commit --no-gpg-sign -m "feat(hiscore): module struct and dskit service lifecycle"
```

---

## Task 13: App wiring and example configs

**Files:**
- Modify: `cmd/goscape/app/modules.go`
- Modify: `cmd/goscape/app/config.go`
- Modify: `cmd/goscape/app/app.go`
- Modify: `examples/full-config-reference.yaml`
- Modify: `examples/bundled/goscape.yaml`
- Test: `cmd/goscape/app/config_test.go` (or the existing app-level config test file)

**Interfaces:**
- Consumes: `hiscore.New`, `hiscore.Config`.
- Produces: module target `Hiscore = "hiscore"`, `(*App).initHiscore`.

- [ ] **Step 1: Write the failing test**

Append to the app package's existing config test file (match its package and helper style):

```go
// TestConfig_HiscoreDefaults pins that the hiscore module is registered
// in the app config and defaults to off.
func TestConfig_HiscoreDefaults(t *testing.T) {
	cfg := newDefaultAppConfig(t) // use whatever helper this file already provides

	if cfg.Hiscore.Enable {
		t.Error("Hiscore.Enable: got true, want false by default")
	}
	if cfg.Hiscore.Profile != "main" {
		t.Errorf("Hiscore.Profile: got %q, want main", cfg.Hiscore.Profile)
	}
	if cfg.Hiscore.LeaderboardMaxRank != 500000 {
		t.Errorf("Hiscore.LeaderboardMaxRank: got %d, want 500000", cfg.Hiscore.LeaderboardMaxRank)
	}
}
```

If no such helper exists, construct the config the way the file's other tests do — do not invent a new construction path.

- [ ] **Step 2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape/app/ -run TestConfig_Hiscore -v`

Expected: FAIL — build error, `cfg.Hiscore undefined`

- [ ] **Step 3: Add the config section**

In `cmd/goscape/app/config.go`, add the field to the app `Config` struct alongside the other modules:

```go
	Hiscore hiscore.Config `yaml:"hiscore"`
```

and register its flags where the other modules' `RegisterFlagsAndApplyDefaults` calls live:

```go
	c.Hiscore.RegisterFlagsAndApplyDefaults(f)
```

and add it to `Validate` alongside the other modules:

```go
	if err := c.Hiscore.Validate(); err != nil {
		return err
	}
```

Import `"github.com/zsrv/goscape/modules/hiscore"`.

- [ ] **Step 4: Register the module**

In `cmd/goscape/app/modules.go`:

Add the target constant beside the others:

```go
	Hiscore  string = "hiscore"
```

Add the init function, modelled on `initAccount` and `initOnDemand` (the latter is the reference for building a dskit server and composing its service):

```go
func (g *App) initHiscore() (services.Service, error) {
	if !g.cfg.Hiscore.Enable {
		// arch-29.8: see initOnDemand's disabled branch for rationale.
		g.logger.Info("module disabled", "module", "hiscore")
		return nil, nil
	}

	logLevel := slog.Level(g.cfg.LogLevel)
	if g.cfg.Hiscore.Server.LogLevel != nil {
		logLevel = *g.cfg.Hiscore.Server.LogLevel
	}
	logger, err := log.NewLogger(logLevel, g.cfg.LogFormat, os.Stdout, log.WithSourceFormat(g.cfg.LogSource))
	if err != nil {
		return nil, fmt.Errorf("failed to create hiscore logger: %w", err)
	}
	logger = logger.With("component", "hiscore")

	g.cfg.Hiscore.Server.Log = logger
	server.DisableSignalHandling(&g.cfg.Hiscore.Server)
	serv, err := server.New(g.cfg.Hiscore.Server)
	if err != nil {
		return nil, err
	}

	h, err := hiscore.New(g.cfg.Hiscore, g.cfg.Database, logger, serv)
	if err != nil {
		// server.New already bound the listener; failing here would
		// otherwise leak the socket (same posture as initOnDemand).
		_ = serv.Close()
		return nil, fmt.Errorf("failed to create hiscore: %w", err)
	}
	g.hiscore = h

	return hiscore.NewHiscoreService(h, serv), nil
}
```

Register it and wire dependencies:

```go
	mm.RegisterModule(Hiscore, g.initHiscore)
```

```go
		Hiscore: {Common, Database},
```

and add `Hiscore` to `SingleBinary`'s dependency list:

```go
		SingleBinary: {OnDemand, Friends, Login, World, Account, Hiscore},
```

Add the `hiscore *hiscore.Hiscore` field to the `App` struct in `cmd/goscape/app/app.go`, beside `account`.

Also add `Database` to the disabled-check in `initDatabase` so the migration anchor runs when only hiscore is enabled:

```go
	if !g.cfg.Login.Enable && !g.cfg.Friends.Enable && !g.cfg.Account.Enable && !g.cfg.Hiscore.Enable {
```

- [ ] **Step 5: Add the composed service constructor**

Append to `modules/hiscore/hiscore.go`:

```go
// NewHiscoreService composes the module with the dskit server that
// carries its routes, so the server's Run/Shutdown lifecycle is driven
// by the same service the module manager supervises. Mirrors
// modules/ondemand.NewOndemandService.
//
// The module is started first (it registers routes and opens the pool),
// then the HTTP server runs; on shutdown the server stops before the
// module releases its pool.
func NewHiscoreService(h *Hiscore, serv *server.Server) services.Service {
	serverDone := make(chan error, 1)

	startingFn := func(ctx context.Context) error {
		return services.StartAndAwaitRunning(ctx, h)
	}

	runFn := func(ctx context.Context) error {
		go func() {
			defer close(serverDone)
			serverDone <- serv.Run()
		}()
		select {
		case <-ctx.Done():
			return nil
		case err := <-serverDone:
			if err != nil {
				return err
			}
			// A clean server exit while the context is still live means
			// the listener went away underneath us — a failure, not a
			// shutdown. Same posture as NewOndemandService.
			return fmt.Errorf("hiscore server stopped unexpectedly")
		}
	}

	stoppingFn := func(_ error) error {
		// Shut the HTTP server down first (this also unblocks runFn),
		// wait for the server goroutine, and only then let the module
		// release its database pool — no in-flight handler can still be
		// querying by that point.
		serv.Shutdown()
		<-serverDone
		serv.Log.Info("hiscore server stopped")

		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return services.StopAndAwaitTerminated(stopCtx, h)
	}

	return services.NewBasicService(startingFn, runFn, stoppingFn)
}
```

Add `"time"` to `hiscore.go`'s imports.

This mirrors `modules/ondemand.NewOndemandService` (`modules/ondemand/ondemand.go:104`) with two deliberate differences: ondemand passes `nil` as its `startingFn` because its routes are registered in `initOnDemand` before the service exists, whereas here `startingFn` starts the module so routes are registered and the pool is open **before** `serv.Run()` accepts traffic; and ondemand takes a `servicesToWaitFor` callback to drain dependents, which hiscore has none of.

- [ ] **Step 6: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./cmd/goscape/app/ -run TestConfig_Hiscore -v`

Expected: PASS

- [ ] **Step 7: Add the example config sections**

In `examples/full-config-reference.yaml`, add a `hiscore:` section documenting **every** option at its default, matching the file's existing comment style (`# ... CLI: --flag (default: X)`):

```yaml
# Hiscores read API. Serves public JSON leaderboards over the hiscore
# tables that the login module populates on logout. Designed to sit
# behind an API gateway (Kong) which owns authentication and per-consumer
# rate limiting; this module is anonymous-safe on its own.
hiscore:
  # CLI: --hiscore.enable (default: false)
  enable: false
  # Default profile queried when a request does not specify one.
  # CLI: --hiscore.profile (default: main)
  profile: main
  # Cache-Control max-age on API responses. Responses are also ETag'd.
  # CLI: --hiscore.cache-max-age (default: 1m)
  cache_max_age: 1m
  # CLI: --hiscore.default-limit (default: 25)
  default_limit: 25
  # CLI: --hiscore.max-limit (default: 100)
  max_limit: 100
  # Deepest rank reachable by offset paging. Cursor paging is unbounded
  # and is the intended mechanism for bulk reads.
  # CLI: --hiscore.leaderboard-max-rank (default: 500000)
  leaderboard_max_rank: 500000
  # Read gateway-supplied X-Consumer-* headers for logging. Nothing is
  # ever authorized by them; leave false unless a gateway fronts this.
  # CLI: --hiscore.trust-gateway-headers (default: false)
  trust_gateway_headers: false
  # Coarse in-process requests/minute per caller, for when no gateway
  # limits apply. 0 disables. CLI: --hiscore.backstop-rate (default: 120)
  backstop_rate: 120
  # Listener + logging options are the standard dskit server block.
  # CLI: --hiscore.http-listen-address (default: 127.0.0.1)
  http_listen_address: 127.0.0.1
  # CLI: --hiscore.http-listen-port (default: 8082)
  http_listen_port: 8082
```

Set the default listen port in `config.go` to `8082` (portal HTTP is 8081) if `server.Config`'s default does not already differ per module — check what `RegisterFlagsAndApplyDefaults` produces and set an explicit default so two modules never collide in the `all` target.

In `examples/bundled/goscape.yaml`, enable the module in the "run everything" preset:

```yaml
hiscore:
  enable: true
```

- [ ] **Step 8: Verify the example configs parse**

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go run -trimpath ./cmd/goscape \
  --config.file examples/bundled/goscape.yaml --config.verify=true
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache CGO_ENABLED=0 go run -trimpath ./cmd/goscape \
  --config.file examples/full-config-reference.yaml --config.verify=true
```

Expected: both exit 0. Decoding is strict — an unknown or misspelled key is a fatal boot error, so this catches YAML/struct-tag drift.

- [ ] **Step 9: Build and test everything**

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
```

Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add cmd/goscape/app/ modules/hiscore/hiscore.go examples/
git commit --no-gpg-sign -m "feat(hiscore): register module in the app and example configs"
```

---

## Task 14: Helm Kong configuration

**Files:**
- Create: `production/helm/goscape/templates/hiscore-kong.yaml`
- Modify: `production/helm/goscape/values.yaml`
- Modify: `production/helm/goscape/values.schema.json`
- Modify: `production/helm/goscape/templates/_helpers.tpl` (the `goscape.baseConfig` define)
- Modify: `production/helm/goscape/templates/service.yaml`
- Modify: `production/helm/goscape/templates/deployment.yaml` and `statefulset.yaml` (container port) — confirm which one carries `containerPort` entries; they may both go through `goscape.podTemplate` in `_helpers.tpl`, in which case only that define changes.

**Interfaces:**
- Consumes: nothing from Go code.
- Produces: values keys `goscape.ports.hiscoreHTTP`, and the top-level `hiscoreGateway.*` block.

**Chart facts this task depends on** (already verified, do not re-derive): the chart has **one** Service (`templates/service.yaml`) exposing named ports driven by `.Values.goscape.ports.*`, not a Service per module. The goscape config file is rendered by the `goscape.baseConfig` define in `_helpers.tpl`, gated on `.Values.deploymentMode`; DB-backed modules use the gate `or (eq $mode "SingleBinary") (eq $mode "Management")`. Helper names are `goscape.fullname` and `goscape.labels`. `templates/ondemand-ingress.yaml` is the ingress precedent.

Note: `goscape.baseConfig` currently has no `account:` section either. Adding one is **out of scope** — do not touch it.

- [ ] **Step 1: Add the port and gateway values**

In `production/helm/goscape/values.yaml`, add to the existing `goscape.ports` block:

```yaml
    # -- hiscore API HTTP container port
    hiscoreHTTP: 8082
```

and append a new top-level block (named `hiscoreGateway`, not nested under `goscape`, so it reads as gateway configuration rather than as a goscape process setting):

```yaml
# Kong gateway configuration for the hiscores API.
#
# This renders Kong Ingress Controller CONFIGURATION only (Ingress,
# KongPlugin, KongConsumer). It does NOT install Kong. Kong is a
# prerequisite, installed separately from its own chart, because a
# gateway is cluster-wide infrastructure shared by every service behind
# it rather than a per-application component.
hiscoreGateway:
  createGatewayConfig: false
  host: ""
  ingressClassName: kong
  # Consumer used for unauthenticated callers (the website, casual
  # users). key-auth falls back to it instead of rejecting, because
  # hiscore data is public — keys exist for quota and accountability.
  anonymousConsumer: hiscore-anonymous
  rateLimit:
    anonymousPerMinute: 60
    consumerPerMinute: 600
  cacheTTLSeconds: 60
  corsOrigins: []
  # Third-party consumers. Each entry names an EXISTING Secret holding
  # the key-auth credential; key material never lives in values.yaml.
  #   - username: partner-a
  #     credentialSecret: hiscore-partner-a-key
  consumers: []
```

Add matching entries to `values.schema.json` following the file's existing conventions.

- [ ] **Step 2: Render the module config**

In the `goscape.baseConfig` define in `_helpers.tpl`, add a `hiscore:` section after the `friends:` section, using the same deployment-mode gate as the other database-backed modules:

```yaml
hiscore:
  enable: {{ or (eq $mode "SingleBinary") (eq $mode "Management") }}
  http_listen_network: tcp
  http_listen_address: 0.0.0.0
  http_listen_port: {{ $g.ports.hiscoreHTTP }}
  profile: {{ $g.node.profile | quote }}
  trust_gateway_headers: {{ $.Values.hiscoreGateway.createGatewayConfig }}
```

`trust_gateway_headers` follows the gateway toggle: the headers are only meaningful when a gateway is actually in front, and nothing is authorized by them either way.

**Note:** `goscape.baseConfig` is invoked with a context where `$g` is `.Values.goscape`; confirm whether `$.Values` resolves inside that define (it is a `define`, so `$` is the template's root context as passed in). If it does not, bind the value into a variable at the top of the define alongside `$mode` and `$g`, matching the existing style.

- [ ] **Step 3: Expose the port on the Service**

In `templates/service.yaml`, add a `hiscore-http` port alongside the existing `login-grpc` / `friends-grpc` entries, under the **same** deployment-mode condition those use (read the surrounding `{{- if }}` blocks and match them exactly):

```yaml
    - name: hiscore-http
      port: {{ .Values.goscape.ports.hiscoreHTTP }}
      targetPort: hiscore-http
      protocol: TCP
```

Add the corresponding `containerPort` named `hiscore-http` wherever the other module ports are declared (check `goscape.podTemplate` in `_helpers.tpl` first — if the container ports live there, that is the only place to change).

- [ ] **Step 4: Write the template**

Create `production/helm/goscape/templates/hiscore-kong.yaml`:

```yaml
{{- if .Values.hiscoreGateway.createGatewayConfig }}
{{- if not (.Capabilities.APIVersions.Has "configuration.konghq.com/v1") }}
{{- fail "hiscoreGateway.createGatewayConfig is true but the Kong Ingress Controller CRDs (configuration.konghq.com/v1) are not installed in this cluster. Kong is a prerequisite of this chart, not a dependency: install it from its own chart first, or set hiscoreGateway.createGatewayConfig=false." }}
{{- end }}
{{- if not .Values.hiscoreGateway.host }}
{{- fail "hiscoreGateway.host must be set when hiscoreGateway.createGatewayConfig is true." }}
{{- end }}
---
apiVersion: configuration.konghq.com/v1
kind: KongPlugin
metadata:
  name: {{ include "goscape.fullname" . }}-hiscore-key-auth
  labels:
    {{- include "goscape.labels" . | nindent 4 }}
plugin: key-auth
config:
  key_names:
    - apikey
  # Unauthenticated callers fall back to the anonymous consumer instead
  # of being rejected: hiscore data is public, so keys exist for quota
  # and accountability, not confidentiality.
  anonymous: {{ .Values.hiscoreGateway.anonymousConsumer | quote }}
  hide_credentials: true
---
apiVersion: configuration.konghq.com/v1
kind: KongPlugin
metadata:
  name: {{ include "goscape.fullname" . }}-hiscore-rate-limiting
  labels:
    {{- include "goscape.labels" . | nindent 4 }}
plugin: rate-limiting
config:
  minute: {{ .Values.hiscoreGateway.rateLimit.consumerPerMinute }}
  policy: local
  limit_by: consumer
---
apiVersion: configuration.konghq.com/v1
kind: KongPlugin
metadata:
  name: {{ include "goscape.fullname" . }}-hiscore-proxy-cache
  labels:
    {{- include "goscape.labels" . | nindent 4 }}
plugin: proxy-cache
config:
  strategy: memory
  cache_ttl: {{ .Values.hiscoreGateway.cacheTTLSeconds }}
  content_type:
    - application/json
    - application/json; charset=utf-8
  request_method:
    - GET
    - HEAD
  response_code:
    - 200
    - 304
{{- if .Values.hiscoreGateway.corsOrigins }}
---
apiVersion: configuration.konghq.com/v1
kind: KongPlugin
metadata:
  name: {{ include "goscape.fullname" . }}-hiscore-cors
  labels:
    {{- include "goscape.labels" . | nindent 4 }}
plugin: cors
config:
  origins:
    {{- toYaml .Values.hiscoreGateway.corsOrigins | nindent 4 }}
  methods:
    - GET
    - HEAD
  credentials: false
{{- end }}
{{- range .Values.hiscoreGateway.consumers }}
---
apiVersion: configuration.konghq.com/v1
kind: KongConsumer
metadata:
  name: {{ .username }}
  labels:
    {{- include "goscape.labels" $ | nindent 4 }}
  annotations:
    kubernetes.io/ingress.class: {{ $.Values.hiscoreGateway.ingressClassName | quote }}
username: {{ .username | quote }}
credentials:
  - {{ .credentialSecret | quote }}
{{- end }}
---
apiVersion: configuration.konghq.com/v1
kind: KongConsumer
metadata:
  name: {{ .Values.hiscoreGateway.anonymousConsumer }}
  labels:
    {{- include "goscape.labels" . | nindent 4 }}
  annotations:
    kubernetes.io/ingress.class: {{ .Values.hiscoreGateway.ingressClassName | quote }}
username: {{ .Values.hiscoreGateway.anonymousConsumer | quote }}
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: {{ include "goscape.fullname" . }}-hiscore
  labels:
    {{- include "goscape.labels" . | nindent 4 }}
  annotations:
    konghq.com/plugins: >-
      {{ include "goscape.fullname" . }}-hiscore-key-auth,{{ include "goscape.fullname" . }}-hiscore-rate-limiting,{{ include "goscape.fullname" . }}-hiscore-proxy-cache{{ if .Values.hiscoreGateway.corsOrigins }},{{ include "goscape.fullname" . }}-hiscore-cors{{ end }}
    konghq.com/strip-path: "false"
spec:
  ingressClassName: {{ .Values.hiscoreGateway.ingressClassName }}
  rules:
    - host: {{ .Values.hiscoreGateway.host | quote }}
      http:
        paths:
          - path: /v1
            pathType: Prefix
            backend:
              service:
                # The chart exposes one Service with named ports; there
                # is no per-module Service to target.
                name: {{ include "goscape.fullname" . }}
                port:
                  name: hiscore-http
{{- end }}
```

`goscape.fullname` and `goscape.labels` are the chart's real helper names (`production/helm/goscape/templates/_helpers.tpl:7,26`), and the Ingress targets the chart's single Service by the `hiscore-http` port name added in Step 3.

- [ ] **Step 5: Verify the chart renders both ways**

Run:

```bash
helm template test production/helm/goscape > /dev/null
helm template test production/helm/goscape \
  --set hiscoreGateway.createGatewayConfig=true \
  --set hiscoreGateway.host=hiscores.example.com \
  --api-versions configuration.konghq.com/v1 | grep -c KongPlugin
```

Expected: the first renders cleanly with no Kong objects; the second prints `3` (key-auth, rate-limiting, proxy-cache — no cors, since `corsOrigins` is empty).

- [ ] **Step 6: Verify the capability guard fires**

Run:

```bash
helm template test production/helm/goscape \
  --set hiscoreGateway.createGatewayConfig=true \
  --set hiscoreGateway.host=hiscores.example.com
```

Expected: FAIL with the "Kong Ingress Controller CRDs ... are not installed" message (no `--api-versions`, so the capability check is not satisfied).

- [ ] **Step 7: Run the chart's own checks**

Run: `make -C production/helm/goscape` (or whatever target that Makefile provides — read it first; if it runs `helm lint`, ensure it passes).

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add production/helm/goscape/
git commit --no-gpg-sign -m "feat(helm): optional Kong gateway configuration for the hiscore API"
```

---

## Task 15: Documentation

**Files:**
- Create: `docs/guides/hiscores-api.md` (confirm the real guides directory first)
- Modify: `docs/PORTING.md`
- Modify: the docs-site navigation file (`mkdocs.yml` or equivalent — confirm)

**Interfaces:** none (documentation only).

- [ ] **Step 1: Locate the docs structure**

Run:

```bash
ls docs/
find . -maxdepth 2 -name 'mkdocs*.yml' -o -maxdepth 2 -name 'zensical*' | head
```

Read one existing guide to match its front-matter and heading conventions before writing. Place the new guide where the others live; do **not** create a new top-level directory.

- [ ] **Step 2: Write the guide**

The guide must cover, each as its own section:

1. **What it is** — a public read API over hiscore data; written on logout by the login module; staff and banned accounts excluded.
2. **Enabling it** — the `hiscore:` config block, the `--target=hiscore` standalone mode, and the fact that it needs the central database.
3. **Endpoint reference** — all three endpoints with a real `curl` and a real JSON response body for each. Copy the exact field names from `playerResponse`, `skillEntry`, `leaderboardResponse`, and `boardEntry`; do not paraphrase them.
4. **XP units** — stated explicitly: the API returns whole XP; the database stores tenths.
5. **Sparse skills** — why `ranked: false` entries exist (the level-15 export threshold).
6. **Pagination** — offset for random access, cursor for bulk, with the explicit guidance that a full-board walk must use cursors because offset paging is O(offset) per request and quadratic overall.
7. **Caching** — `ETag` / `If-None-Match` / `Cache-Control`, and that clients should honour them.
8. **Deploying behind Kong** — the prerequisite `helm install` for Kong's own chart, then the goscape values to set, then a worked third-party onboarding example:

```bash
kubectl create secret generic hiscore-partner-a-key \
  --from-literal=kongCredType=key-auth \
  --from-literal=key="$(openssl rand -hex 32)"
```

   followed by adding `{username: partner-a, credentialSecret: hiscore-partner-a-key}` to `hiscoreGateway.consumers`, and a `curl -H 'apikey: ...'` showing the higher rate limit in effect.
9. **DB-less Kong** — a note that a Kong running DB-less is configured by decK rather than these CRDs, and that the rendered objects translate directly.
10. **Security posture** — the module is anonymous-safe; gateway headers are never authorization; `trust_gateway_headers` only affects logging.

- [ ] **Step 3: Add the PORTING.md note**

Add an entry to `docs/PORTING.md` in the arc list, matching the surrounding entries' style, recording that:

- `modules/hiscore` is a **goscape extension with no Engine-TS counterpart** — Engine-TS has no hiscore serving endpoint at any pinned revision (`hiscore` appears there only in `db/types.ts`, the Prisma migrations, and `LoginServer.ts`).
- It is therefore **outside the TS fidelity ledger** and must not be reported by a future parity audit as an unclosed divergence.
- The only central-database change is additive indexes (migration `000004`); the write path in `modules/login` is untouched.
- Spec: `docs/superpowers/specs/2026-08-19-hiscore-api-design.md`.

- [ ] **Step 4: Add the guide to the docs navigation**

Add the new page to the site navigation file, in the position matching the other module guides.

- [ ] **Step 5: Verify the docs build**

Run the repository's docs build command (read the `Makefile` or docs tooling config to find it; do not guess).

Expected: builds cleanly, new page present in the navigation.

- [ ] **Step 6: Final full verification**

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/hiscore/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...
CGO_ENABLED=1 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/hiscore/... ./pkg/gamedb/...
```

Expected: all PASS. Note `modules/world`'s suite is slow (~150s); that is pre-existing.

- [ ] **Step 7: Optional — run against Postgres**

If a Postgres instance is available, re-run the store tests against the real backend to confirm the SQL is dialect-clean:

```bash
GOSCAPE_TEST_POSTGRES_DSN='postgres://...' \
  GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/hiscore/...
```

Expected: PASS. If unavailable, note it as unverified rather than claiming it passed.

- [ ] **Step 8: Commit**

```bash
git add docs/
git commit --no-gpg-sign -m "docs(hiscore): API guide and PORTING extension note"
```

---

## Post-Implementation

Not part of this plan, tracked so they are not forgotten:

1. **User smoke test.** The server must be launched by the user, not by an agent. Provide the `curl` commands from the guide.
2. **Backport** to rev-254 / rev-245.2 / rev-244 / rev-225. The central DB schema and the login write path are identical across branches, so the module should be near-copyable; per-branch adaptation is expected only in `cmd/goscape/app/` wiring and the example configs.
3. **Nothing is pushed.** Per this repository's convention, commits stay local until the user asks.
