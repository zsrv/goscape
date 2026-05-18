# Friends-server bridge — Slice 1: `modules/friends/` skeleton + proto + in-memory repo

**Date:** 2026-05-18
**Tech stack:** Go 1.26+ per `[[go_version]]`. New direct deps: none beyond what `modules/login` already pulls in (`google.golang.org/grpc`, `google.golang.org/protobuf`). New generated package: `pkg/friendspb` (mirrors `pkg/loginpb`).
**Cadence:** Medium — new module (~600–800 LOC: proto + generated code + module skeleton + in-memory repo + tests), no changes to existing modules, no DB, no world-side wiring. Sibling-to-`modules/login` shape throughout.
**Predecessor close memory:** `[[nai214-login-moderation-bridge-close]]` shipped the login-bridge-mod half of the original NAI-72 umbrella. This slice opens the friends-bridge half via a 7-slice decomposition; slice 1 lands the friends-server module so subsequent slices can wire the world side and add persistence.

---

## §0. Decomposition context (7 slices)

The full friends-server port is too large for one spec. This document specs **slice 1 only**; the rest are forward references for reviewer context.

| # | Slice | Lands | Depends on |
|---|---|---|---|
| **1** | **friends-pb + `modules/friends/` skeleton + in-memory repo** (this spec) | New `pkg/friendspb` proto (9 unary outbound RPCs + `SubscribeUpdates` server-streaming stub), new `modules/friends/` dskit module (Config, gRPC server, handler, in-memory repository), repo + handler tests | — |
| 2 | World-side `FriendsClient` + `grpcFriendsBridge` (NAI-214 shape) | `modules/world/friends_client.go` (interface + grpc impl), `grpcFriendsBridge` adapter (goroutine fan-out), `defaultFriendsBridge` helper, `server.go` flip, `fakeFriendsClient` test pattern, end-to-end smoke against slice 1's server | 1 |
| 3 | SQLite persistence for friends-server | `modules/friends/db.go` + `migrations/`, repository swap in-memory → SQLite, `db_test.go` | 1 |
| 4 | Server→world push (UPDATE_FRIENDLIST / IGNORELIST / PRIVATE_MESSAGE) | Per-player `SubscribeUpdates` stream client world-side, per-`(world,player)` subscriber registry server-side, `broadcastWorldToFollowers` port, world-side pump, reconnect/backpressure | 1, 2, 3 |
| 5 | Cross-world RELAY_* admin broadcasts | 9 RELAY_* RPCs (MUTE/KICK/SHUTDOWN/BROADCAST/TRACK/RELOAD/CLEARLOGINS/CLEARLOGOUTS/QUEUESCRIPT); per-world stream context | 1, 4 |
| 6 | Chat logging (PUBLIC_CHAT_LOG + private_chat persistence) | DB tables + RPCs; wire from existing chat handlers | 3 |
| 7 | `Player.session` per-login UUID assignment | Small bolt-on at `login_handle.go`; consumes friends-server `PlayerLogin` arg path | 1 (proto) or 4 |

Critical-path order: **1 → 2 → 3 → 4**. Slices 5/6/7 are leaves that can land in any order after their deps.

---

## §1. Scope

`modules/world/bridges.go:16-29` declares `FriendsBridge` with six outbound methods. All four call sites (`handler_chatsetmode.go:23`, `handler_social_list.go:43`, `handler_message_private.go:47`, plus a doc-only reference in `handler_reportabuse.go:29`) resolve to `noopBridges{}` (`server.go:271`). The bridge has nothing to talk to: there is no friends-server.

This slice ships the friends-server itself — the gRPC contract (`pkg/friendspb`), the dskit module hosting it (`modules/friends/`), and an in-memory repository tracking per-player presence + friend/ignore sets. The module is gated by `friends.enable` (default `false`) and added to the `all` target sibling to `modules/login`. No world-side code changes.

`SubscribeUpdates` (server-streaming) is declared in the proto with concrete message types but its handler returns `codes.Unimplemented` — slice 4 fills the body. Locking the contract early lets slice 2 design `FriendsClient` against the final shape.

**In scope:**

