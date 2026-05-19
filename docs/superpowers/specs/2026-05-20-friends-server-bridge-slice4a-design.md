# Friends-server bridge — slice 4a design: SubscribeUpdates stream foundation

**Date:** 2026-05-20
**Slice:** 4a of 7 (friends-server bridge arc; slice 4 decomposed into 4a/4b/4c)
**Predecessor:** slice 3 (close commit `d830800c`, retired `NAI-S1-D-INMEMORY-REPO`)
**Closes:** `NAI-S1-D-NO-FOLLOWER-BROADCAST`
**Opens (forward to 4b/4c):** `NAI-S4A-D-NO-INGAME-PACKET-EMIT`, `NAI-S4A-D-DROP-ON-FULL`, `NAI-S4A-D-PERPLAYER-NOT-PERWORLD-STREAM`

## 0. Why slice 4 is decomposed (4a/4b/4c)

The "slice 4" resume note groups four deviation retirements under one slice:

| Tag | Closes by |
|---|---|
| `NAI-S1-D-NO-FOLLOWER-BROADCAST` | Server-side fan-out via SubscribeUpdates stream (this slice, 4a) |
| `NAI-S1-D-PM-NO-DELIVERY` | Server routes PrivateMessage to recipient's stream (slice 4b) |
| `NAI-S1-D-PLAYERCAP-LOG-ONLY` | World acts on PlayerLoginResponse.Accepted (slice 4c) |
| `NAI-S2-D-PLAYERLOGIN-IGNORES-ACCEPTED` | World acts on PlayerLoginResponse.Accepted (slice 4c) |

Decomposition keeps blast radius bounded and matches slice-3's discipline. 4a builds the per-player stream registry + fan-out + world-side subscriber + supervisor — ~60% of the total work. 4b reuses 4a's registry for one new RPC route (~20%). 4c is orthogonal one-line gating in the world's PlayerLogin call site (~20%). Each gets its own spec → plan → execution cycle.

## 1. Forward map (what ships in this slice)

| File | New / changed | Notes |
|---|---|---|
| `modules/friends/subscriptions.go` | **new** | Per-player subscriber registry + send loop |
| `modules/friends/subscriptions_test.go` | **new** | Registry unit tests (register/deregister/dup-kick/drop-on-full) |
| `modules/friends/handler.go` | **changed** | `SubscribeUpdates` real impl; `broadcastWorldToFollowers` + `sendPlayerWorldUpdate` wired into 7 RPC handlers |
| `modules/friends/handler_test.go` | **changed** | broadcast-fanout coverage |
| `modules/friends/repository.go` | **changed** | Add `IsVisibleToMany(viewer, []candidates)` batch helper (or inline pattern; see §5) |
| `modules/friends/repository_test.go` | **changed** | `IsVisibleToMany` tests |
| `modules/friends/friends.go` | **changed** | Wire the subscriber registry through `newGRPCServer` |
| `modules/friends/grpcServer.go` | **changed** | Pass registry into handler |
| `modules/world/friends_subscriber.go` | **new** | Per-player gRPC stream subscriber + exp-backoff supervisor |
| `modules/world/friends_subscriber_test.go` | **new** | Subscriber lifecycle, supervisor, dispatch routing |
| `modules/world/bridges.go` | **changed** | Add `FriendsDispatcher` interface; default impl logs (gated by NAI-S4A-D-NO-INGAME-PACKET-EMIT) |
| `modules/world/server.go` | **changed** | Wire subscriber start/stop into PlayerLogin/PlayerLogout paths that already call `friendsBridge.AddFriend/etc.` |
| `modules/world/friends_smoke_test.go` | **changed** | Extend e2e smoke to assert fan-out reaches the world-side dispatcher |
| `modules/world/friends_client.go` | **changed** | Add `SubscribeUpdates(ctx, req) (stream, error)` method to FriendsClient interface |
| `modules/world/friends_client_fake_test.go` | **changed** | fake impl supports stream |

LOC estimate: ~900 added, ~30 deleted. Larger than slice 3 (~700) because of the world-side supervisor + dispatcher surface area. Still under slice 2 (~1100).

## 2. Stream identity: per-(world, player), not per-world

