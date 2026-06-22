# Friends-server bridge slice 4c — world acts on PlayerLoginResponse.Accepted

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface `PlayerLoginResponse.Accepted` from the friends-server to the world. Change `FriendsClient.PlayerLogin` to accept a callback; wire the call site in `processLogins` to log a warn on cap-rejection. Retires `NAI-S1-D-PLAYERCAP-LOG-ONLY` (server doc-only) and `NAI-S2-D-PLAYERLOGIN-IGNORES-ACCEPTED` (world code change).

**Architecture:** TS-faithful posture (Option A): log warn on `Accepted=false`, do not interrupt the world's login flow, keep the subscriber start unconditional. Callback shape (Option B): preserves the bridge's fire-and-forget convention; callback runs in the goroutine that issued the RPC.

**Tech Stack:** Go 1.x, gRPC, `pkg/friendspb` proto, `log/slog`, existing `modules/world/friends_smoke_test.go` in-process e2e fixture, existing `mockFriendsPBClient` test scaffold.

**Reference spec:** `docs/superpowers/specs/2026-05-22-friends-server-bridge-slice4c-design.md`

---

## Task 1: Atomic signature change — interface, production impl, fake, call site

**Why first:** The signature change ripples through interface + production + fake + call site. Splitting across tasks leaves the package half-compiled. After this task the package builds, existing tests pass (the fake defaults to `accepted=true`, preserving prior assertions), and the new behavior is in place. Retires `NAI-S2-D-PLAYERLOGIN-IGNORES-ACCEPTED`.

**Files:**
- Modify: `modules/world/friends_client.go`
- Modify: `modules/world/friends_client_fake_test.go`
- Modify: `modules/world/friends_client_test.go` (update one test case)
- Modify: `modules/world/tick.go`

- [ ] **Step 1: Update the `FriendsClient` interface**

In `modules/world/friends_client.go`, replace the interface's `PlayerLogin` line (currently line 25):

```go
	PlayerLogin(ctx context.Context, req *friendspb.PlayerLoginRequest)
```

with:

```go
	// PlayerLogin registers the player on the friends server. onResponse is
	// invoked once after the RPC completes: accepted=true on success,
	// accepted=false on cap-reached or RPC error. May be nil.
	PlayerLogin(ctx context.Context, req *friendspb.PlayerLoginRequest, onResponse func(accepted bool))
```

Also update the file-level FriendsClient doc-comment at lines 14-22 — the line `All RPCs except Close and SubscribeUpdates are fire-and-forget` is still accurate (PlayerLogin remains fire-and-forget; the callback is an out-of-band signal, not a synchronous return). No change required to that paragraph.

- [ ] **Step 2: Update `grpcFriendsClient.PlayerLogin` impl**

In `modules/world/friends_client.go`, replace the existing PlayerLogin method (currently lines 84-93):

```go
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
```

with:

```go
// PlayerLogin registers the player on the friends server. onResponse is
// invoked once after the RPC completes: accepted=true on success,
// accepted=false on cap-rejection or RPC error. May be nil. Errors are
// logged warn + swallowed before the callback fires (matches the
// fire-and-forget posture of every other void RPC on this client).
func (c *grpcFriendsClient) PlayerLogin(ctx context.Context, req *friendspb.PlayerLoginRequest, onResponse func(accepted bool)) {
	resp, err := c.client.PlayerLogin(ctx, req)
	if err != nil {
		c.log.Warn("PlayerLogin RPC failed",
			slog.Uint64("username37", req.Username37),
			slog.Any("err", err),
		)
		if onResponse != nil {
			onResponse(false)
		}
		return
	}
	if onResponse != nil {
		onResponse(resp.GetAccepted())
	}
}
```

- [ ] **Step 3: Update `fakeFriendsClient` to match**

In `modules/world/friends_client_fake_test.go`:

(a) Add a new field to the struct (after the existing `subscribeErr` field, line 34):

```go
	// playerLoginAccepted is the value passed to PlayerLogin's onResponse
	// callback. Defaults to true; set false to simulate cap-rejection.
	// Read under mu.
	playerLoginAccepted bool
```

(b) In `newFakeFriendsClient` (line 46-57), initialize the new field:

