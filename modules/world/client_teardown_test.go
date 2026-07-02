package world

import (
	"errors"
	"flag"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// arch-28.4b: buffers may be pool-returned only after BOTH the conn
// goroutine and the tick have dropped their refs; each side's drop is
// idempotent. Run with -race: pre-fix the conn-side release races the
// tick-side flush.
func TestClientTeardownRefcount(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	c := newClient(c2, time.Second, slog.Default())

	if got := c.teardownRefs.Load(); got != 1 {
		t.Fatalf("fresh client refs: got %d, want 1", got)
	}
	c.teardownRefs.Add(1) // simulate successful login (tick becomes co-owner)

	var wg sync.WaitGroup
	wg.Go(func() { c.dropConnRef() })
	wg.Go(func() {
		c.bufw.WriteByte(0) // tick-side write while conn side is tearing down
		c.dropTickRef()
	})
	wg.Wait()

	if got := c.teardownRefs.Load(); got != 0 {
		t.Fatalf("refs after both drops: got %d, want 0", got)
	}
	c.dropTickRef() // double-drop must be a no-op (idle logout + disconnect)
	c.dropConnRef()
	if got := c.teardownRefs.Load(); got != 0 {
		t.Fatalf("refs after redundant drops: got %d, want 0 (no double release)", got)
	}
}

// arch-28.4d: a write failure means the socket is dead or the client
// stopped reading — close the conn so the read loop tears the connection
// down through the normal disconnect path instead of ignoring the sticky
// bufio error forever.
type errWriteConn struct {
	net.Conn
	closed atomic.Bool
}

func (c *errWriteConn) Write([]byte) (int, error)      { return 0, errors.New("stalled peer") }
func (c *errWriteConn) Close() error                   { c.closed.Store(true); return c.Conn.Close() }
func (*errWriteConn) SetWriteDeadline(time.Time) error { return nil }

func TestFlushWriteOrCloseClosesOnError(t *testing.T) {
	p1, p2 := net.Pipe()
	defer p1.Close()
	ec := &errWriteConn{Conn: p2}
	c := newClient(ec, time.Second, slog.Default())
	c.bufw.WriteByte(0xFF) // something to flush
	c.flushWriteOrClose()
	if !ec.closed.Load() {
		t.Fatal("conn not closed after flush error")
	}
}

func TestWriteTimeoutDefault(t *testing.T) {
	var cfg Config
	fs := flag.NewFlagSet("t", flag.PanicOnError)
	cfg.RegisterFlagsAndApplyDefaults(fs)
	if cfg.TCPServerWriteTimeout != 2*time.Second {
		t.Fatalf("default write timeout: got %v, want 2s", cfg.TCPServerWriteTimeout)
	}
}
