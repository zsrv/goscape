// Package packetcapture implements the wire-tap data path: a per-process ring
// buffer of captured packet records, a shipper that drains them into Kafka,
// and the Tap / SessionStarted / SessionEnded API (the pkg/tapper seam)
// consumed by modules/world.
//
// Shape mirrors pkg/telemetry exactly (shared MPSC ring buffer + single
// drainer goroutine).
package packetcapture

import (
	"sync"
	"sync/atomic"

	"github.com/twmb/franz-go/pkg/kgo"
)

// RingBuffer is a bounded MPSC queue with drop-oldest semantics. Push is safe
// from many goroutines; PopBatch is for a single drainer.
type RingBuffer struct {
	mu      sync.Mutex
	buf     []*kgo.Record
	head    int // next read index
	tail    int // next write index
	count   int
	cap     int
	dropped atomic.Uint64
}

func NewRingBuffer(capacity int) *RingBuffer {
	if capacity < 1 {
		capacity = 1
	}
	return &RingBuffer{buf: make([]*kgo.Record, capacity), cap: capacity}
}

// Push always succeeds. Returns true iff it dropped the oldest record.
func (r *RingBuffer) Push(rec *kgo.Record) (dropped bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.count == r.cap {
		r.head = (r.head + 1) % r.cap
		r.count--
		r.dropped.Add(1)
		dropped = true
	}
	r.buf[r.tail] = rec
	r.tail = (r.tail + 1) % r.cap
	r.count++
	return dropped
}

// PopBatch removes up to max records, preserving FIFO order. Returns nil
// when the buffer is empty.
func (r *RingBuffer) PopBatch(max int) []*kgo.Record {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.count == 0 || max < 1 {
		return nil
	}
	n := min(r.count, max)
	out := make([]*kgo.Record, n)
	for i := range n {
		out[i] = r.buf[r.head]
		r.buf[r.head] = nil
		r.head = (r.head + 1) % r.cap
	}
	r.count -= n
	return out
}

func (r *RingBuffer) DroppedTotal() uint64 {
	return r.dropped.Load()
}

func (r *RingBuffer) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count
}
