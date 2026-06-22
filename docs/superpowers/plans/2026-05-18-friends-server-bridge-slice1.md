# Friends-server bridge — Slice 1 implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land `pkg/friendspb` (gRPC contract for the friends-server) and `modules/friends/` (dskit module hosting an in-memory friend/ignore repository + 9 unary RPCs + 1 server-streaming Unimplemented stub), gated by `friends.enable` (default `false`). No world-side changes — slice 2 wires `modules/world/` to call this server.

**Architecture:** Sibling-to-`modules/login` throughout. Proto package `friends.v1`, generated Go at `pkg/friendspb/`. New module at `modules/friends/` with the same file shape as `modules/login/` minus the DB layer (`config.go` / `friends.go` / `server.go` / `handler.go` / `repository.go`). Single `sync.RWMutex` on the repo; no I/O inside critical sections. Module registered in `cmd/goscape/app/modules.go` as a peer of `Login`, added to the `SingleBinary` dependency list.

**Tech Stack:** Go 1.26+; `google.golang.org/grpc`; `google.golang.org/protobuf`; `log/slog`. Proto generation via `make proto` (wraps `buf generate`, config in `buf.yaml` + `buf.gen.yaml`). No new direct dependencies, no DB migrations.

**Spec:** `docs/superpowers/specs/2026-05-18-friends-server-bridge-slice1-design.md` (commit `b724ce37`)

---

## Pre-flight context

Before starting Task 1, an executor unfamiliar with the codebase should know:

- **Project rules (CLAUDE.md):** All `go` invocations must be prefixed `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`. All `git commit` must pass `--no-gpg-sign`.
- **Modern Go:** Project is on Go 1.26+. Invoke the `use-modern-go` skill before writing any Go code in this slice. Notable: prefer `any` over `interface{}`, prefer `range N` over `range 0..N`, use `slices`/`maps` stdlib helpers, use `t.Context()` in tests instead of `context.Background()`.
- **TDD cadence:** Write failing test → run → confirm failure → minimal impl → run → confirm pass → commit. Don't skip the "confirm failure" step (it's how you know the test exercises new code, not existing code).
- **Sibling reference module:** `modules/login/` is the canonical dskit-module pattern. When in doubt about file structure or naming, mirror what `modules/login/<same-file>.go` does. Specifically:
  - `config.go` exports `Config` struct with `RegisterFlagsAndApplyDefaults(f *flag.FlagSet)` + `Validate() error`
  - `login.go` exports a top-level struct (here: `Friends`) with `New(cfg, logger)` + `NewLoginService(cfg, logger) (services.Service, error)` factory + `starting`/`running`/`stopping` lifecycle methods
  - `server.go` exports `grpcServer` wrapper with `newGRPCServer(cfg, deps...)` + `listen` + `serve` + `shutdown`
  - `handler.go` defines unexported `handler` struct embedding `loginpb.UnimplementedLoginServiceServer`; methods on `*handler` implement the gRPC service
  - `handler_test.go` constructs `handler` directly (no `bufconn`, no real gRPC channel) and calls methods as plain Go calls
