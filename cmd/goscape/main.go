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
	"github.com/zsrv/goscape/pkg/util/log"
)

func main() {
	config, configVerify, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse config: %v\n", err)
		os.Exit(1)
	}

	logger, err := log.NewLogger(slog.Level(config.LogLevel), config.LogFormat, os.Stdout)
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

	// TODO: OpenTelemetry

	// Start goscape
	g, err := app.New(logger, *config)
	if err != nil {
		logger.Error("error initializing goscape", "err", err)
		os.Exit(1)
	}

	logger.Info("starting goscape", "target", config.Target) // TODO: add version

	if err := g.Run(); err != nil {
		logger.Error("error running goscape", "err", err)
		os.Exit(1)
	}
}

// configIsValid runs hard validation and warns the user for suspect configurations.
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
		return false
	}
	return true
}

func loadConfig() (*app.Config, bool, error) {
	const (
		configFileOption      = "config.file"
		configExpandEnvOption = "config.expand-env"
		configVerifyOption    = "config.verify"
	)

	var (
		configFile      string
		configExpandEnv bool
		configVerify    bool
	)

	args := os.Args[1:]
	config := &app.Config{}

	// get the config file
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	fs.StringVar(&configFile, configFileOption, "", "")
	fs.BoolVar(&configExpandEnv, configExpandEnvOption, false, "")
	fs.BoolVar(&configVerify, configVerifyOption, false, "")

	// Try to find -config.file & -config.expand-env flags. As Parsing stops on the first error, eg. unknown flag,
	// we simply try remaining parameters until we find config flag, or there are no params left.
	// (ContinueOnError just means that flag.Parse doesn't call panic or os.Exit, but it returns error, which we ignore)
	for len(args) > 0 {
		_ = fs.Parse(args)
		args = args[1:]
	}

	// load config defaults and register flags
	config.RegisterFlagsAndApplyDefaults(flag.CommandLine)

	// overlay with config file if provided
	if configFile != "" {
		buf, err := os.ReadFile(configFile)
		if err != nil {
			return nil, false, fmt.Errorf("failed to read configFile %s: %w", configFile, err)
		}

		if configExpandEnv {
			s, err := envsubst.EvalEnv(string(buf))
			if err != nil {
				return nil, false, fmt.Errorf("failed to expand env vars from configFile %s: %w", configFile, err)
			}
			buf = []byte(s)
		}

		err = yaml.UnmarshalStrict(buf, config)
		if err != nil {
			return nil, false, fmt.Errorf("failed to parse configFile %s: %w", configFile, err)
		}
	}

	// overlay with cli
	flagext.IgnoredFlag(flag.CommandLine, configFileOption, "Configuration file to load")
	flagext.IgnoredFlag(flag.CommandLine, configExpandEnvOption, "Whether to expand environment variables in the config file")
	flagext.IgnoredFlag(flag.CommandLine, configVerifyOption, "Verify configuration and exit")
	flag.Parse()

	return config, configVerify, nil
}
