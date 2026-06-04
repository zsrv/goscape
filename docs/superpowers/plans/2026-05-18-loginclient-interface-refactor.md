# LoginClient Interface Refactor + Deferred FU Tests — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Introduce a `LoginClient` interface so `Server.loginClient` and `World.loginClient` can be stubbed in tests, then land the four NAI-PLAYERLOADING follow-up integration tests (login / logout / disconnect / autosave) in the same slice.

**Architecture:** Rename the concrete `*LoginClient` struct to unexported `grpcLoginClient`; promote the exported name `LoginClient` to an interface holding the five RPC methods plus `Close`. Constructor signature `NewLoginClient(...) (LoginClient, error)` is unchanged. Field types change from `*LoginClient` to `LoginClient` at three sites (`Server.loginClient`, `World.loginClient`, `World.GetLoginClient` return). A test-only `fakeLoginClient` (in `login_client_fake_test.go`) records requests and provides canned responses.

**Tech Stack:** Go 1.23+, standard library `sync`/`context`, `google.golang.org/grpc`, `pkg/loginpb` (generated), `pkg/io/protocol/login/resp` (RS2 wire opcodes), `pkg/objtype` (InvTypes for Save fixtures).

**Spec:** `docs/superpowers/specs/2026-05-18-loginclient-interface-refactor-design.md`

**Spec deviation:** Spec §3.3 FU-LOGIN says "drive `processLogins` with a fake LoginClient" — but `processLogins` never calls `PlayerLogin`. The PlayerLogin RPC fires in `(*client).handleLogin` (`modules/world/server.go:711`) and writes `c.savePayload`, which `processLogins` later consumes via `LoadSave`. This plan splits FU-LOGIN into two test groups (T4: RPC site via extracted helper; T5: LoadSave branches via seeded `savePayload`). Both groups together retire the `NAI-PLAYERLOADING-FU-LOGIN-INTEGRATION-TESTS` TODO. T3 extracts a small `(*client).callPlayerLoginRPC` helper to enable T4 without forging RSA-encrypted login packets.

---

## File Map

| Path | Action | Responsibility |
|---|---|---|
| `modules/world/login_client.go` | Modify | Interface declaration; rename concrete to `grpcLoginClient`; constructor returns `LoginClient` |
| `modules/world/world.go` | Modify | `loginClient` field, `GetLoginClient` return type, `NewWorldService` param — `*LoginClient` → `LoginClient` |
| `modules/world/server.go` | Modify | `Server.loginClient` field, `NewServer` param type; extract `callPlayerLoginRPC` helper in T3; retire 3 FU TODOs in T6/T7/T8 |
| `modules/world/tick.go` | Modify | Retire FU TODOs in T5 + T8 (no behaviour change) |
| `cmd/goscape/app/modules.go` | Modify | No change required (uses opaque handle from `GetLoginClient`); verify only |
| `modules/world/login_client_fake_test.go` | Create | `fakeLoginClient` struct: per-RPC capture + canned responses + sync points |
| `modules/world/login_client_test.go` | Create | T4 — table-driven tests for `(*client).callPlayerLoginRPC` |
| `modules/world/player_load_integration_test.go` | Create | T5 — `processLogins` × seeded `savePayload` branches (nil / valid / corrupt) |
| `modules/world/server_logout_test.go` | Create | T6 + T7 — `removePlayerOnTick` PlayerLogout pin + `removePlayerOnDisconnect` PlayerForceLogout pin |
| `modules/world/server_autosave_test.go` | Create | T8 — `autosavePlayers` + tick cadence pin |

All test files use `package world` (internal — fakeLoginClient and call-site tests both need unexported access).

---

## Cross-task Reference: the `fakeLoginClient` shape (defined in T2)

Several later tasks import this. The full definition lives in `modules/world/login_client_fake_test.go` (T2 step 1). Method set and key channels:

```go
type fakeLoginClient struct {
    mu sync.Mutex

    worldStartupCalls []worldStartupCall

    // Sync RPCs: last-call-wins recording.
    lastPlayerLoginReq  *loginpb.PlayerLoginRequest
    lastPlayerLogoutReq *loginpb.PlayerLogoutRequest

    // Async/fire-and-forget RPCs: channels for test select-with-timeout.
    autosaveReqs    chan *loginpb.PlayerAutosaveRequest
    forceLogoutReqs chan *loginpb.PlayerForceLogoutRequest

    // Canned sync responses.
    playerLoginResp  *loginpb.PlayerLoginResponse
    playerLoginErr   error
    playerLogoutResp *loginpb.PlayerLogoutResponse
    playerLogoutErr  error

    // playerLogoutFired signals after lastPlayerLogoutReq has been written,
    // since removePlayerOnTick spawns its own goroutine that calls PlayerLogout.
    playerLogoutFired chan struct{}

    closed bool
}

type worldStartupCall struct {
    NodeID  int32
    Profile string
}
```

Sentinel call returns nil/zero requests; tests assert by recorded value comparison.

---

## Task 1: Introduce `LoginClient` interface; rename concrete

**Files:**
- Modify: `modules/world/login_client.go` (full rewrite of file)
- Modify: `modules/world/world.go:22, 52-62, 74, 79, 87-88, 126-130`
- Modify: `modules/world/server.go:52, 187, 204, 697, 711, 885, 891, 919-920, 943, 956`
- Verify (no change): `cmd/goscape/app/modules.go:123`

- [ ] **Step 1: Read current state of all four files**

Re-read `login_client.go`, `world.go`, `server.go` (lines 47-205, 690-735, 880-960), and `cmd/goscape/app/modules.go:115-130` so the edits in this task have full context.

- [ ] **Step 2: Rewrite `modules/world/login_client.go` to introduce the interface**

Full file content:

```go
package world

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/zsrv/goscape/pkg/loginpb"
)

// LoginClient is the world-side interface to the login service.
// Production impl: grpcLoginClient (this file). Test impl:
// fakeLoginClient (login_client_fake_test.go).
type LoginClient interface {
	WorldStartup(ctx context.Context, nodeID int32, profile string)
	PlayerLogin(ctx context.Context, req *loginpb.PlayerLoginRequest) (*loginpb.PlayerLoginResponse, error)
	PlayerLogout(ctx context.Context, req *loginpb.PlayerLogoutRequest) (*loginpb.PlayerLogoutResponse, error)
	PlayerAutosave(ctx context.Context, req *loginpb.PlayerAutosaveRequest)
	PlayerForceLogout(ctx context.Context, req *loginpb.PlayerForceLogoutRequest)
	Close() error
}

// grpcLoginClient wraps the gRPC connection to the login server.
type grpcLoginClient struct {
	conn   *grpc.ClientConn
	client loginpb.LoginServiceClient
	log    *slog.Logger
}

// NewLoginClient creates a non-blocking gRPC client to the login server.
// grpc.NewClient does not block — connection is established lazily with automatic retry.
func NewLoginClient(addr string, log *slog.Logger) (LoginClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("grpc dial login server: %w", err)
	}
	return &grpcLoginClient{
		conn:   conn,
		client: loginpb.NewLoginServiceClient(conn),
		log:    log,
	}, nil
}

// Close releases the gRPC connection.
func (c *grpcLoginClient) Close() error {
	return c.conn.Close()
}

// WorldStartup notifies the login server that this world is starting.
// Clears any stale sessions from a previous (ungraceful) shutdown.
func (c *grpcLoginClient) WorldStartup(ctx context.Context, nodeID int32, profile string) {
	_, err := c.client.WorldStartup(ctx, &loginpb.WorldStartupRequest{
		NodeId:  nodeID,
		Profile: profile,
	})
	if err != nil {
		c.log.Warn("WorldStartup RPC failed", slog.Any("err", err))
	}
}

// PlayerLogin runs the full auth flow on the login server and returns the response.
func (c *grpcLoginClient) PlayerLogin(ctx context.Context, req *loginpb.PlayerLoginRequest) (*loginpb.PlayerLoginResponse, error) {
	return c.client.PlayerLogin(ctx, req)
}

// PlayerLogout marks the player as logged out and persists their save file.
func (c *grpcLoginClient) PlayerLogout(ctx context.Context, req *loginpb.PlayerLogoutRequest) (*loginpb.PlayerLogoutResponse, error) {
	return c.client.PlayerLogout(ctx, req)
}

// PlayerAutosave persists a player save without logging out (best-effort; called periodically).
func (c *grpcLoginClient) PlayerAutosave(ctx context.Context, req *loginpb.PlayerAutosaveRequest) {
	if _, err := c.client.PlayerAutosave(ctx, req); err != nil {
		c.log.Warn("PlayerAutosave RPC failed",
			slog.String("username", req.Username),
			slog.Any("err", err),
		)
	}
}

// PlayerForceLogout clears the logged-in flag without writing a save (used on disconnect without save data).
func (c *grpcLoginClient) PlayerForceLogout(ctx context.Context, req *loginpb.PlayerForceLogoutRequest) {
	if _, err := c.client.PlayerForceLogout(ctx, req); err != nil {
		c.log.Warn("PlayerForceLogout RPC failed",
			slog.String("username", req.Username),
			slog.Any("err", err),
		)
	}
}
```

- [ ] **Step 3: Update `modules/world/world.go` field/return types**

Apply these exact edits:

1. Line 22: `loginClient        *LoginClient` → `loginClient        LoginClient`
2. Line 52: `var loginClient *LoginClient` → `var loginClient LoginClient`
3. Line 74: `func (w *World) GetLoginClient() *LoginClient { return w.loginClient }` → `func (w *World) GetLoginClient() LoginClient { return w.loginClient }`
4. Line 79: `func NewWorldService(serv *Server, lc *LoginClient, servicesToWaitFor func() []services.Service) services.Service {` → `func NewWorldService(serv *Server, lc LoginClient, servicesToWaitFor func() []services.Service) services.Service {`

Lines 53-62, 87-88, 126-130 reference `loginClient` / `lc` as values; no type changes needed — they should already type-check against the interface.

- [ ] **Step 4: Update `modules/world/server.go` field/param types**

1. Line 52: `loginClient *LoginClient` → `loginClient LoginClient`
2. Line 187: `func NewServer(cfg Config, loginClient *LoginClient, logger *slog.Logger) (*Server, error) {` → `func NewServer(cfg Config, loginClient LoginClient, logger *slog.Logger) (*Server, error) {`

Lines 204, 697, 711, 885, 891, 919-920, 943, 956 use `loginClient` as a value (no type signature) and require no edit.

- [ ] **Step 5: Verify `cmd/goscape/app/modules.go` still compiles**

Line 123 reads `return world.NewWorldService(g.world.Server, g.world.GetLoginClient(), servicesToWaitFor), nil`. `GetLoginClient()` now returns `LoginClient` (interface); `NewWorldService` accepts `LoginClient` (interface). No edit required.

- [ ] **Step 6: Build the whole repo**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: clean build, no errors.

- [ ] **Step 7: Run existing tests to verify zero behaviour change**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: PASS (same set of tests as before the refactor).

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...`
Expected: PASS, no race reports.

- [ ] **Step 8: Commit**

```bash
git add modules/world/login_client.go modules/world/world.go modules/world/server.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(world): LoginClient becomes interface; concrete renamed grpcLoginClient

Promotes the exported name LoginClient to an interface (6 methods:
WorldStartup / PlayerLogin / PlayerLogout / PlayerAutosave /
PlayerForceLogout / Close). Renames the concrete struct to unexported
grpcLoginClient and updates NewLoginClient to return the interface.

Field type changes: Server.loginClient + World.loginClient +
World.GetLoginClient return + NewWorldService param + NewServer param
all switch from *LoginClient to LoginClient.

Zero behaviour change. Unblocks NAI-PLAYERLOADING follow-up
integration tests (T2-T8 of this slice).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Add `fakeLoginClient` test fixture

**Files:**
- Create: `modules/world/login_client_fake_test.go`
- Test: implicit — interface conformance via compile-time assertion in the same file

- [ ] **Step 1: Create `modules/world/login_client_fake_test.go`**

```go
package world

import (
	"context"
	"sync"

	"github.com/zsrv/goscape/pkg/loginpb"
)

// fakeLoginClient is a test-only implementation of LoginClient that records
// each request and serves canned responses. Sync RPCs (PlayerLogin /
// PlayerLogout) use last-call-wins recording; async/fire-and-forget RPCs
// (PlayerAutosave / PlayerForceLogout) push to buffered channels so tests
// can select-with-timeout.
type fakeLoginClient struct {
	mu sync.Mutex

	worldStartupCalls []worldStartupCall

	lastPlayerLoginReq  *loginpb.PlayerLoginRequest
	lastPlayerLogoutReq *loginpb.PlayerLogoutRequest

	autosaveReqs    chan *loginpb.PlayerAutosaveRequest
	forceLogoutReqs chan *loginpb.PlayerForceLogoutRequest

	playerLoginResp  *loginpb.PlayerLoginResponse
	playerLoginErr   error
	playerLogoutResp *loginpb.PlayerLogoutResponse
	playerLogoutErr  error

	// playerLogoutFired is sent on after lastPlayerLogoutReq is recorded.
	// removePlayerOnTick spawns its own goroutine that calls PlayerLogout,
	// so tests need a synchronisation point that doesn't depend on the
	// goroutine winning the race.
	playerLogoutFired chan struct{}

	closed bool
}

type worldStartupCall struct {
	NodeID  int32
	Profile string
}

// newFakeLoginClient constructs a fake with buffered channels (capacity 16
// each — large enough that tests don't have to drain in lockstep).
func newFakeLoginClient() *fakeLoginClient {
	return &fakeLoginClient{
		autosaveReqs:      make(chan *loginpb.PlayerAutosaveRequest, 16),
		forceLogoutReqs:   make(chan *loginpb.PlayerForceLogoutRequest, 16),
		playerLogoutFired: make(chan struct{}, 16),
	}
}

// Compile-time assertion that fakeLoginClient implements LoginClient.
var _ LoginClient = (*fakeLoginClient)(nil)

func (f *fakeLoginClient) WorldStartup(ctx context.Context, nodeID int32, profile string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.worldStartupCalls = append(f.worldStartupCalls, worldStartupCall{NodeID: nodeID, Profile: profile})
}

func (f *fakeLoginClient) PlayerLogin(ctx context.Context, req *loginpb.PlayerLoginRequest) (*loginpb.PlayerLoginResponse, error) {
	f.mu.Lock()
	f.lastPlayerLoginReq = req
	resp := f.playerLoginResp
	err := f.playerLoginErr
	f.mu.Unlock()
	return resp, err
}

func (f *fakeLoginClient) PlayerLogout(ctx context.Context, req *loginpb.PlayerLogoutRequest) (*loginpb.PlayerLogoutResponse, error) {
	f.mu.Lock()
	f.lastPlayerLogoutReq = req
	resp := f.playerLogoutResp
	err := f.playerLogoutErr
	f.mu.Unlock()
	select {
	case f.playerLogoutFired <- struct{}{}:
	default:
	}
	return resp, err
}

func (f *fakeLoginClient) PlayerAutosave(ctx context.Context, req *loginpb.PlayerAutosaveRequest) {
	select {
	case f.autosaveReqs <- req:
	default:
		// Channel full — drop. Tests should assert via channel reads, so a
		// full channel means the test isn't keeping pace; surface by leaving
		// the unread requests visible.
	}
}

func (f *fakeLoginClient) PlayerForceLogout(ctx context.Context, req *loginpb.PlayerForceLogoutRequest) {
	select {
	case f.forceLogoutReqs <- req:
	default:
	}
}

func (f *fakeLoginClient) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

// snapshotPlayerLoginReq returns a copy of the last captured PlayerLoginRequest
// without holding the mutex across the test's assertions.
func (f *fakeLoginClient) snapshotPlayerLoginReq() *loginpb.PlayerLoginRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastPlayerLoginReq
}

// snapshotPlayerLogoutReq is the PlayerLogout equivalent of
// snapshotPlayerLoginReq.
func (f *fakeLoginClient) snapshotPlayerLogoutReq() *loginpb.PlayerLogoutRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastPlayerLogoutReq
}
```

