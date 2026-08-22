package world

import (
	"bytes"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// closeSignalConn reports when the connection is closed. Everything else
// (including SetWriteDeadline semantics) is the embedded net.Pipe end, so
// the stalls these tests drive are real deadline expiries, not simulated
// ones. Waiting on closed is how a test observes a close without a peer
// Read, which on a net.Pipe would satisfy the very write it is testing.
type closeSignalConn struct {
	net.Conn
	once   sync.Once
	closed chan struct{}
}

func newCloseSignalConn(c net.Conn) *closeSignalConn {
	return &closeSignalConn{Conn: c, closed: make(chan struct{})}
}

func (c *closeSignalConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

// SEC1 M-2 / DEVIATION SEC1-D3: a peer that never reads must not block
// the writer's caller (the tick goroutine). The queue absorbs writes
// instantly and the connection is closed once a cap is exceeded.
func TestOutboundWriter_NeverBlocksCaller(t *testing.T) {
	client, server := net.Pipe() // client side never reads
	t.Cleanup(func() { client.Close(); server.Close() })
	sc := newCloseSignalConn(server)
	// A generous per-write deadline (the production default) on purpose:
	// the failure this pins is the *queue cap*, so the writer goroutine
	// must still be parked in its first conn.Write when the caps are hit.
	// A short deadline would let the goroutine fail on its own and the
	// loop would exit on net.ErrClosed instead of errOutboundFull. It also
	// sharpens the blocking check below — a synchronous writer would hang
	// on the very first frame for 2s, far past the 500ms budget.
	o := newOutboundWriter(sc, 2*time.Second, discardLogger())

	frame := bytes.Repeat([]byte{0xAB}, 8<<10) // 8 KiB
	start := time.Now()
	var lastErr error
	for range 200 { // 1.6 MiB ≫ maxOutboundQueueBytes
		if _, err := o.Write(frame); err != nil {
			lastErr = err
			break
		}
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("Write blocked for %v", d)
	}
	if lastErr == nil {
		t.Fatal("expected an overflow error once the queue cap was exceeded")
	}
	if !errors.Is(lastErr, errOutboundFull) {
		t.Fatalf("overflow error: got %v, want errOutboundFull", lastErr)
	}
	// Overflow must actually close the socket. Assert that on the conn
	// itself: polling client.Read here could never fail the test, because
	// its own read deadline produces a non-nil error whether or not the
	// overflow path closed anything.
	select {
	case <-sc.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("queue overflow did not close the connection")
	}
	// And the peer observes it rather than blocking forever.
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := client.Read(make([]byte, 1)); err == nil {
		t.Fatal("peer read succeeded on a connection that should be closed")
	}
}

// SEC1 M-2 review: writeTimeout <= 0 disables the per-write deadlines,
// which the config allows. Teardown must stay bounded anyway — a writer
// parked forever in a deadline-less conn.Write against a stalled peer
// would otherwise survive closeConn and leak the fd, its goroutine, and
// the reader-side teardown waiting on the socket — without throwing away
// the drain that delivers a logout byte or login rejection.
func TestOutboundWriter_CloseWithoutTimeout(t *testing.T) {
	// A peer that never reads: the goroutine is parked in conn.Write with
	// no deadline that will ever fire, so Close is the only thing that can
	// free it. The socket must still end up closed, promptly.
	t.Run("stalled peer still gets closed", func(t *testing.T) {
		client, server := net.Pipe()
		t.Cleanup(func() { client.Close(); server.Close() })
		sc := newCloseSignalConn(server)
		o := newOutboundWriter(sc, 0, discardLogger())

		if _, err := o.Write([]byte("stuck")); err != nil {
			t.Fatal(err)
		}
		o.Close()
		select {
		case <-sc.closed:
		case <-time.After(5 * time.Second):
			t.Fatal("Close left the socket open with deadlines disabled")
		}
		if _, err := o.Write([]byte("late")); !errors.Is(err, net.ErrClosed) {
			t.Fatalf("write after close: got %v, want net.ErrClosed", err)
		}
	})

	// ...and a peer that IS reading must still receive what was queued.
	// Bounding teardown by closing the socket outright would silently drop
	// the login rejection / logout byte that the drain exists to deliver
	// (the end-to-end case is
	// TestHandleLogin_TruncatedRSABlock_RejectsWithClientOutOfDate, whose
	// server harness leaves tcp_server_write_timeout at 0).
	t.Run("reading peer still gets the queued bytes", func(t *testing.T) {
		client, server := net.Pipe()
		t.Cleanup(func() { client.Close() })
		o := newOutboundWriter(server, 0, discardLogger())

		got := make(chan []byte, 1)
		go func() { b, _ := io.ReadAll(client); got <- b }()
		if _, err := o.Write([]byte("bye")); err != nil {
			t.Fatal(err)
		}
		o.Close()
		select {
		case b := <-got:
			if string(b) != "bye" {
				t.Fatalf("got %q, want %q", b, "bye")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("queued bytes were discarded instead of drained")
		}
	})
}

// Bytes queued before Close are delivered in order, then the peer sees EOF.
func TestOutboundWriter_CloseDrainsInOrder(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { client.Close() })
	o := newOutboundWriter(server, time.Second, discardLogger())

	got := make(chan []byte, 1)
	go func() {
		b, _ := io.ReadAll(client) // returns on EOF after server closes
		got <- b
	}()
	for _, chunk := range [][]byte{[]byte("one"), []byte("two"), []byte("three")} {
		if _, err := o.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	o.Close()
	o.Close() // idempotent

	select {
	case b := <-got:
		if string(b) != "onetwothree" {
			t.Fatalf("got %q", b)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("peer never received EOF")
	}
	if _, err := o.Write([]byte("late")); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("write after close: got %v, want net.ErrClosed", err)
	}
}

// A write that the peer stalls past writeTimeout closes the conn instead
// of hanging the goroutine forever.
//
// The peer must never read here: on a net.Pipe a single peer Read
// satisfies the pending write, the deadline never fires, and the conn is
// never closed. So the close is observed directly on the conn (which
// cannot pass for the wrong reason) and only then is the peer's own view
// of the closed pipe checked.
func TestOutboundWriter_WriteTimeoutCloses(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() { client.Close(); server.Close() })
	sc := newCloseSignalConn(server)
	o := newOutboundWriter(sc, 30*time.Millisecond, discardLogger())
	if _, err := o.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	// Nobody reads `client`; the goroutine's conn.Write must time out and close.
	select {
	case <-sc.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("expected the server side to close after write timeout")
	}
	// And the peer observes it rather than blocking forever.
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := client.Read(make([]byte, 4)); err == nil {
		t.Fatal("peer read succeeded on a connection that should be closed")
	}
}

// End-to-end through client: closeConn after writeOut+flush delivers the
// frame, and a second closeConn is harmless.
func TestClientCloseConn_DeliversPendingBytes(t *testing.T) {
	c, peer := newTestClient(t)
	got := make(chan []byte, 1)
	go func() { b, _ := io.ReadAll(peer); got <- b }()
	c.bufw.Write([]byte{7, 8, 9})
	if err := c.flushWrite(); err != nil {
		t.Fatal(err)
	}
	c.closeConn()
	c.closeConn()
	select {
	case b := <-got:
		if !bytes.Equal(b, []byte{7, 8, 9}) {
			t.Fatalf("got %v", b)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pending bytes not delivered before close")
	}
}

// SEC1 M-2 / DEVIATION SEC1-D3: Backlogged is the soft high-water signal
// the OnDemand pump consults so a slow downloader gets served slowly
// instead of being pushed into the hard cap and disconnected. It must go
// true well before the cap and back to false once the peer catches up.
func TestOutboundWriter_Backlogged(t *testing.T) {
	client, server := net.Pipe() // peer does not read yet
	t.Cleanup(func() { client.Close(); server.Close() })
	// A generous deadline so the writer goroutine stays parked in its
	// first conn.Write: the queue must fill, not fail.
	o := newOutboundWriter(server, 5*time.Second, discardLogger())

	if o.Backlogged() {
		t.Fatal("empty writer reports backlogged")
	}

	frame := bytes.Repeat([]byte{0xCD}, 8<<10) // 8 KiB
	// Just over the byte high-water and far below both hard caps, so this
	// pins the soft threshold and not the overflow path. Two frames of
	// slack cover the one the writer goroutine has already dequeued.
	frames := outboundHighWater/len(frame) + 2
	for range frames {
		if _, err := o.Write(frame); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	if frames >= maxOutboundQueueSlots || frames*len(frame) >= maxOutboundQueueBytes {
		t.Fatalf("test writes %d frames — that reaches a hard cap, not the soft one", frames)
	}
	if !o.Backlogged() {
		t.Fatalf("writer with %d queued bytes is not backlogged (high-water %d)",
			frames*len(frame), outboundHighWater)
	}

	// Now let the peer drain; the writer must fall back below the mark.
	readDone := make(chan struct{})
	go func() { defer close(readDone); _, _ = io.Copy(io.Discard, client) }()

	deadline := time.Now().Add(5 * time.Second)
	for o.Backlogged() {
		if time.Now().After(deadline) {
			t.Fatal("still backlogged after the peer drained the queue")
		}
		time.Sleep(time.Millisecond)
	}

	o.Close()
	client.Close()
	<-readDone
}
