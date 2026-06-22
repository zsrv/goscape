# NAI-214 login moderation bridge — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire `LoginBridgeMod.NotifyPlayer{Ban,Mute}` (currently `noopBridges`) into the existing `modules/login` `PlayerBan`/`PlayerMute` gRPC RPCs via a thin `loginGRPCBridgeMod` adapter, and flip `NewServer`'s default so any production server constructed with a non-nil `LoginClient` automatically routes moderation actions to the login server.

**Architecture:** Two atomic seams — (a) extend `world.LoginClient` interface with two fire-and-forget RPC methods (`PlayerBan`, `PlayerMute`) implemented on both `grpcLoginClient` and `fakeLoginClient`; (b) add a `loginGRPCBridgeMod` adapter that turns each `Notify*` into a `go b.client.Player{Ban,Mute}(...)` call with `time.Time → *timestamppb.Timestamp` translation, and a `defaultLoginBridgeMod` package-level helper for the constructor default-flip.

**Tech Stack:** Go 1.26+; existing `google.golang.org/grpc` + `google.golang.org/protobuf/types/known/timestamppb`; `log/slog`. No new dependencies, no proto changes, no DB migrations.

**Spec:** `docs/superpowers/specs/2026-05-18-nai-214-login-moderation-bridge-design.md`

---

## Pre-flight context

Before starting Task 1, an executor unfamiliar with the codebase should know:

- **Project rule (CLAUDE.md):** All `go` invocations must be prefixed `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache`. All `git commit` must pass `--no-gpg-sign`.
- **TDD cadence:** Write failing test → run → confirm failure → minimal impl → run → confirm pass → commit. Don't skip the "confirm failure" step (it's how you know the test exercises new code, not existing code).
- **LoginClient compile-cascade:** `LoginClient` is an interface with two impls (`grpcLoginClient` in `login_client.go`, `fakeLoginClient` in `login_client_fake_test.go`). Adding a method to the interface requires both impls to land in the same commit — otherwise `go build ./...` and `go test ./...` fail to compile.
- **`fakeLoginClient` capture convention** (`login_client_fake_test.go:15-45`): sync RPCs use last-call-wins fields under `f.mu`; fire-and-forget RPCs use `make(chan *Req, 16)` with non-blocking `select`/`default` send. PlayerBan/Mute are fire-and-forget → use channels.
- **`grpcLoginClient` swallow-and-log convention** (`login_client.go:78-95`): fire-and-forget methods return `void`. On RPC error, log a `Warn` with keys `"username"` (string) + `"err"` (any). No retry, no return.
- **`recordingBridges`** (`bridges_test.go`) already captures `NotifyPlayerBan`/`Mute` calls. It is unchanged by this slice — handler-level tests using `installRecordingBridges` continue to pass without edits.
- **`newTestServer`** (`server_test.go:300+`) builds `Server{}` directly and explicitly sets `s.loginBridgeMod = noopBridges{}`. This line stays — the production default flip via `NewServer` does not reach test-only construction.
- **Why `go` fan-out:** Bridge call sites (`handler_reportabuse.go:50,60`, `handler_message_private.go:42`, `handlers_game.go:1187,1206`) are inside synchronous packet/handler paths. Blocking on a network RPC would stall the tick. Existing fire-and-forget RPCs (`PlayerAutosave`, `PlayerForceLogout`) at `server.go:982,1018` already use `go ...` fan-out — bridge matches that policy.
- **No deviation tag introduced.** TS posts moderation via `loginThread.postMessage` (a worker-thread MessagePort); goscape uses gRPC throughout. That architectural substitution predates this slice; no new tag is needed.

---

## File structure

| File | State | Responsibility |
|---|---|---|
| `modules/world/login_client.go` | MODIFIED | Adds `PlayerBan` + `PlayerMute` to `LoginClient` interface and `grpcLoginClient` impl (~30 LOC, mirrors existing `PlayerAutosave` block at L78-86). |
| `modules/world/login_client_test.go` | MODIFIED | Adds tests for `grpcLoginClient.PlayerBan`/`.PlayerMute` log-on-error path using an in-package mock `loginpb.LoginServiceClient`. |
| `modules/world/login_client_fake_test.go` | MODIFIED | Adds `playerBanReqs`, `playerMuteReqs` cap-16 buffered channels to `fakeLoginClient` + matching method impls + channel init in `newFakeLoginClient`. |
| `modules/world/bridges.go` | MODIFIED | Adds `loginGRPCBridgeMod` type, compile-time interface check, and `defaultLoginBridgeMod(client, log)` helper; retires the `NAI-72-D-LOGIN-SERVER-BRIDGE-MOD` doc-comment. |
| `modules/world/bridges_test.go` | MODIFIED | Adds 5 new tests: 3 for bridge fan-out (NotifyPlayerBan/Mute fires RPC, fire-and-forget non-blocking), 2 for `defaultLoginBridgeMod` selection (nil → noop, non-nil → grpc). |
| `modules/world/server.go` | MODIFIED | Replaces `s.loginBridgeMod = noopBridges{}` (line 272) with `s.loginBridgeMod = defaultLoginBridgeMod(loginClient, s.log)`. One-line change. |

