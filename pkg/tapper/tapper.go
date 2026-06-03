package tapper

import "time"

// Direction discriminates packet flow direction for both the OTel
// direction attribute and the proto's PacketEvent.dir field.
type Direction uint32

const (
	DirIn  Direction = 0
	DirOut Direction = 1
)

// DirInbound / DirOutbound are the OTel attribute values for a Direction. They
// live with the public Direction type because String() (a method on Direction)
// must be declared in the same package as the type.
const (
	DirInbound  = "inbound"
	DirOutbound = "outbound"
)

// String returns the OTel attribute value matching the direction.
func (d Direction) String() string {
	if d == DirOut {
		return DirOutbound
	}
	return DirInbound
}

// CloseReason sentinel values carried in SessionEnded events.
const (
	CloseReasonLogout     = "logout"
	CloseReasonDisconnect = "disconnect"
	CloseReasonKick       = "kick"
	CloseReasonCrash      = "crash"
)

// Tapper is the seam the world module taps. The public no-op
// discards everything; a downstream build installs the real implementation.
type Tapper interface {
	// Enabled reports whether the tap is active; callers use it to skip
	// per-session and per-packet work entirely when the tap is off. The public
	// no-op default returns false, preserving the zero-overhead disabled path.
	Enabled() bool
	SessionStarted(accountID int64, sessionID string, ts time.Time)
	Tap(accountID int64, sessionID string, dir Direction, opcode uint8, payload []byte, ts time.Time)
	SessionEnded(accountID int64, sessionID string, ts time.Time, closeReason string)
}

type noopTapper struct{}

func (noopTapper) Enabled() bool                                          { return false }
func (noopTapper) SessionStarted(int64, string, time.Time)                {}
func (noopTapper) Tap(int64, string, Direction, uint8, []byte, time.Time) {}
func (noopTapper) SessionEnded(int64, string, time.Time, string)          {}

// NoopTapper returns a Tapper that discards all events (public default).
func NoopTapper() Tapper { return noopTapper{} }
