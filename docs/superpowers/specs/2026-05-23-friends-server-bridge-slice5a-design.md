# Friends-server bridge — slice 5a design: RELAY_* transport foundation

**Date:** 2026-05-23
**Slice:** 5a of 7 (friends-server bridge arc; slice 5 decomposed into 5a/5b)
**Predecessor:** slice 4c (close commit `ef8cb009`, retired `NAI-S1-D-PLAYERCAP-LOG-ONLY` + `NAI-S2-D-PLAYERLOGIN-IGNORES-ACCEPTED`; see `[[friends-server-slice4c-close]]`)
**Closes:** none (foundation only — slice 5b retires the slice-1/2 inventory; slice 5 was greenfield as of slice-4c close)
**Opens:** `NAI-S5A-D-PERWORLD-EVENTS-STREAM-SEPARATE`, `NAI-S5A-D-DISPATCHER-NO-ACTION`, `NAI-S5A-D-NO-ADMIN-AUTH-AT-SERVER`, `NAI-S5A-D-WORLDEVENTS-DROP-ON-FULL`

## 0. Why slice 5 is decomposed (5a/5b)

The "slice 5" resume note groups nine RELAY_* admin opcodes under one umbrella. The total work is ~1300-1600 LOC if shipped in one slice — larger than slice 4a (~1100). Decomposition matches the 4a/4b/4c discipline.

| Sub-slice | Scope | Retires |
|---|---|---|
| **5a (this slice)** | Proto + outbound RPC surface + per-world stream + slog-only inbound dispatcher | none — foundation |
| **5b** | World-state action handlers (SHUTDOWN, RELOAD, CLEARLOGINS, CLEARLOGOUTS, MUTE, KICK); defer BROADCAST/TRACK behind `NAI-S5-D-NO-INGAME-EMIT-*`, defer QUEUESCRIPT behind `NAI-S5-D-NO-RUNESCRIPT-RUNTIME` | each opcode that wires retires its own NAI-S5A-D-DISPATCHER-NO-ACTION sub-bullet |

5a builds the entire transport — proto, server-side fan-out, world-side outbound bridge, inbound stream subscriber, dispatcher interface. 5b layers action behavior onto the existing dispatcher. Each gets its own spec → plan → execution cycle.

## 1. Forward map (what ships in this slice)

| File | New / changed | Notes |
|---|---|---|
| `proto/friends/friends.proto` | **changed** | 9 unary RPCs + 1 stream RPC + 9 Request messages + `WorldEvent` oneof + 9 event variants + `SubscribeWorldEventsRequest` |
| `pkg/friendspb/friends.pb.go` + `friends_grpc.pb.go` | **regenerated** | `make proto` (see §10 follow-up: `.PHONY` fix) |
| `modules/friends/world_subscriptions.go` | **new** | Per-WORLD subscriber registry (single subscriber per worldId; mirrors `subscriptions.go` shape but keyed by `int32` worldId) |
| `modules/friends/world_subscriptions_test.go` | **new** | Registry unit tests (register/deregister/dup-kick/drop-on-full) |
| `modules/friends/handler.go` | **changed** | 9 new RelayXxx handler methods + `SubscribeWorldEvents` real impl |
| `modules/friends/handler_test.go` | **changed** | 9 unit tests + SubscribeWorldEvents stream test |
| `modules/friends/friends.go` | **changed** | Wire the per-world registry through `newGRPCServer` |
| `modules/friends/server.go` | **changed** | Pass per-world registry into handler |
| `modules/world/friends_client.go` | **changed** | `FriendsClient` interface gains 9 outbound `Relay*` methods + 1 `SubscribeWorldEvents(ctx, req) (stream, error)` method |
| `modules/world/friends_client_fake_test.go` | **changed** | `fakeFriendsClient` impls; per-method capture channels |
| `modules/world/friends_client_test.go` | **changed** | Per-method gRPC failure-logging tests |
| `modules/world/bridges.go` | **changed** | New `FriendsAdminBridge` interface (9 outbound methods) + new `WorldEventsDispatcher` interface (9 inbound methods) + `slogWorldEventsDispatcher` default impl + `defaultFriendsAdminBridge` constructor (collapses nil `FriendsClient` → `noopAdminBridge{}`) |
| `modules/world/admin_bridge.go` | **new** | `grpcFriendsAdminBridge` impl + `noopAdminBridge{}` (extracted file; mirrors `friendsBridge` shape) |
| `modules/world/admin_bridge_test.go` | **new** | Per-method bridge pin tests (table-driven mapping bridge→client) |
| `modules/world/world_events_dispatcher_test.go` | **new** | `slogWorldEventsDispatcher` per-method log-pin tests |
| `modules/world/world_events_subscriber.go` | **new** | Per-WORLD gRPC stream subscriber + exp-backoff supervisor (mirrors `friends_subscriber.go`) |
| `modules/world/world_events_subscriber_test.go` | **new** | Subscriber lifecycle + dispatch routing tests |
| `modules/world/server.go` | **changed** | `Server` gains `friendsAdminBridge` + `worldEventsDispatcher` + per-world subscriber lifecycle (start at Server init, cancel at Server close) |
| `modules/world/friends_smoke_test.go` | **changed** | New `TestFriendsClient_E2E_RelayWorldEventsRoundTrip` — two SubscribeWorldEvents subscribers (different worldIds), issue Relay* RPCs cross-world, assert dispatch arrives at the correct target |
| `Makefile` | **changed** | Fix `proto:` target — add to `.PHONY` (open since slice 1; first slice to actually regenerate the proto) |

