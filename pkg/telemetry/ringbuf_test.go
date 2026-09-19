package telemetry_test

import (
	"strconv"
	"sync"
	"testing"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/zsrv/goscape/pkg/telemetry"
)

func mkRec(i int) *kgo.Record {
	return &kgo.Record{Topic: "t", Value: []byte(strconv.Itoa(i))}
}

func TestRingBuffer_PushPopWithinCapacity(t *testing.T) {
	rb := telemetry.NewRingBuffer(8)
	for i := 0; i < 5; i++ {
		if dropped := rb.Push(mkRec(i)); dropped {
			t.Fatalf("unexpected drop at i=%d", i)
		}
	}
	got := rb.PopBatch(10)
	if len(got) != 5 {
		t.Fatalf("PopBatch len = %d, want 5", len(got))
	}
	for i, r := range got {
		if string(r.Value) != strconv.Itoa(i) {
			t.Errorf("got[%d].Value = %q, want %q", i, r.Value, strconv.Itoa(i))
		}
	}
	if rb.DroppedTotal() != 0 {
		t.Errorf("DroppedTotal = %d, want 0", rb.DroppedTotal())
	}
}

func TestRingBuffer_DropOldestOnOverflow(t *testing.T) {
	rb := telemetry.NewRingBuffer(3)
	for i := 0; i < 5; i++ {
		rb.Push(mkRec(i))
	}
	got := rb.PopBatch(10)
	if len(got) != 3 {
		t.Fatalf("PopBatch len = %d, want 3", len(got))
	}
	for i, want := range []string{"2", "3", "4"} {
		if string(got[i].Value) != want {
			t.Errorf("got[%d].Value = %q, want %q", i, got[i].Value, want)
		}
	}
	if rb.DroppedTotal() != 2 {
		t.Errorf("DroppedTotal = %d, want 2", rb.DroppedTotal())
	}
}

func TestRingBuffer_ConcurrentPushSingleDrain(t *testing.T) {
	const writers = 16
	const perWriter = 100
	rb := telemetry.NewRingBuffer(writers * perWriter)

	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				rb.Push(mkRec(i))
			}
		}()
	}
	wg.Wait()

	got := rb.PopBatch(writers * perWriter)
	if len(got) != writers*perWriter {
		t.Errorf("got %d records, want %d", len(got), writers*perWriter)
	}
	if rb.DroppedTotal() != 0 {
		t.Errorf("DroppedTotal = %d, want 0", rb.DroppedTotal())
	}
}

func TestRingBuffer_Len(t *testing.T) {
	rb := telemetry.NewRingBuffer(4)
	if rb.Len() != 0 {
		t.Errorf("empty Len = %d, want 0", rb.Len())
	}
	rb.Push(mkRec(1))
	rb.Push(mkRec(2))
	if rb.Len() != 2 {
		t.Errorf("after 2 pushes Len = %d, want 2", rb.Len())
	}
	rb.PopBatch(1)
	if rb.Len() != 1 {
		t.Errorf("after 1 pop Len = %d, want 1", rb.Len())
	}
}
