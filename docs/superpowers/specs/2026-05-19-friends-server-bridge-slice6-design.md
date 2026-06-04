# Friends-server bridge — slice 6 design: chat logging (`private_chat` persistence)

**Date:** 2026-05-19
**Slice:** 6 of 7 (friends-server bridge arc)
**Predecessor:** slice 5b (close commit `d6cbfe0e`, retired `NAI-S5A-D-DISPATCHER-NO-ACTION`; see `[[friends-server-slice5b-close]]`)
**Closes:** `NAI-S1-D-PM-NO-PERSISTENCE`
**Opens:** `NAI-S6-D-NO-READ-RPC` (permanent), `NAI-S6-D-NO-RETENTION` (permanent), `NAI-S6-D-PUBLIC-CHAT-DEFERRED` (retires post-slice-7)

## 1. Scope

Persist every PrivateMessage RPC server-side to a new SQLite `private_chat` table before the registry routes the `PrivateMessageDelivery` to the recipient's `SubscribeUpdates` stream. Insert-then-send mirrors TS `FriendServer.ts:273-285` exactly. On insert error the handler returns `codes.Internal` and the PM is **not** delivered, mirroring the TS thrown-await pattern and the established Internal-error posture of `FriendlistAdd`/`FriendlistDel`/`IgnorelistAdd`/`IgnorelistDel`.

Slice 6 is **PM-only**. TS also persists `public_chat` (a parallel table for `PUBLIC_CHAT_LOG` opcode messages), but its required `session_uuid` column is the per-login UUID introduced by slice 7 (`Player.session`). Shipping `public_chat` would either force a placeholder uuid or wait for slice 7. Slice 6 explicitly defers `public_chat` behind `NAI-S6-D-PUBLIC-CHAT-DEFERRED`; the follow-up slice (post-7) adds the table, the `PublicMessageRequest` RPC, and the world-side hook.

No proto change. No world-side production code change. No config change.

## 2. Forward map (what changes)

| File | New / changed | Notes |
|---|---|---|
| `modules/friends/migrations/000002_private_chat.up.sql` | **new** | Create `private_chat` table + two indexes |
| `modules/friends/repository.go` | **changed** | Add `LogPrivateMessage(ctx, from, to, coord, message) error` |
| `modules/friends/repository_test.go` | **changed** | Add 4 `TestRepository_LogPrivateMessage_*` tests |
| `modules/friends/db_test.go` | **changed** | Extend `TestOpenDB_AppliesMigrations` to assert `private_chat` table |
| `modules/friends/handler.go` | **changed** | `PrivateMessage` calls `repo.LogPrivateMessage` before `subs.send`; retire `NAI-S1-D-PM-NO-PERSISTENCE` tag, switch `_ context.Context` to `ctx`, doc-comment updated |
| `modules/friends/handler_test.go` | **changed** | Add 2 new tests (persists-before-sending / insert-error-blocks-send); keep existing slice-4b delivery tests intact |
| `modules/world/friends_smoke_test.go` | **changed** | Add `TestFriendsClient_E2E_PrivateMessagePersistsRow` |

**LOC estimate:** ~150 added, ~10 deleted.

## 3. Schema

New file `modules/friends/migrations/000002_private_chat.up.sql`:

```sql
CREATE TABLE private_chat (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    profile TEXT NOT NULL,
    from_username37 INTEGER NOT NULL,
    to_username37 INTEGER NOT NULL,
    coord INTEGER NOT NULL,
    message TEXT NOT NULL,
    created TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_private_chat_to
    ON private_chat (profile, to_username37, created);

CREATE INDEX idx_private_chat_from
    ON private_chat (profile, from_username37, created);
```

**Divergences from TS schema** (TS at `prisma/singleworld/migrations/20251229170623_clean/migration.sql:120-129`):

