package world

// ondemand.go ports OnDemandThread.ts from Engine-TS at commit dee467c8 (rev-274).
//
// The 274 rewrite replaced the old (254) fixed-50ms-cycle "send every queued
// file fully" scheduler (OnDemand.ts @9aadcec4) with an immediate, throttled,
// priority-preemptive, round-robin pump. Wire protocol (request frame + chunk
// frame) is byte-identical 254↔274 — only the SCHEDULING changed.
//
// Per-client state lives in clientQueue (TS ClientQueue): three priority queues
// (0/1/2), a key→request dedup map, a pendingCount, the in-progress chunked
// response (active), and a scheduled flag. The global roundRobin holds the ids
// of clients with work; the pump shifts one at a time, serves a bounded slice,
// and re-appends if more work remains.
//
// Go adaptations (documented vs TS behaviour):
//
//  1. Goroutine pump vs worker thread:
//     TS runs OnDemandThread as a worker thread driven by parentPort messages
//     and setImmediate(pump). Go runs a single pump goroutine (onDemand.run)
//     that blocks on a buffered signal channel (cap 1). enqueue/clientClosed
//     are called from the per-connection goroutines under od.mu, then signal
//     the pump. This replaces TS schedulePump()/setImmediate.
//
//  2. No MAX_PUMP_MS bound:
//     TS bounds the pump loop to 8ms so it yields back to the event loop
//     (its socket writes, message handling, etc. all run on that one loop).
//     In Go there is no shared event loop to starve: the conn goroutines
//     enqueue concurrently under od.mu while the pump runs, and the pump
//     picks them up on its next roundRobin pass. So the pump drains the
//     roundRobin to empty (or until a whole pass sends nothing — see
//     adaptation (7)), then blocks on the signal — the per-client slice
//     cap (MAX_BYTES_PER_CLIENT_SLICE / MAX_CHUNKS_PER_CLIENT_SLICE) is what
//     preserves round-robin fairness + priority preemption, NOT the time
//     bound. visitsRemaining (= roundRobin length at the start of a drain
//     pass) is still honoured so a client re-appended mid-pass waits for the
//     next pass — matching TS's per-pump fairness.
//
//  3. Queue mutex for goroutine safety:
//     od.mu guards the clients map, roundRobin, every clientQueue's fields,
//     and the pumpSignal bookkeeping. The pump takes od.mu to select the next
//     client + the next request/chunk, then RELEASES it for the actual
//     cache.Read + client.send (both potentially slow / blocking I/O). This
//     keeps enqueue latency flat — conn goroutines never block on a long send.
//
//  4. cacheMu guards every cache access — FileStream is not concurrency-safe.
//     Held only for the cache.Read, separately from od.mu.
//
//  5. send error handling:
//     TS postChunk is fire-and-forget. Go's odClient.send() returns an error;
//     on error we abandon the active response for that client (drop it) but do
//     NOT close — the connection's own read goroutine detects the dead socket
//     and closes it (which fires clientClosed). Connection lifecycle stays
//     conn-goroutine-owned.
//
//  6. FileStream decompress=false (TS OnDemandThread.ts cache.read(archive+1,
//     file) passes no decompress arg → false).
//
//  7. Outbound backpressure (DEVIATION SEC1-D3):
//     TS postChunk hands the chunk to socket.write, whose Node buffer is
//     unbounded, so the pump is never throttled. goscape's writes used to be
//     synchronous (clientODAdapter.send → blocking conn.Write), which threw
//     the throttling in for free; once SEC1 M-2 moved writes onto a bounded
//     per-client queue that throttle disappeared, and a slow-but-alive
//     downloader would be pushed into the hard cap and disconnected — at only
//     ~260 KiB, since a ~506-byte chunk occupies a whole slot. serveClient
//     therefore ends the slice as soon as odClient.backlogged() reports the
//     soft high-water mark, leaving cq.active and the pending requests
//     untouched; the client stays scheduled and is retried on a later pass
//     (pump's per-pass bound guarantees the current pass still terminates,
//     and run re-arms after backloggedRetryInterval). Slow downloaders are
//     served slowly rather than dropped. Ordering, priority preemption and
//     the slice caps are unchanged (OnDemandThread.ts:202-228).

