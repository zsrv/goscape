package world

import (
	"fmt"
	"strconv"

	"github.com/zsrv/goscape/pkg/coordgrid"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/pathfinder/reach"
	"github.com/zsrv/goscape/pkg/script"
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

// unsetMapFlag clears the player's waypoint queue and emits the
// OpUnsetMapFlag packet. Mirrors TS Player.unsetMapFlag
// (Engine-TS/.../Player.ts:2169-2172) — the bundled
// clearWaypoints + write helper. Distinct from the wire-only
// sendUnsetMapFlag(p), which is preserved for decode-time handler
// call sites that already manage waypoint state inline.
//
// Per memory ts_helper_method_bundles.md: when porting a TS site
// that calls unsetMapFlag(), use this method, not sendUnsetMapFlag.
func (p *Player) unsetMapFlag() {
	p.waypointIndex = -1
	sendUnsetMapFlag(p)
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

	// NAI-79 Stage 1 — pre-step state capture for Frame B emit at tail.
	// All target-coord fields refer to the INITIAL target; target_still_set
	// separately signals whether p.target was nulled during the tick.
	// Trigger-lookup capture is gated on NodeDebug to avoid the per-tick
	// hot-path cost of two extra GetByTrigger calls when instrumentation
	// is disabled. Frame B emission gates again on NodeDebug at the tail.
	hadTarget := true
	initialTarget := p.target
	initialTargetX, initialTargetZ, _ := p.target.Coords()
	var opTriggerPresent, apTriggerPresent bool
	if s.cfg.NodeDebug {
		opTriggerPresent = getOpTrigger(p, s) != nil
		apTriggerPresent = getApTrigger(p, s) != nil
	}
	p.lastInteractBranchPre = 0
	p.lastInteractBranchPost = 0
	p.interactCallSlot = 0

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
		// NAI-79 Stage 1 — emit Frame B even on level-mismatch clear so
		// the captured log shows the cross-level ClearInteraction case.
		emitInteractionTickFrame(s, p, hadTarget, initialTarget,
			initialTargetX, initialTargetZ, opTriggerPresent,
			apTriggerPresent, false /*interactedFinal*/)
		return
	}

	interacted := false

	// Pre-step interact arm (TS L1209-1224).
	if !followOp {
		p.processWalktrigger()
	}
	p.interactCallSlot = 0
	interacted = p.tryInteract(false)

	// Post-step arm (TS L1227-1252). Skipped when pre-step interacted.
	if !interacted {
		// Recalc path (TS L1228-1229).
		p.pathToPathingTarget()

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
			p.interactCallSlot = 1
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

	// NAI-79 Stage 1 — Frame B emit at tail. Gated on hadTarget (a
	// tick with no target at entry should never emit) and NodeDebug.
	emitInteractionTickFrame(s, p, hadTarget, initialTarget,
		initialTargetX, initialTargetZ, opTriggerPresent,
		apTriggerPresent, interacted)
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
// goscape's documented activeScript.Pointers&PtrProtectedActivePlayer
// convergence (see CanAccess doc-comment in player_script.go).
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
	// NAI-147 T5 closes NAI-78-D-HASINTERACTION-GUARD — TS
	// Player.ts:1114 3-part guard. !HasInteraction() filters follow-op
	// (targetOp=3 with *Player target). !CanAccess() filters delayed,
	// modal, and protected-script states. CanAccess() is STRICTER than
	// processInteraction:196's delayed-only check; modal/protected-script
	// short-circuits previously reachable through tryInteract are now
	// blocked here. Mirrors TS canAccess semantics via NAI-111 narrowed
	// convergence.
	if p.target == nil || !p.HasInteraction() || !p.CanAccess() {
		recordTryInteractBranch(p, 0) // NAI-79 Stage 1 (combined early-return)
		return false
	}
	srv := p.client.server

	opTrigger := getOpTrigger(p, srv)
	apTrigger := getApTrigger(p, srv)

	tx, tz, _ := p.target.Coords()
	operable := inOperableDistance(p, p.target)
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
		recordTryInteractBranch(p, 1) // NAI-79 Stage 1
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
			recordTryInteractBranch(p, 2) // NAI-79 Stage 1 (retry-no-op)
			return false
		}
		recordTryInteractBranch(p, 2) // NAI-79 Stage 1
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
		recordTryInteractBranch(p, 3) // NAI-79 Stage 1
		return false
	}

	// Branch 4 — default-OP NIH (TS Player.ts:1179-1182).
	if (isPathing || allowOpScenery) && operable {
		defaultOp(p, opTrigger, apTrigger)
		recordTryInteractBranch(p, 4) // NAI-79 Stage 1
		return true
	}

	recordTryInteractBranch(p, 0) // NAI-79 Stage 1 (fallthrough)
	return false
}

