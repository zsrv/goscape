package asset

type Config struct {
	HTTPListenNetwork string `mapstructure:"http_listen_network"`
	HTTPListenAddress string `mapstructure:"http_listen_address"`
	HTTPListenPort    int    `mapstructure:"http_listen_port"`
}