LOC estimate: ~900 added, ~20 deleted.

## 2. Stream identity: per-WORLD, NOT per-(world, player)

Slice 5a adds a **second**, independent stream RPC: `SubscribeWorldEvents(world_id) returns (stream WorldEvent)`. This sits **alongside** the slice-4a `SubscribeUpdates(world_id, username37) returns (stream FriendsUpdate)`, which remains per-(world, player).

**Why not reuse `SubscribeUpdates`?**

`FriendsUpdate` events are per-PLAYER (friendlist updates for player X, PM addressed to player Y). `WorldEvent` events are per-WORLD (kick player Z from world A, shut down world B). The grain is fundamentally different:

- **Per-player stream + extend `FriendsUpdate` to carry world events:** every WorldEvent fires for N players on world A (the world has N open per-player streams). Either we send N copies (wasteful + requires per-player dedup at action layer) or we pick "the canonical subscriber" (race: which player owns the world push? what if they log out mid-push?). Both are bad. Rejected.
- **Reshape `SubscribeUpdates` per-world:** retires `NAI-S4A-D-PERPLAYER-NOT-PERWORLD-STREAM` but rewrites slice 4a + 4b registry + dispatcher + ~12 tests. Out of scope for 5a; would dwarf the actual slice-5 work.
- **Separate `SubscribeWorldEvents` stream:** clean grain — one subscriber per world process, owned by `world.Server`, not tied to player lifecycle. Additive, isolates risk. **Chosen.**

**Deviation tag opened:** `NAI-S5A-D-PERWORLD-EVENTS-STREAM-SEPARATE` — the goscape friends transport now has **two parallel stream RPCs** where TS has one socket. Permanent; cannot be retired without unifying the two streams (which would in turn require retiring `NAI-S4A-D-PERPLAYER-NOT-PERWORLD-STREAM`). Reviewer traceability only.

| Aspect | TS reference | goscape slice 4a | goscape slice 5a |
|---|---|---|---|
| Topology | One WebSocket per world | One stream per (world, player) | One stream per world (this slice) |
| Registry key | `socketByWorld[worldId]` | `subscribers[username37]` | `worldSubscribers[worldId]` |
| Lifecycle | World socket is durable | Per-player open/close | Per-world, owned by `world.Server`, durable for the world process |
| Dropped on full buffer | (no buffer in TS) | log warn, drop newest | log warn, drop newest (same posture) |

## 3. Proto changes

### 3.1 New unary RPCs

