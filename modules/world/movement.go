package world

import (
	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/pathfinder/routefinder"
)

// queueWaypoint clears any existing path and sets a single destination.
func (p *Player) queueWaypoint(x, z int) {
	p.waypoints[0] = coordgrid.PackCoord(p.level, x, z)
	p.waypointIndex = 0
}

// queueWaypoints replaces the current path with the given packed coords.
// waypoints[0] is the final destination; the last element is the first step.
func (p *Player) queueWaypoints(packed []int) {
	if len(packed) == 0 {
		p.waypointIndex = -1
		return
	}
	n := len(packed)
	if n > len(p.waypoints) {
		n = len(p.waypoints)
	}
	for i := 0; i < n; i++ {
		p.waypoints[i] = packed[i]
	}
	p.waypointIndex = n - 1
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

	p.lastTickX = p.x
	p.lastTickZ = p.z
	p.lastLevel = p.level

	if p.waypointIndex < 0 {
		p.walkDir = -1
		p.runDir = -1
		return
	}

	dir, ok := p.stepOnce()
	if !ok {
		p.walkDir = -1
		p.runDir = -1
		return
	}
	p.walkDir = int(dir)
	p.runDir = -1

	if p.moveSpeed == MoveSpeedRun && p.runenergy > 0 && p.waypointIndex >= 0 {
		dir2, ok2 := p.stepOnce()
		if ok2 {
			p.runDir = int(dir2)
			p.drainRunEnergy()
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
		if !p.client.server.gamemap.CanTravel(p.level, p.x, p.z, dx, dz) {
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

// drainRunEnergy applies the TS run-energy decay formula once per running step.
func (p *Player) drainRunEnergy() {
	decay := (67 + 67*p.runweight/64) / 100
	if decay < 1 {
		decay = 1
	}
	p.runenergy -= decay
	if p.runenergy < 0 {
		p.runenergy = 0
	}
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
			route := p.client.server.gamemap.Pathfinder.FindPathDefault(p.level, p.x, p.z, dest.X, dest.Z)
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
