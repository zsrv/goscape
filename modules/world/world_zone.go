package world

import (
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/objtype"
)

// AddLoc routes a loc spawn through the world's zone map. Wires
// collision flags via gamemap.ChangeLocCollision when the loc's
// LocType has BlockWalk=true. Mirrors TS World.addLoc
// (Engine-TS/src/engine/World.ts:1337-1348).
//
// IsActive=true is written by the called Zone.AddLoc (pkg/zone/zone.go),
// matching TS Zone.addLoc (Zone.ts:226). duration > 0 schedules
// a despawn-revert via NonPathing.SetLifeCycle, which Registers the
// loc in s.locObjTracker for per-tick processing.
func (s *Server) AddLoc(loc *entitypkg.Loc, duration int) {
	if s.gamemap != nil && s.locTypes != nil {
		if lt := s.locTypeOrNil(loc.Type()); lt != nil && lt.BlockWalk {
			s.gamemap.ChangeLocCollision(loc.Shape(), loc.Angle(), lt.BlockRange,
				loc.Length, loc.Width, loc.X, loc.Z, loc.Level, true)
		}
	}
	z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
	z.AddLoc(loc)
	s.TrackZone(z)
	loc.SetLifeCycle(duration, s.currentTick, s.locObjTracker)
}

// ChangeLoc rewrites the loc's render fields to (typ, shape, angle)
// and reschedules its lifecycle to despawn/revert at currentTick+duration.
// Mirrors TS World.changeLoc (Engine-TS/src/engine/World.ts:1350-1386).
//
// Order matters per TS: (1) early-return if DESPAWN+!IsActive (don't
// return inactive DESPAWN to game world; goscape uses IsActive where
// TS uses isValid — see spec D-N86-2 — defensive gate, TS-equivalent);
// (2) remove old collision; (3) loc.Change(); (4) add new collision;
// (5) zone.ChangeLoc; (6) trackZone; (7) SetLifeCycle (duration if
// changed-or-DESPAWN, else -1 to untrack a no-op static change).
//
// IsActive=true is written by the called Zone.ChangeLoc (pkg/zone/zone.go),
// matching TS Zone.changeLoc (Zone.ts:232).
func (s *Server) ChangeLoc(loc *entitypkg.Loc, typ, shape, angle, duration int) {
	if loc.Lifecycle == entitypkg.LifecycleDespawn && !loc.IsActive {
		return
	}
	if loc.IsActive && s.gamemap != nil && s.locTypes != nil {
		if oldLt := s.locTypeOrNil(loc.Type()); oldLt != nil && oldLt.BlockWalk {
			s.gamemap.ChangeLocCollision(loc.Shape(), loc.Angle(), oldLt.BlockRange,
				loc.Length, loc.Width, loc.X, loc.Z, loc.Level, false)
		}
	}
	loc.Change(typ, shape, angle)
	if s.gamemap != nil && s.locTypes != nil {
		if newLt := s.locTypeOrNil(typ); newLt != nil && newLt.BlockWalk {
			s.gamemap.ChangeLocCollision(loc.Shape(), loc.Angle(), newLt.BlockRange,
				loc.Length, loc.Width, loc.X, loc.Z, loc.Level, true)
		}
	}
	z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
	z.ChangeLoc(loc)
	s.TrackZone(z)
	if loc.IsChanged() || loc.Lifecycle == entitypkg.LifecycleDespawn {
		loc.SetLifeCycle(duration, s.currentTick, s.locObjTracker)
	} else {
		loc.SetLifeCycle(-1, s.currentTick, nil)
	}
}

// RemoveLoc clears collision (if BlockWalk), routes the zone-side
// removal, and reschedules respawn (RESPAWN) or untracks (DESPAWN).
// Mirrors TS World.removeLoc (Engine-TS/src/engine/World.ts:1402-1425).
//
// IsActive=false is written by the called Zone.RemoveLoc (pkg/zone/zone.go),
// matching TS Zone.removeLoc (Zone.ts:254).
func (s *Server) RemoveLoc(loc *entitypkg.Loc, duration int) {
	if !loc.IsActive {
		return
	}
	if s.gamemap != nil && s.locTypes != nil {
		if lt := s.locTypeOrNil(loc.Type()); lt != nil && lt.BlockWalk {
			s.gamemap.ChangeLocCollision(loc.Shape(), loc.Angle(), lt.BlockRange,
				loc.Length, loc.Width, loc.X, loc.Z, loc.Level, false)
		}
	}
	z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
	z.RemoveLoc(loc)
	s.TrackZone(z)
	if loc.Lifecycle == entitypkg.LifecycleRespawn {
		loc.SetLifeCycle(duration, s.currentTick, s.locObjTracker)
	} else {
		loc.SetLifeCycle(-1, s.currentTick, nil)
	}
}

