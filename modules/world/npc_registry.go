package world

import (
	"errors"

	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
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

// addNpc registers n in the world. Mirrors TS World.addNpc at
// World.ts:1258-1294.
//
// firstSpawn=true: allocate a slot, register in s.npcs + s.npcLoop
// (the original goscape behavior). firstSpawn=false: skip those —
// used by revertType's respawn cycle (NPC keeps its slot).
//
// Always: tele to (startX, startZ), clear dead flag, toggle collision
// flags on per n.typ.BlockWalk, run resetEntityForRespawn (stats
// reseed + waypoint/queue clear + tele/mask flag), reset animation,
// and (if duration > -1) write n.lifecycleTick.
//
// Caller responsible for synchronisation (called during NewServer or
// under playersMu).
func (s *Server) addNpc(n *Npc, duration int, firstSpawn bool) error {
	if firstSpawn {
		nid := s.allocNpcSlot()
		if nid < 0 {
			return errNpcsFull
		}
		n.nid = nid
		n.server = s
		s.npcs[nid] = n
		s.npcLoop = append(s.npcLoop, n)
		// TODO(NAI-19): rsbuf.AddNpc(n.nid, n.typeId) when rsbuf
		// API surface lands.
	}
	n.x = n.startX
	n.z = n.startZ
	n.dead = false
	// DEVIATION NAI-19-D1: zone.enter omitted — Zone abstraction
	// not ported. See spec § Tracked deviations.
	if s.gamemap != nil {
		switch n.blockWalk {
		case objtype.BlockWalkNPC:
			s.gamemap.ChangeNPCCollision(n.size, n.x, n.z, n.level, true)
		case objtype.BlockWalkAll:
			s.gamemap.ChangeNPCCollision(n.size, n.x, n.z, n.level, true)
			s.gamemap.ChangePlayerCollision(n.size, n.x, n.z, n.level, true)
		}
	}
	s.resetEntityForRespawn(n)
	n.animID = -1
	n.animDelay = 0
	// AI_SPAWN trigger queue (matches TS World.ts:1284-1289). Fires
	// unconditionally — for both firstSpawn=true (server boot) and
	// firstSpawn=false (revertType respawn). NPCs without a registered
	// AI_SPAWN script never enter the queue (the script != nil guard).
	// processNpcEventQueue dispatches at tick.go:40. Mirrors the
	// existing AI_DESPAWN producer pattern at npc_ai.go:47-58.
	if s.scriptProvider != nil && n.typ != nil {
		sf := s.scriptProvider.GetByTrigger(
			script.TriggerAiSpawn, n.typeId, n.typ.Category)
		if sf != nil {
			s.npcEventQueue = append(s.npcEventQueue,
				NpcEventRequest{
					Type:   NpcEventSpawn,
					Script: sf,
					Npc:    n,
				})
		}
	}
	if duration > -1 {
		n.lifecycleTick = duration
	}
	return nil
}

// resetEntityForRespawn applies the TS Npc.resetEntity(true) reseed
// (TS Npc.ts:280-317, respawn=true branch) factored out so addNpc and
// the future revertType refactor (Task 5e) share one definition.
//
// Resets typeId/uid to baseType (with fresh n.typ lookup), reseeds
// all 6 stats from n.typ.Stats, clears queue/waypoints, sets tele +
// CHANGE_TYPE mask, resets hunt fields. Does NOT touch n.x/n.z (the
// caller handles position) or collision flags (the caller handles
// those via gamemap).
func (s *Server) resetEntityForRespawn(n *Npc) {
	if n.typeId != n.baseType {
		n.typeId = n.baseType
		n.uid = (n.typeId << 16) | n.nid
		if newTyp := n.lookupType(n.baseType); newTyp != nil {
			n.typ = newTyp
		}
		n.masks |= rsbuf.NpcMaskChangeType
	}
	if n.typ != nil {
		for i := range min(objtype.NpcStatCount, len(n.typ.Stats)) {
			v := int(n.typ.Stats[i])
			n.levels[i] = v
			n.baseLevels[i] = v
		}
	}
	n.queue = nil
	n.waypointIndex = -1
	n.tele = true
	n.huntClock = 0
	n.huntTarget = nil
	if n.typ != nil {
		n.huntRange = int(n.typ.HuntRange)
		n.huntMode = n.typ.HuntMode
	}
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
	// DEVIATION NAI-19-D1: zone.leave omitted — Zone abstraction
	// not ported. See spec § Tracked deviations.
	n.dead = true
	if s.gamemap != nil {
		switch n.blockWalk {
		case objtype.BlockWalkNPC:
			s.gamemap.ChangeNPCCollision(n.size, n.x, n.z, n.level, false)
		case objtype.BlockWalkAll:
			s.gamemap.ChangeNPCCollision(n.size, n.x, n.z, n.level, false)
			s.gamemap.ChangePlayerCollision(n.size, n.x, n.z, n.level, false)
		}
	}
	if n.lifecycle == NpcLifecycleDespawn {
		// TODO(NAI-19): rsbuf.RemoveNpc(n.nid) when rsbuf API surface lands.
		// TODO(NAI-19): full registry cleanup (delete from s.npcs[],
		// splice s.npcLoop) remains deferred per pre-existing dead-bool
		// model — see npc_registry.go header history.
	} else if n.lifecycle == NpcLifecycleRespawn && duration > -1 {
		n.lifecycleTick = s.scaleByPlayerCount(duration)
	}
}
