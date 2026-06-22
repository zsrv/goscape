# Friends-server bridge — Slice 2: world-side `FriendsClient` + `grpcFriendsBridge`

**Date:** 2026-05-18
**Tech stack:** Go 1.26+ per `[[go_version]]`. No new direct deps — reuses `google.golang.org/grpc`, `pkg/friendspb` (slice 1), `pkg/util/jstring` for base37 encoding.
**Cadence:** Medium — new world-side client wrapper + bridge adapter + test fixtures + 5 surgical edits to existing world code (~600–800 LOC including tests). Sibling-to-`modules/world/login_client.go` shape throughout; mirrors `[[nai214-login-moderation-bridge-close]]` for the bridge half.
**Predecessor close memory:** `[[friends-server-slice1-close]]` shipped the friends-server itself (proto + module + in-memory repo, default `friends.enable=false`). The world side still uses `noopBridges{}` at `server.go:271`. This slice connects them.

---

## §0. Decomposition context (7 slices)

This document specs **slice 2 only**; remaining slices are forward references for reviewer context. Slice numbering carried forward from `[[friends-server-slice1-close]]`.

| # | Slice | Status |
|---|---|---|
| 1 | proto + `modules/friends/` skeleton + in-memory repo | **CLOSED** 2026-05-18 |
| **2** | **World-side `FriendsClient` + `grpcFriendsBridge`** (this spec) | **next** |
| 3 | SQLite persistence for friends-server | future |
| 4 | Server→world push (`SubscribeUpdates` impl) | future |
| 5 | Cross-world RELAY_* admin broadcasts | future |
| 6 | Chat logging (PUBLIC_CHAT_LOG + private_chat persistence) | future |
| 7 | `Player.session` per-login UUID assignment | future |

Critical-path: 1 → **2** → 3 → 4.

---

## §1. Scope

`modules/world/bridges.go:16-29` declares `FriendsBridge` with six outbound methods. All four production call sites resolve to `noopBridges{}` at `server.go:271`. Slice 1 shipped the friends-server module sibling to `modules/login`. Slice 2 builds the wire that connects them.

The shape mirrors the LoginClient/LoginBridgeMod split established by `[[loginclient-interface-close]]` and `[[nai214-login-moderation-bridge-close]]`:

- A new `FriendsClient` interface owns the gRPC seam (one method per outbound RPC + `Close`). Production impl `grpcFriendsClient` wraps `friendspb.FriendsServiceClient`. Test impl `fakeFriendsClient` records each call.
- A new `grpcFriendsBridge` adapter wraps `FriendsClient` + a captured `worldID`, implements the existing `FriendsBridge` interface (the six high-level methods called from packet handlers), and goroutine-fans-out each call so handlers never block on network I/O.
- A new `defaultFriendsBridge(client, worldID, log)` helper returns the gRPC bridge when `client != nil`, otherwise `noopBridges{}` — mirrors `defaultLoginBridgeMod`.
- Two non-bridge direct call sites get wired to `FriendsClient` (not via the bridge): `WorldConnect` at world startup (alongside `lc.WorldStartup`), and per-player `PlayerLogin`/`PlayerLogout` lifecycle (alongside `loginClient.PlayerLogin`/`.PlayerLogout`/`.PlayerForceLogout`).

Default `world.friends-server-enabled=false` matches slice 1's `friends.enable=false`: stock deployments see zero behaviour change.

**In scope:**

