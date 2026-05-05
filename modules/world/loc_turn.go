package world

import (
	entitypkg "github.com/zsrv/goscape/pkg/entity"
)

// turnLoc is the per-tick dispatch for a tracked Loc. Called from
// Server.processZones for each NonPathing in s.locObjTracker whose
// Parent() is a *Loc. Mirrors TS Loc.turn (Engine-TS/.../Loc.ts:54-74).
//
// Goscape uses Server.currentTick as the authoritative clock and stores
// the absolute target transition tick in LifecycleTick (set via
// Entity.SetLifecycle). TS decrements lifecycleTick-- per tick; the
// observable behavior is equivalent (deviation D-N86-4 in spec §5).
func (s *Server) turnLoc(l *entitypkg.Loc, now int) {
	if l.LifecycleTick != now {
		return
	}
	switch {
	case l.Lifecycle == entitypkg.LifecycleDespawn && l.IsActive:
		s.RemoveLoc(l, 0)
	case l.Lifecycle == entitypkg.LifecycleRespawn && l.IsChanged() && l.IsActive:
		s.RevertLoc(l)
	case l.Lifecycle == entitypkg.LifecycleRespawn && !l.IsActive:
		s.AddLoc(l, 0)
	default:
		// Mirrors TS console.error fallthrough — should not happen.
		// Unconditionally untrack to prevent unbounded re-iteration.
		s.log.Error("loc tracked but no event matched",
			"type", l.Type(), "x", l.X, "z", l.Z, "lifecycle", l.Lifecycle, "active", l.IsActive)
		l.SetLifeCycle(-1, now, nil)
	}
}

// RevertLoc snaps a RESPAWN loc's CurrentInfo back to BaseInfo, swaps
// collision, emits a zone ChangeLoc event, and untracks the lifecycle.
// Mirrors TS World.revertLoc (Engine-TS/.../World.ts:1427-1448). Called
// from turnLoc for the RESPAWN+IsChanged+IsActive branch.
func (s *Server) RevertLoc(l *entitypkg.Loc) {
	if s.gamemap != nil && s.locTypes != nil {
		if oldLt := s.locTypeOrNil(l.Type()); oldLt != nil && oldLt.BlockWalk {
			s.gamemap.ChangeLocCollision(l.Shape(), l.Angle(), oldLt.BlockRange,
				l.Length, l.Width, oldLt.Active, l.X, l.Z, l.Level, false)
		}
	}
	l.Revert()
	if s.gamemap != nil && s.locTypes != nil {
		if newLt := s.locTypeOrNil(l.Type()); newLt != nil && newLt.BlockWalk {
			s.gamemap.ChangeLocCollision(l.Shape(), l.Angle(), newLt.BlockRange,
				l.Length, l.Width, newLt.Active, l.X, l.Z, l.Level, true)
		}
	}
	z := s.zoneMap.Get(l.Level, l.X, l.Z)
	z.ChangeLoc(l)
	// TS-faithful tail order (World.ts:1445-1447): SetLifeCycle(-1) BEFORE
	// TrackZone, the inverse of AddLoc/ChangeLoc/RemoveLoc. The two writes
	// touch independent data structures (locObjTracker vs zonesTracking)
	// so the order is observably equivalent — preserving the TS sequence
	// for audit clarity.
	l.SetLifeCycle(-1, s.currentTick, nil)
	s.TrackZone(z)
}
