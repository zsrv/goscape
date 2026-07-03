package friends

import (
	"fmt"
	"log/slog"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"

	"github.com/zsrv/goscape/pkg/friendspb"
)

// defaultGracefulStopBound caps how long shutdown waits for GracefulStop
// before forcing a hard Stop (arch-29.4). Friends.running closes every
// registered subscriber's done channel (via subscriptions.closeAll /
// worldSubscriptions.closeAll) before calling shutdown, so in the normal
// case GracefulStop finishes almost immediately. This bound exists only
// for stragglers the registries could not reach — e.g. a Subscribe that
// lands in the narrow race between closeAll running and GracefulStop
// actually closing the listener. Stop() cuts that straggler's connection,
// which drives its handler's stream ctx.Done() branch to return.
//
// arch-29.5: also the default value of the friends.graceful-shutdown-timeout
// flag (Config.GracefulShutdownTimeout) and the fallback newGRPCServer uses
// when a caller constructs a Config literal without setting that field
// (e.g. tests), so the effective grace window is always this bound unless
// an operator overrides it.
const defaultGracefulStopBound = 5 * time.Second

// grpcServer wraps a *grpc.Server registered with the friends handler.
// Sibling to modules/login/server.go.
type grpcServer struct {
	server *grpc.Server
	log    *slog.Logger
	// grace bounds shutdown's wait on GracefulStop. Derived from
	// Config.GracefulShutdownTimeout in newGRPCServer (falling back to
	// defaultGracefulStopBound when unset); tests may override it directly
	// to keep the forced-Stop path fast.
	grace time.Duration
}

func newGRPCServer(cfg Config, repo *Repository, subs *subscriptions, worldSubs *worldSubscriptions, log *slog.Logger) *grpcServer {
	// arch-29.2: permit the world's 30s keepalive probes (default
	// EnforcementPolicy MinTime is 5m and would GOAWAY the client).
	s := grpc.NewServer(
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             15 * time.Second,
			PermitWithoutStream: true,
		}),
	)
	friendspb.RegisterFriendsServiceServer(s, &handler{
		repo:      repo,
		subs:      subs,
		worldSubs: worldSubs,
		cfg:       cfg,
		log:       log,
	})
	reflection.Register(s)
	// arch-29.5: Config literals built directly (tests, or any caller that
	// skips RegisterFlagsAndApplyDefaults) leave GracefulShutdownTimeout at
	// its zero value; fall back to defaultGracefulStopBound rather than
	// wiring shutdown() to time.After(0).
	grace := cfg.GracefulShutdownTimeout
	if grace <= 0 {
		grace = defaultGracefulStopBound
	}
	return &grpcServer{server: s, log: log, grace: grace}
}

// listen binds the TCP port and returns the listener. Called during
// Starting phase so the service is not Running until the port is bound.
func (s *grpcServer) listen(cfg Config) (net.Listener, error) {
	addr := fmt.Sprintf("%s:%d", cfg.GRPCListenAddress, cfg.GRPCListenPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("grpc listen %s: %w", addr, err)
	}
	s.log.Info("friends gRPC server listening", slog.String("addr", addr))
	return lis, nil
}

// serve starts accepting connections on lis. Blocks until the server stops.
func (s *grpcServer) serve(lis net.Listener) error {
	return s.server.Serve(lis)
}

// shutdown drains gracefully but never hangs: the registries' closeAll
// (called by Friends.running before shutdown) releases every stream a
// client isn't actively ending, so GracefulStop normally returns almost
// immediately; anything still open once the grace window elapses is cut
// with a hard Stop (arch-29.4).
func (s *grpcServer) shutdown() {
	done := make(chan struct{})
	go func() {
		s.server.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(s.grace):
		s.log.Warn("GracefulStop grace window elapsed; forcing Stop")
		s.server.Stop()
		<-done
	}
}