The slice-1 proto defined `SubscribeUpdates(world_id, username37) → stream FriendsUpdate`. This **already differs from the TS WebSocket-per-world model** (`FriendServer.ts` `socketByWorld[world] = socket`). The proto choice was made at slice-1 spec time; this slice honors it.

| Aspect | TS reference | goscape slice 4a |
|---|---|---|
| Connection topology | One WebSocket per world | One gRPC stream per (world, player) |
| Registry key | `socketByWorld[worldId]` | `subscribers[username37]` |
| `sendPlayerWorldUpdate(viewer, other)` | Looks up viewer's world → world's socket → multiplexes on username37 | Looks up viewer's stream directly |
| Per-player lifecycle | None (world socket is durable) | Stream opens on player login, closes on logout |
| Cross-world fan-out | TS: nodeId-keyed socket lookup, push to single world socket | goscape: registry lookup keyed by follower username37 |

**Deviation tag:** `NAI-S4A-D-PERPLAYER-NOT-PERWORLD-STREAM` — permanent, baked into proto contract. Documented for reviewer traceability; cannot be retired without a proto change.

**Trade-off accepted:** more streams open simultaneously (one per logged-in player vs one per world). For a 2000-player world that's 2000 gRPC streams instead of 1. gRPC handles this cleanly via HTTP/2 multiplexing on a single TCP connection per world. The simpler dispatch pattern (no multiplex layer) is worth the streams.

## 3. Server-side: subscription registry

New file `modules/friends/subscriptions.go`:

```go
package friends

import (
    "log/slog"
    "sync"

    "github.com/zsrv/goscape/pkg/friendspb"
)

// subscriber represents a single open SubscribeUpdates stream for one
// (worldId, username37) pair. The stream is owned by the gRPC handler
// goroutine; ch is the fan-out queue written by RPC handlers.
type subscriber struct {
    worldId    int32
    username37 uint64
    ch         chan *friendspb.FriendsUpdate
    done       chan struct{} // closed when subscription ends
}

// subscriptions is the per-player subscriber registry. All methods are
// goroutine-safe.
type subscriptions struct {
    mu  sync.Mutex
    log *slog.Logger
    by  map[uint64]*subscriber // username37 → subscriber
}

func newSubscriptions(log *slog.Logger) *subscriptions { ... }

// register installs sub under sub.username37. If a prior subscriber
// exists for the same username37, it is kicked (done closed, ch
// closed) before sub replaces it. Returns sub for chaining.
//
// Mirrors TS FriendServer.initializeWorld terminate-then-replace
// (FriendServer.ts:412-419), generalized from per-world to per-player.
func (s *subscriptions) register(sub *subscriber) *subscriber { ... }

// deregister removes sub from the registry IFF it is the currently
// registered subscriber for sub.username37 (the registry may already
// have a newer subscriber if the player rapidly re-logged-in).
func (s *subscriptions) deregister(sub *subscriber) { ... }

// send pushes u to the subscriber for username37 (no-op if none).
// Non-blocking; on full channel, logs warn and drops the update.
//
// NAI-S4A-D-DROP-ON-FULL — TS has no explicit backpressure (WebSocket
// internal buffer); we choose drop-newest over block to keep RPC
// handlers from stalling.
func (s *subscriptions) send(username37 uint64, u *friendspb.FriendsUpdate) { ... }
```

Buffer size 64 chosen as a reasonable default; can revisit if benchmarks show drops in realistic load. Channel size is a constant in this file, not config-exposed.

## 4. Server-side: SubscribeUpdates handler

`handler.SubscribeUpdates` (in `handler.go`, NOT `grpcServer.go` — handler.go owns all method bodies for `friendspb.FriendsServiceServer`):

