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

// SEC1 M-2 / DEVIATION SEC1-D3: a peer that never reads must not block
// the writer's caller (the tick goroutine). The queue absorbs writes
// instantly and the connection is closed once a cap is exceeded.
func TestOutboundWriter_NeverBlocksCaller(t *testing.T) {
	client, server := net.Pipe() // client side never reads
	t.Cleanup(func() { client.Close(); server.Close() })
	// A generous per-write deadline (the production default) on purpose:
	// the failure this pins is the *queue cap*, so the writer goroutine
	// must still be parked in its first conn.Write when the caps are hit.
	// A short deadline would let the goroutine fail on its own and the
	// loop would exit on net.ErrClosed instead of errOutboundFull. It also
	// sharpens the blocking check below — a synchronous writer would hang
	// on the very first frame for 2s, far past the 500ms budget.
	o := newOutboundWriter(server, 2*time.Second, discardLogger())

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
	// Peer observes the close: the read end errors out promptly.
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := client.Read(buf); err != nil {
			return // closed pipe (io.EOF or io.ErrClosedPipe)
		}
	}
	t.Fatal("peer never saw the connection close")
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

// closeSignalConn reports when the connection is closed. Everything else
// (including SetWriteDeadline semantics) is the embedded net.Pipe end, so
// the stall this drives is a real deadline expiry, not a simulated one.
type closeSignalConn struct {
	net.Conn
	once   sync.Once
	closed chan struct{}
}

func (c *closeSignalConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return c.Conn.Close()
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
	sc := &closeSignalConn{Conn: server, closed: make(chan struct{})}
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
