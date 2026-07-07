package world

import (
	"context"
	"io"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/friendspb"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
)

// orderRecordingFriendsClient wraps a fakeFriendsClient and additionally
// records the method name of every mutation call, in call order, under mu.
// fakeFriendsClient's own per-method channels already let a test assert
// "this specific request arrived"; this wrapper adds the one thing they
// cannot: a single global order across DIFFERENT RPC types, needed to prove
// the dispatcher's worker drains heterogeneous queued mutations in strict
// enqueue order (TestFriendsMutationsRouteThroughDispatcher).
type orderRecordingFriendsClient struct {
	*fakeFriendsClient
	mu    sync.Mutex
	order []string
}

func newOrderRecordingFriendsClient() *orderRecordingFriendsClient {
	return &orderRecordingFriendsClient{fakeFriendsClient: newFakeFriendsClient()}
}

func (o *orderRecordingFriendsClient) record(name string) {
	o.mu.Lock()
	o.order = append(o.order, name)
	o.mu.Unlock()
}

// snapshotOrder returns a copy of the recorded call order so far.
func (o *orderRecordingFriendsClient) snapshotOrder() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.order...)
}

func (o *orderRecordingFriendsClient) PlayerLogin(ctx context.Context, req *friendspb.PlayerLoginRequest, onResponse func(accepted bool)) {
	o.record("PlayerLogin")
	o.fakeFriendsClient.PlayerLogin(ctx, req, onResponse)
}

func (o *orderRecordingFriendsClient) PlayerLogout(ctx context.Context, req *friendspb.PlayerLogoutRequest) {
	o.record("PlayerLogout")
	o.fakeFriendsClient.PlayerLogout(ctx, req)
}

func (o *orderRecordingFriendsClient) ChatSetMode(ctx context.Context, req *friendspb.ChatSetModeRequest) {
	o.record("ChatSetMode")
	o.fakeFriendsClient.ChatSetMode(ctx, req)
}

func (o *orderRecordingFriendsClient) FriendlistAdd(ctx context.Context, req *friendspb.FriendlistAddRequest) {
	o.record("FriendlistAdd")
	o.fakeFriendsClient.FriendlistAdd(ctx, req)
}

func (o *orderRecordingFriendsClient) FriendlistDel(ctx context.Context, req *friendspb.FriendlistDelRequest) {
	o.record("FriendlistDel")
	o.fakeFriendsClient.FriendlistDel(ctx, req)
}

func (o *orderRecordingFriendsClient) IgnorelistAdd(ctx context.Context, req *friendspb.IgnorelistAddRequest) {
	o.record("IgnorelistAdd")
	o.fakeFriendsClient.IgnorelistAdd(ctx, req)
}

func (o *orderRecordingFriendsClient) IgnorelistDel(ctx context.Context, req *friendspb.IgnorelistDelRequest) {
	o.record("IgnorelistDel")
	o.fakeFriendsClient.IgnorelistDel(ctx, req)
}

func (o *orderRecordingFriendsClient) PrivateMessage(ctx context.Context, req *friendspb.PrivateMessageRequest) {
	o.record("PrivateMessage")
	o.fakeFriendsClient.PrivateMessage(ctx, req)
}

var _ FriendsClient = (*orderRecordingFriendsClient)(nil)

