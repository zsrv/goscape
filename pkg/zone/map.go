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
