package app

import (
	"flag"
	"log/slog"
	"time"

	"github.com/zsrv/goscape/modules/account"
	"github.com/zsrv/goscape/modules/friends"
	"github.com/zsrv/goscape/modules/hiscore"
	"github.com/zsrv/goscape/modules/login"
	"github.com/zsrv/goscape/modules/ondemand"
	packetcapturemodule "github.com/zsrv/goscape/modules/packetcapture"
	telemetrymodule "github.com/zsrv/goscape/modules/telemetry"
	"github.com/zsrv/goscape/modules/world"
	"github.com/zsrv/goscape/pkg/admin"
	"github.com/zsrv/goscape/pkg/gamedb"
	"github.com/zsrv/goscape/pkg/util/log"
)

type Config struct {
	Target    string           `yaml:"target,omitempty"`
	LogFormat string           `yaml:"log_format,omitempty"`
	LogLevel  log.Level        `yaml:"log_level,omitempty"`  // global log level, default for modules too
	LogSource log.SourceFormat `yaml:"log_source,omitempty"` // how the `source` attribute is rendered

	// Admin is the optional supervisor HTTP listener (GET /healthz, GET
	// /readyz). Empty admin.listen — the default — binds nothing.
	Admin admin.Config `yaml:"admin,omitempty"`

	// ShutdownDelay is how long to wait between SIGTERM and actually
	// stopping the modules. During that window /readyz and the gRPC health
	// service report not-ready, so an orchestrator has time to take this
	// pod out of its Service endpoints before it stops serving. 0 (the
	// default) stops immediately, exactly as before.
	ShutdownDelay time.Duration `yaml:"shutdown_delay,omitempty"`

	Database gamedb.Config `yaml:"database,omitempty"`

	OnDemand ondemand.Config `yaml:"ondemand,omitempty"`
	Friends  friends.Config  `yaml:"friends,omitempty"`
	Login    login.Config    `yaml:"login,omitempty"`
	World    world.Config    `yaml:"world,omitempty"`
	Account  account.Config  `yaml:"account,omitempty"`
	Hiscore  hiscore.Config  `yaml:"hiscore,omitempty"`

	// Telemetry and PacketCapture are user-invisible modules: they are never
	// a --target, they run whenever their own `enabled:` is true, and both
	// default to off.
	Telemetry     telemetrymodule.Config     `yaml:"telemetry,omitempty"`
	PacketCapture packetcapturemodule.Config `yaml:"packetcapture,omitempty"`
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

	f.StringVar(&c.Target, "target", SingleBinary, "Target module: ondemand, world, login, friends, account, hiscore, or all. Dependencies are pulled in (world pulls in login and friends; ondemand pulls in world), and each module still runs only if its <module>.enable is true.")
	f.TextVar(&c.LogLevel, "log.level", log.Level(slog.LevelInfo), "Only log messages with the given severity or above. Valid levels: [trace, debug, info, warn, error]")
	f.StringVar(&c.LogFormat, "log.format", "text", "Output log messages in the given format. Valid formats: [text, json]")
	f.TextVar(&c.LogSource, "log.source", log.SourceRelative, "Render the source attribute as a path. relative (default): module-root-relative, e.g. modules/world/file.go:42 (clickable from the repo root). short: filename only. full: the compiler's path.")
	f.DurationVar(&c.ShutdownDelay, "shutdown-delay", 0, "How long to wait between SIGTERM and shutdown. After receiving SIGTERM, /readyz and the gRPC health service report not-ready/NOT_SERVING.")

	// Everything else

	c.Admin.RegisterFlagsAndApplyDefaults(f)
	c.Database.RegisterFlagsAndApplyDefaults(f)
	c.OnDemand.RegisterFlagsAndApplyDefaults(f)
	c.Friends.RegisterFlagsAndApplyDefaults(f)
	c.Login.RegisterFlagsAndApplyDefaults(f)
	c.World.RegisterFlagsAndApplyDefaults(f)
	c.Account.RegisterFlagsAndApplyDefaults(f)
	c.Hiscore.RegisterFlagsAndApplyDefaults(f)
	c.Telemetry.RegisterFlagsAndApplyDefaults(f)
	packetcapturemodule.RegisterFlagsAndApplyDefaults(&c.PacketCapture, f)
}

// Validate fans out to each module's Validate, returning the first error.
//
// Telemetry goes through the module-level Validate, never the embedded
// Config.Validate: the module accepts an otlp-only shape (enabled with no
// Kafka brokers) that the embedded one reads as a missing broker list. See
// modules/telemetry.Validate.
//
// PacketCapture is validated through packetCaptureConfig so the brokers it
// inherits from telemetry are in place first — otherwise a capture that
// deliberately names no brokers of its own would fail here on the very
// value initPacketCapture is about to supply.
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
	if err := c.Hiscore.Validate(); err != nil {
		return err
	}
	if err := telemetrymodule.Validate(&c.Telemetry); err != nil {
		return err
	}
	capture := c.packetCaptureConfig()
	if err := capture.Validate(); err != nil {
		return err
	}
	return nil
}

// packetCaptureConfig returns the packetcapture config with the values it
// borrows from its neighbours filled in: the world ID it tags rows with, and
// the Kafka brokers it falls back to when it names none of its own. Both are
// owned by another module's config, so an operator enabling capture on a
// telemetry-exporting server flips one knob (packetcapture.enabled).
//
// The receiver is not mutated — c.PacketCapture stays the config as written,
// and both Validate and initPacketCapture read the resolved copy from here so
// the two cannot drift.
func (c *Config) packetCaptureConfig() packetcapturemodule.Config {
	cfg := c.PacketCapture
	cfg.WorldID = int32(c.World.NodeID)
	if len(cfg.Kafka.Brokers) == 0 {
		cfg.Kafka.Brokers = c.Telemetry.Kafka.Brokers
	}
	return cfg
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
