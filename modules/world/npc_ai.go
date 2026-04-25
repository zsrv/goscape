package world

import (
	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/script"
)

// turn runs once per tick from processNpcs.
func (n *Npc) turn(s *Server) {
	// === Script-lifecycle prefix (NAI-2..NAI-4) ===
	// Matches TS Npc.ts:112 "if (this.isActive)" guard.
	if !n.dead {
		// Delayed expiration. Matches TS Npc.ts:113.
		if n.delayed && s.currentTick >= n.delayedUntil {
			n.delayed = false
		}
		// Resume suspended script. Matches TS Npc.ts:116-118.
		if !n.delayed && n.activeScript != nil &&
			n.activeScript.Execution == script.NpcSuspended {
			state := n.activeScript
			state.Execution = script.Running
			s.resumeOrFinishNpc(state, n)
		}
	}

	// === Events block (NAI-5 — matches TS Npc.ts:121-151) ===
	if !n.delayed {
		n.lifecycleTick--
		if n.lifecycleTick == 0 {
			switch n.lifecycle {
			case NpcLifecycleRespawn:
				if n.dead {
					// Respawn: flip dead, reset position, revert type.
					n.dead = false
					n.x, n.z, n.level = n.startX, n.startZ, n.startLevel
					n.revertType()
				} else {
					// Revert morphed NPC (post-changetype).
					n.revertType()
				}
				// Lifecycle event fired this tick — skip movement so tele is
				// visible to the renderer and not overwritten by the walk path.
				return
			case NpcLifecycleDespawn:
				if !n.dead {
					s.removeNpc(n, -1)
					if s.scriptProvider != nil && n.typ != nil {
						sf := s.scriptProvider.GetByTrigger(
							script.TriggerAiDespawn, n.typeId, n.typ.Category)
						if sf != nil {
							s.npcEventQueue = append(s.npcEventQueue,
								NpcEventRequest{
									Type:   NpcEventDespawn,
									Script: sf,
									Npc:    n,
								})
						}
					}
				}
				return
			}
		}
	}

	// === isValid gate (NAI-5 — matches TS Npc.ts:154) ===
	if n.dead || n.delayed {
		return
	}

	// === Hunt + consume + regen + timer + queue (NAI-7..10, NAI-6, NAI-4, NAI-3) ===
	s.processNpcHunt(n)    // NAI-7 — matches TS Npc.ts:158-171
	s.consumeHuntTarget(n) // NAI-10 — matches TS Npc.ts:174
	s.processNpcRegen(n)   // NAI-6 — matches TS Npc.ts:176
	s.processNpcTimer(n)
	s.processNpcQueue(n)

	// === Movement / interaction (NAI-11) ===
	n.processMovementInteraction(s)
}

// queueWaypoint clears any existing path and sets a single destination.
func (n *Npc) queueWaypoint(x, z int) {
	n.waypoints[0] = coordgrid.PackCoord(n.level, x, z)
	n.waypointIndex = 0
}

// Kill is a test-only helper that marks the NPC dead and schedules respawn.
func (n *Npc) Kill() {
	n.dead = true
	n.lifecycleTick = n.respawnRate
	if n.lifecycleTick <= 0 {
		n.lifecycleTick = 50
	}
}
