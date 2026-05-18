package friends

import (
	"flag"
	"time"
)

// Config holds the friends-server module's runtime configuration.
type Config struct {
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
	f.DurationVar(&c.GracefulShutdownTimeout, "friends.graceful-shutdown-timeout", 30*time.Second, "Timeout for graceful gRPC server shutdown.")
}

func (c *Config) Validate() error {
	return nil
}
