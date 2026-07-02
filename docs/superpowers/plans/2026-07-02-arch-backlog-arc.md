# Architecture Backlog Arc (Arc 29 fix IDs, PORTING entry "Arc 31") Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the remaining findings from the 2026-07-02 architecture review: the OnDemand third-writer race residual, all MED operational gaps (keepalives, WorldStartup retry, friends shutdown hang, config verification, health endpoint, lifecycle hygiene), and the LOW debt (packet API footguns, dead code, version stamping, Makefile, test gaps).

**Architecture:** Each fix is an independent branch off `rev-274` (`fix/arch29-<slug>`), merged back after its RED→GREEN cycle — same cadence as Arc 28. Task order matters in two places: Task 7 (API footguns) changes `filestream.New` and `cache.MakeCRCs` signatures that Task 8 (lifecycle hygiene) consumes. All fixes are rev-274-only for now; tasks tagged **[BACKPORT-FIDELITY]** are candidates for a later fidelity backport pass (they restore behavior the TS engine's in-process architecture had); untagged tasks are Go-side operational improvements that stay per the no-forward-port policy.

**Tech Stack:** Go 1.26, google.golang.org/grpc v1.81.1 (`keepalive` package not yet imported anywhere), dskit-port, modernc sqlite.

## Global Constraints

- Base branch: `rev-274`. Branch per task `fix/arch29-<slug>`; merge `--no-ff`; delete branch. Tasks 7→8 must land in order; others are order-independent but keep the sequence.
- Every `go` command: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`.
- Every commit: `git commit --no-gpg-sign`, trailer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.
- Go 1.26 idioms (`atomic.Int64`, `wg.Go`, `t.Context()`, `any`, `min`/`max`).
- `CGO_ENABLED=1 go test -race` on every touched package before declaring a task green; `gofmt -l` must stay empty (CI gates it).
- Fidelity gate: no wire/gameplay change anywhere in this arc. New HTTP endpoints (Task 6) and flags (Task 9) are Go-side operational surface, documented where applicable.
- After each merge: `go build ./...` + `go test ./...` green on `rev-274`.
- Never `git add` pre-existing untracked files (`goscape`, `.superpowers/`, stray dotfiles).

---

### Task 1: OnDemand pump transient ref — closes the third-writer race **[BACKPORT-FIDELITY]**

**Files:**
- Modify: `modules/world/client.go` (~line 141 `teardownRefs` block, ~172-187 drop methods, ~330-333 `clientODAdapter.send`, the `teardownRefs` doc comment at 124-140)
- Modify: `modules/world/server.go` (the `handleTCPConn` defer's pre-login flush branch)
- Test: `modules/world/client_teardown_test.go` (append)

**Interfaces:**
- Produces: `client.tryRef() bool` (CAS acquire-if-live); `clientODAdapter.send` returns `net.ErrClosed` when the client is tearing down.

**Why:** the OnDemand pump (`onDemand.serveClient` → `cq.c.send(frame)` with `od.mu` released → `clientODAdapter.send` → `c.write`/`c.flushWrite` on `c.bufw`) can be in flight while the conn goroutine's defer runs `clientClosed` + `dropConnRef` (refcount 1→0 for pure-OnDemand conns → immediate pool return). Documented arch-28 residual; this closes it. Second half: the defer's pre-login `flushWrite` also races an in-flight pump flush — skip it for `ClientStateOndemand` conns (nothing meaningful to flush at teardown; state is only written by the conn goroutine itself, so the read is safe).

- [ ] **Step 1: Write the failing tests** (append to `client_teardown_test.go`)

```go
// arch-29.1: the OnDemand pump takes a transient ref around each send so
// the pooled buffers can never be returned while a frame flush is in
// flight. After the last owner drops, send must refuse with net.ErrClosed.
func TestODAdapterSendRefusedAfterTeardown(t *testing.T) {
	p1, p2 := net.Pipe()
	defer p1.Close()
	c := newClient(p2, time.Second, slog.Default())
	c.dropConnRef() // last owner out: buffers returned
	a := &clientODAdapter{c: c}
	if err := a.send([]byte{0x01}); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("send after teardown: got %v, want net.ErrClosed", err)
	}
}

// Run with -race: concurrent pump sends versus the conn-side drop must
// never touch a pool-returned buffer. Pre-fix this is the documented
// arch-28 residual data race.
func TestODAdapterSendTeardownRace(t *testing.T) {
	for range 50 {
		p1, p2 := net.Pipe()
		go func() { // keep the pipe drained so flush doesn't block
			buf := make([]byte, 256)
			for {
				if _, err := p1.Read(buf); err != nil {
					return
				}
			}
		}()
		c := newClient(p2, 50*time.Millisecond, slog.Default())
		a := &clientODAdapter{c: c}
		var wg sync.WaitGroup
		wg.Go(func() {
			for range 10 {
				if err := a.send([]byte{0xFF}); err != nil {
					return
				}
			}
		})
		wg.Go(func() { c.dropConnRef() })
		wg.Wait()
		p1.Close()
	}
}

func TestTryRefLifecycle(t *testing.T) {
	p1, p2 := net.Pipe()
	defer p1.Close()
	c := newClient(p2, time.Second, slog.Default())
	if !c.tryRef() {
		t.Fatal("tryRef on live client must succeed")
	}
	c.dropRef()     // transient ref back: 2->1
	c.dropConnRef() // 1->0, released
	if c.tryRef() {
		t.Fatal("tryRef after release must fail")
	}
}
```

- [ ] **Step 2: Run to verify FAIL** — `go test ./modules/world/ -run 'TestODAdapter|TestTryRef' -v` → "undefined: tryRef" (and the adapter's send has no refusal path).

- [ ] **Step 3: Implement**

`modules/world/client.go` — add next to the drop methods:

```go
// tryRef acquires a transient buffer reference iff the client is still
// live (refcount > 0). The OnDemand pump takes one around each send so
// dropRef's pool return can never race an in-flight frame flush. CAS
// loop: a plain Add would resurrect a client whose buffers were already
// returned.
func (c *client) tryRef() bool {
	for {
		v := c.teardownRefs.Load()
		if v <= 0 {
			return false
		}
		if c.teardownRefs.CompareAndSwap(v, v+1) {
			return true
		}
	}
}
```

Rewrite `clientODAdapter.send` (current body at client.go:330-333):

```go
func (a *clientODAdapter) send(data []byte) error {
	// arch-29.1: transient ref brackets the write+flush so teardown's
	// pool return waits out an in-flight frame. A refused send means the
	// connection is closing; serveClient treats it like any send error.
	if !a.c.tryRef() {
		return net.ErrClosed
	}
	defer a.c.dropRef()
	a.c.write(data)
	return a.c.flushWrite()
}
```

Update the `teardownRefs` doc comment (client.go:124-140): replace the "known third bufw writer OUTSIDE this model ... arch-28 residual" sentences with: the OnDemand pump participates via transient `tryRef`/`dropRef` pairs around each send (arch-29.1 closed the arch-28 residual; PORTING.md Arc 31).

`modules/world/server.go` — in the `handleTCPConn` defer, the pre-login flush branch currently reads `} else if err := c.flushWrite(); err != nil {`. Change so OnDemand-state conns skip the flush:

```go
		} else if c.state != ClientStateOndemand {
			// Pre-login: this goroutine is the only writer; flush any
			// pending login reply before closing. OnDemand-state conns
			// skip it — the pump goroutine co-owns bufw via transient
			// refs (arch-29.1) and there is nothing useful to flush at
			// teardown.
			if err := c.flushWrite(); err != nil {
				s.logNet.Warn("failed to flush on connection close", "error", err, "remote_addr", conn.RemoteAddr())
			}
		}
