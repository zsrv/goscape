package packetcapture

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/metric/noop"
)

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