- New proto file `proto/friends/friends.proto` declaring `service FriendsService` with 10 RPCs: 9 unary outbound (`WorldConnect`, `PlayerLogin`, `PlayerLogout`, `ChatSetMode`, `FriendlistAdd`, `FriendlistDel`, `IgnorelistAdd`, `IgnorelistDel`, `PrivateMessage`) and 1 server-streaming stub (`SubscribeUpdates`). All request messages carry `int32 world_id`.
- Generated Go code in `pkg/friendspb/`: `friends.pb.go`, `friends_grpc.pb.go` (same `protoc-gen-go` / `protoc-gen-go-grpc` toolchain as `pkg/loginpb`).
- New module `modules/friends/`:
  - `config.go` — `Config` struct + `RegisterFlagsAndApplyDefaults` + `Validate`. Flags: `friends.enable` (bool, default false), `friends.grpc-listen-address` (default `127.0.0.1`), `friends.grpc-listen-port` (default `2005`), `friends.node-profile` (default `main`), `friends.world-player-limit` (int, default `2000`), `friends.graceful-shutdown-timeout` (default `30s`).
  - `friends.go` — `Friends` struct + `New()` + `NewFriendsService()` factory + `starting`/`running`/`stopping` lifecycle. Mirrors `modules/login/login.go` minus the DB.
  - `server.go` — `grpcServer` wrapper: `newGRPCServer(cfg, repo, log)`, `listen()`, `serve()`, `shutdown()`. Mirrors `modules/login/server.go`.
  - `handler.go` — `handler` struct implementing `friendspb.FriendsServiceServer`. Each RPC: validate, delegate to repo, return response. `SubscribeUpdates` returns `status.Error(codes.Unimplemented, "deferred to slice 4")`.
  - `repository.go` — in-memory `Repository` with `sync.RWMutex`. State: `worlds map[int32]*worldState`, `players map[uint64]*playerState`, `friends map[uint64]map[uint64]struct{}`, `ignores map[uint64]map[uint64]struct{}`. Methods mirror `FriendServerRepository.ts` (port reference at `/home/owner/Code/github.com/LostCityRS/Engine-TS/src/server/friend/FriendServerRepository.ts`): `InitializeWorld`, `Register`, `Unregister`, `SetChatMode`, `AddFriend`, `DeleteFriend`, `AddIgnore`, `DeleteIgnore`, `GetFriends`, `GetIgnores`, `GetFollowers`, `IsVisibleTo`, `GetWorld`.
  - `repository_test.go` — unit tests per method + concurrent-access `-race` test.
  - `handler_test.go` — direct method calls against a real repo (no `bufconn`, mirrors `modules/login/handler_test.go`).
- `cmd/goscape/app/config.go` — embed `friends.Config` in the root `Config` struct.
- `cmd/goscape/app/modules.go` — `const Friends = "friends"`, `initFriends()` (mirrors `initLogin`), `RegisterModule(Friends, g.initFriends)`, add `Friends` to `SingleBinary`'s dependency list.

**Out of scope (deferred to later slices, each with a deferral tag — see §6):**

- **World-side wiring.** Bridge stays `noopBridges{}` after slice 1; no `modules/world/` changes. Slice 2 adds `FriendsClient` + `grpcFriendsBridge`.
- **SQLite persistence.** In-memory only. Slice 3 swaps the repo to SQL-backed.
- **Server→world push.** `SubscribeUpdates` returns `Unimplemented`. `broadcastWorldToFollowers` is not called from slice 1 handlers; mutation handlers update state and return — no fan-out. Slice 4 ships push.
- **Private-message delivery + persistence.** `PrivateMessage` handler accepts and logs; does not deliver (slice 4) and does not persist (slice 6).
- **RELAY_* admin opcodes.** 9 cross-world admin RPCs (MUTE/KICK/SHUTDOWN/BROADCAST/TRACK/RELOAD/CLEARLOGINS/CLEARLOGOUTS/QUEUESCRIPT). Slice 5.
- **PUBLIC_CHAT_LOG.** Slice 6.
- **`Player.session` UUID generation.** Slice 7. The `PlayerLogin` RPC request shape does not reserve a session field in slice 1; slice 7 adds it.
- **Authentication / authorization between world and friends-server.** Matches `modules/login` posture: intra-cluster trust, no auth. If exposed externally later, both modules need coordinated auth design.
- **Rate limiting, per-RPC quotas.** Not needed for slice 1; matches login posture.
- **Smoke-pack stage.** Friends-server has no on-disk artifact to byte-pin against the TS reference.

