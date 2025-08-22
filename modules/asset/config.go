package asset

import "flag"

type Config struct {
	HTTPListenNetwork string `yaml:"http_listen_network,omitempty"`
	HTTPListenAddress string `yaml:"http_listen_address,omitempty"`
	HTTPListenPort    int    `yaml:"http_listen_port,omitempty"`
}

// RegisterFlagsAndApplyDefaults registers flags and applies defaults.
func (c *Config) RegisterFlagsAndApplyDefaults(f *flag.FlagSet) {
	f.StringVar(&c.HTTPListenNetwork, "asset.http-listen-network", "tcp", "HTTP server listen network")
	f.StringVar(&c.HTTPListenAddress, "asset.http-listen-address", "127.0.0.1", "HTTP server listen address")
	f.IntVar(&c.HTTPListenPort, "asset.http-listen-port", 8080, "HTTP server listen port")
}

func (c *Config) Validate() error {
	return nil
}
