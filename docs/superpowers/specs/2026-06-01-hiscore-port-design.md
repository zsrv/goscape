# Hiscore write-path port — design spec

**Date:** 2026-06-01
**Branch:** `fix/hiscore-port`
**Closes audit IDs:** `login-server-9`, `gap-db-datastruct-9` (2-row merged alias)
**Prior state:** EXCEPTION-DOCUMENTED (`fix/med-bundle-19`, `e48fcf6f`), STALE-DEFER in the
2026-05-28 fresh audit ledger.

## Problem

TS `LoginServer` exports a logged-out player's per-stat XP and levels into two
leaderboard tables on every graceful logout. goscape ports none of it: there are
no hiscore tables, no export call, and the `PlayerLogout` handler carries a
24-line `PORTING-EXCEPTION` comment documenting the deliberate omission.

The blocking dependency for this STALE-DEFER cluster was "the hiscore subsystem
does not exist". This spec ships that subsystem's **write path** — which is the
entirety of what TS does — thereby unblocking closure per the
shipped-dependency-unblocks-subsystem-blocked-STALE-DEFER pattern the CategoryType
port (`46d43c9d`) established.

## TS reference

- `Engine-TS/src/server/login/LoginServer.ts:19-109` — `updateHiscores(account, player, profile)`.
- `Engine-TS/src/server/login/LoginServer.ts:450` — the call site, invoked from
  inside the `player_logout` RPC branch immediately after the success response is
  sent and after `setLoggedOut`.
- `Engine-TS/prisma/singleworld/schema.prisma:47-69` — `model hiscore` (value `Int`)
  and `model hiscore_large` (value `BigInt`), each PK `@@id([profile, type, account_id])`.
- `Engine-TS/src/engine/entity/PlayerStat.ts:53` — `PlayerStatEnabled` (21-element
  bool array; indices 18/19 false).
- `Engine-TS/src/engine/entity/PlayerLoading.ts:93-94` — stats read as `g4s()`,
  base levels **derived** via `getLevelByExp(stats[i])` (stored level byte ignored).

### TS `updateHiscores` behaviour (the contract to mirror)

1. `if (!account) return;` — no account, no-op.
2. `if (account.staffmodlevel > 1) return;` — staff above mod-level-1 are excluded.
3. `if (account.banned_until !== null && new Date(account.banned_until) >= new Date()) return;`
   — actively-banned accounts excluded (ban end `>= now`).
4. Sum `totalXp += stats[i]` and `totalLevel += baseLevels[i]` over indices where
   `PlayerStatEnabled[i]` is true.
5. Upsert `hiscore_large` row `type = 0` with `(level = totalLevel, value = totalXp)`.
   TS: SELECT existing; if present and `value !== totalXp` → UPDATE (bumps `date`);
   if absent → INSERT; if present and equal → **no-op (date unchanged)**.
6. For each enabled stat with `baseLevels[stat] >= 15`: upsert `hiscore` row
   `type = stat + 1` with `(level = baseLevels[stat], value = stats[stat])`, same
   present/absent/equal logic.

Note: XP is stored ×10 fixed-point in **both** TS and goscape (`Player.ts:1754-1757`;
goscape `pkg/objtype/playerstat.go` `MaxXP` comment), so the summed XP is directly
comparable — no scaling conversion.

## Scope (decided)

**Write-path parity only.** TS has no hiscore *serving* endpoint — `hiscore`
appears in the TS source only in `db/types.ts` (table types) and `LoginServer.ts`
(this write path). Building a read/serve endpoint would exceed TS parity and has no
goscape consumer, so it is explicitly **out of scope** (YAGNI). This spec closes the
audit IDs by porting exactly what TS does and nothing more.

## Architecture

The login module is SQLite (`modernc.org/sqlite`), and already uses SQLite upsert
(`db.go:184` — `INSERT ... ON CONFLICT(account_id, profile) DO UPDATE SET ...`).
Five well-bounded units:

### Unit 1 — schema: `modules/login/migrations/000004_hiscore.up.sql`

