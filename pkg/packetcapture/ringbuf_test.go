package packetcapture

import (
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"
)

func TestRingBuffer_PushPop(t *testing.T) {
	r := NewRingBuffer(4)
	r.Push(&kgo.Record{Key: []byte("a")})
	r.Push(&kgo.Record{Key: []byte("b")})
	r.Push(&kgo.Record{Key: []byte("c")})

	out := r.PopBatch(10)
	if len(out) != 3 {
		t.Fatalf("len(out)=%d, want 3", len(out))
	}
	if string(out[0].Key) != "a" || string(out[1].Key) != "b" || string(out[2].Key) != "c" {
		t.Fatalf("FIFO order broken: %q %q %q", out[0].Key, out[1].Key, out[2].Key)
	}
	if got := r.DroppedTotal(); got != 0 {
		t.Fatalf("DroppedTotal=%d, want 0", got)
	}
}

func TestRingBuffer_DropOldestWhenFull(t *testing.T) {
	r := NewRingBuffer(2)
	r.Push(&kgo.Record{Key: []byte("a")})
	r.Push(&kgo.Record{Key: []byte("b")})
	r.Push(&kgo.Record{Key: []byte("c")}) // overflows; "a" is dropped

	out := r.PopBatch(10)
	if len(out) != 2 {
		t.Fatalf("len(out)=%d, want 2", len(out))
	}
	if string(out[0].Key) != "b" || string(out[1].Key) != "c" {
		t.Fatalf("FIFO-after-drop order broken: %q %q", out[0].Key, out[1].Key)
	}
	if got := r.DroppedTotal(); got != 1 {
		t.Fatalf("DroppedTotal=%d, want 1", got)
	}
}

func TestRingBuffer_PopBatchRespectsMax(t *testing.T) {
	r := NewRingBuffer(8)
	for range 5 {
		r.Push(&kgo.Record{})
	}
	out := r.PopBatch(2)
	if len(out) != 2 {
		t.Fatalf("len(out)=%d, want 2", len(out))
	}
	if r.Len() != 3 {
		t.Fatalf("Len=%d, want 3", r.Len())
	}
}

func TestRingBuffer_PopEmpty(t *testing.T) {
	r := NewRingBuffer(4)
	if out := r.PopBatch(10); out != nil {
		t.Fatalf("PopBatch on empty buffer = %v, want nil", out)
	}
}
