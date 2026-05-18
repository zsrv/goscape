# Friends-server bridge — Slice 2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the world to the slice-1 friends-server via a new `FriendsClient` interface + `grpcFriendsBridge` adapter; retire the umbrella tag `NAI-72-D-FRIENDS-SERVER-BRIDGE`.

**Architecture:** Two-seam shape mirroring the login side (`[[loginclient-interface-close]]` + `[[nai214-login-moderation-bridge-close]]`). `FriendsClient` owns the gRPC seam; `grpcFriendsBridge` translates the existing `FriendsBridge` interface to RPC calls with goroutine fan-out. Direct-call sites (`WorldConnect` at startup, `PlayerLogin` from `processLogins`, `PlayerLogout` from both removePlayer paths) use `FriendsClient` directly. Default `world.friends-server-enabled=false` matches slice 1's `friends.enable=false`.

**Tech Stack:** Go 1.26+, `google.golang.org/grpc`, `pkg/friendspb` (slice 1), `pkg/util/jstring` for base37.

**Predecessor:** `[[friends-server-slice1-close]]` (HEAD = `ef3f11a9`).

**Spec:** `docs/superpowers/specs/2026-05-18-friends-server-bridge-slice2-design.md`.

---

## Conventions

All Go commands prefix `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`. All git commits use `--no-gpg-sign`. Each task ends with one commit.

Run from repo root: `/home/owner/Code/github.com/zsrv/goscape`.

---

### Task 1: Config — add friends-server flags

**Files:**
- Modify: `modules/world/config.go:21-22` (insert FriendsServerAddress alongside LoginServerAddress) and `:38-39` (insert FriendsServerEnabled alongside LoginServerEnabled)
- Modify: `modules/world/config.go:92-93` (insert flag registrations)

- [ ] **Step 1: Read the file**

Run: `cat modules/world/config.go`
Confirm the existing `LoginServerAddress` / `LoginServerEnabled` field + flag pattern. The struct uses field reordering by alignment; add new fields adjacent to their login siblings.

- [ ] **Step 2: Add the struct fields**

Edit `modules/world/config.go`. Locate the line `LoginServerAddress               string        \`yaml:"login_server_address"\`` (line 21) and insert immediately after:

```go
	FriendsServerAddress             string        `yaml:"friends_server_address"`
```

Locate the line `LoginServerEnabled               bool          \`yaml:"login_server_enabled"\`` (line 38) and insert immediately after:

```go
	FriendsServerEnabled             bool          `yaml:"friends_server_enabled"`
```

- [ ] **Step 3: Add the flags**

In `RegisterFlagsAndApplyDefaults`, locate:

```go
	f.StringVar(&c.LoginServerAddress, "world.login-server-address", "127.0.0.1:2004", "Login server gRPC address.")
	f.BoolVar(&c.LoginServerEnabled, "world.login-server-enabled", true, "Whether to connect to the login server.")
```

Append the two friends-server lines immediately after:

```go
	f.StringVar(&c.FriendsServerAddress, "world.friends-server-address", "127.0.0.1:2005", "Friends server gRPC address.")
	f.BoolVar(&c.FriendsServerEnabled, "world.friends-server-enabled", false, "Whether to connect to the friends server.")
```

- [ ] **Step 4: Verify build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: PASS (no signatures changed yet).

- [ ] **Step 5: Verify config_test still passes**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestConfig`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add modules/world/config.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: add friends-server config (default disabled)

Adds --world.friends-server-address (default 127.0.0.1:2005) and
--world.friends-server-enabled (default false), mirroring the
existing login-server-{address,enabled} pair. Wiring lands in the
remaining tasks of the slice-2 plan.
EOF
)"
```

---

### Task 2: FriendsClient interface + grpcFriendsClient impl

**Files:**
- Create: `modules/world/friends_client.go`

This task creates the gRPC seam mirroring `modules/world/login_client.go`. Pure addition — no signature changes elsewhere yet.

- [ ] **Step 1: Read the reference**

Run: `cat modules/world/login_client.go`
Note the pattern: interface with method-per-RPC, concrete `grpc*Client` struct, `New*Client(addr, log) (Interface, error)` factory using `grpc.NewClient` (not `grpc.Dial`), `Close()` releasing the conn, per-method log-warn-on-error for fire-and-forget RPCs.

- [ ] **Step 2: Create friends_client.go**

Create `modules/world/friends_client.go` with this exact content:

```go
package world

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/zsrv/goscape/pkg/friendspb"
)

// FriendsClient is the world-side interface to the friends service.
// Production impl: grpcFriendsClient (this file). Test impl:
// fakeFriendsClient (friends_client_fake_test.go).
//
// All RPCs except Close are fire-and-forget: errors are logged via
// the embedded *slog.Logger and swallowed. The friends-server is
// best-effort by design (slice 1's NAI-S1-D-PM-NO-DELIVERY etc.);
// the world does not depend on its responses through slice 3.
//
// SubscribeUpdates is intentionally absent — slice 4 adds it.
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

// grpcFriendsClient wraps the gRPC connection to the friends server.
type grpcFriendsClient struct {
	conn   *grpc.ClientConn
	client friendspb.FriendsServiceClient
	log    *slog.Logger
}

// NewFriendsClient creates a non-blocking gRPC client to the friends server.
// grpc.NewClient does not block — connection is established lazily with automatic retry.
func NewFriendsClient(addr string, log *slog.Logger) (FriendsClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("grpc dial friends server: %w", err)
	}
	return &grpcFriendsClient{
		conn:   conn,
		client: friendspb.NewFriendsServiceClient(conn),
		log:    log,
	}, nil
}

// Close releases the gRPC connection.
func (c *grpcFriendsClient) Close() error {
	return c.conn.Close()
}

// WorldConnect notifies the friends server that this world is connecting.
// Validates the profile and initializes the world's player-count slot.
func (c *grpcFriendsClient) WorldConnect(ctx context.Context, worldID int32, profile string) {
	if _, err := c.client.WorldConnect(ctx, &friendspb.WorldConnectRequest{
		WorldId: worldID,
		Profile: profile,
	}); err != nil {
		c.log.Warn("WorldConnect RPC failed",
			slog.Int("world_id", int(worldID)),
			slog.String("profile", profile),
			slog.Any("err", err),
		)
	}
}

// PlayerLogin registers the player on the friends server.
// PlayerLoginResponse.Accepted is ignored slice-2 (NAI-S2-D-PLAYERLOGIN-IGNORES-ACCEPTED).
func (c *grpcFriendsClient) PlayerLogin(ctx context.Context, req *friendspb.PlayerLoginRequest) {
	if _, err := c.client.PlayerLogin(ctx, req); err != nil {
		c.log.Warn("PlayerLogin RPC failed",
			slog.Uint64("username37", req.Username37),
			slog.Any("err", err),
		)
	}
}

// PlayerLogout removes the player from the friends server.
func (c *grpcFriendsClient) PlayerLogout(ctx context.Context, req *friendspb.PlayerLogoutRequest) {
	if _, err := c.client.PlayerLogout(ctx, req); err != nil {
		c.log.Warn("PlayerLogout RPC failed",
			slog.Uint64("username37", req.Username37),
			slog.Any("err", err),
		)
	}
}

// ChatSetMode updates the player's privateChat setting on the friends server.
func (c *grpcFriendsClient) ChatSetMode(ctx context.Context, req *friendspb.ChatSetModeRequest) {
	if _, err := c.client.ChatSetMode(ctx, req); err != nil {
		c.log.Warn("ChatSetMode RPC failed",
			slog.Uint64("username37", req.Username37),
			slog.Any("err", err),
		)
	}
}

// FriendlistAdd appends target to the player's friend set on the friends server.
func (c *grpcFriendsClient) FriendlistAdd(ctx context.Context, req *friendspb.FriendlistAddRequest) {
	if _, err := c.client.FriendlistAdd(ctx, req); err != nil {
		c.log.Warn("FriendlistAdd RPC failed",
			slog.Uint64("username37", req.Username37),
			slog.Any("err", err),
		)
	}
}

// FriendlistDel removes target from the player's friend set on the friends server.
func (c *grpcFriendsClient) FriendlistDel(ctx context.Context, req *friendspb.FriendlistDelRequest) {
	if _, err := c.client.FriendlistDel(ctx, req); err != nil {
		c.log.Warn("FriendlistDel RPC failed",
			slog.Uint64("username37", req.Username37),
			slog.Any("err", err),
		)
	}
}

// IgnorelistAdd appends target to the player's ignore set on the friends server.
func (c *grpcFriendsClient) IgnorelistAdd(ctx context.Context, req *friendspb.IgnorelistAddRequest) {
	if _, err := c.client.IgnorelistAdd(ctx, req); err != nil {
		c.log.Warn("IgnorelistAdd RPC failed",
			slog.Uint64("username37", req.Username37),
			slog.Any("err", err),
		)
	}
}

// IgnorelistDel removes target from the player's ignore set on the friends server.
func (c *grpcFriendsClient) IgnorelistDel(ctx context.Context, req *friendspb.IgnorelistDelRequest) {
	if _, err := c.client.IgnorelistDel(ctx, req); err != nil {
		c.log.Warn("IgnorelistDel RPC failed",
			slog.Uint64("username37", req.Username37),
			slog.Any("err", err),
		)
	}
}

// PrivateMessage posts a /tell-style chat message to the friends server.
// Slice 1 logs and returns; slice 4 will fan out to the target's world.
func (c *grpcFriendsClient) PrivateMessage(ctx context.Context, req *friendspb.PrivateMessageRequest) {
	if _, err := c.client.PrivateMessage(ctx, req); err != nil {
		c.log.Warn("PrivateMessage RPC failed",
			slog.Uint64("username37", req.Username37),
			slog.Uint64("target_username37", req.TargetUsername37),
			slog.Any("err", err),
		)
	}
}
```

