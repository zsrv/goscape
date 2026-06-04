# Friends-server bridge slice 4a — SubscribeUpdates stream foundation implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the slice-1 `codes.Unimplemented` stub for `SubscribeUpdates` with a per-(world, player) stream foundation and wire `broadcastWorldToFollowers` fan-out into all seven mutating RPC handlers; build the world-side stream subscriber with exp-backoff supervisor and dispatch to a logging-only `FriendsDispatcher` (NAI-S4A-D-NO-INGAME-PACKET-EMIT gates client emission).

**Architecture:** New `modules/friends/subscriptions.go` holds a `map[uint64]*subscriber` registered when `SubscribeUpdates` opens and torn down when the stream closes. RPC handlers call `broadcastWorldToFollowers(other)` after mutating state; that helper resolves followers via `repo.GetFollowers(ctx, other)`, applies `IsVisibleToMany` (new batch helper), and pushes a one-entry `FriendlistUpdate` into each follower's buffered channel (size 64, drop-newest on full). World-side opens one stream per logged-in player; supervisor reconnects with exp backoff 1s→30s (reset@60s steady) mirroring `[[content-watcher-auto-restart-close]]`.

**Tech Stack:** Go 1.24, `google.golang.org/grpc`, existing `pkg/friendspb` proto, `database/sql` via `*sql.DB`. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-05-20-friends-server-bridge-slice4a-design.md`

**Conventions (CLAUDE.md):**
- All `go` invocations: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`
- All commits: `git commit --no-gpg-sign`
- Use `use-modern-go` skill conventions when writing Go (slog, `t.Context()`, `errors.Is`, `for range N` where applicable, `any` over `interface{}`).

---

## Phase 1 — Server-side foundation

### Task 1: Add `subscriptions.go` (registry types + methods)

**Files:**
- Create: `modules/friends/subscriptions.go`

- [ ] **Step 1: Create `modules/friends/subscriptions.go`**

```go
package friends

import (
	"log/slog"
	"sync"

	"github.com/zsrv/goscape/pkg/friendspb"
)

// subscriberBufferSize is the per-subscriber channel buffer. Tuned for
// modest broadcast bursts; oversize beyond plausible per-player update
// rate so steady-state never drops.
//
// NAI-S4A-D-DROP-ON-FULL — overflowing the buffer drops the newest
// update with a Warn log instead of blocking the RPC handler.
const subscriberBufferSize = 64

// subscriber is a single open SubscribeUpdates stream for one
// (worldId, username37) pair. ch is written by RPC handlers; the
// gRPC stream goroutine drains ch and calls stream.Send. done is
// closed by deregister to signal the gRPC goroutine to exit.
type subscriber struct {
	worldId    int32
	username37 uint64
	ch         chan *friendspb.FriendsUpdate
	done       chan struct{}
}

// newSubscriber allocates ch + done with the standard buffer size.
func newSubscriber(worldId int32, username37 uint64) *subscriber {
	return &subscriber{
		worldId:    worldId,
		username37: username37,
		ch:         make(chan *friendspb.FriendsUpdate, subscriberBufferSize),
		done:       make(chan struct{}),
	}
}

// subscriptions is the per-player subscriber registry. All methods are
// goroutine-safe.
type subscriptions struct {
	mu  sync.Mutex
	by  map[uint64]*subscriber // username37 -> subscriber
	log *slog.Logger
}

func newSubscriptions(log *slog.Logger) *subscriptions {
	return &subscriptions{
		by:  make(map[uint64]*subscriber),
		log: log,
	}
}

// register installs sub under sub.username37. If a prior subscriber
// exists for the same username37, it is kicked (its done is closed)
// before sub replaces it. Generalizes TS FriendServer.initializeWorld
// terminate-then-replace (FriendServer.ts:412-419) from per-world to
// per-player.
func (s *subscriptions) register(sub *subscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if prior, ok := s.by[sub.username37]; ok {
		close(prior.done)
	}
	s.by[sub.username37] = sub
}

// deregister removes sub from the registry IFF it is still the
// currently registered subscriber for sub.username37 (a rapid
// re-login may have replaced it under register).
func (s *subscriptions) deregister(sub *subscriber) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cur, ok := s.by[sub.username37]; ok && cur == sub {
		delete(s.by, sub.username37)
	}
}

// send pushes u to the subscriber for username37 (no-op if none).
// Non-blocking; on full channel, logs warn and drops the update.
func (s *subscriptions) send(username37 uint64, u *friendspb.FriendsUpdate) {
	s.mu.Lock()
	sub, ok := s.by[username37]
	s.mu.Unlock()
	if !ok {
		return
	}
	select {
	case sub.ch <- u:
	default:
		s.log.Warn("friends subscriber buffer full; dropping update",
			slog.Uint64("username37", username37))
	}
}
```

- [ ] **Step 2: Compile-check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/friends/...`
Expected: builds clean (no errors).

- [ ] **Step 3: Commit**

```bash
git add modules/friends/subscriptions.go
git commit --no-gpg-sign -m "friends: add per-player subscriber registry (subscriptions.go)"
```

---

### Task 2: Add `subscriptions_test.go`

**Files:**
- Create: `modules/friends/subscriptions_test.go`

- [ ] **Step 1: Write tests**

```go
package friends

import (
	"io"
	"log/slog"
	"testing"

	"github.com/zsrv/goscape/pkg/friendspb"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestSubscriptions_RegisterDeregister(t *testing.T) {
	s := newSubscriptions(discardLogger())
	sub := newSubscriber(1, 100)
	s.register(sub)
	s.send(100, &friendspb.FriendsUpdate{})
	select {
	case u := <-sub.ch:
		if u == nil {
			t.Fatalf("unexpected nil update")
		}
	default:
		t.Fatalf("expected update on sub.ch")
	}
	s.deregister(sub)
	s.send(100, &friendspb.FriendsUpdate{}) // no-op now
	select {
	case <-sub.ch:
		t.Fatalf("expected no update after deregister")
	default:
	}
}

func TestSubscriptions_DupRegisterKicksPrior(t *testing.T) {
	s := newSubscriptions(discardLogger())
	a := newSubscriber(1, 100)
	b := newSubscriber(1, 100)
	s.register(a)
	s.register(b)
	select {
	case <-a.done:
	default:
		t.Fatalf("expected prior subscriber done to be closed")
	}
	// b should still be in registry; send routes to b
	s.send(100, &friendspb.FriendsUpdate{})
	select {
	case <-b.ch:
	default:
		t.Fatalf("expected update on new subscriber b.ch")
	}
	// a should not receive
	select {
	case <-a.ch:
		t.Fatalf("expected no update on prior subscriber a.ch")
	default:
	}
}

func TestSubscriptions_DropOnFull(t *testing.T) {
	s := newSubscriptions(discardLogger())
	sub := newSubscriber(1, 100)
	s.register(sub)
	// Fill buffer.
	for range subscriberBufferSize {
		s.send(100, &friendspb.FriendsUpdate{})
	}
	// Next send drops (no panic, no block).
	s.send(100, &friendspb.FriendsUpdate{})
	// Drain to verify exactly subscriberBufferSize updates queued.
	got := 0
	for {
		select {
		case <-sub.ch:
			got++
			continue
		default:
		}
		break
	}
	if got != subscriberBufferSize {
		t.Fatalf("got %d updates, want %d", got, subscriberBufferSize)
	}
}

func TestSubscriptions_DeregisterIgnoresStale(t *testing.T) {
	s := newSubscriptions(discardLogger())
	a := newSubscriber(1, 100)
	b := newSubscriber(1, 100)
	s.register(a)
	s.register(b) // kicks a
	s.deregister(a) // a is stale; b should remain
	s.send(100, &friendspb.FriendsUpdate{})
	select {
	case <-b.ch:
	default:
		t.Fatalf("expected b to still be registered")
	}
}

func TestSubscriptions_SendUnknownNoop(t *testing.T) {
	s := newSubscriptions(discardLogger())
	// No panic, no block.
	s.send(999, &friendspb.FriendsUpdate{})
}
```

- [ ] **Step 2: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/ -run TestSubscriptions -v`
Expected: PASS (5 tests).

- [ ] **Step 3: Commit**

```bash
git add modules/friends/subscriptions_test.go
git commit --no-gpg-sign -m "friends: add subscriptions registry tests"
```

---

### Task 3: Add `IsVisibleToMany` batch helper to repository

**Files:**
- Modify: `modules/friends/repository.go`

Spec §5 describes the batch semantic. For viewers V, target other O:
- If O has no presence row, all viewers see false.
- privateChat 0 (ON) → all viewers true.
- privateChat 1 (FRIENDS) → SQL `WHERE owner = O AND target IN (V...)`; viewers present in result are true.
- privateChat 2 (OFF) / unknown → all viewers false.

- [ ] **Step 1: Add the method to `modules/friends/repository.go`**

Append after the existing `IsVisibleTo` method (after line 314):

```go

// IsVisibleToMany is the batched analogue of IsVisibleTo. Returns a
// map[viewer]bool with one entry per input viewer. The empty result is
// a valid response — callers must check the map, not nil.
//
// Locking discipline: same as IsVisibleTo — r.mu is released before any
// SQL call.
//
// Algorithm:
//
//	other.privateChat 0 (ON)      -> all viewers true
//	other.privateChat 1 (FRIENDS) -> one SQL IN query against friendlist
//	                                 where owner = other and target IN
//	                                 viewers; viewers in result are true
//	other.privateChat 2 (OFF)     -> all viewers false
//	other has no presence row     -> all viewers false
//
// Slice 4a uses this from handler.broadcastWorldToFollowers to avoid
// the N+1 round trips that a scalar-IsVisibleTo loop would incur.
func (r *Repository) IsVisibleToMany(ctx context.Context, viewers []uint64, other uint64) (map[uint64]bool, error) {
	out := make(map[uint64]bool, len(viewers))
	if len(viewers) == 0 {
		return out, nil
	}

	r.mu.RLock()
	ps, ok := r.players[other]
	if !ok {
		r.mu.RUnlock()
		for _, v := range viewers {
			out[v] = false
		}
		return out, nil
	}
	mode := ps.privateChat
	r.mu.RUnlock()

	switch mode {
	case 0: // ON
		for _, v := range viewers {
			out[v] = true
		}
		return out, nil
	case 1: // FRIENDS
		// Build a parameterized IN clause.
		placeholders := make([]byte, 0, 2*len(viewers))
		args := make([]any, 0, 2+len(viewers))
		args = append(args, r.profile, int64(other))
		for i, v := range viewers {
			if i > 0 {
				placeholders = append(placeholders, ',')
			}
			placeholders = append(placeholders, '?')
			args = append(args, int64(v))
		}
		query := `SELECT target_username37 FROM friendlist
		          WHERE profile = ? AND owner_username37 = ?
		            AND target_username37 IN (` + string(placeholders) + `)`
		rows, err := r.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("IsVisibleToMany: %w", err)
		}
		defer rows.Close()

		// Default everyone to false; flip the ones returned.
		for _, v := range viewers {
			out[v] = false
		}
		for rows.Next() {
			var t int64
			if err := rows.Scan(&t); err != nil {
				return nil, fmt.Errorf("IsVisibleToMany scan: %w", err)
			}
			out[uint64(t)] = true
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("IsVisibleToMany rows: %w", err)
		}
		return out, nil
	default: // OFF or unknown
		for _, v := range viewers {
			out[v] = false
		}
		return out, nil
	}
}
```

- [ ] **Step 2: Compile-check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/friends/...`
Expected: builds clean.

- [ ] **Step 3: Commit**

```bash
git add modules/friends/repository.go
git commit --no-gpg-sign -m "friends: add IsVisibleToMany batch helper"
```

---

### Task 4: Add `IsVisibleToMany` tests

**Files:**
- Modify: `modules/friends/repository_test.go`

- [ ] **Step 1: Append tests**

The existing `repository_test.go` exposes `newTestRepo(t)` which returns a `*Repository` backed by a temp-dir SQLite DB. Use the same helper.

Append at end of `modules/friends/repository_test.go`:

```go

func TestIsVisibleToMany_EmptyViewers(t *testing.T) {
	r, _ := newTestRepo(t)
	r.InitializeWorld(1, 100)
	r.Register(1, 100, 0, 0)
	got, err := r.IsVisibleToMany(t.Context(), nil, 100)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want empty map", got)
	}
}