import (
	"strconv"
	"sync"
	"time"

	"github.com/zsrv/goscape/pkg/io/filestream"
)

// OnDemandThread.ts:48-51 constants.
const (
	maxPendingPerClient     = 2048
	maxBytesPerClientSlice  = 8000
	maxChunksPerClientSlice = 16
)

// backloggedRetryInterval is how long run waits before re-running the pump
// for a client that yielded on outbound backpressure (adaptation (7)). One
// server tick: long enough that a still-backlogged client costs one cheap
// no-op pass per tick, short enough that a draining one resumes promptly.
// goscape-only — TS has no equivalent, its pump is never throttled.
const backloggedRetryInterval = 600 * time.Millisecond

// odClient is the writer/closer seam OnDemand needs from a connection.
// The production adapter wraps *client; tests use fakes.
//
// id() is the STABLE per-connection identity used as the clientQueue map key
// and the roundRobin entry. The production *client mints it once at
// newClient (a uuid), so it survives across the many transient
// clientODAdapter values created per onClientData call (server.go:1056).
type odClient interface {
	send(data []byte) error
	close()
	id() string
	// backlogged reports that the connection's outbound queue has passed
	// its soft high-water mark. The pump ends the client's slice when it
	// is true and retries on a later pass, instead of pushing the queue
	// into its hard cap (which would disconnect a merely-slow downloader).
	// See outbound.go and adaptation (7).
	backlogged() bool
}

// odRequest mirrors PendingRequest in OnDemandThread.ts:31-38.
type odRequest struct {
	archive   int
	file      int
	priority  int
	key       string
	cancelled bool
}

// odActive mirrors ActiveResponse in OnDemandThread.ts:40-45: an in-progress
// chunked response. data is the full file bytes (nil → rejection); pos is the
// next byte to send; part is the 0-based chunk index.
type odActive struct {
	req  odRequest
	data []byte
	pos  int
	part int
}

// clientQueue mirrors ClientQueue in OnDemandThread.ts:47-55. Holds one
// client's three priority queues, the key→request dedup map, the in-progress
// response, and the scheduled flag. All fields are guarded by onDemand.mu.
type clientQueue struct {
	c            odClient
	queues       [3][]*odRequest // queues[priority]; index 2 = urgent
	pendingByKey map[string]*odRequest
	pendingCount int
	active       *odActive
	scheduled    bool
}

// onDemand holds the per-client queues, the global round-robin schedule, and
// the cache handle. Mirrors the module-level state in OnDemandThread.ts:71-77.
type onDemand struct {
	mu         sync.Mutex // guards clients, roundRobin, every clientQueue
	clients    map[string]*clientQueue
	roundRobin []string

	// pumpSignal wakes the pump goroutine. Buffered cap 1: a signal while one
	// is already queued is a no-op (the pump drains everything each wake).
	// Replaces TS schedulePump()/setImmediate (OnDemandThread.ts:159-166).
	pumpSignal chan struct{}

	// cacheMu guards every cache access — FileStream is not concurrency-safe.
	cacheMu sync.Mutex
	cache   *filestream.FileStream // nil-able in parse-only / unit tests
}

// newOnDemand constructs an onDemand with an optional cache (nil for tests that
// only exercise parsing/enqueueing, not the send path).
func newOnDemand(cache *filestream.FileStream) *onDemand {
	return &onDemand{
		clients:    make(map[string]*clientQueue),
		pumpSignal: make(chan struct{}, 1),
		cache:      cache,
	}
}

// signalPump wakes the pump goroutine (non-blocking; coalesces). Mirrors TS
// schedulePump() (OnDemandThread.ts:159-166) but without the event-loop
// debounce — the buffered cap-1 channel is the debounce.
func (od *onDemand) signalPump() {
	select {
	case od.pumpSignal <- struct{}{}:
	default:
	}
}

