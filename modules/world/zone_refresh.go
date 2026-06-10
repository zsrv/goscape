package world

import "github.com/zsrv/goscape/pkg/objtype"

// This file ports TS PathingEntity.refreshZonePresence (Engine-TS@e1dea19f
// PathingEntity.ts:160-185), split per goscape's Player/Npc forks. Each
// *Presence function does the TS body in TS order:
//
//  1. Collision-follow (TS :162-179): when the position changed, move the
//     entity's collision footprint — switch (this.blockWalk):
//     case NPC: changeNpcCollision(width, prev..., false) + (new..., true)
//     case ALL: same PLUS the changePlayerCollision pair.
//     Runs whether or not the zone changed.
//  2. Zone-membership swap (TS :181-184): only when the zone (x>>3, z>>3,
//     level) changed.
//
// TS :177-178 also sets lastStepX/Z = prev inside the moved-guard; goscape
// keeps that bookkeeping at the existing sites instead (see the lastStep
// note on refreshPlayerZonePresence) — do NOT double-set it here.

// refreshPlayerZonePresence is the Player fork of TS refreshZonePresence.
// Called from (*Player).applyStep (movement.go), (*Player).Teleport, and
// (*Player).TeleJump (player_script.go) after the position is mutated.
//
// Collision width is 1: TS Player constructs with width=length=1
// (Player.ts:410-412), matching the existing SetVisibility / logout
// hardcode. p.blockWalk is BlockWalkNpc from newPlayer (TS Player.ts:411
// BlockWalk.NPC) and toggles NPC↔NONE via SetVisibility (TS Player.ts:
// 1875-1886).
//
// lastStep mapping (TS :177-178): (*Player).applyStep already records
// lastStepX/Z = pre-step position before mutating, and Teleport overwrites
// with x-1/z per TS :291-292 — equivalent net state, so this function does
// not touch lastStepX/Z.
//
// nil-guards: test fixtures may lack client/server/gamemap; skip collision
// silently (the zone swap has its own guards).
func refreshPlayerZonePresence(p *Player, prevX, prevZ, prevLevel int) {
	if p.client != nil && p.client.server != nil && p.client.server.gamemap != nil &&
		(p.x != prevX || p.z != prevZ || p.level != prevLevel) {
		gm := p.client.server.gamemap
		switch p.blockWalk {
		case BlockWalkNpc:
			// TS PathingEntity.ts:166-169.
			gm.ChangeNPCCollision(1, prevX, prevZ, prevLevel, false)
			gm.ChangeNPCCollision(1, p.x, p.z, p.level, true)
		case BlockWalkAll:
			// TS PathingEntity.ts:170-175.
			gm.ChangeNPCCollision(1, prevX, prevZ, prevLevel, false)
			gm.ChangeNPCCollision(1, p.x, p.z, p.level, true)
			gm.ChangePlayerCollision(1, prevX, prevZ, prevLevel, false)
			gm.ChangePlayerCollision(1, p.x, p.z, p.level, true)
		}
	}
	refreshPlayerZone(p, prevX, prevZ, prevLevel)
}

// refreshNpcZonePresence is the NPC fork of TS refreshZonePresence. Called
// from (*Npc).applyStep (npc_interaction.go) and (*Npc).Teleport
// (npc_script.go — wanderMode home-tele, patrolMode waypoint-tele, NPC_TELE)
// after the position is mutated.
//
// n.blockWalk / n.size are the NewNpc-time snapshots of typ.BlockWalk /
// typ.Size (npc.go:128-131, :172-173) — the same source the addNpc/removeNpc seeds
// read, so the moved footprint always matches the seeded footprint across
// a morph cycle. n.blockWalk uses the objtype constant family (mirror of
// TS BlockWalk.ts). Width is passed like TS (multi-tile NPCs move a
// size×size footprint).
//
// The respawn lifecycle path in (*Npc).turn (npc_ai.go) deliberately calls
// the zone-only refreshNpcZone instead: TS routes respawn through
// World.addNpc (World.ts:1258-1294, seed switch :1271-1279), whose collision seed goscape fires via
// revertType → addNpc; the death-tile flags were already cleared by
// removeNpc at death, so there is nothing to move.
//
// lastStep: Npc has no lastStepX/Z fields (D4-NPC documented dead-API skip,
// see npc_script.go DEVIATION block) — nothing to set.
func refreshNpcZonePresence(s *Server, n *Npc, prevX, prevZ, prevLevel int) {
	if s != nil && s.gamemap != nil &&
		(n.x != prevX || n.z != prevZ || n.level != prevLevel) {
		switch n.blockWalk {
		case objtype.BlockWalkNPC:
			// TS PathingEntity.ts:166-169.
			s.gamemap.ChangeNPCCollision(n.size, prevX, prevZ, prevLevel, false)
			s.gamemap.ChangeNPCCollision(n.size, n.x, n.z, n.level, true)
		case objtype.BlockWalkAll:
			// TS PathingEntity.ts:170-175.
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
// half of TS PathingEntity.refreshZonePresence (PathingEntity.ts:181-184);
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