```go
func newFakeFriendsClient() *fakeFriendsClient {
	return &fakeFriendsClient{
		playerLoginReqs:     make(chan *friendspb.PlayerLoginRequest, 16),
		playerLogoutReqs:    make(chan *friendspb.PlayerLogoutRequest, 16),
		chatSetModeReqs:     make(chan *friendspb.ChatSetModeRequest, 16),
		friendlistAddReqs:   make(chan *friendspb.FriendlistAddRequest, 16),
		friendlistDelReqs:   make(chan *friendspb.FriendlistDelRequest, 16),
		ignorelistAddReqs:   make(chan *friendspb.IgnorelistAddRequest, 16),
		ignorelistDelReqs:   make(chan *friendspb.IgnorelistDelRequest, 16),
		privateMessageReqs:  make(chan *friendspb.PrivateMessageRequest, 16),
		playerLoginAccepted: true,
	}
}
```

(c) Replace the `PlayerLogin` method (currently lines 68-73):

```go
func (f *fakeFriendsClient) PlayerLogin(ctx context.Context, req *friendspb.PlayerLoginRequest, onResponse func(accepted bool)) {
	select {
	case f.playerLoginReqs <- req:
	default:
	}
	f.mu.Lock()
	accepted := f.playerLoginAccepted
	f.mu.Unlock()
	if onResponse != nil {
		onResponse(accepted)
	}
}
```

- [ ] **Step 4: Update the existing `TestGRPCFriendsClient_LogsErrorOnFailure` PlayerLogin entry**

In `modules/world/friends_client_test.go`, the `PlayerLogin` entry in the table-driven test (currently line 279-281):

```go
		{"PlayerLogin", func(c *grpcFriendsClient) {
			c.PlayerLogin(context.Background(), &friendspb.PlayerLoginRequest{Username37: 1})
		}},
```

becomes:

```go
		{"PlayerLogin", func(c *grpcFriendsClient) {
			c.PlayerLogin(context.Background(), &friendspb.PlayerLoginRequest{Username37: 1}, nil)
		}},
```

The test's contract (asserts log-on-error swallow) is preserved; passing `nil` for the callback is the documented "may be nil" path. Task 2 adds focused callback assertions.

- [ ] **Step 5: Update the call site in `processLogins`**

In `modules/world/tick.go`, replace lines 166-177 (the entire `if s.friendsClient != nil && p.username != ""` PlayerLogin block):

```go
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
```

with:

```go
			// NAI-S2-D-PLAYERLOGIN-AT-PROCESSLOGINS: register the player on
			// the friends server when they enter the world. Mirrors TS's
			// PLAYER_LOGIN-on-world-entry semantics. On cap-rejection the
			// world logs warn and continues — TS-faithful: the friends-
			// server is silent toward the world on rejection (TS
			// FriendServer.ts:128-132 early-returns without notifying).
			if s.friendsClient != nil && p.username != "" {
				username37 := p.username37
				worldID := int32(s.cfg.NodeID)
				go s.friendsClient.PlayerLogin(context.Background(), &friendspb.PlayerLoginRequest{
					WorldId:     worldID,
					Username37:  username37,
					PrivateChat: int32(p.privateChat),
					StaffLvl:    p.staffModLevel,
				}, func(accepted bool) {
					if !accepted {
						s.log.Warn("friends-server rejected player login (cap reached or RPC error)",
							slog.Int("world_id", int(worldID)),
							slog.Uint64("username37", username37),
						)
					}
				})
			}
```

If `slog` is not yet imported in `tick.go`, add it. (Check the existing imports at the top of `tick.go`.)

- [ ] **Step 6: Build and run the existing test suite for the affected packages**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... ./modules/friends/...
```

Expected: PASS. The `playerLoginAccepted=true` default keeps existing test assertions unchanged. The `TestGRPCFriendsClient_LogsErrorOnFailure/PlayerLogin` case passes because the mock returns an error and the production impl logs warn before invoking the (nil) callback.

- [ ] **Step 7: Commit**

```bash
git add modules/world/friends_client.go modules/world/friends_client_fake_test.go modules/world/friends_client_test.go modules/world/tick.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: surface friends-server PlayerLoginResponse.Accepted via callback (T1)

FriendsClient.PlayerLogin gains a func(accepted bool) callback parameter.
grpcFriendsClient invokes it with resp.Accepted on success and false on
RPC error (uniform "rejected-or-error" signal). fakeFriendsClient gains
a playerLoginAccepted field (defaults to true). The processLogins call
site passes a callback that logs warn on rejection — TS-faithful posture
per FriendServer.ts:128-132. Subscriber start stays unconditional.

Retires NAI-S2-D-PLAYERLOGIN-IGNORES-ACCEPTED on both annotated sites
(friends_client.go:85 doc-comment and tick.go:168-169 inline). Server-
side NAI-S1-D-PLAYERCAP-LOG-ONLY doc-comment retirement deferred to T5.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Pin grpcFriendsClient callback contract

