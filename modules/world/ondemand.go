package world

// ondemand.go ports OnDemand.ts:11-16 (struct + queues), :18-40 (cycle),
// :42-85 (onClientData), and :87-120 (send) from Engine-TS at commit 9aadcec4.
//
// Go adaptations (documented vs TS behaviour):
//
//  1. Caller-owned buffering / consumed return (vs TS socket-wrapper buffering):
//     In TS, ClientSocket.available tracks buffered bytes; onClientData reads
//     directly from the socket wrapper. In Go, each connection goroutine owns
//     its read buffer. onClientData therefore accepts a []byte slice and returns
//     the number of bytes consumed. The caller keeps any trailing partial frame
//     (<4 bytes) in its buffer for the next read — mirroring TS OnDemand.ts:47-49
//     (client.available < 4 → return) but placed at the caller level.
//
//  2. state==2 gate lives at the caller (vs TS OnDemand.ts:43-45):
//     TS onClientData checks client.state !== 2 and returns early. In Go the
//     connection goroutine (conn_handler.go) only routes bytes to onClientData
//     when the connection is already in the OnDemand state, so the guard is
//     enforced before the call. This is documented here to preserve TS parity
//     intent (OnDemand.ts:42-45).
//
//  3. Queue mutex for goroutine safety:
//     TS runs in a single-threaded event loop. Go uses per-connection goroutines
//     that enqueue concurrently while the cycle goroutine drains.
//     mu guards all three queues.
//
//  4. cycle() pop-all-then-send pattern (vs TS in-place splice):
//     TS OnDemand.ts:21-25 splices each element as it processes (i-- after
//     splice), draining the array while iterating. Go pops all entries under mu
//     in one shot, then sends outside the lock. This keeps enqueue latency flat
//     (conn goroutines never block waiting for the cycle lock during a long send)
//     and is behaviourally equivalent: any requests arriving while sends are in
//     progress accumulate in the queue for the next cycle.
//
//  5. 50ms run loop via time.Ticker (vs TS setTimeout re-arm):
//     TS OnDemand.ts:39: setTimeout(this.cycle.bind(this), 50) re-arms each
//     cycle. Go's run() uses a time.Ticker(50ms) which fires independently of
//     send duration — a long send can overlap the next tick, but since cycle()
//     is only called from the single run() goroutine there is no concurrent
//     cycle execution.
//
//  6. send error handling:
//     TS OnDemand.ts:109 calls client.send(…) with no error path (fire-and-
//     forget, matches the TS event-loop model). Go's odClient.send() returns
//     an error; on the first error we stop sending further chunks to that
//     client but do NOT close the connection — the connection's own read
//     goroutine will detect the dead socket and close it independently. This
//     avoids a race between the cycle goroutine and the conn goroutine on
//     connection teardown, and matches the least-surprise local convention
//     (connection lifecycle is conn-goroutine-owned).
//
//  7. FileStream decompress=false (matches TS default):
//     TS FileStream.ts:43: read(archive, file, decompress = false). The TS
//     OnDemand.ts:88 call site passes only two arguments, so decompress=false.
//     Go's Read(archive, file, false) matches this exactly.

import (
	"sync"
	"time"

	"github.com/zsrv/goscape/pkg/io/filestream"
)

// odClient is the writer/closer seam OnDemand needs from a connection.
// The production adapter wraps *client (wired in the next task); tests use
// fakes. The methods are deliberately unexported and narrower than
// io.Closer: close() is fire-and-forget (mirroring TS client.close(),
// which returns nothing — OnDemand has no error path for a failed close).
type odClient interface {
	send(data []byte) error
	close()
}

// odRequest mirrors OnDemandRequest in OnDemand.ts:5-9.
type odRequest struct {
	client  odClient
	archive int
	file    int
}

// onDemand holds the three priority queues and the cache handle.
// It mirrors the OnDemand class fields at OnDemand.ts:11-16.
type onDemand struct {
	mu     sync.Mutex  // guards urgent/extra/ingame (conn goroutines enqueue; cycle goroutine drains)
	urgent []odRequest // priority 2 — needed ASAP (OnDemand.ts:14)
	extra  []odRequest // priority 1 — not logged in, preloading extras (OnDemand.ts:15)
	ingame []odRequest // priority 0/else — logged in, preloading extras (OnDemand.ts:16)

	// cacheMu guards every cache access — FileStream is not
	// concurrency-safe (B1 port doc). Acquired by the cycle goroutine's
	// send path (next task) and by any reload-time CRC rebuild; nothing
	// in the parse/enqueue half touches the cache.
	cacheMu sync.Mutex
	cache   *filestream.FileStream // nil-able in parse-only / unit tests
}

// newOnDemand constructs an onDemand with an optional cache (nil for tests that
// only exercise parsing/enqueueing, not the send path).
func newOnDemand(cache *filestream.FileStream) *onDemand {
	return &onDemand{cache: cache}
}