- `modules/world/friends_client.go` — `FriendsClient` interface + `grpcFriendsClient` impl + `NewFriendsClient(addr, log)`. Mirrors `modules/world/login_client.go`.
- `modules/world/bridges.go` — extend with `grpcFriendsBridge` adapter + `defaultFriendsBridge` helper. Existing `FriendsBridge` interface signature unchanged.
- `modules/world/friends_client_fake_test.go` — `fakeFriendsClient` with cap-16 channels for each fire-and-forget RPC + mutex-protected snapshots for assertion. Compile-time `var _ FriendsClient = (*fakeFriendsClient)(nil)`.
- `modules/world/friends_client_test.go` — per-RPC bridge fan-out tests + bridge fire-and-forget non-blocking test + `defaultFriendsBridge` selector tests.
- `modules/world/friends_smoke_test.go` — end-to-end smoke test starting a real `modules/friends` `Friends` service on an ephemeral port, dialling it via `NewFriendsClient`, exercising one RPC per kind to prove the wire works.
- `modules/world/config.go` — add `FriendsServerEnabled bool` + `FriendsServerAddress string` flags. Defaults: `false`, `"127.0.0.1:2005"`.
- `modules/world/world.go` — extend `World` constructor to dial `NewFriendsClient` when enabled; add `friendsClient` field + `GetFriendsClient()` accessor; pass to `NewServer`; add `WorldConnect` invocation in `NewWorldService.startingFn` parallel to `lc.WorldStartup`; close in `stoppingFn`.
- `modules/world/server.go` — add `friendsClient FriendsClient` field; flip `s.friendsBridge = defaultFriendsBridge(...)` at line 271; extend `NewServer` signature to accept `friendsClient FriendsClient`; fire `PlayerLogout` RPCs from `removePlayerOnTick` + `removePlayerOnDisconnect` parallel to existing `loginClient` calls.
- `modules/world/tick.go` — fire `PlayerLogin` RPC from `processLogins` after `addPlayer` success.
- `modules/world/server_test.go`, `npc_interaction_test.go`, `interaction_test.go`, `bridges_test.go` — update test bootstrappers that pin `s.friendsBridge = noopBridges{}` to keep working with the new wiring (signature changes only; intent unchanged).
- `cmd/goscape/app/modules.go` — pass `friendsClient` from `World` through to `NewServer` (already done via the existing `loginClient` precedent; needs the friends parallel).
- Retire `NAI-72-D-FRIENDS-SERVER-BRIDGE`: 8 doc-comment references across `modules/world/` get rewritten to point at the production wiring.

**Out of scope (deferred to later slices):**

