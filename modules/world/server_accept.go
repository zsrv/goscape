package world

import (
	"errors"
	"fmt"
	"io"
	"net"
	"runtime/debug"
	"time"

	"github.com/zsrv/goscape/pkg/io/protocol"
	"github.com/zsrv/goscape/pkg/tapper"
	applog "github.com/zsrv/goscape/pkg/util/log"
)

// trackConn registers an accepted connection so Shutdown can close it.
// The read loop re-arms its deadline on every read, so without an
// explicit close a chatty client keeps its goroutine alive and
// tcpWg.Wait never returns.
//
// Called at the two admission sites (serveTCP's accept loop; HandleConn),
// both under admissionGateMu and synchronous with the matching
// tcpWg.Add(1) — NOT inside the spawned serveConn goroutine. The gate
// makes admission atomic with Shutdown's close(quit)+closeLiveConns, so
// closeLiveConns can never miss a connection that Wait() would block on:
// a chatty client re-arms its read deadline forever, so an untracked conn
// wedges Shutdown even in production.
func (s *Server) trackConn(conn net.Conn) {
	s.liveConnsMu.Lock()
	if s.liveConns == nil {
		s.liveConns = make(map[net.Conn]struct{})
	}
	s.liveConns[conn] = struct{}{}
	s.liveConnsMu.Unlock()
}

// untrackConn removes a connection from the live registry. Called from
// serveConn's teardown, after which Shutdown's closeLiveConns will no
// longer see (or double-close) this conn.
func (s *Server) untrackConn(conn net.Conn) {
	s.liveConnsMu.Lock()
	delete(s.liveConns, conn)
	s.liveConnsMu.Unlock()
}

// closeLiveConns closes every currently-tracked connection. Safe to call
// concurrently with trackConn/untrackConn: each closed conn's read loop
// exits through its normal error path and untracks itself independently.
//
// The registry is snapshotted under liveConnsMu but the Close calls run
// AFTER unlocking: Close is not guaranteed cheap (the WS bridge's NetConn
// performs a close handshake with an internal timeout), and holding the
// lock through it would serialize every exiting conn goroutine's
// untrackConn behind that handshake.
func (s *Server) closeLiveConns() {
	s.liveConnsMu.Lock()
	conns := make([]net.Conn, 0, len(s.liveConns))
	for conn := range s.liveConns {
		conns = append(conns, conn)
	}
	s.liveConnsMu.Unlock()
	for _, conn := range conns {
		_ = conn.Close()
	}
}

func (s *Server) serveTCP() error {
	// Shutdown is the primary listener owner (nil-guarded, under the admission
	// gate); this defer additionally covers a serveTCP accept error that
	// returns before Shutdown runs. The resulting double-Close is harmless
	// (the second returns ErrClosed, which callers ignore).
	defer s.tcpListener.Close()
	defer s.tcpWg.Done()

	// Accept incoming connections in a loop
	// Use a for loop so the server will accept each incoming connection,
	// handle it in a goroutine, and loop back around, ready to accept
	// the next connection
	for {
		// conn underlying type is net.TCPConn
		conn, err := s.tcpListener.Accept()
		if err != nil {
			// handshake between server and client failed, or the listener closed
			select {
			case <-s.quit:
				s.log.Debug("tcp listener closed")
				return nil
			default:
				return fmt.Errorf("failed to accept connection: %w", err)
			}
		}

		// Admission gate, mirroring HandleConn: atomic with Shutdown's
		// close(quit)+closeLiveConns, this conn is either registered in
		// tcpWg AND the live-conn registry before quit closes (so Shutdown
		// both closes it and waits for it) or refused. Without the gate, a
		// conn accepted just before closeLiveConns but tracked just after
		// is invisible to it — and a chatty client re-arms its read
		// deadline forever, so even in production such an untracked conn
		// wedges Shutdown; the gate is what makes closeLiveConns complete.
		// trackConn stays synchronous with the Add(1), BEFORE the goroutine
		// is spawned, for the same reason (arch-28.4c review).
		s.admissionGateMu.Lock()
		select {
		case <-s.quit:
			s.admissionGateMu.Unlock()
			// Shutdown already ran closeLiveConns; nothing will close this
			// conn later, so refuse it here. Return (not continue), matching
			// the loop's quit semantics above: the listener is closing, so
			// the next Accept would just error into that same return path.
			_ = conn.Close()
			s.log.Debug("refusing connection accepted during shutdown")
			return nil
		default:
		}
		s.tcpWg.Add(1)
		s.trackConn(conn)
		s.admissionGateMu.Unlock()

		// Handle the connection in a new goroutine for concurrency
		go s.serveConn(conn)
	}
}

