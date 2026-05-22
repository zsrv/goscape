package telemetry_test

import (
	"context"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/telemetry"
)

func TestInitProviders_DisabledIsNoop(t *testing.T) {
	cfg := telemetry.Config{Enabled: false}
	shutdown, err := telemetry.InitProviders(context.Background(), cfg)
	if err != nil {
		t.Fatalf("InitProviders: %v", err)
	}
	if shutdown == nil {
		t.Fatal("shutdown is nil")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := shutdown(ctx); err != nil {
		t.Errorf("shutdown(disabled): %v", err)
	}
	if err := shutdown(ctx); err != nil {
		t.Errorf("shutdown(2nd call, disabled): %v", err)
	}
}

func TestInitProviders_EnabledReturnsShutdown(t *testing.T) {
	cfg := telemetry.Config{
		Enabled: true,
		OTLP: telemetry.OTLPConfig{
			Endpoint:    "localhost:14317",
			Insecure:    true,
			SampleRatio: 0.01,
		},
	}
	shutdown, err := telemetry.InitProviders(context.Background(), cfg)
	if err != nil {
		t.Fatalf("InitProviders(enabled): %v", err)
	}
	defer shutdown(context.Background())
	if shutdown == nil {
		t.Fatal("shutdown is nil")
	}
}