```go
func (h *handler) SubscribeUpdates(req *friendspb.SubscribeUpdatesRequest, stream friendspb.FriendsService_SubscribeUpdatesServer) error {
    sub := &subscriber{
        worldId:    req.WorldId,
        username37: req.Username37,
        ch:         make(chan *friendspb.FriendsUpdate, 64),
        done:       make(chan struct{}),
    }
    h.subs.register(sub)
    defer h.subs.deregister(sub)

    // Send initial snapshots (TS sendFriendsListToPlayer +
    // sendIgnoreListToPlayer at FriendServer.ts:138-139, but TS does
    // this on PLAYER_LOGIN; goscape does it on Subscribe because the
    // proto decouples login from stream-open).
    if err := h.sendInitialFriendlist(stream.Context(), sub); err != nil {
        return err
    }
    if err := h.sendInitialIgnorelist(stream.Context(), sub); err != nil {
        return err
    }

    for {
        select {
        case <-stream.Context().Done():
            return nil
        case <-sub.done:
            return nil
        case u, ok := <-sub.ch:
            if !ok {
                return nil
            }
            if err := stream.Send(u); err != nil {
                return err
            }
        }
    }
}
```

`sendInitialFriendlist`: calls `repo.GetFriends(ctx, username37)`; for each friend, looks up the friend's current world (via `repo.GetWorld`); builds one FriendlistUpdate with all entries, applying `IsVisibleTo` per entry. Mirrors TS `sendFriendsListToPlayer` (FriendServer.ts:421-431).

`sendInitialIgnorelist`: calls `repo.GetIgnores(ctx, username37)`; builds one IgnorelistUpdate. Mirrors TS `sendIgnoreListToPlayer` (FriendServer.ts:433-443).

## 5. Server-side: broadcastWorldToFollowers + sendPlayerWorldUpdate

Two new private methods on `handler`:

```go
// broadcastWorldToFollowers fans out a one-entry FriendlistUpdate to
// each of `other`'s followers that has an open subscription. Mirrors
// TS FriendServer.broadcastWorldToFollowers (FriendServer.ts:445-451).
//
// Concurrency: snapshot followers + their friend sets under one
// repo lock cycle to avoid holding mu across stream sends.
func (h *handler) broadcastWorldToFollowers(ctx context.Context, other uint64) error {
    followers, err := h.repo.GetFollowers(ctx, other)
    if err != nil {
        return err
    }
    otherWorld := h.repo.GetWorld(other) // 0 if offline
    visibility, err := h.repo.IsVisibleToMany(ctx, followers, other)
    if err != nil {
        return err
    }
    for _, viewer := range followers {
        worldForViewer := int32(0)
        if visibility[viewer] {
            worldForViewer = otherWorld
        }
        h.subs.send(viewer, &friendspb.FriendsUpdate{
            Update: &friendspb.FriendsUpdate_Friendlist{
                Friendlist: &friendspb.FriendlistUpdate{
                    Entries: []*friendspb.FriendEntry{{
                        WorldId:    worldForViewer,
                        Username37: other,
                    }},
                },
            },
        })
    }
    return nil
}

// sendPlayerWorldUpdate pushes a single-friend update to viewer.
// Mirrors TS FriendServer.sendPlayerWorldUpdate (FriendServer.ts:462-478).
// Called only by FriendlistAdd to notify the adder of the new friend's
// current world.
func (h *handler) sendPlayerWorldUpdate(ctx context.Context, viewer, other uint64) error { ... }
```

### Wiring into handler methods

| RPC | TS line | Calls in goscape slice 4a |
|---|---|---|
| `PlayerLogin` | FS.ts:128-142 | `sendInitial*` not called from here (initial snapshot is Subscribe-side); broadcastWorldToFollowers(username37) after Register |
| `PlayerLogout` | FS.ts:143-161 | broadcastWorldToFollowers(username37) after Unregister |
| `ChatSetMode` | FS.ts:162-184 | broadcastWorldToFollowers(username37) after SetChatMode |
| `FriendlistAdd` | FS.ts:185-204 | sendPlayerWorldUpdate(adder, target) THEN broadcastWorldToFollowers(adder) |
| `FriendlistDel` | FS.ts:205-221 | broadcastWorldToFollowers(remover) |
| `IgnorelistAdd` | FS.ts:222-238 | broadcastWorldToFollowers(adder) |
| `IgnorelistDel` | FS.ts:239-255 | broadcastWorldToFollowers(remover) |

All seven wirings retire `NAI-S1-D-NO-FOLLOWER-BROADCAST`.

### IsVisibleToMany batch helper

The existing `IsVisibleTo(viewer, other) (bool, error)` does (per call): RLock → snapshot privateChat / staff state → RUnlock → SQL `GetFriends(viewer)` → in-memory intersection. For broadcast fan-out, calling N times means N SQL round trips — the N+1 risk the resume note flagged.

