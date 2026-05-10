package world

import (
	entitypkg "github.com/zsrv/goscape/pkg/entity"
)

// processPostDecode runs the per-tick post-decode block at TS
// Engine-TS/src/engine/World.ts:611-641. Called from end of processIn,
// before processInputTracking (matching TS L611-646 ordering).
//
// Activates the NAI-144 moveClickRequest gate at movement.go:64 by
// porting the L624-628 setter. Folds in the NAI-77 walktrigger
// fallback (L635-641), retiring processWalkTriggerFallbacks; this
// also closes NAI-77-D-WALKTRIGGER-FALLBACK-PHASE-CHOICE by shifting
// the fallback from after-processPathing to before-processPathing
// (TS-faithful slot).
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

	// TS L624-628: moveClickRequest setter. Activates the gate at
	// modules/world/movement.go:64 (NAI-144 — previously inert at HEAD).
	if !p.Busy() && p.opcalled {
		p.moveClickRequest = false
	} else {
		p.moveClickRequest = true
	}
}
