# rev-244 worker/multiworld evaluation (B5 deliverable, executed early)

**Date:** 2026-06-04
**Status:** Complete — unblocks B2 scope freeze
**Branch:** rev-244 (umbrella spec: `2026-06-03-rev244-port-design.md`; this is the
"written worker/multiworld evaluation" deliverable from B5, executed before B2 per
the umbrella's ordering rationale)

## Question

Did 244's worker architecture change the login/friends **wire**? If yes, that
surface belongs to B2's scope decisions; if no, B2 may freeze handler shapes.

## Verdict (B2-relevant)

**No.** The worker architecture is transport-only — it changes *how bytes move
between threads in one process*, never *what bytes mean*. Nothing in the worker
delta moves any surface into B2:

- The game-client byte stream is produced/consumed unchanged by the worker path
  (`WorkerClientSocket` is just another `ClientSocket` impl over `postMessage`).
- The login/friends/logger **internal** protocols did change at the field level,
  but goscape's internal wire is its own gRPC design (`proto/login/login.proto`,
  `proto/friends/friends.proto`) — those deltas map to **B5** proto/schema/
  behavior work, not B2 protocol work.
- Client-facing wire changes DO exist nearby (login handshake re-shape, in-engine
  OnDemand on the game port) but originate in `World.ts`/`TcpServer.ts`/`web.ts`
  — **B3 surface**, already scoped there; not worker-caused. See §Boundary notes.

**B2 may freeze handler shapes.**

## 1. What the worker architecture actually is

New files at `9aadcec4`: `src/util/WorkerFactory.ts` (+11), `src/appWorker.ts`
(+8), `src/server/worker/WorkerServer.ts` (+50),
`src/server/worker/WorkerClientSocket.ts` (+24); plus `STANDALONE_BUNDLE`
branches in `LoginThread.ts`/`FriendThread.ts`/`LoggerThread.ts` and ~10 other
files (`git grep -l STANDALONE_BUNDLE 9aadcec4 -- src`).

- `WorkerFactory.createWorker` picks Web `Worker` (when `STANDALONE_BUNDLE`)
  vs Node `worker_threads.Worker` (otherwise). TS WorkerFactory.ts:5-11.
- `appWorker.ts` runs `World.start()` inside a worker; the parent forwards
  client sockets as `{type: connection|data|close, id, data}` postMessages;
  `WorkerServer.start` demuxes them into `WorkerClientSocket`s
  (TS WorkerServer.ts:11-49).
- `STANDALONE_BUNDLE` is "bundler/webrtc browser mode" (Environment.ts:9-10):
  `fetch()` replaces `fs` for PEM/script loads (World.ts:103, PemUtil.ts:10,
  ScriptFile.ts:139), `self.onmessage` replaces `parentPort`, CrcTable and
  FontType/WordEnc/map loads are skipped at World start (World.ts:319-326),
  reload/rebuild cheats disabled (ClientCheatHandler.ts:149-151).

Purpose: run the entire stack inside a browser. The parent↔worker messages are
in-process structured clones — **not a network protocol**; there is nothing to
byte-match.

**goscape mapping (decision):** NOT-PORTED, architecture-mapped. goscape's
dskit module system already provides process composition (`--target
ondemand|world|login|friends|all`); a browser bundle of a Go server is an
inapplicable platform feature (the Go analog would be a WASM build — out of
scope, no user demand). Formal per-file decision rows land in the B5 audit
trail; nothing for B2.

## 2. Login internal protocol delta (world ↔ login server)

JSON over `InternalClient` WebSocket in TS; gRPC in goscape. Extraction:
`git -C ../Engine-TS diff e1dea19f..9aadcec4 -- src/server/login`.

| TS change (244 pin) | goscape impact | Bundle |
|---|---|---|
| `world_startup` drops `profile` (LoginClient.ts:20-26) | drop `WorldStartupRequest.profile` (reserve field 2) | B5 |
| `player_login` reply drops `remaining` (LoginClient.ts:55-67, index.d.ts) | **already aligned** — goscape never carried `remaining` | — |
| `messageCount` now real: unread website messages via new `Messages.ts` `getUnreadMessageCount` (Kysely over `message_thread`/`message_status`/`message`) wired at LoginServer.ts:322,395 | proto already has `message_count=7`; implement the query + message tables in the login SQLite schema, or record a deferral row | B5 |
| NEW per-attempt `login`-table insert + same-account+IP rate limit: 3 rows in 5s → `response: 8` (LoginServer.ts:234-269) | new `login` table + rate-limit in `modules/login` handler | B5 |
| NEW 45s hop timer: `staffmodlevel < 2 && logged_out ∉ {null,0,nodeId} && logout_time ≥ now−45s` → `response: 6` (LoginServer.ts:366-379) | hop-timer check in `modules/login` handler; needs `logged_out`/`logout_time` columns (goscape already persists logout_time — Arc-31 M25-27) | B5 |
| LoginThread message set toward World: unchanged (only the `STANDALONE_BUNDLE` transport wrapper, LoginThread.ts:12-39) | none | — |

Note: TS `response` values 6/8 flow through `LoginClient.playerLogin` into the
World login pipeline and ultimately onto the client login reply byte — mapping
them into goscape's `LoginResult` enum (which currently tops out at
`IP_BANNED=9` with different semantics for 6/8) is a B5 design point; the
client-visible reply byte is fixed by the 244 client, so B5 must map enum →
wire byte exactly as TS does.

