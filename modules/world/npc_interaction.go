package world

import (
	"math/rand/v2"

	"github.com/zsrv/goscape/pkg/coordgrid"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
)

// checkOpTrigger reports whether targetOp falls in any OP-trigger band
// (OPPLAYER / OPLOC / OPOBJ / OPNPC — 5 values each, four bands).
// Matches TS Npc.checkOpTrigger at Engine-TS/src/engine/entity/Npc.ts:1073-1080.
func checkOpTrigger(op int) bool {
	return (op >= objtype.NPCModeOpPlayer1 && op <= objtype.NPCModeOpPlayer5) ||
		(op >= objtype.NPCModeOpLoc1 && op <= objtype.NPCModeOpLoc5) ||
		(op >= objtype.NPCModeOpObj1 && op <= objtype.NPCModeOpObj5) ||
		(op >= objtype.NPCModeOpNpc1 && op <= objtype.NPCModeOpNpc5)
}

// checkApTrigger reports whether targetOp falls in any AP-trigger band
// (APPLAYER / APLOC / APOBJ / APNPC — 5 values each, four bands).
// Matches TS Npc.checkApTrigger at Engine-TS/src/engine/entity/Npc.ts:1064-1071.
func checkApTrigger(op int) bool {
	return (op >= objtype.NPCModeApPlayer1 && op <= objtype.NPCModeApPlayer5) ||
		(op >= objtype.NPCModeApLoc1 && op <= objtype.NPCModeApLoc5) ||
		(op >= objtype.NPCModeApObj1 && op <= objtype.NPCModeApObj5) ||
		(op >= objtype.NPCModeApNpc1 && op <= objtype.NPCModeApNpc5)
}

// resetDefaults clears target/targetOp to defaultMode baseline and re-emits
// the faceEntity mask bit. Matches TS Npc.resetDefaults at
// Engine-TS/.../Npc.ts:411-425 (the `this.masks |= this.entitymask` at
// :416). INTENTIONALLY does NOT clear apRange, apRangeCalled, faceEntity,
// or the rest of masks — those are overwritten only by the next
// SetInteraction call.
func (n *Npc) resetDefaults() {
	n.target = nil
	n.targetOp = n.defaultMode()
	n.masks |= n.entitymask
}

// clearInteraction resets interaction state to idle, including apRange
// fields. Matches TS PathingEntity.clearInteraction. Does NOT touch
// faceEntity/masks — those are cleared by the masks frame-pass, not here.
func (n *Npc) clearInteraction() {
	n.target = nil
	n.targetOp = -1
	n.apRange = 10
	n.apRangeCalled = false
	n.targetSubject = npcTargetSubject{com: -1, typ: -1}
}

// noMode is the NPCMode.NONE branch — just walks the existing path if
// any. Matches TS noMode at Engine-TS/.../Npc.ts:693-695.
func (n *Npc) noMode(s *Server) {
	n.updateMovement(s)
}

// wanderMode is the NPCMode.WANDER branch — a 1/8-tick random walk
// within WanderRange of spawn plus movement + a 500-tick
// teleport-to-spawn counter. Matches TS wanderMode at Npc.ts:697-715.
//
// The queueWaypoint skip-if-equal-to-current guard mirrors the TS
// "if we rolled our own tile, don't queue a null path" check.
func (n *Npc) wanderMode(s *Server) {
	if n.typ == nil {
		return
	}
	if n.moveRestrict != MoveRestrictNoMove && n.typ.WanderRange > 0 && rand.IntN(8) == 0 {
		rng := int(n.typ.WanderRange)
		dx := rand.IntN(rng*2+1) - rng
		dz := rand.IntN(rng*2+1) - rng
		if n.startX+dx != n.x || n.startZ+dz != n.z {
			n.queueWaypoint(n.startX+dx, n.startZ+dz)
		}
	}
	n.updateMovement(s)
	onSpawn := n.x == n.startX && n.z == n.startZ && n.level == n.startLevel
	n.wanderCounter++
	if n.wanderCounter >= 500 {
		if !onSpawn {
			n.x, n.z, n.level = n.startX, n.startZ, n.startLevel
			n.tele = true
		}
		n.wanderCounter = 0
	}
}

