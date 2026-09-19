package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// AttrTopic is the semconv messaging.destination.name attribute key carried by
// the shipper instruments.
const AttrTopic = "messaging.destination.name"

// Metrics holds the telemetry shipper's OTel instruments.
//
// Prometheus names after the OTLP translation:
//
//	goscape.telemetry.produce_errors        -> goscape_telemetry_produce_errors_total
//	goscape.telemetry.shipped_total         -> goscape_telemetry_shipped_total
//	goscape.telemetry.ringbuf_dropped_total -> goscape_telemetry_ringbuf_dropped_total
type Metrics struct {
	ProduceErrors metric.Int64Counter
	Shipped       metric.Int64Counter

	reg metric.Registration
}

// NewMetrics registers the shipper instruments on meter. The ring-buffer drop
// instrument is an observable *counter*, not a gauge: RingBuffer.DroppedTotal
// is cumulative and monotonic, and a counter is what makes rate()/increase()
// valid on the Prometheus side (the Prometheus name is identical either way).
func NewMetrics(meter metric.Meter, buf *RingBuffer) (*Metrics, error) {
	var m Metrics
	var err error

	m.ProduceErrors, err = meter.Int64Counter(
		"goscape.telemetry.produce_errors",
		metric.WithUnit("{error}"),
		metric.WithDescription("Telemetry records whose Kafka produce callback returned an error."),
	)
	if err != nil {
		return nil, fmt.Errorf("produce_errors: %w", err)
	}

	m.Shipped, err = meter.Int64Counter(
		"goscape.telemetry.shipped_total",
		metric.WithUnit("{record}"),
		metric.WithDescription("Telemetry records acknowledged by Kafka."),
	)
	if err != nil {
		return nil, fmt.Errorf("shipped_total: %w", err)
	}

	dropped, err := meter.Int64ObservableCounter(
		"goscape.telemetry.ringbuf_dropped_total",
		metric.WithUnit("{record}"),
		metric.WithDescription("Telemetry records discarded by the ring buffer's drop-oldest policy."),
	)
	if err != nil {
		return nil, fmt.Errorf("ringbuf_dropped_total: %w", err)
	}
	m.reg, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		o.ObserveInt64(dropped, int64(buf.DroppedTotal()))
		return nil
	}, dropped)
	if err != nil {
		return nil, fmt.Errorf("ringbuf_dropped_total callback: %w", err)
	}

	return &m, nil
}

// Close unregisters the observable callback so the ring buffer can be
// collected. Safe on a nil receiver.
func (m *Metrics) Close() error {
	if m == nil || m.reg == nil {
		return nil
	}
	return m.reg.Unregister()
}

func topicAttr(topic string) metric.MeasurementOption {
	return metric.WithAttributes(attribute.String(AttrTopic, topic))
}