func TestIsVisibleToMany_OtherNotRegistered(t *testing.T) {
	r, _ := newTestRepo(t)
	got, err := r.IsVisibleToMany(t.Context(), []uint64{1, 2, 3}, 100)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, v := range []uint64{1, 2, 3} {
		if got[v] {
			t.Errorf("viewer %d: got true, want false (other not registered)", v)
		}
	}
}

func TestIsVisibleToMany_ChatModeOnAllVisible(t *testing.T) {
	r, _ := newTestRepo(t)
	r.InitializeWorld(1, 100)
	r.Register(1, 100, 0, 0) // privateChat ON
	got, err := r.IsVisibleToMany(t.Context(), []uint64{1, 2, 3}, 100)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, v := range []uint64{1, 2, 3} {
		if !got[v] {
			t.Errorf("viewer %d: got false, want true (mode ON)", v)
		}
	}
}

func TestIsVisibleToMany_ChatModeOffNoneVisible(t *testing.T) {
	r, _ := newTestRepo(t)
	r.InitializeWorld(1, 100)
	r.Register(1, 100, 2, 0) // privateChat OFF
	got, err := r.IsVisibleToMany(t.Context(), []uint64{1, 2, 3}, 100)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, v := range []uint64{1, 2, 3} {
		if got[v] {
			t.Errorf("viewer %d: got true, want false (mode OFF)", v)
		}
	}
}

func TestIsVisibleToMany_ChatModeFriendsOnlyFriendsVisible(t *testing.T) {
	r, _ := newTestRepo(t)
	r.InitializeWorld(1, 100)
	r.Register(1, 100, 1, 0) // privateChat FRIENDS
	// 100 added 2 and 3 as friends; 1 is not a friend.
	if err := r.AddFriend(t.Context(), 100, 2); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	if err := r.AddFriend(t.Context(), 100, 3); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	got, err := r.IsVisibleToMany(t.Context(), []uint64{1, 2, 3}, 100)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got[1] {
		t.Errorf("viewer 1: got true, want false (not a friend)")
	}
	if !got[2] {
		t.Errorf("viewer 2: got false, want true (friend)")
	}
	if !got[3] {
		t.Errorf("viewer 3: got false, want true (friend)")
	}
}

func TestIsVisibleToMany_MatchesScalarIsVisibleTo(t *testing.T) {
	r, _ := newTestRepo(t)
	r.InitializeWorld(1, 100)
	r.Register(1, 100, 1, 0)
	if err := r.AddFriend(t.Context(), 100, 2); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}

	viewers := []uint64{1, 2, 3, 4, 5}
	batch, err := r.IsVisibleToMany(t.Context(), viewers, 100)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	for _, v := range viewers {
		want, err := r.IsVisibleTo(t.Context(), v, 100)
		if err != nil {
			t.Fatalf("IsVisibleTo: %v", err)
		}
		if batch[v] != want {
			t.Errorf("viewer %d: batch=%v scalar=%v", v, batch[v], want)
		}
	}
}
```

- [ ] **Step 2: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/ -run TestIsVisibleToMany -v`
Expected: PASS (6 tests).

- [ ] **Step 3: Commit**

```bash
git add modules/friends/repository_test.go
git commit --no-gpg-sign -m "friends: test IsVisibleToMany batch helper"
```

---

### Task 5: Thread `subscriptions` through Friends module + handler struct

**Files:**
- Modify: `modules/friends/handler.go`
- Modify: `modules/friends/grpcServer.go`
- Modify: `modules/friends/friends.go`

Current shape (slice 3):
```go
// handler.go
type handler struct {
    friendspb.UnimplementedFriendsServiceServer
    repo *Repository
    cfg  Config
    log  *slog.Logger
}

// grpcServer.go
func newGRPCServer(cfg Config, repo *Repository, log *slog.Logger) *grpcServer {
    s := grpc.NewServer()
    friendspb.RegisterFriendsServiceServer(s, &handler{repo: repo, cfg: cfg, log: log})
    return &grpcServer{server: s, log: log}
}

// friends.go starting()
srv := newGRPCServer(f.cfg, repo, f.log)
```

- [ ] **Step 1: Add `subs *subscriptions` field to handler**

In `modules/friends/handler.go`, change the struct (around line 14-20):

```go
type handler struct {
	friendspb.UnimplementedFriendsServiceServer

	repo *Repository
	subs *subscriptions
	cfg  Config
	log  *slog.Logger
}
```

- [ ] **Step 2: Update `newGRPCServer` to receive and pass subscriptions**

In `modules/friends/grpcServer.go`, change `newGRPCServer` signature (around line 20):

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

- [ ] **Step 3: Update `friends.go` `starting()` to construct subscriptions**

In `modules/friends/friends.go`, update the `starting` method (around line 42-58):

```go
func (f *Friends) starting(_ context.Context) error {
	db, err := openDB(f.cfg.SQLiteDSN)
	if err != nil {
		return fmt.Errorf("open friends db: %w", err)
	}
	repo := NewRepository(db, f.cfg.NodeProfile)
	subs := newSubscriptions(f.log)
	srv := newGRPCServer(f.cfg, repo, subs, f.log)
	lis, err := srv.listen(f.cfg)
	if err != nil {
		db.Close()
		return err
	}
	f.db = db
	f.repo = repo
	f.subs = subs
	f.srv = srv
	f.lis = lis
	return nil
}
```

Also add the `subs` field to the `Friends` struct (around line 15-25):

```go
type Friends struct {
	services.Service

	cfg Config
	log *slog.Logger

	db   *sql.DB
	repo *Repository
	subs *subscriptions
	srv  *grpcServer
	lis  net.Listener
}
```

- [ ] **Step 4: Compile-check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/friends/...`
Expected: builds clean. (Existing handler tests may fail because they construct `handler{}` directly — fix in step 5.)

- [ ] **Step 5: Update handler test fixtures**

In `modules/friends/handler_test.go`, find every `handler{repo: ..., cfg: ..., log: ...}` literal and add `subs: newSubscriptions(log)` (or a helper). Search:

Run: `grep -n "handler{" modules/friends/handler_test.go`

For each match, add the `subs` field. Example transformation:

Before:
```go
h := &handler{repo: r, cfg: cfg, log: log}
```

After:
```go
h := &handler{repo: r, subs: newSubscriptions(log), cfg: cfg, log: log}
```

- [ ] **Step 6: Run all friends tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/ -v`
Expected: PASS (all existing tests + new ones from T1-T4).

- [ ] **Step 7: Commit**

```bash
git add modules/friends/handler.go modules/friends/grpcServer.go modules/friends/friends.go modules/friends/handler_test.go
git commit --no-gpg-sign -m "friends: thread subscriptions through Friends/grpcServer/handler"
```

---

### Task 6: Implement `SubscribeUpdates` RPC handler

**Files:**
- Modify: `modules/friends/handler.go`

Replace the inherited `UnimplementedFriendsServiceServer.SubscribeUpdates` (returns Unimplemented) with a real implementation that:
1. Registers a new subscriber in the registry (kicking any prior).
2. Sends initial UPDATE_FRIENDLIST + UPDATE_IGNORELIST snapshots.
3. Drains sub.ch into stream.Send until ctx/done.

- [ ] **Step 1: Add `SubscribeUpdates` method to handler**

Append to `modules/friends/handler.go`:

```go

// SubscribeUpdates streams server -> world friends updates for one
// (worldId, username37) pair. Mirrors TS FriendServer's WebSocket-per-
// world push channel, but proto-typed per (world, player). Sends initial
// UPDATE_FRIENDLIST + UPDATE_IGNORELIST snapshots on attach, then drains
// the subscriber's channel until the stream context or done signal.
//
// Replaces the slice-1 codes.Unimplemented stub.
//
// NAI-S1-D-NO-FOLLOWER-BROADCAST — fan-out wiring lives in
// broadcastWorldToFollowers (this file); SubscribeUpdates only owns the
// stream lifecycle.
func (h *handler) SubscribeUpdates(req *friendspb.SubscribeUpdatesRequest, stream friendspb.FriendsService_SubscribeUpdatesServer) error {
	h.ensureWorld(req.WorldId)

	sub := newSubscriber(req.WorldId, req.Username37)
	h.subs.register(sub)
	defer h.subs.deregister(sub)

	ctx := stream.Context()

	// Initial snapshots (TS FriendServer sendFriendsListToPlayer +
	// sendIgnoreListToPlayer, FriendServer.ts:138-139, but on subscribe
	// instead of login).
	if err := h.sendInitialFriendlist(ctx, stream, req.Username37); err != nil {
		return err
	}
	if err := h.sendInitialIgnorelist(ctx, stream, req.Username37); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-sub.done:
			return nil
		case u := <-sub.ch:
			if err := stream.Send(u); err != nil {
				return err
			}
		}
	}
}

// sendInitialFriendlist mirrors TS sendFriendsListToPlayer
// (FriendServer.ts:421-431). Builds one FriendlistUpdate containing
// every friend with the friend's current world (0 if offline). Visibility
// rules are applied via IsVisibleToMany batched across the friend set.
func (h *handler) sendInitialFriendlist(ctx context.Context, stream friendspb.FriendsService_SubscribeUpdatesServer, viewer uint64) error {
	friends, err := h.repo.GetFriends(ctx, viewer)
	if err != nil {
		return status.Errorf(codes.Internal, "GetFriends: %v", err)
	}
	entries := make([]*friendspb.FriendEntry, 0, len(friends))
	for _, f := range friends {
		entries = append(entries, &friendspb.FriendEntry{
			WorldId:    h.worldIfVisible(ctx, viewer, f),
			Username37: f,
		})
	}
	return stream.Send(&friendspb.FriendsUpdate{
		Update: &friendspb.FriendsUpdate_Friendlist{
			Friendlist: &friendspb.FriendlistUpdate{Entries: entries},
		},
	})
}

// sendInitialIgnorelist mirrors TS sendIgnoreListToPlayer
// (FriendServer.ts:433-443).
func (h *handler) sendInitialIgnorelist(ctx context.Context, stream friendspb.FriendsService_SubscribeUpdatesServer, viewer uint64) error {
	ignores, err := h.repo.GetIgnores(ctx, viewer)
	if err != nil {
		return status.Errorf(codes.Internal, "GetIgnores: %v", err)
	}
	return stream.Send(&friendspb.FriendsUpdate{
		Update: &friendspb.FriendsUpdate_Ignorelist{
			Ignorelist: &friendspb.IgnorelistUpdate{Username37: ignores},
		},
	})
}

// worldIfVisible is the per-entry visibility helper used by initial
// snapshots. For the broadcast hot path use IsVisibleToMany.
func (h *handler) worldIfVisible(ctx context.Context, viewer, other uint64) int32 {
	visible, err := h.repo.IsVisibleTo(ctx, viewer, other)
	if err != nil {
		h.log.Warn("IsVisibleTo failed; treating as not visible",
			slog.Uint64("viewer", viewer),
			slog.Uint64("other", other),
			slog.Any("err", err))
		return 0
	}
	if !visible {
		return 0
	}
	return h.repo.GetWorld(other)
}
```

- [ ] **Step 2: Verify imports**

`modules/friends/handler.go` should already import `codes`, `status`, and `friendspb`. Confirm `context` is imported (it should be).

- [ ] **Step 3: Compile-check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/friends/...`
Expected: builds clean.

- [ ] **Step 4: Commit**

```bash
git add modules/friends/handler.go
git commit --no-gpg-sign -m "friends: implement SubscribeUpdates handler (initial snapshots + drain loop)"
```

---

### Task 7: Implement `broadcastWorldToFollowers` + `sendPlayerWorldUpdate`

**Files:**
- Modify: `modules/friends/handler.go`

- [ ] **Step 1: Append both methods to handler.go**

```go

// broadcastWorldToFollowers fans out a one-entry FriendlistUpdate to
// each of `other`'s followers that has an open subscription. Mirrors
// TS FriendServer.broadcastWorldToFollowers (FriendServer.ts:445-451).
// Errors are logged at Warn but never block the RPC caller; the
// friends-server is best-effort by design.
//
// NAI-S1-D-NO-FOLLOWER-BROADCAST — slice 4a retires this tag by wiring
// this helper into the seven mutating RPC handlers.
func (h *handler) broadcastWorldToFollowers(ctx context.Context, other uint64) {
	followers, err := h.repo.GetFollowers(ctx, other)
	if err != nil {
		h.log.Warn("broadcastWorldToFollowers: GetFollowers failed",
			slog.Uint64("other", other),
			slog.Any("err", err))
		return
	}
	if len(followers) == 0 {
		return
	}
	visibility, err := h.repo.IsVisibleToMany(ctx, followers, other)
	if err != nil {
		h.log.Warn("broadcastWorldToFollowers: IsVisibleToMany failed",
			slog.Uint64("other", other),
			slog.Any("err", err))
		return
	}
	otherWorld := h.repo.GetWorld(other)
	for _, viewer := range followers {
		worldForViewer := int32(0)
		if visibility[viewer] {
			worldForViewer = otherWorld
		}
		h.subs.send(viewer, &friendspb.FriendsUpdate{
			Update: &friendspb.FriendsUpdate_Friendlist{
				Friendlist: &friendspb.FriendlistUpdate{
					Entries: []*friendspb.FriendEntry{{
						WorldId:    worldForViewer,
						Username37: other,
					}},
				},
			},
		})
	}
}

// sendPlayerWorldUpdate pushes a single-friend update to viewer's
// subscription. Mirrors TS FriendServer.sendPlayerWorldUpdate
// (FriendServer.ts:462-478). Called by FriendlistAdd to notify the
// adder of the new friend's current world.
func (h *handler) sendPlayerWorldUpdate(ctx context.Context, viewer, other uint64) {
	visible, err := h.repo.IsVisibleTo(ctx, viewer, other)
	if err != nil {
		h.log.Warn("sendPlayerWorldUpdate: IsVisibleTo failed",
			slog.Uint64("viewer", viewer),
			slog.Uint64("other", other),
			slog.Any("err", err))
		visible = false
	}
	worldForViewer := int32(0)
	if visible {
		worldForViewer = h.repo.GetWorld(other)
	}
	h.subs.send(viewer, &friendspb.FriendsUpdate{
		Update: &friendspb.FriendsUpdate_Friendlist{
			Friendlist: &friendspb.FriendlistUpdate{
				Entries: []*friendspb.FriendEntry{{
					WorldId:    worldForViewer,
					Username37: other,
				}},
			},
		},
	})
}
```

- [ ] **Step 2: Compile-check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/friends/...`
Expected: builds clean.

- [ ] **Step 3: Commit**

```bash
git add modules/friends/handler.go
git commit --no-gpg-sign -m "friends: add broadcastWorldToFollowers + sendPlayerWorldUpdate"
```

---

### Task 8: Wire broadcast into seven RPC handlers

**Files:**
- Modify: `modules/friends/handler.go`

Updates the existing handlers (`PlayerLogin`, `PlayerLogout`, `ChatSetMode`, `FriendlistAdd`, `FriendlistDel`, `IgnorelistAdd`, `IgnorelistDel`) to call `broadcastWorldToFollowers` after the mutation. `FriendlistAdd` additionally calls `sendPlayerWorldUpdate(adder, target)` first.

Also: update the doc-comments to retire `NAI-S1-D-NO-FOLLOWER-BROADCAST` from each handler that currently references it.

- [ ] **Step 1: Update `PlayerLogin`**

Replace the existing method (around lines 60-73) with:

```go
// PlayerLogin registers the player on the given world. Always returns OK;
// PlayerLoginResponse.Accepted is false iff the world's player cap is
// reached.
//
// NAI-S1-D-PLAYERCAP-LOG-ONLY — cap rejection logs warn but does not error.
// Slice 4c surfaces Accepted to callers.
func (h *handler) PlayerLogin(ctx context.Context, req *friendspb.PlayerLoginRequest) (*friendspb.PlayerLoginResponse, error) {
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
		// No broadcast on rejection — player isn't on any world.
		return &friendspb.PlayerLoginResponse{Accepted: false}, nil
	}
	h.broadcastWorldToFollowers(ctx, req.Username37)
	return &friendspb.PlayerLoginResponse{Accepted: true}, nil
}
```

Note: the prior method took `_ context.Context`. Now takes `ctx`.

- [ ] **Step 2: Update `PlayerLogout`**

Replace (around lines 75-82):

```go
// PlayerLogout removes the player from whichever world they're on.
// Idempotent on unknown players. Broadcasts the (now-offline) world to
// followers after Unregister.
func (h *handler) PlayerLogout(ctx context.Context, req *friendspb.PlayerLogoutRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	h.repo.Unregister(req.Username37)
	h.broadcastWorldToFollowers(ctx, req.Username37)
	return &emptypb.Empty{}, nil
}
```

- [ ] **Step 3: Update `ChatSetMode`**

Replace (around lines 90-94):

```go
// ChatSetMode updates the player's privateChat setting. Invalid values
// are coerced to 0 (ON), matching TS FriendServer.ts:176-179. No-op on
// unknown player (state lives at the player record, which doesn't exist
// pre-login).
func (h *handler) ChatSetMode(ctx context.Context, req *friendspb.ChatSetModeRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	h.repo.SetChatMode(req.Username37, coercePrivateChat(req.PrivateChat))
	h.broadcastWorldToFollowers(ctx, req.Username37)
	return &emptypb.Empty{}, nil
}
```

- [ ] **Step 4: Update `FriendlistAdd`**

Replace (around lines 96-104):

```go
// FriendlistAdd appends target to the player's friend set (idempotent).
// Sends a single-friend update to the adder for `target` (TS
// sendPlayerWorldUpdate at FriendServer.ts:200) and then broadcasts the
// adder's world to all followers (TS FriendServer.ts:204).
func (h *handler) FriendlistAdd(ctx context.Context, req *friendspb.FriendlistAddRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	if err := h.repo.AddFriend(ctx, req.Username37, req.TargetUsername37); err != nil {
		return nil, status.Errorf(codes.Internal, "AddFriend: %v", err)
	}
	h.sendPlayerWorldUpdate(ctx, req.Username37, req.TargetUsername37)
	h.broadcastWorldToFollowers(ctx, req.Username37)
	return &emptypb.Empty{}, nil
}
```

- [ ] **Step 5: Update `FriendlistDel`**

Replace (around lines 106-114):

