package packetcapture

import (
	"fmt"

	"go.opentelemetry.io/otel/metric"
)

// Metrics holds the eight replay-domain instruments. Names and attribute keys
// follow OpenTelemetry semantic conventions (general/naming.md, no _total
// suffix, dot-namespaced, direction as attribute).
//
// Standard messaging-semconv producer metrics (messaging.client.sent.messages,
// messaging.client.operation.duration) are emitted by the kgo client's
// kotel integration — wired separately in the dskit module, not constructed
// here. They count what the client SENT, which is why a record whose produce
// promise comes back with an error needs ProduceErrors: it appears in none of
// them.
type Metrics struct {
	PacketRecorded       metric.Int64Counter
	PacketDropped        metric.Int64Counter
	PacketIO             metric.Int64Counter
	PacketSize           metric.Int64Histogram
	SessionStarted       metric.Int64Counter
	SessionEnded         metric.Int64Counter
	ProduceErrors        metric.Int64Counter
	ShipperBatchDuration metric.Float64Histogram
}

// Attribute key constants — kept in one place so handlers don't drift.
const (
	AttrDirection          = "goscape.replay.direction"
	AttrDropReason         = "goscape.replay.drop.reason"
	AttrSessionCloseReason = "goscape.replay.session.close_reason"

	// AttrTopic is the semconv messaging.destination.name attribute carried
	// by goscape.replay.produce.errors, spelled exactly as pkg/telemetry's
	// shipper instruments spell it.
	AttrTopic = "messaging.destination.name"

	// Drop reasons carried by goscape.replay.packet.dropped. Every record the
	// pipeline loses is counted under exactly one of them. The first two are
	// charged to the PUSH that found the ring full, not to the record that was
	// evicted (which may be of either kind) — that is how the packet path has
	// read since it was written.
	//
	//	ringbuf_full                the ring buffer's drop-oldest policy made
	//	                            room for a captured packet.
	//	session_marker_ringbuf_full the same policy made room for a SESSION
	//	                            STARTED / ENDED marker. Worth its own
	//	                            value: markers are how a consumer knows a
	//	                            session's bounds, so losing one is worse
	//	                            than losing a packet.
	//	shutdown_abandoned          the shipper's stop-timeout budget expired
	//	                            with records still buffered, so the final
	//	                            drain left them behind.
	DropReasonRingbufFull              = "ringbuf_full"
	DropReasonSessionMarkerRingbufFull = "session_marker_ringbuf_full"
	DropReasonShutdownAbandoned        = "shutdown_abandoned"
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
		metric.WithDescription("Number of captured records — packets and session markers alike — dropped before reaching Kafka."),
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

	m.ProduceErrors, err = meter.Int64Counter(
		"goscape.replay.produce.errors",
		metric.WithDescription("Number of captured records whose Kafka produce callback returned an error."),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		return nil, fmt.Errorf("produce.errors: %w", err)
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
