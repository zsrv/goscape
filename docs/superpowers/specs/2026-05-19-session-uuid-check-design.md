# Design: schema-tightened `session.session_uuid` with CHECK constraint

**Status:** Approved (post-friends-arc cleanup item B3)
**Date:** 2026-05-19
**Predecessor:** [[friends-server-slice7-close]], [[post-friends-arc-cleanup-b-close]]

## Problem

The login server's `session` table is an append-only audit log keyed on `(id AUTOINCREMENT)` with a `session_uuid TEXT NOT NULL` column. Slice 7 of the friends-server bridge arc changed the value source from `req.Socket` (which held `c.conn.RemoteAddr().String()` — values like `127.0.0.1:42193`) to `uuid.NewString()`. Pre-slice-7 rows in any long-lived `data/login.db` therefore hold IP:port strings in a column intended to be a UUID, while post-slice-7 rows hold real UUIDs. Nothing in production code reads `session_uuid` back from this table today, so the schema drift is currently inert, but the column is wide open: any future regression that writes a non-UUID value would not surface until a consumer tried to parse it.

The same IP:port string that pre-slice-7 rows hold in `session_uuid` is independently stored in the `remote_address` column on the same row, so coercing the `session_uuid` value to empty for legacy rows loses no forensic information.

## Goal

Tighten the schema so that, going forward, `session.session_uuid` is enforced at the database level to be either a UUID (shape-checked, dash-separated 8-4-4-4-12) or the empty string. Coerce existing legacy rows in-place during the migration so they satisfy the new constraint without losing the rest of their audit data.

## Non-goals

- **No drop of legacy rows.** They retain `remote_address`, `account_id`, `login_time`, etc.; the only column touched is `session_uuid`.
- **No application code change.** `insertSession` in `modules/login/db.go` already writes UUIDs as of slice 7; the CHECK is purely defensive.
- **No friends-server schema change.** `public_chat.session_uuid` and `private_chat.session_uuid` came into existence at/after slice 7 and hold only real UUIDs; they are out of scope.
- **No strict UUID-v4 validation.** Shape-level (36 chars, dashes at positions 9/14/19/24, any character in non-dash positions) is sufficient to catch the actual drift (`127.0.0.1:42193`-style values) and avoids tying the schema to a specific UUID variant.
- **No `down.sql`.** The login server doesn't use down-migrations (`000001_init.up.sql` has none); we don't introduce them here.

## Architecture

Single forward-only migration file: `modules/login/migrations/000002_session_uuid_check.up.sql`.

SQLite cannot `ALTER COLUMN ADD CHECK` on an existing column, so the migration follows the standard SQLite table-rebuild pattern:

1. `CREATE TABLE session_new` with identical columns to `session`, plus a `CHECK` on `session_uuid`.
2. `INSERT INTO session_new SELECT (with inline CASE that coerces non-UUID-shaped values to '')` from `session`.
3. `DROP TABLE session`.
4. `ALTER TABLE session_new RENAME TO session`.

The entire migration runs inside an implicit golang-migrate transaction (the migrate tool wraps each file in `BEGIN`/`COMMIT` for SQLite). On failure, the transaction rolls back and `schema_migrations.dirty = 1` is set, surfacing the failure at the next `openDB` call.

### CHECK shape

```sql
CHECK (session_uuid = ''
   OR  session_uuid GLOB '????????-????-????-????-????????????')
```

- `GLOB` is built into SQLite (no extension required, unlike `REGEXP`).
- The `?` wildcards match any single character — strictly speaking this allows non-hex characters in UUID positions, but the only realistic source of drift was IP:port strings, which never match the dash-positioned pattern. The shape check is sufficient to catch the actual problem class.
- Empty string is permitted so the legacy-row coercion target satisfies the constraint.

### Migration file (full)