Add to `service FriendsService` in `proto/friends/friends.proto`:

```proto
// Cross-world admin relay (slice 5). Each RPC accepts a target_world_id;
// the server forwards a WorldEvent to that world's SubscribeWorldEvents
// stream. No-op if no world is subscribed for target_world_id (matches
// TS FriendServer.ts:298-302 `if (typeof this.socketByWorld[nodeId] !== 'undefined')`).
// Slice 5a accepts and routes; slice 5b applies world-state effects.
rpc RelayMute(RelayMuteRequest)                 returns (google.protobuf.Empty);
rpc RelayKick(RelayKickRequest)                 returns (google.protobuf.Empty);
rpc RelayShutdown(RelayShutdownRequest)         returns (google.protobuf.Empty);
rpc RelayBroadcast(RelayBroadcastRequest)       returns (google.protobuf.Empty);
rpc RelayTrack(RelayTrackRequest)               returns (google.protobuf.Empty);
rpc RelayReload(RelayReloadRequest)             returns (google.protobuf.Empty);
rpc RelayClearLogins(RelayClearLoginsRequest)   returns (google.protobuf.Empty);
rpc RelayClearLogouts(RelayClearLogoutsRequest) returns (google.protobuf.Empty);
rpc RelayQueueScript(RelayQueueScriptRequest)   returns (google.protobuf.Empty);

// Server -> world push for cross-world admin events. One subscriber per
// world (owned by world.Server). Slice 5a opens the stream; slice 5b
// wires dispatcher actions onto inbound events.
rpc SubscribeWorldEvents(SubscribeWorldEventsRequest) returns (stream WorldEvent);
```

### 3.2 New request messages

Field layout mirrors TS `FriendServer.ts:298-396` payloads. `target_world_id` replaces TS `nodeId`. Each request carries only what TS sends.

```proto
// All Relay*Request messages: target_world_id is the world that should
// RECEIVE the event. The friends-server forwards by target_world_id;
// no auth check (NAI-S5A-D-NO-ADMIN-AUTH-AT-SERVER).

message RelayMuteRequest {
  int32  target_world_id = 1;
  uint64 username37      = 2;
  // Mute expiry as epoch milliseconds (matches TS `muted_until: number`).
  // 0 = unmute. Negative = permanent (matches existing modules/login PlayerMute).
  int64  muted_until_ms  = 3;
}

message RelayKickRequest {
  int32  target_world_id = 1;
  uint64 username37      = 2;
}

message RelayShutdownRequest {
  int32  target_world_id = 1;
  // Shutdown countdown in ticks (TS `duration`).
  int32  duration_ticks  = 2;
}

message RelayBroadcastRequest {
  int32  target_world_id = 1;
  // Game-wide chat broadcast text (TS `broadcast` → `message`).
  string message         = 2;
}

message RelayTrackRequest {
  int32  target_world_id = 1;
  uint64 username37      = 2;
  // TS-faithful `state` (FriendServer.ts:348 `const { nodeId, username, state }`).
  // TS field is untyped; goscape pins as int32 so slice 5b can interpret per
  // the anti-cheat tracking subsystem (logger_bridge.go:55-58). Bool would
  // over-narrow; defer interpretation to 5b.
  int32  state           = 3;
}

message RelayReloadRequest {
  int32  target_world_id = 1;
}

message RelayClearLoginsRequest {
  int32  target_world_id = 1;
}

message RelayClearLogoutsRequest {
  int32  target_world_id = 1;
}

message RelayQueueScriptRequest {
  int32  target_world_id = 1;
  string script_name     = 2;
  uint64 username37      = 3;
}

message SubscribeWorldEventsRequest {
  int32 world_id = 1;
}
```

### 3.3 New WorldEvent oneof

