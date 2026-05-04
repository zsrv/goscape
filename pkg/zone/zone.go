package zone

import (
	"iter"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/rsbuf"
)

// PlayerLike is the minimum surface Zone needs from a player-like entity.
// Defined here (rather than imported from modules/world) to avoid a cyclic
// import — modules/world imports pkg/zone, not the reverse.
//
// Mirrors TS Player's role inside Zone.enter / Zone.leave at Zone.ts:80-83
// (only IsValid + identity are needed; richer accessors stay in modules/world).
type PlayerLike interface {
	IsValid() bool
	Slot() int
}

// NpcLike is the minimum surface Zone needs from an npc-like entity.
// Same cyclic-import rationale as PlayerLike.
//
// Mirrors TS Npc's role inside Zone.enter / Zone.leave at Zone.ts:84-87.
type NpcLike interface {
	IsValid() bool
	Nid() int
}

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

	// PathingEntity subscription lists. Per TS Zone.ts:47-48, players and
	// npcs are tracked separately. Reset does NOT clear these — subscription
	// persists across ticks until LeaveX is called explicitly.
	players DoublyLinkList[PlayerLike]
	npcs    DoublyLinkList[NpcLike]
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
// processCleanup at the end of each tick. Locs and Objs persist. The
// players/npcs subscription lists also persist — they are managed via
// EnterX/LeaveX, not per-tick (mirrors TS Zone.reset at Zone.ts:197-201).
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

// encodeNested builds [opcode, ...payload] by calling the rsbuf encoder
// into a fresh Packet. Returns a newly-owned byte slice.
func encodeNested(opcode byte, fn func(*packet.Packet)) []byte {
	buf := packet.NewPacket(nil)
	buf.P1(opcode)
	fn(buf)
	return append([]byte(nil), buf.Data...)
}

// ---- loc mutations ----

// AddStaticLoc appends a static (LifecycleRespawn) loc to z.Locs WITHOUT
// queuing a zone event. Statics are delivered to clients via the mapsquare
// download (sub-spec 5b), not via zone events. Called once per loc during
// world init.
func (z *Zone) AddStaticLoc(loc *entity.Loc) {
	z.Locs = append(z.Locs, loc)
	loc.IsActive = true
}

// AddLoc activates a loc and queues a LOC_ADD_CHANGE Enclosed event. For
// dynamic (Despawn-lifecycle) locs the pointer is appended to z.Locs so
// the full-follows replay in 4b-4 can iterate active dynamics.
func (z *Zone) AddLoc(loc *entity.Loc) {
	coord := coordgrid.PackZoneCoord(loc.X, loc.Z)
	bytes := encodeNested(rsbuf.ZoneOpLocAddChange, func(buf *packet.Packet) {
		rsbuf.EncodeLocAddChange(buf, coord, loc.Shape(), loc.Angle(), loc.Type())
	})
	if loc.Lifecycle == entity.LifecycleDespawn {
		z.Locs = append(z.Locs, loc)
	}
	z.queueEvent(&loc.NonPathing, ZoneEvent{
		Type:       ZoneEventEnclosed,
		ReceiverID: PublicReceiver,
		Bytes:      bytes,
	})
}

// ChangeLoc emits a LOC_ADD_CHANGE for an existing active loc whose
// type/shape/angle changed. Does not modify Locs.
func (z *Zone) ChangeLoc(loc *entity.Loc) {
	coord := coordgrid.PackZoneCoord(loc.X, loc.Z)
	bytes := encodeNested(rsbuf.ZoneOpLocAddChange, func(buf *packet.Packet) {
		rsbuf.EncodeLocAddChange(buf, coord, loc.Shape(), loc.Angle(), loc.Type())
	})
	z.queueEvent(&loc.NonPathing, ZoneEvent{
		Type:       ZoneEventEnclosed,
		ReceiverID: PublicReceiver,
		Bytes:      bytes,
	})
}

// RemoveLoc removes a dynamic loc from z.Locs (if Despawn lifecycle),
// tombstones its pending events, and queues a LOC_DEL Enclosed event.
func (z *Zone) RemoveLoc(loc *entity.Loc) {
	coord := coordgrid.PackZoneCoord(loc.X, loc.Z)
	if loc.Lifecycle == entity.LifecycleDespawn {
		for i, l := range z.Locs {
			if l == loc {
				z.Locs = append(z.Locs[:i], z.Locs[i+1:]...)
				break
			}
		}
	}
	z.clearQueuedEvents(&loc.NonPathing)
	bytes := encodeNested(rsbuf.ZoneOpLocDel, func(buf *packet.Packet) {
		rsbuf.EncodeLocDel(buf, coord, loc.Shape(), loc.Angle())
	})
	z.queueEvent(&loc.NonPathing, ZoneEvent{
		Type:       ZoneEventEnclosed,
		ReceiverID: PublicReceiver,
		Bytes:      bytes,
	})
}

