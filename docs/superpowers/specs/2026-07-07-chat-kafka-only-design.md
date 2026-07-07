# Chat goes Kafka-only: drop `public_chat`/`private_chat` from the central DB

**Date:** 2026-07-07
**Status:** Approved
**Repos:** `goscape` (all 5 rev branches), `goscape-telemetry-platform` (follow-up workstream)

## Decision

Remove the `public_chat` and `private_chat` tables from the goscape central
database and make the telemetry event pipeline (Kafka → ClickHouse in the
telemetry platform) the only chat sink. This is a **deliberate, documented
divergence from Engine-TS**, which persists both tables; it applies to the
public goscape repo on all 5 rev branches (274, 254, 245.2, 244, 225).

Decisions made during brainstorming:

| Question | Decision |
|---|---|
| Scope | Public goscape repo itself (not a platform-side fork/toggle) |
| Delivery guarantee | Fire-and-forget: chat rides the existing non-blocking `telemetry.Emitter` contract; a Kafka outage or buffer overflow drops chat records |
| Branches | All 5 rev branches |
| Existing data | Drop migration destroys existing chat rows; no backfill |
| Approach | A (retire dead plumbing), with private chat emitting from the friends server |
| Naming | Public event message is `PublicChatEvent` (rename of `ChatMessageEvent`); `Channel` enum dropped |

## Background / current state

- The tables are **write-only**: `modules/friends/handler.go` inserts via
  `Repository.LogPublicMessage` / `LogPrivateMessage`; nothing in goscape or
  the platform ever reads them back. They mirror TS `FriendServer.ts`
  moderation logging.
- **No real chat events are emitted today.** `proto/events/v1/world.proto`
  has `ChatMessageEvent` and `player_input.proto` has `PrivateChatEvent`, and
  ClickHouse `001_events.sql` has matching columns, but the only producer is
  the platform's demo seeder. Real chat currently lands only in the DB tables.
- The platform is **not a fork**: it imports goscape as a Go module
  (`replace github.com/zsrv/goscape => ../goscape-rev225`) and installs its
  Kafka emitter via the `pkg/telemetry.Set` seam (no-op in the public repo).
- ClickHouse ingests Kafka protobuf via duplicated schema copies in
  `deploy/bundled/clickhouse/format_schemas/`, matching **columns to proto
  field names** — proto renames force platform-side migration work.

## Design

### 1. Event schema (`proto/events/v1/`, goscape; identical on all branches)

`world.proto`:

- Rename `ChatMessageEvent` → `PublicChatEvent`; rename the `WorldEnvelope`
  oneof field `chat` → `public_chat` (field number 103 unchanged).
- Drop the `Channel` enum (`CHANNEL_PUBLIC`/`CHANNEL_CLAN`): these revisions
  are 2004–05 era; clan chat does not exist. Speculative dead weight.
- New shape: `PublicChatEvent { string session_uuid = 1; int32 coord = 2;
  string text = 3; }`

`player_input.proto`:

- `PrivateChatEvent` stays in `PlayerInputEnvelope` (oneof field 104); add
  `int32 coord = 3` → `{ int64 recipient_account_id = 1; string text = 2;
  int32 coord = 3; }`

Envelope fields (`account_id`, `world_id`, `ts`) cover the rest. Coverage vs.
today's DB rows: nothing captured today is lost. `private_chat.profile` has
no home anywhere in the event schema (the platform is single-profile) —
accepted.

Regenerate `pkg/eventspb` via buf.

### 2. Public chat: emit from the world module, retire the RPC

- `modules/world/handlers_game.go` (~line 428): replace
  `s.friendsBridge.PublicMessage(p.sessionOrHeadless(), coord, decoded)` with
  `telemetry.Get().EmitWorld(...)` carrying a `PublicChatEvent`
  (`session_uuid` = `p.sessionOrHeadless()`, `coord`, `text` = decoded;
  envelope `account_id` = player's account id, `world_id` = node id). Same
  emission pattern as the mouse-move event in `handler_events.go`.
- Delete the now-dead plumbing end to end:
  - `FriendsBridge.PublicMessage` (`modules/world/bridges.go`) + all impls
    (grpc bridge, noops, test fakes)
  - `grpcFriendsClient.PublicMessage` (`modules/world/friends_client.go`)
  - friends-server `PublicMessage` handler (`modules/friends/handler.go`)
  - `Repository.LogPublicMessage` (`modules/friends/repository.go`)
  - `rpc PublicMessage` + `PublicMessageRequest` from the friends proto;
    regenerate `pkg/friendspb`.

### 3. Private chat: emit from the friends handler

The recipient's account id exists only in the friends server (the world side
has username37s and no central-DB access), and the TS silent-drop semantics
live there — so emission happens in `handler.PrivateMessage`:

1. Resolve both endpoint accounts. The existing `errAccountMissing`
   silent-drop behavior (TS `FriendServer.ts:266-284`: either account
   unresolvable → no delivery, successful result) moves out of
   `LogPrivateMessage` into a resolve-only repository method. Delivery
   semantics must not drift.
2. Emit `PlayerInputEnvelope{PrivateChatEvent}`: envelope `account_id` =
   sender's resolved account id, `world_id` = `req.WorldId`;
   `recipient_account_id`, `text` = `req.Chat`, `coord` = `req.Coord`.
3. Deliver via `h.subs.send` exactly as today.

`LogPrivateMessage`'s INSERT is deleted. The friends process emits through
the same global `telemetry` seam (no wiring change in goscape; the platform
installs the emitter in whatever process composition it runs).

### 4. Schema migration

New `000002_drop_chat.up.sql` in both `pkg/gamedb/migrations/sqlite/` and
`.../postgres/`:

```sql
DROP TABLE IF EXISTS public_chat;
DROP TABLE IF EXISTS private_chat;
```

`000001_init.up.sql` is left untouched (existing deployments are at version
1; editing init would fork fresh vs. upgraded schemas). Existing chat rows
are destroyed by design — no backfill.

### 5. Fidelity documentation

Per the true-to-TS gate, every removal site carries a comment citing this
spec (divergence: TS persists chat to the DB; goscape emits telemetry events
instead), and the docs get a note (porting/architecture doc for the affected
guides). A plain public deployment (no telemetry emitter installed) records
chat **nowhere** — accepted consequence of the scope decision.

### 6. Branch rollout

rev-274 first, then port to 254 / 245.2 / 244 / 225.

- Proto + eventspb changes: COPYABLE (event schema identical across
  branches).
- Friends/world deletions and the drop migration: ADAPT per branch —
  245.2/244 have the 6-column `public_chat` (profile/world in the RPC
  request), 225 has the pre-B5 schema; the deleted surface differs.
- Per-branch compile-all gate: `go test -run '^$' ./...` plus the touched
  packages' tests.

### 7. Platform workstream (`goscape-telemetry-platform`, follow-up)

Separate plan, executed after the goscape side (at minimum after rev-225
lands, since the platform builds against `../goscape-rev225`):

- Sync `deploy/bundled/clickhouse/format_schemas/world.proto` and
  `player_input.proto` with the renamed/extended messages.
- New ClickHouse migration: rework the `events_world` Kafka raw table +
  materialized view + storage table (`chat.*` → `public_chat.*` naming, drop
  `chat_channel`, add `session_uuid`/`coord` columns); add
  `private_chat.coord` to the `events_player_input` pipeline.
- Update the demo seeder: `cmd/goscape-cli/demo/publish.go` (synthetic chat
  events → new message shapes), `pgwrite.go` (currently writes the
  now-dropped Postgres tables), and narratives.
- Sweep dashboards/detections/queries for `chat_channel`/`chat_text`
  references and adjust.
- Rebuild against the updated rev-225 worktree; end-to-end verify chat →
  Kafka → ClickHouse rows in the bundled deploy.

## Error handling

Inherited, by decision: emission is fire-and-forget per the `Emitter`
contract ("must not block"); Kafka unavailability silently drops chat
records. PM delivery failure modes are unchanged from today. The removed
`codes.Internal` insert-failure path in `PublicMessage`/`PrivateMessage`
disappears with the inserts (public chat can no longer fail server-side at
all; private chat can still fail on account resolution → same
`codes.Internal` posture for genuine resolve errors, silent drop for missing
accounts).

## Testing

goscape (per branch):

- Friends handler tests: PM emission asserted via a capture emitter
  (`telemetry.Set` + `Reset` for isolation, as existing emit tests do);
  silent-drop on unresolvable account preserved; no chat INSERT issued;
  `PublicMessage` handler/RPC gone.
- World handler test: `MESSAGE_PUBLIC` produces a `PublicChatEvent` with
  session uuid, coord, decoded text.
- `pkg/gamedb/migrate_test.go`: migrated schema contains neither table
  (both backends).
- Existing tests referencing the tables/RPC (`repository_test.go`,
  `handler_test.go`, `bridges_test.go`, `friends_smoke_test.go`,
  `login_username_test.go`) updated or removed with the surface.

Platform: migration applies clean on a fresh and an existing ClickHouse;
demo seed integration test green; end-to-end chat visible in ClickHouse via
the bundled deploy.

## Out of scope

- Any durability upgrade for chat events (spill queues, acks) — explicitly
  declined in favor of the fire-and-forget contract.
- Backfill of existing DB chat rows into ClickHouse.
- Multi-profile support in the event schema.
