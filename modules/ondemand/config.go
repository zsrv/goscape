package ondemand

import (
	"flag"
	"time"

	"github.com/zsrv/goscape/pkg/dskit/server"
)

// TODO: asset request path rewriting middleware, cache middleware
// TODO: make a cache module similar to tempo but in-memory only?
// TODO: OR embed all files in binary
// TODO: OR use cache but watch files on disk for changes and invalidate cache on change

type Config struct {
	Server server.Config `yaml:",inline"`
	Enable bool          `yaml:"enable"`

	// CachePath is the directory containing the packed client cache tree
	// (client/… jags + client/songs/*.mid + client/maps/*). Rev-225 serves
	// these as static files; the default preserves the historical
	// hardcoded data/pack relative path (resolved against the process
	// working directory). Go-original embedding knob — same pattern as
	// world.rsa_private_key_path.
	CachePath string `yaml:"cache_path"`

	// PublicDir is the filesystem directory served as a static-file fallback
	// after named routes do not match. Mirrors web.ts:114-119 in Engine-TS.
	PublicDir string `yaml:"public_dir"`

	// NodeID, Members, Port, Debug are mirrored from the world module's
	// Environment.NODE_* values so the /rs2.cgi bootstrap handler (web.ts:88-113)
	// can serve correct client/applet params without cross-module import.
	// Operators are responsible for keeping these in sync with the world
	// module's analogous flags when running both modules together.
	NodeID  int  `yaml:"node_id"`
	Members bool `yaml:"node_members"`
	Port    int  `yaml:"node_port"`
	Debug   bool `yaml:"node_debug"`

	// DebugStatusEnabled gates GET /debug/status. This endpoint is
	// unauthenticated and returns load/presence information (current tick,
	// players online, tick age), making it an oracle for attackers.
	// Defaults to false (SEC1 M-12).
	DebugStatusEnabled bool `yaml:"debug_status_enabled"`

	// WebSocket toggles a / WebSocket bridge that accepts WS-framed
	// connections and hands them off to the world module's TCP connection
	// handler. Mirrors web.ts:125-127 in Engine-TS.
	WebSocket WebSocketConfig `yaml:"websocket"`
}

// WebSocketConfig configures the OnDemand module's WebSocket → world bridge.
type WebSocketConfig struct {
	Enable bool `yaml:"enable"`

	// AllowedOrigins is the explicit Origin allowlist. Empty slice ⇒
	// allow all (matches TS WEB_CORS_ALLOWED_ORIGINS empty default at
	// Environment.ts:13). Non-empty ⇒ Origin header must exactly match
	// one entry or the upgrade is rejected with 403 before handshake.
	// Operators populate via YAML; no CLI flag (matches the slice-shape
	// registration pattern used elsewhere in this repo).
	AllowedOrigins []string `yaml:"allowed_origins"`

	// MaxPayloadBytes caps inbound WS message size. Mirrors TS
	// maxPayloadLength: 2_000 at web.ts:125.
	MaxPayloadBytes int64 `yaml:"max_payload_bytes"`
}

