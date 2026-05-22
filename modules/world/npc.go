package world

import (
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
	"github.com/zsrv/goscape/pkg/zone"
)

// NPC lifecycle constants.
const (
	NpcLifecycleForever = 0
	NpcLifecycleRespawn = 1
	NpcLifecycleDespawn = 2
)

// npcTargetSubject captures the "initial target snapshot" fields TS
// Npc uses in validateTarget to detect mid-interaction changetypes.
// Mirrors TS targetSubject = { com: number, type: number }.
type npcTargetSubject struct {
	com int // -1 when unset (TS truthy-coerced from falsy values)
	typ int // -1 when unset or when target is a Player
}

// Npc is a non-player game entity.
type Npc struct {
	nid    int
	typeId int
	typ    *objtype.NpcType

	// uid = (typeId << 16) | nid; computed in NewNpc and exposed via NpcUID.
	uid int
	// varns is per-NPC int-typed vars; sized to len(server.varnTypes.Configs)
	// at first resetEntityForRespawn (called inside Server.addNpc). Nil for
	// raw &Npc{} test literals; reads via NpcVarN return 0 defensively. Per-
	// type seeded by resetEntityForRespawn (TS Npc.ts:296-303) — INT→0,
	// non-INT-non-STRING→-1.
	varns []int32
	// varnsString is the parallel STRING-typed slot array. Sized identically
	// to varns; nil for raw &Npc{} test literals; reads via NpcVarNString
	// return "" defensively. Mirrors TS Npc.varsString.
	varnsString []string

	// === lifecycle ===
	lifecycle                  int
	lifecycleTick              int
	respawnRate                int
	dead                       bool
	startX, startZ, startLevel int
	baseType                   int
	regenInterval              int
	regenClock                 int

	// === coords ===
	x, z, level                     int
	lastTickX, lastTickZ, lastLevel int
	originX, originZ                int

	// zoneListElement is the NPC's intrusive subscription element in
	// pkg/zone.Zone.npcs. Set by Zone.EnterNpc; nilled after Zone.LeaveNpc.
	// Per NAI-28 Bundle 2.
	zoneListElement *zone.Element[zone.NpcLike]

	// === movement ===
	moveSpeed       MoveSpeed
	moveRestrict    MoveRestrict
	moveStrategy    MoveStrategy
	walkDir, runDir int
	waypointIndex   int
	waypoints       [25]int
	tele            bool
	stepsTaken      int
	// NAI-82 (writer) + NAI-125 (reader): TS PathingEntity.lastMovement
	// (Engine-TS/.../PathingEntity.ts:56). Written to currentTick + 1 at
	// end of updateMovement when position changed (npc_interaction.go:334);
	// read by NPC_ARRIVEDELAY via ActiveNpc.LastMovement (handlers_npc.go).
	lastMovement int

	// === script state ===
	server        *Server // back-reference; set by Server.addNpc
	activeScript  *script.ScriptState
	delayed       bool
	delayedUntil  int
	queue         []script.NpcQueueRequest
	timerInterval int
	timerClock    int

	// === hunt ===
	huntMode   int
	huntRange  int
	huntClock  int
	huntTarget entity

	// === AI ===
	targetOp        int
	wanderCounter   int
	nextPatrolTick  int
	nextPatrolPoint int
	delayedPatrol   bool

	// walktrigger queues a deferred AI-queue trigger (0..19, -1 = unset)
	// to fire on the next updateMovement tick (BEFORE step consumption).
	// Written by the NPC_WALKTRIGGER (opcode 2545) handler at
	// pkg/script/handlers_npc.go:407 (transformed queueID-1). Read +
	// cleared by (*Npc).updateMovement at npc_interaction.go (NAI-51 T2.1).
	// Mirrors TS Npc.walktrigger / Npc.ts:343-360. Default in NewNpc is -1
	// (sentinel); raw &Npc{...} test literals default to 0 — safe because
	// existing tests build via NewNpc, and the consumer's `n.typ != nil`
	// guard short-circuits any literal that omits typ.
	walktrigger    int
	walktriggerArg int

	// === interaction ===
	target        entity
	faceEntity    int
	apRange       int              // NAI-11: default 10; -1 sentinel = "no AP script"
	apRangeCalled bool             // NAI-11
	targetSubject npcTargetSubject // NAI-11
	targetX       int              // NAI-11: fine-grained coord for non-PathingEntity targets
	targetZ       int
	faceAngleX    int // NAI-11: fine-grained coord, mask-emitted via faceSquare
	faceAngleZ    int

	// === geometry snapshot (NAI-20) ===
	// Captured at NewNpc; UNCHANGED by changetype to mirror TS PathingEntity
	// (World.ts:1271, 1302). Read by addNpc/removeNpc collision toggles
	// instead of n.typ.Size / n.typ.BlockWalk so a size-changing morph→revert
	// cycle leaves base-size collision flags rather than morph-size flags.
	blockWalk int
	size      int

	// === masks ===
	masks      int
	entitymask int

	// === mask state ===
	animID, animDelay                         int
	sayText                                   []byte
	damageAmt, damageType                     int
	levels                                    [objtype.NpcStatCount]int // NAI-17: current (boosted) stat values
	baseLevels                                [objtype.NpcStatCount]int // NAI-17: base values (regen convergence target)
	resetOnRevert                             bool                      // NAI-17: TS Npc.ts:72; CHANGETYPE→true, KEEPALL→false
	spotanimID, spotanimHeight, spotanimDelay int
	faceSquareX, faceSquareZ                  int

	// heroPoints tracks per-player damage contributions for loot routing on
	// NPC death. Initialised to cap=16 in NewNpc. NAI-120 Bundle 2D.
	heroPoints HeroPoints

	// OrientationX, OrientationZ default to -1 per upstream npc.rs:16-17.
	// See NAI-30-D1 (orientation field plumbed without producer).
	OrientationX, OrientationZ int

	changeTypeID int
}