New helper `IsVisibleToMany(ctx, viewers []uint64, other uint64) (map[uint64]bool, error)`:
- Single locked snapshot of all viewers' `privateChat`/`staff` rows (and `other`'s).
- Single SQL `SELECT owner_username37 FROM friendlist WHERE profile = ? AND target_username37 = ? AND owner_username37 IN (?,?,...)` — gives "which viewers have `other` as a friend".
- In-memory combine: for each viewer, apply same predicate as scalar `IsVisibleTo`.

If viewers is empty, returns `(map{}, nil)` (no SQL). If len(viewers)==1, may still call the batch path (one-row IN is fine).

## 6. World-side: subscriber + supervisor

New file `modules/world/friends_subscriber.go`:

```go
// friendsSubscriber manages one player's SubscribeUpdates stream.
// Started on player login (after friendsBridge.AddFriend etc. are
// wired); stopped on player logout/disconnect.
//
// Reconnect supervisor mirrors [[content-watcher-auto-restart-close]]:
// exp backoff 1s→30s, reset@60s steady. Stops on logout (ctx canceled).
type friendsSubscriber struct {
    client     FriendsClient
    worldID    int32
    username37 uint64
    dispatcher FriendsDispatcher
    log        *slog.Logger
}

// start launches the subscription supervisor goroutine. Returns
// immediately. The goroutine exits when ctx is canceled.
func (s *friendsSubscriber) start(ctx context.Context) {
    go s.supervise(ctx)
}

func (s *friendsSubscriber) supervise(ctx context.Context) {
    backoff := time.Second
    lastFailure := time.Time{}
    for {
        if ctx.Err() != nil {
            return
        }
        runStart := time.Now()
        err := s.runOnce(ctx)
        if ctx.Err() != nil {
            return
        }
        // Reset backoff if last run was steady for ≥60s.
        if time.Since(lastFailure) > 60*time.Second && time.Since(runStart) > 60*time.Second {
            backoff = time.Second
        }
        lastFailure = time.Now()
        s.log.Warn("friends subscriber disconnected, reconnecting",
            slog.Uint64("username37", s.username37),
            slog.Duration("backoff", backoff),
            slog.Any("err", err))
        select {
        case <-ctx.Done():
            return
        case <-time.After(backoff):
        }
        backoff = min(2*backoff, 30*time.Second)
    }
}

func (s *friendsSubscriber) runOnce(ctx context.Context) error {
    stream, err := s.client.SubscribeUpdates(ctx, &friendspb.SubscribeUpdatesRequest{
        WorldId:    s.worldID,
        Username37: s.username37,
    })
    if err != nil {
        return err
    }
    for {
        u, err := stream.Recv()
        if err != nil {
            return err
        }
        s.dispatch(u)
    }
}

func (s *friendsSubscriber) dispatch(u *friendspb.FriendsUpdate) {
    switch upd := u.Update.(type) {
    case *friendspb.FriendsUpdate_Friendlist:
        s.dispatcher.OnFriendlistUpdate(s.username37, upd.Friendlist.Entries)
    case *friendspb.FriendsUpdate_Ignorelist:
        s.dispatcher.OnIgnorelistUpdate(s.username37, upd.Ignorelist.Username37)
    case *friendspb.FriendsUpdate_PrivateMessage:
        // Slice 4b will wire this; 4a logs unknown shape.
        s.log.Debug("friends subscriber received PrivateMessageDelivery (4b territory)")
    }
}
```

### FriendsClient interface extension

`modules/world/friends_client.go`: add stream-returning method.

```go
type FriendsClient interface {
    // ... existing methods ...
    SubscribeUpdates(ctx context.Context, req *friendspb.SubscribeUpdatesRequest) (friendspb.FriendsService_SubscribeUpdatesClient, error)
}
```

`grpcFriendsClient.SubscribeUpdates` delegates to `c.client.SubscribeUpdates(ctx, req)`. Unlike the other RPCs in this file, this one returns the error (cannot be fire-and-forget; the supervisor needs to react).

`fakeFriendsClient` (test): returns a controllable stream (chan-backed `FriendsService_SubscribeUpdatesClient` impl).

