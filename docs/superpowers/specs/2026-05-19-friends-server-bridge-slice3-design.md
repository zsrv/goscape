# Friends-server bridge — slice 3 design: SQLite persistence

**Date:** 2026-05-19
**Slice:** 3 of 7 (friends-server bridge arc)
**Predecessor:** slice 2 (close commit `01ca9d11`, retired `NAI-72-D-FRIENDS-SERVER-BRIDGE`)
**Closes:** `NAI-S1-D-INMEMORY-REPO`

## 0. Forward map (what ships in this slice)

| File | New / changed | Notes |
|---|---|---|
| `modules/friends/migrations/000001_init.up.sql` | **new** | `friendlist` + `ignorelist` tables, profile-scoped |
| `modules/friends/db.go` | **new** | `openDB(dsn)` + `migrateDB`; mirrors `modules/login/db.go` |
| `modules/friends/db_test.go` | **new** | DSN parsing, migration apply, pragma checks, idempotency |
| `modules/friends/repository.go` | **changed** | swap maps → SQL for `friends` + `ignores`; presence stays in-memory |
| `modules/friends/repository_test.go` | **changed** | `r := NewRepository()` → `r, _ := newTestRepo(t)`; 15 existing tests' assertions remain |
| `modules/friends/config.go` | **changed** | add `SQLiteDSN string` + `friends.sqlite-dsn` flag (default `data/friends.db`) |
| `modules/friends/friends.go` | **changed** | `starting()` opens DB before constructing repo; `stopping()` closes DB |
| `modules/friends/handler_test.go` | **changed** | test fixture switches to `newTestRepo` (3 call sites) |

Total LOC estimate: ~700 added, ~80 deleted. Sibling-shape to login: `modules/login/db.go` is 212 LOC, `modules/login/db_test.go` is 374 LOC — friends slice 3 will land somewhat smaller because the surface is narrower (no bcrypt, no IP bans, no sessions; only friends + ignores).

## 1. Persistence model: presence in-memory, social-lists in SQL

The TS reference (`Engine-TS/src/server/friend/FriendServerRepository.ts`) keeps two distinct categories of state:

| State | TS treatment | goscape slice 3 |
|---|---|---|
| `playersByWorld`, `worldByPlayer`, `privateChatByPlayer`, `playerStaff` | In-memory only | **In-memory** (unchanged from slice 1) |
| `playerFriends`, `playerIgnores` | In-memory cache + SQL persistence (`friendlist`, `ignorelist` tables) | **SQL-backed** (no cache layer) |

**Rationale.** Presence is per-connection ephemeral state. A crashed world leaves stale presence rows that have to be reconciled against reality (TS handles this via `WorldConnect` calling `initializeWorld(world, size)` which resets the playerCount slot to zero). Persisting presence rows just to wipe them on the next `WorldConnect` is pointless. Friends/ignores, by contrast, are durable user-owned data — losing them on restart is a regression.

This matches the Engine-TS architecture exactly. The slice-1 in-memory friends/ignores maps were a stand-in; this slice replaces them with the equivalent SQL tables.

**Deferred:** the slice-1 spec floated keeping a write-through in-memory cache layer over SQL for friends. Skipping for slice 3 — the call rates are bounded (one `GetFriends` per player login, occasional `Add/DeleteFriend`), and dropping the cache simplifies the slice substantially. Add later if benchmarks demand it.

## 2. Schema

```sql
-- modules/friends/migrations/000001_init.up.sql

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

### Schema decisions

- **`username37` is the row key, not `account_id`.** The friends-server is a separate process with its own DB; it has no `account` table to FK into. Mapping every operation through a base37→string→account_id lookup (the TS pattern) is pointless overhead here. `username37` ≤ 2³⁷ < 2⁶³ → fits cleanly in signed `INTEGER` (SQLite's only integer type). **Deviation tag:** `NAI-S3-D-USERNAME37-NOT-ACCOUNTID` (permanent — architectural).
- **`profile` is the first PK column.** Mirrors TS schema (`PRIMARY KEY (profile, account_id, friend_account_id)`). Range scans for a profile (e.g., bulk admin tooling) hit a contiguous prefix.
- **`idx_friendlist_target`** supports `GetFollowers(target)` — currently called by no handler (slice 4 work), but slice 4 will fan out broadcasts via this query and the index keeps the call cheap. No matching index on `ignorelist` — `GetFollowers` semantic doesn't apply to ignores.
- **`created TEXT`** uses `CURRENT_TIMESTAMP` default (mirrors TS `DATETIME` default and matches `modules/login/migrations/000001_init.up.sql` which uses TEXT for time fields). Not exposed via the Repository API; purely diagnostic.
- **No `down` migration.** `modules/login/migrations/` has only `.up.sql`; we follow that precedent. Schema migrations roll forward only.

## 3. Repository API

The public method set stays identical. Bodies switch to SQL for the four social-list methods + `GetFollowers`; presence methods keep their map-based bodies.

**Signature changes:**

```go
// Before (slice 1):
func NewRepository() *Repository