// getClient returns the clientQueue for c.id(), creating it if absent.
// Mirrors OnDemandThread.ts:128-145. Caller must hold od.mu.
func (od *onDemand) getClient(c odClient) *clientQueue {
	id := c.id()
	if cq, ok := od.clients[id]; ok {
		return cq
	}
	cq := &clientQueue{
		c:            c,
		pendingByKey: make(map[string]*odRequest),
	}
	od.clients[id] = cq
	return cq
}

// scheduleClient appends the client to the round-robin if not already on it.
// Mirrors OnDemandThread.ts:147-154. Caller must hold od.mu.
func (od *onDemand) scheduleClient(cq *clientQueue) {
	if cq.scheduled {
		return
	}
	cq.scheduled = true
	od.roundRobin = append(od.roundRobin, cq.c.id())
}

// enqueue validates and adds a request, performing dedup, priority-cancel, and
// MAX_PENDING enforcement. Mirrors OnDemandThread.ts:91-126. Caller must hold
// od.mu. Returns false if the client must be closed (caller closes outside the
// lock so close() never runs under od.mu).
func (od *onDemand) enqueue(c odClient, archive, file, priority int) (ok bool) {
	if archive < 0 || archive > 3 || priority < 0 || priority > 2 {
		return false
	}

	cq := od.getClient(c)
	key := odKey(archive, file)

	// Same key as the in-progress response → ignore (OnDemandThread.ts:102-104).
	if cq.active != nil && cq.active.req.key == key {
		return true
	}

	// Existing pending with same key: keep the higher priority; if the new one
	// is strictly higher, cancel the existing (OnDemandThread.ts:106-113).
	if existing, found := cq.pendingByKey[key]; found {
		if existing.priority >= priority {
			return true
		}
		existing.cancelled = true
		cq.pendingCount--
	}

	if cq.pendingCount >= maxPendingPerClient {
		return false
	}

	req := &odRequest{archive: archive, file: file, priority: priority, key: key}
	cq.queues[priority] = append(cq.queues[priority], req)
	cq.pendingByKey[key] = req
	cq.pendingCount++
	od.scheduleClient(cq)
	return true
}

func odKey(archive, file int) string {
	// TS uses `${archive}:${file}`; the exact string only needs to be a stable
	// per-(archive,file) key, but we mirror the TS form for parity.
	return strconv.Itoa(archive) + ":" + strconv.Itoa(file)
}

// pump drains the round-robin: one visit per scheduled client (as of the start
// of the pass), serving a bounded slice each. Re-schedules clients with
// remaining work; deletes those without. Loops until the round-robin is empty
// or a whole pass made no progress.
// Mirrors OnDemandThread.ts:168-200 (sans MAX_PUMP_MS — see adaptation (2)).
//
// The per-pass bound is what keeps the loop terminating now that a client can
// end its slice without sending anything (adaptation (7)): such a client is
// re-scheduled with all its work intact, so "loop until the round-robin is
// empty" alone would spin on it forever. Each pass visits at most the number
// of clients that were on the ring when the pass started, and the outer loop
// only starts another pass if that one actually sent something. yielded
// reports that at least one client still has work but was backlogged, so the
// caller knows to re-arm rather than sleep until the next request arrives.
func (od *onDemand) pump() (yielded bool) {
	for {
		od.mu.Lock()
		visits := len(od.roundRobin)
		if visits == 0 {
			od.mu.Unlock()
			return yielded
		}
		od.mu.Unlock()

		progress := false
		for ; visits > 0; visits-- {
			od.mu.Lock()
			if len(od.roundRobin) == 0 {
				od.mu.Unlock()
				break
			}
			id := od.roundRobin[0]
			od.roundRobin = od.roundRobin[1:]
			cq, ok := od.clients[id]
			if !ok {
				// Client was deleted (clientClosed) while on the round-robin —
				// skip it, mirroring TS clients.get(clientId) miss → continue.
				od.mu.Unlock()
				continue
			}
			cq.scheduled = false
			od.mu.Unlock()

			sent, backlogged := od.serveClient(cq)
			if sent > 0 {
				progress = true
			}

			od.mu.Lock()
			if od.hasWork(cq) {
				od.scheduleClient(cq)
				// Only a client that is both backlogged AND still holding
				// work needs a retry; one that merely ran out of requests
				// will be woken by its next request like any other.
				if backlogged {
					yielded = true
				}
			} else {
				delete(od.clients, cq.c.id())
			}
			od.mu.Unlock()
		}
		if !progress {
			// Every client on the ring was visited and none of them sent a
			// byte (all backlogged, or all out of servable work). Another
			// pass would repeat that verbatim, so stop and let run decide
			// when to come back.
			return yielded
		}
	}
}

