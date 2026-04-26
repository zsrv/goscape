package rsbuf

import "github.com/zsrv/goscape/pkg/coordgrid"

// Sizing constants mirror upstream build.rs:75-78 BuildArea constants
// (2004scape/rsbuf branch 225).
const (
	// preferredPlayers caps each player's tracked-player set.
	preferredPlayers = 250

	// preferredNpcs caps each player's tracked-npc set.
	preferredNpcs = 255

	// preferredViewDistance is the player's spatial culling radius
	// in tiles. Fixed at 15 in NAI-29-30; NAI-32 ports the dynamic
	// shrink/grow logic from upstream BuildArea::resize at build.rs:100-121.
	preferredViewDistance uint8 = 15
)

// BuildArea tracks per-player encoder state: the set of currently-
// observed players + npcs, and a tick-keyed map of recently-sent
// appearance hashes. Mirrors upstream build.rs BuildArea
// (2004scape/rsbuf branch 225, src/build.rs:64-96) minus the
// view-distance resize logic + spatial-discovery helpers (those land
// in NAI-32 / NAI-30).
//
// Concurrency: tick-goroutine-owned. Allocated by *Buf.AddPlayer;
// cleaned by *Buf.RemovePlayer at logout.
//
// NAI-29 deliberately omits these upstream fields/methods (deferred):
//   - forceViewDistance bool                            (NAI-32; engine-override)
//   - lastResize uint32                                 (NAI-32; resize bookkeeping)
//   - INTERVAL uint8 = 10                               (NAI-32; resize-step interval)
//   - Resize() / RebuildPlayers() / RebuildNpcs()       (NAI-32)
//   - getNearbyPlayers / getNearbyPlayersZones /
//     getNearbyPlayersNearest / filterPlayer            (NAI-32; consume view_distance)
//   - getNearbyNpcs / filterNpc                         (NAI-30; fixed PREFERRED_VIEW_DISTANCE)
//   - spiral-search helpers                             (NAI-32; player-side only)
type BuildArea struct {
	Players *idBitSet // 2048-bit set, capacity preferredPlayers
	Npcs    *idBitSet // 8192-bit set, capacity preferredNpcs

	// appearances[pid] = tick-when-the-appearance-payload-was-last-sent-to-this-player.
	// HasAppearance(pid, tick) returns true iff the stored tick == tick. Mirrors
	// upstream BuildArea.appearances at build.rs:68 + has_appearance/save_appearance
	// at build.rs:151-158.
	appearances [2048]uint32

	// ViewDistance is the per-player spatial-cull radius. Fixed at
	// preferredViewDistance in NAI-29-30; resize-able in NAI-32.
	ViewDistance uint8
}

// newBuildArea constructs a fresh BuildArea with empty tracking sets,
// zeroed appearances, and ViewDistance = preferredViewDistance. Mirrors
// upstream BuildArea::new at build.rs:81-90.
func newBuildArea() *BuildArea {
	return &BuildArea{
		Players:      newIdBitSet(2048, preferredPlayers),
		Npcs:         newIdBitSet(8192, preferredNpcs),
		ViewDistance: preferredViewDistance,
	}
}

// Cleanup empties the tracking sets and zeros the appearances cache.
// Mirrors upstream BuildArea::cleanup at build.rs:93-97.
func (b *BuildArea) Cleanup() {
	b.Players.Clear()
	b.Npcs.Clear()
	for i := range b.appearances {
		b.appearances[i] = 0
	}
}

// HasAppearance reports whether the appearance payload for pid was
// already sent to the local player on the named tick. Mirrors upstream
// BuildArea::has_appearance at build.rs:151-153.
func (b *BuildArea) HasAppearance(pid int32, tick uint32) bool {
	return b.appearances[pid] == tick
}

// SaveAppearance records that the appearance payload for pid was sent
// to the local player on tick. Mirrors upstream BuildArea::save_appearance
// at build.rs:155-157.
func (b *BuildArea) SaveAppearance(pid int32, tick uint32) {
	b.appearances[pid] = tick
}

// GetNearbyPlayers returns up to (preferredPlayers - len(b.Players))
// pids of players within preferredViewDistance zones (Chebyshev) of
// (x, level, z), excluding the local player (pid) and any player
// already in the tracking set. Mirrors upstream
// BuildArea::get_nearby_players_zones at build.rs:178-213 (zone-walk
// variant; the dispatcher between this and the spiral fallback
// get_nearby_players_nearest is NAI-32 scope).
//
// View distance is fixed at preferredViewDistance in NAI-30; NAI-32
// will introduce dynamic resize via a parameter.
func (b *BuildArea) GetNearbyPlayers(players *[2048]*Player, zoneMap *zoneMap, pid int32, x, level, z int) []int32 {
	distance := int(preferredViewDistance)
	startZX := (x - distance) >> 3
	startZZ := (z - distance) >> 3
	endZX := (x + distance) >> 3
	endZZ := (z + distance) >> 3

	count := b.Players.Len()
	remaining := int(preferredPlayers) - count
	if remaining <= 0 {
		return nil
	}
	nearby := make([]int32, 0, remaining)

	for zx := startZX; zx <= endZX; zx++ {
		for zz := startZZ; zz <= endZZ; zz++ {
			if len(nearby)+count >= int(preferredPlayers) {
				return nearby
			}
			zonePlayers := zoneMap.Zone(zx<<3, level, zz<<3).players
			for candidate := range zonePlayers { // map keys
				if len(nearby)+count >= int(preferredPlayers) {
					return nearby
				}
				if b.filterPlayer(players, candidate, pid, x, level, z) {
					nearby = append(nearby, candidate)
				}
			}
		}
	}
	return nearby
}

