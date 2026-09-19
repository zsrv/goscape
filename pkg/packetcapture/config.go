package packetcapture

import (
	"errors"
	"flag"
	"fmt"
	"time"

	"github.com/zsrv/goscape/pkg/io/protocol/revision"
)

type Config struct {
	Enabled        bool          `yaml:"enabled"`
	Revision       uint16        `yaml:"revision"`
	RingBufferSize int           `yaml:"ring_buffer_size"`
	DrainInterval  time.Duration `yaml:"drain_interval"`
	DrainBatchMax  int           `yaml:"drain_batch_max"`
	StopTimeout    time.Duration `yaml:"stop_timeout"`
	WorldID        int32         `yaml:"world_id"`
	Kafka          KafkaConfig   `yaml:"kafka"`
}

type KafkaConfig struct {
	Brokers  []string `yaml:"brokers"`
	ClientID string   `yaml:"client_id"`
	Topic    string   `yaml:"topic"`
}

func (c *Config) RegisterFlagsAndApplyDefaults(f *flag.FlagSet) {
	f.BoolVar(&c.Enabled, "packetcapture.enabled", false,
		"Enable wire-tap capture into Kafka topic events.replay_packets.")
	c.Revision = revision.Expected
	f.Func("packetcapture.revision", "Wire-protocol revision tag for captured rows (must match the binary's compiled revision).", func(s string) error {
		var v uint16
		if _, err := fmt.Sscanf(s, "%d", &v); err != nil {
			return err
		}
		c.Revision = v
		return nil
	})
	f.IntVar(&c.RingBufferSize, "packetcapture.ring-buffer-size", 65536,
		"Capacity of the replay-capture ring buffer.")
	f.DurationVar(&c.DrainInterval, "packetcapture.drain-interval", 10*time.Millisecond,
		"Cadence at which the shipper drains the ring buffer.")
	f.IntVar(&c.DrainBatchMax, "packetcapture.drain-batch-max", 1024,
		"Maximum records drained per tick.")
	f.DurationVar(&c.StopTimeout, "packetcapture.stop-timeout", 5*time.Second,
		"Maximum time the shipper waits for final flush on stop.")
	f.StringVar(&c.Kafka.ClientID, "packetcapture.kafka.client-id", "goscape", "Kafka client ID.")
	f.StringVar(&c.Kafka.Topic, "packetcapture.kafka.topic", KafkaTopic, "Kafka topic.")
}

func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.Revision == 0 {
		return errors.New("packetcapture: revision must be > 0 when enabled")
	}
	if c.Revision != revision.Expected {
		return fmt.Errorf("packetcapture: configured revision %d does not match binary revision %d",
			c.Revision, uint16(revision.Expected))
	}
	if c.RingBufferSize < 1 || c.RingBufferSize > 1<<20 {
		return fmt.Errorf("packetcapture: ring_buffer_size must be in [1, %d], got %d", 1<<20, c.RingBufferSize)
	}
	if c.DrainBatchMax < 1 {
		return fmt.Errorf("packetcapture: drain_batch_max must be >= 1, got %d", c.DrainBatchMax)
	}
	if len(c.Kafka.Brokers) == 0 {
		return errors.New("packetcapture: at least one Kafka broker required when enabled")
	}
	return nil
}
