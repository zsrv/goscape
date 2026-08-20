package hiscore

import (
	"flag"
	"strings"
	"testing"
	"time"
)

func defaultConfig(t *testing.T) Config {
	t.Helper()
	var cfg Config
	fs := flag.NewFlagSet("test", flag.PanicOnError)
	cfg.RegisterFlagsAndApplyDefaults(fs)
	return cfg
}

func TestConfig_Defaults(t *testing.T) {
	cfg := defaultConfig(t)

	if cfg.Enable {
		t.Error("Enable: got true, want false (module off unless asked for)")
	}
	if cfg.Profile != "main" {
		t.Errorf("Profile: got %q, want main", cfg.Profile)
	}
	if cfg.CacheMaxAge != 60*time.Second {
		t.Errorf("CacheMaxAge: got %v, want 60s", cfg.CacheMaxAge)
	}
	if cfg.DefaultLimit != 25 {
		t.Errorf("DefaultLimit: got %d, want 25", cfg.DefaultLimit)
	}
	if cfg.MaxLimit != 100 {
		t.Errorf("MaxLimit: got %d, want 100", cfg.MaxLimit)
	}
	if cfg.LeaderboardMaxRank != 500000 {
		t.Errorf("LeaderboardMaxRank: got %d, want 500000", cfg.LeaderboardMaxRank)
	}
	if cfg.TrustGatewayHeaders {
		t.Error("TrustGatewayHeaders: got true, want false (safe default)")
	}
	if cfg.BackstopRate != 120 {
		t.Errorf("BackstopRate: got %d, want 120", cfg.BackstopRate)
	}
	if cfg.Server.HTTPListenPort != 8082 {
		t.Errorf("Server.HTTPListenPort: got %d, want 8082 (must not collide with portal 8081 or ondemand 8080)", cfg.Server.HTTPListenPort)
	}
	// Both blank selects dskit's built-in proxy-header chain, which is
	// what Kong populates.
	if cfg.Server.LogSourceIPsHeader != "" || cfg.Server.LogSourceIPsRegex != "" {
		t.Errorf("source IP header/regex = %q/%q, want both empty",
			cfg.Server.LogSourceIPsHeader, cfg.Server.LogSourceIPsRegex)
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{"defaults are valid", func(*Config) {}, ""},
		{"empty profile", func(c *Config) { c.Profile = "" }, "profile"},
		{"zero default limit", func(c *Config) { c.DefaultLimit = 0 }, "default_limit"},
		{"default above max", func(c *Config) { c.DefaultLimit = 500 }, "default_limit"},
		{"zero max limit", func(c *Config) { c.MaxLimit = 0 }, "max_limit"},
		{"zero max rank", func(c *Config) { c.LeaderboardMaxRank = 0 }, "leaderboard_max_rank"},
		{"max limit above max rank", func(c *Config) { c.LeaderboardMaxRank = 50; c.MaxLimit = 100 }, "max_limit"},
		{"max limit equal to max rank is valid", func(c *Config) { c.LeaderboardMaxRank = 100; c.MaxLimit = 100 }, ""},
		{"negative cache age", func(c *Config) { c.CacheMaxAge = -time.Second }, "cache_max_age"},
		{"negative backstop", func(c *Config) { c.BackstopRate = -1 }, "backstop_rate"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultConfig(t)
			tc.mutate(&cfg)
			err := cfg.Validate()
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("Validate(): unexpected error %v", err)
			case tc.wantErr != "" && err == nil:
				t.Fatalf("Validate(): got nil, want error mentioning %q", tc.wantErr)
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Fatalf("Validate(): error %q does not mention %q", err, tc.wantErr)
			}
		})
	}
}

// Zero disables the backstop limiter entirely and must stay valid.
func TestConfig_BackstopZeroIsValid(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.BackstopRate = 0
	if err := cfg.Validate(); err != nil {
		t.Fatalf("BackstopRate=0 must be valid (disables limiter), got %v", err)
	}
}
