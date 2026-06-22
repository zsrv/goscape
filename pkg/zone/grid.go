package zone

// Ported from Engine-TS/src/engine/zone/ZoneGrid.ts.

const (
	// ZoneGridSize is the side length of the world in zones (2048 × 8 = 16384 tiles).
	ZoneGridSize = 2048

	zoneGridIntBits     = 5
	zoneGridIntBitsFlag = (1 << zoneGridIntBits) - 1

	// ZoneGridDefaultSize is the int32-slice length needed for a full-world grid.
	ZoneGridDefaultSize = ZoneGridSize * (ZoneGridSize >> zoneGridIntBits)
)

// ZoneGrid is a per-level 2D bitmap of zone-occupancy flags. One bit per
// (zoneX, zoneZ) pair; set when the zone contains at least one player.
type ZoneGrid struct {
	grid []int32
}

// NewZoneGrid returns a zero-initialised ZoneGrid sized for the full world.
func NewZoneGrid() *ZoneGrid {
	return &ZoneGrid{grid: make([]int32, ZoneGridDefaultSize)}
}

func zoneGridIndex(zoneX, zoneZ int) int {
	return (zoneX << zoneGridIntBits) | (zoneZ >> zoneGridIntBits)
}

// Flag marks (zoneX, zoneZ) as occupied.
func (g *ZoneGrid) Flag(zoneX, zoneZ int) {
	g.grid[zoneGridIndex(zoneX, zoneZ)] |= 1 << (zoneZ & zoneGridIntBitsFlag)
}

// Unflag clears the occupied bit at (zoneX, zoneZ).
func (g *ZoneGrid) Unflag(zoneX, zoneZ int) {
	g.grid[zoneGridIndex(zoneX, zoneZ)] &= ^(1 << (zoneZ & zoneGridIntBitsFlag))
}

// IsFlagged reports whether ANY zone within `radius` of (zoneX, zoneZ) is flagged.
func (g *ZoneGrid) IsFlagged(zoneX, zoneZ, radius int) bool {
	minX := max(0, zoneX-radius)
	maxX := min(ZoneGridSize-1, zoneX+radius)
	minY := max(0, zoneZ-radius)
	maxY := min(ZoneGridSize-1, zoneZ+radius)
	bits := zoneGridIntBitsFlag
	startY := minY & ^bits
	endY := maxY >> zoneGridIntBits << zoneGridIntBits

	for x := minX; x <= maxX; x++ {
		for y := startY; y <= endY; y += 32 {
			index := zoneGridIndex(x, y)
			line := g.grid[index]
			trailingTrimmed := line
			if y+bits > maxY {
				trailingTrimmed = line & ((1 << (maxY - y + 1)) - 1)
			}
			leadingTrimmed := trailingTrimmed
			if y < minY {
				leadingTrimmed = trailingTrimmed >> (minY - y)
			}
			if leadingTrimmed != 0 {
				return true
			}
		}
	}
	return false
}