---

## §2. Layout

| File | State | Purpose |
|---|---|---|
| `proto/friends/friends.proto` | **NEW** | Service definition: 9 unary RPCs + 1 server-streaming stub. All request messages carry `int32 world_id`. |
| `pkg/friendspb/friends.pb.go` | **NEW (generated)** | Message types. Generated by `protoc-gen-go`. |
| `pkg/friendspb/friends_grpc.pb.go` | **NEW (generated)** | Server/client stubs. Generated by `protoc-gen-go-grpc`. |
| `modules/friends/config.go` | **NEW** | `Config` + `RegisterFlagsAndApplyDefaults` + `Validate`. |
| `modules/friends/friends.go` | **NEW** | `Friends` module + `New` + `NewFriendsService` + lifecycle methods. |
| `modules/friends/server.go` | **NEW** | `grpcServer` wrapper (listen / serve / shutdown). |
| `modules/friends/handler.go` | **NEW** | `handler` struct implementing `friendspb.FriendsServiceServer`. |
| `modules/friends/repository.go` | **NEW** | In-memory `Repository` + `sync.RWMutex`. |
| `modules/friends/repository_test.go` | **NEW** | Per-method unit tests + race-clean concurrent test. |
| `modules/friends/handler_test.go` | **NEW** | Handler tests via direct method calls. |
| `cmd/goscape/app/config.go` | **EDIT** | Embed `friends.Config` (add one struct field + one `RegisterFlagsAndApplyDefaults` call). |
| `cmd/goscape/app/modules.go` | **EDIT** | Add `const Friends`, `initFriends()`, registration call, `SingleBinary` dep. |

No edits to `modules/world/`, `modules/login/`, `modules/asset/`, `internal/dskit/`, `pkg/loginpb/`, or any other existing file.

---

## §3. Architecture

A new gRPC server module sibling to `modules/login`. Listens on its own port (`2005` default). Hosts an in-memory `Repository` tracking per-player presence + friend/ignore sets + per-world player counts.

Each RPC carries an explicit `world_id` field — matches `modules/login`'s `NodeId` convention. The TS friends-server uses a persistent WebSocket per world to keep world identity implicit on the connection; that's a Node-runtime artifact, not load-bearing behavior. gRPC unary is the established cross-module transport for goscape.

`WorldConnect` is the first RPC a world calls. It validates `profile == cfg.NodeProfile` and initializes the world's player-count tracker. TS lazily inits the world on any non-`WorldConnect` first message (`FriendServer.ts:108-115`); slice 1 keeps that lazy-init behavior for TS faithfulness.

Concurrency: single `sync.RWMutex` on the repo. Every handler call takes `repo.mu.Lock()` (or `RLock` for `GetWorld`/`GetFollowers`/`GetFriends`/`GetIgnores`/`IsVisibleTo`). No I/O inside the critical section — every method is `Lock → mutate-or-read → Unlock`. `-race`-clean by construction.

Module lifecycle (mirrors `modules/login/login.go`):
- `starting` — construct repo, construct `grpcServer`, bind listener
- `running` — `grpcServer.serve()`; block on context-done or server error
- `stopping` — graceful stop, close listener if not yet handed to gRPC

The module is registered with `SingleBinary` dependencies, gated by `friends.enable` (default `false`) — same pattern as `modules/login`. Default deployments see no behavior change; only configs flipping the flag start the gRPC server.

---

## §4. Proto contract

`proto/friends/friends.proto`:

