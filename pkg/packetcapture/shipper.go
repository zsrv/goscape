package packetcapture

import (
	"context"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
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

// Run blocks until ctx is cancelled. On cancel, it makes one final drain pass
// using a fresh context bounded by the configured stop timeout (the parent ctx
// is already cancelled at that point), so a slow or unreachable broker cannot
// stall shutdown.
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
	start := time.Now()
	recs := s.buf.PopBatch(s.batchMax)
	for _, rec := range recs {
		s.client.Produce(ctx, rec, s.onProduce)
	}
	if s.metrics != nil {
		s.metrics.ShipperBatchDuration.Record(ctx, time.Since(start).Seconds())
	}
}

func (s *Shipper) onProduce(_ *kgo.Record, _ error) {
	// Kafka failures are surfaced via the standard messaging.client.* metrics
	// from kotel; this callback exists to satisfy the kgo.Produce signature.
}
