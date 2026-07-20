package app

import (
	"flag"
	"log/slog"

	"github.com/zsrv/goscape/modules/account"
	"github.com/zsrv/goscape/modules/friends"
	"github.com/zsrv/goscape/modules/login"
	"github.com/zsrv/goscape/modules/ondemand"
	"github.com/zsrv/goscape/modules/world"
	"github.com/zsrv/goscape/pkg/gamedb"
	"github.com/zsrv/goscape/pkg/util/log"
)

type Config struct {
	Target    string           `yaml:"target,omitempty"`
	LogFormat string           `yaml:"log_format,omitempty"`
	LogLevel  log.Level        `yaml:"log_level,omitempty"`  // global log level, default for modules too
	LogSource log.SourceFormat `yaml:"log_source,omitempty"` // how the `source` attribute is rendered

	Database gamedb.Config `yaml:"database,omitempty"`

	OnDemand ondemand.Config `yaml:"ondemand,omitempty"`
	Friends  friends.Config  `yaml:"friends,omitempty"`
	Login    login.Config    `yaml:"login,omitempty"`
	World    world.Config    `yaml:"world,omitempty"`
	Account  account.Config  `yaml:"account,omitempty"`
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

	c.Database.RegisterFlagsAndApplyDefaults(f)
	c.OnDemand.RegisterFlagsAndApplyDefaults(f)
	c.Friends.RegisterFlagsAndApplyDefaults(f)
	c.Login.RegisterFlagsAndApplyDefaults(f)
	c.World.RegisterFlagsAndApplyDefaults(f)
	c.Account.RegisterFlagsAndApplyDefaults(f)
}

// Validate fans out to each module's Validate, returning the first error.
func (c *Config) Validate() error {
	// database module (task 3): Database.Validate runs before the
	// module fan-out below — unlike World/Login/Friends it has no
	// .Enable of its own (login and friends opt into the shared
	// database instead), so it always requires a valid backend.
	if err := c.Database.Validate(); err != nil {
		return err
	}
	// CFG-2 (Arc 18): fan out world.Validate so port-range, cache-path,
	// and content-watch/content-path coupling are caught at startup.
	if err := c.World.Validate(); err != nil {
		return err
	}
	if err := c.Account.Validate(); err != nil {
		return err
	}
	return nil
}

// CheckConfig checks if config values are suspect and returns a bundled list of warnings and explanation.
func (c *Config) CheckConfig() []ConfigWarning {
	var warnings []ConfigWarning

	// TODO

	return warnings
}

// ConfigWarning bundles message and explanation strings in one structure.
type ConfigWarning struct {
	Message string
	Explain string
}

// TODO: Add ConfigWarnings