```sql
-- Tighten session.session_uuid: enforce UUID-shape-or-empty at the schema
-- level. Pre-slice-7 rows hold RemoteAddr().String() (e.g. "127.0.0.1:42193")
-- in this column; that same value lives in the separate remote_address
-- column, so coercing session_uuid to '' on legacy rows loses no audit data.
-- Going forward, insertSession (slice 7) only writes uuid.NewString() values
-- so the CHECK is defensive against future regressions.

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

## Data flow

On any boot of the login server where `schema_migrations.version < 2`:

1. `openDB` calls `migrateDB` (existing flow, `modules/login/db.go:36-74`).
2. golang-migrate's `iofs` source enumerates `migrations/*.sql`, finds `000002_session_uuid_check.up.sql`, opens an implicit transaction, runs the four statements above.
3. Pre-slice-7 rows: each row is copied with `session_uuid` rewritten to `''`; all other columns preserved including `id` (AUTOINCREMENT values are preserved through `INSERT INTO ... SELECT id, ...`).
4. Post-slice-7 rows: each row is copied unchanged (real UUIDs pass the GLOB in the CASE WHEN clause).
5. The original `session` table is dropped; `session_new` is renamed to `session`. The new table has the CHECK constraint attached.
6. `schema_migrations` writes `version = 2, dirty = 0`. Idempotent on subsequent boots (migrate skips already-applied versions).

For a fresh DB, `000001_init.up.sql` runs first (creating the unconstrained `session` table), then `000002_session_uuid_check.up.sql` rebuilds it with the CHECK. No legacy rows exist, so the CASE WHEN clause is exercised on zero input. Net effect: fresh DBs end up with the constrained schema directly.

## Application code

**No changes.** `insertSession` in `modules/login/db.go:158-168` already passes a `uuid.NewString()` value (from `handler.go` slice-7 code, see `45cd3e44`). The CHECK is purely defensive against future regressions.

## Tests

Three new tests in `modules/login/db_test.go` (existing test file; extends the `openDB`/`insertSession` test patterns established by slices 6 and 7).

### `TestSessionUUIDCheckRejectsNonUUID`

Open a fresh in-memory DB (golang-migrate applies both `000001` and `000002`). Issue a raw `db.Exec(INSERT INTO session ...)` with `session_uuid = "not-a-uuid"` and otherwise-valid columns. Assert the insert returns a non-nil error and that the error message contains `"CHECK"` (sqlite reports `constraint failed: CHECK constraint failed: ...`).

Pin: the constraint is actually wired and rejects values that aren't UUID-shaped.

### `TestSessionUUIDCheckAcceptsEmpty`

Same fresh-DB harness. Issue a raw `db.Exec(INSERT INTO session ...)` with `session_uuid = ""`. Assert it succeeds.

Pin: the empty-string carve-out (used by the legacy-row coercion) is permitted by the CHECK.

### `TestMigration002CoercesLegacyRows`

Open a fresh DB at schema version 1 only via `migrate.Steps(1)` (advances exactly one migration from the empty starting state). Insert an `account` row first (the `session.account_id` foreign key requires it), then insert a fake pre-slice-7 row directly into the unconstrained `session` table with `session_uuid = "127.0.0.1:42193"`, `remote_address = "127.0.0.1:42193"`, and a valid `login_time`. Run `migrate.Up()` to apply `000002`. Read the row back via `SELECT session_uuid, account_id, remote_address, login_time, id FROM session WHERE id = ?`. Assert:
- `session_uuid` is now `""`.
- `account_id`, `remote_address`, `login_time` are preserved unchanged.
- The same `id` value is preserved (AUTOINCREMENT pass-through).

Pin: the legacy-data coercion path actually transforms in-place without dropping rows.

### Test gating

All existing `modules/login/db_test.go` and `modules/login/handler_test.go` tests should pass unchanged — they all write UUID values through `insertSession`. Add the three new tests; do not modify existing ones.

## Rollout

Standard golang-migrate flow. On next boot of any `goscape` `login` module, `openDB` calls `migrateDB`, which detects `schema_migrations.version < 2`, applies `000002_session_uuid_check.up.sql` in a transaction, writes `version = 2`. Idempotent on re-run.

For local dev DBs that get wiped frequently: no observable effect (no legacy rows to coerce).
For any long-lived DB with pre-slice-7 audit history: legacy rows get cleaned up exactly once on first boot after this lands. Subsequent boots are no-ops.

No CI/deploy coordination needed. No application-code rollout dependency.

## Verification gates

- `go test -race ./modules/login/... -count=1` clean.
- `go test -race ./... -count=1 -timeout 600s` clean (full suite — confirm slice-6/slice-7 chat-audit tests still pass since they use real UUIDs and the friends DB is unaffected).
- `cmd/goscape-cli smoke-pack --content-dir LostCityRS/content` → 12 OK / 0 ERR / 0 SKIP (gates that login-module build still compiles into the full binary).

## Memory close

Single small commit + memory close memo:
- File: `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/post_friends_arc_cleanup_b3_close.md`.
- MEMORY.md entry inserted ABOVE the existing `post-friends-arc-cleanup-b-close` line.
- Retires the deferred-item note in the predecessor memo (`post_friends_arc_cleanup_b_close.md`) — B3 is no longer deferred.

## Risks

1. **golang-migrate transaction failure mid-rebuild leaves the DB in a partial state.** Mitigation: golang-migrate wraps each `.up.sql` in a transaction by default for SQLite; on failure the transaction rolls back and `dirty=1` is set on `schema_migrations`. The operator sees the failure at the next `openDB` call and can intervene.
2. **AUTOINCREMENT continuity.** SQLite's `sqlite_sequence` table tracks the max ID per AUTOINCREMENT table. The `DROP TABLE session` step removes the `sqlite_sequence` row for `session`; the `ALTER TABLE session_new RENAME TO session` step updates the surviving `session_new` row's `name` to `session`. Net effect: the AUTOINCREMENT counter for the renamed table reflects the max ID of the copied rows, so new inserts continue from `(max copied id) + 1`. The implementation plan should add an explicit assertion in `TestMigration002CoercesLegacyRows` that a post-migration `insertSession` call produces an ID strictly greater than the legacy row's ID — this catches any SQLite-version-specific surprise without requiring a separate sequence-targeted test.
3. **External tooling that reads `session_uuid` outside Go.** None known. The login server is the sole writer; nothing outside the login module reads. If an operator has ad-hoc queries assuming raw text in `session_uuid`, the CHECK constraint only affects future writes, not existing reads.

