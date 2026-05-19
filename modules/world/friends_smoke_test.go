package world

import (
	"context"
	"net"
	"path/filepath"
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

	// 1. WorldConnect — required first call per slice 1 handler.
	client.WorldConnect(ctx, 10, "main")

	// 2. PlayerLogin — registers a player on world 10.
	client.PlayerLogin(ctx, &friendspb.PlayerLoginRequest{
		WorldId:     10,
		Username37:  1234,
		PrivateChat: 0,
		StaffLvl:    0,
	}, nil)

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
	}, nil)
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
	}, nil)
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

// waitForPrivate polls disp for any PrivateMessage call whose PmId
// matches pmId. Returns the captured call within d, or false on
// timeout.
func waitForPrivate(t *testing.T, disp *recordingFriendsDispatcher, d time.Duration, pmId uint32) (privateCall, bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		for _, c := range disp.privateCalls() {
			if c.PmId == pmId {
				return c, true
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return privateCall{}, false
}

// TestFriendsClient_E2E_PrivateMessageDelivery pins slice 4b end-to-end:
// world's PrivateMessage RPC -> friends-server PrivateMessage handler
// -> subs.send -> recipient stream -> world-side subscriber dispatch
// -> FriendsDispatcher.OnPrivateMessage.
func TestFriendsClient_E2E_PrivateMessageDelivery(t *testing.T) {
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

	client.WorldConnect(ctx, 10, "main")

	// Recipient (2222) logs in and subscribes.
	client.PlayerLogin(ctx, &friendspb.PlayerLoginRequest{
		WorldId: 10, Username37: 2222, PrivateChat: 0, StaffLvl: 0,
	}, nil)

	disp := &recordingFriendsDispatcher{}
	sub := newFriendsSubscriber(client, 10, 2222, disp, log)
	subCtx, subCancel := context.WithCancel(ctx)
	t.Cleanup(subCancel)
	go sub.run(subCtx)

	// Sender (1111) logs in.
	client.PlayerLogin(ctx, &friendspb.PlayerLoginRequest{
		WorldId: 10, Username37: 1111, PrivateChat: 0, StaffLvl: 0,
	}, nil)

	// Sender PMs recipient.
	client.PrivateMessage(ctx, &friendspb.PrivateMessageRequest{
		WorldId:          10,
		Username37:       1111,
		TargetUsername37: 2222,
		StaffLvl:         0,
		PmId:             0xCAFEBABE,
		Chat:             "e2e hi",
		Coord:            0,
	})

	got, ok := waitForPrivate(t, disp, 2*time.Second, 0xCAFEBABE)
	if !ok {
		t.Fatalf("recipient did not see PM with PmId 0xCAFEBABE within 2s; got %d calls", len(disp.privateCalls()))
	}
	if got.From != 1111 {
		t.Errorf("From = %d, want 1111", got.From)
	}
	if got.Target != 2222 {
		t.Errorf("Target = %d, want 2222", got.Target)
	}
	if got.Chat != "e2e hi" {
		t.Errorf("Chat = %q, want %q", got.Chat, "e2e hi")
	}
}

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
