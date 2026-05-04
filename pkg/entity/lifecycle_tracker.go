package entity

// LifecycleTracker is the back-channel that NonPathing.SetLifeCycle
// uses to (de)register entities for per-tick processing. The concrete
// implementation lives in modules/world (it owns the doubly-linked
// list and the per-NonPathing element back-pointer); pkg/entity stays
// dependency-free.
//
// Mirrors the role of TS World.locObjTracker: each entity with
// duration > 0 is tracked; SetLifeCycle calls during the tracker's
// iteration must be reentrant-safe (Server.processZones snapshots
// before iterating to avoid mid-iteration mutation).
type LifecycleTracker interface {
	Register(np *NonPathing)
	Unregister(np *NonPathing)
}
