package friends

import (
	"flag"
	"fmt"
	"time"

	"github.com/zsrv/goscape/pkg/util/log"
)

// Config holds the friends-server module's runtime configuration.
type Config struct {
	LogLevel                *log.Level    `yaml:"log_level"` // optional per-module override; nil = inherit global
	GRPCListenAddress       string        `yaml:"grpc_listen_address"`
	NodeProfile             string        `yaml:"node_profile"`
	GRPCListenPort          int           `yaml:"grpc_listen_port"`
	WorldPlayerLimit        int           `yaml:"world_player_limit"`
	Enable                  bool          `yaml:"enable"`
	GracefulShutdownTimeout time.Duration `yaml:"graceful_shutdown_timeout"`
}

func (c *Config) RegisterFlagsAndApplyDefaults(f *flag.FlagSet) {
	f.StringVar(&c.GRPCListenAddress, "friends.grpc-listen-address", "127.0.0.1", "Friends server gRPC listen address.")
	f.IntVar(&c.GRPCListenPort, "friends.grpc-listen-port", 2005, "Friends server gRPC listen port.")
	f.StringVar(&c.NodeProfile, "friends.node-profile", "main", "Profile name validated at WorldConnect.")
	f.IntVar(&c.WorldPlayerLimit, "friends.world-player-limit", 2000, "Per-world player slot cap.")
	f.BoolVar(&c.Enable, "friends.enable", false, "Whether to run the friends module.")
	f.DurationVar(&c.GracefulShutdownTimeout, "friends.graceful-shutdown-timeout", defaultGracefulStopBound, "Bounds how long GracefulStop waits for open streams to close (after Friends.running's closeAll) before shutdown forces a hard Stop.")
}

// Validate enforces runtime invariants (docs/PORTING.md Arc 18 CFG-1 mirror).
// When the module is disabled it short-circuits.
func (c *Config) Validate() error {
	if !c.Enable {
		return nil
	}
	if c.GRPCListenPort < 1 || c.GRPCListenPort > 65535 {
		return fmt.Errorf("friends: GRPCListenPort must be in [1, 65535], got %d", c.GRPCListenPort)
	}
	if c.WorldPlayerLimit < 1 {
		return fmt.Errorf("friends: WorldPlayerLimit must be >= 1, got %d", c.WorldPlayerLimit)
	}
	// newGRPCServer coerces a non-positive grace to defaultGracefulStopBound
	// rather than wiring shutdown to time.After(0). Reject it here so an
	// operator who writes graceful_shutdown_timeout: 0s gets a clear error
	// instead of a silent 5s fallback (an omitted key keeps the default).
	if c.GracefulShutdownTimeout <= 0 {
		return fmt.Errorf("friends: GracefulShutdownTimeout must be > 0, got %s", c.GracefulShutdownTimeout)
	}
	return nil
}
