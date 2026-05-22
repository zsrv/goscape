package login

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/zsrv/goscape/pkg/loginpb"
)

type grpcServer struct {
	server *grpc.Server
	log    *slog.Logger
}

func newGRPCServer(cfg Config, db *sql.DB, log *slog.Logger) *grpcServer {
	s := grpc.NewServer()
	loginpb.RegisterLoginServiceServer(s, &handler{
		db:  db,
		cfg: cfg,
		log: log,
	})
	reflection.Register(s)
	return &grpcServer{server: s, log: log}
}

// listen binds the TCP port and returns the listener. Called during Starting
// phase so the service is not considered running until the port is bound.
func (s *grpcServer) listen(cfg Config) (net.Listener, error) {
	addr := fmt.Sprintf("%s:%d", cfg.GRPCListenAddress, cfg.GRPCListenPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("grpc listen %s: %w", addr, err)
	}
	s.log.Info("login gRPC server listening", slog.String("addr", addr))
	return lis, nil
}

// serve starts accepting connections on lis. It blocks until the server stops.
func (s *grpcServer) serve(lis net.Listener) error {
	return s.server.Serve(lis)
}

// shutdown triggers a graceful stop of the gRPC server.
func (s *grpcServer) shutdown() {
	s.server.GracefulStop()
}
