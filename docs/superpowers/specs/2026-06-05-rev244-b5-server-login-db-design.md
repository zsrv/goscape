# rev-244 B5 — server/login/db — design

**Date:** 2026-06-05
**Status:** Approved
**Branch:** rev-244 (umbrella: `2026-06-03-rev244-port-design.md` §B5; resume
context: `docs/superpowers/handoffs/2026-06-05-RESUME-rev244-port-b5.md`;
worker/multiworld evaluation (B5-early deliverable, shipped before B2):
`2026-06-04-rev244-worker-multiworld-eval.md`)

## Goal

Port the server/login/db slice of the 225→244 Engine-TS delta: the
login-server rate-limit replacement (3-in-5s same-account+IP + 45s hop
timer — the B3-opened protection gap), the real `messageCount` query, the
friends-server multi-profile conversion + `public_message` re-key, the
logger `report` seam re-key, the consumer-backed prisma→SQLite schema
deltas, and the formal NOT-PORTED rows for the worker files deferred from
the eval. All TS citations refer to the 244 pin `9aadcec4`.

## Scope slice (extraction command)

```
git -C ../Engine-TS diff e1dea19f..9aadcec4 -- \
  src/server/login src/server/friend src/server/logger src/server/worker prisma
```

(14 src files + 2 schema.prisma + migrations churn.) Plus the B3/B4 tracker
rows assigned here: login rate limiting absent everywhere (B3 row 1),
`world_heartbeat` (B3 row 2), logger/friends message shapes (B3 row 4),
`messageCount` real query (B3 row 5).

## Exploration findings that reshape the handoff brief

