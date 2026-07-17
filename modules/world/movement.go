package world

import (
	"math/rand/v2"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/pathfinder/routefinder"
)

// queueWaypoint clears any existing path and sets a single destination.
// Mirrors TS PathingEntity.queueWaypoint (PathingEntity.ts:253-256 @dee467c8).
// Upstream #100 deleted the AllowRepath mechanism (and the
// setAllowRepath(BEFOREDEST) tail) — the "I can't reach that!" fix.
func (p *Player) queueWaypoint(x, z int) {
	p.waypoints[0] = coordgrid.PackCoord(p.level, x, z)
	p.waypointIndex = 0
}

// queueWaypoints replaces the current path with the given packed coords.
// Mirrors TS PathingEntity.queueWaypoints (Engine-TS PathingEntity.ts:262-268
// @dee467c8): reverses the input on copy so that internal storage is
// [dest, …, first_step]. validateAndAdvanceStep reads waypoints[waypointIndex]
// starting at n-1 (= first_step) and decrements toward 0 (= dest).
//
// Truncation: when len(packed) exceeds len(p.waypoints), entries closest to
// dest are preserved (input iterates from length-1 down; output bounded above
// by waypoints buffer cap). TS-faithful: TS truncates the same way via
// output < this.waypoints.length.
//
// Empty input clears the path (index stays -1). Upstream #100 deleted the
// AllowRepath mechanism (and the setAllowRepath(BEFOREDEST) tail).
func (p *Player) queueWaypoints(packed []int) {
	index := -1
	for input, output := len(packed)-1, 0; input >= 0 && output < len(p.waypoints); input, output = input-1, output+1 {
		p.waypoints[output] = packed[input]
		index++
	}
	p.waypointIndex = index
}

