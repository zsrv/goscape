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
}

func (c *Config) Validate() error {
	return nil
}
