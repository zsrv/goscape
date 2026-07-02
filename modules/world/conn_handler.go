package world

import "net"

// HandleConn satisfies pkg/world/connhandler.ConnHandler by delegating
// to the unexported per-connection state machine. The ondemand module's
// WebSocket bridge uses this entry point so that WS-framed connections
// flow through the same login → game pipeline as raw TCP.
//
// HandleConn takes ownership of conn (including closing it on return)
// and blocks for the lifetime of the connection. Connections arriving
// after shutdown begins are closed without handling.
func (s *Server) HandleConn(conn net.Conn) {
	// Admission gate, atomic with Shutdown's close(s.quit) — both run
	// under admissionGateMu. Either this connection registers in tcpWg
	// before quit closes (so Shutdown's tcpWg.Wait observes it), or quit
	// is already closed and the connection is refused; no login flow
	// starts on a dying server. Without the mutex the check-then-Add pair
	// could interleave with Shutdown's close(quit)+Wait, and an Add
	// concurrent with a Wait that saw a transient zero counter is
	// WaitGroup misuse.
	s.admissionGateMu.Lock()
	select {
	case <-s.quit:
		s.admissionGateMu.Unlock()
		_ = conn.Close()
		return
	default:
	}
	s.tcpWg.Add(1)
	// trackConn synchronously with the Add(1), inside the gate: Shutdown
	// runs closeLiveConns after releasing admissionGateMu, so a connection
	// admitted here is either tracked before closeLiveConns iterates (and
	// gets closed) or refused above — never counted in tcpWg yet invisible
	// to closeLiveConns. Lock order is admissionGateMu → liveConnsMu
	// (trackConn takes only the leaf liveConnsMu; nothing acquires
	// admissionGateMu while holding liveConnsMu).
	s.trackConn(conn)
	s.admissionGateMu.Unlock()
	// serveConn (not handleTCPConn directly): it owns the tcpWg.Done and
	// the per-connection recover — without it a malformed WS-framed login
	// panics the whole process (gap-login-wire-1, see server.go).
	s.serveConn(conn)
}
