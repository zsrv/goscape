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
// playerGrid). NAI-30 adds Encoder fields (PlayerInfo, NpcInfo);
// Renderer fields and the renderer-cache compute-info wiring migrate
// at NAI-31. NAI-32 will add the view-distance / spiral-search
// optimization hooks.
type Buf struct {
	players    [2048]*Player
	npcs       [8192]*Npc
	zoneMap    *zoneMap
	playerGrid map[uint32][]int32 // tile-keyed; initialized here, populated by NAI-32 spiral search
	PlayerInfo *PlayerInfo
	NpcInfo    *NpcInfo
}

// New constructs an empty Buf with all slot tables nil-initialized,
// empty zoneMap, empty playerGrid. Mirrors upstream Lazy::new at
// lib.rs:28-37.
func New() *Buf {
	return &Buf{
		zoneMap:    newZoneMap(),
		playerGrid: map[uint32][]int32{},
		PlayerInfo: NewPlayerInfo(),
		NpcInfo:    NewNpcInfo(),
	}
}

// AddPlayer registers pid by allocating a *Player at slot[pid] with
// sentinel defaults + a fresh BuildArea. Mirrors upstream add_player
// at lib.rs:178-184.
//
// No-op if pid < 0 or pid >= 2048 (slot array bound). Double-add
// overwrites (matches upstream's unconditional assignment). Note:
// upstream guards only pid == -1; goscape broadens to pid < 0 because
// negative slice indexing panics in Go.
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
// No-op if pid < 0, pid >= 2048, or slot[pid] is nil. (Upstream guards
// only pid == -1; goscape broadens to pid < 0 — negative slice indexing
// panics in Go.)
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

// AddNpc registers nid with NPC type ntype by allocating an *Npc at
// slot[nid]. Mirrors upstream add_npc at lib.rs:305-311.
//
// No-op if nid < 0, nid >= 8192, or ntype < 0. (Upstream guards
// nid == -1 || ntype == -1; goscape broadens to <0 for slice safety.)
func (b *Buf) AddNpc(nid, ntype int32) {
	if nid < 0 || int(nid) >= len(b.npcs) || ntype < 0 {
		return
	}
	b.npcs[nid] = newNpc(nid, ntype)
}

// RemoveNpc unregisters nid. Steps (mirroring upstream remove_npc at
// lib.rs:313-324):
//  1. Remove nid from the zoneMap zone at the npc's last coord
//  2. (NAI-30) NPC_RENDERER.removePermanent(nid) — skipped here
//  3. Set slot[nid] = nil
//
// No-op if nid < 0, nid >= 8192, or slot[nid] is nil. (Upstream guards
// nid == -1; goscape broadens to nid < 0 for slice safety.)
func (b *Buf) RemoveNpc(nid int32) {
	if nid < 0 || int(nid) >= len(b.npcs) {
		return
	}
	n := b.npcs[nid]
	if n == nil {
		return
	}
	// Step 1: remove nid from zoneMap.
	pos := coordgrid.UnpackCoord(n.Coord)
	b.zoneMap.Zone(pos.X, pos.Level, pos.Z).RemoveNpc(nid)
	// Step 2 deferred to NAI-30.
	// Step 3: nil the slot.
	b.npcs[nid] = nil
}