// patrolMode is the NPCMode.PATROL branch — advance through PatrolCoord
// with per-waypoint PatrolDelay, 30-tick stuck-teleport horizon, and a
// delayedPatrol latch so the at-waypoint delay doesn't double-trigger.
// Matches TS patrolMode at Engine-TS/.../Npc.ts:717-744.
func (n *Npc) patrolMode(s *Server) {
	if n.typ == nil || len(n.typ.PatrolCoord) == 0 {
		return
	}
	patrolDelay := 0
	if n.nextPatrolPoint < len(n.typ.PatrolDelay) {
		patrolDelay = int(n.typ.PatrolDelay[n.nextPatrolPoint])
	}
	dest := coordgrid.UnpackCoord(int(n.typ.PatrolCoord[n.nextPatrolPoint]))

	n.updateMovement(s)

	if n.waypointIndex < 0 && n.target == nil {
		n.queueWaypoint(dest.X, dest.Z)
	}
	if (n.x != dest.X || n.z != dest.Z) && n.nextPatrolTick > -1 && s.currentTick >= n.nextPatrolTick {
		n.x, n.z, n.level = dest.X, dest.Z, 0
		n.tele = true
	}
	if n.x == dest.X && n.z == dest.Z && !n.delayedPatrol {
		n.nextPatrolTick = s.currentTick + patrolDelay
		n.delayedPatrol = true
	}
	if n.nextPatrolTick > s.currentTick {
		return
	}

	n.nextPatrolPoint = (n.nextPatrolPoint + 1) % len(n.typ.PatrolCoord)
	n.nextPatrolTick = s.currentTick + 30
	n.delayedPatrol = false
	dest = coordgrid.UnpackCoord(int(n.typ.PatrolCoord[n.nextPatrolPoint]))
	n.queueWaypoint(dest.X, dest.Z)
}

// processMovementInteraction is the NPC's per-tick movement + interaction
// dispatcher. Replaces the inline wander/patrol/advanceWaypoint block
// that NAI-2..NAI-10 kept in npc_ai.go (the block is collapsed to a
// single call by Task 30). Mirrors TS Npc.processMovementInteraction
// at Engine-TS/.../Npc.ts:562-603.
//
// Dispatch order matches TS:
//  1. delayed / dead bail.
//  2. Last-tick coord bookkeeping + tele flag reset.
//  3. Null-targetOp failsafe → defaultMode.
//  4. Targetless modes (None / Wander / Patrol).
//  5. Targeted-mode prelude (target-nil or validateTarget-fail → resetDefaults).
//  6. Targeted-mode dispatch: PLAYER* modes reset to default (deferred);
//     everything else routes to aiMode.
//
// DEVIATION: PLAYERESCAPE / PLAYERFOLLOW / PLAYERFACE / PLAYERFACECLOSE
// modes are out of scope for NAI-11 (Q1 scope decision) and fall through
// to resetDefaults. Tracked follow-up.
func (n *Npc) processMovementInteraction(s *Server) {
	if n.delayed || n.dead {
		return
	}

	n.lastTickX, n.lastTickZ, n.lastLevel = n.x, n.z, n.level
	n.tele = false

	if n.targetOp == objtype.NPCModeNull {
		n.targetOp = n.defaultMode()
	}

	// Targetless modes.
	switch n.targetOp {
	case objtype.NPCModeNone:
		n.noMode(s)
		return
	case objtype.NPCModeWander:
		n.wanderMode(s)
		return
	case objtype.NPCModePatrol:
		n.patrolMode(s)
		return
	}

	// Targeted-mode prelude.
	if n.target == nil || !n.validateTarget() {
		n.resetDefaults()
		return
	}

	// Targeted-mode dispatch.
	switch n.targetOp {
	case objtype.NPCModePlayerEscape,
		objtype.NPCModePlayerFollow,
		objtype.NPCModePlayerFace,
		objtype.NPCModePlayerFaceClose:
		n.resetDefaults()
	default:
		n.aiMode(s)
	}
}

