package login

import (
	"flag"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/zsrv/goscape/pkg/util/log"
)

type Config struct {
	LogLevel                *log.Level    `yaml:"log_level"` // optional per-module override; nil = inherit global
	GRPCListenAddress       string        `yaml:"grpc_listen_address"`
	SQLiteDSN               string        `yaml:"sqlite_dsn"`
	SavePath                string        `yaml:"save_path"`
	NodeProfile             string        `yaml:"node_profile"`
	GRPCListenPort          int           `yaml:"grpc_listen_port"`
	BCryptCost              int           `yaml:"bcrypt_cost"`
	AutoRegister            bool          `yaml:"auto_register"`
	AutoSubscribeMembers    bool          `yaml:"auto_subscribe_members"`
	Enable                  bool          `yaml:"enable"`
	GracefulShutdownTimeout time.Duration `yaml:"graceful_shutdown_timeout"`
}

func (c *Config) RegisterFlagsAndApplyDefaults(f *flag.FlagSet) {
	f.StringVar(&c.GRPCListenAddress, "login.grpc-listen-address", "127.0.0.1", "Login server gRPC listen address.")
	f.IntVar(&c.GRPCListenPort, "login.grpc-listen-port", 2004, "Login server gRPC listen port.")
	f.StringVar(&c.SQLiteDSN, "login.sqlite-dsn", "data/login.db", "Login server SQLite DSN.")
	f.StringVar(&c.SavePath, "login.save-path", "data/players", "Player save file root directory.")
	f.IntVar(&c.BCryptCost, "login.bcrypt-cost", 10, "bcrypt work factor for password hashing.")
	f.StringVar(&c.NodeProfile, "login.node-profile", "main", "Profile name for DB queries.")
	f.BoolVar(&c.AutoRegister, "login.auto-register", true, "Automatically create accounts on first login.")
	f.BoolVar(&c.AutoSubscribeMembers, "login.auto-subscribe-members", true, "Automatically upgrade non-member accounts to members on member worlds.")
	f.BoolVar(&c.Enable, "login.enable", false, "Whether to run the login module.")
	f.DurationVar(&c.GracefulShutdownTimeout, "login.graceful-shutdown-timeout", 30*time.Second, "Timeout for graceful gRPC server shutdown.")
}

// Validate enforces runtime invariants (docs/PORTING.md Arc 18 CFG-1).
// When the module is disabled it short-circuits — the values are not
// consulted by any live code path.
func (c *Config) Validate() error {
	if !c.Enable {
		return nil
	}
	if c.BCryptCost < bcrypt.MinCost || c.BCryptCost > bcrypt.MaxCost {
		return fmt.Errorf("login: BCryptCost must be in [%d, %d], got %d", bcrypt.MinCost, bcrypt.MaxCost, c.BCryptCost)
	}
	if c.GRPCListenPort < 1 || c.GRPCListenPort > 65535 {
		return fmt.Errorf("login: GRPCListenPort must be in [1, 65535], got %d", c.GRPCListenPort)
	}
	if c.SQLiteDSN == "" {
		return fmt.Errorf("login: SQLiteDSN must be non-empty when login.enable=true")
	}
	if c.SavePath == "" {
		return fmt.Errorf("login: SavePath must be non-empty when login.enable=true")
	}
	return nil
}
