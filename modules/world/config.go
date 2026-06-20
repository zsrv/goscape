package world

import (
	"flag"
	"fmt"
	"log/slog"
	"time"

	"github.com/zsrv/goscape/pkg/dskit/server"
	"github.com/zsrv/goscape/pkg/io/protocol"
)

type Config struct {
	SignalHandler     SignalHandler `yaml:"-"`
	LogLevel          *slog.Level   `yaml:"log_level"`
	LogFormat         string        `yaml:"log_format"`
	NodeDebugprocChar string        `yaml:"node_debugproc_char"`
	TCPListenNetwork  string        `yaml:"tcp_listen_network"`
	TCPListenAddress  string        `yaml:"tcp_listen_address"`
	NodeProfile       string        `yaml:"node_profile"`
	CachePath         string        `yaml:"cache_path"`
	ContentPath       string        `yaml:"content_path"`
	// RSAPrivateKeyPath optionally points to a PEM-encoded RSA private key
	// (PKCS#1 or PKCS#8) used to decrypt the login block, replacing the
	// built-in default key in pkg/io/protocol/rsakey.go. Empty (default) uses
	// the built-in key. Mirrors Engine-TS World.ts:104 (data/config/private.pem).
	// The Java client must be rebuilt with the matching public key
	// (Client.java LOGIN_RSAN / LOGIN_RSAE) or every login fails.
	RSAPrivateKeyPath                string             `yaml:"rsa_private_key_path"`
	LoginServerAddress               string             `yaml:"login_server_address"`
	FriendsServerAddress             string             `yaml:"friends_server_address"`
	TCPServerIdleTimeout             time.Duration      `yaml:"tcp_server_idle_timeout"`
	NodeMaxNPCs                      int                `yaml:"node_max_npcs"`
	TCPServerWriteTimeout            time.Duration      `yaml:"tcp_server_write_timeout"`
	TCPKeepAlivePeriod               time.Duration      `yaml:"tcp_keepalive_period"`
	NodeWalktriggerSetting           WalkTriggerSetting `yaml:"node_walktrigger_setting"`
	TCPServerReadTimeout             time.Duration      `yaml:"tcp_server_read_timeout"`
	ServerGracefulShutdownTimeout    time.Duration      `yaml:"graceful_shutdown_timeout"`
	NodeID                           int                `yaml:"node_id"`
	TCPServerReadHeaderTimeout       time.Duration      `yaml:"tcp_server_read_header_timeout"`
	NodeMaxConnected                 int                `yaml:"node_max_connected"`
	NodeXPRate                       int                `yaml:"node_xp_rate"`
	NodeMaxPlayers                   int                `yaml:"node_max_players"`
	TCPListenPort                    int                `yaml:"tcp_listen_port"`
	NodeLimitBytesPerTrackingSession int                `yaml:"node_limit_bytes_per_tracking_session"`
	NodeMinimumWealthValueEvent      int                `yaml:"node_minimum_wealth_value_event"`
	NodeRatelimitAddressLogin        int                `yaml:"node_ratelimit_address_login"`
	NodeRatelimitDeviceLogin         int                `yaml:"node_ratelimit_device_login"`
	NodeMembers                      bool               `yaml:"node_members"`
	LoginServerEnabled               bool               `yaml:"login_server_enabled"`
	FriendsServerEnabled             bool               `yaml:"friends_server_enabled"`
	NodeDebugProfile                 bool               `yaml:"node_debug_profile"`
	NodeDebugSocket                  bool               `yaml:"node_debug_socket"`
	NodeClientRoutefinder            bool               `yaml:"node_client_routefinder"`
	NodeDebug                        bool               `yaml:"node_debug"`
	NodeSubmitInput                  bool               `yaml:"node_submit_input"`
	NodeProduction                   bool               `yaml:"node_production"`
	NodeAutoSubscribeMembers         bool               `yaml:"node_auto_subscribe_members"`
	ContentWatch                     bool               `yaml:"content_watch"`
	Enable                           bool               `yaml:"enable"`
	EnableTCPServer                  bool               `yaml:"enable_tcp_server"`
}

