# Friends-server bridge slice 5a — RELAY_* transport foundation

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add 9 unary outbound RELAY_* RPCs (Mute/Kick/Shutdown/Broadcast/Track/Reload/ClearLogins/ClearLogouts/QueueScript) + new per-world inbound stream RPC (`SubscribeWorldEvents`) + world-side `FriendsAdminBridge` outbound surface + `WorldEventsDispatcher` slog-only inbound surface, with the per-world subscriber owned by `world.Server`. Slice 5b will layer world-state actions onto the dispatcher; slice 5a is the transport-only foundation.

**Architecture:** Server-side: new `worldSubscriptions` registry keyed by `int32` worldId (mirrors slice-4a `subscriptions.go` shape). 9 handler methods route to target world; new `SubscribeWorldEvents` stream installs the per-world subscriber. World-side: `FriendsClient` interface gains 9 fire-and-forget outbound methods + 1 stream method; new `FriendsAdminBridge` shim wraps them; new `worldEventsSubscriber` (mirrors slice-4a `friendsSubscriber`) owns the per-world stream lifetime under `Server`. Default `slogWorldEventsDispatcher` logs each event at Info; no world-state effects.

**Tech Stack:** Go 1.x, gRPC (`pkg/friendspb` proto regenerated via `buf generate`), `log/slog`, existing `modules/world/friends_smoke_test.go` in-process e2e fixture, existing `fakeFriendsClient` test scaffold.

**Reference spec:** `docs/superpowers/specs/2026-05-23-friends-server-bridge-slice5a-design.md`

---

## Task 0: Prep — Makefile `.PHONY: proto` fix + extract `syncBuffer` helper

**Why first:** Two preflight chores that unblock the rest of the slice. (a) `make proto` shadows the `proto/` directory because the target isn't in `.PHONY`; T1 needs to regenerate the proto, so this must land first. (b) The dispatcher tests in T6 and T5 will need a mutex-wrapped buffer for slog polling, mirroring slice 4c's race fix; extracting `syncBuffer` once now means every future test that needs it can just use it.

**Files:**
- Modify: `Makefile` (line 68 `.PHONY` declaration)
- Create: `modules/world/world_test_util.go`
- Modify: `modules/world/tick_friends_login_test.go` (remove the local `syncBuffer` type)

- [ ] **Step 1: Add `proto` to `.PHONY` in Makefile**

In `Makefile` line 68, change:

```makefile
.PHONY: all images check-generated-files goscape goscape-debug goscape-cli lint test clean yacc protos
```

to:

```makefile
.PHONY: all images check-generated-files goscape goscape-debug goscape-cli lint test clean yacc protos proto
```

(Appending `proto` at the end. The existing `protos` is a different target — leave it.)

- [ ] **Step 2: Verify `make proto` works as a non-shadowed target**

Run:

```bash
make -n proto
```

Expected: prints `PATH="$(go env GOPATH)/bin:$PATH" buf generate` (the recipe body). Previously, with `proto/` as a directory, Make would consider the directory up-to-date and silently skip; now `.PHONY` forces rebuild.

- [ ] **Step 3: Create `modules/world/world_test_util.go`**

```go
package world

import (
	"bytes"
	"sync"
)

// syncBuffer wraps bytes.Buffer with a mutex so a test's polling
// goroutine and a callback's logging goroutine don't race on the
// underlying buffer state. Extracted from tick_friends_login_test.go
// (slice 4c T3) for re-use across world-package tests.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}
```

- [ ] **Step 4: Remove the local `syncBuffer` type from `tick_friends_login_test.go`**

In `modules/world/tick_friends_login_test.go`, delete lines 14-33 (the comment block + type + Write + String methods). Verify that the `import` block still has `bytes` and `sync` only if other tests in that file use them — currently only `syncBuffer` used `sync.Mutex`, so the `sync` import is now unused. Remove `sync` from the imports if it's not referenced elsewhere in the file. Same check for `bytes` — if `syncBuffer` is the only consumer, drop it too.

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/world/...
```

Expected: no errors. If `go vet` complains about unused imports, drop them.

- [ ] **Step 5: Run world-package tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -count=1
```

Expected: PASS. The extraction is purely structural — `syncBuffer` is identical except for location, and `tick_friends_login_test.go` reaches it via the same package.

- [ ] **Step 6: Commit**

```bash
git add Makefile modules/world/world_test_util.go modules/world/tick_friends_login_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
prep slice 5a: Makefile proto .PHONY + extract syncBuffer (T0)

- Add `proto` to Makefile .PHONY so the recipe runs (previously shadowed
  by the proto/ directory; latent since slice 1).
- Extract syncBuffer from tick_friends_login_test.go to world_test_util.go
  for re-use by slice-5a dispatcher/subscriber tests.
EOF
)"
```

---

## Task 1: Proto changes — 9 RELAY_* RPCs + SubscribeWorldEvents + messages + regenerate

**Why this task:** Lays the wire contract. After this task, `pkg/friendspb` has the new methods, server-side `friendspb.UnimplementedFriendsServiceServer` covers them (returning Unimplemented on call), and downstream tasks can add real impls.

**Files:**
- Modify: `proto/friends/friends.proto`
- Regenerate: `pkg/friendspb/friends.pb.go`, `pkg/friendspb/friends_grpc.pb.go`

- [ ] **Step 1: Append RELAY_* RPC declarations to `service FriendsService`**

In `proto/friends/friends.proto`, after the `rpc SubscribeUpdates(...)` line (currently line 33), add:

```proto

  // Cross-world admin relay (slice 5a). Each RPC accepts a target_world_id;
  // the server forwards a WorldEvent to that world's SubscribeWorldEvents
  // stream. No-op if no world is subscribed for target_world_id (matches
  // TS FriendServer.ts:298-302 `if (typeof this.socketByWorld[nodeId] !== 'undefined')`).
  // Slice 5a accepts and routes; slice 5b applies world-state effects.
  // NAI-S5A-D-NO-ADMIN-AUTH-AT-SERVER — friends-server is dumb routing;
  // admin checks live on both sender and receiver world.
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
  // NAI-S5A-D-PERWORLD-EVENTS-STREAM-SEPARATE — second stream RPC
  // alongside SubscribeUpdates; goscape has two parallel streams where
  // TS has one socket. Permanent; reviewer traceability only.
  rpc SubscribeWorldEvents(SubscribeWorldEventsRequest) returns (stream WorldEvent);
```

- [ ] **Step 2: Append request messages at end of file**

At the end of `proto/friends/friends.proto` (after the existing messages), append:

```proto

// Slice 5a RELAY_* request messages. Each carries target_world_id (the
// world that should RECEIVE the event) plus opcode-specific payload.

message RelayMuteRequest {
  int32  target_world_id = 1;
  uint64 username37      = 2;
  // Mute expiry as epoch milliseconds (matches TS `muted_until: number`).
  // 0 = unmute. Negative = permanent (matches existing modules/login
  // PlayerMute semantics).
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
  // TS-faithful `state` (FriendServer.ts:348). Untyped in TS; pinned as
  // int32 so slice 5b can interpret per the anti-cheat tracking subsystem.
  int32  state           = 3;
}

message RelayReloadRequest {
  int32 target_world_id = 1;
}

message RelayClearLoginsRequest {
  int32 target_world_id = 1;
}

message RelayClearLogoutsRequest {
  int32 target_world_id = 1;
}

message RelayQueueScriptRequest {
  int32  target_world_id = 1;
  string script_name     = 2;
  uint64 username37      = 3;
}

message SubscribeWorldEventsRequest {
  int32 world_id = 1;
}

// WorldEvent is the inbound push variant pushed to a world's
// SubscribeWorldEvents stream. Each variant strips target_world_id from
// the corresponding Relay*Request: the receiving world already knows
// its own ID, and the routing field is irrelevant to the action layer.
message WorldEvent {
  oneof event {
    MuteEvent           mute          = 1;
    KickEvent           kick          = 2;
    ShutdownEvent       shutdown      = 3;
    BroadcastEvent      broadcast     = 4;
    TrackEvent          track         = 5;
    ReloadEvent         reload        = 6;
    ClearLoginsEvent    clear_logins  = 7;
    ClearLogoutsEvent   clear_logouts = 8;
    QueueScriptEvent    queue_script  = 9;
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

- [ ] **Step 3: Regenerate Go bindings**

Run:

```bash
make proto
```

Expected: regenerates `pkg/friendspb/friends.pb.go` and `pkg/friendspb/friends_grpc.pb.go` without errors. If `buf` is missing, install it: `go install github.com/bufbuild/buf/cmd/buf@latest` then retry.

- [ ] **Step 4: Verify the generated code compiles**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./pkg/friendspb/...
```

Expected: no errors.

- [ ] **Step 5: Verify dependent packages still build (server stubs land as Unimplemented)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...
```

Expected: clean build. The existing `handler` struct embeds `friendspb.UnimplementedFriendsServiceServer`, which provides default `Unimplemented`-returning impls for the new methods. The world-side `FriendsClient` interface does NOT yet include the new methods, so production code is unaware.

- [ ] **Step 6: Verify existing tests still pass**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... -count=1
```

Expected: PASS — slice 5a is purely additive at the proto layer.

- [ ] **Step 7: Commit**

```bash
git add proto/friends/friends.proto pkg/friendspb/friends.pb.go pkg/friendspb/friends_grpc.pb.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: proto slice 5a — RELAY_* RPCs + SubscribeWorldEvents stream (T1)

9 unary RELAY_* RPCs (Mute/Kick/Shutdown/Broadcast/Track/Reload/
ClearLogins/ClearLogouts/QueueScript) + new SubscribeWorldEvents
per-world stream + WorldEvent oneof. Opens
NAI-S5A-D-PERWORLD-EVENTS-STREAM-SEPARATE (permanent),
NAI-S5A-D-NO-ADMIN-AUTH-AT-SERVER (permanent).
EOF
)"
```

---

## Task 2: Server-side per-world subscription registry

**Why this task:** The handler methods in T3 send events through this registry; the SubscribeWorldEvents handler installs subscribers into it. Build the registry standalone first with full unit-test coverage, then wire it.

**Files:**
- Create: `modules/friends/world_subscriptions.go`
- Create: `modules/friends/world_subscriptions_test.go`

- [ ] **Step 1: Write the failing tests**

Create `modules/friends/world_subscriptions_test.go`:

```go
package friends

import (
	"testing"

	"github.com/zsrv/goscape/pkg/friendspb"
)

func TestWorldSubscriptions_RegisterDeregister(t *testing.T) {
	s := newWorldSubscriptions(noopLogger())
	sub := newWorldSubscriber(1)
	s.register(sub)
	// Send routes to the registered subscriber.
	ev := &friendspb.WorldEvent{Event: &friendspb.WorldEvent_Reload{Reload: &friendspb.ReloadEvent{}}}
	s.send(1, ev)
	select {
	case got := <-sub.ch:
		if got != ev {
			t.Fatalf("got %v, want %v", got, ev)
		}
	default:
		t.Fatal("expected event in channel; got none")
	}
	s.deregister(sub)
	// Now send is a silent no-op (no subscriber).
	s.send(1, ev)
	select {
	case <-sub.ch:
		t.Fatal("expected no event after deregister")
	default:
	}
}

func TestWorldSubscriptions_DupRegisterKicksPrior(t *testing.T) {
	s := newWorldSubscriptions(noopLogger())
	prior := newWorldSubscriber(1)
	s.register(prior)
	next := newWorldSubscriber(1)
	s.register(next)
	// Prior's done is closed; next is current.
	select {
	case <-prior.done:
	default:
		t.Fatal("expected prior.done to be closed by register-on-conflict")
	}
	ev := &friendspb.WorldEvent{Event: &friendspb.WorldEvent_Reload{Reload: &friendspb.ReloadEvent{}}}
	s.send(1, ev)
	select {
	case <-next.ch:
	default:
		t.Fatal("expected event to route to next, not prior")
	}
	select {
	case <-prior.ch:
		t.Fatal("event routed to prior; should have gone to next only")
	default:
	}
}

func TestWorldSubscriptions_DeregisterIdentityChecked(t *testing.T) {
	s := newWorldSubscriptions(noopLogger())
	prior := newWorldSubscriber(1)
	s.register(prior)
	next := newWorldSubscriber(1)
	s.register(next) // kicks prior
	// Deregistering the (now-stale) prior must NOT remove next.
	s.deregister(prior)
	// next must still be registered: send routes to it.
	ev := &friendspb.WorldEvent{Event: &friendspb.WorldEvent_Reload{Reload: &friendspb.ReloadEvent{}}}
	s.send(1, ev)
	select {
	case <-next.ch:
	default:
		t.Fatal("expected event after stale deregister; got none")
	}
}

func TestWorldSubscriptions_SendNoSubscriberSilent(t *testing.T) {
	s := newWorldSubscriptions(noopLogger())
	ev := &friendspb.WorldEvent{Event: &friendspb.WorldEvent_Reload{Reload: &friendspb.ReloadEvent{}}}
	// Should not panic, should not block.
	s.send(42, ev)
}

func TestWorldSubscriptions_DropOnFull(t *testing.T) {
	s := newWorldSubscriptions(noopLogger())
	sub := newWorldSubscriber(1)
	s.register(sub)
	ev := &friendspb.WorldEvent{Event: &friendspb.WorldEvent_Reload{Reload: &friendspb.ReloadEvent{}}}
	// Fill the buffer.
	for range worldSubscriberBufferSize {
		s.send(1, ev)
	}
	// Overflow event is dropped (not blocking the caller).
	s.send(1, ev)
	// Drain to verify exactly worldSubscriberBufferSize events queued.
	got := 0
	drain:
	for {
		select {
		case <-sub.ch:
			got++
		default:
			break drain
		}
	}
	if got != worldSubscriberBufferSize {
		t.Fatalf("got %d events, want %d", got, worldSubscriberBufferSize)
	}
}
```