No new files. No changes to `proto/login/`, `pkg/loginpb/`, `modules/login/`, `cmd/goscape/app/`, `go.mod`, or `go.sum`.

---

## Task 1: Extend `LoginClient` interface with `PlayerBan` + `PlayerMute`

**Why this task exists:** The interface lives in `login_client.go`; both `grpcLoginClient` (production) and `fakeLoginClient` (test) must implement every method or the package fails to compile. This task lands all three changes (interface + grpc impl + fake impl) in one atomic step, plus log-on-error unit tests for the grpc impl. After this commit, `LoginClient` exposes the RPCs the bridge will call in Task 2, but nothing else has changed — `loginBridgeMod` is still `noopBridges{}`, so production behaviour is unchanged.

**Files:**
- Modify: `modules/world/login_client.go` (interface + grpcLoginClient.PlayerBan/Mute)
- Modify: `modules/world/login_client_fake_test.go` (fakeLoginClient.PlayerBan/Mute + capture channels)
- Modify: `modules/world/login_client_test.go` (new tests for log-on-error)

### Step 1.1: Write failing test for `grpcLoginClient.PlayerBan` error logging

- [ ] **Step 1.1: Write the failing test**

Append to `modules/world/login_client_test.go`:

```go
// mockLoginPBClient is an in-package stub of loginpb.LoginServiceClient
// used by grpcLoginClient unit tests. Only the methods exercised by tests
// are overridden; the embedded loginpb.LoginServiceClient panics on any
// unstubbed call (intentional — unexpected calls should surface loudly).
type mockLoginPBClient struct {
	loginpb.LoginServiceClient

	mu              sync.Mutex
	gotPlayerBanReq  *loginpb.PlayerBanRequest
	gotPlayerMuteReq *loginpb.PlayerMuteRequest
	playerBanErr     error
	playerMuteErr    error
}

func (m *mockLoginPBClient) PlayerBan(ctx context.Context, in *loginpb.PlayerBanRequest, opts ...grpc.CallOption) (*loginpb.PlayerBanResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gotPlayerBanReq = in
	if m.playerBanErr != nil {
		return nil, m.playerBanErr
	}
	return &loginpb.PlayerBanResponse{}, nil
}

func (m *mockLoginPBClient) PlayerMute(ctx context.Context, in *loginpb.PlayerMuteRequest, opts ...grpc.CallOption) (*loginpb.PlayerMuteResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gotPlayerMuteReq = in
	if m.playerMuteErr != nil {
		return nil, m.playerMuteErr
	}
	return &loginpb.PlayerMuteResponse{}, nil
}

func TestGRPCLoginClient_PlayerBan_PassesRequest(t *testing.T) {
	mock := &mockLoginPBClient{}
	c := &grpcLoginClient{client: mock, log: discardLogger()}

	req := &loginpb.PlayerBanRequest{
		Staff:    "alice",
		Username: "evilbob",
		Until:    timestamppb.New(time.Unix(1747569600, 0)),
	}
	c.PlayerBan(context.Background(), req)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.gotPlayerBanReq == nil {
		t.Fatal("PlayerBan was not invoked on underlying client")
	}
	if mock.gotPlayerBanReq.Staff != "alice" || mock.gotPlayerBanReq.Username != "evilbob" {
		t.Errorf("req fields: got Staff=%q Username=%q; want alice evilbob",
			mock.gotPlayerBanReq.Staff, mock.gotPlayerBanReq.Username)
	}
}

func TestGRPCLoginClient_PlayerBan_LogsErrorOnFailure(t *testing.T) {
	var logBuf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	mock := &mockLoginPBClient{playerBanErr: errors.New("rpc down")}
	c := &grpcLoginClient{client: mock, log: log}

	c.PlayerBan(context.Background(), &loginpb.PlayerBanRequest{Username: "evilbob"})

	got := logBuf.String()
	if !strings.Contains(got, "PlayerBan RPC failed") {
		t.Errorf("log output missing message; got: %s", got)
	}
	if !strings.Contains(got, "evilbob") {
		t.Errorf("log output missing username; got: %s", got)
	}
	if !strings.Contains(got, "rpc down") {
		t.Errorf("log output missing error; got: %s", got)
	}
}

func TestGRPCLoginClient_PlayerMute_PassesRequest(t *testing.T) {
	mock := &mockLoginPBClient{}
	c := &grpcLoginClient{client: mock, log: discardLogger()}

	req := &loginpb.PlayerMuteRequest{
		Staff:    "alice",
		Username: "evilbob",
		Until:    timestamppb.New(time.Unix(1747569600, 0)),
	}
	c.PlayerMute(context.Background(), req)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.gotPlayerMuteReq == nil {
		t.Fatal("PlayerMute was not invoked on underlying client")
	}
	if mock.gotPlayerMuteReq.Staff != "alice" || mock.gotPlayerMuteReq.Username != "evilbob" {
		t.Errorf("req fields: got Staff=%q Username=%q; want alice evilbob",
			mock.gotPlayerMuteReq.Staff, mock.gotPlayerMuteReq.Username)
	}
}

func TestGRPCLoginClient_PlayerMute_LogsErrorOnFailure(t *testing.T) {
	var logBuf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	mock := &mockLoginPBClient{playerMuteErr: errors.New("rpc down")}
	c := &grpcLoginClient{client: mock, log: log}

	c.PlayerMute(context.Background(), &loginpb.PlayerMuteRequest{Username: "evilbob"})

	got := logBuf.String()
	if !strings.Contains(got, "PlayerMute RPC failed") {
		t.Errorf("log output missing message; got: %s", got)
	}
	if !strings.Contains(got, "evilbob") {
		t.Errorf("log output missing username; got: %s", got)
	}
	if !strings.Contains(got, "rpc down") {
		t.Errorf("log output missing error; got: %s", got)
	}
}
```