```protobuf
syntax = "proto3";

package friends;

option go_package = "github.com/zsrv/goscape/pkg/friendspb";

service FriendsService {
  // Lifecycle. Called once per world on startup; validates profile and
  // initializes the world's player-count slot. Mirrors TS FriendServer
  // WORLD_CONNECT opcode (FriendServer.ts:89-106).
  rpc WorldConnect(WorldConnectRequest) returns (WorldConnectResponse);

  // Player presence. TS PLAYER_LOGIN/PLAYER_LOGOUT/PLAYER_CHAT_SETMODE.
  rpc PlayerLogin(PlayerLoginRequest) returns (PlayerLoginResponse);
  rpc PlayerLogout(PlayerLogoutRequest) returns (PlayerLogoutResponse);
  rpc ChatSetMode(ChatSetModeRequest) returns (ChatSetModeResponse);

  // Social-list mutations. TS FRIENDLIST_ADD/DEL, IGNORELIST_ADD/DEL.
  rpc FriendlistAdd(FriendlistAddRequest) returns (FriendlistAddResponse);
  rpc FriendlistDel(FriendlistDelRequest) returns (FriendlistDelResponse);
  rpc IgnorelistAdd(IgnorelistAddRequest) returns (IgnorelistAddResponse);
  rpc IgnorelistDel(IgnorelistDelRequest) returns (IgnorelistDelResponse);

  // Chat. TS PRIVATE_MESSAGE.
  rpc PrivateMessage(PrivateMessageRequest) returns (PrivateMessageResponse);

  // Server → world push. Server streams update events for a single (world,
  // player) subscription. Slice 1 handler returns Unimplemented; slice 4
  // implements (per-subscriber registry, broadcastWorldToFollowers fan-out).
  rpc SubscribeUpdates(SubscribeUpdatesRequest) returns (stream FriendsUpdate);
}

message WorldConnectRequest {
  int32 world_id = 1;
  string profile = 2;
}
message WorldConnectResponse {}

message PlayerLoginRequest {
  int32 world_id = 1;
  uint64 username37 = 2;
  int32 private_chat = 3;
  int32 staff_lvl = 4;
}
message PlayerLoginResponse {
  // Reserved for slice 4 (per-RPC player-cap surfacing). Slice 1 leaves
  // unset; handler logs warn on cap rejection but accepts the call.
  bool accepted = 1;
}

message PlayerLogoutRequest {
  int32 world_id = 1;
  uint64 username37 = 2;
}
message PlayerLogoutResponse {}

message ChatSetModeRequest {
  int32 world_id = 1;
  uint64 username37 = 2;
  int32 private_chat = 3;
}
message ChatSetModeResponse {}

message FriendlistAddRequest {
  int32 world_id = 1;
  uint64 username37 = 2;
  uint64 target_username37 = 3;
}
message FriendlistAddResponse {}

message FriendlistDelRequest {
  int32 world_id = 1;
  uint64 username37 = 2;
  uint64 target_username37 = 3;
}
message FriendlistDelResponse {}

message IgnorelistAddRequest {
  int32 world_id = 1;
  uint64 username37 = 2;
  uint64 target_username37 = 3;
}
message IgnorelistAddResponse {}

message IgnorelistDelRequest {
  int32 world_id = 1;
  uint64 username37 = 2;
  uint64 target_username37 = 3;
}
message IgnorelistDelResponse {}

message PrivateMessageRequest {
  int32 world_id = 1;
  uint64 username37 = 2;
  uint64 target_username37 = 3;
  int32 staff_lvl = 4;
  uint32 pm_id = 5;
  string chat = 6;
  int32 coord = 7;
}
message PrivateMessageResponse {}

message SubscribeUpdatesRequest {
  int32 world_id = 1;
  uint64 username37 = 2;
}

message FriendsUpdate {
  oneof update {
    FriendlistUpdate friendlist = 1;
    IgnorelistUpdate ignorelist = 2;
    PrivateMessageDelivery private_message = 3;
  }
}

message FriendlistUpdate {
  // (world_id, username37) pairs. world_id=0 means offline/hidden.
  repeated FriendEntry entries = 1;
}
message FriendEntry {
  int32 world_id = 1;
  uint64 username37 = 2;
}

message IgnorelistUpdate {
  repeated uint64 username37 = 1;
}

message PrivateMessageDelivery {
  uint64 from_username37 = 1;
  int32 staff_lvl = 2;
  uint32 pm_id = 3;
  string chat = 4;
}
```

`username37` uses `uint64` on the wire; world-side bridge (slice 2) does the `string → base37` conversion using existing `pkg/jstring` (or equivalent — slice 2 will identify the canonical helper).

`coord` is the packed `coordgrid.PackCoord` value (matches the existing `FriendsBridge.PrivateMessage` signature in `modules/world/bridges.go:28`).

---

## §5. Repository contract

`Repository` is a struct with `sync.RWMutex` and four maps. Methods are TS-faithful ports of `FriendServerRepository.ts`:

```go
type Repository struct {
    mu      sync.RWMutex
    worlds  map[int32]*worldState
    players map[uint64]*playerState
    friends map[uint64]map[uint64]struct{}
    ignores map[uint64]map[uint64]struct{}
}

type worldState struct {
    playerCount int
    limit       int
}

type playerState struct {
    worldId     int32
    privateChat int32 // 0=ON, 1=FRIENDS, 2=OFF (matches TS ChatModePrivate)
    staffLvl    int32
}

func NewRepository() *Repository

// World lifecycle.
// InitializeWorld resets playerCount to 0 and sets the limit.
// Overwrite semantics match TS terminate-then-rebind (FriendServer.ts:412-418).
func (r *Repository) InitializeWorld(worldId int32, limit int)

// Player presence.
// Register returns false iff worldState.playerCount >= worldState.limit.
// Callers must Unregister first to dedupe across worlds (TS does this in
// PLAYER_LOGIN: this.repository.unregister(username37) before register).
func (r *Repository) Register(worldId int32, username37 uint64, privateChat, staffLvl int32) bool
func (r *Repository) Unregister(username37 uint64)
func (r *Repository) SetChatMode(username37 uint64, privateChat int32)

// Social lists. Idempotent.
func (r *Repository) AddFriend(username37, target uint64)
func (r *Repository) DeleteFriend(username37, target uint64)
func (r *Repository) AddIgnore(username37, target uint64)
func (r *Repository) DeleteIgnore(username37, target uint64)

// Read accessors (RLock).
func (r *Repository) GetWorld(username37 uint64) int32     // 0 if not present
func (r *Repository) GetChatMode(username37 uint64) int32  // 0 (ON) if not present
func (r *Repository) GetFriends(username37 uint64) []uint64
func (r *Repository) GetIgnores(username37 uint64) []uint64
// GetFollowers returns usernames who have `target` in their friend list.
// O(n) scan over friends map; acceptable for slice 1 since slice 4 is
// the only consumer and it doesn't ship yet.
func (r *Repository) GetFollowers(target uint64) []uint64
// IsVisibleTo applies TS visibility rules (FriendServerRepository.ts logic):
//   privateChat 0 (ON)      → always visible
//   privateChat 1 (FRIENDS) → visible only if viewer is in other's friends
//   privateChat 2 (OFF)     → never visible
// Note: TS code uses opposite-direction lookups in places; port the
// TS semantics directly, with an inline doc-comment cross-reference to
// FriendServerRepository.ts.
func (r *Repository) IsVisibleTo(viewer, other uint64) bool
```

All methods take `r.mu.Lock()` for writes, `r.mu.RLock()` for reads. No I/O inside critical sections.

---

## §6. Data flow

Slice 1 only exercises outbound (world → friends-server).

**`WorldConnect`:**
```
world → WorldConnect{world_id, profile}
        ├── if profile != cfg.NodeProfile → InvalidArgument (TS closes socket, FriendServer.ts:98-102)
        └── repo.InitializeWorld(world_id, cfg.WorldPlayerLimit)
```

**`PlayerLogin`:**
```
world → PlayerLogin{world_id, username37, private_chat, staff_lvl}
        ├── if !worldInitialized(world_id) → repo.InitializeWorld(world_id, cfg.WorldPlayerLimit)
        │     (TS-faithful lazy init, FriendServer.ts:108-115)
        ├── if private_chat ∉ {0,1,2} → coerce to 0 (ON)
        │     (TS-faithful per FriendServer.ts:120-123. Enum mapping:
        │      ON=0, FRIENDS=1, OFF=2 per Engine-TS ChatModes.ts)
        ├── repo.Unregister(username37)
        ├── accepted := repo.Register(world_id, username37, private_chat, staff_lvl)
        ├── if !accepted → log.Warn("world player-cap reached", ...); response.accepted=false
        │     (slice 4 surfaces this to caller; slice 1 callers ignore the field)
        └── return PlayerLoginResponse{accepted}
```

**`PlayerLogout`:**
```
world → PlayerLogout{world_id, username37}
        ├── if !worldInitialized → lazy init (TS-faithful, FriendServer.ts:144-151)
        └── repo.Unregister(username37)
```

**`ChatSetMode`:**
```
world → ChatSetMode{world_id, username37, private_chat}
        ├── if !worldInitialized → lazy init
        ├── coerce private_chat ∉ {0,1,2} → ON
        └── repo.SetChatMode(username37, private_chat)
```

