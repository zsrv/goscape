package world

import (
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
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
	// varns is per-NPC vars; nil until first SetNpcVarN write.
	varns []int32

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

	// === movement ===
	moveSpeed       MoveSpeed
	moveRestrict    MoveRestrict
	moveStrategy    MoveStrategy
	walkDir, runDir int
	waypointIndex   int
	waypoints       [25]int
	tele            bool
	stepsTaken      int

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
	changeTypeID                              int
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
		changeTypeID:    -1,
		entitymask:      rsbuf.NpcMaskFaceEntity,
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
func (n *Npc) EnqueueScriptForTrigger(trigger script.ServerTriggerType, delay, intArg int) {
	n.queue = append(n.queue, script.NpcQueueRequest{
		Trigger: trigger,
		Delay:   delay,
		IntArg:  intArg,
	})
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
// "clear" command). Callers do no bounds validation; the consumer
// (processNpcHunt) validates when looking up the HuntType. Mirrors
// TS NpcOps.ts:178-185. Implements script.ActiveNpc.SetHuntMode.
func (n *Npc) SetHuntMode(mode int) {
	n.huntMode = mode
}

// revertType restores the NPC to its baseline type. Called from the
// Events block (npc_ai.go:37-40) when lifecycleTick hits 0 on
// RESPAWN+alive, and from the respawn path on revival.
//
// Branches on resetOnRevert (written by changeTypeImpl):
//   - resetOnRevert=false (KEEPALL path): TS Npc.ts:1086-1090 light path.
//     Restore typeId/uid + raise CHANGE_TYPE mask. No stats reset, no
//     queue clear, no waypoint clear, no hunt-field reset. Intended
//     for short-lived morphs that must preserve combat state.
//   - resetOnRevert=true (default, CHANGETYPE path): heavy-path behavior
//     matching TS resetEntity at Engine-TS/.../Npc.ts:280-317 —
//     inline reset of typeId/uid/typ, full 6-slot stats reseed
//     (TS resetEntity:287-290; expanded from S6d's HP-only reseed),
//     queue clear, waypoint clear, tele flag + CHANGE_TYPE mask, and
//     hunt-field reset (NAI-7; TS Npc.ts:309-312).
//
// What revertType does NOT do on either branch (intentional):
//   - varn resets (future; VarNpc subsystem not yet wired)
//   - activeScript clear (TS behaviour: a revert does not cancel an
//     in-flight script)
//
// NAI-17-D1 (tracked deviation): TS's heavy path is World.removeNpc +
// World.addNpc — a despawn+respawn that re-runs the constructor. Go
// does an INLINE reset instead, pre-existing since S6d. See spec §8.
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

	// DEVIATION NAI-17-D1: inline reset; see doc comment above.
	// Heavy path — inline reset matching TS resetEntity:280-317 semantics
	// (minus the World.removeNpc/addNpc structural call).
	if n.typeId != n.baseType {
		n.typeId = n.baseType
		n.uid = (n.typeId << 16) | n.nid
		if newTyp := n.lookupType(n.baseType); newTyp != nil {
			n.typ = newTyp
		}
	}
	// Full 6-slot stats reseed (TS resetEntity:287-290).
	if n.typ != nil {
		for i := range min(objtype.NpcStatCount, len(n.typ.Stats)) {
			v := int(n.typ.Stats[i])
			n.levels[i] = v
			n.baseLevels[i] = v
		}
	}
	n.queue = nil
	n.waypointIndex = -1
	n.tele = true
	n.masks |= rsbuf.NpcMaskChangeType
	// NAI-7: hunt-field resets. Matches TS resetEntity at
	// Engine-TS/.../Npc.ts:309-312.
	n.huntClock = 0
	n.huntTarget = nil
	if n.typ != nil {
		n.huntRange = int(n.typ.HuntRange)
		n.huntMode = n.typ.HuntMode
	}
	n.resetOnRevert = true // re-arm default for next morph cycle
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
