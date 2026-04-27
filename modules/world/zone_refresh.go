package world

// refreshPlayerZone moves the player's pkg/zone subscription if (prevX>>3,
// prevZ>>3, prevLevel) differs from (p.x>>3, p.z>>3, p.level). Called from
// (*Player).stepOnce, (*Player).Teleport, and (*Player).TeleJump after the
// position is mutated.
//
// Mirrors TS PathingEntity.refreshZone at PathingEntity.ts:182-183, applied
// at every per-step boundary check (TS dispatches via instanceof inside Zone;
// Go uses the typed PlayerLike branch).
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

// refreshNpcZone is the NPC-side analogue of refreshPlayerZone. Called from
// (*Npc).stepOnce, (*Npc).Teleport (used by wanderMode home-tele,
// patrolMode waypoint-tele, and the NPC_TELE script handler), and the
// respawn lifecycle path in (*Npc).turn (npc_ai.go ~:37).
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
