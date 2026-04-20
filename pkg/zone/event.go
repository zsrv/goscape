package zone

// PublicReceiver is the sentinel ReceiverID for zone events visible to
// every observer. Mirrors the TS NO_RECEIVER bigint -1n.
const PublicReceiver = -1

// ZoneEventType distinguishes events that are broadcast to every observer
// of the zone from those that are routed to a specific recipient player.
type ZoneEventType int

const (
	// ZoneEventEnclosed events are shared across every observer; they are
	// concatenated into Zone.shared by ComputeShared and then delivered
	// inside UpdateZonePartialEnclosed packets.
	ZoneEventEnclosed ZoneEventType = iota

	// ZoneEventFollows events are per-receiver; delivery code filters by
	// ReceiverID and writes each inside an UpdateZonePartialFollows wrapper.
	ZoneEventFollows
)

// ZoneEvent carries one already-encoded zone-nested message. Bytes is
// exactly [opcode_byte, ...payload] and is ready to concat into the shared
// buffer (Enclosed) or write per-player (Follows).
//
// A nil Bytes is a tombstone — produced by clearQueuedEvents when an entity
// is removed after queuing events. ComputeShared skips tombstoned entries.
type ZoneEvent struct {
	Type       ZoneEventType
	ReceiverID int // PublicReceiver = -1 for Enclosed events and public Follows
	Bytes      []byte
}
