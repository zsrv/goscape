package app

import (
	"flag"
	"fmt"
	"log/slog"

	"github.com/zsrv/goscape/modules/friends"
	"github.com/zsrv/goscape/modules/login"
	"github.com/zsrv/goscape/modules/ondemand"
	"github.com/zsrv/goscape/modules/world"
	"github.com/zsrv/goscape/pkg/util/log"
)

type Config struct {
	Target    string           `yaml:"target,omitempty"`
	LogFormat string           `yaml:"log_format,omitempty"`
	LogLevel  log.Level        `yaml:"log_level,omitempty"`  // global log level, default for modules too
	LogSource log.SourceFormat `yaml:"log_source,omitempty"` // how the `source` attribute is rendered

	OnDemand ondemand.Config `yaml:"ondemand,omitempty"`
	Friends  friends.Config  `yaml:"friends,omitempty"`
	Login    login.Config    `yaml:"login,omitempty"`
	World    world.Config    `yaml:"world,omitempty"`
}

func NewDefaultConfig() *Config {
	defaultConfig := &Config{}
	defaultFS := flag.NewFlagSet("", flag.PanicOnError)
	defaultConfig.RegisterFlagsAndApplyDefaults(defaultFS)
	return defaultConfig
}

func (c *Config) RegisterFlagsAndApplyDefaults(f *flag.FlagSet) {
	c.Target = SingleBinary

	// Global settings

	f.StringVar(&c.Target, "target", SingleBinary, "Target module")
	f.TextVar(&c.LogLevel, "log.level", log.Level(slog.LevelInfo), "Only log messages with the given severity or above. Valid levels: [trace, debug, info, warn, error]")
	f.StringVar(&c.LogFormat, "log.format", "text", "Output log messages in the given format. Valid formats: [text, json]")
	f.TextVar(&c.LogSource, "log.source", log.SourceRelative, "Render the source attribute as a path. relative (default): module-root-relative, e.g. modules/world/file.go:42 (clickable from the repo root). short: filename only. full: the compiler's path.")

	// Everything else

	c.OnDemand.RegisterFlagsAndApplyDefaults(f)
	c.Friends.RegisterFlagsAndApplyDefaults(f)
	c.Login.RegisterFlagsAndApplyDefaults(f)
	c.World.RegisterFlagsAndApplyDefaults(f)
}

// Validate fans out to each module's Validate, returning the first error
// wrapped with the module's name.
//
// CFG-2 (Arc 18) fanned out world; arch-29.5 completes the fan-out —
// --config.verify used to green-light login/friends/ondemand configs that
// then failed at boot. World, Login, and Friends each short-circuit
// internally on !c.Enable; OnDemand.Validate does not (it only guards
// WsTokenProtection), so the call is gated here instead.
func (c *Config) Validate() error {
	if err := c.World.Validate(); err != nil {
		return fmt.Errorf("world: %w", err)
	}
	if err := c.Login.Validate(); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	if err := c.Friends.Validate(); err != nil {
		return fmt.Errorf("friends: %w", err)
	}
	if c.OnDemand.Enable {
		if err := c.OnDemand.Validate(); err != nil {
			return fmt.Errorf("ondemand: %w", err)
		}
	}
	return nil
}

// CheckConfig checks if config values are suspect and returns a bundled list
// of warnings and explanation. Unlike Validate, a non-empty result does not
// indicate an invalid configuration — the server can still start.
//
// arch-29.5: ondemand and world each carry their own copy of node_id,
// node_port/tcp_listen_port, and node_members (documented in their Config
// structs — cross-importing would break dskit module independence).
// Operators running both modules together must keep them in sync by hand;
// this only warns when they drift, since /rs2.cgi would otherwise silently
// advertise the wrong values to the Java applet.
func (c *Config) CheckConfig() []ConfigWarning {
	var warnings []ConfigWarning

	if c.World.Enable && c.OnDemand.Enable {
		if c.OnDemand.Port != c.World.TCPListenPort {
			warnings = append(warnings, ConfigWarning{
				Message: "ondemand.node_port does not match world.tcp_listen_port",
				Explain: "/rs2.cgi will advertise a game port the world is not listening on",
			})
		}
		if c.OnDemand.NodeID != c.World.NodeID {
			warnings = append(warnings, ConfigWarning{
				Message: "ondemand.node_id does not match world.node_id",
			})
		}
		if c.OnDemand.Members != c.World.NodeMembers {
			warnings = append(warnings, ConfigWarning{
				Message: "ondemand.node_members does not match world.node_members",
			})
		}
	}

	return warnings
}

// ConfigWarning bundles message and explanation strings in one structure.
type ConfigWarning struct {
	Message string
	Explain string
}
