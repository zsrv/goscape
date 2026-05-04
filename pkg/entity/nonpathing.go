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
}

// Parent returns the back-pointer set at construction. Bundle 2 of
// NAI-86 type-asserts on the result inside Server.processZones to
// dispatch turnLoc / (future) turnObj.
func (np *NonPathing) Parent() any { return np.parent }

// SetLifeCycle is the duration-aware lifecycle override that registers
// the entity in a LifecycleTracker. Bundle 2 of NAI-86 lands the
// tracker; this stub records the transition tick only and ignores the
// tracker arg.
//
// TODO(NAI-86 Bundle 2): rewire to call tracker.Register / Unregister
// and remove this stub doc-line.
func (np *NonPathing) SetLifeCycle(duration, currentTick int, tracker any) {
	if duration > 0 {
		np.SetLifecycle(currentTick+duration, currentTick)
	} else {
		np.SetLifecycle(-1, currentTick)
	}
}
