package app

import (
	"strings"
	"testing"
)

// TestNewDefaultConfig pins that flag registration and defaults can be
// applied without panicking — this is the path used by the production binary
// to construct a baseline before YAML/env/CLI overrides are layered on.
// COV-1 (Arc 18).
func TestNewDefaultConfig(t *testing.T) {
	cfg := NewDefaultConfig()
	if cfg == nil {
		t.Fatal("NewDefaultConfig returned nil")
	}
	if cfg.Target != SingleBinary {
		t.Errorf("Target = %q, want %q", cfg.Target, SingleBinary)
	}
	if cfg.LogFormat != "text" {
		t.Errorf("LogFormat = %q, want %q", cfg.LogFormat, "text")
	}
}

// TestConfigValidate_ZeroValue confirms the zero-value Config is accepted —
// it represents the fully-disabled state (all modules opt-in via .Enable),
// which is the baseline for our App.Run-with-disabled-modules tests.
// COV-1 (Arc 18).
func TestConfigValidate_ZeroValue(t *testing.T) {
	c := &Config{}
	if err := c.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

// TestConfigValidate_WorldFanOut confirms Validate propagates world
// validation errors. World is the second child Validate calls. Uses an
// invalid port to trigger world.Config.Validate (which only checks port
// range when Enable=true, per CFG-2 in Arc 18).
// COV-1 (Arc 18).
func TestConfigValidate_WorldFanOut(t *testing.T) {
	c := NewDefaultConfig()
	c.World.Enable = true
	c.World.TCPListenPort = 70000 // out of [1,65535]
	err := c.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want world port-range error")
	}
	if !strings.Contains(err.Error(), "tcp-listen-port") {
		t.Errorf("Validate() = %v, want error mentioning tcp-listen-port", err)
	}
}

// TestConfigCheckConfig confirms the warnings hook returns an empty slice
// (the production implementation is currently a stub but is on the
// public surface).
// COV-1 (Arc 18).
func TestConfigCheckConfig(t *testing.T) {
	c := &Config{}
	if got := c.CheckConfig(); len(got) != 0 {
		t.Errorf("CheckConfig() = %v, want empty", got)
	}
}

// newDefaultTestConfig returns a fresh default Config (flags registered on a
// throwaway FlagSet, mirroring NewDefaultConfig) for tests that need to
// mutate fields afterward. arch-29.5.
func newDefaultTestConfig(t *testing.T) *Config {
	t.Helper()
	return NewDefaultConfig()
}

// TestValidateFansOutToAllModules confirms Validate reaches every module's
// Validate, not just World's — before arch-29.5, --config.verify green-lit
// login/friends/ondemand configs that then failed at boot. arch-29.5.
func TestValidateFansOutToAllModules(t *testing.T) {
	cfg := newDefaultTestConfig(t)
	cfg.Login.Enable = true
	cfg.Login.BCryptCost = 99 // out of range per modules/login/config.go Validate
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "login") {
		t.Fatalf("want login validation error, got %v", err)
	}
}

// TestCheckConfigWarnsOnOndemandWorldDrift pins the ondemand<->world
// node_port drift warning: when both modules are enabled but their mirrored
// node fields disagree, /rs2.cgi silently advertises a game port the world
// isn't listening on. arch-29.5.
func TestCheckConfigWarnsOnOndemandWorldDrift(t *testing.T) {
	cfg := newDefaultTestConfig(t)
	cfg.World.Enable = true
	cfg.OnDemand.Enable = true
	cfg.World.TCPListenPort = 43594
	cfg.OnDemand.Port = 40000 // drifted
	warnings := cfg.CheckConfig()
	if len(warnings) == 0 {
		t.Fatal("want a node_port drift warning")
	}
}

// TestCheckConfigSilentWhenAligned confirms the default ondemand/world node
// fields agree out of the box, so enabling both modules with no overrides
// produces no drift warnings. arch-29.5.
func TestCheckConfigSilentWhenAligned(t *testing.T) {
	cfg := newDefaultTestConfig(t)
	cfg.World.Enable = true
	cfg.OnDemand.Enable = true
	// defaults are aligned
	if w := cfg.CheckConfig(); len(w) != 0 {
		t.Fatalf("want no warnings, got %v", w)
	}
}
