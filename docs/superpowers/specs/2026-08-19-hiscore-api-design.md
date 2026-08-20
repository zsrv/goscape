# Hiscores Web API — Design

**Date:** 2026-08-19
**Status:** Approved (brainstorm complete)
**Branch:** rev-274 (backport to rev-254 / rev-245.2 / rev-244 / rev-225 as a follow-up)

## Problem

goscape writes hiscore data but nothing can read it back.

The write path shipped in Arc 29 (2026-06-01): on logout, `modules/login`
exports a player's enabled-stat XP and levels into the central-database
`hiscore` (per-stat) and `hiscore_large` (total) tables, mirroring TS
`LoginServer.updateHiscores`. Engine-TS has **no** serving endpoint — `hiscore`
appears in TS source only in `db/types.ts`, the Prisma migrations, and
`LoginServer.ts` — so goscape faithfully built none either.

That leaves the data write-only. There is no way for an eventual goscape
website to display leaderboards, and no way for an authenticated third party
to query them.

## Decisions (settled during brainstorm)

1. **Scope:** a hiscores-only module (`modules/hiscore`), not a general public
   web API. Smallest surface; a broader API module can be added beside it later.
2. **Auth split:** Kong OSS owns authentication and rate limiting; the module is
   a pure, anonymous-safe read API. Rationale: hiscore data is *public* — the
   website serves it to anonymous visitors anyway — so authentication here buys
   quota and accountability, not confidentiality. Kong's `key-auth` +
   per-consumer `rate-limiting` deliver exactly that with zero Go code, and
   building a key-management subsystem inside goscape to duplicate it is
   unjustified. Third parties are onboarded as `KongConsumer` objects.
3. **Format:** JSON only, versioned under `/v1`. No Jagex-style CSV
   compatibility endpoint — nothing legacy needs it, and a second contract with
   no version field is a permanent liability. Revisit only if a third party asks.
4. **Ranks:** computed on read in SQL against new indexes. No materialized
   snapshot, no in-memory index, no scheduler, no staleness to explain. Kong's
   `proxy-cache` plus our `ETag` absorbs repeat traffic. A snapshot is a
   drop-in later change if it is ever needed.
5. **Deployment artifacts:** the Helm chart renders Kong Ingress Controller
   resources (Ingress annotations, `KongPlugin`, `KongConsumer`), all off by
   default. Kong itself is a **prerequisite**, not a dependency of this chart.
6. **Module internals:** standalone module built on `pkg/dskit/server` (as
   `ondemand` does), not raw `net/http` (as `account` does). dskit's server
   already provides `LogSourceIPsHeader`/`LogSourceIPsRegex` — the exact
   "real client IP behind a proxy" problem Kong introduces — plus request
   logging, the full timeout set, graceful shutdown, and `DisableSignalHandling`.
7. **Branch scope:** rev-274 first, then backport across the fleet, following
   the account module's precedent.

### Rejected alternatives

- **Routes on the existing `ondemand` server.** `ondemand` depends on `World`,
  not `Database`; the public API would only exist while the game server runs,
  and a cache-delivery surface for game clients would share a listener with a
  public JSON API. Same blast-radius argument that ruled out `modules/account`.
- **gRPC only, transcoded by Kong's `grpc-gateway`.** Needs a generated
  protoset shipped to and kept in sync with Kong; makes `ETag`/`Cache-Control`
  — the biggest performance lever here — awkward; leaves the API
  undebuggable with `curl` unless Kong is running.
- **Module-owned API keys.** Ties keys to portal accounts with no Admin API
  coupling, but re-implements `key-auth` and makes per-consumer quotas
  goscape's problem. Door left open: if self-service key issuance from the
  portal is ever wanted, the portal calls Kong's Admin API (decision 2 above
  is what makes that a later, additive change).

## Data semantics

These are properties of the **existing** write path. The API contract is
downstream of them, and getting any of them wrong is a silent correctness bug.

- **XP is stored as fixed-point tenths (`×10`).** `hiscore.value` and
  `hiscore_large.value` are the raw save values, which are XP × 10
  (`pkg/objtype/playerstat.go` — `levelExperience` thresholds are `×10` so
  increments can be fractional). The API exposes **whole XP** (`value / 10`);
  the `×10` representation never leaves the module.
- **Per-stat rows exist only at base level ≥ 15.** `updateHiscores` skips any
  stat below level 15, so a player's low skills have *no row*. This is sparse
  data, not zero data.
- **The overall row counts every enabled stat regardless of the level gate.**
  `hiscore_large` type 0 holds total XP and total level summed across all
  enabled stats, so overall exists for every exported player.
