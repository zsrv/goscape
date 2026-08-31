package world

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/io/filestream"
)

// testODClient is a fake odClient: records sent data and tracks close calls.
// firstSend (when non-nil) is closed on the first send — a race-free signal
// for tests that wait on the async run loop instead of polling.
type testODClient struct {
	mu        sync.Mutex
	closed    *bool
	sent      [][]byte
	firstSend chan struct{}
	// blocked is what backlogged() reports — the fake's stand-in for an
	// outbound queue past its soft high-water mark.
	blocked bool
}

func newTestODClient(closed *bool) *testODClient {
	return &testODClient{closed: closed}
}

func (c *testODClient) send(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make([]byte, len(data))
	copy(cp, data)
	c.sent = append(c.sent, cp)
	if c.firstSend != nil && len(c.sent) == 1 {
		close(c.firstSend)
	}
	return nil
}

func (c *testODClient) close() {
	*c.closed = true
}

// backlogged reports the fake's outbound-backpressure state (SEC1 M-2 /
// DEVIATION SEC1-D3). Defaults to false; setBacklogged flips it.
func (c *testODClient) backlogged() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.blocked
}

func (c *testODClient) setBacklogged(v bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.blocked = v
}

// frames returns a snapshot of everything sent so far.
func (c *testODClient) frames() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([][]byte(nil), c.sent...)
}

func TestOnDemandRequestParsing(t *testing.T) {
	od := newOnDemand(nil)
	closed := false
	// two whole requests + a partial third: urgent(archive=0,file=1,priority=2) + ingame(archive=3,file=5,priority=0) + 1 trailing byte
	buf := []byte{0, 0, 1, 2, 3, 0, 5, 0, 1}
	consumed := od.onClientData(newTestODClient(&closed), buf)
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
		od.onClientData(newTestODClient(&closed), bad)
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
	consumed := od.onClientData(newTestODClient(&closed), buf)
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
	od.onClientData(newTestODClient(&closed), buf)
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
			od.onClientData(newTestODClient(&closed), buf)
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

// ---------------------------------------------------------------------------
// Send + cycle tests (Task 17: OnDemand.ts:18-40 cycle, :87-120 send)
// ---------------------------------------------------------------------------

// makeODFS returns a filestream written to a temp dir with createNew=true.
// archive in TS terms is 0-based; filestream.Write uses the raw archive
// index, so TS archive 0 → Write(1, file, …) because TS send calls
// cache.read(archive+1, file).
func makeODFS(t *testing.T) *filestream.FileStream {
	t.Helper()
	return filestream.New(t.TempDir(), true, false)
}

// header6 builds the 6-byte OnDemand frame header:
//
//	p1(archive), p2(file), p2(totalLen), p1(part)
//
// Mirrors OnDemand.ts:100-104.
func header6(archive, file, totalLen, part int) []byte {
	return []byte{
		byte(archive),
		byte(file >> 8), byte(file),
		byte(totalLen >> 8), byte(totalLen),
		byte(part),
	}
}

// TestOnDemandSendRejection pins the size=0 rejection frame for a missing file.
// TS OnDemand.ts:112-118: if !req → send 6 zero-payload bytes.
func TestOnDemandSendRejection(t *testing.T) {
	fs := makeODFS(t)
	od := newOnDemand(fs)
	closed := false
	c := newTestODClient(&closed)

	// Enqueue archive=0, file=99 (not written → cache.Read returns nil).
	od.mu.Lock()
	od.urgent = append(od.urgent, odRequest{client: c, archive: 0, file: 99})
	od.mu.Unlock()

	od.cycle()

	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.sent) != 1 {
		t.Fatalf("rejection: want 1 frame sent, got %d", len(c.sent))
	}
	want := header6(0, 99, 0, 0)
	if !bytes.Equal(c.sent[0], want) {
		t.Fatalf("rejection frame = %v, want %v", c.sent[0], want)
	}
}

