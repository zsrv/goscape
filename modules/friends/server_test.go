package friends

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/zsrv/goscape/pkg/friendspb"
)

// TestGRPCServer_Shutdown_ForcesStopAfterGrace pins the arch-29.4 backstop
// half of the fix: a stream whose done channel was never closed — the
// late-registration race Friends.running documents (a Subscribe that lands
// after closeAll has already run but before GracefulStop stops accepting
// new streams on an existing connection) — must not hold shutdown() open
// forever. Once the grace window elapses, shutdown forces Stop(), which
// cuts the connection so the handler's stream ctx.Done() branch returns
// and GracefulStop's wait unblocks.
//
// This test never calls (*subscriptions).closeAll — it exercises
// grpcServer.shutdown in isolation, standing in for a straggler subscriber
// that closeAll could not have reached.
func TestGRPCServer_Shutdown_ForcesStopAfterGrace(t *testing.T) {
	db := createTestDB(t)
	repo := NewRepository(db, "main")
	log := noopLogger()
	subs := newSubscriptions(log)
	worldSubs := newWorldSubscriptions(log)
	cfg := Config{NodeProfile: "main", WorldPlayerLimit: 100}

	srv := newGRPCServer(cfg, repo, subs, worldSubs, log)
	srv.grace = 100 * time.Millisecond // keep the test fast; production uses defaultGracefulStopBound

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.serve(lis) }()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	client := friendspb.NewFriendsServiceClient(conn)

	stream, err := client.SubscribeUpdates(context.Background(), &friendspb.SubscribeUpdatesRequest{
		WorldId: 1, Username37: 42,
	})
	if err != nil {
		t.Fatalf("SubscribeUpdates: %v", err)
	}

	// Wait for the handler to actually register the subscriber server-side
	// before racing shutdown, so we know GracefulStop has a live RPC to
	// wait on.
	waitFor(t, func() bool {
		subs.mu.Lock()
		defer subs.mu.Unlock()
		return len(subs.by) == 1
	})

	shutdownDone := make(chan struct{})
	start := time.Now()
	go func() {
		srv.shutdown()
		close(shutdownDone)
	}()
	select {
	case <-shutdownDone:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not return within bound; GracefulStop backstop failed")
	}
	elapsed := time.Since(start)
	if elapsed < srv.grace {
		t.Fatalf("shutdown returned in %v, faster than the grace window %v; want forced Stop after grace elapses", elapsed, srv.grace)
	}

	// Drain past the initial friendlist/ignorelist snapshots (already sent
	// before the handler blocked in its select loop) until Recv reports
	// the stream ended.
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
			t.Fatal("expected the stream to end with a non-nil error after forced Stop")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the stream to end after forced Stop")
	}
	<-serveErr
}

// TestGRPCServer_Shutdown_FastWhenIdle pins the happy path: with no
// subscribers registered, shutdown returns quickly and never reaches the
// grace-window fallback.
func TestGRPCServer_Shutdown_FastWhenIdle(t *testing.T) {
	db := createTestDB(t)
	repo := NewRepository(db, "main")
	log := noopLogger()
	subs := newSubscriptions(log)
	worldSubs := newWorldSubscriptions(log)
	cfg := Config{NodeProfile: "main", WorldPlayerLimit: 100}

	srv := newGRPCServer(cfg, repo, subs, worldSubs, log)
	srv.grace = 100 * time.Millisecond

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go srv.serve(lis)

	start := time.Now()
	srv.shutdown()
	if elapsed := time.Since(start); elapsed >= srv.grace {
		t.Fatalf("shutdown took %v with no subscribers; want well under the grace window %v", elapsed, srv.grace)
	}
}