// resolveMovement advances the player along their waypoint queue for one tick.
// Called from processPathing before processClientsOut so walkDir/runDir are
// set for the outgoing info block.
func (p *Player) resolveMovement() {
	// NAI-44 T3: stepsTaken accumulates per-step in validateAndAdvanceStep.
	// Reset at start of each tick's movement cycle so processInteraction
	// (which runs after processPathing in tick.go:38-39) reads the
	// per-tick step count. TS Player.processInteraction reads
	// stepsTaken === 0 to gate post-step retry timing (Player.ts:1245).
	p.stepsTaken = 0

	// NAI-144: TS Player.ts:657 movement gate. When the player has an
	// outstanding move-click request AND is busy (modal/delayed) AND has
	// unfinished primary-queue OR engineQueue work, suppress movement
	// for this tick.
	//
	// Setter source: NAI-146 (*Player).processPostDecode (TS World.ts:
	// 611-641 port). Closes the previously-tracked
	// NAI-144-D-MoveClickRequestSetter; gate is now live in production.
	//
	// Gate body explicitly clears walkDir/runDir to avoid stale prior-tick
	// values bleeding into the current tick's outbound info block (the
	// existing "no waypoints" branch below sets the same pattern).
	if p.moveClickRequest && p.Busy() && (len(p.queue) > 0 || len(p.engineQueue) > 0) {
		p.walkDir = -1
		p.runDir = -1
		return
	}

	// NAI-135: Bridge p.run → moveSpeed. Mirrors TS Player.ts:661-668.
	// moveSpeed==Instant skip preserves the teleport-jump invariant
	// from P_TELEJUMP / RebuildNormal (TS Player.ts:556).
	if p.moveSpeed != MoveSpeedInstant {
		p.moveSpeed = p.defaultMoveSpeed()
		if p.runanim == -1 {
			p.moveSpeed = MoveSpeedWalk
		} else if p.tempRun != 0 {
			p.moveSpeed = MoveSpeedRun
		}
	}

	p.lastTickX = p.x
	p.lastTickZ = p.z
	p.lastLevel = p.level

	// pathing-2: TS PathingEntity.processMovement (PathingEntity.ts:134-137)
	// early-returns when moveSpeed is INSTANT or STATIONARY. INSTANT is the
	// load-bearing case for Player — P_TELEJUMP / RebuildNormal
	// (player_script.go:600, login_resync.go:98) set it for the teleport
	// tick, and the bridge above preserves it. Without this gate, a queued
	// waypoint from a prior move-click still gets stepped on the
	// teleport tick, producing an animated walk inside the jump tick.
	// Player.updateMovement's !super.processMovement() branch (TS
	// Player.ts:670-673) resets tempRun in this case; we mirror that here.
	// STATIONARY is structurally-parity only for Player (the bridge above
	// overwrites it to WALK/RUN unless moveSpeed entered as INSTANT) but
	// kept to match TS L135 verbatim.
	if p.moveSpeed == MoveSpeedInstant || p.moveSpeed == MoveSpeedStationary {
		p.walkDir = -1
		p.runDir = -1
		p.tempRun = 0
		return
	}

	if p.waypointIndex < 0 {
		p.walkDir = -1
		p.runDir = -1
		// NAI-135: no waypoints → no steps → tempRun reset (TS Player.ts:670-673).
		p.tempRun = 0
		return
	}

	// TS PathingEntity.processMovement (PathingEntity.ts:146-152): a single
	// validateAndAdvanceStep per speed slot; -1 (blocked OR waypoint-consumed)
	// simply leaves the dir at -1. f0ccbe8a: a blocked step no longer resets
	// tempRun — TS resets tempRun only when processMovement returns false
	// (no waypoints / INSTANT / STATIONARY, the two branches above); the
	// pre-rev-254 goscape tempRun-reset-on-blocked is retired with the
	// recursion (see validateAndAdvanceStep below).
	p.walkDir = p.validateAndAdvanceStep()
	p.runDir = -1

	if p.moveSpeed == MoveSpeedRun && p.walkDir != -1 {
		p.runDir = p.validateAndAdvanceStep()
	}

	// NAI-82: TS Player.processMovement at Engine-TS/.../Player.ts:675-677
	// writes lastMovement = World.currentTick + 1 whenever stepsTaken > 0
	// after the tick's movement resolves. The defensive client/server nil
	// guard mirrors the established takeStep convention —
	// fixture tests that construct a bare *Player with no client get a
	// silent skip.
	if p.stepsTaken > 0 && p.client != nil && p.client.server != nil {
		p.lastMovement = p.client.server.currentTick + 1
	}
}

// validateDistanceWalked forces a teleport-style jump when the player moved
// more than 2 tiles from its start-of-tick position. Mirrors TS
// PathingEntity.validateDistanceWalked (PathingEntity.ts:303-315): the client
// can only animate a walk (1 tile) or run (2 tiles) per tick, so any larger
// displacement must be sent as a jump or the avatar visibly slides. lastTickX/Z
// are snapshotted before stepping in resolveMovement (movement.go:79), matching
// the TS field's start-of-tick value.
//
// Distance uses the player's own footprint on both sides (1x1), per TS
// CoordGrid.distanceTo(this, {x:lastTickX, z:lastTickZ, width, length}). M3.
func (p *Player) validateDistanceWalked() {
	if coordgrid.DistanceTo(p.x, p.z, p.Width(), p.Length(),
		p.lastTickX, p.lastTickZ, p.Width(), p.Length()) > 2 {
		p.jump = true
	}
}