**`FriendlistAdd` / `FriendlistDel` / `IgnorelistAdd` / `IgnorelistDel`:**
```
world → FriendlistAdd{world_id, username37, target_username37}
        ├── if !worldInitialized → lazy init
        └── repo.AddFriend(username37, target_username37)
```
TS calls `broadcastWorldToFollowers` after each mutation (`FriendServer.ts:200-204` etc.) — slice 1 does **not** broadcast; the call site exists as a TODO comment retired by slice 4. Repo state still mutates correctly so slice 4's stream code sees fresh state.

**`PrivateMessage`:**
```
world → PrivateMessage{world_id, username37, target_username37, staff_lvl, pm_id, chat, coord}
        ├── if !worldInitialized → lazy init
        ├── log.Debug("private message received", ...)
        └── return OK
```
No delivery (slice 4), no persistence (slice 6). Handler doc-comment pins both deferrals.

**`SubscribeUpdates`:**
```
world → SubscribeUpdates{world_id, username37}
        └── return status.Error(codes.Unimplemented, "deferred to slice 4")
```

**Concurrency model:** every handler call enters via the public `Repository.*` method, which acquires the appropriate lock. No long-held locks; no I/O inside critical sections. `-race`-clean by construction.

**Error model:**
- `codes.InvalidArgument` — profile mismatch on `WorldConnect`
- `codes.Unimplemented` — `SubscribeUpdates`
- Player-cap exhaustion does **not** return an error in slice 1 — logs a warn, sets `accepted=false`, returns OK. Slice 4 changes this if needed.
- No `codes.Internal` paths (no I/O can fail in slice 1).

---

## §7. Testing strategy

Two layers, both ship in slice 1.

**Layer 1 — `repository_test.go`** (unit tests, no gRPC):

- `TestRepository_InitializeWorld_OverwritesExisting` — second init for same world resets playerCount
- `TestRepository_Register_RespectsPlayerLimit` — N+1th registration with limit=N returns false; first N return true
- `TestRepository_Register_DedupesAcrossWorlds` — same username37 registered on world A then world B → repo says world B
- `TestRepository_AddFriend_Idempotent` — double-add is a no-op (size unchanged)
- `TestRepository_DeleteFriend_AbsentNoop` — delete on non-friend doesn't error
- `TestRepository_GetFollowers_TraversesCorrectly` — given A→B and C→B (A and C each have B as a friend), `GetFollowers(B)` returns sorted {A,C}
- `TestRepository_IsVisibleTo_ChatModeOff` — privateChat=OFF hides from everyone (including friends)
- `TestRepository_IsVisibleTo_FriendsOnly` — privateChat=FRIENDS hides from non-friends, shows to friends
- `TestRepository_Concurrent_RaceClean` — `t.Parallel()` + N goroutines mixed add/remove/get; runs under `-race`; no assertion beyond "doesn't race or panic"

**Layer 2 — `handler_test.go`** (handler via direct method calls — no `bufconn`, mirrors `modules/login/handler_test.go`):

- `TestHandler_WorldConnect_OK` — happy path; assert repo got initialized via `GetFollowers` returning empty (proxy for world-existence)
- `TestHandler_WorldConnect_ProfileMismatch` — returns `codes.InvalidArgument`
- `TestHandler_PlayerLogin_BeforeWorldConnect_LazyInit` — call PlayerLogin without prior WorldConnect; assert repo has the player + world is now initialized
- `TestHandler_PlayerLogin_PrivateChatCoercion` — privateChat=99 → coerced to 0 (ON); assert via `repo.GetChatMode(username37) == 0`
- `TestHandler_PlayerLogin_PlayerCapAccepted_False` — fill world to limit, N+1th login → response.accepted=false, log captured
- `TestHandler_PlayerLogout_Idempotent` — logout on unknown username returns OK
- `TestHandler_ChatSetMode_UpdatesState` — observe via visibility rule change
- `TestHandler_FriendlistAdd_Persists` — read back via repo
- `TestHandler_FriendlistDel_RemovesEntry`
- `TestHandler_IgnorelistAdd_Persists`
- `TestHandler_IgnorelistDel_RemovesEntry`
- `TestHandler_PrivateMessage_NoOp_Slice1` — returns OK; pin doc-comment with `// NAI-S1-D-PM-NO-DELIVERY`
- `TestHandler_SubscribeUpdates_Unimplemented` — pin contract until slice 4 lands

