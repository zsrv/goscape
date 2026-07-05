# Central Database Consolidation + PostgreSQL Backend Design

**Date:** 2026-07-05
**Branch:** rev-274 only (explicit user decision; no ports to other rev branches in this effort)
**Tech:** Go 1.26+, modernc.org/sqlite (existing), jackc/pgx/v5 (new, phase 2), golang-migrate (existing)
**Scope:** Merge the login and friends servers' separate SQLite databases into one **central database** that services connect to as independent clients, re-key the friends tables to the TS 274 schema shape (restoring the TS behaviors the split blocked), add real FK constraints as a goscape extension, and add PostgreSQL as a user-selectable backend alongside SQLite.

## Context

### Where we are

goscape currently runs two SQLite databases:

- **`data/login.db`** (`modules/login/migrations/`): `account`, `account_login`, `session`, `login`, `ipban`, `hiscore`, `hiscore_large`, plus dormant landing tables (`message_thread`, `message`, `message_status`, `account_session`, `wealth_event`). Account-referencing tables already carry real FKs with `ON DELETE CASCADE`.
- **`data/friends.db`** (`modules/friends/migrations/`): `friendlist`, `ignorelist`, `private_chat`, `public_chat` — keyed by **username37 hashes**, with **no account table** to reference.

The split is the documented **DB-2 federation** decision (PORTING.md Arc 18, `modules/friends/db.go:21-35`): the friends service was deliberately decoupled from the login/account store, accepting orphan rows and giving up TS's account-existence checks (exception blocks `NAI-S4A-D-FED-NO-ACCOUNT-EXISTENCE-CHECK` in `modules/friends/handler.go` / `repository.go`). This design **reverses DB-2**: the new model makes those exceptions' structural rationale disappear, so per the true-to-TS fidelity gate the corresponding TS behaviors must be restored.

### What TS 274 actually does

Reference: `LostCityRS/Engine-TS` branch `274-GOSCAPE` @ `dee467c8` (local canonical checkout).

