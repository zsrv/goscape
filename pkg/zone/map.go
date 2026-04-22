package zone

import "github.com/zsrv/goscape/pkg/coordgrid"

// ZoneMap is the world's collection of Zones, indexed by packed (x,z,level).
// Zones are created on first access; empty zones carry zero state and cost.
type ZoneMap struct {
	zones map[int]*Zone
	grids map[int]*ZoneGrid
}

// NewZoneMap returns an empty map.
func NewZoneMap() *ZoneMap {
	return &ZoneMap{
		zones: make(map[int]*Zone),
		grids: make(map[int]*ZoneGrid),
	}
}

// Get returns the Zone at (level, worldX, worldZ), creating it if absent.
func (m *ZoneMap) Get(level, worldX, worldZ int) *Zone {
	return m.GetByIndex(coordgrid.ZoneIndex(worldX, worldZ, level))
}

// GetByIndex returns the Zone with the given packed index, creating it if absent.
func (m *ZoneMap) GetByIndex(index int) *Zone {
	if z, ok := m.zones[index]; ok {
		return z
	}
	x, z, level := coordgrid.UnpackZoneIndex(index)
	zone := New(index, level, x>>3, z>>3)
	m.zones[index] = zone
	return zone
}

// Grid returns the per-level ZoneGrid, creating it if absent.
func (m *ZoneMap) Grid(level int) *ZoneGrid {
	if g, ok := m.grids[level]; ok {
		return g
	}
	g := NewZoneGrid()
	m.grids[level] = g
	return g
}

// ZoneCount returns the number of materialised zones.
func (m *ZoneMap) ZoneCount() int { return len(m.zones) }


// NearbyZones returns all materialised zones whose (zoneX, zoneZ) is
// within zoneRadius Chebyshev distance of the zone containing
// (x, z) at the given level. Unmaterialised zones are skipped —
// callers don't need to nil-check entries.
//
// Iteration order is dx ascending (outer) then dz ascending (inner),
// matching the grid.NearbyPlayers/NearbyNpcs convention (not the TS
// HuntIterator's descending order — order is distribution-neutral
// for the random picker in huntAll; logged as deviation D1 in
// 2026-04-22-nai-9-hunt-variant-fill-design.md).
//
// Used by modules/world/npc_hunt_entities.go for huntObjs/huntLocs
// zone-iteration.
func (m *ZoneMap) NearbyZones(level, x, z, zoneRadius int) []*Zone {
	zoneX := x >> 3
	zoneZ := z >> 3
	out := make([]*Zone, 0, (2*zoneRadius+1)*(2*zoneRadius+1))
	for dx := -zoneRadius; dx <= zoneRadius; dx++ {
		for dz := -zoneRadius; dz <= zoneRadius; dz++ {
			zx := zoneX + dx
			zz := zoneZ + dz
			if zx < 0 || zz < 0 {
				continue
			}
			idx := coordgrid.ZoneIndex(zx<<3, zz<<3, level)
			if z, ok := m.zones[idx]; ok {
				out = append(out, z)
			}
		}
	}
	return out
}

// LocCount sums len(Locs) across all materialised zones.
func (m *ZoneMap) LocCount() int {
	total := 0
	for _, z := range m.zones {
		total += len(z.Locs)
	}
	return total
}

// ObjCount sums len(Objs) across all materialised zones.
func (m *ZoneMap) ObjCount() int {
	total := 0
	for _, z := range m.zones {
		total += len(z.Objs)
	}
	return total
}
