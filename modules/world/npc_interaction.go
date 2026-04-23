package world

import (
	"github.com/zsrv/goscape/pkg/coordgrid"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/objtype"
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

// resetDefaults clears target/targetOp to defaultMode baseline. Matches
// TS Npc.resetDefaults — INTENTIONALLY does NOT clear apRange,
// apRangeCalled, faceEntity, or masks. Those are overwritten only by
// the next SetInteraction call.
func (n *Npc) resetDefaults() {
	n.target = nil
	n.targetOp = n.defaultMode()
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
// (Chebyshev, excluding same tile). Mirrors the player-side shape at
// interaction.go:148-164.
//
// DEVIATION from TS (PathingEntity.ts:392-406): no LoS gating. TS's
// isApproached walks the collision map; NAI-11 inherits player-side's
// S6l-D4 no-LoS posture. Tracked follow-up.
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
