package packetcapture

import (
	"flag"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/io/protocol/revision"
)

func TestConfig_RegisterFlagsAppliesDefaults(t *testing.T) {
	var c Config
	fs := flag.NewFlagSet("test", flag.PanicOnError)
	c.RegisterFlagsAndApplyDefaults(fs)
	_ = fs.Parse(nil)
	if c.Enabled {
		t.Fatal("default Enabled should be false")
	}
	if c.Revision != revision.Expected {
		t.Fatalf("default Revision=%d, want %d", c.Revision, uint16(revision.Expected))
	}
	if c.RingBufferSize != 65536 {
		t.Fatalf("default RingBufferSize=%d, want 65536", c.RingBufferSize)
	}
	if c.DrainBatchMax != 1024 {
		t.Fatalf("default DrainBatchMax=%d, want 1024", c.DrainBatchMax)
	}
}

func TestConfig_ValidateDisabledOK(t *testing.T) {
	var c Config
	c.Enabled = false
	if err := c.Validate(); err != nil {
		t.Fatalf("disabled config rejected: %v", err)
	}
}

func TestConfig_ValidateEnabledRequiresBrokers(t *testing.T) {
	var c Config
	c.Enabled = true
	c.Revision = revision.Expected
	c.RingBufferSize = 1024
	c.DrainBatchMax = 64
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "broker") {
		t.Fatalf("expected broker-required error, got %v", err)
	}
}

func TestConfig_ValidateRejectsRevisionMismatch(t *testing.T) {
	var c Config
	c.Enabled = true
	c.Revision = revision.Expected + 1
	c.RingBufferSize = 1024
	c.DrainBatchMax = 64
	c.Kafka.Brokers = []string{"localhost:9092"}
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "revision") {
		t.Fatalf("expected revision-mismatch error, got %v", err)
	}
}

func TestConfig_ValidateRejectsZeroRingSize(t *testing.T) {
	var c Config
	c.Enabled = true
	c.Revision = revision.Expected
	c.RingBufferSize = 0
	c.DrainBatchMax = 64
	c.Kafka.Brokers = []string{"localhost:9092"}
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "ring") {
		t.Fatalf("expected ring-size error, got %v", err)
	}
}