// aiMode is the interactive-target branch of processMovementInteraction.
// Mirrors TS aiMode at Engine-TS/.../Npc.ts:832-858.
//
// Try-twice pattern: tryInteract runs before AND after updateMovement
// so that stepping into range fires the trigger same-tick. The two
// calls differ in allowOpScenery: true pre-move (click-to-interact on
// scenery), false post-move (prevents scenery OP from firing on a
// walk-in that wasn't click-initiated).
//
// The givechase clause: if the NPC took a step but its NpcType forbids
// chasing, clear the interaction so it doesn't walk any further.
func (n *Npc) aiMode(s *Server) {
	if n.typ == nil {
		return
	}
	n.wanderCounter = 0

	// Pre-move attempt.
	if n.tryInteract(s, true) {
		return
	}

	// Not in range — path toward target and step.
	n.pathToTarget()
	moved := n.updateMovement(s)

	if moved && !n.typ.GiveChase {
		n.resetDefaults()
		return
	}

	// Post-move attempt (OP-scenery gated off).
	if n.target != nil {
		n.tryInteract(s, false)
	}
}

// tryInteract evaluates whether an AP or OP trigger should fire this tick.
// OP is checked first (contact range); AP second (approach range).
// allowOpScenery gates whether Loc/Obj OP fires — true on the pre-move
// call, false on the post-move call so scenery OP can't fire twice.
// Mirrors TS Npc.tryInteract at Engine-TS/.../Npc.ts:861-883.
//
// Returns true if a trigger fired (or would have fired, in the Loc/Obj
// + !allowOpScenery short-circuit case — caller treats both as "did
// something this tick").
func (n *Npc) tryInteract(s *Server, allowOpScenery bool) bool {
	if n.target == nil || n.typ == nil {
		return false
	}

	// OP branch — contact range.
	if checkOpTrigger(n.targetOp) && n.inOperableDistance(n.target) {
		_, isPlayer := n.target.(*Player)
		_, isNpc := n.target.(*Npc)
		isPathing := isPlayer || isNpc
		if isPathing || allowOpScenery {
			n.fireAiOpTrigger(s)
			return true
		}
		return false
	}

	// AP branch — approach range.
	if checkApTrigger(n.targetOp) && n.inApproachDistance(int(n.typ.AttackRange), n.target) {
		n.fireAiApTrigger(s)
		return true
	}

	return false
}

// updateMovement consumes up to 1 waypoint step (walk) or 2 (run) per
// tick. Returns true if the NPC moved. Writes walkDir (step 1) and
// runDir (step 2 when running). Replaces npc_ai.go advanceWaypoint
// (migrated into stepOnce below).
func (n *Npc) updateMovement(s *Server) bool {
	if n.moveRestrict == MoveRestrictNoMove {
		n.walkDir = -1
		n.runDir = -1
		return false
	}
	if n.waypointIndex < 0 {
		n.walkDir = -1
		n.runDir = -1
		return false
	}

	advanced1, dir1 := n.stepOnce(s)
	if !advanced1 {
		n.walkDir = -1
		n.runDir = -1
		return false
	}
	n.walkDir = dir1

	if n.moveSpeed == MoveSpeedRun && n.waypointIndex >= 0 {
		advanced2, dir2 := n.stepOnce(s)
		if advanced2 {
			n.runDir = dir2
		} else {
			n.runDir = -1
		}
	} else {
		n.runDir = -1
	}
	return true
}