- [ ] **Step 2: Verify tests fail to compile (type does not exist)**

Run:

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/... -run TestWorldSubscriptions_ -count=1
```

Expected: FAIL — `newWorldSubscriptions`, `newWorldSubscriber`, `worldSubscriberBufferSize`, `WorldEvent_Reload`, `ReloadEvent` may or may not exist. The proto types (`WorldEvent`, `ReloadEvent`, `WorldEvent_Reload`) exist from T1; the registry types don't.

- [ ] **Step 3: Create `modules/friends/world_subscriptions.go`**

```go
package friends

import (
	"log/slog"
	"sync"

	"github.com/zsrv/goscape/pkg/friendspb"
)

// worldSubscriberBufferSize is the per-world-subscriber channel buffer.
// Same posture as subscriberBufferSize from subscriptions.go but a
// separate constant in case admin-burst rate differs from per-player
// update rate.
//
// NAI-S5A-D-WORLDEVENTS-DROP-ON-FULL — overflowing the buffer drops the
// newest event with a Warn log instead of blocking the RPC handler.
const worldSubscriberBufferSize = 64

// worldSubscriber owns one open SubscribeWorldEvents stream for one
// world ID. ch is written by RELAY_* handler methods; the gRPC stream
// goroutine drains ch and calls stream.Send. done is closed by a
// duplicate register to signal the prior stream goroutine to exit.
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
	by  map[int32]*worldSubscriber // worldId -> subscriber
	log *slog.Logger
}

func newWorldSubscriptions(log *slog.Logger) *worldSubscriptions {
	return &worldSubscriptions{
		by:  make(map[int32]*worldSubscriber),
		log: log,
	}
}

// register installs sub under sub.worldId. If a prior subscriber exists
// for the same worldId, it is kicked (its done is closed) before sub
// replaces it.
func (s *worldSubscriptions) register(sub *worldSubscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if prior, ok := s.by[sub.worldId]; ok {
		close(prior.done)
	}
	s.by[sub.worldId] = sub
}

// deregister removes sub from the registry IFF it is still the currently
// registered subscriber for sub.worldId (a rapid re-subscribe may have
// replaced it under register).
func (s *worldSubscriptions) deregister(sub *worldSubscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cur, ok := s.by[sub.worldId]; ok && cur == sub {
		delete(s.by, sub.worldId)
	}
}

// send pushes ev to the subscriber for worldId (no-op if none).
// Non-blocking; on full channel, logs warn and drops the event.
func (s *worldSubscriptions) send(worldId int32, ev *friendspb.WorldEvent) {
	s.mu.Lock()
	sub, ok := s.by[worldId]
	s.mu.Unlock()
	if !ok {
		return
	}
	select {
	case sub.ch <- ev:
	default:
		s.log.Warn("world events subscriber buffer full; dropping event",
			slog.Int("world_id", int(worldId)))
	}
}
```

- [ ] **Step 4: Run the tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/... -run TestWorldSubscriptions_ -race -count=1
```

Expected: PASS — all 5 tests green.

- [ ] **Step 5: Commit**

```bash
git add modules/friends/world_subscriptions.go modules/friends/world_subscriptions_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: server-side worldSubscriptions registry (T2)

Per-world subscriber registry keyed by int32 worldId. Mirrors slice-4a
subscriptions.go shape. Single subscriber per worldId; re-subscribe
kicks prior. Non-blocking send with drop-newest-on-full.

NAI-S5A-D-WORLDEVENTS-DROP-ON-FULL noted on the buffer-size constant.
EOF
)"
```

---

## Task 3: Server-side handler — 9 Relay* methods + SubscribeWorldEvents stream

**Why this task:** Wires the proto methods through to the registry. After this task the friends-server is functionally complete for slice 5a; the rest of the slice is world-side.

**Files:**
- Modify: `modules/friends/handler.go` (add `worldSubs` field; add 9 Relay* methods + SubscribeWorldEvents)
- Modify: `modules/friends/server.go` (pass `worldSubs` into handler)
- Modify: `modules/friends/friends.go` (construct `worldSubs` in starting; pass to newGRPCServer)
- Modify: `modules/friends/handler_test.go` (9 routing pins + dup-kick + no-subscriber-silent)
- Modify: `modules/friends/world_subscriptions_test.go` (no change — registry tests already cover the lower layer)

- [ ] **Step 1: Write the failing handler tests**

