package hiscore

import (
	"errors"
	"flag"
	"fmt"
	"time"

	"github.com/zsrv/goscape/pkg/dskit/server"
)

// Config is the hiscore module's configuration. The embedded
// server.Config supplies the listener, timeouts, request logging and
// source-IP extraction (log_source_ips_header / _regex), which is how
// the real client IP is recovered from behind the gateway.
type Config struct {
	Server server.Config `yaml:",inline"`
	Enable bool          `yaml:"enable"`

	// Profile is the default profile queried when a request does not
	// specify one. Boards are per-profile, matching the write path.
	Profile string `yaml:"profile"`

	// CacheMaxAge drives Cache-Control: public, max-age=N. Responses are
	// also ETag'd; this is what makes an edge cache (Kong proxy-cache)
	// effective and is the largest single lever on database load.
	CacheMaxAge time.Duration `yaml:"cache_max_age"`

	DefaultLimit int `yaml:"default_limit"`
	MaxLimit     int `yaml:"max_limit"`

	// LeaderboardMaxRank bounds offset paging (offset+limit must not
	// exceed it). This is a product boundary — the depth of board the
	// hiscores display — not a safety valve; cursor paging is the
	// mechanism for cheap deep reads.
	LeaderboardMaxRank int `yaml:"leaderboard_max_rank"`

	// TrustGatewayHeaders enables reading Kong's X-Consumer-* headers
	// for logging. Default false: nothing is ever authorized by them, so
	// this only controls whether an unverified header can reach a log
	// line. Enable it where a gateway actually fronts the module.
	TrustGatewayHeaders bool `yaml:"trust_gateway_headers"`

	// BackstopRate is a coarse in-process request ceiling per caller per
	// minute, for the case where the module is reached without a
	// gateway. It is not the quota system — Kong's per-consumer
	// rate-limiting is. 0 disables it.
	BackstopRate int `yaml:"backstop_rate"`
}

func (c *Config) RegisterFlagsAndApplyDefaults(f *flag.FlagSet) {
	// server.Config has no flag-registration method of its own — each
	// module registers the server fields it uses under its own prefix.
	// modules/ondemand/config.go is the reference implementation.
	f.StringVar(&c.Server.HTTPListenAddress, "hiscore.http-listen-address", "127.0.0.1", "Hiscore API listen address.")
	f.StringVar(&c.Server.HTTPListenNetwork, "hiscore.http-listen-network", server.DefaultNetwork, "Hiscore API listen network.")
	f.IntVar(&c.Server.HTTPListenPort, "hiscore.http-listen-port", 8082, "Hiscore API listen port.")
	f.DurationVar(&c.Server.ServerGracefulShutdownTimeout, "hiscore.graceful-shutdown-timeout", 30*time.Second, "Timeout for graceful shutdowns.")
	f.DurationVar(&c.Server.HTTPServerReadTimeout, "hiscore.http-read-timeout", 30*time.Second, "Read timeout for the hiscore HTTP server.")
	f.DurationVar(&c.Server.HTTPServerWriteTimeout, "hiscore.http-write-timeout", 30*time.Second, "Write timeout for the hiscore HTTP server.")
	f.DurationVar(&c.Server.HTTPServerIdleTimeout, "hiscore.http-idle-timeout", 120*time.Second, "Idle timeout for the hiscore HTTP server.")

	// Source-IP extraction is how the real client IP is recovered from
	// behind the gateway. Both blank selects dskit's built-in
	// Forwarded / X-Real-IP / X-Forwarded-For chain, which is what Kong
	// populates — deliberately unlike ondemand, which defaults to
	// cf-connecting-ip for its Cloudflare-fronted deployment.
	f.BoolVar(&c.Server.LogSourceIPs, "hiscore.log-source-ips-enabled", true, "Log source IPs on hiscore API requests.")
	f.StringVar(&c.Server.LogSourceIPsHeader, "hiscore.log-source-ips-header", "", "Header holding the source IP. Blank uses the built-in Forwarded/X-Real-IP/X-Forwarded-For chain.")
	f.StringVar(&c.Server.LogSourceIPsRegex, "hiscore.log-source-ips-regex", "", "Regex for extracting the source IP from the configured header. Must be set together with the header.")
	f.BoolVar(&c.Server.LogSourceIPsFull, "hiscore.log-source-ips-full", false, "Log all source IPs instead of the first match.")

	f.BoolVar(&c.Enable, "hiscore.enable", false, "Whether to run the hiscore API module.")
	f.StringVar(&c.Profile, "hiscore.profile", "main", "Default profile queried by the hiscore API.")
	f.DurationVar(&c.CacheMaxAge, "hiscore.cache-max-age", 60*time.Second, "Cache-Control max-age on hiscore API responses.")
	f.IntVar(&c.DefaultLimit, "hiscore.default-limit", 25, "Default leaderboard page size.")
	f.IntVar(&c.MaxLimit, "hiscore.max-limit", 100, "Maximum leaderboard page size.")
	f.IntVar(&c.LeaderboardMaxRank, "hiscore.leaderboard-max-rank", 500000, "Deepest rank reachable by offset paging.")
	f.BoolVar(&c.TrustGatewayHeaders, "hiscore.trust-gateway-headers", false, "Read gateway-supplied X-Consumer-* headers for logging.")
	f.IntVar(&c.BackstopRate, "hiscore.backstop-rate", 120, "In-process requests/minute per caller when no gateway limits apply. 0 disables.")
}

func (c *Config) Validate() error {
	if c.Profile == "" {
		return errors.New("hiscore: profile must not be empty")
	}
	if c.MaxLimit < 1 {
		return fmt.Errorf("hiscore: max_limit must be >= 1, got %d", c.MaxLimit)
	}
	if c.DefaultLimit < 1 || c.DefaultLimit > c.MaxLimit {
		return fmt.Errorf("hiscore: default_limit must be in [1, max_limit=%d], got %d", c.MaxLimit, c.DefaultLimit)
	}
	if c.LeaderboardMaxRank < 1 {
		return fmt.Errorf("hiscore: leaderboard_max_rank must be >= 1, got %d", c.LeaderboardMaxRank)
	}
	if c.CacheMaxAge < 0 {
		return fmt.Errorf("hiscore: cache_max_age must not be negative, got %v", c.CacheMaxAge)
	}
	if c.BackstopRate < 0 {
		return fmt.Errorf("hiscore: backstop_rate must not be negative, got %d", c.BackstopRate)
	}
	return nil
}