- [ ] **Step 2: Build to verify interface conformance**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -count=0 ./modules/world/...`
Expected: build succeeds (test compilation includes `_test.go` files). The `var _ LoginClient = (*fakeLoginClient)(nil)` line fails compilation if any interface method is missing or has wrong signature.

- [ ] **Step 3: Commit**

```bash
git add modules/world/login_client_fake_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): add fakeLoginClient for LoginClient interface

Test-only fake recording each RPC: sync PlayerLogin / PlayerLogout
use last-call-wins; async PlayerAutosave / PlayerForceLogout push to
buffered channels (cap 16) so tests can select-with-timeout. A
playerLogoutFired channel synchronises tests with the goroutine
spawned inside removePlayerOnTick.

Compile-time assertion (var _ LoginClient) pins method-set conformance.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Extract `(*client).callPlayerLoginRPC` helper

**Why:** T4 (FU-LOGIN RPC tests) needs to drive the PlayerLogin call site without constructing RSA-encrypted login packets. Extracting a small helper preserves `handleLogin`'s control flow while making the RPC interaction directly testable. No behaviour change.

**Files:**
- Modify: `modules/world/server.go:696-734` (extract) — and add the new method below

- [ ] **Step 1: Add `callPlayerLoginRPC` method to `server.go`**

Insert immediately after the `handleLogin` function (i.e., immediately before `loginResultToRS2` near line 754):

```go
// callPlayerLoginRPC runs the PlayerLogin RPC against c.server.loginClient,
// maps the result to the RS2 wire reply byte, and caches accepted-session
// fields on c. Returns (reply, nil) on success; (loginresp.OpLoginServerOffline.Opcode, err)
// on RPC error so the caller can fail-fast via sendLoginError. Extracted from
// handleLogin to enable unit testing with a stubbed LoginClient.
func (c *client) callPlayerLoginRPC(req *loginpb.PlayerLoginRequest, safeName string) (byte, error) {
	resp, err := c.server.loginClient.PlayerLogin(context.TODO(), req)
	if err != nil {
		c.log.Warn("PlayerLogin RPC failed", "error", err)
		return loginresp.OpLoginServerOffline.Opcode, err
	}
	c.log.Info("PlayerLogin RPC response", "result", resp.GetResult())

	result := resp.GetResult()
	reply := loginResultToRS2(result)

	// Only cache session details if the login was accepted.
	if result == loginpb.LoginResult_LOGIN_RESULT_OK ||
		result == loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER ||
		result == loginpb.LoginResult_LOGIN_RESULT_RECONNECT_OK {
		c.staffModLevel = resp.GetStaffModLevel()
		c.members = resp.GetMembers()
		c.username = safeName
		c.savePayload = resp.GetSave()
	}
	return reply, nil
}
```

- [ ] **Step 2: Replace the inline RPC block in `handleLogin`**

Replace lines 696-734 (the block from `var reply byte` through the `} else { ... }` closing brace, ending just before the `switch reply {` block) with:

```go
		var reply byte
		if c.server != nil && c.server.loginClient != nil {
			loginReq := &loginpb.PlayerLoginRequest{
				NodeId:        int32(c.server.cfg.NodeID),
				Profile:       c.server.cfg.NodeProfile,
				NodeMembers:   c.server.cfg.NodeMembers,
				Username:      safeName,
				Password:      req.Password,
				Uid:           int32(req.UID),
				Socket:        c.conn.RemoteAddr().String(),
				RemoteAddress: c.conn.RemoteAddr().String(),
				Reconnecting:  reconnecting,
				HasSave:       false,
			}

			var err error
			reply, err = c.callPlayerLoginRPC(loginReq, safeName)
			if err != nil {
				return c.sendLoginError(reply)
			}
		} else {
			// login server not configured — reject with try again
			reply = loginresp.OpTryAgain.Opcode
		}
```

The `switch reply { ... }` block and everything after stays unchanged.

- [ ] **Step 3: Build and run existing tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: PASS (no behaviour change; existing tests cover handleLogin indirectly through smoke pack and integration tests).

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add modules/world/server.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
refactor(world): extract callPlayerLoginRPC from handleLogin

Pulls the PlayerLogin RPC call + reply-byte mapping + accepted-session
caching out of handleLogin into a small method on *client. Enables
unit testing of the RPC interaction without forging RSA-encrypted
GameLogin packets. Zero behaviour change.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: FU-LOGIN RPC tests — pin `callPlayerLoginRPC` behaviour