## 3. Friends internal protocol delta (world ↔ friends server)

Extraction: `git -C ../Engine-TS diff e1dea19f..9aadcec4 -- src/server/friend`.

| TS change | goscape impact | Bundle |
|---|---|---|
| Multi-profile server: `repositories[profile]`, `socketByWorld[profile][world]`; profile-mismatch reject **removed**; every client→server message now carries `profile` (FriendServer.ts:61-104 + each opcode block; FriendClient sends at :563-720) | add `profile` to friends RPC requests (or hoist to per-connection state established by `WorldConnect`, which already carries `profile` — gRPC is connection-oriented, unlike the TS WS-reconnect-races that force per-message profile). Design decision for B5; both shapes faithful to observable behavior | B5 |
| `PUBLIC_CHAT_LOG`: `session_uuid` → `username`, adds `nodeId`; `public_chat` rows now keyed `account_id`+`profile`+`world` (FriendServer.ts:290-309, FriendThread.ts:109-115) | rename `PublicMessageRequest.session_uuid` → `username` (+ world/profile), rework `public_chat` insert to resolve account id | B5 |
| FriendThread→World message set otherwise unchanged | none | — |

## 4. Logger internal protocol delta (dormant seam)

Extraction: `git -C ../Engine-TS diff e1dea19f..9aadcec4 -- src/server/logger`.
`report`/`input_track` re-key `session_uuid` → `username` (+world/profile on
input_track); `buf: string` → structured `blobs: InputTrackingBlob[]`;
`session_log` table → `account_session`; wealth events reshaped
(LoggerClient.ts:48-86, LoggerServer.ts:24-80).

goscape impact: the logger is the dormant no-op seam in this public repo
(`modules/world/logger_bridge.go`, `proto/events/v1/*`); consuming these deltas
is the private sibling project's concern. For rev-244: keep seams compiling;
`InputTrackingBlob.ts` itself is B3 surface (umbrella B3 row). No B2 impact.

## 5. World-side login rate limiting REMOVED in 244 (flag for B3/B5)

`NODE_RATELIMIT_ADDRESS_LOGIN` / `NODE_RATELIMIT_DEVICE_LOGIN` and their
World.ts:2106-2171 (225) enforcement are **gone** at `9aadcec4` — superseded by
the login-server-side 3-in-5s limit + hop timer (§2). goscape carries the
225 behavior in `modules/world/login_ratelimit*.go` + `ttlcache` + config
fields.

- Removal belongs to **B3** (World.ts surface).
- Replacement lands in **B5**.
- Ordering note: if B3 ships before B5, there is a window with no login rate
  limiting anywhere — acceptable on a dev branch, but B3's plan must carry a
  tracker row pointing at B5 for the replacement (don't silently drop the
  protection).

## 6. Boundary notes for B2 (client-facing changes found adjacent — NOT worker-caused)

These live in B3 files but neighbor code B2 touches
(`modules/world/client.go` login state machine, `pkg/io/protocol/login`):

- **Connect-time 8-byte seed send removed** from `TcpServer.ts:21-26` /
  `web.ts` WS-open. The seed now goes out in the **opcode-14 response**: 8 zero
  bytes, then a `0` status byte, then the 8-byte seed (World.ts:2146-2156 at
  the pin). Opcodes 16/18 are 1-byte-length framed and carry a plaintext
  revision byte checked against `ENGINE_REVISION` → reply `6` on mismatch
  (World.ts:2157-2162).
- **In-engine OnDemand on the game port**: `client.state !== 0` routes to
  `OnDemand.onClientData` (TcpServer.ts:30-37); WS gets it only under
  `NODE_WS_ONDEMAND` (web.ts:165-173).
- `web.ts` serves cache pages from `OnDemand.cache` reads + per-deployment
  token (`PemUtil` — primitive already ported in B1 as `pkg/util/pemtoken`).

**Recommendation:** keep all of the above in B3 (where the umbrella already
scopes World.ts/OnDemand.ts). B2 stays: game-packet opcode renumber
(`ClientGameProt`/`ServerGameProt`), the interaction-handler family, and the
rsbuf re-audit. The end-to-end 244-client login smoke is gated "after B2+B3"
(umbrella §Testing), which tolerates the handshake landing in B3. If B2's
handler work and B3's handshake work collide in `modules/world/client.go`,
resolve at B2 plan time with an explicit pull-forward decision row (B1
precedent: the clientinterface writer pull-forward).

## 7. B5 inventory established by this evaluation

For the eventual B5 spec (beyond the §2-4 wire deltas): the
singleworld/multiworld prisma schemas diverged heavily across the pins
(287 ± lines in `prisma/singleworld/schema.prisma` alone): new `login`,
`account_session` (replaces `session_log`), `wealth_event` (replaces
`session_wealth`), `tag`/`account_tag`, `newspost`, `message_thread`/`message`/
`message_tag`/`message_status`, `mod_action`, `input_report_event_raw` models;
plus `LoginServer` now imports `startManagementWeb` (management web moves into
the login process). Map onto goscape's SQLite migrations in
`modules/login/migrations` + `modules/friends/migrations` at B5 spec time.