// serveClient sends up to one bounded slice of chunks for the client: at most
// MAX_CHUNKS_PER_CLIENT_SLICE chunks and MAX_BYTES_PER_CLIENT_SLICE bytes.
// Mirrors OnDemandThread.ts:202-228.
//
// od.mu is acquired per-step to advance the active response / pick the next
// request, then RELEASED for cache.Read and client.send (slow I/O).
//
// It also ends the slice early when the connection's outbound queue is
// backlogged (DEVIATION SEC1-D3, adaptation (7)): cq.active and every pending
// request are left exactly as they are, nothing is sent, and the connection is
// NOT closed. The second return value tells the caller to keep the client
// scheduled for a later pass.
func (od *onDemand) serveClient(cq *clientQueue) (bytesSent int, backlogged bool) {
	chunks := 0

	for bytesSent < maxBytesPerClientSlice && chunks < maxChunksPerClientSlice {
		// Checked before anything is popped or read, so yielding costs the
		// client nothing but a delay.
		if cq.c.backlogged() {
			return bytesSent, true
		}
		od.mu.Lock()
		if cq.active == nil {
			req := od.nextRequest(cq)
			if req == nil {
				od.mu.Unlock()
				return bytesSent, false
			}
			cq.active = &odActive{req: *req}
			// Read the file under cacheMu (FileStream not concurrency-safe).
			od.mu.Unlock()
			od.cacheMu.Lock()
			if od.cache != nil {
				cq.active.data = od.cache.Read(req.archive+1, req.file, false)
			}
			od.cacheMu.Unlock()
			od.mu.Lock()
		}

		frame := nextChunk(cq.active)
		active := cq.active
		// active becomes nil when the file is fully sent (or it was a
		// rejection: data==nil → one frame then done).
		if active.data == nil || active.pos >= len(active.data) {
			cq.active = nil
		}
		od.mu.Unlock()

		if err := cq.c.send(frame); err != nil {
			// Drop the in-progress response on send error; do not close (see
			// adaptation (5)). The conn read goroutine will close + clientClosed.
			od.mu.Lock()
			cq.active = nil
			od.mu.Unlock()
			return bytesSent, false
		}
		bytesSent += len(frame)
		chunks++
	}
	return bytesSent, false
}

// nextRequest pops the next non-cancelled request, scanning priority 2→0.
// Mirrors OnDemandThread.ts:230-247. Caller must hold od.mu.
func (od *onDemand) nextRequest(cq *clientQueue) *odRequest {
	for priority := 2; priority >= 0; priority-- {
		q := cq.queues[priority]
		for len(q) > 0 {
			req := q[0]
			q = q[1:]
			if req.cancelled {
				continue
			}
			cq.queues[priority] = q
			delete(cq.pendingByKey, req.key)
			cq.pendingCount--
			return req
		}
		cq.queues[priority] = q
	}
	return nil
}

// nextChunk builds the next ≤500-byte chunk frame for an active response,
// advancing pos/part. A nil data field yields one 6-byte rejection frame
// (length=0, part=0). Mirrors OnDemandThread.ts:249-261 + encodeChunk
// (:263-277). The 6-byte header layout is byte-identical to the 254 send.
func nextChunk(a *odActive) []byte {
	if a.data == nil {
		return encodeChunk(a.req.archive, a.req.file, 0, 0, nil)
	}
	remaining := len(a.data) - a.pos
	if remaining > 500 {
		remaining = 500
	}
	frame := encodeChunk(a.req.archive, a.req.file, len(a.data), a.part, a.data[a.pos:a.pos+remaining])
	a.pos += remaining
	a.part++
	return frame
}