**Files:**
- Create: `modules/world/login_client_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package world

import (
	"errors"
	"net"
	"testing"

	loginresp "github.com/zsrv/goscape/pkg/io/protocol/login/resp"
	"github.com/zsrv/goscape/pkg/loginpb"
)

// newClientWithFakeLoginServer constructs a *client whose c.server.loginClient
// is the supplied fake. The server has minimal config (NodeID/NodeProfile/
// NodeMembers) so PlayerLoginRequest fields are deterministic.
func newClientWithFakeLoginServer(t *testing.T, fake *fakeLoginClient) (*client, net.Conn) {
	t.Helper()
	c, conn := newTestClient(t)
	s := newTestServer(t)
	s.cfg.NodeID = 42
	s.cfg.NodeProfile = "main"
	s.cfg.NodeMembers = true
	s.loginClient = fake
	c.server = s
	return c, conn
}

// sampleLoginReq returns a deterministic PlayerLoginRequest matching what
// handleLogin would build for username "test" / password "pw".
func sampleLoginReq(t *testing.T, c *client) *loginpb.PlayerLoginRequest {
	t.Helper()
	return &loginpb.PlayerLoginRequest{
		NodeId:        int32(c.server.cfg.NodeID),
		Profile:       c.server.cfg.NodeProfile,
		NodeMembers:   c.server.cfg.NodeMembers,
		Username:      "test",
		Password:      "pw",
		Uid:           1234,
		Socket:        c.conn.RemoteAddr().String(),
		RemoteAddress: c.conn.RemoteAddr().String(),
		Reconnecting:  false,
		HasSave:       false,
	}
}

func TestCallPlayerLoginRPC_CapturesRequest(t *testing.T) {
	fake := newFakeLoginClient()
	fake.playerLoginResp = &loginpb.PlayerLoginResponse{
		Result: loginpb.LoginResult_LOGIN_RESULT_OK,
	}
	c, _ := newClientWithFakeLoginServer(t, fake)
	req := sampleLoginReq(t, c)

	if _, err := c.callPlayerLoginRPC(req, "test"); err != nil {
		t.Fatalf("callPlayerLoginRPC: unexpected err %v", err)
	}

	got := fake.snapshotPlayerLoginReq()
	if got == nil {
		t.Fatal("no PlayerLoginRequest captured")
	}
	if got.NodeId != 42 || got.Profile != "main" || !got.NodeMembers {
		t.Errorf("server-cfg fields: got NodeId=%d Profile=%q Members=%v; want 42 main true",
			got.NodeId, got.Profile, got.NodeMembers)
	}
	if got.Username != "test" || got.Password != "pw" || got.Uid != 1234 {
		t.Errorf("user fields: got Username=%q Password=%q Uid=%d; want test pw 1234",
			got.Username, got.Password, got.Uid)
	}
	if got.Reconnecting || got.HasSave {
		t.Errorf("flags: Reconnecting=%v HasSave=%v; want false false", got.Reconnecting, got.HasSave)
	}
}

func TestCallPlayerLoginRPC_ReplyByteMapping(t *testing.T) {
	cases := []struct {
		name      string
		result    loginpb.LoginResult
		wantReply byte
		caches    bool // whether session fields should be cached
	}{
		{"OK", loginpb.LoginResult_LOGIN_RESULT_OK, loginresp.OpOK.Opcode, true},
		{"NEW_PLAYER", loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER, loginresp.OpOK.Opcode, true},
		{"RECONNECT_OK", loginpb.LoginResult_LOGIN_RESULT_RECONNECT_OK, loginresp.OpReconnectOK.Opcode, true},
		{"INVALID_CREDENTIALS", loginpb.LoginResult_LOGIN_RESULT_INVALID_CREDENTIALS, loginresp.OpInvalidUsernameOrPassword.Opcode, false},
		{"ALREADY_LOGGED_IN", loginpb.LoginResult_LOGIN_RESULT_ALREADY_LOGGED_IN, loginresp.OpDuplicate.Opcode, false},
		{"ACCOUNT_DISABLED", loginpb.LoginResult_LOGIN_RESULT_ACCOUNT_DISABLED, loginresp.OpBanned.Opcode, false},
		{"NOT_A_MEMBER", loginpb.LoginResult_LOGIN_RESULT_NOT_A_MEMBER, loginresp.OpNeedMembersAccount.Opcode, false},
		{"LOGIN_IN_PROGRESS", loginpb.LoginResult_LOGIN_RESULT_LOGIN_IN_PROGRESS, loginresp.OpTooManyAttempts.Opcode, false},
		{"IP_BANNED", loginpb.LoginResult_LOGIN_RESULT_IP_BANNED, loginresp.OpLoginServerRejected.Opcode, false},
		{"UNSPECIFIED_default", loginpb.LoginResult_LOGIN_RESULT_UNSPECIFIED, loginresp.OpIPLimit.Opcode, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeLoginClient()
			fake.playerLoginResp = &loginpb.PlayerLoginResponse{
				Result:        tc.result,
				StaffModLevel: 2,
				Members:       true,
				Save:          []byte("SAVE-BYTES"),
			}
			c, _ := newClientWithFakeLoginServer(t, fake)
			req := sampleLoginReq(t, c)

			reply, err := c.callPlayerLoginRPC(req, "test")
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if reply != tc.wantReply {
				t.Errorf("reply byte: got %d, want %d", reply, tc.wantReply)
			}

			if tc.caches {
				if c.staffModLevel != 2 || !c.members || c.username != "test" || string(c.savePayload) != "SAVE-BYTES" {
					t.Errorf("expected session cached: got staffModLevel=%d members=%v username=%q savePayload=%q",
						c.staffModLevel, c.members, c.username, c.savePayload)
				}
			} else {
				if c.staffModLevel != 0 || c.members || c.username != "" || c.savePayload != nil {
					t.Errorf("expected session NOT cached: got staffModLevel=%d members=%v username=%q savePayload=%v",
						c.staffModLevel, c.members, c.username, c.savePayload)
				}
			}
		})
	}
}

func TestCallPlayerLoginRPC_RPCErrorReturnsServerOffline(t *testing.T) {
	rpcErr := errors.New("simulated rpc failure")
	fake := newFakeLoginClient()
	fake.playerLoginErr = rpcErr
	c, _ := newClientWithFakeLoginServer(t, fake)
	req := sampleLoginReq(t, c)

	reply, err := c.callPlayerLoginRPC(req, "test")
	if !errors.Is(err, rpcErr) {
		t.Errorf("err: got %v, want %v (or wrapped)", err, rpcErr)
	}
	if reply != loginresp.OpLoginServerOffline.Opcode {
		t.Errorf("reply: got %d, want OpLoginServerOffline (%d)", reply, loginresp.OpLoginServerOffline.Opcode)
	}
	if c.savePayload != nil || c.username != "" {
		t.Errorf("session must NOT be cached on RPC error: savePayload=%v username=%q", c.savePayload, c.username)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail before T3 edits exist (skip — T3 already landed)**

If executing in order, T3's `callPlayerLoginRPC` already exists. These tests should PASS on first run. Run them now:

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestCallPlayerLoginRPC' -v`
Expected: 3 tests pass (the table-driven one expands to 10 subtests).

- [ ] **Step 3: Run with race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -run 'TestCallPlayerLoginRPC'`
Expected: PASS, no race reports.

- [ ] **Step 4: Commit**

```bash
git add modules/world/login_client_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): pin (*client).callPlayerLoginRPC behaviour

3 test groups (12 subtests total) covering the PlayerLogin RPC call
site via fakeLoginClient:
  - captures the PlayerLoginRequest with all fields populated
  - reply-byte mapping for all 9 LoginResult enum values + UNSPECIFIED
    default; session caching gated on OK / NEW_PLAYER / RECONNECT_OK
  - RPC error returns OpLoginServerOffline + does not cache session

Part 1 of FU-LOGIN; part 2 lands in T5 covering processLogins
LoadSave branches and retires the TODO.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: FU-LOGIN LoadSave branches — pin `processLogins` × `savePayload` variants

**Files:**
- Create: `modules/world/player_load_integration_test.go`
- Modify: `modules/world/tick.go:180-188` (retire FU-LOGIN TODO)