// TestOnDemandSendSingleFrame pins a payload ≤500 bytes → exactly 1 frame.
// TS OnDemand.ts:94-110: while pos < req.length loop, 500-byte cap.
func TestOnDemandSendSingleFrame(t *testing.T) {
	fs := makeODFS(t)
	payload := bytes.Repeat([]byte{0xAB}, 1) // 1 byte → 1 frame
	fs.Write(1, 7, payload, 0)               // TS archive 0 → fs index 1
	od := newOnDemand(fs)
	closed := false
	c := newTestODClient(&closed)

	od.mu.Lock()
	od.urgent = append(od.urgent, odRequest{client: c, archive: 0, file: 7})
	od.mu.Unlock()
	od.cycle()

	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.sent) != 1 {
		t.Fatalf("1-byte payload: want 1 frame, got %d", len(c.sent))
	}
	want := append(header6(0, 7, 1, 0), 0xAB)
	if !bytes.Equal(c.sent[0], want) {
		t.Fatalf("1-byte frame = %v, want %v", c.sent[0], want)
	}
}

// TestOnDemandSendExactlyFiveHundred pins 500 bytes → 1 frame (boundary).
func TestOnDemandSendExactlyFiveHundred(t *testing.T) {
	fs := makeODFS(t)
	payload := bytes.Repeat([]byte{0xCC}, 500)
	fs.Write(1, 3, payload, 0)
	od := newOnDemand(fs)
	closed := false
	c := newTestODClient(&closed)

	od.mu.Lock()
	od.urgent = append(od.urgent, odRequest{client: c, archive: 0, file: 3})
	od.mu.Unlock()
	od.cycle()

	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.sent) != 1 {
		t.Fatalf("500-byte payload: want 1 frame, got %d", len(c.sent))
	}
	wantHdr := header6(0, 3, 500, 0)
	if !bytes.Equal(c.sent[0][:6], wantHdr) {
		t.Fatalf("500-byte frame header = %v, want %v", c.sent[0][:6], wantHdr)
	}
	if len(c.sent[0]) != 506 {
		t.Fatalf("500-byte frame total len = %d, want 506", len(c.sent[0]))
	}
}

// TestOnDemandSendFiveOhOneSplitsTwo pins 501 bytes → 2 frames (500 + 1).
// TS OnDemand.ts:96-98: if remaining > 500 { remaining = 500 }.
func TestOnDemandSendFiveOhOneSplitsTwo(t *testing.T) {
	fs := makeODFS(t)
	payload := bytes.Repeat([]byte{0xAB}, 501)
	fs.Write(1, 7, payload, 0) // TS archive 0 → fs index 1
	od := newOnDemand(fs)
	closed := false
	c := newTestODClient(&closed)

	od.mu.Lock()
	od.urgent = append(od.urgent, odRequest{client: c, archive: 0, file: 7})
	od.mu.Unlock()
	od.cycle()

	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.sent) != 2 {
		t.Fatalf("501-byte payload: want 2 frames, got %d", len(c.sent))
	}
	// Frame 0: hdr(archive=0,file=7,totalLen=501,part=0) + 500 bytes
	// p2(501) = 0x01 0xF5
	wantHdr0 := header6(0, 7, 501, 0)
	if !bytes.Equal(c.sent[0][:6], wantHdr0) {
		t.Fatalf("frame0 header = %v, want %v", c.sent[0][:6], wantHdr0)
	}
	if len(c.sent[0]) != 506 {
		t.Fatalf("frame0 len = %d, want 506", len(c.sent[0]))
	}
	// Frame 1: hdr(archive=0,file=7,totalLen=501,part=1) + 1 byte
	wantHdr1 := header6(0, 7, 501, 1)
	if !bytes.Equal(c.sent[1][:6], wantHdr1) {
		t.Fatalf("frame1 header = %v, want %v", c.sent[1][:6], wantHdr1)
	}
	if len(c.sent[1]) != 7 {
		t.Fatalf("frame1 len = %d, want 7", len(c.sent[1]))
	}
}

