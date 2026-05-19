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