**Why now:** T1 added the callback wiring. Add focused unit tests that pin the contract (accepted-true → callback(true); accepted-false → callback(false); RPC error → callback(false)). Pure pin; no production change.

**Files:**
- Modify: `modules/world/friends_client_test.go`

- [ ] **Step 1: Add the pin test**

Append to `modules/world/friends_client_test.go` (after `TestGRPCFriendsClient_LogsErrorOnFailure`):

```go
// TestGRPCFriendsClient_PlayerLogin_InvokesCallback pins slice 4c's
// callback contract: onResponse fires with accepted=true on
// PlayerLoginResponse{Accepted: true}, accepted=false on Accepted: false,
// and accepted=false on RPC error. Replaces the slice-2 posture that
// discarded the response (NAI-S2-D-PLAYERLOGIN-IGNORES-ACCEPTED, retired
// by slice 4c).
func TestGRPCFriendsClient_PlayerLogin_InvokesCallback(t *testing.T) {
	cases := []struct {
		name     string
		resp     *friendspb.PlayerLoginResponse
		err      error
		wantAcc  bool
	}{
		{"AcceptedTrue", &friendspb.PlayerLoginResponse{Accepted: true}, nil, true},
		{"AcceptedFalse", &friendspb.PlayerLoginResponse{Accepted: false}, nil, false},
		{"RPCError", nil, errors.New("simulated"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockFriendsPBClient{
				playerLoginFn: func(ctx context.Context, in *friendspb.PlayerLoginRequest, opts ...grpc.CallOption) (*friendspb.PlayerLoginResponse, error) {
					return tc.resp, tc.err
				},
			}
			c := &grpcFriendsClient{client: mock, log: discardLogger()}
			ch := make(chan bool, 1)
			c.PlayerLogin(context.Background(), &friendspb.PlayerLoginRequest{Username37: 1}, func(accepted bool) {
				ch <- accepted
			})
			select {
			case got := <-ch:
				if got != tc.wantAcc {
					t.Errorf("callback accepted: got %v, want %v", got, tc.wantAcc)
				}
			case <-time.After(time.Second):
				t.Fatal("timeout waiting for callback")
			}
		})
	}
}
```

The required imports (`errors`, `time`) are likely already present from the existing `TestGRPCFriendsClient_LogsErrorOnFailure` test in the same file. Verify and add if missing.

- [ ] **Step 2: Run the new test**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run TestGRPCFriendsClient_PlayerLogin_InvokesCallback ./modules/world/
```

Expected: PASS for all three sub-cases.

- [ ] **Step 3: Commit**

```bash
git add modules/world/friends_client_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: pin grpcFriendsClient.PlayerLogin callback contract (T2)

Three sub-cases pin the slice-4c contract: accepted=true on
PlayerLoginResponse{Accepted: true}, accepted=false on Accepted: false,
and accepted=false on RPC error. Uses the existing mockFriendsPBClient
table-driven scaffold.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Pin processLogins warn-log on cap-rejection

**Why now:** T1 wired the call site's callback to log warn on rejection. Add an integration test in `tick_friends_login_test.go` that captures slog output and asserts the warn record fires when `playerLoginAccepted=false`. Pure pin.

**Files:**
- Modify: `modules/world/tick_friends_login_test.go`

- [ ] **Step 1: Add the pin test**

Append to `modules/world/tick_friends_login_test.go`:

```go
// TestProcessLogins_FriendsPlayerLogin_LogsWarnOnRejection pins slice
// 4c's TS-faithful posture: when the friends-server returns
// Accepted=false (cap reached), processLogins logs a warn on the world
// side but does not interrupt the login flow. Mirrors TS
// FriendServer.ts:128-132 (server is silent toward the world; the world
// observes via its own logging only).
func TestProcessLogins_FriendsPlayerLogin_LogsWarnOnRejection(t *testing.T) {
	s := newTestServer(t)
	s.cfg.NodeID = 10
	s.cfg.NodeProfile = "main"

	var buf bytes.Buffer
	s.log = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	fake := newFakeFriendsClient()
	fake.playerLoginAccepted = false
	s.friendsClient = fake

	c, conn := newTestClient(t)
	c.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	go io.Copy(io.Discard, conn)
	p := newPlayer(c)
	p.username = "alice"
	p.username37 = 1234
	s.appendNewPlayer(p)

	s.processLogins()

	// Drain the captured RPC.
	select {
	case <-fake.playerLoginReqs:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for PlayerLogin RPC")
	}

	// Poll the log buffer for the expected warn — the callback fires from
	// the RPC goroutine, so the write to buf is not synchronous with the
	// channel send above.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), "friends-server rejected player login") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	got := buf.String()
	if !strings.Contains(got, "friends-server rejected player login") {
		t.Errorf("expected warn log containing 'friends-server rejected player login'; got %q", got)
	}
	if !strings.Contains(got, "username37=1234") {
		t.Errorf("expected log to include username37=1234; got %q", got)
	}
	if !strings.Contains(got, "world_id=10") {
		t.Errorf("expected log to include world_id=10; got %q", got)
	}
}
```

