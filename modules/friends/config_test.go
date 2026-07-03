package friends

import (
	"testing"
	"time"
)

// validConfig returns an enabled Config that passes Validate, so each test can
// flip a single field to isolate the invariant it exercises.
func validConfig() Config {
	return Config{
		Enable:                  true,
		GRPCListenPort:          2005,
		SQLiteDSN:               "data/friends.db",
		WorldPlayerLimit:        2000,
		Profile:                 "main",
		GracefulShutdownTimeout: defaultGracefulStopBound,
	}
}

func TestConfigValidate_Valid(t *testing.T) {
	c := validConfig()
	if err := c.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

// A disabled module short-circuits Validate: even an otherwise-invalid zero
// grace must not error, since nothing consumes it.
func TestConfigValidate_DisabledSkipsChecks(t *testing.T) {
	c := Config{Enable: false, GracefulShutdownTimeout: 0}
	if err := c.Validate(); err != nil {
		t.Fatalf("disabled config rejected: %v", err)
	}
}

// A non-positive graceful_shutdown_timeout used to be silently coerced to the
// default (newGRPCServer falls back rather than wiring time.After(0)), which
// masked an operator typo. Validate now rejects it outright.
func TestConfigValidate_RejectsNonPositiveGrace(t *testing.T) {
	for _, d := range []time.Duration{0, -1 * time.Second} {
		c := validConfig()
		c.GracefulShutdownTimeout = d
		if err := c.Validate(); err == nil {
			t.Fatalf("GracefulShutdownTimeout=%s: got nil error, want rejection", d)
		}
	}
}