// stepOnce walks one tile toward the current waypoint and returns
// (advanced, dir). Factors the shared step logic from the old
// advanceWaypoint at npc_ai.go:145-175. Decrements waypointIndex when
// the destination is reached; sets it to -1 when a CanTravel gate
// blocks the step.
func (n *Npc) stepOnce(s *Server) (bool, int) {
	if n.waypointIndex < 0 {
		return false, -1
	}
	dest := coordgrid.UnpackCoord(n.waypoints[n.waypointIndex])
	dir := coordgrid.Face(n.x, n.z, dest.X, dest.Z)
	if dir == -1 {
		n.waypointIndex--
		return false, -1
	}
	dx := coordgrid.DeltaX(dir)
	dz := coordgrid.DeltaZ(dir)
	if s != nil && s.gamemap != nil && !s.gamemap.CanTravel(n.level, n.x, n.z, dx, dz) {
		n.waypointIndex = -1
		return false, -1
	}
	n.x += dx
	n.z += dz
	n.stepsTaken++
	if n.x == dest.X && n.z == dest.Z {
		n.waypointIndex--
	}
	return true, int(dir)
}

// pathToTarget queues a single waypoint at the target's current tile.
// Naive-only port — TS's pathToTarget at PathingEntity.ts:457-508 has
// a full SMART branch using findPath / findPathToEntity / findPathToLoc.
//
// DEVIATION: SMART branch deferred. NAI-11 uses naive pathing for
// every mode; a full route-finder port is a separate sub-spec.
func (n *Npc) pathToTarget() {
	if n.target == nil {
		return
	}
	tx, tz, _ := n.target.Coords()
	n.queueWaypoint(tx, tz)
}

// validateTarget enforces per-tick target validity. Four gates matching
// TS Npc.validateTarget at Engine-TS/.../Npc.ts:606-627:
//  1. Same-level.
//  2. targetWithinMaxRange (per-mode maxrange math).
//  3. targetSubject.typ equality (catches changetype mid-interaction)
//     for Npc/Loc targets.
//  4. Concrete lifecycle: *Npc → isActive (!dead && !delayed);
//     *Loc/*Obj → intrinsic + zone-membership; *Player → IsValid.
//
// The *Npc branch's `!dead && !delayed` check is the TS "isActive"
// predicate — stricter than Go's Npc.IsValid() which is only `!dead`.
// See the Npc.IsValid DEVIATION note for the layering rationale.
func (n *Npc) validateTarget() bool {
	if n.target == nil {
		return false
	}

	// Gate 1: level.
	_, _, tlevel := n.target.Coords()
	if tlevel != n.level {
		return false
	}

	// Gate 2: maxrange.
	if !n.targetWithinMaxRange() {
		return false
	}

	// Gate 3: type-changed check for Npc/Loc (TS :618).
	switch t := n.target.(type) {
	case *Npc:
		if n.targetSubject.typ != t.typeId {
			return false
		}
	case *entitypkg.Loc:
		if n.targetSubject.typ != t.Type() {
			return false
		}
	}

	// Gate 4: concrete lifecycle.
	switch t := n.target.(type) {
	case *Npc:
		return !t.dead && !t.delayed
	case *entitypkg.Loc:
		tx, tz, lvl := t.Coords()
		return t.IsValid() && locStillValid(n.server, t, n.targetSubject.typ, tx, tz, lvl)
	case *entitypkg.Obj:
		tx, tz, lvl := t.Coords()
		return t.IsValid() && objStillValid(n.server, t, tx, tz, lvl)
	case *Player:
		return t.IsValid()
	default:
		return n.target.IsValid()
	}
}

// targetWithinMaxRange enforces the per-mode maxrange rules on n.target.
// Three branches: OP (maxrange+1 with corner-removal quirk), AP
// (maxrange + attackrange SW-distance), default (maxrange+1 SW-distance).
// Matches TS Npc.targetWithinMaxRange at Engine-TS/.../Npc.ts:629-680.
//
// DEVIATION: PLAYERESCAPE branch (TS :657-673) dropped with the other
// PLAYER* modes; scope defers the player-escape maxrange semantics to
// a future sub-spec.
func (n *Npc) targetWithinMaxRange() bool {
	if n.target == nil {
		return true
	}
	if n.typ == nil {
		return false
	}
	maxrng := int(n.typ.MaxRange)
	attackrng := int(n.typ.AttackRange)

	tx, tz, _ := n.target.Coords()
	dx := tx - n.startX
	if dx < 0 {
		dx = -dx
	}
	dz := tz - n.startZ
	if dz < 0 {
		dz = -dz
	}

	switch {
	case checkOpTrigger(n.targetOp):
		// TS :640-648 — maxrange+1 with corner-removal quirk.
		maxAxis := max(dx, dz)
		if maxAxis > maxrng+1 {
			return false
		}
		if dx == maxrng+1 && dz == maxrng+1 {
			return false
		}
		return true

	case checkApTrigger(n.targetOp):
		// TS :651-654 — SW-distance up to maxrange + attackrange.
		d := coordgrid.DistanceToSW(tx, tz, n.startX, n.startZ)
		return d <= maxrng+attackrng

	default:
		// TS :676 — SW-distance up to maxrange + 1.
		d := coordgrid.DistanceToSW(tx, tz, n.startX, n.startZ)
		return d <= maxrng+1
	}
}

