package account

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/zsrv/goscape/pkg/accountpb"
)

// stubHealthServer answers SERVING for everything; the app root registers the
// real grpcutil.HealthCheck here.
type stubHealthServer struct {
	grpc_health_v1.UnimplementedHealthServer
}

func (stubHealthServer) Check(context.Context, *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
}

// TestHealthServiceBypassesAdminAuth pins the interceptor exemption: a kubelet
// probe or a service mesh cannot present account.admin_token, so the gRPC
// health service must answer without any metadata — while an admin RPC with
// no token is still rejected on the very same connection.
func TestHealthServiceBypassesAdminAuth(t *testing.T) {
	s := openTestStore(t)
	cfg := defaultConfig(t)
	cfg.AdminToken = "sekrit"
	cfg.PublicURL = "http://portal.test"

	lis := bufconn.Listen(1 << 20)
	srv := newGRPCServer(cfg, s, testLogger(t))
	grpc_health_v1.RegisterHealthServer(srv, stubHealthServer{})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	resp, err := grpc_health_v1.NewHealthClient(conn).Check(t.Context(), &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("health Check without a token: %v", err)
	}
	if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("health status = %v, want SERVING", resp.Status)
	}

	_, err = accountpb.NewAccountServiceClient(conn).SearchAccounts(t.Context(),
		&accountpb.SearchAccountsRequest{Query: "anything"})
	if code := status.Code(err); code != codes.Unauthenticated {
		t.Fatalf("admin RPC without a token = %v (code %v), want Unauthenticated", err, code)
	}
}