// serveConn runs handleTCPConn for one accepted connection and accounts for it
// against tcpWg (which Shutdown waits on).
//
// gap-login-wire-1: the RS2 packet read methods (G1/G2/G4/GData/GJStrLF) panic
// on under-read rather than returning errors, so an unauthenticated, malformed
// login packet — e.g. a short/truncated RSA block — drives RSADec into a
// slice-out-of-range / io.EOF panic during login decode (see
// req.TestUnmarshalBinary_TruncatedRSABlockPanics). Without per-connection
// isolation that panic crosses the goroutine boundary and crashes the entire
// world process, dropping every connected player. The recover() below contains
// any such panic to the single offending connection: handleTCPConn's own defer
// has already run the connection teardown (player removal, flush, socket close)
// during unwinding, so this is the Go equivalent of TS's per-connection
// try/catch -> client.terminate() (TcpServer.ts:29-41). tcpWg.Done() is
// deferred so Shutdown's tcpWg.Wait() can never hang on a panicked connection.
//
// The caller must have already registered conn via trackConn, synchronously
// with its tcpWg.Add(1) (serveTCP's accept loop; HandleConn's admission
// gate) — tracking here instead would open a scheduling window where the
// conn holds a tcpWg count but is invisible to closeLiveConns, letting
// Shutdown's Wait block on a connection it cannot close (arch-28.4c
// review). untrackConn stays deferred here, pairing with tcpWg.Done: both
// fire on every exit path, including mid-panic-recovery.
func (s *Server) serveConn(conn net.Conn) {
	defer s.tcpWg.Done()
	defer s.untrackConn(conn)
	defer func() {
		if r := recover(); r != nil {
			s.logNet.Error("recovered panic in connection handler",
				"panic", r,
				"remote_addr", conn.RemoteAddr(),
				"stack", string(debug.Stack()))
		}
	}()
	s.handleTCPConn(conn)
}

