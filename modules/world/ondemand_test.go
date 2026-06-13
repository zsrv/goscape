package world

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/io/filestream"
)

// testODClient is a fake odClient: records sent data, tracks close calls, and
// returns a stable id(). firstSend (when non-nil) is closed on the first send —
// a race-free signal for tests that wait on the async pump instead of polling.
type testODClient struct {
	mu        sync.Mutex
	clientID  string
	closed    *bool
	sent      [][]byte
	firstSend chan struct{}
}

func newTestODClient(closed *bool) *testODClient {
	return &testODClient{clientID: "client-default", closed: closed}
}

func newTestODClientID(id string, closed *bool) *testODClient {
	return &testODClient{clientID: id, closed: closed}
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

func (c *testODClient) close() { *c.closed = true }

func (c *testODClient) id() string { return c.clientID }

func (c *testODClient) frames() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([][]byte, len(c.sent))
	copy(out, c.sent)
	return out
}

// pendingCount returns the live pending count for the client (under od.mu).
func (od *onDemand) pendingCountFor(t *testing.T, id string) int {
	t.Helper()
	od.mu.Lock()
	defer od.mu.Unlock()
	cq, ok := od.clients[id]
	if !ok {
		return 0
	}
	return cq.pendingCount
}

// queueLens returns the three priority-queue lengths for the client (under od.mu).
func (od *onDemand) queueLens(t *testing.T, id string) (p0, p1, p2 int) {
	t.Helper()
	od.mu.Lock()
	defer od.mu.Unlock()
	cq, ok := od.clients[id]
	if !ok {
		return 0, 0, 0
	}
	return len(cq.queues[0]), len(cq.queues[1]), len(cq.queues[2])
}

// hasClient reports whether a clientQueue exists for id (under od.mu).
func (od *onDemand) hasClient(id string) bool {
	od.mu.Lock()
	defer od.mu.Unlock()
	_, ok := od.clients[id]
	return ok
}

