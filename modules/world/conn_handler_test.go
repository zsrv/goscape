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
		quit: make(chan struct{}),
	}
	s.initChildLoggers(s.log)
	return s
}

// panicReadConn panics on first Read — a stand-in for any connection
// whose bytes drive the RS2 packet readers into their documented
// panic-on-underflow behavior (gap-login-wire-1).
type panicReadConn struct{ net.Conn }

func (panicReadConn) Read([]byte) (int, error) { panic("malformed login packet") }

func TestHandleConn_ContainsPanic(t *testing.T) {
	s := newHandleConnTestServer(t)
	client, server := net.Pipe()
	defer client.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.HandleConn(panicReadConn{server}) // pre-fix: panics the test binary
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

// TestHandleConn_ShutdownRace exercises the admissionGateMu admission gate: many
// HandleConn calls race one shutdown transition, all released by a single
// start channel (no sleeps). The shutdown goroutine mirrors exactly the
// gate-relevant slice of Server.Shutdown — close(s.quit) under admissionGateMu,
// then tcpWg.Wait() — because full Shutdown needs a live tcpListener and
// tick plumbing the minimal test Server lacks. Pre-mutex, this interleaving
// could Add(1) concurrently with a Wait that had observed a transient zero
// counter (WaitGroup misuse: possible panic, early Wait return, and a data
// race under -race). The test asserts no panic, no race, and that Wait
// completes — i.e. every admitted connection is observed by the Wait.
func TestHandleConn_ShutdownRace(t *testing.T) {
	s := newHandleConnTestServer(t)

	const conns = 64
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range conns {
		wg.Go(func() {
			<-start
			s.HandleConn(eofConn{})
		})
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