// RegisterFlagsAndApplyDefaults registers flags and applies defaults.
func (c *Config) RegisterFlagsAndApplyDefaults(f *flag.FlagSet) {
	f.StringVar(&c.TCPListenAddress, "world.tcp-listen-address", "127.0.0.1", "TCP world server listen address")
	f.StringVar(&c.TCPListenNetwork, "world.tcp-listen-network", server.DefaultNetwork, "TCP world server listen network, default tcp")
	//f.StringVar(&c.Config.HTTPTLSConfig.TLSCertPath, "ondemand.http-tls-cert-path", "", "HTTP OnDemand server cert path.")
	//f.StringVar(&c.Config.HTTPTLSConfig.TLSKeyPath, "ondemand.http-tls-key-path", "", "HTTP OnDemand server key path.")
	//f.StringVar(&c.Config.HTTPTLSConfig.ClientAuth, "ondemand.http-tls-client-auth", "", "HTTP TLS Client Auth type.")
	//f.StringVar(&c.Config.HTTPTLSConfig.ClientCAs, "ondemand.http-tls-ca-path", "", "HTTP TLS Client CA path.")
	//f.StringVar(&c.Config.CipherSuites, "ondemand.http-tls-cipher-suites", "", "HTTP TLS Cipher Suites.")
	//f.StringVar(&c.Config.MinVersion, "ondemand.http-tls-min-version", "", "HTTP TLS Min Version.")
	f.IntVar(&c.TCPListenPort, "world.tcp-listen-port", 43594, "TCP world server listen port")
	//f.IntVar(&c.Config.HTTPConnLimit, "ondemand.http-conn-limit", 0, "Maximum number of simultaneous http connections, <=0 to disable")
	f.DurationVar(&c.ServerGracefulShutdownTimeout, "world.graceful-shutdown-timeout", 30*time.Second, "Timeout for graceful shutdowns")
	// logger-transport-3 (2026-05-28 audit): TS TcpServer.ts:19 sets the
	// idle-socket timeout to 30000 ms via `s.setTimeout(30000)`. The pre-fix
	// 5s default disconnected idle clients 6x more aggressively than TS,
	// observable as keep-alive connections being killed between game ticks
	// during low-activity periods (debug-socket mode at server.go:830 already
	// bypasses the deadline; this default is for the production read path).
	f.DurationVar(&c.TCPServerReadTimeout, "world.tcp-read-timeout", 30*time.Second, "Read timeout for TCP server")
	f.DurationVar(&c.TCPServerWriteTimeout, "world.tcp-write-timeout", 30*time.Second, "Write timeout for TCP server")
	f.DurationVar(&c.TCPServerIdleTimeout, "world.tcp-idle-timeout", 120*time.Second, "Idle timeout for TCP server")
	f.DurationVar(&c.TCPKeepAlivePeriod, "world.tcp-keepalive-period", 30*time.Second,
		"TCP keepalive idle period before first probe; set to 0 to disable")

	f.IntVar(&c.NodeID, "world.node-id", 10, "World ID, offset by 9")
	f.BoolVar(&c.NodeMembers, "world.node-members", true, "Whether members content is available on this world")
	f.BoolVar(&c.NodeAutoSubscribeMembers, "world.node-auto-subscribe-members", true, "Whether to automatically upgrade accounts that log into this world to members")
	f.IntVar(&c.NodeXPRate, "world.node-xp-rate", 1, "addxp multiplier")
	f.BoolVar(&c.NodeProduction, "world.node-production", false, "Whether to run in production mode")
	f.BoolVar(&c.NodeSubmitInput, "world.node-submit-input", false, "Unused at 254 (TS Environment still defines NODE_SUBMIT_INPUT; nothing reads it)")
	f.IntVar(&c.NodeLimitBytesPerTrackingSession, "world.node-limit-bytes-per-tracking-session", 50_000, "Unused at 254 (TS Environment still defines NODE_LIMIT_BYTES_PER_TRACKING_SESSION; nothing reads it)")
	f.IntVar(&c.NodeMinimumWealthValueEvent, "world.node-minimum-wealth-value-event", 10, "")
	f.BoolVar(&c.NodeDebug, "world.node-debug", true, "Extra debug info, e.g. missing triggers")
	f.BoolVar(&c.NodeDebugProfile, "world.node-debug-profile", false, "")
	f.BoolVar(&c.NodeDebugSocket, "world.node-debug-socket", false, "")
	f.BoolVar(&c.NodeClientRoutefinder, "world.node-client-route-finder", true, "")
	f.IntVar((*int)(&c.NodeWalktriggerSetting), "world.node-walk-trigger-setting", int(WalkTriggerSettingPlayerpacket), "WalkTriggerSetting: 0=PLAYERPACKET (default), 1=PLAYERSETUP, 2=PLAYERMOVEMENT")
	f.StringVar(&c.NodeProfile, "world.node-profile", "main", "")
	f.StringVar(&c.CachePath, "world.cache-path", "./data/pack", "Cache root; gamemap loads map-pack files from <path>/maps/")
	f.StringVar(&c.ContentPath, "world.content-path", "", "Source content root for ::rebuild's in-process PackAll. Empty disables the cheat.")
	f.StringVar(&c.RSAPrivateKeyPath, "world.rsa-private-key-path", "", "Optional PEM RSA private key (PKCS#1/PKCS#8) for login decryption, replacing the built-in default key. The client must carry the matching public key. Empty uses the built-in key.")
	f.BoolVar(&c.ContentWatch, "world.content-watch", false, "Watch ContentPath subdirs and auto-trigger ::rebuild on changes (debounced 1s). Requires --world.content-path.")
	f.IntVar(&c.NodeMaxPlayers, "world.node-max-players", 2047, "")
	f.IntVar(&c.NodeMaxConnected, "world.node-max-connected", 1000, "")
	f.IntVar(&c.NodeMaxNPCs, "world.node-max-npcs", 16383, "Max live NPCs. Mirrors TS Environment.ts NODE_MAX_NPCS (254 default 16383; was 8191 pre-254).")
	f.StringVar(&c.NodeDebugprocChar, "world.node-debugproc-char", "~", "")
	// rev-254 A4 login rate limits (TS Environment.ts:57-58 @2e3bcf43).
	// Only active in production mode (NodeProduction) — TS gates both
	// checks on NODE_PRODUCTION (World.ts:2107/2172). 0 disables. The
	// 60s/15s windows are TS-hardcoded TTLCache options (World.ts:176-177),
	// not config — see login_ratelimit.go.
	f.IntVar(&c.NodeRatelimitAddressLogin, "world.node-ratelimit-address-login", 30, "Mirror of TS NODE_RATELIMIT_ADDRESS_LOGIN: max op-14 login attempts per remote IP inside a sliding 60s window (production mode only; 0 disables). Exceeded -> reply 16 + close.")
	f.IntVar(&c.NodeRatelimitDeviceLogin, "world.node-ratelimit-device-login", 5, "Mirror of TS NODE_RATELIMIT_DEVICE_LOGIN: max op-16/18 login attempts per uid@IP inside a sliding 15s window (production mode only; 0 disables). Exceeded -> reply 16 + close.")

	f.StringVar(&c.LoginServerAddress, "world.login-server-address", "127.0.0.1:2004", "Login server gRPC address.")
	f.BoolVar(&c.LoginServerEnabled, "world.login-server-enabled", true, "Whether to connect to the login server.")
	f.StringVar(&c.FriendsServerAddress, "world.friends-server-address", "127.0.0.1:2005", "Friends server gRPC address.")
	f.BoolVar(&c.FriendsServerEnabled, "world.friends-server-enabled", false, "Whether to connect to the friends server.")
}

func (c *Config) Validate() error {
	// CFG-2 (Arc 18): only enforce range/required checks when the world
	// module is enabled — Validate runs as part of the cross-module
	// config-merge whether target=world or not, and ondemand-only deployments
	// have no world settings to gate.
	if c.Enable {
		if c.TCPListenPort < 1 || c.TCPListenPort > 65535 {
			return fmt.Errorf("world.tcp-listen-port must be in [1, 65535], got %d", c.TCPListenPort)
		}
		if c.CachePath == "" {
			return fmt.Errorf("world.cache-path must be non-empty when world.enable=true")
		}
		if c.ContentWatch && c.ContentPath == "" {
			return fmt.Errorf("world.content-path must be non-empty when world.content-watch=true")
		}
		if c.RSAPrivateKeyPath != "" {
			if _, err := protocol.LoadRSAKeyPEM(c.RSAPrivateKeyPath); err != nil {
				return fmt.Errorf("world.rsa-private-key-path: %w", err)
			}
		}
	}
	return nil
}
