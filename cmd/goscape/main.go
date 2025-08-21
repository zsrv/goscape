package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/drone/envsubst"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/zsrv/goscape/cmd/goscape/app"
)

var rootCmd = &cobra.Command{
	Use:   "goscape",
	Short: "An implementation of RuneScape server revision 225",

	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: validate config here
		// TODO: move into its own func

		config, err := loadConfig(cmd)
		if err != nil {
			return err
		}

		fmt.Printf("%+v\n", config)

		return nil
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().String("config.file", "", "configuration file to load")
	rootCmd.Flags().Bool("config.expand-env", false, "whether to expand environment variables in the config file")
	rootCmd.Flags().Bool("config.verify", false, "verify configuration and exit")
	rootCmd.Flags().String("target", "all", "target module to run")
}

func loadConfig(cmd *cobra.Command) (*app.Config, error) {
	// Configuration precedence (highest to lowest): command-line flag, environment variable, config file, default value

	// 1. Set up Viper to use environment variables
	viper.SetEnvPrefix("GOSCAPE")
	// Allow for nested keys in environment variables (e.g. `MYAPP_DATABASE_HOST`)
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	viper.AutomaticEnv() // read in environment variables that match

	// 2. Bind Cobra flags to Viper.
	// This is the magic that makes the flag values available through Viper.
	// It binds the full flag set of the command passed in.
	if err := viper.BindPFlags(cmd.Flags()); err != nil {
		return nil, err
	}

	// 3. Handle the configuration file
	if configFile := viper.GetString("config.file"); configFile != "" {
		buf, err := os.ReadFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file %s: %w", configFile, err)
		}

		if viper.GetBool("config.expand-env") {
			s, err := envsubst.EvalEnv(string(buf))
			if err != nil {
				return nil, fmt.Errorf("failed to expand env vars from config file %s: %w", configFile, err)
			}
			buf = []byte(s)
		}

		viper.SetConfigType(strings.TrimPrefix(filepath.Ext(viper.GetString("config.file")), "."))
		if err := viper.ReadConfig(bytes.NewBuffer(buf)); err != nil {
			return nil, fmt.Errorf("failed to unmarshal config file %s: %w", configFile, err)
		}
	}

	config := &app.Config{}

	if err := viper.Unmarshal(config); err != nil {
		return nil, err
	}

	return config, nil
}
