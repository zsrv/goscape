package world

// ondemand.go ports OnDemand.ts:11-16 (struct + queues) and :42-85 (onClientData)
// from Engine-TS at commit 9aadcec4.
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
//     that enqueue concurrently while the cycle goroutine (next task) drains.
//     mu guards all three queues.

import (
	"sync"

	"github.com/zsrv/goscape/pkg/io/filestream"
)

// odClient is the writer/closer seam OnDemand needs from a connection.
// The production adapter wraps *client (wired in the next task); tests use fakes.
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
	mu     sync.Mutex // guards urgent/extra/ingame (conn goroutines enqueue; cycle goroutine drains)
	urgent []odRequest // priority 2 — needed ASAP (OnDemand.ts:14)
	extra  []odRequest // priority 1 — not logged in, preloading extras (OnDemand.ts:15)
	ingame []odRequest // priority 0/else — logged in, preloading extras (OnDemand.ts:16)

	cacheMu sync.Mutex          // FileStream is not concurrency-safe (B1 port doc)
	cache   *filestream.FileStream // nil-able in parse-only / unit tests
}

// newOnDemand constructs an onDemand with an optional cache (nil for tests that
// only exercise parsing/enqueueing, not the send path).
func newOnDemand(cache *filestream.FileStream) *onDemand {
	return &onDemand{cache: cache}
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