- **One database, selectable backend.** `src/db/query.ts` picks a Kysely dialect from `Environment.db.backend`: `sqlite` (node:sqlite, `db.sqlite`) or `mysql` (mysql2 pool). Both the login flow and the FriendServer issue their own queries against this single DB.
- **No FK constraints anywhere.** `prisma/singleworld/schema.prisma` and `prisma/multiworld/schema.prisma` contain **zero `@relation` declarations**. TS's single-DB benefit is *joinability*, not declared integrity. goscape's login.db already goes beyond TS with FK+CASCADE — this design extends that posture to the friends tables.
- **Friends persistence is account-id-keyed; wire/in-memory state is username37-keyed.** `FriendServerRepository.ts` keeps `playersByWorld`/`playerFriends`/`playerIgnores` keyed by username37/username (goscape's in-memory `Repository` already matches), but every persistence call resolves usernames to account rows:
  - `loadFriends` — double `INNER JOIN account` on `friendlist`, `orderBy f.created asc`.
  - `addFriend` (`FriendServerRepository.ts:204+`) — resolves **both** owner and target accounts; either missing → no insert.
  - `addIgnore` — resolves the **owner only** (missing → no-op); the target is stored as a **raw username string** (`ignorelist.value`) with **no existence check**: TS deliberately lets you ignore usernames that don't exist.
  - `deleteFriend` (`FriendServerRepository.ts:197-200`) — deletes via `IN (SELECT id FROM account WHERE username = ?)` subqueries.
  - Private messages (`FriendServer.ts:270-285`) — both endpoints resolved via `executeTakeFirstOrThrow`; missing account → message dropped (no insert, no delivery).
  - `public_chat` (schema.prisma:131-139) — `{id, session_uuid, timestamp, coord, message}`. No profile/world columns: TS recovers those by joining `session`. goscape's extra `profile`/`world` columns exist *only* because the federated DB couldn't do that join.
- Per-backend conflict handling: TS branches `onConflict.doNothing()` (sqlite) vs `onDuplicateKeyUpdate` (mysql). Go is simpler: `INSERT … ON CONFLICT DO NOTHING` is valid in **both** SQLite and PostgreSQL.

### The architectural model (user-supplied)

The original game plausibly ran a standalone, **central account database** that the login servers, the website's account management, and the friend server each connected to directly — credentials, account validity, friend-target verification all resolved by independent clients of one DB. This design adopts that model literally: the database is *infrastructure*, not a module's private storage. `login` and `friends` each open their **own pool**, even co-resident in one process. Future consumers (a website, hiscores viewers, the private sibling's tooling — the `hiscore` and dormant landing tables are exactly such consumers' surface) connect the same way with zero changes here.

One consequence stated plainly: with the **Postgres** backend, "independent clients" works across hosts (a true network central DB). With **SQLite**, the central database is a central *file* — all connecting services must share a filesystem. SQLite = single-host central DB; Postgres = network central DB.

## Decisions (from brainstorm Q&A)

1. **Schema shape:** re-key friends tables to TS 274 shape **and** declare FK constraints with `ON DELETE CASCADE` (goscape extension, consistent with login.db precedent). Exceptions where TS behavior forbids an FK are documented below.
2. **Branch scope:** rev-274 only.
3. **Existing data:** **clean break.** No merge/migration tooling. New unified DB starts empty at a new default path; old `login.db`/`friends.db` files are left untouched on disk.
4. **Sequencing:** one spec, two implementation phases — Phase 1 consolidation (SQLite), Phase 2 PostgreSQL.
5. **Deployment scope:** full — examples and Helm updated for the unified config, and Helm gains Postgres backend support (external DB, secret-based credentials).

## Goals

- One central database (default `data/goscape.db` SQLite; any `postgres://` DSN) holding all persistent server state except player save files.
- `login` and `friends` as independent DB clients: own pools, no shared handles, friend-target verification = the friends service's own `SELECT` against `account`.
- Friends tables re-keyed to TS 274 shape; all DB-2 behavioral exceptions retired and TS behaviors restored, pinned by tests.
- FK + `ON DELETE CASCADE` integrity on all account-id references (goscape extension over TS).
- Backend selection `sqlite | postgres` analogous to TS's `Environment.db.backend`.
- Clean-break config: old per-module DSN keys removed; strict decoding fails old configs loudly.

## Non-Goals

- Porting to rev-254/245.2/244/225 (may be decided later, separately).
- Data migration tooling from the old split DBs.
- MySQL support (TS has it; we choose Postgres instead — explicit user decision).
- Moving player save files into the DB (TS uses flat files too).
- An account RPC service (contradicts the direct-connection model; rejected in brainstorm).
- `auto_migrate` config toggle or `goscape-cli db migrate` provisioning command (YAGNI'd; future work if operators need DDL-less service users).
- Query-builder/codegen adoption (sqlc/squirrel) — overkill for ~25 hand-written statements.
- Website/account-management consumers themselves (the schema is their contract; building them is out of scope).

## Design

### 1. `pkg/gamedb` — the central-DB client library

New package. Every service that needs the DB uses it; it owns all dialect knowledge.

```go
type Config struct {
    Backend  string         // "sqlite" | "postgres"
    SQLite   SQLiteConfig   // DSN (default "data/goscape.db")
    Postgres PostgresConfig // DSN (default ""), MaxOpenConns (default 8)
}

func Open(cfg Config, logger *slog.Logger) (*DB, error)

type DB struct {
    *sql.DB
    // dialect (unexported) drives Rebind and pool posture
}

func (d *DB) Rebind(query string) string // "?" → "$N" on postgres; identity on sqlite
func (d *DB) Migrate(ctx context.Context) error
```

- **SQLite posture (unchanged from today):** modernc.org/sqlite, `SetMaxOpenConns(1)`, `PRAGMA journal_mode=WAL`, per-connection `_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)`, parent-dir creation. Lifted from the (deleted) `modules/{login,friends}/db.go`.
- **Postgres posture (phase 2):** `jackc/pgx/v5` via its `database/sql` stdlib adapter (`pgx/v5/stdlib`), `SetMaxOpenConns(cfg.Postgres.MaxOpenConns)`.
- **Rebind:** applied once per statement at construction time in the modules (the ~25 statements stay hand-written `database/sql`). Constraint documented + tested: no `?` characters inside SQL string literals.
- **Migrations:** embedded FS with `migrations/sqlite/` and `migrations/postgres/` directories. **Fresh `000001` lineage** — the old per-module lineages are retired with the clean break, which also lets the new DDL drop legacy artifacts (e.g. the `session.session_uuid = ''` allowance for pre-slice-7 rows). golang-migrate drivers: `sqlite` (existing), `postgres` (phase 2; takes a pg advisory lock during migration).
- **Timestamps contract:** per-dialect column types — `TEXT` ISO-8601 + `DEFAULT CURRENT_TIMESTAMP` (SQLite, today's posture) vs `timestamptz` + `DEFAULT now()` (Postgres). All Go-side reads/writes go through `time.Time` or stay inside SQL comparisons; never dialect-specific string formatting in module code. Scanning specifics (modernc TEXT↔`time.Time`) get pinned by the dialect-parameterized repository tests.

### 2. `database` module — migration anchor in the dskit graph

New module registered in `cmd/goscape/app/modules.go`:

```
database → common
login    → common, database
friends  → common, database
```

A `services.BasicService` whose `startingFn` runs `gamedb.Open` + `Migrate` to completion and closes that connection; `runFn` just waits for ctx. The graph's topological start order guarantees schema exists before `login`/`friends` accept work, in **every** target combination (`all`, `login`, `friends`). In split deployments each process migrates at its own boot: safe on Postgres (advisory lock), and on SQLite split processes are same-host/same-file where `busy_timeout` mediates. `world` and `ondemand` do not depend on it and never touch the DB.

### 3. Module rewiring

- `modules/login` and `modules/friends` each call `gamedb.Open` themselves during their `startingFn` (own pool; **no shared handles**, including under `target=all` — two SQLite write pools in one process are mediated by WAL + busy_timeout; both services' write rates are low). Their `db.go` open/migrate code is deleted; queries move to `db.Rebind(...)`-wrapped statements.
- `modules/login` queries are otherwise **unchanged** (its schema carries over as-is).
- `modules/friends` persistence re-keys to account-JOIN forms mirroring `FriendServerRepository.ts` function-by-function (see Behavior restorations). The in-memory `Repository` (username37-keyed online-player state) and the friends wire protocol are **untouched** — that is TS's own shape.
- `world`'s gRPC-to-login and friends-protocol clients are untouched: the user's model ("login servers … connect to the central DB") is preserved — world talks to services, services talk to the DB.

### 4. Unified schema

Login-side tables carry over **unchanged** (already TS-shaped with documented goscape extensions and FK+CASCADE): `account`, `account_login`, `session`, `login`, `ipban`, `hiscore`, `hiscore_large`, and the dormant landing tables (`message_thread`, `message`, `message_status`, `account_session`, `wealth_event` — deliberately FK-free, mirroring their Prisma-generated DDL; prior user decision stands).

Friends-side tables re-keyed to TS 274 shape, adopting TS column names (clean break):

| Table | Shape (TS 274 `schema.prisma`) | FKs (goscape extension) |
|---|---|---|
| `friendlist` | `profile, account_id, friend_account_id, created` — PK `(profile, account_id, friend_account_id)` | `account_id`, `friend_account_id` → `account(id)` ON DELETE CASCADE |
| `ignorelist` | `profile, account_id, value TEXT, created` — PK `(profile, account_id, value)` | `account_id` → `account(id)` CASCADE. **`value` gets no FK** — TS lets you ignore nonexistent usernames (`addIgnore` never checks the target) |
| `private_chat` | `id, account_id, profile, timestamp, coord, to_account_id, message` | `account_id`, `to_account_id` → `account(id)` CASCADE |
| `public_chat` | `id, session_uuid, timestamp, coord, message` | **none** — headless players emit uuids with no `session` row (world sends `p.sessionOrHeadless()`), and TS accepts those rows too. profile/world recovered by joining `session`, as TS does |

goscape's current `public_chat.profile`/`world` extension columns are **dropped** (their only justification was the federation's missing join). Supporting indexes are recreated against the new keys: a `friend_account_id`-side index (backs `GetFollowers`), private-chat to/from indexes, public-chat session/recent indexes.

### 5. Behavior restorations (retiring the DB-2 exceptions)

Consolidation removes the structural rationale for every documented DB-2 exception; the fidelity gate therefore requires restoring TS behavior. Each item is pinned by tests referencing the TS source:

1. **`addFriend`** resolves owner and target accounts; either missing → no insert (`FriendServerRepository.ts:204+`). Keeps the 100-cap and `ON CONFLICT DO NOTHING`.
2. **`addIgnore`** resolves the owner only (missing → no-op); target stored as raw username string, unchecked — per TS.
3. **`loadFriends`/`loadIgnores`/`deleteFriend`/`deleteIgnore`** become the account-JOIN / subquery forms TS runs (`loadFriends`' double `INNER JOIN account`, `orderBy f.created asc`).
4. **Private messages**: both endpoints resolved; either missing → message dropped, no insert, no delivery (`FriendServer.ts:270-285`). The `NAI-S4A-D-FED-NO-ACCOUNT-EXISTENCE-CHECK` exception blocks in `handler.go`/`repository.go` are removed.
5. **`LogPublicMessage`** inserts TS's exact row `{session_uuid, timestamp, coord, message}`.
6. The DB-2 federation doc-comment in `modules/friends/db.go` is deleted with the file; `docs/PORTING.md` records the DB-2 exceptions as **restored** (not excepted), with pointers back to this spec.

### 6. Config & deployment surface

New top-level config section (`cmd/goscape/app/config.go`), standard `RegisterFlagsAndApplyDefaults` + `Validate`:

```yaml
database:
  backend: sqlite            # sqlite | postgres
  sqlite:
    dsn: data/goscape.db
  postgres:
    dsn: ""                  # postgres://user:pass@host:5432/goscape?sslmode=disable
    max_open_conns: 8
```

- `login.sqlite_dsn` and `friends.sqlite_dsn` **removed**. Strict decoding makes any old config fail at boot with an unknown-key error naming the stale key — the clean break announcing itself.
- `Validate`: `backend` ∈ {sqlite, postgres}; `postgres.dsn` required when backend is postgres; `sqlite.dsn` required when backend is sqlite.
- `examples/bundled/goscape.yaml` and `examples/full-config-reference.yaml` updated (reference file documents every new key at its default).
- **Helm** (`production/helm/goscape`): new `goscape.database.*` values — `backend` (default `sqlite`, preserving current behavior), and for postgres: `host`, `port`, `database`, `user`, `existingSecret`/`secretKey` for the password. The DSN is rendered with a `$GOSCAPE_DB_PASSWORD` env-var reference (from the secret) and `--config.expand-env=true` added to container args, so the secret never lands in the ConfigMap. `SingleBinary`/`Management` keep the StatefulSet either way (player saves still need the PVC). NetworkPolicy gains a postgres egress rule when enabled. `World` mode untouched. Chart tests updated.

### 7. Error handling

- **Boot:** migration failure or unreachable DB → `database` module `Failed` → FailureWatcher tears the process down with the underlying error (fail-fast; orchestrator restarts). Schema *ahead* of the binary (rollback scenario) surfaces the same way via golang-migrate's version check.
- **FK-violation races:** existence-check→insert sequences can lose a race against account deletion; the resulting FK violation is handled as TS's "account missing" path — operation dropped, debug log, no propagation. (The only new error class FK enforcement introduces, and it maps onto behavior TS already has.)
- **Runtime connection loss (Postgres):** `database/sql` pools reconnect per-query; per-op failures keep today's posture (friends logs and continues; login returns the error → login denied).
- **SQLite contention:** `busy_timeout(5000)`, now also mediating the two in-process pools under `target=all`.

### 8. Testing

- Existing login/friends suites move to `gamedb.Open` with temp-file SQLite DSNs; they re-pin every re-keyed query against the new schema.
- **TS-parity pins** (dual-pinned — presence AND absence): addFriend with existing/missing target; addIgnore of a nonexistent username **succeeding**; PM dropped when either endpoint missing / delivered when both exist; `loadFriends` join ordering; `public_chat` exact row shape.
- **FK/CASCADE:** deleting an account cascades friendlist/ignorelist/private_chat rows; `foreign_keys(1)` actually enforced on every pooled connection.
- **Independent-clients integration test:** two separate `gamedb.Open` pools against one temp SQLite file — account created through one pool, resolved through the other; WAL contention smoke.
- **`Rebind` unit tests** (incl. the no-`?`-in-literals constraint as a documented test).
- **Config tests:** old keys fail loudly; validation messages.
- **Postgres (phase 2):** repository suites are dialect-parameterized and run against a real server only when `GOSCAPE_TEST_POSTGRES_DSN` is set; skipped otherwise. Honest limitation: postgres migration files are only exercised in that gated mode — default CI is SQLite-only.
- `-race` on touched packages (CGO_ENABLED=1 works on this box). Final smoke test is a user-launched server (bundled preset boots, login + friends flows work end-to-end).

## Implementation phases

- **Phase 1 — Consolidation (SQLite only):** `pkg/gamedb` (with the dialect seam and `Rebind` in place from day one, sqlite-only wiring), `database` module, unified `000001` migration set (sqlite), friends re-key + behavior restorations, config clean break, examples, Helm updated for the unified config (still sqlite), PORTING.md exception retirement. Fully shippable on its own.
- **Phase 2 — PostgreSQL:** pgx/v5 stdlib driver, `migrations/postgres/` DDL, backend selection + validation live, dialect-parameterized tests behind `GOSCAPE_TEST_POSTGRES_DSN`, Helm postgres values (secret-based DSN, expand-env, NetworkPolicy egress).

## Risks & mitigations

- **Two SQLite write pools in one process** (`target=all`): theoretical contention, mediated by WAL + busy_timeout; both services write sparsely (list mutations, chat logs, login events). Pinned by the integration test. If it ever bites in practice, the recorded fallback is raising `busy_timeout` or revisiting in-process handle sharing — but not before evidence.
- **modernc SQLite `time.Time` scanning** vs `TEXT` columns: exact behavior pinned early in phase 1 by repository tests before mass query migration.
- **golang-migrate on SQLite across split processes**: first-boot DDL races are possible only in the unusual "two processes, same file, simultaneous first boot" case; documented guidance is to stagger first boot. Postgres has no such caveat (advisory lock).
- **Behavior changes are deliberate**: friends operations now *reject* unknown targets (addFriend) and *drop* PMs to missing accounts where the federated design silently accepted them. These are restorations of TS behavior, not regressions; the dual-pin tests make the direction explicit.