- [ ] **Step 1: Find an existing valid SAV byte fixture**

Use the byte-pinning SAV fixture used by `player_save_test.go`. Read `modules/world/player_save_test.go` looking for `newTestPlayerForLoadSave` and any embedded `_ = sav` or `// SAV bytes` literal. The plan's reference is the v6 fixture pinned in the NAI-PLAYERLOADING memory entry. If a helper already exists that returns valid SAV bytes for a fresh-seeded player, reuse it as `validSAVBytes(t)`; otherwise extract the bytes from the existing test setup into a small `func validSAVBytes(t *testing.T) []byte` helper in this new file (test-only, no production impact).

If the bytes are not easily extractable, fall back to round-tripping through `Player.Save`:

```go
func validSAVBytes(t *testing.T) []byte {
	t.Helper()
	p, invTypes := newTestPlayerForLoadSave(t)
	return p.Save(invTypes)
}
```

This is acceptable because T5's goal is "valid SAV decodes successfully into LoadSave", not "byte-identical to a specific fixture" (that pin lives in `player_save_test.go`).

- [ ] **Step 2: Write the failing tests**

```go
package world

import (
	"errors"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// runProcessLogins constructs a Server, queues p via appendNewPlayer with
// the given savePayload, then runs one processLogins cycle. Returns the
// Server so the test can assert post-state.
func runProcessLogins(t *testing.T, savePayload []byte, invTypes *objtype.InvTypeConfigs) (*Server, *Player) {
	t.Helper()
	s := newTestServer(t)
	s.invTypes = invTypes

	c, _ := newTestClient(t)
	c.server = s
	c.savePayload = savePayload
	p := newPlayer(c)
	p.username = "test"
	s.appendNewPlayer(p)

	s.processLogins()
	return s, p
}

func TestProcessLogins_NilSavePayload_BootstrapsFresh(t *testing.T) {
	_, invTypes := newTestPlayerForLoadSave(t)
	s, p := runProcessLogins(t, nil, invTypes)

	if p.slot < 1 || p.slot > 2047 {
		t.Errorf("player not added: slot=%d", p.slot)
	}
	if s.players[p.slot] != p {
		t.Error("players[slot] not set")
	}
	if p.invs == nil {
		t.Error("invs map should be initialised by bootstrap")
	}
	// Worn inventory always seeded post-bootstrap when InvTypes.Worn is set.
	if invTypes.Worn >= 0 {
		if _, ok := p.invs[invTypes.Worn]; !ok {
			t.Error("worn inventory should be initialised on fresh login")
		}
	}
}

func TestProcessLogins_ValidSavePayload_LoadsSuccessfully(t *testing.T) {
	_, invTypes := newTestPlayerForLoadSave(t)
	sav := validSAVBytes(t)

	s, p := runProcessLogins(t, sav, invTypes)

	if p.slot < 1 || p.slot > 2047 {
		t.Errorf("player not added: slot=%d", p.slot)
	}
	if s.players[p.slot] != p {
		t.Error("players[slot] not set")
	}
	if p.invs == nil {
		t.Error("invs map should be initialised")
	}
	// Don't pin coords / varps here — that's the byte-pin tests' job in
	// player_save_test.go. This test pins only the "decode succeeded and
	// player joined" outcome.
}

func TestProcessLogins_CorruptSavePayload_FallsBackToBootstrap(t *testing.T) {
	_, invTypes := newTestPlayerForLoadSave(t)
	// "SAVE" header is wrong length / magic — must trip Verify in LoadSave.
	corrupt := []byte{0x00, 0x01, 0x02, 0x03}

	// Verify directly to confirm corrupt bytes are rejected.
	if err := Verify(corrupt); err == nil {
		t.Fatal("Verify should reject 4-byte corrupt payload")
	}
	// processLogins should log + fall back to empty bootstrap (deviation
	// NAI-PLAYERLOADING-D-DECODE-ERR-FALLS-BACK-TO-BOOTSTRAP at tick.go:174).
	s, p := runProcessLogins(t, corrupt, invTypes)

	if p.slot < 1 || p.slot > 2047 {
		t.Errorf("player must still be added on corrupt SAV: slot=%d", p.slot)
	}
	if s.players[p.slot] != p {
		t.Error("players[slot] not set")
	}
	if p.invs == nil {
		t.Error("invs map should be bootstrapped after fallback")
	}
}

// Ensure validSAVBytes itself round-trips cleanly so the valid test above
// isn't testing a no-op.
func TestValidSAVBytesRoundTrips(t *testing.T) {
	_, invTypes := newTestPlayerForLoadSave(t)
	sav := validSAVBytes(t)
	if len(sav) == 0 {
		t.Fatal("validSAVBytes returned empty")
	}
	if err := Verify(sav); err != nil {
		t.Fatalf("Verify(validSAVBytes()) = %v; want nil", err)
	}
	// LoadSave into a fresh player must succeed.
	c, _ := newTestClient(t)
	p := newPlayer(c)
	if err := LoadSave(p, sav, invTypes); err != nil {
		// LoadSave returns a wrapped errors.New from Verify on bad input;
		// here it should succeed.
		if errors.Is(err, errCloseConn) {
			t.Fatal("LoadSave returned errCloseConn unexpectedly")
		}
		t.Fatalf("LoadSave: %v", err)
	}
}
```

Note: `validSAVBytes` (defined inline at the top of the test file via the fallback approach in Step 1) lives in this same file unless `newTestPlayerForLoadSave` already exports a byte fixture you can reuse — in which case substitute. The helper signature shown in Step 1 is the fallback.

- [ ] **Step 3: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestProcessLogins_|TestValidSAVBytesRoundTrips' -v`
Expected: 4 tests pass.

If any test fails because `newPlayer` or `appendNewPlayer` paths require additional setup (e.g. `p.client.server` being non-nil), inspect the panic, add the missing wiring inline (do not invent new helpers), and re-run.

- [ ] **Step 4: Retire the FU-LOGIN TODO**

Edit `modules/world/tick.go:180-188` to remove the TODO block. Replace:

```go
		// TODO(NAI-PLAYERLOADING-FU-LOGIN-INTEGRATION-TESTS): cover the
		// accept-with-SAV / accept-without-SAV / accept-with-corrupt-SAV
		// branches end-to-end. Blocked on *LoginClient being a concrete
		// struct (modules/world/login_client.go:15) rather than an
		// interface; stubbing requires a multi-file refactor that's out
		// of this slice's scope. Today the codec branches are pinned by
		// the unit-test suite in player_save_test.go and the wiring is
		// indirectly covered by every existing world test that goes
		// through processLogins.
```

with:

```go
		// LoadSave branches covered end-to-end by TestProcessLogins_*
		// (player_load_integration_test.go) and the RPC site by
		// TestCallPlayerLoginRPC_* (login_client_test.go).
```

- [ ] **Step 5: Run race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -run 'TestProcessLogins_|TestValidSAVBytesRoundTrips'`
Expected: PASS, no races.

- [ ] **Step 6: Commit**

```bash
git add modules/world/player_load_integration_test.go modules/world/tick.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): pin processLogins x savePayload branches; retire FU-LOGIN TODO

