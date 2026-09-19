package telemetry

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"

	"github.com/zsrv/goscape/pkg/dskit/services"
	"github.com/zsrv/goscape/pkg/kafkaotel"
	pkgtelemetry "github.com/zsrv/goscape/pkg/telemetry"
)

// defaultStopTimeout is the shutdown budget used when the config carries no
// positive StopTimeout. Matches the telemetry.stop-timeout flag default.
const defaultStopTimeout = 5 * time.Second

type Telemetry struct {
	services.Service

	cfg Config
	log *slog.Logger

	client       *kgo.Client
	shipper      *pkgtelemetry.Shipper
	metrics      *pkgtelemetry.Metrics
	shipperDone  chan struct{}
	shipperCtx   context.Context
	shipperStop  context.CancelFunc
	otelShutdown func(context.Context) error
}

func New(cfg Config, logger *slog.Logger) (*Telemetry, error) {
	if err := Validate(&cfg); err != nil {
		return nil, err
	}
	t := &Telemetry{cfg: cfg, log: logger}
	t.Service = services.NewBasicService(t.starting, t.running, t.stopping)
	return t, nil
}

// NewTelemetryService is the factory used by the dskit module manager.
func NewTelemetryService(cfg Config, logger *slog.Logger) (services.Service, error) {
	return New(cfg, logger)
}

func (t *Telemetry) starting(ctx context.Context) error {
	shutdown, err := pkgtelemetry.InitProviders(ctx, t.cfg)
	if err != nil {
		return fmt.Errorf("init OTel: %w", err)
	}
	t.otelShutdown = shutdown

	// otlp-only: providers are up, but no Kafka client, no ring buffer, and no
	// Emitter registration. Also covers the fully-disabled case.
	if !t.cfg.Enabled {
		return nil
	}
	if OTLPOnly(&t.cfg) {
		// Never silent: this shape ships metrics/traces/logs but NO game
		// events, so the operator must see which mode the module chose.
		t.log.Info("telemetry: otlp-only mode — no Kafka emitter registered",
			"otlp_endpoint", t.cfg.OTLP.Endpoint)
		return nil
	}

	cli, err := kgo.NewClient(
		kgo.SeedBrokers(t.cfg.Kafka.Brokers...),
		kgo.ClientID(t.cfg.Kafka.ClientID),
		kgo.RecordPartitioner(kgo.StickyKeyPartitioner(nil)),
		// LeaderAck minimises shipper latency; drop-oldest at the ring buffer
		// already accepts loss, so idempotency's at-least-once guarantee adds
		// nothing here. franz-go requires acks=all when idempotency is on, so
		// disable it explicitly.
		kgo.RequiredAcks(kgo.LeaderAck()),
		kgo.DisableIdempotentWrite(),
		kgo.MaxBufferedRecords(t.cfg.RingBufferSize),
		kafkaotel.Option(),
	)
	if err != nil {
		return fmt.Errorf("kgo.NewClient: %w", err)
	}
	t.client = cli

	buf := pkgtelemetry.NewRingBuffer(t.cfg.RingBufferSize)
	metrics, err := pkgtelemetry.NewMetrics(
		otel.GetMeterProvider().Meter("github.com/zsrv/goscape/pkg/telemetry"),
		buf)
	if err != nil {
		cli.Close()
		return fmt.Errorf("telemetry metrics: %w", err)
	}
	t.metrics = metrics
	t.shipper = pkgtelemetry.NewShipper(cli, buf, t.cfg.DrainInterval, t.cfg.DrainBatchMax,
		pkgtelemetry.WithMetrics(metrics), pkgtelemetry.WithLogger(t.log),
		pkgtelemetry.WithStopTimeout(t.cfg.StopTimeout))

	emitter := pkgtelemetry.NewEmitter(buf)
	pkgtelemetry.Set(emitter)

	t.shipperCtx, t.shipperStop = context.WithCancel(context.Background())
	t.shipperDone = make(chan struct{})
	go func() { t.shipper.Run(t.shipperCtx); close(t.shipperDone) }()

	return nil
}

func (t *Telemetry) running(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (t *Telemetry) stopping(_ error) error {
	pkgtelemetry.Reset()
	if t.shipperStop != nil {
		t.shipperStop()
		<-t.shipperDone
	}
	if t.metrics != nil {
		_ = t.metrics.Close()
	}
	if t.client != nil {
		t.client.Close()
	}
	if t.otelShutdown != nil {
		// Clamp a non-positive StopTimeout (a struct-literal config that never
		// went through the flag defaults) up to the 5s default: without it
		// WithTimeout hands otelShutdown an already-expired context and every
		// clean stop warns "context deadline exceeded".
		budget := cmp.Or(max(t.cfg.StopTimeout, 0), defaultStopTimeout)
		ctx, cancel := context.WithTimeout(context.Background(), budget)
		defer cancel()
		if err := t.otelShutdown(ctx); err != nil {
			// Best effort: an unreachable collector at shutdown must not put
			// the module into Failed. The dskit manager would log "module
			// failed" and the binary would exit non-zero on an otherwise
			// clean stop.
			t.log.Warn("telemetry: OTel provider shutdown", "err", err)
		}
	}
	return nil
}