// NewNpc constructs an Npc at the given coord, anchoring its spawn point.
func NewNpc(nid, typeId, x, z, level int, typ *objtype.NpcType) *Npc {
	n := &Npc{
		nid:             nid,
		typeId:          typeId,
		baseType:        typeId,
		typ:             typ,
		uid:             (typeId << 16) | nid,
		lifecycle:       NpcLifecycleRespawn,
		respawnRate:     int(typ.RespawnRate),
		timerInterval:   int(typ.Timer),
		regenInterval:   int(typ.RegenRate),
		huntMode:        typ.HuntMode,
		huntRange:       int(typ.HuntRange),
		blockWalk:       typ.BlockWalk,
		size:            int(typ.Size),
		startX:          x,
		startZ:          z,
		startLevel:      level,
		x:               x,
		z:               z,
		level:           level,
		originX:         x,
		originZ:         z,
		lastTickX:       -1,
		lastTickZ:       -1,
		lastLevel:       -1,
		moveSpeed:       MoveSpeedInstant,
		moveStrategy:    MoveStrategyNaive,
		moveRestrict:    MoveRestrict(typ.MoveRestrict),
		walkDir:         -1,
		runDir:          -1,
		waypointIndex:   -1,
		nextPatrolPoint: 0,
		walktrigger:     -1,
		walktriggerArg:  0,
		faceEntity:      -1,
		apRange:         10,
		apRangeCalled:   false,
		targetSubject:   npcTargetSubject{com: -1, typ: -1},
		targetX:         -1,
		targetZ:         -1,
		faceAngleX:      -1,
		faceAngleZ:      -1,
		animID:          -1,
		animDelay:       -1,
		damageAmt:       -1,
		damageType:      -1,
		spotanimID:      -1,
		spotanimHeight:  -1,
		spotanimDelay:   -1,
		faceSquareX:     -1,
		faceSquareZ:     -1,
		OrientationX:    -1,
		OrientationZ:    -1,
		changeTypeID:    -1,
		entitymask:      rsbuf.NpcMaskFaceEntity,
		heroPoints:      NewHeroPoints(16), // NAI-120 Bundle 2D
	}
	n.targetOp = n.defaultMode()
	// NAI-17: seed levels[]/baseLevels[] from typ.Stats (mirrors TS Npc.ts:90-94).
	if typ != nil {
		for i := range min(objtype.NpcStatCount, len(typ.Stats)) {
			v := int(typ.Stats[i])
			n.levels[i] = v
			n.baseLevels[i] = v
		}
	}
	n.resetOnRevert = true
	return n
}

// Slot returns the NPC's nid for the entity interface.
func (n *Npc) Slot() int { return n.nid }

// Width returns the NPC's tile footprint width. NPCs are square (size×size);
// width and length both return n.size. Mirrors TS Npc.width which equals
// NpcType.size at construction.
func (n *Npc) Width() int { return n.size }

// Length returns the NPC's tile footprint length. Square: equals Width().
func (n *Npc) Length() int { return n.size }

