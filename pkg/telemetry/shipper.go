package telemetry

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// ProducerClient is the slice of *kgo.Client the shipper depends on.
// Defining it as an interface keeps the shipper testable with a fake client.
type ProducerClient interface {
	Produce(ctx context.Context, rec *kgo.Record, promise func(*kgo.Record, error))
	Flush(ctx context.Context) error
	Close()
}

type Shipper struct {
	client   ProducerClient
	buf      *RingBuffer
	tick     time.Duration
	batchMax int

	metrics     *Metrics
	log         *slog.Logger
	warn        *warnSampler
	stopTimeout time.Duration
}

// ShipperOpt configures optional Shipper behaviour. Options are variadic so
// the pre-existing four-argument NewShipper call sites keep compiling.
type ShipperOpt func(*Shipper)

// WithMetrics installs the OTel instruments the shipper feeds.
func WithMetrics(m *Metrics) ShipperOpt { return func(s *Shipper) { s.metrics = m } }

// WithLogger installs the logger used for sampled produce-error warnings.
// Without it the shipper stays silent (metrics still count every failure).
func WithLogger(l *slog.Logger) ShipperOpt { return func(s *Shipper) { s.log = l } }

// WithStopTimeout bounds the final drain + flush performed after Run's context
// is cancelled. Fed from telemetry.stop_timeout, whose flag help promises
// exactly this; a non-positive value leaves defaultStopTimeout in place.
func WithStopTimeout(d time.Duration) ShipperOpt {
	return func(s *Shipper) {
		if d > 0 {
			s.stopTimeout = d
		}
	}
}

func NewShipper(client ProducerClient, buf *RingBuffer, tick time.Duration, batchMax int, opts ...ShipperOpt) *Shipper {
	s := &Shipper{
		client:      client,
		buf:         buf,
		tick:        tick,
		batchMax:    batchMax,
		warn:        &warnSampler{interval: produceWarnInterval, now: time.Now},
		stopTimeout: defaultStopTimeout,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// defaultStopTimeout bounds the post-cancel drain + flush when the caller
// configures no explicit stop timeout, so a slow / unreachable Kafka broker
// cannot stall shutdown indefinitely. Matches pkg/packetcapture.
const defaultStopTimeout = 5 * time.Second

// produceWarnInterval is the floor between two produce-error warnings. A broker
// outage fails every record in the drain batch, so an unsampled Warn would
// drown the process output.
const produceWarnInterval = 10 * time.Second

// Run blocks until ctx is cancelled. On cancel, it makes one final drain pass
// and Flushes the underlying client. Because ctx itself is already cancelled
// at that point, the final-drain + flush uses a fresh context.Background with
// a bounded timeout (the configured stop timeout) — otherwise a
// slow/unresponsive Kafka broker would block shutdown forever.
func (s *Shipper) Run(ctx context.Context) {
	t := time.NewTicker(s.tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			drainCtx, cancel := context.WithTimeout(context.Background(), s.stopTimeout)
			s.drainOnce(drainCtx)
			_ = s.client.Flush(drainCtx)
			cancel()
			return
		case <-t.C:
			s.drainOnce(ctx)
		}
	}
}

func (s *Shipper) drainOnce(ctx context.Context) {
	recs := s.buf.PopBatch(s.batchMax)
	for _, rec := range recs {
		s.client.Produce(ctx, rec, s.onProduce)
	}
}

// onProduce is the kgo produce promise. It runs on a franz-go goroutine after
// the drain context may already be gone, so it always measures against
// context.Background.
func (s *Shipper) onProduce(rec *kgo.Record, err error) {
	topic := ""
	if rec != nil {
		topic = rec.Topic
	}
	if err == nil {
		if s.metrics != nil {
			s.metrics.Shipped.Add(context.Background(), 1, topicAttr(topic))
		}
		return
	}
	if s.metrics != nil {
		s.metrics.ProduceErrors.Add(context.Background(), 1, topicAttr(topic))
	}
	if s.log != nil && s.warn.allow() {
		s.log.Warn("telemetry: kafka produce failed", "topic", topic, "err", err)
	}
}

// warnSampler admits at most one event per interval. Both fields are set once
// at construction and never written again, so allow() is the only reader of
// now and needs no synchronisation around it; the in-package sampler test
// builds a warnSampler directly to drive a fake clock.
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