In `modules/friends/handler_test.go`, append (preserving the existing test-helper conventions — check the file for the existing `newTestHandler(t)` or equivalent harness; if there isn't one, use the `newHandlerWithRepo` pattern from existing tests):

```go
// --- slice 5a: RELAY_* handler routing tests ---

func TestHandler_RelayKick_RoutesToSubscriber(t *testing.T) {
	h, _, worldSubs := newTestHandlerWithWorldSubs(t)
	sub := newWorldSubscriber(2)
	worldSubs.register(sub)
	defer worldSubs.deregister(sub)

	_, err := h.RelayKick(context.Background(), &friendspb.RelayKickRequest{
		TargetWorldId: 2,
		Username37:    nameToBase37("alice"),
	})
	if err != nil {
		t.Fatalf("RelayKick: %v", err)
	}
	select {
	case ev := <-sub.ch:
		kick := ev.GetKick()
		if kick == nil {
			t.Fatalf("got event variant %T, want Kick", ev.Event)
		}
		if kick.Username37 != nameToBase37("alice") {
			t.Fatalf("kick.Username37 = %d, want %d", kick.Username37, nameToBase37("alice"))
		}
	default:
		t.Fatal("expected KickEvent on subscriber channel")
	}
}

func TestHandler_RelayKick_NoSubscriberSilent(t *testing.T) {
	h, _, _ := newTestHandlerWithWorldSubs(t)
	// No subscriber registered for world 99.
	_, err := h.RelayKick(context.Background(), &friendspb.RelayKickRequest{
		TargetWorldId: 99,
		Username37:    nameToBase37("alice"),
	})
	if err != nil {
		t.Fatalf("RelayKick silent-drop expected OK; got %v", err)
	}
}

func TestHandler_RelayMute_RoutesPayload(t *testing.T) {
	h, _, worldSubs := newTestHandlerWithWorldSubs(t)
	sub := newWorldSubscriber(2)
	worldSubs.register(sub)
	defer worldSubs.deregister(sub)
	_, err := h.RelayMute(context.Background(), &friendspb.RelayMuteRequest{
		TargetWorldId: 2, Username37: nameToBase37("bob"), MutedUntilMs: 12345,
	})
	if err != nil {
		t.Fatalf("RelayMute: %v", err)
	}
	ev := mustRecvWorldEvent(t, sub)
	mute := ev.GetMute()
	if mute == nil || mute.Username37 != nameToBase37("bob") || mute.MutedUntilMs != 12345 {
		t.Fatalf("mute payload mismatch: %+v", mute)
	}
}

func TestHandler_RelayShutdown_RoutesPayload(t *testing.T) {
	h, _, worldSubs := newTestHandlerWithWorldSubs(t)
	sub := newWorldSubscriber(2)
	worldSubs.register(sub)
	defer worldSubs.deregister(sub)
	_, err := h.RelayShutdown(context.Background(), &friendspb.RelayShutdownRequest{
		TargetWorldId: 2, DurationTicks: 50,
	})
	if err != nil {
		t.Fatalf("RelayShutdown: %v", err)
	}
	ev := mustRecvWorldEvent(t, sub)
	if sd := ev.GetShutdown(); sd == nil || sd.DurationTicks != 50 {
		t.Fatalf("shutdown payload mismatch: %+v", sd)
	}
}

func TestHandler_RelayBroadcast_RoutesPayload(t *testing.T) {
	h, _, worldSubs := newTestHandlerWithWorldSubs(t)
	sub := newWorldSubscriber(2)
	worldSubs.register(sub)
	defer worldSubs.deregister(sub)
	_, err := h.RelayBroadcast(context.Background(), &friendspb.RelayBroadcastRequest{
		TargetWorldId: 2, Message: "hello world",
	})
	if err != nil {
		t.Fatalf("RelayBroadcast: %v", err)
	}
	ev := mustRecvWorldEvent(t, sub)
	if bc := ev.GetBroadcast(); bc == nil || bc.Message != "hello world" {
		t.Fatalf("broadcast payload mismatch: %+v", bc)
	}
}

func TestHandler_RelayTrack_RoutesPayload(t *testing.T) {
	h, _, worldSubs := newTestHandlerWithWorldSubs(t)
	sub := newWorldSubscriber(2)
	worldSubs.register(sub)
	defer worldSubs.deregister(sub)
	_, err := h.RelayTrack(context.Background(), &friendspb.RelayTrackRequest{
		TargetWorldId: 2, Username37: nameToBase37("carol"), State: 1,
	})
	if err != nil {
		t.Fatalf("RelayTrack: %v", err)
	}
	ev := mustRecvWorldEvent(t, sub)
	tk := ev.GetTrack()
	if tk == nil || tk.Username37 != nameToBase37("carol") || tk.State != 1 {
		t.Fatalf("track payload mismatch: %+v", tk)
	}
}

func TestHandler_RelayReload_RoutesEmpty(t *testing.T) {
	h, _, worldSubs := newTestHandlerWithWorldSubs(t)
	sub := newWorldSubscriber(2)
	worldSubs.register(sub)
	defer worldSubs.deregister(sub)
	_, err := h.RelayReload(context.Background(), &friendspb.RelayReloadRequest{TargetWorldId: 2})
	if err != nil {
		t.Fatalf("RelayReload: %v", err)
	}
	ev := mustRecvWorldEvent(t, sub)
	if ev.GetReload() == nil {
		t.Fatalf("expected Reload variant; got %T", ev.Event)
	}
}

func TestHandler_RelayClearLogins_RoutesEmpty(t *testing.T) {
	h, _, worldSubs := newTestHandlerWithWorldSubs(t)
	sub := newWorldSubscriber(2)
	worldSubs.register(sub)
	defer worldSubs.deregister(sub)
	_, err := h.RelayClearLogins(context.Background(), &friendspb.RelayClearLoginsRequest{TargetWorldId: 2})
	if err != nil {
		t.Fatalf("RelayClearLogins: %v", err)
	}
	if ev := mustRecvWorldEvent(t, sub); ev.GetClearLogins() == nil {
		t.Fatalf("expected ClearLogins variant; got %T", ev.Event)
	}
}

func TestHandler_RelayClearLogouts_RoutesEmpty(t *testing.T) {
	h, _, worldSubs := newTestHandlerWithWorldSubs(t)
	sub := newWorldSubscriber(2)
	worldSubs.register(sub)
	defer worldSubs.deregister(sub)
	_, err := h.RelayClearLogouts(context.Background(), &friendspb.RelayClearLogoutsRequest{TargetWorldId: 2})
	if err != nil {
		t.Fatalf("RelayClearLogouts: %v", err)
	}
	if ev := mustRecvWorldEvent(t, sub); ev.GetClearLogouts() == nil {
		t.Fatalf("expected ClearLogouts variant; got %T", ev.Event)
	}
}

func TestHandler_RelayQueueScript_RoutesPayload(t *testing.T) {
	h, _, worldSubs := newTestHandlerWithWorldSubs(t)
	sub := newWorldSubscriber(2)
	worldSubs.register(sub)
	defer worldSubs.deregister(sub)
	_, err := h.RelayQueueScript(context.Background(), &friendspb.RelayQueueScriptRequest{
		TargetWorldId: 2, ScriptName: "debug:dump", Username37: nameToBase37("dan"),
	})
	if err != nil {
		t.Fatalf("RelayQueueScript: %v", err)
	}
	ev := mustRecvWorldEvent(t, sub)
	qs := ev.GetQueueScript()
	if qs == nil || qs.ScriptName != "debug:dump" || qs.Username37 != nameToBase37("dan") {
		t.Fatalf("queue_script payload mismatch: %+v", qs)
	}
}
```

Append the two test helpers at the end of `handler_test.go`:

```go
// newTestHandlerWithWorldSubs constructs a handler with both per-player
// and per-world subscription registries wired up. Returns (handler, subs,
// worldSubs). Re-uses the test-DB helper pattern used by the existing
// handler tests.
func newTestHandlerWithWorldSubs(t *testing.T) (*handler, *subscriptions, *worldSubscriptions) {
	t.Helper()
	db := openMemDB(t)
	repo := NewRepository(db, "main")
	subs := newSubscriptions(noopLogger())
	worldSubs := newWorldSubscriptions(noopLogger())
	h := &handler{
		repo:      repo,
		subs:      subs,
		worldSubs: worldSubs,
		cfg:       Config{NodeProfile: "main", WorldPlayerLimit: 2000},
		log:       noopLogger(),
	}
	return h, subs, worldSubs
}

// mustRecvWorldEvent reads one event from sub.ch with a short timeout
// (helpers in this file use 1s for similar drains). Fails the test if
// no event arrives.
func mustRecvWorldEvent(t *testing.T, sub *worldSubscriber) *friendspb.WorldEvent {
	t.Helper()
	select {
	case ev := <-sub.ch:
		return ev
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for world event")
		return nil
	}
}
```

If `openMemDB`, `noopLogger`, `nameToBase37`, and `time` import are not already present in `handler_test.go`, check the existing test file for equivalents. If the existing helpers are named differently (e.g. `newTestHandler(t)`), update both `newTestHandlerWithWorldSubs` and `mustRecvWorldEvent` to match the existing conventions — keep the new helpers compatible with the existing harness.

- [ ] **Step 2: Verify tests fail to compile**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/... -run TestHandler_Relay -count=1
```

Expected: FAIL — handler has no `worldSubs` field; methods `RelayMute`/etc. don't exist on handler (the embedded `UnimplementedFriendsServiceServer` provides Unimplemented impls, but the field reference in the helper fails to compile).

- [ ] **Step 3: Add `worldSubs` field to `handler` struct**

In `modules/friends/handler.go`, modify the `handler` struct (currently lines 13-22) to add `worldSubs *worldSubscriptions`:

```go
type handler struct {
	friendspb.UnimplementedFriendsServiceServer

	repo      *Repository
	subs      *subscriptions
	worldSubs *worldSubscriptions
	cfg       Config
	log       *slog.Logger
}
```

- [ ] **Step 4: Add 9 RELAY_* handler methods to `handler.go`**

At the end of `modules/friends/handler.go` (after `sendPlayerWorldUpdate`), append:

```go

// --- slice 5a: RELAY_* admin relay handlers ---
//
// All Relay* methods forward a WorldEvent to the target world's
// SubscribeWorldEvents subscriber. No-op if no world is subscribed for
// req.TargetWorldId (matches TS FriendServer.ts:298-302 silent-drop on
// missing socketByWorld). No auth check at this layer.
//
// NAI-S5A-D-NO-ADMIN-AUTH-AT-SERVER — friends-server is dumb routing;
//   admin checks live on both sender and receiver world. Permanent.
// NAI-S5A-D-DISPATCHER-NO-ACTION — slice 5a default WorldEventsDispatcher
//   on the receiving side logs only; slice 5b retires this piecewise as
//   each opcode's world-state action is wired.

func (h *handler) RelayMute(_ context.Context, req *friendspb.RelayMuteRequest) (*emptypb.Empty, error) {
	h.worldSubs.send(req.TargetWorldId, &friendspb.WorldEvent{
		Event: &friendspb.WorldEvent_Mute{Mute: &friendspb.MuteEvent{
			Username37:   req.Username37,
			MutedUntilMs: req.MutedUntilMs,
		}},
	})
	return &emptypb.Empty{}, nil
}

func (h *handler) RelayKick(_ context.Context, req *friendspb.RelayKickRequest) (*emptypb.Empty, error) {
	h.worldSubs.send(req.TargetWorldId, &friendspb.WorldEvent{
		Event: &friendspb.WorldEvent_Kick{Kick: &friendspb.KickEvent{Username37: req.Username37}},
	})
	return &emptypb.Empty{}, nil
}

func (h *handler) RelayShutdown(_ context.Context, req *friendspb.RelayShutdownRequest) (*emptypb.Empty, error) {
	h.worldSubs.send(req.TargetWorldId, &friendspb.WorldEvent{
		Event: &friendspb.WorldEvent_Shutdown{Shutdown: &friendspb.ShutdownEvent{DurationTicks: req.DurationTicks}},
	})
	return &emptypb.Empty{}, nil
}

func (h *handler) RelayBroadcast(_ context.Context, req *friendspb.RelayBroadcastRequest) (*emptypb.Empty, error) {
	h.worldSubs.send(req.TargetWorldId, &friendspb.WorldEvent{
		Event: &friendspb.WorldEvent_Broadcast{Broadcast: &friendspb.BroadcastEvent{Message: req.Message}},
	})
	return &emptypb.Empty{}, nil
}

func (h *handler) RelayTrack(_ context.Context, req *friendspb.RelayTrackRequest) (*emptypb.Empty, error) {
	h.worldSubs.send(req.TargetWorldId, &friendspb.WorldEvent{
		Event: &friendspb.WorldEvent_Track{Track: &friendspb.TrackEvent{Username37: req.Username37, State: req.State}},
	})
	return &emptypb.Empty{}, nil
}

func (h *handler) RelayReload(_ context.Context, req *friendspb.RelayReloadRequest) (*emptypb.Empty, error) {
	h.worldSubs.send(req.TargetWorldId, &friendspb.WorldEvent{
		Event: &friendspb.WorldEvent_Reload{Reload: &friendspb.ReloadEvent{}},
	})
	return &emptypb.Empty{}, nil
}

func (h *handler) RelayClearLogins(_ context.Context, req *friendspb.RelayClearLoginsRequest) (*emptypb.Empty, error) {
	h.worldSubs.send(req.TargetWorldId, &friendspb.WorldEvent{
		Event: &friendspb.WorldEvent_ClearLogins{ClearLogins: &friendspb.ClearLoginsEvent{}},
	})
	return &emptypb.Empty{}, nil
}

func (h *handler) RelayClearLogouts(_ context.Context, req *friendspb.RelayClearLogoutsRequest) (*emptypb.Empty, error) {
	h.worldSubs.send(req.TargetWorldId, &friendspb.WorldEvent{
		Event: &friendspb.WorldEvent_ClearLogouts{ClearLogouts: &friendspb.ClearLogoutsEvent{}},
	})
	return &emptypb.Empty{}, nil
}

func (h *handler) RelayQueueScript(_ context.Context, req *friendspb.RelayQueueScriptRequest) (*emptypb.Empty, error) {
	h.worldSubs.send(req.TargetWorldId, &friendspb.WorldEvent{
		Event: &friendspb.WorldEvent_QueueScript{QueueScript: &friendspb.QueueScriptEvent{
			ScriptName: req.ScriptName,
			Username37: req.Username37,
		}},
	})
	return &emptypb.Empty{}, nil
}

// SubscribeWorldEvents streams server -> world admin events for one
// world. One subscriber per worldId; re-subscribe terminates the prior.
// Slice 5a opens the stream; slice 5b layers world-state action handlers
// on the world side via WorldEventsDispatcher.
//
// Replaces the slice-1 codes.Unimplemented stub.
func (h *handler) SubscribeWorldEvents(req *friendspb.SubscribeWorldEventsRequest, stream friendspb.FriendsService_SubscribeWorldEventsServer) error {
	sub := newWorldSubscriber(req.WorldId)
	h.worldSubs.register(sub)
	defer h.worldSubs.deregister(sub)

	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-sub.done:
			return nil
		case ev := <-sub.ch:
			if err := stream.Send(ev); err != nil {
				return err
			}
		}
	}
}
```

- [ ] **Step 5: Update `newGRPCServer` to accept and pass `worldSubs`**

In `modules/friends/server.go` lines 20-29, change:

```go
func newGRPCServer(cfg Config, repo *Repository, subs *subscriptions, log *slog.Logger) *grpcServer {
	s := grpc.NewServer()
	friendspb.RegisterFriendsServiceServer(s, &handler{
		repo: repo,
		subs: subs,
		cfg:  cfg,
		log:  log,
	})
	return &grpcServer{server: s, log: log}
}
```

to:

```go
func newGRPCServer(cfg Config, repo *Repository, subs *subscriptions, worldSubs *worldSubscriptions, log *slog.Logger) *grpcServer {
	s := grpc.NewServer()
	friendspb.RegisterFriendsServiceServer(s, &handler{
		repo:      repo,
		subs:      subs,
		worldSubs: worldSubs,
		cfg:       cfg,
		log:       log,
	})
	return &grpcServer{server: s, log: log}
}
```

- [ ] **Step 6: Wire `worldSubs` into `Friends.starting`**

In `modules/friends/friends.go`, modify the `Friends` struct (line 15-26) to add a `worldSubs` field, then the `starting` method (lines 43-62) to construct and pass it:

```go
type Friends struct {
	services.Service

	cfg Config
	log *slog.Logger

	db        *sql.DB
	repo      *Repository
	subs      *subscriptions
	worldSubs *worldSubscriptions
	srv       *grpcServer
	lis       net.Listener
}
```

```go
func (f *Friends) starting(_ context.Context) error {
	db, err := openDB(f.cfg.SQLiteDSN)
	if err != nil {
		return fmt.Errorf("open friends db: %w", err)
	}
	repo := NewRepository(db, f.cfg.NodeProfile)
	subs := newSubscriptions(f.log)
	worldSubs := newWorldSubscriptions(f.log)
	srv := newGRPCServer(f.cfg, repo, subs, worldSubs, f.log)
	lis, err := srv.listen(f.cfg)
	if err != nil {
		db.Close()
		return err
	}
	f.db = db
	f.repo = repo
	f.subs = subs
	f.worldSubs = worldSubs
	f.srv = srv
	f.lis = lis
	return nil
}
```

- [ ] **Step 7: Run the tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/... -race -count=1
```

Expected: PASS — 10 new tests (1 per RELAY method + 1 silent-drop), all existing friends tests still pass.

- [ ] **Step 8: Add SubscribeWorldEvents dup-kick test**

Append to `modules/friends/handler_test.go`:

```go
func TestHandler_SubscribeWorldEvents_DupKicksPrior(t *testing.T) {
	h, _, _ := newTestHandlerWithWorldSubs(t)

	// Open the first subscription in a goroutine; capture its EOF via
	// the stream's err return.
	srv1 := newFakeWorldEventsServerStream(t)
	done1 := make(chan error, 1)
	go func() { done1 <- h.SubscribeWorldEvents(&friendspb.SubscribeWorldEventsRequest{WorldId: 7}, srv1) }()

	// Spin until srv1 has installed its subscriber (registry has entry).
	waitFor(t, func() bool {
		h.worldSubs.mu.Lock()
		defer h.worldSubs.mu.Unlock()
		return h.worldSubs.by[7] != nil
	})

	// Open a second subscription for the same worldId.
	srv2 := newFakeWorldEventsServerStream(t)
	done2 := make(chan error, 1)
	go func() { done2 <- h.SubscribeWorldEvents(&friendspb.SubscribeWorldEventsRequest{WorldId: 7}, srv2) }()

	// srv1's stream goroutine sees done closed and returns nil.
	select {
	case err := <-done1:
		if err != nil {
			t.Fatalf("prior stream returned %v; want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for prior stream to exit")
	}

	// Close srv2's ctx to terminate the second stream.
	srv2.cancel()
	<-done2
}
```

Append the helper `fakeWorldEventsServerStream` and `waitFor` at the end of `handler_test.go` (if `waitFor` already exists in the file, skip its definition):

