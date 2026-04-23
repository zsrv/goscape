package world

// PLAYER* NPC mode implementations. Each per-tick mode method is dispatched
// from (*Npc).processMovementInteraction's targeted-mode switch when
// n.targetOp matches the corresponding NPCModePlayer* constant. Mirrors
// TS Engine-TS/src/engine/entity/Npc.ts:746-830.
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
// anchored, not per-tick here. The `s` arg is taken for symmetry with
// other mode methods (noMode / wanderMode / aiMode) and is unused.
func (n *Npc) playerFaceMode(s *Server) {
	if _, ok := n.target.(*Player); !ok {
		if n.server != nil && n.server.log != nil {
			n.server.log.Warn("playerFaceMode: non-Player target",
				"nid", n.nid, "targetOp", n.targetOp)
		}
		return
	}
}