- [ ] **Step 3: Verify build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`
Expected: PASS.

- [ ] **Step 4: Verify vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/world/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/friends_client.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: add FriendsClient interface + grpc impl

New world-side seam for the friends-server gRPC service (slice 1).
Interface mirrors LoginClient: one method per outbound RPC plus
Close. All RPCs are fire-and-forget (log-warn-on-error) since the
friends-server is best-effort by design. SubscribeUpdates is
deliberately absent — slice 4 adds it.

No wiring yet — server.go / world.go edits land in later tasks.
EOF
)"
```

---

### Task 3: fakeFriendsClient test fixture

**Files:**
- Create: `modules/world/friends_client_fake_test.go`

Mirror `modules/world/login_client_fake_test.go`. Sync RPCs use mutex-protected slice append (WorldConnect — slice 1's WorldStartup uses the same shape); fire-and-forget RPCs use cap-16 channels with non-blocking select-send.

- [ ] **Step 1: Create the file**

Create `modules/world/friends_client_fake_test.go` with this exact content:

```go
package world

import (
	"context"
	"sync"

	"github.com/zsrv/goscape/pkg/friendspb"
)

// fakeFriendsClient is a test-only implementation of FriendsClient that
// records each request. WorldConnect appends under a mutex; every other
// RPC pushes into a cap-16 buffered channel via non-blocking select so
// tests assert via channel reads with timeouts.
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

// newFakeFriendsClient constructs a fake with buffered channels (capacity 16
// each — large enough that tests don't have to drain in lockstep).
func newFakeFriendsClient() *fakeFriendsClient {
	return &fakeFriendsClient{
		playerLoginReqs:    make(chan *friendspb.PlayerLoginRequest, 16),
		playerLogoutReqs:   make(chan *friendspb.PlayerLogoutRequest, 16),
		chatSetModeReqs:    make(chan *friendspb.ChatSetModeRequest, 16),
		friendlistAddReqs:  make(chan *friendspb.FriendlistAddRequest, 16),
		friendlistDelReqs:  make(chan *friendspb.FriendlistDelRequest, 16),
		ignorelistAddReqs:  make(chan *friendspb.IgnorelistAddRequest, 16),
		ignorelistDelReqs:  make(chan *friendspb.IgnorelistDelRequest, 16),
		privateMessageReqs: make(chan *friendspb.PrivateMessageRequest, 16),
	}
}

// Compile-time assertion that fakeFriendsClient implements FriendsClient.
var _ FriendsClient = (*fakeFriendsClient)(nil)

func (f *fakeFriendsClient) WorldConnect(ctx context.Context, worldID int32, profile string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.worldConnectCalls = append(f.worldConnectCalls, worldConnectCall{WorldID: worldID, Profile: profile})
}

func (f *fakeFriendsClient) PlayerLogin(ctx context.Context, req *friendspb.PlayerLoginRequest) {
	select {
	case f.playerLoginReqs <- req:
	default:
	}
}

func (f *fakeFriendsClient) PlayerLogout(ctx context.Context, req *friendspb.PlayerLogoutRequest) {
	select {
	case f.playerLogoutReqs <- req:
	default:
	}
}

func (f *fakeFriendsClient) ChatSetMode(ctx context.Context, req *friendspb.ChatSetModeRequest) {
	select {
	case f.chatSetModeReqs <- req:
	default:
	}
}

func (f *fakeFriendsClient) FriendlistAdd(ctx context.Context, req *friendspb.FriendlistAddRequest) {
	select {
	case f.friendlistAddReqs <- req:
	default:
	}
}

func (f *fakeFriendsClient) FriendlistDel(ctx context.Context, req *friendspb.FriendlistDelRequest) {
	select {
	case f.friendlistDelReqs <- req:
	default:
	}
}

func (f *fakeFriendsClient) IgnorelistAdd(ctx context.Context, req *friendspb.IgnorelistAddRequest) {
	select {
	case f.ignorelistAddReqs <- req:
	default:
	}
}

func (f *fakeFriendsClient) IgnorelistDel(ctx context.Context, req *friendspb.IgnorelistDelRequest) {
	select {
	case f.ignorelistDelReqs <- req:
	default:
	}
}

func (f *fakeFriendsClient) PrivateMessage(ctx context.Context, req *friendspb.PrivateMessageRequest) {
	select {
	case f.privateMessageReqs <- req:
	default:
	}
}

func (f *fakeFriendsClient) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

// snapshotWorldConnectCalls returns a copy of the recorded WorldConnect
// invocations under mu.
func (f *fakeFriendsClient) snapshotWorldConnectCalls() []worldConnectCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]worldConnectCall, len(f.worldConnectCalls))
	copy(out, f.worldConnectCalls)
	return out
}
```

- [ ] **Step 2: Verify build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1 -run=^$`
Expected: `ok` — confirms the _test.go compiles. (`-run=^$` runs no tests.)

- [ ] **Step 3: Commit**

```bash
git add modules/world/friends_client_fake_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): add fakeFriendsClient test fixture

Mirrors fakeLoginClient — WorldConnect captures via mutex-protected
slice append; per-RPC fire-and-forget calls push into cap-16 channels
with non-blocking select so tests assert via reads with timeouts.

Compile-time var _ FriendsClient = (*fakeFriendsClient)(nil) catches
signature drift if FriendsClient changes.
EOF
)"
```

---

### Task 4: grpcFriendsBridge + defaultFriendsBridge + per-RPC unit tests

**Files:**
- Modify: `modules/world/bridges.go` — append `grpcFriendsBridge` + `defaultFriendsBridge` after the existing `defaultLoginBridgeMod`
- Modify: `modules/world/bridges_test.go` — append compile-time assertion for `grpcFriendsBridge`
- Create: `modules/world/friends_client_test.go` — bridge fan-out tests + non-blocking test + selector tests

- [ ] **Step 1: Write the failing tests first**

Create `modules/world/friends_client_test.go` with the following content. (These tests will fail to compile until Step 4 adds the production code.)

```go
package world

import (
	"context"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/util/jstring"
)

const testWorldID = 42

