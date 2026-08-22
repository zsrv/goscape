package ondemand

import (
	"errors"
	"flag"
	"time"

	"github.com/zsrv/goscape/pkg/dskit/server"
	"github.com/zsrv/goscape/pkg/util/pemtoken"
)

// TODO: asset request path rewriting middleware, cache middleware
// TODO: make a cache module similar to tempo but in-memory only?
// TODO: OR embed all files in binary
// TODO: OR use cache but watch files on disk for changes and invalidate cache on change

type Config struct {
	Server server.Config `yaml:",inline"`
	Enable bool          `yaml:"enable"`

	// CachePath is the directory containing the dat/idx client cache files
	// (main_file_cache.dat + main_file_cache.idx0..4). The ondemand module
	// opens a read-only FileStream here to serve archive-0 files over HTTP
	// (web.ts:65-80 at Engine-TS 9aadcec4). Defaults to "./data/pack",
	// matching the world module's cache-path flag idiom (world.Config:93).
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

	// WsTokenProtection enables the per-deployment token gate for rs2.cgi.
	// When true, Rs2CgiHandler computes the public per-deployment token from
	// PubPEM (via pkg/util/pemtoken) and injects it into the client.html
	// template so the browser sets a cookie before opening a WebSocket
	// connection. Mirrors web.ts:105 + Environment.ts:21
	// WEB_SOCKET_TOKEN_PROTECTION (default false).
	WsTokenProtection bool `yaml:"ws_token_protection"`

	// PubPEM is the RSA public key in PKIX PEM form used to derive the
	// per-deployment token when WsTokenProtection is true. Loaded from the
	// same RSA public key file that the world module uses for login RSA.
	// YAML-only (no CLI flag — PEM path is operator configuration).
	PubPEM []byte `yaml:"pub_pem,omitempty"`
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

	// WsOndemand mirrors NODE_WS_ONDEMAND (Environment.ts:62, default false).
	// When true, WS-originated connections are permitted to enter the OnDemand
	// (state-2) path in the world connection state machine. TS gates this at
	// the message handler (web.ts:171-175); in goscape the WS proxy bridges
	// raw bytes to the world conn handler which owns state dispatch, so the
	// gate cannot be enforced at the proxy layer without WS-origin tracking
	// plumbing. The field is recorded here for operational parity and future
	// use — see PORTING-EXCEPTION (rev244-b3-ws-ondemand-gate).
	// PORTING-EXCEPTION (rev244-b3-ws-ondemand-gate): TS web.ts:165-176 gates
	// state-2 OnDemand routing on NODE_WS_ONDEMAND at the WS message layer.
	// goscape's WS proxy delegates to worldConn.HandleConn which owns the state
	// machine; the proxy has no visibility into client.state. Enforcing the gate
	// world-side requires a WS-origin marker on the client struct (out of scope
	// for B3). Config field recorded for documentation; gate is not enforced
	// until a WS-origin marker is added to the world client. See docs/PORTING.md.
	WsOndemand bool `yaml:"ws_ondemand"`
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

	f.StringVar(&c.CachePath, "ondemand.cache-path", "./data/pack", "Cache root containing main_file_cache.dat + idx files; archive-0 files are served over HTTP from here.")
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
	// NODE_WS_ONDEMAND (Environment.ts:62, default false). See WsOndemand doc
	// comment above for the gate-enforcement limitation.
	f.BoolVar(&c.WebSocket.WsOndemand, "ondemand.websocket-ws-ondemand", false, "Mirror of TS NODE_WS_ONDEMAND: permits WS-originated connections to enter the OnDemand (state-2) path. See config doc for enforcement limitation.")

	// WEB_SOCKET_TOKEN_PROTECTION (Environment.ts:21, default false). PubPEM
	// is YAML-only (operator-supplied PEM bytes; no CLI equivalent).
	f.BoolVar(&c.WsTokenProtection, "ondemand.websocket-token-protection", false, "Mirror of TS WEB_SOCKET_TOKEN_PROTECTION: injects a per-deployment token cookie into the rs2.cgi client page. Requires pub_pem to be set in config YAML.")
}

// Validate verifies that the config is self-consistent.
//
// When WsTokenProtection is true, PubPEM must be non-empty and parse as a
// valid RSA public key. This mirrors PemUtil.ts:10 (Engine-TS 9aadcec4) which
// reads and parses the PEM at module load — a bad or missing file causes a
// startup-fatal error rather than a per-request failure.
func (c *Config) Validate() error {
	if c.WsTokenProtection {
		if len(c.PubPEM) == 0 {
			return errors.New("ondemand: WsTokenProtection requires pub_pem to be set")
		}
		// Pre-parse by calling Token with a throwaway hostname; if the PEM is
		// malformed the error surfaces at startup, not at first request.
		if _, err := pemtoken.Token(c.PubPEM, ""); err != nil {
			return errors.New("ondemand: WsTokenProtection pub_pem is invalid: " + err.Error())
		}
	}
	return nil
}
