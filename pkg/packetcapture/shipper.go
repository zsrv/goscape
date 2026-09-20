package packetcapture

import (
	"context"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// ProducerClient is the slice of *kgo.Client the shipper depends on.
// Defining it as an interface keeps the shipper testable.
type ProducerClient interface {
	Produce(ctx context.Context, rec *kgo.Record, promise func(*kgo.Record, error))
	Flush(ctx context.Context) error
	Close()
}

// defaultStopTimeout bounds the post-cancel drain + flush when the caller
// configures no explicit StopTimeout. Matches pkg/telemetry.
const defaultStopTimeout = 5 * time.Second

type Shipper struct {
	client      ProducerClient
	buf         *RingBuffer
	tick        time.Duration
	batchMax    int
	metrics     *Metrics
	log         *slog.Logger
	stopTimeout time.Duration
}

// ShipperOpt configures optional Shipper behaviour. Variadic so the existing
// five-argument NewShipper call sites keep compiling.
type ShipperOpt func(*Shipper)

// WithStopTimeout bounds the final drain + flush performed after Run's context
// is cancelled. Fed from packetcapture.stop_timeout; a non-positive value
// leaves defaultStopTimeout in place.
func WithStopTimeout(d time.Duration) ShipperOpt {
	return func(s *Shipper) {
		if d > 0 {
			s.stopTimeout = d
		}
	}
}

// WithLogger installs the logger the shipper reports losses on. Without it it
// stays silent and the counters carry the whole story.
func WithLogger(l *slog.Logger) ShipperOpt {
	return func(s *Shipper) { s.log = l }
}

func NewShipper(client ProducerClient, buf *RingBuffer, tick time.Duration, batchMax int, m *Metrics, opts ...ShipperOpt) *Shipper {
	s := &Shipper{
		client:      client,
		buf:         buf,
		tick:        tick,
		batchMax:    batchMax,
		metrics:     m,
		stopTimeout: defaultStopTimeout,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Run blocks until ctx is cancelled. On cancel, it drains the WHOLE ring and
// then flushes, using a fresh context bounded by the configured stop timeout
// (the parent ctx is already cancelled at that point) so a slow or unreachable
// broker cannot stall shutdown. Whatever that budget does not cover is
// reported rather than dropped in silence.
func (s *Shipper) Run(ctx context.Context) {
	t := time.NewTicker(s.tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			drainCtx, cancel := context.WithTimeout(context.Background(), s.stopTimeout)
			s.drainAll(drainCtx)
			_ = s.client.Flush(drainCtx)
			cancel()
			s.reportAbandoned()
			return
		case <-t.C:
			s.drainOnce(ctx)
		}
	}
}

// drainAll empties the ring one batchMax-sized pass at a time, until either
// the buffer runs dry or ctx's budget expires. One pass moves at most batchMax
// records, so a single-pass final drain abandoned everything behind the first
// batch — with the shipped defaults, up to 64,512 of a 65,536-slot ring,
// including the session-ended markers the world pushes for every live
// connection while it stops.
func (s *Shipper) drainAll(ctx context.Context) {
	for ctx.Err() == nil && s.drainOnce(ctx) > 0 {
	}
}

// drainOnce pops one batch and hands it to the producer, returning how many
// records it moved (0 means the ring is empty).
func (s *Shipper) drainOnce(ctx context.Context) int {
	start := time.Now()
	recs := s.buf.PopBatch(s.batchMax)
	for _, rec := range recs {
		s.client.Produce(ctx, rec, s.onProduce)
	}
	if s.metrics != nil {
		s.metrics.ShipperBatchDuration.Record(ctx, time.Since(start).Seconds())
	}
	return len(recs)
}

// reportAbandoned accounts for whatever is still buffered once the stop
// timeout has expired: one Warn, and one addition to the drop counter under a
// reason of its own, so a shutdown that outran its budget is visible to an
// operator instead of being a silent hole in the capture. Runs after the
// bounded drain has finished, so it measures the final, settled remainder.
func (s *Shipper) reportAbandoned() {
	rem := s.buf.Len()
	if rem == 0 {
		return
	}
	if s.metrics != nil {
		// The drain context is spent by now; the counter takes a live one.
		s.metrics.PacketDropped.Add(context.Background(), int64(rem),
			metric.WithAttributes(attribute.String(AttrDropReason, DropReasonShutdownAbandoned)))
	}
	if s.log != nil {
		s.log.Warn("packetcapture: stop timeout expired before the buffer drained; records abandoned",
			"records", rem, "stop_timeout", s.stopTimeout)
	}
}

func (s *Shipper) onProduce(_ *kgo.Record, _ error) {
	// Kafka failures are surfaced via the standard messaging.client.* metrics
	// from kotel; this callback exists to satisfy the kgo.Produce signature.
}
