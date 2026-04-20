package zone

import "github.com/zsrv/goscape/pkg/entity"

// Zone is an 8×8 tile region of the world. It owns the active dynamic
// entities inside the region, a per-tick event queue, and a composed
// shared buffer of every Enclosed event (built by ComputeShared).
//
// All state except Locs and Objs is per-tick; Reset clears it between ticks.
// Locs and Objs persist until the entities are explicitly removed.
type Zone struct {
	Index       int
	X, Z, Level int // X and Z in zone units (tile >> 3)

	Locs []*entity.Loc
	Objs []*entity.Obj

	events       []ZoneEvent
	entityEvents map[*entity.NonPathing][]int // entity pointer → indexes into events

	shared []byte
}

// New constructs a zone for the given packed index and (level, zoneX, zoneZ).
// Callers typically reach Zones through ZoneMap.Get rather than calling this.
func New(index, level, x, z int) *Zone {
	return &Zone{
		Index:        index,
		X:            x,
		Z:            z,
		Level:        level,
		entityEvents: make(map[*entity.NonPathing][]int),
	}
}

// Reset clears per-tick state: events, entityEvents, shared. Called from
// processCleanup at the end of each tick. Locs and Objs persist.
func (z *Zone) Reset() {
	z.events = z.events[:0]
	clear(z.entityEvents)
	z.shared = nil
}

// Shared returns the composed Enclosed-event buffer for the current tick,
// or nil if ComputeShared hasn't been called or no Enclosed events exist.
func (z *Zone) Shared() []byte { return z.shared }

// Events returns the per-tick event queue (read-only view; callers that
// need per-player Follows iteration read from this slice directly).
func (z *Zone) Events() []ZoneEvent { return z.events }

// ComputeShared concatenates the Bytes of every non-tombstoned Enclosed
// event into z.shared. Must be called once per tick before any per-player
// delivery pass. Safe on a zone with zero Enclosed events — sets shared
// to nil.
func (z *Zone) ComputeShared() {
	// First pass: compute total size to size the buffer exactly once.
	total := 0
	for i := range z.events {
		e := &z.events[i]
		if e.Type != ZoneEventEnclosed || e.Bytes == nil {
			continue
		}
		total += len(e.Bytes)
	}
	if total == 0 {
		z.shared = nil
		return
	}
	buf := make([]byte, 0, total)
	for i := range z.events {
		e := &z.events[i]
		if e.Type != ZoneEventEnclosed || e.Bytes == nil {
			continue
		}
		buf = append(buf, e.Bytes...)
	}
	z.shared = buf
}

// queueEvent appends an event and records its index under np so
// clearQueuedEvents can purge it if the entity is removed this same tick.
func (z *Zone) queueEvent(np *entity.NonPathing, e ZoneEvent) {
	idx := len(z.events)
	z.events = append(z.events, e)
	if np != nil {
		z.entityEvents[np] = append(z.entityEvents[np], idx)
	}
}

// clearQueuedEvents tombstones every event indexed by np. ComputeShared
// skips tombstones. Per-player Follows iteration must also skip nil-Bytes.
func (z *Zone) clearQueuedEvents(np *entity.NonPathing) {
	for _, idx := range z.entityEvents[np] {
		z.events[idx].Bytes = nil
	}
	delete(z.entityEvents, np)
}
