package telemetry

import (
	"errors"
	"flag"
	"fmt"
	"time"
)

type Config struct {
	Enabled        bool          `yaml:"enabled"`
	Kafka          KafkaConfig   `yaml:"kafka"`
	OTLP           OTLPConfig    `yaml:"otlp"`
	RingBufferSize int           `yaml:"ring_buffer_size"`
	DrainInterval  time.Duration `yaml:"drain_interval"`
	DrainBatchMax  int           `yaml:"drain_batch_max"`
	StopTimeout    time.Duration `yaml:"stop_timeout"`
}

type KafkaConfig struct {
	Brokers  []string `yaml:"brokers"`
	ClientID string   `yaml:"client_id"`
}

type OTLPConfig struct {
	Endpoint    string  `yaml:"endpoint"`
	Insecure    bool    `yaml:"insecure"`
	SampleRatio float64 `yaml:"sample_ratio"`
}

func (c *Config) RegisterFlagsAndApplyDefaults(f *flag.FlagSet) {
	f.BoolVar(&c.Enabled, "telemetry.enabled", false, "Enable telemetry emission to Kafka and OpenTelemetry collector.")
	f.IntVar(&c.RingBufferSize, "telemetry.ring-buffer-size", 65536, "Capacity of the per-process telemetry ring buffer.")
	f.DurationVar(&c.DrainInterval, "telemetry.drain-interval", 10*time.Millisecond, "Cadence at which the shipper drains the ring buffer.")
	f.IntVar(&c.DrainBatchMax, "telemetry.drain-batch-max", 1024, "Maximum records drained per tick.")
	f.DurationVar(&c.StopTimeout, "telemetry.stop-timeout", 5*time.Second, "Maximum time the shipper waits for final flush on stop.")
	f.StringVar(&c.Kafka.ClientID, "telemetry.kafka.client-id", "goscape", "Kafka client ID.")
	f.StringVar(&c.OTLP.Endpoint, "telemetry.otlp.endpoint", "localhost:4317", "OTLP gRPC endpoint.")
	f.BoolVar(&c.OTLP.Insecure, "telemetry.otlp.insecure", true, "Connect to OTLP endpoint without TLS.")
	f.Float64Var(&c.OTLP.SampleRatio, "telemetry.otlp.sample-ratio", 0.01, "Trace sampling ratio (0.0-1.0).")
}

func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if len(c.Kafka.Brokers) == 0 {
		return errors.New("telemetry: at least one Kafka broker required when enabled")
	}
	if c.RingBufferSize < 1 {
		return fmt.Errorf("telemetry: RingBufferSize must be >= 1, got %d", c.RingBufferSize)
	}
	if c.DrainBatchMax < 1 {
		return fmt.Errorf("telemetry: DrainBatchMax must be >= 1, got %d", c.DrainBatchMax)
	}
	if c.OTLP.SampleRatio < 0 || c.OTLP.SampleRatio > 1 {
		return fmt.Errorf("telemetry: OTLP.SampleRatio must be in [0,1], got %v", c.OTLP.SampleRatio)
	}
	return nil
}
