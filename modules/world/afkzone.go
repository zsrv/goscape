package world

// packAfkCoord packs (level, x, z) into a single int32 using the standard
// RS coord layout: (level<<28) | ((x&0x3FFF)<<14) | (z&0x3FFF).
func packAfkCoord(level, x, z int) int32 {
	return int32((level&0x3)<<28 | (x&0x3FFF)<<14 | (z & 0x3FFF))
}

// unpackAfkCoord reverses packAfkCoord, returning (x, z). Level is unused by
// AFK logic (the checks are 2D) so we don't bother returning it.
func unpackAfkCoord(packed int32) (x, z int) {
	x = int((packed >> 14) & 0x3FFF)
	z = int(packed & 0x3FFF)
	return
}

// rectsIntersect returns whether two axis-aligned rectangles overlap.
// Coords are the south-west corner; sizes extend north/east.
func rectsIntersect(ax, az, aw, ah, bx, bz, bw, bh int) bool {
	return ax < bx+bw && ax+aw > bx && az < bz+bh && az+ah > bz
}

// updateAfkZones advances the AFK-zone state machine. Pure server-side — no
// packet is ever sent. See TS Player.ts::updateAfkZones for the reference.
func (p *Player) updateAfkZones() {
	if p.lastAfkZone < 1000 {
		p.lastAfkZone++
	}
	if p.withinAfkZone() {
		return
	}
	coord := packAfkCoord(0, p.x-10, p.z-10)
	if p.moveSpeed == MoveSpeedInstant && p.jump {
		p.afkZones[1] = coord
	} else {
		p.afkZones[1] = p.afkZones[0]
	}
	p.afkZones[0] = coord
	p.lastAfkZone = 0
}

// withinAfkZone returns true if the player's 1×1 footprint still overlaps
// either of the two tracked 21×21 AFK windows.
func (p *Player) withinAfkZone() bool {
	const size = 21
	for i := range len(p.afkZones) {
		zx, zz := unpackAfkCoord(p.afkZones[i])
		if rectsIntersect(p.x, p.z, 1, 1, zx, zz, size, size) {
			return true
		}
	}
	return false
}

// IsZonesAfk returns true once lastAfkZone saturates at 1000 ticks (the
// player has not left either zone for that long).
func (p *Player) IsZonesAfk() bool { return p.lastAfkZone == 1000 }