Required imports to add at the top of `tick_friends_login_test.go` (the existing imports are `io`, `testing`, `time`, and `io2` aliased to `pkg/io/isaac`):

```go
import (
	"bytes"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
)
```

- [ ] **Step 2: Run the new test**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run TestProcessLogins_FriendsPlayerLogin_LogsWarnOnRejection ./modules/world/
```

Expected: PASS.

- [ ] **Step 3: Run the full `tick_friends_login_test.go` group to confirm no regression**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run TestProcessLogins ./modules/world/
```

Expected: PASS (existing tests still pass — the fake's `playerLoginAccepted` default of `true` preserves their assertions).

- [ ] **Step 4: Commit**

```bash
git add modules/world/tick_friends_login_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: pin processLogins warn-log on friends-server cap-rejection (T3)

Captures slog output via a TextHandler bound to a bytes.Buffer; sets
fake.playerLoginAccepted=false to trigger the callback's warn path.
Polls the buffer because the callback fires on the RPC goroutine, not
synchronously with the channel send.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: e2e cap-rejection smoke test

**Why now:** Pin slice 4c against a real friends-server. Mirrors the slice-4a/4b e2e smokes. Verifies the full path: real gRPC over loopback + real handler + real repo + real cap enforcement.

**Files:**
- Modify: `modules/world/friends_smoke_test.go`

- [ ] **Step 1: Add the e2e test**

Append to `modules/world/friends_smoke_test.go`:

```go
// TestFriendsClient_E2E_PlayerLoginCapRejected boots a real friends
// service with WorldPlayerLimit=1, logs in two players on the same
// world, and asserts the second player's callback fires with
// accepted=false. Pins slice 4c end-to-end: proto compat + handler
// cap-enforcement + grpcFriendsClient callback wiring.
func TestFriendsClient_E2E_PlayerLoginCapRejected(t *testing.T) {
	port := freePort(t)
	cfg := friends.Config{
		GRPCListenAddress:       "127.0.0.1",
		GRPCListenPort:          port,
		NodeProfile:             "main",
		WorldPlayerLimit:        1,
		Enable:                  true,
		GracefulShutdownTimeout: 5 * time.Second,
		SQLiteDSN:               filepath.Join(t.TempDir(), "friends.db"),
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

	client.WorldConnect(ctx, 10, "main")

	// First login fills the world (cap=1).
	ch1 := make(chan bool, 1)
	client.PlayerLogin(ctx, &friendspb.PlayerLoginRequest{
		WorldId:    10,
		Username37: 1001,
	}, func(accepted bool) { ch1 <- accepted })
	select {
	case acc := <-ch1:
		if !acc {
			t.Fatalf("first PlayerLogin: expected accepted=true, got false")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for first PlayerLogin callback")
	}

	// Second login exceeds the cap → server returns Accepted=false.
	ch2 := make(chan bool, 1)
	client.PlayerLogin(ctx, &friendspb.PlayerLoginRequest{
		WorldId:    10,
		Username37: 1002,
	}, func(accepted bool) { ch2 <- accepted })
	select {
	case acc := <-ch2:
		if acc {
			t.Fatalf("second PlayerLogin: expected accepted=false, got true")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for second PlayerLogin callback")
	}
}
```

The existing imports at the top of `friends_smoke_test.go` already include `context`, `filepath`, `strconv`, `testing`, `time`, `friends`, and `friendspb`. No new imports required.

- [ ] **Step 2: Update the existing e2e smoke tests' PlayerLogin call sites**

`TestFriendsClient_E2E_SmokeAgainstFriendsServer` and `TestFriendsClient_E2E_SubscribeUpdatesStream` already call `client.PlayerLogin(ctx, ...)`. After T1's signature change, these call sites need the new third argument. Pass `nil` for the callback (these tests don't assert on the response):

Grep to find every call:

```bash
grep -n "client.PlayerLogin" modules/world/friends_smoke_test.go
```

For each match, append `, nil` before the closing `)`. (If T1 already fixed these in Step 5's `go build ./...` pass, this step is a no-op — verify and skip.)

- [ ] **Step 3: Run the new e2e and the full smoke-test file**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run TestFriendsClient_E2E ./modules/world/
```

Expected: PASS for all e2e tests including the new one.

- [ ] **Step 4: Commit**

```bash
git add modules/world/friends_smoke_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: e2e smoke for slice-4c PlayerLogin cap-rejection (T4)

Boots a real friends service with WorldPlayerLimit=1, drives two
PlayerLogin RPCs through grpcFriendsClient, and asserts the second
callback fires with accepted=false. Mirrors the slice-4a/4b e2e
pattern (TestFriendsClient_E2E_SubscribeUpdatesStream /
PrivateMessageDelivery).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Doc-only retirement of `NAI-S1-D-PLAYERCAP-LOG-ONLY`

**Why last:** Mechanical doc-comment edit on the server side. Defer until the world-side surfacing (T1) is committed so the comment retirement is causally correct.

**Files:**
- Modify: `modules/friends/handler.go`

- [ ] **Step 1: Update the server-side `PlayerLogin` doc-comment**

In `modules/friends/handler.go`, replace the doc-comment block on `PlayerLogin` (currently lines 55-60):

```go
// PlayerLogin registers the player on the given world. Always returns OK;
// PlayerLoginResponse.Accepted is false iff the world's player cap is
// reached.
//
// NAI-S1-D-PLAYERCAP-LOG-ONLY — cap rejection logs warn but does not error.
// Slice 4c surfaces Accepted to callers.
```

with:

```go
// PlayerLogin registers the player on the given world. Always returns OK;
// PlayerLoginResponse.Accepted is false iff the world's player cap is
// reached. The world acts on the rejection in
// modules/world/tick.go's processLogins callback (slice 4c).
```

- [ ] **Step 2: Verify the tag isn't referenced elsewhere**

```bash
grep -rn "NAI-S1-D-PLAYERCAP-LOG-ONLY" --include="*.go" --include="*.md" .
```

Expected: zero `.go` matches; only references in `docs/superpowers/specs/2026-05-18-friends-server-bridge-slice1-design.md` (historical), `docs/superpowers/specs/2026-05-22-friends-server-bridge-slice4c-design.md` (this slice's spec), and `.claude/resume/2026-05-22-friends-bridge-slice4c-resume.md`. If a stray `.go` reference exists, retire it too.

Similarly grep for `NAI-S2-D-PLAYERLOGIN-IGNORES-ACCEPTED` to confirm T1 caught both sites:

```bash
grep -rn "NAI-S2-D-PLAYERLOGIN-IGNORES-ACCEPTED" --include="*.go" .
```

Expected: zero `.go` matches.

- [ ] **Step 3: Run the friends handler tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/
```

Expected: PASS (doc-only edit; no behavior change).

- [ ] **Step 4: Commit**

```bash
git add modules/friends/handler.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: retire NAI-S1-D-PLAYERCAP-LOG-ONLY (T5)

The server already returns PlayerLoginResponse{Accepted: false} on
cap-reached; slice 4c's T1 wired the world-side callback to log warn on
rejection, satisfying the deferral. Doc-comment now references the
world-side action point instead of carrying the deviation tag.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Whole-slice gate

After T5 commits, run the full project gate:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... -count=1 -timeout 600s
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape-cli smoke-pack --content-dir $HOME/Code/github.com/LostCityRS/content
```

Expected:
- `go test -race ./...`: all packages PASS.
- `smoke-pack`: `12 OK / 0 ERR / 0 SKIP` (regression check; slice 4c should not touch any packer).

If the whole-slice gate flags a consistency drift (per slice-4a's missing compile-time assertion finding), address it in a follow-up commit before closing the slice.

---

## Summary

| Task | Files | Purpose |
|---|---|---|
| T1 | `friends_client.go`, `friends_client_fake_test.go`, `friends_client_test.go`, `tick.go` | Atomic signature change + impl + call site; retires `NAI-S2-D-PLAYERLOGIN-IGNORES-ACCEPTED` |
| T2 | `friends_client_test.go` | Pin grpcFriendsClient callback contract |
| T3 | `tick_friends_login_test.go` | Pin processLogins warn-log on rejection |
| T4 | `friends_smoke_test.go` | e2e cap-rejection smoke |
| T5 | `modules/friends/handler.go` | Retire `NAI-S1-D-PLAYERCAP-LOG-ONLY` (doc-only) |

Net LOC: ~130 added, ~30 deleted across 6 files. After T5 closes, slice 4 cluster (4a + 4b + 4c) is fully closed; remaining friends-server arc work is slices 5/6/7.
