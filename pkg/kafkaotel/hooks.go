// Package kafkaotel is the single place that builds the OpenTelemetry hooks
// installed on every production franz-go client in this module.
//
// kotel's Meter implements the kgo broker/produce/fetch hooks and exports the
// messaging.kafka.* client metrics (connects, connect_errors, disconnects,
// write_errors, write_bytes, read_errors, read_bytes, produce_bytes,
// produce_records, fetch_bytes, fetch_records) attributed by node_id and
// topic. It covers producer and broker activity only; consumer lag is not one
// of the hooks kotel exposes.
package kafkaotel

import (
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/plugin/kotel"
	"go.opentelemetry.io/otel/metric"
)

// MeterHooks returns the kotel metric hooks bound to the global OTel meter
// provider. Instruments are created against the global delegating provider, so
// clients constructed before modules/telemetry installs the real provider are
// still exported once it is installed.
func MeterHooks() []kgo.Hook { return MeterHooksWith(nil) }

// MeterHooksWith returns the kotel metric hooks bound to provider. A nil
// provider means the global one (kotel.MeterProvider ignores nil).
func MeterHooksWith(provider metric.MeterProvider) []kgo.Hook {
	return kotel.NewKotel(kotel.WithMeter(kotel.NewMeter(kotel.MeterProvider(provider)))).Hooks()
}

// Option is the kgo option every production kgo.NewClient call adds.
func Option() kgo.Opt { return kgo.WithHooks(MeterHooks()...) }