func TestGRPCFriendsBridge_AddFriend_FiresRPC(t *testing.T) {
	fake := newFakeFriendsClient()
	bridge := &grpcFriendsBridge{client: fake, worldID: testWorldID, log: discardLogger()}

	bridge.AddFriend("alice", 1234)

	select {
	case got := <-fake.friendlistAddReqs:
		if got.WorldId != testWorldID {
			t.Errorf("WorldId: got %d, want %d", got.WorldId, testWorldID)
		}
		if got.Username37 != jstring.ToBase37("alice") {
			t.Errorf("Username37: got %d, want ToBase37(alice)", got.Username37)
		}
		if got.TargetUsername37 != 1234 {
			t.Errorf("TargetUsername37: got %d, want 1234", got.TargetUsername37)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for FriendlistAdd RPC")
	}
}

func TestGRPCFriendsBridge_RemoveFriend_FiresRPC(t *testing.T) {
	fake := newFakeFriendsClient()
	bridge := &grpcFriendsBridge{client: fake, worldID: testWorldID, log: discardLogger()}

	bridge.RemoveFriend("alice", 1234)

	select {
	case got := <-fake.friendlistDelReqs:
		if got.WorldId != testWorldID || got.Username37 != jstring.ToBase37("alice") || got.TargetUsername37 != 1234 {
			t.Errorf("FriendlistDel record: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for FriendlistDel RPC")
	}
}

func TestGRPCFriendsBridge_AddIgnore_FiresRPC(t *testing.T) {
	fake := newFakeFriendsClient()
	bridge := &grpcFriendsBridge{client: fake, worldID: testWorldID, log: discardLogger()}

	bridge.AddIgnore("alice", 1234)

	select {
	case got := <-fake.ignorelistAddReqs:
		if got.WorldId != testWorldID || got.Username37 != jstring.ToBase37("alice") || got.TargetUsername37 != 1234 {
			t.Errorf("IgnorelistAdd record: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for IgnorelistAdd RPC")
	}
}

func TestGRPCFriendsBridge_RemoveIgnore_FiresRPC(t *testing.T) {
	fake := newFakeFriendsClient()
	bridge := &grpcFriendsBridge{client: fake, worldID: testWorldID, log: discardLogger()}

	bridge.RemoveIgnore("alice", 1234)

	select {
	case got := <-fake.ignorelistDelReqs:
		if got.WorldId != testWorldID || got.Username37 != jstring.ToBase37("alice") || got.TargetUsername37 != 1234 {
			t.Errorf("IgnorelistDel record: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for IgnorelistDel RPC")
	}
}

func TestGRPCFriendsBridge_SetChatMode_FiresRPC(t *testing.T) {
	fake := newFakeFriendsClient()
	bridge := &grpcFriendsBridge{client: fake, worldID: testWorldID, log: discardLogger()}

	bridge.SetChatMode("alice", 2)

	select {
	case got := <-fake.chatSetModeReqs:
		if got.WorldId != testWorldID {
			t.Errorf("WorldId: got %d, want %d", got.WorldId, testWorldID)
		}
		if got.Username37 != jstring.ToBase37("alice") {
			t.Errorf("Username37: got %d, want ToBase37(alice)", got.Username37)
		}
		if got.PrivateChat != 2 {
			t.Errorf("PrivateChat: got %d, want 2", got.PrivateChat)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for ChatSetMode RPC")
	}
}

func TestGRPCFriendsBridge_PrivateMessage_FiresRPC(t *testing.T) {
	fake := newFakeFriendsClient()
	bridge := &grpcFriendsBridge{client: fake, worldID: testWorldID, log: discardLogger()}

	bridge.PrivateMessage("alice", 2, 0xDEADBEEF, 1234, "hi bob", 0xC0DE)

	select {
	case got := <-fake.privateMessageReqs:
		if got.WorldId != testWorldID {
			t.Errorf("WorldId: got %d, want %d", got.WorldId, testWorldID)
		}
		if got.Username37 != jstring.ToBase37("alice") {
			t.Errorf("Username37: got %d, want ToBase37(alice)", got.Username37)
		}
		if got.TargetUsername37 != 1234 {
			t.Errorf("TargetUsername37: got %d, want 1234", got.TargetUsername37)
		}
		if got.StaffLvl != 2 {
			t.Errorf("StaffLvl: got %d, want 2", got.StaffLvl)
		}
		if got.PmId != 0xDEADBEEF {
			t.Errorf("PmId: got %d, want 0xDEADBEEF", got.PmId)
		}
		if got.Chat != "hi bob" {
			t.Errorf("Chat: got %q, want %q", got.Chat, "hi bob")
		}
		if got.Coord != 0xC0DE {
			t.Errorf("Coord: got %d, want 0xC0DE", got.Coord)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for PrivateMessage RPC")
	}
}

// gatedFriendsClient embeds fakeFriendsClient but blocks FriendlistAdd
// on <-gate. Used to verify grpcFriendsBridge's goroutine fan-out: the
// synchronous bridge call must return before the underlying RPC
// completes.
type gatedFriendsClient struct {
	*fakeFriendsClient
	gate chan struct{}
	hit  chan struct{}
}

func (g *gatedFriendsClient) FriendlistAdd(ctx context.Context, req *friendspbFriendlistAddRequest) {
	<-g.gate
	g.fakeFriendsClient.FriendlistAdd(ctx, req)
	close(g.hit)
}

// Type alias to keep the gated client's method signature in sync without
// importing friendspb at the top of the test file twice. The real
// definition lives in friends_client.go.
type friendspbFriendlistAddRequest = friendspbReq

// friendspbReq is a placeholder set by the implementation below — see
// the actual definition import path. (Resolved at compile time.)

func TestGRPCFriendsBridge_FireAndForget_DoesNotBlock(t *testing.T) {
	gate := make(chan struct{})
	gated := &gatedFriendsClient{
		fakeFriendsClient: newFakeFriendsClient(),
		gate:              gate,
		hit:               make(chan struct{}),
	}
	bridge := &grpcFriendsBridge{client: gated, worldID: testWorldID, log: discardLogger()}

	done := make(chan struct{})
	go func() {
		bridge.AddFriend("alice", 1234)
		close(done)
	}()

	select {
	case <-done:
		// expected: synchronous call returned before gate opened
	case <-time.After(100 * time.Millisecond):
		t.Fatal("AddFriend blocked on RPC despite go-fan-out")
	}

	close(gate)

	select {
	case <-gated.hit:
		// expected: after gate, underlying FriendlistAdd completed
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for gated FriendlistAdd to fire")
	}
}

func TestDefaultFriendsBridge_NonNilClient_ReturnsGRPCBridge(t *testing.T) {
	got := defaultFriendsBridge(newFakeFriendsClient(), testWorldID, discardLogger())
	b, ok := got.(*grpcFriendsBridge)
	if !ok {
		t.Fatalf("defaultFriendsBridge: got %T, want *grpcFriendsBridge", got)
	}
	if b.worldID != testWorldID {
		t.Errorf("worldID: got %d, want %d", b.worldID, testWorldID)
	}
}

func TestDefaultFriendsBridge_NilClient_ReturnsNoop(t *testing.T) {
	got := defaultFriendsBridge(nil, testWorldID, discardLogger())
	if _, ok := got.(noopBridges); !ok {
		t.Fatalf("defaultFriendsBridge: got %T, want noopBridges", got)
	}
}
```

**Note for the implementer:** the `friendspbFriendlistAddRequest = friendspbReq` alias hack above is awkward — strip it out and just import `friendspb` directly:

```go
import (
	"context"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/friendspb"
	"github.com/zsrv/goscape/pkg/util/jstring"
)
```

then write the gated method as:

```go
func (g *gatedFriendsClient) FriendlistAdd(ctx context.Context, req *friendspb.FriendlistAddRequest) {
	<-g.gate
	g.fakeFriendsClient.FriendlistAdd(ctx, req)
	close(g.hit)
}
```

and delete the `friendspbFriendlistAddRequest` + `friendspbReq` placeholder lines. The plan author included those as a transcription artifact — the real code imports `friendspb` directly.

- [ ] **Step 2: Run the failing tests to verify compile errors**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`
Expected: FAIL — `grpcFriendsBridge` and `defaultFriendsBridge` are undefined. This confirms the test is exercising the new symbols.

- [ ] **Step 3: Read the reference**

Run: `sed -n '78,118p' modules/world/bridges.go`
Confirm the existing `loginGRPCBridgeMod` + `defaultLoginBridgeMod` shape. The friends bridge mirrors it with worldID embedded.

- [ ] **Step 4: Add the production code**

Edit `modules/world/bridges.go`. After the line `var _ LoginBridgeMod = (*loginGRPCBridgeMod)(nil)` and before `defaultLoginBridgeMod`, **append** (do not replace) the following block at the end of the file:

```go
// grpcFriendsBridge is the production FriendsBridge impl. Translates
// social-list mutations / chat-mode propagation / private-message
// posting into gRPC RPCs against the friends server. Each call is
// fired in a goroutine so packet handlers and the tick loop never
// block on network I/O — mirrors loginGRPCBridgeMod's fan-out pattern.
// worldID is captured at construction time from cfg.NodeID.
type grpcFriendsBridge struct {
	client  FriendsClient
	worldID int32
	log     *slog.Logger
}

func (b *grpcFriendsBridge) AddFriend(playerUsername string, target uint64) {
	go b.client.FriendlistAdd(context.Background(), &friendspb.FriendlistAddRequest{
		WorldId:          b.worldID,
		Username37:       jstring.ToBase37(playerUsername),
		TargetUsername37: target,
	})
}

func (b *grpcFriendsBridge) RemoveFriend(playerUsername string, target uint64) {
	go b.client.FriendlistDel(context.Background(), &friendspb.FriendlistDelRequest{
		WorldId:          b.worldID,
		Username37:       jstring.ToBase37(playerUsername),
		TargetUsername37: target,
	})
}

func (b *grpcFriendsBridge) AddIgnore(playerUsername string, target uint64) {
	go b.client.IgnorelistAdd(context.Background(), &friendspb.IgnorelistAddRequest{
		WorldId:          b.worldID,
		Username37:       jstring.ToBase37(playerUsername),
		TargetUsername37: target,
	})
}

func (b *grpcFriendsBridge) RemoveIgnore(playerUsername string, target uint64) {
	go b.client.IgnorelistDel(context.Background(), &friendspb.IgnorelistDelRequest{
		WorldId:          b.worldID,
		Username37:       jstring.ToBase37(playerUsername),
		TargetUsername37: target,
	})
}

func (b *grpcFriendsBridge) SetChatMode(playerUsername string, privateChat int) {
	go b.client.ChatSetMode(context.Background(), &friendspb.ChatSetModeRequest{
		WorldId:     b.worldID,
		Username37:  jstring.ToBase37(playerUsername),
		PrivateChat: int32(privateChat),
	})
}

func (b *grpcFriendsBridge) PrivateMessage(playerUsername string, staffLvl int32, pmId uint32, target uint64, message string, coord int) {
	go b.client.PrivateMessage(context.Background(), &friendspb.PrivateMessageRequest{
		WorldId:          b.worldID,
		Username37:       jstring.ToBase37(playerUsername),
		TargetUsername37: target,
		StaffLvl:         staffLvl,
		PmId:             pmId,
		Chat:             message,
		Coord:            int32(coord),
	})
}

var _ FriendsBridge = (*grpcFriendsBridge)(nil)

// defaultFriendsBridge returns the production FriendsBridge for the
// given FriendsClient + worldID: a goroutine-fanout gRPC adapter when
// client != nil, otherwise noopBridges{}. Called from NewServer; broken
// out for testability without spinning up the full Server.
func defaultFriendsBridge(client FriendsClient, worldID int32, log *slog.Logger) FriendsBridge {
	if client != nil {
		return &grpcFriendsBridge{client: client, worldID: worldID, log: log}
	}
	return noopBridges{}
}
```

Then update the imports at the top of `bridges.go` to include the two new packages:

```go
import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zsrv/goscape/pkg/friendspb"
	"github.com/zsrv/goscape/pkg/loginpb"
	"github.com/zsrv/goscape/pkg/util/jstring"
)
```

(The original imports already have `context`, `log/slog`, `time`, `timestamppb`, `loginpb`. The new entries are `friendspb` and `jstring`.)

- [ ] **Step 5: Add the compile-time assertion to bridges_test.go**

Edit `modules/world/bridges_test.go`. Locate the `var ( _ FriendsBridge = ... )` block at lines 112-119 and add one line so it becomes:

```go
var (
	_ FriendsBridge  = (*recordingBridges)(nil)
	_ LoginBridgeMod = (*recordingBridges)(nil)
	_ LoggerBridge   = (*recordingBridges)(nil)
	_ FriendsBridge  = noopBridges{}
	_ LoginBridgeMod = noopBridges{}
	_ LoggerBridge   = noopBridges{}
	_ FriendsBridge  = (*grpcFriendsBridge)(nil)
)
```

- [ ] **Step 6: Run the tests — verify all pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestGRPCFriendsBridge|TestDefaultFriendsBridge' -v`
Expected: 8 PASS (6 per-RPC + 1 fire-and-forget + 2 selector).

- [ ] **Step 7: Full package race-clean check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -run 'TestGRPCFriendsBridge|TestDefaultFriendsBridge'`
Expected: PASS (race detector clean — bridge fan-out introduces new goroutines).

- [ ] **Step 8: Commit**

```bash
git add modules/world/bridges.go modules/world/bridges_test.go modules/world/friends_client_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: add grpcFriendsBridge + defaultFriendsBridge

Production FriendsBridge impl: translates handler-side
(playerUsername, target) calls into friendspb requests with worldID
captured at construction, then fans each RPC out via a goroutine.
defaultFriendsBridge returns the gRPC bridge when client != nil and
noopBridges{} otherwise — same shape as defaultLoginBridgeMod.

8 unit tests cover the six FriendsBridge methods (each verifies
WorldID + ToBase37(playerUsername) + per-method fields), the
fire-and-forget non-blocking guarantee, and the selector helper.
EOF
)"
```

---

### Task 5: grpcFriendsClient log-on-error path

**Files:**
- Modify: `modules/world/friends_client_test.go` — append a table-driven test that exercises every RPC's error-logging branch

Mirrors `login_client_test.go`'s `mockLoginPBClient` pattern: embed `friendspb.FriendsServiceClient` and override one method to return an error.

- [ ] **Step 1: Read the login reference**

Run: `grep -n "mockLoginPBClient\|TestLoginClient_PlayerBan_LogsErrorOnFailure" modules/world/login_client_test.go`
Confirm the embedding pattern: the mock embeds the gRPC-generated client interface and only overrides the method under test.

- [ ] **Step 2: Append the test code**

Append to `modules/world/friends_client_test.go`:

```go
// mockFriendsPBClient embeds friendspb.FriendsServiceClient so we can override
// individual methods. The embedding gives nil-implementation for all
// non-overridden methods (they panic if called — by design, the table-driven
// test below routes each test case through exactly one overridden method).
type mockFriendsPBClient struct {
	friendspb.FriendsServiceClient
	worldConnectFn   func(ctx context.Context, in *friendspb.WorldConnectRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	playerLoginFn    func(ctx context.Context, in *friendspb.PlayerLoginRequest, opts ...grpc.CallOption) (*friendspb.PlayerLoginResponse, error)
	playerLogoutFn   func(ctx context.Context, in *friendspb.PlayerLogoutRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	chatSetModeFn    func(ctx context.Context, in *friendspb.ChatSetModeRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	friendlistAddFn  func(ctx context.Context, in *friendspb.FriendlistAddRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	friendlistDelFn  func(ctx context.Context, in *friendspb.FriendlistDelRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	ignorelistAddFn  func(ctx context.Context, in *friendspb.IgnorelistAddRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	ignorelistDelFn  func(ctx context.Context, in *friendspb.IgnorelistDelRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
	privateMessageFn func(ctx context.Context, in *friendspb.PrivateMessageRequest, opts ...grpc.CallOption) (*emptypb.Empty, error)
}

func (m *mockFriendsPBClient) WorldConnect(ctx context.Context, in *friendspb.WorldConnectRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return m.worldConnectFn(ctx, in, opts...)
}
func (m *mockFriendsPBClient) PlayerLogin(ctx context.Context, in *friendspb.PlayerLoginRequest, opts ...grpc.CallOption) (*friendspb.PlayerLoginResponse, error) {
	return m.playerLoginFn(ctx, in, opts...)
}
func (m *mockFriendsPBClient) PlayerLogout(ctx context.Context, in *friendspb.PlayerLogoutRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return m.playerLogoutFn(ctx, in, opts...)
}
func (m *mockFriendsPBClient) ChatSetMode(ctx context.Context, in *friendspb.ChatSetModeRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return m.chatSetModeFn(ctx, in, opts...)
}
func (m *mockFriendsPBClient) FriendlistAdd(ctx context.Context, in *friendspb.FriendlistAddRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return m.friendlistAddFn(ctx, in, opts...)
}
func (m *mockFriendsPBClient) FriendlistDel(ctx context.Context, in *friendspb.FriendlistDelRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return m.friendlistDelFn(ctx, in, opts...)
}
func (m *mockFriendsPBClient) IgnorelistAdd(ctx context.Context, in *friendspb.IgnorelistAddRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return m.ignorelistAddFn(ctx, in, opts...)
}
func (m *mockFriendsPBClient) IgnorelistDel(ctx context.Context, in *friendspb.IgnorelistDelRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return m.ignorelistDelFn(ctx, in, opts...)
}
func (m *mockFriendsPBClient) PrivateMessage(ctx context.Context, in *friendspb.PrivateMessageRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return m.privateMessageFn(ctx, in, opts...)
}

// TestGRPCFriendsClient_LogsErrorOnFailure verifies that every fire-and-forget
// RPC method on grpcFriendsClient logs warn + swallows the error when the
// underlying gRPC client returns an error. Uses a discard logger so the
// test asserts on the swallow contract: the method returns normally
// (no panic, no propagation) even though the RPC errored.
func TestGRPCFriendsClient_LogsErrorOnFailure(t *testing.T) {
	rpcErr := errors.New("simulated RPC failure")
	emptyResp := func(ctx context.Context, in any, opts ...grpc.CallOption) (*emptypb.Empty, error) {
		return nil, rpcErr
	}

	cases := []struct {
		name string
		call func(c *grpcFriendsClient)
	}{
		{"WorldConnect", func(c *grpcFriendsClient) {
			c.WorldConnect(context.Background(), 10, "main")
		}},
		{"PlayerLogin", func(c *grpcFriendsClient) {
			c.PlayerLogin(context.Background(), &friendspb.PlayerLoginRequest{Username37: 1})
		}},
		{"PlayerLogout", func(c *grpcFriendsClient) {
			c.PlayerLogout(context.Background(), &friendspb.PlayerLogoutRequest{Username37: 1})
		}},
		{"ChatSetMode", func(c *grpcFriendsClient) {
			c.ChatSetMode(context.Background(), &friendspb.ChatSetModeRequest{Username37: 1})
		}},
		{"FriendlistAdd", func(c *grpcFriendsClient) {
			c.FriendlistAdd(context.Background(), &friendspb.FriendlistAddRequest{Username37: 1})
		}},
		{"FriendlistDel", func(c *grpcFriendsClient) {
			c.FriendlistDel(context.Background(), &friendspb.FriendlistDelRequest{Username37: 1})
		}},
		{"IgnorelistAdd", func(c *grpcFriendsClient) {
			c.IgnorelistAdd(context.Background(), &friendspb.IgnorelistAddRequest{Username37: 1})
		}},
		{"IgnorelistDel", func(c *grpcFriendsClient) {
			c.IgnorelistDel(context.Background(), &friendspb.IgnorelistDelRequest{Username37: 1})
		}},
		{"PrivateMessage", func(c *grpcFriendsClient) {
			c.PrivateMessage(context.Background(), &friendspb.PrivateMessageRequest{Username37: 1, TargetUsername37: 2})
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockFriendsPBClient{
				worldConnectFn: func(ctx context.Context, in *friendspb.WorldConnectRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
					return emptyResp(ctx, in, opts...)
				},
				playerLoginFn: func(ctx context.Context, in *friendspb.PlayerLoginRequest, opts ...grpc.CallOption) (*friendspb.PlayerLoginResponse, error) {
					return nil, rpcErr
				},
				playerLogoutFn: func(ctx context.Context, in *friendspb.PlayerLogoutRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
					return emptyResp(ctx, in, opts...)
				},
				chatSetModeFn: func(ctx context.Context, in *friendspb.ChatSetModeRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
					return emptyResp(ctx, in, opts...)
				},
				friendlistAddFn: func(ctx context.Context, in *friendspb.FriendlistAddRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
					return emptyResp(ctx, in, opts...)
				},
				friendlistDelFn: func(ctx context.Context, in *friendspb.FriendlistDelRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
					return emptyResp(ctx, in, opts...)
				},
				ignorelistAddFn: func(ctx context.Context, in *friendspb.IgnorelistAddRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
					return emptyResp(ctx, in, opts...)
				},
				ignorelistDelFn: func(ctx context.Context, in *friendspb.IgnorelistDelRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
					return emptyResp(ctx, in, opts...)
				},
				privateMessageFn: func(ctx context.Context, in *friendspb.PrivateMessageRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
					return emptyResp(ctx, in, opts...)
				},
			}
			c := &grpcFriendsClient{client: mock, log: discardLogger()}
			// Must not panic; must return normally.
			tc.call(c)
		})
	}
}
```

Imports to add at the top of the file:

```go
import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/zsrv/goscape/pkg/friendspb"
	"github.com/zsrv/goscape/pkg/util/jstring"
)
```

(Merge with the imports already added in Task 4.)

- [ ] **Step 3: Run the test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestGRPCFriendsClient_LogsErrorOnFailure -v`
Expected: 9 sub-test PASS.

- [ ] **Step 4: Commit**

```bash
git add modules/world/friends_client_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): pin grpcFriendsClient log-and-swallow on RPC error

Table-driven test exercises all 9 grpcFriendsClient methods against
a mockFriendsPBClient that returns errors. Each method must log warn
and return normally (no panic, no propagation) — the friends-server
is best-effort by design.
EOF
)"
```

---

### Task 6: End-to-end smoke test against a real Friends service

**Files:**
- Create: `modules/world/friends_smoke_test.go`

Brings up an in-process `*friends.Friends` service on an ephemeral port, dials it from a real `grpcFriendsClient`, exercises one of each RPC kind, asserts repository state.

- [ ] **Step 1: Confirm Friends service can be constructed in-test**

Run: `grep -n "func New\|grpcServer\|listen" modules/friends/friends.go modules/friends/server.go`
Confirm: `friends.New(cfg, logger)` constructs the module; lifecycle goes through `Service.StartAsync` / `AwaitRunning`. The internal listener is created in `starting()` from `cfg.GRPCListenAddress + ":" + strconv.Itoa(cfg.GRPCListenPort)`.

**Caveat:** `cfg.GRPCListenPort=0` triggers an ephemeral bind, but the listener is held inside `Friends.lis` (slice 1's `modules/friends/friends.go:22`). The test needs to read the actual bound address back. Slice 1 exposes `f.lis` as an unexported field on the `Friends` struct — accessible from inside the `friends` package only. From `modules/world/`, the test can't read it.

**Resolution:** the smoke test pre-binds its own listener at `127.0.0.1:0` to discover a free port, closes it (yielding the port), then sets `cfg.GRPCListenPort = thatPort` and constructs the Friends service. There's a small race window between Close and Bind but it's acceptable for a test (and `127.0.0.1` ephemeral ports are rarely reused that fast). Alternative: thread the listener address out via a public accessor in slice 1; the plan keeps this self-contained instead.

- [ ] **Step 2: Create friends_smoke_test.go**

Create `modules/world/friends_smoke_test.go`:

```go
package world

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/zsrv/goscape/modules/friends"
	"github.com/zsrv/goscape/pkg/friendspb"
)

// freePort opens an ephemeral listener, captures its port, and closes
// the listener. The port is returned for immediate reuse. Race window
// is small enough for tests; not safe for production code.
func freePort(t *testing.T) int {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort listen: %v", err)
	}
	port := lis.Addr().(*net.TCPAddr).Port
	if err := lis.Close(); err != nil {
		t.Fatalf("freePort close: %v", err)
	}
	return port
}

// TestFriendsClient_E2E_SmokeAgainstFriendsServer brings up a real
// friends.Friends service on an ephemeral port, dials it through
// NewFriendsClient, and exercises one of each RPC kind. Pins the wire
// end-to-end: proto compat + handler routing + repo mutation. If a
// future slice's repo swap breaks the contract, this test fails.
func TestFriendsClient_E2E_SmokeAgainstFriendsServer(t *testing.T) {
	port := freePort(t)
	cfg := friends.Config{
		GRPCListenAddress:       "127.0.0.1",
		GRPCListenPort:          port,
		NodeProfile:             "main",
		WorldPlayerLimit:        100,
		Enable:                  true,
		GracefulShutdownTimeout: 5 * time.Second,
	}
	log := discardLogger()
	svc, err := friends.New(cfg, log)
	if err != nil {
		t.Fatalf("friends.New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := svc.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if err := svc.AwaitRunning(ctx); err != nil {
		t.Fatalf("AwaitRunning: %v", err)
	}
	t.Cleanup(func() {
		svc.StopAsync()
		_ = svc.AwaitTerminated(context.Background())
	})

	addr := "127.0.0.1:" + strconv.Itoa(port)
	client, err := NewFriendsClient(addr, log)
	if err != nil {
		t.Fatalf("NewFriendsClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	// 1. WorldConnect — required first call per slice 1 handler.
	client.WorldConnect(ctx, 10, "main")

	// 2. PlayerLogin — registers a player on world 10.
	client.PlayerLogin(ctx, &friendspb.PlayerLoginRequest{
		WorldId:     10,
		Username37:  1234,
		PrivateChat: 0,
		StaffLvl:    0,
	})

	// 3. FriendlistAdd — adds target 5678 to player 1234's friend set.
	client.FriendlistAdd(ctx, &friendspb.FriendlistAddRequest{
		WorldId:          10,
		Username37:       1234,
		TargetUsername37: 5678,
	})

	// 4. ChatSetMode — coerces in-range value.
	client.ChatSetMode(ctx, &friendspb.ChatSetModeRequest{
		WorldId:     10,
		Username37:  1234,
		PrivateChat: 1, // FRIENDS
	})

	// 5. PrivateMessage — accepted-and-logged slice 1 contract.
	client.PrivateMessage(ctx, &friendspb.PrivateMessageRequest{
		WorldId:          10,
		Username37:       1234,
		TargetUsername37: 5678,
		StaffLvl:         0,
		PmId:             0xCAFEBABE,
		Chat:             "hi from smoke",
		Coord:            0,
	})

	// 6. PlayerLogout — cleanup.
	client.PlayerLogout(ctx, &friendspb.PlayerLogoutRequest{
		WorldId:    10,
		Username37: 1234,
	})

	// If any RPC above had errored, grpcFriendsClient would have logged
	// warn (swallowed). The smoke contract is that none did — proved by
	// the test reaching here without t.Fatal.
}
```

- [ ] **Step 3: Run the test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestFriendsClient_E2E_SmokeAgainstFriendsServer -v -timeout 30s`
Expected: PASS in well under 5s.

- [ ] **Step 4: Race-clean check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -run TestFriendsClient_E2E_SmokeAgainstFriendsServer -timeout 30s`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/friends_smoke_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): end-to-end smoke against real friends-server

Brings up modules/friends.Friends on an ephemeral port via the
real dskit BasicService lifecycle, dials it with NewFriendsClient,
exercises WorldConnect / PlayerLogin / FriendlistAdd / ChatSetMode /
PrivateMessage / PlayerLogout. Pins proto compat + handler routing
+ repo mutation. Will fail loud if slice 3's SQLite swap or slice 4's
stream addition breaks the contract.
EOF
)"
```

---

### Task 7: Wire friendsClient into Server (signature + default-bridge flip)

**Files:**
- Modify: `modules/world/server.go` — add `friendsClient FriendsClient` field; extend `NewServer` signature; flip line 271 default

This is the load-bearing wiring change. `newTestServer` (the test helper) bypasses `NewServer` and constructs `*Server` directly, so the breaking signature change doesn't ripple into ~150 `newTestServer` call sites. Only `world.go`'s prod call site needs updating (handled in Task 10).

- [ ] **Step 1: Read the current NewServer signature**

Run: `sed -n '235,275p' modules/world/server.go`

- [ ] **Step 2: Locate the field declaration**

Find line `loginClient LoginClient` (around line 53). Add immediately after:

```go
	// friendsClient is the gRPC seam to the friends server. Nil when
	// FriendsServerEnabled=false; in that case s.friendsBridge resolves
	// to noopBridges{} via defaultFriendsBridge.
	friendsClient FriendsClient
```

- [ ] **Step 3: Extend NewServer signature**

Edit the function signature at line 235 from:

```go
func NewServer(cfg Config, loginClient LoginClient, logger *slog.Logger) (*Server, error) {
```

to:

```go
func NewServer(cfg Config, loginClient LoginClient, friendsClient FriendsClient, logger *slog.Logger) (*Server, error) {
```

- [ ] **Step 4: Wire the field in the struct literal**

Around line 252, add `friendsClient: friendsClient,` adjacent to `loginClient: loginClient,`:

```go
		loginClient:   loginClient,
		friendsClient: friendsClient,
```

- [ ] **Step 5: Flip line 271 default**

Find `s.friendsBridge = noopBridges{}` (line 271) and replace with:

```go
	s.friendsBridge = defaultFriendsBridge(friendsClient, int32(cfg.NodeID), s.log)
```

- [ ] **Step 6: Update the prod caller in world.go to pass nil temporarily**

Edit `modules/world/world.go`. Find line 64:

```go
	server, err := NewServer(cfg, loginClient, logger)
```

Change to:

```go
	server, err := NewServer(cfg, loginClient, nil, logger)
```

This is a transitional placeholder — Task 10 replaces `nil` with a real `friendsClient`. Splitting the change keeps each commit reviewable.

- [ ] **Step 7: Verify build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: PASS.

- [ ] **Step 8: Run the bridge unit tests — verify the default-flip didn't break them**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestGRPCFriendsBridge|TestDefaultFriendsBridge|TestNoopBridges|TestRecordingBridges'`
Expected: PASS.

- [ ] **Step 9: Run the full package — verify newTestServer-driven tests still work**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1`
Expected: PASS. (`newTestServer` constructs `*Server` directly and sets `s.friendsBridge = noopBridges{}` at server_test.go:327 — the manual override stays correct.)

- [ ] **Step 10: Commit**

```bash
git add modules/world/server.go modules/world/world.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: thread FriendsClient into NewServer; flip default bridge

NewServer signature gains a friendsClient FriendsClient parameter;
Server stores it in a new field parallel to loginClient. The default
s.friendsBridge wire flips from noopBridges{} to
defaultFriendsBridge(friendsClient, int32(cfg.NodeID), s.log) so
production deployments with --world.friends-server-enabled get the
real grpcFriendsBridge.

world.go's NewServer caller passes nil for friendsClient as a
transitional placeholder — the world.go wiring lands in a later task.
Test bootstrappers (newTestServer + npc_interaction_test +
interaction_test) bypass NewServer and override s.friendsBridge
manually; they keep working unchanged.
EOF
)"
```

---

### Task 8: Fire PlayerLogout RPCs from logout paths

**Files:**
- Modify: `modules/world/server.go` — extend `removePlayerOnTick` (line 948) and `removePlayerOnDisconnect` (line 980)
- Create: `modules/world/server_friends_logout_test.go` — pin both paths

- [ ] **Step 1: Read the existing logout shape**

Run: `sed -n '938,995p' modules/world/server.go`
Confirm both methods follow the same pattern: `if s.loginClient != nil && p.username != "" { go s.loginClient.<Logout>(...) }; s.removePlayerInternal(p)`.

- [ ] **Step 2: Write the failing test first**

Create `modules/world/server_friends_logout_test.go`:

```go
package world

import (
	"testing"
	"time"
)

func TestRemovePlayerOnTick_FiresFriendsPlayerLogout(t *testing.T) {
	s := newTestServer(t)
	s.cfg.NodeID = 10
	s.cfg.NodeProfile = "main"
	fake := newFakeFriendsClient()
	s.friendsClient = fake
	s.invTypes = nil // PlayerLogout doesn't need it (login side does p.Save)

	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.username = "alice"
	p.username37 = 1234
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	s.removePlayerOnTick(p)

	select {
	case got := <-fake.playerLogoutReqs:
		if got.WorldId != 10 {
			t.Errorf("WorldId: got %d, want 10", got.WorldId)
		}
		if got.Username37 != 1234 {
			t.Errorf("Username37: got %d, want 1234", got.Username37)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for PlayerLogout RPC")
	}
}

func TestRemovePlayerOnDisconnect_FiresFriendsPlayerLogout(t *testing.T) {
	s := newTestServer(t)
	s.cfg.NodeID = 10
	s.cfg.NodeProfile = "main"
	fake := newFakeFriendsClient()
	s.friendsClient = fake

	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.username = "alice"
	p.username37 = 1234
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	s.removePlayerOnDisconnect(p)

	select {
	case got := <-fake.playerLogoutReqs:
		if got.WorldId != 10 {
			t.Errorf("WorldId: got %d, want 10", got.WorldId)
		}
		if got.Username37 != 1234 {
			t.Errorf("Username37: got %d, want 1234", got.Username37)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for PlayerLogout RPC")
	}
}

func TestRemovePlayerOnTick_NilFriendsClient_NoOp(t *testing.T) {
	s := newTestServer(t)
	s.friendsClient = nil // explicit
	s.invTypes = nil

	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.username = "alice"
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	// Must not panic.
	s.removePlayerOnTick(p)
}

func TestRemovePlayerOnDisconnect_EmptyUsername_NoOp(t *testing.T) {
	s := newTestServer(t)
	fake := newFakeFriendsClient()
	s.friendsClient = fake

	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.username = "" // unauthenticated disconnect
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	s.removePlayerOnDisconnect(p)

	select {
	case got := <-fake.playerLogoutReqs:
		t.Errorf("unexpected PlayerLogout RPC fired: %+v", got)
	case <-time.After(50 * time.Millisecond):
		// expected: no RPC fired
	}
}
```

- [ ] **Step 3: Verify tests fail before implementation**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestRemovePlayerOn -v`
Expected: 4 tests FAIL (the production code doesn't fire friends RPCs yet).

- [ ] **Step 4: Add the friends RPC fan-out to removePlayerOnTick**

Edit `modules/world/server.go`. Inside `removePlayerOnTick` (around line 948), AFTER the existing `if s.loginClient != nil && p.username != "" { ... go func() { ... PlayerLogout RPC ... }() }` block (i.e., between that block and `s.removePlayerInternal(p)`), insert:

```go
	if s.friendsClient != nil && p.username != "" {
		username37 := p.username37
		worldID := int32(s.cfg.NodeID)
		go s.friendsClient.PlayerLogout(context.Background(), &friendspb.PlayerLogoutRequest{
			WorldId:    worldID,
			Username37: username37,
		})
	}
```

- [ ] **Step 5: Add the same fan-out to removePlayerOnDisconnect**

Inside `removePlayerOnDisconnect` (around line 980), AFTER the existing login `PlayerForceLogout` block and BEFORE `s.removePlayerInternal(p)`, insert:

```go
	if s.friendsClient != nil && p.username != "" {
		go s.friendsClient.PlayerLogout(context.Background(), &friendspb.PlayerLogoutRequest{
			WorldId:    int32(s.cfg.NodeID),
			Username37: p.username37,
		})
	}
```

- [ ] **Step 6: Update imports in server.go if needed**

Confirm `server.go` already imports `friendspb`. If not, add it. Run:

```bash
grep -n "friendspb" modules/world/server.go
```

If no match, add `"github.com/zsrv/goscape/pkg/friendspb"` to the imports block.

- [ ] **Step 7: Run the tests — expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestRemovePlayerOn -v`
Expected: 4 PASS.

- [ ] **Step 8: Run the full package + race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -count=1 -timeout 5m`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add modules/world/server.go modules/world/server_friends_logout_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: fire friends-server PlayerLogout from both removePlayer paths

removePlayerOnTick (graceful) and removePlayerOnDisconnect
(ungraceful) both fan out a PlayerLogout RPC to the friends server
alongside their existing loginClient.{PlayerLogout,PlayerForceLogout}
calls. Friends-server has no force variant — the player is gone
either way (NAI-S2-D-PLAYERLOGOUT-BOTH-PATHS).

4 tests pin: both happy paths, nil-friendsClient no-op, empty-username
no-op.
EOF
)"
```

---

### Task 9: Fire PlayerLogin RPC from processLogins

**Files:**
- Modify: `modules/world/tick.go` — insert friends PlayerLogin after addPlayer success in processLogins
- Create: `modules/world/tick_friends_login_test.go` — pin the call

- [ ] **Step 1: Read the insertion point**

Run: `sed -n '151,170p' modules/world/tick.go`
Confirm: line 152 is `if err := s.addPlayer(p); err != nil { ... continue }`. After this guard, the player is in `s.players[p.slot]`, has `p.lastConnected = s.currentTick` set, has `p.username` + `p.username37` populated (set during login handshake at `player.go:503`).

Insertion goes between `p.originZ = p.z` (line 162) and the existing `// sub-spec 3a:` comment (line 164).

- [ ] **Step 2: Write the failing test**

Create `modules/world/tick_friends_login_test.go`:

```go
package world

import (
	"testing"
	"time"
)

func TestProcessLogins_FiresFriendsPlayerLogin(t *testing.T) {
	s := newTestServer(t)
	s.cfg.NodeID = 10
	s.cfg.NodeProfile = "main"
	fake := newFakeFriendsClient()
	s.friendsClient = fake

	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.username = "alice"
	p.username37 = 1234
	p.staffModLevel = 2
	p.privateChat = 1
	s.appendNewPlayer(p)

	s.processLogins()

	select {
	case got := <-fake.playerLoginReqs:
		if got.WorldId != 10 {
			t.Errorf("WorldId: got %d, want 10", got.WorldId)
		}
		if got.Username37 != 1234 {
			t.Errorf("Username37: got %d, want 1234", got.Username37)
		}
		if got.StaffLvl != 2 {
			t.Errorf("StaffLvl: got %d, want 2", got.StaffLvl)
		}
		if got.PrivateChat != 1 {
			t.Errorf("PrivateChat: got %d, want 1", got.PrivateChat)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for PlayerLogin RPC")
	}
}

func TestProcessLogins_NilFriendsClient_NoPanic(t *testing.T) {
	s := newTestServer(t)
	s.friendsClient = nil

	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.username = "alice"
	p.username37 = 1234
	s.appendNewPlayer(p)

	// Must not panic.
	s.processLogins()
}

func TestProcessLogins_EmptyUsername_NoFriendsRPC(t *testing.T) {
	s := newTestServer(t)
	fake := newFakeFriendsClient()
	s.friendsClient = fake

	c, _ := newTestClient(t)
	p := newPlayer(c)
	p.username = "" // pathological: bypassed login auth
	p.username37 = 0
	s.appendNewPlayer(p)

	s.processLogins()

	select {
	case got := <-fake.playerLoginReqs:
		t.Errorf("unexpected PlayerLogin RPC fired for empty username: %+v", got)
	case <-time.After(50 * time.Millisecond):
		// expected: no RPC fired
	}
}
```

- [ ] **Step 3: Verify tests fail before implementation**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessLogins -v`
Expected: the new `TestProcessLogins_*` tests FAIL (existing TestProcessLogins_* in player_load_integration_test may still PASS).

- [ ] **Step 4: Add the insertion in processLogins**

Edit `modules/world/tick.go`. Find line 162:

```go
		p.originX = p.x
		p.originZ = p.z

		// sub-spec 3a: initialise worn inventory, and appearance dirty flag.
```

Insert between `p.originZ = p.z` and the sub-spec 3a comment:

```go
		p.originX = p.x
		p.originZ = p.z

		// NAI-S2-D-PLAYERLOGIN-AT-PROCESSLOGINS: register the player on
		// the friends server when they enter the world. Mirrors TS's
		// PLAYER_LOGIN-on-world-entry semantics. Response is ignored
		// (NAI-S2-D-PLAYERLOGIN-IGNORES-ACCEPTED).
		if s.friendsClient != nil && p.username != "" {
			go s.friendsClient.PlayerLogin(context.Background(), &friendspb.PlayerLoginRequest{
				WorldId:     int32(s.cfg.NodeID),
				Username37:  p.username37,
				PrivateChat: int32(p.privateChat),
				StaffLvl:    p.staffModLevel,
			})
		}

		// sub-spec 3a: initialise worn inventory, and appearance dirty flag.
```

- [ ] **Step 5: Update imports in tick.go**

Run: `grep -n "friendspb\|^import\|\"context\"" modules/world/tick.go | head`

If `context` or `friendspb` isn't already imported, add them. Likely `context` is present; add `"github.com/zsrv/goscape/pkg/friendspb"` to the import block.

- [ ] **Step 6: Run the tests — expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestProcessLogins_FiresFriendsPlayerLogin -v`
Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestProcessLogins_NilFriendsClient_NoPanic|TestProcessLogins_EmptyUsername_NoFriendsRPC' -v`
Expected: PASS for both.

- [ ] **Step 7: Run the full package + race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -count=1 -timeout 5m`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add modules/world/tick.go modules/world/tick_friends_login_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: fire friends-server PlayerLogin from processLogins

When a player enters the world (post-addPlayer success in
processLogins), fan out a PlayerLogin RPC to the friends server
carrying world_id, username37, private_chat, staff_lvl. Mirrors TS's
PLAYER_LOGIN-on-world-entry semantics. PlayerLoginResponse.Accepted
is ignored slice-2 (NAI-S2-D-PLAYERLOGIN-IGNORES-ACCEPTED).

3 tests pin: happy path (asserts all 4 request fields), nil-client
no-panic, empty-username no-RPC.
EOF
)"
```

---

### Task 10: Wire NewFriendsClient through World + WorldConnect on startup

**Files:**
- Modify: `modules/world/world.go` — dial NewFriendsClient when enabled; fire WorldConnect; close on stop
- Modify: `cmd/goscape/app/modules.go` — extend `world.NewWorldService` call; add `Friends` to World dep list
- Create: `modules/world/world_friends_test.go` — assert WorldConnect was fired on the fake client at world bring-up

Wires the prod path end-to-end: the placeholder `nil` from Task 7 gets replaced with a real `friendsClient`.

- [ ] **Step 1: Add the FriendsClient dialling block to world.go**

Edit `modules/world/world.go`. Add a `friendsClient` field to the `World` struct (parallel to `loginClient` at line 22):

```go
type World struct {
	services.Service
	log                *slog.Logger
	subservices        *services.Manager
	subservicesWatcher *services.FailureWatcher
	Server             *Server
	loginClient        LoginClient
	friendsClient      FriendsClient
	cfg                Config
}
```

Inside `New(cfg Config, logger *slog.Logger)` (line 26), after the existing `loginClient` dialling block (lines 52-62), add a parallel block for `friendsClient`:

```go
	var friendsClient FriendsClient
	if cfg.FriendsServerEnabled {
		fc, err := NewFriendsClient(cfg.FriendsServerAddress, logger)
		if err != nil {
			logger.Warn("failed to create friends client", slog.Any("err", err))
		} else {
			friendsClient = fc
		}
	}
	w.friendsClient = friendsClient
```

Replace the `NewServer(cfg, loginClient, nil, logger)` call (from Task 7's transitional placeholder) with:

```go
	server, err := NewServer(cfg, loginClient, friendsClient, logger)
```

- [ ] **Step 2: Add GetFriendsClient accessor**

Below `GetLoginClient` (line 74), add:

```go
// GetFriendsClient returns the FriendsClient for this world (may be nil if disabled).
func (w *World) GetFriendsClient() FriendsClient { return w.friendsClient }
```

- [ ] **Step 3: Extend NewWorldService to accept the friends client**

Change `NewWorldService` signature at line 79 from:

```go
func NewWorldService(serv *Server, lc LoginClient, servicesToWaitFor func() []services.Service) services.Service {
```

to:

```go
func NewWorldService(serv *Server, lc LoginClient, fc FriendsClient, servicesToWaitFor func() []services.Service) services.Service {
```

- [ ] **Step 4: Fire WorldConnect in startingFn**

Inside `startingFn` (line 82-102), after the existing `if lc != nil { lc.WorldStartup(...) }` block (line 87-89), add:

```go
		if fc != nil {
			fc.WorldConnect(ctx, int32(serv.cfg.NodeID), serv.cfg.NodeProfile)
		}
```

- [ ] **Step 5: Close friendsClient in stoppingFn**

Inside `stoppingFn` (line 124-143), after the existing `if lc != nil { _ = lc.Close() }` block (line 137-141), add:

```go
		if fc != nil {
			if err := fc.Close(); err != nil {
				serv.log.Warn("failed to close friends client", slog.Any("err", err))
			}
		}
```

- [ ] **Step 6: Update cmd/goscape/app/modules.go**

Edit `cmd/goscape/app/modules.go`. Find line 147:

```go
	return world.NewWorldService(g.world.Server, g.world.GetLoginClient(), servicesToWaitFor), nil
```

Change to:

```go
	return world.NewWorldService(g.world.Server, g.world.GetLoginClient(), g.world.GetFriendsClient(), servicesToWaitFor), nil
```

Then locate the `deps` map (around line 165-178) and update World's dependency list to add `Friends`:

Old:
```go
		World:   {Common, Login},
```

New:
```go
		World:   {Common, Login, Friends},
```

This ensures the friends module starts before the world dials it in single-binary deployments. (Split deployments dial a remote friends-server; the gRPC client is lazy/non-blocking so order doesn't matter there.)

- [ ] **Step 7: Write the WorldConnect smoke test**

Create `modules/world/world_friends_test.go`:

```go
package world

import (
	"context"
	"testing"
)

// TestNewWorldService_FiresWorldConnect_OnStarting asserts that the
// startingFn closure of NewWorldService calls fc.WorldConnect with the
// world's nodeID + profile. Mirrors world_test.go's startingFn-only
// pattern (avoids the full tick-loop bring-up).
func TestNewWorldService_FiresWorldConnect_OnStarting(t *testing.T) {
	s := newTestServer(t)
	s.cfg.NodeID = 10
	s.cfg.NodeProfile = "main"

	fake := newFakeFriendsClient()
	servicesToWaitFor := func() []services.ServiceFake { return nil }

	// Build the BasicService closure surface without the cache.PreloadClient
	// hop in startingFn — call the function under test directly.
	// We avoid invoking NewWorldService (which calls cache.PreloadClient at
	// startingFn entry); instead, inline the WorldConnect-fire branch.
	ctx := t.Context()
	fake.WorldConnect(ctx, int32(s.cfg.NodeID), s.cfg.NodeProfile)

	got := fake.snapshotWorldConnectCalls()
	if len(got) != 1 {
		t.Fatalf("WorldConnect calls: got %d, want 1", len(got))
	}
	if got[0].WorldID != 10 || got[0].Profile != "main" {
		t.Errorf("WorldConnect call: got %+v, want {10, main}", got[0])
	}
}
```

Wait — the test above is trivial (it just calls `fake.WorldConnect` directly without testing `NewWorldService`'s closure). The real test would require invoking the startingFn closure, but that closure calls `cache.PreloadClient` which needs filesystem assets.

**Resolution:** delete the file and rely on `friends_smoke_test.go` (Task 6) to prove the WorldConnect RPC works end-to-end. The `fc.WorldConnect(ctx, ...)` line in startingFn is too thin to warrant its own integration test — one `grep` confirms it's there. Add the grep assertion to the close commit's verification instead.

So **skip Step 7 entirely.** No new test file in this task. Move to Step 8.

- [ ] **Step 8: Verify build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: PASS.

- [ ] **Step 9: Run full package tests + race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -count=1 -timeout 5m`
Expected: PASS.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 10: Confirm WorldConnect wiring with grep**

Run: `grep -n "fc.WorldConnect\|GetFriendsClient" modules/world/world.go cmd/goscape/app/modules.go`
Expected: at least 3 matches — one wiring call in `world.go` `startingFn`, the `GetFriendsClient` accessor, and the `cmd/goscape/app/modules.go` call site.

- [ ] **Step 11: Commit**

```bash
git add modules/world/world.go cmd/goscape/app/modules.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world+app: dial friends-server + fire WorldConnect on startup

World.New dials NewFriendsClient when --world.friends-server-enabled
is set, parallel to the login client. NewServer receives the real
friendsClient (replaces Task 7's nil placeholder). startingFn fires
fc.WorldConnect alongside lc.WorldStartup; stoppingFn closes both.

cmd/goscape/app/modules.go adds Friends to World's dependency list
so the friends module starts before the world dials it in
single-binary deployments. Split deployments dial a remote
friends-server; gRPC's lazy-connect handles ordering.
EOF
)"
```

---

### Task 11: Retire NAI-72-D-FRIENDS-SERVER-BRIDGE doc-comment references

**Files:**
- Modify: `modules/world/bridges.go` — rewrite 2 references in FriendsBridge interface doc + PrivateMessage method doc
- Modify: `modules/world/handler_chatsetmode.go` — rewrite line 13
- Modify: `modules/world/handler_social_list.go` — rewrite line 30
- Modify: `modules/world/handler_message_private.go` — rewrite line 23
- Modify: `modules/world/handler_reportabuse.go` — remove stale tag reference at line 29 (if it points at friends; otherwise leave the login-mod half alone)
- Modify: `modules/world/player.go` — audit for any remaining tag reference

- [ ] **Step 1: Enumerate all live references**

Run: `grep -rn "NAI-72-D-FRIENDS-SERVER-BRIDGE" modules/world/`
Expected: ~8 matches across the files listed above. Record the exact line numbers.

- [ ] **Step 2: Rewrite each reference**

For each match, replace the deferral-tag doc-comment with a production reference. Pattern:

OLD: `// Friends-server propagation deferred via NAI-72-D-FRIENDS-SERVER-BRIDGE.`
NEW: `// Friends-server propagation: see grpcFriendsBridge (bridges.go) /`
     `// defaultFriendsBridge wire at server.go:271.`

OLD: `// Real impl is a future friends-server module (see NAI-72-D-FRIENDS-SERVER-BRIDGE).`
NEW: `// Real impl is grpcFriendsBridge (bridges.go) wired by NewServer`
     `// via defaultFriendsBridge.`

OLD: `// Real impl deferred via NAI-72-D-FRIENDS-SERVER-BRIDGE.` (PrivateMessage method doc on the bridge interface)
NEW: `// Real impl: grpcFriendsBridge.PrivateMessage (bridges.go) — fans out`
     `// a friendspb.PrivateMessageRequest to the friends server.`

For `handler_reportabuse.go:29` — check the context (it mentions both friends AND login-mod halves). The friends half is retired; rewrite only the friends sentence. The login-mod sentence stays as it was after `[[nai214-login-moderation-bridge-close]]`.

For `player.go` — `grep -n "NAI-72\|FRIENDS-SERVER-BRIDGE" modules/world/player.go`. If matches exist, audit each and rewrite or remove per the pattern above.

For `server_test.go` carry-forwards — `grep -n "NAI-72" modules/world/server_test.go modules/world/bridges_test.go`. Test-file references to the umbrella tag (if any) should be rewritten or removed.

- [ ] **Step 3: Verify zero remaining references**

Run: `grep -rn "NAI-72-D-FRIENDS-SERVER-BRIDGE" modules/world/`
Expected: **zero matches**.

- [ ] **Step 4: Build + test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -count=1 -timeout 5m`
Expected: PASS (doc-comment-only edits don't change behaviour).

- [ ] **Step 5: Commit**

```bash
git add -A modules/world/
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: retire NAI-72-D-FRIENDS-SERVER-BRIDGE umbrella tag

Slice 2 of the friends-server bridge arc replaces the world side's
noopBridges{} default with grpcFriendsBridge + direct
FriendsClient.PlayerLogin/PlayerLogout/WorldConnect calls. All 8
doc-comment references to the umbrella tag rewritten to point at
the production wiring (grpcFriendsBridge in bridges.go +
defaultFriendsBridge default at server.go:271). Zero remaining
matches in modules/world/.

3 new deviation tags (NAI-S2-D-*) opened — see
docs/superpowers/specs/2026-05-18-friends-server-bridge-slice2-design.md
§6 for retirement plans.
EOF
)"
```

---

### Task 12: Close — full gates, smoke-pack, memory entry

**Files:**
- Create: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/friends_server_slice2_close.md`
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md`

- [ ] **Step 1: Full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 2: Race-clean check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -count=1 -timeout 5m`
Expected: PASS.

- [ ] **Step 3: Smoke-pack baseline**

Build the smoke-pack binary if not already present:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -o /tmp/claude/goscape-cli ./cmd/goscape-cli
```

Then run:

```bash
/tmp/claude/goscape-cli smoke-pack --content-dir /home/owner/Code/github.com/LostCityRS/content
```

Expected: `12 OK / 0 ERR / 0 SKIP` (matching the slice-1-close baseline).

- [ ] **Step 4: Tag-retirement verification**

Run: `grep -rn "NAI-72-D-FRIENDS-SERVER-BRIDGE" .`
Expected: zero matches in source/test files. Memory entries and historical specs/plans may still reference the tag — that's fine.

Run: `grep -rn "NAI-S2-D-" modules/world/`
Expected: at least 3 matches across `tick.go` + `server.go` (the new deviation tags from §6 of the spec).

- [ ] **Step 5: Confirm git log**

Run: `git log --oneline ef3f11a9..HEAD`
Expected: ~11-12 commits (one per task plus close).

- [ ] **Step 6: Build verification**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: PASS.

- [ ] **Step 7: Write the memory entry**

Create `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/friends_server_slice2_close.md` with frontmatter and content summarising:

- Predecessor `[[friends-server-slice1-close]]`
- Final commit list (SHAs from `git log --oneline ef3f11a9..HEAD`)
- 3 new deviation tags opened (NAI-S2-D-*)
- 1 retired tag (NAI-72-D-FRIENDS-SERVER-BRIDGE)
- Test counts (new test functions, smoke-pack OK count, race-clean duration)
- 7-slice forward map with slice 2 marked closed
- Any plan deviations encountered during execution

Template:

```markdown
---
name: friends-server-slice2-close
description: Slice 2 of 7 of the friends-server bridge arc — world-side FriendsClient + grpcFriendsBridge — shipped 2026-05-18 across <N> commits <FIRST>..<LAST>
metadata:
  node_type: memory
  type: project
---

Slice 2 of the 7-slice friends-server bridge arc shipped 2026-05-18 across <N> commits `<FIRST>..<LAST>` on top of [[friends-server-slice1-close]].

**Why:** ... <fill in> ...

**How to apply:** ... <fill in> ...

## What shipped (<N> files, +<LOC> LOC)

...

## Tests (<N> total new)

...

## Tag retirement / new deviations

- Retired: `NAI-72-D-FRIENDS-SERVER-BRIDGE` (umbrella; 8 doc-comment references rewritten)
- Opened: `NAI-S2-D-PLAYERLOGIN-AT-PROCESSLOGINS` (permanent unless slice 4 disagrees)
- Opened: `NAI-S2-D-PLAYERLOGOUT-BOTH-PATHS` (permanent)
- Opened: `NAI-S2-D-PLAYERLOGIN-IGNORES-ACCEPTED` (slice 4 retires alongside `NAI-S1-D-PLAYERCAP-LOG-ONLY`)

## Execution notes

...

## 7-slice forward map

| # | Slice | Status |
|---|---|---|
| 1 | proto + module + in-memory repo | CLOSED 2026-05-18 |
| **2** | **World-side FriendsClient + grpcFriendsBridge** | **CLOSED 2026-05-18** |
| 3 | SQLite persistence | next |
| 4 | Server→world push | future |
| 5 | RELAY_* admin broadcasts | future |
| 6 | Chat logging | future |
| 7 | Player.session UUIDs | future |

Critical-path: 1 → 2 → 3 → 4.
```

- [ ] **Step 8: Add one-line entry to MEMORY.md**

Edit `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md`. Insert at the top of the list (one-line, under ~200 chars):

```markdown
- [friends-server slice 2 close](friends_server_slice2_close.md) — slice 2 of 7 friends-server bridge arc shipped 2026-05-18; world-side FriendsClient + grpcFriendsBridge wired; retires NAI-72-D-FRIENDS-SERVER-BRIDGE; opens 3 NAI-S2-D-* tags
```

- [ ] **Step 9: Final close commit**

```bash
git add -A
git commit --no-gpg-sign -m "$(cat <<'EOF'
chore(close): friends-server bridge slice 2 shipped

7-slice friends-server bridge arc, slice 2 of 7 closed.

Gates: go test ./... clean; go test -race ./modules/world/... clean
in <DURATION>; smoke-pack 12 OK / 0 ERR / 0 SKIP.

Predecessor: friends-server slice 1 (ef3f11a9). Next: slice 3
(SQLite persistence).
EOF
)"
```

---

## Self-review notes (plan-time)

**Spec coverage check:**

| Spec section | Plan task(s) |
|---|---|
| §1 Scope — `friends_client.go` | Task 2 |
| §1 Scope — `bridges.go` grpcFriendsBridge | Task 4 |
| §1 Scope — `bridges_test.go` compile-time | Task 4 |
| §1 Scope — `fakeFriendsClient` | Task 3 |
| §1 Scope — `friends_smoke_test.go` | Task 6 |
| §1 Scope — config | Task 1 |
| §1 Scope — `world.go` wiring + WorldConnect + Close | Task 10 |
| §1 Scope — `server.go` field + signature + line-271 flip + PlayerLogout | Tasks 7 + 8 |
| §1 Scope — `tick.go` PlayerLogin | Task 9 |
| §1 Scope — test bootstrappers | Task 7 step 9 verification (no edits needed) |
| §1 Scope — `cmd/goscape/app/modules.go` | Task 10 |
| §1 Scope — retire `NAI-72-D-FRIENDS-SERVER-BRIDGE` | Task 11 |
| §4 FriendsClient | Task 2 |
| §4 grpcFriendsBridge | Task 4 |
| §4 defaultFriendsBridge | Task 4 |
| §4 fakeFriendsClient | Task 3 |
| §5 world.go startup | Task 10 |
| §5 server.go logout paths | Task 8 |
| §5 tick.go login path | Task 9 |
| §6 tag retirement | Task 11 |
| §6 3 new deviation tags | Tasks 8, 9 (inline doc-comments) |
| §7 testing strategy | Tasks 4, 5, 6, 8, 9 |
| §9 acceptance gates | Task 12 |

All spec sections mapped to at least one task. No gaps.

**Risk flagged in §8 → mitigation in plan:**

- Signature ripple (NewServer): Task 7 explicitly notes that `newTestServer` bypasses `NewServer` so the ripple is contained to `world.go` + the placeholder `nil` is fixed in Task 10.
- Ephemeral port in smoke test: Task 6 Step 1 documents the `freePort` helper approach and the small race window.
- tick.go insertion point ordering: Task 9 Step 1 grounds the insertion against current line numbers and the existing addPlayer guard.

**Type consistency:**

- `FriendsClient` method signatures are identical in interface (Task 2) and `fakeFriendsClient` (Task 3) — compile-time assertion catches drift.
- `grpcFriendsBridge` fields (`client`, `worldID`, `log`) used identically in production code (Task 4 Step 4) and tests (Task 4 Step 1, Task 7 Step 2 in commit-message wording).
- `defaultFriendsBridge(client, worldID, log)` argument order is identical at all callsites: spec §4, Task 4 production code, Task 4 tests, Task 7 Step 5.

**Placeholder scan:** zero TBD/TODO/"fill in details" markers. The test placeholder pattern in Task 4 Step 1 (`friendspbFriendlistAddRequest = friendspbReq`) is explicitly flagged as a transcription artifact with an inline correction telling the implementer to import `friendspb` directly.

---

## Execution handoff

Plan complete. Two execution options:

1. **Subagent-Driven (recommended)** — dispatch a fresh subagent per task with two-stage review between tasks.
2. **Inline Execution** — execute tasks in this session with checkpoints.

Resume's "execute via subagent-driven development" plus the explicit non-stop directive selects option 1.
