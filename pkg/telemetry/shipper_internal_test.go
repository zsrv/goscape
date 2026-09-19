package telemetry

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// TestWarnSampler_AdmitsOnePerInterval pins the 1-per-10s produce-error log
// sampler: a broker outage fails every record in a drain batch, so an
// unsampled Warn would drown the process output.
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

// blockingClient records the deadline of the context its Flush is called with
// and then waits for that context to expire, standing in for an unreachable
// broker at shutdown.
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

// TestShipper_FinalDrainHonoursConfiguredStopTimeout pins that
// telemetry.stop_timeout actually bounds the final flush, which is what the
// flag's help text promises. Mirrors pkg/packetcapture's twin.
func TestShipper_FinalDrainHonoursConfiguredStopTimeout(t *testing.T) {
	buf := NewRingBuffer(4)
	buf.Push(&kgo.Record{Topic: "events.auth", Value: []byte{1}})

	fake := &blockingClient{}
	s := NewShipper(fake, buf, time.Hour, 10, WithStopTimeout(50*time.Millisecond))

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

// TestShipper_FinalDrainDefaultsToDefaultStopTimeout covers the call sites
// that pass no option (and a non-positive configured value): the 5s default
// still bounds the drain.
func TestShipper_FinalDrainDefaultsToDefaultStopTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the 5s default stop timeout in real time")
	}
	buf := NewRingBuffer(4)
	fake := &blockingClient{}
	s := NewShipper(fake, buf, time.Hour, 10, WithStopTimeout(0)) // non-positive: keeps the default

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
