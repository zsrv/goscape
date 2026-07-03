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
// A full production Server (NewServer + Run(), with tick loop, login/
// friends clients, and a real cache) is disproportionate for this test: it
// exists only to prove Shutdown doesn't wedge on a chatty connection.
// tickWg/saveWg are left at their zero value (Wait() returns immediately,
// matching a Server that never started those goroutines), and
// worldEventsCancel/bridgesCancel are left nil (both guarded with a nil
// check in Shutdown). That leaves Shutdown's tcpListener.Close +
// closeLiveConns + tcpWg.Wait sequence — the part arch-28.4c actually
// changes — exercised through the genuine serveTCP/serveConn/handleTCPConn
// code path instead of a hand-rolled stand-in.
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
// on its own, and tcpWg.Wait blocks until the test's 2s bound fires.
func TestShutdownClosesChattyConn(t *testing.T) {
	s := newChattyShutdownTestServer(t)

	conn, err := net.Dial("tcp", s.tcpListener.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// rev-225 predates the rev-244 B3 removal of the connect-time seed
	// (TcpServer.ts:24-27): handleTCPConn writes an unsolicited 8-byte
	// seed before it ever reads from the client. So the read below
	// observes that seed rather than a reply to our write — its purpose
	// is only to block until the accept pipeline (Accept -> admission
	// gate -> trackConn -> serveConn -> handleTCPConn's seed flush) has
	// actually run for this connection, so the chatty goroutine below
	// only starts once we know the server is servicing it — otherwise
	// Shutdown could race the accept and this test would pass vacuously
	// regardless of the fix.
	//
	// Chatty-payload design — must stay incomplete FOREVER, not just per
	// write: bufferData APPENDS every received chunk to c.in, and
	// ClientStateLogin never consumes an incomplete packet
	// (CheckPacketLength → ErrPayloadTooSmall → continue), so repeated
	// bytes ACCUMULATE toward packet completion. A resent 2-byte header
	// {16, 200} would complete a 202-byte packet after ~101 sends (~5s at
	// 50ms), whereupon UnmarshalHeader parses garbage, the revision check
	// fails, and the conn self-closes via sendLoginError — masking a
	// reverted closeLiveConns (the original vacuous shape this test was
	// rewritten to eliminate). Instead: one OpReqInitGameConnection (16)
	// header declaring the maximum payload a -1-size login op can carry
	// (255, one-byte length prefix → full packet = 1 opcode + 1 length +
	// 255 payload = 257 bytes), then drip ONE filler byte per 50ms.
	// Arithmetic: 20 bytes/sec; even over the whole 2s Shutdown bound the
	// buffer holds 2 + ~40 = ~42 of 257 bytes — completion would need
	// ~12.75s, >6x the assertion window (and >2.5x the old 10s guard), so
	// no parse, no self-close, and each drip re-arms the 200ms read
	// deadline, keeping the conn alive until closeLiveConns kills it.
	maxHeader := []byte{16, 255}
	if _, err := conn.Write(maxHeader); err != nil {
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
	go func() { // chatty client: drips filler, re-arming the read deadline
		filler := []byte{0}
		for {
			select {
			case <-stop:
				return
			case <-time.After(50 * time.Millisecond):
				if _, err := conn.Write(filler); err != nil {
					return
				}
			}
		}
	}()
	defer close(stop)

	// 2s bound: tight enough that any self-close path slower than the drip
	// cadence (packet completion needs ~12.75s; the 200ms read deadline
	// never fires while the drip continues) cannot fake a pass — only
	// closeLiveConns actually closing the conn lets Shutdown return here.
	done := make(chan struct{})
	go func() { defer close(done); s.Shutdown() }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown wedged on a chatty connection")
	}
}
