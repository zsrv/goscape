package zone

// ZoneIndex packs (worldX, worldZ, level) into a single int using the same
// bit layout as the TS reference:
//
//	zone_x = worldX >> 3, zone_z = worldZ >> 3
//	index  = (zone_x & 0x7FF) | ((zone_z & 0x7FF) << 11) | ((level & 0x3) << 22)
func ZoneIndex(worldX, worldZ, level int) int {
	return ((worldX >> 3) & 0x7FF) | (((worldZ >> 3) & 0x7FF) << 11) | ((level & 0x3) << 22)
}

// UnpackIndex reverses ZoneIndex. Returns TILE-unit coordinates at the
// zone's SW corner (zoneX << 3, zoneZ << 3).
func UnpackIndex(index int) (worldX, worldZ, level int) {
	worldX = (index & 0x7FF) << 3
	worldZ = ((index >> 11) & 0x7FF) << 3
	level = (index >> 22) & 0x3
	return
}

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
	return m.GetByIndex(ZoneIndex(worldX, worldZ, level))
}

// GetByIndex returns the Zone with the given packed index, creating it if absent.
func (m *ZoneMap) GetByIndex(index int) *Zone {
	if z, ok := m.zones[index]; ok {
		return z
	}
	x, z, level := UnpackIndex(index)
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