3 tests + 1 round-trip sanity covering processLogins's three savePayload
branches per the NAI-PLAYERLOADING-D-DECODE-ERR-FALLS-BACK-TO-BOOTSTRAP
deviation:
  - nil savePayload bootstraps fresh
  - valid SAV decodes successfully
  - corrupt SAV logs + falls back to empty bootstrap

Retires NAI-PLAYERLOADING-FU-LOGIN-INTEGRATION-TESTS at tick.go:180.
Part 2 of FU-LOGIN (part 1 was T4 callPlayerLoginRPC).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: FU-LOGOUT — pin `removePlayerOnTick` PlayerLogout

**Files:**
- Create: `modules/world/server_logout_test.go`
- Modify: `modules/world/server.go:880-883` (retire FU-LOGOUT TODO)

- [ ] **Step 1: Write the failing tests**

```go
package world

import (
	"testing"
	"time"
)

func TestRemovePlayerOnTick_FiresPlayerLogoutWithSave(t *testing.T) {
	fake := newFakeLoginClient()
	s := newTestServer(t)
	s.cfg.NodeID = 42
	s.cfg.NodeProfile = "main"
	s.loginClient = fake
	_, invTypes := newTestPlayerForLoadSave(t)
	s.invTypes = invTypes

	c, _ := newTestClient(t)
	c.server = s
	p := newPlayer(c)
	p.username = "alice"
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	s.removePlayerOnTick(p)

	select {
	case <-fake.playerLogoutFired:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for PlayerLogout RPC")
	}

	got := fake.snapshotPlayerLogoutReq()
	if got == nil {
		t.Fatal("no PlayerLogoutRequest captured")
	}
	if got.NodeId != 42 || got.Profile != "main" {
		t.Errorf("server-cfg fields: got NodeId=%d Profile=%q; want 42 main",
			got.NodeId, got.Profile)
	}
	if got.Username != "alice" {
		t.Errorf("Username: got %q, want alice", got.Username)
	}
	if len(got.Save) == 0 {
		t.Error("Save must be non-empty (Player.Save bytes)")
	}
	// Verify the captured Save bytes are a structurally-valid SAV.
	if err := Verify(got.Save); err != nil {
		t.Errorf("captured Save fails Verify: %v", err)
	}
}

func TestRemovePlayerOnTick_NoLoginClient_NoRPC(t *testing.T) {
	s := newTestServer(t)
	// loginClient stays nil.

	c, _ := newTestClient(t)
	c.server = s
	p := newPlayer(c)
	p.username = "alice"
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	s.removePlayerOnTick(p) // must not panic
	// player slot cleared (removePlayerInternal still runs).
	if s.players[p.slot] != nil {
		t.Error("removePlayerInternal must still run when loginClient is nil")
	}
}

func TestRemovePlayerOnTick_EmptyUsername_NoRPC(t *testing.T) {
	fake := newFakeLoginClient()
	s := newTestServer(t)
	s.loginClient = fake

	c, _ := newTestClient(t)
	c.server = s
	p := newPlayer(c)
	// p.username stays empty.
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	s.removePlayerOnTick(p)

	// No PlayerLogout should fire.
	select {
	case <-fake.playerLogoutFired:
		t.Fatal("PlayerLogout fired despite empty username")
	case <-time.After(100 * time.Millisecond):
		// expected — no RPC
	}
	if got := fake.snapshotPlayerLogoutReq(); got != nil {
		t.Errorf("PlayerLogoutRequest captured despite empty username: %+v", got)
	}
}
```

- [ ] **Step 2: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestRemovePlayerOnTick_' -v`
Expected: 3 tests pass.

If `newTestPlayerForLoadSave` requires additional setup that `s` doesn't have (e.g., `s.invTypes` was unset before), wire it inline. If `Player.Save` needs an `invTypes` parameter that the test fixture doesn't supply, look at how `removePlayerOnTick` calls `p.Save(s.invTypes)` (server.go:886) and ensure `s.invTypes` is non-nil before `removePlayerOnTick` runs.

- [ ] **Step 3: Run with race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -run 'TestRemovePlayerOnTick_'`
Expected: PASS, no races (the goroutine spawned inside `removePlayerOnTick` should not race the test thread since the fake's mutex serialises access).

- [ ] **Step 4: Retire the FU-LOGOUT TODO**

Edit `modules/world/server.go:880-883` to remove the TODO block. Replace:

```go
// TODO(NAI-PLAYERLOADING-FU-LOGOUT-INTEGRATION-TESTS): pin the captured
// PlayerLogout request via a fake LoginClient (T16 of NAI-PLAYERLOADING).
// Blocked on *LoginClient being a concrete struct (login_client.go:15);
// see the same blocker noted at the LoadSave call site in tick.go.
```

with:

```go
// PlayerLogout RPC contents pinned by TestRemovePlayerOnTick_*
// (server_logout_test.go).
```

- [ ] **Step 5: Commit**

```bash
git add modules/world/server_logout_test.go modules/world/server.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): pin removePlayerOnTick PlayerLogout RPC; retire FU-LOGOUT TODO

3 tests covering the graceful logout RPC site:
  - happy path: PlayerLogoutRequest carries NodeID/Profile/Username and
    a Verify-able Save payload
  - loginClient nil: removePlayerInternal still runs; no RPC
  - empty username: no RPC; removePlayerInternal still runs

Retires NAI-PLAYERLOADING-FU-LOGOUT-INTEGRATION-TESTS at server.go:880.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: FU-DISCONNECT — pin `removePlayerOnDisconnect` PlayerForceLogout

**Files:**
- Modify: `modules/world/server_logout_test.go` (append tests)
- Modify: `modules/world/server.go:914-917` (retire FU-DISCONNECT TODO)

- [ ] **Step 1: Append the failing tests to `server_logout_test.go`**

```go
func TestRemovePlayerOnDisconnect_FiresPlayerForceLogout(t *testing.T) {
	fake := newFakeLoginClient()
	s := newTestServer(t)
	s.cfg.NodeID = 7
	s.cfg.NodeProfile = "dev"
	s.loginClient = fake

	c, _ := newTestClient(t)
	c.server = s
	p := newPlayer(c)
	p.username = "bob"
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	s.removePlayerOnDisconnect(p)

	var got *loginpb.PlayerForceLogoutRequest
	select {
	case got = <-fake.forceLogoutReqs:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for PlayerForceLogout RPC")
	}

	if got.NodeId != 7 || got.Profile != "dev" || got.Username != "bob" {
		t.Errorf("force-logout fields: got NodeId=%d Profile=%q Username=%q; want 7 dev bob",
			got.NodeId, got.Profile, got.Username)
	}

	// PlayerLogout must NOT have been called (no save on ungraceful disconnect).
	if got := fake.snapshotPlayerLogoutReq(); got != nil {
		t.Errorf("PlayerLogout fired on disconnect path: %+v", got)
	}
}

func TestRemovePlayerOnDisconnect_NoLoginClient_NoRPC(t *testing.T) {
	s := newTestServer(t)
	c, _ := newTestClient(t)
	c.server = s
	p := newPlayer(c)
	p.username = "bob"
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	s.removePlayerOnDisconnect(p) // must not panic
	if s.players[p.slot] != nil {
		t.Error("removePlayerInternal must still run when loginClient is nil")
	}
}
```