// locTypeOrNil returns the LocType for id with bounds checking, or nil
// if id is out of range or the type is unloaded.
func (s *Server) locTypeOrNil(id int) *objtype.LocType {
	if s.locTypes == nil || id < 0 || id >= len(s.locTypes.Configs) {
		return nil
	}
	return s.locTypes.Configs[id]
}

// AnimLoc routes a loc animation.
func (s *Server) AnimLoc(loc *entitypkg.Loc, seq int) {
	z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
	z.AnimLoc(loc, seq)
	s.TrackZone(z)
}

// MergeLoc routes a multi-tile loc merge.
func (s *Server) MergeLoc(
	loc *entitypkg.Loc,
	playerSlot, startCycle, endCycle int,
	east, south, west, north int,
) {
	z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
	z.MergeLoc(loc, playerSlot, startCycle, endCycle, east, south, west, north)
	s.TrackZone(z)
}

// AddObj routes a ground-item spawn. receiverID == zone.PublicReceiver for
// public drops; otherwise the receiver's player slot.
func (s *Server) AddObj(obj *entitypkg.Obj, receiverID int) {
	z := s.zoneMap.Get(obj.Level, obj.X, obj.Z)
	z.AddObj(obj, receiverID)
	s.TrackZone(z)
}

// ChangeObj updates obj.Count and routes an OBJ_COUNT follows event.
func (s *Server) ChangeObj(obj *entitypkg.Obj, oldCount, newCount int) {
	z := s.zoneMap.Get(obj.Level, obj.X, obj.Z)
	z.ChangeObj(obj, oldCount, newCount, s.currentTick)
	s.TrackZone(z)
}

// RemoveObj routes an obj removal. Respects the lastLifecycleTick check.
func (s *Server) RemoveObj(obj *entitypkg.Obj) {
	z := s.zoneMap.Get(obj.Level, obj.X, obj.Z)
	z.RemoveObj(obj, s.currentTick)
	s.TrackZone(z)
}

// RevealObj transitions a private drop to public.
func (s *Server) RevealObj(obj *entitypkg.Obj, receiverSlot int) {
	z := s.zoneMap.Get(obj.Level, obj.X, obj.Z)
	z.RevealObj(obj, receiverSlot)
	s.TrackZone(z)
}

// AnimMap routes a tile-based spotanim event.
func (s *Server) AnimMap(level, x, zc, spotanim, height, delay int) {
	z := s.zoneMap.Get(level, x, zc)
	z.AnimMap(x, zc, spotanim, height, delay)
	s.TrackZone(z)
}

// MapProjAnim routes a projectile event. The zone is keyed by the source
// tile (the TS reference does the same).
func (s *Server) MapProjAnim(
	level, srcX, srcZ, dstX, dstZ int,
	target, spotanim, srcHeight, dstHeight int,
	startDelay, endDelay, peak, arc int,
) {
	z := s.zoneMap.Get(level, srcX, srcZ)
	z.MapProjAnim(srcX, srcZ, dstX, dstZ,
		target, spotanim, srcHeight, dstHeight,
		startDelay, endDelay, peak, arc)
	s.TrackZone(z)
}

// IsZoneAllocated reports whether the (level, x, z) zone has been allocated
// in the FlagMap collision layer (pkg/pathfinder/collision/flagmap.go:142).
// Mirrors TS World.gameMap.isZoneAllocated; called by Player.Teleport and
// Npc.Teleport per TS PathingEntity.ts:271 to silently reject teleports
// into uninitialised zones. NAI-36-T7.
//
// When s.gamemap is nil (test fixtures that bypass the standard map
// loader), returns true so existing tests don't see false rejections.
// Production paths always have gamemap set during App start.
func (s *Server) IsZoneAllocated(level, x, z int) bool {
	if s == nil || s.gamemap == nil {
		return true
	}
	return s.gamemap.Pathfinder.Flags.IsZoneAllocated(x, z, level)
}