```

- [ ] **Step 4: Verify** — targeted tests + `CGO_ENABLED=1 go test -race ./modules/world/ -run 'TestODAdapter|TestTryRef|TestClientTeardown'` PASS; full `go test ./modules/world/` PASS.

- [ ] **Step 5: Commit** — `fix(world): transient pump ref closes OnDemand third-writer race` + arch-29.1 body (names the arch-28 residual being closed) + trailer.

---

### Task 2: gRPC keepalives on both clients and both servers **[BACKPORT-FIDELITY]**

**Files:**
- Create: `modules/world/grpc_keepalive.go`
- Modify: `modules/world/login_client.go:37-49`, `modules/world/friends_client.go:75-87`
- Modify: `modules/login/server.go:20-29`, `modules/friends/server.go:21-32`
- Test: `modules/world/grpc_keepalive_test.go` (new)

**Interfaces:**
- Produces: `worldClientKeepalive() grpc.DialOption` (modules/world); `serverKeepalivePolicy() []grpc.ServerOption` duplicated as an unexported helper in each of modules/login and modules/friends (mirrored-sibling convention — do NOT create a shared package for two 10-line helpers).

**Why:** no keepalives anywhere; a silent partition wedges `friendsSubscriber.runOnce`/`worldEventsSubscriber.runOnce` in `stream.Recv()` forever (the backoff supervisor only fires on stream errors).

- [ ] **Step 1: Write the failing test** (`modules/world/grpc_keepalive_test.go`)

```go
package world

import (
	"testing"
	"time"
)

// arch-29.2: pins the keepalive contract. Time must be low enough to
// detect a dead NAT flow before players notice frozen friends state;
// PermitWithoutStream keeps the probe alive between RPCs.
func TestClientKeepaliveParams(t *testing.T) {
	p := clientKeepaliveParams()
	if p.Time != 30*time.Second {
		t.Errorf("Time: got %v, want 30s", p.Time)
	}
	if p.Timeout != 10*time.Second {
		t.Errorf("Timeout: got %v, want 10s", p.Timeout)
	}
	if !p.PermitWithoutStream {
		t.Error("PermitWithoutStream must be true (subscriber streams are idle-heavy)")
	}
}
```

- [ ] **Step 2: FAIL** — undefined: clientKeepaliveParams.

- [ ] **Step 3: Implement**

`modules/world/grpc_keepalive.go` (new):

```go
package world

import (
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

// clientKeepaliveParams is the keepalive contract for both bridge
// clients (login, friends). Without it a NAT/firewall dropping
// connection state without RST leaves the subscriber streams blocked in
// Recv() forever — the reconnect supervisors only run on stream errors
// (arch-29.2).
func clientKeepaliveParams() keepalive.ClientParameters {
	return keepalive.ClientParameters{
		Time:                30 * time.Second,
		Timeout:             10 * time.Second,
		PermitWithoutStream: true,
	}
}

func worldClientKeepalive() grpc.DialOption {
	return grpc.WithKeepaliveParams(clientKeepaliveParams())
}
```

In `NewLoginClient` and `NewFriendsClient`, add `worldClientKeepalive(),` as a second DialOption after `grpc.WithTransportCredentials(insecure.NewCredentials()),`.

`modules/login/server.go` and `modules/friends/server.go` (mirrored): change `s := grpc.NewServer()` to:

```go
	// arch-29.2: permit the world's 30s keepalive probes (default
	// EnforcementPolicy MinTime is 5m and would GOAWAY the client).
	s := grpc.NewServer(
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             15 * time.Second,
			PermitWithoutStream: true,
		}),
	)