// filterPlayer reports whether candidate should be added to a
// nearby-players result. Mirrors upstream BuildArea::filter_player
// at build.rs:298-312. Five reject conditions: already tracked,
// out-of-distance (Chebyshev), pid==-1 (empty-slot marker),
// pid==self (self exclusion), level mismatch.
func (b *BuildArea) filterPlayer(players *[2048]*Player, candidate, pid int32, x, level, z int) bool {
	if candidate < 0 || int(candidate) >= len(players) {
		return false
	}
	other := players[candidate]
	if other == nil {
		return false
	}
	if b.Players.Contains(candidate) {
		return false
	}
	if other.PID == -1 {
		return false
	}
	if other.PID == pid {
		return false
	}
	otherPos := coordgrid.UnpackCoord(other.Coord)
	if otherPos.Level != level {
		return false
	}
	if !withinDistanceSW(otherPos.X, otherPos.Z, x, z, int(preferredViewDistance)) {
		return false
	}
	return true
}

// GetNearbyNpcs returns up to (preferredNpcs - len(b.Npcs)) nids of
// active NPCs within preferredViewDistance zones (Chebyshev) of
// (x, level, z), excluding any NPC already in the tracking set.
// Mirrors upstream BuildArea::get_nearby_npcs at build.rs:262-296.
//
// View distance is the const preferredViewDistance (15); upstream
// hardcodes BuildArea::PREFERRED_VIEW_DISTANCE here even when player
// view distance shrinks (NPCs don't downsize their search radius).
func (b *BuildArea) GetNearbyNpcs(npcs *[8192]*Npc, zoneMap *zoneMap, x, level, z int) []int32 {
	distance := int(preferredViewDistance)
	startZX := (x - distance) >> 3
	startZZ := (z - distance) >> 3
	endZX := (x + distance) >> 3
	endZZ := (z + distance) >> 3

	count := b.Npcs.Len()
	remaining := int(preferredNpcs) - count
	if remaining <= 0 {
		return nil
	}
	nearby := make([]int32, 0, remaining)

	for zx := startZX; zx <= endZX; zx++ {
		for zz := startZZ; zz <= endZZ; zz++ {
			if len(nearby)+count >= int(preferredNpcs) {
				return nearby
			}
			zoneNpcs := zoneMap.Zone(zx<<3, level, zz<<3).npcs
			for candidate := range zoneNpcs { // map keys
				if len(nearby)+count >= int(preferredNpcs) {
					return nearby
				}
				if b.filterNpc(npcs, candidate, x, level, z) {
					nearby = append(nearby, candidate)
				}
			}
		}
	}
	return nearby
}

// filterNpc reports whether candidate should be added to a
// nearby-npcs result. Mirrors upstream BuildArea::filter_npc at
// build.rs:314-327. Five reject conditions: already tracked,
// nid==-1 (empty-slot marker), !active, level mismatch,
// out-of-distance (Chebyshev).
func (b *BuildArea) filterNpc(npcs *[8192]*Npc, candidate int32, x, level, z int) bool {
	if candidate < 0 || int(candidate) >= len(npcs) {
		return false
	}
	other := npcs[candidate]
	if other == nil {
		return false
	}
	if b.Npcs.Contains(candidate) {
		return false
	}
	if other.NID == -1 {
		return false
	}
	if !other.Active {
		return false
	}
	otherPos := coordgrid.UnpackCoord(other.Coord)
	if otherPos.Level != level {
		return false
	}
	if !withinDistanceSW(otherPos.X, otherPos.Z, x, z, int(preferredViewDistance)) {
		return false
	}
	return true
}

// withinDistanceSW returns true if the Chebyshev distance between
// (ax, az) and (bx, bz) is <= radius. Mirrors upstream
// CoordGrid::within_distance_sw at coord.rs:50-58 (max of |dx|, |dz|
// against radius).
func withinDistanceSW(ax, az, bx, bz, radius int) bool {
	dx := ax - bx
	if dx < 0 {
		dx = -dx
	}
	dz := az - bz
	if dz < 0 {
		dz = -dz
	}
	if dx > dz {
		return dx <= radius
	}
	return dz <= radius
}
