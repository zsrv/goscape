package world

import (
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/script"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
)

// worldVarsView adapts *Server to script.WorldVars. Kept value-typed so
// tests can construct it without a running server.
type worldVarsView struct {
	s *Server
}

func (w worldVarsView) VarsInt(id int) int32 {
	if w.s == nil || id < 0 || id >= len(w.s.vars) {
		return 0
	}
	return w.s.vars[id]
}

func (w worldVarsView) SetVarsInt(id int, val int32) {
	if w.s == nil || id < 0 || id >= len(w.s.vars) {
		return
	}
	w.s.vars[id] = val
}

func (w worldVarsView) VarsString(id int) string {
	if w.s == nil || id < 0 || id >= len(w.s.varsStrings) {
		return ""
	}
	return w.s.varsStrings[id]
}

func (w worldVarsView) SetVarsString(id int, val string) {
	if w.s == nil || id < 0 || id >= len(w.s.varsStrings) {
		return
	}
	w.s.varsStrings[id] = val
}

// CurrentTick returns the server's current tick counter. Used by
// MAP_CLOCK opcode.
func (w worldVarsView) CurrentTick() int {
	if w.s == nil {
		return 0
	}
	return w.s.currentTick
}

// PlayerCount returns the number of players currently in the world.
// Used by PLAYERCOUNT opcode.
func (w worldVarsView) PlayerCount() int {
	if w.s == nil {
		return 0
	}
	w.s.playersMu.RLock()
	n := len(w.s.playerLoop)
	w.s.playersMu.RUnlock()
	return n
}

// MapMembers returns 1 if the server is a members world, else 0.
// Matches TS Environment.NODE_MEMBERS. Used by MAP_MEMBERS opcode.
func (w worldVarsView) MapMembers() int {
	if w.s == nil || !w.s.cfg.NodeMembers {
		return 0
	}
	return 1
}

// MapLive returns 1 if the server is in production mode, else 0.
// Matches TS Environment.NODE_PRODUCTION. Used by MAP_LIVE opcode.
func (w worldVarsView) MapLive() int {
	if w.s == nil || !w.s.cfg.NodeProduction {
		return 0
	}
	return 1
}

// IsMapBlocked delegates to gamemap.Pathfinder.Flags. FlagBlockWalk is the
// canonical "this tile blocks walking" flag at pkg/pathfinder/collision/flag.go:41.
// Mirrors TS GameMap.isMapBlocked (CollisionFlag.WALK_BLOCKED). NAI-35-T6.
func (w worldVarsView) IsMapBlocked(level, x, z int) bool {
	if w.s == nil || w.s.gamemap == nil {
		return false
	}
	flag := w.s.gamemap.Pathfinder.Flags.Get(x, z, level)
	return flag&collision.FlagBlockWalk != 0
}

// IsMulti delegates to the world's GameMap.IsMulti, swapping arg order to
// match the WorldVars convention (level, x, z) — gamemap uses (x, z, level).
// Mirrors TS World.gameMap.isMulti(coord). NAI-120 Bundle 2A.
func (w worldVarsView) IsMulti(level, x, z int) bool {
	if w.s == nil || w.s.gamemap == nil {
		return false
	}
	return w.s.gamemap.IsMulti(x, z, level)
}

// IsFreeToPlay delegates to gamemap.IsFreeToPlay. Mirrors TS
// World.gameMap.isFreeToPlay. NAI-35-T6.
func (w worldVarsView) IsFreeToPlay(x, z int) bool {
	if w.s == nil || w.s.gamemap == nil {
		return false
	}
	return w.s.gamemap.IsFreeToPlay(x, z)
}

// AnimMap delegates to Server.AnimMap. Used by SPOTANIM_MAP (opcode 1020).
// NAI-36 T1: interface-satisfaction shim. Server.AnimMap already exists at
// world_zone.go:76.
func (w worldVarsView) AnimMap(level, x, z, spotanim, height, delay int) {
	if w.s == nil {
		return
	}
	w.s.AnimMap(level, x, z, spotanim, height, delay)
}

// RemoveObj implements script.WorldVars.RemoveObj. Type-asserts the
// script-side ActiveObj to the world-side *entitypkg.Obj and routes
// via Server.RemoveObj.
//
// NAI-115-D2: TS passes ObjType.respawnrate as duration to
// World.removeObj; goscape's Server.RemoveObj has no duration arg
// (RESPAWN-lifecycle respawn-after-delay is a foundation gap).
func (w worldVarsView) RemoveObj(obj script.ActiveObj) {
	if w.s == nil {
		return
	}
	realObj, ok := obj.(*entitypkg.Obj)
	if !ok {
		return
	}
	w.s.RemoveObj(realObj)
}

