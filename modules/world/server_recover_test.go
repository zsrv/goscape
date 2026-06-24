package world

import (
	"io"
	"log/slog"
	"net"
	"testing"
	"time"
)

// panicOnReadConn is a net.Conn whose Read panics, simulating any panic that
// can originate while a per-connection goroutine decodes client data — most
// importantly the unauthenticated login-RSA under-read panic of
// gap-login-wire-1 (see req.TestUnmarshalBinary_TruncatedRSABlockPanics).
// Writes are discarded. At rev-244 there is no connect-time seed send
// (removed per TcpServer.ts:9aadcec4), so handleTCPConn goes straight to
// the read loop; the first Read panics and is caught by serveConn's recover.
type panicOnReadConn struct{}

func (panicOnReadConn) Read([]byte) (int, error)         { panic("simulated decode panic") }
func (panicOnReadConn) Write(b []byte) (int, error)      { return len(b), nil }
func (panicOnReadConn) Close() error                     { return nil }
func (panicOnReadConn) LocalAddr() net.Addr              { return dummyAddr{} }
func (panicOnReadConn) RemoteAddr() net.Addr             { return dummyAddr{} }
func (panicOnReadConn) SetDeadline(time.Time) error      { return nil }
func (panicOnReadConn) SetReadDeadline(time.Time) error  { return nil }
func (panicOnReadConn) SetWriteDeadline(time.Time) error { return nil }

type dummyAddr struct{}

func (dummyAddr) Network() string { return "tcp" }
func (dummyAddr) String() string  { return "test:0" }

// TestServeConn_ContainsPanicAndReleasesWaitGroup pins the gap-login-wire-1
// fix: a panic raised anywhere inside per-connection handling (here, a read
// panic standing in for the unauthenticated login-RSA under-read) must be
// contained to that one connection — it must NOT propagate out of serveConn
// and crash the whole world process — and the tcpWg accounting that Shutdown
// waits on must still be released. TS isolates per-connection via
// try/catch -> client.terminate() (TcpServer.ts:29-41).
func TestServeConn_ContainsPanicAndReleasesWaitGroup(t *testing.T) {
	s := &Server{
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		cfg: Config{
			// Skip the read deadline so the read loop reaches conn.Read and
			// panics deterministically rather than tripping a timeout first.
			NodeDebugSocket: true,
		},
	}
	s.initChildLoggers(s.log)

	s.tcpWg.Add(1)

	returned := make(chan struct{})
	go func() {
		// If serveConn does not recover, this panic propagates and crashes the
		// test binary (mirroring the production whole-server crash).
		s.serveConn(panicOnReadConn{})
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("serveConn did not return after a connection-handling panic")
	}

	waited := make(chan struct{})
	go func() {
		s.tcpWg.Wait()
		close(waited)
	}()
	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("tcpWg was not released after serveConn handled a panic")
	}
}
