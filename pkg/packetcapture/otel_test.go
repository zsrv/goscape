package packetcapture

import (
	"testing"

	"go.opentelemetry.io/otel/metric/noop"
)

func TestNewMetricsAllInstrumentsConstructed(t *testing.T) {
	m, err := NewMetrics(noop.NewMeterProvider().Meter("test"))
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}
	if m == nil {
		t.Fatal("NewMetrics returned nil")
	}
	if m.PacketRecorded == nil || m.PacketDropped == nil || m.PacketIO == nil ||
		m.PacketSize == nil || m.SessionStarted == nil || m.SessionEnded == nil ||
		m.ProduceErrors == nil || m.ShipperBatchDuration == nil {
		t.Fatal("one or more instruments unset")
	}
}