```

(import `google.golang.org/grpc/keepalive` in both.)

- [ ] **Step 4: Verify** — targeted test PASS; `go build ./...`; full `go test ./modules/world/ ./modules/login/ ./modules/friends/`; `-race` on the three packages' targeted suites. Note in the report that the params test pins values only — real keepalive behavior is not integration-tested (would need fault injection; out of scope).

- [ ] **Step 5: Commit** — `fix(grpc): keepalive probes on bridge clients + enforcement on servers` + arch-29.2 body + trailer.

---

### Task 3: WorldStartup/WorldConnect timeout + background retry **[BACKPORT-FIDELITY]**

**Files:**
- Modify: `modules/world/login_client.go:58-66` (WorldStartup returns error), `modules/world/friends_client.go` (WorldConnect returns error — find its swallow-and-Warn body, same shape)
- Modify: `modules/world/world.go` (startingBody, lines ~198-219)
- Modify: any test fakes implementing `LoginClient`/`FriendsClient` (grep `WorldStartup(` in `modules/world/*_test.go` — signature change ripples)
- Test: `modules/world/world_startup_retry_test.go` (new)

**Interfaces:**
- Consumes: `Server.bridgesCtx`, `bridgeCallTimeout` (5s, bridges.go).
- Produces: `LoginClient.WorldStartup(ctx, nodeID, profile) error`; `FriendsClient.WorldConnect(ctx, nodeID, profile) error`; `Server.retryBridgeRegistration(name string, call func(context.Context) error)` (spawns a goroutine on bridgesCtx; not exported).

**Why:** `WorldStartup` is the ONLY mechanism clearing stale `account_login.logged_in` rows after an unclean crash; one failed attempt at boot (login restarting) strands every crashed-out player at `ALREADY_LOGGED_IN` until an operator intervenes. The client currently swallows the error (`login_client.go:58-66` Warn-only), so the caller can't even tell.

- [ ] **Step 1: Write the failing test**

```go
package world

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// arch-29.3: a failed WorldStartup at boot must retry in the background
// until it succeeds (it is an idempotent UPDATE) instead of stranding
// logged_in=1 rows forever.
func TestRetryBridgeRegistrationRetriesUntilSuccess(t *testing.T) {
	s := newTestServer(t)
	s.bridgeRetryDelay = time.Millisecond
	var calls atomic.Int32
	done := make(chan struct{})
	s.retryBridgeRegistration("login WorldStartup", func(ctx context.Context) error {
		if calls.Add(1) < 3 {
			return errors.New("login restarting")
		}
		close(done)
		return nil
	})
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("registration never succeeded")
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("calls: got %d, want 3", got)
	}
}

func TestRetryBridgeRegistrationStopsOnShutdown(t *testing.T) {
	s := newTestServer(t)
	s.bridgeRetryDelay = time.Millisecond
	var calls atomic.Int32
	s.retryBridgeRegistration("friends WorldConnect", func(ctx context.Context) error {
		calls.Add(1)
		return errors.New("always down")
	})
	time.Sleep(20 * time.Millisecond) // let a few attempts happen
	s.bridgesCancel()
	n := calls.Load()
	time.Sleep(20 * time.Millisecond)
	if calls.Load() > n+1 { // at most one straggler attempt racing the cancel
		t.Fatalf("retry loop kept running after bridgesCancel: %d -> %d", n, calls.Load())
	}
}
```

(Check `newTestServer` initializes `bridgesCtx`/`bridgesCancel`; if not, initialize them in the test via the same pattern `NewServer` uses.)

- [ ] **Step 2: FAIL** — undefined: retryBridgeRegistration / bridgeRetryDelay.

- [ ] **Step 3: Implement**

`modules/world/login_client.go` — `WorldStartup` returns the error (keep the Warn):

```go
func (c *grpcLoginClient) WorldStartup(ctx context.Context, nodeID int32, profile string) error {
	_, err := c.client.WorldStartup(ctx, &loginpb.WorldStartupRequest{
		NodeId:  nodeID,
		Profile: profile,
	})
	if err != nil {
		c.log.Warn("WorldStartup RPC failed", slog.Any("err", err))
	}
	return err
}
```

Update the `LoginClient` interface signature and every fake. Mirror for `FriendsClient.WorldConnect`.

`modules/world/server.go` (near the bridge wiring): add field `bridgeRetryDelay time.Duration` (set `5 * time.Second` in `NewServer` next to `logoutSaveRetryDelay`), and:

```go
// retryBridgeRegistration runs an idempotent bridge registration call
// (WorldStartup / WorldConnect) until it succeeds once, in a background
// goroutine parented to bridgesCtx. Each attempt gets bridgeCallTimeout.
// arch-29.3: one failed WorldStartup at boot used to strand every
// crashed-out player at ALREADY_LOGGED_IN with no self-healing.
func (s *Server) retryBridgeRegistration(name string, call func(context.Context) error) {
	go func() {
		for {
			ctx, cancel := context.WithTimeout(s.bridgesCtx, bridgeCallTimeout)
			err := call(ctx)
			cancel()
			if err == nil {
				return
			}
			if s.bridgesCtx.Err() != nil {
				return
			}
			s.log.Warn("bridge registration failed; retrying",
				slog.String("call", name), slog.Any("err", err))
			select {
			case <-time.After(s.bridgeRetryDelay):
			case <-s.bridgesCtx.Done():
				return
			}
		}
	}()
}
```

`modules/world/world.go` startingBody — replace the two bare calls:

```go
	if lc != nil {
		serv.retryBridgeRegistration("login WorldStartup", func(ctx context.Context) error {
			return lc.WorldStartup(ctx, int32(serv.cfg.NodeID), serv.cfg.NodeProfile)
		})
	}
	if fc != nil {
		serv.retryBridgeRegistration("friends WorldConnect", func(ctx context.Context) error {
			return fc.WorldConnect(ctx, int32(serv.cfg.NodeID), serv.cfg.NodeProfile)
		})
	}
```

(Boot no longer blocks on the ~20s gRPC min-connect timeout either — the review's secondary complaint.)

- [ ] **Step 4: Verify** — targeted + `-race`; full world suite; `go build ./...` (interface change ripples — compile-all gate `go test -run '^$' ./...`).

- [ ] **Step 5: Commit** — `fix(world): retry WorldStartup/WorldConnect until success` + arch-29.3 body + trailer.

---

### Task 4: Friends shutdown — close subscriber streams, bound GracefulStop **[BACKPORT-FIDELITY]**

**Files:**
- Modify: `modules/friends/subscriptions.go` (registry `closeAll`), `modules/friends/world_subscriptions.go` (mirror), `modules/friends/friends.go:67-84` (running), `modules/friends/server.go` (bounded shutdown)
- Test: `modules/friends/subscriptions_test.go` (append), `modules/friends/friends_shutdown_test.go` (new, only if a bufconn harness already exists in handler_test.go — otherwise registry tests + a direct `grpcServer.shutdown` bound test suffice; document the choice)

**Interfaces:**
- Produces: `(*subscriptions).closeAll()`, `(*worldSubscriptions).closeAll()`; `grpcServer.shutdown()` becomes bounded (GracefulStop raced against 5s → `Stop()`).

**Why:** `running()` calls `f.srv.shutdown()` (bare `GracefulStop`) then blocks on `<-serverDone`; `SubscribeUpdates`/`SubscribeWorldEvents` loops exit only on client close or `sub.done` — nothing server-side closes them, so `--target friends` standalone never finishes SIGTERM while worlds are attached.

- [ ] **Step 1: Write the failing tests** (registry-level; mirror for worldSubscriptions)

```go
// arch-29.4: closeAll releases every subscriber's done channel exactly
// once (register's close-prior must not double-close afterwards).
func TestSubscriptionsCloseAll(t *testing.T) {
	subs := newSubscriptions(slog.Default()) // adapt to the real constructor
	a := subs.register(subKeyFor(1, "main"))  // adapt: however handler.go registers
	b := subs.register(subKeyFor(2, "main"))
	subs.closeAll()
	for _, sub := range []*subscriber{a, b} {
		select {
		case <-sub.done:
		default:
			t.Fatal("subscriber done not closed by closeAll")
		}
	}
	// registry emptied: a later register must not re-close a's done (panic guard)
	_ = subs.register(subKeyFor(1, "main"))
}
```

(Adapt constructor/registration calls to the real API in subscriptions.go:52-90 — the struct is `subscriptions{mu, by map[subKey]*subscriber, log}`.)

- [ ] **Step 2: FAIL** — undefined: closeAll.

- [ ] **Step 3: Implement**

Both registries (mirrored):

```go
// closeAll releases every live subscriber's stream loop and empties the
// registry. Called at service stop so GracefulStop is not held open by
// streams only a client could otherwise end (arch-29.4). Deleting while
// closing keeps register's close-prior path from double-closing.
func (s *subscriptions) closeAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, sub := range s.by {
		close(sub.done)
		delete(s.by, k)
	}
}
```

`modules/friends/server.go` — bound the stop:

```go
// shutdown drains gracefully but never hangs: streams are closed by the
// registries first; anything still open after the grace window is cut
// hard (arch-29.4).
func (g *grpcServer) shutdown() {
	done := make(chan struct{})
	go func() {
		g.server.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		g.log.Warn("GracefulStop grace window elapsed; forcing Stop")
		g.server.Stop()
		<-done
	}
}
```

`modules/friends/friends.go` running(): before `f.srv.shutdown()` on the ctx.Done path, add `f.subs.closeAll()` and `f.worldSubs.closeAll()` (check the actual field names on `Friends` — grep `subs`/`worldSubs` in friends.go; they're passed to `newGRPCServer`).

- [ ] **Step 4: Verify** — registry tests + `-race ./modules/friends/`; full friends suite. If handler_test.go has a bufconn/stream harness, add one end-to-end test: open a Subscribe stream, call the running-ctx-cancel path, assert return within ~6s.

- [ ] **Step 5: Commit** — `fix(friends): close subscriber streams at stop; bound GracefulStop` + arch-29.4 body + trailer.

---

### Task 5: Config verification — fan-out, CheckConfig warnings, hard-fail, dead keys

**Files:**
- Modify: `cmd/goscape/app/config.go:51-68`, `cmd/goscape/main.go:31-40` area and `configIsValid` (57-74)
- Modify: `modules/world/config.go` (delete `LogFormat` line 16 and `EnableTCPServer` line 61)
- Modify: `examples/full-config-reference.yaml` (remove/adjust the corresponding documented keys; the reference documents `enable_tcp_server` as dead ~line 350 and must drop it; keep the dskit-inline `ondemand.log_format` key but document it as accepted-and-ignored — it comes from the upstream dskit server.Config and stays for port parity)
- Test: `cmd/goscape/app/config_test.go` (append)

**Why:** `--config.verify` green-lights configs that fail at boot (only World validates); `CheckConfig` is an empty TODO while ondemand/world `node_id`/`node_port`/`members` drift silently breaks `/rs2.cgi` portoff; normal-mode boot *ignores* `isValid`; `world.log_format`/`world.enable_tcp_server` decode strictly but do nothing (the project's own reference-yaml rationale calls such keys "strictly worse than omitting them").

- [ ] **Step 1: Write the failing tests** (append to `cmd/goscape/app/config_test.go`; mirror existing test style there)

```go
func TestValidateFansOutToAllModules(t *testing.T) {
	cfg := newDefaultTestConfig(t) // adapt: however existing tests build a default Config
	cfg.Login.Enable = true
	cfg.Login.BcryptCost = 99 // out of range per modules/login/config.go Validate
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "login") {
		t.Fatalf("want login validation error, got %v", err)
	}
}

func TestCheckConfigWarnsOnOndemandWorldDrift(t *testing.T) {
	cfg := newDefaultTestConfig(t)
	cfg.World.Enable = true
	cfg.OnDemand.Enable = true
	cfg.World.TCPListenPort = 43594
	cfg.OnDemand.NodePort = 40000 // drifted
	warnings := cfg.CheckConfig()
	if len(warnings) == 0 {
		t.Fatal("want a node_port drift warning")
	}
}

func TestCheckConfigSilentWhenAligned(t *testing.T) {
	cfg := newDefaultTestConfig(t)
	cfg.World.Enable = true
	cfg.OnDemand.Enable = true
	// defaults are aligned
	if w := cfg.CheckConfig(); len(w) != 0 {
		t.Fatalf("want no warnings, got %v", w)
	}
}
```

(Adapt exact field names — ondemand's mirrors are at `modules/ondemand/config.go:32-40`; grep for `node_port`/`NodePort`/`NodeID`/`Members` there and use the real names. If no `newDefaultTestConfig` helper exists, build one locally: a `Config` with `RegisterFlagsAndApplyDefaults` applied via a throwaway FlagSet, matching existing config tests.)

- [ ] **Step 2: FAIL** — fan-out missing (login error not surfaced); CheckConfig returns empty.

- [ ] **Step 3: Implement**

`cmd/goscape/app/config.go`:

```go
func (c *Config) Validate() error {
	// CFG-2 (Arc 18) fanned out world; arch-29.5 completes the fan-out —
	// --config.verify used to green-light login/friends/ondemand configs
	// that then failed at boot.
	if err := c.World.Validate(); err != nil {
		return fmt.Errorf("world: %w", err)
	}
	if err := c.Login.Validate(); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	if err := c.Friends.Validate(); err != nil {
		return fmt.Errorf("friends: %w", err)
	}
	if err := c.OnDemand.Validate(); err != nil {
		return fmt.Errorf("ondemand: %w", err)
	}
	return nil
}
```

(Confirm each module's `Validate` short-circuits on `!c.Enable` — login/friends/ondemand do per the review; if one doesn't, gate the call on `.Enable` here instead.)

`CheckConfig` — implement the drift warnings (both modules enabled only):

```go
func (c *Config) CheckConfig() []ConfigWarning {
	var warnings []ConfigWarning
	if c.World.Enable && c.OnDemand.Enable {
		if c.OnDemand.NodePort != c.World.TCPListenPort {
			warnings = append(warnings, ConfigWarning{
				Message: "ondemand.node_port does not match world.tcp_listen_port",
				Explain: "/rs2.cgi will advertise a game port the world is not listening on",
			})
		}
		if c.OnDemand.NodeID != c.World.NodeID {
			warnings = append(warnings, ConfigWarning{
				Message: "ondemand.node_id does not match world.node_id",
			})
		}
		if c.OnDemand.Members != c.World.NodeMembers {
			warnings = append(warnings, ConfigWarning{
				Message: "ondemand.node_members does not match world.node_members",
			})
		}
	}
	return warnings
}
```

(Adapt field names to the real ondemand Config; check `ConfigWarning`'s real field set in config.go.)

`cmd/goscape/main.go`:
- `configIsValid`: warnings are logged but NO LONGER return false (a warning that fails verify is a contradiction — the review flagged it). Only `Validate()` errors return false.
- Normal mode: after the `isValid` computation, exit hard when invalid: find the block at main.go:31-40 (verify-mode exits with a code; normal mode currently proceeds); make normal mode `os.Exit(1)` after logging when `!isValid` (removes the boot-then-fail-twice path).

Delete `modules/world/config.go` line 16 (`LogFormat`) and line 61 (`EnableTCPServer`) + any reference-yaml lines documenting them. NOTE for the commit body: strict decoding means configs still carrying `world.log_format`/`world.enable_tcp_server` become fatal boot errors — both were silent no-ops before, and neither appears in any example config.

- [ ] **Step 4: Verify** — new tests PASS; `go build ./...`; `make goscape && make validate-example-configs` (all example configs must still verify — they don't carry the deleted keys); full `go test ./cmd/... ./modules/...` targeted packages.

- [ ] **Step 5: Commit** — `fix(config): full Validate fan-out, ondemand/world drift warnings, hard-fail on invalid, drop dead keys` + arch-29.5 body + trailer.

---

### Task 6: /healthz + /debug/status on the ondemand mux

**Files:**
- Modify: `modules/world/server.go` (health atomics), `modules/world/tick.go` (stamp last tick), `modules/world/player_list или server.go` (players counter — at the same add/remove sites `removePlayerInternal`/`addPlayer` use)
- Create: `modules/ondemand/health.go`
- Modify: `cmd/goscape/app/modules.go` initOnDemand (route wiring)
- Modify: `production/helm/goscape/templates/_helpers.tpl:175-177` (readiness probe → httpGet /healthz)
- Test: `modules/ondemand/health_test.go` (new), `modules/world/health_snapshot_test.go` (new)

**Interfaces:**
- Produces: `world.Server.HealthSnapshot() world.HealthSnapshot` where `type HealthSnapshot struct { LastTick time.Time; CurrentTick int64; PlayersOnline int; LastCycleMillis int }`; `ondemand.RegisterHealthRoutes(mux *http.ServeMux, snap func() (ondemand.WorldHealth, bool))` — define `type WorldHealth = ...` as a small local interface `interface{ HealthSnapshot() world.HealthSnapshot }`? NO — ondemand must not import world (check import direction first: modules/ondemand currently imports `pkg/world/connhandler`, not modules/world). Instead define in ondemand:

```go
// HealthSnapshot is the subset of world state the health endpoints
// need; modules/world's Server provides a compatible method and the app
// wires it through an adapter func to avoid a modules/ondemand →
// modules/world import.
type HealthSnapshot struct {
	LastTick        time.Time
	CurrentTick     int64
	PlayersOnline   int
	LastCycleMillis int
}
```

and `RegisterHealthRoutes(mux *http.ServeMux, snap func() (HealthSnapshot, bool))` — `snap` returns `false` when no world is wired (standalone ondemand → healthz is a plain process-up 200). The app adapter in modules.go converts `world.HealthSnapshot` → `ondemand.HealthSnapshot` field-by-field (two small structs beat an import cycle).

**Why:** zero runtime observability; the Helm tcpSocket probe reports ready while a wedged tick loop strands players. Last-tick age is the one signal that catches the real failure mode.

- [ ] **Step 1: Write the failing tests**

`modules/ondemand/health_test.go`:

```go
func TestHealthzFreshTick(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHealthRoutes(mux, func() (HealthSnapshot, bool) {
		return HealthSnapshot{LastTick: time.Now(), CurrentTick: 42, PlayersOnline: 3, LastCycleMillis: 12}, true
	})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("fresh tick: got %d, want 200", rr.Code)
	}
}

func TestHealthzStaleTick(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHealthRoutes(mux, func() (HealthSnapshot, bool) {
		return HealthSnapshot{LastTick: time.Now().Add(-time.Minute)}, true
	})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/healthz", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("stale tick: got %d, want 503", rr.Code)
	}
}

func TestHealthzNoWorld(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHealthRoutes(mux, func() (HealthSnapshot, bool) { return HealthSnapshot{}, false })
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("standalone ondemand: got %d, want 200 (process-up)", rr.Code)
	}
}

func TestDebugStatusJSON(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHealthRoutes(mux, func() (HealthSnapshot, bool) {
		return HealthSnapshot{LastTick: time.Now(), CurrentTick: 7, PlayersOnline: 2, LastCycleMillis: 9}, true
	})
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/debug/status", nil))
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if got["players_online"].(float64) != 2 || got["current_tick"].(float64) != 7 {
		t.Fatalf("unexpected payload: %v", got)
	}
}
```

`modules/world/health_snapshot_test.go`:

```go
func TestHealthSnapshotTracksTickAndPlayers(t *testing.T) {
	s := newTestServer(t)
	s.stampTick() // whatever helper name Step 3 introduces
	snap := s.HealthSnapshot()
	if time.Since(snap.LastTick) > time.Second {
		t.Fatal("LastTick not stamped")
	}
	// player counter: exercise via the same registration helper other
	// tests use (grep addPlayer usage in world tests) and assert
	// PlayersOnline increments/decrements around add/remove.
}
```

- [ ] **Step 2: FAIL** — undefined symbols.

- [ ] **Step 3: Implement**

`modules/world/server.go`: fields `lastTickNano atomic.Int64`, `playersOnline atomic.Int32`. `stampTick()` sets `lastTickNano.Store(time.Now().UnixNano())` — call it in the tick loop body right where `s.currentTick++` happens (tick.go:308). Increment `playersOnline` where a player enters `s.players` (`addPlayer`/the `processLogins` registration — find the single site that adds to `s.players` and pair with `removePlayerInternal`'s removal; use the slot-identity-guarded paths so double-removal doesn't double-decrement — decrement only when `removePlayerInternal` actually unlinks). `HealthSnapshot()`:

```go
type HealthSnapshot struct {
	LastTick        time.Time
	CurrentTick     int64
	PlayersOnline   int
	LastCycleMillis int
}

// HealthSnapshot is read by the ondemand health endpoints via an app-
// level adapter (arch-29.6). All fields come from atomics/LastCycleStat;
// callable from any goroutine.
func (s *Server) HealthSnapshot() HealthSnapshot {
	return HealthSnapshot{
		LastTick:        time.Unix(0, s.lastTickNano.Load()),
		CurrentTick:     int64(s.currentTick), // NO: currentTick is tick-owned; add currentTickAtomic
		...
	}
}
```

**Correction to lock in:** `s.currentTick` is tick-goroutine-owned (`int`); do NOT read it cross-goroutine. Add `currentTickAtomic atomic.Int64` stamped alongside `lastTickNano` in `stampTick()` (`s.currentTickAtomic.Store(int64(s.currentTick))`), and read that. Same for cycle time: `LastCycleStat` reads `lastCycleStats` which the tick writes — copy `statCycle` into an atomic in `stampTick()` too (`lastCycleMillis atomic.Int64`). Three atomics, one stamp function, zero locks on the tick path.

`modules/ondemand/health.go`:

```go
package ondemand

import (
	"encoding/json"
	"net/http"
	"time"
)

// healthzStaleAfter: a tick loop silent for this long is wedged — the
// world accepts TCP but strands players, which is exactly what the old
// tcpSocket readiness probe could not see (arch-29.6).
const healthzStaleAfter = 10 * time.Second

type HealthSnapshot struct {
	LastTick        time.Time
	CurrentTick     int64
	PlayersOnline   int
	LastCycleMillis int
}

func RegisterHealthRoutes(mux *http.ServeMux, snap func() (HealthSnapshot, bool)) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		s, hasWorld := snap()
		if hasWorld && time.Since(s.LastTick) > healthzStaleAfter {
			http.Error(w, "tick stale", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /debug/status", func(w http.ResponseWriter, r *http.Request) {
		s, hasWorld := snap()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"world_wired":      hasWorld,
			"last_tick_age_ms": time.Since(s.LastTick).Milliseconds(),
			"current_tick":     s.CurrentTick,
			"players_online":   s.PlayersOnline,
			"tick_ms":          s.LastCycleMillis,
		})
	})
}
```

`cmd/goscape/app/modules.go` initOnDemand — after the existing `HandleFunc("GET /", ...)` wiring:

```go
	worldSrv := (*world.Server)(nil)
	if g.world != nil {
		worldSrv = g.world.Server
	}
	g.ondemand.Server.HTTP // (use the same mux variable the file already uses)
	ondemand.RegisterHealthRoutes(g.ondemand.Server.HTTP, func() (ondemand.HealthSnapshot, bool) {
		if worldSrv == nil {
			return ondemand.HealthSnapshot{}, false
		}
		s := worldSrv.HealthSnapshot()
		return ondemand.HealthSnapshot{
			LastTick:        s.LastTick,
			CurrentTick:     s.CurrentTick,
			PlayersOnline:   s.PlayersOnline,
			LastCycleMillis: s.LastCycleMillis,
		}, true
	})
```

Helm `_helpers.tpl:175-177`: replace the tcpSocket readiness probe with `httpGet { path: /healthz, port: <the ondemand HTTP port name used in the chart> }` — grep the chart for the port name; ONLY for deployments that run ondemand (check how the chart gates probes per target; if the world-only deployment has no HTTP port, keep tcpSocket there and note it).

- [ ] **Step 4: Verify** — all new tests; `-race ./modules/world/ ./modules/ondemand/` targeted; `make helm-lint` (exists per Makefile facts); full builds.

- [ ] **Step 5: Commit** — `feat(observability): /healthz (tick liveness) + /debug/status on ondemand mux` + arch-29.6 body + trailer.

---

### Task 7: Packet/filestream/protocol API footguns

**Files:**
- Modify: `pkg/io/packet/packet.go` (RSADec ~571-598, GData 277-280), `pkg/io/packet/packetbit.go` (PBit 61-94, AccessBits doc), `pkg/tapper/tapper.go` (Tap doc, 40-48), `pkg/io/filestream/filestream.go` (New 43-103), `pkg/io/protocol/protocol.go` (doc comment), `pkg/cache/crctable.go` (MakeCRCs returns error)
- Modify (ripple): `modules/ondemand/ondemand.go:73-79` (filestream.New error), `modules/world/world.go` startingBody + `modules/world/reload.go:273` (MakeCRCs error), `cmd/goscape-cli` if it calls filestream.New (grep)
- Test: `pkg/io/packet/packet_footgun_test.go`, `pkg/io/filestream/filestream_error_test.go` (new)

**Why:** `RSADec` promises an error it never returns (its real failure mode is a panic contained only by serveConn's recover — a misleading signature inviting crashes at new call sites); `GData` silently under-copies when `dest` is short while advancing `Pos` (desync corruption with no crash signal); `PBit` compares an absolute byte index against Pos-relative `Len()` and swallows one grow error; `filestream.New` panics on I/O errors inside error-returning constructors; `CheckPacketLength`'s first return value means "available" on two paths and "needed" on another with no doc.

- [ ] **Step 1: Write the failing tests**

```go
// pkg/io/packet/packet_footgun_test.go
func TestRSADecTruncatedReturnsError(t *testing.T) {
	p := NewFromBytes([]byte{50}) // adapt to real constructor: declares 50-byte block, provides none
	_, err := p.RSADec(testRSAKey(t)) // adapt: how existing RSADec tests build a key
	if err == nil {
		t.Fatal("truncated RSA block must return an error, not panic (arch-29.7)")
	}
}

func TestGDataShortDestPanics(t *testing.T) {
	p := NewFromBytes([]byte{1, 2, 3, 4})
	defer func() {
		if recover() == nil {
			t.Fatal("GData with short dest must panic loudly, not silently under-copy")
		}
	}()
	dest := make([]byte, 2)
	p.GData(dest, 4)
}
```

(Adapt constructors to the real packet API — grep existing `RSADec` tests in `pkg/io/protocol/login/req/` for the truncated-block fixture; `req.TestUnmarshalBinary_TruncatedRSABlockPanics` exists and will need updating: after this fix the req-layer behavior CHANGES from panic to returned error — update that test to assert the error path and rename it accordingly. The two existing RSADec callers (`req.go:126-129`, `server.go:1228-1231`) already check the error, so behavior at those sites improves from process-panic-recovered to clean error close: verify the world login path still rejects the connection (existing gap-login-wire-1 test may need its expectation adjusted from "recovered panic" to "clean close" — check and adjust, documenting it).)

```go
// pkg/io/filestream/filestream_error_test.go
func TestNewErrorsOnUnreadableDir(t *testing.T) {
	_, err := New(filepath.Join(t.TempDir(), "no", "such", "deep", "\x00bad"), false, true)
	if err == nil {
		t.Fatal("want error, got nil")
	}
}
```

- [ ] **Step 2: FAIL** — RSADec panics instead of erroring; GData copies silently; New panics.

- [ ] **Step 3: Implement**

- `RSADec`: at the top of the block read, bounds-check: `if int(numBytes) > p.Len() { return nil, fmt.Errorf("rsa block truncated: declared %d, have %d", numBytes, p.Len()) }` (keep the rest as-is).
- `GData`: `if len(dest) < length { panic(fmt.Sprintf("packet: GData dest %d < length %d", len(dest), length)) }` before the copy (consistent with the panic-on-underflow model; the doc comment states it).
- `PBit`: change both grow checks from `bytePos+1 > p.Len()` to `bytePos+1 > len(p.Data)` and make the second check handle the Write error like the first. One-line doc on `AccessBits` noting `BitPos` is absolute.
- `Tapper.Tap` doc: add "payload is only valid for the duration of the call; implementations must copy it before retaining or exporting it asynchronously — it aliases a live connection buffer."
- `filestream.New` → `func New(dir string, createNew, readOnly bool) (*FileStream, error)`: every `panic(err)` becomes `return nil, fmt.Errorf(...)`. Ripple: `modules/ondemand/ondemand.go:73-79` propagates (`New` already returns error); `cache.MakeCRCs` → `func MakeCRCs(cachePath string) error` propagating filestream errors; `modules/world/world.go` startingBody: `if err := cache.MakeCRCs(cachePath); err != nil { return fmt.Errorf("crc table: %w", err) }` (boot fails cleanly instead of a raw panic stack); `modules/world/reload.go:273`: log the error, keep the old snapshot (reload should not crash the world). grep `filestream.New` and `MakeCRCs(` repo-wide for any other caller (goscape-cli pack/unpack paths) and propagate/log appropriately per binary.
- `CheckPacketLength`: add a doc comment stating the first return is bytes-available on the two insufficient-header paths (lines 28, 38) and declared-packet-size on the others — callers must branch on the bool, not interpret the int uniformly.

- [ ] **Step 4: Verify** — targeted tests; the adjusted req/world tests; compile-all `go test -run '^$' ./...`; full `go test ./pkg/... ./modules/...`; `-race` on touched packages.

- [ ] **Step 5: Commit** — `fix(io): honest error contracts for RSADec/GData/PBit/filestream; doc CheckPacketLength + Tap retention` + arch-29.7 body + trailer.

---

### Task 8: Lifecycle hygiene — acquisition in starting, rollback, zero-CRC, disabled modules, signal handler

**Files:**
- Modify: `modules/world/server.go` (NewServer 446-520: stop binding the listener + stop spawning worldEventsSubscriber), `modules/world/world.go` (startingBody acquires; New closes clients on error path)
- Modify: `pkg/dskit/server/server.go:111-114` (close httpListener on middleware error)
- Modify: `cmd/goscape/app/modules.go` (disabled modules return `nil, nil`; os.Exit→error at the 4 logger sites; zero-CRC guard), `cmd/goscape/app/app.go` (signal handler Stop on all paths; App.Stop idempotent), `pkg/dskit/signals/signals.go` (idempotent Stop)
- Test: `modules/world/server_lifecycle_test.go` (new), `cmd/goscape/app/modules_disabled_test.go` (new)

**Interfaces:**
- Produces: `Server.Listen() error` (binds the TCP listener; called from startingBody before the Run spawn); `Server.startWorldEventsSubscriber()` (spawns the subscriber; called from startingBody). `NewServer` no longer binds or spawns.
- Consumes: Task 7's `cache.MakeCRCs(path) error`.

**Why:** constructors acquire real resources (world binds TCP + spawns a live subscriber goroutine talking to friends before friends' listener exists; dskit server leaks its listener if middleware building fails; module-init has no rollback so an HTTP port conflict leaves the world listener bound and goroutines running). Disabled modules masquerade as Running (IdleService), which is also what makes ondemand-without-world silently serve a zero-value CRC snapshot.

- [ ] **Step 1: Write the failing tests**

```go
// modules/world/server_lifecycle_test.go
// arch-29.8: NewServer must not bind — two Servers on the same port can
// coexist until Listen(); acquisition belongs to the service's starting
// phase so failed init of a LATER module doesn't leak this one's socket.
func TestNewServerDoesNotBind(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()
	cfg := minimalServerConfig(t) // adapt: however world tests build a Config for NewServer
	cfg.TCPListenAddress = "127.0.0.1"
	cfg.TCPListenPort = lis.Addr().(*net.TCPAddr).Port
	s, err := NewServer(cfg, ...) // adapt args to the real signature
	if err != nil {
		t.Fatalf("NewServer must succeed without binding: %v", err)
	}
	if err := s.Listen(); err == nil {
		t.Fatal("Listen on an occupied port must fail")
	}
}
```

```go
// cmd/goscape/app/modules_disabled_test.go
// arch-29.8: a disabled module contributes no service — no vacuous
// "Running" state, no vacuously-satisfied AwaitRunning for dependents.
func TestDisabledModulesYieldNoService(t *testing.T) {
	g := newTestApp(t) // adapt: however app tests construct an App; all modules disabled by default
	svcs, err := g.ModuleManager.InitModuleServices(All)
	if err != nil {
		t.Fatalf("init with everything disabled: %v", err)
	}
	if len(svcs) != 0 {
		t.Fatalf("want 0 services for all-disabled target, got %d: %v", len(svcs), maps.Keys(svcs))
	}
}
```

(Verify first that `pkg/dskit/modules` `InitModuleServices` tolerates nil services — the review cited modules.go:139-147; read it. If it doesn't, the initFn returns `nil, nil` must be handled there — upstream dskit supports it; adjust minimally with a comment if the port dropped it.)

- [ ] **Step 2: FAIL** — NewServer binds today; disabled modules return IdleService.

- [ ] **Step 3: Implement** (keyed changes)

- `Server.Listen()`: move the `net.Listen` block (server.go:456-459) into it; `NewServer` leaves `tcpListener` nil. `serveTCP`/`Shutdown` nil-guards: `Shutdown` already closes `s.tcpListener` — guard `if s.tcpListener != nil`.
- `Server.startWorldEventsSubscriber()`: move the block at server.go:515-519 into it.
- `world.go` startingBody: order — `if err := cache.MakeCRCs(cachePath); err != nil { return ... }` → `if err := serv.Listen(); err != nil { return ... }` → `serv.startWorldEventsSubscriber()` → registrations (Task 3's retry calls) → content-watch block → spawn Run.
- `world.New` error path: if `NewServer` fails, close both bridge clients before returning (they're non-nil-guarded `Close()` calls).
- dskit `server.New`: on the `BuildHTTPMiddleware` error path add `_ = httpListener.Close()` (with a one-line comment) before returning.
- `modules.go`: all four `if !enabled` branches → `return nil, nil` with one `logger.Info("module disabled", "module", ...)`; the four `os.Exit(1)` logger-failure sites → `return nil, fmt.Errorf("failed to create %s logger: %w", name, err)`.
- Zero-CRC: in initOnDemand, when `g.world == nil || g.world.Server == nil` and `g.cfg.OnDemand.CachePath != ""`: `if err := cache.MakeCRCs(g.cfg.OnDemand.CachePath); err != nil { return nil, ... }` with a comment (standalone ondemand must build its own CRC snapshot — previously served zeros).
- Signal handler: in `App.Run`, ensure `handler.Stop()` runs on non-signal exits (defer a `sync.OnceFunc` wrapper); make `signals.Handler.Stop` idempotent (guard the `close(h.quit)` with its own `sync.Once`).

- [ ] **Step 4: Verify** — new tests; FULL suite `go test ./...` (lifecycle reordering can break world tests that assumed NewServer binds — the shutdown/chatty harnesses construct Servers manually with their own listeners, so they should be unaffected, but check `newChattyShutdownTestServer`); `-race` on world + app; `make goscape && make validate-example-configs`; boot smoke: `./cmd/goscape/goscape --config.file examples/bundled/goscape.yaml --config.verify=true`.

- [ ] **Step 5: Commit** — `fix(lifecycle): acquire in starting, rollback on init failure, standalone-ondemand CRCs, nil disabled modules` + arch-29.8 body + trailer.

---

### Task 9: Dead code sweep, --version, DRY module-name resolution

**Files:**
- Modify: `cmd/goscape/app/app.go` (drop `shutdownRequested` write-only var 123/162; extract `resolveModuleName`/`isRequestedStop` shared by `serviceFailed` + `failedServicesError`), `cmd/goscape/main.go` (version flag + startup log), `modules/world/world.go:49-52` (discarded signals.Handler), `modules/login/login.go:35-38` + `modules/friends/friends.go:39-42` (unused factories), `modules/ondemand/ondemand.go` 56/81-91 + `cmd/goscape/app/modules.go:89-91` (commented scaffolding)
- NOT touched: unused-but-upstream dskit APIs (`UserInvisibleTargetableModule`, `IsTargetableModule`, `UserVisibleModuleNames`) — upstream-parity beats dead-code removal there; say so in the commit body.
- Test: `cmd/goscape/app/app_failure_test.go` (adjust if helper extraction changes call shape), `pkg/util/build/build_test.go` (new)

- [ ] **Step 1: Write the failing test** (`pkg/util/build/build_test.go`)

```go
func TestVersionString(t *testing.T) {
	Version, Revision, Branch = "v1", "abc1234", "rev-274"
	got := String()
	for _, want := range []string{"v1", "abc1234", "rev-274", runtime.Version()} {
		if !strings.Contains(got, want) {
			t.Errorf("String() = %q missing %q", got, want)
		}
	}
}
```

- [ ] **Step 2: FAIL** — undefined: String.

- [ ] **Step 3: Implement**

`pkg/util/build/build.go`:

```go
// String renders the ldflags-injected build metadata for --version and
// the startup log. Empty fields (local `go run`) render as "unknown".
func String() string {
	v := cmp.Or(Version, "unknown")
	r := cmp.Or(Revision, "unknown")
	b := cmp.Or(Branch, "unknown")
	return fmt.Sprintf("goscape %s (revision %s, branch %s, %s)", v, r, b, GoVersion)
}
```

`cmd/goscape/main.go`: register a `--version` bool flag alongside the config flags (check how flags are registered there — the config layering parses its own FlagSet; add version to the same set, fast-path `if *printVersion { fmt.Println(build.String()); return }` before config loading); change line 49 to `logger.Info("starting goscape", "target", config.Target, "version", build.Version, "revision", build.Revision)` and drop the TODO.

Dead-code removals as listed in Files. DRY extraction in app.go:

```go
// resolveModuleName maps a service back to its module key ("unknown"
// if unregistered) — shared by the failure listener and the post-stop
// exit-code check so their classifications can't drift (arch-29.9).
func resolveModuleName(serviceMap map[string]services.Service, svc services.Service) string { ... }

// isRequestedStop reports whether a FailureCase represents a requested
// shutdown rather than a failure (ErrStopProcess, context.Canceled).
func isRequestedStop(err error) bool { ... }
```

Both `serviceFailed` and `failedServicesError` use them.

- [ ] **Step 4: Verify** — build_test PASS; `go build ./...`; full `go test ./cmd/... ./pkg/util/... ./modules/...`; manually: `GOPATH=... go run ./cmd/goscape --version` prints the string and exits 0.

- [ ] **Step 5: Commit** — `chore(app): dead-code sweep, --version flag, shared failure classification` + arch-29.9 body + trailer.

---

### Task 10: Makefile pruning

**Files:**
- Modify: `Makefile` (delete targets `benchmark-store` 316-318, `test-integration` 254-255, `doc`/`check-doc` 388-396, `goscape-mixin` 178-194; remove the `faillint` invocation from `lint` 238-245 keeping golangci-lint; `check-format` 381-384: `GIT_TARGET_BRANCH ?= $(shell git rev-parse --abbrev-ref HEAD)`)

- [ ] **Step 1: Verify current breakage** — `make -n benchmark-store` etc. fail on missing paths; capture before-output.
- [ ] **Step 2: Apply the deletions/edits.** Grep the Makefile for any other reference to the deleted targets (e.g. in `help`, `.PHONY`, or aggregate targets) and clean those too.
- [ ] **Step 3: Verify** — `make -n goscape goscape-cli test lint pack images helm-lint validate-example-configs check-format` all print sane recipes (lint may still require golangci-lint installed — `-n` only); `make goscape` builds; `make validate-example-configs` passes.
- [ ] **Step 4: Commit** — `chore(make): prune Loki-derived dead targets; derive check-format base from current branch` + arch-29.10 body + trailer.

---

### Task 11: Test debt — bridgesCtx mid-retry abort + ShutdownRace on the real Shutdown

**Files:**
- Test: `modules/world/logout_save_retry_test.go` (append), `modules/world/conn_handler_test.go` (replace `TestHandleConn_ShutdownRace`'s gate-slice mirror with the real `Shutdown`)

- [ ] **Step 1: Write the tests**

Append to `logout_save_retry_test.go`:

```go
// arch-29.11: cancelling bridgesCtx mid-retry must abort the loop
// promptly (shutdown's bridgesCancel path) instead of burning the
// remaining attempts.
func TestLogoutSaveAbortsOnBridgesCancel(t *testing.T) {
	block := make(chan struct{})
	fake := &blockingLoginClient{release: block} // PlayerLogout waits on ctx.Done, returns ctx.Err()
	s := newRetryTestServer(t, fake)
	s.logoutSaveRetryDelay = time.Hour // any retry sleep would hang the test — abort must preempt it
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.sendPlayerLogoutWithRetry("player_one", []byte{1})
	}()
	time.Sleep(20 * time.Millisecond)
	s.bridgesCancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("retry loop did not abort on bridgesCancel")
	}
	if got := fake.calls.Load(); got > 2 {
		t.Fatalf("calls after cancel: got %d, want <= 2", got)
	}
}
```

(`blockingLoginClient`: embed/copy the `flakyLoginClient` shape; its `PlayerLogout` does `<-ctx.Done(); return nil, ctx.Err()` and counts calls. Verify `newRetryTestServer`'s `newTestServer` exposes `bridgesCancel`; wire it if the fixture leaves it nil.)

Rewrite `TestHandleConn_ShutdownRace` (conn_handler_test.go:99-131) to use the real path: construct via `newChattyShutdownTestServer(t)` (server_shutdown_test.go:27-55 — it has a real listener + serveTCP), race 64 `HandleConn(eofConn{})` goroutines against one real `s.Shutdown()` call, keep the single `close(start)` release and the 5s guard. Delete the gate-slice mirror comment; the test now pins BOTH sides of the admission protocol (the prior version couldn't detect a future removal of Shutdown's gate usage — that was reviewer finding T3-minor in the Arc 28 ledger).

- [ ] **Step 2: Run** — new abort test must fail if the retry loop lacks prompt abort... it doesn't (the code already aborts): these are REGRESSION-PINNING tests, expected GREEN on first run. Prove non-vacuity instead: temporarily neuter the abort (`case <-s.bridgesCtx.Done():` branch removed) and confirm the abort test FAILS; temporarily remove `s.admissionGateMu.Lock()` from Shutdown's close(quit) and confirm the race test trips `-race` or the guard. Capture both RED outputs in the report, then restore.

- [ ] **Step 3: Verify** — both tests + `-race ./modules/world/ -run 'TestLogoutSave|TestHandleConn|TestShutdown'`; full world suite.

- [ ] **Step 4: Commit** — `test(world): pin bridgesCtx retry abort; race real Shutdown in HandleConn race test` + arch-29.11 body + trailer.

---

### Task 12: Documentation — PORTING.md arc row

**Files:**
- Modify: `docs/PORTING.md`

- [ ] **Step 1:** Confirm next arc number (`grep -n "^### Arc\|Arc 3[0-9]" docs/PORTING.md | tail`) — expected Arc 31 (Arc 30 = the arch-28 fix arc).
- [ ] **Step 2:** Append "Arc 31 — Architecture backlog arc" in the Arc-30 style: one row per fix (arch-29.1..29.11) with branch tip + merge SHA, the arch-28 OnDemand residual marked CLOSED by 29.1, the [BACKPORT-FIDELITY] tags recorded (29.1/29.2/29.3/29.4 are backport candidates; the rest are rev-274-only operational improvements per the no-forward-port policy), and the compat note from Task 5 (configs carrying `world.log_format`/`world.enable_tcp_server` now fail strict decode).
- [ ] **Step 3:** Commit — `docs(porting): record Arc 31 architecture backlog arc` + trailer.

---

## Execution notes

- Order: 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 → 9 → 10 → 11 → 12. Hard dependency: 8 consumes 7's `filestream.New`/`MakeCRCs` signatures. Task 3 changes the `LoginClient`/`FriendsClient` interfaces — its implementer must run the compile-all gate to catch every fake.
- Merge cadence: per-task branch → gates green → `--no-ff` into rev-274 → next task branches from updated rev-274.
- Backport pass (later, on request): tasks tagged [BACKPORT-FIDELITY] only, same worktree machinery as the arch-28 backport.