// defaultOp implements the NIH (Not-Implemented-Here) fallback fired by
// tryInteract branch 4 when the player reaches operable distance but no
// [op…] script is registered. Mirrors LostCityRS/Engine-TS
// Player.ts:1072-1097.
//
// NAI-147 T4 closes NAI-78-D-DEBUG-MSG-DEFERRED: under cfg.NodeDebug
// (TS !NODE_PRODUCTION analogue) and both triggers nil, emit the TS
// L1076-1093 debug chat. NAI-147-D-TRIGGER-NAME-NUMERIC: trigger name
// emitted in numeric form because pkg/script.ServerTriggerType has no
// String() table — adding a 50+ entry name table for one debug-only
// chat is over-investment.
func defaultOp(p *Player, opTrigger, apTrigger *script.ScriptFile) {
	if p.client != nil && p.client.server != nil {
		s := p.client.server
		if s.cfg.NodeDebug && opTrigger == nil && apTrigger == nil {
			debugname := defaultOpDebugname(p, s)
			p.MessageGame(fmt.Sprintf("No trigger for [%d,%s]", p.targetOp+7, debugname))
		}
	}
	p.MessageGame("Nothing interesting happens.")
	p.waypointIndex = -1 // TS Player.ts:1096 — clearWaypoints()
}

// defaultOpDebugname mirrors TS Player.ts:1077-1090 fan-out, returning
// a human-readable name for the player's current target. Used only by
// defaultOp's NodeDebug-gated chat. Internal — not exported.
func defaultOpDebugname(p *Player, s *Server) string {
	switch tgt := p.target.(type) {
	case *Npc:
		if tgt.typ != nil && tgt.typ.DebugName != "" {
			return tgt.typ.DebugName
		}
		return strconv.Itoa(tgt.typeId)
	case *entitypkg.Loc:
		typeId := tgt.Type()
		if s.locTypes != nil && typeId >= 0 && typeId < len(s.locTypes.Configs) {
			if lt := s.locTypes.Configs[typeId]; lt != nil && lt.DebugName != "" {
				return lt.DebugName
			}
		}
		return strconv.Itoa(typeId)
	case *entitypkg.Obj:
		if s.objTypes != nil && tgt.Type >= 0 && tgt.Type < len(s.objTypes.Configs) {
			if ot := s.objTypes.Configs[tgt.Type]; ot != nil && ot.DebugName != "" {
				return ot.DebugName
			}
		}
		return strconv.Itoa(tgt.Type)
	}

	// T-trigger com-branch (TS L1086). The `com != -1` guard applies
	// only to APNPCT in TS; APPLAYERT/APLOCT/APOBJT enter the com
	// branch unconditionally and fall through to the numeric
	// targetSubject form when com is -1.
	if (p.targetSubject.com != -1 && p.targetOp == targetOpNpcT) ||
		p.targetOp == targetOpPlayerT || p.targetOp == targetOpLocT ||
		p.targetOp == targetOpObjT {
		com := p.targetSubject.com
		if s.componentTypes != nil && com >= 0 && com < len(s.componentTypes.Configs) {
			if ct := s.componentTypes.Configs[com]; ct != nil && ct.ComName != "" {
				return ct.ComName
			}
		}
		return strconv.Itoa(com)
	}

	// targetSubject.typ override branch (TS L1088 — TS field name `type`,
	// goscape field name `typ` per player.go:143).
	if p.targetSubject.typ != -1 {
		typ := p.targetSubject.typ
		if s.objTypes != nil && typ >= 0 && typ < len(s.objTypes.Configs) {
			if ot := s.objTypes.Configs[typ]; ot != nil && ot.DebugName != "" {
				return ot.DebugName
			}
		}
		return strconv.Itoa(typ)
	}

	return "_"
}

