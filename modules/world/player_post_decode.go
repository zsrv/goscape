package world

import (
	entitypkg "github.com/zsrv/goscape/pkg/entity"
)

// processPostDecode runs the per-tick post-decode block at TS
// Engine-TS/src/engine/World.ts (rev-254 pin: :613-626). Called from end of
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
	// TS L611: isClientConnected(player) && player.decodeIn()
	if !p.decodedThisTick {
		return
	}
	// TS L613: userPath.length > 0 || opcalled
	if len(p.userPath) == 0 && !p.opcalled {
		return
	}

	// (goscape defensive; TS skips this check) — server may be nil in
	// fixtures that don't wire p.client.server. Bail safely.
	if p.client == nil || p.client.server == nil {
		return
	}

	// TS L614-617: delayed → unsetMapFlag and skip the rest of the block.
	if p.delayed {
		p.unsetMapFlag()
		return
	}

	// TS L619-622: faceEntity reset for non-PathingEntity targets.
	if p.faceEntity != -1 {
		switch p.target.(type) {
		case nil, *entitypkg.Loc, *entitypkg.Obj:
			p.faceEntity = -1
			p.masks |= p.entitymask
		}
	}

	// TS L620-624: moveClickRequest setter. Activates the gate at
	// modules/world/movement.go (NAI-144).
	if !p.Busy() && p.opcalled {
		p.moveClickRequest = false
	} else {
		p.moveClickRequest = true
	}
}
