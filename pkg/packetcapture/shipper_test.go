package packetcapture

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// recordingHandler collects every slog record the shipper emits so a test can
// assert both the message and how many times it was logged.
type recordingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	h.records = append(h.records, r.Clone())
	h.mu.Unlock()
	return nil
}

func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler      { return h }

func (h *recordingHandler) snapshot() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]slog.Record(nil), h.records...)
}

// sumInt64ForAttr totals the data points of the named int64 counter whose
// attribute key carries want.
func sumInt64ForAttr(t *testing.T, rm *metricdata.ResourceMetrics, name, key, want string) int64 {
	t.Helper()
	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("%s: unexpected data type %T", name, m.Data)
			}
			for _, dp := range sum.DataPoints {
				if v, ok := dp.Attributes.Value(attribute.Key(key)); ok && v.AsString() == want {
					total += dp.Value
				}
			}
		}
	}
	return total
}

// sumInt64ForReason totals the named counter's points for one drop reason.
func sumInt64ForReason(t *testing.T, rm *metricdata.ResourceMetrics, name, reason string) int64 {
	t.Helper()
	return sumInt64ForAttr(t, rm, name, AttrDropReason, reason)
}

// sumInt64ForTopic totals the named counter's points for one topic.
func sumInt64ForTopic(t *testing.T, rm *metricdata.ResourceMetrics, name, topic string) int64 {
	t.Helper()
	return sumInt64ForAttr(t, rm, name, AttrTopic, topic)
}

// fakeProducer records produced records; never errors.
type fakeProducer struct {
	mu      sync.Mutex
	records []*kgo.Record
	flushed int
}

func (f *fakeProducer) Produce(_ context.Context, r *kgo.Record, promise func(*kgo.Record, error)) {
	f.mu.Lock()
	f.records = append(f.records, r)
	f.mu.Unlock()
	if promise != nil {
		promise(r, nil)
	}
}

func (f *fakeProducer) Flush(_ context.Context) error {
	f.mu.Lock()
	f.flushed++
	f.mu.Unlock()
	return nil
}

func (f *fakeProducer) Close() {}

func (f *fakeProducer) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.records)
}

func TestShipper_DrainsOnTick(t *testing.T) {
	buf := NewRingBuffer(64)
	for range 5 {
		buf.Push(&kgo.Record{Topic: KafkaTopic})
	}
	fake := &fakeProducer{}
	m, _ := NewMetrics(noop.NewMeterProvider().Meter("test"))
	s := NewShipper(fake, buf, 5*time.Millisecond, 10, m)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()

	// Wait long enough for at least one drain tick.
	time.Sleep(25 * time.Millisecond)
	cancel()
	<-done

	if got := fake.count(); got != 5 {
		t.Fatalf("produced %d, want 5", got)
	}
	if fake.flushed == 0 {
		t.Fatalf("flushed = 0, want >= 1 (final flush)")
	}
}

func TestShipper_FinalDrainOnCancel(t *testing.T) {
	buf := NewRingBuffer(64)
	buf.Push(&kgo.Record{Topic: KafkaTopic})
	fake := &fakeProducer{}
	m, _ := NewMetrics(noop.NewMeterProvider().Meter("test"))
	// Long tick so the only drain pass that runs is the final one on cancel.
	s := NewShipper(fake, buf, 1*time.Hour, 100, m)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()

	time.Sleep(10 * time.Millisecond) // let Run reach the select
	cancel()
	<-done

	if got := fake.count(); got != 1 {
		t.Fatalf("produced %d, want 1 on cancel-drain", got)
	}
}

