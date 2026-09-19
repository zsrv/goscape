package packetcapture

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"

	"github.com/zsrv/goscape/pkg/dskit/services"
	"github.com/zsrv/goscape/pkg/kafkaotel"
	pkgcapture "github.com/zsrv/goscape/pkg/packetcapture"
)

// Module is the dskit service wrapper around pkg/packetcapture.Capture.
type Module struct {
	services.Service

	cfg     Config
	log     *slog.Logger
	capture *pkgcapture.Capture
	metrics *pkgcapture.Metrics

	client      *kgo.Client
	shipper     *pkgcapture.Shipper
	shipperDone chan struct{}
	shipperCtx  context.Context
	shipperStop context.CancelFunc
}

// New constructs the module after validating cfg.
// Capture (and Metrics when enabled) are constructed here so that
// initWorld() — which snapshots m.Capture() at module-init time, before
// any starting() runs — receives the real Capture rather than the noop.
func New(cfg Config, logger *slog.Logger) (*Module, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	m := &Module{cfg: cfg, log: logger}

	if !cfg.Enabled {
		m.capture = pkgcapture.NoopCapture()
	} else {
		metrics, err := pkgcapture.NewMetrics(otel.GetMeterProvider().Meter("github.com/zsrv/goscape/pkg/packetcapture"))
		if err != nil {
			return nil, fmt.Errorf("packetcapture metrics: %w", err)
		}
		m.metrics = metrics
		m.capture = pkgcapture.New(pkgcapture.CaptureOpts{
			Enabled:      true,
			WorldID:      cfg.WorldID,
			Revision:     cfg.Revision,
			RingCapacity: cfg.RingBufferSize,
			Metrics:      metrics,
		})
	}

	m.Service = services.NewBasicService(m.starting, m.running, m.stopping)
	return m, nil
}

// NewPacketCaptureService is the factory consumed by the dskit module manager.
func NewPacketCaptureService(cfg Config, logger *slog.Logger) (services.Service, error) {
	return New(cfg, logger)
}

// Capture exposes the public Tap / SessionStarted / SessionEnded API to
// modules/world. Returns a noop when capture is disabled (or before Start),
// so callers don't need a nil check.
func (m *Module) Capture() *pkgcapture.Capture {
	if m.capture == nil {
		return pkgcapture.NoopCapture()
	}
	return m.capture
}

func (m *Module) starting(ctx context.Context) error {
	if !m.cfg.Enabled {
		return nil
	}

	// LeaderAck minimises latency; idempotency adds nothing because the
	// ring buffer already drops on overflow.
	var err error
	m.client, err = kgo.NewClient(
		kgo.SeedBrokers(m.cfg.Kafka.Brokers...),
		kgo.ClientID(m.cfg.Kafka.ClientID),
		kgo.RequiredAcks(kgo.LeaderAck()),
		kgo.DisableIdempotentWrite(),
		kafkaotel.Option(),
	)
	if err != nil {
		return fmt.Errorf("packetcapture kgo.NewClient: %w", err)
	}

	m.shipper = pkgcapture.NewShipper(m.client, m.capture.Ring(),
		m.cfg.DrainInterval, m.cfg.DrainBatchMax, m.metrics,
		pkgcapture.WithStopTimeout(m.cfg.StopTimeout))

	m.shipperCtx, m.shipperStop = context.WithCancel(context.Background())
	m.shipperDone = make(chan struct{})
	go func() {
		m.shipper.Run(m.shipperCtx)
		close(m.shipperDone)
	}()

	m.log.Info("packetcapture started",
		"world_id", m.cfg.WorldID, "revision", m.cfg.Revision,
		"ring_buffer_size", m.cfg.RingBufferSize)
	return nil
}

func (m *Module) running(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (m *Module) stopping(_ error) error {
	if m.shipperStop == nil {
		return nil
	}
	m.shipperStop()
	<-m.shipperDone
	if m.client != nil {
		m.client.Close()
	}
	return nil
}

// compile-time guard
var _ services.Service = (*Module)(nil)