// takeStep computes the (dx, dz) delta for one tile of travel toward the
// current waypoint WITHOUT mutating position. Mirrors TS PathingEntity.
// takeStep at the rev-254 pin (PathingEntity.ts:629-675, rewritten by
// f0ccbe8a): returns [0,0] on failsafe (no waypoint / nil strategy) or when
// every canTravel arm fails; the diagonal arm is gated on width==1 (players
// are always 1×1), then E/W, then N/S axis fallbacks.
//
// NAI-176-D-FLY-NO-CONTENT-WIRES: MoveStrategyFly bypasses collision
// entirely (TS L654-657). No content currently assigns Fly to Player;
// engine-fidelity only.
func (p *Player) takeStep() (int, int) {
	if p.waypointIndex == -1 {
		// failsafe check (TS L632-635)
		return 0, 0
	}
	cs := p.getCollisionStrategy()
	extraFlag := p.blockWalkFlag()
	if cs == nil {
		// failsafe check (TS L640-643)
		return 0, 0
	}

	dest := coordgrid.UnpackCoord(p.waypoints[p.waypointIndex])
	dir := coordgrid.Face(p.x, p.z, dest.X, dest.Z)
	dx := coordgrid.DeltaX(dir)
	dz := coordgrid.DeltaZ(dir)

	// Already on the waypoint tile (Face == -1 → zero deltas). TS feeds the
	// (0,0) deltas through canTravel, whose result is irrelevant — every arm
	// yields [0,0] either way. goscape's StepValidator panics on a (0,0)
	// offset, so short-circuit the observationally-identical result here.
	if dx == 0 && dz == 0 {
		return 0, 0
	}

	if p.moveStrategy == MoveStrategyFly {
		return dx, dz
	}

	if p.client == nil || p.client.server == nil || p.client.server.gamemap == nil {
		// Test-fixture path: no gamemap → skip collision and step freely.
		// (goscape defensive; TS always has rsmod loaded.)
		return dx, dz
	}
	gm := p.client.server.gamemap
	// Move diagonal (TS L659-662) — gated on width==1.
	if p.Width() == 1 && gm.CanTravel(p.level, p.x, p.z, dx, dz, p.Width(), extraFlag, *cs) {
		return dx, dz
	}
	// Move E/W (TS L664-667).
	if dx != 0 && gm.CanTravel(p.level, p.x, p.z, dx, 0, p.Width(), extraFlag, *cs) {
		return dx, 0
	}
	// Move N/S (TS L669-672).
	if dz != 0 && gm.CanTravel(p.level, p.x, p.z, 0, dz, p.Width(), extraFlag, *cs) {
		return 0, dz
	}
	// https://x.com/JagexAsh/status/1727609489954664502
	return 0, 0
}

// validateAndAdvanceStep validates and applies one step of movement,
// returning the step direction or -1 when no tile was covered. Mirrors TS
// PathingEntity.validateAndAdvanceStep at the rev-254 pin (PathingEntity.ts:
// 202-248, rewritten by f0ccbe8a):
//
//   - nomove (nil strategy / NULL blockwalk flag) now CLEARS the waypoint
//     queue outright (TS L206-209) instead of decrementing per call.
//   - NO recursion: a consumed waypoint costs the remainder of this tick's
//     step slot (the pre-f0ccbe8a try-next-waypoint cascade is retired).
//   - refreshZonePresence runs whenever a waypoint existed, EVEN IF the
//     entity did not move (TS L226-227) — this also re-anchors lastStepX/Z
//     to the pre-step (== current, when blocked) tile every attempt.
//   - orientation focus + stepsTaken only on actual movement (TS L237-245).
func (p *Player) validateAndAdvanceStep() int {
	cs := p.getCollisionStrategy()
	extraFlag := p.blockWalkFlag()

	// Clear waypoints if no movement is allowed (TS L206-209).
	if cs == nil || extraFlag == collision.FlagNull {
		p.waypointIndex = -1
	}

	// If no waypoints, return (TS L211-214).
	if p.waypointIndex == -1 {
		return -1
	}

	dx, dz := p.takeStep()

	srcX, srcZ := p.x, p.z

	// Move entity (TS L222-224).
	p.x += dx
	p.z += dz

	// Refresh zone presence if we had a waypoint, even if we didn't move
	// (TS L226-227). Sets lastStepX/Z = (srcX, srcZ) unconditionally —
	// see refreshPlayerZonePresence.
	refreshPlayerZonePresence(p, srcX, srcZ, p.level)

	// Update waypoint index if we reached the current waypoint (TS L229-235).
	if p.waypointIndex != -1 {
		coord := coordgrid.UnpackCoord(p.waypoints[p.waypointIndex])
		if coord.X == p.x && coord.Z == p.z {
			p.waypointIndex--
		}
	}

	// If we actually moved, update orientation and steps taken (TS L237-245).
	if p.x != srcX || p.z != srcZ {
		// Focus the tile in front. client=false → faceAngle only (no
		// FACE_COORD wire mask); keeps a walking entity's rendered
		// orientation tracking its movement for newly-visible observers.
		focusX := p.x + dx
		focusZ := p.z + dz
		p.focus(coordgrid.Fine(focusX, 1), coordgrid.Fine(focusZ, 1), false)
		p.stepsTaken++
		return int(coordgrid.Face(srcX, srcZ, p.x, p.z))
	}

	return -1
}