// blockWalkFlag returns the CollisionFlag this NPC imposes on its
// occupied tile during pathfinding. Mirrors TS Npc.blockWalkFlag
// (Npc.ts:381-398). goscape MoveRestrict has no BLOCKED_NORMAL — that
// TS branch is skipped.
func (n *Npc) blockWalkFlag() int {
	switch n.moveRestrict {
	case MoveRestrictNormal:
		return collision.FlagBlockNPCs
	case MoveRestrictBlocked:
		return collision.FlagOpen
	case MoveRestrictIndoors:
		return collision.FlagBlockNPCs
	case MoveRestrictOutdoors:
		return collision.FlagBlockNPCs
	case MoveRestrictNoMove:
		return collision.FlagNull
	case MoveRestrictPassthru:
		return collision.FlagOpen
	default:
		return collision.FlagNull
	}
}

// getCollisionStrategy returns the collision search type for this NPC,
// or nil for MoveRestrictNoMove. Mirrors TS PathingEntity.getCollisionStrategy
// (PathingEntity.ts:558-575). Same as Player.getCollisionStrategy because
// the TS impl lives on the PathingEntity base.
func (n *Npc) getCollisionStrategy() *collision.Type {
	switch n.moveRestrict {
	case MoveRestrictNormal:
		t := collision.TypeNormal
		return &t
	case MoveRestrictBlocked:
		t := collision.TypeBlocked
		return &t
	case MoveRestrictIndoors:
		t := collision.TypeIndoors
		return &t
	case MoveRestrictOutdoors:
		t := collision.TypeOutdoors
		return &t
	case MoveRestrictNoMove:
		return nil
	case MoveRestrictPassthru:
		t := collision.TypeNormal
		return &t
	default:
		return nil
	}
}

// StoreActiveScript saves a Suspended ScriptState so Npc.turn() can
// resume it when the NPC's delay expires. Part of the ActiveNpc
// interface; mirrors *Player.StoreActiveScript.
func (n *Npc) StoreActiveScript(state *script.ScriptState) {
	n.activeScript = state
}

// ClearActiveScript discards any stored ScriptState. Called after
// Finished/Aborted runs. Part of the ActiveNpc interface; mirrors
// *Player.ClearActiveScript.
func (n *Npc) ClearActiveScript() {
	n.activeScript = nil
}

// Cleanup mirrors TS Npc.cleanup at Engine-TS/src/engine/entity/Npc.ts:187-193.
// Zeros identity / script / hunt / queue fields after (*Server).removeNpc
// has released the registry slot on DESPAWN-lifecycle. Defensive
// nullification: any consumer still holding the *Npc pointer post-DESPAWN
// reads -1 sentinels rather than valid-looking state. NAI-19.
func (n *Npc) Cleanup() {
	n.nid = -1
	n.uid = -1
	n.activeScript = nil
	n.huntTarget = nil
	n.queue = nil
}

// OnScriptFinishedOrAborted handles the Finished/Aborted post-Execute
// tail for an npc-anchored script. If state matches the npc's
// activeScript, nulls it; otherwise no-op. Mirrors TS
// Npc.executeScript tail (Npc.ts:226-228). The match-guard preserves
// an NpcSuspended-stored activeScript when a different fresh script
// Finishes on the same npc in the same tick. NPCs have no modals.
//
// NAI-54 T2.
func (n *Npc) OnScriptFinishedOrAborted(state *script.ScriptState) {
	if n.activeScript != state {
		return
	}
	n.activeScript = nil
}

// SetDelayed marks the NPC as suspended for `ticks` more ticks starting
// next tick. delayedUntil = currentTick + 1 + ticks, matching TS
// Npc.delay() and ActivePlayer.SetDelayed semantics.
func (n *Npc) SetDelayed(ticks int) {
	n.delayed = true
	n.delayedUntil = n.server.currentTick + 1 + ticks
}

// EnqueueScriptForTrigger appends a queued ai_queueN dispatch.
// Implements script.ActiveNpc.EnqueueScriptForTrigger. Script
// resolution is deferred to fire time via
// scriptProvider.GetByTrigger — matches TS Npc.enqueueScript.
// lastIntArg is stored on the request and copied into state.LastInt
// at fire time (mirrors TS Npc.ts:554-555).
func (n *Npc) EnqueueScriptForTrigger(trigger script.ServerTriggerType, delay, lastIntArg int) {
	n.queue = append(n.queue, script.NpcQueueRequest{
		Trigger: trigger,
		Delay:   delay,
		LastInt: lastIntArg,
	})
	if n.server != nil && n.server.cfg.NodeDebug && n.server.log != nil {
		n.server.log.Info("nai128.npc.enqueue",
			"npc", n.uid,
			"typeId", n.typeId,
			"trigger", int(trigger),
			"delay", delay,
			"lastInt", lastIntArg,
			"queueLen", len(n.queue),
		)
	}
}