```go
// fakeWorldEventsServerStream is a test impl of
// friendspb.FriendsService_SubscribeWorldEventsServer.
type fakeWorldEventsServerStream struct {
	grpc.ServerStream
	ctx    context.Context
	cancel context.CancelFunc
	sent   chan *friendspb.WorldEvent
}

func newFakeWorldEventsServerStream(t *testing.T) *fakeWorldEventsServerStream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	return &fakeWorldEventsServerStream{
		ctx:    ctx,
		cancel: cancel,
		sent:   make(chan *friendspb.WorldEvent, 16),
	}
}

func (s *fakeWorldEventsServerStream) Send(ev *friendspb.WorldEvent) error {
	s.sent <- ev
	return nil
}
func (s *fakeWorldEventsServerStream) Context() context.Context { return s.ctx }

// waitFor polls cond at 10ms intervals up to 2s.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("waitFor: condition not met within 2s")
}
```

If `grpc` is not yet imported in `handler_test.go`, add `"google.golang.org/grpc"` to the imports.

- [ ] **Step 9: Run all friends tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/... -race -count=1 -timeout 60s
```

Expected: PASS.

- [ ] **Step 10: Run full project tests (-race) to catch downstream regressions**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... -count=1 -timeout 600s
```

Expected: PASS — no other package is affected by the handler additions yet (the FriendsClient interface in world is unchanged so far).

- [ ] **Step 11: Commit**

```bash
git add modules/friends/handler.go modules/friends/handler_test.go modules/friends/server.go modules/friends/friends.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: server-side RELAY_* handlers + SubscribeWorldEvents stream (T3)

9 unary Relay* handlers forward to target world's worldSubscriptions
entry; missing subscriber = silent drop (TS-faithful). New
SubscribeWorldEvents stream handler with dup-subscriber kick semantics.

10 routing pins (1 per method + silent-drop) + 1 dup-kick test. Wires
worldSubs through newGRPCServer + Friends.starting.
EOF
)"
```

---

## Task 4: World-side `FriendsClient` interface + fake + grpc impls + failure-logging test

**Why this task:** Drops the 9 outbound methods + 1 stream method onto the world's `FriendsClient` interface, updates the fake to compile, implements the production grpc shim, and pins the failure-logging contract via a table-driven test.

**Files:**
- Modify: `modules/world/friends_client.go` (interface + grpcFriendsClient impls)
- Modify: `modules/world/friends_client_fake_test.go` (fake impls + stream fake)
- Modify: `modules/world/friends_client_test.go` (add `TestGRPCFriendsClient_Relay_LogsErrorOnFailure`)

- [ ] **Step 1: Write the failing table-driven failure-logging test**

In `modules/world/friends_client_test.go`, append:

```go
// TestGRPCFriendsClient_Relay_LogsErrorOnFailure exercises each Relay*
// RPC path under a forced-error gRPC client and asserts the production
// fire-and-forget impl logs warn + swallows the error. Table-driven to
// keep the 9 cases concise.
func TestGRPCFriendsClient_Relay_LogsErrorOnFailure(t *testing.T) {
	cases := []struct {
		name string
		op   string // substring expected in the warn log
		call func(c *grpcFriendsClient, ctx context.Context)
	}{
		{"RelayMute", "RelayMute", func(c *grpcFriendsClient, ctx context.Context) {
			c.RelayMute(ctx, &friendspb.RelayMuteRequest{TargetWorldId: 2, Username37: 1, MutedUntilMs: 0})
		}},
		{"RelayKick", "RelayKick", func(c *grpcFriendsClient, ctx context.Context) {
			c.RelayKick(ctx, &friendspb.RelayKickRequest{TargetWorldId: 2, Username37: 1})
		}},
		{"RelayShutdown", "RelayShutdown", func(c *grpcFriendsClient, ctx context.Context) {
			c.RelayShutdown(ctx, &friendspb.RelayShutdownRequest{TargetWorldId: 2, DurationTicks: 0})
		}},
		{"RelayBroadcast", "RelayBroadcast", func(c *grpcFriendsClient, ctx context.Context) {
			c.RelayBroadcast(ctx, &friendspb.RelayBroadcastRequest{TargetWorldId: 2, Message: "x"})
		}},
		{"RelayTrack", "RelayTrack", func(c *grpcFriendsClient, ctx context.Context) {
			c.RelayTrack(ctx, &friendspb.RelayTrackRequest{TargetWorldId: 2, Username37: 1, State: 0})
		}},
		{"RelayReload", "RelayReload", func(c *grpcFriendsClient, ctx context.Context) {
			c.RelayReload(ctx, &friendspb.RelayReloadRequest{TargetWorldId: 2})
		}},
		{"RelayClearLogins", "RelayClearLogins", func(c *grpcFriendsClient, ctx context.Context) {
			c.RelayClearLogins(ctx, &friendspb.RelayClearLoginsRequest{TargetWorldId: 2})
		}},
		{"RelayClearLogouts", "RelayClearLogouts", func(c *grpcFriendsClient, ctx context.Context) {
			c.RelayClearLogouts(ctx, &friendspb.RelayClearLogoutsRequest{TargetWorldId: 2})
		}},
		{"RelayQueueScript", "RelayQueueScript", func(c *grpcFriendsClient, ctx context.Context) {
			c.RelayQueueScript(ctx, &friendspb.RelayQueueScriptRequest{TargetWorldId: 2, ScriptName: "s", Username37: 1})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := &syncBuffer{}
			log := slog.New(slog.NewTextHandler(buf, nil))
			c := &grpcFriendsClient{
				client: &erroringFriendsPBClient{},
				log:    log,
			}
			tc.call(c, context.Background())
			if !strings.Contains(buf.String(), tc.op+" RPC failed") {
				t.Fatalf("log missing %q; got: %s", tc.op+" RPC failed", buf.String())
			}
		})
	}
}
```

If `erroringFriendsPBClient` doesn't already exist in `friends_client_test.go` (or its sibling test file), use the slice-4c equivalent. Search for it:

```bash
grep -rn "erroringFriendsPBClient\|mockFriendsPBClient" modules/world/ 2>&1 | head -5
```

If a similar mock exists (e.g. `mockFriendsPBClient`), reuse it. If neither exists, define a minimal `erroringFriendsPBClient` at the end of the same test file:

```go
// erroringFriendsPBClient returns codes.Unavailable for every RPC. Used
// to exercise the production grpcFriendsClient's error-logging branches.
type erroringFriendsPBClient struct {
	friendspb.FriendsServiceClient
}

func (erroringFriendsPBClient) RelayMute(context.Context, *friendspb.RelayMuteRequest, ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unavailable, "test")
}
func (erroringFriendsPBClient) RelayKick(context.Context, *friendspb.RelayKickRequest, ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unavailable, "test")
}
func (erroringFriendsPBClient) RelayShutdown(context.Context, *friendspb.RelayShutdownRequest, ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unavailable, "test")
}
func (erroringFriendsPBClient) RelayBroadcast(context.Context, *friendspb.RelayBroadcastRequest, ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unavailable, "test")
}
func (erroringFriendsPBClient) RelayTrack(context.Context, *friendspb.RelayTrackRequest, ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unavailable, "test")
}
func (erroringFriendsPBClient) RelayReload(context.Context, *friendspb.RelayReloadRequest, ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unavailable, "test")
}
func (erroringFriendsPBClient) RelayClearLogins(context.Context, *friendspb.RelayClearLoginsRequest, ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unavailable, "test")
}
func (erroringFriendsPBClient) RelayClearLogouts(context.Context, *friendspb.RelayClearLogoutsRequest, ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unavailable, "test")
}
func (erroringFriendsPBClient) RelayQueueScript(context.Context, *friendspb.RelayQueueScriptRequest, ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, status.Error(codes.Unavailable, "test")
}
```

If an existing mock IS present (e.g. the one used by the slice-4c `TestGRPCFriendsClient_LogsErrorOnFailure` test), extend it with the 9 new methods using the same pattern. Inspect the existing test file and follow its conventions.

Ensure imports include `"strings"`, `"context"`, `"log/slog"`, `"testing"`, `"google.golang.org/grpc"`, `"google.golang.org/grpc/codes"`, `"google.golang.org/grpc/status"`, `"google.golang.org/protobuf/types/known/emptypb"`, and `"github.com/zsrv/goscape/pkg/friendspb"` as needed.

- [ ] **Step 2: Verify the test fails to compile**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestGRPCFriendsClient_Relay_LogsErrorOnFailure -count=1
```

Expected: FAIL — `grpcFriendsClient` has no `RelayMute`/etc. methods.

- [ ] **Step 3: Add 9 outbound + 1 stream method to the `FriendsClient` interface**

In `modules/world/friends_client.go`, after the existing `SubscribeUpdates` line (line 40 in the current file) and before `Close() error` (line 41), add:

```go

	// --- slice 5a: RELAY_* admin outbound (all fire-and-forget; errors logged) ---
	RelayMute(ctx context.Context, req *friendspb.RelayMuteRequest)
	RelayKick(ctx context.Context, req *friendspb.RelayKickRequest)
	RelayShutdown(ctx context.Context, req *friendspb.RelayShutdownRequest)
	RelayBroadcast(ctx context.Context, req *friendspb.RelayBroadcastRequest)
	RelayTrack(ctx context.Context, req *friendspb.RelayTrackRequest)
	RelayReload(ctx context.Context, req *friendspb.RelayReloadRequest)
	RelayClearLogins(ctx context.Context, req *friendspb.RelayClearLoginsRequest)
	RelayClearLogouts(ctx context.Context, req *friendspb.RelayClearLogoutsRequest)
	RelayQueueScript(ctx context.Context, req *friendspb.RelayQueueScriptRequest)

	// SubscribeWorldEvents opens the per-world admin push stream. Like
	// SubscribeUpdates, this RPC returns the error so the supervisor can
	// drive reconnect backoff.
	SubscribeWorldEvents(ctx context.Context, req *friendspb.SubscribeWorldEventsRequest) (friendspb.FriendsService_SubscribeWorldEventsClient, error)
```

- [ ] **Step 4: Add 9 outbound + 1 stream impl to `grpcFriendsClient`**

At the end of `modules/world/friends_client.go` (after `SubscribeUpdates`, line 184), append:

```go

// --- slice 5a: RELAY_* admin outbound shims ---
//
// Each method is fire-and-forget per the FriendsClient convention; the
// RPC error is logged at Warn and swallowed. The friends-server is
// best-effort by design — see the file-level FriendsClient doc-comment.

func (c *grpcFriendsClient) RelayMute(ctx context.Context, req *friendspb.RelayMuteRequest) {
	if _, err := c.client.RelayMute(ctx, req); err != nil {
		c.log.Warn("RelayMute RPC failed",
			slog.Int("target_world_id", int(req.TargetWorldId)),
			slog.Uint64("username37", req.Username37),
			slog.Any("err", err),
		)
	}
}

func (c *grpcFriendsClient) RelayKick(ctx context.Context, req *friendspb.RelayKickRequest) {
	if _, err := c.client.RelayKick(ctx, req); err != nil {
		c.log.Warn("RelayKick RPC failed",
			slog.Int("target_world_id", int(req.TargetWorldId)),
			slog.Uint64("username37", req.Username37),
			slog.Any("err", err),
		)
	}
}

func (c *grpcFriendsClient) RelayShutdown(ctx context.Context, req *friendspb.RelayShutdownRequest) {
	if _, err := c.client.RelayShutdown(ctx, req); err != nil {
		c.log.Warn("RelayShutdown RPC failed",
			slog.Int("target_world_id", int(req.TargetWorldId)),
			slog.Any("err", err),
		)
	}
}

func (c *grpcFriendsClient) RelayBroadcast(ctx context.Context, req *friendspb.RelayBroadcastRequest) {
	if _, err := c.client.RelayBroadcast(ctx, req); err != nil {
		c.log.Warn("RelayBroadcast RPC failed",
			slog.Int("target_world_id", int(req.TargetWorldId)),
			slog.Any("err", err),
		)
	}
}

func (c *grpcFriendsClient) RelayTrack(ctx context.Context, req *friendspb.RelayTrackRequest) {
	if _, err := c.client.RelayTrack(ctx, req); err != nil {
		c.log.Warn("RelayTrack RPC failed",
			slog.Int("target_world_id", int(req.TargetWorldId)),
			slog.Uint64("username37", req.Username37),
			slog.Any("err", err),
		)
	}
}

func (c *grpcFriendsClient) RelayReload(ctx context.Context, req *friendspb.RelayReloadRequest) {
	if _, err := c.client.RelayReload(ctx, req); err != nil {
		c.log.Warn("RelayReload RPC failed",
			slog.Int("target_world_id", int(req.TargetWorldId)),
			slog.Any("err", err),
		)
	}
}

