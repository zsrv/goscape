package login

import "flag"

// NewTestConfig returns a Config at flag defaults. Exported for
// cross-module integration tests (modules/login e2e); production code
// builds Config through the app's flag/YAML pipeline instead.
func NewTestConfig() Config {
	var c Config
	fs := flag.NewFlagSet("", flag.PanicOnError)
	c.RegisterFlagsAndApplyDefaults(fs)
	return c
}
