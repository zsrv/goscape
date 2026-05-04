package entity

// NonPathing is the shared concrete base for entities that don't walk —
// Locs and Objs. Exists to give zone code a single embedded base that
// future zone-event machinery can key against via interface satisfaction.
type NonPathing struct {
	Entity

	// parent is a back-pointer to the concrete *Loc / *Obj wrapping this
	// NonPathing. Populated by NewLoc / NewObj. Bundle 2's lifecycle
	// tracker iterates *NonPathing handles and recovers the concrete
	// entity through this field.
	parent any

	// tracker holds the LifecycleTracker the entity is currently
	// registered in. Used by SetLifeCycle to Unregister the previous
	// node before re-registering.
	tracker LifecycleTracker
}

// Parent returns the back-pointer set at construction. Bundle 2 of
// NAI-86 type-asserts on the result inside Server.processZones to
// dispatch turnLoc / (future) turnObj.
func (np *NonPathing) Parent() any { return np.parent }

// SetLifeCycle schedules the entity's next lifecycle transition at
// currentTick + duration and (de)registers it in the supplied
// LifecycleTracker. duration <= 0 untracks. Mirrors TS
// NonPathingEntity.setLifeCycle (Engine-TS/.../NonPathingEntity.ts:11-25).
//
// Distinct from [Entity.SetLifecycle](transitionTick, currentTick): this method
// takes a duration relative to currentTick and (de)registers in a
// LifecycleTracker. The casing difference is deliberate and mirrors TS
// setLifeCycle vs setLifecycle.
//
// Idempotent: a second call always Unregisters the previous tracker
// node before registering the new one, even if the tracker arg is the
// same pointer. duration <= 0 with tracker=nil is the "untrack only"
// shape used by Server.RevertLoc and the no-op-static-change branch
// of Server.ChangeLoc.
func (np *NonPathing) SetLifeCycle(duration, currentTick int, tracker LifecycleTracker) {
	if np.tracker != nil {
		np.tracker.Unregister(np)
		np.tracker = nil
	}
	if duration > 0 {
		// Defensive nil-tracker guard (goscape; TS skips this check): in the
		// fully-wired server tracker is always non-nil when duration>0, but
		// test fixtures may build a NonPathing without going through Server.
		// SetLifecycle still records the absolute transition tick.
		if tracker != nil {
			tracker.Register(np)
			np.tracker = tracker
		}
		np.SetLifecycle(currentTick+duration, currentTick)
	} else {
		np.SetLifecycle(-1, currentTick)
	}
}