```go
// FriendlistDel removes target from the player's friend set (idempotent).
// Broadcasts the remover's world to followers (TS FriendServer.ts:221).
func (h *handler) FriendlistDel(ctx context.Context, req *friendspb.FriendlistDelRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	if err := h.repo.DeleteFriend(ctx, req.Username37, req.TargetUsername37); err != nil {
		return nil, status.Errorf(codes.Internal, "DeleteFriend: %v", err)
	}
	h.broadcastWorldToFollowers(ctx, req.Username37)
	return &emptypb.Empty{}, nil
}
```

- [ ] **Step 6: Update `IgnorelistAdd`**

Replace (around lines 116-124):

```go
// IgnorelistAdd appends target to the player's ignore set (idempotent).
// Broadcasts the adder's world to followers (TS FriendServer.ts:238).
func (h *handler) IgnorelistAdd(ctx context.Context, req *friendspb.IgnorelistAddRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	if err := h.repo.AddIgnore(ctx, req.Username37, req.TargetUsername37); err != nil {
		return nil, status.Errorf(codes.Internal, "AddIgnore: %v", err)
	}
	h.broadcastWorldToFollowers(ctx, req.Username37)
	return &emptypb.Empty{}, nil
}
```

- [ ] **Step 7: Update `IgnorelistDel`**

Replace (around lines 126-134):

```go
// IgnorelistDel removes target from the player's ignore set (idempotent).
// Broadcasts the remover's world to followers (TS FriendServer.ts:255).
func (h *handler) IgnorelistDel(ctx context.Context, req *friendspb.IgnorelistDelRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	if err := h.repo.DeleteIgnore(ctx, req.Username37, req.TargetUsername37); err != nil {
		return nil, status.Errorf(codes.Internal, "DeleteIgnore: %v", err)
	}
	h.broadcastWorldToFollowers(ctx, req.Username37)
	return &emptypb.Empty{}, nil
}
```

- [ ] **Step 8: Run existing handler tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/ -v`
Expected: PASS. The existing handler tests assert RPC return values, which haven't changed. Any failure indicates a regression — investigate before proceeding.

- [ ] **Step 9: Commit**

```bash
git add modules/friends/handler.go
git commit --no-gpg-sign -m "friends: wire broadcastWorldToFollowers into 7 RPC handlers (retires NAI-S1-D-NO-FOLLOWER-BROADCAST)"
```

---

### Task 9: Add handler-level fan-out tests

**Files:**
- Modify: `modules/friends/handler_test.go`

These tests exercise `SubscribeUpdates` + a mutating RPC together, asserting the follower's stream receives the expected `FriendlistUpdate`.

- [ ] **Step 1: Build a small helper for test subscriber capture**

Append to `modules/friends/handler_test.go`:

```go

// testStream is a minimal friendspb.FriendsService_SubscribeUpdatesServer
// impl that captures Send calls into a channel. Cancel ctx to stop the
// handler's drain loop.
type testStream struct {
	grpc.ServerStream
	ctx    context.Context
	cancel context.CancelFunc
	out    chan *friendspb.FriendsUpdate
}

func newTestStream(t *testing.T) *testStream {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	return &testStream{ctx: ctx, cancel: cancel, out: make(chan *friendspb.FriendsUpdate, 32)}
}

func (s *testStream) Context() context.Context { return s.ctx }
func (s *testStream) Send(u *friendspb.FriendsUpdate) error {
	select {
	case s.out <- u:
	default:
	}
	return nil
}

// recvWithin waits up to d for the next update on s.out; t.Fatal on
// timeout.
func (s *testStream) recvWithin(t *testing.T, d time.Duration) *friendspb.FriendsUpdate {
	t.Helper()
	select {
	case u := <-s.out:
		return u
	case <-time.After(d):
		t.Fatalf("timed out waiting for update")
		return nil
	}
}
```

Required imports (top of `handler_test.go`): `time`, `google.golang.org/grpc`. Add if missing.

- [ ] **Step 2: Test initial snapshots on SubscribeUpdates attach**

```go

func TestSubscribeUpdates_InitialSnapshots(t *testing.T) {
	r, _ := newTestRepo(t)
	cfg := Config{NodeProfile: "main", WorldPlayerLimit: 100}
	h := &handler{repo: r, subs: newSubscriptions(discardLogger()), cfg: cfg, log: discardLogger()}
	r.InitializeWorld(1, 100)
	r.Register(1, 100, 0, 0)
	if err := r.AddFriend(t.Context(), 100, 200); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	if err := r.AddIgnore(t.Context(), 100, 300); err != nil {
		t.Fatalf("AddIgnore: %v", err)
	}

	stream := newTestStream(t)
	errc := make(chan error, 1)
	go func() {
		errc <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 1, Username37: 100}, stream)
	}()
	t.Cleanup(func() {
		stream.cancel()
		<-errc
	})

	// First message: FriendlistUpdate with one entry for 200.
	u1 := stream.recvWithin(t, 2*time.Second)
	fl, ok := u1.Update.(*friendspb.FriendsUpdate_Friendlist)
	if !ok {
		t.Fatalf("first update = %T, want FriendsUpdate_Friendlist", u1.Update)
	}
	if len(fl.Friendlist.Entries) != 1 || fl.Friendlist.Entries[0].Username37 != 200 {
		t.Fatalf("entries = %v, want one entry for 200", fl.Friendlist.Entries)
	}

	// Second message: IgnorelistUpdate with [300].
	u2 := stream.recvWithin(t, 2*time.Second)
	il, ok := u2.Update.(*friendspb.FriendsUpdate_Ignorelist)
	if !ok {
		t.Fatalf("second update = %T, want FriendsUpdate_Ignorelist", u2.Update)
	}
	if len(il.Ignorelist.Username37) != 1 || il.Ignorelist.Username37[0] != 300 {
		t.Fatalf("ignored = %v, want [300]", il.Ignorelist.Username37)
	}
}
```

- [ ] **Step 3: Test broadcastWorldToFollowers via PlayerLogin**

```go

func TestPlayerLogin_BroadcastsToFollowers(t *testing.T) {
	r, _ := newTestRepo(t)
	cfg := Config{NodeProfile: "main", WorldPlayerLimit: 100}
	h := &handler{repo: r, subs: newSubscriptions(discardLogger()), cfg: cfg, log: discardLogger()}
	r.InitializeWorld(1, 100)
	// Follower 100 friended target 200.
	if err := r.AddFriend(t.Context(), 100, 200); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	// 100 is online so the subscription has a presence row to query.
	r.Register(1, 100, 0, 0)

	// 100 subscribes.
	stream := newTestStream(t)
	errc := make(chan error, 1)
	go func() {
		errc <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 1, Username37: 100}, stream)
	}()
	t.Cleanup(func() {
		stream.cancel()
		<-errc
	})
	// Drain initial snapshots.
	stream.recvWithin(t, 2*time.Second)
	stream.recvWithin(t, 2*time.Second)

	// 200 logs in on world 1.
	if _, err := h.PlayerLogin(t.Context(), &friendspb.PlayerLoginRequest{
		WorldId:     1,
		Username37:  200,
		PrivateChat: 0,
		StaffLvl:    0,
	}); err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}

	// 100's stream should see a one-entry FriendlistUpdate naming 200, world=1.
	u := stream.recvWithin(t, 2*time.Second)
	fl, ok := u.Update.(*friendspb.FriendsUpdate_Friendlist)
	if !ok {
		t.Fatalf("update = %T, want FriendsUpdate_Friendlist", u.Update)
	}
	if len(fl.Friendlist.Entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(fl.Friendlist.Entries))
	}
	e := fl.Friendlist.Entries[0]
	if e.Username37 != 200 || e.WorldId != 1 {
		t.Fatalf("entry = (%d, %d), want (1, 200)", e.WorldId, e.Username37)
	}
}
```

- [ ] **Step 4: Test ChatMode visibility gating in broadcast**

```go

func TestBroadcast_ChatModeOffHidesWorld(t *testing.T) {
	r, _ := newTestRepo(t)
	cfg := Config{NodeProfile: "main", WorldPlayerLimit: 100}
	h := &handler{repo: r, subs: newSubscriptions(discardLogger()), cfg: cfg, log: discardLogger()}
	r.InitializeWorld(1, 100)
	if err := r.AddFriend(t.Context(), 100, 200); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	r.Register(1, 100, 0, 0)

	stream := newTestStream(t)
	errc := make(chan error, 1)
	go func() {
		errc <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 1, Username37: 100}, stream)
	}()
	t.Cleanup(func() {
		stream.cancel()
		<-errc
	})
	stream.recvWithin(t, 2*time.Second)
	stream.recvWithin(t, 2*time.Second)

	// 200 logs in with privateChat OFF.
	if _, err := h.PlayerLogin(t.Context(), &friendspb.PlayerLoginRequest{
		WorldId:     1,
		Username37:  200,
		PrivateChat: 2, // OFF
		StaffLvl:    0,
	}); err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}

	u := stream.recvWithin(t, 2*time.Second)
	fl := u.Update.(*friendspb.FriendsUpdate_Friendlist)
	if fl.Friendlist.Entries[0].WorldId != 0 {
		t.Fatalf("WorldId = %d, want 0 (privateChat OFF should hide)", fl.Friendlist.Entries[0].WorldId)
	}
}
```

- [ ] **Step 5: Test PlayerLogout broadcasts world=0**

```go

func TestPlayerLogout_BroadcastsZeroWorld(t *testing.T) {
	r, _ := newTestRepo(t)
	cfg := Config{NodeProfile: "main", WorldPlayerLimit: 100}
	h := &handler{repo: r, subs: newSubscriptions(discardLogger()), cfg: cfg, log: discardLogger()}
	r.InitializeWorld(1, 100)
	if err := r.AddFriend(t.Context(), 100, 200); err != nil {
		t.Fatalf("AddFriend: %v", err)
	}
	r.Register(1, 100, 0, 0)
	r.Register(1, 200, 0, 0)

	stream := newTestStream(t)
	errc := make(chan error, 1)
	go func() {
		errc <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 1, Username37: 100}, stream)
	}()
	t.Cleanup(func() {
		stream.cancel()
		<-errc
	})
	stream.recvWithin(t, 2*time.Second)
	stream.recvWithin(t, 2*time.Second)

	if _, err := h.PlayerLogout(t.Context(), &friendspb.PlayerLogoutRequest{
		WorldId:    1,
		Username37: 200,
	}); err != nil {
		t.Fatalf("PlayerLogout: %v", err)
	}

	u := stream.recvWithin(t, 2*time.Second)
	fl := u.Update.(*friendspb.FriendsUpdate_Friendlist)
	if fl.Friendlist.Entries[0].WorldId != 0 || fl.Friendlist.Entries[0].Username37 != 200 {
		t.Fatalf("entry = (%d, %d), want (0, 200)", fl.Friendlist.Entries[0].WorldId, fl.Friendlist.Entries[0].Username37)
	}
}
```

- [ ] **Step 6: Test FriendlistAdd sends both update to adder and broadcast**

```go

