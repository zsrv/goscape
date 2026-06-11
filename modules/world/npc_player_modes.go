package world

import (
	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
)

// PLAYER* NPC mode implementations. Each per-tick mode method is dispatched
// from (*Npc).processMovementInteraction's targeted-mode switch when
// n.targetOp matches the corresponding NPCModePlayer* constant. Mirrors
// TS Engine-TS/src/engine/entity/Npc.ts:746-830. The `s *Server` arg is
// the standard mode-method signature shared with noMode / wanderMode /
// patrolMode / aiMode, and carries the logger through for diagnostics.
//
// Contract (inherited from the processMovementInteraction prelude):
//  - n.target != nil
//  - validateTarget() returned true
//  - n.typ != nil (enforced by validateTarget's maxrange branch)
//
// On *Player target-type mismatch, each method logs a warn and returns
// without mutating state. TS throws in this case (Npc.ts:748/804/817/823);
// Go's tick loop has no throw-and-recover scope, so we log-and-return.
// This is a minor deviation tracked in the NAI-13 spec § error handling.

// playerFaceMode — TS Npc.ts:815-819. No body beyond the type guard:
// the faceEntity mask bit is emitted by SetInteraction's
// `n.masks |= n.entitymask` line at the time the interaction was
// anchored, not per-tick here.
func (n *Npc) playerFaceMode(s *Server) {
	if _, ok := n.target.(*Player); !ok {
		s.log.Warn("playerFaceMode: non-Player target",
			"nid", n.nid, "targetOp", n.targetOp)
		return
	}
}

// playerFaceCloseMode — TS Npc.ts:821-829. Keeps the interaction active only
// while the target Player is within Chebyshev distance 1; otherwise clears
// the interaction via resetDefaults. The faceEntity mask bit is (as with
// playerFaceMode) emitted by the original SetInteraction call.
func (n *Npc) playerFaceCloseMode(s *Server) {
	p, ok := n.target.(*Player)
	if !ok {
		s.log.Warn("playerFaceCloseMode: non-Player target",
			"nid", n.nid, "targetOp", n.targetOp)
		return
	}

	// TS CoordGrid.distanceTo(this, target) — size-aware Chebyshev.
	// NAI-20 closes the size-approximation follow-up at this site.
	tx, tz, _ := p.Coords()
	tw, tl := approachEntitySize(n.target)
	if coordgrid.DistanceTo(n.x, n.z, n.size, n.size, tx, tz, tw, tl) > 1 {
		n.resetDefaults()
		return
	}
}

// playerFollowMode — TS Npc.ts:801-812. Each tick, path toward the Player's
// current tile and step one waypoint via pathToTarget (which dispatches to
// pathToTargetSmart / Naive per moveStrategy — see npc_interaction.go:546).
func (n *Npc) playerFollowMode(s *Server) {
	if _, ok := n.target.(*Player); !ok {
		s.log.Warn("playerFollowMode: non-Player target",
			"nid", n.nid, "targetOp", n.targetOp)
		return
	}
	n.pathToTarget()
	n.updateMovement(s)
}

// pickEscapeDirection returns the flee-step deltas (dx, dz) for a target
// relative to the NPC. Matches the quadrant if/else at the head of TS
// playerEscapeMode (d39e707d Npc.ts:778-787 — the per-quadrant wall-flag
// pairs were removed by that commit). Orientation in RS coords: +x = east,
// +z = north. Target at (+x, +z) from the NPC is NE of it → the NPC flees
// SW (delta (-1, -1)), etc.
func pickEscapeDirection(npcX, npcZ, targetX, targetZ int) (dx, dz int) {
	switch {
	case targetX >= npcX && targetZ >= npcZ:
		return -1, -1 // target NE → flee SW (TS: Direction.SOUTH_WEST)
	case targetX >= npcX && targetZ < npcZ:
		return -1, +1 // target SE → flee NW (TS: Direction.NORTH_WEST)
	case targetX < npcX && targetZ >= npcZ:
		return +1, -1 // target NW → flee SE (TS: Direction.SOUTH_EAST)
	default:
		return +1, +1 // target SW → flee NE (TS: Direction.NORTH_EAST)
	}
}

