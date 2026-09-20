package packetcapture

import (
	"fmt"

	"go.opentelemetry.io/otel/metric"
)

// Metrics holds the seven replay-domain instruments. Names and attribute keys
// follow OpenTelemetry semantic conventions (general/naming.md, no _total
// suffix, dot-namespaced, direction as attribute).
//
// Standard messaging-semconv producer metrics (messaging.client.sent.messages,
// messaging.client.operation.duration) are emitted by the kgo client's
// kotel integration — wired separately in the dskit module, not constructed
// here.
type Metrics struct {
	PacketRecorded       metric.Int64Counter
	PacketDropped        metric.Int64Counter
	PacketIO             metric.Int64Counter
	PacketSize           metric.Int64Histogram
	SessionStarted       metric.Int64Counter
	SessionEnded         metric.Int64Counter
	ShipperBatchDuration metric.Float64Histogram
}

// Attribute key constants — kept in one place so handlers don't drift.
const (
	AttrDirection          = "goscape.replay.direction"
	AttrDropReason         = "goscape.replay.drop.reason"
	AttrSessionCloseReason = "goscape.replay.session.close_reason"

	// Drop reasons carried by goscape.replay.packet.dropped. Every record the
	// pipeline loses is counted under exactly one of them.
	//
	//	ringbuf_full        the ring buffer's drop-oldest policy evicted the
	//	                    oldest record to make room for a new one.
	//	shutdown_abandoned  the shipper's stop-timeout budget expired with
	//	                    records still buffered, so the final drain left
	//	                    them behind.
	DropReasonRingbufFull       = "ringbuf_full"
	DropReasonShutdownAbandoned = "shutdown_abandoned"
)

func NewMetrics(meter metric.Meter) (*Metrics, error) {
	var m Metrics
	var err error

	m.PacketRecorded, err = meter.Int64Counter(
		"goscape.replay.packet.recorded",
		metric.WithDescription("Number of game packets captured into the replay pipeline."),
		metric.WithUnit("{packet}"),
	)
	if err != nil {
		return nil, fmt.Errorf("packet.recorded: %w", err)
	}

	m.PacketDropped, err = meter.Int64Counter(
		"goscape.replay.packet.dropped",
		metric.WithDescription("Number of captured packets dropped before reaching Kafka."),
		metric.WithUnit("{packet}"),
	)
	if err != nil {
		return nil, fmt.Errorf("packet.dropped: %w", err)
	}

	m.PacketIO, err = meter.Int64Counter(
		"goscape.replay.packet.io",
		metric.WithDescription("Total bytes of packet payloads captured."),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, fmt.Errorf("packet.io: %w", err)
	}

	m.PacketSize, err = meter.Int64Histogram(
		"goscape.replay.packet.size",
		metric.WithDescription("Distribution of captured packet payload sizes."),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, fmt.Errorf("packet.size: %w", err)
	}

	m.SessionStarted, err = meter.Int64Counter(
		"goscape.replay.session.started",
		metric.WithDescription("Number of replay sessions opened."),
		metric.WithUnit("{session}"),
	)
	if err != nil {
		return nil, fmt.Errorf("session.started: %w", err)
	}

	m.SessionEnded, err = meter.Int64Counter(
		"goscape.replay.session.ended",
		metric.WithDescription("Number of replay sessions closed."),
		metric.WithUnit("{session}"),
	)
	if err != nil {
		return nil, fmt.Errorf("session.ended: %w", err)
	}

	m.ShipperBatchDuration, err = meter.Float64Histogram(
		"goscape.replay.shipper.batch.duration",
		metric.WithDescription("Duration of the packetcapture shipper's per-tick drain + flush."),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("shipper.batch.duration: %w", err)
	}

	return &m, nil
}
