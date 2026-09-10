package world

import (
	"context"
	"log/slog"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/zsrv/goscape/pkg/friendspb"
	"github.com/zsrv/goscape/pkg/loginpb"
)

// A login client built with an injected dialer must reach a bufconn-served
// gRPC server with no TCP anywhere in the path.
func TestNewLoginClientUsesInjectedDialer(t *testing.T) {
	lis := bufconn.Listen(64 * 1024)
	srv := grpc.NewServer()
	loginpb.RegisterLoginServiceServer(srv, &stubLoginServer{})
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	dial := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}

	c, err := NewLoginClient("passthrough:///login", slog.New(slog.DiscardHandler),
		grpc.WithContextDialer(dial))
	if err != nil {
		t.Fatalf("NewLoginClient: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.WorldStartup(ctx, 10, "main"); err != nil {
		t.Fatalf("WorldStartup over bufconn: %v", err)
	}
}

type stubLoginServer struct {
	loginpb.UnimplementedLoginServiceServer
}

// WorldStartup's response type is *emptypb.Empty (see login.proto), not a
// WorldStartupResponse — that type does not exist in pkg/loginpb. The brief
// for this test named a WorldStartupResponse type; this uses the actual
// generated signature so the stub compiles and overrides the embedded
// UnimplementedLoginServiceServer method instead of silently falling back
// to it.
func (s *stubLoginServer) WorldStartup(context.Context, *loginpb.WorldStartupRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

// The friends bridge gets the same treatment as login. Worth its own test
// rather than trusting symmetry: nothing structurally ties the two
// constructors together, so a change that stopped NewFriendsClient from
// forwarding its opts — dropping `opts...` from the append — would leave the
// login test green while singleplayer's friends bridge went back to dialling
// TCP. Confirmed by doing exactly that: WorldConnect then fails the connect
// handshake against the bufconn listener.
//
// The append order itself is NOT what this pins. WithContextDialer and
// WithTransportCredentials set different fields, so they never conflict and
// reversing the two halves of the append changes nothing.
func TestNewFriendsClientUsesInjectedDialer(t *testing.T) {
	lis := bufconn.Listen(64 * 1024)
	srv := grpc.NewServer()
	friendspb.RegisterFriendsServiceServer(srv, &stubFriendsServer{})
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	dial := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}

	c, err := NewFriendsClient("passthrough:///friends", slog.New(slog.DiscardHandler),
		grpc.WithContextDialer(dial))
	if err != nil {
		t.Fatalf("NewFriendsClient: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	// WorldConnect is the one unary friends RPC that returns its error
	// (arch-29.3), so it reports transport failure directly instead of only
	// logging it the way the fire-and-forget calls do.
	if err := c.WorldConnect(ctx, 10, "main"); err != nil {
		t.Fatalf("WorldConnect over bufconn: %v", err)
	}
}

type stubFriendsServer struct {
	friendspb.UnimplementedFriendsServiceServer
}

func (s *stubFriendsServer) WorldConnect(context.Context, *friendspb.WorldConnectRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
