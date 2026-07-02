package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/drone/envsubst"
	"go.yaml.in/yaml/v2"

	"github.com/zsrv/goscape/cmd/goscape/app"
	"github.com/zsrv/goscape/pkg/dskit/flagext"
	"github.com/zsrv/goscape/pkg/util/build"
	"github.com/zsrv/goscape/pkg/util/log"
)

func main() {
	config, configVerify, printVersion, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse config: %v\n", err)
		os.Exit(1)
	}

	if printVersion {
		fmt.Println(build.String())
		return
	}

	logger, err := log.NewLogger(slog.Level(config.LogLevel), config.LogFormat, os.Stdout, log.WithSourceFormat(config.LogSource))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create logger: %v\n", err)
		os.Exit(1)
	}

	isValid := configIsValid(logger, config)

	if configVerify {
		if !isValid {
			os.Exit(1)
		}
		os.Exit(0)
	}

	if !isValid {
		// arch-29.5: normal-mode boot used to log and proceed anyway,
		// letting an invalid config fail again later (e.g. mid-Manager
		// startup) instead of failing fast at the same point verify mode
		// would have caught it.
		logger.Error("configuration invalid; refusing to start")
		os.Exit(1)
	}

	// TODO: OpenTelemetry

	// Start goscape
	g, err := app.New(logger, *config)
	if err != nil {
		logger.Error("error initializing goscape", "err", err)
		os.Exit(1)
	}

	logger.Info("starting goscape", "target", config.Target, "version", build.Version, "revision", build.Revision)

	if err := g.Run(); err != nil {
		logger.Error("error running goscape", "err", err)
		os.Exit(1)
	}
}

// configIsValid runs hard validation and warns the user for suspect
// configurations. Only Validate errors make the config invalid — CheckConfig
// warnings are logged but never fail verification (a warning that fails
// --config.verify would be a contradiction: warnings describe configs that
// can still boot, just suspiciously).
func configIsValid(logger *slog.Logger, config *app.Config) bool {
	if err := config.Validate(); err != nil {
		logger.Error("configuration invalid", "err", err)
		return false
	}
	if warnings := config.CheckConfig(); len(warnings) > 0 {
		for _, w := range warnings {
			output := []any{"msg", w.Message}
			if w.Explain != "" {
				output = append(output, "explain", w.Explain)
			}
			logger.Warn("configuration warnings exist", output...)
		}
	}
	return true
}

// loadConfig returns the loaded config, whether --config.verify was
// requested, and whether --version was requested. --version is a fast
// path detected during the pre-scan below, before the config file is
// read or validated — the returned config is nil whenever version is
// true, so a bare `goscape --version` never needs --config.file.
func loadConfig() (*app.Config, bool, bool, error) {
	const (
		configFileOption      = "config.file"
		configExpandEnvOption = "config.expand-env"
		configVerifyOption    = "config.verify"
		versionOption         = "version"
	)

	var (
		configFile      string
		configExpandEnv bool
		configVerify    bool
		printVersion    bool
	)

	args := os.Args[1:]
	config := &app.Config{}

	// get the config file
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	fs.StringVar(&configFile, configFileOption, "", "")
	fs.BoolVar(&configExpandEnv, configExpandEnvOption, false, "")
	fs.BoolVar(&configVerify, configVerifyOption, false, "")
	fs.BoolVar(&printVersion, versionOption, false, "")

	// Try to find -config.file & -config.expand-env flags. As Parsing stops on the first error, eg. unknown flag,
	// we simply try remaining parameters until we find config flag, or there are no params left.
	// (ContinueOnError just means that flag.Parse doesn't call panic or os.Exit, but it returns error, which we ignore)
	for len(args) > 0 {
		_ = fs.Parse(args)
		args = args[1:]
	}

	if printVersion {
		return nil, false, true, nil
	}

	// load config defaults and register flags
	config.RegisterFlagsAndApplyDefaults(flag.CommandLine)

	// overlay with config file if provided
	if configFile != "" {
		buf, err := os.ReadFile(configFile)
		if err != nil {
			return nil, false, false, fmt.Errorf("failed to read configFile %s: %w", configFile, err)
		}

		if configExpandEnv {
			s, err := envsubst.EvalEnv(string(buf))
			if err != nil {
				return nil, false, false, fmt.Errorf("failed to expand env vars from configFile %s: %w", configFile, err)
			}
			buf = []byte(s)
		}

		err = yaml.UnmarshalStrict(buf, config)
		if err != nil {
			return nil, false, false, fmt.Errorf("failed to parse configFile %s: %w", configFile, err)
		}
	}

	// overlay with cli
	flagext.IgnoredFlag(flag.CommandLine, configFileOption, "Configuration file to load")
	flagext.IgnoredFlag(flag.CommandLine, configExpandEnvOption, "Whether to expand environment variables in the config file")
	flagext.IgnoredFlag(flag.CommandLine, configVerifyOption, "Verify configuration and exit")
	flagext.IgnoredFlag(flag.CommandLine, versionOption, "Print version information and exit")
	flag.Parse()

	return config, configVerify, false, nil
}