- **`type` encoding:** `0` = overall (in `hiscore_large`); `stat + 1` = per-stat
  (in `hiscore`). 19 stats are enabled; indices 18 and 19 are unused 2004-era
  reserved slots (`objtype.PlayerStatEnabled`).
- **`level` is base level** derived from XP (1–99), not current/boosted level.
- **`date` is the last time `value` changed**, not the last logout — the upsert
  carries `WHERE value <> excluded.value`.
- **Staff and actively-banned accounts are skipped at write time**
  (`staff_mod_level > 1`; `banned_until >= now`).
- **`account.username` is the base37 "safe name"** — lowercase, underscores for
  spaces, ≤ 12 chars (`pkg/util/jstring.ToSafeName`).

## Architecture

New module `modules/hiscore`, registered in `cmd/goscape/app/modules.go`:

```
hiscore   HTTP read API over the hiscore tables   → common, database
```

Added to the `SingleBinary` (`all`) target and selectable standalone as
`--target=hiscore`. It owns a private `gamedb` pool, following the
independent-clients model that `login`, `friends`, and `account` already use.

| File | Responsibility |
|---|---|
| `config.go` | `Config`, `RegisterFlagsAndApplyDefaults`, `Validate`; embeds `server.Config` |
| `hiscore.go` | module struct, `New`, dskit `BasicService` wiring |
| `skills.go` | `type` ↔ skill-name mapping derived from `objtype` |
| `store.go` | every SQL query, `gamedb.Rebind`'d for SQLite and Postgres |
| `handlers.go` | routing, validation, JSON encoding, caps, cache headers |
| `cursor.go` | opaque cursor encode/decode |
| `gateway.go` | consumer-identity observation, backstop limiter |

The store exposes exactly four operations — `LookupAccountByName`,
`PlayerCard`, `LeaderboardByOffset`, `LeaderboardByCursor` — and nothing above
it writes SQL.

`skills.go` derives the skill enum from `objtype.PlayerStatNames` and
`objtype.PlayerStatEnabled` rather than restating it, so the API and the write
path cannot drift.

## API contract

All endpoints are `GET`, under `/v1`.

### `GET /v1/skills`

Static metadata: the 19 enabled skills with their hiscore `type` id and
canonical name, so a client need not hardcode the enum. Changes only with a
build; cached hard.

### `GET /v1/players/{name}`

One player's full card.

`{name}` is normalized through `jstring.ToSafeName`, so `Zezima`, `zezima`, and
`Ze zima` resolve identically; the response echoes the `ToDisplayName` form.

Returns `overall` plus an array of **all 19 enabled skills**. Skills the player
has no row for — i.e. below the level-15 export threshold — return
`"ranked": false` with `rank`, `level`, and `xp` null, rather than being
omitted, so consumers can render a fixed table without special-casing.

`404` when the account does not exist, has never been exported, or is filtered
by the visibility rules below.

### `GET /v1/leaderboards/{skill}`

One paginated ranking. `{skill}` is `overall` (reads `hiscore_large`) or any
enabled skill name (reads `hiscore`).

| Param | Default | Bound |
|---|---|---|
| `limit` | 25 | hard max 100 |
| `offset` | 0 | `offset + limit` ≤ `leaderboard_max_rank` (default 500 000) |
| `cursor` | — | mutually exclusive with `offset` |
| `profile` | config default | must be non-empty |

Out-of-range values are `400`, never silently clamped.

### Cross-cutting

- **Whole XP only.** `xp` is `value / 10`. Pinned by an explicit test.
- **`rank` is 1-based and unique.** See ranking semantics below.
- **Errors** use one shape, `{"error": {"code", "message"}}`, with codes
  `not_found`, `invalid_request`, `rate_limited`, `internal`. No stack traces,
  no SQL, no internal identifiers.
- **Caching on every response:** `Cache-Control: public, max-age=<configurable,
  default 60>`, a strong `ETag`, and `304` on conditional requests.
  `Last-Modified` is set from the newest `date` in the result where the
  response has one — `/v1/skills` is build-static and carries `ETag` only.
  This is what makes `proxy-cache` effective and is the single largest lever
  on DB load.

## Ranking and visibility

**Ordering is `value DESC, date ASC, account_id ASC`.** The `date` tiebreak
means that at equal XP, whoever reached it first ranks higher — the
conventional behaviour. Because the ordering is total, **every rank is unique**,
so there is no dense-vs-competition ranking ambiguity to document or test.

**Rank for a player card** is `1 + COUNT(rows strictly ahead)`, expressed as one
correlated subquery so a full card is a single round trip rather than 20. The
subquery **must apply the same visibility filter as the leaderboard** — a rank
that counted hidden rows would disagree with the rank the leaderboard reports
for the same player:

```sql
SELECT h.type, h.level, h.value, h.date,
       1 + (SELECT COUNT(*) FROM hiscore r
             JOIN account ra ON ra.id = r.account_id
             WHERE r.profile = h.profile AND r.type = h.type
               AND ra.staff_mod_level <= 1
               AND (ra.banned_until IS NULL OR ra.banned_until < ?)
               AND (r.value > h.value
                 OR (r.value = h.value
                     AND (r.date < h.date
                       OR (r.date = h.date AND r.account_id < h.account_id)))))
FROM hiscore h
WHERE h.profile = ? AND h.account_id = ?
```

Rank agreement between the player card and the leaderboard is a test, not an
assumption (see Testing).

**Rank on a leaderboard page** is `offset + index + 1` for offset paging, since
the page is already in rank order; for cursor paging it is carried in the
cursor (below).

**Indexes** — new migration, both tables:

- `(profile, type, value DESC, date ASC, account_id ASC)` serves the ordering
  and the rank count.
- `(profile, account_id)` serves the player card. The existing PK is
  `(profile, type, account_id)`, which cannot serve a lookup that does not know
  `type`.

Adding indexes to TS-mirrored tables is a goscape extension with no behavioural
change, documented the same way the existing FK posture is.

**Visibility filter.** The write path skips staff and actively-banned accounts,
but it only runs on logout, so an account banned *after* its last logout keeps
stale rows. The API therefore also filters at read time, joining `account` to
exclude `staff_mod_level > 1` and `banned_until >= now`, so a ban takes effect
on the hiscores immediately rather than at the offender's next logout.

The cost is that the rank `COUNT` is no longer a pure index scan — it becomes a
nested loop against `account`. That is the right trade at this scale
(correctness over micro-optimisation). If it ever bites, the fix is a
materialized visibility column, not a redesign.

## Pagination

Two modes on the leaderboard endpoint, because two callers need different
things.

**Offset paging**, capped at a configurable 500 000-rank window, exists for the
website's "jump to page N", which genuinely requires random access. The cap is a
*product* boundary matching what the hiscores have historically displayed, not a
performance fudge, and it lives in config as `leaderboard_max_rank`.

**Cursor paging** is O(limit) at any depth: `WHERE (value, date, account_id) <
(…)` is a keyset seek the same index serves directly, with no skipping. It
exists because `OFFSET` is O(offset) in every SQL engine, so a third party
walking the whole board 25 rows at a time issues ~20 000 requests whose offsets
sum to ~5 billion row-skips — a full-board scrape is quadratic under offset
paging and linear under cursor paging. Bulk readers are documented as expected
to use cursors, and Kong's per-consumer rate limiting makes that stick.

Because ordering is unique, the cursor carries the next row's rank, so
cursor-paged responses report **true absolute ranks** rather than degrading to
position-within-page. If a player's XP changes mid-walk, a carried rank can
drift by a row or two; this is documented and not solved.

Cursors are base64url over a compact typed struct (`value`, `date`,
`account_id`, `rank`). They are deliberately **unsigned**: no privilege attaches
to one, so a tampered cursor can only produce a wrong page for its own sender.
Malformed cursors are `400`, never a panic.

## Gateway contract

The module treats Kong as an **untrusted upstream**. This is what keeps it
correct standalone — in dev, in the single-binary target, and under `go test`.

- **`X-Consumer-Username` / `X-Consumer-ID` / `X-Anonymous-Consumer` are read
  for logging and metrics only.** Nothing is gated on them, so forging them
  buys an attacker nothing but a wrong log line. They are additionally ignored
  outright unless `trust_gateway_headers` is set — **default false**, enabled
  in the Helm values where Kong actually fronts the service.
- **Client IP** comes from dskit's `LogSourceIPsHeader`/`LogSourceIPsRegex`,
  used for log lines and for keying the backstop limiter — never for
  authorization.
- **The backstop limiter** is a coarse in-process limit for the case where the
  module is reached without Kong. It is not the quota system; Kong's
  per-consumer `rate-limiting` is.

Kong-side plugin set (all Kong OSS; only `rate-limiting-advanced` is
Enterprise and it is not needed):

| Plugin | Purpose |
|---|---|
| `key-auth` | third-party keys, with an **anonymous consumer** so the website and casual users work unauthenticated |
| `rate-limiting` | a low anonymous tier and a higher per-consumer tier |
| `proxy-cache` | honours our `Cache-Control` |
| `cors` | scoped to the website origin |

## Helm artifacts

