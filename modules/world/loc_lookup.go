package world

import (
	entitypkg "github.com/zsrv/goscape/pkg/entity"
)

// GetLoc returns the loc at (level, x, z) whose type matches locId, or
// nil if no such loc exists in the corresponding zone. Mirrors TS
// World.getLoc(x, z, level, locId) used by OpLocHandler validation.
//
// Iteration is O(zone-loc-count); zones typically hold a handful of
// locs, so a linear scan is fine. If profiling later shows hot zones,
// a coord-keyed map can replace the slice.
func (s *Server) GetLoc(level, x, z, locId int) *entitypkg.Loc {
	zn := s.zoneMap.Get(level, x, z)
	if zn == nil {
		return nil
	}
	for _, l := range zn.Locs {
		if l == nil {
			continue
		}
		if l.Level == level && l.X == x && l.Z == z && l.Type() == locId {
			return l
		}
	}
	return nil
}
