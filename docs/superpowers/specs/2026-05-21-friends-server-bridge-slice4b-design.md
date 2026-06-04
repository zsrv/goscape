# Friends-server bridge — slice 4b design: PrivateMessage delivery via stream

**Date:** 2026-05-21
**Slice:** 4b of 7 (friends-server bridge arc; slice 4 decomposed into 4a/4b/4c)
**Predecessor:** slice 4a (close commit `60258c77`, retired `NAI-S1-D-NO-FOLLOWER-BROADCAST`; see `[[friends-server-slice4a-close]]`)
**Closes:** `NAI-S1-D-PM-NO-DELIVERY`
**Opens:** `NAI-S4B-D-NO-INGAME-PM-EMIT` (mirrors `NAI-S4A-D-NO-INGAME-PACKET-EMIT`, blocked on NAI-182-D5)

## 1. Scope

Replace the server-side `handler.PrivateMessage` no-op with a route through slice 4a's `subscriptions` registry. Sender's world calls the existing `PrivateMessage` RPC; server forwards to the recipient's open `SubscribeUpdates` stream as a `PrivateMessageDelivery` update; recipient's world subscriber routes that update to `FriendsDispatcher.OnPrivateMessage`.

All world-side surfaces — `friends_subscriber.dispatch`, `FriendsDispatcher` interface, `slogFriendsDispatcher.OnPrivateMessage` — were wired in slice 4a in anticipation of 4b. No world-side production code change. World-side e2e test is added.

Persistence of PMs (a SQL `private_chat` table) remains deferred to slice 6 (`NAI-S1-D-PM-NO-PERSISTENCE`). In-game `MESSAGE_PRIVATE` packet emit to the recipient's client connection remains deferred to NAI-182-D5 (social-cluster ServerGameProt port); slice 4b opens a new dedicated tag (`NAI-S4B-D-NO-INGAME-PM-EMIT`) on `slogFriendsDispatcher.OnPrivateMessage` to make this gating explicit.

## 2. Forward map (what changes)

| File | New / changed | Notes |
|---|---|---|
| `modules/friends/handler.go` | **changed** | `PrivateMessage` body replaced with `h.subs.send(...)` + `PrivateMessageDelivery`; retire `NAI-S1-D-PM-NO-DELIVERY` tag |
| `modules/friends/handler_test.go` | **changed** | Replace `TestHandler_PrivateMessage_NoOp_Slice1` with three new tests (delivered / no-subscription / cross-world) |
| `modules/world/bridges.go` | **changed** (doc only) | Open `NAI-S4B-D-NO-INGAME-PM-EMIT` on `slogFriendsDispatcher.OnPrivateMessage` doc-comment |
| `modules/world/friends_client.go` | **changed** (doc only) | Update doc-comment that references `NAI-S1-D-PM-NO-DELIVERY` as exemplar; replace with a still-open tag |
| `modules/world/friends_smoke_test.go` | **changed** | Add `TestFriendsClient_E2E_PrivateMessageDelivery` |

LOC estimate: ~150 added, ~30 deleted. Mechanically the smallest sub-slice of the 4a/4b/4c trio — substantially smaller than the ~20% estimate in the resume because 4a pre-built all the world-side plumbing.

## 3. Why no world-side code change

Slice 4a's `friends_subscriber.dispatch` already contains:

```go
case *friendspb.FriendsUpdate_PrivateMessage:
    pm := v.PrivateMessage
    s.dispatcher.OnPrivateMessage(s.username37, pm.FromUsername37, pm.StaffLvl, pm.PmId, pm.Chat)
```

and `bridges.go` declares the matching `FriendsDispatcher.OnPrivateMessage(target, from, staffLvl, pmId, chat)` method. `slogFriendsDispatcher.OnPrivateMessage` logs at Debug. `noopBridges.OnPrivateMessage` is a no-op. The wiring is complete for any `PrivateMessageDelivery` that arrives on the stream — 4a built this for forward compatibility precisely so 4b could be server-only.

The reason the in-game packet write to the recipient's client is **not** added in 4b is the same reason `slogFriendsDispatcher.OnFriendlistUpdate` doesn't write `UPDATE_FRIENDLIST` in 4a: the ServerGameProt social-cluster opcodes (MESSAGE_PRIVATE, UPDATE_FRIENDLIST, UPDATE_IGNORELIST) are blocked on NAI-182-D5. Until those writers exist, both dispatchers log only.