### FriendsDispatcher interface

`modules/world/bridges.go`: new interface alongside `FriendsBridge`.

```go
type FriendsDispatcher interface {
    OnFriendlistUpdate(viewer uint64, entries []*friendspb.FriendEntry)
    OnIgnorelistUpdate(viewer uint64, ignored []uint64)
    OnPrivateMessage(target uint64, from uint64, staffLvl int32, pmId uint32, chat string)
}
```

Default production impl: `slogFriendsDispatcher` — logs each event at Debug, does NOT emit ServerGameProt packets to the player's client connection. **Deviation tag:** `NAI-S4A-D-NO-INGAME-PACKET-EMIT` — gated on NAI-182-D5 (the "social cluster" ServerGameProt deferral mentioned at `tick.go:226`). When NAI-182-D5 retires, the dispatcher impl will replace its log with `player.write(UpdateFriendList(...))` / `UpdateIgnoreList(...)` / `MessagePrivate(...)` calls.

Test impl: `recordingFriendsDispatcher` captures events for assertions.

### Lifecycle wiring

Subscriber start/stop hooks into the same call sites as slice 2's `friendsBridge.AddFriend` wiring — wherever the world calls `friendsBridge` for a per-player social RPC, slice 4a also owns the subscriber lifecycle for that player. Concretely:

- **Start:** at the same point that triggers `friendsClient.PlayerLogin` (after the world admits the player) — start the subscriber's `supervise` goroutine with the player's ctx.
- **Stop:** at the same point that triggers `friendsClient.PlayerLogout` — cancel the player's ctx; supervisor exits without reconnect.

This may be a per-`Player` field `friendsSub *friendsSubscriber` initialized in PlayerLogin and torn down in PlayerLogout. Exact wiring inspected during plan-write — the slice-2 call sites are the anchors.

## 7. Testing

