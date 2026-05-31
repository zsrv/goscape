package collision

const (
	totalZoneCount = 256 * 256 * 4 * zoneTileCount
	zoneTileCount  = 8 * 8
)

func ZoneIndex(x int, z int, level int) int {
	return ((x >> 3) & 0x7FF) | (((z >> 3) & 0x7FF) << 11) | ((level & 0x3) << 22)
}

func TileIndex(x int, z int) int {
	return (x & 0x7) | ((z & 0x7) << 3)
}

type FlagMap struct {
	flags [][]int
}

func NewFlagMap() FlagMap {
	m := FlagMap{
		flags: make([][]int, totalZoneCount),
	}
	return m
}

// Get returns the collision bitmask of the tile at coordinates (absoluteX, absoluteZ, level).
//
// If the zone respective to the input coordinates has not been allocated,
// FlagNull (0x7FFFFFFF — every flag bit set except FlagRoof) is returned.
func (m *FlagMap) Get(absoluteX int, absoluteZ int, level int) int {
	zoneIndex := ZoneIndex(absoluteX, absoluteZ, level)
	tileIndex := TileIndex(absoluteX, absoluteZ)

	if len(m.flags) <= zoneIndex {
		return FlagNull
	}

	flags := m.flags[zoneIndex]
	if flags == nil || len(flags) <= tileIndex {
		return FlagNull
	}
	return flags[tileIndex]
}

// Set sets the collision bitmask of the tile at coordinates (absoluteX, absoluteZ, level) to mask.
//
// If the zone respective to the input coordinates has not been previously set, it will be
// allocated before setting the mask bitflag onto the tile.
func (m *FlagMap) Set(absoluteX int, absoluteZ int, level int, mask int) {
	if m.flags == nil {
		return
	}

	tiles := m.flags[ZoneIndex(absoluteX, absoluteZ, level)]
	if tiles == nil {
		tiles = m.AllocateIfAbsent(absoluteX, absoluteZ, level)
	}
	tiles[TileIndex(absoluteX, absoluteZ)] = mask
}

// Add appends the collision mask to the existing bitflags located at coordinates
// (absoluteX, absoluteZ, level).
//
// If the zone respective to the input coordinates has not been previously yet, it will be
// allocated before applying the mask bitflag onto the tile.
func (m *FlagMap) Add(absoluteX int, absoluteZ int, level int, mask int) {
	// If the zone has not been allocated previously, the Set method will
	// allocate/initialize it. We do not want the flag.FlagNull value
	// to be used. This is why we don't use the Get method and instead reuse similar
	// code below.

	zoneIndex := ZoneIndex(absoluteX, absoluteZ, level)
	tileIndex := TileIndex(absoluteX, absoluteZ)

	if len(m.flags) <= zoneIndex {
		return
	}

	flags := m.flags[zoneIndex]
	currentFlags := FlagOpen
	if flags != nil && tileIndex < len(flags) {
		currentFlags = flags[tileIndex]
	}

	m.Set(absoluteX, absoluteZ, level, currentFlags|mask)
}

func (m *FlagMap) Remove(absoluteX int, absoluteZ int, level int, mask int) {
	currentFlags := m.Get(absoluteX, absoluteZ, level)
	m.Set(absoluteX, absoluteZ, level, currentFlags & ^mask)
}

// AllocateIfAbsent allocates and initializes the collision flags for the zone found at coordinates
// (absoluteX, absoluteZ, level). If the zone has already been allocated, nothing will happen.
//
// The x and z coordinate can range anywhere from 0-7 tiles in respect to the zone base coordinates.
// For example, calling this method with the arguments (3202, 3204, level) will have the same result
// as calling it with (3200, 3200, level).
func (m *FlagMap) AllocateIfAbsent(absoluteX int, absoluteZ int, level int) []int {
	zoneIndex := ZoneIndex(absoluteX, absoluteZ, level)

	if len(m.flags) <= zoneIndex {
		return nil
	}

	existingFlags := m.flags[zoneIndex]
	if existingFlags != nil {
		return existingFlags
	}

	tileFlags := make([]int, zoneTileCount)
	m.flags[zoneIndex] = tileFlags
	return tileFlags
}

// DeallocateIfPresent deallocates the collision flags for the zone that can be found at coordinates
// (absoluteX, absoluteZ, level).
//
// The x and z coordinate can range anywhere from 0-7 tiles in respect to the zone base coordinates.
// For example, calling this method with the arguments (3202, 3204, level) will have the same result
// as calling it with (3200, 3200, level).
func (m *FlagMap) DeallocateIfPresent(absoluteX int, absoluteZ int, level int) {
	m.flags[ZoneIndex(absoluteX, absoluteZ, level)] = nil
}

func (m *FlagMap) IsZoneAllocated(absoluteX int, absoluteZ int, level int) bool {
	return m.flags[ZoneIndex(absoluteX, absoluteZ, level)] != nil
}

// IsFlagged reports whether the tile at (x,z,level) has any of the supplied
// `flags` bits set. Mirrors rsmod CollisionFlagMap::isFlagged
// (Engine/rsmod/collision.rs:58-65):
//
//	let current = self.get(x, z, level);
//	if current == CollisionFlag::Null as u32 { return false; }
//	return (current & masks) != CollisionFlag::Open as u32;
//
// The FlagNull short-circuit is load-bearing: an off-map / unallocated tile
// reports FALSE here. The blocked-vs-open distinction for movement is
// enforced separately by CanMove, which treats FlagNull as impassable for
// movement purposes (TestFlagNullStillBlocksMovement); the IsFlagged
// contract is a LineValidator/LineRouteFinder probe whose canonical meaning
// is "is this tile EXPLICITLY flagged" — and off-map tiles are not
// explicitly flagged. Pre-fix this returned true for every off-map probe
// (because FlagNull = 0x7FFFFFFF AND any non-zero flag mask is non-zero),
// inverting the rsmod contract and breaking RayCast/LineValidator LoS/LoW
// where the ray ventures off-map.
func (m *FlagMap) IsFlagged(x int, z int, level int, flags int) bool {
	current := m.Get(x, z, level)
	if current == FlagNull {
		return false
	}
	return (current & flags) != FlagOpen
}
