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
	// closedCh fires on the first Close. Since SEC1 M-2 the socket write
	// happens on the outbound writer's goroutine, so the close that a
	// failed write triggers is observed asynchronously — a channel, not a
	// sleep, is how the test waits for it.
	closedCh  chan struct{}
	closeOnce sync.Once
}

func newErrWriteConn(c net.Conn) *errWriteConn {
	return &errWriteConn{Conn: c, closedCh: make(chan struct{})}
}

func (c *errWriteConn) Write([]byte) (int, error) { return 0, errors.New("stalled peer") }
func (c *errWriteConn) Close() error {
	c.closed.Store(true)
	c.closeOnce.Do(func() { close(c.closedCh) })
	return c.Conn.Close()
}
func (*errWriteConn) SetWriteDeadline(time.Time) error { return nil }

func TestFlushWriteOrCloseClosesOnError(t *testing.T) {
	p1, p2 := net.Pipe()
	defer p1.Close()
	ec := newErrWriteConn(p2)
	c := newClient(ec, time.Second, slog.Default())
	c.bufw.WriteByte(0xFF) // something to flush
	// The flush itself now only hands the frame to the outbound writer;
	// the doomed socket write — and the close it triggers — happen on that
	// writer's goroutine (SEC1 M-2).
	c.flushWriteOrClose()
	select {
	case <-ec.closedCh:
	case <-time.After(3 * time.Second):
		t.Fatal("conn not closed after flush error")
	}
	if !ec.closed.Load() {
		t.Fatal("conn not closed after flush error")
	}
	// And the sticky-error path still closes: once the writer is marked
	// failed, the next flush reports net.ErrClosed and flushWriteOrClose
	// takes its close branch (idempotent — the conn is already closed).
	c.bufw.WriteByte(0xFF)
	if err := c.flushWrite(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("flush after socket failure: got %v, want net.ErrClosed", err)
	}
	c.flushWriteOrClose()
}

func TestWriteTimeoutDefault(t *testing.T) {
	var cfg Config
	fs := flag.NewFlagSet("t", flag.PanicOnError)
	cfg.RegisterFlagsAndApplyDefaults(fs)
	if cfg.TCPServerWriteTimeout != 2*time.Second {
		t.Fatalf("default write timeout: got %v, want 2s", cfg.TCPServerWriteTimeout)
	}
}

// arch-29.1: the OnDemand pump takes a transient ref around each send so
// the pooled buffers can never be returned while a frame flush is in
// flight. After the last owner drops, send must refuse with net.ErrClosed.
func TestODAdapterSendRefusedAfterTeardown(t *testing.T) {
	p1, p2 := net.Pipe()
	defer p1.Close()
	c := newClient(p2, time.Second, slog.Default())
	c.dropConnRef() // last owner out: buffers returned
	a := &clientODAdapter{c: c}
	if err := a.send([]byte{0x01}); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("send after teardown: got %v, want net.ErrClosed", err)
	}
}

// Run with -race: concurrent pump sends versus the conn-side drop must
// never touch a pool-returned buffer. Pre-fix this is the documented
// arch-28 residual data race.
func TestODAdapterSendTeardownRace(t *testing.T) {
	for range 50 {
		p1, p2 := net.Pipe()
		go func() { // keep the pipe drained so flush doesn't block
			buf := make([]byte, 256)
			for {
				if _, err := p1.Read(buf); err != nil {
					return
				}
			}
		}()
		c := newClient(p2, 50*time.Millisecond, slog.Default())
		a := &clientODAdapter{c: c}
		var wg sync.WaitGroup
		wg.Go(func() {
			for range 10 {
				if err := a.send([]byte{0xFF}); err != nil {
					return
				}
			}
		})
		wg.Go(func() { c.dropConnRef() })
		wg.Wait()
		p1.Close()
	}
}

func TestTryRefLifecycle(t *testing.T) {
	p1, p2 := net.Pipe()
	defer p1.Close()
	c := newClient(p2, time.Second, slog.Default())
	if !c.tryRef() {
		t.Fatal("tryRef on live client must succeed")
	}
	c.dropRef()     // transient ref back: 2->1
	c.dropConnRef() // 1->0, released
	if c.tryRef() {
		t.Fatal("tryRef after release must fail")
	}
}
