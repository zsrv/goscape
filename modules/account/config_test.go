// modules/account/config_test.go
package account

import (
	"flag"
	"strings"
	"testing"
	"time"
)

func defaultConfig(t *testing.T) Config {
	t.Helper()
	var c Config
	fs := flag.NewFlagSet("", flag.PanicOnError)
	c.RegisterFlagsAndApplyDefaults(fs)
	return c
}

func TestConfig_Defaults(t *testing.T) {
	c := defaultConfig(t)
	if c.Enable {
		t.Error("Enable must default false")
	}
	if c.HTTPListenPort != 8081 || c.GRPCListenPort != 2006 {
		t.Errorf("ports: http=%d grpc=%d", c.HTTPListenPort, c.GRPCListenPort)
	}
	if c.CharacterLimit != 5 {
		t.Errorf("CharacterLimit = %d, want 5", c.CharacterLimit)
	}
	if c.Argon2.MemoryKiB != 65536 || c.Argon2.Time != 2 || c.Argon2.Parallelism != 1 {
		t.Errorf("argon2 defaults: %+v", c.Argon2)
	}
	if c.Session.IdleTTL != 168*time.Hour || c.Session.AbsoluteTTL != 720*time.Hour {
		t.Errorf("session defaults: %+v", c.Session)
	}
	if got := c.Gate.Providers; len(got) != 1 || got[0] != "discord" {
		t.Errorf("Gate.Providers = %v, want [discord]", got)
	}
	if c.SMTP.Port != 587 {
		t.Errorf("SMTP.Port = %d, want 587", c.SMTP.Port)
	}
}

func TestConfig_ValidateDisabledIsAlwaysOK(t *testing.T) {
	c := defaultConfig(t)
	c.Enable = false
	c.HTTPListenPort = -1 // nonsense values must not matter when disabled
	if err := c.Validate(); err != nil {
		t.Fatalf("disabled module must validate clean, got %v", err)
	}
}

func TestConfig_ValidateEnabled(t *testing.T) {
	base := func() Config {
		c := defaultConfig(t)
		c.Enable = true
		c.PublicURL = "http://127.0.0.1:8081"
		return c
	}
	baseC := base()
	if err := baseC.Validate(); err != nil {
		t.Fatalf("valid enabled config rejected: %v", err)
	}

	cases := []struct {
		name    string
		mutate  func(*Config)
		wantSub string
	}{
		{"bad http port", func(c *Config) { c.HTTPListenPort = 0 }, "http_listen_port"},
		{"bad grpc port", func(c *Config) { c.GRPCListenPort = 70000 }, "grpc_listen_port"},
		{"missing public url", func(c *Config) { c.PublicURL = "" }, "public_url"},
		{"trailing slash", func(c *Config) { c.PublicURL = "http://x/" }, "public_url"},
		{"bad limit", func(c *Config) { c.CharacterLimit = 0 }, "character_limit"},
		{"bad argon2 memory", func(c *Config) { c.Argon2.MemoryKiB = 1024 }, "argon2"},
		{"bad argon2 time", func(c *Config) { c.Argon2.Time = 0 }, "argon2"},
		{"argon2 parallelism too high", func(c *Config) { c.Argon2.Parallelism = 256 }, "argon2"},
		{"bad idle ttl", func(c *Config) { c.Session.IdleTTL = 0 }, "session"},
		{"idle > absolute", func(c *Config) { c.Session.IdleTTL = 1000 * time.Hour }, "session"},
		{"unknown gate provider", func(c *Config) { c.Gate.Providers = []string{"myspace"} }, "gate.providers"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base()
			tc.mutate(&c)
			err := c.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("want error containing %q, got %v", tc.wantSub, err)
			}
			if !strings.HasPrefix(err.Error(), "account: ") {
				t.Fatalf("errors must self-prefix 'account: ', got %v", err)
			}
		})
	}
}