// tsTriggerForOpFire returns the TS-faithful OP* ServerTriggerType for the
// given target/targetOp pair, used only by defaultOp's debug chat
// (NodeDebug-gated).
//
// TS Player.ts:1093 emits ServerTriggerType[targetOp+7] where targetOp is
// the AP* trigger set by setInteraction; +7 maps AP* -> OP*. Goscape stores
// targetOp as an op-slot int (1..5) or one of the targetOp{Loc,Npc,Player,Obj}
// {T,U} sentinels (interaction.go:36-45). This helper bridges both namespaces.
//
// Sentinel matches dispatch by targetOp alone (TS L1086 — APNPCT/APPLAYERT/
// APLOCT/APOBJT all evaluate independent of target type). Numeric op-slots
// disambiguate via target type. Returns ServerTriggerType(-1) when target is
// nil or unrecognised, or targetOp is out-of-range — goscape defensive; TS
// would throw via `undefined.toLowerCase()` (DEVIATION-NAI-148-D-OPFIRE-FALLBACK).
func tsTriggerForOpFire(target entity, targetOp int) script.ServerTriggerType {
	switch targetOp {
	case targetOpLocT:
		return script.TriggerOpLocT
	case targetOpLocU:
		return script.TriggerOpLocU
	case targetOpNpcT:
		return script.TriggerOpNpcT
	case targetOpNpcU:
		return script.TriggerOpNpcU
	case targetOpPlayerT:
		return script.TriggerOpPlayerT
	case targetOpPlayerU:
		return script.TriggerOpPlayerU
	case targetOpObjT:
		return script.TriggerOpObjT
	case targetOpObjU:
		return script.TriggerOpObjU
	}
	if targetOp < 1 || targetOp > 5 {
		return script.ServerTriggerType(-1)
	}
	offset := script.ServerTriggerType(targetOp - 1)
	switch target.(type) {
	case *Npc:
		return script.TriggerOpNpc1 + offset
	case *entitypkg.Loc:
		return script.TriggerOpLoc1 + offset
	case *entitypkg.Obj:
		return script.TriggerOpObj1 + offset
	case *Player:
		return script.TriggerOpPlayer1 + offset
	}
	return script.ServerTriggerType(-1)
}

// inOperableDistance reports whether p is in contact range of target.
// Mirrors TS Player.inOperableDistance (Player.ts:1099-1111):
//   - Loc targets dispatch to pkg/pathfinder/reach.Reached for shape /
//     angle / forceapproach-aware reach (NAI-91).
//   - PathingEntity (Player, Npc) and Obj targets fall through to
//     inOperableDistanceCheb (Chebyshev≤1, excludes same tile) pending
//     entity-shape / reachedObj port (DEVIATION
//     NAI-91-D-OPERABLE-CHEB-FALLBACK).
//
// target.level mismatch returns false (TS guard preserved at all arms).
//
// INVARIANT: pkg/entity/Loc.Width / Loc.Length store ABSOLUTE (un-rotated)
// dimensions — verified at modules/world/script_loc_ops.go:35-43 and
// pkg/gamemap/load.go:128. reach.Reached rotates internally via
// rotation.Rotate(locAngle, destWidth, destLength); no double-rotation.
func inOperableDistance(p *Player, target entity) bool {
	tx, tz, tlevel := target.Coords()
	if tlevel != p.level {
		return false
	}
	if loc, ok := target.(*entitypkg.Loc); ok {
		srv := p.client.server
		// goscape defensive: gamemap is always initialised by Server.Init in
		// production but may be nil in narrow unit tests that don't load map
		// data. Fall back to Chebyshev when absent so pre-NAI-91 tests that
		// don't exercise the shape-aware path continue to compile and run.
		if srv.gamemap == nil {
			return inOperableDistanceCheb(p.x, p.z, tx, tz)
		}
		flags := srv.gamemap.Pathfinder.Flags
		var fap int
		if cfg := srv.locTypeOrNil(loc.Type()); cfg != nil {
			fap = cfg.ForceApproach
		}
		return reach.Reached(flags, p.level, p.x, p.z, tx, tz,
			loc.Width, loc.Length, 1, loc.Angle(), loc.Shape(), fap)
	}
	return inOperableDistanceCheb(p.x, p.z, tx, tz)
}