// TestShipper_FinalDrainEmptiesTheWholeRing pins that a shutdown ships every
// buffered record, not just the first batchMax of them. The pre-existing
// coverage pushed ONE record with batchMax=100, so a final drain that popped a
// single batch looked correct; a production ring holds 65,536 slots and drains
// 1,024 at a time, so all but the first batch — including the session-ended
// markers the world pushes as it stops — used to be discarded in silence.
func TestShipper_FinalDrainEmptiesTheWholeRing(t *testing.T) {
	const total = 5000
	buf := NewRingBuffer(1 << 13)
	for range total {
		buf.Push(&kgo.Record{Topic: KafkaTopic})
	}
	fake := &fakeProducer{}
	m, _ := NewMetrics(noop.NewMeterProvider().Meter("test"))
	// Long tick so the only drain that runs is the final one on cancel.
	s := NewShipper(fake, buf, time.Hour, 1024, m)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	cancel()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return")
	}

	if got := fake.count(); got != total {
		t.Fatalf("produced %d records on the final drain, want %d", got, total)
	}
	if got := buf.Len(); got != 0 {
		t.Fatalf("ring still holds %d records after shutdown, want 0", got)
	}
}

// stallingProducer stands in for an unreachable broker: every Produce waits
// out the caller's context instead of accepting the record.
type stallingProducer struct{}

func (stallingProducer) Produce(ctx context.Context, _ *kgo.Record, promise func(*kgo.Record, error)) {
	<-ctx.Done()
	if promise != nil {
		promise(nil, ctx.Err())
	}
}

func (stallingProducer) Flush(ctx context.Context) error { return ctx.Err() }
func (stallingProducer) Close()                          {}

// TestShipper_AbandonedRecordsAreCountedAndLoggedOnce pins the other half of
// the shutdown contract: when the stop-timeout budget runs out with records
// still buffered, Run still returns inside that budget and the loss is
// reported — once in the log, and on the drop counter under its own reason.
func TestShipper_AbandonedRecordsAreCountedAndLoggedOnce(t *testing.T) {
	const (
		total    = 5000
		batchMax = 1024
	)
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })
	m, err := NewMetrics(mp.Meter("test"))
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}

	buf := NewRingBuffer(1 << 13)
	for range total {
		buf.Push(&kgo.Record{Topic: KafkaTopic})
	}

	h := &recordingHandler{}
	s := NewShipper(stallingProducer{}, buf, time.Hour, batchMax, m,
		WithStopTimeout(100*time.Millisecond), WithLogger(slog.New(h)))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	start := time.Now()
	go func() { s.Run(ctx); close(done) }()
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return: the whole-ring drain is not bounded by the stop timeout")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("Run took %v, want it bounded by the 100ms stop timeout", elapsed)
	}

	// The stalled producer consumed exactly one batch before the budget
	// expired; everything behind it stayed in the ring.
	wantAbandoned := int64(total - batchMax)
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got := sumInt64ForReason(t, &rm, "goscape.replay.packet.dropped", DropReasonShutdownAbandoned); got != wantAbandoned {
		t.Errorf("dropped[%s] = %d, want %d", DropReasonShutdownAbandoned, got, wantAbandoned)
	}

	// The stalled producer also fails every record it was handed, so filter
	// the abandonment warning out of the produce-failure ones.
	if got := countWarns(h, "abandoned"); got != 1 {
		t.Fatalf("logged %d warnings about the abandoned records, want exactly 1", got)
	}
}

// countWarns returns how many Warn records carry substr in their message.
func countWarns(h *recordingHandler, substr string) int {
	n := 0
	for _, r := range h.snapshot() {
		if r.Level == slog.LevelWarn && strings.Contains(r.Message, substr) {
			n++
		}
	}
	return n
}

// failingProducer fails every record's promise, standing in for a broker that
// rejects or never acknowledges what it is sent.
type failingProducer struct{ err error }

func (f failingProducer) Produce(_ context.Context, rec *kgo.Record, promise func(*kgo.Record, error)) {
	if promise != nil {
		promise(rec, f.err)
	}
}

func (failingProducer) Flush(context.Context) error { return nil }
func (failingProducer) Close()                      {}

