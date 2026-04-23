package world

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