// RegisterFlagsAndApplyDefaults registers flags and applies defaults.
func (c *Config) RegisterFlagsAndApplyDefaults(f *flag.FlagSet) {
	f.StringVar(&c.Server.HTTPListenAddress, "ondemand.http-listen-address", "127.0.0.1", "HTTP OnDemand server listen address.")
	f.StringVar(&c.Server.HTTPListenNetwork, "ondemand.http-listen-network", server.DefaultNetwork, "HTTP OnDemand server listen network, default tcp")
	//f.StringVar(&c.Config.HTTPTLSConfig.TLSCertPath, "ondemand.http-tls-cert-path", "", "HTTP OnDemand server cert path.")
	//f.StringVar(&c.Config.HTTPTLSConfig.TLSKeyPath, "ondemand.http-tls-key-path", "", "HTTP OnDemand server key path.")
	//f.StringVar(&c.Config.HTTPTLSConfig.ClientAuth, "ondemand.http-tls-client-auth", "", "HTTP TLS Client Auth type.")
	//f.StringVar(&c.Config.HTTPTLSConfig.ClientCAs, "ondemand.http-tls-ca-path", "", "HTTP TLS Client CA path.")
	//f.StringVar(&c.Config.CipherSuites, "ondemand.http-tls-cipher-suites", "", "HTTP TLS Cipher Suites.")
	//f.StringVar(&c.Config.MinVersion, "ondemand.http-tls-min-version", "", "HTTP TLS Min Version.")
	f.IntVar(&c.Server.HTTPListenPort, "ondemand.http-listen-port", 8080, "HTTP OnDemand server listen port.")
	//f.IntVar(&c.Config.HTTPConnLimit, "ondemand.http-conn-limit", 0, "Maximum number of simultaneous http connections, <=0 to disable")
	f.DurationVar(&c.Server.ServerGracefulShutdownTimeout, "ondemand.graceful-shutdown-timeout", 30*time.Second, "Timeout for graceful shutdowns")
	f.DurationVar(&c.Server.HTTPServerReadTimeout, "ondemand.http-read-timeout", 30*time.Second, "Read timeout for HTTP server")
	f.DurationVar(&c.Server.HTTPServerWriteTimeout, "ondemand.http-write-timeout", 30*time.Second, "Write timeout for HTTP server")
	f.DurationVar(&c.Server.HTTPServerIdleTimeout, "ondemand.http-idle-timeout", 120*time.Second, "Idle timeout for HTTP server")

	// Source-IP extraction for client IP logging. The default header
	// matches the TS upstream's getIp() primary in Engine-TS/src/web.ts
	// (cf-connecting-ip), and the default regex extracts only the first
	// comma-separated address to match TS' .split(',')[0].trim() behaviour.
	// Note: dskit's SourceIPExtractor is single-source — setting a custom
	// header replaces (does not augment) its built-in Forwarded / X-Real-IP
	// / X-Forwarded-For chain. Blank both header and regex to fall back to
	// that chain (covers x-forwarded-for) at the cost of cf-connecting-ip.
	f.BoolVar(&c.Server.LogSourceIPs, "ondemand.log-source-ips-enabled", true, "Optionally log the source IPs.")
	f.StringVar(&c.Server.LogSourceIPsHeader, "ondemand.log-source-ips-header", "cf-connecting-ip", "Header field storing the source IPs. Used in conjunction with ondemand.log-source-ips-regex. Leave both empty to use the built-in Forwarded/X-Real-IP/X-Forwarded-For chain.")
	f.StringVar(&c.Server.LogSourceIPsRegex, "ondemand.log-source-ips-regex", `^\s*([^,]+?)\s*(?:,|$)`, "Regex for matching the source IPs. The first capture group is used. Used in conjunction with ondemand.log-source-ips-header.")
	f.BoolVar(&c.Server.LogSourceIPsFull, "ondemand.log-source-ips-full", false, "Log all source IPs instead of returning the first match.")

	f.StringVar(&c.CachePath, "ondemand.cache-path", "./data/pack", "Cache root; archive and song/map files are served from <path>/client/.")
	f.StringVar(&c.PublicDir, "ondemand.public-dir", "./public", "Filesystem directory served as a static-file fallback after named routes do not match.")

	// /rs2.cgi bootstrap params (mirror web.ts:88-113 + Environment.NODE_*).
	// Duplicated here rather than cross-imported from world.Config to keep
	// dskit modules independent; defaults match the world module's analogues.
	f.IntVar(&c.NodeID, "ondemand.node-id", 10, "World ID emitted by the /rs2.cgi bootstrap.")
	f.BoolVar(&c.Members, "ondemand.node-members", true, "Whether members content is available; emitted by /rs2.cgi.")
	f.IntVar(&c.Port, "ondemand.node-port", 43594, "World TCP port; /rs2.cgi emits portoff = node-port - 43594 to the Java applet.")
	f.BoolVar(&c.Debug, "ondemand.node-debug", true, "Whether /rs2.cgi may serve the Java applet template when plugin=1.")
	f.BoolVar(&c.DebugStatusEnabled, "ondemand.debug-status-enabled", false, "Serve GET /debug/status (players online, tick age) on the public ondemand listener. Off by default: it is an unauthenticated load/presence oracle (SEC1 M-12).")

	// WebSocket bridge (mirrors web.ts:125-127). AllowedOrigins is YAML-only
	// to match the slice-shape registration pattern used elsewhere; empty
	// default matches TS WEB_CORS_ALLOWED_ORIGINS empty default (allow all).
	f.BoolVar(&c.WebSocket.Enable, "ondemand.websocket-enable", true, "Serve a WebSocket bridge at / that forwards binary frames to the world module's TCP connection handler.")
	f.Int64Var(&c.WebSocket.MaxPayloadBytes, "ondemand.websocket-max-payload-bytes", 2000, "Maximum size of a single inbound WebSocket message (bytes). Matches TS web.ts:125 maxPayloadLength: 2_000.")
}

func (c *Config) Validate() error {
	return nil
}
