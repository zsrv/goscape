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

// sendUnsetMapFlag clears the client's pending map-click indicator.
func sendUnsetMapFlag(p *Player) {
	p.writeOut(gameserver.OpUnsetMapFlag, nil)
}

// SetInteraction anchors the interaction state machine on a target entity.
func (p *Player) SetInteraction(kind InteractionKind, target entity, op int) {
	p.target = target
	p.targetOp = op
	p.interactionKind = kind
	p.apRange = 10
	p.apRangeCalled = false
	p.interacted = false
	p.repathed = false
}

// ClearInteraction resets interaction state to idle.
func (p *Player) ClearInteraction() {
	p.target = nil
	p.targetOp = -1
	p.apRangeCalled = false
	p.interacted = false
	p.repathed = false
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
		if npc, ok := p.target.(*Npc); ok {
			p.SetFaceEntity(npc.nid)
		}
		p.interacted = true
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

// pathToTarget sets a waypoint to (tx, tz) via the existing move-click
// pathing pipeline so pathfinding (or direct-step mode) applies uniformly.
func (p *Player) pathToTarget(tx, tz int) {
	packed := []int{coordgrid.PackCoord(p.level, tx, tz)}
	needsFinding := !p.client.server.cfg.NodeClientRoutefinder
	p.pathToMoveClick(packed, needsFinding)
}
