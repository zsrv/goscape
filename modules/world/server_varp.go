package world

import (
	"github.com/zsrv/goscape/pkg/pathfinder/collision"
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

// IsZoneAllocated reports whether the (level, x, z) zone is allocated.
// Delegates to *Server.IsZoneAllocated, which forwards to the FlagMap
// collision layer at pkg/pathfinder/collision/flagmap.go:142. Used by
// Teleport safety check per TS PathingEntity.ts:271 (NPC_TELE / PLAYER_TELE
// downstream of the script VM). NAI-36-T7.
func (w worldVarsView) IsZoneAllocated(level, x, z int) bool {
	if w.s == nil {
		// Permissive default mirrors *Server.IsZoneAllocated's nil-gamemap
		// handling: tests that don't wire a server should not see false
		// rejections.
		return true
	}
	return w.s.IsZoneAllocated(level, x, z)
}
