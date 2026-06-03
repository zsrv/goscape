package world

import "net"

// HandleConn satisfies pkg/world/connhandler.ConnHandler by delegating
// to the unexported per-connection state machine. The ondemand module's
// WebSocket bridge uses this entry point so that WS-framed connections
// flow through the same login → game pipeline as raw TCP.
//
// HandleConn takes ownership of conn (including closing it on return)
// and blocks for the lifetime of the connection.
func (s *Server) HandleConn(conn net.Conn) {
	s.tcpWg.Add(1)
	defer s.tcpWg.Done()
	s.handleTCPConn(conn)
}