```sql
CREATE TABLE hiscore (
    account_id INTEGER NOT NULL REFERENCES account(id),
    profile    TEXT    NOT NULL DEFAULT 'main',
    type       INTEGER NOT NULL,
    level      INTEGER NOT NULL,
    value      INTEGER NOT NULL,
    date       TEXT    NOT NULL,
    PRIMARY KEY (profile, type, account_id)
);

CREATE TABLE hiscore_large (
    account_id INTEGER NOT NULL REFERENCES account(id),
    profile    TEXT    NOT NULL DEFAULT 'main',
    type       INTEGER NOT NULL,
    level      INTEGER NOT NULL,
    value      INTEGER NOT NULL,
    date       TEXT    NOT NULL,
    PRIMARY KEY (profile, type, account_id)
);
```

- PK ordering `(profile, type, account_id)` matches prisma `@@id`.
- `hiscore.value` holds per-stat XP (≤ `MaxXP` = 2e9, fits prisma `Int`); SQLite
  `INTEGER` is 64-bit so `hiscore_large.value` (the BigInt total) needs no distinct
  column type — one schema serves both tables, matching the prisma shape where the
  *only* difference is the value's logical width.
- `date` is `TEXT` (ISO `dbTimeFormat = "2006-01-02 15:04:05"`), matching the
  module's other timestamps (`logout_time`, `login_time`).
- Up-only — the module has no `.down.sql` files; match that convention.

### Unit 2 — stat extraction: extend `modules/login/save.go`

```go
// saveStats extracts the 21 per-stat XP values from a SAV blob. Mirrors the
// stat block PlayerLoading reads (modules/world/player_load.go:151-156): right
// after playtime, 21 entries of (i32 XP + u8 current level); only the XP is
// needed here, base levels derive from it. Version-aware: playtime is i32 for
// v2+ and u16 for v1 (player_load.go:144-149).
func saveStats(save []byte) ([objtype.PlayerStatCount]int32, bool)
```

- Reuses the existing `save.go` header constants. Stat block offset =
  `24 + (version >= 2 ? 4 : 2)`; stride 5, reading the leading `i32` of each entry.
- Returns `(zero, false)` when the blob is too short to contain all 21 stats.
- Base levels are **not** read from the blob; callers derive them via
  `objtype.GetLevelByExp(xp)` exactly as `player_load.go:154` and TS
  `PlayerLoading.ts:94` do — the stored u8 level byte is ignored.

### Unit 3 — export logic: new file `modules/login/hiscore.go`

```go
// updateHiscores exports a logged-out player's enabled-stat XP/levels into the
// hiscore (per-stat) and hiscore_large (total) tables. Mirrors TS
// LoginServer.ts:19-109. Best-effort: callers log on error and do not fail the
// logout RPC (TS sends the success response before awaiting updateHiscores).
func updateHiscores(ctx context.Context, db *sql.DB, account *accountRow, save []byte, profile string) error
```

- **Gates (early-return nil, no-op):**
  - `account.StaffModLevel > 1`.
  - active ban: `account.BannedUntil.Valid` and the parsed time is `>= time.Now().UTC()`
    (i.e. `!now.After(t)`), matching TS `>= new Date()`.
  - `saveStats` returns `false` (blob too short) → log + no-op (defensive; a valid
    logout save always contains stats).
- Compute `totalXp` (int64) and `totalLevel` over `objtype.PlayerStatEnabled[i]`
  indices, base level via `objtype.GetLevelByExp(int(xp))`.
- One transaction (`db.BeginTx`), mirroring `setLoggedOut`'s tx + deferred-rollback
  shape:
  - Upsert `hiscore_large` `type=0` with `(totalLevel, totalXp)`.
  - For each enabled stat with `baseLevel >= 15`: upsert `hiscore` `type=stat+1`
    with `(baseLevel, xp)`.