// TestOnDemandSendThousandBytesTwoFrames pins 1000 bytes → 2 frames of 500.
func TestOnDemandSendThousandBytesTwoFrames(t *testing.T) {
	fs := makeODFS(t)
	payload := bytes.Repeat([]byte{0xDD}, 1000)
	fs.Write(1, 2, payload, 0)
	od := newOnDemand(fs)
	closed := false
	c := newTestODClient(&closed)

	od.mu.Lock()
	od.urgent = append(od.urgent, odRequest{client: c, archive: 0, file: 2})
	od.mu.Unlock()
	od.cycle()

	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.sent) != 2 {
		t.Fatalf("1000-byte payload: want 2 frames, got %d", len(c.sent))
	}
	if len(c.sent[0]) != 506 || len(c.sent[1]) != 506 {
		t.Fatalf("1000-byte frames: got lens %d,%d, want 506,506",
			len(c.sent[0]), len(c.sent[1]))
	}
	// part numbers
	if c.sent[0][5] != 0 {
		t.Fatalf("frame0 part = %d, want 0", c.sent[0][5])
	}
	if c.sent[1][5] != 1 {
		t.Fatalf("frame1 part = %d, want 1", c.sent[1][5])
	}
}

// TestOnDemandCycleDrainOrder pins the urgent → extra → ingame FIFO drain
// order from OnDemand.ts:18-37. All requests are for missing files (→ each
// triggers a single 6-byte rejection frame), so we can assert send order by
// inspecting the archive field of each sent frame.
func TestOnDemandCycleDrainOrder(t *testing.T) {
	fs := makeODFS(t)
	od := newOnDemand(fs)

	closed := false
	c := newTestODClient(&closed)

	// Enqueue in reverse order: ingame first, then extra, then urgent.
	// After cycle() drains urgent→extra→ingame, the send order must be
	// archive 2 (urgent), 1 (extra), 0 (ingame).
	od.mu.Lock()
	od.ingame = append(od.ingame, odRequest{client: c, archive: 0, file: 99})
	od.extra = append(od.extra, odRequest{client: c, archive: 1, file: 99})
	od.urgent = append(od.urgent, odRequest{client: c, archive: 2, file: 99})
	od.mu.Unlock()

	od.cycle()

	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.sent) != 3 {
		t.Fatalf("drain order: want 3 frames, got %d", len(c.sent))
	}
	// Each frame is a 6-byte rejection; byte 0 is archive.
	wantOrder := []byte{2, 1, 0} // urgent, extra, ingame
	for i, want := range wantOrder {
		if c.sent[i][0] != want {
			t.Errorf("frame[%d] archive = %d, want %d (drain order wrong)", i, c.sent[i][0], want)
		}
	}
}

// TestOnDemandCycleClearsQueues pins that after cycle() all three queues are empty.
func TestOnDemandCycleClearsQueues(t *testing.T) {
	fs := makeODFS(t)
	od := newOnDemand(fs)
	closed := false
	c := newTestODClient(&closed)

	od.mu.Lock()
	od.urgent = append(od.urgent, odRequest{client: c, archive: 0, file: 99})
	od.extra = append(od.extra, odRequest{client: c, archive: 1, file: 99})
	od.ingame = append(od.ingame, odRequest{client: c, archive: 2, file: 99})
	od.mu.Unlock()

	od.cycle()

	od.mu.Lock()
	defer od.mu.Unlock()
	if len(od.urgent)+len(od.extra)+len(od.ingame) != 0 {
		t.Fatalf("queues not drained after cycle: urgent=%d extra=%d ingame=%d",
			len(od.urgent), len(od.extra), len(od.ingame))
	}
}