func TestFriendlistAdd_AdderGetsTargetWorldAndFollowersBroadcast(t *testing.T) {
	r, _ := newTestRepo(t)
	cfg := Config{NodeProfile: "main", WorldPlayerLimit: 100}
	h := &handler{repo: r, subs: newSubscriptions(discardLogger()), cfg: cfg, log: discardLogger()}
	r.InitializeWorld(1, 100)
	// Pre-existing: adder 100 has a follower 50 who already friended 100.
	if err := r.AddFriend(t.Context(), 50, 100); err != nil {
		t.Fatalf("AddFriend (50->100): %v", err)
	}
	r.Register(1, 100, 0, 0)
	r.Register(1, 50, 0, 0)
	r.Register(1, 200, 0, 0)

	// Both subscribers attach.
	adderStream := newTestStream(t)
	followerStream := newTestStream(t)
	errAdder := make(chan error, 1)
	errFollower := make(chan error, 1)
	go func() {
		errAdder <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 1, Username37: 100}, adderStream)
	}()
	go func() {
		errFollower <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 1, Username37: 50}, followerStream)
	}()
	t.Cleanup(func() {
		adderStream.cancel()
		followerStream.cancel()
		<-errAdder
		<-errFollower
	})
	// Drain initial snapshots from both.
	adderStream.recvWithin(t, 2*time.Second)
	adderStream.recvWithin(t, 2*time.Second)
	followerStream.recvWithin(t, 2*time.Second)
	followerStream.recvWithin(t, 2*time.Second)

	// 100 adds 200 as a friend.
	if _, err := h.FriendlistAdd(t.Context(), &friendspb.FriendlistAddRequest{
		WorldId:          1,
		Username37:       100,
		TargetUsername37: 200,
	}); err != nil {
		t.Fatalf("FriendlistAdd: %v", err)
	}

	// Adder (100): single-entry update for 200 (sendPlayerWorldUpdate) +
	// broadcast (100 is in its own followers? no — 50 follows 100, 100
	// doesn't follow itself). So adder sees only the sendPlayerWorldUpdate.
	uAdder := adderStream.recvWithin(t, 2*time.Second)
	adderFL := uAdder.Update.(*friendspb.FriendsUpdate_Friendlist)
	if adderFL.Friendlist.Entries[0].Username37 != 200 {
		t.Fatalf("adder entry = %d, want 200", adderFL.Friendlist.Entries[0].Username37)
	}
	// Follower (50): broadcast about 100's world.
	uFollower := followerStream.recvWithin(t, 2*time.Second)
	followerFL := uFollower.Update.(*friendspb.FriendsUpdate_Friendlist)
	if followerFL.Friendlist.Entries[0].Username37 != 100 || followerFL.Friendlist.Entries[0].WorldId != 1 {
		t.Fatalf("follower entry = (%d, %d), want (1, 100)", followerFL.Friendlist.Entries[0].WorldId, followerFL.Friendlist.Entries[0].Username37)
	}
}
```

- [ ] **Step 7: Run all friends handler tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/ -v`
Expected: PASS (all existing tests + 5 new SubscribeUpdates/broadcast tests).

- [ ] **Step 8: Commit**

```bash
git add modules/friends/handler_test.go
git commit --no-gpg-sign -m "friends: test SubscribeUpdates initial snapshots + broadcast fan-out"
```

---

## Phase 2 — World-side subscriber + dispatcher

### Task 10: Extend `FriendsClient` with `SubscribeUpdates`

**Files:**
- Modify: `modules/world/friends_client.go`
- Modify: `modules/world/friends_client_fake_test.go`

Adds the streaming RPC to the FriendsClient interface. Unlike the fire-and-forget RPCs, this one returns the stream + error so the supervisor can react to dial failures.

- [ ] **Step 1: Add interface method**

In `modules/world/friends_client.go`, extend the `FriendsClient` interface (around lines 24-35):

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
	// SubscribeUpdates opens a server-streaming RPC. Returns the stream on
	// success; the caller drains stream.Recv(). Unlike the other RPCs,
	// this one is not fire-and-forget — the supervisor needs the error
	// to drive reconnect backoff.
	SubscribeUpdates(ctx context.Context, req *friendspb.SubscribeUpdatesRequest) (friendspb.FriendsService_SubscribeUpdatesClient, error)
	Close() error
}
```

- [ ] **Step 2: Implement on grpcFriendsClient**

Append to `modules/world/friends_client.go`:

```go

// SubscribeUpdates opens the server-streaming SubscribeUpdates RPC.
func (c *grpcFriendsClient) SubscribeUpdates(ctx context.Context, req *friendspb.SubscribeUpdatesRequest) (friendspb.FriendsService_SubscribeUpdatesClient, error) {
	return c.client.SubscribeUpdates(ctx, req)
}
```

- [ ] **Step 3: Extend fakeFriendsClient**

In `modules/world/friends_client_fake_test.go`, append:

```go

// fakeSubscribeStream is a controllable test impl of
// friendspb.FriendsService_SubscribeUpdatesClient. Tests push updates
// onto recv; Recv drains and returns them. Close ctx (passed to
// SubscribeUpdates) to terminate the stream.
type fakeSubscribeStream struct {
	grpc.ClientStream
	ctx  context.Context
	recv chan *friendspb.FriendsUpdate
}

func newFakeSubscribeStream(ctx context.Context) *fakeSubscribeStream {
	return &fakeSubscribeStream{ctx: ctx, recv: make(chan *friendspb.FriendsUpdate, 16)}
}

