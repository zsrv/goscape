package world

import (
	"github.com/zsrv/goscape/pkg/coordgrid"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
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
	targetOpObjT    = 12 // APOBJT / OPOBJT dispatch marker (NAI-50)
	targetOpObjU    = 13 // APOBJU / OPOBJU dispatch marker (NAI-50)
)

// sendUnsetMapFlag clears the client's pending map-click indicator.
func sendUnsetMapFlag(p *Player) {
	p.writeOut(gameserver.OpUnsetMapFlag, nil)
}

// SetInteraction anchors the interaction state machine on a target entity.
// The com parameter carries:
//   - OpLocT/OpNpcT/OpObjT/OpPlayerT: spellCom (UI component ID of the spell).
//   - OpPlayerU: useObj (the obj/item ID used on the target player; NAI-62
//     producer fix per TS OpPlayerUHandler.ts:77).
//   - OpLoc1..5 / OpNpc1..5 / OpObj1..5 / OpLocU / OpNpcU / OpObjU: -1.
// Storage canonicalises com=0 → -1 (NAI-62, matching TS truthy
// PathingEntity.ts:520) so the lookup-side != -1 override check in
// resolveTriggerTypeId behaves identically to TS !== -1.
//
// faceEntity dispatch mirrors TS PathingEntity.setInteraction
// (PathingEntity.ts:530-541) and the in-codebase Npc.SetInteraction
// template (npc_interaction.go:651-666). NAI-41 closed
// NAI-40-D-PLAYER-NO-FACEENTITY-ON-OPCLICK and the pre-existing
// *Player→*Npc contact-time-write divergence by moving the faceEntity
// write here from processInteraction's contact branch.
//
// TS PathingEntity.ts:528 — focus() is called on every SetInteraction with
// the target's fine coord. The four (kind × target-shape) wire-write cases:
//   - Npc/Player target (any kind): instant=false — faceAngle written only.
//   - Loc/Obj target + InteractionEngine: instant=true — faceAngle + faceSquare
//     written, MaskFaceCoord ORed. Mirrors (*Npc).SetInteraction at
//     modules/world/npc_interaction.go:657-666.
//   - Loc/Obj target + InteractionScript: instant=false — faceAngle only.
func (p *Player) SetInteraction(kind InteractionKind, target entity, op, com int) {
	p.target = target
	p.targetOp = op
	// TS PathingEntity.ts:520 truthy: com=0 → -1. Lookup-side checks
	// use != -1, so canonicalising at storage means a single sentinel
	// reaches resolveTriggerTypeId.
	if com == 0 {
		p.targetSubject.com = -1
	} else {
		p.targetSubject.com = com
	}
	p.interactionKind = kind
	p.apRange = 10
	p.apRangeCalled = false
	p.interacted = false
	p.repathed = false
	p.interactionFired = false

	// TS PathingEntity.ts:528 — focus on the target's fine coord.
	// instant=true ⇔ NonPathingEntity (Loc/Obj) clicked via the engine
	// (kind == InteractionEngine). Any other combination passes
	// instant=false: faceAngle still written, but faceSquare/mask are
	// not. Mirrors (*Npc).SetInteraction at modules/world/npc_interaction.go:657-666.
	tx, tz, _ := target.Coords()
	tw, tl := targetWidthLength(target)
	fx := coordgrid.Fine(tx, tw)
	fz := coordgrid.Fine(tz, tl)
	isNonPathing := false
	switch target.(type) {
	case *entitypkg.Loc, *entitypkg.Obj:
		isNonPathing = true
	}
	p.focus(fx, fz, isNonPathing && kind == InteractionEngine)

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
		// Loc/Obj target — cache fine-coord for reorient consumption.
		// TS PathingEntity.ts:542-545. Closes
		// NAI-41-D-PLAYER-NO-LOCOBJ-TARGETXZ in NAI-66 (consumer is
		// (*Player).reorient at modules/world/movement.go).
		p.targetX = fx
		p.targetZ = fz
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
	// TS L1203.
	p.nextTarget = nil

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

	// nextTarget pop + auto-clear (TS L1255-1263). When an OP/AP
	// trigger script called p_op_* mid-trigger, the fire helpers
	// captured the script-set target into p.nextTarget; pop it here.
	// Otherwise, auto-clear the interaction. followOp paths can still
	// reach the else-if when tryInteract returned true at the pre-step
	// arm (contact range with target=*Player op=3); TS does the same —
	// followOp gates SKIP post-step-interact, not the auto-clear
	// itself. NAI-68 closed NAI-44-D-IMMEDIATE-POP-VS-NEXTTARGET via
	// this reshape; NAI-69 closes NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED
	// by routing the same-tick retry signal through tryInteract.
	if p.nextTarget != nil {
		p.target = p.nextTarget
	} else if interacted && !p.apRangeCalled {
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
// invoked by processInteraction's pre-step and post-step arms. Looks up
// the queued script id, clears the field BEFORE the script-found check
// (TS clear-before-check semantics at Player.ts:1064), then dispatches
// via runScript with protect=true. Mirrors TS Player.processWalktrigger
// at Player.ts:1057-1070.
//
// The !p.protectedScriptActive() gate mirrors TS L1062 !this.protect via
// goscape's documented activeScript.Protect convergence (see CanAccess
// doc-comment in player_script.go).
func (p *Player) processWalktrigger() {
	if p.walktrigger == -1 || p.delayed || p.protectedScriptActive() {
		return
	}
	if p.client == nil || p.client.server == nil {
		return
	}
	s := p.client.server
	sf := s.scriptProvider.GetByID(uint32(p.walktrigger))
	p.walktrigger = -1
	if sf == nil {
		return
	}
	s.runScript(sf, p, nil, true, nil, nil)
}

// tryInteract is the contact/approach-distance dispatch unifying the
// OP and AP arms that processInteraction previously inlined. Mirrors
// LostCityRS/Engine-TS Player.tryInteract at Player.ts:1113-1184.
//
// 4-branch dispatch (after NAI-78). Resolves opTrigger/apTrigger via
// getOpTrigger/getApTrigger (interaction_trigger.go) at entry, then
// dispatches:
//
//	1. opTrigger != nil && (PathingEntity || allowOpScenery) && operable
//	   → fire OP, return true.
//	2. apTrigger != nil && approach
//	   → fire AP, return true (or false on NAI-69 same-tick retry).
//	3. approach (apTrigger nil)
//	   → apRange=-1, return false. Allows processInteraction's post-step
//	   to run pathToTarget + tryInteract(allowOpScenery=true).
//	4. (PathingEntity || allowOpScenery) && operable (opTrigger nil)
//	   → defaultOp NIH ("Nothing interesting happens." + clear waypoints),
//	   return true.
//
// Fixes NAI-78 root cause: pre-NAI-78, the 2-branch shape returned true
// from the AP block even when no AP script existed, which gated the
// post-step branch off and let the auto-clear nuke the anchor — the
// Tutorial Island RS Guide door symptom. Branch 3's `return false`
// closure is the load-bearing change.
//
// allowOpScenery gates branches 1 and 4 for non-PathingEntity targets
// (Loc, Obj). Mirrors TS Player.tryInteract(allowOpScenery: boolean).
// Callers:
//   - pre-step (always false): scenery OP blocked before movement
//   - post-step (stepsTaken==0): scenery OP allowed only if no walk
//
// NPC side equivalent: (*Npc).tryInteract(s, allowOpScenery bool)
// at npc_interaction.go:247.
func (p *Player) tryInteract(allowOpScenery bool) bool {
	if p.target == nil {
		return false
	}
	// DEVIATION NAI-78-D-HASINTERACTION-GUARD: TS Player.ts:1114 also
	// gates on `!this.hasInteraction()` (false for follow-op:
	// APPLAYER3 / OPPLAYER3). Pre-existing gap — was absent in the
	// 2-branch shape too. NAI-78 shifts the path the case follows (now
	// branch 3 → followOp post-step gate at processInteraction:221
	// rather than direct OP-block dispatch) but the underlying gap
	// is unchanged. Defer port alongside the rest of the follow-op
	// semantics.
	srv := p.client.server

	opTrigger := getOpTrigger(p, srv)
	apTrigger := getApTrigger(p, srv)

	tx, tz, _ := p.target.Coords()
	operable := inOperableDistance(p.x, p.z, tx, tz)
	approach := inApproachDistance(p.x, p.z, tx, tz, effectiveApRange(p))

	isPathing := false
	switch p.target.(type) {
	case *Npc, *Player:
		isPathing = true
	}

	// Branch 1 — OP fire (TS Player.ts:1123).
	if opTrigger != nil && (isPathing || allowOpScenery) && operable {
		p.interacted = true
		if !p.interactionFired {
			tryFireOpTrigger(p)
		}
		return true
	}

	// Branch 2 — AP fire (TS Player.ts:1139).
	if apTrigger != nil && approach {
		p.interacted = true
		if !p.interactionFired {
			tryFireApTrigger(p)
		}
		// NAI-69 same-tick retry (TS Player.ts:1158-1167). Closes
		// NAI-68-D-AP-APRANGE-REVERT-NOT-PORTED.
		if p.nextTarget == nil && p.apRangeCalled {
			p.interactionFired = false
			return false
		}
		return true
	}

	// Branch 3 — default-AP no-op (TS Player.ts:1173-1175).
	// Player is in approach distance but no [ap…] script exists.
	// Force apRange = -1 so the AP block can never re-enter on this
	// interaction, then return false to let processInteraction's
	// post-step branch run (pathToTarget → walktrigger → post-step
	// tryInteract with allowOpScenery=true so branch 1 can fire OP).
	if approach {
		p.apRange = -1
		return false
	}

	// Branch 4 — default-OP NIH (TS Player.ts:1179-1182).
	if (isPathing || allowOpScenery) && operable {
		defaultOp(p)
		return true
	}

	return false
}

// defaultOp implements the NIH (Not-Implemented-Here) fallback fired by
// tryInteract branch 4 when the player reaches operable distance but no
// [op…] script is registered. Mirrors LostCityRS/Engine-TS
// Player.ts:1072-1097.
//
// DEVIATION NAI-78-D-DEBUG-MSG-DEFERRED: TS Player.ts:1076-1093 emits a
// `[debug] No trigger for [op<entity><op>,<typeName>]` chat message under
// `!NODE_PRODUCTION`. Goscape's analogue is `Cfg.NodeDebug` (config.go:76,
// "Extra debug info, e.g. missing triggers"), but no other interaction-tier
// missing-trigger handler consults it today. The debug-emit port is
// deferred to a future sub-spec for cross-handler consistency.
func defaultOp(p *Player) {
	p.MessageGame("Nothing interesting happens.")
	p.waypointIndex = -1 // TS Player.ts:1096 — clearWaypoints()
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