// TestOnDemandRunLoopLifecycle verifies that run() drains a pending request
// within a few 50ms cycles, then stops cleanly when the stop channel is
// closed. No goroutine leak (verified by the race detector + test timeout).
func TestOnDemandRunLoopLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("lifecycle test sleeps ~150ms")
	}
	fs := makeODFS(t)
	od := newOnDemand(fs)
	closed := false
	c := newTestODClient(&closed)
	c.firstSend = make(chan struct{})

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		od.run(stop)
	})

	// Enqueue a request (missing file → rejection frame).
	od.mu.Lock()
	od.urgent = append(od.urgent, odRequest{client: c, archive: 0, file: 99})
	od.mu.Unlock()

	// Wait for the run loop's cycle (50ms cadence) to drain the request —
	// channel signal, not polling, so a loaded CI scheduler can't flake it.
	select {
	case <-c.firstSend:
	case <-time.After(5 * time.Second):
		t.Fatal("run loop never drained the request within 5s")
	}

	// Stop the loop and verify it exits.
	close(stop)
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("run loop did not exit within 500ms after stop")
	}
}

// TestOnDemandCycleYieldsWhenBacklogged pins DEVIATION SEC1-D3's producer
// side on this branch's 50ms-cycle scheduler: a client whose outbound queue
// is past its soft high-water mark is skipped, not served and not closed, and
// its requests stay queued in priority order for a later cycle. Once the queue
// drains, the very same pending requests flow normally.
func TestOnDemandCycleYieldsWhenBacklogged(t *testing.T) {
	const fileLen = 1200 // 3 chunks each (500 + 500 + 200)
	fs := makeODFS(t)
	for f := range 3 {
		fs.Write(1, f, bytes.Repeat([]byte{byte(f + 1)}, fileLen), 0) // TS archive 0 → fs index 1
	}
	od := newOnDemand(fs)
	closed := false
	c := newTestODClient(&closed)
	c.setBacklogged(true)

	od.mu.Lock()
	od.urgent = append(od.urgent, odRequest{client: c, archive: 0, file: 0})
	od.extra = append(od.extra, odRequest{client: c, archive: 0, file: 1})
	od.ingame = append(od.ingame, odRequest{client: c, archive: 0, file: 2})
	od.mu.Unlock()

	od.cycle()

	if n := len(c.frames()); n != 0 {
		t.Fatalf("cycle sent %d frames to a backlogged client, want 0", n)
	}
	if closed {
		t.Fatal("cycle closed a backlogged client; backpressure must not disconnect")
	}
	od.mu.Lock()
	gotU, gotE, gotI := len(od.urgent), len(od.extra), len(od.ingame)
	od.mu.Unlock()
	if gotU != 1 || gotE != 1 || gotI != 1 {
		t.Fatalf("deferred queues: urgent=%d extra=%d ingame=%d, want 1/1/1 (requests must be untouched)",
			gotU, gotE, gotI)
	}

	// Peer caught up: the same requests are now served, in priority order.
	c.setBacklogged(false)
	od.cycle()

	frames := c.frames()
	if len(frames) != 9 {
		t.Fatalf("after the backlog cleared: %d frames, want 9", len(frames))
	}
	if want := header6(0, 0, fileLen, 0); !bytes.Equal(frames[0][:6], want) {
		t.Fatalf("first frame header = %v, want %v", frames[0][:6], want)
	}
	// urgent (file 0) → extra (file 1) → ingame (file 2).
	for i, wantFile := range []int{0, 1, 2} {
		if got := int(frames[i*3][1])<<8 | int(frames[i*3][2]); got != wantFile {
			t.Errorf("batch %d starts with file %d, want %d (priority order lost)", i, got, wantFile)
		}
	}
	if closed {
		t.Fatal("client closed while being served normally")
	}
	od.mu.Lock()
	defer od.mu.Unlock()
	if len(od.urgent)+len(od.extra)+len(od.ingame) != 0 {
		t.Fatal("queues not drained after the backlog cleared")
	}
}