### Server-side
- `subscriptions_test.go`: register/deregister/dup-kick (verify old subscriber's done is closed); drop-on-full (fill 64-slot buffer, 65th send drops + logs); register-deregister-race under `-race`.
- `handler_test.go`: broadcastWorldToFollowers integration — A and B are mutual friends both online; A logs in; B's subscriber sees a FriendlistUpdate with A's worldId; visibility=0 when A has privateChat=OFF; visibility=A's world when privateChat=ON or privateChat=FRIENDS.
- `handler_test.go`: SubscribeUpdates RPC integration — open stream, assert initial UPDATE_FRIENDLIST + UPDATE_IGNORELIST arrive; trigger a friend's login; assert FriendlistUpdate arrives.

### World-side
- `friends_subscriber_test.go`: supervisor backoff (mock client returns connection-refused N times; assert backoff sequence 1s→2s→...→30s and reset after 60s steady; verify ctx.Done causes clean exit without reconnect).
- `friends_subscriber_test.go`: dispatch routing (controllable fake stream pushes FriendlistUpdate / IgnorelistUpdate; recordingFriendsDispatcher captures; assert correct routing).

### End-to-end smoke (extends `friends_smoke_test.go`)
- Boot in-process Friends module with `SQLiteDSN: filepath.Join(t.TempDir(), "friends.db")`.
- Create players A and B. A adds B as friend (via gRPC FriendlistAdd).
- A opens SubscribeUpdates stream (via world-side subscriber wrapped around a real FriendsClient).
- B PlayerLogin.
- Assert A's recordingFriendsDispatcher receives FriendlistUpdate `[{WorldId: 1, Username37: B}]` within 1s.
- B PlayerLogout.
- Assert A's dispatcher receives FriendlistUpdate `[{WorldId: 0, Username37: B}]`.

### Gates
- `go test -race ./...` clean.
- `goscape-cli smoke-pack` 12 OK / 0 ERR holds.

## 8. Open architectural notes resolved

| Resume-note question | Answer |
|---|---|
| Stream identity (per-world vs per-(world, player)) | Per-(world, player); proto contract |
| Backpressure on slow subscriber | Drop-newest with Warn log; 64-slot buffer per subscriber. `NAI-S4A-D-DROP-ON-FULL`. |
| IsVisibleTo N+1 risk | `IsVisibleToMany(ctx, viewers, other)` batch helper; one SQL `IN` query + in-memory combine |
| `accepted=false` semantics | Slice 4c territory; out of slice 4a scope |

## 9. Deviation tag inventory

### Retired by slice 4a (1)
- `NAI-S1-D-NO-FOLLOWER-BROADCAST` — broadcastWorldToFollowers + sendPlayerWorldUpdate wired into 7 RPC handlers.

### Opened by slice 4a (3)
- `NAI-S4A-D-PERPLAYER-NOT-PERWORLD-STREAM` — permanent, proto contract.
- `NAI-S4A-D-DROP-ON-FULL` — permanent design choice for stream backpressure.
- `NAI-S4A-D-NO-INGAME-PACKET-EMIT` — world-side dispatcher logs but does not write ServerGameProt UPDATE_FRIENDLIST / UPDATE_IGNORELIST to the player's client. Blocked on NAI-182-D5 (social-cluster ServerGameProt port). Retires when NAI-182-D5 retires.

### Carried forward (from slices 1-2, not 4a's concern)
- `NAI-S1-D-LAZY-WORLDINIT` (permanent)
- `NAI-S1-D-PM-NO-DELIVERY` (slice 4b)
- `NAI-S1-D-PM-NO-PERSISTENCE` (slice 6)
- `NAI-S1-D-PLAYERCAP-LOG-ONLY` (slice 4c)
- `NAI-S2-D-PLAYERLOGIN-AT-PROCESSLOGINS` (permanent)
- `NAI-S2-D-PLAYERLOGOUT-BOTH-PATHS` (permanent)
- `NAI-S2-D-PLAYERLOGIN-IGNORES-ACCEPTED` (slice 4c)
- `NAI-S3-D-*` (3, permanent, spec-only)

## 10. Risk register

- **R1: Player-Subscribe lifecycle ↔ PlayerLogin ordering.** The friends-server's `broadcastWorldToFollowers` triggered by A's PlayerLogin only fires updates to followers whose Subscribe stream is already open. If A logs in before A's followers' subscribers connect (race window during cold-start), the followers miss the login update. **Mitigation:** the proto already sends the initial UPDATE_FRIENDLIST snapshot on Subscribe arrival, so followers always see the current state on stream open. The race window only loses *transition* events, which is benign — the follower's next snapshot or next broadcast is consistent.
- **R2: stream.Send back-pressure when many subscribers slow-drain at once.** Drop-on-full with Warn log; if observed in load testing, raise buffer or add per-subscriber drop metrics.
- **R3: Repeated rapid logout/login (e.g., bot reconnect) creates churn on dup-kick path.** The kick path is O(1) (close+replace under registry mu); no real risk. Verified under `-race` registry test.
- **R4: Cross-package import cycle.** `modules/world` already imports `pkg/friendspb`; slice 4a doesn't introduce new dependencies. No cycle risk.
- **R5: e2e smoke must be `t.Parallel()`-safe.** Existing `friends_smoke_test.go` uses `t.TempDir()` for DB isolation; the new test follows the same shape with its own gRPC port (server picks port 0).

## 11. Out of scope (deferred to 4b, 4c, or 5+)

- Server routing of `PrivateMessage` to recipient's stream — slice 4b.
- World action on PlayerLoginResponse.Accepted — slice 4c.
- ServerGameProt UPDATE_FRIENDLIST / UPDATE_IGNORELIST / MESSAGE_PRIVATE writers — NAI-182-D5 / social cluster.
- RELAY_* admin broadcasts — slice 5.
- PUBLIC_CHAT_LOG / private_chat persistence — slice 6.
- `Player.session` per-login UUID — slice 7.

## 12. Sibling references

- TS canonical: `Engine-TS/src/server/friend/FriendServer.ts:62-498` (FriendServer class); `Engine-TS/src/engine/World.ts:1951-2000` (world-side onFriendMessage dispatch).
- Supervisor pattern: `[[content-watcher-auto-restart-close]]` (exp backoff 1s→30s, reset@60s).
- Per-player goroutine + ctx lifecycle: `modules/login/grpc_*` (login slice's similar shape).
- Test-impl seam pattern: `friends_client_fake_test.go` (slice 2's two-seam shape).
