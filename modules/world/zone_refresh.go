package world

import "github.com/zsrv/goscape/pkg/objtype"

// This file ports TS PathingEntity.refreshZonePresence (Engine-TS@2e3bcf43
// PathingEntity.ts:163-185), split per goscape's Player/Npc forks. Each
// *Presence function does the TS body in TS order:
//
//  1. Collision-follow (TS :164-177): move the entity's collision footprint
//     — switch (this.blockWalk):
//     case NPC: changeNpcCollision(width, prev..., false) + (new..., true)
//     case ALL: same PLUS the changePlayerCollision pair.
//     Runs whether or not the zone changed.
//  2. lastStepX/Z = prev (TS :178-179, player fork only — Npc has no
//     lastStep fields, see refreshNpcZonePresence).
//  3. Zone-membership swap (TS :181-184): only when the zone (x>>3, z>>3,
//     level) changed.
//
// f0ccbe8a removed the pre-rev-254 "only when the entity moved" guard around
// steps 1-2: the collision toggle + lastStep update now run UNCONDITIONALLY
// on every call — a blocked step attempt (prev == cur) re-asserts the
// entity's collision flag on its current tile (remove+add of the same tile,
// net flag unchanged once set) and re-anchors lastStepX/Z to the current
// tile. The lastStep re-anchor is load-bearing for player-follow: a blocked
// follower's followX/Z become its own tile rather than the stale pre-block
// step.

// refreshPlayerZonePresence is the Player fork of TS refreshZonePresence.
// Called from (*Player).validateAndAdvanceStep (movement.go — every step
// attempt with a waypoint, moved or not, per f0ccbe8a), (*Player).Teleport,
// and (*Player).TeleJump (player_script.go) after the position is mutated.
//
// Collision width is 1: TS Player constructs with width=length=1
// (Player.ts:412-417), matching the existing SetVisibility / logout
// hardcode. p.blockWalk is BlockWalkNpc from newPlayer (TS Player.ts:416
// BlockWalk.NPC) and toggles NPC↔NONE via SetVisibility (TS Player.ts:
// 1899-1904).
//
// lastStep (TS :178-179): set UNCONDITIONALLY to the previous position —
// f0ccbe8a moved this out of the moved-guard. Teleport/TeleJump overwrite
// with x-1/z after this call per TS teleport (PathingEntity.ts:313-314).
//
// nil-guards: test fixtures may lack client/server/gamemap; skip collision
// silently (the zone swap has its own guards).
func refreshPlayerZonePresence(p *Player, prevX, prevZ, prevLevel int) {
	if p.client != nil && p.client.server != nil && p.client.server.gamemap != nil {
		gm := p.client.server.gamemap
		switch p.blockWalk {
		case BlockWalkNpc:
			// TS PathingEntity.ts:167-170.
			gm.ChangeNPCCollision(1, prevX, prevZ, prevLevel, false)
			gm.ChangeNPCCollision(1, p.x, p.z, p.level, true)
		case BlockWalkAll:
			// TS PathingEntity.ts:171-176.
			gm.ChangeNPCCollision(1, prevX, prevZ, prevLevel, false)
			gm.ChangeNPCCollision(1, p.x, p.z, p.level, true)
			gm.ChangePlayerCollision(1, prevX, prevZ, prevLevel, false)
			gm.ChangePlayerCollision(1, p.x, p.z, p.level, true)
		}
	}
	// TS PathingEntity.ts:178-179.
	p.lastStepX = prevX
	p.lastStepZ = prevZ
	refreshPlayerZone(p, prevX, prevZ, prevLevel)
}

