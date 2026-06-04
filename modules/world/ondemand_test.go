package world

import (
	"sync"
	"testing"
)

// testODClient is a fake odClient for testing: records sent data and tracks close calls.
type testODClientImpl struct {
	mu     sync.Mutex
	closed *bool
	sent   [][]byte
}

func testODClient(closed *bool) odClient {
	return &testODClientImpl{closed: closed}
}

func (c *testODClientImpl) send(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make([]byte, len(data))
	copy(cp, data)
	c.sent = append(c.sent, cp)
	return nil
}

func (c *testODClientImpl) close() {
	*c.closed = true
}

func TestOnDemandRequestParsing(t *testing.T) {
	od := newOnDemand(nil)
	closed := false
	// two whole requests + a partial third: urgent(archive=0,file=1,priority=2) + ingame(archive=3,file=5,priority=0) + 1 trailing byte
	buf := []byte{0, 0, 1, 2, 3, 0, 5, 0, 1}
	consumed := od.onClientData(testODClient(&closed), buf)
	if consumed != 8 {
		t.Fatalf("consumed = %d, want 8 (partial frame stays buffered at caller)", consumed)
	}
	od.mu.Lock()
	defer od.mu.Unlock()
	if len(od.urgent) != 1 || len(od.ingame) != 1 || len(od.extra) != 0 {
		t.Fatalf("queues: urgent=%d extra=%d ingame=%d, want urgent=1 extra=0 ingame=1",
			len(od.urgent), len(od.extra), len(od.ingame))
	}
	if closed {
		t.Fatal("clean requests must not close the connection")
	}
	// field decode pin: archive=g1, file=g2 big-endian, priority=g1
	if od.urgent[0].archive != 0 || od.urgent[0].file != 1 {
		t.Fatalf("urgent[0] = %+v, want archive=0 file=1", od.urgent[0])
	}
	if od.ingame[0].archive != 3 || od.ingame[0].file != 5 {
		t.Fatalf("ingame[0] = %+v, want archive=3 file=5", od.ingame[0])
	}
}

func TestOnDemandRejectsBadRequest(t *testing.T) {
	for _, bad := range [][]byte{
		{4, 0, 0, 0}, // archive > 3
		{0, 0, 0, 3}, // priority > 2
	} {
		closed := false
		od := newOnDemand(nil)
		od.onClientData(testODClient(&closed), bad)
		if !closed {
			t.Errorf("bad request %v did not close the connection", bad)
		}
	}
}

// TestOnDemandExtraPriority pins priority 1 → extra queue.
func TestOnDemandExtraPriority(t *testing.T) {
	od := newOnDemand(nil)
	closed := false
	// archive=1, file=0x0002 (big-endian: 0x00, 0x02), priority=1 → extra
	buf := []byte{1, 0, 2, 1}
	consumed := od.onClientData(testODClient(&closed), buf)
	if consumed != 4 {
		t.Fatalf("consumed = %d, want 4", consumed)
	}
	od.mu.Lock()
	defer od.mu.Unlock()
	if len(od.extra) != 1 || len(od.urgent) != 0 || len(od.ingame) != 0 {
		t.Fatalf("queues: urgent=%d extra=%d ingame=%d, want extra=1 only",
			len(od.urgent), len(od.extra), len(od.ingame))
	}
	if od.extra[0].archive != 1 || od.extra[0].file != 2 {
		t.Fatalf("extra[0] = %+v, want archive=1 file=2", od.extra[0])
	}
	if closed {
		t.Fatal("valid request must not close")
	}
}

// TestOnDemandRejectMidBufferAbandons verifies TS :60-63 return semantics:
// a bad frame closes and returns immediately, abandoning subsequent valid frames.
func TestOnDemandRejectMidBufferAbandons(t *testing.T) {
	od := newOnDemand(nil)
	closed := false
	// valid urgent(archive=0,file=1,priority=2) + bad(archive=4 invalid) + valid ingame
	buf := []byte{
		0, 0, 1, 2, // valid urgent
		4, 0, 0, 0, // invalid archive=4 → close + return
		0, 0, 2, 0, // valid ingame (must NOT be enqueued)
	}
	od.onClientData(testODClient(&closed), buf)
	if !closed {
		t.Fatal("mid-buffer bad frame must close the connection")
	}
	od.mu.Lock()
	defer od.mu.Unlock()
	// The first frame was enqueued before the bad one; the third must be abandoned.
	if len(od.urgent) != 1 {
		t.Fatalf("urgent = %d, want 1 (first valid frame before reject)", len(od.urgent))
	}
	if len(od.ingame) != 0 {
		t.Fatalf("ingame = %d, want 0 (frame after reject must be abandoned)", len(od.ingame))
	}
}

// TestOnDemandConcurrentEnqueue verifies queue mutex is race-clean.
func TestOnDemandConcurrentEnqueue(t *testing.T) {
	od := newOnDemand(nil)
	const goroutines = 20
	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			closed := false
			// alternate priorities 0/1/2 across goroutines
			priority := byte(n % 3)
			buf := []byte{byte(n % 4), 0, byte(n), priority}
			od.onClientData(testODClient(&closed), buf)
		}(i)
	}
	wg.Wait()
	od.mu.Lock()
	defer od.mu.Unlock()
	total := len(od.urgent) + len(od.extra) + len(od.ingame)
	if total != goroutines {
		t.Fatalf("total enqueued = %d, want %d", total, goroutines)
	}
}
