package friends

import (
	"flag"
	"fmt"
	"time"
)

// Config holds the friends-server module's runtime configuration.
type Config struct {
	GRPCListenAddress       string        `yaml:"grpc_listen_address"`
	SQLiteDSN               string        `yaml:"sqlite_dsn"`
	Profile                 string        `yaml:"profile"`
	GRPCListenPort          int           `yaml:"grpc_listen_port"`
	WorldPlayerLimit        int           `yaml:"world_player_limit"`
	Enable                  bool          `yaml:"enable"`
	GracefulShutdownTimeout time.Duration `yaml:"graceful_shutdown_timeout"`
}

func (c *Config) RegisterFlagsAndApplyDefaults(f *flag.FlagSet) {
	f.StringVar(&c.GRPCListenAddress, "friends.grpc-listen-address", "127.0.0.1", "Friends server gRPC listen address.")
	f.IntVar(&c.GRPCListenPort, "friends.grpc-listen-port", 2005, "Friends server gRPC listen port.")
	f.StringVar(&c.SQLiteDSN, "friends.sqlite-dsn", "data/friends.db", "Friends server SQLite DSN.")
	f.IntVar(&c.WorldPlayerLimit, "friends.world-player-limit", 2000, "Per-world player slot cap.")
	f.StringVar(&c.Profile, "friends.profile", "main", "The single profile this friends server serves (TS FriendServer @2e3bcf43 binds to Environment.NODE_PROFILE, default 'main'; mismatched WORLD_CONNECTs are rejected).")
	f.BoolVar(&c.Enable, "friends.enable", false, "Whether to run the friends module.")
	f.DurationVar(&c.GracefulShutdownTimeout, "friends.graceful-shutdown-timeout", 30*time.Second, "Timeout for graceful gRPC server shutdown.")
}

// Validate enforces runtime invariants (PORTING.md Arc 18 CFG-1 mirror).
// When the module is disabled it short-circuits.
func (c *Config) Validate() error {
	if !c.Enable {
		return nil
	}
	if c.GRPCListenPort < 1 || c.GRPCListenPort > 65535 {
		return fmt.Errorf("friends: GRPCListenPort must be in [1, 65535], got %d", c.GRPCListenPort)
	}
	if c.SQLiteDSN == "" {
		return fmt.Errorf("friends: SQLiteDSN must be non-empty when friends.enable=true")
	}
	if c.WorldPlayerLimit < 1 {
		return fmt.Errorf("friends: WorldPlayerLimit must be >= 1, got %d", c.WorldPlayerLimit)
	}
	if c.Profile == "" {
		return fmt.Errorf("friends: Profile must be non-empty when friends.enable=true")
	}
	return nil
}
