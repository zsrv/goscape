package world

import (
	"flag"
	"log/slog"
	"time"

	"github.com/zsrv/goscape/internal/dskit/server"
)

type Config struct {
	SignalHandler                    SignalHandler `yaml:"-"`
	LogLevel                         *slog.Level   `yaml:"log_level"`
	LogFormat                        string        `yaml:"log_format"`
	NodeDebugprocChar                string        `yaml:"node_debugproc_char"`
	TCPListenNetwork                 string        `yaml:"tcp_listen_network"`
	TCPListenAddress                 string        `yaml:"tcp_listen_address"`
	NodeProfile                      string        `yaml:"node_profile"`
	CachePath                        string        `yaml:"cache_path"`
	LoginServerAddress               string        `yaml:"login_server_address"`
	TCPServerIdleTimeout             time.Duration `yaml:"tcp_server_idle_timeout"`
	NodeMaxNPCs                      int           `yaml:"node_max_npcs"`
	TCPServerWriteTimeout            time.Duration `yaml:"tcp_server_write_timeout"`
	TCPKeepAlivePeriod               time.Duration `yaml:"tcp_keepalive_period"`
	NodeWalktriggerSetting           int           `yaml:"node_walktrigger_setting"`
	TCPServerReadTimeout             time.Duration `yaml:"tcp_server_read_timeout"`
	ServerGracefulShutdownTimeout    time.Duration `yaml:"graceful_shutdown_timeout"`
	NodeID                           int           `yaml:"node_id"`
	TCPServerReadHeaderTimeout       time.Duration `yaml:"tcp_server_read_header_timeout"`
	NodeMaxConnected                 int           `yaml:"node_max_connected"`
	NodeXPRate                       int           `yaml:"node_xp_rate"`
	NodeMaxPlayers                   int           `yaml:"node_max_players"`
	TCPListenPort                    int           `yaml:"tcp_listen_port"`
	NodeLimitBytesPerTrackingSession int           `yaml:"node_limit_bytes_per_tracking_session"`
	NodeMinimumWealthValueEvent      int           `yaml:"node_minimum_wealth_value_event"`
	NodeMembers                      bool          `yaml:"node_members"`
	LoginServerEnabled               bool          `yaml:"login_server_enabled"`
	NodeDebugProfile                 bool          `yaml:"node_debug_profile"`
	NodeDebugSocket                  bool          `yaml:"node_debug_socket"`
	NodeClientRoutefinder            bool          `yaml:"node_client_routefinder"`
	NodeDebug                        bool          `yaml:"node_debug"`
	NodeSubmitInput                  bool          `yaml:"node_submit_input"`
	NodeProduction                   bool          `yaml:"node_production"`
	NodeAutoSubscribeMembers         bool          `yaml:"node_auto_subscribe_members"`
	Enable                           bool          `yaml:"enable"`
	EnableTCPServer                  bool          `yaml:"enable_tcp_server"`
}