```proto
message WorldEvent {
  oneof event {
    MuteEvent           mute            = 1;
    KickEvent           kick            = 2;
    ShutdownEvent       shutdown        = 3;
    BroadcastEvent      broadcast       = 4;
    TrackEvent          track           = 5;
    ReloadEvent         reload          = 6;
    ClearLoginsEvent    clear_logins    = 7;
    ClearLogoutsEvent   clear_logouts   = 8;
    QueueScriptEvent    queue_script    = 9;
  }
}

message MuteEvent {
  uint64 username37     = 1;
  int64  muted_until_ms = 2;
}

message KickEvent {
  uint64 username37 = 1;
}

message ShutdownEvent {
  int32 duration_ticks = 1;
}

message BroadcastEvent {
  string message = 1;
}

message TrackEvent {
  uint64 username37 = 1;
  int32  state      = 2;
}

message ReloadEvent {}

message ClearLoginsEvent {}

message ClearLogoutsEvent {}

message QueueScriptEvent {
  string script_name = 1;
  uint64 username37  = 2;
}
```

### 3.4 Why event variants are NOT just aliased to the Request types

Each `Relay*Request` carries `target_world_id` — irrelevant to the receiving world (which already knows its own ID). Each `*Event` strips that field. Cleaner contract: requests describe "to where", events describe "what". One small benefit: prevents a buggy server impl from accidentally exposing the routing field to receivers.

## 4. Server-side: per-world subscription registry

New file `modules/friends/world_subscriptions.go`. Shape mirrors `subscriptions.go` (slice 4a) but the key is `int32` (world ID) instead of `uint64` (username37), and the buffer is shared with the same posture (`NAI-S4A-D-DROP-ON-FULL` is mirrored as `NAI-S5A-D-WORLDEVENTS-DROP-ON-FULL` — same posture, separate tag because the channel is different).

```go
package friends

import (
    "log/slog"
    "sync"

    "github.com/zsrv/goscape/pkg/friendspb"
)

// worldSubscriberBufferSize is the per-world-subscriber channel buffer.
// Same posture as subscriberBufferSize but separate constant in case
// admin-burst rate differs from per-player update rate.
//
// NAI-S5A-D-WORLDEVENTS-DROP-ON-FULL — overflowing the buffer drops the
// newest event with a Warn log instead of blocking the RPC handler.
const worldSubscriberBufferSize = 64

type worldSubscriber struct {
    worldId int32
    ch      chan *friendspb.WorldEvent
    done    chan struct{}
}

func newWorldSubscriber(worldId int32) *worldSubscriber {
    return &worldSubscriber{
        worldId: worldId,
        ch:      make(chan *friendspb.WorldEvent, worldSubscriberBufferSize),
        done:    make(chan struct{}),
    }
}

// worldSubscriptions is the per-world subscriber registry. All methods
// are goroutine-safe. Exactly one subscriber per worldId; re-subscribe
// kicks the prior (matches TS FriendServer.initializeWorld at
// FriendServer.ts:412-419 — `socket.terminate()` on re-WORLD_CONNECT).
type worldSubscriptions struct {
    mu  sync.Mutex
    by  map[int32]*worldSubscriber
    log *slog.Logger
}

func newWorldSubscriptions(log *slog.Logger) *worldSubscriptions {
    return &worldSubscriptions{by: make(map[int32]*worldSubscriber), log: log}
}

func (s *worldSubscriptions) register(sub *worldSubscriber)    { /* terminate-then-replace by worldId */ }
func (s *worldSubscriptions) deregister(sub *worldSubscriber)  { /* identity-checked delete */ }
func (s *worldSubscriptions) send(worldId int32, ev *friendspb.WorldEvent) {
    // Non-blocking; drop-newest with warn log on full channel.
}
```

## 5. Server-side: handler methods

9 new methods on `*handler` (`modules/friends/handler.go`), one per RELAY opcode. Each is a 4-liner:

```go
// RelayKick forwards a kick command to target_world_id's subscriber.
// No-op if no world is subscribed (matches TS FriendServer.ts:312-321).
// No auth check at this layer (NAI-S5A-D-NO-ADMIN-AUTH-AT-SERVER).
func (h *handler) RelayKick(ctx context.Context, req *friendspb.RelayKickRequest) (*emptypb.Empty, error) {
    h.worldSubs.send(req.TargetWorldId, &friendspb.WorldEvent{
        Event: &friendspb.WorldEvent_Kick{Kick: &friendspb.KickEvent{Username37: req.Username37}},
    })
    return &emptypb.Empty{}, nil
}
```