// After (slice 3):
func NewRepository(db *sql.DB, profile string) *Repository
```

The DB and profile are passed in so the Repository remains a pure value-bag and is testable in isolation from `Friends.starting()`. Profile is captured at construction (matches TS `constructor(profile)` shape) and used as the WHERE-clause prefix on every SQL call.

**Per-call context handling.** SQL methods accept a `context.Context` parameter:

```go
func (r *Repository) AddFriend(ctx context.Context, owner, target uint64) error
func (r *Repository) DeleteFriend(ctx context.Context, owner, target uint64) error
func (r *Repository) GetFriends(ctx context.Context, owner uint64) ([]uint64, error)
func (r *Repository) AddIgnore(ctx context.Context, owner, target uint64) error
func (r *Repository) DeleteIgnore(ctx context.Context, owner, target uint64) error
func (r *Repository) GetIgnores(ctx context.Context, owner uint64) ([]uint64, error)
func (r *Repository) GetFollowers(ctx context.Context, target uint64) ([]uint64, error)
```

This breaks slice 1's "no error returns" pattern. The five presence methods (`GetWorld`, `InitializeWorld`, `initializeWorldIfAbsent`, `Register`, `Unregister`, `SetChatMode`, `GetChatMode`, `IsVisibleTo`) keep their pure-in-memory signatures with no `ctx` and no error return — they touch no I/O.

`IsVisibleTo` is a special case: TS implementation reads from in-memory `playerFriends` to check the friends-only branch. To keep `IsVisibleTo` lock-free + I/O-free, the slice-3 impl will look up friends from SQL inside `IsVisibleTo`. **However**, `IsVisibleTo` is called from `GetFriends` (per TS) — turning it into an SQL call inside a loop creates an N+1 in the slice 4 broadcast path. **Decision:** `IsVisibleTo` gains a `ctx context.Context` parameter and returns `(bool, error)`. Callers in slice 4 will batch.

Handlers in `handler.go` must thread `ctx` through to every Repository call. The gRPC handler signatures already receive a `context.Context` from the gRPC stack — use that.

**Idempotency.** `AddFriend` / `AddIgnore` use `INSERT OR IGNORE` to preserve slice 1's "Idempotent" docstring contract. `DeleteFriend` / `DeleteIgnore` use plain `DELETE` (already idempotent — no row → no-op).

## 4. Config

Add to `modules/friends/config.go`:

```go
SQLiteDSN string `yaml:"sqlite_dsn"`
```

Flag: `friends.sqlite-dsn`, default `data/friends.db`. Matches `login.sqlite-dsn` shape exactly.

## 5. Lifecycle

`Friends.starting(ctx)` becomes:

```go
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
```

`Friends.stopping(error)` closes `f.db` after the existing `f.lis.Close()` edge-case.

The `Friends` struct gains `db *sql.DB` alongside `repo *Repository`.

## 6. Test surface

### 6.1 Existing 15 tests in `repository_test.go`

Every test starts with `r := NewRepository()`. After slice 3 this becomes:

```go
r, _ := newTestRepo(t) // helper closes the DB via t.Cleanup
```

`newTestRepo(t *testing.T) (*Repository, *sql.DB)` mirrors `modules/login/db_test.go:createTestDB`:

```go
func newTestRepo(t *testing.T) (*Repository, *sql.DB) {
    t.Helper()
    dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", url.PathEscape(t.Name()))
    db, err := openDB(dsn)
    if err != nil { t.Fatalf("openDB: %v", err) }
    t.Cleanup(func() { db.Close() })
    return NewRepository(db, "test"), db
}
```

The 15 existing test bodies' **assertions** stay byte-identical. The mechanical edit is: `NewRepository()` → `newTestRepo(t)` (and where the test uses the repo without holding a `*sql.DB` reference, the `_` discard suffices).

Tests that call SQL-backed methods (`AddFriend`, `GetFollowers`, etc.) will need to thread `t.Context()` — adding one parameter per call. The assertion shape is unchanged.

### 6.2 New tests in `db_test.go`

Following `modules/login/db_test.go` cadence:

- `TestOpenDB_AppliesMigrations` — open DB, query `sqlite_schema` for `friendlist` + `ignorelist`
- `TestOpenDB_Idempotent` — open twice, second call no-ops (migrate returns `ErrNoChange`)
- `TestOpenDB_SetsPragmas` — `PRAGMA journal_mode` → `wal` (or `memory` for in-memory DSN), `PRAGMA foreign_keys` → 1
- `TestOpenDB_BadDSN` — invalid DSN returns error, no panic

### 6.3 New SQL-concern tests in `repository_test.go`

- `TestRepository_AddFriend_Idempotent_SQL` — call AddFriend twice, single row
- `TestRepository_AddFriend_RespectsProfileBoundary` — same `owner_username37` + `target_username37` across two profiles → two rows; `GetFriends` on each profile sees only its own
- `TestRepository_GetFollowers_UsesTargetIndex` — multiple owners follow one target, `GetFollowers(target)` returns them all
- `TestRepository_GetFriends_Order` — entries returned, order unspecified but deterministic per call (SQLite `PRIMARY KEY` ordering)

### 6.4 End-to-end smoke (already exists)

`modules/world/friends_smoke_test.go` (T6 from slice 2) exercises every RPC against a live friends-server. Slice 3 re-runs it post-swap; if it still passes, the wire-protocol contract holds with SQLite-backed storage.

## 7. Deviation tags

Opened by this slice:

- `NAI-S3-D-USERNAME37-NOT-ACCOUNTID` (**permanent**) — schema PK is `username37`, not `account_id`. Friends-server is a separate process with no `account` table. Rationale in §2.
- `NAI-S3-D-NO-IN-MEMORY-CACHE` (**permanent**) — TS keeps an in-memory cache of friends/ignores over its SQL store. Slice 3 reads SQL on every call. Acceptable given the call rate; revisit on benchmarks.
- `NAI-S3-D-NO-LIST-CAPS` (**permanent**) — TS enforces a 100-entry list cap inside `addFriend`/`addIgnore`. Slice 3 leaves this to the eventual world-side caller; persistence layer doesn't reject inserts on size.

Retired by this slice:

- `NAI-S1-D-INMEMORY-REPO`

Stays open:

- `NAI-S1-D-LAZY-WORLDINIT` (permanent, TS-faithful)
- `NAI-S1-D-PLAYERCAP-LOG-ONLY` (slice 4)
- `NAI-S1-D-NO-FOLLOWER-BROADCAST` (slice 4)
- `NAI-S1-D-PM-NO-DELIVERY` (slice 4)
- `NAI-S1-D-PM-NO-PERSISTENCE` (slice 6)
- `NAI-S2-D-PLAYERLOGIN-AT-PROCESSLOGINS` (permanent)
- `NAI-S2-D-PLAYERLOGOUT-BOTH-PATHS` (permanent)
- `NAI-S2-D-PLAYERLOGIN-IGNORES-ACCEPTED` (slice 4)

## 8. Out of scope

- **In-memory cache over SQL.** Defer to a later slice if measurements warrant.
- **Friend/ignore list size caps.** TS enforces a 100-entry cap in `addFriend`/`addIgnore`. Slice 1's in-memory repo doesn't enforce this; slice 3 won't either. Open as `NAI-S3-D-NO-LIST-CAPS` (**permanent** — caps live at the call site once the world-side enforces them, not in the persistence layer).
- **`SubscribeUpdates` push stream.** Slice 4.
- **Chat logging tables (`PUBLIC_CHAT_LOG`, `private_chat`).** Slice 6.
- **Multi-profile reconciliation.** Each module instance pins one profile via `friends.node-profile`. Cross-profile queries are out.
- **Down migrations.** Login precedent doesn't ship them; we won't either.

## 9. Build sequence

1. `modules/friends/migrations/000001_init.up.sql` — schema-as-code, no Go yet.
2. `modules/friends/db.go` — `openDB` + `migrateDB`, mirror login.
3. `modules/friends/db_test.go` — exercise `openDB` against in-memory DSN.
4. `modules/friends/repository.go` — change `NewRepository` signature, switch the four SQL methods + `GetFollowers` + `IsVisibleTo` to SQL.
5. `modules/friends/repository_test.go` — swap fixture, thread `ctx`, add new SQL-concern tests.
6. `modules/friends/handler.go` — thread `ctx` from RPC handlers into Repository calls.
7. `modules/friends/handler_test.go` — swap fixture in the 3 call sites.
8. `modules/friends/config.go` — add `SQLiteDSN` + flag.
9. `modules/friends/friends.go` — `starting()` opens DB, `stopping()` closes it.
10. Re-run `modules/world/friends_smoke_test.go` against live friends-server.
11. Retire `NAI-S1-D-INMEMORY-REPO` doc-comment ref in `repository.go`.

Each step is its own commit and its own subagent task in the plan.

## 10. Risk / open follow-ups

- **Migration directory + `proto:` Makefile target.** Pre-existing working-tree dirty Makefile edit (the `proto:` target shadow); not blocking slice 3 but worth a 1-line `.PHONY` fix during this slice if the Makefile is already modified.
- **SQLite `cache=shared` test DSN gotcha.** The in-memory DSN must include `cache=shared` or each `sql.DB` connection in the pool gets its own empty database. Already handled in login.
- **`golang-migrate` driver close behaviour.** `m.Close()` closes the underlying `*sql.DB`. Login already documents this (`db.go:56-58`); slice 3 inherits the same workaround verbatim.
- **`IsVisibleTo` N+1.** Resolved at design time: slice 4 broadcast loop must batch the friends lookup, not call `IsVisibleTo` per target.
