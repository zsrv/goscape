package kafkaotel_test

import (
	"context"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/plugin/kotel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/zsrv/goscape/pkg/kafkaotel"
)

func TestMeterHooks_ReturnsExactlyTheKotelMeter(t *testing.T) {
	hooks := kafkaotel.MeterHooks()
	if len(hooks) != 1 {
		t.Fatalf("MeterHooks() returned %d hooks, want 1", len(hooks))
	}
	if _, ok := hooks[0].(*kotel.Meter); !ok {
		t.Fatalf("MeterHooks()[0] is %T, want *kotel.Meter", hooks[0])
	}
}

func TestOption_IsAcceptedByKgo(t *testing.T) {
	cl, err := kgo.NewClient(kgo.SeedBrokers("127.0.0.1:1"), kafkaotel.Option())
	if err != nil {
		t.Fatalf("kgo.NewClient with kafkaotel.Option(): %v", err)
	}
	cl.Close()
}

func TestMeterHooksWith_RecordsBrokerConnectErrors(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	// 127.0.0.1:1 refuses immediately, so OnBrokerConnect fires with an error.
	cl, err := kgo.NewClient(
		kgo.SeedBrokers("127.0.0.1:1"),
		kgo.WithHooks(kafkaotel.MeterHooksWith(mp)...),
	)
	if err != nil {
		t.Fatalf("kgo.NewClient: %v", err)
	}
	defer cl.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_ = cl.Ping(ctx)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "messaging.kafka.connect_errors.count" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("messaging.kafka.connect_errors.count not recorded: kotel meter hooks are not installed")
	}
}
