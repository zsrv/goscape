// Package connhandler exposes a minimal interface for handing accepted
// network connections to the world module's TCP connection handler.
// Defined outside both modules so the asset module can depend on the
// interface without importing all of modules/world.
package connhandler

import "net"

// ConnHandler is implemented by anything that can accept a net.Conn and
// drive the world's per-connection state machine. The implementation
// MUST take ownership of conn (including closing it). The call MAY block
// for the lifetime of the connection.
type ConnHandler interface {
	HandleConn(conn net.Conn)
}