// AnimLoc queues a LOC_ANIM event. Does not modify Locs.
func (z *Zone) AnimLoc(loc *entity.Loc, seq int) {
	coord := coordgrid.PackZoneCoord(loc.X, loc.Z)
	bytes := encodeNested(rsbuf.ZoneOpLocAnim, func(buf *packet.Packet) {
		rsbuf.EncodeLocAnim(buf, coord, loc.Shape(), loc.Angle(), seq)
	})
	z.queueEvent(&loc.NonPathing, ZoneEvent{
		Type:       ZoneEventEnclosed,
		ReceiverID: PublicReceiver,
		Bytes:      bytes,
	})
}

// MergeLoc queues a LOC_MERGE event. east/south/west/north are absolute
// tile coordinates; the encoder stores deltas relative to srcX/srcZ.
func (z *Zone) MergeLoc(
	loc *entity.Loc,
	playerSlot, startCycle, endCycle int,
	east, south, west, north int,
) {
	coord := coordgrid.PackZoneCoord(loc.X, loc.Z)
	bytes := encodeNested(rsbuf.ZoneOpLocMerge, func(buf *packet.Packet) {
		rsbuf.EncodeLocMerge(buf, coord,
			loc.Shape(), loc.Angle(), loc.Type(),
			startCycle, endCycle, playerSlot,
			east-loc.X, south-loc.Z, west-loc.X, north-loc.Z)
	})
	z.queueEvent(&loc.NonPathing, ZoneEvent{
		Type:       ZoneEventEnclosed,
		ReceiverID: PublicReceiver,
		Bytes:      bytes,
	})
}

// ---- obj mutations ----

// AddObj appends a dynamic obj to z.Objs and queues an OBJ_ADD event.
// Enclosed if receiverID == PublicReceiver; Follows otherwise.
// TODO(beyond-4b): enforce per-zone obj cap (TS: OBJS = 129) with
// oldest-obj eviction.
func (z *Zone) AddObj(obj *entity.Obj, receiverID int) {
	coord := coordgrid.PackZoneCoord(obj.X, obj.Z)
	if obj.Lifecycle == entity.LifecycleDespawn {
		z.Objs = append(z.Objs, obj)
	}
	bytes := encodeNested(rsbuf.ZoneOpObjAdd, func(buf *packet.Packet) {
		rsbuf.EncodeObjAdd(buf, coord, obj.Type, obj.Count)
	})
	evType := ZoneEventEnclosed
	if receiverID != PublicReceiver {
		evType = ZoneEventFollows
	}
	z.queueEvent(&obj.NonPathing, ZoneEvent{
		Type:       evType,
		ReceiverID: receiverID,
		Bytes:      bytes,
	})
}

// ChangeObj updates obj.Count + LastChange and queues a FOLLOWS OBJ_COUNT
// event routed to the obj's current receiver.
func (z *Zone) ChangeObj(obj *entity.Obj, oldCount, newCount, currentTick int) {
	obj.Count = newCount
	obj.LastChange = currentTick
	coord := coordgrid.PackZoneCoord(obj.X, obj.Z)
	bytes := encodeNested(rsbuf.ZoneOpObjCount, func(buf *packet.Packet) {
		rsbuf.EncodeObjCount(buf, coord, obj.Type, oldCount, newCount)
	})
	z.queueEvent(&obj.NonPathing, ZoneEvent{
		Type:       ZoneEventFollows,
		ReceiverID: obj.ReceiverID,
		Bytes:      bytes,
	})
}

// RemoveObj removes a dynamic obj from z.Objs, tombstones pending events,
// and queues an OBJ_DEL event — unless this same tick already transitioned
// the obj's lifecycle (matches TS's lastLifecycleTick === currentTick check).
func (z *Zone) RemoveObj(obj *entity.Obj, currentTick int) {
	coord := coordgrid.PackZoneCoord(obj.X, obj.Z)
	if obj.Lifecycle == entity.LifecycleDespawn {
		for i, o := range z.Objs {
			if o == obj {
				z.Objs = append(z.Objs[:i], z.Objs[i+1:]...)
				break
			}
		}
	}
	z.clearQueuedEvents(&obj.NonPathing)
	if obj.LastLifecycleTick == currentTick {
		return
	}
	bytes := encodeNested(rsbuf.ZoneOpObjDel, func(buf *packet.Packet) {
		rsbuf.EncodeObjDel(buf, coord, obj.Type)
	})
	evType := ZoneEventEnclosed
	receiver := PublicReceiver
	if obj.Lifecycle == entity.LifecycleDespawn && obj.ReceiverID != PublicReceiver {
		evType = ZoneEventFollows
		receiver = obj.ReceiverID
	}
	z.queueEvent(&obj.NonPathing, ZoneEvent{
		Type:       evType,
		ReceiverID: receiver,
		Bytes:      bytes,
	})
}

// RevealObj transitions a private drop to public and queues an ENCLOSED
// OBJ_REVEAL event. Simplified vs TS — does not consult ObjType
// tradeable/members flags.
// TODO(beyond-4b): wire tradeability gating.
func (z *Zone) RevealObj(obj *entity.Obj, receiverSlot int) {
	obj.ReceiverID = PublicReceiver
	obj.Reveal = -1
	obj.LastChange = -1
	coord := coordgrid.PackZoneCoord(obj.X, obj.Z)
	bytes := encodeNested(rsbuf.ZoneOpObjReveal, func(buf *packet.Packet) {
		rsbuf.EncodeObjReveal(buf, coord, obj.Type, obj.Count, receiverSlot)
	})
	z.queueEvent(&obj.NonPathing, ZoneEvent{
		Type:       ZoneEventEnclosed,
		ReceiverID: PublicReceiver,
		Bytes:      bytes,
	})
}

