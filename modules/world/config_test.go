package world

import (
	"flag"
	"testing"
)

func TestConfigNodeLimitBytesPerTrackingSessionDefault(t *testing.T) {
	var c Config
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	c.RegisterFlagsAndApplyDefaults(fs)
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.NodeLimitBytesPerTrackingSession != 50000 {
		t.Errorf("NodeLimitBytesPerTrackingSession default: got %d, want 50000", c.NodeLimitBytesPerTrackingSession)
	}
}
