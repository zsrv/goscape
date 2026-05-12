package world

import (
	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/pathfinder/routefinder"
)

// queueWaypoint clears any existing path and sets a single destination.
func (p *Player) queueWaypoint(x, z int) {
	p.waypoints[0] = coordgrid.PackCoord(p.level, x, z)
	p.waypointIndex = 0
}

// queueWaypoints replaces the current path with the given packed coords.
// Mirrors TS PathingEntity.queueWaypoints (Engine-TS PathingEntity.ts:248-254):
// reverses the input on copy so that internal storage is [dest, …, first_step].
// stepOnce reads waypoints[waypointIndex] starting at n-1 (= first_step) and
// decrements toward 0 (= dest).
//
// Truncation: when len(packed) exceeds len(p.waypoints), entries closest to
// dest are preserved (input iterates from length-1 down; output bounded above
// by waypoints buffer cap). TS-faithful: TS truncates the same way via
// output < this.waypoints.length.
func (p *Player) queueWaypoints(packed []int) {
	if len(packed) == 0 {
		p.waypointIndex = -1
		return
	}
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
	// NAI-44 T3: stepsTaken accumulates per-step in stepOnce (movement.go:88).
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

	if p.waypointIndex < 0 {
		p.walkDir = -1
		p.runDir = -1
		// NAI-135: no waypoints → no steps → tempRun reset (TS Player.ts:670-673).
		p.tempRun = 0
		return
	}

	dir, ok := p.stepOnce()
	if !ok {
		p.walkDir = -1
		p.runDir = -1
		// NAI-135: step blocked → no steps → tempRun reset (TS Player.ts:670-673).
		p.tempRun = 0
		return
	}
	p.walkDir = int(dir)
	p.runDir = -1

	if p.moveSpeed == MoveSpeedRun && p.waypointIndex >= 0 {
		dir2, ok2 := p.stepOnce()
		if ok2 {
			p.runDir = int(dir2)
		}
	}

	// NAI-82: TS Player.processMovement at Engine-TS/.../Player.ts:675-677
	// writes lastMovement = World.currentTick + 1 whenever stepsTaken > 0
	// after the tick's movement resolves. The defensive client/server nil
	// guard mirrors the established stepOnce convention (movement.go:84) —
	// fixture tests that construct a bare *Player with no client get a
	// silent skip.
	if p.stepsTaken > 0 && p.client != nil && p.client.server != nil {
		p.lastMovement = p.client.server.currentTick + 1
	}
}

// stepOnce advances one tile toward the current waypoint.
//
// NAI-175-D-PLAYER-STEP-COLLISION: (*Player).stepOnce passes
// (size=1, extraFlag=0, TypeNormal) to gamemap.CanTravel. TS port
// (PathingEntity.ts:617-683) calls getCollisionStrategy() and
// blockWalkFlag() per-step. For players this is mostly correct
// (MoveRestrictNormal almost always), but FlagBlockPlayers should
// be the extraFlag per Player.blockWalkFlag (player.go:608-610),
// not 0. Latent bug: players walk through NPCs whose tile carries
// FlagBlockPlayers. Not duck-symptom-binding; tracked under NAI-176.
func (p *Player) stepOnce() (coordgrid.Direction, bool) {
	if p.waypointIndex < 0 {
		return -1, false
	}
	dest := coordgrid.UnpackCoord(p.waypoints[p.waypointIndex])
	dir := coordgrid.Face(p.x, p.z, dest.X, dest.Z)
	if dir == -1 {
		p.waypointIndex--
		return -1, false
	}

	dx := coordgrid.DeltaX(dir)
	dz := coordgrid.DeltaZ(dir)
	if p.client != nil && p.client.server != nil && p.client.server.gamemap != nil {
		if !p.client.server.gamemap.CanTravel(p.level, p.x, p.z, dx, dz, 1, 0, collision.TypeNormal) {
			p.waypointIndex = -1
			return -1, false
		}
	}

	p.lastStepX = p.x
	p.lastStepZ = p.z
	p.x += dx
	p.z += dz
	p.stepsTaken++

	// Per-step refreshZone — mirrors TS PathingEntity.ts:182-183.
	// Level cannot change in stepOnce (single-tile delta); pass p.level for both.
	refreshPlayerZone(p, p.lastStepX, p.lastStepZ, p.level)

	if p.x == dest.X && p.z == dest.Z {
		p.waypointIndex--
	}
	return dir, true
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

// pathToMoveClick translates a MOVE_GAMECLICK / MOVE_OPCLICK waypoint list
// into the player's movement queue. If needsFinding is true and moveStrategy
// is SMART, the server runs its own pathfinder; otherwise it trusts the
// client-supplied coords.
func (p *Player) pathToMoveClick(packed []int, needsFinding bool) {
	if len(packed) == 0 {
		return
	}

	switch p.moveStrategy {
	case MoveStrategySmart:
		if needsFinding && p.client != nil && p.client.server != nil && p.client.server.gamemap != nil {
			dest := coordgrid.UnpackCoord(packed[0])
			route := p.client.server.gamemap.Pathfinder.FindPathPlain(p.level, p.x, p.z, dest.X, dest.Z)
			if coords := routeToPacked(route); len(coords) > 0 {
				p.queueWaypoints(coords)
			}
		} else {
			p.queueWaypoints(packed)
		}
	case MoveStrategyNaive:
		dest := coordgrid.UnpackCoord(packed[len(packed)-1])
		p.queueWaypoint(dest.X, dest.Z)
	}
}

// reorient is the per-tick refocus invoked from Server.processInfo
// before rsbuf compute. Mirrors TS PathingEntity.reorient at
// Engine-TS/src/engine/entity/PathingEntity.ts:349-361.
//
// PathingEntity targets (Player/Npc) are refocused on the target's
// current position (target may have moved this tick). Non-pathing
// targets (Loc/Obj) trigger one-shot focus + clear of the cached
// fine-coord (targetX/Z) iff the player took zero steps this tick —
// semantically "the entity moved off while we were trying to reach it."
func (p *Player) reorient() {
	switch t := p.target.(type) {
	case *Player:
		p.focus(coordgrid.Fine(t.x, 1), coordgrid.Fine(t.z, 1), false)
	case *Npc:
		p.focus(coordgrid.Fine(t.x, t.size), coordgrid.Fine(t.z, t.size), false)
	default:
		_ = t
		if p.targetX != -1 && p.stepsTaken == 0 {
			p.focus(p.targetX, p.targetZ, false)
			p.targetX = -1
			p.targetZ = -1
		}
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
