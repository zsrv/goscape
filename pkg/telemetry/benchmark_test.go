package telemetry_test

import (
	"sort"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/zsrv/goscape/pkg/telemetry"
)

func nowNano() int64 { return time.Now().UnixNano() }

func BenchmarkEmit_BrokerHealthy(b *testing.B) {
	rb := telemetry.NewRingBuffer(b.N + 1)
	rec := &kgo.Record{Topic: "events.auth", Value: make([]byte, 128)}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Push(rec)
	}
}

func BenchmarkEmit_BufferFull(b *testing.B) {
	rb := telemetry.NewRingBuffer(1024)
	rec := &kgo.Record{Topic: "events.auth", Value: make([]byte, 128)}
	for i := 0; i < 1024; i++ {
		rb.Push(rec)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rb.Push(rec)
	}
}

// TestEmitLatency_P99 measures wall-clock latency of Push when the buffer is
// saturated and asserts p99 stays under 10 microseconds — the design's
// "world tick loop is sacred" load-bearing claim.
func TestEmitLatency_P99(t *testing.T) {
	const samples = 100_000
	rb := telemetry.NewRingBuffer(1024)
	rec := &kgo.Record{Topic: "events.auth", Value: make([]byte, 128)}
	for i := 0; i < 1024; i++ {
		rb.Push(rec)
	}

	durs := make([]int64, samples)
	for i := 0; i < samples; i++ {
		start := nowNano()
		rb.Push(rec)
		durs[i] = nowNano() - start
	}

	sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
	p99 := durs[int(float64(samples)*0.99)]
	if p99 > 10_000 {
		t.Errorf("p99 emit latency = %d ns, want <= 10000 (10us)", p99)
	}
}