// inOperableDistance checks whether target is in contact range
// (Chebyshev ≤ 1, excluding same tile). Mirrors the player-side shape
// at interaction.go:128-141.
//
// DEVIATION from TS (PathingEntity.ts:378-389): does not dispatch to
// reachedEntity / reachedLoc / reachedObj — uses Chebyshev for all
// target types. Loc shape/angle/forceapproach and Obj size reach logic
// is deferred; inherits player-side's S6l-D4 posture. Tracked follow-up.
func (n *Npc) inOperableDistance(target entity) bool {
	tx, tz, tlevel := target.Coords()
	if tlevel != n.level {
		return false
	}
	dx := n.x - tx
	if dx < 0 {
		dx = -dx
	}
	dz := n.z - tz
	if dz < 0 {
		dz = -dz
	}
	if dx > 1 || dz > 1 {
		return false
	}
	return !(dx == 0 && dz == 0)
}

// inApproachDistance checks whether target is within rng tiles
// (Chebyshev, excluding same tile) AND within TS-style line-of-sight.
// Mirrors TS PathingEntity.inApproachDistance at
// PathingEntity.ts:392-406 (NPC branch at :402-403).
//
// NAI-12 closes the NAI-11 "no LoS gating" deferral.
//
// FIDELITY: "Los for Npcs is always calculated backwards for all Entity
// types" — source is target, dest is self. TS's isApproached
// (GameMap.ts:433-435) dispatches to hasLineOfSight with
// CollisionFlag.PLAYER as extraFlag — Go equivalent
// collision.FlagBlockPlayers.
//
// DEVIATION: TS passes target.width+target.length and this.width+this.length
// (four size args). Go's HasLineOfSight collapses src to scalar srcSize;
// NAI-12 approximates with srcSize=1, destWidth=1, destLength=1 matching
// the hunt-variant convention. Tracked as size-aware follow-up in
// nai_followups.md.
func (n *Npc) inApproachDistance(rng int, target entity) bool {
	if rng <= 0 {
		return false
	}
	tx, tz, tlevel := target.Coords()
	if tlevel != n.level {
		return false
	}
	dx := n.x - tx
	if dx < 0 {
		dx = -dx
	}
	dz := n.z - tz
	if dz < 0 {
		dz = -dz
	}
	if dx > rng || dz > rng {
		return false
	}
	// LoS gate — TS PathingEntity.ts:402-405. Target-as-source + self-as-dest
	// (NPC-backward quirk); FlagBlockPlayers as extraFlag (GameMap.ts:433-435).
	// gamemap==nil short-circuits to gate-pass; see NAI-12 spec § error handling.
	if n.server != nil && n.server.gamemap != nil &&
		!n.server.gamemap.Pathfinder.LineValidator.HasLineOfSight(
			n.level, tx, tz, n.x, n.z, 1, 1, 1, collision.FlagBlockPlayers) {
		return false
	}
	return !(dx == 0 && dz == 0)
}

