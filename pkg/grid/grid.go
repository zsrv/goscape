package grid

// Grid indexes players and npcs by zone (8x8 tile squares) for nearby lookups.
type Grid struct {
	playerZones map[uint32][]int // packed zone key -> player slots
	npcZones    map[uint32][]int // packed zone key -> npc ids
}

// New returns an empty grid.
func New() *Grid {
	return &Grid{
		playerZones: map[uint32][]int{},
		npcZones:    map[uint32][]int{},
	}
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
	g.playerZones[key] = append(g.playerZones[key], slot)
}

// Remove un-records a player from the given coordinate.
func (g *Grid) Remove(slot, x, z, level int) {
	key := packZone(x, z, level)
	slots := g.playerZones[key]
	for i, s := range slots {
		if s == slot {
			g.playerZones[key] = append(slots[:i], slots[i+1:]...)
			if len(g.playerZones[key]) == 0 {
				delete(g.playerZones, key)
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
			out = append(out, g.playerZones[key]...)
		}
	}
	return out
}

// AddNpc records an npc at the given coordinate.
func (g *Grid) AddNpc(nid, x, z, level int) {
	key := packZone(x, z, level)
	g.npcZones[key] = append(g.npcZones[key], nid)
}

// RemoveNpc un-records an npc from the given coordinate.
func (g *Grid) RemoveNpc(nid, x, z, level int) {
	key := packZone(x, z, level)
	slots := g.npcZones[key]
	for i, s := range slots {
		if s == nid {
			g.npcZones[key] = append(slots[:i], slots[i+1:]...)
			if len(g.npcZones[key]) == 0 {
				delete(g.npcZones, key)
			}
			return
		}
	}
}

// NearbyNpcs returns all npc ids within zoneRadius zones (Chebyshev distance)
// of (x, z, level). The level must match exactly.
func (g *Grid) NearbyNpcs(x, z, level, zoneRadius int) []int {
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
			out = append(out, g.npcZones[key]...)
		}
	}
	return out
}
