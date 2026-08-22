package world

import (
	"bytes"
	"errors"
	"log/slog"
	"net"
	"sync"
	"time"
)

// Outbound queue caps. A healthy client drains a few KiB per tick; a
// client that stops reading fills its kernel window within seconds and
// then hits these. DEVIATION SEC1-D3: TS socket.write buffers without
// bound and never disconnects; goscape bounds memory and closes.
//
// Bytes is the real limiter; the slot count only exists so the channel
// is bounded. Slots are deliberately generous because the OnDemand pump
// is a producer too, and it lost its only backpressure when writes went
// asynchronous: it emits one ~508-byte cache chunk per send, so a tight
// slot cap would disconnect a legitimately-downloading client after only
// a few tens of KiB. At 512 slots the byte cap (1 MiB) is what a stalled
// peer trips, whatever the frame size.
const (
	maxOutboundQueueSlots = 512
	maxOutboundQueueBytes = 1 << 20
)

// fallbackDrainTimeout bounds teardown when writeTimeout <= 0 has turned
// the per-write deadlines off. It is short on purpose: at that point the
// connection is going away, and all it has to cover is handing an
// already-queued logout byte or login rejection to a peer that is still
// reading.
const fallbackDrainTimeout = 250 * time.Millisecond

var errOutboundFull = errors.New("outbound queue overflow")

// outboundState is the writer's lifecycle, guarded by outboundWriter.mu.
// It replaces the sync.Once pair the first draft used: with a single
// state word every Write/Close interleaving is decided under one lock,
// so a Close cannot slip between a Write's "am I open?" check and its
// enqueue and strand bytes on a queue nobody will ever drain.
type outboundState int

const (
	// outboundIdle: nothing has been written yet, so no goroutine exists.
	outboundIdle outboundState = iota
	// outboundRunning: the writer goroutine has been started. It owns the
	// socket from here on and is the only thing that closes it.
	outboundRunning
	// outboundClosed: Close has run. No further writes are accepted.
	outboundClosed
)

// outboundWriter sits between a client's bufio.Writer and its net.Conn.
// Write never blocks: it copies the bytes onto a bounded queue that a
// per-client goroutine drains to the socket under a per-write deadline.
// This is what keeps a stalled or slow-reading client from holding the
// tick goroutine inside net.Conn.Write (SEC1 M-2).
//
// Single producer (whoever owns bufw: the conn goroutine pre-login, the
// tick goroutine post-login, the OnDemand pump in state 2 — each already
// serialised by the existing ownership rules), single consumer (the
// goroutine). Close is safe from any goroutine.
type outboundWriter struct {
	conn         net.Conn
	writeTimeout time.Duration
	log          *slog.Logger

	queue chan []byte
	done  chan struct{}

	mu     sync.Mutex // guards queued, state, failed
	queued int
	state  outboundState
	// failed records that the writer goroutine hit a socket error and
	// closed the connection. Further writes are pointless; they report
	// net.ErrClosed so bufio's sticky error surfaces to the caller the
	// same way a direct conn.Write failure used to.
	failed bool
}

func newOutboundWriter(conn net.Conn, writeTimeout time.Duration, log *slog.Logger) *outboundWriter {
	return &outboundWriter{
		conn:         conn,
		writeTimeout: writeTimeout,
		log:          log,
		queue:        make(chan []byte, maxOutboundQueueSlots),
		done:         make(chan struct{}),
	}
}

// Write enqueues a copy of p. It returns net.ErrClosed after Close (or
// after a socket failure) and errOutboundFull — having closed the
// connection — when either cap would be exceeded.
//
// The copy is mandatory: bufio.Writer.Flush hands us its *internal*
// buffer, which it reuses the moment Write returns.
func (o *outboundWriter) Write(p []byte) (int, error) {
	o.mu.Lock()
	if o.state == outboundClosed || o.failed {
		o.mu.Unlock()
		return 0, net.ErrClosed
	}
	if o.queued+len(p) > maxOutboundQueueBytes || len(o.queue) >= maxOutboundQueueSlots {
		queued := o.queued
		o.failed = true
		o.mu.Unlock()
		o.log.Warn("outbound queue overflow; closing connection",
			"remote_addr", o.conn.RemoteAddr(), "queued_bytes", queued, "frame_bytes", len(p))
		_ = o.conn.Close()
		return 0, errOutboundFull
	}
	o.queued += len(p)
	start := o.state == outboundIdle
	if start {
		o.state = outboundRunning
	}
	// Sending under mu is safe and keeps enqueue atomic with the state
	// check: the slot check above ran under the same lock and only the
	// consumer removes entries, so the channel cannot be full here and
	// the send never blocks.
	o.queue <- bytes.Clone(p)
	o.mu.Unlock()

	if start {
		go o.run()
	}
	return len(p), nil
}