Each handler swallows the silent-drop case (TS `if (typeof socketByWorld[nodeId] !== 'undefined')`). No error returned on missing world subscriber — that's the TS-faithful posture.

### 5.1 SubscribeWorldEvents stream handler

Mirrors `SubscribeUpdates` in handler.go (slice 4a). Skeleton:

```go
func (h *handler) SubscribeWorldEvents(req *friendspb.SubscribeWorldEventsRequest, stream friendspb.FriendsService_SubscribeWorldEventsServer) error {
    sub := newWorldSubscriber(req.WorldId)
    h.worldSubs.register(sub)
    defer h.worldSubs.deregister(sub)

    for {
        select {
        case <-stream.Context().Done():
            return stream.Context().Err()
        case <-sub.done:
            return nil // kicked by a newer subscriber for the same worldId
        case ev := <-sub.ch:
            if err := stream.Send(ev); err != nil {
                return err
            }
        }
    }
}
```

## 6. World-side: outbound bridge + inbound dispatcher + per-world subscriber

### 6.1 `FriendsClient` interface additions

`modules/world/friends_client.go`:

```go
type FriendsClient interface {
    // ... existing slice 1-4c methods ...

    // RELAY_* (slice 5a). All fire-and-forget; errors are logged.
    RelayMute(ctx context.Context, req *friendspb.RelayMuteRequest)
    RelayKick(ctx context.Context, req *friendspb.RelayKickRequest)
    RelayShutdown(ctx context.Context, req *friendspb.RelayShutdownRequest)
    RelayBroadcast(ctx context.Context, req *friendspb.RelayBroadcastRequest)
    RelayTrack(ctx context.Context, req *friendspb.RelayTrackRequest)
    RelayReload(ctx context.Context, req *friendspb.RelayReloadRequest)
    RelayClearLogins(ctx context.Context, req *friendspb.RelayClearLoginsRequest)
    RelayClearLogouts(ctx context.Context, req *friendspb.RelayClearLogoutsRequest)
    RelayQueueScript(ctx context.Context, req *friendspb.RelayQueueScriptRequest)

    // SubscribeWorldEvents opens a per-world server-streaming RPC for
    // admin push events. Like SubscribeUpdates, this one returns the
    // error so the supervisor can drive reconnect backoff.
    SubscribeWorldEvents(ctx context.Context, req *friendspb.SubscribeWorldEventsRequest) (friendspb.FriendsService_SubscribeWorldEventsClient, error)
}
```

`grpcFriendsClient` impls follow the existing fire-and-forget pattern for the unaries:

```go
func (c *grpcFriendsClient) RelayKick(ctx context.Context, req *friendspb.RelayKickRequest) {
    if _, err := c.client.RelayKick(ctx, req); err != nil {
        c.log.Warn("RelayKick RPC failed", slog.Int("target_world_id", int(req.TargetWorldId)), slog.Any("err", err))
    }
}
```

### 6.2 `FriendsAdminBridge` interface

New interface in `modules/world/bridges.go` (or extracted to `admin_bridge.go`):

```go
// FriendsAdminBridge mirrors TS World.friendThread.postMessage(...) for
// cross-world RELAY_* admin commands. Production impl is
// grpcFriendsAdminBridge; wired by NewServer via defaultFriendsAdminBridge.
//
// The bridge is the surface for admin-action code paths to issue
// cross-world commands. Slice 5a exposes the surface; slice 5b layers
// dispatcher actions on the receiving side. Admin chat-command wiring
// (::kick, ::mute, etc.) is future integration work — slice 5 does not
// touch existing cheat handlers.
type FriendsAdminBridge interface {
    Mute(targetWorldID int32, username37 uint64, mutedUntilMs int64)
    Kick(targetWorldID int32, username37 uint64)
    Shutdown(targetWorldID int32, durationTicks int32)
    Broadcast(targetWorldID int32, message string)
    Track(targetWorldID int32, username37 uint64, state int32)
    Reload(targetWorldID int32)
    ClearLogins(targetWorldID int32)
    ClearLogouts(targetWorldID int32)
    QueueScript(targetWorldID int32, scriptName string, username37 uint64)
}
```

