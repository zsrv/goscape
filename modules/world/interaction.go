package world

import (
	"github.com/zsrv/goscape/pkg/coordgrid"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// InteractionKind distinguishes engine-triggered from script-queued
// interactions. S6a wired InteractionEngine for wire clicks (OpLoc,
// OpNpc, etc.). S6v wired InteractionScript as the kind set by
// p_op_loc / p_op_npc script opcodes. Both kinds fire the same AP/OP
// trigger dispatch via processInteraction — the kind is metadata for
// provenance, not a gate on trigger-firing.
type InteractionKind int

const (
	InteractionEngine InteractionKind = iota
	InteractionScript
)

// Sentinel targetOp values for non-op-numbered T/U interaction variants.
// OpLoc1..5/OpNpc1..5 use op = 1..5 (the op slot clicked); T/U variants
// use these sentinels so fireXxxTriggerYyy can dispatch to the correct
// single-trigger (e.g. APLOCT, OPNPCU). The targetOp interpretation is
// per-entity-type: tryFireXxxTrigger type-switches on p.target first,
// then each branch reads targetOp independently. Distinct NPC values
// (8, 9) chosen for clarity — reusing 6, 7 is safe via type-switch
// but less self-documenting.
const (
	targetOpLocT    = 6  // APLOCT / OPLOCT dispatch marker
	targetOpLocU    = 7  // APLOCU / OPLOCU dispatch marker
	targetOpNpcT    = 8  // APNPCT / OPNPCT dispatch marker (S6o)
	targetOpNpcU    = 9  // APNPCU / OPNPCU dispatch marker (S6o)
	targetOpPlayerT = 10 // APPLAYERT / OPPLAYERT dispatch marker (NAI-40)
	targetOpPlayerU = 11 // APPLAYERU / OPPLAYERU dispatch marker (NAI-40)
)

// sendUnsetMapFlag clears the client's pending map-click indicator.
func sendUnsetMapFlag(p *Player) {
	p.writeOut(gameserver.OpUnsetMapFlag, nil)
}

// SetInteraction anchors the interaction state machine on a target entity.
// For OpLocT the com parameter carries the spell-component ID; for OpLocU
// pass -1 (item tracking uses lastUseItem/lastUseSlot instead). For
// OpLoc1..5 and OpNpc1..5, callers pass -1.
//
// faceEntity dispatch mirrors TS PathingEntity.setInteraction
// (PathingEntity.ts:530-541) and the in-codebase Npc.SetInteraction
// template (npc_interaction.go:651-666). NAI-41 closed
// NAI-40-D-PLAYER-NO-FACEENTITY-ON-OPCLICK and the pre-existing
// *Player→*Npc contact-time-write divergence by moving the faceEntity
// write here from processInteraction's contact branch.
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

	switch t := target.(type) {
	case *Player:
		slot := t.slot + 32768
		if p.faceEntity != slot {
			p.faceEntity = slot
			p.masks |= p.entitymask
		}
	case *Npc:
		if p.faceEntity != t.nid {
			p.faceEntity = t.nid
			p.masks |= p.entitymask
		}
	default:
		// DEVIATION NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ: TS L542-545 sets
		// targetX = CoordGrid.fine(target.x, target.width) and targetZ
		// analogously for *Loc/*Obj targets. Player has no targetX/Z
		// fields and no consumer reads them; deferred to the focus/
		// step-tracking sub-spec that closes NAI-34-D3 (which already
		// touches Player fine-coord infra).
	}
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

// isFollowOp reports whether the current interaction is in chase-the-target
// mode. TS Player.ts:1205: followOp = targetOp == APPLAYER3 || OPPLAYER3.
// Goscape's targetOp is the raw op slot 1..4 (interaction.go:56), so a
// single equality check covers both AP and OP variants of slot 3. Player
// targets only — OPLOC/OPNPC/OPOBJ slot-3 ops are unrelated to the
// player→player chase semantics.
func isFollowOp(p *Player) bool {
	if p.targetOp != 3 {
		return false
	}
	_, ok := p.target.(*Player)
	return ok
}

// processInteraction runs once per tick per player after pathing.
// Mirrors TS Player.processInteraction (Player.ts:1200-1264).
//
// Branch summary:
//   - No target / no client / delayed: no-op.
//   - Target on different level: clear + UnsetMapFlag (subset of TS
//     validateTarget; goscape has no isValid()-style alive/visible
//     registry).
//   - Pre-step arm: walktrigger (skipped when followOp) + tryInteract.
//   - If pre-step did not interact: repath, post-step walktrigger (if
//     waypoints), waypoint-exhaustion clear (if followOp), post-step
//     tryInteract (skipped when followOp).
//   - Auto-clear: interacted && !apRangeCalled → ClearInteraction
//     (TS L1261-1263).
//
// Goscape's updateMovement runs in processPathing (tick.go:38), BEFORE
// processInteractions (tick.go:39). TS embeds it inline at L1241; the
// order-of-operations difference is by goscape design.
func (p *Player) processInteraction() {
	if p.target == nil {
		return
	}
	if p.client == nil || p.client.server == nil {
		return
	}
	s := p.client.server
	// DEVIATION NAI-44-D-CANACCESS-NO-STUN-CHECK: TS canAccess() also tests
	// stun/freeze; goscape has no stun system, so the !p.delayed subset is
	// the in-tree approximation.
	if p.delayed && s.currentTick < p.delayedUntil {
		return
	}

	// TS L1201-1202.
	p.followX = p.lastStepX
	p.followZ = p.lastStepZ
	// TS L1203 (this.nextTarget = null) — DEVIATION NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET:
	// goscape's p_op* opcodes do immediate SetInteraction swaps rather
	// than queueing a nextTarget for next-tick application. No nextTarget
	// field exists on *Player; the reshape below has no nextTarget block.
	// Closure: future p_op* opcode reshape sub-spec.

	followOp := isFollowOp(p)

	_, _, tlevel := p.target.Coords()
	if tlevel != p.level {
		p.ClearInteraction()
		sendUnsetMapFlag(p)
		return
	}

	interacted := false

	// Pre-step interact arm (TS L1209-1224).
	if !followOp {
		p.processWalktrigger()
	}
	interacted = p.tryInteract(false)

	// Post-step arm (TS L1227-1252). Skipped when pre-step interacted.
	if !interacted {
		// Recalc path (TS L1228-1229).
		if !p.repathed {
			tx, tz, _ := p.target.Coords()
			p.pathToTarget(tx, tz)
			p.repathed = true
		}

		if p.hasWaypoints() {
			p.processWalktrigger()
		}

		// followOp + waypoint exhaustion → clear (TS L1237-1239).
		if !p.hasWaypoints() && followOp {
			p.ClearInteraction()
		}

		// Post-step interact (TS L1244-1252). Skipped when followOp
		// (the chase keeps interaction anchored across steps).
		if p.target != nil && !followOp {
			interacted = p.tryInteract(p.stepsTaken == 0)
			if !interacted && !p.hasWaypoints() && p.stepsTaken == 0 {
				p.MessageGame("I can't reach that!")
				p.ClearInteraction()
			}
		}
	}

	// Auto-clear (TS L1261-1263). NAI-44 closure of
	// NAI-40-D-OPPLAYER3-FOLLOWOP-NOT-PORTED's auto-clear gap.
	// Note: followOp paths can still reach this when tryInteract returned
	// true at the pre-step arm (contact range with target=*Player op=3).
	// TS does the same — followOp gates SKIP_post-step-interact, not
	// the auto-clear itself.
	if interacted && !p.apRangeCalled {
		p.ClearInteraction()
	}

	// Tail mapflag clear (TS L1266-1268). When the player has consumed
	// at least one step this tick and no waypoints remain, clear the
	// client's pending map-click indicator. Without this, a player who
	// walks a full path without reaching the target (path blocked, target
	// moved out of reach) leaves the yellow X on screen until the next
	// click. Idempotent against the auto-clear above (which also nulls
	// waypoints via ClearInteraction).
	if !p.hasWaypoints() && p.stepsTaken > 0 {
		sendUnsetMapFlag(p)
	}
}

// hasWaypoints reports whether the player has an active waypoint queue.
// Goscape's convention: waypointIndex == -1 means no path; >= 0 means
// active. Mirrors TS Player.hasWaypoints(); the predicate is consumed by
// processInteraction's pre/post-step arms.
func (p *Player) hasWaypoints() bool {
	return p.waypointIndex >= 0
}

// processWalktrigger is the per-tick walktrigger consumption hook
// invoked by processInteraction's pre-step and post-step arms.
//
// DEVIATION NAI-44-D-PLAYER-WALKTRIGGER-NOOP: TS Player.ts:1219-1234
// calls processWalktrigger which dispatches the player's queued
// walktrigger script. Goscape has no walktrigger consumer yet (sibling
// to NAI-37-D-WALKTRIGGER-NOREADER on the Npc side at npc.go:92).
// Empty no-op preserves TS-faithful processInteraction shape so a
// future consumer can wire here without further reshape.
func (p *Player) processWalktrigger() {}

// tryInteract is the contact/approach-distance dispatch unifying the
// OP and AP arms that processInteraction previously inlined.
// Returns true when an OP or AP trigger fired this tick.
//
// allowOpScenery gates the OP branch for non-PathingEntity targets
// (Loc, Obj). Mirrors TS Player.tryInteract(allowOpScenery: boolean)
// at Player.ts:1113. Callers:
//   - pre-step (always false): scenery OP blocked before movement
//   - post-step (stepsTaken==0): scenery OP allowed only if no walk
//
// NPC side equivalent: (*Npc).tryInteract(s, allowOpScenery bool)
// at npc_interaction.go:247.
func (p *Player) tryInteract(allowOpScenery bool) bool {
	tx, tz, _ := p.target.Coords()
	if inOperableDistance(p.x, p.z, tx, tz) {
		_, isNpc := p.target.(*Npc)
		_, isPlayer := p.target.(*Player)
		if isNpc || isPlayer || allowOpScenery {
			p.interacted = true
			if !p.interactionFired {
				tryFireOpTrigger(p)
			}
			return true
		}
		// Loc/Obj + !allowOpScenery: fall through to AP check.
	}
	if inApproachDistance(p.x, p.z, tx, tz, effectiveApRange(p)) {
		p.interacted = true
		if !p.interactionFired {
			tryFireApTrigger(p)
		}
		return true
	}
	return false
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

// effectiveApRange returns the approach-range in tiles the player's
// current target should be checked against by inApproachDistance.
// For *Npc targets: the NPC's NpcType.AttackRange (fixed per-type,
// never mutated). For *Loc and all other targets: p.apRange (the
// mutable Player field, defaulted to 10 in SetInteraction and
// settable via p_aprange per S6l).
//
// Matches TS Npc.checkApTrigger (Npc.ts:~876) which reads
// type.attackrange, diverging from Player.tryInteract (Player.ts:~1139)
// which reads player.apRange.
//
// Returns 0 (which inApproachDistance rejects) if the target is an
// NPC with a nil NpcType — defensive guard; production cache always
// registers NpcType for any spawned NPC. Edge case: NpcType with
// AttackRange == 0 (uninitialized) will also yield 0 here, meaning
// APNPC never fires for that NPC. Intentional — production cache
// always sets attackrange for NPCs that have AP scripts.
func effectiveApRange(p *Player) int {
	if npc, ok := p.target.(*Npc); ok {
		if npc.typ == nil {
			return 0
		}
		return int(npc.typ.AttackRange)
	}
	return p.apRange
}

// pathToTarget sets a waypoint to (tx, tz) via the existing move-click
// pathing pipeline so pathfinding (or direct-step mode) applies uniformly.
func (p *Player) pathToTarget(tx, tz int) {
	packed := []int{coordgrid.PackCoord(p.level, tx, tz)}
	needsFinding := !p.client.server.cfg.NodeClientRoutefinder
	p.pathToMoveClick(packed, needsFinding)
}