Behind `hiscore.enabled` and `hiscore.kong.createGatewayConfig`, both off by
default so nobody who does not run Kong is affected. The flag is named to make
it unmistakable that it renders *configuration*, not Kong itself.

Rendered: a Service, an Ingress annotated with `konghq.com/plugins`, and
`KongPlugin` objects for the four plugins. `KongConsumer` objects render from a
values list where each entry **names an existing Secret** — key material never
lands in `values.yaml`.

**Kong is a prerequisite, not a dependency.** A gateway is cluster-wide
infrastructure shared by every service behind it, not a per-application
component; bundling it would mean one Kong per goscape release, and two Kongs
if the cluster already runs one. The chart guards with
`.Capabilities.APIVersions.Has "configuration.konghq.com/v1"` and `fail`s at
install time with a message naming the prerequisite, so a missing controller
surfaces as an error rather than as silently inert resources.

The docs carry the `helm install` for Kong's own chart, a worked third-party
onboarding example, and a note that a Kong running DB-less is configured by
decK rather than these CRDs (the operator translates the rendered objects).

## Configuration

New `hiscore:` section, following the layered
defaults → file → env → flags precedence:

| Key | Default | Notes |
|---|---|---|
| `enable` | `false` | |
| `server.*` | dskit defaults | embedded `server.Config` (listen address/port, timeouts, source-IP logging) |
| `log_level` | inherit | per-module override |
| `profile` | `main` | default profile for queries |
| `cache_max_age` | `60s` | drives `Cache-Control` |
| `default_limit` | `25` | |
| `max_limit` | `100` | |
| `leaderboard_max_rank` | `500000` | offset-paging ceiling |
| `trust_gateway_headers` | `false` | read `X-Consumer-*` only when true |
| `backstop_rate` | `120` req/min per key | in-process limiter, gateway-absent safety net; keyed by consumer when trusted, else client IP. `0` disables |

Also updated: `cmd/goscape/app/config.go`, `examples/full-config-reference.yaml`
(every option at its default), and `examples/bundled/goscape.yaml`.

## Error handling

- Unknown skill name, bad `limit`/`offset`/`cursor`, `offset` beyond
  `leaderboard_max_rank`, both `offset` and `cursor` supplied → `400`
  `invalid_request`.
- Unknown / never-exported / filtered player → `404` `not_found`.
- Backstop limiter tripped → `429` `rate_limited` with `Retry-After`.
- DB error → `500` `internal`, logged with detail server-side, opaque to the
  caller.
- A DB outage must not panic the module; the service stays up and returns
  `500`s, matching how other modules treat their pools.

## Testing

TDD throughout, per the project workflow. Pinned specifically:

- **Rank correctness** in `store` tests over `gamedbtest`, both backends: ties
  broken by `date` then `account_id`; ranks unique and gapless.
- **Offset/cursor equivalence** — walking a board both ways yields identical
  rows and identical ranks. This is what keeps two paging modes honest.
- **Card/leaderboard rank agreement** — the rank a player card reports for a
  skill equals the rank the leaderboard reports for that same player, including
  when hidden (staff/banned) accounts sit above them.
- **The `×10` XP conversion**, explicitly, because it is the silent
  tenfold-error risk.
- **Visibility** — a player banned after their last logout disappears from both
  the board and the player card; staff never appear.
- **Unranked skills** return explicit nulls with `ranked: false`, not omitted.
- **Conditional requests** — stable `ETag` across identical requests, `304` on
  match.
- **Name normalization** — `Zezima` / `zezima` / `Ze zima` resolve identically.
- **Gateway headers** — ignored when `trust_gateway_headers` is false; never
  grant anything when true.
- Module lifecycle test mirroring `account_test.go`.
- `gamedbtest` schema-leak trap avoided: never `t.Context()` inside
  `t.Cleanup`.

Race detector on touched packages (`CGO_ENABLED=1 go test -race`).

## Fidelity notes

This subsystem is a **goscape extension with no Engine-TS counterpart**. Engine-TS
has no hiscore serving endpoint at any pinned revision, so this is outside the
fidelity ledger rather than an unclosed parity gap. `PORTING.md` records it as
such, so a future parity audit does not flag it as a divergence to close.

The write path is untouched by this work. The only central-database change is
additive indexes.

## Out of scope (v1)

- Any write or mutation surface. The API is read-only.
- Jagex-style CSV compatibility endpoint (decision 3; revisit on demand).
- Self-service API-key issuance from the portal (decision 2; additive later).
- Per-skill "players around my rank" / rank-neighbourhood queries.
- Historical XP tracking or deltas over time — the schema stores current values
  only, and adding history is its own project.
- Clan / group leaderboards.
- The goscape website itself. This ships the API it will consume.