Production impl `grpcFriendsAdminBridge` is a thin shim that fans out to the corresponding `FriendsClient.Relay*` call with `context.Background()`. The shim collapses the `FriendsClient` nil case (when `FriendsServerEnabled=false`) to a `noopAdminBridge{}` static — same pattern as `friendsBridge` (`modules/world/bridges.go:55-58` precedent).

### 6.3 `WorldEventsDispatcher` interface (slog-only default)

```go
// WorldEventsDispatcher is the world-side sink for inbound RELAY_*
// admin events received over the SubscribeWorldEvents stream. Slice 5a
// default impl (slogWorldEventsDispatcher) logs each event at Info; no
// world-state effects.
//
// NAI-S5A-D-DISPATCHER-NO-ACTION — retired piecewise by slice 5b as each
// opcode's action is wired (e.g. Shutdown → services.Manager.StopAsync).
type WorldEventsDispatcher interface {
    OnMute(username37 uint64, mutedUntilMs int64)
    OnKick(username37 uint64)
    OnShutdown(durationTicks int32)
    OnBroadcast(message string)
    OnTrack(username37 uint64, state int32)
    OnReload()
    OnClearLogins()
    OnClearLogouts()
    OnQueueScript(scriptName string, username37 uint64)
}
```

Default impl logs at Info (one level above the slogFriendsDispatcher debug-only logging) because admin events are rare and operationally interesting.

### 6.4 `worldEventsSubscriber` (per-world stream supervisor)

New file `modules/world/world_events_subscriber.go`. Structurally identical to `friends_subscriber.go` (slice 4a) — same exp-backoff supervisor (`backoffMin=1s`, `backoffMax=30s`, `steady=60s`), same EOF-vs-error log distinction. Only differences:

- Subscribes via `client.SubscribeWorldEvents(ctx, &SubscribeWorldEventsRequest{WorldId: worldID})`.
- Dispatches on `WorldEvent.Event` oneof variants to `WorldEventsDispatcher`.
- Lifecycle owned by `world.Server` (1 per process), NOT per-player.

```go
type worldEventsSubscriber struct {
    client     FriendsClient
    worldID    int32
    dispatcher WorldEventsDispatcher
    log        *slog.Logger
}

func newWorldEventsSubscriber(client FriendsClient, worldID int32, dispatcher WorldEventsDispatcher, log *slog.Logger) *worldEventsSubscriber { /* ... */ }

func (s *worldEventsSubscriber) run(ctx context.Context) { /* identical supervisor shape to friendsSubscriber.run */ }
```

### 6.5 `world.Server` lifecycle

Add to `Server` struct (`modules/world/server.go`):

```go
friendsAdminBridge    FriendsAdminBridge
worldEventsDispatcher WorldEventsDispatcher
worldEventsSub        *worldEventsSubscriber
worldEventsCancel     context.CancelFunc
```

Start the subscriber from `NewServer` (after `friendsClient` is wired, alongside `s.friendsBridge`/`s.friendsDispatcher`):

```go
s.friendsAdminBridge    = defaultFriendsAdminBridge(friendsClient, s.log)
s.worldEventsDispatcher = newSlogWorldEventsDispatcher(s.log)

if friendsClient != nil {
    s.worldEventsSub = newWorldEventsSubscriber(friendsClient, int32(cfg.NodeID), s.worldEventsDispatcher, s.log)
    ctx, cancel := context.WithCancel(context.Background())
    s.worldEventsCancel = cancel
    go s.worldEventsSub.run(ctx)
}
```

