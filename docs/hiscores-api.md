# Hiscores API

`modules/hiscore` serves a public, read-only JSON API over player hiscore
data. It is a **goscape extension**: Engine-TS has no hiscore serving
endpoint at any pinned revision — `hiscore` appears there only in
`db/types.ts`, the Prisma migrations, and the write path in
`LoginServer.ts`. This module is therefore outside the TS fidelity ledger.
See `docs/PORTING.md` and spec `docs/superpowers/specs/2026-08-19-hiscore-api-design.md`.

## 1. What it is

The hiscore/hiscore_large central-database tables are written by
`modules/login` on every player logout (`modules/login/hiscore.go`,
`updateHiscores`). This module never writes; it only reads those tables
and renders them as JSON.

Visibility is filtered **twice**:

- **At write time** — `modules/login`'s `updateHiscores` skips accounts
  with `staff_mod_level > 1` and accounts whose `banned_until` is still
  in the future. A staff or currently-banned account's stats are simply
  never exported on logout.
- **At read time** — every query in `modules/hiscore/store.go` re-applies
  the same predicate (`staff_mod_level <= 1 AND (banned_until IS NULL OR
  banned_until < now)`).

The read-time filter is deliberate, not redundant: it is what makes a ban
take effect on the boards **immediately**, rather than waiting for the
banned player's next logout to re-export (which, for an already-logged-out
account, might never happen). An account hidden by either rule reports
identically to an account that has never played — the API does not
distinguish "banned" from "unknown" in its responses.

## 2. Enabling it

Add a `hiscore:` block to the config file and set `enable: true`:

```yaml
hiscore:
  enable: true
  profile: main
  http_listen_address: 127.0.0.1
  http_listen_port: 8082
```

See `examples/full-config-reference.yaml` (search `hiscore:`) for every
key at its default, and `examples/bundled/goscape.yaml` for the minimal
form used by the bundled all-in-one server.