func (s *fakeSubscribeStream) Recv() (*friendspb.FriendsUpdate, error) {
	select {
	case u, ok := <-s.recv:
		if !ok {
			return nil, io.EOF
		}
		return u, nil
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
}
func (s *fakeSubscribeStream) Context() context.Context { return s.ctx }

// SubscribeUpdates returns a fakeSubscribeStream the test can push to
// via the field exposed below.
func (f *fakeFriendsClient) SubscribeUpdates(ctx context.Context, req *friendspb.SubscribeUpdatesRequest) (friendspb.FriendsService_SubscribeUpdatesClient, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.subscribeErr != nil {
		err := f.subscribeErr
		f.subscribeErr = nil // one-shot
		return nil, err
	}
	s := newFakeSubscribeStream(ctx)
	f.lastStream = s
	f.subscribeReqs = append(f.subscribeReqs, req)
	return s, nil
}
```

Also extend the `fakeFriendsClient` struct (around lines 14-29):

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

	// SubscribeUpdates state.
	subscribeReqs []*friendspb.SubscribeUpdatesRequest
	lastStream    *fakeSubscribeStream
	subscribeErr  error // one-shot error returned on next call; tests set to simulate dial failures

	closed bool
}
```

Imports to add: `io`, `google.golang.org/grpc`. The file already has `context`.

- [ ] **Step 4: Compile-check both files**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -run Compile -count=1` (the `-run Compile` matches nothing; just exercises compilation).

Or simpler: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...` — note this won't compile *test files*. To compile test files: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/world/...` covers both.

Expected: builds clean.

- [ ] **Step 5: Commit**

```bash
git add modules/world/friends_client.go modules/world/friends_client_fake_test.go
git commit --no-gpg-sign -m "world: extend FriendsClient with SubscribeUpdates stream"
```

---

### Task 11: Add `FriendsClient.SubscribeUpdates` production-wiring test

**Files:**
- Modify: `modules/world/friends_client_test.go`

The existing `friends_client_test.go` has a `mockFriendsPBClient` that lets us assert grpcFriendsClient delegates to the right pb-level method. Extend it.

- [ ] **Step 1: Add SubscribeUpdates wiring test**

Read the existing file to understand the mockFriendsPBClient shape:

Run: `grep -n "mockFriendsPBClient\|subscribeUpdatesFn" modules/world/friends_client_test.go | head -20`

Find the struct definition and the existing per-method `fn` fields pattern. Add a `subscribeUpdatesFn` field and impl. Add a test verifying that `c.SubscribeUpdates(...)` delegates to the mock:

```go

func TestGRPCFriendsClient_SubscribeUpdates_Delegates(t *testing.T) {
	called := make(chan *friendspb.SubscribeUpdatesRequest, 1)
	mock := &mockFriendsPBClient{
		subscribeUpdatesFn: func(ctx context.Context, in *friendspb.SubscribeUpdatesRequest, opts ...grpc.CallOption) (friendspb.FriendsService_SubscribeUpdatesClient, error) {
			called <- in
			return nil, status.Error(codes.Unavailable, "test")
		},
	}
	c := &grpcFriendsClient{client: mock, log: discardLogger()}
	_, err := c.SubscribeUpdates(context.Background(), &friendspb.SubscribeUpdatesRequest{WorldId: 5, Username37: 42})
	if err == nil {
		t.Fatalf("expected error from mock")
	}
	select {
	case got := <-called:
		if got.WorldId != 5 || got.Username37 != 42 {
			t.Fatalf("got = %v, want WorldId=5 Username37=42", got)
		}
	case <-time.After(time.Second):
		t.Fatalf("mock not called")
	}
}
```

The `mockFriendsPBClient` may need a new field; if so add:

```go
	subscribeUpdatesFn func(ctx context.Context, in *friendspb.SubscribeUpdatesRequest, opts ...grpc.CallOption) (friendspb.FriendsService_SubscribeUpdatesClient, error)
```

and the method:

```go
func (m *mockFriendsPBClient) SubscribeUpdates(ctx context.Context, in *friendspb.SubscribeUpdatesRequest, opts ...grpc.CallOption) (friendspb.FriendsService_SubscribeUpdatesClient, error) {
	if m.subscribeUpdatesFn != nil {
		return m.subscribeUpdatesFn(ctx, in, opts...)
	}
	return nil, nil
}
```

Imports: `google.golang.org/grpc/status`, `google.golang.org/grpc/codes`.

- [ ] **Step 2: Run test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -run TestGRPCFriendsClient_SubscribeUpdates -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add modules/world/friends_client_test.go
git commit --no-gpg-sign -m "world: test grpcFriendsClient.SubscribeUpdates delegates to gRPC client"
```

---

### Task 12: Add `FriendsDispatcher` interface + impls

**Files:**
- Modify: `modules/world/bridges.go`

- [ ] **Step 1: Add interface + slog default + noop impl**

Append to `modules/world/bridges.go` (after the `LoggerBridge` interface, before `noopBridges`):

```go

// FriendsDispatcher is the world-side sink for server -> world friends
// updates received over the SubscribeUpdates stream. Production impl
// (slogFriendsDispatcher, below) logs each event at Debug; the
// in-game ServerGameProt packet emit (UPDATE_FRIENDLIST /
// UPDATE_IGNORELIST / MESSAGE_PRIVATE writes to the player's client
// connection) is gated on NAI-182-D5 (the "social cluster"
// ServerGameProt deferral noted at tick.go:226).
//
// NAI-S4A-D-NO-INGAME-PACKET-EMIT — retires when NAI-182-D5 retires
// and the dispatcher is wired through to player.write(...).
type FriendsDispatcher interface {
	OnFriendlistUpdate(viewer uint64, entries []*friendspb.FriendEntry)
	OnIgnorelistUpdate(viewer uint64, ignored []uint64)
	OnPrivateMessage(target uint64, from uint64, staffLvl int32, pmId uint32, chat string)
}

// slogFriendsDispatcher is the default FriendsDispatcher. Logs each
// event at Debug; does NOT emit ServerGameProt packets to the player.
// See NAI-S4A-D-NO-INGAME-PACKET-EMIT above.
type slogFriendsDispatcher struct {
	log *slog.Logger
}

func newSlogFriendsDispatcher(log *slog.Logger) FriendsDispatcher {
	return &slogFriendsDispatcher{log: log}
}

func (d *slogFriendsDispatcher) OnFriendlistUpdate(viewer uint64, entries []*friendspb.FriendEntry) {
	d.log.Debug("friends dispatch: friendlist update",
		slog.Uint64("viewer", viewer),
		slog.Int("entries", len(entries)))
}

func (d *slogFriendsDispatcher) OnIgnorelistUpdate(viewer uint64, ignored []uint64) {
	d.log.Debug("friends dispatch: ignorelist update",
		slog.Uint64("viewer", viewer),
		slog.Int("ignored", len(ignored)))
}

func (d *slogFriendsDispatcher) OnPrivateMessage(target uint64, from uint64, staffLvl int32, pmId uint32, chat string) {
	d.log.Debug("friends dispatch: private message",
		slog.Uint64("target", target),
		slog.Uint64("from", from),
		slog.Uint64("pm_id", uint64(pmId)))
}
```

- [ ] **Step 2: Extend noopBridges to also implement FriendsDispatcher**

Find `noopBridges` (around line 69 — current methods stop at SubmitSessionLogs). Append:

```go
func (noopBridges) OnFriendlistUpdate(uint64, []*friendspb.FriendEntry)          {}
func (noopBridges) OnIgnorelistUpdate(uint64, []uint64)                          {}
func (noopBridges) OnPrivateMessage(uint64, uint64, int32, uint32, string)       {}
```

`friendspb` should already be imported in bridges.go (slice 2). If not, add it.

- [ ] **Step 3: Compile-check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/world/...`
Expected: builds clean.

- [ ] **Step 4: Commit**

```bash
git add modules/world/bridges.go
git commit --no-gpg-sign -m "world: add FriendsDispatcher interface + slog default impl"
```

---

### Task 13: Add `friends_subscriber.go` (supervisor + dispatch)

**Files:**
- Create: `modules/world/friends_subscriber.go`

- [ ] **Step 1: Write the subscriber**

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

// friendsSubscriberBackoffMin is the initial reconnect delay after a
// stream failure. Doubles up to friendsSubscriberBackoffMax; resets to
// the min after the most recent run lasted ≥ friendsSubscriberSteady.
// Mirrors the [[content-watcher-auto-restart]] supervisor cadence.
const (
	friendsSubscriberBackoffMin = time.Second
	friendsSubscriberBackoffMax = 30 * time.Second
	friendsSubscriberSteady     = 60 * time.Second
)

// friendsSubscriber owns one player's SubscribeUpdates stream lifetime.
// Started by world.Server when the player is admitted to the world;
// stopped by canceling its ctx when the player logs out / disconnects.
//
// Each iteration:
//   - SubscribeUpdates(ctx, req) → stream
//   - Recv loop dispatches updates to FriendsDispatcher
//   - On error/EOF: log, exp-backoff, reconnect (unless ctx canceled)
type friendsSubscriber struct {
	client     FriendsClient
	worldID    int32
	username37 uint64
	dispatcher FriendsDispatcher
	log        *slog.Logger
}

func newFriendsSubscriber(client FriendsClient, worldID int32, username37 uint64, dispatcher FriendsDispatcher, log *slog.Logger) *friendsSubscriber {
	return &friendsSubscriber{
		client:     client,
		worldID:    worldID,
		username37: username37,
		dispatcher: dispatcher,
		log:        log,
	}
}

// run is the supervisor loop. Blocks until ctx is canceled. Caller
// should typically invoke as `go sub.run(ctx)`.
func (s *friendsSubscriber) run(ctx context.Context) {
	backoff := friendsSubscriberBackoffMin
	for {
		if ctx.Err() != nil {
			return
		}
		runStart := time.Now()
		err := s.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		// Reset backoff if the failed run lasted long enough to count
		// as "steady". Distinct from a fast-fail loop that should keep
		// the longer backoff.
		if time.Since(runStart) >= friendsSubscriberSteady {
			backoff = friendsSubscriberBackoffMin
		}
		// EOF means the server closed cleanly (e.g., we got kicked by a
		// newer subscriber for the same username37). Log at Info rather
		// than Warn.
		if errors.Is(err, io.EOF) {
			s.log.Info("friends subscriber EOF; reconnecting",
				slog.Uint64("username37", s.username37),
				slog.Duration("backoff", backoff))
		} else {
			s.log.Warn("friends subscriber disconnected; reconnecting",
				slog.Uint64("username37", s.username37),
				slog.Duration("backoff", backoff),
				slog.Any("err", err))
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = nextBackoff(backoff)
	}
}

func nextBackoff(d time.Duration) time.Duration {
	d *= 2
	if d > friendsSubscriberBackoffMax {
		d = friendsSubscriberBackoffMax
	}
	return d
}

// runOnce opens a single stream and drains it. Returns when the stream
// ends (error or EOF).
func (s *friendsSubscriber) runOnce(ctx context.Context) error {
	stream, err := s.client.SubscribeUpdates(ctx, &friendspb.SubscribeUpdatesRequest{
		WorldId:    s.worldID,
		Username37: s.username37,
	})
	if err != nil {
		return err
	}
	for {
		u, err := stream.Recv()
		if err != nil {
			return err
		}
		s.dispatch(u)
	}
}

// dispatch routes one FriendsUpdate to the appropriate dispatcher
// method based on the oneof variant.
func (s *friendsSubscriber) dispatch(u *friendspb.FriendsUpdate) {
	switch v := u.Update.(type) {
	case *friendspb.FriendsUpdate_Friendlist:
		s.dispatcher.OnFriendlistUpdate(s.username37, v.Friendlist.Entries)
	case *friendspb.FriendsUpdate_Ignorelist:
		s.dispatcher.OnIgnorelistUpdate(s.username37, v.Ignorelist.Username37)
	case *friendspb.FriendsUpdate_PrivateMessage:
		pm := v.PrivateMessage
		s.dispatcher.OnPrivateMessage(s.username37, pm.FromUsername37, pm.StaffLvl, pm.PmId, pm.Chat)
	default:
		s.log.Warn("friends subscriber received unknown update variant",
			slog.Uint64("username37", s.username37))
	}
}
```

- [ ] **Step 2: Compile-check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./modules/world/...`
Expected: builds clean.

- [ ] **Step 3: Commit**

```bash
git add modules/world/friends_subscriber.go
git commit --no-gpg-sign -m "world: add friendsSubscriber with exp-backoff supervisor"
```

---

### Task 14: Add `friends_subscriber_test.go`

**Files:**
- Create: `modules/world/friends_subscriber_test.go`

- [ ] **Step 1: Write the tests**

```go
package world

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/friendspb"
)

// recordingFriendsDispatcher captures dispatch calls under mu.
type recordingFriendsDispatcher struct {
	mu       sync.Mutex
	friend   []friendlistCall
	ignore   []ignorelistCall
	private  []privateCall
}

type friendlistCall struct {
	Viewer  uint64
	Entries []*friendspb.FriendEntry
}
type ignorelistCall struct {
	Viewer  uint64
	Ignored []uint64
}
type privateCall struct {
	Target, From uint64
	StaffLvl     int32
	PmId         uint32
	Chat         string
}

func (d *recordingFriendsDispatcher) OnFriendlistUpdate(v uint64, e []*friendspb.FriendEntry) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.friend = append(d.friend, friendlistCall{Viewer: v, Entries: e})
}
func (d *recordingFriendsDispatcher) OnIgnorelistUpdate(v uint64, ig []uint64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ignore = append(d.ignore, ignorelistCall{Viewer: v, Ignored: ig})
}
func (d *recordingFriendsDispatcher) OnPrivateMessage(target, from uint64, staffLvl int32, pmId uint32, chat string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.private = append(d.private, privateCall{Target: target, From: from, StaffLvl: staffLvl, PmId: pmId, Chat: chat})
}

func (d *recordingFriendsDispatcher) friendCalls() []friendlistCall {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]friendlistCall(nil), d.friend...)
}

func TestFriendsSubscriber_DispatchesFriendlist(t *testing.T) {
	fc := newFakeFriendsClient()
	disp := &recordingFriendsDispatcher{}
	sub := newFriendsSubscriber(fc, 1, 42, disp, discardLogger())

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); sub.run(ctx) }()

	// Wait until the fake client has produced a stream.
	deadline := time.Now().Add(2 * time.Second)
	for fc.lastStream == nil {
		if time.Now().After(deadline) {
			t.Fatalf("fake stream never created")
		}
		time.Sleep(time.Millisecond)
	}

	// Push a FriendlistUpdate.
	fc.lastStream.recv <- &friendspb.FriendsUpdate{
		Update: &friendspb.FriendsUpdate_Friendlist{
			Friendlist: &friendspb.FriendlistUpdate{
				Entries: []*friendspb.FriendEntry{{WorldId: 7, Username37: 99}},
			},
		},
	}

	// Wait for dispatch.
	deadline = time.Now().Add(2 * time.Second)
	for len(disp.friendCalls()) == 0 {
		if time.Now().After(deadline) {
			t.Fatalf("dispatcher never invoked")
		}
		time.Sleep(time.Millisecond)
	}

	calls := disp.friendCalls()
	if calls[0].Viewer != 42 {
		t.Fatalf("viewer = %d, want 42", calls[0].Viewer)
	}
	if len(calls[0].Entries) != 1 || calls[0].Entries[0].WorldId != 7 || calls[0].Entries[0].Username37 != 99 {
		t.Fatalf("entries = %v, want [(7, 99)]", calls[0].Entries)
	}

	cancel()
	<-done
}

func TestFriendsSubscriber_CtxCancelStopsCleanly(t *testing.T) {
	fc := newFakeFriendsClient()
	disp := &recordingFriendsDispatcher{}
	sub := newFriendsSubscriber(fc, 1, 42, disp, discardLogger())

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); sub.run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for fc.lastStream == nil {
		if time.Now().After(deadline) {
			t.Fatalf("fake stream never created")
		}
		time.Sleep(time.Millisecond)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("sub.run did not return on ctx cancel")
	}
}

func TestFriendsSubscriber_EOFTriggersReconnect(t *testing.T) {
	fc := newFakeFriendsClient()
	disp := &recordingFriendsDispatcher{}
	sub := newFriendsSubscriber(fc, 1, 42, disp, discardLogger())

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	done := make(chan struct{})
	go func() { defer close(done); sub.run(ctx) }()

	// First stream appears.
	deadline := time.Now().Add(2 * time.Second)
	for fc.lastStream == nil {
		if time.Now().After(deadline) {
			t.Fatalf("first stream never created")
		}
		time.Sleep(time.Millisecond)
	}
	firstStream := fc.lastStream

	// Simulate clean EOF.
	close(firstStream.recv)

	// Wait for a new stream to appear (note: backoff is 1s first time;
	// test waits longer).
	deadline = time.Now().Add(3 * time.Second)
	for fc.lastStream == firstStream {
		if time.Now().After(deadline) {
			t.Fatalf("supervisor did not reconnect after EOF")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestNextBackoff_DoublesAndCaps(t *testing.T) {
	got := nextBackoff(time.Second)
	if got != 2*time.Second {
		t.Errorf("nextBackoff(1s) = %v, want 2s", got)
	}
	got = nextBackoff(16 * time.Second)
	if got != friendsSubscriberBackoffMax {
		t.Errorf("nextBackoff(16s) = %v, want %v", got, friendsSubscriberBackoffMax)
	}
	got = nextBackoff(friendsSubscriberBackoffMax)
	if got != friendsSubscriberBackoffMax {
		t.Errorf("nextBackoff(max) = %v, want %v (cap)", got, friendsSubscriberBackoffMax)
	}
}

func TestFriendsSubscriber_DispatchesIgnorelist(t *testing.T) {
	fc := newFakeFriendsClient()
	disp := &recordingFriendsDispatcher{}
	sub := newFriendsSubscriber(fc, 1, 42, disp, discardLogger())

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); sub.run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })

	deadline := time.Now().Add(2 * time.Second)
	for fc.lastStream == nil {
		if time.Now().After(deadline) {
			t.Fatalf("fake stream never created")
		}
		time.Sleep(time.Millisecond)
	}

	fc.lastStream.recv <- &friendspb.FriendsUpdate{
		Update: &friendspb.FriendsUpdate_Ignorelist{
			Ignorelist: &friendspb.IgnorelistUpdate{Username37: []uint64{100, 200}},
		},
	}

	deadline = time.Now().Add(2 * time.Second)
	for {
		disp.mu.Lock()
		n := len(disp.ignore)
		disp.mu.Unlock()
		if n > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("dispatcher OnIgnorelistUpdate never called")
		}
		time.Sleep(time.Millisecond)
	}

	disp.mu.Lock()
	defer disp.mu.Unlock()
	if disp.ignore[0].Viewer != 42 {
		t.Errorf("viewer = %d, want 42", disp.ignore[0].Viewer)
	}
	if len(disp.ignore[0].Ignored) != 2 {
		t.Errorf("ignored len = %d, want 2", len(disp.ignore[0].Ignored))
	}
}

// Smoke test that runOnce returns the error from SubscribeUpdates when
// the client fails to dial. (Exercises the supervisor's error path.)
func TestFriendsSubscriber_RunOnce_DialErrorPropagates(t *testing.T) {
	fc := newFakeFriendsClient()
	fc.subscribeErr = io.ErrUnexpectedEOF
	sub := newFriendsSubscriber(fc, 1, 42, &recordingFriendsDispatcher{}, discardLogger())

	err := sub.runOnce(t.Context())
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("err = %v, want io.ErrUnexpectedEOF", err)
	}
}
```

- [ ] **Step 2: Run tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -run TestFriendsSubscriber -v -timeout 30s`
Expected: PASS (6 tests). The EOF-reconnect test may take up to ~2s due to the 1s backoff.

- [ ] **Step 3: Commit**

```bash
git add modules/world/friends_subscriber_test.go
git commit --no-gpg-sign -m "world: test friendsSubscriber lifecycle + dispatch + backoff"
```

---

## Phase 3 — Lifecycle wiring + end-to-end

### Task 15: Wire subscriber start at PlayerLogin site + dispatcher into Server

**Files:**
- Modify: `modules/world/server.go`
- Modify: `modules/world/tick.go`

We need:
1. A new field `s.friendsDispatcher FriendsDispatcher` on Server, initialized in NewServer with `newSlogFriendsDispatcher` (and overridable via test).
2. A new field `p.friendsSub *friendsSubscriber` + `p.friendsSubCancel context.CancelFunc` on Player.
3. At the tick.go PlayerLogin site (line 170), after the existing fire-and-forget `friendsClient.PlayerLogin`, also start the subscriber.

- [ ] **Step 1: Add `friendsDispatcher` field to Server**

In `modules/world/server.go`, add to the Server struct near `friendsBridge` (around line 164):

```go
	friendsBridge     FriendsBridge
	friendsDispatcher FriendsDispatcher
```

In `NewServer` (around line 277), after the friendsBridge line, add:

```go
	s.friendsBridge = defaultFriendsBridge(friendsClient, int32(cfg.NodeID), s.log)
	s.friendsDispatcher = newSlogFriendsDispatcher(s.log)
```

- [ ] **Step 2: Add subscriber fields to Player**

Search for the Player struct definition:

Run: `grep -n "type Player struct" modules/world/player.go`

Add (alongside other lifecycle fields — wherever feels natural):

```go
	// friendsSub is the per-player SubscribeUpdates subscription. Set
	// at PlayerLogin (after the world admits the player); torn down by
	// canceling friendsSubCancel at logout/disconnect. Nil when
	// friendsClient is nil (FriendsServerEnabled=false).
	friendsSub       *friendsSubscriber
	friendsSubCancel context.CancelFunc
```

If `context` is not imported in player.go, add it.

- [ ] **Step 3: Start the subscriber at the tick.go PlayerLogin site**

In `modules/world/tick.go`, after the existing `friendsClient.PlayerLogin` call (after line 177), add:

```go
			// NAI-S4A: start the SubscribeUpdates stream subscriber.
			// Lives until logout/disconnect cancels p.friendsSubCancel.
			if s.friendsClient != nil && p.username != "" {
				subCtx, subCancel := context.WithCancel(context.Background())
				p.friendsSubCancel = subCancel
				p.friendsSub = newFriendsSubscriber(s.friendsClient, int32(s.cfg.NodeID), p.username37, s.friendsDispatcher, s.log)
				go p.friendsSub.run(subCtx)
			}
```

- [ ] **Step 4: Compile-check**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/world/...`
Expected: builds clean.

- [ ] **Step 5: Commit**

```bash
git add modules/world/server.go modules/world/player.go modules/world/tick.go
git commit --no-gpg-sign -m "world: start friendsSubscriber on player admit"
```

---

### Task 16: Wire subscriber stop into both logout paths

**Files:**
- Modify: `modules/world/server.go`

Both `removePlayerOnTick` (line 954) and `removePlayerOnDisconnect` (line 994) must cancel the subscriber's ctx. Cleanest spot: just before `s.removePlayerInternal(p)` (both ~line 981 / ~line 1009).

- [ ] **Step 1: Add cancel call in `removePlayerOnTick`**

Before `s.removePlayerInternal(p)` at the end (around line 981):

```go
	if p.friendsSubCancel != nil {
		p.friendsSubCancel()
		p.friendsSubCancel = nil
	}
	s.removePlayerInternal(p)
}
```

- [ ] **Step 2: Add the same cancel call in `removePlayerOnDisconnect`**

Before `s.removePlayerInternal(p)` (around line 1009):

```go
	if p.friendsSubCancel != nil {
		p.friendsSubCancel()
		p.friendsSubCancel = nil
	}
	s.removePlayerInternal(p)
}
```

- [ ] **Step 3: Build + existing world tests still pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -count=1 -timeout 120s`
Expected: PASS. Watch for any test that constructs Player by-value without `friendsSubCancel`; the nil-check above protects all callers.

- [ ] **Step 4: Commit**

```bash
git add modules/world/server.go
git commit --no-gpg-sign -m "world: cancel friendsSubscriber ctx on logout/disconnect"
```

---

### Task 17: Extend friends_smoke_test for stream e2e

**Files:**
- Modify: `modules/world/friends_smoke_test.go`

Existing smoke covers fire-and-forget RPCs end-to-end. Add an assertion that SubscribeUpdates produces the initial snapshots and reacts to a follower's PlayerLogin.

- [ ] **Step 1: Add a new e2e test function**

Append to `modules/world/friends_smoke_test.go`:

```go

// TestFriendsClient_E2E_SubscribeUpdatesStream verifies the slice-4a
// stream end-to-end: open SubscribeUpdates for viewer A, then trigger
// follower B's PlayerLogin and assert A's stream sees a FriendlistUpdate
// naming B with B's world.
//
// Boots an in-process friends.Friends with a t.TempDir-backed SQLite.
// Mirrors TestFriendsClient_E2E_SmokeAgainstFriendsServer.
func TestFriendsClient_E2E_SubscribeUpdatesStream(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
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

	// World setup.
	client.WorldConnect(ctx, 10, "main")

	// A logs in.
	client.PlayerLogin(ctx, &friendspb.PlayerLoginRequest{
		WorldId: 10, Username37: 1111, PrivateChat: 0, StaffLvl: 0,
	})
	// A friends B.
	client.FriendlistAdd(ctx, &friendspb.FriendlistAddRequest{
		WorldId: 10, Username37: 1111, TargetUsername37: 2222,
	})

	// A subscribes via the world-side subscriber.
	disp := &recordingFriendsDispatcher{}
	sub := newFriendsSubscriber(client, 10, 1111, disp, log)
	subCtx, subCancel := context.WithCancel(ctx)
	t.Cleanup(subCancel)
	go sub.run(subCtx)

	// Initial snapshot includes B (world=0 since not logged in).
	if !waitForFriendlistEntry(t, disp, 2*time.Second, 2222) {
		t.Fatalf("initial snapshot missing friend 2222")
	}

	// B logs in. A should see B's world.
	client.PlayerLogin(ctx, &friendspb.PlayerLoginRequest{
		WorldId: 10, Username37: 2222, PrivateChat: 0, StaffLvl: 0,
	})
	if !waitForFriendlistEntryWithWorld(t, disp, 2*time.Second, 2222, 10) {
		t.Fatalf("expected FriendlistUpdate naming 2222 on world 10")
	}

	// B logs out. A should see world=0.
	client.PlayerLogout(ctx, &friendspb.PlayerLogoutRequest{
		WorldId: 10, Username37: 2222,
	})
	if !waitForFriendlistEntryWithWorld(t, disp, 2*time.Second, 2222, 0) {
		t.Fatalf("expected FriendlistUpdate naming 2222 on world 0 after logout")
	}
}

// waitForFriendlistEntry polls disp for any FriendlistUpdate that
// includes target. Returns true within d.
func waitForFriendlistEntry(t *testing.T, disp *recordingFriendsDispatcher, d time.Duration, target uint64) bool {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		for _, c := range disp.friendCalls() {
			for _, e := range c.Entries {
				if e.Username37 == target {
					return true
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// waitForFriendlistEntryWithWorld polls disp for any FriendlistUpdate
// where target appears with the specified worldId.
func waitForFriendlistEntryWithWorld(t *testing.T, disp *recordingFriendsDispatcher, d time.Duration, target uint64, worldId int32) bool {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		for _, c := range disp.friendCalls() {
			for _, e := range c.Entries {
				if e.Username37 == target && e.WorldId == worldId {
					return true
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}
```

- [ ] **Step 2: Run the smoke test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -run TestFriendsClient_E2E_SubscribeUpdatesStream -v -timeout 60s`
Expected: PASS.

- [ ] **Step 3: Run all friends-related tests together to catch interactions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/ ./modules/world/ -count=1 -timeout 180s`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add modules/world/friends_smoke_test.go
git commit --no-gpg-sign -m "world: smoke-test SubscribeUpdates stream e2e (initial + broadcast)"
```

---

## Phase 4 — Tag housekeeping + project-wide verification

### Task 18: Retire `NAI-S1-D-NO-FOLLOWER-BROADCAST` and open the 3 new tags

**Files:**
- Modify: `modules/friends/handler.go` (delete retiring tag references already handled in T8; this task searches the broader codebase for stragglers)
- Search: `grep -rn "NAI-S1-D-NO-FOLLOWER-BROADCAST" .`

- [ ] **Step 1: Search for any remaining references to NAI-S1-D-NO-FOLLOWER-BROADCAST**

Run: `grep -rn "NAI-S1-D-NO-FOLLOWER-BROADCAST" --include="*.go" .`

Expected: zero matches after T8. If any remain, replace each comment occurrence:

Before:
```go
// NAI-S1-D-NO-FOLLOWER-BROADCAST — slice 4 will broadcast; slice 1 just mutates state.
```

After:
```go
// Broadcast wiring lives in handler.broadcastWorldToFollowers (slice 4a).
```

- [ ] **Step 2: Verify the three new tags are present in source**

Run: `grep -rn "NAI-S4A-D-DROP-ON-FULL\|NAI-S4A-D-NO-INGAME-PACKET-EMIT\|NAI-S4A-D-PERPLAYER-NOT-PERWORLD-STREAM" --include="*.go" .`

Expected:
- `NAI-S4A-D-DROP-ON-FULL` in `modules/friends/subscriptions.go` (subscriberBufferSize doc).
- `NAI-S4A-D-NO-INGAME-PACKET-EMIT` in `modules/world/bridges.go` (FriendsDispatcher doc).
- `NAI-S4A-D-PERPLAYER-NOT-PERWORLD-STREAM` is documented spec-only (per spec §2). Verify no code currently references it; that is correct (the tag covers an architectural choice, not a code site).

If `NAI-S4A-D-PERPLAYER-NOT-PERWORLD-STREAM` is missing from source and the prior subagent didn't add it, append a one-line referencing comment to `modules/friends/handler.go`'s `SubscribeUpdates` method docstring:

```go
// NAI-S4A-D-PERPLAYER-NOT-PERWORLD-STREAM — proto-baked architectural
// choice; TS keeps one socket per world, goscape one stream per
// (world, player). Permanent.
```

- [ ] **Step 3: Commit if any tag-cleanup edits were made**

```bash
git add -A
git status # confirm only doc-comment changes
git commit --no-gpg-sign -m "friends: clean up retired/opened deviation tags for slice 4a"
```

If nothing changed, skip.

---

### Task 19: Full project gate (-race + smoke-pack)

- [ ] **Step 1: Run full -race**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... -count=1 -timeout 600s`
Expected: PASS (zero failures). Note any flaky tests; re-run once to confirm.

- [ ] **Step 2: Run smoke-pack**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape-cli smoke-pack --content-dir /home/owner/Code/github.com/LostCityRS/content`
Expected: `12 OK / 0 ERR / 0 SKIP`. If any stage degrades, that is a blocker — investigate before claiming slice 4a complete.

- [ ] **Step 3: Tag the slice close**

Final commit at end of slice (squash/no-squash — match prior slice cadence of one commit per logical chunk; no merge commit needed):

```bash
git log --oneline 112f8171..HEAD
# expect ~16-18 commits (Phase 1: 8-9, Phase 2: 4-5, Phase 3: 3, Phase 4: 1-2)
```

Optional final marker commit if there's a doc to land separately. Otherwise the last test/wiring commit closes the slice.

---

## Self-review against spec

**Spec coverage map:**

| Spec section | Task(s) |
|---|---|
| §0 Decomposition rationale | (informational; no task) |
| §1 Forward map | All tasks cover the listed files |
| §2 Stream identity per-(world, player) | T6 (SubscribeUpdates), T18 (tag) |
| §3 Server-side subscription registry | T1, T2 |
| §4 SubscribeUpdates handler | T6 |
| §5 broadcastWorldToFollowers + sendPlayerWorldUpdate | T7, T8 (wiring), T3+T4 (IsVisibleToMany) |
| §6 World-side subscriber + supervisor | T13, T14 |
| §6 FriendsClient extension | T10, T11 |
| §6 FriendsDispatcher | T12 |
| §6 Lifecycle wiring | T15, T16 |
| §7 Testing | T2, T4, T9, T11, T14, T17 |
| §7 e2e smoke | T17 |
| §7 -race + smoke-pack gates | T19 |
| §8 Open arch questions resolved | All baked into design |
| §9 Deviation tags | T8 (retirement), T18 (housekeeping) |

**Type consistency check:**
- `subscriber` (struct), `subscriptions` (registry), `newSubscriber`, `newSubscriptions`, `register`, `deregister`, `send` — used consistently across T1-T9.
- `FriendsDispatcher` methods `OnFriendlistUpdate(viewer, entries)` / `OnIgnorelistUpdate(viewer, ignored)` / `OnPrivateMessage(target, from, staffLvl, pmId, chat)` — defined T12, consumed T13, asserted T14/T17.
- `friendsSubscriber` (struct), `newFriendsSubscriber(client, worldID, username37, dispatcher, log)`, `run(ctx)`, `runOnce(ctx)`, `dispatch(u)` — consistent T13/T14/T15/T17.
- `IsVisibleToMany(ctx, viewers, other) (map[uint64]bool, error)` — T3 defines, T4 tests, T7 consumes (in broadcastWorldToFollowers).
- `broadcastWorldToFollowers(ctx, other)` (no error return; logs internally) — T7 defines, T8 wires, T9 asserts.

**Placeholder scan:** no TBDs, no "TODO", no "similar to Task N", all steps have concrete code blocks.

---

## Notes for executor

- **Pull lifecycle forward when needed.** Slice 3's lesson: T6 of slice 3 lit a code path requiring DB-open lifecycle that wasn't scheduled until T11, causing a world-e2e regression. For this plan, T15 (subscriber start) lights up a code path that depends on T12 (FriendsDispatcher type) and T13 (friendsSubscriber type) and T10 (FriendsClient.SubscribeUpdates). All three are in earlier tasks — but if you run T15 ahead of order, you will see compile failures. Stick to the task order.
- **Per-task test scope.** When running tests in T8, T15, T16, run `./modules/friends/ ./modules/world/` together — those are the change surface. Phase 4's T19 widens to `./...` once Phase 3 is green.
- **Reviewer fences:** between tasks, verify the commit graph is clean (one commit per task, no working-tree leftovers). Slice 3's `[[working_tree_amend_silent_fail]]` lesson applies — confirm SHAs with `git show <sha> --stat`.
- **Backoff timing in tests.** T14's EOF-reconnect test sleeps until the next stream appears (~1s initial backoff). If the test is flaky on slow CI, raise the deadline to 3s. Do NOT shorten `friendsSubscriberBackoffMin` — that's a production constant.
- **Drop log noise.** The drop-on-full Warn log will be loud in burst scenarios. Acceptable for slice 4a (visibility into a real degraded condition); if it becomes spammy in load testing, add a rate-limit later.
