package telemetry_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/zsrv/goscape/pkg/telemetry"
)

type fakeClient struct {
	mu       sync.Mutex
	produced []*kgo.Record
	failNext bool
}

func (f *fakeClient) Produce(_ context.Context, rec *kgo.Record, promise func(*kgo.Record, error)) {
	f.mu.Lock()
	f.produced = append(f.produced, rec)
	fail := f.failNext
	f.failNext = false
	f.mu.Unlock()
	if promise != nil {
		if fail {
			promise(rec, context.DeadlineExceeded)
		} else {
			promise(rec, nil)
		}
	}
}

func (f *fakeClient) Flush(context.Context) error { return nil }
func (f *fakeClient) Close()                      {}

func (f *fakeClient) Snapshot() []*kgo.Record {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*kgo.Record, len(f.produced))
	copy(out, f.produced)
	return out
}

func TestShipper_DrainsBufferAtCadence(t *testing.T) {
	rb := telemetry.NewRingBuffer(100)
	for i := 0; i < 5; i++ {
		rb.Push(&kgo.Record{Topic: "t", Value: []byte{byte(i)}})
	}
	fc := &fakeClient{}
	s := telemetry.NewShipper(fc, rb, 1*time.Millisecond, 100)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(fc.Snapshot()) == 5 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	cancel()
	<-done

	if got := fc.Snapshot(); len(got) != 5 {
		t.Fatalf("produced %d records, want 5", len(got))
	}
}

func TestShipper_FinalFlushOnStop(t *testing.T) {
	rb := telemetry.NewRingBuffer(100)
	fc := &fakeClient{}
	s := telemetry.NewShipper(fc, rb, time.Hour, 100)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()

	for i := 0; i < 3; i++ {
		rb.Push(&kgo.Record{Topic: "t", Value: []byte{byte(i)}})
	}
	cancel()
	<-done

	if got := fc.Snapshot(); len(got) != 3 {
		t.Fatalf("produced %d records, want 3", len(got))
	}
}
