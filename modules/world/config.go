package world

import (
	"flag"
	"fmt"
	"time"

	"github.com/zsrv/goscape/pkg/dskit/server"
	"github.com/zsrv/goscape/pkg/io/protocol"
	"github.com/zsrv/goscape/pkg/util/log"
)

type Config struct {
	SignalHandler     SignalHandler `yaml:"-"`
	LogLevel          *log.Level    `yaml:"log_level"`
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
	RSAPrivateKeyPath string `yaml:"rsa_private_key_path"`
	// WordEncPath is the path to the wordenc jagfile that encfilter.Load
	// reads at boot to build the chat word-censoring filter. Defaults to
	// "data/raw/wordenc", the TS-faithful hardcoded relative path (Engine-TS
	// WordEnc.ts:35-37) resolved against the process working directory,
	// exactly as before this field existed. Go-original operational knob
	// (same pattern as RSAPrivateKeyPath): lets an embedder that runs from a
	// cwd other than the goscape repo root (e.g. a singleplayer binary)
	// point at an absolute path instead.
	WordEncPath                      string             `yaml:"wordenc_path"`
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
	f.DurationVar(&c.TCPServerWriteTimeout, "world.tcp-write-timeout", 2*time.Second, "Per-write deadline for the client's outbound socket writer and the drain budget on close. Socket writes never run on the tick goroutine (SEC1 M-2).")
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
	f.IntVar((*int)(&c.NodeWalktriggerSetting), "world.node-walk-trigger-setting", int(WalkTriggerSettingPlayerpacket), "WalkTriggerSetting: 0=PLAYERPACKET (default), 1=PLAYERSETUP, 2=PLAYERMOVEMENT")
	f.StringVar(&c.NodeProfile, "world.node-profile", "main", "")
	f.StringVar(&c.CachePath, "world.cache-path", "./data/pack", "Cache root; gamemap loads map-pack files from <path>/maps/")
	f.StringVar(&c.ContentPath, "world.content-path", "", "Source content root for ::rebuild's in-process PackAll. Empty disables the cheat.")
	f.StringVar(&c.RSAPrivateKeyPath, "world.rsa-private-key-path", "", "Optional PEM RSA private key (PKCS#1/PKCS#8) for login decryption, replacing the built-in default key. The client must carry the matching public key. Empty uses the built-in key.")
	f.StringVar(&c.WordEncPath, "world.wordenc-path", "data/raw/wordenc", "Path to the wordenc jagfile (chat word-censoring filter data), resolved against the process working directory. Default is the TS-faithful hardcoded relative path; override for embedders that run from a different cwd.")
	f.BoolVar(&c.ContentWatch, "world.content-watch", false, "Watch ContentPath subdirs and auto-trigger ::rebuild on changes (debounced 1s). Requires --world.content-path.")
	f.IntVar(&c.NodeMaxPlayers, "world.node-max-players", 2047, "")
	f.IntVar(&c.NodeMaxConnected, "world.node-max-connected", 1000, "")
	f.IntVar(&c.NodeMaxNPCs, "world.node-max-npcs", 8191, "")
	f.StringVar(&c.NodeDebugprocChar, "world.node-debugproc-char", "~", "")

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