// TestShipper_ProduceFailuresAreCountedAndSampled pins that a failed produce
// is accounted for. onProduce used to be empty, with a comment claiming kotel
// surfaced these: kotel counts what the client SENT, so a record whose promise
// came back with an error appeared in no instrument at all and in no log.
func TestShipper_ProduceFailuresAreCountedAndSampled(t *testing.T) {
	const total = 5

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })
	m, err := NewMetrics(mp.Meter("test"))
	if err != nil {
		t.Fatalf("NewMetrics: %v", err)
	}

	buf := NewRingBuffer(64)
	for range total {
		buf.Push(&kgo.Record{Topic: KafkaTopic})
	}

	h := &recordingHandler{}
	s := NewShipper(failingProducer{err: errors.New("broker unreachable")}, buf,
		time.Hour, 64, m, WithLogger(slog.New(h)))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	cancel()
	<-done

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got := sumInt64ForTopic(t, &rm, "goscape.replay.produce.errors", KafkaTopic); got != total {
		t.Errorf("goscape.replay.produce.errors{%s} = %d, want %d", KafkaTopic, got, total)
	}

	// A broker outage fails every record in the batch; the sampler admits at
	// most one warning per interval so the output is not drowned.
	if got := countWarns(h, "produce"); got != 1 {
		t.Fatalf("logged %d produce-failure warnings, want exactly 1 per sampler interval", got)
	}
}

// TestWarnSampler_AdmitsOnePerInterval drives the local sampler with a fake
// clock. pkg/telemetry's twin is unexported, so this package keeps its own.
func TestWarnSampler_AdmitsOnePerInterval(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	s := &warnSampler{interval: 10 * time.Second, now: func() time.Time { return now }}

	if !s.allow() {
		t.Fatal("the first event must be admitted")
	}
	if s.allow() {
		t.Fatal("a second event at the same instant must be suppressed")
	}
	now = now.Add(9 * time.Second)
	if s.allow() {
		t.Fatal("an event at t+9s must still be suppressed")
	}
	now = now.Add(2 * time.Second)
	if !s.allow() {
		t.Fatal("an event at t+11s must be admitted")
	}
}

// blockingClient blocks inside Flush until its context is done, recording the
// deadline it was handed so the test can assert the configured stop timeout
// (and not the old hard-coded 5s) bounded the final drain.
type blockingClient struct {
	mu       sync.Mutex
	deadline time.Duration
	seen     bool
}

func (b *blockingClient) Produce(context.Context, *kgo.Record, func(*kgo.Record, error)) {}

func (b *blockingClient) Flush(ctx context.Context) error {
	if dl, ok := ctx.Deadline(); ok {
		b.mu.Lock()
		b.deadline = time.Until(dl)
		b.seen = true
		b.mu.Unlock()
	}
	<-ctx.Done()
	return ctx.Err()
}

func (b *blockingClient) Close() {}

func (b *blockingClient) flushDeadline() (time.Duration, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.deadline, b.seen
}

func TestShipper_FinalDrainHonoursConfiguredStopTimeout(t *testing.T) {
	buf := NewRingBuffer(4)
	buf.Push(&kgo.Record{Topic: "events.replay_packets", Value: []byte{1}})

	fake := &blockingClient{}
	s := NewShipper(fake, buf, time.Hour, 10, nil, WithStopTimeout(50*time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return: the final drain is not bounded by the stop timeout")
	}

	dl, seen := fake.flushDeadline()
	if !seen {
		t.Fatal("Flush was called without a deadline")
	}
	if dl > time.Second {
		t.Fatalf("final flush deadline = %v, want <= the configured 50ms stop timeout", dl)
	}
}

func TestShipper_FinalDrainDefaultsToDefaultStopTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the 5s default stop timeout in real time")
	}
	buf := NewRingBuffer(4)
	fake := &blockingClient{}
	s := NewShipper(fake, buf, time.Hour, 10, nil) // no WithStopTimeout

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	cancel()

	// Flush is entered almost immediately after cancel; poll for the deadline
	// it recorded, then let it expire naturally (defaultStopTimeout is 5s).
	deadline := time.Now().Add(3 * time.Second)
	var dl time.Duration
	var seen bool
	for time.Now().Before(deadline) && !seen {
		dl, seen = fake.flushDeadline()
		time.Sleep(time.Millisecond)
	}
	if !seen {
		t.Fatal("Flush was never called with a deadline")
	}
	if dl < 3*time.Second || dl > defaultStopTimeout {
		t.Fatalf("default flush deadline = %v, want just under %v", dl, defaultStopTimeout)
	}

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after the default stop timeout expired")
	}
}