Stop on Server close. `Server.Shutdown()` (modules/world/server.go:537) already closes `s.quit` and the TCP listener; the per-world subscriber cancel is the natural sibling. Add inside `Shutdown`:

```go
if s.worldEventsCancel != nil { s.worldEventsCancel() }
```

The cancel is sufficient — the supervisor's `run(ctx)` loop returns when `ctx.Err() != nil`, gracefully closing the stream. No `sync.WaitGroup` join is required for slice 5a (the per-player subscriber pattern at `tick.go` doesn't join either; same posture).

**Lifecycle note:** the subscriber starts UNCONDITIONAL of whether any player is logged in. The world is "present" on the friends-server from process boot. This matches the TS model where the world's WebSocket is established at module init, not per-player.

## 7. Authentication: friends-server is dumb routing (NAI-S5A-D-NO-ADMIN-AUTH-AT-SERVER)

The friends-server does NOT check whether the caller of `RelayKick` is an admin. Admin authorization is the responsibility of the world that ORIGINATED the command (gated at the chat-command-handler / staffLvl check on that world) AND the world that RECEIVES the event (re-checks because the friends-server is untrusted intermediary).

Slice 5a's `WorldEventsDispatcher.OnX` methods receive the event verbatim and act on it. The slog-only default has no auth surface (it just logs). Slice 5b's real action handlers will need to think about defense-in-depth re-check (e.g., `OnKick` may want to verify the target player's world is actually this world; otherwise reject) — that's a 5b concern, not 5a.

**Tag opened:** `NAI-S5A-D-NO-ADMIN-AUTH-AT-SERVER` — permanent; matches TS. Reviewer traceability only.

## 8. Test surfaces

| Test | File | What it pins |
|---|---|---|
| `TestWorldSubscriptions_RegisterDeregisterDupKick` | `modules/friends/world_subscriptions_test.go` | Registry register/deregister/dup-kick (mirrors `TestSubscriptions_*` from slice 4a) |
| `TestWorldSubscriptions_SendNoSubscriber` | same | No-op silent-drop on unsubscribed worldId |
| `TestWorldSubscriptions_DropOnFull` | same | Drop-newest + warn log when channel full |
| `TestHandler_RelayKick_RoutesToSubscriber` | `modules/friends/handler_test.go` | One representative `Relay*` handler routes correctly |
| `TestHandler_RelayKick_NoSubscriberSilent` | same | Missing target world is silent OK (TS-faithful) |
| `TestHandler_SubscribeWorldEvents_DupKicksPrior` | same | Re-subscribe terminates prior subscriber |
| Per-RPC: 8 more thin pins | same | One pin per other `Relay*` proves the routing shape applies (mostly mechanical) |
| `TestGRPCFriendsClient_Relay_LogsErrorOnFailure` | `modules/world/friends_client_test.go` | Table-driven failure logging covering all 9 `Relay*` RPCs in one test (mirrors the existing `TestGRPCFriendsClient_LogsErrorOnFailure` shape from slice 4c) |
| `TestGRPCFriendsAdminBridge_KickIssuesRelayKick` | `modules/world/admin_bridge_test.go` | Bridge → client mapping |
| `TestSlogWorldEventsDispatcher_LogsAtInfo` | `modules/world/world_events_dispatcher_test.go` | Default impl logs each variant at Info |
| `TestWorldEventsSubscriber_DispatchRouting` | `modules/world/world_events_subscriber_test.go` | Oneof → dispatcher method mapping |
| `TestWorldEventsSubscriber_ReconnectBackoff` | same | Supervisor backoff (mirrors slice 4a) |
| `TestWorldEventsSubscriber_EOFLogsAtInfo` | same | Server-side close logs at Info, not Warn |
| `TestFriendsClient_E2E_RelayWorldEventsRoundTrip` | `modules/world/friends_smoke_test.go` | End-to-end: boot one friends-server, open TWO `SubscribeWorldEvents` streams (worlds A=1 and B=2), issue `Relay*` from world A targeting world B, assert dispatcher on world B receives the event and dispatcher on world A does NOT |

LOC estimate for tests: ~600 of the ~900 total. The handler tests are mostly mechanical (one pin per Relay* opcode); the e2e smoke is the load-bearing integration check.

### 8.1 `-race` discipline

Per slice-4a T14 / slice-4c T3 lesson (`[[friends-server-slice4c-close]]`): any test polling shared state across goroutines needs a mutex-wrapped helper. The slice-4c `syncBuffer` test helper in `tick_friends_login_test.go` is reusable — slice 5a should **extract it to a shared `world_test_util.go`** if any of the new tests need it (likely the dispatcher tests will). Decision: extract preemptively in T1 of the plan to avoid the race-detector surprise that bit slice 4a T14 and slice 4c T3.

## 9. Lifecycle: when does the per-world subscriber start?

The subscriber starts **eagerly at `NewServer` time**, not on first-player-login. Three reasons:

1. **Admin pushes must arrive even when no players are logged in.** An admin `RELAY_SHUTDOWN` to an empty world still needs to reach the world.
2. **Matches TS.** The TS world establishes its socket once, at module init.
3. **Same supervisor shape as slice 4a.** The exp-backoff supervisor is the same; reusing the pattern is simpler than a different lifecycle.

The `NewServer` wiring uses `context.WithCancel(context.Background())`. The Server-level cancel fires from the existing close/stopping path. This is independent of player-level `ctx.Done()` (which lives on the per-player `friendsSubscriber`).

## 10. Open follow-ups (not blockers)

- **`Makefile proto:` target not in `.PHONY`** (open since slice 1, see `[[friends-server-slice1-close]]`). Slice 5a is the first slice to regenerate the proto, so the bug becomes load-bearing. **Action: fix `.PHONY: proto` in T1 of the plan as the proto-regeneration prerequisite** — explicit, not implicit.
- **`syncBuffer` extraction** from `tick_friends_login_test.go` (per [[friends-server-slice4c-close]]). Action: extract to `world_test_util.go` in T1 alongside the Makefile fix.
- **NAI-182-D5 (social-cluster ServerGameProt port)** still blocks slice 5b's `Broadcast` / `Track` action wiring. Slice 5a does not depend on it (slog-only dispatcher). Slice 5b will open `NAI-S5-D-NO-INGAME-EMIT-BROADCAST` + `NAI-S5-D-NO-INGAME-EMIT-TRACK` mirroring the 4a/4b pattern.

## 11. Tag summary for slice 5a

| Action | Tag | Permanence |
|---|---|---|
| Retire | (none) | Slice 5a is purely additive foundation |
| Open | `NAI-S5A-D-PERWORLD-EVENTS-STREAM-SEPARATE` | **Permanent** — proto contract, can't retire without unifying with `SubscribeUpdates` |
| Open | `NAI-S5A-D-DISPATCHER-NO-ACTION` | **Retires piecewise in slice 5b** as each opcode action wires |
| Open | `NAI-S5A-D-NO-ADMIN-AUTH-AT-SERVER` | **Permanent** — TS-faithful, friends-server is dumb routing |
| Open | `NAI-S5A-D-WORLDEVENTS-DROP-ON-FULL` | **Permanent** — same posture as slice-4a `NAI-S4A-D-DROP-ON-FULL` |

## 12. Validation gates

Before slice 5a is considered closed:

1. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... -count=1 -timeout 600s` — clean across all 30 packages.
2. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape-cli smoke-pack --content-dir /home/owner/Code/github.com/LostCityRS/content` — 12 OK / 0 ERR / 0 SKIP (baseline must hold; slice 5a is server/world-only and does not touch packing).
3. Tag accounting: 4 NAI-S5A-D-* tags opened, 0 retired (slice 5b retires the dispatcher-no-action tag piecewise). All other slice-1/2/3/4 tags unchanged.
4. Whole-slice review per `[[friends-server-slice4a-close]]` lesson: even with verbatim-plan execution, a final cross-task consistency pass catches drift. Slice 5a is the largest sub-slice in slice 5 — review is warranted.
