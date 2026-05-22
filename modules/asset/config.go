package asset

import (
	"flag"
	"time"

	"github.com/zsrv/goscape/internal/dskit/server"
)

// TODO: asset request path rewriting middleware, cache middleware
// TODO: make a cache module similar to tempo but in-memory only?
// TODO: OR embed all files in binary
// TODO: OR use cache but watch files on disk for changes and invalidate cache on change

type Config struct {
	Server server.Config `yaml:",inline"`
	Enable bool          `yaml:"enable"`

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

	// WebSocket toggles a / WebSocket bridge that accepts WS-framed
	// connections and hands them off to the world module's TCP connection
	// handler. Mirrors web.ts:125-127 in Engine-TS.
	WebSocket WebSocketConfig `yaml:"websocket"`
}

// WebSocketConfig configures the asset module's WebSocket → world bridge.
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
	f.StringVar(&c.Server.HTTPListenAddress, "asset.http-listen-address", "127.0.0.1", "HTTP asset server listen address.")
	f.StringVar(&c.Server.HTTPListenNetwork, "asset.http-listen-network", server.DefaultNetwork, "HTTP asset server listen network, default tcp")
	//f.StringVar(&c.Config.HTTPTLSConfig.TLSCertPath, "asset.http-tls-cert-path", "", "HTTP asset server cert path.")
	//f.StringVar(&c.Config.HTTPTLSConfig.TLSKeyPath, "asset.http-tls-key-path", "", "HTTP asset server key path.")
	//f.StringVar(&c.Config.HTTPTLSConfig.ClientAuth, "asset.http-tls-client-auth", "", "HTTP TLS Client Auth type.")
	//f.StringVar(&c.Config.HTTPTLSConfig.ClientCAs, "asset.http-tls-ca-path", "", "HTTP TLS Client CA path.")
	//f.StringVar(&c.Config.CipherSuites, "asset.http-tls-cipher-suites", "", "HTTP TLS Cipher Suites.")
	//f.StringVar(&c.Config.MinVersion, "asset.http-tls-min-version", "", "HTTP TLS Min Version.")
	f.IntVar(&c.Server.HTTPListenPort, "asset.http-listen-port", 8080, "HTTP asset server listen port.")
	//f.IntVar(&c.Config.HTTPConnLimit, "asset.http-conn-limit", 0, "Maximum number of simultaneous http connections, <=0 to disable")
	f.DurationVar(&c.Server.ServerGracefulShutdownTimeout, "asset.graceful-shutdown-timeout", 30*time.Second, "Timeout for graceful shutdowns")
	f.DurationVar(&c.Server.HTTPServerReadTimeout, "asset.http-read-timeout", 30*time.Second, "Read timeout for HTTP server")
	f.DurationVar(&c.Server.HTTPServerWriteTimeout, "asset.http-write-timeout", 30*time.Second, "Write timeout for HTTP server")
	f.DurationVar(&c.Server.HTTPServerIdleTimeout, "asset.http-idle-timeout", 120*time.Second, "Idle timeout for HTTP server")

	// Source-IP extraction for client IP logging. The default header
	// matches the TS upstream's getIp() primary in Engine-TS/src/web.ts
	// (cf-connecting-ip), and the default regex extracts only the first
	// comma-separated address to match TS' .split(',')[0].trim() behaviour.
	// Note: dskit's SourceIPExtractor is single-source — setting a custom
	// header replaces (does not augment) its built-in Forwarded / X-Real-IP
	// / X-Forwarded-For chain. Blank both header and regex to fall back to
	// that chain (covers x-forwarded-for) at the cost of cf-connecting-ip.
	f.BoolVar(&c.Server.LogSourceIPs, "asset.log-source-ips-enabled", true, "Optionally log the source IPs.")
	f.StringVar(&c.Server.LogSourceIPsHeader, "asset.log-source-ips-header", "cf-connecting-ip", "Header field storing the source IPs. Used in conjunction with asset.log-source-ips-regex. Leave both empty to use the built-in Forwarded/X-Real-IP/X-Forwarded-For chain.")
	f.StringVar(&c.Server.LogSourceIPsRegex, "asset.log-source-ips-regex", `^\s*([^,]+?)\s*(?:,|$)`, "Regex for matching the source IPs. The first capture group is used. Used in conjunction with asset.log-source-ips-header.")
	f.BoolVar(&c.Server.LogSourceIPsFull, "asset.log-source-ips-full", false, "Log all source IPs instead of returning the first match.")

	f.StringVar(&c.PublicDir, "asset.public-dir", "./public", "Filesystem directory served as a static-file fallback after named routes do not match.")

	// /rs2.cgi bootstrap params (mirror web.ts:88-113 + Environment.NODE_*).
	// Duplicated here rather than cross-imported from world.Config to keep
	// dskit modules independent; defaults match the world module's analogues.
	f.IntVar(&c.NodeID, "asset.node-id", 10, "World ID emitted by the /rs2.cgi bootstrap.")
	f.BoolVar(&c.Members, "asset.node-members", true, "Whether members content is available; emitted by /rs2.cgi.")
	f.IntVar(&c.Port, "asset.node-port", 43594, "World TCP port; /rs2.cgi emits portoff = node-port - 43594 to the Java applet.")
	f.BoolVar(&c.Debug, "asset.node-debug", true, "Whether /rs2.cgi may serve the Java applet template when plugin=1.")

	// WebSocket bridge (mirrors web.ts:125-127). AllowedOrigins is YAML-only
	// to match the slice-shape registration pattern used elsewhere; empty
	// default matches TS WEB_CORS_ALLOWED_ORIGINS empty default (allow all).
	f.BoolVar(&c.WebSocket.Enable, "asset.websocket-enable", true, "Serve a WebSocket bridge at / that forwards binary frames to the world module's TCP connection handler.")
	f.Int64Var(&c.WebSocket.MaxPayloadBytes, "asset.websocket-max-payload-bytes", 2000, "Maximum size of a single inbound WebSocket message (bytes). Matches TS web.ts:125 maxPayloadLength: 2_000.")
}

func (c *Config) Validate() error {
	return nil
}
