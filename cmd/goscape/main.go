package main

import (
	"fmt"
	"os"
	"strings"

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
		// Use config file from the flag.
		viper.SetConfigFile(configFile)

		if err := viper.ReadInConfig(); err != nil {
			return nil, err
		}
	}

	config := &app.Config{}

	if err := viper.Unmarshal(config); err != nil {
		return nil, err
	}

	return config, nil
}