// inRoundRobin reports whether id appears in the round-robin (under od.mu).
func (od *onDemand) inRoundRobin(id string) bool {
	od.mu.Lock()
	defer od.mu.Unlock()
	for _, r := range od.roundRobin {
		if r == id {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Request parsing (OnDemandThread request frame; byte-identical to 254)
// ---------------------------------------------------------------------------

// TestOnDemandRequestParsing pins the 4-byte frame decode (archive=g1,
// file=g2 big-endian, priority=g1), the consumed contract (partial frame
// stays buffered), and that the requests land in the right per-client
// priority queues.
func TestOnDemandRequestParsing(t *testing.T) {
	od := newOnDemand(nil)
	closed := false
	c := newTestODClient(&closed)
	// two whole requests + a partial third:
	//   urgent(archive=0,file=1,priority=2) + ingame(archive=3,file=5,priority=0) + 1 trailing byte
	buf := []byte{0, 0, 1, 2, 3, 0, 5, 0, 1}
	consumed := od.onClientData(c, buf)
	if consumed != 8 {
		t.Fatalf("consumed = %d, want 8 (partial frame stays buffered at caller)", consumed)
	}
	if closed {
		t.Fatal("clean requests must not close the connection")
	}
	p0, p1, p2 := od.queueLens(t, c.id())
	if p2 != 1 || p1 != 0 || p0 != 1 {
		t.Fatalf("queues: p0=%d p1=%d p2=%d, want p0=1 p1=0 p2=1", p0, p1, p2)
	}
	if od.pendingCountFor(t, c.id()) != 2 {
		t.Fatalf("pendingCount = %d, want 2", od.pendingCountFor(t, c.id()))
	}
}

// TestOnDemandRejectsBadRequest pins archive>3 / priority>2 → close.
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

// TestOnDemandClosesOnClientKeepalive pins that we MATCH the pristine TS
// reference (Engine-TS OnDemand.ts onClientData) for the client's no-timeout
// keepalive frame [0,0,0,10]: priority 10 > 2 is rejected → connection closed,
// with no out-of-range on the [3]-wide priority queues (the guard returns before
// queues[priority]). The keepalive is deliberately NOT special-cased here —
// doing so would diverge from the reference engine. The goscape-client recovers
// from the close on its next frame (its F2 send-retry cadence), so this is the
// intended server behavior, not a bug to "fix".
func TestOnDemandClosesOnClientKeepalive(t *testing.T) {
	closed := false
	od := newOnDemand(nil)
	od.onClientData(newTestODClient(&closed), []byte{0, 0, 0, 10})
	if !closed {
		t.Fatal("client keepalive [0,0,0,10] must close the connection, matching the pristine TS reference")
	}
}

// TestOnDemandExtraPriority pins priority 1 → queues[1].
func TestOnDemandExtraPriority(t *testing.T) {
	od := newOnDemand(nil)
	closed := false
	c := newTestODClient(&closed)
	// archive=1, file=0x0002, priority=1 → extra (queues[1])
	consumed := od.onClientData(c, []byte{1, 0, 2, 1})
	if consumed != 4 {
		t.Fatalf("consumed = %d, want 4", consumed)
	}
	p0, p1, p2 := od.queueLens(t, c.id())
	if p1 != 1 || p0 != 0 || p2 != 0 {
		t.Fatalf("queues: p0=%d p1=%d p2=%d, want p1=1 only", p0, p1, p2)
	}
	if closed {
		t.Fatal("valid request must not close")
	}
}

// TestOnDemandRejectMidBufferAbandons verifies a bad frame closes and returns
// immediately, abandoning subsequent valid frames (TS enqueue closeClient is
// terminal for the parse loop).
func TestOnDemandRejectMidBufferAbandons(t *testing.T) {
	od := newOnDemand(nil)
	closed := false
	c := newTestODClient(&closed)
	buf := []byte{
		0, 0, 1, 2, // valid urgent
		4, 0, 0, 0, // invalid archive=4 → close + return
		0, 0, 2, 0, // valid ingame (must NOT be enqueued)
	}
	od.onClientData(c, buf)
	if !closed {
		t.Fatal("mid-buffer bad frame must close the connection")
	}
	p0, _, p2 := od.queueLens(t, c.id())
	if p2 != 1 {
		t.Fatalf("p2 = %d, want 1 (first valid frame before reject)", p2)
	}
	if p0 != 0 {
		t.Fatalf("p0 = %d, want 0 (frame after reject must be abandoned)", p0)
	}
}

// TestOnDemandConcurrentEnqueue verifies the queue mutex is race-clean across
// many concurrent clients (run with -race).
func TestOnDemandConcurrentEnqueue(t *testing.T) {
	od := newOnDemand(nil)
	const goroutines = 20
	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			closed := false
			c := newTestODClientID(fmt.Sprintf("client-%d", n), &closed)
			priority := byte(n % 3)
			od.onClientData(c, []byte{byte(n % 4), 0, byte(n), priority})
		}(i)
	}
	wg.Wait()
	total := 0
	od.mu.Lock()
	for _, cq := range od.clients {
		total += cq.pendingCount
	}
	od.mu.Unlock()
	if total != goroutines {
		t.Fatalf("total enqueued = %d, want %d", total, goroutines)
	}
}

// ---------------------------------------------------------------------------
// Chunk format + rejection (byte-identical to 254 — MUST stay stable)
// ---------------------------------------------------------------------------

func makeODFS(t *testing.T) *filestream.FileStream {
	t.Helper()
	return filestream.New(t.TempDir(), true, false)
}

// header6 builds the 6-byte OnDemand chunk header:
//
//	p1(archive), p2(file), p2(totalLen), p1(part)
func header6(archive, file, totalLen, part int) []byte {
	return []byte{
		byte(archive),
		byte(file >> 8), byte(file),
		byte(totalLen >> 8), byte(totalLen),
		byte(part),
	}
}

// enqueueOne enqueues a single request and serves the client synchronously
// (no pump goroutine). Mirrors what a single pump pass would do for one
// already-scheduled client — but serveClient bounds to one slice, so callers
// that need >16 chunks / >8000 bytes must call serveClient again.
func enqueueAndServe(t *testing.T, od *onDemand, c odClient, archive, file, priority int) {
	t.Helper()
	od.mu.Lock()
	ok := od.enqueue(c, archive, file, priority)
	cq := od.clients[c.id()]
	od.mu.Unlock()
	if !ok {
		t.Fatalf("enqueue(%d,%d,%d) returned not-ok", archive, file, priority)
	}
	od.serveClient(cq)
}

// TestOnDemandChunkRejection pins the size=0 rejection frame for a missing file.
func TestOnDemandChunkRejection(t *testing.T) {
	od := newOnDemand(makeODFS(t))
	closed := false
	c := newTestODClient(&closed)
	enqueueAndServe(t, od, c, 0, 99, 2) // archive=0, file=99 not written → nil

	frames := c.frames()
	if len(frames) != 1 {
		t.Fatalf("rejection: want 1 frame sent, got %d", len(frames))
	}
	want := header6(0, 99, 0, 0)
	if !bytes.Equal(frames[0], want) {
		t.Fatalf("rejection frame = %v, want %v", frames[0], want)
	}
}

// TestOnDemandChunkSingleFrame pins a 1-byte payload → exactly 1 frame.
func TestOnDemandChunkSingleFrame(t *testing.T) {
	fs := makeODFS(t)
	fs.Write(1, 7, []byte{0xAB}, 0) // TS archive 0 → fs index 1
	od := newOnDemand(fs)
	closed := false
	c := newTestODClient(&closed)
	enqueueAndServe(t, od, c, 0, 7, 2)

	frames := c.frames()
	if len(frames) != 1 {
		t.Fatalf("1-byte payload: want 1 frame, got %d", len(frames))
	}
	want := append(header6(0, 7, 1, 0), 0xAB)
	if !bytes.Equal(frames[0], want) {
		t.Fatalf("1-byte frame = %v, want %v", frames[0], want)
	}
}

// TestOnDemandChunkFiveHundred pins 500 bytes → 1 frame (boundary).
func TestOnDemandChunkFiveHundred(t *testing.T) {
	fs := makeODFS(t)
	fs.Write(1, 3, bytes.Repeat([]byte{0xCC}, 500), 0)
	od := newOnDemand(fs)
	closed := false
	c := newTestODClient(&closed)
	enqueueAndServe(t, od, c, 0, 3, 2)

	frames := c.frames()
	if len(frames) != 1 {
		t.Fatalf("500-byte payload: want 1 frame, got %d", len(frames))
	}
	if !bytes.Equal(frames[0][:6], header6(0, 3, 500, 0)) {
		t.Fatalf("500-byte frame header = %v, want %v", frames[0][:6], header6(0, 3, 500, 0))
	}
	if len(frames[0]) != 506 {
		t.Fatalf("500-byte frame total len = %d, want 506", len(frames[0]))
	}
}

// TestOnDemandChunkFiveOhOneSplitsTwo pins 501 bytes → 2 frames (500 + 1).
func TestOnDemandChunkFiveOhOneSplitsTwo(t *testing.T) {
	fs := makeODFS(t)
	fs.Write(1, 7, bytes.Repeat([]byte{0xAB}, 501), 0)
	od := newOnDemand(fs)
	closed := false
	c := newTestODClient(&closed)
	enqueueAndServe(t, od, c, 0, 7, 2)

	frames := c.frames()
	if len(frames) != 2 {
		t.Fatalf("501-byte payload: want 2 frames, got %d", len(frames))
	}
	if !bytes.Equal(frames[0][:6], header6(0, 7, 501, 0)) {
		t.Fatalf("frame0 header = %v, want %v", frames[0][:6], header6(0, 7, 501, 0))
	}
	if len(frames[0]) != 506 {
		t.Fatalf("frame0 len = %d, want 506", len(frames[0]))
	}
	if !bytes.Equal(frames[1][:6], header6(0, 7, 501, 1)) {
		t.Fatalf("frame1 header = %v, want %v", frames[1][:6], header6(0, 7, 501, 1))
	}
	if len(frames[1]) != 7 {
		t.Fatalf("frame1 len = %d, want 7", len(frames[1]))
	}
}

// ---------------------------------------------------------------------------
// 274 scheduling semantics
// ---------------------------------------------------------------------------

// TestOnDemandImmediatePump verifies that enqueuing via onClientData and
// running the pump (driven by run()) delivers the file promptly — no 50ms
// wait. The test signals via firstSend (closed on the first send).
func TestOnDemandImmediatePump(t *testing.T) {
	fs := makeODFS(t)
	fs.Write(1, 7, []byte{0xAB}, 0)
	od := newOnDemand(fs)
	closed := false
	c := newTestODClient(&closed)
	c.firstSend = make(chan struct{})

	stop := make(chan interface{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); od.run(stop) }()
	t.Cleanup(func() { close(stop); wg.Wait() })

	od.onClientData(c, []byte{0, 0, 7, 2}) // archive=0 file=7 priority=2

	select {
	case <-c.firstSend:
	case <-time.After(2 * time.Second):
		t.Fatal("pump never delivered the request within 2s")
	}
	frames := c.frames()
	if len(frames) != 1 {
		t.Fatalf("want 1 frame, got %d", len(frames))
	}
	if !bytes.Equal(frames[0], append(header6(0, 7, 1, 0), 0xAB)) {
		t.Fatalf("frame = %v", frames[0])
	}
}

// TestOnDemandPriorityPreemption verifies that an urgent (priority 2) request
// enqueued AFTER several ingame (priority 0) requests is served BEFORE the
// remaining ingame files. The active file (if any) completes first, then the
// urgent jumps ahead of the queued ingame files.
func TestOnDemandPriorityPreemption(t *testing.T) {
	fs := makeODFS(t)
	// Each file is a single 1-byte payload → 1 frame; the frame's archive byte
	// (frame[0]) lets us read off the serve order.
	fs.Write(1, 10, []byte{0x10}, 0) // archive 0
	fs.Write(2, 11, []byte{0x11}, 0) // archive 1
	fs.Write(3, 12, []byte{0x12}, 0) // archive 2 (the urgent one)
	od := newOnDemand(fs)
	closed := false
	c := newTestODClient(&closed)

	od.mu.Lock()
	od.enqueue(c, 0, 10, 0) // ingame
	od.enqueue(c, 1, 11, 0) // ingame
	od.enqueue(c, 2, 12, 2) // urgent — must jump ahead of the second ingame
	cq := od.clients[c.id()]
	od.mu.Unlock()

	// Drain everything (one client; loop serveClient until no work remains).
	for {
		od.serveClient(cq)
		od.mu.Lock()
		more := od.hasWork(cq)
		od.mu.Unlock()
		if !more {
			break
		}
	}

	frames := c.frames()
	if len(frames) != 3 {
		t.Fatalf("want 3 frames, got %d", len(frames))
	}
	// First served must be the urgent (archive byte 2), then the two ingame in
	// FIFO order (archive 0 then 1).
	gotOrder := []byte{frames[0][0], frames[1][0], frames[2][0]}
	wantOrder := []byte{2, 0, 1}
	if !bytes.Equal(gotOrder, wantOrder) {
		t.Fatalf("serve order (archive bytes) = %v, want %v (urgent first)", gotOrder, wantOrder)
	}
}

// TestOnDemandSliceThrottleRoundRobin verifies serveClient bounds one client
// to ≤16 chunks per slice, then the pump yields to the next client — so two
// clients each requesting a large multi-chunk file interleave rather than
// A-fully-then-B.
func TestOnDemandSliceThrottleRoundRobin(t *testing.T) {
	fs := makeODFS(t)
	// 20 chunks worth (20*500 = 10000 bytes) → exceeds both the 16-chunk and
	// 8000-byte slice caps, so one slice can't finish the file.
	big := bytes.Repeat([]byte{0xEE}, 20*500)
	fs.Write(1, 1, big, 0) // archive 0
	od := newOnDemand(fs)
	closedA, closedB := false, false
	a := newTestODClientID("A", &closedA)
	b := newTestODClientID("B", &closedB)

	od.mu.Lock()
	od.enqueue(a, 0, 1, 2)
	od.enqueue(b, 0, 1, 2)
	cqA := od.clients["A"]
	cqB := od.clients["B"]
	od.mu.Unlock()

	// One slice each.
	od.serveClient(cqA)
	od.serveClient(cqB)

	// The byte cap (8000) is hit before the chunk cap (16): 16 chunks would be
	// 8 frames*500... actually 16 chunks = up to 8000+ bytes. The byte guard
	// stops at the first chunk that would cross 8000: 16*500=8000 reached after
	// 16 chunks, but bytes includes the 6-byte header per frame, so the byte
	// cap trips first. Assert neither finished AND each got a bounded count.
	fa := a.frames()
	fb := b.frames()
	if len(fa) == 0 || len(fb) == 0 {
		t.Fatalf("both clients must get at least one chunk in the first round: A=%d B=%d", len(fa), len(fb))
	}
	if len(fa) > maxChunksPerClientSlice || len(fb) > maxChunksPerClientSlice {
		t.Fatalf("slice exceeded %d-chunk cap: A=%d B=%d", maxChunksPerClientSlice, len(fa), len(fb))
	}
	// Neither finished (file is 20 chunks; one slice can't deliver all).
	if len(fa) >= 20 || len(fb) >= 20 {
		t.Fatalf("a single slice delivered the whole file: A=%d B=%d (no throttle)", len(fa), len(fb))
	}
	// Each must still have work (active response mid-file).
	od.mu.Lock()
	if !od.hasWork(cqA) || !od.hasWork(cqB) {
		od.mu.Unlock()
		t.Fatal("both clients should still have an in-progress response after one slice")
	}
	od.mu.Unlock()
}

// TestOnDemandDedupSamePriority verifies enqueuing the same (archive,file) twice
// at the same priority coalesces to one pending request.
func TestOnDemandDedupSamePriority(t *testing.T) {
	od := newOnDemand(nil)
	closed := false
	c := newTestODClient(&closed)
	od.mu.Lock()
	od.enqueue(c, 0, 5, 1)
	od.enqueue(c, 0, 5, 1) // duplicate key, same priority → ignored
	od.mu.Unlock()
	if got := od.pendingCountFor(t, c.id()); got != 1 {
		t.Fatalf("pendingCount = %d, want 1 (dedup)", got)
	}
}

// TestOnDemandDedupPriorityUpgrade verifies that enqueuing key K at priority 0
// then K at priority 2 cancels the low-priority pending and serves at priority 2
// (so it jumps ahead of other priority-0 work).
func TestOnDemandDedupPriorityUpgrade(t *testing.T) {
	fs := makeODFS(t)
	fs.Write(1, 5, []byte{0x55}, 0) // archive 0, file 5 (the upgraded key)
	fs.Write(1, 6, []byte{0x66}, 0) // archive 0, file 6 (another ingame)
	od := newOnDemand(fs)
	closed := false
	c := newTestODClient(&closed)

	od.mu.Lock()
	od.enqueue(c, 0, 5, 0) // K at priority 0
	od.enqueue(c, 0, 6, 0) // another ingame, queued after K-at-0
	od.enqueue(c, 0, 5, 2) // K upgraded to priority 2 → cancels K-at-0, requeues at 2
	cq := od.clients[c.id()]
	od.mu.Unlock()

	// pendingCount: K(once) + file6 = 2 (the cancelled K-at-0 was decremented).
	if got := od.pendingCountFor(t, c.id()); got != 2 {
		t.Fatalf("pendingCount = %d, want 2 (cancelled low-pri not double-counted)", got)
	}

	for {
		od.serveClient(cq)
		od.mu.Lock()
		more := od.hasWork(cq)
		od.mu.Unlock()
		if !more {
			break
		}
	}

	frames := c.frames()
	if len(frames) != 2 {
		t.Fatalf("want 2 frames (K + file6), got %d", len(frames))
	}
	// First served must be the upgraded K (file=5), not the FIFO-earlier... well
	// K was enqueued first, but the cancelled-then-requeued K is now priority 2,
	// which serves before file6 (priority 0). file byte is frames[i][1..2].
	gotFile0 := int(frames[0][1])<<8 | int(frames[0][2])
	if gotFile0 != 5 {
		t.Fatalf("first served file = %d, want 5 (upgraded key jumps ahead)", gotFile0)
	}
}

// TestOnDemandActiveKeyIgnore verifies that enqueuing the same key as the
// currently-active (in-progress) response is ignored.
func TestOnDemandActiveKeyIgnore(t *testing.T) {
	fs := makeODFS(t)
	// Multi-chunk file so the response stays active across serveClient slices.
	fs.Write(1, 1, bytes.Repeat([]byte{0xEE}, 20*500), 0)
	od := newOnDemand(fs)
	closed := false
	c := newTestODClient(&closed)

	od.mu.Lock()
	od.enqueue(c, 0, 1, 2)
	cq := od.clients[c.id()]
	od.mu.Unlock()

	// One slice → response becomes active (not finished — 20 chunks).
	od.serveClient(cq)
	od.mu.Lock()
	activeKey := ""
	if cq.active != nil {
		activeKey = cq.active.req.key
	}
	od.mu.Unlock()
	if activeKey != odKey(0, 1) {
		t.Fatalf("expected key %q active after one slice, got %q", odKey(0, 1), activeKey)
	}

	// Enqueue the SAME key while it is active → ignored (no new pending).
	od.mu.Lock()
	od.enqueue(c, 0, 1, 2)
	pc := cq.pendingCount
	od.mu.Unlock()
	if pc != 0 {
		t.Fatalf("pendingCount = %d, want 0 (active-key enqueue ignored)", pc)
	}
}

// TestOnDemandMaxPending verifies 2049 distinct pending requests for one client
// closes the connection.
func TestOnDemandMaxPending(t *testing.T) {
	od := newOnDemand(nil)
	closed := false
	c := newTestODClient(&closed)
	// Build a buffer of maxPendingPerClient+1 distinct (archive 0, file i)
	// requests at priority 0. archive byte 0, file hi/lo, priority 0.
	buf := make([]byte, 0, 4*(maxPendingPerClient+1))
	for i := 0; i <= maxPendingPerClient; i++ {
		buf = append(buf, 0, byte(i>>8), byte(i), 0)
	}
	od.onClientData(c, buf)
	if !closed {
		t.Fatalf("enqueuing %d distinct pending did not close the client", maxPendingPerClient+1)
	}
	// At the close point pendingCount is exactly maxPendingPerClient (the
	// 2049th tripped the guard before insertion).
	if got := od.pendingCountFor(t, c.id()); got != maxPendingPerClient {
		t.Fatalf("pendingCount = %d, want %d", got, maxPendingPerClient)
	}
}

// TestOnDemandClientClosedCleanup verifies clientClosed removes the client's
// queue and round-robin entry, and the pump then never serves it.
func TestOnDemandClientClosedCleanup(t *testing.T) {
	od := newOnDemand(makeODFS(t))
	closed := false
	c := newTestODClient(&closed)

	od.onClientData(c, []byte{0, 0, 7, 2}) // enqueue → scheduled on round-robin
	if !od.hasClient(c.id()) {
		t.Fatal("client queue should exist after enqueue")
	}
	if !od.inRoundRobin(c.id()) {
		t.Fatal("client should be on the round-robin after enqueue")
	}

	od.clientClosed(c)

	if od.hasClient(c.id()) {
		t.Fatal("client queue should be gone after clientClosed")
	}
	// The round-robin still holds the stale id (TS doesn't splice it); the pump
	// must skip it via the clients.get miss and not deliver anything.
	od.pump()
	if len(c.frames()) != 0 {
		t.Fatalf("pump served a closed client: %d frames", len(c.frames()))
	}
	// And the round-robin is drained (the stale entry was shifted off).
	if od.inRoundRobin(c.id()) {
		t.Fatal("stale round-robin entry should be drained by the pump")
	}
}

// TestOnDemandRunLoopLifecycle verifies run() delivers a pending request and
// exits cleanly when the stop channel is closed. No goroutine leak (verified
// by -race + the test timeout).
func TestOnDemandRunLoopLifecycle(t *testing.T) {
	fs := makeODFS(t)
	fs.Write(1, 9, []byte{0x09}, 0)
	od := newOnDemand(fs)
	closed := false
	c := newTestODClient(&closed)
	c.firstSend = make(chan struct{})

	stop := make(chan interface{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); od.run(stop) }()

	od.onClientData(c, []byte{0, 0, 9, 2})

	select {
	case <-c.firstSend:
	case <-time.After(5 * time.Second):
		t.Fatal("run loop never delivered the request within 5s")
	}

	close(stop)
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("run loop did not exit within 500ms after stop")
	}
}
