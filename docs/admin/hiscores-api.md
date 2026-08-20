# Hiscores API

`hiscore` is a public, read-only JSON API over player hiscore data. It is a
**goscape extension** with no equivalent in upstream Engine-TS, which never
serves hiscores over HTTP — only writes them during login. This page covers
enabling the module, its endpoints, and the operational details that matter
for running it behind a gateway.

## What it is

Hiscore data is written by the `login` module on every player logout. This
module never writes; it only reads the central database's hiscore tables and
renders them as JSON.

Visibility is filtered **twice**:

- **At write time** — `login` skips accounts with a staff level above 1 and
  accounts that are currently banned. A staff or currently-banned account's
  stats are never exported on logout.
- **At read time** — every query re-applies the same predicate (not staff,
  not currently banned).

The read-time filter is deliberate, not redundant: it is what makes a ban
take effect on the boards **immediately**, rather than waiting for the
banned player's next logout to re-export — which, for an already-logged-out
account, might never happen. An account hidden by either rule reports
identically to an account that has never played; the API does not
distinguish "banned" from "unknown" in its responses.

## Enabling it

Add a `hiscore:` block to the config file and set `enable: true`:

```yaml
hiscore:
  enable: true
  profile: main
  http_listen_address: 127.0.0.1
  http_listen_port: 8082
```