func (c *grpcFriendsClient) RelayClearLogins(ctx context.Context, req *friendspb.RelayClearLoginsRequest) {
	if _, err := c.client.RelayClearLogins(ctx, req); err != nil {
		c.log.Warn("RelayClearLogins RPC failed",
			slog.Int("target_world_id", int(req.TargetWorldId)),
			slog.Any("err", err),
		)
	}
}

func (c *grpcFriendsClient) RelayClearLogouts(ctx context.Context, req *friendspb.RelayClearLogoutsRequest) {
	if _, err := c.client.RelayClearLogouts(ctx, req); err != nil {
		c.log.Warn("RelayClearLogouts RPC failed",
			slog.Int("target_world_id", int(req.TargetWorldId)),
			slog.Any("err", err),
		)
	}
}

func (c *grpcFriendsClient) RelayQueueScript(ctx context.Context, req *friendspb.RelayQueueScriptRequest) {
	if _, err := c.client.RelayQueueScript(ctx, req); err != nil {
		c.log.Warn("RelayQueueScript RPC failed",
			slog.Int("target_world_id", int(req.TargetWorldId)),
			slog.String("script_name", req.ScriptName),
			slog.Uint64("username37", req.Username37),
			slog.Any("err", err),
		)
	}
}

// SubscribeWorldEvents opens the server-streaming SubscribeWorldEvents
// RPC. Like SubscribeUpdates, it returns the error so the supervisor
// can drive reconnect backoff (NOT fire-and-forget).
func (c *grpcFriendsClient) SubscribeWorldEvents(ctx context.Context, req *friendspb.SubscribeWorldEventsRequest) (friendspb.FriendsService_SubscribeWorldEventsClient, error) {
	return c.client.SubscribeWorldEvents(ctx, req)
}
```

- [ ] **Step 5: Add 9 outbound + 1 stream impl to `fakeFriendsClient`**

In `modules/world/friends_client_fake_test.go`, add new fields to the `fakeFriendsClient` struct (after the `subscribeErr` field, before `closed`):

```go
	// --- slice 5a Relay* capture channels (cap-16 buffered; non-blocking) ---
	relayMuteReqs         chan *friendspb.RelayMuteRequest
	relayKickReqs         chan *friendspb.RelayKickRequest
	relayShutdownReqs     chan *friendspb.RelayShutdownRequest
	relayBroadcastReqs    chan *friendspb.RelayBroadcastRequest
	relayTrackReqs        chan *friendspb.RelayTrackRequest
	relayReloadReqs       chan *friendspb.RelayReloadRequest
	relayClearLoginsReqs  chan *friendspb.RelayClearLoginsRequest
	relayClearLogoutsReqs chan *friendspb.RelayClearLogoutsRequest
	relayQueueScriptReqs  chan *friendspb.RelayQueueScriptRequest

	// SubscribeWorldEvents state.
	worldSubscribeReqs    []*friendspb.SubscribeWorldEventsRequest
	lastWorldStream       *fakeWorldEventsStream
	worldSubscribeErr     error // one-shot
```

Initialize the channels in `newFakeFriendsClient` (after `privateMessageReqs`):

```go
		relayMuteReqs:         make(chan *friendspb.RelayMuteRequest, 16),
		relayKickReqs:         make(chan *friendspb.RelayKickRequest, 16),
		relayShutdownReqs:     make(chan *friendspb.RelayShutdownRequest, 16),
		relayBroadcastReqs:    make(chan *friendspb.RelayBroadcastRequest, 16),
		relayTrackReqs:        make(chan *friendspb.RelayTrackRequest, 16),
		relayReloadReqs:       make(chan *friendspb.RelayReloadRequest, 16),
		relayClearLoginsReqs:  make(chan *friendspb.RelayClearLoginsRequest, 16),
		relayClearLogoutsReqs: make(chan *friendspb.RelayClearLogoutsRequest, 16),
		relayQueueScriptReqs:  make(chan *friendspb.RelayQueueScriptRequest, 16),
```

Append method impls at the end of the file (after the existing `SubscribeUpdates`):

```go

// --- slice 5a fakeFriendsClient impls ---

func (f *fakeFriendsClient) RelayMute(ctx context.Context, req *friendspb.RelayMuteRequest) {
	select {
	case f.relayMuteReqs <- req:
	default:
	}
}
func (f *fakeFriendsClient) RelayKick(ctx context.Context, req *friendspb.RelayKickRequest) {
	select {
	case f.relayKickReqs <- req:
	default:
	}
}
func (f *fakeFriendsClient) RelayShutdown(ctx context.Context, req *friendspb.RelayShutdownRequest) {
	select {
	case f.relayShutdownReqs <- req:
	default:
	}
}
func (f *fakeFriendsClient) RelayBroadcast(ctx context.Context, req *friendspb.RelayBroadcastRequest) {
	select {
	case f.relayBroadcastReqs <- req:
	default:
	}
}
func (f *fakeFriendsClient) RelayTrack(ctx context.Context, req *friendspb.RelayTrackRequest) {
	select {
	case f.relayTrackReqs <- req:
	default:
	}
}
func (f *fakeFriendsClient) RelayReload(ctx context.Context, req *friendspb.RelayReloadRequest) {
	select {
	case f.relayReloadReqs <- req:
	default:
	}
}
func (f *fakeFriendsClient) RelayClearLogins(ctx context.Context, req *friendspb.RelayClearLoginsRequest) {
	select {
	case f.relayClearLoginsReqs <- req:
	default:
	}
}
func (f *fakeFriendsClient) RelayClearLogouts(ctx context.Context, req *friendspb.RelayClearLogoutsRequest) {
	select {
	case f.relayClearLogoutsReqs <- req:
	default:
	}
}
func (f *fakeFriendsClient) RelayQueueScript(ctx context.Context, req *friendspb.RelayQueueScriptRequest) {
	select {
	case f.relayQueueScriptReqs <- req:
	default:
	}
}

// fakeWorldEventsStream is the per-world counterpart to
// fakeSubscribeStream. Tests push events onto recv; Recv drains.
type fakeWorldEventsStream struct {
	grpc.ClientStream
	ctx  context.Context
	recv chan *friendspb.WorldEvent
}

func newFakeWorldEventsStream(ctx context.Context) *fakeWorldEventsStream {
	return &fakeWorldEventsStream{ctx: ctx, recv: make(chan *friendspb.WorldEvent, 16)}
}

func (s *fakeWorldEventsStream) Recv() (*friendspb.WorldEvent, error) {
	select {
	case ev, ok := <-s.recv:
		if !ok {
			return nil, io.EOF
		}
		return ev, nil
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
}
func (s *fakeWorldEventsStream) Context() context.Context { return s.ctx }

func (f *fakeFriendsClient) SubscribeWorldEvents(ctx context.Context, req *friendspb.SubscribeWorldEventsRequest) (friendspb.FriendsService_SubscribeWorldEventsClient, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.worldSubscribeErr != nil {
		err := f.worldSubscribeErr
		f.worldSubscribeErr = nil // one-shot
		return nil, err
	}
	s := newFakeWorldEventsStream(ctx)
	f.lastWorldStream = s
	f.worldSubscribeReqs = append(f.worldSubscribeReqs, req)
	return s, nil
}
```

- [ ] **Step 6: Run the failure-logging test**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestGRPCFriendsClient_Relay_LogsErrorOnFailure -race -count=1
```

Expected: PASS — 9 subtests green.

- [ ] **Step 7: Run all world-package tests for regression check**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -count=1
```

Expected: PASS — existing tests unchanged; the new `fakeFriendsClient` methods preserve compile.

- [ ] **Step 8: Commit**

```bash
git add modules/world/friends_client.go modules/world/friends_client_fake_test.go modules/world/friends_client_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: FriendsClient gains 9 Relay* + SubscribeWorldEvents (T4)

Interface, fakeFriendsClient (per-method cap-16 chans), and
grpcFriendsClient impls for the 9 unary RELAY_* outbound RPCs +
per-world stream method. Table-driven
TestGRPCFriendsClient_Relay_LogsErrorOnFailure pins the
fire-and-forget warn-and-swallow contract across all 9 methods.
EOF
)"
```

---

## Task 5: World-side `FriendsAdminBridge` + `WorldEventsDispatcher` interfaces and default impls

**Why this task:** Exposes the outbound surface (admin bridge) callers will use to issue RELAY_* RPCs and the inbound dispatcher slot for receiving events. Slog-only default impls land here so slice 5b can swap in action-bearing impls without changing the interface.

**Files:**
- Modify: `modules/world/bridges.go` (add interfaces + slog dispatcher default)
- Create: `modules/world/admin_bridge.go` (grpcFriendsAdminBridge + noopAdminBridge + defaultFriendsAdminBridge)
- Create: `modules/world/admin_bridge_test.go` (bridge mapping pins)
- Create: `modules/world/world_events_dispatcher_test.go` (slog dispatcher pins)

- [ ] **Step 1: Write the failing admin-bridge mapping test**

Create `modules/world/admin_bridge_test.go`:

```go
package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/friendspb"
)

// TestGRPCFriendsAdminBridge_Mute_IssuesRelayMute pins the bridge -> client mapping.
func TestGRPCFriendsAdminBridge_Mute_IssuesRelayMute(t *testing.T) {
	fake := newFakeFriendsClient()
	b := &grpcFriendsAdminBridge{client: fake, log: discardLogger()}
	b.Mute(2, 123, 4567)
	req := <-fake.relayMuteReqs
	if req.TargetWorldId != 2 || req.Username37 != 123 || req.MutedUntilMs != 4567 {
		t.Fatalf("unexpected RelayMute req: %+v", req)
	}
}

func TestGRPCFriendsAdminBridge_Kick_IssuesRelayKick(t *testing.T) {
	fake := newFakeFriendsClient()
	b := &grpcFriendsAdminBridge{client: fake, log: discardLogger()}
	b.Kick(2, 123)
	req := <-fake.relayKickReqs
	if req.TargetWorldId != 2 || req.Username37 != 123 {
		t.Fatalf("unexpected RelayKick req: %+v", req)
	}
}

func TestGRPCFriendsAdminBridge_Shutdown_IssuesRelayShutdown(t *testing.T) {
	fake := newFakeFriendsClient()
	b := &grpcFriendsAdminBridge{client: fake, log: discardLogger()}
	b.Shutdown(2, 100)
	req := <-fake.relayShutdownReqs
	if req.TargetWorldId != 2 || req.DurationTicks != 100 {
		t.Fatalf("unexpected RelayShutdown req: %+v", req)
	}
}

func TestGRPCFriendsAdminBridge_Broadcast_IssuesRelayBroadcast(t *testing.T) {
	fake := newFakeFriendsClient()
	b := &grpcFriendsAdminBridge{client: fake, log: discardLogger()}
	b.Broadcast(2, "hello")
	req := <-fake.relayBroadcastReqs
	if req.TargetWorldId != 2 || req.Message != "hello" {
		t.Fatalf("unexpected RelayBroadcast req: %+v", req)
	}
}

func TestGRPCFriendsAdminBridge_Track_IssuesRelayTrack(t *testing.T) {
	fake := newFakeFriendsClient()
	b := &grpcFriendsAdminBridge{client: fake, log: discardLogger()}
	b.Track(2, 123, 1)
	req := <-fake.relayTrackReqs
	if req.TargetWorldId != 2 || req.Username37 != 123 || req.State != 1 {
		t.Fatalf("unexpected RelayTrack req: %+v", req)
	}
}

func TestGRPCFriendsAdminBridge_Reload_IssuesRelayReload(t *testing.T) {
	fake := newFakeFriendsClient()
	b := &grpcFriendsAdminBridge{client: fake, log: discardLogger()}
	b.Reload(2)
	req := <-fake.relayReloadReqs
	if req.TargetWorldId != 2 {
		t.Fatalf("unexpected RelayReload req: %+v", req)
	}
}

func TestGRPCFriendsAdminBridge_ClearLogins_IssuesRelayClearLogins(t *testing.T) {
	fake := newFakeFriendsClient()
	b := &grpcFriendsAdminBridge{client: fake, log: discardLogger()}
	b.ClearLogins(2)
	req := <-fake.relayClearLoginsReqs
	if req.TargetWorldId != 2 {
		t.Fatalf("unexpected RelayClearLogins req: %+v", req)
	}
}

func TestGRPCFriendsAdminBridge_ClearLogouts_IssuesRelayClearLogouts(t *testing.T) {
	fake := newFakeFriendsClient()
	b := &grpcFriendsAdminBridge{client: fake, log: discardLogger()}
	b.ClearLogouts(2)
	req := <-fake.relayClearLogoutsReqs
	if req.TargetWorldId != 2 {
		t.Fatalf("unexpected RelayClearLogouts req: %+v", req)
	}
}

