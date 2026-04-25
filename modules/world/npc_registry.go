package world

import (
	"errors"

	"github.com/zsrv/goscape/pkg/objtype"
)

var errNpcsFull = errors.New("npc registry full")

// allocNpcSlot returns a free nid (1..8191). Returns -1 if full.
func (s *Server) allocNpcSlot() int {
	for offset := 0; offset < len(s.npcs)-1; offset++ {
		i := s.nextNpcSlot + offset
		if i < 1 {
			i = 1
		}
		if i >= len(s.npcs) {
			i = (i % (len(s.npcs) - 1)) + 1
		}
		if s.npcs[i] == nil {
			s.nextNpcSlot = (i + 1) % len(s.npcs)
			if s.nextNpcSlot < 1 {
				s.nextNpcSlot = 1
			}
			return i
		}
	}
	return -1
}

// addNpc places n into a free slot, sets n.nid, appends to npcLoop.
// Caller responsible for synchronisation (called during NewServer or under playersMu).
func (s *Server) addNpc(n *Npc) error {
	nid := s.allocNpcSlot()
	if nid < 0 {
		return errNpcsFull
	}
	n.nid = nid
	n.server = s
	s.npcs[nid] = n
	s.npcLoop = append(s.npcLoop, n)
	return nil
}

// removeNpc marks n as logically absent from the world. Mirrors TS
// World.removeNpc at World.ts:1296-1319.
//
// Per TS: scales `duration` by player count, runs zone.leave (DEFERRED
// per NAI-19-D1), flips isActive=false (n.dead=true in goscape), toggles
// collision flags off per n.typ.BlockWalk, and branches on lifecycle:
//   - DESPAWN: TS removes from rsbuf + registry + cleanup. Goscape
//     keeps the dead-bool model (registry cleanup is orthogonal; tracked
//     by the existing npc_registry.go header comment).
//   - RESPAWN+duration>-1: writes n.lifecycleTick = scaledDuration.
func (s *Server) removeNpc(n *Npc, duration int) {
	adjustedDuration := s.scaleByPlayerCount(duration)
	// DEVIATION NAI-19-D1: zone.leave omitted — Zone abstraction
	// not ported. See spec § Tracked deviations.
	n.dead = true
	if n.typ != nil && s.gamemap != nil {
		switch n.typ.BlockWalk {
		case objtype.BlockWalkNPC:
			s.gamemap.ChangeNPCCollision(int(n.typ.Size), n.x, n.z, n.level, false)
		case objtype.BlockWalkAll:
			s.gamemap.ChangeNPCCollision(int(n.typ.Size), n.x, n.z, n.level, false)
			s.gamemap.ChangePlayerCollision(int(n.typ.Size), n.x, n.z, n.level, false)
		}
	}
	if n.lifecycle == NpcLifecycleDespawn {
		// TODO(NAI-19): rsbuf.RemoveNpc(n.nid) when rsbuf API surface lands.
		// TODO(NAI-19): full registry cleanup (delete from s.npcs[],
		// splice s.npcLoop) remains deferred per pre-existing dead-bool
		// model — see npc_registry.go header history.
	} else if n.lifecycle == NpcLifecycleRespawn && duration > -1 {
		n.lifecycleTick = adjustedDuration
	}
}