| TS column | goscape column | Why |
|---|---|---|
| `account_id` / `to_account_id` (FK to `account.id`) | `from_username37` / `to_username37` (INTEGER) | goscape addresses players by `username37` everywhere — slice 3's `NAI-S3-D-USERNAME37-NOT-ACCOUNTID` (permanent) established this. friendlist/ignorelist use the same shape. |
| `timestamp DATETIME NOT NULL` | `created TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP` | Matches existing friendlist/ignorelist column shape from slice 3. `created` is server-side wall-clock at insert; TS's `timestamp` is also server-side (`toDbDate(Date.now())` at FriendServer.ts:279). |
| no `profile` column | `profile TEXT NOT NULL` | Matches friendlist/ignorelist multi-tenancy. TS persists `profile` from the request (FriendServer.ts:277); goscape scopes by `r.profile` (set at `NewRepository` time, per slice-3 pattern). |
| no index | two composite indexes | Anticipates read RPCs (NAI-S6-D-NO-READ-RPC is "not in slice 6", not "never"). `(profile, to_username37, created)` is the natural inbox query; `(profile, from_username37, created)` is the natural outbox query. Both indexes are cheap on append-only tables. |
| `id` AUTOINCREMENT | `id` AUTOINCREMENT | Same. Surrogate key; not used by writes, available for reads/audit. |
| `message` TEXT | `message` TEXT | Same. No length cap, no validation, matches TS. |
| `coord` INTEGER | `coord` INTEGER | Same. Already on the wire as `PrivateMessageRequest.Coord` (slice 4b kept it but unused server-side; slice 6 starts using it). |

