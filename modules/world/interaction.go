package world

import (
	"github.com/zsrv/goscape/pkg/coordgrid"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// InteractionKind distinguishes engine-triggered from script-queued
// interactions. Only InteractionEngine is used in sub-spec 6a;
// InteractionScript is reserved for the RuneScript integration.
type InteractionKind int

const (
	InteractionEngine InteractionKind = iota
	InteractionScript
)

// Sentinel targetOp values for non-op-numbered Loc interaction types.
// OpLoc1..5 use op = 1..5 (the op slot clicked); T and U variants use
// these sentinels so fireXxxTriggerLoc can dispatch to the correct
// single-trigger (APLOCT/OPLOCT or APLOCU/OPLOCU). Matches TS's model
// where setInteraction stores the APLOC trigger value directly;
// goscape uses sentinel integers instead (see S6j-D3 convention).
const (
	targetOpLocT = 6 // APLOCT / OPLOCT dispatch marker
	targetOpLocU = 7 // APLOCU / OPLOCU dispatch marker
)

// sendUnsetMapFlag clears the client's pending map-click indicator.
func sendUnsetMapFlag(p *Player) {
	p.writeOut(gameserver.OpUnsetMapFlag, nil)
}

// SetInteraction anchors the interaction state machine on a target entity.
// For OpLocT the com parameter carries the spell-component ID; for OpLocU
// pass -1 (item tracking uses lastUseItem/lastUseSlot instead). For
// OpLoc1..5 and OpNpc1..5, callers pass -1.
func (p *Player) SetInteraction(kind InteractionKind, target entity, op, com int) {
	p.target = target
	p.targetOp = op
	p.targetSubject.com = com
	p.interactionKind = kind
	p.apRange = 10
	p.apRangeCalled = false
	p.interacted = false
	p.repathed = false
	p.interactionFired = false
}

// ClearInteraction resets interaction state to idle.
func (p *Player) ClearInteraction() {
	p.target = nil
	p.targetOp = -1
	p.apRange = 10 // S6l: reset to default (TS PathingEntity.ts:554)
	p.apRangeCalled = false
	p.interacted = false
	p.repathed = false
	p.interactionFired = false
}

// processInteraction runs once per tick per player after pathing.
//   - No target: no-op.
//   - Delayed: no-op.
//   - Target on different level: clear + UnsetMapFlag.
//   - In operable distance: face target, interacted=true.
//   - Out of range: set waypoint toward target.
func (p *Player) processInteraction() {
	if p.target == nil {
		return
	}
	if p.client == nil || p.client.server == nil {
		return
	}
	s := p.client.server
	if p.delayed && s.currentTick < p.delayedUntil {
		return
	}

	tx, tz, tlevel := p.target.Coords()
	if tlevel != p.level {
		p.ClearInteraction()
		sendUnsetMapFlag(p)
		return
	}

	if inOperableDistance(p.x, p.z, tx, tz) {
		// Contact range — fire OP. Matches TS Player.ts:1123-1135
		// (OP checked before AP at contact).
		if npc, ok := p.target.(*Npc); ok {
			p.SetFaceEntity(npc.nid)
		}
		p.interacted = true
		if p.interactionKind == InteractionEngine && !p.interactionFired {
			tryFireOpTrigger(p)
		}
		return
	}

	if inApproachDistance(p.x, p.z, tx, tz, p.apRange) {
		// Approach range — fire AP. Matches TS Player.ts:1139-1170.
		// DEVIATION S6l-D1: goscape skips TS's apRange=-1 sentinel
		// optimization; each tick does a fresh provider lookup.
		p.interacted = true
		if p.interactionKind == InteractionEngine && !p.interactionFired {
			tryFireApTrigger(p)
		}
		return
	}

	if !p.repathed {
		p.pathToTarget(tx, tz)
		p.repathed = true
	}
}

// inOperableDistance is Chebyshev <= 1 between (px,pz) and (tx,tz),
// excluding the same tile. Adjacent (including diagonals) counts as
// operable for 1x1 targets. Multi-tile + strict-adjacency come with
// real combat.
func inOperableDistance(px, pz, tx, tz int) bool {
	dx := px - tx
	if dx < 0 {
		dx = -dx
	}
	dz := pz - tz
	if dz < 0 {
		dz = -dz
	}
	if dx > 1 || dz > 1 {
		return false
	}
	return !(dx == 0 && dz == 0)
}

// inApproachDistance returns true when (px,pz) is within apRange
// Chebyshev tiles of (tx,tz), excluding the same tile. Range-portion
// of TS PathingEntity.inApproachDistance, sans LOS (DEVIATION S6l-D4).
// apRange <= 0 always returns false — the caller is responsible for
// distinguishing "not yet in range" from "no AP script exists."
func inApproachDistance(px, pz, tx, tz, apRange int) bool {
	if apRange <= 0 {
		return false
	}
	dx := px - tx
	if dx < 0 {
		dx = -dx
	}
	dz := pz - tz
	if dz < 0 {
		dz = -dz
	}
	if dx > apRange || dz > apRange {
		return false
	}
	return !(dx == 0 && dz == 0)
}

// pathToTarget sets a waypoint to (tx, tz) via the existing move-click
// pathing pipeline so pathfinding (or direct-step mode) applies uniformly.
func (p *Player) pathToTarget(tx, tz int) {
	packed := []int{coordgrid.PackCoord(p.level, tx, tz)}
	needsFinding := !p.client.server.cfg.NodeClientRoutefinder
	p.pathToMoveClick(packed, needsFinding)
}
