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
	// NAI-13 inherits the 1,1,1,1 size approximation tracked as the
	// NAI-12 "size-aware LoS" follow-up; single-tile NPCs + single-tile
	// Players reduce this to plain max(|dx|, |dz|).
	tx, tz, _ := p.Coords()
	dx := n.x - tx
	if dx < 0 {
		dx = -dx
	}
	dz := n.z - tz
	if dz < 0 {
		dz = -dz
	}
	if max(dx, dz) > 1 {
		n.resetDefaults()
		return
	}
}

// playerFollowMode — TS Npc.ts:801-812. Each tick, path toward the Player's
// current tile and step one waypoint. Naive-only: the SMART A* branch in
// pathToTarget is deferred from NAI-11 (PathingEntity.ts:457-508).
func (n *Npc) playerFollowMode(s *Server) {
	if _, ok := n.target.(*Player); !ok {
		s.log.Warn("playerFollowMode: non-Player target",
			"nid", n.nid, "targetOp", n.targetOp)
		return
	}
	n.pathToTarget()
	n.updateMovement(s)
}

// escapeDirection captures the per-quadrant flee data for playerEscapeMode.
// Each record maps "where the player is relative to the NPC" to the flee
// step delta, the wall-flag pair that blocks the candidate tile, and the
// axis-fallback formula. Mirrors TS Npc.ts:758-797's quadrant if/else.
type escapeDirection struct {
	dx, dz    int
	wallFlags int
	// fallbackUseNpcX: true → fallback waypoint is (n.x, mz)  [N variants]
	//                  false → fallback waypoint is (mx, n.z) [S variants]
	fallbackUseNpcX bool
}

// pickEscapeDirection picks the quadrant record for a target relative to
// the NPC. Matches TS Npc.ts:758-770 exactly. Orientation in RS coords:
// +x = east, +z = north. So target at (+x, +z) from NPC is NE of NPC; NPC
// flees SW (delta (-1, -1)), checking WALL_SOUTH|WALL_WEST on the candidate
// tile. Fallback-axis comments: "NPC fallback X" means the fallback
// waypoint uses the NPC's x and the candidate's z (keeps X fixed).
func pickEscapeDirection(npcX, npcZ, targetX, targetZ int) escapeDirection {
	switch {
	case targetX >= npcX && targetZ >= npcZ:
		// Target NE of NPC → flee SW. TS: direction = SOUTH_WEST.
		// fallbackUseNpcX=false → axis fallback moves along X (mx, n.z).
		return escapeDirection{dx: -1, dz: -1,
			wallFlags:       collision.FlagWallSouth | collision.FlagWallWest,
			fallbackUseNpcX: false}
	case targetX >= npcX && targetZ < npcZ:
		// Target SE of NPC → flee NW. TS: direction = NORTH_WEST.
		// fallbackUseNpcX=true → axis fallback moves along Z (n.x, mz).
		return escapeDirection{dx: -1, dz: +1,
			wallFlags:       collision.FlagWallNorth | collision.FlagWallWest,
			fallbackUseNpcX: true}
	case targetX < npcX && targetZ >= npcZ:
		// Target NW of NPC → flee SE. TS: direction = SOUTH_EAST.
		// fallbackUseNpcX=false → axis fallback moves along X (mx, n.z).
		return escapeDirection{dx: +1, dz: -1,
			wallFlags:       collision.FlagWallSouth | collision.FlagWallEast,
			fallbackUseNpcX: false}
	default:
		// Target SW of NPC → flee NE. TS: direction = NORTH_EAST.
		// fallbackUseNpcX=true → axis fallback moves along Z (n.x, mz).
		return escapeDirection{dx: +1, dz: +1,
			wallFlags:       collision.FlagWallNorth | collision.FlagWallEast,
			fallbackUseNpcX: true}
	}
}

// playerEscapeMode — TS Npc.ts:746-799. Each tick:
//
//  1. Type-guard on *Player.
//  2. Abandon if SW-distance to target > 25.
//  3. Pick flee quadrant → candidate tile.
//  4. If the candidate tile's wall flags block the flee direction,
//     resetDefaults (can't move there; give up).
//  5. If the candidate is still within the NPC's MaxRange of its start,
//     queue a diagonal waypoint at (mx, mz).
//  6. Otherwise fall back to a single-axis waypoint: NE/NW → (n.x, mz);
//     SE/SW → (mx, n.z). This is the "walk along other axis" branch.
//
// Step 4's wall check requires a wired gamemap. When s.gamemap is nil
// (test fixtures that don't seed collision data), the wall check is
// skipped — same convention as NAI-12's inApproachDistance LoS short-circuit.
func (n *Npc) playerEscapeMode(s *Server) {
	p, ok := n.target.(*Player)
	if !ok {
		s.log.Warn("playerEscapeMode: non-Player target",
			"nid", n.nid, "targetOp", n.targetOp)
		return
	}

	tx, tz, _ := p.Coords()

	// TS :751-754 — abandon if already > 25 tiles SW-distance.
	if coordgrid.DistanceToSW(n.x, n.z, tx, tz) > 25 {
		n.resetDefaults()
		return
	}

	// TS :756-770 — quadrant pick + flee-direction deltas.
	dir := pickEscapeDirection(n.x, n.z, tx, tz)
	mx := n.x + dir.dx
	mz := n.z + dir.dz

	// TS :775-778 — wall-flag check. Skip when gamemap is nil (test fixture).
	if s != nil && s.gamemap != nil &&
		s.gamemap.Pathfinder.Flags.IsFlagged(mx, mz, n.level, dir.wallFlags) {
		n.resetDefaults()
		return
	}

	// TS :780-790 — within-maxrange diagonal waypoint.
	if coordgrid.DistanceToSW(mx, mz, n.startX, n.startZ) < int(n.typ.MaxRange) {
		n.queueWaypoint(mx, mz)
		n.updateMovement(s)
		return
	}

	// TS :793-797 — axis fallback.
	if dir.fallbackUseNpcX {
		n.queueWaypoint(n.x, mz)
	} else {
		n.queueWaypoint(mx, n.z)
	}
	n.updateMovement(s)
}
