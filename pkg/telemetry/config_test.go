package telemetry_test

import (
	"flag"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/telemetry"
)

func TestConfigDefaults(t *testing.T) {
	var cfg telemetry.Config
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cfg.RegisterFlagsAndApplyDefaults(fs)

	if cfg.Enabled {
		t.Errorf("Enabled default = true, want false")
	}
	if cfg.RingBufferSize != 65536 {
		t.Errorf("RingBufferSize = %d, want 65536", cfg.RingBufferSize)
	}
	if cfg.DrainInterval != 10*time.Millisecond {
		t.Errorf("DrainInterval = %v, want 10ms", cfg.DrainInterval)
	}
	if cfg.DrainBatchMax != 1024 {
		t.Errorf("DrainBatchMax = %d, want 1024", cfg.DrainBatchMax)
	}
	if cfg.StopTimeout != 5*time.Second {
		t.Errorf("StopTimeout = %v, want 5s", cfg.StopTimeout)
	}
	if cfg.Kafka.ClientID != "goscape" {
		t.Errorf("Kafka.ClientID = %q, want %q", cfg.Kafka.ClientID, "goscape")
	}
	if cfg.OTLP.Endpoint != "localhost:4317" {
		t.Errorf("OTLP.Endpoint = %q, want %q", cfg.OTLP.Endpoint, "localhost:4317")
	}
	if cfg.OTLP.SampleRatio != 0.01 {
		t.Errorf("OTLP.SampleRatio = %v, want 0.01", cfg.OTLP.SampleRatio)
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     telemetry.Config
		wantErr bool
	}{
		{"disabled-skips-validation", telemetry.Config{Enabled: false}, false},
		{"enabled-empty-brokers", telemetry.Config{Enabled: true, Kafka: telemetry.KafkaConfig{Brokers: nil}}, true},
		{"enabled-valid", telemetry.Config{
			Enabled:        true,
			Kafka:          telemetry.KafkaConfig{Brokers: []string{"localhost:9092"}, ClientID: "goscape"},
			OTLP:           telemetry.OTLPConfig{Endpoint: "localhost:4317", SampleRatio: 0.01},
			RingBufferSize: 1024,
			DrainInterval:  10 * time.Millisecond,
			DrainBatchMax:  100,
			StopTimeout:    time.Second,
		}, false},
		{"enabled-negative-buffer", telemetry.Config{
			Enabled:        true,
			Kafka:          telemetry.KafkaConfig{Brokers: []string{"localhost:9092"}},
			RingBufferSize: -1,
		}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