- **Proto package naming:** `proto/login/login.proto` declares `package login.v1` and `option go_package = "github.com/zsrv/goscape/pkg/loginpb"`. Friends follows the identical pattern: `package friends.v1` + `option go_package = "github.com/zsrv/goscape/pkg/friendspb"`.
- **Proto generation:** `make proto` runs `buf generate` (config in `buf.yaml` + `buf.gen.yaml`). Generated `*.pb.go` and `*_grpc.pb.go` files ARE committed to the repo (look at `pkg/loginpb/`). Treat them as build artifacts — never edit by hand, always regenerate.
- **`buf` toolchain prerequisites:** `protoc-gen-go` and `protoc-gen-go-grpc` must be on `PATH` (the Makefile prepends `$(go env GOPATH)/bin`). If `make proto` errors with "plugin not found," install with: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest`, then re-run with the same `PATH` augmentation as the Makefile.
- **In-memory test DSN pattern:** `modules/login/db_test.go:15-24`'s `createTestDB(t)` uses an in-memory SQLite DSN. We don't need it (no DB), but the convention of a `t.Helper()` factory returning a fresh repo per test is the right pattern to mirror for `Repository`.
- **`noopLogger()` helper:** `modules/login/db_test.go:42-44` returns a `*slog.Logger` that discards all output. Worth copying into `modules/friends/repository_test.go` (or `handler_test.go`) so the module is self-contained.
- **Module gating pattern:** `cmd/goscape/app/modules.go:73-93` (`initLogin`) checks `g.cfg.Login.Enable` and returns `services.NewIdleService(nil, nil)` when disabled. Mirror this in `initFriends`. The default `Enable=false` keeps the module a no-op for existing deploys.
- **No `Login` import cycle risk:** `modules/world/` does NOT import `modules/login/`, and `modules/friends/` similarly will NOT import `modules/world/`. The friends-server is a peer module that the world (in slice 2) will dial via a gRPC client. Slice 1's friends-server has no compile-time dependency on `modules/world/`.
- **TS reference (read-only, do not modify):** `$HOME/Code/github.com/LostCityRS/Engine-TS/src/server/friend/FriendServer.ts` (the WebSocket+JSON port we're translating to gRPC) and `FriendServerRepository.ts` (the persistence layer — slice 1 ports its in-memory semantics; slice 3 swaps to SQLite). When TS semantics are unclear, read these files directly.
- **`ChatModePrivate` enum mapping** (TS `src/engine/entity/ChatModes.ts` and `FriendServer.ts:120-123`): `ON=0`, `FRIENDS=1`, `OFF=2`. Invalid values coerce to `ON` (i.e. `0`).
- **Deviation tags introduced by this slice** (per spec §8): six new tags, all prefixed `NAI-S1-D-*`. Each gets a doc-comment at its mentioned site with a `Retired by: slice <N>` annotation. None are retired in slice 1.

---

## File structure

| File | State | Responsibility |
|---|---|---|
| `proto/friends/friends.proto` | CREATE | Service definition: 10 RPCs (9 unary + 1 server-streaming). Package `friends.v1`. |
| `pkg/friendspb/friends.pb.go` | CREATE (generated) | Message types. Regenerated via `make proto`. |
| `pkg/friendspb/friends_grpc.pb.go` | CREATE (generated) | Server/client stubs. Regenerated via `make proto`. |
| `modules/friends/config.go` | CREATE | `Config` struct, `RegisterFlagsAndApplyDefaults`, `Validate`. ~40 LOC. |
| `modules/friends/repository.go` | CREATE | In-memory `Repository` with `sync.RWMutex`. 13 methods. ~200 LOC. |
| `modules/friends/repository_test.go` | CREATE | Unit tests per repo method + `-race`-clean concurrent test. ~250 LOC. |
| `modules/friends/handler.go` | CREATE | `handler` struct implementing `friendspb.FriendsServiceServer`. ~180 LOC. |
| `modules/friends/handler_test.go` | CREATE | Direct method calls against a real repo. ~300 LOC. |
| `modules/friends/server.go` | CREATE | `grpcServer` wrapper (`listen` / `serve` / `shutdown`). ~50 LOC. |
| `modules/friends/friends.go` | CREATE | `Friends` module struct + `New` + `NewFriendsService` + lifecycle methods. ~80 LOC. |
| `cmd/goscape/app/config.go` | MODIFY | Embed `friends.Config` (one field + one `RegisterFlagsAndApplyDefaults` call). |
| `cmd/goscape/app/modules.go` | MODIFY | Add `const Friends`, `initFriends()`, registration call, `SingleBinary` dep edit. |

No edits to `modules/world/`, `modules/login/`, `modules/asset/`, `internal/dskit/`, `pkg/loginpb/`, or any other existing file.

---

## Task 1: Define the proto contract and regenerate Go stubs

**Why this task exists:** Locks the wire shape before any Go code references it. Slice 2's `FriendsClient` interface in `modules/world/` will be designed against this contract, so getting it right (or at least committed) up front matters. Generated stubs land in the same commit as the proto file — that's the repo convention (see `pkg/loginpb/*.pb.go` are tracked).

**Files:**
- Create: `proto/friends/friends.proto`
- Create: `pkg/friendspb/friends.pb.go` (generated, do not hand-edit)
- Create: `pkg/friendspb/friends_grpc.pb.go` (generated, do not hand-edit)

- [ ] **Step 1.1: Write the proto file**

Create `proto/friends/friends.proto`:

```protobuf
syntax = "proto3";
package friends.v1;

import "google/protobuf/empty.proto";

option go_package = "github.com/zsrv/goscape/pkg/friendspb";

service FriendsService {
  // Lifecycle. Called once per world on startup; validates profile and
  // initializes the world's player-count slot. Mirrors TS FriendServer
  // WORLD_CONNECT opcode (FriendServer.ts:89-106).
  rpc WorldConnect(WorldConnectRequest) returns (google.protobuf.Empty);

  // Player presence. Mirrors TS PLAYER_LOGIN/PLAYER_LOGOUT/PLAYER_CHAT_SETMODE.
  rpc PlayerLogin(PlayerLoginRequest)     returns (PlayerLoginResponse);
  rpc PlayerLogout(PlayerLogoutRequest)   returns (google.protobuf.Empty);
  rpc ChatSetMode(ChatSetModeRequest)     returns (google.protobuf.Empty);

  // Social-list mutations. Mirrors TS FRIENDLIST_ADD/DEL, IGNORELIST_ADD/DEL.
  rpc FriendlistAdd(FriendlistAddRequest) returns (google.protobuf.Empty);
  rpc FriendlistDel(FriendlistDelRequest) returns (google.protobuf.Empty);
  rpc IgnorelistAdd(IgnorelistAddRequest) returns (google.protobuf.Empty);
  rpc IgnorelistDel(IgnorelistDelRequest) returns (google.protobuf.Empty);

  // Chat. Mirrors TS PRIVATE_MESSAGE. Slice 1 accepts and logs; delivery
  // is deferred to slice 4, persistence to slice 6.
  rpc PrivateMessage(PrivateMessageRequest) returns (google.protobuf.Empty);

  // Server -> world push. Server streams update events for a single
  // (world, player) subscription. Slice 1 handler returns Unimplemented;
  // slice 4 implements (per-subscriber registry, broadcastWorldToFollowers
  // fan-out).
  rpc SubscribeUpdates(SubscribeUpdatesRequest) returns (stream FriendsUpdate);
}

message WorldConnectRequest {
  int32  world_id = 1;
  string profile  = 2;
}

message PlayerLoginRequest {
  int32  world_id     = 1;
  uint64 username37   = 2;
  int32  private_chat = 3;
  int32  staff_lvl    = 4;
}

message PlayerLoginResponse {
  // Reserved for slice 4 (per-RPC player-cap surfacing). Slice 1 leaves
  // unset; handler logs warn on cap rejection but accepts the call.
  bool accepted = 1;
}

message PlayerLogoutRequest {
  int32  world_id   = 1;
  uint64 username37 = 2;
}

message ChatSetModeRequest {
  int32  world_id     = 1;
  uint64 username37   = 2;
  int32  private_chat = 3;
}

message FriendlistAddRequest {
  int32  world_id          = 1;
  uint64 username37        = 2;
  uint64 target_username37 = 3;
}

message FriendlistDelRequest {
  int32  world_id          = 1;
  uint64 username37        = 2;
  uint64 target_username37 = 3;
}

message IgnorelistAddRequest {
  int32  world_id          = 1;
  uint64 username37        = 2;
  uint64 target_username37 = 3;
}

message IgnorelistDelRequest {
  int32  world_id          = 1;
  uint64 username37        = 2;
  uint64 target_username37 = 3;
}

message PrivateMessageRequest {
  int32  world_id          = 1;
  uint64 username37        = 2;
  uint64 target_username37 = 3;
  int32  staff_lvl         = 4;
  uint32 pm_id             = 5;
  string chat              = 6;
  int32  coord             = 7;
}

message SubscribeUpdatesRequest {
  int32  world_id   = 1;
  uint64 username37 = 2;
}

message FriendsUpdate {
  oneof update {
    FriendlistUpdate       friendlist      = 1;
    IgnorelistUpdate       ignorelist      = 2;
    PrivateMessageDelivery private_message = 3;
  }
}

message FriendlistUpdate {
  // (world_id, username37) pairs. world_id=0 means offline/hidden.
  repeated FriendEntry entries = 1;
}

message FriendEntry {
  int32  world_id   = 1;
  uint64 username37 = 2;
}

message IgnorelistUpdate {
  repeated uint64 username37 = 1;
}

message PrivateMessageDelivery {
  uint64 from_username37 = 1;
  int32  staff_lvl       = 2;
  uint32 pm_id           = 3;
  string chat            = 4;
}
```

- [ ] **Step 1.2: Generate the Go stubs**

Run: `make proto`
Expected: `pkg/friendspb/friends.pb.go` and `pkg/friendspb/friends_grpc.pb.go` are created (or updated). Exit 0. No diff to `pkg/loginpb/*` (other modules' generated files are unchanged).

If `make proto` fails with "plugin not found," install the plugins first:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
PATH="$TMPDIR/go/bin:$PATH" make proto
```

- [ ] **Step 1.3: Verify the generated code compiles**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/friendspb/...`
Expected: Exit 0, no output.

- [ ] **Step 1.4: Verify generated symbols exist**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go doc github.com/zsrv/goscape/pkg/friendspb FriendsServiceServer`
Expected: Output shows interface with 10 methods: `WorldConnect`, `PlayerLogin`, `PlayerLogout`, `ChatSetMode`, `FriendlistAdd`, `FriendlistDel`, `IgnorelistAdd`, `IgnorelistDel`, `PrivateMessage`, `SubscribeUpdates`, plus `mustEmbedUnimplementedFriendsServiceServer`.

- [ ] **Step 1.5: Commit**

```bash
git add proto/friends/friends.proto pkg/friendspb/
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: define friends-server gRPC contract

Slice 1 of 7 of the friends-server bridge (NAI-72-D-FRIENDS-SERVER-BRIDGE).
Adds proto/friends/friends.proto with 9 unary outbound RPCs plus a
SubscribeUpdates server-streaming RPC (slice 4 fills the handler).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Add `modules/friends/config.go`

**Why this task exists:** Every dskit module starts with a `Config` struct that registers its flags. Landing this first means subsequent tasks can reference real `Config` fields (e.g. `cfg.NodeProfile`, `cfg.WorldPlayerLimit`) instead of placeholders.

**Files:**
- Create: `modules/friends/config.go`

- [ ] **Step 2.1: Write `Config` struct, flag registration, and `Validate`**

Create `modules/friends/config.go`:

```go
package friends

import (
	"flag"
	"time"
)

// Config holds the friends-server module's runtime configuration.
type Config struct {
	GRPCListenAddress       string        `yaml:"grpc_listen_address"`
	NodeProfile             string        `yaml:"node_profile"`
	GRPCListenPort          int           `yaml:"grpc_listen_port"`
	WorldPlayerLimit        int           `yaml:"world_player_limit"`
	Enable                  bool          `yaml:"enable"`
	GracefulShutdownTimeout time.Duration `yaml:"graceful_shutdown_timeout"`
}

func (c *Config) RegisterFlagsAndApplyDefaults(f *flag.FlagSet) {
	f.StringVar(&c.GRPCListenAddress, "friends.grpc-listen-address", "127.0.0.1", "Friends server gRPC listen address.")
	f.IntVar(&c.GRPCListenPort, "friends.grpc-listen-port", 2005, "Friends server gRPC listen port.")
	f.StringVar(&c.NodeProfile, "friends.node-profile", "main", "Profile name validated at WorldConnect.")
	f.IntVar(&c.WorldPlayerLimit, "friends.world-player-limit", 2000, "Per-world player slot cap.")
	f.BoolVar(&c.Enable, "friends.enable", false, "Whether to run the friends module.")
	f.DurationVar(&c.GracefulShutdownTimeout, "friends.graceful-shutdown-timeout", 30*time.Second, "Timeout for graceful gRPC server shutdown.")
}

func (c *Config) Validate() error {
	return nil
}
```

- [ ] **Step 2.2: Verify it compiles**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/friends/...`
Expected: Exit 0, no output.

- [ ] **Step 2.3: Commit**

```bash
git add modules/friends/config.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: add module config

Sibling to modules/login/config.go. Flags: friends.enable (default false),
friends.grpc-listen-{address,port} (127.0.0.1:2005), friends.node-profile
(main), friends.world-player-limit (2000), friends.graceful-shutdown-timeout
(30s). Validate is a no-op for slice 1.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Add `Repository` core state + presence methods (`InitializeWorld` / `Register` / `Unregister` / `GetWorld`)

**Why this task exists:** Establishes the repo type and its locking discipline, plus the four methods the handler needs for `WorldConnect` / `PlayerLogin` / `PlayerLogout`. Splits the full 13-method repo into a smaller, reviewable chunk; Task 4 adds the social-list half.

**Files:**
- Create: `modules/friends/repository.go`
- Create: `modules/friends/repository_test.go`

- [ ] **Step 3.1: Write failing test for `NewRepository` returning a usable empty repo**

Create `modules/friends/repository_test.go`:

```go
package friends

import (
	"io"
	"log/slog"
	"sync"
	"testing"
)

// noopLogger returns a *slog.Logger that discards all output.
// Mirrors modules/login/db_test.go:42-44.
func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRepository_NewRepository_Empty(t *testing.T) {
	r := NewRepository()
	if got := r.GetWorld(0xDEADBEEF); got != 0 {
		t.Errorf("GetWorld on empty repo: got %d, want 0", got)
	}
}
```

- [ ] **Step 3.2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run TestRepository_NewRepository_Empty -v`
Expected: FAIL with "undefined: NewRepository" (or similar build error).

- [ ] **Step 3.3: Write minimal `Repository` + `NewRepository` + `GetWorld`**

Create `modules/friends/repository.go`:

```go
// Package friends hosts the friends-server gRPC module. The in-memory
// repository here is slice 1's persistence stand-in; slice 3 swaps it
// for a SQLite-backed equivalent without changing this method surface.
//
// NAI-S1-D-INMEMORY-REPO — state is lost on restart. Retired by slice 3.
package friends

import "sync"

// Repository is the in-memory friend/ignore/presence store. All methods
// are safe for concurrent use; a single sync.RWMutex guards every map.
// No I/O happens inside critical sections.
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

// playerState tracks a logged-in player's world placement and chat-mode
// settings. privateChat is the TS ChatModePrivate enum: 0=ON, 1=FRIENDS,
// 2=OFF (see Engine-TS ChatModes.ts).
type playerState struct {
	worldId     int32
	privateChat int32
	staffLvl    int32
}

func NewRepository() *Repository {
	return &Repository{
		worlds:  make(map[int32]*worldState),
		players: make(map[uint64]*playerState),
		friends: make(map[uint64]map[uint64]struct{}),
		ignores: make(map[uint64]map[uint64]struct{}),
	}
}

// GetWorld returns the world id the player is logged in to, or 0 if the
// player is not currently registered.
func (r *Repository) GetWorld(username37 uint64) int32 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if ps, ok := r.players[username37]; ok {
		return ps.worldId
	}
	return 0
}
```

- [ ] **Step 3.4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run TestRepository_NewRepository_Empty -v`
Expected: PASS.

- [ ] **Step 3.5: Write failing test for `InitializeWorld` overwrite semantics**

Append to `modules/friends/repository_test.go`:

```go
func TestRepository_InitializeWorld_OverwritesExisting(t *testing.T) {
	r := NewRepository()
	r.InitializeWorld(7, 5)
	// Register one player so playerCount=1.
	if !r.Register(7, 0xAAAA, 0, 0) {
		t.Fatalf("first Register: got false, want true")
	}
	// Re-init the world: playerCount must reset to 0.
	r.InitializeWorld(7, 5)
	// Five fresh registrations must all succeed.
	for i := uint64(1); i <= 5; i++ {
		if !r.Register(7, i, 0, 0) {
			t.Errorf("Register #%d after re-init: got false, want true", i)
		}
	}
}
```

- [ ] **Step 3.6: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run TestRepository_InitializeWorld_OverwritesExisting -v`
Expected: FAIL with "undefined: InitializeWorld" or "undefined: Register".

- [ ] **Step 3.7: Implement `InitializeWorld`, `Register`, `Unregister`**

Append to `modules/friends/repository.go`:

```go
// InitializeWorld (re)creates the per-world player-count slot, resetting
// playerCount to 0 and setting the limit. Mirrors TS FriendServer
// initializeWorld (FriendServer.ts:412-418) where re-init implicitly
// drops any prior socket binding; here it simply resets the counter.
func (r *Repository) InitializeWorld(worldId int32, limit int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.worlds[worldId] = &worldState{playerCount: 0, limit: limit}
}

// Register places the player on the given world. Returns false iff the
// world's playerCount has already hit its limit. Callers must Unregister
// the player first to dedupe across worlds (TS does this in PLAYER_LOGIN,
// FriendServer.ts:125-127). worldId must have been initialized.
func (r *Repository) Register(worldId int32, username37 uint64, privateChat, staffLvl int32) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	ws, ok := r.worlds[worldId]
	if !ok {
		// Lazy-init for the player-cap path: a Register without prior
		// InitializeWorld means the world is unknown. Lazy-create with
		// a sentinel "no limit" of 0 so the cap check below trips
		// immediately and the caller can observe the false return.
		return false
	}
	if ws.playerCount >= ws.limit {
		return false
	}
	r.players[username37] = &playerState{
		worldId:     worldId,
		privateChat: privateChat,
		staffLvl:    staffLvl,
	}
	ws.playerCount++
	return true
}

// Unregister removes the player from whichever world they're on and
// decrements that world's playerCount. No-op if the player is not
// registered (TS FriendServer unregister is also a no-op on miss).
func (r *Repository) Unregister(username37 uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ps, ok := r.players[username37]
	if !ok {
		return
	}
	if ws, ok := r.worlds[ps.worldId]; ok && ws.playerCount > 0 {
		ws.playerCount--
	}
	delete(r.players, username37)
}
```

- [ ] **Step 3.8: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run TestRepository_InitializeWorld_OverwritesExisting -v`
Expected: PASS.

- [ ] **Step 3.9: Write failing tests for the rest of the presence behaviors**

Append to `modules/friends/repository_test.go`:

```go
func TestRepository_Register_RespectsPlayerLimit(t *testing.T) {
	r := NewRepository()
	r.InitializeWorld(1, 3)
	for i := uint64(1); i <= 3; i++ {
		if !r.Register(1, i, 0, 0) {
			t.Fatalf("Register %d: got false, want true", i)
		}
	}
	if r.Register(1, 99, 0, 0) {
		t.Errorf("Register beyond limit: got true, want false")
	}
}

func TestRepository_Register_DedupesAcrossWorlds(t *testing.T) {
	r := NewRepository()
	r.InitializeWorld(1, 10)
	r.InitializeWorld(2, 10)
	if !r.Register(1, 0xAAAA, 0, 0) {
		t.Fatalf("Register on world 1: got false, want true")
	}
	// Move the player to world 2 without explicit Unregister — TS does
	// Unregister-then-Register in PLAYER_LOGIN. Here we just verify that
	// after caller dedupes, the player is on world 2.
	r.Unregister(0xAAAA)
	if !r.Register(2, 0xAAAA, 0, 0) {
		t.Fatalf("Register on world 2: got false, want true")
	}
	if got := r.GetWorld(0xAAAA); got != 2 {
		t.Errorf("GetWorld after move: got %d, want 2", got)
	}
}

func TestRepository_Register_UninitializedWorld_ReturnsFalse(t *testing.T) {
	r := NewRepository()
	if r.Register(42, 0xAAAA, 0, 0) {
		t.Errorf("Register on uninitialized world: got true, want false")
	}
}

func TestRepository_Unregister_UnknownPlayer_NoOp(t *testing.T) {
	r := NewRepository()
	r.InitializeWorld(1, 10)
	r.Unregister(0xDEADBEEF) // must not panic
}
```

- [ ] **Step 3.10: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run TestRepository -v`
Expected: All 5 tests PASS.

- [ ] **Step 3.11: Commit**

```bash
git add modules/friends/repository.go modules/friends/repository_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: add in-memory repository — presence half

Repository core: NewRepository, GetWorld, InitializeWorld, Register,
Unregister. Single sync.RWMutex. Player-cap enforced at Register;
Register on uninitialized world returns false. Tests cover overwrite
semantics, cap enforcement, cross-world dedupe, and Unregister-on-miss.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Add `Repository` social-list methods + chat-mode + accessors

**Why this task exists:** Completes the repository surface — friend/ignore add/del, chat-mode set/get, friend/ignore list accessors, and `GetFollowers` / `IsVisibleTo` for slice 4's broadcast. After this task the repository is feature-complete for slice 1.

**Files:**
- Modify: `modules/friends/repository.go`
- Modify: `modules/friends/repository_test.go`

- [ ] **Step 4.1: Write failing tests for chat mode + friend/ignore mutations**

Append to `modules/friends/repository_test.go`:

```go
func TestRepository_SetChatMode_Updates(t *testing.T) {
	r := NewRepository()
	r.InitializeWorld(1, 10)
	r.Register(1, 0xAAAA, 0, 0)
	r.SetChatMode(0xAAAA, 2)
	if got := r.GetChatMode(0xAAAA); got != 2 {
		t.Errorf("GetChatMode after Set: got %d, want 2", got)
	}
}

func TestRepository_GetChatMode_UnknownPlayer_ReturnsZero(t *testing.T) {
	r := NewRepository()
	if got := r.GetChatMode(0xDEADBEEF); got != 0 {
		t.Errorf("GetChatMode on unknown: got %d, want 0", got)
	}
}

func TestRepository_SetChatMode_UnknownPlayer_NoOp(t *testing.T) {
	r := NewRepository()
	r.SetChatMode(0xDEADBEEF, 2) // must not panic
}

func TestRepository_AddFriend_Idempotent(t *testing.T) {
	r := NewRepository()
	r.AddFriend(0xAAAA, 0xBBBB)
	r.AddFriend(0xAAAA, 0xBBBB)
	got := r.GetFriends(0xAAAA)
	if len(got) != 1 || got[0] != 0xBBBB {
		t.Errorf("GetFriends after double-add: got %v, want [0xBBBB]", got)
	}
}

func TestRepository_DeleteFriend_AbsentNoOp(t *testing.T) {
	r := NewRepository()
	r.DeleteFriend(0xAAAA, 0xBBBB) // must not panic
	if got := r.GetFriends(0xAAAA); len(got) != 0 {
		t.Errorf("GetFriends after delete-missing: got %v, want empty", got)
	}
}

func TestRepository_AddIgnore_Idempotent(t *testing.T) {
	r := NewRepository()
	r.AddIgnore(0xAAAA, 0xBBBB)
	r.AddIgnore(0xAAAA, 0xBBBB)
	got := r.GetIgnores(0xAAAA)
	if len(got) != 1 || got[0] != 0xBBBB {
		t.Errorf("GetIgnores after double-add: got %v, want [0xBBBB]", got)
	}
}

func TestRepository_DeleteIgnore_Removes(t *testing.T) {
	r := NewRepository()
	r.AddIgnore(0xAAAA, 0xBBBB)
	r.DeleteIgnore(0xAAAA, 0xBBBB)
	if got := r.GetIgnores(0xAAAA); len(got) != 0 {
		t.Errorf("GetIgnores after delete: got %v, want empty", got)
	}
}
```

- [ ] **Step 4.2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run TestRepository -v`
Expected: New tests FAIL with "undefined: SetChatMode / GetChatMode / AddFriend / ...". Existing tests still PASS.

- [ ] **Step 4.3: Implement chat-mode and social-list mutations + accessors**

Append to `modules/friends/repository.go`:

```go
// SetChatMode updates the player's privateChat setting. No-op if the
// player is not registered.
func (r *Repository) SetChatMode(username37 uint64, privateChat int32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ps, ok := r.players[username37]; ok {
		ps.privateChat = privateChat
	}
}

// GetChatMode returns the player's privateChat setting, or 0 (ON) if the
// player is not registered.
func (r *Repository) GetChatMode(username37 uint64) int32 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if ps, ok := r.players[username37]; ok {
		return ps.privateChat
	}
	return 0
}

// AddFriend adds target to username37's friend set. Idempotent.
func (r *Repository) AddFriend(username37, target uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	set, ok := r.friends[username37]
	if !ok {
		set = make(map[uint64]struct{})
		r.friends[username37] = set
	}
	set[target] = struct{}{}
}

// DeleteFriend removes target from username37's friend set. No-op if absent.
func (r *Repository) DeleteFriend(username37, target uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if set, ok := r.friends[username37]; ok {
		delete(set, target)
	}
}

// AddIgnore adds target to username37's ignore set. Idempotent.
func (r *Repository) AddIgnore(username37, target uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	set, ok := r.ignores[username37]
	if !ok {
		set = make(map[uint64]struct{})
		r.ignores[username37] = set
	}
	set[target] = struct{}{}
}

// DeleteIgnore removes target from username37's ignore set. No-op if absent.
func (r *Repository) DeleteIgnore(username37, target uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if set, ok := r.ignores[username37]; ok {
		delete(set, target)
	}
}

// GetFriends returns a copy of the player's friend set in unspecified order.
// Returns nil if the player has no friends.
func (r *Repository) GetFriends(username37 uint64) []uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	set, ok := r.friends[username37]
	if !ok || len(set) == 0 {
		return nil
	}
	out := make([]uint64, 0, len(set))
	for u := range set {
		out = append(out, u)
	}
	return out
}

// GetIgnores returns a copy of the player's ignore set in unspecified order.
// Returns nil if the player has no ignores.
func (r *Repository) GetIgnores(username37 uint64) []uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	set, ok := r.ignores[username37]
	if !ok || len(set) == 0 {
		return nil
	}
	out := make([]uint64, 0, len(set))
	for u := range set {
		out = append(out, u)
	}
	return out
}
```

- [ ] **Step 4.4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run TestRepository -v`
Expected: All tests PASS.

- [ ] **Step 4.5: Write failing tests for `GetFollowers` + `IsVisibleTo`**

First, extend the import block at the top of `modules/friends/repository_test.go` to include `"slices"` (alphabetical between `"log/slog"` and `"sync"`):

```go
import (
	"io"
	"log/slog"
	"slices"
	"sync"
	"testing"
)
```

Then append:

```go
func TestRepository_GetFollowers_TraversesCorrectly(t *testing.T) {
	r := NewRepository()
	// A friends B; C friends B; D friends X (irrelevant).
	r.AddFriend(0xAAAA, 0xBBBB)
	r.AddFriend(0xCCCC, 0xBBBB)
	r.AddFriend(0xDDDD, 0xEEEE)
	got := r.GetFollowers(0xBBBB)
	slices.Sort(got)
	want := []uint64{0xAAAA, 0xCCCC}
	if !slices.Equal(got, want) {
		t.Errorf("GetFollowers(B): got %v, want %v", got, want)
	}
}

func TestRepository_GetFollowers_NoFollowers_Nil(t *testing.T) {
	r := NewRepository()
	if got := r.GetFollowers(0xBBBB); got != nil {
		t.Errorf("GetFollowers on no-followers: got %v, want nil", got)
	}
}

func TestRepository_IsVisibleTo_ChatModeOn_Always(t *testing.T) {
	r := NewRepository()
	r.InitializeWorld(1, 10)
	r.Register(1, 0xAAAA, 0, 0) // privateChat=ON
	if !r.IsVisibleTo(0xBBBB, 0xAAAA) {
		t.Errorf("IsVisibleTo with ON: got false, want true")
	}
}

func TestRepository_IsVisibleTo_ChatModeOff_Never(t *testing.T) {
	r := NewRepository()
	r.InitializeWorld(1, 10)
	r.Register(1, 0xAAAA, 2, 0) // privateChat=OFF
	r.AddFriend(0xAAAA, 0xBBBB) // even friends don't see
	if r.IsVisibleTo(0xBBBB, 0xAAAA) {
		t.Errorf("IsVisibleTo with OFF: got true, want false")
	}
}

func TestRepository_IsVisibleTo_ChatModeFriends_OnlyFriends(t *testing.T) {
	r := NewRepository()
	r.InitializeWorld(1, 10)
	r.Register(1, 0xAAAA, 1, 0)  // privateChat=FRIENDS
	r.AddFriend(0xAAAA, 0xBBBB) // A friends B
	if !r.IsVisibleTo(0xBBBB, 0xAAAA) {
		t.Errorf("IsVisibleTo for friend with FRIENDS: got false, want true")
	}
	if r.IsVisibleTo(0xCCCC, 0xAAAA) {
		t.Errorf("IsVisibleTo for non-friend with FRIENDS: got true, want false")
	}
}

func TestRepository_IsVisibleTo_UnknownPlayer_NotVisible(t *testing.T) {
	r := NewRepository()
	if r.IsVisibleTo(0xBBBB, 0xDEADBEEF) {
		t.Errorf("IsVisibleTo on unknown other: got true, want false")
	}
}
```

- [ ] **Step 4.6: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run TestRepository -v`
Expected: New tests FAIL with "undefined: GetFollowers / IsVisibleTo".

- [ ] **Step 4.7: Implement `GetFollowers` + `IsVisibleTo`**

Append to `modules/friends/repository.go`:

```go
// GetFollowers returns every username that has target in their friend set.
// O(n) scan over the friends map; acceptable for slice 1 since slice 4 is
// the only consumer (broadcastWorldToFollowers fan-out) and it doesn't
// ship yet.
//
// NAI-S1-D-NO-FOLLOWER-BROADCAST — handlers don't call this in slice 1.
// Retired by slice 4.
func (r *Repository) GetFollowers(target uint64) []uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []uint64
	for follower, set := range r.friends {
		if _, ok := set[target]; ok {
			out = append(out, follower)
		}
	}
	return out
}

// IsVisibleTo applies TS visibility rules
// (FriendServerRepository.ts isVisibleTo):
//
//	other.privateChat 0 (ON)      -> always visible
//	other.privateChat 1 (FRIENDS) -> visible only if viewer is in other's friend set
//	other.privateChat 2 (OFF)     -> never visible
//
// If other is not registered, returns false.
func (r *Repository) IsVisibleTo(viewer, other uint64) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ps, ok := r.players[other]
	if !ok {
		return false
	}
	switch ps.privateChat {
	case 0: // ON
		return true
	case 1: // FRIENDS
		if set, ok := r.friends[other]; ok {
			_, isFriend := set[viewer]
			return isFriend
		}
		return false
	default: // OFF or unknown
		return false
	}
}
```

- [ ] **Step 4.8: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run TestRepository -v`
Expected: All tests PASS.

- [ ] **Step 4.9: Add `-race`-clean concurrent test**

Append to `modules/friends/repository_test.go`:

```go
func TestRepository_Concurrent_RaceClean(t *testing.T) {
	r := NewRepository()
	r.InitializeWorld(1, 10000)

	const goroutines = 16
	const iterations = 200

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := range goroutines {
		go func(id int) {
			defer wg.Done()
			base := uint64(id) * 1000
			for i := range iterations {
				u := base + uint64(i)
				r.Register(1, u, int32(i%3), 0)
				r.AddFriend(u, u+1)
				r.SetChatMode(u, int32((i+1)%3))
				_ = r.GetFriends(u)
				_ = r.IsVisibleTo(u+1, u)
				r.DeleteFriend(u, u+1)
				r.Unregister(u)
			}
		}(g)
	}
	wg.Wait()
}
```

- [ ] **Step 4.10: Run the concurrent test with `-race`**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/ -run TestRepository_Concurrent_RaceClean -v`
Expected: PASS, no DATA RACE warnings.

- [ ] **Step 4.11: Run the full repository suite under `-race`**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/ -v`
Expected: All `TestRepository_*` tests PASS, no DATA RACE.

- [ ] **Step 4.12: Commit**

```bash
git add modules/friends/repository.go modules/friends/repository_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: complete in-memory repository (social lists + visibility)

Adds SetChatMode/GetChatMode, AddFriend/DeleteFriend/GetFriends,
AddIgnore/DeleteIgnore/GetIgnores, GetFollowers (O(n) scan, slice 4
consumer), and IsVisibleTo (TS-faithful three-mode visibility rule).
-race-clean under N=16 goroutine concurrent test.

NAI-S1-D-INMEMORY-REPO and NAI-S1-D-NO-FOLLOWER-BROADCAST tags documented
inline. Repository surface is now complete for slice 1.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Add handler skeleton + `WorldConnect` RPC

**Why this task exists:** Stands up the `handler` struct (embeds `friendspb.UnimplementedFriendsServiceServer` so unimplemented methods auto-error during incremental impl). Lands `WorldConnect` first because its profile-validation behavior is unique among the RPCs — most other handlers will use a shared lazy-init helper that the WorldConnect path establishes.

**Files:**
- Create: `modules/friends/handler.go`
- Create: `modules/friends/handler_test.go`

- [ ] **Step 5.1: Write failing test for `WorldConnect` happy path**

Create `modules/friends/handler_test.go`:

```go
package friends

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/zsrv/goscape/pkg/friendspb"
)

// newTestHandler returns a handler wired to a fresh in-memory repo,
// configured with NodeProfile="main" and WorldPlayerLimit=10.
func newTestHandler(t *testing.T) *handler {
	t.Helper()
	return &handler{
		repo: NewRepository(),
		cfg: Config{
			NodeProfile:      "main",
			WorldPlayerLimit: 10,
		},
		log: noopLogger(),
	}
}

func TestHandler_WorldConnect_OK(t *testing.T) {
	h := newTestHandler(t)
	if _, err := h.WorldConnect(t.Context(), &friendspb.WorldConnectRequest{
		WorldId: 1,
		Profile: "main",
	}); err != nil {
		t.Fatalf("WorldConnect: %v", err)
	}
	// Indirect verification: a Register on world 1 must now succeed up
	// to the configured limit.
	if !h.repo.Register(1, 0xAAAA, 0, 0) {
		t.Errorf("Register after WorldConnect: got false, want true")
	}
}

func TestHandler_WorldConnect_ProfileMismatch(t *testing.T) {
	h := newTestHandler(t)
	_, err := h.WorldConnect(t.Context(), &friendspb.WorldConnectRequest{
		WorldId: 1,
		Profile: "wrong",
	})
	if err == nil {
		t.Fatalf("WorldConnect with bad profile: got nil error")
	}
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Errorf("status code: got %v, want InvalidArgument", got)
	}
}
```

- [ ] **Step 5.2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run TestHandler_WorldConnect -v`
Expected: FAIL with "undefined: handler" or build error.

- [ ] **Step 5.3: Write handler skeleton + `WorldConnect`**

Create `modules/friends/handler.go`:

```go
package friends

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/zsrv/goscape/pkg/friendspb"
)

// handler implements friendspb.FriendsServiceServer.
type handler struct {
	friendspb.UnimplementedFriendsServiceServer

	repo *Repository
	cfg  Config
	log  *slog.Logger
}

// ensureWorld lazy-inits the world's player slot if not already known.
// TS-faithful behavior: FriendServer.ts:108-115 (and similar branches)
// lazily call initializeWorld on the first non-WorldConnect message
// from a new world. Kept permanently.
//
// NAI-S1-D-LAZY-WORLDINIT — for reviewer traceability; not retired.
func (h *handler) ensureWorld(worldId int32) {
	h.repo.initializeWorldIfAbsent(worldId, h.cfg.WorldPlayerLimit)
}

// WorldConnect validates the profile and initializes the world's slot.
// Mirrors TS FriendServer WORLD_CONNECT (FriendServer.ts:89-106).
// Re-init by the same world resets that world's player counter to 0.
func (h *handler) WorldConnect(_ context.Context, req *friendspb.WorldConnectRequest) (*emptypb.Empty, error) {
	if req.Profile != h.cfg.NodeProfile {
		return nil, status.Errorf(codes.InvalidArgument,
			"profile mismatch: got %q, want %q", req.Profile, h.cfg.NodeProfile)
	}
	h.repo.InitializeWorld(req.WorldId, h.cfg.WorldPlayerLimit)
	return &emptypb.Empty{}, nil
}
```

Add the lazy-init helper to `modules/friends/repository.go` (append):

```go
// initializeWorldIfAbsent is the lazy-init variant of InitializeWorld:
// it only creates the world slot if it does not already exist, leaving
// existing playerCount untouched. Used by ensureWorld in the handler
// for the TS-faithful lazy-init paths (non-WorldConnect first message
// from an unknown world).
func (r *Repository) initializeWorldIfAbsent(worldId int32, limit int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.worlds[worldId]; ok {
		return
	}
	r.worlds[worldId] = &worldState{playerCount: 0, limit: limit}
}
```

- [ ] **Step 5.4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run TestHandler_WorldConnect -v`
Expected: Both tests PASS.

- [ ] **Step 5.5: Verify the full package still builds + races clean**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/ -v`
Expected: All `TestRepository_*` + `TestHandler_WorldConnect_*` tests PASS, no DATA RACE.

- [ ] **Step 5.6: Commit**

```bash
git add modules/friends/handler.go modules/friends/handler_test.go modules/friends/repository.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: add handler skeleton + WorldConnect RPC

Stands up handler struct embedding UnimplementedFriendsServiceServer,
plus the WorldConnect RPC with profile validation (InvalidArgument on
mismatch). Adds initializeWorldIfAbsent helper to repo for the
TS-faithful lazy-init paths the rest of the handlers will use.

NAI-S1-D-LAZY-WORLDINIT tag documented inline.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Implement player-presence RPCs (`PlayerLogin`, `PlayerLogout`, `ChatSetMode`)

**Why this task exists:** Lands the three RPCs that mutate player state. `PlayerLogin` carries the most behavior (dedupe across worlds, chat-mode coercion, player-cap rejection logging). `PlayerLogout` and `ChatSetMode` are thin wrappers.

**Files:**
- Modify: `modules/friends/handler.go`
- Modify: `modules/friends/handler_test.go`

- [ ] **Step 6.1: Write failing tests for `PlayerLogin`**

Append to `modules/friends/handler_test.go`:

```go
func TestHandler_PlayerLogin_BeforeWorldConnect_LazyInit(t *testing.T) {
	h := newTestHandler(t)
	resp, err := h.PlayerLogin(t.Context(), &friendspb.PlayerLoginRequest{
		WorldId:     1,
		Username37:  0xAAAA,
		PrivateChat: 0,
		StaffLvl:    0,
	})
	if err != nil {
		t.Fatalf("PlayerLogin without prior WorldConnect: %v", err)
	}
	if !resp.Accepted {
		t.Errorf("Accepted: got false, want true")
	}
	if got := h.repo.GetWorld(0xAAAA); got != 1 {
		t.Errorf("GetWorld after lazy-init login: got %d, want 1", got)
	}
}

func TestHandler_PlayerLogin_PrivateChatCoercion(t *testing.T) {
	h := newTestHandler(t)
	if _, err := h.WorldConnect(t.Context(), &friendspb.WorldConnectRequest{WorldId: 1, Profile: "main"}); err != nil {
		t.Fatalf("WorldConnect: %v", err)
	}
	if _, err := h.PlayerLogin(t.Context(), &friendspb.PlayerLoginRequest{
		WorldId:     1,
		Username37:  0xAAAA,
		PrivateChat: 99, // invalid -> coerce to 0 (ON)
		StaffLvl:    0,
	}); err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if got := h.repo.GetChatMode(0xAAAA); got != 0 {
		t.Errorf("GetChatMode after invalid coercion: got %d, want 0", got)
	}
}

func TestHandler_PlayerLogin_PlayerCapAccepted_False(t *testing.T) {
	h := newTestHandler(t)
	h.cfg.WorldPlayerLimit = 2
	if _, err := h.WorldConnect(t.Context(), &friendspb.WorldConnectRequest{WorldId: 1, Profile: "main"}); err != nil {
		t.Fatalf("WorldConnect: %v", err)
	}
	for i := uint64(1); i <= 2; i++ {
		resp, err := h.PlayerLogin(t.Context(), &friendspb.PlayerLoginRequest{WorldId: 1, Username37: i})
		if err != nil {
			t.Fatalf("PlayerLogin %d: %v", i, err)
		}
		if !resp.Accepted {
			t.Errorf("Accepted #%d: got false, want true", i)
		}
	}
	// 3rd login: cap reached.
	resp, err := h.PlayerLogin(t.Context(), &friendspb.PlayerLoginRequest{WorldId: 1, Username37: 3})
	if err != nil {
		t.Fatalf("PlayerLogin beyond cap: %v", err)
	}
	if resp.Accepted {
		t.Errorf("Accepted past cap: got true, want false")
	}
}
```

- [ ] **Step 6.2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run TestHandler_PlayerLogin -v`
Expected: FAIL with "method PlayerLogin not implemented" (because of UnimplementedFriendsServiceServer embed) or build error.

- [ ] **Step 6.3: Implement `PlayerLogin`**

Append to `modules/friends/handler.go`:

```go
// coercePrivateChat clamps a TS ChatModePrivate value to the valid range.
// Invalid values become 0 (ON). Mirrors TS FriendServer.ts:120-123.
func coercePrivateChat(v int32) int32 {
	if v < 0 || v > 2 {
		return 0
	}
	return v
}

// PlayerLogin registers the player on the given world. Always returns OK;
// PlayerLoginResponse.Accepted is false iff the world's player cap is
// reached.
//
// NAI-S1-D-PLAYERCAP-LOG-ONLY — cap rejection logs warn but does not error.
// Slice 4 surfaces Accepted to callers; slice 1 callers ignore the field.
func (h *handler) PlayerLogin(_ context.Context, req *friendspb.PlayerLoginRequest) (*friendspb.PlayerLoginResponse, error) {
	h.ensureWorld(req.WorldId)
	pc := coercePrivateChat(req.PrivateChat)
	// TS-faithful: PLAYER_LOGIN unregisters first to dedupe across worlds.
	h.repo.Unregister(req.Username37)
	accepted := h.repo.Register(req.WorldId, req.Username37, pc, req.StaffLvl)
	if !accepted {
		h.log.Warn("friends-server player cap reached",
			slog.Int("world_id", int(req.WorldId)),
			slog.Uint64("username37", req.Username37),
		)
	}
	return &friendspb.PlayerLoginResponse{Accepted: accepted}, nil
}
```

- [ ] **Step 6.4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run TestHandler_PlayerLogin -v`
Expected: All 3 tests PASS.

- [ ] **Step 6.5: Write failing tests for `PlayerLogout` + `ChatSetMode`**

Append to `modules/friends/handler_test.go`:

```go
func TestHandler_PlayerLogout_Idempotent(t *testing.T) {
	h := newTestHandler(t)
	// No prior WorldConnect, no prior Login — Logout must succeed.
	if _, err := h.PlayerLogout(t.Context(), &friendspb.PlayerLogoutRequest{
		WorldId:    1,
		Username37: 0xDEADBEEF,
	}); err != nil {
		t.Fatalf("PlayerLogout on unknown player: %v", err)
	}
}

func TestHandler_PlayerLogout_RemovesPlayer(t *testing.T) {
	h := newTestHandler(t)
	if _, err := h.WorldConnect(t.Context(), &friendspb.WorldConnectRequest{WorldId: 1, Profile: "main"}); err != nil {
		t.Fatalf("WorldConnect: %v", err)
	}
	if _, err := h.PlayerLogin(t.Context(), &friendspb.PlayerLoginRequest{WorldId: 1, Username37: 0xAAAA}); err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if _, err := h.PlayerLogout(t.Context(), &friendspb.PlayerLogoutRequest{WorldId: 1, Username37: 0xAAAA}); err != nil {
		t.Fatalf("PlayerLogout: %v", err)
	}
	if got := h.repo.GetWorld(0xAAAA); got != 0 {
		t.Errorf("GetWorld after logout: got %d, want 0", got)
	}
}

func TestHandler_ChatSetMode_UpdatesState(t *testing.T) {
	h := newTestHandler(t)
	if _, err := h.WorldConnect(t.Context(), &friendspb.WorldConnectRequest{WorldId: 1, Profile: "main"}); err != nil {
		t.Fatalf("WorldConnect: %v", err)
	}
	if _, err := h.PlayerLogin(t.Context(), &friendspb.PlayerLoginRequest{WorldId: 1, Username37: 0xAAAA, PrivateChat: 0}); err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if _, err := h.ChatSetMode(t.Context(), &friendspb.ChatSetModeRequest{
		WorldId:     1,
		Username37:  0xAAAA,
		PrivateChat: 2,
	}); err != nil {
		t.Fatalf("ChatSetMode: %v", err)
	}
	if got := h.repo.GetChatMode(0xAAAA); got != 2 {
		t.Errorf("GetChatMode after ChatSetMode(OFF): got %d, want 2", got)
	}
}

func TestHandler_ChatSetMode_PrivateChatCoercion(t *testing.T) {
	h := newTestHandler(t)
	if _, err := h.WorldConnect(t.Context(), &friendspb.WorldConnectRequest{WorldId: 1, Profile: "main"}); err != nil {
		t.Fatalf("WorldConnect: %v", err)
	}
	if _, err := h.PlayerLogin(t.Context(), &friendspb.PlayerLoginRequest{WorldId: 1, Username37: 0xAAAA, PrivateChat: 0}); err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}
	if _, err := h.ChatSetMode(t.Context(), &friendspb.ChatSetModeRequest{
		WorldId:     1,
		Username37:  0xAAAA,
		PrivateChat: 99, // invalid -> 0
	}); err != nil {
		t.Fatalf("ChatSetMode: %v", err)
	}
	if got := h.repo.GetChatMode(0xAAAA); got != 0 {
		t.Errorf("GetChatMode after coercion: got %d, want 0", got)
	}
}
```

- [ ] **Step 6.6: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run "TestHandler_PlayerLogout|TestHandler_ChatSetMode" -v`
Expected: FAIL with "method PlayerLogout not implemented" / "ChatSetMode not implemented".

- [ ] **Step 6.7: Implement `PlayerLogout` + `ChatSetMode`**

Append to `modules/friends/handler.go`:

```go
// PlayerLogout removes the player from whichever world they're on.
// Idempotent on unknown players.
func (h *handler) PlayerLogout(_ context.Context, req *friendspb.PlayerLogoutRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	h.repo.Unregister(req.Username37)
	return &emptypb.Empty{}, nil
}

// ChatSetMode updates the player's privateChat setting. Invalid values
// are coerced to 0 (ON), matching TS FriendServer.ts:176-179. No-op on
// unknown player (state lives at the player record, which doesn't exist
// pre-login).
//
// NAI-S1-D-NO-FOLLOWER-BROADCAST — slice 4 will broadcast the new mode
// to followers; slice 1 just mutates state.
func (h *handler) ChatSetMode(_ context.Context, req *friendspb.ChatSetModeRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	h.repo.SetChatMode(req.Username37, coercePrivateChat(req.PrivateChat))
	return &emptypb.Empty{}, nil
}
```

- [ ] **Step 6.8: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run "TestHandler_PlayerLogout|TestHandler_ChatSetMode" -v`
Expected: All 4 tests PASS.

- [ ] **Step 6.9: Commit**

```bash
git add modules/friends/handler.go modules/friends/handler_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: implement player-presence RPCs

PlayerLogin (dedupe across worlds, chat-mode coercion, cap-log-only),
PlayerLogout (idempotent), ChatSetMode (with coercion). Shared
coercePrivateChat helper mirrors TS FriendServer.ts:120-123 clamping.

NAI-S1-D-PLAYERCAP-LOG-ONLY tag documented at PlayerLogin;
NAI-S1-D-NO-FOLLOWER-BROADCAST tag documented at ChatSetMode.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Implement social-list RPCs (`FriendlistAdd/Del`, `IgnorelistAdd/Del`)

**Why this task exists:** Four near-identical handlers wrapping repo `AddFriend`/`DeleteFriend`/`AddIgnore`/`DeleteIgnore`. Co-located in one task because the surface is tiny and the tests follow the same shape.

**Files:**
- Modify: `modules/friends/handler.go`
- Modify: `modules/friends/handler_test.go`

- [ ] **Step 7.1: Write failing tests for the four social-list handlers**

Append to `modules/friends/handler_test.go`:

```go
func TestHandler_FriendlistAdd_Persists(t *testing.T) {
	h := newTestHandler(t)
	if _, err := h.FriendlistAdd(t.Context(), &friendspb.FriendlistAddRequest{
		WorldId:          1,
		Username37:       0xAAAA,
		TargetUsername37: 0xBBBB,
	}); err != nil {
		t.Fatalf("FriendlistAdd: %v", err)
	}
	got := h.repo.GetFriends(0xAAAA)
	if len(got) != 1 || got[0] != 0xBBBB {
		t.Errorf("GetFriends after FriendlistAdd: got %v, want [0xBBBB]", got)
	}
}

func TestHandler_FriendlistDel_RemovesEntry(t *testing.T) {
	h := newTestHandler(t)
	h.repo.AddFriend(0xAAAA, 0xBBBB)
	if _, err := h.FriendlistDel(t.Context(), &friendspb.FriendlistDelRequest{
		WorldId:          1,
		Username37:       0xAAAA,
		TargetUsername37: 0xBBBB,
	}); err != nil {
		t.Fatalf("FriendlistDel: %v", err)
	}
	if got := h.repo.GetFriends(0xAAAA); len(got) != 0 {
		t.Errorf("GetFriends after FriendlistDel: got %v, want empty", got)
	}
}

func TestHandler_IgnorelistAdd_Persists(t *testing.T) {
	h := newTestHandler(t)
	if _, err := h.IgnorelistAdd(t.Context(), &friendspb.IgnorelistAddRequest{
		WorldId:          1,
		Username37:       0xAAAA,
		TargetUsername37: 0xBBBB,
	}); err != nil {
		t.Fatalf("IgnorelistAdd: %v", err)
	}
	got := h.repo.GetIgnores(0xAAAA)
	if len(got) != 1 || got[0] != 0xBBBB {
		t.Errorf("GetIgnores after IgnorelistAdd: got %v, want [0xBBBB]", got)
	}
}

func TestHandler_IgnorelistDel_RemovesEntry(t *testing.T) {
	h := newTestHandler(t)
	h.repo.AddIgnore(0xAAAA, 0xBBBB)
	if _, err := h.IgnorelistDel(t.Context(), &friendspb.IgnorelistDelRequest{
		WorldId:          1,
		Username37:       0xAAAA,
		TargetUsername37: 0xBBBB,
	}); err != nil {
		t.Fatalf("IgnorelistDel: %v", err)
	}
	if got := h.repo.GetIgnores(0xAAAA); len(got) != 0 {
		t.Errorf("GetIgnores after IgnorelistDel: got %v, want empty", got)
	}
}
```

- [ ] **Step 7.2: Run tests to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run "TestHandler_Friendlist|TestHandler_Ignorelist" -v`
Expected: FAIL with "method FriendlistAdd not implemented" etc.

- [ ] **Step 7.3: Implement the four social-list handlers**

Append to `modules/friends/handler.go`:

```go
// FriendlistAdd appends target to the player's friend set (idempotent).
// NAI-S1-D-NO-FOLLOWER-BROADCAST — slice 4 will broadcast.
func (h *handler) FriendlistAdd(_ context.Context, req *friendspb.FriendlistAddRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	h.repo.AddFriend(req.Username37, req.TargetUsername37)
	return &emptypb.Empty{}, nil
}

// FriendlistDel removes target from the player's friend set (idempotent).
// NAI-S1-D-NO-FOLLOWER-BROADCAST — slice 4 will broadcast.
func (h *handler) FriendlistDel(_ context.Context, req *friendspb.FriendlistDelRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	h.repo.DeleteFriend(req.Username37, req.TargetUsername37)
	return &emptypb.Empty{}, nil
}

// IgnorelistAdd appends target to the player's ignore set (idempotent).
// NAI-S1-D-NO-FOLLOWER-BROADCAST — slice 4 will broadcast.
func (h *handler) IgnorelistAdd(_ context.Context, req *friendspb.IgnorelistAddRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	h.repo.AddIgnore(req.Username37, req.TargetUsername37)
	return &emptypb.Empty{}, nil
}

// IgnorelistDel removes target from the player's ignore set (idempotent).
// NAI-S1-D-NO-FOLLOWER-BROADCAST — slice 4 will broadcast.
func (h *handler) IgnorelistDel(_ context.Context, req *friendspb.IgnorelistDelRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	h.repo.DeleteIgnore(req.Username37, req.TargetUsername37)
	return &emptypb.Empty{}, nil
}
```

- [ ] **Step 7.4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run "TestHandler_Friendlist|TestHandler_Ignorelist" -v`
Expected: All 4 tests PASS.

- [ ] **Step 7.5: Commit**

```bash
git add modules/friends/handler.go modules/friends/handler_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: implement social-list RPCs

FriendlistAdd/Del + IgnorelistAdd/Del — thin wrappers over the repo
mutations. NAI-S1-D-NO-FOLLOWER-BROADCAST tag documented at each
handler; slice 4 fills in the follower broadcast.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Implement `PrivateMessage` (slice-1 no-op) + verify `SubscribeUpdates` stub

**Why this task exists:** Closes out the handler surface. `PrivateMessage` accepts the call and logs but does not deliver (slice 4) or persist (slice 6). `SubscribeUpdates` is auto-Unimplemented via the embedded `UnimplementedFriendsServiceServer` — task adds a test that pins this expectation.

**Files:**
- Modify: `modules/friends/handler.go`
- Modify: `modules/friends/handler_test.go`

- [ ] **Step 8.1: Write failing test for `PrivateMessage`**

Append to `modules/friends/handler_test.go`:

```go
func TestHandler_PrivateMessage_NoOp_Slice1(t *testing.T) {
	h := newTestHandler(t)
	// Acceptance is the assertion: returns OK without erroring. Delivery
	// is slice 4, persistence is slice 6.
	if _, err := h.PrivateMessage(t.Context(), &friendspb.PrivateMessageRequest{
		WorldId:          1,
		Username37:       0xAAAA,
		TargetUsername37: 0xBBBB,
		StaffLvl:         0,
		PmId:             1,
		Chat:             "hi",
		Coord:            12345,
	}); err != nil {
		t.Fatalf("PrivateMessage: %v", err)
	}
}
```

- [ ] **Step 8.2: Run test to verify it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run TestHandler_PrivateMessage -v`
Expected: FAIL with "method PrivateMessage not implemented".

- [ ] **Step 8.3: Implement `PrivateMessage`**

Append to `modules/friends/handler.go`:

```go
// PrivateMessage accepts and logs the call. Delivery is deferred to
// slice 4 (server -> world push via SubscribeUpdates). Persistence is
// deferred to slice 6 (private_chat DB table).
//
// NAI-S1-D-PM-NO-DELIVERY — slice 4 retires.
// NAI-S1-D-PM-NO-PERSISTENCE — slice 6 retires.
func (h *handler) PrivateMessage(_ context.Context, req *friendspb.PrivateMessageRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	h.log.Debug("friends-server received private message",
		slog.Int("world_id", int(req.WorldId)),
		slog.Uint64("from", req.Username37),
		slog.Uint64("to", req.TargetUsername37),
		slog.Uint64("pm_id", uint64(req.PmId)),
	)
	return &emptypb.Empty{}, nil
}
```

- [ ] **Step 8.4: Run test to verify it passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run TestHandler_PrivateMessage -v`
Expected: PASS.

- [ ] **Step 8.5: Write `SubscribeUpdates` Unimplemented pin-test**

Append to `modules/friends/handler_test.go`:

```go
// subscribeUpdatesRecorder is a minimal in-package implementation of
// friendspb.FriendsService_SubscribeUpdatesServer for testing the
// Unimplemented stub. We never actually send on this stream — the
// handler must error out before reaching any Send call.
type subscribeUpdatesRecorder struct {
	friendspb.FriendsService_SubscribeUpdatesServer
}

func TestHandler_SubscribeUpdates_Unimplemented(t *testing.T) {
	h := newTestHandler(t)
	err := h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{
		WorldId:    1,
		Username37: 0xAAAA,
	}, &subscribeUpdatesRecorder{})
	if err == nil {
		t.Fatalf("SubscribeUpdates: got nil error, want Unimplemented")
	}
	if got := status.Code(err); got != codes.Unimplemented {
		t.Errorf("status code: got %v, want Unimplemented", got)
	}
}
```

- [ ] **Step 8.6: Run test to verify it passes**

The `UnimplementedFriendsServiceServer` embed automatically returns `Unimplemented` for any unoverridden method, including `SubscribeUpdates`. So this test should pass without any handler implementation.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run TestHandler_SubscribeUpdates -v`
Expected: PASS.

- [ ] **Step 8.7: Run the whole package under `-race`**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/ -v`
Expected: All tests PASS (~25 tests across `TestRepository_*` and `TestHandler_*`), no DATA RACE.

- [ ] **Step 8.8: Commit**

```bash
git add modules/friends/handler.go modules/friends/handler_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: implement PrivateMessage no-op + pin SubscribeUpdates Unimplemented

PrivateMessage accepts and debug-logs; delivery (slice 4) and
persistence (slice 6) deferred per spec. SubscribeUpdates pin-test
asserts the embedded UnimplementedFriendsServiceServer returns
codes.Unimplemented as expected for slice 1; slice 4 will replace the
stub with the real broadcastWorldToFollowers fan-out.

NAI-S1-D-PM-NO-DELIVERY and NAI-S1-D-PM-NO-PERSISTENCE tags documented
inline. Handler surface is now complete for slice 1.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Add `server.go` (gRPC wrapper) + `friends.go` (module lifecycle)

**Why this task exists:** Wraps the handler in a dskit-compatible `services.Service` so the module can start, run, and stop alongside `modules/login` and `modules/world`. Mirrors `modules/login/server.go` + `modules/login/login.go` shape.

**Files:**
- Create: `modules/friends/server.go`
- Create: `modules/friends/friends.go`

- [ ] **Step 9.1: Write `server.go`**

Create `modules/friends/server.go`:

```go
package friends

import (
	"fmt"
	"log/slog"
	"net"

	"google.golang.org/grpc"

	"github.com/zsrv/goscape/pkg/friendspb"
)

// grpcServer wraps a *grpc.Server registered with the friends handler.
// Sibling to modules/login/server.go.
type grpcServer struct {
	server *grpc.Server
	log    *slog.Logger
}

func newGRPCServer(cfg Config, repo *Repository, log *slog.Logger) *grpcServer {
	s := grpc.NewServer()
	friendspb.RegisterFriendsServiceServer(s, &handler{
		repo: repo,
		cfg:  cfg,
		log:  log,
	})
	return &grpcServer{server: s, log: log}
}

// listen binds the TCP port and returns the listener. Called during
// Starting phase so the service is not Running until the port is bound.
func (s *grpcServer) listen(cfg Config) (net.Listener, error) {
	addr := fmt.Sprintf("%s:%d", cfg.GRPCListenAddress, cfg.GRPCListenPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("grpc listen %s: %w", addr, err)
	}
	s.log.Info("friends gRPC server listening", slog.String("addr", addr))
	return lis, nil
}

// serve starts accepting connections on lis. Blocks until the server stops.
func (s *grpcServer) serve(lis net.Listener) error {
	return s.server.Serve(lis)
}

// shutdown triggers a graceful stop of the gRPC server.
func (s *grpcServer) shutdown() {
	s.server.GracefulStop()
}
```

- [ ] **Step 9.2: Write `friends.go`**

Create `modules/friends/friends.go`:

```go
package friends

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/zsrv/goscape/internal/dskit/services"
)

// Friends is the friends-server module. It owns the gRPC server and the
// in-memory repository.
type Friends struct {
	services.Service

	cfg Config
	log *slog.Logger

	repo *Repository
	srv  *grpcServer
	lis  net.Listener
}

// New validates the config and constructs the Friends module.
func New(cfg Config, logger *slog.Logger) (*Friends, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	f := &Friends{cfg: cfg, log: logger}
	f.Service = services.NewBasicService(f.starting, f.running, f.stopping)
	return f, nil
}

// NewFriendsService is the factory used by the dskit module manager.
func NewFriendsService(cfg Config, logger *slog.Logger) (services.Service, error) {
	return New(cfg, logger)
}

func (f *Friends) starting(_ context.Context) error {
	repo := NewRepository()
	srv := newGRPCServer(f.cfg, repo, f.log)
	lis, err := srv.listen(f.cfg)
	if err != nil {
		return err
	}
	f.repo = repo
	f.srv = srv
	f.lis = lis
	return nil
}

func (f *Friends) running(ctx context.Context) error {
	serverDone := make(chan error, 1)
	lis := f.lis
	f.lis = nil // gRPC now owns the listener
	go func() { serverDone <- f.srv.serve(lis) }()

	select {
	case <-ctx.Done():
		f.srv.shutdown()
		<-serverDone
		return nil
	case err := <-serverDone:
		if err != nil {
			return fmt.Errorf("grpc server: %w", err)
		}
		return nil
	}
}

func (f *Friends) stopping(_ error) error {
	// Covers the edge case where StopAsync is called between starting()
	// returning and running() being invoked — gRPC never took ownership
	// of the listener.
	if f.lis != nil {
		f.lis.Close()
	}
	return nil
}
```

- [ ] **Step 9.3: Verify the module builds**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/friends/...`
Expected: Exit 0, no output.

- [ ] **Step 9.4: Verify the full repo still builds**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: Exit 0, no output.

- [ ] **Step 9.5: Verify all tests still pass under `-race`**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/...`
Expected: All `TestRepository_*` + `TestHandler_*` tests PASS, no DATA RACE.

- [ ] **Step 9.6: Commit**

```bash
git add modules/friends/server.go modules/friends/friends.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: add module lifecycle (server.go + friends.go)

grpcServer wrapper mirrors modules/login/server.go; Friends module
struct + New + NewFriendsService factory + starting/running/stopping
lifecycle mirrors modules/login/login.go (minus the DB layer).

After this commit modules/friends/ is a complete dskit-compatible
module ready to be wired into the app's module manager.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Wire `modules/friends/` into the app

**Why this task exists:** Final step — surface the module to `cmd/goscape/app/` so it gets a config block, an init function, and a slot in the `SingleBinary` target. With `friends.enable=false` (default), no behavior change for existing deploys.

**Files:**
- Modify: `cmd/goscape/app/config.go`
- Modify: `cmd/goscape/app/modules.go`

- [ ] **Step 10.1: Add `friends.Config` to the app `Config` struct**

In `cmd/goscape/app/config.go`, add the import (alphabetical between `asset` and `login`) and the struct field.

Modify import block from:

```go
import (
	"flag"
	"log/slog"

	"github.com/zsrv/goscape/modules/asset"
	"github.com/zsrv/goscape/modules/login"
	"github.com/zsrv/goscape/modules/world"
)
```

to:

```go
import (
	"flag"
	"log/slog"

	"github.com/zsrv/goscape/modules/asset"
	"github.com/zsrv/goscape/modules/friends"
	"github.com/zsrv/goscape/modules/login"
	"github.com/zsrv/goscape/modules/world"
)
```

Modify the `Config` struct from:

```go
type Config struct {
	Target    string     `yaml:"target,omitempty"`
	LogFormat string     `yaml:"log_format,omitempty"`
	LogLevel  slog.Level `yaml:"log_level,omitempty"` // global log level, default for modules too

	Asset asset.Config `yaml:"asset,omitempty"`
	Login login.Config `yaml:"login,omitempty"`
	World world.Config `yaml:"world,omitempty"`
}
```

to:

```go
type Config struct {
	Target    string     `yaml:"target,omitempty"`
	LogFormat string     `yaml:"log_format,omitempty"`
	LogLevel  slog.Level `yaml:"log_level,omitempty"` // global log level, default for modules too

	Asset   asset.Config   `yaml:"asset,omitempty"`
	Friends friends.Config `yaml:"friends,omitempty"`
	Login   login.Config   `yaml:"login,omitempty"`
	World   world.Config   `yaml:"world,omitempty"`
}
```

Modify `RegisterFlagsAndApplyDefaults` from:

```go
	c.Asset.RegisterFlagsAndApplyDefaults(f)
	c.Login.RegisterFlagsAndApplyDefaults(f)
	c.World.RegisterFlagsAndApplyDefaults(f)
```

to:

```go
	c.Asset.RegisterFlagsAndApplyDefaults(f)
	c.Friends.RegisterFlagsAndApplyDefaults(f)
	c.Login.RegisterFlagsAndApplyDefaults(f)
	c.World.RegisterFlagsAndApplyDefaults(f)
```

- [ ] **Step 10.2: Add `initFriends` and registration to `modules.go`**

In `cmd/goscape/app/modules.go`, add the import (alphabetical):

```go
	"github.com/zsrv/goscape/modules/friends"
```

Add `Friends` to the targets const block (alphabetical between `Asset` and `Login`):

```go
const (
	// Individual targets

	Asset   string = "asset"
	Friends string = "friends"
	Login   string = "login"
	World   string = "world"

	// Composite targets

	SingleBinary string = "all"
)
```

Add `initFriends` after `initLogin` (around line 93 in the current file):

```go
func (g *App) initFriends() (services.Service, error) {
	if !g.cfg.Friends.Enable {
		return services.NewIdleService(nil, nil), nil
	}
	logger, err := log.NewLogger(g.cfg.LogLevel, g.cfg.LogFormat, os.Stdout)
	if err != nil {
		g.logger.Error("failed to create logger", "module", "friends", "err", err)
		os.Exit(1)
	}
	return friends.NewFriendsService(g.cfg.Friends, logger)
}
```

Add the registration in `setupModuleManager`:

```go
	mm.RegisterModule(Asset, g.initAsset)
	mm.RegisterModule(Friends, g.initFriends) // NEW
	mm.RegisterModule(Login, g.initLogin)
	mm.RegisterModule(World, g.initWorld)
```

Edit the `deps` map to add `Friends` and include it in `SingleBinary`:

```go
	deps := map[string][]string{
		Common: {},

		// Individual targets

		Asset:   {Common, World},
		Friends: {Common},
		Login:   {Common},
		World:   {Common, Login},

		// composite targets

		SingleBinary: {Asset, Friends, Login, World},
	}
```

- [ ] **Step 10.3: Verify the app builds**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: Exit 0, no output.

- [ ] **Step 10.4: Verify the `friends.*` flags are exposed**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run ./cmd/goscape --help 2>&1 | grep -E 'friends\.' | head -20`
Expected: 6 lines mentioning `friends.enable`, `friends.grpc-listen-address`, `friends.grpc-listen-port`, `friends.node-profile`, `friends.world-player-limit`, `friends.graceful-shutdown-timeout`.

- [ ] **Step 10.5: Run the full test suite under `-race` (regression gate)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...`
Expected: All tests PASS across all packages. No new failures introduced by the wiring.

This is the slice 1 done-criterion test from spec §7.

- [ ] **Step 10.6: Run the smoke-pack baseline check**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape-cli smoke-pack \
  --content-dir $HOME/Code/github.com/LostCityRS/content
```
Expected: `12 OK / 0 ERR / 0 SKIP` (the baseline from `[[smoke_pack_worldmap_stage_wiring]]` / NAI-214's close). Slice 1 doesn't touch packing — this should be unchanged.

- [ ] **Step 10.7: Commit**

```bash
git add cmd/goscape/app/config.go cmd/goscape/app/modules.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
app: wire modules/friends into the app

Adds Friends to the targets const block, initFriends factory (mirrors
initLogin), module registration, and dependency edge into SingleBinary.
Gated by friends.enable (default false) — default deploys unchanged.

Closes slice 1 of the 7-slice friends-server bridge arc. The friends
gRPC server now starts when friends.enable=true; modules/world/ still
points at noopBridges{} (slice 2 wires the world side).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-review

After all 10 tasks complete:

**Spec coverage check** — every spec requirement maps to a task:

| Spec section | Implemented in |
|---|---|
| §1 In scope — proto file | Task 1 |
| §1 In scope — generated `pkg/friendspb/` | Task 1 |
| §1 In scope — `modules/friends/config.go` | Task 2 |
| §1 In scope — `modules/friends/friends.go` | Task 9 |
| §1 In scope — `modules/friends/server.go` | Task 9 |
| §1 In scope — `modules/friends/handler.go` | Tasks 5, 6, 7, 8 |
| §1 In scope — `modules/friends/repository.go` | Tasks 3, 4 |
| §1 In scope — `modules/friends/repository_test.go` | Tasks 3, 4 |
| §1 In scope — `modules/friends/handler_test.go` | Tasks 5, 6, 7, 8 |
| §1 In scope — `cmd/goscape/app/config.go` edits | Task 10 |
| §1 In scope — `cmd/goscape/app/modules.go` edits | Task 10 |
| §4 Proto contract — all 10 RPCs + messages | Task 1 |
| §5 Repository contract — all 13 methods | Tasks 3, 4 |
| §6 Data flow — WorldConnect | Task 5 |
| §6 Data flow — PlayerLogin (dedupe, coercion, cap-log) | Task 6 |
| §6 Data flow — PlayerLogout | Task 6 |
| §6 Data flow — ChatSetMode (coercion) | Task 6 |
| §6 Data flow — Friendlist/Ignorelist Add/Del | Task 7 |
| §6 Data flow — PrivateMessage no-op | Task 8 |
| §6 Data flow — SubscribeUpdates Unimplemented | Task 8 |
| §7 Testing — repo unit tests (9 named) | Tasks 3, 4 |
| §7 Testing — handler tests (13 named) | Tasks 5, 6, 7, 8 |
| §7 Done criterion — `go test -race ./...` clean | Task 10.5 |
| §7 Done criterion — `go build ./...` clean | Task 10.3 |
| §7 Done criterion — smoke-pack 12 OK / 0 ERR | Task 10.6 |
| §8 Deviation tags — `NAI-S1-D-INMEMORY-REPO` | Task 3 (repo package doc) |
| §8 Deviation tags — `NAI-S1-D-NO-FOLLOWER-BROADCAST` | Tasks 4 (GetFollowers), 6 (ChatSetMode), 7 (4 handlers) |
| §8 Deviation tags — `NAI-S1-D-PM-NO-DELIVERY` | Task 8 |
| §8 Deviation tags — `NAI-S1-D-PM-NO-PERSISTENCE` | Task 8 |
| §8 Deviation tags — `NAI-S1-D-PLAYERCAP-LOG-ONLY` | Task 6 |
| §8 Deviation tags — `NAI-S1-D-LAZY-WORLDINIT` | Task 5 |

All spec sections covered.

**Type-consistency check:**

- `Repository` method names match between Task 3/4 definitions and Task 5/6/7/8 handler calls: `InitializeWorld`, `Register`, `Unregister`, `GetWorld`, `GetChatMode`, `SetChatMode`, `AddFriend`, `DeleteFriend`, `AddIgnore`, `DeleteIgnore`, `GetFriends`, `GetIgnores`, `GetFollowers`, `IsVisibleTo`, `initializeWorldIfAbsent` ✓
- `handler` struct fields (`repo`, `cfg`, `log`) consistent across all tasks ✓
- proto field names match generated Go: `WorldId`, `Username37`, `TargetUsername37`, `PrivateChat`, `StaffLvl`, `PmId`, `Chat`, `Coord`, `Profile`, `Accepted` (proto snake_case → Go PascalCase) ✓
- `coercePrivateChat(int32) int32` defined once in Task 6; reused implicitly in PrivateChat coercion test in Task 6 + ChatSetMode in Task 6 ✓
- `noopLogger()` test helper defined once in Task 3, reused across all `handler_test.go` tests (via `newTestHandler(t)`) ✓
- `newTestHandler(t) *handler` defined once in Task 5, reused across Tasks 6, 7, 8 ✓

No type drift detected.

**Placeholder scan:**

Searched for "TBD", "TODO", "implement later", "FIXME" in the plan body. The only `TODO`-style markers are in *generated commit messages* describing what future slices will do (e.g. "slice 4 retires"). Those are intentional forward-references, not plan placeholders.

No fix-ups needed.
