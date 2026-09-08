package login

import (
	"log/slog"
	"testing"

	"google.golang.org/grpc/test/bufconn"
)

func TestListenReturnsInjectedListener(t *testing.T) {
	lis := bufconn.Listen(64 * 1024)
	s := &grpcServer{log: slog.New(slog.DiscardHandler)}

	got, err := s.listen(Config{Listener: lis})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if got != lis {
		t.Fatalf("listen returned %v, want the injected listener", got)
	}
}

func TestValidateSkipsPortCheckWhenListenerInjected(t *testing.T) {
	cfg := Config{
		Enable:     true,
		Listener:   bufconn.Listen(64 * 1024),
		BCryptCost: 10,
		SavePath:   "data/players",
		AuthMode:   AuthModeLocal,
		// GRPCListenPort deliberately 0 — meaningless with a listener.
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}
