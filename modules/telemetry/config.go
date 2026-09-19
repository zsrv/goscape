package telemetry

import (
	"errors"
	"flag"
	"fmt"

	pkgtelemetry "github.com/zsrv/goscape/pkg/telemetry"
)

// Config re-exports pkg/telemetry.Config under the module package so the dskit
// modules.go registration matches the convention used by other modules.
type Config = pkgtelemetry.Config

// RegisterFlagsAndApplyDefaults forwards to the embedded telemetry config.
func RegisterFlagsAndApplyDefaults(cfg *Config, f *flag.FlagSet) {
	cfg.RegisterFlagsAndApplyDefaults(f)
}

// OTLPOnly reports whether the module runs in otlp-only mode: telemetry is
// enabled but no Kafka broker is configured, so the module boots the global
// OTel providers and registers NO Emitter and NO Kafka client. It is the
// posture for a deployment that wants metrics, traces and logs over OTLP
// without exporting any game events.
func OTLPOnly(cfg *Config) bool {
	return cfg.Enabled && len(cfg.Kafka.Brokers) == 0
}

// Validate is the composition-level replacement for the embedded
// Config.Validate: every composition must call this and never cfg.Validate().
//
// It splits on OTLPOnly. Outside otlp-only mode it IS the public
// Config.Validate, unchanged. In otlp-only mode — the shape the public one
// rejects, because it reads "enabled with no brokers" as a missing broker list
// — the public checks are skipped wholesale and replaced by the only two that
// still mean anything without a Kafka client: an OTLP endpoint is required (a
// module with neither brokers nor an endpoint would boot providers that export
// nowhere), and the trace sample ratio must be a probability. Every Kafka knob
// goes deliberately unvalidated in this mode — nothing reads it.
func Validate(cfg *Config) error {
	if !OTLPOnly(cfg) {
		return cfg.Validate()
	}
	if cfg.OTLP.Endpoint == "" {
		return errors.New("telemetry: otlp.endpoint is required when enabled with no Kafka brokers")
	}
	if cfg.OTLP.SampleRatio < 0 || cfg.OTLP.SampleRatio > 1 {
		return fmt.Errorf("telemetry: OTLP.SampleRatio must be in [0,1], got %v", cfg.OTLP.SampleRatio)
	}
	return nil
}
