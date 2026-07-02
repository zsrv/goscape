package world

import (
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"
)

// newHandleConnTestServer builds the minimal Server literal exercised by
// HandleConn: a quit channel, a discard base logger fanned out via
// initChildLoggers (so logNet is non-nil, matching
// TestServeConn_ContainsPanicAndReleasesWaitGroup in
// server_recover_test.go), and a Config with NodeDebugSocket set so the
// read deadline is skipped and the read loop reaches conn.Read
// deterministically — see server_recover_test.go's comment.
func newHandleConnTestServer(t *testing.T) *Server {
	t.Helper()
	s := &Server{
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		cfg:  Config{NodeDebugSocket: true},
		quit: make(chan interface{}),
	}
	s.initChildLoggers(s.log)
	return s
}

// panicReadFakeConn panics on first Read — a stand-in for any connection
// whose bytes drive the RS2 packet readers into their documented
// panic-on-underflow behavior (gap-login-wire-1). Write/Close are
// non-blocking no-ops rather than a real net.Pipe peer: at rev-225 (pre
// rev-244 B3), handleTCPConn still sends an 8-byte connect-time seed
// before entering its read loop (TcpServer.ts:24-27), so a net.Pipe conn
// with a non-draining peer would block forever on that seed write and
// never reach the Read that panics.
type panicReadFakeConn struct{}

func (panicReadFakeConn) Read([]byte) (int, error)         { panic("malformed login packet") }
func (panicReadFakeConn) Write(b []byte) (int, error)      { return len(b), nil }
func (panicReadFakeConn) Close() error                     { return nil }
func (panicReadFakeConn) LocalAddr() net.Addr              { return dummyAddr{} }
func (panicReadFakeConn) RemoteAddr() net.Addr             { return dummyAddr{} }
func (panicReadFakeConn) SetDeadline(time.Time) error      { return nil }
func (panicReadFakeConn) SetReadDeadline(time.Time) error  { return nil }
func (panicReadFakeConn) SetWriteDeadline(time.Time) error { return nil }

func TestHandleConn_ContainsPanic(t *testing.T) {
	s := newHandleConnTestServer(t)

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.HandleConn(panicReadFakeConn{}) // pre-fix: panics the test binary
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("HandleConn did not return")
	}
}

func TestHandleConn_QuitGate(t *testing.T) {
	s := newHandleConnTestServer(t)
	close(s.quit)
	client, server := net.Pipe()
	defer client.Close()

	done := make(chan struct{})
	go func() { defer close(done); s.HandleConn(server) }()
	select {
	case <-done: // returned without touching tcpWg
	case <-time.After(2 * time.Second):
		t.Fatal("HandleConn did not honor closed quit channel")
	}
	// server side must be closed: a read on the peer should error out.
	client.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 1)
	if _, err := client.Read(buf); err == nil {
		t.Error("conn should be closed when quit is already closed")
	}
}

// eofConn is a net.Conn whose Read returns io.EOF immediately, so
// handleTCPConn's read loop exits on first read and serveConn returns
// promptly. Used to churn many short-lived HandleConn calls.
type eofConn struct{}

func (eofConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (eofConn) Write(b []byte) (int, error)      { return len(b), nil }
func (eofConn) Close() error                     { return nil }
func (eofConn) LocalAddr() net.Addr              { return dummyAddr{} }
func (eofConn) RemoteAddr() net.Addr             { return dummyAddr{} }
func (eofConn) SetDeadline(time.Time) error      { return nil }
func (eofConn) SetReadDeadline(time.Time) error  { return nil }
func (eofConn) SetWriteDeadline(time.Time) error { return nil }

// TestHandleConn_ShutdownRace exercises the admissionGateMu admission gate:
// many HandleConn calls race one shutdown transition, all released by a
// single start channel (no sleeps). The shutdown goroutine mirrors exactly
// the gate-relevant slice of Server.Shutdown — close(s.quit) under
// admissionGateMu, then tcpWg.Wait() — because full Shutdown needs a live
// tcpListener and tick plumbing the minimal test Server lacks. Pre-mutex,
// this interleaving could Add(1) concurrently with a Wait that had observed
// a transient zero counter (WaitGroup misuse: possible panic, early Wait
// return, and a data race under -race). The test asserts no panic, no
// race, and that Wait completes — i.e. every admitted connection is
// observed by the Wait.
func TestHandleConn_ShutdownRace(t *testing.T) {
	s := newHandleConnTestServer(t)

	const conns = 64
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range conns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			s.HandleConn(eofConn{})
		}()
	}

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-start
		s.admissionGateMu.Lock()
		close(s.quit)
		s.admissionGateMu.Unlock()
		s.tcpWg.Wait()
	}()

	close(start)
	wg.Wait()
	select {
	case <-shutdownDone:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown-side tcpWg.Wait did not complete")
	}
}