func TestGRPCFriendsAdminBridge_QueueScript_IssuesRelayQueueScript(t *testing.T) {
	fake := newFakeFriendsClient()
	b := &grpcFriendsAdminBridge{client: fake, log: discardLogger()}
	b.QueueScript(2, "debug:dump", 123)
	req := <-fake.relayQueueScriptReqs
	if req.TargetWorldId != 2 || req.ScriptName != "debug:dump" || req.Username37 != 123 {
		t.Fatalf("unexpected RelayQueueScript req: %+v", req)
	}
}

// TestDefaultFriendsAdminBridge_NilClient_NoopReturnsCleanly pins the
// nil-FriendsClient fallback to noopAdminBridge.
func TestDefaultFriendsAdminBridge_NilClient_NoopReturnsCleanly(t *testing.T) {
	b := defaultFriendsAdminBridge(nil, discardLogger())
	// All methods must not panic.
	b.Mute(1, 1, 1)
	b.Kick(1, 1)
	b.Shutdown(1, 1)
	b.Broadcast(1, "x")
	b.Track(1, 1, 1)
	b.Reload(1)
	b.ClearLogins(1)
	b.ClearLogouts(1)
	b.QueueScript(1, "x", 1)
}
```

`discardLogger()` already exists in `modules/world/server_test.go:21-23` (returns `slog.New(slog.NewTextHandler(io.Discard, nil))`). Re-use it directly — no new helper needed.

- [ ] **Step 2: Write the failing slog-dispatcher test**

Create `modules/world/world_events_dispatcher_test.go`:

```go
package world

import (
	"log/slog"
	"strings"
	"testing"
)

func TestSlogWorldEventsDispatcher_LogsAtInfo(t *testing.T) {
	cases := []struct {
		name string
		want string // substring expected in log line
		call func(d WorldEventsDispatcher)
	}{
		{"Mute", "world event: mute", func(d WorldEventsDispatcher) { d.OnMute(123, 4567) }},
		{"Kick", "world event: kick", func(d WorldEventsDispatcher) { d.OnKick(123) }},
		{"Shutdown", "world event: shutdown", func(d WorldEventsDispatcher) { d.OnShutdown(100) }},
		{"Broadcast", "world event: broadcast", func(d WorldEventsDispatcher) { d.OnBroadcast("hi") }},
		{"Track", "world event: track", func(d WorldEventsDispatcher) { d.OnTrack(123, 1) }},
		{"Reload", "world event: reload", func(d WorldEventsDispatcher) { d.OnReload() }},
		{"ClearLogins", "world event: clear_logins", func(d WorldEventsDispatcher) { d.OnClearLogins() }},
		{"ClearLogouts", "world event: clear_logouts", func(d WorldEventsDispatcher) { d.OnClearLogouts() }},
		{"QueueScript", "world event: queue_script", func(d WorldEventsDispatcher) { d.OnQueueScript("dbg", 123) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := &syncBuffer{}
			d := newSlogWorldEventsDispatcher(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
			tc.call(d)
			if !strings.Contains(buf.String(), tc.want) {
				t.Fatalf("log missing %q; got: %s", tc.want, buf.String())
			}
		})
	}
}
```

- [ ] **Step 3: Verify both tests fail to compile**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestGRPCFriendsAdminBridge_|TestDefaultFriendsAdminBridge_|TestSlogWorldEventsDispatcher_" -count=1
```

Expected: FAIL — types not declared.

- [ ] **Step 4: Add the two interfaces + slog dispatcher to `bridges.go`**

In `modules/world/bridges.go`, after the `slogFriendsDispatcher` definition (or at any natural insertion point near the existing interfaces), append:

```go

// FriendsAdminBridge mirrors TS World.friendThread.postMessage(...) for
// cross-world RELAY_* admin commands (slice 5a). Production impl is
// grpcFriendsAdminBridge (modules/world/admin_bridge.go); wired by
// NewServer via defaultFriendsAdminBridge. When FriendsClient is nil
// (friends-server disabled), the bridge resolves to noopAdminBridge{}.
//
// The bridge is the surface that admin-action code paths use to issue
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

// WorldEventsDispatcher is the world-side sink for inbound RELAY_*
// admin events received over the SubscribeWorldEvents stream (slice 5a).
// Default impl (slogWorldEventsDispatcher) logs each event at Info; no
// world-state effects.
//
// NAI-S5A-D-DISPATCHER-NO-ACTION — slice 5b retires this piecewise as
// each opcode's action is wired (e.g. OnShutdown → services.Manager.StopAsync).
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

// slogWorldEventsDispatcher is the default WorldEventsDispatcher. Logs
// each event at Info; does NOT apply world-state effects. See
// NAI-S5A-D-DISPATCHER-NO-ACTION above.
type slogWorldEventsDispatcher struct {
	log *slog.Logger
}

func newSlogWorldEventsDispatcher(log *slog.Logger) WorldEventsDispatcher {
	return &slogWorldEventsDispatcher{log: log}
}

func (d *slogWorldEventsDispatcher) OnMute(username37 uint64, mutedUntilMs int64) {
	d.log.Info("world event: mute",
		slog.Uint64("username37", username37),
		slog.Int64("muted_until_ms", mutedUntilMs))
}

func (d *slogWorldEventsDispatcher) OnKick(username37 uint64) {
	d.log.Info("world event: kick", slog.Uint64("username37", username37))
}

func (d *slogWorldEventsDispatcher) OnShutdown(durationTicks int32) {
	d.log.Info("world event: shutdown", slog.Int("duration_ticks", int(durationTicks)))
}

func (d *slogWorldEventsDispatcher) OnBroadcast(message string) {
	d.log.Info("world event: broadcast", slog.String("message", message))
}

func (d *slogWorldEventsDispatcher) OnTrack(username37 uint64, state int32) {
	d.log.Info("world event: track",
		slog.Uint64("username37", username37),
		slog.Int("state", int(state)))
}

func (d *slogWorldEventsDispatcher) OnReload() {
	d.log.Info("world event: reload")
}

func (d *slogWorldEventsDispatcher) OnClearLogins() {
	d.log.Info("world event: clear_logins")
}

func (d *slogWorldEventsDispatcher) OnClearLogouts() {
	d.log.Info("world event: clear_logouts")
}

func (d *slogWorldEventsDispatcher) OnQueueScript(scriptName string, username37 uint64) {
	d.log.Info("world event: queue_script",
		slog.String("script_name", scriptName),
		slog.Uint64("username37", username37))
}
```

- [ ] **Step 5: Create `modules/world/admin_bridge.go`**

```go
package world

import (
	"context"
	"log/slog"

	"github.com/zsrv/goscape/pkg/friendspb"
)

// grpcFriendsAdminBridge is the production FriendsAdminBridge impl. Each
// method fans out to the FriendsClient's corresponding Relay* RPC with
// context.Background() — admin commands are fire-and-forget, errors are
// logged inside the FriendsClient layer.
type grpcFriendsAdminBridge struct {
	client FriendsClient
	log    *slog.Logger
}

var _ FriendsAdminBridge = (*grpcFriendsAdminBridge)(nil)

func (b *grpcFriendsAdminBridge) Mute(targetWorldID int32, username37 uint64, mutedUntilMs int64) {
	b.client.RelayMute(context.Background(), &friendspb.RelayMuteRequest{
		TargetWorldId: targetWorldID, Username37: username37, MutedUntilMs: mutedUntilMs,
	})
}

func (b *grpcFriendsAdminBridge) Kick(targetWorldID int32, username37 uint64) {
	b.client.RelayKick(context.Background(), &friendspb.RelayKickRequest{
		TargetWorldId: targetWorldID, Username37: username37,
	})
}

func (b *grpcFriendsAdminBridge) Shutdown(targetWorldID int32, durationTicks int32) {
	b.client.RelayShutdown(context.Background(), &friendspb.RelayShutdownRequest{
		TargetWorldId: targetWorldID, DurationTicks: durationTicks,
	})
}

func (b *grpcFriendsAdminBridge) Broadcast(targetWorldID int32, message string) {
	b.client.RelayBroadcast(context.Background(), &friendspb.RelayBroadcastRequest{
		TargetWorldId: targetWorldID, Message: message,
	})
}

func (b *grpcFriendsAdminBridge) Track(targetWorldID int32, username37 uint64, state int32) {
	b.client.RelayTrack(context.Background(), &friendspb.RelayTrackRequest{
		TargetWorldId: targetWorldID, Username37: username37, State: state,
	})
}

func (b *grpcFriendsAdminBridge) Reload(targetWorldID int32) {
	b.client.RelayReload(context.Background(), &friendspb.RelayReloadRequest{TargetWorldId: targetWorldID})
}

func (b *grpcFriendsAdminBridge) ClearLogins(targetWorldID int32) {
	b.client.RelayClearLogins(context.Background(), &friendspb.RelayClearLoginsRequest{TargetWorldId: targetWorldID})
}

func (b *grpcFriendsAdminBridge) ClearLogouts(targetWorldID int32) {
	b.client.RelayClearLogouts(context.Background(), &friendspb.RelayClearLogoutsRequest{TargetWorldId: targetWorldID})
}

func (b *grpcFriendsAdminBridge) QueueScript(targetWorldID int32, scriptName string, username37 uint64) {
	b.client.RelayQueueScript(context.Background(), &friendspb.RelayQueueScriptRequest{
		TargetWorldId: targetWorldID, ScriptName: scriptName, Username37: username37,
	})
}

// noopAdminBridge is the fallback when FriendsClient is nil
// (FriendsServerEnabled=false). Mirrors the noopBridges{} pattern used
// by defaultFriendsBridge for the social-list bridge.
type noopAdminBridge struct{}

var _ FriendsAdminBridge = noopAdminBridge{}

func (noopAdminBridge) Mute(int32, uint64, int64)        {}
func (noopAdminBridge) Kick(int32, uint64)               {}
func (noopAdminBridge) Shutdown(int32, int32)            {}
func (noopAdminBridge) Broadcast(int32, string)          {}
func (noopAdminBridge) Track(int32, uint64, int32)       {}
func (noopAdminBridge) Reload(int32)                     {}
func (noopAdminBridge) ClearLogins(int32)                {}
func (noopAdminBridge) ClearLogouts(int32)               {}
func (noopAdminBridge) QueueScript(int32, string, uint64) {}

// defaultFriendsAdminBridge returns grpcFriendsAdminBridge when client
// is non-nil; otherwise noopAdminBridge{}.
func defaultFriendsAdminBridge(client FriendsClient, log *slog.Logger) FriendsAdminBridge {
	if client == nil {
		return noopAdminBridge{}
	}
	return &grpcFriendsAdminBridge{client: client, log: log}
}
```

- [ ] **Step 6: Run the tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run "TestGRPCFriendsAdminBridge_|TestDefaultFriendsAdminBridge_|TestSlogWorldEventsDispatcher_" -race -count=1
```

Expected: PASS — 19 subtests green (9 mapping + 1 nil-fallback + 9 slog).

- [ ] **Step 7: Commit**

```bash
git add modules/world/bridges.go modules/world/admin_bridge.go modules/world/admin_bridge_test.go modules/world/world_events_dispatcher_test.go modules/world/world_test_util.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: FriendsAdminBridge + WorldEventsDispatcher interfaces (T5)

Outbound: grpcFriendsAdminBridge wraps the 9 FriendsClient Relay* calls
with context.Background(); noopAdminBridge fallback when FriendsClient
is nil. Inbound: slogWorldEventsDispatcher logs each event variant at
Info (NAI-S5A-D-DISPATCHER-NO-ACTION — slice 5b retires piecewise).

19 unit tests: 9 mapping pins + 1 noop pin + 9 slog log pins.
EOF
)"
```

---

## Task 6: `worldEventsSubscriber` — per-world stream supervisor

**Why this task:** Drains the per-world stream + dispatches to the `WorldEventsDispatcher`. Mirrors slice 4a's `friendsSubscriber` but per-world lifecycle. After this task the world has a complete inbound surface; T7 wires it into the Server lifecycle.

**Files:**
- Create: `modules/world/world_events_subscriber.go`
- Create: `modules/world/world_events_subscriber_test.go`

- [ ] **Step 1: Write the failing tests**

Create `modules/world/world_events_subscriber_test.go`:

```go
package world

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/friendspb"
)

// recordingWorldEventsDispatcher captures all received events.
type recordingWorldEventsDispatcher struct {
	mute        chan struct{ U uint64; M int64 }
	kick        chan uint64
	shutdown    chan int32
	broadcast   chan string
	track       chan struct{ U uint64; S int32 }
	reload      chan struct{}
	clearLogins chan struct{}
	clearLogouts chan struct{}
	queueScript chan struct{ Name string; U uint64 }
}

