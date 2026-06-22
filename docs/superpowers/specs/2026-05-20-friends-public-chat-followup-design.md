# Friends-server follow-up — `public_chat` persistence

**Date:** 2026-05-20
**Position:** standalone follow-up (NOT part of the slice 1-7 arc — the arc is fully closed by `[[friends-server-slice7-close]]`)
**Predecessor:** slice 7 close (HEAD `0c0709b8`; introduced `Player.session` per-login UUID v4 across world + login server; see `[[friends-server-slice7-close]]`)
**Schema template:** slice 6 (`[[friends-server-slice6-close]]`) — same `Repository(*sql.DB, profile)` ctor pattern, same `r.profile`-scoped INSERT, same `codes.Internal` posture on insert error
**Closes:** `NAI-S6-D-PUBLIC-CHAT-DEFERRED` (the sole conditional deviation that was waiting on slice 7's `Player.session` UUID)
**Opens:** none expected (TS-faithful design has no further deferrals)

## 1. Scope

Persist every public-chat utterance to a new SQLite `public_chat` table on the friends-server, keyed by the per-login UUID introduced in slice 7 (`Player.session`). Adds a new `PublicMessage` unary RPC on `FriendsService`, a new world-side hook in `handleMessagePublic` that fires the RPC fire-and-forget after in-world chat propagation, and matching repository + handler code on the server side.

Mirrors TS `FriendServer.ts:286-297` (handler) and `FriendServer.ts:670-695` (world-side emitter `sendPublicMessage`). The TS handler does **nothing but insert** — no delivery, no validation, no replay, no session-uuid lookup. The follow-up matches that posture exactly.

**Non-goals:**

- No admin read RPC. `NAI-S6-D-NO-READ-RPC` (permanent) continues to apply to both `private_chat` and `public_chat`.
- No retention policy. `NAI-S6-D-NO-RETENTION` (permanent) continues to apply.
- No in-world public-chat behavior changes. The hook fires **after** the existing `p.Chat(...)` call; in-world propagation is unchanged.
- No `session_uuid` validation (e.g., joining against the `session` table). TS skips this; goscape matches.
- No backfill / data migration. `public_chat` is new; the table is empty after migration.

## 2. Forward map (what changes)

| File | New / changed | Notes |
|---|---|---|
| `proto/friends/friends.proto` | **changed** | Add `PublicMessage` RPC + `PublicMessageRequest` message |
| `pkg/friendspb/friends.pb.go` | **regenerated** | `make protos` — friends-only |
| `pkg/friendspb/friends_grpc.pb.go` | **regenerated** | `make protos` — friends-only |
| `modules/friends/migrations/000003_public_chat.up.sql` | **new** | Create `public_chat` table + two indexes |
| `modules/friends/repository.go` | **changed** | Add `LogPublicMessage(ctx, sessionUUID, coord, message) error` |
| `modules/friends/repository_test.go` | **changed** | Add 4 `TestRepository_LogPublicMessage_*` tests |
| `modules/friends/db_test.go` | **changed** | Extend `TestOpenDB_AppliesMigrations` to assert `public_chat` table |
| `modules/friends/handler.go` | **changed** | Add `PublicMessage(ctx, *friendspb.PublicMessageRequest) (*emptypb.Empty, error)`; retire `NAI-S6-D-PUBLIC-CHAT-DEFERRED` tag |
| `modules/friends/handler_test.go` | **changed** | Add 2 `TestHandler_PublicMessage_*` tests (persists-row / insert-error-returns-Internal) |
| `modules/world/friends_client.go` | **changed** | Extend `FriendsClient` interface with `PublicMessage(...)`; add `grpcFriendsClient.PublicMessage` impl |
| `modules/world/friends_client_test.go` | **changed** | Extend the existing `fakeFriendsClient` (or equivalent test double) with `PublicMessage` capture |
| `modules/world/handlers_game.go` | **changed** | Extend `handleMessagePublic` to fire-and-forget the RPC after `p.Chat(...)`; skip when `p.session == "headless"` |
| `modules/world/handlers_game_test.go` | **changed** | Add 3 unit tests (RPC fires with expected fields / skipped when session=="headless" / log-warn on RPC failure via syncBuffer) |
| `modules/world/friends_smoke_test.go` | **changed** | Add `TestFriendsClient_E2E_PublicMessagePersistsRow` |

**LOC estimate:** ~280 added, ~5 deleted (deviation-tag comment retirement).

## 3. Schema

New file `modules/friends/migrations/000003_public_chat.up.sql`:

```sql
CREATE TABLE public_chat (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    profile TEXT NOT NULL,
    session_uuid TEXT NOT NULL,
    coord INTEGER NOT NULL,
    message TEXT NOT NULL,
    created TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_public_chat_session
    ON public_chat (profile, session_uuid, created);

CREATE INDEX idx_public_chat_recent
    ON public_chat (profile, created);
```

**Divergences from TS schema** (TS at `prisma/singleworld/migrations/20251229170623_clean/migration.sql` `public_chat` table):

| TS column | goscape column | Why |
|---|---|---|
| `session_uuid` TEXT (NOT FK) | `session_uuid` TEXT NOT NULL | Same. Slice 7 made `Player.session` a real UUID v4; TS-faithful single-column session keying. No `from_username37` (admin tooling joins to `session` table downstream — same posture as TS). |
| `timestamp` DATETIME | `created` TEXT DEFAULT CURRENT_TIMESTAMP | Matches existing `private_chat` shape from slice 6 and friendlist/ignorelist shape from slice 3. Server-side wall-clock at insert. |
| no `profile` column | `profile` TEXT NOT NULL | Matches `private_chat` from slice 6 and friendlist/ignorelist from slice 3. Multi-tenancy column; all writes pass `r.profile` verbatim. |
| no index | two composite indexes | Anticipates future admin read RPCs (per `NAI-S6-D-NO-READ-RPC` — "not in scope", not "never"). `(profile, session_uuid, created)` is the natural per-session lookup ("show me what session X said"); `(profile, created)` is the natural recent-activity scan ("show me all public chat in the last 30 minutes"). Cheap on append-only tables. |
| `id` AUTOINCREMENT | `id` AUTOINCREMENT | Same. Surrogate key; not used by writes. |
| `message` TEXT | `message` TEXT | Same. No length cap, no validation. |
| `coord` INTEGER | `coord` INTEGER | Same. Packed 30-bit RS coord (`coordgrid.PackCoord` output). |

**No `from_username37` column.** Mirrors TS exactly. `session_uuid` is the only identifier the row carries. Admin tooling that needs the speaker's username joins `public_chat.session_uuid` against `session.session_uuid` (the login-server-side table populated by `[[friends-server-slice7-close]]`). The session table stores `account_id`/username; the join resolves cleanly. Adding `from_username37` to the row would either duplicate that join (cheap reads, denormalized writes) or diverge from TS in a way slice 6 explicitly declined. **Recommend TS-faithful** — admin tooling does its own join.

## 4. Repository surface

Add to `modules/friends/repository.go`:

```go
// LogPublicMessage appends one row to public_chat under r.profile.
// Mirrors TS FriendServer.ts:286-297 — append-only, no dedupe, no
// validation, no session_uuid existence check. Insert is the
// synchronous gate for the RPC: a failure returns an error to the
// handler which surfaces codes.Internal to the caller, matching the
// TS thrown-await pattern and slice 6's posture.
func (r *Repository) LogPublicMessage(ctx context.Context, sessionUUID string, coord int32, message string) error {
    _, err := r.db.ExecContext(ctx,
        `INSERT INTO public_chat (profile, session_uuid, coord, message)
         VALUES (?, ?, ?, ?)`,
        r.profile, sessionUUID, coord, message)
    return err
}
```

**Signature shape:** mirrors slice-6 `LogPrivateMessage(ctx, from, to uint64, coord int32, message string) error`. `sessionUUID string` replaces the two `uint64` username keys; `coord int32` and `message string` match slice 6 exactly. Returns `error` per slice-3 ctx-and-error pattern.

**Empty-message tolerance.** TS does not validate; goscape matches. The repository accepts `message=""` and inserts a row. Test pins this (see §6.1).

## 5. Handler surface

Add to `modules/friends/handler.go`:

```go
// PublicMessage persists one row to public_chat. Mirrors TS
// FriendServer.ts:286-297 — append-only, no delivery, no validation.
// Insert error → codes.Internal (matches slice 6 PrivateMessage posture
// and the FRIENDLIST/IGNORELIST mutation handlers).
func (h *handler) PublicMessage(ctx context.Context, req *friendspb.PublicMessageRequest) (*emptypb.Empty, error) {
    if err := h.repo.LogPublicMessage(ctx, req.SessionUuid, req.Coord, req.Chat); err != nil {
        h.log.LogAttrs(ctx, slog.LevelError, "PublicMessage insert failed",
            slog.Int("world_id", int(req.WorldId)),
            slog.String("session_uuid", req.SessionUuid),
            slog.Any("error", err))
        return nil, status.Errorf(codes.Internal, "log public message: %v", err)
    }
    return &emptypb.Empty{}, nil
}
```

Field reads (`req.SessionUuid`, `req.Coord`, `req.Chat`, `req.WorldId`) bind to the proto-generated names — see §6 for the proto definition. The handler does **not** validate non-empty fields, does **not** check that the session_uuid exists in any `session`-equivalent table, and does **not** broadcast through `subscriptions.send` (no `PublicMessageDelivery` exists; `public_chat` is audit-only).

## 6. Proto surface

Append to `proto/friends/friends.proto`:

```proto
service FriendsService {
  // ... existing RPCs ...

  // Public-chat audit log. Mirrors TS FriendServer.ts:286-297 — append-
  // only persistence keyed by per-login session UUID (Player.session,
  // populated by slice 7). No delivery half; the world handles in-world
  // chat propagation itself. Insert error → codes.Internal.
  rpc PublicMessage(PublicMessageRequest) returns (google.protobuf.Empty);
}

// ... existing messages ...

message PublicMessageRequest {
  int32  world_id     = 1;
  string session_uuid = 2;
  int32  coord        = 3;
  string chat         = 4;
}
```

**Field order rationale.** Matches `PrivateMessageRequest`'s envelope-first convention: `world_id` is field 1 (carries the originating world for admin filtering and matches `WorldConnectRequest`, `PlayerLoginRequest`, etc.). `session_uuid`/`coord`/`chat` follow in the order TS sends them on the wire (`FriendServer.ts:683-687` → `session_uuid, coord, chat`).

**No `nodeTime` field.** TS includes `nodeTime: Date.now()` on the wire and writes it as `timestamp` in the row. goscape mirrors slice 6 instead: server-side `created TEXT DEFAULT CURRENT_TIMESTAMP` populated at insert. The client-side timestamp would carry sub-tick precision that the audit use case doesn't need, and the `CURRENT_TIMESTAMP` shape matches every other goscape table.

**`make protos` runs friends-only.** Per slice-7 lesson #9, `make protos` regenerates ALL `.pb.go` files; watch for unrelated drift and stage only `pkg/friendspb/*.pb.go`.

## 7. World-side surface

### 7.1 `FriendsClient` interface

Extend the interface in `modules/world/friends_client.go`:

```go
type FriendsClient interface {
    // ... existing methods ...
    PublicMessage(ctx context.Context, req *friendspb.PublicMessageRequest) (*emptypb.Empty, error)
}
```

Add the `grpcFriendsClient.PublicMessage` impl as a thin pass-through to the generated `FriendsServiceClient.PublicMessage`. Mirrors `grpcFriendsClient.PrivateMessage` (slice 4b shape).

### 7.2 Hook in `handleMessagePublic`

Existing handler at `modules/world/handlers_game.go:344`:

```go
func handleMessagePublic(p *Player, payload []byte) error {
    if len(payload) < 2 {
        return nil
    }
    color := int(payload[0])
    effect := int(payload[1])
    msg := bytes.Clone(payload[2:])
    p.Chat(color, effect, int(p.staffModLevel), msg)
    return nil
}
```

Extended:

```go
func handleMessagePublic(p *Player, payload []byte) error {
    if len(payload) < 2 {
        return nil
    }
    color := int(payload[0])
    effect := int(payload[1])
    msg := bytes.Clone(payload[2:])
    p.Chat(color, effect, int(p.staffModLevel), msg)

    // Audit-log to friends-server, fire-and-forget. Skipped when the
    // session UUID isn't a real per-login UUID (unbridged paths, e.g.
    // standalone-world without a login-server bridge — see slice 7
    // close). The RPC is goroutine-wrapped so the tick never blocks.
    if p.client != nil && p.client.server != nil {
        s := p.client.server
        if s.friendsClient != nil && p.session != "" && p.session != "headless" {
            go s.publishPublicChatAudit(p.session, p.Coord(), string(msg))
        }
    }
    return nil
}
```

A new helper on `*Server`:

```go
// publishPublicChatAudit fires a fire-and-forget PublicMessage RPC.
// Errors are log-warned and dropped — audit logging must never gate
// gameplay (discipline lesson #12).
func (s *Server) publishPublicChatAudit(sessionUUID string, coord int32, chat string) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    if _, err := s.friendsClient.PublicMessage(ctx, &friendspb.PublicMessageRequest{
        WorldId:     int32(s.worldID),
        SessionUuid: sessionUUID,
        Coord:       coord,
        Chat:        chat,
    }); err != nil {
        s.log.LogAttrs(context.Background(), slog.LevelWarn, "friends PublicMessage failed",
            slog.String("session_uuid", sessionUUID),
            slog.Any("error", err))
    }
}
```

**Why a method on `*Server` rather than inline?** Three reasons:
1. The method is the unit testable surface for the "fire-and-forget" + "log-warn on failure" + "5s timeout" contract. Tests stub `friendsClient` and assert behavior on the method directly.
2. `handleMessagePublic` keeps its existing two-line shape with one new conditional, minimizing the diff and matching the established `grep_modules_world_handlers_game.go_for_friendsClient.go` discoverability.
3. Future hooks (e.g., a similar audit for the `MESSAGE_QUICKCHAT` opcode) can call the same method.

**Coord source.** `p.Coord()` returns the packed RS coord at the moment of utterance — same coord shape as `private_chat` (slice 6) and matches TS `coord: this.coord` at `FriendServer.ts:686`. If `p.Coord()` is not the canonical accessor, the plan-write step pins it against the existing `MESSAGE_PUBLIC` path's coord handling and adjusts.

**5-second timeout.** A generous but bounded ceiling. Tick takes 600 ms; the audit RPC must not pile up indefinitely if the friends-server is unreachable. The goroutine wrap prevents tick blocking; the timeout bounds goroutine lifetime. Matches the implicit context-deadline pattern used elsewhere in `modules/world` for outbound RPCs (verify in plan-write).

**Skip when `p.session == "headless"` or empty.** Two skip conditions:
- `p.session == "headless"` — unbridged path (no login-server bridge configured); the `"headless"` literal is the tick fallback from slice 7. Audit row would be meaningless.
- `p.session == ""` — defensive; slice 7 stamps `p.session` at `newPlayer(c)` so this should never trigger in production, but defensive against test paths or future regressions.

Both skips happen at the call-site so the goroutine isn't even spawned. Tests pin both.

### 7.3 Nil-`friendsClient` guard

The `s.friendsClient != nil` guard is required because `modules/world` has long supported running without a friends-server (the bridge module is optional; `[[friends-server-slice1-close]]` documents the gate at `friends.enable=false`). Existing slice-4a/4b hooks have the same guard; mirror their shape.

## 8. Test plan

### 8.1 Repository tests (`modules/friends/repository_test.go`)

Add four tests mirroring `TestRepository_LogPrivateMessage_*`:

| Test | Asserts |
|---|---|
| `TestRepository_LogPublicMessage_PersistsRow` | After one `LogPublicMessage(...)`, the `public_chat` table has exactly one row with the expected `(profile, session_uuid, coord, message)` and a non-empty `created` timestamp. |
| `TestRepository_LogPublicMessage_AppendOnly` | Two consecutive `LogPublicMessage(...)` calls with identical args insert two rows (no upsert, no dedup). Verifies `id` AUTOINCREMENT distinct. |
| `TestRepository_LogPublicMessage_ProfileScoping` | Two repositories on the same DB with different `profile` strings each write their own row; selecting by `profile` returns only the matching row. |
| `TestRepository_LogPublicMessage_EmptyMessageTolerated` | `LogPublicMessage(ctx, "uuid-x", 0, "")` succeeds and writes the empty string verbatim (no validation). |

### 8.2 DB migration test (`modules/friends/db_test.go`)

Extend `TestOpenDB_AppliesMigrations` (added in slice 1, extended in slice 6) to assert the `public_chat` table exists with the expected column set after migrations apply. One-line add to the existing table-name assertion list.

### 8.3 Handler tests (`modules/friends/handler_test.go`)

Add two tests:

| Test | Asserts |
|---|---|
| `TestHandler_PublicMessage_PersistsRow` | `PublicMessage(ctx, req)` with a valid request returns `(&emptypb.Empty{}, nil)` AND a follow-up direct read of the `public_chat` table shows the row inserted. (Uses the real `Repository` over an in-memory `*sql.DB` — the same `newTestRepo` helper from slice 6.) |
| `TestHandler_PublicMessage_InsertErrorReturnsInternal` | A repository whose underlying DB is closed (forcing `INSERT` to fail) returns `nil, status.Error(codes.Internal, ...)` from the handler. Matches slice-6 PM insert-error test shape exactly. |

### 8.4 World-side unit tests (`modules/world/handlers_game_test.go`)

Add three tests:

| Test | Asserts |
|---|---|
| `TestHandleMessagePublic_FiresPublicMessageRPC` | Setting up a `Server` with a `fakeFriendsClient` and a player with `p.session = "00000000-0000-0000-0000-000000000001"`, calling `handleMessagePublic` causes (after a short poll) the fake to record exactly one `PublicMessage` call with the expected `WorldId`, `SessionUuid`, `Coord`, `Chat`. |
| `TestHandleMessagePublic_SkipsWhenSessionHeadless` | With `p.session = "headless"`, calling `handleMessagePublic` records zero `PublicMessage` calls on the fake. |
| `TestHandleMessagePublic_LogWarnOnRPCFailure` | With a fake that returns an error from `PublicMessage`, calling `handleMessagePublic` produces a `WARN`-level log entry containing `"friends PublicMessage failed"` (captured via the slice-4c `syncBuffer` slog handler) AND does not error out (the test asserts the handler returns `nil`). |

The fake's `PublicMessage` method must be safe for concurrent capture (goroutine wrap means the test polls via `waitForPublic(fake, n, timeout)` — mirror slice-4b `waitForPrivate`).

### 8.5 World-side e2e (`modules/world/friends_smoke_test.go`)

Add one e2e test:

| Test | Asserts |
|---|---|
| `TestFriendsClient_E2E_PublicMessagePersistsRow` | Boots an in-process friends-server module against an on-disk SQLite file, opens a second `*sql.DB` on the same file, calls `PublicMessage` end-to-end via the world's `FriendsClient`, polls briefly for WAL settling, then SELECTs the row and asserts `(profile, session_uuid, coord, message)`. Mirrors slice-6 `TestFriendsClient_E2E_PrivateMessagePersistsRow` exactly. |

### 8.6 Concurrency / race

`go test -race ./...` is mandatory (discipline #8). The hook fires from the tick goroutine into a `go` call; the fake test double must guard its capture slice with a mutex (mirror slice-4b's `mu sync.Mutex` on `fakeFriendsClient.privateCalls`).

## 9. Discipline rules carried into executor prompts

From `[[friends-server-slice7-close]]` and the resume file §"Discipline lessons":

1. **NEVER `git checkout` / `git restore` tracked files.**
2. **Before every `git commit`:** `git status` first, `git show --stat HEAD` after. Recover via `git reset --mixed HEAD~1` (never amend). Per `[[git-pre-commit-status-check]]` feedback.
3. **Test helper files use `_test.go` suffix.**
4. **`unset GOROOT; export PATH="$HOME/go/current/bin:$PATH"`** prefix required for every shell session.
5. **`GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`** prefix required for every `go` command (global CLAUDE.md).
6. **`git commit --no-gpg-sign`** required by global CLAUDE.md.
7. **Stale-IDE-LSP / `go list` "No packages found" diagnostics are environmental** — ignore.
8. **`go test -race` always.** The hook fires from the tick goroutine into a goroutine — verify no in-tick blocking and no data races on the fake's capture slice.
9. **`make protos` regenerates ALL `.pb.go`.** Stage only `pkg/friendspb/*.pb.go` changes; revert any unrelated drift.
10. **Plan-write investigations save executor time.** Before writing T-blocks: read the existing `handleMessagePublic` in full, read `Repository.LogPrivateMessage` in full, read `TestFriendsClient_E2E_PrivateMessagePersistsRow` in full, confirm `p.Coord()` is the canonical packed-coord accessor on `*Player`.
11. **Reviewer fix-ups happen.** Expect 0-2 minor fix-ups per task.
12. **The world MUST NOT block on the friends-server RPC.** The hook is `go`-wrapped at the call site. An RPC failure log-warns and does NOT delay in-world chat propagation.

## 10. Out of scope (closed permanently or deferred)

- **Admin read RPC** — `NAI-S6-D-NO-READ-RPC` (permanent).
- **Retention policy** — `NAI-S6-D-NO-RETENTION` (permanent).
- **Ingame `PUBLIC_MESSAGE` packet emission to other worlds** — n/a. Public chat is in-world only; cross-world public-chat would need a `PublicMessageDelivery` analog and live cross-world broadcast, neither of which exist on the TS side either.
- **`session_uuid` existence validation** — TS skips it; goscape skips it.
- **`MESSAGE_QUICKCHAT` audit** — not in TS scope for `public_chat` (quickchat uses a separate audit path). Out of scope.
- **Pre-slice-7 garbage rows in `session.session_uuid`** — separate concern carried forward from `[[friends-server-slice7-close]]`. Does not affect `public_chat` writes.

## 11. Closing the follow-up

When all tasks ship clean:
1. Write `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/friends_server_public_chat_followup_close.md` mirroring `[[friends-server-slice7-close]]` format.
2. Add a one-line entry to `MEMORY.md`.
3. **Resulting state:** all friends-server work is at stable rest. All deviation tags are either retired or permanent; no further conditional retirements remain.