**No `cmd/goscape/app/`-level integration test** — slice 2 will add a world↔friends end-to-end smoke once `FriendsClient` exists. Slice 1's module-lifecycle correctness is delegated to the dskit `BasicService` machinery already exercised by `modules/login`.

**No smoke-pack stage.** Friends-server produces no on-disk artifacts.

**Done criterion:**
- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/...` clean
- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...` clean (no regressions)
- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...` clean
- Smoke-pack baseline (12 OK / 0 ERR) unchanged

---

## §8. Deviation tags

**Opens (live after slice 1):**

| Tag | Site | Retired by |
|---|---|---|
| `NAI-S1-D-INMEMORY-REPO` | `modules/friends/repository.go` package doc | Slice 3 (SQLite swap) |
| `NAI-S1-D-NO-FOLLOWER-BROADCAST` | `handler.go` Friendlist*/Ignorelist*/ChatSetMode handlers (post-mutation comment) | Slice 4 (SubscribeUpdates impl) |
| `NAI-S1-D-PM-NO-DELIVERY` | `handler.go` `PrivateMessage` | Slice 4 |
| `NAI-S1-D-PM-NO-PERSISTENCE` | `handler.go` `PrivateMessage` | Slice 6 (chat logging) |
| `NAI-S1-D-PLAYERCAP-LOG-ONLY` | `handler.go` `PlayerLogin` (cap rejection → log warn, response.accepted=false but no caller consumes) | Slice 4 |
| `NAI-S1-D-LAZY-WORLDINIT` | `handler.go` per-RPC "if world unknown, init" branch | None — TS-faithful, kept permanently; tag exists for reviewer traceability |

**Retires:** none.

The existing `NAI-72-D-FRIENDS-SERVER-BRIDGE` tag (8 references in `modules/world/` + spec docs) is **not** retired by slice 1 — it points to "the world's friends bridge is still `noopBridges{}`," which slice 2 fixes. Slice 1 makes the server exist; the world doesn't talk to it yet.

**Carry-forward inherited from NAI-72:**
- `NAI-72-D-FRIENDS-SERVER-BRIDGE` (slice 2 retires)
- `Player.session` UUID assignment (slice 7)

---

## §9. Module integration

`cmd/goscape/app/modules.go` edits (additive, mirrors `Login` block):

```go
const (
    Asset   string = "asset"
    Friends string = "friends" // NEW
    Login   string = "login"
    World   string = "world"
    SingleBinary string = "all"
)

// initFriends mirrors initLogin.
func (g *App) initFriends() (services.Service, error) {
    if !g.cfg.Friends.Enable {
        return services.NewIdleService(nil, nil), nil
    }
    logger, err := log.NewLogger(g.cfg.LogLevel, g.cfg.LogFormat, os.Stdout)
    if err != nil { ... }
    return friends.NewFriendsService(g.cfg.Friends, logger)
}

mm.RegisterModule(Friends, g.initFriends) // NEW

deps := map[string][]string{
    ...
    SingleBinary: {Asset, Friends, Login, World}, // EDIT: add Friends
}
```

`cmd/goscape/app/config.go` edits (embed):

```go
type Config struct {
    ...
    Asset   asset.Config   `yaml:"asset"`
    Friends friends.Config `yaml:"friends"` // NEW
    Login   login.Config   `yaml:"login"`
    World   world.Config   `yaml:"world"`
}
```

`RegisterFlagsAndApplyDefaults` gets one new line: `c.Friends.RegisterFlagsAndApplyDefaults(f)`.

---

## §10. Memory closure

**On slice 1 close, write memory:** `[[friends-server-slice1-close]]` (or whatever NAI ticket number gets assigned at execution time).

Cross-link `[[nai214-login-moderation-bridge-close]]` as the precedent close that established the bridge-mod shape this slice extends to friends.

Update `MEMORY.md` index with a one-line summary listing: commit range, new module path, the 6 deviation tags opened, repo-LOC count, test counts, and confirmation that `go test -race ./...` + smoke-pack baseline hold.

Slice 2's spec (drafted after slice 1 closes) will retire `NAI-72-D-FRIENDS-SERVER-BRIDGE` and reference `[[friends-server-slice1-close]]` as its predecessor.