func newRecordingWorldEventsDispatcher() *recordingWorldEventsDispatcher {
	return &recordingWorldEventsDispatcher{
		mute:         make(chan struct{ U uint64; M int64 }, 8),
		kick:         make(chan uint64, 8),
		shutdown:     make(chan int32, 8),
		broadcast:    make(chan string, 8),
		track:        make(chan struct{ U uint64; S int32 }, 8),
		reload:       make(chan struct{}, 8),
		clearLogins:  make(chan struct{}, 8),
		clearLogouts: make(chan struct{}, 8),
		queueScript:  make(chan struct{ Name string; U uint64 }, 8),
	}
}

func (r *recordingWorldEventsDispatcher) OnMute(u uint64, m int64)        { r.mute <- struct{ U uint64; M int64 }{u, m} }
func (r *recordingWorldEventsDispatcher) OnKick(u uint64)                  { r.kick <- u }
func (r *recordingWorldEventsDispatcher) OnShutdown(d int32)               { r.shutdown <- d }
func (r *recordingWorldEventsDispatcher) OnBroadcast(m string)             { r.broadcast <- m }
func (r *recordingWorldEventsDispatcher) OnTrack(u uint64, s int32)        { r.track <- struct{ U uint64; S int32 }{u, s} }
func (r *recordingWorldEventsDispatcher) OnReload()                        { r.reload <- struct{}{} }
func (r *recordingWorldEventsDispatcher) OnClearLogins()                   { r.clearLogins <- struct{}{} }
func (r *recordingWorldEventsDispatcher) OnClearLogouts()                  { r.clearLogouts <- struct{}{} }
func (r *recordingWorldEventsDispatcher) OnQueueScript(n string, u uint64) { r.queueScript <- struct{ Name string; U uint64 }{n, u} }

func TestWorldEventsSubscriber_DispatchRouting(t *testing.T) {
	fake := newFakeFriendsClient()
	disp := newRecordingWorldEventsDispatcher()
	sub := newWorldEventsSubscriber(fake, 7, disp, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		sub.run(ctx)
		close(done)
	}()

	// Wait for SubscribeWorldEvents to be called and stream installed.
	waitForWorldStream(t, fake)
	stream := fake.lastWorldStream

	stream.recv <- &friendspb.WorldEvent{Event: &friendspb.WorldEvent_Mute{Mute: &friendspb.MuteEvent{Username37: 1, MutedUntilMs: 9}}}
	got := <-disp.mute
	if got.U != 1 || got.M != 9 {
		t.Fatalf("got %+v", got)
	}

	stream.recv <- &friendspb.WorldEvent{Event: &friendspb.WorldEvent_Kick{Kick: &friendspb.KickEvent{Username37: 2}}}
	if u := <-disp.kick; u != 2 {
		t.Fatalf("kick = %d", u)
	}

	stream.recv <- &friendspb.WorldEvent{Event: &friendspb.WorldEvent_Shutdown{Shutdown: &friendspb.ShutdownEvent{DurationTicks: 10}}}
	if d := <-disp.shutdown; d != 10 {
		t.Fatalf("shutdown = %d", d)
	}

	stream.recv <- &friendspb.WorldEvent{Event: &friendspb.WorldEvent_Reload{Reload: &friendspb.ReloadEvent{}}}
	<-disp.reload

	cancel()
	<-done
}

func TestWorldEventsSubscriber_EOFLogsAtInfo(t *testing.T) {
	fake := newFakeFriendsClient()
	disp := newRecordingWorldEventsDispatcher()
	buf := &syncBuffer{}
	log := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	sub := newWorldEventsSubscriber(fake, 7, disp, log)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { sub.run(ctx); close(done) }()

	waitForWorldStream(t, fake)
	// Close stream channel → Recv returns io.EOF.
	close(fake.lastWorldStream.recv)

	// Wait until "EOF; reconnecting" appears (supervisor logs Info).
	waitForLog(t, buf, "world events subscriber EOF; reconnecting")

	cancel()
	<-done
	if strings.Contains(buf.String(), "level=WARN") {
		t.Fatalf("EOF should log at Info, not Warn; got: %s", buf.String())
	}
}

func TestWorldEventsSubscriber_ReconnectBackoff(t *testing.T) {
	fake := newFakeFriendsClient()
	disp := newRecordingWorldEventsDispatcher()
	fake.mu.Lock()
	fake.worldSubscribeErr = errors.New("dial fail")
	fake.mu.Unlock()

	buf := &syncBuffer{}
	log := slog.New(slog.NewTextHandler(buf, nil))
	sub := newWorldEventsSubscriber(fake, 7, disp, log)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { sub.run(ctx); close(done) }()

	// First run returns immediately with the dial error; supervisor sleeps
	// 1s before retrying. Wait for the first "disconnected; reconnecting"
	// log line, then cancel.
	waitForLog(t, buf, "world events subscriber disconnected; reconnecting")

	cancel()
	<-done
}

// waitForWorldStream polls fake.lastWorldStream up to 2s.
func waitForWorldStream(t *testing.T, fake *fakeFriendsClient) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		fake.mu.Lock()
		s := fake.lastWorldStream
		fake.mu.Unlock()
		if s != nil {
			_ = io.EOF // silence unused import if ends up unused
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timeout waiting for world events subscriber to open stream")
}

// waitForLog polls buf for a substring up to 2s.
func waitForLog(t *testing.T, buf *syncBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for log %q; got: %s", want, buf.String())
}
```

- [ ] **Step 2: Verify tests fail to compile**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestWorldEventsSubscriber_ -count=1
```

Expected: FAIL — `newWorldEventsSubscriber` undefined.

- [ ] **Step 3: Create `modules/world/world_events_subscriber.go`**

```go
package world

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	"github.com/zsrv/goscape/pkg/friendspb"
)

// worldEventsSubscriberBackoff* tunes the supervisor reconnect cadence.
// Same posture as friendsSubscriberBackoff* but separate constants so
// future tuning can diverge.
const (
	worldEventsSubscriberBackoffMin = time.Second
	worldEventsSubscriberBackoffMax = 30 * time.Second
	worldEventsSubscriberSteady     = 60 * time.Second
)

// worldEventsSubscriber owns one world's SubscribeWorldEvents stream
// lifetime. Started by world.Server at process boot; stopped when the
// Server's ctx is canceled (Server.Shutdown).
//
// Each iteration:
//   - SubscribeWorldEvents(ctx, req) → stream
//   - Recv loop dispatches WorldEvent variants to WorldEventsDispatcher
//   - On error/EOF: log, exp-backoff, reconnect (unless ctx canceled)
//
// Structurally identical to friendsSubscriber (modules/world/friends_subscriber.go)
// but for the per-world stream + dispatcher.
type worldEventsSubscriber struct {
	client     FriendsClient
	worldID    int32
	dispatcher WorldEventsDispatcher
	log        *slog.Logger
}

func newWorldEventsSubscriber(client FriendsClient, worldID int32, dispatcher WorldEventsDispatcher, log *slog.Logger) *worldEventsSubscriber {
	return &worldEventsSubscriber{
		client:     client,
		worldID:    worldID,
		dispatcher: dispatcher,
		log:        log,
	}
}

// run is the supervisor loop. Blocks until ctx is canceled. Caller
// should typically invoke as `go sub.run(ctx)`.
func (s *worldEventsSubscriber) run(ctx context.Context) {
	backoff := worldEventsSubscriberBackoffMin
	for {
		if ctx.Err() != nil {
			return
		}
		runStart := time.Now()
		err := s.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		if time.Since(runStart) >= worldEventsSubscriberSteady {
			backoff = worldEventsSubscriberBackoffMin
		}
		if errors.Is(err, io.EOF) {
			s.log.Info("world events subscriber EOF; reconnecting",
				slog.Int("world_id", int(s.worldID)),
				slog.Duration("backoff", backoff))
		} else {
			s.log.Warn("world events subscriber disconnected; reconnecting",
				slog.Int("world_id", int(s.worldID)),
				slog.Duration("backoff", backoff),
				slog.Any("err", err))
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = nextWorldEventsBackoff(backoff)
	}
}

func nextWorldEventsBackoff(d time.Duration) time.Duration {
	d *= 2
	if d > worldEventsSubscriberBackoffMax {
		d = worldEventsSubscriberBackoffMax
	}
	return d
}

// runOnce opens a single stream and drains it. Returns when the stream
// ends (error or EOF).
func (s *worldEventsSubscriber) runOnce(ctx context.Context) error {
	stream, err := s.client.SubscribeWorldEvents(ctx, &friendspb.SubscribeWorldEventsRequest{
		WorldId: s.worldID,
	})
	if err != nil {
		return err
	}
	for {
		ev, err := stream.Recv()
		if err != nil {
			return err
		}
		s.dispatch(ev)
	}
}

// dispatch routes one WorldEvent variant to the dispatcher.
func (s *worldEventsSubscriber) dispatch(ev *friendspb.WorldEvent) {
	switch v := ev.Event.(type) {
	case *friendspb.WorldEvent_Mute:
		s.dispatcher.OnMute(v.Mute.Username37, v.Mute.MutedUntilMs)
	case *friendspb.WorldEvent_Kick:
		s.dispatcher.OnKick(v.Kick.Username37)
	case *friendspb.WorldEvent_Shutdown:
		s.dispatcher.OnShutdown(v.Shutdown.DurationTicks)
	case *friendspb.WorldEvent_Broadcast:
		s.dispatcher.OnBroadcast(v.Broadcast.Message)
	case *friendspb.WorldEvent_Track:
		s.dispatcher.OnTrack(v.Track.Username37, v.Track.State)
	case *friendspb.WorldEvent_Reload:
		s.dispatcher.OnReload()
	case *friendspb.WorldEvent_ClearLogins:
		s.dispatcher.OnClearLogins()
	case *friendspb.WorldEvent_ClearLogouts:
		s.dispatcher.OnClearLogouts()
	case *friendspb.WorldEvent_QueueScript:
		s.dispatcher.OnQueueScript(v.QueueScript.ScriptName, v.QueueScript.Username37)
	default:
		s.log.Warn("world events subscriber received unknown event variant",
			slog.Int("world_id", int(s.worldID)))
	}
}
```

- [ ] **Step 4: Run the tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestWorldEventsSubscriber_ -race -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add modules/world/world_events_subscriber.go modules/world/world_events_subscriber_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: worldEventsSubscriber per-world stream supervisor (T6)

Mirrors slice-4a friendsSubscriber: exp-backoff supervisor (1s→30s,
reset@60s), EOF logged at Info / error logged at Warn, dispatcher
routing for all 9 WorldEvent variants. Per-world lifecycle (1 per
Server process; not per-player).

3 tests: dispatch routing, EOF-Info-not-Warn, reconnect-backoff.
EOF
)"
```

---

## Task 7: Wire `worldEventsSubscriber` into `Server` lifecycle

**Why this task:** Closes the loop — `Server` now starts the per-world subscriber at construction time and cancels it at Shutdown. After this task, a running goscape world process maintains a live SubscribeWorldEvents stream against the friends-server (when `FriendsServerEnabled=true`).

**Files:**
- Modify: `modules/world/server.go` (Server struct + NewServer + Shutdown)

- [ ] **Step 1: Add new fields to `Server` struct**

In `modules/world/server.go`, locate the `Server` struct around line 164 (`friendsBridge`, `friendsDispatcher`). Add three new fields right after `friendsDispatcher`:

```go
	friendsAdminBridge    FriendsAdminBridge
	worldEventsDispatcher WorldEventsDispatcher
	worldEventsCancel     context.CancelFunc
```

(Note: we do NOT store the `*worldEventsSubscriber` itself; it owns its own goroutine and the cancel is the only handle we need.)

- [ ] **Step 2: Wire into `NewServer`**

In `modules/world/server.go`, locate the existing wiring block in `NewServer` around lines 278-279:

```go
	s.friendsBridge = defaultFriendsBridge(friendsClient, int32(cfg.NodeID), s.log)
	s.friendsDispatcher = newSlogFriendsDispatcher(s.log)
```

Add three lines after `s.friendsDispatcher = ...`:

```go
	s.friendsAdminBridge = defaultFriendsAdminBridge(friendsClient, s.log)
	s.worldEventsDispatcher = newSlogWorldEventsDispatcher(s.log)
	if friendsClient != nil {
		ctx, cancel := context.WithCancel(context.Background())
		s.worldEventsCancel = cancel
		sub := newWorldEventsSubscriber(friendsClient, int32(cfg.NodeID), s.worldEventsDispatcher, s.log)
		go sub.run(ctx)
	}
