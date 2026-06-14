package world

import (
	"runtime/debug"

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
			if s.fireNpcLifecycle(n) {
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

	// "// Update target facing" — TS Npc.ts:183-184 @2e3bcf43 (ee28c1aa):
	// setFaceEntity() runs after processMovementInteraction, deriving
	// faceEntity from the (possibly just-cleared/just-set) target.
	n.setFaceEntity()

	// rev-274: TS Npc.ts:186 calls validateDistanceWalked() at the NPC
	// processing tail, which sets this.jump. The rev-274 NpcInfo add-leaf
	// gained a jump bit (crate 66911610), so rsbuf.ComputeNpc now passes
	// n.jump (World.ts:1048 @dee467c8 — npc.jump after npc.tele) and the flag
	// reaches the wire. This was a documented dead-API skip pre-274 (the M3
	// PORTING-EXCEPTION) because the NpcInfo protocol had no jump bit. The call
	// is unconditional (TS Npc.ts:186 has no EXACT_MOVE gate — that's the
	// player-only World.ts:733 path). jump is reset each tick in
	// resetPathingEntity (npc_masks.go).
	n.validateDistanceWalked()
}

// fireNpcLifecycle runs the once-per-cycle lifecycle transition (respawn /
// type-revert / despawn) under a recover that retries next tick on panic,
// mirroring TS Npc.ts:122-150 (try { … } catch { … this.setLifeCycle(1) }).
//
// Returns fired=true when a transition ran (respawn or despawn) so turn()
// skips this tick's movement — preserving goscape's behavior of not
// overwriting a teleport with a walk path on the transition tick.
//
// On panic the transition is logged and lifecycleTick is re-armed to 1 (TS
// setLifeCycle(1) — retry next tick) instead of letting the panic bubble to
// recoverNpc, which would evict the NPC via removeNpc(n,-1). This is the
// INNER of TS's two recovery layers: inner retry (Npc.ts:144-150) pre-empts
// outer evict (World.ts:681-690 → goscape recoverNpc). ARCH-1.
func (s *Server) fireNpcLifecycle(n *Npc) (fired bool) {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("panic in npc lifecycle transition (retrying next tick)",
				"nid", n.nid,
				"typeId", n.typeId,
				"lifecycle", n.lifecycle,
				"err", r,
				"stack", string(debug.Stack()))
			n.lifecycleTick = 1 // TS setLifeCycle(1): retry next tick
			fired = true        // skip movement this tick
		}
	}()
	switch n.lifecycle {
	case NpcLifecycleRespawn:
		if n.dead {
			// Respawn: flip dead, reset position, revert type.
			n.dead = false
			prevX, prevZ, prevLevel := n.x, n.z, n.level
			n.x, n.z, n.level = n.startX, n.startZ, n.startLevel
			// Zone-only refresh — deliberately NOT the collision-following
			// refreshNpcZonePresence: TS routes respawn through World.addNpc
			// (World.ts:1295-1316) which seeds collision at the start tile
			// (goscape: revertType → addNpc); the death-tile flags were
			// already cleared by removeNpc at death, so a presence-move here
			// would phantom-remove at the death tile.
			refreshNpcZone(s, n, prevX, prevZ, prevLevel)
			n.revertType()
		} else {
			// Revert morphed NPC (post-changetype).
			n.revertType()
		}
		return true
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
		return true
	}
	return false
}

// QueueWaypoint clears any existing path and sets a single destination.
// Exported for use by pkg/script's ActiveNpc adapter (NAI-36).
// Mirrors TS PathingEntity.queueWaypoint (PathingEntity.ts:253-256 @dee467c8).
// Upstream #100 deleted the AllowRepath mechanism (and the
// setAllowRepath(BEFOREDEST) tail).
func (n *Npc) QueueWaypoint(x, z int) {
	n.waypoints[0] = coordgrid.PackCoord(n.level, x, z)
	n.waypointIndex = 0
}

// queueWaypoints replaces the current path with the given packed coords.
// Mirrors TS PathingEntity.queueWaypoints (Engine-TS PathingEntity.ts:248-254);
// cross-reference (*Player).queueWaypoints (modules/world/movement.go).
//
// Reverses the input on copy so that internal storage is [dest, …, first_step].
// validateAndAdvanceStep reads waypoints[waypointIndex] starting at n-1 (= first_step) and
// decrements toward 0 (= dest). Truncation drops far-from-dest entries when
// input exceeds the waypoint buffer cap (TS-faithful).
//
// Unexported because external script-VM callers use QueueWaypoint
// (single-step) only.
//
// Empty input clears the path (index stays -1). Upstream #100 deleted the
// AllowRepath mechanism (and the setAllowRepath(BEFOREDEST) tail).
func (n *Npc) queueWaypoints(packed []int) {
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
