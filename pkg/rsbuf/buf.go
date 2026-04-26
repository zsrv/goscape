package rsbuf

import (
	"github.com/zsrv/goscape/pkg/coordgrid"
)

// Buf is the rsbuf instance handle. One per world. Mirrors the upstream
// lib.rs unsafe-static globals (PLAYERS, NPCS, ZONE_MAP, PLAYER_GRID,
// PLAYER_RENDERER, NPC_RENDERER, PLAYER_INFO, NPC_INFO at lib.rs:28-37)
// collected onto a single value type.
//
// Concurrency: tick-goroutine-owned. All methods are tick-goroutine-
// only; no internal synchronization (matches upstream's WASM single-
// threaded model).
//
// NAI-29 lands the entity-state subset (players, npcs, zoneMap,
// playerGrid). NAI-30 will add Renderer + Encoder fields; NAI-31 will
// add the renderer-cache compute-info wiring; NAI-32 will add the
// view-distance / spiral-search optimization hooks.
type Buf struct {
	players    [2048]*Player
	npcs       [8192]*Npc
	zoneMap    *zoneMap
	playerGrid map[uint32][]int32 // tile-keyed (NAI-32 spiral search backing)
}

// New constructs an empty Buf with all slot tables nil-initialized,
// empty zoneMap, empty playerGrid. Mirrors upstream Lazy::new at
// lib.rs:28-37.
func New() *Buf {
	return &Buf{
		zoneMap:    newZoneMap(),
		playerGrid: map[uint32][]int32{},
	}
}

// AddPlayer registers pid by allocating a *Player at slot[pid] with
// sentinel defaults + a fresh BuildArea. Mirrors upstream add_player
// at lib.rs:178-184.
//
// No-op if pid == -1 or pid >= 2048 (slot array bound). Double-add
// overwrites (matches upstream's unconditional assignment).
func (b *Buf) AddPlayer(pid int32) {
	if pid < 0 || int(pid) >= len(b.players) {
		return
	}
	p := newPlayer(pid)
	p.Build = newBuildArea()
	b.players[pid] = p
}

// RemovePlayer unregisters pid. Steps (mirroring upstream remove_player
// at lib.rs:186-203):
//  1. Remove pid from the zoneMap zone at the player's last coord
//  2. For each nid in player.Build.Npcs.Iter(), decrement npcs[nid].Observers (floor at 0)
//  3. Call player.Build.Cleanup() (clears tracking + appearances)
//  4. (NAI-30) PLAYER_RENDERER.removePermanent(pid) — skipped here
//  5. Set slot[pid] = nil
//
// No-op if pid == -1, pid >= 2048, or slot[pid] is nil.
func (b *Buf) RemovePlayer(pid int32) {
	if pid < 0 || int(pid) >= len(b.players) {
		return
	}
	p := b.players[pid]
	if p == nil {
		return
	}
	// Step 1: remove from zoneMap.
	pos := coordgrid.UnpackCoord(p.Coord)
	b.zoneMap.Zone(pos.X, pos.Level, pos.Z).RemovePlayer(pid)
	// Step 2: decrement observer counts for tracked npcs.
	for _, nid := range p.Build.Npcs.Iter() {
		if int(nid) >= len(b.npcs) {
			continue
		}
		n := b.npcs[nid]
		if n != nil && n.Observers > 0 {
			n.Observers--
		}
	}
	// Step 3: cleanup BuildArea.
	p.Build.Cleanup()
	// Step 4 deferred to NAI-30.
	// Step 5: nil the slot.
	b.players[pid] = nil
}

// AddNpc is a temporary minimal stub. Bundle 3 Task 3.3 replaces this
// with the full implementation that also wires zoneMap. Allocates the
// slot only — sufficient to let Task 3.2's RemovePlayer tests run.
//
// TEMPORARY: replaced by Bundle 3 Task 3.3.
func (b *Buf) AddNpc(nid, ntype int32) {
	if nid < 0 || int(nid) >= len(b.npcs) {
		return
	}
	b.npcs[nid] = newNpc(nid, ntype)
}