// RemoveNpc implements script.WorldVars.RemoveNpc. Type-asserts the
// script-side ActiveNpc to *Npc and calls the existing
// Server.removeNpc. Mirrors RemoveObj. Type-assert miss is a silent
// no-op (matches RemoveObj behavior); production NPC pointers are
// always *Npc.
func (w worldVarsView) RemoveNpc(npc script.ActiveNpc, duration int) {
	realNpc, ok := npc.(*Npc)
	if !ok {
		return
	}
	w.s.removeNpc(realNpc, duration)
}

// AddObj implements script.WorldVars.AddObj. Constructs a
// DESPAWN-lifecycle Obj at (level, x, z) with typeID/count, sets
// ReceiverID to the caller's UID (or PublicReceiver=-1 for broadcast),
// and routes via Server.AddObj. Mirrors TS World.addObj for despawnable
// drops.
//
// NAI-115-D2: duration is accepted but not yet honored (no
// despawn-after-N-ticks scheduler hooked up). DESPAWN-lifecycle objs
// (the firemaking smoke target) are unaffected because the smoke only
// requires the obj to appear at all.
func (w worldVarsView) AddObj(level, x, z, typeID, count, duration, receiverID int) script.ActiveObj {
	if w.s == nil {
		return nil
	}
	obj := entitypkg.NewObj(level, x, z, entitypkg.LifecycleDespawn, typeID, count)
	obj.ReceiverID = receiverID
	w.s.AddObj(obj, receiverID)
	_ = duration // NAI-115-D2: foundation gap
	if w.s.cfg.NodeDebug && w.s.log != nil {
		w.s.log.Info("nai128.obj.add",
			"level", level,
			"x", x,
			"z", z,
			"typeID", typeID,
			"count", count,
			"duration", duration,
			"receiverID", receiverID,
		)
	}
	return obj
}

// EnqueueObjDelayed implements script.WorldVars.EnqueueObjDelayed
// (NAI-134). Constructs a DESPAWN-lifecycle Obj at (level,x,z) with
// typeID/count, sets ReceiverID, and appends to s.objDelayedQueue via
// s.enqueueObjDelayed.
//
// The Obj is constructed at enqueue time (not drain time), mirroring TS
// InvOps.ts:207-208 where `new Obj(...)` is the call-site argument to
// `objDelayedQueue.addTail`.
//
// NAI-115-D2 sibling: duration is plumbed onto the queue entry but the
// drain (processObjDelayedQueue, obj_delayed_queue.go) discards it
// because Server.AddObj does not yet accept a duration param. Single-point
// retire when NAI-115-D2 closes.
func (w worldVarsView) EnqueueObjDelayed(level, x, z, typeID, count, duration, delay, receiverID int) {
	if w.s == nil {
		return
	}
	obj := entitypkg.NewObj(level, x, z, entitypkg.LifecycleDespawn, typeID, count)
	obj.ReceiverID = receiverID
	w.s.enqueueObjDelayed(obj, receiverID, duration, delay)
	if w.s.cfg.NodeDebug && w.s.log != nil {
		w.s.log.Info("nai134.obj.delayed.enqueue",
			"level", level, "x", x, "z", z,
			"typeID", typeID, "count", count,
			"duration", duration, "delay", delay,
			"receiverID", receiverID,
		)
	}
}

// LookupPlayerByUID implements script.WorldVars. Delegates to
// Server.LookupPlayerByUID (server.go:791). NAI-127 Bundle 1.
func (w worldVarsView) LookupPlayerByUID(uid int) script.ActivePlayer {
	if w.s == nil {
		return nil
	}
	return w.s.LookupPlayerByUID(uid)
}

// MapProjAnim implements script.WorldVars.MapProjAnim. Stub at
// NAI-150 T1; real delegation lands in T5.
func (w worldVarsView) MapProjAnim(level, srcX, srcZ, dstX, dstZ, target, spotanim, srcHeight, dstHeight, startDelay, endDelay, peak, arc int) {
}

// LookupNpcBySlot implements script.WorldVars.LookupNpcBySlot. Stub
// at NAI-150 T1; real lookup lands in T5.
func (w worldVarsView) LookupNpcBySlot(slot int) script.ActiveNpc { return nil }

// Compile-time conformance assertion for script.WorldVars. Adding any
// new WorldVars method that worldVarsView fails to implement breaks
// the build here. NAI-150 T1.
var _ script.WorldVars = worldVarsView{}
