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
}

func (c *Config) Validate() error {
	return nil
}