// ComputePlayer writes ALL per-tick state for pid in one call. Mirrors
// upstream compute_player at lib.rs:39-153. Argument order matches
// upstream verbatim except pid is promoted to position 0 (Go method-
// receiver convention; upstream places pid at param 6 — lib.rs:46).
//
// Side effects:
//  1. If new coord crosses a zone boundary OR changes level (vs the
//     player's previous Coord): zoneMap.Zone(old).RemovePlayer(pid)
//     then zoneMap.Zone(new).AddPlayer(pid). Same-zone moves skip this
//     step (matches upstream lib.rs:115's zone-bound check).
//  2. Write all 35+ fields onto players[pid].
//  3. (NAI-30) PLAYER_RENDERER.compute_info(player) — skipped here.
//  4. Push pid onto playerGrid[player.Coord] (tile-keyed; unconditional;
//     mirrors upstream lib.rs:151).
//
// No-op if pid < 0, pid >= 2048, or slot[pid] is nil. (Upstream guards
// pid == -1; goscape broadens to pid < 0 for slice safety.)
//
// Sub-struct construction:
//   - say *string is stored verbatim (nil = no say this tick).
//   - message []byte: nil produces Chat=nil; non-nil produces Chat with
//     {bytes, color, effect, ignored}.
//   - exactStartX < 0 produces ExactMove=nil; otherwise a populated
//     ExactMove. Mirrors upstream lib.rs:90-103.
func (b *Buf) ComputePlayer(
	pid int32,
	x, level, z int,
	originX, originZ int,
	tele, jump bool,
	runDir, walkDir int8,
	visibility Visibility,
	staffModLevel int32,
	active bool,
	masks uint32,
	appearance []byte,
	lastAppearance int32,
	faceEntity, faceX, faceZ int32,
	orientationX, orientationZ int32,
	damageTaken, damageType int32,
	currentHitpoints, baseHitpoints int32,
	animID, animDelay int32,
	say *string,
	message []byte, color, effect, ignored uint8,
	graphicID, graphicHeight, graphicDelay int32,
	exactStartX, exactStartZ int32,
	exactEndX, exactEndZ int32,
	exactMoveStart, exactMoveEnd, exactMoveDirection int32,
) {
	if pid < 0 || int(pid) >= len(b.players) {
		return
	}
	p := b.players[pid]
	if p == nil {
		return
	}

	newCoord := coordgrid.PackCoord(level, x, z)

	// Step 1: zone-bound check (mirrors lib.rs:115).
	if newCoord != p.Coord {
		oldPos := coordgrid.UnpackCoord(p.Coord)
		// Zone change iff zone-x, zone-z, or level differ.
		if (oldPos.X>>3) != (x>>3) || (oldPos.Z>>3) != (z>>3) || oldPos.Level != level {
			b.zoneMap.Zone(oldPos.X, oldPos.Level, oldPos.Z).RemovePlayer(pid)
			b.zoneMap.Zone(x, level, z).AddPlayer(pid)
		}
	}

	// Step 2: write fields.
	p.Coord = newCoord
	p.Origin = coordgrid.PackCoord(level, originX, originZ)
	p.Tele = tele
	p.Jump = jump
	p.RunDir = runDir
	p.WalkDir = walkDir
	p.Visibility = visibility
	p.StaffModLevel = staffModLevel
	p.Active = active
	p.Masks = masks
	p.Appearance = appearance
	p.LastAppearance = lastAppearance
	p.FaceEntity = faceEntity
	p.FaceX = faceX
	p.FaceZ = faceZ
	p.OrientationX = orientationX
	p.OrientationZ = orientationZ
	p.DamageTaken = damageTaken
	p.DamageType = damageType
	p.CurrentHitpoints = currentHitpoints
	p.BaseHitpoints = baseHitpoints
	p.AnimID = animID
	p.AnimDelay = animDelay
	p.Say = say

	// Sub-struct construction: Chat from message bytes; ExactMove from
	// the exact-move 7-tuple (sentinel exactStartX < 0 = no exact move).
	if message != nil {
		p.Chat = &Chat{
			Bytes:   message,
			Color:   color,
			Effect:  effect,
			Ignored: ignored,
		}
	} else {
		p.Chat = nil
	}

	p.GraphicID = graphicID
	p.GraphicHeight = graphicHeight
	p.GraphicDelay = graphicDelay

	if exactStartX >= 0 {
		p.ExactMove = &ExactMove{
			StartX: exactStartX, StartZ: exactStartZ,
			EndX:   exactEndX, EndZ: exactEndZ,
			Begin:  exactMoveStart, Finish: exactMoveEnd,
			Dir:    exactMoveDirection,
		}
	} else {
		p.ExactMove = nil
	}

	// Step 3 deferred to NAI-30/31 (renderer compute_info).

	// Step 4: unconditional playerGrid push (mirrors lib.rs:151).
	key := uint32(newCoord)
	b.playerGrid[key] = append(b.playerGrid[key], pid)
}

