package packetcapture

import (
	"context"
	"log/slog"
	"sync"
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

// produceWarnInterval is the floor between two produce-failure warnings. A
// broker outage fails every record in the drain batch, so an unsampled Warn
// would drown the process output. Matches pkg/telemetry.
const produceWarnInterval = 10 * time.Second

type Shipper struct {
	client      ProducerClient
	buf         *RingBuffer
	tick        time.Duration
	batchMax    int
	metrics     *Metrics
	log         *slog.Logger
	warn        *warnSampler
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
		warn:        &warnSampler{interval: produceWarnInterval, now: time.Now},
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

// onProduce is the kgo produce promise. franz-go runs it on its own goroutine,
// after the drain context may already be gone, so it always measures against
// context.Background.
//
// It used to be empty, on the claim that kotel already surfaced these. It does
// not: kotel's messaging.client.* instruments count what the client SENT, and a
// record whose promise comes back with an error was never sent — it appeared in
// no instrument and in no log. Mirrors pkg/telemetry's shipper.
func (s *Shipper) onProduce(rec *kgo.Record, err error) {
	if err == nil {
		return
	}
	topic := ""
	if rec != nil {
		topic = rec.Topic
	}
	if s.metrics != nil {
		s.metrics.ProduceErrors.Add(context.Background(), 1,
			metric.WithAttributes(attribute.String(AttrTopic, topic)))
	}
	if s.log != nil && s.warn.allow() {
		s.log.Warn("packetcapture: kafka produce failed", "topic", topic, "err", err)
	}
}

// warnSampler admits at most one event per interval. Both fields are set once
// at construction and never written again, so allow() is the only reader of
// now and needs no synchronisation around it.
//
// pkg/telemetry has the same type and does not export it, so this package
// keeps its own rather than widening that package's API for one caller; the
// two are independent by design anyway, since a broker outage should not let
// one shipper's warning suppress the other's.
type warnSampler struct {
	interval time.Duration
	now      func() time.Time

	mu   sync.Mutex
	last time.Time
}

func (s *warnSampler) allow() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if !s.last.IsZero() && now.Sub(s.last) < s.interval {
		return false
	}
	s.last = now
	return true
}