// defaultMoveSpeed maps p.run → MoveSpeed. Mirrors TS
// Engine-TS/src/engine/entity/Player.ts:710-712:
//
//	defaultMoveSpeed(): MoveSpeed {
//	    return this.run ? MoveSpeed.RUN : MoveSpeed.WALK;
//	}
//
// NAI-135.
func (p *Player) defaultMoveSpeed() MoveSpeed {
	if p.run != 0 {
		return MoveSpeedRun
	}
	return MoveSpeedWalk
}

// randomWalk queues a single waypoint one tile away on a random axis.
// Mirrors TS PathingEntity.randomWalk (PathingEntity.ts:430-439, added by
// f0ccbe8a; replaces the removed pathToMoveClick on the player side).
// Consumed by pathToPathingTarget's NAIVE under-target arm.
func (p *Player) randomWalk() {
	x, z := p.x, p.z
	if rand.IntN(2) == 0 {
		if rand.IntN(2) == 0 {
			x--
		} else {
			x++
		}
	} else {
		if rand.IntN(2) == 0 {
			z--
		} else {
			z++
		}
	}
	p.queueWaypoint(x, z)
}

// reorientEntity refocuses the serverside faceAngle toward a pathing
// (Player/Npc) target, run BEFORE movement/interaction so it captures the
// target's pre-move position for a newly-visible observer this tick.
// client=false: no face-coord mask — FACE_ENTITY (not FACE_COORD) is what
// live-tracks the target for existing observers. Mirrors TS
// PathingEntity.reorientEntity, TS PathingEntity.ts @4c95f87e
// (Engine-TS/src/engine/entity/PathingEntity.ts:364-369).
func (p *Player) reorientEntity() {
	switch t := p.target.(type) {
	case *Player:
		p.focus(coordgrid.Fine(t.x, 1), coordgrid.Fine(t.z, 1), false)
	case *Npc:
		p.focus(coordgrid.Fine(t.x, t.size), coordgrid.Fine(t.z, t.size), false)
	}
}

// reorient faces a loc/obj target once the player has stopped moving
// (targetX != -1 && stepsTaken == 0), shipping the face-coord mask
// (client=true) — the only path that ships the face-coord for loc/obj
// facing. Early-returns for a pathing target: reorientEntity already
// handled it earlier this tick. MUST run AFTER movement so stepsTaken
// reflects this tick's steps. Mirrors TS PathingEntity.reorient, TS
// PathingEntity.ts @4c95f87e (Engine-TS/src/engine/entity/PathingEntity.ts:377-388).
func (p *Player) reorient() {
	switch p.target.(type) {
	case *Player, *Npc:
		return
	}
	if p.targetX != -1 && p.stepsTaken == 0 {
		p.focus(p.targetX, p.targetZ, true)
		p.targetX = -1
		p.targetZ = -1
	}
}

// routeToPacked converts a pathfinder.Route into packed coord ints.
func routeToPacked(route routefinder.Route) []int {
	if !route.Success || len(route.Waypoints) == 0 {
		return nil
	}
	out := make([]int, len(route.Waypoints))
	for i, rc := range route.Waypoints {
		out[i] = coordgrid.PackCoord(rc.Level(), rc.X(), rc.Z())
	}
	return out
}