// ComputeNpc writes ALL per-tick state for nid in one call. Mirrors
// upstream compute_npc at lib.rs:217-281. Argument order matches
// upstream verbatim except nid/ntype are promoted to positions 0/1
// (Go method-receiver convention + symmetry with ComputePlayer;
// upstream places (x, y, z) at positions 0-2 and nid/ntype at 3/4 —
// lib.rs:218-222).
//
// Side effects:
//  1. If new coord crosses a zone boundary OR changes level: zoneMap
//     remove+add (mirrors lib.rs:251).
//  2. Write 22 fields onto npcs[nid] (note: ntype is overwritten —
//     mirrors upstream lib.rs:256).
//  3. (NAI-30) NPC_RENDERER.compute_info(npc) — skipped here.
//
// No-op if nid < 0, nid >= 8192, ntype < 0, or slot[nid] is nil.
// (Upstream guards nid == -1 || ntype == -1; goscape broadens to <0
// for slice safety.)
//
// Note: NPCs do NOT update playerGrid (matches upstream — the
// tile-keyed grid is player-only). NPCs are spatially indexed only
// via zoneMap.zones[k].npcs.
func (b *Buf) ComputeNpc(
	nid, ntype int32,
	x, level, z int,
	tele bool,
	runDir, walkDir int8,
	active bool,
	masks uint32,
	faceEntity, faceX, faceZ int32,
	orientationX, orientationZ int32,
	damageTaken, damageType int32,
	currentHitpoints, baseHitpoints int32,
	animID, animDelay int32,
	say *string,
	graphicID, graphicHeight, graphicDelay int32,
) {
	if nid < 0 || int(nid) >= len(b.npcs) || ntype < 0 {
		return
	}
	n := b.npcs[nid]
	if n == nil {
		return
	}

	newCoord := coordgrid.PackCoord(level, x, z)

	// Step 1: zone-bound check (mirrors lib.rs:251).
	if newCoord != n.Coord {
		oldPos := coordgrid.UnpackCoord(n.Coord)
		// Zone change iff zone-x, zone-z, or level differ.
		if (oldPos.X>>3) != (x>>3) || (oldPos.Z>>3) != (z>>3) || oldPos.Level != level {
			b.zoneMap.Zone(oldPos.X, oldPos.Level, oldPos.Z).RemoveNpc(nid)
			b.zoneMap.Zone(x, level, z).AddNpc(nid)
		}
	}

	// Step 2: write fields. NType is overwritten (mirrors lib.rs:256).
	n.NType = ntype
	n.Coord = newCoord
	n.Tele = tele
	n.RunDir = runDir
	n.WalkDir = walkDir
	n.Active = active
	n.Masks = masks
	n.FaceEntity = faceEntity
	n.FaceX = faceX
	n.FaceZ = faceZ
	n.OrientationX = orientationX
	n.OrientationZ = orientationZ
	n.DamageTaken = damageTaken
	n.DamageType = damageType
	n.CurrentHitpoints = currentHitpoints
	n.BaseHitpoints = baseHitpoints
	n.AnimID = animID
	n.AnimDelay = animDelay
	n.Say = say
	n.GraphicID = graphicID
	n.GraphicHeight = graphicHeight
	n.GraphicDelay = graphicDelay

	// Step 3 (renderer compute_info) deferred to NAI-30/31.
}