// cycle drains all three queues — urgent, then extra, then ingame — in FIFO
// order within each queue, mirroring OnDemand.ts:18-37. The TS comment
// "todo: limit requests per client per cycle" is preserved as-is; the limit
// is not implemented (matching TS state at pin 9aadcec4).
//
// All pending entries are popped under mu in a single batch per queue, then
// sent outside the lock — see adaptation note (4) above.
func (od *onDemand) cycle() {
	// Pop urgent.
	od.mu.Lock()
	urgentSnap := od.urgent
	od.urgent = nil
	od.mu.Unlock()
	for _, req := range urgentSnap {
		od.send(req.client, req.archive, req.file)
	}

	// Pop extra.
	od.mu.Lock()
	extraSnap := od.extra
	od.extra = nil
	od.mu.Unlock()
	for _, req := range extraSnap {
		od.send(req.client, req.archive, req.file)
	}

	// Pop ingame.
	od.mu.Lock()
	ingameSnap := od.ingame
	od.ingame = nil
	od.mu.Unlock()
	for _, req := range ingameSnap {
		od.send(req.client, req.archive, req.file)
	}
}

// run executes the 50ms OnDemand cycle loop, mirroring the re-arm chain
// begun by OnDemand.ts:39 (setTimeout(this.cycle.bind(this), 50)).
// It blocks until stop is closed (called by the world service shutdown).
//
// stop is chan interface{} to match the Server.quit field type used
// throughout the world module for shutdown signalling.
func (od *onDemand) run(stop <-chan interface{}) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			od.cycle()
		}
	}
}

// send reads archive+1, file from the cache and streams it to the client in
// ≤500-byte chunks, each prefixed by a 6-byte header. If the file is absent
// (cache.Read returns nil) a single 6-byte rejection frame is sent instead.
//
// Mirrors OnDemand.ts:87-120. Decompress=false matches the TS default
// (FileStream.ts:43) — see adaptation note (7) above.
//
// Header layout per chunk (OnDemand.ts:100-104, mirroring Packet p1/p2/p2/p1):
//
//	byte 0:    archive  (p1)
//	bytes 1-2: file     (p2, big-endian)
//	bytes 3-4: totalLen (p2, big-endian) — always the full file length
//	byte 5:    part     (p1, 0-based chunk index)
//
// On send error: further chunks for this request are skipped; the connection
// is NOT closed here (see adaptation note (6) above).
func (od *onDemand) send(c odClient, archive, file int) {
	od.cacheMu.Lock()
	var data []byte
	if od.cache != nil {
		// TS OnDemand.ts:88: this.cache.read(archive + 1, file)
		// decompress=false mirrors TS FileStream.ts:43 default.
		data = od.cache.Read(archive+1, file, false)
	}
	od.cacheMu.Unlock()

	if data != nil {
		// TS OnDemand.ts:91-110: chunked send loop.
		totalLen := len(data)
		pos := 0
		part := 0
		for pos < totalLen {
			remaining := totalLen - pos
			if remaining > 500 {
				remaining = 500
			}
			frame := make([]byte, 6+remaining)
			frame[0] = byte(archive)
			frame[1] = byte(file >> 8)
			frame[2] = byte(file)
			frame[3] = byte(totalLen >> 8)
			frame[4] = byte(totalLen)
			frame[5] = byte(part)
			copy(frame[6:], data[pos:pos+remaining])
			if err := c.send(frame); err != nil {
				// Stop sending further chunks to this client on error;
				// do not close — see adaptation note (6).
				return
			}
			pos += remaining
			part++
		}
	} else {
		// TS OnDemand.ts:112-118: "rejected if size=0" — single 6-byte frame.
		frame := []byte{
			byte(archive),
			byte(file >> 8), byte(file),
			0, 0, // p2(0) — totalLen=0 signals rejection to the client
			0,    // p1(0) — part=0
		}
		c.send(frame) //nolint:errcheck // fire-and-forget, mirrors TS
	}
}

// onClientData parses whole 4-byte OnDemand request frames from data and enqueues
// them by priority. It returns the number of bytes consumed; the caller must
// retain any trailing partial frame (<4 bytes) for the next call.
//
// Frame layout (OnDemand.ts:56-58, mirroring Packet g1/g2/g1):
//
//	byte 0:   archive  (g1)
//	bytes 1-2: file    (g2, big-endian)
//	byte 3:   priority (g1)
//
// Rejection (OnDemand.ts:60-63): archive > 3 || priority > 2 → close the
// connection and return immediately, abandoning any remaining buffer bytes.
//
// Note: the state==2 guard (OnDemand.ts:43-45) is enforced at the caller;
// see package-level adaptation note (2) above.
func (od *onDemand) onClientData(c odClient, data []byte) (consumed int) {
	for len(data)-consumed >= 4 {
		i := consumed
		archive := int(data[i])
		file := int(data[i+1])<<8 | int(data[i+2])
		priority := int(data[i+3])

		// OnDemand.ts:60-63: reject invalid archive/priority, close and return immediately.
		if archive > 3 || priority > 2 {
			c.close()
			return consumed
		}

		req := odRequest{client: c, archive: archive, file: file}

		od.mu.Lock()
		switch priority {
		case 2: // urgent (OnDemand.ts:65-70)
			od.urgent = append(od.urgent, req)
		case 1: // extra (OnDemand.ts:71-76)
			od.extra = append(od.extra, req)
		default: // ingame — priority 0 and anything else ≤2 (OnDemand.ts:77-82)
			od.ingame = append(od.ingame, req)
		}
		od.mu.Unlock()

		consumed += 4
	}
	return consumed
}