// SetTimer sets the tick interval between ai_timer trigger fires.
// interval == -1 is a silent no-op, matching TS Npc.setTimer at
// Engine-TS/.../Npc.ts:210-214. Implements script.ActiveNpc.SetTimer.
func (n *Npc) SetTimer(interval int) {
	if interval == -1 {
		return
	}
	n.timerInterval = interval
}

// SetHuntRange sets the NPC's hunt search radius. Called by the
// NPC_SETHUNT opcode. Implements script.ActiveNpc.SetHuntRange.
func (n *Npc) SetHuntRange(r int) {
	n.huntRange = r
}

// SetHuntMode sets the NPC's HuntType id. -1 clears the hunt mode
// (unlike SetTimer's -1 no-op — SetHuntMode accepts -1 as a valid
// "clear" command). Validation of non-(-1) ids against the HuntType
// registry happens at the NPC_SETHUNTMODE opcode site (handlers_npc.go
// checkHuntType), mirroring TS check(huntTypeId, HuntTypeValid) at
// NpcOps.ts:185; processNpcHunt still defensively rejects out-of-range
// ids that slipped through other call paths. Mirrors TS NpcOps.ts:178-186.
// Implements script.ActiveNpc.SetHuntMode.
func (n *Npc) SetHuntMode(mode int) {
	n.huntMode = mode
}

// SetWalkTrigger sets the deferred AI-queue trigger index (0..19) that
// fires when this NPC completes a walk step. Called by the
// NPC_WALKTRIGGER handler (handlers_npc.go) after queueID validation.
// Mirrors TS Npc.walktrigger field write at NpcOps.ts:488.
func (n *Npc) SetWalkTrigger(queueID int) { n.walktrigger = queueID }

// SetWalkTriggerArg sets the arg passed to the walktrigger script when
// it eventually fires. Mirrors TS Npc.walktriggerArg field write at
// NpcOps.ts:489.
func (n *Npc) SetWalkTriggerArg(arg int) { n.walktriggerArg = arg }

// revertType restores the NPC to its baseType after a temporary CHANGETYPE
// or KEEPALL morph.
//
// Branches on resetOnRevert (written by changeTypeImpl):
//   - resetOnRevert=false (KEEPALL path): TS Npc.ts:1086-1090 light path.
//     Restore typeId/uid + raise CHANGE_TYPE mask. No stats reset, no
//     queue clear, no waypoint clear, no hunt-field reset. Intended
//     for short-lived morphs that must preserve combat state.
//   - resetOnRevert=true (default, CHANGETYPE path): structural TS port
//     per Npc.ts:1083-1085 — World.removeNpc(this, -1) + World.addNpc(
//     this, -1, false). The addNpc respawn cycle reseeds typeId/uid/typ,
//     reseeds all 6 stats, clears queue/waypoints, teles to
//     (startX, startZ), and re-arms collision flags.
//
// What revertType does NOT do on either branch (intentional):
//   - activeScript clear (TS behaviour: a revert does not cancel an
//     in-flight script)
//
// What revertType does NOT do on the KEEPALL light path only:
//   - varn resets (heavy path reseeds all varns via
//     resetEntityForRespawn at npc_registry.go:157; KEEPALL
//     deliberately preserves varn state to match TS Npc.ts:1086-1090).
//
// Tail re-arm: sets resetOnRevert = true on BOTH branches so a
// subsequent CHANGETYPE on the same NPC starts from the default. TS
// gets this for free via the ctor rerun; Go must re-arm explicitly.
func (n *Npc) revertType() {
	if !n.resetOnRevert {
		// Light path — TS Npc.ts:1086-1090.
		if n.typeId != n.baseType {
			n.typeId = n.baseType
			n.uid = (n.typeId << 16) | n.nid
		}
		n.masks |= rsbuf.NpcMaskChangeType
		n.resetOnRevert = true
		return
	}

	// Heavy path — structural TS port per Npc.ts:1083-1085.
	n.server.removeNpc(n, -1)
	_ = n.server.addNpc(n, -1, false) // err only on slot-full; firstSpawn=false skips alloc
	n.resetOnRevert = true            // re-arm default for next morph cycle
}

// IsValid returns whether the NPC's session slot is alive (!n.dead).
// This maps to TS Entity.isActive — the base liveness flag. TS
// Npc.isValid is the stricter predicate (additionally checks !delayed);
// in Go that delayed-gate lives in validateTarget at the target's call
// site.
//
// DEVIATION: TS isValid is a single method; in Go the "not delayed"
// half is enforced externally rather than inline, to keep the layering
// rule "pkg/entity knows nothing about scheduling state".
func (n *Npc) IsValid() bool {
	return !n.dead
}