// TestFriendsMutationsRouteThroughDispatcher pins arch-29.13's routing
// contract at the granularity of INDIVIDUAL CALL SITES: every friends
// mutation call site (the 6 grpcFriendsBridge mutation methods, tick.go's
// PlayerLogin dispatch in processLogins, and server.go's PlayerLogout
// dispatch in removePlayerOnTick) must enqueue on the friendsMutationDispatcher
// rather than fire its own goroutine.
//
// The existing FIFO-ordering tests (TestFriendsDispatchPreservesLoginLogoutOrder
// et al., friends_dispatcher_test.go) prove the WORKER is serial once
// something reaches the queue, but nothing in the suite fails if a call site
// regresses to "go func() { ... }()" — such a regression would still (from
// the fake's point of view) eventually deliver the RPC, just out of band from
// the dispatcher, so a test that only checks "the fake eventually received
// the right request" cannot tell the difference. This test can, by never
// starting the dispatcher's worker: with nothing to dequeue the queue,
// (a) the fake must receive ZERO calls no matter how long we wait, and
// (b) the dispatcher's queue depth must grow by exactly one per call site
// invoked. A reverted call site fails (a) immediately (a fire-and-forget
// goroutine calls the fake directly) and fails (b) too (nothing was
// enqueued, so depth does not move).
//
// Non-vacuity is on record in this task's report: AddFriend was temporarily
// reverted to a bare "go func(){ b.client.FriendlistAdd(...) }()" and this
// test failed, naming "AddFriend" in both the zero-calls and depth-growth
// assertions, before the revert was undone.
func TestFriendsMutationsRouteThroughDispatcher(t *testing.T) {
	// startWorker=false: the queue is never drained until this test
	// explicitly starts+drains it at the very end, so every assertion up to
	// that point observes enqueue's effect in isolation.
	s := newTestServerWithDispatcher(t, false)
	s.cfg.NodeID = 10
	s.cfg.NodeProfile = "main"
	d := s.friendsMutationDispatcher

	fake := newOrderRecordingFriendsClient()
	s.friendsClient = fake

	bridge := &grpcFriendsBridge{client: fake, worldID: testWorldID, profile: "main", dispatcher: d, log: discardLogger()}

	wantDepth := 0
	assertRoutedThroughDispatcher := func(t *testing.T, site string) {
		t.Helper()
		if got := fake.snapshotOrder(); len(got) != 0 {
			t.Fatalf("%s: fake received a call with the dispatcher worker never started — this call site is not routing through friendsMutationDispatcher.enqueue (fire-and-forget goroutine?); calls so far: %v", site, got)
		}
		wantDepth++
		if got := d.depth(); got != wantDepth {
			t.Fatalf("%s: dispatcher queue depth: got %d, want %d — this call site did not enqueue on friendsMutationDispatcher (fire-and-forget goroutine?)", site, got, wantDepth)
		}
	}

	// --- the 6 grpcFriendsBridge mutation methods ---
	bridge.AddFriend("alice", 1)
	assertRoutedThroughDispatcher(t, "AddFriend")

	bridge.RemoveFriend("alice", 1)
	assertRoutedThroughDispatcher(t, "RemoveFriend")

	bridge.AddIgnore("alice", 1)
	assertRoutedThroughDispatcher(t, "AddIgnore")

	bridge.RemoveIgnore("alice", 1)
	assertRoutedThroughDispatcher(t, "RemoveIgnore")

	bridge.SetChatMode("alice", 1)
	assertRoutedThroughDispatcher(t, "SetChatMode")

	bridge.PrivateMessage("alice", 0, 1, 1, "hi", 0)
	assertRoutedThroughDispatcher(t, "PrivateMessage")

	// --- tick.go processLogins' inline friends PlayerLogin dispatch ---
	loginConn, loginNetConn := newTestClient(t)
	loginConn.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	go io.Copy(io.Discard, loginNetConn)
	loginPlayer := newPlayer(loginConn)
	loginPlayer.username = "alice"
	loginPlayer.username37 = 1234
	s.appendNewPlayer(loginPlayer)
	s.processLogins()
	assertRoutedThroughDispatcher(t, "PlayerLogin (tick.go processLogins)")

	// --- server.go removePlayerOnTick's inline friends PlayerLogout dispatch ---
	logoutConn, _ := newTestClient(t)
	logoutPlayer := newPlayer(logoutConn)
	logoutPlayer.username = "bob"
	logoutPlayer.username37 = 5678
	if err := s.addPlayer(logoutPlayer); err != nil {
		t.Fatalf("addPlayer: %v", err)
	}
	s.removePlayerOnTick(logoutPlayer)
	assertRoutedThroughDispatcher(t, "PlayerLogout (server.go removePlayerOnTick)")

	// Now start the worker and drain the queue: every mutation enqueued
	// above must execute exactly once, strictly in enqueue order.
	workerCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		d.run(workerCtx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("dispatcher worker did not exit after cancel")
		}
	})

	wantOrder := []string{
		"FriendlistAdd", "FriendlistDel", "IgnorelistAdd", "IgnorelistDel",
		"ChatSetMode", "PrivateMessage",
		"PlayerLogin", "PlayerLogout",
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(fake.snapshotOrder()) >= len(wantOrder) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := fake.snapshotOrder(); !reflect.DeepEqual(got, wantOrder) {
		t.Fatalf("execution order after drain: got %v, want %v", got, wantOrder)
	}
	if got := d.depth(); got != 0 {
		t.Errorf("dispatcher depth after drain: got %d, want 0", got)
	}
}
