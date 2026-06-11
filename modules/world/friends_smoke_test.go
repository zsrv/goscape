package world

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/zsrv/goscape/modules/friends"
	"github.com/zsrv/goscape/modules/login"
	"github.com/zsrv/goscape/pkg/friendspb"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/loginpb"
	"github.com/zsrv/goscape/pkg/script"
	jstring "github.com/zsrv/goscape/pkg/util/jstring"

	_ "modernc.org/sqlite"
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
		Profile:                 "main",
		GRPCListenAddress:       "127.0.0.1",
		GRPCListenPort:          port,
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
		Profile:     "main",
		Username37:  1234,
		PrivateChat: 0,
		StaffLvl:    0,
	}, nil)

	// 3. FriendlistAdd — adds target 5678 to player 1234's friend set.
	client.FriendlistAdd(ctx, &friendspb.FriendlistAddRequest{
		WorldId:          10,
		Profile:          "main",
		Username37:       1234,
		TargetUsername37: 5678,
	})

	// 4. ChatSetMode — coerces in-range value.
	client.ChatSetMode(ctx, &friendspb.ChatSetModeRequest{
		WorldId:     10,
		Profile:     "main",
		Username37:  1234,
		PrivateChat: 1, // FRIENDS
	})

	// 5. PrivateMessage — accepted-and-logged slice 1 contract.
	client.PrivateMessage(ctx, &friendspb.PrivateMessageRequest{
		WorldId:          10,
		Profile:          "main",
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
		Profile:    "main",
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
		Profile:                 "main",
		GRPCListenAddress:       "127.0.0.1",
		GRPCListenPort:          port,
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
		WorldId: 10, Profile: "main", Username37: 1111, PrivateChat: 0, StaffLvl: 0,
	}, nil)
	// A friends B.
	client.FriendlistAdd(ctx, &friendspb.FriendlistAddRequest{
		WorldId: 10, Profile: "main", Username37: 1111, TargetUsername37: 2222,
	})

	// A subscribes via the world-side subscriber.
	disp := &recordingFriendsDispatcher{}
	sub := newFriendsSubscriber(client, 10, "main", 1111, disp, log)
	subCtx, subCancel := context.WithCancel(ctx)
	t.Cleanup(subCancel)
	go sub.run(subCtx)

	// Initial snapshot includes B (world=0 since not logged in).
	if !waitForFriendlistEntry(t, disp, 2*time.Second, 2222) {
		t.Fatalf("initial snapshot missing friend 2222")
	}

	// B logs in. A should see B's world.
	client.PlayerLogin(ctx, &friendspb.PlayerLoginRequest{
		WorldId: 10, Profile: "main", Username37: 2222, PrivateChat: 0, StaffLvl: 0,
	}, nil)
	if !waitForFriendlistEntryWithWorld(t, disp, 2*time.Second, 2222, 10) {
		t.Fatalf("expected FriendlistUpdate naming 2222 on world 10")
	}

	// B logs out. A should see world=0.
	client.PlayerLogout(ctx, &friendspb.PlayerLogoutRequest{
		WorldId: 10, Profile: "main", Username37: 2222,
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
		Profile:                 "main",
		GRPCListenAddress:       "127.0.0.1",
		GRPCListenPort:          port,
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
		WorldId: 10, Profile: "main", Username37: 2222, PrivateChat: 0, StaffLvl: 0,
	}, nil)

	disp := &recordingFriendsDispatcher{}
	sub := newFriendsSubscriber(client, 10, "main", 2222, disp, log)
	subCtx, subCancel := context.WithCancel(ctx)
	t.Cleanup(subCancel)
	go sub.run(subCtx)

	// Sender (1111) logs in.
	client.PlayerLogin(ctx, &friendspb.PlayerLoginRequest{
		WorldId: 10, Profile: "main", Username37: 1111, PrivateChat: 0, StaffLvl: 0,
	}, nil)

	// Sender PMs recipient.
	client.PrivateMessage(ctx, &friendspb.PrivateMessageRequest{
		WorldId:          10,
		Profile:          "main",
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

// TestFriendsClient_E2E_PrivateMessagePersistsRow pins slice 6:
// a client.PrivateMessage call against a real in-process
// friends.Friends produces a row in private_chat under r.profile,
// queryable via a second *sql.DB open against the same on-disk file.
//
// This is the persistence half of the slice-4b-and-slice-6 chain;
// delivery is pinned by TestFriendsClient_E2E_PrivateMessageDelivery.
func TestFriendsClient_E2E_PrivateMessagePersistsRow(t *testing.T) {
	port := freePort(t)
	dbPath := filepath.Join(t.TempDir(), "friends.db")
	cfg := friends.Config{
		Profile:                 "main",
		GRPCListenAddress:       "127.0.0.1",
		GRPCListenPort:          port,
		WorldPlayerLimit:        100,
		Enable:                  true,
		GracefulShutdownTimeout: 5 * time.Second,
		SQLiteDSN:               dbPath,
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
	client.PlayerLogin(ctx, &friendspb.PlayerLoginRequest{
		WorldId: 10, Profile: "main", Username37: 2222, PrivateChat: 0, StaffLvl: 0,
	}, nil)
	client.PlayerLogin(ctx, &friendspb.PlayerLoginRequest{
		WorldId: 10, Profile: "main", Username37: 1111, PrivateChat: 0, StaffLvl: 0,
	}, nil)

	client.PrivateMessage(ctx, &friendspb.PrivateMessageRequest{
		WorldId:          10,
		Profile:          "main",
		Username37:       1111,
		TargetUsername37: 2222,
		StaffLvl:         0,
		PmId:             0x1234,
		Chat:             "persisted",
		Coord:            42,
	})

	// Open a second *sql.DB against the same file. Poll up to 2s for
	// the row — synchronous RPC completion should mean the row is
	// already committed, but WAL settling on a fresh file under -race
	// can take a few ms.
	rdb, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })

	deadline := time.Now().Add(2 * time.Second)
	var from, to int64
	var coord int32
	var msg string
	for time.Now().Before(deadline) {
		err := rdb.QueryRowContext(t.Context(),
			`SELECT from_username37, to_username37, coord, message
			 FROM private_chat
			 WHERE profile = 'main'
			 ORDER BY id DESC
			 LIMIT 1`).Scan(&from, &to, &coord, &msg)
		if err == nil {
			break
		}
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("query private_chat: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if from != 1111 || to != 2222 || coord != 42 || msg != "persisted" {
		t.Errorf("private_chat row = (%d, %d, %d, %q), want (1111, 2222, 42, %q)",
			from, to, coord, msg, "persisted")
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
		Profile:                 "main",
		GRPCListenAddress:       "127.0.0.1",
		GRPCListenPort:          port,
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
		Profile:                 "main",
		GRPCListenAddress:       "127.0.0.1",
		GRPCListenPort:          port,
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

	subA := newWorldEventsSubscriber(client, 1, "main", dispA, log)
	subB := newWorldEventsSubscriber(client, 2, "main", dispB, log)

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
		client.RelayKick(context.Background(), &friendspb.RelayKickRequest{TargetWorldId: 2, Username37: 9999, Profile: "main"})
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
		TargetWorldId: 2, Username37: 123, MutedUntilMs: 4567, Profile: "main",
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
		TargetWorldId: 2, DurationTicks: 50, Profile: "main",
	})
	select {
	case d := <-dispB.shutdown:
		if d != 50 {
			t.Fatalf("shutdown = %d", d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for shutdown on world B")
	}

	client.RelayReload(context.Background(), &friendspb.RelayReloadRequest{TargetWorldId: 2, Profile: "main"})
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
		TargetWorldId: 1, Message: "hello-A", Profile: "main",
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

// TestFriendsClient_E2E_RelayShutdownAppliesAction pins the slice-5b
// integration: a real friends-server fanouts RelayShutdown to a world
// whose actionWorldEventsDispatcher routes through WorldStateOps to
// *Server.rebootTimer — assert s.shutdownTick advances. Mirror for
// RelayReload (asserts rebuildReq receives a value).
func TestFriendsClient_E2E_RelayShutdownAppliesAction(t *testing.T) {
	port := freePort(t)
	cfg := friends.Config{
		Profile:                 "main",
		GRPCListenAddress:       "127.0.0.1",
		GRPCListenPort:          port,
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

	// Build a test *Server (sans TCP) with the production action
	// dispatcher wired against itself as WorldStateOps.
	s := newTestServer(t)
	s.currentTick = 100
	inner := newSlogWorldEventsDispatcher(log)
	dispatcher := newActionWorldEventsDispatcher(inner, s)
	const targetWorldID = 7
	sub := newWorldEventsSubscriber(client, targetWorldID, "main", dispatcher, log)
	subCtx, subCancel := context.WithCancel(context.Background())
	subDone := make(chan struct{})
	go func() { sub.run(subCtx); close(subDone) }()
	defer func() {
		subCancel()
		<-subDone
	}()

	// Give the SubscribeWorldEvents stream a moment to register on the
	// server's per-world subscriptions table. Slice-5a established that
	// stream handshake completes within a few hundred ms; use a fixed
	// settle wait rather than a probe loop (the action-routing chain
	// makes a probe-based readiness check awkward — drainRelayActions
	// has no visible side-effect on lookup-miss closures).
	time.Sleep(300 * time.Millisecond)

	// Issue RelayShutdown(duration=50) and assert shutdownTick advances.
	wantTick := s.currentTick + 50
	client.RelayShutdown(context.Background(), &friendspb.RelayShutdownRequest{
		TargetWorldId: targetWorldID, DurationTicks: 50, Profile: "main",
	})

	// Poll up to 2s for the closure to land + drain.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.drainRelayActions()
		if s.shutdownTick == wantTick {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if s.shutdownTick != wantTick {
		t.Fatalf("shutdownTick after RelayShutdown: got %d, want %d", s.shutdownTick, wantTick)
	}

	// Issue RelayReload; assert a config-only reload(false) is invoked
	// (gap-world-reload-events-1 — TS World.ts:2036). Stub reloadFn to
	// record the call without touching the (absent) test cache.
	reloadCh := make(chan bool, 1)
	s.reloadFn = func(clearInvs bool) error {
		select {
		case reloadCh <- clearInvs:
		default:
		}
		return nil
	}
	client.RelayReload(context.Background(), &friendspb.RelayReloadRequest{TargetWorldId: targetWorldID, Profile: "main"})
	deadline = time.Now().Add(2 * time.Second)
	var gotClearInvs bool
	delivered := false
	for time.Now().Before(deadline) && !delivered {
		s.drainRelayActions()
		select {
		case gotClearInvs = <-reloadCh:
			delivered = true
		case <-time.After(20 * time.Millisecond):
		}
	}
	if !delivered {
		t.Fatal("reloadFn not invoked after RelayReload + drain (NAI-S5B routing broken)")
	}
	if gotClearInvs {
		t.Fatal("RELAY_RELOAD must reload(false) (config-only), not clobber inventories")
	}
}

// TestLoginClient_E2E_PlayerSessionIsUUID pins slice 7 end-to-end:
// driving a PlayerLogin RPC against a real in-process modules/login
// server returns a SessionUuid that (a) is a valid UUID v4 and (b)
// matches the row stored in the login server's session table.
//
// This is the persistence-and-propagation gate for the slice: it
// proves login UUID → response → world client end-to-end on real
// DB + real proto + real gRPC.
func TestLoginClient_E2E_PlayerSessionIsUUID(t *testing.T) {
	port := freePort(t)
	dbPath := filepath.Join(t.TempDir(), "login.db")
	savePath := t.TempDir()

	cfg := login.Config{
		GRPCListenAddress:       "127.0.0.1",
		GRPCListenPort:          port,
		SQLiteDSN:               dbPath,
		SavePath:                savePath,
		AutoRegister:            true,
		AutoSubscribeMembers:    true,
		BCryptCost:              4,
		Enable:                  true,
		GracefulShutdownTimeout: 5 * time.Second,
	}
	log := discardLogger()
	svc, err := login.New(cfg, log)
	if err != nil {
		t.Fatalf("login.New: %v", err)
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
	loginClient, err := NewLoginClient(addr, log)
	if err != nil {
		t.Fatalf("NewLoginClient: %v", err)
	}
	t.Cleanup(func() { _ = loginClient.Close() })

	resp, err := loginClient.PlayerLogin(ctx, &loginpb.PlayerLoginRequest{
		NodeId:        1,
		Profile:       "main",
		NodeMembers:   true,
		Username:      "sliceseven",
		Password:      "pw",
		Uid:           42,
		RemoteAddress: "127.0.0.1:12345",
	})
	if err != nil {
		t.Fatalf("PlayerLogin: %v", err)
	}

	if resp.SessionUuid == "" {
		t.Fatal("SessionUuid: got empty, want a UUID v4")
	}
	u, err := uuid.Parse(resp.SessionUuid)
	if err != nil {
		t.Fatalf("uuid.Parse(%q): %v", resp.SessionUuid, err)
	}
	if u.Version() != 4 {
		t.Errorf("uuid version: got %d, want 4", u.Version())
	}

	// Cross-check against the session-table row.
	rdb, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })

	var stored string
	if err := rdb.QueryRowContext(t.Context(),
		`SELECT session_uuid FROM session WHERE account_id = ?`,
		resp.AccountId,
	).Scan(&stored); err != nil {
		t.Fatalf("SELECT session_uuid: %v", err)
	}
	if stored != resp.SessionUuid {
		t.Errorf("session table session_uuid = %q, response SessionUuid = %q; want equal", stored, resp.SessionUuid)
	}
}

// TestFriendsClient_E2E_PublicMessagePersistsRow pins the public_chat
// follow-up end-to-end: a client.PublicMessage call against a real
// in-process friends.Friends produces a row in public_chat under
// r.profile, queryable via a second *sql.DB open against the same
// on-disk file. Mirrors slice 6's TestFriendsClient_E2E_
// PrivateMessagePersistsRow.
func TestFriendsClient_E2E_PublicMessagePersistsRow(t *testing.T) {
	port := freePort(t)
	dbPath := filepath.Join(t.TempDir(), "friends.db")
	cfg := friends.Config{
		Profile:                 "main",
		GRPCListenAddress:       "127.0.0.1",
		GRPCListenPort:          port,
		WorldPlayerLimit:        100,
		Enable:                  true,
		GracefulShutdownTimeout: 5 * time.Second,
		SQLiteDSN:               dbPath,
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

	// rev-254 A3 re-key: keyed by the per-login session UUID
	// (TS World.ts:1567-1574 @2e3bcf43 logPublicChat posts session_uuid).
	client.PublicMessage(ctx, &friendspb.PublicMessageRequest{
		WorldId:     10,
		Profile:     "main",
		SessionUuid: "sess-alice",
		Coord:       42,
		Chat:        "persisted publicly",
	})

	// Open a second *sql.DB against the same file. Poll up to 2s for
	// the row — synchronous RPC completion should mean the row is
	// already committed, but WAL settling on a fresh file under -race
	// can take a few ms.
	rdb, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })

	deadline := time.Now().Add(2 * time.Second)
	var sessionUUID, msg string
	var coord int32
	for time.Now().Before(deadline) {
		err := rdb.QueryRowContext(t.Context(),
			`SELECT session_uuid, coord, message
			 FROM public_chat
			 WHERE profile = 'main'
			 ORDER BY id DESC
			 LIMIT 1`).Scan(&sessionUUID, &coord, &msg)
		if err == nil {
			break
		}
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("query public_chat: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if sessionUUID != "sess-alice" || coord != 42 || msg != "persisted publicly" {
		t.Errorf("public_chat row = (%q, %d, %q), want (sess-alice, 42, %q)",
			sessionUUID, coord, msg, "persisted publicly")
	}
}

// TestFriendsClient_E2E_OnPrivateMessageEmitsWirePacket pins NAI-182-D5
// end-to-end: real friends-server PM RPC → real gRPC SubscribeUpdates
// stream → emitFriendsDispatcher.OnPrivateMessage → enqueueRelayAction →
// drainRelayActions (tick goroutine) → sendMessagePrivate → ISAAC-encrypted
// OpMessagePrivate byte on the recipient's net.Conn.
//
// Companion to TestFriendsClient_E2E_PrivateMessageDelivery (which used
// recordingFriendsDispatcher to assert routing). This one uses the
// production emitFriendsDispatcher to close the loop on wire emit.
//
// Design note: the SubscribeUpdates stream sends an initial snapshot
// (empty friendlist → FRIENDLIST_LOADED(2) = 2 bytes, then one empty
// UPDATE_IGNORELIST = 3 bytes) before delivering the PM. We accumulate
// ALL bytes from the wire and check the PM opcode at its correct
// position (byte 5, after the snapshot packets) to avoid conflating
// the packets.
func TestFriendsClient_E2E_OnPrivateMessageEmitsWirePacket(t *testing.T) {
	port := freePort(t)
	cfg := friends.Config{
		Profile:                 "main",
		GRPCListenAddress:       "127.0.0.1",
		GRPCListenPort:          port,
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

	// Seed a recipient *Player + *Server with the production
	// emitFriendsDispatcher wired in.
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	const recipient uint64 = 2222
	p.username37 = recipient
	p.active = true
	p.slot = 1
	p.client.server = s // required: sendMessagePrivate calls server.wordenc.Filter
	s.players.set(1, p)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	disp := newEmitFriendsDispatcher(s, log)

	// Recipient logs in and subscribes via the real gRPC stream.
	client.PlayerLogin(ctx, &friendspb.PlayerLoginRequest{
		WorldId: 10, Profile: "main", Username37: recipient, PrivateChat: 0, StaffLvl: 0,
	}, nil)
	sub := newFriendsSubscriber(client, 10, "main", recipient, disp, log)
	subCtx, subCancel := context.WithCancel(ctx)
	t.Cleanup(subCancel)
	go sub.run(subCtx)

	// Sender logs in.
	client.PlayerLogin(ctx, &friendspb.PlayerLoginRequest{
		WorldId: 10, Profile: "main", Username37: 1111, PrivateChat: 0, StaffLvl: 0,
	}, nil)

	// accumulated collects all wire bytes across multiple reads. The
	// initial subscription snapshot sends FRIENDLIST_LOADED(2) (2 bytes,
	// empty friendlist batch completion) + one empty UPDATE_IGNORELIST
	// (opcode + 2-byte length = 3 bytes) before the PM arrives. We keep
	// reading until we have enough bytes for the PM opcode at offset 5.
	accumulated := make(chan []byte, 32)
	go func() {
		buf := make([]byte, 4096)
		for {
			cc.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
			n, err := cc.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				accumulated <- chunk
			}
			if err != nil {
				return
			}
		}
	}()

	// Sender PMs recipient.
	client.PrivateMessage(ctx, &friendspb.PrivateMessageRequest{
		WorldId:          10,
		Profile:          "main",
		Username37:       1111,
		TargetUsername37: recipient,
		StaffLvl:         0,
		PmId:             0xCAFEBABE,
		Chat:             "e2e",
		Coord:            0,
	})

	// Poll: dispatcher enqueues on s.relayActionQueue; we must drain
	// from the tick-goroutine seat (the test goroutine here) before
	// writeOut runs. Accumulate all wire bytes until we have the full
	// initial snapshot (5 bytes) and the PM opcode byte (byte 5).
	//
	// Wire layout (friendlist snapshot first, then ignorelist —
	// handler.go SubscribeUpdates; the empty friendlist snapshot still
	// emits the batch-completion FRIENDLIST_LOADED(2), TS World.ts:2008
	// @43e02957):
	//   got[0] = encrypted OpFriendlistLoaded opcode (ISAAC advance #1)
	//   got[1] = status 2 (online)
	//   got[2] = encrypted OpUpdateIgnoreList opcode (ISAAC advance #2)
	//   got[3:5] = 2-byte length = 0 (empty ignorelist)
	//   got[5] = encrypted OpMessagePrivate opcode (ISAAC advance #3)
	const pmOpcodeOffset = 5 // byte index of PM opcode in accumulated bytes
	deadline := time.Now().Add(2 * time.Second)
	var got []byte
	for time.Now().Before(deadline) {
		s.drainRelayActions()
		p.client.flushWrite()
		// Drain all pending chunks from the reader goroutine.
	drainLoop:
		for {
			select {
			case chunk := <-accumulated:
				got = append(got, chunk...)
			default:
				break drainLoop
			}
		}
		if len(got) > pmOpcodeOffset {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(got) <= pmOpcodeOffset {
		t.Fatalf("recipient wire received only %d bytes within 2s (need > %d for PM opcode)", len(got), pmOpcodeOffset)
	}

	// Advance enc past ISAAC advances #1 and #2 (the FRIENDLIST_LOADED
	// and UPDATE_IGNORELIST opcodes).
	enc.GetNext()
	enc.GetNext()
	// ISAAC advance #3 is the PM opcode.
	wantOpcode := byte((int(gameserver.OpMessagePrivate.Opcode) + int(enc.GetNext())) & 0xff)
	if got[pmOpcodeOffset] != wantOpcode {
		t.Errorf("PM wire opcode byte (offset %d): got 0x%02x, want 0x%02x (encrypted OpMessagePrivate)",
			pmOpcodeOffset, got[pmOpcodeOffset], wantOpcode)
	}
}

// TestFriendsClient_E2E_RelayQueueScriptAppliesAction pins the full
// round-trip closed by the runtime-fixups-cluster slice: friends server
// emits RELAY_QUEUESCRIPT → world's per-world subscriber → action
// dispatcher → ops.QueueScript → looked-up player's p.queue receives
// a QueueNormal entry referencing the registered [queue,<name>] script.
//
// Mirrors TestFriendsClient_E2E_RelayShutdownAppliesAction's shape.
func TestFriendsClient_E2E_RelayQueueScriptAppliesAction(t *testing.T) {
	port := freePort(t)
	cfg := friends.Config{
		Profile:                 "main",
		GRPCListenAddress:       "127.0.0.1",
		GRPCListenPort:          port,
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

	s := newTestServer(t)
	s.scriptProvider = script.NewProvider()
	sf := &script.ScriptFile{Name: "[queue,e2e_dispatch]", LookupKey: 0xE2E5C}
	s.scriptProvider.Register(sf)

	p := registerActivePlayer(t, s, "alice", 1)
	u37 := jstring.ToBase37("alice")

	inner := newSlogWorldEventsDispatcher(log)
	dispatcher := newActionWorldEventsDispatcher(inner, s)
	const targetWorldID = 7
	sub := newWorldEventsSubscriber(client, targetWorldID, "main", dispatcher, log)
	subCtx, subCancel := context.WithCancel(context.Background())
	subDone := make(chan struct{})
	go func() { sub.run(subCtx); close(subDone) }()
	defer func() {
		subCancel()
		<-subDone
	}()

	// Allow the SubscribeWorldEvents stream to register on the friends
	// server (same wait as TestFriendsClient_E2E_RelayShutdownAppliesAction).
	time.Sleep(300 * time.Millisecond)

	// Issue RelayQueueScript and poll for the closure to land.
	client.RelayQueueScript(context.Background(), &friendspb.RelayQueueScriptRequest{
		TargetWorldId: targetWorldID,
		Profile:       "main",
		ScriptName:    "e2e_dispatch",
		Username37:    u37,
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.drainRelayActions()
		if len(p.queue) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if len(p.queue) != 1 {
		t.Fatalf("p.queue len after RelayQueueScript: got %d, want 1", len(p.queue))
	}
	if got := p.queue[0].Script; got != sf {
		t.Errorf("p.queue[0].Script: got %v, want sf", got)
	}
	if got := p.queue[0].Type; got != script.QueueNormal {
		t.Errorf("p.queue[0].Type: got %v, want QueueNormal", got)
	}
}