// Cleanup resets the tile-keyed playerGrid and calls cleanup() on every
// populated Player + Npc. Called once per tick at end-of-tick (after
// info encoding completes). Mirrors upstream cleanup at lib.rs:348-363.
//
// (NAI-30) PLAYER_RENDERER.removeTemporary + NPC_RENDERER.removeTemporary
// at lib.rs:351-352 are skipped here pending NAI-31 renderer port.
func (b *Buf) Cleanup() {
	// Clear playerGrid (tile-keyed; rebuilt fresh each tick).
	clear(b.playerGrid)
	for _, p := range b.players {
		if p != nil {
			p.cleanup()
		}
	}
	for _, n := range b.npcs {
		if n != nil {
			n.cleanup()
		}
	}
}

// CleanupPlayerBuildArea calls Cleanup on the named player's BuildArea
// (clears tracking sets + appearances). Used at logout pre-flush.
// Mirrors upstream cleanup_player_buildarea at lib.rs:365-373.
//
// No-op if pid < 0, pid >= 2048, or slot[pid] is nil. (Upstream guards
// pid == -1; goscape broadens to pid < 0 for slice safety.)
func (b *Buf) CleanupPlayerBuildArea(pid int32) {
	if pid < 0 || int(pid) >= len(b.players) {
		return
	}
	p := b.players[pid]
	if p == nil {
		return
	}
	p.Build.Cleanup()
}

// HasPlayer reports whether pid currently observes other (i.e., other
// is in pid's BuildArea.Players tracking set). Mirrors upstream
// has_player at lib.rs:205-214.
//
// Returns false if pid < 0, other < 0, pid >= 2048, or pid's slot is
// nil. (Upstream guards pid == -1 || other == -1; goscape broadens to
// <0 + bounds-check for slice safety.)
func (b *Buf) HasPlayer(pid, other int32) bool {
	if pid < 0 || other < 0 || int(pid) >= len(b.players) {
		return false
	}
	p := b.players[pid]
	if p == nil {
		return false
	}
	return p.Build.Players.Contains(other)
}

// HasNpc reports whether pid currently observes nid (i.e., nid is in
// pid's BuildArea.Npcs tracking set). Mirrors upstream has_npc at
// lib.rs:326-335.
//
// Returns false if pid < 0, nid < 0, pid >= 2048, or pid's slot is
// nil. (Upstream guards pid == -1 || nid == -1; goscape broadens to
// <0 + bounds-check for slice safety.)
func (b *Buf) HasNpc(pid, nid int32) bool {
	if pid < 0 || nid < 0 || int(pid) >= len(b.players) {
		return false
	}
	p := b.players[pid]
	if p == nil {
		return false
	}
	return p.Build.Npcs.Contains(nid)
}

// GetNpcObservers returns the count of players currently observing nid.
// Mirrors upstream get_npc_observers at lib.rs:337-346.
//
// Returns 0 if nid < 0, nid >= 8192, or slot[nid] is nil. (Upstream
// guards nid == -1; goscape broadens to nid < 0 + bounds-check for
// slice safety.)
func (b *Buf) GetNpcObservers(nid int32) int32 {
	if nid < 0 || int(nid) >= len(b.npcs) {
		return 0
	}
	n := b.npcs[nid]
	if n == nil {
		return 0
	}
	return n.Observers
}

// PlayerForTest returns the *Player at slot pid, or nil if unset.
// Test-only accessor exposed for cross-package integration tests in
// modules/world; production code uses PlayerInfo.Encode and the
// dedicated query methods (HasPlayer, etc.).
func (b *Buf) PlayerForTest(pid int32) *Player {
	if pid < 0 || int(pid) >= len(b.players) {
		return nil
	}
	return b.players[pid]
}

// NpcForTest returns the *Npc at slot nid, or nil if unset.
// Test-only accessor exposed for cross-package integration tests
// in modules/world.
func (b *Buf) NpcForTest(nid int32) *Npc {
	if nid < 0 || int(nid) >= len(b.npcs) {
		return nil
	}
	return b.npcs[nid]
}