The module needs the **central database** — the same `database:` block
used by `login`, `friends`, and `account`. In the module dependency graph
(`cmd/goscape/app/modules.go`), `hiscore` depends on `database`, which is
the migration anchor that brings the schema up to date (including
migration `000004_hiscore_indexes`, described in
[Pagination](#6-pagination)) before any DB-using module starts.

Run it standalone with:

```bash
CGO_ENABLED=0 go run -trimpath ./cmd/goscape \
  --config.file examples/bundled/goscape.yaml \
  --target=hiscore
```

`--target=hiscore` starts only the `database` anchor and the `hiscore`
module — no world, login, friends, or account server. It can also run as
part of `--target=all` (the default), alongside every other module.

## 3. Endpoint reference

All three endpoints are `GET`-only and served under `/v1`. Field names
below are copied verbatim from `modules/hiscore/handlers.go` — do not
paraphrase them when integrating.

### `GET /v1/skills`

Lists every board the API serves.

```bash
curl http://localhost:8082/v1/skills
```

```json
{
  "skills": [
    { "type": 1, "name": "attack" },
    { "type": 2, "name": "defence" },
    { "type": 3, "name": "strength" },
    { "type": 4, "name": "hitpoints" },
    { "type": 5, "name": "ranged" },
    { "type": 6, "name": "prayer" },
    { "type": 7, "name": "magic" },
    { "type": 8, "name": "cooking" },
    { "type": 9, "name": "woodcutting" },
    { "type": 10, "name": "fletching" },
    { "type": 11, "name": "fishing" },
    { "type": 12, "name": "firemaking" },
    { "type": 13, "name": "crafting" },
    { "type": 14, "name": "smithing" },
    { "type": 15, "name": "mining" },
    { "type": 16, "name": "herblore" },
    { "type": 17, "name": "agility" },
    { "type": 18, "name": "thieving" },
    { "type": 21, "name": "runecraft" }
  ]
}
```

This endpoint lists only the 19 enabled per-stat boards; `overall` (type
0) is **not** among them — `initSkills` (`modules/hiscore/skills.go`)
deliberately keeps it out of `skillList` (pinned by
`TestSkills_ExcludesOverall`), since it is an aggregate over every stat
rather than a stat itself. It is what `/v1/players/{name}` returns as
`overall`, and it is still a valid `{skill}` path value for
`/v1/leaderboards/{skill}` (see below) — a client wanting an overall
leaderboard must know the name `overall` out of band, because
enumerating `/v1/skills` can never surface it.

Every listed `type` is the underlying stat index **+ 1** (e.g. `attack`
is stat index 0, board type 1). The values are not contiguous: `type`
19 and 20 do not appear — they belong to `stat18`/`stat19`, two
2004-era reserved slots that Engine-TS keeps for index parity but never
enables, so goscape's write path never exports them and this endpoint
never lists them. `runecraft` (stat index 20) is board type 21. Select
a leaderboard by `name`, not by `type`.

### `GET /v1/players/{name}`

A player's card: overall standing plus one row per enabled skill.

```bash
curl http://localhost:8082/v1/players/Zezima
```

```json
{
  "name": "Zezima",
  "profile": "main",
  "overall": {
    "type": 0,
    "name": "overall",
    "ranked": true,
    "rank": 1,
    "level": 1247,
    "xp": 41346969,
    "updated_at": "2026-08-18T22:14:03Z"
  },
  "skills": [
    {
      "type": 1,
      "name": "attack",
      "ranked": true,
      "rank": 5321,
      "level": 99,
      "xp": 13034431,
      "updated_at": "2026-08-18T22:14:03Z"
    },
    {
      "type": 21,
      "name": "runecraft",
      "ranked": false,
      "rank": null,
      "level": null,
      "xp": null,
      "updated_at": null
    }
  ]
}
```

(Only 2 of the 19 `skills` entries are shown above for brevity; a real
response always lists all 19, in stat order — see
[Sparse skills](#5-sparse-skills) for why `runecraft` here is `ranked:
false`.)

`profile` accepts an optional `?profile=` query parameter, defaulting to
the module's configured `profile`. A name with no visible standing —
whether unknown, or hidden by the ban/staff filter — returns:

```bash
curl -i http://localhost:8082/v1/players/NoSuchPlayer
```

```json
{ "error": { "code": "not_found", "message": "player not found" } }
```
(HTTP 404.)

### `GET /v1/leaderboards/{skill}`

One page of a board, `{skill}` being any board `name` accepted by
`SkillByName` — the 19 names listed by `/v1/skills` plus `overall`,
which `/v1/skills` deliberately omits (see the note under
`GET /v1/skills` above for why).

```bash
curl 'http://localhost:8082/v1/leaderboards/attack?limit=3'
```

```json
{
  "skill": "attack",
  "profile": "main",
  "entries": [
    {
      "rank": 1,
      "name": "Zezima",
      "level": 99,
      "xp": 20000000,
      "updated_at": "2026-08-18T22:14:03Z"
    },
    {
      "rank": 2,
      "name": "Woox",
      "level": 99,
      "xp": 19850000,
      "updated_at": "2026-08-17T09:02:11Z"
    },
    {
      "rank": 3,
      "name": "Suomi",
      "level": 99,
      "xp": 18420000,
      "updated_at": "2026-08-16T14:55:47Z"
    }
  ],
  "next_cursor": "eyJ2IjoxODQyMDAwMDAsImQiOiIyMDI2LTA4LTE2VDE0OjU1OjQ3WiIsImEiOjQyLCJyIjo0fQ"
}
```

`next_cursor` is only populated when the page returned a full `limit`
rows (i.e. more may follow); it is empty at the end of the board. See
[Pagination](#6-pagination) for `offset` vs `cursor`.

Unknown skill names and out-of-range parameters are rejected with `400`
and the same error envelope shown above (`code: "invalid_request"`), not
silently clamped.

Skill names in `{skill}` are matched exactly, lower-case, against
`SkillByName` — unlike `{name}` on `/v1/players/{name}`, which is
normalized (base37 safe-name round trip; see `GET /v1/players/{name}`
above). `/v1/leaderboards/Attack` is therefore a `400`, not a
case-folded match to `attack`. This is a deliberate API contract, not a
bug: case-sensitive skill selectors are reasonable, they are just
asymmetric with the player-name path.

## 4. XP units

**The API always returns whole XP. The central database stores XP as
fixed-point tenths (×10).** `wholeXP` in `handlers.go` is the single
place this conversion happens (`valueX10 / 10`) — every `xp` field in
every response above has already been divided by 10. If you are cross-
checking against a raw database read of the `hiscore` / `hiscore_large`
tables' `value` column, or against a save file, remember to divide by 10
first, or you will be off by a factor of ten.

## 5. Sparse skills

`/v1/players/{name}` always lists all 19 enabled skills, in stat order,
so a consumer can render a fixed table without special-casing absence.
But not every skill has real data: `modules/login`'s write path only
exports a per-stat row once the skill's **base level reaches 15**
(`modules/login/hiscore.go`). A skill below that threshold has never been
written to the `hiscore` table for that account, so the card is
genuinely sparse.

A missing skill is represented as:

```json
{ "type": 21, "name": "runecraft", "ranked": false, "rank": null, "level": null, "xp": null, "updated_at": null }
```

`ranked: false` and all four value fields `null` together mean "this
player has never crossed the level-15 export threshold for this skill" —
not zero XP, not an error.

## 6. Pagination

`/v1/leaderboards/{skill}` supports two paging modes, mutually exclusive
(`400` if both `offset` and `cursor` are supplied):

- **`offset`** — zero-based, for random access ("jump to page N"):
  `?offset=200&limit=25`.
- **`cursor`** — an opaque token from a previous response's
  `next_cursor`, for bulk sequential reads: `?cursor=<token>&limit=25`.

**Bulk readers must use cursors, not incrementing offset.** `OFFSET` is
`O(offset)` per SQL query (the database still has to walk and discard
every earlier row), so paging through an entire board by repeatedly
incrementing `offset` is quadratic overall. Cursor paging seeks directly
from the last row's `(value, date, account_id)` position and is linear.

Offset paging is also bounded: `offset + limit` must not exceed
`leaderboard_max_rank` (config key `hiscore.leaderboard_max_rank`,
default `500000` — the deepest rank the hiscores display reaches by
offset). This is a product boundary, not a safety valve you can raise
your way around for bulk reads; going past it returns `400`:

```json
{
  "error": {
    "code": "invalid_request",
    "message": "offset+limit must not exceed 500000; use cursor paging for deep reads"
  }
}
```

Cursor paging has no such ceiling. Out-of-range `limit`/`offset` values
(e.g. `limit` outside `[1, max_limit]`, negative `offset`) are rejected
with `400` the same way — never silently clamped to the nearest valid
value.

(The additive index migration `000004_hiscore_indexes` — both `sqlite`
and `postgres` variants under `pkg/gamedb/migrations/` — is what makes
both paging modes an index range scan rather than a sort; it changes no
existing table shape and touches no write-path behavior.)

## 7. Caching

Every successful response carries:

- `ETag` — a hash of the response body.
- `Cache-Control: public, max-age=N` (`N` = `hiscore.cache_max_age`,
  default `60s`).
- `Last-Modified`, on `/v1/players/{name}` and `/v1/leaderboards/{skill}`
  only (the most recent `updated_at` among the rows returned) —
  `/v1/skills` omits it, since its content is build-static.

Send back `If-None-Match` with a previously-seen `ETag` to revalidate:

```bash
curl -i http://localhost:8082/v1/players/Zezima \
  -H 'If-None-Match: "kf3AbCdEfGhIjKlMnOpQrS"'
```

A match returns `304 Not Modified` with an empty body. Clients — and any
caching proxy in front of the module, such as Kong's `proxy-cache`
plugin (see [§9](#9-deploying-behind-kong)) — should honour these
headers rather than polling on a fixed interval.

**A `304` still counts against the in-process backstop limiter.**
Revalidation is cheap for the database, but it is still a request that
passes through `guard` before the 304 is produced, so it still consumes
one slot in the caller's rate-limit window.

Error responses are the opposite: they are always sent with
`Cache-Control: no-store`, since an error is per-caller and must never
be cached at a shared edge.

## 8. GET-only

The API serves **`GET` only**. Every route is registered as
`mux.HandleFunc("GET /v1/...", ...)` (Go 1.22+ method-specific
`http.ServeMux` patterns), plus a catch-all `mux.HandleFunc("/", ...)`
in `register` (`modules/hiscore/handlers.go`) that matches every method
on every otherwise-unmatched path. `http.ServeMux` only synthesizes its
own `405 Method Not Allowed` when a request path matches no registered
pattern *at all*; here it always matches something — either a specific
`GET`-only route or the method-agnostic catch-all — so that synthesized
405 never fires. A non-`GET` request instead falls through to the
catch-all and gets the module's ordinary `not_found` envelope:

```bash
curl -i -X POST http://localhost:8082/v1/players/Zezima
```

```
HTTP/1.1 404 Not Found
Content-Type: application/json; charset=utf-8
Cache-Control: no-store

{"error":{"code":"not_found","message":"no such resource"}}
```

There is no `Allow` header — the catch-all does not know the path was
otherwise valid, only that the method didn't match a registered `GET`
route for it. If you are writing a client, this is good news rather
than a gotcha: every response, `GET` or not, matched route or not,
carries the same `{"error": {...}}` shape. There is no transport-level
exception to special-case.

## 9. Deploying behind Kong

The chart in `production/helm/goscape` renders Kong **configuration
only** — `KongPlugin` / `KongConsumer` / `Ingress` objects under
`production/helm/goscape/templates/hiscore-kong.yaml`. It deliberately
does **not** install Kong itself: Kong is a prerequisite, not a
dependency of this chart. Install the Kong Ingress Controller from its
own chart first (consult Kong's own documentation for the exact values
for your cluster/version; a typical install looks like):

```bash
helm repo add kong https://charts.konghq.com
helm repo update
helm install kong kong/ingress -n kong --create-namespace
```

Once Kong's CRDs (`configuration.konghq.com/v1`) are present in the
cluster, enable goscape's rendered configuration in `values.yaml`:

```yaml
hiscoreGateway:
  createGatewayConfig: true
  host: hiscores.example.com
  rateLimit:
    anonymousPerMinute: 60   # unauthenticated / anonymousConsumer callers
    consumerPerMinute: 600   # any registered consumer (partners), key-authed
```

`createGatewayConfig: true` without the CRDs installed, or without
`hiscoreGateway.host` set, fails the Helm render with an explicit
`fail`, rather than deploying half-wired routing.

### Worked example: onboarding a third-party consumer

1. Create the credential as a Secret — key material never lives in
   `values.yaml`:

   ```bash
   kubectl create secret generic hiscore-partner-a-key \
     --from-literal=kongCredType=key-auth \
     --from-literal=key="$(openssl rand -hex 32)"
   ```

2. Register the consumer in `values.yaml`:

   ```yaml
   hiscoreGateway:
     consumers:
       - username: partner-a
         credentialSecret: hiscore-partner-a-key
   ```

3. `helm upgrade` to apply, then call the API with the issued key. The
   request is now keyed by Kong consumer (`consumerPerMinute`, 600/min
   above) instead of the anonymous per-IP tier (`anonymousPerMinute`,
   60/min):

   ```bash
   curl -H 'apikey: <the key generated in step 1>' \
     https://hiscores.example.com/v1/players/Zezima
   ```

## 10. DB-less Kong

A Kong instance running in **DB-less mode** is not configured by these
`KongPlugin`/`KongConsumer`/`Ingress` CRDs at all — DB-less Kong reads
its entire configuration from a declarative file, typically managed with
[decK](https://docs.konghq.com/deck/). The objects this chart renders
translate directly: each `KongPlugin` becomes a `plugins:` entry, each
`KongConsumer` (plus its credential Secret) becomes a `consumers:` entry
with a `keyauth_credentials:` block, and the `Ingress` becomes a
`services:`/`routes:` pair. `helm template` this chart to get the
rendered YAML, then hand-translate it into your decK config — the CRDs
themselves are inert against a DB-less Kong.

## 11. Security posture

The module is **anonymous-safe**: every one of the three endpoints
serves data that is, by design, public (or nothing, once the visibility
filter hides an account). Nothing in this module is ever authorized by
gateway-supplied headers. `X-Consumer-Username` / `X-Anonymous-Consumer`
(read only when `hiscore.trust_gateway_headers: true`) select **a log
line and a rate-limit bucket** — never a permission, never a data scope.
The consumer name, whether the caller was anonymous, and its resolved
client IP are attached (via `callerAttrs`, `modules/hiscore/gateway.go`)
to the two diagnostically useful log lines: the rate-limit rejection in
`guard` and the internal-error path in `internal`. There is
deliberately no per-request access log at `info` level — dskit's server
already logs every request, and this endpoint is meant to be cacheable
and high-volume.

Two caveats surfaced during review, both about the rate limiter rather
than data exposure:

- **`trust_gateway_headers` without a real gateway in front.** If this
  is enabled but the module is directly reachable (no Kong, or a
  misconfigured one, actually setting these headers), any caller can
  set its own `X-Consumer-Username` on each request and mint a fresh
  rate-limit bucket every time — silently defeating the in-process
  backstop limiter. No data or access is put at risk (nothing is
  authorized by the header), but the limiter simply stops limiting, with
  no error or signal that it happened. **Leave `trust_gateway_headers`
  false unless a gateway genuinely fronts this module** and strips/sets
  those headers itself.

- **`limit_by: ip` depends on Kong's own client-IP resolution.** The
  anonymous per-IP rate tier (`hiscoreGateway.rateLimit.anonymousPerMinute`,
  wired to Kong's `rate-limiting` plugin with `limit_by: ip` — see
  `hiscore-kong.yaml`) only gives each real caller its own bucket if Kong
  itself is configured to trust the correct upstream hop for the client
  IP (Kong's own `real_ip_header` / `trusted_ips` settings — nginx-level
  Kong configuration this chart does not and cannot set, since it only
  renders `KongPlugin`/`KongConsumer`/`Ingress` objects, not Kong's proxy
  configuration). Deployed behind another load balancer or CDN without
  that configured correctly, Kong may see every anonymous caller as the
  same upstream address, collapsing them onto one shared bucket. This is
  a **different** setting from goscape's own `trust_gateway_headers`
  above — fixing one does not fix the other.