// Close stops accepting writes and lets the writer goroutine drain what
// is already queued under one writeTimeout budget before it closes the
// socket. It does not wait for that drain — no caller of Close (least of
// all the tick goroutine) ever blocks on the network. Idempotent and
// safe from any goroutine.
//
// Teardown is bounded even when writeTimeout <= 0 disables the per-write
// deadlines, which the config allows (Config.Validate does not constrain
// tcp_server_write_timeout). Without that bound a writer already parked
// in a deadline-less conn.Write against a stalled peer would survive
// closeConn and leak the fd, its goroutine, and the reader-side teardown
// waiting on the socket. Close therefore stamps a fallbackDrainTimeout
// deadline on the socket, which applies to the *pending* write as well as
// to drain's (net.Conn: "the deadline applies to all future and pending
// I/O"). That both frees the parked write and caps the drain under one
// absolute budget, after which run's defer closes the socket. Closing the
// socket outright here instead would be simpler but would silently
// discard the logout byte or login rejection that the drain exists to
// deliver.
func (o *outboundWriter) Close() {
	o.mu.Lock()
	if o.state == outboundClosed {
		o.mu.Unlock()
		return
	}
	// A running goroutine owns the socket and closes it after draining;
	// if it already exited on a socket error it closed the socket then.
	// Only the never-started case leaves the close to us.
	started := o.state == outboundRunning
	o.state = outboundClosed
	o.mu.Unlock()

	close(o.done)
	if !started {
		// Nothing was ever written, so there is nothing to drain and no
		// goroutine that will ever close the socket.
		_ = o.conn.Close()
		return
	}
	if o.writeTimeout <= 0 {
		_ = o.conn.SetWriteDeadline(time.Now().Add(fallbackDrainTimeout))
	}
}

// run is the single consumer. It owns every conn.Write and every write
// deadline for the connection's lifetime, and closes the socket on the
// way out (drained, failed, or otherwise).
func (o *outboundWriter) run() {
	defer func() { _ = o.conn.Close() }()
	for {
		// done wins over pending frames. Without this priority check the
		// select below picks a ready arm at random, so after Close the
		// queue keeps feeding writeOne — each frame with its own fresh
		// deadline — and teardown could take slots × writeTimeout instead
		// of the single budget drain() applies.
		select {
		case <-o.done:
			o.drain()
			return
		default:
		}
		select {
		case buf := <-o.queue:
			if !o.writeOne(buf) {
				o.discardAll()
				return
			}
		case <-o.done:
			o.drain()
			return
		}
	}
}

// drain writes whatever is already queued under a single absolute
// deadline, so a logout byte or login rejection flushed immediately
// before closeConn still reaches the peer without letting a dead peer
// hold the goroutine for one writeTimeout per frame.
func (o *outboundWriter) drain() {
	// With deadlines disabled the socket already carries the
	// fallbackDrainTimeout that Close stamped on it; leave it in force
	// rather than clearing the only bound teardown has.
	if o.writeTimeout > 0 {
		_ = o.conn.SetWriteDeadline(time.Now().Add(o.writeTimeout))
	}
	for {
		select {
		case buf := <-o.queue:
			if _, err := o.conn.Write(buf); err != nil {
				o.discardAll()
				return
			}
		default:
			return
		}
	}
}

// writeOne writes buf under a fresh deadline; false on failure.
func (o *outboundWriter) writeOne(buf []byte) bool {
	if o.writeTimeout > 0 {
		_ = o.conn.SetWriteDeadline(time.Now().Add(o.writeTimeout))
	}
	_, err := o.conn.Write(buf)
	o.mu.Lock()
	o.queued -= len(buf)
	if err != nil {
		o.failed = true
	}
	o.mu.Unlock()
	if err != nil {
		o.log.Info("outbound write failed; closing connection",
			"remote_addr", o.conn.RemoteAddr(), "err", err)
		return false
	}
	return true
}

// discardAll drops anything still queued. Only reached once the socket
// is known dead, so the bytes have nowhere to go.
func (o *outboundWriter) discardAll() {
	for {
		select {
		case <-o.queue:
		default:
			return
		}
	}
}
