package world

import (
	"net"
	"testing"
	"time"
)

// newChattyShutdownTestServer builds a Server minimally sufficient to run
// the real accept/serve/shutdown path end to end: a live TCP listener plus
// serveTCP/serveConn/handleTCPConn (unlike newHandleConnTestServer, which
// deliberately exercises only HandleConn's admission gate — see its
// comment on TestHandleConn_ShutdownRace explaining why a full Shutdown
// needs a live listener the minimal Server there lacks).
//
// A full production Server (NewServer + Run(), with tick loop, OnDemand
// pump, login/friends clients, and a real cache) is disproportionate for
// this test: it exists only to prove Shutdown doesn't wedge on a chatty
// connection. tickWg/odWg/saveWg are left at their zero value (Wait()
// returns immediately, matching a Server that never started those
// goroutines), and worldEventsCancel/bridgesCancel are left nil (both
// guarded with a nil check in Shutdown). That leaves Shutdown's
// tcpListener.Close + closeLiveConns + tcpWg.Wait sequence — the part
// arch-28.4c actually changes — exercised through the genuine
// serveTCP/serveConn/handleTCPConn code path instead of a hand-rolled
// stand-in.
func newChattyShutdownTestServer(t *testing.T) *Server {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &Server{
		log: discardLogger(),
		cfg: Config{
			// Short enough that a pre-fix test run (no closeLiveConns)
			// would need the chatty goroutine below to keep re-arming the
			// deadline for the whole 10s guard, but long enough that the
			// synchronous handshake below never spuriously times out.
			TCPServerReadTimeout:  200 * time.Millisecond,
			TCPServerWriteTimeout: time.Second,
		},
		quit:        make(chan struct{}),
		tcpListener: lis,
	}
	s.initChildLoggers(s.log)
	// Mirrors NewServer's floor registration: serveTCP's own
	// `defer s.tcpWg.Done()` consumes it, keeping tcpWg's counter above
	// zero for the life of the accept loop so a concurrent Shutdown can
	// never observe (and Wait on) a transient zero count.
	s.tcpWg.Add(1)
	go s.serveTCP()
	t.Cleanup(func() { lis.Close() })
	return s
}

// arch-28.4c: a client that keeps sending must not wedge Shutdown — the
// live-conn registry closes it. Pre-fix: the read deadline re-arms on
// every read, so a connection that never stops talking never errors out
// on its own, and tcpWg.Wait blocks until the test's 10s guard fires.
func TestShutdownClosesChattyConn(t *testing.T) {
	s := newChattyShutdownTestServer(t)

	conn, err := net.Dial("tcp", s.tcpListener.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// checklogin (op 14) is harmless to repeat indefinitely: an
	// incomplete/undersized read is a no-op continue and a complete one
	// is processed and replied to — neither closes the connection (unlike
	// most other opcodes in ClientStateLogin, which close on any error).
	// Send one synchronously first and wait for the reply, so the
	// goroutine below only starts once we know the server has actually
	// accepted and is servicing this connection (i.e. trackConn has
	// already run) — otherwise Shutdown could race the accept and this
	// test would pass vacuously regardless of the fix.
	checklogin := []byte{14, 0}
	if _, err := conn.Write(checklogin); err != nil {
		t.Fatalf("initial write: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	buf := make([]byte, 32)
	if n, err := conn.Read(buf); err != nil || n == 0 {
		t.Fatalf("initial handshake read: n=%d err=%v", n, err)
	}

	stop := make(chan struct{})
	go func() { // chatty client: keeps sending, re-arming the read deadline
		for {
			select {
			case <-stop:
				return
			case <-time.After(50 * time.Millisecond):
				if _, err := conn.Write(checklogin); err != nil {
					return
				}
			}
		}
	}()
	defer close(stop)

	done := make(chan struct{})
	go func() { defer close(done); s.Shutdown() }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Shutdown wedged on a chatty connection")
	}
}