1. **`world_heartbeat` is dead-at-pin (#177).** The 244 consumer is
   `case 'world_heartbeat': break;` (LoginThread.ts:183-185) — the message
   World.savePlayers posts (World.ts:1269-1273) is discarded at the thread
   boundary and never reaches the login server. No LoginClient method
   exists. The handoff's "login gRPC proto surface" framing overstated it;
   the faithful closure is a decision row, not an RPC.
2. **`world_startup` profile drop is broken upstream.** 244 removed
   `profile` from the message (LoginClient.ts:20-26) but the LoginServer
   handler still filters the session reset by the now-undefined `profile`
   (LoginServer.ts:160-171) — the reset matches nothing at the pin.
3. **The hop timer forces the login-server-7 closure.** It reads per-profile
   `account_login.logged_out` (node id) + `logout_time`
   (LoginServer.ts:366-371); goscape has no `logged_out` column anywhere
   (the db.go "nothing reads it" note is stale at 244) and stamps
   `logout_time` per-account — exactly the documented
   `PORTING-EXCEPTION (login-server-7)` divergence.

## User decisions (recorded 2026-06-05)

1. **Schema scope: consumer-backed + logger landing sites.** Port tables a
   goscape code path reads or writes (`login`, `account_login` re-shape,
   `message_thread`/`message`/`message_status`, friends `public_chat`
   re-key) PLUS `account_session` and `wealth_event` as schema-only landing
   sites for the dormant logger seam (no Go writer in this public repo;
   documented as dormant). Website-only models — `newspost`, `tag`,
   `account_tag`, `message_tag`, `mod_action`, `input_report_event_raw`,
   and the `account` 2FA/email/oauth/notes/password_updated columns — get
   formal NOT-PORTED rows (no goscape consumer; goscape has no website).
2. **login-server-7 closes fully, legacy column dropped.** All five steps
   of the closure plan written at db.go:234-258: migration adds
   `account_login.logged_out` + `logout_time` with backfill from
   `account.logout_time`; `setLoggedOut` stamps per-profile; the M25
   safety reject reads per-profile; `account.logout_time` is dropped (the
   re-create is already a CREATE-COPY-DROP-RENAME dance).
3. **Friends: per-message profile, multi-profile server.** `profile` is
   added to the friends RPC request messages (TS-shaped); the WorldConnect
   profile-mismatch reject is removed; repositories and registries re-key
   by (profile, world).
4. **`world_heartbeat` + `WorldStartupRequest.profile` doc-close.**
   world_heartbeat → NO-OP decision row (dead-at-pin consumer; producer
   not modeled). `WorldStartupRequest.profile` is KEPT with a
   PORTING-EXCEPTION row — dropping it would replicate an upstream
   regression for zero fidelity gain (rev244-b3-ws-origin precedent).

## Design

### 1. Login schema — migration 000005 (lands first; everything else reads it)

One migration file, five shapes:

- **`login` attempts table** (TS prisma `login` model): `uuid TEXT PRIMARY
  KEY`, `account_id INTEGER NOT NULL REFERENCES account(id) ON DELETE
  CASCADE`, `world INTEGER NOT NULL`, `timestamp TEXT NOT NULL`,
  `uid INTEGER NOT NULL`, `ip TEXT` + an index on `(account_id, ip,
  timestamp)` backing the 5s window scan. goscape's per-attempt
  `sessionUUID` stands in for TS's socket uuid as the PK — same
  one-row-per-attempt cardinality (TS LoginServer.ts:255-265 inserts
  `uuid: socket`).
- **`account_login` re-create** adding `logged_out INTEGER NOT NULL
  DEFAULT 0` and `logout_time TEXT` (NULL allowed), preserving the
  existing PK/FK/cascade shape; backfill `logout_time` from
  `account.logout_time` for every existing row (logged_out backfills 0 —
  the node id was never recorded; hop-timer treats 0 as "no block",
  matching TS's `logged_out !== 0` gate).
- **`account` re-create** dropping `logout_time` (login-server-7 step v).
- **`message_thread`, `message`, `message_status`** mirroring the 244
  prisma models field-for-field (the three tables the unread query
  touches; `message_tag` is website-only → NOT-PORTED).
- **`account_session`, `wealth_event`** mirroring the 244 prisma models —
  schema-only dormant landing sites (user decision 1); no Go reader or
  writer; in-code comment + decision row record the posture.

### 2. Login handler — rate limit, hop timer, messageCount

`PlayerLogin` (modules/login/handler.go) gains three TS-faithful blocks,
in TS 244's order:

- **3-in-5s rate limit** (TS LoginServer.ts:234-269): after account
  resolution including the auto-register path, BEFORE the password
  compare — `if account != nil`: count `login` rows for (account_id, ip)
  with `timestamp >= now-5s`; 3 → return `LOGIN_RESULT_RATE_LIMITED`;
  otherwise insert the attempt row (uuid = this attempt's sessionUUID).
  Note the TS posture ports as-is: the attempt insert and limit apply
  only when the account exists, and rejected-rate-limited attempts do NOT
  insert a row (TS returns before the insert).
- **45s hop timer** (TS LoginServer.ts:366-379): a new else-if arm on the
  logged-in chain (goscape handler step 7) — fires when NOT reconnect and
  NOT already-logged-in: `staffModLevel < 2 && logged_out != 0 &&
  logged_out != nodeId && logout_time != NULL && logout_time >= now-45s`
  → return `LOGIN_RESULT_HOP_TIMER`. Reads the new per-profile columns
  via the extended `accountByUsername` LEFT JOIN.
- **`getUnreadMessageCount`** (TS Messages.ts:1-37, new file): SQL port of
  the Kysely query — threads where the account is from/to participant,
  INNER JOIN a last-non-deleted-message-per-thread subquery, LEFT JOIN
  `message_status` for this account, kept when status.deleted/read is
  NULL or older than the last message, excluding threads where
  `last_message_from = accountId`; returns COUNT. Wired into
  `buildLoginResponse` on BOTH the full-login and reconnect paths (TS
  calls it at LoginServer.ts:322 and :395). Empty tables → 0: identical
  observable to today's stub until rows exist.

`setLoggedOut` (db.go) gains a `nodeID` parameter (sourced from
`PlayerLogoutRequest.node_id`, already in the proto) and stamps
`account_login.logged_out = nodeID, logout_time = now` per
(account, profile) — TS LoginServer.ts:484-494. The M25 safety reject
(handler.go:232) re-points to the per-profile `logout_time`.
`clearLoggedInFlag` (force-logout) stays logout_time-free (login-server-2
posture unchanged). The login-server-7 marker converts to closure notes.

### 3. Login proto/wire — two enum values, fixed bytes

`proto/login/login.proto` `LoginResult` gains:

- `LOGIN_RESULT_RATE_LIMITED = 10` — TS response 8 (3-in-5s).
- `LOGIN_RESULT_HOP_TIMER = 11` — TS response 6 (45s hop).

`loginResultToRS2` (modules/world/server.go:1291) maps them to the client
bytes the 244 World reply dispatch fixes (World.ts:1891-1906): response 6
→ byte **9** (`loginresp.OpIPLimit`, "Login limit exceeded"), response 8
→ byte **16** (`loginresp.OpTooManyAttempts`). The existing
`LOGIN_RESULT_LOGIN_IN_PROGRESS` → 16 mapping is unchanged (it mirrors
TS's world/login-server in-flight checks, which also respond 8).

`WorldStartupRequest.profile` is KEPT (user decision 4) — new
`PORTING-EXCEPTION (rev244-b5-startup-profile, …)` marker at the
WorldStartup handler citing the broken-at-pin upstream filter.

### 4. Friends — multi-profile server, per-message profile, public_chat re-key

- **Proto:** add `string profile` to the client→server request messages
  that carry it in TS 244 (PLAYER_LOGIN/LOGOUT, PLAYER_CHAT_SETMODE,
  FRIENDLIST_ADD/DEL, IGNORELIST_ADD/DEL, PRIVATE_MESSAGE, PUBLIC_CHAT_LOG,
  RELAY_* — the plan verifies the exact per-opcode set against
  FriendServer.ts/FriendClient with cat-n before fields are assigned;
  WorldConnect and Subscribe* already carry or gain it for registry
  keying). World-side `friends_client.go`/`bridges.go` populate it from
  the world's profile config.
- **Server:** WorldConnect profile-mismatch reject REMOVED (TS deleted it,
  FriendServer.ts:89-104 at 225 → gone at 244). `handler` holds a lazily
  initialized `map[profile]*Repository` (TS `repositories[profile]`,
  FriendServer.ts:435-447); subscriber + world-event registries re-key
  from `world` to `(profile, world)` (TS `socketByWorld[profile][world]`).
  Config keeps `NodeProfile` only as the world-side default it SENDS; the
  friends server no longer validates against it.
- **`public_chat` re-key** (TS FriendServer.ts:287-305 + prisma model):
  migration re-creates `public_chat` as `(id, profile, world, username,
  coord, message, created)`; `PublicMessageRequest.session_uuid` →
  `username` + `world_id` already present + new `profile`. TS resolves
  username → account_id against the shared `account` table; goscape's
  friends DB is username37-keyed by pre-existing design (no account
  table) → store the username directly; the account_id resolution gets a
  NO-LANDING-SITE decision row. World call site
  (modules/world/handlers_game.go:436) sends `p.username` instead of
  `p.session`.
- **FriendServerRepository internals diff** (orderBy API form change,
  addFriend select slimming, hardcoded 100-cap inline): expected NO-OP /
  N-A rows against goscape's diverged repository — verified hunk-by-hunk
  in the audit table, not assumed.

### 5. Logger seam (dormant) + worker rows

- `slogLoggerBridge.NotifyPlayerReport` re-keys per the 244 LoggerClient
  `report` shape (session_uuid → username, + world/profile; exact field
  set extracted from LoggerClient.ts at plan time with cat-n).
  `input_track` was already adapted in B3 (`2f67fed2`). `proto/events/v1`
  message shapes stay untouched — the private sibling owns them
  (telemetry-split posture); decision row records the deferral.
- `session_log` → `account_session` rename and the wealth-event reshape
  land ONLY as the dormant schema tables from user decision 1 + decision
  rows; the in-world telemetry structs (B4 known-residual TRADE
  recipient_items row) are NOT touched by B5.
- **Formal NOT-PORTED rows** (closes the eval's deferred-rows item):
  `src/util/WorkerFactory.ts`, `src/appWorker.ts`,
  `src/server/worker/WorkerServer.ts`,
  `src/server/worker/WorkerClientSocket.ts`, and the
  LoginThread/FriendThread/LoggerThread `STANDALONE_BUNDLE` branches —
  platform-inapplicable browser-bundle mode, architecture-mapped to dskit
  (worker-eval verdict; B3 NOT-PORTED taxonomy).
- **`world_heartbeat`** → NO-OP decision row (exploration finding 1):
  dead-at-pin consumer, producer not modeled, no proto change.
- **`LoginClient` `remaining` drop** → already-aligned row (goscape never
  carried the field; eval §2).

## Error handling

Rate-limit and hop-timer rejections are normal `PlayerLoginResponse`
results, not gRPC errors (matching every existing reject path). DB
failures keep the existing `codes.Internal` posture. The attempt-row
insert sits OUTSIDE the existing session/login-row transaction (TS
inserts it unconditionally before auth outcome is known); its failure is
a real error (`codes.Internal`), matching TS's thrown-await.

## Testing

TDD per slice (RED→GREEN pins):

- **Rate limit:** 2 attempts pass / 3rd in 5s rejected / window expiry
  readmits / unknown-account attempts bypass (no insert, no limit) /
  attempt-row shape (uuid/account_id/world/timestamp/uid/ip) / rejected
  attempt does NOT insert a 4th row.
- **Hop timer:** fires inside 45s from another node; bypasses on
  staffModLevel ≥ 2 (supermod tier exists since B3 T18), logged_out = 0,
  same-node logged_out, logout_time NULL, > 45s.
- **Wire mapping:** RATE_LIMITED → 16, HOP_TIMER → 9 byte pins
  (world-side `loginResultToRS2` test).
- **Migration:** backfill lands existing `account.logout_time` per-profile;
  M25 reject still arms post-migration; `account.logout_time` gone.
- **messageCount:** fixture matrix over the unread semantics — unread
  thread counted; read-after-last-message not counted; deleted-status
  older than last message counted; `last_message_from = self` excluded;
  deleted messages excluded from last-message; empty tables → 0; wired on
  both login and reconnect paths.
- **Friends:** same world id under two profiles fully isolated
  (repositories + registries); mismatch reject gone; per-message profile
  threading; public_chat insert shape (profile/world/username); world-side
  PublicMessage sends username.
- **Logger:** NotifyPlayerReport field-shape pin.

Gates: `CGO_ENABLED=0 go build -trimpath ./...`, `go vet ./...`
(pre-existing `pkg/util/build` only), full `go test ./... -count=1`,
`-race` (CGO_ENABLED=1) on modules/login + modules/friends +
modules/world (B4 lesson: world tests exercise the PublicMessage bridge
and login reply-byte contracts — run the world suite in any task touching
them). Proto regeneration via the repo's existing protoc tooling; the
generated-code diff rides the same commit as its .proto change.

## Risks

1. **`account` table re-create** touches the most-referenced table in the
   login DB (FKs from account_login, session, hiscore, login). SQLite
   table-copy migrations run with foreign_keys OFF inside the migration
   tx (000003 precedent documents this); the migration test asserts
   post-migration FK integrity (`PRAGMA foreign_key_check`).
2. **Friends proto re-key** is a wide mechanical change across handler,
   repository, subscriptions, world bridges, and test fakes —
   NEW-INTERFACE-METHOD-COMPILE-CASCADE applies; budget the cascade into
   the slice.
3. **Kysely→SQL translation** of the unread query is the only
   non-mechanical logic port; the fixture matrix above is the contract.

## Process

B1-B4 cadence: this spec (commit) → writing-plans (bite-sized TDD; exact
TS extraction commands as contracts; every citation cat-n-verified before
writing) → subagent-driven execution (implementer (sonnet) → TS-parity
spec reviewer → quality reviewer per substantive task; controller-direct
for leaf tasks) → full-suite gate + PORTING.md §B5 correspondence audit +
final whole-bundle integration review.
