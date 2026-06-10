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
					prevX, prevZ, prevLevel := n.x, n.z, n.level
					n.x, n.z, n.level = n.startX, n.startZ, n.startLevel
					// Zone-only refresh — deliberately NOT the collision-
					// following refreshNpcZonePresence: TS routes respawn
					// through World.addNpc (World.ts:1258-1294) which seeds
					// collision at the start tile (goscape: revertType →
					// addNpc below); the death-tile flags were already
					// cleared by removeNpc at death, so a presence-move
					// here would phantom-remove at the death tile.
					refreshNpcZone(s, n, prevX, prevZ, prevLevel)
					n.revertType()
				} else {
					// Revert morphed NPC (post-changetype).
					n.revertType()
				}
				// Lifecycle event fired this tick — skip movement so tele is
				// visible to the renderer and not overwritten by the walk path.
				return
			// PORTING-EXCEPTION (ARCH-1 / NAI-5): synchronous despawn here vs
			// TS try/catch retry at Npc.ts:144-150. tick_recovery covers the
			// panic case. See PORTING.md.
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

	// PORTING-EXCEPTION (M3, npc-validateDistanceWalked): TS Npc.ts:184 calls
	// validateDistanceWalked() here, which sets this.jump. But rsbuf.computeNpc
	// (World.ts:1047-1073) passes npc.tele and never npc.jump — the NpcInfo
	// protocol has no jump bit — so the NPC call is a no-op on the wire (note
	// TS's own "Dev note: Is this necessary?"). goscape's Npc therefore has no
	// jump field and intentionally omits the call. The player side (real wire
	// effect via computePlayer) is ported in processValidateDistanceWalked. See
	// PORTING.md / fix-tracker M3.
}

// QueueWaypoint clears any existing path and sets a single destination.
// Exported for use by pkg/script's ActiveNpc adapter (NAI-36).
func (n *Npc) QueueWaypoint(x, z int) {
	n.waypoints[0] = coordgrid.PackCoord(n.level, x, z)
	n.waypointIndex = 0
}

// queueWaypoints replaces the current path with the given packed coords.
// Mirrors TS PathingEntity.queueWaypoints (Engine-TS PathingEntity.ts:248-254);
// cross-reference (*Player).queueWaypoints (modules/world/movement.go).
//
// Reverses the input on copy so that internal storage is [dest, …, first_step].
// stepOnce reads waypoints[waypointIndex] starting at n-1 (= first_step) and
// decrements toward 0 (= dest). Truncation drops far-from-dest entries when
// input exceeds the waypoint buffer cap (TS-faithful).
//
// Unexported because external script-VM callers use QueueWaypoint
// (single-step) only.
func (n *Npc) queueWaypoints(packed []int) {
	if len(packed) == 0 {
		n.waypointIndex = -1
		return
	}
	index := -1
	for input, output := len(packed)-1, 0; input >= 0 && output < len(n.waypoints); input, output = input-1, output+1 {
		n.waypoints[output] = packed[input]
		index++
	}
	n.waypointIndex = index
}

// Kill is a test-only helper that marks the NPC dead and schedules respawn.
func (n *Npc) Kill() {
	n.dead = true
	n.lifecycleTick = n.respawnRate
	if n.lifecycleTick <= 0 {
		n.lifecycleTick = 50
	}
}