**Profile column rationale (single-row-per-write):** TS writes `message.profile` taken from the request envelope. goscape's `r.profile` is set once at `NewRepository(*sql.DB, profile)` per slice 3 — there's exactly one profile per running friends-server instance. The column exists for forward-compat with multi-tenant deployments where multiple profiles share a DB (matches friendlist/ignorelist's existing `profile` column). All writes pass `r.profile` verbatim.

## 4. Repository surface

Add to `modules/friends/repository.go`:

```go
// LogPrivateMessage appends one row to private_chat under r.profile.
// Mirrors TS FriendServer.ts:273-283 — append-only, no dedupe, no
// validation. Insert is the synchronous gate for PrivateMessage
// delivery: a failure here returns an error to the handler which
// surfaces codes.Internal to the caller, matching the TS thrown-
// await pattern.
func (r *Repository) LogPrivateMessage(ctx context.Context, from, to uint64, coord int32, message string) error {
    _, err := r.db.ExecContext(ctx,
        `INSERT INTO private_chat (profile, from_username37, to_username37, coord, message)
         VALUES (?, ?, ?, ?, ?)`,
        r.profile, int64(from), int64(to), coord, message,
    )
    if err != nil {
        return fmt.Errorf("LogPrivateMessage: %w", err)
    }
    return nil
}
```

No read method. `NAI-S6-D-NO-READ-RPC` (permanent — admin tooling will live elsewhere; not in this slice).

## 5. Handler change

`modules/friends/handler.go`, `PrivateMessage` (lines 146-173 today):

```go
// PrivateMessage persists the PM to private_chat under r.profile,
// then routes a PrivateMessageDelivery to the target's open stream
// (if any). Mirrors TS FriendServer.ts:266-285 — insert-then-send,
// fail the RPC on insert error (matches the established
// codes.Internal posture of FriendlistAdd/Del/IgnorelistAdd/Del).
//
// req.Coord is server-side-persisted (and otherwise unused for
// routing). req.WorldId is unused for routing because the registry
// is keyed solely by username37; cross-world routing falls out for
// free.
func (h *handler) PrivateMessage(ctx context.Context, req *friendspb.PrivateMessageRequest) (*emptypb.Empty, error) {
    h.ensureWorld(req.WorldId)
    if err := h.repo.LogPrivateMessage(ctx, req.Username37, req.TargetUsername37, req.Coord, req.Chat); err != nil {
        return nil, status.Errorf(codes.Internal, "LogPrivateMessage: %v", err)
    }
    h.subs.send(req.TargetUsername37, &friendspb.FriendsUpdate{
        Update: &friendspb.FriendsUpdate_PrivateMessage{
            PrivateMessage: &friendspb.PrivateMessageDelivery{
                FromUsername37: req.Username37,
                StaffLvl:       req.StaffLvl,
                PmId:           req.PmId,
                Chat:           req.Chat,
            },
        },
    })
    return &emptypb.Empty{}, nil
}
```

Changes vs slice 4b:
- `_ context.Context` → `ctx context.Context` (now passed to `LogPrivateMessage`).
- New persistence call before `subs.send`.
- `NAI-S1-D-PM-NO-PERSISTENCE — slice 6 retires.` line removed.
- Doc-comment rewritten to slice-6 form.

## 6. Repository tests (`repository_test.go`)

Four new tests, all using `newTestRepo(t)` from `repository_test.go:20`.

### 6.1 `TestRepository_LogPrivateMessage_PersistsRow`

- Call `r.LogPrivateMessage(ctx, 1111, 2222, 12345, "hi")`.
- `SELECT COUNT(*) FROM private_chat WHERE profile = 'test'` → 1.
- `SELECT from_username37, to_username37, coord, message FROM private_chat` → `(1111, 2222, 12345, "hi")`.

### 6.2 `TestRepository_LogPrivateMessage_AppendOnly`

- Call `r.LogPrivateMessage(ctx, 1111, 2222, 0, "first")`.
- Call `r.LogPrivateMessage(ctx, 1111, 2222, 0, "second")` — same sender / target / coord / message-prefix.
- `SELECT COUNT(*)` → 2 (no upsert, no dedupe).

### 6.3 `TestRepository_LogPrivateMessage_RespectsProfile`

- `r, db := newTestRepo(t)` — profile = `"test"`.
- `r2 := NewRepository(db, "other")` — second Repository on the same `*sql.DB`.
- `r.LogPrivateMessage(ctx, 1, 2, 0, "from default")`.
- `r2.LogPrivateMessage(ctx, 1, 2, 0, "from other")`.
- `SELECT profile, message FROM private_chat ORDER BY id` → `("test", "from default")`, `("other", "from other")`.

### 6.4 `TestRepository_LogPrivateMessage_EmptyMessageAllowed`

- `r.LogPrivateMessage(ctx, 1, 2, 0, "")` — no error.
- `SELECT COUNT(*) FROM private_chat WHERE message = ''` → 1.
- Pins that goscape does not server-side-validate; matches TS (`FriendServer.ts:268-283` does not check `chat.length`).

## 7. db_test extension

`modules/friends/db_test.go:26` (`TestOpenDB_AppliesMigrations`) currently asserts the presence of `friendlist` and `ignorelist`. Extend `wantTables := []string{"friendlist", "ignorelist", "private_chat"}`.

## 8. Handler tests (`handler_test.go`)

Two new tests. The existing slice-4b tests (`TestPrivateMessage_DeliveredToRecipient`, `TestPrivateMessage_NoSubscription_Drops`, `TestPrivateMessage_CrossWorld`) **stay green** — they don't query `private_chat` and the new INSERT succeeds inside `newTestRepo`.

### 8.1 `TestHandler_PrivateMessage_PersistsBeforeSending`

- Setup: `r, db := newTestRepo(t)`; build `handler` per existing fixture; open `testStream` for username `200`; drain initial snapshots.
- Sender: `PrivateMessage(WorldId=1, Username37=100, TargetUsername37=200, Coord=12345, Chat="hi", PmId=0xCAFEBABE)`.
- Assert `SELECT from_username37, to_username37, coord, message FROM private_chat` returns one row with `(100, 200, 12345, "hi")`.
- Assert `recvWithin(t, stream, 2s)` yields a `FriendsUpdate_PrivateMessage` with `PmId=0xCAFEBABE`.

### 8.2 `TestHandler_PrivateMessage_InsertErrorBlocksSend`

- Setup: `r, db := newTestRepo(t)`; **`db.Close()`** to force INSERT failure; build handler around `r`; open `testStream` for `200`; drain initial snapshots **before** closing `db` (since initial snapshots read `friendlist`/`ignorelist`).

  **Wrinkle:** the existing initial-snapshot path uses the same `*sql.DB`. Closing the DB after snapshots completes means subsequent reads on the stream don't issue queries — only the handler's INSERT does. Sequencing:
  1. Open stream, drain 2 initial snapshot messages.
  2. `db.Close()`.
  3. Call `PrivateMessage`.
  4. Assert it returns `codes.Internal`.
  5. Assert `recvWithin(t, stream, 200ms)` returns `false` (no delivery).

- Assert `status.Code(err) == codes.Internal`.
- Assert no PrivateMessageDelivery arrives on the stream within a short window.
- This pins **insert-then-send** ordering: a broken DB blocks delivery.

The "drain initial snapshots before closing" sequencing is the established pattern for slice-3 tests that needed a broken DB (slice 3 didn't ship one of these tests, but the same constraint applied).

## 9. World-side e2e test

`modules/world/friends_smoke_test.go` — add immediately after `TestFriendsClient_E2E_PrivateMessageDelivery` (line 261):

`TestFriendsClient_E2E_PrivateMessagePersistsRow`:

- Boot in-process `friends.Friends` with `t.TempDir()/friends.db` (real on-disk SQLite, not in-memory, because we'll query it after the RPC).
- Two `PlayerLogin` calls (sender 1111 on world 10, recipient 2222 on world 10).
- Single `client.PrivateMessage(...)` call with `Coord=42, Chat="persisted"`.
- Open a fresh `*sql.DB` against the same DSN (cfg.SQLiteDSN) and `SELECT from_username37, to_username37, coord, message FROM private_chat`.
- Poll up to 2s for the row (matches `waitForPrivate` cadence — the friends-server commits synchronously inside the RPC, but cgo-less SQLite + WAL settles within ms; 2s is generous).
- Assert one row with `(1111, 2222, 42, "persisted")`.

Skip the recipient dispatcher / subscriber wiring — that's already pinned by `TestFriendsClient_E2E_PrivateMessageDelivery`. The new test is **persistence-only**; the existing delivery test is **delivery-only**. Together they pin both halves of the insert-then-send chain.

**On-disk DSN rationale:** the existing slice-4b e2e uses `filepath.Join(t.TempDir(), "friends.db")`, which is on-disk. Slice 6's e2e reuses that. The "second `*sql.DB` open against same DSN" pattern works because both opens hit the same file. (For pure in-memory DSN with `mode=memory&cache=shared`, the pattern would also work but requires careful cleanup ordering; on-disk is simpler.)

## 10. Why no `ChatPersister` abstraction

Slice 5b composed dispatchers via `actionWorldEventsDispatcher` because there were **two** concerns (slog logging + world-state actions) and a clean composition boundary. Slice 6 has **one** concern (write a row to SQLite) and one production caller (`handler.PrivateMessage`). Introducing a `ChatPersister` interface would be premature abstraction — call the Repository method directly, same as `AddFriend`/`DeleteIgnore`/etc.

If slice 6 ever needs to gate persistence by config (e.g., a "disable chat logging" flag), the interface comes in then. Today, no.

## 11. Why insert-then-send (not send-then-insert, not fire-and-forget)

Three considered orderings:

| Ordering | Pros | Cons | TS parity |
|---|---|---|---|
| **insert-then-send** (chosen) | TS-faithful. Failed insert blocks delivery → caller learns about audit-trail failures. Matches established `FriendlistAdd`/etc. Internal-error posture. | A transient SQL error drops the PM. | exact (FriendServer.ts:273-285) |
| send-then-insert | PM delivery never blocked by audit failure. | Audit can lag/skew vs delivery; if insert fails, delivery already happened — no rollback. Diverges from TS. | no |
| fire-and-forget (goroutine) | Lowest latency. | Failed inserts silent — audit holes. Goroutine bookkeeping for graceful shutdown. Diverges from TS. | no |

Slice 6 picks TS-faithful. PM delivery is best-effort overall (registry drops if subscriber buffer full — `NAI-S4A-D-DROP-ON-FULL` is permanent) so a transient SQL error blocking the PM is consistent with the broader posture.

## 12. Deviation tag inventory

### Retired by slice 6 (1)
- `NAI-S1-D-PM-NO-PERSISTENCE` — server `PrivateMessage` persists to `private_chat`.

### Opened by slice 6 (3)
- `NAI-S6-D-NO-READ-RPC` (permanent) — slice 6 ships write-only. Read RPC / admin tooling lives in a future slice or admin tooling repo.
- `NAI-S6-D-NO-RETENTION` (permanent) — `private_chat` is append-only forever. No retention pruning, no rolling-window deletion, no max-row caps. TS makes the same choice.
- `NAI-S6-D-PUBLIC-CHAT-DEFERRED` (retires post-slice-7) — `public_chat` table not shipped; requires `Player.session` UUID from slice 7. Follow-up slice (named tentatively "slice 7b" or "slice 8") adds the table + RPC + world-side hook.

### Carried forward (not slice 6's concern)
- `NAI-S1-D-LAZY-WORLDINIT` (permanent)
- `NAI-S2-D-PLAYERLOGIN-AT-PROCESSLOGINS` (permanent)
- `NAI-S2-D-PLAYERLOGOUT-BOTH-PATHS` (permanent)
- `NAI-S3-D-*` (3, permanent, spec-only)
- `NAI-S4A-D-DROP-ON-FULL` (permanent)
- `NAI-S4A-D-NO-INGAME-PACKET-EMIT` (blocked on NAI-182-D5)
- `NAI-S4A-D-PERPLAYER-NOT-PERWORLD-STREAM` (permanent)
- `NAI-S4B-D-NO-INGAME-PM-EMIT` (blocked on NAI-182-D5)
- `NAI-S5A-D-NO-ADMIN-AUTH-AT-SERVER` (permanent)
- `NAI-S5A-D-WORLDEVENTS-DROP-ON-FULL` (permanent)
- `NAI-S5A-D-PERWORLD-EVENTS-STREAM-SEPARATE` (permanent)
- `NAI-S5B-D-CLEARLOGOUTS-NO-GOSCAPE-QUEUE` (permanent)
- `NAI-S5B-D-NO-RUNESCRIPT-RUNTIME` (retires when runescript runtime can dispatch named scripts)

## 13. Risk register

- **R1: Initial-snapshot read in test 8.2 fails because the DB is closed.** Mitigated by sequencing: drain snapshots BEFORE closing `db`. Spec call-out in §8.2.
- **R2: e2e SQLite WAL doesn't flush before the test's secondary read.** The `PRAGMA journal_mode=WAL` in `db.go` enables WAL, which can defer commits. Mitigation: the secondary `*sql.DB` open + `SELECT` against the same on-disk file sees committed rows immediately (SQLite WAL guarantees visibility within the same process; cross-connection visibility is also immediate for committed transactions). 2s poll cadence covers any edge.
- **R3: `private_chat` table name collides with TS migration tooling.** Goscape's migration files are `embed.FS`-bundled and applied via `golang-migrate`; they don't share state with TS Prisma. Name collision is irrelevant.
- **R4: Insert error masks a delivery bug.** Test 8.2 explicitly forces an insert error; test 8.1 explicitly asserts both insert and delivery happened. The two together cover the regression surface.
- **R5: Race between insert and concurrent SubscribeUpdates teardown.** `LogPrivateMessage` is a single `ExecContext` with no ambient lock; the registry's `subs.send` is independent. No new race surface.

## 14. Out of scope (deferred to slice 7 and post-7)

- `Player.session` per-login UUID — slice 7.
- `public_chat` persistence + `PublicMessageRequest` RPC + world-side hook — post-slice-7, behind `NAI-S6-D-PUBLIC-CHAT-DEFERRED`.
- Admin read RPC / admin tooling — post-arc, behind `NAI-S6-D-NO-READ-RPC`.
- Retention pruning — never (permanent posture, `NAI-S6-D-NO-RETENTION`).
- In-game `MESSAGE_PRIVATE` packet emit — blocked on `NAI-182-D5` (social-cluster ServerGameProt port).

## 15. Sibling references

- TS canonical: `Engine-TS/src/server/friend/FriendServer.ts:266-285` (PRIVATE_MESSAGE branch — INSERT then sendPrivateMessage).
- TS schema: `Engine-TS/prisma/singleworld/migrations/20251229170623_clean/migration.sql:120-129` (`private_chat` table).
- TS types: `Engine-TS/src/db/types.ts:63-71` (`private_chat` row shape).
- Slice-3 SQL pattern: `[[friends-server-slice3-close]]` (createTestDB, NewRepository(db, profile), ctx + error on every SQL method).
- Slice-4b PM routing: `[[friends-server-slice4b-close]]` (`subs.send` + `PrivateMessageDelivery`).
- Established Internal-error-on-SQL-failure posture: `handler.go:106` (FriendlistAdd), `handler.go:117` (FriendlistDel), `handler.go:128` (IgnorelistAdd), `handler.go:139` (IgnorelistDel).
