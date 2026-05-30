package world

import (
	entitypkg "github.com/zsrv/goscape/pkg/entity"
)

// GetLoc returns the loc at (level, x, z) whose type matches locId AND
// is currently active (loc.IsActive), or nil if no such loc exists in
// the corresponding zone. Mirrors TS World.getLoc → Zone.getLoc →
// Zone.getLocsSafe (Zone.ts:259-266, 471-477) which yields only
// loc.isValid()-passing locs; Entity.isValid() === isActive per
// Entity.ts:32-34, and Loc does not override that predicate. Used by
// OpLoc / OpLocT / OpLocU validation (handler_oploc.go) and by LOC_FIND
// (via serverLocOps.GetLoc → script LocOps.ts:79-94). Skipping the
// isActive gate would let stale removed locs (still linked in zn.Locs
// when lifecycle==RESPAWN) re-fire interactions.
//
// Iteration is O(zone-loc-count); zones typically hold a handful of
// locs, so a linear scan is fine. If profiling later shows hot zones,
// a coord-keyed map can replace the slice.
func (s *Server) GetLoc(level, x, z, locId int) *entitypkg.Loc {
	zn := s.zoneMap.Get(level, x, z)
	for _, l := range zn.Locs {
		if l == nil {
			continue
		}
		if l.Level == level && l.X == x && l.Z == z && l.Type() == locId && l.IsActive {
			return l
		}
	}
	return nil
}