- **Player-cap rejection handling.** `friendspb.PlayerLoginResponse.Accepted` is ignored slice-2 (slice 1's `NAI-S1-D-PLAYERCAP-LOG-ONLY` carries through). Slice 4 surfaces the rejection.
- **Server→world push.** No `SubscribeUpdates` stream client. Bridge writes are fire-and-forget; followers won't see updates. Tag `NAI-S1-D-NO-FOLLOWER-BROADCAST` stays open.
- **SQLite persistence.** Slice 3. The repo on the friends-server remains in-memory through slice 2.
- **Private-message delivery + persistence.** `PrivateMessage` bridge calls the slice-1 handler which logs and returns. Slice 4 / slice 6 retire those tags.
- **RELAY_* admin broadcasts.** Slice 5.
- **PUBLIC_CHAT_LOG.** Slice 6.
- **`Player.session` UUID generation.** Slice 7. `PlayerLogin` RPC payload doesn't carry a session field today; slice 7 adds it.
- **Auth between world and friends-server.** Matches `modules/login` posture: intra-cluster trust.
- **Smoke-pack stage.** No on-disk artifact to byte-pin.

---

## §2. Layout

| File | State | Purpose |
|---|---|---|
| `modules/world/friends_client.go` | **NEW** | `FriendsClient` interface + `grpcFriendsClient` impl + `NewFriendsClient`. |
| `modules/world/friends_client_fake_test.go` | **NEW** | `fakeFriendsClient` test fixture (cap-16 chans + sync snapshots). |
| `modules/world/friends_client_test.go` | **NEW** | Bridge fan-out unit tests + `defaultFriendsBridge` selector tests. |
| `modules/world/friends_smoke_test.go` | **NEW** | End-to-end smoke against a real `Friends` module. |
| `modules/world/bridges.go` | **EDIT** | Add `grpcFriendsBridge` + `defaultFriendsBridge`. `FriendsBridge` interface unchanged. |
| `modules/world/bridges_test.go` | **EDIT** | One-line update if needed; compile-time interface assertion already covers the new impl. |
| `modules/world/config.go` | **EDIT** | Add `FriendsServerEnabled` + `FriendsServerAddress` config + flags. |
| `modules/world/world.go` | **EDIT** | Dial `NewFriendsClient` when enabled; fire `WorldConnect`; close on shutdown. |
| `modules/world/server.go` | **EDIT** | Accept `friendsClient` in `NewServer`; store as field; flip line 271 default; fire `PlayerLogout` in remove paths. |
| `modules/world/tick.go` | **EDIT** | Fire `PlayerLogin` in `processLogins` after `addPlayer`. |
| `modules/world/server_test.go` | **EDIT** | Test bootstrapper aligned with new `NewServer` signature. |
| `modules/world/npc_interaction_test.go` | **EDIT** | Same — 2 sites at lines 1831, 2092 are `s.friendsBridge = noopBridges{}` overrides. |
| `modules/world/interaction_test.go` | **EDIT** | Same — 2 sites at lines 1871, 2726. |
| `modules/world/handler_chatsetmode.go` | **EDIT** | Replace deferral-tag doc-comment with production reference. |
| `modules/world/handler_social_list.go` | **EDIT** | Same. |
| `modules/world/handler_message_private.go` | **EDIT** | Same. |
| `modules/world/handler_reportabuse.go` | **EDIT** | Remove stale tag reference in doc comment. |
| `modules/world/player.go` | **EDIT** | Remove stale tag reference in doc comment (if present after `[[nai214-login-moderation-bridge-close]]`'s cleanup left a friends one — verify and update). |
| `cmd/goscape/app/modules.go` | **EDIT** | Wire `friendsClient` from `World` through `NewServer`. |

No edits to `internal/dskit/`, `pkg/friendspb/`, `proto/friends/`, or `modules/friends/`.

---

## §3. Architecture

Two-seam shape mirroring the login side:

```
                                ┌──────────────────────────────────────────┐
   FriendsBridge interface ───► │ grpcFriendsBridge {client, worldID, log} │
   (handler call sites)         │   - goroutine fan-out per call           │
                                │   - util.ToBase37(username) translation  │
                                └──────────────┬───────────────────────────┘
                                               │
                                               ▼
                                ┌──────────────────────────────────────────┐
   Direct call sites      ────► │ FriendsClient interface                  │
   (WorldConnect from           │   - grpcFriendsClient (prod)             │
    NewWorldService.starting,   │   - fakeFriendsClient (test)             │
    PlayerLogin from            │   - log+swallow on RPC error             │
    processLogins,              └──────────────┬───────────────────────────┘
    PlayerLogout from                          │
    removePlayerOn{Tick,Disconnect})           ▼
                                   friendspb.FriendsServiceClient
                                   (slice 1's gRPC server, default :2005)
```

Why two seams (matches NAI-214 reasoning):

- **`FriendsBridge`** is the existing per-handler call surface. Handlers don't know about gRPC, request shapes, world IDs, or base37 encoding — they pass `(playerUsername, target uint64)`. Slice 1 left this interface in place; slice 2 must keep it stable so handler call sites need zero edits (only their deferral-tag doc-comments change).
- **`FriendsClient`** is the RPC seam. It exists so that (a) tests can stub the gRPC layer with `fakeFriendsClient`, (b) non-bridge call sites (lifecycle: WorldConnect / PlayerLogin / PlayerLogout) can talk to the friends-server without a synthetic bridge method, (c) the `grpcFriendsBridge` is a thin translator on top.

Why goroutine fan-out in the bridge: handler call sites run on the per-connection goroutine and on the tick goroutine (PrivateMessage path). Blocking either on network I/O would stall the world. Mirrors `loginGRPCBridgeMod` at `modules/world/bridges.go:90-104`.

Why `WorldConnect` is fire-and-forget: matches `lc.WorldStartup(ctx, ...)` at `world.go:88`. Returns no error to the caller; logs warn if the RPC fails. Re-call on world restart is safe (slice 1's `WorldConnect` re-inits the world's slot deterministically).

Why `PlayerLogin` fires from `processLogins` (not `callPlayerLoginRPC`): `processLogins` is where the player actually enters the world (post `addPlayer`). At that point `p.username` + `p.username37` are populated and the player has a slot. Mirrors TS's PLAYER_LOGIN-on-world-entry semantics. The login-server's PlayerLogin RPC ran one tick earlier during the handshake; the friends-server PlayerLogin fires when the player is live on the world.

Why `PlayerLogout` fires from **both** `removePlayerOnTick` AND `removePlayerOnDisconnect`: the friends-server doesn't care about save-vs-no-save semantics. Either way the player has left this world and slot inventory should drop on the friends-server side. Mirrors how `loginClient.PlayerLogout` vs `.PlayerForceLogout` both signal "player gone" — slice 2 doesn't need the split for friends.

Concurrency: `grpcFriendsBridge` spawns a goroutine per call. Lifetime is bounded by the RPC; `context.Background()` is used because there is no per-tick cancellation requirement (the friends-server is best-effort by design — slice 1 tag `NAI-S1-D-PM-NO-DELIVERY` and `NAI-S1-D-NO-FOLLOWER-BROADCAST` already establish that the world doesn't depend on friends-server replies).

Disabled flow: when `FriendsServerEnabled=false`, `World.friendsClient` stays `nil`. `Server.friendsBridge` resolves to `noopBridges{}` via `defaultFriendsBridge(nil, ...)`. `WorldConnect` / `PlayerLogin` / `PlayerLogout` direct calls are guarded by `if s.friendsClient != nil` checks — same pattern as `loginClient` guards at `server.go:949`, `:981`, `:1005`.

---

## §4. Interface contracts

### `FriendsClient` (new, mirrors `LoginClient`)

```go
type FriendsClient interface {
    WorldConnect(ctx context.Context, worldID int32, profile string)
    PlayerLogin(ctx context.Context, req *friendspb.PlayerLoginRequest)
    PlayerLogout(ctx context.Context, req *friendspb.PlayerLogoutRequest)
    ChatSetMode(ctx context.Context, req *friendspb.ChatSetModeRequest)
    FriendlistAdd(ctx context.Context, req *friendspb.FriendlistAddRequest)
    FriendlistDel(ctx context.Context, req *friendspb.FriendlistDelRequest)
    IgnorelistAdd(ctx context.Context, req *friendspb.IgnorelistAddRequest)
    IgnorelistDel(ctx context.Context, req *friendspb.IgnorelistDelRequest)
    PrivateMessage(ctx context.Context, req *friendspb.PrivateMessageRequest)
    Close() error
}
```

All RPC methods are **fire-and-forget** (no return value, RPC errors logged-warn-and-swallowed). This matches `LoginClient`'s split-pattern: methods whose response is consumed (LoginClient.PlayerLogin) return `(resp, err)`; methods whose response is ignored (LoginClient.PlayerAutosave / .PlayerForceLogout / .PlayerBan / .PlayerMute) return nothing. Friends-server slice 2 ignores every response (slice 1 `PlayerLoginResponse.Accepted` is log-only on the server side too), so all methods are void.

`SubscribeUpdates` is **not** in the interface — slice 4 adds it.

Production impl `grpcFriendsClient` wraps `friendspb.FriendsServiceClient`. Each method delegates to the embedded client; on error, `c.log.Warn("X RPC failed", slog.String("...", ...), slog.Any("err", err))`. Matches `grpcLoginClient` style at `modules/world/login_client.go:79-115`.

`Close()` shuts down the underlying `*grpc.ClientConn`.

### `grpcFriendsBridge` (new, implements existing `FriendsBridge`)

```go
type grpcFriendsBridge struct {
    client  FriendsClient
    worldID int32
    log     *slog.Logger
}

var _ FriendsBridge = (*grpcFriendsBridge)(nil)
```

Each high-level method:

1. Resolves `playerUsername string → username37 uint64` via `util.ToBase37`.
2. Constructs the per-RPC request proto with `WorldId: b.worldID`.
3. Spawns `go b.client.<RPC>(context.Background(), req)`.

The bridge owns the `worldID` (captured at construction time from `cfg.NodeID`) so handler call sites stay world-agnostic. Mirrors the way `loginGRPCBridgeMod` owns the `*slog.Logger` for warn paths.

### `defaultFriendsBridge` (new helper)

```go
func defaultFriendsBridge(client FriendsClient, worldID int32, log *slog.Logger) FriendsBridge {
    if client != nil {
        return &grpcFriendsBridge{client: client, worldID: worldID, log: log}
    }
    return noopBridges{}
}
```

Called from `NewServer`. Unit-testable without spinning up the full server bring-up. Mirrors `defaultLoginBridgeMod` at `modules/world/bridges.go:112-117`.

### `fakeFriendsClient` (new, test-only)

Same pattern as `fakeLoginClient`:

```go
type fakeFriendsClient struct {
    mu sync.Mutex

    worldConnectCalls []worldConnectCall

    playerLoginReqs    chan *friendspb.PlayerLoginRequest
    playerLogoutReqs   chan *friendspb.PlayerLogoutRequest
    chatSetModeReqs    chan *friendspb.ChatSetModeRequest
    friendlistAddReqs  chan *friendspb.FriendlistAddRequest
    friendlistDelReqs  chan *friendspb.FriendlistDelRequest
    ignorelistAddReqs  chan *friendspb.IgnorelistAddRequest
    ignorelistDelReqs  chan *friendspb.IgnorelistDelRequest
    privateMessageReqs chan *friendspb.PrivateMessageRequest

    closed bool
}

type worldConnectCall struct {
    WorldID int32
    Profile string
}
```

- WorldConnect appends under `mu` (mirrors `fakeLoginClient.worldStartupCalls`).
- Every RPC request channel has capacity 16 + non-blocking `select { case ch <- req: default: }` send. Tests assert via `select { case got := <-ch: ... case <-time.After(time.Second): t.Fatal(...) }`.
- `snapshotWorldConnectCalls() []worldConnectCall` returns a copy of the slice under `mu`. No per-RPC snapshot needed — channels are the assertion surface.
- Compile-time assertion: `var _ FriendsClient = (*fakeFriendsClient)(nil)`.

---

## §5. Call-site wiring

### `world.go` — startup

Existing pattern at `world.go:52-89`:

```go
var loginClient LoginClient
if cfg.LoginServerEnabled {
    lc, err := NewLoginClient(cfg.LoginServerAddress, logger)
    if err != nil { logger.Warn("failed to create login client", slog.Any("err", err)) }
    else { loginClient = lc }
}
w.loginClient = loginClient
server, err := NewServer(cfg, loginClient, logger)
```

Slice 2 adds a parallel block for `friendsClient`. Then in `NewWorldService.startingFn`:

```go
if lc != nil { lc.WorldStartup(ctx, int32(serv.cfg.NodeID), serv.cfg.NodeProfile) }
if fc != nil { fc.WorldConnect(ctx, int32(serv.cfg.NodeID), serv.cfg.NodeProfile) }
```

And in `stoppingFn`:

```go
if lc != nil { _ = lc.Close() }
if fc != nil { _ = fc.Close() }
```

`NewWorldService` signature gains a `fc FriendsClient` parameter (positionally after `lc LoginClient`).

### `server.go` — wiring

- `NewServer` signature becomes `func NewServer(cfg Config, loginClient LoginClient, friendsClient FriendsClient, logger *slog.Logger) (*Server, error)`. Test callsite `newTestServer` updates similarly.
- New field `Server.friendsClient FriendsClient` parallel to `loginClient` at line 53.
- Line 271 flips from `s.friendsBridge = noopBridges{}` to `s.friendsBridge = defaultFriendsBridge(friendsClient, int32(cfg.NodeID), s.log)`.

### `server.go` — logout paths

`removePlayerOnTick` (server.go:948) gains a sibling block after the `loginClient.PlayerLogout` goroutine:

```go
if s.friendsClient != nil && p.username != "" {
    username37 := p.username37
    go s.friendsClient.PlayerLogout(context.Background(), &friendspb.PlayerLogoutRequest{
        WorldId:    int32(s.cfg.NodeID),
        Username37: username37,
    })
}
```

`removePlayerOnDisconnect` (server.go:980) gets the same block. (Both fire `PlayerLogout` — friends-server has no "force" variant; the player is gone either way.)

### `tick.go` — login path

`processLogins` (tick.go:145), inside the per-player loop, **after** the existing `if err := s.addPlayer(p); err != nil { ... continue }` guard at line 152 succeeds and **after** `p.lastConnected = s.currentTick` is set:

```go
if s.friendsClient != nil && p.username != "" {
    req := &friendspb.PlayerLoginRequest{
        WorldId:     int32(s.cfg.NodeID),
        Username37:  p.username37,
        PrivateChat: int32(p.privateChat),
        StaffLvl:    p.staffModLevel,
    }
    go s.friendsClient.PlayerLogin(context.Background(), req)
}
```

Placement: just before the existing `// sub-spec 3a: initialise worn inventory` block at tick.go:164. The player has username/username37/staffModLevel populated by `callPlayerLoginRPC`; `privateChat` defaults to `0` (ON) until the player issues `ChatSetMode`.

### Handler call sites — no edits

All four handler files (`handler_chatsetmode.go:23`, `handler_social_list.go:43-53`, `handler_message_private.go:47`) already call `friendsBridge.<Method>(...)`. The bridge swap at line 271 is the only change needed for them to start firing real RPCs. Only their deferral-tag doc-comments change (see §6).

### Test bootstrappers

5 sites in test files set `s.friendsBridge = noopBridges{}` after `NewServer`. These pre-date the bridge-default flip; once `NewServer` returns a bridge already wired to a (possibly nil) `friendsClient`, the manual override is redundant but harmless. Slice 2 leaves them in place to keep test isolation explicit, except:

- `bridges_test.go:104` (`installRecordingBridges`) — still needed; tests want `recordingBridges` for capture.
- `server_test.go:327`, `npc_interaction_test.go:1831,2092`, `interaction_test.go:1871,2726` — leave as `noopBridges{}` so tests that don't care about friends-server traffic see deterministic no-ops.

The compile-time interface assertion at `bridges_test.go:113` (`_ FriendsBridge = (*recordingBridges)(nil)`) catches any signature drift; no edits needed there since the `FriendsBridge` interface itself is unchanged.

---

## §6. Tag retirement & deviations

### Retired

- `NAI-72-D-FRIENDS-SERVER-BRIDGE` — the umbrella deferral tag for "world's friends bridge is `noopBridges{}`." 8 references across `modules/world/`:
  - `bridges.go:15` (FriendsBridge interface doc)
  - `bridges.go:27` (PrivateMessage method doc)
  - `handler_chatsetmode.go:13`
  - `handler_social_list.go:30`
  - `handler_message_private.go:23`
  - `handler_reportabuse.go:29` (mention; not load-bearing)
  - 2 server_test.go-side carry-forwards if any remain

  All rewritten to point at the production wiring. Tag is removed from the repo.

### Carried forward (open)

| Tag | Slice 1 site | Retired by |
|---|---|---|
| `NAI-S1-D-INMEMORY-REPO` | `modules/friends/repository.go:5` | Slice 3 |
| `NAI-S1-D-LAZY-WORLDINIT` | `modules/friends/handler.go:28` | Permanent (TS-faithful) |
| `NAI-S1-D-PLAYERCAP-LOG-ONLY` | `modules/friends/handler.go:58` | Slice 4 |
| `NAI-S1-D-NO-FOLLOWER-BROADCAST` | 6 sites in `modules/friends/` | Slice 4 |
| `NAI-S1-D-PM-NO-DELIVERY` | `modules/friends/handler.go:132` | Slice 4 |
| `NAI-S1-D-PM-NO-PERSISTENCE` | `modules/friends/handler.go:133` | Slice 6 |

### New deviation tags (slice 2)

| Tag | Sites | Why | Retired by |
|---|---|---|---|
| `NAI-S2-D-PLAYERLOGIN-AT-PROCESSLOGINS` | `tick.go` processLogins | PlayerLogin fires when the player is added to the world (post-`addPlayer`), not when the login-server handshake completes. Matches TS "PLAYER_LOGIN on world entry" semantics. If slice 4's `SubscribeUpdates` integration reveals a TS ordering constraint that demands an earlier firing, this tag opens the discussion. | Permanent (TS-faithful) unless slice 4 disagrees |
| `NAI-S2-D-PLAYERLOGOUT-BOTH-PATHS` | `server.go` removePlayerOnTick + removePlayerOnDisconnect | Friends-server has no "force" PlayerLogout variant — both graceful and ungraceful logout fire the same RPC. The login-server splits PlayerLogout vs PlayerForceLogout because save-payload-vs-not matters; friends-server only cares about presence. | Permanent |
| `NAI-S2-D-PLAYERLOGIN-IGNORES-ACCEPTED` | `tick.go` processLogins | `PlayerLoginResponse.Accepted` is ignored. The world treats friends-server registration as best-effort: a cap rejection logs warn on the server side (slice 1 `NAI-S1-D-PLAYERCAP-LOG-ONLY`) but the player still enters the world. Slice 4 surfaces the rejection if needed. | Slice 4 (alongside `NAI-S1-D-PLAYERCAP-LOG-ONLY`) |

---

## §7. Testing strategy

Three test files, three concerns.

### `friends_client_test.go` — unit tests (no real gRPC server)

Mirrors the existing `bridges_test.go` pattern for `loginGRPCBridgeMod`:

1. **Per-RPC bridge fan-out** (one test per method on `grpcFriendsBridge`):
   - `TestGRPCFriendsBridge_AddFriend_FiresRPC` — bridge.AddFriend("alice", 42) → fake.friendlistAddReqs receives `FriendlistAddRequest{WorldId, Username37: util.ToBase37("alice"), TargetUsername37: 42}`.
   - Same shape for RemoveFriend / AddIgnore / RemoveIgnore / SetChatMode / PrivateMessage.

2. **Fire-and-forget non-blocking** (one test, gated-fake pattern from `bridges_test.go:285-331`):
   - `TestGRPCFriendsBridge_FireAndForget_DoesNotBlock` — `gatedFriendsClient` whose `FriendlistAdd` blocks on `<-gate`. Bridge call must return < 100ms despite the gate; gate-release → fake captures.

3. **`defaultFriendsBridge` selector** (mirrors `TestDefaultLoginBridgeMod_*`):
   - `TestDefaultFriendsBridge_NonNilClient_ReturnsGRPCBridge` — `defaultFriendsBridge(newFakeFriendsClient(), 10, discardLogger())` returns `*grpcFriendsBridge`.
   - `TestDefaultFriendsBridge_NilClient_ReturnsNoop` — returns `noopBridges{}`.
   - `TestDefaultFriendsBridge_CapturesWorldID` — bridge with worldID=42 → first RPC carries `WorldId: 42`.

4. **`grpcFriendsClient` log-on-error** (mirrors `TestLoginClient_PlayerBan_LogsErrorOnFailure` pattern; uses a `mockFriendsPBClient` embedding `friendspb.FriendsServiceClient` with one method overridden to return an error). One test per RPC method to verify error path logs warn — collapse into a single table-driven test.

### `friends_client_fake_test.go` — fixture only (no tests)

Declares `fakeFriendsClient`, `worldConnectCall`, `newFakeFriendsClient()`, snapshot helpers. Compile-time `var _ FriendsClient = (*fakeFriendsClient)(nil)`.

### `friends_smoke_test.go` — end-to-end against real friends-server

```go
func TestFriendsClient_E2E_SmokeAgainstFriendsServer(t *testing.T) {
    log := discardLogger()
    cfg := friends.Config{
        GRPCListenAddress: "127.0.0.1", GRPCListenPort: 0, // ephemeral
        NodeProfile: "main", WorldPlayerLimit: 100,
        GracefulShutdownTimeout: 5 * time.Second,
    }
    // Build the Friends service directly (not via dskit Manager — too heavy
    // for a unit test). starting() binds the listener; we read the port back.
    // ...

    addr := lis.Addr().String() // captured before grpcServer takes ownership
    client, err := NewFriendsClient(addr, log)
    // ...
    defer client.Close()

    ctx := t.Context()
    client.WorldConnect(ctx, 10, "main")
    client.PlayerLogin(ctx, &friendspb.PlayerLoginRequest{...})
    client.FriendlistAdd(ctx, &friendspb.FriendlistAddRequest{...})
    // ... assert repo state via the Friends.repo accessor or by re-querying
    //     a side-effect (e.g., follow-up FriendlistDel + assert no error).

    client.PlayerLogout(ctx, ...)
}
```

Sequencing: each call is sync from the client perspective (the gRPC stub `Invoke` blocks until the server replies). The test runs in single-threaded order and asserts via repository read-back. No goroutine sync needed.

Cleanup: defer `client.Close()` + `friendsService.shutdown()`.

This pins the wire end-to-end: proto compat + handler routing + repo mutation. If slice 3's SQLite swap or slice 4's stream addition breaks the proto, this test fails.

### Race detector

`go test -race ./modules/world/...` must remain clean. The new goroutine-fan-out in `grpcFriendsBridge` and the new `go s.friendsClient.PlayerLogout(...)` calls in `removePlayerOn{Tick,Disconnect}` introduce new concurrent paths; the existing race-test bootstrap covers them.

### Smoke-pack

`goscape-cli smoke-pack` must remain `12 OK / 0 ERR / 0 SKIP`. Slice 2 touches no packers or content paths; no regression expected, but the close commit re-runs it as a guardrail.

---

## §8. Risks & rejected alternatives

### Rejected: add `WorldConnect/PlayerLogin/PlayerLogout` to `FriendsBridge`

Would put all friends RPCs behind a single interface. But the existing `FriendsBridge` callers (packet handlers) don't carry `worldID` / `username37` / `staffLvl` — those are server-side concerns. Pushing the lifecycle RPCs through the bridge would force every test bootstrapper (`installRecordingBridges` and friends) to handle calls it doesn't care about. Two seams is cleaner.

### Rejected: synchronous bridge methods

Would simplify the bridge — no `go` keyword. But the PrivateMessage handler is on the tick goroutine (server_pmid.go:nextPmId is tick-private); blocking on a friends-server RPC there stalls the tick. The login-side pattern explicitly fans-out at the bridge level for the same reason. Slice 2 matches.

### Rejected: separate `WorldConnect` retry loop

Slice 1's `WorldConnect` is idempotent (re-init resets the world slot). Slice 2 fires it once at world startup; if the friends-server is down, the warn log is the trail. Retrying would require a supervisor loop similar to `runContentWatcher` — disproportionate for a contract this loose. Slice 4's `SubscribeUpdates` will need real reconnection logic; that's the natural place to add `WorldConnect` re-fire on stream restart.

### Risk: tick.go injection point ordering

`processLogins` does a lot per-player. Firing `PlayerLogin` too early (before `addPlayer`) would leak the RPC on world-full rejections; too late (after the LOGIN trigger script) would delay registration unnecessarily. Spec §5 placement (just before `// sub-spec 3a: initialise worn inventory`) is after `addPlayer` success + `lastConnected` update, before any cache-load work. The plan must verify this insertion point against tick.go's current line numbers since this file has been edited multiple times in 2026-05.

### Risk: signature ripple

Adding `FriendsClient` to `NewServer` and `NewWorldService` is a breaking change to public-ish constructors. Test bootstrappers (`newTestServer` in `server_test.go:300+`) thread through nil for the new arg. The plan must catch every call site — `grep -n "NewServer\b" modules/world/` and `grep -rn "NewWorldService\b"` are the audit commands.

### Risk: ephemeral port in smoke test

`net.Listen("tcp", "127.0.0.1:0")` yields a free port, but slice 1's `grpcServer.listen` calls `net.Listen` from inside `starting()` using `cfg.GRPCListenPort`. We need to set `cfg.GRPCListenPort=0` and then read `lis.Addr()` back from the Friends service after `starting()` returns. Slice 1 doesn't expose the listener — the spec needs a test-only accessor or constructs the listener externally and passes it in. **Resolution:** construct the `Friends` service via `friends.New(cfg, log)`, then call its `Service.StartAsync(ctx)` and `Service.AwaitRunning(ctx)` from dskit, and grab the address via `cfg.GRPCListenAddress + ":" + portFromListener`. If slice 1's API doesn't expose this cleanly, the plan adds a tiny test-only listener override seam in slice 1's `friends.go`.

---

## §9. Acceptance gates

Slice 2 is done when:

1. `go build ./...` clean.
2. `go test ./...` clean.
3. `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...` clean.
4. `goscape-cli smoke-pack --content-dir $HOME/Code/github.com/LostCityRS/content` → `12 OK / 0 ERR / 0 SKIP` (or matching the slice-1 baseline).
5. `grep -rn NAI-72-D-FRIENDS-SERVER-BRIDGE modules/world/` returns **zero matches**.
6. `friends_smoke_test.go` PASS — proves the wire is real.
7. 3 new deviation tags (`NAI-S2-D-*`) opened in code with retirement plans documented in this spec's §6.
8. Memory entry `friends_server_slice2_close.md` updated with final commit list + plan-deviation log + gate results.

---

## §10. Open questions / things the plan must verify

- **`p.privateChat` default value at processLogins time.** If `p.privateChat = 0` is the post-`LoadSave` value before any client opcode, the slice-2 `PlayerLogin` RPC sends `0` (ON). If `LoadSave` populates it from the SAV, the saved value flows through. Plan task should grep `LoadSave` to confirm whether `privateChat` is loaded from save.
- **`util.ToBase37` round-trip on player username.** `p.username37` is set at `player.go:503` as `util.ToBase37(c.username)` during player construction. The bridge can use `p.username37` directly **only if** the bridge has access to the `*Player` — it doesn't (the `FriendsBridge` interface takes `playerUsername string`). So the bridge re-encodes via `util.ToBase37(playerUsername)`. Per `[[curly-quote-charcode-truncation-fix]]`, double-check that `ToBase37` handles ASCII-only correctly (no multi-byte rune issues for usernames — they're ASCII by RS2 grammar).
- **`mockFriendsPBClient` embedding pattern.** `LoginClient` tests at `login_client_test.go` embed `loginpb.LoginServiceClient` (the gRPC-generated interface) and override one method. Verify `friendspb.FriendsServiceClient` is structurally identical (it should be — same code-gen tooling) before plan codifies the helper struct.
- **`bridges_test.go:113` compile-time assertion stays.** New `grpcFriendsBridge` should join: `_ FriendsBridge = (*grpcFriendsBridge)(nil)`. The plan codifies this one-line addition.
