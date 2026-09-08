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
