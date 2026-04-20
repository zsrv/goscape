package buildarea

import (
	"sort"

	"github.com/zsrv/goscape/pkg/coordgrid"
)

// BuildArea tracks a player's 13x13 mapsquare window centred on the last
// anchor point (OriginX, OriginZ). Mirrors the TS BuildArea from
// LostCityRS/Engine-TS.
type BuildArea struct {
	OriginX     int
	OriginZ     int
	LastBuild   int
	LoadedZones map[int]bool
	ActiveZones map[int]bool
	Mapsquares  map[uint16]bool
	Players     map[int]struct{}
	Appearance  map[int]uint64
	Npcs        map[int]struct{}
}

func New() *BuildArea {
	return &BuildArea{
		OriginX:     -1,
		OriginZ:     -1,
		LoadedZones: map[int]bool{},
		ActiveZones: map[int]bool{},
		Mapsquares:  map[uint16]bool{},
		Players:     map[int]struct{}{},
		Appearance:  map[int]uint64{},
		Npcs:        map[int]struct{}{},
	}
}

// HasAppearance returns true if the given appearance hash has already been
// recorded for the given slot.
func (ba *BuildArea) HasAppearance(slot int, hash uint64) bool {
	stored, ok := ba.Appearance[slot]
	return ok && stored == hash
}

// RecordAppearance caches the appearance hash for the given slot.
func (ba *BuildArea) RecordAppearance(slot int, hash uint64) {
	ba.Appearance[slot] = hash
}

// ShouldRebuild reports whether the player has crossed the 13x13 zone window
// centred on (OriginX, OriginZ), or whether reconnect is true (force).
func (ba *BuildArea) ShouldRebuild(playerX, playerZ int, reconnect bool) bool {
	if ba.OriginX == -1 {
		return true
	}
	if reconnect {
		return true
	}
	originZoneX := ba.OriginX >> 3
	originZoneZ := ba.OriginZ >> 3
	reloadLeftX := (originZoneX - 4) << 3
	reloadRightX := (originZoneX + 5) << 3
	reloadTopZ := (originZoneZ + 5) << 3
	reloadBottomZ := (originZoneZ - 4) << 3
	if playerX < reloadLeftX || playerZ < reloadBottomZ ||
		playerX > reloadRightX-1 || playerZ > reloadTopZ-1 {
		return true
	}
	return false
}

// Rebuild resets the build area, recomputes the 13x13 zone window mapsquares,
// and commits the new origin. Returns mapsquare list packed as (mapX<<8)|mapZ.
func (ba *BuildArea) Rebuild(playerX, playerZ, currentTick int) []uint16 {
	ba.LoadedZones = map[int]bool{}
	ba.ActiveZones = map[int]bool{}
	ba.Mapsquares = map[uint16]bool{}

	zoneX := playerX >> 3
	zoneZ := playerZ >> 3
	for dx := -6; dx <= 6; dx++ {
		for dz := -6; dz <= 6; dz++ {
			zx := zoneX + dx
			zz := zoneZ + dz
			if zx < 0 || zz < 0 {
				continue
			}
			mapX := zx >> 3
			mapZ := zz >> 3
			if mapX > 0xff || mapZ > 0xff {
				continue
			}
			ba.Mapsquares[uint16((mapX<<8)|mapZ)] = true
			ba.ActiveZones[coordgrid.ZoneIndex(zx<<3, zz<<3, 0)] = true // NEW
		}
	}

	ba.OriginX = playerX
	ba.OriginZ = playerZ
	ba.LastBuild = currentTick

	out := make([]uint16, 0, len(ba.Mapsquares))
	for m := range ba.Mapsquares {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