// SetInteraction anchors the NPC's interaction on target. Mirrors TS
// PathingEntity.setInteraction at Engine-TS/.../PathingEntity.ts:510-548.
// Closes the seven NAI-10 deferred setInteraction fields:
//  1. apRange = 10
//  2. apRangeCalled = false
//  3. targetSubject.com/typ snapshot
//  4. focus() → faceAngleX/Z
//  5. faceEntity + masks|=entitymask (Player/Npc targets)
//  6. targetX/targetZ (Loc/Obj targets)
//  7. target.IsValid() pre-check
//
// TS quirk preserved: `com ? com : -1` coerces 0 → -1 on subject.com.
//
// DEVIATION: n.entitymask is currently always 0 (the mask-plumbing
// sub-spec will wire it), so `n.masks |= n.entitymask` is a harmless
// no-op. The statement is kept for structural parity with TS so the
// mask-plumbing port is a one-line change rather than a body rewrite.
func (n *Npc) SetInteraction(kind InteractionKind, target entity, op, com int) bool {
	if !target.IsValid() {
		return false
	}

	n.target = target
	n.targetOp = op
	n.apRange = 10
	n.apRangeCalled = false

	// TS "com ? com : -1": 0 coerces to -1.
	if com == 0 {
		n.targetSubject.com = -1
	} else {
		n.targetSubject.com = com
	}

	// targetSubject.typ snapshot for changetype-detection in validateTarget.
	switch t := target.(type) {
	case *Npc:
		n.targetSubject.typ = t.typeId
	case *entitypkg.Loc:
		n.targetSubject.typ = t.Type()
	case *entitypkg.Obj:
		n.targetSubject.typ = t.Type
	default:
		n.targetSubject.typ = -1
	}

	// focus — fine-grained face-angle coord. Non-pathing targets
	// (Loc/Obj) use the engine-face path when the kind is engine;
	// pathing targets (Player/Npc) never set instant.
	tx, tz, _ := target.Coords()
	tw, tl := targetWidthLength(target)
	fx := coordgrid.Fine(tx, tw)
	fz := coordgrid.Fine(tz, tl)
	isNonPathing := false
	switch target.(type) {
	case *entitypkg.Loc, *entitypkg.Obj:
		isNonPathing = true
	}
	n.focus(fx, fz, isNonPathing && kind == InteractionEngine)

	// faceEntity (Player/Npc) or targetX/Z (Loc/Obj) dispatch.
	switch t := target.(type) {
	case *Player:
		slot := t.slot + 32768
		if n.faceEntity != slot {
			n.faceEntity = slot
			n.masks |= n.entitymask
		}
	case *Npc:
		if n.faceEntity != t.nid {
			n.faceEntity = t.nid
			n.masks |= n.entitymask
		}
	default:
		n.targetX = fx
		n.targetZ = fz
	}

	return true
}

// targetWidthLength returns the target's (width, length) for fine-grained
// coord math. 1x1 for PathingEntity; real dimensions for Loc; 1x1 for Obj.
func targetWidthLength(target entity) (width, length int) {
	if l, ok := target.(*entitypkg.Loc); ok {
		return l.Width, l.Length
	}
	return 1, 1
}

// focus records the fine-grained face-angle target. Called from
// SetInteraction with CoordGrid.fine of the target's width/length.
// Matches TS PathingEntity.focus.
//
// DEVIATION: TS takes an `instant` flag distinguishing engine-face
// from script-face, which selects between two wire-protocol paths.
// Go's current protocol doesn't branch on it, so the flag is accepted
// for signature parity but currently stored write-only. Follow-up:
// "face-instant wire protocol" sub-spec.
func (n *Npc) focus(fx, fz int, instant bool) {
	n.faceAngleX = fx
	n.faceAngleZ = fz
	_ = instant
}

// defaultMode returns the NPC's baseline mode based on its NpcType
// config. Patrol if PatrolCoord is set; else Wander if WanderRange>0;
// else None. Single source of truth used by NewNpc (initial targetOp)
// and resetDefaults (revert targetOp). Matches TS NpcType.defaultmode.
func (n *Npc) defaultMode() int {
	if n.typ == nil {
		return objtype.NPCModeNone
	}
	if len(n.typ.PatrolCoord) > 0 {
		return objtype.NPCModePatrol
	}
	if n.typ.WanderRange > 0 {
		return objtype.NPCModeWander
	}
	return objtype.NPCModeNone
}
