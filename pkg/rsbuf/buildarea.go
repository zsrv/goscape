package rsbuf

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
// cleaned by *Buf.CleanupPlayerBuildArea (logout) or *Buf.RemovePlayer.
//
// NAI-29 deliberately omits these upstream fields/methods (deferred):
//   - forceViewDistance bool                   (NAI-32; engine-override)
//   - lastResize uint32                        (NAI-32; resize bookkeeping)
//   - Resize() / RebuildPlayers() / RebuildNpcs()       (NAI-32)
//   - getNearbyPlayers / getNearbyNpcs / spiral-search  (NAI-32)
//   - filterPlayer / filterNpc                          (NAI-32)
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