// playerEscapeMode — TS Npc.playerEscapeMode as rewritten by Engine-TS
// d39e707d "fix: Retreat logic (#97)". Each tick:
//
//  1. Type-guard on *Player (TS throws; Go logs + returns).
//  2. Abandon if SW-distance to target > 25 (unchanged).
//  3. Pick flee quadrant → diagonal candidate (mx, mz).
//  4. diagonalStepValid = canTravel(dx, dz) AND candidate within maxrange
//     of spawn (DistanceToSW <= maxrange — d39e707d widened the old
//     strict `<`). The pre-d39e707d wall-flag IsFlagged check (which
//     resetDefaults'd on a misread wall pair) is GONE — real step
//     validation replaces it.
//  5. Otherwise try the single-axis steps: primary = X-axis (mx, n.z),
//     secondary = Z-axis (n.x, mz) — TS spells four direction branches
//     that are all literally identical (X preferred over Z in every
//     quadrant), so a single arm is the faithful port. Each axis arm
//     needs BOTH canTravel and the maxrange bound; if neither is valid,
//     nothing is queued this tick.
//  6. updateMovement; a tick with no movement increments the stuck
//     counter (goscape: wanderCounter — TS d39e707d calls it
//     stuckCounter, the post-pin #91 rename of the same wanderCounter
//     field; updateMovement resets it on movement in both engines).
//  7. After 5+ stuck ticks, resetDefaults + counter reset — UNLESS the
//     NPC is already at max range from spawn on BOTH axes
//     (atMaxRangeBoth), in which case it holds position.
//
// canTravel needs a wired gamemap. When s.gamemap is nil (test fixtures
// without collision data) the travel checks pass — same convention as
// takeStep's fixture path.
func (n *Npc) playerEscapeMode(s *Server) {
	p, ok := n.target.(*Player)
	if !ok {
		s.log.Warn("playerEscapeMode: non-Player target",
			"nid", n.nid, "targetOp", n.targetOp)
		return
	}

	tx, tz, _ := p.Coords()

	// d39e707d pre-image :757-760 (unchanged) — abandon if already > 25
	// tiles SW-distance. TS uses distanceToSW (NOT distanceTo).
	if coordgrid.DistanceToSW(n.x, n.z, tx, tz) > 25 {
		n.resetDefaults()
		return
	}

	// Quadrant pick + flee-direction deltas.
	dx, dz := pickEscapeDirection(n.x, n.z, tx, tz)
	mx := n.x + dx
	mz := n.z + dz
	maxRange := int(n.typ.MaxRange)

	// TS: getCollisionStrategy() ?? CollisionType.NORMAL. (Since 2787f1fb
	// the nil case is exactly NoMove; the ?? keeps retreat stepping
	// validatable rather than nil-crashing.)
	collisionStrategy := collision.TypeNormal
	if cs := n.getCollisionStrategy(); cs != nil {
		collisionStrategy = *cs
	}
	extraFlag := n.blockWalkFlag()

	canTravel := func(ddx, ddz int) bool {
		if s == nil || s.gamemap == nil {
			// Test-fixture path: no gamemap → travel checks pass.
			// (goscape defensive; TS always has rsmod loaded.)
			return true
		}
		return s.gamemap.CanTravel(n.level, n.x, n.z, ddx, ddz, n.Width(), extraFlag, collisionStrategy)
	}

	diagonalTravelValid := canTravel(dx, dz)
	diagonalStepValid := diagonalTravelValid &&
		coordgrid.DistanceToSW(mx, mz, n.startX, n.startZ) <= maxRange

	if diagonalStepValid {
		n.QueueWaypoint(mx, mz)
	} else {
		// TS d39e707d: four identical direction branches — primary is
		// always the X-axis step (mx, this.z), secondary always the
		// Z-axis step (this.x, mz).
		primaryTravelValid := canTravel(dx, 0)
		secondaryTravelValid := canTravel(0, dz)
		primaryValid := primaryTravelValid &&
			coordgrid.DistanceToSW(mx, n.z, n.startX, n.startZ) <= maxRange
		secondaryValid := secondaryTravelValid &&
			coordgrid.DistanceToSW(n.x, mz, n.startX, n.startZ) <= maxRange

		if primaryValid {
			n.QueueWaypoint(mx, n.z)
		} else if secondaryValid {
			n.QueueWaypoint(n.x, mz)
		}
	}

	if !n.updateMovement(s) {
		n.wanderCounter++
	}

	// Both axes are checked INDEPENDENTLY (distX >= maxRange AND distZ >=
	// maxRange) — stricter than a single Chebyshev DistanceToSW >= maxRange;
	// mirrors the TS per-axis distanceToSW calls with one coord pinned.
	distX := n.x - n.startX
	if distX < 0 {
		distX = -distX
	}
	distZ := n.z - n.startZ
	if distZ < 0 {
		distZ = -distZ
	}
	atMaxRangeBoth := distX >= maxRange && distZ >= maxRange

	// Resets if it has been stuck for 5 ticks and is not at max range in
	// both directions.
	if n.wanderCounter >= 5 && !atMaxRangeBoth {
		n.resetDefaults()
		n.wanderCounter = 0
	}
}
