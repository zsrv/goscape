package friends

import (
	"context"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/zsrv/goscape/pkg/friendspb"
)

// freeFriendsPort opens an ephemeral listener, captures its port, and
// closes it so Friends.starting can bind the same port. Race window is
// small enough for tests; not safe for production code. Mirrors
// modules/world/friends_smoke_test.go's freePort helper (unexported to
// each package, so duplicated rather than shared across a module
// boundary).
func freeFriendsPort(t *testing.T) int {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freeFriendsPort listen: %v", err)
	}
	port := lis.Addr().(*net.TCPAddr).Port
	if err := lis.Close(); err != nil {
		t.Fatalf("freeFriendsPort close: %v", err)
	}
	return port
}

// TestFriends_Running_StopsWithOpenSubscriberStream reproduces the
// arch-29.4 bug end to end: a standalone `--target friends` node with a
// world's SubscribeUpdates stream still attached must still terminate
// promptly on stop, instead of GracefulStop hanging forever because
// nothing server-side ends a stream the client never closes.
//
// Drives the real dskit Service lifecycle (StartAsync/AwaitRunning/
// StopAsync/AwaitTerminated) against a real listener and a real gRPC
// client — the same shape as
// modules/world/friends_smoke_test.go's TestFriendsClient_E2E — rather
// than a bufconn harness (handler_test.go only has a fake in-process
// ServerStream, not a dialable one). The client deliberately never
// closes its stream, so the only thing that can end it server-side is
// running's closeAll call.
//
// The AwaitTerminated bound is deliberately 2s — meaningfully below
// defaultGracefulStopBound (5s) — so the two halves of the arch-29.4
// fix stay distinguishable: finishing under 2s REQUIRES the closeAll
// wiring in Friends.running (with only the backstop, shutdown takes the
// full 5s grace window — measured 5.02s with the closeAll calls
// reverted — and fails this bound; with the wiring it completes in
// ~20ms). The backstop itself is pinned separately by
// TestGRPCServer_Shutdown_ForcesStopAfterGrace in server_test.go.
func TestFriends_Running_StopsWithOpenSubscriberStream(t *testing.T) {
	port := freeFriendsPort(t)
	cfg := Config{
		GRPCListenAddress: "127.0.0.1",
		GRPCListenPort:    port,
		WorldPlayerLimit:  100,
		Enable:            true,
		SQLiteDSN:         filepath.Join(t.TempDir(), "friends.db"),
	}
	f, err := New(cfg, noopLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	startCtx, cancelStart := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelStart()
	if err := f.StartAsync(startCtx); err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if err := f.AwaitRunning(startCtx); err != nil {
		t.Fatalf("AwaitRunning: %v", err)
	}

	addr := "127.0.0.1:" + strconv.Itoa(port)
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	client := friendspb.NewFriendsServiceClient(conn)

	stream, err := client.SubscribeUpdates(context.Background(), &friendspb.SubscribeUpdatesRequest{
		WorldId: 1, Profile: "main", Username37: 42,
	})
	if err != nil {
		t.Fatalf("SubscribeUpdates: %v", err)
	}
	// Wait for the handler to register the subscriber before stopping, so
	// the test actually exercises a live stream rather than a race where
	// stop wins before the server-side registration happens.
	waitFor(t, func() bool {
		f.subs.mu.Lock()
		defer f.subs.mu.Unlock()
		return len(f.subs.by) == 1
	})

	// The client never calls stream.CloseSend or cancels its context —
	// closeAll (via Friends.running) is the only thing that can end this
	// stream server-side.
	f.StopAsync()

	// 2s: below defaultGracefulStopBound so only the closeAll wiring —
	// not the forced-Stop backstop — can satisfy it (see doc comment).
	termCtx, cancelTerm := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelTerm()
	if err := f.AwaitTerminated(termCtx); err != nil {
		t.Fatalf("AwaitTerminated: %v (stop with an open subscriber stream took >=2s; closeAll wiring in Friends.running regressed — only the GracefulStop backstop is ending the stream)", err)
	}

	// The stream must have ended too (drop past any buffered initial
	// snapshot messages until Recv reports the stream is over).
	recvErr := make(chan error, 1)
	go func() {
		for {
			if _, err := stream.Recv(); err != nil {
				recvErr <- err
				return
			}
		}
	}()
	select {
	case err := <-recvErr:
		if err == nil {
			t.Fatal("expected the stream to end with a non-nil error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the client stream to observe the server stop")
	}
}