See the [Config reference](config-reference.md) for every `hiscore.*` key at
its default. The module needs the **central database** — the same
`database:` block used by `login`, `friends`, and `account`. In the
[module dependency graph](index.md#module-dependency-graph), `hiscore`
depends on `database`, which brings the schema up to date (including the
index migration described in [Pagination](#pagination)) before any
database-using module starts.

Run it standalone with:

```bash
CGO_ENABLED=0 go run -trimpath ./cmd/goscape \
  --config.file examples/bundled/goscape.yaml \
  --target=hiscore
```

`--target=hiscore` starts only the `database` anchor and the `hiscore`
module — no world, login, friends, or account server. It also runs as part
of `--target=all` (the default), alongside every other module. See
[Choosing which modules run](configuration.md#choosing-which-modules-run-target)
for how `target` and each module's `enable` key combine.

## Endpoint reference

All three endpoints are `GET`-only and served under `/v1`.

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

This endpoint lists only the 19 enabled per-stat boards; `overall` (type 0)
is **not** among them — it is an aggregate over every stat rather than a
stat itself. It is what `/v1/players/{name}` returns as `overall`, and it
is still a valid `{skill}` path value for `/v1/leaderboards/{skill}` (see
below) — a client wanting an overall leaderboard must know the name
`overall` out of band, because enumerating `/v1/skills` can never surface
it.

Every listed `type` is the underlying stat index **+ 1** (e.g. `attack` is
stat index 0, board type 1). The values are not contiguous: `type` 19 and
20 do not appear — those two 2004-era reserved stat slots are never
enabled, so the write path never exports them and this endpoint never
lists them. `runecraft` (stat index 20) is board type 21. Select a
leaderboard by `name`, not by `type`.

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
[Sparse skills](#sparse-skills) for why `runecraft` here is `ranked:
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

One page of a board, `{skill}` being any of the 19 names listed by
`/v1/skills` plus `overall`, which `/v1/skills` deliberately omits (see the
note under `GET /v1/skills` above for why).

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

`next_cursor` is only populated when the page returned a full `limit` rows
(i.e. more may follow); it is empty at the end of the board. See
[Pagination](#pagination) for `offset` vs `cursor`.

!!! warning "Out-of-range parameters are rejected, never clamped"
    Unknown skill names and out-of-range parameters (an unrecognized
    `{skill}`, a `limit`/`offset` outside the valid range) are always
    rejected with `400` and the same error envelope shown above
    (`code: "invalid_request"`). The API never silently clamps a value to
    the nearest valid one.

Skill names in `{skill}` are matched exactly, lower-case — unlike `{name}`
on `/v1/players/{name}`, which is normalized. `/v1/leaderboards/Attack` is
therefore a `400`, not a case-folded match to `attack`. This is a
deliberate API contract, not a bug: case-sensitive skill selectors are
reasonable, they are just asymmetric with the player-name path.

## XP units

**The API always returns whole XP. The central database stores XP as
fixed-point tenths (×10).** Every `xp` field in every response above has
already been divided by 10. If you are cross-checking against a raw
database read of the hiscore tables, or against a save file, remember to
divide by 10 first, or you will be off by a factor of ten.

## Sparse skills

`/v1/players/{name}` always lists all 19 enabled skills, in stat order, so
a consumer can render a fixed table without special-casing absence. But not
every skill has real data: the write path only exports a per-stat row once
the skill's **base level reaches 15**. A skill below that threshold has
never been written to the hiscore table for that account, so the card is
genuinely sparse.

A missing skill is represented as:

```json
{ "type": 21, "name": "runecraft", "ranked": false, "rank": null, "level": null, "xp": null, "updated_at": null }
```

`ranked: false` and all four value fields `null` together mean "this
player has never crossed the level-15 export threshold for this skill" —
not zero XP, not an error.

## Pagination

`/v1/leaderboards/{skill}` supports two paging modes, mutually exclusive
(`400` if both `offset` and `cursor` are supplied):

- **`offset`** — zero-based, for random access ("jump to page N"):
  `?offset=200&limit=25`.
- **`cursor`** — an opaque token from a previous response's `next_cursor`,
  for bulk sequential reads: `?cursor=<token>&limit=25`.

!!! warning "Bulk readers must use cursors, not incrementing offset"
    `OFFSET` is **O(offset)** per SQL query — the database still has to
    walk and discard every earlier row — so paging through an entire board
    by repeatedly incrementing `offset` is **quadratic** overall. Cursor
    paging seeks directly from the last row's position and is **linear**.
    Any integration that walks a full board must use `cursor`, not
    `offset`.

Offset paging is also bounded: `offset + limit` must not exceed
`leaderboard_max_rank` (config key `hiscore.leaderboard_max_rank`, default
`500000` — the deepest rank the hiscores display reaches by offset). This
is a product boundary, not a safety valve you can raise your way around
for bulk reads; going past it returns `400`:

```json
{
  "error": {
    "code": "invalid_request",
    "message": "offset+limit must not exceed 500000; use cursor paging for deep reads"
  }
}
```

Cursor paging has no such ceiling. Out-of-range `limit`/`offset` values
(e.g. `limit` outside its valid range, negative `offset`) are rejected
with `400` the same way — never silently clamped to the nearest valid
value.

An additive index migration is what makes both paging modes an index range
scan rather than a sort; it changes no existing table shape and touches no
write-path behavior. It runs automatically as part of the `database`
module's startup migrations — see
[Back up the database before migrating](operations.md#upgrading-the-binary).

## Caching

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
caching proxy in front of the module, such as Kong's `proxy-cache` plugin
(see [Deploying behind Kong](#deploying-behind-kong)) — should honour
these headers rather than polling on a fixed interval.

!!! note "A 304 still counts against the rate limiter"
    Revalidation is cheap for the database, but it is still a request that
    passes through the module's in-process rate limiter before the `304`
    is produced, so it still consumes one slot in the caller's rate-limit
    window.

Error responses are the opposite: they are always sent with
`Cache-Control: no-store`, since an error is per-caller and must never be
cached at a shared edge.

## GET-only

The API serves **`GET` only**. A non-`GET` request does not get an HTTP
`405 Method Not Allowed` — it falls through to the same catch-all handler
that serves unmatched paths, and gets the module's ordinary `not_found`
JSON envelope instead:

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
route for it. If you are writing a client, this is good news rather than a
gotcha: every response, `GET` or not, matched route or not, carries the
same `{"error": {...}}` shape. There is no transport-level exception to
special-case.

## Deploying behind Kong

The Helm chart renders Kong **configuration only** — it deliberately does
**not** install Kong itself: Kong is a prerequisite, not a dependency of
the chart. Install the Kong Ingress Controller from its own chart first
(consult Kong's own documentation for the exact values for your
cluster/version; a typical install looks like):

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
`hiscoreGateway.host` set, fails the Helm render with an explicit `fail`,
rather than deploying half-wired routing. See
[Kubernetes with the Helm chart](deployment.md#4-kubernetes-with-the-helm-chart)
for the rest of the chart's deployment modes.

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

### DB-less Kong

A Kong instance running in **DB-less mode** is not configured by these
Kubernetes CRDs at all — DB-less Kong reads its entire configuration from
a declarative file, typically managed with
[decK](https://docs.konghq.com/deck/). The chart's rendered objects
translate directly: each plugin resource becomes a `plugins:` entry, each
consumer resource (plus its credential Secret) becomes a `consumers:`
entry with a `keyauth_credentials:` block, and the ingress resource
becomes a `services:`/`routes:` pair. `helm template` the chart to get the
rendered YAML, then hand-translate it into your decK config — the CRDs
themselves are inert against a DB-less Kong.

## Security posture

The module is **anonymous-safe**: every one of the three endpoints serves
data that is, by design, public (or nothing, once the visibility filter
hides an account). Nothing in this module is ever authorized by
gateway-supplied headers. `X-Consumer-Username` / `X-Anonymous-Consumer`
(read only when `hiscore.trust_gateway_headers: true`) select **a log line
and a rate-limit bucket** — never a permission, never a data scope. The
consumer name, whether the caller was anonymous, and its resolved client
IP are attached to the two diagnostically useful log lines: the
rate-limit rejection and the internal-error path. There is deliberately
no per-request access log at `info` level — the request already gets a
line from the shared HTTP server layer, and this endpoint is meant to be
cacheable and high-volume.

Two caveats surfaced during review, both about the rate limiter rather
than data exposure:

!!! warning "`trust_gateway_headers` without a real gateway in front"
    If this is enabled but the module is directly reachable (no Kong, or a
    misconfigured one, actually setting these headers), any caller can set
    its own `X-Consumer-Username` on each request and mint a fresh
    rate-limit bucket every time — silently defeating the in-process
    backstop limiter. No data or access is put at risk (nothing is
    authorized by the header), but the limiter simply stops limiting, with
    no error or signal that it happened. **Leave `trust_gateway_headers`
    false unless a gateway genuinely fronts this module** and strips/sets
    those headers itself.

!!! warning "`limit_by: ip` depends on Kong's own client-IP resolution"
    The anonymous per-IP rate tier (`hiscoreGateway.rateLimit.anonymousPerMinute`,
    wired to Kong's `rate-limiting` plugin with `limit_by: ip`) only gives
    each real caller its own bucket if Kong itself is configured to trust
    the correct upstream hop for the client IP — Kong's own
    `real_ip_header` / `trusted_ips` settings, which are nginx-level Kong
    configuration the chart does not and cannot set, since it only renders
    Kong CRDs, not Kong's proxy configuration. Deployed behind another
    load balancer or CDN without that configured correctly, Kong may see
    every anonymous caller as the same upstream address, collapsing them
    onto one shared bucket. **This is a different setting from goscape's
    own `trust_gateway_headers` above — fixing one does not fix the
    other.**
