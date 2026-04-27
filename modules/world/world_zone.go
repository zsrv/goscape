package world

import (
	entitypkg "github.com/zsrv/goscape/pkg/entity"
)

// AddLoc routes a loc spawn/change through the world's zone map.
func (s *Server) AddLoc(loc *entitypkg.Loc) {
	z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
	z.AddLoc(loc)
	s.TrackZone(z)
}

// ChangeLoc routes a loc type/shape/angle mutation.
func (s *Server) ChangeLoc(loc *entitypkg.Loc) {
	z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
	z.ChangeLoc(loc)
	s.TrackZone(z)
}

// RemoveLoc routes a loc removal.
func (s *Server) RemoveLoc(loc *entitypkg.Loc) {
	z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
	z.RemoveLoc(loc)
	s.TrackZone(z)
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