// inOperableDistanceCheb is the Chebyshev≤1 predicate (excludes same tile)
// retained for PathingEntity (Player, Npc) and Obj targets pending the
// TS reachedEntity / reachedObj ports. Lives under DEVIATION
// NAI-91-D-OPERABLE-CHEB-FALLBACK.
func inOperableDistanceCheb(px, pz, tx, tz int) bool {
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

// pathToPathingTarget mirrors TS Player.pathToPathingTarget
// (Engine-TS/src/engine/entity/Player.ts:1034-1055). Called once per tick
// from processInteraction's post-step branch when !interacted (TS L1228-1229).
//
// Dispatch:
//   - Loc/Obj target: no-op (TS L1035-1037). In TS, Loc/Obj targets get
//     their initial path from MoveClick/scripts; tickloop never repaths.
//     Pre-NAI-98 goscape ran pathToTarget once per interaction for these
//     targets too (legacy `!p.repathed` gate). DEVIATION
//     NAI-98-D-LOC-OBJ-NO-OP-ALIGNED-TO-TS: aligned to TS no-op as part of
//     this fix; smoke targets are *Npc, but the gate retirement is the
//     same code path so Loc/Obj alignment is a free byproduct. If a
//     downstream Loc/Obj smoke surfaces a residual, revisit.
//   - PathingEntity + isLastOrNoWaypoint + followOp (APPLAYER3/OPPLAYER3):
//     queueWaypoint to target's followX/followZ (TS L1039-1042).
//     Player-on-player chase fast-path. Goscape's *Player has followX/Z;
//     *Npc does not (DEVIATION NAI-98-D-NPC-NO-FOLLOWXY: ports of TS
//     PathingEntity.ts:1201-1202 base behavior limited to *Player today;
//     followOp branch fires only when target is *Player anyway).
//   - !canAccess: no-op (TS L1044-1046). Goscape canAccess approximation:
//     !p.delayed && !p.protectedScriptActive() (per CanAccess doc-comment
//     in player_script.go; DEVIATION NAI-44-D-CANACCESS-NO-STUN-CHECK).
//   - NODE_CLIENT_ROUTEFINDER + intersects: queueWaypoints via
//     FindNaivePath (TS L1048-1051). Mirrors the same shortcut at
//     pathToTarget Smart/PathingEntity arm (interaction.go:638-644).
//   - PathingEntity + isLastOrNoWaypoint (no followOp, no intersects):
//     pathToTarget (TS L1052-1054).
//
// isLastOrNoWaypoint mirrors TS PathingEntity.isLastOrNoWaypoint
// (PathingEntity.ts:374-376): true when the player has consumed all but
// the final waypoint or has none queued.
//
// Retires the goscape divergent `!p.repathed` once-per-interaction gate
// at interaction.go:236-239 (pre-NAI-98). The `repathed` field stays
// declared + reset in SetInteraction/ClearInteraction as TS-vestigial
// (TS PathingEntity.ts:64 declares it + Player.ts:459 resets it but
// nothing reads it).
func (p *Player) pathToPathingTarget() {
	if p.target == nil {
		return
	}
	if _, ok := p.target.(pathingEntity); !ok {
		// Loc/Obj target — TS no-op.
		return
	}
	if p.isLastOrNoWaypoint() && isFollowOp(p) {
		// Player-on-player chase: queue waypoint to target's last-step coord.
		// followOp implies target is *Player (per isFollowOp at :145-151);
		// goscape's *Player has followX/followZ (player.go:104).
		if t, ok := p.target.(*Player); ok {
			p.queueWaypoint(t.followX, t.followZ)
		}
		return
	}
	if p.delayed || p.protectedScriptActive() {
		// canAccess gate (TS L1044-1046). DEVIATION
		// NAI-44-D-CANACCESS-NO-STUN-CHECK: stun/freeze unmodelled.
		return
	}
	if p.client == nil || p.client.server == nil {
		return
	}
	srv := p.client.server
	tx, tz, _ := p.target.Coords()
	if t, ok := p.target.(pathingEntity); ok {
		tw, tl := t.Width(), t.Length()
		if srv.cfg.NodeClientRoutefinder && coordgrid.Intersects(p.x, p.z, p.Width(), p.Length(), tx, tz, tw, tl) {
			pf := srv.pathfinder()
			if pf == nil {
				p.queueWaypoint(tx, tz)
				return
			}
			route := pf.FindNaivePath(p.level, p.x, p.z, tx, tz, p.Width(), p.Length(), tw, tl, 0, collision.TypeNormal)
			p.queueWaypoints(routeToPacked(route))
			return
		}
	}
	if p.isLastOrNoWaypoint() {
		p.pathToTarget()
	}
}

// isLastOrNoWaypoint mirrors TS PathingEntity.isLastOrNoWaypoint
// (PathingEntity.ts:374-376): true when the player has consumed all but
// the final waypoint or has none queued.
func (p *Player) isLastOrNoWaypoint() bool {
	return p.waypointIndex <= 0
}

// pathToTarget queues waypoints from p.x/p.z to p.target via shape-aware
// findPath helpers. Mirrors TS PathingEntity.pathToTarget
// (PathingEntity.ts:457-508).
//
// Type-switches on p.target to select the appropriate FindPath* wrapper:
//   - *entitypkg.Loc:        FindPathToLoc with shape/angle/forceapproach.
//   - *Player / *Npc:        FindPathToEntity (shape=-2 entity sentinel),
//                            FindNaivePath shortcut on NODE_CLIENT_ROUTEFINDER+intersect.
//   - *entitypkg.Obj same:   queueWaypoint (TS workaround for findPath returning (0,0)).
//   - *entitypkg.Obj diff:   FindPathPlain (TS plain findPath).
//
// NAIVE strategy: PathingEntity → FindNaivePath, others → single waypoint.
// No-strategy else: nomove guards + single waypoint.
//
// History: NAI-11 deferred the SMART branch with a stub queueing a single
// waypoint at target.Coords(). NAI-92 closed the deferral.
//
// Counterpart: (*Npc).pathToTarget (modules/world/npc_interaction.go) overrides
// this base dispatch with an unconditional intersect shortcut per Npc.ts:319-335.
func (p *Player) pathToTarget() {
	if p.target == nil {
		return
	}

	switch p.moveStrategy {
	case MoveStrategySmart:
		p.pathToTargetSmart()
	case MoveStrategyNaive:
		p.pathToTargetNaive()
	default:
		p.pathToTargetNoStrategy()
	}
}

// pathToTargetSmart dispatches by target type for the SMART strategy.
// Cross-reference: modules/world/npc_interaction.go pathToTargetSmart.
// Logic is duplicated rather than factored because of asymmetric server-
// access (Player: client.server, Npc: server). Risk register R2 mitigation.
func (p *Player) pathToTargetSmart() {
	srv := p.client.server
	pf := srv.pathfinder()
	tx, tz, _ := p.target.Coords()

	switch t := p.target.(type) {
	case *entitypkg.Loc:
		if pf == nil {
			// (goscape defensive; TS skips this check) — gamemap not
			// initialised in some test fixtures; queue a direct-step waypoint.
			p.queueWaypoint(tx, tz)
			return
		}
		var fap int
		// (goscape defensive; TS skips this check) — TS LocType.get(t.type)
		// throws on missing; goscape returns nil and we treat as forceapproach=0.
		if cfg := srv.locTypeOrNil(t.Type()); cfg != nil {
			fap = cfg.ForceApproach
		}
		route := pf.FindPathToLoc(p.level, p.x, p.z, tx, tz, p.Width(), t.Width, t.Length, t.Angle(), t.Shape(), fap)
		p.queueWaypoints(routeToPacked(route))

	case pathingEntity:
		// *Player or *Npc target. Mirrors TS PathingEntity.pathToTarget
		// PathingEntity branch (PathingEntity.ts:464-468). When NODE_CLIENT_-
		// ROUTEFINDER is enabled and player+target bboxes intersect, shortcut
		// to FindNaivePath; else use the full FindPathToEntity search with
		// the entity-target shape sentinel (=-2 inside the wrapper).
		if pf == nil {
			// (goscape defensive; TS skips this check)
			p.queueWaypoint(tx, tz)
			return
		}
		tw, tl := t.Width(), t.Length()
		if srv.cfg.NodeClientRoutefinder && coordgrid.Intersects(p.x, p.z, p.Width(), p.Length(), tx, tz, tw, tl) {
			route := pf.FindNaivePath(p.level, p.x, p.z, tx, tz, p.Width(), p.Length(), tw, tl, 0, collision.TypeNormal)
			p.queueWaypoints(routeToPacked(route))
		} else {
			route := pf.FindPathToEntity(p.level, p.x, p.z, tx, tz, p.Width(), tw, tl)
			p.queueWaypoints(routeToPacked(route))
		}

	case *entitypkg.Obj:
		// TS PathingEntity.pathToTarget Obj arm (PathingEntity.ts:469-475).
		// Same-tile workaround: TS findPath returns (0,0) when src==dest, so
		// queue one waypoint at the target tile directly. Different-tile:
		// shape-blind FindPathPlain (TS plain findPath).
		if p.x == tx && p.z == tz {
			p.queueWaypoint(tx, tz)
		} else {
			if pf == nil {
				// (goscape defensive; TS skips this check)
				p.queueWaypoint(tx, tz)
				return
			}
			route := pf.FindPathPlain(p.level, p.x, p.z, tx, tz)
			p.queueWaypoints(routeToPacked(route))
		}

	default:
		// Unhandled target type (TS pathToTarget has no fallthrough default).
		// (goscape defensive; TS skips this check)
		if pf == nil {
			p.queueWaypoint(tx, tz)
			return
		}
		route := pf.FindPathPlain(p.level, p.x, p.z, tx, tz)
		p.queueWaypoints(routeToPacked(route))
	}
}

// pathToTargetNaive — NAIVE strategy. PathingEntity targets use
// FindNaivePath with the entity's blockWalkFlag/collisionStrategy;
// non-PathingEntity targets queue a single waypoint at the target tile.
// Mirrors TS PathingEntity.pathToTarget NAIVE arm (PathingEntity.ts:477-493).
// Cross-reference: modules/world/npc_interaction.go pathToTargetNaive.
func (p *Player) pathToTargetNaive() {
	cs := p.getCollisionStrategy()
	if cs == nil {
		// nomove moverestrict returns nil = no walking allowed.
		return
	}
	extraFlag := p.blockWalkFlag()
	if extraFlag == collision.FlagNull {
		// nomove moverestrict returns NULL = no walking allowed. Note
		// Player.blockWalkFlag is unconditional FlagBlockPlayers in TS, so
		// this branch is structurally present but never fires for Player.
		return
	}

	tx, tz, _ := p.target.Coords()
	if t, ok := p.target.(pathingEntity); ok {
		pf := p.client.server.pathfinder()
		if pf == nil {
			// (goscape defensive; TS skips this check)
			p.queueWaypoint(tx, tz)
			return
		}
		route := pf.FindNaivePath(p.level, p.x, p.z, tx, tz, p.Width(), p.Length(), t.Width(), t.Length(), extraFlag, *cs)
		p.queueWaypoints(routeToPacked(route))
	} else {
		p.queueWaypoint(tx, tz)
	}
}

// pathToTargetNoStrategy is TS PathingEntity.pathToTarget's third else
// branch (PathingEntity.ts:494-507): runs the same nomove + blockwalk
// guards as NAIVE but always queues a single waypoint regardless of
// target type. Engaged by MoveStrategy values outside Smart/Naive
// (defensive future-proofing — goscape's enum only has Smart+Naive).
// Cross-reference: modules/world/npc_interaction.go pathToTargetNoStrategy.
func (p *Player) pathToTargetNoStrategy() {
	if p.getCollisionStrategy() == nil {
		return
	}
	if p.blockWalkFlag() == collision.FlagNull {
		return
	}
	tx, tz, _ := p.target.Coords()
	p.queueWaypoint(tx, tz)
}
