package world

// processPostDecode runs the per-tick post-decode block at TS
// Engine-TS/src/engine/World.ts (rev-254 pin @2e3bcf43: :614-627). Called from end of
// processIn, before processInputTracking (matching TS ordering).
//
// Activates the NAI-144 moveClickRequest gate at movement.go by porting the
// setter. f0ccbe8a removed the rest of the block: the op-driven pathToTarget
// shortcut and the NODE_WALKTRIGGER_SETTING pathToMoveClick/walktrigger
// fallbacks are gone — pathing for a move click now happens entirely in the
// MoveClick handler (queueWaypoints at decode time), and the op-driven
// initial path comes from pathToPathingTarget/pathToTarget in
// processInteraction.
func (p *Player) processPostDecode() {
	// TS L614: isClientConnected(player) && player.decodeIn()
	if !p.decodedThisTick {
		return
	}
	// TS L615: userPath.length > 0 || opcalled
	if len(p.userPath) == 0 && !p.opcalled {
		return
	}

	// (goscape defensive; TS skips this check) — server may be nil in
	// fixtures that don't wire p.client.server. Bail safely.
	if p.client == nil || p.client.server == nil {
		return
	}

	// TS L616-619: delayed → unsetMapFlag and skip the rest of the block.
	if p.delayed {
		p.unsetMapFlag()
		return
	}

	// ee28c1aa @2e3bcf43 REMOVED the faceEntity-reset block that lived here
	// (old TS: `if ((!player.target || target instanceof Loc || Obj) &&
	// player.faceEntity !== -1) { faceEntity = -1; masks |= entitymask; }`)
	// — facing for nil/Loc/Obj targets is now cleared by the per-tick
	// setFaceEntity() derivation (face_entity.go).

	// TS L621-625: moveClickRequest setter. Activates the gate at
	// modules/world/movement.go (NAI-144).
	if !p.Busy() && p.opcalled {
		p.moveClickRequest = false
	} else {
		p.moveClickRequest = true
	}
}