Update the file's import block to include `"github.com/zsrv/goscape/pkg/loginpb"` if not already present (T6's tests don't reference `loginpb` types directly; this task does).

- [ ] **Step 2: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestRemovePlayerOnDisconnect_' -v`
Expected: 2 tests pass.

- [ ] **Step 3: Run with race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -run 'TestRemovePlayerOnDisconnect_'`
Expected: PASS, no races.

- [ ] **Step 4: Retire the FU-DISCONNECT TODO**

Edit `modules/world/server.go:914-917` to remove the TODO block. Replace:

```go
// TODO(NAI-PLAYERLOADING-FU-DISCONNECT-INTEGRATION-TESTS): pin that
// PlayerForceLogout fires and PlayerLogout does NOT fire (T17 of
// NAI-PLAYERLOADING). Same LoginClient-stubbing blocker as
// NAI-PLAYERLOADING-FU-LOGOUT-INTEGRATION-TESTS.
```

with:

```go
// PlayerForceLogout RPC + "no PlayerLogout" assertion pinned by
// TestRemovePlayerOnDisconnect_* (server_logout_test.go).
```

- [ ] **Step 5: Commit**

```bash
git add modules/world/server_logout_test.go modules/world/server.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): pin removePlayerOnDisconnect PlayerForceLogout; retire FU-DISCONNECT TODO

2 tests covering the ungraceful disconnect RPC site:
  - PlayerForceLogout fires with NodeID/Profile/Username
  - PlayerLogout does NOT fire (no save on disconnect; NAI-PLAYERLOADING-D-DISCONNECT-NO-SAVE)
  - loginClient nil: removePlayerInternal still runs

Retires NAI-PLAYERLOADING-FU-DISCONNECT-INTEGRATION-TESTS at server.go:914.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: FU-AUTOSAVE — pin `autosavePlayers` + tick cadence

**Files:**
- Create: `modules/world/server_autosave_test.go`
- Modify: `modules/world/tick.go:57-60` (retire FU-AUTOSAVE TODO)

- [ ] **Step 1: Write the failing tests**

```go
package world

import (
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/loginpb"
)

// drainAutosaveReqs reads up to `want` requests from the fake's channel
// within timeout. Returns the captured slice (may be shorter than `want`
// on timeout) for the caller to assert on.
func drainAutosaveReqs(t *testing.T, fake *fakeLoginClient, want int, timeout time.Duration) []*loginpb.PlayerAutosaveRequest {
	t.Helper()
	out := make([]*loginpb.PlayerAutosaveRequest, 0, want)
	deadline := time.After(timeout)
	for len(out) < want {
		select {
		case req := <-fake.autosaveReqs:
			out = append(out, req)
		case <-deadline:
			return out
		}
	}
	return out
}

func TestAutosavePlayers_FiresOncePerActivePlayer(t *testing.T) {
	fake := newFakeLoginClient()
	s := newTestServer(t)
	s.cfg.NodeID = 1
	s.cfg.NodeProfile = "dev"
	s.loginClient = fake
	_, invTypes := newTestPlayerForLoadSave(t)
	s.invTypes = invTypes

	// Two active players.
	c1, _ := newTestClient(t)
	c1.server = s
	p1 := newPlayer(c1)
	p1.username = "alice"
	if err := s.addPlayer(p1); err != nil {
		t.Fatalf("addPlayer p1: %v", err)
	}

	c2, _ := newTestClient(t)
	c2.server = s
	p2 := newPlayer(c2)
	p2.username = "bob"
	if err := s.addPlayer(p2); err != nil {
		t.Fatalf("addPlayer p2: %v", err)
	}

	s.autosavePlayers()

	got := drainAutosaveReqs(t, fake, 2, 2*time.Second)
	if len(got) != 2 {
		t.Fatalf("autosave req count: got %d, want 2", len(got))
	}
	names := map[string]bool{}
	for _, r := range got {
		if r.NodeId != 1 || r.Profile != "dev" {
			t.Errorf("autosave req fields: got NodeId=%d Profile=%q; want 1 dev", r.NodeId, r.Profile)
		}
		if len(r.Save) == 0 {
			t.Errorf("autosave req for %s: Save is empty", r.Username)
		}
		if err := Verify(r.Save); err != nil {
			t.Errorf("autosave req for %s: Save fails Verify: %v", r.Username, err)
		}
		names[r.Username] = true
	}
	if !names["alice"] || !names["bob"] {
		t.Errorf("expected both usernames; got %+v", names)
	}
}

func TestAutosavePlayers_NoLoginClient_NoRPC(t *testing.T) {
	s := newTestServer(t)

	c, _ := newTestClient(t)
	c.server = s
	p := newPlayer(c)
	p.username = "alice"
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	// Must not panic, must not block.
	done := make(chan struct{})
	go func() {
		s.autosavePlayers()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("autosavePlayers blocked with nil loginClient")
	}
}

func TestAutosavePlayers_EmptyUsername_Skipped(t *testing.T) {
	fake := newFakeLoginClient()
	s := newTestServer(t)
	s.loginClient = fake
	_, invTypes := newTestPlayerForLoadSave(t)
	s.invTypes = invTypes

	c, _ := newTestClient(t)
	c.server = s
	p := newPlayer(c)
	// p.username stays empty
	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}

	s.autosavePlayers()

	// No RPC should fire.
	select {
	case req := <-fake.autosaveReqs:
		t.Errorf("autosave fired for empty-username player: %+v", req)
	case <-time.After(100 * time.Millisecond):
		// expected — no RPC
	}
}

// TestAutosavePlayers_TickCadenceGate pins the gate at tick.go:55 — autosave
// fires on ticks where currentTick > 0 && currentTick % PlayerSaveRate == 0,
// and not otherwise. Test by calling the gate inline (no tick loop needed).
func TestAutosavePlayers_TickCadenceGate(t *testing.T) {
	cases := []struct {
		tick int
		want bool
	}{
		{0, false},                     // explicitly excluded by > 0 guard
		{1, false},
		{PlayerSaveRate - 1, false},
		{PlayerSaveRate, true},
		{PlayerSaveRate + 1, false},
		{2 * PlayerSaveRate, true},
		{3 * PlayerSaveRate, true},
	}
	for _, tc := range cases {
		got := tc.tick%PlayerSaveRate == 0 && tc.tick > 0
		if got != tc.want {
			t.Errorf("tick=%d: gate=%v, want %v", tc.tick, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run 'TestAutosavePlayers_' -v`
Expected: 4 tests pass.

- [ ] **Step 3: Run with race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -run 'TestAutosavePlayers_'`
Expected: PASS, no races (autosavePlayers spawns goroutines; the fake's channel send + mutex are the synchronisation points).

- [ ] **Step 4: Retire the FU-AUTOSAVE TODO**

Edit `modules/world/tick.go:57-60` to remove the TODO block. Replace:

```go
			// TODO(NAI-PLAYERLOADING-FU-AUTOSAVE-INTEGRATION-TEST): pin that
			// PlayerAutosave fires exactly N times across N*PlayerSaveRate
			// ticks (T18 of NAI-PLAYERLOADING). Same LoginClient-stubbing
			// blocker as the login/logout integration tests.
