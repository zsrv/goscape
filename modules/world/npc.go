package world

import (
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// NPC lifecycle constants.
const (
	NpcLifecycleForever = 0
	NpcLifecycleRespawn = 1
	NpcLifecycleDespawn = 2
)

// NPC AI mode constants (sub-spec 3c).
const (
	NpcModeNone   = -1
	NpcModeWander = 0
	NpcModePatrol = 1
)

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
	server       *Server          // back-reference; set by Server.addNpc
	activeScript *script.ScriptState
	delayed      bool
	delayedUntil int
	queue        []script.NpcQueueRequest

	// === AI ===
	targetOp        int
	wanderCounter   int
	nextPatrolTick  int
	nextPatrolPoint int
	delayedPatrol   bool

	// === interaction ===
	target     entity
	faceEntity int

	// === masks ===
	masks      int
	entitymask int

	// === mask state ===
	animID, animDelay                         int
	sayText                                   []byte
	damageAmt, damageType                     int
	curHP, baseHP                             int
	spotanimID, spotanimHeight, spotanimDelay int
	faceSquareX, faceSquareZ                  int
	changeTypeID                              int
}

// NewNpc constructs an Npc at the given coord, anchoring its spawn point.
func NewNpc(nid, typeId, x, z, level int, typ *objtype.NpcType) *Npc {
	mode := NpcModeNone
	if len(typ.PatrolCoord) > 0 {
		mode = NpcModePatrol
	} else if typ.WanderRange > 0 {
		mode = NpcModeWander
	}

	return &Npc{
		nid:             nid,
		typeId:          typeId,
		typ:             typ,
		uid:             (typeId << 16) | nid,
		lifecycle:       NpcLifecycleRespawn,
		respawnRate:     int(typ.RespawnRate),
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
		targetOp:        mode,
		nextPatrolPoint: 0,
		faceEntity:      -1,
		animID:          -1,
		animDelay:       -1,
		damageAmt:       -1,
		damageType:      -1,
		curHP:           initialHP(typ),
		baseHP:          initialHP(typ),
		spotanimID:      -1,
		spotanimHeight:  -1,
		spotanimDelay:   -1,
		faceSquareX:     -1,
		faceSquareZ:     -1,
		changeTypeID:    -1,
	}
}

// initialHP returns the max HP stored in an NpcType, defaulting to 0 when
// typ is nil or Stats doesn't cover the Hitpoints slot. Called from NewNpc
// (to seed curHP + baseHP) and from *Npc.ResetHP.
func initialHP(typ *objtype.NpcType) int {
	if typ == nil || len(typ.Stats) <= objtype.NpcStatHitpoints {
		return 0
	}
	hp := int(typ.Stats[objtype.NpcStatHitpoints])
	if hp < 0 {
		return 0
	}
	return hp
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