Update the import block at the top of `modules/world/login_client_test.go` to add:

```go
import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	loginresp "github.com/zsrv/goscape/pkg/io/protocol/login/resp"
	"github.com/zsrv/goscape/pkg/loginpb"
)
```

(Existing imports `errors`, `net`, `testing`, `loginresp`, `loginpb` already present — leave them; this block shows the full final state of the import.)

- [ ] **Step 1.2: Run tests to verify they fail (compile error)**

Run:
```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestGRPCLoginClient_Player(Ban|Mute)' -v
```

Expected: compile error. Either `grpcLoginClient.PlayerBan: undefined` (if the test compiles past `mockLoginPBClient` before failing) or `loginpb.LoginServiceClient` interface-satisfaction error (because `LoginClient` will gain methods that `*grpcLoginClient` doesn't yet have, breaking compile-time assertions in other files).

This is the expected "red" state: the methods don't exist yet.

### Step 1.3: Extend `LoginClient` interface

- [ ] **Step 1.3: Edit `modules/world/login_client.go` interface**

Change the interface block (lines 16-23) to:

```go
// LoginClient is the world-side interface to the login service.
// Production impl: grpcLoginClient (this file). Test impl:
// fakeLoginClient (login_client_fake_test.go).
type LoginClient interface {
	WorldStartup(ctx context.Context, nodeID int32, profile string)
	PlayerLogin(ctx context.Context, req *loginpb.PlayerLoginRequest) (*loginpb.PlayerLoginResponse, error)
	PlayerLogout(ctx context.Context, req *loginpb.PlayerLogoutRequest) (*loginpb.PlayerLogoutResponse, error)
	PlayerAutosave(ctx context.Context, req *loginpb.PlayerAutosaveRequest)
	PlayerForceLogout(ctx context.Context, req *loginpb.PlayerForceLogoutRequest)
	PlayerBan(ctx context.Context, req *loginpb.PlayerBanRequest)
	PlayerMute(ctx context.Context, req *loginpb.PlayerMuteRequest)
	Close() error
}
```

### Step 1.4: Implement `grpcLoginClient.PlayerBan` + `.PlayerMute`

- [ ] **Step 1.4: Append to `modules/world/login_client.go` after the existing `PlayerForceLogout` method**

```go
// PlayerBan sets the banned_until timestamp on the login server (fire-and-forget).
func (c *grpcLoginClient) PlayerBan(ctx context.Context, req *loginpb.PlayerBanRequest) {
	if _, err := c.client.PlayerBan(ctx, req); err != nil {
		c.log.Warn("PlayerBan RPC failed",
			slog.String("username", req.Username),
			slog.Any("err", err),
		)
	}
}

// PlayerMute sets the muted_until timestamp on the login server (fire-and-forget).
func (c *grpcLoginClient) PlayerMute(ctx context.Context, req *loginpb.PlayerMuteRequest) {
	if _, err := c.client.PlayerMute(ctx, req); err != nil {
		c.log.Warn("PlayerMute RPC failed",
			slog.String("username", req.Username),
			slog.Any("err", err),
		)
	}
}
```

### Step 1.5: Extend `fakeLoginClient` with capture channels

- [ ] **Step 1.5: Edit `modules/world/login_client_fake_test.go`**

Add two fields to the `fakeLoginClient` struct (after `forceLogoutReqs`, around line 24):

```go
	playerBanReqs   chan *loginpb.PlayerBanRequest
	playerMuteReqs  chan *loginpb.PlayerMuteRequest
```

Add two channel initialisers inside `newFakeLoginClient()` (after `forceLogoutReqs`):

```go
		playerBanReqs:    make(chan *loginpb.PlayerBanRequest, 16),
		playerMuteReqs:   make(chan *loginpb.PlayerMuteRequest, 16),
```

Add two methods at the bottom of the file, mirroring `PlayerAutosave`/`PlayerForceLogout`:

```go
func (f *fakeLoginClient) PlayerBan(ctx context.Context, req *loginpb.PlayerBanRequest) {
	select {
	case f.playerBanReqs <- req:
	default:
	}
}

func (f *fakeLoginClient) PlayerMute(ctx context.Context, req *loginpb.PlayerMuteRequest) {
	select {
	case f.playerMuteReqs <- req:
	default:
	}
}
```

### Step 1.6: Run tests to verify pass

- [ ] **Step 1.6: Run the new tests + full module suite**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestGRPCLoginClient_Player(Ban|Mute)' -v
```

Expected: 4 tests PASS (`_PassesRequest` × 2, `_LogsErrorOnFailure` × 2).

Then run the full module suite to verify nothing else regressed (the interface extension cascades through all callers):

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: PASS (no other tests touch `PlayerBan`/`PlayerMute`; the only new state is the four `TestGRPCLoginClient_Player(Ban|Mute)_*` tests).

### Step 1.7: Commit

- [ ] **Step 1.7: Commit**

```bash
git add modules/world/login_client.go modules/world/login_client_fake_test.go modules/world/login_client_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: extend LoginClient with PlayerBan + PlayerMute RPCs

Adds PlayerBan and PlayerMute to the LoginClient interface as
fire-and-forget RPCs (return void, log-and-swallow errors).
grpcLoginClient delegates to the existing generated client;
fakeLoginClient captures into cap-16 channels matching the
PlayerAutosave/PlayerForceLogout convention.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Add `loginGRPCBridgeMod` adapter

**Why this task exists:** The adapter is the bridge between the `LoginBridgeMod` interface (called by handlers with `time.Time` arguments) and the `LoginClient` RPCs (which take `*loginpb.Player{Ban,Mute}Request` with `*timestamppb.Timestamp` fields). It also adds the `go ...` goroutine fan-out so synchronous handler paths never block on network I/O. After this task, the type exists and is unit-tested but not yet wired into `NewServer`.

**Files:**
- Modify: `modules/world/bridges.go` (add type + interface assertion)
- Modify: `modules/world/bridges_test.go` (add 3 tests)

### Step 2.1: Write failing tests for bridge fan-out

- [ ] **Step 2.1: Append to `modules/world/bridges_test.go`**

```go
func TestLoginGRPCBridgeMod_NotifyPlayerBan_FiresRPC(t *testing.T) {
	fake := newFakeLoginClient()
	bridge := &loginGRPCBridgeMod{client: fake, log: discardLogger()}

	until := time.Unix(1747569600, 0)
	bridge.NotifyPlayerBan("alice", "evilbob", until)

	select {
	case got := <-fake.playerBanReqs:
		if got.Staff != "alice" {
			t.Errorf("Staff: got %q, want alice", got.Staff)
		}
		if got.Username != "evilbob" {
			t.Errorf("Username: got %q, want evilbob", got.Username)
		}
		if !got.Until.AsTime().Equal(until) {
			t.Errorf("Until: got %v, want %v", got.Until.AsTime(), until)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for PlayerBan RPC")
	}
}

func TestLoginGRPCBridgeMod_NotifyPlayerMute_FiresRPC(t *testing.T) {
	fake := newFakeLoginClient()
	bridge := &loginGRPCBridgeMod{client: fake, log: discardLogger()}

	until := time.Unix(1747569600, 0)
	bridge.NotifyPlayerMute("alice", "evilbob", until)

	select {
	case got := <-fake.playerMuteReqs:
		if got.Staff != "alice" {
			t.Errorf("Staff: got %q, want alice", got.Staff)
		}
		if got.Username != "evilbob" {
			t.Errorf("Username: got %q, want evilbob", got.Username)
		}
		if !got.Until.AsTime().Equal(until) {
			t.Errorf("Until: got %v, want %v", got.Until.AsTime(), until)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for PlayerMute RPC")
	}
}

// gatedLoginClient is a one-off fake whose PlayerBan blocks on <-gate
// before recording. Used to verify the bridge's go-fan-out: the
// synchronous NotifyPlayerBan call must return before the underlying
// RPC completes.
type gatedLoginClient struct {
	*fakeLoginClient
	gate chan struct{}
	hit  chan struct{}
}

func (g *gatedLoginClient) PlayerBan(ctx context.Context, req *loginpb.PlayerBanRequest) {
	<-g.gate
	g.fakeLoginClient.PlayerBan(ctx, req)
	close(g.hit)
}

func TestLoginGRPCBridgeMod_FireAndForget_DoesNotBlock(t *testing.T) {
	gate := make(chan struct{})
	gated := &gatedLoginClient{
		fakeLoginClient: newFakeLoginClient(),
		gate:            gate,
		hit:             make(chan struct{}),
	}
	bridge := &loginGRPCBridgeMod{client: gated, log: discardLogger()}

	done := make(chan struct{})
	go func() {
		bridge.NotifyPlayerBan("alice", "evilbob", time.Now())
		close(done)
	}()

	select {
	case <-done:
		// expected: synchronous call returned before gate opened
	case <-time.After(100 * time.Millisecond):
		t.Fatal("NotifyPlayerBan blocked on RPC despite go-fan-out")
	}

	close(gate)

	select {
	case <-gated.hit:
		// expected: after gate, underlying PlayerBan completed
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for gated PlayerBan to fire")
	}
}
```

Update `modules/world/bridges_test.go` imports to include `context`:

```go
import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/loginpb"
)
```

(If `loginpb` is not currently imported in `bridges_test.go`, add it; the new tests reference `loginpb.PlayerBanRequest` via the gated fake's method signature.)

- [ ] **Step 2.2: Run tests to verify they fail (compile error)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestLoginGRPCBridgeMod_' -v
```

Expected: compile error `undefined: loginGRPCBridgeMod`.

### Step 2.3: Add `loginGRPCBridgeMod` type to `bridges.go`

- [ ] **Step 2.3: Edit `modules/world/bridges.go`**

Replace the `LoginBridgeMod` doc-comment block (lines 23-26) to retire the deviation reference. Change from:

```go
// LoginBridgeMod mirrors TS World.loginThread.postMessage('player_ban'/
// 'player_mute', ...). The existing LoginClient is auth-only; this is a
// separate moderation channel. Real impl deferred via
// NAI-72-D-LOGIN-SERVER-BRIDGE-MOD.
type LoginBridgeMod interface {
```

to:

```go
// LoginBridgeMod mirrors TS World.loginThread.postMessage('player_ban'/
// 'player_mute', ...). Production impl is loginGRPCBridgeMod (below),
// which delegates to LoginClient.PlayerBan / .PlayerMute.
type LoginBridgeMod interface {
```

Then add the new type and helper at the end of the file (after the last `noopBridges` method):

```go

// loginGRPCBridgeMod is the production LoginBridgeMod impl. Translates
// moderation actions into gRPC RPCs against the login server. Calls are
// fired in a goroutine so packet handlers and the tick loop never block
// on network I/O — mirrors the goroutine fan-out used by autosave
// (server.go:1018) and force-logout (server.go:982). Callers must
// compute the absolute deadline (time.Now().Add(d)) before invocation;
// the bridge does not coerce zero times.
type loginGRPCBridgeMod struct {
	client LoginClient
	log    *slog.Logger
}

func (b *loginGRPCBridgeMod) NotifyPlayerBan(staff, username string, until time.Time) {
	go b.client.PlayerBan(context.Background(), &loginpb.PlayerBanRequest{
		Staff:    staff,
		Username: username,
		Until:    timestamppb.New(until),
	})
}

func (b *loginGRPCBridgeMod) NotifyPlayerMute(staff, username string, until time.Time) {
	go b.client.PlayerMute(context.Background(), &loginpb.PlayerMuteRequest{
		Staff:    staff,
		Username: username,
		Until:    timestamppb.New(until),
	})
}

var _ LoginBridgeMod = (*loginGRPCBridgeMod)(nil)
```

Update imports at the top of `bridges.go` from:

```go
import "time"
```

to:

```go
import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zsrv/goscape/pkg/loginpb"
)
```

### Step 2.4: Run tests to verify pass

- [ ] **Step 2.4: Run the three new bridge tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestLoginGRPCBridgeMod_' -v
```

Expected: 3 PASS (`_NotifyPlayerBan_FiresRPC`, `_NotifyPlayerMute_FiresRPC`, `_FireAndForget_DoesNotBlock`).

### Step 2.5: Commit

- [ ] **Step 2.5: Commit**

```bash
git add modules/world/bridges.go modules/world/bridges_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: add loginGRPCBridgeMod adapter

Routes LoginBridgeMod.NotifyPlayer{Ban,Mute} calls to the login server
via LoginClient.PlayerBan/PlayerMute. Goroutine fan-out matches the
existing fire-and-forget pattern at server.go:982,1018 so synchronous
handler paths never block on network I/O.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Add `defaultLoginBridgeMod` helper

**Why this task exists:** `NewServer` opens a TCP listener and loads the full content cache (locTypes, params, objTypes, gamemap, invTypes, dbTable/Row, varp/vars/varn — `server.go:236-330`). Testing the "default is grpc bridge when loginClient != nil" decision against that full setup is heavyweight and brittle. Extracting the decision into a one-line helper makes it unit-testable in two lines. The helper is the single source of truth for "what `LoginBridgeMod` does a production `Server` get?"

**Files:**
- Modify: `modules/world/bridges.go` (add helper)
- Modify: `modules/world/bridges_test.go` (add 2 tests)

### Step 3.1: Write failing tests for `defaultLoginBridgeMod`

- [ ] **Step 3.1: Append to `modules/world/bridges_test.go`**

```go
func TestDefaultLoginBridgeMod_NonNilClient_ReturnsGRPCBridge(t *testing.T) {
	got := defaultLoginBridgeMod(newFakeLoginClient(), discardLogger())
	if _, ok := got.(*loginGRPCBridgeMod); !ok {
		t.Fatalf("defaultLoginBridgeMod: got %T, want *loginGRPCBridgeMod", got)
	}
}

func TestDefaultLoginBridgeMod_NilClient_ReturnsNoop(t *testing.T) {
	got := defaultLoginBridgeMod(nil, discardLogger())
	if _, ok := got.(noopBridges); !ok {
		t.Fatalf("defaultLoginBridgeMod: got %T, want noopBridges", got)
	}
}
```

- [ ] **Step 3.2: Run tests to verify they fail (compile error)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestDefaultLoginBridgeMod_' -v
```

Expected: compile error `undefined: defaultLoginBridgeMod`.

### Step 3.3: Add the helper to `bridges.go`

- [ ] **Step 3.3: Append to `modules/world/bridges.go` after the `loginGRPCBridgeMod` block**

```go

// defaultLoginBridgeMod returns the production LoginBridgeMod for the
// given LoginClient: a goroutine-fanout gRPC adapter when client != nil,
// otherwise noopBridges{}. Called from NewServer; broken out for
// testability without spinning up the full Server.
func defaultLoginBridgeMod(client LoginClient, log *slog.Logger) LoginBridgeMod {
	if client != nil {
		return &loginGRPCBridgeMod{client: client, log: log}
	}
	return noopBridges{}
}
```

### Step 3.4: Run tests to verify pass

- [ ] **Step 3.4: Run the two new helper tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestDefaultLoginBridgeMod_' -v
```

Expected: 2 PASS.

### Step 3.5: Commit

- [ ] **Step 3.5: Commit**

```bash
git add modules/world/bridges.go modules/world/bridges_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: add defaultLoginBridgeMod helper

Encapsulates the nil-or-grpc default decision so NewServer's
loginBridgeMod default can be unit-tested without spinning up a
full Server (TCP listener + full cache load).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Wire `NewServer` to use the helper

**Why this task exists:** This is the production default flip. After this commit, any `Server` constructed via `NewServer(cfg, loginClient, log)` with a non-nil `loginClient` routes `NotifyPlayerBan`/`Mute` calls to the login server instead of dropping them. Test-only `newTestServer` and tests that call `installRecordingBridges` are unaffected because they construct/override directly.

**Files:**
- Modify: `modules/world/server.go` (line 272)

### Step 4.1: Edit `server.go` to call the helper

- [ ] **Step 4.1: Replace line 272 of `modules/world/server.go`**

Change:

```go
	s.friendsBridge = noopBridges{}
	s.loginBridgeMod = noopBridges{}
	s.loggerBridge = NewSlogLoggerBridge(s.log)
```

to:

```go
	s.friendsBridge = noopBridges{}
	s.loginBridgeMod = defaultLoginBridgeMod(loginClient, s.log)
	s.loggerBridge = NewSlogLoggerBridge(s.log)
```

No new imports needed (`bridges.go` is the same package).

### Step 4.2: Run the full `modules/world` test suite

- [ ] **Step 4.2: Run module tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
```

Expected: PASS. The four sites that override `s.loginBridgeMod` post-construction (`newTestServer` at `server_test.go:300+`, `interaction_test.go:1872,2727`, `npc_interaction_test.go:1832,2093`, plus the four `installRecordingBridges` callers) all assign explicitly, so the production default flip doesn't reach them.

### Step 4.3: Run with race detector

- [ ] **Step 4.3: Run module tests under `-race`**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...
```

Expected: PASS, no DATA RACE warnings. The new concurrency surfaces are the `go` fan-out in `loginGRPCBridgeMod.NotifyPlayer*` and the `mu`-protected `mockLoginPBClient` capture; both are correct by inspection.

### Step 4.4: Commit

- [ ] **Step 4.4: Commit**

```bash
git add modules/world/server.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: default loginBridgeMod from non-nil LoginClient

NewServer now wires loginBridgeMod via defaultLoginBridgeMod, so any
production server constructed with a real LoginClient routes
NotifyPlayer{Ban,Mute} to the login server via gRPC. Standalone-world
mode (--login.address unset → loginClient == nil) retains noopBridges.

Retires NAI-72-D-LOGIN-SERVER-BRIDGE-MOD.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Verification and close

**Why this task exists:** Final defensive gates — full `-race` clean across the repo, smoke-pack baseline holds, no stray references to the retired deviation tag.

**Files:** None modified (verification only).

### Step 5.1: Full repo race-detector run

- [ ] **Step 5.1: Run all tests with race detector**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
```

Expected: PASS across all packages, no DATA RACE warnings. Slice does not touch any package outside `modules/world`, so other packages should be unaffected, but the gate is cheap.

### Step 5.2: Smoke-pack baseline

- [ ] **Step 5.2: Run smoke-pack 12 OK / 0 ERR baseline**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape-cli smoke-pack --content-dir $HOME/Code/github.com/LostCityRS/content
```

Expected: `12 OK / 0 ERR / 0 SKIP` (matches the standing baseline established in `[[smoke_pack_phase1_close]]`). This slice does not touch any packing surface; the gate is defensive.

If the run fails to launch (e.g., `--reference-dir` is required or `smoke-pack` expects more flags), check `cmd/goscape-cli/main.go` and `[[smoke_pack_phase1_close]]` / `[[smoke_pack_phase2_close]]` for the exact flag set — exit if smoke-pack tooling has drifted, do not invent flags.

### Step 5.3: Verify retired tag has zero remaining references

- [ ] **Step 5.3: Grep for the retired deviation tag**

```bash
grep -rn "NAI-72-D-LOGIN-SERVER-BRIDGE-MOD" --include="*.go" --include="*.md" .
```

Expected: only matches in `MEMORY.md` and historical spec/plan/close-memory files. Zero matches in `modules/world/*.go` (the doc-comment retirement in Task 2 was correct).

If a code-side match remains, find and edit it out (it's a missed reference; should not exist after Task 2 Step 2.3).

### Step 5.4: Write close memory

- [ ] **Step 5.4: Write the close memory**

Create `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai214_login_moderation_bridge_close.md`:

```markdown
---
name: nai214-login-moderation-bridge-close
description: NAI-214 LoginBridgeMod gRPC wire-up shipped 2026-05-18; retires NAI-72-D-LOGIN-SERVER-BRIDGE-MOD
metadata:
  type: project
---

NAI-214 wired `LoginBridgeMod.NotifyPlayer{Ban,Mute}` to the existing `modules/login` `PlayerBan`/`PlayerMute` gRPC RPCs. Shipped 2026-05-18 across N commits <sha-range>.

**Why:** the login-server side (`proto/login/login.proto` + `modules/login/handler.go`) already implemented PlayerBan/PlayerMute (with SQLite persistence); only the world-side wire-up was missing. Five call sites (`handler_reportabuse.go:50,60`, `handler_message_private.go:42`, `handlers_game.go:1187,1206`) were silently no-opping via `noopBridges{}` until this slice.

**How to apply:** Future moderation channels (e.g., FRIENDS-SERVER-BRIDGE) should follow the same two-seam shape: extend `LoginClient`/`FriendsClient` interface with fire-and-forget RPC methods, add a `*grpcBridge` adapter wrapping the client, flip the `NewServer` default via a `default*Bridge(client, log)` helper that's unit-testable without the full server bring-up.

**Surfaces touched:** `modules/world/login_client.go` (interface + grpc impl), `modules/world/login_client_fake_test.go` (cap-16 channel capture), `modules/world/bridges.go` (adapter + helper), `modules/world/bridges_test.go` (5 tests), `modules/world/server.go` (one-line default flip), `modules/world/login_client_test.go` (4 grpc-impl tests).

**Tag retired:** `NAI-72-D-LOGIN-SERVER-BRIDGE-MOD` removed from `bridges.go:23-26`.

**Deviation tags introduced:** None. TS posts moderation via `loginThread.postMessage` (worker-thread MessagePort); goscape uses gRPC throughout — same architectural substitution that predates this slice.

**Open follow-ups:** None from this slice. The companion `NAI-72-D-FRIENDS-SERVER-BRIDGE` is still open and is a distinct slice (separate friends-server dskit module).

**Gates held:** smoke-pack 12 OK / 0 ERR; `-race` clean.
```

Fill in `<sha-range>` from `git log --oneline` showing the four task commits.

### Step 5.5: Add memory index entry

- [ ] **Step 5.5: Add a one-line entry to `MEMORY.md`**

Append to `$HOME/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md`:

```markdown
- [NAI-214 login moderation bridge close](nai214_login_moderation_bridge_close.md) — LoginBridgeMod.NotifyPlayer{Ban,Mute} → modules/login PlayerBan/Mute gRPC shipped 2026-05-18; new loginGRPCBridgeMod adapter + defaultLoginBridgeMod helper; retires NAI-72-D-LOGIN-SERVER-BRIDGE-MOD; 5 call sites un-noop'd; 9 new tests; -race clean; smoke-pack 12 OK / 0 ERR holds
```

### Step 5.6: Final commit (close memory + MEMORY.md entry)

- [ ] **Step 5.6: Commit memory updates**

Note that the memory file is outside the repo (`$HOME/.claude/projects/...`), so this commit does NOT include the close memory — it's persisted by the harness via the Write tool calls in 5.4 and 5.5. No repo-side commit is needed for Task 5; the closing acts are tests + memory writes.

If the verification steps surfaced any issue that required a repo-side change (e.g., a missed reference to the retired tag), commit that fix separately with `git commit --no-gpg-sign -m "world: <one-line fix description>"` before declaring the slice closed.

---

## Self-review

### Spec coverage check

- §1 In scope items:
  - `LoginClient` interface extension → Task 1 ✓
  - `grpcLoginClient` impl → Task 1 ✓
  - `fakeLoginClient` extension → Task 1 ✓
  - `loginGRPCBridgeMod` adapter → Task 2 ✓
  - `defaultLoginBridgeMod` helper → Task 3 ✓
  - `NewServer` default flip → Task 4 ✓
  - Retire `NAI-72-D-LOGIN-SERVER-BRIDGE-MOD` tag → Task 2 (doc-comment) + Task 5 (verify) ✓
  - ~6 new tests → Task 1 (4) + Task 2 (3) + Task 3 (2) = 9 tests ✓

- §1 Out of scope items: none touched (audit log, retries, in-process kick, FriendsBridge impl, LoggerBridge changes, response parsing, CLI flag, login-server modifications, deviation tag introduction) — all correctly absent from the plan.

- §4 Test plan items:
  - §4.1 `_PassesRequest` + `_LogsErrorOnFailure` × 2 → Task 1 Step 1.1 (4 tests) ✓
  - §4.2 bridge fan-out tests × 3 → Task 2 Step 2.1 ✓
  - §4.3 fake extension → Task 1 Step 1.5 ✓
  - §4.4 helper tests × 2 → Task 3 Step 3.1 ✓
  - §4.5 call-site smoke (no new tests, existing tests pass) → Task 4 Step 4.2 ✓
  - §4.6 race detector → Task 5 Step 5.1 ✓

- §5 Implementation order → 4 production commits + 1 verification step ✓

- §7 Exit criteria → all gates listed in Task 5 ✓

### Placeholder scan

No "TBD"/"TODO"/"fill in later"/"appropriate error handling" patterns in the plan. Step 5.4's close memory has a `<sha-range>` placeholder that is explicitly filled from `git log` output at execution time — this is concrete data not yet known at plan-write time, not a vague placeholder.

### Type consistency

- `mockLoginPBClient` field names: `gotPlayerBanReq`, `gotPlayerMuteReq` — consistent throughout Task 1.
- `gatedLoginClient` fields: `gate chan struct{}`, `hit chan struct{}` — consistent throughout Task 2.
- `loginGRPCBridgeMod` field names: `client`, `log` — consistent in spec §3.3, Task 2 Step 2.3, Task 3 Step 3.3, Task 4 Step 4.1.
- `defaultLoginBridgeMod(client LoginClient, log *slog.Logger) LoginBridgeMod` — same signature in spec §3.4, Task 3 Step 3.3, Task 4 Step 4.1.
- `fakeLoginClient` capture channels: `playerBanReqs`, `playerMuteReqs` — consistent in Task 1 Step 1.5 and Task 2 Step 2.1.
- All `LoginClient` method signatures match `loginpb` generated types (`PlayerBanRequest`/`PlayerMuteRequest` / `PlayerBanResponse`/`PlayerMuteResponse` per `pkg/loginpb/login_grpc.pb.go:41-42,103-117`).
