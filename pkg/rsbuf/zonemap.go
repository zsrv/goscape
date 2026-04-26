package rsbuf

// zone holds the player + npc id sets for a single 8x8-tile zone.
// Mirrors upstream grid.rs Zone (2004scape/rsbuf branch 225,
// src/grid.rs:4-37).
//
// Concurrency: tick-goroutine-owned. No internal synchronization.
type zone struct {
	players map[int32]struct{} // pids
	npcs    map[int32]struct{} // nids
}

func newZone() *zone {
	return &zone{
		players: map[int32]struct{}{},
		npcs:    map[int32]struct{}{},
	}
}

// AddPlayer registers pid in this zone. Idempotent.
func (z *zone) AddPlayer(pid int32) { z.players[pid] = struct{}{} }

// RemovePlayer unregisters pid from this zone. No-op if absent.
func (z *zone) RemovePlayer(pid int32) { delete(z.players, pid) }

// AddNpc registers nid in this zone. Idempotent.
func (z *zone) AddNpc(nid int32) { z.npcs[nid] = struct{}{} }

// RemoveNpc unregisters nid from this zone. No-op if absent.
func (z *zone) RemoveNpc(nid int32) { delete(z.npcs, nid) }

// zoneMap is the rsbuf-internal spatial index keyed by packed zone
// index. Mirrors upstream grid.rs ZoneMap (src/grid.rs:39-75).
//
// Coord packing (matches upstream grid.rs:54-58 ZoneMap::zone_index
// AND goscape's pkg/coordgrid.ZoneIndex):
//
//	((x >> 3) & 0x7ff) | (((z >> 3) & 0x7ff) << 11) | ((level & 0x3) << 22)
//
// Concurrency: tick-goroutine-owned.
type zoneMap struct {
	zones map[uint32]*zone
}

func newZoneMap() *zoneMap {
	return &zoneMap{
		zones: map[uint32]*zone{},
	}
}

// Zone returns the zone at (x, level, z), creating an empty zone on miss
// and caching it in the map. Mirrors upstream ZoneMap::zone at
// grid.rs:69-74.
//
// Argument order matches goscape's pkg/coordgrid convention (level
// before z) rather than upstream's (x, y, z). The packed key is
// identical.
func (m *zoneMap) Zone(x, level, z int) *zone {
	key := zoneKey(x, level, z)
	if zn := m.zones[key]; zn != nil {
		return zn
	}
	zn := newZone()
	m.zones[key] = zn
	return zn
}

// zoneKey returns the packed zone index. Equivalent to
// pkg/coordgrid.ZoneIndex(x, z, level) but pinned here for
// upstream-side-by-side review against grid.rs:54-58.
func zoneKey(x, level, z int) uint32 {
	return uint32((x>>3)&0x7ff) | uint32((z>>3)&0x7ff)<<11 | uint32(level&0x3)<<22
}
