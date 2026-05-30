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

	// pathing-2: TS PathingEntity.processMovement (PathingEntity.ts:134-137)
	// early-returns when moveSpeed is INSTANT or STATIONARY. INSTANT is the
	// load-bearing case for Player — P_TELEJUMP / RebuildNormal
	// (player_script.go:600, login_resync.go:98) set it for the teleport
	// tick, and the bridge above preserves it. Without this gate, a queued
	// waypoint from a prior pathToMoveClick still gets stepped on the
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

	dir, ok := p.validateAndAdvanceStep()
	if !ok {
		p.walkDir = -1
		p.runDir = -1
		// NAI-135: step blocked → no steps → tempRun reset (TS Player.ts:670-673).
		p.tempRun = 0
		return
	}
	p.walkDir = dir
	p.runDir = -1

	if p.moveSpeed == MoveSpeedRun && p.waypointIndex >= 0 {
		dir2, ok2 := p.validateAndAdvanceStep()
		if ok2 {
			p.runDir = dir2
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

// stepOnce walks one tile toward the current waypoint and returns
// (dir, status). Mirrors TS PathingEntity.takeStep (PathingEntity.ts:617-683)
// for width=1 entities (Player.Width() ≡ 1). Position update + dest-check
// decrement of waypointIndex happen inline via applyStep; transient-block /
// done / no-move classifications go to validateAndAdvanceStep for
// waypointIndex bookkeeping.
//
// NAI-176 B3: plumbs p.blockWalkFlag() (= FlagBlockPlayers) and
// p.getCollisionStrategy() per-step + D1 axis-fallback (X-only / Z-only).
// Retires NAI-175-D-PLAYER-STEP-COLLISION.
func (p *Player) stepOnce() (int, stepStatus) {
	if p.waypointIndex < 0 {
		return -1, stepBlocked
	}
	cs := p.getCollisionStrategy()
	if cs == nil {
		return -1, stepDone
	}
	extraFlag := p.blockWalkFlag()
	if extraFlag == collision.FlagNull {
		return -1, stepDone
	}
	dest := coordgrid.UnpackCoord(p.waypoints[p.waypointIndex])
	dir := coordgrid.Face(p.x, p.z, dest.X, dest.Z)
	if dir == -1 {
		return -1, stepDone
	}
	dx := coordgrid.DeltaX(dir)
	dz := coordgrid.DeltaZ(dir)

	// NAI-176-D-FLY-NO-CONTENT-WIRES: TS PathingEntity.takeStep:663-665
	// — MoveStrategyFly bypasses collision entirely. No content currently
	// assigns MoveStrategyFly to Player; engine-fidelity only.
	if p.moveStrategy == MoveStrategyFly {
		return p.applyStep(dest, dx, dz, int(dir))
	}

	if p.client == nil || p.client.server == nil || p.client.server.gamemap == nil {
		// Test-fixture path: no gamemap → skip collision and apply step.
		return p.applyStep(dest, dx, dz, int(dir))
	}
	gm := p.client.server.gamemap
	if gm.CanTravel(p.level, p.x, p.z, dx, dz, 1, extraFlag, *cs) {
		return p.applyStep(dest, dx, dz, int(dir))
	}
	if dx != 0 && gm.CanTravel(p.level, p.x, p.z, dx, 0, 1, extraFlag, *cs) {
		axisDir := coordgrid.Face(p.x, p.z, dest.X, p.z)
		return p.applyStep(dest, dx, 0, int(axisDir))
	}
	if dz != 0 && gm.CanTravel(p.level, p.x, p.z, 0, dz, 1, extraFlag, *cs) {
		axisDir := coordgrid.Face(p.x, p.z, p.x, dest.Z)
		return p.applyStep(dest, 0, dz, int(axisDir))
	}
	// NAI-176 D2: TS L682 returns null (transient block); wrapper preserves
	// waypointIndex.
	return -1, stepBlocked
}

// applyStep advances the player one tile by (dx, dz), refreshes zone
// presence, and decrements waypointIndex if the destination is reached.
// lastStepX/Z capture pre-step position (consumed by interaction.go and
// player_script.go follower paths — see NAI-174). Returns (dir, stepMoved).
func (p *Player) applyStep(dest coordgrid.Position, dx, dz, dir int) (int, stepStatus) {
	p.lastStepX = p.x
	p.lastStepZ = p.z
	p.x += dx
	p.z += dz
	// M2: per-step focus (TS PathingEntity.ts:216-220). After advancing, face the
	// tile one further in the travel direction so faceAngle tracks movement.
	// client=false → faceAngle only (no FACE_COORD wire mask). Paired with D1's
	// per-tick faceSquare reset, this is what keeps a walking entity's rendered
	// orientation (effectiveFaceCoord → faceAngle once faceSquare clears) pointing
	// where it walks for newly-visible observers, instead of a stale square.
	p.focus(coordgrid.Fine(coordgrid.MoveX(p.x, coordgrid.Direction(dir)), 1), coordgrid.Fine(coordgrid.MoveZ(p.z, coordgrid.Direction(dir)), 1), false)
	p.stepsTaken++
	refreshPlayerZone(p, p.lastStepX, p.lastStepZ, p.level)
	if p.x == dest.X && p.z == dest.Z {
		p.waypointIndex--
	}
	return dir, stepMoved
}

// validateAndAdvanceStep wraps stepOnce with waypointIndex bookkeeping
// + recursive try-next-waypoint cascade. Mirrors TS PathingEntity.
// validateAndAdvanceStep (PathingEntity.ts:202-232).
func (p *Player) validateAndAdvanceStep() (int, bool) {
	dir, status := p.stepOnce()
	switch status {
	case stepBlocked:
		return -1, false
	case stepDone:
		p.waypointIndex--
		if p.waypointIndex >= 0 {
			return p.validateAndAdvanceStep()
		}
		return -1, false
	case stepMoved:
		return dir, true
	}
	return -1, false
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
// is SMART, the server runs its own pathfinder; otherwise (Naive or Fly)
// it queues a waypoint at the last coord.
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
	case MoveStrategyNaive, MoveStrategyFly:
		// TS PathingEntity.pathToMoveClick L408-420: any non-SMART strategy
		// queues a single waypoint at the LAST coord of the input.
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
