package world

import (
	"log/slog"
	"testing"

	"google.golang.org/grpc/test/bufconn"
)

func TestListenAdoptsInjectedListener(t *testing.T) {
	lis := bufconn.Listen(64 * 1024)
	s := &Server{
		cfg: Config{Listener: lis},
		log: slog.New(slog.DiscardHandler),
	}

	if err := s.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	if s.tcpListener != lis {
		t.Fatalf("tcpListener = %v, want the injected listener", s.tcpListener)
	}
}

func TestValidateSkipsPortCheckWhenListenerInjected(t *testing.T) {
	cfg := Config{
		Enable:    true,
		Listener:  bufconn.Listen(64 * 1024),
		CachePath: "data/pack",
		// TCPListenPort deliberately 0.
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}
