package telemetry_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	moduletelemetry "github.com/zsrv/goscape/modules/telemetry"
	pkgtelemetry "github.com/zsrv/goscape/pkg/telemetry"
)

func TestTelemetryService_DisabledIsNoop(t *testing.T) {
	cfg := pkgtelemetry.Config{Enabled: false}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := moduletelemetry.NewTelemetryService(cfg, logger)
	if err != nil {
		t.Fatalf("NewTelemetryService: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := svc.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if err := svc.AwaitRunning(ctx); err != nil {
		t.Fatalf("AwaitRunning: %v", err)
	}
	svc.StopAsync()
	if err := svc.AwaitTerminated(ctx); err != nil {
		t.Fatalf("AwaitTerminated: %v", err)
	}
}

func TestValidate_OTLPOnlyAllowsEmptyBrokers(t *testing.T) {
	cfg := moduletelemetry.Config{
		Enabled: true,
		OTLP:    pkgtelemetry.OTLPConfig{Endpoint: "localhost:4317", Insecure: true, SampleRatio: 0.5},
	}
	if !moduletelemetry.OTLPOnly(&cfg) {
		t.Fatal("enabled with no brokers must select otlp-only mode")
	}
	if err := moduletelemetry.Validate(&cfg); err != nil {
		t.Fatalf("module Validate rejected the otlp-only shape: %v", err)
	}
	// The public Validate still rejects it — that asymmetry is exactly why the
	// module-level wrapper exists.
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected the public Config.Validate to reject empty brokers")
	}
}

func TestValidate_OTLPOnlyRequiresEndpointAndRatio(t *testing.T) {
	noEndpoint := moduletelemetry.Config{Enabled: true}
	if err := moduletelemetry.Validate(&noEndpoint); err == nil {
		t.Fatal("expected an error for otlp-only with an empty endpoint")
	}
	badRatio := moduletelemetry.Config{
		Enabled: true,
		OTLP:    pkgtelemetry.OTLPConfig{Endpoint: "localhost:4317", SampleRatio: 1.5},
	}
	if err := moduletelemetry.Validate(&badRatio); err == nil {
		t.Fatal("expected an error for a sample ratio outside [0,1]")
	}
}

func TestValidate_KafkaModeStillDelegatesToPublicValidate(t *testing.T) {
	cfg := moduletelemetry.Config{
		Enabled:        true,
		Kafka:          pkgtelemetry.KafkaConfig{Brokers: []string{"localhost:9092"}},
		RingBufferSize: 0, // invalid: the public Validate requires >= 1
	}
	if moduletelemetry.OTLPOnly(&cfg) {
		t.Fatal("brokers present must not select otlp-only mode")
	}
	if err := moduletelemetry.Validate(&cfg); err == nil {
		t.Fatal("expected the public ring-buffer validation to still apply")
	}
}

func TestTelemetryService_OTLPOnlyRegistersNoEmitter(t *testing.T) {
	pkgtelemetry.Reset()
	t.Cleanup(pkgtelemetry.Reset)

	// With nothing installed, Get() hands back the package's no-op. Keeping it
	// gives the assertion below something to compare against: a buffering
	// emitter would be a different dynamic type.
	wantNoop := pkgtelemetry.Get()

	cfg := moduletelemetry.Config{
		Enabled: true,
		// No broker is contacted in otlp-only mode; the OTLP exporters are
		// lazy gRPC clients, so no collector needs to exist for this test.
		OTLP:        pkgtelemetry.OTLPConfig{Endpoint: "127.0.0.1:14317", Insecure: true, SampleRatio: 0},
		StopTimeout: 200 * time.Millisecond,
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
	svc, err := moduletelemetry.NewTelemetryService(cfg, logger)
	if err != nil {
		t.Fatalf("NewTelemetryService: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := svc.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if err := svc.AwaitRunning(ctx); err != nil {
		t.Fatalf("AwaitRunning: %v", err)
	}

	// Anything other than the no-op means a Kafka emitter was installed.
	if got := pkgtelemetry.Get(); got != wantNoop {
		t.Fatalf("otlp-only mode must not install the buffering emitter, got %T", got)
	}

	// The mode must never be silent: an operator who set telemetry.enabled
	// expecting events on Kafka has to see that none will be shipped.
	if !strings.Contains(logs.String(), "otlp-only mode") {
		t.Fatalf("otlp-only mode logged nothing about the missing Kafka emitter; log was:\n%s", logs.String())
	}

	svc.StopAsync()
	if err := svc.AwaitTerminated(ctx); err != nil {
		t.Fatalf("AwaitTerminated: %v", err)
	}
}
