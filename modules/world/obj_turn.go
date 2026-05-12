package world

import (
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/zone"
)

// turnObj is the per-tick dispatch for a tracked Obj. Called from
// Server.processZones for each NonPathing in s.locObjTracker whose
// Parent() is a *Obj. Mirrors TS Obj.turn
// (Engine-TS/src/engine/entity/Obj.ts:27-50).
//
// Two independent arms:
//  1. Reveal countdown — fires every tick the obj is tracked,
//     independent of the lifecycle arm. On hitting 0, dispatches
//     OBJ_REVEAL via the receiver's player slot (0 if the receiver
//     logged out, matching TS `?? 0`).
//  2. Lifecycle arm — fires only when LifecycleTick == now per
//     DEVIATION-NAI-86-D-N86-4 (absolute tick vs TS per-tick
//     decrement; observably identical; see turnLoc for the matching
//     label on the Loc side). No negative-tick branch is needed —
//     impossible under the absolute-tick model.
func (s *Server) turnObj(o *entitypkg.Obj, now int) {
	// Arm 1: reveal countdown
	if o.Reveal > -1 {
		o.Reveal--
		if o.Reveal == 0 {
			slot := 0
			if p := s.LookupPlayerByUID(o.ReceiverID); p != nil {
				slot = p.Slot()
			}
			s.RevealObj(o, slot)
		}
	}

	// Arm 2: lifecycle (absolute-tick gate)
	if o.LifecycleTick != now {
		return
	}
	switch {
	case o.Lifecycle == entitypkg.LifecycleDespawn && o.IsActive:
		s.RemoveObj(o)
	case o.Lifecycle == entitypkg.LifecycleRespawn && !o.IsActive:
		s.AddObj(o, zone.PublicReceiver, 0)
	default:
		s.log.Error("obj tracked but no event matched",
			"type", o.Type, "x", o.X, "z", o.Z,
			"lifecycle", o.Lifecycle, "active", o.IsActive)
		o.SetLifeCycle(-1, now, nil)
	}
}