// RegisterFlagsAndApplyDefaults registers flags and applies defaults.
func (c *Config) RegisterFlagsAndApplyDefaults(f *flag.FlagSet) {
	f.StringVar(&c.TCPListenAddress, "world.tcp-listen-address", "127.0.0.1", "TCP world server listen address")
	f.StringVar(&c.TCPListenNetwork, "world.tcp-listen-network", server.DefaultNetwork, "TCP world server listen network, default tcp")
	//f.StringVar(&c.Config.HTTPTLSConfig.TLSCertPath, "asset.http-tls-cert-path", "", "HTTP asset server cert path.")
	//f.StringVar(&c.Config.HTTPTLSConfig.TLSKeyPath, "asset.http-tls-key-path", "", "HTTP asset server key path.")
	//f.StringVar(&c.Config.HTTPTLSConfig.ClientAuth, "asset.http-tls-client-auth", "", "HTTP TLS Client Auth type.")
	//f.StringVar(&c.Config.HTTPTLSConfig.ClientCAs, "asset.http-tls-ca-path", "", "HTTP TLS Client CA path.")
	//f.StringVar(&c.Config.CipherSuites, "asset.http-tls-cipher-suites", "", "HTTP TLS Cipher Suites.")
	//f.StringVar(&c.Config.MinVersion, "asset.http-tls-min-version", "", "HTTP TLS Min Version.")
	f.IntVar(&c.TCPListenPort, "world.tcp-listen-port", 43594, "TCP world server listen port")
	//f.IntVar(&c.Config.HTTPConnLimit, "asset.http-conn-limit", 0, "Maximum number of simultaneous http connections, <=0 to disable")
	f.DurationVar(&c.ServerGracefulShutdownTimeout, "world.graceful-shutdown-timeout", 30*time.Second, "Timeout for graceful shutdowns")
	f.DurationVar(&c.TCPServerReadTimeout, "world.tcp-read-timeout", 5*time.Second, "Read timeout for TCP server")
	f.DurationVar(&c.TCPServerWriteTimeout, "world.tcp-write-timeout", 30*time.Second, "Write timeout for TCP server")
	f.DurationVar(&c.TCPServerIdleTimeout, "world.tcp-idle-timeout", 120*time.Second, "Idle timeout for TCP server")
	f.DurationVar(&c.TCPKeepAlivePeriod, "world.tcp-keepalive-period", 30*time.Second,
		"TCP keepalive idle period before first probe; set to 0 to disable")

	f.IntVar(&c.NodeID, "world.node-id", 10, "World ID, offset by 9")
	f.BoolVar(&c.NodeMembers, "world.node-members", true, "Whether members content is available on this world")
	f.BoolVar(&c.NodeAutoSubscribeMembers, "world.node-auto-subscribe-members", true, "Whether to automatically upgrade accounts that log into this world to members")
	f.IntVar(&c.NodeXPRate, "world.node-xp-rate", 1, "addxp multiplier")
	f.BoolVar(&c.NodeProduction, "world.node-production", false, "Whether to run in production mode")
	f.BoolVar(&c.NodeSubmitInput, "world.node-submit-input", false, "Whether clients should be instructed to submit detailed tracking events to the server")
	f.IntVar(&c.NodeLimitBytesPerTrackingSession, "world.node-limit-bytes-per-tracking-session", 50_000, "Maximum approximate number of bytes allowed per single input tracking session")
	f.IntVar(&c.NodeMinimumWealthValueEvent, "world.node-minimum-wealth-value-event", 10, "")
	f.BoolVar(&c.NodeDebug, "world.node-debug", true, "Extra debug info, e.g. missing triggers")
	f.BoolVar(&c.NodeDebugProfile, "world.node-debug-profile", false, "")
	f.BoolVar(&c.NodeDebugSocket, "world.node-debug-socket", false, "")
	f.BoolVar(&c.NodeClientRoutefinder, "world.node-client-route-finder", true, "")
	f.IntVar(&c.NodeWalktriggerSetting, "world.node-walk-trigger-setting", 0, "") // TODO: replace default with enum
	f.StringVar(&c.NodeProfile, "world.node-profile", "main", "")
	f.StringVar(&c.CachePath, "world.cache-path", "./data/pack/client", "Cache root; gamemap loads map-pack files from <path>/maps/")
	f.IntVar(&c.NodeMaxPlayers, "world.node-max-players", 2047, "")
	f.IntVar(&c.NodeMaxConnected, "world.node-max-connected", 1000, "")
	f.IntVar(&c.NodeMaxNPCs, "world.node-max-npcs", 8191, "")
	f.StringVar(&c.NodeDebugprocChar, "world.node-debugproc-char", "~", "")

	f.StringVar(&c.LoginServerAddress, "world.login-server-address", "127.0.0.1:2004", "Login server gRPC address.")
	f.BoolVar(&c.LoginServerEnabled, "world.login-server-enabled", true, "Whether to connect to the login server.")
}

func (c *Config) Validate() error {
	return nil
}