## 4. Server-side change

`modules/friends/handler.go`, the `PrivateMessage` method:

```go
// PrivateMessage routes a PM from req.Username37 to req.TargetUsername37
// by pushing PrivateMessageDelivery into the target's open stream (if
// any). Mirrors TS FriendServer.sendPrivateMessage (FriendServer.ts:480-
// 497): silently no-ops when the target has no open stream (TS:
// `if (!socket) return Promise.resolve()`). The registry's send method
// already implements the no-op-on-absent-subscriber semantic.
//
// Persistence of the PM to private_chat is slice 6.
// NAI-S1-D-PM-NO-PERSISTENCE — slice 6 retires.
func (h *handler) PrivateMessage(_ context.Context, req *friendspb.PrivateMessageRequest) (*emptypb.Empty, error) {
    h.ensureWorld(req.WorldId)
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

Notes:
- `req.Coord` is **unused** server-side, matching TS. The world-side handler already records coord in the local message log before issuing the RPC (`handler_message_private.go`). The friends-server is the dumb-pipe between the two worlds; recipient's world does not need coord to deliver the chat overlay.
- `req.WorldId` (sender's world) is **unused** for routing because the registry is keyed solely by `username37`. The recipient's subscription was opened with the recipient's `worldId`, so the cross-world hop happens naturally. `ensureWorld(req.WorldId)` is kept for the standard lazy-init audit (it tracks sender's world player slot, same as every other RPC).
- The `h.log.Debug` call that recorded slice-1's "accepted-and-logged" semantic is **removed**. Logging every delivered PM at Debug is noise; the world-side dispatcher already logs at the recipient end. (4a's broadcast helpers are similarly silent in steady state.)

## 5. Server-side tests

Replace `TestHandler_PrivateMessage_NoOp_Slice1` (its assertion — "returns OK with no observable side effect" — becomes false). Three new tests under `modules/friends/handler_test.go`, all using the existing `testStream` fixture:

### 5.1 `TestPrivateMessage_DeliveredToRecipient`
- Setup: register recipient `200` on world `1`; open a `SubscribeUpdates` stream for `200`; drain initial snapshots.
- Sender invokes `PrivateMessage(WorldId=1, Username37=100, TargetUsername37=200, StaffLvl=2, PmId=0xCAFEBABE, Chat="hello", Coord=12345)`.
- Assert: stream's next `recvWithin(t, 2s)` returns a `FriendsUpdate_PrivateMessage` with `FromUsername37=100, StaffLvl=2, PmId=0xCAFEBABE, Chat="hello"`.

### 5.2 `TestPrivateMessage_NoSubscription_Drops`
- Setup: no recipient stream open.
- Sender invokes `PrivateMessage` targeting `200`.
- Assert: RPC returns `(emptypb.Empty, nil)` with no error. (The drop is silent — there is no observable "drop counter" to assert against; the assertion is the absence of a panic/error and absence of any side effect.)

### 5.3 `TestPrivateMessage_CrossWorld`
- Setup: register sender on world `1`; register recipient on world `20` (via `r.Register(20, 100, ...)` and `r.Register(20, 200, ...)`); open `SubscribeUpdates(WorldId=20, Username37=200)`.
- Sender from world `1` invokes `PrivateMessage(WorldId=1, Username37=100, TargetUsername37=200, ...)`.
- Assert: recipient's world-20 stream receives the delivery. Pins that registry routing is world-agnostic.

## 6. World-side tests

`modules/world/friends_smoke_test.go` — add `TestFriendsClient_E2E_PrivateMessageDelivery`:

- Boot in-process `friends.Friends` with `t.TempDir()` SQLite, `freePort()`.
- Two `NewFriendsClient` instances (or one — the friends-server treats clients symmetrically; one client suffices for the e2e but two makes the cross-world story explicit). Choice: **one client** for clarity. The dispatcher + subscriber on the goscape test side is what differentiates "recipient world" from "sender world".
- `WorldConnect(10, "main")`.
- `PlayerLogin(WorldId=10, Username37=2222)` — recipient.
- `recordingFriendsDispatcher`; `newFriendsSubscriber(client, 10, 2222, disp, log)`; `go sub.run(subCtx)`.
- Drain initial empty snapshot (or skip the wait — recipient has no friends, initial snapshot is empty).
- `PlayerLogin(WorldId=10, Username37=1111)` — sender.
- `PrivateMessage(WorldId=10, Username37=1111, TargetUsername37=2222, StaffLvl=0, PmId=0xCAFEBABE, Chat="e2e hi", Coord=0)`.
- Poll `disp.private` (need to add a `privateCalls()` accessor to `recordingFriendsDispatcher`, sibling of `friendCalls()`) for an entry with `From=1111, Target=2222, PmId=0xCAFEBABE, Chat="e2e hi"` within 2s.

This pins the full e2e path: world A's PrivateMessage RPC → friends-server `PrivateMessage` handler → `subs.send` → recipient stream `Send` → grpc → world-side subscriber `Recv` → dispatch → `FriendsDispatcher.OnPrivateMessage`.

## 7. Doc-only changes for tag hygiene

### 7.1 Open `NAI-S4B-D-NO-INGAME-PM-EMIT`

`modules/world/bridges.go` — `slogFriendsDispatcher.OnPrivateMessage` (currently lines 106-111). Extend its (currently zero) doc-comment:

```go
// OnPrivateMessage logs the inbound PM at Debug. The MESSAGE_PRIVATE
// ServerGameProt packet write to player.client (mirroring TS
// World.ts:2000 `player.write(new MessagePrivate(...))`) is gated on
// NAI-182-D5 (social-cluster ServerGameProt port) — see
// NAI-S4A-D-NO-INGAME-PACKET-EMIT for the parallel friendlist/
// ignorelist gating.
//
// NAI-S4B-D-NO-INGAME-PM-EMIT — retires when NAI-182-D5 retires and
// the dispatcher is wired through to player.write(MessagePrivate{...}).
```

Also extend the `FriendsDispatcher` interface doc-comment (lines 67-81) to reference both deferred-emit tags (`NAI-S4A-D-NO-INGAME-PACKET-EMIT` for the friendlist/ignorelist methods and `NAI-S4B-D-NO-INGAME-PM-EMIT` for the PM method) rather than only the first.

### 7.2 Retire `NAI-S1-D-PM-NO-DELIVERY` references

- `modules/friends/handler.go:151` — remove the tag line; rewrite the doc-comment to its slice-4b form (see §4).
- `modules/world/friends_client.go:20` — the comment `(slice 1's NAI-S1-D-PM-NO-DELIVERY etc.)` cites a retired tag as exemplar of "friends-server is best-effort by design". The "best-effort" posture is still true; cite a still-open tag instead: `NAI-S2-D-PLAYERLOGOUT-BOTH-PATHS` is the cleanest parallel (also permanent, also best-effort-by-design). Or generalize to "friends-server is best-effort by design — slice-1 and slice-2 deviation tags document the posture."

## 8. Cross-world routing — what the test pins

The friends-server registry is keyed by `username37` only. When player A on world 10 sends a PM to player B on world 20:

1. World 10's client calls `PrivateMessage(WorldId=10, Username37=A, TargetUsername37=B, ...)`.
2. Handler calls `h.subs.send(B, FriendsUpdate{PrivateMessage{...}})`.
3. Registry lookup finds B's subscriber regardless of which world B is on — registry doesn't track world for routing.
4. Update goes down B's gRPC stream, which is bound to world 20 (B subscribed with `WorldId=20`).
5. World 20's subscriber dispatches to its `FriendsDispatcher.OnPrivateMessage`.

Test 5.3 pins this explicitly so a future "key registry by (world, player)" refactor would fail loudly.

## 9. Architectural notes resolved (resume questions)

| Question (from resume) | Resolution |
|---|---|
| No-subscription recipient | `subs.send` already silently no-ops (subscriptions.go:85-87). TS parity. |
| Cross-world routing | Registry is `username37`-keyed; works for free. Test 5.3 pins. |
| `socialProtect` rate limiting | World-side `handler_message_private.go:50` sets `p.socialProtect = true` after RPC issued. Slice 4b doesn't touch this. |
| Ignore-list filter | TS does NOT check recipient's ignore list at delivery time (server is dumb-pipe; client-side filters). Slice 4b doesn't either. |
| Option A vs B for in-game emit | **Option B chosen** per resume recommendation: new `NAI-S4B-D-NO-INGAME-PM-EMIT` tag; slog impl unchanged. Mirrors slice 4a's discipline. |

## 10. Deviation tag inventory

### Retired by slice 4b (1)
- `NAI-S1-D-PM-NO-DELIVERY` — server `PrivateMessage` routes via `subs.send`.

### Opened by slice 4b (1)
- `NAI-S4B-D-NO-INGAME-PM-EMIT` — `slogFriendsDispatcher.OnPrivateMessage` logs but does not emit `MESSAGE_PRIVATE` ServerGameProt to recipient's client. Blocked on NAI-182-D5. Mirrors `NAI-S4A-D-NO-INGAME-PACKET-EMIT`.

### Carried forward (not 4b's concern)
- `NAI-S1-D-LAZY-WORLDINIT` (permanent)
- `NAI-S1-D-PLAYERCAP-LOG-ONLY` (slice 4c)
- `NAI-S1-D-PM-NO-PERSISTENCE` (slice 6)
- `NAI-S2-D-PLAYERLOGIN-AT-PROCESSLOGINS` (permanent)
- `NAI-S2-D-PLAYERLOGOUT-BOTH-PATHS` (permanent)
- `NAI-S2-D-PLAYERLOGIN-IGNORES-ACCEPTED` (slice 4c)
- `NAI-S3-D-*` (3, permanent, spec-only)
- `NAI-S4A-D-DROP-ON-FULL` (permanent)
- `NAI-S4A-D-NO-INGAME-PACKET-EMIT` (blocked on NAI-182-D5)
- `NAI-S4A-D-PERPLAYER-NOT-PERWORLD-STREAM` (permanent)

## 11. Risk register

- **R1: `testStream.Send` buffer overflow under 5.1.** `testStream.out` is 32-buffered; initial snapshot (2 messages: empty friendlist + empty ignorelist) plus one PM = 3 messages. Well under cap. Not a risk.
- **R2: Test 5.1 may race against initial snapshot.** The handler sends initial snapshots synchronously on stream open (4a contract) before draining the subscriber channel. Tests must `recvWithin` to consume the 2 initial messages before asserting the PM lands as the 3rd. This is the same pattern slice-4a tests already use.
- **R3: World-side e2e timing.** The 2-second poll in `waitForX` helpers covers a gRPC round-trip on localhost (~ms) plus subscriber dispatch (~ms). No tightness risk.
- **R4: `recordingFriendsDispatcher.private` accessor missing.** The struct has the `private []privateCall` field but no `privateCalls()` accessor like `friendCalls()`. Slice 4b adds one and a `waitForPrivate(t, disp, d, pmId)` helper alongside the existing `waitForFriendlistEntry` family.
- **R5: cross-world test pollutes 100→world-1 + 100→world-20 slots.** Test 5.3 uses a fresh `newTestRepo(t)` per existing pattern; each `Register` call is to a distinct (world, username37) combo. No collision.

## 12. Out of scope (deferred to 4c, 5, 6, 7)

- World action on `PlayerLoginResponse.Accepted` — slice 4c.
- ServerGameProt `MESSAGE_PRIVATE` writer (in-game emit) — NAI-182-D5.
- PM persistence to `private_chat` table — slice 6.
- RELAY_* admin broadcasts — slice 5.
- `Player.session` per-login UUID — slice 7.

## 13. Sibling references

- TS canonical: `Engine-TS/src/server/friend/FriendServer.ts:480-497` (`sendPrivateMessage`).
- TS world-side dispatch: `Engine-TS/src/engine/World.ts:1981-2001` (PRIVATE_MESSAGE branch of `onFriendMessage`).
- Per-player registry pattern: `[[friends-server-slice4a-close]]`.
- World-side dispatcher gating pattern: `NAI-S4A-D-NO-INGAME-PACKET-EMIT` (slice 4a) — 4b's `NAI-S4B-D-NO-INGAME-PM-EMIT` is its exact mirror.
- World-side e2e fixture pattern: `TestFriendsClient_E2E_SubscribeUpdatesStream` in `modules/world/friends_smoke_test.go`.