```

- [ ] **Step 3: Wire cancel into `Server.Shutdown`**

In `modules/world/server.go`, the `Shutdown` method at line 537. Insert the cancel call at the top, before `close(s.quit)`:

```go
func (s *Server) Shutdown() {
	if s.worldEventsCancel != nil {
		s.worldEventsCancel()
	}
	close(s.quit)
	s.log.Debug("closing tcp listener")
	s.tcpListener.Close()
	s.log.Debug("waiting for tcp connections to close")
	s.tcpWg.Wait()
	s.log.Debug("all tcp connections closed")
	// ... (rest unchanged)
}
```

- [ ] **Step 4: Verify the world package builds**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...
```

Expected: clean.

- [ ] **Step 5: Run the world-package tests**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... -count=1 -timeout 120s
```

Expected: PASS — server-construction tests that pass `friendsClient != nil` will now spawn an extra goroutine. The fakeFriendsClient.SubscribeWorldEvents impl from T4 handles the call cleanly. If any existing test fails because it constructs a Server and doesn't cancel cleanly, fix by adding a `defer s.Shutdown()` call.

If a test deadlocks or races, the most likely cause is a test that constructs Server, never calls Shutdown, and lets the supervisor goroutine outlive the test. Use `defer` to ensure each Server constructed in a test gets Shutdown.

- [ ] **Step 6: Commit**

```bash
git add modules/world/server.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: wire worldEventsSubscriber into Server lifecycle (T7)

NewServer starts the per-world subscriber when friendsClient != nil;
Server.Shutdown cancels it. Adds friendsAdminBridge +
worldEventsDispatcher to Server fields (default impls). The subscriber
runs unconditional of player presence — the world is "present" on the
friends-server from process boot (matches TS WORLD_CONNECT lifecycle).
EOF
)"
```

---

## Task 8: E2E smoke — `TestFriendsClient_E2E_RelayWorldEventsRoundTrip`

**Why this task:** The integration test that pins the cross-world routing end-to-end against a real friends-server. Mirrors slice 4a's `TestFriendsClient_E2E_SubscribeUpdatesStream` and slice 4b's `TestFriendsClient_E2E_PrivateMessageDelivery` patterns.

**Files:**
- Modify: `modules/world/friends_smoke_test.go`

- [ ] **Step 1: Write the failing e2e smoke**

In `modules/world/friends_smoke_test.go`, append:

```go
// TestFriendsClient_E2E_RelayWorldEventsRoundTrip boots a real
// friends-server, opens two SubscribeWorldEvents streams (one per
// world), issues Relay* RPCs cross-world from world A targeting world
// B, and asserts the dispatcher on world B receives the events while
// world A's dispatcher does NOT.
//
// Slice 5a e2e contract.
func TestFriendsClient_E2E_RelayWorldEventsRoundTrip(t *testing.T) {
	// Boot a real friends-server (inline; no shared harness in this file —
	// follow the pattern from TestFriendsClient_E2E_SmokeAgainstFriendsServer).
	port := freePort(t)
	cfg := friends.Config{
		GRPCListenAddress:       "127.0.0.1",
		GRPCListenPort:          port,
		NodeProfile:             "main",
		WorldPlayerLimit:        100,
		Enable:                  true,
		GracefulShutdownTimeout: 5 * time.Second,
		SQLiteDSN:               filepath.Join(t.TempDir(), "friends.db"),
	}
	log := discardLogger()
	svc, err := friends.New(cfg, log)
	if err != nil {
		t.Fatalf("friends.New: %v", err)
	}
	bootCtx, bootCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer bootCancel()
	if err := svc.StartAsync(bootCtx); err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if err := svc.AwaitRunning(bootCtx); err != nil {
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

	dispA := newRecordingWorldEventsDispatcher()
	dispB := newRecordingWorldEventsDispatcher()

	subA := newWorldEventsSubscriber(client, 1, dispA, log)
	subB := newWorldEventsSubscriber(client, 2, dispB, log)

	ctxA, cancelA := context.WithCancel(context.Background())
	ctxB, cancelB := context.WithCancel(context.Background())
	doneA := make(chan struct{})
	doneB := make(chan struct{})
	go func() { subA.run(ctxA); close(doneA) }()
	go func() { subB.run(ctxB); close(doneB) }()
	defer func() {
		cancelA()
		cancelB()
		<-doneA
		<-doneB
	}()

	// Give the streams a moment to register on the server side. The
	// subscriber installs its registry entry as soon as
	// SubscribeWorldEvents returns the stream; the RPC itself is
	// asynchronous, so wait until both worlds appear in the registry.
	// We can't observe the server's registry directly from the test, so
	// poll via a probe Relay*: issue a no-target probe first and check
	// for arrival.
	//
	// Simpler: issue RelayKick(target=2) and wait for dispB.kick. Retry
	// up to 2s.
	probeDelivered := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !probeDelivered {
		client.RelayKick(context.Background(), &friendspb.RelayKickRequest{TargetWorldId: 2, Username37: 9999})
		select {
		case <-dispB.kick:
			probeDelivered = true
		case <-time.After(50 * time.Millisecond):
			// retry
		}
	}
	if !probeDelivered {
		t.Fatal("timeout waiting for initial RelayKick probe to reach world B")
	}

	// Now issue cross-world events targeting world B and assert they arrive.
	client.RelayMute(context.Background(), &friendspb.RelayMuteRequest{
		TargetWorldId: 2, Username37: 123, MutedUntilMs: 4567,
	})
	select {
	case got := <-dispB.mute:
		if got.U != 123 || got.M != 4567 {
			t.Fatalf("mute payload mismatch: %+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for mute on world B")
	}

	client.RelayShutdown(context.Background(), &friendspb.RelayShutdownRequest{
		TargetWorldId: 2, DurationTicks: 50,
	})
	select {
	case d := <-dispB.shutdown:
		if d != 50 {
			t.Fatalf("shutdown = %d", d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for shutdown on world B")
	}

	client.RelayReload(context.Background(), &friendspb.RelayReloadRequest{TargetWorldId: 2})
	select {
	case <-dispB.reload:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for reload on world B")
	}

	// World A's dispatcher must NOT have received any of the events
	// directed at world B.
	select {
	case got := <-dispA.mute:
		t.Fatalf("world A unexpectedly received mute: %+v", got)
	default:
	}
	select {
	case d := <-dispA.shutdown:
		t.Fatalf("world A unexpectedly received shutdown: %d", d)
	default:
	}
	select {
	case <-dispA.reload:
		t.Fatal("world A unexpectedly received reload")
	default:
	}

	// Cross-direction sanity: target world A → arrives on world A.
	client.RelayBroadcast(context.Background(), &friendspb.RelayBroadcastRequest{
		TargetWorldId: 1, Message: "hello-A",
	})
	select {
	case msg := <-dispA.broadcast:
		if msg != "hello-A" {
			t.Fatalf("broadcast on A = %q, want %q", msg, "hello-A")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for broadcast on world A")
	}
}
```

Imports for this test (verify against the file's existing import block; many are already present from prior e2e tests): `"context"`, `"path/filepath"`, `"strconv"`, `"testing"`, `"time"`, `"github.com/zsrv/goscape/modules/friends"`, `"github.com/zsrv/goscape/pkg/friendspb"`.

- [ ] **Step 2: Verify the test fails first (will succeed if proto regenerated cleanly in T1)**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/... -run TestFriendsClient_E2E_RelayWorldEventsRoundTrip -race -count=1 -timeout 60s
```

Expected: this is the integration test that ties all prior tasks together. If T1-T7 are correct, this should pass on first run.

If it fails:
- "subscribers don't see events" → check that SubscribeWorldEvents stream registers in T3 handler (worldSubs.register).
- "world A receives B's event" → check the registry's per-worldId key isolation in T2.
- "first probe times out" → the subscriber goroutines may not have opened streams yet; the probe-loop above mitigates this with retry.

- [ ] **Step 3: Commit**

```bash
git add modules/world/friends_smoke_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: e2e smoke for slice-5a RELAY_* round-trip (T8)

Boots a real friends-server, opens two SubscribeWorldEvents streams
(worlds 1 + 2), issues Relay* RPCs from world A targeting world B,
asserts B receives + A does NOT. Includes cross-direction sanity:
RelayBroadcast to world A → arrives on A. Pins the slice-5a transport
contract end-to-end.
EOF
)"
```

---

## Task 9: Final gates — race + smoke-pack + tag accounting

**Why this task:** Closes the slice. Verifies no race regressions across the project, the smoke-pack baseline holds, and the 4 new NAI-S5A-D-* tags are correctly annotated in the source.

**Files:**
- No new files; tag accounting + verification only.

- [ ] **Step 1: Full project -race gate**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... -count=1 -timeout 600s
```

Expected: PASS across all packages.

- [ ] **Step 2: Smoke-pack baseline check**

```bash
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape-cli smoke-pack --content-dir /home/owner/Code/github.com/LostCityRS/content
```

Expected: 12 OK / 0 ERR / 0 SKIP. Slice 5a is server/world-only — packing is unaffected.

- [ ] **Step 3: Verify tag accounting in source**

Confirm the 4 new tags are annotated and the existing inventory is unchanged:

```bash
grep -rn "NAI-S5A-D-" modules/friends/ modules/world/ proto/ | sort
```

Expected output (paraphrased; exact line numbers will vary):

- `proto/friends/friends.proto: ... NAI-S5A-D-NO-ADMIN-AUTH-AT-SERVER ...`
- `proto/friends/friends.proto: ... NAI-S5A-D-PERWORLD-EVENTS-STREAM-SEPARATE ...`
- `modules/friends/world_subscriptions.go: ... NAI-S5A-D-WORLDEVENTS-DROP-ON-FULL ...`
- `modules/friends/handler.go: ... NAI-S5A-D-NO-ADMIN-AUTH-AT-SERVER ...`
- `modules/friends/handler.go: ... NAI-S5A-D-DISPATCHER-NO-ACTION ...`
- `modules/world/bridges.go: ... NAI-S5A-D-DISPATCHER-NO-ACTION ...`

If any of these annotations are missing, add them inline at the relevant call site as a doc-comment line (matching slice 4a/4b/4c convention). No new tests required for tag-annotation-only fixes; commit separately as a "tag accounting" cleanup.

- [ ] **Step 4: Verify no slice-1/2/3/4 tags were inadvertently changed**

```bash
git diff main -- 'modules/friends/**.go' 'modules/world/**.go' | grep -E "NAI-S[1-4]"
```

Expected: only the slice-5a additions (no deletions or modifications to slice-1/2/3/4 tag annotations).

- [ ] **Step 5: Tag closure summary commit (only if Step 3 required adjustments)**

If any tag annotations were missing, commit them:

```bash
git add <files>
git commit --no-gpg-sign -m "friends: slice 5a tag annotation cleanup (T9)"
```

If everything was already in place from earlier tasks, skip this step.

- [ ] **Step 6: Slice close — write `MEMORY.md` entry**

After the slice is fully verified, the controller should add a memory entry per the existing `friends-server slice 4c close` pattern at `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/`. This is controller-level work, not a code task. Skip if delegated.

---

## Self-review notes (recorded inline, not a separate task)

- **Spec coverage:** Every section §1-§12 of the spec maps to a task above. The Makefile fix (§10) → T0. Proto (§3) → T1. Server registry (§4) → T2. Server handlers (§5) → T3. World interface + bridge + dispatcher + subscriber (§6) → T4 + T5 + T6 + T7. E2E smoke (§8 table) → T8. Validation gates (§12) → T9.
- **Placeholder scan:** zero TBD/TODO/"add appropriate" lines.
- **Type consistency:** Method signatures defined in T4 (`FriendsClient.RelayMute(ctx, *RelayMuteRequest)`) match the usage in T5 (`grpcFriendsAdminBridge.Mute → client.RelayMute(ctx, &RelayMuteRequest{...})`). Dispatcher signatures defined in T5 (`OnMute(username37 uint64, mutedUntilMs int64)`) match the recording impl in T6 + the e2e test in T8. The `worldSubs` field added to `handler` in T3 step 3 matches all read sites in step 4. The `worldSubscriberBufferSize` constant in T2 step 3 matches the test's `for range worldSubscriberBufferSize` in step 1.
- **One known judgment-call in T4 step 1:** the `erroringFriendsPBClient` helper may or may not already exist (likely the slice-4c equivalent is named `mockFriendsPBClient`). The plan instructs the implementer to search and reuse if present; otherwise define fresh. If a similar helper exists with the 9 slice-1/2 RPCs already mocked, EXTEND it rather than duplicating — pure judgment call by the implementer.
