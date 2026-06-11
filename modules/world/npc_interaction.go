package world

import (
	"math/rand/v2"

	"github.com/zsrv/goscape/pkg/coordgrid"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/pathfinder/reach"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
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
// TS Npc.resetDefaults at Engine-TS/.../Npc.ts:412-422 @2e3bcf43 —
// ee28c1aa REMOVED the `faceEntity = -1; masks |= entitymask` tail
// (facing is now derived per-turn by setFaceEntity(), called from
// turn() after processMovementInteraction and from the ResetMasks tail).
//
// INTENTIONALLY retains NAI-11's stripped-flat shape — apRange,
// apRangeCalled, targetSubject, huntMode, huntrange, huntClock,
// huntTarget, and timerInterval are all non-cleared (TS clears them via
// clearInteraction + :418-424; Go keeps the flat shape as a tracked
// deviation). See spec § Scope — what's OUT §1 for the full deviation
// tree at docs/superpowers/specs/2026-04-23-nai-14-face-entity-clearing-design.md.
func (n *Npc) resetDefaults() {
	n.target = nil
	n.targetOp = n.defaultMode()
}

// clearInteraction resets interaction state to idle: target, targetOp,
// apRange, apRangeCalled, targetSubject. Matches TS Npc.clearInteraction
// at Engine-TS/.../Npc.ts:407-410 @2e3bcf43 — ee28c1aa REMOVED the
// `faceEntity = -1; masks |= FACE_ENTITY` tail (clients see the NPC stop
// facing via the per-turn setFaceEntity() derivation instead).
//
// Pre-existing deviation (NOT ee28c1aa scope): TS sets targetOp =
// NpcMode.NONE (0); goscape keeps -1 (NPCModeNull), which the
// processMovementInteraction failsafe coerces to defaultMode() next turn.
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
// teleport-to-spawn counter. Matches TS wanderMode at Npc.ts:703-721
// @2e3bcf43. (The pin range renamed TS's private Npc.randomWalk(range)
// to wander(range) — goscape inlines that roll right here, so the
// rename is a no-op for Go; PathingEntity.randomWalk() (no-arg, one
// tile) is a DIFFERENT method, ported at movement.go/npc_interaction.go.)
//
// The QueueWaypoint skip-if-equal-to-current guard mirrors the TS
// "if we rolled our own tile, don't queue a null path" check.
//
// The 1/8 roll gate is `moverestrict !== NOMOVE && Math.random() < 0.125`
// per TS Npc.ts:707; TS then calls `wander(wanderrange)` UNCONDITIONALLY
// (Npc.ts:708). For WanderRange=0, the inner block's
// `rand.IntN(rng*2+1)` reduces to `rand.IntN(1) == 0` so dx=dz=0; the
// QueueWaypoint skip-if-equal-to-current guard then re-queues
// (startX, startZ) only when the NPC has drifted off-spawn — matching
// TS wander(0) at Npc.ts:688-697. 2026-05-28 audit row npc-ai-4
// removed the goscape-only `&& WanderRange > 0` clause that suppressed
// the wander roll entirely for 0-range NPCs.
func (n *Npc) wanderMode(s *Server) {
	if n.typ == nil {
		return
	}
	// Read moverestrict LIVE from the current type: TS Npc.wanderMode
	// (Npc.ts:703-709) does `NpcType.get(this.type)` on every tick, so a
	// ChangeType to a different-moverestrict type takes effect immediately
	// on the next wander roll (2787f1fb removed the frozen field entirely).
	if n.liveMoveRestrict() != MoveRestrictNoMove && rand.IntN(8) == 0 {
		rng := int(n.typ.WanderRange)
		dx := rand.IntN(rng*2+1) - rng
		dz := rand.IntN(rng*2+1) - rng
		if n.startX+dx != n.x || n.startZ+dz != n.z {
			n.QueueWaypoint(n.startX+dx, n.startZ+dz)
		}
	}
	n.updateMovement(s)
	onSpawn := n.x == n.startX && n.z == n.startZ && n.level == n.startLevel
	n.wanderCounter++
	if n.wanderCounter >= 500 {
		if !onSpawn {
			n.Teleport(n.startX, n.startZ, n.startLevel)
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
		n.QueueWaypoint(dest.X, dest.Z)
	}
	if (n.x != dest.X || n.z != dest.Z) && n.nextPatrolTick > -1 && s.currentTick >= n.nextPatrolTick {
		// NAI-36-T7: pass dest.Level (was hardcoded 0) per TS Npc.ts:729.
		// PatrolCoord packs the level via PackCoord; preserving it through
		// the patrol-tele preserves multi-level patrol routes.
		n.Teleport(dest.X, dest.Z, dest.Level)
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
	n.QueueWaypoint(dest.X, dest.Z)
}

// processMovementInteraction is the NPC's per-tick movement + interaction
// dispatcher. Replaces the inline wander/patrol/advanceWaypoint block
// that NAI-2..NAI-10 kept in npc_ai.go (the block is collapsed to a
// single call by Task 30). Mirrors TS Npc.processMovementInteraction
// at Engine-TS/.../Npc.ts:562-603.
//
// Dispatch order matches TS:
//  1. delayed / dead bail.
//  2. Null-targetOp failsafe → defaultMode.
//  3. Targetless modes (None / Wander / Patrol).
//  4. Targeted-mode prelude (target-nil or validateTarget-fail → resetDefaults).
//  5. Targeted-mode dispatch: PLAYER* modes → dedicated methods (NAI-13);
//     everything else routes to aiMode.
//
// Note: last-tick coord bookkeeping (lastTickX/Z/lastLevel) and tele flag
// reset have moved to resetPathingEntity (npc_masks.go), called from
// processCleanup at end-of-tick — NAI-167.
func (n *Npc) processMovementInteraction(s *Server) {
	if n.delayed || n.dead {
		return
	}

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
	case objtype.NPCModePlayerEscape:
		n.playerEscapeMode(s)
	case objtype.NPCModePlayerFollow:
		n.playerFollowMode(s)
	case objtype.NPCModePlayerFace:
		n.playerFaceMode(s)
	case objtype.NPCModePlayerFaceClose:
		n.playerFaceCloseMode(s)
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
// (migrated into takeStep + validateAndAdvanceStep below).
func (n *Npc) updateMovement(s *Server) bool {
	// TS Npc.updateMovement (Npc.ts:339-343) does `const type =
	// NpcType.get(this.type); if (type.moverestrict === MoveRestrict.NOMOVE)
	// return false` on every tick — a ChangeType to a NoMove (or out of
	// NoMove) type takes effect immediately on the next movement tick.
	// 2787f1fb removed the frozen PathingEntity field entirely; goscape's
	// liveMoveRestrict (npc.go) is the single on-demand read.
	if n.liveMoveRestrict() == MoveRestrictNoMove {
		n.walkDir = -1
		n.runDir = -1
		return false
	}

	if n.waypointIndex >= 0 {
		// NAI-51: walktrigger consumer (TS Npc.ts:350-359). Fire BEFORE
		// step consumption. TS clears walktrigger BEFORE the script-found
		// check, so a missing script still resets the field. The n.typ
		// guard defends against the nil-typ test path; production NPCs
		// always have typ set by NewNpc, but defensive parity with TS's
		// NpcType.get(this.type) lookup avoids a nil deref here.
		if n.walktrigger != -1 && n.typ != nil && s.scriptProvider != nil {
			trigger := script.TriggerAiQueue1 + script.ServerTriggerType(n.walktrigger)
			sf := s.scriptProvider.GetByTrigger(trigger, n.typeId, n.typ.Category)
			wtArg := n.walktriggerArg
			n.walktrigger = -1
			if sf != nil {
				s.runNpcScript(sf, n, nil, trigger, []int{wtArg}, nil)
			}
		}

		// TS PathingEntity.processMovement (PathingEntity.ts:146-152): a
		// single validateAndAdvanceStep per speed slot; -1 (blocked OR
		// waypoint-consumed) leaves the dir at -1. The pre-rev-254
		// recursion is retired (f0ccbe8a) — see (*Npc).validateAndAdvanceStep.
		n.walkDir = n.validateAndAdvanceStep(s)
		n.runDir = -1
		if n.moveSpeed == MoveSpeedRun && n.walkDir != -1 {
			n.runDir = n.validateAndAdvanceStep(s)
		}
	} else {
		n.walkDir = -1
		n.runDir = -1
	}

	// NAI-82: TS Npc.updateMovement (Npc.ts:364-369) writes lastMovement =
	// World.currentTick + 1 AND resets wanderCounter = 0 when the NPC's
	// position changed this tick, and RETURNS that positional moved check
	// (rev-254 aligns the return to TS — previously goscape returned
	// "stepped this call", equivalent except when an earlier same-tick
	// teleport moved the NPC). Read by AI_ARRIVEDELAY / AI_TARGETMOVED.
	// No nil-server guard: the function already dereferences s above.
	//
	// The wanderCounter reset is what makes the wanderMode 500-tick
	// teleport-to-spawn a *stuck-recovery*: a wandering NPC that keeps moving
	// never accumulates 500, so it only snaps home after 500 consecutive
	// stationary ticks. Omitting it (pre-fix) made every healthy wandering
	// NPC teleport home every ~500 ticks (Hans / Lumbridge goblins resetting).
	moved := n.x != n.lastTickX || n.z != n.lastTickZ
	if moved {
		n.lastMovement = s.currentTick + 1
		n.wanderCounter = 0
	}
	return moved
}

// takeStep computes the (dx, dz) delta for one tile of travel toward the
// current waypoint WITHOUT mutating position. Mirrors TS PathingEntity.
// takeStep at the rev-254 pin (PathingEntity.ts:629-675, rewritten by
// f0ccbe8a): returns (0,0) on failsafe (no waypoint / nil strategy) or when
// every canTravel arm fails. The diagonal arm is gated on width==1, so
// width>1 NPCs naturally fall through to the E/W → N/S axis arms — this
// supersedes the pre-rev-254 NAI-176 D3 width>1 axis-split special case.
//
// NAI-176-D-FLY-NO-CONTENT-WIRES: MoveStrategyFly bypasses collision
// entirely (TS L654-657). No NpcType or content in goscape's cache currently
// assigns Fly; engine-fidelity only.
func (n *Npc) takeStep(s *Server) (int, int) {
	if n.waypointIndex == -1 {
		// failsafe check (TS L632-635)
		return 0, 0
	}
	cs := n.getCollisionStrategy()
	extraFlag := n.blockWalkFlag()
	if cs == nil {
		// failsafe check (TS L640-643)
		return 0, 0
	}

	dest := coordgrid.UnpackCoord(n.waypoints[n.waypointIndex])
	dir := coordgrid.Face(n.x, n.z, dest.X, dest.Z)
	dx := coordgrid.DeltaX(dir)
	dz := coordgrid.DeltaZ(dir)

	// Already on the waypoint tile (Face == -1 → zero deltas). TS feeds the
	// (0,0) deltas through canTravel, whose result is irrelevant — every arm
	// yields [0,0] either way. goscape's StepValidator panics on a (0,0)
	// offset, so short-circuit the observationally-identical result here.
	if dx == 0 && dz == 0 {
		return 0, 0
	}

	if n.moveStrategy == MoveStrategyFly {
		return dx, dz
	}

	if s == nil || s.gamemap == nil {
		// Test-fixture path: no gamemap → skip collision and step freely.
		// (goscape defensive; TS always has rsmod loaded.)
		return dx, dz
	}
	// Move diagonal (TS L659-662) — gated on width==1; width>1 NPCs never
	// step diagonally.
	if n.Width() == 1 && s.gamemap.CanTravel(n.level, n.x, n.z, dx, dz, n.Width(), extraFlag, *cs) {
		return dx, dz
	}
	// Move E/W (TS L664-667).
	if dx != 0 && s.gamemap.CanTravel(n.level, n.x, n.z, dx, 0, n.Width(), extraFlag, *cs) {
		return dx, 0
	}
	// Move N/S (TS L669-672).
	if dz != 0 && s.gamemap.CanTravel(n.level, n.x, n.z, 0, dz, n.Width(), extraFlag, *cs) {
		return 0, dz
	}
	// https://x.com/JagexAsh/status/1727609489954664502
	return 0, 0
}

// validateAndAdvanceStep validates and applies one step of movement,
// returning the step direction or -1 when no tile was covered. Mirrors TS
// PathingEntity.validateAndAdvanceStep at the rev-254 pin (PathingEntity.ts:
// 202-248, rewritten by f0ccbe8a). See the Player fork in movement.go for
// the structural notes (nomove → clear waypoints; no recursion; zone
// presence refreshed even on a blocked attempt).
func (n *Npc) validateAndAdvanceStep(s *Server) int {
	cs := n.getCollisionStrategy()
	extraFlag := n.blockWalkFlag()

	// Clear waypoints if no movement is allowed (TS L206-209).
	if cs == nil || extraFlag == collision.FlagNull {
		n.waypointIndex = -1
	}

	// If no waypoints, return (TS L211-214).
	if n.waypointIndex == -1 {
		return -1
	}

	dx, dz := n.takeStep(s)

	srcX, srcZ := n.x, n.z

	// Move entity (TS L222-224).
	n.x += dx
	n.z += dz

	// Refresh zone presence if we had a waypoint, even if we didn't move
	// (TS L226-227). Collision-follow + zone swap per refreshNpcZonePresence.
	refreshNpcZonePresence(s, n, srcX, srcZ, n.level)

	// Update waypoint index if we reached the current waypoint (TS L229-235).
	if n.waypointIndex != -1 {
		coord := coordgrid.UnpackCoord(n.waypoints[n.waypointIndex])
		if coord.X == n.x && coord.Z == n.z {
			n.waypointIndex--
		}
	}

	// If we actually moved, update orientation and steps taken (TS L237-245).
	if n.x != srcX || n.z != srcZ {
		// Focus the tile in front (faceAngle only; no FACE_COORD wire mask).
		focusX := n.x + dx
		focusZ := n.z + dz
		n.focus(coordgrid.Fine(focusX, n.size), coordgrid.Fine(focusZ, n.size), false)
		n.stepsTaken++
		return int(coordgrid.Face(srcX, srcZ, n.x, n.z))
	}

	return -1
}

// pathToTarget mirrors TS Npc.pathToTarget at the rev-254 pin
// (Npc.ts:321-337). Override of PathingEntity.pathToTarget that
// random-walks one tile when bbox-intersecting a PathingEntity target
// (f0ccbe8a — was a FindNaivePath escape). Otherwise delegates to
// pathToTargetBase which mirrors PathingEntity.pathToTarget.
func (n *Npc) pathToTarget() {
	if n.target == nil {
		return
	}

	if t, ok := n.target.(pathingEntity); ok {
		tx, tz, _ := t.Coords()
		tw, tl := t.Width(), t.Length()
		if coordgrid.Intersects(n.x, n.z, n.Width(), n.Length(), tx, tz, tw, tl) {
			// TS Npc.ts:331-334 (f0ccbe8a): overlapped → randomWalk.
			n.randomWalk()
			return
		}
	}

	n.pathToTargetBase()
}

// randomWalk queues a single waypoint one tile away on a random axis.
// Mirrors TS PathingEntity.randomWalk (PathingEntity.ts:430-439, added by
// f0ccbe8a). Distinct from the wanderMode wander-range roll (TS wander(),
// renamed from randomWalk(range) by the same commit — goscape inlines that
// roll in wanderMode).
func (n *Npc) randomWalk() {
	x, z := n.x, n.z
	if rand.IntN(2) == 0 {
		if rand.IntN(2) == 0 {
			x--
		} else {
			x++
		}
	} else {
		if rand.IntN(2) == 0 {
			z--
		} else {
			z++
		}
	}
	n.QueueWaypoint(x, z)
}

// naivePathToTarget is the base-class naive chase repath — TS PathingEntity.
// naivePathToTarget (PathingEntity.ts:441-455, added by f0ccbe8a). Queues
// the naive route unless its first coord is the NPC's current tile
// ("don't queue naive waypoint if already there").
//
// NOTE: TS passes the Loc angle in findNaivePath's extraFlag parameter slot
// (PathingEntity.ts:449) — mirrored verbatim. The Loc arm is currently
// unreachable (the only caller gates on PathingEntity targets) but kept for
// base-class parity.
func (n *Npc) naivePathToTarget() {
	if n.target == nil {
		return
	}
	angle := 0
	if loc, ok := n.target.(*entitypkg.Loc); ok {
		angle = loc.Angle()
	}
	tx, tz, _ := n.target.Coords()
	tw, tl := approachTargetSize(n.target)
	pf := n.pathfinder()
	if pf == nil {
		// (goscape defensive; TS skips this check) — gamemap absent in
		// test fixtures; queue a single waypoint at the target tile.
		n.QueueWaypoint(tx, tz)
		return
	}
	route := pf.FindNaivePath(n.level, n.x, n.z, tx, tz, n.Width(), n.Length(), tw, tl, angle, collision.TypeNormal)
	packed := routeToPacked(route)
	if len(packed) == 0 {
		// TS unpackCoord(undefined) yields NaN ≠ x/z → queueWaypoints(empty)
		// → waypoint clear; mirror the clear explicitly.
		n.queueWaypoints(nil)
		return
	}
	first := coordgrid.UnpackCoord(packed[0])
	if first.X != n.x || first.Z != n.z {
		n.queueWaypoints(packed)
	}
}

// pathToTargetBase is the shared base dispatch consumed by Npc, mirroring
// TS PathingEntity.pathToTarget (PathingEntity.ts:457-508). Identical
// structure to Player.pathToTarget — see modules/world/interaction.go for
// the cross-reference. Logic is duplicated rather than factored because of
// asymmetric server-access (Player: client.server, Npc: server).
func (n *Npc) pathToTargetBase() {
	switch n.moveStrategy {
	case MoveStrategySmart:
		n.pathToTargetSmart()
	case MoveStrategyNaive:
		n.pathToTargetNaive()
	default:
		n.pathToTargetNoStrategy()
	}
}

// pathToTargetSmart — Npc-side analogue of Player.pathToTargetSmart.
// Cross-reference: modules/world/interaction.go pathToTargetSmart.
// NB: NODE_CLIENT_ROUTEFINDER+intersect shortcut from the Player side is
// NOT mirrored here because Npc.pathToTarget already short-circuits
// intersect cases UNCONDITIONALLY (TS Npc.ts:319-335). The PathingEntity
// no-intersect case falls through to FindPathToEntity unconditionally.
func (n *Npc) pathToTargetSmart() {
	pf := n.pathfinder()
	tx, tz, _ := n.target.Coords()

	switch t := n.target.(type) {
	case *entitypkg.Loc:
		if pf == nil {
			// (goscape defensive; TS skips this check)
			n.QueueWaypoint(tx, tz)
			return
		}
		var fap int
		// (goscape defensive; TS skips this check) — TS LocType.get(t.type)
		// throws on missing; goscape returns nil and we treat as forceapproach=0.
		if cfg := n.server.locTypeOrNil(t.Type()); cfg != nil {
			fap = cfg.ForceApproach
		}
		route := pf.FindPathToLoc(n.level, n.x, n.z, tx, tz, n.Width(), t.Width, t.Length, t.Angle(), t.Shape(), fap)
		n.queueWaypoints(routeToPacked(route))

	case pathingEntity:
		// Intersect shortcut handled in pathToTarget override; this is the
		// no-intersect fallthrough.
		if pf == nil {
			// (goscape defensive; TS skips this check)
			n.QueueWaypoint(tx, tz)
			return
		}
		tw, tl := t.Width(), t.Length()
		route := pf.FindPathToEntity(n.level, n.x, n.z, tx, tz, n.Width(), tw, tl)
		n.queueWaypoints(routeToPacked(route))

	case *entitypkg.Obj:
		if n.x == tx && n.z == tz {
			n.QueueWaypoint(tx, tz)
			return
		}
		if pf == nil {
			// (goscape defensive; TS skips this check)
			n.QueueWaypoint(tx, tz)
			return
		}
		route := pf.FindPathPlain(n.level, n.x, n.z, tx, tz)
		n.queueWaypoints(routeToPacked(route))

	default:
		// Unhandled target type (TS pathToTarget has no fallthrough default).
		// (goscape defensive; TS skips this check)
		if pf == nil {
			n.QueueWaypoint(tx, tz)
			return
		}
		route := pf.FindPathPlain(n.level, n.x, n.z, tx, tz)
		n.queueWaypoints(routeToPacked(route))
	}
}

// pathToTargetNaive — Npc-side analogue of Player.pathToTargetNaive.
// Cross-reference: modules/world/interaction.go pathToTargetNaive.
// Mirrors TS PathingEntity.pathToTarget NAIVE arm (PathingEntity.ts:477-493).
func (n *Npc) pathToTargetNaive() {
	cs := n.getCollisionStrategy()
	if cs == nil {
		// nomove moverestrict returns nil = no walking allowed.
		return
	}
	extraFlag := n.blockWalkFlag()
	if extraFlag == collision.FlagNull {
		// nomove moverestrict returns NULL = no walking allowed. Unlike
		// Player.blockWalkFlag (which is unconditional), this branch CAN
		// fire on NPCs (Npc.blockWalkFlag returns NULL for MoveRestrictNoMove
		// and the default fallthrough; see Npc.ts:381-398).
		return
	}

	if _, ok := n.target.(pathingEntity); ok {
		// TS PathingEntity.ts:485-486 (f0ccbe8a): dispatch to
		// naivePathToTarget — angle-aware, plain NORMAL collision (the
		// pre-rev-254 extraFlag/collisionStrategy-parameterized
		// FindNaivePath call is retired; the guards above remain).
		n.naivePathToTarget()
	} else {
		tx, tz, _ := n.target.Coords()
		n.QueueWaypoint(tx, tz)
	}
}

// pathToTargetNoStrategy — Npc-side analogue of Player.pathToTargetNoStrategy.
// Cross-reference: modules/world/interaction.go pathToTargetNoStrategy.
// Mirrors TS PathingEntity.pathToTarget trailing else (PathingEntity.ts:494-507).
func (n *Npc) pathToTargetNoStrategy() {
	if n.getCollisionStrategy() == nil {
		return
	}
	if n.blockWalkFlag() == collision.FlagNull {
		return
	}
	tx, tz, _ := n.target.Coords()
	n.QueueWaypoint(tx, tz)
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
// The *Npc branch's `!dead && !delayed` check mirrors TS Npc.isValid
// (Npc.ts:370-375). goscape's Npc.IsValid() now provides the same
// predicate (npc.go:453), so the spelled-out check here is redundant
// defense in depth — kept inline for parity with the gate-3 site in
// (*Player).validateTarget (interaction.go:208).
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
// Five branches: PLAYERFOLLOW (always true), PLAYERESCAPE (AND-gated
// retreat using NPC-to-start AND target-to-start), OP (maxrange+1 with
// corner-removal quirk), AP (maxrange + attackrange SW-distance), default
// (maxrange+1 SW-distance). Matches TS Npc.targetWithinMaxRange at
// Engine-TS/.../Npc.ts:629-680.
func (n *Npc) targetWithinMaxRange() bool {
	if n.target == nil {
		return true
	}
	if n.typ == nil {
		return false
	}

	// TS :633-635 — PLAYERFOLLOW has no retreat bound.
	if n.targetOp == objtype.NPCModePlayerFollow {
		return true
	}

	maxrng := int(n.typ.MaxRange)
	attackrng := int(n.typ.AttackRange)
	tx, tz, _ := n.target.Coords()

	// TS :657-673 — PLAYERESCAPE retreat. Size-aware distanceTo from BOTH
	// NPC and target to (startX, startZ); rejects only when BOTH exceed
	// maxrange. TS quirk: the start-coord rectangle adopts the SUBJECT's
	// width/length on each call (n's size for the n-call, target's size
	// for the target-call). NAI-20 closes the NAI-12 size-approximation
	// follow-up at this site.
	if n.targetOp == objtype.NPCModePlayerEscape {
		tw, tl := approachEntitySize(n.target)
		distanceToEscape := coordgrid.DistanceTo(
			n.x, n.z, n.size, n.size,
			n.startX, n.startZ, n.size, n.size)
		targetDistanceFromStart := coordgrid.DistanceTo(
			tx, tz, tw, tl,
			n.startX, n.startZ, tw, tl)
		if targetDistanceFromStart > maxrng && distanceToEscape > maxrng {
			return false
		}
		return true
	}

	switch {
	case checkOpTrigger(n.targetOp):
		// TS :640-648 — maxrange+1 with corner-removal quirk. dx/dz
		// locality matches TS (:640-641 computes them inside this branch).
		dx := tx - n.startX
		if dx < 0 {
			dx = -dx
		}
		dz := tz - n.startZ
		if dz < 0 {
			dz = -dz
		}
		maxAxis := max(dx, dz)
		if maxAxis > maxrng+1 {
			return false
		}
		if dx == maxrng+1 && dz == maxrng+1 {
			return false
		}
		return true

	case checkApTrigger(n.targetOp):
		// TS :651-654 — SW-distance up to maxrange + attackrange. Per TS,
		// this branch uses distanceToSW (no size); NAI-20 audit confirmed
		// TS does not size this comparison. KEEP DistanceToSW.
		d := coordgrid.DistanceToSW(tx, tz, n.startX, n.startZ)
		return d <= maxrng+attackrng

	default:
		// TS :676 — SW-distance up to maxrange + 1. Per TS, this branch
		// uses distanceToSW (no size); KEEP DistanceToSW (NAI-20 audit).
		d := coordgrid.DistanceToSW(tx, tz, n.startX, n.startZ)
		return d <= maxrng+1
	}
}

// TargetWithinMaxRange exports the unexported targetWithinMaxRange for
// the ActiveNpc.TargetWithinMaxRange surface consumed by NPC_INRANGE
// (TS NpcOps.ts:556-558). Thin wrapper; no logic. NAI-160 T7.
func (n *Npc) TargetWithinMaxRange() bool {
	return n.targetWithinMaxRange()
}

// inOperableDistance reports whether n is in contact range of target.
// Mirrors TS PathingEntity.inOperableDistance (PathingEntity.ts:378-390):
//   - Loc targets dispatch to pkg/pathfinder/reach.Reached (shape /
//     angle / forceapproach-aware) with srcSize=n.size (NAI-91).
//   - Obj targets dispatch to reach.Reached with locShape=-1
//     (reachedObj). No OR-chain — TS base class uses reachedObj only;
//     Player.ts:1110 overrides to OR reachedEntity but Npc inherits the
//     base (NAI-152 B2 T3). Same-tile pickup succeeds via the
//     strategy.go:37 short-circuit.
//   - PathingEntity (Player, Npc) targets dispatch to reach.Reached with
//     locShape=-2 (reachedEntity) (NAI-173). srcSize=n.size with the
//     "if srcSize <= 0 { srcSize = 1 }" defensive guard mirrored from the
//     Loc/Obj branches.
//
// Defensive: nil n.server / nil gamemap falls through to Chebyshev so test
// fixtures constructing minimal *Npc without a server keep working
// (goscape defensive; production Server.Init always sets gamemap).
func (n *Npc) inOperableDistance(target entity) bool {
	tx, tz, tlevel := target.Coords()
	if tlevel != n.level {
		return false
	}
	if loc, ok := target.(*entitypkg.Loc); ok && n.server != nil && n.server.gamemap != nil {
		flags := n.server.gamemap.Pathfinder.Flags
		var fap int
		if cfg := n.server.locTypeOrNil(loc.Type()); cfg != nil {
			fap = cfg.ForceApproach
		}
		srcSize := n.size
		if srcSize <= 0 {
			srcSize = 1
		}
		return reach.Reached(flags, n.level, n.x, n.z, tx, tz,
			loc.Width, loc.Length, srcSize, loc.Angle(), loc.Shape(), fap)
	}
	if obj, ok := target.(*entitypkg.Obj); ok && n.server != nil && n.server.gamemap != nil {
		// TS PathingEntity.ts:389 (base class) — reachedObj only. Asymmetric
		// with Player.ts:1110's reachedEntity || reachedObj override; Npc
		// inherits the base. Per audit_full_method_against_ts.md +
		// ts_base_class_read_for_inherited_behavior.md.
		flags := n.server.gamemap.Pathfinder.Flags
		srcSize := n.size
		if srcSize <= 0 {
			srcSize = 1
		}
		return reach.Reached(flags, n.level, n.x, n.z, tx, tz,
			obj.Width, obj.Length, srcSize, 0, -1, 0)
	}
	if t, ok := target.(pathingEntity); ok && n.server != nil && n.server.gamemap != nil {
		flags := n.server.gamemap.Pathfinder.Flags
		srcSize := n.size
		if srcSize <= 0 {
			srcSize = 1
		}
		// TS PathingEntity.ts:383 — reachedEntity (locShape=-2,
		// blockAccessFlags=0). Npc inherits this from the base class
		// (no Player-style Obj override; that's npc_interaction.go's Obj branch).
		return reach.Reached(flags, n.level, n.x, n.z, tx, tz,
			t.Width(), t.Length(), srcSize, 0, -2, 0)
	}
	// Defensive: nil server / nil gamemap (test fixtures), or non-pathing
	// non-Loc non-Obj target (test doubles only). Production target is always
	// one of those types and production server always has a gamemap.
	return inOperableDistanceCheb(n.x, n.z, tx, tz)
}

// approachEntitySize returns target (width, length) for the NPC-side
// LoS sizing call in inApproachDistance. Mirrors TS PathingEntity.width
// and .length per concrete entity type:
//
//	*Player → (1, 1)           players are always square size-1
//	*Npc    → (typ.Size, typ.Size)  NPCs are square; typ.Size is side length
//	default → (1, 1)           test doubles / future non-pathing entities
//
// Length is returned for API symmetry with TS; current callers consume
// only width because Go's HasLineOfSight collapses src to scalar srcSize
// (see FIDELITY note on inApproachDistance).
func approachEntitySize(e entity) (width, length int) {
	switch t := e.(type) {
	case *Player:
		return 1, 1
	case *Npc:
		size := int(t.size)
		return size, size
	default:
		return 1, 1
	}
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
// FIDELITY: LoS sizing uses approachEntitySize per target concrete
// type (*Player → 1, *Npc → typ.Size; all current pathing entities
// are square). Go's HasLineOfSight collapses src to scalar srcSize
// (linevalidator.go:21 forces srcLength = srcWidth in the underlying
// RayCast), which is lossless for square entities. NAI-18 closed the
// NAI-12 tracked size-aware deferral.
func (n *Npc) inApproachDistance(rng int, target entity) bool {
	if rng <= 0 {
		return false
	}
	tx, tz, tlevel := target.Coords()
	if tlevel != n.level {
		return false
	}
	tw, tl := approachTargetSize(target)
	// TS PathingEntity.ts:395-398 — a source on/under the target footprint is
	// not in approach distance, but ONLY when the target is a PathingEntity
	// (Player / Npc). For Loc / Obj targets, TS skips this bail entirely: a
	// player or NPC standing on/inside a Loc footprint (e.g. a banker counter,
	// shop stall) or on an Obj tile is still in approach distance to fire its
	// AP script. npc-ai-5 / pathing-5 / interaction-5: the gate was applied
	// unconditionally, suppressing valid Loc/Obj AP fires whenever the
	// source overlapped the target footprint.
	switch target.(type) {
	case *Player, *Npc:
		if coordgrid.Intersects(n.x, n.z, n.size, n.size, tx, tz, tw, tl) {
			return false
		}
	}
	// TS PathingEntity.ts:404 — CoordGrid.distanceTo (edge-aware Chebyshev
	// across both footprints), NOT origin-corner distance. Matches the
	// player-side inApproachDistance and the size-aware targetWithinMaxRange.
	if coordgrid.DistanceTo(n.x, n.z, n.size, n.size, tx, tz, tw, tl) > rng {
		return false
	}
	// LoS gate — TS PathingEntity.ts:402-405. Target-as-source + self-as-dest
	// (NPC-backward quirk); FlagBlockPlayers as extraFlag (GameMap.ts:433-435).
	// gamemap==nil short-circuits to gate-pass; see NAI-12 spec § error handling.
	targetSize, _ := approachEntitySize(target)
	selfSize := int(n.size)
	if n.server != nil && n.server.gamemap != nil &&
		!n.server.gamemap.Pathfinder.LineValidator.HasLineOfSight(
			n.level, tx, tz, n.x, n.z, targetSize, selfSize, selfSize,
			collision.FlagBlockPlayers) {
		return false
	}
	return true
}

// SetInteraction anchors the NPC's interaction on target. Mirrors TS
// PathingEntity.setInteraction at Engine-TS/.../PathingEntity.ts:526-557
// @2e3bcf43. Side-effect set (ex-NAI-10 deferrals):
//  1. apRange = 10
//  2. apRangeCalled = false
//  3. targetSubject.com/typ snapshot
//  4. focus() → faceAngleX/Z
//  5. targetX/targetZ (Loc/Obj targets)
//  6. target.IsValid() pre-check
//
// faceEntity is NOT written here — ee28c1aa @2e3bcf43 removed the
// Player/Npc faceEntity arms; facing is derived per-turn by
// setFaceEntity() (face_entity.go).
//
// TS quirk preserved: `com ? com : -1` coerces 0 → -1 on subject.com.
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

	// TS PathingEntity.ts:544-547 (f0ccbe8a) — script-set interactions
	// re-allow naive repathing.
	if kind == InteractionScript {
		n.allowRepath = AllowRepathBeforeDest
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

	// ee28c1aa @2e3bcf43: setInteraction no longer writes faceEntity — the
	// Player/Npc faceEntity arms moved to setFaceEntity() (face_entity.go),
	// called per-turn (npc_ai.go turn() + ResetMasks tail). Only the
	// NonPathingEntity targetX/Z cache remains: TS PathingEntity.ts:551-554
	// @2e3bcf43.
	if isNonPathing {
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

// focus records the fine-grained face-angle coord. Mirrors TS
// PathingEntity.focus (Engine-TS/src/engine/entity/PathingEntity.ts:321-333).
// instant=true ALSO writes faceSquareX/Z to (fx, fz) and ORs
// NpcMaskFaceCoord into masks.
//
// Coord-frame note: focus() takes RAW fine coords. Distinct from
// (*Npc).FaceSquare in modules/world/npc_masks.go which takes absolute
// coords and applies *2+1.
//
// Drivers per TS: takeStep (PathingEntity.ts:220), Teleport
// (PathingEntity.ts:289), reorient (PathingEntity.ts:353,358),
// setInteraction (PathingEntity.ts:528). The setInteraction site
// (modules/world/npc_interaction.go:665) is the only one that ever
// passes instant=true.
func (n *Npc) focus(fx, fz int, instant bool) {
	n.faceAngleX = fx
	n.faceAngleZ = fz
	if instant {
		n.faceSquareX = fx
		n.faceSquareZ = fz
		n.masks |= rsbuf.NpcMaskFaceCoord
	}
}

// unfocus restores the default-south face-angle. Mirrors TS
// PathingEntity.unfocus (Engine-TS/src/engine/entity/PathingEntity.ts:338-341).
// No mask emit — TS unfocus leaves coordmask alone (faceAngle is the
// "where am I oriented" channel; mask is the wire signal, only fired
// from focus(instant=true) or FaceSquare).
//
// Caller: resetEntityForRespawn (modules/world/npc_registry.go), the
// goscape-shape equivalent of TS Npc.resetEntity(true) at Npc.ts:284.
func (n *Npc) unfocus() {
	n.faceAngleX = coordgrid.Fine(n.x, n.size)
	n.faceAngleZ = coordgrid.Fine(n.z-1, n.size)
}

// effectiveFaceCoord returns the fine coord the NPC should be shown facing on
// the wire: its active faceSquare when set, otherwise its resting orientation
// (faceAngle — default south via unfocus on spawn, or the last-movement
// direction). The rsbuf FACE_COORD payload is always forced into the NPC
// low-def (renderer.go), so a freshly-observed resting NPC whose faceSquare is
// unset (-1) must fall back to faceAngle; otherwise the client receives
// FACE_COORD(-1,-1) and orients the NPC to its own default (north-east) instead
// of south. Mirrors TS, which passes faceAngleX/Z to rsbuf as the orientation.
func (n *Npc) effectiveFaceCoord() (x, z int) {
	if n.faceSquareX == -1 && n.faceSquareZ == -1 {
		return n.faceAngleX, n.faceAngleZ
	}
	return n.faceSquareX, n.faceSquareZ
}

// reorient is the Npc-side per-tick refocus invoked from
// Server.processInfo before rsbuf compute. Mirrors TS
// PathingEntity.reorient at PathingEntity.ts:349-361. Same shape as
// (*Player).reorient.
func (n *Npc) reorient() {
	switch t := n.target.(type) {
	case *Player:
		n.focus(coordgrid.Fine(t.x, 1), coordgrid.Fine(t.z, 1), false)
	case *Npc:
		n.focus(coordgrid.Fine(t.x, t.size), coordgrid.Fine(t.z, t.size), false)
	default:
		_ = t
		if n.targetX != -1 && n.stepsTaken == 0 {
			n.focus(n.targetX, n.targetZ, false)
			n.targetX = -1
			n.targetZ = -1
		}
	}
}

// defaultMode returns the NPC's baseline mode — the stored
// NpcType.DefaultMode (opcode 210, default WANDER). Single source of truth
// used by NewNpc (initial targetOp) and resetDefaults (revert targetOp).
// Mirrors TS, which reads type.defaultmode directly (Npc.ts:100, 414); it
// does NOT re-derive the mode from patrol/wander config.
func (n *Npc) defaultMode() int {
	if n.typ == nil {
		return objtype.NPCModeNone
	}
	return n.typ.DefaultMode
}

// clearPatrol resets the patrol-tick countdown so the NPC immediately
// resumes patrol-pathing on the next tick. Mirrors TS Npc.clearPatrol at
// Engine-TS/.../Npc.ts:377-379.
//
// Called by the NPC_SETMODE script handler when the new mode is PATROL
// (NAI-36).
func (n *Npc) clearPatrol() {
	n.nextPatrolTick = -1
}