- Each upsert is a single statement (`date` param = `time.Now().UTC().Format(dbTimeFormat)`):
  ```sql
  INSERT INTO <table> (account_id, profile, type, level, value, date)
  VALUES (?, ?, ?, ?, ?, ?)
  ON CONFLICT(profile, type, account_id)
  DO UPDATE SET level = excluded.level, value = excluded.value, date = excluded.date
  WHERE value <> excluded.value;
  ```
  The `WHERE value <> excluded.value` reproduces TS's **skip-when-unchanged**
  semantic (the `date` is bumped only when the value actually changed) in one
  statement — a behaviour-equivalent idiom improvement over TS's N+1
  select-then-insert/update. Final DB state is identical to TS.

### Unit 4 — wire-in: `modules/login/handler.go` `PlayerLogout`

Replace the 24-line `PORTING-EXCEPTION` comment block (`handler.go:304-325`) with the
real call, placed exactly where TS puts it — after `setLoggedOut`, mirroring
`LoginServer.ts:450`:

```go
if err := updateHiscores(ctx, h.db, account, req.Save, req.Profile); err != nil {
    h.log.Warn("updateHiscores failed; logout unaffected",
        slog.String("username", req.Username), slog.String("profile", req.Profile),
        slog.Any("err", err))
}
return &loginpb.PlayerLogoutResponse{Success: true}, nil
```

**Best-effort, matching TS ordering:** TS sends `{response: 0}` *before* `await
updateHiscores`, so a hiscore failure never reaches the client. goscape logs the
error and still returns `Success: true`. The export is fire-and-forget on the logout
path — consistent with the "fully contained, no logout/login-resume path reads
hiscores" framing the old exception block already established.

### Unit 5 — tests: `modules/login/hiscore_test.go`

Table-driven against an in-memory migrated SQLite DB (reuse the existing test DB
harness in `db_test.go` / `handler_test.go`):

- staffmodlevel > 1 → no rows written.
- actively-banned account → no rows written.
- happy path → `hiscore_large` `type=0` row with correct `(totalLevel, totalXp)`;
  per-stat `hiscore` rows only for stats with `baseLevel >= 15`, each `type=stat+1`.
- a sub-15 stat is **excluded** from `hiscore` but **still counted** in the
  `hiscore_large` total (the level-15 gate is per-stat-table only).
- re-run with a **changed** stat value → row updated (new value, `date` may change).
- re-run with **unchanged** values → row's `date` is byte-identical to the prior
  write (the conditional upsert suppressed the update) — pins the skip-when-equal
  semantic without needing an injected clock.

Augment one existing `PlayerLogout` happy-path test to assert hiscore rows land
end-to-end through the handler.

### Unit 6 — docs

- Remove the in-code `PORTING-EXCEPTION` comment (Unit 4 already does).
- Flip the `login-server-9 / gap-db-datastruct-9` row in `docs/PORTING-CLOSED.md`
  from **EXCEPTION-DOCUMENTED → FIXED**, citing this branch and spec, noting the
  scope decision (write-path parity; no serving endpoint, matching TS).
- Extend the relevant PORTING.md Arc line with the closure.

## Out of scope / non-goals

- No hiscore read/serve HTTP endpoint (no TS equivalent, no goscape consumer).
- No change to the world save format or `player_load.go`.
- No `PlayerLogoutRequest` proto change — the save blob is already carried; the
  stats are re-parsed in the login module exactly as TS re-loads the save.
- No multiworld/profile-routing changes beyond the `profile` column already threaded
  through `PlayerLogout`.

## Verification

- `go build ./...` clean.
- `go test ./modules/login/...` green (new `hiscore_test.go` + unchanged existing
  tests).
- `gofmt` clean on touched files.
- `#274` no-op-flip check: branch touches neither `deploy/bundled/goscape.yaml` nor
  `pkg/util/build/build.go` (md5 byte-identical pre/post-FF).
- `#289` parallel-main check before FF.

## Execution

Subagent-driven cadence (1 implementer + 1 spec reviewer + 1 quality reviewer per
task), per the PORTING.md "Subagent-driven cadence for dedicated-commit ports"
convention. Straight-FF onto main, sandbox-disabled retry for the primary-worktree
merge as in every prior arc FF.