// encodeChunk builds a chunk frame: 6-byte header + payload. Mirrors
// OnDemandThread.ts:263-277 (and goscape's prior 254 send layout exactly).
//
//	byte 0:    archive
//	bytes 1-2: file     (big-endian)
//	bytes 3-4: length   (big-endian; full file length, 0 for a rejection)
//	byte 5:    part     (0-based chunk index)
func encodeChunk(archive, file, length, part int, payload []byte) []byte {
	frame := make([]byte, 6+len(payload))
	frame[0] = byte(archive)
	frame[1] = byte(file >> 8)
	frame[2] = byte(file)
	frame[3] = byte(length >> 8)
	frame[4] = byte(length)
	frame[5] = byte(part)
	copy(frame[6:], payload)
	return frame
}

// hasWork reports whether the client has an active response or pending requests.
// Mirrors OnDemandThread.ts:279-281. Caller must hold od.mu.
func (od *onDemand) hasWork(cq *clientQueue) bool {
	return cq.active != nil || cq.pendingCount > 0
}

// run drives the pump: block on the signal channel, then drain the round-robin
// to empty. Replaces the old 254 50ms ticker. Mirrors the TS
// schedulePump→pump wake chain (OnDemandThread.ts:159-200), but the goroutine
// loops the round-robin to empty per wake instead of re-arming via setImmediate.
//
// stop is a receive-only signal channel matching Server.quit used throughout
// the world server.
//
// When a pass yielded on outbound backpressure (adaptation (7)) the round-robin
// still holds work that no incoming request will necessarily wake, so the wait
// is capped at backloggedRetryInterval instead of blocking on pumpSignal alone.
// Re-signalling immediately would busy-spin against a client that is still
// backlogged; one server tick of delay is invisible next to the time the peer
// needs to drain a queue that is already half a megabyte deep.
func (od *onDemand) run(stop <-chan struct{}) {
	yielded := false
	for {
		var retry <-chan time.Time
		if yielded {
			retry = time.After(backloggedRetryInterval)
		}
		select {
		case <-stop:
			return
		case <-od.pumpSignal:
		case <-retry:
		}
		yielded = od.pump()
	}
}

// onClientData parses whole 4-byte OnDemand request frames from data and
// enqueues them per-client. Returns the number of bytes consumed; the caller
// retains any trailing partial frame (<4 bytes) for the next call.
//
// Frame layout (OnDemandThread request, byte-identical to 254):
//
//	byte 0:    archive  (g1)
//	bytes 1-2: file     (g2, big-endian)
//	byte 3:    priority (g1)
//
// Rejection (OnDemandThread.ts:92-95): archive∉[0,3] || priority∉[0,2] → close
// the connection and return immediately, abandoning remaining buffer bytes.
//
// Signals the pump once after enqueuing so a request is served immediately
// (the fix for slow NPC chat-head model delivery — no 50ms wait).
func (od *onDemand) onClientData(c odClient, data []byte) (consumed int) {
	enqueued := false
	for len(data)-consumed >= 4 {
		i := consumed
		archive := int(data[i])
		file := int(data[i+1])<<8 | int(data[i+2])
		priority := int(data[i+3])

		od.mu.Lock()
		ok := od.enqueue(c, archive, file, priority)
		od.mu.Unlock()
		if !ok {
			// Invalid request OR over MAX_PENDING → close + return immediately
			// (TS enqueue closeClient paths, OnDemandThread.ts:93/116).
			c.close()
			if enqueued {
				od.signalPump()
			}
			return consumed
		}
		enqueued = true
		consumed += 4
	}
	if enqueued {
		od.signalPump()
	}
	return consumed
}

// clientClosed removes a client's queue and round-robin entry. Mirrors TS
// deleteClient (OnDemandThread.ts:289-302) driven by the 'client_closed'
// message. Wired into the connection-close path (server.go handleTCPConn
// teardown) so a disconnected client's queue + round-robin entry don't leak.
//
// The round-robin entry (if mid-pass) is left to the pump's clients.get miss
// to skip — we only delete from the clients map here, matching TS, which does
// NOT splice the round-robin (the pump's lookup miss handles it).
func (od *onDemand) clientClosed(c odClient) {
	od.mu.Lock()
	delete(od.clients, c.id())
	od.mu.Unlock()
}
