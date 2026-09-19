package telemetry_test

import (
	"context"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/zsrv/goscape/pkg/telemetry"
)

// sumInt64 totals every data point of the named int64 monotonic-sum
// instrument (counters and observable counters both land here).
func sumInt64(t *testing.T, rm *metricdata.ResourceMetrics, name string) int64 {
	t.Helper()
	var total int64
	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			found = true
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("%s: unexpected data type %T", name, m.Data)
			}
			for _, dp := range sum.DataPoints {
				total += dp.Value
			}
		}
	}
	if !found {
		t.Fatalf("instrument %q was not collected", name)
	}
	return total
}

func TestShipperMetrics_CountsShippedDroppedAndProduceErrors(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	// Capacity 2 with three pushes forces exactly one drop-oldest.
	rb := telemetry.NewRingBuffer(2)
	m, err := telemetry.NewMetrics(mp.Meter("test"), rb)
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })

	for i := range 3 {
		rb.Push(&kgo.Record{Topic: "events.auth", Value: []byte{byte(i)}})
	}

	// fakeClient (shipper_test.go) fails exactly the first record, then acks.
	fc := &fakeClient{failNext: true}
	s := telemetry.NewShipper(fc, rb, time.Millisecond, 10, telemetry.WithMetrics(m))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && len(fc.Snapshot()) < 2 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done

	if got := len(fc.Snapshot()); got != 2 {
		t.Fatalf("produced %d records, want 2", got)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got := sumInt64(t, &rm, "goscape.telemetry.produce_errors"); got != 1 {
		t.Errorf("goscape.telemetry.produce_errors = %d, want 1", got)
	}
	if got := sumInt64(t, &rm, "goscape.telemetry.shipped_total"); got != 1 {
		t.Errorf("goscape.telemetry.shipped_total = %d, want 1", got)
	}
	if got := sumInt64(t, &rm, "goscape.telemetry.ringbuf_dropped_total"); got != 1 {
		t.Errorf("goscape.telemetry.ringbuf_dropped_total = %d, want 1", got)
	}
}

func TestShipperMetrics_TopicAttributeIsSet(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	rb := telemetry.NewRingBuffer(4)
	m, err := telemetry.NewMetrics(mp.Meter("test"), rb)
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })

	rb.Push(&kgo.Record{Topic: "events.wealth", Value: []byte{1}})

	fc := &fakeClient{}
	s := telemetry.NewShipper(fc, rb, time.Millisecond, 10, telemetry.WithMetrics(m))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && len(fc.Snapshot()) < 1 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	gotTopic := ""
	for _, sm := range rm.ScopeMetrics {
		for _, mm := range sm.Metrics {
			if mm.Name != "goscape.telemetry.shipped_total" {
				continue
			}
			for _, dp := range mm.Data.(metricdata.Sum[int64]).DataPoints {
				if v, ok := dp.Attributes.Value(telemetry.AttrTopic); ok {
					gotTopic = v.AsString()
				}
			}
		}
	}
	if gotTopic != "events.wealth" {
		t.Fatalf("%s attribute = %q, want %q", telemetry.AttrTopic, gotTopic, "events.wealth")
	}
}

// TestMetricNames_AreTheDashboardContract pins the LITERAL instrument and
// attribute names this package publishes. Dashboards and alert rules query
// their Prometheus translations (goscape_telemetry_produce_errors_total,
// goscape_telemetry_shipped_total, goscape_telemetry_ringbuf_dropped_total);
// a rename here empties those panels and rules silently, because PromQL over
// a missing series is not an error.
func TestMetricNames_AreTheDashboardContract(t *testing.T) {
	if telemetry.AttrTopic != "messaging.destination.name" {
		t.Errorf("AttrTopic = %q, want %q (the dashboard legends key off it)",
			telemetry.AttrTopic, "messaging.destination.name")
	}

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	rb := telemetry.NewRingBuffer(2)
	m, err := telemetry.NewMetrics(mp.Meter("test"), rb)
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })

	// A counter with no measurements is never exported, so seed both at zero.
	ctx := context.Background()
	m.ProduceErrors.Add(ctx, 0)
	m.Shipped.Add(ctx, 0)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	for _, name := range []string{
		"goscape.telemetry.produce_errors",        // -> goscape_telemetry_produce_errors_total
		"goscape.telemetry.shipped_total",         // -> goscape_telemetry_shipped_total
		"goscape.telemetry.ringbuf_dropped_total", // -> goscape_telemetry_ringbuf_dropped_total
	} {
		if !collected(&rm, name) {
			t.Errorf("instrument %q is not published under that exact name", name)
		}
	}
}

// collected reports whether an instrument of that exact name was exported.
func collected(rm *metricdata.ResourceMetrics, name string) bool {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return true
			}
		}
	}
	return false
}
