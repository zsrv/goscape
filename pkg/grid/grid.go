package grid

// Grid indexes players by zone (8x8 tile squares) for nearby-player lookup.
type Grid struct {
	zones map[uint32][]int // packed zone key -> player slots
}

// New returns an empty grid.
func New() *Grid {
	return &Grid{zones: map[uint32][]int{}}
}

// packZone packs (zoneX, zoneZ, level) into a single uint32 key.
func packZone(x, z, level int) uint32 {
	zoneX := (x >> 3) & 0x7FF
	zoneZ := (z >> 3) & 0x7FF
	return (uint32(level)&0x3)<<22 | uint32(zoneX)<<11 | uint32(zoneZ)
}

// Add records a player at the given coordinate.
func (g *Grid) Add(slot, x, z, level int) {
	key := packZone(x, z, level)
	g.zones[key] = append(g.zones[key], slot)
}

// Remove un-records a player from the given coordinate.
func (g *Grid) Remove(slot, x, z, level int) {
	key := packZone(x, z, level)
	slots := g.zones[key]
	for i, s := range slots {
		if s == slot {
			g.zones[key] = append(slots[:i], slots[i+1:]...)
			if len(g.zones[key]) == 0 {
				delete(g.zones, key)
			}
			return
		}
	}
}

// NearbyPlayers returns all player slots within zoneRadius zones (Chebyshev
// distance) of (x, z, level). The level must match exactly.
func (g *Grid) NearbyPlayers(x, z, level, zoneRadius int) []int {
	zoneX := x >> 3
	zoneZ := z >> 3
	out := []int{}
	for dx := -zoneRadius; dx <= zoneRadius; dx++ {
		for dz := -zoneRadius; dz <= zoneRadius; dz++ {
			zx := zoneX + dx
			zz := zoneZ + dz
			if zx < 0 || zz < 0 {
				continue
			}
			key := packZone(zx<<3, zz<<3, level)
			out = append(out, g.zones[key]...)
		}
	}
	return out
}