// refreshNpcZonePresence is the NPC fork of TS refreshZonePresence. Called
// from (*Npc).validateAndAdvanceStep (npc_interaction.go — every step attempt
// with a waypoint, moved or not, per f0ccbe8a) and (*Npc).Teleport
// (npc_script.go — wanderMode home-tele, patrolMode waypoint-tele, NPC_TELE)
// after the position is mutated.
//
// n.blockWalk / n.size are the NewNpc-time snapshots of typ.BlockWalk /
// typ.Size (npc.go:124-130) — the same source the addNpc/removeNpc seeds
// read, so the moved footprint always matches the seeded footprint across
// a morph cycle. n.blockWalk uses the objtype constant family (mirror of
// TS BlockWalk.ts). Width is passed like TS (multi-tile NPCs move a
// size×size footprint).
//
// The respawn lifecycle path in (*Npc).turn (npc_ai.go) deliberately calls
// the zone-only refreshNpcZone instead: TS routes respawn through
// World.addNpc (World.ts:1295-1316), whose collision seed goscape fires via
// revertType → addNpc; the death-tile flags were already cleared by
// removeNpc at death, so there is nothing to move.
//
// lastStep: Npc has no lastStepX/Z fields (D4-NPC documented dead-API skip,
// see npc_script.go DEVIATION block) — nothing to set.
func refreshNpcZonePresence(s *Server, n *Npc, prevX, prevZ, prevLevel int) {
	if s != nil && s.gamemap != nil {
		switch n.blockWalk {
		case objtype.BlockWalkNPC:
			// TS PathingEntity.ts:167-170.
			s.gamemap.ChangeNPCCollision(n.size, prevX, prevZ, prevLevel, false)
			s.gamemap.ChangeNPCCollision(n.size, n.x, n.z, n.level, true)
		case objtype.BlockWalkAll:
			// TS PathingEntity.ts:171-176.
			s.gamemap.ChangeNPCCollision(n.size, prevX, prevZ, prevLevel, false)
			s.gamemap.ChangeNPCCollision(n.size, n.x, n.z, n.level, true)
			s.gamemap.ChangePlayerCollision(n.size, prevX, prevZ, prevLevel, false)
			s.gamemap.ChangePlayerCollision(n.size, n.x, n.z, n.level, true)
		}
	}
	refreshNpcZone(s, n, prevX, prevZ, prevLevel)
}

// refreshPlayerZone moves the player's pkg/zone subscription if (prevX>>3,
// prevZ>>3, prevLevel) differs from (p.x>>3, p.z>>3, p.level). Zone swap
// half of TS PathingEntity.refreshZonePresence (PathingEntity.ts:184-187);
// production movement callers go through refreshPlayerZonePresence above.
//
// nil-guards: if p.client/server/zoneMap are unset (e.g., test fixtures that
// bypass the standard server setup), skip silently.
func refreshPlayerZone(p *Player, prevX, prevZ, prevLevel int) {
	if p.client == nil || p.client.server == nil || p.client.server.zoneMap == nil {
		return
	}
	if (prevX>>3) == (p.x>>3) && (prevZ>>3) == (p.z>>3) && prevLevel == p.level {
		return
	}
	s := p.client.server
	prevZone := s.zoneMap.Get(prevLevel, prevX, prevZ)
	newZone := s.zoneMap.Get(p.level, p.x, p.z)
	prevZone.LeavePlayer(p, p.zoneListElement, s.zoneMap.Grid(prevLevel))
	p.zoneListElement = newZone.EnterPlayer(p, s.zoneMap.Grid(p.level))
}

// refreshNpcZone is the NPC-side analogue of refreshPlayerZone (zone swap
// half of TS refreshZonePresence). Production movement callers go through
// refreshNpcZonePresence above; the respawn lifecycle path in (*Npc).turn
// (npc_ai.go ~:37) calls this directly (collision is re-seeded by
// revertType → addNpc there — see refreshNpcZonePresence doc).
//
// NPC enter/leave do NOT touch ZoneGrid (only player branch flags).
func refreshNpcZone(s *Server, n *Npc, prevX, prevZ, prevLevel int) {
	if s == nil || s.zoneMap == nil {
		return
	}
	if (prevX>>3) == (n.x>>3) && (prevZ>>3) == (n.z>>3) && prevLevel == n.level {
		return
	}
	prevZone := s.zoneMap.Get(prevLevel, prevX, prevZ)
	newZone := s.zoneMap.Get(n.level, n.x, n.z)
	prevZone.LeaveNpc(n, n.zoneListElement)
	n.zoneListElement = newZone.EnterNpc(n)
}
