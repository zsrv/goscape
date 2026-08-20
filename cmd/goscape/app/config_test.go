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

// TestConfigValidate_ZeroValue confirms a config with just its registered
// defaults applied (no CLI/env overrides) is accepted — it represents the
// fully-disabled state (all modules opt-in via .Enable), which is the
// baseline for our App.Run-with-disabled-modules tests. Unlike
// World/Login/Friends, gamedb.Config has no .Enable of its own (login and
// friends opt into the shared database instead), so its Validate always
// requires a valid backend — a raw `&Config{}` literal (Database.Backend
// == "") now fails; NewDefaultConfig applies RegisterFlagsAndApplyDefaults
// first, which gives Database.Backend its "sqlite" default (database
// module, task 3).
// COV-1 (Arc 18).
func TestConfigValidate_ZeroValue(t *testing.T) {
	c := NewDefaultConfig()
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

// TestConfig_HiscoreDefaults pins that the hiscore module is registered
// in the app config and defaults to off.
func TestConfig_HiscoreDefaults(t *testing.T) {
	cfg := NewDefaultConfig()

	if cfg.Hiscore.Enable {
		t.Error("Hiscore.Enable: got true, want false by default")
	}
	if cfg.Hiscore.Profile != "main" {
		t.Errorf("Hiscore.Profile: got %q, want main", cfg.Hiscore.Profile)
	}
	if cfg.Hiscore.LeaderboardMaxRank != 500000 {
		t.Errorf("Hiscore.LeaderboardMaxRank: got %d, want 500000", cfg.Hiscore.LeaderboardMaxRank)
	}
}
