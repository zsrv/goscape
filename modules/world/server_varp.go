package world

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/script"
	"github.com/zsrv/goscape/pkg/zone"

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

// NodeID returns the server's configured node ID. Used as the world_id
// partition key on telemetry envelopes emitted from script handlers.
func (w worldVarsView) NodeID() int {
	if w.s == nil {
		return 0
	}
	return w.s.cfg.NodeID
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
// via Server.RemoveObj with the caller's respawn duration.
func (w worldVarsView) RemoveObj(obj script.ActiveObj, duration int) {
	if w.s == nil {
		return
	}
	realObj, ok := obj.(*entitypkg.Obj)
	if !ok {
		return
	}
	w.s.RemoveObj(realObj, duration)
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

// AddNpcAt implements script.WorldVars.AddNpcAt. Looks up the NpcType,
// constructs a despawn-lifecycle Npc via NewNpc (overriding the default
// RESPAWN lifecycle to DESPAWN), and routes through (*Server).addNpc with
// firstSpawn=true. Bubbles errNpcsFull on registry-full; returns a
// goscape-defensive error on unknown typeID (TS-side checkNpcType already
// rejects at the handler — TS skips this check). Mirrors TS World.addNpc
// consumer pattern at NpcOps.ts:42-53. NAI-163 B3.
func (w worldVarsView) AddNpcAt(level, x, z, typeID, duration int) (script.ActiveNpc, error) {
	if w.s == nil {
		return nil, fmt.Errorf("AddNpcAt: no server")
	}
	if w.s.npcTypes == nil {
		return nil, fmt.Errorf("AddNpcAt: no NpcTypes loaded")
	}
	if typeID < 0 || typeID >= len(w.s.npcTypes.Configs) {
		return nil, fmt.Errorf("AddNpcAt: typeID %d out of range", typeID)
	}
	typ := w.s.npcTypes.Configs[typeID]
	if typ == nil {
		return nil, fmt.Errorf("AddNpcAt: no NpcType for id %d", typeID)
	}
	n := NewNpc(0 /* nid placeholder; allocated inside addNpc */, typeID, x, z, level, typ)
	n.lifecycle = NpcLifecycleDespawn
	if err := w.s.addNpc(n, duration, true); err != nil {
		return nil, err
	}
	return n, nil
}

// AddObj implements script.WorldVars.AddObj. Constructs a
// DESPAWN-lifecycle Obj at (level, x, z) with typeID/count, sets
// ReceiverID to the caller's UID (or PublicReceiver=-1 for broadcast),
// initialises the receiver-targeted reveal countdown, and routes via
// Server.AddObj. Mirrors TS World.addObj (Engine-TS/.../World.ts:1467-1484).
func (w worldVarsView) AddObj(level, x, z, typeID, count, duration, receiverID int) script.ActiveObj {
	if w.s == nil {
		return nil
	}
	obj := entitypkg.NewObj(level, x, z, entitypkg.LifecycleDespawn, typeID, count)
	obj.ReceiverID = receiverID
	if receiverID != zone.PublicReceiver {
		obj.Reveal = entitypkg.ObjReveal
	}
	w.s.AddObj(obj, receiverID, duration)
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
func (w worldVarsView) EnqueueObjDelayed(level, x, z, typeID, count, duration, delay, receiverID int) {
	if w.s == nil {
		return
	}
	obj := entitypkg.NewObj(level, x, z, entitypkg.LifecycleDespawn, typeID, count)
	obj.ReceiverID = receiverID
	if receiverID != zone.PublicReceiver {
		obj.Reveal = entitypkg.ObjReveal
	}
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

// MapProjAnim implements script.WorldVars.MapProjAnim. Delegates to
// Server.MapProjAnim (modules/world/world_zone.go:164) which routes
// the event by source-coord zone and tracks the zone for end-of-tick
// flush. NAI-150.
func (w worldVarsView) MapProjAnim(level, srcX, srcZ, dstX, dstZ, target, spotanim, srcHeight, dstHeight, startDelay, endDelay, peak, arc int) {
	if w.s == nil {
		return
	}
	w.s.MapProjAnim(level, srcX, srcZ, dstX, dstZ, target, spotanim,
		srcHeight, dstHeight, startDelay, endDelay, peak, arc)
}

// LookupNpcBySlot implements script.WorldVars.LookupNpcBySlot.
// Returns s.npcs[slot] cast to script.ActiveNpc, or nil for OOB slot
// or empty slot. Slot-only — does NOT verify the high-16 type bits,
// unlike NpcLookup.FindNpcByUID. Mirrors TS World.getNpc(slot).
// NAI-150.
func (w worldVarsView) LookupNpcBySlot(slot int) script.ActiveNpc {
	if w.s == nil {
		return nil
	}
	if slot < 0 || slot >= len(w.s.npcs) {
		return nil
	}
	n := w.s.npcs[slot]
	if n == nil {
		return nil
	}
	return n
}

// GetObj implements script.WorldVars.GetObj. Delegates to the existing
// Server.GetObj at modules/world/obj_lookup.go:13 (already consumed by
// modules/world/handler_opobj.go for OPOBJ-family handlers). The
// returned *entity.Obj implements script.ActiveObj via the NAI-153
// surface extension. Returns nil when no matching obj is at the tile
// or the caller lacks visibility. NAI-154.
func (w worldVarsView) GetObj(level, x, z, objId, receiverUID int) script.ActiveObj {
	if w.s == nil {
		return nil
	}
	o := w.s.GetObj(level, x, z, objId, receiverUID)
	if o == nil {
		return nil
	}
	return o
}

// ZoneObjs implements script.WorldVars.ZoneObjs. Reads the zone's Objs
// slice directly via zoneMap.Get and adapts each *entity.Obj to
// script.ActiveObj. Mirrors serverLocOps.AllLocsInZone at
// modules/world/script_loc_ops.go:85-92. Empty zone or out-of-range
// returns nil/empty. NAI-154.
func (w worldVarsView) ZoneObjs(level, zoneX, zoneZ int) []script.ActiveObj {
	if w.s == nil {
		return nil
	}
	z := w.s.zoneMap.Get(level, zoneX, zoneZ)
	if z == nil {
		return nil
	}
	out := make([]script.ActiveObj, 0, len(z.Objs))
	for _, o := range z.Objs {
		out = append(out, o)
	}
	return out
}

// IsIndoors reports whether the tile at (x, z, level) carries the
// FlagRoof bit in the global collision FlagMap. Implements
// script.WorldVars.IsIndoors. Mirrors TS isIndoors (GameMap.ts:417-419).
// NAI-162 B1.
func (w worldVarsView) IsIndoors(x, z, level int) bool {
	if w.s == nil || w.s.gamemap == nil {
		return false
	}
	flag := w.s.gamemap.Pathfinder.Flags.Get(x, z, level)
	return collision.IsIndoors(flag)
}

// MergeLoc implements script.WorldVars.MergeLoc. Type-asserts loc to
// *entitypkg.Loc and player to *Player to extract the concrete slot.
// Delegates to Server.MergeLoc. Type-assert misses are silent no-ops
// (matches RemoveObj behavior). NAI-162 B2.
func (w worldVarsView) MergeLoc(loc script.ActiveLoc, player script.ActivePlayer, startCycle, endCycle, south, east, north, west int) {
	if w.s == nil {
		return
	}
	realLoc, ok := loc.(*entitypkg.Loc)
	if !ok {
		return
	}
	var playerSlot int
	if player != nil {
		playerSlot = player.Slot()
	}
	w.s.MergeLoc(realLoc, playerSlot, startCycle, endCycle, east, south, west, north)
}

// Compile-time conformance assertion for script.WorldVars. Adding any
// new WorldVars method that worldVarsView fails to implement breaks
// the build here. NAI-150 T1.
var _ script.WorldVars = worldVarsView{}
