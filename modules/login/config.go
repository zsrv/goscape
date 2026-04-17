package login

import (
	"flag"
)

// Config holds configuration for the login module.
type Config struct {
	// SavePath is the base directory for player save files.
	// Save files are stored at {SavePath}/{profile}/{username}.sav.
	SavePath string `yaml:"save_path"`

	// NodeProfile is the default profile name used when one is not otherwise specified.
	NodeProfile string `yaml:"node_profile"`

	// AutoRegister, when true, creates an account on first login.
	AutoRegister bool `yaml:"auto_register"`

	// AutoSubscribeMembers, when true, upgrades accounts to members when
	// they attempt to log into a members world.
	AutoSubscribeMembers bool `yaml:"auto_subscribe_members"`

	// BCryptCost is the cost parameter used when hashing new passwords.
	BCryptCost int `yaml:"bcrypt_cost"`
}

// RegisterFlagsAndApplyDefaults registers flags and applies defaults.
func (c *Config) RegisterFlagsAndApplyDefaults(f *flag.FlagSet) {
	f.StringVar(&c.SavePath, "login.save-path", "data/saves", "Base directory for player save files")
	f.StringVar(&c.NodeProfile, "login.node-profile", "main", "Default profile name")
	f.BoolVar(&c.AutoRegister, "login.auto-register", true, "Create an account on first login if none exists")
	f.BoolVar(&c.AutoSubscribeMembers, "login.auto-subscribe-members", true, "Automatically upgrade accounts to members when logging into a members world")
	f.IntVar(&c.BCryptCost, "login.bcrypt-cost", 10, "bcrypt cost parameter for hashing new passwords")
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	return nil
}
