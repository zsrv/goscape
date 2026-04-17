package login

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net"

	"google.golang.org/grpc"

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
	return &grpcServer{server: s, log: log}
}

// run starts the gRPC listener. It blocks until the server stops.
func (s *grpcServer) run(cfg Config) error {
	addr := fmt.Sprintf("%s:%d", cfg.GRPCListenAddress, cfg.GRPCListenPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("grpc listen %s: %w", addr, err)
	}
	s.log.Info("login gRPC server listening", slog.String("addr", addr))
	return s.server.Serve(lis)
}

// shutdown triggers a graceful stop of the gRPC server.
func (s *grpcServer) shutdown() {
	s.server.GracefulStop()
}