func (s *Server) handleTCPConn(conn net.Conn) {
	//c := newClient(conn, s, s.log)
	c := newClient(conn, s.cfg.TCPServerWriteTimeout, s.logNet)
	c.server = s
	c.tap = s.tap

	// Fix 1: disable Nagle's algorithm so small game packets are sent immediately.
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		if err := tcpConn.SetNoDelay(true); err != nil {
			s.logNet.Warn("failed to set TCP_NODELAY", "error", err)
		}
		if s.cfg.TCPKeepAlivePeriod > 0 {
			if err := tcpConn.SetKeepAlive(true); err != nil {
				s.logNet.Warn("failed to enable TCP keepalive", "error", err)
			} else if err := tcpConn.SetKeepAlivePeriod(s.cfg.TCPKeepAlivePeriod); err != nil {
				s.logNet.Warn("failed to set TCP keepalive period", "error", err)
			}
		}
	}

	defer func() {
		if c.tap != nil && c.sessionID != "" {
			c.tap.SessionEnded(c.accountID, c.sessionID, time.Now(), tapper.CloseReasonDisconnect)
			c.sessionID = ""
		}
		if c.player != nil {
			// Post-login: the tick co-owns bufw/c.in until it processes
			// the removal — no flush here (it would race the tick's own
			// flushWrite) and no pool return (dropConnRef defers that to
			// whichever owner exits last).
			s.removePlayerOnDisconnect(c.player)
			c.player = nil
		} else if c.state != ClientStateOndemand {
			// Pre-login: this goroutine is the only writer; flush any
			// pending login reply before closing. OnDemand-state conns
			// skip it — the pump goroutine co-owns bufw via transient
			// refs (arch-29.1) and there is nothing useful to flush at
			// teardown.
			if err := c.flushWrite(); err != nil {
				s.logNet.Warn("failed to flush on connection close", "error", err, "remote_addr", conn.RemoteAddr())
			}
		}
		c.closeConn()
		c.dropConnRef()
		s.logNet.Debug("connection closed", "remote_addr", conn.RemoteAddr())
	}()

	// rev-244 B3: connect-time seed send REMOVED.
	// At 225, TcpServer.ts:24-27 sent an 8-byte seed immediately on connect.
	// At 244, TcpServer.ts has no such send (9aadcec4) — the seed is now
	// generated and sent inside the op-14 reply (World.ts:2151-2155). A fresh
	// connection receives NO unsolicited bytes.

	buf := getReadBuf64k()
	defer putReadBuf64k(buf)
	for {
		// Fix 6: skip the read deadline in debug-socket mode so long-running
		// bot/integration tests aren't killed by the normal timeout.
		if !s.cfg.NodeDebugSocket {
			if err := c.conn.SetReadDeadline(time.Now().Add(s.cfg.TCPServerReadTimeout)); err != nil {
				s.logNet.Error("failed to set read deadline", "error", err)
				return
			}
		}

		n, err := c.bufr.Read(buf)
		if err != nil {
			if err != io.EOF {
				s.logNet.Error("connection read error", "error", err)
			}
			// logger-transport-4: TS TcpServer.ts:44-67 emits an ENGINE
			// session log on every disconnect kind (close / error / timeout)
			// while a player is attached. Gating on c.player != nil mirrors
			// TS's `if (client.player)` check — pre-login disconnects have
			// no session_uuid to attach the log to.
			if c.player != nil {
				msg, extra := disconnectSessionLogEvent(err)
				c.player.AddSessionLog(LoggerEventTypeEngine, msg, extra...)
			}
			return
		}

		msg := buf[:n]
		// LOG-1: per-packet, demoted to trace (firehose).
		applog.Trace(c.log, "received data", "num_bytes", len(msg))
		applog.Trace(c.log, "received data payload", "data", fmt.Sprintf("%v", msg))

		switch c.state {
		case ClientStateLogin:
			if !c.bufferData(msg) {
				c.log.Warn("incoming buffer overflow, closing connection", "remote_addr", conn.RemoteAddr())
				return
			}
			err = c.handleData()
			if err != nil {
				if errors.Is(err, protocol.ErrPayloadTooSmall) {
					continue
				}
				if errors.Is(err, errCloseConn) {
					return
				}
				c.log.Error("handleData error, closing connection", "error", err)
				return
			}
		case ClientStateGame:
			c.inMu.Lock()
			ok := c.bufferData(msg)
			c.inMu.Unlock()
			if !ok {
				c.log.Warn("incoming buffer overflow, closing connection", "remote_addr", conn.RemoteAddr())
				return
			}
		case ClientStateOndemand:
			// rev-244 B3: op-15 transitioned this connection to OnDemand mode.
			// Route received bytes to the onDemand handler via a *clientODAdapter.
			// Per-connection buffering: accumulate msg into c.in so partial frames
			// (<4 bytes) are retained across reads, matching the consumed-contract
			// of onClientData (ondemand.go adaptation note (1)).
			// TS: TcpServer.ts:35-37 → OnDemand.onClientData(client).
			if s.onDemand == nil {
				// Defensive: only reachable on hand-built test servers that
				// never ran NewServer. Don't silently drop frames — close.
				c.log.Warn("ondemand handler nil; closing connection", "remote_addr", conn.RemoteAddr())
				return
			}
			if !c.bufferData(msg) {
				c.log.Warn("ondemand buffer overflow, closing connection", "remote_addr", conn.RemoteAddr())
				return
			}
			adapter := &clientODAdapter{c: c}
			pending := c.in.Bytes()
			consumed := s.onDemand.onClientData(adapter, pending)
			if consumed > 0 {
				// Advance Pos by consumed bytes; Next() is the Packet equivalent
				// of discard — it returns the slice and advances Pos.
				c.in.Next(consumed)
			}
		}
	}
}
