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
//
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
	// targetSubject.typ snapshot for changetype detection in validateTarget
	// (TS PathingEntity.ts:521-526 — targetSubject.type = target.type for
	// Npc/Loc/Obj, else -1). interaction-2: the player path previously left
	// typ untouched, so Npc/Player targets retained a leftover typ from a
	// prior interaction. Mirrors (*Npc).SetInteraction (npc_interaction.go).
	// For Loc/Obj this also re-asserts what the OpLoc/OpObj handlers snapshot
	// afterwards (identical value); the Npc and non-typed cases are new.
	switch t := target.(type) {
	case *Npc:
		p.targetSubject.typ = t.typeId
	case *entitypkg.Loc:
		p.targetSubject.typ = t.Type()
	case *entitypkg.Obj:
		p.targetSubject.typ = t.Type
	default:
		p.targetSubject.typ = -1
	}
	p.interactionKind = kind
	// TS PathingEntity.ts:517-518: setInteraction resets ONLY apRange + apRangeCalled.
	// goscape previously also reset interacted/repathed here as belt-and-braces;
	// interaction-6 closed that gap by making ResetMasks reset interacted per-tick
	// (TS PathingEntity.ts:587), so the non-TS resets here are now redundant.
	// `repathed` has no production reader (NAI-98 retired its gate) and no
	// production writer, so it stays at its zero-value (false) for the lifetime
	// of every Player — the field is vestigial.
	p.apRange = 10
	p.apRangeCalled = false

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
		playerSlot := t.slot + 32768 // TS PathingEntity.ts:534 @2e3bcf43
		if p.faceEntity != playerSlot {
			p.faceEntity = playerSlot
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

// validateTarget enforces per-tick target validity for the player
// interaction loop. Mirrors TS Player.validateTarget
// (Engine-TS/.../Player.ts:1186-1198) — three gates, any failure of which
// makes the caller clearInteraction()+unsetMapFlag():
//  1. same level,
//  2. changetype for Npc/Loc targets (targetSubject.typ snapshot vs the
//     target's current type — catches a mid-interaction morph),
//  3. the polymorphic target.isValid(hash64): Npc → !dead && !delayed
//     (TS Npc.isValid), Obj → private-reveal + count keyed on this player's
//     UID (TS Obj.isValid(hash64); hash64 → composeUID per NAI-153-D2),
//     else the intrinsic Entity.isValid (Player loggingOut/visibility,
//     Loc/other isActive).
//
// Gate 2 reads p.targetSubject.typ, snapshotted by SetInteraction. This is
// the player counterpart of (*Npc).validateTarget (npc_interaction.go);
// the player path additionally honours the obj reveal hash64 because, unlike
// an NPC, a player is an observer with a UID. interaction-1.
func (p *Player) validateTarget() bool {
	if p.target == nil {
		return false
	}

	// Gate 1: same level (TS L1188).
	_, _, tlevel := p.target.Coords()
	if tlevel != p.level {
		return false
	}

	// Gate 2: changetype for Npc/Loc (TS L1193).
	switch t := p.target.(type) {
	case *Npc:
		if p.targetSubject.typ != t.typeId {
			return false
		}
	case *entitypkg.Loc:
		if p.targetSubject.typ != t.Type() {
			return false
		}
	}

	// Gate 3: target.isValid(hash64) (TS L1197).
	switch t := p.target.(type) {
	case *Npc:
		// TS Npc.isValid (Npc.ts:370-375): !delayed && isActive. goscape's
		// Npc.IsValid() now mirrors this exactly (npc.go:453), so the
		// spelled-out check below is redundant defense in depth — kept for
		// readability at the gate 4 call site shared with (*Npc).validateTarget.
		return !t.dead && !t.delayed
	case *entitypkg.Obj:
		// TS Obj.isValid(hash64) (Obj.ts:52-62): private-reveal + count.
		return t.IsValidFor(p.UID())
	default:
		// Player → !loggingOut && visibility==DEFAULT && isActive;
		// Loc/other → intrinsic isActive (TS Entity.isValid base).
		return p.target.IsValid()
	}
}

// ClearInteraction resets interaction state to idle.
//
// Mirrors TS PathingEntity.clearInteraction (PathingEntity.ts:550-555):
// target, targetOp, the targetSubject identity snapshot, apRange, and
// apRangeCalled are reset. goscape's targetSubject additionally carries the
// loc/obj x/z/level snapshot (written by handler_oploc/handler_opobj for
// locStillValid/objStillValid), so all five fields reset to -1 — interaction-4
// (TS resets only {type:-1, com:-1}; the x/z/level extension is goscape's).
//
// interacted/repathed are NOT reset here — the per-tick reset of `interacted`
// now lives in ResetMasks (interaction-6), matching TS PathingEntity.ts:587.
// `repathed` is vestigial (no production reader since NAI-98, no production
// writer ever).
func (p *Player) ClearInteraction() {
	p.target = nil
	p.targetOp = -1
	p.targetSubject.typ = -1
	p.targetSubject.x = -1
	p.targetSubject.z = -1
	p.targetSubject.level = -1
	p.targetSubject.com = -1
	p.apRange = 10 // S6l: reset to default (TS PathingEntity.ts:554)
	p.apRangeCalled = false
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

// interactTickState carries per-tick interaction state from the pre-move
// pass to the post-move pass (see the Player.interactTick field comment).
type interactTickState struct {
	// active is true when processInteractionPreMove ran past its guards
	// (had a target, client, not delayed) and did NOT already finalize via
	// the level-mismatch clear. Only then does the post-move pass run.
	active           bool
	interacted       bool
	followOp         bool
	initialTarget    entity
	initialTargetX   int
	initialTargetZ   int
	opTriggerPresent bool
	apTriggerPresent bool
}

// processInteraction runs the full interaction cycle in a single call with
// NO movement between the pre-step and post-step arms. The production tick
// loop does NOT call this directly — it calls processInteractionPreMove
// before processPathing and processInteractionPostMove after, so this·
// (combined) form is the back-to-back equivalent used by tests and any
// caller that drives interaction in isolation. Mirrors TS
// Player.processInteraction (Player.ts:1200-1264) minus the inline
// updateMovement (which the split passes interleave with processPathing).
func (p *Player) processInteraction() {
	p.processInteractionPreMove()
	p.processInteractionPostMove()
}

// processInteractionPreMove is the part of TS Player.processInteraction that
// runs BEFORE updateMovement (Player.ts:1200-1240): the unconditional top
// writes, entry guards, level-mismatch clear, the pre-step interact arm, and
// the path recompute / walktrigger / stun-clear of the post-step arm's head.
//
// In the production tick this runs as its own pass BEFORE processPathing, so
// the pre-step interact fires at the player's PRE-movement position — which
// is what lets a player who clicks an NPC already within range attack from
// where they stand instead of stepping to contact first. State needed by the
// post-move pass is stashed on p.interactTick.
func (p *Player) processInteractionPreMove() {
	// TS L1201-1203 — unconditional top writes. These fire every tick
	// for every player regardless of target/canAccess state. The
	// followX/Z refresh is required for player-follow: a follower's
	// pathToPathingTarget arm reads the leader's followX/Z, so the leader
	// must update them even when they have no target of their own (NAI-174).
	p.followX = p.lastStepX
	p.followZ = p.lastStepZ
	p.nextTarget = nil
	p.interactTick = interactTickState{}

	if p.target == nil {
		return
	}
	if p.client == nil || p.client.server == nil {
		return
	}
	s := p.client.server
	// interaction-7: the goscape-only `delayed && currentTick<delayedUntil`
	// short-circuit that used to live here pre-empted the post-step HEAD
	// (TS Player.ts:1227-1239) for a delayed player, skipping
	// pathToPathingTarget's followOp chase recompute (TS L1039-1042) and
	// the L1237 exhaustion clear. TS only gates the INTERACT arms on
	// canAccess (L1210, L1244); the post-step HEAD runs unconditionally
	// on !interacted. The pre-step arm's CanAccess() gate below (and the
	// post-move pass's matching gate) handle the actually-delayed case
	// TS-faithfully via the existing canAccess gates — no goscape-side
	// short-circuit is needed. Pinned by
	// TestProcessInteraction_CanAccessGate_Delayed_FollowOp_PathRecomputed.

	// NAI-79 Stage 1 — pre-step state capture for Frame B emit (post-move
	// pass). Trigger-lookup capture is gated on NodeDebug to avoid the
	// per-tick hot-path cost when instrumentation is disabled.
	p.interactTick.active = true
	p.interactTick.initialTarget = p.target
	p.interactTick.initialTargetX, p.interactTick.initialTargetZ, _ = p.target.Coords()
	if s.cfg.NodeDebug {
		p.interactTick.opTriggerPresent = getOpTrigger(p, s) != nil
		p.interactTick.apTriggerPresent = getApTrigger(p, s) != nil
	}
	p.lastInteractBranchPre = 0
	p.lastInteractBranchPost = 0
	p.interactCallSlot = 0

	followOp := isFollowOp(p)
	p.interactTick.followOp = followOp

	interacted := false

	// Pre-step interact arm (TS L1209-1224). Gated on target + CanAccess
	// so a modal/protected/delayed state preserves the interaction across
	// the tick (TS L1210 mirror; NAI-155).
	if p.target != nil && p.CanAccess() {
		// validateTarget (TS L1212-1218): level / changetype / isValid.
		// On any failure TS clears the interaction, unsets the map flag, and
		// returns from processInteraction. In goscape's pre/post split that
		// means finalizing the tick here — emit Frame B and deactivate the
		// interactTick so the post-move pass does not re-run or double-emit.
		// interaction-1: previously only the level gate was ported, as an
		// inline check ahead of (and outside) this CanAccess gate.
		if !p.validateTarget() {
			p.ClearInteraction()
			p.unsetMapFlag()
			emitInteractionTickFrame(s, p, true, p.interactTick.initialTarget,
				p.interactTick.initialTargetX, p.interactTick.initialTargetZ,
				p.interactTick.opTriggerPresent, p.interactTick.apTriggerPresent,
				false /*interactedFinal*/)
			p.interactTick.active = false
			return
		}
		if !followOp {
			p.processWalktrigger()
		}
		p.interactCallSlot = 0
		interacted = p.tryInteract(false)
	}

	// Post-step arm HEAD (TS L1227-1239), which runs BEFORE updateMovement.
	// Skipped when pre-step interacted.
	if !interacted {
		// Recalc path (TS L1228-1229).
		p.pathToPathingTarget()

		// Process walktrigger if waypoints exist AND CanAccess (TS L1232).
		if p.hasWaypoints() && p.CanAccess() {
			p.processWalktrigger()
		}

		// followOp + waypoint exhaustion → clear (TS L1237-1239).
		if !p.hasWaypoints() && followOp {
			p.ClearInteraction()
		}
	}

	p.interactTick.interacted = interacted
}

// processInteractionPostMove is the part of TS Player.processInteraction that
// runs AFTER updateMovement (Player.ts:1242-1268): the post-step interact arm
// (which now sees the player's POST-movement position), the nextTarget pop /
// auto-clear, the tail mapflag clear, and the Frame B emit. Runs as its own
// pass AFTER processPathing in the production tick.
func (p *Player) processInteractionPostMove() {
	st := &p.interactTick
	if !st.active {
		return
	}
	if p.client == nil || p.client.server == nil {
		return
	}
	s := p.client.server
	interacted := st.interacted
	followOp := st.followOp

	// Post-step interact (TS L1244-1252). Gated on CanAccess: when a
	// modal/protected/delayed state transiently blocks interaction,
	// PRESERVE the anchor — do NOT fire "I can't reach!" + Clear.
	if !interacted {
		if p.target != nil && p.CanAccess() && !followOp {
			p.interactCallSlot = 1
			interacted = p.tryInteract(p.stepsTaken == 0)
			if !interacted && !p.hasWaypoints() && p.stepsTaken == 0 {
				p.MessageGame("I can't reach that!")
				p.ClearInteraction()
			}
		}
	}

	// nextTarget pop + auto-clear (TS L1255-1263). When an OP/AP trigger
	// script called p_op_* mid-trigger, the fire helpers captured the
	// script-set target into p.nextTarget; pop it here. Otherwise,
	// auto-clear the interaction. NAI-68/NAI-69.
	if p.nextTarget != nil {
		p.target = p.nextTarget
	} else if interacted && !p.apRangeCalled {
		p.ClearInteraction()
	}

	// Tail mapflag clear (TS L1266-1268). When the player has consumed at
	// least one step this tick and no waypoints remain, clear the client's
	// pending map-click indicator.
	if !p.hasWaypoints() && p.stepsTaken > 0 {
		sendUnsetMapFlag(p)
	}

	// NAI-79 Stage 1 — Frame B emit at tail (NodeDebug-gated internally).
	emitInteractionTickFrame(s, p, true, st.initialTarget,
		st.initialTargetX, st.initialTargetZ, st.opTriggerPresent,
		st.apTriggerPresent, interacted)
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
	s.runScript(sf, p, nil, script.TriggerWalkTrigger, true, nil, nil)
}

// tryInteract is the contact/approach-distance dispatch unifying the
// OP and AP arms that processInteraction previously inlined. Mirrors
// LostCityRS/Engine-TS Player.tryInteract at Player.ts:1113-1184.
//
// 4-branch dispatch (after NAI-78). Resolves opTrigger/apTrigger via
// getOpTrigger/getApTrigger (interaction_trigger.go) at entry, then
// dispatches:
//
//  1. opTrigger != nil && (PathingEntity || allowOpScenery) && operable
//     → fire OP, return true.
//  2. apTrigger != nil && approach
//     → fire AP, return true (or false on NAI-69 same-tick retry).
//  3. approach (apTrigger nil)
//     → apRange=-1, return false. Allows processInteraction's post-step
//     to run pathToTarget + tryInteract(allowOpScenery=true).
//  4. (PathingEntity || allowOpScenery) && operable (opTrigger nil)
//     → defaultOp NIH ("Nothing interesting happens." + clear waypoints),
//     return true.
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
	// NAI-155 closes NAI-78-D-HASINTERACTION-GUARD's CanAccess half:
	// CanAccess gate lifted to processInteraction call sites (TS L1210/
	// L1244 parity). Inner guard now matches TS Player.ts:1114 minus
	// canAccess: target presence + HasInteraction (follow-op filter).
	// Production callers (interaction.go:249, 269) gate CanAccess at the
	// call site; no other production caller of (*Player).tryInteract
	// exists per NAI-155 controller-preflight enumeration.
	if p.target == nil || !p.HasInteraction() {
		recordTryInteractBranch(p, 0) // NAI-79 Stage 1 (combined early-return)
		return false
	}
	srv := p.client.server

	opTrigger := getOpTrigger(p, srv)
	apTrigger := getApTrigger(p, srv)

	tx, tz, _ := p.target.Coords()
	tw, tl := approachTargetSize(p.target)
	operable := inOperableDistance(p, p.target)

	isPathing := false
	switch p.target.(type) {
	case *Npc, *Player:
		isPathing = true
	}

	// TS PathingEntity.inApproachDistance (PathingEntity.ts:405, player/else
	// branch) is `distanceTo(...) <= range && isApproached(...)`. The free
	// inApproachDistance below is the range half; approachHasLineOfSight is the
	// LoS half (M1 — the player side previously omitted it and could fire AP
	// through walls; the NPC branch already gated). Short-circuits: when the
	// range half is false (incl. apRange<=0) LoS is never evaluated, so no
	// gamemap call happens when no AP script exists. isPathing gates the
	// footprint-overlap bail per TS PathingEntity.ts:395 (npc-ai-5 /
	// pathing-5 / interaction-5).
	approach := inApproachDistance(p.x, p.z, tx, tz, tw, tl, effectiveApRange(p), isPathing) &&
		p.approachHasLineOfSight(tx, tz, tw, tl)

	// Branch 1 — OP fire (TS Player.ts:1123). Fire is unconditional per
	// TS — the Go-only interactionFired gate that previously wrapped this
	// call was removed in NAI-218 (TS-parity restore); intra-tick re-fire
	// is already prevented by processInteraction's `if !interacted`
	// post-step gate.
	if opTrigger != nil && (isPathing || allowOpScenery) && operable {
		p.interacted = true
		tryFireOpTrigger(p)
		recordTryInteractBranch(p, 1) // NAI-79 Stage 1
		return true
	}

	// Branch 2 — AP fire (TS Player.ts:1139). Fire is unconditional per
	// TS (interactionFired removed in NAI-218).
	if apTrigger != nil && approach {
		p.interacted = true
		tryFireApTrigger(p)
		// NAI-69 same-tick retry (TS Player.ts:1158-1167). When the AP
		// script set apRangeCalled and did not call p_op_* (no nextTarget),
		// return false so processInteraction's post-step path-then-retry
		// arm re-enters and can fire AP again at the new range.
		if p.nextTarget == nil && p.apRangeCalled {
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
// NAI-147 T4 closed NAI-78-D-DEBUG-MSG-DEFERRED: under cfg.NodeDebug
// (TS !NODE_PRODUCTION analogue) and both triggers nil, emit the TS
// L1076-1093 debug chat.
//
// NAI-148 closed NAI-147-D-TRIGGER-NAME-NUMERIC: trigger name now
// resolved via tsTriggerForOpFire(p.target, p.targetOp).String() —
// emits TS-faithful lowered name (e.g. "opnpc1") instead of the
// numeric `targetOp+7` form. tsTriggerForOpFire bridges goscape's
// op-slot/sentinel namespace to TS's AP*/OP* ServerTriggerType.
func defaultOp(p *Player, opTrigger, apTrigger *script.ScriptFile) {
	if p.client != nil && p.client.server != nil {
		s := p.client.server
		if s.cfg.NodeDebug && opTrigger == nil && apTrigger == nil {
			debugname := defaultOpDebugname(p, s)
			trigger := tsTriggerForOpFire(p.target, p.targetOp)
			p.MessageGame(fmt.Sprintf("No trigger for [%s,%s]", trigger, debugname))
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
//   - Obj targets dispatch to reach.Reached twice — locShape=-2
//     (reachedEntity) OR locShape=-1 (reachedObj). Same-tile pickup
//     succeeds via the locShape=-1 short-circuit at strategy.go:37
//     (NAI-152 B2). 1×1 Obj invariant: NewObj sets Width=Length=1
//     unconditionally (pkg/entity/obj.go:39).
//   - PathingEntity (Player, Npc) targets dispatch to reach.Reached with
//     locShape=-2 (reachedEntity) (NAI-173). reachRectangle1 has no
//     diagonal arm, so diagonally-adjacent entity targets reject —
//     TS-faithful semantic divergence from the pre-NAI-173 Chebyshev
//     fallback.
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
	if obj, ok := target.(*entitypkg.Obj); ok {
		// TS Player.ts:1110 — reachedEntity || reachedObj. Same-tile
		// pickup relies on the locShape=-1 short-circuit; reachedEntity
		// (locShape=-2) returns false on 1×1 same-tile because
		// ReachExclusiveRectangle's Collides() detects the src/dest
		// overlap and rejects (TS rsmod has identical semantics).
		srv := p.client.server
		if srv.gamemap == nil {
			return inOperableDistanceCheb(p.x, p.z, tx, tz)
		}
		flags := srv.gamemap.Pathfinder.Flags
		if reach.Reached(flags, p.level, p.x, p.z, tx, tz,
			obj.Width, obj.Length, p.Width(), 0, -2, 0) {
			return true
		}
		return reach.Reached(flags, p.level, p.x, p.z, tx, tz,
			obj.Width, obj.Length, p.Width(), 0, -1, 0)
	}
	if t, ok := target.(pathingEntity); ok {
		srv := p.client.server
		// goscape defensive: production sets gamemap in Server.Init; test
		// fixtures may construct a *Server without one. Fall through to
		// Chebyshev so those tests keep compiling.
		if srv.gamemap == nil {
			return inOperableDistanceCheb(p.x, p.z, tx, tz)
		}
		flags := srv.gamemap.Pathfinder.Flags
		// TS Player.ts:1104 — reachedEntity (locShape=-2, blockAccessFlags=0).
		// reach.Reached selects rectangleExclusiveStrategy → same-tile rejects
		// via Collides; orthogonal-adjacent passes when the src tile's
		// matching wall-flag is clear; diagonals reject (no rect1 diag arm).
		return reach.Reached(flags, p.level, p.x, p.z, tx, tz,
			t.Width(), t.Length(), p.Width(), 0, -2, 0)
	}
	// Defensive: target is neither *Loc nor *Obj nor pathingEntity (test
	// doubles only — production target is always one of those types).
	return inOperableDistanceCheb(p.x, p.z, tx, tz)
}

// inOperableDistanceCheb is the goscape-defensive Chebyshev≤1 fallback
// (excludes same-tile) used only by the nil-gamemap test-fixture paths in
// inOperableDistance and (*Npc).inOperableDistance. Production never
// reaches this since NAI-91 (Loc), NAI-152 B2 (Obj), and NAI-173
// (PathingEntity) cover all production target types via reach.Reached.
//
// (goscape defensive; TS skips this check.)
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

// inApproachDistance returns true when a 1x1 source at (px,pz) is within
// apRange tiles of the target footprint at (tx,tz) sized tw×tl. This is the
// RANGE half of TS PathingEntity.inApproachDistance; the LoS half is applied
// separately by (*Player).approachHasLineOfSight at the tryInteract call site
// (M1 closed the former DEVIATION S6l-D4 LoS omission).
// apRange <= 0 always returns false — the caller is responsible for
// distinguishing "not yet in range" from "no AP script exists."
//
// isPathingTarget gates the footprint-overlap bail per TS PathingEntity.ts:395:
// the "you are not within ap distance ... if you are underneath it" exclusion
// applies ONLY when the target is a PathingEntity (Player/Npc). For Loc/Obj
// targets TS skips the bail; a source standing on a Loc footprint or Obj tile
// is still in approach distance to fire its AP script (npc-ai-5 / pathing-5 /
// interaction-5).
//
// Distance is EDGE-aware (TS uses CoordGrid.distanceTo, not origin-corner
// Chebyshev). For a multi-tile target, origin distance > edge distance, so
// the old origin form made ranged/magic attackers approach (size-1) tiles
// too close before the AP could fire.
func inApproachDistance(px, pz, tx, tz, tw, tl, apRange int, isPathingTarget bool) bool {
	if apRange <= 0 {
		return false
	}
	// TS PathingEntity.ts:395-398 — footprint-overlap bail gated on
	// PathingEntity target (see doc-comment above).
	if isPathingTarget && coordgrid.Intersects(px, pz, 1, 1, tx, tz, tw, tl) {
		return false
	}
	// TS PathingEntity.ts:404 — CoordGrid.distanceTo (edge-aware Chebyshev),
	// NOT origin-corner distance.
	return coordgrid.DistanceTo(px, pz, 1, 1, tx, tz, tw, tl) <= apRange
}

// approachHasLineOfSight is the player-side LoS half of TS
// PathingEntity.inApproachDistance (PathingEntity.ts:405, the else/non-Npc
// branch): isApproached(level, this.x, this.z, target.x, target.z, this.width,
// this.length, target.width, target.length), which forwards to
// rsmod.hasLineOfSight(..., CollisionFlag.PLAYER) (GameMap.ts:433-435).
//
// Unlike the NPC branch — where "LoS is always calculated backwards" so the
// source is the target and the dest is self ((*Npc).inApproachDistance) — the
// player branch casts FORWARD: source = self, dest = target. Go's
// HasLineOfSight collapses the source footprint to a scalar srcSize, lossless
// here because the player is always square 1×1 (p.Width()).
//
// A nil server/gamemap short-circuits to gate-pass, mirroring
// (*Npc).inApproachDistance's NAI-12 error handling so headless tests without a
// map are not gated.
func (p *Player) approachHasLineOfSight(tx, tz, tw, tl int) bool {
	if p.client == nil || p.client.server == nil || p.client.server.gamemap == nil {
		return true
	}
	return p.client.server.gamemap.Pathfinder.LineValidator.HasLineOfSight(
		p.level, p.x, p.z, tx, tz, p.Width(), tw, tl, collision.FlagBlockPlayers)
}

// approachTargetSize returns the target's tile footprint for approach-distance
// math. Distinct from targetWidthLength (face-coord helper, returns 1×1 for
// NPCs) and approachEntitySize (LOS helper, doesn't handle Locs). A footprint
// is always at least 1×1 — a 0 dimension (e.g. an un-sized test-fixture NPC)
// is clamped so the edge-distance / intersect math stays well-formed.
func approachTargetSize(target entity) (width, length int) {
	switch t := target.(type) {
	case *Npc:
		width, length = int(t.size), int(t.size)
	case *entitypkg.Loc:
		width, length = t.Width, t.Length
	default: // *Player, *Obj — 1x1
		return 1, 1
	}
	if width < 1 {
		width = 1
	}
	if length < 1 {
		length = 1
	}
	return width, length
}

// effectiveApRange returns the approach-range in tiles the player's
// current target should be checked against by inApproachDistance.
// Always returns p.apRange (the mutable Player field, defaulted to 10
// in SetInteraction and settable via p_aprange per S6l), matching TS
// Player.tryInteract (Player.ts:1139) which reads this.apRange
// regardless of target type.
//
// Previously branched on *Npc targets to return npc.typ.AttackRange
// (the NPC's fixed combat reach); this broke ranged attacks against
// melee NPCs because the bow's apheld trigger's p_aprange(N) was
// silently overridden by the NPC's attackrange=1. Most visible
// through a fence (BlockRange=false scenery): walk-blocked adjacency
// + AttackRange=1 AP gate = projectile never fires. NAI-69 closure.
func effectiveApRange(p *Player) int {
	return p.apRange
}

// pathToPathingTarget mirrors TS Player.pathToPathingTarget
// (Engine-TS/src/engine/entity/Player.ts:1034-1055). Called once per tick
// from processInteraction's post-step branch when !interacted (TS L1228-1229).
//
// Dispatch:
//   - Loc/Obj target: no-op (TS L1035-1037). In TS, Loc/Obj targets get
//     their initial path from MoveClick/scripts; tickloop never repaths.
//     NAI-98 retired the legacy goscape `!p.repathed` once-per-interaction
//     gate that pre-emptively ran pathToTarget for these targets;
//     post-NAI-98 the arm matches TS exactly. If a downstream Loc/Obj
//     smoke surfaces a residual, revisit.
//   - PathingEntity + isLastOrNoWaypoint + followOp (APPLAYER3/OPPLAYER3):
//     queueWaypoint to target's followX/followZ (TS L1039-1042).
//     Player-on-player chase fast-path. TS declares followX/Z on the
//     PathingEntity base class (PathingEntity.ts:51-52); goscape
//     declares them on *Player only — sufficient because isFollowOp's
//     *Player type assertion (goscape defensive; TS skips this check,
//     relying on APPLAYER3/OPPLAYER3 targetOp identity at Player.ts:1205)
//     means *Npc targets cannot reach this arm in either engine.
//   - !canAccess: no-op (TS L1044-1046). Gated on full p.CanAccess() —
//     delayed + modal-Main/Chat + protectedScriptActive (player_script.go:390).
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
	if !p.CanAccess() {
		// canAccess gate (TS L1044-1046). Mirrors TS canAccess
		// (Player.ts:805-812: !protect && !busy(); busy includes
		// !containsModalInterface). NAI-170.
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
//     FindNaivePath shortcut on NODE_CLIENT_ROUTEFINDER+intersect.
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