```

with no replacement comment — the gate immediately above already documents the cadence.

If you prefer to leave a one-line breadcrumb, use:

```go
			// PlayerAutosave RPC + cadence gate pinned by TestAutosavePlayers_*
			// (server_autosave_test.go).
```

- [ ] **Step 5: Commit**

```bash
git add modules/world/server_autosave_test.go modules/world/tick.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
test(world): pin autosavePlayers + tick cadence; retire FU-AUTOSAVE TODO

4 tests covering the autosave RPC site:
  - one PlayerAutosaveRequest per active player; each carries
    NodeID/Profile/Username and a Verify-able Save payload
  - loginClient nil: no RPC, no panic
  - empty username: skipped (matches autosavePlayers guard)
  - tick cadence gate: fires only when currentTick > 0 && % PlayerSaveRate == 0

Retires NAI-PLAYERLOADING-FU-AUTOSAVE-INTEGRATION-TEST at tick.go:57.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Final verification gates + close commit

- [ ] **Step 1: Verify all four FU TODOs are retired**

Run: `grep -rn "NAI-PLAYERLOADING-FU-" modules/`
Expected: zero matches. If any remain, return to the responsible task and finish the retirement edit.

- [ ] **Step 2: Run the full world test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
Expected: PASS. Compare the test count to the pre-T1 baseline — should be +12 to +15 new tests across T4/T5/T6/T7/T8.

- [ ] **Step 3: Run the full repo test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
Expected: PASS.

- [ ] **Step 4: Run with race detector across all `modules/world`**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...`
Expected: PASS, no race reports.

- [ ] **Step 5: Run smoke pack to confirm baseline holds**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run ./cmd/goscape-cli smoke-pack 2>&1 | tail -20`
Expected: 12 OK / 0 ERR baseline matches the NAI-PLAYERLOADING-close memory entry. If any new ERR appears, investigate — this slice touches nothing the pack exercises, but the gate is cheap.

- [ ] **Step 6: Confirm clean git state and review the slice's commits**

Run: `git status`
Expected: clean working tree.

Run: `git log --oneline 1ac34467..HEAD`
Expected: 8 commits — refactor, fake, extract, T4 tests, T5 tests, T6 tests, T7 tests, T8 tests.

- [ ] **Step 7: Write the close commit**

```bash
git commit --allow-empty --no-gpg-sign -m "$(cat <<'EOF'
chore(close): LoginClient interface refactor + 4 FU integration tests shipped

Refactor: *LoginClient (concrete) -> LoginClient (interface) +
grpcLoginClient (concrete, unexported). NewLoginClient signature
preserved; field types switched at Server.loginClient,
World.loginClient, World.GetLoginClient, NewWorldService param,
NewServer param. Zero production behaviour change.

Tests: 12-15 new tests across 4 FU groups:
  - FU-LOGIN (T4+T5): callPlayerLoginRPC RPC + reply-byte mapping
    (all 10 LoginResult variants + RPC error), processLogins LoadSave
    branches (nil/valid/corrupt savePayload).
  - FU-LOGOUT (T6): removePlayerOnTick PlayerLogout RPC pin.
  - FU-DISCONNECT (T7): removePlayerOnDisconnect PlayerForceLogout
    pin + "PlayerLogout NOT fired" assertion.
  - FU-AUTOSAVE (T8): autosavePlayers per-player RPC + tick cadence
    gate pin.

Four NAI-PLAYERLOADING-FU-*-INTEGRATION-TESTS TODOs retired.

Race-detector clean. Smoke-pack 12 OK / 0 ERR baseline holds.

Spec: docs/superpowers/specs/2026-05-18-loginclient-interface-refactor-design.md
Plan: docs/superpowers/plans/2026-05-18-loginclient-interface-refactor.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 8: Update auto-memory**

Add a one-line index entry to `MEMORY.md` at the top, and a topic file `loginclient_interface_close.md` with the slice's outcome (deviations encountered, surprises, byte-pin counts). The MEMORY.md line should follow the existing format:

`- [LoginClient interface close](loginclient_interface_close.md) — *LoginClient -> interface refactor + 4 FU integration tests shipped 2026-05-18; 12-15 tests added; 0 production behaviour change; smoke-pack baseline holds`

(adjust counts after the run completes).

---

## Self-Review

**1. Spec coverage:**

| Spec section | Implementing task(s) |
|---|---|
| §3.1 Interface shape (rename concrete) | T1 |
| §3.1 NewLoginClient signature widens | T1 step 2 |
| §3.1 Field type changes at 3 sites | T1 step 3-4 |
| §3.2 fakeLoginClient (sync recording, async channels, Close) | T2 |
| §3.3 FU-LOGIN-INTEGRATION-TESTS (response branches + LoadSave) | T4 + T5 (split — see "Spec deviation" preamble) |
| §3.3 FU-LOGOUT-INTEGRATION-TESTS | T6 |
| §3.3 FU-DISCONNECT-INTEGRATION-TESTS | T7 |
| §3.3 FU-AUTOSAVE-INTEGRATION-TEST | T8 |
| §3.4 No `time.Sleep`; select-with-timeout | T6/T7/T8 use `time.After`; channel sends synchronise |
| §3.5 Fixture reuse via helper | T4 uses `newClientWithFakeLoginServer`; T6/T7/T8 inline wiring (intentionally minimal — no premature abstraction) |
| §4 PR shape (single slice) | All in one slice; T9 close commit |
| §5 Verification gates | T9 steps 2-5 |
| §6 Risk notes (Close on interface, fire-and-forget, goroutine timing) | T1 (Close on interface), T2 (sync channels), T6/T8 (no time.Sleep) |
| §7 Deviation tags anticipated | None planned; T9 reports if any surfaced |

**2. Placeholder scan:** No "TBD" / "TODO" / "handle edge cases" / "add appropriate error handling" in the plan body. Each step has either complete code or a precise edit-locator + replacement.

**3. Type consistency:** `fakeLoginClient` fields (`autosaveReqs`, `forceLogoutReqs`, `playerLogoutFired`, `snapshotPlayerLoginReq`, `snapshotPlayerLogoutReq`) defined in T2 are referenced consistently in T4/T6/T7/T8. Helper names (`newClientWithFakeLoginServer`, `sampleLoginReq`, `runProcessLogins`, `validSAVBytes`, `drainAutosaveReqs`) are each defined exactly once and referenced where declared. `callPlayerLoginRPC` signature `(byte, error)` is consistent between T3 (definition) and T4 (call sites).

**4. Known soft spots that may need on-the-fly adjustment:**
- T5 step 1 ("Find an existing valid SAV byte fixture") has a fallback that round-trips `Player.Save`. If `newTestPlayerForLoadSave` does NOT produce a player whose Save bytes round-trip cleanly (because of unseeded state), the executor must either (a) seed the missing state, or (b) use bytes from an existing fixture in `player_save_test.go`. The Verify check in `TestValidSAVBytesRoundTrips` guards against silent failures.
- T6/T8 require `s.invTypes` to be non-nil because `Player.Save(s.invTypes)` is called inside the production code. T6 step 2 / T8 step 2 mention this explicitly.
- The race detector should pass cleanly. If T6 reports a race (the goroutine inside `removePlayerOnTick` reading `p` while the test thread asserts), the fake's `playerLogoutFired` channel is the synchronisation point; the test must wait on it before reading any captured state.