// ---- non-entity events ----

// AnimMap queues a MAP_ANIM event at (x, zCoord).
func (z *Zone) AnimMap(x, zCoord, spotanim, height, delay int) {
	coord := coordgrid.PackZoneCoord(x, zCoord)
	bytes := encodeNested(rsbuf.ZoneOpMapAnim, func(buf *packet.Packet) {
		rsbuf.EncodeMapAnim(buf, coord, spotanim, height, delay)
	})
	z.queueEvent(nil, ZoneEvent{
		Type:       ZoneEventEnclosed,
		ReceiverID: PublicReceiver,
		Bytes:      bytes,
	})
}

// MapProjAnim queues a MAP_PROJANIM event for a projectile traveling
// (srcX, srcZ) → (dstX, dstZ).
func (z *Zone) MapProjAnim(
	srcX, srcZ, dstX, dstZ int,
	target, spotanim, srcHeight, dstHeight int,
	startDelay, endDelay, peak, arc int,
) {
	coord := coordgrid.PackZoneCoord(srcX, srcZ)
	bytes := encodeNested(rsbuf.ZoneOpMapProjAnim, func(buf *packet.Packet) {
		rsbuf.EncodeMapProjAnim(buf, coord,
			dstX-srcX, dstZ-srcZ,
			target, spotanim,
			srcHeight, dstHeight,
			startDelay, endDelay,
			peak, arc)
	})
	z.queueEvent(nil, ZoneEvent{
		Type:       ZoneEventEnclosed,
		ReceiverID: PublicReceiver,
		Bytes:      bytes,
	})
}

// ---- PathingEntity subscription (NAI-28) ----

// EnterPlayer adds p to z.players and returns the *Element for caller storage.
// If z's player count transitions 0→1, grid.Flag(z.X, z.Z) fires.
//
// Mirrors TS Zone.enter Player branch at Zone.ts:80-83.
func (z *Zone) EnterPlayer(p PlayerLike, grid *ZoneGrid) *Element[PlayerLike] {
	wasEmpty := z.players.Size() == 0
	e := z.players.AddTail(p)
	if wasEmpty && grid != nil {
		grid.Flag(z.X, z.Z)
	}
	return e
}

// LeavePlayer removes the element from z.players. If z's player count
// transitions 1→0, grid.Unflag(z.X, z.Z) fires. Caller must null its
// stored *Element after this call.
//
// Mirrors TS Zone.leave Player branch at Zone.ts:90-96.
func (z *Zone) LeavePlayer(p PlayerLike, e *Element[PlayerLike], grid *ZoneGrid) {
	if e == nil {
		return
	}
	e.Unlink()
	if z.players.Size() == 0 && grid != nil {
		grid.Unflag(z.X, z.Z)
	}
}

// EnterNpc adds n to z.npcs and returns the *Element for caller storage.
// NPC entries do NOT touch the grid (only player entries do).
//
// Mirrors TS Zone.enter Npc branch at Zone.ts:84-87.
func (z *Zone) EnterNpc(n NpcLike) *Element[NpcLike] {
	return z.npcs.AddTail(n)
}

// LeaveNpc removes the element from z.npcs.
//
// Mirrors TS Zone.leave Npc branch at Zone.ts:97-99.
func (z *Zone) LeaveNpc(n NpcLike, e *Element[NpcLike]) {
	if e == nil {
		return
	}
	e.Unlink()
}

// PlayersSafe yields players that pass IsValid(). reverse=true iterates
// in reverse insertion order. Mirrors TS Zone.getAllPlayersSafe at Zone.ts:387-393.
func (z *Zone) PlayersSafe(reverse bool) iter.Seq[PlayerLike] {
	return func(yield func(PlayerLike) bool) {
		for p := range z.players.All(reverse) {
			if !p.IsValid() {
				continue
			}
			if !yield(p) {
				return
			}
		}
	}
}

// NpcsSafe yields npcs that pass IsValid(). Mirrors TS Zone.getAllNpcsSafe
// at Zone.ts:399-405.
func (z *Zone) NpcsSafe(reverse bool) iter.Seq[NpcLike] {
	return func(yield func(NpcLike) bool) {
		for n := range z.npcs.All(reverse) {
			if !n.IsValid() {
				continue
			}
			if !yield(n) {
				return
			}
		}
	}
}

// PlayersCount returns the number of players currently subscribed.
// Mirrors TS Zone.playersCount field at Zone.ts:51.
func (z *Zone) PlayersCount() int { return z.players.Size() }

// NpcsCount returns the number of npcs currently subscribed.
// Mirrors TS Zone.npcsCount field at Zone.ts:52.
func (z *Zone) NpcsCount() int { return z.npcs.Size() }
